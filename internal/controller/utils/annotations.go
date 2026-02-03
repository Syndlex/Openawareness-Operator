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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetRequiredAnnotations extracts and validates required annotations from a Kubernetes object.
// It checks if all specified annotation keys are present and non-empty.
//
// Parameters:
//   - obj: The Kubernetes object to extract annotations from
//   - annotationKeys: Variable number of annotation keys that must be present
//
// Returns:
//   - A map of annotation key to value for all requested annotations
//   - An error if any annotation is missing or empty
//
// Example usage:
//
//	annotations, err := GetRequiredAnnotations(resource,
//	    "openawareness.io/client-name",
//	    "openawareness.io/mimir-tenant")
func GetRequiredAnnotations(obj metav1.Object, annotationKeys ...string) (map[string]string, error) {
	if obj.GetAnnotations() == nil {
		return nil, fmt.Errorf("resource %s/%s has no annotations", obj.GetNamespace(), obj.GetName())
	}

	result := make(map[string]string, len(annotationKeys))
	annotations := obj.GetAnnotations()

	for _, key := range annotationKeys {
		value, exists := annotations[key]
		if !exists || value == "" {
			return nil, fmt.Errorf("required annotation '%s' is missing or empty for %s/%s",
				key, obj.GetNamespace(), obj.GetName())
		}
		result[key] = value
	}

	return result, nil
}
