package spec

import (
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

// Addon defines integration with external resources
type Addon struct {
	unversioned.TypeMeta `json:",inline"`
	v1.ObjectMeta        `json:"metadata,omitempty"`
	Spec                 AddonSpec `json:"spec"`
}

// AddonList is a list of Addons.
type AddonList struct {
	unversioned.TypeMeta `json:",inline"`
	unversioned.ListMeta `json:"metadata,omitempty"`

	Items []*Addon `json:"items"`
}

// AddonSpec holds specification parameters of an addon
type AddonSpec struct {
	Type      string `json:"type"`
	BaseImage string `json:"baseImage"`
	Version   string `json:"version"`
	Replicas  int32  `json:"replicas"`
	Port      int32  `json:"port"`
}
