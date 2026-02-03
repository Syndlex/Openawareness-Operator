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
	"k8s.io/apimachinery/pkg/util/intstr"
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

// CreateSimplePrometheusRule creates a PrometheusRule with a single alert rule for testing.
func CreateSimplePrometheusRule(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	clientName, tenant string,
) (*monitoringv1.PrometheusRule, error) {
	groups := []monitoringv1.RuleGroup{
		{
			Name: "test-alerts",
			Rules: []monitoringv1.Rule{
				{
					Alert: "TestAlert",
					Expr:  intstr.FromString("up == 0"),
					Labels: map[string]string{
						"severity": "warning",
					},
					Annotations: map[string]string{
						"summary": "Test alert",
					},
				},
			},
		},
	}

	return CreatePrometheusRule(ctx, k8sClient, name, namespace, clientName, tenant, groups)
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
			if finalizer == utils.MyFinalizerName {
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

// AddPrometheusRuleAnnotation adds an annotation to a PrometheusRule.
// It handles potential update conflicts by retrying.
func AddPrometheusRuleAnnotation(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	key, value string,
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

		if prometheusRule.Annotations == nil {
			prometheusRule.Annotations = make(map[string]string)
		}
		prometheusRule.Annotations[key] = value

		return k8sClient.Update(ctx, prometheusRule)
	}, timeout, interval).Should(Succeed(), "Should add annotation to PrometheusRule")

	return nil
}

// VerifyMimirRuleGroup verifies that a rule group exists in Mimir API.
func VerifyMimirRuleGroup(
	ctx context.Context,
	mimirClient *mimir.MimirClient,
	namespace, groupName string,
	timeout, interval time.Duration,
) error {
	return verifyMimirRuleGroupCondition(
		ctx, mimirClient, namespace, groupName, timeout, interval,
		func(group *rulefmt.RuleGroup) bool { return true },
		"Rule group '%s' should exist in Mimir namespace '%s'",
	)
}

// VerifyMimirRuleGroupDeleted verifies that a rule group has been deleted from Mimir API.
func VerifyMimirRuleGroupDeleted(
	ctx context.Context,
	mimirClient *mimir.MimirClient,
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
	mimirClient *mimir.MimirClient,
	namespace, groupName string,
	expectedRuleCount int,
	timeout, interval time.Duration,
) error {
	return verifyMimirRuleGroupCondition(
		ctx, mimirClient, namespace, groupName, timeout, interval,
		func(group *rulefmt.RuleGroup) bool { return len(group.Rules) == expectedRuleCount },
		"Rule group '%s' should have %d rules in Mimir namespace '%s'",
		expectedRuleCount,
	)
}

// verifyMimirRuleGroupCondition is a helper that reduces duplication in Mimir verification functions.
func verifyMimirRuleGroupCondition(
	ctx context.Context,
	mimirClient *mimir.MimirClient,
	namespace, groupName string,
	timeout, interval time.Duration,
	condition func(*rulefmt.RuleGroup) bool,
	messageFormat string,
	messageArgs ...interface{},
) error {
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

// GetPrometheusRule retrieves a PrometheusRule resource.
func GetPrometheusRule(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
) (*monitoringv1.PrometheusRule, error) {
	prometheusRule := &monitoringv1.PrometheusRule{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, prometheusRule)
	if err != nil {
		return nil, err
	}
	return prometheusRule, nil
}

// BuildAlertRule creates a monitoringv1.Rule with an alert configuration.
func BuildAlertRule(name, expr string, labels, annotations map[string]string) monitoringv1.Rule {
	return monitoringv1.Rule{
		Alert:       name,
		Expr:        intstr.FromString(expr),
		Labels:      labels,
		Annotations: annotations,
	}
}

// BuildRecordingRule creates a monitoringv1.Rule with a recording rule configuration.
func BuildRecordingRule(record, expr string) monitoringv1.Rule {
	return monitoringv1.Rule{
		Record: record,
		Expr:   intstr.FromString(expr),
	}
}

// BuildRuleGroup creates a monitoringv1.RuleGroup with the specified name and rules.
func BuildRuleGroup(name string, rules ...monitoringv1.Rule) monitoringv1.RuleGroup {
	return monitoringv1.RuleGroup{
		Name:  name,
		Rules: rules,
	}
}
