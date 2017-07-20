package v1alpha1

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/pkg/api/v1"
)

func TestPlanMeta(t *testing.T) {
	storageSize := resource.MustParse("5Gi")
	resourceCPU, resourceMem := resource.MustParse("100"), resource.MustParse("1Gi")
	computeResources := v1.ResourceRequirements{
		Limits: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resourceCPU,
			v1.ResourceMemory: resourceMem,
		},
		Requests: map[v1.ResourceName]resource.Quantity{
			v1.ResourceCPU:    resourceCPU,
			v1.ResourceMemory: resourceMem,
		},
	}
	p := &Plan{
		Spec: PlanSpec{
			Type:      PlanTypeStorage,
			Resources: computeResources,
			Storage:   storageSize,
		},
	}
	cpuLimit, cpuRequest := p.CPU()
	if *cpuLimit != resourceCPU || *cpuRequest != resourceCPU {
		t.Errorf("GOT LIMIT: %#v, GOT REQUEST: %#v, EXPECTED: %#v", cpuLimit, cpuRequest, resourceCPU)
	}
	memLimit, memRequest := p.Memory()
	if *memLimit != resourceMem || *memRequest != resourceMem {
		t.Errorf("GOT LIMIT: %#v, GOT REQUEST: %#v, EXPECTED: %#v", memLimit, memRequest, resourceMem)
	}

	if storageSize != *p.Storage() {
		t.Errorf("GOT: %#v, EXPECTED: %#v", storageSize, p.Storage())
	}
}
