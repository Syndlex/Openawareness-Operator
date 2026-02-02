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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
)

// ClientConfigReconciler reconciles a ClientConfig object
type ClientConfigReconciler struct {
	k8sClient.Client
	RulerClients clients.RulerClientCacheInterface
	Scheme       *runtime.Scheme
}

// +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the ClientConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *ClientConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	clientConfig := &openawarenessv1beta1.ClientConfig{}
	if err := r.Get(ctx, req.NamespacedName, clientConfig); err != nil {
		logger.Info("unable to get clientConfig")
		return ctrl.Result{}, k8sClient.IgnoreNotFound(err)
	}

	logger.Info("Found new Client Config", "name", clientConfig.Name, "namespace", clientConfig.Namespace)

	// Handle finalizer lifecycle
	isDeleting, err := utils.HandleFinalizer(ctx, r.Client, clientConfig, utils.MyFinalizerName, func(ctx context.Context) error {
		// Cleanup: remove client from cache
		logger.Info("Removing client from cache", "name", clientConfig.Name, "namespace", clientConfig.Namespace)
		r.RulerClients.RemoveClient(clientConfig.Name)
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to handle finalizer", "name", clientConfig.Name, "namespace", clientConfig.Namespace)
		return ctrl.Result{}, err
	}

	// If resource is being deleted, finalizer has been handled, return early
	if isDeleting {
		return ctrl.Result{}, nil
	}

	// Normal reconciliation: resource is NOT being deleted
	{
		// Attempt to create and validate client connection
		spec := clientConfig.Spec
		var err error

		// Check if client already exists in cache - if so, remove it first to ensure refresh
		// This handles cases where the client type, address, or tenant has changed
		if existingClient, getErr := r.RulerClients.GetClient(clientConfig.Name); getErr == nil && existingClient != nil {
			logger.Info("Existing client found in cache, removing for refresh",
				"name", clientConfig.Name,
				"namespace", clientConfig.Namespace,
				"type", spec.Type)
			r.RulerClients.RemoveClient(clientConfig.Name)
		}

		switch spec.Type {
		case openawarenessv1beta1.Mimir:
			// Extract tenant ID from annotation
			tenantID := ""
			if clientConfig.Annotations != nil {
				tenantID = clientConfig.Annotations[utils.MimirTenantAnnotation]
			}

			// Check if tenant annotation is missing for Mimir client
			if tenantID == "" {
				logger.Info("Mimir ClientConfig is missing required tenant annotation",
					"annotation", utils.MimirTenantAnnotation,
					"name", clientConfig.Name)

				// Update status with specific reason for missing annotation
				message := fmt.Sprintf("Missing required annotation '%s' for Mimir client type", utils.MimirTenantAnnotation)
				if statusErr := r.updateStatus(ctx, clientConfig,
					openawarenessv1beta1.ConnectionStatusDisconnected,
					metav1.ConditionFalse,
					openawarenessv1beta1.ReasonMissingAnnotation,
					message,
					nil); statusErr != nil {
					logger.Error(statusErr, "Failed to update status")
					return ctrl.Result{}, statusErr
				}
				// Requeue to check again in case annotation is added
				return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
			}

			err = r.RulerClients.AddMimirClient(spec.Address, clientConfig.Name, tenantID, ctx)
		case openawarenessv1beta1.Prometheus:
			err = r.RulerClients.AddPromClient(spec.Address, clientConfig.Name, ctx)
		}

		// Update status based on connection result
		if err != nil {
			logger.Error(err, "Failed to add client", "name", clientConfig.Name, "namespace", clientConfig.Namespace, "type", spec.Type)
			reason, message := utils.CategorizeError(err)
			if statusErr := r.updateStatus(ctx, clientConfig,
				openawarenessv1beta1.ConnectionStatusDisconnected,
				metav1.ConditionFalse,
				reason,
				message,
				err); statusErr != nil {
				logger.Error(statusErr, "Failed to update status")
				return ctrl.Result{}, statusErr
			}
			// Requeue to retry connection
			return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
		}

		logger.Info("Added new Client Config", "name", clientConfig.Name, "namespace", clientConfig.Namespace, "type", spec.Type)

		// Update status to connected
		if statusErr := r.updateStatus(ctx, clientConfig,
			openawarenessv1beta1.ConnectionStatusConnected,
			metav1.ConditionTrue,
			openawarenessv1beta1.ReasonConnected,
			"Successfully connected to endpoint",
			nil); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
			return ctrl.Result{}, statusErr
		}
	} // End of normal reconciliation scope

	return ctrl.Result{}, nil
}

// updateStatus updates the ClientConfig status with the given connection state and condition.
// It consolidates all status update logic into a single method to reduce code duplication
// and ensure consistent status handling across all reconciliation paths.
func (r *ClientConfigReconciler) updateStatus(ctx context.Context,
	clientConfig *openawarenessv1beta1.ClientConfig,
	connectionStatus openawarenessv1beta1.ConnectionStatus,
	conditionStatus metav1.ConditionStatus,
	reason, message string,
	err error) error {

	now := metav1.Now()

	clientConfig.Status.ConnectionStatus = connectionStatus
	if err != nil {
		clientConfig.Status.ErrorMessage = err.Error()
	} else {
		clientConfig.Status.ErrorMessage = ""
	}

	if connectionStatus == openawarenessv1beta1.ConnectionStatusConnected {
		clientConfig.Status.LastConnectionTime = &now
	}

	condition := metav1.Condition{
		Type:               openawarenessv1beta1.ConditionTypeReady,
		Status:             conditionStatus,
		ObservedGeneration: clientConfig.Generation,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	utils.SetCondition(&clientConfig.Status.Conditions, condition)

	return r.Status().Update(ctx, clientConfig)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClientConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openawarenessv1beta1.ClientConfig{}).
		Complete(r)
}
