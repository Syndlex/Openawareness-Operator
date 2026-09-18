// Package utils provides common utilities for the openawareness controller
//
//nolint:revive // utils is a standard package name for utilities
package utils

const (
	// FinalizerAnnotation is the finalizer used for all openawareness resources
	FinalizerAnnotation string = "openawareness.io/finalizers"

	// Mimir annotations (existing)

	// ClientNameAnnotation references the ClientConfig for Mimir API access
	ClientNameAnnotation string = "openawareness.io/client-name"
	// MimirTenantAnnotation specifies the Mimir tenant for rules and alerts
	MimirTenantAnnotation string = "openawareness.io/mimir-tenant"
	// DefaultTenantID is the default tenant used when no tenant annotation is specified
	DefaultTenantID string = "anonymous"

	// ActiveClientRefAnnotation is written by the controller onto a PrometheusRule
	// after each successful sync, recording which ClientConfig was last used.
	// On deletion, this annotation is used to clean up rules even if the
	// ClientConfig has already been deleted.
	ActiveClientRefAnnotation string = "openawareness.io/active-client-ref"
)
