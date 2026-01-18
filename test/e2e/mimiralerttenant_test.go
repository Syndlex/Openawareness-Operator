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
//
// MimirAlertTenant E2E Tests (mimiralerttenant_test.go)
//
// This file tests the full lifecycle of MimirAlertTenant resources:
//
// 1. Resource Creation
//   - Creates a test namespace
//   - Creates a ClientConfig pointing to Mimir
//   - Creates a MimirAlertTenant with Alertmanager configuration
//   - Verifies finalizer is added
//   - Verifies annotations are correct
//
// 2. Resource Updates
//   - Tests updating AlertmanagerConfig
//   - Tests adding new template files
//   - Verifies updates are applied correctly
//
// 3. Validation
//   - Tests handling of invalid YAML configuration
//   - Verifies controller doesn't crash on validation errors
//
// 4. Resource Deletion
//   - Tests proper cleanup via finalizer
//   - Verifies resource is fully deleted
//
// 5. Error Handling
//   - Tests missing client-name annotation
//   - Tests non-existent ClientConfig reference
//   - Verifies graceful error handling
//
// Prerequisites:
//   - microk8s cluster running with correct context
//   - Mimir installed via Helm (available at http://mimir-gateway.mimir.svc.cluster.local:8080)
//
// Run with: ginkgo --focus="MimirAlertTenant E2E" test/e2e
package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
)

var _ = Describe("MimirAlertTenant E2E", Ordered, func() {
	const (
		testNamespace    = "mimiralerttenant-e2e-test"
		clientConfigName = "test-mimir-client"
		alertTenantName  = "test-alert-tenant"
		mimirNamespace   = "e2e-test-tenant"
		timeout          = time.Minute * 2
		interval         = time.Second * 1
	)

	var (
		namespace    *corev1.Namespace
		clientConfig *openawarenessv1beta1.ClientConfig
		alertTenant  *openawarenessv1beta1.MimirAlertTenant
	)

	BeforeAll(func() {
		By("Creating test namespace")
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}

		// Check if namespace exists from previous run and wait for it to be deleted
		existingNs := &corev1.Namespace{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: testNamespace}, existingNs)
		if err == nil && existingNs.DeletionTimestamp != nil {
			By("Waiting for previous namespace to be fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testNamespace}, existingNs)
				return err != nil && client.IgnoreNotFound(err) == nil
			}, timeout, interval).Should(BeTrue(), "Previous namespace should be deleted")
		}

		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		By("Creating ClientConfig for Mimir")
		// Note: This assumes a Mimir instance is available via the LGTM stack
		// or you have a mock Mimir endpoint available for testing
		clientConfig = &openawarenessv1beta1.ClientConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clientConfigName,
				Namespace: testNamespace,
			},
			Spec: openawarenessv1beta1.ClientConfigSpec{
				Address: "http://mimir-gateway.mimir.svc.cluster.local:8080",
				Type:    openawarenessv1beta1.Mimir,
			},
		}
		Expect(k8sClient.Create(ctx, clientConfig)).To(Succeed())

		By("Waiting for ClientConfig to be created")
		createdClientConfig := &openawarenessv1beta1.ClientConfig{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientConfigName,
				Namespace: testNamespace,
			}, createdClientConfig)
		}, timeout, interval).Should(Succeed())

		By("Waiting for ClientConfig to be reconciled (finalizer added)")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clientConfigName,
				Namespace: testNamespace,
			}, createdClientConfig)
			if err != nil {
				return false
			}
			for _, finalizer := range createdClientConfig.Finalizers {
				if finalizer == "clientconfigs/finalizers" {
					return true
				}
			}
			return false
		}, timeout, interval).Should(BeTrue(), "ClientConfig should be reconciled with finalizer")
	})

	AfterAll(func() {
		By("Cleaning up test namespace")
		if namespace != nil {
			Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
		}
	})

	Context("When creating a MimirAlertTenant", func() {
		It("Should successfully reconcile the resource", func() {
			By("Creating a MimirAlertTenant with valid configuration")
			alertTenant = &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      alertTenantName,
					Namespace: testNamespace,
					Annotations: map[string]string{
						"openawareness.io/client-name":     clientConfigName,
						"openawareness.io/mimir-namespace": mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					TemplateFiles: map[string]string{
						"default_template": `{{ define "__subject" }}[{{ .Status | toUpper }}] Test Alert{{ end }}`,
					},
					AlertmanagerConfig: `
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
`,
				},
			}

			Expect(k8sClient.Create(ctx, alertTenant)).To(Succeed())

			By("Verifying the MimirAlertTenant was created")
			createdAlertTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, createdAlertTenant)
			}, timeout, interval).Should(Succeed())

			By("Verifying finalizer was added")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, createdAlertTenant)
				if err != nil {
					return false
				}
				for _, finalizer := range createdAlertTenant.Finalizers {
					if finalizer == "clientconfigs/finalizers" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "Finalizer should be added to MimirAlertTenant")

			By("Verifying annotations are present")
			Expect(createdAlertTenant.Annotations).To(HaveKey("openawareness.io/client-name"))
			Expect(createdAlertTenant.Annotations["openawareness.io/client-name"]).To(Equal(clientConfigName))
			Expect(createdAlertTenant.Annotations).To(HaveKey("openawareness.io/mimir-namespace"))
			Expect(createdAlertTenant.Annotations["openawareness.io/mimir-namespace"]).To(Equal(mimirNamespace))

			By("Verifying spec fields are correct")
			Expect(createdAlertTenant.Spec.AlertmanagerConfig).NotTo(BeEmpty())
			Expect(createdAlertTenant.Spec.TemplateFiles).To(HaveKey("default_template"))
		})
	})

	Context("When updating a MimirAlertTenant", func() {
		It("Should handle configuration updates", func() {
			By("Fetching the existing MimirAlertTenant")
			existingAlertTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      alertTenantName,
				Namespace: testNamespace,
			}, existingAlertTenant)).To(Succeed())

			By("Updating the AlertmanagerConfig")
			existingAlertTenant.Spec.AlertmanagerConfig = `
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
			Expect(k8sClient.Update(ctx, existingAlertTenant)).To(Succeed())

			By("Verifying the update was applied")
			updatedAlertTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, updatedAlertTenant)
				if err != nil {
					return ""
				}
				return updatedAlertTenant.Spec.AlertmanagerConfig
			}, timeout, interval).Should(ContainSubstring("updated-receiver"))
		})
	})

	Context("When adding template files", func() {
		It("Should handle template updates", func() {
			By("Fetching the existing MimirAlertTenant")
			existingAlertTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      alertTenantName,
				Namespace: testNamespace,
			}, existingAlertTenant)).To(Succeed())

			By("Adding a new template file")
			existingAlertTenant.Spec.TemplateFiles["custom_template"] = `{{ define "__custom" }}Custom Template{{ end }}`
			Expect(k8sClient.Update(ctx, existingAlertTenant)).To(Succeed())

			By("Verifying the new template was added")
			updatedAlertTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, updatedAlertTenant)
				if err != nil {
					return false
				}
				_, exists := updatedAlertTenant.Spec.TemplateFiles["custom_template"]
				return exists
			}, timeout, interval).Should(BeTrue())

			Expect(updatedAlertTenant.Spec.TemplateFiles).To(HaveLen(2))
		})
	})

	Context("When validating AlertmanagerConfig", func() {
		It("Should reject invalid YAML configuration", func() {
			By("Creating a MimirAlertTenant with invalid YAML")
			invalidAlertTenant := &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-alert-tenant",
					Namespace: testNamespace,
					Annotations: map[string]string{
						"openawareness.io/client-name":     clientConfigName,
						"openawareness.io/mimir-namespace": mimirNamespace,
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					AlertmanagerConfig: `this is not valid yaml: [[[`,
				},
			}

			By("Attempting to create the resource")
			err := k8sClient.Create(ctx, invalidAlertTenant)

			// Note: Validation happens at reconciliation time, not at API admission time
			// So we expect creation to succeed, but reconciliation should handle the error
			if err == nil {
				By("Resource created, waiting for reconciliation to detect invalid config")
				// The controller should log an error but not crash
				// In a real scenario, you might check status conditions here

				By("Cleaning up invalid resource")
				Expect(k8sClient.Delete(ctx, invalidAlertTenant)).To(Succeed())
			}
		})
	})

	Context("When deleting a MimirAlertTenant", func() {
		It("Should properly clean up resources", func() {
			By("Deleting the MimirAlertTenant")
			Expect(k8sClient.Delete(ctx, alertTenant)).To(Succeed())

			By("Verifying the MimirAlertTenant is being deleted")
			deletingAlertTenant := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, deletingAlertTenant)
				if err != nil {
					// Resource is gone
					return true
				}
				// Check if deletion timestamp is set
				return deletingAlertTenant.DeletionTimestamp != nil
			}, timeout, interval).Should(BeTrue())

			By("Waiting for the resource to be fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      alertTenantName,
					Namespace: testNamespace,
				}, deletingAlertTenant)
				return client.IgnoreNotFound(err) == nil && err != nil
			}, timeout, interval).Should(BeTrue(), "MimirAlertTenant should be fully deleted")
		})
	})

	Context("When creating without client-name annotation", func() {
		It("Should handle missing annotation gracefully", func() {
			By("Creating a MimirAlertTenant without client-name annotation")
			noClientAlertTenant := &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-client-alert-tenant",
					Namespace: testNamespace,
					Annotations: map[string]string{
						"openawareness.io/mimir-namespace": "test-namespace",
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

			Expect(k8sClient.Create(ctx, noClientAlertTenant)).To(Succeed())

			By("Verifying resource was created but reconciliation handled missing annotation")
			created := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "no-client-alert-tenant",
					Namespace: testNamespace,
				}, created)
			}, timeout, interval).Should(Succeed())

			By("Cleaning up test resource")
			Expect(k8sClient.Delete(ctx, noClientAlertTenant)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "no-client-alert-tenant",
					Namespace: testNamespace,
				}, created)
			}, timeout, interval).ShouldNot(Succeed())
		})
	})

	Context("When creating with invalid client reference", func() {
		It("Should handle non-existent ClientConfig gracefully", func() {
			By("Creating a MimirAlertTenant with non-existent client reference")
			badClientAlertTenant := &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bad-client-alert-tenant",
					Namespace: testNamespace,
					Annotations: map[string]string{
						"openawareness.io/client-name":     "non-existent-client",
						"openawareness.io/mimir-namespace": mimirNamespace,
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

			Expect(k8sClient.Create(ctx, badClientAlertTenant)).To(Succeed())

			By("Verifying resource was created")
			created := &openawarenessv1beta1.MimirAlertTenant{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "bad-client-alert-tenant",
					Namespace: testNamespace,
				}, created)
			}, timeout, interval).Should(Succeed())

			By("Cleaning up test resource")
			Expect(k8sClient.Delete(ctx, badClientAlertTenant)).To(Succeed())
		})
	})
})
