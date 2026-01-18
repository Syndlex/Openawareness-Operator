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

package e2e

import (
	"context"
	"fmt"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/test/helper"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"os/exec"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	ctx       context.Context
	k8sClient client.Client
	kubeCtl   *helper.Kubectl
	test      helper.ClientTestContext
)

// Run e2e tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting openawareness-controller suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx = context.WithoutCancel(context.TODO())

	var err error
	err = openawarenessv1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// set KUBECONFIG to ~/.kube/config if not set
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
		GinkgoWriter.Printf("KUBECONFIG not set, setting %s\n", kubeconfig)
		err = os.Setenv("KUBECONFIG", kubeconfig)
		Expect(err).NotTo(HaveOccurred())
	}

	// get kubeconfig from KUBECONFIG: requires KUBECONFIG to be set!
	cfg, err := ctrl.GetConfig()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	checkCurrentKubeContext()

	By("installing CRDs")
	cmd := exec.Command("make", "install")
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

	// Use installed kubectl binary, so having kubectl in PATH is required. If this turns out to be a problem, we could download it like we do for other binaries.
	// Note that we first create a cfg from a kubeconfig and then create a kubeconfig from the cfg again. If this causes problems, we could also use the kubeconfig directly.
	kubeCtl, err = helper.New(cfg, "kubectl", logf.Log.WithName("Kubectl"))
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	test = helper.ClientTestContext{
		Ctx:    ctx,
		Client: k8sClient,
	}

	_, err = kubeCtl.Run("wait", "--for=condition=Established", "--timeout=5s", "crd/geoservers.geoserver.dlr.de")
	Expect(err).NotTo(HaveOccurred())
	_, err = kubeCtl.Run("wait", "--for=condition=Established", "--timeout=5s", "crd/workspaces.geoserver.dlr.de")
	Expect(err).NotTo(HaveOccurred())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0", // disable metrics service to avoid port conflicts when running suites in parallel
		},
	})

	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()
})

// checkCurrentKubeContext checks if the current Kubernetes context is set to the expected context for e2e tests.
// This prevents running the tests against the wrong cluster by accident.
func checkCurrentKubeContext() {
	GinkgoHelper()
	configAccess := clientcmd.NewDefaultPathOptions()
	rawConfig, err := configAccess.GetStartingConfig()
	Expect(err).NotTo(HaveOccurred())

	currentContext := rawConfig.CurrentContext
	expectedContext := "microk8s"
	Expect(currentContext).To(Equal(expectedContext), "Unexpected current Kubernetes context")
}
