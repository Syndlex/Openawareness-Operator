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
func (m *MockRulerClientCache) AddMimirClient(_ context.Context, address string, name string) error {
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

// GetOrCreateMimirClient gets an existing client or creates a new one.
// One client handles all tenants for that Mimir instance via X-Scope-OrgID header.
func (m *MockRulerClientCache) GetOrCreateMimirClient(
	ctx context.Context,
	address string,
	clientName string,
) (AwarenessClient, error) {
	// Check if client already exists using simple client name
	if client, exists := m.clients[clientName]; exists {
		return client, nil
	}

	// Create new client
	if err := m.AddMimirClient(ctx, address, clientName); err != nil {
		return nil, fmt.Errorf("creating Mimir client: %w", err)
	}

	return m.clients[clientName], nil
}

// AddPromClient simulates adding a Prometheus client
func (m *MockRulerClientCache) AddPromClient(_ context.Context, _ string, _ string) error {
	return errors.New("prometheus client not yet implemented")
}

// RemoveClient removes a client from the cache
func (m *MockRulerClientCache) RemoveClient(name string) {
	if m.clients[name] == nil {
		return
	}
	delete(m.clients, name)
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

// CreateRuleGroup creates or updates a rule group in the mock client.
func (m *MockAwarenessClient) CreateRuleGroup(_ context.Context, _ string, _ rulefmt.RuleGroup, _ string) error {
	if m.createRuleGroupError != nil {
		return m.createRuleGroupError
	}
	return nil
}

// DeleteRuleGroup deletes a rule group from the mock client.
func (m *MockAwarenessClient) DeleteRuleGroup(_ context.Context, _, _ string, _ string) error {
	if m.deleteRuleGroupError != nil {
		return m.deleteRuleGroupError
	}
	return nil
}

// GetRuleGroup retrieves a rule group from the mock client.
func (m *MockAwarenessClient) GetRuleGroup(_ context.Context, _, _ string, _ string) (*rulefmt.RuleGroup, error) {
	return nil, nil
}

// ListRules lists all rules in a namespace from the mock client.
func (m *MockAwarenessClient) ListRules(_ context.Context, _ string, _ string) (map[string][]rulefmt.RuleGroup, error) {
	return nil, nil
}

// DeleteNamespace deletes a namespace from the mock client.
func (m *MockAwarenessClient) DeleteNamespace(_ context.Context, _ string, _ string) error {
	return nil
}

// CreateAlertmanagerConfig creates or updates an Alertmanager configuration in the mock client.
func (m *MockAwarenessClient) CreateAlertmanagerConfig(_ context.Context, _ string, _ map[string]string, _ string) error {
	if m.createAlertConfigError != nil {
		return m.createAlertConfigError
	}
	return nil
}

// DeleteAlermanagerConfig deletes the Alertmanager configuration from the mock client.
func (m *MockAwarenessClient) DeleteAlermanagerConfig(_ context.Context, _ string) error {
	if m.deleteAlertConfigError != nil {
		return m.deleteAlertConfigError
	}
	return nil
}

// GetAlertmanagerConfig retrieves the Alertmanager configuration from the mock client.
func (m *MockAwarenessClient) GetAlertmanagerConfig(_ context.Context, _ string) (string, map[string]string, error) {
	return "", nil, nil
}

// GetAlertmanagerStatus retrieves the Alertmanager status from the mock client.
func (m *MockAwarenessClient) GetAlertmanagerStatus(_ context.Context, _ string) (string, error) {
	return "", nil
}
