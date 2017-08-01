package receive

import (
	"bufio"
	"fmt"
	"log"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/rest"
	platform "kolihub.io/koli/pkg/apis/v1alpha1"
	"kolihub.io/koli/pkg/apis/v1alpha1/draft"
	"kolihub.io/koli/pkg/clientset"
	"kolihub.io/koli/pkg/git/k8s"
	gitutil "kolihub.io/koli/pkg/git/util"
)

// Run executes the git-receive hook
func Run(conf *Config, oldRev, newRev, refName string) error {
	var err error

	isNewBranch := false
	if oldRev == "0000000000000000000000000000000000000000" {
		isNewBranch = true
	}

	client, err := clientset.NewKubernetesClient(&rest.Config{Host: conf.Host})
	// clientset, err := clientset.NewKuber(conf.Host)
	if err != nil {
		return fmt.Errorf("failed retrieving clientset (%s)", err)
	}

	gitSha, err := draft.NewSha(newRev)
	if err != nil {
		return err
	}
	obj, err := client.Extensions().Deployments(conf.Namespace).Get(conf.DeployName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed retrieving deployment: %s", err)
	}
	dp := draft.NewDeployment(obj)
	buildRevision := dp.BuildRevision() + 1
	dp.SetAnnotation(platform.AnnotationBuildRevision, strconv.Itoa(buildRevision))
	// TODO: check if kolihub.io/build == true, means that a build was already started
	// if dp.Annotations == nil {
	// 	dp.Annotations = map[string]string{
	// 		constants.BuildRevisionKey: strconv.Itoa(buildRevision),
	// 		constants.AutoDeployKey:    "true",
	// 	}
	// } else {
	// 	// TODO: try to recover the revision number
	// 	buildRevision, _ = strconv.Atoi(dp.Annotations[constants.BuildRevisionKey])
	// 	buildRevision = buildRevision + 1
	// 	dp.Annotations[constants.BuildRevisionKey] = strconv.Itoa(buildRevision)
	// }
	gitTask := gitutil.NewServerTask(conf.GitHome, gitutil.NewObjectMeta(dp.Name, dp.Namespace))

	dp.Annotations[platform.AnnotationBuild] = "true"
	dp.Annotations[platform.AnnotationGitRepository] = gitTask.GetRepository()
	dp.Annotations[platform.AnnotationGitRemote] = conf.GitAPIHostname
	dp.Annotations[platform.AnnotationGitRevision] = gitSha.Full()
	// dp.Annotations[platform.AnnotationAuthToken] = conf.UserJwtToken
	dp.Annotations[platform.AnnotationBuildSource] = "local"

	if _, err := client.Extensions().Deployments(dp.Namespace).Update(dp.GetObject()); err != nil {
		return fmt.Errorf("failed starting build: %s", err)
	}

	// TODO: accept branch with slashes. E.g.: <ref>/<branch>
	// accept the newRev creating the refs/head/<branch>, otherwise it will fail to clone when building it
	if err := gitTask.WriteBranchRef(refName, newRev); err != nil {
		return fmt.Errorf("failed writing ref: %s", err)
	}
	if isNewBranch {
		// remove it otherwise the git will fail to create the new ref
		defer gitTask.RemoveBranchRef(refName)
	} else {
		// update to the old one, otherwise the git will fail to update the ref with the new one
		defer gitTask.WriteBranchRef(refName, oldRev)
	}

	pw := k8s.NewPodWatcher(client, dp.Namespace)

	stopCh := make(chan struct{})
	defer close(stopCh)
	go pw.Controller.Run(stopCh)

	fmt.Printf("Starting build... but first, coffee!\n")
	if err := waitForPod(
		pw, dp.Namespace,
		gitSha.Short(),
		strconv.Itoa(buildRevision),
		conf.SessionIdleInterval(),
		conf.BuilderPodTickDuration(),
		conf.BuilderPodWaitDuration(),
	); err != nil {
		return fmt.Errorf("watching events for builder pod startup (%s)", err)
	}

	buildPodName := fmt.Sprintf("sb-%s-v%d", dp.Name, buildRevision)
	req := client.Core().RESTClient().
		Get().
		Namespace(dp.Namespace).
		Name(buildPodName).
		Resource("pods").
		SubResource("log").
		VersionedParams(&api.PodLogOptions{
			Follow: true,
		}, api.ParameterCodec)

	rc, err := req.Stream()
	defer rc.Close()
	if err != nil {
		return fmt.Errorf("attempting to stream logs (%s)", err)
	}

	// Stream logs to stdout in real time
	logReader := bufio.NewReader(rc)
	for {
		line, err := logReader.ReadBytes('\n')
		if err != nil {
			break // EOF
		}
		log.Print(string(line))
	}

	// check the state and exit code of the build pod.
	// if the code is not 0 return error
	if err := waitForPodEnd(
		pw,
		dp.Namespace,
		gitSha.Short(),
		strconv.Itoa(buildRevision),
		conf.BuilderPodTickDuration(),
		conf.BuilderPodWaitDuration(),
	); err != nil {
		return fmt.Errorf("error getting builder pod status (%s)", err)
	}

	buildPod, err := client.Core().Pods(dp.Namespace).Get(buildPodName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("error getting builder pod status (%s)", err)
	}

	for _, containerStatus := range buildPod.Status.ContainerStatuses {
		state := containerStatus.State.Terminated
		if state.ExitCode != 0 {
			return fmt.Errorf("Build pod exited with code %d, stopping build.", state.ExitCode)
		}
	}
	return nil
}
