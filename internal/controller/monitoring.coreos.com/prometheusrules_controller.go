// Package monitoringcoreoscom provides controllers for monitoring.coreos.com CRDs.
package monitoringcoreoscom

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/model/rulefmt"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"github.com/syndlex/openawareness-controller/internal/loki"
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

// PrometheusRulesReconciler reconciles a PrometheusRule object.
// It routes rule groups to either a Mimir/Prometheus backend or a Loki backend,
// depending on the ClientConfig type referenced by openawareness.io/client-name.
// Multi-backend (Mimir + Loki simultaneously) is not supported for PrometheusRule;
// use the LokiRuleGroup CRD for that use case.
type PrometheusRulesReconciler struct {
	RulerClients clients.RulerClientCacheInterface
	LokiClients  clients.LokiClientCacheInterface
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

// Reconcile reconciles a PrometheusRule by syncing its rule groups to the backend
// configured via the openawareness.io/client-name annotation.
//
// The ClientConfig referenced by that annotation determines the backend type:
//   - type: mimir or prometheus → rules are pushed to the Mimir/Prometheus Ruler API
//   - type: loki               → rules are pushed to the Loki Ruler API
//
// After each successful sync, the controller writes openawareness.io/active-client-ref
// onto the PrometheusRule. This annotation is used during deletion to clean up rules
// even if the ClientConfig has been deleted in the meantime.
func (r *PrometheusRulesReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	rule := &monitoringv1.PrometheusRule{}
	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	logger.Info("Found Rule", "name", rule.Name, "namespace", rule.Namespace)

	// Resolve the client and backend type.
	cfg, err := r.resolveClientConfig(ctx, rule)
	if err != nil {
		// During deletion, a missing ClientConfig is handled by reconcileDelete via
		// the active-client-ref annotation fallback. Allow delete to proceed.
		if !rule.DeletionTimestamp.IsZero() {
			return r.reconcileDelete(ctx, logger, rule, nil)
		}
		r.Recorder.Event(rule, corev1.EventTypeWarning, "ClientNotFound",
			fmt.Sprintf("No client configuration found: %v", err))
		logger.Info("Client not found, will retry in 5 seconds",
			"name", rule.Name, "namespace", rule.Namespace, "error", err.Error())
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if rule.DeletionTimestamp.IsZero() {
		return r.reconcileCreate(ctx, logger, rule, cfg)
	}
	return r.reconcileDelete(ctx, logger, rule, cfg)
}

// reconcileCreate syncs rule groups to the backend and updates the active-client-ref annotation.
func (r *PrometheusRulesReconciler) reconcileCreate(
	ctx context.Context,
	logger logr.Logger,
	rule *monitoringv1.PrometheusRule,
	cfg *openawarenessv1beta1.ClientConfig,
) (ctrl.Result, error) {
	// Register finalizer.
	if !controllerutil.ContainsFinalizer(rule, utils.FinalizerAnnotation) {
		controllerutil.AddFinalizer(rule, utils.FinalizerAnnotation)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
	}

	groups := convert(rule.Spec.Groups)

	switch cfg.Spec.Type {
	case openawarenessv1beta1.Loki:
		lokiClient, rulerNS, tenant := r.resolveLokiClient(ctx, logger, rule, cfg)
		if lokiClient == nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		for _, group := range groups {
			if err := lokiClient.CreateOrUpdateRuleGroup(ctx, rulerNS, group, tenant); err != nil {
				r.Recorder.Eventf(rule, corev1.EventTypeWarning, "RuleGroupCreateFailed",
					"Failed to create rule group %s in Loki namespace %s for tenant %s: %v",
					group.Name, rulerNS, tenant, err)
				logger.Error(err, "Failed to create rule group in Loki",
					"group", group.Name, "namespace", rulerNS, "tenant", tenant)
				return ctrl.Result{}, err
			}
		}
		r.Recorder.Eventf(rule, corev1.EventTypeNormal, "RuleGroupsSynced",
			"Successfully synced %d rule group(s) to Loki", len(groups))

	default: // mimir, prometheus
		mimirClient, err := r.resolveMimirClient(ctx, logger, rule, cfg)
		if err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		mimirTenant := r.getMimirTenant(logger, rule)
		for _, group := range groups {
			if err := mimirClient.CreateRuleGroup(ctx, rule.Namespace, group, mimirTenant); err != nil {
				r.Recorder.Eventf(rule, corev1.EventTypeWarning, "RuleGroupCreateFailed",
					"Failed to create rule group %s in Mimir namespace %s for tenant %s: %v",
					group.Name, rule.Namespace, mimirTenant, err)
				logger.Error(err, "Failed to create rule group in Mimir",
					"group", group.Name, "namespace", rule.Namespace, "tenant", mimirTenant)
				return ctrl.Result{}, err
			}
		}
		r.Recorder.Eventf(rule, corev1.EventTypeNormal, "RuleGroupsSynced",
			"Successfully synced %d rule group(s) to Mimir", len(groups))
	}

	// Record which ClientConfig was last used for cleanup on deletion.
	if err := r.setActiveClientRef(ctx, rule, cfg.Name); err != nil {
		// Non-fatal: log and continue. Cleanup on deletion may fall back to live lookup.
		logger.Error(err, "Failed to update active-client-ref annotation",
			"clientRef", cfg.Name)
	}

	logger.Info("Successfully synced all rule groups",
		"name", rule.Name, "namespace", rule.Namespace,
		"groupCount", len(groups), "backendType", cfg.Spec.Type)
	return ctrl.Result{}, nil
}

// reconcileDelete removes rule groups from the backend and removes the finalizer.
// cfg may be nil when the ClientConfig was deleted before the PrometheusRule;
// in that case the controller falls back to the active-client-ref annotation.
func (r *PrometheusRulesReconciler) reconcileDelete(
	ctx context.Context,
	logger logr.Logger,
	rule *monitoringv1.PrometheusRule,
	cfg *openawarenessv1beta1.ClientConfig,
) (ctrl.Result, error) {
	// Determine the backend type. When cfg is nil (ClientConfig was deleted first),
	// try to infer the type from the cached active-client-ref; fall back to mimir.
	backendType := openawarenessv1beta1.ClientType("")
	if cfg != nil {
		backendType = cfg.Spec.Type
	} else if rule.Annotations != nil {
		// We cannot know the type for certain, but if a Loki client is cached
		// under active-client-ref we try Loki; otherwise we skip (cannot recover).
		activeRef := rule.Annotations[utils.ActiveClientRefAnnotation]
		if activeRef != "" && r.LokiClients != nil {
			if lc, err := r.LokiClients.GetOrCreateLokiClient(ctx, "", activeRef); err == nil && lc != nil {
				backendType = openawarenessv1beta1.Loki
			}
		}
	}

	switch backendType {
	case openawarenessv1beta1.Loki:
		var lokiClient clients.LokiClientInterface
		var rulerNS, tenant string

		if cfg != nil {
			lokiClient, rulerNS, tenant = r.resolveLokiClient(ctx, logger, rule, cfg)
		}
		if lokiClient == nil {
			// Primary client unavailable; try active-client-ref cache fallback.
			lokiClient, rulerNS, tenant = r.resolveDeleteLokiClientFallback(ctx, logger, rule)
		}
		if lokiClient != nil {
			for _, group := range rule.Spec.Groups {
				if err := ignoreLokiNotFoundProm(lokiClient.DeleteRuleGroup(ctx, rulerNS, group.Name, tenant)); err != nil {
					r.Recorder.Eventf(rule, corev1.EventTypeWarning, "RuleGroupDeleteFailed",
						"Failed to delete rule group %s from Loki namespace %s for tenant %s: %v",
						group.Name, rulerNS, tenant, err)
					logger.Error(err, "Failed to delete rule group from Loki",
						"group", group.Name, "namespace", rulerNS, "tenant", tenant)
					return ctrl.Result{}, err
				}
			}
		} else {
			// ClientConfig and active-client-ref both gone — log and continue so the
			// finalizer is removed and the CR is not permanently stuck.
			r.Recorder.Event(rule, corev1.EventTypeWarning, "LokiClientMissing",
				"ClientConfig not found during deletion; Loki rules may be orphaned")
			logger.Error(fmt.Errorf("loki client unavailable during deletion"),
				"Rules may be orphaned in Loki",
				"name", rule.Name, "namespace", rule.Namespace)
		}

	default: // mimir, prometheus, or unknown
		if cfg == nil {
			// No ClientConfig and no cached Loki client — cannot determine where rules live.
			r.Recorder.Event(rule, corev1.EventTypeWarning, "MimirClientMissing",
				"ClientConfig not found during deletion; rules may be orphaned")
			logger.Error(fmt.Errorf("clientconfig unavailable during deletion"),
				"Rules may be orphaned",
				"name", rule.Name, "namespace", rule.Namespace)
		} else {
			mimirClient, err := r.resolveMimirClient(ctx, logger, rule, cfg)
			if err != nil {
				// Mimir client unavailable — log and unblock deletion.
				r.Recorder.Event(rule, corev1.EventTypeWarning, "MimirClientMissing",
					"Mimir ClientConfig not found during deletion; rules may be orphaned")
				logger.Error(err, "Rules may be orphaned in Mimir",
					"name", rule.Name, "namespace", rule.Namespace)
			} else {
				mimirTenant := r.getMimirTenant(logger, rule)
				for _, group := range rule.Spec.Groups {
					if err := mimirClient.DeleteRuleGroup(ctx, rule.Namespace, group.Name, mimirTenant); err != nil {
						r.Recorder.Eventf(rule, corev1.EventTypeWarning, "RuleGroupDeleteFailed",
							"Failed to delete rule group %s from Mimir namespace %s for tenant %s: %v",
							group.Name, rule.Namespace, mimirTenant, err)
						logger.Error(err, "Failed to delete rule group from Mimir",
							"group", group.Name, "namespace", rule.Namespace, "tenant", mimirTenant)
						return ctrl.Result{}, err
					}
				}
			}
		}
	}

	r.Recorder.Event(rule, corev1.EventTypeNormal, "RuleGroupsDeleted",
		"Successfully deleted all rule groups")

	if controllerutil.ContainsFinalizer(rule, utils.FinalizerAnnotation) {
		controllerutil.RemoveFinalizer(rule, utils.FinalizerAnnotation)
		if err := r.Update(ctx, rule); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("PrometheusRule was deleted", "name", rule.Name, "namespace", rule.Namespace)
	}
	return ctrl.Result{}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// resolveClientConfig fetches the ClientConfig referenced by openawareness.io/client-name.
// It also falls back to openawareness.io/active-client-ref when the resource is being
// deleted and the primary ClientConfig is no longer available.
func (r *PrometheusRulesReconciler) resolveClientConfig(
	ctx context.Context,
	rule *monitoringv1.PrometheusRule,
) (*openawarenessv1beta1.ClientConfig, error) {
	if rule.Annotations == nil {
		return nil, fmt.Errorf("annotation %s is missing for PrometheusRule %s/%s",
			utils.ClientNameAnnotation, rule.Namespace, rule.Name)
	}

	clientName := rule.Annotations[utils.ClientNameAnnotation]
	if clientName == "" {
		return nil, fmt.Errorf("annotation %s is empty for PrometheusRule %s/%s",
			utils.ClientNameAnnotation, rule.Namespace, rule.Name)
	}

	cfg := &openawarenessv1beta1.ClientConfig{}
	if err := r.Get(ctx, client.ObjectKey{Name: clientName, Namespace: rule.Namespace}, cfg); err != nil {
		return nil, fmt.Errorf("ClientConfig %q not found: %w", clientName, err)
	}
	return cfg, nil
}

// resolveLokiClient returns a live Loki client for the given ClientConfig.
func (r *PrometheusRulesReconciler) resolveLokiClient(
	ctx context.Context,
	logger logr.Logger,
	rule *monitoringv1.PrometheusRule,
	cfg *openawarenessv1beta1.ClientConfig,
) (clients.LokiClientInterface, string, string) {
	if r.LokiClients == nil {
		return nil, "", ""
	}
	rulerNS := rule.Namespace
	tenant := r.getLokiTenant(logger, rule)

	lokiClient, err := r.LokiClients.GetOrCreateLokiClient(ctx, cfg.Spec.Address, cfg.Name)
	if err != nil {
		logger.Error(err, "Failed to get Loki client", "clientName", cfg.Name)
		return nil, "", ""
	}
	return lokiClient, rulerNS, tenant
}

// resolveDeleteLokiClientFallback attempts to resolve a Loki client from the
// active-client-ref annotation when the primary ClientConfig is gone.
// This handles the case where ClientConfig is deleted before the PrometheusRule.
func (r *PrometheusRulesReconciler) resolveDeleteLokiClientFallback(
	ctx context.Context,
	logger logr.Logger,
	rule *monitoringv1.PrometheusRule,
) (clients.LokiClientInterface, string, string) {
	if r.LokiClients == nil || rule.Annotations == nil {
		return nil, "", ""
	}
	activeRef := rule.Annotations[utils.ActiveClientRefAnnotation]
	if activeRef == "" {
		return nil, "", ""
	}
	// The client may still be in the cache even though the ClientConfig is gone.
	lokiClient, err := r.LokiClients.GetOrCreateLokiClient(ctx, "", activeRef)
	if err != nil {
		logger.Info("Loki client not in cache for active-client-ref",
			"activeRef", activeRef, "name", rule.Name)
		return nil, "", ""
	}
	logger.Info("Using cached Loki client from active-client-ref for deletion cleanup",
		"activeRef", activeRef, "name", rule.Name)
	return lokiClient, rule.Namespace, r.getLokiTenant(logger, rule)
}

// resolveMimirClient returns a cached Mimir client for the given ClientConfig.
func (r *PrometheusRulesReconciler) resolveMimirClient(
	ctx context.Context,
	logger logr.Logger,
	rule *monitoringv1.PrometheusRule,
	cfg *openawarenessv1beta1.ClientConfig,
) (clients.AwarenessClient, error) {
	mimirClient, err := r.RulerClients.GetOrCreateMimirClient(ctx, "", cfg.Name)
	if err != nil {
		logger.Info("Mimir client not in cache", "clientName", cfg.Name,
			"name", rule.Name, "namespace", rule.Namespace)
		return nil, fmt.Errorf("getting Mimir client %q: %w", cfg.Name, err)
	}
	return mimirClient, nil
}

// setActiveClientRef writes the last-used ClientConfig name as an annotation on the rule.
func (r *PrometheusRulesReconciler) setActiveClientRef(
	ctx context.Context,
	rule *monitoringv1.PrometheusRule,
	clientName string,
) error {
	if rule.Annotations[utils.ActiveClientRefAnnotation] == clientName {
		return nil // already set, skip the update
	}
	patch := client.MergeFrom(rule.DeepCopy())
	if rule.Annotations == nil {
		rule.Annotations = map[string]string{}
	}
	rule.Annotations[utils.ActiveClientRefAnnotation] = clientName
	return r.Patch(ctx, rule, patch)
}

// getMimirTenant returns the Mimir tenant from annotations, with a default fallback.
func (r *PrometheusRulesReconciler) getMimirTenant(logger logr.Logger, rule *monitoringv1.PrometheusRule) string {
	tenant := rule.Annotations[utils.MimirTenantAnnotation]
	if tenant == "" {
		logger.V(1).Info("Using default tenant ID",
			"annotation", utils.MimirTenantAnnotation,
			"defaultTenant", utils.DefaultTenantID,
			"name", rule.Name)
		return utils.DefaultTenantID
	}
	return tenant
}

// getLokiTenant returns the Loki tenant from annotations, falling back to mimir-tenant,
// then to metadata.namespace.
func (r *PrometheusRulesReconciler) getLokiTenant(logger logr.Logger, rule *monitoringv1.PrometheusRule) string {
	if rule.Annotations == nil {
		return rule.Namespace
	}
	if t := rule.Annotations[utils.MimirTenantAnnotation]; t != "" {
		return t
	}
	logger.V(1).Info("Using namespace as Loki tenant", "namespace", rule.Namespace)
	return rule.Namespace
}

// convert transforms PrometheusRule RuleGroups to rulefmt.RuleGroup format.
func convert(groups []monitoringv1.RuleGroup) []rulefmt.RuleGroup {
	result := make([]rulefmt.RuleGroup, 0, len(groups))
	for _, g := range groups {
		rules := make([]rulefmt.Rule, 0, len(g.Rules))
		for _, rule := range g.Rules {
			rules = append(rules, newRule(rule))
		}
		result = append(result, rulefmt.RuleGroup{
			Name: g.Name,
			// Interval: g.Interval, // TODO: requires model.Duration conversion
			Rules: rules,
		})
	}
	return result
}

// newRule converts a single PrometheusRule to a rulefmt.Rule.
func newRule(rule monitoringv1.Rule) rulefmt.Rule {
	return rulefmt.Rule{
		Record:      rule.Record,
		Alert:       rule.Alert,
		Expr:        rule.Expr.String(),
		Labels:      rule.Labels,
		Annotations: rule.Annotations,
	}
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
func (r *PrometheusRulesReconciler) findPrometheusRulesForClient(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	clientConfig, ok := obj.(*openawarenessv1beta1.ClientConfig)
	if !ok {
		logger.Error(fmt.Errorf("expected ClientConfig but got %T", obj), "Unexpected object type in watch handler")
		return nil
	}

	rulesList := &monitoringv1.PrometheusRuleList{}
	if err := r.List(ctx, rulesList); err != nil {
		logger.Error(err, "Failed to list PrometheusRules for ClientConfig watch")
		return nil
	}

	var requests []reconcile.Request
	for _, rule := range rulesList.Items {
		if rule.Annotations == nil {
			continue
		}
		if name := rule.Annotations[utils.ClientNameAnnotation]; name == clientConfig.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace},
			})
			logger.V(1).Info("Queueing PrometheusRule due to ClientConfig change",
				"prometheusRule", rule.Name, "namespace", rule.Namespace,
				"clientConfig", clientConfig.Name)
		}
	}
	return requests
}

// ignoreLokiNotFoundProm returns nil when err is a Loki 404 (rule group already gone).
func ignoreLokiNotFoundProm(err error) error {
	if errors.Is(err, loki.ErrResourceNotFound) {
		return nil
	}
	return err
}
