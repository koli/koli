package spec

import (
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

// VolumeSpec facilitate passing definitions
// of volumes (v1.Volumes) and how to mount then (v1.VolumeMount)
type VolumeSpec struct {
	Volumes      []v1.Volume
	VolumeMounts []v1.VolumeMount
}

func makePetSet(addon *Addon, old *v1alpha1.PetSet, labels map[string]string, args []string, vol *VolumeSpec) *v1alpha1.PetSet {
	petset := &v1alpha1.PetSet{
		ObjectMeta: v1.ObjectMeta{
			Name: addon.Name,
		},
		Spec: getPetSetSpec(addon, labels, append(addon.Spec.Args, args...), vol),
	}
	if old != nil {
		petset.Annotations = old.Annotations
	}
	return petset
}

// MakePetSetService generates a &v1.Service
func MakePetSetService(addon *Addon) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: addon.Name,
			Labels: map[string]string{
				"sys.io/app": addon.Name,
			},
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "None", // headless service
			Ports: []v1.ServicePort{
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
func getPetSetSpec(addon *Addon, labels map[string]string, args []string, vol *VolumeSpec) v1alpha1.PetSetSpec {
	terminationGracePeriod := int64(30) // TODO: should be base on the app type
	return v1alpha1.PetSetSpec{
		ServiceName: addon.Name,
		Replicas:    addon.GetReplicas(),
		Template: v1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: labels,
				Annotations: map[string]string{
					"pod.alpha.kubernetes.io/initialized": "true",
				},
			},
			Spec: v1.PodSpec{
				Volumes: vol.Volumes,
				Containers: []v1.Container{
					{
						Name:  addon.Name,
						Image: addon.GetImage(),
						Args:  args,
						Env:   addon.Spec.Env,
						Ports: []v1.ContainerPort{
							{
								Name:          "addon",
								ContainerPort: addon.Spec.Port, // TODO: should be based on the app type
								Protocol:      v1.ProtocolTCP,
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
