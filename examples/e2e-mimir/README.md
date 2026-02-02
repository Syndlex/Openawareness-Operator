# E2E Mimir Examples

This directory contains example resources that work with the Mimir instance deployed during e2e tests.

## Overview

These examples demonstrate how to:
1. Connect to the e2e Mimir instance using `ClientConfig`
2. Configure alert routing with `MimirAlertTenant`
3. Define Prometheus rules that get synced to Mimir using `PrometheusRule`

## Mimir Deployment Details

The e2e tests deploy Mimir with the following configuration:
- **Namespace**: `mimir`
- **Release**: `mimir`
- **Gateway Service**: `mimir-gateway.mimir.svc.cluster.local:80`
- **Distributor Service**: `mimir-distributor.mimir.svc.cluster.local:8080`

## Prerequisites

1. E2E Mimir instance must be running (see `test/e2e/README.md`)
2. Controller must be deployed
3. Required CRDs must be installed

## Usage

### Apply All Resources

```bash
kubectl apply -f examples/e2e-mimir/
```

### Apply Individual Resources

```bash
# 1. First, create the ClientConfig
kubectl apply -f 01-clientconfig.yaml

# 2. Then create the MimirAlertTenant (optional)
kubectl apply -f 02-mimiralerttenant.yaml

# 3. Finally, create PrometheusRules
kubectl apply -f 03-prometheusrule.yaml
```

## Resource Descriptions

### 01-clientconfig.yaml
Defines the connection to the e2e Mimir instance. This ClientConfig is referenced by other resources via the `openawareness.io/client-name` annotation.

### 02-mimiralerttenant.yaml
Configures Alertmanager for the `devops-team` tenant. This sets up:
- Email notification templates
- Routing rules based on severity
- Multiple receivers for different alert types
- Inhibition rules to prevent alert spam

### 03-prometheusrule.yaml
Defines example Prometheus recording and alerting rules that get synced to Mimir:
- Pipeline health monitoring
- Service availability checks
- Recording rules for metrics aggregation

## Verification

### Check if resources are created

```bash
# Check ClientConfig
kubectl get clientconfig e2e-mimir-client -o yaml

# Check MimirAlertTenant
kubectl get mimiralerttenant devops-alerts -o yaml

# Check PrometheusRule
kubectl get prometheusrule devops-pipeline-rules -o yaml
```

### Verify rules in Mimir

You can use `mimirtools` to query and verify the rules:

```bash
# Port-forward to Mimir gateway
kubectl port-forward -n mimir svc/mimir-gateway 8089:80
```
 
```bash
# In another terminal, configure mimirtools with the Mimir address
export MIMIR_ADDRESS=http://localhost:8089
export MIMIR_TENANT_ID=devops-team

# List all rule groups for the tenant
mimirtool rules list

# Get detailed rules for a specific namespace
mimirtool rules get

# Print all rules in a human-readable format
mimirtool rules print

# Verify alertmanager configuration
mimirtool alertmanager get
```

## Multi-Tenancy

Each PrometheusRule uses the `openawareness.io/mimir-tenant` annotation to specify which Mimir tenant the rules belong to. This enables multi-tenant isolation:

- Rules with namespace `devops-team` are isolated from other tenants
- Different teams can have separate alert configurations
- Queries must include the `X-Scope-OrgID` header matching the namespace

## Cleanup

```bash
# Delete all example resources
kubectl delete -f examples/e2e-mimir/

# Or delete individually
kubectl delete prometheusrule devops-pipeline-rules
kubectl delete mimiralerttenant devops-alerts
kubectl delete clientconfig e2e-mimir-client
```

## Troubleshooting

### Controller not reconciling resources

Check controller logs:
```bash
kubectl logs -n openawareness-controller-system deployment/openawareness-controller-controller-manager -f
```

### Rules not appearing in Mimir

1. Verify ClientConfig is valid:
   ```bash
   kubectl describe clientconfig e2e-mimir-client
   ```

2. Check PrometheusRule events:
   ```bash
   kubectl describe prometheusrule devops-pipeline-rules
   ```

3. Verify Mimir is accessible from within the cluster:
   ```bash
   kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
     curl -v http://mimir-gateway.mimir.svc.cluster.local:80/ready
   ```

### Alert routing not working

1. Check MimirAlertTenant status:
   ```bash
   kubectl get mimiralerttenant devops-alerts -o yaml
   ```

2. Verify alertmanager config in Mimir:
   ```bash
   # Port-forward to Mimir
   kubectl port-forward -n mimir svc/mimir-gateway 8080:80
   
   # Use mimirtools to check alertmanager config
   export MIMIR_ADDRESS=http://localhost:8080
   export MIMIR_TENANT_ID=devops-team
   mimirtool alertmanager get
   
   # Or verify alertmanager status
   mimirtool alertmanager verify
   ```

## Additional Examples

For more examples, see:
- `config/samples/` - Additional sample configurations
- `test/e2e/` - E2E test scenarios
