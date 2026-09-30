---
title: pod-security-admission-config
status: provisional
authors:
  - "@ibihim"
  - "@ehearne-redhat"
reviewers:
  - "@liouk"
  - "@everettraven"
approvers:
  - "@deads2k"
  - "@sjenning"
api-approvers:
  - "@deads2k"
  - "@JoelSpeed"
creation-date: 2025-01-23
last-updated: 2025-09-30
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3722
see-also:
  - "/enhancements/authentication/pod-security-admission.md"
  - "enhancements/authentication/pod-security-admission-autolabeling.md"
replaces: [autolabeling]
superseded-by: []
---

# Pod Security Admission Enforcement Config

## Summary

[Pod Security Admission (PSA)](https://kubernetes.io/docs/concepts/security/pod-security-admission/) enforcement is on by default in OpenShift today. **This enhancement turns it off by default and makes it opt-in**, through a new `podSecurityAdmission` field on the existing `config.openshift.io/v1 APIServer` resource that records the administrator's requested [Pod Security Standard (PSS)](https://kubernetes.io/docs/concepts/security/pod-security-standards/) and is applied only once the cluster has been evaluated as able to meet it.

Two changes deliver this, and they are independent:

1. **`OpenShiftPodSecurityAdmission` leaves the `Default` feature set**, so the kube-apiserver's global `enforce` level becomes `privileged` instead of `restricted`.
2. **The PSA label syncer is retired**, in every mode, on every cluster. It does not run at `Privileged` and it does not come back when an administrator opts in.

The two go together historically: the syncer exists *because* the global default is `restricted`, as the thing that keeps a Namespace whose ServiceAccounts cannot meet that standard from having its workloads rejected. Remove the mandatory `restricted` default and its damage control goes with it — see [Why it is retired outright](#why-it-is-retired-outright).

This is a deliberate reduction in the out-of-the-box security posture, traded for the guarantee that no cluster acquires failing workloads without its administrator opting in. It requires explicit security sign-off, a [release note](#release-note), and the migration handling in [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy).

This enhancement expands the ["PodSecurity admission in OpenShift"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission.md) and ["Pod Security Admission Autolabeling"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission-autolabeling.md) enhancements.

### What "enabling" and "disabling" mean

These words are used precisely throughout and are easy to over-read:

- **Opting in** means `spec.podSecurityAdmission.enforcementMode: Baseline` or `Restricted`. The kube-apiserver's global `enforce` level becomes that standard, applying to every Namespace with no `pod-security.kubernetes.io/enforce` label of its own.
- **Opting out** means `Privileged`, or leaving the field unset. The two are the same state and it is the shipped default — see [Unset means `Privileged`](#unset-means-privileged).

Throughout this document `spec.enforcementMode` and `status.enforcementMode` are shorthand for `spec.podSecurityAdmission.enforcementMode` and `status.podSecurityAdmission.enforcementMode` on `apiserver/cluster`, written out in full where the distinction matters.

`spec.enforcementMode` selects exactly one thing: the global `enforce` level. It turns no component on or off. "Disabling PSA" means **exactly `enforce: privileged`** — the `PodSecurity` admission plugin stays loaded, `warn` and `audit` stay pinned to `restricted` so violations remain observable, the `PodSecurityReadinessController` keeps evaluating, and per-Namespace labels keep taking precedence over the global default.

Because nothing computes a per-Namespace label any more, a Namespace with no `enforce` label on a cluster that opts in is held to the global standard directly. That is what makes the [violation evaluation](#podsecurityreadinesscontroller) the load-bearing safety mechanism of this enhancement rather than a convenience.

## Motivation

After PSA and SCC-based autolabeling were introduced, some clusters were found to have Namespaces with Pod Security violations. The number has dropped significantly over the last several releases, but it is not zero, and it is essential to avoid any scenario where users end up with failing workloads.

Making enforcement elective puts the onus on the administrator to have PSA-compliant workloads before enforcement is turned on, rather than discovering non-compliance as admission failures after an upgrade. Out of the box, `Privileged` is the global default and the label syncer does not run. The `PodSecurityReadinessController` keeps evaluating regardless — that is the part that must not be switched off, because with the syncer retired it is the *only* thing that tells an administrator whether opting in is safe. When the administrator requests `Baseline` or `Restricted` and the evaluation finds no violating Namespaces, the requested level is applied. Namespaces that cannot meet it are surfaced in `status.violatingNamespaces` with a reason, for the administrator to fix or label by hand, rather than being quietly granted a lower standard.

OpenShift strives to offer the highest security standards, and PSA enforcement is on by default today in service of that. This enhancement trades that default for predictability: rather than enforcing a standard a minority of clusters cannot meet, it gives administrators an explicit, reversible lever and the diagnostics to use it safely. Clusters that opt in reach the same posture as today; clusters that do not remain at `Privileged` until their administrator chooses otherwise.

The cost of that trade is that clusters taking no action end up less restrictive than they are today. Mitigating it: per-Namespace `pod-security.kubernetes.io/*` labels continue to take precedence and are unaffected, and SCCs — OpenShift's primary workload admission control — are unchanged.

### Goals

1. Allow users to configure Pod Security Admission enforcement on their clusters.
2. Prevent clusters from acquiring failing workloads by making this feature elective.
3. Allow users to enable and disable this feature at will.
4. **Retire the PSA label synchronization controller outright** — on every cluster, in every enforcement mode — and retire the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation with it. This is not conditional on the new API and is not reversed by opting in: it happens whatever `spec.enforcementMode` says. It has the largest blast radius of anything in this enhancement, and reviewing the API without reviewing this has not reviewed the change.

### Non-Goals

1. **Keeping PSA enforcement enabled by default.** Enforcement is on by default today; this enhancement deliberately makes it opt-in, and no mechanism preserves the current default for clusters that take no action.
2. **Providing per-Pod violation detail in the API.** The API reports violating Namespaces and a reason per Namespace, so that `status` stays bounded on clusters with many Namespaces. Per-Pod detail remains available via the server-side dry-run in [Support Procedures](#support-procedures).
3. **Restricting per-Namespace PSA configuration.** Per-Namespace labels continue to take precedence over the cluster-wide default. This enhancement only changes the global default applied to Namespaces carrying no `enforce` label.

## Proposal

### User Stories

As a System Administrator:

- I want to electively enable PSA enforcement only if the cluster would have no failing workloads.
- If workloads in certain Namespaces would fail under enforcement, I want to identify which Namespaces need adjusting.
- If I enable PSA enforcement and decide I can no longer use it, I want to disable it across my clusters at will.

### Workflow Description

The **cluster administrator** is responsible for the security posture of a cluster and is the only actor that writes `apiserver/cluster`'s `spec.podSecurityAdmission`. The **application owner** deploys workloads; they never touch this API but experience its consequences when a Pod is rejected. The **`PodSecurityReadinessController`** and the **Config Observer** both run in `cluster-kube-apiserver-operator`, owning `status.podSecurityAdmission` and the kube-apiserver's global `PodSecurity` admission configuration respectively.

The **PSA label syncer** is not an actor here. It does not run in any enforcement mode and does not read this API; it appears in this document only as the source of labels already present on upgraded clusters.

The starting state is a cluster on release `n+1` or later with `spec.enforcementMode` unset, resolving to `Privileged`.

Enabling enforcement:

1. The administrator reads the current evaluation with `oc get apiserver cluster -o jsonpath='{.status.podSecurityAdmission}' | jq`. `conditions` shows whether an evaluation has ever completed (`Evaluated`) and whether it is recent (`StatusStale`); `violatingNamespaces` lists what would break, with a reason for each.
2. They either resolve each violating Namespace — see [Support Procedures](#support-procedures) — or decide to accept them.
3. They record their intent:

   ```bash
   oc patch apiserver cluster --type=merge \
       -p '{"spec":{"podSecurityAdmission":{"enforcementMode":"Restricted"}}}'
   ```

   If violations are currently recorded, the apiserver returns an advisory `Warning` header naming the count and the acknowledgement required. The update is not rejected.
4. On its next sweep the `PodSecurityReadinessController` resolves `spec` into `status`. With no violations outstanding it sets `status.enforcementMode: Restricted` and `EnforcementBlocked=False`.
5. The Config Observer re-renders the kube-apiserver configuration with `enforce: restricted`, cutting a new static pod revision that rolls the control plane one node at a time. Nothing else starts: no controller begins writing Namespace labels, and every Namespace without an `enforce` label of its own is now held to `restricted`.
6. They confirm with `oc get apiserver cluster -o jsonpath='{.status.podSecurityAdmission.enforcementMode}'`.

**Variation — violations outstanding.** At step 4 the controller leaves `status.enforcementMode` at `Privileged` and sets `EnforcementBlocked=True` with reason `ViolatingNamespaces`. The request stays on record: resolving the violations causes the requested mode to take effect on the next evaluation with no further action. Alternatively the administrator copies `status.podSecurityAdmission.knownViolationsHash` into `spec.podSecurityAdmission.acknowledgeKnownViolations`, which unblocks that specific set of findings and no later ones.

**Variation — disabling.** The administrator sets `spec.enforcementMode: Privileged`, or removes the field. The global `enforce` level returns to `privileged`; nothing else about PSA changes, and Namespaces that already carry an `enforce` label keep enforcing at that label's level. This is applied unconditionally and never gated on an evaluation, because the escape hatch has to work when the evaluation machinery is exactly what has failed. On an upgraded cluster this may not be enough — see [Retained labels on upgraded clusters](#retained-labels-on-upgraded-clusters).

**Variation — Day 0.** Enforcement can in principle be requested at install time; the install-time surface is not yet specified. See [Open Questions](#day-0-configuration).

### API Extensions

This enhancement adds two optional fields to an existing API and changes three existing surfaces:

- **`spec.podSecurityAdmission` and `status.podSecurityAdmission` on `config.openshift.io/v1 APIServer`** — the installer-rendered cluster-scoped singleton `apiserver/cluster`, carrying the administrator's requested level in `spec` and the resolved, actually-in-force level in `status`. Both are gated on `PodSecurityAdmissionConfiguration`. **No new CRD is introduced**; see [Why this lives on `APIServer`](#why-this-lives-on-apiserver).
- **The kube-apiserver's `PodSecurity` admission plugin configuration** changes meaning: `enforce` becomes derived from `status.podSecurityAdmission.enforcementMode` rather than from the `OpenShiftPodSecurityAdmission` feature gate alone. `audit` and `warn` stay pinned to `restricted` and are unchanged.
- **Namespace metadata written by another component.** The PSA label syncer, owned by `cluster-policy-controller`, no longer runs at all, so no `pod-security.kubernetes.io/*` label and no `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation is written by it on any cluster. Namespaces are a core upstream resource; the change is purely subtractive, and metadata already present is retained untouched.
- **An advisory `Warning` header** on updates that raise `spec.podSecurityAdmission.enforcementMode` while violations are recorded. It never rejects a request, and it is emitted only when that field actually changes — an administrator editing `tlsSecurityProfile` on the same object must not be warned about PSA.

No admission webhooks, conversion webhooks, aggregated API servers or finalizers are introduced. Because the fields land on an existing `v1` type there is also **no API version migration**: nothing is promoted from `v1alpha1` to `v1`, no two versions are served at once, and no conversion is written. See [Operational Aspects of API Extensions](#operational-aspects-of-api-extensions).

#### API version and availability

Throughout this document, release `n` is the release in which PSA enforcement stops being the default, currently targeted at **OpenShift 5.1**; `n-1` is 5.0 and `n+1` is 5.2.

`APIServer` is already `config.openshift.io/v1` and is present on every standard cluster, so what ships per release is the **fields**, not the type. Both carry `+openshift:enable:FeatureGate=PodSecurityAdmissionConfiguration`, following `tlsAdherence` and `encryption.kms` on this same type: the field is compiled into the CRD schema only in feature sets where the gate is on. The fields are therefore in `TechPreviewNoUpgrade` in release `n` and in `Default` from release `n+1`.

This means the configuration surface is **not** available on a supported, upgradeable cluster in release `n`. That is accepted, because the two changes are needed in different releases: release `n` delivers *PSA enforcement is off by default*, which requires only removing `OpenShiftPodSecurityAdmission` from the `Default` feature set; release `n+1` delivers *and here is the supported way to turn it back on*. Turning enforcement **off** is the safe direction, so release `n` needs no opt-out mechanism. An administrator on release `n` who wants enforcement back on before the fields are generally available can set it through `unsupportedConfigOverrides` on `kubeapiservers.operator.openshift.io`, as documented in the [original PSA enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/pod-security-admission.md). That is unsupported and blocks upgrade until removed.

#### Why this lives on `APIServer`

A dedicated `PSAEnforcementConfig` CRD was the earlier shape of this proposal. Three properties moved it onto `config.openshift.io/v1 APIServer` instead.

**It is already the cluster-wide API server configuration surface.** `enforce` is a kube-apiserver admission plugin setting, and the resource that already carries `audit`, `encryption` and `tlsSecurityProfile` — each of them a kube-apiserver behaviour an administrator sets cluster-wide — is where a reader looks for one more. A separate CRD would have added a *fourth* place to hold PSA state, alongside the rendered admission configuration, the Namespace labels and the feature gate.

**It is already consumed by `cluster-kube-apiserver-operator`'s config observers**, which is precisely the machinery this enhancement extends, and it is installer-rendered, so `apiserver/cluster` exists on every standard cluster from install. That deletes a whole state from the design: outside MicroShift there is no "singleton absent" case, and nothing has to decide whether to create an object on the administrator's behalf.

**HyperShift's request plumbing already exists.** `HostedCluster.spec.configuration.apiServer` is a `*configv1.APIServerSpec` ([`hostedcluster_types.go#L2818`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)), so the *spec* side reaches a hosted cluster with no new API surface at all. The *status* side does not — `ClusterConfiguration` carries specs only — which makes that the sharper half of the [open question](#hypershift-configuration-surface) rather than the whole of it.

The cost is real and api-approvers will weigh it: **this enhancement would be the first writer of `APIServerStatus`**, which is an empty struct today ([`types_apiserver.go#L270`](https://github.com/openshift/api/blob/master/config/v1/types_apiserver.go)). It is precedent-setting for this type, though not for the group — `Infrastructure` and `Network` both carry operator-written status in `config.openshift.io/v1`. The mechanics are less of a step than the emptiness suggests: the generated CRD already declares a `status` property and already enables the `status` subresource, so what changes is the schema under it, not the resource's shape. What genuinely is new is that `cluster-kube-apiserver-operator` becomes a *writer* of a `config.openshift.io` resource it has only ever read, which needs RBAC it does not have today and makes a config resource partly operator-owned for the first time.

Two consequences are designed for rather than waved away:

- **Everything nests under `status.podSecurityAdmission`** rather than being hoisted to the root of `APIServerStatus`. `conditions` in particular is deliberately *not* claimed at top level. Hoisting it would make PSA the owner of a name the type as a whole will eventually want, and would place a feature-gated field where turning the gate on changes the shape of the status root. Nested, the subtree is added, gated and pruned as one unit.
- **`violatingNamespaces` must stay bounded.** A list that grows with Namespace count is a heavier liability on a core config resource that every cluster has than on a CRD only this feature reads. It is capped, and because a cap that silently truncates is worse than no cap, the full count is carried in the `EnforcementBlocked` condition message and in the `pod_security_readiness_violating_namespaces` metric.

#### The `podSecurityAdmission` fields

`spec` is owned exclusively by the user and is never written by any controller. `status` is owned exclusively by the `PodSecurityReadinessController`. Where the two disagree — because the user requested `Restricted` but violating Namespaces were found — `status.podSecurityAdmission.enforcementMode` reports what is actually in force and the `EnforcementBlocked` condition explains why. This keeps the user's intent recorded, so resolving violations lets the requested mode take effect without re-stating it, and avoids fighting GitOps controllers that continuously re-apply the desired `spec`.

```go
package v1 // config/v1/types_apiserver.go in openshift/api

type APIServerSpec struct {
	// ... existing fields: servingCerts, clientCA, additionalCORSAllowedOrigins,
	// encryption, tlsSecurityProfile, tlsAdherence, audit ...

	// podSecurityAdmission configures the Pod Security Standard that the
	// kube-apiserver enforces on Namespaces that carry no
	// pod-security.kubernetes.io/enforce label of their own.
	//
	// When omitted, this means the cluster has opted out of Pod Security
	// Admission enforcement. The PodSecurity admission plugin remains
	// configured, with warn and audit pinned to restricted.
	//
	// +openshift:enable:FeatureGate=PodSecurityAdmissionConfiguration
	// +optional
	PodSecurityAdmission PodSecurityAdmissionConfig `json:"podSecurityAdmission,omitempty,omitzero"`
}

type APIServerStatus struct {
	// podSecurityAdmission reports the Pod Security Admission enforcement
	// actually in force, and the cluster's readiness to raise it. It is written
	// by the PodSecurityReadinessController in cluster-kube-apiserver-operator.
	//
	// +openshift:enable:FeatureGate=PodSecurityAdmissionConfiguration
	// +optional
	PodSecurityAdmission PodSecurityAdmissionStatus `json:"podSecurityAdmission,omitempty,omitzero"`
}

// PodSecurityEnforcementMode defines the Pod Security Standard that should be applied.
// +kubebuilder:validation:Enum:=Privileged;Baseline;Restricted
type PodSecurityEnforcementMode string

const (
	// PodSecurityEnforcementModePrivileged indicates that the cluster should not enforce PSA restrictions and stay in a privileged mode.
	PodSecurityEnforcementModePrivileged PodSecurityEnforcementMode = "Privileged"
	// PodSecurityEnforcementModeBaseline indicates that the cluster should partially enforce PSA restrictions, if no violating Namespaces are found.
	PodSecurityEnforcementModeBaseline PodSecurityEnforcementMode = "Baseline"
	// PodSecurityEnforcementModeRestricted indicates that the cluster should enforce all PSA restrictions, if no violating Namespaces are found.
	PodSecurityEnforcementModeRestricted PodSecurityEnforcementMode = "Restricted"
)

// PodSecurityAdmissionConfig records the administrator's requested Pod Security
// Admission enforcement. It is never written by a controller.
type PodSecurityAdmissionConfig struct {
	// enforcementMode is the Pod Security Standard the user requests be enforced
	// on Namespaces that carry no pod-security.kubernetes.io/enforce label.
	// This field records user intent only and is never written by a controller.
	// - Privileged opts out of PSA enforcement. The PodSecurity admission plugin
	//   remains configured, with warn and audit pinned to restricted.
	// - Baseline enables the cluster to partially enforce PSA.
	// - Restricted enables the cluster to completely enforce PSA.
	//
	// This field selects the global enforce level and nothing else. In
	// particular, no mode starts the PSA label syncer: it is retired and nothing
	// computes a per-Namespace enforce label. A Namespace that cannot meet the
	// requested standard must carry its own pod-security.kubernetes.io/enforce
	// label, and is otherwise reported in status.
	//
	// The requested mode is only applied once the PodSecurityReadinessController
	// has confirmed the cluster has no violating Namespaces. Until then,
	// status.podSecurityAdmission.enforcementMode reports what is actually in
	// force, the EnforcementBlocked condition explains why, and
	// status.podSecurityAdmission.violatingNamespaces lists what must be resolved.
	//
	// The default is Privileged. Omitting this field and setting it to
	// Privileged are equivalent and both mean the cluster has opted out of PSA
	// enforcement. Unlike most optional enums in this group, the default is a
	// fixed commitment rather than a platform choice that may change: a cluster
	// that takes no action must never acquire enforcement it did not request.
	//
	// +optional
	EnforcementMode PodSecurityEnforcementMode `json:"enforcementMode,omitempty"`

	// acknowledgeKnownViolations allows the requested enforcementMode to be applied
	// even though the PodSecurityReadinessController has reported violating Namespaces.
	// It exists so that raising enforcement on a cluster with known, accepted violations
	// is a deliberate act rather than a silent one.
	//
	// The value must be the current status.podSecurityAdmission.knownViolationsHash,
	// which ties the acknowledgement to a specific set of findings. An acknowledgement
	// does not carry over to violations discovered by a later evaluation, but it does
	// survive an evaluation that rediscovers the same set, and it is unaffected by
	// changes to unrelated fields of this resource.
	//
	// When omitted, this means the user has no opinion and violations block the requested
	// mode from taking effect.
	//
	// +kubebuilder:validation:MaxLength=64
	// +optional
	AcknowledgeKnownViolations string `json:"acknowledgeKnownViolations,omitempty"`
}

// PodSecurityAdmissionStatus signals to the user the current state of PSA enforcement.
type PodSecurityAdmissionStatus struct {
	// enforcementMode indicates the PSA enforcement state actually in force in the
	// kube-apiserver's global PodSecurity configuration. This may lag or differ from
	// spec.podSecurityAdmission.enforcementMode when the requested mode has not been
	// applied; consult the EnforcementBlocked condition in that case.
	//
	// +optional
	EnforcementMode PodSecurityEnforcementMode `json:"enforcementMode,omitempty"`

	// evaluatedLevel is the Pod Security Standard that violatingNamespaces was
	// actually computed against by the sweep that produced it. It is not derivable
	// from spec.podSecurityAdmission.enforcementMode: when spec changes, the
	// previously reported list remains until the next sweep completes, and this
	// field dates it. Read it together with observedGeneration and
	// lastEvaluationTime.
	//
	// It is the requested mode when that is Baseline or Restricted, and Restricted
	// otherwise, so that a cluster which has not opted in still reports what opting
	// in would cost.
	//
	// +kubebuilder:validation:Enum:=Baseline;Restricted
	// +optional
	EvaluatedLevel PodSecurityEnforcementMode `json:"evaluatedLevel,omitempty"`

	// conditions reports the state of the evaluation and of enforcement. Defined types are:
	// - "Evaluated" is False with reason NeverRan until the controller completes its
	//   first successful sweep, so that an absent violatingNamespaces list is never
	//   mistaken for a clean cluster.
	// - "StatusStale" is True when the last evaluation is older than twice the
	//   evaluation interval.
	// - "EnforcementBlocked" is True with reason ViolatingNamespaces when
	//   the requested mode is Baseline or Restricted but violations prevent it. Its
	//   message carries the total violating Namespace count, which is authoritative
	//   when violatingNamespaces has been truncated.
	//
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the generation of the APIServer spec that
	// lastEvaluationTime and violatingNamespaces correspond to. When it is behind
	// metadata.generation, the reported status predates the current spec. Note that
	// this resource's generation also advances on changes to fields unrelated to
	// Pod Security Admission.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastEvaluationTime is when the PodSecurityReadinessController last evaluated
	// the cluster for PSA violations.
	//
	// +optional
	LastEvaluationTime metav1.Time `json:"lastEvaluationTime,omitempty"`

	// knownViolationsHash is a digest of the set of Namespace names in
	// violatingNamespaces, computed over the full set even when the reported list
	// is truncated. Copying it into spec.podSecurityAdmission.acknowledgeKnownViolations
	// acknowledges exactly these violations.
	//
	// It is a content digest rather than a resourceVersion because this resource is
	// shared: a resourceVersion would be invalidated by every unrelated write,
	// including the controller's own periodic status updates, silently revoking an
	// acknowledgement nobody withdrew.
	//
	// +kubebuilder:validation:MaxLength=64
	// +optional
	KnownViolationsHash string `json:"knownViolationsHash,omitempty"`

	// violatingNamespaces lists Namespaces that would violate evaluatedLevel.
	// When that level is the one requested in spec, these are the Namespaces that
	// must be resolved or acknowledged before the request takes effect. On a cluster
	// that has not opted in, the level is Restricted and the list is advisory: it
	// reports what raising enforcement would cost.
	//
	// The list is capped so that this resource stays bounded on clusters with many
	// Namespaces. When it is truncated, the EnforcementBlocked condition message and
	// the pod_security_readiness_violating_namespaces metric carry the true count.
	//
	// +kubebuilder:validation:MaxItems=100
	// +listType=map
	// +listMapKey=name
	// +optional
	ViolatingNamespaces []ViolatingNamespace `json:"violatingNamespaces,omitempty"`
}

// ViolatingNamespace provides information about a namespace that cannot comply
// with the chosen enforcement mode.
type ViolatingNamespace struct {
	// name is the Namespace that has been flagged as potentially violating if enforced.
	Name string `json:"name"`

	// reason is a textual description explaining why the Namespace is incompatible
	// with evaluatedLevel. It contains a prefix indicating which part
	// of the evaluation found the conflict:
	// - PSAConfig: Misconfigured OpenShift Namespace
	// - PSALabel: ServiceAccount with insufficient SCCs
	//
	// +optional
	Reason string `json:"reason,omitempty"`

	// lastTransitionTime is the time at which the state transitioned.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}
```

Two changes here are consequences of sharing a resource rather than owning one, and both are improvements.

**The acknowledgement is a content hash, not a `resourceVersion`.** On a dedicated CRD, `resourceVersion` was a serviceable token for "the findings I looked at". On `apiserver/cluster` it is not: the version advances when an administrator edits `tlsSecurityProfile`, and it advances on the readiness controller's own status writes — including the four-hourly `lastEvaluationTime` tick, which would revoke every acknowledgement within one sweep. `knownViolationsHash` is computed over the Namespace names, so it is stable across rediscovery of the same set and changes exactly when the set does. This is the behaviour the field was always described as having.

**`observedGeneration` is a weaker signal than it was.** `metadata.generation` on this resource advances on any spec change, PSA-related or not, so a stale `observedGeneration` no longer implies the *PSA* spec changed. It remains correct as a "status may predate spec" guard, which is what it is used for, and `evaluatedLevel` is what actually dates the finding against a level.

#### Unset means `Privileged`

| `spec.podSecurityAdmission.enforcementMode` | Meaning |
|---|---|
| `Baseline` or `Restricted` | **opt in** |
| `Privileged`, or the field omitted entirely | **opt out** |

`Privileged` *is* the default. A cluster that never mentions the field and a cluster that explicitly sets `Privileged` are in the same state and are treated identically by every consumer. There is no "no opinion" tier, and no consumer may branch on which of the two it sees.

Two API notes. **The enum does not include `""`.** Including the empty string is an established pattern — `config.openshift.io/v1 FeatureSet` does exactly that — but only because `FeatureSet` has no named value for its default. This enum does, and adding `""` would give two spellings of one state, forcing every consumer to normalise and making `oc get -o jsonpath='{.spec.podSecurityAdmission.enforcementMode}'` return different strings for identical clusters. Omitting the field, which `+optional` and `omitempty` already allow, is how the default is expressed. And **the field documents a fixed default, not a platform-chosen one**: the usual OpenShift wording for an optional enum, "the platform chooses a default, which is subject to change over time", is deliberately not used, because pinning the default to `Privileged` is the point of the enhancement.

Everything absent resolves the same way, which matters because "absent" arises in four shapes during the rollout:

| State | How it arises | Resolves to |
|---|---|---|
| the field is not in the schema | release `n` on a non-TechPreview cluster: the gate is off, so the CRD has no such field | `Privileged` |
| `apiserver/cluster` does not exist | MicroShift, where this configuration surface is not served — see [MicroShift](#single-node-deployments-or-microshift) | `Privileged` |
| `spec.podSecurityAdmission.enforcementMode` unset | the field exists and the administrator has not set it | `Privileged` |
| `status.podSecurityAdmission.enforcementMode` unset | the readiness controller has not resolved `spec` yet | `Privileged` |

A consumer that treats a field missing from the schema differently from a field left unset has a bug. The last row is the exception worth care: it resolves to `Privileged` like the rest, but it is an *unresolved* state rather than a settled one, which is why the Config Observer gates raising enforcement on the `Evaluated` condition rather than on `status.podSecurityAdmission.enforcementMode` being non-empty — see [Freshness is an interlock, not a hint](#freshness-is-an-interlock-not-a-hint). Lowering is never gated on it.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is affected structurally. The following are verified against the `openshift/hypershift` source:

- **There is no `cluster-kube-apiserver-operator` in a hosted control plane.** The control-plane-operator renders the kube-apiserver's PodSecurity configuration directly (`control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go`), keyed off whether `OpenShiftPodSecurityAdmission=true` appears in the rendered feature gate list. The Config Observer mechanism this enhancement builds on has a second, independent implementation that has to change in step with it.
- **The PSA label syncer runs as a management-cluster Deployment**, reconciled as a control-plane component (`.../v2/clusterpolicy/component.go`). The hosted-cluster-config-operator (HCCO) runs no syncer logic of its own; it reconciles the *guest-cluster* RBAC the management-side syncer needs to write Namespace labels. Because the syncer is retired unconditionally, HCP's change is a straight removal. Whether the guest-cluster RBAC goes too is a HyperShift owner's call: it grants a management-cluster controller write access to guest Namespaces, so leaving it is a standing privilege with no consumer, and removing it is awkward to reverse if MicroShift's outcome differs.
- **The `PodSecurityViolation` alert already ships in HCP**, embedded in the HCCO resources and reconciled unconditionally — a copy of the standalone asset that has already drifted from its source. Any change to it has to be made twice; this enhancement retains it unchanged in both.
- HCCO opts `kube-system` out of label syncing in the guest cluster, which becomes inert. The management side additionally carries a `PodSecurityAdmissionLabelOverrideAnnotation` and a `restricted-psa` image label handled in `hypershift-operator/controllers/hostedcluster/hostedcluster_controller.go`; both interact with the enforcement level and need reconciling with the new API.

Two decisions remain open — the configuration surface ([Open Questions](#hypershift-configuration-surface)) and behaviour while management and guest sides are on different releases ([Version Skew Strategy](#version-skew-strategy)). **A HyperShift reviewer is required before this enhancement is marked implementable.**

#### Standalone Clusters

Standalone is the primary topology; everything outside this section is written against it. Disconnected and bare-metal clusters are unaffected: no component involved reaches outside the cluster, and the only egress-adjacent surface — the `pod_security_enforcement_mode` telemetry metric — degrades to not being reported like any other.

#### Single-node Deployments or MicroShift

**Single Node OpenShift.** Every change to the effective enforcement level re-renders the kube-apiserver configuration, cutting a new static pod revision. With one kube-apiserver that rollout is a short API outage rather than a rolling update — including the rollout that *disables* enforcement, which is precisely the path an administrator reaches for when something is already wrong. Two things follow: the rollout cost has to be stated in the break-glass procedure, and the Config Observer's inputs must be debounced so a flapping status field cannot cut revisions in a loop. The observer's input set is small for exactly this reason (see [PodSecurity Configuration](#podsecurity-configuration)).

The new recurring cost on SNO is the readiness controller's sweep: one Namespace LIST plus a per-Namespace Pod LIST and the workload-template LISTs added by [Scope of the evaluation](#scope-of-the-evaluation), every four hours through a client throttled to QPS=2 / Burst=2. Peak memory and CPU still need quantifying; see [Open Questions](#evaluation-cost-and-pagination).

**MicroShift** is affected and cannot be waved off. It hardcodes `enforce: restricted` in `assets/controllers/kube-apiserver/defaultconfig.yaml`; it vendors and runs `cluster-policy-controller` with `"*"` controllers and no feature gates, so it falls through to `NewEnforcingPodSecurityAdmissionLabelSynchronizationController` and runs the syncer in enforcing mode today; it ships the syncer's RBAC and documents the behaviour in `docs/user/howto_pod_security.md`; and it has no CVO, no `FeatureGate` CR and no installer-rendered `config.openshift.io/v1 APIServer` singleton, so none of this enhancement's configuration surface reaches it. Moving the API onto an existing config resource does not change this: MicroShift's problem is that it serves no such resource, not that a CRD would have been one more thing to ship.

MicroShift is therefore the one topology where the syncer is unambiguously load-bearing: with `enforce: restricted` hardcoded, the computed per-Namespace labels are the only thing keeping non-compliant Namespaces admitting workloads. Three outcomes are possible and the choice is MicroShift's:

- **Follow OpenShift**: the global default moves to `privileged` and the syncer goes. Consistent, but a posture change for an edge product whose users did not ask for one, without the opt-back-in that standalone gets.
- **Keep `restricted` and keep the syncer.** Then the syncer is retired only in OpenShift and HCP, and `cluster-policy-controller` must keep the code alive and tested for a single consumer — the deciding input for [Is the syncer deleted or merely never started](#is-the-syncer-deleted-or-merely-never-started).
- **Keep `restricted` and drop the syncer**, requiring every MicroShift user to label their own Namespaces. A breaking change needing its own deprecation.

A MicroShift reviewer is required, and this enhancement should not merge as `implementable` with the question open, because the second outcome changes what "retired" means everywhere else in this document.

#### OpenShift Kubernetes Engine

The enablement lever works on OKE: SCCs and the PodSecurity plugin are core components in both products, and `config.openshift.io/v1 APIServer` is served by the same kube-apiserver.

The safety net does not. Both alerts in [Metrics and alerts](#metrics-and-alerts) depend on cluster monitoring, which is excluded from the OKE product offering. An OKE administrator who enables `Restricted` gets the enforcement without the staleness or blocked-enforcement signal, leaving `status.conditions` as the only feedback channel — pull-based, and therefore only seen by someone already looking. Whether enabling `Baseline` or `Restricted` on OKE is supported without that alerting needs an answer before GA, because it determines what the documentation may recommend.
### Implementation Details/Notes/Constraints

The changes, by component:

| Component | Change |
|---|---|
| `openshift/api` | `podSecurityAdmission` on `APIServerSpec` and on `APIServerStatus` in `config/v1/types_apiserver.go`, both gated on a new `PodSecurityAdmissionConfiguration`; `OpenShiftPodSecurityAdmission` removed from `Default` |
| `cluster-kube-apiserver-operator` — RBAC | `update` and `patch` on `apiservers/status` in `config.openshift.io`. The operator reads this resource today but has never written its status |
| `cluster-kube-apiserver-operator` — Config Observer | derive the kube-apiserver `PodSecurity` `enforce` key from `status.enforcementMode`; add a `Baseline` branch |
| `cluster-kube-apiserver-operator` — `PodSecurityReadinessController` | own `status.podSecurityAdmission` on `apiserver/cluster`; drop its two syncer-derived inputs; drop the `disabledSyncer` bucket; export metrics |
| `cluster-policy-controller` | stop starting the PSA label syncer, unconditionally |
| `apiserver-library-go` | use the vendored `securityv1.ValidatedSCCSubjectTypeAnnotation` constant instead of a string literal at [`admission.go#L402`](https://github.com/openshift/apiserver-library-go/blob/42e5e402ca430b5072f9d6fd5df275aff9eaf5ce/pkg/securitycontextconstraints/sccadmission/admission.go#L402) |

Two things it is worth being explicit about, because both are easy to misread:

- **The syncer is not wired to the new API at all.** `cluster-policy-controller` gains no watch, no client and no dependency on `apiserver/cluster`. This is a removal from the startup path, not a new conditional.
- **The SCC-to-PSS computation is retired with the syncer, not relocated.** See [The minimally sufficient standard retires with it](#the-minimally-sufficient-standard-retires-with-it).

#### Already shipped: SCC provenance annotations

Two annotations, both defined in [`openshift/api`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/security/v1/consts.go#L9-L20), already exist and are **not new work**. They are described here because the rest of this enhancement builds on one and disposes of the other.

**`security.openshift.io/validated-scc-subject-type` — retained.** The `openshift.io/scc` annotation says which SCC admitted a workload but not *how* it was granted. This one records that distinction, which matters because the syncer only ever considered SCCs reachable by a Namespace's ServiceAccounts and ignored SCCs granted through the user's own roles — one of the two root causes behind most violating Namespaces found by the [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md). It is set on the Pod by the [SCC admission plugin](https://github.com/openshift/apiserver-library-go/blob/42e5e402ca430b5072f9d6fd5df275aff9eaf5ce/pkg/securitycontextconstraints/sccadmission/admission.go#L400-L403) on the same path that sets `openshift.io/scc`, with values `serviceaccount`, `user` or `none`. Because it is applied at admission time it is only present on Pods created after the release that introduced it; see [Open Questions](#annotation-coverage-on-upgraded-clusters).

**`security.openshift.io/MinimallySufficientPodSecurityStandard` — does not survive.** Written by the syncer at [`podsecurity_label_sync_controller.go#L375-L380`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L375-L380), it records the minimal PSS that would be enforced if an `enforce` label were set — useful when a user has overridden `warn` and `audit` so neither can be read back. It is written in the same server-side apply as the labels, so it is absent from Namespaces the syncer does not control. It is the one item here that release `n` removes rather than builds on; see [The minimally sufficient standard retires with it](#the-minimally-sufficient-standard-retires-with-it).

#### Role of the `OpenShiftPodSecurityAdmission` feature gate

The gate already exists and today means exactly one thing: *PSA enforcement is on by default*. It is in the `Default` feature set, which is why the config observer emits `enforce: restricted` ([`podsecurityadmission.go#L99-L111`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L99-L111)) and the label syncer runs in enforcing mode ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/cmd/controller/psalabelsyncer.go#L17-L50)) on every cluster.

This enhancement keeps that meaning and removes the gate from `Default`. That removal *is* the mechanism by which enforcement becomes optional. Two consequences are load-bearing:

- **Feature gate membership is a property of the payload, not a cluster setting.** Which gates are on in a feature set is compiled in at build time ([`payload-manifests/featuregates/`](https://github.com/openshift/api/tree/de86ee3bf48122ecb00fde7287aa633642ddc215/payload-manifests/featuregates)); an administrator selects a `FeatureSet`, not individual gates. The only feature set permitting per-gate control is `CustomNoUpgrade`, documented as unsupported, irreversible and upgrade-blocking. No supported administrator workflow toggles this gate, so no part of this design may depend on one.
- **The opt-in machinery must not sit behind this gate.** The `podSecurityAdmission` fields and the observer wiring that reads `status` are guarded by their own gate, `PodSecurityAdmissionConfiguration`, on their own schedule. Guarding them with `OpenShiftPodSecurityAdmission` would remove the only means of turning enforcement back on at the same moment it removes the enforcement.

The three levers are distinct:

| Lever | Controlled by | Audience | Effect |
|---|---|---|---|
| `OpenShiftPodSecurityAdmission` in `Default` | Red Hat, at build time | fleet-wide, per release | whether PSA enforcement is the default posture |
| `PodSecurityAdmissionConfiguration` in `Default` | Red Hat, at build time | fleet-wide, per release | whether the `podSecurityAdmission` fields exist in the `APIServer` schema and are honored |
| `spec.enforcementMode` | cluster administrator | one cluster | the enforcement level that cluster actually runs |

#### PodSecurityReadinessController

The [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) owns `apiserver/cluster`'s `status.podSecurityAdmission` and never writes `spec`. It writes that subtree and nothing else on the resource, through the `status` subresource, so it cannot disturb a field it does not own. The question it asks changes with this enhancement: it was built to answer "what would the label syncer apply here, and would that break anything?" — a readiness check for a mandatory rollout that is now cancelled. From release `n` it answers "if this cluster set `enforcementMode: Restricted`, what would break?"

**Its sweep is not cluster-wide.** [`nonEnforcingSelector`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go#L111-L119) builds `!pod-security.kubernetes.io/enforce` and `sync` lists with it, so any Namespace already carrying an `enforce` label is outside its view. This is easy to misread as a whole-cluster sweep and is not one; the scoping decides what the controller can and cannot be asked to do.

On each evaluation it resolves `spec.enforcementMode` into `status`:

| `spec.enforcementMode` | `status.evaluatedLevel` | Violations at that level | Acknowledged | `status.enforcementMode` | `EnforcementBlocked` |
|---|---|---|---|---|---|
| unset or `Privileged` | `Restricted` | reported, but advisory only | n/a | `Privileged` | `False` |
| `Baseline` | `Baseline` | no | n/a | `Baseline` | `False` |
| `Baseline` | `Baseline` | yes | no | `Privileged` | `True`, reason `ViolatingNamespaces` |
| `Baseline` | `Baseline` | yes | yes | `Baseline` | `False`, reason `ViolationsAcknowledged` |
| `Restricted` | `Restricted` | no | n/a | `Restricted` | `False` |
| `Restricted` | `Restricted` | yes | no | `Privileged` | `True`, reason `ViolatingNamespaces` |
| `Restricted` | `Restricted` | yes | yes | `Restricted` | `False`, reason `ViolationsAcknowledged` |

**`Baseline` is blocked only by Namespaces that cannot meet `baseline`**, not by the `restricted` sweep that always runs. A cluster clean at `baseline` but not at `restricted` can opt in to `Baseline`, which is the case the intermediate level exists for; see [Changes to the evaluation](#changes-to-the-evaluation). On a `Privileged` cluster `violatingNamespaces` is still populated against `restricted` and blocks nothing — it is the pre-flight list an administrator reads before deciding.

Lowering enforcement is never blocked: a change to `Privileged` is applied unconditionally, so the escape hatch cannot be gated by an evaluation that is failing or stale. "Acknowledged" means `spec.podSecurityAdmission.acknowledgeKnownViolations` equals the current `status.podSecurityAdmission.knownViolationsHash`, so a later sweep that finds a different set of Namespaces blocks again rather than being covered by an earlier acknowledgement, while one that rediscovers the same set does not. `status.observedGeneration` and `status.lastEvaluationTime` distinguish "evaluated against my current request and clean" from "status predates my change" — which matters because a user who resolves violations does **not** re-state their `spec`; the next evaluation clears the condition and `status.enforcementMode` advances on its own. Raising `spec` while violations are already recorded returns an advisory `Warning` header on the update rather than rejecting it, so the user learns at the moment they act rather than by polling.

##### Freshness is an interlock, not a hint

An empty `violatingNamespaces` list is produced both by a cluster with no violations and by a controller that has never successfully run. These must not be indistinguishable: a controller that is crash-looping — RBAC lost after an upgrade, OOM on a cluster with very many Namespaces, wedged leader election — would otherwise present as a clean bill of health, and a user would raise enforcement while `oc get co` reports everything healthy. Therefore:

- The Config Observer raises the enforcement level only when `Evaluated` is `True`. A non-empty `status.enforcementMode` is not by itself sufficient.
- The controller sets `StatusStale=True` when `now - status.lastEvaluationTime` exceeds twice the evaluation interval, and the observer treats stale the same as unevaluated. It does **not** lower enforcement in response to staleness: dropping enforcement because a monitoring component is unhealthy would be a worse failure than leaving it in place.
- Repeated evaluation failure is propagated to the `kube-apiserver` ClusterOperator as `Degraded=True` with reason `PodSecurityReadinessEvaluationFailing`, so it is visible to `oc get co`, must-gather and fleet monitoring, per [`CONVENTIONS.md`](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md).

##### Scope of the evaluation

PSA is a validating admission plugin. It runs only when something attempts to **create** a Pod; it never re-evaluates running Pods, and raising the enforcement level never evicts a workload. A sweep that inspects only the Pods existing at that moment is therefore a point-in-time snapshot, not a guarantee:

- a Deployment scaled to zero, a CronJob that has not fired, or a DaemonSet whose nodes are cordoned — no Pods exist to inspect;
- **an already-running violating Pod that is recreated** by a node drain, reboot, upgrade, eviction or crash-loop restart. Recreation is a new Pod creation, admitted against the current level for the first time, so a routine MachineConfig rollout weeks later can take down a workload the evaluation reported as clean;
- **any user Namespace created after the sweep**, on a cluster that has opted in. With the syncer retired nothing computes an `enforce` label for it, so it is held to the global standard from the moment it exists.

Two in-scope mechanisms close part of the gap. **Workload templates are evaluated alongside live Pods** — `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`, `ReplicationController` and `DeploymentConfig` each describe the Pod that will be created, so the same check runs against `spec.template` whether or not a Pod exists today, covering the first two bullets. And **`warn` and `audit` stay at the target standard while `enforce` is `privileged`**, so every creation that *would* be rejected is surfaced continuously without being blocked.

Neither covers the third bullet: both operate before enforcement is raised, and once a cluster is at `Restricted` a Namespace created afterwards is enforced immediately, with the next sweep reporting it only after the fact. The syncer used to cover this by labelling new Namespaces as they appeared; nothing does now. It is upstream Kubernetes' behaviour, but not what OpenShift administrators have experienced, and it is the residual risk of opting in. Payload Namespaces are exempt — they carry PSA labels from their own CVO manifests, verified in CI (see [Test Plan](#test-plan)) — with OLM the known exception, handled under [`openshift-operators` falls through](#openshift-operators-falls-through).

##### Changes to the evaluation

`determineEnforceLabelForNamespace` currently picks the dry-run level through a three-step precedence: the `MinimallySufficientPodSecurityStandard` annotation, the most restrictive syncer-owned `warn`/`audit` label, then `restricted` as a fallback. The first two are syncer inputs and go with it, so the function collapses to a constant and disappears, taking with it the `ExtractNamespace(ns, syncerControllerName)` call that existed only to build the syncer-owned view those steps read. What remains: dry-run apply `enforce: restricted` to every Namespace with no `enforce` label, report it violating if the apiserver warns. Classification by `validated-scc-subject-type` is unaffected.

**The sweep is pinned at `restricted`; the blocking decision is not.** These are two different questions, and conflating them makes `Baseline` unreachable — a cluster could only opt in to it after becoming clean at `restricted`, by which point it has no reason to.

Every sweep dry-runs every in-scope Namespace at `restricted`, on every cluster in every mode. Evaluating only against the configured level would mean a `Privileged` cluster trivially finds nothing, and since `Privileged` is the default from release `n`, the fleet signal would go dark across almost the whole installed base at the point it becomes most useful. That pinned pass is what `pod_security_readiness_violating_namespaces` reports, and it keeps *could this cluster opt in?* answerable everywhere.

What **blocks** enforcement is evaluated against the level the administrator actually asked for, recorded in `status.evaluatedLevel`. When that level is `baseline`, the controller re-runs the dry-run against **only the Namespaces that failed the `restricted` pass**. The Pod Security Standards are cumulative, so a Namespace admitted under `restricted` is admitted under `baseline` and needs no second look; that nesting is the assumption the optimisation rests on and is worth confirming in review. `Restricted` and `Privileged` add no second pass at all.

**The second pass reports pass or fail, never a recommended level.** Returning "this Namespace's best level is `baseline`" would be the retired SCC-to-PSS computation rebuilt per Namespace in a second repository — see [The minimally sufficient standard retires with it](#the-minimally-sufficient-standard-retires-with-it). The question asked is closed — *can this Namespace meet the requested standard?*, answered by observing workloads — not the syncer's open one, *what standard does this Namespace need?*, inferred from SCC grants. That open question stays an administrator action via `oc label --dry-run=server`.

The added cost is one extra dry-run per already-failing Namespace, only when `Baseline` is requested, through the same throttled client. It is bounded by the set the administrator is being asked to fix, but it is a second input to [Evaluation cost and pagination](#evaluation-cost-and-pagination).

Two consequences need warning ahead of the release, or both read as regressions. **Violation counts will rise**: a Namespace annotated `baseline` is dry-run against `baseline` today and against `restricted` from release `n`, and Namespaces annotated `privileged` move from almost never violating to almost always. Nothing about them changed — the question did. **The `disabledSyncer` classification is dropped**, because `security.openshift.io/scc.podSecurityLabelSync: "false"` only ever meant "exclude this Namespace from the syncer". That removes the `PodSecurityDisabledSyncerEvaluationConditionsDetected` condition type, which is emitted on every sync whether or not it has members, leaving five of six condition types on the `kube-apiserver` ClusterOperator — a change to a public status surface. Whoever owns the `ClusterFleetEvaluation` dashboards and the Insights rules keyed on these conditions needs both.

**Coverage is asymmetric between fresh and upgraded clusters, permanently.** The controller's population is Namespaces with no `enforce` label: on a fresh install at release `n` that is every Namespace, but on an upgraded cluster it is a small remainder — `openshift-operators`, the run-level-zero Namespaces, whatever the syncer skipped, and anything created since — because retained labels exclude the rest and no cleanup ever returns them.

##### Metrics and alerts

The controller exports no metrics today. It gains four:

| Metric | Type | Purpose |
|---|---|---|
| `pod_security_readiness_last_evaluation_timestamp_seconds` | gauge | drives the staleness alert; the basis for trusting an empty violation list |
| `pod_security_readiness_evaluation_errors_total` | counter | detects a controller that is running but failing |
| `pod_security_readiness_violating_namespaces` | gauge, labelled by `reason` | how many Namespaces fail the pinned `restricted` pass, and why — reported in every mode, so the fleet signal does not go dark when a cluster opts out |
| `pod_security_enforcement_mode` | gauge, labelled by `mode` | the posture actually in force |

`pod_security_enforcement_mode` is included in telemetry so support and fleet dashboards can answer "is this cluster enforcing, and at what level?" without a live debugging session. The others are cluster-local. Note that `pod_security_readiness_violating_namespaces` and `status.violatingNamespaces` can legitimately differ on a cluster requesting `Baseline`: the metric counts `restricted` failures, the status list counts `baseline` failures, and the first is a superset of the second.

Two alerts ship, each `Warning` severity and each therefore requiring a runbook under [`openshift/runbooks`](https://github.com/openshift/runbooks) before merge:

| Alert | Fires when | `for` |
|---|---|---|
| `PodSecurityReadinessEvaluationStale` | `time() - pod_security_readiness_last_evaluation_timestamp_seconds` exceeds twice the evaluation interval | 1h |
| `PodSecurityEnforcementBlocked` | `EnforcementBlocked` has been `True` continuously — the requested mode is not in force and nobody has resolved or acknowledged the violations | 24h |

The long `for` durations are deliberate: neither represents an outage in progress.

The existing [`PodSecurityViolation`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/alerts/podsecurity-violations.yaml) alert is **retained unchanged**. It fires on `pod_security_evaluations_total{decision="deny",mode="audit"}`, and because `audit` stays pinned to `restricted` regardless of enforcement level, its signal is unaffected. It continues to report workloads that would be denied under `restricted` — exactly the pre-flight evidence an administrator needs before opting in — and clusters that never adopt the new API see no behaviour change.

No alert fires for a retained label that has become stricter than its Namespace needs; see [Nothing removes them, and nothing reports them](#nothing-removes-them-and-nothing-reports-them).

#### PodSecurity Configuration

A Config Observer in `cluster-kube-apiserver-operator` manages the kube-apiserver's global PodSecurity configuration.

**There is no "unset" option.** [`defaultconfig.yaml#L12-L22`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/config/defaultconfig.yaml#L12-L22) hardcodes all six keys as `invalid-to-force-substitution`, so a kube-apiserver whose PodSecurity block was never substituted fails to start. [`observePodSecurityAdmissionEnforcement`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/auth/podsecurityadmission.go) accordingly has exactly two branches today — `privileged` and `restricted` — and never returns a state in which the path is absent. The observer may change *which* level it writes; it may never decline to write one. "Disabling PSA" means substituting `privileged`, not omitting the configuration.

Its inputs are:

- `status.enforcementMode` — the administrator's request, after the readiness controller has resolved it;
- the `OpenShiftPodSecurityAdmission` feature gate — used only to pick the level substituted when `status.enforcementMode` is `Privileged`, empty or unavailable. The fallback is `restricted` while the gate is in `Default` and `privileged` once it leaves in release `n`. This exists only to preserve today's behaviour across the transition;
- the `Evaluated` and `StatusStale` conditions.

That is the whole input set. The observer does not list Namespaces and has no input that varies with ordinary cluster activity — which is what makes debouncing tractable on SNO.

It substitutes a level above the fallback only when `status.enforcementMode` is `Baseline` or `Restricted` **and** `Evaluated=True` and `StatusStale=False`. Substituting `privileged` is subject to neither condition: lowering enforcement must work when the readiness controller is broken, because that is precisely when an administrator needs it.

This enhancement changes only the `enforce` key. Both existing branches pin `audit` and `warn` to `restricted` regardless of enforcement level ([`#L32-L39`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L32-L39), [`#L50-L57`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L50-L57)) and that is retained deliberately: it is what keeps violations observable while `enforce` is `privileged`. The `*-version` keys are unchanged. `Baseline` has no implementation today, so a third branch must be added.

#### Retiring the PSA label syncer

The [PSA label syncer](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) is **retired**. It does not run in any enforcement mode, it is not kept running in a reduced advising or annotation-only mode, and it is not restarted when an administrator opts in. There is no `status.enforcementMode` value that starts it and no cluster configuration that brings it back.

| `status.enforcementMode` | Syncer | Namespace labels |
|---|---|---|
| any value, including unset | **not running** | nothing is written, ever. Labels already present are left exactly as they are, because a controller that never applies never prunes |

Today the choice is made once at construction from the `OpenShiftPodSecurityAdmission` gate ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50)), enforcing by default and advising as the alternative. Both branches go. Whether the code is removed outright or left unreferenced is [an open question](#is-the-syncer-deleted-or-merely-never-started) whose answer depends on MicroShift.

The set it acted on — and therefore the set carrying its labels on any cluster upgrading into release `n` — is defined by `isNSControlled` ([`#L475-L522`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L475-L522)): user Namespaces that have not opted out via `security.openshift.io/scc.podSecurityLabelSync=false`. **OpenShift's own Namespaces are unaffected**, excluded twice over by the hardcoded `nsexemptions` payload list and by the `openshift-` prefix; they carry PSA labels from their own manifests, which the syncer never owned and could neither write nor remove. That definition is now historical — it describes where the retained labels came from, not a set any running controller acts on.

##### Why it is retired outright

The obvious alternative is to tie the syncer to the enforcement mode: off at `Privileged`, on at `Baseline` or `Restricted`. That is rejected. One reason applies to the opt-out and three to the opt-in.

**At `Privileged` the syncer has nothing to do.** It exists to make a `restricted` default survivable, by computing a *less* restrictive `enforce` label for Namespaces whose ServiceAccounts cannot meet it. Once the default is `privileged`, an unlabelled Namespace is admitted regardless and the computed label protects against nothing — per-Namespace metadata no admission decision consumes, written indefinitely, including on Namespaces whose administrator explicitly opted out.

**At `Baseline` or `Restricted` the syncer contradicts the request.** An administrator who sets `Restricted` is asking for unlabelled Namespaces to be held to `restricted`. The syncer would immediately grant many of them a *lower* standard, computed from their SCCs, without telling anyone: the cluster would report `status.enforcementMode: Restricted` while running a patchwork neither the administrator chose nor the API describes. Under the old mandatory model that was the point — damage control for a default nobody opted into. Under an opt-in model it silently weakens the thing that was opted into.

**The inference it relies on is the known-unreliable part of the system.** The [Motivation](#motivation) exists because deriving a standard from the SCCs available to a Namespace's ServiceAccounts is wrong often enough to matter: it ignores user-based SCCs and is defeated by user-overridden labels, the two root causes behind most violating Namespaces the fleet evaluation finds. Keeping the syncer would keep that inference in the *enforcement* path, where being wrong means either a rejected workload or a Namespace quietly running below the requested standard. A direct dry-run observes what a Namespace's workloads actually do rather than predicting it.

**A controller that starts and stops is harder to reason about than one that does not exist.** A mode-switched syncer must handle being started on a cluster it has never seen, catching up across every Namespace before enforcement is safe to raise, and being stopped mid-apply — cross-operator sequencing between two operators reconciling the same inputs on independent timing, where raising the level before the labels land rejects workloads. Retiring the syncer deletes that problem rather than specifying it; no labelling handshake between `cluster-policy-controller` and `cluster-kube-apiserver-operator` is needed.

**Advising mode is neither freezing nor removing**, though it looks like the safe middle ground. What becomes of the labels there is decided by server-side apply, not by cleanup code: the syncer rebuilds its apply configuration each sync from `syncedLabels`, which in advising mode holds `warn` and `audit` only, and SSA prunes fields a manager no longer sends — so the `enforce` label *is* dropped, but only on the next apply, and `shouldUpdate` skips the apply unless a tracked value has drifted. A Namespace already correct keeps its label indefinitely; one that sees an unrelated SCC change six months later loses it at that moment. The posture decays non-deterministically and identically configured clusters diverge on unrelated churn. The same mechanics run in reverse where opting in *restarts* the syncer: a Namespace frozen at `restricted` would be relaxed to whatever it now computed, so raising the global level could lower several Namespaces in the same act. A controller that never applies never prunes, so retention is a property of not running rather than a behaviour to test against SSA's ownership semantics.

##### What retiring it costs

Three things the syncer produced stop being produced.

- **No Namespace gets a computed `enforce` label, on any cluster, ever again.** The global level applies directly to every Namespace without a label of its own — including Namespaces created after the opt-in, which nothing labels and which the last evaluation could not have seen. Labelling is now the Namespace owner's responsibility: for payload Namespaces that is the component team, the label travels in the CVO manifest, and CI enforces it; for user Namespaces it is the administrator, with no equivalent check. This is upstream Kubernetes' behaviour and it is defensible, but it makes opting in a sharper action than it was.
- **`MinimallySufficientPodSecurityStandard` stops being maintained and is not replaced.** See below.
- **No cluster can be told that a retained `enforce` label is now stricter than the Namespace needs**, because that judgement requires the computation above. See [Nothing removes them, and nothing reports them](#nothing-removes-them-and-nothing-reports-them).

Per-Namespace `warn` and `audit` labels also stop being written, but nothing depends on them: the global configuration pins both to `restricted` regardless of enforcement level, so violations remain observable cluster-wide without any per-Namespace label.

##### The minimally sufficient standard retires with it

`security.openshift.io/MinimallySufficientPodSecurityStandard` records what the syncer *would* apply to a Namespace, derived by mapping the SCCs reachable by its ServiceAccounts onto the weakest PSS level that admits them. It is the syncer's working answer, cached on the object; with no syncer there is no such decision pending and nothing to cache.

Relocating the computation into the `PodSecurityReadinessController` is the natural counter-proposal and costs more than it looks. That controller does not sweep every Namespace: [`nonEnforcingSelector`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go#L111-L119) restricts it to Namespaces carrying no `enforce` label, so every Namespace the relocation would serve — those holding retained syncer labels — is excluded by construction. It would mean reimplementing the SCC-to-PSS mapping in a second repository *and* widening the sweep to the labelled population, which on an upgraded cluster is nearly all of it.

What the annotation served, and what serves it from release `n`:

| What it served | What serves it now |
|---|---|
| the readiness controller's choice of dry-run level ([`violation.go#L55`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/violation.go#L55)), its **only** non-test consumer product-wide | evaluation against `restricted`, unconditionally — see [Changes to the evaluation](#changes-to-the-evaluation) |
| an administrator asking what level a Namespace can meet | `oc label --dry-run=server`, per [Resolving violating Namespaces](#resolving-violating-namespaces) |
| support reading a Namespace to see what the syncer had computed | the stale annotation itself, left in place |

Existing annotations are **not** stripped, for the same reason the labels are not: they record what the cluster computed before release `n`, and deleting them destroys evidence to no benefit. Once the readiness controller stops reading them they are neither written nor read, which is what makes leaving them safe — an annotation nothing consumes cannot mislead a running component, only a human, and a human reading it has `managedFields` and the release note to date it.

#### Retained labels on upgraded clusters

Every cluster running today has `OpenShiftPodSecurityAdmission` in its `Default` feature set and runs the syncer's **enforcing** constructor, so `pod-security.kubernetes.io/enforce` labels are already present across the fleet, applied via server-side apply under the field manager `pod-security-admission-label-synchronization-controller` ([`#L386`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L386)).

Retiring the syncer stops it writing new labels. It does not remove existing ones, no code path in any component removes them, and because no enforcement mode restarts the syncer, nothing ever reconciles them again. **They are frozen at whatever value the last pre-upgrade sync left.** Release `n` does not introduce this label — it stops writing one that is written today.

Retention is deliberate, for three reasons:

- **Optional is not the same as off.** Stripping every syncer-written label on upgrade would replace "every cluster must enforce" with "every cluster must stop enforcing". Optionality means the cluster's existing state persists until an administrator chooses otherwise.
- **One direction is reversible and the other is not.** Retention can be undone by hand. Stripping cannot: server-side apply deletes the value and archives nothing, and returning to `Restricted` would recompute from current SCC and RBAC state rather than restore what was there.
- **The exposure is compliance, not availability.** Relaxing enforcement only admits more workloads; it breaks nothing. The risk is a security control disappearing without announcement from clusters that may be attesting to it under FedRAMP, PCI or DISA STIG, and being discovered long afterwards. The labels plus their `managedFields` entries are that attestation's evidence.

Two consequences follow, and both cut against the feature's purpose.

**The API is inert on upgraded clusters.** Per-Namespace labels take precedence over the global default, and the config observer only sets the global default. On a cluster where essentially every managed Namespace carries a syncer-written `enforce` label, setting `enforcementMode: Privileged` changes the effective level of *nothing that already has a label*. An upgraded cluster is opt-in for *new* Namespaces and unchanged for existing ones until somebody removes those labels by hand.

**A frozen label can become a fossil.** Take a Namespace labelled `enforce: restricted` before the upgrade, whose administrator then grants its ServiceAccount the `anyuid` SCC to run a workload needing a fixed UID. Before release `n` the syncer would have relaxed the label to `baseline`; now it stays pinned at `restricted` and the workload is rejected at admission — on a cluster whose administrator never opted into this feature and has no reason to associate the failure with an upgrade. Because the syncer is retired unconditionally, no configuration resolves this: the only fixes are to edit the label by hand or delete it and let the Namespace fall to the global default, both covered in [Removing retained `enforce` labels](#removing-retained-enforce-labels).

##### Nothing removes them, and nothing reports them

**No cleanup ships, and that is a decision rather than a deferral.** Nothing in release `n` or later deletes a retained `enforce` label: not on the transition to `Privileged`, not on a schedule, not at all. A one-shot cleanup triggered by an API field is still a fleet-wide silent reduction in enforcement posture — the exact compliance event the three arguments above exist to avoid — differing from deleting the labels on upgrade only in what triggers it. Removal is an administrator act, performed per Namespace by someone who has decided they want it, supported by [Removing retained `enforce` labels](#removing-retained-enforce-labels).

**No detection ships either.** There is no alert, metric or condition reporting that a retained label has become stricter than its Namespace needs. Detecting that requires the minimally sufficient standard — the computation retired above — and the target population is outside the readiness controller's selector, so it would mean resurrecting the SCC-to-PSS mapping in a second repository *and* widening the sweep to every labelled Namespace, a cost carried by every cluster forever.

This is the sharpest edge of the design, and what it costs is **attribution, not silence**: the workload is rejected with a PSA message naming the control it violated, surfacing on the ReplicaSet or Job as described in [Operational Aspects of API Extensions](#in-general). What the administrator does not get is the connection between that rejection and an upgrade weeks earlier, which is why the [release note](#release-note) is load-bearing here rather than a courtesy. Two things narrow the exposure: the trigger is an SCC or RBAC grant made *after* the upgrade, a deliberate act rather than background drift, so whoever hits the rejection is usually whoever just changed something; and the remedy is per-Namespace and immediate once identified.

##### `openshift-operators` falls through

Retaining labels protects only Namespaces that *have* one. The kube-apiserver's PodSecurity configuration carries no Namespace exemptions at all — [`defaultconfig.yaml`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/bindata/assets/config/defaultconfig.yaml) exempts exactly one username, `system:serviceaccount:openshift-infra:build-controller` — so any Namespace without its own `enforce` label falls through to the global default and is relaxed when that default moves to `privileged`.

`openshift-operators`, the Namespace OLM users install operator bundles into, is exactly that. It is deliberately kept out of the `nsexemptions` list, which carries an `IMPORTANT:` comment stating it must not be exempted, but it is then caught by the `openshift-` prefix skip, so the syncer never labelled it even when it ran. With neither a syncer-written nor a manifest-written label, it is enforced at `restricted` today purely by the global default, and on upgrade to release `n` its effective level silently becomes `privileged`. Conversely, on a cluster opting into `Restricted`, every operator bundle needing more than restricted fails admission — the happy path of this feature is a mass-breakage path for OLM users.

**This is out of scope.** `openshift-operators` is OLM's Namespace and labelling it is an OLM-side change. Retiring the syncer neither causes nor worsens the gap — the prefix skip means the syncer never labelled it — but it removes one candidate mitigation. Adding the Namespace to the PodSecurity exemption list contradicts the `IMPORTANT:` comment; having OLM ship an explicit label on the Namespace it owns is the correct fix. What this enhancement owes the gap is disclosure: the release note states the change in effective level explicitly, and [Monitor tests for managed Namespaces](#monitor-tests-for-managed-namespaces) encodes the exclusion rather than leaving it implicit. **OLM and layered-product reviewers are required to accept the hand-off**, not to resolve it here.

##### Release note

Because no alert and no cleanup ship, the release note is not a summary of the change — it is the primary mitigation for it, and the only place an administrator is told any of this before meeting it in production. At minimum it must carry:

- **The default posture changes.** PSA enforcement is no longer on by default; the cluster-wide default becomes `privileged`. Clusters that take no action are less restrictive than before.
- **Existing `enforce` labels are kept and are now frozen.** They continue to enforce at their current level and no component will ever update them again. `enforcementMode: Privileged` does not remove them, so on an upgraded cluster the new API governs new and unlabelled Namespaces only.
- **The symptom, named as a symptom.** A workload that runs today can be rejected after an administrator grants its ServiceAccount a broader SCC, because the Namespace's frozen label no longer reflects what it is permitted to run. Describe it as what they will see — a rejected Pod, an SCC grant that appears to have had no effect — not as a mechanism.
- **What to do about it**, linking to [Removing retained `enforce` labels](#removing-retained-enforce-labels).
- **`openshift-operators` changes effective level**, from `restricted` to `privileged`.
- **Downgrade re-enables enforcement**, per [On Downgrade](#on-downgrade).

The syncer's retirement is a removal with no supported way back, and must be said plainly rather than framed only as "enforcement is now optional".

#### New Installation

Fresh installs do not enforce PSA by default. If the administrator does not configure PSA at install time, `spec.enforcementMode` is unset, resolving to `Privileged`; nothing else about PSA is switched off. From there the administrator raises `spec.enforcementMode` to the level they are comfortable with, and returns it to `Privileged` to go back.

A fresh install is the only case where retiring the syncer costs nothing: no Namespace carries a syncer-written label, so there are no fossils, the API is not inert, and `Privileged` really is the cluster's effective posture rather than only its default. The trade appears later — once the administrator opts in to `Restricted`, every Namespace their workloads create from then on is `restricted` unless they label it, and nothing is computing labels on their behalf. That is the behaviour to document, because it is what an administrator adopting this on a new cluster meets first.

The `PodSecurityReadinessController` gates nothing on a fresh install, because OpenShift's own workloads and Namespaces are labelled by their own manifests. It becomes load-bearing once customer workloads and Namespaces exist.

### Risks and Mitigations

- **The out-of-the-box security posture is reduced.** Clusters that take no action end up less restrictive than today. *Mitigation:* per-Namespace labels still take precedence, SCCs are unchanged, existing enforce labels are retained, and the release note calls it out. Not fully mitigated by design — it is the trade this enhancement makes, and it **requires explicit security sign-off**.
- **Opting in has no runtime safety net for unlabelled user Namespaces.** `Baseline` or `Restricted` applies directly to every Namespace with no `enforce` label, and nothing computes a gentler standard. *Mitigation:* payload Namespaces get their labels from CVO manifests, verified by the monitor tests in [Test Plan](#test-plan); for user Namespaces, the evaluation must report zero violations before the observer raises the level. *Not mitigated:* user Namespaces created after the evaluation, and OLM's Namespaces.
- **`openshift-operators` has no label from any source** and follows the global default in both directions. **Unmitigated and out of scope**; see [`openshift-operators` falls through](#openshift-operators-falls-through).
- **A retained `enforce` label can reject a workload long after the upgrade, with nothing on the cluster reporting it.** *Mitigation:* the release note and documentation, which are the whole of it since no alert and no cleanup ship. See [Nothing removes them, and nothing reports them](#nothing-removes-them-and-nothing-reports-them).
- **The readiness evaluation is near-blind on upgraded clusters**, because its sweep covers only Namespaces without an `enforce` label and retained labels exclude most of such a cluster. *Not mitigated*, permanently; it bounds how much assurance `status.violatingNamespaces` can offer someone considering opting in.
- **A clean evaluation is not a guarantee.** PSA is validating-admission only, so a workload not running at evaluation time, or recreated later by a drain or upgrade, can fail long after the cluster was reported clean. *Mitigation:* workload templates are evaluated alongside live Pods, and `warn`/`audit` stay pinned to `restricted`. See [Scope of the evaluation](#scope-of-the-evaluation).
- **An empty violation list can mean "no violations" or "never evaluated".** *Mitigation:* the `Evaluated` and `StatusStale` conditions gate any raise, and repeated failure degrades the ClusterOperator. See [Freshness is an interlock, not a hint](#freshness-is-an-interlock-not-a-hint).
- **Every enforcement change rolls the kube-apiserver**, which on SNO is an API outage, including on the disable path. *Mitigation:* lowering is unconditional so it is never blocked, the cost is documented in the break-glass procedure, and the observer's inputs are debounced.
- **The subject-type annotation is absent on pre-existing Pods**, which makes both possible readings wrong. Unmitigated pending [Open Questions](#annotation-coverage-on-upgraded-clusters).

Security review is required from the OpenShift security architecture group, covering the default-posture change specifically rather than the API. UX review is required from the console and docs teams for the `oc` workflow and the wording of the advisory `Warning` header.

### Drawbacks

- **It ships a less secure default than the product has today**, with no mechanism preserving the current default for clusters that take no action. For a product positioned as secure by default, that is a real cost and not only a documentation problem.
- **Opting in is a weaker guarantee than the enforcement it replaces.** Today's syncer puts each Namespace at the strictest standard it can actually meet. `Restricted` means `restricted` everywhere unlabelled — stricter in principle, but only survivable if the cluster is already clean, and offering nothing to one that is mostly clean. There is no per-Namespace middle ground, so administrators with heterogeneous clusters must label Namespaces themselves or stay at `Privileged`.
- **The feature does nothing on an upgraded cluster, and no future release changes that.** Retained labels outrank the global default and no cleanup ships in `n` or later, so the clusters most in need of relief get a manual procedure rather than an API. Those labels also become a new class of permanently unmaintained cluster state — written by a controller that no longer exists, refreshed by no supported action — that support must reason about for as long as those clusters live.
- **OpenShift loses the ability to say what standard a Namespace needs.** The answer is still obtainable one Namespace at a time via `oc label --dry-run=server`, but it moves from a value the platform maintained on every object to something an administrator must go and ask for, and support loses it from must-gather on any cluster installed at release `n` or later.
- **A downgrade can silently erase the administrator's recorded intent.** This is the price of a gated field over a dedicated CRD. `n-1`'s `APIServer` schema has no `podSecurityAdmission`, so the subtree is pruned — at the latest on the next write to `apiserver/cluster` by anyone, for any reason, which may be weeks after the downgrade. A CRD would have persisted intact and inert. Re-upgrading does not restore what was pruned. See [Schema pruning erases the configuration](#schema-pruning-erases-the-configuration).
- **It makes a `config.openshift.io` resource partly operator-owned.** `apiserver/cluster` is a pure intent resource today; from release `n` one subtree of it is written by `cluster-kube-apiserver-operator` on a four-hourly cycle. Anyone diffing the resource to audit configuration drift now sees controller writes mixed in with administrator edits.
- **It bundles three separable changes** — the already-shipped SCC annotation work, retiring the syncer, and the new API. Retiring the syncer has by far the largest blast radius and is not even conditional on the API: it happens on every cluster whatever `spec.enforcementMode` says, so a reviewer who evaluates only the API has not evaluated the change.
- **Applying a configuration change costs a control-plane rollout**, which on SNO is an outage, in both directions.

## Alternatives (Not Implemented)

The alternatives below have been identified but not yet evaluated to a conclusion. **This section must be completed before the EP moves from `provisional` to `implementable`.**

- **A dedicated cluster-scoped `PSAEnforcementConfig` CRD.** This was the earlier shape of the proposal and is the alternative closest to being chosen; the reasons it was not are in [Why this lives on `APIServer`](#why-this-lives-on-apiserver). Its two genuine advantages over the chosen design are that it survives a downgrade intact rather than being pruned, and that it keeps controller writes out of a resource administrators treat as pure intent. Both are recorded as [Drawbacks](#drawbacks) of what was chosen.
- **Spec on `APIServer`, status on `operator.openshift.io/v1 KubeAPIServer`.** Considered because `KubeAPIServer` already carries conditions and already hosts the `PodSecurity*EvaluationConditionsDetected` conditions this controller writes today, so it needs no new status surface at all. Rejected because splitting intent and in-force state across two resources breaks the property the whole design leans on — that one object answers "what did I ask for, and what am I actually running?" — and because conditions on `kubeapiservers.operator.openshift.io` are unioned into the `kube-apiserver` ClusterOperator, which is exactly the leak the [downgrade rules](#per-resource-downgrade-matrix) exist to prevent.
- **Hoisting `conditions` to the root of `APIServerStatus`** rather than nesting under `podSecurityAdmission`. Conventional for a status struct, and rejected only because this feature should not be the one that claims a name the type will want for other purposes, behind a feature gate.
- **Document `unsupportedConfigOverrides` and ship nothing.** Zero API surface, and it is the recovery path release `n` relies on already. Rejected in principle because an unsupported override is not an acceptable long-term answer for a supported posture decision, but the comparison should be written out.
- **Report status through the existing `PodSecurity*EvaluationConditionsDetected` ClusterOperator conditions** instead of a new status struct. Avoids the scale problem in `status.violatingNamespaces` on large clusters, at the cost of not being able to name the specific Namespaces.

## Open Questions

### Rerun the evaluation before enforcing

The last evaluation can be hours old when enforcement is raised. It may be better to run one immediately before applying `Baseline` or `Restricted` rather than relying on `StatusStale`.

### Initial evaluation by the PodSecurityReadinessController

If a cluster upgrades from `n-1` through `n` to `n+1`, the controller may not have time to run once in release `n`. To prevent an unchecked transition on `n+1` it retries until `status.lastEvaluationTime` is set. This overlaps with [EUS-to-EUS skip upgrades](#eus-to-eus-skip-upgrades) and the two should be resolved together.

### Annotation coverage on upgraded clusters

`security.openshift.io/validated-scc-subject-type` is written at admission time, so it is absent on exactly the long-running, pre-existing workloads whose SCC provenance the evaluation most needs. Neither default reading is safe:

- treating absence as `serviceaccount` produces false negatives — the Namespace is reported clean and the workload fails the next time it is recreated;
- treating absence as `user` produces false positives on every Pod on every upgraded cluster, making the feature unusable on any cluster not born with the annotation.

The likely answer is an explicit third state surfaced in the per-Namespace `reason`, plus a coverage signal — the proportion of Pods carrying the annotation — that the Config Observer can gate on. Separately, nothing currently states that the SCC admission plugin overwrites a user-supplied value for this annotation; if it did not, a user able to create Pods could hide a violation from the evaluation.

### How CI lanes get a `restricted` cluster

The expected PSS for OpenShift's own components stays `Restricted`, and monitor and periodic tests depend on that. The syncer was never what provided it — `isNSControlled` skips every `openshift-` prefixed Namespace, so payload Namespaces get their labels from their own CVO manifests. What is undecided is how lanes get a `restricted` cluster now. Opting in sets the global default, but does not bring the syncer with it, so any test Namespace the lanes create themselves (`e2e-test-*` and similar, which are not `openshift-` prefixed and were syncer-managed) inherits `restricted` with nothing computing a gentler label. Whether that breaks existing e2e suites, and whether the fix is per-test labelling or a broader exemption, is the input the [Test Plan](#test-plan) is missing.

### HyperShift configuration surface

Putting the API on `config.openshift.io/v1 APIServer` answers half of this for free. `HostedCluster.spec.configuration.apiServer` is already a `*configv1.APIServerSpec`, so a management-side administrator can set `enforcementMode` on a hosted cluster with no new API, and the existing `ClusterConfiguration` precedent settles RBAC and tenancy for the request the same way it settles them for `audit` and `tlsSecurityProfile`.

The unanswered half is status, and it is now the harder one. `ClusterConfiguration` carries *specs only* — there is no `HostedCluster.status.configuration` for a controller to write back into. So on a hosted cluster there is nowhere to report `evaluatedLevel`, `violatingNamespaces`, `knownViolationsHash` or the conditions, which means the blocking interlock — *the requested mode is applied only once the cluster is evaluated clean* — has no obvious expression in HyperShift at all. Three shapes are possible: write status into the guest cluster's own `apiserver/cluster` (readable by the tenant, invisible to the management side that set the spec); add a status surface to `HostedCluster` (new HyperShift API, correct audience); or drop the interlock in HCP and apply the requested mode unconditionally (simple, and abandons the safety property that justifies the feature). This is the main unresolved dependency and needs a HyperShift API owner.

### Is the syncer deleted or merely never started

This enhancement says the syncer never runs on OpenShift in any mode. It does not say whether the code is removed from `cluster-policy-controller` or left in the tree with nothing constructing it:

- **Deleted.** A controller nobody may enable cannot rot, cannot be re-enabled by a well-meaning patch, and cannot accumulate a second divergent copy of the SCC-to-PSS mapping. It also forecloses the option — there is no path back short of reverting a deletion.
- **Left in place, unstarted.** Keeps a revert cheap through the Tech Preview window and keeps the code available to non-OpenShift consumers of the library. The cost is a large body of unreachable, untested code whose CI signal disappears the moment nothing starts it.

The deciding input is MicroShift, per [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift). Retiring the computation rather than relocating it narrows the question — because nothing on the OpenShift side reimplements the mapping, there is no risk of two divergent copies and no shared-library requirement — but does not close it. **This enhancement should not merge as `implementable` with this question open**, because the answer changes which repositories the implementation touches.

### Evaluation cost and pagination

The evaluation adds a per-Namespace Pod LIST, and [Scope of the evaluation](#scope-of-the-evaluation) adds workload-template LISTs on top. These are unpaginated, with no per-sweep timeout, no backoff and no defined partial-completion behaviour. A truncated `violatingNamespaces` is indistinguishable from "fewer violations found", which is precisely the signal that causes an administrator to enable enforcement. Publication needs to be all-or-nothing or explicitly marked partial, and peak memory and CPU need quantifying for SNO and for a large cluster.

### Day 0 configuration

The Summary and [New Installation](#new-installation) both state enforcement can be requested at install time, but no install-config field, manifest name or bootstrap rendering path is specified, and no installer reviewer is assigned. There is also a bootstrap hazard: a day-1 manifest sets an enforcement level before the readiness controller has ever run, which is the exact situation the `Evaluated` interlock exists to prevent.

## Test Plan

### Switching between PSA modes

Tests must cover, for each transition between `Restricted`, `Baseline` and `Privileged`:

- the kube-apiserver's effective `enforce` level, read from the revisioned `config-<revision>` ConfigMap rather than from `status`, per [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect);
- **that the PSA label syncer never runs**, in any mode including `Baseline` and `Restricted`. This is an invariant the transition tests must not be able to violate. "Not running" is what distinguishes this design from advising mode, so it needs a direct test rather than being inferred from labels not changing;
- that a Namespace's existing `enforce` label is **byte-for-byte unchanged** across every transition, including after an unrelated SCC or RBAC change to that Namespace — the churn case advising mode gets wrong — and including on the transition *back up* to `Restricted`;
- that a Namespace with **no** `enforce` label, created while the cluster was at `Privileged`, is held to the global level as soon as the cluster opts in, with nothing computing a gentler label for it. This is the safety-net loss in [What retiring it costs](#what-retiring-it-costs) and should be asserted deliberately rather than discovered;
- that `warn` and `audit` stay pinned to `restricted` in the global configuration in every mode;
- that the `PodSecurityReadinessController` keeps evaluating while `Privileged` is in force, since "disabled" must not mean the diagnostics stop.

### Monitor tests for managed Namespaces

Retiring the syncer moves the assurance for OpenShift's own Namespaces from a runtime controller to CI. After release `n` nothing on a running cluster would notice a payload Namespace shipping without a label, so the assurance has to be explicit.

- **SCC labelling (exists).** A monitor test already checks that workloads run under the SCC their Namespace's configuration implies. It is unaffected by this enhancement and must not regress once the default posture is `privileged`.
- **Managed Namespaces are labelled (new, required).** A monitor test asserting that every payload Namespace carries a `pod-security.kubernetes.io/enforce` label from its own manifest. A pre-merge check on the payload is the right place for this given that payload labels are static and ship in manifests. **It must be in place before the default changes in release `n`, not at graduation.**

The test cannot cover OLM, which expresses its requirement through SCC configuration rather than Namespace labels; that exclusion has to be encoded in the test rather than left implicit. Neither test says anything about user-created Namespaces, and nothing in CI can.

### Readiness controller after the syncer

The existing unit tests asserting annotation-driven level selection encode behaviour that is being removed, so they are replaced rather than updated:

- a Namespace carrying a stale `MinimallySufficientPodSecurityStandard` annotation of `privileged` or `baseline` is evaluated against `restricted` anyway, proving the annotation is no longer an input;
- the same for a Namespace carrying syncer-owned `warn`/`audit` labels, proving the second fallback is gone;
- the pinned sweep runs at `restricted` with `enforcementMode` set to each of `Privileged`, `Baseline` and `Restricted`, proving the fleet signal does not track the configured mode;
- `status.evaluatedLevel` is `Restricted` when `enforcementMode` is unset or `Privileged`, and `Baseline` when `Baseline` is requested;
- a Namespace that fails `restricted` but passes `baseline` is reported in `status.violatingNamespaces` and blocks when `Restricted` is requested, and is **absent and non-blocking** when `Baseline` is requested — the case that makes the intermediate level usable;
- a Namespace that passes `restricted` is never submitted for a second dry-run, pinning the cumulative-standards optimisation;
- no code path returns a per-Namespace recommended level, only pass or fail against `status.evaluatedLevel`;
- a Namespace carrying an `enforce` label is absent from the results in every mode, pinning the selector's scope so a later change cannot silently widen the sweep;
- `PodSecurityDisabledSyncerEvaluationConditionsDetected` is not present on the `kube-apiserver` ClusterOperator, and a Namespace labelled `security.openshift.io/scc.podSecurityLabelSync: "false"` is classified on its merits.

The expected rise in violation counts should be measured in a pre-merge lane against a cluster with a representative label spread, so the size of the telemetry shift is known before the release rather than inferred from the dashboards afterwards.

### Release transitions

[Switching between PSA modes](#switching-between-psa-modes) covers transitions of `spec.enforcementMode` within one release. The transitions *between* releases are where [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy) locates its sharpest hazards, and they need their own coverage. The two directions are not equally testable, and the plan reflects that rather than pretending otherwise.

**Upgrade `n-1` → `n`, in an upgrade lane.** The invariant is that the upgrade changes the default and nothing else:

- every Namespace carrying a syncer-written `enforce` label before the upgrade carries **the same label value afterwards**, and the `managedFields` entry naming `pod-security-admission-label-synchronization-controller` is intact with an unchanged `time`. Snapshot before, compare after; this is the audit record the whole retention argument rests on, and a silent SSA prune would destroy it without any other test noticing;
- no Namespace *gains* an `enforce` label during or after the upgrade;
- the effective `enforce` level in the revisioned `config-<revision>` ConfigMap is `privileged` once the rollout settles, and `warn`/`audit` are still `restricted`;
- the label syncer is not running in `cluster-policy-controller` after the upgrade;
- the transient in [Operator skew](#operator-skew) is benign in both orderings — the upgrade completes with consistent state whichever of the two operators rolls first.

**Downgrade `n` → `n-1` cannot be covered in CI**, because y-stream downgrade is not a supported OpenShift operation and no lane exists for it. Rather than leave the hazard untested, test the property that makes it safe:

- enumerate every condition type this enhancement's controllers write, and assert that none is written to `kubeapiservers.operator.openshift.io` except the single documented exception — `Degraded` for a repeatedly failing evaluation. This is the assertion that keeps the [per-resource downgrade matrix](#per-resource-downgrade-matrix)'s last row from biting, and it holds without performing a downgrade;
- assert that the conditions this enhancement introduces are written to `status.podSecurityAdmission.conditions` on `apiserver/cluster` and nowhere else, pinning the first of the two rules in [On Downgrade](#on-downgrade);
- assert against the generated CRD manifests directly, which needs no cluster at all: the `Default` manifest for release `n` has no `podSecurityAdmission` under either `spec` or `status`, and the `TechPreviewNoUpgrade` one has both. This is what makes the [pruning](#schema-pruning-erases-the-configuration) behaviour a known quantity rather than a surprise;
- the remaining downgrade behaviour — `n-1`'s syncer reclaiming labels, and the pruning of `podSecurityAdmission` on the first write after the downgrade — is a QE procedure against a deliberately downgraded cluster, not a CI lane, and should be written up as such. The pruning case in particular needs a step that writes an unrelated field afterwards, because the erasure does not happen at downgrade time.

**Status written by a previous payload is not trusted.** Covering the [EUS-to-EUS](#eus-to-eus-skip-upgrades) and [initial evaluation](#initial-evaluation-by-the-podsecurityreadinesscontroller) concerns: an operator starting against an object whose `status` was written by a different release reports `Evaluated=False` until its own sweep completes, and the Config Observer does not raise enforcement on the strength of the inherited `status`. This is unit-testable by seeding a `status` with a stale `lastEvaluationTime` and a populated `enforcementMode`; it does not need a skip-upgrade lane, though one would be better.

### Hardcoded SCC-to-PSS mapping

The syncer maps SCCs to PSS through a hard-coded rule set with the PSA version set to `latest`, which risks drifting from upstream. **This enhancement disposes of the problem for OpenShift** rather than inheriting it: no OpenShift component maps SCCs to PSS levels from release `n` onward, so there is no OpenShift-side copy to drift and no test needed to protect one. The problem survives only where the syncer does — if MicroShift keeps running it, the mapping stays live there under MicroShift's hardcoded `enforce: restricted`, and a test guarding it remains worth writing for MicroShift's benefit. It is not a deliverable of this enhancement.

## Graduation Criteria

> **Draft.** The criteria below are being reworked against `dev-guide/feature-zero-to-hero.md` and currently understate the documented bar — the 14-runs-per-platform requirement is missing, the platform list is incomplete, and the "95% over 7 consecutive days" figure is not the documented window.

Graduation is tiered, because the two halves of this enhancement ship in different releases.

### Tier 1: removing `OpenShiftPodSecurityAdmission` from the `Default` feature set

This is what makes PSA enforcement optional, so it is the first tier. Prerequisites:

- all OpenShift workloads and Namespaces are labelled appropriately with correct SCC pinning, demonstrated by the managed-Namespace labelling monitor test rather than by inspection. This is a hard prerequisite: once the syncer is retired and the default is `privileged`, an unlabelled payload Namespace produces no signal on a running cluster, so CI is the only place it can be caught;
- the existing SCC labelling monitor test does not regress once the default posture is no longer enforcing.

This tier changes only the default. It does not remove the enforcement code paths and does not depend on the `podSecurityAdmission` fields, which are still `TechPreviewNoUpgrade` at this point.

### Tier 2: graduating the opt-in configuration

In release `n+1`, once the default has moved, the opt-in path graduates — the API, the controller status it depends on, and the Config Observer that consumes it. The label syncer is not part of this tier; it is not a consumer of the API and is already gone by tier 1.

- `PodSecurityAdmissionConfiguration` moves from `TechPreviewNoUpgrade` to `Default`, which is the whole of the API change: the fields are on a type that is already `v1`, so **there is no version to promote**, no second version served in parallel, and no conversion to write. Anyone who adopted the fields under TechPreview keeps exactly the objects they had.
- API validation integration tests land alongside the `openshift/api` change.

### Dev Preview -> Tech Preview

This feature does not pass through Dev Preview. The fields are introduced directly behind `PodSecurityAdmissionConfiguration` in `TechPreviewNoUpgrade` in release `n`, because the behaviour they configure already ships and is already exercised across the fleet; what is new is the configuration surface, not the enforcement.

Entering Tech Preview requires:

- the `podSecurityAdmission` fields merged in `openshift/api` with their feature gate, and API validation integration tests alongside them, including that the gated fields are absent from the `Default` CRD manifest and present in the `TechPreviewNoUpgrade` one;
- the `PodSecurityReadinessController` populating `status` end to end, including the `Evaluated`, `StatusStale` and `EnforcementBlocked` conditions, with its syncer-derived inputs removed;
- the Config Observer selecting the enforcement level from `status`, with the absent-field and absent-resource paths exercised;
- the four metrics and two alerts in [Metrics and alerts](#metrics-and-alerts) shipped, with runbooks;
- tests labelled `[OCPFeatureGate:PodSecurityAdmissionConfiguration]` and `[Jira:"auth"]`, running in both the TechPreviewNoUpgrade and Default Prow variants.

### Tech Preview -> GA

- At least five tests present in Sippy, individually trackable with clear success/failure signal.
- Tests run at least 7 times per week, on all supported platforms — AWS, Azure, GCP, vSphere, Bare Metal.
- Tests pass at least 95 percent of the time over 7 consecutive days.
- The above met at least 14 days before branching.
- `spec.enforcementMode` can be set to any of the three values and the feature can be disabled at will with no adverse effects.
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/) covering the opt-in workflow, the diagnostics, the break-glass procedure and the label-removal procedure.
- `/test verify-feature-promotion` passing.

### Removing a deprecated feature

This enhancement removes no deprecated API. It removes a behaviour outright: the label syncer stops running in release `n` and does not run again in any configuration, the `enforce` labels it previously wrote stop being maintained, and `MinimallySufficientPodSecurityStandard` stops being written or read. There is no supported setting under which the old behaviour returns, which is unusual enough that the release note must say it plainly.

The annotation is removed from use without being removed from the API. `securityv1.MinimallySufficientPodSecurityStandard` stays defined in `openshift/api`, because existing clusters carry values written under it and those values are retained; deleting the constant would strand data support still reads. It has no producer and no consumer from release `n`.

Whether the *code* goes with the behaviour is [an open question](#is-the-syncer-deleted-or-merely-never-started). Either way the advising mode is deleted: it has no caller, and removing it also removes the non-deterministic server-side-apply decay described in [Why it is retired outright](#why-it-is-retired-outright).

## Upgrade / Downgrade Strategy

### On Upgrade

#### Release plan

No change is backported to release `n-1`: it contains no consumer of the API. It does *not* follow that a stored value survives a downgrade — because the API is now a gated field rather than a CRD, `n-1`'s schema prunes it. See [On Downgrade](#on-downgrade).

- **Release `n`:**
  - Remove `OpenShiftPodSecurityAdmission` from the `Default` feature set. The config observer's fallback moves from `restricted` to `privileged`.
  - Stop starting the label syncer. A separate change in `cluster-policy-controller`, unconditional — not gated on the feature set, not gated on the API, not reversed by opting in.
  - Ship `spec.podSecurityAdmission` and `status.podSecurityAdmission` on `config.openshift.io/v1 APIServer` behind `PodSecurityAdmissionConfiguration` in `TechPreviewNoUpgrade`.
  - Enable the `PodSecurityReadinessController` to set `status`. It does not write `spec`.
  - Enable the Config Observer to set the global `enforce` level from `status.enforcementMode` when it is `Baseline` or `Restricted`.
- **Release `n+1`:** promote `PodSecurityAdmissionConfiguration` to `Default`. No API version moves, because the fields are already on a `v1` type.

#### The state a cluster actually arrives in

A cluster upgrading from `n-1` into `n` arrives with `apiserver/cluster` already present — the installer rendered it at install time and it has been there ever since — and in the common case with the `podSecurityAdmission` fields absent from its schema, because they ship behind a `TechPreviewNoUpgrade` gate and such a cluster cannot upgrade at all. "The resource exists, the field does not, `spec.enforcementMode` therefore unset" is not an edge case; it is the state every upgraded cluster is in, and the state the design has to be correct in first.

This is where landing on an existing config resource pays. Nothing has to create anything: the CVO reconciles the `APIServer` CRD to the schema its feature set implies, the administrator's existing `apiserver/cluster` object gains a field it does not set, and the readiness controller writes into a status subtree of an object it can already see. There is no "does the singleton exist yet" race for the observer to handle on a standard cluster, and no upgrade path on which an administrator's PSA intent is manufactured for them. Where the resource genuinely is absent — MicroShift — the established pattern for an optional config input is [`ObserveMinimumKubeletVersion`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/node/observe_minimum_kubelet_version.go), which logs `NotFound` as a warning and leaves the observed config alone.

Because `observedConfig` is never empty (see [PodSecurity Configuration](#podsecurity-configuration)), both "the field is not in the schema" and "evaluation not yet complete" must resolve to the gate-implied default, which in release `n` is `privileged`. The sequence is therefore: the kube-apiserver rolls to a revision whose PodSecurity block says `privileged`; `cluster-policy-controller` restarts with the label syncer not running, for its own reason rather than because of the feature set; and nothing further happens until an administrator sets `spec.podSecurityAdmission.enforcementMode`. **There is no window in which enforcement is raised as a side effect of the upgrade.**

The relaxation is also not retroactive: the global default applies only to Namespaces with no `enforce` label, and on an upgraded cluster most Namespaces have one. Those Namespaces keep enforcing at their existing level. See [Retained labels on upgraded clusters](#retained-labels-on-upgraded-clusters).

#### `unsupportedConfigOverrides` already blocks the upgrade

`targetconfigcontroller.manageKubeAPIServerConfig` builds `config.yaml` with `resourcemerge.MergePrunedConfigMap`, layering `defaultconfig.yaml`, the authorization-mode override, `config-overrides.yaml`, `spec.observedConfig` and finally `spec.unsupportedConfigOverrides` — which is merged last and wins. On a cluster carrying the PSA override documented in the [original PSA enhancement](pod-security-admission.md), `spec.podSecurityAdmission.enforcementMode` is therefore silently inert while `status` reports success.

No new condition is needed. library-go's `UnsupportedConfigOverridesController`, already run by `cluster-kube-apiserver-operator`, sets `UnsupportedConfigOverridesUpgradeable=False` with reason `UnsupportedConfigOverridesSet` whenever the field is non-empty, and its message enumerates the overridden leaf paths — so such a cluster already reports `admission.pluginConfig.PodSecurity.configuration.defaults.enforce` among them. What it does not say is that this is why the administrator's requested mode is being ignored; making that connection is a [support procedure](#psa-enforcement-configuration-appears-to-have-no-effect). The readiness controller does not attempt to detect, reconcile or remove the override.

### On Downgrade

Y-stream downgrade is not a supported OpenShift operation, so this describes recovery rather than a guarantee.

Downgrading from `n` to `n-1` **restores mandatory enforcement**, because `n-1` still has `OpenShiftPodSecurityAdmission` in its `Default` feature set and has no controller reading the `podSecurityAdmission` fields. For a cluster that had opted out with `Privileged`, this silently re-enables enforcement on a cluster that was opted out precisely because it could not survive enforcement. This is the dangerous direction and must be called out in the release note. The reverse case is benign: a cluster running `Restricted` on `n` downgrades into a release that enforces `restricted` anyway.

The recovery on `n-1` is `unsupportedConfigOverrides`, and it does work: `n-1`'s observer unconditionally overwrites the whole PodSecurity defaults block with `restricted`, and the override is merged afterwards and wins. The cost is that the cluster is then pinned at `n-1` by `UnsupportedConfigOverridesUpgradeable=False` until the override is removed.

#### Schema pruning erases the configuration

This is new with the decision to put the API on an existing resource, and it is the one respect in which a dedicated CRD would have behaved better. A CRD and its stored objects survive a downgrade untouched, because the CVO does not delete CRDs. A *field* does not: `n-1`'s `APIServer` schema has no `podSecurityAdmission`, and a structural schema with `preserveUnknownFields: false`, which every `config.openshift.io` CRD has, prunes what it does not recognise.

Three things follow and all three need stating in the release note.

- **The administrator's recorded intent is destroyed, not merely ignored.** After the prune, `spec.podSecurityAdmission` is gone. Re-upgrading to `n` does not bring it back: the field returns to the schema empty, and the cluster reads as never having opted in.
- **The timing is not the downgrade.** Pruning happens on write, and nothing on `n-1` writes `apiserver/cluster` as a matter of course. The field may sit in etcd, unserved and unreachable, until some unrelated action — an administrator changing `tlsSecurityProfile`, a GitOps controller re-applying the resource, a storage migration — writes the object and takes the subtree with it. A support engineer looking at the cluster the day after the downgrade and a week after it may see different things.
- **The status subtree goes the same way**, which is mostly a relief: the stale `conditions`, `violatingNamespaces` and `evaluatedLevel` from `n` cannot be read as current on `n-1` because they are not reachable. Unlike the operator-CR conditions below, they are also not unioned into any ClusterOperator, so a stale `Degraded` cannot leak out of this subtree.

The mitigation is documentation, not code. An administrator performing a downgrade should record `spec.podSecurityAdmission` first — `oc get apiserver cluster -o jsonpath='{.spec.podSecurityAdmission}'` — and re-apply it after re-upgrading. Writing code to preserve a field across an unsupported operation would mean either a CVO-managed backup object or an annotation shadowing the field, and both are worse than the disease.

#### Per-resource downgrade matrix

| Resource | State on `n` | What `n-1` does to it | Administrator action |
|---|---|---|---|
| `observedConfig` → `admission.pluginConfig.PodSecurity.configuration.defaults` | `privileged`, or the level from `status.enforcementMode` | unconditionally rewrites the whole six-key block to `restricted` and rolls a new revision | none, unless enforcement must stay off — then apply the override above |
| Namespace `pod-security.kubernetes.io/*` labels | unmaintained but intact in every mode — the syncer neither writes nor prunes | `n-1`'s syncer runs enforcing and re-adds and re-owns all three at the minimally sufficient level on every Namespace it controls | none for syncer-controlled Namespaces; those opted out, or whose labels are owned by another field manager, keep whatever `n` left and must be fixed by hand |
| Namespace `MinimallySufficientPodSecurityStandard` | stale in every mode, read by nothing | `n-1`'s syncer rewrites it | none |
| Pod `openshift.io/scc`, `validated-scc-subject-type` | set by SCC admission at creation time | nothing; per-Pod and immutable after admission | none — inputs to the evaluation, not to enforcement |
| `apiserver/cluster` `spec.podSecurityAdmission` | the administrator's recorded intent | pruned by `n-1`'s schema, on the next write to the object by anyone | record the value before downgrading and re-apply it after re-upgrading; nothing recovers it automatically |
| `apiserver/cluster` `status.podSecurityAdmission` | `status` current | pruned with the spec subtree; unreachable in the meantime | none — and unlike the row below, these conditions are not unioned into any ClusterOperator |
| `kubeapiservers.operator.openshift.io` conditions added by this enhancement | `Evaluated`, `StatusStale`, the readiness conditions | nothing writes or garbage-collects them | see below |

That last row is the one that bites. A condition left behind by a controller that no longer exists is not cleared by anything, and the status controller unions every `*Upgradeable` and `*Degraded` condition into the ClusterOperator. A `Degraded=True` written in the seconds before a downgrade would make the `kube-apiserver` ClusterOperator permanently `Degraded` on `n-1`, with no component able to explain why. Two rules follow, and they constrain the API design:

- Condition types introduced by this enhancement live in `status.podSecurityAdmission.conditions` on `apiserver/cluster` wherever possible, not on `kubeapiservers.operator.openshift.io`. Nothing unions them into a ClusterOperator, and `n-1`'s schema prunes them, so they cannot outlive the release that wrote them in a form anything reads.
- Where a condition must live on the operator CR — `Degraded` for a repeatedly failing evaluation — removing it by hand is a documented support step; see [Clearing conditions left behind by a downgrade](#clearing-conditions-left-behind-by-a-downgrade).

Re-upgrading `n-1` → `n` is the case that needs care in both directions. If the subtree was already pruned, the cluster arrives with no intent and no status, which is safe and correct. If it was *not* yet pruned — because nothing wrote the object while on `n-1` — the field returns to the schema still holding a `status` written by the previous `n`, hours or months stale. That `status` must not be treated as current; `Evaluated` and `lastEvaluationTime` exist for exactly this, and the controller's first action on start is to reassert `Evaluated=False` rather than to trust what it finds.

## Version Skew Strategy

Four skew windows matter. Three are windows in which two components disagree about the effective PSA level; the fourth is a window in which the evaluation never happens.

### Intra-rollout skew

Changing the PodSecurity configuration produces a new static pod revision, and revisions roll one master at a time. For the duration — minutes to tens of minutes — one kube-apiserver enforces the new level while the others enforce the old one, and which one a Pod creation reaches is a load-balancer decision. A Deployment scaling from 0 to 5 can have some replicas admitted and some rejected. This window exists on every change, in both directions.

The two directions are not symmetric, and this enhancement deliberately makes only one safe by construction:

- **Raising** enforcement is gated on `Evaluated` reporting no violating Namespaces, so by the time the rollout starts the expected number of Pods the stricter apiservers would reject is zero. The interlock is not only a pre-flight check for the administrator; it is what makes the mixed window tolerable.
- **Lowering** enforcement, including the break-glass path, is *not* instantaneous. Until the last master has taken the new revision, some fraction of Pod creations is still rejected. Support must be told to wait for the rollout — every `currentRevision` equal to `latestAvailableRevision` — rather than concluding the change did not take; see [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect).

On single-node deployments there is no mixed window, because there is only one kube-apiserver; there is an API outage instead while the static pod restarts.

### Operator skew

Retiring the syncer removes most of this problem. The producer of `status.enforcementMode` and its only consumer are both in `cluster-kube-apiserver-operator`, so they ship and roll together and cannot disagree about the API. `cluster-policy-controller` reads none of it.

What remains is a transient during the upgrade itself. `cluster-policy-controller` runs as a container in the kube-controller-manager static pod, owned by a different operator, with no ordering guarantee — so for part of every upgrade the kube-apiserver has already rolled to `privileged` while an `n-1` `cluster-policy-controller` is still stamping `enforce` labels, or the syncer has already stopped while the kube-apiserver is still enforcing `restricted`. Neither is harmful: the first writes labels that are merely redundant and become the retained labels; the second changes nothing, because Namespaces keep the labels the syncer last wrote.

The enum is the sharper edge, and it applies to out-of-payload and hosted-control-plane consumers. A consumer from an earlier release that encounters an unknown value must **preserve its existing behaviour** — not error, and not treat the field as unset. Treating it as unset would silently relax enforcement; erroring would degrade a ClusterOperator over a value valid on the cluster's own API server. So consumers switch on the values they know and fall through to "make no change to the currently effective level", and the enum is only ever extended, never having a value removed or repurposed. Because the API server validating a write is always at least as new as any consumer, a value can be accepted by validation and be unknown to a consumer; that asymmetry is normal during an upgrade, not a bug to be designed out.

### HyperShift skew

HyperShift has no `cluster-kube-apiserver-operator`; the management-cluster `control-plane-operator` renders the hosted kube-apiserver's configuration directly and carries its own copy of the privileged-versus-restricted decision. The management cluster is required to be at or ahead of the hosted cluster's release, so the skew is one-directional and can span several releases: a `control-plane-operator` at `n+2` may be rendering for a hosted cluster whose payload is at `n`. This means a hosted cluster's opt-in cannot be expressed purely in the guest without the management side honouring it, and the management side is the one that may be arbitrarily newer. Which side owns the configuration surface is [an open question](#hypershift-configuration-surface) and the main unresolved dependency here.

### EUS-to-EUS skip upgrades

`PodSecurityReadinessController` resyncs every four hours (`checkInterval = 240 * time.Minute`) with a throttled client whose sweep can itself take a long time on a cluster with many Namespaces. A 5.0 → 5.1 → 5.2 EUS-to-EUS upgrade passes through 5.1 in well under four hours. The library-go factory does run one sync immediately on start, so the operator restarting into 5.1 kicks off an evaluation — but on a large cluster it may still be in flight when the cluster leaves 5.1.

The interlock is therefore defined over the evaluation, not over the release:

- `Evaluated` is `True` only when a sweep has run to completion, and `lastEvaluationTime` records when. Neither is inherited from a `status` written by a different payload; re-entering a release, or arriving from a skip, starts from `Evaluated=False`.
- Enforcement is never raised on the strength of an evaluation that did not complete. The failure mode of a skipped release is that the cluster arrives at 5.2 still `Privileged` — safe and visible — rather than enforced on the basis of a partial sweep.
- Nothing about the skip re-raises enforcement on its own. The gate is out of `Default` in both 5.1 and 5.2, so the cluster-wide default stays `privileged` across the whole skip.

## Operational Aspects of API Extensions

### In general

- Administrators facing issues on a cluster set to a stricter level can change `spec.enforcementMode` to `Privileged` to halt enforcement.
- ClusterAdmins must ensure that directly created workloads (user-based SCCs) have correct `securityContext` settings. Updating default workload templates can help.
- The evaluation runs once every 4 hours with a throttled client to avoid a denial of service on clusters with many Namespaces, so it can take several hours to identify a violating Namespace.
- **Raising the enforcement level never evicts a running workload.** PSA acts only on Pod creation. The corollary is that a workload can be admitted today and rejected weeks later when a node drain, upgrade or crash-loop restart recreates it — see [Scope of the evaluation](#scope-of-the-evaluation).
- **PSA denials are not visible in `kubectl get pods`.** When a Pod is created by a controller the rejection surfaces on the owning object, because no Pod is ever created. For a Deployment it appears on its ReplicaSet, for a CronJob on its Job:

  ```bash
  kubectl -n $NAMESPACE describe replicaset $NAME
  kubectl -n $NAMESPACE get events --field-selector reason=FailedCreate
  ```

  This is the most common source of confusion when debugging PSA, and support should reach for it first.
- To identify specific problems in a violating Namespace:

  ```bash
  kubectl label --dry-run=server --overwrite ns/$NAMESPACE \
      pod-security.kubernetes.io/enforce=restricted
  ```

  Server-side dry run reports only on Pods that exist at that moment; it says nothing about workloads scaled to zero or not yet created. This command is also the answer to "what standard does this Namespace need?", which the platform no longer answers on its own — the `MinimallySufficientPodSecurityStandard` annotation carried a maintained value until release `n`. It is the same dry-run the `PodSecurityReadinessController` performs internally.

### Health and failure modes of the extension

The `podSecurityAdmission` fields introduce no webhook and no aggregated apiserver, so they add no latency to any request path and cannot make another resource unavailable. Because they extend a resource that every cluster already has, they also add no new watch, no new CRD to reconcile and no new object for the CVO to manage. Their failure modes are those of the controller behind them:

| Failure mode | Signal | Effect |
|---|---|---|
| readiness controller never runs | `Evaluated=False`, reason `NeverRan` | enforcement cannot be raised; existing enforcement unaffected |
| readiness controller stops running | `StatusStale=True`; `PodSecurityReadinessEvaluationStale` | enforcement cannot be raised; existing enforcement deliberately left in place |
| readiness controller fails repeatedly | ClusterOperator `Degraded=True`, reason `PodSecurityReadinessEvaluationFailing` | as above, plus a fleet-visible signal |
| field absent from the schema, or `apiserver/cluster` absent | none | every consumer falls back to the feature-gate-implied default |
| `unsupportedConfigOverrides` set | ClusterOperator `Upgradeable=False`, reason `UnsupportedConfigOverridesSet` | `spec.enforcementMode` is silently inert while `status` reports success; the cluster cannot upgrade until the override is removed |

Scale is bounded by one singleton per cluster, and it is a singleton that already existed, so the extension does not affect general API throughput. The one new pressure is on `apiserver/cluster` itself: the readiness controller writes its status subtree on every sweep, so anything watching that resource — the config observers, and any GitOps controller reconciling it — now wakes on a four-hourly write it did not see before. The per-sweep cost of the evaluation scales with cluster size and is still to be quantified; see [Evaluation cost and pagination](#evaluation-cost-and-pagination).

Escalation goes to the OpenShift Auth team. Failures manifesting as admission rejections in `openshift-*` Namespaces are likely to involve OLM and the owning layered product.

## Support Procedures

The procedures themselves are written as `openshift-docs` modules and
`openshift/runbooks` entries rather than carried here. This section records the
set that has to exist and the constraints the design places on them.

| Procedure | Purpose | Destination |
|---|---|---|
| `PodSecurityReadinessEvaluationStale` | why a stale evaluation blocks raising enforcement but never lowers it | `openshift/runbooks` |
| `PodSecurityEnforcementBlocked` | the three ways out — fix, label, or acknowledge | `openshift/runbooks` |
| PSA enforcement configuration appears to have no effect | six causes, chiefly `unsupportedConfigOverrides` and retained labels | openshift-docs, support KCS |
| Removing retained `enforce` labels | the only mechanism by which a syncer-written label is ever removed | openshift-docs, **linked from the release note** |
| Break glass: disabling enforcement | `Privileged`, its rollout cost, and the operator-wedged fallback | openshift-docs, support KCS |
| Resolving violating Namespaces | per-reason remediation, including `openshift-operators` | openshift-docs |

Both alerts are `Warning` severity, so their runbooks are **required before the
alerts can merge**, at
`https://github.com/openshift/runbooks/blob/master/alerts/cluster-kube-apiserver-operator/<AlertName>.md`.

Four constraints below are design consequences rather than procedure steps, and
are recorded here because the procedures are wrong without them.

### PSA enforcement configuration appears to have no effect

**`status` is not evidence of what is enforced.** `spec.enforcementMode` can be
silently inert: `unsupportedConfigOverrides` is merged last and wins, and any
per-Namespace label outranks the cluster-wide default. Support must read the
effective configuration from the revision each master is actually running, not
from an operator's `status`:

```bash
oc get kubeapiserver/cluster \
    -o jsonpath='{range .status.nodeStatuses[*]}{.nodeName}{"\t"}{.currentRevision}{"\n"}{end}'

oc get cm config-<revision> -n openshift-kube-apiserver -o jsonpath='{.data.config\.yaml}' \
    | jq '.admission.pluginConfig.PodSecurity.configuration.defaults'
```

Despite the key's name the value is JSON, because the merge encodes through
`UnstructuredJSONScheme`. The `Upgradeable=False` / `UnsupportedConfigOverridesSet`
condition already enumerates the overridden leaf paths; what it does not say is
that this is why `spec.podSecurityAdmission` is being ignored.

### Removing retained `enforce` labels

**Removal is ordered, and its provenance check fails unsafe.** Lower the
cluster-wide default and wait for the rollout *before* removing any label: in
between, a Namespace that just lost a `baseline` label is governed by a default
still set to `restricted`, stricter than what it had. Ownership is recorded per
label key in `metadata.managedFields`, and a manager of
`pod-security-admission-label-synchronization-controller` or the historical
`cluster-policy-controller` marks a label as safe to remove. **No recorded owner
is ambiguous, not safe** — etcd restore and some migration paths drop
`managedFields` — and the cost of guessing wrong is silently relaxing a Namespace
somebody intended to constrain. Deletion is irreversible, so on a cluster
attesting under FedRAMP, PCI or DISA STIG each removal is a change to an audited
control.

### Clearing conditions left behind by a downgrade

Condition types introduced in release `n` are not garbage collected on downgrade,
and the status controller unions every `*Degraded` and `*Upgradeable` condition
into the ClusterOperator, so one captured at the moment of downgrade can leave
`kube-apiserver` permanently degraded for a reason no running component can
explain. Removing it is manual:

```bash
oc patch kubeapiserver/cluster --type=json --subresource=status \
    -p '[{"op":"remove","path":"/status/conditions/<index>"}]'
```

### Resolving violating Namespaces

**Granting an SCC does not change what PSA enforces.** With the syncer retired,
fixing a Namespace's ServiceAccount stops the *evaluation* reporting it, but
writes no label — so if the Namespace needs an effective level other than the
cluster-wide one, the `enforce` label must be set by hand. On a cluster that has
opted in, that is the only mechanism.

Two facts the procedures depend on: PSA denials do not appear in `oc get pods`,
because no Pod is created — they surface on the owning ReplicaSet or Job via
`oc -n $NS get events --field-selector reason=FailedCreate`. And the per-Namespace
question "what standard can this meet?", which the platform no longer answers on
its own, is asked with `oc label --dry-run=server --overwrite ns/$NS
pod-security.kubernetes.io/enforce=restricted`.

**Still unwritten**, and required before GA: symptoms and log lines for the
readiness controller not running, crash-looping or failing to evaluate; the exact
PSA denial message; audit-log correlation via the
`pod-security.kubernetes.io/enforce-policy` annotation; and must-gather coverage
for the new CR, the effective admission configuration and the violating-Namespace
list.

## Infrastructure Needed

New periodic CI lanes are needed to satisfy the graduation criteria: the feature has to be exercised in both the `TechPreviewNoUpgrade` and `Default` Prow variants, across the provider, topology, architecture and network variants required by `dev-guide/feature-zero-to-hero.md`, at a frequency that reaches the required number of runs per platform before branch cut. The exact lane list follows from the reworked [Graduation Criteria](#graduation-criteria) and is not yet enumerated.
