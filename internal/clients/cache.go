package clients

import (
	"context"
	"errors"
	"fmt"

	"github.com/grafana/dskit/crypto/tls"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/syndlex/openawareness-controller/internal/mimir"
)

type AwarenessClient interface {
	CreateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup) error
	DeleteRuleGroup(ctx context.Context, namespace, groupName string) error
	GetRuleGroup(ctx context.Context, namespace, groupName string) (*rulefmt.RuleGroup, error)
	ListRules(ctx context.Context, namespace string) (map[string][]rulefmt.RuleGroup, error)
	DeleteNamespace(ctx context.Context, namespace string) error
	CreateAlertmanagerConfig(ctx context.Context, cfg string, templates map[string]string) error
	DeleteAlermanagerConfig(ctx context.Context) error
	GetAlertmanagerConfig(ctx context.Context) (string, map[string]string, error)
}

type RulerClientCache struct {
	clients map[string]AwarenessClient
}

func NewRulerClientCache() *RulerClientCache {
	return &RulerClientCache{
		clients: map[string]AwarenessClient{},
	}
}

func (e *RulerClientCache) AddMimirClient(address string, name string, ctx context.Context) error {
	client, err := mimir.New(mimir.Config{
		User:            "",
		Key:             "",
		Address:         address,
		ID:              "",
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
