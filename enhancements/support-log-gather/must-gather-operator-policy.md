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

An administrator creates a `MustGatherPolicy`. When a `MustGather` is reconciled,
the operator resolves the applicable policy, merges the policy-provided defaults
with the `MustGather` spec, provisions or reuses a PVC accordingly, applies the
resource requests/limits and (optionally) the gather-pod `NetworkPolicy`,
validates the requested upload target against the allow list, and records the
resolved policy reference and observed revision in `MustGather.status`.

A dedicated controller reconciles `MustGatherPolicy` (validating it and surfacing
status), separate from the existing `MustGather` controller which consumes the
resolved policy.

Policy inheritance is **opt-in and OFF by default**: until the administrator
creates a `MustGatherPolicy`, `MustGather` behavior is unchanged and
`status.inheritsPolicy` is unset.

### API Extensions

Two new kinds are introduced under the existing `operator.openshift.io` group,
plus additive changes to the existing `MustGather` kind. The API is versioned so
that Phase 1 ships `v1alpha1` and Phase 2 promotes to `v1`.

#### Phase 1 (Tech Preview): `v1alpha1` singleton `MustGatherPolicy`

`MustGatherPolicy` is **cluster-scoped** and, in Phase 1, a **singleton**: the
only accepted name is `cluster`. This mirrors the well-established OpenShift
config singleton convention (e.g. `config.openshift.io` resources) and avoids the
ambiguity of "which policy applies" before the selection mechanism exists in
Phase 2.

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

Additive change to `MustGather` in Phase 1 — a new **status** field only (no spec
change):

```go
// api/v1/mustgather_types.go (and v1alpha1)

// MustGatherStatus defines the observed state of MustGather
type MustGatherStatus struct {
	// ... existing fields unchanged ...

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

`observedGeneration` records the policy's `metadata.generation` that was last
reconciled into this `MustGather`. Because Phase 1 adds only an optional `status` field, it is fully backward
compatible. The existing `MustGather` spec-immutability validation
(`self.spec == oldSelf.spec`) is unaffected because no spec field is added.

#### Phase 2 (GA): `v1` multiple policies + user-selectable inheritance

The API graduates to `v1`. `MustGatherPolicy` is no longer a singleton: multiple
policies may exist, and a `MustGather` selects one to inherit. The policy also
gains the ability to declare which inherited fields a user may override.

Additive change to `MustGather` in Phase 2 — a new optional **spec** field
(set at creation, consistent with the existing immutable-spec model), plus reuse
of the same `status.inheritsPolicy` introduced in Phase 1:

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

The `status.inheritsPolicy` field is identical to Phase 1, which is what makes
the transition forward compatible: a Phase 1 client that only reads
`status.inheritsPolicy` continues to work unchanged; in Phase 2 the reconciler
simply resolves the policy named in `spec.inheritsPolicy` (instead of the
singleton) and records the same reference + `observedGeneration` in status.

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

The migration from `v1alpha1` to `v1` follows the same identity-conversion
pattern already used for `MustGather` (v1 is a superset/compatible copy of
v1alpha1, so Kubernetes performs identity conversion without a conversion
webhook; v1 becomes the storage version and v1alpha1 remains served).

### Implementation Details/Notes/Constraints

- **Opt-in / OFF by default.** The `MustGather` controller only consults a policy
  when one exists. In Phase 1 that means the singleton `MustGatherPolicy/cluster`;
  in Phase 2 it means a policy named by `spec.inheritsPolicy`. Absent that,
  behavior is exactly as it is today and `status.inheritsPolicy` stays unset.

- **Resolution & merge order.** For each inheritable field, the effective value is:
  the user-set `MustGather` spec value if present and permitted, otherwise the
  policy value, otherwise the operator default. In Phase 2, if a field is *not*
  in the policy's `allowUserOverrides` and the user set it, the run is rejected.

- **Inheritance scope.** Only `storage`, `resources`, `uploadTarget`, and
  `networkPolicy` participate in inheritance. `serviceAccountName`,
  `imageStreamRef`, and `obfuscate` are explicitly excluded for now (see
  Non-Goals).

- **Status recording.** On every successful reconcile the controller sets
  `status.inheritsPolicy.name` and `status.inheritsPolicy.observedGeneration` to
  the policy actually applied. Downstream tooling can compare
  `observedGeneration` against the live policy's `metadata.generation` to detect
  runs made under a stale policy.

- **Two controllers.** A new controller owns `MustGatherPolicy` (validation,
  status, and lifecycle of managed PVCs — retention, full-threshold handling,
  optional expansion). The existing `MustGather` controller consumes the resolved
  policy. Care is needed so PVC lifecycle actions (retention/reclaim) do not race
  with in-flight `MustGather` runs still mounting the volume.

- **Singleton enforcement (Phase 1).** Enforced via a CEL/`XValidation` rule
  pinning `metadata.name == "cluster"`, matching OpenShift config-singleton
  conventions.

- **NetworkPolicy ownership.** The operator overwrites the `podSelector` of the
  materialized `NetworkPolicy` so it can only ever target the gather pods it
  creates; a user-supplied `podSelector` is ignored to prevent a policy from
  affecting unrelated workloads.

### Topology Considerations
Nil

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

All `MustGather` changes are additive (a status field in Phase 1, an optional
spec field in Phase 2), so upgrading the operator never breaks existing
`MustGather` objects. 

The `v1alpha1` -> `v1` API graduation uses identity conversion, so no conversion webhook is required and stored objects
convert transparently.

## Operational Aspects of API Extensions

**Storage**:
- Two reconcilers are added. The `MustGatherPolicy` controller manages PVC lifecycle; operator should surface `Degraded`/`Progressing`-style conditions on the policy status and metrics for provisioned/retained/reclaimed volumes.
- Failure modes to define: policy references an invalid/unavailable StorageClass; requested upload target not in the allow list (must fail the run clearly); `NetworkPolicy` unsupported on the platform; PVC full and expansion not possible.

## Test Plan

<!--
### Risks and Mitigations

### Drawbacks

### Removing a deprecated feature

## Version Skew Strategy

## Support Procedures

-->
