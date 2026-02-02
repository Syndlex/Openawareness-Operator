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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateNamespace creates a test namespace.
// It handles the case where a namespace from a previous run is still being deleted.
func CreateNamespace(
	ctx context.Context,
	k8sClient client.Client,
	name string,
	timeout, interval time.Duration,
) (*corev1.Namespace, error) {
	// Check if namespace exists from previous run and wait for it to be deleted
	existingNs := &corev1.Namespace{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, existingNs)
	if err == nil && existingNs.DeletionTimestamp != nil {
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, existingNs)
			return err != nil && client.IgnoreNotFound(err) == nil
		}, timeout, interval).Should(BeTrue(), "Previous namespace should be deleted")
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := k8sClient.Create(ctx, namespace); err != nil {
		return nil, err
	}

	return namespace, nil
}

// DeleteNamespace deletes a test namespace and waits for it to be fully removed.
func DeleteNamespace(
	ctx context.Context,
	k8sClient client.Client,
	namespace *corev1.Namespace,
	timeout, interval time.Duration,
) error {
	if namespace == nil {
		return nil
	}

	if err := k8sClient.Delete(ctx, namespace); err != nil {
		return client.IgnoreNotFound(err)
	}

	// Wait for namespace to be fully deleted
	Eventually(func() bool {
		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: namespace.Name}, ns)
		return err != nil && client.IgnoreNotFound(err) == nil
	}, timeout, interval).Should(BeTrue(), "Namespace should be deleted")

	return nil
}

// WaitForDeletionTimestamp waits for a resource to have its DeletionTimestamp set.
func WaitForDeletionTimestamp(
	ctx context.Context,
	k8sClient client.Client,
	obj client.Object,
	timeout, interval time.Duration,
) error {
	namespacedName := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	Eventually(func() bool {
		if err := k8sClient.Get(ctx, namespacedName, obj); err != nil {
			// Resource is gone
			return true
		}
		// Check if deletion timestamp is set
		return obj.GetDeletionTimestamp() != nil
	}, timeout, interval).Should(BeTrue(), "DeletionTimestamp should be set")

	return nil
}
