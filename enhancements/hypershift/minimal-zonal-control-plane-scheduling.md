---
title: minimal-zonal-control-plane-scheduling
authors:
  - "@TBD"
reviewers:
  - "@enxebre, for HCP scheduling and CPOv2 defaults"
  - "@csrwng, for HCP scheduling and API"
  - "@sjenning, for the historical zone-spread design and rollout strategy"
  - "@TBD-cluster-network-operator, for the HyperShift-mode operand rendering changes"
approvers:
  - "@TBD"
api-approvers:
  - "@TBD"
creation-date: 2026-09-03
last-updated: 2026-09-03
status: provisional
tracking-link:
  - TBD
see-also:
  - "/enhancements/hypershift/api-driven-azure-topology-and-private-connectivity.md"
---

# Minimal Zonal Control-Plane Scheduling for Hosted Control Planes

## Summary

Today every highly-available hosted-control-plane (HCP) component is hard-spread
across management-cluster availability zones (AZs) via required `podAntiAffinity`,
and the API-critical components run as 3-replica triplets. On managed platforms
such as ARO-HCP this forces the *entire* HCP onto scarce, AZ-balanced compute
quota, even for components that gain nothing from cross-AZ placement.

This enhancement adds an **opt-in** `HostedCluster` API that:

1. restricts mandatory cross-AZ spreading to only the components that actually
   need it — quorum (`etcd`), request-serving components, and blocking-webhook
   backends;
2. runs those non-quorum, highly-available components as 2-replica **pairs**
   instead of 3-replica triplets;
3. steers all remaining ("float") components onto separate **non-zonal
   (overflow)** node pools; and
4. implements the spreading with `topologySpreadConstraints` (TSC) instead of
   `podAntiAffinity`.

The feature is off by default. Existing hosted clusters — and hosted clusters on
platforms where guaranteed 3-AZ spreading is not available — are never affected.

## Motivation

On ARO-HCP the management cluster guarantees a triplet of AZ-balanced node pools
(core parity per AZ), but can obtain substantially more single-AZ capacity
cheaply. The scarce resource is therefore the *balanced* triplet, and the goal is
to minimize the compute placed on it per hosted control plane.

Analysis of the production fleet (14-day p95 request data across twelve
management clusters) shows:

- Roughly **40–55% of an HCP's CPU footprint** is components that do not need AZ
  spreading at all (leader-elected controllers, operators, catalog sources, CSI
  controllers, and similar). These are currently pinned to the balanced triplet
  only because a colocation affinity pulls them onto the same nodes as everything
  else.
- Of the compute that *does* stay zonal, reducing the non-quorum API-critical
  components from 3 replicas to 2 frees a further **~20% of the zonal footprint**,
  consistently across dev and production clusters.

Together, these let the balanced triplet host materially more hosted clusters per
unit of scarce cross-AZ capacity, while preserving AZ-failure resilience for the
components that genuinely require it.

### User Stories

- As a managed-service operator, I want to place only the AZ-failure-critical
  control-plane components on my balanced-AZ quota, so that I can host more hosted
  clusters per unit of scarce cross-AZ capacity.
- As a managed-service operator, I want to opt individual hosted clusters into
  this placement without changing behavior for any existing cluster, so that I can
  roll it out safely and reversibly.
- As an SRE, I want non-quorum control-plane components to survive an AZ loss by
  rescheduling into a spare AZ rather than sitting `Pending`, so that recovery is
  automatic.
- As an OpenShift engineer, I want the spreading mechanism to balance replicas
  evenly across the three balanced node pools, so that no single AZ pool becomes
  a hot spot as the fleet grows.

### Goals

- Provide an opt-in, per-`HostedCluster` API that bundles minimal AZ spreading,
  2-replica pairs for non-quorum HA components, and overflow placement for float
  components, implemented with `topologySpreadConstraints`.
- Guarantee zero behavior change for clusters that do not opt in and for excluded
  platforms.
- Define a clear management-cluster node contract (zonal vs overflow pools) that
  both the control-plane-operator (CPO) and the cluster-network-operator (CNO)
  honor.
- Preserve strict cross-AZ placement and 3-replica quorum for `etcd`.

### Non-Goals

- Changing the default scheduling behavior for hosted clusters that do not opt
  in.
- Supporting this feature on platforms other than those with a guaranteed
  balanced-AZ triplet plus overflow capacity. The feature is gated to an
  explicit platform **allow-list** (Azure/ARO first), rather than enabled
  everywhere and denied on a few platforms, since platforms such as bare-metal
  `None` have no availability-zone topology at all. KubeVirt and OpenStack — where
  guaranteed 3-AZ spreading is not available — are among those not on the
  allow-list and retain today's behavior.
- Coexisting with the dedicated-request-serving-nodes topology
  (`hypershift.openshift.io/topology=dedicated-request-serving-components`). The
  two are mutually exclusive.

## Proposal

Add a new optional, feature-gated field,
`HostedClusterSpec.controlPlaneAvailabilityZoneScheduling` (of type
`ControlPlaneAvailabilityZoneScheduling`), to `HostedClusterSpec`, mirrored onto
a matching `HostedControlPlaneSpec.controlPlaneAvailabilityZoneScheduling` field
(the same pattern used today for `nodeSelector` and `tolerations`). When its
`policy` is set to `Minimal`, control-plane components are partitioned into two
tiers and scheduled accordingly. The full type is defined in
[API Extensions](#api-extensions).

The `Minimal` policy deliberately bundles four coordinated behaviors — the
minimal zone-critical set, 2-replica pairs for non-quorum highly-available
components, `topologySpreadConstraints`-based spreading, and overflow placement
for the remainder — behind a single opt-in. These behaviors are synergistic, and
fusing them keeps the validation and test matrix (and the cross-repository
contract with the cluster-network-operator) tractable. `policy` is a closed enum,
so additional strategies that decompose these behaviors differently can be added
as new enum values later without breaking existing consumers.

All configuration is expressed through strongly-typed API fields validated with
CEL; this enhancement introduces no new annotations.

### Component tiers

**Zone-critical** — spread across availability zones (stays on the balanced
triplet):

- `etcd` — quorum; remains a **3-replica triplet**.
- Request-serving components — `kube-apiserver`, `oauth-openshift`, `router`,
  `ignition-server-proxy`.
- API-critical overrides — `openshift-apiserver`, `openshift-oauth-apiserver`.
  These are not formally flagged request-serving but are on the guest API path
  and are kept zone-critical deliberately.
- Blocking-webhook backends — `network-node-identity` and
  `multus-admission-controller`. Both back guest-cluster
  `ValidatingWebhookConfiguration`s that default to `failurePolicy: Fail`, so an
  AZ loss that takes them all down would block the guest apiserver
  (`nodes/status`, `pods/status`, and network-attachment-definition writes).

All non-quorum members of the zone-critical tier run as **2-replica pairs**.

**Float** — everything else (leader-elected controllers such as
`kube-controller-manager`, `kube-scheduler`, `cluster-policy-controller`,
`openshift-controller-manager`, `openshift-route-controller-manager`;
`packageserver`; `ignition-server`; `konnectivity-agent`;
`ovnkube-control-plane`; CSI controllers; and all single-replica operators and
catalog sources). Float components are steered onto **overflow** (non-zonal) node
pools.

### Scheduling primitives

`topologySpreadConstraints` replace `podAntiAffinity` for all affected
components (rationale in Alternatives and Implementation Details):

- `etcd` (a `StatefulSet`): `maxSkew=1, topologyKey=topology.kubernetes.io/zone,
  whenUnsatisfiable=DoNotSchedule, minDomains=3`. Strict: never sacrifice the
  per-AZ isolation that quorum's AZ-failure tolerance depends on. If three AZs
  are not available, prefer `Pending` over co-locating members. `matchLabelKeys`
  is not used here — it is a Deployment/ReplicaSet concept (`pod-template-hash`)
  and does not apply to StatefulSet pods, and etcd's ordinal rollout does not need
  revision-scoped spreading. This is behaviorally identical to today's required
  `podAntiAffinity` for management-node drains: in both cases a member whose AZ is
  being drained cannot reschedule elsewhere and remains `Pending` until its AZ has
  capacity, so operators continue to drain one AZ at a time. etcd is converted to
  TSC only for fleet-wide policy self-consistency; the semantics are unchanged.
- Non-quorum zone-critical pairs (Deployments): `maxSkew=1,
  topologyKey=topology.kubernetes.io/zone, whenUnsatisfiable=DoNotSchedule,
  matchLabelKeys=[pod-template-hash]`. Balanced across the three pools,
  rollout-scoped (so a new revision can surge into the spare AZ), and graceful on
  AZ loss (the lost replica reschedules into a surviving AZ rather than sitting
  `Pending`). No `minDomains`.
- Float pairs: a hard `kubernetes.io/hostname` TSC (`maxSkew=1,
  whenUnsatisfiable=DoNotSchedule`) for node-level spread, plus a best-effort zone
  TSC (`maxSkew=1, topologyKey=topology.kubernetes.io/zone,
  whenUnsatisfiable=ScheduleAnyway`). When the overflow pool spans more than one
  AZ the float replicas spread across them for free; when it does not, scheduling
  is never blocked. Float components sacrifice guaranteed AZ resilience by design
  (see Risks).
- A `kubernetes.io/hostname` TSC with `maxSkew=1` is retained for node-level
  spread on the zone-critical tier as well.

Colocation is scoped per tier under `Minimal`: the preferred `podAffinity` that
packs a hosted cluster's pods together (`setColocation`) is applied within each
tier — zone-critical pods colocate with zone-critical pods, float pods with float
pods — so it no longer pulls float pods back toward the zonal nodes and undermine
the overflow steering (particularly in soft mode).

### Node placement

Float components receive a node affinity toward the overflow pools, either
`Preferred` (soft; may spill onto zonal nodes when overflow is unavailable) or
`Required` (hard) per the API. In hard mode the balanced (zonal) pools are also
tainted so float pods cannot land there. Zone-critical components receive a node
affinity toward the zonal pools (and, in hard mode, the toleration for the zonal
taint).

The concrete node labels that identify zonal vs overflow pools, and the zonal
taint key, are **well-known constants** (following the request-serving precedent,
where `hypershift.openshift.io/request-serving-component` is a fixed label).
A single role label is used so that every control-plane node pool has exactly one
role:

- Label key: `hypershift.openshift.io/control-plane-node-role`
- Values: `zonal` (the balanced triplet) or `overflow` (non-zonal capacity)
- Hard-mode taint on the zonal pools:
  `hypershift.openshift.io/control-plane-node-role=zonal:NoSchedule`

CPO and CNO reference these constants directly when emitting node affinity, TSC,
and tolerations; the management-cluster infrastructure (ARO) labels its node
pools accordingly. No per-hosted-cluster configuration and no annotations are
involved.

Node *roles* are never carried in `HostedClusterSpec`. The hypershift-operator
discovers them by watching management-cluster `Node`s and reading these labels —
the same approach its dedicated-request-serving scheduler already uses. Because
this environmental fact (does this management cluster actually have at least three
labeled zonal AZ pools and overflow capacity?) cannot be validated at admission
time, the hypershift-operator validates it at runtime and, when the node contract
is not satisfied, sets the `ControlPlaneAvailabilityZoneSchedulingAvailable`
`HostedCluster` status condition to `False` rather than failing scheduling
silently (see Operational Aspects).

### Workflow Description

**service operator** is a human/automation responsible for the management
cluster and hosted-cluster lifecycle.

1. The management-cluster infrastructure (ARO) provisions and labels the balanced
   node pools with `hypershift.openshift.io/control-plane-node-role=zonal` and the
   overflow pool(s) with `hypershift.openshift.io/control-plane-node-role=overflow`.
   In hard mode it also taints the zonal pools with
   `hypershift.openshift.io/control-plane-node-role=zonal:NoSchedule`.
2. The service operator creates or updates a `HostedCluster` with
   `controlPlaneAvailabilityZoneScheduling.policy: Minimal` (permitted only when
   `controllerAvailabilityPolicy: HighlyAvailable`).
3. The hypershift-operator copies the field onto the `HostedControlPlane` spec
   (as it already does for `nodeSelector` and `tolerations`), and — by watching
   management-cluster `Node`s — verifies the node contract is satisfied, setting a
   `HostedCluster` condition if it is not. CPOv2 renders zone-critical components
   with the zonal TSC and zonal node affinity, renders float components with
   overflow node affinity/tolerations, and sets replica counts (`etcd` 3,
   non-quorum HA 2).
4. CNO, which already reads the `HostedControlPlane` CR, reads the same policy
   field and renders `ovnkube-control-plane` onto overflow capacity while keeping
   `network-node-identity` and `multus-admission-controller` as zone-critical
   pairs.
5. Hosted clusters that do not opt in, and hosted clusters on excluded platforms,
   render exactly as they do today.

#### Reverting

Clearing the field returns a hosted cluster to legacy scheduling. Pods adopt the
new (legacy) spec on their next rollout.

### API Extensions

This adds one optional, feature-gated field to `HostedClusterSpec`, plus an
identical field on `HostedControlPlaneSpec` that the hypershift-operator
populates by copying from the `HostedCluster` (the established pattern for
`nodeSelector`/`tolerations`). It does not add webhooks, aggregated API servers,
finalizers, or annotations. Validation is expressed as CEL on the
`HostedCluster` type.

```go
// HostedClusterSpec
// ...
// Implemented as a non-pointer struct with omitzero (not a pointer): the struct has
// a required `policy` field, so `{}` is never valid user input and no pointer is
// needed to disambiguate "unset" from "empty". This matches the repository's etcd
// sharding precedent.
// +optional
// +openshift:enable:FeatureGate=ControlPlaneAvailabilityZoneScheduling
ControlPlaneAvailabilityZoneScheduling ControlPlaneAvailabilityZoneScheduling `json:"controlPlaneAvailabilityZoneScheduling,omitzero"`

// ControlPlaneAvailabilityZoneScheduling configures how hosted-control-plane
// components are distributed across management-cluster availability zones. When
// unset, all highly-available components are spread across availability zones and
// the API-critical components run as 3-replica triplets (the historical behavior).
type ControlPlaneAvailabilityZoneScheduling struct {
	// policy selects the availability-zone spreading strategy for control-plane
	// components.
	//
	// "Minimal": only quorum (etcd), request-serving, and blocking-webhook
	// components are spread across availability zones. etcd runs as a 3-replica
	// triplet; the other zone-critical components run as 2-replica pairs. All
	// remaining components are placed on non-zonal (overflow) capacity.
	//
	// +required
	// +kubebuilder:validation:Enum=Minimal
	Policy ControlPlaneAZSchedulingPolicy `json:"policy,omitempty"`

	// nonZonalPlacement controls how strictly non-zone-critical ("float")
	// components are kept off the zonal node pools.
	//
	// "Preferred" (default): float components prefer overflow capacity but may
	// spill onto zonal nodes when overflow capacity is unavailable.
	//
	// "Required": float components must run on overflow capacity; if none is
	// available they remain Pending.
	//
	// +optional
	// +default="Preferred"
	// +kubebuilder:validation:Enum=Preferred;Required
	NonZonalPlacement NonZonalPlacementPolicy `json:"nonZonalPlacement,omitempty"`
}

type ControlPlaneAZSchedulingPolicy string

const (
	ControlPlaneAZSchedulingMinimal ControlPlaneAZSchedulingPolicy = "Minimal"
)

type NonZonalPlacementPolicy string

const (
	NonZonalPlacementPreferred NonZonalPlacementPolicy = "Preferred"
	NonZonalPlacementRequired  NonZonalPlacementPolicy = "Required"
)
```

CEL validation on the `HostedCluster`:

- `controlPlaneAvailabilityZoneScheduling` may only be set when
  `spec.controllerAvailabilityPolicy == "HighlyAvailable"` (the minimal policy is
  meaningless with a single replica) **and** the platform is Azure/ARO (the only
  platform with a guaranteed balanced-AZ triplet initially; platforms such as
  KubeVirt, OpenStack, and bare-metal `None` are not supported). These are
  spec-only constraints expressed in CEL.

  Implementation note: these two constraints are expressed as a **single combined**
  `FeatureGateAwareXValidation` rule (`... || (HighlyAvailable && Azure)`), not two
  separate rules. Two or more same-gate `FeatureGateAwareXValidation` markers on a
  heavily-validated type (`HostedClusterSpec`) make the openshift/api codegen emit
  the type's `x-kubernetes-validations` in a non-deterministic (map-iteration)
  order, which breaks `verify-git-clean`. A single combined rule generates
  deterministically. This is a codegen bug (map iteration in
  `tools/codegen/pkg/emptypartialschemas`); once fixed upstream the rules can be
  split for clearer per-constraint messages.

Mutual exclusivity with the dedicated-request-serving-nodes topology **cannot**
be expressed in CEL. That topology is selected by the
`hypershift.openshift.io/topology` annotation, and CRD validation rules can only
read `metadata.name` and `metadata.generateName` from `metadata` — not
`annotations` or `labels`. This was confirmed both by the upstream API
documentation (k8s ≥1.30: "No other metadata properties are accessible") and
empirically via an envtest spike, where the apiserver rejected a CRD whose
root rule referenced `self.metadata.annotations` (`undefined field
'annotations'`). Mutual exclusivity is therefore enforced at controller-time in
the hypershift-operator: when both the `Minimal` policy and the dedicated
request-serving topology annotation are present, the dedicated request-serving
topology takes precedence, the `Minimal` policy is not applied, and the
hypershift-operator sets `ControlPlaneAvailabilityZoneSchedulingAvailable=False`
with a reason describing the conflict.

Environmental preconditions that cannot be checked at admission time — whether
the management cluster actually has at least three labeled zonal AZ pools and
overflow capacity — are validated at runtime by the hypershift-operator and
surfaced through the same `ControlPlaneAvailabilityZoneSchedulingAvailable`
condition, not by CEL.

This field is guarded by the `ControlPlaneAvailabilityZoneScheduling` feature
gate (via the `+openshift:enable:FeatureGate=ControlPlaneAvailabilityZoneScheduling`
marker, following the existing HyperShift API convention). The type lives in the
`api` module, which is vendored and serialized independently by downstream
consumers (notably ARO-HCP), so the change follows the module's rules:
`omitempty`/pointer conventions, N-1/N+1 serialization compatibility tests,
CEL-only validation, and envtest coverage across the supported Kubernetes
versions.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is entirely HCP-specific. It changes the pod specs of
control-plane workloads that run on the **management** cluster; it does not alter
guest-cluster workloads.

Critically, it requires a coordinated change in a component that runs split
between the management and guest clusters: the **cluster-network-operator**. In
HyperShift mode the CNO runs in the hosted-control-plane namespace on the
management cluster and itself renders the `ovnkube-control-plane`,
`network-node-identity`, and `multus-admission-controller` deployments (including
their zone anti-affinity). HyperShift does not render these operands, so the new
placement for them must be implemented in the CNO, which already parses the
`HostedControlPlane` CR and will read the new policy field from it directly. See
Implementation Details.

#### Standalone Clusters

Not applicable. This enhancement is exclusively for HyperShift; the API lives on
`HostedCluster` and has no effect outside a hosted control plane.

#### Single-node Deployments or MicroShift

Not applicable. The feature is restricted to `HighlyAvailable` hosted control
planes; single-node and MicroShift deployments are single-replica.

#### OpenShift Kubernetes Engine

No impact. The feature is a HyperShift management-cluster scheduling concern and
does not depend on features excluded from OKE.

### Implementation Details/Notes/Constraints

**Control-plane-operator (CPOv2), `support/controlplane-component/defaults.go`:**

- Read the mirrored `HostedControlPlaneSpec.controlPlaneAvailabilityZoneScheduling`
  field to determine whether the `Minimal` policy and hard/soft placement apply.
- Introduce a `ZoneSpreadCritical()` classification, evaluating to true for
  `etcd`, request-serving components, the API-critical overrides
  (`openshift-apiserver`, `openshift-oauth-apiserver`), and any component backing
  a `failurePolicy: Fail` guest-apiserver webhook.
- Under the `Minimal` policy, emit `topologySpreadConstraints` (per the tiers
  above) in place of `podAntiAffinity` for the affected components, steer float
  components with overflow node affinity/tolerations, steer zone-critical
  components with zonal node affinity (plus zonal-taint toleration in hard mode),
  and scope the colocation `podAffinity` per tier.
- Adjust `DefaultReplicas`: `etcd` stays 3; non-quorum highly-available
  components become 2. The existing deployment rollout strategy already handles
  2-replica surge (`maxSurge=1`), so a 2-replica component can surge a third pod
  into the spare AZ during a rollout without dropping below two healthy replicas.

**Hypershift-operator:**

- Copy `HostedClusterSpec.controlPlaneAvailabilityZoneScheduling` onto the
  `HostedControlPlane` spec (alongside the existing `nodeSelector`/`tolerations`
  copy).
- Enforce mutual exclusivity with the dedicated request-serving topology at
  controller-time (this cannot be done in CEL — see API Extensions): if the
  `hypershift.openshift.io/topology=dedicated-request-serving-components`
  annotation is present, the dedicated topology wins, the `Minimal` policy is not
  applied, and `ControlPlaneAvailabilityZoneSchedulingAvailable` is set to
  `False`.
- Validate the node contract at runtime by watching management-cluster `Node`s
  (reusing the pattern of the dedicated-request-serving scheduler): confirm at
  least three `hypershift.openshift.io/control-plane-node-role=zonal` AZ pools and
  at least one `=overflow` pool exist, and set
  `ControlPlaneAvailabilityZoneSchedulingAvailable=False` when the contract is not
  met.

**Cluster-network-operator (separate repository — required companion change):**

- CNO already parses the `HostedControlPlane` CR in HyperShift mode (for
  `controllerAvailabilityPolicy`, `nodeSelector`, `tolerations`, and similar). It
  reads the new `controlPlaneAvailabilityZoneScheduling` field from that CR
  directly (`ParseHostedControlPlane` in `pkg/hypershift`) — no new environment
  variables or annotations — and references the same well-known zonal/overflow node
  labels and taint key as CPO (mirrored as constants in `pkg/hypershift`).
- Rather than editing the bindata templates, CNO applies a single programmatic
  transform (`applyMinimalZonalScheduling` in `pkg/network`) to the rendered
  operand Deployments at the end of `Render`, mirroring how CPOv2 mutates pod specs
  in Go. This keeps the Minimal-policy logic in one testable place instead of
  spreading conditionals across three templates. The transform, for
  `ovnkube-control-plane` (float → overflow) and `network-node-identity` /
  `multus-admission-controller` (zone-critical pairs → zonal): replaces the zone
  `podAntiAffinity` with `topologySpreadConstraints` (hard zone spread for
  zone-critical, best-effort for float; hard host spread for all), steers each
  onto the correct node pool via the `control-plane-node-role` label, adds the
  zonal toleration to zone-critical pods, scopes colocation per tier, and drops
  `network-node-identity` from three replicas to two. It is a no-op when the policy
  is unset.
- This must land in lockstep with the HyperShift change. Until the CNO change
  ships, network operands retain legacy behavior (safe; see Version Skew).

**Node contract:** the zonal/overflow node labels and the zonal taint key are
well-known constants referenced by both CPO and CNO. The management-cluster
infrastructure (ARO) is responsible for provisioning the overflow node pool(s)
and for applying these labels/taints to its node pools — this pool provisioning
is part of the overall initiative, though it is an infrastructure workstream
rather than HyperShift code. The hypershift-operator watches management-cluster
`Node`s to validate the contract at runtime and surfaces violations as a
`HostedCluster` condition; it does not create node pools itself.

**No data-plane / fleet-wide rollout impact:** this feature changes only
management-cluster control-plane pod specs (affinity, `topologySpreadConstraints`,
replica counts). It does not touch any input to the NodePool/data-plane
configuration hash — that hash is derived from `HostedCluster.spec.configuration`,
the cloud config, and the release image, none of which this field affects — so it
never triggers a NodePool (worker) rollout. The only rollout is the intended
per-hosted-cluster reschedule of control-plane pods, which occurs when (and only
when) that hosted cluster opts in. Because the field is opt-in, no existing
hosted cluster changes until its owner sets it.

### Risks and Mitigations

- **Overflow exhaustion (hard mode).** With `NonZonalPlacement: Required`, float
  pods go `Pending` if overflow capacity is unavailable. Mitigation: `Preferred`
  is the default and permits spillover onto zonal nodes.
- **Reduced replicas during an AZ outage.** A 2-replica pair drops to one replica
  when an AZ is lost. This is acceptable for stateless components and self-heals:
  the lost replica reschedules into the spare AZ. `etcd` is exempt (stays 3,
  strict).
- **kube-apiserver at two replicas requires correct API Priority & Fairness
  (AP&F) and GOAWAY tuning.** `kube-apiserver` runs as a 2-replica pair sized by
  the existing `ClusterSizingConfiguration`. Two effects must be handled or a
  single-replica failure can cascade: (a) without GOAWAY, one replica tends to
  own a disproportionate share of long-lived connections in steady state, so load
  is uneven across the pair; and (b) when one replica crashes, the surviving
  replica absorbs a reconnection storm of `LIST`+`WATCH` requests — a large CPU and
  memory spike — while a third replica spins up. Without correct GOAWAY and AP&F
  configuration this is a known cause of cascading kube-apiserver failures.
  Ensuring GOAWAY and AP&F keep the server stable at the size chosen by the
  `ClusterSizingConfiguration` is a prerequisite for enabling this feature.
- **Overflow AZ loss degrades float components.** Overflow pools often, but not
  always, span more than one AZ. When they span only one AZ, the loss of that AZ
  takes down both replicas of each float pair until capacity returns. This is an
  accepted tradeoff: the service-level objective is guest kube-apiserver
  availability, and every component on the guest API path is zone-critical and
  stays on the balanced triplet. The best-effort zone TSC on float pairs limits
  this exposure whenever overflow spans multiple AZs.
- **Node-contract misconfiguration.** If the management cluster lacks the labeled
  zonal/overflow pools (or has fewer than three zonal AZ pools), zone-critical
  and/or float pods cannot schedule. The hypershift-operator detects this at
  runtime and reports it via a `HostedCluster` condition rather than failing
  silently.
- **Blocking webhooks.** `network-node-identity` and `multus-admission-controller`
  are kept multi-AZ precisely because their `failurePolicy: Fail` gates the guest
  apiserver; their compute cost is small (~4% CPU / ~6% memory of an HCP), so
  keeping them zonal is cheap.
- **Fleet imbalance.** TSC `maxSkew=1` balances replicas across the three pools,
  closing a gap in the current `podAntiAffinity` approach (which forbids
  co-location but does not balance which zones are used).
- **Cross-repository skew.** CNO and HyperShift must ship together; see Version
  Skew Strategy.

Security review: the change is limited to scheduling constraints and node
labels/taints; no new privilege or network surface. UX review: the API is a
single opt-in field reviewed by the HyperShift and API-review teams.

### Drawbacks

- It introduces a novel, managed-platform-oriented scheduling mode plus an
  operational contract (management node labeling/tainting) that operators must
  maintain.
- It splits a hosted cluster's pods across multiple node pools, weakening the
  single-cluster colocation packing for the float tier.
- It requires a coordinated change in a second repository
  (cluster-network-operator), increasing maintenance and release-coordination
  burden.

## Alternatives (Not Implemented)

- **Keep `podAntiAffinity`; only reduce replicas and add overflow affinity.**
  This avoids the TSC migration but loses even fleet balancing and
  rollout-surge headroom, and retains the O(pods^2) scheduler cost of
  anti-affinity. Notably, per-revision zone spread was attempted with
  hand-rolled pod-template-hash labels in 2022 and reverted, because
  `topologySpreadConstraints`' `matchLabelKeys` (which now solves this natively)
  was not yet available. The minimum supported management-cluster Kubernetes
  version (1.35+) makes `matchLabelKeys` and `minDomains` available with margin.
- **Change the default globally (no opt-in).** Rejected: it would alter behavior
  for existing hosted clusters, violating the core requirement that this never
  affect current users.
- **Promote the request-serving topology annotation into a unified
  `controlPlaneTopology` enum that also expresses this mode.** Cleaner long-term
  and naturally mutually exclusive, but a larger, riskier migration for ARO-HCP,
  which relies on the existing annotation. Deferred.

## Open Questions [optional]

1. **Cross-repository PR sequencing.** Confirm the merge/release order between the
   HyperShift change and the cluster-network-operator change so the two land in
   the same payload without a regression window.

## Test Plan

- Unit tests over the matrix of `policy` x `nonZonalPlacement` x component tier,
  asserting the generated `topologySpreadConstraints`, node affinity, tolerations,
  and replica counts.
- CEL envtests for the combined HA-plus-Azure admission rule, across the
  supported Kubernetes version matrix.
- Unit tests for the controller-time mutual-exclusivity handling (dedicated
  request-serving topology takes precedence; `Minimal` is not applied; the
  `ControlPlaneAvailabilityZoneSchedulingAvailable` condition is set to `False`).
- API N-1/N+1 serialization-compatibility tests in the `api` module.
- Cluster-network-operator template and unit tests for the mirrored logic.
- End-to-end tests on a management cluster with zonal and overflow node pools:
  placement correctness, AZ-kill reschedule of a pair into the spare AZ,
  overflow-full behavior in both `Preferred` and `Required` modes, and
  "zero failed scheduling events" during an HA control-plane upgrade.
- A test confirming that opting a hosted cluster in reschedules only its
  control-plane pods and produces no NodePool (data-plane) rollout.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Feature-gated end-to-end capability on ARO-HCP integration environments.
- SLIs exposed as metrics: count of `Pending` control-plane pods (overflow
  pressure) and per-AZ commitment on the zonal pools.
- Sufficient unit/e2e coverage; user-facing documentation drafted.

### Tech Preview -> GA

- Upgrade, downgrade, and scale testing, including the cross-repo (CNO) change.
- Telemetry backhaul for HCP-per-zonal-core density.
- End-to-end tests are required for GA.
- User-facing documentation in openshift-docs.

The cluster-network-operator change graduates in lockstep with the HyperShift
change.

### Removing a deprecated feature

Not applicable. This enhancement does not deprecate or remove any existing
feature; it adds a new opt-in field whose absence preserves the current behavior.

## Upgrade / Downgrade Strategy

The field is opt-in. Upgrading HyperShift and CNO changes nothing until a
`HostedCluster` sets `controlPlaneAvailabilityZoneScheduling`. Setting the field
triggers rescheduling of the affected control-plane components on their next
rollout; clearing it reverts the cluster to legacy scheduling on the next
rollout. Making use of the feature requires a management cluster whose node pools
are labeled (and, for hard mode, tainted) per the node contract, and a CNO
version that understands the policy.

## Version Skew Strategy

HyperShift and the cluster-network-operator must both understand the policy.
Coupling is provided purely by the release payload: the cluster-network-operator
image is pinned per OpenShift release, so a given HyperShift release always
carries a matching CNO — a well-established mechanism that needs no additional
version guard. As an additional safety margin, the CNO field read is
loosely-coupled (it reads the field from the `HostedControlPlane` CR); if a CNO
that predates the change is ever paired with a newer HyperShift, the network
operands simply render with legacy multi-AZ triplets while the CPO-managed
components adopt the new placement — a correct but less complete result, never an
incorrect one. Because the minimum supported management-cluster Kubernetes
version is 1.35+, the `matchLabelKeys` and `minDomains`
`topologySpreadConstraints` fields are generally available with margin, so there
is no kubelet/scheduler skew concern for the primitives used.

## Operational Aspects of API Extensions

No conversion/admission webhooks or aggregated API servers are added; validation
is CEL on the `HostedCluster` type, evaluated at admission time.

- SLIs an administrator/support can use: number of `Pending` control-plane pods
  (overflow-pool pressure), per-AZ commitment on the zonal pools, and
  hosted-clusters-per-zonal-core density.
- Impact on existing SLIs: `topologySpreadConstraints` are cheaper for the
  scheduler than the `podAntiAffinity` they replace, so scheduling throughput
  should be neutral-to-improved. The API extension is a single optional field and
  does not affect general API throughput.
- Failure modes: overflow starvation (hard mode), node-contract mislabeling
  (zonal/overflow labels or taints missing), and conflict with the dedicated
  request-serving topology all surface through the
  `ControlPlaneAvailabilityZoneSchedulingAvailable` `HostedCluster` condition
  (and, for capacity issues, unschedulable control-plane pods and pod scheduling
  events).
- Teams likely called upon in escalation: HyperShift (scheduling/CPOv2) and the
  cluster-network-operator team.

## Support Procedures

- Detect: the `ControlPlaneAvailabilityZoneSchedulingAvailable` `HostedCluster`
  condition reporting `False` (node-contract violation or conflict with the
  dedicated request-serving topology), and `Pending` control-plane pods with
  `topologySpreadConstraints` or node affinity "didn't match" scheduling events.
  In hard mode, overflow-pool saturation presents as float pods unable to
  schedule.
- Mitigate: switch `nonZonalPlacement` from `Required` to `Preferred` to allow
  spillover, add overflow capacity, or correct the zonal/overflow node
  labels/taints.
- Disable: clear `controlPlaneAvailabilityZoneScheduling`. On the next reconcile
  and rollout, the hosted cluster returns to legacy scheduling with no data loss.
  The feature fails gracefully and resumes cleanly when re-enabled.

## Infrastructure Needed [optional]

The management-cluster infrastructure (ARO) must provision the overflow node
pool(s) and apply the well-known zonal/overflow labels (and, for hard mode, the
zonal taint) to its node pools. Standing up these pools is part of the overall
initiative but is an infrastructure workstream rather than HyperShift code. CI
must be able to provision a management cluster with distinct zonal and overflow
node pools for the end-to-end tests.
