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

	errMsg := err.Error()

	// Check for context timeout/deadline errors first (these are Go errors, not string-based)
	if errors.Is(err, context.DeadlineExceeded) {
		return openawarenessv1beta1.ReasonTimeoutError, "Operation deadline exceeded"
	}

	// Check for DNS errors (highest priority for network errors)
	if strings.Contains(errMsg, "no such host") || strings.Contains(errMsg, "dns") {
		return openawarenessv1beta1.ReasonDNSResolutionError, "DNS resolution failed"
	}

	// Check for timeout errors
	if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "deadline exceeded") || strings.Contains(errMsg, "i/o timeout") {
		return openawarenessv1beta1.ReasonTimeoutError, "Connection timeout"
	}

	// Check for connection refused or network errors
	if strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "network") || strings.Contains(errMsg, "dial tcp") ||
		strings.Contains(errMsg, "dial udp") {
		return openawarenessv1beta1.ReasonNetworkError, "Network connection error"
	}

	// Check for URL parsing errors (check after network errors since parse errors might mention "dial")
	if strings.Contains(errMsg, "missing protocol scheme") || strings.Contains(errMsg, "invalid URL") ||
		strings.Contains(errMsg, "unsupported protocol") {
		return openawarenessv1beta1.ReasonInvalidURL, "Invalid URL format"
	}

	// Check for TLS errors
	if strings.Contains(errMsg, "tls") || strings.Contains(errMsg, "certificate") || strings.Contains(errMsg, "x509") {
		return openawarenessv1beta1.ReasonInvalidTLSConfig, "TLS configuration error"
	}

	// Check for HTTP status code errors
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

	// Default to network error for unknown errors
	return openawarenessv1beta1.ReasonNetworkError, fmt.Sprintf("Connection failed: %s", errMsg)
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
