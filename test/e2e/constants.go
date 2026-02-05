package e2e

import "time"

const (
	// DefaultTimeout Test timeouts and intervals
	DefaultTimeout  = time.Minute * 2
	DefaultInterval = time.Second * 1

	// MimirGatewayAddress Mimir configuration
	MimirGatewayAddress = "http://mimir-gateway.mimir.svc.cluster.local:8080"
	MimirLocalAddress   = "http://localhost:8080"

	// ClientConfigTestNamespace Test namespaces
	ClientConfigTestNamespace     = "clientconfig-e2e-test"
	MimirAlertTenantTestNamespace = "mimiralerttenant-e2e-test"
)
