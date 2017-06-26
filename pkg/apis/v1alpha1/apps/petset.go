package apps

import (
	"kolihub.io/koli/pkg/spec"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api/v1"
	v1beta1 "k8s.io/client-go/pkg/apis/apps/v1beta1"
)

// VolumeSpec facilitate passing definitions
// of volumes (api.Volumes) and how to mount then (api.VolumeMount)
type VolumeSpec struct {
	Volumes      []v1.Volume
	VolumeMounts []v1.VolumeMount
}

func makePetSet(addon *spec.Addon, old *v1beta1.StatefulSet, labels map[string]string, args []string, vol *VolumeSpec) *v1beta1.StatefulSet {
	petset := &v1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
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
func MakePetSetService(addon *spec.Addon) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: addon.Name,
			Labels: map[string]string{
				"koli.io/app": addon.Name,
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
				"koli.io/app": addon.Name,
			},
		},
	}
	return svc
}

// getPetSetSpec returns a generic PetSetSpec
func getPetSetSpec(addon *spec.Addon, labels map[string]string, args []string, vol *VolumeSpec) v1beta1.StatefulSetSpec {
	terminationGracePeriod := int64(30) // TODO: should be base on the app type
	return v1beta1.StatefulSetSpec{
		ServiceName: addon.Name,
		Replicas:    addon.GetReplicas(),
		Template: v1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
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
