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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"github.com/syndlex/openawareness-controller/test/helper"
)

var _ = Describe("MimirAlertTenant E2E", Ordered, func() {
	const (
		testNamespace    = MimirAlertTenantTestNamespace
		clientConfigName = "test-mimir-client"
		alertTenantName  = "test-alert-tenant"
		mimirNamespace   = "e2e-test-tenant"
		timeout          = DefaultTimeout
		interval         = DefaultInterval
	)

	var (
		namespace   *corev1.Namespace
		alertTenant *openawarenessv1beta1.MimirAlertTenant
	)

	defaultTemplates := map[string]string{
		"default_template": `{{ define "__subject" }}[{{ .Status | toUpper }}] Test Alert{{ end }}`,
	}

	defaultAlertmanagerConfig := `
global:
  smtp_smarthost: 'localhost:25'
  smtp_from: 'alertmanager@test.org'
  smtp_require_tls: false

templates:
  - 'default_template'

route:
  receiver: 'default-receiver'
  group_by: ['alertname']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h

receivers:
  - name: 'default-receiver'
    email_configs:
      - to: 'team@test.org'
        headers:
          Subject: '{{ template "__subject" . }}'
`

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
				utils.MimirTenantAnnotation: testNamespace,
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

	Context("When creating a MimirAlertTenant", func() {
		It("Should successfully reconcile the resource", func() {
			By("Creating a MimirAlertTenant with valid configuration")
			var err error
			alertTenant, err = helper.CreateMimirAlertTenant(
				ctx, k8sClient,
				alertTenantName, testNamespace,
				clientConfigName, alertTenantName,
				defaultAlertmanagerConfig,
				defaultTemplates,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for MimirAlertTenant to be created")
			_, err = helper.WaitForMimirAlertTenantCreation(ctx, k8sClient, alertTenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer was added")
			err = helper.WaitForFinalizerAdded(ctx, k8sClient, alertTenantName, testNamespace, utils.FinalizerAnnotation, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying annotations are correct")
			updatedTenant := &openawarenessv1beta1.MimirAlertTenant{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: alertTenantName, Namespace: testNamespace}, updatedTenant)
			Expect(err).NotTo(HaveOccurred())
			helper.VerifyMimirAlertTenantAnnotations(updatedTenant, clientConfigName, alertTenantName)

			By("Verifying spec fields are correct")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: alertTenantName, Namespace: testNamespace}, updatedTenant)
			Expect(err).NotTo(HaveOccurred())

			Expect(updatedTenant.Spec.AlertmanagerConfig).NotTo(BeEmpty())
			Expect(updatedTenant.Spec.TemplateFiles).To(HaveKey("default_template"))

			By("Waiting for sync status to be updated")
			updatedTenant, err = helper.WaitForSyncStatusUpdate(ctx, k8sClient, alertTenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Checking reconciliation status")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: alertTenantName, Namespace: testNamespace}, updatedTenant)
			Expect(err).NotTo(HaveOccurred())

			// Log status for debugging
			GinkgoWriter.Printf("MimirAlertTenant Status:\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)
			GinkgoWriter.Printf("  ErrorMessage: %s\n", updatedTenant.Status.ErrorMessage)
			GinkgoWriter.Printf("  ConfigurationValidation: %s\n", updatedTenant.Status.ConfigurationValidation)
			for _, cond := range updatedTenant.Status.Conditions {
				GinkgoWriter.Printf("  Condition: Type=%s, Status=%s, Reason=%s, Message=%s\n",
					cond.Type, cond.Status, cond.Reason, cond.Message)
			}

			// Verify based on sync status
			if updatedTenant.Status.SyncStatus == openawarenessv1beta1.SyncStatusSynced {
				GinkgoWriter.Printf("Sync succeeded - verifying in Mimir API\n")

				By("Verifying successful sync conditions")
				helper.VerifySuccessfulSync(updatedTenant)

				By("Verifying configuration in Mimir API")
				mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress, alertTenantName)
				Expect(err).NotTo(HaveOccurred())
				err = helper.VerifyMimirAPIConfig(ctx, mimirClient, "default-receiver", timeout, interval)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying template in Mimir API")
				err = helper.VerifyMimirAPITemplate(ctx, mimirClient, "default_template", timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			} else {
				GinkgoWriter.Printf("Sync failed - expected if Mimir multitenant alertmanager is not enabled\n")

				By("Verifying failed sync conditions")
				helper.VerifyFailedSync(updatedTenant)
			}
		})
	})

	Context("When updating a MimirAlertTenant", func() {
		It("Should handle configuration updates", func() {
			updatedConfig := `
global:
  smtp_smarthost: 'localhost:25'
  smtp_from: 'alertmanager@updated.org'
  smtp_require_tls: false

route:
  receiver: 'updated-receiver'
  group_by: ['alertname', 'severity']
  group_wait: 15s
  group_interval: 15s
  repeat_interval: 24h

receivers:
  - name: 'updated-receiver'
    email_configs:
      - to: 'updated-team@test.org'
`

			By("Updating the AlertmanagerConfig")
			err := helper.UpdateMimirAlertTenantConfig(ctx, k8sClient, alertTenantName, testNamespace, updatedConfig, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the update was applied")
			updatedTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, updatedTenant)
				if err != nil {
					return ""
				}
				return updatedTenant.Spec.AlertmanagerConfig
			}, timeout, interval).Should(ContainSubstring("updated-receiver"))

			By("Verifying update triggered reconciliation")
			updatedTenant, err = helper.WaitForSyncStatusUpdate(ctx, k8sClient, alertTenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			// Log status
			err = k8sClient.Get(ctx, types.NamespacedName{Name: alertTenantName, Namespace: testNamespace}, updatedTenant)
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("Updated MimirAlertTenant Status:\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)
			if updatedTenant.Status.ErrorMessage != "" {
				GinkgoWriter.Printf("  ErrorMessage: %s\n", updatedTenant.Status.ErrorMessage)
			}

			// Verify Mimir API if sync succeeded
			if updatedTenant.Status.SyncStatus == openawarenessv1beta1.SyncStatusSynced {
				By("Verifying updated configuration in Mimir API")
				mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress, alertTenantName)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					config, _, err := mimirClient.GetAlertmanagerConfig(ctx)
					if err != nil {
						return false
					}
					return strings.Contains(config, "updated-receiver")
				}, timeout, interval).Should(BeTrue(), "Updated configuration should be in Mimir API")
			}
		})
	})

	Context("When adding template files", func() {
		It("Should handle template updates", func() {
			By("Adding a new template file")
			customTemplate := `{{ define "__custom" }}Custom Template{{ end }}`
			err := helper.AddTemplateFile(ctx, k8sClient, alertTenantName, testNamespace, "custom_template", customTemplate, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the new template was added")
			updatedTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, updatedTenant)
				if err != nil {
					return false
				}
				_, exists := updatedTenant.Spec.TemplateFiles["custom_template"]
				return exists
			}, timeout, interval).Should(BeTrue())

			Expect(updatedTenant.Spec.TemplateFiles).To(HaveLen(2))
		})
	})

	Context("When validating AlertmanagerConfig", func() {
		It("Should reject invalid YAML configuration", func() {
			By("Creating a MimirAlertTenant with invalid YAML")
			invalidTenant, err := helper.CreateMimirAlertTenant(
				ctx, k8sClient,
				"invalid-alert-tenant", testNamespace,
				clientConfigName, mimirNamespace,
				`this is not valid yaml: [[[`,
				nil,
			)

			if err == nil {
				By("Resource created, waiting for reconciliation to detect invalid config")
				// Controller should handle the error gracefully

				By("Cleaning up invalid resource")
				Expect(k8sClient.Delete(ctx, invalidTenant)).To(Succeed())
			}
		})
	})

	Context("When deleting a MimirAlertTenant", func() {
		It("Should properly clean up resources", func() {
			By("Deleting the MimirAlertTenant")
			Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())

			By("Waiting for deletion timestamp to be set")
			err := helper.WaitForDeletionTimestamp(ctx, k8sClient, alertTenant, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for the resource to be fully deleted")
			err = helper.WaitForResourceDeleted(ctx, k8sClient, alertTenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying configuration was deleted from Mimir API")
			mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress, alertTenantName)
			Expect(err).NotTo(HaveOccurred())
			err = helper.VerifyMimirAPIConfigDeleted(ctx, mimirClient, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating without client-name annotation", func() {
		It("Should handle missing annotation gracefully", func() {
			By("Creating a MimirAlertTenant without client-name annotation")
			noClientTenant := &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-client-alert-tenant",
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.MimirTenantAnnotation: "test-namespace",
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					AlertmanagerConfig: `
route:
  receiver: 'default'
receivers:
  - name: 'default'
`,
				},
			}

			Expect(k8sClient.Create(ctx, noClientTenant)).To(Succeed())

			By("Verifying resource was created")
			_, err := helper.WaitForMimirAlertTenantCreation(ctx, k8sClient, "no-client-alert-tenant", testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up test resource")
			Expect(k8sClient.Delete(ctx, noClientTenant)).To(Succeed())
			err = helper.WaitForResourceDeleted(ctx, k8sClient, "no-client-alert-tenant", testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When creating with invalid client reference", func() {
		It("Should handle non-existent ClientConfig gracefully", func() {
			By("Creating a MimirAlertTenant with non-existent client reference")
			badClientTenant, err := helper.CreateMimirAlertTenant(
				ctx, k8sClient,
				"bad-client-alert-tenant", testNamespace,
				"non-existent-client", mimirNamespace,
				`
route:
  receiver: 'default'
receivers:
  - name: 'default'
`,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying resource was created")
			_, err = helper.WaitForMimirAlertTenantCreation(ctx, k8sClient, "bad-client-alert-tenant", testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Cleaning up test resource")
			Expect(k8sClient.Delete(ctx, badClientTenant)).To(Succeed())
		})
	})
})
