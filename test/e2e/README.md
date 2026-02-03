# End-to-End (E2E) Tests for openawareness-controller

This directory contains end-to-end tests for the openawareness-controller, including tests for the MimirAlertTenant CRD.

## Prerequisites

### Required Tools
- Go 1.24.0 or later
- kubectl
- Helm 3.x
- microk8s (or another Kubernetes cluster)

### Cluster Setup

The e2e tests expect a microk8s cluster to be running. When you run `make test-e2e`, it will automatically:
1. Switch to the microk8s context (if available)
2. Install Mimir via Helm (if not already installed)
3. Create a service alias for test access
4. Deploy the controller
5. Create a port forward `kubectl port-forward -n mimir svc/mimir-gateway 8080:80`
6. Run test scenarios
7. Clean up resources

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

#### Multitenant Configuration

Mimir is installed with **multitenancy enabled** to support the MimirAlertTenant controller. The installation configures:

- `multitenancy_enabled: true` - Enables tenant isolation via `X-Scope-OrgID` header
- Alertmanager component with API enabled
- Lightweight configuration for e2e testing (no persistent volumes)

#### Available API Endpoints

With multitenancy enabled, the following endpoints are available:

**Alertmanager APIs:**
- `GET/POST/DELETE /api/v1/alerts` - Alertmanager configuration per tenant
- `GET /multitenant_alertmanager/status` - Alertmanager status

**Prometheus Rules APIs:**
- `GET/POST/DELETE /prometheus/config/v1/rules/{namespace}` - Prometheus rules per namespace

All requests must include the `X-Scope-OrgID` header to specify the tenant.

#### Manual Testing

To test the Mimir API manually:

```bash
# Port-forward to access Mimir locally
make mimir-port-forward

# In another terminal, test alertmanager config endpoint
curl -H "X-Scope-OrgID: test-tenant" \
  http://localhost:8080/api/v1/alerts

# Test prometheus rules endpoint
curl -H "X-Scope-OrgID: test-tenant" \
  http://localhost:8080/prometheus/config/v1/rules/default
```

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

### PrometheusRule E2E Tests (`prometheusrule_test.go`)

Tests the full lifecycle of PrometheusRule resources (from prometheus-operator) with Mimir integration:

#### 1. Valid Configuration
- Creates a PrometheusRule with both alert rules and recording rules
- Verifies finalizer is added
- **Verifies rule groups are pushed to Mimir API**
- **Verifies rule group content (number of rules)**
- Tests deletion and cleanup from Mimir

#### 2. Missing Annotations
- Tests PrometheusRule without client-name annotation
- Verifies graceful handling without crashes
- Tests PrometheusRule without mimir-tenant annotation
- Verifies default tenant is used

#### 3. Invalid References
- Tests PrometheusRule with non-existent ClientConfig
- Verifies resource is created but sync is skipped
- Verifies proper error handling

#### 4. Rule Updates
- Creates initial PrometheusRule with one alert
- Updates rule groups to add additional rules
- **Verifies updated rules are synced to Mimir**
- Verifies rule group content is updated

#### 5. Multiple Rules
- Creates multiple PrometheusRule resources using same ClientConfig
- Verifies both are synced independently
- Tests deletion of one while other remains
- Verifies proper cleanup

#### 6. Complex Rules
- Tests PrometheusRule with multiple alert types (critical, warning)
- Tests recording rules with proper naming conventions
- Verifies complex labels and annotations
- **Verifies all rule types are correctly synced to Mimir**

### MimirAlertTenant E2E Tests (`mimiralerttenant_test.go`)

Tests the full lifecycle of MimirAlertTenant resources with actual Mimir API verification:

#### 1. Resource Creation
- Creates a test namespace
- Creates a ClientConfig pointing to Mimir
- Creates a MimirAlertTenant with Alertmanager configuration
- Verifies finalizer is added
- Verifies annotations are correct
- **Verifies configuration is pushed to Mimir API via GET request**

#### 2. Resource Updates
- Tests updating AlertmanagerConfig
- Tests adding new template files
- Verifies updates are applied correctly in Kubernetes
- **Verifies updated configuration is present in Mimir API**

#### 3. Validation
- Tests handling of invalid YAML configuration
- Verifies controller doesn't crash on validation errors

#### 4. Resource Deletion
- Tests proper cleanup via finalizer
- Verifies resource is fully deleted from Kubernetes
- **Verifies configuration is deleted from Mimir API**

#### 5. Error Handling
- Tests missing client-name annotation
- Tests non-existent ClientConfig reference
- Verifies graceful error handling

### ClientConfig E2E Tests (`clientconfig_test.go`)

Tests the full lifecycle of ClientConfig resources with focus on status updates:

#### 1. Successful Connection
- Creates a ClientConfig pointing to a valid Mimir instance
- Verifies ConnectionStatus is "Connected"
- Verifies Ready condition is True
- Verifies LastConnectionTime is set

#### 2. Failed Connection - Invalid URL
- Creates a ClientConfig with malformed URL
- Verifies ConnectionStatus is "Disconnected"
- Verifies Ready condition is False with reason "InvalidURL"
- Verifies ErrorMessage contains details

#### 3. Failed Connection - Unreachable Host
- Creates a ClientConfig with unreachable endpoint
- Verifies ConnectionStatus is "Disconnected"
- Verifies Ready condition is False with network error reason
- Verifies ErrorMessage contains connection details

#### 4. Connection Recovery
- Updates a failed ClientConfig with valid URL
- Verifies status transitions from Disconnected to Connected
- Verifies conditions are updated appropriately

## Test Helper Functions

The `test/helper/` directory contains reusable helper functions to reduce code duplication:

### MimirAlertTenant Helpers (`mimiralerttenant_helpers.go`)

#### Resource Creation & Lifecycle
- `CreateMimirAlertTenant()` - Creates a MimirAlertTenant with specified config
- `WaitForMimirAlertTenantCreation()` - Waits for resource to be created
- `WaitForFinalizerAdded()` - Waits for finalizer to be added
- `WaitForSyncStatusUpdate()` - Waits for SyncStatus to be updated
- `WaitForResourceDeleted()` - Waits for resource to be fully deleted

#### Resource Updates
- `UpdateMimirAlertTenantConfig()` - Updates AlertmanagerConfig with retry logic
- `AddTemplateFile()` - Adds a template file with retry logic

#### Verification
- `VerifyMimirAlertTenantAnnotations()` - Verifies required annotations
- `VerifySuccessfulSync()` - Verifies successful sync status and conditions
- `VerifyFailedSync()` - Verifies failed sync status and conditions

#### Mimir API Verification
- `CreateMimirClient()` - Creates a Mimir client for API testing
- `VerifyMimirAPIConfig()` - Verifies config was pushed to Mimir
- `VerifyMimirAPITemplate()` - Verifies template was pushed to Mimir
- `VerifyMimirAPIConfigDeleted()` - Verifies config was deleted from Mimir

### ClientConfig Helpers (`clientconfig_helpers.go`)

#### Resource Creation & Lifecycle
- `CreateClientConfig()` - Creates a ClientConfig resource
- `WaitForClientConfigCreation()` - Waits for resource to be created
- `WaitForClientConfigFinalizerAdded()` - Waits for finalizer to be added
- `WaitForClientConfigDeleted()` - Waits for resource to be fully deleted

#### Status Verification
- `WaitForConnectionStatus()` - Waits for specific ConnectionStatus
- `WaitForConditionsSet()` - Waits for conditions to be set
- `FindCondition()` - Finds a condition by type in status

#### Resource Updates
- `UpdateClientConfigAddress()` - Updates address with retry logic
- `AddClientConfigAnnotation()` - Adds annotation with retry logic

#### Verification
- `VerifyConnectedStatus()` - Verifies Connected status with proper conditions
- `VerifyDisconnectedStatus()` - Verifies Disconnected status with error details

### PrometheusRule Helpers (`prometheusrule_helpers.go`)

#### Resource Creation & Lifecycle
- `CreatePrometheusRule()` - Creates a PrometheusRule with specified configuration
- `CreateSimplePrometheusRule()` - Creates a PrometheusRule with single alert for testing
- `WaitForPrometheusRuleCreation()` - Waits for resource to be created
- `WaitForPrometheusRuleFinalizerAdded()` - Waits for finalizer to be added
- `WaitForPrometheusRuleDeleted()` - Waits for resource to be fully deleted

#### Resource Updates
- `UpdatePrometheusRuleGroups()` - Updates rule groups with retry logic
- `AddPrometheusRuleAnnotation()` - Adds annotation with retry logic

#### Mimir API Verification
- `VerifyMimirRuleGroup()` - Verifies rule group exists in Mimir API
- `VerifyMimirRuleGroupDeleted()` - Verifies rule group was deleted from Mimir
- `VerifyMimirRuleGroupContent()` - Verifies rule group has expected number of rules

#### Resource Retrieval
- `GetPrometheusRule()` - Retrieves a PrometheusRule resource

### Generic Kubernetes Helpers (`generic_helpers.go`)

#### Namespace Management
- `CreateNamespace()` - Creates a namespace with cleanup handling
- `DeleteNamespace()` - Deletes a namespace and waits for removal

#### Generic Operations
- `WaitForDeletionTimestamp()` - Waits for DeletionTimestamp to be set

## Debugging Tips

### Port-Forward to Mimir

Access Mimir API locally for manual testing:
```bash
make mimir-port-forward

# In another terminal
curl -H "X-Scope-OrgID: test-tenant" http://localhost:8080/api/v1/alerts
```

### Check Controller Logs

View controller logs during test execution:
```bash
kubectl logs -n openawareness-controller-system \
  -l control-plane=controller-manager -f
```

### Inspect Resource Status

Check resource status after test execution:
```bash
# MimirAlertTenant status
kubectl get mimiralerttenant -n <namespace> -o yaml

# ClientConfig status  
kubectl get clientconfig -n <namespace> -o yaml

# Check conditions
kubectl get clientconfig <name> -n <namespace> -o jsonpath='{.status.conditions}'
```

### Common Test Failures

#### "Mimir API not accessible"
- Verify Mimir is running: `kubectl get pods -n mimir`
- Check service endpoints: `kubectl get endpoints -n mimir`
- Port-forward and test manually

#### "Resource stuck in deletion"
- Check finalizers: `kubectl get <resource> -o yaml | grep finalizers`
- Check controller logs for errors
- Manually remove finalizer if needed (test cleanup only)

#### "Timeout waiting for status update"
- Increase timeout values in test constants
- Check controller is reconciling: look for logs
- Verify RBAC permissions are correct

## Additional Resources

- [Ginkgo Documentation](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)
- [Controller Runtime Testing](https://book.kubebuilder.io/reference/testing)
- [Grafana Mimir Documentation](https://grafana.com/docs/mimir/)
- [Kubernetes E2E Testing Best Practices](https://kubernetes.io/docs/reference/using-api/api-concepts/)
