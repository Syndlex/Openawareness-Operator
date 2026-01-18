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

package openawareness

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
)

var _ = Describe("MimirAlertTenant Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-alert-tenant"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind MimirAlertTenant")
			resource := &openawarenessv1beta1.MimirAlertTenant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
					Annotations: map[string]string{
						"openawareness.io/client-name":     "test-client",
						"openawareness.io/mimir-namespace": "test-tenant",
					},
				},
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					AlertmanagerConfig: `
global:
  smtp_smarthost: 'localhost:25'
  smtp_from: 'test@example.org'
route:
  receiver: default
receivers:
  - name: default
    email_configs:
      - to: 'team@example.org'
`,
					TemplateFiles: map[string]string{
						"default": "{{ define \"test\" }}Test{{ end }}",
					},
				},
			}
			err := testClient.Get(ctx, typeNamespacedName, &openawarenessv1beta1.MimirAlertTenant{})
			if err != nil && errors.IsNotFound(err) {
				Expect(testClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance MimirAlertTenant")
			resource := &openawarenessv1beta1.MimirAlertTenant{}
			err := testClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(testClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should handle reconciliation when client is missing", func() {
			By("Reconciling the created resource without RulerClients")
			controllerReconciler := &MimirAlertTenantReconciler{
				Client:       testClient,
				Scheme:       testClient.Scheme(),
				RulerClients: nil, // No client cache - should return early
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// Should not error when client is not found
			Expect(err).NotTo(HaveOccurred())

			By("Checking that resource exists but has no finalizer (because client lookup failed)")
			resource := &openawarenessv1beta1.MimirAlertTenant{}
			err = testClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			// Finalizer should NOT be added because we return early when client is not found
			Expect(resource.Finalizers).NotTo(ContainElement(utils.MyFinalizerName))
		})

		It("should successfully process the resource (verification test)", func() {
			By("Getting the created resource")
			resource := &openawarenessv1beta1.MimirAlertTenant{}
			err := testClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying resource was created with correct spec")
			Expect(resource.Spec.AlertmanagerConfig).NotTo(BeEmpty())
			Expect(resource.Spec.TemplateFiles).To(HaveKey("default"))
		})

		It("should validate alertmanager config", func() {
			By("Getting the created resource")
			resource := &openawarenessv1beta1.MimirAlertTenant{}
			err := testClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Validating the alertmanager configuration")
			err = resource.ValidateAlertmanagerConfig()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should convert to config DTO", func() {
			By("Getting the created resource")
			resource := &openawarenessv1beta1.MimirAlertTenant{}
			err := testClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Converting to config DTO")
			config := resource.ToConfigDTO()
			Expect(config).To(Equal(resource.Spec.AlertmanagerConfig))
		})

		It("should convert to templates DTO", func() {
			By("Getting the created resource")
			resource := &openawarenessv1beta1.MimirAlertTenant{}
			err := testClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Converting to templates DTO")
			dto := resource.ToTemplatesDTO()
			Expect(dto).To(HaveLen(1))
			Expect(dto["default"]).To(Equal("{{ define \"test\" }}Test{{ end }}"))
		})
	})

	Context("When validating invalid configurations", func() {
		It("should reject invalid YAML", func() {
			resource := &openawarenessv1beta1.MimirAlertTenant{
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					AlertmanagerConfig: "invalid: yaml: config: [",
				},
			}
			err := resource.ValidateAlertmanagerConfig()
			Expect(err).To(HaveOccurred())
		})

		It("should reject empty config", func() {
			resource := &openawarenessv1beta1.MimirAlertTenant{
				Spec: openawarenessv1beta1.MimirAlertTenantSpec{
					AlertmanagerConfig: "",
				},
			}
			err := resource.ValidateAlertmanagerConfig()
			Expect(err).To(HaveOccurred())
		})
	})
})
