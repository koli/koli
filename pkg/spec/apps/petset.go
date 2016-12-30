package apps

import (
	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/kubernetes/pkg/api"
	apps "k8s.io/kubernetes/pkg/apis/apps"
	"k8s.io/kubernetes/pkg/util/intstr"
)

// VolumeSpec facilitate passing definitions
// of volumes (api.Volumes) and how to mount then (api.VolumeMount)
type VolumeSpec struct {
	Volumes      []api.Volume
	VolumeMounts []api.VolumeMount
}

func makePetSet(addon *spec.Addon, old *apps.StatefulSet, labels map[string]string, args []string, vol *VolumeSpec) *apps.StatefulSet {
	petset := &apps.StatefulSet{
		ObjectMeta: api.ObjectMeta{
			Name: addon.Name,
		},
		Spec: getPetSetSpec(addon, labels, append(addon.Spec.Args, args...), vol),
	}
	if old != nil {
		petset.Annotations = old.Annotations
	}
	return petset
}

// MakePetSetService generates a &api.Service
func MakePetSetService(addon *spec.Addon) *api.Service {
	svc := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name: addon.Name,
			Labels: map[string]string{
				"sys.io/app": addon.Name,
			},
		},
		Spec: api.ServiceSpec{
			ClusterIP: "None", // headless service
			Ports: []api.ServicePort{
				{
					Name:       addon.Spec.Type,
					Port:       addon.Spec.Port,
					TargetPort: intstr.FromString("addon"),
				},
			},
			Selector: map[string]string{
				"sys.io/app": addon.Name,
			},
		},
	}
	return svc
}

// getPetSetSpec returns a generic PetSetSpec
func getPetSetSpec(addon *spec.Addon, labels map[string]string, args []string, vol *VolumeSpec) apps.StatefulSetSpec {
	terminationGracePeriod := int64(30) // TODO: should be base on the app type
	return apps.StatefulSetSpec{
		ServiceName: addon.Name,
		Replicas:    addon.GetReplicas(),
		Template: api.PodTemplateSpec{
			ObjectMeta: api.ObjectMeta{
				Labels: labels,
				Annotations: map[string]string{
					"pod.alpha.kubernetes.io/initialized": "true",
				},
			},
			Spec: api.PodSpec{
				Volumes: vol.Volumes,
				Containers: []api.Container{
					{
						Name:  addon.Name,
						Image: addon.GetImage(),
						Args:  args,
						Env:   addon.Spec.Env,
						Ports: []api.ContainerPort{
							{
								Name:          "addon",
								ContainerPort: addon.Spec.Port, // TODO: should be based on the app type
								Protocol:      api.ProtocolTCP,
							},
						},
						VolumeMounts: vol.VolumeMounts,
					},
				},
				TerminationGracePeriodSeconds: &terminationGracePeriod,
			},
		},
	}
}
