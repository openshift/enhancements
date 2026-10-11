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

[Pod Security Admission (PSA)](https://kubernetes.io/docs/concepts/security/pod-security-admission/) enforcement **has never been enabled by default in a shipped OpenShift release**: the `OpenShiftPodSecurityAdmission` feature gate that would turn it on is in `inDefault()` on `master` but has been stripped from `Default` on every release branch since it was introduced in 4.12, so every customer cluster runs `enforce: privileged` today — see [What actually ships](#what-actually-ships), the single most important fact for reviewing this document. **This enhancement makes that de facto posture official and configurable**: a new `podSecurityAdmission` field on the existing `config.openshift.io/v1 APIServer` resource records the [Pod Security Standard (PSS)](https://kubernetes.io/docs/concepts/security/pod-security-standards/) the administrator wants enforced and the version it is evaluated against, and the two components built to carry the never-completed migration to `restricted` — the PSA label syncer and the `PodSecurityReadinessController` — are retired with it. All three changes are carried by the existing feature gate, redefined rather than replaced, so they land together or not at all. **No customer cluster changes enforcement posture**: the default stays `privileged` and no `enforce` label moves. What customers do lose is reporting, in three parts — the advising syncer's `warn`/`audit` labels stop being maintained; the six `PodSecurity*EvaluationConditionsDetected` ClusterOperator conditions are withdrawn; and, from `n+1`, **`warn` and `audit` follow `enforceLevel` rather than staying pinned at `restricted`**, so an opted-out cluster reports nothing at all. The last of those is a deliberate choice to stop warning every user of every cluster about a standard the product is not enforcing, and its cost is set out in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).

This enhancement expands the ["PodSecurity admission in OpenShift"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission.md) and ["Pod Security Admission Autolabeling"](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/enhancements/authentication/pod-security-admission-autolabeling.md) enhancements.

Terms used throughout: **opting in** is `spec.podSecurityAdmission.enforceLevel: Baseline` or `Restricted`; **opting out** is `Privileged` or leaving the field unset, which are the same state and the shipped default. `spec.enforceLevel` is shorthand for the full path on `apiserver/cluster`. Disabling enforcement means **`enforce`, `warn` and `audit` all at `privileged`** — the `PodSecurity` plugin stays loaded and per-Namespace labels keep taking precedence, but the cluster reports nothing. **All three levels move together**, derived from the single `enforceLevel` field, so a cluster is either enforcing and reporting at one standard or doing neither; there is no mode in which it reports against a standard stricter than the one it enforces. The requested level is applied as requested: there is no evaluation to wait for and no condition to clear, because retiring the `PodSecurityReadinessController` removes the component that would have supplied them. What an administrator gets instead is set out in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).

## Motivation

After PSA and SCC-based autolabeling were introduced, some clusters were found to have Namespaces with Pod Security violations. The number has dropped significantly over the last several releases, but it is not zero, and it is essential to avoid any scenario where users end up with failing workloads.

That finding is why the gate has been held out of `Default` at every branch cut since 4.12. The migration to enforcement was never cancelled on paper; it was deferred, silently, nine releases running. The cost of leaving it that way is that the product's actual posture is set by a branching convention rather than by a decision anyone wrote down, the API says nothing about it, and an administrator who *wants* enforcement has no supported way to ask for it — `TechPreviewNoUpgrade` is not an answer, because it is irreversible and disqualifies the cluster from upgrades.

Making enforcement elective converts that into a decision. It puts the responsibility on the administrator to have PSA-compliant workloads before enforcement is turned on, rather than discovering non-compliance as admission failures after an upgrade — and, for the first time, gives them a way to turn it on at all. Out of the box, `Privileged` is the global default, as it already is, and neither the label syncer nor the readiness controller runs.

Both of those components served the same cancelled plan. The syncer existed to make a mandatory `restricted` default survivable; the readiness controller existed to tell Red Hat which clusters were ready to be moved to enforcement and to find, through [ClusterFleetEvaluation](https://github.com/openshift/enhancements/blob/61581dcd985130357d6e4b0e72b87ee35394bf6e/dev-guide/cluster-fleet-evaluation.md), what was stopping the rest. Neither question survives delegating the decision to the administrator: nobody is being moved, so there is no readiness to assess and no fleet rollout to sequence.

OpenShift strives to offer the highest security standards, and PSA enforcement was intended to serve that. This enhancement does not trade that away — release branching already did, nine times. What it trades is the *pretence*: rather than carrying a default nobody ships and a migration nobody runs, it gives administrators an explicit, reversible lever. Clusters that opt in reach the posture the migration was aiming for; clusters that do not remain at `Privileged`, exactly where they already are.

**Clusters taking no action are no less restrictive than they are today.** The residual security cost is threefold, and all three are losses of *reporting* rather than of enforcement: the advising syncer's `warn`/`audit` labels stop being maintained; `warn` and `audit` follow `enforceLevel` instead of staying pinned at `restricted`, so an opted-out cluster emits no PSA signal at all and `PodSecurityViolation` cannot fire on it; and `privileged` becomes a declared default that an administrator must actively leave, rather than a transitional state the product was notionally moving off.

**The second of those is a deliberate reversal and the sharpest trade in this enhancement.** Pinning `warn` at `restricted` on a cluster enforcing `privileged` means every user of every cluster in the fleet gets a terminal warning on every `oc apply` of a Pod that would fail a standard the cluster has no intention of applying — nine releases of warnings about a migration that never happened. Making the three levels move together ends that, at the cost of the only continuous PSA signal an opted-out cluster had. The case against is in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in), and a reviewer who thinks the signal is worth the noise should say so there. Per-Namespace `pod-security.kubernetes.io/*` labels continue to take precedence and are unaffected, and SCCs — OpenShift's primary workload admission control — are unchanged.

#### `Privileged` is a posture, not only an opt-out

The rest of this document mostly treats `Privileged` as the state a cluster is in before it opts in, or the state it breaks glass back to. That framing undersells one of the user stories above, and the omission is worth correcting here rather than leaving a reviewer to supply the argument.

**PSA is not OpenShift's primary workload admission control; SCC is.** The two overlap substantially — both constrain privilege escalation, host namespaces, capabilities and `runAsUser` — and SCC is the one that has governed every OpenShift cluster since before PSA existed, is enforced on every Pod regardless of Namespace labels, and is unaffected by anything in this enhancement. An administrator who sets `Privileged` deliberately, having decided that their SCC configuration already expresses the constraints they want, is choosing **one admission control rather than two**, not choosing none. That is a defensible posture, and it is a different act from an administrator who has never heard of the field.

This matters in three places. It is why [Goal 1](#goals) makes the field reversible rather than a one-way hardening ratchet. It is why the enum carries a named `Privileged` value rather than expressing opt-out as absence alone. And it is the counterweight to the first [Drawback](#drawbacks): the product is not abandoning secure-by-default so much as declining to add a *second* control on top of the one it already enforces, on clusters whose administrators have not asked for it. The drawback stands — a cluster running both is better defended than a cluster running one — but it is a narrower loss than "enforcement is now off".

### Goals

1. **Allow administrators to configure Pod Security Admission enforcement on their clusters, and to change it at will.** The level is one field on an existing resource, settable and reversible at any time. There is no one-way door and no acknowledgement to clear: a cluster that raises to `Restricted` returns to `Privileged` with a single write, and the reverse.
2. **Ensure no cluster acquires failing workloads without its administrator having asked for the change.** Enforcement is elective and stays where it is unless somebody raises it, so neither this enhancement nor any upgrade under it causes a rejection. This is a guarantee about what the *platform* does unbidden, and it is the only guarantee in this area — see Goal 3.
3. **Make opting in as safe as it can be made, while guaranteeing nothing about the outcome.** Once an administrator raises the level, workloads can and will fail, and no component on the cluster can prevent that or reliably predict it. What this enhancement owes is a cluster that is as easy as possible to assess beforehand, to diagnose during, and to back out of afterwards: a documented dry-run sweep, `warn` and `audit` tracking `enforceLevel` so that a cluster which has opted in reports violations at exactly the standard it is applying, a retained `PodSecurityViolation` alert on opted-in clusters, and a break-glass path that is one field write. It does not owe, and does not provide, an assurance that opting in is free of breakage. **Note what is no longer on that list**: violations are *not* visible in every mode. An opted-out cluster reports nothing, so the dry-run sweep is the whole of the pre-flight assessment — see [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).
4. **Report the enforcement configuration actually in force, so the API can be read as well as written.** `APIServerStatus` gains the resolved `enforceLevel` and `enforceVersion`, which is how an administrator finds out that a version they requested was silently clamped and that the level they asked for was accepted. **It reports configuration, not readiness.** Nothing answers "would this cluster survive `restricted`?" as a standing value, and adding a violation count, a Namespace list or a condition to this subtree is out of scope — not merely because the `PodSecurityReadinessController` is retired, but because any such field needs a recurring evaluation of what the cluster contains, which is the cost this enhancement is declining. The status is a report of what the config observer resolved; it is not a per-master statement either. See [What `status` does not say](#what-status-does-not-say), [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in) and [Keep the readiness controller](#keep-the-readiness-controller).
5. **Retire the SCC-mapping PSA label synchronization controller outright** — on every product, in every enforcement mode — and deprecate the two pieces of Namespace metadata it owns, `security.openshift.io/MinimallySufficientPodSecurityStandard` and the `security.openshift.io/scc.podSecurityLabelSync` opt-out label. This is not conditional on the new API and is not reversed by opting in: it happens whatever `spec.enforceLevel` says, and it happens on MicroShift too. It has the largest blast radius of anything in this enhancement, and reviewing the API without reviewing this has not reviewed the change.
6. **Retire the `PodSecurityReadinessController`**, and with it the PSA ClusterFleetEvaluation. Also unconditional, and also not reversed by opting in.

### Non-Goals

1. **Enabling PSA enforcement by default.** This enhancement does not complete the deferred migration, and does not move any customer cluster to `restricted`. It makes enforcement opt-in and leaves the shipped default where release branching has kept it since 4.12. The clusters whose level does move are `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade` ones, which move *down* from `restricted` to `privileged` — and, since `warn` and `audit` follow the level, from reporting everything to reporting nothing. **A preview cluster that wants the old behaviour must now ask for it**, by setting `enforceLevel: Restricted`; there is no feature-set-dependent default, and the observer cannot distinguish a preview cluster from a `Default` one. CI lanes exercising enforcement therefore have to set the field rather than inherit it, which is the subject of [How CI lanes get a `restricted` cluster](#how-ci-lanes-get-a-restricted-cluster).
2. **Preventing workload failures after an opt-in, or gating the requested level on a clean evaluation.** An administrator who asks for `Restricted` gets `restricted`, whether or not the cluster would pass it: no acknowledgement token, no blocked state, no condition to clear first. This is listed because it is the thing readers will most want the enhancement to do, and it cannot be done — not merely because the readiness controller is retired, but because PSA evaluates at Pod creation, so a cluster that is clean when assessed can fail minutes later as a Deployment scales, a CronJob fires, a node drains, or a Namespace is created. An interlock built on a point-in-time sweep would gate a real decision on an assessment with known blind spots, and the retired controller did not close that gap either. The commitment is [Goal 3](#goals)'s: make the decision as well-informed and as reversible as the platform can make it, and be explicit that the residual risk sits with the administrator who opts in.
3. **Restricting per-Namespace PSA configuration.** Per-Namespace labels continue to take precedence over the cluster-wide default. This enhancement only changes the global default applied to Namespaces carrying no `enforce` label.

## Proposal

### User Stories

As a System Administrator:

- I want to enable PSA enforcement as I have labelled my namespaces effectively and I am confident that my workloads will not break.
- I want to pin the Pod Security Standards revision my cluster is evaluated against, so that upgrading the cluster does not tighten the standard my workloads are held to without my asking.
- I want to read back the enforcement level and standards revision actually in force, so that I can confirm my change was observed and detect a version that was clamped.
- I want to revert my PSA enforcement from `restricted` to either `baseline` or `privileged` because I discovered failing workloads.
- I want to move PSA enforcement from either `restricted` or `baseline` to `privileged` as I want to rely solely on workload SCCs instead.

### Workflow Description

The **cluster administrator** is responsible for the security posture of a cluster and is the only actor that writes `spec.podSecurityAdmission`. The **application owner** deploys workloads and is the one who knows which Pod Security Standard those workloads actually require; they write that requirement nowhere, because the per-Namespace `pod-security.kubernetes.io/*` labels that express it are the administrator's to set, so it reaches the cluster only by being communicated to the administrator out of band. They experience the consequences when a Pod is rejected. The **Config Observer** in `cluster-kube-apiserver-operator` is the single consumer, turning `spec` into the kube-apiserver's global `PodSecurity` admission configuration. The **PSA label syncer** and the **`PodSecurityReadinessController`** are not actors: neither runs in any enforcement mode, and neither reads this API. The syncer appears below only as the source of labels already present on upgraded clusters.

**This section describes what the administrator does, not how to invoke it.** The field it drives — `spec.podSecurityAdmission`, with `enforceLevel` one of `Privileged`, `Baseline` or `Restricted` and `enforceVersion` pinning the standards revision — is defined in [The `podSecurityAdmission` fields](#the-podsecurityadmission-fields), which by the template's ordering comes after this one, and the commands are with it.

The starting state is a cluster on release `n+1` or later with `enforceLevel` unset, resolving to `Privileged`. **The worked example below raises it to `Baseline`, not `Restricted`, because that is the realistic first move and the examples throughout this document should not imply otherwise.** `Baseline` blocks the known privilege escalations — privileged containers, host namespaces, hostPath volumes, dangerous capabilities — while permitting the default Pod configuration, so most workloads that run today pass it unchanged. `Restricted` additionally requires `runAsNonRoot`, `seccompProfile: RuntimeDefault`, dropping all capabilities and disallowing privilege escalation, which a large fraction of real workloads do not satisfy without being modified. An administrator going straight to `Restricted` is making a much bigger change than one going to `Baseline`, and the two should not be presented as interchangeable. Enabling enforcement:

1. The administrator establishes whether the cluster can survive the level they want, using the dry-run sweep in [Finding the violating Namespaces](#finding-the-violating-namespaces). **This step is entirely theirs**, and on an opted-out cluster the sweep is the only instrument there is: the cluster holds no precomputed answer, emits no `audit` signal to read, and will not object if they skip the step. See [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in).
2. They resolve whatever the sweep found, or decide to accept it.
3. They record their intent by setting `enforceLevel` on `apiserver/cluster` to the standard they want. The write is validated for shape and nothing else. **It is never rejected on the strength of what the cluster contains, and never warned about** — no component knows enough to warn. The invocation is in [Setting the field](#setting-the-field), with the type it writes to.
4. The Config Observer re-renders the kube-apiserver configuration with `enforce`, `warn` and `audit` all at `baseline`, cutting a new static pod revision that rolls the control plane one node at a time. Nothing else starts: no controller begins writing Namespace labels, and every Namespace without a `pod-security.kubernetes.io` label of its own is now held to `baseline`.
5. The administrator re-reads `apiserver/cluster` and compares `spec.podSecurityAdmission` against `status.podSecurityAdmission`. Matching values mean the observer resolved what was asked for; a differing `enforceVersion` means it was clamped. **This confirms the decision, not the rollout** — for that they still compare each master's `currentRevision` against `latestAvailableRevision` on `kubeapiserver/cluster`, and for the effective level on a cluster carrying `unsupportedConfigOverrides` they still read the revisioned `config-<revision>` ConfigMap; see [What `status` does not say](#what-status-does-not-say) and [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect).

**Variation — raising again, to `Restricted`.** Having settled at `Baseline`, the administrator repeats the whole loop at the higher standard: re-run the sweep with `=restricted`, resolve or label what it finds, write the field, compare `status`. **This staged path is the one the three-value enum exists for**, and it is strictly safer than a single jump, because the cluster spends the intervening period enforcing *and reporting* at `Baseline` rather than at nothing — a `Restricted` violation in a Namespace that already fails `Baseline` shows up at the first stage, when fewer things are changing at once. It is also the path on which the dry-run is most trustworthy, since each stage is a smaller delta than the last. Nothing in the API encourages or records staging; it is a documentation matter, and the opt-in procedure should present it as the default approach rather than as an advanced one.

**Variation — pinning the standard's version.** `enforceVersion` fixes which revision of the Pod Security Standards is applied, for all three of `enforce`, `warn` and `audit`. Left unset it tracks the payload's Kubernetes version, so the standard tightens as the cluster is upgraded. A value newer than the cluster's own version is clamped rather than rejected, and the clamp is reported: `status.enforceVersion` carries the value actually applied, so comparing it against `spec.enforceVersion` is how an administrator detects one.

**Variation — disabling.** The administrator sets `Privileged`, or removes the field. The global `enforce`, `warn` and `audit` levels all return to `privileged`, so the cluster stops rejecting, stops warning and stops recording audit annotations in one write; Namespaces that already carry a `pod-security.kubernetes.io/*` label keep behaving as that label says. **This is a bigger step than it looks**: it is not only a relaxation of enforcement but the end of all PSA reporting on the cluster, so an administrator breaking glass under pressure also loses the signal that would tell them whether the problem was PSA at all. Dropping to `Baseline` rather than `Privileged` keeps both enforcement and reporting at a level most workloads can meet, and the documentation should present it as the first thing to try. Like every other level change it is applied as requested, and the path is now about as short as it can be made: one field, read by one observer, with no controller in between whose failure could hold it up. Namespaces carrying an administrator-set or payload-set `enforce` label are unaffected in either direction — see [Labels on upgraded clusters](#labels-on-upgraded-clusters).

**Variation — Day 0.** Enforcement can in principle be requested at install time; the surface is not yet specified. See [Open Questions](#day-0-configuration).

### API Extensions

This enhancement adds one optional spec field and one status subtree to an existing API, and changes three existing surfaces. No admission webhooks, conversion webhooks, aggregated API servers or finalizers are introduced.

- **`spec.podSecurityAdmission` on `config.openshift.io/v1 APIServer`** — the installer-rendered cluster-scoped singleton `apiserver/cluster`, carrying the administrator's requested level and PSS version, gated on the redefined `OpenShiftPodSecurityAdmission` feature gate. **No new CRD and no new feature gate is introduced**; both alternatives are weighed in [Alternatives](#alternatives-not-implemented).
- **The kube-apiserver's `PodSecurity` admission plugin configuration** changes meaning in two ways: `enforce` becomes derived from `spec.podSecurityAdmission.enforceLevel` rather than from the `OpenShiftPodSecurityAdmission` feature gate alone, and the three `*-version` keys become derived from `spec.podSecurityAdmission.enforceVersion` rather than fixed. `audit` and `warn` stay pinned to the `restricted` *level*.
- **Namespace metadata written by another component.** The SCC-mapping PSA label syncer, owned by `cluster-policy-controller`, no longer runs, so no `pod-security.kubernetes.io/*` label and no `security.openshift.io/MinimallySufficientPodSecurityStandard` annotation is written by it. Namespaces are a core upstream resource; the change is purely subtractive, and metadata already present is retained untouched. **The separate `privileged-namespaces-psa-label-syncer` is not affected** — see [Two syncers, only one retired](#two-syncers-only-one-retired).
- **Six conditions leave the `kube-apiserver` ClusterOperator.** The `PodSecurity*EvaluationConditionsDetected` family is written by the `PodSecurityReadinessController`, which stops running, so all six stop being set and the existing ones are removed on upgrade. This is a **public status surface being withdrawn**, not reshaped: see [What stops being reported](#what-stops-being-reported).

- **`status.podSecurityAdmission` on the same resource** — the resolved `enforceLevel` and `enforceVersion`, written by the same config observer that resolves them. `APIServerStatus` is an empty struct today ([`types_apiserver.go#L270`](https://github.com/openshift/api/blob/master/config/v1/types_apiserver.go)), so **this enhancement is the first writer of it**, and `cluster-kube-apiserver-operator` stops being a pure reader of `config.openshift.io`. That is a precedent and is argued in [Why there is a status after all](#why-there-is-a-status-after-all).

#### API version and availability

Throughout this document, release `n` is the release in which PSA enforcement stops being the default, currently targeted at **OpenShift 5.1**; `n-1` is 5.0 and `n+1` is 5.2.

`APIServer` is already `config.openshift.io/v1` and is present on every standard cluster, so what ships per release is the **field**, not the type. It carries `+openshift:enable:FeatureGate=OpenShiftPodSecurityAdmission` — the same gate that governs the behaviour, which is the whole point of the single-gate design — so it is in `TechPreviewNoUpgrade` in release `n` and in `Default` from release `n+1`. There is no API version migration: nothing is promoted from `v1alpha1`, no two versions are served at once, and no conversion is written.

The configuration surface is therefore **not** available on a supported, upgradeable cluster in release `n` — but neither is the behaviour it configures. **Release `n` changes nothing for a `Default` cluster**: the gate stays where it already is ([What actually ships](#what-actually-ships)) while its meaning is redefined, and no gate-off branch is edited. Everything here arrives for those clusters when the gate enters `Default` in `n+1`.

**Note what this does *not* claim.** One gate does not avoid a release in which enforcement is off by default with no supported way to turn it back on — that is the state every cluster has been in since 4.12 and will remain in throughout release `n`, and no gating arrangement available here changes it. This enhancement **ends** a nine-release gap rather than avoiding a one-release one; `unsupportedConfigOverrides` remains the only transitional opt-in before `n+1`. What one gate does buy is that the field and the behaviour graduate together, so the field is never served on a cluster whose observer ignores it — see [Why one gate rather than two](#why-one-gate-rather-than-two).

#### The `podSecurityAdmission` fields

**An `openshift/api` PR must be open and linked from this section before the EP moves to `implementable`**; the types below are the proposal, not the merged shape.

```go
package v1 // config/v1/types_apiserver.go in openshift/api

type APIServerSpec struct {
	// ... existing fields: servingCerts, clientCA, additionalCORSAllowedOrigins,
	// encryption, tlsSecurityProfile, tlsAdherence, audit ...

	// podSecurityAdmission configures the Pod Security Standard the
	// kube-apiserver enforces, warns on and audits for Namespaces that
	// carry no pod-security.kubernetes.io label of their own.
	//
	// When omitted, this means the cluster has opted out of Pod Security
	// Admission. The PodSecurity admission plugin remains configured, with
	// all three of enforce, warn and audit set to privileged, so an
	// opted-out cluster neither rejects nor reports.
	//
	// +openshift:enable:FeatureGate=OpenShiftPodSecurityAdmission
	// +optional
	PodSecurityAdmission PodSecurityAdmissionConfig `json:"podSecurityAdmission,omitempty,omitzero"`
}

// APIServerStatus gains one subtree, declared below. It is empty today, so this
// enhancement is its first writer; the precedent is argued in "Why there is a
// status after all".

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
	// enforceLevel is the Pod Security Standard applied to Namespaces that
	// carry no pod-security.kubernetes.io label of their own. A Namespace
	// that carries one is governed by that label and is unaffected by this
	// field.
	//
	// It sets the warn and audit levels as well as the enforce level, so a
	// cluster reports violations at exactly the standard it applies and
	// never warns about one it does not. Setting Privileged therefore
	// silences PSA entirely rather than leaving it reporting: there is no
	// value that reports against a stricter standard than it enforces, and
	// no separate field to ask for one.
	//
	// No value starts the PSA label syncer: it is retired, and nothing
	// computes a per-Namespace label, so a Namespace that cannot meet the
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
	// as a Kubernetes minor version such as "v1.34", or "latest". It applies
	// to the enforce, warn and audit levels together.
	//
	// It has no observable effect while enforceLevel is Privileged, since
	// the privileged standard is identical in every revision. It is still
	// accepted and still reported in status, so that a pin survives a
	// period at Privileged and takes effect unchanged on the way back up.
	//
	// A version newer than the cluster's own Kubernetes version is clamped
	// down to it rather than rejected, so that one configuration can be
	// applied across a fleet at mixed releases. This field is left as
	// written; the value actually applied is reported in
	// status.podSecurityAdmission.enforceVersion.
	//
	// Setting this to "latest" asks for the newest standard the cluster
	// knows, which is a moving target: the Pod Security Standards gain
	// checks between Kubernetes releases, so "latest" means the cluster is
	// held to a stricter standard after every upgrade, and a workload that
	// is admitted today can be rejected after an upgrade that changed
	// nothing else. Prefer an explicit version unless tracking the newest
	// standard is what you want; "latest" is a deliberate choice to accept
	// tightening you have not reviewed, not a safe default.
	//
	// When omitted, this means the cluster's own Kubernetes version, which
	// has the same moving-target property as "latest". Pin this field to an
	// explicit version to hold the standard still.
	//
	// +kubebuilder:validation:Pattern=`^(latest|v1\.(0|[1-9][0-9]*))$`
	// +kubebuilder:validation:MaxLength=10
	// +optional
	EnforceVersion string `json:"enforceVersion,omitempty"`
}

type APIServerStatus struct {
	// podSecurityAdmission reports the Pod Security Admission configuration the
	// cluster resolved from spec.podSecurityAdmission. It is written by
	// cluster-kube-apiserver-operator and must not be set by anyone else.
	//
	// +optional
	PodSecurityAdmission *PodSecurityAdmissionStatus `json:"podSecurityAdmission,omitempty"`
}

// PodSecurityAdmissionStatus reports the resolved Pod Security Admission
// configuration: what the operator substituted into the kube-apiserver's
// admission configuration. It is not a statement about the cluster's
// readiness to meet that standard, and three things limit how it should be
// read.
//
// It is not a per-master statement. The operator resolves one value and
// writes it once; the kube-apiservers then roll one at a time, so during a
// rollout this reports the new configuration while some masters still run
// the old one. Rollout completion is read from currentRevision against
// latestAvailableRevision on kubeapiserver/cluster.
//
// It does not account for spec.unsupportedConfigOverrides, which is merged
// after the observed configuration and wins. A cluster carrying a PodSecurity
// override reports a level here that no kube-apiserver is enforcing; the
// UnsupportedConfigOverridesSet condition is the only signal of that.
//
// It says nothing about per-Namespace pod-security.kubernetes.io labels,
// which take precedence over this cluster-wide configuration. On a cluster
// where most Namespaces are labelled, this can be accurate and still
// describe the effective posture of very little.
type PodSecurityAdmissionStatus struct {
	// enforceLevel is the level substituted into the kube-apiserver's
	// PodSecurity admission configuration. With spec.enforceLevel unset or
	// Privileged this reads Privileged; it is otherwise an echo of spec, and
	// carries information only in that it confirms the spec was observed.
	//
	// +optional
	EnforceLevel PodSecurityStandard `json:"enforceLevel,omitempty"`

	// enforceVersion is the Pod Security Standards revision actually applied,
	// after clamping to the cluster's own Kubernetes version. It is always a
	// concrete version: where spec.enforceVersion is unset or "latest", this
	// reports the version those resolved to, so that the value is one an
	// administrator can compare against.
	//
	// It is the only way to detect a clamp through the API, but the two
	// fields differing does not by itself mean one occurred: spec "latest"
	// against status "v1.34" is an ordinary resolution. A clamp is spec
	// naming an explicit version higher than the one status reports.
	//
	// +kubebuilder:validation:MaxLength=10
	// +optional
	EnforceVersion string `json:"enforceVersion,omitempty"`
}

```

Three things the doc comments state but do not justify. `spec` is owned exclusively by the administrator and written by no controller, so `spec` stays pure intent and the operator writes only to `status`. The enum deliberately omits `""`: unlike `FeatureSet` it has a named value for its default, and adding `""` would give one state two spellings. And `status.enforceVersion` resolves `latest` to a concrete version rather than echoing the word, because an administrator comparing `spec` against `status` to detect a clamp cannot do so if one side says `latest`.

##### Setting the field

The whole user interface to this feature, for the workflow in [Workflow Description](#workflow-description) and for the break-glass path alike:

```bash
# Opt in. Baseline is the usual first destination: it blocks the known
# privilege escalations while permitting the default Pod configuration.
oc patch apiserver cluster --type=merge \
    -p '{"spec":{"podSecurityAdmission":{"enforceLevel":"Baseline"}}}'

# Read back what the operator resolved. A differing enforceVersion is a clamp;
# equal values mean the spec was observed, not that the rollout has finished.
oc get apiserver cluster -o jsonpath='{.spec.podSecurityAdmission}{"\n"}{.status.podSecurityAdmission}{"\n"}'

# Raise again once the cluster has been shown to meet the harder standard.
oc patch apiserver cluster --type=merge \
    -p '{"spec":{"podSecurityAdmission":{"enforceLevel":"Restricted"}}}'

# Break glass. Returns enforce, warn and audit to privileged together, and
# with them all PSA reporting; prefer stepping back to Baseline if that is
# enough to clear the failures.
oc patch apiserver cluster --type=merge \
    -p '{"spec":{"podSecurityAdmission":{"enforceLevel":"Privileged"}}}'
```

There is no `oc adm` verb, no console surface and no installer field, so these three invocations plus the dry-run sweep in [Finding the violating Namespaces](#finding-the-violating-namespaces) are the entire operator experience. That is why the documentation obligations in [Graduation Criteria](#tech-preview---ga) are load-bearing rather than supplementary, and why UX review is required.

#### Why there is a status after all

Adding a status to this resource is a precedent — `APIServerStatus` is empty today, so `cluster-kube-apiserver-operator` becomes its first writer and stops being a pure reader of `config.openshift.io`. It is worth the precedent for one reason: **`enforceVersion` is clamped, and a clamp with no report is a silent change to a security control.** A cluster at 1.34 given `v1.36` enforces 1.34's standards and, without a status, looks through the API exactly like one honouring the pin. The only other place the applied value appears is the `config-<revision>` ConfigMap in `openshift-kube-apiserver`, which is operator-owned, revision-suffixed and holds JSON inside a YAML key — a support artefact, not an answer an administrator can be asked to find.

`enforceLevel` in `status` is a near-pure echo of `spec` and carries little on its own. It is included because a status subtree carrying one field invites the next reader to ask what the level is and get no answer, and because the pair together let "did my change take effect?" be answered with one `oc get`.

The cost is not the field but the writer. It needs `update` and `patch` on `apiservers/status`, it makes a `config.openshift.io` resource partly operator-owned, and it adds rows to the downgrade matrix. Those are accepted because the clamp is the one piece of state that genuinely has nowhere else to live.

##### What `status` does not say

The three limits are in the `PodSecurityAdmissionStatus` doc comment above — not per-master, blind to `unsupportedConfigOverrides`, silent about per-Namespace labels — because each is a reading a reasonable administrator would otherwise make, and the comment is where they will look. Two obligations follow that the comment cannot carry: [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect) must keep naming all three as causes, and the support procedure remains the only place the `UnsupportedConfigOverridesSet` condition is connected to a PSA setting being inert.

"Absent" arises in three shapes and all resolve to `Privileged`: the field is not in the schema (release `n` without TechPreview); `apiserver/cluster` does not exist (MicroShift); the field is present and unset. A consumer that treats a field missing from the schema differently from one left unset has a bug.

#### Deciding whether it is safe to opt in

This is the question the `PodSecurityReadinessController` was built to answer, and from release `n+1` **nothing on the cluster answers it at all**. The dry-run in [Finding the violating Namespaces](#finding-the-violating-namespaces) is the whole of the replacement: point-in-time, run by hand, covering workloads that are not currently running but not ones that do not yet exist.

**This is the weakest point in the enhancement.** Because `warn` and `audit` follow `enforceLevel`, **an opted-out cluster evaluates nothing, counts nothing and cannot fire [`PodSecurityViolation`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/alerts/podsecurity-violations.yaml)**, so the signal that would tell an administrator whether opting in is safe exists only once they already have.

The trade is not obviously the right one. Pinning `warn` and `audit` at `restricted` would buy a free, continuous, fleet-wide readiness signal; it costs a warning on every `oc apply` of a Pod that would fail a standard the cluster is not applying, on every cluster, about a migration that never happened. The judgement is that a signal nobody asked for, attached to noise everybody sees, is worse than no signal plus a documented command, and that the population who will actually opt in is small and deliberate enough to run the sweep. **A reviewer who disagrees is disagreeing with the central trade of this section**; the two coherent reversals are [Keep both pinned at `restricted`](#keep-both-pinned-at-restricted) and [Split `warn` from `audit`](#split-warn-from-audit), and [Keep the readiness controller](#keep-the-readiness-controller) is a third that answers the question properly rather than as a side effect of a warning channel.

So there is no machine-readable cluster-level answer to "would this cluster survive `restricted`?", and no continuous one either. That is accepted because the evaluation existed to instrument a migration Red Hat would perform, and delegating the decision retires the migration along with the question.

**No answer to this question is a guarantee, and the documentation must not imply otherwise.** PSA evaluates at Pod creation, so any assessment is true only for the workloads that existed when it ran. A cluster that sweeps clean can reject a Pod minutes later because a CronJob fired, a Deployment scaled up, a node drained, or a Namespace was created — and that was equally true when the readiness controller existed, which reported a four-hourly snapshot of the same incomplete picture. What the platform owes an administrator considering an opt-in is therefore not assurance but **leverage**: the best available picture before the change, visibility of violations as they happen *once the change is made*, and a reversal that costs one field write. Those are the three things this enhancement is accountable for, and they are the shape of [Goal 3](#goals) and [Non-Goal 2](#non-goals). The middle one has narrowed — it used to hold before the change as well as after it. An administrator opting in is accepting a risk the platform has helped them size, not one it has removed.

#### Finding the violating Namespaces

With no evaluation on the cluster, this loop is **the** diagnostic rather than a way of getting detail behind a summary the API already provides:

```bash
LEVEL=baseline   # or restricted, when raising again
for ns in $(oc get ns -l '!pod-security.kubernetes.io/enforce' -o name); do
    oc label --dry-run=server --overwrite "$ns" \
        "pod-security.kubernetes.io/enforce=$LEVEL" 2>&1 \
        | grep -q 'violate' && echo "$ns"
done
```

**The level is a parameter and the sweep must be run at the one being asked for**, which is the respect in which it improves on what it replaces: the retired evaluation was pinned at `restricted` for fleet comparability, so it gave an administrator considering `Baseline` a list of Namespaces that were never going to matter to them. Running it at `baseline` first and `restricted` later is the staged path, and the two runs usually return very different lists — most clusters have a handful of `Baseline` violations and a great many `Restricted` ones. Drop the label selector to include Namespaces carrying their own `enforce` label, which the sweep could never see.

Three obligations follow, and they are heavier now that this is the only mechanism: the loop belongs in `openshift-docs` rather than left for an administrator to derive; must-gather must collect its output, or a support bundle contains nothing at all about PSA readiness; and because it is `O(Namespaces)` serial server round-trips, **whether a documented shell loop is an adequate deliverable at all is an [open question](#is-a-shell-loop-an-adequate-diagnostic)** on the clusters where the answer matters most.

#### `enforceVersion` and why the default moves

The field's semantics are in the doc comments above; what they do not carry is why the default is the way it is.

**Omitted and `latest` both track the payload's Kubernetes version, and that default is a moving target.** The standards gain checks between releases, so a cluster that does not pin is held to a slightly stricter standard after each upgrade and can acquire a violation from an upgrade alone. That sits uneasily beside this enhancement's thesis that no cluster should acquire failing workloads without opting in. It is accepted because it is what the cluster does today — the observer already substitutes `latest` — and because freezing each cluster at its install-time revision would strand the fleet on a standard that ages out of step with the product.

**`latest` is therefore the one setting under which an ordinary upgrade can start rejecting workloads that have changed in no way at all.** Every other path to a rejection in this document requires somebody to act: raising `enforceLevel`, labelling a Namespace, granting an SCC. This one delegates the decision to whatever the next Kubernetes release decides the restricted standard contains, on a cluster nobody has re-assessed. It is kept as a value, because a fleet deliberately tracking the newest standard is a legitimate posture — but **the documentation must present an explicit version as the recommended setting and `latest` as the opt-in to continuous tightening**, which is the reverse of how the word reads.

Two consequences for the documentation rather than the API. **Leaving the field unset has the same property and is the likelier way to acquire it**, so the opt-in procedure must say that opting in without pinning is opting in to the level *and* its future revisions. And it **interacts badly with the loss of the pre-flight signal**: nothing now warns about newly-added checks before the upgrade that enforces them, so an administrator tracking `latest` has to re-run the sweep after each upgrade.

**A version the cluster cannot evaluate is clamped, not rejected.** Validation accepts any well-formed `v1.X`, including one ahead of the cluster's own Kubernetes version — an administrator pinning ahead of a planned upgrade, or a GitOps repository applied across a fleet at mixed releases. The operator clamps to the payload's version when substituting and leaves the field alone, so a pin survives a downgrade and is restored on the way back up; rejecting the write instead would make a legitimate pin fail on whichever cluster happened to be oldest. Why clamping is a safety requirement rather than a convenience is in [PodSecurity Configuration](#podsecurity-configuration).

**The clamp is visible in `status`, and that is why the status exists.** A cluster at 1.34 given `v1.36` reports `spec.enforceVersion: v1.36` and `status.enforceVersion: v1.34`, and that difference is the detection mechanism — **with one qualification `latest` introduces**: `spec: latest` against `status: v1.34` also differs and is not a clamp, so the rule is "`spec` names an explicit version higher than `status`", not "the two differ". Anything scripting clamp detection in GitOps drift checks has to encode the narrower rule, or it reports a clamp on every cluster tracking `latest`. Subject to that, `oc get apiserver cluster -o jsonpath` over both fields answers "am I enforcing what I pinned?" without reading an operator-owned ConfigMap. This was the open question that [Why there is a status after all](#why-there-is-a-status-after-all) resolves, and it is the single thing that justifies the precedent. Two residual gaps remain and are not closed by the status: it does not account for `unsupportedConfigOverrides`, and it is not a per-master statement; both are in [What `status` does not say](#what-status-does-not-say).

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is affected structurally. The following are verified against the `openshift/hypershift` source:

- **There is no `cluster-kube-apiserver-operator` in a hosted control plane.** The control-plane-operator renders the kube-apiserver's PodSecurity configuration directly (`.../v2/kas/config.go`), keyed off whether the literal string `OpenShiftPodSecurityAdmission=true` appears in the rendered feature gate list. The Config Observer mechanism this enhancement builds on has a second, independent implementation that has to change in step with it — on the `=true` side only, since the else branch already renders `privileged` and is left alone. Because the gate is redefined rather than added, a *string match against a gate list from a different release* is the construct the redefinition would break; `featureGate-Hypershift-Default.yaml` shows the gate `disabled` on every release branch, so supported hosted clusters are all on the unedited side and the exposure is confined to preview ones. See [HyperShift skew](#hypershift-skew).
- **The PSA label syncer runs as a management-cluster Deployment**, reconciled as a control-plane component (`.../v2/clusterpolicy/component.go`); the hosted-cluster-config-operator reconciles the *guest-cluster* RBAC it needs. The syncer is not retired unconditionally — it is retired when the gate is on — so HCP's change is a conditional, carrying the same version gate as the configuration above, rather than a straight removal. The guest-cluster RBAC must therefore stay: it is needed on every hosted cluster below `n`, and on any hosted cluster that is downgraded. Removing it is a later cleanup tied to the same sunset, not part of this work.
- **The `PodSecurityViolation` alert already ships in HCP**, embedded in the HCCO resources — a copy of the standalone asset that has already drifted from its source. Any change has to be made twice; this enhancement retains the rule unchanged in both, but its *meaning* changes on HCP exactly as it does on standalone — it cannot fire on a hosted cluster that has not opted in, because `control-plane-operator` renders `warn` and `audit` from the same `enforceLevel`. The description and runbook changes required in [Alerts and metrics](#alerts-and-metrics) therefore have to be made in the HCCO copy too, and the drift between the two copies makes that easy to miss.
- HCCO opts `kube-system` out of label syncing in the guest cluster, which becomes inert. The management side additionally carries a `PodSecurityAdmissionLabelOverrideAnnotation` and a `restricted-psa` image label in `hostedcluster_controller.go`; both interact with the enforcement level and need reconciling with the new API.

**No status is reported on hosted clusters**, and this is the one respect in which the feature is less capable on HyperShift than on standalone. `ClusterConfiguration` carries *specs only* — `HostedCluster.spec.configuration.apiServer` is a `*configv1.APIServerSpec` and there is **no `HostedCluster.status.configuration`** for anything to write back into. The spec reaches a hosted cluster through machinery that already exists; the status has nowhere to go that is both reachable by its writer and visible to its reader. `status.podSecurityAdmission` is therefore simply absent on a hosted cluster, and `control-plane-operator` writes nothing.

Two alternatives were considered and rejected as disproportionate to one subtree of two scalar fields. Writing into the **guest cluster's** `apiserver/cluster` puts the answer where a hosted-cluster administrator would look, but the value is resolved on the management side, so it needs HCCO or equivalent plumbing to carry it across the boundary — new cross-cluster machinery for a report. Adding a status surface to **`HostedCluster`** is cleaner for the management side but is a new API surface on HyperShift's primary type, and it puts the answer where the hosted-cluster administrator cannot see it at all.

**What this costs is the clamp, and the cost is real.** `control-plane-operator` clamps `enforceVersion` against the *hosted* cluster's Kubernetes version, so clamping happens on HyperShift exactly as it does on standalone — and there it remains undetectable through the API, recoverable only from the rendered admission configuration. Since detecting the clamp is the entire justification for the status ([Why there is a status after all](#why-there-is-a-status-after-all)), **the justification does not hold on HyperShift**, and the honest reading is that hosted clusters keep the drawback that standalone has just shed. Three obligations follow:

- **Absence must be documented as "not reported on this platform", not read as "not configured".** A consumer that treats a missing `status.podSecurityAdmission` as meaning the feature is off will be wrong on every hosted cluster.
- **The clamp must stay diagnosable another way on HyperShift**, which means the support procedure in [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect) needs a hosted-cluster branch pointing at the rendered configuration.
- **Revisit if HyperShift gains a configuration status surface.** Writing into it would then be additive and cheap, and this decision should not outlive the constraint that produced it.

Behaviour under management/guest skew ([Version Skew Strategy](#version-skew-strategy)) remains open on top of this, and **a HyperShift reviewer is still required** — but on the status question they are now confirming a decision rather than making one.

#### Standalone Clusters

Standalone is the primary topology; everything outside this section is written against it. Disconnected and bare-metal clusters are unaffected: no component involved reaches outside the cluster, and the only egress-adjacent surface — the `pod_security_enforcement_mode` telemetry metric — degrades to not being reported like any other.

#### Single-node Deployments or MicroShift

**Single Node OpenShift.** Every change to the effective level re-renders the kube-apiserver configuration. With one kube-apiserver that rollout is a short API outage rather than a rolling update — including the rollout that *disables* enforcement, which is precisely the path an administrator reaches for when something is already wrong, so the cost has to be stated in the break-glass procedure. The observer's only inputs are two spec fields and a feature gate, none of which varies with cluster activity, so there is no flapping input that could cut revisions in a loop.

SNO is also where retiring the readiness controller pays most directly: the four-hourly sweep — one Namespace LIST plus a per-Namespace Pod LIST and the workload-template LISTs, through a client throttled to QPS=2 / Burst=2 — was this enhancement's only new recurring resource cost, and the one whose peak memory and CPU a single-node cluster could least afford. It goes, and with it the open question about bounding it. What an SNO administrator loses is the same thing everyone loses: the sweep was also the only PSA diagnostic that did not require them to run something.

**MicroShift** is affected and cannot be waved off, and it is the only topology this enhancement changes the posture of. It hardcodes `enforce: restricted` in `assets/controllers/kube-apiserver/defaultconfig.yaml`; it vendors and runs `cluster-policy-controller` with `"*"` controllers and **no feature gates at all**, so it reaches the syncer through the `default:` arm of the switch — `NewEnforcingPodSecurityAdmissionLabelSynchronizationController` — rather than through either gate value; it ships the syncer's RBAC and documents the behaviour in `docs/user/howto_pod_security.md`; and it has no CVO, no `FeatureGate` CR and no installer-rendered `APIServer` singleton, so none of this enhancement's configuration surface reaches it. Moving the API onto an existing config resource does not change this: MicroShift serves no such resource.

##### The absent arm changes, and MicroShift has no gate to stage it behind

The switch in `runPodSecurityAdmissionLabelSynchronizationController` has three arms — gate explicitly `false`, gate explicitly `true`, and gate absent — and **two of them change**. `=true` stops starting the syncer, which is the standalone opt-in path. The absent arm stops starting it too, which is MicroShift's. Only `=false` is left alone, and that is the arm every supported standalone cluster is on, so the property in [The gate-off branch must not be touched](#the-gate-off-branch-must-not-be-touched) still holds in full: nothing a standalone `Default` cluster runs is edited.

**MicroShift's change cannot be staged behind the feature gate, because MicroShift has no feature gates.** It reaches the syncer through the `default:` arm precisely because it never sets `OpenShiftPodSecurityAdmission` either way, so the change lands for MicroShift the moment it vendors a `cluster-policy-controller` containing it — not when a gate moves into `Default`. Standalone and MicroShift are therefore on independent schedules by construction, and **the sequencing must be chosen rather than inherited**:

- Editing the absent arm in release `n` stops the syncer on MicroShift a release before the manifest changes below are expected to land, and before the [release note](#release-note) that explains it.
- Editing it in `n+1` aligns MicroShift with standalone, and is what this enhancement assumes. It means release `n`'s `cluster-policy-controller` changes only the `=true` arm, and the absent arm is edited in the release where the gate enters `Default`.

**This is a hard coordination point with MicroShift**, and the ordering within it matters more than the release. The syncer must not stop before MicroShift's global default moves to `privileged`: a vendoring bump that arrives ahead of the manifest change leaves customer Namespaces unlabelled under a hardcoded `restricted`, which is the one genuinely breaking configuration in this whole design and is reachable purely by getting the order wrong. A MicroShift reviewer must confirm that the vendoring and the manifest changes land in the same release, and the absent arm needs a comment naming it and a test of its own, asserting that the syncer does **not** start when the gate is reported as neither set nor unset.

Beyond the mechanics, the question for MicroShift is what replaces the syncer's computed labels once it stops running, given that an unlabelled Namespace there is currently held to `restricted`.

**MicroShift moves its global default to `privileged` and labels its own Namespaces explicitly.** Customer-created Namespaces then default to `privileged`, exactly as on standalone, so **nothing a MicroShift user is running can break**; MicroShift's own components keep the posture they have today, carried by per-Namespace labels in their own manifests rather than by a cluster-wide default they share with customer workloads. This is the arrangement that makes the syncer removable on every product without a breaking change anywhere, and it is why the SCC-mapping keys can be deprecated outright — see [Deprecating the SCC-mapping keys](#deprecating-the-scc-mapping-keys).

Three changes deliver it, and the second has been enumerated rather than estimated:

1. **`enforce: restricted` becomes `enforce: privileged`** in `assets/controllers/kube-apiserver/defaultconfig.yaml`. **What `warn` and `audit` become is a MicroShift decision and this enhancement does not take it.** On standalone they now follow `enforceLevel`, so an opted-out cluster is silent; matching that on MicroShift means setting all three to `privileged` and leaving MicroShift with no PSA reporting at all. Leaving them pinned at `restricted` keeps the signal and keeps the warnings. The argument differs from standalone's in one respect that should decide it: **a MicroShift administrator has no API to turn reporting back on**, since none of this enhancement's configuration surface reaches the product, so silence there is permanent rather than one field write away. That asymmetry favours leaving `warn` and `audit` at `restricted` on MicroShift even though standalone drops them — at the cost of the two products no longer behaving the same way, which has to be documented rather than left for a user to discover.
2. **Eight shipped Namespace manifests gain an explicit `enforce` label.** Thirteen already carry one and are unaffected by the default moving, including `openshift-operators` at `privileged` and `openshift-operator-lifecycle-manager` at `restricted`. The eight without are, in core paths, `openshift-controller-manager`, `openshift-route-controller-manager` and `openshift-service-ca`; and in `assets/optional/`, `cert-manager`, `kserve`, `redhat-ods-applications`, `openshift-gateway-api` and `openshift-observability`. A ninth match, `system`, is a kustomize placeholder in cert-manager's upstream manifests rather than a Namespace MicroShift creates, and should be confirmed as such rather than labelled.
3. **The syncer stops running**, through the absent arm, as above.

**The five `openshift-`-prefixed Namespaces in that list are the easy half.** `isNSControlled` skips that prefix, so the syncer never labelled them and they are already running under the hardcoded `restricted`. Pinning them to `restricted` records what is already true and changes nothing — which is the strongest form this argument can take, and stronger than observing that they happen to be unlabelled.

**The remaining three need checking rather than assuming.** `cert-manager`, `kserve` and `redhat-ods-applications` are not `openshift-` prefixed, so they *are* syncer-controlled today and carry a *computed* label that may be looser than `restricted`. Their workloads look restricted-compatible — `runAsNonRoot`, `seccompProfile: RuntimeDefault`, `drop: ALL`, `allowPrivilegeEscalation: false` — but each must be pinned to the level it actually requires, confirmed by dry-run before the change rather than set to `restricted` on the strength of reading the manifests. They ship only with optional components, so a default install does not exercise them.

The residual cost is a posture reduction for MicroShift users, and it should be stated plainly rather than hidden behind "nothing breaks". A customer Namespace that today receives a computed minimally-sufficient label will tomorrow receive `privileged`, which is looser than it needed. That is the same trade standalone makes, with one difference worth noting: on standalone the default was already `privileged`, so nothing moves, whereas **MicroShift genuinely relaxes**. It relaxes rather than tightens, so no workload fails, and violations remain visible **if** MicroShift keeps `warn` and `audit` at `restricted` per change 1 — a MicroShift user relying on restricted-by-default as a security control loses the control either way, and loses the ability to see it coming as well if those keys move too. The release note must say so. **A MicroShift reviewer must accept that trade**, and owns changes 1 and 2.

Single-gating makes the default outcome the safe one: because the syncer's code has to survive for the gate-off path anyway, keeping it for MicroShift costs nothing extra, where under a design that deleted the code it would have cost a fork. A MicroShift reviewer is still required — the first and third outcomes are theirs to choose — but the enhancement no longer blocks on the answer, because the second is what happens if nobody decides.

#### OpenShift Kubernetes Engine

OKE behaves as Standalone above; the API and the enforcement path are identical. The one difference is that cluster monitoring is excluded from the OKE offering, so the `PodSecurityViolation` alert and the metric it reads are unavailable and the dry-run sweep is the only diagnostic an OKE administrator has — **before and after an opt-in alike**, where a standalone administrator at least regains the alert once they opt in. **Whether opting in is supportable on that basis needs an answer before GA**, and it is a narrower question than it was: the alert was already unavailable here, so what OKE loses from the `warn`/`audit` change is only the API warnings, which were the one PSA signal it did have.

### Implementation Details/Notes/Constraints

The changes, by component:

| Component | Change |
|---|---|
| `openshift/api` | `podSecurityAdmission` on `APIServerSpec` in `config/v1/types_apiserver.go`, gated on `OpenShiftPodSecurityAdmission`; that gate removed from `inDefault()` and `inOKD()` in `features.go`, retained in `inTechPreviewNoUpgrade()` and `inDevPreviewNoUpgrade()` |
| `cluster-kube-apiserver-operator` — Config Observer | change **only the gate-on branch** of `observePodSecurityAdmissionEnforcement`: **all three of `enforce`, `warn` and `audit`** from `spec.enforceLevel` defaulting to `privileged` — dropping the historical `restricted` pinning of `warn` and `audit` on this branch only — and the three `*-version` keys from `spec.enforceVersion` clamped to the payload's Kubernetes version; add a `Baseline` case. **The gate-off branch is left exactly as it is** — it already produces `privileged`; see [The gate-off branch must not be touched](#the-gate-off-branch-must-not-be-touched) |
| `cluster-kube-apiserver-operator` — `PodSecurityReadinessController` | start it only when the gate is **off**; when on, do not start it and remove its six ClusterOperator conditions if present |
| `cluster-policy-controller` | in `runPodSecurityAdmissionLabelSynchronizationController`, change the **`=true` arm** and the **absent/`default:` arm** — both stop starting the syncer. The `=false` arm stays advising, which is every supported standalone cluster's path. The absent arm is MicroShift's and is edited in `n+1`, not `n`; see [The absent arm changes](#the-absent-arm-changes-and-microshift-has-no-gate-to-stage-it-behind) |
| `hypershift` — `control-plane-operator` | change only the `=true` side of the `kas/config.go` PodSecurity branch to match, and version-gate it per [Gate redefinition skew](#gate-redefinition-skew) |

**One new RBAC grant is needed.** The operator reads `apiserver/cluster` today and now also writes its status, so it gains `update` and `patch` on `apiservers/status`. Nothing else changes: no new watch, no new resource, and no grant on `apiservers` itself, because `spec` stays administrator-owned.

Neither retired controller is wired to the new API: `cluster-policy-controller` gains no watch, no client and no dependency on `apiserver/cluster`, and the readiness controller does not become conditional on `enforceLevel`. Both are conditional on the gate alone.

#### Already shipped: SCC provenance annotations

Two annotations defined in [`openshift/api`](https://github.com/openshift/api/blob/de86ee3bf48122ecb00fde7287aa633642ddc215/security/v1/consts.go#L9-L20) already exist and are **not new work**. Neither has a consumer after this enhancement: the readiness controller read both, and it is retired.

**`security.openshift.io/validated-scc-subject-type`** records whether a Pod's SCC was granted through its ServiceAccount or through the user's own roles — the distinction that explains one of the two root causes behind most violating Namespaces found by ClusterFleetEvaluation. It is set at admission by the [SCC admission plugin](https://github.com/openshift/apiserver-library-go/blob/42e5e402ca430b5072f9d6fd5df275aff9eaf5ce/pkg/securitycontextconstraints/sccadmission/admission.go#L400-L403) and stays shipped and maintained; it is simply no longer read by anything, since the controller that classified violations by it is gone. It remains useful to a human debugging a rejection. **The open question about its coverage on upgraded clusters is closed by this change**, not answered: nothing now has to decide what an absent value means.

**`security.openshift.io/MinimallySufficientPodSecurityStandard`** is written by the syncer at [`#L375-L380`](https://github.com/openshift/cluster-policy-controller/blob/c9e9a348260921c9e788e33a51e904502cbe2d13/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L375-L380) and does not survive. See [Retiring the PSA label syncer](#retiring-the-psa-label-syncer).

#### Role of the `OpenShiftPodSecurityAdmission` feature gate

This enhancement **redefines an existing gate rather than adding one**, and that is the single most consequential decision in the document. It is what lets all three changes land atomically, and it is also the only place where an older component can misread a newer cluster — bounded, and bounded to HyperShift, in [Gate redefinition skew](#gate-redefinition-skew).

Today the gate *means* "PSA enforcement is on by default", and both of its branches exist in code:

| | gate enabled (today) | gate disabled (today) |
|---|---|---|
| Config Observer | `enforce: restricted` | `enforce: privileged` |
| PSA label syncer | enforcing | advising |
| `PodSecurityReadinessController` | running | running |

The observer's two branches are [`podsecurityadmission.go#L105-L113`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/auth/podsecurityadmission.go); the syncer's are [`psalabelsyncer.go`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/cmd/controller/psalabelsyncer.go).

##### What actually ships

**Every customer cluster is on the *disabled* side of that table, and always has been.** The gate is in `inDefault()` and `inOKD()` on `master` ([`features.go`](https://github.com/openshift/api/blob/master/features/features.go)), but **`master` is not what ships**, and reading the gate's enablement there is the single easiest way to get this design wrong. On release branches it is enabled only in the two preview feature sets:

| Release branch | `OpenShiftPodSecurityAdmission` |
|---|---|
| `release-4.11` | gate does not exist |
| `release-4.12`, `release-4.13` | `TechPreviewNoUpgrade` only |
| `release-4.14` | defined, **added to no feature set at all** |
| `release-4.15` … `release-4.21` | `enableIn(DevPreviewNoUpgrade, TechPreviewNoUpgrade)` |
| `master` | `enable(inDefault(), inOKD(), inTechPreviewNoUpgrade(), inDevPreviewNoUpgrade())` |

The generated payload manifests agree, and they are the artefact the cluster actually reads: `payload-manifests/featuregates/featureGate-SelfManagedHA-Default.yaml` and `featureGate-Hypershift-Default.yaml` list `OpenShiftPodSecurityAdmission` under **`disabled`** on every release branch that has them. **PSA enforcement has therefore never been on by default in a shipped OpenShift release**, and the fleet has never carried syncer-written `enforce` labels from a `Default` cluster.

Three consequences, and the rest of this document depends on all three.

- **The shipped global default is already `privileged`.** `!featureGates.Enabled(...)` selects `SetPodSecurityAdmissionToEnforcePrivileged`, which writes `enforce: privileged` with `audit` and `warn` pinned to `restricted`. The level this enhancement makes configurable is the level customers are already running.
- **The syncer customers run is the *advising* one**, not the enforcing one. library-go's feature-gate observer is constructed with a nil whitelist for `cluster-policy-controller` ([`observe_config_controller.go#L129-L134`](https://github.com/openshift/cluster-kube-controller-manager-operator/blob/master/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go)), which sets `allowAll` ([`observe_featuregates.go#L22`](https://github.com/openshift/library-go/blob/master/pkg/operator/configobserver/featuregates/observe_featuregates.go)), so **every** known gate is emitted — disabled ones as the literal `Name=false`. `cluster-policy-controller` therefore receives `OpenShiftPodSecurityAdmission=false` and takes the first arm of the switch. Advising mode's label set is `loggingLabels` — `warn` and `audit` only, **no `enforce`** ([`podsecurity_label_sync_controller.go#L43-L51`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go)).
- **The readiness controller runs unconditionally**, on both sides, started at [`starter.go#L508`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/starter.go) with no gate check. Its retirement is the one change in this enhancement that lands on every customer cluster regardless of anything else.

MicroShift is the exception that proves the rule: it passes no feature gates to `cluster-policy-controller` at all, so it reaches the `default:` arm and runs the **enforcing** syncer. It is the only shipped configuration that does. See [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift).

##### The standing branch-cut hazard

`master` says `inDefault()`; every release branch since 4.12 says otherwise. **Something removes the gate from `Default` at each branch cut, and it has had to be right nine times running.** Establishing what that something is — an automated step, a documented checklist item, or institutional memory — is a prerequisite for this enhancement rather than an aside, because the failure mode is that one branch cut forgets and the fleet receives mandatory `restricted` enforcement with no API to turn it off. That is the same outcome as the design defect in [The gate-off branch must not be touched](#the-gate-off-branch-must-not-be-touched), reached by a different route, and it is live today independent of anything proposed here.

Release `n` removes the gate from `inDefault()` and `inOKD()` on `master`, which **retires the hazard as a side effect**: once `master` and the release branches agree, there is nothing left for a branch cut to get wrong. That is a genuine benefit of this enhancement and is claimed as one, but it should not be the reason the hazard goes uninvestigated in the meantime — releases will be cut before this work lands.

After this enhancement the gate means *the new PSA configuration generation is active*:

| | gate enabled (after) | gate disabled (after) |
|---|---|---|
| Config Observer | `enforce` from `spec.podSecurityAdmission`, unset → `privileged` | `enforce: privileged` — **unchanged** |
| PSA label syncer | **not running** | advising — **unchanged** |
| `PodSecurityReadinessController` | **not running** | running — **unchanged** |
| `spec.podSecurityAdmission` in schema | present | absent |

##### The gate-off branch must not be touched

**This is the most important consequence of [What actually ships](#what-actually-ships), and the easiest thing in this enhancement to get catastrophically wrong.**

The intuitive way to redefine a gate is to *invert* it — make gate-off produce `enforce: restricted` with an enforcing syncer, so that gate-off reproduces what the enabled side meant before. **That would enforce `restricted` on every cluster in the fleet.** A `Default` cluster is gate-off; redefining what gate-off means leaves it gate-off and now hands it `restricted` and an enforcing syncer. The upgrade `n-1` → `n`, which must be invisible to `Default` clusters, would instead be a fleet-wide forced migration to `restricted` — precisely the event this enhancement exists to prevent, delivered by the enhancement itself, one release before the API that could switch it off.

The reasoning that leads there is seductive because it is almost right: it assumes `Default` clusters are gate-*on*, which is what `master` says. They are not, and never have been. **Anyone re-deriving this design from `features.go` on `master` will reach the inverted version.** That is why the evidence is set out above and why the regression test below is mandatory.

The design is therefore **not an inversion**. The gate-off branch already produces exactly the end state the gate-off branch should produce, so it is left alone, and only the gate-on branch changes:

| Component | Gate-off branch | Gate-on branch |
|---|---|---|
| Config Observer | **untouched** — `enforce: privileged`, `warn`/`audit` pinned at `restricted` | all three levels from `spec.enforceLevel`, unset → `privileged` throughout; add a `Baseline` case |
| `cluster-policy-controller` switch, `=false` arm | **untouched** — already advising | — |
| `cluster-policy-controller` switch, `=true` arm | — | enforcing → **do not start the syncer** |
| `cluster-policy-controller` switch, absent arm | not a gate branch — MicroShift's path, edited in `n+1` | enforcing → **do not start the syncer** |
| `PodSecurityReadinessController` | **untouched** — keeps running | add a gate check: do not start, and clear the six conditions |
| `hypershift` `kas/config.go` | **untouched** — already the else branch | all three levels from the spec, unset → `privileged` throughout |

Three things follow.

- **The `=false` arm of the syncer switch is the one that survives, and it is the one every supported standalone cluster is on.** The other two both stop starting the syncer, so the switch collapses from three distinct behaviours to two — but *not* into a single "not enabled" branch, because `=false` must keep constructing the advising controller while the gate-off path exists. The refactoring hazard is now the mirror image of the usual one: a tidy-up that merges all three arms removes the advising syncer from release `n` and changes the path real clusters run. It needs a comment and a test.
- **The advising constructor keeps its caller.** `NewAdvisingPodSecurityAdmissionLabelSynchronizationController` stays reachable from the `=false` arm, so it is **not** deleted, and the non-deterministic server-side-apply decay described in [Why it is retired outright](#why-it-is-retired-outright) stays in the tree for as long as the gate-off branch does. That is a real cost of this shape and it should be seen: an arrangement that retired the gate could delete that code, and this one cannot.
- **The gate-off branch is byte-for-byte today's behaviour by construction, not by assertion.** It is true because nothing is written, not because a test says so — though the test is still required, to catch the slip rather than to prove the design.

What remains to be got right is HyperShift's copy, which is an exact string match rather than a gate lookup — `slices.Contains(p.FeatureGates, "OpenShiftPodSecurityAdmission=true")` ([`kas/config.go#L162`](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go)) — so anything that is not that literal string, including a hosted cluster that simply never emitted it, lands in the else branch. The else branch is now untouched, so the residual skew is confined to the `=true` branch and to hosted clusters running a preview feature set; see [Gate redefinition skew](#gate-redefinition-skew).

##### Why one gate rather than two

Reusing the gate is safe because **no supported cluster ever observes the redefinition**. Gate membership is compiled into the payload, the gate is out of `Default` throughout release `n` ([What actually ships](#what-actually-ships)), and the branch a `Default` cluster is on is not edited at all ([The gate-off branch must not be touched](#the-gate-off-branch-must-not-be-touched)). The new meaning exists only in the two preview feature sets, whose clusters cannot upgrade out of it.

**The decisive argument is the one about edits, not the one about skew.** Reusing the gate is the only arrangement under which the code path every supported cluster runs is not modified at all. Every alternative that retires `OpenShiftPodSecurityAdmission` instead of redefining it has to collapse the observer's two branches into one and decide what `cluster-policy-controller`'s switch does with no gate to read — edits to exactly the path where a mistake is both likeliest and least visible, for no posture benefit. After that, the skew comparison is close to a tie: the residual redefinition exposure is confined to preview clusters, and the HyperShift version gate that would otherwise be its main cost is optional. See [Alternatives](#shape-of-the-feature-gating).

**What the gate is really staging is the two retirements, not the field.** The field needs a gate by convention, to graduate from Tech Preview; it is inert until an administrator writes to it and harmless if it ships early. The retirements are the part that changes behaviour on every cluster — six withdrawn ClusterOperator conditions and an unmaintained set of `warn`/`audit` labels — and they are the reason a dark release is wanted at all. An arrangement that gated the field but shipped the retirements ungated would have the staging exactly backwards.

**The three changes are not strictly coupled.** The coupling argument in [Why it is retired outright](#why-it-is-retired-outright) — that the syncer contradicts an administrator asking for `Restricted` by computing a *lower* standard — is an argument about the **enforcing** syncer. The advising syncer customers actually run writes `warn` and `audit` only and no `enforce`, so it cannot grant a lower standard and does not contradict an opt-in. What survives is weaker and worth stating honestly: a Namespace carrying an advising-written `warn: baseline` while enforcing `restricted` under-warns relative to what it enforces, which is a visibility defect rather than a correctness one. Single-gating is therefore chosen for payload consistency and for the edit-minimisation above, **not** because the changes cannot be separated.

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

**A violating Namespace keeps running what it already has, and cannot replace it.** This is the single most useful sentence to give an administrator about what an opt-in does, and it is worth stating before the detail. Existing Pods keep running, untouched and un-re-evaluated; nothing is evicted and nothing restarts. But the first time any of those Pods has to be *recreated*, the replacement is rejected and does not come back. The Namespace therefore degrades rather than fails: it looks entirely healthy until some ordinary cluster operation recreates a Pod, and then loses capacity one Pod at a time, with no single event an administrator would think to look at.

What counts as recreation is the ordinary business of running a cluster: a node drain or reboot, a MachineConfig rollout, an eviction under node pressure or preemption, a scale-up, a rolling update, a `CronJob` firing, a `StatefulSet` replacing a member. **A container crash-looping inside an existing Pod is not one of them** — the kubelet restarts the container and the Pod object survives, so no admission decision is taken and the workload keeps retrying unaffected. The distinction matters because the two look alike in `oc get pods` and only one of them is a PSA problem.

- **A workload admitted today can be rejected weeks later**, by any of the above — a routine MachineConfig rollout is enough. The gap between cause and symptom is the real hazard: nothing connects the rejection to an opt-in from a month earlier, and the administrator debugging it is often not the one who made the change. The dry-run loop catches the Deployment-scaled-to-zero and CronJob-not-yet-fired cases, but only when run.
- **A Namespace created after the opt-in is enforced immediately**, with nothing labelling it and nothing reporting it. This is upstream Kubernetes' behaviour, but it is not what OpenShift administrators have experienced, and it is the sharpest residual risk of opting in.

Payload Namespaces are exempt from the second — they carry labels from their own CVO manifests, verified in CI — with OLM the known exception, handled under [OLM Namespaces carry no label](#olm-namespaces-carry-no-label). Both cases are *observable* as they happen, because a cluster that has opted in has `warn` and `audit` at its own enforcement level, so the rejection and the signal describe the same standard. That is the whole of the mitigation, and it holds only for a cluster that has opted in — the same risks on an opted-out cluster produce no signal, but also no rejection.

##### Discovering failures after an opt-in

An administrator who raises the level and *then* finds workloads failing is the subject of a user story, and it is a different problem from deciding whether to opt in: the question is no longer "would this break?" but "is this breakage mine?". Nothing on the cluster connects a rejected Pod to the `spec.podSecurityAdmission` write that caused it, so the three signals below are the whole of the answer and the documentation has to name all three.

- **The rejection does not appear where it is looked for.** Per [In general](#in-general), a Pod rejected at admission is never created, so a controller-owned workload shows nothing in `oc get pods`; the error lands on the ReplicaSet, Job or StatefulSet, and `oc -n $NAMESPACE get events --field-selector reason=FailedCreate` is the first thing to reach for. This is the single most common source of confusion when debugging PSA and it is sharper immediately after an opt-in, when the administrator is looking for an effect they expect to see.
- **The `PodSecurityViolation` alert continues to fire after the opt-in, not only before it.** This rests on a property of the admission plugin that the rest of this document assumes without stating: PSA evaluates `enforce`, `audit` and `warn` **independently for every admission request**, so a Pod denied under `enforce: restricted` is also counted in the `audit` stream as `pod_security_evaluations_total{decision="deny",mode="audit"}`. The alert reads that stream, so it does not fall silent at exactly the moment enforcement starts rejecting things. **This must be confirmed against the upstream plugin during implementation and asserted by a test**, because if an enforce-mode denial short-circuits the audit evaluation then the only proactive PSA signal the product ships stops working the moment it is most needed — and it would fail silently, looking like a clean cluster.
- **The alert now means what it says, and only fires on clusters that opted in.** Because `audit` tracks `enforceLevel`, a firing `PodSecurityViolation` on a cluster enforcing `Baseline` counts workloads that genuinely failed `baseline`, not ones that merely would have failed `restricted` — the over-report that the old pinning produced is gone, and support can read the alert as evidence of real rejections. The price is at the other end: on an opted-out cluster `audit` is `privileged`, nothing is ever counted, and the alert is structurally incapable of firing. **Its absence on such a cluster carries no information whatsoever**, which the runbook and the alert's own description have to say, because "no PSA alerts" reads as "no PSA problems".

The remedy is the revert path, and it is deliberately short: one field write to `Baseline` or `Privileged`, with no component able to hold it up ([PodSecurity Configuration](#podsecurity-configuration)) and the rollout cost stated in [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift). Narrowing the revert to `Baseline` rather than going all the way to `Privileged` needs the dry-run re-run at `=baseline`, per [Finding the violating Namespaces](#finding-the-violating-namespaces), since nothing reports which level the cluster could actually meet.

##### Alerts and metrics

**No metric and no alert ship with this enhancement**, and the four metrics plus the `PodSecurityReadinessEvaluationStale` alert that were derived from the sweep go with it.

The existing [`PodSecurityViolation`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/alerts/podsecurity-violations.yaml) alert is **retained unchanged in its rule**, but its meaning and its population both change, and it is the one existing asset this enhancement most alters without editing.

It fires on `pod_security_evaluations_total{decision="deny",mode="audit"}`. Because `audit` now tracks `enforceLevel`:

- **On an opted-out cluster it can never fire.** `audit: privileged` denies nothing, the counter never increments, and the alert is structurally silent. That is every cluster in the fleet on the day `n+1` ships, so in practice the alert goes from fleet-wide to firing only where somebody has opted in.
- **On an opted-in cluster it reports real rejections** at the level actually enforced, rather than hypothetical ones against `restricted`. That is a genuine improvement in precision and the reason the rule needs no edit.

**Three things follow and none is optional.** The alert's description and runbook must say that silence on an opted-out cluster means nothing was evaluated, not that nothing is wrong — otherwise the alert's absence will be read as assurance. **Its thresholds and routing should be reviewed**, since it was tuned as a secondary signal alongside the withdrawn conditions and is now both the only proactive signal and a much rarer one. And the degradation from stale per-Namespace labels narrows rather than disappears: a frozen `audit` label looser than the cluster-wide level still hides denials, but only on clusters that opted in, since there are none to hide anywhere else. See [Labels on upgraded clusters](#labels-on-upgraded-clusters).

**The noise and the signal are separable, and the decision not to separate them is deliberate.** `warn` and `audit` are independent keys with very different costs: `warn` returns an API warning that `oc` prints to the user's terminal on every apply, which is the noise customers complain about, while `audit` only annotates the audit-log event and increments the metric, which nobody sees but the alert reads. Keeping `audit: restricted` while letting `warn` follow `enforceLevel` would therefore have ended the visible noise *and* kept the fleet-wide readiness signal. It is rejected in [Split `warn` from `audit`](#split-warn-from-audit), and a reviewer who wants the signal back should go there first — it is a cheaper reversal than restoring the full pinning.

#### PodSecurity Configuration

A Config Observer in `cluster-kube-apiserver-operator` manages the kube-apiserver's global PodSecurity configuration. **There is no "unset" option.** [`defaultconfig.yaml#L12-L22`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/bindata/assets/config/defaultconfig.yaml#L12-L22) hardcodes all six keys as `invalid-to-force-substitution`, so a kube-apiserver whose PodSecurity block was never substituted fails to start. [`observePodSecurityAdmissionEnforcement`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/auth/podsecurityadmission.go) accordingly has two branches today and never returns a state in which the path is absent. "Disabling PSA" means substituting `privileged`, not omitting the configuration.

Its inputs are exactly three: `spec.podSecurityAdmission.enforceLevel`, read directly; `spec.podSecurityAdmission.enforceVersion`, clamped; and the `OpenShiftPodSecurityAdmission` feature gate, which selects the branch. The gate-off branch substitutes `privileged` and is unchanged from today; the gate-on branch substitutes the level from `enforceLevel`, defaulting to `privileged` when it is `Privileged`, empty or unavailable. **Both branches default `enforce` to `privileged`**, so crossing the gate boundary never changes what a cluster enforces. It does change what the cluster *reports*, because the gate-off branch keeps `warn` and `audit` pinned at `restricted` while the gate-on branch derives them from `enforceLevel` too.

**The observer is now the whole of the implementation on the enforcement path**, and that is the main structural benefit of retiring the readiness controller. It reads two administrator-written fields and a compiled-in gate, and depends on no other controller having run. There is no producer whose failure could hold up a level change, nothing to debounce because no input varies with cluster activity, and no ordering to get wrong. This is worth protecting: any design in which the observer consumes conditions written by another controller makes the break-glass path depend on the health of the component most likely to be broken when it is needed.

**This enhancement changes all six keys on the gate-on branch.** Both existing branches pin `audit` and `warn` to `restricted` regardless of enforcement level ([`#L32-L39`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/configobservation/auth/podsecurityadmission.go#L32-L39)). The gate-on branch stops doing that: `enforce`, `warn` and `audit` are all set from `enforceLevel`, and the three `*-version` keys all from `enforceVersion`, so the whole block derives from two fields. `Baseline` has no implementation today, so a third case must be added.

**The gate-off branch keeps the pinning**, like everything else on that branch — it is not edited, so release `n` renders exactly what `n-1` does. The consequence is that the two branches no longer agree for a cluster that has not opted in: gate-off gives `warn`/`audit: restricted` and gate-on-with-the-field-unset gives `warn`/`audit: privileged`. **Crossing the gate boundary therefore does change the rendered configuration**, where under the pinned design it did not, and that costs a control-plane rollout at `n+1` — see [The state a cluster actually arrives in](#the-state-a-cluster-actually-arrives-in).

**The clamp is the observer's responsibility and must never be skipped.** Because `defaultconfig.yaml` makes the block fatal, the observer is the last component that can stop a user-supplied version reaching a kube-apiserver that will not start with it — and the recovery for a bad enforcement setting is a write to the API server that just failed to start, which on a single-node cluster is unrecoverable without host access. The operator must therefore never substitute a version it has not bounded, regardless of what upstream's plugin does with an unknown one. *Whether upstream's loader in fact rejects or silently clamps such a value should be confirmed during implementation; the design does not depend on the answer.*

#### Retiring the PSA label syncer

The [PSA label syncer](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go) does not run when the gate is on, in any enforcement mode, and no `enforceLevel` value brings it back. Nothing is written, ever; labels already present are left exactly as they are, because a controller that never applies never prunes. The set it acted on — and therefore the set carrying its labels on any cluster upgrading into release `n+1` — is defined by `isNSControlled` ([`#L475-L522`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L475-L522)): user Namespaces that have not opted out via `security.openshift.io/scc.podSecurityLabelSync=false`. **OpenShift's own Namespaces are unaffected**, excluded both by the hardcoded `nsexemptions` payload list and by the `openshift-` prefix; they carry PSA labels from their own manifests, which the syncer never owned.

What this costs: no Namespace gets a computed `enforce` label again, so labelling becomes the Namespace owner's responsibility — component teams in CVO manifests, enforced by CI, and the administrator for user Namespaces with no equivalent check. Per-Namespace `warn` and `audit` labels stop being written too, and **these matter more than an earlier draft of this document allowed**. It said nothing depended on them because the global configuration pinned both to `restricted` regardless, so a per-Namespace label could only ever loosen what was reported. That is no longer true in either direction, and the reversal is set out in [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused).

`security.openshift.io/MinimallySufficientPodSecurityStandard` goes with it. It cached the syncer's working answer — the weakest PSS level admitting the SCCs reachable by a Namespace's ServiceAccounts — and its only non-test consumer product-wide was the readiness controller's choice of dry-run level ([`violation.go#L55`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c128b63ac1e9c45aa67032987b96370af783843e/pkg/operator/podsecurityreadinesscontroller/violation.go#L55)), which is retired too. Relocating the computation has no host left to relocate into, so "what standard does this Namespace need?" becomes `oc label --dry-run=server`, asked by a person. Existing annotations are **not** stripped, for the same reason the labels are not: they record what the cluster computed beforehand, and once nothing reads them they can mislead only a human, who has `managedFields` and the release note to date them.

##### Why it is retired outright

The obvious alternative — tie the syncer to the enforcement mode, off at `Privileged` and on at `Baseline` or `Restricted` — is rejected on four grounds.

- **At `Privileged` it has nothing to do.** It exists to make a `restricted` default survivable by computing a *less* restrictive label for Namespaces that cannot meet it. Once the default is `privileged` an unlabelled Namespace is admitted regardless, and the computed label is metadata no admission decision consumes.
- **At `Baseline` or `Restricted` it contradicts the request.** An administrator setting `Restricted` is asking for unlabelled Namespaces to be held to `restricted`; the syncer would immediately grant many of them a *lower* standard computed from their SCCs, so the cluster would report `enforceLevel: Restricted` while running a patchwork neither the administrator chose nor the API describes.
- **Its inference is the known-unreliable part of the system.** The [Motivation](#motivation) exists because deriving a standard from the SCCs reachable by a Namespace's ServiceAccounts is wrong often enough to matter, and keeping the syncer keeps that inference on the enforcement path.
- **A controller that starts and stops is harder to reason about than one that does not exist.** A mode-switched syncer must handle being started on a cluster it has never seen, catching up across every Namespace before the level can safely be raised, and being stopped mid-apply — cross-operator sequencing on independent timing.

**Advising mode is neither freezing nor removing**, though it looks like the safe middle ground: `syncedLabels` there holds `warn` and `audit` only and SSA prunes fields a manager no longer sends, so the `enforce` label *is* dropped — but only on the next apply, and `shouldUpdate` skips the apply unless a tracked value has drifted. A Namespace already correct keeps its label indefinitely; one that sees an unrelated SCC change six months later loses it at that moment. Identically configured clusters diverge on unrelated churn.

#### Labels on upgraded clusters

**There is no fleet-wide population of syncer-written `enforce` labels.** Per [What actually ships](#what-actually-ships) the gate has never been in `Default` in a shipped release, so the constructor customers run is the advising one, whose label set is `warn` and `audit` only. It is worth stating in the negative, because the opposite assumption — that the fleet is carpeted in computed `enforce` labels — generates a long chain of consequences that do not in fact arise: a retention-versus-stripping dilemma, an inert API on upgraded clusters, and workloads rejected weeks later by a label frozen too strict. None of them has a subject here.

What is actually on an upgraded cluster, and what happens to it:

| Label | Written by | On retirement |
|---|---|---|
| `pod-security.kubernetes.io/warn`, `audit` on syncer-controlled Namespaces | the advising syncer, SSA field manager `pod-security-admission-label-synchronization-controller` ([`#L386`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go#L386)) | stop being maintained; existing values retained untouched, because a controller that never applies never prunes |
| `pod-security.kubernetes.io/enforce` on payload Namespaces | CVO manifests | unaffected — the syncer never owned these, being excluded by `nsexemptions` and the `openshift-` prefix |
| `pod-security.kubernetes.io/*` set by an administrator or a layered product | whoever set it | unaffected; continues to take precedence over the global default |
| `pod-security.kubernetes.io/*` on an `openshift-`-prefixed Namespace that **opted in** with `scc.podSecurityLabelSync: "true"` | the syncer, **force-applied**, overriding the manifest that set them | retained and frozen; usually reasserted by the owning manifest on the next install or upgrade — see [Opted-in `openshift-` Namespaces](#opted-in-openshift--namespaces) |
| `pod-security.kubernetes.io/enforce` from the *enforcing* syncer | `TechPreviewNoUpgrade`, `DevPreviewNoUpgrade` and **MicroShift** | retained and frozen; the first two are excluded from upgrades, MicroShift is not — see [Retained labels are MicroShift's problem, not standalone's](#retained-labels-are-microshifts-problem-not-standalones) |

On standalone the surviving exposure is the first row. A frozen `warn`/`audit` label still cannot reject a workload — `enforce` was never in the advising syncer's label set — but it is no longer harmless either, and the next section explains why.

##### Frozen `warn` labels outlive the noise they caused

**This is the one place where letting `warn` and `audit` follow `enforceLevel` does not deliver what it promises.**

Per-Namespace labels outrank the cluster-wide configuration. While `warn` was pinned globally at `restricted`, a syncer-written label computed at the minimally-sufficient level could only ever be *looser* than the global setting, so its effect was to suppress warnings and its retention was free. Once the global setting drops to `privileged` on an opted-out cluster, the comparison inverts: a frozen `warn: baseline` or `warn: restricted` label is now **stricter** than the cluster-wide value, outranks it, and keeps producing exactly the terminal warnings this change exists to stop.

The population is not small. The advising syncer labelled every user Namespace that had not opted out, at whatever level its ServiceAccounts' SCCs implied, so on an upgraded cluster the Namespaces still warning after `n+1` are precisely the ones users work in every day. A fresh install is quiet; an upgraded cluster is substantially as noisy as before, in the Namespaces that matter most, with nothing writing those labels any more and nothing explaining them.

Three things follow, and the third is unresolved.

- **The release note must not claim the warnings stop.** It can say the cluster-wide configuration no longer warns, which is true, and that Namespaces carrying labels from the retired syncer continue to warn at the level recorded in those labels, which is also true. Claiming the noise is gone would be wrong on every upgraded cluster.
- **The label-removal procedure becomes a mainstream workflow, not a MicroShift corner.** [Removing retained `enforce` labels](#removing-retained-enforce-labels) is written for the `enforce` case on MicroShift; a standalone administrator now has a reason to remove retained `warn`/`audit` labels too, and that is a safer operation — deleting a `warn` label cannot relax an admission decision, because `warn` makes none.
- **Whether a cleanup should ship after all is reopened**, having been settled on the premise that stale labels harmed nothing. See [Why no cleanup ships](#why-no-cleanup-ships) and the open question in [Should the retired syncer's `warn` labels be cleaned up](#should-the-retired-syncers-warn-labels-be-cleaned-up).

##### Opted-in `openshift-` Namespaces

The claim that the syncer never touches `openshift-` Namespaces holds for the payload and not in general, and the exception is worth stating because it is a layered-product install pattern rather than a curiosity. `isNSControlled` tests the opt-in **before** the prefix skip:

```go
if nsexemptions.IsNamespacePSALabelSyncExemptedInVendoredOCPVersion(nsName) { return false }
if ns.Labels[labelSyncControlLabel] == "true" { return true }   // wins over the prefix
if strings.HasPrefix(nsName, "openshift-") { return false }
```

So an `openshift-`-prefixed Namespace outside `nsexemptions` that sets `security.openshift.io/scc.podSecurityLabelSync: "true"` **is** syncer-managed — and `mustSync` then feeds `Force: mustSync` into the Apply and bypasses the per-label ownership check, so the syncer *force-takes* those labels from whichever manager set them. The documented Windows Machine Config Operator namespace manifest does exactly this, setting the opt-in alongside `pod-security.kubernetes.io/enforce: "privileged"`; today the syncer overrides that manifest value with its own computed one, and nobody notices because the two agree. `openshift-operators` is eligible too, being deliberately kept out of `nsexemptions`.

Retirement freezes these like any other, and the outcome is usually benign: the owning manifest normally sets an explicit `enforce` label, which reasserts once no manager is competing for it. **"Usually" is doing real work there** — it is not guaranteed, nothing detects the cases where the manifest sets no label, and a product whose computed label was looser than its manifest's will tighten on the next apply. **Layered-product reviewers own this row**, and it is the second thing they are being handed, after [OLM Namespaces carry no label](#olm-namespaces-carry-no-label).

##### Retained labels are MicroShift's problem, not standalone's

MicroShift is the only shipped product where the *enforcing* syncer ran across customer Namespaces, so it is the only one that inherits a population of frozen `enforce` labels — and therefore the only one where the consequences the rest of this section dismisses actually bite.

**Moving MicroShift's default to `privileged` does not reach them.** Per-Namespace labels outrank the global default, so a customer Namespace carrying a syncer-written `enforce: restricted` keeps enforcing `restricted` after the upgrade. The relaxation described in [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift) applies to **unlabelled** Namespaces only, and the claim that nothing a MicroShift user runs can break should be read as *nothing breaks at the moment of upgrade* — which is true, because freezing a label changes no admission decision on the day.

**What breaks is later, and it is the fossil case.** Take a MicroShift Namespace labelled `enforce: restricted` by the syncer, whose administrator afterwards grants its ServiceAccount a broader SCC to run a workload needing a fixed UID. Before the change the syncer would have relaxed the label to `baseline`; now it stays pinned and the workload is rejected, on a deployment whose administrator has no reason to connect the failure to an upgrade weeks earlier. No configuration resolves it — not the new API, which does not reach MicroShift, and not the global default, which the label outranks. The remedies are to edit the label or delete it and let the Namespace fall to `privileged`, both covered in [Removing retained `enforce` labels](#removing-retained-enforce-labels).

Nothing detects it and no cleanup ships, for the reasons below. The mitigation is the MicroShift [release note](#release-note) and the dry-run sweep, and this is the strongest argument in the document for MicroShift's documentation telling users to **audit their syncer-written labels at upgrade time** rather than waiting to meet the problem.

##### Why no cleanup ships

No cleanup ships anywhere. On MicroShift that is a deliberate decision against a real population; **on standalone it is no longer the easy call it was**, and the `warn` case is genuinely open.

**On standalone, `enforce` is trivial and `warn`/`audit` is not.** There are no stranded `enforce` labels on a `Default` cluster, so nothing there needs cleaning. The `warn`/`audit` labels were dismissed on the premise that they harm nothing, which held while the global setting pinned both to `restricted` and a per-Namespace label could only loosen it. [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused) removes that premise: those labels are now the only thing still warning on an opted-out cluster, and they are the reason the noise reduction does not reach upgraded clusters.

The case for pruning them is correspondingly strong, and the usual objection does not apply: deleting a `warn` or `audit` label **cannot relax an admission decision**, because neither mode makes one, so the asymmetry that decides the MicroShift `enforce` case — stripped labels fail silently against a compliance control — has no force here. What remains against it is cost and precedent: a one-shot controller writing to every Namespace on every cluster, shipped to remove metadata, with its own failure modes and its own upgrade story. **This is not settled**; see [Should the retired syncer's `warn` labels be cleaned up](#should-the-retired-syncers-warn-labels-be-cleaned-up).

**On MicroShift it is a real decision against a real population**, and the reasoning is the one the earlier premise made unnecessary on standalone. A one-shot cleanup that deleted syncer-written `enforce` labels would drop every affected Namespace to the new `privileged` default in a single sweep — a silent, fleet-wide relaxation on an edge product, applied to Namespaces whose administrators chose none of it, and irreversible because server-side apply archives nothing. Freezing is worse for the individual fossil Namespace and better for everyone else, and it has the property that an administrator can undo it per Namespace once they decide they want to, which stripping does not. The asymmetry decides it: a frozen label is too strict and fails loudly at admission; a stripped one is too loose and fails silently, at review time, possibly against a compliance control.

Existing values are left exactly as they are in both cases, dated by `managedFields` for anyone who needs to know when they stopped being true.

##### OLM Namespaces carry no label

`openshift-operators`, where OLM installs operator bundles by default, carries no PSA label from any source. The kube-apiserver's PodSecurity configuration has no Namespace exemptions at all — [`defaultconfig.yaml`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/bindata/assets/config/defaultconfig.yaml) exempts exactly one username — so it is governed entirely by the global default. It is deliberately kept out of `nsexemptions`, which carries an `IMPORTANT:` comment saying it must not be exempted, but it is then caught by the `openshift-` prefix skip, so the syncer never labelled it even when it ran.

**Only one direction bites, and it is not the upgrade.** The effective level does not drop on upgrade, because the global default on a shipped cluster is already `privileged`. What remains is the opt-in direction, and **it is overwhelmingly a `Restricted` problem rather than an opt-in problem**. On a cluster setting `Restricted`, every operator bundle needing more than restricted fails admission, which is most of them — operators routinely want a fixed UID, a host path or an elevated capability — so for that cluster this is a mass-breakage path. At `Baseline` the picture is very different: a bundle fails only if it wants a genuine privilege escalation, which is a much smaller population, and many clusters will find `openshift-operators` clean. The distinction is worth drawing because the two cases warrant different advice, and collapsing them makes opting in at all look unsafe. Either way it is triggered by a deliberate administrator act rather than by an upgrade, which makes it predictable and discoverable by the dry-run sweep beforehand — run at the level actually being requested. **This is out of scope**: labelling that Namespace is an OLM-side change, retiring the syncer neither causes nor worsens the gap, and exempting the Namespace contradicts the comment. What this enhancement owes is disclosure, in the release note and in [Monitor tests for managed Namespaces](#monitor-tests-for-managed-namespaces). **OLM and layered-product reviewers are required to accept the hand-off**, not to resolve it here.

MicroShift differs here and the difference is worth recording: it ships `openshift-operators` with an explicit `pod-security.kubernetes.io/enforce: privileged` label in its own manifests, so the gap is standalone-only.

##### Release note

The release note must **not** say that the default posture changes: per [What actually ships](#what-actually-ships) the cluster-wide default is `privileged` before the upgrade and `privileged` after it, and a note claiming otherwise would send administrators looking for a regression that is not there.

At minimum, six things:

- **Enforcement is now configurable, and `privileged` is now the declared default.** The cluster-wide level has been `privileged` on every shipped release since the gate was introduced; what is new is `spec.podSecurityAdmission` to raise it, and the fact that this is now a documented product decision rather than a side effect of release branching. Administrators who assumed they were enforcing `restricted` should be told plainly that they were not.
- **PSA warnings and audit annotations stop on clusters that have not opted in.** `warn` and `audit` now follow the configured level, so a cluster at `privileged` no longer warns on `oc apply` about `restricted` violations it was never going to reject. This is the change users will *feel*, and it is intended. It has a consequence they will not feel: **`PodSecurityViolation` can no longer fire on such a cluster**, so its silence stops being evidence of anything. Anyone treating that alert as a PSA health check needs to know, and the note should point at the dry-run procedure as the replacement.
- **Namespaces labelled by the retired syncer keep warning.** On an upgraded cluster, per-Namespace `warn` labels outrank the cluster-wide setting, so the Namespaces the syncer managed — most user Namespaces — continue to warn at the level recorded in their labels. **The note must not claim the warnings are gone**; it must say where the remaining ones come from and how to remove them. See [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused).
- **The six `PodSecurity*EvaluationConditionsDetected` conditions are withdrawn**, naming them individually. This is the change that lands on every cluster: they are public surface that anything may be reading, and their disappearance is otherwise indistinguishable from a broken operator.
- **Per-Namespace `warn` and `audit` labels stop being maintained.** Existing values are kept and frozen. Nothing is rejected as a result — `enforce` was never in the advising syncer's label set — but the values may be stale, and they are now the only source of PSA warnings on an opted-out cluster.
- **`openshift-operators` is governed by the global default**, which is `privileged` and does not change here, but becomes a mass-breakage path for any cluster that opts in to `Restricted`. See [OLM Namespaces carry no label](#olm-namespaces-carry-no-label).

Both retirements are removals with no supported way back, and must be said plainly rather than framed only as "enforcement is now optional". Clusters running `TechPreviewNoUpgrade` or `DevPreviewNoUpgrade` need a separate line: they are the only ones whose enforcement level moves, from `restricted` down to `privileged`, and they are the only ones carrying syncer-written `enforce` labels, which are retained and frozen.

**MicroShift needs its own release note, and unlike standalone's it announces a real posture change.** It must say that the global default moves from `restricted` to `privileged`, so a customer-created Namespace with no `enforce` label of its own is **less** restricted than before, not more; that MicroShift's own Namespaces are unaffected because they now carry explicit labels; and that no workload should fail as a result, since the change only relaxes. It must name `security.openshift.io/scc.podSecurityLabelSync` as deprecated and inert, since `docs/user/howto_pod_security.md` has been telling users to set it, and point at per-Namespace labelling as the replacement for anyone who wants the old level back. Because the change is a relaxation, the audience that needs warning is the one relying on restricted-by-default as a security control — the opposite of the audience a breakage note addresses, and easy to miss when drafting.

#### New Installation

Fresh installs do not enforce PSA by default: with `enforceLevel` unset the cluster resolves to `Privileged` for `enforce`, `warn` and `audit` alike, so the plugin is loaded and silent. From there the administrator raises the level they are comfortable with, and returns it to `Privileged` to go back.

A fresh install is the only case where retiring the syncer costs nothing: no Namespace carries a syncer-written label, so there are no fossils, the API is not inert, and `Privileged` really is the cluster's effective posture rather than only its default. **It is also the only case where the noise reduction is complete** — with no frozen `warn` labels anywhere, a new cluster genuinely stops warning, which an upgraded one does not; see [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused). The trade appears later — once the administrator opts in to `Restricted`, every Namespace their workloads create from then on is `restricted` unless they label it, and nothing is computing labels on their behalf.

### Risks and Mitigations

- **`privileged` becomes the *declared* default rather than an undeclared one.** Clusters that take no action are no less restrictive than today — per [What actually ships](#what-actually-ships) the shipped default is already `privileged` — but this enhancement ratifies that and removes the expectation that a future release will move them. *Mitigation:* per-Namespace labels still take precedence, SCCs are unchanged, and opting in is now possible where it previously required an irreversible preview feature set. **Still requires explicit security sign-off**, on the ratification rather than on a regression.
- **An opted-out cluster reports nothing about PSA at all.** `warn` and `audit` follow `enforceLevel`, so at `Privileged` nothing is evaluated, no denial is counted and `PodSecurityViolation` cannot fire. *Mitigation:* the dry-run sweep answers on demand, and nothing is being rejected on such a cluster either, so what is lost is foresight rather than incident detection. *Not mitigated:* the loss is exactly the pre-flight signal an administrator would want **before** opting in, which is the one moment it mattered. This is the central trade of the enhancement and it is argued at [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in), with cheaper reversals recorded at [Shape of the `warn` and `audit` levels](#shape-of-the-warn-and-audit-levels). **Requires explicit security sign-off alongside the posture ratification.**
- **The alert's silence is indistinguishable from health.** On every opted-out cluster — which is the whole fleet at `n+1` — `PodSecurityViolation` is structurally incapable of firing. *Mitigation:* the alert description and runbook must state that silence means nothing was evaluated. *Not mitigated:* dashboards and Insights rules keyed on this alert will show a fleet-wide drop to zero that looks like remediation and is not, and the owners need the same advance notice as for the withdrawn conditions.
- **The noise reduction does not reach upgraded clusters.** Frozen per-Namespace `warn` labels outrank the cluster-wide `privileged` and keep warning in exactly the Namespaces users work in. *Mitigation:* none currently ships. *Open:* whether to clean them up — see [Should the retired syncer's `warn` labels be cleaned up](#should-the-retired-syncers-warn-labels-be-cleaned-up) and [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused).
- **Nothing on the cluster tells an administrator whether opting in is safe, and nothing warns them when they do it.** The requested level is applied on a cluster the platform has formed no opinion about. *Mitigation:* the dry-run loop answers on demand. *Not mitigated:* an administrator who does not run it, and there is no longer a passive signal for one who does not. This is the risk the retired readiness controller existed to reduce, and it is now the enhancement's weakest point — argued in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in), with the post-opt-in half in [Discovering failures after an opt-in](#discovering-failures-after-an-opt-in).
- **The only proactive PSA signal depends on an unverified property of the upstream plugin.** `PodSecurityViolation` reads `mode="audit"` denials, so on an opted-in cluster it works only if PSA evaluates `audit` independently of an `enforce` denial rather than short-circuiting — and with `audit` now equal to `enforce`, every denial is a case where both fire at once, so the question is sharper than it was. *Mitigation:* confirm against the upstream plugin during implementation and assert it in a test. *Residual:* if the assumption is wrong the alert never fires on any cluster, opted in or out, and the product ships with no proactive PSA signal whatsoever. See [Discovering failures after an opt-in](#discovering-failures-after-an-opt-in).
- **A clean dry-run is not a guarantee.** PSA is validating-admission only, so a workload not running at the time, or recreated later by a drain or upgrade, can fail long after a sweep came back clean — and nothing re-runs the check on its own. *Mitigation:* on an opted-in cluster `warn` and `audit` sit at the enforced level, so the eventual failure is observable when it happens. *Not mitigated* before the opt-in, when the sweep is the only instrument. See [What opting in still risks](#what-opting-in-still-risks-with-nothing-evaluating).
- **A Namespace created after the opt-in is enforced immediately**, with nothing labelling it, nothing evaluating it and nothing reporting it. *Mitigation:* payload Namespaces get labels from CVO manifests, verified by the monitor tests. *Not mitigated* for user Namespaces or OLM's.
- **Six ClusterOperator conditions are withdrawn**, breaking any dashboard or Insights rule keyed on them. *Mitigation:* advance notice to the owners, and removal of stale conditions on upgrade rather than leaving them standing. See [What stops being reported](#what-stops-being-reported).
- **Red Hat loses its fleet view of PSA readiness** at the point it would be used to measure this feature's own adoption. *Unmitigated*, by [Non-Goal](#non-goals).
- **`openshift-operators` has no label from any source** and follows the global default in both directions. **Unmitigated and out of scope**; see [OLM Namespaces carry no label](#olm-namespaces-carry-no-label).
- **A frozen `warn`/`audit` label can misreport a Namespace indefinitely**, and now outranks a cluster-wide setting that is looser rather than one that is stricter, so it warns where the cluster would not. *Mitigation:* it still cannot reject anything, and removing it is safe for the same reason. No `Default` cluster carries syncer-written `enforce` labels, so there is no corresponding risk of a frozen label rejecting a workload weeks after an upgrade. See [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused).
- **A clamped `enforceVersion` is applied without the administrator being told**, though it is now *detectable*: `status.enforceVersion` reports the applied value and differs from `spec`. *Residual:* nothing surfaces the divergence actively — no condition, no event, no alert — so it is found only by someone who thinks to compare the two. See [`enforceVersion` and why the default moves](#enforceversion-and-why-the-default-moves).
- **An unset `enforceVersion` tightens the standard on every upgrade**, so a cluster can acquire a violation from an upgrade alone. *Mitigation:* pinning holds it still, and the behaviour matches today's. *Not mitigated for clusters that do not pin*, which will be most of them.
- **Every enforcement change rolls the kube-apiserver**, which on SNO is an API outage, including on the disable path. *Mitigation:* the observer reads `spec` directly and depends on no other controller, so the change cannot be held up by a failing component; the cost is documented in the break-glass procedure.
- **The feature gate is redefined rather than added, so the same gate name means different things in different releases.** Anything reading it across a version boundary can act on the wrong meaning. *Mitigation:* the gate is already out of `Default` and stays out of it through release `n`, and the gate-off branch is not edited, so the two readings agree on every standalone cluster by construction; the one place the boundary is crossable — HyperShift's `control-plane-operator` — gets a bounded version gate that sunsets. *Not mitigated* for an out-of-payload consumer nobody knows about that string-matches the gate list; the enum-preservation rule does not help here, because the gate is not the enum. See [Gate redefinition skew](#gate-redefinition-skew).
- **Four gate-on branches have to be changed in step**, in `cluster-kube-apiserver-operator` twice, `cluster-policy-controller` and `hypershift`, and a wrong branch is a silent posture change rather than a failure. *Mitigation:* no gate-off branch is edited, so the path every real cluster is on is unchanged by construction rather than by assertion; MicroShift's absent arm is tested as a case in its own right rather than inferred from the default. *Residual:* the arms sit in one switch, so a tidying refactor can still catch the arms that were meant to be left alone.
- **MicroShift's effective posture relaxes for customer Namespaces**, from a computed minimally-sufficient label to `privileged`. *Mitigation:* MicroShift's own Namespaces keep their current level through explicit manifest labels, and violations remain visible if MicroShift keeps `warn` and `audit` at `restricted` — which is change 1 and is still undecided. *Not mitigated:* a user relying on restricted-by-default as a security control loses it, and unlike standalone has no API to ask for it back; if `warn` and `audit` move too, they lose the visibility with no way to restore that either. **Requires explicit MicroShift sign-off**; see [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift).
- **The three MicroShift changes must land together, and only one of them is in this enhancement's repositories.** Dropping the syncer is a `cluster-policy-controller` change; moving the default and labelling the eight Namespaces are MicroShift-side manifest changes. *Hazard:* the syncer stopping **before** the default moves leaves customer Namespaces unlabelled under a hardcoded `restricted` — which is precisely the breaking change this arrangement exists to avoid, reintroduced by a sequencing slip. The ordering is not enforced by anything; it needs to be stated in both repositories and verified in CI.
- **MicroShift's change is not staged behind the feature gate and can land in the wrong release.** The absent arm is edited in code, not gated, so it reaches MicroShift whenever the vendored `cluster-policy-controller` is bumped — independently of the manifest changes above. *Mitigation:* edit the arm in `n+1` and have MicroShift confirm both the vendoring and the manifest changes land there.
- **A deprecated `scc.podSecurityLabelSync` fails silently.** From `n+1` the write succeeds and does nothing, which is indistinguishable from working. *Unmitigated in-product:* the label is ordinary Namespace metadata that no component validates, so there is no admission-time feedback to add. The release note and the MicroShift docs rewrite are the whole of it.
- **Inverting the gate-off branch would enforce `restricted` fleet-wide in release `n`.** This is the sharpest failure available here, and it is reachable by an implementer who reads `inDefault()` on `master` and concludes that `Default` clusters are gate-on — which is the natural reading. *Mitigation:* [What actually ships](#what-actually-ships) states the evidence, and the gate-off regression test makes the error fail loudly in CI rather than at a customer. See [The gate-off branch must not be touched](#the-gate-off-branch-must-not-be-touched).
- **A downgrade from `n+1` restarts the syncer as well as re-enabling enforcement**, because one gate carries both. The syncer resumes writing `enforce` labels computed against SCC state that may have moved on considerably since it last ran. *Partly mitigated:* the result is a state the product shipped for years rather than an untested hybrid, and `unsupportedConfigOverrides` can hold enforcement off, though it cannot stop the syncer. See [On Downgrade](#on-downgrade).

Security review is required from the OpenShift security architecture group, covering the default-posture change and the withdrawal of the readiness signal together rather than the API. UX review is required from the console and docs teams for the `oc` workflow, which is now the entire user interface to this feature.

### Drawbacks

- **It makes a weak default permanent.** The product has shipped `privileged` for nine releases while carrying `inDefault()` on `master` and the stated intention to migrate. This enhancement ends that intention. Nothing about the running posture gets worse, but the plan to make it better is cancelled, and for a product positioned as secure by default that is the real cost here — a strategic one rather than a regression. It is narrowed, but not answered, by [`Privileged` is a posture, not only an opt-out](#privileged-is-a-posture-not-only-an-opt-out): SCC still governs every Pod, so what is given up is a second overlapping control rather than admission control as such. A cluster running both is still better defended than a cluster running one.
- **The platform stops being able to say anything about PSA, and stops looking.** Before this enhancement OpenShift maintained a per-Namespace answer to "what standard can this Namespace meet?", a per-cluster answer to "could this cluster enforce `restricted`?", and a continuous `audit` stream counting would-be violations on every cluster whatever it enforced. After it, none of the three exists on a cluster that has not opted in, and all become things a person asks one cluster at a time with a shell loop. **The third is the one added late and it changes the character of the first two**: retiring the controller removed the standing answer, but letting `warn` and `audit` follow `enforceLevel` removes the raw observation the answer was derived from, so the cluster is not merely unreported but unobserved. This is the largest reduction in what the product knows about itself and the change most likely to be objected to in review.
- **Opting in is a coarser instrument than the enforcement it replaces.** Today's syncer puts each Namespace at the strictest standard it can actually meet, which is a per-Namespace answer; this enhancement offers one cluster-wide level for every unlabelled Namespace. `Baseline` is a real middle ground and most heterogeneous clusters can reach it, so the choice is not as stark as "clean enough for `restricted`, or nothing" — but it is still one number for the whole cluster. An administrator whose Namespaces genuinely differ must label them by hand, with nothing computing the per-Namespace answer the syncer used to supply, and the work of doing so is the work this enhancement transfers from the platform to them.
- **The stale `warn`/`audit` labels become a permanently unmaintained class of cluster state** that support must reason about for as long as those clusters live — and a larger class than it looks, since they now override a looser cluster-wide setting rather than being overridden by a stricter one. The exposure is still limited to reporting: with no syncer-written `enforce` labels on a `Default` cluster, the API governs admission for every unlabelled Namespace exactly as intended. But they are why the noise reduction, which is one of this change's two stated benefits, does not reach the clusters that already exist.
- **The status is a precedent, a partial answer, and standalone-only.** `cluster-kube-apiserver-operator` becomes the first writer of `APIServerStatus`, so a `config.openshift.io` resource that was pure administrator intent is now partly operator-owned, and the next field proposed for that status will cite this one. It answers less than its name suggests — not per-master, not net of `unsupportedConfigOverrides`, not the effective level on a labelled Namespace ([What `status` does not say](#what-status-does-not-say)) — and it **does not exist on HyperShift at all**, so hosted clusters keep the invisible clamp that standalone sheds. Paying a precedent for a report that two of the three topologies get and one does not is the weakest part of this choice.
- **A downgrade can silently erase the administrator's recorded intent.** This is the price of a gated field over a dedicated CRD, and re-upgrading does not restore it. See [Schema pruning erases the configuration](#schema-pruning-erases-the-configuration).
- **It bundles three separable changes** — retiring the syncer, retiring the readiness controller, and the new API. The first two land on every cluster whatever `enforceLevel` says, and between them they remove more code and more public surface than the API adds, so a reviewer who evaluates only the API has not evaluated the change.
- **Applying a configuration change costs a control-plane rollout**, which on SNO is an outage, in both directions.

## Alternatives (Not Implemented)

The alternatives below have been identified but not yet evaluated to a conclusion. **This section must be completed before the EP moves from `provisional` to `implementable`.**

### Keep the readiness controller

**The most likely objection to this enhancement, and the one to argue out first.** The administrator's question — *can my cluster survive `restricted`?* — did not disappear when the migration did; only Red Hat's question did. A controller already exists that answers it on a schedule, without being asked, and replacing it with a documented shell loop is a worse answer to a question that opting in deliberately makes *more* important.

It is rejected on three grounds, in descending weight. Its population is wrong and cannot be cheaply fixed, since `nonEnforcingSelector` excludes every Namespace carrying an `enforce` label — after retirement a permanent and usually overwhelming majority — so the standing answer would be most confident about the clusters it understood least. Its answer is pinned at `restricted` for fleet comparability, which gives a `Baseline` administrator a permanent false positive. And it is the entire operational cost of the feature: the four-hourly sweep, the status subtree, the `apiservers/status` RBAC, four metrics, an alert and a runbook, all serving advice nothing acts on. A narrower variant — evaluate at the *configured* level over *all* Namespaces and write only a count — answers the first two objections and should be costed before this is settled, but it is a substantially new controller rather than a retained one.

**This objection compounds with the `warn`/`audit` decision.** Pinned at `restricted`, those levels gave the cluster a continuous readiness signal as a side effect, so retiring the controller would have cost the *standing* answer but not the *existence* of one. Deriving them from `enforceLevel` removes that too: an opted-out cluster has neither an evaluation nor an audit stream, and the only instrument left is a shell loop somebody has to decide to run. **If one of the two must be put back, [Split `warn` from `audit`](#split-warn-from-audit) is the cheaper reversal** — it restores the continuous signal without a controller, a four-hourly sweep or a telemetry surface, and is the option to exhaust before this one.

### Keep a minimal enforcement-mode metric

Export `pod_security_enforcement_mode` from the config observer and ship nothing else: a single gauge, derived from a field read with no sweep, letting Red Hat measure adoption and letting support answer the question without a live session. Rejected to keep the retirement clean — a feature that ships one metric still owns a telemetry surface, a dashboard and the question of what else belongs beside it, and the value is to Red Hat rather than to the administrator. **This is a cheap decision to reverse**, recorded so that reversing it is deliberate rather than a rediscovery.

### Shape of the `warn` and `audit` levels

Three arrangements were available: keep both pinned at `restricted` as today, let `warn` follow `enforceLevel` while `audit` stays pinned, or move all three together. The chosen one is the most aggressive, and it is the one that makes an opted-out cluster silent. **[Split `warn` from `audit`](#split-warn-from-audit) is the cheaper reversal if the pre-flight signal has to come back**, and [Keep both pinned at `restricted`](#keep-both-pinned-at-restricted) is the full reversal; both are costed in [Appendix: Extended Alternatives](#extended-the-warn-and-audit-levels). The choice is argued in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in) and [Alerts and metrics](#alerts-and-metrics).

### Shape of the feature gating

Every arrangement reaches the same end state and differs only in how many gates it takes and what has to be version-gated. **The criterion that decides between them is edit surface: how much of the code path supported clusters actually run does the arrangement require touching?** Redefinition requires none; every alternative requires some, and the edits land on exactly the branch where a mistake is least visible. Because there is no posture change, the axes that would otherwise discriminate — release count and posture sequencing — do not. The chosen shape is argued in [Why one gate rather than two](#why-one-gate-rather-than-two); the rejected ones, including [Add a new gate and retire the existing one](#add-a-new-gate-and-retire-the-existing-one) and the deferred `cluster-policy-controller` flag, are in [Appendix: Extended Alternatives](#extended-the-feature-gating-shape).

### Shape of the API

Four shapes were considered: a dedicated `PSAEnforcementConfig` CRD, spec-only with no `status`, spec on `APIServer` with status on `KubeAPIServer`, and documenting `unsupportedConfigOverrides` and shipping nothing. `APIServer` won because it is already the cluster-wide API server configuration surface, already consumed by the config observers this enhancement extends, and already plumbed into HyperShift for the spec. The status is the contested part: shipping none is the fallback if the first-writer precedent on `APIServerStatus` proves unacceptable to API review, and is in effect what HyperShift gets anyway. Full comparison in [Appendix: Extended Alternatives](#extended-the-api-shape).

### Shape of the enforcement decision

**Gating the raise on a clean evaluation** is the alternative with the strongest safety story and the one most likely to be re-proposed. It is rejected because it protected the wrong thing — the global default governs only Namespaces with no label of their own — because a point-in-time sweep with known blind spots becomes a liability the moment a false positive blocks an administrator from configuring their own cluster, and because it had no expression in HyperShift. It is now additionally foreclosed: with the readiness controller retired there is no evaluation to gate on, so re-proposing it means re-proposing [Keep the readiness controller](#keep-the-readiness-controller) first. Reporting violations in `status` is foreclosed with it. Detail in [Appendix: Extended Alternatives](#extended-the-enforcement-decision).
## Open Questions

Retiring the readiness controller closes four questions that would otherwise be open, and they are listed rather than omitted so that reviewers can see they were answered by removal rather than overlooked: whether a write to `spec` should trigger an immediate re-evaluation; whether a cluster passing through release `n` would ever complete an initial evaluation; how to bound the sweep's cost and pagination on large clusters and SNO; and how to read an absent `validated-scc-subject-type` annotation on workloads predating it. With no evaluation on the cluster, none of them has a subject.

### Should the retired syncer's `warn` labels be cleaned up

Letting `warn` and `audit` follow `enforceLevel` is meant to stop an opted-out cluster warning about a standard it does not apply. On a **fresh install** it does. On an **upgraded** cluster it largely does not, because the advising syncer's frozen per-Namespace `warn` labels outrank the cluster-wide `privileged` and keep warning — in exactly the user Namespaces people work in. See [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused).

The options are to ship a one-shot cleanup that deletes `warn` and `audit` labels whose `managedFields` owner is `pod-security-admission-label-synchronization-controller`, which is safe in a way the `enforce` cleanup is not because neither mode makes an admission decision; to document the removal and leave it to administrators, which means most clusters stay noisy indefinitely; or to accept the noise on upgraded clusters and claim the benefit only for new installs.

**A decision is needed before GA**, and it should be made explicitly rather than inherited from [Why no cleanup ships](#why-no-cleanup-ships), whose reasoning assumed a premise this change removes. It also determines what the [release note](#release-note) can honestly claim.

### Is a shell loop an adequate diagnostic

The dry-run in [Finding the violating Namespaces](#finding-the-violating-namespaces) is now the only way to ask whether a cluster can meet a standard, and it is `O(Namespaces)` serial server round-trips run by hand against a cluster that may already be unhealthy. It scales worst exactly where the answer matters most. The options are to ship it as a documented procedure and accept that; to add `oc adm` tooling that parallelises and bounds it; or to ship it only as a must-gather collector and have administrators read the bundle. **A decision is needed before GA**, because it determines whether the documentation can honestly recommend opting in on a large cluster.

### Removing the withdrawn ClusterOperator conditions

Six `PodSecurity*EvaluationConditionsDetected` conditions stop being written, and stale ones must be removed rather than left standing, since nothing garbage-collects a condition whose writer is gone and the status controller unions them into the ClusterOperator. What is unspecified is *what* does the removing, given that the natural candidate is the controller being retired. The likely answer is a one-shot removal on operator start in release `n`, which has to be written, has to be idempotent, and has to survive the controller's code being deleted. Who is told, and when, is a separate obligation recorded in [What stops being reported](#what-stops-being-reported).

### How CI lanes get a `restricted` cluster

The expected PSS for OpenShift's own components stays `Restricted`, and monitor and periodic tests depend on that. The syncer was never what provided it — `isNSControlled` skips every `openshift-` prefixed Namespace. What is undecided is how lanes get a `restricted` cluster now: opting in sets the global default but does not bring the syncer with it, so any test Namespace the lanes create themselves (`e2e-test-*` and similar, which were syncer-managed) inherits `restricted` with nothing computing a gentler label. Whether that breaks existing e2e suites, and whether the fix is per-test labelling or a broader exemption, is the input the [Test Plan](#test-plan) is missing.

**Two things changed this question rather than answering it.** Preview feature sets no longer imply `restricted`, so a `TechPreviewNoUpgrade` lane is `privileged` unless something writes `enforceLevel` — the lane has to set the field as a setup step, which is a change to lane configuration rather than to this enhancement, but it has to be made somewhere and nobody owns it yet. And because `warn` and `audit` follow the level, a lane running at `Privileged` produces no PSA signal at all, so a suite that silently stopped exercising PSA would pass rather than fail. **At least one lane must run at `Restricted`** for the admission assertions in [Switching between PSA modes](#switching-between-psa-modes) to mean anything.

### Day 0 configuration

The Summary and [New Installation](#new-installation) both state enforcement can be requested at install time, but no install-config field, manifest name or bootstrap rendering path is specified, and no installer reviewer is assigned. The bootstrap kube-apiserver must render the same `enforce`, `warn` and `audit` keys as the post-bootstrap observer, or the first control-plane rollout silently changes the cluster's posture out from under an administrator who asked for it at install time. One hazard that might be expected here does not arise: a day-1 manifest setting a level before anything has evaluated the cluster is no different from a day-2 write, since nothing evaluates at either.

## Test Plan

### The gate-off path is today's behaviour

**This is the most important test in the enhancement and the one to write first.** Every other assertion here describes new behaviour, which is visible when it is wrong; this one describes *unchanged* behaviour, which is not. The gate is off in `Default` for the whole of release `n` and remains off on every downgrade target afterwards, so the gate-off branch is the path real clusters are actually on. Nothing on that branch is edited, so these tests guard against an accidental change rather than verifying an intended one. That is precisely why they must exist: an unintended edit here is a fleet-wide posture change that no other assertion in this plan would catch.

With `OpenShiftPodSecurityAdmission` off, assert against the state the product ships today rather than against a description of it:

- the rendered `admission.pluginConfig.PodSecurity.configuration.defaults` block is **byte-for-byte identical** to the one release `n-1` renders — `enforce: privileged` with `audit` and `warn` pinned to `restricted` — all six keys, compared against a golden file generated from `n-1` rather than hand-written;
- the PSA label syncer is running and in **advising** mode, asserted by observing that it maintains `warn` and `audit` on a Namespace whose ServiceAccounts' SCCs change and does **not** write `enforce`, rather than by inspecting construction;
- the `PodSecurityReadinessController` is running and the six `PodSecurity*EvaluationConditionsDetected` conditions are present on the `kube-apiserver` ClusterOperator;
- `podSecurityAdmission` is **absent from the `Default` CRD schema**, and a write setting it is pruned rather than rejected.

Separately, and not covered by any of the above because it does not read the gate at all: **the absent-gate arm of `cluster-policy-controller`'s switch.** This is MicroShift's path and nothing else's, so it has to be a unit test over the switch with the feature gate accessor reporting the gate as neither set nor unset — the condition `cluster-policy-controller` runs under when vendored without a `FeatureGate` CR. Asserting it through the gate-off case instead would pass while the behaviour was broken, because the two arms now differ: `=false` constructs the advising syncer and absent constructs nothing.

The assertion **changes between releases**, and the test has to change with it: in release `n` the absent arm still yields the enforcing syncer, and in `n+1` it yields no syncer at all. That makes it the one test in this plan whose expected value is release-dependent, so it needs a comment naming `n+1` and tying it to [The absent arm changes](#the-absent-arm-changes-and-microshift-has-no-gate-to-stage-it-behind) — otherwise whoever flips it will look like they are fixing a failing test rather than implementing a decision.

### Switching between PSA modes

With the gate on, for each transition between `Restricted`, `Baseline` and `Privileged`:

- the kube-apiserver's effective `enforce` level, read from the revisioned `config-<revision>` ConfigMap, **and `status.podSecurityAdmission` agreeing with it** once the rollout completes — the ConfigMap remains the ground truth and the status is asserted against it, not instead of it;
- that `status.enforceVersion` reports the **clamped** value when `spec.enforceVersion` names a version ahead of the cluster's own, since that divergence is the entire reason the status exists and is the one assertion that would fail silently if the clamp were applied without being reported;
- **that neither the PSA label syncer nor the `PodSecurityReadinessController` runs**, in any mode. "Not running" is what distinguishes this design from a reduced or advising mode, so both need a direct test rather than being inferred from nothing changing — and, since the same binaries do run them when the gate is off, from nothing being constructed either;
- that a Namespace's existing `enforce` label is **byte-for-byte unchanged** across every transition, including after an unrelated SCC or RBAC change and including on the transition back up to `Restricted`;
- that a Namespace with **no** `enforce` label is held to the global level as soon as the cluster opts in, with nothing computing a gentler label for it;
- that **all three of `enforce`, `warn` and `audit` carry the configured level** in every mode, asserted on all six keys of the rendered block rather than on `enforce` alone. The hazard is a partial implementation that moves `enforce` and leaves the historical `restricted` pinning in place, which produces a working cluster that is wrong only in its reporting — invisible to any test that checks admission behaviour;
- that on a cluster at `Privileged` the `audit` denial counter **does not increment** when a `restricted`-violating Pod is created, and that `PodSecurityViolation` consequently cannot fire. This asserts the central trade of the enhancement rather than an incidental property, and it is the test that will look wrong to a reviewer who has not read [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in) — it needs a comment pointing there;
- that a Pod **denied under `enforce: restricted` still increments** `pod_security_evaluations_total{decision="deny",mode="audit"}`, asserted on the metric rather than inferred from the configuration. This is the assumption the `PodSecurityViolation` alert rests on, and with `audit` now always equal to `enforce` it is the *only* configuration in which the alert can fire at all — so its failure leaves the product with no proactive PSA signal anywhere while looking healthy;
- that a Namespace carrying a `pod-security.kubernetes.io/warn` label **still warns at that label's level** on a cluster configured to `Privileged`, since per-Namespace labels outrank the cluster-wide setting. This is the upgraded-cluster case in [Frozen `warn` labels outlive the noise they caused](#frozen-warn-labels-outlive-the-noise-they-caused), and it is worth a test precisely because it is the behaviour the change is expected to have removed and has not;
- that `status.podSecurityAdmission` is written by **`cluster-kube-apiserver-operator` and by nothing else**, asserted on the `managedFields` entry for the subtree rather than on its value. A test that fails when a second writer appears is worth more than one checking values: the subtree is the first thing ever written to `APIServerStatus`, so the next component wanting to report something about the API server will find a precedent and a convenient home, and a second manager fighting the observer over the same two fields would show up as flapping rather than as an error.

### Monitor tests for managed Namespaces

Retiring the syncer moves the assurance for OpenShift's own Namespaces from a runtime controller to CI: after release `n` nothing on a running cluster would notice a payload Namespace shipping without a label. The existing SCC labelling monitor test must not regress once the default posture is `privileged`, and a **new, required** monitor test must assert that every payload Namespace carries a `pod-security.kubernetes.io/enforce` label from its own manifest. Payload labels are static and ship in manifests, so a pre-merge check on the payload is the right place for it, and **it must be in place before the default changes in release `n`, not at graduation.** The test cannot cover OLM, which expresses its requirement through SCC configuration rather than Namespace labels; that exclusion has to be encoded rather than left implicit. Neither test says anything about user-created Namespaces, and nothing in CI can.

### Retiring the readiness controller

The controller's existing unit tests encode behaviour being removed entirely, so they are deleted rather than replaced. What needs testing is the removal itself and its blast radius:

- none of the six `PodSecurity*EvaluationConditionsDetected` conditions is present on the `kube-apiserver` ClusterOperator after the operator has started, **including on a cluster that had them before the upgrade** — the removal path is new code and is the part most likely to be forgotten;
- no metric with the `pod_security_readiness_` prefix is exported;
- the `PodSecurityReadinessEvaluationStale` alerting rule is absent from the operator's rendered assets, and `PodSecurityViolation` is still present and unchanged;
- the operator's RBAC on `apiservers/status` is `update` and `patch` only, with **no grant on `apiservers` itself**, asserted against the generated manifests rather than at runtime — the grant exists for the config observer's status write and must not widen into a grant over `spec`, which stays administrator-owned;
- the ClusterOperator does not go `Degraded` or `Progressing` as a result of the controller's absence, on a cluster where it previously ran.

The expected *fall* in reported violations should be anticipated rather than measured here: the conditions simply stop, so any dashboard reading them goes blank rather than to zero. That distinction has to be in the notice sent to their owners.

### Release transitions

**Upgrade `n-1` → `n`, in an upgrade lane.** The invariant is that **nothing observable changes**, which is an unusual thing to assert in an upgrade lane and is the reason to assert it explicitly: the gate is out of `Default` on both sides and the gate-off branch is not edited, so a `Default` cluster crosses the redefinition without noticing. The effective `enforce` level is `privileged` before and after, the advising syncer is running before and after, the readiness controller's six conditions are present before and after, and no Namespace label changes value. A lane that passes because it tested nothing looks the same as one that passes because nothing changed, so this is worth writing as positive assertions against the rendered configuration rather than as an absence of alerts.

**Upgrade `n` → `n+1`, in an upgrade lane.** This is the transition that carries the whole change, and the invariant is that it changes the default and nothing else: every Namespace carrying a syncer-written `enforce` label before the upgrade carries **the same value afterwards**, with the `managedFields` entry naming `pod-security-admission-label-synchronization-controller` intact and its `time` unchanged — snapshot before, compare after, because this is the audit record the whole retention argument rests on and a silent SSA prune would destroy it without any other test noticing. No Namespace *gains* a label; the syncer is not running afterwards; the six conditions are gone; and the transient in [Operator skew](#operator-skew) is benign in both orderings. **The rendered block does change and the lane must assert how**: `enforce` stays `privileged` while `warn` and `audit` move from `restricted` to `privileged`, exactly one revision is cut, and no Pod is rejected at any point during the rollout. Asserting "no change" here, which the earlier design allowed, would now fail for the right reason and be fixed in the wrong way.

**Downgrade cannot be covered in CI**, because y-stream downgrade is not a supported operation and no lane exists. Rather than leave the hazard untested, test the properties that make it safe, none of which needs a downgrade: that this enhancement writes **no condition anywhere**, asserted directly rather than inferred — on `apiserver/cluster`, on `kubeapiservers.operator.openshift.io` and on the ClusterOperator alike; that the gate-off path is byte-for-byte today's behaviour, per the first section of this plan, which is what makes `n+1` → `n` a return to a tested state rather than a step into an untested one; and, against the generated CRD manifests, that the `Default` manifest for release `n` has no `podSecurityAdmission` under `spec` while the `TechPreviewNoUpgrade` one does, that the `n+1` `Default` manifest does, and that none of them has anything under `status`. The remaining behaviour — the syncer and readiness controller resuming on `n`, and the [pruning](#schema-pruning-erases-the-configuration) of `spec.podSecurityAdmission` on the first write after the downgrade — is a QE procedure against a deliberately downgraded cluster. The pruning case needs a step that writes an unrelated field afterwards, because the erasure does not happen at downgrade time.

### Gate redefinition and HyperShift

The hazard in [Gate redefinition skew](#gate-redefinition-skew) is a management-cluster component applying new semantics to an old gate list, so it cannot be caught by any test that runs inside a single release. Two things need covering, both in `openshift/hypershift`:

- **A unit test over the CPO's PodSecurity rendering, parameterised by hosted release version**, that reproduces the table in that section row by row — a hosted version below `n` with `OpenShiftPodSecurityAdmission=true` in its gate list must render `restricted` and must keep the `cluster-policy-controller` deployment, and the same gate list at `n+1` must render `privileged`. A test that only exercises the current release passes under exactly the bug this guards against.
- **The sunset is asserted, not remembered.** The version gate should carry a test that fails once the minimum supported hosted release reaches `n`, so the dead code announces itself rather than waiting to be noticed.

A hosted-cluster e2e across a real management/hosted version boundary is the honest test and is likely out of reach; the unit tests above are the practical substitute and should say so.

**A test is owed for status written by an earlier payload being inherited on re-upgrade.** A cluster that reaches `n+1`, is downgraded to `n` without anything writing `apiserver/cluster`, and is then re-upgraded, arrives with a `status.podSecurityAdmission` that `n` never maintained and that may describe a level `n` was not enforcing. The observer must overwrite it on its first pass rather than trusting it, and the test must assert that rather than assert the field is absent.

## Graduation Criteria

**Graduation is a single event.** One gate carries the API, the posture change and both retirements, so there are no tiers and nothing lands ahead of anything else: `OpenShiftPodSecurityAdmission` returning to `Default` in release `n+1` is simultaneously the moment the field becomes generally available, the moment the global default becomes `privileged`, and the moment the syncer and the readiness controller stop running on a supported cluster. A reviewer looking for a feature gate protecting the two retirements will find this one.

Because one gate carries all three changes, **everything below is a prerequisite of the same promotion**, including the parts that have nothing to do with the API. A design splitting the work across two gates and two releases would let them graduate separately; the reasons for not doing that are in [Why one gate rather than two](#why-one-gate-rather-than-two).

### Prerequisites carried from the posture change

These gate the promotion even though they are not API work, and they are the long poles:

- **All payload workloads SCC-pinned and all payload Namespaces labelled**, demonstrated by the managed-Namespace labelling monitor test rather than by inspection. This is a hard prerequisite because once the default is `privileged` an unlabelled payload Namespace produces no signal on a running cluster, and it is the largest single body of work in this enhancement.
- **The existing SCC labelling monitor test does not regress.**
- **Advance notice to the owners of anything consuming the `PodSecurity*EvaluationConditionsDetected` conditions**, which is a prerequisite rather than a follow-up — see [What stops being reported](#what-stops-being-reported).
- **Resolution of the MicroShift question** in [Single-node Deployments or MicroShift](#single-node-deployments-or-microshift), because MicroShift reaches the syncer through the branch this enhancement must leave alone.

### Promoting the gate

`OpenShiftPodSecurityAdmission` moves from `TechPreviewNoUpgrade` back to `Default` and `OKD`. The field is on a type that is already `v1`, so **there is no version to promote**, no second version served in parallel, and no conversion to write — anyone who adopted it under TechPreview keeps exactly the objects they had. API validation integration tests land alongside the `openshift/api` change.

Promoting a gate into `Default` for the first time after nine releases in the preview feature sets is unusual, and reviewers will ask about it — including why `master` has it in `inDefault()` when no release branch does. Both answers are in [What actually ships](#what-actually-ships): the gate has never been enabled on a supported cluster, so there is no direction change to observe, and release `n` brings `master` into line.

### Dev Preview -> Tech Preview

This feature does not pass through Dev Preview. The gate is redefined and removed from `Default` in release `n`, which leaves the new behaviour reachable only in `TechPreviewNoUpgrade`. Entering Tech Preview requires:

- the `podSecurityAdmission` field merged in `openshift/api` gated on `OpenShiftPodSecurityAdmission`, with API validation integration tests, including that both the `spec` field and the `status` subtree are absent from the `Default` CRD manifest and present in the `TechPreviewNoUpgrade` one, and that `status.podSecurityAdmission` carries no condition types;
- the gate removed from `inDefault()` and `inOKD()` on `master` in the same change that redefines it, so that no payload ever ships the new meaning inside `Default` and `master` stops disagreeing with every release branch;
- **the gate-off path asserted to be byte-for-byte today's behaviour** — `privileged`, advising syncer, running readiness controller — which is what makes release `n` a no-op for `Default` clusters and is the single most important test in this enhancement, even though the design achieves it by not editing the branch at all;
- the Config Observer selecting level and version from `spec` when the gate is on, with the absent-field, absent-resource and clamped-version paths exercised;
- both retired controllers confirmed not running **when the gate is on**, and the withdrawn ClusterOperator conditions confirmed removed, per [Retiring the readiness controller](#retiring-the-readiness-controller);
- the dry-run sweep documented and collected by must-gather — the status answers what is configured, not whether the cluster can meet it, so the sweep remains the only readiness diagnostic and is a Tech Preview prerequisite rather than a GA one;
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

- user-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/) covering the opt-in workflow, the dry-run diagnostic, **labelling a Namespace that cannot meet the cluster-wide level**, diagnosing rejections after an opt-in, the break-glass procedure and the label-removal procedure — load-bearing rather than supplementary, because the status reports configuration rather than readiness and the documentation is the diagnostic. The labelling procedure is the one most easily overlooked, because it did not exist while the syncer was computing labels; after retirement it is the only way to run a cluster at `Restricted` that has any Namespace which cannot meet it, and without it the feature is all-or-nothing in practice;
- the dry-run sweep collected by must-gather;
- an answer to [Is a shell loop an adequate diagnostic](#is-a-shell-loop-an-adequate-diagnostic), and a hosted-cluster branch in the clamp support procedure, since [HyperShift reports no status](#hypershift--hosted-control-planes);
- the OKE question in [OpenShift Kubernetes Engine](#openshift-kubernetes-engine), where the alert and metric are unavailable.

### Removing a deprecated feature

This enhancement removes no deprecated API, and it **does** deprecate two pieces of Namespace metadata — see [Deprecating the SCC-mapping keys](#deprecating-the-scc-mapping-keys) for the schedule and the obligations.

It stops two behaviours. From `n+1` the SCC-mapping label syncer runs in **no shipped configuration**: not on standalone with the gate on, and not on MicroShift, which drops it through the absent arm. The `warn`/`audit` labels it wrote are left unmaintained and `MinimallySufficientPodSecurityStandard` is written and read by nothing. Separately, the `PodSecurityReadinessController` does not run where the gate is on, withdrawing six ClusterOperator conditions that are public surface today. There is no supported setting under which either returns, which is unusual enough that the release note must say it plainly.

**Neither is removed from the tree in `n+1`**, and the reason is release `n` rather than MicroShift. The gate-off `=false` arm is deliberately untouched and still constructs the advising syncer, so `NewAdvisingPodSecurityAdmissionLabelSynchronizationController` keeps its caller, the syncer keeps its code, and `securityv1.MinimallySufficientPodSecurityStandard` stays defined in `openshift/api` — release `n` writes it, clusters carry retained values under it, and deleting the constant would strand data support still reads. The same applies to the readiness controller, which the gate-off path still runs. All of it becomes deletable in `n+2`, once downgrade to `n` leaves support and the gate-off branch can go with it.

#### Deprecating the SCC-mapping keys

Because MicroShift drops the syncer too, **no shipped configuration runs the SCC-mapping syncer from release `n+1`**, and the two metadata keys it owns can be deprecated properly rather than scoped indefinitely:

- `security.openshift.io/MinimallySufficientPodSecurityStandard`, the annotation it writes.
- `security.openshift.io/scc.podSecurityLabelSync`, the Namespace label by which an administrator opts out of its management.

**Both are deprecated in `n+1` and removable in `n+2`, and the gap is not negotiable.** It exists because release `n`'s `=false` arm — the standalone gate-off path, deliberately untouched — still constructs the advising syncer, which still honours the opt-out label through `isNSControlled` and still writes the annotation. Note that the annotation write sits *outside* the `syncedLabels` loop ([`#L375-L380`](https://github.com/openshift/cluster-policy-controller/blob/master/pkg/psalabelsyncer/podsecurity_label_sync_controller.go)), so advising mode writes it as well; it is not confined to the enforcing constructor, and anyone reasoning from the label set alone will get this wrong. Release `n` is therefore a live writer of both keys and remains the downgrade target for `n+1`. Only once that window closes can the gate-off branch, the syncer and the constant all go.

| Release | `scc.podSecurityLabelSync` | `MinimallySufficientPodSecurityStandard` |
|---|---|---|
| `n` | honoured by the advising syncer on the gate-off path; honoured by MicroShift until it vendors the absent-arm change | written by the advising syncer and by MicroShift |
| `n+1` | **deprecated** — inert everywhere; setting it has no effect and neither does its absence | **deprecated** — written by nothing, read by nothing; retained values frozen and stale |
| `n+2` | removable with the syncer, once downgrade to `n` is out of support | constant removable from `openshift/api` on the same condition |

What deprecation obliges, beyond the release note:

- **A `// Deprecated:` marker on `securityv1.MinimallySufficientPodSecurityStandard`** in `openshift/api`, naming `n+1` and the reason. The constant itself stays until `n+2` because clusters carry retained values under it that support still reads.
- **Rewriting MicroShift's `docs/user/howto_pod_security.md`**, which currently instructs users to set the opt-out label and will be actively wrong. It is the only user-facing documentation of either key, and its replacement — label your own Namespaces — is the whole of a MicroShift user's migration path.
- **Not stripping retained values.** Neither key is cleaned up on upgrade, for the reasons in [Why no cleanup ships](#why-no-cleanup-ships): a stale annotation misleads only a human, who has `managedFields` and the release note to date it.

**An inert opt-out label is a worse failure mode than a removed one**, and it is the reason the release note must name the label explicitly. An administrator who sets `scc.podSecurityLabelSync=false` from `n+1` gets no error, no warning and no event — the write succeeds and nothing happens, which is indistinguishable from it having worked. There is no admission-time feedback to add, because the label is ordinary Namespace metadata that no component validates.

**And there are two audiences, not one.** The `"false"` case above is an administrator opting a Namespace *out*, and it becomes harmlessly redundant. The `"true"` case is a layered product opting an `openshift-` Namespace *in*, per [Opted-in `openshift-` Namespaces](#opted-in-openshift--namespaces), and it is the sharper of the two: those manifests stop getting a computed label *and* stop having their own `enforce` value force-overridden, so the effective level can move on reinstall even though nothing about the manifest changed. Every shipped manifest carrying `scc.podSecurityLabelSync: "true"` needs an owner to confirm its explicit `enforce` label is the level it actually wants, since from `n+1` that label is what it gets. The Windows Machine Config Operator manifest is the known instance and is unlikely to be the only one; **finding the rest is a layered-product survey this enhancement does not perform**, and it is recorded here as an obligation rather than a finding.

## Upgrade / Downgrade Strategy

### On Upgrade

#### Release plan

No change is backported to release `n-1`: it contains no consumer of the API. It does *not* follow that a stored value survives a downgrade — because the API is a gated field rather than a CRD, `n-1`'s schema prunes it. See [On Downgrade](#on-downgrade).

- **Release `n` — invisible to `Default` clusters.** Redefine `OpenShiftPodSecurityAdmission` and **keep it out of `inDefault()` and `inOKD()`**, where it already is not ([What actually ships](#what-actually-ships)), so the new meaning exists only in the two preview feature sets. Change the Config Observer's gate-on branch, change the syncer's `=true` arm, make the `PodSecurityReadinessController` conditional on the gate, and ship `spec.podSecurityAdmission` behind the same gate. A `Default` cluster upgrading into `n` sees none of it — and now genuinely so, because nothing on its branch is edited. Note that `master` must also be changed to *remove* the gate from `inDefault()` and `inOKD()`, where it currently sits; that is a change to `master` only, and it brings `master` into line with what every release branch already does.
- **Release `n+1` — the gate enters `Default` for the first time.** Add the gate to `Default` and `OKD`. The field becomes generally available, the syncer and the readiness controller stop running, and the six ClusterOperator conditions are withdrawn — in one upgrade. **The global default does not move**: it is `privileged` before and `privileged` after. No API version moves, because the field is already on a `v1` type.

**The release note therefore belongs to `n+1`, not `n`.** Everything in [Release note](#release-note) describes the upgrade into the release where the gate returns to `Default`. Release `n` needs no note beyond the Tech Preview announcement.

#### The state a cluster actually arrives in

**Upgrading `n-1` → `n` changes nothing**, because the gate is out of `Default` on both sides ([What actually ships](#what-actually-ships)): no revision is cut, no label moves, no condition is withdrawn. What follows is therefore the upgrade that matters — **`n` → `n+1`, when the gate enters `Default` for the first time in the product's history** — and applies unchanged to a cluster arriving from `n-1` by way of `n`.

Such a cluster arrives with `apiserver/cluster` already present, because the installer rendered it at install time, and with `podSecurityAdmission` unset, because the field only entered its schema at that moment. That is the state every upgraded cluster is in rather than an edge case, and an unset field resolves to the gate-implied default. The sequence is: `cluster-policy-controller` restarts with the label syncer not running; `cluster-kube-apiserver-operator` restarts without the readiness controller and clears the conditions it left behind; and nothing further happens until an administrator sets `enforceLevel`.

**The PodSecurity block does change, and a revision is cut**, which follows directly from `warn` and `audit` tracking `enforceLevel`. The gate-off branch the cluster is leaving renders `enforce: privileged` with `audit` and `warn` pinned at `restricted`; the gate-on branch it arrives at, with the field unset, renders all three at `privileged`. Two of the six keys therefore move, the observer produces a new `observedConfig`, and the static pod rolls.

What that costs is bounded, and the bound is the reason it is accepted. **Enforcement does not move** — `enforce` is `privileged` on both sides — so the rollout changes what the cluster reports, not what it admits, and no workload can be rejected as a result in either the mixed window or the end state. The rollout also lands during an upgrade, when the control plane is being replaced revision by revision anyway, so on a multi-master cluster it is absorbed rather than added. **On SNO it is still an API outage**, and it is now an outage attributable to this enhancement where previously there was none; that has to be in the upgrade documentation rather than discovered. Where the resource is genuinely absent, as on MicroShift, the established pattern for an optional config input is [`ObserveMinimumKubeletVersion`](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/pkg/operator/configobservation/node/observe_minimum_kubelet_version.go): log `NotFound` and leave the observed config alone.

All three changes are branches of one gate read at startup, so they take effect in whatever order the affected operators happen to restart; the orderings are enumerated in [Intra-rollout skew](#intra-rollout-skew) and none is harmful, but there is no sequencing guarantee to lean on and the design must not acquire one. **Two things change visibly on the day**: the six `PodSecurity*EvaluationConditionsDetected` conditions disappear from `oc get co/kube-apiserver`, and PSA warnings stop appearing on `oc apply` in Namespaces that carry no label of their own. The second is the one users will notice and the one they will not report, because it looks like an improvement; the [release note](#release-note) has to claim it deliberately rather than let it be found.

#### `unsupportedConfigOverrides` already blocks the upgrade

`targetconfigcontroller.manageKubeAPIServerConfig` layers `defaultconfig.yaml`, the authorization-mode override, `config-overrides.yaml`, `spec.observedConfig` and finally `spec.unsupportedConfigOverrides`, which is merged last and wins. On a cluster carrying the PSA override documented in the [original PSA enhancement](pod-security-admission.md), `enforceLevel` is therefore silently inert while `status` reports success.

No new condition is needed. library-go's `UnsupportedConfigOverridesController`, already run by the operator, sets `UnsupportedConfigOverridesUpgradeable=False` with reason `UnsupportedConfigOverridesSet` whenever the field is non-empty, enumerating the overridden leaf paths. What it does not say is that this is why the requested mode is being ignored; making that connection is a [support procedure](#psa-enforcement-configuration-appears-to-have-no-effect). **The status does not help here and is actively misleading**: the observer cannot see the override, so `status.podSecurityAdmission` reports the level it resolved while a different level is in force — the second of the gaps in [What `status` does not say](#what-status-does-not-say). Nothing in this enhancement detects, reconciles or removes the override.

### On Downgrade

Y-stream downgrade is not a supported OpenShift operation, so this describes recovery rather than a guarantee.

**Downgrading `n` → `n-1` is a non-event**, because release `n` changed nothing for a `Default` cluster ([What actually ships](#what-actually-ships)). There is no stored `podSecurityAdmission` to lose: the field was never in a `Default` cluster's schema.

**The downgrade that matters is `n+1` → `n`, and because the gate-off branch is untouched it does not re-enable enforcement.** An `n+1` cluster moving back lands on the gate-off branch, which is today's behaviour: `enforce: privileged`, advising syncer, readiness controller running. A cluster that had opted out with `Privileged` keeps `enforce: privileged` but **regains the `warn`/`audit` pinning at `restricted`**, so PSA warnings return on every `oc apply` across the fleet — benign, loud, and the first thing anyone will notice about the downgrade. **A cluster that had opted in to `Baseline` or `Restricted` silently loses its enforcement**, because `n`'s gate-off branch substitutes `privileged` unconditionally and the stored field is pruned from the schema. That is the dangerous direction: a loss of a security control rather than an unwanted imposition of one. It is the safer of the two possible failure modes, but it must still be in the release note.

Two second-order effects:

- **The advising syncer restarts, and advising mode prunes.** Going from "not running" back to advising means the syncer resumes applying `loggingLabels` under the field manager `pod-security-admission-label-synchronization-controller`. Server-side apply prunes fields a manager no longer sends, so on any Namespace where that manager owns an `enforce` label — which on a `Default` cluster is none, but on a cluster that passed through `TechPreviewNoUpgrade` or on MicroShift is not none — the `enforce` label is **dropped** on the next apply. A downgrade can therefore *remove* enforcement labels rather than restore them. The effect is bounded to the populations listed in [Labels on upgraded clusters](#labels-on-upgraded-clusters), and `shouldUpdate` means it happens lazily rather than at once, which makes it non-deterministic in timing.
- **`unsupportedConfigOverrides` is not needed on this path.** With gate-off producing `privileged`, there is no enforcement to hold off after a downgrade. The override — which pins the cluster at `n` until removed — is only relevant to a cluster that wants to *keep* enforcing across the downgrade, and it does work for that, since `n`'s observer overwrites the whole PodSecurity block and the override is merged afterwards.

Everything from here to the end of this section describes the `n+1` → `n` direction; `n-1` is named only where the schema differs.

#### Schema pruning erases the configuration

This is the one respect in which a dedicated CRD would have behaved better. A CRD and its stored objects survive a downgrade untouched; a *field* does not. Release `n`'s `Default` `APIServer` schema has no `podSecurityAdmission` — the gate is out of `Default` there, which is the whole point of release `n` — and a structural schema with `preserveUnknownFields: false`, which every `config.openshift.io` CRD has, prunes what it does not recognise. Two things follow and both need stating in the release note.

- **The administrator's recorded intent is destroyed, not merely ignored.** Re-upgrading to `n+1` does not bring it back: the field returns to the schema empty, and the cluster reads as never having opted in.
- **The timing is not the downgrade.** Pruning happens on write, and nothing on `n` writes `apiserver/cluster` as a matter of course. The field may sit in etcd, unserved and unreachable, until some unrelated action — a `tlsSecurityProfile` change, a GitOps re-apply, a storage migration — takes the subtree with it. A support engineer looking the day after the downgrade and a week after it may see different things.

**The status subtree is pruned too, and unlike `spec` nothing is lost by it.** Release `n`'s `Default` schema has neither field, so both go on the next write. That is the right outcome: a stale `status.podSecurityAdmission` describing a level `n` is not enforcing would be worse than its absence, and because `n` has no writer for it there is nothing to correct it. No *conditions* are involved — the status carries two scalar fields and no condition types — so the rule constraining where condition types may live, and the support procedure for removing ones stranded on an operator CR, are both still unnecessary. **That remains a benefit of keeping the status this small**, and it is the respect in which this is cheaper than a readiness-bearing status would have been.

Pruning is also the reason a downgrade cannot be made self-healing by the gate alone. The gate state and the stored field move together — `n` has neither — so there is no release in which the field survives but is ignored, which would have been the recoverable case.

The mitigation is documentation, not code: record `spec.podSecurityAdmission` before downgrading and re-apply it after re-upgrading. Preserving a field across an unsupported operation would mean a CVO-managed backup object or an annotation shadowing the field, and both are worse than the disease.

#### Per-resource downgrade matrix

Keyed on the downgrade that matters, `n+1` → `n`. Every row is the gate going from on to off.

| Resource | State on `n+1` | What `n` does to it | Administrator action |
|---|---|---|---|
| `observedConfig` → `admission.pluginConfig.PodSecurity.configuration.defaults` | all three levels from `spec`, or `privileged` throughout if opted out | the gate-off branch rewrites the block to `enforce: privileged` with `warn` and `audit` pinned back to `restricted`. **Every cluster rolls, including one that was at `Privileged`**, because the pinning returns; an opted-in cluster additionally loses enforcement | none, but expect PSA warnings to reappear fleet-wide on `oc apply` — the downgrade restores the noise this enhancement removed |
| Namespace `pod-security.kubernetes.io/warn`, `audit` | unmaintained but intact | the advising syncer resumes and re-owns both at the minimally sufficient level on every Namespace it controls | none |
| Namespace `pod-security.kubernetes.io/enforce` written by the *enforcing* syncer | present only on preview-feature-set clusters and MicroShift; absent on `Default` | **pruned by SSA** on the next apply, because advising mode no longer sends it | none on `Default`, where there are none; elsewhere, re-apply by hand if the label was wanted |
| Namespace `MinimallySufficientPodSecurityStandard` | stale, read by nothing | the advising syncer rewrites it | none |
| Pod `openshift.io/scc`, `validated-scc-subject-type` | set by SCC admission at creation | nothing; per-Pod and immutable after admission | none |
| `apiserver/cluster` `spec.podSecurityAdmission` | the administrator's recorded intent | pruned by `n`'s `Default` schema, on the next write by anyone | record before downgrading and re-apply after re-upgrading; nothing recovers it automatically |
| `PodSecurity*EvaluationConditionsDetected` on `co/kube-apiserver` | removed by `n+1`, absent | the readiness controller starts and writes them again | none; they return by themselves |

**Every row is either benign or recovered by `n` resuming what it used to do.** Leaving the gate-off branch untouched is what makes that true: a downgrade is not a partial rollback into a state nobody tested but a return to the one that has shipped since 4.12. The advising syncer and the readiness controller both come back, with their labels, annotations and conditions.

Two rows do not recover, and they are the ones to read the table for. `spec.podSecurityAdmission` has no owner on `n` to rewrite it. And a cluster that had **opted in** loses its enforcement on the way down — silently, because the pruning that destroys the record and the substitution that lowers the level are independent. For an opted-out cluster, which is the default and the common case, the downgrade genuinely is a version change rather than a posture change; for an opted-in one it is a posture change in the relaxing direction, and the release note must distinguish the two rather than describing the downgrade once.

Re-upgrading `n` → `n+1` is correspondingly dull. If the field was already pruned the cluster arrives with no recorded intent, which is safe; if it was not — because nothing wrote the object while on `n` — it returns intact and is acted on, so an administrator who downgraded away from an enforcing configuration gets it back without re-requesting it. That is correct but worth stating in the release note alongside the pruning case, since the two outcomes differ on nothing the administrator did.

**There is a stale status to reconcile on the way back up.** Release `n` has no writer for `status.podSecurityAdmission` and, until something writes the object, no schema that prunes it either, so a cluster can arrive back at `n+1` carrying a status written by its previous `n+1` payload — describing a level `n` was not enforcing. The observer must overwrite it on its first pass rather than treating it as current, which is the test owed in [Gate redefinition and HyperShift](#gate-redefinition-and-hypershift). The failure mode if it does not is narrow but nasty: an administrator reads a status agreeing with a `spec` that was pruned, and concludes the cluster is enforcing something it is not.

## Version Skew Strategy

Four skew windows matter, each a window in which two components disagree about the effective PSA level. A fifth, in which a cluster passed through release `n` too quickly for the readiness evaluation to complete, is closed by that evaluation no longer existing.

The second of the four is created by the design itself rather than by the product: redefining `OpenShiftPodSecurityAdmission` rather than adding a gate means the *same string* denotes different behaviour in different releases, and anything that reads the gate across a version boundary has to be told which release's meaning applies.

### Intra-rollout skew

Changing the PodSecurity configuration produces a new static pod revision, and revisions roll one master at a time. For minutes to tens of minutes one kube-apiserver enforces the new level while the others enforce the old one, and which one a Pod creation reaches is a load-balancer decision. A Deployment scaling from 0 to 5 can have some replicas admitted and some rejected. This window exists on every change, in both directions, and removing the interlock is what stopped it being safe by construction.

**Raising** enforcement is not interlocked against a clean evaluation, so nothing bounds the number of rejections during the mixed window: a raise on a cluster with violations produces a window in which the *same* Pod creation succeeds or fails depending on which master it reaches, which is worse than failing consistently because it looks like flakiness rather than policy. The window is bounded by the rollout and ends in the stricter state, but the administrator entering it has had no warning from the cluster at all — not an interlock, not a condition, not an advisory header — so whether they knew is entirely a function of whether they ran the dry-run. It is why the break-glass path has to be fast. **Lowering** enforcement, including break-glass, is likewise *not* instantaneous; support must be told to wait for every `currentRevision` to equal `latestAvailableRevision` rather than concluding the change did not take. On single-node deployments there is no mixed window, because there is only one kube-apiserver; there is an API outage instead while the static pod restarts.

### Operator skew

Retiring both controllers removes most of this problem, and reading `spec` directly removes the rest: the Config Observer's input is the administrator's own field rather than another controller's output, so there is no producer and consumer that can get out of step, and after release `n` the observer is the only component in the picture. `cluster-policy-controller` reads none of it.

What remains is a transient during the upgrade itself. `cluster-policy-controller` runs as a container in the kube-controller-manager static pod, owned by a different operator, with no ordering guarantee — so for part of every upgrade the kube-apiserver has already rolled to `privileged` while an `n-1` `cluster-policy-controller` is still stamping labels, or the syncer has already stopped while the kube-apiserver is still enforcing `restricted`. Neither is harmful: the first writes labels that are merely redundant and become the retained labels; the second changes nothing.

The enum is the sharper edge, and it applies to out-of-payload and hosted-control-plane consumers. A consumer from an earlier release that encounters an unknown value must **preserve its existing behaviour** — not error, and not treat the field as unset. Treating it as unset would silently relax enforcement; erroring would degrade a ClusterOperator over a value valid on the cluster's own API server. So consumers switch on the values they know and fall through to "make no change to the currently effective level", and the enum is only ever extended. Because the API server validating a write is always at least as new as any consumer, a value can be accepted by validation and be unknown to a consumer; that asymmetry is normal during an upgrade, not a bug to be designed out.

### Gate redefinition skew

A feature gate's name is normally a stable key: a component that reads `OpenShiftPodSecurityAdmission=true` can rely on it meaning the same thing in every release that has the gate. Redefining the gate breaks that, so this section states exactly where the breakage is reachable and what bounds it.

**It is unreachable on a standalone cluster.** The reader and the gate list ship in the same payload: `cluster-kube-apiserver-operator` and `cluster-policy-controller` are both rendered from the release the cluster is on, and `FeatureGate` status is reconciled by the CVO from that same release. There is no supported way to put a release-`n+1` reader in front of a release-`n-1` gate list. Intra-upgrade, the two can disagree for minutes — that is [Operator skew](#operator-skew) — but each component reads a list produced by a payload whose semantics it shares, because the gate's membership in `Default` changes in lockstep with its meaning.

**It is reachable in HyperShift, and only there**, because the reader and the gate list come from different releases by design. The mechanics, the exposure and the fix are in [HyperShift skew](#hypershift-skew).

This is the concrete cost of choosing one gate over two. A new gate name would have needed no version gate anywhere, because it has no old meaning to collide with — but it would have required editing the observer branch every supported cluster runs, which is the trade argued in [Add a new gate and retire the existing one](#add-a-new-gate-and-retire-the-existing-one). The cost is also smaller than it looks, since the population it protects is preview clusters alone and the version gate is consequently optional; see [HyperShift skew](#hypershift-skew).

### HyperShift skew

**HyperShift is the one place the [gate redefinition](#gate-redefinition-skew) is reachable**, because the reader and the gate list come from different releases by design. The `control-plane-operator` on the management cluster renders the hosted kube-apiserver's PodSecurity configuration by string-matching `OpenShiftPodSecurityAdmission=true` in the hosted cluster's rendered gate list (`.../v2/kas/config.go`), and the management cluster is required to be at or ahead of the hosted one. A CPO carrying the new semantics therefore meets gate lists written under the old ones:

| Hosted release | Gate in its `Default` list | Old meaning | A new-semantics CPO would render |
|---|---|---|---|
| `≤ n-1`, `Default` | `=false` | `privileged`, advising syncer | `privileged`, advising syncer — **correct** |
| `≤ n-1`, preview feature set | `=true` | enforce `restricted`, enforcing syncer | `privileged`, syncer stopped — **wrong** |
| `n`, `Default` | `=false` | — | `privileged`, advising syncer — correct |
| `≥ n+1` | `=true` | — | `privileged` or the configured level — correct |

**The skew is narrow, because the dangerous row is not the row hosted clusters are on.** A `Default` hosted cluster carries `=false` — `featureGate-Hypershift-Default.yaml` confirms it on every release branch — and a new-semantics CPO reading `=false` takes the else branch, which is unedited, so it renders exactly what the hosted cluster had. **The only wrong row is a hosted cluster running `TechPreviewNoUpgrade` or `DevPreviewNoUpgrade`**, where it is wrong in the dangerous direction: a silent reduction from `restricted` to `privileged` with the syncer stopped. The affected population is preview clusters already excluded from upgrades, not the supported fleet.

**The fix is a bounded version gate in the CPO, and it is optional rather than required.** The PodSecurity branch — and the equivalent decision in the hosted `cluster-policy-controller` deployment (`.../v2/clusterpolicy/component.go`) — can be keyed on the *hosted control plane's* release version rather than on the gate string alone: below `n`, use the old mapping regardless of the gate; at `n` and above, use the new one. The CPO already resolves the hosted release version and already version-gates other behaviour on it, so this is a known pattern rather than new machinery, and it **sunsets when the oldest supported hosted release reaches `n`**.

**Whether it is worth shipping at all is a live question for HyperShift reviewers.** It protects exactly one population: hosted clusters below release `n` running a preview feature set, which are unsupported for upgrade in the first place. Against that, it is permanent-looking code in a hot path that must be remembered and deleted. The honest options are to ship it for completeness, or to accept the preview-cluster exposure and document it. If it is shipped, the sunset must be a comment naming the release, because an unlabelled version gate is the kind of thing that survives for years.

Underneath the redefinition there is a standing skew about *configuration* rather than semantics, and it does not sunset. The skew is one-directional and can span several releases: a `control-plane-operator` at `n+3` may be rendering for a hosted cluster whose payload is at `n+1`. A hosted cluster's opt-in therefore cannot be expressed purely in the guest without the management side honouring it, and the management side is the one that may be arbitrarily newer. The surface itself is settled by the API being spec-only: `HostedCluster.spec.configuration.apiServer` is a `*configv1.APIServerSpec`, so `podSecurityAdmission` arrives on the management side with no new plumbing and nothing has to be reported back into the guest. What remains is an implementation obligation on HyperShift rather than an open design question — `control-plane-operator` must read the field, apply the same clamp against the *hosted* cluster's Kubernetes version rather than the management cluster's, and preserve its current behaviour for an `enforceLevel` value it does not recognise. **HyperShift reviewers are required to accept that obligation**, and the version gate above with it; together they are the main unresolved dependency here.

### EUS-to-EUS skip upgrades

This was a problem and is no longer one, which is worth recording because it was the readiness controller's sharpest skew hazard. The controller resynced every four hours with a throttled client, so a 5.0 → 5.1 → 5.2 skip could pass through 5.1 without ever completing a sweep — leaving no assessment available in the one release where the administrator was expected to make the opt-in decision.

With no evaluation, nothing is in flight to be interrupted. Enforcement follows `spec`, `spec` is carried across the skip untouched, and a cluster arrives at 5.2 enforcing exactly what was asked for. The gate is out of `Default` in both 5.1 and 5.2, so a cluster that has not opted in stays `privileged` throughout. The administrator passing through 5.1 has the same diagnostics as one sitting on it: the dry-run, whenever they choose to run it.

## Operational Aspects of API Extensions

### In general

- Administrators facing issues on a cluster set to a stricter level can change `enforceLevel` to `Privileged` to halt enforcement. Nothing can block that change, because nothing is consulted before it is applied.
- ClusterAdmins must ensure that directly created workloads (user-based SCCs) have correct `securityContext` settings. Updating default workload templates can help.
- **A violating Namespace keeps its workloads running but cannot replace them.** Raising the level evicts nothing, because PSA acts only on Pod creation — but the next time a Pod is recreated, by a node drain, reboot, MachineConfig rollout, eviction, scale-up or rolling update, the replacement is rejected and the Namespace quietly loses capacity. A container crash-looping inside an existing Pod does **not** trigger this, since the Pod object survives and no admission decision is taken. See [What opting in still risks](#what-opting-in-still-risks-with-nothing-evaluating).
- **PSA denials are not visible in `kubectl get pods`.** When a Pod is created by a controller the rejection surfaces on the owning object, because no Pod is ever created — on the ReplicaSet for a Deployment, on the Job for a CronJob. `kubectl -n $NAMESPACE get events --field-selector reason=FailedCreate` is the most common source of confusion when debugging PSA, and support should reach for it first.
- To identify specific problems in a violating Namespace, `kubectl label --dry-run=server --overwrite ns/$NAMESPACE pod-security.kubernetes.io/enforce=restricted`. From release `n` this is the only mechanism, for administrators and for support alike: it is also the answer to "what standard does this Namespace need?", which the platform no longer answers on its own. It reports only on Pods that exist at that moment.
- **`apiserver/cluster` answers what was asked for and what the observer resolved, never what every master is running.** `spec` is what somebody asked for, `status` is what the observer resolved from it — which catches a clamped version and nothing else. Support asking what a cluster is *actually* enforcing still reads the revisioned `config-<revision>` ConfigMap, because `status` predates the rollout, cannot see `unsupportedConfigOverrides` and says nothing about per-Namespace labels. All three can differ from each other, and no component on the cluster notices.

### Health and failure modes of the extension

The `podSecurityAdmission` field introduces no webhook, no aggregated apiserver and no controller, so it adds no latency to any request path and cannot make another resource unavailable. The status is written by the existing config observer on configuration change, not on a timer, so it adds no recurring write traffic — a cluster whose `spec` never changes sees one write at upgrade and none thereafter. Because it extends a resource every cluster already has, it adds no new watch, no new CRD and no new object for the CVO to manage. The write traffic it does add to `apiserver/cluster` is the smallest kind: edge-triggered on a configuration the administrator changes by hand, so typically one write at upgrade and none thereafter. The readiness-bearing design this replaces would have written on a four-hourly cycle whether anything had changed or not, waking every watcher of that resource each time; retiring the readiness controller removes the recurring write while keeping the subtree.

**The failure modes are the observer's, and there are only three.** The table is short because the readiness controller is gone, and that is worth reading carefully rather than as a simplification: every failure mode it had was a way the *evaluation* could fail, none of which could affect enforcement in either direction. Retiring a component whose failure modes were all benign shortens the table without making anything safer — what it removes is the *diagnosis* those rows described.

| Failure mode | Signal | Effect |
|---|---|---|
| field absent from the schema, or `apiserver/cluster` absent | none | the observer falls back to the feature-gate-implied default, `privileged` from release `n` |
| `spec.enforceVersion` newer than the payload | **none** | clamped down; the applied value is reported in `status.enforceVersion`, which differs from `spec` |
| `unsupportedConfigOverrides` set | ClusterOperator `Upgradeable=False`, reason `UnsupportedConfigOverridesSet` | `enforceLevel` is silently inert; the cluster cannot upgrade until the override is removed |

Two of those three signal nothing specific to PSA, which is the operational character of this feature: it is configured in one place and observed in another, and the connection between them is made by a person following a documented procedure.

Escalation goes to the OpenShift Auth team. Failures manifesting as admission rejections in `openshift-*` Namespaces are likely to involve OLM and the owning layered product.

## Support Procedures

The procedures themselves are written as `openshift-docs` modules rather than
carried here. This section records the set that has to exist, and the design
consequences the procedures are wrong without.

**No `openshift/runbooks` entry is needed.** This enhancement ships no alert, so
there is nothing to write one against.

| Procedure | Purpose | Destination |
|---|---|---|
| [PSA enforcement configuration appears to have no effect](#psa-enforcement-configuration-appears-to-have-no-effect) | the causes: `unsupportedConfigOverrides`, an unfinished rollout, retained labels, a clamped version | openshift-docs, support KCS |
| [Removing retained `enforce` labels](#removing-retained-enforce-labels) | the only mechanism by which a syncer-written label is ever removed; chiefly a MicroShift concern | **MicroShift docs**, openshift-docs, **linked from both release notes** |
| Break glass: disabling enforcement | `Privileged`, its rollout cost, and the operator-wedged fallback | openshift-docs, support KCS |
| Finding and resolving violating Namespaces | the dry-run sweep, then per-reason remediation, including `openshift-operators` | openshift-docs |
| Labelling a Namespace that cannot meet the cluster-wide level | the replacement for the retired syncer: choose a level with `--dry-run=server`, set `pod-security.kubernetes.io/enforce` by hand, and understand that the label then outranks the cluster-wide default permanently | openshift-docs, **MicroShift docs** |
| Diagnosing rejections after raising the level | `FailedCreate` on the owning object rather than `oc get pods`, the `PodSecurityViolation` alert and why its silence on an opted-out cluster means nothing, and the revert | openshift-docs, support KCS |
| PSA warnings on a cluster that has opted out | why a Namespace still warns when the cluster-wide level is `privileged`: a frozen label from the retired syncer, and how to remove it safely | openshift-docs, support KCS |
| The SCC-mapping keys mean different things on MicroShift | `scc.podSecurityLabelSync` and `MinimallySufficientPodSecurityStandard` are live on MicroShift and inert on standalone; every page or KCS article mentioning either must name the product | openshift-docs, MicroShift docs, support KCS |

**must-gather must collect the dry-run sweep**, and this is now a hard
requirement rather than a convenience. Nothing on the cluster records PSA
readiness: the status reports configuration rather than assessment, and there is
no condition, no metric, and no annotation since
`MinimallySufficientPodSecurityStandard` stopped being maintained. A support
bundle from a release `n` cluster that does not run the sweep at collection time
contains **nothing whatsoever** about whether the cluster can meet a standard.
It is `O(Namespaces)` serial API calls against a cluster that may already be
unhealthy, so it needs a bound and a partial-output marker, and the bundle must
record when it ran — the output is a point-in-time sweep, not cluster state.

### PSA enforcement configuration appears to have no effect

**Read `status.podSecurityAdmission` first, then stop trusting it.**
`spec` records what somebody asked for and `status` records what the operator
resolved — which is enough to catch a clamp and nothing else. `enforceLevel`
can be silently inert *while `status` reports it as applied*:
`unsupportedConfigOverrides` is merged last and wins, any per-Namespace label
outranks the cluster-wide default, and a rollout in progress means different
masters are enforcing different levels. The effective configuration is read from
the revision each master is actually running — `config-<revision>` in
`openshift-kube-apiserver`, whose `config.yaml` key holds JSON because the merge
encodes through `UnstructuredJSONScheme`. The `UnsupportedConfigOverridesSet`
condition enumerates the overridden leaf paths but does not say that this is why
`spec.podSecurityAdmission` is being ignored. A clamped `enforceVersion` is the
one case the status *does* settle on its own: compare `spec.enforceVersion`
against `status.enforceVersion` and a difference is the clamp.

### Removing retained `enforce` labels

**This is primarily a MicroShift procedure**, because MicroShift is where syncer-written `enforce` labels exist in quantity; on standalone it applies only to clusters that passed through a preview feature set, or to an `openshift-` Namespace that opted in with `scc.podSecurityLabelSync: "true"`. The destination in the table above should reflect that.

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
admission configuration, and the sweep. Symptoms and log lines for a failing
readiness controller are not owed, since it is no longer a running component.

## Infrastructure Needed

New periodic CI lanes are needed to satisfy the graduation criteria: the feature has to be exercised in both the `TechPreviewNoUpgrade` and `Default` Prow variants, across the provider, topology, architecture and network variants required by `dev-guide/feature-zero-to-hero.md`, at a frequency that reaches the required number of runs per platform before branch cut. The exact lane list follows from the reworked [Graduation Criteria](#graduation-criteria) and is not yet enumerated.

## Appendix: Extended Alternatives

The detail behind the `Shape of…` summaries in [Alternatives (Not Implemented)](#alternatives-not-implemented). Nothing here is a different decision from the main body; it is the comparison that produced it, kept out of the argument so that the argument reads.

### Extended: the `warn` and `audit` levels

Three arrangements were available once the levels stopped being pinned, and the chosen one is the most aggressive of them. It is argued in [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in) and [Alerts and metrics](#alerts-and-metrics).

#### Keep both pinned at `restricted`

The shipped behaviour. Every cluster, whatever it enforces, warns and audits against `restricted`, so the `audit` denial stream and `PodSecurityViolation` work everywhere and the readiness question has a continuous fleet-wide answer at no recurring cost. **Rejected because of what it does to users who never asked for PSA:** a cluster enforcing `privileged` still returns a warning on every `oc apply` of a Pod that fails `restricted`, so the product spends nine releases warning about a standard it has no plan to apply. It also produces the `Baseline` false positive described under the retired readiness controller — a cluster enforcing `baseline` is told about `restricted` violations it has deliberately accepted. **This is the fallback if the loss of the pre-flight signal proves unacceptable in review**, and reversing to it is a two-line change in the observer.

#### Split `warn` from `audit`

The middle ground, and the one most likely to be re-proposed: `warn` follows `enforceLevel` so the terminal noise tracks what the cluster actually applies, while `audit` stays pinned at `restricted` so the denial stream and the alert survive on every cluster. It gets most of the noise reduction — the complaint is about warnings, not about audit annotations nobody reads — while keeping the continuous readiness signal that [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in) now does without.

It is rejected on two grounds, and neither is overwhelming. **It reintroduces the disagreement the single field exists to prevent:** a cluster would audit against a standard it does not enforce or warn about, so `PodSecurityViolation` would again mean "would fail `restricted`" rather than "failed", and the `Baseline` over-report would survive in the alert while disappearing from the warnings — arguably the most confusing of the three outcomes, since the two channels would then disagree about the same Pod. **And it keeps an evaluation running on every cluster for a signal whose consumer is mostly Red Hat**, which is the same argument that rejected the readiness controller and [the enforcement-mode metric](#keep-a-minimal-enforcement-mode-metric); accepting it here while rejecting it there would be inconsistent.

**It remains the best available answer if the pre-flight signal has to come back**, because it is strictly cheaper than restoring the full pinning: it costs the audit annotations and nothing a user sees. A reviewer pressing on [Deciding whether it is safe to opt in](#deciding-whether-it-is-safe-to-opt-in) should be pointed here rather than at the readiness controller.

### Extended: the feature gating shape

All the arrangements below reach the same end state and differ in how many gates it takes and what has to be version-gated. The chosen one is argued in [Why one gate rather than two](#why-one-gate-rather-than-two).

**The criterion that decides between them is edit surface: how much of the code path that supported clusters actually run does the arrangement require touching?** Redefinition requires none. Every alternative requires some. Because there is no posture change — the default is `privileged` before and after — the axes that would otherwise separate these options, release count and posture sequencing, turn out not to discriminate.

#### Ship the posture change a release ahead of the API

**Foreclosed — there is no posture change to ship ahead.** The proposal is two gates over two releases: release `n` turns `OpenShiftPodSecurityAdmission` off with its existing meaning, and a new gate carries the field, graduating at `n+1`. The first step is a no-op, because the gate is already off on every release branch and has been since 4.12, so the arrangement collapses into [Add a new gate](#add-a-new-gate-and-retire-the-existing-one) and is not a distinct option.

The objection usually raised against it is worth answering anyway, because it recurs: that for the whole of release `n` a `Default` cluster would have enforcement off with **no supported way to turn it back on**. True, but that describes the state the product has shipped in for nine consecutive releases. It is an argument for shipping the API sooner, not for any particular gating shape.

#### Add a new gate and retire the existing one

One release, a new name: a new gate carries the field and both retirements, and `OpenShiftPodSecurityAdmission` is deleted in the same change rather than left inert. Nothing is redefined, so [Gate redefinition skew](#gate-redefinition-skew) disappears entirely and HyperShift needs no version gate at all. Promoting a brand-new gate to `Default` is also routine, where promoting this one — which has sat in the preview feature sets for nine releases while `master` claimed it was in `Default` — will attract questions for as long as anyone reads the git history.

The obvious objection — that it leaves a gate stranded in `Default`, read by nothing — does not apply, because the gate is not in `Default`. What would be left behind is an inert gate in two preview feature sets, and deleting it in the same change leaves nothing behind at all. It is rejected for two other reasons:

- **It requires editing the branch every supported cluster is on.** Deleting the gate means collapsing the observer's two branches into one, and that one must be the old *disabled* branch. Collapsing to the wrong one reinstates the fleet-wide `restricted` defect described in [The gate-off branch must not be touched](#the-gate-off-branch-must-not-be-touched), from the opposite direction. It also forces a decision about what `cluster-policy-controller`'s switch does with no gate to read, with MicroShift's `default:` arm immediately adjacent. Redefinition touches neither.
- **It needs two gates alive at once in release `n`.** A preview cluster would carry the old gate — enabled, meaning `restricted` and an enforcing syncer — and the new gate, enabled and meaning the opposite. They contradict, so the deletion cannot be deferred to `n+1`, and release `n` becomes an addition and a deletion that must be sequenced against each other rather than a single change of meaning.

It remains the fallback if the HyperShift version gate proves unacceptable to its owners — a weak reason to reach for it, since [HyperShift skew](#hypershift-skew) makes that version gate optional in the first place.

#### Make the syncer's mode explicit rather than gate-derived

Not an alternative to the gating shape so much as a change the gating shape keeps deferring, recorded here because every arrangement above leaves it in place. Standalone wants the SCC-mapping syncer advising and then not running at all; MicroShift wants it enforcing, permanently. Those are divergent long-term requirements expressed through a shared feature-gate read, and the three-arm switch in `runPodSecurityAdmissionLabelSynchronizationController` is the result — with MicroShift's requirement encoded as the *absence* of a gate rather than as a statement of intent. A flag on `cluster-policy-controller`, set explicitly by each consumer, would let both products say what they want and would remove the sharpest remaining edge in this enhancement. It is out of scope here because it is a `cluster-policy-controller` interface change needing its own review, but it should be proposed separately rather than forgotten.

### Extended: the API shape

- **A dedicated cluster-scoped `PSAEnforcementConfig` CRD.** It survives a downgrade intact rather than being pruned, recorded as a [Drawback](#drawbacks) of what was chosen. Three things moved the API onto `APIServer`. It is already the cluster-wide API server configuration surface — `enforce` is a kube-apiserver admission plugin setting, and the resource already carrying `audit`, `encryption` and `tlsSecurityProfile` is where a reader looks for one more; a separate CRD would have been a *fourth* place holding PSA state. It is already consumed by the config observers this enhancement extends, and installer-rendered, so there is no "singleton absent" case outside MicroShift. And HyperShift's plumbing already exists for the spec: `HostedCluster.spec.configuration.apiServer` is a `*configv1.APIServerSpec` ([`hostedcluster_types.go`](https://github.com/openshift/hypershift/blob/main/api/hypershift/v1beta1/hostedcluster_types.go)). **The status weakens this third argument rather than defeating it**: a CRD would have reached a hosted cluster through no existing plumbing at all, whereas `APIServer` reaches it for the spec and leaves only the status unsolved — a smaller gap, but no longer no gap, per [Hypershift / Hosted Control Planes](#hypershift--hosted-control-planes). The second argument against a CRD, that it would have become partly operator-owned, has **lapsed in the other direction**: `APIServer` is now partly operator-owned too, so that distinction no longer separates the two options.
- **Ship no `status` at all.** The narrower shape: `spec` only, with the applied configuration readable solely from the operator-owned `config-<revision>` ConfigMap. It avoids making `cluster-kube-apiserver-operator` a writer of a `config.openshift.io` resource, avoids the `apiservers/status` RBAC, avoids the first-writer precedent on `APIServerStatus`, avoids two rows in the downgrade matrix, and — the largest of these — avoids owing HyperShift a status surface it does not have. **Rejected because the clamp has nowhere else to live:** a silently clamped `enforceVersion` is a security control quietly weakened, and leaving its only trace in a revision-suffixed ConfigMap makes "what am I enforcing?" a support question rather than an administrator one. It is also, in effect, what HyperShift gets: [no status is reported on hosted clusters](#hypershift--hosted-control-planes), so this alternative describes the shipped behaviour there and remains the fallback if the precedent on `APIServerStatus` proves unacceptable to API review.
- **Spec on `APIServer`, status on `operator.openshift.io/v1 KubeAPIServer`.** Live again now that there is a status, and the main alternative to writing `APIServerStatus`. It avoids the first-writer precedent entirely, since `KubeAPIServer` is already operator-owned and already written by this operator — no new RBAC, no `config.openshift.io` ownership question. Rejected because splitting intent and in-force state across two resources is exactly the comparison the status exists to support: an administrator detecting a clamp would read `spec` on one resource and `status` on another, which is worse than the problem being solved. **It does not help HyperShift either**, since a hosted cluster's `KubeAPIServer` lives on the management side.
- **Document `unsupportedConfigOverrides` and ship nothing.** Zero API surface, and it is the recovery path release `n` relies on already. Rejected in principle because an unsupported override is not an acceptable long-term answer for a supported posture decision, but the comparison should be written out.

### Extended: the enforcement decision

**Gate the raise on a clean evaluation.** Apply `Baseline` or `Restricted` only once the evaluation reports no violations, with an `acknowledgeKnownViolations` escape hatch keyed to a hash of the violation set, and an `EnforcementBlocked` condition and alert when the raise is withheld. **This is the alternative with the strongest safety story and the one most likely to be re-proposed in review.** It is rejected because it protected the wrong thing — it gated the global default, which governs only Namespaces with no `enforce` label — because a point-in-time sweep with known blind spots becomes a liability the moment a false positive stops an administrator configuring their own cluster, because it had no expression in HyperShift, where `ClusterConfiguration` carries specs only, and because it was most of the API's complexity. It is now additionally foreclosed: with the readiness controller retired there is no evaluation to gate on, so re-proposing it means re-proposing [Keep the readiness controller](#keep-the-readiness-controller) first.

**Report the result in `status`**, either as a `violatingNamespaces` list naming each Namespace and the standard it fails, or through the existing `PodSecurity*EvaluationConditionsDetected` ClusterOperator conditions. The list is what an administrator actually wants, but it has to be bounded, and a truncated list is indistinguishable from a short one on exactly the large clusters where the names matter most; the conditions are not co-located with the `spec` field they describe, and are themselves being [withdrawn](#what-stops-being-reported). Both are foreclosed with the evaluation that fed them.
