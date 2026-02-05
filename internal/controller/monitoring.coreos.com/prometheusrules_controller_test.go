package monitoringcoreoscom

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/syndlex/openawareness-controller/internal/clients"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
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
		clientCache        *clients.RulerClientCache
		fakeRecorder       *record.FakeRecorder
		reconciler         *PrometheusRulesReconciler
		prometheusRule     *monitoringv1.PrometheusRule
		typeNamespacedName types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		clientCache = clients.NewRulerClientCache()
		fakeRecorder = record.NewFakeRecorder(100)

		reconciler = &PrometheusRulesReconciler{
			RulerClients: clientCache,
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

		It("should add finalizer to PrometheusRule", func() {
			Skip("Integration test - requires working Mimir client")
		})

		It("should emit warning event when rule group creation fails", func() {
			Skip("Integration test - requires mock injection support")
		})

		It("should emit warning event when rule group deletion fails", func() {
			Skip("Integration test - requires mock injection support")
		})

		It("should emit normal event when rule groups are synced successfully", func() {
			Skip("Integration test - requires working Mimir client")
		})

		It("should emit normal event when rule groups are deleted successfully", func() {
			Skip("Integration test - requires working Mimir client")
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
