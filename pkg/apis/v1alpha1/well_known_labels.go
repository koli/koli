package v1alpha1

const (
	// LabelAllowKongTraffic is a key which accepts a boolean string to enable/disable network traffic
	// flow between the kong namespace and all namespaces that has this annotation.
	LabelAllowKongTraffic = "kolihub.io/allow-kong-traffic"
	// LabelCustomer it's a string representing the name of the customer in the platform
	LabelCustomer = "kolihub.io/customer"
	// LabelOrganization it's a string representing the name of the organization in the platform
	LabelOrganization = "kolihub.io/org"
	// LabelClusterPlan refers to the specified plan of the resource (statefulset, deployment)
	LabelClusterPlan = "kolihub.io/clusterplan"
	// LabelStoragePlan refers to the specified storage plan of the resource (statefulset, deployment)
	LabelStoragePlan = "kolihub.io/storage-plan"
	// LabelDefault indicates a resource as default (boolean)
	LabelDefault = "kolihub.io/default"

	// LabelSecretController identifies resources created by the secret controller
	LabelSecretController = "secret.kolihub.io"
)
