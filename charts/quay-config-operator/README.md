# quay-config-operator

A Helm chart for the Quay Config Operator - manages Quay Container Registry repository mirror configurations

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

## Installation

```console
helm repo add quay-config-operator https://ayoy.github.io/quay-config-operator
helm install quay-config-operator quay-config-operator/quay-config-operator
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Affinity rules for pod assignment |
| fullnameOverride | string | `""` | Override the full name of the resources |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"ghcr.io/ayoyab/quay-config-operator"` | Container image repository |
| image.tag | string | `""` | Image tag (defaults to chart appVersion if not set) |
| imagePullSecrets | list | `[]` | Image pull secrets for private registries |
| leaderElection.enabled | bool | `true` | Enable leader election for high availability |
| metrics.enabled | bool | `true` | Enable metrics service |
| metrics.service.port | int | `8080` | Metrics service port |
| nameOverride | string | `""` | Override the name of the chart |
| nodeSelector | object | `{}` | Node selector for pod assignment |
| podAnnotations | object | `{}` | Annotations to add to the pod |
| podSecurityContext | object | `{"runAsNonRoot":true,"seccompProfile":{"type":"RuntimeDefault"}}` | Security context for the pod |
| replicaCount | int | `1` | Number of replicas for the controller deployment |
| resources | object | `{"limits":{"cpu":"500m","memory":"128Mi"},"requests":{"cpu":"10m","memory":"64Mi"}}` | Resource limits and requests for the controller |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Security context for the controller container |
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| serviceAccount.create | bool | `true` | Enable service account creation |
| serviceAccount.name | string | `""` | Service account name (generated if not set) |
| tolerations | list | `[]` | Tolerations for pod assignment |
