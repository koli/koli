package install

// import (
// 	"k8s.io/apimachinery/pkg/apimachinery/announced"
// 	"k8s.io/apimachinery/pkg/apimachinery/registered"
// 	"k8s.io/apimachinery/pkg/runtime"
// 	"k8s.io/client-go/pkg/api"
// 	platform "kolihub.io/koli/pkg/apis/core/v1alpha1"
// )

// func init() {
// 	Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
// }

// // Install registers the API group and adds types to a scheme
// func Install(groupFactoryRegistry announced.APIGroupFactoryRegistry, registry *registered.APIRegistrationManager, scheme *runtime.Scheme) {
// 	if err := announced.NewGroupMetaFactory(
// 		&announced.GroupMetaFactoryArgs{
// 			GroupName:              platform.GroupName,
// 			VersionPreferenceOrder: []string{platform.SchemeGroupVersion.Version},
// 			ImportPrefix:           "kolihub.io/koli/pkg/apis/core/v1alpha1",
// 			// RootScopedKinds:            sets.NewString("PodSecurityPolicy", "ThirdPartyResource"),
// 			AddInternalObjectsToScheme: platform.AddToScheme,
// 		},
// 		announced.VersionToSchemeFunc{
// 			platform.SchemeGroupVersion.Version: platform.AddToScheme,
// 		},
// 	).Announce(groupFactoryRegistry).RegisterAndEnable(registry, scheme); err != nil {
// 		panic(err)
// 	}
// }
