package mimir

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"

	"github.com/prometheus/prometheus/model/rulefmt"

	"gopkg.in/yaml.v3"
)

// CreateRuleGroup creates or updates a rule group in the specified namespace.
// It marshals the rule group to YAML and sends it to the Mimir API.
// The tenantID parameter specifies which tenant this rule group belongs to.
// Returns an error if marshaling fails or if the API request fails.
func (r *Client) CreateRuleGroup(ctx context.Context, namespace string, rg rulefmt.RuleGroup, tenantID string) error {
	payload, err := yaml.Marshal(&rg)
	if err != nil {
		return err
	}

	escapedNamespace := url.PathEscape(namespace)
	path := r.apiPath + "/" + escapedNamespace

	res, err := r.doRequest(ctx, path, "POST", bytes.NewBuffer(payload), int64(len(payload)), tenantID)
	if err != nil {
		return err
	}

	if err := res.Body.Close(); err != nil {
		return err
	}

	return nil
}

// DeleteRuleGroup deletes a specific rule group from the given namespace.
// The tenantID parameter specifies which tenant this rule group belongs to.
// Returns an error if the API request fails.
func (r *Client) DeleteRuleGroup(ctx context.Context, namespace, groupName string, tenantID string) error {
	escapedNamespace := url.PathEscape(namespace)
	escapedGroupName := url.PathEscape(groupName)
	path := r.apiPath + "/" + escapedNamespace + "/" + escapedGroupName

	res, err := r.doRequest(ctx, path, "DELETE", nil, -1, tenantID)
	if err != nil {
		return err
	}

	if err := res.Body.Close(); err != nil {
		return err
	}

	return nil
}

// GetRuleGroup retrieves a specific rule group from the given namespace.
// The tenantID parameter specifies which tenant this rule group belongs to.
// Returns the rule group or an error if the API request or unmarshaling fails.
func (r *Client) GetRuleGroup(ctx context.Context, namespace, groupName string, tenantID string) (*rulefmt.RuleGroup, error) {
	escapedNamespace := url.PathEscape(namespace)
	escapedGroupName := url.PathEscape(groupName)
	path := r.apiPath + "/" + escapedNamespace + "/" + escapedGroupName

	fmt.Println(path)
	res, err := r.doRequest(ctx, path, "GET", nil, -1, tenantID)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)

	if err != nil {
		return nil, err
	}

	rg := rulefmt.RuleGroup{}
	err = yaml.Unmarshal(body, &rg)
	if err != nil {
		r.log.Info("failed to unmarshal rule group from response",
			"body", body,
		)

		return nil, fmt.Errorf("unable to unmarshal response, %w", err)
	}

	return &rg, nil
}

// ListRules retrieves all rule groups, optionally filtered by namespace.
// If namespace is empty, retrieves all rule groups for the tenant.
// The tenantID parameter specifies which tenant to query.
// Returns a map of namespace to rule groups, or an error if the request fails.
func (r *Client) ListRules(ctx context.Context, namespace string, tenantID string) (map[string][]rulefmt.RuleGroup, error) {
	path := r.apiPath
	if namespace != "" {
		path = path + "/" + namespace
	}

	res, err := r.doRequest(ctx, path, "GET", nil, -1, tenantID)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)

	if err != nil {
		return nil, err
	}

	ruleSet := map[string][]rulefmt.RuleGroup{}
	err = yaml.Unmarshal(body, &ruleSet)
	if err != nil {
		return nil, err
	}

	return ruleSet, nil
}

// DeleteNamespace deletes all rule groups in a namespace including the namespace itself.
// The tenantID parameter specifies which tenant this namespace belongs to.
// Returns an error if the API request fails.
func (r *Client) DeleteNamespace(ctx context.Context, namespace string, tenantID string) error {
	escapedNamespace := url.PathEscape(namespace)
	path := r.apiPath + "/" + escapedNamespace

	res, err := r.doRequest(ctx, path, "DELETE", nil, -1, tenantID)
	if err != nil {
		return err
	}

	if err := res.Body.Close(); err != nil {
		return err
	}

	return nil
}
