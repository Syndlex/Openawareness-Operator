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
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
)

// ClientConfigReconciler reconciles a ClientConfig object
type ClientConfigReconciler struct {
	k8sClient.Client
	RulerClients *clients.RulerClientCache
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

	logger.Info("Found new Client Config", "Name", clientConfig.Name)

	// examine DeletionTimestamp to determine if object is under deletion
	if clientConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Register finalizer
		if !controllerutil.ContainsFinalizer(clientConfig, utils.MyFinalizerName) {
			logger.Info("Add Finalizer to ClientConfig")
			controllerutil.AddFinalizer(clientConfig, utils.MyFinalizerName)
			if err := r.Update(ctx, clientConfig); err != nil {
				logger.Error(err, "Problem adding Finalizer to client config")
				return ctrl.Result{}, err
			}
		}
		spec := clientConfig.Spec
		switch spec.Type {
		case openawarenessv1beta1.Mimir:
			r.RulerClients.AddMimirClient(spec.Address, clientConfig.Name, ctx)
		case openawarenessv1beta1.Prometheus:
			r.RulerClients.AddPromClient(spec.Address, clientConfig.Name, ctx)
		}
		logger.Info("Added new Client Config", "Name", clientConfig.Name)
	} else {
		// The object is being deleted check for finalizer
		if controllerutil.ContainsFinalizer(clientConfig, utils.MyFinalizerName) {
			logger.Info("Removing finalizer from ClientConfig")
			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(clientConfig, utils.MyFinalizerName)
			if err := r.Update(ctx, clientConfig); err != nil {
				return ctrl.Result{}, err
			}
		}
		logger.Info("Removing client from cache")
		r.RulerClients.RemoveClient(clientConfig.Name)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClientConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openawarenessv1beta1.ClientConfig{}).
		Complete(r)
}
