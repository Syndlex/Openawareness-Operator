// Package clients provides client cache management for Loki ruler API.
package clients

import (
	"context"
	"fmt"

	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/syndlex/openawareness-controller/internal/loki"
)

// LokiClientInterface defines the operations the LokiRuleGroup controller needs.
type LokiClientInterface interface {
	CreateOrUpdateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup, tenantID string) error
	DeleteRuleGroup(ctx context.Context, namespace, groupName, tenantID string) error
	DeleteNamespace(ctx context.Context, namespace, tenantID string) error
	GetRuleGroup(ctx context.Context, namespace, groupName, tenantID string) (*rulefmt.RuleGroup, error)
	ListRules(ctx context.Context, namespace, tenantID string) (map[string][]rulefmt.RuleGroup, error)
	HealthCheck(ctx context.Context) error
}

// LokiClientCacheInterface manages a pool of named Loki clients.
type LokiClientCacheInterface interface {
	GetOrCreateLokiClient(ctx context.Context, address, name string) (LokiClientInterface, error)
	RemoveLokiClient(name string)
}

// LokiClientCache is the default implementation of LokiClientCacheInterface.
// One client per named Loki instance; tenant isolation is achieved per-request
// via the X-Scope-OrgID header.
type LokiClientCache struct {
	clients map[string]LokiClientInterface
}

// Ensure LokiClientCache satisfies the interface at compile time.
var _ LokiClientCacheInterface = (*LokiClientCache)(nil)

// NewLokiClientCache returns an empty LokiClientCache.
func NewLokiClientCache() *LokiClientCache {
	return &LokiClientCache{
		clients: make(map[string]LokiClientInterface),
	}
}

// GetOrCreateLokiClient returns a cached client for the given name, or creates and caches
// a new one pointed at address. A health-check is performed on first creation.
func (c *LokiClientCache) GetOrCreateLokiClient(
	ctx context.Context,
	address, name string,
) (LokiClientInterface, error) {
	if cl, ok := c.clients[name]; ok {
		return cl, nil
	}

	cl, err := loki.New(ctx, loki.Config{Address: address})
	if err != nil {
		return nil, fmt.Errorf("creating Loki client %q: %w", name, err)
	}

	if err := cl.HealthCheck(ctx); err != nil {
		return nil, fmt.Errorf("Loki client %q health check failed: %w", name, err)
	}

	c.clients[name] = cl
	return cl, nil
}

// RemoveLokiClient removes the client with the given name from the cache.
func (c *LokiClientCache) RemoveLokiClient(name string) {
	delete(c.clients, name)
}
