# OpenAwareness Operator Helm Charts

For more information, visit the [GitHub repository](https://github.com/Syndlex/Openawareness-Operator).

## Usage

[Helm](https://helm.sh) must be installed to use the charts. Please refer to Helm's [documentation](https://helm.sh/docs) to get started.

Add the Helm repository:

```bash
helm repo add openawareness https://syndlex.github.io/openawareness-operator
helm repo update
```

Install the chart:

```bash
helm install openawareness openawareness/openawareness-controller \
  --namespace openawareness-system \
  --create-namespace
```

Uninstall the chart:

```bash
helm uninstall openawareness --namespace openawareness-system
```
