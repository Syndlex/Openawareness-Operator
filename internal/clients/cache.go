// Package clients provides client cache management for Mimir and Prometheus ruler APIs
package clients

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/dskit/crypto/tls"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/syndlex/openawareness-controller/internal/mimir"
)

// RulerClientCacheInterface defines the interface for managing ruler clients.
// It provides methods to add, remove, and retrieve clients for both Mimir and Prometheus.
type RulerClientCacheInterface interface {
	AddMimirClient(ctx context.Context, address string, name string) error
	AddPromClient(ctx context.Context, address string, name string) error
	RemoveClient(name string)
	GetOrCreateMimirClient(
		ctx context.Context,
		address string,
		clientName string,
	) (AwarenessClient, error)
}

// AwarenessClient defines the interface for interacting with rule and alert APIs.
// It abstracts the operations for both Mimir and Prometheus clients.
// All methods accept a tenantID parameter for multi-tenant isolation.
type AwarenessClient interface {
	CreateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup, tenantID string) error
	DeleteRuleGroup(ctx context.Context, namespace, groupName string, tenantID string) error
	GetRuleGroup(ctx context.Context, namespace, groupName string, tenantID string) (*rulefmt.RuleGroup, error)
	ListRules(ctx context.Context, namespace string, tenantID string) (map[string][]rulefmt.RuleGroup, error)
	DeleteNamespace(ctx context.Context, namespace string, tenantID string) error
	CreateAlertmanagerConfig(ctx context.Context, cfg string, templates map[string]string, tenantID string) error
	DeleteAlermanagerConfig(ctx context.Context, tenantID string) error
	GetAlertmanagerConfig(ctx context.Context, tenantID string) (string, map[string]string, error)
	GetAlertmanagerStatus(ctx context.Context, tenantID string) (string, error)
}

// RulerClientCache implements RulerClientCacheInterface and manages a cache of ruler clients.
// It stores clients in a map keyed by client name - one client per Mimir instance handles all tenants.
type RulerClientCache struct {
	clients map[string]AwarenessClient
}

// Ensure RulerClientCache implements RulerClientCacheInterface
var _ RulerClientCacheInterface = (*RulerClientCache)(nil)

// NewRulerClientCache creates and returns a new RulerClientCache instance.
func NewRulerClientCache() *RulerClientCache {
	return &RulerClientCache{
		clients: map[string]AwarenessClient{},
	}
}

// AddMimirClient creates a new Mimir client and adds it to the cache.
// It performs a health check to verify connectivity before caching the client.
// The client is created without a tenant ID - tenant isolation is achieved
// via the X-Scope-OrgID header on each request (passed via tenantID parameter).
// Returns an error if client creation or health check fails.
func (e *RulerClientCache) AddMimirClient(ctx context.Context, address string, name string) error {
	// Create client without tenant ID - tenant will be passed per-request via tenantID parameter
	client, err := mimir.New(ctx, mimir.Config{
		User:            "",
		Key:             "",
		Address:         address,
		TLS:             tls.ClientConfig{},
		UseLegacyRoutes: false,
		MimirHTTPPrefix: "",
		AuthToken:       "",
		ExtraHeaders:    nil,
	})
	if err != nil {
		return fmt.Errorf("creating Mimir client: %w", err)
	}

	// Perform health check to verify connectivity
	if err := client.HealthCheck(ctx); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	e.clients[name] = client
	return nil
}

// GetOrCreateMimirClient gets an existing client or creates a new one.
// The cache key is simply the clientName - one client handles all tenants for that Mimir instance.
// Tenant isolation is achieved via the X-Scope-OrgID header on each request (namespace parameter).
// Returns the cached or newly created client, or an error if creation fails.
func (e *RulerClientCache) GetOrCreateMimirClient(
	ctx context.Context,
	address string,
	clientName string,
) (AwarenessClient, error) {
	// Check if client already exists using simple client name
	if client, exists := e.clients[clientName]; exists {
		return client, nil
	}

	// Create new client without tenant ID - tenant passed per-request
	if err := e.AddMimirClient(ctx, address, clientName); err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	return e.clients[clientName], nil
}

// RemoveClient removes a client from the cache by name.
// This is typically called when a ClientConfig is deleted.
func (e *RulerClientCache) RemoveClient(name string) {
	if e.clients[name] == nil {
		return
	}
	delete(e.clients, name)
}

// AddPromClient would create a Prometheus client and add it to the cache.
// Currently not implemented - returns an error indicating this.
func (e *RulerClientCache) AddPromClient(_ context.Context, _ string, _ string) error {
	return errors.New("prometheus client not yet implemented")
}
