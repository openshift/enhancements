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
  - TBD
see-also: []
replaces: []
superseded-by: []
---

# NRI plugin mutation policy

## Summary

CRI-O merges NRI plugin adjustments and validates the combined result in a
single step. This enhancement proposes a small, standalone NRI policy plugin
not policy logic embedded in the CRI-O daemon that decides whether a
workload's Kubernetes namespace may receive that merged change. Policy is held
in configuration outside the core daemon, with only minimal CRI-O settings to
enable NRI and load the plugin. How that configuration is delivered and updated
on OpenShift (for example node provisioning / Machine Config versus cluster API
/ Operator / CRD) is left to the enhancement design and alternatives.

## Motivation

Cluster administrators need a clear contract for which namespaces may receive
NRI-driven container adjustments, and whether the cluster is observe-only or
enforcing. That decision must apply to the **merged** adjustment from all
plugins, not to a single plugin's isolated `CreateContainer` contribution
otherwise other plugins' changes can still be present in the merge and a
reliable "block the combined mutation" rule is not possible.

### User Stories

- As a cluster administrator, I want to deploy a standalone NRI policy plugin
  and manage its rules through the OpenShift-aligned path we agree in this
  enhancement (node-level config and/or API-driven rollout), and to run
  permissive mode while tuning and strict mode when enforcing, so that I can
  control whether merged NRI container adjustments apply per namespace without
  relying on per-plugin `CreateContainer` logic that cannot see the full merged
  result.

### Goals

- Standalone NRI plugin that enforces namespace policy against **merged**
  container adjustments at the validation stage.
- Strict and permissive modes; namespace-scoped rules in v1.

### Non-Goals

## Proposal

### Enforcement point

The plugin participates in the NRI stage where CRI-O validates **merged**
container adjustments after all contributing plugins have applied. In the NRI
API this corresponds to **`ValidateContainerAdjustment`** policy is evaluated
there so approve/reject applies to the combined adjustment, not to one plugin's
slice of it.

This is the critical distinction from embedding policy logic in each individual
NRI plugin's `CreateContainer` hook: at `CreateContainer` time, a plugin can
only see its own proposed changes. At `ValidateContainerAdjustment` time, the
policy plugin sees the **full merged result** from every contributing plugin,
making it the only point where a reliable "block the combined mutation" decision
is possible.

The policy plugin does not participate in `CreateContainer` at all it only
registers `ValidateContainerAdjustment`. CRI-O's NRI stub discovers this
automatically from the methods the plugin exposes.

### Policy schema (v1)

The on-disk file and any future CR **spec** share the same shape so plugin
behavior is identical regardless of delivery path (file-based or
Operator-managed). `policies[]` is evaluated in order — first matching entry
wins.

#### Field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.mode` | string | yes | `strict` — reject disallowed mutations. `permissive` warn only, never block. Defaults to `permissive` if omitted. |
| `spec.policies[].namespaceSelector.matchNames` | []string | yes | Exact namespace names this policy entry covers. |
| `spec.policies[].allowedMutations` | []string | no | Mutation categories permitted for matching namespaces. If omitted or empty, all categories are permitted for that namespace. |

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
    
```

#### Evaluation logic

| Condition | permissive mode | strict mode |
|---|---|---|
| Namespace matches no policy | warn, allow | reject |
| Namespace matches policy, `allowedMutations` empty | allow all | allow all |
| Namespace matches policy, mutation type in `allowedMutations` | allow | allow |
| Namespace matches policy, mutation type NOT in `allowedMutations` | warn, allow | reject |

### Proposal A: file-based delivery

- Ship the standalone NRI policy plugin and its policy file as node
  configuration. CRI-O enables NRI and loads the plugin through the normal NRI
  integration path.

- At validation time, the plugin evaluates the workload namespace against the
  configured rules (see **Policy schema (v1)**). The plugin reads config once at
  startup from a fixed canonical path. An `ALLOW_MUTATIONS_CONFIG` environment
  variable overrides the path for testing only.

- Policy is read from the single canonical path:
  `/etc/crio/nri_plugins/AllowMutations/config.yaml`.

- Changes ship via the Machine Config Operator: a `MachineConfig` for the
  target pool (typically `worker`) uses Ignition `storage.files` to install:
  1. the plugin binary at `/usr/local/bin/nri-allow-mutations` (stable,
     documented path on RHCOS; avoid ad-hoc writes under `/usr` from operators);
  2. the policy file at `/etc/crio/nri_plugins/AllowMutations/config.yaml`;
  3. a CRI-O drop-in under `/etc/crio/crio.conf.d/` that enables NRI and
     registers the plugin.

- Policy changes require a new MachineConfig rollout — MCO drains and reboots
  nodes one at a time. This is the accepted cost of Day Zero delivery. The
  `permissive` mode is intended for use while tuning policy to avoid workload
  disruption; `strict` mode is enabled once rules are confirmed correct.

#### Pros

- Policy is enforced from the very first container on a fresh node — works
  before the Kubernetes API server is up.
- No additional in-cluster components smaller attack surface and no new
  failure domain.
- Reuses the existing MCO/Ignition delivery path already proven for all other
  node configuration in OpenShift.

#### Cons

- Every policy change requires a MachineConfig rollout nodes are drained and
  rebooted, taking 20–45 minutes per cluster and disrupting running workloads.
- No live status reporting there is no CR condition to observe whether the
  policy is in sync on each node.
- Hot-reload is not possible config changes take effect only after a node
  reboot.

### Proposal B: API-managed policy (Operator + reconciliation)

- Expose mutation policy as a first-class Kubernetes/OpenShift API so
  administrators can create, update, and observe policy without authoring raw
  Ignition/MachineConfig for every change. Enforcement remains the same
  standalone NRI plugin; the Operator renders the **Policy schema (v1)**
  document to disk for the plugin to read. The plugin binary is unchanged
  between Proposal A and Proposal B.

- Introduce a CRD for a cluster-scoped policy object (exact name, scope,
  group/version TBD; API review). The **spec** matches the on-disk shape above
  exactly — same fields, same semantics.

- An Operator watches policy CRs, validates spec, and reconciles desired state
  to the fleet: reject invalid config with clear conditions; update CR status
  (Available, Progressing, Degraded, per-node summary); emit Kubernetes events
  for notable failures.

- The Operator runs a DaemonSet (one pod per node) that writes the rendered
  plugin config to `/etc/crio/nri_plugins/AllowMutations/config.yaml` on the
  host via a `hostPath` volume mount.

#### Pros

- Policy changes take effect in seconds with `kubectl apply` no node drain,
  no reboot, no pod disruption.
- CR status conditions give real-time visibility into whether policy is in sync
  across all nodes.
- Operator validates spec before applying it, surfacing errors as CR conditions
  rather than silently mis-configuring nodes.

#### Cons

- Does not work before the API server is up a fresh node may have an absent or
  stale config file if the Operator or DaemonSet is not yet running.
- Operator is a new failure domain if it crashes or is misconfigured, policy
  updates silently stop propagating to nodes.
- DaemonSet pods require privileged access or a custom SCC to write to the host
  filesystem, increasing the security review surface.

#### Relationship between proposals

Proposal A (Day Zero / MCO) is the target for the initial implementation.
Proposal B (Day One / CRD + Operator) is documented here to open the community
discussion on the long-term delivery path and will be tracked in a separate
enhancement once Proposal A is proven end-to-end. The plugin binary itself does
not change between the two proposals — only who writes the config file to disk
differs.

## Test Plan

### Proposal A: Unit tests

Unit tests live in `plugin/plugin_test.go` in the
[`nri-allow-mutations`](https://github.com/amritansh1502/nri-allow-mutations)
repository. Run with `go test ./plugin/...`.

| Test | What it verifies |
|---|---|
| `TestLoad_ModeIsCaseSensitive` | `mode: Strict` is rejected; mode values are case-sensitive |
| `TestLoad_EmptyModeDefaultsToPermissive` | Omitting `mode` is valid and defaults to permissive |
| `TestFirstMatchWins` | First matching policy entry wins; no merging across entries |
| `TestNilAdjustmentIsAlwaysAllowed` | Nil adjustment (no NRI mutation) is always allowed |
| `TestEmptyAllowedMutationsPermitsAll` | Empty `allowedMutations` means all mutation types permitted |
| `TestStrictRejectsOnFirstDisallowedMutationType` | Strict mode rejects on the first disallowed mutation type |
| `TestPermissiveNeverReturnsError` | Permissive mode never returns an error under any condition |
| `TestStrictRejectsNamespaceWithNoPolicy` | Strict mode rejects namespaces not covered by any policy |
| `TestResourcesAndDevicesDetectedViaLinuxSubStruct` | `resources` and `devices` are correctly detected via `adj.GetLinux()` |
| `TestConcurrentLoadAndValidate` | Concurrent `Load()` and `ValidateContainerAdjustment()` calls do not race |

### Proposal A: OpenShift cluster test

Tested on a 6-node OpenShift cluster (3 control-plane, 3 workers). Plugin and
config delivered to worker nodes; systemd unit and CRI-O drop-in delivered via
MachineConfig (`99-nri-allow-mutations`, Ignition 3.5.0).

#### Permissive mode

Deployed a pod in `default` namespace. Plugin detected `resources` and `hooks`
mutations applied by CRI-O's internal NRI path, logged warnings, and allowed
the pod to start — confirming `ValidateContainerAdjustment` is called on every
container start and permissive mode never blocks.

#### Strict mode

Deployed a pod in `test-blocked` namespace with only `env` in `allowedMutations`.
Plugin rejected the container at the NRI validation stage when CRI-O added
`resources` and `hooks` pod did not start, confirming strict enforcement works.

## Graduation Criteria

### Dev Preview -> Tech Preview

TBD

### Tech Preview -> GA

TBD

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

- New repository: `github.com/amritansh1502/nri-allow-mutations` hosts the
  standalone NRI policy plugin binary and its Go source.
