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

**GitHub Organization Search**: `https://github.com/orgs/openshift/repositories?q=<keyword>`

**Examples**:
```bash
# By component name
https://github.com/orgs/openshift/repositories?q=storage
https://github.com/orgs/openshift/repositories?q=network

# By type
https://github.com/orgs/openshift/repositories?q=operator
https://github.com/orgs/openshift/repositories?q=library

# By language
https://github.com/orgs/openshift/repositories?language=go
https://github.com/orgs/openshift/repositories?language=python
```

## Operator Naming Conventions

| Pattern | Example | Notes |
|---------|---------|-------|
| `cluster-<component>-operator` | `cluster-etcd-operator` | Historical naming (directory grouping) |
| `<component>-operator` | `machine-api-operator` | Standard operator naming |
| `<component>` | `installer` | Tool/utility (non-operator) |

**Note**: The `cluster-` prefix was historically used for directory organization in early OpenShift development, not to indicate semantic differences between operator types. Do not infer component type or importance from naming patterns alone.

## Finding Component Ownership

**OWNERS Files**: Each repo has `OWNERS` file with approvers/reviewers

```bash
# View owners (most repos use 'main', some older repos use 'master')
curl https://raw.githubusercontent.com/openshift/<repo>/main/OWNERS

# Or use GitHub API to auto-detect default branch:
gh api repos/openshift/<repo> --jq '.default_branch' | \
  xargs -I {} curl https://raw.githubusercontent.com/openshift/<repo>/{}/OWNERS
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
