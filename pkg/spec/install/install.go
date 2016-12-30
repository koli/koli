package install

import (
	"k8s.io/kubernetes/pkg/apimachinery/announced"
	"kolihub.io/koli/pkg/spec"
)

func init() {
	if err := announced.NewGroupMetaFactory(
		&announced.GroupMetaFactoryArgs{
			GroupName:                  spec.GroupName,
			VersionPreferenceOrder:     []string{spec.SchemeGroupVersion.Version},
			ImportPrefix:               "kolihub.io/koli/pkg/spec",
			AddInternalObjectsToScheme: spec.AddToScheme,
		},
		announced.VersionToSchemeFunc{
			spec.SchemeGroupVersion.Version: spec.AddToScheme,
		},
	).Announce().RegisterAndEnable(); err != nil {
		panic(err)
	}
}
