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
last-updated: 2026-10-01
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

[Pod Security Admission (PSA)](https://kubernetes.io/docs/concepts/security/pod-security-admission/) enforcement is on by default in OpenShift today. **This enhancement turns it off by default and makes it opt-in**, through a new `podSecurityAdmission` field on the existing `config.openshift.io/v1 APIServer` resource that records the [Pod Security Standard (PSS)](https://kubernetes.io/docs/concepts/security/pod-security-standards/) the administrator wants enforced, and the PSS version it is evaluated against.

Three changes deliver this, and **all three are carried by a single feature gate** — `OpenShiftPodSecurityAdmission`, redefined rather than replaced. They land together or not at all:

1. **The global `enforce` level becomes `privileged` instead of `restricted`**, and is thereafter derived from `spec.podSecurityAdmission` rather than from the gate.
2. **The PSA label syncer is retired**, in every mode, on every cluster. It does not run at `Privileged` and it does not come back when an administrator opts in.
3. **The `PodSecurityReadinessController` is retired with it.** It exists to decide whether a cluster can be moved to enforcement automatically, and to report the fleet's readiness for that migration. The migration is cancelled, so the question it answers is no longer asked — see [Retiring the PodSecurityReadinessController](#retiring-the-podsecurityreadinesscontroller).

Because one gate governs all three, **a cluster is either entirely in the old world or entirely in the new one**, and there is no release in which enforcement is optional but un-re-enableable. The cost is that the gate's meaning is redefined in place, which is the sharpest hazard in this document: see [Role of the `OpenShiftPodSecurityAdmission` feature gate](#role-of-the-openshiftpodsecurityadmission-feature-gate) and [Gate redefinition skew](#gate-redefinition-skew).

The first two go together historically: the syncer exists *because* the global default is `restricted`, as the thing that keeps a Namespace whose ServiceAccounts cannot meet that standard from having its workloads rejected. Remove the mandatory `restricted` default and its damage control goes with it — see [Why it is retired outright](#why-it-is-retired-outright). The third follows from the same cancellation rather than from the syncer: the readiness controller has only a weak dependency on the syncer and would keep working without it.

**The result is a write-only API.** `spec.podSecurityAdmission` carries two fields and `APIServerStatus` stays the empty struct it is today. Nothing in this enhancement writes a status, a condition, a metric or an alert.

This is a deliberate reduction in the out-of-the-box security posture, traded for the guarantee that no cluster acquires failing workloads without its administrator opting in. It requires explicit security sign-off, a [release note](#release-note), and the migration handling in [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy).

This enhancement expands the ["PodSecurity admission in OpenShift"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission.md) and ["Pod Security Admission Autolabeling"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission-autolabeling.md) enhancements.

Terms used throughout: **opting in** is `spec.podSecurityAdmission.enforceLevel: Baseline` or `Restricted`; **opting out** is `Privileged` or leaving the field unset, which are the same state and the shipped default. `spec.enforceLevel` is shorthand for the full path on `apiserver/cluster`. Disabling enforcement means **exactly `enforce: privileged`** — the `PodSecurity` plugin stays loaded, `warn` and `audit` stay pinned to `restricted` so violations stay observable, and per-Namespace labels keep taking precedence. The requested level is applied as requested: there is no evaluation to wait for and no condition to clear, because the third change above removes the component that would have supplied them. What an administrator gets instead is set out in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).

## Motivation

After PSA and SCC-based autolabeling were introduced, some clusters were found to have Namespaces with Pod Security violations. The number has dropped significantly over the last several releases, but it is not zero, and it is essential to avoid any scenario where users end up with failing workloads.

Making enforcement elective puts the onus on the administrator to have PSA-compliant workloads before enforcement is turned on, rather than discovering non-compliance as admission failures after an upgrade. Out of the box, `Privileged` is the global default and neither the label syncer nor the readiness controller runs.

Both of those components served the same cancelled plan. The syncer existed to make a mandatory `restricted` default survivable; the readiness controller existed to tell Red Hat which clusters were ready to be moved to enforcement and to find, through [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md), what was stopping the rest. Neither question survives delegating the decision to the administrator: nobody is being moved, so there is no readiness to assess and no fleet rollout to sequence.

OpenShift strives to offer the highest security standards, and PSA enforcement is on by default today in service of that. This enhancement trades that default for predictability: rather than enforcing a standard a minority of clusters cannot meet, it gives administrators an explicit, reversible lever. Clusters that opt in reach the same posture as today; clusters that do not remain at `Privileged` until their administrator chooses otherwise.

The cost of that trade is that clusters taking no action end up less restrictive than they are today. Mitigating it: per-Namespace `pod-security.kubernetes.io/*` labels continue to take precedence and are unaffected, and SCCs — OpenShift's primary workload admission control — are unchanged.

### Goals

1. Allow users to configure Pod Security Admission enforcement on their clusters.
2. Prevent clusters from acquiring failing workloads by making this feature elective.
3. Allow users to enable and disable this feature at will.
4. **Retire the PSA label synchronization controller outright** — on every cluster, in every enforcement mode — and retire the `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation with it. This is not conditional on the new API and is not reversed by opting in: it happens whatever `spec.enforceLevel` says. It has the largest blast radius of anything in this enhancement, and reviewing the API without reviewing this has not reviewed the change.
5. **Retire the `PodSecurityReadinessController`**, and with it the PSA ClusterFleetEvaluation. Also unconditional, and also not reversed by opting in.

### Non-Goals

1. **Keeping PSA enforcement enabled by default.** Enforcement is on by default today; this enhancement deliberately makes it opt-in, and no mechanism preserves the current default for clusters that take no action.
2. **Reporting PSA readiness in the API.** Nothing on the cluster answers "would this cluster survive `restricted`?" as a standing value. `APIServerStatus` gains no field, no condition and no count — not a bounded summary and not a list of Namespaces. The question is answered on demand with the server-side dry-run in [Finding the violating Namespaces](#finding-the-violating-namespaces), and continuously-but-indirectly by the `audit` signal; see [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).
3. **Gating the requested level on a clean evaluation.** An administrator who asks for `Restricted` gets `restricted`, whether or not the cluster would pass it. No acknowledgement token, no blocked state, no condition the administrator must clear first. With the readiness controller retired there is nothing left that could gate it.
4. **Restricting per-Namespace PSA configuration.** Per-Namespace labels continue to take precedence over the cluster-wide default. This enhancement only changes the global default applied to Namespaces carrying no `enforce` label.
5. **Replacing the PSA fleet signal.** No metric, alert or telemetry series is added to recover what ClusterFleetEvaluation reported. The pre-existing `PodSecurityViolation` alert and the kube-apiserver's own `pod_security_evaluations_total` are what remain, and neither was introduced here.

## Proposal

### User Stories

As a System Administrator:

- I want to electively enable PSA enforcement only if the cluster would have no failing workloads.
- If workloads in certain Namespaces would fail under enforcement, I want to identify which Namespaces need adjusting.
- If I enable PSA enforcement and decide I can no longer use it, I want to disable it across my clusters at will.

### Workflow Description

The **cluster administrator** is responsible for the security posture of a cluster and is the only actor that writes `spec.podSecurityAdmission`, and the only actor that reads it back. The **application owner** deploys workloads; they never touch this API but experience its consequences when a Pod is rejected. The **Config Observer** in `cluster-kube-apiserver-operator` is the single consumer, turning `spec` into the kube-apiserver's global `PodSecurity` admission configuration. The **PSA label syncer** and the **`PodSecurityReadinessController`** are not actors: neither runs in any enforcement mode, and neither reads this API. The syncer appears below only as the source of labels already present on upgraded clusters.

The starting state is a cluster on release `n+1` or later with `enforceLevel` unset, resolving to `Privileged`. Enabling enforcement:

1. The administrator establishes whether the cluster can survive the level they want, using the dry-run sweep in [Finding the violating Namespaces](#finding-the-violating-namespaces) and the standing `audit` signal. **This step is entirely theirs**; the cluster holds no precomputed answer and will not object if they skip it. See [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).
2. They resolve whatever the sweep found, or decide to accept it.
3. They record their intent:

   ```bash
   oc patch apiserver cluster --type=merge \
       -p '{"spec":{"podSecurityAdmission":{"enforceLevel":"Restricted"}}}'
   ```

   The write is validated for shape and nothing else. **It is never rejected on the strength of what the cluster contains, and never warned about** — no component knows enough to warn.
4. The Config Observer re-renders the kube-apiserver configuration with `enforce: restricted`, cutting a new static pod revision that rolls the control plane one node at a time. Nothing else starts: no controller begins writing Namespace labels, and every Namespace without an `enforce` label of its own is now held to `restricted`.
5. The administrator confirms the rollout finished, by comparing each master's `currentRevision` against `latestAvailableRevision` on `kubeapiserver/cluster`. **Re-reading `apiserver/cluster` tells them nothing**: `spec` is what they just wrote and there is no status to consult. Confirming the effective level means reading the revisioned `config-<revision>` ConfigMap; see [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect).

**Variation — pinning the standard's version.** `enforceVersion` fixes which revision of the Pod Security Standards is applied, for all three of `enforce`, `warn` and `audit`. Left unset it tracks the payload's Kubernetes version, so the standard tightens as the cluster is upgraded. A value newer than the cluster's own version is clamped silently, with the same consequence as above: the ConfigMap is the only place the applied value appears.

**Variation — disabling.** The administrator sets `Privileged`, or removes the field. The global `enforce` level returns to `privileged`; nothing else about PSA changes, and Namespaces that already carry an `enforce` label keep enforcing at that label's level. Like every other level change it is applied as requested, and the path is now about as short as it can be made: one field, read by one observer, with no controller in between whose failure could hold it up. On an upgraded cluster this may not be enough — see [Retained labels on upgraded clusters](#retained-labels-on-upgraded-clusters).

**Variation — Day 0.** Enforcement can in principle be requested at install time; the surface is not yet specified. See [Open Questions](#day-0-configuration).

### API Extensions

This enhancement adds one optional field to an existing API and changes three existing surfaces. No admission webhooks, conversion webhooks, aggregated API servers or finalizers are introduced, and **no status is written anywhere**.

- **`spec.podSecurityAdmission` on `config.openshift.io/v1 APIServer`** — the installer-rendered cluster-scoped singleton `apiserver/cluster`, carrying the administrator's requested level and PSS version, gated on the redefined `OpenShiftPodSecurityAdmission` feature gate. **No new CRD and no new feature gate is introduced**; both alternatives are weighed in [Alternatives](#alternatives-not-implemented).
- **The kube-apiserver's `PodSecurity` admission plugin configuration** changes meaning in two ways: `enforce` becomes derived from `spec.podSecurityAdmission.enforceLevel` rather than from the `OpenShiftPodSecurityAdmission` feature gate alone, and the three `*-version` keys become derived from `spec.podSecurityAdmission.enforceVersion` rather than fixed. `audit` and `warn` stay pinned to the `restricted` *level*.
- **Namespace metadata written by another component.** The SCC-mapping PSA label syncer, owned by `cluster-policy-controller`, no longer runs, so no `pod-security.kubernetes.io/*` label and no `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation is written by it. Namespaces are a core upstream resource; the change is purely subtractive, and metadata already present is retained untouched. **The separate `privileged-namespaces-psa-label-syncer` is not affected** — see [Two syncers, only one retired](#two-syncers-only-one-retired).
- **Six conditions leave the `kube-apiserver` ClusterOperator.** The `PodSecurity*EvaluationConditionsDetected` family is written by the `PodSecurityReadinessController`, which stops running, so all six stop being set and the existing ones are removed on upgrade. This is a **public status surface being withdrawn**, not reshaped: see [What stops being reported](#what-stops-being-reported).

**`APIServerStatus` stays an empty struct** ([`types_apiserver.go#L270`](https://github.com/openshift/api/blob/master/config/v1/types_apiserver.go)), and `cluster-kube-apiserver-operator` remains a pure *reader* of `config.openshift.io`. Retiring the readiness controller removed the only candidate writer, so rather than find a status subtree a new owner it was dropped; the one thing it would have carried that `spec` does not is the clamped `enforceVersion`, which is not worth being the first writer of that type.

#### API version and availability

Throughout this document, release `n` is the release in which PSA enforcement stops being the default, currently targeted at **OpenShift 5.1**; `n-1` is 5.0 and `n+1` is 5.2.

`APIServer` is already `config.openshift.io/v1` and is present on every standard cluster, so what ships per release is the **field**, not the type. It carries `+openshift:enable:FeatureGate=OpenShiftPodSecurityAdmission` — the same gate that governs the behaviour, which is the whole point of the single-gate design — so it is in `TechPreviewNoUpgrade` in release `n` and in `Default` from release `n+1`. There is no API version migration: nothing is promoted from `v1alpha1`, no two versions are served at once, and no conversion is written.

The configuration surface is therefore **not** available on a supported, upgradeable cluster in release `n` — but neither is the behaviour it configures. **Release `n` changes nothing for a `Default` cluster.** The gate is removed from `Default` at the same moment its meaning is redefined, precisely so that the redefinition does not ship to the fleet before it has been exercised; a `Default` cluster on release `n` enforces `restricted`, runs the syncer and runs the readiness controller, exactly as it does on `n-1`. Everything in this enhancement arrives for those clusters when the gate returns to `Default` in release `n+1`.

This is the property that justifies one gate. There is no release in which enforcement is off by default with no supported way to turn it back on, so no cluster is ever in a state this enhancement cannot describe, and `unsupportedConfigOverrides` is not needed as a transitional opt-in. The price is that the posture change cannot ship ahead of the API even if its prerequisites — payload Namespace labelling and SCC pinning — are ready first; the two-release alternative that would have allowed that is recorded in [Alternatives](#ship-the-posture-change-a-release-ahead-of-the-api).

#### The `podSecurityAdmission` fields

**An `openshift/api` PR must be open and linked from this section before the EP moves to `implementable`**; the types below are the proposal, not the merged shape.

```go
package v1 // config/v1/types_apiserver.go in openshift/api

type APIServerSpec struct {
	// ... existing fields: servingCerts, clientCA, additionalCORSAllowedOrigins,
	// encryption, tlsSecurityProfile, tlsAdherence, audit ...

	// podSecurityAdmission configures the Pod Security Standard the
	// kube-apiserver enforces on Namespaces that carry no
	// pod-security.kubernetes.io/enforce label of their own.
	//
	// When omitted, this means the cluster has opted out of Pod Security
	// Admission enforcement. The PodSecurity admission plugin remains
	// configured, with warn and audit pinned to the restricted level.
	//
	// +openshift:enable:FeatureGate=OpenShiftPodSecurityAdmission
	// +optional
	PodSecurityAdmission PodSecurityAdmissionConfig `json:"podSecurityAdmission,omitempty,omitzero"`
}

// APIServerStatus is unchanged by this enhancement and stays empty.

// PodSecurityStandard names one of the upstream Pod Security Standards.
// +kubebuilder:validation:Enum:=Privileged;Baseline;Restricted
type PodSecurityStandard string

const (
	// PodSecurityStandardPrivileged is the unrestricted standard. As an
	// enforcement level it means the cluster has opted out of enforcement.
	PodSecurityStandardPrivileged PodSecurityStandard = "Privileged"
	// PodSecurityStandardBaseline prevents known privilege escalations while
	// permitting the default Pod configuration.
	PodSecurityStandardBaseline PodSecurityStandard = "Baseline"
	// PodSecurityStandardRestricted is the hardened standard, following current
	// Pod hardening best practices.
	PodSecurityStandardRestricted PodSecurityStandard = "Restricted"
)

// PodSecurityAdmissionConfig records the administrator's requested Pod Security
// Admission enforcement. It is never written by a controller.
type PodSecurityAdmissionConfig struct {
	// enforceLevel is the Pod Security Standard enforced on Namespaces that
	// carry no pod-security.kubernetes.io/enforce label of their own. A
	// Namespace that carries one is governed by that label and is unaffected by
	// this field.
	//
	// This field selects the global enforce level and nothing else. No value
	// starts the PSA label syncer: it is retired, and nothing computes a
	// per-Namespace enforce label, so a Namespace that cannot meet the
	// requested standard must carry its own.
	//
	// The requested level is applied as written and is validated for shape
	// only. No component evaluates whether the cluster's workloads can meet
	// it, so this field is never rejected or warned about on the strength of
	// what the cluster contains; establishing that beforehand is the
	// administrator's responsibility.
	//
	// When omitted, this means Privileged. Omitting this field and setting it
	// to Privileged are the same state and no consumer may distinguish them.
	// The default is a fixed commitment rather than a platform choice that
	// may change: a cluster that takes no action must never acquire
	// enforcement it did not request.
	//
	// +optional
	EnforceLevel PodSecurityStandard `json:"enforceLevel,omitempty"`

	// enforceVersion pins the revision of the Pod Security Standards applied,
	// as a Kubernetes minor version such as "v1.34", or "latest". It applies to
	// the enforce, warn and audit levels together, including when enforceLevel
	// is Privileged, where it still selects the revision warn and audit report
	// against.
	//
	// A version newer than the cluster's own Kubernetes version is clamped
	// down to it rather than rejected, so that one configuration can be
	// applied across a fleet at mixed releases. The field is left as written
	// and the clamp is reported nowhere: this API has no status, so the
	// applied value appears only in the kube-apiserver's rendered admission
	// configuration, in the config-<revision> ConfigMap. A cluster silently
	// enforcing an older standard than the one pinned here is
	// indistinguishable, through this API, from one honouring the pin.
	//
	// When omitted, this means the cluster's own Kubernetes version, so the
	// standard tightens as the cluster is upgraded. Pin this field to hold it
	// still.
	//
	// +kubebuilder:validation:Pattern=`^(latest|v1\.(0|[1-9][0-9]*))$`
	// +kubebuilder:validation:MaxLength=10
	// +optional
	EnforceVersion string `json:"enforceVersion,omitempty"`
}

```

Three things the doc comments state but do not justify. `spec` is owned exclusively by the administrator and written by no controller, so `apiserver/cluster` stays pure intent. The enum deliberately omits `""`: unlike `FeatureSet` it has a named value for its default, and adding `""` would give one state two spellings. And the API is write-only *by consequence, not by preference* — the reporting had an owner, the owner is retired, and no substitute was invented to keep the shape; a status subtree can be added back compatibly later, which is the right way round.

"Absent" arises in three shapes during the rollout and all resolve to `Privileged`: the field is not in the schema (release `n` without TechPreview); `apiserver/cluster` does not exist (MicroShift); the field is present and unset. A consumer that treats a field missing from the schema differently from one left unset has a bug.

#### Deciding whether it is safe to opt in

This is the question the `PodSecurityReadinessController` was built to answer, and from release `n` **nothing on the cluster answers it as a standing value**. Two things replace it. The `audit` signal is continuous: `warn` and `audit` stay pinned to `restricted` in every mode, so every would-be violation is counted in `pod_security_evaluations_total{decision="deny",mode="audit"}` and already fires the pre-existing [`PodSecurityViolation`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/alerts/podsecurity-violations.yaml) alert. The dry-run in [Finding the violating Namespaces](#finding-the-violating-namespaces) is point-in-time but covers workloads that are not currently running.

Neither covers a workload that is neither running nor swept, so there is no longer a machine-readable cluster-level answer to "would this cluster survive `restricted`?". That is accepted: the evaluation existed to instrument a migration Red Hat would perform, and delegating the decision to the administrator retires the migration along with the question. **Reviewers who think the standing signal should outlive the migration that justified it should say so** — it is recorded as an alternative in [Keep the readiness controller](#keep-the-readiness-controller).

#### Finding the violating Namespaces

With no evaluation on the cluster, this loop is **the** diagnostic — not a way of getting detail the API summarised, which is what it was in the previous revision:

```bash
for ns in $(oc get ns -l '!pod-security.kubernetes.io/enforce' -o name); do
    oc label --dry-run=server --overwrite "$ns" \
        pod-security.kubernetes.io/enforce=restricted 2>&1 \
        | grep -q 'violate' && echo "$ns"
done
```

Substitute `=baseline` on a cluster considering `Baseline`; unlike the retired sweep, which was pinned at `restricted` and so gave a `Baseline` administrator nothing usable, the dry-run answers at whatever level is asked. Drop the label selector to include Namespaces carrying their own `enforce` label, which the sweep could never see.

Three obligations follow, and they are heavier now that this is the only mechanism: the loop belongs in `openshift-docs` rather than left for an administrator to derive; must-gather must collect its output, or a support bundle contains nothing at all about PSA readiness; and because it is `O(Namespaces)` serial server round-trips, **whether a documented shell loop is an adequate deliverable at all is an [open question](#is-a-shell-loop-an-adequate-diagnostic)** on the clusters where the answer matters most.

#### `enforceVersion` and why the default moves

`enforceVersion` pins the revision of the Pod Security Standards, and applies to `enforce`, `warn` and `audit` together — a cluster warned against one revision and enforced against another would report violations it does not have, or miss ones it does. It is never inert: at `Privileged` it still selects the revision `warn` and `audit` report against, which is the whole of the diagnostic signal in that mode, so the break-glass path never requires two edits.

**Omitted means the payload's Kubernetes version, which is a moving target.** The standards gain checks between releases, so a cluster that pins nothing is held to a slightly stricter standard after each upgrade and can acquire a violation from an upgrade alone. That sits uneasily beside this enhancement's thesis that no cluster should acquire failing workloads without opting in. It is accepted because it is what the cluster does today — the observer already pins `latest`, which has exactly this property — and because the alternative, freezing each cluster at its install-time revision, would strand the fleet on a standard that ages out of step with the product. **Pinning is the cure and the documentation should present it that way.**

**A version the cluster cannot evaluate is clamped, not rejected.** Validation accepts any well-formed `v1.X`, including one ahead of the cluster's own Kubernetes version — an administrator pinning ahead of a planned upgrade, or a GitOps repository applied across a fleet at mixed releases. The operator clamps to the payload's version when substituting and leaves the field alone, so a pin survives a downgrade and is restored on the way back up; rejecting the write instead would make a legitimate pin fail on whichever cluster happened to be oldest. Why clamping is a safety requirement rather than a convenience is in [PodSecurity Configuration](#podsecurity-configuration).

**The clamp is invisible**, as the field's doc comment says: a cluster at 1.34 given `v1.36` enforces 1.34's standards and looks, through this API, exactly like one honouring the pin. The applied value is recoverable only from the revisioned `config-<revision>` ConfigMap, which is a support action rather than something read alongside the field. It is carried as a [drawback](#drawbacks) rather than mitigated — a status field is the subtree that was just dropped, an Event is cheap but expires, and rejecting the write defeats the purpose of clamping — and **should be settled before GA**, since it is the one place where dropping status costs an administrator something.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is affected structurally. The following are verified against the `openshift/hypershift` source:

- **There is no `cluster-kube-apiserver-operator` in a hosted control plane.** The control-plane-operator renders the kube-apiserver's PodSecurity configuration directly (`.../v2/kas/config.go`), keyed off whether the literal string `OpenShiftPodSecurityAdmission=true` appears in the rendered feature gate list. The Config Observer mechanism this enhancement builds on has a second, independent implementation that has to be inverted in step with it — and, because the gate is redefined rather than added, a *string match against a gate list from a different release* is exactly the construct the redefinition breaks. HyperShift is the only place in the product where that boundary is crossable, and it needs a bounded version gate: see [Gate redefinition skew](#gate-redefinition-skew). This is the single largest piece of HyperShift work in the enhancement.
- **The PSA label syncer runs as a management-cluster Deployment**, reconciled as a control-plane component (`.../v2/clusterpolicy/component.go`); the hosted-cluster-config-operator reconciles the *guest-cluster* RBAC it needs. The syncer is not retired unconditionally — it is retired when the gate is on — so HCP's change is a conditional, carrying the same version gate as the configuration above, rather than a straight removal. The guest-cluster RBAC must therefore stay: it is needed on every hosted cluster below `n`, and on any hosted cluster that is downgraded. Removing it is a later cleanup tied to the same sunset, not part of this work.
- **The `PodSecurityViolation` alert already ships in HCP**, embedded in the HCCO resources — a copy of the standalone asset that has already drifted from its source. Any change has to be made twice; this enhancement retains it unchanged in both, which matters more now that it is one of only two PSA signals anywhere.
- HCCO opts `kube-system` out of label syncing in the guest cluster, which becomes inert. The management side additionally carries a `PodSecurityAdmissionLabelOverrideAnnotation` and a `restricted-psa` image label in `hostedcluster_controller.go`; both interact with the enforcement level and need reconciling with the new API.

**Dropping status resolves HyperShift's hardest problem for free.** `ClusterConfiguration` carries *specs only* — `HostedCluster.spec.configuration.apiServer` is a `*configv1.APIServerSpec` and there is no `HostedCluster.status.configuration` for anything to write back into. A previous revision owed HyperShift either a new status surface on `HostedCluster` or a status written into the guest cluster where the wrong audience would read it. A spec-only API needs neither: the one field reaches a hosted cluster through machinery that already exists, and there is nothing left that wants a path back.

What remains open is behaviour under management/guest skew ([Version Skew Strategy](#version-skew-strategy)). **A HyperShift reviewer is still required before this enhancement is marked implementable**, but the dependency is now a review rather than a design decision.

#### Standalone Clusters

Standalone is the primary topology; everything outside this section is written against it. Disconnected and bare-metal clusters are unaffected: no component involved reaches outside the cluster, and the only egress-adjacent surface — the `pod_security_enforcement_mode` telemetry metric — degrades to not being reported like any other.

#### Single-node Deployments or MicroShift

**Single Node OpenShift.** Every change to the effective level re-renders the kube-apiserver configuration. With one kube-apiserver that rollout is a short API outage rather than a rolling update — including the rollout that *disables* enforcement, which is precisely the path an administrator reaches for when something is already wrong, so the cost has to be stated in the break-glass procedure. The observer's only inputs are two spec fields and a feature gate, none of which varies with cluster activity, so there is no flapping input that could cut revisions in a loop.

SNO is also where retiring the readiness controller pays most directly: the four-hourly sweep — one Namespace LIST plus a per-Namespace Pod LIST and the workload-template LISTs, through a client throttled to QPS=2 / Burst=2 — was this enhancement's only new recurring resource cost, and the one whose peak memory and CPU a single-node cluster could least afford. It goes, and with it the open question about bounding it. What an SNO administrator loses is the same thing everyone loses: the sweep was also the only PSA diagnostic that did not require them to run something.

**MicroShift** is affected and cannot be waved off. It hardcodes `enforce: restricted` in `assets/controllers/kube-apiserver/defaultconfig.yaml`; it vendors and runs `cluster-policy-controller` with `"*"` controllers and **no feature gates at all**, so it reaches the syncer through the `default:` arm of the switch — `NewEnforcingPodSecurityAdmissionLabelSynchronizationController` — rather than through either gate value; it ships the syncer's RBAC and documents the behaviour in `docs/user/howto_pod_security.md`; and it has no CVO, no `FeatureGate` CR and no installer-rendered `APIServer` singleton, so none of this enhancement's configuration surface reaches it. Moving the API onto an existing config resource does not change this: MicroShift serves no such resource.

**That absent arm is a hard constraint on the inversion**, and it is the easiest thing in this enhancement to get wrong. The switch in `runPodSecurityAdmissionLabelSynchronizationController` has three arms — gate explicitly `false`, gate explicitly `true`, and gate absent — and only the first two invert. The third must keep meaning *enforcing*, unchanged, because it is MicroShift's only path to the syncer and MicroShift never sets the gate either way. A reviewer reading the inversion as "swap the branches" will take the absent arm with them and silently disable the syncer on every MicroShift deployment, which with `enforce: restricted` hardcoded would break non-compliant Namespaces on an edge product with no opt-out. The absent arm needs a comment saying so and a test of its own; it is listed as a risk rather than left to the diff.

Beyond the mechanics, MicroShift is the one topology where the syncer is unambiguously load-bearing: with `enforce: restricted` hardcoded, the computed per-Namespace labels are the only thing keeping non-compliant Namespaces admitting workloads. Three outcomes are possible and the choice is MicroShift's: **follow OpenShift**, consistent but a posture change for an edge product whose users did not ask for one, without the opt-back-in standalone gets; **keep `restricted` and keep the syncer**, which is what preserving the absent arm delivers by default and what this enhancement assumes; or **keep `restricted` and drop the syncer**, requiring every MicroShift user to label their own Namespaces, a breaking change needing its own deprecation.

Single-gating makes the default outcome the safe one: because the syncer's code has to survive for the gate-off path anyway, keeping it for MicroShift costs nothing extra, where under a design that deleted the code it would have cost a fork. A MicroShift reviewer is still required — the first and third outcomes are theirs to choose — but the enhancement no longer blocks on the answer, because the second is what happens if nobody decides.

#### OpenShift Kubernetes Engine

OKE behaves as Standalone above; the API and the enforcement path are identical. The one difference is that cluster monitoring is excluded from the OKE offering, so the `PodSecurityViolation` alert and the metric it reads are unavailable and the dry-run sweep is the only diagnostic an OKE administrator has. **Whether opting in is supportable on that basis needs an answer before GA.**

### Implementation Details/Notes/Constraints

The changes, by component:

| Component | Change |
|---|---|
| `openshift/api` | `podSecurityAdmission` on `APIServerSpec` in `config/v1/types_apiserver.go`, gated on `OpenShiftPodSecurityAdmission`; that gate removed from `inDefault()` and `inOKD()` in `features.go`, retained in `inTechPreviewNoUpgrade()` and `inDevPreviewNoUpgrade()` |
| `cluster-kube-apiserver-operator` — Config Observer | **invert both branches** of `observePodSecurityAdmissionEnforcement`: gate off → `restricted` as today; gate on → level from `spec.enforceLevel` defaulting to `privileged`, and the three `*-version` keys from `spec.enforceVersion` clamped to the payload's Kubernetes version; add a `Baseline` branch |
| `cluster-kube-apiserver-operator` — `PodSecurityReadinessController` | start it only when the gate is **off**; when on, do not start it and remove its six ClusterOperator conditions if present |
| `cluster-policy-controller` | in `runPodSecurityAdmissionLabelSynchronizationController`, **invert the two explicit branches** — `=true` stops starting the syncer, `=false` starts it enforcing — and **leave the absent/`default:` branch enforcing**, which is MicroShift's path |
| `hypershift` — `control-plane-operator` | invert the `kas/config.go` PodSecurity branch to match, and version-gate it per [Gate redefinition skew](#gate-redefinition-skew) |

**No new RBAC is needed anywhere.** The operator reads `apiserver/cluster` today and continues only to read it; the previous revision's `update` and `patch` on `apiservers/status` go with the status subtree.

Neither retired controller is wired to the new API: `cluster-policy-controller` gains no watch, no client and no dependency on `apiserver/cluster`, and the readiness controller does not become conditional on `enforceLevel`. Both are conditional on the gate alone.

#### Already shipped: SCC provenance annotations

Two annotations defined in [`openshift/api`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/security/v1/consts.go#L9-L20) already exist and are **not new work**. Neither has a consumer after this enhancement, which is a change from the previous revision, where the readiness controller read both.

**`security.openshift.io/validated-scc-subject-type`** records whether a Pod's SCC was granted through its ServiceAccount or through the user's own roles — the distinction that explains one of the two root causes behind most violating Namespaces found by ClusterFleetEvaluation. It is set at admission by the [SCC admission plugin](https://github.com/openshift/apiserver-library-go/blob/42e5e402ca430b5072f9d6fd5df275aff9eaf5ce/pkg/securitycontextconstraints/sccadmission/admission.go#L400-L403) and stays shipped and maintained; it is simply no longer read by anything, since the controller that classified violations by it is gone. It remains useful to a human debugging a rejection. **The open question about its coverage on upgraded clusters is closed by this change**, not answered: nothing now has to decide what an absent value means.

**`security.openshift.io/MinimallySufficientPodSecurityStandard`** is written by the syncer at [`#L375-L380`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L375-L380) and does not survive. See [Retiring the PSA label syncer](#retiring-the-psa-label-syncer).

#### Role of the `OpenShiftPodSecurityAdmission` feature gate

This enhancement **redefines an existing gate rather than adding one**, and that is the single most consequential decision in the document. It is what lets all three changes land atomically, and it is also the only place where an older component can misread a newer cluster — bounded, and bounded to HyperShift, in [Gate redefinition skew](#gate-redefinition-skew).

Today the gate means *PSA enforcement is on by default*, and **both of its branches are shipped and live**:

| | gate enabled (today) | gate disabled (today) |
|---|---|---|
| Config Observer | `enforce: restricted` | `enforce: privileged` |
| PSA label syncer | enforcing | advising |
| `PodSecurityReadinessController` | running | running |

It is in `inDefault()`, `inOKD()`, `inTechPreviewNoUpgrade()` and `inDevPreviewNoUpgrade()` ([`features.go#L108`](https://github.com/openshift/api/blob/master/features/features.go)), so every cluster is on the enabled side. The observer's two branches are [`podsecurityadmission.go#L105-L113`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/auth/podsecurityadmission.go); the syncer's are [`psalabelsyncer.go`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go).

After this enhancement the gate means *the new PSA configuration generation is active*:

| | gate enabled (after) | gate disabled (after) |
|---|---|---|
| Config Observer | `enforce` from `spec.podSecurityAdmission`, unset → `privileged` | `enforce: restricted` |
| PSA label syncer | **not running** | enforcing |
| `PodSecurityReadinessController` | **not running** | running |
| `spec.podSecurityAdmission` in schema | present | absent |

**The disabled column is today's enabled column.** This is an inversion, not an extension, and every consequence below follows from that one fact.

##### What has to change

- **Both observer branches flip.** `!Enabled` must now produce `restricted` and the enabled branch must read `spec`. Neither branch is left alone.
- **The syncer's two *explicit* branches flip, and its third does not.** The switch keys on string presence, not on a boolean: `OpenShiftPodSecurityAdmission=false` currently constructs the *advising* controller, `=true` falls through to *enforcing*, and **absence also falls through to enforcing**. The absent case is how MicroShift gets an enforcing syncer with no feature-gate machinery at all, so **it must keep meaning enforcing**. The natural refactor — collapsing `=false` and absent into one "not enabled" branch — silently retires the syncer on MicroShift. This is written down because it is a one-line mistake with a product-wide blast radius.
- **The advising constructor loses its only caller.** `NewAdvisingPodSecurityAdmissionLabelSynchronizationController` is reachable today only from the `=false` branch. With that branch inverted nothing constructs it, and it can be deleted along with the non-deterministic server-side-apply decay described in [Why it is retired outright](#why-it-is-retired-outright).
- **HyperShift's copy flips too**, and is an exact string match rather than a gate lookup — `slices.Contains(p.FeatureGates, "OpenShiftPodSecurityAdmission=true")` ([`kas/config.go#L159`](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go)) — so anything that is not that literal string, including a hosted cluster that simply never emitted it, lands in the else branch. It is also the one reader that sees gate lists from other releases, which is why it alone needs a version gate on top of the inversion.

##### Why one gate rather than two

Reusing the gate is safe because **no supported cluster ever observes the redefinition**: feature gate membership is compiled into the payload rather than chosen per cluster, and the gate leaves `Default` in the same release its meaning changes, so a `Default` cluster sees the old behaviour on both sides of that boundary. The new meaning exists only in `TechPreviewNoUpgrade`, and those clusters cannot upgrade out of it. A second gate would have let the posture change ship a release earlier, at the cost of a release in which enforcement is off with no supported way to turn it back on; that trade is recorded in [Alternatives](#ship-the-posture-change-a-release-ahead-of-the-api), along with the one residual cost of this choice, a bounded version gate in HyperShift.

**`inOKD()` is assumed to move with `inDefault()`**, so that OKD tracks OCP rather than receiving the new behaviour a release early. An OKD reviewer should confirm.

The two levers are distinct:

| Lever | Controlled by | Audience | Effect |
|---|---|---|---|
| `OpenShiftPodSecurityAdmission` in `Default` | Red Hat, at build time | fleet-wide, per release | whether the cluster is in the old world or the new one — posture default, both retirements, and whether the API field exists |
| `spec.podSecurityAdmission.enforceLevel` | cluster administrator | one cluster | the enforcement level that cluster actually runs, once it is in the new world |

##### Two syncers, only one retired

`cluster-policy-controller` runs **two** PSA label controllers and only the first is in scope:

- `runPodSecurityAdmissionLabelSynchronizationController`, the SCC-mapping syncer, gated and retired here.
- `runPrivilegedNamespacesPSALabelSyncer`, which constructs `NewPrivilegedNamespacesPSALabelSyncer` ([`privileged_namespaces_controller.go`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/privileged_namespaces_controller.go)) and pins `default`, `kube-system` and `kube-public` to privileged. It is **ungated and unconditional**, it does not map SCCs to standards, and it **keeps running unchanged**.

Every statement in this document about "the syncer" means the first. Retiring the second would leave three core Namespaces governed by the global default, which on an opted-in cluster would enforce `restricted` on `kube-system` — an obvious way to break a cluster, and an easy mistake to make while deleting code that lives in the same package.

#### Retiring the PodSecurityReadinessController

The [`PodSecurityReadinessController`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go) stops running when the gate is on. It is not reduced in scope and not restarted when an administrator opts in; its code stays in the tree because the gate-off path still runs it.

Repurposing it rather than retiring it was rejected on three properties that were defensible as migration instrumentation but not as a standing feature: [`nonEnforcingSelector`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/podsecurityreadinesscontroller.go#L111-L119) confines the sweep to Namespaces carrying no `enforce` label, which after retirement is a permanent and, on upgraded clusters, overwhelming exclusion; its answer is pinned at `restricted` for fleet comparability, giving a `Baseline` administrator a permanent false positive; and it made a pure-intent config resource partly operator-owned, writing status on a four-hourly cycle. The case for keeping it anyway is argued in [Keep the readiness controller](#keep-the-readiness-controller).

##### What stops being reported

**Six ClusterOperator conditions.** The `PodSecurity*EvaluationConditionsDetected` family on the `kube-apiserver` ClusterOperator is set by this controller and by nothing else. All six stop being written, and existing ones must be **actively removed on upgrade** — a condition whose writer no longer exists is cleared by nothing, and these are unioned into the ClusterOperator. This is a public status surface being withdrawn, so **whoever owns the ClusterFleetEvaluation dashboards and any Insights rule keyed on these conditions has to be told before release `n+1`**. It is the single most likely thing to break outside the cluster.

**The fleet view itself.** Red Hat stops receiving any signal about PSA readiness, at the moment that question becomes an adoption question rather than a migration one — so the rollout of this feature cannot be measured against it. Accepted deliberately, recorded as a [drawback](#drawbacks); a telemetry-only gauge was considered and rejected as [an alternative](#keep-a-minimal-enforcement-mode-metric).

##### What opting in still risks, with nothing evaluating

PSA is a *validating* admission plugin: it acts only on Pod creation, never re-evaluates a running Pod, and raising the level never evicts anything. Two consequences outlive the sweep and are properties of the feature rather than limits of a controller.

- **A workload admitted today can be rejected weeks later**, when a drain, reboot, upgrade or crash-loop recreates it — a routine MachineConfig rollout is enough. The dry-run loop catches the Deployment-scaled-to-zero and CronJob-not-yet-fired cases, but only when run.
- **A Namespace created after the opt-in is enforced immediately**, with nothing labelling it and nothing reporting it. This is upstream Kubernetes' behaviour, but it is not what OpenShift administrators have experienced, and it is the sharpest residual risk of opting in.

Payload Namespaces are exempt from the second — they carry labels from their own CVO manifests, verified in CI — with OLM the known exception, handled under [OLM Namespaces carry no label](#olm-namespaces-carry-no-label). `warn` and `audit` pinned at `restricted` mean every one of these cases is *observable* as it happens, which is the whole of the mitigation.

##### Alerts and metrics

**No metric and no alert ship with this enhancement**, and the four metrics plus the `PodSecurityReadinessEvaluationStale` alert that were derived from the sweep go with it.

The existing [`PodSecurityViolation`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/alerts/podsecurity-violations.yaml) alert is **retained unchanged** and becomes the only thing on the cluster that proactively reports a PSA problem. It fires on `pod_security_evaluations_total{decision="deny",mode="audit"}`, which is unaffected by anything here because `audit` stays pinned to `restricted` in every mode. **Its thresholds and routing should be reviewed** against that heavier role, since it was tuned as a secondary signal alongside the conditions. Nothing alerts on a retained label that has become stricter than its Namespace needs; see [Nothing removes them, and nothing reports them](#nothing-removes-them-and-nothing-reports-them).

#### PodSecurity Configuration

A Config Observer in `cluster-kube-apiserver-operator` manages the kube-apiserver's global PodSecurity configuration. **There is no "unset" option.** [`defaultconfig.yaml#L12-L22`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/config/defaultconfig.yaml#L12-L22) hardcodes all six keys as `invalid-to-force-substitution`, so a kube-apiserver whose PodSecurity block was never substituted fails to start. [`observePodSecurityAdmissionEnforcement`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/auth/podsecurityadmission.go) accordingly has two branches today and never returns a state in which the path is absent. "Disabling PSA" means substituting `privileged`, not omitting the configuration.

Its inputs are exactly three: `spec.podSecurityAdmission.enforceLevel`, read directly; `spec.podSecurityAdmission.enforceVersion`, clamped; and the `OpenShiftPodSecurityAdmission` feature gate, used only to pick the level substituted when `enforceLevel` is `Privileged`, empty or unavailable — `restricted` while the gate is in `Default`, `privileged` once it leaves in release `n`, which exists only to preserve today's behaviour across the transition.

**The observer is now the whole of the implementation on the enforcement path**, and that is the main structural benefit of retiring the readiness controller. It reads two administrator-written fields and a compiled-in gate, and depends on no other controller having run. There is no producer whose failure could hold up a level change, nothing to debounce because no input varies with cluster activity, and no ordering to get wrong. An earlier draft had it consuming two conditions written by the readiness controller, which made the break-glass path depend on the health of the component most likely to be broken when it was needed.

This enhancement changes the `enforce` key and the three `*-version` keys. Both existing branches pin `audit` and `warn` to `restricted` regardless of enforcement level ([`#L32-L39`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L32-L39)), retained deliberately because it is what keeps violations observable while `enforce` is `privileged`. The three `*-version` keys move together from `enforceVersion`. `Baseline` has no implementation today, so a third branch must be added.

**The clamp is the observer's responsibility and must never be skipped.** Because `defaultconfig.yaml` makes the block fatal, the observer is the last component that can stop a user-supplied version reaching a kube-apiserver that will not start with it — and the recovery for a bad enforcement setting is a write to the API server that just failed to start, which on a single-node cluster is unrecoverable without host access. The operator must therefore never substitute a version it has not bounded, regardless of what upstream's plugin does with an unknown one. *Whether upstream's loader in fact rejects or silently clamps such a value should be confirmed during implementation; the design does not depend on the answer.*

#### Retiring the PSA label syncer

The [PSA label syncer](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) does not run when the gate is on, in any enforcement mode, and no `enforceLevel` value brings it back. Nothing is written, ever; labels already present are left exactly as they are, because a controller that never applies never prunes. The set it acted on — and therefore the set carrying its labels on any cluster upgrading into release `n+1` — is defined by `isNSControlled` ([`#L475-L522`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L475-L522)): user Namespaces that have not opted out via `security.openshift.io/scc.podSecurityLabelSync=false`. **OpenShift's own Namespaces are unaffected**, excluded both by the hardcoded `nsexemptions` payload list and by the `openshift-` prefix; they carry PSA labels from their own manifests, which the syncer never owned.

What this costs: no Namespace gets a computed `enforce` label again, so labelling becomes the Namespace owner's responsibility — component teams in CVO manifests, enforced by CI, and the administrator for user Namespaces with no equivalent check. Per-Namespace `warn` and `audit` labels stop being written too, but nothing depends on them, since the global configuration pins both to `restricted` regardless.

`security.openshift.io/MinimallySufficientPodSecurityStandard` goes with it. It cached the syncer's working answer — the weakest PSS level admitting the SCCs reachable by a Namespace's ServiceAccounts — and its only non-test consumer product-wide was the readiness controller's choice of dry-run level ([`violation.go#L55`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/violation.go#L55)), which is retired too. Relocating the computation has no host left to relocate into, so "what standard does this Namespace need?" becomes `oc label --dry-run=server`, asked by a person. Existing annotations are **not** stripped, for the same reason the labels are not: they record what the cluster computed beforehand, and once nothing reads them they can mislead only a human, who has `managedFields` and the release note to date them.

##### Why it is retired outright

The obvious alternative — tie the syncer to the enforcement mode, off at `Privileged` and on at `Baseline` or `Restricted` — is rejected on four grounds.

- **At `Privileged` it has nothing to do.** It exists to make a `restricted` default survivable by computing a *less* restrictive label for Namespaces that cannot meet it. Once the default is `privileged` an unlabelled Namespace is admitted regardless, and the computed label is metadata no admission decision consumes.
- **At `Baseline` or `Restricted` it contradicts the request.** An administrator setting `Restricted` is asking for unlabelled Namespaces to be held to `restricted`; the syncer would immediately grant many of them a *lower* standard computed from their SCCs, so the cluster would report `enforceLevel: Restricted` while running a patchwork neither the administrator chose nor the API describes.
- **Its inference is the known-unreliable part of the system.** The [Motivation](#motivation) exists because deriving a standard from the SCCs reachable by a Namespace's ServiceAccounts is wrong often enough to matter, and keeping the syncer keeps that inference on the enforcement path.
- **A controller that starts and stops is harder to reason about than one that does not exist.** A mode-switched syncer must handle being started on a cluster it has never seen, catching up across every Namespace before the level can safely be raised, and being stopped mid-apply — cross-operator sequencing on independent timing.

**Advising mode is neither freezing nor removing**, though it looks like the safe middle ground: `syncedLabels` there holds `warn` and `audit` only and SSA prunes fields a manager no longer sends, so the `enforce` label *is* dropped — but only on the next apply, and `shouldUpdate` skips the apply unless a tracked value has drifted. A Namespace already correct keeps its label indefinitely; one that sees an unrelated SCC change six months later loses it at that moment. Identically configured clusters diverge on unrelated churn.

#### Retained labels on upgraded clusters

Every cluster running today has `OpenShiftPodSecurityAdmission` in its `Default` feature set and runs the syncer's **enforcing** constructor, so `pod-security.kubernetes.io/enforce` labels are already present across the fleet, applied via server-side apply under the field manager `pod-security-admission-label-synchronization-controller` ([`#L386`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L386)). Retiring the syncer stops it writing new labels; nothing removes existing ones and nothing ever reconciles them again. **They are frozen at whatever value the last pre-upgrade sync left.** Release `n` does not introduce this label — it stops writing one that is written today.

Retention is deliberate: optional is not off, and stripping every syncer-written label on upgrade would replace "every cluster must enforce" with "every cluster must stop enforcing"; retention can be undone by hand while stripping cannot, since SSA archives nothing and returning to `Restricted` would recompute from current state rather than restore what was there; and the exposure is compliance rather than availability — relaxing enforcement breaks nothing, but a security control disappearing without announcement from clusters attesting under FedRAMP, PCI or DISA STIG is a real risk, and the labels plus their `managedFields` entries are that attestation's evidence.

Two consequences follow, and both cut against the feature's purpose.

**The API is inert on upgraded clusters.** Per-Namespace labels take precedence over the global default, and the config observer only sets the global default. On a cluster where essentially every managed Namespace carries a syncer-written label, changing `enforceLevel` changes the effective level of *nothing that already has one*. An upgraded cluster is opt-in for *new* Namespaces and unchanged for existing ones until somebody removes those labels by hand.

**A frozen label can become a fossil.** Take a Namespace labelled `enforce: restricted` before the upgrade, whose administrator then grants its ServiceAccount the `anyuid` SCC to run a workload needing a fixed UID. Before release `n` the syncer would have relaxed the label to `baseline`; now it stays pinned at `restricted` and the workload is rejected — on a cluster whose administrator never opted into this feature and has no reason to associate the failure with an upgrade. No configuration resolves this: the fixes are to edit the label by hand or delete it and let the Namespace fall to the global default, both covered in [Removing retained `enforce` labels](#removing-retained-enforce-labels).

##### Nothing removes them, and nothing reports them

No cleanup ships, and that is a decision rather than a deferral: nothing in release `n+1` or later deletes a retained `enforce` label, on any trigger. A one-shot cleanup would be a fleet-wide silent reduction in enforcement posture — the exact compliance event the retention argument exists to avoid — so removal is an administrator act, performed per Namespace by someone who has decided they want it. Nothing detects the problem either, because deciding that a retained label is now stricter than its Namespace needs requires the SCC-to-standard computation that was just retired. What an administrator loses is not the failure itself but the explanation for it: the workload is rejected with a PSA message naming the control it violated, surfacing on the ReplicaSet or Job, but nothing connects that message to an upgrade weeks earlier. The [release note](#release-note) is the whole of the mitigation. Two things narrow the exposure — the trigger is an SCC or RBAC grant made *after* the upgrade, so whoever hits the rejection is usually whoever just changed something, and the remedy is immediate once identified.

##### OLM Namespaces carry no label

Retaining labels protects only Namespaces that have one, and OLM's do not. The kube-apiserver's PodSecurity configuration carries no Namespace exemptions at all — [`defaultconfig.yaml`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/bindata/assets/config/defaultconfig.yaml) exempts exactly one username — so `openshift-operators`, where OLM installs operator bundles by default, is governed entirely by the global default. It is deliberately kept out of `nsexemptions`, which carries an `IMPORTANT:` comment saying it must not be exempted, but it is then caught by the `openshift-` prefix skip, so the syncer never labelled it even when it ran.

Both directions bite. On upgrade its effective level silently drops from `restricted` to `privileged`; on a cluster opting in to `Restricted`, every operator bundle needing more than restricted fails admission, so the happy path of this feature is a mass-breakage path for OLM users. **This is out of scope**: labelling that Namespace is an OLM-side change, retiring the syncer neither causes nor worsens the gap, and exempting the Namespace contradicts the comment. What this enhancement owes is disclosure, in the release note and in [Monitor tests for managed Namespaces](#monitor-tests-for-managed-namespaces). **OLM and layered-product reviewers are required to accept the hand-off**, not to resolve it here.

##### Release note

Because no alert and no cleanup ship, the release note is the primary mitigation rather than a summary, and the only place an administrator is told any of this before meeting it in production. At minimum: **the default posture changes** — the cluster-wide default becomes `privileged`, so clusters that take no action are less restrictive than before; **existing `enforce` labels are kept and are now frozen**, so on an upgraded cluster the new API governs new and unlabelled Namespaces only; **the symptom, named as a symptom** — a workload that runs today can be rejected after its ServiceAccount is granted a broader SCC, described as what they will see rather than as a mechanism, linking to [Removing retained `enforce` labels](#removing-retained-enforce-labels); **`openshift-operators` changes effective level**, from `restricted` to `privileged`; **downgrade re-enables enforcement**, per [On Downgrade](#on-downgrade); and **the six `PodSecurity*EvaluationConditionsDetected` conditions are withdrawn**, naming them individually, since they are public surface that anything may be reading and their disappearance is otherwise indistinguishable from a broken operator. Both retirements are removals with no supported way back, and must be said plainly rather than framed only as "enforcement is now optional".

#### New Installation

Fresh installs do not enforce PSA by default: with `enforceLevel` unset the cluster resolves to `Privileged`, and nothing else about PSA is switched off. From there the administrator raises the level they are comfortable with, and returns it to `Privileged` to go back.

A fresh install is the only case where retiring the syncer costs nothing: no Namespace carries a syncer-written label, so there are no fossils, the API is not inert, and `Privileged` really is the cluster's effective posture rather than only its default. The trade appears later — once the administrator opts in to `Restricted`, every Namespace their workloads create from then on is `restricted` unless they label it, and nothing is computing labels on their behalf.

### Risks and Mitigations

- **The out-of-the-box security posture is reduced.** Clusters that take no action end up less restrictive than today. *Mitigation:* per-Namespace labels still take precedence, SCCs are unchanged, existing enforce labels are retained, and the release note calls it out. Not fully mitigated by design — it is the trade this enhancement makes, and it **requires explicit security sign-off**.
- **Nothing on the cluster tells an administrator whether opting in is safe, and nothing warns them when they do it.** The requested level is applied on a cluster the platform has formed no opinion about. *Mitigation:* the `audit` signal and the `PodSecurityViolation` alert make violations visible before and after the act, and the dry-run loop answers on demand. *Not mitigated:* an administrator who does not run it. This is the risk the retired readiness controller existed to reduce, and it is now the enhancement's weakest point — argued in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).
- **A clean dry-run is not a guarantee.** PSA is validating-admission only, so a workload not running at the time, or recreated later by a drain or upgrade, can fail long after a sweep came back clean — and unlike the previous revision nothing re-runs the check on its own. *Mitigation:* `warn`/`audit` pinned to `restricted` make the eventual failure observable. See [What opting in still risks](#what-opting-in-still-risks-with-nothing-evaluating).
- **A Namespace created after the opt-in is enforced immediately**, with nothing labelling it, nothing evaluating it and nothing reporting it. *Mitigation:* payload Namespaces get labels from CVO manifests, verified by the monitor tests. *Not mitigated* for user Namespaces or OLM's.
- **Six ClusterOperator conditions are withdrawn**, breaking any dashboard or Insights rule keyed on them. *Mitigation:* advance notice to the owners, and removal of stale conditions on upgrade rather than leaving them standing. See [What stops being reported](#what-stops-being-reported).
- **Red Hat loses its fleet view of PSA readiness** at the point it would be used to measure this feature's own adoption. *Unmitigated*, by [Non-Goal](#non-goals).
- **`openshift-operators` has no label from any source** and follows the global default in both directions. **Unmitigated and out of scope**; see [OLM Namespaces carry no label](#olm-namespaces-carry-no-label).
- **A retained `enforce` label can reject a workload long after the upgrade, with nothing on the cluster reporting it.** *Mitigation:* the release note and documentation, which are the whole of it since no alert and no cleanup ship. See [Nothing removes them, and nothing reports them](#nothing-removes-them-and-nothing-reports-them).
- **A clamped `enforceVersion` is silently applied**, visible only in the rendered admission configuration. See [the invisible clamp](#enforceversion-and-why-the-default-moves).
- **An unset `enforceVersion` tightens the standard on every upgrade**, so a cluster can acquire a violation from an upgrade alone. *Mitigation:* pinning holds it still, and the behaviour matches today's. *Not mitigated for clusters that do not pin*, which will be most of them.
- **Every enforcement change rolls the kube-apiserver**, which on SNO is an API outage, including on the disable path. *Mitigation:* the observer reads `spec` directly and depends on no other controller, so the change cannot be held up by a failing component; the cost is documented in the break-glass procedure.
- **The feature gate is redefined rather than added, so the same gate name means different things in different releases.** Anything reading it across a version boundary can act on the wrong meaning. *Mitigation:* removing the gate from `Default` in the same release its meaning changes makes the two readings agree on every standalone cluster, and the one place the boundary is crossable — HyperShift's `control-plane-operator` — gets a bounded version gate that sunsets. *Not mitigated* for an out-of-payload consumer nobody knows about that string-matches the gate list; the enum-preservation rule does not help here, because the gate is not the enum. See [Gate redefinition skew](#gate-redefinition-skew).
- **Four branch inversions have to be got right simultaneously**, in `cluster-kube-apiserver-operator` twice, `cluster-policy-controller` and `hypershift`, and an inverted branch is a silent posture change rather than a failure. The `cluster-policy-controller` switch is the sharpest: it has three arms and only two of them invert, because the absent arm is MicroShift's. *Mitigation:* the gate-off path is asserted to be byte-for-byte today's behaviour, which is the single most important test in this enhancement, and MicroShift's path is tested as a case in its own right rather than inferred from the default.
- **A downgrade from `n+1` restarts the syncer as well as re-enabling enforcement**, because one gate carries both. The syncer resumes writing `enforce` labels computed against SCC state that may have moved on considerably since it last ran. *Partly mitigated:* the result is a state the product shipped for years rather than an untested hybrid, and `unsupportedConfigOverrides` can hold enforcement off, though it cannot stop the syncer. See [On Downgrade](#on-downgrade).

Security review is required from the OpenShift security architecture group, covering the default-posture change and the withdrawal of the readiness signal together rather than the API. UX review is required from the console and docs teams for the `oc` workflow, which is now the entire user interface to this feature.

### Drawbacks

- **It ships a less secure default than the product has today**, with no mechanism preserving the current default for clusters that take no action. For a product positioned as secure by default, that is a real cost and not only a documentation problem.
- **The platform stops being able to say anything about PSA.** Before this enhancement OpenShift maintained a per-Namespace answer to "what standard can this Namespace meet?" and a per-cluster answer to "could this cluster enforce `restricted`?". After it, neither exists, in the API or in telemetry, and both become things a person asks one cluster at a time with a shell loop. That is a large reduction in what the product knows about itself, and it is the change most likely to be objected to in review.
- **Opting in is a weaker guarantee than the enforcement it replaces.** Today's syncer puts each Namespace at the strictest standard it can actually meet; `Restricted` means `restricted` everywhere unlabelled — stricter in principle, but only survivable if the cluster is already clean, and offering nothing to one that is mostly clean. There is no per-Namespace middle ground, so administrators with heterogeneous clusters must label Namespaces themselves or stay at `Privileged`.
- **The feature does nothing on an upgraded cluster, and no future release changes that.** Retained labels outrank the global default and no cleanup ships, so the clusters most in need of relief get a manual procedure rather than an API — and those labels become a new class of permanently unmaintained cluster state that support must reason about for as long as those clusters live.
- **The API cannot be observed, only written.** There is no way to ask the cluster what it is enforcing without reading an operator-owned ConfigMap, which makes the clamp invisible and makes "did my change take effect?" a support question rather than an administrator one.
- **A downgrade can silently erase the administrator's recorded intent.** This is the price of a gated field over a dedicated CRD, and re-upgrading does not restore it. See [Schema pruning erases the configuration](#schema-pruning-erases-the-configuration).
- **It bundles three separable changes** — retiring the syncer, retiring the readiness controller, and the new API. The first two land on every cluster whatever `enforceLevel` says, and between them they remove more code and more public surface than the API adds, so a reviewer who evaluates only the API has not evaluated the change.
- **Applying a configuration change costs a control-plane rollout**, which on SNO is an outage, in both directions.

## Alternatives (Not Implemented)

The alternatives below have been identified but not yet evaluated to a conclusion. **This section must be completed before the EP moves from `provisional` to `implementable`.**

### Keep the readiness controller

**The most likely objection to this enhancement, and the one to argue out first.** The administrator's question — *can my cluster survive `restricted`?* — did not disappear when the migration did; only Red Hat's question did. A controller already exists that answers it on a schedule, without being asked, and replacing it with a documented shell loop is a worse answer to a question that opting in deliberately makes *more* important.

It is rejected on three grounds, in descending weight. Its population is wrong and cannot be cheaply fixed, since `nonEnforcingSelector` excludes every Namespace carrying an `enforce` label — after retirement a permanent and usually overwhelming majority — so the standing answer would be most confident about the clusters it understood least. Its answer is pinned at `restricted` for fleet comparability, which gives a `Baseline` administrator a permanent false positive. And it is the entire operational cost of the feature: the four-hourly sweep, the status subtree, the `apiservers/status` RBAC, four metrics, an alert and a runbook, all serving advice nothing acts on. A narrower variant — evaluate at the *configured* level over *all* Namespaces and write only a count — answers the first two objections and should be costed before this is settled, but it is a substantially new controller rather than a retained one.

### Keep a minimal enforcement-mode metric

Export `pod_security_enforcement_mode` from the config observer and ship nothing else: a single gauge, derived from a field read with no sweep, letting Red Hat measure adoption and letting support answer the question without a live session. Rejected to keep the retirement clean — a feature that ships one metric still owns a telemetry surface, a dashboard and the question of what else belongs beside it, and the value is to Red Hat rather than to the administrator. **This is a cheap decision to reverse**, recorded so that reversing it is deliberate rather than a rediscovery.

### Shape of the feature gating

Three arrangements were considered; all reach the same end state and differ in how many releases it takes and what has to be version-gated. The chosen one is argued in [Why one gate rather than two](#why-one-gate-rather-than-two).

#### Ship the posture change a release ahead of the API

Two gates, two releases: release `n` simply turns `OpenShiftPodSecurityAdmission` off with its existing meaning, and a new gate carries the field and its observer, graduating at `n+1`. Nothing is redefined, so [Gate redefinition skew](#gate-redefinition-skew) does not exist and HyperShift needs no version gate, and the posture change and the API can be reviewed and reverted independently. It is rejected for the state in between: for the whole of release `n` a `Default` cluster has enforcement off and **no supported way to turn it back on**, because the field is not yet in the schema and `unsupportedConfigOverrides` blocks upgrades. The team's judgement, recorded because it is the pivotal one, is that a one-release gap in opt-in capability is not acceptable even though it is temporary.

#### Add a new gate and leave the existing one alone

One gate and one release, but a new name: `OpenShiftPodSecurityAdmission` stays in `Default`, becomes inert, and a new gate carries all three changes. This avoids both the two-release gap and the redefinition. It is rejected for what it leaves behind — a gate still in `Default`, read by nothing, and named after the feature it no longer controls — and because every component branching on the old gate must branch on the new one instead, so the saving is in the skew story rather than in the diff. It remains the cheapest fallback if the HyperShift version gate proves unacceptable to its owners.

### Shape of the API

- **A dedicated cluster-scoped `PSAEnforcementConfig` CRD.** The earlier shape of this proposal: it survives a downgrade intact rather than being pruned, recorded as a [Drawback](#drawbacks) of what was chosen. Three things moved the API onto `APIServer`. It is already the cluster-wide API server configuration surface — `enforce` is a kube-apiserver admission plugin setting, and the resource already carrying `audit`, `encryption` and `tlsSecurityProfile` is where a reader looks for one more; a separate CRD would have been a *fourth* place holding PSA state. It is already consumed by the config observers this enhancement extends, and installer-rendered, so there is no "singleton absent" case outside MicroShift. And HyperShift's plumbing already exists: `HostedCluster.spec.configuration.apiServer` is a `*configv1.APIServerSpec` ([`hostedcluster_types.go`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)), which with a spec-only API is now the *whole* of what HyperShift needs. The second argument against a CRD — that it would have become partly operator-owned — has lapsed, since nothing writes status anywhere now.
- **Keep a minimal `status` carrying only the applied level and version.** The narrowest way to preserve something after the readiness controller's removal, written by a small status controller on configuration change rather than on a timer. Rejected because `status.enforceLevel` would be a pure echo of `spec` carrying no information, leaving the clamped `enforceVersion` as the only real content — which does not justify making `cluster-kube-apiserver-operator` a writer of a `config.openshift.io` resource, taking the `apiservers/status` RBAC, setting the first-writer precedent on `APIServerStatus`, and adding two rows to the downgrade matrix. The cost of that choice is [the invisible clamp](#enforceversion-and-why-the-default-moves), and a status subtree can be added back compatibly if it proves necessary.
- **Spec on `APIServer`, status on `operator.openshift.io/v1 KubeAPIServer`.** Moot now that there is no status, and recorded because it was seriously considered: rejected because splitting intent and in-force state across two resources breaks the property the design leans on, and because conditions there are unioned into the `kube-apiserver` ClusterOperator.
- **Document `unsupportedConfigOverrides` and ship nothing.** Zero API surface, and it is the recovery path release `n` relies on already. Rejected in principle because an unsupported override is not an acceptable long-term answer for a supported posture decision, but the comparison should be written out.

### Shape of the enforcement decision

**Gate the raise on a clean evaluation.** Earlier revisions applied `Baseline` or `Restricted` only once the evaluation reported no violations, with an `acknowledgeKnownViolations` escape hatch keyed to a hash of the violation set, and an `EnforcementBlocked` condition and alert when the raise was withheld. **This is the alternative with the strongest safety story and the one most likely to be re-proposed in review.** It is rejected because it protected the wrong thing — it gated the global default, which governs only Namespaces with no `enforce` label — because a point-in-time sweep with known blind spots becomes a liability the moment a false positive stops an administrator configuring their own cluster, because it had no expression in HyperShift, where `ClusterConfiguration` carries specs only, and because it was most of the API's complexity. It is now additionally foreclosed: with the readiness controller retired there is no evaluation to gate on, so re-proposing it means re-proposing [Keep the readiness controller](#keep-the-readiness-controller) first.

**Report the result in `status`**, either as a `violatingNamespaces` list naming each Namespace and the standard it fails, or through the existing `PodSecurity*EvaluationConditionsDetected` ClusterOperator conditions. The list is what an administrator actually wants, but it has to be bounded, and a truncated list is indistinguishable from a short one on exactly the large clusters where the names matter most; the conditions are not co-located with the `spec` field they describe, and are themselves being [withdrawn](#what-stops-being-reported). Both are foreclosed with the evaluation that fed them.

## Open Questions

Retiring the readiness controller closes four questions the previous revision carried, and they are listed rather than deleted so that reviewers can see they were answered by removal rather than overlooked: whether a write to `spec` should trigger an immediate re-evaluation; whether a cluster passing through release `n` would ever complete an initial evaluation; how to bound the sweep's cost and pagination on large clusters and SNO; and how to read an absent `validated-scc-subject-type` annotation on workloads predating it. None has a subject any more.

### Is a shell loop an adequate diagnostic

The dry-run in [Finding the violating Namespaces](#finding-the-violating-namespaces) is now the only way to ask whether a cluster can meet a standard, and it is `O(Namespaces)` serial server round-trips run by hand against a cluster that may already be unhealthy. It scales worst exactly where the answer matters most. The options are to ship it as a documented procedure and accept that; to add `oc adm` tooling that parallelises and bounds it; or to ship it only as a must-gather collector and have administrators read the bundle. **A decision is needed before GA**, because it determines whether the documentation can honestly recommend opting in on a large cluster.

### Removing the withdrawn ClusterOperator conditions

Six `PodSecurity*EvaluationConditionsDetected` conditions stop being written, and stale ones must be removed rather than left standing, since nothing garbage-collects a condition whose writer is gone and the status controller unions them into the ClusterOperator. What is unspecified is *what* does the removing, given that the natural candidate is the controller being retired. The likely answer is a one-shot removal on operator start in release `n`, which has to be written, has to be idempotent, and has to survive the controller's code being deleted. Who is told, and when, is a separate obligation recorded in [What stops being reported](#what-stops-being-reported).

### How CI lanes get a `restricted` cluster

The expected PSS for OpenShift's own components stays `Restricted`, and monitor and periodic tests depend on that. The syncer was never what provided it — `isNSControlled` skips every `openshift-` prefixed Namespace. What is undecided is how lanes get a `restricted` cluster now: opting in sets the global default but does not bring the syncer with it, so any test Namespace the lanes create themselves (`e2e-test-*` and similar, which were syncer-managed) inherits `restricted` with nothing computing a gentler label. Whether that breaks existing e2e suites, and whether the fix is per-test labelling or a broader exemption, is the input the [Test Plan](#test-plan) is missing.

### Day 0 configuration

The Summary and [New Installation](#new-installation) both state enforcement can be requested at install time, but no install-config field, manifest name or bootstrap rendering path is specified, and no installer reviewer is assigned. The bootstrap kube-apiserver must render the same `enforce`, `warn` and `audit` keys as the post-bootstrap observer, or the first control-plane rollout silently changes the cluster's posture out from under an administrator who asked for it at install time. The hazard the previous revision noted here — a day-1 manifest setting a level before anything had evaluated the cluster — no longer distinguishes day 0 from day 2, since nothing evaluates at either.

## Test Plan

### The gate-off path is today's behaviour

**This is the most important test in the enhancement and the one to write first.** Every other assertion here describes new behaviour, which is visible when it is wrong; this one describes *unchanged* behaviour, which is not. The gate is off in `Default` for the whole of release `n` and remains off on every downgrade target afterwards, so the gate-off branch is the path real clusters are actually on, and a mistake in the inversion lands there silently.

With `OpenShiftPodSecurityAdmission` off, assert against the state the product ships today rather than against a description of it:

- the rendered `admission.pluginConfig.PodSecurity.configuration.defaults` block is **byte-for-byte identical** to the one release `n-1` renders, all six keys, compared against a golden file generated from `n-1` rather than hand-written;
- the PSA label syncer is running and in **enforcing** mode, asserted by observing it write a label on a Namespace whose ServiceAccounts' SCCs change, not by inspecting construction;
- the `PodSecurityReadinessController` is running and the six `PodSecurity*EvaluationConditionsDetected` conditions are present on the `kube-apiserver` ClusterOperator;
- `podSecurityAdmission` is **absent from the `Default` CRD schema**, and a write setting it is pruned rather than rejected.

Separately, and not covered by any of the above because it does not read the gate at all: **the absent-gate arm of `cluster-policy-controller`'s switch still yields the enforcing syncer.** This is MicroShift's path and nothing else's, so it has to be a unit test over the switch with the feature gate accessor reporting the gate as neither set nor unset — the condition `cluster-policy-controller` runs under when it is vendored without a `FeatureGate` CR. Asserting it through the gate-off case instead would pass while the behaviour was broken.

### Switching between PSA modes

With the gate on, for each transition between `Restricted`, `Baseline` and `Privileged`:

- the kube-apiserver's effective `enforce` level, read from the revisioned `config-<revision>` ConfigMap — which is now the *only* place it can be read, so this assertion carries the weight the previous revision spread across `status` as well;
- **that neither the PSA label syncer nor the `PodSecurityReadinessController` runs**, in any mode. "Not running" is what distinguishes this design from a reduced or advising mode, so both need a direct test rather than being inferred from nothing changing — and, since the same binaries do run them when the gate is off, from nothing being constructed either;
- that a Namespace's existing `enforce` label is **byte-for-byte unchanged** across every transition, including after an unrelated SCC or RBAC change and including on the transition back up to `Restricted`;
- that a Namespace with **no** `enforce` label is held to the global level as soon as the cluster opts in, with nothing computing a gentler label for it;
- that `warn` and `audit` stay pinned to `restricted` in every mode, including `Privileged` — this is what keeps the `audit` signal alive, and with the readiness controller gone it is load-bearing rather than a nicety;
- that `apiserver/cluster` has **no `status.podSecurityAdmission`** in any mode and that nothing writes `apiservers/status`. A test that fails if a writer appears is worth more than one checking values, because the hazard is a second writer being added later by someone who assumes a status exists.

### Monitor tests for managed Namespaces

Retiring the syncer moves the assurance for OpenShift's own Namespaces from a runtime controller to CI: after release `n` nothing on a running cluster would notice a payload Namespace shipping without a label. The existing SCC labelling monitor test must not regress once the default posture is `privileged`, and a **new, required** monitor test must assert that every payload Namespace carries a `pod-security.kubernetes.io/enforce` label from its own manifest. Payload labels are static and ship in manifests, so a pre-merge check on the payload is the right place for it, and **it must be in place before the default changes in release `n`, not at graduation.** The test cannot cover OLM, which expresses its requirement through SCC configuration rather than Namespace labels; that exclusion has to be encoded rather than left implicit. Neither test says anything about user-created Namespaces, and nothing in CI can.

### Retiring the readiness controller

The controller's existing unit tests encode behaviour being removed entirely, so they are deleted rather than replaced. What needs testing is the removal itself and its blast radius:

- none of the six `PodSecurity*EvaluationConditionsDetected` conditions is present on the `kube-apiserver` ClusterOperator after the operator has started, **including on a cluster that had them before the upgrade** — the removal path is new code and is the part most likely to be forgotten;
- no metric with the `pod_security_readiness_` prefix is exported;
- the `PodSecurityReadinessEvaluationStale` alerting rule is absent from the operator's rendered assets, and `PodSecurityViolation` is still present and unchanged;
- the operator holds no RBAC permitting a write to `apiservers/status`, asserted against the generated manifests rather than at runtime;
- the ClusterOperator does not go `Degraded` or `Progressing` as a result of the controller's absence, on a cluster where it previously ran.

The expected *fall* in reported violations should be anticipated rather than measured here: the conditions simply stop, so any dashboard reading them goes blank rather than to zero. That distinction has to be in the notice sent to their owners.

### Release transitions

**Upgrade `n-1` → `n`, in an upgrade lane.** The invariant is that **nothing observable changes**, which is an unusual thing to assert in an upgrade lane and is the reason to assert it explicitly: the gate leaves `Default` in the same release its meaning changes, so a `Default` cluster crosses the redefinition without noticing. The effective `enforce` level is `restricted` before and after, the syncer is running before and after, the readiness controller's six conditions are present before and after, and no Namespace label changes value. A lane that passes because it tested nothing looks the same as one that passes because nothing changed, so this is worth writing as positive assertions against the rendered configuration rather than as an absence of alerts.

**Upgrade `n` → `n+1`, in an upgrade lane.** This is the transition that carries the whole change, and the invariant is that it changes the default and nothing else: every Namespace carrying a syncer-written `enforce` label before the upgrade carries **the same value afterwards**, with the `managedFields` entry naming `pod-security-admission-label-synchronization-controller` intact and its `time` unchanged — snapshot before, compare after, because this is the audit record the whole retention argument rests on and a silent SSA prune would destroy it without any other test noticing. No Namespace *gains* a label; the effective level settles at `privileged` with `warn`/`audit` still `restricted`; the syncer is not running afterwards; the six conditions are gone; and the transient in [Operator skew](#operator-skew) is benign in both orderings.

**Downgrade cannot be covered in CI**, because y-stream downgrade is not a supported operation and no lane exists. Rather than leave the hazard untested, test the properties that make it safe, none of which needs a downgrade: that this enhancement writes **no condition anywhere**, asserted directly rather than inferred — on `apiserver/cluster`, on `kubeapiservers.operator.openshift.io` and on the ClusterOperator alike; that the gate-off path is byte-for-byte today's behaviour, per the first section of this plan, which is what makes `n+1` → `n` a return to a tested state rather than a step into an untested one; and, against the generated CRD manifests, that the `Default` manifest for release `n` has no `podSecurityAdmission` under `spec` while the `TechPreviewNoUpgrade` one does, that the `n+1` `Default` manifest does, and that none of them has anything under `status`. The remaining behaviour — the syncer and readiness controller resuming on `n`, and the [pruning](#schema-pruning-erases-the-configuration) of `spec.podSecurityAdmission` on the first write after the downgrade — is a QE procedure against a deliberately downgraded cluster. The pruning case needs a step that writes an unrelated field afterwards, because the erasure does not happen at downgrade time.

### Gate redefinition and HyperShift

The hazard in [Gate redefinition skew](#gate-redefinition-skew) is a management-cluster component applying new semantics to an old gate list, so it cannot be caught by any test that runs inside a single release. Two things need covering, both in `openshift/hypershift`:

- **A unit test over the CPO's PodSecurity rendering, parameterised by hosted release version**, that reproduces the table in that section row by row — a hosted version below `n` with `OpenShiftPodSecurityAdmission=true` in its gate list must render `restricted` and must keep the `cluster-policy-controller` deployment, and the same gate list at `n+1` must render `privileged`. A test that only exercises the current release passes under exactly the bug this guards against.
- **The sunset is asserted, not remembered.** The version gate should carry a test that fails once the minimum supported hosted release reaches `n`, so the dead code announces itself rather than waiting to be noticed.

A hosted-cluster e2e across a real management/hosted version boundary is the honest test and is likely out of reach; the unit tests above are the practical substitute and should say so.

The previous revision owed a further test here, that status written by an earlier payload is not inherited on re-upgrade. A write-only API cannot inherit anything, so the case is gone rather than passing.

## Graduation Criteria

**Graduation is a single event.** One gate carries the API, the posture change and both retirements, so there are no tiers and nothing lands ahead of anything else: `OpenShiftPodSecurityAdmission` returning to `Default` in release `n+1` is simultaneously the moment the field becomes generally available, the moment the global default becomes `privileged`, and the moment the syncer and the readiness controller stop running on a supported cluster. A reviewer looking for a feature gate protecting the two retirements will find this one.

This is a change from earlier revisions, which split the work across two gates and two releases so the posture change could ship first. The consequence is that **everything below is a prerequisite of the same promotion**, including the parts that have nothing to do with the API.

### Prerequisites carried from the posture change

These gate the promotion even though they are not API work, and they are the long poles:

- **All payload workloads SCC-pinned and all payload Namespaces labelled**, demonstrated by the managed-Namespace labelling monitor test rather than by inspection. This is a hard prerequisite because once the default is `privileged` an unlabelled payload Namespace produces no signal on a running cluster, and it is the largest single body of work in this enhancement.
- **The existing SCC labelling monitor test does not regress.**
- **Advance notice to the owners of anything consuming the `PodSecurity*EvaluationConditionsDetected` conditions**, which is a prerequisite rather than a follow-up — see [What stops being reported](#what-stops-being-reported).
- **Resolution of the MicroShift question** in [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift), because MicroShift reaches the syncer through the branch this enhancement must leave alone.

### Promoting the gate

`OpenShiftPodSecurityAdmission` moves from `TechPreviewNoUpgrade` back to `Default` and `OKD`. The field is on a type that is already `v1`, so **there is no version to promote**, no second version served in parallel, and no conversion to write — anyone who adopted it under TechPreview keeps exactly the objects they had. API validation integration tests land alongside the `openshift/api` change.

Promoting a gate that was previously *in* `Default` and was removed is unusual, and reviewers should expect to be asked about it. The answer is that the gate never changed direction from a cluster's point of view: no supported cluster ever observed it disabled, because it left `Default` in the same release its meaning changed and `TechPreviewNoUpgrade` clusters cannot upgrade.

### Dev Preview -> Tech Preview

This feature does not pass through Dev Preview. The gate is redefined and removed from `Default` in release `n`, which leaves the new behaviour reachable only in `TechPreviewNoUpgrade`. Entering Tech Preview requires:

- the `podSecurityAdmission` field merged in `openshift/api` gated on `OpenShiftPodSecurityAdmission`, with API validation integration tests, including that it is absent from the `Default` CRD manifest and present in the `TechPreviewNoUpgrade` one, and that `APIServerStatus` gains nothing;
- the gate removed from `inDefault()` and `inOKD()` in the same change that inverts the branches, so that no payload ever ships the new meaning inside `Default`;
- **the gate-off path asserted to be byte-for-byte today's behaviour** — `restricted`, enforcing syncer, running readiness controller — which is what makes release `n` a no-op for `Default` clusters and is the single most important test in this enhancement;
- the Config Observer selecting level and version from `spec` when the gate is on, with the absent-field, absent-resource and clamped-version paths exercised;
- both retired controllers confirmed not running **when the gate is on**, and the withdrawn ClusterOperator conditions confirmed removed, per [Retiring the readiness controller](#retiring-the-readiness-controller);
- the dry-run sweep documented and collected by must-gather — with no status and no alert, it is the only diagnostic, so it is a Tech Preview prerequisite rather than a GA one;
- tests labelled `[OCPFeatureGate:OpenShiftPodSecurityAdmission]` and `[Jira:"auth"]`, running in both the TechPreviewNoUpgrade and Default Prow variants — noting that in release `n` the Default variant asserts the *old* behaviour, which is the point.

### Tech Preview -> GA

The bar is `dev-guide/feature-zero-to-hero.md`, restated here against this feature so that a reviewer can check it without a second document.

**At least five tests, each individually trackable in Sippy** with an unambiguous success/failure signal. The five carrying the promotion are: the gate-off path is byte-for-byte today's behaviour; each of the three `enforceLevel` values renders the expected admission configuration and every transition between them is reversible with no adverse effect; `enforceVersion` is clamped to the payload's Kubernetes version, including the unset and too-new cases; neither retired controller runs when the gate is on and the six withdrawn ClusterOperator conditions are absent; and the managed-Namespace labelling monitor test from [Prerequisites carried from the posture change](#prerequisites-carried-from-the-posture-change). All are labelled `[OCPFeatureGate:OpenShiftPodSecurityAdmission]` and `[Jira:"auth"]`.

**Frequency, spread and pass rate**, all of which must be in place **no less than 14 days before branch cut** for release `n+1`:

- every test run **at least 7 times per week**;
- every test run **at least 14 times per supported platform**;
- every test passing **at least 95% of the time**;
- every test running in **both the `TechPreviewNoUpgrade` and `Default` Prow job variants** — in release `n` the `Default` variant skips them, which is expected until the gate is promoted.

**All supported platforms**, per the [canonical list](https://github.com/openshift/api/blob/8a46f746f2cf87624651e6e8a85421b49bef3b6e/tools/codegen/cmd/featuregate-test-analyzer.go#L332-L383):

| Provider | Topology | Architecture | Network Stack |
|---|---|---|---|
| AWS | HA | amd64 | default |
| AWS | Single | amd64 | default |
| Azure | HA | amd64 | default |
| GCP | HA | amd64 | default |
| vSphere | HA | amd64 | default |
| Baremetal | HA | amd64 | IPv4 |
| Baremetal | HA | amd64 | IPv6 |
| Baremetal | HA | amd64 | Dual |

The AWS Single row is the one to watch, because single-node is where every enforcement change is an API outage rather than a rolling update; the three Baremetal rows matter because the IPv6 jobs run against a **disconnected** cluster. Nothing here reaches outside the cluster, so the tests are expected to pass disconnected unmodified — but that is an assumption to be proven on the lane rather than argued. **No testing exception is sought**: the feature is supported on every platform in the list, so the [SBAR](https://docs.google.com/presentation/d/1djF3MaC7rgKFC3_8SPelRcBUB825vx0FO8tz_HwzeWM/edit?usp=sharing) route is not available for convenience, and if coverage proves unreachable on a platform that becomes a blocker to raise rather than a waiver to request. HyperShift and MicroShift are not on this list and are covered by their own sign-offs, in [Hypershift / Hosted Control Planes](#hypershift--hosted-control-planes) and [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift).

**Promotion mechanics.** `/test verify-feature-promotion` must pass on the `openshift/api` promotion PR; which tests it counts can be previewed at `https://sippy.dptools.openshift.org/sippy-ng/feature_gates/{openshiftRelease}/OpenShiftPodSecurityAdmission`. Promotion is also not backportable to a z-stream except through SBAR, which matters here only in that the posture change cannot be hurried into an earlier release if `n+1` slips.

**Also required, and not covered by the testing bar:**

- user-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/) covering the opt-in workflow, the dry-run diagnostic, the break-glass procedure and the label-removal procedure — load-bearing rather than supplementary, because with no status and no alert the documentation is the diagnostic;
- the dry-run sweep collected by must-gather;
- answers to [Is a shell loop an adequate diagnostic](#is-a-shell-loop-an-adequate-diagnostic) and to [the invisible clamp](#enforceversion-and-why-the-default-moves);
- the OKE question in [OpenShift Kubernetes Engine](#openshift-kubernetes-engine), where the alert and metric are unavailable.

### Removing a deprecated feature

This enhancement removes no deprecated API. It removes two behaviours outright: the label syncer stops running in release `n` and does not run again in any configuration, with the `enforce` labels it wrote left unmaintained and `MinimallySufficientPodSecurityStandard` written and read by nothing; and the `PodSecurityReadinessController` stops running, withdrawing six ClusterOperator conditions that are public surface today. There is no supported setting under which either returns, which is unusual enough that the release note must say it plainly.

The annotation is removed from use without being removed from the API: `securityv1.MinimallySufficientPodSecurityStandard` stays defined in `openshift/api`, because existing clusters carry retained values under it and deleting the constant would strand data support still reads. The *code* stays, because the gate-off branch and MicroShift both still run the syncer that writes it. The advising mode is nonetheless deleted: it has no caller, and removing it also removes the non-deterministic server-side-apply decay described in [Why it is retired outright](#why-it-is-retired-outright).

## Upgrade / Downgrade Strategy

### On Upgrade

#### Release plan

No change is backported to release `n-1`: it contains no consumer of the API. It does *not* follow that a stored value survives a downgrade — because the API is a gated field rather than a CRD, `n-1`'s schema prunes it. See [On Downgrade](#on-downgrade).

- **Release `n` — invisible to `Default` clusters.** Redefine `OpenShiftPodSecurityAdmission` and remove it from `inDefault()` and `inOKD()` in the same change, so the new meaning exists only in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`. Invert the Config Observer's two branches, invert the syncer's two explicit branches while leaving the absent branch enforcing, make the `PodSecurityReadinessController` conditional on the gate, and ship `spec.podSecurityAdmission` behind the same gate. A `Default` cluster upgrading into `n` sees none of it.
- **Release `n+1` — everything arrives at once.** Return the gate to `Default` and `OKD`. The global default becomes `privileged`, the field becomes generally available, the syncer and the readiness controller stop running, and the six ClusterOperator conditions are withdrawn — in one upgrade. No API version moves, because the field is already on a `v1` type.

**The release note therefore belongs to `n+1`, not `n`.** Everything in [Release note](#release-note) describes the upgrade into the release where the gate returns to `Default`. Release `n` needs no note beyond the Tech Preview announcement.

#### The state a cluster actually arrives in

**Upgrading `n-1` → `n` changes nothing.** The gate leaves `Default` in the same release its meaning changes, so a `Default` cluster keeps `enforce: restricted`, keeps its enforcing syncer and keeps its readiness controller; no revision is cut on account of this enhancement, no label moves, no condition is withdrawn. What follows is therefore the upgrade that matters — **`n` → `n+1`, when the gate returns to `Default`** — and applies unchanged to a cluster arriving from `n-1` by way of `n`.

Such a cluster arrives with `apiserver/cluster` already present, because the installer rendered it at install time, and with `podSecurityAdmission` unset, because the field only entered its schema at that moment. That is the state every upgraded cluster is in rather than an edge case, and an unset field resolves to the gate-implied default. The sequence is: the kube-apiserver rolls to a revision whose PodSecurity block says `privileged`; `cluster-policy-controller` restarts with the label syncer not running; `cluster-kube-apiserver-operator` restarts without the readiness controller and clears the conditions it left behind; and nothing further happens until an administrator sets `enforceLevel`. **Enforcement is never raised as a side effect of the upgrade**, and the relaxation is not retroactive either — the global default governs only Namespaces with no `enforce` label, and on an upgraded cluster most have one ([Retained labels on upgraded clusters](#retained-labels-on-upgraded-clusters)). Where the resource is genuinely absent, as on MicroShift, the established pattern for an optional config input is [`ObserveMinimumKubeletVersion`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/node/observe_minimum_kubelet_version.go): log `NotFound` and leave the observed config alone.

All three changes are branches of one gate read at startup, so they take effect in whatever order the affected operators happen to restart; the orderings are enumerated in [Intra-rollout skew](#intra-rollout-skew) and none is harmful, but there is no sequencing guarantee to lean on and the design must not acquire one. The one thing the cluster loses immediately is the conditions: an administrator reading `PodSecurity*EvaluationConditionsDetected` on `oc get co/kube-apiserver` finds them gone at the same moment the default posture changes and the syncer stops. All of it landing in one upgrade is an argument for an unusually explicit [release note](#release-note), not an argument against the design — the alternative was spreading the same surprises across two releases.

#### `unsupportedConfigOverrides` already blocks the upgrade

`targetconfigcontroller.manageKubeAPIServerConfig` layers `defaultconfig.yaml`, the authorization-mode override, `config-overrides.yaml`, `spec.observedConfig` and finally `spec.unsupportedConfigOverrides`, which is merged last and wins. On a cluster carrying the PSA override documented in the [original PSA enhancement](pod-security-admission.md), `enforceLevel` is therefore silently inert while `status` reports success.

No new condition is needed. library-go's `UnsupportedConfigOverridesController`, already run by the operator, sets `UnsupportedConfigOverridesUpgradeable=False` with reason `UnsupportedConfigOverridesSet` whenever the field is non-empty, enumerating the overridden leaf paths. What it does not say is that this is why the requested mode is being ignored; making that connection is a [support procedure](#psa-enforcement-configuration-appears-to-have-no-effect), and with no status on `apiserver/cluster` there is now nothing else on the cluster that could even hint at it. Nothing in this enhancement detects, reconciles or removes the override.

### On Downgrade

Y-stream downgrade is not a supported OpenShift operation, so this describes recovery rather than a guarantee.

**Downgrading `n` → `n-1` is a non-event**, because release `n` changed nothing for a `Default` cluster: the gate is out of `Default` on both sides, both releases enforce `restricted`, and both run the syncer and the readiness controller. There is no stored `podSecurityAdmission` to lose, because the field was never in a `Default` cluster's schema.

**The downgrade that matters is `n+1` → `n`**, and it **restores mandatory enforcement**. Release `n` has the gate out of `Default`, so an `n+1` cluster moving back lands on the gate-off branch: `enforce: restricted`, syncer enforcing, readiness controller running. For a cluster that had opted out with `Privileged`, this silently re-enables enforcement on a cluster that was opted out precisely because it could not survive enforcement. This is the dangerous direction and must be called out in the release note; the reverse is benign.

Single-gating makes this *worse than a two-gate design would have*, and the trade should be seen plainly: because one gate carries the posture, the syncer and the readiness controller together, a downgrade does not merely re-enable enforcement — it simultaneously restarts a syncer that will begin rewriting `enforce` labels across the cluster against SCC state that may have moved on considerably since it last ran. The recovery is `unsupportedConfigOverrides`, which does work — `n`'s observer unconditionally overwrites the whole PodSecurity defaults block and the override is merged afterwards — at the cost of pinning the cluster at `n` until it is removed. It does not stop the syncer.

Everything from here to the end of this section describes the `n+1` → `n` direction; `n-1` is named only where the schema differs.

#### Schema pruning erases the configuration

This is the one respect in which a dedicated CRD would have behaved better. A CRD and its stored objects survive a downgrade untouched; a *field* does not. Release `n`'s `Default` `APIServer` schema has no `podSecurityAdmission` — the gate is out of `Default` there, which is the whole point of release `n` — and a structural schema with `preserveUnknownFields: false`, which every `config.openshift.io` CRD has, prunes what it does not recognise. Two things follow and both need stating in the release note.

- **The administrator's recorded intent is destroyed, not merely ignored.** Re-upgrading to `n+1` does not bring it back: the field returns to the schema empty, and the cluster reads as never having opted in.
- **The timing is not the downgrade.** Pruning happens on write, and nothing on `n` writes `apiserver/cluster` as a matter of course. The field may sit in etcd, unserved and unreachable, until some unrelated action — a `tlsSecurityProfile` change, a GitOps re-apply, a storage migration — takes the subtree with it. A support engineer looking the day after the downgrade and a week after it may see different things.

There is no status subtree to prune, and no condition anywhere that a downgrade could strand. The previous revision needed a rule constraining where condition types could live, and a support procedure for removing ones left behind on the operator CR; a write-only API has neither problem. **Downgrade is the clearest benefit of dropping status**, and it is worth weighing against [the invisible clamp](#enforceversion-and-why-the-default-moves).

Pruning is also the reason a downgrade cannot be made self-healing by the gate alone. The gate state and the stored field move together — `n` has neither — so there is no release in which the field survives but is ignored, which would have been the recoverable case.

The mitigation is documentation, not code: record `spec.podSecurityAdmission` before downgrading and re-apply it after re-upgrading. Preserving a field across an unsupported operation would mean a CVO-managed backup object or an annotation shadowing the field, and both are worse than the disease.

#### Per-resource downgrade matrix

Keyed on the downgrade that matters, `n+1` → `n`. Every row is the gate going from on to off.

| Resource | State on `n+1` | What `n` does to it | Administrator action |
|---|---|---|---|
| `observedConfig` → `admission.pluginConfig.PodSecurity.configuration.defaults` | `privileged`, or the level and version from `spec` | the gate-off branch unconditionally rewrites the whole six-key block to `restricted` and rolls a new revision | none, unless enforcement must stay off — then apply the override above |
| Namespace `pod-security.kubernetes.io/*` labels | unmaintained but intact in every mode | the syncer starts enforcing again and re-adds and re-owns all three at the minimally sufficient level on every Namespace it controls — computed against *current* SCC state, which may have moved since it last ran | none for syncer-controlled Namespaces; those opted out, or owned by another field manager, keep whatever `n+1` left and must be fixed by hand |
| Namespace `MinimallySufficientPodSecurityStandard` | stale, read by nothing | the syncer rewrites it | none |
| Pod `openshift.io/scc`, `validated-scc-subject-type` | set by SCC admission at creation | nothing; per-Pod and immutable after admission | none |
| `apiserver/cluster` `spec.podSecurityAdmission` | the administrator's recorded intent | pruned by `n`'s `Default` schema, on the next write by anyone | record before downgrading and re-apply after re-upgrading; nothing recovers it automatically |
| `PodSecurity*EvaluationConditionsDetected` on `co/kube-apiserver` | removed by `n+1`, absent | the readiness controller starts and writes them again | none; they return by themselves |

**Every row is either benign or recovered by `n` resuming what it used to do.** Single-gating is what makes that true: because the gate-off branch is required to be byte-for-byte today's behaviour, a downgrade is not a partial rollback into a state nobody tested but a return to the one that has shipped for years. The syncer and the readiness controller both come back, with their labels, annotations and conditions. The one row that does not recover is `spec.podSecurityAdmission`, which has no owner on `n` to rewrite it.

The same property is what makes the rows *consequential* rather than inert, and the table should be read both ways: the labels row is a fleet-wide write, not a no-op, and it is the reason the release note has to treat `n+1` → `n` as a posture change rather than a version change.

Re-upgrading `n` → `n+1` is correspondingly dull. If the field was already pruned the cluster arrives with no recorded intent, which is safe; if it was not — because nothing wrote the object while on `n` — it returns intact and is acted on, so an administrator who downgraded away from an enforcing configuration gets it back without re-requesting it. That is correct but worth stating in the release note alongside the pruning case, since the two outcomes differ on nothing the administrator did. There is no stale status to reconcile on the way back up, because there is no status.

## Version Skew Strategy

Four skew windows matter, each a window in which two components disagree about the effective PSA level. A fifth, in which a cluster passed through release `n` too quickly for the readiness evaluation to complete, is closed by that evaluation no longer existing.

The second of the four is new to this revision and is created by the design itself: redefining `OpenShiftPodSecurityAdmission` rather than adding a gate means the *same string* denotes different behaviour in different releases, and anything that reads the gate across a version boundary has to be told which release's meaning applies.

### Intra-rollout skew

Changing the PodSecurity configuration produces a new static pod revision, and revisions roll one master at a time. For minutes to tens of minutes one kube-apiserver enforces the new level while the others enforce the old one, and which one a Pod creation reaches is a load-balancer decision. A Deployment scaling from 0 to 5 can have some replicas admitted and some rejected. This window exists on every change, in both directions, and removing the interlock is what stopped it being safe by construction.

**Raising** enforcement was, two revisions ago, allowed only once the evaluation reported the cluster clean, so the expected number of rejections during the mixed window was zero. Nothing guarantees that now: a raise on a cluster with violations produces a window in which the *same* Pod creation succeeds or fails depending on which master it reaches, which is worse than failing consistently because it looks like flakiness rather than policy. The window is bounded by the rollout and ends in the stricter state, but the administrator entering it has had no warning from the cluster at all — not an interlock, not a condition, not an advisory header — so whether they knew is entirely a function of whether they ran the dry-run. It is why the break-glass path has to be fast. **Lowering** enforcement, including break-glass, is likewise *not* instantaneous; support must be told to wait for every `currentRevision` to equal `latestAvailableRevision` rather than concluding the change did not take. On single-node deployments there is no mixed window, because there is only one kube-apiserver; there is an API outage instead while the static pod restarts.

### Operator skew

Retiring both controllers removes most of this problem, and reading `spec` directly removes the rest: the Config Observer's input is the administrator's own field rather than another controller's output, so there is no producer and consumer that can get out of step, and after release `n` the observer is the only component in the picture. `cluster-policy-controller` reads none of it.

What remains is a transient during the upgrade itself. `cluster-policy-controller` runs as a container in the kube-controller-manager static pod, owned by a different operator, with no ordering guarantee — so for part of every upgrade the kube-apiserver has already rolled to `privileged` while an `n-1` `cluster-policy-controller` is still stamping labels, or the syncer has already stopped while the kube-apiserver is still enforcing `restricted`. Neither is harmful: the first writes labels that are merely redundant and become the retained labels; the second changes nothing.

The enum is the sharper edge, and it applies to out-of-payload and hosted-control-plane consumers. A consumer from an earlier release that encounters an unknown value must **preserve its existing behaviour** — not error, and not treat the field as unset. Treating it as unset would silently relax enforcement; erroring would degrade a ClusterOperator over a value valid on the cluster's own API server. So consumers switch on the values they know and fall through to "make no change to the currently effective level", and the enum is only ever extended. Because the API server validating a write is always at least as new as any consumer, a value can be accepted by validation and be unknown to a consumer; that asymmetry is normal during an upgrade, not a bug to be designed out.

### Gate redefinition skew

A feature gate's name is normally a stable key: a component that reads `OpenShiftPodSecurityAdmission=true` can rely on it meaning the same thing in every release that has the gate. Redefining the gate breaks that, so this section states exactly where the breakage is reachable and what bounds it.

**It is unreachable on a standalone cluster.** The reader and the gate list ship in the same payload: `cluster-kube-apiserver-operator` and `cluster-policy-controller` are both rendered from the release the cluster is on, and `FeatureGate` status is reconciled by the CVO from that same release. There is no supported way to put a release-`n+1` reader in front of a release-`n-1` gate list. Intra-upgrade, the two can disagree for minutes — that is [Operator skew](#operator-skew) — but each component reads a list produced by a payload whose semantics it shares, because the gate's membership in `Default` changes in lockstep with its meaning.

**It is reachable in HyperShift, and only there**, because the reader and the gate list come from different releases by design. The mechanics, the exposure and the fix are in [HyperShift skew](#hypershift-skew).

This is the concrete cost of choosing one gate over two, and it should be weighed against [Ship the posture change a release ahead of the API](#ship-the-posture-change-a-release-ahead-of-the-api) rather than treated as incidental. A second gate would have needed no version gate anywhere, because a new name has no old meaning to collide with.

### HyperShift skew

**HyperShift is the one place the [gate redefinition](#gate-redefinition-skew) is reachable**, because the reader and the gate list come from different releases by design. The `control-plane-operator` on the management cluster renders the hosted kube-apiserver's PodSecurity configuration by string-matching `OpenShiftPodSecurityAdmission=true` in the hosted cluster's rendered gate list (`.../v2/kas/config.go`), and the management cluster is required to be at or ahead of the hosted one. A CPO carrying the new semantics therefore meets gate lists written under the old ones:

| Hosted release | Gate in its `Default` list | Old meaning | A new-semantics CPO would render |
|---|---|---|---|
| `≤ n-1` | `=true` | enforce `restricted`, run the syncer | `privileged`, syncer stopped — **wrong** |
| `n` | `=false` | — | `restricted`, syncer running — correct |
| `≥ n+1` | `=true` | — | `privileged` or the configured level — correct |

Only the first row is wrong, and it is wrong in the dangerous direction: a **silent posture reduction** on a hosted cluster that asked for nothing, from `restricted` to `privileged`, with the label syncer stopped at the same time. Release `n` is the row that is correct by construction — removing the gate from `Default` in the same release its meaning changes is what makes the old and new readings agree there, and that is the property the single-gate design rests on.

**The fix is a bounded version gate in the CPO.** The PodSecurity branch — and the equivalent decision in the hosted `cluster-policy-controller` deployment (`.../v2/clusterpolicy/component.go`) — must be keyed on the *hosted control plane's* release version, not on the gate string alone: below `n`, use the old mapping regardless of the gate; at `n` and above, use the new one. The CPO already resolves the hosted release version and already version-gates other behaviour on it, so this is a known pattern rather than new machinery. The exposure is bounded on both sides — it covers exactly the hosted releases below `n`, and it **sunsets when the oldest supported hosted release reaches `n`**, at which point the version gate is dead code and can be deleted. That sunset should be written into the code as a comment naming the release, because an unlabelled version gate is the kind of thing that survives for years.

Underneath the redefinition there is a standing skew about *configuration* rather than semantics, and it does not sunset. The skew is one-directional and can span several releases: a `control-plane-operator` at `n+3` may be rendering for a hosted cluster whose payload is at `n+1`. A hosted cluster's opt-in therefore cannot be expressed purely in the guest without the management side honouring it, and the management side is the one that may be arbitrarily newer. The surface itself is settled by the API being spec-only: `HostedCluster.spec.configuration.apiServer` is a `*configv1.APIServerSpec`, so `podSecurityAdmission` arrives on the management side with no new plumbing and nothing has to be reported back into the guest. What remains is an implementation obligation on HyperShift rather than an open design question — `control-plane-operator` must read the field, apply the same clamp against the *hosted* cluster's Kubernetes version rather than the management cluster's, and preserve its current behaviour for an `enforceLevel` value it does not recognise. **HyperShift reviewers are required to accept that obligation**, and the version gate above with it; together they are the main unresolved dependency here.

### EUS-to-EUS skip upgrades

This was a problem and is no longer one, which is worth recording because it was the readiness controller's sharpest skew hazard. The controller resynced every four hours with a throttled client, so a 5.0 → 5.1 → 5.2 skip could pass through 5.1 without ever completing a sweep — leaving no assessment available in the one release where the administrator was expected to make the opt-in decision.

With no evaluation, nothing is in flight to be interrupted. Enforcement follows `spec`, `spec` is carried across the skip untouched, and a cluster arrives at 5.2 enforcing exactly what was asked for. The gate is out of `Default` in both 5.1 and 5.2, so a cluster that has not opted in stays `privileged` throughout. The administrator passing through 5.1 has the same diagnostics as one sitting on it: the dry-run, whenever they choose to run it.

## Operational Aspects of API Extensions

### In general

- Administrators facing issues on a cluster set to a stricter level can change `enforceLevel` to `Privileged` to halt enforcement. Nothing can block that change, because nothing is consulted before it is applied.
- ClusterAdmins must ensure that directly created workloads (user-based SCCs) have correct `securityContext` settings. Updating default workload templates can help.
- **Raising the enforcement level never evicts a running workload.** PSA acts only on Pod creation. The corollary is that a workload can be admitted today and rejected weeks later when a node drain, upgrade or crash-loop restart recreates it — see [What opting in still risks](#what-opting-in-still-risks-with-nothing-evaluating).
- **PSA denials are not visible in `kubectl get pods`.** When a Pod is created by a controller the rejection surfaces on the owning object, because no Pod is ever created — on the ReplicaSet for a Deployment, on the Job for a CronJob. `kubectl -n $NAMESPACE get events --field-selector reason=FailedCreate` is the most common source of confusion when debugging PSA, and support should reach for it first.
- To identify specific problems in a violating Namespace, `kubectl label --dry-run=server --overwrite ns/$NAMESPACE pod-security.kubernetes.io/enforce=restricted`. From release `n` this is the only mechanism, for administrators and for support alike: it is also the answer to "what standard does this Namespace need?", which the platform no longer answers on its own. It reports only on Pods that exist at that moment.
- **Nothing about PSA is readable from `apiserver/cluster` except what someone wrote there.** Support asking what a cluster is enforcing reads the revisioned `config-<revision>` ConfigMap; asking whether it *should* be enforcing reads `spec`. The two can differ, and no component on the cluster notices.

### Health and failure modes of the extension

The `podSecurityAdmission` field introduces no webhook, no aggregated apiserver and no controller, so it adds no latency to any request path and cannot make another resource unavailable. Because it extends a resource every cluster already has, it adds no new watch, no new CRD and no new object for the CVO to manage, and because nothing writes status it adds no write traffic to `apiserver/cluster` at all — the previous revision's four-hourly status write, which would have woken every watcher of that resource, is gone with the controller.

**The failure modes are the observer's, and there are only three.** This table is much shorter than the previous revision's, and the reason is worth stating: every row that was removed described a way the readiness evaluation could fail, which could never affect enforcement in either direction. Removing a component whose failure modes were all benign removes them from the table without making anything safer — what it removes is the *diagnosis* those rows described.

| Failure mode | Signal | Effect |
|---|---|---|
| field absent from the schema, or `apiserver/cluster` absent | none | the observer falls back to the feature-gate-implied default, `privileged` from release `n` |
| `spec.enforceVersion` newer than the payload | **none** | silently clamped down; recoverable only from the rendered configuration, see [the invisible clamp](#enforceversion-and-why-the-default-moves) |
| `unsupportedConfigOverrides` set | ClusterOperator `Upgradeable=False`, reason `UnsupportedConfigOverridesSet` | `enforceLevel` is silently inert; the cluster cannot upgrade until the override is removed |

Two of those three signal nothing specific to PSA, which is the operational character of this feature: it is configured in one place and observed in another, and the connection between them is made by a person following a documented procedure.

Escalation goes to the OpenShift Auth team. Failures manifesting as admission rejections in `openshift-*` Namespaces are likely to involve OLM and the owning layered product.

## Support Procedures

The procedures themselves are written as `openshift-docs` modules rather than
carried here. This section records the set that has to exist, and the design
consequences the procedures are wrong without.

**No `openshift/runbooks` entry is needed.** This enhancement ships no alert, so
there is nothing to write one against — a change from the previous revision,
which owed a runbook for `PodSecurityReadinessEvaluationStale` before that alert
could merge.

| Procedure | Purpose | Destination |
|---|---|---|
| [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect) | the causes: `unsupportedConfigOverrides`, an unfinished rollout, retained labels, a clamped version | openshift-docs, support KCS |
| [Removing retained `enforce` labels](#removing-retained-enforce-labels) | the only mechanism by which a syncer-written label is ever removed | openshift-docs, **linked from the release note** |
| Break glass: disabling enforcement | `Privileged`, its rollout cost, and the operator-wedged fallback | openshift-docs, support KCS |
| Finding and resolving violating Namespaces | the dry-run sweep, then per-reason remediation, including `openshift-operators` | openshift-docs |

**must-gather must collect the dry-run sweep**, and this is now a hard
requirement rather than a convenience. Nothing on the cluster records PSA
readiness: no status, no condition, no metric, and no annotation since
`MinimallySufficientPodSecurityStandard` stopped being maintained. A support
bundle from a release `n` cluster that does not run the sweep at collection time
contains **nothing whatsoever** about whether the cluster can meet a standard.
It is `O(Namespaces)` serial API calls against a cluster that may already be
unhealthy, so it needs a bound and a partial-output marker, and the bundle must
record when it ran — the output is a point-in-time sweep, not cluster state.

### PSA enforcement configuration appears to have no effect

**There is no `status` to check, and that is the first thing support must
internalise.** `spec.podSecurityAdmission` records what somebody asked for and
nothing records what happened. `enforceLevel` can be silently inert:
`unsupportedConfigOverrides` is merged last and wins, any per-Namespace label
outranks the cluster-wide default, and a rollout in progress means different
masters are enforcing different levels. The effective configuration is read from
the revision each master is actually running — `config-<revision>` in
`openshift-kube-apiserver`, whose `config.yaml` key holds JSON because the merge
encodes through `UnstructuredJSONScheme`. The `UnsupportedConfigOverridesSet`
condition enumerates the overridden leaf paths but does not say that this is why
`spec.podSecurityAdmission` is being ignored. A clamped `enforceVersion` is
diagnosed the same way, and only that way.

### Removing retained `enforce` labels

**Removal is ordered, and its provenance check fails unsafe.** Lower the
cluster-wide default and wait for the rollout *before* removing any label, or a
Namespace that just lost a `baseline` label is briefly governed by a default
still set to `restricted`. Ownership is recorded per label key in
`metadata.managedFields`; a manager of
`pod-security-admission-label-synchronization-controller` or the historical
`cluster-policy-controller` marks a label as safe to remove. **No recorded owner
is ambiguous, not safe** — etcd restore and some migration paths drop
`managedFields` — and guessing wrong silently relaxes a Namespace somebody
intended to constrain. Deletion is irreversible, so on a cluster attesting under
FedRAMP, PCI or DISA STIG each removal is a change to an audited control.

### Granting an SCC does not change what PSA enforces

With the syncer retired, fixing a Namespace's ServiceAccount writes no label, so
a Namespace needing an effective level other than the cluster-wide one must be
labelled by hand. On a cluster that has opted in, that is the only mechanism.
With the readiness controller retired as well, the fix also produces no change
anywhere the administrator can observe: re-running the dry-run is the
confirmation.

**Still unwritten**, and required before GA: the exact PSA denial message;
audit-log correlation via the `pod-security.kubernetes.io/enforce-policy`
annotation; and must-gather coverage for `apiserver/cluster`, the effective
admission configuration, and the sweep. The previous revision also owed symptoms
and log lines for a failing readiness controller, which is no longer a component.

## Infrastructure Needed

New periodic CI lanes are needed to satisfy the graduation criteria: the feature has to be exercised in both the `TechPreviewNoUpgrade` and `Default` Prow variants, across the provider, topology, architecture and network variants required by `dev-guide/feature-zero-to-hero.md`, at a frequency that reaches the required number of runs per platform before branch cut. The exact lane list follows from the reworked [Graduation Criteria](#graduation-criteria) and is not yet enumerated.
