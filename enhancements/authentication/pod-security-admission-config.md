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

### This Changes the Default Security Posture

**PSA enforcement is enabled by default on OpenShift today, and this enhancement turns it off by default and makes it opt-in.**

Concretely, the `OpenShiftPodSecurityAdmission` feature gate is currently enabled in the `Default` feature set ([`features.go#L108-L114`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/features/features.go#L108-L114)), which causes:

- the kube-apiserver's global PodSecurity configuration to be set to `enforce: restricted` ([`podsecurityadmission.go#L99-L111`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L99-L111)), and
- the PSA label syncer to run in **enforcing** mode, writing `pod-security.kubernetes.io/enforce` on the Namespaces it manages ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/cmd/controller/psalabelsyncer.go#L17-L50)).

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

When the feature is enabled, `Privileged` PSS will be the default for managed namespaces. The `PodSecurityReadinessController` will then evaluate managed namespaces. When the namespaces and their inherent workloads are deemed compliant, the `PodSecurityReadinessController` will move the compliant namespaces to PSS set by the user (Privileged|Baseline|Restricted).

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

## Design Details

The design details will describe how to support upgrading clusters to reach optional PSA enforcement.

### Improved Diagnostics

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

#### Existing Building Blocks (Already Shipped)

The diagnostic improvements described in this section are **already implemented and merged**; they are documented here because the rest of this enhancement builds directly on them.
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

### New API

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

#### API version and availability

The API ships as `v1alpha1` behind the `PodSecurityAdmissionConfiguration` feature gate, which is in `TechPreviewNoUpgrade` in release `n` and promoted to `Default` in release `n+1` once the graduation criteria below are met.

This means the API is **not** available on a supported, upgradeable cluster in release `n`. That is an accepted consequence, because the two changes are needed in different releases:

- Release `n` has to deliver *PSA enforcement is off by default*, which is achieved entirely by removing `OpenShiftPodSecurityAdmission` from the `Default` feature set and requires no new API.
- Release `n+1` delivers *and here is the supported way to turn it back on*.

Turning enforcement **off** is the safe direction, so release `n` needs no opt-out mechanism. An administrator on release `n` who wants enforcement back on before the API is generally available can set it through `unsupportedConfigOverrides` on the `kubeapiservers.operator.openshift.io` resource, as documented in the [original Pod Security Admission enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/pod-security-admission.md). This is unsupported and must be removed before upgrading to release `n+1`; see [Operational Aspects](#operational-aspects).

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
	// - Privileged opts out of PSA enforcement.
	// - Baseline enables the cluster to partially enforce PSA.
	// - Restricted enables the cluster to completely enforce PSA.
	//
	// The requested mode is only applied once the PodSecurityReadinessController
	// has confirmed the cluster has no violating Namespaces. Until then,
	// status.enforcementMode reports what is actually in force, the
	// EnforcementBlocked condition explains why, and status.violatingNamespaces
	// lists what must be resolved.
	//
	// When omitted, this means the user has no opinion and the platform chooses
	// a default, which is subject to change over time. The current default is
	// Privileged.
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
	// It contains a prefix, indicating, which part of PSA validation is conflicting:
	// - the global configuration, which will be set to `Restricted` or
	// - the PSA label syncer, which tries to infer the PSS from the SCCs available to ServiceAccounts in the Namespace.
	//
	// Possible values are:
	// - PSAConfig: Misconfigured OpenShift Namespace
	// - PSAConfig: PSA label syncer disabled
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

If a user encounters `status.violatingNamespaces` when PSA is enabled and configured, they are expected to:

- resolve the violations in the Namespaces, after which the requested mode takes effect on the next evaluation with no further user action, or
- set `spec.enforcementMode=Privileged` and solve the violating Namespaces later.

Because the controller never writes `spec`, a user who requested `Restricted` and hit violations keeps that request on record. The cluster reports `status.enforcementMode: Privileged` with `EnforcementBlocked=True` until the violations are resolved, and then transitions to `Restricted` on its own.

As this is an optional feature, the feature can be disabled if needed e.g. if the user manages several clusters and there are well known violating Namespaces. This can be done by setting `.spec.enforcementMode` to `Privileged`, or by omitting the field.

### Implementation Details

- The `PodSecurityReadinessController` in the `cluster-kube-apiserver-operator` will manage the new API.
- The [`Config Observer Controller`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L135) must be updated to derive the kube-apiserver's `PodSecurity` configuration from the new API's `status`.
- The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50) must be updated to select its enforcing or advising mode from the new API's `status`.
- Disabling PSA enforcement on a running cluster is done by setting `spec.enforcementMode` to `Privileged` (or omitting the field). It is not done by changing the cluster's `FeatureSet`.

#### Role of the `OpenShiftPodSecurityAdmission` feature gate

`OpenShiftPodSecurityAdmission` already exists and today means exactly one thing: *PSA enforcement is on by default*.
It is in the `Default` feature set, which is why the config observer emits `enforce: restricted` and the label syncer runs in enforcing mode on every cluster.

This enhancement keeps that meaning and removes the gate from the `Default` feature set. That removal *is* the mechanism by which PSA enforcement becomes optional — it flips the fleet's default to `Privileged` and the label syncer to advising mode.

Two consequences follow, and both are load-bearing:

- **Feature gate membership is a property of the payload, not a cluster setting.** Which gates are on in a feature set is compiled in at build time ([`payload-manifests/featuregates/`](https://github.com/openshift/api/tree/de86ee3bf48122ecb00fde7287aa633642ddc215/payload-manifests/featuregates)); an administrator selects a `FeatureSet`, not individual gates. The only feature set permitting per-gate control is `CustomNoUpgrade`, which is documented as unsupported, irreversible, and upgrade-blocking ([`types_feature.go#L51-L54`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/config/v1/types_feature.go#L51-L54)). No supported administrator workflow enables or disables this gate, so no part of this design may depend on one.
- **The opt-in machinery must not sit behind this gate.** The `PSAEnforcementConfig` API and the observer and syncer wiring that reads its `status` are guarded by their own gate, `PodSecurityAdmissionConfiguration` (name provisional), on their own graduation schedule. If they were guarded by `OpenShiftPodSecurityAdmission`, removing that gate from `Default` would remove the only means of turning enforcement back on along with the enforcement itself.

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

The alerting described in [Operational Aspects](#operational-aspects) requires signals that do not exist today. The controller exposes:

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

##### Namespace labelling coverage

The controller also answers, during the same sweep, whether every Namespace managed by the `PodSecurityAdmissionLabelSynchronizationController` currently carries a `pod-security.kubernetes.io/enforce` label, and publishes the answer as the `AllManagedNamespacesLabeled` condition.

This check deliberately lives here rather than in the Config Observer. The controller already lists every Namespace on its normal sweep and holds a throttled client sized for that work, whereas an observer has no Namespace informer and would have to acquire one. More importantly, an observer re-renders the kube-apiserver config whenever any of its inputs change, and every change to that config cuts a new static pod revision that rolls the control plane one node at a time. Newly created Namespaces are unlabelled for the short window before the syncer reaches them, so computing the check in the observer would flip its output on ordinary Namespace creation and roll the control plane each time — continuously, on a cluster whose workloads create Namespaces in a loop, and as an API outage on Single Node OpenShift.

Publishing it as a condition means the observer reads one field on one object and re-renders only when the answer genuinely changes.

##### Scope of the evaluation

PSA is a validating admission plugin. It runs only when something attempts to **create** a Pod; it never re-evaluates Pods that are already running, and raising the enforcement level never evicts a running workload.

A sweep that inspects only the Pods that exist at that moment therefore produces a point-in-time snapshot, not a guarantee. Workloads that are absent from the snapshot, or that are recreated after it, are unaffected by a clean result:

- a Deployment scaled to zero, a CronJob that has not yet fired, or a DaemonSet whose nodes are cordoned — no Pods exist to inspect;
- **an already-running violating Pod that is recreated** by a node drain, reboot, upgrade, eviction or crash-loop restart. Recreation is a new Pod creation, so it is admitted against the current enforcement level for the first time. A routine MachineConfig rollout weeks after enforcement was enabled can take down a workload that the evaluation reported as clean;
- any new workload in an unlabelled Namespace once enforcement is on — the gap this enhancement opens by moving the label syncer to advising mode, since those Namespaces no longer receive a computed enforce label.

Two mechanisms close this gap and are both in scope:

- **Evaluate workload templates, not only live Pods.** `Deployment`, `StatefulSet`, `DaemonSet`, `Job`, `CronJob`, `ReplicationController` and `DeploymentConfig` each describe the Pod that will be created, so the same check can run against `spec.template` whether or not a Pod exists today. This is what covers the scaled-to-zero, not-yet-fired and drain-and-reschedule cases, because the template is what gets recreated.
- **Keep `warn` and `audit` at the target standard while `enforce` remains `privileged`.** Every creation that *would* be rejected is then surfaced continuously and nothing is blocked, turning a single snapshot into a running record. This is the mechanism the [original Pod Security Admission enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/pod-security-admission.md) used for the same purpose, and the machinery already exists.

#### PodSecurity Configuration

A Config Observer in the `cluster-kube-apiserver-operator` manages the Global Config for the kube-apiserver.

There is no "unset" option for this configuration. [`defaultconfig.yaml#L12-L22`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/config/defaultconfig.yaml#L12-L22) hardcodes `enforce: "invalid-to-force-substitution"`, so the observer must always substitute a concrete level or the kube-apiserver receives an invalid admission configuration. Accordingly [`podsecurityadmission.go#L99-L111`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L99-L111) has exactly two branches today — `privileged` and `restricted` — and no third state. "Disabling PSA" means substituting `privileged`, not omitting the configuration.

The Config Observer's inputs are:
- `status.enforcementMode` — the administrator's request, after the readiness controller has resolved it;
- the `FeatureGate` `OpenShiftPodSecurityAdmission` — used only to pick the level substituted when the administrator has expressed no opinion. While the gate is in `Default` that fallback is `privileged` once the gate leaves `Default`, and `restricted` until then, preserving today's behavior across the transition;
- the `AllManagedNamespacesLabeled` condition on the `PSAEnforcementConfig` `status`, computed by the `PodSecurityReadinessController` as described above. The observer does not list Namespaces itself.

The observer substitutes a level above the fallback only when all of the following hold:

- `status.enforcementMode` is `Baseline` or `Restricted`;
- `Evaluated` is `True` and `StatusStale` is `False`, so the result rests on a real and recent sweep rather than on a controller that has never run — see [Freshness is an interlock, not a hint](#freshness-is-an-interlock-not-a-hint);
- `AllManagedNamespacesLabeled` is `True`.

Substituting `privileged` is subject to none of these conditions. Lowering enforcement must work when the readiness controller is broken, because that is precisely when an administrator is most likely to need it.

This enhancement changes only the `enforce` key. Both existing branches pin `audit` and `warn` to `restricted` regardless of the enforcement level ([`podsecurityadmission.go#L32-L39`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L32-L39), [`#L50-L57`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L50-L57)) and that is retained deliberately: it is what keeps violations observable while `enforce` is `privileged`, per [Scope of the evaluation](#scope-of-the-evaluation). The corresponding `*-version` keys are unchanged.

`Baseline` has no implementation today — only the privileged and restricted helpers exist — so a third branch must be added.

This state must be watched continuously.
If `status.enforcementMode` returns to `Privileged`, the observer applies that immediately and unconditionally — the escape hatch is never gated on the labelling precondition.

#### PodSecurityAdmissionLabelSynchronizationController

The [PodSecurityAdmissionLabelSynchronizationController (PSA label syncer)](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) will be retired as its primary use was with this feature being non-optional in mind. As it is now going to be optional, the PSA Label Syncer's future purpose will be assisting with monitoring and testing of openshift managed workloads and namespaces instead. The below is kept for context on this. Please read with this in mind.

The [PodSecurityAdmissionLabelSynchronizationController (PSA label syncer)](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) labels all the Namespaces it manages.
Without PSA enforcement it sets the `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit` labels on managed Namespaces.

Namespaces that are **managed** by the `PodSecurityAdmissionLabelSynchronizationController` are Namespaces that:

- are not named `kube-node-lease`, `kube-system`, `kube-public`, `default` or `openshift` and
- are not  prefixed with `openshift-` and
- have no `security.openshift.io/scc.podSecurityLabelSync=false` label set and
- at least one PSA label (including `pod-security.kubernetes.io/enforce`) isn't set by the user or
- if the user sets all PSA labels, it also has set the `security.openshift.io/scc.podSecurityLabelSync=true` label.

The PSA label syncer's choice between enforcing and advising mode must be driven by `status.enforcementMode`.
Today that choice is made once at construction from the `OpenShiftPodSecurityAdmission` gate ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50)), with enforcing as the default branch.
The gate continues to supply that default for clusters where the administrator has expressed no opinion; `status.enforcementMode`, when set, takes precedence over it.

In enforcing mode — `status.enforcementMode` of `Baseline` or `Restricted` — the syncer labels managed Namespaces with `pod-security.kubernetes.io/enforce` as evaluated in the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation.
In advising mode it sets only `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit`.

This state needs to be watched continuously.
If `status.enforcementMode` returns to `Privileged`, the syncer must stop writing `pod-security.kubernetes.io/enforce` on managed Namespaces.

An earlier draft required release `n-1` to be able to remove the `pod-security.kubernetes.io/enforce` labels set in release `n`. That requirement has been dropped: the label is not introduced by release `n`. It is written today, by the syncer's enforcing default, on every cluster with `OpenShiftPodSecurityAdmission` in `Default`. Release `n` stops writing it on newly evaluated Namespaces rather than starting to.

What release `n` does inherit is the labels already on upgraded clusters, which no code path currently removes. That is an existing-cluster migration question, addressed next.

### Existing Clusters

Every cluster running today has `OpenShiftPodSecurityAdmission` in its `Default` feature set, and the label syncer's **enforcing** constructor is the default branch ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50)) — advising mode requires explicitly passing `OpenShiftPodSecurityAdmission=false`. So `pod-security.kubernetes.io/enforce` labels are already present across the fleet, applied via server-side apply under the field manager `pod-security-admission-label-synchronization-controller` ([`podsecurity_label_sync_controller.go#L386`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L386)).

Moving the syncer to advising mode stops it writing new labels. It does not remove the existing ones, and no code path in any component removes them.

#### The labels are retained

Deleting them on upgrade would silently reduce the enforcement posture of clusters that are relying on it, which is a compliance event for customers under FedRAMP, PCI or DISA STIG. Retaining them is the safer default and is what this enhancement does.

Retaining them unchanged, however, introduces a regression of its own. Consider a cluster upgraded into release `n`, where `some-namespace` is labelled `pod-security.kubernetes.io/enforce: restricted` by the syncer before the upgrade:

1. An administrator grants that Namespace's ServiceAccount the `anyuid` SCC, expecting to run a workload that needs a fixed UID.
2. Before release `n`, the syncer would have recomputed the Namespace's minimally sufficient standard as `baseline` and relaxed the label accordingly.
3. In release `n` the syncer no longer writes the enforce label, so it stays pinned at `restricted`.
4. The workload is rejected at admission, on a cluster whose administrator never opted into this feature and has no reason to associate the failure with an upgrade.

The label is now a fossil: it records a decision made by a controller that has stopped maintaining it.

#### Annotation-only mode

To prevent that, the syncer is not switched off. It continues to run in **annotation-only** mode: it keeps computing each Namespace's minimally sufficient standard and writing it to the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation, while no longer writing `pod-security.kubernetes.io/enforce`.

Two changes are required beyond the mode switch:

- **The annotation write is decoupled from the label write.** Today both happen in the same apply and `sync` returns early for Namespaces where `isNSControlled` is false ([`podsecurity_label_sync_controller.go#L204`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L204)), so uncontrolled Namespaces — including every `openshift-`-prefixed one — receive no annotation at all and have no computed baseline to compare against. The annotation must be written independently of whether the syncer owns the Namespace's labels.
- **This does not alter enforcement anywhere.** The annotation is diagnostic and is not read by any admission path. In particular, payload Namespaces set their PSA labels explicitly in their own CVO manifests — `openshift-kube-apiserver-operator` is pinned `restricted` and `openshift-kube-apiserver` `privileged` — so the `Restricted`-by-default expectation that OpenShift components are developed against is held by those manifests, not by the syncer or the global default, and is unaffected. For such Namespaces the annotation may report a *lower* minimally sufficient standard than the label enforces. That is informational and must not be read as a recommendation to relax a deliberately pinned Namespace.

#### Detecting fossilised labels

With the annotation maintained everywhere, a Namespace whose enforce label is more restrictive than its computed minimum is detectable. This drives a metric and an alert, so the `anyuid` case above surfaces to the administrator instead of being debugged cold.

The alert must not fire on labels an administrator set deliberately. Ownership is recorded in `metadata.managedFields` and is already parsed by the syncer for its own write decisions ([`extractNSFieldsPerManager`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L546), [`getManagerForLabel`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L562)), tracked per label key rather than per Namespace. A label written with `oc label`, `oc edit` or by a GitOps controller carries that manager's name and is plainly distinguishable from one the syncer wrote.

Ownership resolves to three cases:

| Owner of `pod-security.kubernetes.io/enforce` | Interpretation | Behavior |
|---|---|---|
| `pod-security-admission-label-synchronization-controller`, or the historical `cluster-policy-controller` | written by the syncer, now unmaintained | alert when more restrictive than the computed minimum |
| any other manager | deliberate configuration by an administrator, GitOps controller or CVO manifest | never alert |
| no owner recorded | ambiguous | metric only, no alert |

The unowned case is ambiguous rather than merely unknown. Backup and restore tooling, etcd restore and some migration paths drop or rewrite `managedFields`, so a label the syncer wrote can lose its ownership record; the same ownership rename already happened inside OpenShift, which is why [`forceHistoricalLabelsOwnership`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L224) exists and why both manager names are accepted above. Alerting on unowned labels would page administrators about configuration they may have set deliberately, on exactly the long-lived clusters where deliberate configuration is most likely. The count is therefore exposed as a metric for support to consult during an investigation, without an alert attached.

Note that this ownership signal is safe here because nothing is deleted. It fails safe for deciding whether to *write* a label — the worst case is setting one nobody owned — but it would fail unsafe for deciding whether to *delete* one, since an ownership record lost to a restore is indistinguishable from an administrator's deliberate label. Retaining the labels avoids that direction entirely.

#### Release note

The change in default posture, the retention of existing labels, and the new alert are called out in the release notes for release `n`, since an upgraded cluster's effective behavior changes without any administrator action.

## Open Questions

### Rerun the evaluation before enforcing

It could be better to run the evaluation before enforcing `Restricted` mode as the last run can be a couple of months old.
With alerts in place in the release before, it could be sufficient to run the evaluation just before enforcing `Restricted` mode.
In addition we could list the Namespaces that are violating the PSS in the `status.violatingNamespaces` field.

### Initial evaluation by the PodSecurityReadinessController

What happens if the `PodSecurityReadinessController`, didn't have the time to run at least once after an upgrade?

If a cluster upgrades from release `n-1`, through release `n` to release `n+1`, the `PodSecurityReadinessController` might not have time to run at least once in release `n` during the upgrade.
To prevent an unchecked transition into `Restricted` mode on release `n+1`, it will retry checking Namespaces until `status.lastTransitionTime` is set.
Once it ran successfully, it needs to set the `status.lastTransitionTime`.
This will give the user the ability to see how old the basis for the decision is..

### PSA label syncer turned off

The PSA Label Syncer will now be turned off by default on all clusters. This is because its use case was to allow customers to automatically migrate customers to `Restricted` PSS. As the default will now be `Privileged` on customer workloads, it is no longer required.

As the expected default PSS for OpenShift developers will remain `Restricted`, they will remain using the PSA label syncer to ensure OpenShift workloads still comply, such as in monitor and periodic tests.

### Impact on HyperShift

> **This section is known to be inaccurate and is being rewritten. Please do not review it yet.**
> Code verification has shown that the PSA label syncer runs as a management-cluster Deployment rather than as HCCO logic, that the `PodSecurityViolation` alert already ships and is unconditionally reconciled, and that HCP has no `cluster-kube-apiserver-operator` at all — the control-plane-operator renders the PodSecurity configuration directly. The replacement will live in a proper `Topology Considerations` section.

The impact on HyperShift isn't too much, as HyperShift conforms to the same PSS expectations as in standalone i.e. `Restricted` .

How the feature is consumed is different, and changes will need to be made to its feature set alongside standalone. Also the HCCO (Hosted Cluster Config Operator) will need to have its PSA Label Syncer logic removed, and PodSecurityViolation alert can be triggered if the feature is enabled. 

Essentially, similar to standalone, changes will need to be made so HyperShift logic is aware that PSA enforcement is no longer mandatory, is disabled by default, and can be enabled. Test logic will likely need to be re-worked to account for the new expectation in behavior.

## Test Plan

### Switching between PSA modes and enable/disable .

We need to be able to switch between different PSA modes (`restricted`,`baseline`, `privleged`) when PSA is enabled. Additionally, we should be able to enable or disable the feature at will. Tests must be added to ensure this is possible.

### Non-urgent, but important: Hardcoded SCC to PSA mapping

The PSA label syncer currently maps SCCs to PSS through a hard-coded rule set, and the PSA version is set to `latest`.
This setup risks becoming outdated if the mapping logic changes upstream.
To protect user workloads, an end-to-end test should fail if the mapping logic no longer behaves as expected.
Ideally, the PSA label syncer would use the `podsecurityadmission` package directly.
Otherwise, it can't be guaranteed that all possible SCCs are mapped correctly.

## Graduation Criteria
> **Draft.** The criteria below are being reworked against `dev-guide/feature-zero-to-hero.md` and currently understate the documented bar — the 14-runs-per-platform requirement is missing, the platform list is incomplete, and the "95% over 7 consecutive days" figure is not the documented window.

Graduation occurs in a tiered approach.

### Tier 1: removing `OpenShiftPodSecurityAdmission` from the `Default` feature set

`OpenShiftPodSecurityAdmission` is already in use today, and we re-use it rather than introducing a second gate for the same decision. Removing it from the `Default` feature set is what makes PSA enforcement optional, so it is the first tier of graduation.

Prerequisites for the removal:

- all OpenShift workloads and namespaces are labelled appropriately and have the correct SCC pinning;
- the existing monitor tests do not regress once the default posture is no longer enforcing.

This tier changes only the default. It does not remove the enforcement code paths, and it does not depend on the `PSAEnforcementConfig` API, which ships ungated.

This tier does not depend on the `PSAEnforcementConfig` API, which is still `TechPreviewNoUpgrade` at this point.

### Tier 2: graduating the opt-in configuration

In release `n+1`, once the default has moved and clusters are running with enforcement off, we graduate the opt-in path — the `PSAEnforcementConfig` API and the observer and syncer wiring that consume it.

This is a combined API and feature-gate promotion:

- `PSAEnforcementConfig` moves from `v1alpha1` to `v1`, with the `v1alpha1` version retained and served for one release for anyone who adopted it under TechPreview.
- `PodSecurityAdmissionConfiguration` moves from `TechPreviewNoUpgrade` to `Default`.
- API validation integration tests land alongside the `openshift/api` change, as required for all API changes.

In order for this feature to be promoted, the following is required in regards to testing:

- At least five tests are present in Sippy.
- Tests must be ran at least 7 times per week.
- Tests must run on all supported platforms on Sippy.
  - AWS, Azure, GCP, vSphere, Bare Metal.
- Each test must be individually trackable and show clear signal of success or failure.
- Tests must pass at least 95 percent of the time over 7 consecutive days.

<u>The above criteria must be met at least 14 days before branching for promotion to be accepted.</u>

Apart from test requirements, if `spec.enforcementMode = Restricted|Baseline|Privileged` can be set when the feature is enabled, and the feature can be disabled by the user at-will with no adverse effects. We can say the feature has graduated.

## Upgrade / Downgrade Strategy

### On Upgrade

No change is backported to release `n-1`. An earlier draft proposed backporting the API there; that has been dropped, because release `n-1` contains no consumer of the API and because the CRD and any stored object survive a downgrade regardless of whether `n-1` shipped the type.

- Release `n`:
  - Remove `OpenShiftPodSecurityAdmission` from the `Default` feature set. This is the change that makes PSA enforcement optional: the config observer's fallback moves from `restricted` to `privileged`, and the label syncer's default moves from enforcing to advising.
  - Ship `PSAEnforcementConfig` `v1alpha1` behind `PodSecurityAdmissionConfiguration` in `TechPreviewNoUpgrade`.
  - Enable the `PodSecurityReadinessController` to set the API's `status` — `enforcementMode`, `conditions`, `observedGeneration`, `lastEvaluationTime` and `violatingNamespaces`. It does not write `spec`.
  - Enable the `Config Observer Controller` to set the kube-apiserver's global `PodSecurity` `enforce` level from `status.enforcementMode` when it is `Baseline` or `Restricted`. `Privileged` and unset fall back to the level implied by the `OpenShiftPodSecurityAdmission` gate.
- Release `n+1`:
  - Promote `PSAEnforcementConfig` to `v1` and `PodSecurityAdmissionConfiguration` to `Default`, making the opt-in path generally available.

A cluster upgrading into release `n` does not have the `PSAEnforcementConfig` CRD unless it is on `TechPreviewNoUpgrade`. Every consumer must therefore treat an absent CRD as "no opinion expressed" and fall back to the gate-implied default, rather than erroring or degrading.

The `PSAEnforcementConfig` API and the controller wiring that consumes it are **not** guarded by `OpenShiftPodSecurityAdmission`; see [Role of the `OpenShiftPodSecurityAdmission` feature gate](#role-of-the-openshiftpodsecurityadmission-feature-gate). Guarding them with it would remove the opt-in path at the same moment it removes the default enforcement.

### On Downgrade

Y-stream downgrade is not a supported OpenShift operation, so this section describes recovery rather than a guarantee.

Downgrading from release `n` to release `n-1` restores mandatory enforcement, because `n-1` still has `OpenShiftPodSecurityAdmission` in its `Default` feature set and has no controller that reads `PSAEnforcementConfig`. For a cluster that had opted out with `spec.enforcementMode: Privileged`, this silently re-enables enforcement. The only recovery on `n-1` is `unsupportedConfigOverrides` on the `kubeapiservers.operator.openshift.io` resource. This must be called out in a release note.

The reverse case is benign: a cluster running `Restricted` on `n` downgrades into a release that enforces `restricted` anyway.

Note that the CVO does not delete CRDs on downgrade, so a `PSAEnforcementConfig` object created on `n` persists on `n-1` with no controller reading it, carrying a `status` that becomes stale. Re-upgrading to `n` must not treat that `status` as current; the `Evaluated` condition and `lastEvaluationTime` exist for exactly this.

## New Installation

Fresh installs won't have PSA enabled by default and will be disabled.
If the system administrator does not configure PSA at install time, `spec.enforcementMode` is unset, which resolves to `Privileged`.
This means that the system administrator can electively increase the value of `spec.enforcementMode` to the level of enforcement they are comfortable with.
The System Administrator needs to either disable PSA or configure the new API’s `spec.enforcementMode`, for `spec.enforcementMode = Privileged` to revert this.
There is no need for `PodSecurityReadinessController` to run as OpenShift workloads and namespaces should have been labelled appropriately. <u>It would need to run if enabled in existing clusters or when workloads are added as there will be existing customer workloads and namespaces.</u>

## Operational Aspects

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

### `pod-security.kubernetes.io/enforce` label has not been set

All namespaces should be labelled as appropriate. If this is not the case, see below:

To assess if your Namespace is capable of running with the `Restricted` PSS, run this:

```bash
  kubectl label --dry-run=server --overwrite $NAMESPACE --all \
      pod-security.kubernetes.io/enforce=restricted
```

To assess if your Namespace is capable of running with the `Baseline` PSS, run this:

```bash
  kubectl label --dry-run=server --overwrite $NAMESPACE --all \
      pod-security.kubernetes.io/enforce=baseline
```

If both commands return warning messages, the Namespace needs `Privileged` PSS in its current state.
It can be useful to read the warning messages to identify fields in the Pod manifest that could be adjusted to meet a higher security standard.

To set the label, remove the `--dry-run=server` flag.

#### Namespace workload doesn't use ServiceAccount-based SCC (user-based SCCs)

It can be, that a Namespace workload doesn't use ServiceAccount SCC, but receives the SCCs of the executing user.
This usually happens, when a workload isn't running through a deployment with a properly set up ServiceAccount.
This can be verified by checking the `security.openshift.io/validated-scc-subject-type` annotation on the Pod manifest.
Another way would be to check the `security.openshift.io/scc` value and check if the current ServiceAccount for the workload is capable of that SCC.

To solve the issue:

- Update the ServiceAccount to be able to use the necessary SCCs.
	The necessary SCC can be identified in the annotation `security.openshift.io/scc` of the existing workloads.
	After that is done, the PSA label syncer will update the PSA labels.
- Otherwise set the `pod-security.kubernetes.io/enforce` label manually.