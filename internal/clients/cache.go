package clients

import (
	"context"
	"errors"
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

func (e *RulerClientCache) AddMimirClient(address string, name string, ctx context.Context) {
	client, _ := mimir.New(mimir.Config{
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
	e.clients[name] = client
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

func (e *RulerClientCache) AddPromClient(address string, name string, ctx context.Context) {
	panic("implement me")
}
