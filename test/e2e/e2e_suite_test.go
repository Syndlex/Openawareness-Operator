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
	"os"
	"os/exec"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	openawarenessv1beta1 "github.com/syndlex/openawareness-controller/api/openawareness/v1beta1"
	"github.com/syndlex/openawareness-controller/test/helper"
	"github.com/syndlex/openawareness-controller/test/utils"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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

	err = monitoringv1.AddToScheme(scheme.Scheme)
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

	By("installing Mimir")
	err = utils.InstallMimir()
	Expect(err).NotTo(HaveOccurred(), "Failed to install Mimir")

	By("installing CRDs")
	cmd := exec.Command("make", "install")
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("installing Prometheus Operator CRDs")
	cmd = exec.Command("kubectl", "apply", "-f", "config/crd/monitoring.coreos.com_prometheusrules.yaml")
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install Prometheus Operator CRDs")

	// Use installed kubectl binary, so having kubectl in PATH is required. If this turns out to be a problem, we could download it like we do for other binaries.
	// Note that we first create a cfg from a kubeconfig and then create a kubeconfig from the cfg again. If this causes problems, we could also use the kubeconfig directly.
	kubeCtl, err = helper.New(cfg, "kubectl", logf.Log.WithName("Kubectl"))
	Expect(err).NotTo(HaveOccurred())

	// Wait for Prometheus Operator CRDs to be established
	_, err = kubeCtl.Run("wait", "--for=condition=Established", "--timeout=30s", "crd/prometheusrules.monitoring.coreos.com")
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	test = helper.ClientTestContext{
		Ctx:    ctx,
		Client: k8sClient,
	}

	// Wait for openawareness CRDs to be established
	_, err = kubeCtl.Run("wait", "--for=condition=Established", "--timeout=30s", "crd/clientconfigs.openawareness.syndlex")
	Expect(err).NotTo(HaveOccurred())
	_, err = kubeCtl.Run("wait", "--for=condition=Established", "--timeout=30s", "crd/mimirtenants.openawareness.syndlex")
	Expect(err).NotTo(HaveOccurred())
	_, err = kubeCtl.Run("wait", "--for=condition=Established", "--timeout=30s", "crd/mimiralerttenants.openawareness.syndlex")
	Expect(err).NotTo(HaveOccurred())

	By("building and deploying controller")
	// We deploy the controller in-cluster rather than running it in-process because:
	// 1. In-process controller cannot resolve Mimir service DNS (mimir-distributor.mimir.svc.cluster.local)
	//    since the test process runs outside the cluster
	// 2. In-cluster deployment provides a more realistic test environment matching production
	// 3. This approach tests the actual deployment manifests and RBAC configuration

	// Build controller image locally
	// Use simple name without registry prefix for local builds
	imageName := "openawareness-controller:e2e-test"
	cmd = exec.Command("make", "docker-build", "IMG="+imageName)
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to build controller image")

	// Tag with docker.io/library prefix for microk8s import
	// microk8s containerd expects this naming convention
	microk8sImageName := "docker.io/library/" + imageName
	cmd = exec.Command("docker", "tag", imageName, microk8sImageName)
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to tag controller image")

	// Import image directly into microk8s containerd
	// Remove any existing tar file first to avoid conflicts
	tarPath := "/tmp/openawareness-controller-e2e.tar"
	_ = os.Remove(tarPath)

	// Save the image to a tar file with microk8s naming
	cmd = exec.Command("docker", "save", microk8sImageName, "-o", tarPath)
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to save controller image")

	// Import into microk8s (imports directly into containerd)
	cmd = exec.Command("microk8s", "images", "import", tarPath)
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to import controller image into microk8s")

	// Deploy controller using the microk8s image name
	// This ensures the deployment references the correct image in containerd
	cmd = exec.Command("make", "deploy", "IMG="+microk8sImageName)
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy controller")

	// Patch deployment to use imagePullPolicy: IfNotPresent
	// This allows using the locally imported image while still allowing pulls if needed
	cmd = exec.Command("kubectl", "patch", "deployment", "-n", "openawareness-controller-system",
		"openawareness-controller-controller-manager", "--type=json",
		"-p=[{\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/0/imagePullPolicy\", \"value\": \"IfNotPresent\"}]")
	_, err = helper.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to patch controller deployment")

	// Wait for controller to be ready
	_, err = kubeCtl.Run("wait", "--for=condition=Available", "--timeout=120s", "-n", "openawareness-controller-system", "deployment/openawareness-controller-controller-manager")
	Expect(err).NotTo(HaveOccurred(), "Controller deployment did not become ready")
})

var _ = AfterSuite(func() {
	By("undeploying controller")
	cmd := exec.Command("make", "undeploy")
	_, _ = helper.Run(cmd) // Ignore errors during cleanup
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
