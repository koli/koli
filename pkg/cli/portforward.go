package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"

	koliutil "github.com/kolibox/koli/pkg/cli/util"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/api"
	coreclient "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset/typed/core/internalversion"
	"k8s.io/kubernetes/pkg/client/restclient"

	"k8s.io/kubernetes/pkg/client/unversioned/portforward"
	"k8s.io/kubernetes/pkg/client/unversioned/remotecommand"
	cmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

// PortForwardOptions contains all the options for running the port-forward cli command.
type PortForwardOptions struct {
	Namespace     string
	PodName       string
	Config        *restclient.Config
	PodClient     coreclient.PodsGetter
	Ports         []string
	PortForwarder portForwarder
	StopChannel   chan struct{}
	ReadyChannel  chan struct{}
}

var (
	portforwardExample = dedent.Dedent(`
		# Listen on ports 5000 and 6000 locally, forwarding data to/from ports 5000 and 6000 in the pod
		kubectl port-forward mypod 5000 6000

		# Listen on port 8888 locally, forwarding to 5000 in the pod
		kubectl port-forward mypod 8888:5000

		# Listen on a random port locally, forwarding to 5000 in the pod
		kubectl port-forward mypod :5000

		# Listen on a random port locally, forwarding to 5000 in the pod
		kubectl port-forward  mypod 0:5000`)
)

// NewCmdPortForward .
func NewCmdPortForward(comm *koliutil.CommandParams) *cobra.Command {
	opts := &PortForwardOptions{
		PortForwarder: &defaultPortForwarder{
			cmdOut: comm.Out,
			cmdErr: comm.Err,
		},
	}
	cmd := &cobra.Command{
		Use:     "port-forward POD [LOCAL_PORT:]REMOTE_PORT [...[LOCAL_PORT_N:]REMOTE_PORT_N]",
		Short:   "Forward one or more local ports to a pod",
		Long:    "Forward one or more local ports to a pod.",
		Example: portforwardExample,
		Run: func(cmd *cobra.Command, args []string) {
			comm.Cmd = cmd
			if err := opts.Complete(comm, args); err != nil {
				cmdutil.CheckErr(err)
			}
			if err := opts.Validate(); err != nil {
				cmdutil.CheckErr(cmdutil.UsageError(cmd, err.Error()))
			}
			if err := opts.RunPortForward(); err != nil {
				cmdutil.CheckErr(err)
			}
		},
	}
	cmd.Flags().StringP("pod", "p", "", "Pod name")
	// TODO support UID
	return cmd
}

type portForwarder interface {
	ForwardPorts(method string, url *url.URL, opts PortForwardOptions) error
}

type defaultPortForwarder struct {
	cmdOut, cmdErr io.Writer
}

func (f *defaultPortForwarder) ForwardPorts(method string, url *url.URL, opts PortForwardOptions) error {
	dialer, err := remotecommand.NewExecutor(opts.Config, method, url)
	if err != nil {
		return err
	}
	fw, err := portforward.New(dialer, opts.Ports, opts.StopChannel, opts.ReadyChannel, f.cmdOut, f.cmdErr)
	if err != nil {
		return err
	}
	return fw.ForwardPorts()
}

// Complete completes all the required options for port-forward cmd.
func (o *PortForwardOptions) Complete(comm *koliutil.CommandParams, args []string) error {
	o.PodName = cmdutil.GetFlagString(comm.Cmd, "pod")
	if len(o.PodName) == 0 && len(args) == 0 {
		return cmdutil.UsageError(comm.Cmd, "POD is required for port-forward")
	}

	if len(o.PodName) != 0 {
		o.Ports = args
	} else {
		o.PodName = args[0]
		o.Ports = args[1:]
	}

	err := koliutil.SetNamespacePrefix(comm.Cmd.Flag("namespace"), comm.User().ID)
	if err != nil {
		return err
	}
	o.Namespace, err = comm.Factory.DefaultNamespace(comm.Cmd, true)
	if err != nil {
		return err
	}

	o.PodClient, err = comm.KFactory().ClientSet()
	if err != nil {
		return err
	}

	o.Config, err = comm.KFactory().ClientConfig()
	if err != nil {
		return err
	}

	o.StopChannel = make(chan struct{}, 1)
	o.ReadyChannel = make(chan struct{})
	return nil
}

// Validate validates all the required options for port-forward cmd.
func (o PortForwardOptions) Validate() error {
	if len(o.PodName) == 0 {
		return fmt.Errorf("pod name must be specified")
	}

	if len(o.Ports) < 1 {
		return fmt.Errorf("at least 1 PORT is required for port-forward")
	}

	if o.PortForwarder == nil || o.PodClient == nil || o.Config == nil {
		return fmt.Errorf("client, client config, and portforwarder must be provided")
	}
	return nil
}

// RunPortForward implements all the necessary functionality for port-forward cmd.
func (o PortForwardOptions) RunPortForward() error {
	pod, err := o.PodClient.Pods(o.Namespace).Get(o.PodName)
	if err != nil {
		return err
	}

	if pod.Status.Phase != api.PodRunning {
		return fmt.Errorf("unable to forward port because pod is not running. Current status=%v", pod.Status.Phase)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	go func() {
		<-signals
		if o.StopChannel != nil {
			close(o.StopChannel)
		}
	}()

	restClient, err := restclient.RESTClientFor(o.Config)
	if err != nil {
		return err
	}

	req := restClient.Post().
		Resource("pods").
		Namespace(o.Namespace).
		Name(pod.Name).
		SubResource("portforward")

	return o.PortForwarder.ForwardPorts("POST", req.URL(), o)
}
