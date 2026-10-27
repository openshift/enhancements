# Enhancement Catalog: openshift/enhancements

## Overview

This repository contains 70+ enhancement domain areas with hundreds of design proposals. This catalog highlights the major areas by EP count and key proposals. Browse `enhancements/<domain>/` for complete listings.

## Major Enhancement Areas

| Domain | EP Count | Key Topics |
|--------|----------|------------|
| `network/` | 55+ | OVN-Kubernetes, network policy, dual-stack, SCTP, EgressIP, bond CNI |
| `ingress/` | 53+ | Route sharding, IngressController, TLS, HAProxy tuning, HTTP/2 |
| `installer/` | 44+ | IPI/UPI platforms, agent-based install, bootstrap, cloud credentials |
| `microshift/` | 40+ | Edge deployment, minimal footprint, config-file driven, rpm-ostree updates |
| `cluster-logging/` | 37+ | Vector, Loki, log forwarding, ClusterLogForwarder API |
| `storage/` | 28+ | CSI drivers, PV lifecycle, snapshot/restore, SharedResource |
| `hypershift/` | 27+ | Hosted Control Planes, management/guest split, multi-cluster |
| `authentication/` | 23+ | OAuth, OIDC, external identity providers, bound tokens |
| `machine-api/` | 22+ | Machine lifecycle, MachineSet scaling, MachineHealthCheck |
| `update/` | 18+ | CVO orchestration, upgrade ordering, OTA, channel management |
| `machine-config/` | 18+ | MCO, MachineConfig, OS layering, node configuration |
| `olm/` | 15+ | Operator Lifecycle Manager, catalog, subscription |
| `console/` | 14+ | Web console plugins, dynamic plugins, UI extensions |
| `kube-apiserver/` | 13+ | API server configuration, audit logging, encryption |
| `baremetal/` | 13+ | Bare metal IPI, BMC management, metal3 |
| `monitoring/` | 12+ | Prometheus, alerting, user workload monitoring |
| `windows-containers/` | 12+ | Windows node support, WMCO |
| `oc/` | 10+ | CLI improvements, oc-mirror, oc-adm |
| `etcd/` | 9+ | Etcd operator, backup/restore, encryption |
| `security/` | 8+ | SCC, pod security admission, FIPS, compliance |
| `scheduling/` | 5+ | Descheduler, topology-aware, NUMA |

## Cross-Cutting Enhancement Areas

| Area | Location | Scope |
|------|----------|-------|
| Multi-architecture | `enhancements/multi-arch/` | ARM64, s390x, ppc64le support |
| Cluster API | `enhancements/cluster-api/` | CAPI integration, machine management |
| Node tuning | `enhancements/node-tuning/` | Performance profiles, kernel tuning |
| Workload partitioning | `enhancements/workload-partitioning/` | CPU pinning, management workloads |
| Release/ART | `enhancements/art/`, `enhancements/release/` | Build pipeline, payload management |
| Testing platform | `enhancements/test-platform/`, `enhancements/testing/` | CI infrastructure, test frameworks |

## Key Standalone Proposals

Several cross-cutting EPs live at the `enhancements/` root level:

| File | Topic |
|------|-------|
| `compact-clusters.md` | 3-node clusters (combined control plane + worker) |
| `arbiter-clusters.md` | Arbiter node for etcd quorum in 2-datacenter deployments |
| `external-control-plane-topology.md` | External control plane topology type |
| `host-network-configuration.md` | Host network configuration management |
| `coreos-bootimages.md` | CoreOS boot image management |

## Upstream References

- [Kubernetes Enhancement Proposals (KEPs)](https://github.com/kubernetes/enhancements)
- [OpenShift API repository](https://github.com/openshift/api)
- [OpenShift Documentation](https://docs.openshift.com)
