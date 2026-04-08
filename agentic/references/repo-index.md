# OpenShift Repository Index

**Last Updated**: 2026-04-08  

## Purpose

Map of all OpenShift component repositories with their agentic documentation status.

## Core Platform

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [machine-config-operator](https://github.com/openshift/machine-config-operator) | OS configuration and updates | [AGENTS.md](https://github.com/openshift/machine-config-operator/blob/master/AGENTS.md) | [agentic/](https://github.com/openshift/machine-config-operator/tree/master/agentic) | ✅ Tier 2 |
| [cluster-version-operator](https://github.com/openshift/cluster-version-operator) | Platform upgrades | Planned | Planned | ⏳ Planned |
| [installer](https://github.com/openshift/installer) | Cluster installation | Planned | Planned | ⏳ Planned |
| [cluster-etcd-operator](https://github.com/openshift/cluster-etcd-operator) | etcd lifecycle | Planned | Planned | 📝 Not started |

## Networking

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [cluster-network-operator](https://github.com/openshift/cluster-network-operator) | SDN/OVN networking | Planned | Planned | 📝 Not started |
| [ovn-kubernetes](https://github.com/ovn-org/ovn-kubernetes) | OVN implementation | External | External | 📝 Upstream |
| [sdn](https://github.com/openshift/sdn) | OpenShift SDN (deprecated) | N/A | N/A | ⚠️ Deprecated |

## Storage

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [cluster-storage-operator](https://github.com/openshift/cluster-storage-operator) | Storage lifecycle | Planned | Planned | 📝 Not started |
| [csi-operator](https://github.com/openshift/csi-operator) | CSI driver management | Planned | Planned | 📝 Not started |

## Authentication & Authorization

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [cluster-authentication-operator](https://github.com/openshift/cluster-authentication-operator) | OAuth configuration | Planned | Planned | 📝 Not started |
| [oauth-server](https://github.com/openshift/oauth-server) | OAuth implementation | Planned | Planned | 📝 Not started |

## Monitoring & Observability

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [cluster-monitoring-operator](https://github.com/openshift/cluster-monitoring-operator) | Prometheus setup | Planned | Planned | 📝 Not started |
| [cluster-logging-operator](https://github.com/openshift/cluster-logging-operator) | Log collection | Planned | Planned | 📝 Not started |

## Developer Experience

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [console-operator](https://github.com/openshift/console-operator) | Web console | Planned | Planned | 📝 Not started |
| [console](https://github.com/openshift/console) | Web console frontend | Planned | Planned | 📝 Not started |
| [oc](https://github.com/openshift/oc) | CLI tool | Planned | Planned | 📝 Not started |

## Machine API

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [machine-api-operator](https://github.com/openshift/machine-api-operator) | Machine lifecycle | Planned | Planned | 📝 Not started |
| [cluster-api-provider-aws](https://github.com/openshift/cluster-api-provider-aws) | AWS provider | Planned | Planned | 📝 Not started |
| [cluster-api-provider-azure](https://github.com/openshift/cluster-api-provider-azure) | Azure provider | Planned | Planned | 📝 Not started |

## Image Registry

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [cluster-image-registry-operator](https://github.com/openshift/cluster-image-registry-operator) | Registry management | Planned | Planned | 📝 Not started |

## Ingress

| Repo | Purpose | AGENTS.md | Agentic Docs | Status |
|------|---------|-----------|--------------|--------|
| [cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator) | Router management | Planned | Planned | 📝 Not started |
| [router](https://github.com/openshift/router) | HAProxy router | Planned | Planned | 📝 Not started |

## By Capability

### OS Management
- **Primary**: [machine-config-operator](https://github.com/openshift/machine-config-operator) (✅ Tier 2)
- **Related**: installer (first-boot), rhcos (OS images)

### Node Lifecycle
- **Primary**: [machine-api-operator](https://github.com/openshift/machine-api-operator)
- **Related**: installer (initial nodes), machine-config-operator (OS)

### Upgrades
- **Primary**: [cluster-version-operator](https://github.com/openshift/cluster-version-operator)
- **Related**: machine-config-operator (OS updates), machine-api-operator (node updates)

### Networking
- **Primary**: [cluster-network-operator](https://github.com/openshift/cluster-network-operator)
- **Related**: ovn-kubernetes, multus, whereabouts

### Monitoring
- **Primary**: [cluster-monitoring-operator](https://github.com/openshift/cluster-monitoring-operator)
- **Related**: prometheus, alertmanager, grafana

## Status Legend

- ✅ **Tier 2 implemented**: Component has AGENTS.md + agentic/ documentation
- ⏳ **Planned**: On roadmap for Tier 2 creation
- 📝 **Not started**: Not yet planned
- ⚠️ **Deprecated**: Component being phased out
- 📦 **Upstream**: External project (Kubernetes, CNCF)

## Finding a Component

### By Function

**I want to...**
- Configure node OS → [machine-config-operator](https://github.com/openshift/machine-config-operator)
- Manage cluster version → [cluster-version-operator](https://github.com/openshift/cluster-version-operator)
- Set up networking → [cluster-network-operator](https://github.com/openshift/cluster-network-operator)
- Provision nodes → [machine-api-operator](https://github.com/openshift/machine-api-operator)
- Monitor cluster → [cluster-monitoring-operator](https://github.com/openshift/cluster-monitoring-operator)

### By API

**I'm working with...**
- `MachineConfig` → [machine-config-operator](https://github.com/openshift/machine-config-operator)
- `ClusterVersion` → [cluster-version-operator](https://github.com/openshift/cluster-version-operator)
- `Machine` → [machine-api-operator](https://github.com/openshift/machine-api-operator)
- `Network` → [cluster-network-operator](https://github.com/openshift/cluster-network-operator)
- `IngressController` → [cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator)

## Contributing

To add Tier 2 docs to your component:
1. See [Tier 2 Specification](https://github.com/openshift/enhancements/blob/master/agentic/SPECIFICATION.md)
2. Use skill: `claude-code skills run agentic-docs-maintainer:tier2-lean`
3. Update this index in PR

## See Also

- [Enhancement Index](./enhancement-index.md) - Browse enhancement proposals
- [Glossary](./glossary.md) - OpenShift terminology
- [API Reference](./api-reference.md) - Platform APIs
