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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
)

// CreateClientConfig creates a ClientConfig resource for testing.
// It returns the created resource or an error if creation fails.
func CreateClientConfig(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace, address string,
	clientType openawarenessv1beta1.ClientType,
	annotations map[string]string,
) (*openawarenessv1beta1.ClientConfig, error) {
	clientConfig := &openawarenessv1beta1.ClientConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: openawarenessv1beta1.ClientConfigSpec{
			Address: address,
			Type:    clientType,
		},
	}

	if err := k8sClient.Create(ctx, clientConfig); err != nil {
		return nil, err
	}

	return clientConfig, nil
}

// WaitForClientConfigCreation waits for a ClientConfig to be created and returns it.
func WaitForClientConfigCreation(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) (*openawarenessv1beta1.ClientConfig, error) {
	clientConfig := &openawarenessv1beta1.ClientConfig{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() error {
		return k8sClient.Get(ctx, namespacedName, clientConfig)
	}, timeout, interval).Should(Succeed(), "ClientConfig should be created")

	return clientConfig, nil
}

// WaitForClientConfigFinalizerAdded waits for a finalizer to be added to a ClientConfig.
func WaitForClientConfigFinalizerAdded(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) error {
	clientConfig := &openawarenessv1beta1.ClientConfig{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() bool {
		if err := k8sClient.Get(ctx, namespacedName, clientConfig); err != nil {
			return false
		}
		return len(clientConfig.Finalizers) > 0
	}, timeout, interval).Should(BeTrue(), "Finalizer should be added to ClientConfig")

	return nil
}

// WaitForConnectionStatus waits for ClientConfig ConnectionStatus to reach the expected value.
func WaitForConnectionStatus(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	expectedStatus openawarenessv1beta1.ConnectionStatus,
	timeout, interval time.Duration,
) (*openawarenessv1beta1.ClientConfig, error) {
	clientConfig := &openawarenessv1beta1.ClientConfig{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() openawarenessv1beta1.ConnectionStatus {
		if err := k8sClient.Get(ctx, namespacedName, clientConfig); err != nil {
			return ""
		}
		return clientConfig.Status.ConnectionStatus
	}, timeout, interval).Should(Equal(expectedStatus), "ConnectionStatus should be "+string(expectedStatus))

	return clientConfig, nil
}

// WaitForConditionsSet waits for conditions to be set in ClientConfig status.
func WaitForConditionsSet(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) (*openawarenessv1beta1.ClientConfig, error) {
	clientConfig := &openawarenessv1beta1.ClientConfig{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() bool {
		if err := k8sClient.Get(ctx, namespacedName, clientConfig); err != nil {
			return false
		}
		return len(clientConfig.Status.Conditions) > 0
	}, timeout, interval).Should(BeTrue(), "Conditions should be set")

	return clientConfig, nil
}

// WaitForClientConfigDeleted waits for a ClientConfig to be fully deleted from Kubernetes.
func WaitForClientConfigDeleted(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	timeout, interval time.Duration,
) error {
	clientConfig := &openawarenessv1beta1.ClientConfig{}
	namespacedName := types.NamespacedName{Name: name, Namespace: namespace}

	Eventually(func() bool {
		err := k8sClient.Get(ctx, namespacedName, clientConfig)
		return err != nil && client.IgnoreNotFound(err) == nil
	}, timeout, interval).Should(BeTrue(), "ClientConfig should be fully deleted")

	return nil
}

// UpdateClientConfigAddress updates the address of a ClientConfig.
// It handles potential update conflicts by retrying.
func UpdateClientConfigAddress(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	newAddress string,
	timeout, interval time.Duration,
) error {
	Eventually(func() error {
		clientConfig := &openawarenessv1beta1.ClientConfig{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, clientConfig); err != nil {
			return err
		}

		clientConfig.Spec.Address = newAddress
		return k8sClient.Update(ctx, clientConfig)
	}, timeout, interval).Should(Succeed(), "Should update ClientConfig address")

	return nil
}

// AddClientConfigAnnotation adds an annotation to a ClientConfig.
// It handles potential update conflicts by retrying.
func AddClientConfigAnnotation(
	ctx context.Context,
	k8sClient client.Client,
	name, namespace string,
	key, value string,
	timeout, interval time.Duration,
) error {
	Eventually(func() error {
		clientConfig := &openawarenessv1beta1.ClientConfig{}
		if err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, clientConfig); err != nil {
			return err
		}

		if clientConfig.Annotations == nil {
			clientConfig.Annotations = make(map[string]string)
		}
		clientConfig.Annotations[key] = value
		return k8sClient.Update(ctx, clientConfig)
	}, timeout, interval).Should(Succeed(), "Should add annotation to ClientConfig")

	return nil
}

// VerifyConnectedStatus verifies that a ClientConfig has Connected status with proper conditions.
func VerifyConnectedStatus(clientConfig *openawarenessv1beta1.ClientConfig) {
	Expect(clientConfig.Status.ConnectionStatus).To(Equal(openawarenessv1beta1.ConnectionStatusConnected))
	Expect(clientConfig.Status.Conditions).NotTo(BeEmpty())

	readyCondition := FindCondition(clientConfig.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
	Expect(readyCondition).NotTo(BeNil())
	Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
	Expect(readyCondition.Reason).To(Equal(openawarenessv1beta1.ReasonConnected))

	Expect(clientConfig.Status.LastConnectionTime).NotTo(BeNil())
	Expect(clientConfig.Status.ErrorMessage).To(BeEmpty())
}

// VerifyDisconnectedStatus verifies that a ClientConfig has Disconnected status with proper conditions.
func VerifyDisconnectedStatus(clientConfig *openawarenessv1beta1.ClientConfig, expectedReason string) {
	Expect(clientConfig.Status.ConnectionStatus).To(Equal(openawarenessv1beta1.ConnectionStatusDisconnected))
	Expect(clientConfig.Status.Conditions).NotTo(BeEmpty())

	readyCondition := FindCondition(clientConfig.Status.Conditions, openawarenessv1beta1.ConditionTypeReady)
	Expect(readyCondition).NotTo(BeNil())
	Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))

	if expectedReason != "" {
		Expect(readyCondition.Reason).To(Equal(expectedReason))
	}

	Expect(clientConfig.Status.ErrorMessage).NotTo(BeEmpty())
}
