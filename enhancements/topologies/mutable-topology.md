---
title: mutable-topology
authors:
  - "@jeff-roche"
  - "@jaypoulz"
  - "@eggfoobar"
reviewers:
  - "@tjungblu, for cluster-etcd-operator"
  - "@dusk125, for cluster-etcd-operator"
  - "@joelspeed, for API, infrastructure config, and cluster-config-operator scope"
  - "@patrickdillon, for OpenShift installer"
  - "@zaneb, for metal platform interaction"
  - "@ardaguclu, for OpenShift client"
  - "@atiratree, for OpenShift client"
  - "@jerpeter, for OpenShift architecture"
  - "@sdodson, for architecture"
  - "@dgoodwin, for architecture"
approvers:
  - "@jerpeter, for OpenShift architecture"
api-approvers:
  - "@joelspeed, for API and infrastructure config"
creation-date: 2026-05-11
last-updated: 2026-09-16
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-2280
  - https://issues.redhat.com/browse/OCPEDGE-2640
replaces:
  - https://github.com/openshift/enhancements/pull/1905
superseded-by: []
---

# Mutable Topology

## Terms

**Topology Modes** — OpenShift supports several topology configurations. The `TopologyMode` enum defines the API values: `SingleReplica`, `HighlyAvailable`, `DualReplica`, and `HighlyAvailableArbiter`.
Beyond these enum values, OpenShift recognizes deployment shapes that use specific enum values with particular node configurations: compact clusters (control-plane nodes serve as workers), Two-Node with Arbiter (TNA — 2 control-plane nodes + 1 arbiter + workers, uses `HighlyAvailableArbiter`),
and Two-Node with Fencing (TNF — 2 schedulable control-plane nodes with STONITH, uses `DualReplica`).

This enhancement initially targets `controlPlaneTopology` transitions only (SingleReplica → HighlyAvailable). The broader topology landscape is acknowledged here because the architecture must not preclude future support for these additional configurations.

**Mutable Topology** — The capability for an OpenShift cluster to transition between topology modes as a Day 2 operation, removing the existing assumption that topologies are immutable after installation.

**Topology Transition** — A directed change from one topology mode to another (e.g., SingleReplica to HighlyAvailable). Transitions are managed by a controller in cluster-config-operator and follow a set of supported transitions.

**Control Plane Topology** — The cluster-topology mode describing how control-plane nodes are deployed and managed (SingleReplica, HighlyAvailable, or other supported modes). Control-plane nodes are nodes labeled with `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master`.

**Infrastructure Topology** — The cluster-topology mode describing how infrastructure workloads are distributed (SingleReplica, HighlyAvailable, or other supported modes). When there are no dedicated worker nodes, `infrastructureTopology` is set to match `controlPlaneTopology` since control-plane nodes serve as workers.

**Compact Cluster** — A cluster where control-plane nodes also serve as workers. In the initial SNO-to-HA transition, the target is a 3-node compact cluster with no dedicated worker nodes. The compact deployment shape is a consequence of not adding dedicated worker nodes — it is not a distinct `TopologyMode` enum value.

**mastersSchedulable** — A field in the infrastructure status indicating whether control-plane nodes are schedulable for general workloads. The topology transition controller recalculates this value as part of a transition.

**Cluster Administrator** — An entity responsible for managing an existing cluster, including Day 2 operations such as topology transitions and node scaling. This may be a human operator or an external orchestrator/agent.

## Summary

This enhancement introduces "mutable topology" which is defined as "the ability for OpenShift clusters to transition between topology modes as a Day 2 operation". This changes the existing OpenShift assumption that topologies are immutable after installation.

A new `controlPlaneTopology` field in the infrastructure spec expresses the administrator's intent to transition. A topology transition controller in cluster-config-operator watches for changes to this field, validates preconditions, coordinates the transition, and updates the existing topology status fields when the cluster is ready.
The proposed `status.controlPlaneTopologyTransitions` API lets administrators and automation discover applicable transitions and current blockers before requesting a change. CCO does not publish this field today.
A new `oc adm transition topology` CLI command provides an interface for cluster administrators to discover and initiate transitions.
The initial implementation supports transitioning Single Node OpenShift (SNO) clusters to HA compact (3-node) on `platform: none`.

This enhancement supersedes the [Adaptable Topology proposal](https://github.com/openshift/enhancements/pull/1905), which proposed a new `Adaptable` topology mode requiring changes across all core operators. That proposal is withdrawn in favor of this controller-based approach.

## Motivation

Cluster demands change over time. Customers who start with Single Node OpenShift (SNO) at edge locations may later require high availability as workloads become more critical. Today, this requires redeploying the cluster — a disruptive operation that involves workload migration, downtime, and operational overhead.

The previous approach to this problem ([Adaptable Topology](https://github.com/openshift/enhancements/pull/1905)) proposed a new `Adaptable` topology mode where operators would dynamically react to node count changes.
That approach required updating every core operator to handle dynamic topology shifts and introduced a new topology enum value that all operators had to understand. It also coupled topology behavior to node count, making operator logic more complex.

Mutable topology takes a different approach: instead of adding a new topology mode that operators must interpret, transitions are orchestrated by a controller in cluster-config-operator that coordinates the sequencing, validates preconditions, and updates the infrastructure CR only when the cluster is ready for the new mode.
Operators continue to react to the same fixed topology values they already understand. This keeps operator logic simple and concentrates transition complexity in an existing core component.

### User Stories

* As a cluster administrator running Single Node OpenShift (SNO) at an edge location, I want to add control-plane nodes to my cluster to achieve high availability so that I can handle node failures without service disruption as workloads become more critical.

* As a cluster administrator deploying OpenShift clusters at scale, I want to start with minimal footprint deployments that can grow into highly available clusters so that I can reduce initial costs while maintaining scalability.

* As a cluster administrator managing a fleet of edge deployments, I want a supported path to transition my cluster topology so that I don't need to redeploy clusters when my infrastructure requirements change.

* As a cluster administrator, I want topology transitions managed through a well-defined API so that I have a clear interface for monitoring transition state and integrating with my operational tooling.

* As a cluster administrator or fleet orchestrator, I want to discover which transitions apply to a cluster and why they are currently blocked so that I can prepare the cluster before requesting a topology change.

### Goals

* Officially support topology transitions in OpenShift
* Provide a supported interface for administrators to initiate topology transitions
* Expose applicable transitions and their current availability and diagnostic information to administrators and automation
* Support transitioning SNO clusters to HA compact (3-node) on `platform: none` as the initial transition path
* Maintain backward compatibility — existing clusters with fixed topology modes are unaffected
* Establish the architectural foundation for additional transition paths in the future

### Non-Goals

* Supporting all possible topology transitions in the initial implementation (only SNO → HA compact on `platform: none`)
* Supporting transitions for HyperShift or hosted control plane clusters
* Supporting transitions for MicroShift deployments
* Supporting transitions for Image Based Install (IBI) clusters
* Automatic node provisioning or deprovisioning based on workload demands
* Scaling down control-plane or worker nodes (scale-down may be addressed in a future enhancement)
* Supporting bidirectional transitions (e.g., HA → SNO) in the initial implementation

## Proposal

This enhancement introduces infrastructure API fields for transition intent and discovery, and a topology transition controller in cluster-config-operator (CCO; not to be confused with cloud-credential-operator) to enable topology transitions as Day 2 operations.

The approach follows the standard OpenShift spec/status contract and mirrors the pattern used by `oc adm upgrade`:

1. **`controlPlaneTopology` field in InfrastructureSpec** — Expresses the administrator's intent to transition. The CLI patches this field to initiate a transition. The existing `controlPlaneTopology` and `infrastructureTopology` fields in status continue to represent the cluster's observed topology.

2. **`controlPlaneTopologyTransitions` field in InfrastructureStatus** — The proposed API reports controller-computed transitions from the current control-plane topology, whether their preconditions currently pass, and diagnostic information when they do not. It is advisory discovery information; it does not request or admit a transition.

3. **Topology transition controller in cluster-config-operator** — The controller must evaluate transition availability before intent is set, publish it through the same API contract, watch the infrastructure CR for `controlPlaneTopology` spec changes, revalidate requested transitions, coordinate the transition, and update status topology fields when the cluster is ready for the new mode.

4. **`oc adm transition topology` CLI command** — A command that exposes controller-published transition availability, validates client-side preconditions, patches `spec.controlPlaneTopology` on the infrastructure CR to request a transition, and returns immediately after the patch.

The transition controller is proposed to live in cluster-config-operator because CCO is the canonical owner of the `config.openshift.io` API group and the Infrastructure CR.
The controller is feature-gated using the standard library-go FeatureGateAccess pattern: when the gate is disabled the controller is not registered with the manager and incurs negligible runtime overhead; a gate change triggers an operator restart via ForceExit so the new state is picked up cleanly.

See [Alternatives](#alternatives-not-implemented) for the full analysis of controller placement options.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Mutable topology is not compatible with HyperShift clusters. HyperShift uses `External` as its `controlPlaneTopology`, and topology transitions are not applicable to hosted control planes where the control plane lifecycle is managed externally.

Future support for HyperShift is not planned for this enhancement but is not ruled out.

#### Standalone Clusters

Standalone clusters are the primary target for mutable topology. This enhancement enables standalone clusters to start with minimal footprints and transition to multi-node configurations without redeployment.

`platform: none` is the only supported platform for the initial SNO → HA compact transition. This is a deliberate scope constraint:

- The primary customers for mutable topology are edge computing deployments, where SNO clusters are deployed with minimal footprint and need to scale to HA as workloads grow. Edge sites commonly use `platform: none`, making it the natural starting point for this enhancement.
- `platform: none` has no platform-managed infrastructure (no cloud load balancers, no keepalived, no CCM) — the administrator owns all external networking. This eliminates the need for the transition controller to interact with platform-specific infrastructure, keeping the initial implementation focused on the core topology transition mechanics.
- `platform: baremetal` requires keepalived-managed load balancing, which does not currently support single-node clusters. Adding SNO support to `platform: baremetal` is a prerequisite for baremetal topology transitions and is planned for a subsequent phase.
- Cloud platforms (AWS, Azure, GCP) require CCM and platform-specific load balancer integration during node scaling, which adds significant scope.

On `platform: none`, the administrator is responsible for external networking prerequisites (VIPs, DNS, load balancer configuration) as described in the [Pre-Transition](#pre-transition) workflow.

This design does not inhibit expansion to other platforms — the supported transitions list and precondition validation are per-transition, so platform-specific transitions can add their own checks without changing the controller architecture.

#### Single-node Deployments or MicroShift

Single Node OpenShift (SNO) clusters are the primary source topology for transitions. The initial use case is enabling SNO deployments to transition to HA compact (3-node) configurations as requirements change.

The topology transition controller is gated by the `MutableTopology` feature gate and has no resource impact on clusters that do not use this feature.

Image Based Install (IBI) clusters are out of scope for this enhancement. Whether IBI clusters can support topology transitions has not been evaluated.

MicroShift is not affected by this enhancement and is unlikely to be included as a supported transition target.

#### OpenShift Kubernetes Engine

This proposal does not depend on features excluded from the OpenShift Kubernetes Engine (OKE) product offering. Mutable topology modifies core infrastructure components — the infrastructure API, cluster-config-operator, cluster-etcd-operator, and other in-payload operators — all of which are included in OKE.

### Workflow Description

#### Transition: SNO to HA Compact (3-Node)

**Operational guidance**: Administrators should treat topology transitions as a maintenance window. Cluster availability is not guaranteed during the transition — particularly during the 2-member etcd window where any control-plane node failure is fatal.
Administrators should reduce non-critical workload risk accordingly. Administrators should take an etcd backup before and after a successful transition (see [Open Questions](#open-questions) regarding pre-transition backup compatibility).

##### Pre-Transition

Once the discovery producer is implemented, administrators can inspect `status.controlPlaneTopologyTransitions` on `infrastructure/cluster`, directly or through the CLI's discovery interface, to identify applicable transitions and current blockers.
A defined transition with unmet preconditions is reported as `Unavailable` with a reason, while `Unknown` indicates evaluation has not completed. An omitted or empty list does not distinguish evaluation pending from no applicable transitions.
Discovery does not modify cluster intent. Seeing `Available` does not guarantee admission: the cluster may change before a request is processed, and external networking prerequisites remain the administrator's responsibility.

1. The cluster administrator prepares exactly 2 additional control-plane nodes and joins them to the cluster — the kubelet is running on each node and Node objects exist in the Kubernetes API. On `platform: none`, the administrator manages their own load balancing configuration (VIPs, DNS).
2. **Node-driven operator reactions (prerequisite)** — independent of any topology intent, as soon as the new Node objects appear: cluster-etcd-operator (CEO) scales etcd members sequentially (1→2→3) via its existing unsafe/day-2 scaling path, reusing the learner-to-voter promotion mechanism from bootstrapping;
   the kube-apiserver, kube-controller-manager, and kube-scheduler operators render static pod manifests for the new nodes.
   This is existing, unmodified operator behavior — the topology transition controller does not trigger, sequence, or wait on it directly. It only observes the outcome (a healthy 3-member etcd, `Ready` control-plane nodes) as a precondition in step 7. This can complete before, during, or after step 3.
3. The cluster administrator runs `oc adm transition topology HighlyAvailable`
4. The CLI validates preconditions before patching (e.g., feature gate enabled, no transition already in progress)
5. The CLI patches the infrastructure CR: `spec.controlPlaneTopology: HighlyAvailable`
6. The API server validates the topology value and permitted direction using field-level enum and feature-gated CEL validation (see [Admission Control](#admission-control))

##### During Transition

7. The topology transition controller in CCO detects the `controlPlaneTopology` change and validates preconditions:
    - Every ClusterOperator other than `config-operator` itself reports `Available=True`, `Progressing=False`, `Degraded=False`
    - At least 3 nodes with `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master` labels are present, all schedulable, `Ready`, and worker-labeled
    - No dedicated worker nodes are present (the initial implementation targets compact clusters only; clusters with dedicated workers require a different `infrastructureTopology` mapping that is not yet supported)
    - etcd already reports quorum, is not mid-scaling, and already has 3 voting members — i.e., step 2's node-driven scaling has already finished
   If any precondition fails — including an etcd that has not yet finished scaling — the controller does not admit the transition; it records the reason and re-evaluates on the next sync (see [Failure Handling](#failure-handling)).
8. The current CCO controller verifies an upgrade has not already been triggered by CVO, then writes `TopologyTransitionControllerUpgradeable=False` and `TopologyTransitionControllerProgressing=True` to `operator.openshift.io/v1 Config/cluster` before updating Infrastructure status.
   These custom conditions are controller-local diagnostics; they do not set the standard `Upgradeable` condition on `ClusterOperator/config-operator` and therefore do not currently prevent CVO from initiating an upgrade. See [Upgrade Safety Status](#upgrade-safety-status).
9. The controller updates the infrastructure status fields:
   - `controlPlaneTopology` transitions from `SingleReplica` to `HighlyAvailable`
   - `infrastructureTopology` transitions from `SingleReplica` to `HighlyAvailable` (no dedicated workers, so it matches control plane topology)
   - When discovery is implemented, `controlPlaneTopologyTransitions` is cleared or replaced in the same status update so all remaining entries originate from the new `controlPlaneTopology`; initially there are no defined outgoing transitions from `HighlyAvailable`
10. **Topology-driven operator reactions** — operators that watch the infrastructure status topology fields reconcile against the new values and adjust their deployment strategies, replica counts, and placement policies.
    This is a distinct phase from step 2: step 2 covers operators reacting to node presence before the transition is even admitted, step 10 covers operators reacting to the topology status change after admission.
    The set of operators with topology-dependent behavior has not been fully enumerated — building the per-operator topology dependency matrix is a prerequisite for entering dev preview (see [Graduation Criteria](#entering-dev-preview) and [Open Questions](#open-questions)).

    **Note**: OLM-managed operators that read topology at startup rather than watching for changes may need to be restarted after the transition completes. See [Optional (OLM-Managed) Operators and Topology Changes](#optional-olm-managed-operators-and-topology-changes) for details.

##### Post-Transition

11. After a soak period (5 minutes) following the custom progressing condition, the controller checks that control-plane/worker node readiness, etcd health, MachineConfig rollout, ingress router replicas, and API server operator replica counts have reconciled to the target values
12. Once all checks pass, the controller clears the custom progressing condition and sets its custom upgradeable condition to `True`. The infrastructure status reflects the completed transition — `spec.controlPlaneTopology` matches `status.controlPlaneTopology`, so no further action is taken.

The CLI returns immediately after patching `spec.controlPlaneTopology` (step 5). Today, administrators inspect controller-specific transition conditions in `operator.openshift.io/v1 Config/cluster` and the aggregate `ClusterOperator/config-operator` status separately. The target monitoring UX will be defined during dev preview.

##### Failure Handling

The proposed discovery field provides availability diagnostics without starting a transition or setting transition-progress conditions merely because a candidate is blocked.
The controller must revalidate a request regardless of previously advertised availability. CCO conditions report request handling and transition progress; discovery is neither progress tracking nor transition history.

The controller recognizes two distinct failure windows, and makes no guarantees about the node-driven etcd scaling itself:

- **Before admission**: if a precondition never becomes true — for example etcd never finishes scaling to 3 voting members, or a control-plane node never becomes `Ready` — the controller simply never admits the transition.
  `spec.controlPlaneTopology` remains diverged from `status.controlPlaneTopology` indefinitely, and the custom transition conditions on `operator.openshift.io/v1 Config/cluster` carry a diagnostic reason (e.g. `PreflightCheckFailed`) that the administrator can inspect.
  Failures in the node-driven etcd scaling itself — including quorum loss in the 2-member window, which requires manual recovery via `quorum-restore.sh` — are cluster-etcd-operator's existing failure domain; the topology transition controller neither triggers nor is able to recover from them, since they occur before it admits the transition.
- **After admission**: if a post-transition validation criterion never passes (e.g., an operator fails to reconcile), the custom `TopologyTransitionControllerProgressing` condition remains `True` and the custom upgradeable condition remains `False` indefinitely. The administrator inspects CCO and the relevant operator's logs and status for details.
  `spec.controlPlaneTopology` remains unchanged and the controller re-evaluates reconciliation on every sync. The current controller has no safe cancellation behavior once this progressing condition is set.
  Clearing or resetting the spec can reach the completion path without selecting transition validators. Administrators must not use that path until explicit cancellation handling is implemented and tested.
  After the status fields have been updated, the transition is effectively complete and cannot be cancelled — the cluster is in the new topology. This follows the standard Kubernetes pattern where controllers continuously reconcile toward the desired state until the user changes intent.

### API Extensions

#### Infrastructure API Changes

This enhancement modifies the existing infrastructure CR (`infrastructures.config.openshift.io`) following the standard Kubernetes spec/status contract:

**Spec (user intent):**

A new `controlPlaneTopology` field is added to `InfrastructureSpec` to express the administrator's intent to transition:

```go
type InfrastructureSpec struct {
	CloudConfig  ConfigMapFileReference `json:"cloudConfig"`
	PlatformSpec PlatformSpec           `json:"platformSpec,omitempty"`
	// controlPlaneTopology expresses the desired control-plane topology.
	// Setting HighlyAvailable when status.controlPlaneTopology is
	// SingleReplica requests a transition, subject to controller validation.
	// When omitted or equal to status.controlPlaneTopology, no change
	// is requested. The field may be reset to match the observed topology.
	// +optional
	// +openshift:enable:FeatureGate=MutableTopology
	// +kubebuilder:validation:Enum=HighlyAvailable;SingleReplica
	ControlPlaneTopology TopologyMode `json:"controlPlaneTopology,omitempty"`
}
```

The field is empty by default — the installer does not populate it. An empty `spec.controlPlaneTopology` indicates no current transition intent. After a successful transition, the field remains set (e.g., `HighlyAvailable`) and matches `status.controlPlaneTopology` — the controller is idle.
The field is not transition history: administrators can clear it or set it to match the current topology without performing a transition. A transition is requested when the administrator sets `spec.controlPlaneTopology` to a permitted value that differs from `status.controlPlaneTopology`.

The API reuses `TopologyMode` with field-level enum validation restricting accepted values to `SingleReplica` and `HighlyAvailable`. Accepting a topology value does not imply support for every transition involving that value; direction is validated separately as described in [Admission Control](#admission-control).

**Mapping to status fields**: `spec.controlPlaneTopology` expresses intent for the control plane topology only. The controller derives the corresponding `infrastructureTopology` and `mastersSchedulable` values based on the transition definition.
For the initial SNO → HA compact transition: `controlPlaneTopology` and `infrastructureTopology` both transition to `HighlyAvailable` (no dedicated workers), and `mastersSchedulable` remains `true` (it is already `true` on SNO clusters since the single node runs all workloads; it stays `true` for compact clusters).

**Status (observed state):**

The existing fields in `InfrastructureStatus` that the controller updates upon successful transition:

```go
// controlPlaneTopology expresses the expectations for operands that normally
// run on control nodes. Currently documented as "set once by the installer
// and not expected to change." This enhancement changes that contract when
// the MutableTopology feature gate is enabled.
// +kubebuilder:default=HighlyAvailable
ControlPlaneTopology TopologyMode `json:"controlPlaneTopology"`

// infrastructureTopology expresses the expectations for infrastructure
// services that do not run on control plane nodes. When there are no
// dedicated worker nodes, this is set to match controlPlaneTopology.
// +kubebuilder:default=HighlyAvailable
InfrastructureTopology TopologyMode `json:"infrastructureTopology,omitempty"`
```

No new enum values are added to `TopologyMode`. The existing values (`SingleReplica`, `HighlyAvailable`, `DualReplica`, `HighlyAvailableArbiter`) are sufficient.

**Transition discovery (proposed observed availability):**

[openshift/api#3029](https://github.com/openshift/api/pull/3029) proposes an optional, `MutableTopology`-gated `InfrastructureStatus.ControlPlaneTopologyTransitions` field, serialized as `status.controlPlaneTopologyTransitions`.
The field is not yet merged or consumed by CCO. The current CCO follow-up instead references a different draft API field, `status.topologyTransitionStatus`, in [cluster-config-operator#499](https://github.com/openshift/cluster-config-operator/pull/499).
Before dev preview, CCO and oc must target the same merged API contract; this enhancement uses `controlPlaneTopologyTransitions` as the proposed contract below.

Each entry has the following API shape:

| Field | Type | Meaning and validation |
| ----- | ---- | ---------------------- |
| `source` | `TopologyMode` | Required. `SingleReplica` or `HighlyAvailable`; must equal the current `status.controlPlaneTopology`. |
| `target` | `TopologyMode` | Required. `SingleReplica` or `HighlyAvailable`. The schema's accepted values do not define the controller's supported transition paths. |
| `availability` | `TransitionAvailability` | Required. `Available` means evaluation completed and preconditions pass; `Unavailable` means a defined transition cannot currently be initiated; `Unknown` means evaluation has not completed. |
| `reason` | `string` | Required when availability is `Unavailable` or `Unknown`; optional and normally omitted when `Available`. A diagnostic, non-exhaustive machine-readable value matching `^[A-Z][A-Za-z0-9]*$`, between 1 and 128 characters. |
| `message` | `string` | Optional human-readable detail, primarily for `Unavailable` entries. When present, between 1 and 2048 characters; the controller may truncate it. Consumers must not parse it. |

The list has map semantics keyed by `(source, target)`, with no significant ordering and no duplicate pairs. It permits zero to two entries, matching the target enum's cardinality for one current source.
The intended producer behavior is to publish defined transitions from the current topology and retain a defined transition with failing preconditions as `Unavailable` rather than omitting it.
The initial transition graph contains only `SingleReplica` → `HighlyAvailable` on `platform: none`.

API PR #3029 requires `source` and `target` to differ with CEL validation and integration coverage, so same-source/target pairs are rejected.

An omitted field and an explicitly empty list intentionally carry the same meaning. Consumers cannot distinguish "not yet evaluated" from "evaluated with no applicable transitions" using this field alone.
When the current topology is outside the source/target enum, the list is omitted or empty. Future support for `DualReplica` or `HighlyAvailableArbiter` requires widening the field enums and increasing the list bound to match the target cardinality, up to four for the non-`External` topology values.
`External` is not expected to participate in transitions.

The following Infrastructure status excerpts illustrate the intended independent evaluations of the initial transition. Reason values are illustrative diagnostics, not an exhaustive reason enum.

Preconditions pass:

```yaml
status:
  controlPlaneTopology: SingleReplica
  controlPlaneTopologyTransitions:
  - source: SingleReplica
    target: HighlyAvailable
    availability: Available
```

A precondition blocks initiation:

```yaml
status:
  controlPlaneTopology: SingleReplica
  controlPlaneTopologyTransitions:
  - source: SingleReplica
    target: HighlyAvailable
    availability: Unavailable
    reason: PreflightCheckFailed
    message: "etcd requires 3 voting members for this transition; currently 1"
```

Evaluation has not completed:

```yaml
status:
  controlPlaneTopology: SingleReplica
  controlPlaneTopologyTransitions:
  - source: SingleReplica
    target: HighlyAvailable
    availability: Unknown
    reason: EvaluationPending
```

This flat shape represents a recomputed availability snapshot, rather than per-entry `metav1.Condition` objects. It has no evaluation timestamp or transition history.
`Available` is advisory: the controller must revalidate any request against current cluster state, and consumers must not treat a status read as an admission or completion guarantee.
Transitions continue to be requested exclusively through `spec.controlPlaneTopology`.

**Current transition progress implementation:**

The merged CCO controller writes the following custom conditions to the status of `operator.openshift.io/v1 Config/cluster`:

| Condition Type | Meaning |
| -------------- | ------- |
| `TopologyTransitionControllerProgressing` | A transition has been admitted and post-transition validation has not yet passed. `status: True` while awaiting downstream reconciliation, `status: False` when idle, rejected, or complete. |
| `TopologyTransitionControllerUpgradeable` | Controller-local upgradeability state. `status: False` while a transition is requested, pending, or in progress; `status: True` when idle or complete. It is not the CVO-consumed condition. |

`ClusterOperator/config-operator` exposes the canonical aggregate `Progressing` and `Upgradeable` conditions. CVO consumes the canonical `Upgradeable` condition, not `TopologyTransitionControllerUpgradeable`.
The current controller does not write the canonical condition, so it does not yet provide the documented upgrade block. The custom conditions distinguish states with reasons such as `UnsupportedTransition`, `PreflightCheckFailed`, `TopologyTransitionInProgress`, `TopologyTransitionComplete`, and `AsExpected`; aggregated ClusterOperator reasons/messages are not guaranteed to retain these values unchanged.
There is no separate condition for etcd scaling — that scaling is a precondition the controller checks, not a state it tracks or reports on directly (see [Failure Handling](#failure-handling)).

#### Admission Control

**Spec validation**: Field-level enum validation restricts `spec.controlPlaneTopology` to `SingleReplica` and `HighlyAvailable` when set. A `MutableTopology`-gated CEL rule additionally allows omitting or clearing the field, retaining its existing value, setting it to match `status.controlPlaneTopology`, and requesting `HighlyAvailable` when the observed topology is `SingleReplica`.
Other new transition directions are rejected by the API server. Dynamic preconditions are evaluated by CCO.

The current API allows clearing intent at any state. Before dev preview, the API and controller must implement the state-dependent cancellation contract in [Required State and Upgrade Protocol](#required-state-and-upgrade-protocol). In particular, clearing intent after admission must not be accepted as successful transition completion.

**Intent wire operations:** An omitted YAML field and a JSON Patch `remove` operation on `/spec/controlPlaneTopology` both clear the field. An explicit empty string (`controlPlaneTopology: ""`) is invalid and must not be used.
For a Pending request, the supported cancellation operation is a JSON Patch `replace` that sets `/spec/controlPlaneTopology` to the current `status.controlPlaneTopology`; the CLI must first read the Infrastructure resource and use a JSON Patch `test` for the request's current target to detect conflicts.
The API and CLI must reject clear, remove, or target changes after admission or while Progressing. The CLI must expose this as a distinct cancellation operation rather than treating an empty target as a cancellation request.

Access to `spec.controlPlaneTopology` is governed by the existing RBAC for the infrastructure CR (`infrastructures.config.openshift.io`). By default, only users with `cluster-admin` or equivalent roles can modify infrastructure spec fields.
No additional RBAC restrictions are proposed for the initial implementation; a dedicated role for topology transitions may be considered in future iterations if finer-grained access control is needed.

**Status fields**: The existing topology status fields (`controlPlaneTopology`, `infrastructureTopology`, `mastersSchedulable`) are not protected by admission policies. This is consistent with other infrastructure status fields — no special protection exists for them today. An administrator who deliberately modifies these values outside the transition controller does so at their own risk.

#### Feature Gate

The existing `MutableTopology` feature gate gates the spec field and controller registration. It is currently registered for self-managed clusters in `DevPreviewNoUpgrade`.
API PR #3029 also gates discovery fields and their cross-field validation with `MutableTopology`.
The feature gate will progress through the following stages:

- **Dev Preview**: Part of the `DevPreviewNoUpgrade` feature set
- **Tech Preview**: Moved to the `TechPreviewNoUpgrade` feature set
- **GA**: Moved to the `Default` feature set

### Implementation Details/Notes/Constraints

#### Topology Transition Controller

The merged topology transition controller in cluster-config-operator has the following characteristics:

- Watches the infrastructure CR for `spec.controlPlaneTopology` diverging from `status.controlPlaneTopology`
- Gated by the `MutableTopology` feature gate — inactive when the gate is disabled
- Maintains the set of supported transitions (initially only SingleReplica → HighlyAvailable on `platform: none`)
- Validates preconditions before starting a transition
- Updates `controlPlaneTopology` and `infrastructureTopology` in status once preconditions pass
- Reports transition progress through custom conditions in `operator.openshift.io/v1 Config/cluster`

##### Proposed Availability Evaluation

The discovery producer is not implemented in the merged CCO controller. The future controller must evaluate defined transitions originating at the current observed control-plane topology and publish availability and diagnostics through the merged API contract, including periodic refresh without a spec change.
It must share transition definitions and precondition checks with request handling so discovery and execution have the same source of truth.
Publishing a blocked candidate must not initiate a transition or block upgrades.

Discovery is observed state, not a reservation or admission approval. Once intent is set, the controller revalidates the requested transition even if it was previously advertised as `Available`.
When updating observed topology, the controller must clear or replace discovery entries in the same status update to satisfy source/status validation. After the status becomes `HighlyAvailable`, the list is omitted or empty until a defined outgoing transition exists.

While a request is Pending, Admitted, or Progressing, the entry for its source and target remains present with `availability: Unavailable`, `reason: TransitionInProgress`, and a message that names the active request state. This replaces any previous `Available` snapshot until observed topology changes or the request is withdrawn.
The CLI must reject a duplicate request whenever spec differs from observed topology or the controller reports Progressing, regardless of the discovery entry. It must not treat `TransitionInProgress` as a new available transition.

Because the API does not distinguish omitted from empty and has no observation time, automation cannot determine data freshness. Before dev preview, the selected discovery API must add an evaluation timestamp and an observed Infrastructure resource version for the list.
CCO refreshes this marker on its one-minute resync. The CLI accepts discovery for automation only when it is no more than two minutes old and `ClusterOperator/config-operator` is Available and not Degraded; otherwise it reports stale or unavailable discovery and exits nonzero.
The CLI must define stable output and exit behavior for omitted, empty, `Unknown`, `Unavailable`, stale, `TransitionInProgress`, and `Available` data.

##### Current CCO Follow-Up Blockers

Before relying on the CCO follow-up as this enhancement's implementation, it must fail closed in these cases:

- If `MutableTopology` is absent from the feature-gate state, treat it as disabled rather than calling `Enabled` with an unregistered key.
- If reading operator state fails while validating rendered MachineConfigs, return that error and fail validation. Only a genuinely absent or zero transition condition may use the permissive fallback.

##### Upgrade Safety Status

The current controller checks whether CVO has already started an upgrade before it begins a transition, then writes its custom conditions before its Infrastructure status update.
It does not currently write `Upgradeable=False` to `ClusterOperator/config-operator`, the condition CVO uses to decide whether upgrades are permitted.
Therefore, it does not establish mutual exclusion between upgrades and topology transitions today.

Before dev preview, the controller must write the canonical `Upgradeable=False` condition while a request is pending or a transition is in progress, and restore it to the appropriate idle state after cancellation, rejection, or completion.
There is no current CVO acknowledgement API for a persisted `ClusterOperator` condition. A persisted `Upgradeable=False` alone does not prove CVO has observed it, and an unrelated `ClusterVersion` `Upgradeable=False` is not an acknowledgement.
Before dev preview, this enhancement requires a CVO-supported acknowledgement containing the Infrastructure resource version and requested target. CCO waits up to five minutes for the matching acknowledgement before changing Infrastructure status. On timeout, it leaves status unchanged, restores the idle condition, records a retryable diagnostic, and retries from Pending with backoff.
The implementation must protect the sequence against an upgrade or precondition change between preflight and the Infrastructure status update, and must retry status conflicts by re-reading and revalidating the complete sequence.

##### Required State and Upgrade Protocol

The current controller does not implement this protocol. It is required before dev preview so upgrade safety and cancellation do not depend on timing between independent API writes.

1. **Idle**: No transition is requested. `spec.controlPlaneTopology` is omitted or matches observed topology. The canonical `ClusterOperator/config-operator` `Upgradeable` condition is not blocked by mutable topology.
2. **Pending**: A permitted spec value differs from observed topology, but CCO has not admitted it. CCO evaluates preconditions. A user can withdraw this request by resetting spec to observed topology; CCO clears pending diagnostics without updating topology status.
3. **Admitted**: CCO writes canonical `Upgradeable=False` with a topology-transition reason and waits up to five minutes for the CVO acknowledgement that contains the Infrastructure resource version and requested target.
   On acknowledgement timeout, it restores idle conditions and retries from Pending with backoff. CCO then re-reads Infrastructure and ClusterVersion, confirms intent is unchanged, reruns all dynamic preconditions, and performs a resource-version-guarded Infrastructure status update.
4. **Progressing**: The observed topology has changed and post-transition validators run after the soak period. Intent changes, including clear, must not select an empty validator set or re-enable upgrades. There is no user cancellation after this state begins.
5. **Failure handling**: On an admission failure, an intent change before the status commit, a status-update conflict, or a status-write failure, CCO re-reads current state and restarts the complete admission sequence. If no transition was committed, it restores the documented idle conditions. It must not restore upgradeability merely because no transition descriptor matches the current spec.

The implementation and tests must cover an upgrade starting during admission, condition-write failure, acknowledgement timeout, precondition loss after the first check, a condition update that succeeds before the status update fails, a conflict retry after intent changes, and clear/reset behavior before and after the topology status commit.

##### Supported Transitions

For the initial implementation:

```text
SingleReplica (SNO, platform: none) → HighlyAvailable (3-node compact)
```

Future transitions can be added without modifying the core controller logic. Each supported transition defines:

- **Preconditions**: What must be true before the transition can start
- **Orchestration steps**: What the controller coordinates during the transition
- **Validation criteria**: What must be true after the transition for it to be considered complete

##### Transition Orchestration

The controller reconciles `spec.controlPlaneTopology` against `status.controlPlaneTopology` on every sync. For the SNO → HA compact transition:

**Preconditions** (all must hold before a transition is accepted):

- Every ClusterOperator other than `config-operator` itself reports `Available=True`, `Progressing=False`, `Degraded=False`
- At least 3 control-plane-labeled nodes exist, all schedulable, `Ready`, and worker-labeled, and no dedicated worker nodes are present
- etcd has quorum and is not mid-scaling, and 3 voting members are already recorded for it

These preconditions mean the administrator's node join and CEO's existing node-driven etcd scaling must already be complete — the controller does not itself trigger or wait on etcd scaling as part of the transition; it only accepts the transition once that has already happened.

**Orchestration steps** (once preconditions pass):

1. The current controller sets its custom upgradeable and progressing conditions on `operator.openshift.io/v1 Config/cluster` in the same update; this is not yet a CVO upgrade block
2. Re-read the Infrastructure CR and confirm the requested spec has not changed since preconditions were checked
3. Update `status.controlPlaneTopology` and `status.infrastructureTopology` to the target value
4. On later syncs, wait a soak period (5 minutes) after the `Progressing` condition was set, then begin checking post-transition validation criteria

If no supported transition matches, or a precondition fails, the controller records the reason on its custom conditions rather than erroring. With no active progressing condition, setting spec to match status withdraws the request and clears the custom conditions. Cancellation after progressing is set is not safely implemented yet.

**Validation criteria** (checked once the soak period has elapsed; all must pass to consider the transition complete):

- 3 control-plane nodes remain schedulable and `Ready`, and a minimum of 2 worker-labeled nodes, including dual-role compact nodes, are `Ready`.
- etcd retains quorum, is not mid-scaling, and reports 3 voting members
- The SNO-only master MachineConfig has been removed and a new rendered master/worker MachineConfigs exist, with the master MachineConfigPool reporting 3 ready machines
- The default IngressController reports a minimum of 2 available router replicas
- kube-apiserver reports status for 3 nodes and openshift-apiserver reports 3 ready replicas

Once all criteria pass, the controller clears its custom progressing condition and sets its custom upgradeable condition to `True`.

#### `oc adm transition topology` CLI Command

The CLI command provides an interface for topology transitions:

- Once implemented, reads `status.controlPlaneTopologyTransitions` to expose applicable transitions, their current availability, and diagnostic reasons/messages
- Validates preconditions client-side (feature gate enabled, no transition in progress)
- Patches `spec.controlPlaneTopology` on the infrastructure CR
- Returns immediately after a successful patch

The CLI does not contain transition logic — it delegates entirely to the CCO controller. This follows the same pattern as `oc adm upgrade`, which patches `spec.desiredUpdate` and lets the CVO do the work.
Discovery must not patch the Infrastructure CR. The CLI must report `Unknown` as incomplete evaluation and handle absent or empty discovery data without claiming that transitions are definitively unsupported.
An `Available` entry does not bypass request-time validation. Exact discovery command syntax, stable output, and exit behavior must be defined before dev preview.
Administrators monitor current controller-specific progress in `operator.openshift.io/v1 Config/cluster`; the aggregate operator status is `oc get clusteroperator config-operator -o yaml`. A dedicated `oc adm transition topology status` subcommand remains to be defined during dev preview.

#### etcd Scaling: SNO to HA Compact

When transitioning from SNO to a 3-node compact cluster, CEO scales etcd members sequentially. Each new member joins as a learner and is promoted to a voting member using the same learner-to-voter promotion mechanism that CEO uses during cluster bootstrapping.

The overall orchestration differs from bootstrapping: bootstrapping uses a temporary bootstrap member that is later removed before the cluster reaches steady state, while a Day 2 transition adds permanent members to a running production cluster. Critically, the 2-voter intermediate state (steps 4–5 below) is unique to Day 2 transitions — it does not occur during bootstrapping.

1. **Starting state**: 1 etcd voting member (quorum=1)
2. CEO adds an etcd learner on the second control-plane node
3. The learner syncs data from the existing voter via data replication
4. CEO promotes the learner to a voting member — the cluster now has 2 voting members (quorum=2)
5. CEO adds an etcd learner on the third control-plane node
6. The learner syncs data from an existing voter
7. CEO promotes the learner to a voting member — the cluster now has 3 voting members (quorum=2)
8. The cluster can now tolerate the loss of one control-plane node

During the 2-member state (steps 4–5), the cluster has zero fault tolerance for control-plane node failures — losing either member is fatal.

This is a sequential process. The 2-member state in steps 4–5 is the primary risk window — quorum requires both members, so losing either is fatal. This window is minimized by proceeding to step 5 immediately after promotion.

The learner-to-voter promotion code path is well-exercised from cluster bootstrapping. However, the 2-member steady state is unique to Day 2 transitions — during bootstrapping, the temporary bootstrap member is removed before the cluster reaches steady state, so the cluster never operates with exactly 2 voting members handling production traffic.
The blast radius of a failure during the 2-member window is higher than during initial installation because this is a production cluster with live workloads.

#### Component Changes Summary

| Component | Changes Required |
| --------- | ---------------- |
| cluster-config-operator | Topology transition controller; current code watches `spec.controlPlaneTopology`, coordinates the initial transition, updates status topology fields, and reports custom controller conditions; it must add discovery publication against the selected API contract |
| Infrastructure API (`openshift/api`) | `spec.controlPlaneTopology` uses `TopologyMode` with field-level enum and permitted-direction validation; API PR #3029 proposes gated `status.controlPlaneTopologyTransitions` |
| `oc` CLI | New `oc adm transition topology` command; it must consume the selected discovery API contract |
| cluster-etcd-operator | No code changes — its existing node-driven (unsafe) etcd scaling behavior is depended on as a precondition the transition controller checks for, rather than something it triggers or orchestrates |
| ingress, networking, monitoring operators | Reconcile on infrastructure status topology field changes |

#### Platform Support Constraints

See [Standalone Clusters](#standalone-clusters) for platform support details. The initial implementation targets `platform: none` only; `platform: baremetal` and cloud platforms are future work.

The topology transition controller checks for Node objects in the API regardless of how they were provisioned.

### Risks and Mitigations

#### Risk: Quorum Loss During Two-Member Transient State

**Risk**: During sequential etcd scaling (1→2→3), the cluster passes through a 2-member state where quorum=2. Losing either member during this window is fatal — the cluster loses its API and requires manual recovery.

**Mitigation**:
- The 2-member state is transient and the learner-to-voter promotion mechanism is reused from cluster bootstrapping — a well-exercised code path
- Learner instances are used before promoting members to minimize the promotion window
- No availability guarantee during transitions; administrators should treat scaling operations as a maintenance window
- If etcd scaling fails during the 2-member window, quorum is lost and manual recovery via `quorum-restore.sh` is required
- Future iterations may explore admitting two learners simultaneously and promoting only when both are ready, eliminating the 2-member voting window entirely, but that is out of scope for this enhancement

#### Risk: Transition Fails Partway Through

**Risk**: A transition may fail after some operators have begun reconfiguring but before the transition completes, leaving the cluster in an intermediate state. Examples of failures:

- **etcd quorum loss**: during the node-driven prerequisite scaling (before the controller admits the transition), etcd scales to 2 members, a network partition occurs between them, both lose quorum, and the API becomes unavailable. This requires manual recovery via `quorum-restore.sh`, independent of the topology transition controller.
- **Node readiness**: a new control-plane node becomes `NotReady` during the transition (e.g., disk pressure, network misconfiguration), preventing etcd or static pods from starting.
- **Operator reconciliation failure**: after topology status fields are updated, an operator fails to reconcile (e.g., ingress pod fails to schedule on a new node due to resource constraints or `ImagePullBackOff`).

**Mitigation**:
- The controller only admits a transition once its preconditions — including etcd already having quorum and 3 voting members — are satisfied; it does not itself trigger or sequence etcd scaling
- Operators do not see a topology change until the controller updates the infrastructure status
- Etcd scaling failures (including quorum loss) are cluster-etcd-operator's existing failure domain; the transition controller withholds admission but provides no additional recovery guarantees for them. Quorum loss requires manual recovery via `quorum-restore.sh`
- CCO controller-specific conditions on `operator.openshift.io/v1 Config/cluster` provide detailed state for troubleshooting precondition and post-admission failures

#### Risk: Platform Bare Metal May Not Support Single-Node Clusters (Future Scope)

**Risk**: `platform: baremetal` is not in scope for the initial implementation, but is planned for a subsequent phase. If keepalived networking cannot be configured for single-node clusters, `platform: baremetal` will not support SNO → HA transitions, limiting mutable topology to `platform: none` for the foreseeable future.

**Mitigation**:
- Early coordination with the Bare Metal Networking team to assess feasibility
- `platform: none` provides full support as the initial path
- The limitation can be documented while bare metal support is resolved

#### Risk: Cannot Validate External Requirements

**Risk**: On `platform: none`, the topology transition controller cannot validate external requirements such as correct load balancer configuration or DNS setup. An administrator may initiate a transition with misconfigured networking, leading to a partially functional cluster.

**Mitigation**:
- Pre-flight checks validate what is within the cluster's control (node presence, resource requirements, operator health)
- External requirements (VIPs, DNS, load balancer configuration) are documented as the administrator's responsibility
- The CLI can surface warnings about external prerequisites before patching the infrastructure CR

### Drawbacks

#### Coordination Across Teams

The SNO-to-HA transition requires coordination with CEO, ingress, networking, and other operator teams to ensure they reconcile correctly when topology status fields change. This is less coordination than the previous Adaptable Topology approach (which required every operator to handle dynamic node-count awareness), but still significant.

#### Optional (OLM-Managed) Operators and Topology Changes

OLM-managed (optional) operators that read topology values at startup (rather than watching the infrastructure CR for changes) will not automatically react to topology transitions. These operators will need to either be updated to watch the infrastructure CR for topology changes, or be restarted after a transition completes. The scope of affected operators needs investigation.

Tracking an explicit list of which OLM operators need to be restarted per transition does not scale as a long-term design: every new transition path would require updating that list (or every operator would need to support every transition from the start to future-proof itself),
and if scale-down transitions ever become supported, an operator that isn't watching for topology changes could become unstable or degraded without warning.
The more maintainable fix is for operators to watch the infrastructure CR and react to topology changes themselves, rather than being tracked and restarted by this enhancement.

**Scope for this enhancement**: pre-GA, this feature's guarantees apply to in-payload (core) operators only. There is currently no mechanism for an operator to declare which topologies it supports today. Operators are optimistically assumed to support all topologies, similar to how architecture and OS compatibility are declared today.
Defining an OLM manifest mechanism for operators to declare topology support (and any catalog requirements built on it) would let the transition controller and catalog tooling reason about optional-operator compatibility with confidence instead of assuming it.
Designing and implementing that mechanism, and any transition-blocking behavior based on it, is out of scope for this enhancement and is left for a dedicated future enhancement — see [Open Questions](#open-questions).

Before this feature reaches GA, a plan (not necessarily a full implementation) for improving the UX around optional operators during a topology transition — e.g., surfacing which installed OLM operators have unknown or unsupported topology compatibility — must be defined. Actually detecting, blocking, or remediating transitions based on optional-operator topology support remains future work.

#### One-Way Transitions (Initially)

The initial implementation supports only SNO → HA compact. Reverse transitions (HA → SNO) and other paths are future work. Administrators who transition cannot revert without redeploying. Mechanisms will be put in place to gate the transition path at every level of the implementation (CLI, CCO Controller, API).

## Alternatives (Not Implemented)

### Adaptable Topology (Previous Proposal)

The [Adaptable Topology proposal](https://github.com/openshift/enhancements/pull/1905) introduced a new `Adaptable` enum value for `controlPlaneTopology` and `infrastructureTopology`. Operators would dynamically react to node count changes and adjust behavior accordingly.

**Why it was replaced**:
- Required updating core operators that read topology values to understand the new `Adaptable` enum and handle dynamic node-count-based behavior
- Coupled topology behavior to node count, making operator logic more complex
- Required shared library-go utilities that every operator team needed to adopt
- The `Adaptable` enum value created a paradigm that was fundamentally different from existing fixed topology modes

Mutable topology achieves the same end goal (SNO clusters can grow to HA) with less operator-side complexity. Operators continue to react to the same fixed topology values they already understand. Transition complexity is concentrated in a single controller rather than distributed across all operators.

### CLI-Only Transition Runner

An alternative is to embed all transition logic in the `oc adm transition` command without a dedicated operator.

**Why it was rejected**:
- The set of supported topologies is bounded, so the transition graph stays small. However, each transition is a long-running, multi-step process — etcd scaling alone takes minutes.
- A CLI process cannot provide persistent state tracking. A dropped SSH session or terminal close would leave the cluster in an intermediate state with no automated recovery.
- Error recovery and retry logic is better suited to a controller's reconciliation loop than imperative CLI code
- The CLI would need direct access to operator internals, violating separation of concerns

### Dedicated Topology Transition Operator

An earlier revision of this enhancement proposed a standalone topology transition operator deployed on-demand (not installed by default). The operator would own a transition CRD, manage the transition graph, and orchestrate the full transition lifecycle independently.

**Why it was rejected**:
- The scope does not warrant a new operator — cluster-config-operator is the natural home for this logic since it already owns the `config.openshift.io` API group and infrastructure CR lifecycle
- A standalone operator adds payload size, requires its own upgrade/lifecycle management, and introduces another component to monitor
- The transition controller can live in CCO with near-zero overhead when not in use, gated by the `MutableTopology` feature gate

### Extending Another Core Operator

Rather than adding the transition controller to cluster-config-operator, it could be added to another existing core operator. The most plausible candidates:

#### Controller in CVO

An alternative is to add transition controllers to the cluster-version-operator (CVO).

**Why it was rejected**:
- CVO is a critical-path operator — every cluster depends on it for updates. Adding topology transition logic increases the surface area for bugs in a component where failures have outsized blast radius
- CVO is always active and manages every cluster. The topology transition controller is gated by a feature gate and only active when needed. However, embedding long-running orchestration workflows in CVO couples their failure modes unnecessarily
- Topology transitions and version management are operationally distinct workflows with different preconditions, sequencing, and failure handling. While both touch infrastructure state, a topology transition is not a version change — it coordinates operators laterally rather than rolling out a new payload

#### Controller in cluster-etcd-operator (CEO)

CEO already handles the most critical part of a topology transition — etcd member scaling. An alternative is to extend CEO to orchestrate the full transition workflow.

**Why it was rejected**:
- CEO's scope is etcd lifecycle management. Topology transitions require coordinating ingress, networking, and other operators beyond etcd — expanding CEO's responsibility well beyond its current domain
- CEO is a critical-path operator. Bugs in transition orchestration logic could affect etcd operations on clusters that never use topology transitions
- The same blast-radius argument that applies to CVO applies here — critical operators should not absorb optional orchestration workflows

#### Controller in machine-config-operator (MCO)

MCO handles node-level changes and rolling operations, making it a candidate for orchestrating node-topology changes.

**Why it was rejected**:
- MCO's domain is machine configuration (OS, kubelet config, node-level state), not cluster topology orchestration
- Topology transitions require cross-operator coordination (etcd, ingress, networking, infrastructure CR) that is outside MCO's current scope
- Like CVO and CEO, MCO is a critical-path operator where additional surface area increases risk to every cluster

**Note on CCO scope expansion**: The scope-expansion concern raised against CEO and MCO also applies to CCO, which currently focuses on CRD manifests and config synchronization. However, CCO is the canonical owner of the infrastructure CR and the `config.openshift.io` API group, making it the most natural home.
The transition controller is also feature-gated with near-zero overhead when inactive, unlike CEO or MCO where additional code paths could affect core operations regardless of whether transitions are used.

## Open Questions

1. **OLM operator impact**: Which OLM-managed operators read topology values? Do they watch the infrastructure CR or read at startup only? This determines whether operators need code changes or just a restart after transition.

2. **Per-operator transition behavior**: The transition behavior for CEO is understood (etcd sequential scaling). The specific requirements for ingress, networking, monitoring, and other operators during a topology transition need validation during dev preview. The per-operator topology dependency matrix is a prerequisite for entering dev preview — see [Graduation Criteria](#entering-dev-preview).

3. **Minimum resource requirements**: The controller should validate that new control-plane nodes meet minimum resource requirements before initiating a transition. The specific resource thresholds need to be defined.

4. **Backup compatibility across topologies**: If an administrator takes an etcd backup on a SNO cluster and later transitions to HA, is the pre-transition backup usable for restore on the post-transition cluster? A new backup should be taken after a successful transition, but the interaction between pre-transition backups and post-transition cluster state needs investigation.
   Ideally restoring the pre-transition backup would revert the cluster to SNO, but that flow needs to be validated.

5. **OLM operator topology declarations**: A future enhancement should define an OLM manifest mechanism allowing operators to declare which topologies they support, so the transition controller and catalog tooling don't have to optimistically assume support by default. See [Optional (OLM-Managed) Operators and Topology Changes](#optional-olm-managed-operators-and-topology-changes).

## Test Plan

### Proposed API Validation Tests

API PR #3029 covers all availability states; required `source`, `target`, and `availability`; enums; source/target inequality; source/status consistency; conditional reason requirements; reason/message bounds; duplicate `(source, target)` rejection; and rejection above the two-entry list limit.
Before dev preview, selected-API tests must also cover omitted and empty lists for `SingleReplica` and a current topology outside the discovery enum, plus feature-gated schema presence and absence in Infrastructure and the Infrastructure schema embedded in machineconfiguration's ControllerConfig.
Before dev preview, the selected API must also validate the discovery evaluation timestamp and observed resource version.

### CI Lanes

| Lane | Frequency | Description |
| ---- | --------- | ----------- |
| MutableTopology transition suite | Nightly | Run transition test suite: SNO → HA compact on `platform: none` |
| End-to-End tests (e2e) | Weekly | Standard test suite (openshift/conformance/parallel) on post-transition clusters |
| Upgrade between z-streams | Weekly | Test upgrades on post-transition clusters |
| Upgrade between y-streams | Weekly | Test upgrades across minor versions on post-transition clusters |

### CI Tests

#### Pre-Transition Tests

| Test | Description |
| ---- | ----------- |
| Precondition validation | Verify the controller withholds admission when nodes are missing/not-ready, dedicated workers are present, cluster operators are unstable, or etcd has not yet reached quorum with 3 voting members |
| Availability publication | Verify applicable transitions are published before intent is set, blocked transitions retain diagnostics, and availability changes as prerequisites change without initiating a transition or blocking upgrades |
| Incomplete discovery | Verify controller and client handling of `Unknown`, omitted and empty lists, and no applicable transitions |
| CLI interaction | Verify discovery displays controller-published availability and diagnostics without changing spec, and transition requests correctly patch `spec.controlPlaneTopology` |
| Stale availability | Change preconditions after an `Available` status read and verify the controller withholds admission after revalidating the request |
| Active request discovery | Verify Pending, Admitted, and Progressing reads publish `Unavailable` with `TransitionInProgress`, and the CLI rejects duplicate requests regardless of a prior `Available` snapshot |

#### Transition Tests

| Test | Description |
| ---- | ----------- |
| SNO → HA compact (3-node) | Full transition on `platform: none`: node-driven etcd scaling completes first, then the controller admits the transition and updates infrastructure status |
| etcd quorum as precondition | Verify CEO's existing 1→2→3 member addition completes independently of the CLI command, and that the controller does not admit the transition until it has |
| Failure and recovery | Verify the controller withholds admission indefinitely when a precondition never becomes true (e.g., node unreachable, etcd never finishes promotion), and that CEO's own etcd disaster recovery procedures are unaffected by and independent of the transition controller |
| Post-transition operator health | Verify all operators reconcile successfully after infrastructure topology status fields are updated |
| Discovery consistency | Verify topology updates clear or replace old-source entries in the same status update; after SNO → HA the list is omitted or empty, while CCO conditions continue to report post-transition validation |
| Upgrade safety | Verify `ClusterOperator/config-operator` reports the canonical `Upgradeable=False` condition while a transition is pending or in progress, CVO rejects upgrades, and the condition is correctly restored after rejection, cancellation, and completion |
| Race and conflict handling | Verify correct behavior for an upgrade starting during transition admission, precondition loss after preflight, Infrastructure status-update conflicts, and a condition update that succeeds before a status update fails |
| Cancellation | Verify the CLI uses JSON Patch `test` and `replace` to reset Pending intent to observed topology, rejects explicit empty strings and post-admission intent changes, and returns a conflict when the request has changed |
| CCO fail-closed behavior | Verify an absent feature gate disables the controller without panic, and an operator-state read failure blocks rendered-MachineConfig validation |

### QE Testing

Standard QE testing scenarios will include:
- Full SNO → HA compact transition on `platform: none`
- Transition failure and recovery scenarios
- Post-transition cluster stability over 24 hours
- Destructive testing: control-plane node failure during the 2-member etcd window
- Network partition scenarios during transition (e.g., partition between etcd members during scaling)
- Concurrent operation testing: transition + upgrade attempt (verify mutual exclusion)
- Node resource exhaustion during transition (e.g., insufficient disk or memory on new control-plane nodes)
- Backup pre-transition and then restore that backup post-transition

## Graduation Criteria

### Entering Dev Preview

- Manual SNO-to-HA transition tested (scaling a single-replica cluster to multiple replicas) to validate assumptions about operator behavior
- Topology transition controller implemented in cluster-config-operator with SNO → HA compact support
- `controlPlaneTopology` field added to `InfrastructureSpec`
- `oc adm transition topology` CLI command implemented
- `MutableTopology` feature gate added to `DevPreviewNoUpgrade` feature set
- `TopologyMode` enum and permitted-direction validation validated in API integration tests
- Per-operator topology dependency matrix completed: for each in-payload operator that reads `controlPlaneTopology` or `infrastructureTopology`, document what the operator uses the value for (replica count, scheduling, feature enablement) and whether it watches the infrastructure CR for changes or reads the value only at startup
- Operators that read topology only at startup are identified and a restart strategy is documented for post-transition reconciliation
- CCO sets the canonical `Upgradeable=False` condition on `ClusterOperator/config-operator` while a topology transition is pending or in progress; CVO upgrade blocking, cancellation, conflict retry, and upgrade-vs-transition races are covered by tests
- CCO implements the required idle, pending, admitted, and progressing state contract; no intent change can bypass post-transition validation or re-enable upgrades early
- CCO treats an absent feature gate as disabled and fails closed on operator-state read errors during post-transition validation
- The selected discovery API is merged and used consistently by API, CCO, and oc; CCO publishes applicable transitions with `Available`, `Unavailable`, or `Unknown` availability and required diagnostics before a request, and API integration tests validate its contract
- CCO tests cover idle publication, refresh without spec changes, stale-request rejection, `TransitionInProgress` publication, conflict retry, and atomic topology/discovery replacement
- The discovery API exposes evaluation time and observed Infrastructure resource version. CLI discovery defines and tests stable output and exit behavior for omitted, empty, `Unknown`, `Unavailable`, stale, `TransitionInProgress`, and `Available` data
- CVO acknowledgement, acknowledgement timeout, and rollback to idle conditions are implemented and covered by upgrade-vs-transition tests
- CI lanes operational for transition testing
- Developer documentation available

### Dev Preview -> Tech Preview

- Transition test suite validates full SNO → HA compact path
- Tests verify operator health during and after transitions
- Controller failure handling validated; etcd disaster recovery procedures documented for quorum loss scenarios
- `oc adm transition topology` command provides clear diagnostics on failure
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/)
- End-to-end validation that CLI correctly patches `controlPlaneTopology` and the controller rejects unsupported transitions
- **Dependency**: Platform bare metal single-node support status assessed with the Bare Metal Networking team. If keepalived cannot be configured for single-node clusters, the limitation is documented and `platform: none` remains the only supported path

### Tech Preview -> GA

- Full test coverage including upgrades (y-stream and z-stream) on post-transition clusters
- SLOs documented and validated: target transition duration (SNO → HA compact), success rate threshold, and maximum time in the 2-member etcd window
- Monitoring and telemetry for transition metrics: Prometheus metrics exposed (transition_started, transition_completed, transition_failed, transition_duration_seconds) with alerts defined for stuck transitions exceeding SLO thresholds
- Support procedures documented
- Feature gate moved to `Default` feature set
- A plan is defined for improving the workflows (UX, transition functionality, and operator maintenance) around optional (OLM-managed) operators during topology transitions; actually managing optional operators remains out of scope for this enhancement and is left for a future enhancement
  (see [Optional (OLM-Managed) Operators and Topology Changes](#optional-olm-managed-operators-and-topology-changes) for more information on approaches that need to be addressed)

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

### Upgrades

Clusters that have undergone topology transitions follow standard OpenShift upgrade procedures. The resulting topology values (`HighlyAvailable`, `SingleReplica`, etc.) are existing enum values that all operators already support. There are no special upgrade considerations for post-transition clusters.

The topology transition controller upgrades as part of cluster-config-operator via the standard CVO-managed upgrade path.

### Downgrades

**Z-stream downgrades** (within a minor version that supports mutable topology):
Standard downgrade procedures apply. Completed transitions are not reverted — the cluster retains its current topology.

**Y-stream downgrades**:
CVO blocks y-stream downgrades.

## Version Skew Strategy

Mutable topology is gated by the `MutableTopology` feature gate. The topology transition controller is only active when the feature gate is enabled.

The current controller writes only a custom upgradeable condition and does not prevent CVO from initiating an upgrade. Version skew during a transition is therefore not yet safely handled.
The required canonical `ClusterOperator/config-operator` `Upgradeable=False` integration and race tests are a prerequisite for the dev preview upgrade-safety guarantee.

Post-transition clusters use standard topology values that all operator versions understand. There is no version skew risk for completed transitions.

Discovery clients must tolerate a cluster API or controller version that does not expose or populate the selected discovery field. Omitted and empty lists cannot establish whether evaluation has occurred or a transition is unsupported.
The selected discovery API exposes evaluation time and observed Infrastructure resource version. For automation, clients accept data only when it is no more than two minutes old and `ClusterOperator/config-operator` is Available and not Degraded; otherwise they report stale or unavailable discovery. Request-time revalidation remains authoritative regardless of client version or last advertised availability.

## Operational Aspects of API Extensions

This enhancement adds `spec.controlPlaneTopology` to `InfrastructureSpec` and proposes `status.controlPlaneTopologyTransitions` for discovery. The spec field:

- Has no impact when it matches the current `status.controlPlaneTopology` or is empty
- During transitions, the CCO topology transition controller makes API calls to coordinate operator transition. These calls are low-frequency and bounded by the transition sequence.

Field-level enum and CEL validation provide API-server-level checks with no additional services required. Topology status fields are not protected by new admission policies — this is consistent with other infrastructure status fields.

The proposed discovery field is bounded to the singleton Infrastructure resource and a maximum of two entries. It runs on controller syncs even when spec is empty or matches status; evaluation time and observed resource version update on the one-minute resync.
If the controller is unavailable, discovery can be absent or stale. This does not itself change topology or prevent ordinary workload operation. Automation treats data older than two minutes, or data from an unavailable or degraded config-operator, as unavailable.

## Support Procedures

### Team Ownership

**OpenShift Edge Team:**
- Topology transition controller in cluster-config-operator
- CLI (`oc adm transition topology` command)
- Supported transition definitions and validation logic
- Infrastructure CR API changes (`TopologyMode` validation and `spec.controlPlaneTopology` field)
- Proposed Infrastructure discovery API (`status.controlPlaneTopologyTransitions`) and its controller/CLI integration

**Control Plane Team:**
- cluster-etcd-operator (CEO) node-driven etcd scaling — existing, unmodified behavior that the transition controller relies on as a precondition

**Bare Metal Networking Team:**
- Bare metal networking for SNO clusters (future platform support)

**Component Teams:**
- Validate operator behavior during and after transitions

### Detecting Issues

**Transition Discovery Unavailable or Stale (after implementation):**
- Check: `oc get infrastructure cluster -o yaml` for `status.controlPlaneTopologyTransitions`
- For `Unavailable`, inspect `reason` and `message`, address the reported prerequisites, and allow the controller to re-evaluate
- For `Unknown`, evaluation has not completed; check CCO health and controller logs if it persists
- An omitted or empty list can mean evaluation has not occurred or there are no applicable transitions; do not use it for automation
- Treat discovery older than two minutes, or discovery while `config-operator` is unavailable or degraded, as unavailable
- `TransitionInProgress` means a request already exists; inspect the request and controller conditions instead of submitting another request

**Transition Stuck or Failed:**
- Symptom: Custom conditions on `operator.openshift.io/v1 Config/cluster` show transition in progress or failed for an extended period
- Check: `oc get configs.operator.openshift.io cluster -o yaml` for controller-specific conditions
- Check: `oc get clusteroperator config-operator -o yaml` for aggregate operator status and the canonical CVO-consumed `Upgradeable` condition
- Check: cluster-config-operator logs for transition controller errors
- Check: CEO logs for etcd scaling operations
- Resolution: Address the reported issue and retry, or contact support

**etcd Scaling Failures:**
- Symptom: etcd cluster unhealthy during the node-driven prerequisite scaling — the transition controller will not admit the transition until this resolves
- Check: CEO logs for etcd scaling operations
- Check: etcd member list: `oc -n openshift-etcd exec <etcd-pod> -- etcdctl member list`
- Resolution: If quorum is lost, follow standard etcd disaster recovery procedures (`quorum-restore.sh`), independent of the topology transition controller. Automated rollback is not possible without quorum. Restoring to pre-transition snapshot could operate as a fallback recovery procedure pending verification of that procedure. 

### Recovery Procedures

| Failure Mode | Impact | Recovery |
| ------------ | ------ | -------- |
| Controller fails during precondition check | No impact — transition not admitted | Address the precondition and retry |
| etcd quorum loss during prerequisite scaling (2-member window, pre-admission) | API unavailable — no automated recovery possible; transition controller cannot help since the transition was never admitted | Manual intervention required: administrator runs `quorum-restore.sh` per standard etcd disaster recovery procedures, independent of the transition controller |
| Operator fails to reconcile post-transition | Operator-specific impact | Investigate operator logs; file bug against the operator component |
| CCO crash during transition | Transition paused | CCO restarts via deployment controller and the transition controller resumes reconciliation |

## Infrastructure Needed

No additional infrastructure is required for this feature.

CI will experience increased demand as new test lanes are introduced to support:
- Full SNO → HA compact transitions on `platform: none`
- Post-transition cluster stability validation
- Upgrade testing on post-transition clusters
