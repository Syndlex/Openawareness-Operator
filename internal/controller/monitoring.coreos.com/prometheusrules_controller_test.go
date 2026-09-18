package monitoringcoreoscom

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"github.com/syndlex/openawareness-controller/internal/loki"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("PrometheusRules Controller", func() {
	const (
		ruleName      = "test-prometheus-rule"
		ruleNamespace = "default"
		clientName    = "test-client"
		tenantID      = "test-tenant"
	)

	var (
		ctx                context.Context
		clientCache        *clients.MockRulerClientCache
		fakeRecorder       *record.FakeRecorder
		reconciler         *PrometheusRulesReconciler
		prometheusRule     *monitoringv1.PrometheusRule
		typeNamespacedName types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		clientCache = clients.NewMockRulerClientCache()
		fakeRecorder = record.NewFakeRecorder(100)

		reconciler = &PrometheusRulesReconciler{
			RulerClients: clientCache,
			LokiClients:  clients.NewMockLokiClientCache(),
			Client:       k8sClient,
			Scheme:       k8sClient.Scheme(),
			Recorder:     fakeRecorder,
		}

		typeNamespacedName = types.NamespacedName{
			Name:      ruleName,
			Namespace: ruleNamespace,
		}

		prometheusRule = &monitoringv1.PrometheusRule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ruleName,
				Namespace: ruleNamespace,
				Annotations: map[string]string{
					utils.ClientNameAnnotation:  clientName,
					utils.MimirTenantAnnotation: tenantID,
				},
			},
			Spec: monitoringv1.PrometheusRuleSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "test-group",
						Rules: []monitoringv1.Rule{
							{
								Alert: "TestAlert",
								Expr:  intstr.FromString("up == 0"),
								Labels: map[string]string{
									"severity": "critical",
								},
							},
						},
					},
				},
			},
		}
	})

	Context("When reconciling a PrometheusRule", func() {
		It("should emit warning event when client annotation is missing", func() {
			// Create rule without client annotation
			ruleWithoutAnnotation := prometheusRule.DeepCopy()
			ruleWithoutAnnotation.Annotations = nil

			Expect(k8sClient.Create(ctx, ruleWithoutAnnotation)).To(Succeed())

			// Reconcile
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Verify warning event was emitted
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("ClientNotFound")))

			// Cleanup
			Expect(k8sClient.Delete(ctx, ruleWithoutAnnotation)).To(Succeed())
		})

		It("should emit warning event when client does not exist in cache", func() {
			Expect(k8sClient.Create(ctx, prometheusRule)).To(Succeed())

			// Client not in cache - will trigger error

			// Reconcile
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Verify warning event was emitted
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("ClientNotFound")))

			// Cleanup
			Expect(k8sClient.Delete(ctx, prometheusRule)).To(Succeed())
		})

		It("should handle missing tenant annotation by using default tenant", func() {
			// Create rule without tenant annotation but with client annotation
			ruleWithoutTenant := prometheusRule.DeepCopy()
			ruleWithoutTenant.Annotations = map[string]string{
				utils.ClientNameAnnotation: clientName,
			}

			Expect(k8sClient.Create(ctx, ruleWithoutTenant)).To(Succeed())

			// Reconcile - will fail because client doesn't exist, but should use default tenant
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: typeNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			// Should emit ClientNotFound event
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("ClientNotFound")))

			// Cleanup
			Expect(k8sClient.Delete(ctx, ruleWithoutTenant)).To(Succeed())
		})

		// newMimirCfg creates a ClientConfig of type mimir and registers the mock client.
		// Call DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) }) in each test.
		newMimirCfg := func(name string, mock *clients.MockAwarenessClient) *openawarenessv1beta1.ClientConfig {
			clientCache.SetClient(name, mock)
			cfg := &openawarenessv1beta1.ClientConfig{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ruleNamespace},
				Spec: openawarenessv1beta1.ClientConfigSpec{
					Address: "http://mimir:9009",
					Type:    openawarenessv1beta1.Mimir,
				},
			}
			Expect(k8sClient.Create(ctx, cfg)).To(Succeed())
			return cfg
		}

		It("should add finalizer to PrometheusRule when Mimir client exists", func() {
			mock := clients.NewMockAwarenessClient()
			cfg := newMimirCfg(clientName, mock)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := prometheusRule.DeepCopy()
			rule.Name = "prom-add-finalizer"
			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rule) })

			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			updated := &monitoringv1.PrometheusRule{}
			Expect(k8sClient.Get(ctx, nn, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(utils.FinalizerAnnotation))
		})

		It("should emit warning event when rule group creation fails", func() {
			mock := clients.NewMockAwarenessClient()
			mock.SetCreateRuleGroupError(errors.New("mimir: out of capacity"))
			cfg := newMimirCfg("mimir-create-fail", mock)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := prometheusRule.DeepCopy()
			rule.Name = "prom-create-fail"
			rule.Annotations[utils.ClientNameAnnotation] = "mimir-create-fail"
			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rule) })

			// First reconcile adds finalizer; second triggers the create.
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("RuleGroupCreateFailed")))
		})

		It("should emit warning event when rule group deletion fails", func() {
			mock := clients.NewMockAwarenessClient()
			cfg := newMimirCfg("mimir-delete-fail", mock)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := prometheusRule.DeepCopy()
			rule.Name = "prom-delete-fail"
			rule.Annotations[utils.ClientNameAnnotation] = "mimir-delete-fail"
			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())

			// Sync first (two reconciles: finalizer + actual sync), then inject delete error.
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})

			mock.SetDeleteRuleGroupError(errors.New("mimir: timeout"))
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("RuleGroupDeleteFailed")))
		})

		It("should emit normal event when rule groups are synced successfully", func() {
			mock := clients.NewMockAwarenessClient()
			cfg := newMimirCfg("mimir-sync-ok", mock)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := prometheusRule.DeepCopy()
			rule.Name = "prom-sync-ok"
			rule.Annotations[utils.ClientNameAnnotation] = "mimir-sync-ok"
			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rule) })

			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("RuleGroupsSynced")))
		})

		It("should emit normal event when rule groups are deleted successfully", func() {
			mock := clients.NewMockAwarenessClient()
			cfg := newMimirCfg("mimir-delete-ok", mock)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := prometheusRule.DeepCopy()
			rule.Name = "prom-delete-ok"
			rule.Annotations[utils.ClientNameAnnotation] = "mimir-delete-ok"
			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})

			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("RuleGroupsDeleted")))
		})
	})

	Context("When client-name points to a Loki ClientConfig (type: loki)", func() {
		var (
			lokiMock  *clients.MockLokiClient
			lokiCache *clients.MockLokiClientCache
		)

		// newLokiRule creates a PrometheusRule whose client-name annotation points to a
		// Loki ClientConfig. No Mimir annotation is set.
		newLokiRule := func(name, cfgName string) *monitoringv1.PrometheusRule {
			return &monitoringv1.PrometheusRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: ruleNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation: cfgName,
					},
				},
				Spec: prometheusRule.Spec,
			}
		}

		newLokiCfg := func(name string) *openawarenessv1beta1.ClientConfig {
			return &openawarenessv1beta1.ClientConfig{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ruleNamespace},
				Spec: openawarenessv1beta1.ClientConfigSpec{
					Address: "http://loki:3100",
					Type:    openawarenessv1beta1.Loki,
				},
			}
		}

		BeforeEach(func() {
			lokiMock = clients.NewMockLokiClient()
			lokiCache = clients.NewMockLokiClientCache()
			lokiCache.SetClient("loki-cfg", lokiMock)
			reconciler.LokiClients = lokiCache
		})

		It("syncs rule groups to Loki when ClientConfig type is loki", func() {
			cfg := newLokiCfg("loki-cfg")
			Expect(k8sClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := newLokiRule("prom-to-loki", "loki-cfg")
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rule) })

			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("rule group should be in Loki")
			Expect(lokiMock.HasGroup(ruleNamespace, "test-group")).To(BeTrue())

			By("RuleGroupsSynced event should be emitted")
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("RuleGroupsSynced")))
		})

		It("emits ClientNotFound when ClientConfig does not exist", func() {
			rule := newLokiRule("prom-loki-missing-cfg", "loki-does-not-exist")
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, rule) })

			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("ClientNotFound")))

			By("nothing should have reached Loki")
			Expect(lokiMock.RuleCount(ruleNamespace)).To(Equal(0))
		})

		It("deletes rule groups from Loki when CR is deleted", func() {
			cfg := newLokiCfg("loki-cfg")
			Expect(k8sClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := newLokiRule("prom-loki-delete", "loki-cfg")
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())

			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(lokiMock.HasGroup(ruleNamespace, "test-group")).To(BeTrue())

			By("deleting the CR")
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			By("rule group should be removed from Loki")
			Expect(lokiMock.HasGroup(ruleNamespace, "test-group")).To(BeFalse())
		})

		It("ignores Loki 404 on delete and removes finalizer successfully", func() {
			lokiMock404 := clients.NewMockLokiClient()
			lokiCache.SetClient("loki-cfg-404", lokiMock404)

			cfg := newLokiCfg("loki-cfg-404")
			Expect(k8sClient.Create(ctx, cfg)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cfg) })

			rule := newLokiRule("prom-loki-404-delete", "loki-cfg-404")
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())

			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})

			lokiMock404.SetDeleteError(loki.ErrResourceNotFound)

			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred(), "Loki 404 on delete must not block reconciliation")

			Eventually(fakeRecorder.Events).Should(Receive(ContainSubstring("RuleGroupsDeleted")))

			final := &monitoringv1.PrometheusRule{}
			_ = k8sClient.Get(ctx, nn, final)
			Expect(final.Finalizers).NotTo(ContainElement(utils.FinalizerAnnotation))
		})

		It("emits LokiClientMissing warning when ClientConfig deleted before PrometheusRule", func() {
			cfg := newLokiCfg("loki-cfg")
			Expect(k8sClient.Create(ctx, cfg)).To(Succeed())

			rule := newLokiRule("prom-loki-orphan", "loki-cfg")
			Expect(k8sClient.Create(ctx, rule)).To(Succeed())

			nn := types.NamespacedName{Name: rule.Name, Namespace: rule.Namespace}
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
			Expect(lokiMock.HasGroup(ruleNamespace, "test-group")).To(BeTrue())

			By("deleting ClientConfig before PrometheusRule")
			Expect(k8sClient.Delete(ctx, cfg)).To(Succeed())

			By("deleting the PrometheusRule")
			Expect(k8sClient.Delete(ctx, rule)).To(Succeed())
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: nn})

			By("reconcile should not error even though ClientConfig is gone")
			Expect(err).NotTo(HaveOccurred())

			By("LokiClientMissing or RuleGroupsDeleted event should be emitted")
			Eventually(fakeRecorder.Events).Should(Receive(Or(
				ContainSubstring("LokiClientMissing"),
				ContainSubstring("RuleGroupsDeleted"),
			)))
		})
	})

	Context("When converting rule groups", func() {
		It("should convert PrometheusRule groups to Mimir format", func() {
			groups := []monitoringv1.RuleGroup{
				{
					Name: "test-group-1",
					Rules: []monitoringv1.Rule{
						{
							Alert: "TestAlert1",
							Expr:  intstr.FromString("up == 0"),
							Labels: map[string]string{
								"severity": "critical",
							},
							Annotations: map[string]string{
								"summary": "Instance is down",
							},
						},
						{
							Record: "job:up:sum",
							Expr:   intstr.FromString("sum(up) by (job)"),
						},
					},
				},
			}

			converted := convert(groups)

			Expect(converted).To(HaveLen(1))
			Expect(converted[0].Name).To(Equal("test-group-1"))
			Expect(converted[0].Rules).To(HaveLen(2))
			Expect(converted[0].Rules[0].Alert).To(Equal("TestAlert1"))
			Expect(converted[0].Rules[0].Expr).To(Equal("up == 0"))
			Expect(converted[0].Rules[1].Record).To(Equal("job:up:sum"))
		})

		It("should handle multiple rule groups", func() {
			groups := []monitoringv1.RuleGroup{
				{
					Name: "alerts",
					Rules: []monitoringv1.Rule{
						{Alert: "Alert1", Expr: intstr.FromString("up == 0")},
					},
				},
				{
					Name: "recordings",
					Rules: []monitoringv1.Rule{
						{Record: "job:up:sum", Expr: intstr.FromString("sum(up)")},
					},
				},
			}

			converted := convert(groups)

			Expect(converted).To(HaveLen(2))
			Expect(converted[0].Name).To(Equal("alerts"))
			Expect(converted[1].Name).To(Equal("recordings"))
		})
	})
})
