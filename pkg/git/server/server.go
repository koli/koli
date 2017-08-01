package server

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/git/conf"
	gitutil "kolihub.io/koli/pkg/git/util"
)

type gitService struct {
	method     string
	suffix     string
	handleFunc func(gitEnv, string, string, http.ResponseWriter, *http.Request) bool
	rpc        string
}

type gitEnv struct {
	UserJwtToken   string
	DeployName     string
	Namespace      string
	GitHome        string
	KubeSvcHost    string
	GitAPIHostname string
}

// Routing table
var gitServices = [...]gitService{
	{"GET", "/info/refs", handleGetInfoRefs, ""},
	{"POST", "/git-upload-pack", handlePostRPC, "git-upload-pack"},
	{"POST", "/git-receive-pack", handlePostRPC, "git-receive-pack"},
}

// Handler .
type Handler struct {
	//controller controller.Client
	user      *platform.User
	cnf       *conf.Config
	clientset *kubernetes.Clientset
}

// NewHandler .
func NewHandler(cnf *conf.Config, clientset *kubernetes.Clientset) Handler {
	fmt.Println("Starting http server...")
	return Handler{cnf: cnf, clientset: clientset}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var g gitService
	// Look for a matching Git service
	foundService := false
	for _, g = range gitServices {
		if r.Method == g.method && strings.HasSuffix(r.URL.Path, g.suffix) {
			foundService = true
			break
		}
	}
	repo := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, g.suffix), "/"), ".git")
	// Expecting: namespace/myreponame
	splittedRepo := strings.Split(repo, "/")
	if len(splittedRepo) != 2 {
		http.Error(w, "missing namespace. remotes must be in the form: <namespace>/<repository>", 400)
		return
	}
	// TODO: validate if the namespace belongs to the user
	namespace, repo := splittedRepo[0], splittedRepo[1]
	fmt.Printf("Repo Name: %s Namespace: %s \n", repo, namespace)
	if !foundService || !regexp.MustCompile(`^[a-z\d]+(-[a-z\d]+)*$`).MatchString(repo) {
		// The protocol spec in git/Documentation/technical/http-protocol.txt
		// says we must return 403 if no matching service is found.
		http.Error(w, "Forbidden", 403)
		return
	}

	dp, err := h.clientset.Extensions().Deployments(namespace).Get(repo, metav1.GetOptions{})
	if err != nil {
		w.Header().Set("WWW-Authenticate", "Basic")
		msg := fmt.Sprintf("failed retrieving deployment: %s", err)
		http.Error(w, msg, 400)
		return
	}

	gitTask := gitutil.NewServerTask(h.cnf.GitHome, gitutil.NewObjectMeta(dp.Name, dp.Namespace))
	log.Printf("creating repo directory %s", gitTask.FullRepoPath())
	if _, err := gitTask.InitRepository(); err != nil {
		fail500(w, "createRepo", err)
		return
	}

	fmt.Printf("Write update Hook on path: %s\n", gitTask.FullRepoPath())
	if err := writeRepoHook(gitTask.FullRepoPath(), "update", updateHook); err != nil {
		fmt.Println("Error writing update hook")
		fail500(w, "writeUpdateHook", err)
		return
	}

	// The authentication was already validated, should not return error
	// _, jwtTokenString, _ := r.BasicAuth()
	env := gitEnv{
		Namespace:  namespace,
		DeployName: dp.Name,
		// UserJwtToken:   jwtTokenString,
		GitHome:        h.cnf.GitHome,
		KubeSvcHost:    h.cnf.Host,
		GitAPIHostname: h.cnf.GitAPIHostname,
	}
	success := g.handleFunc(env, g.rpc, gitTask.FullRepoPath(), w, r)
	if success && g.rpc == "git-receive-pack" {
		// TODO: do extra stuff
	}
}

func handleGetInfoRefs(env gitEnv, _ string, path string, w http.ResponseWriter, r *http.Request) bool {
	rpc := r.URL.Query().Get("service")
	if !(rpc == "git-upload-pack" || rpc == "git-receive-pack") {
		// The 'dumb' Git HTTP protocol is not supported
		http.Error(w, "Not Found", 404)
		return false
	}
	// Prepare our Git subprocess
	cmd, pipe := gitCommand(env, "git", subCommand(rpc), "--stateless-rpc", "--advertise-refs", path)
	if err := cmd.Start(); err != nil {
		fail500(w, "handleGetInfoRefs", err)
		return false
	}
	defer cleanUpProcessGroup(cmd) // Ensure brute force subprocess clean-up

	// Start writing the response
	w.Header().Add("Content-Type", fmt.Sprintf("application/x-%s-advertisement", rpc))
	w.Header().Add("Cache-Control", "no-cache")
	w.WriteHeader(200) // Don't bother with HTTP 500 from this point on, just return
	if err := pktLine(w, fmt.Sprintf("# service=%s\n", rpc)); err != nil {
		logError(w, "handleGetInfoRefs response", err)
		return false
	}
	if err := pktFlush(w); err != nil {
		logError(w, "handleGetInfoRefs response", err)
		return false
	}
	if _, err := io.Copy(w, pipe); err != nil {
		logError(w, "handleGetInfoRefs read from subprocess", err)
		return false
	}
	if err := cmd.Wait(); err != nil {
		logError(w, "handleGetInfoRefs wait for subprocess", err)
		return false
	}

	return true
}

func handlePostRPC(env gitEnv, rpc string, path string, w http.ResponseWriter, r *http.Request) bool {
	// The client request body may have been gzipped.
	body := r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		var err error
		body, err = gzip.NewReader(r.Body)
		if err != nil {
			fail500(w, "handlePostRPC", err)
			return false
		}
	}

	// Prepare our Git subprocess
	cmd, pipe := gitCommand(env, "git", subCommand(rpc), "--stateless-rpc", path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fail500(w, "handlePostRPC", err)
		return false
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		fail500(w, "handlePostRPC", err)
		return false
	}
	go func(done <-chan bool) {
		<-done
		cleanUpProcessGroup(cmd) // Ensure brute force subprocess clean-up
	}(w.(http.CloseNotifier).CloseNotify())

	// Write the client request body to Git's standard input
	if _, err := io.Copy(stdin, body); err != nil {
		fail500(w, "handlePostRPC write to subprocess", err)
		return false
	}

	// Start writing the response
	w.Header().Add("Content-Type", fmt.Sprintf("application/x-%s-result", rpc))
	w.Header().Add("Cache-Control", "no-cache")
	w.WriteHeader(200) // Don't bother with HTTP 500 from this point on, just return
	if _, err := io.Copy(newWriteFlusher(w), pipe); err != nil {
		logError(w, "handlePostRPC read from subprocess", err)
		return false
	}
	if err := cmd.Wait(); err != nil {
		logError(w, "handlePostRPC wait for subprocess", err)
		return false
	}

	return true
}

// https://git-scm.com/docs/githooks#update
var updateHook = []byte(`#!/bin/bash
set -eo pipefail;

refname="$1"
oldrev="$2"
newrev="$3"

gitreceiver --oldrev $oldrev --newrev $newrev --refname $refname
`)

func writeRepoHook(path, hook string, hookScript []byte) error {
	return ioutil.WriteFile(filepath.Join(path, "hooks", hook), hookScript, 0755)
}

// Git subprocess helpers
func subCommand(rpc string) string {
	return strings.TrimPrefix(rpc, "git-")
}

func gitCommand(env gitEnv, name string, args ...string) (*exec.Cmd, io.Reader) {
	cmd := exec.Command(name, args...)
	// Start the command in its own process group (nice for signalling)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Explicitly set the environment for the Git command
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("NAMESPACE=%s", env.Namespace),
		fmt.Sprintf("DEPLOY_NAME=%s", env.DeployName),
		// fmt.Sprintf("USER_JWT_TOKEN=%s", env.UserJwtToken),
		fmt.Sprintf("KUBERNETES_SERVICE_HOST=%s", env.KubeSvcHost),
		fmt.Sprintf("GIT_HOME=%s", env.GitHome),
		fmt.Sprintf("GIT_API_HOSTNAME=%s", env.GitAPIHostname),
		// TODO: not needed
		fmt.Sprintf("PLATFORM_CLIENT_SECRET=%s", ""),
	)

	r, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout

	return cmd, r
}

func cleanUpProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}

	process := cmd.Process
	if process != nil && process.Pid > 0 {
		// Send SIGTERM to the process group of cmd
		syscall.Kill(-process.Pid, syscall.SIGTERM)
	}

	// reap our child process
	go cmd.Wait()
}

// Git HTTP line protocol functions
func pktLine(w io.Writer, s string) error {
	_, err := fmt.Fprintf(w, "%04x%s", len(s)+4, s)
	return err
}

func pktFlush(w io.Writer) error {
	_, err := fmt.Fprint(w, "0000")
	return err
}

func newWriteFlusher(w http.ResponseWriter) io.Writer {
	return writeFlusher{w.(interface {
		io.Writer
		http.Flusher
	})}
}

type writeFlusher struct {
	wf interface {
		io.Writer
		http.Flusher
	}
}

func (w writeFlusher) Write(p []byte) (int, error) {
	defer w.wf.Flush()
	return w.wf.Write(p)
}

func fail500(w http.ResponseWriter, context string, err error) {
	http.Error(w, "Internal server error", 500)
	logError(w, context, err)
}

func logError(w http.ResponseWriter, msg string, err error) {
	log.Printf("context: %s error: %s", msg, err)
	// logger, _ := ctxhelper.LoggerFromContext(w.(*httphelper.ResponseWriter).Context())
	// logger.Error(msg, "error", err)
}
