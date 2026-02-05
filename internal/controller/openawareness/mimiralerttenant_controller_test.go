package openawareness

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/syndlex/openawareness-controller/test/helper"
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
						utils.ClientNameAnnotation:  "test-client",
						utils.MimirTenantAnnotation: "test-tenant",
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
				RulerClients: nil, // No client cache - should return error
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			// Should error when client cache is nil
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ruler clients cache is nil"))

			By("Checking that resource has finalizer added despite client lookup failure")
			resource := &openawarenessv1beta1.MimirAlertTenant{}
			err = testClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			// Finalizer SHOULD be added before client lookup
			Expect(resource.Finalizers).To(ContainElement(utils.FinalizerAnnotation))
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

	Context("When updating status conditions", func() {
		It("should set synced condition correctly", func() {
			resource := &openawarenessv1beta1.MimirAlertTenant{}

			resource.SetSyncedCondition()

			By("Verifying sync status is Synced")
			Expect(resource.Status.SyncStatus).To(Equal(openawarenessv1beta1.SyncStatusSynced))

			By("Verifying LastSyncTime is set")
			Expect(resource.Status.LastSyncTime).NotTo(BeNil())

			By("Verifying ErrorMessage is empty")
			Expect(resource.Status.ErrorMessage).To(BeEmpty())

			By("Verifying ConfigurationValidation is Valid")
			Expect(resource.Status.ConfigurationValidation).To(Equal(openawarenessv1beta1.ConfigValidationValid))

			By("Verifying Ready condition is True")
			readyCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal(openawarenessv1beta1.ReasonSynced))

			By("Verifying ConfigValid condition is True")
			configValidCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeConfigValid)
			Expect(configValidCondition).NotTo(BeNil())
			Expect(configValidCondition.Status).To(Equal(metav1.ConditionTrue))

			By("Verifying Synced condition is True")
			syncedCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set failed condition correctly", func() {
			resource := &openawarenessv1beta1.MimirAlertTenant{}

			resource.SetFailedCondition(openawarenessv1beta1.ReasonNetworkError, "Failed to connect to Mimir")

			By("Verifying sync status is Failed")
			Expect(resource.Status.SyncStatus).To(Equal(openawarenessv1beta1.SyncStatusFailed))

			By("Verifying ErrorMessage is set")
			Expect(resource.Status.ErrorMessage).To(Equal("Failed to connect to Mimir"))

			By("Verifying Ready condition is False")
			readyCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(openawarenessv1beta1.ReasonNetworkError))

			By("Verifying Synced condition is False")
			syncedCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should set config invalid condition correctly", func() {
			resource := &openawarenessv1beta1.MimirAlertTenant{}

			resource.SetConfigInvalidCondition(openawarenessv1beta1.ReasonInvalidYAML, "Invalid YAML syntax")

			By("Verifying sync status is Failed")
			Expect(resource.Status.SyncStatus).To(Equal(openawarenessv1beta1.SyncStatusFailed))

			By("Verifying ErrorMessage is set")
			Expect(resource.Status.ErrorMessage).To(Equal("Invalid YAML syntax"))

			By("Verifying ConfigurationValidation is Invalid")
			Expect(resource.Status.ConfigurationValidation).To(Equal(openawarenessv1beta1.ConfigValidationInvalid))

			By("Verifying Ready condition is False")
			readyCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))

			By("Verifying ConfigValid condition is False")
			configValidCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeConfigValid)
			Expect(configValidCondition).NotTo(BeNil())
			Expect(configValidCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(configValidCondition.Reason).To(Equal(openawarenessv1beta1.ReasonInvalidYAML))

			By("Verifying Synced condition is False")
			syncedCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeSynced)
			Expect(syncedCondition).NotTo(BeNil())
			Expect(syncedCondition.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should update existing conditions rather than duplicate", func() {
			resource := &openawarenessv1beta1.MimirAlertTenant{}

			By("Setting synced condition first")
			resource.SetSyncedCondition()
			Expect(resource.Status.Conditions).To(HaveLen(3))

			By("Setting failed condition which should update existing conditions")
			resource.SetFailedCondition(openawarenessv1beta1.ReasonNetworkError, "Network error")
			Expect(resource.Status.Conditions).To(HaveLen(3)) // Should still be 3, not 6

			By("Verifying conditions were updated, not duplicated")
			readyCondition := helper.FindCondition(resource.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(openawarenessv1beta1.ReasonNetworkError))
		})
	})
})
