package controller

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
	"kolihub.io/koli/pkg/apis/core/v1alpha1/draft"
	"kolihub.io/koli/pkg/util"
)

// SecretController generates dynamic secrets through namespaces. It manages secrets
// which have specific expiration time, usually used for machine-to-machine communication
// with limited access throughout the platform
type SecretController struct {
	kclient kubernetes.Interface

	nsInf cache.SharedIndexInformer
	skInf cache.SharedIndexInformer

	queue    *TaskQueue
	recorder record.EventRecorder

	jwtSecret string
}

// NewSecretController creates a new SecretController
func NewSecretController(nsInf, skInf cache.SharedIndexInformer, client kubernetes.Interface, jwtSecret string) *SecretController {
	c := &SecretController{
		kclient:   client,
		nsInf:     nsInf,
		skInf:     skInf,
		recorder:  newRecorder(client, "secret-controller"),
		jwtSecret: jwtSecret,
	}
	c.queue = NewTaskQueue(c.syncHandler)
	c.nsInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addNamespace,
		UpdateFunc: c.updateNamespace,
	})
	return c
}

func (c *SecretController) addNamespace(n interface{}) {
	new := n.(*v1.Namespace)
	glog.V(2).Infof("%s(%s) - add-namespace", new.Name, new.ResourceVersion)
	c.queue.Add(new)
}

func (c *SecretController) updateNamespace(o, n interface{}) {
	new := n.(*v1.Namespace)
	glog.V(2).Infof("%s(%s) - update-namespace", new.Name, new.ResourceVersion)
	c.queue.Add(new)
}

// Run the controller.
func (c *SecretController) Run(workers int, stopc <-chan struct{}) {
	// don't let panics crash the process
	defer utilruntime.HandleCrash()
	// make sure the work queue is shutdown which will trigger workers to end
	defer c.queue.shutdown()

	glog.Info("Starting Secret controller...")

	if !cache.WaitForCacheSync(stopc, c.nsInf.HasSynced, c.skInf.HasSynced) {
		return
	}

	for i := 0; i < workers; i++ {
		// runWorker will loop until "something bad" happens.
		// The .Until will then rekick the worker after one second
		go c.queue.run(time.Second, stopc)
	}

	// wait until we're told to stop
	<-stopc
	glog.Info("Shutting down Secret controller")
}

func (c *SecretController) syncHandler(key string) error {
	obj, exists, err := c.nsInf.GetStore().GetByKey(key)
	if err != nil {
		glog.Warningf("%s - failed retrieving object from store [%s]", key, err)
		return err
	}
	if !exists {
		glog.V(3).Infof("%s - the namespace doesn't exists", key)
		return nil
	}
	ns := obj.(*v1.Namespace)
	if ns.DeletionTimestamp != nil {
		glog.V(3).Infof("%s - object marked for deletion, skip ...", key)
		return nil
	}

	nsMeta := draft.NewNamespaceMetadata(ns.Name)
	if !nsMeta.IsValid() {
		glog.V(3).Infof("%s - it's not a valid namespace, skip ...", key)
		return nil
	}

	// Verify the last time an object was synced, using a shared informer doesn't permit having a
	// custom resync time, thus veryfing if an object can perform a sync operation is important.
	// This validation prevents starving resources from the api server
	obj, exists, err = c.skInf.GetStore().GetByKey(fmt.Sprintf("%s/%s", key, platform.SystemSecretName))
	if err != nil {
		glog.Warningf("%s/%s - failed retrieving secret from store [%v]", key, platform.SystemSecretName, err)
	}
	if exists {
		glog.V(5).Infof("%s/%s - secret exists in cache", key, platform.SystemSecretName)
		s := obj.(*v1.Secret)
		if s.Annotations != nil {
			lastUpdated, err := time.Parse(time.RFC3339, s.Annotations[platform.AnnotationSecretLastUpdated])
			if err == nil && lastUpdated.Add(time.Minute*20).After(time.Now().UTC()) {
				glog.V(3).Infof(`%s/%s - to soon for updating secret "%s"`, key, platform.SystemSecretName, lastUpdated.Format(time.RFC3339))
				return nil
			}
			if err != nil {
				glog.Warningf("%s - got error converting time [%v]", key, err)
			}
		}
	}
	// Generate a system token based on the customer and organization of the namespace.
	// The access token has limited access to the resources of the platform
	tokenString, err := util.GenerateNewJwtToken(
		c.jwtSecret,
		nsMeta.Customer(),
		nsMeta.Organization(),
		platform.SystemTokenType,
		time.Now().UTC().Add(time.Hour*1), // hard-coded exp time
	)
	if err != nil {
		glog.Warningf("%s - failed generating system token [%v]", key, err)
		return nil
	}

	// Patching an object primarily is less expensive because the most part of
	// secret resources will be synced in each resync
	_, err = c.kclient.Core().Secrets(ns.Name).Patch(
		platform.SystemSecretName,
		types.StrategicMergePatchType,
		genSystemTokenPatchData(tokenString),
	)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed updating secret [%v]", err)
	}
	if err == nil {
		glog.Infof(`%s/%s secret updated with success`, key, platform.SystemSecretName)
		return nil
	}

	_, err = c.kclient.Core().Secrets(ns.Name).Create(&v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns.Name,
			Name:      platform.SystemSecretName,
			Labels:    map[string]string{platform.LabelSecretController: "true"},
			Annotations: map[string]string{
				platform.AnnotationSecretLastUpdated: time.Now().UTC().Format(time.RFC3339),
			},
		},
		Data: map[string][]byte{
			"token.jwt": bytes.NewBufferString(tokenString).Bytes(),
		},
		Type: v1.SecretTypeOpaque,
	})
	if err != nil {
		return fmt.Errorf("failed creating secret [%v]", err)
	}
	glog.Infof("%s/%s secret created with success", key, platform.SystemSecretName)
	return nil
}

func genSystemTokenPatchData(token string) []byte {
	return []byte(
		fmt.Sprintf(
			`{"metadata": {"labels": {"%s": "true"}, "annotations": {"%s": "%s"}}, "data": {"token.jwt": "%s"}, "type": "Opaque"}`,
			platform.LabelSecretController,
			platform.AnnotationSecretLastUpdated,
			time.Now().UTC().Format(time.RFC3339),
			base64.StdEncoding.EncodeToString(bytes.NewBufferString(token).Bytes()),
		),
	)
}
