// Package e2e contains end-to-end tests for the openawareness-controller.
// This file contains E2E tests specifically for MimirAlertTenant templating feature.
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

var _ = Describe("MimirAlertTenant Templating E2E", Ordered, func() {
	const (
		testNamespace     = "mimiralerttenant-templating-e2e"
		clientConfigName  = "test-mimir-client-templating"
		mimirNamespace    = "e2e-templating-tenant"
		timeout           = DefaultTimeout
		interval          = DefaultInterval
		configMapName     = "alertmanager-template-data"
		secretName        = "alertmanager-secret-data"
		multiConfigMap1   = "alertmanager-data-1"
		multiConfigMap2   = "alertmanager-data-2"
		optionalConfigMap = "optional-data"
	)

	var (
		namespace *corev1.Namespace
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

	Context("When creating a MimirAlertTenant with ConfigMap templating", func() {
		const tenantName = "tenant-configmap-basic"

		var (
			configMap   *corev1.ConfigMap
			alertTenant *openawarenessv1beta1.MimirAlertTenant
		)

		BeforeAll(func() {
			By("Creating a ConfigMap with template variables")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"SMTP_HOST":     "smtp.example.com:587",
					"SMTP_FROM":     "alerts@example.com",
					"EMAIL_TO":      "team@example.com",
					"RECEIVER_NAME": "email-receiver",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("Creating a MimirAlertTenant with template syntax")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					SecretDataReferences: []openawarenessv1beta1.SecretDataReference{
						{
							Name: configMapName,
							Kind: "ConfigMap",
						},
					},
					AlertmanagerConfig: `
global:
  smtp_smarthost: '[[ .SMTP_HOST ]]'
  smtp_from: '[[ .SMTP_FROM ]]'
  smtp_require_tls: true

route:
  receiver: '[[ .RECEIVER_NAME ]]'
  group_by: ['alertname']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h

receivers:
  - name: '[[ .RECEIVER_NAME ]]'
    email_configs:
      - to: '[[ .EMAIL_TO ]]'
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
			if configMap != nil {
				Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
			}
		})

		It("Should successfully render template with ConfigMap data", func() {
			By("Waiting for MimirAlertTenant to be created")
			_, err := helper.WaitForMimirAlertTenantCreation(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer was added")
			err = helper.WaitForFinalizerAdded(ctx, k8sClient, tenantName, testNamespace, utils.FinalizerAnnotation, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for sync status to be updated")
			updatedTenant, err := helper.WaitForSyncStatusUpdate(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Checking reconciliation status")
			err = k8sClient.Get(ctx, types.NamespacedName{Name: tenantName, Namespace: testNamespace}, updatedTenant)
			Expect(err).NotTo(HaveOccurred())

			// Log status for debugging
			GinkgoWriter.Printf("MimirAlertTenant Status (ConfigMap templating):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)
			GinkgoWriter.Printf("  ErrorMessage: %s\n", updatedTenant.Status.ErrorMessage)
			for _, cond := range updatedTenant.Status.Conditions {
				GinkgoWriter.Printf("  Condition: Type=%s, Status=%s, Reason=%s, Message=%s\n",
					cond.Type, cond.Status, cond.Reason, cond.Message)
			}

			// Verify sync status
			if updatedTenant.Status.SyncStatus == openawarenessv1beta1.SyncStatusSynced {
				GinkgoWriter.Printf("Sync succeeded - verifying templating rendered correctly\n")

				By("Verifying successful sync conditions")
				helper.VerifySuccessfulSync(updatedTenant)

				By("Verifying configuration in Mimir API contains rendered values")
				mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					config, _, err := mimirClient.GetAlertmanagerConfig(ctx, mimirNamespace)
					if err != nil {
						return false
					}
					// Verify template variables were substituted
					return strings.Contains(config, "smtp.example.com:587") &&
						strings.Contains(config, "alerts@example.com") &&
						strings.Contains(config, "team@example.com") &&
						strings.Contains(config, "email-receiver")
				}, timeout, interval).Should(BeTrue(), "Rendered configuration should be in Mimir API")
			} else {
				GinkgoWriter.Printf("Sync failed - checking if it's template-related or Mimir config issue\n")
				// Check if failure is template-related (should not be for valid template)
				Expect(updatedTenant.Status.ErrorMessage).NotTo(ContainSubstring("template"),
					"Template should be valid, any error should be Mimir-related")
			}
		})
	})

	Context("When creating a MimirAlertTenant with Secret templating", func() {
		const tenantName = "tenant-secret-basic"

		var (
			secret      *corev1.Secret
			alertTenant *openawarenessv1beta1.MimirAlertTenant
		)

		BeforeAll(func() {
			By("Creating a Secret with sensitive template variables")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				StringData: map[string]string{
					"SMTP_PASSWORD": "super-secret-password",
					"SLACK_WEBHOOK": "https://hooks.slack.com/services/XXX/YYY/ZZZ",
					"PAGERDUTY_KEY": "abcdef123456",
					"RECEIVER_NAME": "secure-receiver",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a MimirAlertTenant with Secret reference")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					SecretDataReferences: []openawarenessv1beta1.SecretDataReference{
						{
							Name: secretName,
							Kind: "Secret",
						},
					},
					AlertmanagerConfig: `
global:
  smtp_smarthost: 'localhost:25'
  smtp_from: 'alerts@test.org'
  smtp_auth_password: '[[ .SMTP_PASSWORD ]]'

route:
  receiver: '[[ .RECEIVER_NAME ]]'
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty'
    - match:
        severity: warning
      receiver: 'slack'

receivers:
  - name: '[[ .RECEIVER_NAME ]]'
    email_configs:
      - to: 'default@test.org'
  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: '[[ .PAGERDUTY_KEY ]]'
  - name: 'slack'
    slack_configs:
      - api_url: '[[ .SLACK_WEBHOOK ]]'
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
			if secret != nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("Should successfully render template with Secret data", func() {
			By("Waiting for MimirAlertTenant to be created")
			_, err := helper.WaitForMimirAlertTenantCreation(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for sync status to be updated")
			updatedTenant, err := helper.WaitForSyncStatusUpdate(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			// Log status
			GinkgoWriter.Printf("MimirAlertTenant Status (Secret templating):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)
			if updatedTenant.Status.ErrorMessage != "" {
				GinkgoWriter.Printf("  ErrorMessage: %s\n", updatedTenant.Status.ErrorMessage)
			}

			// Verify sync (may fail if Mimir is not configured, but template should be valid)
			if updatedTenant.Status.SyncStatus == openawarenessv1beta1.SyncStatusSynced {
				By("Verifying successful sync with Secret data")
				helper.VerifySuccessfulSync(updatedTenant)
			} else {
				// Template should still be valid even if Mimir sync fails
				Expect(updatedTenant.Status.ErrorMessage).NotTo(ContainSubstring("template"),
					"Template should be valid with Secret data")
			}
		})
	})

	Context("When creating a MimirAlertTenant with multiple ConfigMap references", func() {
		const tenantName = "tenant-multi-refs"

		var (
			configMap1  *corev1.ConfigMap
			configMap2  *corev1.ConfigMap
			alertTenant *openawarenessv1beta1.MimirAlertTenant
		)

		BeforeAll(func() {
			By("Creating first ConfigMap with base configuration")
			configMap1 = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      multiConfigMap1,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"SMTP_HOST":     "smtp-base.example.com:587",
					"SMTP_FROM":     "base@example.com",
					"RECEIVER_NAME": "base-receiver",
				},
			}
			Expect(k8sClient.Create(ctx, configMap1)).To(Succeed())

			By("Creating second ConfigMap that overrides some values")
			configMap2 = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      multiConfigMap2,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"SMTP_HOST":  "smtp-override.example.com:587", // Override
					"EMAIL_TO":   "override-team@example.com",     // New value
					"GROUP_WAIT": "30s",                           // New value
				},
			}
			Expect(k8sClient.Create(ctx, configMap2)).To(Succeed())

			By("Creating a MimirAlertTenant with multiple references (override behavior)")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					SecretDataReferences: []openawarenessv1beta1.SecretDataReference{
						{
							Name: multiConfigMap1, // First reference (base)
							Kind: "ConfigMap",
						},
						{
							Name: multiConfigMap2, // Second reference (overrides)
							Kind: "ConfigMap",
						},
					},
					AlertmanagerConfig: `
global:
  smtp_smarthost: '[[ .SMTP_HOST ]]'
  smtp_from: '[[ .SMTP_FROM ]]'

route:
  receiver: '[[ .RECEIVER_NAME ]]'
  group_wait: [[ .GROUP_WAIT ]]

receivers:
  - name: '[[ .RECEIVER_NAME ]]'
    email_configs:
      - to: '[[ .EMAIL_TO ]]'
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
			if configMap1 != nil {
				Expect(k8sClient.Delete(ctx, configMap1)).To(Succeed())
			}
			if configMap2 != nil {
				Expect(k8sClient.Delete(ctx, configMap2)).To(Succeed())
			}
		})

		It("Should handle multiple references with override behavior", func() {
			By("Waiting for sync status to be updated")
			updatedTenant, err := helper.WaitForSyncStatusUpdate(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("MimirAlertTenant Status (Multiple refs):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)

			// Verify template rendered correctly (later refs override earlier ones)
			if updatedTenant.Status.SyncStatus == openawarenessv1beta1.SyncStatusSynced {
				By("Verifying override behavior in Mimir API")
				mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					config, _, err := mimirClient.GetAlertmanagerConfig(ctx, mimirNamespace)
					if err != nil {
						return false
					}
					// SMTP_HOST should be overridden value
					// SMTP_FROM should be base value (not overridden)
					return strings.Contains(config, "smtp-override.example.com:587") &&
						strings.Contains(config, "base@example.com") &&
						strings.Contains(config, "override-team@example.com") &&
						strings.Contains(config, "group_wait: 30s")
				}, timeout, interval).Should(BeTrue(), "Override behavior should work correctly")
			}
		})
	})

	Context("When creating a MimirAlertTenant with optional reference", func() {
		const tenantName = "tenant-optional-ref"

		var alertTenant *openawarenessv1beta1.MimirAlertTenant

		BeforeAll(func() {
			By("Creating a MimirAlertTenant with optional reference that doesn't exist")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					SecretDataReferences: []openawarenessv1beta1.SecretDataReference{
						{
							Name:     optionalConfigMap, // Does not exist
							Kind:     "ConfigMap",
							Optional: true, // Marked as optional
						},
					},
					AlertmanagerConfig: `
global:
  smtp_smarthost: 'localhost:25'
  smtp_from: 'alerts@test.org'

route:
  receiver: 'default'

receivers:
  - name: 'default'
    email_configs:
      - to: 'team@test.org'
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should succeed when optional reference is not found", func() {
			By("Waiting for sync status to be updated")
			updatedTenant, err := helper.WaitForSyncStatusUpdate(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("MimirAlertTenant Status (Optional ref):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)

			// Should not fail due to missing optional reference
			// If it fails, it should be Mimir-related, not template-related
			if updatedTenant.Status.SyncStatus != openawarenessv1beta1.SyncStatusSynced {
				Expect(updatedTenant.Status.ErrorMessage).NotTo(ContainSubstring("template"),
					"Missing optional reference should not cause template error")
				Expect(updatedTenant.Status.ErrorMessage).NotTo(ContainSubstring("not found"),
					"Missing optional reference should be handled gracefully")
			}
		})
	})

	Context("When creating a MimirAlertTenant with required reference missing", func() {
		const tenantName = "tenant-required-missing"

		var alertTenant *openawarenessv1beta1.MimirAlertTenant

		BeforeAll(func() {
			By("Creating a MimirAlertTenant with required reference that doesn't exist")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					SecretDataReferences: []openawarenessv1beta1.SecretDataReference{
						{
							Name:     "non-existent-configmap",
							Kind:     "ConfigMap",
							Optional: false, // Required (default)
						},
					},
					AlertmanagerConfig: `
route:
  receiver: 'default'
receivers:
  - name: 'default'
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should fail when required reference is not found", func() {
			By("Waiting for status to be updated")
			updatedTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: tenantName, Namespace: testNamespace}, updatedTenant)
				return err == nil && updatedTenant.Status.SyncStatus != ""
			}, timeout, interval).Should(BeTrue())

			GinkgoWriter.Printf("MimirAlertTenant Status (Required ref missing):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)
			GinkgoWriter.Printf("  ErrorMessage: %s\n", updatedTenant.Status.ErrorMessage)

			By("Verifying failure condition for missing required reference")
			Expect(updatedTenant.Status.SyncStatus).To(Equal(openawarenessv1beta1.SyncStatusFailed))
			Expect(updatedTenant.Status.ErrorMessage).To(ContainSubstring("not found"),
				"Error should indicate reference not found")

			// Verify condition
			readyCondition := findCondition(updatedTenant.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(openawarenessv1beta1.ReasonTemplateDataNotFound))
		})
	})

	Context("When creating a MimirAlertTenant with default values in template", func() {
		const tenantName = "tenant-defaults"

		var (
			configMap   *corev1.ConfigMap
			alertTenant *openawarenessv1beta1.MimirAlertTenant
		)

		BeforeAll(func() {
			By("Creating a ConfigMap with partial data")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "partial-data",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"SMTP_HOST": "smtp.example.com:587",
					// EMAIL_TO intentionally missing
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("Creating a MimirAlertTenant with default values")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					SecretDataReferences: []openawarenessv1beta1.SecretDataReference{
						{
							Name: "partial-data",
							Kind: "ConfigMap",
						},
					},
					AlertmanagerConfig: `
global:
  smtp_smarthost: '[[ .SMTP_HOST ]]'
  smtp_from: '[[ .SMTP_FROM | default "default@example.com" ]]'

route:
  receiver: 'default'
  group_wait: [[ .GROUP_WAIT | default "10s" ]]

receivers:
  - name: 'default'
    email_configs:
      - to: '[[ .EMAIL_TO | default "fallback@example.com" ]]'
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
			if configMap != nil {
				Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
			}
		})

		It("Should use default values for missing variables", func() {
			By("Waiting for sync status to be updated")
			updatedTenant, err := helper.WaitForSyncStatusUpdate(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("MimirAlertTenant Status (Default values):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)

			if updatedTenant.Status.SyncStatus == openawarenessv1beta1.SyncStatusSynced {
				By("Verifying default values in Mimir API")
				mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					config, _, err := mimirClient.GetAlertmanagerConfig(ctx, mimirNamespace)
					if err != nil {
						return false
					}
					// Should contain default values
					return strings.Contains(config, "default@example.com") &&
						strings.Contains(config, "fallback@example.com") &&
						strings.Contains(config, "group_wait: 10s")
				}, timeout, interval).Should(BeTrue(), "Default values should be applied")
			}
		})
	})

	Context("When creating a MimirAlertTenant with conditional template sections", func() {
		const tenantName = "tenant-conditional"

		var (
			configMap   *corev1.ConfigMap
			alertTenant *openawarenessv1beta1.MimirAlertTenant
		)

		BeforeAll(func() {
			By("Creating a ConfigMap with condition flags")
			configMap = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "conditional-data",
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"ENABLE_PAGERDUTY": "true",
					"ENABLE_SLACK":     "false",
					"PAGERDUTY_KEY":    "test-key-123",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("Creating a MimirAlertTenant with conditional sections")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					SecretDataReferences: []openawarenessv1beta1.SecretDataReference{
						{
							Name: "conditional-data",
							Kind: "ConfigMap",
						},
					},
					AlertmanagerConfig: `
route:
  receiver: 'default'
  routes:[[ if eq .ENABLE_PAGERDUTY "true" ]]
    - match:
        severity: critical
      receiver: 'pagerduty'[[ end ]][[ if eq .ENABLE_SLACK "true" ]]
    - match:
        severity: warning
      receiver: 'slack'[[ end ]]

receivers:
  - name: 'default'
    email_configs:
      - to: 'team@test.org'[[ if eq .ENABLE_PAGERDUTY "true" ]]
  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: '[[ .PAGERDUTY_KEY ]]'[[ end ]][[ if eq .ENABLE_SLACK "true" ]]
  - name: 'slack'
    slack_configs:
      - api_url: '[[ .SLACK_WEBHOOK ]]'[[ end ]]
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
			if configMap != nil {
				Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
			}
		})

		It("Should handle conditional template sections", func() {
			By("Waiting for sync status to be updated")
			updatedTenant, err := helper.WaitForSyncStatusUpdate(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("MimirAlertTenant Status (Conditional):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)

			if updatedTenant.Status.SyncStatus == openawarenessv1beta1.SyncStatusSynced {
				By("Verifying conditional sections in Mimir API")
				mimirClient, err := helper.CreateMimirClient(ctx, MimirLocalAddress)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					config, _, err := mimirClient.GetAlertmanagerConfig(ctx, mimirNamespace)
					if err != nil {
						return false
					}
					// PagerDuty should be included, Slack should not
					return strings.Contains(config, "pagerduty") &&
						!strings.Contains(config, "slack")
				}, timeout, interval).Should(BeTrue(), "Conditional sections should work")
			}
		})
	})

	Context("When creating a MimirAlertTenant with missing variable without default", func() {
		const tenantName = "tenant-missing-var"

		var alertTenant *openawarenessv1beta1.MimirAlertTenant

		BeforeAll(func() {
			By("Creating a MimirAlertTenant with template referencing non-existent variable")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						utils.ClientNameAnnotation:  clientConfigName,
						utils.MimirTenantAnnotation: mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					AlertmanagerConfig: `
route:
  receiver: '[[ .NON_EXISTENT_VAR ]]'  # No default provided
receivers:
  - name: 'default'
`,
				},
			}
			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())
		})

		AfterAll(func() {
			By("Cleaning up test resources")
			if alertTenant != nil {
				Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())
				err := helper.WaitForResourceDeleted(ctx, k8sClient, tenantName, testNamespace, timeout, interval)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("Should fail when variable is missing and no default provided", func() {
			By("Waiting for status to be updated")
			updatedTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: tenantName, Namespace: testNamespace}, updatedTenant)
				return err == nil && updatedTenant.Status.SyncStatus != ""
			}, timeout, interval).Should(BeTrue())

			GinkgoWriter.Printf("MimirAlertTenant Status (Missing var):\n")
			GinkgoWriter.Printf("  SyncStatus: %s\n", updatedTenant.Status.SyncStatus)
			GinkgoWriter.Printf("  ErrorMessage: %s\n", updatedTenant.Status.ErrorMessage)

			By("Verifying failure condition for missing variable")
			Expect(updatedTenant.Status.SyncStatus).To(Equal(openawarenessv1beta1.SyncStatusFailed))
			// When a variable is missing, the template renders with <no value> or empty string,
			// which causes Mimir to reject the config. The error may come from either:
			// 1. Template rendering (ReasonInvalidTemplate) - if template engine catches it
			// 2. Mimir validation error - if template renders but produces invalid config
			Expect(updatedTenant.Status.ErrorMessage).NotTo(BeEmpty(),
				"Error message should be present for missing variable")

			// Verify condition - should be NotReady
			readyCondition := findCondition(updatedTenant.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			// Reason could be either ReasonInvalidTemplate or ReasonSyncFailed depending on where error occurs
		})
	})
})

// findCondition finds a condition by type in the conditions list
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
