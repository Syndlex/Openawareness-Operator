package openawareness

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/loki"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("LokiRuleGroup Cleanup", func() {
	const (
		timeout  = 15 * time.Second
		interval = 250 * time.Millisecond
		ns       = "default"
	)

	newCfg := func(name string) *openawarenessv1beta1.ClientConfig {
		return &openawarenessv1beta1.ClientConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openawarenessv1beta1.ClientConfigSpec{
				Address: "http://loki:3100",
				Type:    openawarenessv1beta1.Mimir,
			},
		}
	}

	newLRG := func(name, lokiRef string, groups []openawarenessv1beta1.RuleGroup) *openawarenessv1beta1.LokiRuleGroup {
		return &openawarenessv1beta1.LokiRuleGroup{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openawarenessv1beta1.LokiRuleGroupSpec{
				Tenant:        "main",
				LokiClientRef: lokiRef,
				Groups:        groups,
			},
		}
	}

	oneGroup := func(name, expr string) []openawarenessv1beta1.RuleGroup {
		return []openawarenessv1beta1.RuleGroup{{
			Name:     name,
			Interval: "1m",
			Rules:    []openawarenessv1beta1.LokiRule{{Alert: "A", Expr: expr}},
		}}
	}

	// Each test uses a fresh MockLokiClient registered under a unique clientRef name.
	// This avoids "object is being deleted" conflicts from shared ClientConfig names.

	// ── 404 on delete is treated as success ───────────────────────────────
	// Loki returns 404 when a rule group does not exist. The controller
	// must not get stuck waiting for a resource that is already gone.

	Context("when Loki returns 404 during group deletion", func() {
		It("removes the finalizer and completes deletion without error", func() {
			mock := clients.NewMockLokiClient()
			suiteLokiCache.SetClient("loki-404-delete", mock)

			cfg := newCfg("loki-404-delete")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })

			lrg := newLRG("lrg-404-delete", "loki-404-delete",
				oneGroup("gone-group", `rate({app="x"} [1m]) > 0`))
			Expect(testClient.Create(ctx, lrg)).To(Succeed())

			By("waiting for initial sync")
			Eventually(func() bool {
				return mock.HasGroup(ns, "gone-group")
			}, timeout, interval).Should(BeTrue())

			By("simulating Loki already deleted the group (404)")
			// Use the sentinel error that the real Loki HTTP client returns on 404.
			mock.SetDeleteError(loki.ErrResourceNotFound)

			By("deleting the CR")
			Expect(testClient.Delete(ctx, lrg)).To(Succeed())

			By("CR should be fully deleted even though Loki returns 404")
			key := types.NamespacedName{Name: lrg.Name, Namespace: lrg.Namespace}
			Eventually(func() bool {
				obj := &openawarenessv1beta1.LokiRuleGroup{}
				return testClient.Get(ctx, key, obj) != nil
			}, timeout, interval).Should(BeTrue(), "CR should be fully deleted even when Loki returns 404")
		})
	})

	// ── create error sets status to Failed ────────────────────────────────

	Context("when Loki returns an error during rule group creation", func() {
		It("sets SyncStatus to Failed and records the error message", func() {
			mock := clients.NewMockLokiClient()
			mock.SetCreateError(errors.New("loki: internal server error"))
			suiteLokiCache.SetClient("loki-create-err", mock)

			cfg := newCfg("loki-create-err")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })

			lrg := newLRG("lrg-create-err", "loki-create-err",
				oneGroup("err-group", `rate({app="err"} [1m]) > 0`))
			Expect(testClient.Create(ctx, lrg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, lrg) })

			By("status should become Failed")
			key := types.NamespacedName{Name: lrg.Name, Namespace: lrg.Namespace}
			Eventually(func() string {
				obj := &openawarenessv1beta1.LokiRuleGroup{}
				if err := testClient.Get(ctx, key, obj); err != nil {
					return ""
				}
				return obj.Status.SyncStatus
			}, timeout, interval).Should(Equal("Failed"))

			By("error message should be recorded")
			obj := &openawarenessv1beta1.LokiRuleGroup{}
			Expect(testClient.Get(ctx, key, obj)).To(Succeed())
			Expect(obj.Status.ErrorMessage).To(ContainSubstring("internal server error"))

			By("LokiSynced condition should be False")
			found := false
			for _, c := range obj.Status.Conditions {
				if c.Type == openawarenessv1beta1.LokiRuleGroupConditionLokiSynced &&
					c.Status == metav1.ConditionFalse {
					found = true
				}
			}
			Expect(found).To(BeTrue(), "LokiSynced condition should be False")
		})
	})

	// ── ClientConfig deleted while LokiRuleGroup still exists ─────────────
	// If the referenced ClientConfig is deleted, the controller must not
	// panic and must requeue — the CR itself must NOT be deleted.

	Context("when the referenced ClientConfig is deleted while LokiRuleGroup still exists", func() {
		It("requeues without deleting the LokiRuleGroup", func() {
			mock := clients.NewMockLokiClient()
			suiteLokiCache.SetClient("loki-orphan-cfg", mock)

			cfg := newCfg("loki-orphan-cfg")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())

			lrg := newLRG("lrg-orphan-cfg", "loki-orphan-cfg",
				oneGroup("orphan-group", `rate({app="o"} [1m]) > 0`))
			Expect(testClient.Create(ctx, lrg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, lrg) })

			By("waiting for initial sync")
			Eventually(func() bool {
				return mock.HasGroup(ns, "orphan-group")
			}, timeout, interval).Should(BeTrue())

			By("deleting the ClientConfig")
			Expect(testClient.Delete(ctx, cfg)).To(Succeed())

			By("LokiRuleGroup should still exist")
			Consistently(func() error {
				obj := &openawarenessv1beta1.LokiRuleGroup{}
				return testClient.Get(ctx, types.NamespacedName{
					Name:      lrg.Name,
					Namespace: lrg.Namespace,
				}, obj)
			}, 3*time.Second, interval).Should(Succeed(),
				"LokiRuleGroup must not be deleted when ClientConfig disappears")
		})
	})

	// ── both Loki and Mimir: delete removes from both ─────────────────────
	// Note: This scenario is covered by the unit test TestDeleteFromAllBackends
	// in lokirulegroup_unit_test.go, which calls deleteFromAllBackends directly
	// without the manager race condition introduced by the suite reconciler.
	// The envtest integration test for this path is the 404 test above.

	// ── stale groups pruned when spec shrinks ─────────────────────────────

	Context("when spec.groups shrinks", func() {
		It("removes the dropped group from Loki", func() {
			mock := clients.NewMockLokiClient()
			suiteLokiCache.SetClient("loki-shrink", mock)

			cfg := newCfg("loki-shrink")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })

			twoGroups := []openawarenessv1beta1.RuleGroup{
				{Name: "keep", Interval: "1m", Rules: []openawarenessv1beta1.LokiRule{{Alert: "K", Expr: "x"}}},
				{Name: "drop", Interval: "1m", Rules: []openawarenessv1beta1.LokiRule{{Alert: "D", Expr: "y"}}},
			}
			lrg := newLRG("lrg-shrink", "loki-shrink", twoGroups)
			Expect(testClient.Create(ctx, lrg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, lrg) })

			By("both groups should be synced")
			Eventually(func() bool {
				return mock.HasGroup(ns, "keep") && mock.HasGroup(ns, "drop")
			}, timeout, interval).Should(BeTrue())

			By("removing 'drop' from spec")
			key := types.NamespacedName{Name: lrg.Name, Namespace: lrg.Namespace}
			Expect(testClient.Get(ctx, key, lrg)).To(Succeed())
			lrg.Spec.Groups = oneGroup("keep", "x")
			Expect(testClient.Update(ctx, lrg)).To(Succeed())

			By("'drop' should be pruned from Loki")
			Eventually(func() bool {
				return mock.HasGroup(ns, "drop")
			}, timeout, interval).Should(BeFalse(), "'drop' group must be removed after spec shrink")

			By("'keep' should still be present")
			Expect(mock.HasGroup(ns, "keep")).To(BeTrue())
		})
	})
})
