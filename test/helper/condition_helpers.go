package helper

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FindCondition searches for a condition by type in a list of conditions.
// Returns the condition if found, or nil if not found.
func FindCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
