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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	// examine DeletionTimestamp to determine if object is under deletion
	if clientConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Register finalizer
		if !controllerutil.ContainsFinalizer(clientConfig, utils.MyFinalizerName) {
			logger.Info("Add Finalizer to ClientConfig", "name", clientConfig.Name, "namespace", clientConfig.Namespace)
			controllerutil.AddFinalizer(clientConfig, utils.MyFinalizerName)
			if err := r.Update(ctx, clientConfig); err != nil {
				logger.Error(err, "Problem adding Finalizer to client config")
				return ctrl.Result{}, err
			}
		}

		// Attempt to create and validate client connection
		spec := clientConfig.Spec
		var err error

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
				if statusErr := r.updateStatusMissingAnnotation(ctx, clientConfig); statusErr != nil {
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
			if statusErr := r.updateStatusDisconnected(ctx, clientConfig, err); statusErr != nil {
				logger.Error(statusErr, "Failed to update status")
				return ctrl.Result{}, statusErr
			}
			// Requeue to retry connection
			return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
		}

		logger.Info("Added new Client Config", "name", clientConfig.Name, "namespace", clientConfig.Namespace, "type", spec.Type)

		// Update status to connected
		if statusErr := r.updateStatusConnected(ctx, clientConfig); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
			return ctrl.Result{}, statusErr
		}
	} else {
		// The object is being deleted check for finalizer
		if controllerutil.ContainsFinalizer(clientConfig, utils.MyFinalizerName) {
			logger.Info("Removing finalizer from ClientConfig", "name", clientConfig.Name, "namespace", clientConfig.Namespace)
			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(clientConfig, utils.MyFinalizerName)
			if err := r.Update(ctx, clientConfig); err != nil {
				return ctrl.Result{}, err
			}
		}
		logger.Info("Removing client from cache", "name", clientConfig.Name, "namespace", clientConfig.Namespace)
		r.RulerClients.RemoveClient(clientConfig.Name)
	}

	return ctrl.Result{}, nil
}

// updateStatusConnected updates the ClientConfig status to indicate successful connection.
// It sets the ConnectionStatus to Connected, records the connection time, clears any error message,
// and updates the Ready condition to True. Returns an error if the status update fails.
func (r *ClientConfigReconciler) updateStatusConnected(ctx context.Context, clientConfig *openawarenessv1beta1.ClientConfig) error {
	now := metav1.Now()

	clientConfig.Status.ConnectionStatus = openawarenessv1beta1.ConnectionStatusConnected
	clientConfig.Status.LastConnectionTime = &now
	clientConfig.Status.ErrorMessage = ""

	// Update conditions
	condition := metav1.Condition{
		Type:               openawarenessv1beta1.ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: clientConfig.Generation,
		LastTransitionTime: now,
		Reason:             openawarenessv1beta1.ReasonConnected,
		Message:            "Successfully connected to endpoint",
	}

	utils.SetCondition(&clientConfig.Status.Conditions, condition)

	return r.Status().Update(ctx, clientConfig)
}

// updateStatusMissingAnnotation updates the ClientConfig status to indicate missing required annotation.
// It sets the ConnectionStatus to Disconnected, records an error message about the missing annotation,
// and updates the Ready condition to False with the MissingAnnotation reason.
// Returns an error if the status update fails.
func (r *ClientConfigReconciler) updateStatusMissingAnnotation(ctx context.Context, clientConfig *openawarenessv1beta1.ClientConfig) error {
	now := metav1.Now()

	clientConfig.Status.ConnectionStatus = openawarenessv1beta1.ConnectionStatusDisconnected
	clientConfig.Status.ErrorMessage = fmt.Sprintf("Missing required annotation '%s' for Mimir client", utils.MimirTenantAnnotation)

	// Update conditions
	condition := metav1.Condition{
		Type:               openawarenessv1beta1.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: clientConfig.Generation,
		LastTransitionTime: now,
		Reason:             openawarenessv1beta1.ReasonMissingAnnotation,
		Message:            fmt.Sprintf("Missing required annotation '%s' for Mimir client type", utils.MimirTenantAnnotation),
	}

	utils.SetCondition(&clientConfig.Status.Conditions, condition)

	return r.Status().Update(ctx, clientConfig)
}

// updateStatusDisconnected updates the ClientConfig status to indicate connection failure.
// It sets the ConnectionStatus to Disconnected, records the error message, and updates the Ready
// condition to False with an appropriate reason based on the error type (e.g., NetworkError, AuthenticationError).
// Returns an error if the status update fails.
func (r *ClientConfigReconciler) updateStatusDisconnected(ctx context.Context, clientConfig *openawarenessv1beta1.ClientConfig, err error) error {
	now := metav1.Now()

	clientConfig.Status.ConnectionStatus = openawarenessv1beta1.ConnectionStatusDisconnected
	clientConfig.Status.ErrorMessage = err.Error()

	// Determine the reason based on the error type using shared utility
	reason, message := utils.CategorizeError(err)

	// Update conditions
	condition := metav1.Condition{
		Type:               openawarenessv1beta1.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
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
