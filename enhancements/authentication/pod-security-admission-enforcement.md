---
title: psa-enforcement-config
authors:
  - "@ibihim"
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
last-updated: 2025-01-23
tracking-link:
  - https://issues.redhat.com/browse/<JIRA_ID>
see-also:
  - "/enhancements/authentication/pod-security-admission.md"
replaces: []
superseded-by: []
---

# Pod Security Admission Enforcement Config

## Summary

This enhancement introduces a **new cluster-scoped API** and changes to the relevant controllers to rollout [Pod Security Admission (PSA)](https://kubernetes.io/docs/concepts/security/pod-security-admission/) enforcement [in OpenShift](https://www.redhat.com/en/blog/pod-security-admission-in-openshift-4.11).
Enforcement means that the `PodSecurityAdmissionLabelSynchronizationController` sets the `pod-security.kubernetes.io/enforce` label on Namespaces, and the PodSecurityAdmission plugin enforces the `Restricted` [Pod Security Standard (PSS)](https://kubernetes.io/docs/concepts/security/pod-security-standards/) globally on Namespaces without any label.

The new API allows users to either enforce the `Restricted` PSS or maintain `Privileged` PSS for several releases. Eventually, all clusters will be required to use `Restricted` PSS.

This enhancement expands the ["PodSecurity admission in OpenShift"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission.md) and ["Pod Security Admission Autolabeling"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission-autolabeling.md) enhancements.

## Motivation

After introducing Pod Security Admission and Autolabeling based on SCCs, some clusters were found to have Namespaces with Pod Security violations.
Over the last few releases, the number of clusters with violating workloads has dropped significantly.
Although these numbers are now quite low, it is essential to avoid any scenario where users end up with failing workloads.

To ensure a safe transition, this proposal suggests that if a potential failure of workloads is being detected in release `n`, that the operator moves into `Upgradeable=false`.
The user would need to either resolve the potential failures or set the enforcing mode to `Privileged` for now in order to be able to upgrade.
In the following release `n+1`, the controller will then do the actual enforcement, if `Restricted` is set.

An overview of the Namespaces with failures will be listed in the API's status, should help the user to fix any issues.

### Goals

1. Rolling out Pod Security Admission enforcement.
2. Minimize the risk of breakage for existing workloads.
3. Allow users to remain in “privileged” mode for a couple of releases.

### Non-Goals

1. Enabling the PSA label-syncer to evaluate workloads with user-based SCC decisions.
2. Providing a detailed list of every Pod Security violation in a Namespace.

## Proposal

### User Stories

As a System Administrator:
- I want to transition to enforcing Pod Security Admission only if the cluster would have no failing workloads.
- If there are workloads in certain Namespaces that would fail under enforcement, I want to be able to identify which Namespaces need to be investigated.
- If I encounter issues with the Pod Security Admission transition, I want to opt out (remain privileged) across my clusters until later.

### Current State

When the `OpenShiftPodSecurityAdmission` feature flag is enabled:
- The [PodSecurity configuration](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/cmd/render/render.go#L350-L358) for the kube-apiserver enforces `restricted` across the cluster.
- The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/327d3cbd82fd013a9d5d5733eb04cc0dcd97aec5/pkg/cmd/controller/psalabelsyncer.go#L17-L52) automatically sets the `pod-security.kubernetes.io/enforce` label.

### Rollout

Release `n` will introduce the new API. It will default to `Restricted` PSS in its `spec` and list violating Namespaces with potentially failing workloads in the `status` field.
If violating Namespaces are detected, the Operator will move into `Upgradeable=false`. To be able to upgrade, the user needs to resolve the violations or set the `spec` to `Privileged`.

In Release `n+1` the the controller will enforce PSA, if the `spec` is set to `Restricted`. If the `spec` is set to `Privileged` or the `FeatureGate` of `OpenShiftPodSecurityAdmission` is disable, the controllers won't enforce. The `FeatureGate` will act as a break-glass option.

#### Examples

Examples of failing workloads include:

- **Category 1**: Namespaces with workloads that use user-bound SCCs (workloads created directly by a user) without meeting the `Restricted` PSS.
- **Category 2**: Namespaces that do not have the `pod-security.kubernetes.io/enforce` label and whose workloads would not satisfy the `Restricted` PSS.
  Possible cases include:
  1. Namespaces with `security.openshift.io/scc.podSecurityLabelSync: "false"` and no `pod-security.kubernetes.io/enforce` label set.
  2. `openshift-` prefixed Namespaces (not necessarily created or managed by OpenShift teams).

### User Control and Insights

To allow user influence over this transition, a new API called `PSAEnforcementConfig` is introduced.
It will let administrators:
- Enable PSA enforcement by leaving the `spec` to `Restricted` and having no violating Namespaces. 
- Block PSA enforcement by setting the `spec` to `Privileged`.
- Get insights which Namespaces would fail in order to resolve the issues.

## Design Details

### Improved Diagnostics

The [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md) revealed that certain clusters would fail enforcement without clear explanations.
A likely root cause is that the [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) (PSA label syncer) does not label Namespaces that rely on user-based SCCs.
In some cases, the evaluation was impossible because PSA labels had been overwritten by users.
Additional diagnostics are required to confirm the full set of potential causes.

#### New SCC Annotation: `security.openshift.io/ValidatedSCCSubjectType`

The annotation `openshift.io/scc` currently indicates which SCC admitted a workload, but it does not distinguish **how** the SCC was granted — whether through a user or a Pod’s ServiceAccount.
A new annotation will help determine if a ServiceAccount with the required SCCs was used, or if a user created the workload out of band.
Because the PSA label syncer does not track user-based SCCs itself, it cannot fully assess labeling under those circumstances.

To address this, the proposal introduces:

```go
// ValidatedSCCSubjectTypeAnnotation indicates the subject type that allowed the
// SCC admission. This can be used by controllers to detect potential issues
// between user-driven SCC usage and the ServiceAccount-driven SCC usage.
ValidatedSCCSubjectTypeAnnotation = "security.openshift.io/validated-scc-subject-type"
```

This annotation will be set by the [`SecurityContextConstraint` admission](https://github.com/openshift/apiserver-library-go/blob/60118cff59e5d64b12e36e754de35b900e443b44/pkg/securitycontextconstraints/sccadmission/admission.go#L138) plugin.

#### Set PSS Annotation: `security.openshift.io/MinimallySufficientPodSecurityStandard`

The PSA label syncer must set the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation.
Because users can modify `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit`, these labels do not reliably indicate the minimal standard.
The new annotation ensures a clear record of the minimal PSS that would be enforced if the `pod-security.kubernetes.io/enforce` label were set.

#### Update the PodSecurityReadinessController

By adding these annotations, the [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) can more accurately identify potentially failing Namespaces and understand their root causes:

- With the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation, it can evaluate Namespaces that lack the `pod-security.kubernetes.io/enforce` label but have user-overridden warn or audit labels.
- With `ValidatedSCCSubjectType`, the controller can classify issues arising from user-based SCC workloads separately.
  Many of the remaining clusters with violations appear to involve workloads admitted by user SCCs.

### Secure Rollout

The Proposal section indicates that enforcement will be introduced first at the Namespace level and later at the global (cluster-wide) level.
In addition to adjusting how the `OpenShiftPodSecurityAdmission` `FeatureGate` behaves, administrators need visibility and control throughout this transition.
A new API is necessary to provide this flexibility.

#### New API

This API offers a gradual way to roll out Pod Security Admission enforcement to clusters.
It gives users the ability to influence the rollout and see feedback on which Namespaces might violate Pod Security standards.

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PSAEnforcementMode indicates the actual enforcement state of Pod Security Admission
// in the cluster. Unlike PSATargetMode, which reflects the user’s desired or “target”
// setting, PSAEnforcementMode describes the effective mode currently active.
//
// The modes define a progression from no enforcement, through label-based enforcement
// to label-based with global config enforcement.
type PSAEnforcementMode string

const (
	// PSAEnforcementModePrivileged indicates that no Pod Security restrictions
	// are effectively applied.
	// This aligns with a pre-rollout or fully "privileged" cluster state,
	// where neither enforce labels are set nor the global config enforces "Restricted".
	PSAEnforcementModePrivileged PSAEnforcementMode = "Privileged"

	// PSAEnforcementModeLabel indicates that the cluster is enforcing Pod Security
	// labels at the Namespace level (via the PodSecurityAdmissionLabelSynchronizationController),
	// but the global kube-apiserver configuration is still "Privileged."
	PSAEnforcementModeLabel PSAEnforcementMode = "LabelEnforcement"

	// PSAEnforcementModeFull indicates that the cluster is enforcing
	// labels at the Namespace level, and the global configuration has been set
	// to "Restricted" on the kube-apiserver.
	// This represents full enforcement, where both Namespace labels and the global config
	// enforce Pod Security Admission restrictions.
	PSAEnforcementModeFull PSAEnforcementMode = "FullEnforcement"
)

// PSATargetMode reflects the user’s chosen (“target”) enforcement level.
type PSATargetMode string

const (
	// PSATargetModePrivileged indicates that the user wants no Pod Security
	// restrictions applied. The desired outcome is that the cluster remains
	// in a fully privileged (pre-rollout) state, ignoring any label enforcement
	// or global config changes.
	PSATargetModePrivileged PSATargetMode = "Privileged"

	// PSATargetModeConditional indicates that the user is willing to let the cluster
	// automatically enforce a stricter enforcement once there are no violating Namespaces.
	// If violations exist, the cluster stays in its previous state until those are resolved.
	// This allows a gradual move towards label and global config enforcement without
	// immediately breaking workloads that are not yet compliant.
	PSATargetModeConditional PSATargetMode = "Conditional"

	// PSATargetModeRestricted indicates that the user wants the strictest possible
	// enforcement, causing the cluster to ignore any existing violations and
	// enforce "Restricted" anyway. This reflects a final, fully enforced state.
	PSATargetModeRestricted PSATargetMode = "Restricted"
)

// PSAEnforcementConfig is the config for the PSA enforcement.
type PSAEnforcementConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user-settable values for configuring Pod Security Admission
	// enforcement
	Spec PSAEnforcementConfigSpec `json:"spec"`

	// status communicates the targeted enforcement mode, including any discovered
	// issues in Namespaces.
	Status PSAEnforcementConfigStatus `json:"status"`
}

// PSAEnforcementConfigSpec defines the desired configuration for Pod Security
// Admission enforcement.
type PSAEnforcementConfigSpec struct {
	// targetMode is the user-selected Pod Security Admission enforcement level.
	// Valid values are:
	//   - "Privileged": ensures the cluster runs with no restrictions
	//   - "Conditional": defers the decision to cluster-based evaluation
	//   - "Restricted": enforces the strictest Pod Security admission
	//
	// If this field is not set, it defaults to "Conditional".
	//
	// +kubebuilder:default=Conditional
	TargetMode PSATargetMode `json:"targetMode"`
}

// PSAEnforcementConfigStatus defines the observed state of Pod Security
// Admission enforcement.
type PSAEnforcementConfigStatus struct {
	// enforcementMode indicates the effective Pod Security Admission enforcement
	// mode in the cluster. Unlike spec.targetMode, which expresses the desired mode,
	// enforcementMode reflects the actual state after considering any existing
	// violations or user overrides.
	EnforcementMode PSAEnforcementMode `json:"enforcementMode"`

	// violatingNamespaces is a list of namespaces that can initially block the
	// cluster from fully enforcing a "Restricted" mode. Administrators should
	// review each listed Namespace to fix any issues to enable strict enforcement.
	//
	// If a cluster is already in a more "Restricted" mode and new violations emerge,
	// it remains in "Restricted" until the user explicitly switches to
	// "spec.mode = Privileged".
	//
	// To revert "Restricted" mode the Administrators need to set the
	// PSAEnfocementMode to "Privileged".
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
	// with the requested Pod Security mode and highlights which mode is affected.
	//
	// Possible values are:
	// - PSAConfig: Misconfigured OpenShift Namespace
	// - PSAConfig: PSA label syncer disabled
	// - PSALabel: ServiceAccount with insufficient SCCs
	//
	// +optional
	Reason string `json:"reason,omitempty"`
}
```

`Privileged` and `Restricted` each ignore cluster feedback and strictly enforce their respective modes:

- `Privileged` -> `Privileged`
- `Restricted` -> `FullEnforcement`

When `Conditional` is selected, enforcement depends on whether there are violating Namespaces and on the current release.

- In `n` and `n+1`: It only progresses from `Privileged` to `LabelEnforcement`, if there would be no PSA label syncer violations.
- In `n+1`: It only progresses from `LabelEnforcement` to`FullEnforcement`, if there would be no PodSecurity config violations.

Below is a table illustrating the expected behavior when the `FeatureGate` `OpenShiftPodSecurityAdmission` is enabled:

| spec.targetMode   | violations found | release | status.enforcementMode |
| ----------------- | ---------------- | ------- | ---------------------- |
| Restricted        | none             | n - 1   | Privileged             |
| Restricted        | found            | n - 1   | Privileged             |
| Privileged        | none             | n - 1   | Privileged             |
| Privileged        | found            | n - 1   | Privileged             |
| Conditional       | none             | n - 1   | Privileged             |
| Conditional       | found            | n - 1   | Privileged             |
| Restricted        | none             | n       | FullEnforcement        |
| Restricted        | found            | n       | FullEnforcement        |
| Privileged        | none             | n       | Privileged             |
| Privileged        | found            | n       | Privileged             |
| Conditional       | none             | n       | LabelEnforcement       |
| Conditional       | found            | n       | LabelEnforcement       |
| Restricted        | none             | n + 1   | FullEnforcement        |
| Restricted        | found            | n + 1   | FullEnforcement        |
| Privileged        | none             | n + 1   | Privileged             |
| Privileged        | found            | n + 1   | Privileged             |
| Conditional       | none             | n + 1   | FullEnforcement        |
| Conditional       | found            | n + 1   | Privileged             |

A cluster that uses `spec.targetMode = Conditional` can revert to `Privileged` only if the user explicitly sets `spec.targetMode = Privileged`.
A cluster in `spec.mode = Conditional` that starts with `status.EnforcementMode = Privileged` may switch to a more restrictive enforcement mode as soon as there are no violations.
To manage the timing of this rollout, an administrator can set `spec.mode = Privileged` and later switch it to `Conditional` when ready.

`status.violatingNamespaces` lists the Namespaces that would fail if `status.enforcementMode` were `LabelEnforcement` or `FullEnforcement`.
The reason field helps identify whether the PSA label syncer or the PodSecurity config is the root cause.
Administrators must query the kube-apiserver (or use the [cluster debugging tool](https://github.com/openshift/cluster-debug-tools)) to pinpoint specific workloads.

### Implementation Details

- The `PodSecurityReadinessController` in the `cluster-kube-apiserver-operator` will manage the new API.
- If the `FeatureGate` is removed from the current `FeatureSet`, the cluster must revert to its previous state.
- The [`Config Observer Controller`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L131) must be updated to watch for the new API alongside the `FeatureGate`.

#### PodSecurityReadinessController

The `PodSecurityReadinessController` will manage the `PSAEnforcementConfig` API.
It already collects most of the necessary data to determine whether a Namespace would fail enforcement or not to create a [`ClusterFleetEvaluation`](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-fleet-evaluation.md).
With the `security.openshift.io/MinimallySufficientPodSecurityStandard`, it will be able to evaluate all Namespaces for failing workloads, if any enforcement would happen.
With the `security.openshift.io/ValidatedSCCSubjectType`, it can categorize violations more accurately and create a more accurate `ClusterFleetEvaluation`.

#### PodSecurity Configuration

A Config Observer in the `cluster-kube-apiserver-operator` manages the Global Config for the kube-apiserver, adjusting behavior based on the feature flag.
It must watch both the `status.enforcementMode` and the `FeatureGate` to make decisions.

#### PSA Label Syncer

The PSA label syncer will watch the `status.enforcementMode` and the `OpenShiftPodSecurityAdmission` feature gate.
If `status.enforcementMode` is `LabelEnforcement` or `FullEnforcement` and `OpenShiftPodSecurityAdmission` is enabled, the syncer will set the `pod-security.kubernetes.io/enforce` label.
Otherwise, it will refrain from setting that label and remove any enforce labels it owns if existent.

Because the ability to set `pod-security.kubernetes.io/enforce` is introduced in release `n`, the ability to remove that label must exist in release `n-1`.
Otherwise, the cluster will be unable to revert to its previous state.

## Open Questions

### Fresh Installs

Needs to be evaluated. The System Administrator needs to pre-configure the new API’s `spec.targetMode`, choosing whether the cluster will be `privileged`, `restricted`, or `conditional` during a fresh install.

### Impact on HyperShift

Needs to be evaluated.

### Baseline Clusters

The current suggestion differentiates between `restricted` and `privileged` PSS.
It may be possible to introduce an intermediate step and set the cluster to `baseline` instead.

## Test Plan

The PSA label syncer currently maps SCCs to PSS through a hard-coded rule set, and the PSA version is set to `latest`.
This setup risks becoming outdated if the mapping logic changes upstream.
To protect user workloads, an end-to-end test should fail if the mapping logic no longer behaves as expected.
Ideally, the PSA label syncer would use the `podsecurityadmission` package directly.
Otherwise, it can't be guaranteed that all possible SCCs are mapped correctly.

## Graduation Criteria

- If `status.enforcementMode = LabelEnforcement` rolls out on most clusters with no adverse effects, `status.enforcementMode = FullEnforcement` can be enabled in the subsequent release.
- If the majority of users have `status.enforcementMode = FullEnforcement`, then upgrades can be blocked on clusters that do not reach that state.

## Upgrade / Downgrade Strategy

### On Upgrade

See the [Release Timing](#release-timing) section for the overall upgrade strategy.

### On Downgrade

See the earlier references, including the [PSA Label Syncer](#psa-label-syncer) subsection in the [Implementation Details](#implementation-details) section, for the downgrade strategy.

## New Installation

The default for new installs is `Conditional`, to prompt administrators toward adopting `Restricted`.

A fresh install should not have any violating Namespaces.
Therefore, as `spec.targetMode` is not set to `Privileged`, the cluster would move to `status.enforcementMode = LabelEnforcement` or `status.enforcementMode = FullEnforcement`.
An administrator can also configure the cluster to start in `Privileged` if desired.

## Operational Aspects

- If a cluster is set to `Conditional` and has initial violations, those may be resolved one by one.
  Once all violations are resolved, the cluster may immediately transition to `Restricted`.
  Some administrators may prefer managing this switch manually.
- After a cluster switches to a stricter `status`, no violating workloads should be possible.
  If a violating workload appears, there is no automatic fallback to a more privileged state, thus avoiding additional kube-apiserver restarts.
- Administrators facing issues in a cluster already set to a stricter enforcement can change `spec.targetMode` to `Privileged` to halt enforcement for other clusters.
- ClusterAdmins must ensure that directly created workloads (user-based SCCs) have correct `securityContext` settings.
  Updating default workload templates can help.
- To identify specific problems in a violating Namespace, administrators can query the kube-apiserver:

  ```bash
  kubectl label --dry-run=server --overwrite $NAMESPACE --all \
      pod-security.kubernetes.io/enforce=$MINIMALLY_SUFFICIENT_POD_SECURITY_STANDARD
  ```
