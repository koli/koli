package apps

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/renstrom/dedent"
	"kolihub.io/koli/pkg/spec"
	"kolihub.io/koli/pkg/spec/util"

	"k8s.io/kubernetes/pkg/api"
	apierrors "k8s.io/kubernetes/pkg/api/errors"
	apps "k8s.io/kubernetes/pkg/apis/apps"
	"k8s.io/kubernetes/pkg/client/cache"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/labels"
)

const (
	redisConfFileName = "redis.conf"
	redisConfFilePath = "/opt/" + redisConfFileName
)

// Redis add-on in memory key value store database
type Redis struct {
	client  clientset.Interface
	addon   *spec.Addon
	psetInf cache.SharedIndexInformer
}

// CreateConfigMap generates a ConfigMap with a redis default configuration
func (r *Redis) CreateConfigMap() error {
	// Update config map based on the most recent configuration.
	redisConfig, err := r.getConfigTemplate()
	if err != nil {
		return err
	}
	var cm *api.ConfigMap
	cm = &api.ConfigMap{
		ObjectMeta: api.ObjectMeta{
			Name:   r.addon.Name,
			Labels: map[string]string{"koli.io/app": r.addon.Name},
		},
		Data: map[string]string{
			redisConfFileName: redisConfig,
		},
	}

	cmClient := r.client.Core().ConfigMaps(r.addon.Namespace)
	_, err = cmClient.Get(r.addon.Name)
	if apierrors.IsNotFound(err) {
		_, err = cmClient.Create(cm)
	} else if err == nil {
		_, err = cmClient.Update(cm)
	}
	return err
}

func (r *Redis) getConfigTemplate() (string, error) {
	redisCfg := dedent.Dedent(`# https://raw.githubusercontent.com/antirez/redis/3.2/redis.conf
	# put your config parameters below, mind the indentation!
	databases 1`)
	var buf bytes.Buffer
	if err := template.Must(template.New("config").Parse(redisCfg)).Execute(&buf, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (r *Redis) makeVolumes() *VolumeSpec {
	return &VolumeSpec{
		Volumes: []api.Volume{
			{
				Name: "config",
				VolumeSource: api.VolumeSource{
					ConfigMap: &api.ConfigMapVolumeSource{
						LocalObjectReference: api.LocalObjectReference{
							// The ConfigMap has the same name of the PetSet
							Name: r.addon.Name,
						},
					},
				},
			},
		},
		VolumeMounts: []api.VolumeMount{
			{
				Name:      "config",
				ReadOnly:  true,
				MountPath: "/opt", // TODO: change to /etc/[...]
			},
		},
	}
}

// CreatePetSet add a new redis PetSet
func (r *Redis) CreatePetSet(sp *spec.ServicePlan) error {
	labels := map[string]string{
		"koli.io/type": "addon",
		"koli.io/app":  r.addon.Name,
	}
	petset := makePetSet(r.addon, nil, labels, []string{redisConfFilePath}, r.makeVolumes())
	petset.Spec.Template.Spec.Containers[0].Resources.Limits = sp.Spec.Resources.Limits
	petset.Spec.Template.Spec.Containers[0].Resources.Requests = sp.Spec.Resources.Requests
	petset.Labels = spec.NewLabel().Add(map[string]string{"clusterplan": sp.Name}).Set
	if _, err := r.client.Apps().StatefulSets(r.addon.Namespace).Create(petset); err != nil {
		return fmt.Errorf("failed creating petset (%s)", err)
	}
	return nil
}

// UpdatePetSet update a redis PetSet
func (r *Redis) UpdatePetSet(old *apps.StatefulSet, sp *spec.ServicePlan) error {
	labels := map[string]string{
		"koli.io/type": "addon",
		"koli.io/app":  r.addon.Name,
	}
	petset := makePetSet(r.addon, old, labels, []string{redisConfFilePath}, r.makeVolumes())
	petset.Spec.Template.Spec.Containers[0].Resources.Limits = sp.Spec.Resources.Limits
	petset.Spec.Template.Spec.Containers[0].Resources.Requests = sp.Spec.Resources.Requests
	petset.SetLabels(spec.NewLabel().Add(map[string]string{"clusterplan": sp.Name}).Set)
	if _, err := r.client.Apps().StatefulSets(r.addon.Namespace).Update(petset); err != nil {
		return fmt.Errorf("failed creating petset (%s)", err)
	}
	return nil
}

// DeleteApp exclude a redis PetSet
func (r *Redis) DeleteApp() error {
	// Update the replica count to 0 and wait for all pods to be deleted.
	psetClient := r.client.Apps().StatefulSets(r.addon.Namespace)
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(r.addon)
	if err != nil {
		return err
	}
	obj, _, err := r.psetInf.GetStore().GetByKey(key)
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

	selector, err := r.GetSelector()
	if err != nil {
		return err
	}
	podClient := r.client.Core().Pods(r.addon.Namespace)

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
	r.client.Core().ConfigMaps(r.addon.Namespace).Delete(r.addon.Name, nil)
	r.client.Core().Services(r.addon.Namespace).Delete(r.addon.Name, nil)
	// Deployment scaled down, we can delete it.
	return psetClient.Delete(r.addon.Name, nil)
}

// GetAddon returns the addon object
func (r *Redis) GetAddon() *spec.Addon {
	return r.addon
}

// GetSelector retrieves the a selector for the redis app based on its name
func (r *Redis) GetSelector() (labels.Selector, error) {
	return labels.Parse("koli.io/type=addon,koli.io/app=" + r.addon.Name)
}
