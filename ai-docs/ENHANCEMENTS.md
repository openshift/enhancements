# Enhancement Proposals Catalog — openshift/enhancements

## About This Repository

This repository IS the enhancement proposal repository for OpenShift. It contains 600+ design proposals across 78 domain areas. This catalog highlights the major domain areas and provides navigation guidance.

## Enhancement Domains

### Core Platform

| Domain | Directory | Focus |
|--------|-----------|-------|
| Update/CVO | `enhancements/update/` | Cluster Version Operator, OTA updates, upgrade orchestration |
| Machine Config | `enhancements/machine-config/` | MCO, node configuration, OS lifecycle, RHCOS layering |
| Installer | `enhancements/installer/` | IPI, UPI, platform-specific installation |
| Authentication | `enhancements/authentication/` | OAuth, OIDC, token management, identity providers |
| API Review | `enhancements/api-review/` | API design decisions, review process |

### Networking & Ingress

| Domain | Directory | Focus |
|--------|-----------|-------|
| Network | `enhancements/network/` | OVN-Kubernetes, CNO, network policy, multi-network |
| Ingress | `enhancements/ingress/` | Route, IngressController, TLS |
| DNS | `enhancements/dns/` | CoreDNS, cluster DNS configuration |

### Compute & Node

| Domain | Directory | Focus |
|--------|-----------|-------|
| Machine API | `enhancements/machine-api/` | Machine management, autoscaling |
| Cluster API | `enhancements/cluster-api/` | CAPI integration |
| Node Tuning | `enhancements/node-tuning/` | NTO, performance profiles, kernel tuning |
| Autoscaling | `enhancements/autoscaling/` | HPA, VPA, cluster autoscaler |

### Storage

| Domain | Directory | Focus |
|--------|-----------|-------|
| Storage | `enhancements/storage/` | CSI drivers, PVs, snapshots, data protection |
| Local Storage | `enhancements/local-storage/` | Local volume provisioner |
| etcd | `enhancements/etcd/` | etcd operator, backup, encryption |

### Observability & Operations

| Domain | Directory | Focus |
|--------|-----------|-------|
| Monitoring | `enhancements/monitoring/` | Prometheus, alerting, metrics |
| Observability | `enhancements/observability/` | Distributed tracing, logging |
| Cluster Logging | `enhancements/cluster-logging/` | Log collection, forwarding |
| Insights | `enhancements/insights/` | Insights operator, telemetry |

### Topology & Deployment

| Domain | Directory | Focus |
|--------|-----------|-------|
| Hypershift | `enhancements/hypershift/` | Hosted Control Planes architecture |
| MicroShift | `enhancements/microshift/` | Edge deployment, minimal footprint |
| Single Node | `enhancements/single-node/` | SNO-specific features |
| Baremetal | `enhancements/baremetal/` | Bare metal IPI, BMH management |
| Multi-Arch | `enhancements/multi-arch/` | Multi-architecture support |

### Workloads & Extensions

| Domain | Directory | Focus |
|--------|-----------|-------|
| OLM | `enhancements/olm/` | Operator Lifecycle Manager |
| Builds | `enhancements/builds/` | Build strategies, Shipwright |
| Console | `enhancements/console/` | Web console features |
| Image Registry | `enhancements/image-registry/` | Internal registry |
| Windows Containers | `enhancements/windows-containers/` | Windows node support |

## Searching Enhancements

```bash
# Find all EPs in a domain
ls enhancements/<domain>/

# Search by keyword across all EPs
grep -rl "keyword" enhancements/

# Find by status
grep -rl "status: implementable" enhancements/

# Find recently updated EPs
git log --since="6 months ago" --name-only --pretty=format: -- enhancements/ | sort -u | head -30
```

## Related Resources

- [Enhancement Template](../guidelines/enhancement_template.md) — Required format for all EPs
- [Enhancement Process Guide](../guidelines/README.md) — How to propose, review, and approve
- [Kubernetes Enhancement Proposals](https://github.com/kubernetes/enhancements) — Upstream KEPs that OpenShift EPs may reference
