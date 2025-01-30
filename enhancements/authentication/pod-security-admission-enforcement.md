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

This enhancement introduces a **new cluster-scoped API** and changes to the `OpenShiftPodSecurityAdmission` `FeatureGate` to roll out [Pod Security Admission (PSA)](https://kubernetes.io/docs/concepts/security/pod-security-admission/) enforcement [in OpenShift](https://www.redhat.com/en/blog/pod-security-admission-in-openshift-4.11) gradually.
Enforcement means that the `PodSecurityAdmissionLabelSynchronizationController` is setting the `pod-security.kubernetes.io/enforce` label on Namespaces, and the PodSecurityAdmission plugin is enforcing the `Restricted` [Pod Security Standard (PSS)](https://kubernetes.io/docs/concepts/security/pod-security-standards/).
Gradually means that both changes happen in separate steps.

The new API offers users the option to manipulate the outcome by enforcing the `Privileged` or `Restricted` PSS directly.
The suggested default decision is `Conditional`, which will only progress if no potentially failing workloads are found.
The progression will start with the `PodSecurityAdmissionLabelSynchronizationController` labeling Namespaces for enforcement and finish with the `PodSecurity` configuration being set to `Restricted` by default.

This enhancement expands the ["PodSecurity admission in OpenShift"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission.md) and ["Pod Security Admission Autolabeling"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission-autolabeling.md) enhancements.

## Motivation

After introducing Pod Security Admission and the Autolabeling based on SCCs, clusters were identified that had Namespaces with Pod Security violations.
Over the last few releases, the number of clusters with failing workloads has been reduced significantly.
Although these numbers are now very low, we still must avoid any scenario where users end up with failing workloads.
To enable a smooth and safe transition, this proposal uses a gradual and conditional rollout based on the new API, which also provides an overview of which Namespaces could contain failing workloads.

### Goals

1. Start the process of rolling out PodSecurityAdmission enforcement.
1. Minimize the risk of breakage for existing workloads.
1. Enable users to remain in "privileged" mode for a couple of releases.

### Non-Goals

1. Enabling the PSA label-syncer to evaluate user permissions for directly created workloads.
1. Providing a detailed list of every Pod Security violation in a Namespace.

## Proposal

This section outlines the necessary changes for a safe, stepwise rollout of Pod Security Admission enforcement.

### User Stories

As a System Administrator:
- I want to transition to enforcing Pod Security Admission only if the cluster would have no failing workloads.
- If there are workloads in certain Namespaces that would fail under enforcement, I want to be able to identify which Namespaces need to be fixed.
- If I encounter issues with the Pod Security Admission transition, I want to opt out (remain privileged) across my clusters until I can fix the issues.

### Current State

When the `OpenShiftPodSecurityAdmission` feature flag is enabled today:
- The [PodSecurity configuration](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/cmd/render/render.go#L350-L358) for the kube-apiserver enforces `restricted` across the cluster.
- The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/327d3cbd82fd013a9d5d5733eb04cc0dcd97aec5/pkg/cmd/controller/psalabelsyncer.go#L17-L52) automatically sets the `pod-security.kubernetes.io/enforce` label.

This all-or-nothing mechanism makes it difficult to safely enforce PSA while minimizing disruptions.

### Gradual Process

To allow a safer rollout of enforcement, the following steps are proposed:

1. **Label Enforce**
   The `PodSecurityAdmissionLabelSynchronizationController` will set the `pod-security.kubernetes.io/enforce` label on Namespaces, provided the cluster has no potentially failing workloads.

2. **Global Config Enforce**
   Once all viable Namespaces are labeled successfully, the cluster will set the `PodSecurity` configuration for the kube-apiserver to `Restricted`, again only if there are no potentially failing workloads.

The feature flag `OpenShiftPodSecurityAdmission` will serve as a break-glass option. If unexpected failures occur, the rollout can be reverted by removing the `FeatureGate` from the default `FeatureSet`.

#### Examples

- **Category 1**: Namespaces with workloads that use user-bound SCCs (workloads created directly by a user) without meeting the `Restricted` PSS.
- **Category 2**: Namespaces that do not have the `pod-security.kubernetes.io/enforce` label and whose workloads would not satisfy the `Restricted` PSS. Possible cases include:
  1. Namespaces with `security.openshift.io/scc.podSecurityLabelSync: "false"` and no `pod-security.kubernetes.io/enforce` label set.
  2. `openshift-` prefixed Namespaces (not necessarily created or managed by OpenShift teams).

### User Control and Insights

To allow users influence over this gradual transition, a new API called `PSAEnforcementConfig` is introduced. It will let administrators:

- Force `Restricted` enforcement, ignoring potential violations.
- Remain in `Privileged` mode, regardless of whether violations exist or not.
- Let the cluster evaluate the state and automatically enforce `Restricted` if no workloads would fail.
- Identify Namespaces that would fail enforcement.

### Release Timing

The gradual process will span three releases:

- **Release `n-1`**: Introduce the new API and diagnostics for identifying violating Namespaces.
- **Release `n`**: Permit the `PodSecurityAdmissionLabelSynchronizationController` to enforce labels if there are no workloads that would fail.
- **Release `n+2`**: Enable the PodSecurity configuration to enforce `restricted` if no failing workloads remain.

Here, `n` could be OCP `4.19`, assuming it is feasible to backport the API and diagnostics to earlier versions.

## Design Details

### Improved Diagnostics

The [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md) revealed that some clusters would fail enforcement, and it was unclear why certain Namespaces in those clusters were failing.
The assumed root-cause is that [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) (PSA label syncer) does not label Namespaces that rely on user-based SCCs.
In some cases, it couldn't be evaluated as all as the PSA labels have been overwritten by the user.
To confirm this and identify all potential causes, we need improved diagnostics.

#### New SCC Annotation: `security.openshift.io/ValidatedSCCSubjectType`

Currently, the annotation `openshift.io/scc` indicates which SCC admitted a workload, but it does not distinguish **how** that SCC was granted—**via a user** or **via the Pod’s service account**.
A new annotation will help determine if a ServiceAccount with the required SCCs was used or if a user created the workload out of band.
Since the PSA label syncer does not track user-based SCCs, it cannot fully assess labeling in those cases.

Based on this reasoning, the proposal introduces:

```go
// ValidatedSCCSubjectTypeAnnotation indicates the subject type that allowed the
// SCC admission. This can be used by controllers to detect potential mismatches
// between user-driven SCC usage and the ServiceAccount-driven usage considered
// by the PSA label syncer.
ValidatedSCCSubjectTypeAnnotation = "security.openshift.io/ValidatedSCCSubjectType"
```

This annotation will be set by the [`SecurityContextConstraint` admission](https://github.com/openshift/apiserver-library-go/blob/60118cff59e5d64b12e36e754de35b900e443b44/pkg/securitycontextconstraints/sccadmission/admission.go#L138) plugin.

#### Set PSS Annotation: `security.openshift.io/MinimallySufficientPodSecurityStandard`

The PSA label syncer must set the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation.
Because users can modify `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit`, these labels do not reliably indicate the minimal standard.
The new annotation ensures a clear record of the minimal Pod Security Standard that would be enforced if the `pod-security.kubernetes.io/enforce` label would be set.

#### Update the PodSecurityRead

By adding these annotations, the [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) PodSecurityReadinessController can more accurately identify potentially failing Namespaces and understand their root causes:

- With the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation, it can evaluate Namespaces that lack the `pod-security.kubernetes.io/enforce` label yet have user-overridden warn or audit labels.
- With ValidatedSCCSubjectType, the controller can classify issues arising from user-based SCC workloads separately. We strongly suspect that many of the remaining clusters with violations involve workloads admitted by user SCCs instead of ServiceAccounts.

### Secure Roll-out

As noted in the Proposal, we will introduce these changes gradually: first enforcing Pod Security Admission at the Namespace level, then at the global (cluster-wide) level.
In addition to adjusting how the `OpenShiftPodSecurityAdmission` `FeatureGate` behaves, we must give administrators both visibility and control over this transition.
Therefore, introducing a new API becomes necessary.

#### New API

This API provides the flexibility to roll out Pod Security Admission enforcement to clusters in a gradual way.
It gives users the ability to control the process and see feedback about the cluster’s state regarding Pod Security violations.

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PSAEnforcementMode is the enforcement mode for Pod Security Admission roll-out.
type PSAEnforcementMode string

const (
	// EnforcementModePrivileged indicates that no Pod Security restrictions are
	// being effectively applied.
	// This puts the cluster in its pre-roll-out state.
	EnforcementModePrivileged PSAEnforcementMode = "Privileged"

	// EnforcementModeConditional defers the enforcement decision to the
	// evaluation of the cluster.
	// In practice:
	//   - If any violating Namespaces exist, the cluster remains at "Privileged".
	//   - If no violating Namespaces exist, the cluster enforces "Restricted".
	// State changes:
	//   - If the state changes from "any violation" to "no violations", the
	//     cluster will start switching to enforcing "Restricted". For a
	//     controlled switch, set EnforcementMode to "Privileged" and change to
	//     "Conditional" back once ready.
	//   - If the state changes from "no violations" to "any violation", and the
	//     cluster has already settled on "Restricted", it will not change back
	//     unless the EnforcementMode is set to "Privileged".
	EnforcementModeConditional PSAEnforcementMode = "Conditional"

	// EnforcementModeRestricted indicates that the strictest Pod Security
	// restrictions are applied. This effectively moves the cluster into the
	// "Restricted" state, despite any existing violations.
	EnforcementModeRestricted PSAEnforcementMode = "Restricted"
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
	// mode is the user-selected Pod Security Admission enforcement level.
	// Valid values are:
	//   - "Privileged": ensures the cluster runs with no restrictions
	//   - "Conditional": defers the decision to cluster-based evaluation
	//   - "Restricted": enforces strict Pod Security admission
	//
	// If this field is not set, it defaults to "Conditional".
	//
	// +kubebuilder:default=Conditional
	Mode PSAEnforcementMode `json:"mode"`
}

// PSAEnforcementConfigStatus defines the observed state of Pod Security
// Admission enforcement.
type PSAEnforcementConfigStatus struct {
	// labelMode is the actual Pod Security Admission mode being
	// enforced by the PSA label syncer. This will differ from spec.mode if
	// spec.mode is "Conditional", then the presence or absence of violations
	// decides upon the outcome.
	// The modes are:
	// - `Restricted`, the PSA label syncer will set the enforce label.
	// - `Privileged`, the PSA label syncer will not set the enforce label.
	// +kubebuilder:validation:Enum=Privileged;Restricted
	LabelMode PSAEnforcementMode `json:"labelMode,omitempty"`

	// configMode is the actual Pod Security Admission mode being
	// enforced by the kube-apiserver. This will differ from spec.mode if
	// spec.mode is "Conditional", then the presence or absence of violations
	// decides upon the outcome.
	// The modes are:
	// - `Restricted`, the PSA plugin will enforce `Restricted` by default.
	// - `Privileged`, the PSA plugin will enforce `Privileged` by default.
	// +kubebuilder:validation:Enum=Privileged;Restricted
	ConfigMode PSAEnforcementMode `json:"configMode,omitempty"`

	// violatingNamespaces is a list of namespaces that can initially block the
	// cluster from fully enforcing a "Restricted" mode. Administrators should
	// review each listed Namespace to fix any issues to enable strict enforcement.
	//
	// If a cluster is already in "Restricted" mode and new violations emerge,
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
	// +optional
	Reason string `json:"reason,omitempty"`
}
```
The `spec` allows the user to choose the desired state:

- `Privileged`
- `Conditional`
- `Restricted`

While `Privileged` and `Restricted` each ignore cluster feedback and strictly enforce their respective modes, `Conditional` depends on whether any Namespaces are violating.
If `spec.mode = Conditional` and no violations are found, the `status` mode becomes `Restricted`. Otherwise, it becomes `Privileged`.

As noted in the [Release Timing](#release-timing) section, when `status` is `Restricted` in release `n`, the PSA label syncer sets the `pod-security.kubernetes.io/enforce` label.
In release `n+1`, `status` `Restricted` implies the kube-apiserver is configured to enforce `Restricted` by default.

Below is a table illustrating the expected behavior:

| spec.mode   | violations found | release | status.labelMode | status.configMode |
| ----------- | ---------------- | ------- | ---------------- | ----------------- |
| Restricted  | none             | n       | Restricted       | Privileged        |
| Restricted  | found            | n       | Restricted       | Privileged        |
| Privileged  | none             | n       | Privileged       | Privileged        |
| Privileged  | found            | n       | Privileged       | Privileged        |
| Conditional | none             | n       | Restricted       | Privileged        |
| Conditional | found            | n       | Privileged       | Privileged        |
| Restricted  | none             | n + 1   | Restricted       | Restricted        |
| Restricted  | found            | n + 1   | Restricted       | Restricted        |
| Privileged  | none             | n + 1   | Privileged       | Privileged        |
| Privileged  | found            | n + 1   | Privileged       | Privileged        |
| Conditional | none             | n + 1   | Restricted       | Restricted        |
| Conditional | found            | n + 1   | Privileged       | Privileged        |

Once a cluster with `spec.mode = Conditional` settles on `Restricted`, it can revert to `Privileged` only if the user explicitly sets `spec.mode = Privileged`.
If a cluster in `spec.mode = Conditional` mode begins with `status` `Privileged`, it may immediately flip to `status` `Restricted` when no violations remain.
To manage rollout timing, a user could set `spec.mode = Privileged` and then switch to `Conditional` back at a suitable time.

`status.violatingNamespaces` lists the Namespaces that would have failing workloads, if `status` `Restricted` is being enforced.
The reason gives the user the ability to identify which Namespaces are failing and in which which type of failure it is: based on the PSA label syncer or the PSA configuration.
To identify specific workloads, an administrator must query the kube-apiserver or optionally use the [cluster debugging tool](https://github.com/openshift/cluster-debug-tools), which can pinpoint the problematic workloads.

### Implementation Details

The `PodSecurityReadinessController` in the `cluster-kube-apiserver-operator` will manage the new API.

If the `FeatureGate` is removed, the cluster must revert to its previous state.

#### PodSecurity Configuration

Currently, a Config Observer in the `cluster-kube-apiserver-operator` manages the Global Config for the kube-apiserver, adjusting behavior based on the feature flag.
It will need to watch both the `status.configMode` (for `Restricted`) and the `FeatureGate` to make its decisions.

#### PSA Label Syncer

The PSA label syncer will watch the `status.labelMode` and the `OpenShiftPodSecurityAdmission` feature gate.
If `status.labelMode` is `Restricted` and `OpenShiftPodSecurityAdmission` is enabled, the syncer will set the `pod-security.kubernetes.io/enforce` label.
Otherwise, it will refrain from setting that label and remove any enforce labels it owns.

Because the ability to set `pod-security.kubernetes.io/enforce` will be introduced in release `n`, the ability to remove that label must exist in release `n-1`.
Otherwise, the cluster won't be able to revert to its previous state.

## Open Questions

### Baseline Clusters

The current suggestion differentiates between `restricted` and `privileged` PSS.
It may be possible to introduce an intermediate step and set the cluster to `baseline` instead.

### Fresh Installs

The System Administrator needs to pre-configure the new API’s `spec.mode`, choosing whether the cluster will be `privileged`, `restricted`, or `conditional` upon a fresh install.

## Test Plan

The PSA label syncer currently maps SCCs to PSS through a hard-coded rule set, and the PSA version is set to `latest`.
This arrangement risks becoming outdated if the mapping logic changes over time upstream.
To protect user workloads, an end-to-end test should fail if the mapping logic no longer behaves as expected.
Ideally, the PSA label syncer would use the `podsecurityadmission` package directly.
Otherwise it can't be guaranteed that all possible SCCs are mapped correctly.

## Graduation Criteria

- If the PSA label syncer can roll out on most clusters without negative feedback, the global PSA config can be set to `restricted`.
- If the majority of users have their global PSA config set to `restricted`, upgrades can be blocked on clusters that are still not in that state.

## Upgrade / Downgrade Strategy

### On Upgrade

See the [Release Timing](#release-timing) section for the overall upgrade strategy.

### On Downgrade

See the earlier references, including the [PSA Label Syncer](#psa-label-syncer) subsection in the [Implementation Details](#implementation-details) section, for the downgrade strategy.

## New Installation

Because a fresh install should not have any violating Namespaces, the cluster would move to `Restricted` if `spec.mode = Privileged` is not set.
As described earlier, the default for new installs is `Conditional`, prompting users to adopt `Restricted`.
An administrator can also configure the cluster to start in `Privileged` if desired.

## Operational Aspects

- If a cluster is set to `Conditional` and has violations initially, those violations might be resolved one-by-one.
  Once all violations are resolved, the cluster may suddenly switch to `Restricted`, which some admins may prefer to manage manually.
- After a cluster switches to `status` `Restricted`, no violating workloads should be possible.
  If a violating workload appears, there is no automatic fallback to `Privileged`, preventing unnecessary kube-apiserver restarts.
- If users identify issues in a cluster that has already switched to `Restricted`, they can proactively set `spec.mode = Privileged` to halt enforcement on other clusters.
- ClusterAdmins must ensure the proper `securityContext` is set for directly created workloads (user-based SCCs).
  Modifying default workload templates may simplify this process.
- To determine specific problems in a violating Namespace, admins can query the kube-apiserver:

  ```bash
  kubectl label --dry-run=server --overwrite $NAMESPACE --all \
      pod-security.kubernetes.io/enforce=$MINIMALLY_SUFFICIENT_POD_SECURITY_STANDARD
  ```
