# End-to-End (E2E) Tests for openawareness-controller

This directory contains end-to-end tests for the openawareness-controller, including tests for the MimirAlertTenant CRD.

## Prerequisites

### Required Tools
- Go 1.23.0 or later
- kubectl
- Helm 3.x
- microk8s (or another Kubernetes cluster)

### Cluster Setup

The e2e tests expect a microk8s cluster to be running. When you run `make test-e2e`, it will automatically:
1. Switch to the microk8s context (if available)
2. Install Mimir via Helm (if not already installed)
3. Create a service alias for test access
4. Deploy the controller
5. Run test scenarios
6. Clean up resources

**Important**: The tests check that the current Kubernetes context is set to `microk8s` to prevent accidentally running against the wrong cluster.

## Running the Tests

### Full E2E Test Suite

To run all e2e tests including the controller deployment:

```bash
# From the project root
make test-e2e
```

### Running Specific Test Suites

To run only the MimirAlertTenant e2e tests:

```bash
# From the project root
cd test/e2e
ginkgo --focus="MimirAlertTenant E2E" .
```

### Running with Verbose Output

For detailed test output:

```bash
ginkgo -v --focus="MimirAlertTenant E2E" .
```


## Test Configuration

### Mimir Connection

The tests assume a Mimir instance is available at:
```
http://mimir-gateway.mimir.svc.cluster.local:8080
```

This is the Mimir gateway service installed by the Helm chart in the `mimir` namespace.

### Test Namespace

Tests run in the `mimiralerttenant-e2e-test` namespace, which is automatically created and cleaned up.

### Automatic Setup via Makefile

The `make test-e2e` target handles all prerequisites automatically:
- Verifies and switches to microk8s context
- Installs Mimir via Helm if not present
- Creates the `lgtm-mimir` service alias
- Runs the complete test suite

### Timeouts

- Default timeout: 2 minutes
- Poll interval: 1 second

Adjust these values in the test file if needed for slower environments.

## Troubleshooting

### Tests Fail to Find Mimir

If tests fail because they can't connect to Mimir:

1. Verify LGTM stack is running:
   ```bash
   kubectl get pods -n default | grep lgtm
   ```

2. Check Mimir service:
   ```bash
   kubectl get svc -n default | grep mimir
   ```

3. Verify the service endpoint:
   ```bash
   kubectl get endpoints -n default lgtm-mimir
   ```

### Context Not Set to microk8s

If you get an error about the wrong Kubernetes context:

```bash
kubectl config use-context microk8s
```

### Controller Not Starting

Check controller logs:
```bash
kubectl logs -n openawareness-controller-system -l control-plane=controller-manager
```

### Clean Up Stuck Resources

If test resources get stuck:

```bash
# Delete test namespace
kubectl delete namespace mimiralerttenant-e2e-test --force --grace-period=0

# Remove finalizers if needed
kubectl patch mimiralerttenant <name> -n mimiralerttenant-e2e-test -p '{"metadata":{"finalizers":[]}}' --type=merge
```

## Development

### Adding New E2E Tests

1. Create a new test file in `test/e2e/`
2. Use the Ginkgo/Gomega framework (see existing tests for examples)
3. Follow the project's test structure:
   - Use `Describe` for test suites
   - Use `Context` for test scenarios
   - Use `It` for individual test cases
   - Use `BeforeAll`/`AfterAll` for setup/cleanup

4. Ensure tests:
   - Create their own namespace
   - Clean up all resources
   - Use `Eventually` for asynchronous checks
   - Have appropriate timeouts

### Running Tests Locally During Development

For faster iteration during development, you can run the controller locally while tests run against the cluster:

1. Install CRDs:
   ```bash
   make install
   ```

2. Run controller locally:
   ```bash
   make run
   ```

3. In another terminal, run specific tests:
   ```bash
   cd test/e2e
   ginkgo --focus="specific test" .
   ```

## CI/CD Integration

The e2e tests can be integrated into CI/CD pipelines. Ensure:

1. Kubernetes cluster is available (microk8s or equivalent)
2. KUBECONFIG is properly set
3. Required tools are installed
4. Sufficient resources are allocated (tests need to deploy controller + LGTM stack)

## Test Coverage

Current e2e test coverage:

- ✅ MimirAlertTenant creation and reconciliation
- ✅ MimirAlertTenant updates (config and templates)
- ✅ MimirAlertTenant deletion and cleanup
- ✅ Error handling (missing annotations, invalid configs)
- ✅ ClientConfig integration
- ✅ Finalizer lifecycle

## Additional Resources

- [Ginkgo Documentation](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)
- [Controller Runtime Testing](https://book.kubebuilder.io/reference/testing)
- [Grafana Mimir Documentation](https://grafana.com/docs/mimir/)
