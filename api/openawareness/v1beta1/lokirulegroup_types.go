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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LokiRuleGroupSpec defines the desired state of LokiRuleGroup.
type LokiRuleGroupSpec struct {
	// Tenant is the Loki/Mimir X-Scope-OrgID header value used for multi-tenancy.
	// Falls back to the annotation openawareness.syndlex/tenant, then to "main".
	// +optional
	Tenant string `json:"tenant,omitempty"`

	// LokiClientRef is the name of a ClientConfig that points to a Loki instance.
	// When set, rule groups are pushed to Loki via the Loki Ruler API.
	// +optional
	LokiClientRef string `json:"lokiClientRef,omitempty"`

	// LokiNamespace overrides the Loki Ruler namespace used for this resource.
	// When unset, the Kubernetes namespace of this object is used.
	// Falls back to the annotation openawareness.syndlex/loki-namespace.
	// +optional
	LokiNamespace string `json:"lokiNamespace,omitempty"`

	// MimirClientRef is the name of a ClientConfig that points to a Mimir/Prometheus instance.
	// When set, rule groups are also pushed to Mimir via the Prometheus Ruler API.
	// +optional
	MimirClientRef string `json:"mimirClientRef,omitempty"`

	// PrometheusNamespace overrides the Mimir/Prometheus Ruler namespace used for this resource.
	// When unset, the Kubernetes namespace of this object is used.
	// Falls back to the annotation openawareness.syndlex/prometheus-namespace.
	// +optional
	PrometheusNamespace string `json:"prometheusNamespace,omitempty"`

	// Groups contains the rule groups to deploy. Each group follows the standard
	// Prometheus-compatible format; expressions should use LogQL for Loki targets
	// and PromQL for Mimir/Prometheus targets.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Groups []RuleGroup `json:"groups"`
}

// RuleGroup represents a single rule group (name + list of rules).
type RuleGroup struct {
	// Name is the unique name of the rule group.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Interval is the evaluation interval for the rules in this group (e.g. "1m", "5m").
	// +optional
	Interval string `json:"interval,omitempty"`

	// Rules is the list of alerting or recording rules in this group.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Rules []LokiRule `json:"rules"`
}

// LokiRule represents a single alerting or recording rule.
type LokiRule struct {
	// Alert is the name of the alert. Mutually exclusive with Record.
	// +optional
	Alert string `json:"alert,omitempty"`

	// Record is the name of the time series to record into. Mutually exclusive with Alert.
	// +optional
	Record string `json:"record,omitempty"`

	// Expr is the LogQL (for Loki) or PromQL (for Mimir/Prometheus) expression.
	// +kubebuilder:validation:Required
	Expr string `json:"expr"`

	// For defines how long the condition must be true before firing an alert.
	// Only applicable for alerting rules.
	// +optional
	For string `json:"for,omitempty"`

	// KeepFiringFor defines how long an alert should keep firing after the condition is no longer true.
	// +optional
	KeepFiringFor string `json:"keepFiringFor,omitempty"`

	// Labels are key-value pairs attached to the rule (e.g. severity labels).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are key-value pairs used to provide additional information for alerts.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Condition type constants for LokiRuleGroup.
const (
	// LokiRuleGroupConditionReady indicates the overall readiness of the resource.
	LokiRuleGroupConditionReady = "Ready"
	// LokiRuleGroupConditionLokiSynced indicates whether rules have been synced to Loki.
	LokiRuleGroupConditionLokiSynced = "LokiSynced"
	// LokiRuleGroupConditionMimirSynced indicates whether rules have been synced to Mimir/Prometheus.
	LokiRuleGroupConditionMimirSynced = "MimirSynced"
)

// Reason constants for LokiRuleGroup conditions.
const (
	LokiRuleGroupReasonSynced        = "Synced"
	LokiRuleGroupReasonSyncFailed    = "SyncFailed"
	LokiRuleGroupReasonClientMissing = "ClientMissing"
	LokiRuleGroupReasonDeleteFailed  = "DeleteFailed"
)

// LokiRuleGroupStatus defines the observed state of LokiRuleGroup.
type LokiRuleGroupStatus struct {
	// Conditions represent the latest available observations of the resource state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync.
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// SyncStatus is a human-readable summary of the current sync state ("Synced", "Failed", "Pending").
	// +optional
	SyncStatus string `json:"syncStatus,omitempty"`

	// ErrorMessage contains the last error message if a sync failed.
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`

	// ResolvedLokiNamespace is the effective Loki Ruler namespace after applying resolution priority.
	// +optional
	ResolvedLokiNamespace string `json:"resolvedLokiNamespace,omitempty"`

	// ResolvedPrometheusNamespace is the effective Mimir/Prometheus Ruler namespace.
	// +optional
	ResolvedPrometheusNamespace string `json:"resolvedPrometheusNamespace,omitempty"`

	// ActiveLokiClientRef is the lokiClientRef that was used in the last successful sync.
	// Used to detect client changes and clean up the old backend.
	// +optional
	ActiveLokiClientRef string `json:"activeLokiClientRef,omitempty"`

	// ActiveMimirClientRef is the mimirClientRef that was used in the last successful sync.
	// Used to detect client changes and clean up the old backend.
	// +optional
	ActiveMimirClientRef string `json:"activeMimirClientRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Tenant",type=string,JSONPath=`.spec.tenant`
// +kubebuilder:printcolumn:name="Loki NS",type=string,JSONPath=`.status.resolvedLokiNamespace`
// +kubebuilder:printcolumn:name="Prom NS",type=string,JSONPath=`.status.resolvedPrometheusNamespace`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.syncStatus`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// LokiRuleGroup is the Schema for the lokirulegroups API.
// It defines one or more LogQL alerting/recording rule groups that the controller
// deploys to a Loki Ruler and/or a Mimir/Prometheus Ruler instance.
type LokiRuleGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LokiRuleGroupSpec   `json:"spec,omitempty"`
	Status LokiRuleGroupStatus `json:"status,omitempty"`
}

// ResolveTenant returns the effective tenant ID for this resource.
// Priority: spec.tenant > annotation openawareness.syndlex/tenant > "main"
func (l *LokiRuleGroup) ResolveTenant() string {
	if l.Spec.Tenant != "" {
		return l.Spec.Tenant
	}
	if l.Annotations != nil {
		if t := l.Annotations["openawareness.syndlex/tenant"]; t != "" {
			return t
		}
	}
	return "main"
}

// ResolveLokiNamespace returns the effective Loki Ruler namespace.
// Priority: spec.lokiNamespace > annotation openawareness.syndlex/loki-namespace > metadata.namespace
func (l *LokiRuleGroup) ResolveLokiNamespace() string {
	if l.Spec.LokiNamespace != "" {
		return l.Spec.LokiNamespace
	}
	if l.Annotations != nil {
		if ns := l.Annotations["openawareness.syndlex/loki-namespace"]; ns != "" {
			return ns
		}
	}
	return l.Namespace
}

// ResolvePrometheusNamespace returns the effective Mimir/Prometheus Ruler namespace.
// Priority: spec.prometheusNamespace > annotation openawareness.syndlex/prometheus-namespace > metadata.namespace
func (l *LokiRuleGroup) ResolvePrometheusNamespace() string {
	if l.Spec.PrometheusNamespace != "" {
		return l.Spec.PrometheusNamespace
	}
	if l.Annotations != nil {
		if ns := l.Annotations["openawareness.syndlex/prometheus-namespace"]; ns != "" {
			return ns
		}
	}
	return l.Namespace
}

// SetSyncedCondition marks the resource as fully synced.
func (l *LokiRuleGroup) SetSyncedCondition(lokiEnabled, mimirEnabled bool) {
	now := metav1.Now()
	l.Status.LastSyncTime = &now
	l.Status.SyncStatus = SyncStatusSynced
	l.Status.ErrorMessage = ""

	l.setCondition(metav1.Condition{
		Type:               LokiRuleGroupConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             LokiRuleGroupReasonSynced,
		Message:            "Rule groups successfully synced",
		LastTransitionTime: now,
	})
	if lokiEnabled {
		l.setCondition(metav1.Condition{
			Type:               LokiRuleGroupConditionLokiSynced,
			Status:             metav1.ConditionTrue,
			Reason:             LokiRuleGroupReasonSynced,
			Message:            "Rule groups successfully synced to Loki",
			LastTransitionTime: now,
		})
	}
	if mimirEnabled {
		l.setCondition(metav1.Condition{
			Type:               LokiRuleGroupConditionMimirSynced,
			Status:             metav1.ConditionTrue,
			Reason:             LokiRuleGroupReasonSynced,
			Message:            "Rule groups successfully synced to Mimir/Prometheus",
			LastTransitionTime: now,
		})
	}
}

// SetFailedCondition marks the resource with a failure on one of the backends.
func (l *LokiRuleGroup) SetFailedCondition(conditionType, reason, message string) {
	now := metav1.Now()
	l.Status.SyncStatus = SyncStatusFailed
	l.Status.ErrorMessage = message

	l.setCondition(metav1.Condition{
		Type:               LokiRuleGroupConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
	l.setCondition(metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}

func (l *LokiRuleGroup) setCondition(newCond metav1.Condition) {
	for i, c := range l.Status.Conditions {
		if c.Type == newCond.Type {
			l.Status.Conditions[i] = newCond
			return
		}
	}
	l.Status.Conditions = append(l.Status.Conditions, newCond)
}

// +kubebuilder:object:root=true

// LokiRuleGroupList contains a list of LokiRuleGroup.
type LokiRuleGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LokiRuleGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LokiRuleGroup{}, &LokiRuleGroupList{})
}
