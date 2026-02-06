//nolint:revive // utils is a standard package name for utilities
package utils

import (
	"context"

	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// HandleFinalizer manages the finalizer lifecycle for a Kubernetes resource.
// It handles both addition of finalizers on creation and removal on deletion,
// along with executing cleanup logic when appropriate.
//
// Parameters:
//   - ctx: The context for the operation
//   - client: The Kubernetes client for updating resources
//   - obj: The Kubernetes object to manage
//   - finalizerName: The name of the finalizer to add/remove
//   - cleanupFunc: Optional cleanup function to execute before finalizer removal (can be nil)
//
// Returns:
//   - isDeleting: true if the resource is being deleted, false otherwise
//   - error: any error that occurred during finalizer management
//
// The function follows this logic:
//  1. If resource is NOT being deleted:
//     - Add finalizer if not present
//     - Return (false, nil) to indicate normal reconciliation should continue
//  2. If resource IS being deleted:
//     - Execute cleanupFunc if provided (errors are logged but don't prevent finalizer removal)
//     - Remove finalizer if present
//     - Return (true, nil) to indicate deletion is in progress
func HandleFinalizer(ctx context.Context, client k8sClient.Client, obj k8sClient.Object,
	finalizerName string, cleanupFunc func(context.Context) error) (bool, error) {

	// Check if object is being deleted
	if obj.GetDeletionTimestamp().IsZero() {
		// Object is NOT being deleted - ensure finalizer is present
		if !controllerutil.ContainsFinalizer(obj, finalizerName) {
			controllerutil.AddFinalizer(obj, finalizerName)
			if err := client.Update(ctx, obj); err != nil {
				return false, err
			}
		}
		return false, nil
	}

	// Object IS being deleted - perform cleanup and remove finalizer
	if controllerutil.ContainsFinalizer(obj, finalizerName) {
		// Execute cleanup function if provided
		if cleanupFunc != nil {
			if err := cleanupFunc(ctx); err != nil {
				// Log error but continue with finalizer removal
				// This prevents resources from being stuck if cleanup fails
				// The error is returned to the caller for logging
				return true, err
			}
		}

		// Remove finalizer
		controllerutil.RemoveFinalizer(obj, finalizerName)
		if err := client.Update(ctx, obj); err != nil {
			return true, err
		}
	}

	return true, nil
}
