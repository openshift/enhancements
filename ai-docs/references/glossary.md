# Glossary

Core stable terms for OpenShift platform. Only includes terms that won't change frequently.

## Platform Terms

| Term | Definition |
|------|------------|
| **ClusterOperator** | Status resource for platform components (Available/Progressing/Degraded reporting) |
| **CVO** | Cluster Version Operator - orchestrates cluster upgrades |
| **CRD** | CustomResourceDefinition - extends Kubernetes API with new resource types |
| **etcd** | Distributed key-value store, backend for Kubernetes API server |
| **Machine** | API resource representing a node in the cluster (via Machine API) |
| **MachineConfig** | Configuration applied to nodes (files, systemd units, kernel args) |
| **MCO** | Machine Config Operator - manages node configuration |
| **Operator** | Controller + CRD, manages lifecycle of applications/services |
| **RHCOS** | Red Hat CoreOS - immutable operating system for OpenShift nodes |
| **rpm-ostree** | Package/update system for immutable operating systems |

## Kubernetes Terms

| Term | Definition |
|------|------------|
| **Admission Controller** | Plugin that intercepts API requests (validation, mutation) |
| **controller-runtime** | Go library for building Kubernetes controllers/operators |
| **envtest** | Integration testing framework (API server + etcd) |
| **Reconciliation** | Controller loop that converges current state to desired state |
| **Watch** | Mechanism to receive notifications when resources change |

## API Terms

| Term | Definition |
|------|------------|
| **Hub Version** | Storage version for API (other versions convert to this) |
| **ObservedGeneration** | Status field tracking last reconciled spec version |
| **Spec** | Desired state (user input) |
| **Status** | Current state (controller output) |
| **Subresource** | Secondary API endpoint (e.g., `/status`, `/scale`) |

## Upgrade Terms

| Term | Definition |
|------|------------|
| **Channel** | Update stream (stable, fast, candidate, eus) |
| **Cincinnati** | Update recommendation service (provides upgrade graph) |
| **EUS** | Extended Update Support - long-term stable releases |
| **N→N+1** | Version skew policy (support current and next minor version) |
| **Version Skew** | Difference between component versions during upgrade |

## Testing Terms

| Term | Definition |
|------|------------|
| **E2E Test** | End-to-end test on full cluster |
| **Flaky Test** | Test that intermittently fails |
| **Integration Test** | Test with multiple components (e.g., envtest) |
| **Unit Test** | Test of single function/method (mocked dependencies) |

## Status Conditions

| Term | Definition |
|------|------------|
| **Available** | Component is functional and serving requests |
| **Degraded** | Component is impaired but still functioning |
| **Progressing** | Component is reconciling changes (upgrade, config) |
| **Upgradeable** | Safe to upgrade (False blocks cluster upgrade) |

## Anti-Staleness Note

This glossary includes only **stable core terms** that are unlikely to change. Avoid adding:
- Release-specific features
- Component-specific terminology
- Temporary or experimental terms

For detailed API documentation, use `oc explain <resource>`.

## Related

- **API Reference**: [api-reference.md](api-reference.md)
- **Full API Docs**: [Kubernetes API Reference](https://kubernetes.io/docs/reference/kubernetes-api/)
