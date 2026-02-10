package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/mimir"
)

// CreateMimirAlertTenant creates a MimirAlertTenant resource for testing.
// It returns the created resource or an error if creation fails.
func CreateMimirAlertTenant(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace, clientName, tenant string,
	config string,
	templates map[string]string,
) (*openawarenessv1beta1.MimirAlertTenant, error) {
	alertTenant := &openawarenessv1beta1.MimirAlertTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				utils.ClientNameAnnotation:  clientName,
				utils.MimirTenantAnnotation: tenant,
			},
		},
		Spec: openawarenessv1beta1.MimirAlertTenantSpec{
			AlertmanagerConfig: config,
			TemplateFiles:      templates,
		},
	}

	if err := k8sClient.Create(ctx, alertTenant); err != nil {
		return nil, err
	}

	return alertTenant, nil
}

// WaitForMimirAlertTenantCreation waits for a MimirAlertTenant to be created and returns it.
func WaitForMimirAlertTenantCreation(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) (*openawarenessv1beta1.MimirAlertTenant, error) {
	alertTenant := &openawarenessv1beta1.MimirAlertTenant{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() error {
		return k8sClient.Get(ctx, namespacedName, alertTenant)
	}, timeout, interval).Should(Succeed(), "MimirAlertTenant should be created")

	return alertTenant, nil
}

// WaitForFinalizerAdded waits for a finalizer to be added to a MimirAlertTenant.
func WaitForFinalizerAdded(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace, finalizer string,
	timeout, interval time.Duration,
) error {
	alertTenant := &openawarenessv1beta1.MimirAlertTenant{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() bool {
		if err := k8sClient.Get(ctx, namespacedName, alertTenant); err != nil {
			return false
		}
		for _, f := range alertTenant.Finalizers {
			if f == finalizer {
				return true
			}
		}
		return false
	}, timeout, interval).Should(BeTrue(), "Finalizer should be added to MimirAlertTenant")

	return nil
}

// WaitForSyncStatusUpdate waits for the SyncStatus to be updated (non-empty and not Pending).
func WaitForSyncStatusUpdate(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) (*openawarenessv1beta1.MimirAlertTenant, error) {
	alertTenant := &openawarenessv1beta1.MimirAlertTenant{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() bool {
		if err := k8sClient.Get(ctx, namespacedName, alertTenant); err != nil {
			return false
		}
		return alertTenant.Status.SyncStatus != "" &&
			alertTenant.Status.SyncStatus != openawarenessv1beta1.SyncStatusPending
	}, timeout, interval).Should(BeTrue(), "Status should be updated by controller")

	return alertTenant, nil
}

// WaitForResourceDeleted waits for a MimirAlertTenant to be fully deleted from Kubernetes.
func WaitForResourceDeleted(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) error {
	alertTenant := &openawarenessv1beta1.MimirAlertTenant{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() bool {
		err := k8sClient.Get(ctx, namespacedName, alertTenant)
		return err != nil && client.IgnoreNotFound(err) == nil
	}, timeout, interval).Should(BeTrue(), "MimirAlertTenant should be fully deleted")

	return nil
}

// UpdateMimirAlertTenantConfig updates the AlertmanagerConfig of a MimirAlertTenant.
// It handles potential update conflicts by retrying.
func UpdateMimirAlertTenantConfig(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	newConfig string,
	timeout, interval time.Duration,
) error {
	Eventually(func() error {
		alertTenant := &openawarenessv1beta1.MimirAlertTenant{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, alertTenant); err != nil {
			return err
		}

		alertTenant.Spec.AlertmanagerConfig = newConfig
		return k8sClient.Update(ctx, alertTenant)
	}, timeout, interval).Should(Succeed(), "Should update AlertmanagerConfig")

	return nil
}

// AddTemplateFile adds a new template file to a MimirAlertTenant.
// It handles potential update conflicts by retrying.
func AddTemplateFile(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	templateName, templateContent string,
	timeout, interval time.Duration,
) error {
	Eventually(func() error {
		alertTenant := &openawarenessv1beta1.MimirAlertTenant{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, alertTenant); err != nil {
			return err
		}

		alertTenant.Spec.TemplateFiles[templateName] = templateContent
		return k8sClient.Update(ctx, alertTenant)
	}, timeout, interval).Should(Succeed(), "Should add template file")

	return nil
}

// VerifyMimirAlertTenantAnnotations verifies that required annotations are present.
func VerifyMimirAlertTenantAnnotations(
	alertTenant *openawarenessv1beta1.MimirAlertTenant,
	expectedClientName, expectedTenant string,
) {
	Expect(alertTenant.Annotations).To(HaveKey(utils.ClientNameAnnotation))
	Expect(alertTenant.Annotations[utils.ClientNameAnnotation]).To(Equal(expectedClientName))
	Expect(alertTenant.Annotations).To(HaveKey(utils.MimirTenantAnnotation))
	Expect(alertTenant.Annotations[utils.MimirTenantAnnotation]).To(Equal(expectedTenant))
}

// VerifySuccessfulSync verifies that a MimirAlertTenant was successfully synced.
func VerifySuccessfulSync(alertTenant *openawarenessv1beta1.MimirAlertTenant) {
	Expect(alertTenant.Status.SyncStatus).To(Equal(openawarenessv1beta1.SyncStatusSynced))
	Expect(alertTenant.Status.LastSyncTime).NotTo(BeNil())
	Expect(alertTenant.Status.ConfigurationValidation).To(Equal(openawarenessv1beta1.ConfigValidationValid))

	// Verify Ready condition is True
	hasSuccessCondition := false
	for _, cond := range alertTenant.Status.Conditions {
		if cond.Type == openawarenessv1beta1.ConditionTypeReady && cond.Status == metav1.ConditionTrue {
			hasSuccessCondition = true
			break
		}
	}
	Expect(hasSuccessCondition).To(BeTrue(), "Should have a Ready=True condition")
}

// VerifyFailedSync verifies that a MimirAlertTenant sync failed as expected.
func VerifyFailedSync(alertTenant *openawarenessv1beta1.MimirAlertTenant) {
	Expect(alertTenant.Status.ErrorMessage).NotTo(BeEmpty())

	// Verify Ready condition is False
	hasFailedCondition := false
	for _, cond := range alertTenant.Status.Conditions {
		if cond.Type == openawarenessv1beta1.ConditionTypeReady && cond.Status == metav1.ConditionFalse {
			hasFailedCondition = true
			break
		}
	}
	Expect(hasFailedCondition).To(BeTrue(), "Should have a Ready=False condition")
}

// CreateMimirClient creates a Mimir client for testing API verification.
func CreateMimirClient(ctx context.Context, address, tenant string) (*mimir.Client, error) {
	cfg := mimir.Config{
		Address:  address,
		TenantID: tenant,
	}
	return mimir.New(ctx, cfg)
}

// VerifyMimirAPIConfig verifies that configuration was pushed to Mimir API.
// It checks that the config contains the expected receiver name.
func VerifyMimirAPIConfig(
	ctx context.Context,
	mimirClient *mimir.Client,
	expectedReceiver string,
	timeout, interval time.Duration,
) error {
	Eventually(func() error {
		config, _, err := mimirClient.GetAlertmanagerConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get config from Mimir: %w", err)
		}

		if !strings.Contains(config, expectedReceiver) {
			return fmt.Errorf("expected receiver '%s' not found in config", expectedReceiver)
		}

		return nil
	}, timeout, interval).Should(Succeed(), "Configuration should be present in Mimir API")

	return nil
}

// VerifyMimirAPITemplate verifies that a template was pushed to Mimir API.
func VerifyMimirAPITemplate(
	ctx context.Context,
	mimirClient *mimir.Client,
	templateName string,
	timeout, interval time.Duration,
) error {
	Eventually(func() error {
		_, templates, err := mimirClient.GetAlertmanagerConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to get templates from Mimir: %w", err)
		}

		if _, exists := templates[templateName]; !exists {
			return fmt.Errorf("expected template '%s' not found", templateName)
		}

		return nil
	}, timeout, interval).Should(Succeed(), "Template should be present in Mimir API")

	return nil
}

// VerifyMimirAPIConfigDeleted verifies that configuration was deleted from Mimir API.
// When a config is deleted, GetAlertmanagerConfig returns empty strings with nil error (not a 404 error).
func VerifyMimirAPIConfigDeleted(
	ctx context.Context,
	mimirClient *mimir.Client,
	timeout, interval time.Duration,
) error {
	Eventually(func() bool {
		config, templates, err := mimirClient.GetAlertmanagerConfig(ctx)
		if err != nil {
			// Unexpected error
			return false
		}
		// Config should be empty after deletion
		return config == "" && (templates == nil || len(templates) == 0)
	}, timeout, interval).Should(BeTrue(), "Configuration should be deleted from Mimir API")

	return nil
}
