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

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:golint,revive
)

const (
	lgtmStackUrl   = "https://raw.githubusercontent.com/grafana/docker-otel-lgtm/refs/heads/main/k8s/lgtm.yaml"
	mimirNamespace = "mimir"
	mimirRelease   = "mimir"
	helmTimeout    = "10m"
	gatewayTimeout = "300s"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) ([]byte, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s failed with error: (%v) %s", command, err, string(output))
	}
	return output, nil
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.Replace(wd, "/test/e2e", "", -1)
	return wd, nil
}

// InstallMimir installs Grafana Mimir via Helm with a lightweight configuration for e2e tests
func InstallMimir() error {
	_, _ = fmt.Fprintf(GinkgoWriter, "Checking if Mimir is installed...\n")

	// Check if namespace exists
	cmd := exec.Command("kubectl", "get", "namespace", mimirNamespace)
	_, err := Run(cmd)

	if err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Installing Mimir via Helm (lightweight config for e2e tests)...\n")

		// Create namespace
		cmd = exec.Command("kubectl", "create", "namespace", mimirNamespace)
		if _, err := Run(cmd); err != nil {
			return fmt.Errorf("creating Mimir namespace: %w", err)
		}

		// Add Grafana Helm repo
		_, _ = fmt.Fprintf(GinkgoWriter, "Adding Grafana Helm repository...\n")
		cmd = exec.Command("helm", "repo", "add", "grafana", "https://grafana.github.io/helm-charts")
		if _, err := Run(cmd); err != nil {
			// Ignore error if repo already exists
			_, _ = fmt.Fprintf(GinkgoWriter, "Grafana repo may already exist, continuing...\n")
		}

		// Update Helm repos
		cmd = exec.Command("helm", "repo", "update")
		if _, err := Run(cmd); err != nil {
			return fmt.Errorf("updating Helm repositories: %w", err)
		}

		// Install Mimir with lightweight configuration
		_, _ = fmt.Fprintf(GinkgoWriter, "Installing Mimir chart...\n")
		cmd = exec.Command("helm", "install", mimirRelease, "grafana/mimir-distributed",
			"--namespace", mimirNamespace,
			"--set", "mimir.structuredConfig.limits.max_global_series_per_user=0",
			"--set", "mimir.structuredConfig.multitenancy_enabled=true",
			"--set", "nginx.enabled=false",
			"--set", "gateway.service.type=ClusterIP",
			"--set", "alertmanager.enabled=true",
			"--set", "alertmanager.replicas=1",
			"--set", "alertmanager.persistentVolume.enabled=false",
			"--set", "ruler.enabled=true",
			"--set", "ruler.replicas=1",
			"--set", "compactor.persistentVolume.enabled=false",
			"--set", "ingester.replicas=1",
			"--set", "ingester.persistentVolume.enabled=false",
			"--set", "ingester.zoneAwareReplication.enabled=false",
			"--set", "store_gateway.persistentVolume.enabled=false",
			"--set", "store_gateway.zoneAwareReplication.enabled=false",
			"--set", "minio.enabled=true",
			"--set", "minio.persistence.enabled=false",
			"--set", "minio.mode=standalone",
			"--set", "minio.resources.requests.memory=128Mi",
			"--set", "kafka.persistence.enabled=false",
			"--wait",
			"--timeout", helmTimeout)

		if _, err := Run(cmd); err != nil {
			return fmt.Errorf("installing Mimir via Helm: %w", err)
		}

		// Wait for gateway to be ready
		_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for Mimir gateway to be ready...\n")
		cmd = exec.Command("kubectl", "wait", "--for=condition=ready", "pod",
			"-l", "app.kubernetes.io/component=gateway",
			"-n", mimirNamespace,
			"--timeout", gatewayTimeout)

		if _, err := Run(cmd); err != nil {
			return fmt.Errorf("waiting for Mimir gateway: %w", err)
		}

		_, _ = fmt.Fprintf(GinkgoWriter, "Mimir installation complete\n")
	} else {
		_, _ = fmt.Fprintf(GinkgoWriter, "Mimir namespace already exists\n")

		// Check if Helm release exists
		cmd = exec.Command("helm", "list", "-n", mimirNamespace)
		output, err := Run(cmd)
		if err != nil {
			return fmt.Errorf("checking Helm releases: %w", err)
		}

		if !strings.Contains(string(output), mimirRelease) {
			_, _ = fmt.Fprintf(GinkgoWriter, "Warning: Mimir namespace exists but Helm release not found\n")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "Mimir is already installed\n")
		}
	}

	return nil
}
