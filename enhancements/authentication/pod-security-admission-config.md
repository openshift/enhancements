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

### Workflow Description

**cluster administrator** is a human user responsible for the security posture of a cluster. They are the only actor that writes `PSAEnforcementConfig.spec`.

**application owner** is a human user who deploys workloads into the cluster. They never interact with this API, but they experience its consequences when a Pod is rejected at admission.

**`PodSecurityReadinessController`** runs in the `cluster-kube-apiserver-operator` and owns `PSAEnforcementConfig.status`.

**Config Observer** runs in the `cluster-kube-apiserver-operator` and renders the kube-apiserver's global `PodSecurity` admission configuration.

**PSA label syncer** runs in the `cluster-policy-controller` and maintains Namespace-level PSA labels and annotations.

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
5. The Config Observer observes that change and re-renders the kube-apiserver configuration with `enforce: restricted`, which cuts a new static pod revision and rolls the control plane one node at a time. The PSA label syncer moves to enforcing mode.
6. The administrator confirms with `oc get psaenforcementconfig cluster -o jsonpath='{.status.enforcementMode}'`.

Variation — violations outstanding: at step 4 the controller leaves `status.enforcementMode` at `Privileged` and sets `EnforcementBlocked=True` with reason `ViolatingNamespaces`. The administrator's request stays on record. Resolving the violations causes the requested mode to take effect on the next evaluation with no further action; alternatively the administrator sets `spec.acknowledgeKnownViolations` to the `resourceVersion` the violations were reported against, which unblocks that specific set of findings and no later ones.

Variation — disabling: the administrator sets `spec.enforcementMode` to `Privileged`, or removes the field. This is applied unconditionally and is never gated on an evaluation, because the escape hatch has to work when the evaluation machinery is exactly what has failed. See [Support Procedures](#support-procedures) for the break-glass procedure and its cost.

Variation — Day 0: the Summary and [New Installation](#new-installation) both assert that enforcement can be requested at install time. The install-time surface is not yet specified; see [Open Questions](#day-0-configuration).

### API Extensions

This enhancement adds one API extension and changes the behaviour of three existing surfaces:

- **New CRD `PSAEnforcementConfig`** — a cluster-scoped singleton carrying the administrator's requested enforcement level in `spec` and the resolved, actually-in-force level in `status`. Group, version, markers and validation are still being settled with the API approvers; the Go sketch below is indicative, not final.
- **The kube-apiserver's `PodSecurity` admission plugin configuration** changes meaning. The `enforce` key becomes derived from `PSAEnforcementConfig.status.enforcementMode` rather than from the `OpenShiftPodSecurityAdmission` feature gate alone. The `audit` and `warn` keys stay pinned to `restricted` and are unchanged.
- **Namespace metadata written by another component.** The PSA label syncer, owned by the cluster-policy-controller, changes which keys it writes: `pod-security.kubernetes.io/enforce` is no longer written by default, and `security.openshift.io/MinimallySufficientPodSecurityStandard` starts being written for Namespaces the syncer does not otherwise control. Namespaces are a core upstream resource; the change is additive for the annotation and subtractive for the label, and existing labels are retained.
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

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is affected, and not in the way an earlier draft of this enhancement assumed. The differences are structural rather than cosmetic, and the following are verified against the `openshift/hypershift` source:

- **There is no `cluster-kube-apiserver-operator` in a hosted control plane.** The control-plane-operator renders the kube-apiserver's PodSecurity configuration directly (`control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go`), keyed off whether `OpenShiftPodSecurityAdmission=true` appears in the rendered feature gate list. The Config Observer mechanism this enhancement builds on therefore has a second, independent implementation in HyperShift that has to change in step with it.
- **The PSA label syncer runs as a management-cluster Deployment**, reconciled as a control-plane component (`control-plane-operator/controllers/hostedcontrolplane/v2/clusterpolicy/component.go`). The hosted-cluster-config-operator (HCCO) runs no syncer logic of its own; what it reconciles is the *guest-cluster* RBAC that the management-side syncer needs in order to write Namespace labels (`control-plane-operator/hostedclusterconfigoperator/controllers/resources/resources.go` and `.../rbac/reconcile.go`). Removing that RBAC, as an earlier draft proposed, would strip permissions from a controller running in a different cluster. Consequently, "watch the new API continuously" means a management-cluster Deployment watching a guest-cluster resource.
- **The `PodSecurityViolation` alert already ships in HCP**, embedded in the HCCO resources and reconciled unconditionally — a copy of the standalone `cluster-kube-apiserver-operator` asset that has already drifted from its source. Any change to that alert has to be made in two places, and this enhancement retains it unchanged in both.
- HCCO opts `kube-system` out of label syncing in the guest cluster, and the management side additionally carries a `PodSecurityAdmissionLabelOverrideAnnotation` and a `restricted-psa` image label handled in `hypershift-operator/controllers/hostedcluster/hostedcluster_controller.go`. Both interact with the enforcement level and need to be reconciled with the new API.

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

This is also the reason the `AllManagedNamespacesLabeled` check is computed by the readiness controller and published as a condition rather than being computed in the observer; see [Namespace labelling coverage](#namespace-labelling-coverage).

On resource consumption, the new recurring cost on SNO is the readiness controller's sweep: one Namespace LIST plus a per-Namespace Pod LIST and the workload-template LISTs added by [Scope of the evaluation](#scope-of-the-evaluation), every four hours, through a client throttled to QPS=2 / Burst=2. Peak memory and CPU for that sweep still need to be quantified for SNO specifically, and the lists need pagination; see [Open Questions](#evaluation-cost-and-pagination).

**MicroShift.** MicroShift is affected and cannot be waved off as not applicable:

- it hardcodes `enforce: restricted` in `assets/controllers/kube-apiserver/defaultconfig.yaml`;
- it vendors and runs the cluster-policy-controller with `"*"` controllers and no feature gates, so it falls through to `NewEnforcingPodSecurityAdmissionLabelSynchronizationController` (`pkg/controllers/cluster-policy-controller.go`) — the label syncer runs in enforcing mode there today;
- it ships the syncer's RBAC and documents the resulting behaviour to users in `docs/user/howto_pod_security.md`;
- it has no CVO, no `FeatureGate` CR and no `PSAEnforcementConfig` CRD. The change that makes the syncer select its mode from the new API's `status` therefore hits a path where the CRD simply does not exist. That path must be a no-op that preserves today's behaviour, not an error or a degraded condition.

The open decision is whether MicroShift keeps `restricted` or follows OpenShift to `privileged`, and, if it is to be configurable, whether the setting surfaces in `/etc/microshift/config.yaml`. A MicroShift reviewer is required.

#### OpenShift Kubernetes Engine

The enablement lever works on OKE. SCCs and the PodSecurity admission plugin are core components present in both products, and `PSAEnforcementConfig` is served by the same kube-apiserver, so an OKE administrator can set `spec.enforcementMode` and have it take effect.

The safety net does not. All three alerts in [Alerts](#alerts) depend on cluster monitoring, which is excluded from the OKE product offering. An OKE administrator who enables `Restricted` gets the enforcement without the staleness signal, without the blocked-enforcement signal and without the fossilised-label signal, leaving `status.conditions` as the only feedback channel — which is pull-based and therefore only seen by someone already looking.

The open decision is whether enabling `Baseline` or `Restricted` on OKE is supported without that alerting, or whether the conditions on the CR are considered sufficient. This needs an answer before GA rather than after, because it determines what the documentation is allowed to recommend.

### Implementation Details/Notes/Constraints

- The `PodSecurityReadinessController` in the `cluster-kube-apiserver-operator` will manage the new API.
- The [`Config Observer Controller`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/218530fdea4e89b93bc6e136d8b5d8c3beacdd51/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L135) must be updated to derive the kube-apiserver's `PodSecurity` configuration from the new API's `status`.
- The [`PodSecurityAdmissionLabelSynchronizationController`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50) must be updated to select its enforcing or advising mode from the new API's `status`.
- Disabling PSA enforcement on a running cluster is done by setting `spec.enforcementMode` to `Privileged` (or omitting the field). It is not done by changing the cluster's `FeatureSet`.

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

The PSA label syncer labels all the Namespaces it manages.
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

##### Three modes, not two

The syncer today has two modes, and they are not sufficient. "Stop maintaining the `enforce` label" and "remove the `enforce` label" are different operations with different authority behind them, and the existing advising mode conflates them — see [Freezing is not the same as removing](#freezing-is-not-the-same-as-removing) for why the current code does neither deterministically. This enhancement therefore defines three modes, selected by `status.enforcementMode` with the `OpenShiftPodSecurityAdmission` gate supplying the default when no opinion has been expressed:

| Mode | Trigger | `enforce` label | `warn` / `audit` |
|---|---|---|---|
| **Enforcing** | `Baseline` or `Restricted` | written and maintained at the level in `security.openshift.io/MinimallySufficientPodSecurityStandard` | written and maintained |
| **Frozen** | gate out of `Default` in `n` and no `enforcementMode` expressed — the state every upgraded cluster lands in | **retained exactly as-is, never updated, never deleted** | written and maintained |
| **Opt-out** | administrator explicitly sets `enforcementMode: Privileged` | **actively removed** wherever the syncer owns it | written and maintained |

The distinction between the middle row and the last is consent, and it is worth being precise about why, because "the upgrade must not relax anything" would contradict this enhancement's own purpose. Release `n` does relax the cluster: it moves the global default from `restricted` to `privileged`. What it does not do is rewrite per-Namespace state that is currently load-bearing. Three reasons:

- **Optional is not the same as off.** Stripping every syncer-written `enforce` label on upgrade would not make enforcement optional; it would replace "every cluster must enforce" with "every cluster must stop enforcing". Optionality means the cluster's existing state persists until an administrator chooses otherwise, and `PSAEnforcementConfig` is how they choose.
- **One direction is reversible and the other is not.** Freezing can be undone by setting `Privileged`, which removes the labels. Stripping cannot be undone: server-side apply deletes the value, and returning to `Restricted` recomputes labels from current SCC and RBAC state rather than restoring what was there. Where only one direction is recoverable, the upgrade should take it.
- **The exposure is compliance, not availability.** Relaxing enforcement can only admit more workloads; it breaks nothing and causes no outage. The risk is a security control disappearing without announcement from clusters that may be attesting to it, and being discovered long afterwards. Weighed against that, requiring one documented administrator action from the clusters that want relief is the cheaper error.

Explicitly setting `Privileged` is that action, and is the only thing that removes labels.

None of the three modes affects OpenShift's own Namespaces. `isNSControlled` ([`podsecurity_label_sync_controller.go#L475-L522`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L475-L522)) excludes them twice: first against the hardcoded payload list in `nsexemptions`, then by skipping any Namespace prefixed `openshift-` outright. Payload Namespaces carry PSA labels from their own manifests, which the syncer never owns and therefore can neither freeze nor remove.

##### The gap the frozen mode cannot close

Freezing labels protects only Namespaces that *have* a label. The kube-apiserver's PodSecurity configuration carries no Namespace exemptions at all — [`defaultconfig.yaml`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/bindata/assets/config/defaultconfig.yaml) exempts exactly one username, `system:serviceaccount:openshift-infra:build-controller` — so any Namespace without its own `enforce` label falls through to the global default and is relaxed when that default moves to `privileged` in release `n`.

`openshift-operators` is exactly that Namespace, and it is the one OLM users install operator bundles into. It is deliberately kept out of the `nsexemptions` list — the list carries an `IMPORTANT:` comment explaining that it must not be exempted — but it is then caught by the `openshift-` prefix skip, so the syncer never labels it. It has neither a syncer-written label nor a manifest-written one, and is enforced at `restricted` today purely by the global default. On upgrade to release `n` its effective level silently becomes `privileged`, and no mode above prevents that.

This is the same `openshift-operators` hole described in [Risks and Mitigations](#risks-and-mitigations), seen from the relaxation side rather than the breakage side. It is not resolved by this enhancement and is [an open question](#openshift-operators-has-no-label-to-freeze).

##### Why the opt-out must remove labels

Per-Namespace PSA labels take precedence over the global default in the kube-apiserver's admission configuration. The config observer only sets that global default from `status.enforcementMode`, so on an upgraded cluster — where essentially every managed Namespace carries a syncer-written `enforce` label — lowering the global default on its own changes nothing at all. Without the opt-out mode the API would be inert on precisely the clusters this enhancement exists to help: the ones already struggling under mandatory enforcement.

This places the opt-out in the label syncer rather than in the config observer, which has a consequence for sequencing. The observer lowering the global default is the cosmetic half of the change; the syncer removing labels is the half that takes effect. The handshake between the two is described in [Cross-operator ordering](#cross-operator-ordering), and the ordering requirement for lowering is the reverse of the one for raising.

Removing labels is also not free to undo. `Privileged` → `Restricted` → `Privileged` is not a round trip: the first transition deletes labels, the second recomputes and rewrites them from current SCC and RBAC state, which may differ from what was deleted. Administrators toggling the field should expect the second `Privileged` to act on a different set of Namespaces than the first.

##### What release `n` inherits

An earlier draft required release `n-1` to be able to remove the `pod-security.kubernetes.io/enforce` labels set in release `n`. That requirement has been dropped: the label is not introduced by release `n`. It is written today, by the syncer's enforcing default, on every cluster with `OpenShiftPodSecurityAdmission` in `Default`. Release `n` stops writing it on newly evaluated Namespaces rather than starting to.

What release `n` does inherit is the labels already on upgraded clusters. Under the three modes above those labels are retained by design, and the frozen mode is what has to be built to guarantee it.

#### Existing Clusters

Every cluster running today has `OpenShiftPodSecurityAdmission` in its `Default` feature set, and the label syncer's **enforcing** constructor is the default branch ([`psalabelsyncer.go#L17-L50`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go#L17-L50)) — advising mode requires explicitly passing `OpenShiftPodSecurityAdmission=false`. So `pod-security.kubernetes.io/enforce` labels are already present across the fleet, applied via server-side apply under the field manager `pod-security-admission-label-synchronization-controller` ([`podsecurity_label_sync_controller.go#L386`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L386)).

Moving the syncer to advising mode stops it writing new labels. It does not remove the existing ones, and no code path in any component removes them.

##### The labels are retained

Deleting them on upgrade would silently reduce the enforcement posture of clusters that are relying on it, which is a compliance event for customers under FedRAMP, PCI or DISA STIG. Retaining them is the safer default and is what this enhancement does.

Retaining them unchanged, however, introduces a regression of its own. Consider a cluster upgraded into release `n`, where `some-namespace` is labelled `pod-security.kubernetes.io/enforce: restricted` by the syncer before the upgrade:

1. An administrator grants that Namespace's ServiceAccount the `anyuid` SCC, expecting to run a workload that needs a fixed UID.
2. Before release `n`, the syncer would have recomputed the Namespace's minimally sufficient standard as `baseline` and relaxed the label accordingly.
3. In release `n` the syncer no longer writes the enforce label, so it stays pinned at `restricted`.
4. The workload is rejected at admission, on a cluster whose administrator never opted into this feature and has no reason to associate the failure with an upgrade.

The label is now a fossil: it records a decision made by a controller that has stopped maintaining it.

##### Annotation-only mode

To prevent that, the syncer is not switched off. It continues to run in **annotation-only** mode: it keeps computing each Namespace's minimally sufficient standard and writing it to the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation, while no longer writing `pod-security.kubernetes.io/enforce`.

Two changes are required beyond the mode switch:

- **The annotation write is decoupled from the label write.** Today both happen in the same apply and `sync` returns early for Namespaces where `isNSControlled` is false ([`podsecurity_label_sync_controller.go#L204`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L204)), so uncontrolled Namespaces — including every `openshift-`-prefixed one — receive no annotation at all and have no computed baseline to compare against. The annotation must be written independently of whether the syncer owns the Namespace's labels.
- **This does not alter enforcement anywhere.** The annotation is diagnostic and is not read by any admission path. In particular, payload Namespaces set their PSA labels explicitly in their own CVO manifests — `openshift-kube-apiserver-operator` is pinned `restricted` and `openshift-kube-apiserver` `privileged` — so the `Restricted`-by-default expectation that OpenShift components are developed against is held by those manifests, not by the syncer or the global default, and is unaffected. For such Namespaces the annotation may report a *lower* minimally sufficient standard than the label enforces. That is informational and must not be read as a recommendation to relax a deliberately pinned Namespace.

##### Detecting fossilised labels

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

##### Release note

The change in default posture, the retention of existing labels, and the new alert are called out in the release notes for release `n`, since an upgraded cluster's effective behavior changes without any administrator action.

#### New Installation

Fresh installs won't have PSA enabled by default and will be disabled.
If the system administrator does not configure PSA at install time, `spec.enforcementMode` is unset, which resolves to `Privileged`.
This means that the system administrator can electively increase the value of `spec.enforcementMode` to the level of enforcement they are comfortable with.
The System Administrator needs to either disable PSA or configure the new API’s `spec.enforcementMode`, for `spec.enforcementMode = Privileged` to revert this.
There is no need for `PodSecurityReadinessController` to run as OpenShift workloads and namespaces should have been labelled appropriately. <u>It would need to run if enabled in existing clusters or when workloads are added as there will be existing customer workloads and namespaces.</u>

### Risks and Mitigations

- **The out-of-the-box security posture is reduced.** Clusters that take no action end up less restrictive than they are today. *Mitigation:* per-Namespace `pod-security.kubernetes.io/*` labels continue to take precedence and are unaffected; SCCs, OpenShift's primary workload admission control, are unchanged; existing enforce labels on upgraded clusters are retained rather than removed; and the change is called out in the release note for release `n`. This risk is not fully mitigated by design — it is the trade this enhancement makes — and it requires explicit security sign-off before the EP is marked implementable.
- **A clean evaluation is not a guarantee.** PSA is validating-admission only, so a workload that is not running at evaluation time, or that is recreated later by a drain or upgrade, can fail long after an administrator was told the cluster was clean. *Mitigation:* workload templates are evaluated alongside live Pods, and `warn`/`audit` stay pinned to `restricted` so violations keep surfacing continuously. See [Scope of the evaluation](#scope-of-the-evaluation).
- **An empty violation list can mean "no violations" or "never evaluated".** *Mitigation:* the `Evaluated` and `StatusStale` conditions gate any raise in enforcement, and repeated evaluation failure degrades the `kube-apiserver` ClusterOperator. See [Freshness is an interlock, not a hint](#freshness-is-an-interlock-not-a-hint).
- **`openshift-operators` is deliberately non-exempt from the syncer but is skipped by the `openshift-` prefix rule**, so once the syncer stops writing enforce labels that Namespace has neither a computed label nor a manifest-pinned one, and falls to the global default. On a cluster that opts into `Restricted`, every operator bundle needing more than restricted then fails admission. **This is currently unmitigated**: the happy path of the feature is a mass-breakage path for OLM users. Decoupling the annotation write makes the situation diagnosable but does not fix it. OLM and layered-product reviewers are required, and the likely mitigation — keeping the syncer enforcing for `openshift-*` Namespaces while disabling it for user Namespaces — still has to be written down and agreed.
- **Two operators reconcile against the same inputs on independent timing.** If the Config Observer raises the global level before the syncer has stamped labels, workloads in those Namespaces are rejected; the reverse ordering is required when lowering. *Mitigation:* the `AllManagedNamespacesLabeled` condition is intended as the handshake, but the full coordination protocol is not yet specified. See [Open Questions](#cross-operator-ordering).
- **Every enforcement change rolls the kube-apiserver.** On SNO that is an API outage, including on the disable path. *Mitigation:* lowering is unconditional so it is never blocked, the cost is documented in the break-glass procedure, and the observer's inputs are debounced.
- **The subject-type annotation is absent on pre-existing Pods**, which makes both possible readings wrong — see [Open Questions](#annotation-coverage-on-upgraded-clusters). Unmitigated pending that decision.

Security review is required from the OpenShift security architecture group, covering the default-posture change specifically rather than the API. UX review is required from the console and docs teams for the `oc` workflow in [Workflow Description](#workflow-description) and for the wording of the advisory `Warning` header.

### Drawbacks

- It ships a less secure default than the product has today, and no mechanism preserves the current default for clusters that take no action. For a product whose positioning includes "secure by default", that is a real cost and not only a documentation problem.
- It adds a second place to look for PSA state. The effective configuration already lives in the kube-apiserver's admission config, in Namespace labels and in the feature gate; a new CRD adds a fourth, with its own reconcile loop and its own staleness semantics. One optional field on an existing config resource would avoid that; see [Alternatives (Not Implemented)](#alternatives-not-implemented).
- It bundles three separable changes — the already-shipped SCC annotation work, the retirement of the label syncer's enforcing mode, and the new configuration API. Approving the API implicitly approves the retirement, which has by far the largest blast radius of the three and is currently receiving the least review attention.
- Retained enforce labels become unmaintained. The fossilised-label alert makes them visible, but the underlying situation — a label written by a controller that no longer updates it — is a new class of cluster state that support will have to reason about for as long as those clusters live.
- **The feature does nothing on an upgraded cluster until the administrator acts.** Because existing `enforce` labels are frozen rather than removed, a cluster that upgrades into release `n` sees no change to any Namespace that already has an effective level. The clusters most in need of relief — those already struggling under mandatory enforcement — get it only after explicitly setting `enforcementMode: Privileged`. This is the deliberate trade argued in [Three modes, not two](#three-modes-not-two), but it means the headline claim "PSA enforcement is now optional" is true of the product and not yet true of any given upgraded cluster, and the release note has to carry that distinction.
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

### PSA label syncer turned off

The PSA Label Syncer will now be turned off by default on all clusters. This is because its use case was to allow customers to automatically migrate customers to `Restricted` PSS. As the default will now be `Privileged` on customer workloads, it is no longer required.

As the expected default PSS for OpenShift developers will remain `Restricted`, they will remain using the PSA label syncer to ensure OpenShift workloads still comply, such as in monitor and periodic tests.

This is stated in several places in this document with different scope — "retired", "off by default on all clusters", "still used by OpenShift developers", "annotation-only mode" — and those statements are not yet reconciled. What is needed is a truth table over {unset or `Privileged`, `Baseline`, `Restricted`} × {payload Namespace, non-exempt `openshift-*` Namespace, user Namespace} saying, for each cell, whether the syncer is active and what it writes.

### `openshift-operators` has no label to freeze

Described in [The gap the frozen mode cannot close](#the-gap-the-frozen-mode-cannot-close): `openshift-operators` is non-exempt but prefix-skipped, so it carries no `enforce` label from any source and is enforced at `restricted` today only by the global default. Release `n` relaxes it to `privileged` with no signal and no way to freeze it.

Options are to add it to the PodSecurity configuration's Namespace exemptions, to make the syncer manage it as a named exception to the prefix skip, or to have OLM ship an explicit label on the Namespace it owns. The last is the most honest about who owns the decision but requires an OLM-side change and an OLM reviewer, neither of which is currently in this enhancement. Whichever is chosen, the release note for `n` must state the change in effective level for this Namespace explicitly.

### HyperShift configuration surface

Is the administrator-facing surface for a hosted cluster a guest-cluster `PSAEnforcementConfig` CR, or a field under `HostedCluster.spec.configuration`? The recommendation is `spec.configuration`, matching existing `ClusterConfiguration` plumbing, but this determines RBAC, tenancy and whether a tenant may change their own enforcement level at all. See [Hypershift / Hosted Control Planes](#hypershift--hosted-control-planes).

### Cross-operator ordering

The Config Observer (in `cluster-kube-apiserver-operator`) and the label syncer (in `cluster-policy-controller`) watch the same inputs on independent reconcile timing. Raising the global level before Namespaces are labelled rejects workloads; lowering requires the opposite order. The `AllManagedNamespacesLabeled` condition is intended as a one-way handshake — the syncer publishes "all managed Namespaces labelled as of generation G", the observer acts only on that — but the generation semantics, and the behaviour on restart, lost watch and leader-election change, are not yet specified.

### Evaluation cost and pagination

The evaluation adds a per-Namespace Pod LIST, and [Scope of the evaluation](#scope-of-the-evaluation) adds workload-template LISTs on top. These are currently unpaginated, with no per-sweep timeout, no backoff and no defined partial-completion behaviour. A truncated `violatingNamespaces` is indistinguishable from "fewer violations found", which is precisely the signal that causes an administrator to enable enforcement. Publication needs to be all-or-nothing, or explicitly marked as partial, and peak memory and CPU need quantifying for SNO and for a large cluster.

### Day 0 configuration

The Summary, Motivation and [New Installation](#new-installation) all state that enforcement can be requested at install time, but no install-config field, manifest name or bootstrap rendering path is specified, and no installer reviewer is assigned. There is also a bootstrap hazard: a day-1 manifest sets an enforcement level before the readiness controller has ever run, which is the exact situation the `Evaluated` interlock exists to prevent.

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

This tier changes only the default. It does not remove the enforcement code paths, and it does not depend on the `PSAEnforcementConfig` API, which is still `TechPreviewNoUpgrade` at this point.

### Tier 2: graduating the opt-in configuration

In release `n+1`, once the default has moved and clusters are running with enforcement off, we graduate the opt-in path — the `PSAEnforcementConfig` API and the observer and syncer wiring that consume it.

This is a combined API and feature-gate promotion:

- `PSAEnforcementConfig` moves from `v1alpha1` to `v1`, with the `v1alpha1` version retained and served for one release for anyone who adopted it under TechPreview.
- `PodSecurityAdmissionConfiguration` moves from `TechPreviewNoUpgrade` to `Default`.
- API validation integration tests land alongside the `openshift/api` change, as required for all API changes.

### Dev Preview -> Tech Preview

This feature does not pass through Dev Preview. The API is introduced directly as `v1alpha1` behind `PodSecurityAdmissionConfiguration` in `TechPreviewNoUpgrade` in release `n`, because the behaviour it configures — PSA enforcement — already ships and is already exercised across the fleet; what is new is the configuration surface, not the enforcement.

Entering Tech Preview requires:

- the API type merged in `openshift/api` with its feature gate, and API validation integration tests alongside it;
- the `PodSecurityReadinessController` populating `status` end to end, including the `Evaluated`, `StatusStale`, `EnforcementBlocked` and `AllManagedNamespacesLabeled` conditions;
- the Config Observer and the label syncer both selecting behaviour from `status`, with the absent-CRD path exercised;
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

This enhancement removes no deprecated API. It does deprecate a behaviour: the label syncer's enforcing mode stops being the default in release `n`, and `pod-security.kubernetes.io/enforce` labels the syncer previously wrote stop being maintained.

Because the behaviour is removed from the default rather than from the code, the deprecation is announced through the release note for release `n` described in [Release note](#release-note), and the resulting unmaintained labels are made visible through the `PodSecurityNamespaceOverRestricted` alert rather than being deleted. Whether the enforcing mode is eventually removed from the code entirely — and whether that retirement is reversible — is not decided by this enhancement.

## Upgrade / Downgrade Strategy

### On Upgrade

#### Release plan

No change is backported to release `n-1`. An earlier draft proposed backporting the API there; that has been dropped, because release `n-1` contains no consumer of the API and because the CRD and any stored object survive a downgrade regardless of whether `n-1` shipped the type.

- Release `n`:
  - Remove `OpenShiftPodSecurityAdmission` from the `Default` feature set. This is the change that makes PSA enforcement optional: the config observer's fallback moves from `restricted` to `privileged`, and the label syncer's default moves from enforcing to advising.
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

The sequence on an upgraded cluster is therefore: the kube-apiserver rolls to a revision whose PodSecurity block says `privileged`, because `OpenShiftPodSecurityAdmission` is no longer in the `Default` feature set; the `cluster-policy-controller` restarts in advising mode for the same reason; and nothing further happens until an administrator creates a `PSAEnforcementConfig`. There is no window in which enforcement is *raised* as a side effect of the upgrade.

#### The relaxation is not retroactive

The global default only applies to Namespaces that carry no `pod-security.kubernetes.io/enforce` label. On a cluster upgraded from `n-1` most Namespaces do carry one, written by the enforcing label syncer, so those Namespaces keep enforcing at their existing level even though the cluster-wide default has moved to `privileged`.

That is intended. Release `n` changes the default; it does not retroactively rewrite Namespaces that already have an effective level, and the syncer's frozen mode exists to guarantee it. The reasoning, including why this is not a contradiction of the enhancement's purpose, is in [Three modes, not two](#three-modes-not-two). The practical consequence is that an upgraded cluster is opt-in for *new* Namespaces and unchanged for existing ones until the administrator acts.

##### Freezing is not the same as removing

The existing code does not guarantee it, and this is the main piece of implementation work the enhancement adds to the `cluster-policy-controller`. What becomes of the labels today is decided by server-side apply, not by any cleanup code. The syncer applies with field manager `pod-security-admission-label-synchronization-controller` and rebuilds its apply configuration from scratch on every sync from its `syncedLabels` map, which in advising mode contains `warn` and `audit` only. Server-side apply prunes fields a manager previously owned and no longer sends, so the `enforce` label *is* dropped — but only on the next apply, and `shouldUpdate` skips the apply entirely unless one of the values the syncer still tracks has drifted. A Namespace whose `warn`, `audit` and `MinimallySufficientPodSecurityStandard` values are all already correct keeps its `enforce` label indefinitely. A Namespace that happens to see an unrelated SCC or RBAC change six months later loses it at that moment.

So the current advising mode delivers neither of the two behaviours the design calls for. It is not freezing, because the labels are eventually deleted; and it is not opting out, because the deletion happens at an arbitrary time per Namespace and never at all where the label is co-owned. The cluster's posture decays non-deterministically, and two identically configured clusters diverge based on unrelated churn.

Frozen mode must therefore stop the pruning rather than merely stop the writing. Two mechanisms are available, and the choice is an implementation detail rather than a design one: apply under a field manager that never owned `enforce`, so there is no ownership to relinquish; or relinquish ownership without deleting the value, for which the syncer already has precedent in the ownership-transfer sequence at [`podsecurity_label_sync_controller.go#L260-L287`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L260-L287) that migrates fields between managers with a paired `Update` and `Apply`.

Because the labels are retained, the release note for `n` cannot say "PSA enforcement is now opt-in" without qualification. On an upgraded cluster it is opt-in for *new* Namespaces; existing Namespaces keep their current level until the administrator sets `enforcementMode: Privileged`, which is what triggers removal. The `PodSecurityNamespaceOverRestricted` alert described in [Removing a deprecated feature](#removing-a-deprecated-feature) makes the retained labels visible in the meantime.

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
| Namespace `pod-security.kubernetes.io/{enforce,warn,audit}` and `-version` labels | `enforce` unmaintained and decaying; `warn`/`audit` maintained by the advising syncer | `n-1`'s syncer runs enforcing, so it re-adds and re-owns `enforce` at the minimally sufficient level on every Namespace it controls | the label syncer, via server-side apply | none for syncer-controlled Namespaces; Namespaces opted out with `security.openshift.io/scc.podSecurityLabelSync: false`, or whose labels are owned by another field manager, keep whatever `n` left and must be fixed by hand |
| Namespace `security.openshift.io/MinimallySufficientPodSecurityStandard` | maintained — the syncer writes it in both enforcing and advising mode | continues to be maintained, unchanged | the label syncer | none |
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

The producer and the consumers of `status.enforcementMode` are in different operators that roll independently during an upgrade. `cluster-kube-apiserver-operator` runs the `PodSecurityReadinessController` and the config observer; the `cluster-policy-controller` runs the label syncer but is deployed as a container in the kube-controller-manager static pod, owned by `cluster-kube-controller-manager-operator`. There is no ordering guarantee between the two, so for part of every upgrade one of them is at `n` and the other at `n-1`.

The consequence is bounded because neither side reads the other's output directly — both read the same declarative inputs — but the enum is the sharp edge. `Baseline` is a value this enhancement introduces. A consumer from an earlier release that encounters it must **preserve its existing behaviour**, not error and not treat the field as unset: treating an unknown value as unset would silently relax enforcement, and erroring would degrade a ClusterOperator over a value that is valid on the cluster's own API server. Concretely:

- Consumers switch on the values they know and fall through to "make no change to the currently effective level" for anything else.
- The `enforcementMode` enum is only ever extended, never has a value removed or repurposed, for the reasons in [API Extensions](#api-extensions).
- Because the API server validating the write is always at least as new as any consumer, a value can be accepted by validation and be unknown to a consumer. That asymmetry is the normal case during an upgrade, not a bug to be designed out.

### HyperShift skew

HyperShift has no `cluster-kube-apiserver-operator`; the management-cluster `control-plane-operator` renders the hosted kube-apiserver's configuration directly, and it carries its own copy of the privileged-versus-restricted decision. The management cluster is required to be at or ahead of the hosted cluster's release, so the skew is one-directional and can span several releases: a `control-plane-operator` at `n+2` may be rendering the kube-apiserver for a hosted cluster whose payload — and whose `cluster-policy-controller` and `PSAEnforcementConfig` consumers — is at `n`.

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
	After that is done, the PSA label syncer will update the PSA labels.
- Otherwise set the `pod-security.kubernetes.io/enforce` label manually.

## Infrastructure Needed

New periodic CI lanes are needed to satisfy the graduation criteria: the feature has to be exercised in both the `TechPreviewNoUpgrade` and `Default` Prow variants, across the provider, topology, architecture and network variants required by `dev-guide/feature-zero-to-hero.md`, at a frequency that reaches the required number of runs per platform before branch cut. The exact lane list follows from the reworked [Graduation Criteria](#graduation-criteria) and is not yet enumerated.
