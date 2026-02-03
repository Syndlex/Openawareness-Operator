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

package openawareness

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
)

// MimirAlertTenantReconciler reconciles a MimirAlertTenant object
type MimirAlertTenantReconciler struct {
	k8sClient.Client
	RulerClients clients.RulerClientCacheInterface
	Scheme       *runtime.Scheme
}

//nolint:lll
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=mimiralerttenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=mimiralerttenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=mimiralerttenants/finalizers,verbs=update

// Reconcile reconciles the MimirAlertTenant resource by syncing Alertmanager configurations
// to the configured Mimir instance. It handles the full lifecycle including creation,
// updates, and deletion of Alertmanager configurations with proper finalizer management.
//
// The reconciliation process:
// 1. Fetches the MimirAlertTenant resource
// 2. Adds finalizer for cleanup on deletion
// 3. Retrieves the Mimir client from annotations
// 4. Validates the Alertmanager configuration
// 5. Pushes configuration to Mimir API
// 6. Updates status to reflect sync state
// 7. On deletion, removes configuration from Mimir and cleans up finalizer
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *MimirAlertTenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rule := &openawarenessv1beta1.MimirAlertTenant{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		return ctrl.Result{}, k8sClient.IgnoreNotFound(err)
	}
	logger.Info("Found MimirAlertTenant", "name", rule.Name, "namespace", rule.Namespace)

	if rule.DeletionTimestamp.IsZero() {
		// Register finalizer first, before checking for client
		if !controllerutil.ContainsFinalizer(rule, utils.FinalizerAnnotation) {
			controllerutil.AddFinalizer(rule, utils.FinalizerAnnotation)
			if err := r.Update(ctx, rule); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Get the alertmanager client
		alertManagerClient, err := r.clientFromCrd(ctx, logger, rule)
		if err != nil {
			logger.Error(err, "Failed to get Alertmanager client",
				"name", rule.Name,
				"namespace", rule.Namespace)
			// Return error to trigger retry
			return ctrl.Result{}, err
		}

		// Validate the Alertmanager configuration before sending to Mimir
		if err := rule.ValidateAlertmanagerConfig(); err != nil {
			logger.Error(err, "Invalid Alertmanager configuration",
				"name", rule.Name,
				"namespace", rule.Namespace)
			rule.SetConfigInvalidCondition(openawarenessv1beta1.ReasonInvalidYAML, err.Error())
			if updateErr := r.Status().Update(ctx, rule); updateErr != nil {
				logger.Error(updateErr, "Failed to update status")
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}

		cfg := rule.ToConfigDTO()
		templates := rule.ToTemplatesDTO()

		err = alertManagerClient.CreateAlertmanagerConfig(ctx, cfg, templates)
		if err != nil {
			logger.Error(err, "Failed to create Alertmanager configuration",
				"name", rule.Name,
				"namespace", rule.Namespace)

			// Categorize the error and set appropriate status using shared utility
			reason, _ := utils.CategorizeError(err)
			rule.SetFailedCondition(reason, err.Error())
			if updateErr := r.Status().Update(ctx, rule); updateErr != nil {
				logger.Error(updateErr, "Failed to update status")
			}
			return ctrl.Result{}, err
		}

		logger.Info("Successfully created Alertmanager configuration",
			"name", rule.Name,
			"namespace", rule.Namespace)

		// Update status to reflect successful sync
		rule.SetSyncedCondition()
		if err := r.Status().Update(ctx, rule); err != nil {
			logger.Error(err, "Failed to update status after successful sync")
			return ctrl.Result{}, err
		}

	} else {
		// The object is being deleted
		// Get the alertmanager client for cleanup
		alertManagerClient, err := r.clientFromCrd(ctx, logger, rule)
		if err != nil {
			logger.Error(err, "Failed to get Alertmanager client for deletion - configuration may be orphaned in Mimir",
				"name", rule.Name,
				"namespace", rule.Namespace,
				"warning", "Unable to cleanup Alertmanager configuration from Mimir API")
			// If we can't get the client, we still need to remove the finalizer
			// to allow deletion to proceed. This may leave orphaned configuration in Mimir.
			// Operators should manually clean up if needed.
			if controllerutil.ContainsFinalizer(rule, utils.FinalizerAnnotation) {
				controllerutil.RemoveFinalizer(rule, utils.FinalizerAnnotation)
				if err := r.Update(ctx, rule); err != nil {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, nil
		}

		err = alertManagerClient.DeleteAlermanagerConfig(ctx)
		if err != nil {
			logger.Error(err, "Failed to delete Alertmanager configuration - configuration may be orphaned in Mimir",
				"name", rule.Name,
				"namespace", rule.Namespace,
				"warning", "Alertmanager configuration may still exist in Mimir API")
			// Continue with finalizer removal even if deletion fails to prevent resource from being stuck.
			// This may leave orphaned configuration in Mimir. Operators should manually clean up if needed.
		} else {
			logger.Info("Successfully deleted Alertmanager configuration from Mimir",
				"name", rule.Name,
				"namespace", rule.Namespace)
		}

		// Remove finalizer
		if controllerutil.ContainsFinalizer(rule, utils.FinalizerAnnotation) {
			controllerutil.RemoveFinalizer(rule, utils.FinalizerAnnotation)
			if err := r.Update(ctx, rule); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("MimirAlertTenant was deleted",
				"name", rule.Name,
				"namespace", rule.Namespace)
		}
	}
	return ctrl.Result{}, nil

}

// clientFromCrd retrieves the appropriate Mimir client for the given MimirAlertTenant.
// It extracts the client name and tenant ID from the resource's annotations,
// fetches the ClientConfig, and returns a tenant-specific Mimir client.
// Returns an error if annotations are missing or if the client cannot be created.
func (r *MimirAlertTenantReconciler) clientFromCrd(
	ctx context.Context,
	logger logr.Logger,
	rule *openawarenessv1beta1.MimirAlertTenant,
) (clients.AwarenessClient, error) {
	if r.RulerClients == nil {
		logger.Info("RulerClients cache is not initialized")
		return nil, fmt.Errorf("ruler clients cache is nil for MimirAlertTenant %s/%s", rule.Namespace, rule.Name)
	}

	// Extract and validate required annotations
	annotations, err := utils.GetRequiredAnnotations(rule, utils.ClientNameAnnotation, utils.MimirTenantAnnotation)
	if err != nil {
		logger.Info("MimirAlertTenant is missing required annotations", "name", rule.Name, "error", err.Error())
		return nil, err
	}

	clientName := annotations[utils.ClientNameAnnotation]
	tenantID := annotations[utils.MimirTenantAnnotation]

	// Get the ClientConfig to retrieve the Mimir address
	clientConfig := &openawarenessv1beta1.ClientConfig{}
	if err := r.Get(ctx, k8sClient.ObjectKey{
		Name:      clientName,
		Namespace: rule.Namespace,
	}, clientConfig); err != nil {
		logger.Error(err, "Failed to get ClientConfig", "clientName", clientName)
		return nil, fmt.Errorf("getting ClientConfig %s: %w", clientName, err)
	}

	// Get or create a client specific to this tenant
	alertManagerClient, err := r.RulerClients.GetOrCreateMimirClient(
		ctx,
		clientConfig.Spec.Address,
		clientName,
		tenantID,
	)
	if err != nil {
		logger.Error(err, "Failed to get or create Mimir client",
			"clientName", clientName,
			"tenantID", tenantID,
			"address", clientConfig.Spec.Address)
		return nil, err
	}

	logger.Info("Got Mimir client for tenant",
		"clientName", clientName,
		"tenantID", tenantID,
		"address", clientConfig.Spec.Address)

	return alertManagerClient, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MimirAlertTenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openawarenessv1beta1.MimirAlertTenant{}).
		Complete(r)
}
