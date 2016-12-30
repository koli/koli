package controller

import (
	"time"

	"github.com/golang/glog"
	"github.com/kolibox/koli/pkg/platform"

	apierrors "k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/client/cache"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/util/wait"
)

var keyFunc = cache.DeletionHandlingMetaNamespaceKeyFunc

func watch3PRs(host, endpoint string, kclient clientset.Interface) error {
	return wait.Poll(1*time.Second, 30*time.Second, func() (bool, error) {
		_, err := kclient.Extensions().ThirdPartyResources().Get(host + endpoint)
		// resp, err := kclient.Core().RESTClient().Get(host + endpoint)
		if err != nil {
			return false, err
		}
		return true, nil
	})
}

// CreatePlatformRoles initialize the needed roles for the platform
func CreatePlatformRoles(kclient clientset.Interface) {
	for _, role := range platform.GetRoles() {
		if _, err := kclient.Rbac().ClusterRoles().Create(role); err != nil && !apierrors.IsAlreadyExists(err) {
			panic(err)
		}
		glog.Infof("provisioned role %s", role.Name)
	}
}
