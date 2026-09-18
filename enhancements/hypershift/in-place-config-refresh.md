---
title: in-place-config-refresh
authors:
  - "@cewong"
reviewers:
  - "@mraee"
  - "@enxebre"
  - "@yuqi-zhang"
approvers:
  - "@enxebre"
api-approvers:
  - None
creation-date: 2026-07-01
last-updated: 2026-07-07
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3299
see-also:
  - "/enhancements/hypershift/node-lifecycle.md"
---

# In-Place Config Refresh for NodePool Worker Nodes

## Summary

This enhancement introduces a config refresh mechanism for HyperShift NodePool
worker nodes that delivers non-disruptive configuration changes — such as
management-side image digest bumps, SSH key rotations, trust bundle updates,
proxy configuration, registry policies, and registry mirror changes — to
existing nodes in-place without node replacement, drain, or reboot. It extends
the hash split from OCPSTRAT-3298 so that user-driven HostedCluster spec
fields (e.g., `sshKey`, `pullSecret`, `additionalTrustBundle`,
`imageContentSources`, `configuration.proxy`, `configuration.image`) are also
excluded from the rollout hash and instead covered by the payload hash, making
them eligible for in-place refresh alongside management-side content. When the
config refresh controller detects a payload hash change with no rollout hash
change, it delivers the full updated ignition payload to nodes via MCD pods
(the same mechanism as the existing in-place upgrader). Safe file paths and
their post-apply actions are communicated to MCD via an additional key in the
existing upgrade ConfigMap, so MCD can apply changes without rebooting. The
mechanism works regardless of whether the NodePool uses a Replace or InPlace
upgrade strategy.

## Motivation

HyperShift computes an ignition configuration for each NodePool by assembling
multiple sources: core configs from the CPO, HAProxy/API server proxy config,
user-provided MachineConfigs, NTO configs, and global config from the
HostedCluster spec. A companion feature (OCPSTRAT-3298 — Predictable NodePool
Rollout Control) introduces a hash split so that management-side changes no
longer trigger Replace rollouts. However, OCPSTRAT-3298's hash split only
covers management-side content (CPO-generated configs, HAProxy image refs,
etc.). User-driven HostedCluster spec changes — such as SSH key rotations,
trust bundle updates, pull secret changes, proxy configuration, and registry
policy changes — still trigger Replace rollouts under OCPSTRAT-3298 because
they remain in the rollout hash.

This feature extends the hash split to also exclude these user-driven
HostedCluster spec fields from the rollout hash, and introduces a config
refresh controller that delivers these non-disruptive changes to existing
nodes in-place — generalizing HyperShift's existing in-place upgrade pattern
to cover all NodePools.

### User Stories

* As a managed service operator (ROSA HCP, ARO HCP), I want management-side
  configuration changes like SSH key rotations and trust bundle updates to be
  delivered to running worker nodes without triggering full node replacement, so
  that security-sensitive changes take effect promptly without customer workload
  disruption.

* As a self-managed HyperShift administrator, I want registry mirror
  configuration and pull secret changes to be applied to existing nodes
  in-place, so that I do not have to manually trigger a rollout to propagate
  these changes across my fleet.

* As an SRE operating ROSA HCP at scale, I want the config refresh mechanism to
  respect maxUnavailable settings and track refresh state separately from
  upgrade state, so that I can monitor progress and ensure the mechanism does
  not interfere with ongoing rollouts.

* As a platform operator, I want the system to guarantee that no config refresh
  operation will reboot a node, so that I can trust the mechanism to be
  non-disruptive for all eligible changes.

### Goals

1. Non-disruptive configuration changes are delivered to existing worker nodes
   without node replacement, drain, or reboot.
2. The config refresh mechanism works for NodePools with Replace upgrade
   strategy (MachineDeployment-managed nodes), not just InPlace.
3. The mechanism guarantees no node reboots by classifying inputs into rollout
   hash (reboot-requiring) and payload hash (refresh-eligible) categories, with
   defense-in-depth validation in MCD.
4. Changes that require a reboot are excluded from the refresh path and
   deferred to the next Replace rollout.
5. NodePool status reports the refresh state, indicating whether nodes are
   up-to-date with the latest non-disruptive config or have a refresh pending.

### Non-Goals

1. Delivering changes that require a reboot (FIPS, kernel arguments, OS image,
   kernel type, extensions) via the refresh path. These continue to require a
   Replace rollout.
2. Changing the behavior of the InPlace upgrade strategy for full version
   upgrades. The config refresh is a complementary mechanism for non-disruptive
   changes, not a replacement for the existing upgrade lifecycle.
3. User-facing API changes to NodePool spec. The config refresh mechanism is
   transparent to the cluster administrator — it operates automatically based
   on the hash split introduced by OCPSTRAT-3298.
4. Supporting config refresh for standalone (non-HyperShift) clusters. The MCO
   handles config delivery in standalone clusters through its own mechanisms.

## Proposal

This feature extends the hash split from OCPSTRAT-3298 and introduces a config
refresh controller in the Hosted Cluster Config Operator (HCCO).

**Hash split extension**: OCPSTRAT-3298 introduces two hashes — a rollout hash
that triggers node replacement and a payload hash that captures the full
ignition content. This enhancement extends the hash split so that user-driven
HostedCluster spec fields that produce non-disruptive changes are excluded from
the rollout hash. The classification operates on **inputs** (which spec field
changed), not outputs (which files changed in the ignition payload):

- **Rollout hash inputs** (changes trigger node replacement): release image,
  NodePool `.config` (user MachineConfigs), FIPS mode, kernel arguments, kernel
  type, extensions, OS image.
- **Payload-only inputs** (changes eligible for in-place refresh):
  `spec.sshKey`, `spec.pullSecret`, `spec.additionalTrustBundle`,
  `spec.imageContentSources`, `spec.configuration.proxy`,
  `spec.configuration.image`, and management-side content (CPO-generated
  configs, HAProxy image refs, API server proxy scripts).

When the payload hash changes but the rollout hash does not, the controller
knows that only refresh-eligible inputs changed and delivers the full updated
ignition payload to nodes via MCD pods — the same mechanism used by the
existing in-place upgrader. The controller does not compute a file-level diff;
MCD handles that by comparing the desired config against the current config
read from disk (`/etc/mcd-currentconfig.json`).

The controller includes a `no-reboot-paths` key in the upgrade ConfigMap,
listing all refresh-eligible file paths and their post-apply actions. MCD reads
this key and uses it to determine the correct post-apply action (none, restart,
reload) for each changed file instead of defaulting to reboot. If the key is
absent, MCD falls back to its existing legacy behavior (backward compatible).
This requires a small change to MCD's HyperShift mode
(`syncNodeHypershift`).

### Workflow Description

**service operator** is the ROSA HCP / ARO HCP managed service team or a
self-managed HyperShift administrator responsible for operating the management
cluster.

**cluster administrator** is the end user managing workloads on a hosted
cluster.

**config refresh controller** is the automated controller running in the HCCO
within the guest cluster.

#### Normal config refresh flow

1. A refresh-eligible change occurs — either a management-side change (e.g., an
   automated image-updater PR bumps the HAProxy image digest) or a user-driven
   HostedCluster spec change (e.g., an administrator rotates SSH keys, updates
   proxy configuration, or changes registry mirror settings).
2. The NodePool controller in the management cluster renders the new ignition
   config. The payload hash changes, but the rollout hash remains the same
   because none of the rollout hash inputs (release image, NodePool `.config`,
   FIPS, kargs, kernelType, extensions) changed.
3. Because the rollout hash is unchanged, no Replace rollout is triggered. The
   updated ignition payload is stored in the user-data secret.
4. The config refresh controller in the HCCO detects the payload hash change
   by comparing each node's `currentRefreshConfig` annotation against the
   target payload hash.
5. The controller creates the upgrade ConfigMap with the full ignition payload
   and a `no-reboot-paths` key listing all refresh-eligible file paths and
   their post-apply actions.
6. For each eligible node (respecting maxUnavailable), the controller creates
   an MCD pod that mounts this ConfigMap.
7. MCD reads the desired config from the ConfigMap and the current config from
   `/etc/mcd-currentconfig.json` on disk, computes the file-level diff, and
   applies only the changed files. For each changed file, MCD checks the
   `no-reboot-paths` key to determine the post-apply action (none, restart,
   reload) instead of defaulting to reboot.
8. The controller updates the node's refresh annotation to record that the
   refresh is complete.
9. The NodePool status reflects the refresh progress.

#### Changes that require a rollout (excluded from refresh)

1. A change occurs to a rollout hash input — e.g., a release image update,
   NodePool `.config` change (user MachineConfig), FIPS mode, kernel arguments,
   kernel type, or extensions.
2. The rollout hash changes, triggering a Replace rollout via the normal
   MachineDeployment update path.
3. The config refresh controller does not act because the rollout hash changed
   — the Replace rollout delivers the new configuration as part of node
   replacement.

#### Mixed changes (rollout + refresh-eligible inputs change simultaneously)

1. If both rollout hash inputs and payload-only inputs change at the same time
   (e.g., a release image update + SSH key rotation), the rollout hash changes
   and a Replace rollout is triggered.
2. The config refresh controller does not act — the Replace rollout delivers
   both sets of changes as part of node replacement.
3. New nodes created by the rollout receive the full configuration including
   all changes.

#### Interaction with concurrent Replace rollout

1. If a Replace rollout is in progress for a NodePool, the config refresh
   controller pauses refresh operations for the entire NodePool until the
   rollout completes. The controller detects an active rollout by checking
   whether the MachineDeployment's `status.updatedReplicas` differs from
   `status.replicas`, or whether any Machines in the NodePool have a deletion
   timestamp set.
2. The config refresh controller must not create an MCD pod on a node that
   already has an MCD pod running (from the in-place upgrader or a previous
   refresh). Before creating an MCD pod, the controller checks for existing
   MCD pods on the target node and skips the node if one is present.
3. New nodes created by the Replace rollout receive the latest full
   configuration (including both spec-driven and refresh-eligible changes),
   so no post-rollout refresh is needed for those nodes.

#### Interaction with InPlace upgrade strategy

1. If an InPlace-strategy NodePool has both a version upgrade pending and a
   config refresh pending, the version upgrade takes precedence. The upgrade
   delivers the full configuration (including refresh-eligible changes), so
   the refresh is subsumed.
2. The config refresh controller checks whether the existing in-place upgrader
   has an active MCD pod on a node before creating a refresh MCD pod. If an
   upgrade MCD pod is running, the refresh is deferred for that node.
3. After a version upgrade completes, the controller re-evaluates whether a
   config refresh is still needed by comparing the node's
   `currentRefreshConfig` annotation against the target payload hash.

### API Extensions

This enhancement does not introduce new CRDs or modify existing public APIs.

#### Node Annotations

The mechanism uses internal annotations on Node objects to track refresh state:

- `hypershift.openshift.io/currentRefreshConfig`: Records the payload hash of
  the last successfully applied config refresh on a node.

These annotations are implementation details managed by the HCCO and are not
part of the user-facing API contract.

#### NodePool Status Conditions

The config refresh controller reports refresh state via NodePool status
conditions. These conditions follow the standard OpenShift condition semantics:

| Condition Type | Status | Meaning |
| -------------- | ------ | ------- |
| `ConfigRefreshProgressing` | `True` | A config refresh is in progress — MCD pods are running on one or more nodes in the NodePool. The `message` field reports progress (e.g., "Refreshing 2/5 nodes"). |
| `ConfigRefreshProgressing` | `False` | No config refresh is in progress. Either all nodes are up to date, or the refresh is paused (e.g., due to a concurrent rollout). |
| `ConfigRefreshComplete` | `True` | All nodes in the NodePool have been refreshed to the latest payload hash. Set when the last node's `currentRefreshConfig` annotation matches the target. |
| `ConfigRefreshComplete` | `False` | One or more nodes have not yet received the latest payload config. This is the normal state when a new payload hash is detected and refresh has not yet started or is in progress. |
| `ConfigRefreshDegraded` | `True` | A config refresh has failed on one or more nodes — e.g., MCD pod failed, timed out, or a post-apply action returned an error. The `message` field identifies affected nodes and the failure reason. |
| `ConfigRefreshDegraded` | `False` | No refresh failures. |

**Condition transitions**:

1. When a new payload hash is detected (rollout hash unchanged):
   `ConfigRefreshComplete` transitions to `False`,
   `ConfigRefreshProgressing` transitions to `True`.
2. As each node completes refresh: `ConfigRefreshProgressing` `message` is
   updated with progress count.
3. When all nodes complete: `ConfigRefreshProgressing` transitions to `False`,
   `ConfigRefreshComplete` transitions to `True`.
4. If an MCD pod fails: `ConfigRefreshDegraded` transitions to `True`.
   `ConfigRefreshProgressing` remains `True` if refresh continues on other
   nodes.
5. After a Replace rollout completes (all nodes replaced):
   `ConfigRefreshComplete` transitions to `True` (new nodes have latest
   config), `ConfigRefreshProgressing` and `ConfigRefreshDegraded` transition
   to `False`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specific to HyperShift. All components involved operate
within the HyperShift architecture:

**Management cluster components**:
- The NodePool controller computes rollout and payload hashes and updates
  user-data secrets and annotations. No new management cluster components are
  introduced.

**Guest cluster components**:
- The config refresh controller runs as part of the HCCO and creates MCD pods
  in the guest cluster to apply file changes on nodes.
- MCD pods run as privileged pods on target nodes with host filesystem access.

**Cross-cluster communication**:
- The HCCO already has credentials and kubeconfig for the guest cluster. No
  new cross-cluster communication channels are introduced.

**Upgrade orchestration**:
- The config refresh mechanism is independent of the control plane upgrade
  lifecycle. It operates between upgrades to deliver non-disruptive changes.
- During a control plane upgrade, the HCCO is updated first (as part of the
  management-side upgrade), and the new HCCO version includes the config
  refresh controller.

**Resource impact on management cluster**: Negligible. The controller is a
lightweight addition to the existing HCCO process.

#### Standalone Clusters

Not applicable. This enhancement is specific to the HyperShift topology. In
standalone clusters, the MCO manages config delivery to nodes through its own
reconciliation loop and MCD daemonset.

#### Single-node Deployments or MicroShift

The config refresh mechanism applies to HyperShift guest cluster worker nodes.
It is compatible with single-node guest clusters (a NodePool with one node).
The maxUnavailable setting governs pacing; with a single node, the refresh
applies to that one node.

MicroShift is not applicable — MicroShift does not use HyperShift or the MCO.

#### OpenShift Kubernetes Engine

This feature does not depend on OCP-specific platform operators (console,
monitoring, registry). It operates at the node configuration layer using MCD,
which is part of the MCO — a core component present in OKE. The feature is
available in OKE deployments that use HyperShift.

### Implementation Details/Notes/Constraints

#### Built-in Path-to-Action Map

The config refresh controller maintains a built-in map of all file paths (and
ignition `passwd` entries) that refresh-eligible inputs can produce, along with
the required post-apply action for each. This map is compiled into the
controller — it is not a runtime-configurable resource. HyperShift controls all
file paths in the ignition config it generates, so the set of eligible paths is
known at build time.

This map is included in full in the `no-reboot-paths` ConfigMap key whenever a
config refresh is triggered. MCD uses it to determine the correct post-apply
action for each changed file it detects during its file-level diff. The map
does not serve as a safety gate for deciding whether to trigger a refresh —
that decision is made entirely by the input-based hash split.

#### ConfigMap `no-reboot-paths` Key

Today, the in-place upgrader creates a ConfigMap in the guest cluster with two
keys: `config` (gzip+base64 ignition payload) and `hash` (target config
version hash). MCD mounts this ConfigMap and reads both keys as files from the
mount directory (`--desired-configmap` flag).

For config refresh, the controller adds a third key to the ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: <pool>-upgrade
  namespace: <pool>-upgrade
data:
  config: <gzip+base64 ignition payload>
  hash: <target config version hash>
  no-reboot-paths: |
    [
      {"path": "/etc/kubernetes/manifests/kube-apiserver-proxy.yaml", "action": "None"},
      {"path": "/etc/kubernetes/apiserver-proxy-config/haproxy.cfg", "action": "None"},
      {"path": "/usr/local/bin/setup-apiserver-ip.sh", "action": "None"},
      {"path": "/usr/local/bin/teardown-apiserver-ip.sh", "action": "None"},
      {"path": "/etc/pki/ca-trust/source/anchors/", "action": "Restart", "services": ["update-ca-trust", "crio"]},
      {"path": "/etc/containers/registries.conf", "action": "Reload", "services": ["crio"]},
      {"path": "/etc/containers/registries.d/", "action": "Reload", "services": ["crio"]}
    ]
```

The `no-reboot-paths` key contains a JSON array of objects, each specifying:
- `path`: File path (or directory prefix) that should not trigger a reboot
- `action`: Post-apply action type (`None`, `Restart`, `Reload`)
- `services`: List of services to restart/reload (when action is not `None`)

Paths already in MCD's legacy safe list (e.g., `/var/lib/kubelet/config.json`,
SSH keys) do not need to be included — MCD already handles them without
rebooting.

#### MCD Change (MCO)

MCD's `syncNodeHypershift()` function requires a small change to read the
`no-reboot-paths` file from the ConfigMap mount directory. The change is in
`calculatePostConfigChangeAction()` (or in `syncNodeHypershift()` itself
before calling it):

1. Read `filepath.Join(dn.hypershiftConfigMap, "no-reboot-paths")`. If the
   file does not exist, fall back to the existing legacy behavior (backward
   compatible).
2. Parse the JSON array of path/action entries.
3. When computing post-config-change actions for file diffs, check each changed
   file against the no-reboot-paths list in addition to the existing hardcoded
   safe list. If a file matches, use the specified action instead of
   defaulting to reboot.

This is a targeted change to the MCD's HyperShift code path only. The
standalone cluster MCD code path (`syncNode`) is not affected.

#### Config Refresh Controller

The controller (new, or an extension of the existing in-place upgrader in the
HCCO) performs the following:

1. **Watch**: Monitors Nodes and Machines associated with all NodePools (not
   just InPlace-strategy ones).
2. **Detect**: Identifies when the payload hash has changed but the rollout
   hash has not. This is determined by comparing the node's
   `currentRefreshConfig` annotation against the target payload hash. Because
   the hash split classifies inputs at the NodePool controller level, the
   config refresh controller does not need to inspect which specific fields
   changed — a payload-only hash change guarantees that only refresh-eligible
   inputs changed.
3. **Guard**: Verifies that no Replace rollout or InPlace upgrade is active for
   the NodePool (see "Interaction with concurrent Replace rollout" above), and
   that no MCD pod is already running on the target node.
4. **Create ConfigMap**: Creates the upgrade ConfigMap in the guest cluster with
   the full ignition payload (`config`), hash (`hash`), and the
   `no-reboot-paths` key containing entries for all refresh-eligible file paths
   and their post-apply actions. The `no-reboot-paths` list is a static,
   built-in mapping of all paths that refresh-eligible inputs can produce —
   it is included in full regardless of which specific inputs changed.
5. **Apply**: Creates MCD pods on eligible nodes (respecting maxUnavailable).
   MCD reads the desired config from the ConfigMap, computes the file-level
   diff against the current config on disk (`/etc/mcd-currentconfig.json`),
   applies only the changed files, and executes the post-apply actions
   specified in `no-reboot-paths` for each changed file instead of rebooting.
6. **Track**: Updates the `hypershift.openshift.io/currentRefreshConfig`
   annotation on each node after successful application.

#### Safety Invariant

The safety of the config refresh mechanism is guaranteed by the **input-based
hash split** in the NodePool controller, not by file-level diff inspection in
the config refresh controller. The hash split classifies every input to the
ignition payload into one of two categories:

- **Rollout hash inputs**: release image, NodePool `.config` (user
  MachineConfigs), FIPS mode, kernel arguments, kernel type, extensions, OS
  image. Changes to these inputs modify the rollout hash, triggering a Replace
  rollout. The config refresh controller does not act when the rollout hash
  changes.
- **Payload-only inputs**: `spec.sshKey`, `spec.pullSecret`,
  `spec.additionalTrustBundle`, `spec.imageContentSources`,
  `spec.configuration.proxy`, `spec.configuration.image`, and
  management-side content. Changes to these inputs modify only the payload
  hash. The config refresh controller acts only when the payload hash changes
  and the rollout hash does not.

This design ensures that the controller never attempts to refresh a change that
requires a reboot — those changes always flow through the rollout hash and
trigger node replacement.

As a defense-in-depth measure, MCD independently validates that hardcoded
reboot triggers (OS update, kargs, FIPS, kernelType, extensions) are not
present in the diff before honoring `no-reboot-paths`. If MCD detects a
reboot-requiring change, it ignores `no-reboot-paths` and falls back to its
legacy reboot behavior.

#### Eligible vs. Ineligible Changes

The eligibility of a change is determined by which **input** (spec field)
changed, not by which output files changed in the ignition payload. The
allowlist covers both `storage.files` paths and ignition `passwd` section
entries (e.g., `sshAuthorizedKeys`).

**Refresh-eligible inputs** (payload-only hash, no reboot, no drain):

| Input (Spec Field) | Affected File Path(s) | Post-Apply Action |
| ------------------ | --------------------- | ----------------- |
| `spec.sshKey` | `passwd.users[].sshAuthorizedKeys` | None |
| `spec.pullSecret` | `/var/lib/kubelet/config.json` | None |
| `spec.additionalTrustBundle` (deprecated) | `/etc/pki/ca-trust/source/anchors/` | Run `update-ca-trust`, restart crio |
| `spec.imageContentSources` | `/etc/containers/registries.conf`, `/etc/containers/registries.d/` | Reload crio |
| `spec.configuration.proxy` | `/etc/mco/proxy.env` and related env files, `/etc/pki/ca-trust/source/anchors/` (via `trustedCA`) | Run `update-ca-trust`, restart crio, kubelet |
| `spec.configuration.image` | `/etc/containers/registries.conf`, `/etc/containers/policy.json`, `/etc/pki/ca-trust/source/anchors/` (via `additionalTrustedCA`) | Reload/restart crio |
| Management-side: static pod manifests | `/etc/kubernetes/manifests/kube-apiserver-proxy.yaml` | None |
| Management-side: HAProxy config | `/etc/kubernetes/apiserver-proxy-config/haproxy.cfg` | None |
| Management-side: API server IP scripts | `/usr/local/bin/setup-apiserver-ip.sh`, `/usr/local/bin/teardown-apiserver-ip.sh` | None |

**Rollout-requiring inputs** (rollout hash, trigger Replace):

| Input | Reason |
| ----- | ------ |
| Release image | Full upgrade lifecycle required |
| NodePool `.config` (user MachineConfigs) | Arbitrary content — may include OS-level changes, systemd units, kernel args |
| FIPS mode | Requires reboot |
| Kernel arguments | Requires reboot |
| OS image | Requires reboot |
| Kernel type (realtime, 64k-pages) | Requires reboot |
| RPM extensions | Requires reboot |

**Note on user MachineConfigs**: NodePool `.config` entries (user-provided
MachineConfigs) always trigger a rollout because they can contain arbitrary
content including systemd units, kernel arguments, and files that require a
reboot. While some user MachineConfig changes (e.g., adding an SSH key) would
be safe for in-place refresh, the system cannot distinguish safe from unsafe
content without inspecting the diff, which conflicts with the input-based
classification model. A future enhancement could introduce opt-in annotations
on individual MachineConfigs to mark them as refresh-eligible.

### Risks and Mitigations

| Risk | Impact | Mitigation |
| ---- | ------ | ---------- |
| Path-to-action map missing a path that requires reboot | MCD applies a change that causes unexpected reboot | MCD independently validates hardcoded reboot triggers (OS, kargs, FIPS, kernelType, extensions) before honoring `no-reboot-paths`. The map is compiled into the controller and reviewed as code changes |
| MCD pod fails or hangs on a node | Node stuck in refresh state | Timeout and retry logic with maxUnavailable pacing prevents fleet-wide impact; degraded state surfaced in NodePool status |
| Post-apply action fails (e.g., crio restart fails) | Node in inconsistent state with new files but old service config | Controller detects failure via MCD pod exit status and node annotation; surfaces degraded condition; node can be replaced via standard rollout |
| Race between config refresh and concurrent Replace rollout | Conflicting node state | Controller pauses refresh for the entire NodePool during active rollouts; checks for existing MCD pods before creating new ones |
| Guest cluster MCD does not support `no-reboot-paths` key | MCD falls back to legacy behavior, rebooting for unrecognized paths | Controller checks guest cluster release version before refresh. Skips refresh and defers to Replace rollout if MCD is too old. Absent key means unchanged behavior |
| MCO code change requires cross-team coordination | Delivery timeline depends on MCO team accepting and shipping the MCD change | The MCD change is small and scoped to the HyperShift code path only. Early engagement with the MCO team is recommended. The HyperShift controller-side changes can be developed independently and gated on the MCD version check |

#### Security: `no-reboot-paths` Threat Model

The `no-reboot-paths` ConfigMap key is a privileged instruction to MCD: "for
these paths, use the specified post-apply action instead of rebooting." This
section addresses the threat of an attacker tampering with this key.

**Access control**: The `<pool>-upgrade` namespace and its ConfigMaps are
created and managed by the HCCO, which runs with elevated privileges in the
guest cluster. The namespace uses restricted RBAC — only the HCCO's service
account has write access. Cluster administrators with `cluster-admin` in the
guest cluster can modify ConfigMaps in any namespace, but this is expected and
consistent with the threat model for privileged guest cluster administrators.

**Defense-in-depth in MCD**: MCD independently validates that its hardcoded
reboot triggers are not present in the diff before honoring `no-reboot-paths`.
Specifically, MCD checks the `machineConfigDiff` struct for `osUpdate`,
`kargs`, `fips`, `kernelType`, and `extensions` changes. If any of these are
true, MCD ignores `no-reboot-paths` and falls back to its legacy reboot
behavior. This means that even if an attacker adds a kernel argument path to
`no-reboot-paths`, MCD still reboots because it detects the kargs change at
the structural level.

**Service restart/reload scope**: The `services` field in `no-reboot-paths`
entries is limited to a hardcoded set of valid service names in MCD (e.g.,
`crio`, `kubelet`, `update-ca-trust`). MCD rejects or ignores service names
not in this set. This prevents injection of arbitrary service operations.

**Attack surface**: This enhancement does not broaden the attack surface for
MCD pods on Replace-strategy nodes. The existing in-place upgrader already
creates MCD pods for InPlace-strategy nodes using the same ConfigMap pattern.
The config refresh controller uses the identical mechanism — the only new
element is the `no-reboot-paths` key, which is additive and gated by MCD's
defense-in-depth checks.

### Drawbacks

- **Increased complexity**: Adding a second config delivery path (refresh
  alongside Replace rollout) increases the surface area for bugs and makes the
  system harder to reason about. The mitigation is clear separation of
  concerns: refresh handles only non-disruptive file changes, while Replace
  handles everything else.

- **Partial config convergence**: After a refresh, nodes have the latest
  non-disruptive config but may still carry stale configuration for
  reboot-requiring changes. This is by design — those changes are deferred —
  but it means nodes are in a "partially updated" state that operators must
  understand.

- **Cross-team dependency on MCO**: The `no-reboot-paths` ConfigMap key
  requires a code change in the Machine Config Daemon (MCO repository). While
  the change is small and scoped to MCD's HyperShift code path, it introduces
  a cross-team dependency for delivery. The HyperShift controller-side work can
  proceed independently, but the feature is not fully functional until the MCD
  change ships in a release.

## Alternatives (Not Implemented)

### Direct file writes without MCD

An alternative approach would bypass MCD entirely and have the config refresh
controller write files directly to the node filesystem via privileged pods.

This was rejected because:
- It would duplicate MCD's file writing, state tracking, and completion
  reporting infrastructure.
- It would create a parallel config management path that could conflict with
  MCD's view of node state (e.g., MCD's `currentConfig` / `desiredConfig`
  annotations on the node).

### ConfigMap/Secret-based config delivery

Another alternative would use Kubernetes ConfigMaps or Secrets mounted into
node pods to deliver configuration changes.

This was rejected because:
- Many of the target files (static pod manifests, system certificates, registry
  config) must exist on the host filesystem, not in pod-mounted volumes.
- It would require a separate agent on each node to watch for ConfigMap changes
  and write them to the host filesystem — essentially reinventing MCD.

### Trigger Replace rollout for all changes

The simplest alternative is the status quo before OCPSTRAT-3298: every config
change triggers a Replace rollout.

This was rejected because it causes the exact problem OCPSTRAT-3298 solves —
unexpected full-fleet node replacements from management-side image bumps — and
would leave no path to deliver non-disruptive changes without node replacement.

### Node Disruption Policies (MachineConfiguration resource)

An alternative approach would create a `MachineConfiguration` resource in the
guest cluster containing Node Disruption Policies (NDPs) to declare which file
paths are safe to apply without rebooting. MCD already supports NDPs in
standalone clusters via `calculatePostConfigChangeNodeDisruptionAction()`.

This was rejected because:
- MCD in HyperShift simplified mode does not initialize the `mcopClient` that
  reads `MachineConfiguration` resources (`HypershiftConnect()` skips it).
- MCD's HyperShift code path (`syncNodeHypershift`) calls the legacy
  `calculatePostConfigChangeAction()`, not the NDP-aware function.
- Creating a `MachineConfiguration` cluster-scoped resource in the guest
  cluster would require additional RBAC and would be visible to the cluster
  administrator, adding confusion about who owns the resource.
- The ConfigMap key approach achieves the same goal with a smaller change
  surface (one key in an already-existing ConfigMap) and no new guest cluster
  resources.

## Open Questions

1. Should the config refresh controller be a new standalone controller in the
   HCCO, or should it extend the existing in-place upgrader? The existing
   in-place upgrader already demonstrates the MCD pod pattern but is currently
   scoped to InPlace-strategy NodePools only.

2. How should the controller handle partial refresh failures (e.g., MCD
   succeeds on 3 of 5 nodes)? Should it continue to the remaining nodes or
   pause and surface a degraded condition?

3. Should there be a mechanism for operators to explicitly trigger a config
   refresh, or should it always be automatic when a payload hash change is
   detected?

4. What is the minimum OCP release version that will include the MCD
   `no-reboot-paths` change? The config refresh controller needs this version
   threshold to gate the feature correctly for guest clusters running older
   releases.

## Test Plan

**Unit tests (HyperShift)**:
- Verify the hash split classifies rollout hash inputs correctly — changes to
  release image, NodePool `.config`, FIPS, kargs, kernelType, extensions
  produce a rollout hash change.
- Verify the hash split classifies payload-only inputs correctly — changes to
  `sshKey`, `pullSecret`, `additionalTrustBundle`, `imageContentSources`,
  `configuration.proxy`, `configuration.image`, and management-side content
  produce only a payload hash change.
- Verify the controller correctly identifies payload-only changes (payload hash
  changed, rollout hash unchanged).
- Verify the controller does not act when the rollout hash changes.
- Verify maxUnavailable pacing logic.
- Verify the controller populates the `no-reboot-paths` ConfigMap key with the
  correct JSON structure containing all refresh-eligible paths.
- Verify the controller pauses refresh when a Replace rollout is active.
- Verify the controller skips nodes with existing MCD pods.

**Unit tests (MCO)**:
- Verify MCD parses the `no-reboot-paths` JSON from the ConfigMap mount
  directory.
- Verify MCD uses the specified action (None, Restart, Reload) for paths
  listed in `no-reboot-paths` instead of defaulting to reboot.
- Verify MCD falls back to legacy behavior when the `no-reboot-paths` file
  does not exist (backward compatibility).
- Verify MCD falls back to legacy behavior when the `no-reboot-paths` file
  contains invalid JSON (defensive parsing).
- Verify MCD still reboots for hardcoded reboot triggers (OS, kargs, FIPS,
  kernelType, extensions) regardless of `no-reboot-paths` content.

**Integration tests**:
- Verify MCD pods are created with the correct ConfigMap containing `config`,
  `hash`, and `no-reboot-paths` keys.
- Verify post-apply actions (crio restart/reload, update-ca-trust) execute
  correctly after file writes.

**E2E tests**:
- Verify config refresh delivers an updated static pod manifest (e.g., HAProxy
  image bump) to existing Replace-strategy nodes without reboot.
- Verify SSH key rotation is applied in-place without node replacement.
- Verify trust bundle update is applied in-place with crio restart but no
  reboot.
- Verify registry mirror config change is applied in-place.
- Verify proxy configuration change is applied in-place with crio/kubelet
  restart but no reboot.
- Verify image registry policy change is applied in-place.
- Verify changes requiring reboot (e.g., kernel argument change) are excluded
  from refresh and deferred to the next Replace rollout.
- Verify mixed changes (e.g., release image + SSH key) trigger Replace rollout
  and do not trigger config refresh.
- Verify config refresh respects maxUnavailable and does not update all nodes
  simultaneously.
- Verify config refresh does not interfere with a concurrent Replace rollout.

## Graduation Criteria

TBD — graduation milestones will be defined once a target release is
identified.

### Dev Preview -> Tech Preview

- Config refresh mechanism functional end-to-end for Replace-strategy
  NodePools.
- Safety invariant validated against all eligible and ineligible change
  categories.
- Basic E2E test coverage.

### Tech Preview -> GA

- E2E tests cover all eligible change categories (SSH keys, trust bundles,
  registry mirrors, pull secrets, proxy configuration, image registry policies,
  static pod manifests).
- Load testing with large NodePools (100+ nodes) to validate maxUnavailable
  pacing.
- Upgrade/downgrade testing from a version without config refresh to a version
  with it.
- NodePool status reporting for refresh state.
- Documentation in openshift-docs.

## Upgrade / Downgrade Strategy

**Upgrade**: When upgrading to a version that includes the config refresh
controller, the HCCO is updated as part of the management-side upgrade. On
first reconcile, the config refresh controller begins monitoring for payload
hash changes. No existing node state is disrupted — the controller only acts on
future payload hash changes.

**Downgrade**: If the management cluster is downgraded to a version without the
config refresh controller, the refresh annotations on nodes are ignored by
older controllers. No cleanup is required. Non-disruptive changes revert to the
pre-OCPSTRAT-3298 behavior of being included in the rollout hash, which may
trigger Replace rollouts on the next config change.

## Version Skew Strategy

The config refresh mechanism involves components at three version boundaries:

1. **Management cluster (HyperShift operator / NodePool controller)** ↔
   **Guest cluster (HCCO / MCD)**: The HCCO version is determined by the
   management-side upgrade. The MCD image used for refresh pods is pulled from
   the guest cluster's release payload, so it matches the guest cluster's OCP
   version.

2. **NodePool controller** ↔ **Config refresh controller**: Both are part of
   the HyperShift codebase and are upgraded together. The config refresh
   controller depends on the rollout/payload hash split from OCPSTRAT-3298
   being present in the NodePool controller.

3. **Config refresh controller** ↔ **MCD version**: The controller creates
   ConfigMaps with the `no-reboot-paths` key, but the MCD image comes from the
   guest cluster's release payload. If the guest cluster is running a release
   that predates the MCD `no-reboot-paths` change, MCD will ignore the key and
   fall back to its legacy behavior — which means it would reboot for file
   paths not in the hardcoded safe list. The controller must check the guest
   cluster release version and only attempt config refresh if the MCD version
   supports `no-reboot-paths`. If the MCD version is too old, the controller
   skips the refresh and defers changes to the next Replace rollout.

During a rolling upgrade of the management cluster, there is a window where
some HyperShift operator replicas have the new code and some do not. The
annotation-based tracking (using a new annotation key) ensures that old
replicas simply ignore the new annotations, and new replicas seed the
annotations on first reconcile without triggering unintended actions.

## Operational Aspects of API Extensions

This enhancement does not introduce new API extensions (CRDs, webhooks, or
aggregated API servers).

The refresh state annotations on Node objects are internal implementation
details. If the config refresh controller is unavailable (e.g., HCCO is down),
nodes continue to operate with their current configuration. No refresh occurs
until the controller recovers. This is a safe degradation — nodes are not
disrupted, they simply do not receive the latest non-disruptive config updates.

## Support Procedures

**Detecting config refresh issues**:
- Check NodePool status conditions for refresh-related degraded states.
- Look for MCD pods in the guest cluster that are stuck or failed:
  `oc get pods -n openshift-machine-config-operator -l app=machine-config-daemon-refresh`
- Check HCCO logs for refresh controller activity:
  `oc logs -n <hcp-namespace> <hcco-pod> | grep "config-refresh"`
- Compare node annotations to detect nodes that have not been refreshed:
  `oc get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.hypershift\.openshift\.io/currentRefreshConfig}{"\n"}{end}'`

**Disabling config refresh**:
- If the config refresh controller is causing issues, it can be disabled by
  removing or gating the controller in the HCCO. Nodes will continue to
  operate with their current configuration. Non-disruptive changes will be
  deferred until the next Replace rollout.

**Recovery**:
- If a node is stuck in a refresh state, delete the MCD pod on that node. The
  controller will retry on the next reconcile.
- If a refresh caused unexpected behavior (e.g., a service failed to restart),
  the node can be drained and replaced via the standard Replace rollout path.

## Infrastructure Needed

No new infrastructure is needed. The implementation uses existing HyperShift
and MCO infrastructure.
