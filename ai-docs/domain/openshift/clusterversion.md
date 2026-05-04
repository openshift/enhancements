# ClusterVersion

**Category**: OpenShift Platform API  
**API Group**: config.openshift.io/v1  
**Last Updated**: 2026-04-29  

## Overview

ClusterVersion represents the desired and current version of the OpenShift cluster. The Cluster Version Operator (CVO) watches this resource and orchestrates upgrades.

**Singleton**: Only one ClusterVersion exists, named `version`.

## Key Fields

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version  # Always "version"
spec:
  channel: stable-4.16
  clusterID: a1b2c3d4-5678-90ab-cdef-1234567890ab
  desiredUpdate:
    version: 4.16.1
    image: quay.io/openshift-release-dev/ocp-release@sha256:...
  upstream: https://api.openshift.com/api/upgrades_info/v1/graph
status:
  availableUpdates:
  - version: 4.16.1
    image: quay.io/openshift-release-dev/ocp-release@sha256:...
  - version: 4.16.2
    image: quay.io/openshift-release-dev/ocp-release@sha256:...
  conditions:
  - type: Available
    status: "True"
    reason: AsExpected
  - type: Progressing
    status: "False"
    reason: AsExpected
  desired:
    version: 4.16.0
    image: quay.io/openshift-release-dev/ocp-release@sha256:...
  history:
  - state: Completed
    version: 4.16.0
    image: quay.io/openshift-release-dev/ocp-release@sha256:...
    startedTime: "2026-04-01T10:00:00Z"
    completionTime: "2026-04-01T11:30:00Z"
  observedGeneration: 5
  versionHash: abc123def456
```

## Key Concepts

- **Desired Update**: Target version set by admin
- **Available Updates**: Valid upgrade paths from upstream
- **History**: Past upgrade attempts and outcomes
- **Channel**: Update stream (stable, fast, candidate, eus)
- **CVO**: Cluster Version Operator orchestrates the upgrade

## Upgrade Flow

```
1. Admin sets spec.desiredUpdate
   ↓
2. CVO validates update (N→N+1, supported path)
   ↓
3. CVO sets status.conditions.Progressing=True
   ↓
4. CVO updates operators in order:
   - etcd
   - kube-apiserver
   - kube-controller-manager
   - kube-scheduler
   - other operators (parallel)
   ↓
5. Each operator reports Progressing=True → False
   ↓
6. CVO sets status.conditions.Progressing=False
   ↓
7. CVO adds entry to status.history
```

## Channels

| Channel | Purpose | Update Frequency |
|---------|---------|------------------|
| **stable-X.Y** | Production | After candidate soak |
| **fast-X.Y** | Early adopters | Weekly |
| **candidate-X.Y** | Pre-release testing | Daily |
| **eus-X.Y** | Extended Update Support | Long-term stable |

**Example**: `stable-4.16`, `fast-4.17`, `eus-4.14`

## Triggering Upgrade

```bash
# View current version
oc get clusterversion

# View available updates
oc adm upgrade

# Start upgrade to specific version
oc adm upgrade --to=4.16.1

# Start upgrade to latest
oc adm upgrade --to-latest
```

## Monitoring Upgrade

```bash
# Watch upgrade progress
oc get clusterversion -w

# View operator status
oc get clusteroperators

# View progressing operators
oc get co -o json | jq -r '.items[] | select(.status.conditions[] | select(.type=="Progressing" and .status=="True")) | .metadata.name'

# View CVO logs
oc logs -n openshift-cluster-version deployment/cluster-version-operator
```

## Upgrade Policies

### Version Skew

- **Supported**: N → N+1 (e.g., 4.15 → 4.16)
- **Not supported**: N → N+2 (e.g., 4.14 → 4.16)
- **EUS exception**: EUS releases allow skipping one version

### Upgrade Paths

```yaml
# Valid upgrade graph
4.14.0 → 4.14.1 → 4.14.2
  ↓         ↓         ↓
4.15.0 ← 4.15.1 ← 4.15.2
  ↓         ↓         ↓
4.16.0 ← 4.16.1 ← 4.16.2
```

**CVO validates**: Upgrade path exists in Cincinnati graph

## Pausing Upgrades

```bash
# Pause cluster upgrades (for maintenance)
oc adm upgrade --pause

# Resume upgrades
oc adm upgrade --resume
```

## Upgrade Conditions

| Condition | Meaning | True When |
|-----------|---------|-----------|
| **Available** | CVO is functional | CVO pod running |
| **Progressing** | Upgrade in progress | Operators being updated |
| **Failing** | Upgrade failed | Operator reconciliation failed |
| **RetrievedUpdates** | Update graph fetched | Cincinnati API reachable |

## History

```yaml
status:
  history:
  - state: Completed
    version: 4.16.0
    completionTime: "2026-04-01T11:30:00Z"
  - state: Completed
    version: 4.15.5
    completionTime: "2026-03-01T10:00:00Z"
  - state: Partial
    version: 4.15.4
    startedTime: "2026-02-15T09:00:00Z"
    # No completionTime = upgrade failed/rolled back
```

**States**: Completed, Partial

## CVO Operator Ordering

```yaml
# manifests/0000_*.yaml files in release image
0000_00_cluster-version-operator_*.yaml
0000_50_cluster-etcd-operator_*.yaml
0000_60_cluster-kube-apiserver-operator_*.yaml
0000_70_cluster-kube-controller-manager-operator_*.yaml
0000_80_cluster-kube-scheduler-operator_*.yaml
0000_90_*  # All other operators (parallel)
```

**Ordering ensures**: etcd → API server → controllers → operators

## Overrides

```yaml
spec:
  overrides:
  - kind: Deployment
    name: my-operator
    namespace: openshift-my-operator
    unmanaged: true  # CVO won't update this
```

**Use case**: Prevent CVO from managing specific resources (debugging only)

## Examples

### View ClusterVersion

```bash
$ oc get clusterversion version -o yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: stable-4.16
  clusterID: abc-123
status:
  desired:
    version: 4.16.0
  history:
  - state: Completed
    version: 4.16.0
```

### Upgrade Status

```bash
$ oc get clusterversion
NAME      VERSION   AVAILABLE   PROGRESSING   SINCE   STATUS
version   4.16.0    True        False         5h      Cluster version is 4.16.0
```

## Best Practices

1. **Use Recommended Updates**: CVO validates upgrade paths
   ```bash
   oc adm upgrade  # Shows recommended updates only
   ```

2. **Monitor Operator Health Before Upgrade**:
   ```bash
   oc get co | grep -v "True.*False.*False"
   ```

3. **Check Upgrade Graph**: Ensure path exists
   ```bash
   oc adm upgrade --to=4.16.1  # Validates before starting
   ```

4. **Watch Operator Progress**: Don't interrupt during upgrade
   ```bash
   watch oc get co
   ```

## Antipatterns

❌ **Version skipping**: 4.14 → 4.16 (use intermediate version)  
❌ **Upgrading degraded cluster**: Fix issues first  
❌ **Manual operator updates**: Always use CVO  
❌ **Interrupting upgrade**: Can leave cluster in inconsistent state

## References

- **API**: `oc explain clusterversion`
- **Source**: [github.com/openshift/api/config/v1](https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go)
- **CVO**: [cluster-version-operator](https://github.com/openshift/cluster-version-operator)
- **Docs**: [Updating Clusters](https://docs.openshift.com/container-platform/latest/updating/index.html)
- **Pattern**: [upgrade-strategies.md](../../platform/openshift-specifics/upgrade-strategies.md)
