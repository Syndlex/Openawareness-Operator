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
	"errors"

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
	RulerClients *clients.RulerClientCache
	Scheme       *runtime.Scheme
}

// +kubebuilder:rbac:groups=openawareness.syndlex,resources=mimiralerttenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=mimiralerttenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=mimiralerttenants/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MimirAlertTenant object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *MimirAlertTenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rule := &openawarenessv1beta1.MimirAlertTenant{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		return ctrl.Result{}, k8sClient.IgnoreNotFound(err)
	}
	logger.Info("Found Rule", "Name", rule.Name)

	alertManagerClient, err := r.clientFromCrd(logger, rule)
	if err != nil {

		logger.V(1).Info("No Alert manger Client found Please create a new "+openawarenessv1beta1.GroupVersion.Group+" ClientConfig", "Name", rule.Name)
		return ctrl.Result{}, nil
	}

	if rule.ObjectMeta.DeletionTimestamp.IsZero() {
		// Register finalizer
		if !controllerutil.ContainsFinalizer(rule, utils.MyFinalizerName) {
			controllerutil.AddFinalizer(rule, utils.MyFinalizerName)
			if err := r.Update(ctx, rule); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Validate the Alertmanager configuration before sending to Mimir
		if err := rule.ValidateAlertmanagerConfig(); err != nil {
			logger.Error(err, "Invalid Alertmanager configuration",
				"name", rule.Name,
				"namespace", rule.Namespace)
			return ctrl.Result{}, err
		}

		cfg := rule.ToConfigDTO()
		templates := rule.ToTemplatesDTO()

		err = alertManagerClient.CreateAlertmanagerConfig(ctx, cfg, templates)
		if err != nil {
			logger.Error(err, "Failed to create Alertmanager configuration",
				"name", rule.Name,
				"namespace", rule.Namespace)
			return ctrl.Result{}, err
		}

		logger.Info("Successfully created Alertmanager configuration",
			"name", rule.Name,
			"namespace", rule.Namespace)

	} else {
		err := alertManagerClient.DeleteAlermanagerConfig(ctx)
		if err != nil {
			logger.Error(err, "Failed to delete Alertmanager configuration",
				"name", rule.Name,
				"namespace", rule.Namespace)
			return ctrl.Result{}, err
		}

		// The object is being deleted check for finalizer
		if controllerutil.ContainsFinalizer(rule, utils.MyFinalizerName) {
			controllerutil.RemoveFinalizer(rule, utils.MyFinalizerName)
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

func (r *MimirAlertTenantReconciler) clientFromCrd(logger logr.Logger, rule *openawarenessv1beta1.MimirAlertTenant) (clients.AwarenessClient, error) {
	if r.RulerClients == nil {
		logger.Info("RulerClients cache is not initialized")
		return nil, errors.New("ruler clients cache is nil")
	}

	if rule.Annotations == nil {
		logger.Info("MimirAlertTenant is missing required annotations", "name", rule.Name)
		return nil, errors.New("annotations are missing")
	}

	clientName := rule.Annotations[utils.ClientNameAnnotation]
	if clientName == "" {
		logger.Info("MimirAlertTenant is missing '"+utils.ClientNameAnnotation+"' annotation", "name", rule.Name)
		return nil, errors.New("client-name annotation is empty")
	}

	alertManagerClient, err := r.RulerClients.GetClient(clientName)
	if err != nil {
		logger.Info("Client does not exist", "clientName", clientName, "alertTenantName", rule.Name)
		return nil, err
	}
	return alertManagerClient, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MimirAlertTenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openawarenessv1beta1.MimirAlertTenant{}).
		Complete(r)
}
