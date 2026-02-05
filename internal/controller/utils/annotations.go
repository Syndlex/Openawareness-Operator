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
