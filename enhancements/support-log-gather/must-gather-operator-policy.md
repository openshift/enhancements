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

# Must Gather Operator Managed Policy for Storage et al

## Summary

Introduce a cluster-scoped `MustGatherPolicy` CR through which administrators set
opt-in defaults and guardrails — storage, resource requests/limits, upload-target
allow list, and gather-pod `NetworkPolicy` — that the namespaced `MustGather` CR
inherits at reconcile time.

Delivered in two phases:

- **Phase 1 (`v1alpha1`):** `MustGatherPolicy` is a singleton (`metadata.name:
  cluster`); every `MustGather` inherits it implicitly.
- **Phase 2 (`v1`):** multiple policies may coexist; a `MustGather` selects one via
  `spec.inheritsPolicy`, and a policy declares which inherited fields a user may
  override.

All `MustGather` changes are strictly additive — a `status` field in Phase 1, an
optional `spec` field in Phase 2 — so no existing consumer breaks.

## Motivation

Today every `MustGather` author must independently know and repeat the right storage
class, capacity, resource sizing, and upload destination, and the cluster has no way
to constrain any of it. There is no mechanism for an administrator to set sane
defaults, bound the resources and egress of a potentially large gather workload, or
restrict where collected cluster data may be uploaded.

`MustGatherPolicy` centralizes that control while keeping individual runs simple.

### User Stories

- As a cluster administrator, I want to set cluster-wide defaults (storage class,
  size, retention, resource requests/limits) once, so users creating a `MustGather`
  need not specify them or pre-provision PVCs by hand.
- As a cluster administrator, I want to restrict the allowed upload targets so
  collected cluster data can only leave the cluster through trusted destinations.
- As a cluster administrator, I want to constrain gather-pod egress with a
  `NetworkPolicy` applied automatically to every run.
- As a cluster administrator, I want to decide per policy which fields a user may
  override, enforcing the rest as hard guardrails.
- As a user creating a `MustGather`, I want the administrator's policy inherited
  automatically.
- As a user, I want my run's status to record exactly which policy and revision was
  applied, for auditability.

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
- Determining the "right" resource sizing for the gather container. The policy
  only carries whatever requests/limits the administrator sets; the operator does
  not profile a cluster, size the workload for it, or adjust the values over time.
  A limit set too low means the gather container is OOM-killed and the run fails —
  that is the administrator's call to make and correct.
- Automatically resizing a PVC that is filling up. The policy sets a fixed `size`
  and the volume is not grown mid-run, nor is a full-utilization threshold acted
  on. Doing this well needs per-run volume-usage metrics the operator does not
  publish yet; once those exist a follow-up can add a full threshold and opt-in
  online expansion as additive fields. Until then, a run that outgrows its volume
  fails and the administrator raises `size`.
- Automatically retrying a failed run. A `MustGather` is attempted once; it is not
  retried, not even when `metadata.generation` changes (for example, when the spec
  is edited after a failure). Re-running means creating a new `MustGather`.

## Proposal

An administrator creates a `MustGatherPolicy`. On each `MustGather` reconcile the
operator resolves the applicable policy, merges its defaults with the spec,
provisions or reuses a PVC, applies the resource limits and gather-pod
`NetworkPolicy`, checks the upload target against the allow list, and records the
policy reference and observed revision in `MustGather.status`. A separate
controller reconciles the policy itself.

Inheritance is opt-in and OFF by default: with no `MustGatherPolicy` present,
`MustGather` behaves as it does today and `status.inheritsPolicy` is unset.

### Workflow Description

**cluster administrator** is a human user with cluster-admin, responsible for the
cluster's storage, egress, and support posture.

**must-gather user** is a human user (support engineer, SRE, or namespace owner)
who creates a `MustGather` to collect data.

**must-gather operator** is the controller pair described in this enhancement: a
`MustGatherPolicy` controller and the existing `MustGather` controller.

Starting state: the must-gather operator is installed and no `MustGatherPolicy`
exists, so runs behave exactly as they do today.

1. The cluster administrator creates the `MustGatherPolicy` named `cluster`,
   setting the storage profile (storage class, size, retention), resource
   requests/limits, the upload-target allow list, and the gather-pod
   `NetworkPolicy`.
2. The policy controller validates the policy, fans the `NetworkPolicy` out to
   every namespace, and reports `Available` with `observedGeneration`.
3. The must-gather user creates a `MustGather` in their namespace, leaving the
   storage, resources, and upload fields unset.
4. The `MustGather` controller resolves the policy, merges its defaults with the
   spec, provisions a PVC from the storage profile, and starts the gather job
   with the policy's resources and `NetworkPolicy` applied.
5. The must-gather user reads `status.inheritsPolicy` to see which policy and
   revision was applied, and collects the archive from the PVC.
6. After the run completes, the `MustGather` controller reclaims the PVC once
   `retentionPeriod` has elapsed.

Variations:

- **User overrides a field.** In Phase 1 a user value always wins over the policy
  default. In Phase 2 the administrator lists the overridable fields in
  `allowUserOverrides`; a value for any other field rejects the run with a clear
  status message rather than silently dropping it.
- **Upload target not allowed.** The controller fails the run before starting the
  gather job and reports the disallowed target in status; nothing is collected
  and nothing leaves the cluster.
- **No policy present.** Steps 1–2 never happen, `status.inheritsPolicy` stays
  unset, and the run uses the operator's existing behavior.
- **Run fails.** The run is not retried (see Non-Goals); the user fixes the cause
  — for example an undersized memory limit or volume — and creates a new
  `MustGather`.

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
    # per-type constraints live under a sub-field named for the type
    sftp:
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

When `spec.resources` is omitted, the operator applies the following defaults to
the gather container:

```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: "2"
    memory: 4Gi
```

These are starting values chosen to let a typical gather run to completion without
letting it monopolise a node, not a sizing recommendation: an administrator who
overrides them owns the consequences, including OOM kills on a too-small memory
limit (see Non-Goals).

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

	// A future revision may add volume-utilization handling here (a full
	// threshold and opt-in online expansion). It is deliberately absent until
	// the operator exposes per-run volume-usage metrics to drive it; see
	// Non-Goals.
}

// UploadTargetPolicy constrains the upload destinations a MustGather may use.
//
// allowedTypes is the discriminator: it says which upload target types are
// permitted at all. Constraints specific to a type live in the sub-field named
// for that type (sftp today), so a new type is added by appending one enum value
// and one sub-field, without reinterpreting any existing field.
//
// +kubebuilder:validation:XValidation:rule="!has(self.sftp) || (has(self.allowedTypes) && 'SFTP' in self.allowedTypes)",message="sftp may only be set when SFTP is listed in allowedTypes"
type UploadTargetPolicy struct {
	// allowedTypes lists the upload target types permitted by this policy. When
	// omitted or empty, no upload target is permitted.
	// +optional
	// +listType=set
	AllowedTypes []UploadType `json:"allowedTypes,omitempty"`

	// sftp constrains SFTP upload targets. It may only be set when SFTP appears
	// in allowedTypes. When SFTP is allowed and this is omitted, no SFTP host is
	// permitted.
	// +optional
	SFTP *SFTPUploadTargetPolicy `json:"sftp,omitempty"`

	// Future upload target types are added here as sibling sub-fields, e.g.
	//   S3 *S3UploadTargetPolicy `json:"s3,omitempty"`
	// each gated on its own value in allowedTypes.
}

// SFTPUploadTargetPolicy constrains SFTP upload destinations.
type SFTPUploadTargetPolicy struct {
	// allowedHosts is an allow list of SFTP destination hostnames permitted as
	// upload targets. When omitted or empty, no SFTP host is permitted.
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
> cluster-scoped→namespaced GC "will not be enforced." That inverts the rule: a
> namespaced dependent with a cluster-scoped owner *is* collected; only the reverse
> (namespaced owner, cross-namespace dependent) is not. We re-verify against the
> target Kubernetes version and backstop with a finalizer regardless (see
> [Deletion handling](#deletion-handling-ownerreferences-and-finalizers)).

The per-run **PVC** is tracked by its `MustGather` via discovery labels rather than
an ownerReference, so retention can outlive the run (see
[Deletion handling](#deletion-handling-ownerreferences-and-finalizers)).

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

A run's NetworkPolicy is the fanned-out one already present in its namespace (owned
by the policy), not a per-run object. The only per-run child is the **PVC**, tracked
by discovery labels (not an ownerReference, so retention can outlive the run), so its
name need not be copied into `MustGather.status`. That keeps `MustGather.status` to
the two `observedGeneration` values and the policy reference.

#### Deletion handling: ownerReferences and finalizers

Two cleanup paths, one per owner. Each pairs an `ownerReference` (GC fast path) with
a finalizer (backstop for what GC can't express).

**Policy → fanned-out NetworkPolicies.** Each child gets a controller
`ownerReference` to the `MustGatherPolicy`, carrying the owner `UID` (so a recreated
same-name policy never adopts stale children) and `blockOwnerDeletion: true`. This
is the supported GC direction — cluster-scoped owner, namespaced dependent — so the
common case needs no controller involvement. We don't rely on it alone; the policy
also carries `must-gather.openshift.io/cleanup-networkpolicies`. On
`deletionTimestamp`: stop writing children, enumerate by
`policy-uid=<uid>` selector, delete all, wait for the selector to go empty, then
drop the finalizer. Idempotent, and correct whether or not cross-scope GC is
enforced on the target version — if GC ran, the finalizer step is a no-op.

Selector-driven cleanup (not the owner graph) also recovers children orphaned by a
mid-fan-out crash; a startup reconcile re-stamps missing owners/labels. Gate the
create path on `deletionTimestamp == nil` so the `Namespace` watch stops
materializing children once deletion starts, and treat `NotFound` (child in a
`Terminating` namespace) as success.

**MustGather → per-run PVC.** The only per-run child is the PVC, a same-namespace
dependent for which a direct `ownerReference` would normally suffice — except it must
outlive the run by `retentionPeriod`, and ownerReference GC is immediate. So a
retained PVC is
deliberately **not** owner-referenced; it carries the discovery labels plus a
`must-gather.openshift.io/reclaim-after: <RFC3339>` annotation, and a reaper deletes
it past that time (skipping volumes with a live mounter). A
`must-gather.openshift.io/pvc-retention` finalizer on the `MustGather` stamps
`reclaim-after` from completion time on an early delete, so retention survives even
when the run is removed first.

ownerReferences handle everything whose lifetime equals its owner's; finalizers
cover the two cases they can't — cross-scope GC that may be unenforced, and
retention that must defer deletion.

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
  `MustGather` controller manages the per-run PVC — provisioning it from the profile
  and applying the policy's retention settings.
  Retention/reclaim must not race with a run still mounting the volume.

- **Singleton enforcement (Phase 1).** A CEL `XValidation` rule pins
  `metadata.name == "cluster"`.

- **Per-type upload constraints.** `uploadTargets` keeps `allowedTypes` as the
  discriminator and puts type-specific constraints in a sub-field named for the
  type (`sftp.allowedHosts`). A flat `allowedHosts` would have to mean different
  things per type — a hostname for SFTP, an endpoint or bucket for an object
  store — so adding a type later would either overload the field or require a
  breaking change. With sub-fields, a new type costs one enum value plus one
  struct, and CEL gates each sub-field on its type being allowed.

- **NetworkPolicy ownership.** The operator sets the `podSelector` itself so each
  `NetworkPolicy` targets only its gather pods; a user-supplied `podSelector` is
  ignored, keeping unrelated workloads unaffected.

- **NetworkPolicy fan-out.** The policy controller writes the `NetworkPolicy` to
  every namespace and watches `Namespace` creation so new namespaces converge.
  Each object is owned by the policy and carries the discovery labels.

- **Lazy PVC provisioning.** The PVC is created only when a run needs one, in the
  run's namespace, tracked by discovery labels (not an ownerReference) so retention
  can outlive the run.

- **Cleanup.** See
  [Deletion handling](#deletion-handling-ownerreferences-and-finalizers):
  ownerReference GC with a finalizer backstop for the policy's NetworkPolicies, and
  retention-aware reclaim for per-run PVCs.

### Topology Considerations

The feature needs a CSI driver and a StorageClass; where either is absent, storage
falls back to ephemeral, which is the same as existing behaviour. The policy
controller and the `MustGather` controller both run in the existing operator and
add no control-plane components.

#### Hypershift / Hosted Control Planes

No special considerations. The must-gather operator runs in the hosted cluster and
the policy is a hosted-cluster resource, so a `MustGatherPolicy` applies to that
hosted cluster's workloads only. Nothing is placed in or read from the management
cluster, and no management-cluster storage is consumed.

#### Standalone Clusters

The primary target topology; everything described in this enhancement applies
without modification.

#### Single-node Deployments or MicroShift

For SNO, the resource defaults matter more than on multi-node clusters: the
gather pod competes with the control plane on the one node. The administrator can
lower the policy's requests/limits for SNO; the operator does not size them (see
Non-Goals). No extra control-plane footprint is added — the policy controller is
part of the existing operator binary.

MicroShift does not ship the must-gather operator, so this enhancement does not
apply there.

#### OpenShift Kubernetes Engine

The must-gather operator is an OLM-delivered optional operator, so it is available
on OKE on the same terms as today; the policy adds nothing OKE-specific.

## Implementation History

- [Must-Gather Operator: PVC destination for gathered data](./must-gather-operator-pvc-destination.md)
- [Must Gather Operator](./must-gather-operator.md)

## Alternatives (Not Implemented)

- **Per-namespace policy object** instead of cluster-scoped fan-out. Rejected: it
  pushes policy authoring onto every namespace owner and defeats the central-control
  goal.

## Infrastructure Needed

None beyond the existing must-gather operator. No new images, CI, or external
services; the new controller ships in the existing operator binary.

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
  controller manages per-run PVCs and emits metrics for
  provisioned/retained/reclaimed volumes.
- Failure modes to define: invalid or unavailable StorageClass; upload target not
  in the allow list (fail the run clearly); `NetworkPolicy` unsupported on the
  platform; PVC filled by a run (the run fails; the volume is not grown — see
  Non-Goals); a subset of namespaces failing
  fan-out (reported via `Degraded`).

## Support Procedures

**Detecting failure modes.**

- A `MustGather` that never starts: `status.inheritsPolicy` shows the resolved
  policy and revision. Unset means no policy applied; set with the run stuck
  means resolution succeeded but admission of the merged spec failed, and the
  reason is in the `MustGather` status conditions.
- Rejected upload target: the run fails before the gather job starts, with a
  condition naming the target and the allow list it violated. Nothing was
  collected and nothing left the cluster.
- Rejected override (Phase 2): the run fails with a condition naming the field
  the user set and the policy's `allowUserOverrides`.
- Storage problems: the per-run PVC stays `Pending` with the usual
  `ProvisioningFailed` events from the CSI driver (bad or missing StorageClass,
  quota exhausted). The operator surfaces this on the `MustGather` and in the
  provisioned/reclaimed PVC metrics.
- Gather container OOM-killed: pod termination reason `OOMKilled` on the gather
  job; the memory limit in the policy is too low for the cluster (see
  Non-Goals). The run is not retried.
- `NetworkPolicy` fan-out trouble: the `MustGatherPolicy` reports `Degraded` with
  the namespaces that failed, and `observedGeneration` lags `metadata.generation`
  while a rollout is incomplete.

**Disabling the API extension.** Deleting the `MustGatherPolicy` disables policy
inheritance: the policy controller removes the `NetworkPolicy` objects it owns,
and subsequent runs fall back to today's behavior with `status.inheritsPolicy`
unset. Cluster health is unaffected — no other component depends on the policy.

Consequences for running workloads: in-flight `MustGather` runs keep the merged
configuration they already resolved and their PVCs, which are retained by
discovery label and the `pvc-retention` finalizer rather than by the policy, so
no collected data is lost. Removing the policy removes the egress restriction on
future gather pods and the upload allow list, so uploads that were previously
blocked become possible again — that is the relevant risk when disabling it, not
cluster health. Removing the `MustGatherPolicy` CRD itself requires deleting the
CR first; the CRs hold only configuration, so no gathered data is lost.

## Test Plan

- **Unit:** field resolution/merge order (user vs. policy vs. default), singleton
  CEL validation, label/ownerReference stamping, and the Phase 2 `allowUserOverrides`
  rejection path.
- **Integration (envtest):** policy create/update/delete drives NetworkPolicy
  fan-out and finalizer cleanup; `Namespace` creation converges; deletion leaves no
  orphaned children; PVC retention/reclaim and the `pvc-retention` finalizer honor
  `retentionPeriod` including early `MustGather` delete.
- **e2e:** end-to-end inheritance of storage, resources, upload-target allow list,
  and gather-pod NetworkPolicy; `status.inheritsPolicy` populated/unset correctly
  (opt-in verified); upload to a disallowed target fails clearly;
  `Degraded` reported when a subset of namespaces fail fan-out.
- **Upgrade:** `v1alpha1`→`v1` identity conversion with stored objects; mixed
  operator/API-server skew during graduation.

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
operator versions interoperate: an older operator ignores the new
`spec`/`status` fields, and the fanned-out NetworkPolicies remain valid on their own.
