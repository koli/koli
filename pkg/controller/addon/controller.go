package addon

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/spec"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	extensions "k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	utilruntime "k8s.io/client-go/1.5/pkg/util/runtime"
	"k8s.io/client-go/1.5/pkg/util/wait"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

const (
	tprAddons = "addon.sys.koli.io"
)

type queue struct {
	ch chan *spec.Addon
}

func newQueue(size int) *queue {
	return &queue{ch: make(chan *spec.Addon, size)}
}

func (q *queue) add(p *spec.Addon) { q.ch <- p }
func (q *queue) close()            { close(q.ch) }

func (q *queue) pop() (*spec.Addon, bool) {
	p, ok := <-q.ch
	return p, ok
}

// Config defines configuration parameters for the Operator.
type Config struct {
	Host        string
	TLSInsecure bool
	TLSConfig   rest.TLSClientConfig
}

// Operator manages lifecycle of ...
type Operator struct {
	kclient *kubernetes.Clientset
	pclient *rest.RESTClient

	addonInf cache.SharedIndexInformer
	psetInf  cache.SharedIndexInformer

	queue *queue

	host string
}

// New creates a new controller.
func New(c Config) (*Operator, error) {
	cfg, err := newClusterConfig(c.Host, c.TLSInsecure, &c.TLSConfig)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	sysclient, err := newSysRESTClient(*cfg)
	if err != nil {
		return nil, err
	}
	return &Operator{
		kclient: client,
		pclient: sysclient,
		queue:   newQueue(200),
		host:    cfg.Host,
	}, nil
}

// Run the controller.
func (c *Operator) Run(workers int, stopc <-chan struct{}) error {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.close()

	glog.Info("Starting addon controller...")

	_, err := c.kclient.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("communicating with server failed: %s", err)
	}
	if err := c.createTPRs(); err != nil {
		return err
	}

	c.addonInf = cache.NewSharedIndexInformer(
		NewSysListWatch(c.pclient),
		&spec.Addon{}, resyncPeriod, cache.Indexers{},
	)
	// depSelector := fields.OneTermEqualSelector("sys.io/type", "addon")

	c.psetInf = cache.NewSharedIndexInformer(
		cache.NewListWatchFromClient(c.kclient.Apps().GetRESTClient(), "petsets", api.NamespaceAll, nil),
		&v1alpha1.PetSet{}, resyncPeriod, cache.Indexers{},
	)

	c.addonInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(a interface{}) {
			addon := a.(*spec.Addon)
			glog.Infof("CREATE ADDON: (%s/%s), spec.type (%s)", addon.Namespace, addon.Name, addon.Spec.Type)
			c.enqueueAddon(addon)
		},
		DeleteFunc: func(a interface{}) {
			addon := a.(*spec.Addon)
			glog.Infof("DELETE ADDON: (%s/%s), spec.type (%s)", addon.Namespace, addon.Name, addon.Spec.Type)
			c.enqueueAddon(addon)
		},
		UpdateFunc: func(o, a interface{}) {
			old := o.(*spec.Addon)
			act := a.(*spec.Addon)

			if old.ResourceVersion == act.ResourceVersion {
				return
			}

			glog.Infof("UPDATE ADDON: (%s/%s), spec.type (%s)", act.Namespace, act.Name, act.Spec.Type)
			c.enqueueAddon(act)
		},
	})

	c.psetInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: func(a interface{}) {
			d := a.(*v1alpha1.PetSet)
			glog.Infof("deleteDeployment: (%s/%s)", d.Namespace, d.Name)
			if addon := c.addonForDeployment(d); addon != nil {
				c.enqueueAddon(addon)
			}
		},
		UpdateFunc: func(o, a interface{}) {
			old := o.(*v1alpha1.PetSet)
			act := a.(*v1alpha1.PetSet)
			// Periodic resync may resend the deployment without changes in-between.
			// Also breaks loops created by updating the resource ourselves.
			if old.ResourceVersion == act.ResourceVersion {
				return
			}

			glog.Infof("updateDeployment: (%s/%s)", act.Namespace, act.Name)
			if addon := c.addonForDeployment(act); addon != nil {
				c.enqueueAddon(addon)
			}
		},
	})

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go wait.Until(c.runWorker, time.Second, stopc)
	}

	go c.addonInf.Run(stopc)
	go c.psetInf.Run(stopc)

	if !cache.WaitForCacheSync(stopc, c.addonInf.HasSynced, c.psetInf.HasSynced) {
		return fmt.Errorf("stop requested")
	}

	// wait until we're told to stop
	<-stopc
	return nil
}

var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

func (c *Operator) enqueueAddon(addon *spec.Addon) {
	c.queue.add(addon)
}

// runWorker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *Operator) runWorker() {
	for {
		a, ok := c.queue.pop()
		if !ok {
			return
		}
		// Get the app based on its type
		app, err := a.GetApp(c.kclient, c.addonInf, c.psetInf)
		if err != nil {
			// If an add-on is provided without a known type
			utilruntime.HandleError(err)
		}
		if err := c.reconcile(app); err != nil {
			utilruntime.HandleError(err)
		}
	}
}

func (c *Operator) reconcile(app spec.AddonInterface) error {
	key, err := keyFunc(app.GetAddon())
	if err != nil {
		return err
	}

	_, exists, err := c.addonInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}
	if !exists {
		// TODO: we want to do server side deletion due to the variety of
		// resources we create.
		// Doing so just based on the deletion event is not reliable, so
		// we have to garbage collect the controller-created resources in some other way.
		//
		// Let's rely on the index key matching that of the created configmap and replica
		// set for now. This does not work if we delete addon resources as the
		// controller is not running – that could be solved via garbage collection later.
		//
		// TODO(san): Maybe deleting a petset on controller is not appropriate.
		// See the controller kubernetes implementation of a Deployment controller and how
		// the kubectl deals removing those kind of resources.
		glog.Infof("deleting deployment (%v) ...", key)
		return app.DeleteApp()
	}

	if err := app.CreateConfigMap(); err != nil {
		return err
	}

	// Ensure we have a replica set running
	psetQ := &v1alpha1.PetSet{}
	psetQ.Namespace = app.GetAddon().Namespace
	psetQ.Name = app.GetAddon().Name

	obj, exists, err := c.psetInf.GetStore().Get(psetQ)
	if err != nil {
		return err
	}

	if !exists {
		if err := app.CreatePetSet(); err != nil {
			return fmt.Errorf("failed creating petset (%s)", err)
		}
		return nil
	}
	return app.UpdatePetSet(obj.(*v1alpha1.PetSet))
}

func (c *Operator) addonForDeployment(p *v1alpha1.PetSet) *spec.Addon {
	key, err := keyFunc(p)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("creating key: %s", err))
		return nil
	}

	// Namespace/Name are one-to-one so the key will find the respective Addon resource.
	a, exists, err := c.addonInf.GetStore().GetByKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("get Addon resource: %s", err))
		return nil
	}
	if !exists {
		return nil
	}
	return a.(*spec.Addon)
}

func (c *Operator) createTPRs() error {
	tprs := []*extensions.ThirdPartyResource{
		{
			ObjectMeta: v1.ObjectMeta{
				Name: tprAddons,
			},
			Versions: []extensions.APIVersion{
				{Name: "v1alpha1"},
			},
			Description: "Addon external service integration",
		},
	}
	tprClient := c.kclient.Extensions().ThirdPartyResources()
	for _, tpr := range tprs {
		if _, err := tprClient.Create(tpr); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
		glog.Infof("Third Party Resource '%s' provisioned", tpr.Name)
	}

	// We have to wait for the TPRs to be ready. Otherwise the initial watch may fail.
	return wait.Poll(3*time.Second, 30*time.Second, func() (bool, error) {
		resp, err := c.kclient.CoreClient.Client.Get(c.host + "/apis/sys.koli.io/v1alpha1/addons")
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK:
			return true, nil
		case http.StatusNotFound: // not set up yet. wait.
			return false, nil
		default:
			return false, fmt.Errorf("invalid status code: %v", resp.Status)
		}
	})
}

func newClusterConfig(host string, tlsInsecure bool, tlsConfig *rest.TLSClientConfig) (*rest.Config, error) {
	var cfg *rest.Config
	var err error

	if len(host) == 0 {
		if cfg, err = rest.InClusterConfig(); err != nil {
			return nil, err
		}
	} else {
		cfg = &rest.Config{
			Host: host,
		}
		hostURL, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("error parsing host url %s : %v", host, err)
		}
		if hostURL.Scheme == "https" {
			cfg.TLSClientConfig = *tlsConfig
			cfg.Insecure = tlsInsecure
		}
	}
	cfg.QPS = 100
	cfg.Burst = 100

	return cfg, nil
}
