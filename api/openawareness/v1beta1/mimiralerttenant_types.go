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

// MimirAlertTenantStatus defines the observed state of MimirAlertTenant
type MimirAlertTenantStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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
