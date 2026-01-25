/*
Copyright 2024 Syndlex.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"fmt"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MimirAlertTenantSpec defines the desired state of MimirAlertTenant
type MimirAlertTenantSpec struct {
	// TemplateFiles contains Alertmanager notification templates
	// Key is the template name, value is the template content
	// +optional
	TemplateFiles map[string]string `json:"templateFiles,omitempty"`

	// AlertmanagerConfig contains the raw Alertmanager configuration in YAML format
	// This should include global settings, routes, receivers, etc.
	// +kubebuilder:validation:Required
	AlertmanagerConfig string `json:"alertmanagerConfig"`
}

// Condition types for MimirAlertTenant
const (
	// ConditionTypeConfigValid indicates whether the Alertmanager configuration is valid
	ConditionTypeConfigValid = "ConfigValid"
	// ConditionTypeSynced indicates whether the configuration has been synced to Mimir
	ConditionTypeSynced = "Synced"
)

// Condition reasons for MimirAlertTenant
const (
	// Configuration validation reasons
	ReasonInvalidYAML               = "InvalidYAML"
	ReasonInvalidAlertmanagerConfig = "InvalidAlertmanagerConfig"
	ReasonMissingTemplates          = "MissingTemplates"
	ReasonInvalidTemplateFormat     = "InvalidTemplateFormat"
	ReasonConfigValidated           = "ConfigValidated"

	// Resource/annotation reasons
	ReasonMissingAnnotations = "MissingAnnotations"
	ReasonMissingClientName  = "MissingClientName"
	ReasonClientNotFound     = "ClientNotFound"
	ReasonClientNotReady     = "ClientNotReady"

	// API/network reasons (reusing from ClientConfig where possible)
	ReasonConflict = "Conflict"

	// Success reasons
	ReasonSynced = "Synced"
)

// Sync status values
const (
	SyncStatusSynced  = "Synced"
	SyncStatusFailed  = "Failed"
	SyncStatusPending = "Pending"
)

// Configuration validation values
const (
	ConfigValidationValid   = "Valid"
	ConfigValidationInvalid = "Invalid"
)

// MimirAlertTenantStatus defines the observed state of MimirAlertTenant
type MimirAlertTenantStatus struct {
	// Conditions represent the latest available observations of the MimirAlertTenant's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync to Mimir
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// SyncStatus indicates the current state of the alertmanager configuration
	// Possible values: "Synced", "Failed", "Pending"
	// +optional
	SyncStatus string `json:"syncStatus,omitempty"`

	// ErrorMessage contains detailed error information if sync failed
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`

	// ConfigurationValidation indicates whether the alertmanager config is valid
	// +optional
	ConfigurationValidation string `json:"configurationValidation,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MimirAlertTenant is the Schema for the mimiralerttenants API
type MimirAlertTenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MimirAlertTenantSpec   `json:"spec,omitempty"`
	Status MimirAlertTenantStatus `json:"status,omitempty"`
}

// ToConfigDTO returns the Alertmanager configuration as a YAML string.
// The configuration is taken directly from the spec.
func (tenant *MimirAlertTenant) ToConfigDTO() string {
	return tenant.Spec.AlertmanagerConfig
}

// ToTemplatesDTO returns the template files as a map.
// Returns an empty map if no templates are defined.
func (tenant *MimirAlertTenant) ToTemplatesDTO() map[string]string {
	if tenant.Spec.TemplateFiles == nil {
		return map[string]string{}
	}
	return tenant.Spec.TemplateFiles
}

// ValidateAlertmanagerConfig validates that the AlertmanagerConfig is valid YAML.
// Returns an error if the configuration cannot be unmarshaled.
func (tenant *MimirAlertTenant) ValidateAlertmanagerConfig() error {
	if tenant.Spec.AlertmanagerConfig == "" {
		return fmt.Errorf("alertmanagerConfig is required")
	}

	// Try to unmarshal to ensure it's valid YAML
	var config interface{}
	if err := yaml.Unmarshal([]byte(tenant.Spec.AlertmanagerConfig), &config); err != nil {
		return fmt.Errorf("invalid YAML in alertmanagerConfig: %w", err)
	}

	return nil
}

// SetSyncedCondition updates the status to indicate successful sync to Mimir.
func (tenant *MimirAlertTenant) SetSyncedCondition() {
	now := metav1.Now()
	tenant.Status.LastSyncTime = &now
	tenant.Status.SyncStatus = SyncStatusSynced
	tenant.Status.ErrorMessage = ""
	tenant.Status.ConfigurationValidation = ConfigValidationValid

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonSynced,
		Message:            "Alertmanager configuration successfully synced to Mimir",
		LastTransitionTime: now,
	})

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeConfigValid,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonConfigValidated,
		Message:            "Alertmanager configuration is valid",
		LastTransitionTime: now,
	})

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeSynced,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonSynced,
		Message:            "Configuration synced to Mimir",
		LastTransitionTime: now,
	})
}

// SetFailedCondition updates the status to indicate a failed sync to Mimir.
func (tenant *MimirAlertTenant) SetFailedCondition(reason, message string) {
	now := metav1.Now()
	tenant.Status.SyncStatus = SyncStatusFailed
	tenant.Status.ErrorMessage = message

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeSynced,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

// SetConfigInvalidCondition updates the status to indicate invalid configuration.
func (tenant *MimirAlertTenant) SetConfigInvalidCondition(reason, message string) {
	now := metav1.Now()
	tenant.Status.SyncStatus = SyncStatusFailed
	tenant.Status.ErrorMessage = message
	tenant.Status.ConfigurationValidation = ConfigValidationInvalid

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeConfigValid,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})

	tenant.setCondition(metav1.Condition{
		Type:               ConditionTypeSynced,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            "Cannot sync invalid configuration",
		LastTransitionTime: now,
	})
}

// setCondition sets or updates a condition in the status.
// If a condition with the same type exists, it updates it; otherwise, it appends the new condition.
func (tenant *MimirAlertTenant) setCondition(newCondition metav1.Condition) {
	existingConditions := tenant.Status.Conditions
	for i, condition := range existingConditions {
		if condition.Type == newCondition.Type {
			// Update existing condition
			existingConditions[i] = newCondition
			tenant.Status.Conditions = existingConditions
			return
		}
	}
	// Append new condition
	tenant.Status.Conditions = append(existingConditions, newCondition)
}

// +kubebuilder:object:root=true

// MimirAlertTenantList contains a list of MimirAlertTenant
type MimirAlertTenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MimirAlertTenant `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MimirAlertTenant{}, &MimirAlertTenantList{})
}
