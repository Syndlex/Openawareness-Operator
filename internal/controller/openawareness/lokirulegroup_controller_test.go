package openawareness

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("LokiRuleGroup Controller", func() {
	const (
		timeout  = 15 * time.Second
		interval = 250 * time.Millisecond
	)

	// ── helpers ────────────────────────────────────────────────────────────

	newClientConfig := func(name, ns, address string) *openawarenessv1beta1.ClientConfig {
		return &openawarenessv1beta1.ClientConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openawarenessv1beta1.ClientConfigSpec{
				Address: address,
				Type:    openawarenessv1beta1.Mimir,
			},
		}
	}

	newLRG := func(name, ns, lokiRef string, groups []openawarenessv1beta1.RuleGroup) *openawarenessv1beta1.LokiRuleGroup {
		return &openawarenessv1beta1.LokiRuleGroup{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openawarenessv1beta1.LokiRuleGroupSpec{
				Tenant:        "main",
				LokiClientRef: lokiRef,
				Groups:        groups,
			},
		}
	}

	singleGroup := func(groupName, expr string) []openawarenessv1beta1.RuleGroup {
		return []openawarenessv1beta1.RuleGroup{{
			Name:     groupName,
			Interval: "1m",
			Rules: []openawarenessv1beta1.LokiRule{
				{Alert: "TestAlert", Expr: expr, Labels: map[string]string{"severity": "warning"}},
			},
		}}
	}

	// Reset the shared suite mock before each test so state does not leak.
	BeforeEach(func() {
		suiteLokiMock = clients.NewMockLokiClient()
		suiteLokiCache.SetClient("loki-main", suiteLokiMock)
	})

	// ── create ────────────────────────────────────────────────────────────

	Context("when a LokiRuleGroup is created", func() {
		It("pushes rule groups to the Loki client and marks status Synced", func() {
			ns := "default"
			cfg := newClientConfig("loki-create", ns, "http://loki:3100")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })
			suiteLokiCache.SetClient("loki-create", suiteLokiMock)

			lrg := newLRG("lrg-create", ns, "loki-create",
				singleGroup("my-alert-group", `sum(rate({app="myapp"} [5m])) > 1`))
			Expect(testClient.Create(ctx, lrg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, lrg) })

			By("waiting for rule group to appear in mock Loki")
			Eventually(func() bool {
				return suiteLokiMock.HasGroup(ns, "my-alert-group")
			}, timeout, interval).Should(BeTrue())

			By("checking status is Synced")
			key := types.NamespacedName{Name: lrg.Name, Namespace: lrg.Namespace}
			Eventually(func() string {
				obj := &openawarenessv1beta1.LokiRuleGroup{}
				if err := testClient.Get(ctx, key, obj); err != nil {
					return ""
				}
				return obj.Status.SyncStatus
			}, timeout, interval).Should(Equal("Synced"))

			By("checking resolved namespace in status")
			obj := &openawarenessv1beta1.LokiRuleGroup{}
			Expect(testClient.Get(ctx, key, obj)).To(Succeed())
			Expect(obj.Status.ResolvedLokiNamespace).To(Equal(ns))
		})
	})

	// ── update ────────────────────────────────────────────────────────────

	Context("when a LokiRuleGroup spec is updated", func() {
		It("syncs the new group and prunes the old one", func() {
			ns := "default"
			cfg := newClientConfig("loki-update", ns, "http://loki:3100")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })
			suiteLokiCache.SetClient("loki-update", suiteLokiMock)

			lrg := newLRG("lrg-update", ns, "loki-update",
				singleGroup("group-v1", `rate({app="a"} [1m]) > 0`))
			Expect(testClient.Create(ctx, lrg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, lrg) })

			Eventually(func() bool {
				return suiteLokiMock.HasGroup(ns, "group-v1")
			}, timeout, interval).Should(BeTrue())

			By("updating the rule group")
			key := types.NamespacedName{Name: lrg.Name, Namespace: lrg.Namespace}
			Expect(testClient.Get(ctx, key, lrg)).To(Succeed())
			lrg.Spec.Groups = singleGroup("group-v2", `rate({app="b"} [1m]) > 0`)
			Expect(testClient.Update(ctx, lrg)).To(Succeed())

			Eventually(func() bool {
				return suiteLokiMock.HasGroup(ns, "group-v2")
			}, timeout, interval).Should(BeTrue(), "updated group should be synced")

			Eventually(func() bool {
				return suiteLokiMock.HasGroup(ns, "group-v1")
			}, timeout, interval).Should(BeFalse(), "stale group should be pruned")
		})
	})

	// ── delete ────────────────────────────────────────────────────────────

	Context("when a LokiRuleGroup is deleted", func() {
		It("removes rule groups from Loki via finalizer cleanup", func() {
			ns := "default"
			cfg := newClientConfig("loki-delete", ns, "http://loki:3100")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })
			suiteLokiCache.SetClient("loki-delete", suiteLokiMock)

			lrg := newLRG("lrg-delete", ns, "loki-delete",
				singleGroup("delete-group", `rate({app="del"} [1m]) > 0`))
			Expect(testClient.Create(ctx, lrg)).To(Succeed())

			Eventually(func() bool {
				return suiteLokiMock.HasGroup(ns, "delete-group")
			}, timeout, interval).Should(BeTrue())

			Expect(testClient.Delete(ctx, lrg)).To(Succeed())

			Eventually(func() bool {
				return suiteLokiMock.HasGroup(ns, "delete-group")
			}, timeout, interval).Should(BeFalse(), "rule group should be removed via finalizer")
		})
	})

	// ── custom lokiNamespace ──────────────────────────────────────────────

	Context("when spec.lokiNamespace is set to a custom value", func() {
		It("syncs rules into the custom namespace, not the K8s namespace", func() {
			ns := "default"
			cfg := newClientConfig("loki-customns", ns, "http://loki:3100")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })
			suiteLokiCache.SetClient("loki-customns", suiteLokiMock)

			lrg := newLRG("lrg-custom-ns", ns, "loki-customns",
				singleGroup("custom-ns-group", `rate({app="x"} [1m]) > 0`))
			lrg.Spec.LokiNamespace = "platform-alerts"
			Expect(testClient.Create(ctx, lrg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, lrg) })

			By("rule should appear in 'platform-alerts', not in 'default'")
			Eventually(func() bool {
				return suiteLokiMock.HasGroup("platform-alerts", "custom-ns-group")
			}, timeout, interval).Should(BeTrue())
			Expect(suiteLokiMock.HasGroup("default", "custom-ns-group")).To(BeFalse())

			By("status should show resolved loki namespace")
			key := types.NamespacedName{Name: lrg.Name, Namespace: lrg.Namespace}
			Eventually(func() string {
				obj := &openawarenessv1beta1.LokiRuleGroup{}
				if err := testClient.Get(ctx, key, obj); err != nil {
					return ""
				}
				return obj.Status.ResolvedLokiNamespace
			}, timeout, interval).Should(Equal("platform-alerts"))
		})
	})

	// ── namespace change cleans up old location ─────────────────────────

	Context("when spec.lokiNamespace is changed", func() {
		It("deletes rules from the old namespace and syncs into the new one", func() {
			ns := "default"
			cfg := newClientConfig("loki-nschange", ns, "http://loki:3100")
			Expect(testClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, cfg) })
			suiteLokiCache.SetClient("loki-nschange", suiteLokiMock)

			lrg := newLRG("lrg-nschange", ns, "loki-nschange",
				singleGroup("migrate-group", `rate({app="m"} [1m]) > 0`))
			lrg.Spec.LokiNamespace = "old-ns"
			Expect(testClient.Create(ctx, lrg)).To(Succeed())
			DeferCleanup(func() { _ = testClient.Delete(ctx, lrg) })

			By("waiting for rule to appear in old-ns")
			Eventually(func() bool {
				return suiteLokiMock.HasGroup("old-ns", "migrate-group")
			}, timeout, interval).Should(BeTrue())

			By("waiting for status to record active namespace")
			key := types.NamespacedName{Name: lrg.Name, Namespace: lrg.Namespace}
			Eventually(func() string {
				obj := &openawarenessv1beta1.LokiRuleGroup{}
				if err := testClient.Get(ctx, key, obj); err != nil {
					return ""
				}
				return obj.Status.ResolvedLokiNamespace
			}, timeout, interval).Should(Equal("old-ns"))

			By("changing lokiNamespace to new-ns")
			Expect(testClient.Get(ctx, key, lrg)).To(Succeed())
			lrg.Spec.LokiNamespace = "new-ns"
			Expect(testClient.Update(ctx, lrg)).To(Succeed())

			By("rule should appear in new-ns")
			Eventually(func() bool {
				return suiteLokiMock.HasGroup("new-ns", "migrate-group")
			}, timeout, interval).Should(BeTrue())

			By("rule should be gone from old-ns")
			Eventually(func() bool {
				return suiteLokiMock.HasGroup("old-ns", "migrate-group")
			}, timeout, interval).Should(BeFalse(), "orphaned rule in old namespace must be cleaned up")
		})
	})
})
