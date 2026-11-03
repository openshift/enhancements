---
title: ingress-canary-tolerations
authors:
  - "@davidesalerno"
reviewers:
  # Ingress SME: canary behaviour, the corrected failure mode, and
  # whether a scheduling knob is the right fix at all.
  - "@Miciah"
  # Ingress: API shape, merge semantics, canary controller changes.
  - "@bentito"
  # HyperShift SME: the management/guest split and the cross-cluster
  # read described under Topology Considerations.
  - "@csrwng"
  # QE: the Test Plan, in particular whether tainting every schedulable
  # node is acceptable in shared CI lanes.
  - "@melvinjoseph86"
approvers:
  - "@Miciah"
api-approvers: # Placeholder; request assignment in #forum-api-review.
  # Consider @everettraven, who approved the parent Ingress kind in
  # gateway-api-crd-management-mode.
  - "@JoelSpeed"
creation-date: 2026-10-14
last-updated: 2026-10-20
status: provisional
tracking-link:
  # Epic: "Tolerate custom taints on Ingress canary daemonset".
  - https://redhat.atlassian.net/browse/NE-1475
  # Story implementing the API change described here.
  - https://redhat.atlassian.net/browse/NE-2928
see-also:
  - "/enhancements/ingress/ingress-fault-detection.md"
  - "/enhancements/ingress/gateway-api-crd-management-mode.md"
  - "/enhancements/ingress/optional-ingress-hypershift.md"
  - "/enhancements/network/configurable-network-diagnostics-pod-placement.md"
  - "/enhancements/dns/configurable-dns-pod-placement.md"
replaces:
superseded-by:
---

# Ingress Canary Tolerations

## Summary

This enhancement extends the cluster-wide Ingress Operator API
(`ingresses.operator.openshift.io/v1`) with a feature-gated
`spec.canary.nodePlacement.tolerations` field that lets cluster
administrators configure scheduling tolerations for the Ingress canary
DaemonSet, independently of router (IngressController) placement, so that
the canary can run on nodes carrying custom taints and ingress health
reporting stays accurate.

## Motivation

The Ingress Operator deploys a canary DaemonSet
(`openshift-ingress-canary/ingress-canary`) that serves a health-check
endpoint and is exposed through a canary route on the default router.
The operator probes that route once per minute and sets
`CanaryChecksSucceeding=False` after five successive failures, which
degrades the default IngressController after a 60 second grace period
(see
[ingress-fault-detection](/enhancements/ingress/ingress-fault-detection.md)).
Today the canary DaemonSet's pod template carries a single hardcoded
toleration, for `node-role.kubernetes.io/infra`, in the
cluster-ingress-operator's canary manifest asset:

https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/manifests/assets/canary/daemonset.yaml

That manifest is not the complete effective pod configuration. Kubernetes
adds standard DaemonSet tolerations: indefinite `NoExecute` tolerations
for `node.kubernetes.io/not-ready` and `node.kubernetes.io/unreachable`,
and `NoSchedule` tolerations for disk, memory and PID pressure and
`node.kubernetes.io/unschedulable`. Host-network pods additionally get
`node.kubernetes.io/network-unavailable:NoSchedule`; see the upstream
[DaemonSet helper](https://github.com/kubernetes/kubernetes/blob/v1.34.0/pkg/controller/daemon/util/daemonset_util.go).

The probe is a single request against the canary route, which
round-robins across the canary Service endpoints. It does not inspect the
canary DaemonSet's status, and the DaemonSet controller excludes nodes
with untolerated taints from `desiredNumberScheduled` rather than leaving
pods Pending there. Tainting *some* nodes therefore does not fail the
probe: newly created canary pods run on fewer nodes, and remaining Ready
endpoints can keep the check succeeding. Running pods are not
automatically Ready Service backends. The failure this enhancement
addresses requires the canary to have *no* eligible node at all, so that
the canary route has no endpoints and every probe fails.

That state is reachable. A cluster can dedicate tainted node pools to
ingress: independently configured router tolerations keep the default
router healthy and serving traffic on those nodes, but the
canary — whose manifest does not tolerate the custom taint — has
nowhere left to create replacement pods. A `NoSchedule` taint does not
evict existing canary pods: endpoint loss requires those pods to be
removed or otherwise cease serving. Once they are gone, the route loses
its endpoints, the probe fails, and the default IngressController is
reported `Degraded` even though ingress is working.

Whether the clusters reported in RFE-2010 are in exactly this state is
Open Question 1, and it gates this enhancement: if those clusters retain
any canary-eligible node, they are not hitting this failure.

Two distinct problems follow:

1. The operator reports a *failure* when what it actually has is *no
   signal*: it has no evidence that ingress is broken, it has lost the
   ability to observe ingress at all. Reporting `Degraded` is misleading
   and costs administrators and support engineers investigation time.
2. Administrators have no supported way to restore canary coverage on
   those nodes, so even once the misleading condition is understood, the
   health signal stays lost.

This enhancement addresses the second problem. The first is a defect in
how the canary reports an unplaceable DaemonSet, discussed under
[Alternatives](#alternatives-not-implemented); the two fixes are
complementary and should be evaluated together.

Canary-specific configuration belongs on the cluster-wide
`ingresses.operator.openshift.io` resource, scoped under
`spec.canary.nodePlacement` for future extensibility, with the initial
scope strictly limited to `tolerations`. This is the sole source of
administrator-configured tolerations for the canary; router placement
does not configure canary placement.

### User Stories

* As a cluster administrator, I want to configure tolerations for the
  ingress canary DaemonSet, so that the canary can be scheduled on nodes
  that carry custom taints (such as specialized infrastructure, GPU or
  SR-IOV operator taints) and ingress health reporting remains accurate.
* As a cluster administrator, I want canary placement to be configurable
  independently of router placement, so that I can run routers and canary
  probes on the node pools that suit each role.
* As an OpenShift support engineer, I want canary scheduling to be
  configurable through a documented, supported API, so that I can
  remediate "default IngressController degraded because the canary is
  unschedulable" cases at scale without unsupported hand-edits of the
  DaemonSet.
* As the operator of a fleet of clusters, I want canary tolerations to be
  managed declaratively through the cluster-wide Ingress configuration,
  so that day-2 operations such as re-tainting infrastructure node pools
  do not silently disable ingress health reporting across the fleet, and
  regressions surface through the `Degraded` conditions and alerts that
  are already monitored.

### Goals

* Provide a supported, validated API for cluster administrators to add
  tolerations to the canary DaemonSet pods.
* Keep canary scheduling configurable independently of
  IngressController (router) scheduling.
* Use only the fixed baseline canary tolerations when the new field is
  unset, independently of router configuration.
* Reject invalid toleration configurations at admission time with clear
  error messages.

### Non-Goals

* Adding node selectors, affinity, replica or resource controls for the
  canary. `spec.canary.nodePlacement` is structured to allow such fields
  in future enhancements, but only `tolerations` is in scope here.
* Changing IngressController (router) placement semantics or its
  `nodePlacement` API.
* Changing the canary health-check logic itself, the conditions it feeds,
  or the alerts that fire on them.
* Removing or weakening the default `node-role.kubernetes.io/infra`
  toleration that the canary carries today.

## Proposal

Extend the cluster-wide Ingress Operator configuration API
(`ingresses.operator.openshift.io/v1`) with a new `spec.canary` section
containing `nodePlacement.tolerations`, disabled by default behind a new
`IngressCanaryNodePlacement` feature gate (initially in the
`TechPreviewNoUpgrade` feature set).

The Ingress Operator watches the cluster-wide `Ingress` configuration and
reconciles the canary DaemonSet: tolerations configured under
`spec.canary.nodePlacement.tolerations` are merged into the canary pod
template, and the baseline `node-role.kubernetes.io/infra` toleration is
always preserved. Configuration can therefore only widen where the canary
may run, never restrict it below today's behavior.

The desired canary DaemonSet tolerations have exactly two sources: the
fixed baseline canary manifest and
`Ingress/cluster.spec.canary.nodePlacement.tolerations` when enabled.
Changing any IngressController's placement or tolerations must not change
the desired canary tolerations, whether custom canary tolerations are
configured or omitted. Omitted custom configuration never inherits
router tolerations. The standard tolerations Kubernetes adds to
DaemonSet pods still apply as described in Motivation. The canary still
probes the default router; placement independence does not change that
health-check relationship.

The configuration lives on `ingresses.operator.openshift.io` rather than
on `ingresscontrollers.operator.openshift.io` because the canary is a
cluster-wide singleton: it validates the default router for the whole
cluster, and exactly one canary DaemonSet exists regardless of how many
IngressControllers do.

Example configuration:

```yaml
apiVersion: operator.openshift.io/v1
kind: Ingress
metadata:
  name: cluster
spec:
  canary:
    nodePlacement:
      tolerations:
      - key: "custom.io/workload"
        operator: "Equal"
        value: "app-nodes"
        effect: "NoSchedule"
```

### Workflow Description

**cluster administrator** is a human user responsible for configuring the
cluster's ingress infrastructure.

1. Starting state: the administrator runs a cluster in which every node
   eligible to run the canary carries a custom taint (for example
   `custom.io/workload=app-nodes:NoSchedule`), while the default
   IngressController tolerates that taint and its routers are healthy.
   Any canary pods that predated the `NoSchedule` taint have already
   been removed; the taint alone does not evict them.
   The canary DaemonSet has `desiredNumberScheduled: 0`, the canary route
   has no endpoints, and after five successive probe failures the default
   IngressController reports `Degraded`.
2. The administrator enables the `TechPreviewNoUpgrade` feature set on
   the cluster. This enables the `IngressCanaryNodePlacement` feature
   gate and exposes `spec.canary` in the `Ingress` CRD schema.
3. The administrator edits the cluster-wide Ingress configuration
   (`oc edit ingresses.operator.openshift.io/cluster`) and adds the
   desired tolerations under `spec.canary.nodePlacement.tolerations`.
4. The API server validates the submitted tolerations against the
   feature-gated CRD schema; malformed tolerations (for example an
   invalid, omitted or empty `operator` or `effect`, or a `value`
   specified together with `operator: Exists`) are rejected with an
   error.
5. The Ingress Operator observes the updated configuration and updates
   the canary DaemonSet pod template, merging the configured tolerations
   with the baseline tolerations.
6. The canary pods become schedulable on the tainted nodes, the canary
   route regains endpoints, the next probe succeeds, and
   `CanaryChecksSucceeding` returns to `True`, clearing `Degraded` on the
   default IngressController. The `Available` condition is not involved:
   the canary check feeds only the degraded computation, so the
   IngressController was `Available` throughout.

```mermaid
sequenceDiagram
    participant Admin as Cluster administrator
    participant API as kube-apiserver
    participant CIO as Ingress Operator
    participant DS as Canary DaemonSet

    Admin->>API: Enable TechPreviewNoUpgrade feature set
    Admin->>API: Update Ingress/cluster (canary tolerations)
    API->>API: Validate tolerations (feature-gated schema)
    API-->>Admin: Configuration accepted
    CIO->>API: Observe Ingress/cluster update
    CIO->>DS: Update pod template (merge tolerations)
    DS->>DS: Rolling update; canary pods schedule on tainted nodes
    CIO->>CIO: Probe succeeds; CanaryChecksSucceeding=True
    CIO-->>API: Default IngressController Degraded=False
```

#### Error handling: tolerations that match no taint

If the administrator submits tolerations that match no existing taint,
admission still succeeds, because the list is syntactically valid. The
canary remains at least as schedulable as before: the baseline
`node-role.kubernetes.io/infra` toleration is preserved and no node
selector is introduced. The administrator can correct the field at any
time. Clearing it means removing the entire `spec.canary` object:
`tolerations: []` is rejected by `MinItems=1`, empty `canary` and
`nodePlacement` objects by `MinProperties=1`, and `spec` itself must
retain at least one field such as `gatewayAPI`.

#### Error handling: rolling the canary DaemonSet

Any change to the effective ordered toleration list mutates the canary
pod template and therefore rolls the DaemonSet (`RollingUpdate`,
`maxUnavailable: 10%`, rounded up),
recreating every canary pod. The probe tolerates this: it fails the
condition only after five successive failures at one minute apart, plus a
60 second grace period, so a roll that converges within roughly five
minutes does not degrade the IngressController. Two cases need care and
must be covered by tests:

* On single-node clusters `maxUnavailable: 10%` rounds up to the only
  pod, so the canary signal is briefly absent entirely.
* If the new pod template is unschedulable, the roll does not converge
  and the canary can end with no running pods, which degrades the
  IngressController after the interval above.

### API Extensions

This enhancement modifies the existing `ingresses.operator.openshift.io`
CRD (group `operator.openshift.io`, version `v1`, kind `Ingress`):

* Adds `spec.canary` (`IngressCanary`), containing
  `spec.canary.nodePlacement` (`CanaryNodePlacement`), containing
  `spec.canary.nodePlacement.tolerations`
  (`[]CanaryToleration`, `+listType=atomic`). `CanaryToleration` is a
  dedicated toleration type rather than `corev1.Toleration`, so the
  schema itself can require `operator` and `effect` and reject `Lt` and
  `Gt`.
* The new fields use
  `+openshift:enable:FeatureGate=IngressCanaryNodePlacement`; generated
  schema availability and runtime gate state are distinct, as described
  under [Dependency on the parent kind](#dependency-on-the-parent-kind).
* No new CRDs, admission or conversion webhooks, aggregated API servers
  or finalizers are added. Validation is enforced by the kube-apiserver
  through the CRD schema (OpenAPI v3 validation and CEL rules).
* The field is optional. When it is unset, desired canary tolerations
  contain only the fixed baseline, not router tolerations. No resources
  owned by other parties are modified.

#### Dependency on the parent kind

The resource being extended is itself gated, and this constrains the
whole graduation path. `operator/v1/types_ingress.go` carries, at the
type level, `+openshift:capability=Ingress` and
`+openshift:enable:FeatureGate=GatewayAPIManagementMode`. Before this
enhancement the kind generated a single merged manifest,
`0000_50_ingress_02_ingresses.crd.yaml`, annotated:

```yaml
capability.openshift.io/name: Ingress
release.openshift.io/feature-set: CustomNoUpgrade,DevPreviewNoUpgrade,TechPreviewNoUpgrade
```

There is no `Default` variant, in contrast to `ingresscontrollers`. The
kind exists because of
[gateway-api-crd-management-mode](/enhancements/ingress/gateway-api-crd-management-mode.md),
and `GatewayAPIManagementMode` is registered with
`enable(inTechPreviewNoUpgrade(), inDevPreviewNoUpgrade())`. The
`IngressSpec` promotion to `v1` landed in openshift/api PR #2890.

Consequences that this enhancement must respect:

* **Disabling `IngressCanaryNodePlacement` is not the only way the field
  can be absent.** On a Default cluster the entire CRD is not installed,
  so `oc get ingresses.operator.openshift.io cluster` returns `the
  server doesn't have a resource type …` rather than an object without
  `spec.canary`. Support procedures must not read that as "the feature
  is off but configurable".
* **The two gates do not cover the same feature sets, so the proposed
  CRD needs three variants.** The parent kind is served in
  `CustomNoUpgrade`, `DevPreviewNoUpgrade` and `TechPreviewNoUpgrade`,
  while `IngressCanaryNodePlacement` enables only in
  `TechPreviewNoUpgrade`. Because the contents now differ per feature
  set, codegen is expected to replace the single merged manifest with
  `0000_50_ingress_02_ingresses-{CustomNoUpgrade,DevPreviewNoUpgrade,TechPreviewNoUpgrade}.crd.yaml`.
  `spec.canary` is present in the `TechPreviewNoUpgrade` variant and in
  the `CustomNoUpgrade` variant (which carries every gated field so an
  administrator can enable any gate), and is **absent** from the
  `DevPreviewNoUpgrade` variant, which is what the former merged
  manifest is renamed to. The implementation PR generates exactly these
  three variants; generated schemas are still not runtime gate-state
  guarantees.
* **Schema availability and runtime gate state are separate.** In the
  proposed `DevPreviewNoUpgrade` schema, `Ingress/cluster` is readable
  but `spec.canary` is not declared. For an unknown field, a request with
  `fieldValidation=Strict` is rejected; `Warn` accepts with a warning and
  prunes the field; `Ignore` accepts and prunes without that warning.
  Read the object back after applying it. In contrast, a
  `CustomNoUpgrade` schema can declare the field even when the runtime
  gate is off: admission can persist it, but the controller ignores it.
  Do not infer runtime gate state solely from schema presence or absence.
* **Graduating this feature to Default requires
  `GatewayAPIManagementMode` to graduate first**, or the parent kind's
  gating to be lifted independently. Flipping only
  `IngressCanaryNodePlacement` to `Default` would ship a field on a CRD
  that is not installed, and the feature would silently no-op. This is a
  cross-feature dependency owned by another workstream and is recorded
  as a graduation criterion below.
* **When the `Ingress` capability is disabled the CRD is not served at
  all**, which is a cleaner boundary than the canary namespace merely
  being absent.

Whether this field rides the Gateway API management-mode promotion or
graduates separately is Open Question 2. Until that is settled, the
Tech Preview -> GA path below cannot name target releases.

The API is modeled by the following Go types in
`github.com/openshift/api/operator/v1`. The implementation is
https://github.com/openshift/api/pull/3115 (open; not yet approved),
and the types below reproduce its shape and validation markers with
doc comments abbreviated: the PR is authoritative for the exact field
documentation and generated schemas, and this section must be updated
if review changes them.

```go
// IngressSpec is the spec of the cluster-wide Ingress Operator
// configuration. gatewayAPI already exists; canary is added here.
// +kubebuilder:validation:MinProperties=1
type IngressSpec struct {
	// (existing field, shown for context)
	// +optional
	GatewayAPI GatewayAPIIngressConfig `json:"gatewayAPI,omitzero"`

	// canary holds configuration for the ingress canary daemonset (the
	// ingress-canary DaemonSet in the openshift-ingress-canary
	// namespace). When specified, canary must set at least one of its
	// fields; an empty object is rejected.
	//
	// +optional
	// +openshift:enable:FeatureGate=IngressCanaryNodePlacement
	Canary IngressCanary `json:"canary,omitzero"`
}

// IngressCanary holds configuration for the ingress canary daemonset.
// +kubebuilder:validation:MinProperties=1
type IngressCanary struct {
	// nodePlacement enables explicit control over the scheduling of the
	// ingress canary daemonset pods. Because canary requires at least
	// one field, nodePlacement can only be absent when canary itself is
	// absent; when specified, an empty object is rejected.
	//
	// +optional
	NodePlacement CanaryNodePlacement `json:"nodePlacement,omitzero"`
}

// CanaryNodePlacement describes node placement configuration for the
// ingress canary daemonset pods.
// +kubebuilder:validation:MinProperties=1
type CanaryNodePlacement struct {
	// tolerations is a list of tolerations applied to the canary
	// daemonset pods in addition to the tolerations the canary daemonset
	// has by default. The default toleration for the
	// node-role.kubernetes.io/infra taint is always preserved. When
	// specified, the list must contain at least 1 toleration; to remove
	// all additional tolerations, omit the entire spec.canary field
	// rather than setting an empty list or leaving empty parent objects.
	//
	// Per-entry constraints are enforced by the markers below and the
	// CEL rules, which are summarised in the table after this block.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=32
	// +kubebuilder:validation:XValidation:rule=`self.all(t, has(t.operator) && t.operator in ['Exists', 'Equal'])`,message="tolerations operator must be 'Exists' or 'Equal'"
	// +kubebuilder:validation:XValidation:rule=`self.all(t, has(t.effect) && t.effect in ['NoSchedule', 'PreferNoSchedule', 'NoExecute'])`,message="tolerations effect must be 'NoSchedule', 'PreferNoSchedule', or 'NoExecute'"
	// +kubebuilder:validation:XValidation:rule=`self.all(t, !has(t.operator) || t.operator != 'Exists' || !has(t.value) || t.value == '')`,message="tolerations value must be empty when operator is 'Exists'"
	// +kubebuilder:validation:XValidation:rule=`self.all(t, has(t.key) && t.key != '')`,message="tolerations key must be non-empty"
	// +kubebuilder:validation:XValidation:rule=`self.all(t, !has(t.key) || !format.qualifiedName().validate(t.key).hasValue())`,message="tolerations key must be a qualified label key with a name of 1-63 characters and an optional DNS prefix of at most 253 characters"
	// +kubebuilder:validation:XValidation:rule=`self.all(t, !has(t.operator) || t.operator != 'Equal' || !has(t.value) || !format.labelValue().validate(t.value).hasValue())`,message="tolerations Equal value must be a valid label value of at most 63 characters"
	// +kubebuilder:validation:XValidation:rule=`self.all(t, !has(t.tolerationSeconds) || (has(t.effect) && t.effect == 'NoExecute'))`,message="tolerations effect must be 'NoExecute' when tolerationSeconds is set"
	Tolerations []CanaryToleration `json:"tolerations,omitempty"`
}

// CanaryToleration describes a toleration added to the ingress canary
// daemonset. Unlike corev1.Toleration, the fields the canary contract
// requires are schema-required, and the operator enum admits only
// Exists and Equal.
type CanaryToleration struct {
	// key is required and identifies the taint to tolerate; see the key
	// syntax rules below.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=317
	Key string `json:"key,omitempty"`

	// operator is required and specifies how the taint value is matched.
	// +required
	Operator CanaryTolerationOperator `json:"operator,omitempty"`

	// value is optional and defaults to the empty string when omitted;
	// it must be empty for Exists and a label value for Equal. Omitted
	// and explicit empty values are both accepted and preserved.
	// +optional
	// +kubebuilder:validation:MinLength=0
	// +kubebuilder:validation:MaxLength=63
	Value *string `json:"value,omitempty"`

	// effect is required and specifies the taint effect to match.
	// +required
	// +kubebuilder:validation:Enum=NoSchedule;PreferNoSchedule;NoExecute
	Effect corev1.TaintEffect `json:"effect,omitempty"`

	// tolerationSeconds is optional and may be set only for effect
	// NoExecute; zero and negative values are preserved.
	// +optional
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty"`
}

// CanaryTolerationOperator describes how an ingress canary toleration
// matches the value of the taint it tolerates.
// +kubebuilder:validation:Enum=Exists;Equal
// +enum
type CanaryTolerationOperator string
```

The shape choices are deliberate and follow the sibling `gatewayAPI`
field on the same spec: `omitzero` rather than `omitempty`, because
`omitempty` has no effect on a struct and would serialize `canary: {}`
into every object; value types rather than pointers for `canary` and
`nodePlacement`, because an absent `nodePlacement` and an empty one mean
the same thing; and `MinProperties=1` on each struct so that an empty
object cannot be written and silently satisfy the spec's own
`MinProperties=1`. Inside `CanaryToleration`, `value` and
`tolerationSeconds` are pointers so that an omitted field is
distinguishable from an explicit zero value; the API applies no
defaulting, so omitted and explicit empty `value` strings, and omitted,
zero and negative `tolerationSeconds`, are all accepted and preserved.

The implementation defines a dedicated `CanaryToleration` type rather
than reusing `corev1.Toleration`. A bare `corev1.Toleration` imports no
Kubernetes Pod admission validation — upstream type comments supply
neither validation nor defaults — and its schema cannot require
`operator` and `effect` or reject `Lt` and `Gt`; the published OpenAPI
enum would also advertise `Lt` and `Gt` through the corev1 const block.
The dedicated type keeps the whole contract in the CRD schema itself.
The implemented constraints on the new field are:

| Constraint | Mechanism | Rationale |
| --- | --- | --- |
| `operator` is `Exists` or `Equal` | required field; enum on `CanaryTolerationOperator`; CEL | explicit operator contract; rejects omitted, empty, `Lt` and `Gt` |
| `effect` is `NoSchedule`, `PreferNoSchedule` or `NoExecute` | required field; schema enum; CEL | explicit effect contract; rejects omitted and empty |
| `value` empty when `operator` is `Exists` | CEL | matches Kubernetes toleration semantics |
| `key` is a non-empty Kubernetes qualified label key | required field; `MinLength=1`/`MaxLength=317`; CEL `format.qualifiedName()` | rejects a keyless `Equal` and empty-key `Exists`, as well as malformed keys |
| `value` for `Equal` is a Kubernetes label value | `MaxLength=63`; CEL `format.labelValue()` | rejects values that Pod admission would reject |
| `tolerationSeconds` only with `NoExecute` | CEL | time-bounding is meaningless for other effects |
| 1 to 32 entries | `MinItems`, `MaxItems` | bounded list; `[]` is not a second spelling of unset |
| at least one property per struct | `MinProperties=1` | an empty object carries no meaning |

The table requires explicit operator and effect values, and the
implementation settles the previously open handling question: omitted
and empty `operator` and `effect` are rejected, with no schema defaults.
Upstream Pod
[toleration semantics](https://github.com/kubernetes/api/blob/v0.34.0/core/v1/toleration.go)
also accept an empty operator as `Equal` and an empty effect as matching
all effects, but those request-time matching rules do not translate
into stored-object defaults, and requiring both fields keeps the
persisted contract explicit. The API PR tests omitted and empty
operator and effect on both create and update.

Key validation must match Kubernetes qualified label-key syntax, not
merely require a non-empty string: an optional lowercase DNS-subdomain
prefix of at most 253 characters followed by `/`, and a required name
of 1 to 63 characters. The name starts and ends with an ASCII
alphanumeric character and may contain ASCII alphanumerics, `-`, `_`
and `.` internally. The prefix must satisfy Kubernetes DNS-subdomain
syntax; empty prefixes, uppercase prefix characters and extra `/`
characters are invalid. The maximum combined key length is 317
characters (253 + 1 + 63), not 63 or 253 for the entire key. Use the
Kubernetes helper's prefix checks rather than inventing additional
per-component limits. An `Equal` value may be empty; otherwise it
has the same character and boundary rules as the name, with a maximum
of 63 characters. These constraints follow Kubernetes
[label validation](https://github.com/kubernetes/apimachinery/blob/v0.34.0/pkg/util/validation/validation.go).
They are implemented by the schema bounds and CEL format validators in
the table above, which `corev1.Toleration` alone does not supply, and
are tested at the length and character boundaries in the API PR.

**The tolerate-everything form is rejected.** A toleration with an empty
`key` and `operator: Exists` tolerates every taint. `CONVENTIONS.md`
states that an operand meeting neither of its two listed exceptions
"must not tolerate all taints", enforced by a CI test in
openshift/origin, and the canary is a health prober that meets neither
exception. Permitting it would also bypass all custom taint isolation,
beyond the standard tolerations already added to DaemonSet pods. The
`key` rule is therefore tightened to
require a non-empty key unconditionally, rather than exempting
`operator: Exists`, and the implementation enforces and tests it. This
is deliberately decided before the API merges, because loosening the
rule later is backward compatible while tightening it later would not
be.

The built-in `node.kubernetes.io/*` condition taints (`not-ready`,
`unreachable`, `unschedulable`, and the pressure taints) are **not**
rejected by validation, because a blanket prohibition risks blocking
legitimate configurations. Most are already tolerated by effective
DaemonSet pods as described in Motivation, so listing them here does not
necessarily widen placement. Additional custom tolerations or different
effects can widen placement; toleration alone neither proves a node can
serve nor makes a pod a Ready Service backend. Administrators must
consider node health, readiness and drain behavior rather than treating
an explicit condition-taint entry as a new exemption created by this API.

**Duplicate tolerations are permitted at admission and normalised by the
controller.** This matches upstream `PodSpec` semantics, where duplicate
tolerations are legal. Admission accepts them; the canary controller
collapses only fully identical entries, comparing `key`, `operator`,
`value`, `effect` and `tolerationSeconds`, including nil versus a finite
duration. It preserves the first occurrence in source order: fixed
baseline canary tolerations, then canary-specific custom tolerations.
Distinct durations and overlapping `Exists`/`Equal` matches are retained,
not sorted.

This is a conservative design policy, not a claim that Kubernetes takes
the minimum duration across duplicate tolerations. Upstream
[`MatchToleration`](https://github.com/kubernetes/api/blob/v0.34.0/core/v1/toleration.go)
ignores `tolerationSeconds` and is therefore insufficient for this
deduplication. Kubernetes
[`GetMatchingTolerations`](https://github.com/kubernetes/kubernetes/blob/v1.34.0/pkg/apis/core/v1/helper/helpers.go)
selects the first matching toleration for each taint and breaks out of
the inner loop; the eviction controller computes a
[minimum](https://github.com/kubernetes/kubernetes/blob/v1.34.0/pkg/controller/tainteviction/taint_eviction.go)
over only those selected per-taint matches. Reordering overlapping
matches can therefore change behavior. Ordered full-field comparison
and exact-only deduplication are proposed here; they are not a tested
controller implementation.

### Topology Considerations

#### Hypershift / Hosted Control Planes

With Hosted Control Planes the ingress operator is **split across both
clusters**, so this enhancement's read path and write path land on
opposite sides of that split.

* **Management cluster.** The ingress operator itself runs here. The
  control-plane operator declares it as a control-plane component
  (`NewDeploymentComponent("ingress-operator", …)`, predicated on
  `isIngressCapabilityEnabled`) and reconciles it as a Deployment in the
  HostedControlPlane namespace. HyperShift deliberately filters the
  ingress operator's Deployment out of the guest CVO payload, because
  the operator is considered part of the hosted control plane; see
  [optional-ingress-hypershift](/enhancements/ingress/optional-ingress-hypershift.md).
  The component injects a konnectivity container and mounts a guest
  service-account kubeconfig for the `ingress-operator` service account
  in the guest's `openshift-ingress-operator` namespace.
* **Guest cluster.** The `ingresses.operator.openshift.io` CRD, the
  operator's RBAC, and all of the canary's resources — the
  `openshift-ingress-canary` namespace, the canary DaemonSet, Service
  and route — live here. The CRD and RBAC are delivered by the guest
  CVO, which does still receive the ingress operator's API manifests.

Consequences specific to this enhancement:

* Reading `spec.canary.nodePlacement.tolerations` is a **cross-cluster
  call**: an operator pod on the management cluster reads the singleton
  from the guest kube-apiserver over konnectivity, then writes the
  guest's canary DaemonSet. The new watch must tolerate konnectivity
  being unavailable, and must not crash-loop or degrade when it is.
* The CRD and the operator's RBAC arrive from the guest CVO while the
  consuming code arrives from the management-side control-plane
  operator. These are **separately versioned**, so a window exists in
  which the operator understands the field but the guest CRD does not
  yet serve it, or vice versa. The operator must distinguish confirmed
  absence from discovery, cache, permission and transport errors. On
  errors it preserves the existing canary template and placement and
  retries, following the read-error contract in Implementation Details.
  This is the same
  ordering hazard the Gateway API work hit on this very resource
  (OCPBUGS-78539, OCPBUGS-32373), where the requirement was that the
  operator tolerate a transient lack of permission rather than
  crash-loop.
* When the Ingress capability is disabled for a HostedCluster, the
  operator is not deployed, the CRD is not in the guest payload, and the
  canary namespace does not exist. The field is then simply absent,
  which is the correct behaviour and needs no special handling.
* Enabling the feature requires the **guest** cluster to run with
  `TechPreviewNoUpgrade`.

No change is expected in openshift/hypershift itself: the ingress
operator's cluster role already grants get/list/watch on
`operator.openshift.io/ingresses` and ships in the guest CVO payload,
and the konnectivity path already exists. This is Open Question 3,
together with whether Hosted Control Planes are in scope for Tech
Preview. The Test Plan below does not cover this topology, so it stays
unvalidated until a lane exists.

#### Standalone Clusters

This is the primary target of the enhancement. The Ingress Operator runs
in the `openshift-ingress-operator` namespace, manages the canary
DaemonSet in `openshift-ingress-canary`, and reconciles the new field
from the cluster-wide `Ingress` configuration as described in the
proposal. No standalone-specific considerations beyond those already
described.

#### Single-node Deployments or MicroShift

The enhancement adds an optional API field and changes no pods or
resources: the canary DaemonSet already runs on single-node clusters
(its pods request 10m CPU and 20Mi of memory per node), so there is no
additional resource consumption on SNO and the default behavior is
unchanged. The feature can in fact help SNO administrators who taint
their single node to keep the canary, and thus ingress health reporting,
running.

MicroShift does not deploy the ingress canary (its e2e suite explicitly
skips canary-related tests) and does not expose the Ingress Operator
API, so this enhancement does not apply to MicroShift and nothing needs
to be surfaced in the MicroShift configuration file.

#### OpenShift Kubernetes Engine

Routing is included in OKE, and this enhancement does not depend on any
capability that is excluded from the OKE product offering (such as the
web console or the monitoring stack). The field is therefore available
on OKE clusters that enable the feature set, with the same behavior as
OCP. See the comparison of OKE and OCP in the product documentation:

https://docs.redhat.com/en/documentation/openshift_container_platform/latest/html/overview/oke-about#about_oke_similarities_and_differences

### Implementation Details/Notes/Constraints

* In openshift/api, define `IngressCanary`, `CanaryNodePlacement`,
  `CanaryToleration` and `CanaryTolerationOperator` in `operator/v1`,
  add `Canary` to `IngressSpec`, and regenerate the CRD manifests,
  deepcopy functions, OpenAPI schemas and feature-gated CRD variants
  with `make update`; `make verify` must pass. Proposed in
  https://github.com/openshift/api/pull/3115.
* Register the `IngressCanaryNodePlacement` feature gate in
  https://github.com/openshift/api/blob/master/features/features.go
  with `enable(inTechPreviewNoUpgrade())`,
  `reportProblemsToJiraComponent("Routing")`, a contact person,
  `productScope(ocpSpecific)`, and a link to the merged enhancement PR.
  Per the OpenShift dev guide, all new features must start disabled by
  default behind a feature gate. The gate is deliberately narrower than
  the parent kind's `GatewayAPIManagementMode`, which also enables in
  `DevPreviewNoUpgrade`; see
  [Dependency on the parent kind](#dependency-on-the-parent-kind) for
  what that asymmetry implies. The gate is registered this way in the
  API PR.
* Validation. The constraints under
  [API Extensions](#api-extensions) are enforced by the schema markers
  and CEL rules implemented in the API PR, including the unconditional
  non-empty qualified label key; Pod admission is not involved in
  validating the configuration object.
* In the cluster-ingress-operator, the canary controller merges the
  configured tolerations into the canary DaemonSet pod template, always
  preserving the baseline `node-role.kubernetes.io/infra` toleration,
  and only honors the field while the feature gate is enabled.
* Merge order and deduplication follow the exact-only policy in
  [API Extensions](#api-extensions): fixed baseline then canary-specific custom
  entries, preserving first occurrence and overlapping matches. Compare
  the resulting ordered canonical lists using full-field equality,
  including `tolerationSeconds` and its nil state.
* Fix the existing toleration comparator before this field ships.
  `canaryDaemonSetChanged` passes `cmpTolerations`, an equality
  predicate, to `cmpopts.SortSlices`, which requires a strict weak
  ordering. Equality is not a valid less-than function and can panic
  go-cmp on identical entries. Remove this sorting path rather than
  introducing a different sort: compare ordered canonical desired and
  live lists with full-field equality, including `tolerationSeconds`.
  Identical input must not cause perpetual diffs; a genuine order change
  must remain visible because overlapping matches can be order-sensitive.
* Add a watch on `ingresses.operator.openshift.io` to the canary
  controller, mapped to the default IngressController, and thread the
  merged tolerations through `ensureCanaryDaemonSet` and
  `desiredCanaryDaemonSet`. RBAC already grants get/list/watch on this
  resource, so no cluster role change is required. A confirmed gate-off
  state ignores custom tolerations and uses baseline-only placement.
  With the gate enabled and the kind served, an authoritative API
  NotFound for the singleton after cache synchronization is treated as
  unset. A confirmed unserved kind instead means the feature is
  unavailable, with baseline-only placement; startup must not wait
  forever for an informer for an API that is not served. Watch lifecycle
  handling must support that case and rediscover an API that becomes
  served; the implementation mechanism still needs review. Do not
  equate a discovery failure, stale or unsynchronized cache, failed
  list/watch, timeout, forbidden response or
  other read error with authoritative absence. On those errors retain
  the existing DaemonSet template and last applied placement, report the
  reconcile error and requeue with backoff, without crash-looping or
  introducing a new ingress degradation solely for the read failure.
  At startup before cache synchronization, do not render an empty custom
  list over an existing DaemonSet; wait and retry rather than treating
  an unsynchronized cache as absence. The existing canary health checks
  remain unchanged.
* Router placement and tolerations are not inputs to the desired canary
  tolerations. Existing default IngressController watches, reconciliation
  mapping and route probing remain in use; those relationships must not
  introduce placement inheritance.
* Add openshift/api integration test suites (YAML suites under
  `operator/v1/tests/ingresses.operator.openshift.io/`) covering valid,
  invalid and omitted toleration configurations with the feature gate
  enabled and disabled, as implemented in the API PR and described in
  the [Test Plan](#test-plan). These suites assert admission outcomes
  only. A duplicate list is one of the valid cases, because the CEL
  rules deliberately accept duplicates; collapsing them is controller
  behaviour and is covered by unit tests instead. The gate-disabled
  suite required extending the shared test framework with per-test
  `fieldValidation` directives and `expectedWarnings` assertions.

### Risks and Mitigations

* Risk: an administrator configures tolerations that do not match their
  taints and the canary still does not run where they expect.
  Mitigation: the baseline toleration is preserved and no node selector
  is introduced, so the canary remains at least as schedulable as today,
  and removing the configuration always restores the default; the
  semantics are documented.
* Risk: widening placement is not unconditionally safe. Custom taints
  can isolate nodes that are unsuitable for canary service. Standard
  condition-taint tolerations already apply to DaemonSet pods; this API
  does not newly introduce those defaults. Tolerations permit placement,
  not readiness: a pod must become a Ready Service backend before it
  normally receives route traffic. A Ready backend with intermittent
  failures can still affect probes. Mitigation: reject the
  tolerate-everything form, document the existing DaemonSet defaults and
  require administrators to assess node health, isolation and drain
  behavior when adding custom tolerations.
* Risk: a `NoExecute` toleration carrying `tolerationSeconds` makes
  canary placement time-bounded, so the canary can be evicted later and
  the cluster silently returns to the failing state. Mitigation:
  document that `tolerationSeconds` should be omitted for canary
  tolerations, and surface the resulting condition as for any other
  unplaceable canary.
* Risk: divergence between feature-gated CRD schema variants.
  Mitigation: variants are generated by `make update` in openshift/api
  and enforced by `make verify` plus integration tests for both gate
  states.
* Security: the field only influences scheduling of the existing canary
  pods; it grants no new privileges, and the canary pods keep their
  current SCC (`restricted-v2`), seccomp profile and security context.
  Security review happens through the standard enhancement and API
  review process.
* UX: the field is administrator-facing and configured through the API;
  user-facing documentation in openshift-docs is part of graduation, and
  no console changes are planned (non-goal).

### Drawbacks

* The enhancement adds an API surface to maintain (feature gate, CRD
  variants, validation and tests) for a narrow scheduling concern.
* The tolerations-only scope leaves administrators who need node
  selectors for the canary unsupported until a follow-up enhancement
  extends `spec.canary.nodePlacement`.
* A cluster-wide singleton knob is a coarser instrument than per-router
  configuration; this is accepted deliberately (see Alternatives).

## Alternatives (Not Implemented)

* Stop reporting `Degraded` when the canary cannot be placed at all.
  When the canary DaemonSet has `desiredNumberScheduled: 0`, the operator
  has no evidence that ingress is broken; it has no signal. Reporting
  `CanaryChecksSucceeding=Unknown` with a distinct reason (for example
  `NoSchedulableNodes`), and not driving `Degraded` from it, states the
  truth and removes the misleading condition for every affected cluster
  with no API, no feature gate and no dependency on the parent CRD's
  graduation, and is backportable. There is direct OpenShift precedent:
  [configurable-network-diagnostics-pod-placement](/enhancements/network/configurable-network-diagnostics-pod-placement.md)
  introduced the `NetworkDiagnosticsAvailable` condition for exactly this
  reason, on the grounds that an informational diagnostic that cannot be
  placed should not drive `Available` and `Degraded`.
  This is **not** rejected: it is complementary and arguably should land
  first. It is listed here because it resolves the misleading condition,
  whereas this enhancement restores the lost health signal. Shipping only
  this alternative would leave administrators with accurate reporting but
  no canary coverage on tainted pools.
* Have the canary tolerate all taints by default, as
  `network-check-target` does today and as `dns-default` did before 4.8
  (see
  [configurable-dns-pod-placement](/enhancements/dns/configurable-dns-pod-placement.md)).
  This is the established pattern for diagnostic DaemonSets and would fix
  the problem with no API at all. It is rejected here for two concrete
  reasons rather than on novelty: `CONVENTIONS.md` states that an operand
  that meets neither of its two listed exceptions "must not tolerate all
  taints", enforced by a CI test in openshift/origin, and the canary (a
  health prober) meets neither; and bypassing all custom taint isolation
  would widen placement beyond the standard DaemonSet tolerations,
  potentially adding unsuitable Ready backends to the probe path.
* Hand-edit the canary DaemonSet: unsupported; the Ingress Operator owns
  the DaemonSet and reverts manual changes, and such edits have no
  declarative life-cycle.
* Do nothing and document that custom taints are unsupported for the
  canary: leaves administrators of GPU, SR-IOV and specialized
  infrastructure pools with recurring false `Degraded` conditions
  (RFE-2010).

## Open Questions [optional]

1. **Does the reported customer topology actually exhibit this failure?**
   The corrected mechanism (see Motivation) requires the canary to have
   no eligible node at all. RFE-2010 could not be read while drafting
   this EP. If the reporting clusters retain any canary-eligible node,
   they are not hitting this failure and the motivation must be
   re-established before implementation proceeds. **This question gates
   the enhancement.** Required evidence remains outstanding: node taints,
   default-router tolerations, effective canary pod tolerations and
   Ready endpoint counts, including whether pre-existing canary pods
   were removed after `NoSchedule` tainting. No customer topology is
   established by this proposal.
2. **Does this feature graduate with `GatewayAPIManagementMode`, or
   separately?** The parent `ingresses.operator.openshift.io` kind ships
   only in the NoUpgrade feature sets because it is gated on that
   feature, so this field cannot reach Default before it does. The
   owners of
   [gateway-api-crd-management-mode](/enhancements/ingress/gateway-api-crd-management-mode.md)
   need to agree whether this field rides their promotion, and both
   target releases need recording.
3. **Is any change required in openshift/hypershift?** The ingress
   operator's cluster role already grants get/list/watch on
   `operator.openshift.io/ingresses`, and that role ships in the guest
   CVO payload, so no RBAC change is expected and the konnectivity path
   already exists. This needs confirmation from a HyperShift reviewer,
   together with whether Hosted Control Planes are in scope for Tech
   Preview or the feature is documented as standalone-only until an HCP
   lane exists.
4. **Is tainting every schedulable node acceptable in shared CI lanes?**
   Test case 5 reproduces the real failure and therefore needs the
   canary to have no eligible node. QE and TRT need to confirm this is
   acceptable in the shared lanes, or the case must be scoped to a
   dedicated tainted node pool. The wider case list and lane design also
   need QE review. A listed QE reviewer is not approval; QE/TRT lane
   approval remains outstanding. A dedicated pool is sufficient for
   case 5 only if no canary-eligible nodes remain outside it.
5. Should `spec.canary.nodePlacement` grow a `nodeSelector` (and
   possibly other placement fields) in a follow-up enhancement? The
   existing `NodePlacement` type in `operator/v1` pairs `nodeSelector`
   with `tolerations`, and both the DNS and network-diagnostics
   precedents shipped the two together, so an API reviewer is likely to
   ask why this one does not.
6. Should the merge semantics here diverge from the `DNSNodePlacement`
   precedent, which **replaces** rather than merges? Two identically
   shaped `nodePlacement` fields in `operator.openshift.io/v1` with
   opposite semantics is a usability hazard.

## Test Plan

Testing work is tracked by NE-1568 (pre-merge testing), NE-1569 (e2e
test automation) and NE-1570 (CI implementation).

The case list and lane design need QE review before this EP moves to
`implementable`; see Open Question 4.

### Unit tests (cluster-ingress-operator)

* Merge behaviour of `desiredCanaryDaemonSet`: nil and empty custom input
  produce only fixed baseline canary tolerations; configured entries are
  appended in order; the baseline `node-role.kubernetes.io/infra` toleration is
  always present.
* Deduplication: fully identical entries within and across baseline
  and canary-specific custom sources collapse to the first occurrence.
  Test equal finite durations, nil versus finite (including zero), and
  differing finite durations. Retain distinct durations and overlapping
  `Exists`/`Equal` matches in source order. Admission accepts duplicates;
  this normalization is controller behavior.
* The toleration comparator: ordered full-field comparison of canonical
  lists neither panics on duplicates nor produces a permanent diff for
  identical input. A seconds-only change and a reordering of overlapping
  matches are detected, not sorted away. Test repeated reconciles and
  restarts with identical input, including baseline/custom order.
* Placement independence: changing the default or any other
  IngressController's placement or tolerations does not change desired
  canary tolerations, both with custom canary input and with it omitted.
  Omitted custom input must never inherit router tolerations; assert
  baseline-only output even when router tolerations are configured.
  Exercise reconciliation triggered by the existing router watch too.
* Feature gate off: the field is ignored and the rendered DaemonSet is
  identical to the fixed baseline configuration without custom input,
  including when a schema that declares the field has persisted it.
  This tests controller behavior separately from API admission.
* Read failures after applying custom placement: discovery/cache errors,
  failed list/watch, timeouts, forbidden reads and konnectivity failures
  preserve the full existing template and placement, return a reconcile
  error and requeue without crash-looping or adding a read-failure-only
  degradation. Recovery reconciles the latest authoritative input.
  Test startup with an unsynchronized cache and an existing DaemonSet.
  Separately test authoritative singleton NotFound after synchronization,
  field omission and gate-off behavior: these use baseline-only
  configuration, not the read-error retention path. Test cold startup
  with the parent kind confirmed unserved, and subsequent API discovery
  and watch recovery without blocking baseline reconciliation.

### API validation tests (openshift/api)

Implemented in the API PR as two YAML suites under
`operator/v1/tests/ingresses.operator.openshift.io/`, running against
schemas that declare the field. `IngressCanaryNodePlacement.yaml` (gate
enabled) covers, on both create and update: the field omitted; a valid
single toleration; a valid list at the upper bound of 32; and each
rejected form — unknown, omitted and empty `operator`; unknown, omitted
and empty `effect`; `value` with `operator: Exists`; an empty `key`
with a non-`Exists` operator; the empty-key `Exists` wildcard;
`tolerationSeconds` without `NoExecute` and with no effect at all; an
empty list; a list above `MaxItems`; empty `canary` and `nodePlacement`
objects; and the unsupported `Lt` operator. Valid cases include
duplicates, overlapping matches, zero and negative `tolerationSeconds`,
and omitted versus explicit empty `value`.

Key syntax is tested at the boundaries: unprefixed and DNS-prefixed
keys; names of lengths 10 and 63; a 253-character single-label prefix;
combined keys of lengths 264 and 317; uppercase characters in the name
portion; and rejection of 64-character names, 254-character and
otherwise malformed prefixes, a 318-character combined key, non-ASCII
characters, empty prefix or name components, extra slashes, invalid
start/end characters, and characters such as `$`. For `Equal`, values
of lengths 0, 7 and 63 are accepted; length 64, invalid characters,
embedded slashes and invalid boundaries are rejected.

`IngressCanaryNodePlacementDisabled.yaml` tests a schema variant
without `spec.canary` (the parent kind still served through
`GatewayAPIManagementMode`) separately from runtime gate settings, on
create and update with `fieldValidation=Strict`, `Warn` and `Ignore`:
Strict rejects with `unknown field "spec.canary"`, Warn accepts with
that warning and prunes, and Ignore accepts and prunes without the
warning, with readback verifying the pruned object. To assert the
warnings, the shared test framework gained per-test `fieldValidation`
directives and `expectedWarnings` assertions: warnings are captured
through a per-test client built from a copied rest config, so reads and
neighboring requests cannot leak into the assertion, and Warning
headers are accumulated across client-go retries.

Still outstanding: a suite exercising a variant that declares the field
with the runtime gate disabled (`CustomNoUpgrade`), proving the field
persists while the gate-off controller ignores it (the controller side
of that case is covered by unit tests); and coverage of an unavailable
parent API, without confusing it with an unknown field.

### End-to-end tests (openshift/origin)

Every case carries `[OCPFeatureGate:IngressCanaryNodePlacement]` and
`[Jira:"Routing"]`. Cases that mutate the cluster-wide Ingress
configuration or node taints cannot run concurrently with other tests
and are therefore `[Serial]`, and those that taint nodes are also
`[Disruptive]` because they block scheduling for unrelated workloads.

1. *canary daemonset should schedule onto tainted nodes when tolerations
   are configured* — `[Serial][Disruptive]`. Taint a subset of nodes,
   record and remove only the existing canary pods on those nodes after
   tainting, and verify replacements cannot run there. Configure a
   matching toleration and assert `desiredNumberScheduled` grows to
   include them and replacement pods become Ready.
2. *canary daemonset should keep its default scheduling when the field is
   unset* — parallel-safe, read-only.
3. *canary daemonset tolerations should be restored after manual edits* —
   `[Serial]`. Edit the DaemonSet directly, assert the operator reverts
   it.
4. *canary daemonset should return to default scheduling when the field
   is cleared* — `[Serial][Disruptive]`. Remove the entire `spec.canary`
   object while retaining another spec field such as `gatewayAPI`, since
   `tolerations: []` and empty parent objects are rejected by admission.
   Assert baseline-only tolerations even while router tolerations remain
   configured. Covers the field remediation described in Support
   Procedures.
5. *ingress controller should recover from a canary with no schedulable
   node* — `[Serial][Disruptive][Slow]`. The full scenario from the
   Motivation; see the feasibility note below.
6. *invalid canary tolerations should be rejected* — parallel-safe,
   admission only.

**Feasibility of case 5.** Reproducing the real failure needs the canary
to have *no* eligible node, which means tainting every schedulable node.
This requires explicit QE/TRT approval of the lane and recovery plan;
survival of unrelated workloads and rescheduling is not assumed.
Keep the canary-specific field unset and choose a custom taint not
tolerated by the fixed canary baseline. Configure matching router
tolerations independently so routers remain healthy on the tainted
nodes; these tolerations are not propagated to the canary. QE/TRT must
approve the taint, canary-pod deletion and recovery procedure for the
lane. This is a proposed test, not evidence that the fixture has run.

Use a unique custom `NoSchedule` taint, never a cluster-wide `NoExecute`
taint. Record the original taints and the UIDs of existing canary pods,
apply the taint to every canary-eligible node, then explicitly delete
only those recorded canary pods in `openshift-ingress-canary`.
`NoSchedule` does not evict them by itself. Wait for zero canary pods,
`desiredNumberScheduled: 0` and zero canary Service endpoints (including
no Ready backends in EndpointSlices) before waiting for probe failures
and asserting `CanaryChecksSucceeding=False` and `Degraded=True` while
routers remain healthy. Do not delete router or platform pods.
Configure the matching canary-specific toleration, then wait for a
positive desired count, Ready canary pods and Ready Service backends,
a successful route probe, `CanaryChecksSucceeding=True` and
`Degraded=False`. Restore the original configuration and taints in
deferred cleanup on success or failure and verify recovery there too.
Because `Degraded` only appears after five successive one-minute probe
failures plus a grace period, this case is inherently slow; the
condition-transition logic itself is covered more cheaply by unit tests.

Whether this is acceptable in the shared lanes, or whether the case must
be scoped to a dedicated tainted node pool, is Open Question 4.

**Disconnected and proxied clusters.** These cases assert on API objects
and conditions rather than fetching the canary route directly, so they
carry no proxy requirement. If any case is later changed to probe the
route with a raw HTTP client, it must honour `HTTP_PROXY`, `HTTPS_PROXY`
and `NO_PROXY`, because the Baremetal IPv6 lanes run against
disconnected clusters.

### CI lanes

* Presubmits on openshift/cluster-ingress-operator and openshift/api for
  the unit and API validation suites.
* Periodic jobs running the e2e cases on each platform required for
  promotion, at least daily so that the seven-runs-per-week threshold is
  met, in both `TechPreviewNoUpgrade` and `Default` variants so coverage
  continues after promotion.
* Because the cases are serial, disruptive and partly slow, they need a
  dedicated suite rather than the default conformance suites; new jobs
  are configured in openshift/release and tracked by NE-1570.

### Upgrade, downgrade and skew

* Upgrade with the field set: the configuration survives and the canary
  DaemonSet still carries the merged tolerations afterwards.
* Downgrade with the field set: covered in Upgrade / Downgrade Strategy.
  The test asserts that `spec.canary` is absent from API responses after
  the older CRD is applied, that the canary reverts to baseline-only
  tolerations regardless of router configuration, and that no "clear the
  field" remediation is reachable.
  It should also exercise downgrade followed by re-upgrade and record
  whether the stored value reappears, since that depends on whether the
  object was written while the field was pruned.
* Operator restart with the field set: the merged tolerations are
  reapplied and the DaemonSet is not rolled again needlessly, which also
  guards against the unstable-ordering defect.

## Graduation Criteria

The feature starts in `TechPreviewNoUpgrade` only, so the first
applicable milestone is Tech Preview -> GA. No target releases are named
yet, because promotion depends on the parent kind's graduation; see Open
Question 2.

### Dev Preview -> Tech Preview

The feature is introduced directly at Tech Preview through the
`TechPreviewNoUpgrade` feature set, so this milestone is not applicable;
the entry criteria (end-to-end usability, relative API stability,
sufficient test coverage) are expected to be met at introduction.

### Tech Preview -> GA

Promotion to the Default feature set requires, per the promotion
requirements in the OpenShift dev guide:

* **The parent kind must be available in the Default feature set
  first.** `ingresses.operator.openshift.io` ships only in
  `CustomNoUpgrade`, `DevPreviewNoUpgrade` and `TechPreviewNoUpgrade`
  because the kind is gated on `GatewayAPIManagementMode`, so promoting
  `IngressCanaryNodePlacement` alone would place the field on a CRD that
  is not installed. Either `GatewayAPIManagementMode` graduates first,
  or this field rides its promotion, or the kind's gating is lifted
  independently. See
  [Dependency on the parent kind](#dependency-on-the-parent-kind).
* At least 5 tests for the feature, run at least 7 times per week and at
  least 14 times per supported platform, passing at least 95% of the
  time, in place no less than 14 days before branch cut.
* Tests running on all supported platforms, including the AWS Single
  (single-node) variant, and noting that the Baremetal IPv6 jobs run
  against disconnected clusters, in both `TechPreviewNoUpgrade` and
  `Default` Prow job variants.
* No regressions to the existing ingress ClusterOperator conditions and
  alerts that carry the canary health signal.
* User-facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/).
* End-to-end tests (required for non-optional features moving to GA).

### Removing a deprecated feature

* Announce the deprecation and the support policy of the existing
  behavior.
* Deprecate the feature and, once it has been enabled by default for at
  least one minor release, remove the feature gate.

## Upgrade / Downgrade Strategy

* The new field is optional and additive. Without canary-specific custom
  configuration, desired canary tolerations contain only the fixed
  baseline. Router configuration does not affect this behavior. Custom
  canary tolerations are applied only when the feature gate is enabled.
* While the feature is Tech Preview it is reachable only through the
  `TechPreviewNoUpgrade` feature set, which is a one-way door: it cannot
  be disabled and the cluster cannot be upgraded while it is enabled.
  Tech Preview clusters are therefore expected to be disposable.
* Changing the field rolls the canary DaemonSet (one pod per node). The
  operator's canary checks tolerate brief, rolling unavailability
  through their sustained-failure logic, so ingress reporting is not
  disrupted by routine updates of the field.
* After promotion to Default, upgrades carry the field like any other
  Ingress configuration field.
* On downgrade the governing mechanism is the CRD schema, not the
  operator. The CVO applies the older payload's `ingresses` CRD, whose
  schema has no `spec.canary`. In apiextensions-apiserver the structural
  pruning decoder wraps the *storage* codec, so pruning is applied on
  every decode out of etcd rather than only on write. The practical
  consequences are:
  * The stored value is not deleted from etcd by the downgrade itself,
    but it is stripped from every API response. `oc get` and `oc edit`
    do not show `spec.canary`, so an administrator cannot inspect,
    correct or remove it. There is no "remove the configuration"
    remediation on a downgraded cluster.
  * The canary reverts to baseline-only tolerations, independently of
    router configuration, because the field is absent from the object
    the operator reads. On clusters where
    every canary-eligible node is tainted, this reintroduces the
    pre-enhancement false `Degraded` condition. The available
    remediations are to untaint at least one canary-eligible node or to
    restore the newer release.
  * The surviving etcd bytes are fragile and must not be relied on. They
    persist only until the next write of the object; because `Ingress`
    has a status subresource and a status write round-trips the pruned
    spec, a single such write persists the pruned form and the value is
    then permanently gone. A downgrade followed by a re-upgrade may
    therefore either silently restore the previous tolerations or come
    back empty, depending on whether anything wrote the object in
    between. Administrators should record their configuration
    out-of-band rather than depend on it surviving the round trip.

## Version Skew Strategy

* On standalone clusters the Ingress Operator and its CRD come from the
  same payload, but the CVO does not apply them atomically: CRD
  manifests are applied at an earlier run level than the operator's
  Deployment, and the operator pod then has to roll. A window therefore
  exists in which the new schema is served while the previous operator
  binary is still running, and the reverse during a rollback. The
  consequence here is benign but silent: the field can be set and
  nothing acts on it until the new operator is running.
* On Hosted Control Planes the skew is structural rather than
  transient, because the CRD and RBAC come from the guest CVO while the
  operator comes from the management-side control-plane operator. See
  [Hypershift / Hosted Control Planes](#hypershift--hosted-control-planes).
* In both cases use the read-error contract in Implementation Details:
  authoritative singleton NotFound means unset; a confirmed unserved
  kind means unavailable. Discovery/cache/read errors and startup cache
  synchronization preserve existing placement and retry. An unreadable
  singleton is not evidence of absence.
* No node-side components (kubelet, CRI, CNI) are involved: canary pods
  are scheduled by the existing kube-scheduler using standard toleration
  semantics available in all supported versions.
* The operator snapshots feature gate state once at startup through the
  library-go feature gates accessor, whose default behaviour is to exit
  the process when the gates change. Changing the cluster's feature set
  therefore restarts the ingress operator pod rather than being picked up
  in place; support should expect that restart.

## Operational Aspects of API Extensions

* SLIs: the health of the affected functionality is already observable
  through the existing `ingress` ClusterOperator conditions
  (`Available`, `Degraded`) and the default IngressController status,
  which reflect canary availability. No new SLIs are introduced.
* Impact on existing SLIs: negligible. The field lives on the singleton
  `Ingress` configuration object, is validated by the kube-apiserver
  through the CRD schema (no webhooks in the request path), and adds a
  small amount of schema to the feature-gated CRD variants.
* Failure modes:
  * Tolerations that match no taint: the canary stays where it is. No
    new symptom; the pre-existing `Degraded` persists.
  * Custom tolerations that widen placement onto unsuitable nodes:
    standard condition-taint tolerations already apply to DaemonSet
    pods, and pod presence alone does not imply a Ready Service backend.
    An unsuitable backend that becomes Ready can still fail probes.
    Compare `oc -n openshift-ingress-canary get pods -o wide`, node
    health and Ready EndpointSlice backends. Validation rejects the
    tolerate-everything form, not every unsafe placement choice.
  * A roll that does not converge, leaving zero canary pods: detected by
    `desiredNumberScheduled` versus `numberReady` on the DaemonSet;
    degrades the IngressController after roughly five minutes.
  * A singleton read fails: preserve existing placement and retry rather
    than reset custom tolerations; inspect reconcile errors separately
    from the unchanged canary health signal.
  * The field is submitted while the feature gate is disabled: admission
    depends on the installed schema and field-validation mode; a
    persisted field is ignored by the gate-off controller. See Support
    Procedures for how to distinguish these cases.
* Escalation path: the `Routing` Jira component owns the operator and the
  canary; API schema questions route to the openshift/api reviewers, who
  should review this enhancement.

## Support Procedures

* Symptoms. The default IngressController reports `Degraded` with
  `CanaryChecksSucceeding=False`, while the routers are healthy and
  serving traffic:

  ```console
  $ oc get ingresscontroller -n openshift-ingress-operator default \
      -o jsonpath='{.status.conditions[?(@.type=="CanaryChecksSucceeding")]}'
  $ oc get co ingress -o jsonpath='{.status.conditions}'
  ```

  The decisive symptom is that the canary DaemonSet has **no pods at
  all**, not that it has fewer pods than nodes:

  ```console
  $ oc -n openshift-ingress-canary get ds ingress-canary
  NAME             DESIRED   CURRENT   READY   ...
  ingress-canary   0         0         0       ...
  ```

  Note that `DESIRED` is `0`, and that there are **no** `FailedScheduling`
  events: the DaemonSet controller excludes nodes with untolerated taints
  from `desiredNumberScheduled` rather than creating Pending pods, so a
  partially-tainted cluster can show `DESIRED == READY` at a reduced count
  and is *not* this failure. With `NoSchedule`, pre-existing canary pods
  can remain even when no replacement node is eligible; confirm actual
  pod and endpoint counts rather than inferring their removal from the
  taint or desired count alone.
* Discriminator. Canary DaemonSet `DESIRED: 0`, no canary pods and no
  route backends, together with untolerated custom taints, identify this
  placement failure. `Running` pods alone do not prove backend readiness.
  With Ready backends and a resolving route, investigate ingress and
  canary serving failures rather than assuming this field will help.

  ```console
  $ oc -n openshift-ingress-canary get endpoints ingress-canary
  $ oc get nodes -o custom-columns=NAME:.metadata.name,TAINTS:.spec.taints
  ```
* Diagnosis:
  * `oc get ingresses.operator.openshift.io cluster -o yaml` to inspect
    `spec.canary.nodePlacement.tolerations`. On a cluster where the
    parent CRD is not installed this returns `the server doesn't have a
    resource type "ingresses.operator.openshift.io"`, which means the
    feature is unavailable, not that it is unset;
  * `oc get featuregate cluster -o yaml` to confirm the enabled feature
    set. Note this shows the selected feature set rather than per-gate
    state. Inspect the installed CRD schema separately from the runtime
    gate state reported to the operator. `CustomNoUpgrade` may declare
    the field while the gate is off. If the schema omits it, Strict
    requests reject it, Warn requests warn and prune it, and Ignore
    requests prune it without that warning. Read back accepted writes;
  * Ingress Operator logs in `openshift-ingress-operator` for
    reconciliation errors.
* Remediation: correct `spec.canary.nodePlacement.tolerations` or
  remove the entire `spec.canary` object; `tolerations: []` and empty
  `canary`/`nodePlacement` objects are rejected by admission, and
  `spec` must retain another field such as `gatewayAPI`. Removing the
  configuration restores baseline-only tolerations, regardless of
  router configuration. A change to the effective
  ordered list rolls the canary DaemonSet, recreating its pods; removing
  an exact duplicate alone need not roll it. A read error does not clear
  placement: restore API connectivity or permissions so reconciliation
  can observe the intended configuration.
* Disabling: while the feature is Tech Preview, the supported way to
  remove the configuration is to delete the entire `spec.canary`
  object; the
  `TechPreviewNoUpgrade` feature set itself cannot be disabled once
  enabled, and the CRD is part of the payload and must not be removed.
  Because the feature only widens canary scheduling, leaving it
  configured has no impact beyond the canary's own placement.

## Infrastructure Needed [optional]

No new repositories, container images or external services are required.
The feature does need CI capacity that does not exist today:

* A dedicated test suite for the e2e cases. They taint and untaint
  nodes and assert on cluster-wide ingress conditions, so they are
  serial and disruptive and cannot run in the default conformance
  suites.
* New periodic Prow jobs in openshift/release running that suite on each
  platform required for promotion, at least daily so the
  seven-runs-per-week graduation threshold is met, in both
  `TechPreviewNoUpgrade` and `Default` variants so coverage continues
  after promotion. Tracked by NE-1570.
