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

Cluster administrators need a way to control which namespaces may receive NRI-driven container adjustments and which mutation categories are permitted. That decision must apply to the **merged** adjustment from all plugins, not to a single plugin's `CreateContainer` contribution.

### User Stories

- A cluster administrator can deploy a standalone NRI policy plugin that strips disallowed mutation categories from merged NRI container adjustments on a per namespace basis, with unmatched namespaces passing through untouched, without relying on per-plugin `CreateContainer` logic that cannot see the full merged result.

### Goals

- Standalone NRI plugin that enforces namespace policy against **merged** container adjustments at the validation stage.
- Namespace scoped allow listing of mutation categories with automatic stripping of disallowed mutations in v1.

### Non-Goals

## Proposal

### Workflow Description

A cluster administrator creates or updates the policy configuration (either via MachineConfig for Dev Preview, or via a policy CR for TP/GA). The standalone NRI policy plugin running on each worker node reads the config file at startup and registers the `ValidateContainerAdjustment` NRI hook. When a container is created, CRI-O calls the hook with the fully merged adjustment from all NRI plugins; the policy plugin evaluates the workload namespace against the configured rules and strips any disallowed mutation categories from the adjustment before the container is started.

### API Extensions

None for Dev Preview. Tech Preview introduces a cluster-scoped CRD: `NriPlugin` (`nri.openshift.io/v1alpha1`), managed by the NRI Plugins Operator. The CRD is gated by the `TechPreviewNoUpgrade` feature set. See the Tech Preview delivery section for full schema.

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

- The NRI policy plugin adds a new component to the container startup path. A misconfigured policy could strip mutations that a workload depends on, causing it to run with incorrect resources, missing mounts, or missing environment variables.
- Mitigation: the plugin never rejects containers outright — it strips disallowed mutations and logs what was stripped. Administrators can review the logs to identify misconfigured policies before tightening the allowed list.

### Drawbacks



### Enforcement point

The plugin hooks into `ValidateContainerAdjustment`, which runs after CRI-O merges adjustments from all NRI plugins. At this stage the full combined adjustment is visible, unlike `CreateContainer` where each plugin only sees its own changes. The plugin does not register `CreateContainer` at all—CRI-O's NRI stub discovers the registered methods automatically.

### Policy schema (v1)

The on-disk file and any future CR **spec** share the same shape so plugin behavior is identical regardless of delivery path (file-based or Operator-managed). `policies[]` is evaluated in order—first matching entry wins.

#### Field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.policies[].namespaceSelector.matchNames` | []string | yes | Exact namespace names this policy entry covers. |
| `spec.policies[].allowedMutations` | []string | no | Mutation categories permitted for matching namespaces. If empty or omitted, all mutation types are allowed. Use `["all"]` to explicitly permit all categories. |

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

| Condition | Behavior |
|---|---|
| Namespace matches no policy | No hook, container runs with all mutations untouched |
| Namespace matches policy, `allowedMutations` empty | All mutations allowed |
| Namespace matches policy, `allowedMutations` contains `"all"` | All mutations permitted |
| Namespace matches policy, mutation type in `allowedMutations` | That mutation is kept |
| Namespace matches policy, mutation type NOT in `allowedMutations` | That mutation is stripped |

### Dev Preview: file-based delivery

- Ship the standalone NRI policy plugin and its policy file as node configuration. CRI-O enables NRI and loads the plugin through the normal NRI integration path.

- At validation time, the plugin evaluates the workload namespace against the configured rules (see **Policy schema (v1)**). The plugin reads config once at startup from a fixed canonical path. An `OPENSHIFT_UNSUPPORTED_ALLOW_MUTATIONS_CONFIG` environment variable overrides the path for testing only.

- Policy is read from the single canonical path: `/etc/crio/nri_plugins/AllowMutations/config.yaml`.

- Changes ship via the Machine Config Operator: a `MachineConfig` for the target pool (typically `worker`) uses Ignition `storage.files` to install:
  1. the plugin binary at `/usr/local/bin/nri-allow-mutations` (stable, documented path on RHCOS; avoid ad-hoc writes under `/usr` from operators);
  2. the policy file at `/etc/crio/nri_plugins/AllowMutations/config.yaml`;
  3. a CRI-O drop-in under `/etc/crio/crio.conf.d/` that enables NRI and registers the plugin.

- Policy changes require a new MachineConfig rollout—MCO drains and reboots nodes one at a time. This is the accepted cost of Day Zero delivery. The plugin strips disallowed mutations and logs what was stripped, so operators can tune the `allowedMutations` list and observe the effect before tightening policy.

#### Pros

- Policy is enforced from the very first container on a fresh node—works before the Kubernetes API server is up.
- No additional in-cluster components; smaller attack surface and no new failure domain.
- Reuses the existing MCO/Ignition delivery path already proven for all other node configuration in OpenShift.

#### Cons

- Every policy change requires a MachineConfig rollout—nodes are drained and rebooted, taking 20–45 minutes per cluster and disrupting running workloads.
- No live status reporting; there is no CR condition to observe whether the policy is in sync on each node.
- Hot-reload is not possible; config changes take effect only after a node reboot.

### Tech Preview: Operator-managed delivery

For Tech Preview the file-based MachineConfig delivery path is replaced by the **NRI Plugins Operator** ([`nri-plugins-operator`](https://github.com/amritansh1502/nri-plugins-operator)). The operator is the single supported entry point for NRI plugin deployment on OpenShift. An administrator creates an `NriPlugin` CR to deploy a plugin and optionally creates an `NriMutationPolicy` CR to define namespace-scoped mutation policy.

Out of the box a cluster has no NRI plugins installed. When the first `NriPlugin` CR is created, the operator enables NRI in CRI-O, waits for the MachineConfig rollout, deploys the requested plugin as a DaemonSet, and automatically deploys the allow-mutations validation plugin alongside it. Mutation policy is defined via `NriMutationPolicy` CRs; namespaces with no matching policy pass through untouched.

The operator does not assume exclusive ownership of NRI plugins on the node. Plugins deployed outside the operator (e.g. manually installed DaemonSets) are left untouched — the operator only manages plugins created through `NriPlugin` CRs.

#### NriPlugin CRD

The operator introduces a cluster-scoped CRD (`nri.openshift.io/v1alpha1`, kind `NriPlugin`). The CRD is gated by the `TechPreviewNoUpgrade` feature set.

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.pluginName` | string | yes | Name of the NRI plugin to deploy. |
| `spec.image` | string | yes | Container image for the plugin. The operator deploys a DaemonSet using this image on targeted worker nodes. |
| `spec.nodeSelector` | map[string]string | no | Targets which nodes get this plugin. Tech Preview supports worker nodes only. |
| `spec.pluginConfigCR` | object | no | For plugins that read config from a Kubernetes CR (topology-aware, balloons, etc.). The operator creates the referenced CR with the provided spec passed through as-is. |

Status fields:

| Field | Type | Description |
|---|---|---|
| `status.phase` | string | Current lifecycle phase: `Pending`, `EnablingNRI`, `WaitingForMCO`, `DeployingPlugin`, `Running`, or `Degraded`. |
| `status.readyNodes` | int32 | Number of nodes where the plugin pod is running. |
| `status.desiredNodes` | int32 | Total number of targeted nodes. |
| `status.conditions` | []Condition | Standard Kubernetes conditions. |

#### NriMutationPolicy CRD

The operator introduces a second cluster-scoped CRD (`nri.openshift.io/v1alpha1`, kind `NriMutationPolicy`) for namespace-scoped mutation policy. Mutation policy is managed separately from plugin deployment, so administrators can change policy without touching plugin CRs.

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.policies[].namespaceSelector.matchNames` | []string | yes | Exact namespace names this policy entry covers. |
| `spec.policies[].allowedMutations` | []string | no | Mutation categories permitted for matching namespaces. Same values as Policy schema (v1). If empty or omitted, all mutation types are allowed. Use `["all"]` to explicitly permit all categories. |

The operator watches `NriMutationPolicy` CRs and marshals the combined policy into a ConfigMap mounted into the auto-deployed allow-mutations plugin. Policy changes trigger a rolling restart of the allow-mutations DaemonSet — no node reboot required.

Status reports `phase` (`Active` or `Degraded`), `policyCount` (number of policy entries), and standard Kubernetes `conditions`.

##### NriMutationPolicy example

```yaml
apiVersion: nri.openshift.io/v1alpha1
kind: NriMutationPolicy
metadata:
  name: production-policy
spec:
  policies:
    - namespaceSelector:
        matchNames: ["production", "prod-workloads"]
      allowedMutations: ["env", "annotations"]
    - namespaceSelector:
        matchNames: ["trusted-ns"]
      allowedMutations: ["all"]
```

#### API type definitions

```go
// NriPluginSpec defines the desired state of an NRI plugin deployment.
type NriPluginSpec struct {
	PluginName     string            `json:"pluginName"`
	Image          string            `json:"image"`
	NodeSelector   map[string]string `json:"nodeSelector,omitempty"`
	PluginConfigCR *PluginConfigCR   `json:"pluginConfigCR,omitempty"`
}

// PluginConfigCR describes a config custom resource the operator should
// create for the plugin.
type PluginConfigCR struct {
	APIVersion string               `json:"apiVersion"`
	Kind       string               `json:"kind"`
	Name       string               `json:"name"`
	Namespace  string               `json:"namespace"`
	Spec       runtime.RawExtension `json:"spec"`
}

// NriPluginStatus defines the observed state of an NRI plugin deployment.
type NriPluginStatus struct {
	Phase        string             `json:"phase,omitempty"`
	ReadyNodes   int32              `json:"readyNodes,omitempty"`
	DesiredNodes int32              `json:"desiredNodes,omitempty"`
	Conditions   []metav1.Condition `json:"conditions,omitempty"`
}

// NriMutationPolicySpec defines the desired mutation policy.
type NriMutationPolicySpec struct {
	Policies []PolicyEntry `json:"policies"`
}

// PolicyEntry is a single namespace-scoped mutation policy.
type PolicyEntry struct {
	NamespaceSelector NamespaceSelector `json:"namespaceSelector"`
	AllowedMutations  []string          `json:"allowedMutations,omitempty"`
}

// NamespaceSelector selects namespaces by exact name.
type NamespaceSelector struct {
	MatchNames []string `json:"matchNames"`
}

// NriMutationPolicy is the Schema for the nrimutationpolicies API.
type NriMutationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NriMutationPolicySpec `json:"spec,omitempty"`
}
```

#### Reconciliation overview

When an `NriPlugin` CR is created the operator reconciles in this order:

1. Creates a shared MachineConfig (`99-nri-enable`) that drops a CRI-O config file on every worker node to enable NRI and set the NRI socket path.
2. Waits for the MachineConfig Operator to finish rebooting worker nodes (polls MachineConfigPool `worker` status every 30 seconds).
3. Creates the necessary RBAC and SecurityContextConstraints for the plugin pod.
4. For plugins with `pluginConfigCR`: creates the referenced config CR with the provided spec.
5. Deploys the requested plugin as a DaemonSet (`nri-<pluginName>`) on all targeted worker nodes. Plugin pods run privileged with host networking and mount the NRI socket.
6. Deploys the allow-mutations validation plugin as a DaemonSet (`nri-allow-mutations`) if not already running, configured by `NriMutationPolicy` CRs.
7. Updates CR status (phase, readyNodes, desiredNodes) from the DaemonSet status.

When an `NriMutationPolicy` CR is created or updated, the operator marshals the policy into a ConfigMap and triggers a rolling restart of the allow-mutations DaemonSet.

When the last `NriPlugin` CR is deleted the operator removes both the allow-mutations DaemonSet and the shared MachineConfig, disabling NRI cluster-wide. Plugin DaemonSets are garbage-collected through owner references.

#### balloons CR example

```yaml
apiVersion: nri.openshift.io/v1alpha1
kind: NriPlugin
metadata:
  name: balloons
spec:
  pluginName: balloons
  image: "ghcr.io/containers/nri-plugins/nri-resource-policy-balloons:v0.7.0"
  nodeSelector:
    node-role.kubernetes.io/worker: ""
  pluginConfigCR:
    apiVersion: config.nri/v1alpha1
    kind: BalloonPolicy
    name: default
    namespace: kube-system
    spec:
      pinCPU: true
      pinMemory: true
      reservedResources:
        cpu: "750m"
```

The allow-mutations plugin behavior is identical to Dev Preview; only the delivery changes from static file to `NriMutationPolicy` CR and ConfigMap. Policy changes no longer require a node reboot.

#### Pros

- No node reboot for policy changes.
- Live status reporting via CR status and conditions (`oc get nriplugin`).
- Single entry point for NRI plugin lifecycle.
- Reference-counted MachineConfig cleanup.

#### Cons

- Operator adds a new in-cluster component with its own failure domain.
- First NriPlugin CR still triggers a MachineConfig rollout (one-time node reboot to enable NRI in CRI-O).
- Plugin pods require privileged security context and host networking.

## Alternatives (Not Implemented)

### CRI-O built-in namespace policy

Embedding policy logic directly in the CRI-O daemon was considered but rejected: it couples policy management to CRI-O release cycles, increases daemon complexity, and prevents independent updates to the policy rules. The standalone plugin model keeps policy outside the daemon.

## Test Plan

### Dev Preview: Unit tests

Unit tests live in `plugin/plugin_test.go` in the [`nri-allow-mutations`](https://github.com/amritansh1502/nri-allow-mutations) repository. Run with `go test ./plugin/...`.

| Test | What it verifies |
|---|---|
| `TestFirstMatchWins` | First matching policy entry wins; no merging across entries |
| `TestNilAdjustmentIsAlwaysAllowed` | Nil adjustment (no NRI mutation) is always allowed |
| `TestNoOpForUnmatchedNamespace` | Unmatched namespace passes through untouched, no mutations stripped |
| `TestEmptyAllowedMutationsStripsAll` | Empty `allowedMutations` means all mutation types are stripped |
| `TestAllKeywordPermitsEverything` | `allowedMutations: ["all"]` permits every mutation category |
| `TestStripsDisallowedMutations` | Mutations not in `allowedMutations` are stripped, allowed ones are kept |
| `TestStripsResourcesAndDevices` | `resources` and `devices` are correctly detected and stripped via `adj.GetLinux()` |
| `TestKeepsAllowedMutationsIntact` | All listed mutation categories are preserved after validation |
| `TestNeverReturnsError` | Plugin never returns an error under any condition |
| `TestConcurrentLoadAndValidate` | Concurrent `Load()` and `ValidateContainerAdjustment()` calls do not race |

### Dev Preview: OpenShift cluster test

Tested on a 6-node OpenShift cluster (3 control-plane, 3 workers). Plugin and config delivered to worker nodes; systemd unit and CRI-O drop-in delivered via MachineConfig (`99-nri-allow-mutations`, Ignition 3.5.0).

#### Matched namespace with partial allowedMutations

Deployed a pod in `default` namespace with `allowedMutations: ["env"]`. Plugin detected `resources` and `hooks` mutations applied by CRI-O's internal NRI path, stripped them, logged what was stripped, and allowed the pod to start with only `env` mutations applied—confirming `ValidateContainerAdjustment` is called on every container start and disallowed mutations are stripped.

#### Unmatched namespace pass through

Deployed a pod in a namespace not covered by any policy entry. Plugin did not hook and the container ran with all mutations untouched, confirming the pass through behavior for namespaces not listed in any policy.

### Tech Preview: Operator tests

Operator tests live in the [`nri-plugins-operator`](https://github.com/amritansh1502/nri-plugins-operator) repository.

#### Unit tests (controller)

| Test | What it verifies |
|---|---|
| `TestNriPluginReconcile_CreatesResources` | Creating an NriPlugin CR produces MachineConfig, ServiceAccount, SCC, RBAC, DaemonSet |
| `TestNriPluginReconcile_AllowMutationsConfigMap` | `NriMutationPolicy` spec is marshaled into a ConfigMap mounted at `/config/config.yaml` |
| `TestNriPluginReconcile_StatusUpdates` | CR status reflects DaemonSet readyNodes, desiredNodes, and correct phase transitions |
| `TestNriPluginReconcile_MCOWaitRequeue` | Reconciler requeues while MachineConfigPool is still updating |
| `TestNriPluginReconcile_Cleanup` | Deleting the last NriPlugin CR removes the shared MachineConfig |

## Graduation Criteria

### Dev Preview -> Tech Preview

- Policy enforcement delivered via MachineConfig (Dev Preview) on worker nodes.
- Namespace scoped mutation stripping functional and tested on an OpenShift cluster.
- Unit test suite passing with no data races.
- Enhancement doc reviewed and merged.
- NRI Plugins Operator deployed and functional on OpenShift cluster.
- NriPlugin CRD registered, gated by `TechPreviewNoUpgrade` feature set.
- Operator reconciles NriPlugin CR end-to-end: MachineConfig creation, MCO rollout wait, DaemonSet deployment, status reporting.
- allow-mutations policy delivered via `NriMutationPolicy` CR and ConfigMap, no node reboot for policy changes.
- Reference-counted MachineConfig cleanup verified (last CR deletion disables NRI).

### Tech Preview -> GA

- End-to-end test suite covering all supported plugins, CR lifecycle, and failure recovery.
- Operator available via OLM with a stable channel.
- CRD promoted to stable API version.
- Documentation covering operator installation, plugin deployment, and policy configuration.
- Upgrade path from Tech Preview validated.

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

TBD

## Upgrade / Downgrade Strategy

**Dev Preview**

Plugin binary and config are delivered via MachineConfig. Upgrading means applying a new MachineConfig with the updated binary and config file, which triggers an MCO rollout (drain and reboot per node). Downgrading means reverting or removing the MachineConfig, which also triggers a rollout.

**Tech Preview**

- *Plugin Upgrade*:  update the `image` field in the `NriPlugin` CR. The operator updates the DaemonSet pod template and performs a rolling restart no node reboot required.
- *Policy change*: update the `NriMutationPolicy` CR spec. The operator updates the ConfigMap and triggers a rolling restart of the allow-mutations DaemonSet.
- *Operator upgrade*: standard Deployment rollout. The new controller picks up existing `NriPlugin` CRs and reconciles them. The shared MachineConfig (`99-nri-enable`) persists across operator upgrades — it only enables NRI in CRI-O and does not change between versions.
- *Downgrade*: revert the operator Deployment image. The controller reconciles existing CRs. If the CRD schema changed between versions, the administrator must ensure CR compatibility before downgrading.
- Deleting all `NriPlugin` CRs removes the shared MachineConfig and disables NRI cluster-wide. Re-creating a CR re-enables NRI and triggers another MachineConfig rollout.

## Version Skew Strategy

TBD

## Operational Aspects of API Extensions

TBD

## Support Procedures

TBD

## Infrastructure Needed [optional]

- New repository: `github.com/amritansh1502/nri-allow-mutations` hosts the standalone NRI policy plugin binary and its Go source.

