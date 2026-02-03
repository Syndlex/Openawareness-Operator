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
	clients        map[string]AwarenessClient
	getClientError error
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
func (m *MockRulerClientCache) AddMimirClient(address string, name string, tenantID string, ctx context.Context) error {
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

// GetOrCreateMimirClient gets an existing client or creates a new one for the given tenant
func (m *MockRulerClientCache) GetOrCreateMimirClient(address string, clientName string, tenantID string, ctx context.Context) (AwarenessClient, error) {
	// Create composite key: clientName + tenantID
	cacheKey := fmt.Sprintf("%s-%s", clientName, tenantID)

	// Check if client already exists
	if client, exists := m.clients[cacheKey]; exists {
		return client, nil
	}

	// Create new client with tenant ID
	if err := m.AddMimirClient(address, cacheKey, tenantID, ctx); err != nil {
		return nil, fmt.Errorf("creating Mimir client for tenant %s: %w", tenantID, err)
	}

	return m.clients[cacheKey], nil
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
	if m.getClientError != nil {
		return nil, m.getClientError
	}
	if client, exists := m.clients[name]; exists {
		return client, nil
	}
	return nil, errors.New("client not found")
}

// SetGetClientError sets an error to be returned by GetClient
func (m *MockRulerClientCache) SetGetClientError(err error) {
	m.getClientError = err
}

// SetClient manually sets a client in the cache for testing
func (m *MockRulerClientCache) SetClient(name string, client AwarenessClient) {
	m.clients[name] = client
}

// MockAwarenessClient is a mock implementation of AwarenessClient for testing
type MockAwarenessClient struct {
	createRuleGroupError   error
	deleteRuleGroupError   error
	createAlertConfigError error
	deleteAlertConfigError error
}

// NewMockAwarenessClient creates a new mock awareness client
func NewMockAwarenessClient() *MockAwarenessClient {
	return &MockAwarenessClient{}
}

// SetCreateRuleGroupError sets an error to be returned by CreateRuleGroup
func (m *MockAwarenessClient) SetCreateRuleGroupError(err error) {
	m.createRuleGroupError = err
}

// SetDeleteRuleGroupError sets an error to be returned by DeleteRuleGroup
func (m *MockAwarenessClient) SetDeleteRuleGroupError(err error) {
	m.deleteRuleGroupError = err
}

// SetCreateAlertConfigError sets an error to be returned by CreateAlertmanagerConfig
func (m *MockAwarenessClient) SetCreateAlertConfigError(err error) {
	m.createAlertConfigError = err
}

// SetDeleteAlertConfigError sets an error to be returned by DeleteAlermanagerConfig
func (m *MockAwarenessClient) SetDeleteAlertConfigError(err error) {
	m.deleteAlertConfigError = err
}

func (m *MockAwarenessClient) CreateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup) error {
	if m.createRuleGroupError != nil {
		return m.createRuleGroupError
	}
	return nil
}

func (m *MockAwarenessClient) DeleteRuleGroup(ctx context.Context, namespace, groupName string) error {
	if m.deleteRuleGroupError != nil {
		return m.deleteRuleGroupError
	}
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
	if m.createAlertConfigError != nil {
		return m.createAlertConfigError
	}
	return nil
}

func (m *MockAwarenessClient) DeleteAlermanagerConfig(ctx context.Context) error {
	if m.deleteAlertConfigError != nil {
		return m.deleteAlertConfigError
	}
	return nil
}

func (m *MockAwarenessClient) GetAlertmanagerConfig(ctx context.Context) (string, map[string]string, error) {
	return "", nil, nil
}

func (m *MockAwarenessClient) GetAlertmanagerStatus(ctx context.Context) (string, error) {
	return "", nil
}
