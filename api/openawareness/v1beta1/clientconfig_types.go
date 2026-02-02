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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClientConfigSpec defines the desired state of ClientConfig
type ClientConfigSpec struct {
	Address string `json:"address,omitempty"`

	Type ClientType `json:"type,omitempty"`
}

type ClientType string

const (
	Mimir      ClientType = "mimir"
	Prometheus ClientType = "prometheus"
)

// ClientConfigStatus defines the observed state of ClientConfig
type ClientConfigStatus struct {
	// Conditions represent the latest available observations of the ClientConfig's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastConnectionTime is the timestamp of the last successful connection attempt
	// +optional
	LastConnectionTime *metav1.Time `json:"lastConnectionTime,omitempty"`

	// ConnectionStatus indicates whether the client can connect to Mimir/Prometheus
	// Possible values: "Connected", "Disconnected", "Unknown"
	// +optional
	ConnectionStatus string `json:"connectionStatus,omitempty"`

	// ErrorMessage contains the last error message if connection failed
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// Condition types for ClientConfig
const (
	// ConditionTypeReady indicates whether the ClientConfig is ready to use
	ConditionTypeReady = "Ready"
)

// Condition reasons for ClientConfig
const (
	// ReasonConfigured indicates the ClientConfig is properly configured
	ReasonConfigured = "Configured"
	// ReasonInvalidURL indicates the address cannot be parsed as a valid URL
	ReasonInvalidURL = "InvalidURL"
	// ReasonInvalidTLSConfig indicates the TLS configuration is invalid
	ReasonInvalidTLSConfig = "InvalidTLSConfig"
	// ReasonAuthConflict indicates both basic auth and token are configured
	ReasonAuthConflict = "AuthConflict"
	// ReasonNetworkError indicates a network connectivity error
	ReasonNetworkError = "NetworkError"
	// ReasonTimeoutError indicates the connection timed out
	ReasonTimeoutError = "TimeoutError"
	// ReasonDNSResolutionError indicates DNS resolution failed
	ReasonDNSResolutionError = "DNSResolutionError"
	// ReasonUnauthorized indicates invalid credentials (401)
	ReasonUnauthorized = "Unauthorized"
	// ReasonForbidden indicates insufficient permissions (403)
	ReasonForbidden = "Forbidden"
	// ReasonNotFound indicates the endpoint was not found (404)
	ReasonNotFound = "NotFound"
	// ReasonTooManyRequests indicates rate limiting (429)
	ReasonTooManyRequests = "TooManyRequests"
	// ReasonServerError indicates a server-side error (5xx)
	ReasonServerError = "ServerError"
	// ReasonConnected indicates successful connection
	ReasonConnected = "Connected"
	// ReasonMissingAnnotation indicates a required annotation is missing
	ReasonMissingAnnotation = "MissingAnnotation"
)

// Connection status values
const (
	ConnectionStatusConnected    = "Connected"
	ConnectionStatusDisconnected = "Disconnected"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ClientConfig is the Schema for the clientconfigs API
type ClientConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClientConfigSpec   `json:"spec,omitempty"`
	Status ClientConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClientConfigList contains a list of ClientConfig
type ClientConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClientConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClientConfig{}, &ClientConfigList{})
}
