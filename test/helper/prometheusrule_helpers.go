package helper

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"github.com/syndlex/openawareness-controller/internal/mimir"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreatePrometheusRule creates a PrometheusRule resource with the specified configuration.
func CreatePrometheusRule(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	clientName, tenant string,
	groups []monitoringv1.RuleGroup,
) (*monitoringv1.PrometheusRule, error) {
	annotations := map[string]string{
		utils.ClientNameAnnotation: clientName,
	}
	// Only add tenant annotation if not empty
	if tenant != "" {
		annotations[utils.MimirTenantAnnotation] = tenant
	}

	prometheusRule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: monitoringv1.PrometheusRuleSpec{
			Groups: groups,
		},
	}

	if err := k8sClient.Create(ctx, prometheusRule); err != nil {
		return nil, err
	}

	return prometheusRule, nil
}

// WaitForPrometheusRuleCreation waits for a PrometheusRule to be created.
func WaitForPrometheusRuleCreation(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) (*monitoringv1.PrometheusRule, error) {
	prometheusRule := &monitoringv1.PrometheusRule{}
	Eventually(func() error {
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, prometheusRule)
	}, timeout, interval).Should(Succeed())

	return prometheusRule, nil
}

// WaitForPrometheusRuleFinalizerAdded waits for the finalizer to be added to a PrometheusRule.
func WaitForPrometheusRuleFinalizerAdded(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) error {
	Eventually(func() bool {
		prometheusRule := &monitoringv1.PrometheusRule{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, prometheusRule); err != nil {
			return false
		}

		for _, finalizer := range prometheusRule.GetFinalizers() {
			if finalizer == utils.FinalizerAnnotation {
				return true
			}
		}
		return false
	}, timeout, interval).Should(BeTrue(), "Finalizer should be added to PrometheusRule")

	return nil
}

// WaitForPrometheusRuleDeleted waits for a PrometheusRule to be fully deleted.
func WaitForPrometheusRuleDeleted(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) error {
	Eventually(func() bool {
		prometheusRule := &monitoringv1.PrometheusRule{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, prometheusRule)
		return err != nil && client.IgnoreNotFound(err) == nil
	}, timeout, interval).Should(BeTrue(), "PrometheusRule should be deleted")

	return nil
}

// UpdatePrometheusRuleGroups updates the rule groups in a PrometheusRule.
// It handles potential update conflicts by retrying.
func UpdatePrometheusRuleGroups(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	groups []monitoringv1.RuleGroup,
	timeout, interval time.Duration,
) error {
	Eventually(func() error {
		prometheusRule := &monitoringv1.PrometheusRule{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, prometheusRule); err != nil {
			return err
		}

		prometheusRule.Spec.Groups = groups
		return k8sClient.Update(ctx, prometheusRule)
	}, timeout, interval).Should(Succeed(), "Should update PrometheusRule groups")

	return nil
}

// VerifyMimirRuleGroup verifies that a rule group exists in Mimir API.
func VerifyMimirRuleGroup(
	ctx context.Context,
	mimirClient *mimir.Client,
	namespace, groupName string,
	timeout, interval time.Duration,
) error {
	return verifyMimirRuleGroupCondition(ctx, mimirClient, namespace, groupName, timeout, interval, func(group *rulefmt.RuleGroup) bool { return true }, "Rule group '%s' should exist in Mimir namespace '%s'")
}

// VerifyMimirRuleGroupDeleted verifies that a rule group has been deleted from Mimir API.
func VerifyMimirRuleGroupDeleted(
	ctx context.Context,
	mimirClient *mimir.Client,
	namespace, groupName string,
	timeout, interval time.Duration,
) error {
	Eventually(func() bool {
		ruleSet, err := mimirClient.ListRules(ctx, namespace)
		if err != nil {
			// If we get a 404, the namespace has no rules, which means our group is deleted
			return true
		}

		groups, exists := ruleSet[namespace]
		if !exists {
			return true
		}

		for _, group := range groups {
			if group.Name == groupName {
				return false
			}
		}
		return true
	}, timeout, interval).Should(BeTrue(),
		"Rule group '%s' should be deleted from Mimir namespace '%s'", groupName, namespace)

	return nil
}

// VerifyMimirRuleGroupContent verifies the content of a rule group in Mimir API.
func VerifyMimirRuleGroupContent(
	ctx context.Context,
	mimirClient *mimir.Client,
	namespace, groupName string,
	expectedRuleCount int,
	timeout, interval time.Duration,
) error {
	return verifyMimirRuleGroupCondition(ctx, mimirClient, namespace, groupName, timeout, interval, func(group *rulefmt.RuleGroup) bool { return len(group.Rules) == expectedRuleCount }, "Rule group '%s' should have %d rules in Mimir namespace '%s'")
}

// verifyMimirRuleGroupCondition is a helper that reduces duplication in Mimir verification functions.
func verifyMimirRuleGroupCondition(ctx context.Context, mimirClient *mimir.Client, namespace, groupName string, timeout, interval time.Duration, condition func(*rulefmt.RuleGroup) bool, messageFormat string) error {
	Eventually(func() bool {
		ruleSet, err := mimirClient.ListRules(ctx, namespace)
		if err != nil {
			return false
		}

		groups, exists := ruleSet[namespace]
		if !exists {
			return false
		}

		for _, group := range groups {
			if group.Name == groupName {
				return condition(&group)
			}
		}
		return false
	}, timeout, interval).Should(BeTrue(), messageFormat, namespace)

	return nil
}
