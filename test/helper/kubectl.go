package helper

import (
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"os"
	"os/exec"
	"strings"
)

// Kubectl is a wrapper around the kubectl binary for usage in tests.
type Kubectl struct {
	binPath            string
	kubeconfigFilePath string
	logger             logr.Logger
}

// Run executes kubectl with preconfigured kubeconfig and the arguments given to this method.
func (k *Kubectl) Run(args ...string) (string, error) {
	defaultOpts := []string{"--kubeconfig", k.kubeconfigFilePath}
	allArgs := append(defaultOpts, args...)
	cmd := exec.Command(k.binPath, allArgs...)

	command := strings.Join(cmd.Args, " ")
	k.logger.Info("running command", "command", command)

	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	k.logger.V(1).Info("command output", "output", output)
	if err != nil {
		return output, fmt.Errorf("command %q failed: %w\nCommand Output: %s", command, err, output)
	}

	return output, nil
}

// New creates a new Kubectl instance using the rest.Config to reverse-engineer a kubeconfig file.
func New(cfg *rest.Config, kubectlBinPath string, logger logr.Logger) (k *Kubectl, err error) {
	// Write the kubeconfig to a temporary file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp kubeconfig file: %w", err)
	}
	defer func() {
		err = errors.Join(err, kubeconfigFile.Close())
	}()

	// if this ever causes problems, consider using controlplane.KubeConfigFromREST from controller-runtime instead
	// also see: pkg/internal/testing/controlplane/plane.go in controller-runtime
	kubeconfigContent, err := clientcmd.Write(clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"default": {
				Server:                   cfg.Host,
				CertificateAuthorityData: cfg.CAData,
				CertificateAuthority:     cfg.CAFile,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"default": {
				ClientCertificate:     cfg.CertFile,
				ClientCertificateData: cfg.CertData,
				ClientKey:             cfg.KeyFile,
				ClientKeyData:         cfg.KeyData,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"default": {
				Cluster:  "default",
				AuthInfo: "default",
			},
		},
		CurrentContext: "default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to write kubeconfig content: %w", err)
	}

	_, err = kubeconfigFile.Write(kubeconfigContent)
	if err != nil {
		return nil, fmt.Errorf("failed to write kubeconfig file: %w", err)
	}

	return &Kubectl{
		binPath:            kubectlBinPath,
		kubeconfigFilePath: kubeconfigFile.Name(),
		logger:             logger,
	}, nil
}

func (k *Kubectl) Cleanup() error {
	err := os.Remove(k.kubeconfigFilePath)
	if err != nil {
		return fmt.Errorf("failed to remove kubeconfig file: %w", err)
	}
	return nil
}
