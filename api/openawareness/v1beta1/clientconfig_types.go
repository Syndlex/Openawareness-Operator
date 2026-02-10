package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClientConfigSpec defines the desired state of ClientConfig
type ClientConfigSpec struct {
	// Address is the URL of the Mimir or Prometheus instance
	// +kubebuilder:validation:Required
	Address string `json:"address,omitempty"`

	// Type specifies whether this is a Mimir or Prometheus instance
	// +kubebuilder:validation:Enum=mimir;prometheus
	// +kubebuilder:validation:Required
	Type ClientType `json:"type,omitempty"`
}

// ClientType defines the type of client (Mimir or Prometheus)
type ClientType string

const (
	// Mimir represents a Grafana Mimir client
	Mimir ClientType = "mimir"
	// Prometheus represents a Prometheus client
	Prometheus ClientType = "prometheus"
)

// ConnectionStatus represents the connection state of a ClientConfig
type ConnectionStatus string

const (
	// ConnectionStatusConnected indicates successful connection to the target
	ConnectionStatusConnected ConnectionStatus = "Connected"
	// ConnectionStatusDisconnected indicates failed or lost connection to the target
	ConnectionStatusDisconnected ConnectionStatus = "Disconnected"
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
	// +optional
	ConnectionStatus ConnectionStatus `json:"connectionStatus,omitempty"`

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
	// ReasonInvalidURL indicates the address cannot be parsed as a valid URL
	ReasonInvalidURL = "InvalidURL"
	// ReasonInvalidTLSConfig indicates the TLS configuration is invalid
	ReasonInvalidTLSConfig = "InvalidTLSConfig"
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
