package clients

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/dskit/crypto/tls"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/syndlex/openawareness-controller/internal/mimir"
)

// RulerClientCacheInterface defines the interface for managing ruler clients
type RulerClientCacheInterface interface {
	AddMimirClient(address string, name string, tenantID string, ctx context.Context) error
	AddPromClient(address string, name string, ctx context.Context) error
	RemoveClient(name string)
	GetClient(name string) (AwarenessClient, error)
	GetOrCreateMimirClient(address string, clientName string, tenantID string, ctx context.Context) (AwarenessClient, error)
}

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

type RulerClientCache struct {
	clients map[string]AwarenessClient
}

// Ensure RulerClientCache implements RulerClientCacheInterface
var _ RulerClientCacheInterface = (*RulerClientCache)(nil)

func NewRulerClientCache() *RulerClientCache {
	return &RulerClientCache{
		clients: map[string]AwarenessClient{},
	}
}

func (e *RulerClientCache) AddMimirClient(address string, name string, tenantID string, ctx context.Context) error {
	client, err := mimir.New(mimir.Config{
		User:            "",
		Key:             "",
		Address:         address,
		TenantId:        tenantID,
		TLS:             tls.ClientConfig{},
		UseLegacyRoutes: false,
		MimirHTTPPrefix: "",
		AuthToken:       "",
		ExtraHeaders:    nil,
	}, ctx)
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
func (e *RulerClientCache) GetOrCreateMimirClient(address string, clientName string, tenantID string, ctx context.Context) (AwarenessClient, error) {
	// Create composite key: clientName + tenantID
	cacheKey := fmt.Sprintf("%s-%s", clientName, tenantID)

	// Check if client already exists
	if client, exists := e.clients[cacheKey]; exists {
		return client, nil
	}

	// Create new client with tenant ID
	if err := e.AddMimirClient(address, cacheKey, tenantID, ctx); err != nil {
		return nil, fmt.Errorf("creating Mimir client for tenant %s: %w", tenantID, err)
	}

	return e.clients[cacheKey], nil
}

func (e *RulerClientCache) RemoveClient(name string) {
	if e.clients[name] == nil {
		return
	}
	delete(e.clients, name)
}

func (e *RulerClientCache) GetClient(name string) (AwarenessClient, error) {
	if client, exists := e.clients[name]; exists {
		return client, nil
	}
	return nil, errors.New("client not found")
}

func (e *RulerClientCache) AddPromClient(address string, name string, ctx context.Context) error {
	return errors.New("Prometheus client not yet implemented")
}
