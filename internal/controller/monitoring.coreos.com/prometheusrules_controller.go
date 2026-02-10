// Package monitoringcoreoscom provides controllers for monitoring.coreos.com CRDs.
package monitoringcoreoscom

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/model/rulefmt"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// PrometheusRulesReconciler reconciles a PrometheusRules object
type PrometheusRulesReconciler struct {
	RulerClients *clients.RulerClientCache
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete
//nolint:lll
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules/finalizers,verbs=update
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reconciles the PrometheusRule resource by syncing rule groups
// to the configured Mimir instance. It handles the full lifecycle including creation,
// updates, and deletion of rule groups with proper finalizer management.
//
// Note: Status management is not implemented for PrometheusRule resources because
// the prometheus-operator v0.88.1 ConfigResourceStatus type does not include a
// Conditions field. Status updates are only supported for custom CRDs (ClientConfig
// and MimirAlertTenant) that define their own status structures.
//
// The reconciliation process:
// 1. Fetches the PrometheusRule resource
// 2. Retrieves the Mimir client from annotations
// 3. Adds finalizer for cleanup on deletion
// 4. Converts and pushes rule groups to Mimir API
// 5. On deletion, removes rule groups from Mimir and cleans up finalizer
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
		r.Recorder.Event(rule, corev1.EventTypeWarning, "ClientNotFound",
			fmt.Sprintf("No client configuration found: %v", err))
		logger.Info(
			"Client not found, will retry in 5 seconds. Please create a new "+openawarenessv1beta1.GroupVersion.Group+" ClientConfig",
			"name", rule.Name,
			"namespace", rule.Namespace,
			"error", err.Error(),
		)
		// Requeue to retry when client becomes available
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	namespace := r.getNamespaceFromAnnotations(logger, rule)

	if rule.DeletionTimestamp.IsZero() {
		// Register finalizer
		if !controllerutil.ContainsFinalizer(rule, utils.FinalizerAnnotation) {
			controllerutil.AddFinalizer(rule, utils.FinalizerAnnotation)
			if err := r.Update(ctx, rule); err != nil {
				return ctrl.Result{}, err
			}
		}
		groups := convert(rule.Spec.Groups)
		for _, group := range groups {
			err := alertManagerClient.CreateRuleGroup(ctx, namespace, group)
			if err != nil {
				r.Recorder.Eventf(rule, corev1.EventTypeWarning, "RuleGroupCreateFailed",
					"Failed to create rule group %s in namespace %s: %v", group.Name, namespace, err)
				logger.Error(err, "Failed to create rule group", "group", group.Name, "namespace", namespace, "rule", rule.Name)
				return ctrl.Result{}, err
			}
		}

		r.Recorder.Eventf(rule, corev1.EventTypeNormal, "RuleGroupsSynced",
			"Successfully synced %d rule group(s) to Mimir", len(groups))
		logger.Info("Successfully synced all rule groups",
			"name", rule.Name,
			"namespace", rule.Namespace,
			"groupCount", len(groups))

	} else {
		for _, group := range rule.Spec.Groups {
			err := alertManagerClient.DeleteRuleGroup(ctx, namespace, group.Name)
			if err != nil {
				r.Recorder.Eventf(rule, corev1.EventTypeWarning, "RuleGroupDeleteFailed",
					"Failed to delete rule group %s from namespace %s: %v", group.Name, namespace, err)
				logger.Error(err, "Failed to delete rule group", "group", group.Name, "namespace", namespace, "rule", rule.Name)
				return ctrl.Result{}, err
			}
		}

		r.Recorder.Event(rule, corev1.EventTypeNormal, "RuleGroupsDeleted",
			"Successfully deleted all rule groups from Mimir")

		// The object is being deleted check for finalizer
		if controllerutil.ContainsFinalizer(rule, utils.FinalizerAnnotation) {
			controllerutil.RemoveFinalizer(rule, utils.FinalizerAnnotation)
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
	returnGroups := make([]rulefmt.RuleGroup, 0, len(groups))
	for _, group := range groups {
		returnRules := make([]rulefmt.Rule, 0, len(group.Rules))
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

// newRule converts a single PrometheusRule to a rulefmt.Rule.
// It handles both alert rules (with Alert field) and recording rules (with Record field).
func newRule(rule monitoringv1.Rule) rulefmt.Rule {
	return rulefmt.Rule{
		Record:        rule.Record,
		Alert:         rule.Alert,
		Expr:          rule.Expr.String(),
		For:           0,
		KeepFiringFor: 0,
		Labels:        rule.Labels,
		Annotations:   rule.Annotations,
	}
}

// clientFromAnnotation retrieves the appropriate Mimir client for the given PrometheusRule.
// It extracts the client name from the resource's annotations and returns the cached client.
// Returns an error if the annotation is missing or if the client is not found in the cache.
func (r *PrometheusRulesReconciler) clientFromAnnotation(
	logger logr.Logger,
	rule *monitoringv1.PrometheusRule,
) (clients.AwarenessClient, error) {
	if rule.Annotations == nil {
		logger.Info(
			"PrometheusRule is missing client annotation",
			"annotation", utils.ClientNameAnnotation,
			"name", rule.Name,
			"namespace", rule.Namespace,
		)
		return nil, fmt.Errorf(
			"annotation %s is missing for PrometheusRule %s/%s",
			utils.ClientNameAnnotation,
			rule.Namespace,
			rule.Name,
		)
	}

	clientName := rule.Annotations[utils.ClientNameAnnotation]
	if clientName == "" {
		logger.Info(
			"PrometheusRule client annotation is empty",
			"annotation", utils.ClientNameAnnotation,
			"name", rule.Name,
			"namespace", rule.Namespace,
		)
		return nil, fmt.Errorf(
			"annotation %s is empty for PrometheusRule %s/%s",
			utils.ClientNameAnnotation,
			rule.Namespace,
			rule.Name,
		)
	}

	alertManagerClient, err := r.RulerClients.GetClient(clientName)
	if err != nil {
		logger.Info(
			"Client does not exist in cache",
			"clientName", clientName,
			"name", rule.Name,
			"namespace", rule.Namespace,
		)
		return nil, fmt.Errorf(
			"getting client %s for PrometheusRule %s/%s: %w",
			clientName,
			rule.Namespace,
			rule.Name,
			err,
		)
	}
	return alertManagerClient, nil
}

// getNamespaceFromAnnotations extracts the Mimir tenant namespace from the PrometheusRule annotations.
// Returns the tenant ID from the annotation, or the default tenant ID if the annotation is not set.
func (r *PrometheusRulesReconciler) getNamespaceFromAnnotations(
	logger logr.Logger,
	rule *monitoringv1.PrometheusRule,
) string {
	mimirNamespace := rule.Annotations[utils.MimirTenantAnnotation]
	if mimirNamespace == "" {
		logger.V(1).Info(
			"Using default tenant ID because annotation is missing",
			"annotation", utils.MimirTenantAnnotation,
			"defaultTenant", utils.DefaultTenantID,
			"name", rule.Name,
			"namespace", rule.Namespace,
		)
		return utils.DefaultTenantID
	}
	return mimirNamespace
}

// SetupWithManager sets up the controller with the Manager.
func (r *PrometheusRulesReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1.PrometheusRule{}).
		Watches(
			&openawarenessv1beta1.ClientConfig{},
			handler.EnqueueRequestsFromMapFunc(r.findPrometheusRulesForClient),
		).
		Complete(r)
}

// findPrometheusRulesForClient maps ClientConfig changes to PrometheusRule reconciliation requests.
// When a ClientConfig is created, updated, or deleted, this function finds all PrometheusRules
// that reference it and triggers their reconciliation.
func (r *PrometheusRulesReconciler) findPrometheusRulesForClient(ctx context.Context, client client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	clientConfig, ok := client.(*openawarenessv1beta1.ClientConfig)
	if !ok {
		logger.Error(fmt.Errorf("expected ClientConfig but got %T", client), "Unexpected object type in watch handler")
		return nil
	}

	// List all PrometheusRules
	rulesList := &monitoringv1.PrometheusRuleList{}
	if err := r.List(ctx, rulesList); err != nil {
		logger.Error(err, "Failed to list PrometheusRules for ClientConfig watch")
		return nil
	}

	var requests []reconcile.Request
	for _, rule := range rulesList.Items {
		// Check if this rule references the ClientConfig
		if rule.Annotations != nil {
			if clientName, exists := rule.Annotations[utils.ClientNameAnnotation]; exists && clientName == clientConfig.Name {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      rule.Name,
						Namespace: rule.Namespace,
					},
				})
				logger.V(1).Info("Queueing PrometheusRule reconciliation due to ClientConfig change",
					"prometheusRule", rule.Name,
					"namespace", rule.Namespace,
					"clientConfig", clientConfig.Name)
			}
		}
	}

	logger.V(1).Info("Found PrometheusRules referencing ClientConfig",
		"clientConfig", clientConfig.Name,
		"count", len(requests))

	return requests
}
