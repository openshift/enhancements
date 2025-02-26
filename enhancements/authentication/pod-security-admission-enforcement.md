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

To ensure a safe transition, this proposal suggests that if a potential failure of workloads is being detected in release `n`, that the `cluster-kube-apiserver-operator` moves into `Upgradeable=false`.
The user would need to either resolve the potential failures, setting a higher PSS label for that Namespace or set the enforcing mode to `Privileged` for now in order to be able to upgrade.
`Privileged` will keep the cluster in the previous state, the non enforcing state. This means that the PodSecurity Configuration of the kube-apiserver is set to `Privileged` PSS and that the PSA label syncer doesn't set the `pod-security.kubernetes.io/enforce` label.
In the following release `n+1`, the controller will then do the actual enforcement, if `Restricted` is set.

An overview of the Namespaces with failures will be listed in the API's status, should help the user to fix any issues.

The temporary `Privileged` mode (opt-out from PSA enforcement) exists solely to facilitate a smooth transition. Once a vast majority of clusters have adapted their workloads to operate under `Restricted` PSS, maintaining the option to run in `Privileged` mode would undermine these security objectives.

OpenShift strives to offer the highest security standards. Enforcing PSS ensures that OpenShift is at least as secure as upstream Kubernetes and that OpenShift complies with upstreams security best practices.

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

## Design Details

### Improved Diagnostics

In order to make more accurate predictions about violating Namespaces, which means it would have failing workloads, it is necessary to improve the diagnostics.

The [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md) revealed that certain clusters would fail enforcement.
While it can be distinguished if a workload would fail or not, the explanation is not always clear.
A likely root cause is that the [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) (PSA label syncer) does not label Namespaces that rely on user-based SCCs.
In some other cases, the evaluation was impossible because PSA labels had been overwritten by users.
Additional diagnostics are required to confirm the full set of potential causes.

While the root causes need to be identified in some cases, the result of identifying a violating Namespace is understood.

#### New SCC Annotation: `security.openshift.io/validated-scc-subject-type`

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

This annotation will be set in the Pod by the [`SecurityContextConstraint` admission](https://github.com/openshift/apiserver-library-go/blob/60118cff59e5d64b12e36e754de35b900e443b44/pkg/securitycontextconstraints/sccadmission/admission.go#L138) plugin.

#### Set PSS Annotation: `security.openshift.io/MinimallySufficientPodSecurityStandard`

The PSA label syncer must set the `security.openshift.io/MinimallySufficientPodSecurityStandard` Namespace annotation.
Because users can modify `pod-security.kubernetes.io/warn` and `pod-security.kubernetes.io/audit`, these labels do not reliably indicate the minimal standard.
The new annotation ensures a clear record of the minimal PSS that would be enforced if the `pod-security.kubernetes.io/enforce` label were set.
As this annotation is being managed by the controller, any change from a user would be reconciled.

#### Update the PodSecurityReadinessController

By adding these annotations, the [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) can more accurately identify potentially failing Namespaces and understand their root causes:

- With the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation, it can evaluate Namespaces that lack the `pod-security.kubernetes.io/enforce` label but have user-overridden warn or audit labels.
  If `pod-security.kubernetes.io/enforce` label is overriden, the PSA label syncer still owns `pod-security.kubernetes.io/enforce` in certain cases.
  The `PSAReadinessController` isn't able to deduct in that situation, which `pod-security.kubernetes.io/enforce` would be set by the PSA label syncer, without that annotation.
- With `ValidatedSCCSubjectType`, the controller can classify issues arising from user-based SCC workloads separately.
  Many of the remaining clusters with violations appear to involve workloads admitted by user SCCs.


### New API

This API is used to support users to enforce PSA.
As this is a transitory process, this API will loose its usefulness once PSA enforcement isn't optional anymore.
The API offers two things to the users:
- offers them the ability to halt enforcement and
- offers them the ability to identify failing namespaces.

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PSAEnforcementMode defines the Pod Security Standard that should be applied.
type PSAEnforcementMode string

const (
	// PSAEnforcementModePrivileged indicates that the cluster should not enforce PSA restrictions and stay in Privileged mode.
	PSAEnforcementModePrivileged PSAEnforcementMode = "Privileged"
	// PSAEnforcementModeRestricted indicates that the cluster should enforce PSA restrictions, if no violating Namepsaces are found.
	PSAEnforcementModeRestricted PSAEnforcementMode = "Restricted"
)

// PSAEnforcementConfig is a config that supports the user in the PSA enforcement transition.
// The spec struct enables a user to stop the PSA enforcement, if necessary.
// The status struct supports the user in identifying obstacles in PSA enforcement.
type PSAEnforcementConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is a configuration option that enables the customer to influence the PSA enforcement outcome.
	Spec PSAEnforcementConfigSpec `json:"spec"`

	// status reflects the cluster status wrt PSA enforcement.
	Status PSAEnforcementConfigStatus `json:"status"`
}

// PSAEnforcementConfigSpec is a configuration option that enables the customer to influence the PSA enforcement outcome.
type PSAEnforcementConfigSpec struct {
	// enforcementMode gives the user different options:
	// - Restricted enables the cluster to move to PSA enforcement, if there are no violating Namespaces detected.
	//   If violating Namespaces are found, the operator moves into "Upgradeable=false".
	// - Privileged enables the cluster to opt-out from PSA enforcement for now and it resolves the operator status of "Upgradeable=false" in case of violating Namespaces.
	//
	// defaults to "Restricted"
	EnforcementMode PSAEnforcementMode `json:"enforcementMode"`
}

// PSAEnforcementConfigStatus is a struct that signals to the user, if the cluster is going to start with PSA enforcement and if there are any violating Namespaces.
type PSAEnforcementConfigStatus struct {
	// enforcementMode indicates if PSA enforcement will happen:
	// - "Restricted" indicates that enforcement is possible and will happen.
	// - "Privileged" indidcates that either enforcement will not happen:
	//   - either it is not wished or
	//   - it isn't possible without potentially breaking workloads.
	EnforcementMode PSAEnforcementMode `json:"enforcementMode"`

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
}
```

Here is a boolean table with the expected outcomes:

| `spec.enforcementMode` | length of `status.violatingNamespaces` | `status.enforcementMode` | `OperatorStatus`  |
| ---------------------- | -------------------------------------- | ------------------------ | ----------------- |
| Privileged             | More than 0                            | Privileged               | AsExpected        |
| Privileged             | 0                                      | Privileged               | AsExpected        |
| Restricted             | More than 0                            | Privileged               | Upgradeable=False |
| Restricted             | 0                                      | Restricted               | AsExpected        |

If a user encounters `status.violatingNamespaces` it is expected to:

- resolve the violating Namespaces in order to be able to `Upgrade` or
- set the `spec.enforcementMode=Privileged` and solve the violating Namespaces later.

If a user manages several clusters and there are well known violating Namespaces, the `spec.enforcementMode=Privileged` can be set as a precaution.

### Implementation Details

- The `PodSecurityReadinessController` in the `cluster-kube-apiserver-operator` will manage the new API.
- The [`Config Observer Controller`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L131) must be updated to watch for the new API's status alongside the `FeatureGate`.
- The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50) must be updated to watch for the new API's status alongside the `FeatureGate`.
- If the `FeatureGate` `OpenShiftPodSecurityAdmission` is removed from the current `FeatureSet`, the cluster must revert to its previous state.
  It serves as a break-glass mechanism.

#### PodSecurityReadinessController

The `PodSecurityReadinessController` will manage the `PSAEnforcementConfig` API.
It already collects most of the necessary data to determine whether a Namespace would fail enforcement or not to create a [`ClusterFleetEvaluation`](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-fleet-evaluation.md).
With the `security.openshift.io/MinimallySufficientPodSecurityStandard`, it will be able to evaluate all Namespaces for failing workloads, if any enforcement would happen.
With the `security.openshift.io/ValidatedSCCSubjectType`, it can categorize violations more accurately and create a more accurate `ClusterFleetEvaluation`.

#### PodSecurity Configuration

A Config Observer in the `cluster-kube-apiserver-operator` manages the Global Config for the kube-apiserver, adjusting behavior based on the `OpenShiftPodSecurityAdmission` `FeatureGate`.
It must watch both the `status.enforcementMode` for `Restricted` and the `FeatureGate` `OpenShiftPodSecurityAdmission` to be enabled to make decisions.

#### PodSecurityAdmissionLabelSynchronizationController

The [PodSecurityAdmissionLabelSynchronizationController (PSA label syncer)](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) must watch the `status.enforcementMode` and the `OpenShiftPodSecurityAdmission` `FeatureGate`.
If `spec.enforcementMode` is `Restricted` and the `FeatureGate` `OpenShiftPodSecurityAdmission` is enabled, the syncer will set the `pod-security.kubernetes.io/enforce` label on Namespaces that it manages.
Otherwise, it will refrain from setting that label and remove any enforce labels it owns if existent.

Namespaces that are **not managed** by the `PodSecurityAdmissionLabelSynchronizationController` are Namespaces that:

- Are prefixed with `openshift`,
- Have the label `security.openshift.io/scc.podSecurityLabelSync=false`.
- Have the `pod-security.kubernetes.io/enforce` label set manually.
- Are not a run-level zero Namespace:
  - `kube-system`,
  - `default` or
  - `kube-public`.

Because the ability to set `pod-security.kubernetes.io/enforce` is introduced, the ability to remove that label must exist in the release before.
Otherwise, the cluster will be unable to revert to its previous state.

## Open Questions

### PSA label syncer vs Global Config

The Global Config is a PodSecurity configuration given to the kube-apiserver, which will enforce `Restricted` at some point.
This means that all Namespaces without a `pod-security.kubernetes.io/enforce` label, will be object to the decision of the Global Config.
Most user workspaces rely on the PSA label syncer to label their Namespaces with the `pod-security.kubernetes.io/enforce` label.
If both get turned on at the same time, the Global Config could be enforced on Namespaces that were not yet labeled iby the PSA label syncer.

A simpel solution would be to release the PSA label syncer in OCP `n+1` and the Global Config in `n+2` (while `n` as a reference is the introduction of the controller that manages the API).

A more complex solution that could enable a release of PSA label syncer and Global Config enforcement, would be to maintain two kind of violating Namespaces lists:

- The first list, lists all Namespaces that would violate, if the PSA label syncer would enforce PSA.
- The second list, lists all Namespaces that would violate, if the Global Config would enforce PSA.

The Global Config list is a list of Namespaces that don't get handled by the PSA label syncer and require a PSS that is higher than `Restricted`.
To be able to determine this, we would need to pull the "is managed by PSA label syncer" logic and put it into library-go, such that it can be evaluated by the PodSecurityReadinessController.
Another approach would be to set yet another annotation, which would indicate if the PSA label syncer has ownership of that Namespace.

### Fresh Installs

Needs to be evaluated. The System Administrator needs to pre-configure the new API’s `spec.enforcementMode`, choosing whether the cluster will be `Privileged` or `Restricted` during a fresh install.

### Impact on HyperShift

Needs to be evaluated.

### Baseline Clusters

The current suggestion differentiates between `Restricted` and `Privileged` PSS.
It may be possible to introduce an intermediate step and set the cluster to `baseline` instead.

### Enforce PSA labe syncer, fine-grained

It would be possible to enforce only the `pod-security.kubernetes.io/enforce` labels on Namespaces without enforcing it globally through the `PodSecurity` configuration given to the kube-apiserver.
It would be possible to enforce `pod-security.kubernetes.io/enforce` labels on Namespaces that we know wouldn't fail.

## Test Plan

The PSA label syncer currently maps SCCs to PSS through a hard-coded rule set, and the PSA version is set to `latest`.
This setup risks becoming outdated if the mapping logic changes upstream.
To protect user workloads, an end-to-end test should fail if the mapping logic no longer behaves as expected.
Ideally, the PSA label syncer would use the `podsecurityadmission` package directly.
Otherwise, it can't be guaranteed that all possible SCCs are mapped correctly.

## Graduation Criteria

If `spec.enforcementMode = Restricted` rolls out on most clusters with no adverse effects, the ability to avoid `Upgradeable=false` with violating Namespaces by setting `spec.enforcementMode = Privileged` will be removed.

## Upgrade / Downgrade Strategy

### On Upgrade

The API needs to be introduced before the controllers start to use it:

- Release `n-1`:
  - Backport the API.
- Release `n`:
  - Enable the `PodSecurityReadinessController` to use the API by setting it's `status`.
- Release `n+1`:
  - Enable the `PodSecurityAdmissionLabelSynchronizationController` and `Config Observer Controller` to enforce, if:
    - there are no potentially failing workloads (indicated by violating Namespaces) and
    - the `OpenShiftPodSecurityAdmission` `FeatureGate` is enabled.
  - Enable the `OpenShiftPodSecurityAdmission` `FeatureGate`

### On Downgrade

The changes that will be made on enforcement, need to be able to be reverted:

- Release `n`: The `PodSecurityAdmissionLabelSynchronizationController` needs to be able to remove the enforcement labels that will be set in `n+1`.

## New Installation

TBD

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
  - it isn't supported that a user creates a Namespace with the `openshift` prefix and
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

#### Namespace workload doesn't use ServiceAccount SCC

Namespace workload doesn't use ServiceAccount SCC, but receives the SCCs by the executing user.
This usually happens, when a workload isn't running through a deployment with a properly set up ServiceAccount.
A way to verify that will be to check the `security.openshift.io/validated-scc-subject-type` annotation on the Pod manifest.

To solve the issue:

- Update the ServiceAccount to be able to use the necessary SCCs.
	The necessary SCC can be identified in the annotation `security.openshift.io/scc` of the existing workloads.
	After that is done, the PSA label syncer will update the PSA labels.
- Otherwise set the `pod-security.kubernetes.io/enforce` label manually.
