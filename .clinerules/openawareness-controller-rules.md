# Cline Rules for openawareness-controller

This document defines the rules and guidelines for developing the **openawareness-controller**, a Kubernetes operator for managing Grafana Mimir Prometheus Rules and Alertmanager configurations.

---

## 1. Behavioral Guidelines

### 1.1 Communication Style
- Be **direct and technical** in all responses
- Avoid conversational phrases like "Great", "Certainly", "Okay", "Sure"
- Focus on actionable information and clear technical explanations
- Provide context for technical decisions

### 1.2 Problem-Solving Approach
- **Test-Driven Development (TDD)**: Always write tests BEFORE implementing features
- Break down complex tasks into smaller, testable units
- Analyze existing patterns in the codebase before proposing new solutions
- Prioritize DevOps pipeline integration and alerting use cases

### 1.3 Development Focus
- **Primary Use Case**: Supporting DevOps pipelines with alerts and alert management
- **Target Integration**: Grafana Mimir and Prometheus ecosystem
- **Multi-tenancy**: Design with tenant isolation in mind
- **Kubernetes-native**: Follow Kubernetes operator patterns and best practices

---

## 2. Code Quality Standards

### 2.1 Go Language Standards
- **Go Version**: Use Go 1.23.0+ features and idioms
- **Formatting**: All code must pass `go fmt` and `gofmt`
- **Imports**: Use `goimports` for automatic import management
- **Linting**: All code must pass `golangci-lint run` without errors

### 2.2 Enabled Linters (from .golangci.yml)
Must comply with all enabled linters:
- `dupl` - Duplicate code detection
- `errcheck` - Unchecked error detection
- `exportloopref` - Loop variable exporting issues
- `ginkgolinter` - Ginkgo test framework linting
- `goconst` - Repeated strings that could be constants
- `gocyclo` - Cyclomatic complexity
- `gofmt` - Go formatting
- `goimports` - Import management
- `gosimple` - Simplification suggestions
- `govet` - Go vet checks
- `ineffassign` - Ineffectual assignments
- `lll` - Line length (relaxed for api/* and internal/*)
- `misspell` - Spelling errors
- `nakedret` - Naked returns
- `prealloc` - Slice preallocation
- `revive` - Comprehensive linting with comment-spacings rule
- `staticcheck` - Static analysis
- `typecheck` - Type checking
- `unconvert` - Unnecessary conversions
- `unparam` - Unused parameters
- `unused` - Unused code

### 2.3 Error Handling
- **Never use `panic()`** in production code (controllers, API handlers, clients)
- Always check and handle errors explicitly
- Use structured logging for error context: `logger.Error(err, "message", "key", value)`
- Wrap errors with context using `fmt.Errorf("context: %w", err)`
- Return errors from functions; don't log and ignore
- Use `client.IgnoreNotFound(err)` for Kubernetes resource lookups where appropriate

### 2.4 Logging
- Use structured logging via `sigs.k8s.io/controller-runtime/pkg/log`
- Obtain logger from context: `logger := log.FromContext(ctx)`
- Include relevant context in log messages:
  ```go
  logger.Info("Found Rule", "Name", rule.Name, "Namespace", rule.Namespace)
  logger.Error(err, "Failed to create rule group", "Group", group.Name)
  ```
- Use log levels appropriately:
  - `logger.Info()` - Normal operations
  - `logger.V(1).Info()` - Debug/verbose information
  - `logger.Error()` - Error conditions
- **Never log sensitive data** (passwords, tokens, keys)

### 2.5 Code Organization
- Keep functions focused and single-purpose
- Extract complex logic into helper functions
- Maximum function length: ~50 lines (aim for less)
- Use meaningful variable and function names
- Group related functionality in separate files

---

## 3. Project Structure Rules

### 3.1 Directory Layout (Kubebuilder Standard)
```
openawareness-controller/
├── api/openawareness/v1beta1/     # CRD definitions (types)
├── cmd/                            # Main application entry points
├── config/                         # Kubernetes manifests and kustomize
│   ├── crd/                       # CRD YAML definitions
│   ├── rbac/                      # RBAC roles and bindings
│   ├── manager/                   # Manager deployment
│   └── samples/                   # Example CRs
├── internal/                       # Private application code
│   ├── controller/                # Controller reconcilers
│   │   ├── openawareness/        # Custom CRD controllers
│   │   └── monitoring.coreos.com/ # External CRD controllers
│   ├── clients/                   # Client cache and interfaces
│   └── mimir/                     # Mimir API client
└── test/                          # Test utilities and e2e tests
```

### 3.2 File Naming Conventions
- CRD types: `{resource}_types.go` (e.g., `clientconfig_types.go`)
- Controllers: `{resource}_controller.go` (e.g., `clientconfig_controller.go`)
- Tests: `{name}_test.go` (standard Go testing)
- API clients: Grouped by purpose (e.g., `client.go`, `rules.go`, `alerts.go`)

### 3.3 API Versioning
- Current API version: `v1beta1`
- API Group: `openawareness.syndlex`
- When adding new fields, maintain backward compatibility
- Use `+optional` markers for optional fields in CRDs
- Document breaking changes in CRD comments

---

## 4. Kubernetes Operator Patterns

### 4.1 Controller Reconciliation
- Follow the reconciliation loop pattern:
  1. Fetch the resource
  2. Handle deletion (check `DeletionTimestamp`)
  3. Add finalizer if not present
  4. Perform reconciliation logic
  5. Update status (if applicable)
  6. Remove finalizer on deletion
  7. Return `ctrl.Result{}`

Example pattern from existing code:
```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)
    
    resource := &v1beta1.Resource{}
    if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    if resource.ObjectMeta.DeletionTimestamp.IsZero() {
        // Add finalizer
        if !controllerutil.ContainsFinalizer(resource, utils.MyFinalizerName) {
            controllerutil.AddFinalizer(resource, utils.MyFinalizerName)
            if err := r.Update(ctx, resource); err != nil {
                return ctrl.Result{}, err
            }
        }
        // Reconciliation logic here
    } else {
        // Cleanup logic here
        
        if controllerutil.ContainsFinalizer(resource, utils.MyFinalizerName) {
            controllerutil.RemoveFinalizer(resource, utils.MyFinalizerName)
            if err := r.Update(ctx, resource); err != nil {
                return ctrl.Result{}, err
            }
        }
    }
    
    return ctrl.Result{}, nil
}
```

### 4.2 Finalizers
- Use the constant `utils.MyFinalizerName` for all finalizers
- Always add finalizer before performing external operations
- Clean up external resources before removing finalizer
- Handle finalizer removal errors appropriately

### 4.3 Annotations
Project uses these annotation conventions:
- `openawareness.io/client-name` - References ClientConfig for API access
- `openawareness.io/mimir-tenant` - Specifies Mimir tenant for rules/alerts
- Always validate annotation presence before use
- Provide sensible defaults (e.g., "anonymous" namespace)

### 4.4 RBAC Markers
- Add kubebuilder RBAC markers above `Reconcile()` function:
  ```go
  // +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs,verbs=get;list;watch;create;update;patch;delete
  // +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs/status,verbs=get;update;patch
  // +kubebuilder:rbac:groups=openawareness.syndlex,resources=clientconfigs/finalizers,verbs=update
  ```
- Run `make manifests` after adding/modifying RBAC markers
- Follow least-privilege principle

### 4.5 Status Management
- Update status subresources when tracking reconciliation state
- Use conditions to reflect reconciliation status
- Don't update status on every reconciliation if nothing changed
- Separate status updates from spec updates

---

## 5. Test-Driven Development

### 5.1 TDD Workflow
**MANDATORY**: Follow TDD for all new features and bug fixes:

1. **Write the test first**
   - Define expected behavior
   - Write test that fails
   - Use table-driven tests for multiple scenarios

2. **Implement minimal code**
   - Make the test pass
   - Don't over-engineer

3. **Refactor**
   - Improve code quality
   - Ensure tests still pass

### 5.2 Testing Framework
- **Framework**: Ginkgo v2.21.0 + Gomega v1.35.1
- **Structure**: Use `Describe`, `Context`, `It` blocks
- **Assertions**: Use Gomega matchers (`Expect().To()`, `Eventually()`)

Example test structure:
```go
var _ = Describe("ClientConfig Controller", func() {
    Context("When reconciling a resource", func() {
        It("should add the finalizer", func() {
            // Arrange
            ctx := context.Background()
            clientConfig := &v1beta1.ClientConfig{...}
            
            // Act
            result, err := reconciler.Reconcile(ctx, req)
            
            // Assert
            Expect(err).NotTo(HaveOccurred())
            Expect(clientConfig.Finalizers).To(ContainElement(utils.MyFinalizerName))
        })
    })
})
```

### 5.3 Test Coverage
- **Target**: Aim for >80% test coverage on controller logic
- **Focus Areas**:
  - Controller reconciliation paths
  - Error handling
  - Edge cases (deletion, missing annotations, etc.)
  - Client interactions (use mocks)
- Run tests with coverage: `make test`
- Don't test generated code (`zz_generated.deepcopy.go`)

### 5.4 Mocking External Dependencies
- Mock Mimir API calls using interfaces
- Mock Kubernetes client operations
- Use test contexts and fake clients from controller-runtime
- Don't make real HTTP calls in unit tests

### 5.5 Table-Driven Tests
Use table-driven tests for testing multiple scenarios:
```go
DescribeTable("should handle different client types",
    func(clientType v1beta1.ClientType, expectedCalls int) {
        // Test implementation
    },
    Entry("Mimir client", v1beta1.Mimir, 1),
    Entry("Prometheus client", v1beta1.Prometheus, 1),
)
```

---

## 6. Mimir & Prometheus Integration

### 6.1 Mimir Client Patterns
- Use the `MimirClient` from `internal/mimir/client.go`
- Configure TLS properly for production environments
- Use structured logging for all API operations
- Handle rate limiting (`errTooManyRequests`)
- Retry transient failures appropriately

### 6.2 Rule Group Management
- Convert PrometheusRule CRs to Mimir `rulefmt.RuleGroup` format
- Use `convert()` function pattern for transformations
- Validate rule syntax before sending to Mimir
- Handle both alert rules (`Alert` field) and recording rules (`Record` field)

### 6.3 Alertmanager Configuration
- Map AlertmanagerConfig to Mimir format
- Support templates, routes, and receivers
- Validate configuration before applying
- Handle tenant-specific configurations

### 6.4 Multi-Tenancy
- Use `X-Scope-OrgID` header for tenant identification
- Extract tenant from annotations or resource namespace
- Default to "anonymous" tenant when not specified
- Never mix data between tenants

### 6.5 HTTP Client Best Practices
- Set appropriate timeouts
- Use context for cancellation
- Configure TLS with proper certificate validation
- Add retry logic for transient failures
- Log request/response details (excluding sensitive data)

---

## 7. Documentation Standards

### 7.1 Code Documentation
- **GoDoc comments** for all exported types, functions, and constants:
  ```go
  // ClientConfig represents a configuration for connecting to Mimir or Prometheus.
  // It defines the connection parameters and authentication methods.
  type ClientConfig struct {
      // Spec defines the desired state of ClientConfig
      Spec ClientConfigSpec `json:"spec,omitempty"`
      // Status defines the observed state of ClientConfig
      Status ClientConfigStatus `json:"status,omitempty"`
  }
  ```

- **Inline comments** for complex logic:
  ```go
  // We need to convert PrometheusRule format to Mimir's rulefmt.RuleGroup
  // because Mimir expects a different structure for intervals and labels
  groups := convert(rule.Spec.Groups)
  ```

- **Function documentation**:
  ```go
  // CreateRuleGroup creates or updates a rule group in Mimir.
  // It returns an error if the API call fails or if the rule group is invalid.
  func (c *MimirClient) CreateRuleGroup(ctx context.Context, namespace string, group rulefmt.RuleGroup) error {
  ```

### 7.2 README Requirements
- **Update README.md** when adding new features or changing architecture
- Include:
  - Clear project description and purpose
  - Architecture overview
  - CRD documentation with examples
  - Installation instructions
  - Usage examples for DevOps pipelines
  - Configuration options
  - Troubleshooting common issues
- Remove TODO placeholders
- Keep prerequisites up to date

### 7.3 CRD Documentation
- Add descriptions to CRD fields using comments:
  ```go
  type ClientConfigSpec struct {
      // Address is the URL of the Mimir or Prometheus instance
      // +kubebuilder:validation:Required
      Address string `json:"address"`
      
      // Type specifies whether this is a Mimir or Prometheus instance
      // +kubebuilder:validation:Enum=Mimir;Prometheus
      Type ClientType `json:"type"`
  }
  ```

### 7.4 Example Resources
- Provide working examples in `config/samples/`
- Include comments explaining each field
- Show common use cases for DevOps pipelines
- Ensure examples are valid and tested

---

## 8. Git Workflow & Commit Standards

### 8.1 Conventional Commits
**MANDATORY**: All commits must follow [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only changes
- `style`: Code style changes (formatting, no logic change)
- `refactor`: Code refactoring (no feature or bug fix)
- `test`: Adding or updating tests
- `chore`: Maintenance tasks (dependencies, build, etc.)
- `perf`: Performance improvements
- `ci`: CI/CD changes

**Scopes** (examples):
- `controller`: Controller changes
- `api`: CRD/API changes
- `client`: Mimir/Prometheus client changes
- `tests`: Test-related changes
- `docs`: Documentation changes

**Examples**:
```
feat(controller): implement MimirTenant reconciliation logic

Add reconciliation logic for MimirTenant CRD to manage tenant configurations
in Grafana Mimir. Includes finalizer handling and status updates.

Closes #123
```

```
fix(client): handle rate limiting in Mimir API calls

Add retry logic with exponential backoff when Mimir API returns 429 status.
```

```
docs(readme): update installation instructions

Add helm chart installation method and update prerequisites.
```

```
test(controller): add tests for PrometheusRules reconciliation

Cover edge cases: missing annotations, invalid rules, deletion scenarios.
```

### 8.2 Commit Best Practices
- Keep commits atomic (one logical change per commit)
- Write clear, descriptive commit messages
- Reference issue numbers when applicable
- Don't commit generated files unless necessary
- Run `make manifests` and `make generate` before committing API changes

### 8.3 Pre-commit Checklist
Before committing code:
1. Run `make fmt` - Format code
2. Run `make vet` - Vet code
3. Run `make lint` - Run linters
4. Run `make test` - Run tests
5. Run `make manifests` - Update generated manifests (if API changed)
6. Run `make generate` - Update generated code (if API changed)

---

## 9. Development Workflow

### 9.1 Making Changes
1. **Create feature branch** from main
2. **Write tests first** (TDD approach)
3. **Implement feature** to pass tests
4. **Run pre-commit checks** (see 8.3)
5. **Update documentation** if needed
6. **Commit with conventional commit message**
7. **Create pull request**

### 9.2 Controller Development Workflow
When creating or modifying controllers:

1. **Define/Update CRD types** in `api/openawareness/v1beta1/`
2. **Run code generation**:
   ```bash
   make generate  # Generate DeepCopy methods
   make manifests # Generate CRD YAML
   ```
3. **Write controller tests** (TDD)
4. **Implement controller logic**
5. **Update RBAC markers** if needed
6. **Test locally**:
   ```bash
   make install  # Install CRDs
   make run      # Run controller locally
   ```
7. **Create sample CRs** in `config/samples/`
8. **Update README** with usage examples

### 9.3 Testing Workflow
- **Unit tests**: `make test`
- **E2E tests**: `make test-e2e` (requires cluster access)
- **Local testing**: Use `make run` with sample CRs
- **Integration testing**: Deploy to test cluster with `make deploy`

### 9.4 Build and Deploy Workflow
```bash
# Local development
make run

# Build binary
make build

# Build container image
make docker-build IMG=<registry>/openawareness-controller:tag

# Push container image
make docker-push IMG=<registry>/openawareness-controller:tag

# Deploy to cluster
make deploy IMG=<registry>/openawareness-controller:tag

# Undeploy from cluster
make undeploy
```

---

## 10. Security & Operations

### 10.1 Security Best Practices
- **Secrets Management**:
  - Never hardcode credentials
  - Use Kubernetes Secrets for sensitive data
  - Reference secrets in CRDs using SecretKeySelector
  - Don't log secret values

- **RBAC**:
  - Follow least-privilege principle
  - Use specific resource names when possible
  - Avoid cluster-wide permissions unless necessary
  - Document required permissions

- **Input Validation**:
  - Validate all CRD inputs
  - Use kubebuilder validation markers
  - Sanitize data before external API calls
  - Validate URLs and file paths

- **TLS/HTTPS**:
  - Always use TLS for external communications
  - Validate certificates properly
  - Support custom CA certificates
  - Use secure TLS versions (1.2+)

### 10.2 Logging Best Practices
- Use structured logging consistently
- Include relevant context (namespace, name, etc.)
- Use appropriate log levels (Info, V(1), Error)
- Never log sensitive information:
  - Passwords
  - API tokens
  - Private keys
  - Secret values
  - Sensitive user data

### 10.3 Resource Management
- Set resource limits in deployment manifests
- Handle context cancellation properly
- Close HTTP connections appropriately
- Clean up resources in finalizers
- Avoid memory leaks in long-running operations

### 10.4 Error Handling for Operations
- Provide clear error messages
- Include context in error wrapping
- Log errors with structured fields
- Don't expose internal implementation details in errors
- Use appropriate HTTP status codes for API responses

### 10.5 Observability
- Use structured logging for operation tracking
- Include request IDs or correlation IDs when available
- Log operation start/completion
- Log external API call results
- Support verbose logging with `logger.V(1).Info()`

---

## 11. DevOps Pipeline Integration

### 11.1 Alert Configuration Patterns
When configuring alerts for DevOps pipelines:

- **Use descriptive alert names** that indicate the issue
- **Include relevant labels**:
  - `severity`: critical, warning, info
  - `team`: owning team
  - `service`: affected service
  - `environment`: production, staging, etc.
- **Write clear annotations**:
  - `summary`: Brief description
  - `description`: Detailed explanation with context
  - `runbook_url`: Link to remediation steps

Example:
```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: pipeline-alerts
  annotations:
    openawareness.io/client-name: "production-mimir"
    openawareness.io/mimir-tenant: "devops-team"
spec:
  groups:
  - name: pipeline
    rules:
    - alert: PipelineFailureRate
      expr: rate(pipeline_failures_total[5m]) > 0.1
      labels:
        severity: critical
        team: devops
      annotations:
        summary: "High pipeline failure rate detected"
        description: "Pipeline failure rate is {{ $value }} per second"
```

### 11.2 Rule Organization
- Group related rules together
- Use meaningful group names
- Separate alerts by severity or team
- Keep rule groups focused and maintainable
- Document complex PromQL queries

### 11.3 Multi-Tenant Considerations
- Use `openawareness.io/mimir-tenant` annotation for tenant isolation
- Ensure alerts are routed to correct teams
- Don't share sensitive data across tenants
- Test tenant isolation thoroughly

---

## 12. Common Patterns & Anti-Patterns

### 12.1 DO: Good Patterns
✅ Use structured logging with context
✅ Handle errors explicitly and wrap with context
✅ Write tests before implementation (TDD)
✅ Use finalizers for cleanup of external resources
✅ Validate inputs early
✅ Use constants for repeated strings
✅ Document complex logic with comments
✅ Use table-driven tests for multiple scenarios
✅ Keep functions small and focused
✅ Use meaningful variable names

### 12.2 DON'T: Anti-Patterns
❌ Don't use `panic()` in production code
❌ Don't ignore errors
❌ Don't log sensitive data
❌ Don't hardcode credentials or URLs
❌ Don't make assumptions about resource existence
❌ Don't mix business logic with Kubernetes operations
❌ Don't create untested code
❌ Don't commit without running pre-commit checks
❌ Don't use generic error messages
❌ Don't skip documentation

### 12.3 Example: Good Error Handling
```go
// Good ✅
func (r *Reconciler) processRule(ctx context.Context, rule *v1.PrometheusRule) error {
    logger := log.FromContext(ctx)
    
    client, err := r.getClient(rule)
    if err != nil {
        logger.Error(err, "Failed to get Mimir client", 
            "rule", rule.Name, 
            "namespace", rule.Namespace)
        return fmt.Errorf("getting Mimir client for rule %s: %w", rule.Name, err)
    }
    
    if err := client.CreateRuleGroup(ctx, namespace, group); err != nil {
        logger.Error(err, "Failed to create rule group",
            "rule", rule.Name,
            "group", group.Name,
            "namespace", namespace)
        return fmt.Errorf("creating rule group %s: %w", group.Name, err)
    }
    
    return nil
}

// Bad ❌
func (r *Reconciler) processRule(ctx context.Context, rule *v1.PrometheusRule) error {
    client, _ := r.getClient(rule)  // Ignoring error!
    client.CreateRuleGroup(ctx, namespace, group)  // Ignoring error!
    return nil
}
```

---

## 13. Troubleshooting & Debugging

### 13.1 Common Issues
- **Controller not reconciling**: Check RBAC permissions and event logs
- **CRD validation failures**: Review kubebuilder markers and CRD spec
- **Mimir API errors**: Check client configuration, TLS, and authentication
- **Resource not found**: Verify resource exists and namespace is correct
- **Finalizer stuck**: Check cleanup logic and error handling

### 13.2 Debugging Tips
- Enable verbose logging: `logger.V(1).Info()`
- Check controller logs: `kubectl logs -n <namespace> <pod-name>`
- Inspect resource events: `kubectl describe <resource-type> <name>`
- Verify RBAC: `kubectl auth can-i <verb> <resource> --as=system:serviceaccount:<namespace>:<sa-name>`
- Test API connectivity: Use curl to test Mimir API endpoints
- Review finalizers: `kubectl get <resource> -o yaml` and check `.metadata.finalizers`

### 13.3 Local Development Debugging
```bash
# Run controller locally with verbose logging
make run

# Install CRDs without deploying controller
make install

# Apply sample resources
kubectl apply -f config/samples/

# Watch resource changes
kubectl get <resource-type> -w

# Check controller metrics
kubectl port-forward -n <namespace> <pod-name> 8080:8080
curl localhost:8080/metrics
```

---

## 14. Summary Checklist

Before submitting code, ensure:

- [ ] Tests written BEFORE implementation (TDD)
- [ ] All tests pass (`make test`)
- [ ] Code formatted (`make fmt`)
- [ ] Code vetted (`make vet`)
- [ ] Linters pass (`make lint`)
- [ ] No `panic()` in production code
- [ ] Errors handled explicitly
- [ ] Structured logging used correctly
- [ ] No sensitive data in logs
- [ ] Documentation updated (README, GoDoc, comments)
- [ ] CRD changes reflected in manifests (`make manifests`)
- [ ] Generated code updated (`make generate`)
- [ ] RBAC markers correct
- [ ] Conventional commit message
- [ ] Examples in `config/samples/` work
- [ ] Finalizers handle cleanup properly

---

## 15. Additional Resources

### 15.1 Key Documentation
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [Grafana Mimir Documentation](https://grafana.com/docs/mimir/)
- [Prometheus Operator](https://prometheus-operator.dev/)
- [Conventional Commits](https://www.conventionalcommits.org/)

### 15.2 Internal Project Documentation
- `README.md` - Project overview and setup
- `api/openawareness/v1beta1/` - CRD type definitions
- `config/samples/` - Example custom resources
- `Makefile` - Build and development targets

---

**These rules are living documentation. Update them as the project evolves and new patterns emerge.**
