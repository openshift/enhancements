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
last-updated: 2025-09-20
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

This enhancement introduces a new API and changes to the relevant controllers to allow users to electively rollout [Pod Security Admission (PSA)](https://kubernetes.io/docs/concepts/security/pod-security-admission/) enforcement [in OpenShift](https://www.redhat.com/en/blog/pod-security-admission-in-openshift-4.11).
Enforcement means that the PodSecurityAdmission plugin enforces the `Restricted` or `Baseline` [Pod Security Standard (PSS)](https://kubernetes.io/docs/concepts/security/pod-security-standards/) globally on Namespaces without any `pod-security.kubernetes.io/enforce` label.

### What "enabling" and "disabling" PSA mean here

The words are used precisely throughout this document and are easy to over-read, so they are defined once, up front:

- **Opting in** means setting `spec.enforcementMode` to `Baseline` or `Restricted`. The kube-apiserver's global `enforce` level becomes that standard, and it applies to every Namespace that carries no `pod-security.kubernetes.io/enforce` label of its own.
- **Opting out** means `spec.enforcementMode: Privileged`, or leaving the field unset. The two are the same state, and it is the shipped default — see [Unset means `Privileged`](#unset-means-privileged).

`spec.enforcementMode` selects exactly one thing: the global `enforce` level. It does not turn any component on or off.

**The PSA label syncer is retired, in every mode.** It does not run at `Privileged`, and it does *not* come back when an administrator opts in to `Baseline` or `Restricted` — it is not a consumer of this API and does not read `status.enforcementMode`. The reasoning is in [Why the syncer is retired outright](#why-the-syncer-is-retired-outright), and the consequence is direct: on a cluster that opts in, a Namespace with no `enforce` label of its own is held to the global standard with nothing computing a gentler one on its behalf. That is what makes the [violation evaluation](#podsecurityreadinesscontroller) the load-bearing safety mechanism of this enhancement rather than a convenience.

"Disabling PSA" means **exactly `enforce: privileged`**. It does not mean switching PSA off. The `PodSecurity` admission plugin stays loaded, `warn` and `audit` stay pinned to `restricted` so violations remain observable, the `PodSecurityReadinessController` keeps evaluating the cluster, and per-Namespace `pod-security.kubernetes.io/*` labels keep taking precedence over the global default.

### This Changes the Default Security Posture

**PSA enforcement is enabled by default on OpenShift today, and this enhancement turns it off by default and makes it opt-in.**

Concretely, the `OpenShiftPodSecurityAdmission` feature gate is currently enabled in the `Default` feature set ([`features.go#L108-L114`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/features/features.go#L108-L114)), which causes:

- the kube-apiserver's global PodSecurity configuration to be set to `enforce: restricted` ([`podsecurityadmission.go#L99-L111`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L99-L111)), and
- the PSA label syncer to run in **enforcing** mode, writing `pod-security.kubernetes.io/enforce` on the Namespaces it manages ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/cmd/controller/psalabelsyncer.go#L17-L50)).

Those two go together. The syncer exists *because* the global default is `restricted`: it is what keeps a Namespace whose ServiceAccounts cannot meet that standard from having its workloads rejected. This enhancement removes the mandatory `restricted` default and retires the syncer along with it — including for clusters that opt back in. See [Why the syncer is retired outright](#why-the-syncer-is-retired-outright).

After this enhancement, the shipped default for customer workloads becomes `Privileged` and enforcement must be explicitly requested by the cluster administrator.
This is a deliberate reduction in the out-of-the-box security posture, traded for the guarantee that no cluster acquires failing workloads without its administrator opting in.
It requires explicit security sign-off, a release note, and the migration handling for existing clusters described in [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy).

The kube-apiserver automatically transitions clusters without Pod Security violations to `Privileged` PSS, while silently keeping clusters with violations in `Restricted`/`Baseline` mode.
Users can enable PSA enforcement in fresh clusters either at cluster install time by configuring install options, or by manually enabling PSA via kube-apiserver CRD.

Note that the feature is aimed at clusters that are created after this feature work is complete. This is to prevent failing workloads from clusters that are not PSA compliant. 

<u>Clusters can enable PSA via an explicit ACK by the user to recognise that non-compliant workloads may fail to be admitted to the cluster.</u>

This enhancement expands the ["PodSecurity admission in OpenShift"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission.md) and ["Pod Security Admission Autolabeling"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission-autolabeling.md) enhancements.

## Motivation

After introducing Pod Security Admission and Autolabeling based on SCCs, some clusters were found to have Namespaces with Pod Security violations.
Over the last few releases, the number of clusters with violating workloads has dropped significantly.
Although these numbers are now quite low, it is essential to avoid any scenario where users end up with failing workloads.

This is primarily the motivation behind allowing newly created clusters to enable PSA Enforcement either at install time or via kube-apiserver CRD. This puts the onus on the user to create PSA compliant workloads from the get-go and avoid potential catastrophic failures such as already existing workloads not being admitted at run-time once the feature is enabled. 

Out of the box, `Privileged` is the global default and the PSA label syncer does not run. The `PodSecurityReadinessController` keeps evaluating the cluster regardless — that is the part that must not be switched off, because with the syncer retired it is the *only* thing that tells the administrator whether opting in is safe. When the administrator requests `Baseline` or `Restricted` and the evaluation finds no violating Namespaces, the requested level is applied to the kube-apiserver's global configuration. Namespaces that cannot meet it are surfaced as violations for the administrator to fix or label by hand, rather than being quietly granted a lower standard.

Any namespaces that were not compliant will be listed in the kube-apiserver API status along with the explicit reason(s) for not workloads being admitted, which should help the user to resolve this.

OpenShift strives to offer the highest security standards, and PSA enforcement is enabled by default today in service of that.
This enhancement trades that default for predictability: rather than enforcing a standard that a minority of clusters cannot meet, it gives administrators an explicit, reversible lever and the diagnostics needed to use it safely.
Clusters that opt in reach the same posture as today; clusters that do not remain at `Privileged` until their administrator chooses otherwise.

The cost of that trade is stated plainly in the Summary: clusters that take no action end up less restrictive than they are today.
The mitigating factors are that per-Namespace `pod-security.kubernetes.io/*` labels continue to take precedence and are unaffected, and that SCCs — OpenShift's primary workload admission control — are unchanged by this enhancement.

### Goals

1. Allow users to configure Pod Security Admission enforcement on their clusters.
2. Prevent clusters from having failing workloads by making this feature elective.
3. Allow users to enable this feature as desired.

### Non-Goals

1. Keeping Pod Security Admission enforcement enabled by default.
   As described in the Summary, enforcement is on by default today; this enhancement deliberately makes it opt-in, and no mechanism is proposed to preserve the current default for clusters that take no action.
2. Providing a detailed list of every Pod Security violation in a namespace.
   The API reports violating Namespaces and a reason per Namespace, not per-Pod violation detail, so that the status object stays bounded on clusters with many Namespaces. Per-Pod detail remains available via the server-side dry-run commands in Support Procedures.
3. Restricting per-Namespace PSA configuration.
   Per-Namespace `pod-security.kubernetes.io/*` labels continue to take precedence over the cluster-wide default, and the `security.openshift.io/scc.podSecurityLabelSync=false` opt-out continues to work. This enhancement only changes the global default applied to Namespaces that carry no `pod-security.kubernetes.io/enforce` label.

## Proposal

### User Stories

As a System Administrator:
- I want to electively enable Pod Security Admission enforcement only if the cluster would have no failing workloads.
- If there are workloads in certain Namespaces that would fail under enforcement, I want to be able to identify which Namespaces need to be adjusted.
- If I enable Pod Security Admission enforcement and decide I can no longer use it, I want to disable it across my clusters at-will.

### Workflow Description

**cluster administrator** is a human user responsible for the security posture of a cluster. They are the only actor that writes `PSAEnforcementConfig.spec`.

**application owner** is a human user who deploys workloads into the cluster. They never interact with this API, but they experience its consequences when a Pod is rejected at admission.

**`PodSecurityReadinessController`** runs in the `cluster-kube-apiserver-operator` and owns `PSAEnforcementConfig.status`.

**Config Observer** runs in the `cluster-kube-apiserver-operator` and renders the kube-apiserver's global `PodSecurity` admission configuration.

**PSA label syncer** ran in the `cluster-policy-controller` and maintained Namespace-level PSA labels and annotations. It is **not an actor in this workflow**: it does not run in any enforcement mode and does not read this API. It appears in this document only as the source of the labels already present on upgraded clusters.

The starting state is a cluster on release `n+1` or later with `spec.enforcementMode` unset, which resolves to `Privileged`: the kube-apiserver applies `enforce: privileged` to Namespaces that carry no `pod-security.kubernetes.io/enforce` label, while `warn` and `audit` remain pinned to `restricted`.

Enabling enforcement:

1. The cluster administrator reads the current evaluation:

   ```bash
   oc get psaenforcementconfig cluster -o yaml
   ```

   `status.conditions` shows whether an evaluation has ever completed (`Evaluated`) and whether it is recent (`StatusStale`). `status.violatingNamespaces` lists the Namespaces that would break, with a reason for each.
2. The administrator either resolves each violating Namespace — see [Support Procedures](#support-procedures) — or decides to accept them.
3. The administrator records their intent:

   ```bash
   oc patch psaenforcementconfig cluster --type=merge \
       -p '{"spec":{"enforcementMode":"Restricted"}}'
   ```

   If violations are currently recorded, the apiserver returns an advisory `Warning` header on this update naming the count and the acknowledgement required; the update is not rejected.
4. On its next sweep the `PodSecurityReadinessController` resolves `spec` into `status`. With no violations outstanding it sets `status.enforcementMode: Restricted` and `EnforcementBlocked=False`.
5. The Config Observer observes that change and re-renders the kube-apiserver configuration with `enforce: restricted`, which cuts a new static pod revision and rolls the control plane one node at a time. Nothing else starts: no controller begins writing Namespace labels, and every Namespace without an `enforce` label of its own is now held to `restricted`.
6. The administrator confirms with `oc get psaenforcementconfig cluster -o jsonpath='{.status.enforcementMode}'`.

Variation — violations outstanding: at step 4 the controller leaves `status.enforcementMode` at `Privileged` and sets `EnforcementBlocked=True` with reason `ViolatingNamespaces`. The administrator's request stays on record. Resolving the violations causes the requested mode to take effect on the next evaluation with no further action; alternatively the administrator sets `spec.acknowledgeKnownViolations` to the `resourceVersion` the violations were reported against, which unblocks that specific set of findings and no later ones.

Variation — disabling: the administrator sets `spec.enforcementMode` to `Privileged`, or removes the field. The global `enforce` level returns to `privileged`; nothing else about PSA changes, and Namespaces that already carry an `enforce` label keep enforcing at that label's level. This is applied unconditionally and is never gated on an evaluation, because the escape hatch has to work when the evaluation machinery is exactly what has failed. See [Support Procedures](#support-procedures) for the break-glass procedure and its cost, and [Existing labels are retained, and the opt-out does not remove them](#existing-labels-are-retained-and-the-opt-out-does-not-remove-them) for why this may not be enough on an upgraded cluster.

Variation — Day 0: the Summary and [New Installation](#new-installation) both assert that enforcement can be requested at install time. The install-time surface is not yet specified; see [Open Questions](#day-0-configuration).

### API Extensions

This enhancement adds one API extension and changes the behaviour of three existing surfaces:

- **New CRD `PSAEnforcementConfig`** — a cluster-scoped singleton carrying the administrator's requested enforcement level in `spec` and the resolved, actually-in-force level in `status`. Group, version, markers and validation are still being settled with the API approvers; the Go sketch below is indicative, not final.
- **The kube-apiserver's `PodSecurity` admission plugin configuration** changes meaning. The `enforce` key becomes derived from `PSAEnforcementConfig.status.enforcementMode` rather than from the `OpenShiftPodSecurityAdmission` feature gate alone. The `audit` and `warn` keys stay pinned to `restricted` and are unchanged.
- **Namespace metadata written by another component.** The PSA label syncer, owned by the cluster-policy-controller, no longer runs at all, in any enforcement mode, so no `pod-security.kubernetes.io/*` label and no `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation is written by it on any cluster. Namespaces are a core upstream resource; the change is purely subtractive, and metadata already present is retained untouched. Opting in does not bring it back.
- **An advisory `Warning` header** returned on updates to `PSAEnforcementConfig` that raise enforcement while violations are recorded. It is advisory only and never rejects a request.

No admission webhooks, conversion webhooks, aggregated API servers or finalizers are introduced. The operational impact of these extensions is described in [Operational Aspects of API Extensions](#operational-aspects-of-api-extensions).

#### API version and availability

Throughout this document, release `n` is the release in which PSA enforcement stops being the default, currently targeted at **OpenShift 5.1**; `n-1` is 5.0 and `n+1` is 5.2.

The API ships as `v1alpha1` behind the `PodSecurityAdmissionConfiguration` feature gate, which is in `TechPreviewNoUpgrade` in release `n` and promoted to `Default` in release `n+1` once the graduation criteria below are met.

This means the API is **not** available on a supported, upgradeable cluster in release `n`. That is an accepted consequence, because the two changes are needed in different releases:

- Release `n` has to deliver *PSA enforcement is off by default*, which is achieved entirely by removing `OpenShiftPodSecurityAdmission` from the `Default` feature set and requires no new API.
- Release `n+1` delivers *and here is the supported way to turn it back on*.

Turning enforcement **off** is the safe direction, so release `n` needs no opt-out mechanism. An administrator on release `n` who wants enforcement back on before the API is generally available can set it through `unsupportedConfigOverrides` on the `kubeapiservers.operator.openshift.io` resource, as documented in the [original Pod Security Admission enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/pod-security-admission.md). This is unsupported and must be removed before upgrading to release `n+1`; see [Operational Aspects of API Extensions](#operational-aspects-of-api-extensions).

#### The `PSAEnforcementConfig` type

This API is used to support users to manage the PSA enforcement.
The API enables users to:
- view the version of PSA enforced in cluster,
- disable enforcement with "Privileged" mode,
- enforce PSA with the "Restricted" and partially with "Baseline" mode, and
- rely on the `PodSecurityReadinessController` to identify failing namespaces and report, in `status`, that the requested mode is not being applied.

`spec` is owned exclusively by the user and is never written by any controller.
`status` is owned exclusively by the `PodSecurityReadinessController`.
Where the two disagree — because the user requested `Restricted` but violating Namespaces were found — `status.enforcementMode` reports what is actually in force and the `EnforcementBlocked` condition explains why.
This keeps the user's intent recorded, so that resolving the violations allows the requested mode to take effect without the user having to re-state it, and avoids fighting GitOps controllers that continuously re-apply the desired `spec`.

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PSAEnforcementMode defines the Pod Security Standard that should be applied.
type PSAEnforcementMode string

const (
	// PSAEnforcementModePrivileged indicates that the cluster should not enforce PSA restrictions and stay in a privileged mode.
	PSAEnforcementModePrivileged PSAEnforcementMode = "Privileged"
   // PSAEnforcementModeBaseline indicates that the cluster should partially enforce PSA restrictions, if no violating Namespaces are found.
    PSAEnforcementModeBaseline PSAEnforcementMode = "Baseline"
	// PSAEnforcementModeRestricted indicates that the cluster should enforce all PSA restrictions, if no violating Namepsaces are found.
	PSAEnforcementModeRestricted PSAEnforcementMode = "Restricted"
)

// PSAEnforcementConfig is a config that allows the user to modify PSA Enforcement in a cluster. By default, it is not enabled/configured.
// The spec struct enables a user to enable and disable PSA enforcement, as required.
// The status struct supports the user in identifying obstacles in PSA enforcement.
type PSAEnforcementConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

    // version reflects the version of PSA installed on the cluster.
    Version string `json:"version"`

	// spec is a configuration option that enables the customer to dictate the PSA enforcement outcome.
	Spec PSAEnforcementConfigSpec `json:"spec"`

	// status reflects the cluster status with regards to PSA enforcement.
	Status PSAEnforcementConfigStatus `json:"status"`
}

// PSAEnforcementConfigSpec is a configuration option that enables the customer to dictate the PSA enforcement outcome.
type PSAEnforcementConfigSpec struct {
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
	// label, and is otherwise reported in status.violatingNamespaces.
	//
	// The requested mode is only applied once the PodSecurityReadinessController
	// has confirmed the cluster has no violating Namespaces. Until then,
	// status.enforcementMode reports what is actually in force, the
	// EnforcementBlocked condition explains why, and status.violatingNamespaces
	// lists what must be resolved.
	//
	// The default is Privileged. Omitting this field and setting it to
	// Privileged are equivalent and both mean the cluster has opted out of PSA
	// enforcement. Unlike most optional enums in this group, the default is a
	// fixed commitment rather than a platform choice that may change: a cluster
	// that takes no action must never acquire enforcement it did not request.
	//
	// +kubebuilder:validation:Enum:=Privileged;Baseline;Restricted
	// +optional
	EnforcementMode PSAEnforcementMode `json:"enforcementMode,omitempty"`

	// acknowledgeKnownViolations allows the requested enforcementMode to be applied
	// even though the PodSecurityReadinessController has reported violating Namespaces.
	// It exists so that raising enforcement on a cluster with known, accepted violations
	// is a deliberate act rather than a silent one.
	//
	// The value must be the resourceVersion of this object as of the evaluation being
	// acknowledged, which ties the acknowledgement to a specific set of findings. An
	// acknowledgement does not carry over to violations discovered by a later evaluation.
	//
	// When omitted, this means the user has no opinion and violations block the requested
	// mode from taking effect.
	//
	// +kubebuilder:validation:MaxLength=64
	// +optional
	AcknowledgeKnownViolations string `json:"acknowledgeKnownViolations,omitempty"`
}

// PSAEnforcementConfigStatus is a struct that signals to the user, the current status of PSA enforcement.
type PSAEnforcementConfigStatus struct {
	// enforcementMode indicates the PSA enforcement state actually in force in the
	// kube-apiserver's global PodSecurity configuration. This may lag or differ from
	// spec.enforcementMode when the requested mode has not been applied; consult the
	// EnforcementBlocked condition in that case.
	// - "Baseline" indicates that enforcement is partially enabled.
	// - "Restricted" indicates that enforcement is completely enabled.
	// - "Privileged" indicates that enforcement won't happen.
	//
	// +optional
	EnforcementMode PSAEnforcementMode `json:"enforcementMode,omitempty"`

	// conditions reports the state of the evaluation and of enforcement. Defined types are:
	// - "Evaluated" is False with reason NeverRan until the controller completes its
	//   first successful sweep, so that an absent violatingNamespaces list is never
	//   mistaken for a clean cluster.
	// - "EnforcementBlocked" is True with reason ViolatingNamespaces when
	//   spec.enforcementMode requests Baseline or Restricted but violations prevent it.
	//
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// observedGeneration is the generation of the spec that lastEvaluationTime and
	// violatingNamespaces correspond to. When it is behind metadata.generation, the
	// reported status predates the current spec.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// lastEvaluationTime is when the PodSecurityReadinessController last evaluated 
	// the cluster for PSA violations. This helps determine if a recent evaluation has 
	// taken place, which is particularly important during upgrades to ensure the
	// controller had an opportunity to check for violations before enforcing PSA.
	// +optional
	LastEvaluationTime metav1.Time `json:"lastEvaluationTime,omitempty"`

	// violatingNamespaces lists Namespaces that are violating. Needs to be resolved in order to move to Restricted.
	//
	// +optional
	ViolatingNamespaces []ViolatingNamespace `json:"violatingNamespaces,omitempty"`
}

// ViolatingNamespace provides information about a namespace that cannot comply
// with the chosen enforcement mode.
type ViolatingNamespace struct {
	// name is the Namespace that has been flagged as potentially violating if
	// enforced.
	Name string `json:"name"`

	// reason is a textual description explaining why the Namespace is incompatible
	// with the expected Pod Security mode.
	// It contains a prefix, indicating which part of the evaluation found the
	// conflict:
	// - PSAConfig: the Namespace conflicts with the global enforce level that
	//   spec.enforcementMode requests, which may be Baseline or Restricted.
	// - PSALabel: the standard inferred from the SCCs available to the
	//   ServiceAccounts in the Namespace is lower than that level. This inference
	//   was performed by the PSA label syncer historically and is performed by the
	//   PodSecurityReadinessController now that the syncer is retired.
	//
	// Possible values are:
	// - PSAConfig: Misconfigured OpenShift Namespace
	// - PSALabel: ServiceAccount with insufficient SCCs
	//
	// +optional
	Reason string `json:"reason,omitempty"`

	// state is the current state of the Namespace.
	// Possible values are:
	// - true: The Namespace would violate the enforcement mode if PSA is enforced.
	// - false: The Namespace would not violate the enforcement mode if PSA is enforced.
	//
	// +optional
	IsViolating bool `json:"isViolating,omitempty"`

	// lastTransitionTime is the time at which the state transitioned.
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty"`
}
```

#### Unset means `Privileged`

There are two outcomes, and omitting the field selects one of them:

| `spec.enforcementMode` | Meaning |
|---|---|
| `Baseline` or `Restricted` | **opt in** |
| `Privileged`, or the field omitted entirely | **opt out** |

`Privileged` *is* the default. A cluster that never mentions the field and a cluster that explicitly sets `Privileged` are in the same state and are treated identically by every consumer. There is no "no opinion" tier that behaves differently from an explicit opt-out, and no consumer may branch on which of the two it sees.

Two API notes:

- **The enum does not include `""`.** Including the empty string is an established pattern — `config.openshift.io/v1 FeatureSet` does exactly that, and there `""` *is* the `Default` feature set. But it is there because `FeatureSet` has no named value for its default; `""` is the only way to spell it. This enum does have one. Adding `""` would give two spellings of a single state with no capability gained, forcing every consumer to normalise and making `oc get -o jsonpath='{.spec.enforcementMode}'` return different strings for identical clusters. The enum is therefore `Privileged;Baseline;Restricted`, and omitting the field — which `+optional` and `omitempty` already allow — is how the default is expressed. A client that wants to clear the field sets `Privileged` or removes the key.
- **The field documents a fixed default, not a platform-chosen one.** The usual OpenShift wording for an optional enum is "the platform chooses a default, which is subject to change over time"; that is deliberately not used here, because pinning the default to `Privileged` is the point of the enhancement. API reviewers will ask about this, and the answer is that a cluster which takes no action must never acquire enforcement it did not request — which is only true if the default is a commitment rather than a placeholder.

Everything absent resolves the same way, which matters because "absent" arises in four different shapes during the rollout:

| State | How it arises | Resolves to |
|---|---|---|
| CRD absent | release `n` on a non-TechPreview cluster; MicroShift | `Privileged` |
| CRD present, singleton absent | nothing creates the object automatically | `Privileged` |
| `spec.enforcementMode` unset | the object exists, the field is not set | `Privileged` |
| `status.enforcementMode` unset | the readiness controller has not resolved `spec` yet | `Privileged` |

A consumer that treats a missing CRD differently from an unset field has a bug. The last row is the one exception worth care: it resolves to `Privileged` like the rest, but it is an *unresolved* state rather than a settled one, which is why the Config Observer gates raising enforcement on the `Evaluated` condition rather than on `status.enforcementMode` being non-empty — see [Freshness is an interlock, not a hint](#freshness-is-an-interlock-not-a-hint). Lowering is never gated on it.

#### Resolving violations

If a user encounters `status.violatingNamespaces` when PSA is enabled and configured, they are expected to:

- resolve the violations in the Namespaces, after which the requested mode takes effect on the next evaluation with no further user action, or
- set `spec.enforcementMode=Privileged` and solve the violating Namespaces later.

Because the controller never writes `spec`, a user who requested `Restricted` and hit violations keeps that request on record. The cluster reports `status.enforcementMode: Privileged` with `EnforcementBlocked=True` until the violations are resolved, and then transitions to `Restricted` on its own.

As this is an optional feature, enforcement can be turned off if needed — for example if the user manages several clusters and there are well known violating Namespaces. This is done by setting `.spec.enforcementMode` to `Privileged`, or by removing the field; the two are equivalent.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is affected, and not in the way an earlier draft of this enhancement assumed. The differences are structural rather than cosmetic, and the following are verified against the `openshift/hypershift` source:

- **There is no `cluster-kube-apiserver-operator` in a hosted control plane.** The control-plane-operator renders the kube-apiserver's PodSecurity configuration directly (`control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go`), keyed off whether `OpenShiftPodSecurityAdmission=true` appears in the rendered feature gate list. The Config Observer mechanism this enhancement builds on therefore has a second, independent implementation in HyperShift that has to change in step with it.
- **The PSA label syncer runs as a management-cluster Deployment**, reconciled as a control-plane component (`control-plane-operator/controllers/hostedcontrolplane/v2/clusterpolicy/component.go`). The hosted-cluster-config-operator (HCCO) runs no syncer logic of its own; what it reconciles is the *guest-cluster* RBAC that the management-side syncer needs in order to write Namespace labels (`control-plane-operator/hostedclusterconfigoperator/controllers/resources/resources.go` and `.../rbac/reconcile.go`). Because the syncer is retired unconditionally rather than switched by enforcement mode, HCP's change is a straight removal: the control-plane component stops starting the syncer, and it never has to be started again. Whether the guest-cluster RBAC is also removed is a separate decision — it grants a management-cluster controller write access to guest Namespaces, so leaving it in place is a standing privilege with no consumer, and removing it is the kind of change that is awkward to reverse if MicroShift's or anyone else's outcome differs. It needs a HyperShift owner's call.
- **The `PodSecurityViolation` alert already ships in HCP**, embedded in the HCCO resources and reconciled unconditionally — a copy of the standalone `cluster-kube-apiserver-operator` asset that has already drifted from its source. Any change to that alert has to be made in two places, and this enhancement retains it unchanged in both.
- HCCO opts `kube-system` out of label syncing in the guest cluster, and the management side additionally carries a `PodSecurityAdmissionLabelOverrideAnnotation` and a `restricted-psa` image label handled in `hypershift-operator/controllers/hostedcluster/hostedcluster_controller.go`. The `kube-system` opt-out becomes inert with the syncer retired. The other two interact with the enforcement level and need to be reconciled with the new API.

Two decisions remain open and are tracked in [Open Questions](#hypershift-configuration-surface):

- whether the configuration surface for a hosted cluster is a guest-cluster `PSAEnforcementConfig` CR or a field under `HostedCluster.spec.configuration`. These differ in RBAC, in tenancy, and in whether the tenant may set the value at all. The current recommendation is `spec.configuration`, which matches the existing `ClusterConfiguration` plumbing;
- how management-side and guest-side components behave while they are on different releases, since they upgrade on independent schedules. See [Version Skew Strategy](#version-skew-strategy).

A HyperShift reviewer is required before this enhancement is marked implementable.

#### Standalone Clusters

Standalone is the primary topology for this enhancement; everything outside this section is written against it.

Disconnected clusters are unaffected. No component involved reaches outside the cluster: the evaluation reads Namespaces, Pods and workload templates from the local API server, the enforcement decision is local kube-apiserver admission configuration, and the only egress-adjacent surface — the `pod_security_enforcement_mode` telemetry metric — degrades to not being reported, exactly as every other telemetry metric does on a disconnected cluster.

Bare metal is likewise unaffected. PSA is platform-agnostic; the mechanism is entirely kube-apiserver configuration plus Namespace metadata, and nothing in it depends on a cloud provider, on Machine API or on node-level configuration.

#### Single-node Deployments or MicroShift

**Single Node OpenShift.** Every change to the effective enforcement level re-renders the kube-apiserver configuration, which cuts a new static pod revision. On SNO there is one kube-apiserver, so that rollout is a short API outage rather than a rolling update — and that applies to the rollout that *disables* enforcement just as much as the one that enables it, which is precisely the path an administrator reaches for when something is already wrong. Two things follow:

- the rollout cost has to be stated in the break-glass procedure, so that nobody is surprised by an API outage while recovering from an outage;
- the Config Observer's inputs must be debounced or rate-limited, so that a status field flapping — for example a readiness controller alternating between fresh and stale — cannot cut kube-apiserver revisions in a loop.

The observer's input set is small for exactly this reason: it reads `status.enforcementMode` and two conditions on one singleton, and nothing that varies with ordinary cluster activity such as Namespace creation. Retiring the syncer helps here — with no second controller stamping labels, there is no labelling-progress signal for the observer to track and therefore no input that changes as the cluster churns.

On resource consumption, the new recurring cost on SNO is the readiness controller's sweep: one Namespace LIST plus a per-Namespace Pod LIST and the workload-template LISTs added by [Scope of the evaluation](#scope-of-the-evaluation), every four hours, through a client throttled to QPS=2 / Burst=2. Peak memory and CPU for that sweep still need to be quantified for SNO specifically, and the lists need pagination; see [Open Questions](#evaluation-cost-and-pagination).

**MicroShift.** MicroShift is affected and cannot be waved off as not applicable:

- it hardcodes `enforce: restricted` in `assets/controllers/kube-apiserver/defaultconfig.yaml`;
- it vendors and runs the cluster-policy-controller with `"*"` controllers and no feature gates, so it falls through to `NewEnforcingPodSecurityAdmissionLabelSynchronizationController` (`pkg/controllers/cluster-policy-controller.go`) — the label syncer runs in enforcing mode there today;
- it ships the syncer's RBAC and documents the resulting behaviour to users in `docs/user/howto_pod_security.md`;
- it has no CVO, no `FeatureGate` CR and no `PSAEnforcementConfig` CRD, so none of the configuration surface this enhancement adds reaches it. MicroShift's behaviour is whatever its vendored code does.

Retiring the syncer unconditionally makes MicroShift the sharpest case in this enhancement, because MicroShift is the one topology where the syncer is unambiguously doing load-bearing work: it hardcodes `enforce: restricted`, so the computed per-Namespace labels are the only thing keeping non-compliant Namespaces admitting workloads. Remove the syncer there without also moving the global default to `privileged` and workloads break on the next MicroShift release.

Three outcomes are possible and the choice is MicroShift's to make, not this enhancement's:

- **MicroShift follows OpenShift**: global default moves to `privileged` and the syncer goes. Consistent, but it is a posture change for an edge product whose users did not ask for one, delivered without the `PSAEnforcementConfig` opt-back-in that standalone gets.
- **MicroShift keeps `restricted` and keeps the syncer.** Then the syncer is not retired product-wide, only in OpenShift and HCP, and `cluster-policy-controller` has to keep the code alive and tested for a single consumer — which is the deciding input for [Is the syncer deleted or merely never started](#is-the-syncer-deleted-or-merely-never-started).
- **MicroShift keeps `restricted` and drops the syncer**, requiring every MicroShift user to label their own Namespaces. This is a breaking change and would need its own deprecation.

Whether the setting becomes configurable in `/etc/microshift/config.yaml` follows from that choice. A MicroShift reviewer is required, and this enhancement should not merge as `implementable` with the question open, because the second outcome changes what "retired" means everywhere else in this document.

#### OpenShift Kubernetes Engine

The enablement lever works on OKE. SCCs and the PodSecurity admission plugin are core components present in both products, and `PSAEnforcementConfig` is served by the same kube-apiserver, so an OKE administrator can set `spec.enforcementMode` and have it take effect.

The safety net does not. All three alerts in [Alerts](#alerts) depend on cluster monitoring, which is excluded from the OKE product offering. An OKE administrator who enables `Restricted` gets the enforcement without the staleness signal, without the blocked-enforcement signal and without the fossilised-label signal, leaving `status.conditions` as the only feedback channel — which is pull-based and therefore only seen by someone already looking.

The open decision is whether enabling `Baseline` or `Restricted` on OKE is supported without that alerting, or whether the conditions on the CR are considered sufficient. This needs an answer before GA rather than after, because it determines what the documentation is allowed to recommend.

### Implementation Details/Notes/Constraints

- The `PodSecurityReadinessController` in the `cluster-kube-apiserver-operator` will manage the new API.
- The [`Config Observer Controller`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L135) must be updated to derive the kube-apiserver's `PodSecurity` configuration from the new API's `status`.
- The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50) is **not started, in any enforcement mode**. It is not wired to the new API at all: `cluster-policy-controller` gains no watch, no client and no dependency on `PSAEnforcementConfig`. This is a removal from the startup path, not a new conditional.
- The SCC-to-PSS computation the syncer performed moves to the `PodSecurityReadinessController`, where it becomes a diagnostic input rather than a source of Namespace labels. See [The minimally sufficient standard must still be computed](#the-minimally-sufficient-standard-must-still-be-computed).
- **"Disabling PSA" means `spec.enforcementMode: Privileged`, not switching PSA off.** The `PodSecurity` admission plugin stays loaded and configured; `warn` and `audit` stay pinned to `restricted`; the `PodSecurityReadinessController` keeps evaluating; per-Namespace labels keep taking precedence. Opting in means `Baseline` or `Restricted`; opting out means `Privileged`. Neither starts or stops any controller.
- Disabling PSA enforcement on a running cluster is done by setting `spec.enforcementMode` to `Privileged`, or by removing the field — the two are equivalent. It is not done by changing the cluster's `FeatureSet`.

#### Existing Building Blocks (Already Shipped)

It is necessary to improve the diagnostics to make more accurate predictions about violating Namespaces, which means it would have failing workloads.

The [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md) revealed that certain clusters would fail enforcement.
While it can be distinguished if a workload would fail or not, the explanation is not always clear.
It can be tested for a certain assumption, but a generic diagnosis isn't possible with telemetry.

After investigating the codebase and customer feedback, it is assumed that the root cause are workloads that run with user-based SCCs.
The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) (PSA label syncer) labels Namespaces solely based on SCCs that are available to the ServiceAccounts in the Namespace.
SCCs that are given based on the user's roles are not considered as it is a bad practice to do so.

In some other cases, the evaluation was impossible because PSA labels had been overwritten by users.
Annotating the PSA label syncers decision will help to diagnose these cases.

While the root causes need to be searched for in some cases, identifying a violating Namespace properly is well understood.

The diagnostic improvements described in this subsection are **already implemented and merged**; they are documented here because the rest of this enhancement builds directly on them.
No new work is proposed in this subsection.

##### SCC Subject Type Annotation: `security.openshift.io/validated-scc-subject-type`

The annotation `openshift.io/scc` indicates which SCC admitted a workload, but it does not distinguish **how** the SCC was granted — whether through a user or a Pod's ServiceAccount.
The `security.openshift.io/validated-scc-subject-type` annotation records that distinction, which matters because the PSA label syncer does not track user-based SCCs and therefore cannot correctly label Namespaces under those circumstances.

Both constants are defined in [`openshift/api`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/security/v1/consts.go#L9-L20):

```go
ValidatedSCCAnnotation                 = "openshift.io/scc"
MinimallySufficientPodSecurityStandard = "security.openshift.io/MinimallySufficientPodSecurityStandard"
ValidatedSCCSubjectTypeAnnotation      = "security.openshift.io/validated-scc-subject-type"
```

The subject type annotation is set on the Pod by the [`SecurityContextConstraint` admission plugin](https://github.com/openshift/apiserver-library-go/blob/42e5e402ca430b5072f9d6fd5df275aff9eaf5ce/pkg/securitycontextconstraints/sccadmission/admission.go#L400-L403), on the same code path that sets `openshift.io/scc`.
Its value is derived by [`allowedForType`](https://github.com/openshift/apiserver-library-go/blob/42e5e402ca430b5072f9d6fd5df275aff9eaf5ce/pkg/securitycontextconstraints/sccadmission/scc_authz_check.go#L36-L62), which checks the Pod's ServiceAccount first and falls back to the requesting user.
The value domain is:

- `serviceaccount` — the SCC was granted to the Pod's ServiceAccount.
- `user` — the SCC was granted to the user that created the workload.
- `none` — no SCC matched.

Because the annotation is applied at admission time, it is only present on Pods created after the release that introduced it. Handling of its absence is covered under [Open Questions](#annotation-coverage-on-upgraded-clusters).

##### Minimally Sufficient PSS Annotation: `security.openshift.io/MinimallySufficientPodSecurityStandard`

Users can modify `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit`.
If both of these labels are overridden, it is not easily possible to determine which PSS could be enforced.
This annotation records the minimal PSS that would be enforced if the `pod-security.kubernetes.io/enforce` label were set.
It is written by the PSA label syncer at [`podsecurity_label_sync_controller.go#L375-L380`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L375-L380) and reconciled on user modification.

Note that the annotation is written in the same server-side apply as the PSA labels, so it is **not** set for Namespaces the syncer does not control — including Namespaces opted out with the `security.openshift.io/scc.podSecurityLabelSync=false` label, and every Namespace on the syncer's exemption list.
A Namespace with neither the annotation nor a `pod-security.kubernetes.io/enforce` label is evaluated by the kube-apiserver against the global default.

##### PodSecurityReadinessController Consumption

The [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) already consumes both annotations:

- [`violation.go#L55`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/violation.go#L55) prefers `MinimallySufficientPodSecurityStandard` over the warn/audit fallback, so it can evaluate Namespaces that lack `pod-security.kubernetes.io/enforce` but have user-overridden warn or audit labels.
- [`classification.go#L69`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/classification.go#L69) uses `validated-scc-subject-type` to classify Namespaces whose workloads were admitted via user-based SCCs.

The controller reports its findings today as six operator conditions, surfaced onto the `kube-apiserver` ClusterOperator ([`conditions.go#L17-L22`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/conditions.go#L17-L22)):
`PodSecurityUnknown`, `PodSecurityOpenshift`, `PodSecurityRunLevelZero`, `PodSecurityDisabledSyncer`, `PodSecurityInconclusive`, and `PodSecurityUserSCC` (each suffixed `EvaluationConditionsDetected`).

##### Remaining Gaps

Building on the above, this enhancement proposes:

- The new API described below, and the controller changes needed to populate and consume it.
- Reconciling the new API's status vocabulary with the six existing condition types rather than introducing a parallel one.
- Nudging users away from user-based SCCs for workloads. The existing `PodSecurityViolation` alert fires on kube-apiserver audit-mode metrics, so it does not cover the user-based-SCC case; a signal driven by the `PodSecurityReadinessController` is needed. The `PodSecurityReadinessController` exports no metrics today.
- Switching [`admission.go#L402`](https://github.com/openshift/apiserver-library-go/blob/42e5e402ca430b5072f9d6fd5df275aff9eaf5ce/pkg/securitycontextconstraints/sccadmission/admission.go#L402) from a string literal to the vendored `securityv1.ValidatedSCCSubjectTypeAnnotation` constant.

#### Role of the `OpenShiftPodSecurityAdmission` feature gate

`OpenShiftPodSecurityAdmission` already exists and today means exactly one thing: *PSA enforcement is on by default*.
It is in the `Default` feature set, which is why the config observer emits `enforce: restricted` and the label syncer runs in enforcing mode on every cluster.

This enhancement keeps that meaning and removes the gate from the `Default` feature set. That removal *is* the mechanism by which PSA enforcement becomes optional — it flips the fleet's default to `Privileged` and stops the label syncer from running.

Two consequences follow, and both are load-bearing:

- **Feature gate membership is a property of the payload, not a cluster setting.** Which gates are on in a feature set is compiled in at build time ([`payload-manifests/featuregates/`](https://github.com/openshift/api/tree/de86ee3bf48122ecb00fde7287aa633642ddc215/payload-manifests/featuregates)); an administrator selects a `FeatureSet`, not individual gates. The only feature set permitting per-gate control is `CustomNoUpgrade`, which is documented as unsupported, irreversible, and upgrade-blocking ([`types_feature.go#L51-L54`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/config/v1/types_feature.go#L51-L54)). No supported administrator workflow enables or disables this gate, so no part of this design may depend on one.
- **The opt-in machinery must not sit behind this gate.** The `PSAEnforcementConfig` API and the observer wiring that reads its `status` are guarded by their own gate, `PodSecurityAdmissionConfiguration` (name provisional), on their own graduation schedule. If they were guarded by `OpenShiftPodSecurityAdmission`, removing that gate from `Default` would remove the only means of turning enforcement back on along with the enforcement itself.

The three levers are therefore distinct and should not be conflated:

| Lever | Controlled by | Audience | Effect |
|---|---|---|---|
| `OpenShiftPodSecurityAdmission` in `Default` | Red Hat, at build time | fleet-wide, per release | whether PSA enforcement is the default posture |
| `PodSecurityAdmissionConfiguration` in `Default` | Red Hat, at build time | fleet-wide, per release | whether the `PSAEnforcementConfig` API exists and is honored |
| `spec.enforcementMode` | cluster administrator, at install time or Day 2 | one cluster | the enforcement level that cluster actually runs |

The two payload-level gates move in different releases, which is deliberate: release `n` turns enforcement off for everyone, and release `n+1` makes the supported way of turning it back on generally available. See [Graduation Criteria](#graduation-criteria).

#### PodSecurityReadinessController

The `PodSecurityReadinessController` owns the `status` of the `PSAEnforcementConfig` API. It never writes `spec`.
A [`ClusterFleetEvaluation`](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-fleet-evaluation.md) is not necessary for this enhancement as this feature is optional. However, it already collects most of the necessary data to determine whether a Namespace would fail enforcement or not. 
With the `security.openshift.io/MinimallySufficientPodSecurityStandard`, it will be able to evaluate all Namespaces for failing workloads, if any enforcement would happen.
With the `security.openshift.io/validated-scc-subject-type`, it can categorize violations more accurately.

On each evaluation the controller resolves `spec.enforcementMode` into `status`:

| `spec.enforcementMode` | Violations found | Acknowledged | `status.enforcementMode` | `EnforcementBlocked` |
|---|---|---|---|---|
| unset or `Privileged` | n/a — not evaluated for blocking | n/a | `Privileged` | `False` |
| `Baseline` or `Restricted` | no | n/a | as requested | `False` |
| `Baseline` or `Restricted` | yes | no | `Privileged` | `True`, reason `ViolatingNamespaces` |
| `Baseline` or `Restricted` | yes | yes | as requested | `False`, reason `ViolationsAcknowledged` |

Lowering enforcement is never blocked: a change to `Privileged` (or omitting the field) is applied unconditionally, so the escape hatch cannot be gated by an evaluation that is failing or stale.

"Acknowledged" means `spec.acknowledgeKnownViolations` equals the `resourceVersion` the violations were reported against. Because the acknowledgement names a specific evaluation, violations discovered by a later sweep block again rather than being silently covered by an acknowledgement given earlier for a different set of findings.

##### Feedback at the point of change

A user who sets `spec.enforcementMode` to `Baseline` or `Restricted` while violations are already recorded should not have to discover that by polling `status`. The admission path returns a `Warning` header on the update, listing the violating Namespace count and the acknowledgement needed to proceed, so the message appears in the user's terminal at the moment they make the change:

```
Warning: 12 Namespaces currently violate the Restricted standard and enforcement
will not take effect. Resolve them, or set spec.acknowledgeKnownViolations to
"<resourceVersion>" to proceed. See status.violatingNamespaces.
```

This is advisory only. It does not reject the update, so a user who genuinely intends to record intent ahead of resolving violations is not prevented from doing so.

The controller sets `status.observedGeneration` to the `metadata.generation` it evaluated, and `status.lastEvaluationTime` to the completion time of the sweep.
Together these let a user distinguish "evaluated against my current request and clean" from "status predates my change" — which matters because a user who resolves violations does **not** re-state their `spec`; the next evaluation simply observes that the blocking condition has cleared and `status.enforcementMode` advances to the requested mode on its own.

The `Evaluated` condition is `False` with reason `NeverRan` until the first successful sweep completes. An empty `status.violatingNamespaces` is only meaningful when `Evaluated=True`.

##### Freshness is an interlock, not a hint

An empty `violatingNamespaces` list is produced both by a cluster with no violations and by a controller that has never successfully run. These must not be indistinguishable, because a controller that is crash-looping — RBAC lost after an upgrade, OOM on a cluster with a very large number of Namespaces, wedged leader election — otherwise presents as a clean bill of health. A user reads "no violations", raises enforcement, and workloads begin failing while `oc get co` reports everything healthy.

Therefore:

- The Config Observer raises the enforcement level only when `Evaluated` is `True`. A non-empty `status.enforcementMode` is not by itself sufficient.
- The controller sets the `StatusStale` condition to `True` when `now - status.lastEvaluationTime` exceeds twice the evaluation interval, and the Config Observer treats a stale status the same as an unevaluated one. It does **not** lower enforcement in response to staleness, since dropping enforcement because a monitoring component is unhealthy would be a worse failure than leaving it in place.
- Repeated evaluation failure is propagated to the `kube-apiserver` `ClusterOperator` as `Degraded=True` with reason `PodSecurityReadinessEvaluationFailing`, so the condition is visible to `oc get co`, to must-gather, and to fleet monitoring, per [`CONVENTIONS.md`](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md).

##### Metrics

The alerting described in [Operational Aspects of API Extensions](#operational-aspects-of-api-extensions) requires signals that do not exist today. The controller exposes:

| Metric | Type | Purpose |
|---|---|---|
| `pod_security_readiness_last_evaluation_timestamp_seconds` | gauge | drives the staleness alert; the basis for trusting an empty violation list |
| `pod_security_readiness_evaluation_errors_total` | counter | detects a controller that is running but failing |
| `pod_security_readiness_violating_namespaces` | gauge, labelled by `reason` | how many Namespaces block enforcement, and why |
| `pod_security_enforcement_mode` | gauge, labelled by `mode` | the posture actually in force |

`pod_security_enforcement_mode` is included in telemetry so that support and the fleet dashboards can answer "is this cluster enforcing, and at what level?" without a live debugging session. The others are cluster-local.

##### Alerts

Three alerts ship with this enhancement. Each is `Warning` severity, so each requires a runbook under [`openshift/runbooks`](https://github.com/openshift/runbooks) before merge.

| Alert | Fires when | `for` |
|---|---|---|
| `PodSecurityReadinessEvaluationStale` | `time() - pod_security_readiness_last_evaluation_timestamp_seconds` exceeds twice the evaluation interval | 1h |
| `PodSecurityEnforcementBlocked` | `EnforcementBlocked` has been `True` continuously — the requested mode is not in force and nobody has resolved or acknowledged the violations | 24h |
| `PodSecurityNamespaceOverRestricted` | a Namespace carries a syncer-owned enforce label more restrictive than its computed minimally sufficient standard, per [Detecting fossilised labels](#detecting-fossilised-labels) | 6h |

The long `for` durations are deliberate: none of these represent an outage in progress, and a Namespace being briefly over-restricted during ordinary reconciliation is not worth waking anyone for.

The existing [`PodSecurityViolation`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/alerts/podsecurity-violations.yaml) alert is **retained unchanged**. It fires on `pod_security_evaluations_total{decision="deny",mode="audit"}`, and because `audit` stays pinned to `restricted` regardless of the enforcement level (see [PodSecurity Configuration](#podsecurity-configuration)), its signal is unaffected by this enhancement. It continues to report workloads that would be denied under `restricted`, which is exactly the pre-flight evidence an administrator needs before opting in. Retaining it also means no behavior change for clusters that never adopt the new API.

##### There is no labelling handshake

An earlier draft had the controller publish an `AllManagedNamespacesLabeled` condition, reporting whether every Namespace the syncer manages carried a `pod-security.kubernetes.io/enforce` label, and had the Config Observer refuse to raise the global level until it was `True`. That condition existed to sequence two operators: the syncer had to finish stamping labels before the observer raised the global default, or workloads in not-yet-labelled Namespaces would be rejected.

Retiring the syncer removes both halves. No controller stamps labels, so there is no labelling progress to wait for, and `cluster-policy-controller` is no longer a participant, so there is no second operator to sequence against. The condition is **not part of this design**, and the cross-operator ordering problem it was introduced to solve — two operators reconciling the same inputs on independent timing, where raising the level before the labels land rejects workloads — does not arise.

The word "managed" is doing two jobs here and the distinction is what decides whether anything is actually lost. In the syncer's code, "managed" means the set `isNSControlled` returns true for, which **excludes** every `openshift-`-prefixed Namespace. In ordinary OpenShift usage, "managed Namespaces" means precisely those payload Namespaces. The handshake was about the first set; the assurance that matters for the platform is about the second.

**Platform Namespaces do not rely on this design at all.** They carry PSA labels from their own CVO manifests, which the syncer never owned (see [Why the syncer is retired outright](#why-the-syncer-is-retired-outright)), and correct labelling is verified in CI: a monitor test already checks that workloads run under the SCC their Namespace's labels imply, and a second monitor test asserting that every managed Namespace carries an `enforce` label is a deliverable of this enhancement, listed in [Test Plan](#test-plan). OLM is the known exception — it declares its requirement through SCC configuration rather than Namespace labels, so the labelling test cannot cover it and it is handled separately under [`openshift-operators` has no label to freeze](#openshift-operators-has-no-label-to-freeze).

Those tests are pre-merge verification of the payload, not a runtime guardrail on a customer cluster, and they say nothing about Namespaces the customer creates. That is where the handshake's loss is real: for user Namespaces the guarantee falls from "every one carries a label sized to what it can actually run" to "no violation was found at the moment of the last sweep". A user Namespace created after that sweep, or one whose workloads change after it, has no computed label and nothing standing behind it — it is held to the global standard directly. See [Scope of the evaluation](#scope-of-the-evaluation).

##### Scope of the evaluation

PSA is a validating admission plugin. It runs only when something attempts to **create** a Pod; it never re-evaluates Pods that are already running, and raising the enforcement level never evicts a running workload.

A sweep that inspects only the Pods that exist at that moment therefore produces a point-in-time snapshot, not a guarantee. Workloads that are absent from the snapshot, or that are recreated after it, are unaffected by a clean result:

- a Deployment scaled to zero, a CronJob that has not yet fired, or a DaemonSet whose nodes are cordoned — no Pods exist to inspect;
- **an already-running violating Pod that is recreated** by a node drain, reboot, upgrade, eviction or crash-loop restart. Recreation is a new Pod creation, so it is admitted against the current enforcement level for the first time. A routine MachineConfig rollout weeks after enforcement was enabled can take down a workload that the evaluation reported as clean;
- **any user Namespace created after the sweep**, on a cluster that has opted in. With the syncer retired nothing computes an `enforce` label for it, so it is held to the global standard from the moment it exists. Under `Restricted` that means a new Namespace is `restricted` by default and stays that way until somebody labels it — which is upstream Kubernetes' behaviour, but is not what OpenShift administrators have experienced to date and is the single largest behavioural change for a cluster that opts in. Payload Namespaces are not in this bullet: they ship with their labels and are covered by the monitor tests described in [There is no labelling handshake](#there-is-no-labelling-handshake).

Two mechanisms close part of this gap and are both in scope:

- **Evaluate workload templates, not only live Pods.** `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`, `ReplicationController` and `DeploymentConfig` each describe the Pod that will be created, so the same check can run against `spec.template` whether or not a Pod exists today. This is what covers the scaled-to-zero, not-yet-fired and drain-and-reschedule cases, because the template is what gets recreated.
- **Keep `warn` and `audit` at the target standard while `enforce` remains `privileged`.** Every creation that *would* be rejected is then surfaced continuously and nothing is blocked, turning a single snapshot into a running record. This is the mechanism the [original Pod Security Admission enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/pod-security-admission.md) used for the same purpose, and the machinery already exists.

Neither mechanism covers the last bullet. Both operate before enforcement is raised; once a cluster is at `Restricted`, a Namespace created afterwards is enforced immediately and the evaluation's next sweep reports it only after the fact. The syncer used to cover this case by labelling new Namespaces as they appeared, and for user Namespaces nothing does now. This is the residual risk an administrator accepts by opting in, and it is recorded in [Risks and Mitigations](#risks-and-mitigations).

#### PodSecurity Configuration

A Config Observer in the `cluster-kube-apiserver-operator` manages the Global Config for the kube-apiserver.

There is no "unset" option for this configuration. [`defaultconfig.yaml#L12-L22`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/config/defaultconfig.yaml#L12-L22) hardcodes `enforce: "invalid-to-force-substitution"`, so the observer must always substitute a concrete level or the kube-apiserver receives an invalid admission configuration. Accordingly [`podsecurityadmission.go#L99-L111`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L99-L111) has exactly two branches today — `privileged` and `restricted` — and no third state. "Disabling PSA" means substituting `privileged`, not omitting the configuration.

The Config Observer's inputs are:
- `status.enforcementMode` — the administrator's request, after the readiness controller has resolved it;
- the `FeatureGate` `OpenShiftPodSecurityAdmission` — used only to pick the level substituted when `status.enforcementMode` is `Privileged`, empty, or unavailable. The fallback is `restricted` while the gate is still in the `Default` feature set, and `privileged` once it leaves in release `n`. This exists only to preserve today's behaviour across the transition; from release `n` onward the fallback is `privileged` and stays there, which is what makes unset and `Privileged` equivalent;
- the `Evaluated` and `StatusStale` conditions on the `PSAEnforcementConfig` `status`.

That is the whole input set. The observer does not list Namespaces, and it has no input that varies with ordinary cluster activity — see [There is no labelling handshake](#there-is-no-labelling-handshake).

The observer substitutes a level above the fallback only when both of the following hold:

- `status.enforcementMode` is `Baseline` or `Restricted`;
- `Evaluated` is `True` and `StatusStale` is `False`, so the result rests on a real and recent sweep rather than on a controller that has never run — see [Freshness is an interlock, not a hint](#freshness-is-an-interlock-not-a-hint).

Substituting `privileged` is subject to neither condition. Lowering enforcement must work when the readiness controller is broken, because that is precisely when an administrator is most likely to need it.

This enhancement changes only the `enforce` key. Both existing branches pin `audit` and `warn` to `restricted` regardless of the enforcement level ([`podsecurityadmission.go#L32-L39`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L32-L39), [`#L50-L57`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L50-L57)) and that is retained deliberately: it is what keeps violations observable while `enforce` is `privileged`, per [Scope of the evaluation](#scope-of-the-evaluation). The corresponding `*-version` keys are unchanged.

`Baseline` has no implementation today — only the privileged and restricted helpers exist — so a third branch must be added.

This state must be watched continuously.
If `status.enforcementMode` returns to `Privileged`, the observer applies that immediately and unconditionally — the escape hatch is never gated on the evaluation.

#### PodSecurityAdmissionLabelSynchronizationController

The [PodSecurityAdmissionLabelSynchronizationController (PSA label syncer)](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) is **retired**. It does not run in any enforcement mode, it is not kept running in a reduced advising or annotation-only mode, and it is not restarted when an administrator opts in.

This is the single largest behavioural change in this enhancement, larger than the change of default, and it is stated as a flat rule because every conditional version of it that was considered turned out worse. There is no `status.enforcementMode` value that starts the controller and no cluster configuration that brings it back.

Namespaces that were **managed** by the syncer — the set it acted on, and therefore the set that carries its labels on any cluster upgrading into release `n` — are Namespaces that:

- are not named `kube-node-lease`, `kube-system`, `kube-public`, `default` or `openshift` and
- are not prefixed with `openshift-` and
- have no `security.openshift.io/scc.podSecurityLabelSync=false` label set and
- at least one PSA label (including `pod-security.kubernetes.io/enforce`) isn't set by the user or
- if the user sets all PSA labels, it also has set the `security.openshift.io/scc.podSecurityLabelSync=true` label.

That definition is now historical. It describes where the retained labels came from, not a set of Namespaces any running controller acts on.

##### Why the syncer is retired outright

The obvious design is to tie the syncer to the enforcement mode: off at `Privileged`, on at `Baseline` or `Restricted`. That is rejected, and the reasons divide into one that applies to the opt-out and three that apply to the opt-in.

**At `Privileged` the syncer has nothing to do.** Its premise is a cluster whose global default is `restricted`: in that world every Namespace whose ServiceAccounts cannot meet `restricted` needs a computed, *less* restrictive `enforce` label or its workloads stop being admitted, and the syncer is the machinery that derives that label from the SCCs available in the Namespace. It exists to make a `restricted` default survivable. Once the default is `privileged` — already the most permissive standard PSA offers — a Namespace carrying no label is admitted regardless of what its ServiceAccounts can do, and a computed label protects against nothing. Running it anyway would write per-Namespace PSA metadata that no admission decision consumes, on every managed Namespace, indefinitely, including on Namespaces whose administrator explicitly opted out.

**At `Baseline` or `Restricted` the syncer contradicts the request.** This is the part that is easy to miss. An administrator who sets `Restricted` is asking for a cluster where unlabelled Namespaces are held to `restricted`. The syncer would immediately grant many of those Namespaces a *lower* standard, computed from their SCCs, without telling anyone. The cluster would report `status.enforcementMode: Restricted` while running a per-Namespace patchwork that neither the administrator chose nor the API describes. Under the old mandatory-enforcement model that was the point — it was damage control for a default nobody opted into. Under an opt-in model it silently weakens the thing that was opted into, and the API becomes a claim the cluster does not honour.

**The inference it relies on is the known-unreliable part of the system.** The syncer derives the standard from SCCs available to the Namespace's ServiceAccounts. The [Motivation](#motivation) section of this enhancement exists because that inference is wrong often enough to matter: it does not account for user-based SCCs, and it is defeated by user-overridden labels. Those are the two root causes behind most violating Namespaces found by the fleet evaluation. Keeping the syncer would keep an unreliable inference in the *enforcement* path, where being wrong means either a workload rejected or a Namespace quietly running below the requested standard. Moving the same computation into the readiness controller keeps it where being wrong produces a misleading diagnostic that an administrator can inspect and override — see [The minimally sufficient standard must still be computed](#the-minimally-sufficient-standard-must-still-be-computed).

**A controller that starts and stops is harder to reason about than one that does not exist.** A mode-switched syncer has to handle being started on a cluster it has never seen, catching up across every Namespace before enforcement is safe to raise, and being stopped mid-apply — which is the cross-operator sequencing problem an earlier draft tried to solve with an `AllManagedNamespacesLabeled` handshake. Retiring it deletes that problem rather than specifying it. See [There is no labelling handshake](#there-is-no-labelling-handshake).

The resulting rule is one row, not a table:

| `status.enforcementMode` | Syncer | Namespace labels |
|---|---|---|
| any value, including unset | **not running** | nothing is written, ever. Labels already present are left exactly as they are, because a controller that never applies never prunes |

Today the choice is made once at construction from the `OpenShiftPodSecurityAdmission` gate ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50)), with enforcing as the default branch and advising as the alternative. Both branches go. `cluster-policy-controller` gains no watch on `PSAEnforcementConfig` and no dependency on the `cluster-kube-apiserver-operator`; the change is a deletion from its startup path. Whether the controller's code is removed outright or left in place unreferenced is [an open question](#is-the-syncer-deleted-or-merely-never-started) whose answer depends on MicroShift.

The advising path is the one whose server-side-apply behaviour is [neither freezing nor removing](#freezing-is-not-the-same-as-removing); not running avoids that behaviour rather than having to rebuild it.

None of this affects OpenShift's own Namespaces. `isNSControlled` ([`podsecurity_label_sync_controller.go#L475-L522`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L475-L522)) excluded them twice: first against the hardcoded payload list in `nsexemptions`, then by skipping any Namespace prefixed `openshift-` outright. Payload Namespaces carry PSA labels from their own manifests, which the syncer never owned and therefore could neither write nor remove.

##### What retiring it costs

Three things the syncer produced stop being produced. The first two are load-bearing elsewhere in this enhancement; the third is the one an administrator will actually notice.

- **No Namespace gets a computed `enforce` label, on any cluster, ever again.** On a cluster that opts in to `Baseline` or `Restricted`, the global level applies directly to every Namespace without a label of its own — including Namespaces created after the opt-in, which nothing labels and which the last evaluation could not have seen. Under the previous design those Namespaces were caught by the syncer; now whoever owns the Namespace is responsible for labelling it if it cannot meet the cluster-wide standard. For payload Namespaces that owner is the component team, the label travels in the CVO manifest, and CI enforces it. For user Namespaces it is the administrator, with no equivalent check. This is upstream Kubernetes' behaviour and it is defensible, but it is not what OpenShift administrators have experienced, and it means opting in is a sharper action than it was. It is recorded as a risk in [Risks and Mitigations](#risks-and-mitigations) and as a drawback in [Drawbacks](#drawbacks).

- **The `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation stops being maintained.** It is written by the syncer ([`podsecurity_label_sync_controller.go#L375-L380`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L375-L380)) and consumed by the `PodSecurityReadinessController` ([`violation.go#L55`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/violation.go#L55)) to evaluate Namespaces whose `warn`/`audit` labels have been overridden. With the syncer retired, that input goes stale on existing Namespaces and is absent on new ones — on every cluster, not only the ones that have opted out. The SCC-to-PSS computation therefore has to move into the `PodSecurityReadinessController`, which already sweeps every Namespace. This is no longer optional or conditional work: it is the only remaining implementation of the mapping on an OpenShift cluster. See [The minimally sufficient standard must still be computed](#the-minimally-sufficient-standard-must-still-be-computed) and [Open Questions](#who-computes-the-minimally-sufficient-standard-now-the-syncer-is-retired).
- **The fossilised-label detection in [Detecting fossilised labels](#detecting-fossilised-labels) depends on that same annotation** to know that a retained `enforce` label is now more restrictive than the Namespace needs. It works only if the computation is relocated as above.

Per-Namespace `warn` and `audit` labels also stop being written, but nothing depends on them: the kube-apiserver's global configuration pins `warn` and `audit` to `restricted` regardless of the enforcement level (see [PodSecurity Configuration](#podsecurity-configuration)), so violations remain observable cluster-wide without any per-Namespace label.

##### The gap retiring the syncer cannot close

Retaining existing labels protects only Namespaces that *have* a label. The kube-apiserver's PodSecurity configuration carries no Namespace exemptions at all — [`defaultconfig.yaml`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/bindata/assets/config/defaultconfig.yaml) exempts exactly one username, `system:serviceaccount:openshift-infra:build-controller` — so any Namespace without its own `enforce` label falls through to the global default and is relaxed when that default moves to `privileged` in release `n`.

`openshift-operators` is exactly that Namespace, and it is the one OLM users install operator bundles into. It is deliberately kept out of the `nsexemptions` list — the list carries an `IMPORTANT:` comment explaining that it must not be exempted — but it is then caught by the `openshift-` prefix skip, so the syncer never labelled it even when it ran. It has neither a syncer-written label nor a manifest-written one, and is enforced at `restricted` today purely by the global default. On upgrade to release `n` its effective level silently becomes `privileged`, and nothing above prevents that.

This is the same `openshift-operators` hole described in [Risks and Mitigations](#risks-and-mitigations), seen from the relaxation side rather than the breakage side. It is not resolved by this enhancement and is [an open question](#openshift-operators-has-no-label-to-freeze).

##### Existing labels are retained, and the opt-out does not remove them

Because a controller that does not run performs no server-side apply, the `pod-security.kubernetes.io/enforce` labels already on an upgraded cluster are retained by construction — and stay retained, since no enforcement mode brings the syncer back to reconcile them. That is the intended outcome, for three reasons:

- **Optional is not the same as off.** Stripping every syncer-written `enforce` label on upgrade would not make enforcement optional; it would replace "every cluster must enforce" with "every cluster must stop enforcing". Optionality means the cluster's existing state persists until an administrator chooses otherwise, and `PSAEnforcementConfig` is how they choose.
- **One direction is reversible and the other is not.** Retention can be undone by hand. Stripping cannot: server-side apply deletes the value, and returning to `Restricted` recomputes labels from current SCC and RBAC state rather than restoring what was there. Where only one direction is recoverable, the upgrade should take it.
- **The exposure is compliance, not availability.** Relaxing enforcement can only admit more workloads; it breaks nothing and causes no outage. The risk is a security control disappearing without announcement from clusters that may be attesting to it, and being discovered long afterwards.

The consequence has to be stated plainly, because it cuts against the feature's purpose. Per-Namespace PSA labels take precedence over the global default in the kube-apiserver's admission configuration. The config observer only sets that global default. So on an upgraded cluster — where essentially every managed Namespace carries a syncer-written `enforce` label — setting `enforcementMode: Privileged` lowers the global default and changes the effective level of *nothing that already has a label*. The API is inert on precisely the clusters this enhancement exists to help, until somebody removes those labels.

Removing them is a one-shot cleanup, not a syncer mode: the syncer is retired, so it cannot be the thing that does it in any configuration. Whether that cleanup ships, what performs it, and whether it is automatic on the transition to `Privileged` or a documented `oc` procedure, is [an open question](#removing-retained-enforce-labels-on-opt-out). Until it is answered, [Drawbacks](#drawbacks) records the inertness as real.

##### What release `n` inherits

An earlier draft required release `n-1` to be able to remove the `pod-security.kubernetes.io/enforce` labels set in release `n`. That requirement has been dropped: the label is not introduced by release `n`. It is written today, by the syncer's enforcing default, on every cluster with `OpenShiftPodSecurityAdmission` in `Default`. Release `n` stops writing it rather than starting to.

What release `n` does inherit is the labels already on upgraded clusters, retained as described above.

#### Existing Clusters

Every cluster running today has `OpenShiftPodSecurityAdmission` in its `Default` feature set, and the label syncer's **enforcing** constructor is the default branch ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50)) — the non-enforcing path requires explicitly passing `OpenShiftPodSecurityAdmission=false`. So `pod-security.kubernetes.io/enforce` labels are already present across the fleet, applied via server-side apply under the field manager `pod-security-admission-label-synchronization-controller` ([`podsecurity_label_sync_controller.go#L386`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L386)).

Retiring the syncer stops it writing new labels. It does not remove the existing ones, no code path in any component removes them, and — because no enforcement mode restarts the syncer — nothing ever reconciles them again either. They are frozen at whatever value the last pre-upgrade sync left.

##### The labels are retained

Deleting them on upgrade would silently reduce the enforcement posture of clusters that are relying on it, which is a compliance event for customers under FedRAMP, PCI or DISA STIG. Retaining them is the safer default and is what this enhancement does.

Retaining them unchanged, however, introduces a regression of its own. Consider a cluster upgraded into release `n`, where `some-namespace` is labelled `pod-security.kubernetes.io/enforce: restricted` by the syncer before the upgrade:

1. An administrator grants that Namespace's ServiceAccount the `anyuid` SCC, expecting to run a workload that needs a fixed UID.
2. Before release `n`, the syncer would have recomputed the Namespace's minimally sufficient standard as `baseline` and relaxed the label accordingly.
3. In release `n` the syncer is not running, so the label stays pinned at `restricted`.
4. The workload is rejected at admission, on a cluster whose administrator never opted into this feature and has no reason to associate the failure with an upgrade.

The label is now a fossil: it records a decision made by a controller that is no longer running.

Because the syncer is retired unconditionally, there is no configuration that resolves this. Under the mode-switched design an administrator hitting this could opt in to `Restricted`, which would restart the syncer and cause it to recompute and relax the label on its next sync — an obscure remedy, but a remedy. That no longer exists. The only fixes are to edit the label by hand or to delete it and let the Namespace fall to the global default, and the [alert](#detecting-fossilised-labels) is what tells the administrator to do so.

##### The minimally sufficient standard must still be computed

Detecting that fossil requires knowing what the Namespace's minimally sufficient standard *would* be, which is the calculation the syncer used to perform and record in `security.openshift.io/MinimallySufficientPodSecurityStandard`. With the syncer retired, nothing performs it on any cluster in any mode, so relocating this computation is required work for this enhancement rather than a mitigation for one configuration of it.

An earlier draft solved this by keeping the syncer alive in an annotation-only mode. That is rejected for the reasons in [Why the syncer is retired outright](#why-the-syncer-is-retired-outright): a controller that still watches every Namespace, every ServiceAccount and every SCC, and still writes Namespace metadata, is not retired in any sense an administrator would recognise, and it is the mode whose server-side-apply semantics cause the non-deterministic label decay described below.

The calculation moves instead to the `PodSecurityReadinessController`, which already sweeps every Namespace on a fixed interval and already consumes the annotation today. Relocating it has three consequences that the annotation-only design did not have:

- **Coverage is no longer limited to syncer-controlled Namespaces.** The syncer's `sync` returns early where `isNSControlled` is false ([`podsecurity_label_sync_controller.go#L204`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L204)), so uncontrolled Namespaces — including every `openshift-`-prefixed one — never received an annotation and had no computed baseline to compare against. A readiness-controller-side computation has no such restriction.
- **It is a read, not a write.** The readiness controller can hold the computed standard in `status` rather than stamping it onto Namespaces, which removes the last writer from Namespace PSA metadata on every cluster, in every mode. Whether it is still worth writing the annotation for support and must-gather is [an open question](#who-computes-the-minimally-sufficient-standard-now-the-syncer-is-retired).
- **It changes nothing about enforcement.** The value is diagnostic and is not read by any admission path. Payload Namespaces set their PSA labels explicitly in their own CVO manifests — `openshift-kube-apiserver-operator` is pinned `restricted` and `openshift-kube-apiserver` `privileged` — so the `Restricted`-by-default expectation that OpenShift components are developed against is held by those manifests, not by the syncer or the global default, and is unaffected. For such Namespaces the computed standard may be *lower* than the label enforces; that is informational and must not be read as a recommendation to relax a deliberately pinned Namespace.

##### Detecting fossilised labels

With the minimally sufficient standard computed for every Namespace, a Namespace whose enforce label is more restrictive than that minimum is detectable. This drives a metric and an alert, so the `anyuid` case above surfaces to the administrator instead of being debugged cold.

The alert must not fire on labels an administrator set deliberately. Ownership is recorded in `metadata.managedFields`, tracked per label key rather than per Namespace, and there is existing precedent for parsing it in the syncer's own write decisions ([`extractNSFieldsPerManager`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L546), [`getManagerForLabel`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L562)) that the readiness controller can follow. A label written with `oc label`, `oc edit` or by a GitOps controller carries that manager's name and is plainly distinguishable from one the syncer wrote before it was retired.

Ownership resolves to three cases:

| Owner of `pod-security.kubernetes.io/enforce` | Interpretation | Behavior |
|---|---|---|
| `pod-security-admission-label-synchronization-controller`, or the historical `cluster-policy-controller` | written by the syncer, now unmaintained | alert when more restrictive than the computed minimum |
| any other manager | deliberate configuration by an administrator, GitOps controller or CVO manifest | never alert |
| no owner recorded | ambiguous | metric only, no alert |

The unowned case is ambiguous rather than merely unknown. Backup and restore tooling, etcd restore and some migration paths drop or rewrite `managedFields`, so a label the syncer wrote can lose its ownership record; the same ownership rename already happened inside OpenShift, which is why [`forceHistoricalLabelsOwnership`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L224) exists and why both manager names are accepted above. Alerting on unowned labels would page administrators about configuration they may have set deliberately, on exactly the long-lived clusters where deliberate configuration is most likely. The count is therefore exposed as a metric for support to consult during an investigation, without an alert attached.

Note that this ownership signal is safe here because nothing is deleted. It fails safe for deciding whether to *write* a label — the worst case is setting one nobody owned — but it would fail unsafe for deciding whether to *delete* one, since an ownership record lost to a restore is indistinguishable from an administrator's deliberate label. Retaining the labels avoids that direction entirely.

##### Release note

The change in default posture, the retention of existing labels, and the new alert are called out in the release notes for release `n`, since an upgraded cluster's effective behavior changes without any administrator action.

#### New Installation

Fresh installs are not enforcing PSA by default. If the system administrator does not configure PSA at install time, `spec.enforcementMode` is unset, which resolves to `Privileged`: the kube-apiserver's global `enforce` level is `privileged`.

Nothing else about PSA is switched off. The `PodSecurity` admission plugin is still loaded, `warn` and `audit` are still pinned to `restricted`, the `PodSecurityReadinessController` still evaluates the cluster, and per-Namespace `pod-security.kubernetes.io/*` labels are still honoured. "PSA is disabled" on a fresh install means exactly `enforce: privileged`, and nothing more.

From there the administrator electively raises `spec.enforcementMode` to the level they are comfortable with, and returns it to `Privileged` — or unsets it — to go back.

A fresh install is the cleanest case for this design, and the only one where retiring the syncer costs nothing. No Namespace carries a syncer-written label, so there are no fossils, the API is not inert, and `Privileged` really is the cluster's effective posture rather than only its default. The trade appears later: once the administrator opts in to `Restricted`, every Namespace their workloads create from then on is `restricted` unless they label it, and nothing is computing labels on their behalf. That is the behaviour to document, because it is what an administrator adopting this on a new cluster will meet first.

There is no need for the `PodSecurityReadinessController` to gate anything on a fresh install, because OpenShift's own workloads and Namespaces are labelled appropriately by their own manifests. It becomes load-bearing once customer workloads and Namespaces exist, and on existing clusters where they already do.

### Risks and Mitigations

- **The out-of-the-box security posture is reduced.** Clusters that take no action end up less restrictive than they are today. *Mitigation:* per-Namespace `pod-security.kubernetes.io/*` labels continue to take precedence and are unaffected; SCCs, OpenShift's primary workload admission control, are unchanged; existing enforce labels on upgraded clusters are retained rather than removed; and the change is called out in the release note for release `n`. This risk is not fully mitigated by design — it is the trade this enhancement makes — and it requires explicit security sign-off before the EP is marked implementable.
- **A clean evaluation is not a guarantee.** PSA is validating-admission only, so a workload that is not running at evaluation time, or that is recreated later by a drain or upgrade, can fail long after an administrator was told the cluster was clean. *Mitigation:* workload templates are evaluated alongside live Pods, and `warn`/`audit` stay pinned to `restricted` so violations keep surfacing continuously. See [Scope of the evaluation](#scope-of-the-evaluation).
- **An empty violation list can mean "no violations" or "never evaluated".** *Mitigation:* the `Evaluated` and `StatusStale` conditions gate any raise in enforcement, and repeated evaluation failure degrades the `kube-apiserver` ClusterOperator. See [Freshness is an interlock, not a hint](#freshness-is-an-interlock-not-a-hint).
- **Opting in has no runtime safety net for unlabelled user Namespaces.** With the syncer retired, `Baseline` or `Restricted` applies directly to every Namespace that carries no `enforce` label of its own. Nothing computes a gentler per-Namespace standard, so a Namespace that cannot meet the requested level fails admission rather than being relaxed. *Mitigation:* for payload Namespaces, the CVO manifests supply the labels and the monitor tests in [Test Plan](#test-plan) verify they are correct and complete before the release ships — so the risk here is not a platform risk. For user Namespaces, the evaluation must report zero violating Namespaces before the observer raises the level, and the administrator can label any Namespace by hand. *Not mitigated:* user Namespaces created after the evaluation, whose workloads nothing has assessed; and OLM's Namespaces, which express their requirement through SCC configuration rather than labels and so are outside both the manifests and the labelling test — see [`openshift-operators` has no label to freeze](#openshift-operators-has-no-label-to-freeze). This is the direct consequence of [Why the syncer is retired outright](#why-the-syncer-is-retired-outright) and it makes opting in a materially sharper action for user workloads than it was under mandatory enforcement with a syncer.
- **`openshift-operators` is deliberately non-exempt from the syncer but is skipped by the `openshift-` prefix rule**, so it has neither a computed label nor a manifest-pinned one and falls to the global default. On a cluster that opts into `Restricted`, every operator bundle needing more than restricted then fails admission. **This is currently unmitigated**: the happy path of the feature is a mass-breakage path for OLM users. Retiring the syncer neither causes nor worsens this — the prefix skip means the syncer never labelled `openshift-operators` in the first place — but it removes one candidate mitigation, since there is no longer a running syncer that could be taught to make it an exception. OLM and layered-product reviewers are required, and the mitigation still has to be chosen; the candidates are in [`openshift-operators` has no label to freeze](#openshift-operators-has-no-label-to-freeze).
- **Every enforcement change rolls the kube-apiserver.** On SNO that is an API outage, including on the disable path. *Mitigation:* lowering is unconditional so it is never blocked, the cost is documented in the break-glass procedure, and the observer's inputs are debounced.
- **The subject-type annotation is absent on pre-existing Pods**, which makes both possible readings wrong — see [Open Questions](#annotation-coverage-on-upgraded-clusters). Unmitigated pending that decision.

Security review is required from the OpenShift security architecture group, covering the default-posture change specifically rather than the API. UX review is required from the console and docs teams for the `oc` workflow in [Workflow Description](#workflow-description) and for the wording of the advisory `Warning` header.

### Drawbacks

- It ships a less secure default than the product has today, and no mechanism preserves the current default for clusters that take no action. For a product whose positioning includes "secure by default", that is a real cost and not only a documentation problem.
- It adds a second place to look for PSA state. The effective configuration already lives in the kube-apiserver's admission config, in Namespace labels and in the feature gate; a new CRD adds a fourth, with its own reconcile loop and its own staleness semantics. One optional field on an existing config resource would avoid that; see [Alternatives (Not Implemented)](#alternatives-not-implemented).
- It bundles three separable changes — the already-shipped SCC annotation work, retiring the PSA label syncer, and the new configuration API. Approving the API implicitly approves the syncer's removal, which has by far the largest blast radius of the three and is currently receiving the least review attention. Retiring the syncer is not even conditional on the API: it happens on every cluster whatever `spec.enforcementMode` says, so a reviewer who evaluates only the API has not evaluated the change.
- **Opting in is a weaker guarantee than the enforcement it replaces.** Under today's mandatory `restricted`, the syncer ensures each Namespace ends up at the strictest standard it can actually meet. Under this design, `Restricted` means `restricted` everywhere unlabelled — stricter in principle, but only survivable if the cluster is already clean, and offering nothing to a cluster that is mostly clean. There is no longer a per-Namespace middle ground, so administrators with heterogeneous clusters must either label Namespaces themselves or stay at `Privileged`. Some clusters that today run a syncer-computed mix of `restricted` and `baseline` have no equivalent state available after this change.
- Retiring the syncer removes the cluster's only maintained source of `security.openshift.io/MinimallySufficientPodSecurityStandard`, which the readiness evaluation and the fossilised-label alert both consume. The calculation has to be rebuilt in the `PodSecurityReadinessController` — net new work that the earlier annotation-only design avoided, and a second implementation of the SCC-to-PSS mapping unless it is factored into a shared library. Because the syncer never comes back, this relocation is a hard prerequisite rather than a mitigation. See [Who computes the minimally sufficient standard now the syncer is retired](#who-computes-the-minimally-sufficient-standard-now-the-syncer-is-retired).
- Retained enforce labels become unmaintained, permanently. The fossilised-label alert makes them visible, but the underlying situation — a label written by a controller that no longer exists, which no supported action will ever refresh — is a new class of cluster state that support will have to reason about for as long as those clusters live.
- **The feature does nothing on an upgraded cluster until the labels are dealt with.** Because existing `enforce` labels are retained rather than removed, and because retiring the syncer means no component removes them, a cluster that upgrades into release `n` sees no change to any Namespace that already has an effective level — and setting `enforcementMode: Privileged` does not change that either, since per-Namespace labels outrank the global default. The clusters most in need of relief — those already struggling under mandatory enforcement — get none from the API alone. This is the deliberate trade argued in [Existing labels are retained, and the opt-out does not remove them](#existing-labels-are-retained-and-the-opt-out-does-not-remove-them), but it means the headline claim "PSA enforcement is now optional" is true of the product and not yet true of any given upgraded cluster, and the release note has to carry that distinction.
- It introduces a configuration change whose application costs a control-plane rollout, which on SNO is an outage, in both directions.

## Alternatives (Not Implemented)

The alternatives below have been identified but not yet evaluated to a conclusion. This section is a placeholder for that evaluation and has to be completed before the EP moves from `provisional` to `implementable`.

- **An optional field on `config.openshift.io/v1 APIServer`.** This is the closest existing home: it is already the cluster-wide API server configuration resource, already cluster-scoped and singleton, already consumed by the `cluster-kube-apiserver-operator`'s config observers, and adding a field avoids a new CRD in the payload entirely. The cost of the CRD over this option is a second place to look for PSA state and a second reconcile loop. This is the leading alternative and the one most likely to be raised in API review.
- **An optional field on `operator.openshift.io/v1 KubeAPIServer`.** Closer to the implementation, but the operator resources are conventionally operator-owned rather than an administrator-facing configuration surface, and `unsupportedConfigOverrides` already lives there.
- **Document `unsupportedConfigOverrides` and ship nothing.** Zero API surface, and it is the recovery path release `n` relies on already. Rejected in principle because an unsupported override is not an acceptable long-term answer for a supported posture decision, but the comparison should be written out.
- **Report status through the existing six `PodSecurity*EvaluationConditionsDetected` ClusterOperator conditions** instead of a new status struct. The controller already produces these. This would avoid the scale problem in `status.violatingNamespaces` on large clusters, at the cost of not being able to name the specific Namespaces.

## Open Questions

### Rerun the evaluation before enforcing

It could be better to run the evaluation before enforcing `Restricted` mode as the last run can be a couple of months old.
With alerts in place in the release before, it could be sufficient to run the evaluation just before enforcing `Restricted` mode.
In addition we could list the Namespaces that are violating the PSS in the `status.violatingNamespaces` field.

### Initial evaluation by the PodSecurityReadinessController

What happens if the `PodSecurityReadinessController`, didn't have the time to run at least once after an upgrade?

If a cluster upgrades from release `n-1`, through release `n` to release `n+1`, the `PodSecurityReadinessController` might not have time to run at least once in release `n` during the upgrade.
To prevent an unchecked transition into `Restricted` mode on release `n+1`, it will retry checking Namespaces until `status.lastEvaluationTime` is set.
Once it ran successfully, it needs to set the `status.lastEvaluationTime`.
This will give the user the ability to see how old the basis for the decision is.

### Annotation coverage on upgraded clusters

`security.openshift.io/validated-scc-subject-type` is written at admission time, so it is absent on exactly the long-running, pre-existing workloads whose SCC provenance the evaluation most needs to know. Neither default reading is safe:

- treating absence as `serviceaccount` produces false negatives — the Namespace is reported clean and the workload fails the next time it is recreated;
- treating absence as `user` produces false positives on every Pod on every upgraded cluster, which makes the feature unusable on any cluster not born with the annotation.

The likely answer is an explicit third state surfaced in the per-Namespace `reason`, plus a coverage signal — the proportion of Pods carrying the annotation — that the Config Observer can gate on. Additionally, nothing currently states that the SCC admission plugin overwrites a user-supplied value for this annotation; if it did not, a user able to create Pods could hide a violation from the evaluation.

### PSA label syncer in OpenShift's own CI

The syncer is gone in every mode, per [Why the syncer is retired outright](#why-the-syncer-is-retired-outright). An earlier draft added that OpenShift developers "will remain using the PSA label syncer to ensure OpenShift workloads still comply, such as in monitor and periodic tests", since the expected default PSS for OpenShift's own components stays `Restricted`.

That claim does not survive contact with the code, and under this design it is not available even if it did. `isNSControlled` skips every Namespace prefixed `openshift-` outright, so the syncer never labelled OpenShift's own Namespaces in the first place; their PSA labels come from their own CVO manifests. Whatever the monitor and periodic tests are actually relying on, it is not the syncer labelling payload Namespaces.

What remains to be decided is how CI lanes get a `restricted` cluster at all. Opting in via `PSAEnforcementConfig` sets the global default, which is what payload Namespaces without their own labels would then be held to — but it no longer brings the syncer with it, so any test Namespace the lanes create themselves (`e2e-test-*` and similar, which are not `openshift-` prefixed and were syncer-managed) now inherits `restricted` with nothing computing a gentler label for it. Whether that breaks existing e2e suites, and whether the fix is per-test labelling or a broader exemption, is the input the [Test Plan](#test-plan) is missing.

### Who computes the minimally sufficient standard now the syncer is retired

Retiring the syncer stops `security.openshift.io/MinimallySufficientPodSecurityStandard` being maintained on every cluster, in every mode, and both the readiness evaluation ([`violation.go#L55`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/violation.go#L55)) and [Detecting fossilised labels](#detecting-fossilised-labels) consume it. [What retiring it costs](#what-retiring-it-costs) moves the SCC-to-PSS computation into the `PodSecurityReadinessController`; because there is no mode in which the syncer still supplies the value, that relocation is a prerequisite for this enhancement rather than a choice. What is not decided:

- whether the relocated computation still writes the annotation onto Namespaces — useful for `oc get ns -o yaml`, support and must-gather, but it puts a writer back onto Namespace metadata on clusters that have opted out — or holds the value only in the controller's own `status` and metrics;
- whether the mapping *moves* to `cluster-kube-apiserver-operator` or is *shared* with `cluster-policy-controller`. This follows from [Is the syncer deleted or merely never started](#is-the-syncer-deleted-or-merely-never-started): if the syncer's code goes, the mapping moves and there is one implementation; if it stays for MicroShift's benefit, the mapping has two callers in two repositories and must be a shared library rather than a copy, or it drifts from itself as well as from upstream. Either way the [hardcoded SCC to PSA mapping](#non-urgent-but-important-hardcoded-scc-to-psa-mapping) test has to cover whatever survives;
- what the annotation's value means during the transition, when some Namespaces carry a value written by the old syncer and others carry one written by the readiness controller.

### Removing retained `enforce` labels on opt-out

Because the syncer is retired rather than left advising, nothing prunes the `pod-security.kubernetes.io/enforce` labels it wrote before the upgrade. Per-Namespace labels outrank the global default, so on an upgraded cluster `enforcementMode: Privileged` lowers the default and changes the effective level of nothing that already carries a label. See [Existing labels are retained, and the opt-out does not remove them](#existing-labels-are-retained-and-the-opt-out-does-not-remove-them).

The labels outlive the opt-out question, too. Since no mode restarts the syncer, a cluster that later opts back in to `Restricted` still has those labels, still unmaintained, and still outranking the level it just asked for — so whatever is decided here is the *only* mechanism by which a pre-upgrade label is ever removed.

The options are:

- **Ship nothing.** The API is honest about only governing the global default, and administrators who want the labels gone remove them themselves. Cheapest, and consistent with this enhancement being aimed at newly created clusters — but it means the opt-out does not deliver relief to the clusters that most need it.
- **A one-shot cleanup on transition to `Privileged`**, performed by the `cluster-kube-apiserver-operator` or a dedicated job rather than by the syncer, deleting only labels whose `managedFields` owner is the syncer. This is the behaviour the earlier three-mode draft attributed to the syncer's opt-out mode. The hazard is that ownership records are lost by backup/restore and etcd restore, so an ownership-based delete fails *unsafe* in a way an ownership-based write does not — the point made in [Detecting fossilised labels](#detecting-fossilised-labels).
- **A documented `oc` procedure** in [Support Procedures](#support-procedures), so the deletion is a deliberate administrator act with a visible blast radius.

Whichever is chosen, the observer lowering the global default and the labels being removed are two halves of the same change, and they are not ordered with respect to each other. Lowering first is harmless — a Namespace keeps its label and its current level until the label goes. Removing first is not: between the delete and the kube-apiserver rolling to a `privileged` revision, a Namespace that lost a `baseline` label is governed by a global default that is still `restricted`, which is *stricter* than what it had. Any cleanup mechanism therefore has to run after the observer has taken effect, not merely after the administrator has asked for it.

### `openshift-operators` has no label to freeze

Described in [The gap retiring the syncer cannot close](#the-gap-retiring-the-syncer-cannot-close): `openshift-operators` is non-exempt but prefix-skipped, so it carries no `enforce` label from any source and is enforced at `restricted` today only by the global default. Release `n` relaxes it to `privileged` with no signal, and there is no label to retain because there was never a label.

Retiring the syncer removes one of the candidate mitigations — an earlier draft suggested making the syncer manage this Namespace as a named exception to the prefix skip, and there is no longer a syncer to do it. What is left is to add it to the PodSecurity configuration's Namespace exemptions, or to have OLM ship an explicit label on the Namespace it owns. The latter is the most honest about who owns the decision but requires an OLM-side change and an OLM reviewer, neither of which is currently in this enhancement. Whichever is chosen, the release note for `n` must state the change in effective level for this Namespace explicitly.

### HyperShift configuration surface

Is the administrator-facing surface for a hosted cluster a guest-cluster `PSAEnforcementConfig` CR, or a field under `HostedCluster.spec.configuration`? The recommendation is `spec.configuration`, matching existing `ClusterConfiguration` plumbing, but this determines RBAC, tenancy and whether a tenant may change their own enforcement level at all. See [Hypershift / Hosted Control Planes](#hypershift--hosted-control-planes).

### Is the syncer deleted or merely never started

This enhancement says the `PodSecurityAdmissionLabelSynchronizationController` never runs on OpenShift, in any enforcement mode. It does not say whether the controller's code is removed from `cluster-policy-controller` or left in the tree with nothing constructing it. The two are very different commitments and the answer is not ours alone to give:

- **Deleted.** The honest expression of the design: a controller nobody may enable cannot rot, cannot be re-enabled by a well-meaning patch, and cannot accumulate a second, divergent copy of the SCC-to-PSS mapping. It also forecloses the option — there is no supported path back short of reverting a deletion.
- **Left in place, unstarted.** Keeps a revert cheap through the Tech Preview window, and keeps the code available to any consumer of the library that is not OpenShift. The cost is a large body of unreachable, untested code whose CI signal disappears the moment nothing starts it, which is how controllers quietly stop working.

The deciding input is MicroShift, per [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift). MicroShift hardcodes `enforce: restricted` and has no SCC-derived escape for a workload that cannot meet it, so it is the one topology with a live argument for keeping the syncer running. If MicroShift keeps it, the code stays and must stay tested and shared; if MicroShift follows OpenShift, deletion is available. **This enhancement should not merge as `implementable` with this question open**, because the answer changes which repositories the implementation touches and whether [Who computes the minimally sufficient standard now the syncer is retired](#who-computes-the-minimally-sufficient-standard-now-the-syncer-is-retired) is a move or a split.

### Evaluation cost and pagination

The evaluation adds a per-Namespace Pod LIST, and [Scope of the evaluation](#scope-of-the-evaluation) adds workload-template LISTs on top. These are currently unpaginated, with no per-sweep timeout, no backoff and no defined partial-completion behaviour. A truncated `violatingNamespaces` is indistinguishable from "fewer violations found", which is precisely the signal that causes an administrator to enable enforcement. Publication needs to be all-or-nothing, or explicitly marked as partial, and peak memory and CPU need quantifying for SNO and for a large cluster.

### Day 0 configuration

The Summary, Motivation and [New Installation](#new-installation) all state that enforcement can be requested at install time, but no install-config field, manifest name or bootstrap rendering path is specified, and no installer reviewer is assigned. There is also a bootstrap hazard: a day-1 manifest sets an enforcement level before the readiness controller has ever run, which is the exact situation the `Evaluated` interlock exists to prevent.

## Test Plan

### Switching between PSA modes and enable/disable

We need to be able to switch between the PSA modes (`Restricted`, `Baseline`, `Privileged`) and to opt in or out at will. Tests must cover, for each transition:

- the kube-apiserver's effective `enforce` level, read from the revisioned `config-<revision>` ConfigMap rather than from `status`, per [Support Procedures](#psaenforcementconfig-appears-to-have-no-effect);
- **that the PSA label syncer never runs**, in any mode including `Baseline` and `Restricted`. This is not a per-transition assertion so much as an invariant the transition tests must not be able to violate: the controller does not start, and switching modes does not start it. "Not running" is what distinguishes this design from advising mode, so it needs a direct test rather than being inferred from labels not changing;
- that a Namespace's existing `pod-security.kubernetes.io/enforce` label is **byte-for-byte unchanged** across every transition, including after an unrelated SCC or RBAC change to that Namespace — the churn case that advising mode got wrong, per [Freezing is not the same as removing](#freezing-is-not-the-same-as-removing) — and including on the transition *back up* to `Restricted`, which under an earlier draft would have rewritten it;
- that a Namespace with **no** `enforce` label, created while the cluster was at `Privileged`, is held to the global level as soon as the cluster opts in, with nothing computing a gentler label for it. This is the safety-net loss described in [What retiring it costs](#what-retiring-it-costs), and it should be asserted deliberately rather than discovered;
- that `warn` and `audit` stay pinned to `restricted` in the global configuration in every mode;
- that the `PodSecurityReadinessController` continues to evaluate while `Privileged` is in force, since "disabled" must not mean the diagnostics stop.

### Monitor tests for managed Namespaces

Retiring the syncer moves the assurance for OpenShift's own Namespaces from a runtime controller to CI. That assurance has to be explicit rather than assumed, because after release `n` there is nothing on a running cluster that would notice a payload Namespace shipping without a label.

- **SCC labelling (exists).** A monitor test already checks that workloads run under the SCC their Namespace's configuration implies. It is unaffected by this enhancement and must not regress once the default posture is `privileged`.
- **Managed Namespaces are labelled (new, required by this enhancement).** A monitor test asserting that every payload Namespace carries a `pod-security.kubernetes.io/enforce` label from its own manifest. This is what replaces the `AllManagedNamespacesLabeled` condition described in [There is no labelling handshake](#there-is-no-labelling-handshake) — a pre-merge check on the payload rather than a runtime handshake between two operators, which is the right place for it given that payload labels are static and ship in manifests. It must be in place before the default changes in release `n`, not at graduation.

The test cannot cover OLM, which expresses its requirement through SCC configuration rather than Namespace labels. That exclusion has to be encoded in the test rather than left implicit, and it is the same gap tracked in [`openshift-operators` has no label to freeze](#openshift-operators-has-no-label-to-freeze).

Neither test says anything about user-created Namespaces; nothing in CI can. See [Scope of the evaluation](#scope-of-the-evaluation).

### Non-urgent, but important: Hardcoded SCC to PSA mapping

The PSA label syncer maps SCCs to PSS through a hard-coded rule set, with the PSA version set to `latest`.
This setup risks becoming outdated if the mapping logic changes upstream.
To protect user workloads, an end-to-end test should fail if the mapping logic no longer behaves as expected.
Ideally the mapping would use the `podsecurityadmission` package directly.
Otherwise, it can't be guaranteed that all possible SCCs are mapped correctly.

This enhancement inherits the problem rather than introducing it, but it does move where it lives: the mapping follows the `MinimallySufficientPodSecurityStandard` computation into the `PodSecurityReadinessController`, so the test has to target whichever copies survive [Is the syncer deleted or merely never started](#is-the-syncer-deleted-or-merely-never-started). The stakes also rise. Under the syncer the mapping produced a label that was, at worst, wrong for one Namespace; under this design it produces the `reason` text and the violation verdict that an administrator reads when deciding whether opting in is safe for the whole cluster.

## Graduation Criteria
> **Draft.** The criteria below are being reworked against `dev-guide/feature-zero-to-hero.md` and currently understate the documented bar — the 14-runs-per-platform requirement is missing, the platform list is incomplete, and the "95% over 7 consecutive days" figure is not the documented window.

Graduation occurs in a tiered approach.

### Tier 1: removing `OpenShiftPodSecurityAdmission` from the `Default` feature set

`OpenShiftPodSecurityAdmission` is already in use today, and we re-use it rather than introducing a second gate for the same decision. Removing it from the `Default` feature set is what makes PSA enforcement optional, so it is the first tier of graduation.

Prerequisites for the removal:

- all OpenShift workloads and namespaces are labelled appropriately and have the correct SCC pinning, demonstrated by the managed-Namespace labelling monitor test in [Monitor tests for managed Namespaces](#monitor-tests-for-managed-namespaces) rather than by inspection. This is a hard prerequisite: once the syncer is retired and the default is `privileged`, an unlabelled payload Namespace produces no signal on a running cluster, so CI is the only place it can be caught;
- the existing SCC labelling monitor test does not regress once the default posture is no longer enforcing.

This tier changes only the default. It does not remove the enforcement code paths, and it does not depend on the `PSAEnforcementConfig` API, which is still `TechPreviewNoUpgrade` at this point.

### Tier 2: graduating the opt-in configuration

In release `n+1`, once the default has moved and clusters are running with enforcement off, we graduate the opt-in path — the `PSAEnforcementConfig` API, the `PodSecurityReadinessController` status it depends on, and the Config Observer that consumes it. The label syncer is not part of this tier; it is not a consumer of the API and is already gone by tier 1.

This is a combined API and feature-gate promotion:

- `PSAEnforcementConfig` moves from `v1alpha1` to `v1`, with the `v1alpha1` version retained and served for one release for anyone who adopted it under TechPreview.
- `PodSecurityAdmissionConfiguration` moves from `TechPreviewNoUpgrade` to `Default`.
- API validation integration tests land alongside the `openshift/api` change, as required for all API changes.

### Dev Preview -> Tech Preview

This feature does not pass through Dev Preview. The API is introduced directly as `v1alpha1` behind `PodSecurityAdmissionConfiguration` in `TechPreviewNoUpgrade` in release `n`, because the behaviour it configures — PSA enforcement — already ships and is already exercised across the fleet; what is new is the configuration surface, not the enforcement.

Entering Tech Preview requires:

- the API type merged in `openshift/api` with its feature gate, and API validation integration tests alongside it;
- the `PodSecurityReadinessController` populating `status` end to end, including the `Evaluated`, `StatusStale` and `EnforcementBlocked` conditions, and computing the minimally sufficient standard itself now that the syncer no longer supplies it;
- the Config Observer selecting the enforcement level from `status`, with the absent-CRD path exercised;
- the four metrics in [Metrics](#metrics) exposed, and the three alerts in [Alerts](#alerts) written with runbooks;
- tests labelled `[OCPFeatureGate:PodSecurityAdmissionConfiguration]` and `[Jira:"auth"]`, running in both the TechPreviewNoUpgrade and Default Prow variants.

### Tech Preview -> GA

In order for this feature to be promoted, the following is required in regards to testing:

- At least five tests are present in Sippy.
- Tests must be ran at least 7 times per week.
- Tests must run on all supported platforms on Sippy.
  - AWS, Azure, GCP, vSphere, Bare Metal.
- Each test must be individually trackable and show clear signal of success or failure.
- Tests must pass at least 95 percent of the time over 7 consecutive days.

<u>The above criteria must be met at least 14 days before branching for promotion to be accepted.</u>

Apart from test requirements, if `spec.enforcementMode = Restricted|Baseline|Privileged` can be set when the feature is enabled, and the feature can be disabled by the user at-will with no adverse effects. We can say the feature has graduated.

In addition, GA requires user-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/) covering the opt-in workflow, the diagnostics, and the break-glass procedure; and the `/test verify-feature-promotion` presubmit passing.

### Removing a deprecated feature

This enhancement removes no deprecated API. It removes a behaviour outright: the label syncer stops running in release `n` and does not run again in any configuration, and the `pod-security.kubernetes.io/enforce` labels it previously wrote stop being maintained. This is a removal, not a deprecation with a migration window — there is no supported setting under which the old behaviour returns, which is unusual enough that the release note has to say it plainly rather than describing the feature only as "enforcement is now optional".

Whether the *code* goes with the behaviour is [Is the syncer deleted or merely never started](#is-the-syncer-deleted-or-merely-never-started). Either way, the advising mode is deleted: it no longer has a caller, and removing it also removes the non-deterministic server-side-apply decay described in [Freezing is not the same as removing](#freezing-is-not-the-same-as-removing).

The labels the syncer leaves behind are not deleted with it. They are made visible through the `PodSecurityNamespaceOverRestricted` alert and handled by whatever [Removing retained `enforce` labels on opt-out](#removing-retained-enforce-labels-on-opt-out) settles on.

## Upgrade / Downgrade Strategy

### On Upgrade

#### Release plan

No change is backported to release `n-1`. An earlier draft proposed backporting the API there; that has been dropped, because release `n-1` contains no consumer of the API and because the CRD and any stored object survive a downgrade regardless of whether `n-1` shipped the type.

- Release `n`:
  - Remove `OpenShiftPodSecurityAdmission` from the `Default` feature set. This is the change that makes PSA enforcement optional: the config observer's fallback moves from `restricted` to `privileged`.
  - Stop starting the label syncer. This is a separate change in `cluster-policy-controller`, and it is unconditional — it is not gated on the feature set, not gated on the API, and not reversed by opting in. See [Why the syncer is retired outright](#why-the-syncer-is-retired-outright).
  - Ship `PSAEnforcementConfig` `v1alpha1` behind `PodSecurityAdmissionConfiguration` in `TechPreviewNoUpgrade`.
  - Enable the `PodSecurityReadinessController` to set the API's `status` — `enforcementMode`, `conditions`, `observedGeneration`, `lastEvaluationTime` and `violatingNamespaces`. It does not write `spec`.
  - Enable the `Config Observer Controller` to set the kube-apiserver's global `PodSecurity` `enforce` level from `status.enforcementMode` when it is `Baseline` or `Restricted`. `Privileged` and unset fall back to the level implied by the `OpenShiftPodSecurityAdmission` gate.
- Release `n+1`:
  - Promote `PSAEnforcementConfig` to `v1` and `PodSecurityAdmissionConfiguration` to `Default`, making the opt-in path generally available.

The `PSAEnforcementConfig` API and the controller wiring that consumes it are **not** guarded by `OpenShiftPodSecurityAdmission`; see [Role of the `OpenShiftPodSecurityAdmission` feature gate](#role-of-the-openshiftpodsecurityadmission-feature-gate). Guarding them with it would remove the opt-in path at the same moment it removes the default enforcement.

#### The state a cluster actually arrives in

A cluster upgrading from `n-1` into `n` arrives with no `PSAEnforcementConfig` object, and in the common case without the CRD either: the type ships behind `PodSecurityAdmissionConfiguration`, which is in `TechPreviewNoUpgrade` in release `n`, and a `TechPreviewNoUpgrade` cluster cannot upgrade at all. "CRD absent, no object, `spec.enforcementMode` therefore unset" is not an edge case, it is the state every upgraded cluster is in, and it is the state the design has to be correct in first.

Nothing creates the singleton. `config.openshift.io` singletons are rendered by the installer at install time; no payload operator creates one afterwards, and `cluster-config-operator`'s manifests ship no CR instances at all. The established pattern for a config type that may legitimately be missing is the one in [`ObserveMinimumKubeletVersion`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/node/observe_minimum_kubelet_version.go), which logs `NotFound` as a warning and leaves the observed config alone. This enhancement adds no controller that creates the object: administrators opt in by creating it, and an upgrade never manufactures an opinion on their behalf.

#### `observedConfig` is never empty

[`observePodSecurityAdmissionEnforcement`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/auth/podsecurityadmission.go) writes a complete six-key block at `admission.pluginConfig.PodSecurity.configuration.defaults` — `enforce`, `audit`, `warn` and their `-version` keys — as soon as the initial feature gates have been observed, choosing `privileged` or `restricted` from the gate and nothing else. Before the gates are observed it returns the previous `observedConfig` unchanged. It never returns a state in which the path is absent.

That is deliberate, not incidental: `bindata/assets/config/defaultconfig.yaml` ships all six keys as the literal string `invalid-to-force-substitution`, so a kube-apiserver whose PodSecurity block was never substituted fails to start. The constraint this places on the enhancement is hard. The observer may change *which* level it writes based on `status.enforcementMode`, but it may never decline to write one. Both "CRD absent" and "evaluation not yet complete" must resolve to the gate-implied default, which in release `n` is `privileged`.

The sequence on an upgraded cluster is therefore: the kube-apiserver rolls to a revision whose PodSecurity block says `privileged`, because `OpenShiftPodSecurityAdmission` is no longer in the `Default` feature set; the `cluster-policy-controller` restarts with the label syncer not running, for its own reason rather than because of the feature set; and nothing further happens until an administrator creates a `PSAEnforcementConfig`. There is no window in which enforcement is *raised* as a side effect of the upgrade.

#### The relaxation is not retroactive

The global default only applies to Namespaces that carry no `pod-security.kubernetes.io/enforce` label. On a cluster upgraded from `n-1` most Namespaces do carry one, written by the enforcing label syncer, so those Namespaces keep enforcing at their existing level even though the cluster-wide default has moved to `privileged`.

That is intended. Release `n` changes the default; it does not retroactively rewrite Namespaces that already have an effective level, and retiring the syncer rather than leaving it advising is what guarantees it. The reasoning, including why this is not a contradiction of the enhancement's purpose, is in [Existing labels are retained, and the opt-out does not remove them](#existing-labels-are-retained-and-the-opt-out-does-not-remove-them). The practical consequence is that an upgraded cluster is opt-in for *new* Namespaces and unchanged for existing ones until those labels are dealt with.

##### Freezing is not the same as removing

This is the concrete reason the syncer is retired rather than left running in advising mode, and it is worth spelling out because the advising path looks superficially safe.

What becomes of the labels in advising mode is decided by server-side apply, not by any cleanup code. The syncer applies with field manager `pod-security-admission-label-synchronization-controller` and rebuilds its apply configuration from scratch on every sync from its `syncedLabels` map, which in advising mode contains `warn` and `audit` only. Server-side apply prunes fields a manager previously owned and no longer sends, so the `enforce` label *is* dropped — but only on the next apply, and `shouldUpdate` skips the apply entirely unless one of the values the syncer still tracks has drifted. A Namespace whose `warn`, `audit` and `MinimallySufficientPodSecurityStandard` values are all already correct keeps its `enforce` label indefinitely. A Namespace that happens to see an unrelated SCC or RBAC change six months later loses it at that moment.

So advising mode delivers neither of the two behaviours anyone would want from it. It is not freezing, because the labels are eventually deleted; and it is not opting out, because the deletion happens at an arbitrary time per Namespace and never at all where the label is co-owned. The cluster's posture decays non-deterministically, and two identically configured clusters diverge based on unrelated churn.

Retiring the controller avoids this entirely rather than repairing it. A controller that never issues an apply never prunes, so retention is a property of not running rather than a behaviour that has to be implemented and tested against server-side apply's ownership semantics. The alternative — keeping the syncer alive and stopping it pruning, by applying under a field manager that never owned `enforce`, or by relinquishing ownership without deleting the value as the ownership-transfer sequence at [`podsecurity_label_sync_controller.go#L260-L287`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L260-L287) does — was the earlier design, and it is strictly more code for a controller that has no remaining job.

Because the syncer is retired unconditionally, this reasoning covers the opt-in case as well, and there it is doing more work than it first appears. Under a design where opting in restarted the syncer, these same server-side-apply mechanics would run in reverse: a Namespace frozen at `restricted` since before the upgrade would be *relaxed* to whatever the syncer now computed, at the arbitrary moment its next apply happened to fire. Retention would then have been a property of the cluster's configuration rather than of the labels themselves, and an administrator raising the global level could have lowered several Namespaces' effective level in the same act. Nothing in this design does that.

Because the labels are retained, the release note for `n` cannot say "PSA enforcement is now opt-in" without qualification. On an upgraded cluster it is opt-in for *new* Namespaces; existing Namespaces keep their current level until their labels are removed, and no component removes them — see [Existing labels are retained, and the opt-out does not remove them](#existing-labels-are-retained-and-the-opt-out-does-not-remove-them) and [the open question](#removing-retained-enforce-labels-on-opt-out) on whether a cleanup ships. The `PodSecurityNamespaceOverRestricted` alert described in [Removing a deprecated feature](#removing-a-deprecated-feature) makes the retained labels visible in the meantime.

#### `unsupportedConfigOverrides` already blocks the upgrade

`unsupportedConfigOverrides` is merged last and wins. `targetconfigcontroller.manageKubeAPIServerConfig` builds the kube-apiserver's `config.yaml` with `resourcemerge.MergePrunedConfigMap`, layering, in order: `defaultconfig.yaml`, the authorization-mode override, `config-overrides.yaml`, `spec.observedConfig` and finally `spec.unsupportedConfigOverrides`. Each later document overlays all previous ones. On a cluster carrying the PSA override documented in the [original Pod Security Admission enhancement](pod-security-admission.md), `spec.enforcementMode` is therefore silently inert while `status` reports success.

No new condition is needed to surface this. library-go's `UnsupportedConfigOverridesController`, which `cluster-kube-apiserver-operator` already runs through `staticpod.NewBuilder`, sets `UnsupportedConfigOverridesUpgradeable` to `False` with reason `UnsupportedConfigOverridesSet` whenever `spec.unsupportedConfigOverrides` is non-empty, and the status controller unions every `*Upgradeable` condition into the ClusterOperator's `Upgradeable`. The message enumerates the overridden leaf paths, so a cluster using the PSA override already reports `admission.pluginConfig.PodSecurity.configuration.defaults.enforce` among them.

Two things follow. First, the recovery documented in [On Downgrade](#on-downgrade) pins the cluster at `n-1` until the override is removed. That is correct behaviour and belongs in the release note rather than being worked around. Second, the existing message names the overridden path but does not tell an administrator that it is the reason their `PSAEnforcementConfig` is being ignored; that diagnosis is added to [Support Procedures](#support-procedures). The readiness controller does not attempt to detect, reconcile or remove the override.

### On Downgrade

Y-stream downgrade is not a supported OpenShift operation, so this section describes recovery rather than a guarantee.

Downgrading from release `n` to release `n-1` restores mandatory enforcement, because `n-1` still has `OpenShiftPodSecurityAdmission` in its `Default` feature set and has no controller that reads `PSAEnforcementConfig`. For a cluster that had opted out with `spec.enforcementMode: Privileged`, this silently re-enables enforcement, on a cluster that was opted out precisely because it could not survive enforcement. This is the dangerous direction and it must be called out in a release note.

The recovery on `n-1` is `unsupportedConfigOverrides` on `kubeapiservers.operator.openshift.io`, and it does work: `n-1`'s observer does not prune the PodSecurity path and then leave it for someone else to fill, it unconditionally overwrites `admission.pluginConfig.PodSecurity.configuration.defaults` with a complete `restricted` block, and the override is merged afterwards and wins. The cost is that the cluster is then pinned at `n-1` by `UnsupportedConfigOverridesUpgradeable=False` until the override is removed.

The reverse case is benign: a cluster running `Restricted` on `n` downgrades into a release that enforces `restricted` anyway.

#### Per-resource downgrade matrix

| Resource | State on `n` | What `n-1` does to it | Who cleans up | Administrator action |
|---|---|---|---|---|
| `observedConfig` → `admission.pluginConfig.PodSecurity.configuration.defaults` | `privileged`, or the level from `status.enforcementMode` | `n-1`'s observer unconditionally rewrites the whole six-key block to `restricted` and rolls a new kube-apiserver revision | self-healing, no cleanup needed | none, unless enforcement must stay off — then apply the override above |
| Namespace `pod-security.kubernetes.io/{enforce,warn,audit}` and `-version` labels | all unmaintained but intact, whatever the enforcement mode — the syncer is not running, so it neither writes nor prunes | `n-1`'s syncer runs enforcing, so it re-adds and re-owns all three at the minimally sufficient level on every Namespace it controls | the label syncer, via server-side apply | none for syncer-controlled Namespaces; Namespaces opted out with `security.openshift.io/scc.podSecurityLabelSync: false`, or whose labels are owned by another field manager, keep whatever `n` left and must be fixed by hand |
| Namespace `security.openshift.io/MinimallySufficientPodSecurityStandard` | stale, in every mode — the syncer that wrote it is not running on `n` at all, and the readiness controller's replacement computation may or may not write it back to the Namespace (see [the open question](#who-computes-the-minimally-sufficient-standard-now-the-syncer-is-retired)) | `n-1`'s syncer runs enforcing and rewrites it | the label syncer, on `n-1` | none |
| Pod `openshift.io/scc`, `security.openshift.io/validated-scc-subject-type` | set by SCC admission at creation time | nothing; they are per-Pod and immutable after admission | recreated Pods only | none — these are inputs to the readiness evaluation, not to enforcement, so staleness is harmless on `n-1` |
| `PSAEnforcementConfig` CRD and singleton | present, `status` current | CVO does not delete CRDs on downgrade, so both persist with no controller reading either | nobody | optionally delete the object; leaving it is inert but its `status` goes stale |
| `kubeapiservers.operator.openshift.io` `status.conditions` added by this enhancement | `Evaluated`, `StatusStale` and the readiness conditions | `n-1` has no controller that writes the condition types introduced in `n`, and conditions are not garbage collected | nobody | see below |

That last row is the one that bites. A condition left behind by a controller that no longer exists is not cleared by anything, and the status controller unions every `*Upgradeable` and `*Degraded` condition it finds in the operator CR into the ClusterOperator. A `Degraded=True` written by the readiness controller in the seconds before a downgrade would therefore make the `kube-apiserver` ClusterOperator permanently `Degraded` on `n-1`, with no component able to explain why. Two rules follow, and they constrain the API design in [The `PSAEnforcementConfig` type](#the-psaenforcementconfig-type):

- Condition types introduced by this enhancement live on the `PSAEnforcementConfig` object wherever possible, not on `kubeapiservers.operator.openshift.io`, so that deleting the object disposes of them.
- Where a condition must live on the operator CR — `Degraded` for a repeatedly failing evaluation — removing it by hand is a documented support step, listed in [Support Procedures](#support-procedures).

Finally, re-upgrading `n-1` → `n` finds the persisted object with a `status` written by the previous `n`. That `status` must not be treated as current; the `Evaluated` condition and `lastEvaluationTime` exist for exactly this, and the interlock is described in [Version Skew Strategy](#version-skew-strategy).

## Version Skew Strategy

Four skew windows matter here. Three are windows in which two components disagree about the effective PSA level; the fourth is a window in which the evaluation this enhancement depends on never happens.

### Intra-rollout skew

The PodSecurity configuration is part of the kube-apiserver's config, so changing it produces a new static pod revision, and revisions roll one master at a time. For the duration of the rollout — minutes to tens of minutes — one kube-apiserver enforces the new level while the others still enforce the old one, and which one a Pod creation reaches is a load-balancer decision. A Deployment scaling from 0 to 5 can have some replicas admitted and some rejected. This window exists on every change, in both directions.

The two directions are not symmetric, and this enhancement deliberately makes only one of them safe by construction:

- **Raising** enforcement (`Privileged` → `Baseline`/`Restricted`) is gated on `Evaluated` reporting no violating Namespaces, so by the time the rollout starts the expected number of Pods that the stricter apiservers would reject is zero. The interlock is not only a pre-flight check for the administrator; it is what makes the mixed window tolerable. A workload that slips through anyway — created between the evaluation and the rollout — hits the same non-determinism, which is the residual risk accepted in [Risks and Mitigations](#risks-and-mitigations).
- **Lowering** enforcement, including the break-glass path, is *not* instantaneous. Until the last master has taken the new revision, some fraction of Pod creations is still rejected. Support must be told to wait for the rollout rather than concluding that the change did not take:

  ```bash
  oc get kubeapiserver/cluster -o jsonpath='{.status.latestAvailableRevision}{"\n"}'
  oc get kubeapiserver/cluster -o jsonpath='{range .status.nodeStatuses[*]}{.nodeName}{"\t"}{.currentRevision}{"\n"}{end}'
  ```

  The change has fully landed only when every `currentRevision` equals `latestAvailableRevision`.

On single-node deployments there is no mixed window, because there is only one kube-apiserver; there is an API outage instead while the static pod restarts. See [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift).

### Operator skew

Retiring the syncer removes most of this problem. The producer of `status.enforcementMode` (the `PodSecurityReadinessController`) and its only consumer (the config observer) are both in `cluster-kube-apiserver-operator`, so they ship and roll together and cannot disagree about the API. `cluster-policy-controller` reads none of it.

What remains is a transient during the upgrade itself. `cluster-policy-controller` runs as a container in the kube-controller-manager static pod, owned by `cluster-kube-controller-manager-operator`, and there is no ordering guarantee against `cluster-kube-apiserver-operator`. So for part of every upgrade the kube-apiserver has already rolled to `privileged` while an `n-1` `cluster-policy-controller` is still running the syncer and still stamping `enforce` labels on Namespaces it manages — or, in the other order, the syncer has already stopped while the kube-apiserver is still enforcing `restricted`. Neither is harmful: the first writes labels that are merely redundant and that become the retained labels discussed in [The relaxation is not retroactive](#the-relaxation-is-not-retroactive); the second changes nothing, because Namespaces keep the labels the syncer last wrote. The window is bounded by the upgrade and closes on its own.

The enum is the sharper edge, and it applies to any out-of-payload or hosted-control-plane consumer rather than to `cluster-policy-controller`. `Baseline` is a value this enhancement introduces. A consumer from an earlier release that encounters it must **preserve its existing behaviour**, not error and not treat the field as unset: treating an unknown value as unset would silently relax enforcement, and erroring would degrade a ClusterOperator over a value that is valid on the cluster's own API server. Concretely:

- Consumers switch on the values they know and fall through to "make no change to the currently effective level" for anything else.
- The `enforcementMode` enum is only ever extended, never has a value removed or repurposed, for the reasons in [API Extensions](#api-extensions).
- Because the API server validating the write is always at least as new as any consumer, a value can be accepted by validation and be unknown to a consumer. That asymmetry is the normal case during an upgrade, not a bug to be designed out.

### HyperShift skew

HyperShift has no `cluster-kube-apiserver-operator`; the management-cluster `control-plane-operator` renders the hosted kube-apiserver's configuration directly, and it carries its own copy of the privileged-versus-restricted decision. The management cluster is required to be at or ahead of the hosted cluster's release, so the skew is one-directional and can span several releases: a `control-plane-operator` at `n+2` may be rendering the kube-apiserver for a hosted cluster whose payload — and whose `PSAEnforcementConfig` consumers — is at `n`.

This means the hosted cluster's opt-in cannot be expressed purely in the guest without the management side honouring it, and the management side is the component that may be arbitrarily newer. Which side owns the configuration surface is [an open question](#hypershift-configuration-surface) and is the main unresolved dependency in this section; see [Hypershift / Hosted Control Planes](#hypershift--hosted-control-planes).

### EUS-to-EUS skip upgrades

`PodSecurityReadinessController` resyncs every four hours (`checkInterval = 240 * time.Minute`), with a throttled client whose sweep can itself take a long time on a cluster with many Namespaces. A 5.0 → 5.1 → 5.2 EUS-to-EUS upgrade passes through 5.1 in well under four hours, so the controller may complete no full sweep at all in the skipped release. The library-go controller factory does run one sync immediately on start, so the operator restarting into 5.1 does kick off an evaluation — but on a large cluster that evaluation may still be in flight when the cluster leaves 5.1.

The interlock is therefore defined over the evaluation, not over the release:

- `Evaluated` is `True` only when a sweep has run to completion, and `lastEvaluationTime` records when. Neither is inherited from a `status` written by a different payload; re-entering a release, or arriving from a skip, starts from `Evaluated=False`.
- Enforcement is never raised on the strength of an evaluation that did not complete. The failure mode of a skipped release is that the cluster arrives at 5.2 still `Privileged`, which is safe and visible, rather than arriving enforced on the basis of a partial sweep.
- Conversely, nothing about the skip re-raises enforcement on its own. The gate is out of the `Default` feature set in both 5.1 and 5.2, so the cluster-wide default stays `privileged` across the whole skip, and the per-Namespace labels behave as described in [The relaxation is not retroactive](#the-relaxation-is-not-retroactive).

## Operational Aspects of API Extensions

### In general

- Administrators facing issues in a cluster already set to a stricter enforcement can change `spec.enforcementMode` to `Privileged` to halt enforcement for other clusters.
- ClusterAdmins must ensure that directly created workloads (user-based SCCs) have correct `securityContext` settings.
  Updating default workload templates can help.
- The evaluation of the cluster happens once every 4 hours with a throttled client in order to avoid a denial of service on clusters with a high amount of Namespaces.
  It could happen that it takes several hours to identify a violating Namespace.
- Raising the enforcement level never evicts a running workload. PSA is a validating admission plugin, so it acts only on Pod creation; existing Pods keep running until something recreates them. The corollary is that a workload can be admitted today and rejected weeks later when a node drain, upgrade or crash-loop restart recreates it — see [Scope of the evaluation](#scope-of-the-evaluation).
- **PSA denials are not visible in `kubectl get pods`.** When a Pod is created by a controller, the rejection surfaces on the owning object rather than on a Pod, because no Pod is ever created. For a Deployment the message appears in the events and status of its ReplicaSet, and for a CronJob on its Job:

  ```bash
  kubectl -n $NAMESPACE describe replicaset $NAME
  kubectl -n $NAMESPACE get events --field-selector reason=FailedCreate
  ```

  This is the most common source of confusion when debugging PSA, and support should reach for it first.
- To identify specific problems in a violating Namespace, administrators can query the kube-apiserver:

  ```bash
  kubectl label --dry-run=server --overwrite ns/$NAMESPACE \
      pod-security.kubernetes.io/enforce=$MINIMALLY_SUFFICIENT_POD_SECURITY_STANDARD
  ```

  Note that server-side dry run reports only on Pods that exist at that moment; it says nothing about workloads that are scaled to zero or not yet created.

### Health and failure modes of the extension

`PSAEnforcementConfig` introduces no webhook and no aggregated apiserver, so it adds no latency to any request path and cannot make another resource unavailable. Its failure modes are those of the controller behind it:

| Failure mode | Signal | Effect |
|---|---|---|
| readiness controller never runs | `Evaluated=False`, reason `NeverRan` | enforcement cannot be raised; existing enforcement unaffected |
| readiness controller stops running | `StatusStale=True`; `PodSecurityReadinessEvaluationStale` | enforcement cannot be raised; existing enforcement is deliberately left in place |
| readiness controller fails repeatedly | `kube-apiserver` ClusterOperator `Degraded=True`, reason `PodSecurityReadinessEvaluationFailing` | as above, plus a fleet-visible signal |
| CRD absent | none | every consumer falls back to the feature-gate-implied default |
| `unsupportedConfigOverrides` set | `kube-apiserver` ClusterOperator `Upgradeable=False`, reason `UnsupportedConfigOverridesSet`, message naming the overridden paths | `spec.enforcementMode` is silently inert while `status` reports success; the cluster cannot upgrade until the override is removed |

The scale characteristics are bounded by one singleton CR per cluster, so the extension does not affect general API throughput. The per-sweep cost of the evaluation is the part that scales with cluster size and is still to be quantified; see [Evaluation cost and pagination](#evaluation-cost-and-pagination).

Escalation for these failure modes goes to the OpenShift Auth team. Failures that manifest as workload admission rejections in `openshift-*` Namespaces are likely to involve OLM and the owning layered product; see [Risks and Mitigations](#risks-and-mitigations).

## Support Procedures

This section is incomplete. What exists below is administrator remediation for a Namespace they have already been told about; procedures for the failure of this feature's own machinery still need writing, specifically:

- symptoms and log lines for the readiness controller not running, crash-looping, or failing to evaluate;
- the exact PSA denial message, and the fact that it surfaces on the ReplicaSet or Job rather than on a Pod;
- audit-log correlation via `pod-security.kubernetes.io/enforce-policy` annotations;
- must-gather coverage — whether the new CR, the effective admission configuration and the violating-Namespace list are collected;
- links to the runbooks required by the three alerts.

### `PSAEnforcementConfig` appears to have no effect

The symptom is `spec.enforcementMode` set to a level the cluster is plainly not applying, while `status` reports the change as successful. The causes are distinguished by reading the **effective** configuration rather than the operator's `status`.

`targetconfigcontroller` writes the merged result into the `config` ConfigMap in `openshift-kube-apiserver`, under the key `config.yaml`. Despite the key's name the value is JSON, because the merge encodes through `UnstructuredJSONScheme`, so `jq` reads it directly:

```bash
oc get cm config -n openshift-kube-apiserver -o jsonpath='{.data.config\.yaml}' \
    | jq '.admission.pluginConfig.PodSecurity.configuration.defaults'
```

That ConfigMap is the *desired* configuration. `config` is a revisioned resource: the installer copies it to `config-<revision>`, where the revision is the monotonically increasing integer that each master reports in `status.nodeStatuses[].currentRevision`. To see what a particular kube-apiserver is enforcing right now, read the revision that master is actually on:

```bash
# e.g. master-0 currentRevision 12 -> configmap config-12
oc get kubeapiserver/cluster \
    -o jsonpath='{range .status.nodeStatuses[*]}{.nodeName}{"\t"}{.currentRevision}{"\n"}{end}'

oc get cm config-12 -n openshift-kube-apiserver -o jsonpath='{.data.config\.yaml}' \
    | jq '.admission.pluginConfig.PodSecurity.configuration.defaults'
```

If the effective block does not match `spec.enforcementMode`:

1. **The rollout has not finished.** Compare `status.latestAvailableRevision` against each entry in `status.nodeStatuses[].currentRevision` on `kubeapiserver/cluster`, as described in [Intra-rollout skew](#intra-rollout-skew). While they differ, `cm/config` and `cm/config-<currentRevision>` disagree by design, and the cluster genuinely has two answers depending on which kube-apiserver a request reaches.
2. **`unsupportedConfigOverrides` is winning.** It is the last layer in the merge, so it silently defeats the API:

   ```bash
   oc get kubeapiserver/cluster -o jsonpath='{.spec.unsupportedConfigOverrides}{"\n"}'
   ```

   The `kube-apiserver` ClusterOperator will also be `Upgradeable=False` with reason `UnsupportedConfigOverridesSet` and a message enumerating the overridden paths. That condition is the reliable fleet-wide signal, but it does not say that a `PSAEnforcementConfig` is being ignored — making that connection is the point of this procedure. The only fix is to remove the override; nothing in this enhancement reconciles or overrides it.

### Clearing conditions left behind by a downgrade

After a downgrade from `n` to `n-1`, condition types introduced in `n` remain on `kubeapiservers.operator.openshift.io` with no controller writing them, and are not garbage collected. Because the status controller unions every `*Degraded` and `*Upgradeable` condition into the ClusterOperator, a condition captured at the moment of downgrade can leave `kube-apiserver` permanently `Degraded` or `Upgradeable=False` for a reason no running component can explain. The signature is a condition whose `lastTransitionTime` predates the downgrade and whose type is unknown to the running release. Remove it explicitly:

```bash
oc patch kubeapiserver/cluster --type=json --subresource=status \
    -p '[{"op":"remove","path":"/status/conditions/<index>"}]'
```

See the [per-resource downgrade matrix](#per-resource-downgrade-matrix) for why this is not self-healing.

### Break glass: disabling enforcement

The escape hatch is `spec.enforcementMode: Privileged`, applied unconditionally and never gated on an evaluation. The full procedure — the exact `oc patch`, the expected time to effect, the fact that it requires a kube-apiserver revision rollout and what that costs on SNO, that running Pods are unaffected, and the `unsupportedConfigOverrides` fallback for when the `cluster-kube-apiserver-operator` itself is wedged — still needs to be written out.

### Resolving Violating Namespaces

There are various reasons why the built-in solution can't set the PSS properly in the Namespace.

#### Namespace name starts with `openshift`

*Hint: The `openshift` prefix is reserved for OpenShift and these namespaces should have their own `pod-security.kubernetes.io/enforce` label.* set by OpenShift developers

The Namespace that is listed as violating has a name that starts with `openshift`.
It happens that guides or scripts create Namespaces with the `openshift` prefix.
Another root cause is that the team that owns the Namespace did not set the required PSA labels.
This should not happen, and could indicate that not the newest version is being used.

To solve the issue:

- If the Namespace is being created by the user:
  - it isn't supported that a user creates a Namespace with the `openshift-` prefix and
  - the user should recreate the Namespace with a different name or
  - if not possible, set the `pod-security.kubernetes.io/enforce` label manually.
- If the Namespace is owned by OpenShift:
  - Check for updates.
  - If up to date: report as a bug.

#### `pod-security.kubernetes.io/enforce` label has not been set

All namespaces should be labelled as appropriate. If this is not the case, see below:

To assess if your Namespace is capable of running with the `Restricted` PSS, run this:

```bash
  kubectl label --dry-run=server --overwrite ns/$NAMESPACE \
      pod-security.kubernetes.io/enforce=restricted
```

To assess if your Namespace is capable of running with the `Baseline` PSS, run this:

```bash
  kubectl label --dry-run=server --overwrite ns/$NAMESPACE \
      pod-security.kubernetes.io/enforce=baseline
```

If both commands return warning messages, the Namespace needs `Privileged` PSS in its current state.
It can be useful to read the warning messages to identify fields in the Pod manifest that could be adjusted to meet a higher security standard.

To set the label, remove the `--dry-run=server` flag.

#### Namespace workload doesn't use ServiceAccount-based SCC (user-based SCCs)

It can be, that a Namespace workload doesn't use ServiceAccount SCC, but receives the SCCs of the executing user.
This usually happens, when a workload isn't running through a deployment with a properly set up ServiceAccount.
This can be verified by checking the `security.openshift.io/validated-scc-subject-type` annotation on the Pod manifest.
Another way would be to check the `openshift.io/scc` value and check if the current ServiceAccount for the workload is capable of that SCC.

To solve the issue:

- Update the ServiceAccount to be able to use the necessary SCCs.
	The necessary SCC can be identified in the annotation `openshift.io/scc` of the existing workloads.
	This is enough for the next evaluation to stop reporting the Namespace as violating, in any enforcement mode. It does **not** cause a PSA label to be written: the syncer that used to do that no longer runs on any cluster, so granting the SCC changes what the Namespace is *allowed* to run without changing what PSA *enforces* on it.
- Therefore, if the Namespace needs an effective level other than the cluster-wide one, set the `pod-security.kubernetes.io/enforce` label manually. On a cluster that has opted in to `Baseline` or `Restricted` this is the only mechanism — an unlabelled Namespace is held to the global level with nothing computing a gentler one for it.

## Infrastructure Needed

New periodic CI lanes are needed to satisfy the graduation criteria: the feature has to be exercised in both the `TechPreviewNoUpgrade` and `Default` Prow variants, across the provider, topology, architecture and network variants required by `dev-guide/feature-zero-to-hero.md`, at a frequency that reaches the required number of runs per platform before branch cut. The exact lane list follows from the reworked [Graduation Criteria](#graduation-criteria) and is not yet enumerated.
