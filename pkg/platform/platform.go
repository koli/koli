package platform

const (
	// SystemNamespace is the name of the namespace where resides
	// all the applications needed by the platform
	SystemNamespace = "koli-system"

	// BrokerSystemNamespace is the portion name of a system namespace of the broker.
	// E.g.: system-[customer]-[org]
	BrokerSystemNamespace = "system"

	// BrokerSystemCustomer is the portion name of a system namespace of the broker.
	// E.g.: [namespace]-org-[org]
	BrokerSystemCustomer = "org"
	// GitRepositoryPathPrefix is used to construct the URL path of the repositories
	GitRepositoryPathPrefix = "repos"
	// GitReleasesPathPrefix is used to construct the URL path of the releases
	GitReleasesPathPrefix = "releases"
)
