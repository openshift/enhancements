---
title: infrastructure-topology-transition
authors:
  - "@jeff-roche"
  - "@eggfoobar"
  - "@fracappa"
reviewers:
  - "@joelspeed, for API, infrastructure config, and cluster-config-operator scope"
  - "@jerpeter, for OpenShift architecture"
  - "@sdodson, for architecture"
  - "@dgoodwin, for architecture"
approvers:
  - "@jerpeter, for OpenShift architecture"
  - "@jaypoulz"
api-approvers:
  - "@joelspeed, for API and infrastructure config"
creation-date: 2026-10-07
last-updated: 2026-10-14
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-2640
see-also:
  - "/enhancements/topologies/mutable-topology.md"
---

# Infrastructure Topology Transition

## Summary

This enhancement defines the standalone infrastructure topology transition path: transitioning `infrastructureTopology` from `SingleReplica` to `HighlyAvailable` on clusters where `status.controlPlaneTopology = HighlyAvailable` and worker-capable nodes are present.

This is a companion enhancement to [Mutable Topology](mutable-topology.md), which defines the umbrella architecture including the topology transition controller in cluster-config-operator, shared API fields, and feature gate. For an overview of control-plane driven topology transitions, see the [parent enhancement](mutable-topology.md). This document covers the infrastructure-only workflow, prerequisites, operator matrix, risks, and tests.

The infrastructure transition triggers infrastructure operators (Ingress, Monitoring, Image Registry, Console) to scale workloads from SingleReplica to HA replica counts. Unlike the control-plane topology, where `HighlyAvailable` requires 3 or more control-plane nodes, an infrastructure topology of `HighlyAvailable` requires only 2 or more compatible worker nodes — sufficient to host 2+ replicas of each infrastructure workload.

## Motivation

Clusters whose infrastructure topology is SingleReplica have no supported path to establish redundant infrastructure as a day-2 operation. This includes clusters that were installed with a single worker, or any cluster that ended up with `status.infrastructureTopology = SingleReplica` — regardless of how that state was reached.

Without a standalone infrastructure transition, administrators must either accept SingleReplica infrastructure indefinitely or redeploy the cluster.

### User Stories

* As a cluster administrator running a cluster with infrastructureTopology: SingleReplica, I want to transition to HighlyAvailable as a day-2 operation, so that infrastructure services gain redundancy once worker-capable nodes are available — either by adding new workers or by making my existing control-plane nodes worker-capable.

### Goals

* Support infrastructure topology transitions (SingleReplica → HA) on clusters with worker-capable nodes (including dual-role nodes)
* Transition infrastructure workloads to HA replica counts (e.g., default IngressController, Prometheus, Alertmanager, image registry, and console pods — see the [operator support matrix](#operator-support-matrix))
* Validate worker node prerequisites before admitting transitions
* Trigger infrastructure operator reconciliation to HA replica counts by committing `status.infrastructureTopology`, and validate convergence to the desired state

### Non-Goals

* Reverse infrastructure transitions (HA → SingleReplica)
* Managing OLM-managed (optional) operator topology support — see [Mutable Topology: OLM Operators](mutable-topology.md#optional-olm-managed-operators-and-topology-changes)

## Proposal

The infrastructure topology transition extends the topology transition controller in cluster-config-operator (defined in [Mutable Topology](mutable-topology.md)) with an infrastructure-specific reconciliation path. The controller watches `spec.infrastructureTopology` for divergence from `status.infrastructureTopology`, validates infrastructure-specific prerequisites, and commits the topology status that triggers infrastructure operators to scale to HA replica counts.

The administrator runs `oc adm transition topology --infrastructure=HighlyAvailable --confirm`, which patches `spec.infrastructureTopology`. The controller detects divergence and reconciles.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift clusters use `controlPlaneTopology: External`, and the infrastructure topology axis is conceptually still applicable to them: it concerns only the worker-labeled compute nodes, which guest clusters have. However, the infrastructure transition is driven by the topology transition controller in CCO, which does not run in the hosted control plane model — there is no controller in the guest cluster to validate prerequisites and commit the topology status. HyperShift is therefore not supported by this enhancement. Supporting it would require an equivalent transition driver for hosted clusters; this is not ruled out as future work. See [Mutable Topology: Hypershift](mutable-topology.md#hypershift--hosted-control-planes).

IBI (image-based installs) are not supported: the transition controller requires the platform and cluster state guarantees described in [Standalone Clusters](#standalone-clusters), and IBI support is tracked separately.

#### Standalone Clusters

The infrastructure transition targets standalone clusters. Platform support is rolled out independently as verification coverage lands, and is not necessarily restricted to the same platform matrix as the [parent enhancement](mutable-topology.md#standalone-clusters).

Platform rollout model:

- **Current development target**: the `None` platform (clusters without a cloud provider, e.g., agent-based deployments), where SingleReplica → HA infrastructure transitions are most immediately relevant
- **In progress**: bare metal
- **Remaining platforms**: added to the supported matrix as verification coverage lands — the matrix is coverage-driven, not a predefined list

The per-platform CI strategy and lane ownership will be documented once agreed with the involved teams.

#### Single-node Deployments or MicroShift

Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable`. This enhancement does not target SNO.

MicroShift is not affected by this enhancement.

#### OpenShift Kubernetes Engine

This proposal does not depend on features excluded from OKE. See [Mutable Topology: OKE](mutable-topology.md#openshift-kubernetes-engine).

### Workflow Description

#### Pre-Transition

1. The cluster administrator verifies that sufficient worker nodes are present, `Ready`, and schedulable to host HA infrastructure workloads
2. The cluster administrator runs `oc adm transition topology --infrastructure=HighlyAvailable --confirm`
3. The CLI discovers available transitions from the Infrastructure status, validates client-side preconditions (feature gate enabled, no transition in progress), and patches `spec.infrastructureTopology: HighlyAvailable`. Without `--confirm`, the command shows what would change without admitting the transition.
4. The API server validates `infrastructureTopology` against the `TopologyMode` enum

#### During Transition

5. The topology transition controller in CCO detects the `spec.infrastructureTopology` divergence from `status.infrastructureTopology` and validates preconditions:
   - The infrastructure operators affected by the transition (Ingress, Monitoring, Image Registry, Console — the [operator support matrix](#operator-support-matrix)) report `Available=True`, `Progressing=False`, `Degraded=False`. Operators outside this set are not gated: an unrelated degraded ClusterOperator is not a transition blocker.
   - At least 2 worker-capable nodes are present to host HA infrastructure replicas. A worker node eligible for this check is any node that:
     - Has the `node-role.kubernetes.io/worker` label (nodes that also carry `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master` labels — dual-role nodes — are included)
     - Is `Ready` and schedulable (not cordoned)
     - Is free of blocking taints: any `NoSchedule`/`NoExecute` taint other than the known-safe, kubelet- or role-managed ones (`node.kubernetes.io/not-ready`, `node.kubernetes.io/unreachable`, `node.kubernetes.io/unschedulable`, `node-role.kubernetes.io/master`, `node-role.kubernetes.io/control-plane`) disqualifies the node

   Both the node count and the readiness of at least 2 eligible nodes are checked as separate preconditions.
   - No incompatible upgrade is in progress

   If any precondition fails, the controller records the reason in the per-transition `TopologyTransitionAvailable` evaluations on the Infrastructure CR's `status.topologyTransitionStatus` and re-evaluates on the next sync.
6. Once preconditions pass, the controller establishes upgrade gating: sets `TopologyTransitionControllerProgressing=True` and `TopologyTransitionControllerUpgradeable=False` on the CCO `ClusterOperator`
7. The controller re-reads the Infrastructure CR (fresh API read) to confirm `spec.infrastructureTopology` has not changed since preconditions were checked. If spec has changed, the controller aborts (cancellation before commit — see [Cancellation and Recovery](#cancellation-and-recovery))
8. **Commit topology status**: the controller updates `status.infrastructureTopology` from `SingleReplica` to `HighlyAvailable` and writes `TopologyTransitionCompleted=False` on the Infrastructure CR's `status.topologyTransitionStatus` in the same status write (the condition's timestamp is the soak anchor and its reason identifies the committed transition). This is the point of no return — after this write, operators begin reacting to the new topology.
9. **Operator reconciliation**: infrastructure operators that watch `status.infrastructureTopology` detect the change and adjust replica counts to HA levels

#### Post-Transition

10. After a soak period (5 minutes) anchored on the status commit timestamp, the controller validates that infrastructure operators have converged:
    - Worker nodes remain `Ready` and schedulable
    - Ingress Operator: default IngressController replicas scaled to 2+ (checked only when the `ingress` capability is enabled)
    - Image Registry: registry ready replicas scaled to 2+ — checked only when the registry operator has itself requested HA scaling (`spec.replicas >= 2`); on platforms where HA replicas are not supported (e.g., RWO-only storage), the registry stays at 1 replica and does not block completion
    - Console: console replicas scaled to 2+ (checked only when the `console` capability is enabled)
    - Monitoring: `prometheus-k8s` and `alertmanager-main` StatefulSets at 2+ ready replicas (checked only when the workloads exist — a `Removed` management state does not block completion)
11. Once all checks pass, the controller reports completion: sets `TopologyTransitionCompleted=True` on the Infrastructure CR and clears `TopologyTransitionControllerProgressing`/`TopologyTransitionControllerUpgradeable=False` on the CCO `ClusterOperator`.

The CLI returns immediately after patching `spec.infrastructureTopolog` (step 3); the transition proceeds asynchronously in the controller. Administrators can monitor progress in two ways: via `status.topologyTransitionStatus` on the Infrastructure CR (e.g., `oc get infrastructure cluster -o yaml`), or via `oc adm transition status` for a summarized view.

#### Failure Handling

- **Before admission**: if a precondition fails (e.g., insufficient workers, incompatible upgrade in progress, cluster un-upgradeable), the per-transition `TopologyTransitionAvailable` evaluations report the failure reason. The controller re-evaluates on every sync — if the prerequisite is later satisfied (e.g., workers become Ready), the transition proceeds automatically.
- **After status commit**: if a post-commit validation criterion never passes (e.g., an operator fails to reconcile to HA replicas), `TopologyTransitionCompleted` remains `False` and `TopologyTransitionControllerUpgradeable` remains `False` indefinitely. Unlike preflight checks, post-transition validation results are not reported as structured per-check evaluations — only the summary conditions are exposed. The administrator inspects the Infrastructure CR's `status.topologyTransitionStatus` for the transition state, and CCO's logs for the specific failing check.

#### Cancellation and Recovery

- **Cancellation before status commit** (steps 5–7): the administrator can revise or clear `spec.infrastructureTopology` at any time before the controller commits `status.infrastructureTopology`. On the next sync, the controller detects the spec change and aborts the transition without modifying status.
- **No cancellation after status commit** (step 8 onward): once `status.infrastructureTopology = HighlyAvailable` is written, the transition is committed. Operators have already begun reacting to the new topology. Reverse transitions (HA → SingleReplica) are not supported.
- **Crash recovery after status commit**: if CCO crashes after committing status but before completing the soak, spec equals status on restart. The controller MUST NOT treat spec/status equality as "transition complete" — it checks `TopologyTransitionCompleted`. If `TopologyTransitionCompleted=False`, the controller resumes the soak validation regardless of spec/status equality. `TopologyTransitionCompleted=True` is the authoritative signal that the transition is fully complete.

#### Invariants

The following are constraints on the transition controller's own behavior and design. They are not guarantees about cluster state: the controller does not prevent other actors (administrators, the Machine API, operators) from making concurrent changes during a transition — unexpected changes are handled by the failure handling and post-transition checks instead.

- The transition MUST NOT add, remove, resize, or reconfigure nodes
- Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable` — enforced by CCO through the available transitions list, not by API-level validation

**Spec mutability:** The spec topology fields are always mutable at the API level — there are no API-level CEL or webhook immutability rules. All transition control is enforced by the CCO controller through the available transitions list. Before status changes, the administrator can revise `spec.infrastructureTopology` — effectively cancelling or revising the request. After status changes (e.g., `status.infrastructureTopology = HighlyAvailable`), the available transitions list prevents invalid combinations (e.g., setting `spec.infrastructureTopology = SingleReplica` is not an available transition). This is the natural lock: CCO prevents backwards movement without API-level immutability. This aligns with the "no x-validation for transition eligibility" decision — all behavioral enforcement lives in CCO.

**Concurrent transitions** and **admission control** are shared concepts defined in [Mutable Topology](mutable-topology.md).

### API Extensions

The `spec.infrastructureTopology` field and shared API concepts (field definitions, `TopologyMode` reuse, feature gate, admission control) are defined in [Mutable Topology: API Extensions](mutable-topology.md#api-extensions). This section covers infrastructure-specific API behavior.

**Allowed infrastructure transitions:**

| Source `status.infrastructureTopology` | `spec.infrastructureTopology` | Behavior |
|---|---|---|
| `SingleReplica` | `HighlyAvailable` | **Transition admitted** if worker node prerequisites pass (Ready, schedulable, sufficient count) |
| `HighlyAvailable` | `HighlyAvailable` | Spec matches status — controller idle |
| `HighlyAvailable` | `SingleReplica` | **Rejected** — reverse transition not supported |
| `SingleReplica` | `SingleReplica` | Spec matches status — controller idle |
| Any | omitted | Leave it unset - controller idle |

Cluster-type eligibility is not expressed in `spec.infrastructureTopology` (the field only takes `TopologyMode` values); it is enforced by CCO through the available transitions list based on the cluster's `status` — see [topology considerations](#topology-considerations).

**No cross-field API validation**: Transition eligibility is enforced by CCO through the available transitions list, not by API-level x-validation rules. See [Mutable Topology](mutable-topology.md#infrastructure-api-changes) for rationale.

**Transition progress** is reported via `status.topologyTransitionStatus` on the Infrastructure CR (defined in [openshift/api#3029](https://github.com/openshift/api/pull/3029)): `TopologyTransitionsEvaluated` indicates whether transitions have been evaluated, `TopologyTransitionAvailable` reports per-transition preflight check results, and `TopologyTransitionCompleted` tracks lifecycle progress. `TopologyTransitionControllerUpgradeable` on the CCO `ClusterOperator` gates CVO upgrades during active transitions.

### Implementation Details/Notes/Constraints

#### Transition Orchestration

The controller reconciles `spec.infrastructureTopology` against `status.infrastructureTopology` on every sync. The full sequence — preconditions, orchestration steps, validation criteria, and failure handling — is described in [Workflow Description](#workflow-description). The [Invariants](#invariants) section defines the constraints the controller is designed around: limits on the controller's own behavior, not guarantees it imposes on the cluster.

Implementation-specific notes:

- Transition status is reported via the conditions defined in [openshift/api#3029](https://github.com/openshift/api/pull/3029): `TopologyTransitionsEvaluated`, `TopologyTransitionCompleted`, and per-transition `TopologyTransitionAvailable` evaluations on the Infrastructure CR; `TopologyTransitionControllerUpgradeable` on the CCO `ClusterOperator` for CVO upgrade gating
- The soak period is 5 minutes, anchored on the status commit timestamp
- The controller re-reads the Infrastructure CR (fresh API read) before committing status to guard against spec changes between precondition check and status write
- On restart, `TopologyTransitionCompleted=False` is the signal to resume soak validation — spec/status equality alone does not indicate completion (see [Cancellation and Recovery](#cancellation-and-recovery))

#### Worker Node Prerequisites

The worker node eligibility criteria are defined in [During Transition step 5](#during-transition): any node with the `node-role.kubernetes.io/worker` label — including dual-role nodes — that is `Ready`, schedulable, and free of blocking taints.

The requirements are:

- **Minimum worker node count**: 2 — sufficient to host 2 replicas of each infrastructure workload
- **Capacity check**: none — replica placement is left to the scheduler; a cluster too small to host the HA workloads surfaces as post-transition validation failures rather than an admission blocker
- **Operator health**: the affected infrastructure operators (Ingress, Monitoring, Image Registry, Console) must report `Available=True`, `Progressing=False`, `Degraded=False` before the transition is admitted

#### Operator Support Matrix

The following operators are expected to react to `status.infrastructureTopology` changes from `SingleReplica` to `HighlyAvailable` by scaling their workloads to HA replica counts:

| Operator | Expected Behavior | Notes |
|---|---|---|
| **Ingress Operator** | Scale default IngressController replicas from 1 to 2+ | Topology-aware replica logic needs validation |
| **Monitoring** | Scale Prometheus, Alertmanager to HA replicas | Watches infrastructure topology |
| **Image Registry** | Scale registry replicas | Watches infrastructure topology |
| **Console** | Scale console replicas | Watches infrastructure topology |

The per-operator topology dependency audit and the exact operator set must be confirmed during implementation.

#### Component Changes

| Component | Changes Required |
| --------- | ---------------- |
| cluster-config-operator | Infrastructure transition path in topology transition controller; watches `spec.infrastructureTopology`, validates prerequisites, updates `status.infrastructureTopology` |
| Infrastructure API (`openshift/api`) | `infrastructureTopology` added to `InfrastructureSpec` (defined in [Mutable Topology](mutable-topology.md#infrastructure-api-changes)) |
| `oc` CLI | `--infrastructure` flag for `oc adm transition topology`; discovers available infrastructure transitions |
| Infrastructure operators (Ingress, Monitoring, Image Registry, Console) | Reconcile on `status.infrastructureTopology` changes — scale workloads to HA replica counts |

#### Ownership Boundaries

| Component | Owner | Responsibilities |
|---|---|---|
| **API contract** (`spec.infrastructureTopology`) | openshift/api maintainers | Define spec field using `TopologyMode` with enum validation, feature gate |
| **CCO Infrastructure Transition Controller** | cluster-config-operator maintainers | Watch `spec.infrastructureTopology` for divergence, server-side admission, prerequisites validation, reconciliation, lifecycle reporting via CCO ClusterOperator conditions |
| **`oc adm transition topology --infrastructure` CLI** | openshift/oc maintainers | Discover available infrastructure transitions, patch spec field, display progress |
| **Infrastructure operators** | Individual operator teams | Own reconciliation of their workloads when `status.infrastructureTopology` changes. React to status changes. Do not own the transition lifecycle |

**Boundary rules:** See [Invariants](#invariants) for the full set of MUST/MUST NOT guarantees. Additionally:
- The CLI MUST NOT duplicate server-side validation or write observed status.
- Infrastructure operators react to `status.infrastructureTopology` changes; they do not own or drive the transition lifecycle.

### Risks and Mitigations

#### Risk: Infrastructure Operators May Not React to Topology Changes

**Risk**: Infrastructure operators that do not watch `status.infrastructureTopology` for changes, or that have bugs in their topology-aware scaling logic, may not adjust replica counts after a standalone infrastructure transition. The Ingress Operator in particular needs validation that its topology-aware replica logic correctly handles runtime topology changes.

This risk is scoped to core (payload) operators — the operator support matrix set. Core operators are covered by the transition's CI lanes and post-transition validation and are expected to be well tested. Optional, certified, and third-party operators are out of scope for this enhancement: how they react to topology changes will be addressed in a future enhancement (see the [OLM operators non-goal](#non-goals)).

**Mitigation**:
- The per-operator topology dependency audit is a prerequisite for entering dev preview
- The post-transition soak validates that each expected core operator has converged to HA replicas before the transition is considered complete
- The Ingress dependency is documented and tracked separately
- Operators that read topology only at startup (rather than watching) are identified during the audit and a restart strategy is documented

#### Risk: Transition Fails Partway Through

**Risk**: An infrastructure transition may fail after some operators have begun reconfiguring but before the transition completes, leaving the cluster in an intermediate state. Examples:

- **Operator reconciliation failure**: after topology status fields are updated, an operator fails to reconcile (e.g., ingress pod fails to schedule on a new node due to resource constraints or `ImagePullBackOff`).
- **Node readiness**: a worker node becomes `NotReady` during the transition (e.g., disk pressure, network misconfiguration).

**Mitigation**:
- The controller only admits a transition once its preconditions — including sufficient ready workers — are satisfied
- Operators do not see a topology change until the controller updates the infrastructure status
- CCO ClusterOperator status conditions provide detailed state for troubleshooting precondition and post-admission failures

#### Risk: Cannot Validate External Requirements

**Risk**: The topology transition controller cannot validate external requirements such as correct load balancer configuration or DNS setup.

**Mitigation**:
- Pre-flight checks validate what is within the cluster's control (node presence, resource requirements, operator health)
- External requirements are documented as the administrator's responsibility
- The CLI can surface warnings about external prerequisites before patching the infrastructure CR

### Drawbacks

#### One-Way Transitions (Initially)

The initial implementation supports only SingleReplica → HA. Reverse transitions (HA → SingleReplica) are future work. Administrators who transition cannot revert without redeploying.

#### Coordination Across Infrastructure Operator Teams

The infrastructure transition requires coordination with Ingress, Monitoring, Image Registry, and Console operator teams to ensure they reconcile correctly when `status.infrastructureTopology` changes. Most of this coordination was already done for the [parent enhancement](mutable-topology.md), whose SNO → HA transition moves both `controlPlaneTopology` and `infrastructureTopology` and therefore already required these operators to reconcile on topology changes — the infrastructure transition exercises the same operator behavior on the infrastructure axis alone. Any remaining gaps are tracked through the per-operator topology dependency audit.

## Alternatives (Not Implemented)

### Action-Oriented Infrastructure Transition Request

An alternative for the standalone infrastructure transition was to represent it as a one-time action-oriented request (annotation or status sub-resource) rather than a durable `spec.infrastructureTopology` field.

**Why it was rejected**:
- Introduces a fundamentally different intent mechanism alongside the established spec/status pattern. Two different intent mechanisms for the same class of operation increases cognitive load and implementation divergence.
- Annotations are untyped, unversioned, and not subject to API validation. A structured annotation can be malformed in ways that a typed spec field cannot.
- Without a persistent spec field, crash recovery requires the controller to maintain out-of-band state. The spec/status divergence model handles this naturally.
- Mid-transition behavior is unclear: there is no spec to lock. The controller must invent its own mechanism to prevent changes during an in-flight transition.
- This model trades API discipline for short-term implementation convenience, contradicting the established design direction.

## Test Plan

### CI Lanes

| Lane | Frequency | Description |
| ---- | --------- | ----------- |
| MutableTopology infra transition suite | Nightly | Run infrastructure transition test suite: SingleReplica → HA with worker-capable nodes (dedicated and dual-role) |
| End-to-End tests (e2e) | Weekly | Standard test suite (openshift/conformance/parallel) on post-infrastructure-transition clusters |
| Upgrade between z-streams | Weekly | Test upgrades on post-infrastructure-transition clusters |
| Upgrade between y-streams | Weekly | Test upgrades across minor versions on post-infrastructure-transition clusters |

### CI Tests

#### Pre-Transition Tests

| Test | Description |
| ---- | ----------- |
| Infra precondition validation | Verify the controller withholds admission when workers are insufficient/not-ready, incompatible upgrade in progress, or cluster is un-upgradeable |
| Spec mutability during transition | Verify that spec topology fields are always mutable at the API level — no CEL or webhook rejection. Verify that changing spec before the status commit effectively cancels the transition. Verify that CCO's available transitions list prevents invalid states after the status commit. |
| CLI interaction | Verify `oc adm transition topology --infrastructure=HighlyAvailable` correctly patches `spec.infrastructureTopology` and monitors progress |

#### Transition Tests

| Test | Description |
| ---- | ----------- |
| Infra SingleReplica → HA | Full infrastructure transition: controller validates workers, admits transition, infrastructure operators converge to HA replicas |
| Infra failure and recovery | Verify the controller withholds admission when workers are insufficient, and that admission proceeds automatically when workers become Ready |
| Post-transition infra operator health | Verify infrastructure operators (Ingress, Monitoring, Image Registry, Console) converge to HA replicas after `status.infrastructureTopology` changes |
| Crash recovery before status commit | Kill CCO before status commit, verify controller resumes from spec/status divergence |
| Crash recovery after status commit | Kill CCO after status commit but before soak completes, verify controller resumes soak validation despite spec/status equality — `TopologyTransitionCompleted=False` drives resumption |
| Idempotent behavior | Verify that spec matching status (no-op) and repeated spec writes do not trigger re-transitions |
| Cancellation before status commit | Verify that changing `spec.infrastructureTopology` before the controller commits status cancels the in-progress transition. Verify that after status is committed, reverse transitions are rejected. |

### QE Testing

QE validation mirrors the [CI tests](#ci-tests) above, executed as manual/exploratory campaigns rather than automated lanes:

- The full transition test matrix — happy path (dedicated and dual-role workers), failure and recovery, cancellation, crash recovery, and upgrade gating — as described in [Transition Tests](#transition-tests)
- Additional QE-only scenarios not covered by CI:
  - Post-transition cluster stability over 24 hours
  - Worker node loss after a completed transition (status stays HA, degradation reported)
  - Post-transition upgrade preserves topology status

## Graduation Criteria

The infrastructure transition does not carry its own preview ladder: it ships as part of the [parent enhancement](mutable-topology.md) and enters whatever preview state the parent holds in a given release. The technical specification above is the definition of dev-complete; the criteria below only call out what is allowed to be unfinished before each graduation step.

### Dev Preview -> Tech Preview

None: the Ingress Operator's topology-aware replica logic has been validated to handle runtime topology changes, and no component is allowed to be unfinished entering tech preview.

### Tech Preview -> GA

- Full test coverage including upgrades (y-stream and z-stream) on post-infrastructure-transition clusters
- SLOs documented and validated: target infrastructure transition duration (SingleReplica → HA) and success rate threshold
- Monitoring and telemetry: Prometheus metrics exposed (infra_transition_started, infra_transition_completed, infra_transition_failed, infra_transition_duration_seconds) with alerts for stuck transitions exceeding SLO thresholds
- Support procedures documented for infrastructure transition path
- Infrastructure path fully tested including upgrades

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

On upgrade to a version with `MutableTopology` support, `spec.infrastructureTopology` is omitted; when CCO detects the gate is enabled and the field is omitted, it populates it to match `status.infrastructureTopology`. Since spec matches status after population, no transition is triggered.

See [Mutable Topology: Upgrade / Downgrade Strategy](mutable-topology.md#upgrade--downgrade-strategy) for the full upgrade/downgrade strategy covering both transition paths.

## Version Skew Strategy

The infrastructure transition controller is gated by the `MutableTopology` feature gate and only active when enabled. Version skew during transitions is not a concern because the controller manages the entire sequence within a single cluster version. CCO sets `TopologyTransitionControllerUpgradeable=False` during active transitions, preventing CVO from initiating an upgrade.

See [Mutable Topology: Version Skew Strategy](mutable-topology.md#version-skew-strategy) for details.

## Operational Aspects of API Extensions

The `spec.infrastructureTopology` field has no impact when it matches `status.infrastructureTopology` or is omitted. During transitions, the CCO topology transition controller writes the Infrastructure CR status to trigger operator reconciliation — these calls are low-frequency and bounded by the transition sequence.

See [Mutable Topology: Operational Aspects](mutable-topology.md#operational-aspects-of-api-extensions) for the shared API extension details.

## Support Procedures

### Detecting Issues

**Infrastructure Transition Stuck or Failed:**
- Symptom: `TopologyTransitionCompleted=False` on the Infrastructure CR for an extended period
- Check: `oc get infrastructure cluster -o yaml` for `status.topologyTransitionStatus` conditions and per-transition evaluations
- Check: cluster-config-operator logs for transition controller errors
- Check: relevant infrastructure operator logs (Ingress, Monitoring, Image Registry, Console)
- Resolution: Address the reported issue (e.g., add more workers, fix operator issues) and retry, or contact support

### Recovery Procedures

| Failure Mode | Impact | Recovery |
| ------------ | ------ | -------- |
| Controller fails during precondition check | No impact — transition not admitted | Address the precondition and retry |
| Operator fails to reconcile after status commit | Infrastructure operator not at HA replicas | Investigate operator logs; verify worker node health; file bug against the operator component |
| CCO crash before status commit | Transition paused, no status change | CCO restarts and resumes from spec/status divergence |
| CCO crash after status commit | Status committed, soak incomplete | CCO restarts, sees `TopologyTransitionCompleted=False`, resumes soak validation despite spec/status equality |
| Worker node lost after completed infra transition | Infrastructure operators report degradation (cannot schedule HA replicas) | Add replacement workers; status stays HA — degradation is an error to fix, not a topology change |

### Team Ownership

**OpenShift Edge Team:**
- Infrastructure transition path in topology transition controller
- CLI (`oc adm transition topology --infrastructure` flag)
- Infrastructure CR API changes (`spec.infrastructureTopology`)

**Component Teams:**
- Validate operator behavior during and after infrastructure transitions
- Infrastructure operators (Ingress, Monitoring, Image Registry, Console) own reconciliation when `status.infrastructureTopology` changes

## Infrastructure Needed

No additional infrastructure is required.

CI will experience increased demand as new test lanes are introduced for infrastructure transition testing with dedicated workers and with dual-role nodes.
