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

The feature flag `OpenShiftPodSecurityAdmission` will serve as a break-glass option. If unexpected failures occur, the roll-out can be reverted by removing the `FeatureGate` off the default `FeatureSet`.

#### Examples

- **Category 1**: Namespaces with workloads that use user-bound SCCs (workloads created directly by a user) without meeting the `Restricted` PSS.
- **Category 2**: Namespaces that do not have the `pod-security.kubernetes.io/enforce` label and whose workloads would not satisfy the `Restricted` PSS. Possible cases include:
  1. Namespaces with `security.openshift.io/scc.podSecurityLabelSync: "false"` and no `pod-security.kubernetes.io/enforce` label set.
  2. `openshift-` prefixed Namespaces (not necessarily created or managed by OpenShift teams).

### User Control and Insights

To allow users influence over this gradual transition, a new API called `PSAEnforcementConfig` is introduced. It will let administrators:

- Force `Restricted` enforcement, ignoring potential violations.
- Remain in `Privileged` mode, regardless of whether violations exist or not.
- Let the cluster evaluate the state and enforce `Restricted`, if now workloads would fail.
- Identify Namespaces that would fail enforcement.

### Release Timing

The gradual process will span three releases:

- **Release `n-1`**: Introduce the new API and diagnostics for identifying violating Namespaces.
- **Release `n`**: Permit the `PodSecurityAdmissionLabelSynchronizationController` to enforce labels if there are no workloads that would fail.
- **Release `n+2`**: Enable the PodSecurity configuration to enforce `restricted` if no failing workloads remain.

Here, `n` could be OCP `4.19`, assuming it is feasible to backport the API and diagnostics to earlier versions.

## Design Details

### Improved Diagnostics

The [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md) showed that there are cases were clusters fail.
In some of those cases it couldn't be evaluated yet, why Namespaces in those clusters are failing.
There is a strong assumption that it is due to the [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go)(PSA label syncer) not labeling Namespaces with user-based SCCs.
There is a need to improve the diagnostics to ensure that the root cause has been identified for those clusters that are potentially failing.

#### New SCC Annotation: `security.openshift.io/ValidatedSCCSubjectType`

Today, there is an annotation (`openshift.io/scc`) indicating which SCC was used to admit a workload.
However, it isn't being distinguished **how** that SCC was granted—**via a user** or **via the Pod’s service account**.
A new annotation would help us to understand if there is a ServiceAccount with the required SCCs or if a user created a workload out of bound.
As the PSA label syncer doesn't watch a user's SCCs, it can't assess the labels directly.

Based on that argumentation, the proposal contains a new annotation:

```go
// ValidatedSCCSubjectTypeAnnotation indicates the subject type that allowed the
// SCC admission. This can be used by controllers to detect potential mismatches
// between user-driven SCC usage and the ServiceAccount-driven usage considered
// by the PSA label syncer.
ValidatedSCCSubjectTypeAnnotation = "security.openshift.io/ValidatedSCCSubjectType"
```

That annotation would be set by the [`SecurityContextConstraint` admission](https://github.com/openshift/apiserver-library-go/blob/60118cff59e5d64b12e36e754de35b900e443b44/pkg/securitycontextconstraints/sccadmission/admission.go#L138) plugin.

#### Set SCC Annotation: `security.openshift.io/MinimallySufficientPodSecurityStandard`

The PSA label syncer needs to set the `security.openshift.io/MinimallySufficientPodSecurityStandard`.
`pod-security.kubernetes/warn` and `pod-security.kubernetes/audit` can be modified by the user which leads to the fact that it is not clear what the PSA label syncer would have decided.
This is important to evaluate how the PSA label syncer how it might decide in enforce-mode, if the `pod-security.kubernetes/enforce` label is unset, while the other labels are missing.

#### Update the PodSecurityReadinessController

Adding both annotations enables the [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) to more accurately predict potentially failing Namespaces and their root cause:

- With the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation, the [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) will be able to evaluate the Namespaces that don't have a `pod-security.kubernetes.io/enforce` label, while having an audit and warn label that are overwritten by the user.
- With the usage of `ValidatedSCCSubjectType`, it is possible to create another classification of reason for clusters with failing workloads: user-based workloads. There is a strong assumption that most of the reminding clusters with violations are based on the fact that they are running with user-based SCCs.

### Secure Roll-out

As mentioned in the proposal, the changes will be introduced by a gradual movement towards the enforcement of Pod Security Admission on the Namespace-level and then on the Global-config-level.
Besides the change of the behavior of the `FeatureGate` for the `OpenShiftPodSecurityAdmission`, it is required to give control and insight about the process to the user.
The introduction of a new API is necessary.

#### New API

This API will give us the flexibility to roll out the Pod Security Admission enforcement to clusters in a gradual way.
It gives the users the ability to have control over the process and gives them the necessary feedback about the cluster's current state with regards to Pod Security violations.

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PSAEnforcementMode is the enforcement mode for Pod Security Admission
// roll-out.
type PSAEnforcementMode string

const (
	// EnforcementModePrivileged indicates that no Pod Security restrictions are
	// being applied effectively.
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
	//     cluster settled on enforcing "Restricted" already, the state won't
	//     change back, except the EnforcementMode is set to "Privileged".
	EnforcementModeConditional PSAEnforcementMode = "Conditional"

	// EnforcementModeRestricted indicates that the strictest Pod Security
	// restrictions apply. This effectively moves the cluster into the
	// "Restricted" state, despite violations.
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
	// spec.mode is Conditional, then the initial decision will be based on
	// violating Namespaces.
	// The modes are:
	// - `Restricted`, the PSA label syncer will set the enforce label.
	// - `Privileged`, the PSA label syncer will not set the enforce label.
	// +kubebuilder:validation:Enum=Privileged;Restricted
	LabelMode PSAEnforcementMode `json:"labelMode,omitempty"`

	// configMode is the actual Pod Security Admission mode being
	// enforced by the kube-apiserver. This will differ from spec.mode if
	// spec.mode is Conditional, then the initial decision will be based on
	// violating Namespaces.
	// The modes are:
	// - `Restricted`, the PSA plugin will enforce `Restricted` by default.
	// - `Privileged`, the PSA plugin will enforce `Privileged` by default.
	// +kubebuilder:validation:Enum=Privileged;Restricted
	ConfigMode PSAEnforcementMode `json:"configMode,omitempty"`

	// violatingNamespaces is a list of namespaces that can initially block the
	// cluster from fully enforcing a "Restricted" mode. Administrators should
	// review each listed Namespace and fix any issues to enable strict
	// enforcement.
	//
	// Violations after "Restricted" mode is being applied, have no effect. This
	// means that a cluster can have "status.labelMode = Restricted" or
	// "status.configMode = Restricted" and a list of violating namespaces.
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

	// reason is an textual description explaining why the Namespace is
	// incompatible with the requested Pod Security mode and highlights which mode
	// is being affected.
	//
	// Possible values are:
	// - PSAConfig: Misconfigured OpenShift Namespace
	// - PSAConfig: PSA label syncer disabled
	// - PSALabel: ServiceAccount with insufficient SCCs
	// +optional
	Reason string `json:"reason,omitempty"`
}
```

The `spec` enables the user to specify their wished state:

- `Privileged`,
- `Conditional` or
- `Restricted`.

While `Privileged` and `Restricted` will enforce their respective states, ignoring the feedback the cluster gives, the `Conditional` `spec.mode` will rely on the lack of violating Namespaces.
`spec.mode = Conditional` will set the `status` mode to `Restricted`, depending on the release and if there are no violations found.
It will set the `status` mode to to `Privileged` otherwise.
As mentioned in the [Release Timing](#release-timing) paragraph, the `status` `Restricted` will mean in release `n` that the PSA label syncer will set the `pod-security.kubernetes.io/enforce` label.
In the release `n+1`, the `status` `Restricted` will mean that the PodSecurity configuration for the kube-apiserver will be set to `Restricted` by default.

To be specific, here is a boolean table of the behavior expected in `status.effectiveMode`:

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

Once a cluster with `spec.mode = Conditional` settles on `Restricted`, it can only turn back to `Privileged` if the user sets `spec.mode = Privileged`.
If a cluster with `spec.mode = Conditional` cluster starts with `status` `Privileged`, it might flip immediately the cluster to `status` `Restricted`, once the cluster has no violations.
For a controlled roll-out, the user would need to set the `spec.mode = Privileged`, and time the setting of `spec.mode = Conditional` to a moment that fits best.

The `status.violatingNamespaces` contains a list of Namespaces that could contain failing workloads.
The reason gives the user the ability to identify which Namespaces are failing and in which which type of failure it is: based on the PSA label syncer or the PSA configuration.
To identify those workloads individually, the user would need to query the kube-apiserver to identify the reason.
Optionally the user could use the [cluster debugging tool](https://github.com/openshift/cluster-debug-tools), which contains the capability to identify those workloads.

### Implementation Details

The cluster-kube-apiserver-operator's PodSecurityReadinessController will be responsible for managing the new API.

If the `FeatureGate` gets removed, the cluster must revert to its previous state.

#### PodSecurity Configuration

Currently the Config Observer for the kube-apiserver manages the state of the Global Config based on templates that change based on the feature flag.
The Config Observer will need to watch the `status.configMode` for `Restricted` and the `FeatureGate` and base the decision upon that.

#### PSA Label Syncer

The PSA label syncer will need to watch the `status.labelMode` for `Restricted` and the `FeatureGate` `OpenShiftPodSecurityAdmission`.

If the `status.labelMode` is set to `Restricted` and the `FeatureGate` `OpenShiftPodSecurityAdmission` is set, the PSA label syncer will set the `pod-security.kubernetes.io/enforce` label.
If the opposite is true, `pod-security.kubernetes.io/enforce` will not be set and labels that are owned by the controller will be removed.

The ability to set the `pod-security.kubernetes.io/enforce` label will happen in release `n`.
Therefore the capability to remove the `pod-security.kubernetes.io/enforce` label must be introduced in release `n-1`.

## Open Questions

### Baseline Clusters

The current suggestion differentiates between `restricted` and `privileged` PSS. It could be possible to have an intermediate step and set the cluster to `baseline` as well.

### Fresh Installs

The System Administrator needs to pre-configure the new API's `spec.mode`, such that the cluster will be `privileged`, `restrited` or `conditional` on fresh install.

## Test Plan

The PSA label syncer maps the SCCs by a hard-coded rule-set to an appropriate PSS.
In addition the PSA version is set to `latest`.
This could mean that the mapping of SCC to PSS gets outdated at some moment in time.
In order to protect user workloads, there is a need for an e2e-test that would fail if this behavior changes.
To be completely sure, it would be required to use the podsecurityadmission package logic directly, which would protect custom SCCs as well.

## Graduation Criteria

- If the label syncer is able to roll out on most of the clusters without any negative feedback. The global config for PSA can be set to `restricted`.
- If most of our users have the global config for PSA set to `restricted`, upgrade can be blocked on clusters that didn't reach that state yet.

## Upgrade / Downgrade Strategy

### On Upgrade

The upgrade strategy is mentioned above in the [Release Timing](#release-timing) section.

### On Downgrade

The downgrade strategy is mentioned in above and in particular for the PSA label syncer in the [Implementation Details](#implementation-details), in the [PSA Label Syncer](#psa-label-syncer) section.

## New Installation

As there should be no violating Namespace on a fresh install, the cluster would promote to `Restricted`, if the `spec.mode = Privileged` is not set. As described above, this wouldn't be the case at first.
Fresh installs will start with `Conditional` as default, to nudge the users to move forward. An Admin should be then able to configure the cluster, such that it starts of with `Privileged`.

## Operational Aspects

- If a cluster is `spec.mode = Conditional` and it is violating at first and the violations are removed step-by-step, a sudden switch in `status.effectiveMode` can happen and result in an upgrade to `Restricted`. It could be preferred by the user to do it in a timed way.
- If a cluster once switches into `status.effectiveMode = Restricted`, having a workload that violates shouldn't be possible anymore. In case that this somehow happens, there should be no switch back to `status.effevtiveMode = Privileged` as an alternating state could cause unnecessary rotations of the kube-apiserver.
- If a user identified issues in their cluster fleet with the `status.effectiveMode = Restricted`, the user can pro-actively set the `spec.mode = Privileged` to stop the enforcement process.
- If a Namespace is violating a user could determine the specific issues by querying the kube-apiserver with `kubectl label ...`
- ClusterAdmins need to be taught to set the proper `securityContext`, if they would like to create direct workloads. It could be made easier, by modifying templates for creation of such workloads.
