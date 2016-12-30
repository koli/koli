package apps

import (
	"fmt"
	"time"

	"github.com/kolibox/koli/pkg/spec"
	"github.com/kolibox/koli/pkg/spec/util"

	"k8s.io/kubernetes/pkg/api"
	apps "k8s.io/kubernetes/pkg/apis/apps"
	"k8s.io/kubernetes/pkg/client/cache"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/labels"
)

// Memcached add-on in memory key value store database
type Memcached struct {
	client  clientset.Interface
	addon   *spec.Addon
	psetInf cache.SharedIndexInformer
}

// CreateConfigMap does nothing
func (m *Memcached) CreateConfigMap() error {
	return nil
}

// CreatePetSet add a new memcached PetSet
func (m *Memcached) CreatePetSet(sp *spec.ServicePlan) error {
	labels := map[string]string{
		"koli.io/type": "addon",
		"koli.io/app":  m.addon.Name,
	}
	petset := makePetSet(m.addon, nil, labels, nil, &VolumeSpec{})
	petset.Spec.Template.Spec.Containers[0].Resources.Limits = sp.Spec.Resources.Limits
	petset.Spec.Template.Spec.Containers[0].Resources.Requests = sp.Spec.Resources.Requests
	petset.Labels = spec.NewLabel().Add(map[string]string{"clusterplan": sp.Name}).Set
	if _, err := m.client.Apps().StatefulSets(m.addon.Namespace).Create(petset); err != nil {
		return fmt.Errorf("failed creating petset (%s)", err)
	}
	return nil
}

// UpdatePetSet update a memcached PetSet
func (m *Memcached) UpdatePetSet(old *apps.StatefulSet, sp *spec.ServicePlan) error {
	labels := map[string]string{
		"koli.io/type": "addon",
		"koli.io/app":  m.addon.Name,
	}
	petset := makePetSet(m.addon, old, labels, nil, &VolumeSpec{})
	petset.Spec.Template.Spec.Containers[0].Resources.Limits = sp.Spec.Resources.Limits
	petset.Spec.Template.Spec.Containers[0].Resources.Requests = sp.Spec.Resources.Requests
	petset.SetLabels(spec.NewLabel().Add(map[string]string{"clusterplan": sp.Name}).Set)
	if _, err := m.client.Apps().StatefulSets(m.addon.Namespace).Update(petset); err != nil {
		return fmt.Errorf("failed creating petset (%s)", err)
	}
	return nil
}

// DeleteApp exclude a memcached PetSet
func (m *Memcached) DeleteApp() error {
	// Update the replica count to 0 and wait for all pods to be deleted.
	psetClient := m.client.Apps().StatefulSets(m.addon.Namespace)
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(m.addon)
	if err != nil {
		return err
	}
	obj, _, err := m.psetInf.GetStore().GetByKey(key)
	if err != nil {
		return err
	}
	// Deep-copy otherwise we are mutating our cache.
	oldPset, err := util.StatefulSetDeepCopy(obj.(*apps.StatefulSet))
	if err != nil {
		return err
	}
	oldPset.Spec.Replicas = 0

	if _, err := psetClient.Update(oldPset); err != nil {
		return err
	}

	selector, err := m.getSelector()
	if err != nil {
		return err
	}
	podClient := m.client.Core().Pods(m.addon.Namespace)

	// TODO: temprorary solution until Deployment status provides necessary info to know
	// whether scale-down completed.
	for {
		pods, err := podClient.List(api.ListOptions{LabelSelector: selector})
		if err != nil {
			return err
		}

		if len(pods.Items) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// TODO: must be garbaged collected
	m.client.Core().ConfigMaps(m.addon.Namespace).Delete(m.addon.Name, nil)
	m.client.Core().Services(m.addon.Namespace).Delete(m.addon.Name, nil)
	// Deployment scaled down, we can delete it.
	return psetClient.Delete(m.addon.Name, nil)
}

// GetAddon returns the addon object
func (m *Memcached) GetAddon() *spec.Addon {
	return m.addon
}

// getSelector retrieves the a selector for the memcached app based on its name
func (m *Memcached) getSelector() (labels.Selector, error) {
	return labels.Parse("koli.io/type=addon,koli.io/app=" + m.addon.Name)
}
