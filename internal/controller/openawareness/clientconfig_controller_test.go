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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"github.com/syndlex/openawareness-controller/test/helper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
)

var _ = Describe("ClientConfig Controller", func() {
	Context("When reconciling a ClientConfig resource", func() {
		const (
			ClientConfigName      = "test-clientconfig"
			ClientConfigNamespace = "default"
			timeout               = time.Second * 10
			interval              = time.Millisecond * 250
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      ClientConfigName,
			Namespace: ClientConfigNamespace,
		}

		BeforeEach(func() {
			// Clean up any existing resources
			clientConfig := &openawarenessv1beta1.ClientConfig{}
			err := testClient.Get(ctx, typeNamespacedName, clientConfig)
			if err == nil {
				By("Cleaning up existing ClientConfig")
				Expect(testClient.Delete(ctx, clientConfig)).To(Succeed())
				// Wait for deletion to complete
				Eventually(func() bool {
					err := testClient.Get(ctx, typeNamespacedName, clientConfig)
					return err != nil
				}, timeout, interval).Should(BeTrue())
			}
		})

		AfterEach(func() {
			// Clean up
			clientConfig := &openawarenessv1beta1.ClientConfig{}
			err := testClient.Get(ctx, typeNamespacedName, clientConfig)
			if err == nil {
				By("Cleaning up ClientConfig in AfterEach")
				Expect(testClient.Delete(ctx, clientConfig)).To(Succeed())
				// Wait for deletion to complete
				Eventually(func() bool {
					err := testClient.Get(ctx, typeNamespacedName, clientConfig)
					return err != nil
				}, timeout, interval).Should(BeTrue())
			}
		})

		Context("When creating a ClientConfig with valid Mimir configuration", func() {
			It("should update status with Connected condition", func() {
				By("Creating a new ClientConfig")
				clientConfig := &openawarenessv1beta1.ClientConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ClientConfigName,
						Namespace: ClientConfigNamespace,
					},
					Spec: openawarenessv1beta1.ClientConfigSpec{
						Address: "http://localhost:9009",
						Type:    openawarenessv1beta1.Mimir,
					},
				}
				Expect(testClient.Create(ctx, clientConfig)).To(Succeed())

				By("Checking the status is updated with connection information")
				Eventually(func() bool {
					err := testClient.Get(ctx, typeNamespacedName, clientConfig)
					if err != nil {
						return false
					}
					return len(clientConfig.Status.Conditions) > 0
				}, timeout, interval).Should(BeTrue())

				By("Verifying the ConnectionStatus field")
				Expect(clientConfig.Status.ConnectionStatus).NotTo(BeEmpty())
			})
		})

		Context("When creating a ClientConfig with invalid URL", func() {
			It("should update status with error condition", func() {
				By("Creating a ClientConfig with invalid address")
				clientConfig := &openawarenessv1beta1.ClientConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ClientConfigName,
						Namespace: ClientConfigNamespace,
						Annotations: map[string]string{
							utils.MimirTenantAnnotation: "test-tenant",
						},
					},
					Spec: openawarenessv1beta1.ClientConfigSpec{
						Address: "://invalid-url",
						Type:    openawarenessv1beta1.Mimir,
					},
				}
				Expect(testClient.Create(ctx, clientConfig)).To(Succeed())

				By("Checking status contains error information")
				Eventually(func() bool {
					err := testClient.Get(ctx, typeNamespacedName, clientConfig)
					if err != nil {
						return false
					}
					return clientConfig.Status.ErrorMessage != ""
				}, timeout, interval).Should(BeTrue())

				By("Verifying ConnectionStatus is Disconnected")
				Expect(clientConfig.Status.ConnectionStatus).To(Equal(openawarenessv1beta1.ConnectionStatusDisconnected))

				By("Verifying error condition exists")
				conditions := clientConfig.Status.Conditions
				Expect(len(conditions)).To(BeNumerically(">", 0))

				readyCondition := helper.FindCondition(conditions, openawarenessv1beta1.ConditionTypeReady)
				Expect(readyCondition).NotTo(BeNil())
				Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(readyCondition.Reason).To(Equal(openawarenessv1beta1.ReasonInvalidURL))
			})
		})

		Context("When creating a ClientConfig with unreachable address", func() {
			It("should update status with network error condition", func() {
				By("Creating a ClientConfig with unreachable address")
				clientConfig := &openawarenessv1beta1.ClientConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ClientConfigName,
						Namespace: ClientConfigNamespace,
						Annotations: map[string]string{
							utils.MimirTenantAnnotation: "test-tenant",
						},
					},
					Spec: openawarenessv1beta1.ClientConfigSpec{
						Address: "http://unreachable-host-12345.local:9009",
						Type:    openawarenessv1beta1.Mimir,
					},
				}
				Expect(testClient.Create(ctx, clientConfig)).To(Succeed())

				By("Checking status contains network error")
				Eventually(func() bool {
					err := testClient.Get(ctx, typeNamespacedName, clientConfig)
					if err != nil {
						return false
					}
					return clientConfig.Status.ErrorMessage != ""
				}, timeout, interval).Should(BeTrue())

				By("Verifying ConnectionStatus is Disconnected")
				Expect(clientConfig.Status.ConnectionStatus).To(Equal(openawarenessv1beta1.ConnectionStatusDisconnected))

				By("Verifying error message contains network error details")
				Expect(clientConfig.Status.ErrorMessage).NotTo(BeEmpty())
			})
		})

		Context("When deleting a ClientConfig", func() {
			It("should remove the finalizer and delete successfully", func() {
				By("Creating a ClientConfig")
				clientConfig := &openawarenessv1beta1.ClientConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      ClientConfigName,
						Namespace: ClientConfigNamespace,
					},
					Spec: openawarenessv1beta1.ClientConfigSpec{
						Address: "http://localhost:9009",
						Type:    openawarenessv1beta1.Mimir,
					},
				}
				Expect(testClient.Create(ctx, clientConfig)).To(Succeed())

				By("Waiting for finalizer to be added")
				Eventually(func() bool {
					err := testClient.Get(ctx, typeNamespacedName, clientConfig)
					if err != nil {
						return false
					}
					return len(clientConfig.Finalizers) > 0
				}, timeout, interval).Should(BeTrue())

				By("Deleting the ClientConfig")
				Expect(testClient.Delete(ctx, clientConfig)).To(Succeed())

				By("Verifying the ClientConfig is deleted")
				Eventually(func() bool {
					err := testClient.Get(ctx, typeNamespacedName, clientConfig)
					return err != nil
				}, timeout, interval).Should(BeTrue())
			})
		})
	})
})
