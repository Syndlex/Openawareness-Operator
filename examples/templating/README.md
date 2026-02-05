# Templating Examples for MimirAlertTenant

This directory contains examples demonstrating how to use environment variable templating in `MimirAlertTenant` resources.

## Overview

The `MimirAlertTenant` CRD supports Go template syntax in the `alertmanagerConfig` field, allowing you to inject values from Kubernetes ConfigMaps and Secrets. This is useful for:

- Keeping sensitive data (webhook URLs, API keys) in Secrets
- Sharing common configuration across multiple alert tenants
- Managing environment-specific values (dev/staging/prod)
- Separating configuration from sensitive credentials

## Files

1. **00-clientconfig.yaml** - ClientConfig for connecting to Mimir (required)
2. **01-configmaps-secrets.yaml** - Example ConfigMaps and Secrets with template variables
3. **02-mimiralerttenant-basic.yaml** - Basic example using ConfigMap templating
4. **03-mimiralerttenant-with-secrets.yaml** - Advanced example with ConfigMaps and Secrets

## Quick Start

### 1. Create ClientConfig

First, create the ClientConfig to connect to your Mimir instance:

```bash
# Edit 00-clientconfig.yaml to set your Mimir address
kubectl apply -f 00-clientconfig.yaml

kubectl describe clientconfigs.openawareness.syndlex template-mimir
```

### 2. Create ConfigMaps and Secrets

```bash
kubectl apply -f 01-configmaps-secrets.yaml
```

This creates:
- `alert-smtp-config` ConfigMap with SMTP settings
- `alert-team-emails` ConfigMap with team email addresses
- `alert-webhooks` Secret with sensitive webhook URLs

### 3. Apply a Templated MimirAlertTenant

Basic example (ConfigMaps only):
```bash
kubectl apply -f 02-mimiralerttenant-basic.yaml
```

Advanced example (ConfigMaps + Secrets):
```bash
kubectl apply -f 03-mimiralerttenant-with-secrets.yaml
```

## Template Syntax

### Basic Variable Substitution

```yaml
alertmanagerConfig: |
  global:
    smtp_smarthost: '{{ .SMTP_HOST ]]'
    smtp_from: '{{ .SMTP_FROM ]]'
```

### Default Values

Provide fallback values if a variable is missing:

```yaml
alertmanagerConfig: |
  receivers:
    - name: 'default'
      email_configs:
        - to: '{{ .TEAM_EMAIL | default "fallback@example.org" ]]'
```

### Conditional Sections

Include sections only if a variable exists:

```yaml
alertmanagerConfig: |
  receivers:
    - name: 'critical'
      email_configs:
        - to: '{{ .ONCALL_EMAIL ]]'
      {{- if .SLACK_WEBHOOK_URL ]]
      slack_configs:
        - api_url: '{{ .SLACK_WEBHOOK_URL ]]'
      {{- end ]]
```

### Escaping Alertmanager Templates

Alertmanager uses `[[ ]]` for its own templating. To include literal Alertmanager templates, escape them:

```yaml
alertmanagerConfig: |
  slack_configs:
    - title: 'Alert: [[ "{{" ]] .GroupLabels.alertname [[ "}}" ]]'
```

## SecretDataReferences Field

Reference ConfigMaps and Secrets in the `spec.secretDataReferences` field:

```yaml
spec:
  secretDataReferences:
    - name: alert-smtp-config
      kind: ConfigMap
    - name: alert-team-emails
      kind: ConfigMap
    - name: alert-webhooks
      kind: Secret
      optional: true  # Don't fail if this doesn't exist
```

### Reference Order

References are processed in order. Later references override earlier ones if keys conflict:

```yaml
spec:
  secretDataReferences:
    - name: base-config      # Base values
      kind: ConfigMap
    - name: env-overrides    # Environment-specific overrides
      kind: ConfigMap
```

### Optional References

Mark references as optional to avoid failures when they don't exist:

```yaml
spec:
  secretDataReferences:
    - name: alert-webhooks
      kind: Secret
      optional: true  # Won't fail if secret doesn't exist
```

## Use Cases

### 1. Environment-Specific Configuration

Use different ConfigMaps for dev/staging/prod:

```yaml
# dev-alerts.yaml
spec:
  secretDataReferences:
    - name: alert-config-base
      kind: ConfigMap
    - name: alert-config-dev    # Dev-specific values
      kind: ConfigMap
```

```yaml
# prod-alerts.yaml
spec:
  secretDataReferences:
    - name: alert-config-base
      kind: ConfigMap
    - name: alert-config-prod   # Prod-specific values
      kind: ConfigMap
```

### 2. Shared Team Configuration

Share common team settings across multiple alert configurations:

```yaml
spec:
  secretDataReferences:
    - name: team-devops-contacts  # Shared across all DevOps alerts
      kind: ConfigMap
    - name: service-specific-settings
      kind: ConfigMap
```

### 3. Secrets Management

Keep sensitive data in Secrets, visible configuration in ConfigMaps:

```yaml
spec:
  secretDataReferences:
    - name: public-alert-settings  # Email addresses, SMTP host
      kind: ConfigMap
    - name: private-credentials    # API keys, webhook URLs
      kind: Secret
```

## Troubleshooting

### Check Resource Status

View the MimirAlertTenant status to see if templating succeeded:

```bash
kubectl get mimiralerttenant devops-alerts-templated -o yaml
```

Look for conditions in `.status.conditions`:
- `ConfigInvalid` with reason `TemplateDataNotFound` - Referenced ConfigMap/Secret not found
- `ConfigInvalid` with reason `InvalidTemplate` - Template syntax error

### Verify ConfigMap/Secret Values

Check the actual values in your ConfigMaps and Secrets:

```bash
kubectl get configmap alert-smtp-config -o yaml
kubectl get secret alert-webhooks -o yaml
```

### Test Template Rendering

You can test templates locally before applying:

```bash
# View the rendered config (after applying)
kubectl get mimiralerttenant devops-alerts-templated -o jsonpath='{.spec.alertmanagerConfig}'
```

## Best Practices

1. **Use meaningful variable names**: `SMTP_HOST` instead of `HOST1`
2. **Provide defaults**: Use `{{ .VAR | default "fallback" ]]` for optional values
3. **Document variables**: Add comments to ConfigMaps explaining each variable
4. **Separate concerns**: Keep sensitive data in Secrets, public config in ConfigMaps
5. **Test changes**: Apply to a test namespace before production
6. **Version control**: Store example ConfigMaps in your Git repository
7. **Use optional references**: Mark non-critical Secrets as optional

## Additional Resources

- [Go Template Documentation](https://pkg.go.dev/text/template)
- [Alertmanager Configuration](https://prometheus.io/docs/alerting/latest/configuration/)
- [Kubernetes ConfigMaps](https://kubernetes.io/docs/concepts/configuration/configmap/)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
