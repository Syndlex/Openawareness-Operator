// Package mimir provides client implementations for interacting with Grafana Mimir APIs.
package mimir

import (
	"bytes"
	"context"
	"io"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const alertmanagerAPI = "/api/v1/alerts"
const alertmanagerAPIStatus = "/multitenant_alertmanager/status"

type configCompat struct {
	TemplateFiles      map[string]string `yaml:"template_files"`
	AlertmanagerConfig string            `yaml:"alertmanager_config"`
}

// CreateAlertmanagerConfig creates or updates the Alertmanager configuration for the tenant.
// It packages the configuration and templates into the required format and sends it to the Mimir API.
// Returns an error if marshaling or the API request fails.
func (r *Client) CreateAlertmanagerConfig(ctx context.Context, cfg string, templates map[string]string) error {
	payload, err := yaml.Marshal(&configCompat{
		TemplateFiles:      templates,
		AlertmanagerConfig: cfg,
	})
	if err != nil {
		return err
	}

	res, err := r.doRequest(ctx, alertmanagerAPI, "POST", bytes.NewBuffer(payload), int64(len(payload)))
	if err != nil {
		return err
	}

	if err := res.Body.Close(); err != nil {
		return err
	}

	return nil
}

// DeleteAlermanagerConfig deletes the tenant's Alertmanager configuration.
// Returns an error if the API request fails.
func (r *Client) DeleteAlermanagerConfig(ctx context.Context) error {
	res, err := r.doRequest(ctx, alertmanagerAPI, "DELETE", nil, -1)
	if err != nil {
		return err
	}

	if err := res.Body.Close(); err != nil {
		return err
	}

	return nil
}

// GetAlertmanagerConfig retrieves the tenant's Alertmanager configuration from Mimir.
// Returns the configuration string, template files map, and an error if the request or unmarshaling fails.
func (r *Client) GetAlertmanagerConfig(ctx context.Context) (string, map[string]string, error) {
	res, err := r.doRequest(ctx, alertmanagerAPI, "GET", nil, -1)
	if err != nil {
		log.Debugln("no alert config present in response")
		return "", nil, err
	}

	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", nil, err
	}

	compat := configCompat{}
	err = yaml.Unmarshal(body, &compat)
	if err != nil {
		log.WithFields(log.Fields{
			"body": string(body),
		}).Debugln("failed to unmarshal rule group from response")

		return "", nil, errors.Wrap(err, "unable to unmarshal response")
	}

	return compat.AlertmanagerConfig, compat.TemplateFiles, nil
}

// GetAlertmanagerStatus retrieves the status of the Alertmanager for the tenant.
// Returns the raw status response as a string, or an error if the request fails.
func (r *Client) GetAlertmanagerStatus(ctx context.Context) (string, error) {
	res, err := r.doRequest(ctx, alertmanagerAPIStatus, "GET", nil, -1)
	if err != nil {
		log.Debugln("failed to get alertmanager status")
		return "", err
	}

	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
