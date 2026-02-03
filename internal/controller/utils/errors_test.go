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

//nolint:revive // utils is a standard package name for utilities
package utils

import (
	"context"
	"errors"
	"testing"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedReason string
		expectedMsg    string
	}{
		{
			name:           "nil error",
			err:            nil,
			expectedReason: openawarenessv1beta1.ReasonSynced,
			expectedMsg:    "Operation successful",
		},
		{
			name:           "context deadline exceeded",
			err:            context.DeadlineExceeded,
			expectedReason: openawarenessv1beta1.ReasonTimeoutError,
			expectedMsg:    "Operation deadline exceeded",
		},
		{
			name:           "DNS resolution error - no such host",
			err:            errors.New("dial tcp: lookup example.com: no such host"),
			expectedReason: openawarenessv1beta1.ReasonDNSResolutionError,
			expectedMsg:    "DNS resolution failed",
		},
		{
			name:           "DNS resolution error - dns keyword",
			err:            errors.New("dns lookup failed"),
			expectedReason: openawarenessv1beta1.ReasonDNSResolutionError,
			expectedMsg:    "DNS resolution failed",
		},
		{
			name:           "timeout error - timeout keyword",
			err:            errors.New("connection timeout"),
			expectedReason: openawarenessv1beta1.ReasonTimeoutError,
			expectedMsg:    "Connection timeout",
		},
		{
			name:           "timeout error - i/o timeout",
			err:            errors.New("i/o timeout"),
			expectedReason: openawarenessv1beta1.ReasonTimeoutError,
			expectedMsg:    "Connection timeout",
		},
		{
			name:           "connection refused",
			err:            errors.New("dial tcp 127.0.0.1:8080: connect: connection refused"),
			expectedReason: openawarenessv1beta1.ReasonNetworkError,
			expectedMsg:    "Network connection error",
		},
		{
			name:           "connection reset",
			err:            errors.New("read tcp: connection reset by peer"),
			expectedReason: openawarenessv1beta1.ReasonNetworkError,
			expectedMsg:    "Network connection error",
		},
		{
			name:           "invalid URL - missing protocol",
			err:            errors.New("parse \"example.com\": invalid URI for request: missing protocol scheme"),
			expectedReason: openawarenessv1beta1.ReasonInvalidURL,
			expectedMsg:    "Invalid URL format",
		},
		{
			name:           "TLS error - certificate",
			err:            errors.New("x509: certificate signed by unknown authority"),
			expectedReason: openawarenessv1beta1.ReasonInvalidTLSConfig,
			expectedMsg:    "TLS configuration error",
		},
		{
			name:           "HTTP 401 unauthorized",
			err:            errors.New("server returned HTTP status: 401 Unauthorized"),
			expectedReason: openawarenessv1beta1.ReasonUnauthorized,
			expectedMsg:    "Authentication failed",
		},
		{
			name:           "HTTP 403 forbidden",
			err:            errors.New("server returned HTTP status: 403 Forbidden"),
			expectedReason: openawarenessv1beta1.ReasonForbidden,
			expectedMsg:    "Access forbidden",
		},
		{
			name:           "HTTP 404 not found",
			err:            errors.New("server returned HTTP status: 404 Not Found"),
			expectedReason: openawarenessv1beta1.ReasonNotFound,
			expectedMsg:    "Endpoint not found",
		},
		{
			name:           "HTTP 409 conflict",
			err:            errors.New("server returned HTTP status: 409 Conflict"),
			expectedReason: openawarenessv1beta1.ReasonConflict,
			expectedMsg:    "Resource conflict",
		},
		{
			name:           "HTTP 429 too many requests",
			err:            errors.New("server returned HTTP status: 429 Too Many Requests"),
			expectedReason: openawarenessv1beta1.ReasonTooManyRequests,
			expectedMsg:    "Rate limit exceeded",
		},
		{
			name:           "HTTP 500 server error",
			err:            errors.New("server returned HTTP status: 500 Internal Server Error"),
			expectedReason: openawarenessv1beta1.ReasonServerError,
			expectedMsg:    "Server error",
		},
		{
			name:           "HTTP 503 service unavailable",
			err:            errors.New("server returned HTTP status: 503 Service Unavailable"),
			expectedReason: openawarenessv1beta1.ReasonServerError,
			expectedMsg:    "Server error",
		},
		{
			name:           "unknown error defaults to network error",
			err:            errors.New("something went wrong"),
			expectedReason: openawarenessv1beta1.ReasonNetworkError,
			expectedMsg:    "Connection failed: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, msg := CategorizeError(tt.err)
			if reason != tt.expectedReason {
				t.Errorf("CategorizeError() reason = %v, want %v", reason, tt.expectedReason)
			}
			if msg != tt.expectedMsg {
				t.Errorf("CategorizeError() message = %v, want %v", msg, tt.expectedMsg)
			}
		})
	}
}

func TestSetCondition(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name               string
		existingConditions []metav1.Condition
		newCondition       metav1.Condition
		expectedLength     int
		expectedStatus     metav1.ConditionStatus
	}{
		{
			name:               "add condition to empty list",
			existingConditions: []metav1.Condition{},
			newCondition: metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "Success",
				Message:            "All systems operational",
			},
			expectedLength: 1,
			expectedStatus: metav1.ConditionTrue,
		},
		{
			name: "update existing condition",
			existingConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "Error",
					Message:            "System error",
				},
			},
			newCondition: metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "Success",
				Message:            "All systems operational",
			},
			expectedLength: 1,
			expectedStatus: metav1.ConditionTrue,
		},
		{
			name: "add different condition type",
			existingConditions: []metav1.Condition{
				{
					Type:               "Ready",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "Success",
					Message:            "Ready",
				},
			},
			newCondition: metav1.Condition{
				Type:               "Synced",
				Status:             metav1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "Success",
				Message:            "Synced",
			},
			expectedLength: 2,
			expectedStatus: metav1.ConditionTrue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conditions := tt.existingConditions
			SetCondition(&conditions, tt.newCondition)

			if len(conditions) != tt.expectedLength {
				t.Errorf("SetCondition() resulted in %d conditions, want %d", len(conditions), tt.expectedLength)
			}

			// Find the condition that was added/updated
			var found bool
			for _, c := range conditions {
				if c.Type == tt.newCondition.Type {
					found = true
					if c.Status != tt.expectedStatus {
						t.Errorf("Condition status = %v, want %v", c.Status, tt.expectedStatus)
					}
					if c.Reason != tt.newCondition.Reason {
						t.Errorf("Condition reason = %v, want %v", c.Reason, tt.newCondition.Reason)
					}
				}
			}

			if !found {
				t.Errorf("Condition type %s not found in conditions list", tt.newCondition.Type)
			}
		})
	}
}

func TestSetConditionNilList(t *testing.T) {
	var conditions *[]metav1.Condition
	now := metav1.Now()

	newCondition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             "Success",
		Message:            "All systems operational",
	}

	// Should handle nil pointer gracefully without panic
	SetCondition(conditions, newCondition)
}
