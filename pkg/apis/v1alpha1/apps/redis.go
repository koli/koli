package apps

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/renstrom/dedent"
	"kolihub.io/koli/pkg/spec"
	"kolihub.io/koli/pkg/spec/util"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
	v1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	redisConfFileName = "redis.conf"
	redisConfFilePath = "/opt/" + redisConfFileName
)

// Redis add-on in memory key value store database
type Redis struct {
	client  kubernetes.Interface
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
	var cm *v1.ConfigMap
	cm = &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   r.addon.Name,
			Labels: map[string]string{"koli.io/app": r.addon.Name},
		},
		Data: map[string]string{
			redisConfFileName: redisConfig,
		},
	}

	cmClient := r.client.Core().ConfigMaps(r.addon.Namespace)
	_, err = cmClient.Get(r.addon.Name, metav1.GetOptions{})
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
		Volumes: []v1.Volume{
			{
				Name: "config",
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{
							// The ConfigMap has the same name of the PetSet
							Name: r.addon.Name,
						},
					},
				},
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "config",
				ReadOnly:  true,
				MountPath: "/opt", // TODO: change to /etc/[...]
			},
		},
	}
}

// CreatePetSet add a new redis PetSet
func (r *Redis) CreatePetSet(sp *spec.Plan) error {
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
func (r *Redis) UpdatePetSet(old *v1beta1.StatefulSet, sp *spec.Plan) error {
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
	oldPset, err := util.StatefulSetDeepCopy(obj.(*v1beta1.StatefulSet))
	if err != nil {
		return err
	}
	oldPset.Spec.Replicas = new(int32)

	if _, err := psetClient.Update(oldPset); err != nil {
		return err
	}

	podClient := r.client.Core().Pods(r.addon.Namespace)
	// TODO: temprorary solution until Deployment status provides necessary info to know
	// whether scale-down completed.
	for {
		pods, err := podClient.List(metav1.ListOptions{LabelSelector: r.GetSelector()})
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
func (r *Redis) GetSelector() string {
	return "koli.io/type=addon,koli.io/app=" + r.addon.Name
}
