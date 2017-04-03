package apps

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"kolihub.io/koli/pkg/spec"
	"kolihub.io/koli/pkg/spec/util"

	"github.com/renstrom/dedent"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	v1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
	"k8s.io/client-go/tools/cache"
)

const (
	mysqlConfFileName = "default.cnf"
	mysqlConfFilePath = "/etc/mysql/conf.d/" + mysqlConfFileName
)

// MySQL add-on relational database management system
type MySQL struct {
	client  kubernetes.Interface
	addon   *spec.Addon
	psetInf cache.SharedIndexInformer
}

// CreateConfigMap generates a ConfigMap with a mySQL default configuration
func (m *MySQL) CreateConfigMap() error {
	// Update config map based on the most recent configuration.
	mysqlConfig, err := m.getConfigTemplate()
	if err != nil {
		return err
	}
	var cm *v1.ConfigMap
	cm = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   m.addon.Name,
			Labels: map[string]string{"koli.io/app": m.addon.Name},
		},
		Data: map[string]string{
			mysqlConfFileName: mysqlConfig,
		},
	}

	cmClient := m.client.Core().ConfigMaps(m.addon.Namespace)
	_, err = cmClient.Get(m.addon.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = cmClient.Create(cm)
	} else if err == nil {
		_, err = cmClient.Update(cm)
	}
	return err
}

func (m *MySQL) getConfigTemplate() (string, error) {
	mysqlCfg := dedent.Dedent(`# https://koli.io/docs/addons
	[mysqld]
	max_connections = 128`)
	var buf bytes.Buffer
	if err := template.Must(template.New("config").Parse(mysqlCfg)).Execute(&buf, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (m *MySQL) makeVolumes() *VolumeSpec {
	return &VolumeSpec{
		Volumes: []v1.Volume{
			{
				Name: "config",
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{
							// The ConfigMap has the same name of the PetSet
							Name: m.addon.Name,
						},
					},
				},
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "config",
				ReadOnly:  true,
				MountPath: "/etc/mysql/conf.d",
			},
		},
	}
}

// CreatePetSet add a new mySQL PetSet
func (m *MySQL) CreatePetSet(sp *spec.Plan) error {
	labels := map[string]string{
		"koli.io/type": "addon",
		"koli.io/app":  m.addon.Name,
	}
	petset := makePetSet(m.addon, nil, labels, nil, m.makeVolumes())
	petset.Spec.Template.Spec.Containers[0].Resources.Limits = sp.Spec.Resources.Limits
	petset.Spec.Template.Spec.Containers[0].Resources.Requests = sp.Spec.Resources.Requests
	petset.Labels = spec.NewLabel().Add(map[string]string{"clusterplan": sp.Name}).Set
	if _, err := m.client.Apps().StatefulSets(m.addon.Namespace).Create(petset); err != nil {
		return fmt.Errorf("failed creating petset (%s)", err)
	}
	return nil
}

// UpdatePetSet update a mySQL PetSet
func (m *MySQL) UpdatePetSet(old *v1beta1.StatefulSet, sp *spec.Plan) error {
	labels := map[string]string{
		"koli.io/type": "addon",
		"koli.io/app":  m.addon.Name,
	}
	petset := makePetSet(m.addon, old, labels, nil, m.makeVolumes())
	petset.Spec.Template.Spec.Containers[0].Resources.Limits = sp.Spec.Resources.Limits
	petset.Spec.Template.Spec.Containers[0].Resources.Requests = sp.Spec.Resources.Requests
	petset.SetLabels(spec.NewLabel().Add(map[string]string{"clusterplan": sp.Name}).Set)

	if _, err := m.client.Apps().StatefulSets(m.addon.Namespace).Update(petset); err != nil {
		return fmt.Errorf("failed creating petset (%s)", err)
	}
	return nil
}

// DeleteApp exclude a mySQL PetSet
func (m *MySQL) DeleteApp() error {
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
	oldPset, err := util.StatefulSetDeepCopy(obj.(*v1beta1.StatefulSet))
	if err != nil {
		return err
	}
	oldPset.Spec.Replicas = new(int32)

	if _, err := psetClient.Update(oldPset); err != nil {
		return err
	}

	podClient := m.client.Core().Pods(m.addon.Namespace)
	// TODO: temprorary solution until Deployment status provides necessary info to know
	// whether scale-down completed.
	for {
		pods, err := podClient.List(metav1.ListOptions{LabelSelector: m.getSelector()})
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
func (m *MySQL) GetAddon() *spec.Addon {
	return m.addon
}

// getSelector retrieves the a selector for the redis app based on its name
func (m *MySQL) getSelector() string {
	return "koli.io/type=addon,koli.io/app=" + m.addon.Name
}
