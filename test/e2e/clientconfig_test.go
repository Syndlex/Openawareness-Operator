// Package e2e contains end-to-end tests for the openawareness-controller.
// See test/e2e/README.md for comprehensive test documentation.
package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/internal/controller/utils"
	"github.com/syndlex/openawareness-controller/test/helper"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("ClientConfig E2E", Ordered, func() {
	const (
		testNamespace = ClientConfigTestNamespace
		timeout       = DefaultTimeout
		interval      = DefaultInterval
	)

	var namespace *corev1.Namespace

	BeforeAll(func() {
		var err error

		By("Creating test namespace")
		namespace, err = helper.CreateNamespace(ctx, k8sClient, testNamespace, timeout, interval)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("Cleaning up test namespace")
		if namespace != nil {
			err := helper.DeleteNamespace(ctx, k8sClient, namespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("When creating a ClientConfig with valid Mimir endpoint", func() {
		const clientConfigName = "valid-mimir-client"

		It("Should update status to Connected", func() {
			By("Creating a ClientConfig with valid Mimir address")
			_, err := helper.CreateClientConfig(
				ctx, k8sClient,
				clientConfigName, testNamespace,
				MimirGatewayAddress,
				openawarenessv1beta1.Mimir,
				map[string]string{
					utils.MimirTenantAnnotation: "test-tenant",
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for finalizer to be added")
			err = helper.WaitForClientConfigFinalizerAdded(ctx, k8sClient, clientConfigName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ConnectionStatus is Connected")
			clientConfig, err := helper.WaitForConnectionStatus(ctx, k8sClient, clientConfigName, testNamespace, "Connected", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status conditions")
			helper.VerifyConnectedStatus(clientConfig)

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When creating a ClientConfig with invalid URL", func() {
		const clientConfigName = "invalid-url-client"

		It("Should update status to Disconnected with InvalidURL reason", func() {
			By("Creating a ClientConfig with invalid URL")
			_, err := helper.CreateClientConfig(
				ctx, k8sClient,
				clientConfigName, testNamespace,
				"://invalid-url-format",
				openawarenessv1beta1.Mimir,
				map[string]string{
					utils.MimirTenantAnnotation: "test-tenant",
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for status conditions to be set")
			clientConfig, err := helper.WaitForConditionsSet(ctx, k8sClient, clientConfigName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows disconnection")
			helper.VerifyDisconnectedStatus(clientConfig, "InvalidURL")

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When creating a ClientConfig with unreachable host", func() {
		const clientConfigName = "unreachable-host-client"

		It("Should update status to Disconnected with network error", func() {
			By("Creating a ClientConfig with unreachable host")
			_, err := helper.CreateClientConfig(
				ctx, k8sClient,
				clientConfigName, testNamespace,
				"http://unreachable-host-12345.local:9009",
				openawarenessv1beta1.Mimir,
				map[string]string{
					utils.MimirTenantAnnotation: "test-tenant",
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for status conditions to be set")
			clientConfig, err := helper.WaitForConditionsSet(ctx, k8sClient, clientConfigName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows network error")
			helper.VerifyDisconnectedStatus(clientConfig, "")

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When updating a failed ClientConfig to valid configuration", func() {
		const clientConfigName = "recovery-test-client"

		It("Should transition from Disconnected to Connected", func() {
			By("Creating a ClientConfig with invalid URL initially")
			_, err := helper.CreateClientConfig(
				ctx, k8sClient,
				clientConfigName, testNamespace,
				"://invalid-initially",
				openawarenessv1beta1.Mimir,
				nil,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for initial Disconnected status")
			_, err = helper.WaitForConnectionStatus(ctx, k8sClient, clientConfigName, testNamespace, "Disconnected", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Updating ClientConfig with valid URL")
			err = helper.UpdateClientConfigAddress(ctx, k8sClient, clientConfigName, testNamespace, MimirGatewayAddress, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Adding required annotation")
			err = helper.AddClientConfigAnnotation(ctx, k8sClient, clientConfigName, testNamespace, utils.MimirTenantAnnotation, "test-tenant", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for ConnectionStatus to transition to Connected")
			clientConfig, err := helper.WaitForConnectionStatus(ctx, k8sClient, clientConfigName, testNamespace, "Connected", timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying status shows successful connection")
			helper.VerifyConnectedStatus(clientConfig)

			By("Cleaning up")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())
		})
	})

	Context("When deleting a ClientConfig", func() {
		const clientConfigName = "deletion-test-client"

		It("Should clean up properly via finalizer", func() {
			By("Creating a ClientConfig")
			clientConfig, err := helper.CreateClientConfig(
				ctx, k8sClient,
				clientConfigName, testNamespace,
				MimirGatewayAddress,
				openawarenessv1beta1.Mimir,
				map[string]string{
					utils.MimirTenantAnnotation: "test-tenant",
				},
			)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for finalizer to be added")
			err = helper.WaitForClientConfigFinalizerAdded(ctx, k8sClient, clientConfigName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the ClientConfig")
			Expect(k8sClient.Delete(ctx, clientConfig)).To(Succeed())

			By("Waiting for ClientConfig to be fully deleted")
			err = helper.WaitForResourceDeleted(ctx, k8sClient, clientConfigName, testNamespace, timeout, interval)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
