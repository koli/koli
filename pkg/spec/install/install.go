package install

import (
	"github.com/kolibox/koli/pkg/spec"
	"k8s.io/client-go/1.5/pkg/apimachinery/announced"
)

func init() {
	if err := announced.NewGroupMetaFactory(
		&announced.GroupMetaFactoryArgs{
			GroupName:                  spec.GroupName,
			VersionPreferenceOrder:     []string{spec.SchemeGroupVersion.Version},
			ImportPrefix:               "github.com/kolibox/koli/pkg/spec",
			AddInternalObjectsToScheme: spec.AddToScheme,
		},
		announced.VersionToSchemeFunc{
			spec.SchemeGroupVersion.Version: spec.AddToScheme,
		},
	).Announce().RegisterAndEnable(); err != nil {
		panic(err)
	}
}
