# openawareness-controller

A Kubernetes operator for managing Grafana Mimir Prometheus Rules and Alertmanager configurations in a multi-tenant environment.

![openawareness-man-Mascot](Mascot.png)

## Description

The OpenAwareness Controller provides Kubernetes-native management of:
- **Prometheus recording and alerting rules** via Grafana Mimir
- **Alertmanager configurations** for notification routing and templates
- **Multi-tenant support** for isolated alert management
- **DevOps pipeline integration** for automated alert configuration

This operator bridges the gap between Kubernetes-native PrometheusRule resources and Grafana Mimir's API, enabling GitOps-style management of alerting infrastructure.

## Features

### Custom Resource Definitions (CRDs)

#### 1. ClientConfig
Defines connection settings for Mimir or Prometheus instances.

```yaml
apiVersion: openawareness.syndlex/v1beta1
kind: ClientConfig
metadata:
  name: mimir-client
spec:
  address: "https://mimir.example.com"
  type: Mimir
```

#### 2. MimirAlertTenant
Manages Alertmanager configurations for a specific tenant in Grafana Mimir.

```yaml
apiVersion: openawareness.syndlex/v1beta1
kind:  MimirAlertTenant
metadata:
  name: team-alerts
  annotations:
    openawareness.io/client-name: "mimir-client"
    openawareness.io/mimir-tenant: "devops-team"
spec:
  templateFiles:
    default_template: |
      {{ define "__alertmanager" }}AlertManager{{ end }}
      {{ define "__alertmanagerURL" }}{{ .ExternalURL }}/#/alerts?receiver={{ .Receiver | urlquery }}{{ end }}
  
  alertmanagerConfig: |
    global:
      smtp_smarthost: 'localhost:25'
      smtp_from: 'alerts@example.org'
    templates:
      - 'default_template'
    route:
      receiver: 'default-receiver'
      group_by: ['alertname', 'cluster', 'service']
      group_wait: 10s
      group_interval: 10s
      repeat_interval: 12h
      routes:
        - match:
            severity: critical
          receiver: 'critical-alerts'
    receivers:
      - name: 'default-receiver'
        email_configs:
          - to: 'team@example.org'
      - name: 'critical-alerts'
        email_configs:
          - to: 'oncall@example.org'
```

#### 3. PrometheusRule Support
The controller automatically syncs standard Kubernetes PrometheusRule resources to Grafana Mimir.

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: example-rules
  annotations:
    openawareness.io/client-name: "mimir-client"
    openawareness.io/mimir-tenant: "devops-team"
spec:
  groups:
  - name: example
    rules:
    - alert: HighErrorRate
      expr: rate(http_errors_total[5m]) > 0.05
      labels:
        severity: critical
      annotations:
        summary: "High error rate detected"
```

## Getting Started

### Prerequisites
- Go version v1.23.0+
- Docker version 17.03+
- kubectl version v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster
- Grafana Mimir instance with API access

### Installation

#### Option 1: Helm (Recommended)

##### Install from OCI Registry

```sh
helm install openawareness oci://ghcr.io/syndlex/charts/openawareness-controller \
  --version 0.1.0 \
  --namespace openawareness-system \
  --create-namespace
```

##### Install with custom values

```sh
# Create a values file
cat > my-values.yaml <<EOF
replicaCount: 2

resources:
  limits:
    cpu: 1000m
    memory: 256Mi
  requests:
    cpu: 100m
    memory: 128Mi

metrics:
  enabled: true
  serviceMonitor:
    enabled: true
EOF

# Install with custom values
helm install openawareness oci://ghcr.io/syndlex/charts/openawareness-controller \
  --version 0.1.0 \
  --namespace openawareness-system \
  --create-namespace \
  --values my-values.yaml
```

##### Upgrade to a new version

```sh
helm upgrade openawareness oci://ghcr.io/syndlex/charts/openawareness-controller \
  --version <new-version> \
  --namespace openawareness-system
```

#### Option 2: Kubectl (Direct Installation)

##### 1. Install CRDs
```sh
make install
```

##### 2. Deploy the Controller

**Option A: Deploy to MicroK8s (Recommended for local development)**

The Makefile includes targets for deploying to MicroK8s with its local registry:

```sh
# Build, push to MicroK8s registry, and deploy in one command
make microk8s-deploy
```

This command will:
1. Build the container image
2. Push it to the MicroK8s registry at `localhost:32000`
3. Deploy the controller to your MicroK8s cluster

**Option B: Deploy to External Registry**

Build and push your image to an external registry:
```sh
make docker-build docker-push IMG=<your-registry>/openawareness-controller:tag
```

Deploy to cluster:
```sh
make deploy IMG=<your-registry>/openawareness-controller:tag
```

**Available Make Targets for MicroK8s:**
- `make microk8s-build` - Build image for MicroK8s registry
- `make microk8s-push` - Build and push image to MicroK8s registry
- `make microk8s-deploy` - Build, push, and deploy to MicroK8s cluster

#### 3. Create a ClientConfig
First, create a ClientConfig to connect to your Mimir instance:

```sh
kubectl apply -f - <<EOF
apiVersion: openawareness.syndlex/v1beta1
kind: ClientConfig
metadata:
  name: mimir-client
spec:
  address: "https://your-mimir-instance.example.com"
  type: Mimir
EOF
```

#### 4. Configure Alertmanager (Optional)
Create a MimirAlertTenant to configure Alertmanager for your tenant:

```sh
kubectl apply -f config/samples/openawareness_v1beta1_mimiralerttenant.yaml
```

#### 5. Deploy Prometheus Rules
Create PrometheusRule resources with the appropriate annotations:

```sh
kubectl apply -f config/samples/promrule.yaml
```

## Configuration

### Annotations

The controller uses annotations to determine routing and tenant isolation:

- `openawareness.io/client-name`: References the ClientConfig to use for API calls
- `openawareness.io/mimir-tenant`: Specifies the Mimir tenant/namespace

### Alertmanager Configuration

The MimirAlertTenant CRD supports:

1. **Template Files** (optional): Go templates for notification formatting
   - Define custom templates for email, Slack, PagerDuty, etc.
   - Reference templates in the alertmanagerConfig

2. **Alertmanager Config** (required): Full Alertmanager configuration in YAML
   - Global settings (SMTP, Slack, PagerDuty, etc.)
   - Routing tree with matchers
   - Receivers with notification integrations
   - Inhibition rules

See the [Grafana Mimir Alertmanager API documentation](https://grafana.com/docs/mimir/latest/references/http-api/#set-alertmanager-configuration) for detailed configuration options.

## Development

### Running Tests

#### Unit Tests
Run unit tests with code coverage:
```sh
make test
```

#### End-to-End Tests
Run e2e tests against a running Kubernetes cluster:
```sh
make test-e2e
```

This command will automatically:
1. Check and switch to the microk8s context if available
2. Install Mimir via Helm if not already installed
3. Create a service alias for e2e test access
4. Run the full e2e test suite

**Prerequisites for E2E Tests:**
- microk8s cluster configured with kubectl context
- Helm 3.x installed
- kubectl available in PATH

**Running Specific E2E Tests:**
```sh
# Run only MimirAlertTenant tests
ginkgo --focus="MimirAlertTenant E2E" test/e2e

# Run with verbose output
ginkgo -v test/e2e
```

**Manual Setup (if needed):**
```sh
# Switch to microk8s context
make ensure-microk8s-context

# Install Mimir
make ensure-mimir
```

See [test/e2e/README.md](test/e2e/README.md) for detailed e2e test documentation and troubleshooting.

### Running Locally
```sh
make run
```

### Linting
```sh
make lint
```

### Building
```sh
make build
```

## Architecture

The controller watches for:
1. **PrometheusRule** resources and syncs them to Mimir as rule groups
2. **MimirAlertTenant** resources and configures Alertmanager settings
3. **ClientConfig** resources to manage Mimir API connections

Each controller:
- Uses finalizers to ensure proper cleanup
- Validates configurations before applying
- Provides structured logging for debugging
- Supports multi-tenancy through annotations

## Multi-Tenancy

The controller supports multi-tenant deployments:
- Use the `openawareness.io/mimir-tenant` annotation to specify tenants
- Each tenant has isolated alert rules and Alertmanager configurations
- ClientConfigs can be shared across tenants or isolated per-tenant

## DevOps Integration

The controller is designed for DevOps workflows:

1. **GitOps**: Store PrometheusRules and MimirAlertTenants in Git
2. **CI/CD**: Automatically deploy alert configurations via pipelines
3. **Alert Management**: Version control your alerting strategy
4. **Team Isolation**: Use tenants to separate team alerts
5. **Notification Routing**: Configure per-team notification channels

## Troubleshooting

### Controller Logs
```sh
kubectl logs -n openawareness-controller-system deployment/openawareness-controller-controller-manager
```

### Verify CRD Installation
```sh
kubectl get crd | grep openawareness
```

### Check Resource Status
```sh
kubectl describe mimiralerttenant <name>
kubectl describe prometheusrule <name>
```

### Common Issues

1. **Rules not appearing in Mimir**: Check that `openawareness.io/client-name` annotation references an existing ClientConfig
2. **Alertmanager config errors**: Validate YAML syntax in the alertmanagerConfig field
3. **Connection errors**: Verify Mimir endpoint in ClientConfig and network connectivity

## Helm Chart Development

### Generate Helm Chart from Kustomize

The Helm chart is automatically generated from Kustomize manifests using `helmify`:

```sh
# Generate Helm chart
make helm

# Lint Helm chart
make helm-lint

# Package Helm chart
make helm-package

# Install chart locally for testing
make helm-install

# Uninstall chart
make helm-uninstall
```

### Chart Structure

```
chart/openawareness-controller/
├── Chart.yaml              # Chart metadata
├── values.yaml             # Default values (auto-generated)
├── .helmignore            # Files to ignore
├── crds/                  # CRD definitions
│   ├── clientconfigs-crd.yaml
│   └── mimiralerttenants-crd.yaml
└── templates/             # Kubernetes manifests
    ├── deployment.yaml
    ├── service.yaml
    ├── serviceaccount.yaml
    └── ...
```

### Updating the Chart

When you modify Kubernetes manifests in `config/`, regenerate the Helm chart:

```sh
# Make changes to config/ files
vim config/manager/manager.yaml

# Regenerate chart
make helm

# Test the updated chart
make helm-lint
make helm-install
```

## Uninstalling

### Helm Installation

```sh
helm uninstall openawareness --namespace openawareness-system
```

### Kubectl Installation

Delete sample resources:
```sh
kubectl delete -k config/samples/
```

Undeploy controller:
```sh
make undeploy
```

Remove CRDs:
```sh
make uninstall
```

## Contributing

Contributions are welcome! Please:

1. Follow the [Cline Rules](.clinerules/openawareness-controller-rules.md)
2. Write tests for new features (TDD approach)
3. Use conventional commits for commit messages
4. Run linters and tests before submitting PRs
5. Update documentation for new features
