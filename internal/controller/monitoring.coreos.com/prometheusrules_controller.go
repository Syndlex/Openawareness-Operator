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

package monitoringcoreoscom

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/model/rulefmt"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PrometheusRulesReconciler reconciles a PrometheusRules object
type PrometheusRulesReconciler struct {
	RulerClients *clients.RulerClientCache
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the PrometheusRules object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *PrometheusRulesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rule := &monitoringv1.PrometheusRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Found Rule", "name", rule.Name, "namespace", rule.Namespace)

	alertManagerClient, err := r.clientFromAnnotation(logger, rule)
	if err != nil {
		logger.V(1).Info("No Alertmanager client found. Please create a new "+openawarenessv1beta1.GroupVersion.Group+" ClientConfig", "name", rule.Name, "namespace", rule.Namespace)
		return ctrl.Result{}, nil
	}

	namespace := r.getNamespaceFromAnnotations(logger, rule)

	if rule.ObjectMeta.DeletionTimestamp.IsZero() {
		// Register finalizer
		if !controllerutil.ContainsFinalizer(rule, utils.MyFinalizerName) {
			controllerutil.AddFinalizer(rule, utils.MyFinalizerName)
			if err := r.Update(ctx, rule); err != nil {
				return ctrl.Result{}, err
			}
		}
		groups := convert(rule.Spec.Groups)
		for _, group := range groups {
			err := alertManagerClient.CreateRuleGroup(ctx, namespace, group)
			if err != nil {
				logger.Error(err, "Failed to create rule group", "group", group.Name, "namespace", namespace, "rule", rule.Name)
				return ctrl.Result{}, err
			}
		}

	} else {
		for _, group := range rule.Spec.Groups {
			err := alertManagerClient.DeleteRuleGroup(ctx, namespace, group.Name)
			if err != nil {
				logger.Error(err, "Failed to delete rule group", "group", group.Name, "namespace", namespace, "rule", rule.Name)
				return ctrl.Result{}, err
			}
		}

		// The object is being deleted check for finalizer
		if controllerutil.ContainsFinalizer(rule, utils.MyFinalizerName) {
			controllerutil.RemoveFinalizer(rule, utils.MyFinalizerName)
			if err := r.Update(ctx, rule); err != nil {
				return ctrl.Result{}, err
			}
			logger.Info("PrometheusRule was deleted", "name", rule.Name, "namespace", rule.Namespace)
		}
	}
	return ctrl.Result{}, nil
}

// convert transforms PrometheusRule RuleGroups to Mimir's rulefmt.RuleGroup format.
// It processes each rule group and converts individual rules to the appropriate format.
func convert(groups []monitoringv1.RuleGroup) []rulefmt.RuleGroup {
	returnGroups := make([]rulefmt.RuleGroup, 0)
	for _, group := range groups {
		returnRules := make([]rulefmt.RuleNode, 0)
		for _, rule := range group.Rules {
			returnRules = append(returnRules, newRule(rule))
		}
		returnGroups = append(returnGroups, rulefmt.RuleGroup{
			Name: group.Name,
			//Interval: group.Interval, todo
			Rules: returnRules,
		})
	}

	return returnGroups

}

// newRule converts a single PrometheusRule to a rulefmt.RuleNode.
// It handles both alert rules (with Alert field) and recording rules (with Record field).
func newRule(rule monitoringv1.Rule) rulefmt.RuleNode {
	if rule.Alert != "" {
		return rulefmt.RuleNode{
			Alert:         yaml.Node{Kind: yaml.ScalarNode, Value: rule.Alert},
			Expr:          yaml.Node{Kind: yaml.ScalarNode, Value: rule.Expr.String()},
			For:           0,
			KeepFiringFor: 0,
			Labels:        rule.Labels,
			Annotations:   rule.Annotations,
		}
	} else {
		return rulefmt.RuleNode{
			Record:        yaml.Node{Kind: yaml.ScalarNode, Value: rule.Record},
			Expr:          yaml.Node{Kind: yaml.ScalarNode, Value: rule.Expr.String()},
			For:           0,
			KeepFiringFor: 0,
			Labels:        rule.Labels,
			Annotations:   rule.Annotations,
		}
	}
}

// clientFromAnnotation retrieves the appropriate Mimir client for the given PrometheusRule.
// It extracts the client name from the resource's annotations and returns the cached client.
// Returns an error if the annotation is missing or if the client is not found in the cache.
func (r *PrometheusRulesReconciler) clientFromAnnotation(logger logr.Logger, rule *monitoringv1.PrometheusRule) (clients.AwarenessClient, error) {
	if rule.Annotations == nil {
		logger.Info("PrometheusRule is missing client annotation", "annotation", utils.ClientNameAnnotation, "name", rule.Name, "namespace", rule.Namespace)
		return nil, fmt.Errorf("annotation %s is missing for PrometheusRule %s/%s", utils.ClientNameAnnotation, rule.Namespace, rule.Name)
	}

	clientName := rule.Annotations[utils.ClientNameAnnotation]
	if clientName == "" {
		logger.Info("PrometheusRule client annotation is empty", "annotation", utils.ClientNameAnnotation, "name", rule.Name, "namespace", rule.Namespace)
		return nil, fmt.Errorf("annotation %s is empty for PrometheusRule %s/%s", utils.ClientNameAnnotation, rule.Namespace, rule.Name)
	}

	alertManagerClient, err := r.RulerClients.GetClient(clientName)
	if err != nil {
		logger.Info("Client does not exist in cache", "clientName", clientName, "name", rule.Name, "namespace", rule.Namespace)
		return nil, fmt.Errorf("getting client %s for PrometheusRule %s/%s: %w", clientName, rule.Namespace, rule.Name, err)
	}
	return alertManagerClient, nil
}

// getNamespaceFromAnnotations extracts the Mimir tenant namespace from the PrometheusRule annotations.
// Returns the tenant ID from the annotation, or the default tenant ID if the annotation is not set.
func (r *PrometheusRulesReconciler) getNamespaceFromAnnotations(logger logr.Logger, rule *monitoringv1.PrometheusRule) string {
	mimirNamespace := rule.Annotations[utils.MimirTenantAnnotation]
	if mimirNamespace == "" {
		logger.V(1).Info("Using default tenant ID because annotation is missing", "annotation", utils.MimirTenantAnnotation, "defaultTenant", utils.DefaultTenantID, "name", rule.Name, "namespace", rule.Namespace)
		return utils.DefaultTenantID
	}
	return mimirNamespace
}

// SetupWithManager sets up the controller with the Manager.
func (r *PrometheusRulesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1.PrometheusRule{}).
		Complete(r)
}
