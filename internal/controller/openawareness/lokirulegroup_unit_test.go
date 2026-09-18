package openawareness

import (
	"context"
	"testing"

	"github.com/prometheus/prometheus/model/rulefmt"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/loki"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Unit tests for LokiRuleGroup resolution helpers.
// These run without envtest — no cluster or manager required.

func TestResolveTenant_SpecTakesPriority(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"openawareness.syndlex/tenant": "from-annotation"},
		},
		Spec: openawarenessv1beta1.LokiRuleGroupSpec{Tenant: "from-spec"},
	}
	if got := lrg.ResolveTenant(); got != "from-spec" {
		t.Errorf("expected 'from-spec', got %q", got)
	}
}

func TestResolveTenant_AnnotationFallback(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"openawareness.syndlex/tenant": "via-annotation"},
		},
	}
	if got := lrg.ResolveTenant(); got != "via-annotation" {
		t.Errorf("expected 'via-annotation', got %q", got)
	}
}

func TestResolveTenant_DefaultMain(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{},
	}
	if got := lrg.ResolveTenant(); got != "main" {
		t.Errorf("expected 'main', got %q", got)
	}
}

func TestResolveLokiNamespace_SpecTakesPriority(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "k8s-ns",
			Annotations: map[string]string{"openawareness.syndlex/loki-namespace": "annotated"},
		},
		Spec: openawarenessv1beta1.LokiRuleGroupSpec{LokiNamespace: "spec-ns"},
	}
	if got := lrg.ResolveLokiNamespace(); got != "spec-ns" {
		t.Errorf("expected 'spec-ns', got %q", got)
	}
}

func TestResolveLokiNamespace_AnnotationFallback(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "k8s-ns",
			Annotations: map[string]string{"openawareness.syndlex/loki-namespace": "annotated-ns"},
		},
	}
	if got := lrg.ResolveLokiNamespace(); got != "annotated-ns" {
		t.Errorf("expected 'annotated-ns', got %q", got)
	}
}

func TestResolveLokiNamespace_DefaultK8sNamespace(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{Namespace: "my-team"},
	}
	if got := lrg.ResolveLokiNamespace(); got != "my-team" {
		t.Errorf("expected 'my-team', got %q", got)
	}
}

func TestResolvePrometheusNamespace_SpecTakesPriority(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "k8s-ns",
			Annotations: map[string]string{"openawareness.syndlex/prometheus-namespace": "annotated"},
		},
		Spec: openawarenessv1beta1.LokiRuleGroupSpec{PrometheusNamespace: "prom-spec-ns"},
	}
	if got := lrg.ResolvePrometheusNamespace(); got != "prom-spec-ns" {
		t.Errorf("expected 'prom-spec-ns', got %q", got)
	}
}

func TestResolvePrometheusNamespace_AnnotationFallback(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   "k8s-ns",
			Annotations: map[string]string{"openawareness.syndlex/prometheus-namespace": "prom-annotated"},
		},
	}
	if got := lrg.ResolvePrometheusNamespace(); got != "prom-annotated" {
		t.Errorf("expected 'prom-annotated', got %q", got)
	}
}

func TestResolvePrometheusNamespace_DefaultK8sNamespace(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ops-team"},
	}
	if got := lrg.ResolvePrometheusNamespace(); got != "ops-team" {
		t.Errorf("expected 'ops-team', got %q", got)
	}
}

func TestSetSyncedCondition(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{}
	lrg.SetSyncedCondition(true, false)

	if lrg.Status.SyncStatus != "Synced" {
		t.Errorf("expected SyncStatus 'Synced', got %q", lrg.Status.SyncStatus)
	}
	if lrg.Status.LastSyncTime == nil {
		t.Error("expected LastSyncTime to be set")
	}

	found := false
	for _, c := range lrg.Status.Conditions {
		if c.Type == openawarenessv1beta1.LokiRuleGroupConditionLokiSynced &&
			c.Status == "True" {
			found = true
		}
	}
	if !found {
		t.Error("expected LokiSynced condition to be True")
	}
}

func TestSetFailedCondition(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{}
	lrg.SetFailedCondition(
		openawarenessv1beta1.LokiRuleGroupConditionLokiSynced,
		openawarenessv1beta1.LokiRuleGroupReasonSyncFailed,
		"connection refused",
	)

	if lrg.Status.SyncStatus != "Failed" {
		t.Errorf("expected SyncStatus 'Failed', got %q", lrg.Status.SyncStatus)
	}
	if lrg.Status.ErrorMessage != "connection refused" {
		t.Errorf("unexpected ErrorMessage: %q", lrg.Status.ErrorMessage)
	}

	found := false
	for _, c := range lrg.Status.Conditions {
		if c.Type == openawarenessv1beta1.LokiRuleGroupConditionLokiSynced &&
			c.Status == "False" {
			found = true
		}
	}
	if !found {
		t.Error("expected LokiSynced condition to be False")
	}
}

func TestConvertGroups_EmptyInterval(t *testing.T) {
	groups := []openawarenessv1beta1.RuleGroup{
		{
			Name: "g1",
			Rules: []openawarenessv1beta1.LokiRule{
				{Alert: "A", Expr: `rate({app="x"} [1m]) > 0`,
					Labels:      map[string]string{"severity": "warning"},
					Annotations: map[string]string{"summary": "test"},
				},
				{Record: "rec:metric", Expr: `sum(rate({app="x"} [1m]))`},
			},
		},
	}
	out := convertGroups(groups)
	if len(out) != 1 {
		t.Fatalf("expected 1 group, got %d", len(out))
	}
	if len(out[0].Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(out[0].Rules))
	}
	if out[0].Rules[0].Alert != "A" {
		t.Errorf("expected alert 'A', got %q", out[0].Rules[0].Alert)
	}
	if out[0].Rules[0].Labels["severity"] != "warning" {
		t.Errorf("labels not copied correctly")
	}
	if out[0].Rules[1].Record != "rec:metric" {
		t.Errorf("expected record 'rec:metric', got %q", out[0].Rules[1].Record)
	}
}

func TestGroupNames(t *testing.T) {
	groups := []openawarenessv1beta1.RuleGroup{
		{Name: "alpha", Rules: []openawarenessv1beta1.LokiRule{{Expr: "x"}}},
		{Name: "beta", Rules: []openawarenessv1beta1.LokiRule{{Expr: "y"}}},
	}
	converted := convertGroups(groups)
	names := groupNames(converted)
	if !names.Has("alpha") {
		t.Error("expected 'alpha' in names")
	}
	if !names.Has("beta") {
		t.Error("expected 'beta' in names")
	}
	if names.Has("gamma") {
		t.Error("unexpected 'gamma' in names")
	}
}

func TestSetSyncedCondition_BothBackends(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{}
	lrg.SetSyncedCondition(true, true)

	foundLoki, foundMimir := false, false
	for _, c := range lrg.Status.Conditions {
		if c.Type == openawarenessv1beta1.LokiRuleGroupConditionLokiSynced && c.Status == "True" {
			foundLoki = true
		}
		if c.Type == openawarenessv1beta1.LokiRuleGroupConditionMimirSynced && c.Status == "True" {
			foundMimir = true
		}
	}
	if !foundLoki {
		t.Error("expected LokiSynced=True")
	}
	if !foundMimir {
		t.Error("expected MimirSynced=True")
	}
}

func TestSetSyncedCondition_LokiOnly(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{}
	lrg.SetSyncedCondition(true, false)

	for _, c := range lrg.Status.Conditions {
		if c.Type == openawarenessv1beta1.LokiRuleGroupConditionMimirSynced {
			t.Error("MimirSynced condition should not be set when mimir is disabled")
		}
	}
}

func TestSetConditionIdempotent(t *testing.T) {
	lrg := &openawarenessv1beta1.LokiRuleGroup{}
	lrg.SetSyncedCondition(true, false)
	lrg.SetSyncedCondition(true, false)

	count := 0
	for _, c := range lrg.Status.Conditions {
		if c.Type == openawarenessv1beta1.LokiRuleGroupConditionReady {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 Ready condition, got %d", count)
	}
}

// setupRecWithFakeClient builds a LokiRuleGroupReconciler with a fake K8s client
// that has the given ClientConfig objects pre-populated.
func setupRecWithFakeClient(
	t *testing.T,
	lokiCacheName string, lokiCache clients.LokiClientCacheInterface,
	mimirCacheName string, mimirCache clients.RulerClientCacheInterface,
	cfgNames ...string,
) (*LokiRuleGroupReconciler, *openawarenessv1beta1.LokiRuleGroup) {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := openawarenessv1beta1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}

	var objs []runtime.Object
	for _, name := range cfgNames {
		objs = append(objs, &openawarenessv1beta1.ClientConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       openawarenessv1beta1.ClientConfigSpec{Address: "http://mock:3100", Type: openawarenessv1beta1.Mimir},
		})
	}

	fakeK8s := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	rec := &LokiRuleGroupReconciler{
		Client:      fakeK8s,
		Scheme:      scheme,
		LokiClients: lokiCache,
		MimirClient: mimirCache,
	}

	res := &openawarenessv1beta1.LokiRuleGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: openawarenessv1beta1.LokiRuleGroupSpec{
			Tenant:         "main",
			LokiClientRef:  lokiCacheName,
			MimirClientRef: mimirCacheName,
			Groups: []openawarenessv1beta1.RuleGroup{
				{Name: "g1", Interval: "1m", Rules: []openawarenessv1beta1.LokiRule{{Alert: "A", Expr: "x"}}},
				{Name: "g2", Interval: "1m", Rules: []openawarenessv1beta1.LokiRule{{Alert: "B", Expr: "y"}}},
			},
		},
	}

	return rec, res
}

// TestDeleteFromAllBackends verifies that deleteFromAllBackends calls Delete on
// both Loki and Mimir when both backends are enabled.
func TestDeleteFromAllBackends(t *testing.T) {
	lokiMock := clients.NewMockLokiClient()
	lokiCache := clients.NewMockLokiClientCache()
	lokiCache.SetClient("loki-unit", lokiMock)

	mimirMock := clients.NewMockAwarenessClient()
	mimirCache := clients.NewMockRulerClientCache()
	mimirCache.SetClient("mimir-unit", mimirMock)

	rec, res := setupRecWithFakeClient(t,
		"loki-unit", lokiCache,
		"mimir-unit", mimirCache,
		"loki-unit", "mimir-unit")

	// Pre-populate Loki mock so delete calls are visible.
	_ = lokiMock.CreateOrUpdateRuleGroup(context.Background(), "default",
		rulefmt.RuleGroup{Name: "g1"}, "main")
	_ = lokiMock.CreateOrUpdateRuleGroup(context.Background(), "default",
		rulefmt.RuleGroup{Name: "g2"}, "main")

	err := rec.deleteFromAllBackends(context.Background(), res,
		"default", "default", "main", true, true)
	if err != nil {
		t.Fatalf("deleteFromAllBackends returned error: %v", err)
	}

	if lokiMock.HasGroup("default", "g1") || lokiMock.HasGroup("default", "g2") {
		t.Error("expected both Loki groups to be deleted")
	}
	if mimirMock.DeleteCalls() < 2 {
		t.Errorf("expected at least 2 Mimir DeleteRuleGroup calls, got %d", mimirMock.DeleteCalls())
	}
}

// TestDeleteFromAllBackends_LokiNotFound verifies that 404 from Loki does not
// block deleteFromAllBackends from completing successfully.
func TestDeleteFromAllBackends_LokiNotFound(t *testing.T) {
	lokiMock := clients.NewMockLokiClient()
	lokiMock.SetDeleteError(loki.ErrResourceNotFound)
	lokiCache := clients.NewMockLokiClientCache()
	lokiCache.SetClient("loki-unit-404", lokiMock)

	mimirMock := clients.NewMockAwarenessClient()
	mimirCache := clients.NewMockRulerClientCache()
	mimirCache.SetClient("mimir-unit-404", mimirMock)

	rec, res := setupRecWithFakeClient(t,
		"loki-unit-404", lokiCache,
		"mimir-unit-404", mimirCache,
		"loki-unit-404", "mimir-unit-404")
	res.Spec.Groups = res.Spec.Groups[:1] // one group is enough

	err := rec.deleteFromAllBackends(context.Background(), res,
		"default", "default", "main", true, true)
	if err != nil {
		t.Fatalf("expected no error when Loki returns 404, got: %v", err)
	}
	if mimirMock.DeleteCalls() < 1 {
		t.Error("expected Mimir DeleteRuleGroup to be called even when Loki returns 404")
	}
}
