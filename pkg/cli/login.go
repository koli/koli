package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/dickeyxxx/netrc"
	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

type authData struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// NewCmdLogin .
func NewCmdLogin(comm *koliutil.CommandParams, pathOptions *clientcmd.PathOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [(-u|--user) NAME]",
		Short: "Login with Koli credentials",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				cmd.Help()
				return
			}
			cfg, err := pathOptions.GetStartingConfig()

			cmdutil.CheckErr(err)
			err = isSignIn(comm.Controller(), cfg.CurrentContext)
			cmdutil.CheckErr(err)
		},
	}
	return cmd
}

func setLocalGitCredentials(gitHost, token string) error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("failed retrieving user home path (%s)", err)
	}
	netrcPath := filepath.Join(usr.HomeDir, ".netrc")
	if _, err := os.Stat(netrcPath); os.IsNotExist(err) {
		n := netrc.Netrc{Path: netrcPath}
		n.AddMachine(gitHost, "dunno", token)
		return n.Save()
	}
	n, err := netrc.Parse(netrcPath)
	if err != nil {
		return fmt.Errorf("failed parsing .netrc file (%s)", err)
	}
	machine := n.Machine(gitHost)
	if machine == nil {
		// The username does not matter for the platform
		n.AddMachine(gitHost, "dunno", token)
		return n.Save()
	}
	machine.Set("password", token)
	return n.Save()
}

func isSignIn(controller *koliutil.Controller, currentContext string) error {
	username, password := credentials()
	data, err := json.Marshal(authData{Username: username, Password: password})
	if err != nil {
		return err
	}
	result := controller.Request.POST().
		Resource("/login").
		Body(data).
		Do()
	if result.Error() != nil || result.StatusCode() != 200 {
		if result.StatusCode() == 401 {
			return errors.New("wrong credentials")
		}
		return fmt.Errorf("couldn't logging in (%d) (%#v)", result.StatusCode(), result.Error())
	}
	rawResponse, _ := result.Raw()
	var response map[string]interface{}
	if err := json.Unmarshal(rawResponse, &response); err != nil {
		return fmt.Errorf("error: couldn't decode response (%s)", err)
	}
	fmt.Print("logged in, saving credentials...")
	// TODO: There's a better way to achieve that behavior!
	// TODO: The currentContext is not the name of the user in the config file!
	cmd := exec.Command("koli", "config", "set-credentials", currentContext, "--token", response["token"].(string))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error: couldn't configure credentials (%s)", err)
	}
	// TODO: remove hard-coded host
	err = setLocalGitCredentials("crafter-orion.kolibox.io", response["token"].(string))
	if err != nil {
		fmt.Println("")
		return err
	}
	fmt.Println(" done.")
	return nil
}

func credentials() (string, string) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Enter your Koli credentials.")
	fmt.Print("Username/E-mail: ")
	username, _ := reader.ReadString('\n')
	fmt.Print("Password (typing will be hidden): ")
	bytePassword, _ := terminal.ReadPassword(0)
	password := string(bytePassword)
	fmt.Println("") // Break line
	return strings.TrimSpace(username), strings.TrimSpace(password)
}
