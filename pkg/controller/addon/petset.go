package addon

import (
	"fmt"

	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/apps/v1alpha1"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

func makePetSet(a *spec.Addon, old *v1alpha1.PetSet) *v1alpha1.PetSet {
	// TODO: is this the right point to inject defaults?
	// Ideally we would do it before storing but that's currently not possible.
	// Potentially an update handler on first insertion.

	replicas := a.Spec.Replicas
	if replicas < 1 {
		replicas = 1
	}
	image := fmt.Sprintf("%s:%s", a.Spec.BaseImage, a.Spec.Version)
	_ = image

	var args []string
	if a.Spec.BaseImage == "redis" {
		args = []string{fmt.Sprintf("/opt/%s.conf", a.Name)} // TODO: change to /etc/[...]
	}

	petset := &v1alpha1.PetSet{
		ObjectMeta: v1.ObjectMeta{
			Name: a.Name,
		},
		Spec: makePetSetSpec(a.Name, a.Spec.BaseImage, a.Spec.Port, replicas, args),
	}
	if old != nil {
		petset.Annotations = old.Annotations
	}
	return petset
}

func makeEmptyConfig(name string) *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: v1.ObjectMeta{Name: name},
		Data:       map[string]string{fmt.Sprintf("%s.conf", name): ""},
	}
}

func makePetSetService(a *spec.Addon) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: a.Name,
		},
		Spec: v1.ServiceSpec{
			ClusterIP: "None",
			Ports: []v1.ServicePort{
				{
					Name:       "web",
					Port:       9090,
					TargetPort: intstr.FromString("web"),
				},
			},
			Selector: map[string]string{
				"sys.io/type": "addon",
				"sys.io/app":  a.Name,
			},
		},
	}
	return svc
}

func makePetSetSpec(name, image string, port, replicas int32, args []string) v1alpha1.PetSetSpec {
	terminationGracePeriod := int64(30)
	return v1alpha1.PetSetSpec{
		ServiceName: name,
		Replicas:    &replicas,
		Template: v1.PodTemplateSpec{
			ObjectMeta: v1.ObjectMeta{
				Labels: map[string]string{
					"sys.io/type": "addon",
					"sys.io/app":  name,
				},
				Annotations: map[string]string{
					"pod.alpha.kubernetes.io/initialized": "true",
				},
			},
			Spec: v1.PodSpec{
				Volumes: []v1.Volume{
					{
						Name: "config",
						VolumeSource: v1.VolumeSource{
							ConfigMap: &v1.ConfigMapVolumeSource{
								LocalObjectReference: v1.LocalObjectReference{
									Name: name,
								},
							},
						},
					},
				},
				Containers: []v1.Container{
					{
						Name:  name,
						Image: image,
						Args:  args,
						Ports: []v1.ContainerPort{
							{
								Name:          "addon",
								ContainerPort: port,
								Protocol:      v1.ProtocolTCP,
							},
						},
						VolumeMounts: []v1.VolumeMount{
							{
								Name:      "config",
								ReadOnly:  true,
								MountPath: "/opt", // TODO: change to /etc/[...]
							},
						},
					},
				},
				TerminationGracePeriodSeconds: &terminationGracePeriod,
			},
		},
	}
}
