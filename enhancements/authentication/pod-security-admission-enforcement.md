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
Enforcement means that the `PodSecurityAdmissionLabelSynchronizationController` sets the `pod-security.kubernetes.io/enforce` label on most customer Namespaces, and the PodSecurityAdmission plugin enforces the `Restricted` [Pod Security Standard (PSS)](https://kubernetes.io/docs/concepts/security/pod-security-standards/) globally on Namespaces without any `pod-security.kubernetes.io/enforce` label.

The new API automatically transitions clusters without Pod Security violations to `Restricted` PSS, while silently keeping clusters with violations in `Legacy` mode.
Users can manually opt out by setting `spec.enforcementMode` to `Legacy` or force an opt-in by setting `spec.enforcementMode` to `Restricted`.
Eventually, all clusters will be required to use `Restricted` PSS.

This enhancement expands the ["PodSecurity admission in OpenShift"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission.md) and ["Pod Security Admission Autolabeling"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission-autolabeling.md) enhancements.

## Motivation

After introducing Pod Security Admission and Autolabeling based on SCCs, some clusters were found to have Namespaces with Pod Security violations.
Over the last few releases, the number of clusters with violating workloads has dropped significantly.
Although these numbers are now quite low, it is essential to avoid any scenario where users end up with failing workloads.

To ensure a safe transition, the `PodSecurityReadinessController` will evaluate a cluster in release `n`.
If potential violations are found by the controller, it will move the cluster into Legacy mode.
Legacy mode maintains the previous non-enforcing state: the kube-apiserver keeps its `Privileged` PSS configuration and the PSA label syncer doesn't set `pod-security.kubernetes.io/enforce` label.
An overview of the Namespaces that led to the decision to move the cluster into Legacy mode will be listed in the API's status, which should help the user to fix any issues.

In release `n+1`, all clusters will be moved into enforcing Pod Security Admission, if `spec.enforcementMode = legacy` isn't set by the user or the `PodSecurityReadinessController`.

The temporary legacy-mode (opt-out from PSA enforcement) exists solely to facilitate a smooth transition.
Once a vast majority of clusters have adapted their workloads to operate under PSA enforcement, `Legacy` mode will be deprecated.

OpenShift strives to offer the highest security standards.
Enforcing PSA ensures that OpenShift is at least as secure as upstream Kubernetes and that OpenShift complies with upstreams security best practices.

### Goals

1. Rolling out Pod Security Admission enforcement.
2. Filter out clusters that would have failing workloads.
3. Allow users to remain in “Legacy” mode for several releases.

### Non-Goals

1. Enabling the PSA label-syncer to evaluate workloads with user-based SCC decisions.
2. Providing a detailed list of every Pod Security violation in a Namespace.
3. Dynamically adjust the enforcement mode based on violating namespaces beyond release `n`.

## Proposal

### User Stories

As a System Administrator:
- I want to transition to enforcing Pod Security Admission only if the cluster would have no failing workloads.
- If there are workloads in certain Namespaces that would fail under enforcement, I want to be able to identify which Namespaces need to be adjusted.
- If I encounter issues with the Pod Security Admission transition, I want to opt out (remain in legacy mode) across my clusters until later.

## Design Details

The design details will describe how to support upgrading clusters to reach PSA enforcement.

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

#### New SCC Annotation: `security.openshift.io/validated-scc-subject-type`

The annotation `openshift.io/scc` currently indicates which SCC admitted a workload, but it does not distinguish **how** the SCC was granted — whether through a user or a Pod’s ServiceAccount.
A new annotation will help determine if a ServiceAccount with the required SCCs was used, or if a user created the workload out of band.
As the PSA label syncer does not track user-based SCCs itself, it cannot correctly label Namespaces under those circumstances.

To address this, the proposal introduces:

```go
// ValidatedSCCSubjectTypeAnnotation indicates the subject type that allowed the
// SCC admission. This can be used by controllers to detect potential issues
// between user-driven SCC usage and the ServiceAccount-driven SCC usage.
ValidatedSCCSubjectTypeAnnotation = "security.openshift.io/validated-scc-subject-type"
```

This annotation will be set in the Pod by the [`SecurityContextConstraint` admission controller](https://github.com/openshift/apiserver-library-go/blob/60118cff59e5d64b12e36e754de35b900e443b44/pkg/securitycontextconstraints/sccadmission/admission.go#L138).

#### Set PSS Annotation: `security.openshift.io/MinimallySufficientPodSecurityStandard`

The PSA label syncer must set the `security.openshift.io/MinimallySufficientPodSecurityStandard` Namespace annotation.
Users can modify `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit`.
If both of these labels are overridden, it is not easily possible to determine which PSS could be enforced.
The new annotation ensures a clear record of the minimal PSS that would be enforced if the `pod-security.kubernetes.io/enforce` label would be set.
As this annotation is being managed by the controller, any change from a user would be reconciled.

If a user disabled the PSA label syncer, with the `security.openshift.io/scc.podSecurityLabelSync=false` annotation, this annotation won't be set.
If such a Namespace is missing `pod-security.kubernetes.io/enforce` label it will be evaluated with the `restricted` PSS.
This is what the PodSecurity Configuration by the kube-apiserver would enforce by default.

#### Update the PodSecurityReadinessController

By adding these annotations, the [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) can more accurately identify potentially failing Namespaces and understand their root causes:

- With the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation, it can evaluate Namespaces that lack the `pod-security.kubernetes.io/enforce` label but have user-overridden warn or audit labels.
  This is important to know as the PSA label syncer still owns `pod-security.kubernetes.io/enforce` in certain cases.
  The `PSAReadinessController` isn't able to deduct easily, which `pod-security.kubernetes.io/enforce` would be set by the PSA label syncer, without that annotation.
- With `ValidatedSCCSubjectType`, the controller can classify issues arising from user-based SCC workloads separately.
  The controller needs to iterate over the workloads in such a namespace and identify if there are workloads that run with user-based SCCs and map those SCCs against PSS.
  Many of the remaining clusters with violations appear to involve workloads admitted by such user-based SCCs.

If violating workloads are found, it will be necessary to create an alert for the user to create a call-to-action.
In particular, we should nudge the user to not use user-based SCCs for workloads.
Informing the user at least a release upfront gives them the ability to prepare for the change.

### New API

This API is used to support users to manage the PSA enforcement.
As this is a transitory process, this API will loose its usefulness once PSA enforcement isn't optional anymore.
The API enables users to:
- offers them the ability to halt enforcement and stay in "Legacy" mode,
- offers them the ability to enforce PSA with the "Restricted" mode,
- offers them the ability to rely on the `PodSecurityReadinessController` to identify failing namespaces in release `n` and set the "Legacy" mode, if necessary and
- offers them the ability to identify the root cause: failing namespaces.

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PSAEnforcementMode defines the Pod Security Standard that should be applied.
type PSAEnforcementMode string

const (
	// PSAEnforcementModeLegacy indicates that the cluster should not enforce PSA restrictions and stay in a privileged mode.
	PSAEnforcementModeLegacy PSAEnforcementMode = "Legacy"
	// PSAEnforcementModeRestricted indicates that the cluster should enforce PSA restrictions, if no violating Namepsaces are found.
	PSAEnforcementModeRestricted PSAEnforcementMode = "Restricted"
)

// PSAEnforcementConfig is a config that supports the user in the PSA enforcement transition.
// The spec struct enables a user to stop the PSA enforcement, if necessary.
// The status struct supports the user in identifying obstacles in PSA enforcement.
type PSAEnforcementConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is a configuration option that enables the customer to dictate the PSA enforcement outcome.
	Spec PSAEnforcementConfigSpec `json:"spec"`

	// status reflects the cluster status with regards to PSA enforcement.
	Status PSAEnforcementConfigStatus `json:"status"`
}

// PSAEnforcementConfigSpec is a configuration option that enables the customer to dictate the PSA enforcement outcome.
type PSAEnforcementConfigSpec struct {
	// enforcementMode gives the user different options:
	// - "" will be interpreted as "Restricted".
	// - Restricted enables the cluster to enforce PSA.
	// - Legacy enables the cluster to opt-out from PSA enforcement for now. Legacy might be set by
    //   the PodSecurityReadinessController if violations are detected. If violations were detected,
    //   the status.violatingNamespaces field will be populated.
	//
	// defaults to ""
	EnforcementMode PSAEnforcementMode `json:"enforcementMode"`
}

// PSAEnforcementConfigStatus is a struct that signals to the user, the current status of PSA enforcement.
type PSAEnforcementConfigStatus struct {
	// enforcementMode indicates the PSA enforcement state:
	// - "" indicates that enforcement is not enabled as of yet.
	// - "Restricted" indicates that enforcement is enabled and reflects the state of the enforcement by the kube-apiserver.
	// - "Legacy" indicates that enforcement won't happen.
	EnforcementMode PSAEnforcementMode `json:"enforcementMode"`

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
	// - Current: The Namespace would currently violate the enforcement mode.
	// - Previous: The Namespace would previously violate the enforcement mode.
	//
	// +optional
	State string `json:"state,omitempty"`

	// lastTransitionTime is the time at which the state transitioned.
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty"`
}
```

If a user encounters `status.violatingNamespaces` it is expected to:

- resolve the violations in the Namespaces to be able to safely move to `Restricted` or
- set the `spec.enforcementMode=Legacy` and solve the violating Namespaces later.

If a user manages several clusters and there are well known violating Namespaces, the `spec.enforcementMode=Legacy` can be set as a precaution.

### Implementation Details

- The `PodSecurityReadinessController` in the `cluster-kube-apiserver-operator` will manage the new API.
- The [`Config Observer Controller`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L131) must be updated to watch for the new API's status alongside the `FeatureGate`.
- The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50) must be updated to watch for the new API's status alongside the `FeatureGate`.
- If the `FeatureGate` `OpenShiftPodSecurityAdmission` is removed from the current `FeatureSet`, all clusters must revert to a global pre-enforcement state.
  It serves as a break-glass mechanism.

#### PodSecurityReadinessController

The `PodSecurityReadinessController` will manage the `PSAEnforcementConfig` API.
It already collects most of the necessary data to determine whether a Namespace would fail enforcement or not to create a [`ClusterFleetEvaluation`](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-fleet-evaluation.md).
With the `security.openshift.io/MinimallySufficientPodSecurityStandard`, it will be able to evaluate all Namespaces for failing workloads, if any enforcement would happen.
With the `security.openshift.io/validated-scc-subject-type`, it can categorize violations more accurately and create a more accurate `ClusterFleetEvaluation`.

If the controller identifies a potentially violating Namespace in release `n`, it will set `spec.enforcementMode = Legacy`.
If `spec.enforcementMode = "Legacy"` is set in release `n+1`, it will set `status.enforcementMode = "Legacy"`.
If `spec.enforcementMode = ""` or `spec.enforcementMode = "Restricted"` is set in release `n+1`, it will set the `status.enforcementMode = Restricted`.

After the initial evaluation of potential Pod Security violations, there won't be any continuous evaluations.
If the `spec.enforcementMode` will be set to `Restricted`, while the `status.enforcementMode` is set to `Legacy`, the `status.enforcementMode` will be updated to `Restricted`.
If the `spec.enforcementMode` will be set to `Legacy`, while the `status.enforcementMode` is set to `Restricted`, the `status.enforcementMode`, will be updated to `Legacy`.

#### PodSecurity Configuration

A Config Observer in the `cluster-kube-apiserver-operator` manages the Global Config for the kube-apiserver.
Without PSA enforcement it sets the `PodSecurity` configuration of the kube-apiserver based on the `OpenShiftPodSecurityAdmission` `FeatureGate`.

The Config Observer must watch:
- the `status.enforcementMode`, 
- the `FeatureGate` `OpenShiftPodSecurityAdmission` and
- check whether all Namespaces that are managed by the `PodSecurityAdmissionLabelSynchronizationController` are labeled.

Only when all conditions are in a `Restricted` mode, will the Config Observer change the kube-apiserver's `PodSecurity` configuration.
This means:
- `status.enforcementMode = Restricted`
- `FeatureGate` `OpenShiftPodSecurityAdmission` is enabled
- all Namespaces that are managed by the `PodSecurityAdmissionLabelSynchronizationController` are labeled.

The `API` and `FeatureGate` state needs to be watched continuously.
If any of those two restrictive values change, the `PodSecurity` configuration must stop enforcing `Restricted` on Namespaces without `pod-security.kubernetes.io/enforce` label.

#### PodSecurityAdmissionLabelSynchronizationController

The [PodSecurityAdmissionLabelSynchronizationController (PSA label syncer)](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) labels all the Namespaces it manages.
Without PSA enforcement it sets the `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit` labels on managed Namespaces.

Namespaces that are **managed** by the `PodSecurityAdmissionLabelSynchronizationController` are Namespaces that:

- are not named `kube-node-lease`, `kube-system`, `kube-public`, `default` or `openshift` and
- are not  prefixed with `openshift-` and
- have no `security.openshift.io/scc.podSecurityLabelSync=false` label set and
- at least one PSA label (including `pod-security.kubernetes.io/enforce`) isn't set by the user or
- if the user sets all PSA labels, it also has set the `security.openshift.io/scc.podSecurityLabelSync=true` label.

The PSA label syncer must watch:
- the `status.enforcementMode`and 
- the `FeatureGate` `OpenShiftPodSecurityAdmission`.

Only when all conditions are in a `Restricted` mode, the PSA label syncer will label managed Namespaces with `pod-security.kubernetes.io/enforce` as evaluated in the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation.
This means:
- `status.enforcementMode = Restricted` and
- `FeatureGate` `OpenShiftPodSecurityAdmission` is enabled.

This state needs to be watched continuously.
If any of the values changes, the `PodSecurity` configuration must stop enforcing `Restricted` on Namespaces without `pod-security.kubernetes.io/enforce` label.

Because the ability to set `pod-security.kubernetes.io/enforce` is introduced, the ability to remove that label must exist in the release before.
Otherwise, the cluster will be unable to revert to its previous state.

## Open Questions

## Rerun the evaluation before enforcing

It could be better to run the evaluation before enforcing `Restricted` mode as the last run can be a couple of months old.
With alerts in place in the release before, it could be sufficient to run the evaluation just before enforcing `Restricted` mode.
In addition we could list the Namespaces that are violating the PSS in the `status.violatingNamespaces` field.

## Initial evaluation by the PodSecurityReadinessController

What happens if the `PodSecurityReadinessController`, didn't have the time to run at least once after an upgrade?

If a cluster upgrades from release `n-1`, through release `n` to release `n+1`, the `PodSecurityReadinessController` might not have time to run at least once in release `n` during the upgrade.
To prevent an unchecked transition into `Restricted` mode on release `n+1`, it will retry checking Namespaces until `status.lastTransitionTime` is set.
Once it ran successfully, it needs to set the `status.lastTransitionTime`.
This will give the user the ability to see how old the basis for the decision is..

## PSA label syncer turned off in legacy clusters

Should a cluster that settles at `status.enforcementMode = Legacy` have the PSA label syncer turned off?

### Impact on HyperShift

Needs to be evaluated.

## Test Plan

### Break-glass mechanism

The break-glass mechanism must be tested to ensure that it works as expected.

### Switch to legacy mode and back

The switch to legacy mode and back must be tested to ensure that it works as expected.

### Non-urgent, but important: Hardcoded SCC to PSA mapping

The PSA label syncer currently maps SCCs to PSS through a hard-coded rule set, and the PSA version is set to `latest`.
This setup risks becoming outdated if the mapping logic changes upstream.
To protect user workloads, an end-to-end test should fail if the mapping logic no longer behaves as expected.
Ideally, the PSA label syncer would use the `podsecurityadmission` package directly.
Otherwise, it can't be guaranteed that all possible SCCs are mapped correctly.

## Graduation Criteria

If `spec.enforcementMode = Restricted` rolls out on most clusters with no adverse effects, we could set `Upgradeable=false` on clusters with `spec.enforcementMode = Legacy`.

## Upgrade / Downgrade Strategy

### On Upgrade

The API needs to be introduced before the controllers start to use it:

- Release `n-1`:
  - Backport the API.
- Release `n`:
  - Enable the `PodSecurityReadinessController` to use the API by setting it's `status` and if violating Namespaces are found also `spec.enforcementMode = Legacy`.
- Release `n+1`:
  - Enable the `PodSecurityAdmissionLabelSynchronizationController` and `Config Observer Controller` to enforce, if:
    - `spec.enforcementMode = Restricted` or `spec.enforcementMode = ""` is set and
    - the `OpenShiftPodSecurityAdmission` `FeatureGate` is enabled.
  - Enable the `OpenShiftPodSecurityAdmission` `FeatureGate`

### On Downgrade

The changes that will be made on enforcement, need to be able to be reverted:

- Release `n`: The `PodSecurityAdmissionLabelSynchronizationController` needs to be able to remove the enforcement labels that will be set in `n+1`.

## New Installation

Fresh installs will default to `Restricted` in release `n+1`.
The System Administrator needs to configure the new API’s `spec.enforcementMode`, for `spec.enforcementMode = Legacy` to revert this.
There is no need for the PodSecurityReadinessController to run as it will start with `spec.enforcementMode = Restricted` or be set to `spec.enforcementMode = Legacy` manually.

## Operational Aspects

### In general

- Administrators facing issues in a cluster already set to a stricter enforcement can change `spec.enforcementMode` to `Privileged` to halt enforcement for other clusters.
- ClusterAdmins must ensure that directly created workloads (user-based SCCs) have correct `securityContext` settings.
  They can't rely on the `PodSecurityAdmissionLabelSynchronizationController`, which only watches ServiceAccount-based RBAC.
  Updating default workload templates can help.
- The evaluation of the cluster happens once every 4 hours with a throttled client in order to avoid a denial of service on clusters with a high amount of Namespaces.
  It could happen that it takes several hours to identify a violating Namespace.
- To identify specific problems in a violating Namespace, administrators can query the kube-apiserver:

  ```bash
  kubectl label --dry-run=server --overwrite $NAMESPACE --all \
      pod-security.kubernetes.io/enforce=$MINIMALLY_SUFFICIENT_POD_SECURITY_STANDARD
  ```

### Setting the `pod-security.kubernetes.io/enforce` label manually

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

### Resolving Violating Namespaces

There are different reasons, why the built-in solution can't set the PSS properly in the Namespace.

#### Namespace name starts with `openshift`

*Hint: The `openshift` prefix is reserved for OpenShift and the PSA label syncer will not set the `pod-security.kubernetes.io/enforce` label.*

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

#### Namespace has disabled PSA synchronization

Namespace has disabled [PSA synchronization](https://docs.openshift.com/container-platform/4.17/authentication/understanding-and-managing-pod-security-admission.html#security-context-constraints-psa-opting_understanding-and-managing-pod-security-admission).
This can be identified by checking the label `security.openshift.io/scc.podSecurityLabelSync=false` in the Namespace manifest.

To solve the issue:

- Enable PSA synchronization with `security.openshift.io/scc.podSecurityLabelSync=true` or
- Set the `pod-security.kubernetes.io/enforce` label manually.

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
