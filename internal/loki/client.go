// Package loki provides an HTTP client for the Loki Ruler API.
package loki

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-logr/logr"
	"github.com/prometheus/prometheus/model/rulefmt"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	rulerAPIPath = "/loki/api/v1/rules"
)

// ErrResourceNotFound is returned when the Loki Ruler returns HTTP 404.
var ErrResourceNotFound = errors.New("requested resource not found")

// Config holds the configuration for a LokiClient.
type Config struct {
	// Address is the base URL of the Loki instance, e.g. "http://loki-backend.telemetry.svc.cluster.local:3100"
	Address string `yaml:"address"`

	// AuthToken is an optional Bearer token added to every request.
	AuthToken string `yaml:"authToken,omitempty"`

	// ExtraHeaders are additional HTTP headers added to every request.
	ExtraHeaders map[string]string `yaml:"extraHeaders,omitempty"`
}

// Client is an HTTP client for the Loki Ruler API.
type Client struct {
	endpoint     *url.URL
	httpClient   http.Client
	authToken    string
	extraHeaders map[string]string
	log          logr.Logger
}

// New creates a new Loki Ruler API client from the given Config.
func New(ctx context.Context, cfg Config) (*Client, error) {
	logger := log.FromContext(ctx)

	endpoint, err := url.Parse(cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("parsing loki address %q: %w", cfg.Address, err)
	}

	logger.Info("New Loki client created", "address", cfg.Address)

	return &Client{
		endpoint:     endpoint,
		httpClient:   http.Client{},
		authToken:    cfg.AuthToken,
		extraHeaders: cfg.ExtraHeaders,
		log:          logger,
	}, nil
}

// HealthCheck verifies connectivity by listing all rule namespaces.
func (c *Client) HealthCheck(ctx context.Context) error {
	c.log.V(1).Info("Performing Loki health check")

	resp, err := c.doRequest(ctx, rulerAPIPath, http.MethodGet, nil, -1, "")
	if err != nil {
		c.log.Error(err, "Loki health check failed")
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	c.log.Info("Loki health check successful", "status", resp.Status)
	return nil
}

// CreateOrUpdateRuleGroup pushes a single rule group to the Loki Ruler for the given namespace and tenant.
// Loki's POST /loki/api/v1/rules/{namespace} upserts exactly one rule group per request.
func (c *Client) CreateOrUpdateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup, tenantID string) error {
	payload, err := yaml.Marshal(&rg)
	if err != nil {
		return fmt.Errorf("marshaling rule group %q: %w", rg.Name, err)
	}

	path := rulerAPIPath + "/" + url.PathEscape(namespace)
	resp, err := c.doRequest(ctx, path, http.MethodPost, bytes.NewBuffer(payload), int64(len(payload)), tenantID)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// DeleteRuleGroup deletes a single rule group from the given namespace and tenant.
func (c *Client) DeleteRuleGroup(ctx context.Context, namespace, groupName, tenantID string) error {
	path := rulerAPIPath + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(groupName)
	resp, err := c.doRequest(ctx, path, http.MethodDelete, nil, -1, tenantID)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// DeleteNamespace deletes all rule groups under the given namespace for the given tenant.
func (c *Client) DeleteNamespace(ctx context.Context, namespace, tenantID string) error {
	path := rulerAPIPath + "/" + url.PathEscape(namespace)
	resp, err := c.doRequest(ctx, path, http.MethodDelete, nil, -1, tenantID)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

// GetRuleGroup retrieves a single rule group from Loki.
func (c *Client) GetRuleGroup(ctx context.Context, namespace, groupName, tenantID string) (*rulefmt.RuleGroup, error) {
	path := rulerAPIPath + "/" + url.PathEscape(namespace) + "/" + url.PathEscape(groupName)
	resp, err := c.doRequest(ctx, path, http.MethodGet, nil, -1, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	rg := &rulefmt.RuleGroup{}
	if err := yaml.Unmarshal(body, rg); err != nil {
		return nil, fmt.Errorf("unmarshaling rule group: %w", err)
	}
	return rg, nil
}

// ListRules retrieves all rule groups for the given tenant, optionally scoped to a namespace.
// Returns a map of namespace → []RuleGroup.
func (c *Client) ListRules(ctx context.Context, namespace, tenantID string) (map[string][]rulefmt.RuleGroup, error) {
	path := rulerAPIPath
	if namespace != "" {
		path = path + "/" + url.PathEscape(namespace)
	}

	resp, err := c.doRequest(ctx, path, http.MethodGet, nil, -1, tenantID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	result := map[string][]rulefmt.RuleGroup{}
	if err := yaml.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshaling rule list: %w", err)
	}
	return result, nil
}

// doRequest executes an HTTP request against the Loki API, setting auth and tenant headers.
func (c *Client) doRequest(
	ctx context.Context,
	path, method string,
	payload io.Reader,
	contentLength int64,
	tenantID string,
) (*http.Response, error) {
	// Build full URL
	target := *c.endpoint
	if target.RawPath != "" {
		target.RawPath = strings.TrimSuffix(target.RawPath, "/") + path
	}
	target.Path = strings.TrimSuffix(target.Path, "/") + path

	req, err := http.NewRequestWithContext(ctx, method, target.String(), payload)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	if contentLength >= 0 {
		req.ContentLength = contentLength
	}

	req.Header.Set("User-Agent", "openawareness.operator/loki")
	req.Header.Set("Content-Type", "application/yaml")

	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	for k, v := range c.extraHeaders {
		req.Header.Set(k, v)
	}
	if tenantID != "" {
		req.Header.Set("X-Scope-OrgID", tenantID)
	}

	c.log.Info("sending request to Loki Ruler API",
		"method", method,
		"url", req.URL.String(),
		"tenant", tenantID,
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error(err, "Loki Ruler API request failed", "method", method, "url", req.URL.String())
		return nil, err
	}

	if err := c.checkResponse(resp); err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w: %s %s", err, method, req.URL.String())
	}

	return resp, nil
}

// checkResponse returns an error for non-2xx responses.
func (c *Client) checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return nil
	}

	bodyHead, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	bodyStr := string(bodyHead)

	c.log.Info("Loki Ruler API non-2xx response",
		"status", resp.Status,
		"body", bodyStr,
	)

	if resp.StatusCode == http.StatusNotFound {
		return ErrResourceNotFound
	}

	if bodyStr != "" {
		return fmt.Errorf("server returned HTTP status %s, body: %q", resp.Status, bodyStr)
	}
	return fmt.Errorf("server returned HTTP status %s", resp.Status)
}
