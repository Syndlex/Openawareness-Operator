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
// ClientConfig E2E Tests (clientconfig_test.go)
//
// This file tests the full lifecycle of ClientConfig resources with focus on status updates:
//
// 1. Successful Connection
//   - Creates a ClientConfig pointing to a valid Mimir instance
//   - Verifies ConnectionStatus is "Connected"
//   - Verifies Ready condition is True
//   - Verifies LastConnectionTime is set
//
// 2. Failed Connection - Invalid URL
//   - Creates a ClientConfig with malformed URL
//   - Verifies ConnectionStatus is "Disconnected"
//   - Verifies Ready condition is False with reason "InvalidURL"
//   - Verifies ErrorMessage contains details
//
// 3. Failed Connection - Unreachable Host
//   - Creates a ClientConfig with unreachable endpoint
//   - Verifies ConnectionStatus is "Disconnected"
//   - Verifies Ready condition is False with network error reason
//   - Verifies ErrorMessage contains connection details
//
// 4. Connection Recovery
//   - Updates a failed ClientConfig with valid URL
//   - Verifies status transitions from Disconnected to Connected
//   - Verifies conditions are updated appropriately
//
// Prerequisites:
//   - microk8s cluster running with correct context
//   - Mimir installed via Helm (available at http://mimir-gateway.mimir.svc.cluster.local:8080)
//
// Run with: ginkgo --focus="ClientConfig E2E" test/e2e
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

var _ = Describe("ClientConfig E2E", Ordered, func() {
	const (
		testNamespace = "clientconfig-e2e-test"
		timeout       = time.Minute * 2
		interval      = time.Second * 1
	)

	var namespace *corev1.Namespace

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
	})

	AfterAll(func() {
		By("Cleaning up test namespace")
		if namespace != nil {
			Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
		}
	})

	Context("When creating a ClientConfig with valid Mimir endpoint", func() {
		const clientConfigName = "valid-mimir-client"

		It("Should update status to Connected", func() {
			By("Creating a ClientConfig with valid Mimir address")
			clientConfig := &openawarenessv1beta1.ClientConfig{
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

			By("Waiting for ClientConfig to be reconciled")
			createdClientConfig := &openawarenessv1beta1.ClientConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				if err != nil {
					return false
				}
				return len(createdClientConfig.Finalizers) > 0
			}, timeout, interval).Should(BeTrue(), "Finalizer should be added")

			By("Verifying ConnectionStatus is Connected")
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				if err != nil {
					return ""
				}
				return createdClientConfig.Status.ConnectionStatus
			}, timeout, interval).Should(Equal("Connected"), "ConnectionStatus should be Connected")

			By("Verifying Ready condition is True")
			Expect(createdClientConfig.Status.Conditions).NotTo(BeEmpty())
			readyCondition := findConditionInStatus(createdClientConfig.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCondition.Reason).To(Equal("Connected"))

			By("Verifying LastConnectionTime is set")
			Expect(createdClientConfig.Status.LastConnectionTime).NotTo(BeNil())

			By("Verifying ErrorMessage is empty")
			Expect(createdClientConfig.Status.ErrorMessage).To(BeEmpty())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When creating a ClientConfig with invalid URL", func() {
		const clientConfigName = "invalid-url-client"

		It("Should update status to Disconnected with InvalidURL reason", func() {
			By("Creating a ClientConfig with invalid URL")
			clientConfig := &openawarenessv1beta1.ClientConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clientConfigName,
					Namespace: testNamespace,
				},
				Spec: openawarenessv1beta1.ClientConfigSpec{
					Address: "://invalid-url-format",
					Type:    openawarenessv1beta1.Mimir,
				},
			}
			Expect(k8sClient.Create(ctx, clientConfig)).To(Succeed())

			By("Waiting for ClientConfig to be reconciled")
			createdClientConfig := &openawarenessv1beta1.ClientConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				if err != nil {
					return false
				}
				return len(createdClientConfig.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue(), "Conditions should be set")

			By("Verifying ConnectionStatus is Disconnected")
			Expect(createdClientConfig.Status.ConnectionStatus).To(Equal("Disconnected"))

			By("Verifying Ready condition is False with InvalidURL reason")
			readyCondition := findConditionInStatus(createdClientConfig.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("InvalidURL"))

			By("Verifying ErrorMessage is set")
			Expect(createdClientConfig.Status.ErrorMessage).NotTo(BeEmpty())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When creating a ClientConfig with unreachable host", func() {
		const clientConfigName = "unreachable-host-client"

		It("Should update status to Disconnected with network error", func() {
			By("Creating a ClientConfig with unreachable host")
			clientConfig := &openawarenessv1beta1.ClientConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clientConfigName,
					Namespace: testNamespace,
				},
				Spec: openawarenessv1beta1.ClientConfigSpec{
					Address: "http://unreachable-host-12345.local:9009",
					Type:    openawarenessv1beta1.Mimir,
				},
			}
			Expect(k8sClient.Create(ctx, clientConfig)).To(Succeed())

			By("Waiting for ClientConfig to be reconciled")
			createdClientConfig := &openawarenessv1beta1.ClientConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				if err != nil {
					return false
				}
				return len(createdClientConfig.Status.Conditions) > 0
			}, timeout, interval).Should(BeTrue(), "Conditions should be set")

			By("Verifying ConnectionStatus is Disconnected")
			Expect(createdClientConfig.Status.ConnectionStatus).To(Equal("Disconnected"))

			By("Verifying Ready condition is False")
			readyCondition := findConditionInStatus(createdClientConfig.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			// Reason should be one of the network-related reasons
			Expect(readyCondition.Reason).To(SatisfyAny(
				Equal("NetworkError"),
				Equal("DNSResolutionError"),
				Equal("TimeoutError"),
			))

			By("Verifying ErrorMessage contains network error details")
			Expect(createdClientConfig.Status.ErrorMessage).NotTo(BeEmpty())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When updating a failed ClientConfig to valid configuration", func() {
		const clientConfigName = "recovery-test-client"

		It("Should transition from Disconnected to Connected", func() {
			By("Creating a ClientConfig with invalid URL initially")
			clientConfig := &openawarenessv1beta1.ClientConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clientConfigName,
					Namespace: testNamespace,
				},
				Spec: openawarenessv1beta1.ClientConfigSpec{
					Address: "://invalid-initially",
					Type:    openawarenessv1beta1.Mimir,
				},
			}
			Expect(k8sClient.Create(ctx, clientConfig)).To(Succeed())

			By("Waiting for initial Disconnected status")
			createdClientConfig := &openawarenessv1beta1.ClientConfig{}
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				if err != nil {
					return ""
				}
				return createdClientConfig.Status.ConnectionStatus
			}, timeout, interval).Should(Equal("Disconnected"))

			By("Updating ClientConfig with valid URL")
			createdClientConfig.Spec.Address = "http://mimir-gateway.mimir.svc.cluster.local:8080"
			Expect(k8sClient.Update(ctx, createdClientConfig)).To(Succeed())

			By("Waiting for ConnectionStatus to transition to Connected")
			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				if err != nil {
					return ""
				}
				return createdClientConfig.Status.ConnectionStatus
			}, timeout, interval).Should(Equal("Connected"), "ConnectionStatus should recover to Connected")

			By("Verifying Ready condition is now True")
			readyCondition := findConditionInStatus(createdClientConfig.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))

			By("Verifying ErrorMessage is cleared")
			Expect(createdClientConfig.Status.ErrorMessage).To(BeEmpty())

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When deleting a ClientConfig", func() {
		const clientConfigName = "deletion-test-client"

		It("Should clean up properly via finalizer", func() {
			By("Creating a ClientConfig")
			clientConfig := &openawarenessv1beta1.ClientConfig{
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

			By("Waiting for finalizer to be added")
			createdClientConfig := &openawarenessv1beta1.ClientConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				if err != nil {
					return false
				}
				return len(createdClientConfig.Finalizers) > 0
			}, timeout, interval).Should(BeTrue())

			By("Deleting the ClientConfig")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())

			By("Waiting for ClientConfig to be fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      clientConfigName,
					Namespace: testNamespace,
				}, createdClientConfig)
				return err != nil && client.IgnoreNotFound(err) == nil
			}, timeout, interval).Should(BeTrue(), "ClientConfig should be fully deleted")
		})
	})
})

// Helper function to find a condition by type in status
func findConditionInStatus(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
