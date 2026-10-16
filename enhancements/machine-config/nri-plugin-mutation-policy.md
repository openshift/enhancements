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
- Namespace scoped allow listing of mutation categories, enforced through an all-or-nothing accept/reject decision on the merged adjustment, with a configurable `mode` (`Strict` rejects, `Permissive` logs only) to control enforcement strength in v1.

### Non-Goals

## Proposal

### Workflow Description

A cluster administrator creates or updates the policy configuration (either via MachineConfig for Dev Preview, or via a policy CR for TP/GA). The standalone NRI policy plugin running on each worker node reads the config file at startup and registers the `ValidateContainerAdjustment` NRI hook. When a container is created, CRI-O calls the hook with the fully merged adjustment from all NRI plugins; the policy plugin evaluates the workload namespace against the configured rules. If the merged adjustment contains a mutation category the namespace's `mutations` policy does not permit, the plugin either rejects the container (`mode: Strict`) or logs the violation and lets the container start unchanged (`mode: Permissive`). Namespaces with no matching policy entry are never evaluated and always pass through untouched.

### API Extensions

None for Dev Preview. Tech Preview introduces a cluster-scoped CRD: `NRIMutationPolicy` (`nri.openshift.io/v1alpha1`), managed by the NRI Plugins Operator. The CRD is gated by the `TechPreviewNoUpgrade` feature set, pending the payload-vs-OLM decision in **Open Questions**. See the Tech Preview delivery section for full schema.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The plugin runs on data-plane worker nodes only and does not interact with the hosted control plane. Hosted clusters have no `MachineConfig`/`MachineConfigPool` API at all: HyperShift removes those CRDs from the hosted cluster. In Dev Preview, node configuration is instead delivered as a `ConfigMap` on the **management** cluster, referenced by the data plane's `NodePool`; this doc's `MachineConfig`-based delivery description applies there only in that wrapped form, not as a `MachineConfig` object inside the hosted cluster.

Tech Preview's operator reconciliation (`Creates a shared MachineConfig...`, `Waits for the MachineConfig Operator...`) assumes a standalone-cluster `MachineConfig`/`MachineConfigPool` API and does not work against a HyperShift hosted cluster as written. HyperShift support is out of scope for Tech Preview and is deferred to a future iteration that reworks delivery around `NodePool`-referenced `ConfigMap`s instead of `MachineConfig` CRs.

#### Standalone Clusters

Standard topology; no special considerations beyond the general MCO rollout behaviour described in Dev Preview.

#### Single-node Deployments or MicroShift

On SNO and compact (3-node) clusters, the node(s) belong to the `master` MachineConfigPool, not `worker`: a `MachineConfig` labeled with the `worker` role (as described for a standard cluster in Dev Preview and Tech Preview) never reaches them, since a `MachineConfig` carries a single `machineconfiguration.openshift.io/role` label and a pool only picks up configs its own `machineConfigSelector` matches. Dev Preview and Tech Preview delivery on these topologies must additionally create (or instead create) a `MachineConfig` labeled with the `master` role. A custom pool such as `worker-cnf` needs no separate `MachineConfig` of its own: by convention its `machineConfigSelector` also matches the base `worker` role label, so it inherits the `worker`-labeled `MachineConfig` automatically. Tech Preview's operator must wait on every pool it actually created a `MachineConfig` for (`worker`, and `master` where applicable), not a hardcoded `worker` pool alone, or it would complete immediately against an empty or nonexistent rollout on SNO/compact clusters and falsely report success while the node is never configured.

A MachineConfig rollout reboots the node, causing a temporary cluster outage; on SNO in particular, this means the whole cluster is briefly unavailable, so operators should schedule policy changes during a maintenance window. MicroShift is out of scope for this proposal (a future iteration could ship the plugin as a systemd service).

#### OpenShift Kubernetes Engine

No OKE-specific constraints; the plugin is a node-level NRI binary and does not depend on OpenShift-specific APIs beyond MCO delivery.

### Implementation Details/Notes/Constraints

- The plugin binary is statically compiled and has no runtime dependencies beyond the NRI socket provided by CRI-O.
- The plugin watches its config file and reloads it live in both delivery paths; no node reboot or process restart is needed to pick up a policy-only change. In both Dev Preview and Tech Preview this is the same canonical on-disk path (see Dev Preview delivery and Tech Preview delivery); in Tech Preview the operator reconciles the singleton `NRIMutationPolicy` CR into that file's content, but there is no pod or DaemonSet to restart either way, since the validator is never a Kubernetes pod.
- The `OPENSHIFT_UNSUPPORTED_ALLOW_MUTATIONS_CONFIG` environment variable overrides the canonical config path for unit-test and development use only.
- The plugin registers only `ValidateContainerAdjustment`; CRI-O's NRI stub discovers this automatically from the exported methods.
- Mutation-category detection must be based on NRI's per-field `owners` tracking (which plugin index actually set a given field), not on presence/nil-checks. NRI always ships fields like `Resources` and `Hooks` as non-nil on the adjustment struct passed to validators, even when no plugin has set them, so a presence check produces false positives for every container.
- When handling a `CreateContainer` request, a plugin's response can also carry an `update` for other, already-running containers (distinct from the `adjust` for the container being created). The plugin must inspect both the `adjust` and the `update` fields on every request it validates; checking only `adjust` lets a bundled update to another container through unevaluated. However, a `ContainerUpdate` entry only carries a container ID and resource values, not a namespace; the request's own `pod` field describes the container being created, not the other container referenced in `update[]`. The plugin therefore cannot resolve which namespace's policy applies to a bundled update without becoming a container-lifecycle tracker itself (e.g. building a container-to-namespace map from NRI's `Synchronize` call and ongoing container events), which is out of scope for a plugin that registers only `ValidateContainerAdjustment`. For v1, the plugin logs that an unattributable bundled update occurred (visible for troubleshooting) but does not apply namespace-scoped `mutations` policy to it, since `ContainerUpdate` can only ever carry a `Resources` change and that gap is already documented as a residual risk above. Properly enforcing policy on these updates would require NRI itself to include the target container's pod/namespace information in `update[]` entries; that is a candidate upstream NRI feature request, not something this plugin can close unilaterally.
- CRI-O's `nri_validator_required_plugins` setting fails closed if the validator plugin is not connected, rejecting container creation instead of allowing it unchecked. In both Dev Preview and Tech Preview the validator is started directly by CRI-O from `nri_plugin_dir` rather than created as a Kubernetes container, so it is never itself blocked by this requirement, and no pod ever needs a toleration annotation to bootstrap around it: the validator is already connected before CRI-O starts accepting any `CreateContainer` call, Kubernetes-scheduled or otherwise. See **Risks and Mitigations**.

### Risks and Mitigations

- The NRI policy plugin adds a new component to the container startup path. Because `ValidateContainerAdjustment` can only approve or reject the merged adjustment as a whole, enforcement is all-or-nothing per container: a misconfigured `Strict` policy can reject every container in a namespace, including namespaces an administrator didn't intend to restrict as tightly.
- Mitigation: namespaces with no matching policy entry (including no `["*"]` catch-all) are never evaluated by the hook and always pass through untouched, so unlisted namespaces (e.g. `openshift-dns`, `openshift-monitoring`) are never blocked by omission. This is the intended behavior in every mode, including `Strict`: `Strict` only changes what happens for a namespace that *does* match an entry, it does not change whether a namespace matches one in the first place. (An earlier prototype build rejected unmatched namespaces under `Strict` mode; that was a bug in that build, not the intended design, and is fixed.) For namespaces that do match a policy, the plugin supports a `mode` field: `Permissive` logs the violation and approves the container unchanged, letting administrators observe the effect of a policy before switching the namespace to `Strict`, which rejects the container outright and surfaces a `CreateContainerError` so the failure is immediately visible.

- Deploying the `allow-mutations` validator as a Kubernetes DaemonSet pod, as an earlier design did, creates a node-reboot deadlock: `nri_validator_required_plugins` blocks every container, Kubernetes-scheduled or not, until the validator connects, but validators only run on `CreateContainer` and don't gate pod sandbox creation, so after a reboot every container is blocked, including the CNI (`ovnkube-node`, `multus`) the validator pod itself needs for its own pod network, and, on a control-plane node, `etcd`/`kube-apiserver`. The validator pod can't become `Ready` without CNI, CNI can't start without the validator, and the stuck node counts against the DaemonSet rollout's `maxUnavailable`, stalling the very first rollout; on SNO this blocks the API server itself. Mitigation: the validator is launched directly by CRI-O from `nri_plugin_dir` in both Dev Preview and Tech Preview, never as a Kubernetes pod, so it is connected before CRI-O accepts any `CreateContainer` call at all, including the CNI's own. This removes the deadlock at its root rather than working around it (e.g. via `hostNetwork` on the validator pod, which would also reopen the host-networking exposure the per-plugin SCC mitigation above specifically closes).

- If the policy plugin is not running (crashed, restarting, or failed to start on a config error), CRI-O by default creates containers with no check at all: enforcement silently stops with no visible signal. Mitigation: the CRI-O drop-in sets `nri_validator_required_plugins`, so CRI-O refuses to create any container while the plugin is disconnected instead of failing open. This trades availability for safety: while the plugin is down, no new containers can start on that node, so plugin reliability becomes operationally important. The plugin does not exit on a bad config (see Dev Preview delivery), which removes the most common way it would otherwise go down. A genuine crash (e.g. a panic) can still happen, and CRI-O's `nri_plugin_dir` auto-discovery does not restart a plugin that exits; recovering requires restarting CRI-O on that node (e.g. `systemctl restart crio`) or rebooting it, since the plugin only relaunches as part of CRI-O's own startup. Administrators should alert on CRI-O logging that a required plugin is disconnected, since that is the observable signal that this recovery step is needed.

- `resources` policy is only enforced at container creation time; it cannot catch resource changes made after a container is running. NRI only exposes a validation hook on `CreateContainer`; there is no equivalent hook for `UpdateContainer` or for updates a plugin pushes on its own initiative, so a change made through Kubernetes in-place pod resize, the static CPU manager, or a resource-policy plugin like `balloons` rebalancing containers after a config change bypasses the `mutations` policy entirely, with no way for any NRI validator to reject or even observe it. The same gap also covers updates bundled into another container's `CreateContainer` request (see Enforcement point): the plugin sees that one occurred and logs it, but cannot resolve its target's namespace, so it is never policy-enforced either. This is a limitation of the current NRI hook set, not something this plugin can close in v1. Mitigation: the enhancement documents this gap explicitly rather than implying `resources` policy is enforced for the lifetime of a container; administrators deploying resource-policy plugins alongside namespace-scoped `resources` restrictions should be aware that only the resource values present at container creation are checked.


### Drawbacks

 TBD

### Enforcement point

The plugin registers with NRI under the plugin name `allow-mutations`. This is the name CRI-O matches against for the required-plugin check (`nri_validator_required_plugins`): the setting must reference `allow-mutations`, the registered NRI plugin name, not the binary filename `10-allow-mutations` (CRI-O also accepts that form for a plugin loaded via `nri_plugin_dir` auto-discovery, since it matches both the registered name and the `index-name` form, but this doc uses the plain registered name consistently in both delivery paths to avoid confusion). The validator is never deployed as a Kubernetes pod in either delivery path, so there is no separate DaemonSet resource name to confuse this with.

The plugin hooks into `ValidateContainerAdjustment`, which runs after CRI-O merges adjustments from all NRI plugins at container creation time. At this stage the full combined `adjust` for the container being created is visible, so the plugin can evaluate its mutations against the creating container's namespace policy. Any bundled `update` for other, already-running containers is visible too, but only to the extent of logging that it occurred: a `ContainerUpdate` carries no namespace information, so the plugin cannot resolve which namespace's policy would apply and does not enforce `mutations` policy against it (see Implementation Details/Notes/Constraints and the `resources` risk below).

This is not the complete set of mutations a container can ever receive, however. `ValidateContainerAdjustment` only fires on `CreateContainer`; it is not invoked for resource changes made later via `UpdateContainer` (for example `crictl update`, Kubernetes in-place pod resize, or the static CPU manager), nor for updates a plugin pushes on its own initiative outside of a create/update request (for example a resource-policy plugin like `balloons` rebalancing other containers after a config change). These paths can only change a container's Linux resources (env, mounts, hooks, annotations, and devices cannot change after creation), but resource changes made through them are not evaluated against the `mutations` policy at all. See **Risks and Mitigations**.

### Policy schema (v1)

The on-disk file and any future CR **spec** share the same shape so plugin behavior is identical regardless of delivery path (file-based or Operator-managed). `policies[]` is evaluated in order; first matching entry wins.

#### Field reference

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.policies[].namespaces` | []string | yes | Exact namespace names this policy entry covers. Use the single entry `["*"]` to match every namespace not matched by an earlier entry; this lets an administrator add a default-deny catch-all as the last entry in `policies[]`. |
| `spec.policies[].mutations.type` | string | yes | One of `AllowAll`, `AllowList`, `DenyAll`. A discriminated union instead of a plain list with a magic `"all"` value, so `type: AllowAll` with a populated `allowList` can't be expressed. |
| `spec.policies[].mutations.allowList` | []string | only when `type: AllowList` | Mutation categories permitted for matching namespaces. Setting `allowList` when `type` is `AllowAll` or `DenyAll`, and setting `type: AllowList` with `allowList` missing or empty, are both treated the same as an unrecognized field: the plugin fails to load the config and logs an error. An empty `allowList` under `AllowList` is rejected rather than silently meaning "nothing allowed," since that would just be a second, confusing way to spell `DenyAll` (see the Tech Preview CRD section for the equivalent CEL-enforced, admission-time rejection). |
| `spec.policies[].mode` | string | no | One of `Strict` or `Permissive`. Defaults to `Permissive` if omitted. `Strict` rejects a container carrying a disallowed mutation; `Permissive` logs the violation and approves the container unchanged. Scoped per policy entry, not per file or CR, so different namespaces can use different enforcement strength (e.g. `Strict` for `production`, `Permissive` while tuning a new policy for another namespace). |

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

This tolerance applies only to unrecognized *values* within a known field, not to unrecognized *fields* in the config's structure. A config file or CR containing a field the plugin doesn't know about at all (e.g. a stale `namespaceSelector.matchNames` from before the `namespaces` rename, or a typo'd field name) is loud, logged as an error, rather than silently parsed as an empty policy that matches nothing and lets every namespace pass through unenforced; whether that means refusing to start (Dev Preview) or reporting `Degraded` (Tech Preview) at the point it's encountered, versus keeping the last-known-good policy, follows the same initial-startup-vs-reload distinction described in Implementation Details/Notes/Constraints.

#### Full example

```yaml
spec:
  policies:
    - namespaces: ["kube-system", "openshift-monitoring"]
      mutations:
        type: AllowList
        allowList: [Env, Mounts, Resources]
      mode: Strict

    - namespaces: ["my-app"]
      mutations:
        type: AllowList
        allowList: [Env]
      mode: Permissive

    - namespaces: ["trusted-ns"]
      mutations:
        type: AllowAll

    - namespaces: ["*"]
      mutations:
        type: DenyAll
      mode: Permissive
```

The final `["*"]` entry is a default-deny catch-all: any namespace not matched by an earlier, more specific entry falls through to it instead of passing through unevaluated. Using `mode: Strict` here would reject every container in every namespace not explicitly listed elsewhere in the policy, including OpenShift system namespaces (`openshift-dns`, `openshift-monitoring`, etc.) that this example does not list. An administrator enabling a `Strict` catch-all must first explicitly allow-list (or exclude) every system namespace the cluster depends on; this example uses `mode: Permissive` on the catch-all precisely to avoid that footgun by default.

#### Evaluation logic

Namespace matching and mode/mutations evaluation are two separate steps: a namespace is first matched against `policies[]` (explicit entries, then `["*"]` if present), and only a namespace that matches some entry is then subject to that entry's `mutations`/`mode`. A namespace matching nothing at all, because no `["*"]` entry exists, is never evaluated, regardless of what `mode` or `mutations` values appear elsewhere in the file; this is the intended behavior, not merely the behavior when no catch-all is configured.

| Condition | Behavior |
|---|---|
| Namespace matches no entry, and no `["*"]` catch-all entry is configured | No hook, container runs with all mutations untouched |
| Namespace matches no specific entry, but a `["*"]` catch-all entry is configured | The catch-all entry's `mutations` and `mode` apply |
| Namespace matches policy, `mutations.type: AllowAll` | All mutations permitted |
| Namespace matches policy, `mutations.type: DenyAll` | All mutations disallowed: rejected (`mode: Strict`) or logged (`mode: Permissive`) |
| Namespace matches policy, `mutations.type: AllowList`, category in `allowList` | That mutation is permitted |
| Namespace matches policy, `mutations.type: AllowList`, category NOT in `allowList` | Disallowed: rejected (`mode: Strict`) or logged (`mode: Permissive`) |
| Namespace matches policy, mutated field is not one of the six named categories | Classified as `Other`; follows the same `AllowAll`/`AllowList`/`DenyAll` evaluation as any other category |

### Dev Preview: file-based delivery

- Ship the standalone NRI policy plugin and its policy file as node configuration. CRI-O enables NRI and loads the plugin through the normal NRI integration path.

- At validation time, the plugin evaluates the workload namespace against the configured rules (see **Policy schema (v1)**). The plugin watches the config file at its fixed canonical path and reloads it live on change, so a new policy takes effect without restarting the plugin process. An `OPENSHIFT_UNSUPPORTED_ALLOW_MUTATIONS_CONFIG` environment variable overrides the path for testing only.

- The plugin's behavior on an invalid config depends on whether it already has a working policy loaded, and is deliberately different in each case:
  - On a bad **reload** (the plugin is already running with a previously, successfully loaded policy), it logs the error and keeps enforcing that last-known-good policy without exiting. Enforcement continuity is preserved and nothing is silently bypassed, which is what makes the node disruption policy of `None` (see below) safe: MCO doesn't even restart the plugin after writing a new file, so the plugin itself must never let a bad write take it down.
  - On a bad config at **initial startup** (no policy has ever successfully loaded in this process), the plugin logs the error and **exits**, rather than running with an empty policy. Running with an empty policy would leave the validator connected, satisfying `nri_validator_required_plugins`, while enforcing nothing at all, a silent policy bypass with no visible signal, which is exactly the failure mode `nri_validator_required_plugins` exists to prevent in the first place. Exiting instead makes this failure loud and immediate, like any other reason the validator isn't running: CRI-O refuses to create containers on that node (see **Risks and Mitigations**) until the config is fixed and CRI-O is restarted (`systemctl restart crio`), using the same recovery procedure and the same observable "required plugin disconnected" CRI-O log line already documented there for a genuine crash. `nri_plugin_dir` auto-discovery not restarting an exited plugin is an acceptable consequence here, not something to avoid: the loud failure is the intended outcome.

- Policy is read from the single canonical path: `/etc/crio/nri_plugins/AllowMutations/config.yaml`.

- Changes ship via the Machine Config Operator: a `MachineConfig` labeled with the `worker` role uses Ignition `storage.files` to install the following. A `MachineConfig` carries a single `machineconfiguration.openshift.io/role` label, and a `MachineConfigPool` only picks up configs its own `machineConfigSelector` matches; the `worker`-labeled `MachineConfig` reaches the `worker` pool and any custom worker-derived pool (e.g. `worker-cnf`) whose selector matches the `worker` role by convention, with no separate `MachineConfig` needed for those. On SNO and compact (3-node) clusters, where the node(s) belong to the `master` pool instead, a second `MachineConfig` labeled with the `master` role is additionally required; see **Topology Considerations**. The `worker`-labeled MachineConfig installs:
  1. a one-shot systemd unit, `nri-allow-mutations-extract.service`, that pulls the plugin's container image by digest and extracts the `allow-mutations` binary onto the host filesystem at `/etc/crio/nri_plugins/10-allow-mutations`, the NRI plugin auto-discovery directory configured in step 4 below. The statically linked plugin binary does not fit directly in a `MachineConfig`: MCO caps a rendered `MachineConfig` at 1.5 MiB, and exceeding it fails rendering for the *entire* `MachineConfigPool`, not just this one file, so the binary cannot be inlined via Ignition `storage.files` the way the policy file and drop-in are. Shipping it as a container image instead reuses the already-mature, already-tested image-pull path (including `ImageDigestMirrorSet` support for disconnected clusters) rather than Ignition's size-capped inline content. The unit uses `podman create`/`podman cp`/`podman rm` to copy the binary out of a stopped container without ever running the image's entrypoint:

     ```ini
     [Unit]
     Description=Install the allow-mutations NRI plugin binary from its container image
     Before=crio.service

     [Service]
     Type=oneshot
     RemainAfterExit=yes
     ExecStartPre=-/usr/bin/podman rm -f nri-allow-mutations-extract
     ExecStart=/usr/bin/podman create --name nri-allow-mutations-extract registry.example.com/nri-allow-mutations@sha256:<digest>
     ExecStart=/usr/bin/podman cp nri-allow-mutations-extract:/usr/local/bin/allow-mutations /etc/crio/nri_plugins/10-allow-mutations
     ExecStart=/usr/bin/podman rm nri-allow-mutations-extract
     ExecStartPost=/usr/bin/chmod 0755 /etc/crio/nri_plugins/10-allow-mutations
     ExecStop=/usr/bin/rm -f /etc/crio/nri_plugins/10-allow-mutations

     [Install]
     WantedBy=multi-user.target
     ```

     This is a one-time extraction, not a running container: the plugin still executes as a normal host process launched directly by CRI-O, preserving the exact startup-ordering guarantee this design depends on (a persistently running, self-connecting container would reintroduce the race this proposal already avoided by using `nri_plugin_dir` auto-discovery). Upgrading the plugin means changing the image digest in this unit, which is itself a `MachineConfig` content change and triggers the normal rollout, the same cost as any other binary upgrade. The extracted binary itself is a side effect of this unit running, not a file Ignition declares directly, so `ExecStop` removes it; this matters because Ignition reconciliation stops and disables a systemd unit it previously managed when it is no longer part of a newly rendered `MachineConfig`, which would otherwise leave the extracted binary orphaned on disk after this `MachineConfig` is removed or reverted;
  2. a `crio.service` drop-in, `/etc/systemd/system/crio.service.d/10-nri-allow-mutations-extract.conf`, declaring `Requires=nri-allow-mutations-extract.service` and `After=nri-allow-mutations-extract.service`. `Before=` on the extraction unit alone only orders the two units *if* both end up in the same boot transaction; it does not make `crio.service` pull the extraction unit in as a dependency. This drop-in is what guarantees the extraction unit actually runs, and completes, before CRI-O starts:

     ```ini
     [Unit]
     Requires=nri-allow-mutations-extract.service
     After=nri-allow-mutations-extract.service
     ```
  3. the policy file at `/etc/crio/nri_plugins/AllowMutations/config.yaml`;
  4. a CRI-O drop-in at `/etc/crio/crio.conf.d/10-nri-allow-mutations.conf` that enables NRI, sets `nri_plugin_dir = "/etc/crio/nri_plugins"`, and configures the complete default-validator stanza needed for fail-closed behavior. CRI-O loads drop-ins from `/etc/crio/crio.conf.d/` in lexical filename order, with later files overriding earlier ones on conflicting keys; naming this file explicitly makes its precedence relative to any other drop-in on the node deterministic instead of incidental:

     ```toml
     [crio.nri.default_validator]
     nri_enable_default_validator = true
     nri_validator_required_plugins = ["allow-mutations"]
     ```

     Both keys are required together: `nri_enable_default_validator` defaults to `false`, and without setting it to `true`, CRI-O's default-validator subsystem is never active, so `nri_validator_required_plugins` is never read or enforced at all, regardless of its value. `nri_validator_required_plugins = ["allow-mutations"]` (the registered NRI plugin name, see Enforcement point) is what makes CRI-O refuse to create containers if the validator isn't connected, instead of silently creating them unchecked. No `nri_validator_tolerate_missing_plugins_annotation` setting is needed: that key exists to let a specific Kubernetes pod bypass the requirement, but the validator here is never a pod (see Risks and Mitigations for why deploying it as one would deadlock a node reboot), so nothing ever needs to be exempted. The explicit `nri_plugin_dir` is separately required: upstream CRI-O's default plugin directory is `/opt/nri/plugins`, and OpenShift's MCO-shipped CRI-O config does not set `nri_plugin_dir` to override it, so without this setting CRI-O would never discover a binary placed under `/etc/crio/nri_plugins`.

- The cluster-scoped `MachineConfiguration` object (not the `MachineConfig` itself) declares an [admin-defined node disruption policy](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/admin-defined-node-disruption-policy.md) at `spec.nodeDisruptionPolicy`, mapping the policy file path (`/etc/crio/nri_plugins/AllowMutations/config.yaml`) to action `None`, since the plugin reloads that file live. Policy-only changes are therefore applied by MCO without draining or rebooting the node. Changes to the plugin binary or the CRI-O drop-in still require a full MachineConfig rollout (drain and reboot), since those do require the plugin process or CRI-O to restart. Because `None` means MCO does not even restart the plugin after writing the file, correctness depends entirely on the plugin's reload behavior keeping the last-known-good policy without going down on a bad write; see the reload-vs-startup invalid config behavior described above (a `None`-triggered update is always a reload, never an initial startup, so the plugin never exits in response to one).

#### Pros

- Policy is enforced from the very first container on a fresh node; it works before the Kubernetes API server is up.
- No additional in-cluster components; smaller attack surface and no new failure domain.
- Reuses the existing MCO/Ignition delivery path already proven for all other node configuration in OpenShift.
- Policy-only changes apply without a node reboot, via a node disruption policy of `None` on the policy file combined with the plugin's live config reload.

#### Cons

- Changes to the plugin binary or CRI-O drop-in still require a MachineConfig rollout: nodes are drained and rebooted, taking 20-45 minutes per cluster and disrupting running workloads. Policy-only changes avoid this via the node disruption policy above.
- No live status reporting; there is no CR condition to observe whether the policy is in sync on each node.

### Tech Preview: Operator-managed delivery

**Scope note:** this enhancement covers only the `allow-mutations` validator and its namespace-scoped mutation policy. An earlier draft of this Tech Preview section also specified a general-purpose `NRIPlugin` CRD for deploying *arbitrary* NRI plugins (e.g. `balloons`) as operator-managed DaemonSets. That capability is out of scope here and is being tracked as a separate, future enhancement: deploying arbitrary plugins as Kubernetes-scheduled DaemonSets is a materially different problem (dynamic per-plugin SCC creation, a shadow DaemonSet-like API, its own payload-vs-OLM and API-group questions) than delivering and policing this one specific validator, and bundling both into a single proposal made this document's scope wider than its own PR description claims. The **NRI Plugins Operator** referenced below, for this enhancement, manages only the `allow-mutations` validator's delivery and its singleton `NRIMutationPolicy` CR.

For Tech Preview, the Dev Preview file-based delivery, where an administrator hand-edits a `MachineConfig`, is replaced by the **NRI Plugins Operator** ([`nri-plugins-operator`](https://github.com/amritansh1502/nri-plugins-operator)) managing that same `MachineConfig` content on the administrator's behalf. Installing the operator is sufficient to get the validator running: there is no plugin-selection CR to create, since this operator manages exactly one plugin. An administrator optionally creates the singleton `NRIMutationPolicy` CR (named `cluster`) to define namespace-scoped mutation policy; with none created, the validator runs with an empty policy, the same pass-through-everything behavior as an empty `policies[]` in Dev Preview.

The validator is launched directly by CRI-O from `nri_plugin_dir`, exactly as in Dev Preview, delivered via an operator-managed `MachineConfig` rather than a DaemonSet pod. This is a deliberate, load-bearing design choice, not an incidental one: deploying the validator as a Kubernetes pod would deadlock a node reboot. `nri_validator_required_plugins` blocks every container, Kubernetes-scheduled or not, until the validator connects, but `CreateContainer` validators don't gate pod sandbox creation, so the validator's own pod would need CNI (`ovnkube-node`, `multus`) to become `Ready`, and CNI's containers would themselves be blocked by the same fail-closed gate the validator is supposed to satisfy. See **Risks and Mitigations** for the full failure mode. Launching the validator outside the Kubernetes pod lifecycle in both delivery paths avoids this entirely.

NRI itself is already enabled in CRI-O by default on OpenShift worker nodes (confirmed via `crio config` on a live cluster), so the operator's `MachineConfig` only needs to carry the validator's binary, policy file, and CRI-O drop-in, not a separate NRI-enablement step.

#### NRIMutationPolicy CRD

The operator introduces a second cluster-scoped CRD (`nri.openshift.io/v1alpha1`, kind `NRIMutationPolicy`) for namespace-scoped mutation policy. Mutation policy is managed separately from plugin deployment, so administrators can change policy without touching plugin CRs.

`NRIMutationPolicy` is a **singleton**: exactly one instance is permitted cluster-wide, and it must be named `cluster`, enforced by CRD validation (a CEL rule rejecting any other `metadata.name`). The policy evaluation model (first-match-wins across `policies[]`, a single `["*"]` catch-all) is defined for one ordered list; allowing multiple separate CR instances would leave the order and precedence across CRs, and the behavior of two CRs each defining their own catch-all, undefined. A singleton makes that class of question structurally impossible instead of needing a merge algorithm.

| Field | Type | Required | Description |
|---|---|---|---|
| `spec.policies[].namespaces` | []string | yes | Exact namespace names this policy entry covers. Use `["*"]` as the last entry for a default-deny catch-all. |
| `spec.policies[].mutations.type` | string | yes | One of `AllowAll`, `AllowList`, `DenyAll`. Same semantics as Policy schema (v1). |
| `spec.policies[].mutations.allowList` | []string | only when `type: AllowList` | Mutation categories permitted for matching namespaces. Same values as Policy schema (v1). Enforced by CRD validation: CEL rules reject a CR at admission time if `allowList` is set while `type` isn't `AllowList`, or if `type` is `AllowList` with `allowList` missing or empty (that would just be a second spelling of `DenyAll`), and an enum constrains `allowList` to recognized category values. |
| `spec.policies[].mode` | string | no | One of `Strict` or `Permissive`. Same semantics as Policy schema (v1), scoped per policy entry. |

The operator watches the singleton `NRIMutationPolicy` CR, marshals its `spec.policies[]`, and reconciles it into the same shared MachineConfig that delivers the validator binary, at the same canonical path the validator watches (`/etc/crio/nri_plugins/AllowMutations/config.yaml`), under the same `None`-action node disruption policy Dev Preview uses. The validator (an already-running, CRI-O-launched process, not a pod) reloads it live, so a policy change takes effect without a node reboot and without any pod restart, since there is no pod in the loop to restart. This also avoids the outage window a DaemonSet rolling restart would otherwise introduce under a fail-closed node: if the validator were a pod, restarting it would mean a period where the old pod is gone and the new one hasn't registered yet, during which CRI-O would reject every container on that node. Because the validator is never a pod here, that window doesn't exist.

Status reports standard Kubernetes `conditions` only: type `Reconciled` (`True`/`False`) for whether the spec was successfully marshaled and written into the shared MachineConfig, with a `reason` describing failures (e.g. invalid policy). This is deliberately named `Reconciled`, not `Active`: the operator has no feedback channel from the validator process running on each node, so it cannot confirm the policy was actually parsed and applied there, only that it successfully wrote the content the validator is expected to read. A future iteration could add a plugin-side signal (e.g. per-node status surfaced back to the operator) to support a condition that genuinely means "confirmed applied," but that does not exist for v1. No `phase` or `policyCount` field: `phase` is a deprecated pattern in current API conventions, and a policy count is trivially derivable from `len(spec.policies)` and does not belong in status.

##### NRIMutationPolicy example

```yaml
apiVersion: nri.openshift.io/v1alpha1
kind: NRIMutationPolicy
metadata:
  name: cluster
spec:
  policies:
    - namespaces: ["production", "prod-workloads"]
      mutations:
        type: AllowList
        allowList: [Env, Annotations]
      mode: Strict
    - namespaces: ["trusted-ns"]
      mutations:
        type: AllowAll
```

#### API type definitions

```go
// NRIMutationPolicySpec defines the desired mutation policy.
type NRIMutationPolicySpec struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=256
	// +listType=atomic
	Policies []PolicyEntry `json:"policies"`
}

// PolicyEntry is a single namespace-scoped mutation policy.
type PolicyEntry struct {
	// Namespaces lists exact namespace names this entry covers.
	// Use ["*"] as the last entry for a default-deny catch-all.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +listType=set
	Namespaces []string `json:"namespaces"`
	// +kubebuilder:validation:Required
	Mutations MutationPolicy `json:"mutations"`
	// Mode defaults to Permissive if unset.
	// +kubebuilder:validation:Enum=Strict;Permissive
	// +optional
	Mode Mode `json:"mode,omitempty"`
}

// Mode controls enforcement strength for a policy entry.
type Mode string

const (
	ModeStrict     Mode = "Strict"
	ModePermissive Mode = "Permissive"
)

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

// MutationCategory is one of the named mutation categories, or Other for
// any mutated field not covered by the named ones.
// +kubebuilder:validation:Enum=Env;Mounts;Annotations;Resources;Hooks;Devices;Other
type MutationCategory string

const (
	MutationCategoryEnv         MutationCategory = "Env"
	MutationCategoryMounts      MutationCategory = "Mounts"
	MutationCategoryAnnotations MutationCategory = "Annotations"
	MutationCategoryResources   MutationCategory = "Resources"
	MutationCategoryHooks       MutationCategory = "Hooks"
	MutationCategoryDevices     MutationCategory = "Devices"
	MutationCategoryOther       MutationCategory = "Other"
)

// MutationPolicy selects which mutation categories are permitted.
// +kubebuilder:validation:XValidation:rule="self.type == 'AllowList' || !has(self.allowList)",message="allowList may only be set when type is AllowList"
// +kubebuilder:validation:XValidation:rule="self.type != 'AllowList' || (has(self.allowList) && size(self.allowList) > 0)",message="allowList must be non-empty when type is AllowList"
type MutationPolicy struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=AllowAll;AllowList;DenyAll
	Type MutationType `json:"type"`
	// AllowList is only meaningful, and only accepted by the CEL rules above, when Type is AllowList.
	// +kubebuilder:validation:MaxItems=7
	// +listType=set
	// +optional
	AllowList []MutationCategory `json:"allowList,omitempty"`
}

// NRIMutationPolicyStatus defines the observed state of the mutation policy.
type NRIMutationPolicyStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NRIMutationPolicy is the Schema for the nrimutationpolicies API.
// A singleton: exactly one instance is permitted, and it must be named "cluster".
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message="NRIMutationPolicy is a singleton and must be named 'cluster'"
type NRIMutationPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NRIMutationPolicySpec   `json:"spec,omitempty"`
	Status NRIMutationPolicyStatus `json:"status,omitempty"`
}
```

#### Reconciliation overview

On startup, and on any change relevant to the steps below, the operator reconciles in this order:

1. Checks for a pre-existing Dev Preview file-based installation (the `allow-mutations` binary and CRI-O drop-in at their documented paths) on targeted nodes. If found, the operator does not proceed to step 2; it reports its own status as `Degraded`, naming the conflicting Dev Preview installation as the reason, since installing its own `MachineConfig` alongside an existing one would let two `allow-mutations` processes register under the same NRI plugin name and index simultaneously (NRI does not reject this). See **Upgrade / Downgrade Strategy** for the required migration order.
2. Creates or updates a shared `MachineConfig` (`99-nri-validator-required`) labeled with the `worker` role, installing the `allow-mutations` binary, its policy file, and the CRI-O drop-in, exactly as described in Dev Preview delivery, including the `[crio.nri.default_validator]` stanza. This single, `worker`-labeled `MachineConfig` reaches the `worker` pool and any custom worker-derived pool (e.g. `worker-cnf`) whose `machineConfigSelector` matches the `worker` role by convention; no per-pool enumeration is needed. On SNO/compact clusters, the operator additionally creates a second `MachineConfig` labeled with the `master` role, since those nodes belong to the `master` pool instead (see **Topology Considerations**). The validator is launched by CRI-O itself from `nri_plugin_dir`, never as a Kubernetes pod; deploying it as a DaemonSet instead would deadlock a node reboot (see **Risks and Mitigations**). NRI itself is already enabled by default in CRI-O on OpenShift worker nodes (confirmed via `crio config` on a live cluster), so this MachineConfig only needs to carry the binary, policy file, and drop-in, not a separate NRI-enablement step. This step does not apply on HyperShift; see **Topology Considerations**.
3. Waits for the `worker` pool, and the `master` pool where a second `MachineConfig` was created for it, to finish updating. Waiting on `worker` alone on an SNO/compact cluster would complete immediately against a pool with no relevant rollout and falsely report success while the node is never actually configured.
4. Watches the singleton `NRIMutationPolicy` CR and reconciles its `spec.policies[]` into the same MachineConfig's policy file, at the policy file path, under the `None`-action node disruption policy described in Dev Preview delivery. The already-running validator reloads it live; there is no pod to restart, since the validator is never one.
5. Updates operator-level status (conditions, readyNodes, desiredNodes) from the MachineConfigPool's rollout state.

Deleting the operator, or an operator-level resource that opts out of validator installation (exact mechanism TBD, since there is no longer a per-plugin CR to delete to trigger this), removes the shared `99-nri-validator-required` MachineConfig. This removes the `allow-mutations` binary and reverts CRI-O's `nri_validator_required_plugins` setting, restoring fail-open behavior; it does not disable NRI itself, which remains enabled by default regardless of this MachineConfig's presence. Because the binary and the `nri_validator_required_plugins` setting are delivered by, and removed by, the same `MachineConfig`, a given node always reverts both together in its own single reboot; there is no window on any one node where the fail-closed setting is active with the binary already gone, or vice versa. This was a deliberate reason to deliver both through one `MachineConfig` rather than separate mechanisms. Removing this `MachineConfig` is itself a drain-and-reboot rollout, the same cost as installing it; see **Upgrade / Downgrade Strategy** for the uninstall procedure, including the OLM-specific gap where uninstalling the operator does not, by itself, trigger this cleanup at all.

#### Pros

- No node reboot for policy changes.
- Live status reporting via CR status and conditions.
- Policy management decoupled from the validator's own delivery: administrators manage the `NRIMutationPolicy` CR without touching `MachineConfig` directly.

#### Cons

- Operator adds a new in-cluster component with its own failure domain.
- Installing the operator still triggers a MachineConfig rollout (one-time node reboot to install the `allow-mutations` binary, policy file, and CRI-O drop-in; see Reconciliation overview step 1).

## Alternatives (Not Implemented)

### CRI-O's built-in default validator

CRI-O already ships a built-in policy mechanism, the default validator (`[crio.nri.default_validator]`), which can block `hooks`, `seccomp`, `namespace`, and `sysctl` changes and require specific plugins to be running (the basis for this enhancement's `nri_validator_required_plugins` fail-closed setting). It was considered as the sole enforcement mechanism but is not sufficient on its own: it applies one set of rules **node-wide**, with no concept of the Kubernetes namespace a workload belongs to. It cannot express "namespace A gets a different policy than namespace B," which is the core requirement here. The standalone plugin model is still needed to add that namespace-scoped layer on top of, not instead of, the default validator.

Embedding the full namespace-scoped policy logic directly in the CRI-O daemon (beyond what the default validator already offers) was also considered but rejected: it couples policy management to CRI-O release cycles, increases daemon complexity, and prevents independent updates to the policy rules. The standalone plugin model keeps policy outside the daemon.

### Per-plugin rules instead of per-namespace rules

Rather than scoping policy by the workload's Kubernetes namespace, rules could instead be scoped by plugin identity, for example, "only plugin X may change `resources`." NRI's adjustment request carries `owners` data identifying which plugin set each field, which is enough information to express such a rule. This was not adopted for v1 because the stated requirement is controlling which *workloads* (by namespace) may receive which mutation categories, regardless of which plugin produced them; per-plugin rules answer a different question ("which plugin may mutate what") and would need to compose with, not replace, namespace-scoped policy. It may be worth revisiting as an additional, orthogonal policy dimension in a future iteration.

### A general-purpose `NRIPlugin` CRD for arbitrary plugin deployment

An earlier draft of this enhancement also specified an `NRIPlugin` CRD letting administrators deploy *any* NRI plugin (e.g. a resource-policy plugin like `balloons`) as an operator-managed DaemonSet, with per-plugin SCC creation, image-digest pinning, and node targeting. This was removed from this enhancement's scope: it is a materially different capability (arbitrary Kubernetes-scheduled plugin deployment, not the delivery of this one validator), raises its own unresolved design questions (a prior review discussion concluded that an operator-managed DaemonSet-equivalent API is not a sustainable long-term pattern), and bundling it here made this document's actual scope wider than this enhancement's own stated scope. Generic NRI plugin deployment is tracked as a separate, future enhancement.

## Open Questions

- **Payload vs. OLM delivery for Tech Preview.** This doc currently states both that the `NRIMutationPolicy` CRD is "gated by the `TechPreviewNoUpgrade` feature set" and that the operator is "available via OLM with a stable channel" at GA. These describe two different, mutually exclusive delivery models:
  - *Payload component*: requires a named `FeatureGate` in `openshift/api`, a `ClusterOperator` reporting status, and likely a `Capability` declaration, since this isn't a core cluster feature. `TechPreviewNoUpgrade` gating only applies to this model.
  - *OLM-delivered operator*: distributed via a catalog with a `Subscription` channel (e.g. a `tech-preview` channel distinct from `stable`), independent of the OpenShift release cycle. `TechPreviewNoUpgrade` does not apply here; the `TechPreviewNoUpgrade` references in this doc would need to be removed and replaced with a defined Tech Preview channel. Choosing this model also means the `nri.openshift.io` API group name, which conventionally implies formal `openshift/api` review, needs reconsidering.

  This choice also decides who builds and publishes the plugin and operator images, currently hosted in personal repositories, and how CVE fixes and NRI library bumps reach them: ART owns payload-component builds, Konflux owns OLM-delivered operator/catalog builds. This needs to be resolved, and the repositories moved to an appropriate org, before this enhancement can graduate past Dev Preview.
- **Operator uninstall/opt-out mechanism.** Deleting the singleton `NRIMutationPolicy` CR is the documented trigger for removing the shared `99-nri-validator-required` MachineConfig (see Upgrade / Downgrade Strategy), with a manual ordering requirement in the meantime: delete the CR and wait for the MachineConfig removal to roll out before uninstalling the operator via OLM, since OLM uninstall itself runs no cleanup logic. Whether to add a finalizer to `NRIMutationPolicy` to enforce this automatically, rather than relying on a documented manual order, is not yet decided.

## Test Plan

### Test levels

- **Unit tests**, in the plugin and operator repositories respectively: policy evaluation logic (namespace matching, `mutations`/`mode` semantics, mutation-category detection), and operator reconciliation logic (resource creation, status updates, MachineConfigPool targeting). Run as part of each repository's own CI.
- **Integration tests against a real CRI-O build**, not a mocked NRI server: this proposal depends on subtle, easy-to-get-wrong CRI-O/NRI protocol behavior (`owners` data, bundled `update[]` entries, required-plugin fail-closed behavior), which a mock could misrepresent. These run against CRI-O built from the OpenShift fork, exercising the plugin as a real NRI client.
- **End-to-end tests in `openshift/origin`** (or the equivalent OpenShift conformance suite), covering the full Dev Preview MachineConfig delivery and the Tech Preview operator-managed delivery against a live cluster. Required before GA per this enhancement's graduation criteria.

### Environments

- CRI-O running alongside other NRI plugins (e.g. `balloons`) simultaneously, to verify policy enforcement and the `update[]` namespace-attribution limitation behave correctly when multiple plugins are active, not just the allow-mutations plugin in isolation.
- SNO, compact (3-node), and custom-pool (e.g. `worker-cnf`) topologies, to verify `MachineConfigPool` targeting resolves correctly in both delivery paths.
- Upgrade and downgrade scenarios: operator upgrade with an existing `NRIMutationPolicy` CR in place, and policy schema version skew (an older plugin build encountering a newer config's category names).

### Behaviors to verify

- Mutation-category detection uses NRI's `owners` data, not field presence: `Resources`, `Hooks`, and similar fields are always non-nil by default, so a presence check would produce false positives on every container.
- A plugin's response bundling an `update` for another, already-running container is detected (both `adjust` and `update` inspected), with the documented limitation that the plugin cannot attribute that update to a namespace and therefore does not enforce `mutations` policy on it.
- Fail-closed behavior while the validator plugin is disconnected (`nri_validator_required_plugins`), and that a node recovers via `systemctl restart crio` after a genuine crash or after an initial-startup config error (the plugin exits in that specific case, by design); and that a bad *reload* of an already-running plugin never exits and never loses the last-known-good policy.
- Policy rollout without a reboot or pod restart in both delivery paths, via the same node-disruption-policy-driven live file reload, since the validator is never a pod in either one.
- The validator is never deployed as a Kubernetes pod in Tech Preview: a node reboot with the fail-closed CRI-O config does not deadlock waiting on CNI/the validator's own pod readiness.
- The default-deny `["*"]` catch-all, per-entry `mode` (`Strict` rejects, `Permissive` logs and allows), and the guarantee that a namespace matching no entry at all always passes through untouched regardless of `mode` elsewhere in the policy.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Policy enforcement delivered via MachineConfig (Dev Preview) on worker nodes.
- Namespace scoped mutation enforcement (reject/log via `mode`) functional and tested on an OpenShift cluster.
- Unit test suite passing with no data races.
- Enhancement doc reviewed and merged.
- NRI Plugins Operator deployed and functional on OpenShift cluster.
- `NRIMutationPolicy` CRD registered, gated by `TechPreviewNoUpgrade` feature set (or a defined Tech Preview OLM channel, pending the Open Questions resolution).
- Operator reconciles `allow-mutations` delivery end-to-end: MachineConfig creation, MCO rollout wait, policy reconciliation, status reporting.
- allow-mutations policy delivered via `NRIMutationPolicy` CR reconciled into the node-level policy file, no node reboot for policy changes.
- `allow-mutations` validator delivered via operator-managed MachineConfig, launched directly by CRI-O, confirmed not to deadlock a node reboot (no dependency on CNI or any pod readiness).
- Uninstall/cleanup path verified (removes the `99-nri-validator-required` MachineConfig).

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

- *Policy change*: update the `NRIMutationPolicy` CR spec. The operator reconciles it into the shared MachineConfig's policy file; the validator, already running and launched by CRI-O, reloads it live, no pod restart or node reboot required.
- *allow-mutations binary/CRI-O config upgrade*: the operator updates the shared `99-nri-validator-required` MachineConfig's binary/drop-in content, which triggers the normal MachineConfig rollout (drain and reboot), the same cost as any Dev Preview binary upgrade.
- *Operator upgrade*: standard Deployment rollout. The new controller reconciles the existing `NRIMutationPolicy` CR. The shared MachineConfig (`99-nri-validator-required`) persists across operator upgrades; it does not change between versions unless the validator binary itself is upgraded.
- *Downgrade*: revert the operator Deployment image. The controller reconciles existing CRs. If the CRD schema changed between versions, the administrator must ensure CR compatibility before downgrading.
- *Uninstall*: removing the shared `99-nri-validator-required` MachineConfig removes the `allow-mutations` binary and reverts CRI-O's `nri_validator_required_plugins` setting (restoring fail-open behavior); it does not disable NRI itself, which remains enabled by default regardless of this MachineConfig's presence. This is itself a drain-and-reboot rollout on every affected node, the same cost as installing it, not an instantaneous change. **Uninstalling via OLM does not trigger this on its own**: OLM removes the operator's Deployment and controller, but runs no cleanup logic and does not delete the `NRIMutationPolicy` CR, so nothing is left running to react to a deletion and remove the MachineConfig; left alone, it would persist on the cluster indefinitely after the operator is gone. Until a finalizer-based mechanism exists (tracked in Open Questions), uninstalling requires this order: delete the `NRIMutationPolicy` CR first, wait for the operator to remove the `99-nri-validator-required` MachineConfig and for that rollout to finish on every affected pool, and only then uninstall the operator itself via OLM. Doing this out of order risks the reverse of the earlier startup deadlock: removing the validator before the `MachineConfig` revert has rolled out everywhere would leave nodes still enforcing `nri_validator_required_plugins` with nothing left to satisfy it.

**Dev Preview -> Tech Preview migration**

Dev Preview's hand-authored `MachineConfig` and the Tech Preview operator's `99-nri-validator-required` `MachineConfig` both install an `allow-mutations` binary and CRI-O drop-in at the same conventional paths, and both register the plugin under the same NRI plugin name and index. NRI does not reject a second plugin registering with the same name and index: if both configs were ever present on a node at once, even transiently, two separate `allow-mutations` processes would run simultaneously, potentially enforcing different policies with no defined precedence between them. This must never happen, so migration is an **ordered removal, not a direct swap**:

1. Revert or remove the Dev Preview `MachineConfig`. This triggers its own drain-and-reboot rollout, which fully removes the old binary, policy file, and CRI-O drop-in from the node before anything else is installed.
2. Only after that rollout completes on every affected node, install the NRI Plugins Operator, which creates its own `99-nri-validator-required` MachineConfig and triggers a second, separate rollout.

As a defense beyond relying on an administrator to follow this order correctly, the operator checks for the Dev Preview file-based installation (the binary and CRI-O drop-in at their known paths) on reconciliation. If found, the operator does not create its own `MachineConfig` on top of it; it reports its own status as `Degraded`, with a reason identifying the conflicting Dev Preview installation, until the administrator removes it.

## Version Skew Strategy

TBD

## Operational Aspects of API Extensions

TBD

## Support Procedures

TBD

## Infrastructure Needed [optional]

- New repository: `github.com/amritansh1502/nri-allow-mutations` hosts the standalone NRI policy plugin's Go source.
- New container image, built from that repository and published to a registry, referenced by digest from the node's `nri-allow-mutations-extract.service` unit in both delivery paths (see Dev Preview delivery). The binary itself ships inside this image, not as inline `MachineConfig` content.
- New repository: `github.com/amritansh1502/nri-plugins-operator` hosts the NRI Plugins Operator, scoped in this enhancement to delivering the `allow-mutations` validator and reconciling the singleton `NRIMutationPolicy` CR.

