//nolint:revive // utils is a standard package name for utilities
package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CategorizeError determines the appropriate reason and message for an error.
// It analyzes the error message and returns a standardized reason code and human-readable message.
// This function is used by both ClientConfig and MimirAlertTenant controllers for consistent error handling.
func CategorizeError(err error) (string, string) {
	if err == nil {
		return openawarenessv1beta1.ReasonSynced, "Operation successful"
	}

	// Check for context timeout/deadline errors first
	if errors.Is(err, context.DeadlineExceeded) {
		return openawarenessv1beta1.ReasonTimeoutError, "Operation deadline exceeded"
	}

	errMsg := err.Error()

	// Check error categories in priority order
	if reason, msg := checkDNSError(errMsg); reason != "" {
		return reason, msg
	}
	if reason, msg := checkTimeoutError(errMsg); reason != "" {
		return reason, msg
	}
	if reason, msg := checkNetworkError(errMsg); reason != "" {
		return reason, msg
	}
	if reason, msg := checkURLError(errMsg); reason != "" {
		return reason, msg
	}
	if reason, msg := checkTLSError(errMsg); reason != "" {
		return reason, msg
	}
	if reason, msg := checkHTTPError(errMsg); reason != "" {
		return reason, msg
	}

	// Default to network error for unknown errors
	return openawarenessv1beta1.ReasonNetworkError, fmt.Sprintf("Connection failed: %s", errMsg)
}

func checkDNSError(errMsg string) (string, string) {
	if strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "dns") {
		return openawarenessv1beta1.ReasonDNSResolutionError, "DNS resolution failed"
	}
	return "", ""
}

func checkTimeoutError(errMsg string) (string, string) {
	if strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "deadline exceeded") ||
		strings.Contains(errMsg, "i/o timeout") {
		return openawarenessv1beta1.ReasonTimeoutError, "Connection timeout"
	}
	return "", ""
}

func checkNetworkError(errMsg string) (string, string) {
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "network") ||
		strings.Contains(errMsg, "dial tcp") ||
		strings.Contains(errMsg, "dial udp") {
		return openawarenessv1beta1.ReasonNetworkError, "Network connection error"
	}
	return "", ""
}

func checkURLError(errMsg string) (string, string) {
	if strings.Contains(errMsg, "missing protocol scheme") ||
		strings.Contains(errMsg, "invalid URL") ||
		strings.Contains(errMsg, "unsupported protocol") {
		return openawarenessv1beta1.ReasonInvalidURL, "Invalid URL format"
	}
	return "", ""
}

func checkTLSError(errMsg string) (string, string) {
	if strings.Contains(errMsg, "tls") ||
		strings.Contains(errMsg, "certificate") ||
		strings.Contains(errMsg, "x509") {
		return openawarenessv1beta1.ReasonInvalidTLSConfig, "TLS configuration error"
	}
	return "", ""
}

func checkHTTPError(errMsg string) (string, string) {
	if strings.Contains(errMsg, "401") || strings.Contains(errMsg, "unauthorized") {
		return openawarenessv1beta1.ReasonUnauthorized, "Authentication failed"
	}
	if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "forbidden") {
		return openawarenessv1beta1.ReasonForbidden, "Access forbidden"
	}
	if strings.Contains(errMsg, "404") || strings.Contains(errMsg, "not found") {
		return openawarenessv1beta1.ReasonNotFound, "Endpoint not found"
	}
	if strings.Contains(errMsg, "409") || strings.Contains(errMsg, "conflict") {
		return openawarenessv1beta1.ReasonConflict, "Resource conflict"
	}
	if strings.Contains(errMsg, "429") || strings.Contains(errMsg, "too many requests") {
		return openawarenessv1beta1.ReasonTooManyRequests, "Rate limit exceeded"
	}
	if strings.Contains(errMsg, "500") || strings.Contains(errMsg, "502") ||
		strings.Contains(errMsg, "503") || strings.Contains(errMsg, "504") ||
		strings.Contains(errMsg, "server error") {
		return openawarenessv1beta1.ReasonServerError, "Server error"
	}
	return "", ""
}

// SetCondition sets or updates a condition in the conditions list.
// If a condition with the same type already exists, it updates it; otherwise, it appends the new condition.
// This ensures that each condition type appears only once in the list.
// Note: conditions must be a non-nil pointer to a slice.
func SetCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	if conditions == nil {
		return
	}

	if *conditions == nil {
		*conditions = []metav1.Condition{}
	}

	// Find existing condition of the same type
	for i, condition := range *conditions {
		if condition.Type == newCondition.Type {
			// Update existing condition
			(*conditions)[i] = newCondition
			return
		}
	}

	// Condition doesn't exist, append it
	*conditions = append(*conditions, newCondition)
}
