# ClusterVersion

**Category**: OpenShift Platform API  
**API Group**: config.openshift.io/v1  
**Last Updated**: 2026-05-26
**Scope**: All form factors (see HCP note below)

## Overview

ClusterVersion represents the desired and current version of the OpenShift cluster. The Cluster Version Operator (CVO) watches this resource and orchestrates upgrades.

**Singleton**: Only one ClusterVersion exists, named `version`.

**⚠️ Form Factor Note**: In **Hypershift/HCP**:
- **Guest cluster**: Has ClusterVersion (managed by CVO running in guest cluster)
- **Management cluster**: Has its own separate ClusterVersion (for management cluster infrastructure)
- **HostedCluster API** (in management cluster): Drives version upgrades for the guest cluster
  - `HostedClusterStatus.Version` reflects CVO-reported state from guest cluster
  - Control plane pods run in management cluster but serve the guest cluster
- Control plane (management-side) and data plane (guest cluster) have **independent** version lifecycles

See [hypershift-control-plane-version-status.md](../../../enhancements/hypershift/hypershift-control-plane-version-status.md) for authoritative details on HCP version tracking.

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
4. CVO applies manifests in runlevel order:
   - Runlevel 00-09: Core platform (network, DNS, certs)
   - Runlevel 10-29: Kubernetes operators (API server, controllers, scheduler)
   - Runlevel 30+: Other operators (Machine API, OLM, OpenShift core)
   - Components at same runlevel apply in parallel
   ↓
5. CVO waits for each operator to reach expected state:
   - Operator version matches desired version
   - Operator reports Progressing=False
   - Operator reports Available=True
   ↓
6. Once all operators reach expected state:
   - CVO sets status.conditions.Progressing=False
   - CVO adds entry to status.history with state=Completed
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
- **Not supported**: N → N+2 version skips (e.g., 4.14 → 4.16)

### EUS (Extended Update Support) Releases

**What EUS provides:**
- **Extended support lifecycle**: Designated releases receive ~14 months additional support
  - **OCP 4.x**: Even-numbered releases (4.8, 4.10, 4.12, 4.14, 4.16, etc.)
  - **OCP 5.x**: Every third release (5.2, 5.5, 5.8, etc.)
- **EUS-to-EUS upgrade path**: Streamlined upgrade with reduced worker node reboots

**What EUS does NOT provide:**
- ❌ **Version skipping**: Must still upgrade sequentially through all intermediate versions
- ❌ **Control plane version skipping**: Control plane goes through every version (4.12→4.13→4.14→4.15→4.16)

**EUS-to-EUS upgrade process** (e.g., 4.12 to 4.16):
1. Switch from `eus-4.12` channel to `eus-4.16` channel
2. Cluster upgrades sequentially: 4.12 → 4.13 → 4.14 → 4.15 → 4.16
3. Worker node reboots optimized (not control plane version steps)

**References:**
- [Red Hat OpenShift EUS Policy](https://access.redhat.com/support/policy/updates/openshift-eus) - Official EUS support policy
- [EUS Upgrades MVP Enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/update/eus-upgrades-mvp.md) - "does not attempt to remove *any* steps along the serial upgrade path from 4.6 to 4.7 to 4.8 to 4.9 to 4.10"

### Upgrade Paths

```yaml
# Valid upgrade graph (arrows show valid upgrade paths)
4.14.0 → 4.14.1 → 4.14.2
  ↓         ↓         ↓
4.15.0 → 4.15.1 → 4.15.2
  ↓         ↓         ↓
4.16.0 → 4.16.1 → 4.16.2
```

**CVO validates**: Upgrade path exists in Cincinnati graph

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

CVO applies manifests in lexicographic order by filename during upgrades. Manifests use the naming convention:
`0000_<runlevel>_<component>_<manifest>.yaml`

**Assigned runlevels** (from [CVO dev docs](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/operators.md)):
- **00-04**: CVO itself
- **05**: cluster-config-operator
- **07**: Network operator
- **08**: DNS operator
- **09**: Service certificate authority, machine approver
- **10-29**: Kubernetes operators (e.g., kube-apiserver, kube-controller-manager, kube-scheduler)
- **30-39**: Machine API
- **50-59**: Operator Lifecycle Manager (OLM)
- **60-69**: OpenShift core operators

**Key behaviors**:
- Components at same runlevel execute in **parallel**
- Lower runlevels complete before higher runlevels start
- Ordering only applies during **upgrades** (not initial install or reconciliation)
- Within a component, manifests apply in alphabetical order

## Overrides

```yaml
spec:
  overrides:
  - kind: Deployment
    name: my-operator
    namespace: openshift-my-operator
    unmanaged: true  # CVO won't update this
```

**⚠️ WARNING**: 
- Setting `unmanaged: true` prevents CVO from updating the resource
- **Blocks cluster upgrades** until override is removed
- Per [upgrade-acknowledgment-gate enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/update/upgrades-blocking-on-ack.md): "CVO currently blocks minor level upgrades when overrides are set"
- Puts cluster in unsupported state
- **Use only for emergency debugging**
- Remove all overrides before attempting upgrades

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
