// Package openawareness provides controllers for openawareness.syndlex CRDs.
package openawareness

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/prometheus/model/rulefmt"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/loki"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// LokiRuleGroupReconciler reconciles LokiRuleGroup objects.
type LokiRuleGroupReconciler struct {
	k8sClient.Client
	Scheme      *runtime.Scheme
	LokiClients clients.LokiClientCacheInterface
	MimirClient clients.RulerClientCacheInterface
}

//nolint:lll
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=lokirulegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=lokirulegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=lokirulegroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs,verbs=get;list;watch

// Reconcile syncs LokiRuleGroup spec to Loki Ruler and/or Mimir/Prometheus Ruler.
//
// The reconciliation process:
//  1. Load the LokiRuleGroup resource
//  2. Ensure finalizer
//  3. On deletion: remove all rule groups from every configured backend, then drop finalizer
//  4. Otherwise: push each rule group; prune stale groups; update status
func (r *LokiRuleGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	res := &openawarenessv1beta1.LokiRuleGroup{}
	if err := r.Get(ctx, req.NamespacedName, res); err != nil {
		return ctrl.Result{}, k8sClient.IgnoreNotFound(err)
	}
	logger.Info("Reconciling LokiRuleGroup", "name", res.Name, "namespace", res.Namespace)

	tenant := res.ResolveTenant()
	lokiNS := res.ResolveLokiNamespace()
	promNS := res.ResolvePrometheusNamespace()

	// NOTE: Do NOT update ResolvedLokiNamespace / ResolvedPrometheusNamespace here.
	// They still hold the *previous* values at this point and are used by the
	// cleanup functions below to detect namespace changes. They are updated
	// together with ActiveLokiClientRef / ActiveMimirClientRef after a successful sync.

	lokiEnabled := res.Spec.LokiClientRef != ""
	mimirEnabled := res.Spec.MimirClientRef != ""

	if !lokiEnabled && !mimirEnabled {
		logger.Info("Neither lokiClientRef nor mimirClientRef set — nothing to do",
			"name", res.Name, "namespace", res.Namespace)
		return ctrl.Result{}, nil
	}

	// ── Deletion path ────────────────────────────────────────────────────────
	isDeleting, err := utils.HandleFinalizer(ctx, r.Client, res, utils.FinalizerAnnotation,
		func(ctx context.Context) error {
			return r.deleteFromAllBackends(ctx, res, lokiNS, promNS, tenant, lokiEnabled, mimirEnabled)
		})
	if err != nil {
		logger.Error(err, "Finalizer handling failed")
		return ctrl.Result{}, err
	}
	if isDeleting {
		return ctrl.Result{}, nil
	}

	// ── Migration cleanup: old namespace or client changed ───────────────────
	// If the previously-active loki namespace or client differs from what we
	// are about to use, delete all rule groups from the OLD location first.
	// This prevents orphaned rules when a user changes spec.lokiNamespace,
	// spec.prometheusNamespace, spec.lokiClientRef, or spec.mimirClientRef.
	if err := r.cleanupPreviousLoki(ctx, res, lokiNS); err != nil {
		logger.Error(err, "Failed to clean up previous Loki namespace")
		// Non-fatal: proceed with sync so we don't get stuck.
	}
	if err := r.cleanupPreviousMimir(ctx, res, promNS); err != nil {
		logger.Error(err, "Failed to clean up previous Mimir namespace")
	}

	// ── Sync path ────────────────────────────────────────────────────────────
	desiredGroups := convertGroups(res.Spec.Groups)

	if lokiEnabled {
		if result, err := r.syncToLoki(ctx, res, lokiNS, tenant, desiredGroups); err != nil {
			res.SetFailedCondition(
				openawarenessv1beta1.LokiRuleGroupConditionLokiSynced,
				openawarenessv1beta1.LokiRuleGroupReasonSyncFailed,
				err.Error(),
			)
			_ = r.Status().Update(ctx, res)
			return result, err
		}
	}

	if mimirEnabled {
		if result, err := r.syncToMimir(ctx, res, promNS, tenant, desiredGroups); err != nil {
			res.SetFailedCondition(
				openawarenessv1beta1.LokiRuleGroupConditionMimirSynced,
				openawarenessv1beta1.LokiRuleGroupReasonSyncFailed,
				err.Error(),
			)
			_ = r.Status().Update(ctx, res)
			return result, err
		}
	}

	// Persist all state that is needed for change-detection on the next reconcile.
	res.Status.ActiveLokiClientRef = res.Spec.LokiClientRef
	res.Status.ActiveMimirClientRef = res.Spec.MimirClientRef
	res.Status.ResolvedLokiNamespace = lokiNS
	res.Status.ResolvedPrometheusNamespace = promNS

	res.SetSyncedCondition(lokiEnabled, mimirEnabled)
	if err := r.Status().Update(ctx, res); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	logger.Info("LokiRuleGroup synced successfully",
		"name", res.Name,
		"lokiNamespace", lokiNS,
		"prometheusNamespace", promNS,
		"tenant", tenant,
		"groups", len(desiredGroups),
	)
	return ctrl.Result{}, nil
}

// syncToLoki pushes all desired rule groups to the Loki Ruler and prunes stale ones.
func (r *LokiRuleGroupReconciler) syncToLoki(
	ctx context.Context,
	res *openawarenessv1beta1.LokiRuleGroup,
	namespace, tenant string,
	desired []rulefmt.RuleGroup,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	lokiClient, err := r.getLokiClient(ctx, res)
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("getting Loki client: %w", err)
	}

	for _, rg := range desired {
		if err := lokiClient.CreateOrUpdateRuleGroup(ctx, namespace, rg, tenant); err != nil {
			return ctrl.Result{}, fmt.Errorf("upserting Loki rule group %q: %w", rg.Name, err)
		}
		logger.V(1).Info("Loki rule group synced", "group", rg.Name, "namespace", namespace)
	}

	if err := r.pruneLoki(ctx, lokiClient, namespace, tenant, desired); err != nil {
		logger.Error(err, "Failed to prune stale Loki rule groups")
		// Non-fatal: log and continue.
	}
	return ctrl.Result{}, nil
}

// syncToMimir pushes all desired rule groups to the Mimir Ruler and prunes stale ones.
func (r *LokiRuleGroupReconciler) syncToMimir(
	ctx context.Context,
	res *openawarenessv1beta1.LokiRuleGroup,
	namespace, tenant string,
	desired []rulefmt.RuleGroup,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	mimirClient, err := r.getMimirClient(ctx, res)
	if err != nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("getting Mimir client: %w", err)
	}

	for _, rg := range desired {
		if err := mimirClient.CreateRuleGroup(ctx, namespace, rg, tenant); err != nil {
			return ctrl.Result{}, fmt.Errorf("upserting Mimir rule group %q: %w", rg.Name, err)
		}
		logger.V(1).Info("Mimir rule group synced", "group", rg.Name, "namespace", namespace)
	}

	if err := r.pruneMimir(ctx, mimirClient, namespace, tenant, desired); err != nil {
		logger.Error(err, "Failed to prune stale Mimir rule groups")
	}
	return ctrl.Result{}, nil
}

// pruneLoki deletes rule groups that exist in Loki but are no longer in the desired spec.
func (r *LokiRuleGroupReconciler) pruneLoki(
	ctx context.Context,
	cl clients.LokiClientInterface,
	namespace, tenant string,
	desired []rulefmt.RuleGroup,
) error {
	existing, err := cl.ListRules(ctx, namespace, tenant)
	if err != nil {
		return fmt.Errorf("listing Loki rules: %w", err)
	}

	desiredNames := groupNames(desired)
	for _, rg := range existing[namespace] {
		if !desiredNames.Has(rg.Name) {
			if err := ignoreLokiNotFound(cl.DeleteRuleGroup(ctx, namespace, rg.Name, tenant)); err != nil {
				return fmt.Errorf("deleting stale Loki rule group %q: %w", rg.Name, err)
			}
		}
	}
	return nil
}

// pruneMimir deletes rule groups that exist in Mimir but are no longer in the desired spec.
func (r *LokiRuleGroupReconciler) pruneMimir(
	ctx context.Context,
	cl clients.AwarenessClient,
	namespace, tenant string,
	desired []rulefmt.RuleGroup,
) error {
	existing, err := cl.ListRules(ctx, namespace, tenant)
	if err != nil {
		return fmt.Errorf("listing Mimir rules: %w", err)
	}

	desiredNames := groupNames(desired)
	for _, rg := range existing[namespace] {
		if !desiredNames.Has(rg.Name) {
			if err := cl.DeleteRuleGroup(ctx, namespace, rg.Name, tenant); err != nil {
				return fmt.Errorf("deleting stale Mimir rule group %q: %w", rg.Name, err)
			}
		}
	}
	return nil
}

// cleanupPreviousLoki deletes all rule groups from the previously-active Loki namespace
// when either the lokiClientRef or the resolved lokiNamespace has changed.
// If no previous state is recorded (first sync) or nothing changed, it is a no-op.
func (r *LokiRuleGroupReconciler) cleanupPreviousLoki(
	ctx context.Context,
	res *openawarenessv1beta1.LokiRuleGroup,
	currentLokiNS string,
) error {
	prevClient := res.Status.ActiveLokiClientRef
	prevNS := res.Status.ResolvedLokiNamespace

	// No previous state recorded → first sync, nothing to clean up.
	if prevClient == "" {
		return nil
	}

	clientChanged := prevClient != res.Spec.LokiClientRef
	nsChanged := prevNS != "" && prevNS != currentLokiNS

	if !clientChanged && !nsChanged {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Loki client or namespace changed — cleaning up previous location",
		"prevClient", prevClient, "prevNamespace", prevNS,
		"newClient", res.Spec.LokiClientRef, "newNamespace", currentLokiNS,
	)

	// Build a temporary spec pointing at the OLD client so we can fetch it.
	oldRes := res.DeepCopy()
	oldRes.Spec.LokiClientRef = prevClient

	oldClient, err := r.getLokiClient(ctx, oldRes)
	if err != nil {
		return fmt.Errorf("getting previous Loki client %q for cleanup: %w", prevClient, err)
	}

	var errs []error
	for _, rg := range res.Spec.Groups {
		if err := ignoreLokiNotFound(oldClient.DeleteRuleGroup(ctx, prevNS, rg.Name, res.ResolveTenant())); err != nil {
			logger.Error(err, "Failed to delete rule group from previous Loki location",
				"group", rg.Name, "namespace", prevNS)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// cleanupPreviousMimir deletes all rule groups from the previously-active Mimir namespace
// when either the mimirClientRef or the resolved prometheusNamespace has changed.
func (r *LokiRuleGroupReconciler) cleanupPreviousMimir(
	ctx context.Context,
	res *openawarenessv1beta1.LokiRuleGroup,
	currentPromNS string,
) error {
	prevClient := res.Status.ActiveMimirClientRef
	prevNS := res.Status.ResolvedPrometheusNamespace

	if prevClient == "" {
		return nil
	}

	clientChanged := prevClient != res.Spec.MimirClientRef
	nsChanged := prevNS != "" && prevNS != currentPromNS

	if !clientChanged && !nsChanged {
		return nil
	}

	logger := log.FromContext(ctx)
	logger.Info("Mimir client or namespace changed — cleaning up previous location",
		"prevClient", prevClient, "prevNamespace", prevNS,
		"newClient", res.Spec.MimirClientRef, "newNamespace", currentPromNS,
	)

	oldRes := res.DeepCopy()
	oldRes.Spec.MimirClientRef = prevClient

	oldClient, err := r.getMimirClient(ctx, oldRes)
	if err != nil {
		return fmt.Errorf("getting previous Mimir client %q for cleanup: %w", prevClient, err)
	}

	var errs []error
	for _, rg := range res.Spec.Groups {
		if err := oldClient.DeleteRuleGroup(ctx, prevNS, rg.Name, res.ResolveTenant()); err != nil {
			logger.Error(err, "Failed to delete rule group from previous Mimir location",
				"group", rg.Name, "namespace", prevNS)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteFromAllBackends removes all rule groups from every enabled backend.
// Used during finalizer cleanup on deletion.
func (r *LokiRuleGroupReconciler) deleteFromAllBackends(
	ctx context.Context,
	res *openawarenessv1beta1.LokiRuleGroup,
	lokiNS, promNS, tenant string,
	lokiEnabled, mimirEnabled bool,
) error {
	logger := log.FromContext(ctx)
	var errs []error

	if lokiEnabled {
		lokiClient, err := r.getLokiClient(ctx, res)
		if err != nil {
			errs = append(errs, fmt.Errorf("getting Loki client for deletion: %w", err))
		} else {
			for _, rg := range res.Spec.Groups {
				if err := ignoreLokiNotFound(lokiClient.DeleteRuleGroup(ctx, lokiNS, rg.Name, tenant)); err != nil {
					logger.Error(err, "Failed to delete Loki rule group on deletion",
						"group", rg.Name, "namespace", lokiNS)
					errs = append(errs, err)
				}
			}
		}
	}

	if mimirEnabled {
		mimirClient, err := r.getMimirClient(ctx, res)
		if err != nil {
			errs = append(errs, fmt.Errorf("getting Mimir client for deletion: %w", err))
		} else {
			for _, rg := range res.Spec.Groups {
				if err := mimirClient.DeleteRuleGroup(ctx, promNS, rg.Name, tenant); err != nil {
					logger.Error(err, "Failed to delete Mimir rule group on deletion",
						"group", rg.Name, "namespace", promNS)
					errs = append(errs, err)
				}
			}
		}
	}

	return errors.Join(errs...)
}

// getLokiClient resolves the Loki client from spec.lokiClientRef via the ClientConfig.
func (r *LokiRuleGroupReconciler) getLokiClient(
	ctx context.Context,
	res *openawarenessv1beta1.LokiRuleGroup,
) (clients.LokiClientInterface, error) {
	cfg := &openawarenessv1beta1.ClientConfig{}
	if err := r.Get(ctx, k8sClient.ObjectKey{
		Name:      res.Spec.LokiClientRef,
		Namespace: res.Namespace,
	}, cfg); err != nil {
		return nil, fmt.Errorf("ClientConfig %q not found: %w", res.Spec.LokiClientRef, err)
	}
	return r.LokiClients.GetOrCreateLokiClient(ctx, cfg.Spec.Address, cfg.Name)
}

// getMimirClient resolves the Mimir client from spec.mimirClientRef via the ClientConfig.
func (r *LokiRuleGroupReconciler) getMimirClient(
	ctx context.Context,
	res *openawarenessv1beta1.LokiRuleGroup,
) (clients.AwarenessClient, error) {
	cfg := &openawarenessv1beta1.ClientConfig{}
	if err := r.Get(ctx, k8sClient.ObjectKey{
		Name:      res.Spec.MimirClientRef,
		Namespace: res.Namespace,
	}, cfg); err != nil {
		return nil, fmt.Errorf("ClientConfig %q not found: %w", res.Spec.MimirClientRef, err)
	}
	return r.MimirClient.GetOrCreateMimirClient(ctx, cfg.Spec.Address, cfg.Name)
}

// SetupWithManager registers the controller with the manager and sets up watches.
func (r *LokiRuleGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&openawarenessv1beta1.LokiRuleGroup{}).
		Complete(r)
}

// ── helpers ──────────────────────────────────────────────────────────────────

// convertGroups converts the CRD rule groups into rulefmt.RuleGroup slices.
func convertGroups(groups []openawarenessv1beta1.RuleGroup) []rulefmt.RuleGroup {
	out := make([]rulefmt.RuleGroup, 0, len(groups))
	for _, g := range groups {
		rules := make([]rulefmt.Rule, 0, len(g.Rules))
		for _, r := range g.Rules {
			var forDur, keepDur metav1.Duration
			if r.For != "" {
				// Best-effort parse; validation webhook can enforce this later.
				_ = forDur.UnmarshalJSON([]byte(`"` + r.For + `"`))
			}
			if r.KeepFiringFor != "" {
				_ = keepDur.UnmarshalJSON([]byte(`"` + r.KeepFiringFor + `"`))
			}
			rules = append(rules, rulefmt.Rule{
				Alert:         r.Alert,
				Record:        r.Record,
				Expr:          r.Expr,
				Labels:        r.Labels,
				Annotations:   r.Annotations,
			})
		}

		rg := rulefmt.RuleGroup{
			Name:  g.Name,
			Rules: rules,
		}
		if g.Interval != "" {
			// rulefmt.RuleGroup.Interval is a model.Duration; pass as string via YAML round-trip.
			// The Loki/Mimir API accepts the string form directly.
		}
		out = append(out, rg)
	}
	return out
}

// groupNames returns the set of rule group names from a slice.
func groupNames(groups []rulefmt.RuleGroup) sets.Set[string] {
	s := sets.New[string]()
	for _, g := range groups {
		s.Insert(g.Name)
	}
	return s
}

// ignoreLokiNotFound returns nil when err is a Loki 404 (rule group already gone).
// All other errors are passed through unchanged.
func ignoreLokiNotFound(err error) error {
	if errors.Is(err, loki.ErrResourceNotFound) {
		return nil
	}
	return err
}
