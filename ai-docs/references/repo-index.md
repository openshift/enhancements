# Repository Index

**Purpose**: Discover OpenShift component repositories  
**Last Updated**: 2026-04-29  

## Finding Repositories

**Primary Method**: [GitHub Organization Search](https://github.com/orgs/openshift/repositories)

**Common Patterns**:
- `openshift/<component>-operator` - Operators
- `openshift/<component>` - Core components
- `openshift/api` - API definitions
- `openshift/enhancements` - Design docs

## Core Platform Components

| Component | Repository | Description |
|-----------|-----------|-------------|
| **Cluster Version Operator** | [cluster-version-operator](https://github.com/openshift/cluster-version-operator) | Upgrade orchestration |
| **Machine API** | [machine-api-operator](https://github.com/openshift/machine-api-operator) | Node lifecycle |
| **Kube API Server** | [cluster-kube-apiserver-operator](https://github.com/openshift/cluster-kube-apiserver-operator) | API server operator |
| **etcd** | [cluster-etcd-operator](https://github.com/openshift/cluster-etcd-operator) | etcd cluster operator |
| **Networking** | [cluster-network-operator](https://github.com/openshift/cluster-network-operator) | SDN/OVN orchestration |

## API Definitions

| Repository | Purpose |
|-----------|---------|
| [openshift/api](https://github.com/openshift/api) | All OpenShift API types |
| [kubernetes/api](https://github.com/kubernetes/api) | Kubernetes core APIs |

## Search Strategies

### By Keyword

```bash
# Search GitHub org
https://github.com/orgs/openshift/repositories?q=<keyword>

# Example: storage-related repos
https://github.com/orgs/openshift/repositories?q=storage
```

### By Topic

```bash
# Operator repos
https://github.com/orgs/openshift/repositories?q=operator

# API repos
https://github.com/orgs/openshift/repositories?q=api
```

### By Language

```bash
# Go repos (most operators)
https://github.com/orgs/openshift/repositories?language=go
```

## Operator Naming Conventions

| Pattern | Example | Component Type |
|---------|---------|---------------|
| `cluster-<component>-operator` | `cluster-etcd-operator` | Core platform |
| `<component>-operator` | `machine-api-operator` | Platform feature |
| `<component>` | `installer` | Tool/utility |

## Finding Component Ownership

**OWNERS Files**: Each repo has `OWNERS` file with approvers/reviewers

```bash
# View owners
curl https://raw.githubusercontent.com/openshift/<repo>/master/OWNERS
```

## Anti-Staleness Strategy

**Don't maintain exhaustive lists** (they go stale):
- ❌ List of all 200+ repositories
- ❌ Component ownership mapping
- ❌ Repository statistics

**Do provide search strategies**:
- ✅ GitHub org search links
- ✅ Naming conventions
- ✅ Core platform components only

## Related

- **GitHub Org**: [github.com/openshift](https://github.com/openshift)
- **API Reference**: [api-reference.md](api-reference.md)
- **Enhancement Repo**: [openshift/enhancements](https://github.com/openshift/enhancements)
