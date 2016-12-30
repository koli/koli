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
)
