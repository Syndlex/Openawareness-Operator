// Package utils provides common utilities for the openawareness controller
//
//nolint:revive // utils is a standard package name for utilities
package utils

const (
	// FinalizerAnnotation is the finalizer used for all openawareness resources
	FinalizerAnnotation string = "openawareness.io/finalizers"
	// ClientNameAnnotation references the ClientConfig to use for API access
	ClientNameAnnotation string = "openawareness.io/client-name"
	// MimirTenantAnnotation specifies the Mimir tenant for rules and alerts
	MimirTenantAnnotation string = "openawareness.io/mimir-tenant"
	// DefaultTenantID is the default tenant used when no tenant is specified
	DefaultTenantID string = "anonymous"
)
