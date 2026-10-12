---
title: must-gather-operator-policy
authors:
  - "@swghosh"
reviewers:
  - "@shivprakahsmuley"
  - "@Prashanth684"
approvers:
  - "@Prashanth684"
api-approvers:
  - "@shivprakahsmuley"
  - "@Prashanth684"
creation-date: 2025-08-18
last-updated: 2025-10-09
tracking-link:
  - https://redhat.atlassian.net/browse/RFE-9011
  - https://redhat.atlassian.net/browse/OCPSTRAT-3733
  - https://redhat.atlassian.net/browse/MG-348
status: implementable
see-also:

---

# Must Gather Operator Managed Policy for Storage et al.

## Summary

Introduce a new cluster-scoped `MustGatherPolicy` CR that lets cluster administrators define global, opt-in defaults and guardrails for must-gather runs like storage configuration, resource requests/limits, etc. for the gather pods. The existing namespace-scoped `MustGather` CR inherits the resolved policy at reconcile time.

The feature is delivered in two phases.
- In **Phase 1**: the API is
`v1alpha1` and `MustGatherPolicy` is a **singleton** (`metadata.name: cluster`)
carrying cluster-global configuration; individual `MustGather` CRs inherit it
implicitly. 

- In **Phase 2** the API graduates to `v1`, **multiple**
`MustGatherPolicy` objects may coexist, a `MustGather` selects which policy to
inherit via `spec`, and a policy can declare which of the inherited fields a user
is permitted to override on the `MustGather` spec.

All changes to the existing `MustGather` type are **strictly additive** (a new
`status` field in Phase 1 and a new optional `spec` field in Phase 2), so there
are no breaking API changes for existing consumers.

## Motivation

Today every `MustGather` author must independently know and repeat the right storage class, capacity, resource sizing, and upload destination. There is no cluster-wide mechanism for an administrator to (a) set sane defaults so ad-hoc runs don't have to, (b) constrain where collected data may be uploaded, or (c) bound the resources and network egress of the (potentially large) gather workload.

`MustGatherPolicy` provides that central control configuration while keeping individual runs simple.

### User Stories

- As a cluster administrator, I want to set cluster-wide defaults (storage class,  size, retention, resource requests/limits) once so that users creating a `MustGather` do not have to specify them and do not have to pre-provision PVCs manually.
- As a cluster administrator, I want to restrict the set of allowed upload targets so that collected cluster data can only leave the cluster through trusted destinations.
- As a cluster administrator, I want to attach a `NetworkPolicy` ..
- As a user creating a `MustGather`, I want to inherit the administrator's policy automatically.
- As a user, I want to see exactly which policy (and which revision of it) was applied to my run, recorded on the `MustGather` status, for auditability.
- As a cluster administrator, I want to decide per policy which fields a user may override on their `MustGather` enforcing hard guardrails.

### Goals

- Provide a cluster-scoped `MustGatherPolicy` CR for global must-gather
  configuration: storage, resource requests/limits, upload-target allow list, and
  gather-pod `NetworkPolicy`.
- Make policy inheritance opt-in and observable, not changing any existing
  clusters until an administrator creates the policy.
- Evolve the API from `v1alpha1` (singleton) to `v1` (multiple policies with
  user-selectable inheritance and per-field override control) with no breaking
  changes to the `MustGather` API and forward-compatible guarantees.

### Non-Goals

- Inheriting `serviceAccountName`, `imageStreamRef`, or `obfuscate`
  (`ObfuscateConfig`) from the policy.
- Replacing or deprecating any existing `MustGather` spec field. Policy provides
  defaults/guardrails.
- Providing a general-purpose admission/quota system for must-gather runs beyond
  the guardrails described above.

## Proposal

An administrator creates a `MustGatherPolicy`. On each `MustGather` reconcile the
operator resolves the applicable policy, merges its defaults with the spec,
provisions or reuses a PVC, applies the resource limits and gather-pod
`NetworkPolicy`, checks the upload target against the allow list, and records the
policy reference and observed revision in `MustGather.status`. A separate
controller reconciles the policy itself.

Inheritance is opt-in and OFF by default: with no `MustGatherPolicy` present,
`MustGather` behaves as it does today and `status.inheritsPolicy` is unset.

### API Extensions

One new kind, `MustGatherPolicy`, plus additive changes to `MustGather`. Phase 1
ships `v1alpha1`; Phase 2 promotes to `v1`.

#### Phase 1 (Tech Preview): `v1alpha1` singleton `MustGatherPolicy`

`MustGatherPolicy` is cluster-scoped and, in Phase 1, a singleton named `cluster`,
following the OpenShift config-singleton convention. A singleton sidesteps "which
policy applies" until Phase 2 adds selection.

Example:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGatherPolicy      # cluster-scoped, singleton "cluster"
metadata:
  name: cluster
spec:
  storage:
    type: PersistentVolume
    persistentVolume:
      storageClassName: thin-csi
      size: 50Gi
      # lifecycle management of the provisioned PVCs
      retentionPeriod: 7d
      fullThresholdPercent: 85
      expandIfAllowed: true
  resources:
    requests:
      cpu: 500m
      memory: 512Mi
    limits:
      cpu: "2"
      memory: 4Gi
  uploadTargets:
    # allow list; an empty/omitted allow list means "no uploads permitted"
    allowedTypes:
      - SFTP
    allowedHosts:
      - sftp.access.redhat.com
  networkPolicy:
    # applied to the gather pods; operator owns the podSelector
    spec:
      policyTypes:
        - Egress
      egress:
        - to: []
          ports:
            - protocol: TCP
              port: 22
```

```go
// api/v1alpha1/mustgatherpolicy_types.go

// MustGatherPolicySpec defines cluster-wide defaults and guardrails for
// must-gather runs.
type MustGatherPolicySpec struct {
	// storage defines the cluster default storage configuration and lifecycle
	// policy for the PVCs used to persist must-gather archives.
	// +optional
	Storage *PolicyStorage `json:"storage,omitempty"`

	// resources defines the default resource requests and limits applied to the
	// must-gather gather workload (Job pods).
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// uploadTargets is an allow list constraining where collected data may be
	// uploaded. When omitted or empty, no upload target is permitted.
	// +optional
	UploadTargets *UploadTargetPolicy `json:"uploadTargets,omitempty"`

	// networkPolicy, when set, is materialized as a NetworkPolicy applied to the
	// gather pods to constrain their traffic (typically egress to approved upload
	// endpoints). The operator owns and manages the podSelector.
	// +optional
	NetworkPolicy *NetworkPolicyProfile `json:"networkPolicy,omitempty"`
}

// PolicyStorage extends the run-time Storage config with lifecycle controls that
// only make sense for operator-managed (provisioned) PVCs.
type PolicyStorage struct {
	// type defines the type of storage to use. PersistentVolume only.
	// +required
	Type StorageType `json:"type,omitempty"`

	// persistentVolume defines the default PVC template and lifecycle policy.
	// +required
	PersistentVolume ManagedPersistentVolumeConfig `json:"persistentVolume,omitzero"`
}

// ManagedPersistentVolumeConfig describes how the operator provisions and manages
// PVCs on behalf of MustGather runs.
type ManagedPersistentVolumeConfig struct {
	// storageClassName is the StorageClass used to provision PVCs.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// size is the requested capacity for provisioned PVCs.
	// +required
	Size resource.Quantity `json:"size,omitzero"`

	// retentionPeriod is how long a provisioned PVC (and its data) is retained
	// after the MustGather run completes before the operator reclaims it.
	// +optional
	// +kubebuilder:validation:Format=duration
	RetentionPeriod *metav1.Duration `json:"retentionPeriod,omitempty"`

	// fullThresholdPercent is the utilization percentage above which the operator
	// treats the volume as full and takes action (e.g. stop/alert/expand).
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	FullThresholdPercent *int32 `json:"fullThresholdPercent,omitempty"`

	// expandIfAllowed enables online expansion of the PVC when the StorageClass
	// supports volume expansion and the fullThresholdPercent is reached.
	// +optional
	ExpandIfAllowed *bool `json:"expandIfAllowed,omitempty"`
}

// UploadTargetPolicy constrains the upload destinations a MustGather may use.
type UploadTargetPolicy struct {
	// allowedTypes lists the upload target types permitted by this policy.
	// +optional
	// +listType=set
	AllowedTypes []UploadType `json:"allowedTypes,omitempty"`

	// allowedHosts is an allow list of destination hostnames (e.g. SFTP hosts)
	// permitted as upload targets.
	// +optional
	// +listType=set
	AllowedHosts []string `json:"allowedHosts,omitempty"`
}

// NetworkPolicyProfile carries a NetworkPolicy spec to apply to gather pods.
type NetworkPolicyProfile struct {
	// spec is the NetworkPolicySpec applied to the gather pods. The operator
	// injects/overwrites the podSelector so it targets only the gather pods it
	// creates; any user-supplied podSelector is ignored.
	// +required
	Spec networkingv1.NetworkPolicySpec `json:"spec,omitzero"`
}
```

Phase 1 adds a status field to `MustGather`; the spec is untouched:

```go
// api/v1/mustgather_types.go (and v1alpha1)

// MustGatherStatus defines the observed state of MustGather
type MustGatherStatus struct {
	// ... existing fields unchanged ...

	// observedGeneration is the metadata.generation of this MustGather that the
	// controller has most recently reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// inheritsPolicy records the MustGatherPolicy that was resolved and applied
	// to this MustGather, and the policy revision that was observed. It is unset
	// when no policy applies (i.e. before an administrator creates a policy).
	// +optional
	InheritsPolicy *InheritedPolicyStatus `json:"inheritsPolicy,omitempty"`
}

// InheritedPolicyStatus identifies the applied policy and the revision that was
// reconciled into this MustGather.
type InheritedPolicyStatus struct {
	// name references the applied cluster-scoped MustGatherPolicy.
	corev1.LocalObjectReference `json:",inline"`

	// observedGeneration is the metadata.generation of the MustGatherPolicy that
	// was last reconciled into this MustGather.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}
```

Two `observedGeneration` values, each tracking its own object:

- `status.observedGeneration` — the `MustGather` generation last reconciled.
- `status.inheritsPolicy.observedGeneration` — the `MustGatherPolicy` generation
  last applied to this run, for drift detection against the live policy.

Adding only optional status fields is backward compatible, and the existing
spec-immutability rule (`self.spec == oldSelf.spec`) is untouched since no spec
field changes.

#### Phase 2 (GA): `v1` multiple policies + user-selectable inheritance

The API graduates to `v1`. The singleton restriction is dropped: multiple policies
may coexist, a `MustGather` selects one to inherit, and a policy can declare which
inherited fields a user may override.

Phase 2 adds an optional spec field to `MustGather` (set at creation, per the
immutable-spec model) and reuses the Phase 1 `status.inheritsPolicy`:

```go
// api/v1/mustgather_types.go

type MustGatherSpec struct {
	// ... existing fields unchanged ...

	// inheritsPolicy names the cluster-scoped MustGatherPolicy this MustGather
	// inherits configuration from. When omitted, no policy is inherited.
	// +optional
	InheritsPolicy *corev1.LocalObjectReference `json:"inheritsPolicy,omitempty"`
}
```

Keeping `status.inheritsPolicy` identical across phases is what makes the
transition forward compatible: a Phase 1 client reading that field keeps working,
and the Phase 2 reconciler resolves the policy from `spec.inheritsPolicy` instead
of the singleton, recording the same reference and `observedGeneration`.

Per-field override control is added to the policy spec:

```go
// api/v1/mustgatherpolicy_types.go (v1)

type MustGatherPolicySpec struct {
	// ... storage / resources / uploadTargets / networkPolicy as in v1alpha1 ...

	// allowUserOverrides lists the MustGather spec field paths that a user is
	// permitted to override when inheriting this policy. Fields not listed are
	// enforced by the policy and any user-provided value for them is rejected at
	// admission (or ignored in favor of the policy value). This does not apply to
	// serviceAccountName, imageStreamRef, or obfuscate, which are never inherited.
	// +optional
	// +listType=set
	AllowUserOverrides []MustGatherOverridableField `json:"allowUserOverrides,omitempty"`
}

// MustGatherOverridableField enumerates the inheritable MustGather spec fields
// whose override can be gated by policy (e.g. "storage", "resources",
// "uploadTarget").
// +kubebuilder:validation:Enum=storage;resources;uploadTarget
type MustGatherOverridableField string
```

`v1alpha1`→`v1` uses identity conversion, as `MustGather` already does: `v1` is a
compatible superset, so no conversion webhook is needed. `v1` becomes the storage
version; `v1alpha1` stays served.

#### Child objects: ownership, discovery, and status

Applying a policy produces two kinds of child objects:

- a **NetworkPolicy** in **every namespace**, and
- a **PVC** provisioned **lazily, per `MustGather` run** (the policy holds only the
  storage *profile*).

Fan-out to every namespace is unbounded, so the policy status must not enumerate
its children: a per-namespace `(name, namespace)` list on a cluster-scoped
singleton grows with the namespace count, can exceed the etcd object-size limit,
and makes every status write a conflict hotspot. Other OpenShift fan-out designs
([trust-manager](../cert-manager/trust-manager-controller.md),
[cert-manager network policies](../cert-manager/cert-manager-network-policies.md),
[external-secrets network policy](../external-secrets-operator/external-secrets-network-policy.md))
track children via ownerReferences and labels instead. Three mechanisms cover it:

**1. OwnerReferences (lifecycle / GC).**
Each namespaced `NetworkPolicy` is owned by the cluster-scoped `MustGatherPolicy`.
A namespaced dependent with a cluster-scoped owner is garbage-collected, so
deleting the policy cascades to every NetworkPolicy — the mechanism OLM uses to
clean up across namespaces (see [olm/simplify-apis](../olm/simplify-apis.md)).

> [etcd/automated-backups](../etcd/automated-backups.md) cautions that
> cluster-scoped→namespaced GC "will not be enforced." That reads the rule
> backwards: a namespaced dependent with a cluster-scoped owner *is* collected;
> only the reverse is not. Re-verify against the target Kubernetes version. A
> finalizer on the policy backstops cleanup regardless, removing every fanned-out
> NetworkPolicy before the policy object is released.

The per-run **PVC** is owned by its `MustGather` and cleaned up with the run,
subject to the retention policy.

**2. Labels (discovery).**
Every child carries labels so enumeration is a selector query, not a status read:

- `app.kubernetes.io/managed-by: must-gather-operator`
- `must-gather.openshift.io/policy-name: <name>`
- `must-gather.openshift.io/policy-uid: <uid>` (survives name reuse)
- `must-gather.openshift.io/policy-generation: <generation>` — the policy
  generation the child was rendered from

`oc get networkpolicy -A -l must-gather.openshift.io/policy-name=cluster` lists
them. The `policy-generation` label acts as a per-child observed generation:
children below `policy.metadata.generation` are stale, and a selector finds them.

**3. Conditions + observedGeneration (rollout summary).**
The policy status stays O(1) regardless of namespace count:

```go
// api/v1alpha1/mustgatherpolicy_types.go

// MustGatherPolicyStatus defines the observed state of MustGatherPolicy.
type MustGatherPolicyStatus struct {
	// observedGeneration is the most recent generation observed for this policy.
	// Rollout for a given spec is complete when observedGeneration equals
	// metadata.generation and the Progressing condition is False.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// conditions represent the latest available observations of the policy rollout.
	//   Available:   the policy has been applied cluster-wide.
	//   Progressing: rollout to namespaces is in progress.
	//   Degraded:    one or more namespaces failed; the message carries the count,
	//                e.g. "2 of 42 namespaces failed".
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

Failing namespaces are named in the `Degraded` message, not in a structured list.
A later revision could add a `MaxItems`-capped list of only the failing namespaces
(as [subscription-injection](../subscription-content/subscription-injection.md)
does); Phase 1 stays conditions-only.

Per-run children are tracked the same way: the PVC and NetworkPolicy a `MustGather`
uses are owned by it and carry the discovery labels, so their names need not be
copied into `MustGather.status`. That keeps `MustGather.status` to the two
`observedGeneration` values and the policy reference.

### Implementation Details/Notes/Constraints

- **Opt-in / OFF by default.** The `MustGather` controller consults a policy only
  when one exists — the singleton in Phase 1, or the policy named by
  `spec.inheritsPolicy` in Phase 2. Otherwise behavior is unchanged and
  `status.inheritsPolicy` stays unset.

- **Resolution & merge order.** Per inheritable field: the user's spec value if set
  and permitted, else the policy value, else the operator default. In Phase 2, a
  user value for a field absent from `allowUserOverrides` rejects the run.

- **Inheritance scope.** Only `storage`, `resources`, `uploadTarget`, and
  `networkPolicy` are inherited. `serviceAccountName`, `imageStreamRef`, and
  `obfuscate` are excluded for now (see Non-Goals).

- **Two controllers.** A new controller owns `MustGatherPolicy`: validation,
  rollout status, and fan-out/cleanup of the per-namespace NetworkPolicies. The
  `MustGather` controller owns the per-run PVC — provisioning it from the profile
  and applying the policy's retention, full-threshold, and expansion settings.
  Retention/reclaim must not race with a run still mounting the volume.

- **Singleton enforcement (Phase 1).** A CEL `XValidation` rule pins
  `metadata.name == "cluster"`.

- **NetworkPolicy ownership.** The operator sets the `podSelector` itself so each
  `NetworkPolicy` targets only its gather pods; a user-supplied `podSelector` is
  ignored, keeping unrelated workloads unaffected.

- **NetworkPolicy fan-out.** The policy controller writes the `NetworkPolicy` to
  every namespace and watches `Namespace` creation so new namespaces converge.
  Each object is owned by the policy and carries the discovery labels.

- **Lazy PVC provisioning.** The PVC is created only when a run needs one, in the
  run's namespace, owned by its `MustGather`.

- **Cleanup.** Deleting the policy cascades to its NetworkPolicies via
  ownerReference GC, backstopped by the controller finalizer.

### Topology Considerations

The feature needs the `NetworkPolicy` API and a CSI/StorageClass. Where either is
absent, inheritance degrades gracefully: fan-out is a no-op and storage falls back
to ephemeral. On **SNO** the resource-limit guardrails matter most, bounding a
gather run on a constrained node. **MicroShift**, **Hypershift/HCP**, and **OKE**
specifics are TBD.

## Implementation History

- [Must-Gather Operator: PVC destination for gathered data](./must-gather-operator-pvc-destination.md)
- [Must Gather Operator](./must-gather-operator.md)

## Alternatives (Not Implemented)

## Infrastructure Needed

## Graduation Criteria

### Dev Preview -> Tech Preview (Phase 1)

- `v1alpha1` `MustGatherPolicy` singleton available behind the Tech Preview
  feature gate.
- `MustGather.status.inheritsPolicy` populated when the singleton exists; unset
  otherwise (opt-in / OFF by default verified).
- Storage, resources, upload-target allow list, and gather-pod `NetworkPolicy`
  inheritance implemented and covered by e2e.

### Tech Preview -> GA (Phase 2)

- API graduated to `v1` with identity conversion from `v1alpha1` (no conversion
  webhook), `v1` as storage version, `v1alpha1` still served.
- Multiple `MustGatherPolicy` objects supported; `MustGather.spec.inheritsPolicy`
  selects the policy; `status.inheritsPolicy` semantics unchanged from Phase 1
  (forward compatibility demonstrated).
- Per-field override control (`allowUserOverrides`) implemented and enforced.
- Consistent pass rate of 100% for all e2e test cases over 1 release.

## Upgrade / Downgrade Strategy

All `MustGather` changes are additive, so upgrading the operator never breaks
existing objects. The `v1alpha1`→`v1` graduation uses identity conversion, so no
webhook is needed and stored objects convert transparently.

## Operational Aspects of API Extensions

- The policy controller fans out NetworkPolicies and reports
  `Available`/`Progressing`/`Degraded` plus `observedGeneration`. The `MustGather`
  controller owns per-run PVCs and emits metrics for
  provisioned/retained/reclaimed volumes.
- Failure modes to define: invalid or unavailable StorageClass; upload target not
  in the allow list (fail the run clearly); `NetworkPolicy` unsupported on the
  platform; PVC full with expansion impossible; a subset of namespaces failing
  fan-out (reported via `Degraded`).

## Test Plan

### Risks and Mitigations

- **Fan-out blast radius.** Writing a `NetworkPolicy` to every namespace touches
  `kube-system`, `openshift-*`, and the rest. *Mitigation:* the operator owns the
  `podSelector`, so a policy only ever selects its own gather pods, and an opt-out
  (namespace annotation or exclusion list) lets admins skip sensitive namespaces.
- **Stale children after an update.** Rollout is asynchronous, so a namespace may
  briefly hold an older-generation NetworkPolicy. *Mitigation:* the
  `policy-generation` label surfaces stale objects, and `status.observedGeneration`
  with the `Progressing` condition reports incomplete rollout.
- **GC direction assumption.** Cleanup assumes cluster-scoped-owner →
  namespaced-dependent GC. *Mitigation:* the finalizer backstops it; re-verify
  against the target Kubernetes version.

### Drawbacks

- Fan-out adds reconcile load proportional to namespace count and a `Namespace`
  watch, for a feature many namespaces never use.
- A second controller and a finalizer widen the operational surface.

### Removing a deprecated feature

N/A — nothing is removed; all changes are additive.

## Version Skew Strategy

Changes are additive and both API versions are served during graduation, so mixed
control-plane/operator versions interoperate: an older operator ignores the new
`spec`/`status` fields, and the fanned-out NetworkPolicies remain valid on their
own.

## Support Procedures

- List fanned-out objects:
  `oc get networkpolicy -A -l must-gather.openshift.io/policy-name=<name>`.
- Find stale children: select on `must-gather.openshift.io/policy-generation` and
  compare to the policy's `metadata.generation`.
- Check rollout: `oc get mustgatherpolicy <name> -o jsonpath='{.status}'`.
