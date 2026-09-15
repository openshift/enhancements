---
title: infrastructure-topology-transition
authors:
  - "@jeff-roche"
  - "@jaypoulz"
  - "@eggfoobar"
  - "@fracappa"
reviewers:
  - "@joelspeed, for API, infrastructure config, and cluster-config-operator scope"
  - "@jerpeter, for OpenShift architecture"
  - "@sdodson, for architecture"
  - "@dgoodwin, for architecture"
approvers:
  - "@jerpeter, for OpenShift architecture"
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

This is a companion enhancement to [Mutable Topology](mutable-topology.md), which defines the umbrella architecture including the topology transition controller in cluster-config-operator, shared API fields, and feature gate. For an overview of control-plane driven topology transitions, see the [parent enhancement](mutable-topology.md). This document covers the infrastructure-specific workflow, prerequisites, operator matrix, risks, and tests.

The infrastructure transition coordinates infrastructure operators (Ingress, Monitoring, Image Registry, Console, OAuth) to scale workloads from SingleReplica to HA replica counts.

## Motivation

Clusters whose infrastructure topology is SingleReplica have no supported path to establish redundant infrastructure as a day-2 operation. This includes clusters that were installed with a single worker, or any cluster that ended up with `status.infrastructureTopology = SingleReplica` — regardless of how that state was reached.

Without a standalone infrastructure transition, administrators must either accept SingleReplica infrastructure indefinitely or redeploy the cluster.

### User Stories

* As a cluster administrator whose infrastructure topology is SingleReplica, I want to transition infrastructure topology to HighlyAvailable as a day-2 operation so that infrastructure services gain redundancy once I add worker-capable nodes.

### Goals

* Support infrastructure topology transitions (SingleReplica → HA) on clusters with worker-capable nodes (including dual-role nodes)
* Transition infrastructure workloads to HA replica counts
* Validate worker node prerequisites before admitting transitions
* Coordinate infrastructure operator reconciliation to HA replica counts

### Non-Goals

* Reverse infrastructure transitions (HA → SingleReplica)
* Managing OLM-managed (optional) operator topology support — see [Mutable Topology: OLM Operators](mutable-topology.md#optional-olm-managed-operators-and-topology-changes)

## Proposal

The infrastructure topology transition extends the topology transition controller in cluster-config-operator (defined in [Mutable Topology](mutable-topology.md)) with an infrastructure-specific reconciliation path. The controller watches `spec.infrastructureTopology` for divergence from `status.infrastructureTopology`, validates infrastructure-specific prerequisites, and coordinates infrastructure operators to HA replica counts.

The administrator runs `oc adm transition topology --infrastructure=HighlyAvailable --confirm`, which patches `spec.infrastructureTopology`. The controller detects divergence and reconciles.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not applicable. See [Mutable Topology: Hypershift](mutable-topology.md#hypershift--hosted-control-planes).

#### Standalone Clusters

The infrastructure transition targets standalone clusters. Platform support is rolled out independently as verification tests cover each platform and is not necessarily restricted to the same platform matrix as the [parent enhancement](mutable-topology.md#standalone-clusters).

#### Single-node Deployments or MicroShift

Infrastructure transitions require `status.controlPlaneTopology = HighlyAvailable`. This enhancement does not target SNO or compact clusters.

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
   - Every ClusterOperator other than cluster-config-operator itself reports `Available=True`, `Progressing=False`, `Degraded=False`
   - Sufficient worker-capable nodes are present to host HA infrastructure replicas. A worker node eligible for this check is any node that:
     - Has the `node-role.kubernetes.io/worker` label (nodes that also carry `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master` labels — dual-role nodes — are included)
     - Is `Ready`
     - Is schedulable (not cordoned or draining)
     - Is not blocked by incompatible taints or lifecycle state
     - Is able to host the expected infrastructure replicas
   - No incompatible upgrade is in progress
   - The cluster is not un-upgradeable (CVO `Upgradeable=False`) — avoids compounding risk on an already degraded cluster

   If any precondition fails, the controller does not admit the transition; it records the reason on the `Progressing`/`Upgradeable` conditions (e.g. `PreflightCheckFailed`, `UpgradeInProgress`, `ClusterNotUpgradeable`) and re-evaluates on the next sync.
6. Once preconditions pass, the controller verifies an upgrade has not been triggered by CVO and then sets `Upgradeable=False` and a `Progressing` condition (`reason: InfrastructureTopologyTransitionInProgress`) on the CCO `ClusterOperator` in the same update
7. The controller re-reads the Infrastructure CR (fresh API read) to confirm `spec.infrastructureTopology` has not changed since preconditions were checked
8. **Infrastructure operator reactions** — operators that watch `status.infrastructureTopology` reconcile against the new values and adjust replica counts to HA levels

#### Post-Transition

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

The CLI returns immediately after patching `spec.infrastructureTopology` (step 3). Administrators can monitor transition progress by watching CCO ClusterOperator status conditions (e.g., `oc get clusteroperator cluster-config-operator -o yaml`).

#### Failure Handling

- **Before admission**: if a precondition fails (e.g., insufficient workers, incompatible upgrade in progress, cluster un-upgradeable), the `Progressing`/`Upgradeable` conditions carry a diagnostic reason (e.g. `PreflightCheckFailed`, `UpgradeInProgress`, `ClusterNotUpgradeable`). The controller re-evaluates on every sync — if the prerequisite is later satisfied (e.g., workers become Ready), the transition proceeds automatically.
- **After admission**: if a post-transition validation criterion never passes (e.g., an operator fails to reconcile to HA replicas), the `Progressing` condition remains `True` and `Upgradeable` remains `False` indefinitely. The administrator inspects CCO and the relevant operator's logs and ClusterOperator status conditions for details.

#### Invariants

The following invariants hold throughout the entire infrastructure transition lifecycle:

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
| Any | omitted | CCO populates to match `status.infrastructureTopology` when `MutableTopology` gate is enabled |
| Any | Any (HyperShift) | **Rejected** — not applicable |
| Any | Any (IBI) | **Rejected** — not supported |

**No cross-field API validation**: Transition eligibility is enforced by CCO through the available transitions list, not by API-level x-validation rules. See [Mutable Topology](mutable-topology.md#infrastructure-api-changes) for rationale.

**Transition progress** is reported via conditions on the CCO `ClusterOperator` status.

### Implementation Details/Notes/Constraints

#### Transition Orchestration

The controller reconciles `spec.infrastructureTopology` against `status.infrastructureTopology` on every sync. The full sequence — preconditions, orchestration steps, validation criteria, and failure handling — is described in [Workflow Description](#workflow-description). The [Invariants](#invariants) section defines the guarantees enforced throughout.

Implementation-specific notes:

- Condition reason strings use the `InfrastructureTopologyTransition` prefix (e.g., `InfrastructureTopologyTransitionInProgress`, `InfrastructureTopologyTransitionComplete`)
- The soak period is 5 minutes, anchored on the `Progressing` condition's `LastTransitionTime`
- The controller re-reads the Infrastructure CR (fresh API read) before updating status to guard against spec changes between precondition check and status write

#### Worker Node Prerequisites

The worker node eligibility criteria are defined in [During Transition step 5](#during-transition). In summary: any node with the `node-role.kubernetes.io/worker` label — including dual-role nodes — that is `Ready`, schedulable, and able to host HA replicas.

The exact minimum worker node count, capacity check, and operator-health threshold must be explicitly agreed by API/CCO owners.

#### Operator Support Matrix

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

#### Component Changes

| Component | Changes Required |
| --------- | ---------------- |
| cluster-config-operator | Infrastructure transition path in topology transition controller; watches `spec.infrastructureTopology`, validates prerequisites, updates `status.infrastructureTopology` |
| Infrastructure API (`openshift/api`) | `infrastructureTopology` added to `InfrastructureSpec` (defined in [Mutable Topology](mutable-topology.md#infrastructure-api-changes)) |
| `oc` CLI | `--infrastructure` flag for `oc adm transition topology`; discovers available infrastructure transitions |
| Infrastructure operators (Ingress, Monitoring, Image Registry, Console, OAuth) | Reconcile on `status.infrastructureTopology` changes — scale workloads to HA replica counts |

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

**Mitigation**:
- The per-operator topology dependency audit is a prerequisite for entering dev preview
- The post-transition soak validates that each expected operator has converged to HA replicas before the transition is considered complete
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

The infrastructure transition requires coordination with Ingress, Monitoring, Image Registry, Console, and OAuth operator teams to ensure they reconcile correctly when `status.infrastructureTopology` changes. This coordination is tracked through the per-operator topology dependency audit.

## Alternatives (Not Implemented)

### Action-Oriented Infrastructure Transition Request

An alternative for the standalone infrastructure transition was to represent it as a one-time action-oriented request (annotation or status sub-resource) rather than a durable `spec.infrastructureTopology` field.

**Why it was rejected**:
- Introduces a fundamentally different intent mechanism alongside the established spec/status pattern. Two different intent mechanisms for the same class of operation increases cognitive load and implementation divergence.
- Annotations are untyped, unversioned, and not subject to API validation. A structured annotation can be malformed in ways that a typed spec field cannot.
- Without a persistent spec field, crash recovery requires the controller to maintain out-of-band state. The spec/status divergence model handles this naturally.
- Mid-transition behavior is unclear: there is no spec to lock. The controller must invent its own mechanism to prevent changes during an in-flight transition.
- This model trades API discipline for short-term implementation convenience, contradicting the established design direction.

## Open Questions

1. **Minimum worker node count for infrastructure transitions**: The exact minimum worker count required for a standalone infrastructure topology transition needs to be defined and agreed by API/CCO owners. This includes whether the threshold should be a fixed number or derived from the expected HA replica counts of infrastructure operators.

2. **Operator support matrix confirmation**: The exact set of operators that react to `status.infrastructureTopology` changes and their expected HA behaviors must be confirmed during implementation. The Ingress Operator's topology-aware replica logic needs validation.

3. **Discovery contract shape**: The topology transition discovery field shape must be agreed with API reviewers as part of the openshift/api PR that introduces the topology transition status contract. This includes how transitions are represented (per-axis vs unified), the source/target structure, availability semantics, and how the CLI consumes this information.

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
| Spec mutability during transition | Verify that spec topology fields are always mutable at the API level — no CEL or webhook rejection. Verify that changing spec before status moves effectively cancels the transition. Verify that CCO's available transitions list prevents invalid states after status changes. |
| CLI interaction | Verify `oc adm transition topology --infrastructure=HighlyAvailable` correctly patches `spec.infrastructureTopology` and monitors progress |

#### Transition Tests

| Test | Description |
| ---- | ----------- |
| Infra SingleReplica → HA | Full infrastructure transition: controller validates workers, admits transition, infrastructure operators converge to HA replicas |
| Infra failure and recovery | Verify the controller withholds admission when workers are insufficient, and that admission proceeds automatically when workers become Ready |
| Post-transition infra operator health | Verify infrastructure operators (Ingress, Monitoring, Image Registry, Console, OAuth) converge to HA replicas after `status.infrastructureTopology` changes |
| Crash recovery | Kill CCO during infrastructure transition, verify controller resumes from spec/status divergence |
| Idempotent behavior | Verify that spec matching status (no-op) and repeated spec writes do not trigger re-transitions |
| Spec change during transition | Verify that changing `spec.infrastructureTopology` before status moves cancels the in-progress transition. Verify that after status moves, CCO's available transitions list prevents invalid spec values. |

### QE Testing

- Full infrastructure transition with dedicated workers and with dual-role nodes
- Transition failure and recovery scenarios
- Post-transition cluster stability over 24 hours
- Concurrent operation testing: transition + upgrade attempt (verify `Upgradeable=False` blocks upgrades)
- Spec mutability testing: verify spec topology fields are always mutable at the API level; verify cancellation window (spec change before status moves); verify CCO prevents invalid states after status changes
- Worker node loss after completed infrastructure transition (verify status stays HA, degradation reported)
- Upgrade gating: CVO cannot start an upgrade while infrastructure transition is in progress
- Post-transition upgrade: cluster upgraded after successful infrastructure transition preserves topology status

## Graduation Criteria

### Entering Dev Preview

- `infrastructureTopology` field added to `InfrastructureSpec`
- `TopologyMode` enum validated for `spec.infrastructureTopology` in API integration tests
- Standalone infrastructure transition controller path implemented
- `oc adm transition topology --infrastructure` CLI support implemented
- Infrastructure transition CI lane operational
- Per-operator topology dependency matrix completed for infrastructure operators: document what each operator uses `infrastructureTopology` for and whether it watches the infrastructure CR for changes or reads at startup
- CCO sets `Upgradeable=False` on its ClusterOperator while an infrastructure transition is in progress

### Dev Preview -> Tech Preview

- Infrastructure transition test suite validates full SingleReplica → HA path with dedicated workers and with dual-role nodes
- Tests verify infrastructure operator health during and after transition
- `oc adm transition topology --infrastructure` provides clear diagnostics on failure
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Spec mutability validated end-to-end: cancellation window before status changes, CCO prevents invalid states after status changes
- Infrastructure operator convergence validated (Ingress, Monitoring, Image Registry, Console, OAuth)
- **Dependency**: Ingress Operator topology-aware replica logic validated. If the current implementation does not handle runtime topology changes, the operator must be updated.

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

The infrastructure transition controller is gated by the `MutableTopology` feature gate and only active when enabled. Version skew during transitions is not a concern because the controller manages the entire sequence within a single cluster version. CCO sets `Upgradeable=False` during active transitions, preventing CVO from initiating an upgrade.

See [Mutable Topology: Version Skew Strategy](mutable-topology.md#version-skew-strategy) for details.

## Operational Aspects of API Extensions

The `spec.infrastructureTopology` field has no impact when it matches `status.infrastructureTopology` or is omitted. During transitions, the CCO topology transition controller makes API calls to coordinate operator reconciliation — these calls are low-frequency and bounded by the transition sequence.

See [Mutable Topology: Operational Aspects](mutable-topology.md#operational-aspects-of-api-extensions) for the shared API extension details.

## Support Procedures

### Detecting Issues

**Infrastructure Transition Stuck or Failed:**
- Symptom: CCO ClusterOperator status conditions show `InfrastructureTopologyTransition*` reason in progress or failed for an extended period
- Check: `oc get clusteroperator cluster-config-operator -o yaml` for status conditions with `InfrastructureTopologyTransition*` reasons
- Check: cluster-config-operator logs for transition controller errors
- Check: relevant infrastructure operator logs (Ingress, Monitoring, Image Registry, Console, OAuth)
- Resolution: Address the reported issue (e.g., add more workers, fix operator issues) and retry, or contact support

### Recovery Procedures

| Failure Mode | Impact | Recovery |
| ------------ | ------ | -------- |
| Controller fails during precondition check | No impact — transition not admitted | Address the precondition and retry |
| Operator fails to reconcile post-infra-transition | Infrastructure operator not at HA replicas | Investigate operator logs; verify worker node health; file bug against the operator component |
| CCO crash during transition | Transition paused | CCO restarts via deployment controller and the transition controller resumes reconciliation from spec/status divergence |
| Worker node lost after completed infra transition | Infrastructure operators report degradation (cannot schedule HA replicas) | Add replacement workers; status stays HA — degradation is an error to fix, not a topology change |

### Team Ownership

**OpenShift Edge Team:**
- Infrastructure transition path in topology transition controller
- CLI (`oc adm transition topology --infrastructure` flag)
- Infrastructure CR API changes (`spec.infrastructureTopology`)

**Component Teams:**
- Validate operator behavior during and after infrastructure transitions
- Infrastructure operators (Ingress, Monitoring, Image Registry, Console, OAuth) own reconciliation when `status.infrastructureTopology` changes

## Infrastructure Needed

No additional infrastructure is required.

CI will experience increased demand as new test lanes are introduced for infrastructure transition testing with dedicated workers and with dual-role nodes.
