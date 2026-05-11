---
title: mutable-topology
authors:
  - "@jeff-roche"
  - "@jaypoulz"
  - "@eggfoobar"
reviewers:
  - "@tjungblu, for cluster-etcd-operator"
  - "@joelspeed, for API and infrastructure config"
  - "@spadgett, for console"
  - "@jerpeter, for OpenShift architecture"
approvers:
  - "@jerpeter, for OpenShift architecture"
api-approvers:
  - "@joelspeed, for API and infrastructure config"
creation-date: 2026-05-11
last-updated: 2026-05-11
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-2280
replaces:
  - https://github.com/openshift/enhancements/pull/1905
superseded-by: []
---

# Mutable Topology

## Terms

**Mutable Topology** — The capability for an OpenShift cluster to transition between topology modes as a Day 2 operation, removing the existing assumption that topologies are immutable after installation.

**Topology Transition** — A directed, orchestrated change from one topology mode to another (e.g., SingleReplica to HighlyAvailable). Transitions are managed by a dedicated operator and follow a directed graph of supported paths.

**OpenShift Topology Transition Operator (OTTO)** — An optional payload operator responsible for orchestrating topology transitions. OTTO owns the transition graph, validates preconditions, coordinates with cluster operators, and updates the Infrastructure config once the cluster is ready.

**Control Plane Topology** — The cluster-topology mode describing how control-plane nodes are deployed and managed (SingleReplica, HighlyAvailable, or other supported modes). Control-plane nodes are nodes labeled with `node-role.kubernetes.io/control-plane` or `node-role.kubernetes.io/master`.

**Infrastructure Topology** — The cluster-topology mode describing how infrastructure workloads are distributed (SingleReplica, HighlyAvailable, or other supported modes). When there are no worker nodes, control-plane nodes serve as workers.

**Compact Cluster** — A cluster where control-plane nodes also serve as workers. In the initial SNO-to-HA transition, the target is a 3-node compact cluster with no dedicated worker nodes.

**Cluster Administrator** — An entity responsible for managing an existing cluster, including Day 2 operations such as topology transitions and node scaling.

## Summary

This enhancement introduces "mutable topology" which is defined as "the ability for OpenShift clusters to transition between topology modes as a Day 2 operation". This changes the existing OpenShift assumption that topologies are immutable after installation.

A new optional payload operator, the OpenShift Topology Transition Operator (OTTO), will orchestrate transitions.
OTTO maintains a directed graph of supported transitions along with their preconditions, configuration steps, and validation criteria.
A new `oc adm transition topology` CLI command provides an interactive interface for cluster administrators to configure and execute transitions.
The initial implementation supports transitioning Single Node OpenShift (SNO) clusters to HA compact (3-node) on `platform: none`.

## Motivation

Cluster demands change over time. Customers who start with Single Node OpenShift (SNO) at edge locations may later require high availability as workloads become more critical. Today, this requires redeploying the cluster — a disruptive operation that involves workload migration, downtime, and operational overhead.

The previous approach to this problem ([Adaptable Topology](https://github.com/openshift/enhancements/pull/1905)) proposed a new topology mode
where operators dynamically react to node count changes.
That approach required updating every core operator to handle dynamic topology shifts
and introduced a new topology enum value that all operators had to understand.
It also coupled topology behavior to node count, making operator logic more complex.

Mutable topology takes a different approach: instead of adding a new topology mode that operators must interpret,
transitions are orchestrated by a dedicated operator that coordinates the sequencing, validates preconditions,
and updates the Infrastructure config only when the cluster is ready for the new mode.
Operators continue to react to the same fixed topology values they already understand.
This keeps operator logic simple and concentrates transition complexity in a single component.

### User Stories

* As a cluster administrator running Single Node OpenShift (SNO) at an edge location, I want to add control-plane nodes to my cluster to achieve high availability so that I can handle node failures without service disruption as workloads become more critical.

* As a cluster administrator deploying OpenShift clusters at scale, I want to start with minimal footprint deployments that can grow into highly available clusters so that I can reduce initial costs while maintaining scalability.

* As a cluster administrator managing a fleet of edge deployments, I want a supported path to transition my cluster topology so that I don't need to redeploy clusters when my infrastructure requirements change.

* As a platform engineer, I want topology transitions managed by a dedicated operator so that the transition logic is isolated from my operational tooling and I have a clear interface for monitoring transition state.

### Goals

* Officially support topology transitions in OpenShift
* Provide a topology transition operator (OTTO) that owns the transition graph and orchestrates transitions safely
* Provide an `oc adm transition topology` CLI command for interactive transition management
* Support transitioning SNO clusters to HA compact (3-node) on `platform: none` as the initial transition path
* Maintain backward compatibility — existing clusters with fixed topology modes are unaffected
* Establish the architectural foundation for additional transition paths in the future

### Non-Goals

* Supporting all possible topology transitions in the initial implementation (only SNO → HA compact on `platform: none`)
* Supporting transitions for HyperShift or hosted control plane clusters
* Supporting transitions for MicroShift deployments
* Automatic node provisioning or deprovisioning based on workload demands
* Scaling down control-plane or worker nodes (scale-down may be addressed in a future enhancement)
* Supporting bidirectional transitions (e.g., HA → SNO) in the initial implementation
* Installing OTTO by default on all clusters

## Proposal

Mutable topology introduces a new optional payload operator and CLI command to enable topology transitions as Day 2 operations.

A dedicated operator is the right vehicle for this because topology transitions are long-running, multi-step orchestration workflows that require persistent state, failure recovery, and coordination across multiple cluster operators. This logic does not belong in CVO — CVO is a critical-path operator where additional surface area increases risk to every cluster, and topology transitions are operationally distinct from version management. It does not belong in the CLI because CLI processes cannot survive disconnects or provide the reconciliation loop needed for automatic recovery from partial failures. A separate operator isolates this complexity, ships with the payload but installs only when needed, and can be tested independently.

The approach has two components:

1. **OpenShift Topology Transition Operator (OTTO)** — An operator that ships with the payload but is not installed by default. OTTO owns the transition graph, validates preconditions, orchestrates the transition sequence, and updates the Infrastructure config as the final step.

2. **`oc adm transition topology` CLI command** — An interactive command that installs/activates OTTO if needed, guides the administrator through configuring the transition (nodes, certificates, secrets, etc.), and monitors transition status via OTTO's custom resources.

### Workflow Description

#### Transition: SNO to HA Compact (3-Node)

**cluster administrator** is an entity responsible for managing an existing cluster.

**Non-functional constraint**: There is no availability guarantee during topology transitions. Scaling control-plane nodes is an explicit operational action, and administrators should treat it as a maintenance window. The cluster is expected to be fully available before and after the transition, but not necessarily during.

##### Pre-Transition

1. The cluster administrator prepares the additional control-plane nodes (hardware, network, OS)
2. The cluster administrator runs `oc adm transition topology` to begin the interactive transition flow
3. The CLI checks whether OTTO is installed; if not, it installs/activates it
4. The CLI guides the administrator through providing required configuration:
   - Node identities and access details for the new control-plane nodes
   - Any certificates or secrets required for the transition
   - Load balancing configuration (on `platform: none`, the user manages their own VIPs/DNS)
5. The CLI creates a transition CR with the provided configuration
6. OTTO validates preconditions:
   - Current topology is SingleReplica
   - Target topology is HighlyAvailable
   - Required nodes are reachable and meet minimum resource requirements
   - Required certificates and secrets are present
   - Platform is supported (`platform: none` in the initial implementation)

##### During Transition

7. OTTO signals that a transition is in progress (status on the transition CR)
8. OTTO triggers setup changes on dependent operators:
   - cluster-etcd-operator (CEO) scales etcd members sequentially following the bootstrap pattern (1→2→3)
   - Each new node joins as a learner and is promoted to a voting member before the next is added
   - The kube-apiserver, kube-controller-manager, and kube-scheduler start on new nodes via static pods managed by the kubelet
   - Ingress, networking, and other infrastructure operators prepare for multi-node operation
9. OTTO validates that all operator-specific setup steps have completed successfully
10. OTTO updates the Infrastructure status fields:
    - `controlPlaneTopology` transitions from `SingleReplica` to `HighlyAvailable`
    - `infrastructureTopology` transitions from `SingleReplica` to `HighlyAvailable`
11. Operators reconcile against the new topology values and adjust their deployment strategies, replica counts, and placement policies

##### Post-Transition

12. OTTO validates that all operators have reconciled to a healthy state
13. The transition CR is updated to reflect completion
14. The CLI reports success to the administrator

##### Failure Handling

If a transition fails partway through:

- OTTO reports the failure state on the transition CR with diagnostic information
- For etcd scaling failures, CEO attempts to roll back to the previous member count (e.g., roll back to 1 member if the 1→2→3 scale-up fails)
- The administrator can inspect OTTO logs and the transition CR for details
- The administrator can retry the transition after addressing the issue

### API Extensions

#### Transition Custom Resource

OTTO will define a custom resource for managing topology transitions:

```yaml
apiVersion: topology.openshift.io/v1alpha1
kind: TopologyTransition
metadata:
  name: sno-to-ha-compact
spec:
  targetTopology:
    controlPlane: HighlyAvailable
    infrastructure: HighlyAvailable
  nodes:
    # Node configuration provided by the administrator
    # Exact schema TBD based on OTTO implementation
status:
  phase: Pending | Validating | InProgress | Completed | Failed
  conditions:
    - type: PreflightChecksPassed
      status: "True"
    - type: EtcdScalingComplete
      status: "True"
    - type: InfrastructureUpdated
      status: "True"
  message: "Transition completed successfully"
```

#### Infrastructure Config Changes

No new enum values are added to the Infrastructure config. The existing `controlPlaneTopology` and `infrastructureTopology` fields retain their current values (`SingleReplica`, `HighlyAvailable`, `DualReplica`, `HighlyAvailableArbiter`).

OTTO updates these fields as the final step of a transition, changing them from one fixed mode to another (e.g., `SingleReplica` → `HighlyAvailable`).

A ValidatingAdmissionPolicy will be added to prevent direct edits to topology fields outside of OTTO's service account. This ensures transitions are always orchestrated rather than applied ad hoc.

#### Feature Gate

A new feature gate `MutableTopology` will be added to gate this functionality. The feature gate will progress through the following stages:

- **Dev Preview**: Part of the `DevPreviewNoUpgrade` feature set
- **Tech Preview**: Moved to the `TechPreviewNoUpgrade` feature set
- **GA**: Moved to the `Default` feature set

### Topology Considerations

#### Hypershift / Hosted Control Planes

Mutable topology is not compatible with HyperShift clusters. HyperShift uses `External` as its `controlPlaneTopology`, and topology transitions are not applicable to hosted control planes where the control plane lifecycle is managed externally.

Future support for HyperShift is not planned for this enhancement but is not ruled out.

#### Standalone Clusters

Standalone clusters are the primary target for mutable topology. This enhancement enables standalone clusters to start with minimal footprints and transition to multi-node configurations without redeployment.

`platform: none` will be supported for the initial SNO → HA compact transition. On `platform: none`, the administrator is responsible for managing their own load balancing configuration (VIPs, DNS) when scaling beyond a single node.

`platform: baremetal` support is planned for a subsequent phase pending resolution of keepalived networking for single-node clusters. The Bare Metal Networking team will be consulted to determine keepalived configuration capabilities.

#### Single-node Deployments or MicroShift

Single Node OpenShift (SNO) clusters are the primary source topology for transitions. The initial use case is enabling SNO deployments to transition to HA compact (3-node) configurations as requirements change.

OTTO ships with the payload but is not installed by default, so there is no resource impact on SNO clusters that do not use this feature.

MicroShift is not affected by this enhancement. A separate enhancement may address MicroShift-to-SNO transitions but it is unlikely it will be included as part of supported transitions in OTTO.

#### OpenShift Kubernetes Engine

This proposal does not depend on features excluded from the OpenShift Kubernetes Engine (OKE) product offering. OTTO modifies core infrastructure components — the Infrastructure API, installer, cluster-etcd-operator, and other in-payload operators — all of which are included in OKE. OKE clusters that use mutable topology will benefit from the same transition capabilities as OCP clusters.

### Implementation Details/Notes/Constraints

#### OpenShift Topology Transition Operator (OTTO)

OTTO is a new optional payload operator with the following characteristics:

- **Ships with the payload** but is **not installed by default**
- Installed either manually or via the `oc adm transition topology` command
- Owns the transition graph — the directed graph defining which topology transitions are supported
- Owns the validation criteria for each transition (required nodes, certificates, secrets, operator states)
- Orchestrates transitions by interacting with cluster operators via their existing APIs
- Updates the Infrastructure status field as the final step, after the cluster is ready for the new topology
- Reports transition status via custom resources

##### Transition Graph

OTTO maintains a directed graph of supported transitions. For the initial implementation:

```text
SingleReplica (SNO, platform: none) → HighlyAvailable (3-node compact)
```

Future transitions can be added to the graph without modifying the core operator logic. Each edge in the graph includes:

- **Preconditions**: What must be true before the transition can start
- **Configuration steps**: What OTTO must do during the transition
- **Validation criteria**: What must be true after the transition for it to be considered complete

##### Transition Orchestration

When a transition is triggered, OTTO follows this sequence:

1. **Validate preconditions** — check that the source topology, target topology, platform, and provided configuration are valid
2. **Signal transition in progress** — update the transition CR status
3. **Execute operator-specific setup**:
   - Coordinate with CEO for etcd member scaling
   - Wait for kube-apiserver, kube-controller-manager, and kube-scheduler to start on new nodes
   - Coordinate with ingress, networking, and other operators as needed. Specific requirements will be discovered as part of dev preview work.
4. **Validate operator readiness** — confirm all operators report healthy for the target topology
5. **Update Infrastructure status** — change `controlPlaneTopology` and `infrastructureTopology` to the target values
6. **Validate post-transition health** — confirm operators reconcile successfully against the new topology
7. **Report completion** — update the transition CR status

#### `oc adm transition topology` CLI Command

The CLI command provides an interactive interface for topology transitions:

- Installs/activates OTTO if not already present
- Guides the administrator through required configuration (nodes, certificates, secrets)
- Creates the transition CR with the provided configuration
- Monitors the transition CR for status updates
- Reports success or failure with diagnostic information

The CLI does not contain transition logic — it delegates entirely to OTTO. This keeps the CLI thin and avoids bloating it as more transitions are supported.

#### etcd Scaling: SNO to HA Compact

When transitioning from SNO to a 3-node compact cluster, CEO scales etcd members sequentially — the same approach used during cluster bootstrapping (1→2→3 members):

1. **Starting state**: 1 etcd voting member (quorum=1)
2. CEO adds an etcd learner on the second control-plane node
3. The learner syncs data from the existing voter via snapshot transfer
4. CEO promotes the learner to a voting member — the cluster now has 2 voting members (quorum=2)
5. CEO adds an etcd learner on the third control-plane node
6. The learner syncs data from an existing voter
7. CEO promotes the learner to a voting member — the cluster now has 3 voting members (quorum=2)
8. The cluster can now tolerate the loss of one control-plane node

This is a sequential process. The 2-member state in steps 4–5 is the primary risk window — quorum requires both members, so losing either is fatal. This window is minimized by proceeding to step 5 immediately after promotion.

The 2-member state is transient and follows the same pattern as cluster bootstrapping — a well-exercised code path.

#### Component Changes Summary

| Component | Changes Required |
| --------- | ---------------- |
| OTTO (new) | Transition operator with transition graph, validation, orchestration, and transition control CRD(s) |
| `oc` CLI | New `oc adm transition topology` interactive command |
| Infrastructure API | ValidatingAdmissionPolicy to restrict direct topology field edits |
| cluster-etcd-operator | Coordinate with OTTO for sequential etcd scaling during transitions |
| Ingress, networking, monitoring operators | Respond to OTTO coordination signals during transitions; reconcile on Infrastructure config changes |

#### Platform Support Constraints

The initial implementation targets `platform: none` clusters. On `platform: none`, the administrator is responsible for managing their own load balancing configuration (VIPs, DNS) when scaling beyond a single node.

`platform: baremetal` support is planned for a subsequent phase. Bare metal networking uses keepalived for ingress load balancing, which is not useful and creates a point of failure for SNO deployments. The Bare Metal Networking team will be consulted to determine if this networking setup can be enabled for single-node clusters transitioning to HA.

### Risks and Mitigations

#### Risk: Quorum Loss During Two-Member Transient State

**Risk**: During sequential etcd scaling (1→2→3), the cluster passes through a 2-member state where quorum=2. Losing either member during this window causes quorum loss.

**Mitigation**:
- The 2-member state is transient and follows the same sequential pattern used during cluster bootstrapping — a well-exercised code path
- Learner instances are used before promoting members to minimize the promotion window
- No availability guarantee during transitions; administrators should treat scaling operations as a maintenance window
- CEO will attempt rollback if scaling fails (e.g., rollback to 1 member if the 1→2→3 scale-up fails partway through)
- Future iterations may explore admitting two learners simultaneously and promoting only when both are ready, eliminating the 2-member voting window entirely but that is out of scope for this enhancement

#### Risk: Transition Fails Partway Through

**Risk**: A transition may fail after some operators have begun reconfiguring but before the transition completes, leaving the cluster in an intermediate state.

**Mitigation**:
- OTTO validates preconditions before starting
- OTTO sequences operations so that the Infrastructure config is updated only after all setup steps succeed
- Operators do not see a topology change until OTTO updates the Infrastructure status as the final step
- If setup steps fail, OTTO reports the failure and CEO attempts rollback for etcd
- The transition CR provides detailed status for troubleshooting

#### Risk: Platform Bare Metal May Not Support Single-Node Clusters

**Risk**: If keepalived networking cannot be enabled, `platform: baremetal` will be limited to 2+ nodes, reducing the value of mutable topology for this platform.

**Mitigation**:
- Early coordination with the Bare Metal Networking team
- `platform: none` provides full support as a fallback
- The limitation can be documented while bare metal support is resolved

#### Risk: OTTO Adds Payload Bloat

**Risk**: Adding another operator to the payload increases the overall payload size, even though OTTO is not installed by default.

**Mitigation**:
- OTTO is optional and not installed unless explicitly activated
- The alternative (embedding transition logic in the CLI or CVO) was evaluated and rejected due to scalability and separation of concerns (see [Alternatives](#alternatives-not-implemented))

### Drawbacks

#### Additional Operator in the Payload

OTTO adds a new operator to the OpenShift payload. Even though it is not installed by default, it increases the payload size. This is a deliberate trade-off to keep transition logic isolated and maintainable.

#### Coordination Across Teams

The SNO-to-HA transition requires coordination with CEO, ingress, networking, and other operator teams to ensure they respond correctly to OTTO's orchestration signals. This is less coordination than the previous Adaptable Topology approach (which required every operator to handle dynamic node-count awareness), but still significant.

#### One-Way Transitions (Initially)

The initial implementation supports only SNO → HA compact. Reverse transitions (HA → SNO) and other paths are future work. Administrators who transition cannot revert without redeploying.

## Alternatives (Not Implemented)

### Adaptable Topology (Previous Proposal)

The [Adaptable Topology proposal](https://github.com/openshift/enhancements/pull/1905) introduced a new `Adaptable` enum value for `controlPlaneTopology` and `infrastructureTopology`. Operators would dynamically react to node count changes and adjust behavior accordingly.

**Why it was replaced**:
- Required updating every core operator (30+ in-payload operators) to understand the new `Adaptable` enum and handle dynamic node-count-based behavior
- Coupled topology behavior to node count, making operator logic more complex
- Required shared library-go utilities that every operator team needed to adopt
- The `Adaptable` enum value created a paradigm that was fundamentally different from existing fixed topology modes

Mutable topology achieves the same end goal (SNO clusters can grow to HA) with less operator-side complexity. Operators continue to react to the same fixed topology values they already understand. Transition complexity is concentrated in OTTO rather than distributed across all operators.

### CLI-Only Transition Runner

An alternative is to embed all transition logic in the `oc adm transition` command without a dedicated operator.

**Why it was rejected**:
- The set of supported topologies is bounded, so the transition graph itself stays small. However, each transition is a long-running, multi-step process (etcd scaling alone takes minutes) that requires persistent state tracking a CLI process cannot reliably provide — a dropped SSH session or terminal close would leave the cluster in an intermediate state with no automated recovery
- Error recovery and retry logic is better suited to an operator's reconciliation loop than imperative CLI code
- The CLI would need direct access to operator internals, violating separation of concerns

### Extending an Existing Core Operator

Rather than introducing a new operator, transition logic could be added to an existing core operator. The most plausible candidates:

#### Controller in CVO

An alternative is to add transition controllers to the cluster-version-operator (CVO).

**Why it was rejected**:
- CVO is a critical-path operator — every cluster depends on it for updates. Adding topology transition logic increases the surface area for bugs in a component where failures have outsized blast radius
- CVO is always active and manages every cluster. OTTO is optional and only installed when topology transitions are needed. Embedding optional, long-running orchestration workflows in a required operator couples their failure modes unnecessarily
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
- Topology transitions require cross-operator coordination (etcd, ingress, networking, Infrastructure config) that is outside MCO's current scope
- Like CVO and CEO, MCO is a critical-path operator where additional surface area increases risk to every cluster

## Open Questions [optional]

1. **Transition Graph scope**: Beyond SNO → HA compact on `platform: none`, what transitions should be supported and on what platforms? The initial plan limits scope to this single path. Future transitions can be added to the graph without modifying core architecture.

2. **HyperShift considerations**: Since the scope has broadened from edge-specific to changing the topology assumption for OpenShift as a whole, do we need to consider HyperShift support? Initial answer is no — this would be future work and require its own enhancement.

3. **OTTO activation mechanism**: What is the exact mechanism for OTTO's conditional installation? Options include an OLM subscription, a CVO-managed optional deployment, or a standalone manifest applied by the CLI.

4. **Operator coordination protocol**: How does OTTO signal operators during the transition? Options include direct API calls to operator-specific endpoints, shared condition fields on the transition CR, or operator-specific CRs that OTTO creates.
The primary concern is how to trigger CEO that it should add nodes and how to update the infrastructure API so that the status fields accurately reflect the topology change after completion.

5. **ValidatingAdmissionPolicy scope**: Should the VAP prevent all direct edits to topology fields, or only edits from non-OTTO service accounts? The latter requires RBAC integration with the VAP.

6. **Learner promotion after voter failure**: If CEO runs a learner on a second control-plane node and the voter fails, can quorum restore promote the learner? Or can only former voters be restored with quorum?

## Test Plan

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
| OTTO installation | Verify OTTO installs correctly when activated |
| Precondition validation | Verify OTTO rejects transitions with missing nodes, invalid platforms, or unsupported source topologies |
| CLI interaction | Verify `oc adm transition topology` correctly interacts with OTTO |

#### Transition Tests

| Test | Description |
| ---- | ----------- |
| SNO → HA compact (3-node) | Full transition on `platform: none` with validation of etcd scaling, operator health, and Infrastructure config updates |
| etcd quorum management | Verify CEO correctly manages etcd member addition through the 1→2→3 sequence |
| Failure and rollback | Verify OTTO and CEO handle failures during transition (e.g., node unreachable, etcd promotion failure) |
| Post-transition operator health | Verify all operators reconcile successfully after the Infrastructure config is updated |

### QE Testing

Standard QE testing scenarios will include:
- OTTO installation and activation validation
- Full SNO → HA compact transition on `platform: none`
- Transition failure and recovery scenarios
- Post-transition cluster stability over 24 hours

## Graduation Criteria

### Entering Dev Preview

- OTTO operator implemented with transition CRD and SNO → HA compact graph edge
- `oc adm transition topology` CLI command implemented
- `MutableTopology` feature gate added to `DevPreviewNoUpgrade` feature set
- ValidatingAdmissionPolicy enforces controlled topology field updates
- CI lanes operational for transition testing
- Developer documentation available

### Dev Preview -> Tech Preview

- Transition test suite validates full SNO → HA compact path
- Tests verify operator health during and after transitions
- OTTO failure handling and CEO rollback validated
- `oc adm transition topology` command provides clear diagnostics on failure
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Platform bare metal single-node support resolved or limitation documented

### Tech Preview -> GA

- Full test coverage including upgrades (y-stream and z-stream) on post-transition clusters
- SLOs documented and validated
- Monitoring and telemetry for transition metrics (success/failure rates, duration)
- Support procedures documented
- Feature gate moved to `Default` feature set

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

### Upgrades

Clusters that have undergone topology transitions follow standard OpenShift upgrade procedures. The resulting topology values (`HighlyAvailable`, `SingleReplica`, etc.) are existing enum values that all operators already support. There are no special upgrade considerations for post-transition clusters.

OTTO itself upgrades as part of the payload if installed. If not installed, it has no upgrade impact.

### Downgrades

**Z-stream downgrades** (within a minor version that supports mutable topology):
Standard downgrade procedures apply. OTTO and the transition CRD are preserved. Completed transitions are not reverted — the cluster retains its current topology.

**Y-stream downgrades** (to a minor version without mutable topology support):
The CVO will evaluate the feature gate during downgrade. If the target release does not include the `MutableTopology` feature gate:
- OTTO will not be managed by the target release's CVO
- Completed transitions are not affected — the Infrastructure config contains standard topology values that the target release understands
- In-progress transitions should be completed or rolled back before downgrading

## Version Skew Strategy

Mutable topology is gated by the `MutableTopology` feature gate. OTTO is only active when the feature gate is enabled.

Version skew during transitions is not a concern because OTTO orchestrates the entire sequence within a single cluster version. Administrators should not initiate upgrades while a transition is in progress.

Post-transition clusters use standard topology values that all operator versions understand. There is no version skew risk for completed transitions.

## Operational Aspects of API Extensions

OTTO introduces a `TopologyTransition` CRD. This CRD:

- Is only created when a transition is initiated (not present on clusters that don't use mutable topology)
- Has no impact on existing SLIs when not in use
- During transitions, OTTO makes API calls to coordinate with operators. These calls are low-frequency and bounded by the transition sequence.

The ValidatingAdmissionPolicy that restricts direct topology field edits is evaluated by the API server with no additional services required. If the VAP is unavailable, the API server's existing failure policy applies.

## Support Procedures

### Team Ownership

**OpenShift Edge Team:**
- OTTO operator implementation and maintenance
- CLI (`oc adm transition topology` command)
- Transition graph definition and validation logic
- Infrastructure config ValidatingAdmissionPolicy

**Control Plane Team:**
- cluster-etcd-operator (CEO) etcd scaling coordination with OTTO

**Bare Metal Networking Team:**
- Bare metal networking for SNO clusters (future platform support)

**Component Teams:**
- Validate operator behavior during and after transitions

### Detecting Issues

**OTTO Not Installing:**
- Symptom: `oc adm transition topology` fails to activate OTTO
- Check: Verify OTTO deployment and pod status in the `openshift-topology-transition-operator` namespace
- Resolution: Check OTTO pod logs for startup failures

**Transition Stuck or Failed:**
- Symptom: Transition CR shows `InProgress` or `Failed` for an extended period
- Check: `oc get topologytransition <name> -o yaml` for status conditions and messages
- Check: OTTO pod logs for orchestration errors
- Check: CEO logs for etcd scaling operations
- Resolution: Address the reported issue and retry, or contact support

**etcd Scaling Failures:**
- Symptom: etcd cluster unhealthy after transition attempt
- Check: CEO logs for etcd scaling operations
- Check: etcd member list: `oc -n openshift-etcd exec <etcd-pod> -- etcdctl member list`
- Resolution: CEO should attempt automatic rollback. If rollback fails, follow standard etcd disaster recovery procedures.

### Recovery Procedures

| Failure Mode | Impact | Recovery |
| ------------ | ------ | -------- |
| OTTO fails during precondition check | No impact — transition not started | Address the precondition and retry |
| etcd scaling failure mid-transition | etcd may be in 2-member state | CEO attempts automatic rollback to 1 member; if that fails, follow etcd disaster recovery |
| Operator fails to reconcile post-transition | Operator-specific impact | Investigate operator logs; file bug against the operator component |
| OTTO crash during transition | Transition paused | OTTO restarts via deployment controller and resumes from last checkpoint on the transition CR |

## Infrastructure Needed [optional]

No additional infrastructure is required for this feature.

CI infrastructure will experience increased demand as new test lanes are introduced to support:
- Full SNO → HA compact transitions on `platform: none`
- Post-transition cluster stability validation
- Upgrade testing on post-transition clusters
