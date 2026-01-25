package clients

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/prometheus/prometheus/model/rulefmt"
)

// MockRulerClientCache is a mock implementation of RulerClientCache for testing
type MockRulerClientCache struct {
	clients map[string]AwarenessClient
}

// Ensure MockRulerClientCache implements RulerClientCacheInterface
var _ RulerClientCacheInterface = (*MockRulerClientCache)(nil)

// NewMockRulerClientCache creates a new mock cache for testing
func NewMockRulerClientCache() *MockRulerClientCache {
	return &MockRulerClientCache{
		clients: map[string]AwarenessClient{},
	}
}

// AddMimirClient simulates adding a Mimir client with validation
func (m *MockRulerClientCache) AddMimirClient(address string, name string, ctx context.Context) error {
	// Validate URL format
	parsedURL, err := url.Parse(address)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Check for invalid URL scheme
	if parsedURL.Scheme == "" {
		return errors.New("missing URL scheme")
	}

	// Simulate DNS resolution failure for specific test hosts
	if strings.Contains(address, "unreachable-host") {
		return fmt.Errorf("dial tcp: lookup unreachable-host-12345.local: no such host")
	}

	// Simulate successful connection for valid URLs
	m.clients[name] = &MockAwarenessClient{}
	return nil
}

// AddPromClient simulates adding a Prometheus client
func (m *MockRulerClientCache) AddPromClient(address string, name string, ctx context.Context) error {
	return errors.New("Prometheus client not yet implemented")
}

// RemoveClient removes a client from the cache
func (m *MockRulerClientCache) RemoveClient(name string) {
	if m.clients[name] == nil {
		return
	}
	delete(m.clients, name)
}

// GetClient retrieves a client from the cache
func (m *MockRulerClientCache) GetClient(name string) (AwarenessClient, error) {
	if client, exists := m.clients[name]; exists {
		return client, nil
	}
	return nil, errors.New("client not found")
}

// MockAwarenessClient is a mock implementation of AwarenessClient for testing
type MockAwarenessClient struct{}

func (m *MockAwarenessClient) CreateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup) error {
	return nil
}

func (m *MockAwarenessClient) DeleteRuleGroup(ctx context.Context, namespace, groupName string) error {
	return nil
}

func (m *MockAwarenessClient) GetRuleGroup(ctx context.Context, namespace, groupName string) (*rulefmt.RuleGroup, error) {
	return nil, nil
}

func (m *MockAwarenessClient) ListRules(ctx context.Context, namespace string) (map[string][]rulefmt.RuleGroup, error) {
	return nil, nil
}

func (m *MockAwarenessClient) DeleteNamespace(ctx context.Context, namespace string) error {
	return nil
}

func (m *MockAwarenessClient) CreateAlertmanagerConfig(ctx context.Context, cfg string, templates map[string]string) error {
	return nil
}

func (m *MockAwarenessClient) DeleteAlermanagerConfig(ctx context.Context) error {
	return nil
}

func (m *MockAwarenessClient) GetAlertmanagerConfig(ctx context.Context) (string, map[string]string, error) {
	return "", nil, nil
}
