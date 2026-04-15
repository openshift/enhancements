---
title: nri-plugin-mutation-policy
authors:
  - "@amritansh1502"
  - "@ngopalak-redhat"
reviewers:
  - "@rphillips"
  - "@haircommander"
  - "@saschagrunert"
  - "@harche"
  - "@bitoku"
approvers:
  - "@rphillips"
api-approvers:
  - "@rphillips"
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

This enhancement proposes a small, standalone NRI policy plugin, not policy logic embedded in the CRI-O daemon, that decides whether a workload's Kubernetes namespace may receive that merged change. The plugin registers only `ValidateContainerAdjustment`, which NRI defines as a pass/fail gate: a plugin may approve or reject the merged adjustment, but it cannot edit or remove individual mutations from it. Policy is held in configuration outside the core daemon, with only minimal CRI-O settings to enable NRI and load the plugin.

## Motivation

Cluster administrators need a way to control which namespaces may receive NRI-driven container adjustments and which mutation categories are permitted. That decision must apply to the **merged** adjustment from all plugins, not to a single plugin's `CreateContainer` contribution.

### User Stories

- A cluster administrator can deploy a standalone NRI policy plugin that rejects or logs merged NRI container adjustments containing disallowed mutation categories on a per namespace basis, with unmatched namespaces passing through untouched, without relying on per-plugin `CreateContainer` logic that cannot see the full merged result.

### Goals

- Standalone NRI plugin that enforces namespace policy against **merged** container adjustments at the validation stage (`ValidateContainerAdjustment` only). Pod-level hooks (`RunPodSandbox`, `StopPodSandbox`, `RemovePodSandbox`) and other container lifecycle hooks are not in scope.
- Namespace scoped allow listing of mutation categories, enforced through an all-or-nothing accept/reject decision on the merged adjustment, with a configurable `mode` (`strict` rejects, `permissive` logs only) to control enforcement strength in v1.

### Non-Goals

## Proposal

### Workflow Description

A cluster administrator creates or updates the policy configuration (either via MachineConfig for Dev Preview, or via a policy CR for TP/GA). The standalone NRI policy plugin running on each worker node reads the config file at startup and registers the `ValidateContainerAdjustment` NRI hook. When a container is created, CRI-O calls the hook with the fully merged adjustment from all NRI plugins; the policy plugin evaluates the workload namespace against the configured rules. If the merged adjustment contains a mutation category the namespace's `mutations` policy does not permit, the plugin either rejects the container (`mode: strict`) or logs the violation and lets the container start unchanged (`mode: permissive`). Namespaces with no matching policy entry are never evaluated and always pass through untouched.

### API Extensions

None for Dev Preview. Tech Preview introduces a cluster-scoped CRD: `NRIPlugin` (`nri.openshift.io/v1alpha1`), managed by the NRI Plugins Operator. The CRD is gated by the `TechPreviewNoUpgrade` feature set. See the Tech Preview delivery section for full schema.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The plugin runs on data-plane worker nodes only and does not interact with the hosted control plane. Hosted clusters have no `MachineConfig`/`MachineConfigPool` API at all: HyperShift removes those CRDs from the hosted cluster. In Dev Preview, node configuration is instead delivered as a `ConfigMap` on the **management** cluster, referenced by the data plane's `NodePool`; this doc's `MachineConfig`-based delivery description applies there only in that wrapped form, not as a `MachineConfig` object inside the hosted cluster.

Tech Preview's operator reconciliation (`Creates a shared MachineConfig...`, `Waits for the MachineConfig Operator...`) assumes a standalone-cluster `MachineConfig`/`MachineConfigPool` API and does not work against a HyperShift hosted cluster as written. HyperShift support is out of scope for Tech Preview and is deferred to a future iteration that reworks delivery around `NodePool`-referenced `ConfigMap`s instead of `MachineConfig` CRs.

#### Standalone Clusters

Standard topology; no special considerations beyond the general MCO rollout behaviour described in Dev Preview.

#### Single-node Deployments or MicroShift

On SNO and compact (3-node) clusters, the node(s) belong to the `master` MachineConfigPool, not `worker`: a `MachineConfig` scoped to `worker` (as described for a standard cluster in Dev Preview and Tech Preview) never reaches them. Dev Preview delivery on these topologies must target the `master` pool instead. For Tech Preview, the operator must create and wait on a `MachineConfig` against every `MachineConfigPool` the targeted nodes actually belong to, derived from `spec.nodeSelector` rather than hardcoded to `worker`; a single selector can resolve to more than one pool at once (e.g. `node-role.kubernetes.io/worker: ""` also matches a custom pool like `worker-cnf`, and matches `master` nodes on compact clusters), so this is a set of pools to wait on, not a single one. A hardcoded `worker` pool wait, or waiting on only one of several matched pools, would complete immediately against an empty, nonexistent, or partial rollout and falsely report success while some nodes are never configured.

A MachineConfig rollout reboots the node, causing a temporary cluster outage; on SNO in particular, this means the whole cluster is briefly unavailable, so operators should schedule policy changes during a maintenance window. MicroShift is out of scope for this proposal (a future iteration could ship the plugin as a systemd service).

#### OpenShift Kubernetes Engine

No OKE-specific constraints; the plugin is a node-level NRI binary and does not depend on OpenShift-specific APIs beyond MCO delivery.

### Implementation Details/Notes/Constraints

- The plugin binary is statically compiled and has no runtime dependencies beyond the NRI socket provided by CRI-O.
- The plugin watches its config file and reloads it live in both delivery paths; no node reboot or process restart is needed to pick up a policy-only change. In Dev Preview this is the file at its canonical path (see Dev Preview delivery); in Tech Preview it is the file mounted from the `NRIMutationPolicy`-derived ConfigMap (see Tech Preview delivery), so updating the CR does not require restarting the allow-mutations DaemonSet either.
- The `OPENSHIFT_UNSUPPORTED_ALLOW_MUTATIONS_CONFIG` environment variable overrides the canonical config path for unit-test and development use only.
- The plugin registers only `ValidateContainerAdjustment`; CRI-O's NRI stub discovers this automatically from the exported methods.
- Mutation-category detection must be based on NRI's per-field `owners` tracking (which plugin index actually set a given field), not on presence/nil-checks. NRI always ships fields like `Resources` and `Hooks` as non-nil on the adjustment struct passed to validators, even when no plugin has set them, so a presence check produces false positives for every container.
- When handling a `CreateContainer` request, a plugin's response can also carry an `update` for other, already-running containers (distinct from the `adjust` for the container being created). The plugin must inspect both the `adjust` and the `update` fields on every request it validates; checking only `adjust` lets a bundled update to another container through unevaluated. However, a `ContainerUpdate` entry only carries a container ID and resource values, not a namespace; the request's own `pod` field describes the container being created, not the other container referenced in `update[]`. The plugin therefore cannot resolve which namespace's policy applies to a bundled update without becoming a container-lifecycle tracker itself (e.g. building a container-to-namespace map from NRI's `Synchronize` call and ongoing container events), which is out of scope for a plugin that registers only `ValidateContainerAdjustment`. For v1, the plugin logs that an unattributable bundled update occurred (visible for troubleshooting) but does not apply namespace-scoped `mutations` policy to it, since `ContainerUpdate` can only ever carry a `Resources` change and that gap is already documented as a residual risk above. Properly enforcing policy on these updates would require NRI itself to include the target container's pod/namespace information in `update[]` entries; that is a candidate upstream NRI feature request, not something this plugin can close unilaterally.
- CRI-O's `nri_validator_required_plugins` setting fails closed if the validator plugin is not connected, rejecting container creation instead of allowing it unchecked. In Dev Preview the plugin is started directly by CRI-O from `nri_plugin_dir` rather than created as a container, so it is never itself blocked by this requirement. In Tech Preview, where the plugin runs as a DaemonSet pod, its pod carries the `tolerate-missing-plugins.noderesource.dev: "true"` annotation so it can still be created while the requirement is unmet. See **Risks and Mitigations**.

### Risks and Mitigations

- The NRI policy plugin adds a new component to the container startup path. Because `ValidateContainerAdjustment` can only approve or reject the merged adjustment as a whole, enforcement is all-or-nothing per container: a misconfigured `strict` policy can reject every container in a namespace, including namespaces an administrator didn't intend to restrict as tightly.
- Mitigation: namespaces with no matching policy entry (including no `["*"]` catch-all) are never evaluated by the hook and always pass through untouched, so unlisted namespaces (e.g. `openshift-dns`, `openshift-monitoring`) are never blocked by omission. This is the intended behavior in every mode, including `strict`: `strict` only changes what happens for a namespace that *does* match an entry, it does not change whether a namespace matches one in the first place. (An earlier prototype build rejected unmatched namespaces under `strict` mode; that was a bug in that build, not the intended design, and is fixed.) For namespaces that do match a policy, the plugin supports a `mode` field: `permissive` logs the violation and approves the container unchanged, letting administrators observe the effect of a policy before switching the namespace to `strict`, which rejects the container outright and surfaces a `CreateContainerError` so the failure is immediately visible.

- NRI plugins run as privileged pods and need direct access to the NRI socket on the host; a compromised or buggy plugin can have node-level access as a result. Not every plugin needs the same grants, though: `allow-mutations` only needs the NRI socket hostPath, with no host networking or host ports, while a resource-policy plugin like `balloons` needs broader host resource visibility to pin CPUs/memory. The current operator implementation does not reflect this: every plugin shares one `ServiceAccount` (`nri-plugin`) and one `SecurityContextConstraints` (`nri-plugin-scc`) granting privileged pods, host networking, host ports, and host path mounts to all of them, regardless of what a given plugin actually uses. Mitigation: `NRIPlugin` is a cluster-scoped CR so only cluster admins can deploy plugins in the first place, and the operator must be updated to create a distinct SCC per plugin type, scoped to that plugin's actual requirements (e.g. `allow-mutations` gets hostPath access to the NRI socket only, without host networking or host ports), rather than one shared, maximally-permissive SCC for every plugin.

- If the policy plugin is not running (crashed, restarting, or failed to start on a config error), CRI-O by default creates containers with no check at all: enforcement silently stops with no visible signal. Mitigation: the CRI-O drop-in sets `nri_validator_required_plugins`, so CRI-O refuses to create any container while the plugin is disconnected instead of failing open. This trades availability for safety: while the plugin is down, no new containers can start on that node, so plugin reliability becomes operationally important. The plugin does not exit on a bad config (see Dev Preview delivery), which removes the most common way it would otherwise go down. A genuine crash (e.g. a panic) can still happen, and CRI-O's `nri_plugin_dir` auto-discovery does not restart a plugin that exits; recovering requires restarting CRI-O on that node (e.g. `systemctl restart crio`) or rebooting it, since the plugin only relaunches as part of CRI-O's own startup. Administrators should alert on CRI-O logging that a required plugin is disconnected, since that is the observable signal that this recovery step is needed.

- `resources` policy is only enforced at container creation time; it cannot catch resource changes made after a container is running. NRI only exposes a validation hook on `CreateContainer`; there is no equivalent hook for `UpdateContainer` or for updates a plugin pushes on its own initiative, so a change made through Kubernetes in-place pod resize, the static CPU manager, or a resource-policy plugin like `balloons` rebalancing containers after a config change bypasses the `mutations` policy entirely, with no way for any NRI validator to reject or even observe it. This is a limitation of the current NRI hook set, not something this plugin can close in v1. Mitigation: the enhancement documents this gap explicitly rather than implying `resources` policy is enforced for the lifetime of a container; administrators deploying resource-policy plugins alongside namespace-scoped `resources` restrictions should be aware that only the resource values present at container creation are checked.

- A workload pod can be scheduled and started before a mutating plugin it depends on (e.g. `balloons`) is ready, since nothing today prevents the kubelet from creating that pod while the plugin's DaemonSet is still rolling out; the container then starts without the adjustment it expected, with no error. The `NRIPlugin` CR's `readyNodes`/`desiredNodes` status lets an administrator check plugin readiness before deploying dependent workloads, but that's a manual check, not an enforced guarantee. Mitigation: CRI-O supports a `required-plugins.noderesource.dev` annotation that a *workload* pod can carry to make CRI-O refuse to create its containers until the named plugin is connected, for example `required-plugins.noderesource.dev: '["balloons"]'` (the value must be a YAML list; a plain string is rejected). This is distinct from, and does not need, the node-wide `nri_validator_required_plugins` setting used for the allow-mutations validator: it is opt-in per workload, set by whoever owns the pod spec that depends on a specific plugin, not something the operator can apply on an arbitrary user's behalf.

### Drawbacks

 TBD

### Enforcement point

The plugin registers with NRI under the plugin name `allow-mutations`. This is the name CRI-O matches against for any required-plugin check (`nri_validator_required_plugins`, the `required-plugins.noderesource.dev` pod annotation): both settings must reference `allow-mutations`, the registered NRI plugin name, not `nri-allow-mutations` (the DaemonSet's Kubernetes resource name in Tech Preview) and not the Dev Preview binary filename `10-allow-mutations` (CRI-O also accepts that form for a plugin loaded via `nri_plugin_dir` auto-discovery, since it matches both the registered name and the `index-name` form, but this doc uses the plain registered name consistently in both delivery paths to avoid confusion).

The plugin hooks into `ValidateContainerAdjustment`, which runs after CRI-O merges adjustments from all NRI plugins at container creation time. At this stage the full combined `adjust` for the container being created, and any bundled `update` for other already-running containers, are visible, so the plugin can evaluate mutations against namespace policy for both.

This is not the complete set of mutations a container can ever receive, however. `ValidateContainerAdjustment` only fires on `CreateContainer`; it is not invoked for resource changes made later via `UpdateContainer` (for example `crictl update`, Kubernetes in-place pod resize, or the static CPU manager), nor for updates a plugin pushes on its own initiative outside of a create/update request (for example a resource-policy plugin like `balloons` rebalancing other containers after a config change). These paths can only change a container's Linux resources (env, mounts, hooks, annotations, and devices cannot change after creation), but resource changes made through them are not evaluated against the `mutations` policy at all. See **Risks and Mitigations**.

### Policy schema (v1)

The on-disk file and any future CR **spec** share the same shape so plugin behavior is identical regardless of delivery path (file-based or Operator-managed). `policies[]` is evaluated in order; first matching entry wins.

#### Field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.policies[].namespaces` | []string | yes | Exact namespace names this policy entry covers. Use the single entry `["*"]` to match every namespace not matched by an earlier entry; this lets an administrator add a default-deny catch-all as the last entry in `policies[]`. |
| `spec.policies[].mutations.type` | string | yes | One of `AllowAll`, `AllowList`, `DenyAll`. A discriminated union instead of a plain list with a magic `"all"` value, so `type: AllowAll` with a populated `allowList` can't be expressed. |
| `spec.policies[].mutations.allowList` | []string | only when `type: AllowList` | Mutation categories permitted for matching namespaces. Setting `allowList` when `type` is `AllowAll` or `DenyAll` is treated the same as an unrecognized field: the plugin fails to load the config and logs an error (see the Tech Preview CRD section for the equivalent CEL-enforced, admission-time rejection). |
| `spec.policies[].mode` | string | no | One of `strict` or `permissive`. Defaults to `permissive` if omitted. `strict` rejects a container carrying a disallowed mutation; `permissive` logs the violation and approves the container unchanged. Scoped per policy entry, not per file or CR, so different namespaces can use different enforcement strength (e.g. `strict` for `production`, `permissive` while tuning a new policy for another namespace). |

This field is named `namespaces`, a plain list of exact names, rather than `namespaceSelector`: a Kubernetes `...Selector` field conventionally means label-based matching, which this is not. Matching namespaces by label instead of exact name would let new namespaces be covered automatically without editing the policy, but requires the plugin to have a Kubernetes client watching `Namespace` objects, which Dev Preview's file-based, cluster-API-independent plugin does not have. Label-based matching is deferred to a future iteration of the Tech Preview CRD, where the operator already maintains a Kubernetes client.

#### Allowed mutation category values

Category values use PascalCase, matching standard Kubernetes enum-field convention. NRI's `ContainerAdjustment` can carry more field types than this policy's named categories track individually (e.g. `rlimits`, `CDI_devices`, `args`, `seccomp_policy`, `namespaces`, `sysctl`, `oom_score_adj`, `cgroups_path`, `scheduler`, `rdt`, `net_devices`, `memory_policy`). Rather than enumerate every NRI field as its own category, any mutation to a field not covered by the six named categories below is classified as `Other` and is subject to default-deny: under `type: AllowList`, it is treated as disallowed unless `allowList` explicitly includes `Other`; under `type: AllowAll` it is permitted like any other category. This keeps the policy safe against NRI adding new adjustable fields in future API versions without requiring a schema update to stay restrictive.

| Value | What it covers |
|---|---|
| `Env` | Environment variable changes |
| `Mounts` | Volume / bind-mount changes |
| `Annotations` | Annotation changes |
| `Resources` | CPU / memory resource changes (Linux cgroup) |
| `Hooks` | OCI lifecycle hook changes |
| `Devices` | Linux device node changes |
| `Other` | Any mutated field not covered by the categories above; default-deny applies under `AllowList` |

**Upgrade note:** if a field currently covered only by `Other` is later promoted to its own named category (e.g. a future release adds a dedicated `Rlimits` category for a field that previously fell under `Other`), an existing `allowList: [Other]` entry stops permitting that field once the plugin upgrades to recognize the new name, since the field no longer falls under `Other`. This is expected and safe-directional (upgrading only ever makes an `AllowList` entry more restrictive, never more permissive), but administrators relying on `Other` to cover a specific field should add the field's new category name to `allowList` after an upgrade that introduces it, rather than assuming `Other` continues to cover it.

**Version skew:** because the on-disk file and the CR share this schema, and this category list is expected to grow, a plugin build must tolerate category values it doesn't recognize, for example a newer config shipped alongside an older plugin binary after a category is added in a later release. An unrecognized value in `allowList` is ignored: it never matches any mutation the plugin can detect, so it has no effect and does not grant any permission. This is safe by construction (an unrecognized value can only fail to permit something, never accidentally permit it), but it does mean an administrator relying on a category name an older plugin doesn't yet understand gets silent non-enforcement for that category rather than an error. The plugin logs any `allowList` entry it does not recognize, so this is observable rather than fully silent.

This runtime tolerance is how the Dev Preview file-based plugin handles version skew, since there is no admission-time validation on a plain YAML file. The Tech Preview `NRIMutationPolicy` CRD takes the opposite tradeoff for the same field: it validates `allowList` values against an enum at admission time (see API type definitions), catching a typo immediately rather than letting it silently match nothing. The cost of that stricter CRD-side check is that adopting a genuinely new category name through the CR path requires a CRD schema update first; until then, the API server rejects it rather than the plugin silently ignoring it. This is an intentional difference between the two delivery paths, not an inconsistency: file-based config optimizes for tolerating skew at runtime, the CRD optimizes for catching mistakes at the API boundary.

This tolerance applies only to unrecognized *values* within a known field, not to unrecognized *fields* in the config's structure. At load time, the plugin rejects a config file or CR containing a field it doesn't know about at all (e.g. a stale `namespaceSelector.matchNames` from before the `namespaces` rename, or a typo'd field name) and fails loudly, refusing to start (Dev Preview) or reporting `Degraded` (Tech Preview), rather than silently parsing it as an empty policy that matches nothing and lets every namespace pass through unenforced.

#### Full example

```yaml
spec:
  policies:
    - namespaces: ["kube-system", "openshift-monitoring"]
      mutations:
        type: AllowList
        allowList: [Env, Mounts, Resources]
      mode: strict

    - namespaces: ["my-app"]
      mutations:
        type: AllowList
        allowList: [Env]
      mode: permissive

    - namespaces: ["trusted-ns"]
      mutations:
        type: AllowAll

    - namespaces: ["*"]
      mutations:
        type: DenyAll
      mode: permissive
```

The final `["*"]` entry is a default-deny catch-all: any namespace not matched by an earlier, more specific entry falls through to it instead of passing through unevaluated. Using `mode: strict` here would reject every container in every namespace not explicitly listed elsewhere in the policy, including OpenShift system namespaces (`openshift-dns`, `openshift-monitoring`, etc.) that this example does not list. An administrator enabling a `strict` catch-all must first explicitly allow-list (or exclude) every system namespace the cluster depends on; this example uses `mode: permissive` on the catch-all precisely to avoid that footgun by default.

#### Evaluation logic

Namespace matching and mode/mutations evaluation are two separate steps: a namespace is first matched against `policies[]` (explicit entries, then `["*"]` if present), and only a namespace that matches some entry is then subject to that entry's `mutations`/`mode`. A namespace matching nothing at all, because no `["*"]` entry exists, is never evaluated, regardless of what `mode` or `mutations` values appear elsewhere in the file; this is the intended behavior, not merely the behavior when no catch-all is configured.

| Condition | Behavior |
|---|---|
| Namespace matches no entry, and no `["*"]` catch-all entry is configured | No hook, container runs with all mutations untouched |
| Namespace matches no specific entry, but a `["*"]` catch-all entry is configured | The catch-all entry's `mutations` and `mode` apply |
| Namespace matches policy, `mutations.type: AllowAll` | All mutations permitted |
| Namespace matches policy, `mutations.type: DenyAll` | All mutations disallowed: rejected (`mode: strict`) or logged (`mode: permissive`) |
| Namespace matches policy, `mutations.type: AllowList`, category in `allowList` | That mutation is permitted |
| Namespace matches policy, `mutations.type: AllowList`, category NOT in `allowList` | Disallowed: rejected (`mode: strict`) or logged (`mode: permissive`) |
| Namespace matches policy, mutated field is not one of the six named categories | Classified as `Other`; follows the same `AllowAll`/`AllowList`/`DenyAll` evaluation as any other category |

### Dev Preview: file-based delivery

- Ship the standalone NRI policy plugin and its policy file as node configuration. CRI-O enables NRI and loads the plugin through the normal NRI integration path.

- At validation time, the plugin evaluates the workload namespace against the configured rules (see **Policy schema (v1)**). The plugin watches the config file at its fixed canonical path and reloads it live on change, so a new policy takes effect without restarting the plugin process. An `OPENSHIFT_UNSUPPORTED_ALLOW_MUTATIONS_CONFIG` environment variable overrides the path for testing only.

- The plugin never exits on an invalid config, whether at initial startup or on a later reload. An invalid or unparseable config is logged as an error, and the plugin keeps running and stays connected to CRI-O: on a bad reload, it keeps enforcing the last successfully loaded policy; on a bad config at first startup with no prior successfully loaded policy, it runs with an empty policy, the same documented behavior as `policies[]` being empty (no namespace is matched, so every namespace passes through untouched). This matters because CRI-O's `nri_plugin_dir` auto-discovery does not restart a plugin that exits, so exiting on a bad config would otherwise turn a config typo into node-wide container creation failures (via `nri_validator_required_plugins`) that only clear once something external restarts CRI-O. See **Risks and Mitigations**.

- Policy is read from the single canonical path: `/etc/crio/nri_plugins/AllowMutations/config.yaml`.

- Changes ship via the Machine Config Operator: a `MachineConfig` for the target pool uses Ignition `storage.files` to install. The target pool is `worker` on a standard cluster, but `master` on SNO and compact (3-node) clusters, where nodes belong to the `master` pool; a `worker`-scoped MachineConfig never reaches them. See **Topology Considerations**. The MachineConfig installs:
  1. the plugin binary at `/etc/crio/nri_plugins/10-allow-mutations`, the NRI plugin auto-discovery directory configured in step 3 below (CRI-O's own upstream default is `/opt/nri/plugins`, overridden here to group the binary alongside the policy file under `/etc/crio`). CRI-O itself discovers and starts binaries placed in its configured plugin directory as part of its own startup, using only stock, upstream auto-discovery behavior: no CRI-O fork or custom config section is required, and no separate systemd unit races against CRI-O's startup. The numeric prefix follows NRI's plugin-ordering convention;
  2. the policy file at `/etc/crio/nri_plugins/AllowMutations/config.yaml`;
  3. a CRI-O drop-in under `/etc/crio/crio.conf.d/` that enables NRI, sets `nri_plugin_dir = "/etc/crio/nri_plugins"`, and sets `nri_validator_required_plugins = ["allow-mutations"]` (the registered NRI plugin name, see Enforcement point) so CRI-O refuses to create containers if the validator isn't connected, instead of silently creating them unchecked. The explicit `nri_plugin_dir` is required: upstream CRI-O's default plugin directory is `/opt/nri/plugins`, and OpenShift's MCO-shipped CRI-O config does not set `nri_plugin_dir` to override it, so without this setting CRI-O would never discover a binary placed under `/etc/crio/nri_plugins`.

- The cluster-scoped `MachineConfiguration` object (not the `MachineConfig` itself) declares an [admin-defined node disruption policy](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/admin-defined-node-disruption-policy.md) at `spec.nodeDisruptionPolicy`, mapping the policy file path (`/etc/crio/nri_plugins/AllowMutations/config.yaml`) to action `None`, since the plugin reloads that file live. Policy-only changes are therefore applied by MCO without draining or rebooting the node. Changes to the plugin binary or the CRI-O drop-in still require a full MachineConfig rollout (drain and reboot), since those do require the plugin process or CRI-O to restart. Because `None` means MCO does not even restart the plugin after writing the file, correctness depends entirely on the plugin's live reload handling a bad file without going down; see the "plugin never exits on an invalid config" behavior described above.

#### Pros

- Policy is enforced from the very first container on a fresh node; it works before the Kubernetes API server is up.
- No additional in-cluster components; smaller attack surface and no new failure domain.
- Reuses the existing MCO/Ignition delivery path already proven for all other node configuration in OpenShift.
- Policy-only changes apply without a node reboot, via a node disruption policy of `None` on the policy file combined with the plugin's live config reload.

#### Cons

- Changes to the plugin binary or CRI-O drop-in still require a MachineConfig rollout: nodes are drained and rebooted, taking 20-45 minutes per cluster and disrupting running workloads. Policy-only changes avoid this via the node disruption policy above.
- No live status reporting; there is no CR condition to observe whether the policy is in sync on each node.

### Tech Preview: Operator-managed delivery

For Tech Preview the file-based MachineConfig delivery path is replaced by the **NRI Plugins Operator** ([`nri-plugins-operator`](https://github.com/amritansh1502/nri-plugins-operator)). The operator is the single supported entry point for NRI plugin deployment on OpenShift. An administrator creates an `NRIPlugin` CR to deploy a plugin and optionally creates an `NRIMutationPolicy` CR to define namespace-scoped mutation policy.

Out of the box a cluster has no NRI plugins installed, but NRI itself is already enabled in CRI-O by default on OpenShift worker nodes (confirmed via `crio config` on a live cluster), so the operator does not need to enable it or trigger a MachineConfig rollout to do so. When the first `NRIPlugin` CR is created, the operator deploys the requested plugin as a DaemonSet directly, and automatically deploys the allow-mutations validation plugin alongside it. Mutation policy is defined via `NRIMutationPolicy` CRs; namespaces with no matching policy pass through untouched.

The operator does not assume exclusive ownership of NRI plugins on the node. Plugins deployed outside the operator (e.g. manually installed DaemonSets) are left untouched, the operator only manages plugins created through `NRIPlugin` CRs.

#### NRIPlugin CRD

The operator introduces a cluster-scoped CRD (`nri.openshift.io/v1alpha1`, kind `NRIPlugin`). The CRD is gated by the `TechPreviewNoUpgrade` feature set.

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.pluginName` | string | yes | Identifies which known NRI plugin type this CR deploys (e.g. `allow-mutations`, `balloons`), used to select the plugin-specific SCC and defaults. Not required to be unique: the DaemonSet and other generated resources are named from `metadata.name`, not `pluginName`, so two CRs with the same `pluginName` (e.g. two differently-configured `balloons` deployments) do not collide. |
| `spec.pluginIndex` | int32 | no | The NRI plugin index (`0`-`99`, matching NRI's 2-digit index convention; enforced by CRD validation), which determines call order among mutating plugins when multiple plugins adjust the same container. If unset, the operator assigns the lowest index in `0`-`99` not already used by another `NRIPlugin` CR, reserving `10` for the auto-deployed `allow-mutations` validator so operator-assigned indices start from `11`. |
| `spec.image` | string | yes | Container image for the plugin, **by digest** (`...@sha256:...`), not by mutable tag; enforced by CRD validation (pattern or CEL rule), not only by convention. Required so every node runs identical content and so image mirroring in disconnected clusters resolves correctly: `ImageDigestMirrorSet` only reliably matches digest references, not tags. |
| `spec.nodeSelector` | map[string]string | no | Targets which nodes get this plugin. A single selector can match nodes in more than one `MachineConfigPool` (e.g. `node-role.kubernetes.io/worker: ""` matches `worker` nodes, custom pools such as `worker-cnf` whose nodes also carry the worker role label, and on compact clusters, `master` nodes that run workloads); see Reconciliation overview for how the operator handles this. |
| `spec.pluginConfig` | object | no | For plugins that read config from a Kubernetes CR (topology-aware, balloons, etc.). Uses `group`/`resource` (not `apiVersion`/`kind`), matching OpenShift's convention for referencing other objects. `group` is restricted by CRD validation to the groups the operator's `ClusterRole` can actually manage (currently `config.nri`), so an out-of-scope reference is rejected at CR creation time rather than failing later at reconcile. |

Status fields:

| Field | Type | Description |
|---|---|---|
| `status.readyNodes` | int32 | Number of nodes where the plugin pod is running. |
| `status.desiredNodes` | int32 | Total number of targeted nodes. |
| `status.conditions` | []Condition | Standard Kubernetes conditions, not a `phase` string (deprecated in current API conventions). Documented types: `Available` (plugin DaemonSet has at least one ready pod), `Progressing` (deployment in progress, with a `reason` such as `WaitingForMachineConfig` or `DeployingPlugin`), `Degraded` (deployment failed or pods are crash-looping). |

#### NRIMutationPolicy CRD

The operator introduces a second cluster-scoped CRD (`nri.openshift.io/v1alpha1`, kind `NRIMutationPolicy`) for namespace-scoped mutation policy. Mutation policy is managed separately from plugin deployment, so administrators can change policy without touching plugin CRs.

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.policies[].namespaces` | []string | yes | Exact namespace names this policy entry covers. Use `["*"]` as the last entry for a default-deny catch-all. |
| `spec.policies[].mutations.type` | string | yes | One of `AllowAll`, `AllowList`, `DenyAll`. Same semantics as Policy schema (v1). |
| `spec.policies[].mutations.allowList` | []string | only when `type: AllowList` | Mutation categories permitted for matching namespaces. Same values as Policy schema (v1). Enforced by CRD validation: a CEL rule rejects a CR at admission time if `allowList` is set while `type` isn't `AllowList`, and an enum constrains `allowList` to recognized category values. |
| `spec.policies[].mode` | string | no | One of `strict` or `permissive`. Same semantics as Policy schema (v1), scoped per policy entry. |

The operator watches `NRIMutationPolicy` CRs and marshals the combined policy into a ConfigMap mounted into the auto-deployed allow-mutations plugin. The plugin watches and reloads that mounted file live, the same mechanism used in Dev Preview, so a policy change takes effect as the kubelet syncs the updated ConfigMap to the mounted file, without a DaemonSet pod restart or a node reboot. This avoids the outage window a rolling restart would otherwise introduce now that the node fails closed: restarting the validator pod would mean a period where the old pod is gone and the new one hasn't registered yet, during which CRI-O would reject every container on that node. Live reload keeps the same already-registered plugin process running throughout a policy update.

Status reports standard Kubernetes `conditions` only: type `Active` (`True`/`False`) for whether the policy is currently applied to the allow-mutations plugin, with a `reason` describing failures (e.g. invalid policy). No `phase` or `policyCount` field: `phase` is a deprecated pattern in current API conventions, and a policy count is trivially derivable from `len(spec.policies)` and does not belong in status.

##### NRIMutationPolicy example

```yaml
apiVersion: nri.openshift.io/v1alpha1
kind: NRIMutationPolicy
metadata:
  name: production-policy
spec:
  policies:
    - namespaces: ["production", "prod-workloads"]
      mutations:
        type: AllowList
        allowList: [Env, Annotations]
      mode: strict
    - namespaces: ["trusted-ns"]
      mutations:
        type: AllowAll
```

#### API type definitions

```go
// NRIPluginSpec defines the desired state of an NRI plugin deployment.
type NRIPluginSpec struct {
	PluginName string `json:"pluginName"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=99
	PluginIndex *int32 `json:"pluginIndex,omitempty"`
	// +kubebuilder:validation:Pattern=`^.+@sha256:[0-9a-f]{64}$`
	Image        string            `json:"image"`
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	PluginConfig *PluginConfig     `json:"pluginConfig,omitempty"`
}

// PluginConfig describes a config custom resource the operator should
// create for the plugin.
type PluginConfig struct {
	Group     string               `json:"group"`
	Resource  string               `json:"resource"`
	Name      string               `json:"name"`
	Namespace string               `json:"namespace"`
	Spec      runtime.RawExtension `json:"spec"`
}

// NRIPluginStatus defines the observed state of an NRI plugin deployment.
type NRIPluginStatus struct {
	ReadyNodes   int32              `json:"readyNodes,omitempty"`
	DesiredNodes int32              `json:"desiredNodes,omitempty"`
	Conditions   []metav1.Condition `json:"conditions,omitempty"`
}

// NRIPlugin is the Schema for the nriplugins API.
// +kubebuilder:validation:XValidation:rule="self.metadata.name != 'allow-mutations'",message="metadata.name 'allow-mutations' is reserved for the auto-deployed validator"
type NRIPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NRIPluginSpec   `json:"spec,omitempty"`
	Status NRIPluginStatus `json:"status,omitempty"`
}

// NRIMutationPolicySpec defines the desired mutation policy.
type NRIMutationPolicySpec struct {
	Policies []PolicyEntry `json:"policies"`
}

// PolicyEntry is a single namespace-scoped mutation policy.
type PolicyEntry struct {
	// Namespaces lists exact namespace names this entry covers.
	// Use ["*"] as the last entry for a default-deny catch-all.
	Namespaces []string       `json:"namespaces"`
	Mutations  MutationPolicy `json:"mutations"`
	// Mode is "strict" or "permissive", defaulting to "permissive" if unset.
	Mode string `json:"mode,omitempty"`
}

// MutationType is a discriminated union tag: exactly one of AllowAll,
// AllowList, or DenyAll. Using a plain list with a magic "all" value would
// let AllowList be populated while also meaning "allow everything",
// which is not a meaningful state.
type MutationType string

const (
	MutationTypeAllowAll  MutationType = "AllowAll"
	MutationTypeAllowList MutationType = "AllowList"
	MutationTypeDenyAll   MutationType = "DenyAll"
)

// MutationPolicy selects which mutation categories are permitted.
// +kubebuilder:validation:XValidation:rule="self.type == 'AllowList' || !has(self.allowList)",message="allowList may only be set when type is AllowList"
type MutationPolicy struct {
	// +kubebuilder:validation:Enum=AllowAll;AllowList;DenyAll
	Type MutationType `json:"type"`
	// AllowList is only meaningful, and only accepted by the CEL rule above, when Type is AllowList.
	// +kubebuilder:validation:Enum=Env;Mounts;Annotations;Resources;Hooks;Devices;Other
	AllowList []string `json:"allowList,omitempty"`
}

// NRIMutationPolicy is the Schema for the nrimutationpolicies API.
type NRIMutationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NRIMutationPolicySpec `json:"spec,omitempty"`
}
```

#### Reconciliation overview

When an `NRIPlugin` CR is created the operator reconciles in this order:

1. Determines every `MachineConfigPool` that contains at least one node matched by `spec.nodeSelector`. A single selector can resolve to more than one pool (e.g. `node-role.kubernetes.io/worker: ""` matches `worker` nodes, a custom pool such as `worker-cnf`, and, on compact clusters, `master` nodes). Creates a shared MachineConfig (`99-nri-validator-required`) that sets `nri_validator_required_plugins = ["allow-mutations"]` (the registered NRI plugin name, not the `nri-allow-mutations` DaemonSet name), applied to every pool identified, rather than hardcoding `worker`. This is for the fail-closed behavior described in **Risks and Mitigations**. NRI itself is already enabled by default in CRI-O on OpenShift worker nodes (confirmed via `crio config` on a live cluster), so the operator does not create a MachineConfig to enable it. This step does not apply on HyperShift; see **Topology Considerations**.
2. Waits for every pool identified in step 1 to finish updating. Waiting on a hardcoded `worker` pool, or on only one of several matched pools, would complete immediately against an empty, nonexistent, or partial rollout on SNO/compact clusters and falsely report success.
3. Creates a `ServiceAccount` and a `SecurityContextConstraints` scoped to `spec.pluginName`'s actual requirements, rather than one SCC shared by every plugin: the `allow-mutations` SCC grants hostPath access to the NRI socket only, with no host networking or host ports, while a resource-policy plugin like `balloons` is granted the broader host resource visibility it needs.
4. For plugins with `pluginConfig`: creates the referenced config CR with the provided spec.
5. Deploys the requested plugin as a DaemonSet (`nri-<metadata.name>`) on all nodes matched by `spec.nodeSelector`, regardless of which pool(s) they belong to, using the SCC and ServiceAccount from step 3. All plugin pods mount the NRI socket; host networking and host ports are granted only to plugins whose SCC requires them. The plugin pods do not carry a `required-plugins.noderesource.dev` annotation themselves: the node-wide `nri_validator_required_plugins` setting from step 1 already holds every container, including other plugins' pods, until the validator is connected, so an additional annotation on these pods would be redundant. See **Risks and Mitigations** for the separate, still-open problem of a *workload* pod starting before a mutating plugin like `balloons` is ready. `metadata.name: allow-mutations` is reserved and rejected by CRD validation on `NRIPlugin`, since it would collide with the auto-deployed validator's own `nri-allow-mutations` DaemonSet from step 6.
6. Deploys the allow-mutations validation plugin as a DaemonSet (`nri-allow-mutations`) if not already running, configured by `NRIMutationPolicy` CRs. Its pod template carries the `tolerate-missing-plugins.noderesource.dev: "true"` annotation so the pod can be created even while CRI-O's `nri_validator_required_plugins` requirement is unmet (the plugin is not yet connected to enforce it against its own startup).
7. Updates CR status (conditions, readyNodes, desiredNodes) from the DaemonSet status.

When an `NRIMutationPolicy` CR is created or updated, the operator marshals the policy into a ConfigMap; the allow-mutations plugin picks up the change via its live file reload, without the operator restarting the DaemonSet.

When the last `NRIPlugin` CR is deleted the operator removes both the allow-mutations DaemonSet and the shared `99-nri-validator-required` MachineConfig. This reverts CRI-O's `nri_validator_required_plugins` setting, restoring fail-open behavior for any future plugin connection; it does not disable NRI itself, which remains enabled by default regardless of this MachineConfig's presence. Plugin DaemonSets are garbage-collected through owner references.

#### balloons CR example

```yaml
apiVersion: nri.openshift.io/v1alpha1
kind: NRIPlugin
metadata:
  name: balloons
spec:
  pluginName: balloons
  image: "ghcr.io/containers/nri-plugins/nri-resource-policy-balloons@sha256:3b2b...c1a0"
  nodeSelector:
    node-role.kubernetes.io/worker: ""
  pluginConfig:
    group: config.nri
    resource: balloonpolicies
    name: default
    namespace: kube-system
    spec:
      pinCPU: true
      pinMemory: true
      reservedResources:
        cpu: "750m"
```

The allow-mutations plugin behavior is identical to Dev Preview; only the delivery changes from static file to `NRIMutationPolicy` CR and ConfigMap. Policy changes no longer require a node reboot.

#### Pros

- No node reboot for policy changes.
- Live status reporting via CR status and conditions (`oc get nriplugin`).
- Single entry point for NRI plugin lifecycle.
- Reference-counted MachineConfig cleanup.

#### Cons

- Operator adds a new in-cluster component with its own failure domain.
- First NRIPlugin CR still triggers a MachineConfig rollout (one-time node reboot to set `nri_validator_required_plugins`; NRI itself is already enabled by default and is not being turned on by this rollout).
- Plugin pods require a privileged security context to mount the NRI socket; host networking and host ports are granted only to plugins whose SCC requires them (see step 3 of Reconciliation overview), not to every plugin.

## Alternatives (Not Implemented)

### CRI-O's built-in default validator

CRI-O already ships a built-in policy mechanism, the default validator (`[crio.nri.default_validator]`), which can block `hooks`, `seccomp`, `namespace`, and `sysctl` changes and require specific plugins to be running (the basis for this enhancement's `nri_validator_required_plugins` fail-closed setting). It was considered as the sole enforcement mechanism but is not sufficient on its own: it applies one set of rules **node-wide**, with no concept of the Kubernetes namespace a workload belongs to. It cannot express "namespace A gets a different policy than namespace B," which is the core requirement here. The standalone plugin model is still needed to add that namespace-scoped layer on top of, not instead of, the default validator.

Embedding the full namespace-scoped policy logic directly in the CRI-O daemon (beyond what the default validator already offers) was also considered but rejected: it couples policy management to CRI-O release cycles, increases daemon complexity, and prevents independent updates to the policy rules. The standalone plugin model keeps policy outside the daemon.

### Per-plugin rules instead of per-namespace rules

Rather than scoping policy by the workload's Kubernetes namespace, rules could instead be scoped by plugin identity, for example, "only the `balloons` plugin (`nri-resource-policy-balloons`, the same plugin used in this proposal's `NRIPlugin` example) may change `resources`." NRI's adjustment request carries `owners` data identifying which plugin set each field, which is enough information to express such a rule. This was not adopted for v1 because the stated requirement is controlling which *workloads* (by namespace) may receive which mutation categories, regardless of which plugin produced them; per-plugin rules answer a different question ("which plugin may mutate what") and would need to compose with, not replace, namespace-scoped policy. It may be worth revisiting as an additional, orthogonal policy dimension in a future iteration.

## Open Questions

- **Payload vs. OLM delivery for Tech Preview.** This doc currently states both that the `NRIPlugin`/`NRIMutationPolicy` CRDs are "gated by the `TechPreviewNoUpgrade` feature set" and that the operator is "available via OLM with a stable channel" at GA. These describe two different, mutually exclusive delivery models:
  - *Payload component*: requires a named `FeatureGate` in `openshift/api`, a `ClusterOperator` reporting status, and likely a `Capability` declaration, since this isn't a core cluster feature. `TechPreviewNoUpgrade` gating only applies to this model.
  - *OLM-delivered operator*: distributed via a catalog with a `Subscription` channel (e.g. a `tech-preview` channel distinct from `stable`), independent of the OpenShift release cycle. `TechPreviewNoUpgrade` does not apply here; the `TechPreviewNoUpgrade` references in this doc would need to be removed and replaced with a defined Tech Preview channel.

  This choice also decides who builds and publishes the plugin and operator images, currently hosted in personal repositories, and how CVE fixes and NRI library bumps reach them: ART owns payload-component builds, Konflux owns OLM-delivered operator/catalog builds. This needs to be resolved, and the repositories moved to an appropriate org, before this enhancement can graduate past Dev Preview.

## Test Plan

### Test levels

- **Unit tests**, in the plugin and operator repositories respectively: policy evaluation logic (namespace matching, `mutations`/`mode` semantics, mutation-category detection), and operator reconciliation logic (resource creation, status updates, MachineConfigPool targeting). Run as part of each repository's own CI.
- **Integration tests against a real CRI-O build**, not a mocked NRI server: this proposal depends on subtle, easy-to-get-wrong CRI-O/NRI protocol behavior (`owners` data, bundled `update[]` entries, required-plugin fail-closed behavior), which a mock could misrepresent. These run against CRI-O built from the OpenShift fork, exercising the plugin as a real NRI client.
- **End-to-end tests in `openshift/origin`** (or the equivalent OpenShift conformance suite), covering the full Dev Preview MachineConfig delivery and the Tech Preview operator-managed delivery against a live cluster. Required before GA per this enhancement's graduation criteria.

### Environments

- CRI-O running alongside other NRI plugins (e.g. `balloons`) simultaneously, to verify policy enforcement and the `update[]` namespace-attribution limitation behave correctly when multiple plugins are active, not just the allow-mutations plugin in isolation.
- SNO, compact (3-node), and custom-pool (e.g. `worker-cnf`) topologies, to verify `MachineConfigPool` targeting resolves correctly, including a `nodeSelector` that spans more than one pool.
- Upgrade and downgrade scenarios: operator upgrade with existing `NRIPlugin`/`NRIMutationPolicy` CRs in place, and policy schema version skew (an older plugin build encountering a newer config's category names).

### Behaviors to verify

- Mutation-category detection uses NRI's `owners` data, not field presence: `Resources`, `Hooks`, and similar fields are always non-nil by default, so a presence check would produce false positives on every container.
- A plugin's response bundling an `update` for another, already-running container is detected (both `adjust` and `update` inspected), with the documented limitation that the plugin cannot attribute that update to a namespace and therefore does not enforce `mutations` policy on it.
- Fail-closed behavior while the validator plugin is disconnected (`nri_validator_required_plugins`), and that a node recovers via `systemctl restart crio` after a genuine crash; the plugin never exits on an invalid config, so a config error does not trigger this condition.
- Policy rollout without a reboot or pod restart: Dev Preview's node-disruption-policy-driven live file reload, and Tech Preview's live reload of the mounted `NRIMutationPolicy` ConfigMap.
- The default-deny `["*"]` catch-all, per-entry `mode` (`strict` rejects, `permissive` logs and allows), and the guarantee that a namespace matching no entry at all always passes through untouched regardless of `mode` elsewhere in the policy.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Policy enforcement delivered via MachineConfig (Dev Preview) on worker nodes.
- Namespace scoped mutation enforcement (reject/log via `mode`) functional and tested on an OpenShift cluster.
- Unit test suite passing with no data races.
- Enhancement doc reviewed and merged.
- NRI Plugins Operator deployed and functional on OpenShift cluster.
- NRIPlugin CRD registered, gated by `TechPreviewNoUpgrade` feature set.
- Operator reconciles NRIPlugin CR end-to-end: MachineConfig creation, MCO rollout wait, DaemonSet deployment, status reporting.
- allow-mutations policy delivered via `NRIMutationPolicy` CR and ConfigMap, no node reboot for policy changes.
- Per-plugin SCCs verified: each deployed plugin gets a `ServiceAccount`/`SecurityContextConstraints` scoped to its actual requirements (e.g. `allow-mutations` without host networking or host ports), not one shared, maximally-permissive SCC for every plugin.
- Reference-counted MachineConfig cleanup verified (last CR deletion removes the `99-nri-validator-required` MachineConfig).

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

- *Plugin Upgrade*:  update the `image` field in the `NRIPlugin` CR. The operator updates the DaemonSet pod template and performs a rolling restart no node reboot required.
- *Policy change*: update the `NRIMutationPolicy` CR spec. The operator updates the ConfigMap; the running allow-mutations pod reloads it live, no pod restart or node reboot required.
- *Operator upgrade*: standard Deployment rollout. The new controller picks up existing `NRIPlugin` CRs and reconciles them. The shared MachineConfig (`99-nri-validator-required`) persists across operator upgrades; it only sets `nri_validator_required_plugins` and does not change between versions.
- *Downgrade*: revert the operator Deployment image. The controller reconciles existing CRs. If the CRD schema changed between versions, the administrator must ensure CR compatibility before downgrading.
- Deleting all `NRIPlugin` CRs removes the shared `99-nri-validator-required` MachineConfig. This reverts CRI-O's `nri_validator_required_plugins` setting (restoring fail-open behavior for any future plugin connection); it does not disable NRI itself, which remains enabled by default regardless of this MachineConfig's presence. Re-creating a CR re-creates the MachineConfig and triggers another rollout.

## Version Skew Strategy

TBD

## Operational Aspects of API Extensions

TBD

## Support Procedures

TBD

## Infrastructure Needed [optional]

- New repository: `github.com/amritansh1502/nri-allow-mutations` hosts the standalone NRI policy plugin binary and its Go source.

