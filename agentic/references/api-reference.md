# API Reference

**Last Updated**: 2026-04-08  

## Purpose

Quick reference for core OpenShift and Kubernetes APIs.

## OpenShift Config APIs

**Group**: `config.openshift.io/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| ClusterVersion | Cluster version and upgrades | [clusterversion.md](../domain/openshift/clusterversion.md) |
| ClusterOperator | Operator status reporting | [clusteroperator.md](../domain/openshift/clusteroperator.md) |
| Infrastructure | Cloud provider config | - |
| Network | Cluster network config | - |
| Proxy | Cluster proxy settings | - |

## Machine Config APIs

**Group**: `machineconfiguration.openshift.io/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| MachineConfig | OS configuration | [machineconfig.md](../domain/openshift/machineconfig.md) |
| MachineConfigPool | Node groups | [machineconfig.md](../domain/openshift/machineconfig.md) |
| ContainerRuntimeConfig | Container runtime settings | - |
| KubeletConfig | Kubelet configuration | - |

## Machine API

**Group**: `machine.openshift.io/v1beta1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| Machine | Single node | [machine.md](../domain/openshift/machine.md) |
| MachineSet | Set of nodes | [machine.md](../domain/openshift/machine.md) |
| MachineHealthCheck | Node health monitoring | - |

**Group**: `autoscaling.openshift.io/v1beta1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| MachineAutoscaler | Machine scaling | [machine.md](../domain/openshift/machine.md) |
| ClusterAutoscaler | Cluster-wide autoscaling | - |

## Networking APIs

**Group**: `route.openshift.io/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| Route | External access | [route.md](../domain/openshift/route.md) |

**Group**: `operator.openshift.io/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| Network | Network operator config | - |
| IngressController | Ingress controller config | - |

## Kubernetes Core APIs

**Group**: `v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| Pod | Container group | [pod.md](../domain/kubernetes/pod.md) |
| Service | Network endpoint | [service.md](../domain/kubernetes/service.md) |
| ConfigMap | Configuration data | - |
| Secret | Sensitive data | - |
| Node | Cluster node | [node.md](../domain/kubernetes/node.md) |

**Group**: `apps/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| Deployment | Pod management | - |
| StatefulSet | Stateful applications | - |
| DaemonSet | Node-local workloads | - |

**Group**: `apiextensions.k8s.io/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| CustomResourceDefinition | API extensions | [crds.md](../domain/kubernetes/crds.md) |

**Group**: `rbac.authorization.k8s.io/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| Role | Namespace permissions | [rbac.md](../domain/kubernetes/rbac.md) |
| ClusterRole | Cluster permissions | [rbac.md](../domain/kubernetes/rbac.md) |
| RoleBinding | Grant namespace permissions | [rbac.md](../domain/kubernetes/rbac.md) |
| ClusterRoleBinding | Grant cluster permissions | [rbac.md](../domain/kubernetes/rbac.md) |

## Monitoring APIs

**Group**: `monitoring.coreos.com/v1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| Prometheus | Prometheus instance | - |
| ServiceMonitor | Metrics collection | - |
| PrometheusRule | Alerting rules | - |
| Alertmanager | Alert routing | - |

## Operator Lifecycle Manager

**Group**: `operators.coreos.com/v1alpha1`

| Resource | Purpose | Doc |
|----------|---------|-----|
| ClusterServiceVersion | Operator metadata | - |
| Subscription | Operator installation | - |
| InstallPlan | Operator installation plan | - |
| OperatorGroup | Operator namespace targeting | - |

## API Conventions

See [API Evolution](../practices/development/api-evolution.md) for:
- Versioning (v1alpha1, v1beta1, v1)
- Backward compatibility rules
- Deprecation policy

## Finding APIs

### By Function

**I want to...**
- Configure cluster → `config.openshift.io`
- Manage nodes → `machine.openshift.io`
- Configure networking → `operator.openshift.io` (Network)
- Expose services → `route.openshift.io`

### By Component

See [Repository Index](./repo-index.md) to find component, then check its API types.

## API Documentation

**Upstream Kubernetes**: https://kubernetes.io/docs/reference/kubernetes-api/

**OpenShift API**: https://github.com/openshift/api

**API Conventions**: https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md

## See Also

- [Domain Concepts](../domain/) - Detailed concept explanations
- [Repository Index](./repo-index.md) - Component list
- [API Evolution](../practices/development/api-evolution.md) - API guidelines
