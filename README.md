# openawareness-controller

A Kubernetes operator for managing Grafana Mimir Prometheus Rules and Alertmanager configurations in a multi-tenant environment.

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
kind: MimirAlertTenant
metadata:
  name: team-alerts
  annotations:
    openawareness.io/client-name: "mimir-client"
    openawareness.io/mimir-namespace: "devops-team"
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
    openawareness.io/mimir-namespace: "devops-team"
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

#### 1. Install CRDs
```sh
make install
```

#### 2. Deploy the Controller
Build and push your image:
```sh
make docker-build docker-push IMG=<your-registry>/openawareness-controller:tag
```

Deploy to cluster:
```sh
make deploy IMG=<your-registry>/openawareness-controller:tag
```

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
- `openawareness.io/mimir-namespace`: Specifies the Mimir tenant/namespace

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
```sh
make test
```

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
- Use the `openawareness.io/mimir-namespace` annotation to specify tenants
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

## Uninstalling

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

## License

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
