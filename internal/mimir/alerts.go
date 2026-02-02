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

// CreateAlertmanagerConfig creates a new alertmanager config
func (r *MimirClient) CreateAlertmanagerConfig(ctx context.Context, cfg string, templates map[string]string) error {
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

	res.Body.Close()

	return nil
}

// DeleteAlermanagerConfig deletes the users alertmanagerconfig
func (r *MimirClient) DeleteAlermanagerConfig(ctx context.Context) error {
	res, err := r.doRequest(ctx, alertmanagerAPI, "DELETE", nil, -1)
	if err != nil {
		return err
	}

	res.Body.Close()

	return nil
}

// GetAlertmanagerConfig retrieves a Mimir cluster's Alertmanager config.
func (r *MimirClient) GetAlertmanagerConfig(ctx context.Context) (string, map[string]string, error) {
	res, err := r.doRequest(ctx, alertmanagerAPI, "GET", nil, -1)
	if err != nil {
		log.Debugln("no alert config present in response")
		return "", nil, err
	}

	defer res.Body.Close()
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
// Returns the raw status response as a string.
func (r *MimirClient) GetAlertmanagerStatus(ctx context.Context) (string, error) {
	res, err := r.doRequest(ctx, alertmanagerAPIStatus, "GET", nil, -1)
	if err != nil {
		log.Debugln("failed to get alertmanager status")
		return "", err
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
