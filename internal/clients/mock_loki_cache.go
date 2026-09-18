package clients

import (
	"context"
	"errors"
	"sync"

	"github.com/prometheus/prometheus/model/rulefmt"
)

// errLokiNotFound is returned by the mock when a rule group does not exist.
var errLokiNotFound = errors.New("rule group not found")

// MockLokiClientCache is a test double for LokiClientCacheInterface.
type MockLokiClientCache struct {
	mu      sync.RWMutex
	clients map[string]LokiClientInterface
}

// Ensure compile-time interface satisfaction.
var _ LokiClientCacheInterface = (*MockLokiClientCache)(nil)

// NewMockLokiClientCache returns an empty MockLokiClientCache.
func NewMockLokiClientCache() *MockLokiClientCache {
	return &MockLokiClientCache{clients: make(map[string]LokiClientInterface)}
}

// SetClient registers a pre-built LokiClientInterface under the given name.
func (m *MockLokiClientCache) SetClient(name string, cl LokiClientInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = cl
}

// GetOrCreateLokiClient returns the registered client or a fresh MockLokiClient.
func (m *MockLokiClientCache) GetOrCreateLokiClient(_ context.Context, _, name string) (LokiClientInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cl, ok := m.clients[name]; ok {
		return cl, nil
	}
	cl := NewMockLokiClient()
	m.clients[name] = cl
	return cl, nil
}

// RemoveLokiClient removes the client with the given name.
func (m *MockLokiClientCache) RemoveLokiClient(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, name)
}

// MockLokiClient is an in-memory LokiClientInterface for unit tests.
// It records every call so tests can assert on the interactions.
type MockLokiClient struct {
	mu sync.RWMutex

	// Stored rules: namespace → groupName → RuleGroup
	rules map[string]map[string]rulefmt.RuleGroup

	// Configurable error injection (private — use setters from tests).
	createError error
	deleteError error
	listError   error
}

// Ensure compile-time interface satisfaction.
var _ LokiClientInterface = (*MockLokiClient)(nil)

// NewMockLokiClient returns an empty MockLokiClient.
func NewMockLokiClient() *MockLokiClient {
	return &MockLokiClient{rules: make(map[string]map[string]rulefmt.RuleGroup)}
}

// SetCreateError configures the error returned by CreateOrUpdateRuleGroup.
func (m *MockLokiClient) SetCreateError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createError = err
}

// SetDeleteError configures the error returned by DeleteRuleGroup and DeleteNamespace.
func (m *MockLokiClient) SetDeleteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteError = err
}

// SetListError configures the error returned by ListRules.
func (m *MockLokiClient) SetListError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listError = err
}

// CreateOrUpdateRuleGroup stores the rule group in memory.
func (m *MockLokiClient) CreateOrUpdateRuleGroup(_ context.Context, namespace string, rg rulefmt.RuleGroup, _ string) error {
	m.mu.RLock()
	createErr := m.createError
	m.mu.RUnlock()
	if createErr != nil {
		return createErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rules[namespace] == nil {
		m.rules[namespace] = make(map[string]rulefmt.RuleGroup)
	}
	m.rules[namespace][rg.Name] = rg
	return nil
}

// DeleteRuleGroup removes the rule group from memory.
func (m *MockLokiClient) DeleteRuleGroup(_ context.Context, namespace, groupName, _ string) error {
	m.mu.RLock()
	deleteErr := m.deleteError
	m.mu.RUnlock()
	if deleteErr != nil {
		return deleteErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if ns, ok := m.rules[namespace]; ok {
		delete(ns, groupName)
	}
	return nil
}

// DeleteNamespace removes all rule groups under the namespace.
func (m *MockLokiClient) DeleteNamespace(_ context.Context, namespace, _ string) error {
	m.mu.RLock()
	deleteErr := m.deleteError
	m.mu.RUnlock()
	if deleteErr != nil {
		return deleteErr
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, namespace)
	return nil
}

// GetRuleGroup retrieves a rule group from memory.
func (m *MockLokiClient) GetRuleGroup(_ context.Context, namespace, groupName, _ string) (*rulefmt.RuleGroup, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ns, ok := m.rules[namespace]; ok {
		if rg, ok := ns[groupName]; ok {
			return &rg, nil
		}
	}
	return nil, errLokiNotFound
}

// ListRules returns all stored rule groups, optionally filtered to a namespace.
func (m *MockLokiClient) ListRules(_ context.Context, namespace, _ string) (map[string][]rulefmt.RuleGroup, error) {
	m.mu.RLock()
	listErr := m.listError
	m.mu.RUnlock()
	if listErr != nil {
		return nil, listErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string][]rulefmt.RuleGroup)
	for ns, groups := range m.rules {
		if namespace != "" && ns != namespace {
			continue
		}
		for _, rg := range groups {
			result[ns] = append(result[ns], rg)
		}
	}
	return result, nil
}

// HealthCheck always succeeds for the mock.
func (m *MockLokiClient) HealthCheck(_ context.Context) error { return nil }

// RuleCount returns the number of stored rule groups in a namespace (test helper).
func (m *MockLokiClient) RuleCount(namespace string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.rules[namespace])
}

// HasGroup returns whether a specific group exists in memory (test helper).
func (m *MockLokiClient) HasGroup(namespace, groupName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ns, ok := m.rules[namespace]; ok {
		_, exists := ns[groupName]
		return exists
	}
	return false
}
