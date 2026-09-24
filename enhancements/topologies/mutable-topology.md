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
last-updated: 2026-09-15
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

This enhancement targets two transition paths: `controlPlaneTopology` transitions (SingleReplica → HighlyAvailable on compact clusters) and standalone `infrastructureTopology` transitions (SingleReplica → HighlyAvailable on clusters with dedicated workers). The broader topology landscape is acknowledged here because the architecture must not preclude future support for additional configurations.

**Mutable Topology** — The capability for an OpenShift cluster to transition between topology modes as a Day 2 operation, removing the existing assumption that topologies are immutable after installation.

**Topology Transition** — A directed change from one topology mode to another (e.g., SingleReplica to HighlyAvailable). Transitions are managed by a controller in cluster-config-operator and follow a set of supported transitions.

**Control Plane Topology** — The cluster-topology mode describing how control-plane nodes are deployed and managed (SingleReplica, HighlyAvailable, or other supported modes). Control-plane nodes are nodes labeled with `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master`.

**Infrastructure Topology** — The cluster-topology mode describing how infrastructure workloads are distributed (SingleReplica, HighlyAvailable, or other supported modes). When there are no dedicated worker nodes, `infrastructureTopology` is set to match `controlPlaneTopology` since control-plane nodes serve as workers.

**Infrastructure Topology Transition** — A directed change of infrastructure topology independent of control-plane topology. This applies to clusters where `status.controlPlaneTopology = HighlyAvailable` and worker nodes are present with infrastructure workloads running in SingleReplica mode. The transition moves infrastructure workloads to HA replica counts without modifying the control plane or etcd.

**Compact Cluster** — A cluster where control-plane nodes also serve as workers. In the initial SNO-to-HA transition, the target is a 3-node compact cluster with no dedicated worker nodes. The compact deployment shape is a consequence of not adding dedicated worker nodes — it is not a distinct `TopologyMode` enum value.

**mastersSchedulable** — A field in the infrastructure status indicating whether control-plane nodes are schedulable for general workloads. The topology transition controller recalculates this value as part of a transition.

**Cluster Administrator** — An entity responsible for managing an existing cluster, including Day 2 operations such as topology transitions and node scaling. This may be a human operator or an external orchestrator/agent.

## Summary

This enhancement introduces "mutable topology" which is defined as "the ability for OpenShift clusters to transition between topology modes as a Day 2 operation". This changes the existing OpenShift assumption that topologies are immutable after installation.

Two transition paths are supported:

1. **Control-plane topology transition** (SNO → HA compact): A new `controlPlaneTopology` field in the infrastructure spec expresses the administrator's intent to transition the control plane. The controller coordinates etcd scaling prerequisites, operator reconciliation, and status field updates.

2. **Standalone infrastructure topology transition** (SingleReplica → HA with dedicated workers): A new `infrastructureTopology` field in the infrastructure spec expresses the administrator's intent to transition infrastructure workloads independently of the control plane. Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable` and sufficient worker nodes. This path does not modify the control plane or etcd. The compact cluster path — where the CP transitions and infrastructure follows in lockstep — is future work; the API design supports it naturally but requires updating the CP path and dedicated CI lanes.

A topology transition controller in cluster-config-operator watches for changes to both spec fields, validates preconditions, coordinates transitions, and updates the existing topology status fields when the cluster is ready.
A new `oc adm transition topology` CLI command provides an interface for cluster administrators to initiate transitions.
The initial implementation supports transitioning Single Node OpenShift (SNO) clusters to HA compact (3-node) on `platform: none`, and standalone infrastructure topology transitions (SingleReplica → HA) on clusters with dedicated workers on `platform: none`.

This enhancement supersedes the [Adaptable Topology proposal](https://github.com/openshift/enhancements/pull/1905), which proposed a new `Adaptable` topology mode requiring changes across all core operators. That proposal is withdrawn in favor of this controller-based approach.

## Motivation

Cluster demands change over time. Customers who start with Single Node OpenShift (SNO) at edge locations may later require high availability as workloads become more critical. Today, this requires redeploying the cluster — a disruptive operation that involves workload migration, downtime, and operational overhead.

Similarly, clusters that were deployed with dedicated workers but with infrastructure workloads running in SingleReplica mode may need to transition to HA infrastructure as those workloads become more critical — without touching the control plane.

The previous approach to this problem ([Adaptable Topology](https://github.com/openshift/enhancements/pull/1905)) proposed a new `Adaptable` topology mode where operators would dynamically react to node count changes.
That approach required updating every core operator to handle dynamic topology shifts and introduced a new topology enum value that all operators had to understand. It also coupled topology behavior to node count, making operator logic more complex.

Mutable topology takes a different approach: instead of adding a new topology mode that operators must interpret, transitions are orchestrated by a controller in cluster-config-operator that coordinates the sequencing, validates preconditions, and updates the infrastructure CR only when the cluster is ready for the new mode.
Operators continue to react to the same fixed topology values they already understand. This keeps operator logic simple and concentrates transition complexity in an existing core component.

### User Stories

* As a cluster administrator running Single Node OpenShift (SNO) at an edge location, I want to add control-plane nodes to my cluster to achieve high availability so that I can handle node failures without service disruption as workloads become more critical.

* As a cluster administrator deploying OpenShift clusters at scale, I want to start with minimal footprint deployments that can grow into highly available clusters so that I can reduce initial costs while maintaining scalability.

* As a cluster administrator managing a fleet of edge deployments, I want a supported path to transition my cluster topology so that I don't need to redeploy clusters when my infrastructure requirements change.

* As a cluster administrator, I want topology transitions managed through a well-defined API so that I have a clear interface for monitoring transition state and integrating with my operational tooling.

* As a cluster administrator with dedicated workers running infrastructure workloads in SingleReplica mode, I want to transition infrastructure topology to HighlyAvailable independently of my control plane so that infrastructure services gain redundancy without requiring control-plane changes.

### Goals

* Officially support topology transitions in OpenShift
* Provide a supported interface for administrators to initiate topology transitions
* Support transitioning SNO clusters to HA compact (3-node) on `platform: none` as the initial transition path
* Support standalone infrastructure topology transitions (SingleReplica → HA) on clusters with dedicated workers on `platform: none`
* Maintain backward compatibility — existing clusters with fixed topology modes are unaffected
* Establish the architectural foundation for additional transition paths in the future

### Non-Goals

* Supporting all possible topology transitions in the initial implementation (only SNO → HA compact and standalone infra SingleReplica → HA on `platform: none`)
* Supporting transitions for HyperShift or hosted control plane clusters
* Supporting transitions for MicroShift deployments
* Supporting transitions for Image Based Install (IBI) clusters
* Automatic node provisioning or deprovisioning based on workload demands
* Scaling down control-plane or worker nodes (scale-down may be addressed in a future enhancement)
* Supporting bidirectional transitions (e.g., HA → SNO, HA infra → SingleReplica infra) in the initial implementation

## Proposal

This enhancement introduces new infrastructure API fields and a topology transition controller in cluster-config-operator (CCO; not to be confused with cloud-credential-operator) to enable topology transitions as Day 2 operations.

The approach follows the standard OpenShift spec/status contract and mirrors the pattern used by `oc adm upgrade`:

1. **`controlPlaneTopology` field in InfrastructureSpec** — Expresses the administrator's intent to transition the control plane. The CLI patches this field to initiate a control-plane transition. The existing `controlPlaneTopology` and `infrastructureTopology` fields in status continue to represent the cluster's observed topology.

2. **`infrastructureTopology` field in InfrastructureSpec** — Expresses intent for the cluster's infrastructure topology. This field may be set by the administrator (standalone path) or by the control-plane transition controller (compact cluster path). When set and the value differs from `status.infrastructureTopology`, the topology transition controller initiates an infrastructure transition. The infrastructure controller reconciles `status.infrastructureTopology` regardless of which path produced the spec write.

3. **Topology transition controller in cluster-config-operator** — A new controller in CCO that watches the infrastructure CR for `controlPlaneTopology` and `infrastructureTopology` spec changes, validates preconditions, coordinates transitions, and updates the status topology fields when the cluster is ready for the new mode.

4. **`oc adm transition topology` CLI command** — A command that validates preconditions, patches the appropriate spec field on the infrastructure CR (`spec.controlPlaneTopology` or `spec.infrastructureTopology`), and returns immediately.

The transition controller is proposed to live in cluster-config-operator because CCO is the canonical owner of the `config.openshift.io` API group and the Infrastructure CR.
The controller is feature-gated using the standard library-go FeatureGateAccess pattern: when the gate is disabled the controller is not registered with the manager and incurs negligible runtime overhead; a gate change triggers an operator restart via ForceExit so the new state is picked up cleanly.

See [Alternatives](#alternatives-not-implemented) for the full analysis of controller placement options.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Mutable topology is not compatible with HyperShift clusters. HyperShift uses `External` as its `controlPlaneTopology`, and topology transitions are not applicable to hosted control planes where the control plane lifecycle is managed externally.

Future support for HyperShift is not planned for this enhancement but is not ruled out.

#### Standalone Clusters

Standalone clusters are the primary target for mutable topology. This enhancement enables standalone clusters to start with minimal footprints and transition to multi-node configurations without redeployment.

**Control-plane transitions**: `platform: none` is the only supported platform for the initial SNO → HA compact transition. This is a deliberate scope constraint:

- The primary customers for mutable topology are edge computing deployments, where SNO clusters are deployed with minimal footprint and need to scale to HA as workloads grow. Edge sites commonly use `platform: none`, making it the natural starting point for this enhancement.
- `platform: none` has no platform-managed infrastructure (no cloud load balancers, no keepalived, no CCM) — the administrator owns all external networking. This eliminates the need for the transition controller to interact with platform-specific infrastructure, keeping the initial implementation focused on the core topology transition mechanics.
- `platform: baremetal` requires keepalived-managed load balancing, which does not currently support single-node clusters. Adding SNO support to `platform: baremetal` is a prerequisite for baremetal topology transitions and is planned for a subsequent phase.
- Cloud platforms (AWS, Azure, GCP) require CCM and platform-specific load balancer integration during node scaling, which adds significant scope.

On `platform: none`, the administrator is responsible for external networking prerequisites (VIPs, DNS, load balancer configuration) as described in the [Pre-Transition](#pre-transition) workflow.

**Infrastructure-only transitions**: The standalone infrastructure topology transition targets clusters with dedicated worker nodes. The initial platform support is `platform: none`, matching the control-plane path. The infrastructure transition does not interact with control-plane nodes, etcd, or platform-specific load balancing — it validates worker node readiness and coordinates infrastructure operator reconciliation to HA replica counts.

| Platform | CP Transition | Infra Transition | Notes |
|---|---|---|---|
| `platform: none` | Yes (initial) | Yes (initial) | Administrator manages load balancing, VIPs, DNS |
| `platform: baremetal` | Future | Future | Pending keepalived SNO support resolution |
| Cloud platforms (AWS, Azure, GCP) | Future | Future | Requires CCM and load balancer integration |
| HyperShift | Not applicable | Not applicable | External control plane management is incompatible |
| IBI | Not applicable | Not applicable | Not supported |

This design does not inhibit expansion to other platforms — the supported transitions list and precondition validation are per-transition, so platform-specific transitions can add their own checks without changing the controller architecture.

#### Single-node Deployments or MicroShift

Single Node OpenShift (SNO) clusters are the primary source topology for control-plane transitions. The initial use case is enabling SNO deployments to transition to HA compact (3-node) configurations as requirements change.

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

1. The cluster administrator prepares exactly 2 additional control-plane nodes and joins them to the cluster — the kubelet is running on each node and Node objects exist in the Kubernetes API. On `platform: none`, the administrator manages their own load balancing configuration (VIPs, DNS).
2. **Node-driven operator reactions (prerequisite)** — independent of any topology intent, as soon as the new Node objects appear: cluster-etcd-operator (CEO) scales etcd members sequentially (1→2→3) via its existing unsafe/day-2 scaling path, reusing the learner-to-voter promotion mechanism from bootstrapping;
   the kube-apiserver, kube-controller-manager, and kube-scheduler operators render static pod manifests for the new nodes.
   This is existing, unmodified operator behavior — the topology transition controller does not trigger, sequence, or wait on it directly. It only observes the outcome (a healthy 3-member etcd, `Ready` control-plane nodes) as a precondition in step 7. This can complete before, during, or after step 3.
3. The cluster administrator runs `oc adm transition topology HighlyAvailable`
4. The CLI validates preconditions before patching (e.g., feature gate enabled, no transition already in progress)
5. The CLI patches the infrastructure CR: `spec.controlPlaneTopology: HighlyAvailable`
6. The API server validates `controlPlaneTopology` against the `TopologyMode` enum, rejecting unsupported topology modes before accepting the write

##### During Transition

7. The topology transition controller in CCO detects the `controlPlaneTopology` change and validates preconditions:
   - Every ClusterOperator other than cluster-config-operator itself reports `Available=True`, `Progressing=False`, `Degraded=False`
   - Exactly 3 nodes with `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master` labels are present, all schedulable and `Ready`
   - No dedicated worker nodes are present (the initial implementation targets compact clusters only; clusters with dedicated workers require a standalone infrastructure topology transition — see [Infrastructure Topology Transition](#transition-standalone-infrastructure-topology-singlereplica--ha))
   - etcd already reports quorum, is not mid-scaling, and already has 3 voting members — i.e., step 2's node-driven scaling has already finished
   If any precondition fails — including an etcd that has not yet finished scaling — the controller does not admit the transition; it records the reason and re-evaluates on the next sync (see [Failure Handling](#failure-handling)).
8. Once preconditions pass, the controller verifies an upgrade has not been triggered by CVO and then sets `Upgradeable=False` and a `Progressing` condition on the CCO `ClusterOperator` in the same update, signaling that a transition is in progress and preventing CVO from initiating an upgrade
9. The controller updates the infrastructure status and spec fields:
   - `status.controlPlaneTopology` transitions from `SingleReplica` to `HighlyAvailable`
   - If `spec.infrastructureTopology` is omitted or does not match the target, the controller explicitly sets `spec.infrastructureTopology = HighlyAvailable`. This makes infrastructure topology intent explicit rather than derived. (Note: this compact cluster lockstep behavior is future work — the initial implementation of the standalone infrastructure path does not depend on this step. See [Scope](#standalone-infrastructure-topology-transition) for details.)
   - The infrastructure controller then picks up the `spec.infrastructureTopology` / `status.infrastructureTopology` divergence and reconciles `status.infrastructureTopology` through its own prerequisite validation and lifecycle — validating that the dual-role nodes are Ready and schedulable. If `spec.infrastructureTopology` is already set and matches the target, no additional action is needed.

10. **Topology-driven operator reactions** — operators that watch the infrastructure status topology fields reconcile against the new values and adjust their deployment strategies, replica counts, and placement policies.
    This is a distinct phase from step 2: step 2 covers operators reacting to node presence before the transition is even admitted, step 10 covers operators reacting to the topology status change after admission.
    The set of operators with topology-dependent behavior has not been fully enumerated — building the per-operator topology dependency matrix is a prerequisite for entering dev preview (see [Graduation Criteria](#entering-dev-preview) and [Open Questions](#open-questions)).

    **Note**: OLM-managed operators that read topology at startup rather than watching for changes may need to be restarted after the transition completes. See [Optional (OLM-Managed) Operators and Topology Changes](#optional-olm-managed-operators-and-topology-changes) for details.

##### Post-Transition

11. After a soak period (5 minutes) following the `Progressing` condition, the controller checks that control-plane/worker node readiness, etcd health, MachineConfig rollout, ingress router replicas, and API server operator replica counts have reconciled to the target values
12. Once all checks pass, the controller clears the `Progressing` condition and sets `Upgradeable=True`. The infrastructure status reflects the completed transition — `spec.controlPlaneTopology` matches `status.controlPlaneTopology`, so no further action is taken.

The CLI returns immediately after patching `spec.controlPlaneTopology` (step 5). Administrators can monitor transition progress by watching CCO ClusterOperator status conditions (e.g., `oc get clusteroperator cluster-config-operator -o yaml`).

##### Failure Handling

The controller recognizes two distinct failure windows, and makes no guarantees about the node-driven etcd scaling itself:

- **Before admission**: if a precondition never becomes true — for example etcd never finishes scaling to 3 voting members, or a control-plane node never becomes `Ready` — the controller simply never admits the transition.
  `spec.controlPlaneTopology` remains diverged from `status.controlPlaneTopology` indefinitely, and the `Progressing`/`Upgradeable` conditions carry a diagnostic reason (e.g. `PreflightCheckFailed`) that the administrator can inspect.
  Failures in the node-driven etcd scaling itself — including quorum loss in the 2-member window, which requires manual recovery via `quorum-restore.sh` — are cluster-etcd-operator's existing failure domain; the topology transition controller neither triggers nor is able to recover from them, since they occur before it admits the transition.
- **After admission**: if a post-transition validation criterion never passes (e.g., an operator fails to reconcile), the `Progressing` condition remains `True` and `Upgradeable` remains `False` indefinitely. The administrator inspects CCO and the relevant operator's logs and ClusterOperator status conditions for details.
  `spec.controlPlaneTopology` remains unchanged — the controller re-evaluates reconciliation on every sync. Cancellation is not supported — both spec topology fields are immutable during a transition (see [Immutability during transition](#immutability-during-transition) in the infrastructure workflow). The administrator must wait for the transition to complete or address the failing prerequisite.

#### Transition: Standalone Infrastructure Topology (SingleReplica → HA)

This transition path applies to clusters with dedicated worker nodes. It transitions `status.infrastructureTopology` from `SingleReplica` to `HighlyAvailable` independently of the control plane — no control-plane nodes are added, removed, or reconfigured, and etcd membership is unchanged.

**Operational guidance**: Unlike the control-plane transition, the infrastructure transition does not involve etcd scaling or a 2-member quorum risk window. However, administrators should still treat it as a maintenance window because infrastructure operators will be reconciling to new replica counts.

##### Pre-Transition

1. The cluster administrator verifies that sufficient worker nodes are present, `Ready`, and schedulable to host HA infrastructure workloads
2. The cluster administrator runs `oc adm transition topology --infrastructure=HighlyAvailable --confirm`
3. The CLI discovers available transitions from the Infrastructure status, validates client-side preconditions (feature gate enabled, no transition in progress), and patches `spec.infrastructureTopology: HighlyAvailable`. Without `--confirm`, the command shows what would change without admitting the transition.
4. The API server validates `infrastructureTopology` against the `TopologyMode` enum

##### During Transition

5. The topology transition controller in CCO detects the `spec.infrastructureTopology` divergence from `status.infrastructureTopology` and validates preconditions:
   - Every ClusterOperator other than cluster-config-operator itself reports `Available=True`, `Progressing=False`, `Degraded=False`
   - Sufficient worker-capable nodes are present to host HA infrastructure replicas. A worker node eligible for this check is any node that:
     - Has the `node-role.kubernetes.io/worker` label (nodes that also carry `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master` labels — dual-role nodes — are included)
     - Is `Ready`
     - Is schedulable (not cordoned or draining)
     - Is not blocked by incompatible taints or lifecycle state
     - Is able to host the expected infrastructure replicas
   - The platform is supported (`platform: none` initially)
   - No incompatible upgrade is in progress

   The infrastructure controller does not perform cross-field API validation against `spec.controlPlaneTopology`. However, infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable` — this is enforced by CCO through the available transitions list, not by API-level x-validation rules. See [No cross-field validation](#infrastructure-api-changes) for rationale.

   If any precondition fails, the controller does not admit the transition; it records the reason on the `Progressing`/`Upgradeable` conditions (e.g. `PreflightCheckFailed`, `UnsupportedPlatform`, `UpgradeInProgress`) and re-evaluates on the next sync.
6. Once preconditions pass, the controller verifies an upgrade has not been triggered by CVO and then sets `Upgradeable=False` and a `Progressing` condition (`reason: InfrastructureTopologyTransitionInProgress`) on the CCO `ClusterOperator` in the same update
7. The controller re-reads the Infrastructure CR (fresh API read) to confirm `spec.infrastructureTopology` has not changed since preconditions were checked
8. **Infrastructure operator reactions** — operators that watch `status.infrastructureTopology` reconcile against the new values and adjust replica counts to HA levels

##### Post-Transition

9. After a soak period (5 minutes) following the `Progressing` condition, the controller checks that infrastructure operators have converged:
   - Ingress Operator: default IngressController replicas scaled to 2+
   - Monitoring: Prometheus, Alertmanager at HA replicas
   - Image Registry: registry replicas scaled
   - Console: console replicas scaled
   - OAuth: OAuth server replicas scaled
   - Worker nodes remain `Ready` and schedulable
10. Once all checks pass, the controller updates the infrastructure status:
    - `status.infrastructureTopology` transitions from `SingleReplica` to `HighlyAvailable`

    The controller clears the `Progressing` condition (`reason: InfrastructureTopologyTransitionComplete`) and sets `Upgradeable=True` on the CCO `ClusterOperator`. The infrastructure status reflects the completed transition — `spec.infrastructureTopology` matches `status.infrastructureTopology`, so no further action is taken.

The CLI returns immediately after patching `spec.infrastructureTopology` (step 3). Administrators can monitor transition progress by watching CCO ClusterOperator status conditions (e.g., `oc get clusteroperator cluster-config-operator -o yaml`). The `Reason` field on the `Progressing` condition distinguishes infrastructure transitions (`InfrastructureTopologyTransition*`) from control-plane transitions (`TopologyTransition*`).

##### Failure Handling

- **Before admission**: if a precondition fails (e.g., insufficient workers, unsupported platform, incompatible upgrade in progress, cluster un-upgradeable, CP not HA), the `Progressing`/`Upgradeable` conditions carry a diagnostic reason (e.g. `PreflightCheckFailed`, `UnsupportedPlatform`, `UpgradeInProgress`, `ClusterNotUpgradeable`). The controller re-evaluates on every sync — if the prerequisite is later satisfied (e.g., workers become Ready), the transition proceeds automatically.
- **After admission**: if a post-transition validation criterion never passes (e.g., an operator fails to reconcile to HA replicas), the `Progressing` condition remains `True` and `Upgradeable` remains `False` indefinitely. The administrator inspects CCO and the relevant operator's logs and ClusterOperator status conditions for details.

**Immutability during transition**: Neither `spec.controlPlaneTopology` nor `spec.infrastructureTopology` can be changed unless **both** fields match their status counterparts (`spec.controlPlaneTopology == status.controlPlaneTopology` AND `spec.infrastructureTopology == status.infrastructureTopology`). If either pair diverges, both spec fields are immutable. This matches upgrade behavior — you cannot admit a new upgrade while one is already in progress.

Both fields can be set together in a single write. The immutability check applies at admission time against the current stored state. If both fields currently match status, a single patch that sets both is allowed — the webhook sees that no transition is in progress at the time of the write. After that write, both pairs diverge, and both fields are locked until both transitions complete.

A validating admission webhook (or CEL validation rule) rejects writes to either spec topology field when any spec/status pair diverges. This is about field immutability during transition, not transition eligibility — it does not contradict the "no x-validation for transition eligibility" decision. Once set, neither spec value can change until the transition is completed.

**Cancellation is not supported.** There is no mechanism to cancel or withdraw a transition mid-flight. The administrator must wait for the transition to complete before expressing new intent. This avoids partial-state problems where some operators have already reconciled to the new topology and others have not — there is no rollback mechanism for operators that have already acted. After completion, once both spec/status pairs match again, the fields are mutable and the administrator can request a new transition.

##### Invariants

The following invariants hold throughout the entire infrastructure transition lifecycle:

- `status.controlPlaneTopology` MUST NOT change as a result of an infrastructure topology transition
- `spec.controlPlaneTopology` MUST NOT be modified by the infrastructure transition path
- etcd membership (member count, learner/voter status, quorum) MUST NOT change
- The transition MUST NOT add, remove, resize, or reconfigure control-plane and/or worker nodes
- Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable` — enforced by CCO through the available transitions list, not by API-level validation
- The control-plane transition path on compact clusters MUST explicitly set `spec.infrastructureTopology` when transitioning to HA, rather than treating infrastructure topology as an implicit side effect. The infrastructure controller then reconciles `status.infrastructureTopology` through its own lifecycle.

### API Extensions

#### Infrastructure API Changes

This enhancement modifies the existing infrastructure CR (`infrastructures.config.openshift.io`) following the standard Kubernetes spec/status contract:

**Spec (user intent):**

A `controlPlaneTopology` field is added to `InfrastructureSpec` to express the administrator's intent to transition the control plane, and an `infrastructureTopology` field is added to express the intent to transition infrastructure topology independently:

```go
type InfrastructureSpec struct {
	CloudConfig  ConfigMapFileReference `json:"cloudConfig"`
	PlatformSpec PlatformSpec           `json:"platformSpec,omitempty"`

	// controlPlaneTopology expresses the administrator's intent for the
	// cluster's control plane topology. Empty by default — the field is
	// unset until an administrator explicitly initiates a transition.
	// When set and the value differs from status.controlPlaneTopology,
	// the topology transition controller in cluster-config-operator
	// initiates a transition. An empty value means no transition has
	// been requested.
	// +optional
	// +openshift:enable:FeatureGate=MutableTopology
	ControlPlaneTopology TopologyMode `json:"controlPlaneTopology,omitempty"`

	// infrastructureTopology expresses the administrator's desired
	// infrastructure topology — how infrastructure workloads are
	// distributed across nodes. When omitted, no transition override
	// has been expressed. When the MutableTopology feature gate is
	// enabled, CCO populates this field to match
	// status.infrastructureTopology if it is omitted. Once set, the
	// field represents the desired topology state. When the value
	// differs from status.infrastructureTopology, the topology
	// transition controller in cluster-config-operator evaluates
	// whether the transition is allowed based on the available
	// transitions list.
	//
	// The API accepts any valid enum value. Transition eligibility
	// is enforced by CCO, not by API-level x-validation rules.
	// +optional
	// +openshift:enable:FeatureGate=MutableTopology
	InfrastructureTopology TopologyMode `json:"infrastructureTopology,omitempty"`
}
```

The `controlPlaneTopology` field is empty by default — the installer does not populate it. An empty `spec.controlPlaneTopology` on an existing or upgraded cluster indicates that no transition has ever been requested. After a successful transition, the field remains set (e.g., `HighlyAvailable`) and matches `status.controlPlaneTopology` — the controller is idle.
This makes it straightforward to distinguish clusters that have undergone a transition (field set, matches status) from those that have not (field empty). A transition is initiated when the administrator sets `spec.controlPlaneTopology` to a value that differs from `status.controlPlaneTopology`.

The `infrastructureTopology` field is `+optional` with `omitempty`. There is a difference between **omitted** (field not present in JSON — valid, means no override expressed) and **empty string** (`""` — rejected by kubebuilder enum validation, not a valid `TopologyMode` value). This follows the standard Infrastructure resource pattern for fields that allow a user to "override" existing behavior.

- **Before MutableTopology is enabled:** The field is omitted. CCO does not populate it. No transition is possible.
- **When MutableTopology is enabled and the field is omitted:** CCO detects that the gate is enabled and the field is omitted, and sets it to match `status.infrastructureTopology`. This covers both new installs (where the installer could also set it) and upgrades from versions that predate this field. Since spec matches status after population, no transition is triggered.
- **Once set:** The field represents the administrator's desired topology. When spec diverges from status, the controller evaluates and admits the transition. After a successful transition, spec and status match again and the controller is idle.

This is the same pattern as other opt-in override fields in the Infrastructure resource: omitted means "no override," and the controller populates it when the feature is active.

Both spec fields reuse the existing `TopologyMode` type — no separate named types are introduced. The `TopologyMode` enum (`SingleReplica`, `HighlyAvailable`, `DualReplica`, `HighlyAvailableArbiter`) provides API-level validation. The set of transitions that CCO actually supports is a subset of valid enum values — for the initial implementation, only `SingleReplica` and `HighlyAvailable` are supported. Additional transitions can be enabled via CCO updates without API changes.

**Mapping to status fields**: `spec.controlPlaneTopology` expresses intent for the control plane topology only. The controller derives the corresponding `mastersSchedulable` value based on the transition definition.
For the initial SNO → HA compact transition: `controlPlaneTopology` transitions to `HighlyAvailable`, and `mastersSchedulable` remains `true` (it is already `true` on SNO clusters since the single node runs all workloads; it stays `true` for compact clusters). On compact clusters, when `spec.infrastructureTopology` is unset, the control-plane controller explicitly sets `spec.infrastructureTopology = HighlyAvailable` during a control-plane transition. The infrastructure controller then picks up the spec/status divergence and reconciles `status.infrastructureTopology` through its own prerequisite validation and lifecycle. This makes infrastructure topology intent explicit rather than a side effect.

`spec.infrastructureTopology` expresses intent for infrastructure topology independently. The infrastructure controller reconciles `status.infrastructureTopology` toward this value when prerequisites are met, regardless of whether the spec was written by the control-plane path (compact clusters) or by an administrator (standalone path). This field does not affect `status.controlPlaneTopology` or etcd.

**No cross-field API validation**: There is no cross-field validation between `spec.controlPlaneTopology` and `spec.infrastructureTopology` at the API admission level — no x-validation rules are defined on these fields. The API accepts any valid enum value for either field. All transition eligibility checks — including the CP=HA prerequisite, worker node readiness, platform support, and upgrade state — are enforced by CCO through the available transitions list. This is a deliberate design choice: transition eligibility can be adjusted by updating CCO without API contract changes.

**Allowed infrastructure transitions:**

| Source `status.infrastructureTopology` | `spec.infrastructureTopology` | Behavior |
|---|---|---|
| `SingleReplica` | `HighlyAvailable` | **Transition admitted** if `status.controlPlaneTopology = HighlyAvailable` and worker node prerequisites pass (Ready, schedulable, sufficient count) |
| `HighlyAvailable` | `HighlyAvailable` | Spec matches status — controller idle |
| `HighlyAvailable` | `SingleReplica` | **Rejected** — reverse transition not supported |
| `SingleReplica` | `SingleReplica` | Spec matches status — controller idle |
| Any | omitted | CCO populates to match `status.infrastructureTopology` when `MutableTopology` gate is enabled (see omitted-to-populated lifecycle above) |
| Any | Any (HyperShift) | **Rejected** — not applicable |
| Any | Any (IBI) | **Rejected** — not supported |

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

**Transition progress** for the infrastructure path follows the same pattern as the control-plane path — reported via conditions on the CCO `ClusterOperator` status with `InfrastructureTopology*` reason prefixes to distinguish infrastructure transitions from control-plane transitions. The controller sets `Upgradeable=False` during active transitions to prevent CVO from starting an upgrade.

**Retry**: Because the spec field persists, retry is automatic. If a prerequisite was temporarily lost (e.g., a worker node became NotReady), the controller detects when it recovers and resumes. No manual retry action is needed.

There is no separate condition for etcd scaling — that scaling is a precondition the controller checks, not a state it tracks or reports on directly (see [Failure Handling](#failure-handling)). Reason values will be refined during dev preview implementation.

**Observed status fields during transition:**

| Field | Behavior During Transition |
|---|---|
| `status.infrastructureTopology` | Remains observed state. NOT updated when the administrator sets `spec.infrastructureTopology`. Updated only when the controller has successfully reconciled the infrastructure topology to the target value. |
| `status.controlPlaneTopology` | Invariant. Not modified by the infrastructure transition path. |
| `spec.infrastructureTopology` | Retains the administrator's declared desired state. Persists across restarts and upgrades. Omitted until MutableTopology is enabled; CCO then populates to match status. Immutable during transitions. |
| `spec.controlPlaneTopology` | Not modified by the infrastructure transition path. Independent field. |

**Concurrent transitions**: The control-plane and infrastructure transition graphs are architecturally independent — each has its own rules for admission and reconciliation. Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable`, so a concurrent CP+infra transition from SingleReplica is not possible — the CP transition must complete first. When the CLI runs `oc adm transition topology --control-plane=HighlyAvailable --confirm`, it sets both `spec.controlPlaneTopology = HighlyAvailable` and `spec.infrastructureTopology = HighlyAvailable` in a single patch. CCO evaluates them independently — but because the infrastructure path requires CP=HA, the infrastructure transition waits until the CP transition completes.

#### Admission Control

**Spec validation**: Both `spec.controlPlaneTopology` and `spec.infrastructureTopology` use the existing `TopologyMode` type. The API server rejects values not in the `TopologyMode` enum at admission time via kubebuilder enum validation. The set of transitions that CCO actually supports is a subset of valid enum values — unsupported transitions are rejected by CCO at reconciliation time, not by the API server.

**No cross-field admission for transition eligibility**: There is no cross-field validation between `spec.controlPlaneTopology` and `spec.infrastructureTopology` for transition eligibility at the API admission level. All transition eligibility checks are enforced by CCO at reconciliation time. See [No cross-field validation](#infrastructure-api-changes) for rationale.

**Immutability during transition**: A validating admission webhook (or CEL validation rule) rejects writes to either `spec.controlPlaneTopology` or `spec.infrastructureTopology` when any spec/status pair diverges. This is about field immutability during transition, not transition eligibility. Both fields can be set together in a single write when no transition is in progress. See [Immutability during transition](#immutability-during-transition) in the infrastructure workflow for details.

Access to `spec.controlPlaneTopology` and `spec.infrastructureTopology` is governed by the existing RBAC for the infrastructure CR (`infrastructures.config.openshift.io`). By default, only users with `cluster-admin` or equivalent roles can modify infrastructure spec fields.
No additional RBAC restrictions are proposed for the initial implementation; a dedicated role for topology transitions may be considered in future iterations if finer-grained access control is needed.

**Status fields**: The existing topology status fields (`controlPlaneTopology`, `infrastructureTopology`, `mastersSchedulable`) are not protected by admission policies. This is consistent with other infrastructure status fields — no special protection exists for them today. An administrator who deliberately modifies these values outside the transition controller does so at their own risk.

**Validation behavior summary:**

| Condition | Enforced By | Result |
|---|---|---|
| `spec.infrastructureTopology` set to unsupported enum value | API server | Rejected (kubebuilder enum validation) |
| `MutableTopology` gate disabled | API server | Field not accepted (feature-gated) |
| Either spec topology field changed while any transition is in progress (any `spec.xTopology != status.xTopology`) | API server | Rejected by validating admission webhook — both fields are immutable until both spec/status pairs match |
| `status.controlPlaneTopology != HighlyAvailable` (infra transition) | CCO | Not admitted — infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable` |
| Unsupported transition direction (e.g., HA → SingleReplica) | CCO | Controller records `PreflightCheckFailed` reason; transition not admitted |
| Missing prerequisites (insufficient/NotReady worker nodes) | CCO | Controller reports missing prerequisites via conditions; transition not admitted |
| Unsupported platform | CCO | Controller records `UnsupportedPlatform` reason; transition not admitted |
| Incompatible upgrade in progress | CCO | Controller records `UpgradeInProgress` reason; transition not admitted |
| Cluster is un-upgradeable (CVO `Upgradeable=False`) | CCO | Controller records `ClusterNotUpgradeable` reason; transition not admitted |
| Spec matches status (same-value or already completed) | CCO | Controller idle — no action taken |
| Controller crash during transition | CCO | On restart, controller re-reads spec vs. status and resumes reconciliation |

#### Feature Gate

A new feature gate `MutableTopology` will be added to gate this functionality. Both `spec.controlPlaneTopology` and `spec.infrastructureTopology` are gated by `MutableTopology` — no separate feature gate is introduced for the infrastructure path. The feature gate will progress through the following stages:

- **Dev Preview**: Part of the `DevPreviewNoUpgrade` feature set
- **Tech Preview**: Moved to the `TechPreviewNoUpgrade` feature set
- **GA**: Moved to the `Default` feature set

### Implementation Details/Notes/Constraints

#### Topology Transition Controller

A new topology transition controller is added to cluster-config-operator with the following characteristics:

- Watches the infrastructure CR for `spec.controlPlaneTopology` diverging from `status.controlPlaneTopology` and `spec.infrastructureTopology` diverging from `status.infrastructureTopology`
- Gated by the `MutableTopology` feature gate — inactive when the gate is disabled
- Maintains the set of supported transitions (initially: SingleReplica → HighlyAvailable on `platform: none` for both control-plane and infrastructure paths)
- Validates preconditions before starting a transition — each path validates its own prerequisites independently
- Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable`; concurrent CP+infra transitions from SingleReplica are not possible — the CP transition must complete first
- Updates `controlPlaneTopology` and `infrastructureTopology` in status once preconditions pass
- Reports transition progress via CCO ClusterOperator status conditions

##### Supported Transitions

For the initial implementation:

```text
Control Plane:     SingleReplica (SNO, platform: none) → HighlyAvailable (3-node compact)
Infrastructure:    SingleReplica (platform: none, dedicated workers present) → HighlyAvailable
```

Future transitions can be added without modifying the core controller logic. Each supported transition defines:

- **Preconditions**: What must be true before the transition can start
- **Orchestration steps**: What the controller coordinates during the transition
- **Validation criteria**: What must be true after the transition for it to be considered complete

##### Transition Orchestration: Control-Plane Path

The controller reconciles `spec.controlPlaneTopology` against `status.controlPlaneTopology` on every sync. For the SNO → HA compact transition:

**Preconditions** (all must hold before a transition is accepted):

- Every ClusterOperator other than cluster-config-operator itself reports `Available=True`, `Progressing=False`, `Degraded=False`
- Exactly 3 control-plane-labeled nodes exist, all schedulable and `Ready`, and no dedicated worker nodes are present
- etcd has quorum and is not mid-scaling, and 3 voting members are already recorded for it
These preconditions mean the administrator's node join and CEO's existing node-driven etcd scaling must already be complete — the controller does not itself trigger or wait on etcd scaling as part of the transition; it only accepts the transition once that has already happened.

**Orchestration steps** (once preconditions pass):

1. Set `Upgradeable=False` and a `Progressing` condition on the CCO `ClusterOperator` in the same update, so CVO cannot start an upgrade while the transition is applied
2. Re-read the Infrastructure CR and confirm the requested spec has not changed since preconditions were checked
3. Update `status.controlPlaneTopology` to `HighlyAvailable`. If `spec.infrastructureTopology` is unset, explicitly set `spec.infrastructureTopology = HighlyAvailable`. The infrastructure controller then picks up the spec/status divergence and reconciles `status.infrastructureTopology` through its own prerequisite validation and lifecycle.
4. On later syncs, wait a soak period (5 minutes) after the `Progressing` condition was set, then begin checking post-transition validation criteria

If no supported transition matches, or a precondition fails, the controller records the reason on the `Progressing`/`Upgradeable` conditions rather than erroring. The administrator must address the failing prerequisite so the transition can proceed — the spec field is immutable during transition (see [Immutability during transition](#immutability-during-transition)).

**Validation criteria** (checked once the soak period has elapsed; all must pass to consider the transition complete):

- 3 control-plane nodes remain schedulable and `Ready`, and a minimum of 2 worker-labeled nodes, including dual-role compact nodes, are `Ready`.
- etcd retains quorum, is not mid-scaling, and reports 3 voting members
- The SNO-only master MachineConfig has been removed and a new rendered master/worker MachineConfigs exist, with the master MachineConfigPool reporting 3 ready machines
- The default IngressController reports a minimum of 2 available router replicas
- kube-apiserver reports status for 3 nodes and openshift-apiserver reports 3 ready replicas

Once all criteria pass, the controller clears `Progressing` and sets `Upgradeable=True`.

##### Transition Orchestration: Infrastructure Path

The controller reconciles `spec.infrastructureTopology` against `status.infrastructureTopology` on every sync. For the standalone infrastructure SingleReplica → HA transition:

**Preconditions** (all must hold before a transition is accepted):

- Every ClusterOperator other than cluster-config-operator itself reports `Available=True`, `Progressing=False`, `Degraded=False`
- Sufficient worker-capable nodes (any node with `node-role.kubernetes.io/worker` label, including dual-role nodes) are `Ready`, schedulable, not blocked by incompatible taints, and able to host the expected infrastructure replicas. The exact minimum worker count and capacity check must be agreed during implementation.
- The platform is supported (`platform: none` initially)
- No incompatible upgrade is in progress

The infrastructure controller does not perform cross-field API validation against `spec.controlPlaneTopology`. However, infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable` — this is enforced via the available transitions list, not API x-validation.

**Orchestration steps** (once preconditions pass):

1. Set `Upgradeable=False` and a `Progressing` condition (`reason: InfrastructureTopologyTransitionInProgress`) on the CCO `ClusterOperator` in the same update
2. Re-read the Infrastructure CR and confirm `spec.infrastructureTopology` has not changed since preconditions were checked
3. On later syncs, wait a soak period (5 minutes) after the `Progressing` condition was set, then begin checking post-transition validation criteria

If no supported transition matches, or a precondition fails, the controller records the reason on the `Progressing`/`Upgradeable` conditions (e.g. `PreflightCheckFailed`, `UnsupportedPlatform`, `UpgradeInProgress`). The administrator must address the failing prerequisite so the transition can proceed — the spec field is immutable during transition.

**Validation criteria** (checked once the soak period has elapsed; all must pass to consider the transition complete):

- Worker nodes remain `Ready` and schedulable
- Infrastructure operators have converged to HA replica counts:
  - Ingress Operator: default IngressController reports a minimum of 2 available router replicas
  - Monitoring: Prometheus and Alertmanager at HA replicas
  - Image Registry: registry replicas scaled
  - Console: console replicas scaled
  - OAuth: OAuth server replicas scaled
- `status.controlPlaneTopology` is unchanged
- etcd membership is unchanged

Once all criteria pass, the controller updates `status.infrastructureTopology` to `HighlyAvailable`, clears `Progressing`, and sets `Upgradeable=True`.

**Invariants enforced by the infrastructure path**:
- The controller MUST NOT modify `spec.controlPlaneTopology` or `status.controlPlaneTopology`
- The controller MUST NOT add, remove, resize, or configure control-plane nodes
- The controller MUST NOT trigger, sequence, or recover etcd membership changes
- Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable` — enforced via the available transitions list

##### Discovery

CCO evaluates the current cluster state and advertises available transitions through the Infrastructure status. For the infrastructure path, the controller populates `status.infrastructureTopologyTransitions` — a list of available infrastructure transitions the controller has computed, mirroring the `status.controlPlaneTopologyTransitions` structure:

```yaml
status:
  infrastructureTopologyTransitions:
    - source: SingleReplica
      target: HighlyAvailable
      available: true
      reason: "TransitionAvailable"
      message: "3 worker-capable nodes are Ready and schedulable"
```

Each entry describes a source/target pair with availability, reason, and message. CCO computes and updates this list on every sync. `oc adm transition topology` consumes this discovery information to present available transitions — the CLI does not duplicate server-side validation.

Discovery updates when: worker readiness changes, CP topology changes, upgrades start/complete, cluster upgradeable state changes, or feature gate state changes.

#### `oc adm transition topology` CLI Command

The CLI command provides an interface for topology transitions:

- Discovers available transitions from the Infrastructure status
- Validates preconditions client-side (feature gate enabled, no transition in progress)
- Without `--confirm`, displays prerequisites and their status as a preview of what would change; with `--confirm`, admits the transition
- Supports `--allow-transition-with-warnings` for non-blocking prerequisites
- Uses explicit flags for transition targets: `--control-plane=HighlyAvailable` and `--infrastructure=HighlyAvailable`
- Patches `spec.controlPlaneTopology` or `spec.infrastructureTopology` on the infrastructure CR depending on the transition type
- Returns immediately after a successful patch
- Displays transition progress through a `status` subcommand, reading `TopologyTransitionControllerProgressing` conditions with `InfrastructureTopology*` reasons

An environment variable also gates the `oc adm transition topology` commands from being available by default, alongside the `MutableTopology` feature gate.

For infrastructure-only transitions, the CLI supports `oc adm transition topology --infrastructure=HighlyAvailable --confirm`. The CLI auto-detects available transitions via the discovery contract based on cluster state.

The CLI does not contain transition logic and does not duplicate server-side validation or write observed status — it delegates entirely to the CCO controller. This follows the same pattern as `oc adm upgrade`, which patches `spec.desiredUpdate` and lets the CVO do the work.
Administrators monitor transition progress separately via `oc get clusteroperator cluster-config-operator -o yaml` or a dedicated `oc adm transition topology status` subcommand (exact UX to be determined during dev preview).

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

**Note**: etcd scaling is only relevant to the control-plane transition path. The standalone infrastructure transition does not involve etcd changes.

#### Component Changes Summary

| Component | Changes Required |
| --------- | ---------------- |
| cluster-config-operator | Topology transition controller; watches `spec.controlPlaneTopology` and `spec.infrastructureTopology`, coordinates transitions, updates status topology fields |
| Infrastructure API (`openshift/api`) | Add `controlPlaneTopology` and `infrastructureTopology` to `InfrastructureSpec` using `TopologyMode`; update immutability documentation on status topology fields |
| `oc` CLI | `oc adm transition topology` command with `--infrastructure` flag; discovers available transitions from Infrastructure status |
| cluster-etcd-operator | No code changes — its existing node-driven (unsafe) etcd scaling behavior is depended on as a precondition the control-plane transition controller checks for |
| Infrastructure operators (Ingress, Monitoring, Image Registry, Console, OAuth) | Reconcile on `status.infrastructureTopology` changes — scale workloads to HA replica counts when infrastructure topology transitions to `HighlyAvailable` |

#### Ownership Boundaries

| Component | Owner | Responsibilities |
|---|---|---|
| **API contract** (`spec.controlPlaneTopology`, `spec.infrastructureTopology`) | openshift/api maintainers | Define spec fields using `TopologyMode` with enum validation, feature gate, and condition semantics |
| **CCO Topology Transition Controller** | cluster-config-operator maintainers | Watch both spec fields for divergence, server-side admission, prerequisites validation, reconciliation, lifecycle reporting via CCO ClusterOperator conditions |
| **`oc adm transition topology` CLI** | openshift/oc maintainers | Discover available transitions, patch spec fields, display progress. Thin client — does not perform admission or write status |
| **Infrastructure operators** | Individual operator teams | Own reconciliation of their workloads when `status.infrastructureTopology` changes. React to status changes. Do not own the transition lifecycle |
| **Control-plane and etcd components** | CEO, kube-apiserver, etc. | Remain outside the infrastructure transition path. Not affected by infrastructure transitions |

**Boundary rules:**

- The infrastructure transition path MUST NOT modify `spec.controlPlaneTopology` or `status.controlPlaneTopology`.
- The infrastructure transition path MUST NOT add, remove, resize, or configure control-plane nodes.
- The infrastructure transition path MUST NOT trigger, sequence, or recover etcd membership changes.
- Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable`. Concurrent CP+infra transitions from SingleReplica are not possible — the CP transition must complete first.
- The CLI MUST NOT duplicate server-side validation or write observed status.
- Infrastructure operators react to `status.infrastructureTopology` changes; they do not own or drive the transition lifecycle.

#### Platform Support Constraints

See [Standalone Clusters](#standalone-clusters) for platform support details. The initial implementation targets `platform: none` only for both transition paths; `platform: baremetal` and cloud platforms are future work.

The topology transition controller checks for Node objects in the API regardless of how they were provisioned.

#### Worker Node Prerequisites (Infrastructure Path)

The infrastructure topology transition validates that the cluster has enough nodes capable of hosting infrastructure workloads at HA replica counts. A **worker node** eligible for this check is any node that:
- Has the `node-role.kubernetes.io/worker` label. Nodes that also carry `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master` labels (dual-role nodes) are included — what matters is whether the node can schedule infrastructure workloads, not whether it is exclusively a worker.
- Is `Ready`.
- Is schedulable (not cordoned or draining).
- Is not blocked by incompatible taints or lifecycle state.
- Is able to host the expected infrastructure replicas.

The exact minimum worker node count, capacity check, `Upgradeable` requirement, and operator-health threshold must be explicitly agreed by API/CCO owners.

The infrastructure path MUST NOT import compact SNO → HA etcd orchestration checks. It must preserve `controlPlaneTopology` and etcd membership.

#### Operator Support Matrix (Infrastructure Path)

The following operators are expected to react to `status.infrastructureTopology` changes from `SingleReplica` to `HighlyAvailable` by scaling their workloads to HA replica counts:

| Operator | Expected Behavior | Notes |
|---|---|---|
| **Ingress Operator** | Scale default IngressController replicas from 1 to 2+ | Topology-aware replica logic needs validation |
| **Monitoring** | Scale Prometheus, Alertmanager to HA replicas | Watches infrastructure topology |
| **Image Registry** | Scale registry replicas | Watches infrastructure topology |
| **Console** | Scale console replicas | Watches infrastructure topology |
| **OAuth** | Scale OAuth server replicas | Watches infrastructure topology |

**Ingress Operator note**: The Ingress Operator's topology-aware replica logic needs validation to confirm it correctly reacts to `status.infrastructureTopology` changes. If the current implementation does not scale replicas on topology change, the Ingress Operator must be updated to support this behavior.

The per-operator topology dependency audit and the exact operator set must be confirmed during implementation.

### Risks and Mitigations

#### Risk: Quorum Loss During Two-Member Transient State

**Risk**: During sequential etcd scaling (1→2→3), the cluster passes through a 2-member state where quorum=2. Losing either member during this window is fatal — the cluster loses its API and requires manual recovery. (Applies to control-plane transitions only.)

**Mitigation**:
- The 2-member state is transient and the learner-to-voter promotion mechanism is reused from cluster bootstrapping — a well-exercised code path
- Learner instances are used before promoting members to minimize the promotion window
- No availability guarantee during transitions; administrators should treat scaling operations as a maintenance window
- If etcd scaling fails during the 2-member window, quorum is lost and manual recovery via `quorum-restore.sh` is required
- Future iterations may explore admitting two learners simultaneously and promoting only when both are ready, eliminating the 2-member voting window entirely, but that is out of scope for this enhancement

#### Risk: Transition Fails Partway Through

**Risk**: A transition may fail after some operators have begun reconfiguring but before the transition completes, leaving the cluster in an intermediate state. Examples of failures:

- **etcd quorum loss** (control-plane path only): during the node-driven prerequisite scaling (before the controller admits the transition), etcd scales to 2 members, a network partition occurs between them, both lose quorum, and the API becomes unavailable. This requires manual recovery via `quorum-restore.sh`, independent of the topology transition controller.
- **Node readiness**: a new control-plane node becomes `NotReady` during the transition (e.g., disk pressure, network misconfiguration), preventing etcd or static pods from starting.
- **Operator reconciliation failure**: after topology status fields are updated, an operator fails to reconcile (e.g., ingress pod fails to schedule on a new node due to resource constraints or `ImagePullBackOff`).

**Mitigation**:
- The controller only admits a transition once its preconditions — including etcd already having quorum and 3 voting members (control-plane path) or sufficient ready workers (infrastructure path) — are satisfied
- Operators do not see a topology change until the controller updates the infrastructure status
- Etcd scaling failures (including quorum loss) are cluster-etcd-operator's existing failure domain; the transition controller withholds admission but provides no additional recovery guarantees for them. Quorum loss requires manual recovery via `quorum-restore.sh`
- CCO ClusterOperator status conditions provide detailed state for troubleshooting precondition and post-admission failures

#### Risk: Infrastructure Operators May Not React to Topology Changes

**Risk**: Infrastructure operators that do not watch `status.infrastructureTopology` for changes, or that have bugs in their topology-aware scaling logic, may not adjust replica counts after a standalone infrastructure transition. The Ingress Operator in particular needs validation that its topology-aware replica logic correctly handles runtime topology changes.

**Mitigation**:
- The per-operator topology dependency audit is a prerequisite for entering dev preview
- The post-transition soak validates that each expected operator has converged to HA replicas before the transition is considered complete
- The Ingress dependency is documented and tracked separately
- Operators that read topology only at startup (rather than watching) are identified during the audit and a restart strategy is documented

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

The SNO-to-HA transition requires coordination with CEO, ingress, networking, and other operator teams to ensure they reconcile correctly when topology status fields change. The infrastructure transition additionally requires coordination with Ingress, Monitoring, Image Registry, Console, and OAuth operator teams. This is less coordination than the previous Adaptable Topology approach (which required every operator to handle dynamic node-count awareness), but still significant.

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

The initial implementation supports only SNO → HA compact (control plane) and SingleReplica → HA (infrastructure). Reverse transitions (HA → SNO, HA infra → SingleReplica infra) and other paths are future work. Administrators who transition cannot revert without redeploying. Mechanisms will be put in place to gate the transition path at every level of the implementation (CLI, CCO Controller, API).

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

### Action-Oriented Infrastructure Transition Request (Not Selected)

An alternative for the standalone infrastructure transition was to represent it as a one-time action-oriented request (annotation or status sub-resource) rather than a durable `spec.infrastructureTopology` field.

**Why it was rejected**:
- Introduces a fundamentally different intent mechanism alongside the spec/status pattern established by `spec.controlPlaneTopology`. Two different intent mechanisms for the same class of operation increases cognitive load and implementation divergence.
- Annotations are untyped, unversioned, and not subject to API validation. A structured annotation can be malformed in ways that a typed spec field cannot.
- Without a persistent spec field, crash recovery requires the controller to maintain out-of-band state. The spec/status divergence model handles this naturally.
- Mid-transition behavior is unclear: there is no spec to lock. The controller must invent its own mechanism to prevent changes during an in-flight transition.
- This model trades API discipline for short-term implementation convenience, contradicting the design direction established by `spec.controlPlaneTopology`.

## Open Questions

1. **OLM operator impact**: Which OLM-managed operators read topology values? Do they watch the infrastructure CR or read at startup only? This determines whether operators need code changes or just a restart after transition.

2. **Per-operator transition behavior**: The transition behavior for CEO is understood (etcd sequential scaling). The specific requirements for ingress, networking, monitoring, and other operators during a topology transition need validation during dev preview. The per-operator topology dependency matrix is a prerequisite for entering dev preview — see [Graduation Criteria](#entering-dev-preview).

3. **Minimum resource requirements**: The controller should validate that new control-plane nodes meet minimum resource requirements before initiating a transition. The specific resource thresholds need to be defined.

4. **Backup compatibility across topologies**: If an administrator takes an etcd backup on a SNO cluster and later transitions to HA, is the pre-transition backup usable for restore on the post-transition cluster? A new backup should be taken after a successful transition, but the interaction between pre-transition backups and post-transition cluster state needs investigation.
   Ideally restoring the pre-transition backup would revert the cluster to SNO, but that flow needs to be validated.

5. **OLM operator topology declarations**: A future enhancement should define an OLM manifest mechanism allowing operators to declare which topologies they support, so the transition controller and catalog tooling don't have to optimistically assume support by default. See [Optional (OLM-Managed) Operators and Topology Changes](#optional-olm-managed-operators-and-topology-changes).

6. **Minimum worker node count for infrastructure transitions**: The exact minimum worker count required for a standalone infrastructure topology transition needs to be defined and agreed by API/CCO owners. This includes whether the threshold should be a fixed number or derived from the expected HA replica counts of infrastructure operators.

7. **Operator support matrix confirmation**: The exact set of operators that react to `status.infrastructureTopology` changes and their expected HA behaviors must be confirmed during implementation. The Ingress Operator's topology-aware replica logic needs validation.

8. **Discovery contract shape**: The `status.controlPlaneTopologyTransitions` and `status.infrastructureTopologyTransitions` discovery fields must be agreed with API reviewers. The infrastructure path mirrors the control-plane path structure. This includes the source/target/available/reason/message shape and how the CLI consumes this information.

## Test Plan

### CI Lanes

| Lane | Frequency | Description |
| ---- | --------- | ----------- |
| MutableTopology CP transition suite | Nightly | Run control-plane transition test suite: SNO → HA compact on `platform: none` |
| MutableTopology infra transition suite | Nightly | Run infrastructure transition test suite: SingleReplica → HA on `platform: none` with dedicated workers |
| End-to-End tests (e2e) | Weekly | Standard test suite (openshift/conformance/parallel) on post-transition clusters |
| Upgrade between z-streams | Weekly | Test upgrades on post-transition clusters (both CP and infra transitions) |
| Upgrade between y-streams | Weekly | Test upgrades across minor versions on post-transition clusters |

### CI Tests

#### Pre-Transition Tests

| Test | Description |
| ---- | ----------- |
| CP precondition validation | Verify the controller withholds admission when nodes are missing/not-ready, dedicated workers are present, cluster operators are unstable, or etcd has not yet reached quorum with 3 voting members |
| Infra precondition validation | Verify the controller withholds admission when workers are insufficient/not-ready, unsupported platform, or incompatible upgrade in progress |
| CP=HA prerequisite | Verify that infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable`, enforced by CCO (not API x-validation) |
| Concurrent transitions | Verify concurrent CP+infra transitions from SingleReplica are blocked (CP must be HA first). Verify both spec fields can be set in a single write when no transition is in progress. |
| Immutability during transition | Verify that neither spec topology field can be changed while any transition is in progress |
| CLI interaction | Verify `oc adm transition topology` correctly patches `spec.controlPlaneTopology` and `spec.infrastructureTopology` and monitors progress |

#### Transition Tests

| Test | Description |
| ---- | ----------- |
| SNO → HA compact (3-node) | Full control-plane transition on `platform: none`: node-driven etcd scaling completes first, then the controller admits the transition and updates infrastructure status |
| Infra SingleReplica → HA | Full infrastructure transition on `platform: none`: controller validates workers, admits transition, infrastructure operators converge to HA replicas |
| etcd quorum as precondition | Verify CEO's existing 1→2→3 member addition completes independently of the CLI command, and that the controller does not admit the transition until it has |
| CP failure and recovery | Verify the controller withholds admission indefinitely when a precondition never becomes true, and that CEO's own etcd disaster recovery procedures are unaffected |
| Infra failure and recovery | Verify the controller withholds admission when workers are insufficient, and that admission proceeds automatically when workers become Ready |
| Post-transition CP operator health | Verify all operators reconcile successfully after control-plane topology status fields are updated |
| Post-transition infra operator health | Verify infrastructure operators (Ingress, Monitoring, Image Registry, Console, OAuth) converge to HA replicas after `status.infrastructureTopology` changes |
| CP path sets spec.infrastructureTopology | Verify that on compact clusters, the CP transition path explicitly sets `spec.infrastructureTopology = HighlyAvailable` when it is unset |
| Crash recovery | Kill CCO during either transition type, verify controller resumes from spec/status divergence |
| Idempotent behavior | Verify that spec matching status (no-op) and repeated spec writes do not trigger re-transitions |
| Immutability enforcement | Verify that spec topology fields are immutable during transition — webhook rejects changes while any spec/status pair diverges |
| Infra controller spec-origin agnostic | Verify that the infrastructure controller reconciles `status.infrastructureTopology` regardless of whether the spec was written by the CP path (compact cluster) or by an administrator (standalone path) |

### QE Testing

Standard QE testing scenarios will include:
- Full SNO → HA compact transition on `platform: none`
- Full standalone infrastructure transition on `platform: none` with dedicated workers
- Transition failure and recovery scenarios for both paths
- Post-transition cluster stability over 24 hours
- Destructive testing: control-plane node failure during the 2-member etcd window
- Network partition scenarios during transition (e.g., partition between etcd members during scaling)
- Concurrent operation testing: transition + upgrade attempt (verify `Upgradeable=False` blocks upgrades)
- Concurrent operation testing: verify concurrent CP+infra transitions from SingleReplica are blocked (CP must be HA first)
- Immutability testing: verify spec topology fields cannot be changed while any transition is in progress
- Node resource exhaustion during transition (e.g., insufficient disk or memory on new control-plane nodes)
- Worker node loss after completed infrastructure transition (verify status stays HA, degradation reported)
- Backup pre-transition and then restore that backup post-transition
- Upgrade gating: CVO cannot start an upgrade while either transition is in progress
- Post-transition upgrade: cluster upgraded after successful transition preserves topology status

## Graduation Criteria

### Entering Dev Preview

- Manual SNO-to-HA transition tested (scaling a single-replica cluster to multiple replicas) to validate assumptions about operator behavior
- Topology transition controller implemented in cluster-config-operator with SNO → HA compact support
- `controlPlaneTopology` field added to `InfrastructureSpec`
- `oc adm transition topology` CLI command implemented
- `MutableTopology` feature gate added to `DevPreviewNoUpgrade` feature set
- `TopologyMode` enum validated for spec fields in API integration tests
- Per-operator topology dependency matrix completed: for each in-payload operator that reads `controlPlaneTopology` or `infrastructureTopology`, document what the operator uses the value for (replica count, scheduling, feature enablement) and whether it watches the infrastructure CR for changes or reads the value only at startup
- Operators that read topology only at startup are identified and a restart strategy is documented for post-transition reconciliation
- CCO sets `Upgradeable=False` on its ClusterOperator while a topology transition is in progress
- Valid and invalid cluster transitions are identified in the the infrastructure status (discovery contract)
- CI lanes operational for transition testing
- Developer documentation available
- `infrastructureTopology` field added to `InfrastructureSpec`
- `TopologyMode` enum validated for `spec.infrastructureTopology` in API integration tests
- Standalone infrastructure transition controller path implemented with CP=HA prerequisite enforcement
- `oc adm transition topology --infrastructure` CLI support implemented
- Infrastructure transition CI lane operational

### Dev Preview -> Tech Preview

- Control-plane transition test suite validates full SNO → HA compact path
- Infrastructure transition test suite validates full SingleReplica → HA path with dedicated workers
- Tests verify operator health during and after both transition types
- Controller failure handling validated; etcd disaster recovery procedures documented for quorum loss scenarios
- `oc adm transition topology` command provides clear diagnostics on failure for both paths
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/)
- End-to-end validation that CLI correctly patches both spec fields and the controller rejects unsupported transitions
- CP=HA prerequisite validated end-to-end (infra transitions blocked when CP is not HA)
- Immutability during transition validated end-to-end (webhook rejects changes while any spec/status pair diverges)
- Infrastructure controller reconciles `status.infrastructureTopology` regardless of spec write origin (CP path on compact clusters vs. administrator on standalone path) validated end-to-end
- Infrastructure operator convergence validated (Ingress, Monitoring, Image Registry, Console, OAuth)
- **Dependency**: Platform bare metal single-node support status assessed with the Bare Metal Networking team. If keepalived cannot be configured for single-node clusters, the limitation is documented and `platform: none` remains the only supported path
- **Dependency**: Ingress Operator topology-aware replica logic validated. If the current implementation does not handle runtime topology changes, the operator must be updated.

### Tech Preview -> GA

- Full test coverage including upgrades (y-stream and z-stream) on post-transition clusters for both paths
- SLOs documented and validated: target transition duration (SNO → HA compact and infra SingleReplica → HA), success rate threshold, and maximum time in the 2-member etcd window
- Monitoring and telemetry for transition metrics: Prometheus metrics exposed (transition_started, transition_completed, transition_failed, transition_duration_seconds) with alerts defined for stuck transitions exceeding SLO thresholds — covering both transition paths
- Support procedures documented for both paths
- Feature gate moved to `Default` feature set
- A plan is defined for improving the workflows (UX, transition functionality, and operator maintenance) around optional (OLM-managed) operators during topology transitions; actually managing optional operators remains out of scope for this enhancement and is left for a future enhancement
  (see [Optional (OLM-Managed) Operators and Topology Changes](#optional-olm-managed-operators-and-topology-changes) for more information on approaches that need to be addressed)

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

### Upgrades

Clusters that have undergone topology transitions follow standard OpenShift upgrade procedures. The resulting topology values (`HighlyAvailable`, `SingleReplica`, etc.) are existing enum values that all operators already support. There are no special upgrade considerations for post-transition clusters.

The topology transition controller upgrades as part of cluster-config-operator via the standard CVO-managed upgrade path.

On upgrade to a version with `MutableTopology` support, `spec.controlPlaneTopology` is empty (zero value for optional strings) — the controller interprets empty as "no transition requested" and takes no action. `spec.infrastructureTopology` is omitted; when CCO detects the `MutableTopology` gate is enabled and the field is omitted, it populates it to match the current `status.infrastructureTopology`. Since spec matches status after population, no transition is triggered.

### Downgrades

**Z-stream downgrades** (within a minor version that supports mutable topology):
Standard downgrade procedures apply. Completed transitions are not reverted — the cluster retains its current topology.

**Y-stream downgrades**:
CVO blocks y-stream downgrades.

## Version Skew Strategy

Mutable topology is gated by the `MutableTopology` feature gate. The topology transition controller is only active when the feature gate is enabled.

Version skew during transitions is not a concern because the controller manages the entire sequence within a single cluster version. The CCO topology transition controller enforces this by setting `Upgradeable=False` on its ClusterOperator while any transition (control-plane or infrastructure) is in progress, preventing CVO from initiating an upgrade.

Post-transition clusters use standard topology values that all operator versions understand. There is no version skew risk for completed transitions.

## Operational Aspects of API Extensions

This enhancement adds `controlPlaneTopology` and `infrastructureTopology` fields to `InfrastructureSpec`. These fields:

- Have no impact when they match the current status topology values or are empty
- During transitions, the CCO topology transition controller makes API calls to coordinate operator transition. These calls are low-frequency and bounded by the transition sequence.

Both spec fields use the existing `TopologyMode` type, which provides API-server-level enum validation with no additional services required. Topology status fields are not protected by admission policies — this is consistent with other infrastructure status fields.

## Support Procedures

### Team Ownership

**OpenShift Edge Team:**
- Topology transition controller in cluster-config-operator (both CP and infrastructure paths)
- CLI (`oc adm transition topology` command, including `--infrastructure` flag)
- Supported transition definitions and validation logic
- Infrastructure CR API changes (`controlPlaneTopology` and `infrastructureTopology` spec fields using `TopologyMode`)

**Control Plane Team:**
- cluster-etcd-operator (CEO) node-driven etcd scaling — existing, unmodified behavior that the control-plane transition controller relies on as a precondition

**Bare Metal Networking Team:**
- Bare metal networking for SNO clusters (future platform support)

**Component Teams:**
- Validate operator behavior during and after transitions (both CP and infrastructure)
- Infrastructure operators (Ingress, Monitoring, Image Registry, Console, OAuth) own reconciliation when `status.infrastructureTopology` changes

### Detecting Issues

**Control-Plane Transition Stuck or Failed:**
- Symptom: CCO ClusterOperator status conditions show transition in progress or failed for an extended period
- Check: `oc get clusteroperator cluster-config-operator -o yaml` for status conditions
- Check: cluster-config-operator logs for transition controller errors
- Check: CEO logs for etcd scaling operations
- Resolution: Address the reported issue and retry, or contact support

**Infrastructure Transition Stuck or Failed:**
- Symptom: CCO ClusterOperator status conditions show `InfrastructureTopologyTransition*` reason in progress or failed for an extended period
- Check: `oc get clusteroperator cluster-config-operator -o yaml` for status conditions with `InfrastructureTopologyTransition*` reasons
- Check: cluster-config-operator logs for transition controller errors
- Check: relevant infrastructure operator logs (Ingress, Monitoring, Image Registry, Console, OAuth)
- Resolution: Address the reported issue (e.g., add more workers, fix operator issues) and retry, or contact support

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
| Operator fails to reconcile post-CP-transition | Operator-specific impact | Investigate operator logs; file bug against the operator component |
| Operator fails to reconcile post-infra-transition | Infrastructure operator not at HA replicas | Investigate operator logs; verify worker node health; file bug against the operator component |
| CCO crash during transition | Transition paused | CCO restarts via deployment controller and the transition controller resumes reconciliation from spec/status divergence |
| Worker node lost after completed infra transition | Infrastructure operators report degradation (cannot schedule HA replicas) | Add replacement workers; status stays HA — degradation is an error to fix, not a topology change |

## Infrastructure Needed

No additional infrastructure is required for this feature.

CI will experience increased demand as new test lanes are introduced to support:
- Full SNO → HA compact transitions on `platform: none`
- Full standalone infrastructure transitions on `platform: none` with dedicated workers
- Post-transition cluster stability validation
- Upgrade testing on post-transition clusters
