---
title: nri-plugin-mutation-policy
authors:
  - "@amritansh1502"
  - "@ngopalak-redhat"
reviewers:
  - "@haircommander"
  - "@sgrunert"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-04-15
last-updated: 2026-04-15
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPNODE-3380
see-also: []
replaces: []
superseded-by: []
---

# NRI plugin mutation policy

## Summary

NRI (Node Resource Interface) is a framework for plugging extensions into OCI-compatible runtimes like CRI-O and containerd. NRI plugins are long-running processes that hook into container lifecycle events (`CreateContainer`, `UpdateContainer`, etc.) over a unix-domain socket and can adjust a container's resources, mounts, environment, and annotations before the runtime commits the final OCI spec. When multiple plugins run on a node, the runtime merges all adjustments into a single combined result and applies it atomically.

This enhancement proposes a small, standalone NRI policy plugin, not policy logic embedded in the CRI-O daemon, that decides whether a workload's Kubernetes namespace may receive that merged change. Policy is held in configuration outside the core daemon, with only minimal CRI-O settings to enable NRI and load the plugin.

## Motivation

Cluster administrators need a clear contract for which namespaces may receive NRI-driven container adjustments, and whether the cluster is observe-only or enforcing. That decision must apply to the **merged** adjustment from all plugins, not to a single plugin's isolated `CreateContainer` contribution—otherwise other plugins' changes can still be present in the merge and a reliable "block the combined mutation" rule is not possible.

### User Stories

- A cluster administrator can deploy a standalone NRI policy plugin and run permissive mode while tuning and strict mode when enforcing, to control whether merged NRI container adjustments apply per namespace without relying on per-plugin `CreateContainer` logic that cannot see the full merged result.

### Goals

- Standalone NRI plugin that enforces namespace policy against **merged** container adjustments at the validation stage.
- Strict and permissive modes; namespace-scoped rules in v1.

### Non-Goals

## Proposal

### Workflow Description

A cluster administrator creates or updates the policy configuration (either via MachineConfig for Dev Preview, or via a policy CR for TP/GA). The standalone NRI policy plugin running on each worker node reads the config file at startup and registers the `ValidateContainerAdjustment` NRI hook. When a container is created, CRI-O calls the hook with the fully merged adjustment from all NRI plugins; the policy plugin evaluates the workload namespace against the configured rules and either approves or rejects the adjustment before the container is started.

### API Extensions

None for Dev Preview. TP/GA introduces a cluster-scoped CRD (exact name, group, and version TBD pending API review) whose spec mirrors the on-disk policy schema described below.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The plugin runs on data-plane worker nodes only and does not interact with the hosted control plane. Delivery via config maps (Dev Preview) or DaemonSet (TP/GA) applies to the node pool as usual.

#### Standalone Clusters

Standard topology; no special considerations beyond the general MCO rollout behaviour described in Dev Preview.

#### Single-node Deployments or MicroShift

On SNO the plugin runs on the single node. A MachineConfig rollout reboots that node, causing a temporary cluster outage; operators should schedule policy changes during a maintenance window. MicroShift is out of scope for this proposal (a future iteration could ship the plugin as a systemd service).

#### OpenShift Kubernetes Engine

No OKE-specific constraints; the plugin is a node-level NRI binary and does not depend on OpenShift-specific APIs beyond MCO delivery.

### Implementation Details/Notes/Constraints

- The plugin binary is statically compiled and has no runtime dependencies beyond the NRI socket provided by CRI-O.
- Config is read once at startup; a plugin restart (node reboot for Dev Preview, DaemonSet pod restart for TP/GA) is required to pick up changes.
- The `OPENSHIFT_UNSUPPORTED_ALLOW_MUTATIONS_CONFIG` environment variable overrides the canonical config path for unit-test and development use only.
- The plugin registers only `ValidateContainerAdjustment`; CRI-O's NRI stub discovers this automatically from the exported methods.

### Risks and Mitigations

- The NRI policy plugin adds a new component to the container startup path. A bug or misconfiguration in the plugin could block container creation on affected nodes.
  - Mitigation: the plugin supports a permissive mode that logs violations without rejecting containers, allowing operators to validate rules before switching to strict enforcement.

### Drawbacks



### Enforcement point

The plugin participates in the NRI stage where CRI-O validates **merged** container adjustments after all contributing plugins have applied. In the NRI API this corresponds to **`ValidateContainerAdjustment`**—policy is evaluated there so approve/reject applies to the combined adjustment, not to one plugin's slice of it.

This is the critical distinction from embedding policy logic in each individual NRI plugin's `CreateContainer` hook: at `CreateContainer` time, a plugin can only see its own proposed changes. At `ValidateContainerAdjustment` time, the policy plugin sees the **full merged result** from every contributing plugin, making it the only point where a reliable "block the combined mutation" decision is possible.

The policy plugin does not participate in `CreateContainer` at all—it only registers `ValidateContainerAdjustment`. CRI-O's NRI stub discovers this automatically from the methods the plugin exposes.

### Policy schema (v1)

The on-disk file and any future CR **spec** share the same shape so plugin behavior is identical regardless of delivery path (file-based or Operator-managed). `policies[]` is evaluated in order—first matching entry wins.

#### Field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.mode` | string | yes | `strict` — reject disallowed mutations. `permissive` warn only, never block. Defaults to `permissive` if omitted. |
| `spec.policies[].namespaceSelector.matchNames` | []string | yes | Exact namespace names this policy entry covers. |
| `spec.policies[].allowedMutations` | []string | no | Mutation categories permitted for matching namespaces. If empty or omitted, no mutations are allowed. Use `["all"]` to permit all categories. |

#### Allowed mutation category values

| Value | What it covers |
|---|---|
| `env` | Environment variable changes |
| `mounts` | Volume / bind-mount changes |
| `annotations` | Annotation changes |
| `resources` | CPU / memory resource changes (Linux cgroup) |
| `hooks` | OCI lifecycle hook changes |
| `devices` | Linux device node changes |

#### Full example

```yaml
spec:
  mode: strict
  policies:
    - namespaceSelector:
        matchNames: ["kube-system", "openshift-monitoring"]
      allowedMutations: ["env", "mounts", "resources"]

    - namespaceSelector:
        matchNames: ["my-app"]
      allowedMutations: ["env"]

    - namespaceSelector:
        matchNames: ["trusted-ns"]
      allowedMutations: ["all"]
```

#### Evaluation logic

| Condition | permissive mode | strict mode |
|---|---|---|
| Namespace matches no policy | warn, allow | reject |
| Namespace matches policy, `allowedMutations` empty | warn, allow | reject (none allowed) |
| Namespace matches policy, `allowedMutations` contains `"all"` | allow all | allow all |
| Namespace matches policy, mutation type in `allowedMutations` | allow | allow |
| Namespace matches policy, mutation type NOT in `allowedMutations` | warn, allow | reject |

### Dev Preview: file-based delivery

- Ship the standalone NRI policy plugin and its policy file as node configuration. CRI-O enables NRI and loads the plugin through the normal NRI integration path.

- At validation time, the plugin evaluates the workload namespace against the configured rules (see **Policy schema (v1)**). The plugin reads config once at startup from a fixed canonical path. An `OPENSHIFT_UNSUPPORTED_ALLOW_MUTATIONS_CONFIG` environment variable overrides the path for testing only.

- Policy is read from the single canonical path: `/etc/crio/nri_plugins/AllowMutations/config.yaml`.

- Changes ship via the Machine Config Operator: a `MachineConfig` for the target pool (typically `worker`) uses Ignition `storage.files` to install:
  1. the plugin binary at `/usr/local/bin/nri-allow-mutations` (stable, documented path on RHCOS; avoid ad-hoc writes under `/usr` from operators);
  2. the policy file at `/etc/crio/nri_plugins/AllowMutations/config.yaml`;
  3. a CRI-O drop-in under `/etc/crio/crio.conf.d/` that enables NRI and registers the plugin.

- Policy changes require a new MachineConfig rollout—MCO drains and reboots nodes one at a time. This is the accepted cost of Day Zero delivery. The `permissive` mode is intended for use while tuning policy to avoid workload disruption; `strict` mode is enabled once rules are confirmed correct.

#### Pros

- Policy is enforced from the very first container on a fresh node—works before the Kubernetes API server is up.
- No additional in-cluster components; smaller attack surface and no new failure domain.
- Reuses the existing MCO/Ignition delivery path already proven for all other node configuration in OpenShift.

#### Cons

- Every policy change requires a MachineConfig rollout—nodes are drained and rebooted, taking 20–45 minutes per cluster and disrupting running workloads.
- No live status reporting; there is no CR condition to observe whether the policy is in sync on each node.
- Hot-reload is not possible; config changes take effect only after a node reboot.

### TP/GA: GA delivery via ContainerRuntimeConfig (ctrcfg) API

The GA delivery path is out of scope for this enhancement. Based on reviewer feedback, the agreed direction is to extend the existing `ContainerRuntimeConfig` (`ctrcfg`) API with an NRI mutation policy field rather than introducing a new standalone CRD. The plugin binary itself does not change—only the config delivery mechanism differs. This will be designed and tracked in a separate follow-on enhancement once Dev Preview is proven end-to-end.

## Alternatives (Not Implemented)

### CRI-O built-in namespace policy

Embedding policy logic directly in the CRI-O daemon was considered but rejected: it couples policy management to CRI-O release cycles, increases daemon complexity, and prevents independent updates to the policy rules. The standalone plugin model keeps policy outside the daemon.

## Test Plan

### Dev Preview: Unit tests

Unit tests live in `plugin/plugin_test.go` in the [`nri-allow-mutations`](https://github.com/amritansh1502/nri-allow-mutations) repository. Run with `go test ./plugin/...`.

| Test | What it verifies |
|---|---|
| `TestLoad_ModeIsCaseSensitive` | `mode: Strict` is rejected; mode values are case-sensitive |
| `TestLoad_EmptyModeDefaultsToPermissive` | Omitting `mode` is valid and defaults to permissive |
| `TestFirstMatchWins` | First matching policy entry wins; no merging across entries |
| `TestNilAdjustmentIsAlwaysAllowed` | Nil adjustment (no NRI mutation) is always allowed |
| `TestEmptyAllowedMutationsRejectsAll` | Empty `allowedMutations` means no mutation types permitted |
| `TestStrictRejectsOnFirstDisallowedMutationType` | Strict mode rejects on the first disallowed mutation type |
| `TestPermissiveNeverReturnsError` | Permissive mode never returns an error under any condition |
| `TestStrictRejectsNamespaceWithNoPolicy` | Strict mode rejects namespaces not covered by any policy |
| `TestResourcesAndDevicesDetectedViaLinuxSubStruct` | `resources` and `devices` are correctly detected via `adj.GetLinux()` |
| `TestConcurrentLoadAndValidate` | Concurrent `Load()` and `ValidateContainerAdjustment()` calls do not race |

### Dev Preview: OpenShift cluster test

Tested on a 6-node OpenShift cluster (3 control-plane, 3 workers). Plugin and config delivered to worker nodes; systemd unit and CRI-O drop-in delivered via MachineConfig (`99-nri-allow-mutations`, Ignition 3.5.0).

#### Permissive mode

Deployed a pod in `default` namespace. Plugin detected `resources` and `hooks` mutations applied by CRI-O's internal NRI path, logged warnings, and allowed the pod to start—confirming `ValidateContainerAdjustment` is called on every container start and permissive mode never blocks.

#### Strict mode

Deployed a pod in `test-blocked` namespace with only `env` in `allowedMutations`. Plugin rejected the container at the NRI validation stage when CRI-O added `resources` and `hooks`—pod did not start, confirming strict enforcement works.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Policy enforcement delivered via MachineConfig (Dev Preview) on worker nodes.
- Both `strict` and `permissive` modes functional and tested on an OpenShift cluster.
- Unit test suite passing with no data races.
- Enhancement doc reviewed and merged.
- CRD introduced and gated by the `TechPreviewNoUpgrade` feature set.

### Tech Preview -> GA

GA criteria will be defined in the follow-on enhancement covering `ContainerRuntimeConfig` (`ctrcfg`) API integration.

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

TBD

## Upgrade / Downgrade Strategy

Upgrade expectations: TBD

Downgrade expectations: TBD

## Version Skew Strategy

TBD

## Operational Aspects of API Extensions

TBD

## Support Procedures

TBD

## Infrastructure Needed [optional]

- New repository: `github.com/amritansh1502/nri-allow-mutations` hosts the standalone NRI policy plugin binary and its Go source.
