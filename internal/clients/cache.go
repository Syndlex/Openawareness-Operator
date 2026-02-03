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
	AddMimirClient(ctx context.Context, address string, name string, tenantID string) error
	AddPromClient(ctx context.Context, address string, name string) error
	RemoveClient(name string)
	GetClient(name string) (AwarenessClient, error)
	GetOrCreateMimirClient(
		ctx context.Context,
		address string,
		clientName string,
		tenantID string,
	) (AwarenessClient, error)
}

// AwarenessClient defines the interface for interacting with rule and alert APIs.
// It abstracts the operations for both Mimir and Prometheus clients.
type AwarenessClient interface {
	CreateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup) error
	DeleteRuleGroup(ctx context.Context, namespace, groupName string) error
	GetRuleGroup(ctx context.Context, namespace, groupName string) (*rulefmt.RuleGroup, error)
	ListRules(ctx context.Context, namespace string) (map[string][]rulefmt.RuleGroup, error)
	DeleteNamespace(ctx context.Context, namespace string) error
	CreateAlertmanagerConfig(ctx context.Context, cfg string, templates map[string]string) error
	DeleteAlermanagerConfig(ctx context.Context) error
	GetAlertmanagerConfig(ctx context.Context) (string, map[string]string, error)
	GetAlertmanagerStatus(ctx context.Context) (string, error)
}

// RulerClientCache implements RulerClientCacheInterface and manages a cache of ruler clients.
// It stores clients in a map keyed by client name (or client-tenant combination for multi-tenancy).
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
// Returns an error if client creation or health check fails.
func (e *RulerClientCache) AddMimirClient(ctx context.Context, address string, name string, tenantID string) error {
	client, err := mimir.New(ctx, mimir.Config{
		User:            "",
		Key:             "",
		Address:         address,
		TenantID:        tenantID,
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

// GetOrCreateMimirClient gets an existing client or creates a new one for the given tenant.
// The cache key is a combination of clientName and tenantID to support multi-tenancy.
// This ensures each tenant has its own isolated client instance.
// Returns the cached or newly created client, or an error if creation fails.
func (e *RulerClientCache) GetOrCreateMimirClient(
	ctx context.Context,
	address string,
	clientName string,
	tenantID string,
) (AwarenessClient, error) {
	// Create composite key: clientName + tenantID
	cacheKey := fmt.Sprintf("%s-%s", clientName, tenantID)

	// Check if client already exists
	if client, exists := e.clients[cacheKey]; exists {
		return client, nil
	}

	// Create new client with tenant ID
	if err := e.AddMimirClient(ctx, address, cacheKey, tenantID); err != nil {
		return nil, fmt.Errorf("creating Mimir client for tenant %s: %w", tenantID, err)
	}

	return e.clients[cacheKey], nil
}

// RemoveClient removes a client from the cache by name.
// This is typically called when a ClientConfig is deleted.
func (e *RulerClientCache) RemoveClient(name string) {
	if e.clients[name] == nil {
		return
	}
	delete(e.clients, name)
}

// GetClient retrieves a client from the cache by name.
// Returns an error if the client is not found in the cache.
func (e *RulerClientCache) GetClient(name string) (AwarenessClient, error) {
	if client, exists := e.clients[name]; exists {
		return client, nil
	}
	return nil, errors.New("client not found")
}

// AddPromClient would create a Prometheus client and add it to the cache.
// Currently not implemented - returns an error indicating this.
func (e *RulerClientCache) AddPromClient(_ context.Context, _ string, _ string) error {
	return errors.New("prometheus client not yet implemented")
}
