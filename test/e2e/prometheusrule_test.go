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

// Package e2e contains end-to-end tests for the openawareness-controller.
// See test/e2e/README.md for comprehensive test documentation.
package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"github.com/syndlex/openawareness-controller/test/helper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("PrometheusRule E2E", Ordered, func() {
	const (
		testNamespace    = "prometheusrule-e2e-test"
		clientConfigName = "test-mimir-client"
		timeout          = DefaultTimeout
		interval         = DefaultInterval
	)

	var (
		namespace *corev1.Namespace
		tenant    = testNamespace
	)

	BeforeAll(func() {
		var err error

		By("Creating test namespace")
		namespace, err = helper.CreateNamespace(ctx, k8sClient, testNamespace, timeout, interval)
		Expect(err).NotTo(HaveOccurred())

		By("Creating ClientConfig for Mimir")
		_, err = helper.CreateClientConfig(
			ctx, k8sClient,
			clientConfigName, testNamespace,
			MimirGatewayAddress,
			openawarenessv1beta1.Mimir,
			map[string]string{
				utils.MimirTenantAnnotation: tenant,
			},
		)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for ClientConfig to be reconciled")
		err = helper.WaitForClientConfigFinalizerAdded(ctx, k8sClient, clientConfigName, testNamespace, timeout, interval)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up test namespace")
		if namespace != nil {
			err := helper.DeleteNamespace(ctx, k8sClient, namespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("When creating a PrometheusRule with valid configuration", func() {
		const ruleName = "test-prometheus-rule"

		It("Should successfully sync to Mimir", func() {
			By("Creating a PrometheusRule with alert and recording rules")
			groups := []monitoringv1.RuleGroup{
				{
					Name: "test-alerts",
					Rules: []monitoringv1.Rule{
						{
							Alert: "HighErrorRate",
							Expr:  intstr.FromString("rate(http_errors_total[5m]) > 0.05"),
							Labels: map[string]string{
								"severity": "warning",
								"team":     "devops",
							},
							Annotations: map[string]string{
								"summary":     "High error rate detected",
								"description": "Error rate is above 5%",
							},
						},
					},
				},
				{
					Name: "test-recordings",
					Rules: []monitoringv1.Rule{
						{
							Record: "job:http_requests:rate5m",
							Expr:   intstr.FromString("rate(http_requests_total[5m])"),
						},
					},
				},
			}

			prometheusRule, err := helper.CreatePrometheusRule(
				ctx, k8sClient,
				ruleName, testNamespace,
				clientConfigName, tenant,
				groups,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for PrometheusRule to be created")
			_, err = helper.WaitForPrometheusRuleCreation(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer was added")
			err = helper.WaitForPrometheusRuleFinalizerAdded(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying rule groups in Mimir API")
			mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress, tenant)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "test-alerts", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "test-recordings", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying rule group content")
			err = helper.VerifyMimirRuleGroupContent(ctx, mimirClient, tenant, "test-alerts", 1, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroupContent(ctx, mimirClient, tenant, "test-recordings", 1, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, prometheusRule)).To(Succeed())

			By("Waiting for PrometheusRule to be deleted")
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying rule groups were deleted from Mimir")
			err = helper.VerifyMimirRuleGroupDeleted(ctx, mimirClient, tenant, "test-alerts", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroupDeleted(ctx, mimirClient, tenant, "test-recordings", timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a PrometheusRule without client-name annotation", func() {
		const ruleName = "no-client-rule"

		It("Should handle missing annotation gracefully", func() {
			By("Creating a PrometheusRule without client-name annotation")
			prometheusRule := &monitoringv1.PrometheusRule{
				ObjectMeta: metav1.ObjectMeta{
					Name:      ruleName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.MimirTenantAnnotation: tenant,
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
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, prometheusRule)).To(Succeed())

			By("Verifying resource was created")
			_, err := helper.WaitForPrometheusRuleCreation(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, prometheusRule)).To(Succeed())
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a PrometheusRule without mimir-tenant annotation", func() {
		const ruleName = "no-tenant-rule"

		It("Should use default tenant", func() {
			By("Creating a PrometheusRule without mimir-tenant annotation")
			groups := []monitoringv1.RuleGroup{
				{
					Name: "default-tenant-group",
					Rules: []monitoringv1.Rule{
						{
							Alert: "TestAlert",
							Expr:  intstr.FromString("up == 0"),
						},
					},
				},
			}

			prometheusRule, err := helper.CreatePrometheusRule(
				ctx, k8sClient,
				ruleName, testNamespace,
				clientConfigName, "", // Empty tenant - should use default
				groups,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Removing mimir-tenant annotation to test default behavior")
			Eventually(func() error {
				rule := &monitoringv1.PrometheusRule{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      ruleName,
					Namespace: testNamespace,
				}, rule); err != nil {
					return err
				}
				delete(rule.Annotations, utils.MimirTenantAnnotation)
				return k8sClient.Update(ctx, rule)
			}, timeout, interval).Should(Succeed())

			By("Waiting for PrometheusRule to be created")
			_, err = helper.WaitForPrometheusRuleCreation(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer was added")
			err = helper.WaitForPrometheusRuleFinalizerAdded(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, prometheusRule)).To(Succeed())
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a PrometheusRule with non-existent ClientConfig", func() {
		const ruleName = "bad-client-rule"

		It("Should handle missing ClientConfig gracefully", func() {
			By("Creating a PrometheusRule with non-existent client reference")
			prometheusRule, err := helper.CreatePrometheusRule(
				ctx, k8sClient,
				ruleName, testNamespace,
				"non-existent-client", tenant,
				[]monitoringv1.RuleGroup{
					{
						Name: "test-group",
						Rules: []monitoringv1.Rule{
							{
								Alert: "TestAlert",
								Expr:  intstr.FromString("up == 0"),
							},
						},
					},
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying resource was created")
			_, err = helper.WaitForPrometheusRuleCreation(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, prometheusRule)).To(Succeed())
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When updating a PrometheusRule", func() {
		const ruleName = "update-test-rule"

		It("Should sync updated rules to Mimir", func() {
			By("Creating initial PrometheusRule")
			groups := []monitoringv1.RuleGroup{
				{
					Name: "initial-group",
					Rules: []monitoringv1.Rule{
						{
							Alert: "InitialAlert",
							Expr:  intstr.FromString("up == 0"),
							Labels: map[string]string{
								"severity": "info",
							},
						},
					},
				},
			}

			prometheusRule, err := helper.CreatePrometheusRule(
				ctx, k8sClient,
				ruleName, testNamespace,
				clientConfigName, tenant,
				groups,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for initial rule to be synced")
			err = helper.WaitForPrometheusRuleFinalizerAdded(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress, tenant)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "initial-group", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Updating PrometheusRule with new rules")
			updatedGroups := []monitoringv1.RuleGroup{
				{
					Name: "initial-group",
					Rules: []monitoringv1.Rule{
						{
							Alert: "InitialAlert",
							Expr:  intstr.FromString("up == 0"),
							Labels: map[string]string{
								"severity": "info",
							},
						},
						{
							Alert: "NewAlert",
							Expr:  intstr.FromString("up == 1"),
							Labels: map[string]string{
								"severity": "warning",
							},
						},
					},
				},
			}

			err = helper.UpdatePrometheusRuleGroups(ctx, k8sClient, ruleName, testNamespace, updatedGroups, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying updated rules in Mimir")
			err = helper.VerifyMimirRuleGroupContent(ctx, mimirClient, tenant, "initial-group", 2, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, prometheusRule)).To(Succeed())
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating multiple PrometheusRules with same ClientConfig", func() {
		const (
			rule1Name = "multi-rule-1"
			rule2Name = "multi-rule-2"
		)

		It("Should handle multiple rules correctly", func() {
			By("Creating first PrometheusRule with group 'alerts-1'")
			groups1 := []monitoringv1.RuleGroup{
				{
					Name: "alerts-1",
					Rules: []monitoringv1.Rule{
						{
							Alert: "TestAlert1",
							Expr:  intstr.FromString("up == 0"),
							Labels: map[string]string{
								"severity": "warning",
							},
						},
					},
				},
			}
			rule1, err := helper.CreatePrometheusRule(
				ctx, k8sClient,
				rule1Name, testNamespace,
				clientConfigName, tenant,
				groups1,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Creating second PrometheusRule with group 'alerts-2'")
			groups2 := []monitoringv1.RuleGroup{
				{
					Name: "alerts-2",
					Rules: []monitoringv1.Rule{
						{
							Alert: "TestAlert2",
							Expr:  intstr.FromString("up == 1"),
							Labels: map[string]string{
								"severity": "critical",
							},
						},
					},
				},
			}
			rule2, err := helper.CreatePrometheusRule(
				ctx, k8sClient,
				rule2Name, testNamespace,
				clientConfigName, tenant,
				groups2,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for both rules to be synced")
			err = helper.WaitForPrometheusRuleFinalizerAdded(ctx, k8sClient, rule1Name, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.WaitForPrometheusRuleFinalizerAdded(ctx, k8sClient, rule2Name, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying both rule groups exist in Mimir")
			mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress, tenant)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "alerts-1", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "alerts-2", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up first rule")
			Expect(k8sClient.Delete(ctx, rule1)).To(Succeed())
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, rule1Name, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying first rule group was deleted")
			err = helper.VerifyMimirRuleGroupDeleted(ctx, mimirClient, tenant, "alerts-1", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying second rule group still exists")
			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "alerts-2", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up second rule")
			Expect(k8sClient.Delete(ctx, rule2)).To(Succeed())
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, rule2Name, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying second rule group was deleted")
			err = helper.VerifyMimirRuleGroupDeleted(ctx, mimirClient, tenant, "alerts-2", timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating a PrometheusRule with complex alert rules", func() {
		const ruleName = "complex-rule"

		It("Should handle complex rules correctly", func() {
			By("Creating PrometheusRule with multiple alert types")
			groups := []monitoringv1.RuleGroup{
				{
					Name: "complex-alerts",
					Rules: []monitoringv1.Rule{
						{
							Alert: "CriticalAlert",
							Expr:  intstr.FromString("rate(errors_total[5m]) > 0.1"),
							Labels: map[string]string{
								"severity":    "critical",
								"team":        "devops",
								"environment": "production",
							},
							Annotations: map[string]string{
								"summary":     "Critical error rate",
								"description": "Error rate exceeded 10%",
								"runbook":     "https://wiki.example.com/runbooks/errors",
							},
						},
						{
							Alert: "WarningAlert",
							Expr:  intstr.FromString("rate(errors_total[5m]) > 0.05"),
							Labels: map[string]string{
								"severity": "warning",
								"team":     "devops",
							},
							Annotations: map[string]string{
								"summary": "Elevated error rate",
							},
						},
					},
				},
				{
					Name: "recording-rules",
					Rules: []monitoringv1.Rule{
						{
							Record: "instance:requests:rate5m",
							Expr:   intstr.FromString("rate(requests_total[5m])"),
						},
						{
							Record: "instance:errors:rate5m",
							Expr:   intstr.FromString("rate(errors_total[5m])"),
						},
					},
				},
			}

			prometheusRule, err := helper.CreatePrometheusRule(
				ctx, k8sClient,
				ruleName, testNamespace,
				clientConfigName, tenant,
				groups,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for rules to be synced")
			err = helper.WaitForPrometheusRuleFinalizerAdded(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying rule groups in Mimir")
			mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress, tenant)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "complex-alerts", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroupContent(ctx, mimirClient, tenant, "complex-alerts", 2, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroup(ctx, mimirClient, tenant, "recording-rules", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			err = helper.VerifyMimirRuleGroupContent(ctx, mimirClient, tenant, "recording-rules", 2, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, prometheusRule)).To(Succeed())
			err = helper.WaitForPrometheusRuleDeleted(ctx, k8sClient, ruleName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
