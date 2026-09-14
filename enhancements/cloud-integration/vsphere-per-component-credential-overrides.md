---
title: vsphere-per-component-credential-overrides
authors:
  - "@rvanderp3"
reviewers:
  - "@jcpowermac"
  - "@dlom"
  - "@jstuever"
  - "@patrickdillon"
approvers:
  - "@dlom"
  - "@jstuever"
  - "@patrickdillon"
api-approvers:
  - "@everettraven"
creation-date: 2026-08-17
last-updated: 2026-09-14
tracking-link:
  - https://issues.redhat.com/browse/SPLAT-2874
  - https://issues.redhat.com/browse/SPLAT-2889
  - https://issues.redhat.com/browse/SPLAT-2724
see-also:
  - "/enhancements/installer/credentials-management-outside-openshift-cluster.md"
replaces:
  - []
superseded-by:
  - []
---

# vSphere Per-Component Credential Overrides in CCO

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] API review is complete and the approved `openshift/api` change is linked
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement keeps the cluster-scoped `cloudcredential.openshift.io/v1alpha1`
`VSphereComponentScopedCredential` API in scope while using CCO in
`credentialsMode: Manual`. On vSphere, an administrator supplies the
component-scoped credentials required by generated `CredentialsRequest` targets
after `openshift-install create manifests`, rather than CCO minting, copying, or
rotating them. The administrator also owns the day-2 credential lifecycle:
creating least-privilege vCenter identities, updating the corresponding component
credentials, verifying the affected component, and retiring superseded identities.
The precise Manual-mode consumer and status semantics of the proposed CRD remain
open for API and CCO design review. The feature targets OpenShift 5.1.0 and is
initially gated by `TechPreviewNoUpgrade`.

This is a reduced-scope first phase of the broader vSphere multi-account
credential-management initiative. [SPLAT-2874](https://issues.redhat.com/browse/SPLAT-2874)
is the primary tracking item; [SPLAT-2889](https://issues.redhat.com/browse/SPLAT-2889)
tracks the CCO implementation, and [SPLAT-2724](https://issues.redhat.com/browse/SPLAT-2724)
is the parent initiative.

## Motivation

vSphere clusters today commonly use one credential for several OpenShift
components that interact with the vCenter API. This shared-credential model
creates operational and security challenges:

- **Blast radius**: A compromised credential exposes all vSphere operations, not
  just the affected component.
- **Audit opacity**: vCenter audit logs cannot distinguish which OpenShift
  component performed an action when all components use the same identity.
- **Rotation risk**: Rotating one shared credential requires coordinated changes
  across components and makes partial failures harder to isolate.

### User Stories

**Story 1 - Least-privilege credential separation**: As a cluster administrator,
I want to prepare a distinct vSphere identity and component credential for each
OpenShift component so that each component has only the permissions it requires.

**Story 2 - Installation in Manual mode**: As a cluster administrator, I want to
provide the component credentials after generating installation manifests so that
the cluster can install with CCO in `credentialsMode: Manual` and without a
shared in-cluster administrative credential.

**Story 3 - Safe day-2 rotation**: As a cluster administrator, I want to rotate
one component credential, verify that component, and then retire its old vSphere
identity so that routine rotation has a limited blast radius.

### Goals

- Run CCO in `credentialsMode: Manual` for this vSphere workflow.
- Provide an administrator-owned install-time procedure, beginning after
  `openshift-install create manifests`, for satisfying each relevant vSphere
  `CredentialsRequest` target with component-scoped credentials.
- Keep `VSphereComponentScopedCredential` as the proposed structured,
  cluster-scoped API for the component-credential association without placing
  credential bytes in the API object.
- Document a day-2 lifecycle for creating, rotating, and verifying component
  credentials without requiring CCO to mint, copy, or rotate those credentials.
- Preserve the existing Manual-mode responsibility for administrators to review
  credential requirements when a release changes.

### Non-Goals

- Adding installer-native `install-config.yaml` fields, automatic credential
  provisioning, or automatic credential rotation.
- Having CCO mint, copy, synchronize, or otherwise reconcile component Secret
  data in `credentialsMode: Manual`.
- Extending this API to non-vSphere platforms.
- Supporting HyperShift / Hosted Control Planes or MicroShift in this phase.
- Changing the vSphere cloud-provider configuration (`cloud-provider-config`
  ConfigMap).

## Proposal

### Workflow Description

CCO is configured with `credentialsMode: Manual`. It does not create component
credentials from a root credential and it is not the mechanism that applies or
rotates component Secret data.

#### Installation

1. The cluster administrator runs `openshift-install create manifests`.
2. The administrator uses the generated vSphere `CredentialsRequest` information
   to identify the component targets that need credentials. The target names,
   namespaces, Secret key format, and any release-specific requirements come from
   those generated manifests and the supported vSphere installation procedure;
   this enhancement does not assign new names or formats.
3. For each target, the administrator creates or selects a vCenter identity with
   the least privilege appropriate to that component, outside the cluster.
4. The administrator adds the corresponding component-scoped credential material
   to the installation manifest set in the form required by that target, then
   continues the supported installation workflow.
5. If the final API contract requires a `VSphereComponentScopedCredential`, the
   administrator supplies that resource as part of the documented workflow. The
   relationship between that resource and Manual-mode credential material is not
   yet defined by this proposal and is an open question below.

Credential values are never stored in `VSphereComponentScopedCredential` objects
or written to logs, examples, support bundles, or documentation.

#### Day-2 credential lifecycle

1. To add a component credential, the administrator identifies the component's
   existing `CredentialsRequest` target, creates an appropriately scoped vCenter
   identity, and supplies the credential material using the same supported target
   contract used at installation.
2. To rotate a credential, the administrator first prepares replacement vCenter
   credentials, updates only the affected component's credential material, and
   verifies that component before revoking the previous identity. The exact
   update operation is intentionally not prescribed here because the current
   proposal does not establish a vSphere-specific Secret name, command, or
   replacement mechanism.
3. Verification includes confirming the component's normal health signals and
   relevant vCenter audit activity without exposing credential values. Where a
   final CRD contract defines status or conditions, the administrator also uses
   those documented signals.
4. When a release adds or changes a vSphere `CredentialsRequest`, the
   administrator reviews the new requirement and supplies or updates the
   component credential before allowing the normal Manual-mode upgrade process.

### API Extensions

This enhancement retains the proposed cluster-scoped
`VSphereComponentScopedCredential` CRD in the
`cloudcredential.openshift.io/v1alpha1` API. Its purpose is to provide a
structured component-credential association without placing credential bytes in
the resource. The current proposal shape includes a target Secret reference and
a source Secret reference, but the API review must resolve whether a source
reference is meaningful in a Manual-mode workflow.

The following API intent remains in scope:

| API area | Intended constraint |
|---|---|
| Target reference | Identifies the vSphere `CredentialsRequest` target associated with the component. |
| Source reference | Present in the current proposal, but its Manual-mode meaning and namespace contract remain unresolved. |
| Credential data | Must not be embedded in the CRD. |
| Access | Creation and modification are privileged credential-management operations. |

The final `openshift/api` type and CRD review must define the schema, validation,
immutability rules, any admission dependency, the consumer of the CR, and any
status or condition contract. This enhancement does not claim that CCO reads a
source Secret, writes a target Secret, exposes particular conditions, or performs
validation while operating in Manual mode. An approved `openshift/api` change and
its API-review link are prerequisites before implementation proceeds.

`v1alpha1` is Compatibility Level 4 and will be delivered only when the
`TechPreviewNoUpgrade` feature set is enabled. The CRD and any associated
implementation artifacts must carry the corresponding feature-set gating.

### Topology Considerations

#### Standalone Clusters

The proposed workflow applies to supported standalone vSphere OpenShift clusters
using CCO in Manual mode. It adds no node workload or node-count dependency; the
administrator provides credentials for the generated component targets.

#### HyperShift / Hosted Control Planes

<!-- The current template validator requires this legacy-cased heading string:
#### Hypershift / Hosted Control Planes
-->

HyperShift / Hosted Control Planes are unsupported in this phase. Their
management-cluster and hosted-control-plane credential topology requires a
separate design for ownership, credential placement, and upgrade behavior.

#### Single-node Deployments or MicroShift

OpenShift SNO is supported because this proposal adds no node-level agent or
SNO-specific resource requirement. MicroShift is unsupported because this CCO
API and its feature-gated control-plane dependencies are not part of the
MicroShift contract.

#### OpenShift Kubernetes Engine

There is no OKE-specific implementation. Availability is limited to supported
vSphere OpenShift deployments that include the required feature set; product
support for an OKE offering must be confirmed before enabling the feature there.

### Implementation Details/Notes/Constraints

**Changed components:**

- `openshift/api` — add the proposed cluster-scoped
  `VSphereComponentScopedCredential` type, CRD generation, compatibility-level
  markers, feature-gate metadata, and API review approval.
- CCO configuration and documentation — support and document this vSphere
  workflow with `credentialsMode: Manual`; CCO must not assume responsibility for
  minting or rotating component credentials.
- Installation and day-2 documentation — describe the administrator workflow
  that begins after manifest generation and uses the target contracts already
  produced for the release.
- Tests — cover the final API contract and the Manual-mode installation,
  rotation, verification, and upgrade procedures.

**Credential ownership and authorization:**

The administrator owns the external vCenter identities and the in-cluster
component credential material. Access to create or modify the proposed CRD and
to update component credential material must be limited to cluster
administrators or explicitly delegated credential-management identities. Any
delegation must be narrowly scoped and documented as privileged access. This
enhancement does not prescribe Secret names, namespaces beyond what generated
target references require, or a new RBAC rule set.

**Manual-mode boundary:**

In Manual mode, the administrator—not CCO—creates, updates, rotates, and
verifies component credentials. CCO must not fall back to a root credential,
silently replace a component credential, or imply that the proposed CRD causes
Secret reconciliation until the final API and consumer contract explicitly says
so.

**CRD relationship:**

The current proposal includes `targetSecretRef` and `sourceSecretRef`, but it
does not yet establish how those fields participate when credentials are supplied
by an administrator after manifest generation. The final design must either
define that relationship and its validation/observability or remove or revise
the unsupported field. No controller watch, queue behavior, condition reason, or
Secret synchronization algorithm is specified by this Manual-mode proposal.

### Risks and Mitigations

**Risk: An administrator supplies credentials to the wrong component target.**

Mitigation: the installation and day-2 procedures identify each target from the
generated `CredentialsRequest` material. Privileged access is limited to
credential-management identities, and verification is performed for the affected
component before the previous credential is retired.

**Risk: A component receives credentials with excessive privileges.**

Mitigation: administrators create distinct vCenter identities with permissions
appropriate to each component and use vCenter audit records and component health
signals to verify the result.

**Risk: Rotation interrupts a component.**

Mitigation: prepare replacement credentials first, update one component at a
time, verify it, and revoke the prior identity only after successful
verification. CCO does not make an automatic fallback decision in Manual mode.

**Risk: The proposed CRD implies behavior that is not implemented in Manual mode.**

Mitigation: API review must establish the CRD's consumer, validation, and status
contract before implementation. Until then, documentation must not promise CCO
Secret synchronization or named status conditions.

**Risk: Tech Preview APIs are used during a minor update.**

Mitigation: the CRD and its implementation are gated by `TechPreviewNoUpgrade`;
the documented support posture prohibits minor upgrades while it is enabled.

### Drawbacks

- Credential management moves administrative work outside CCO. Administrators
  must prepare credentials before installation and maintain them for day-2
  changes and upgrades.
- Component isolation increases the number of vCenter identities and credential
  updates to manage.
- The CRD cannot be treated as an automation contract until its Manual-mode
  consumer and validation behavior are approved.

## Alternatives (Not Implemented)

### Annotation-Based Secret Mapping

Selecting a target through annotations on arbitrary Secrets was rejected. It
lacks a structured API and makes validation, immutability, and ownership harder
to review. `VSphereComponentScopedCredential` remains the proposed explicit
association API, subject to the unresolved Manual-mode contract.

### Installer-Native Configuration

Adding per-component credentials to `install-config.yaml` is not proposed. The
chosen workflow deliberately begins after `openshift-install create manifests`,
where an administrator supplies material that satisfies the generated target
contracts. It does not define a new installer field, Secret name, or automatic
installer credential lifecycle.

### Single Multi-Key Secret

A single Secret with several components' credentials is not proposed because
separate component credential material permits better access separation,
independent lifecycle management, and clearer audit trails.

### CredentialsRequest Field Extension

Adding a component-override reference directly to `CredentialsRequest.spec` was
rejected because those objects are owned by consuming components. A dedicated
CRD keeps the proposed user-authored association separate from component-owned
requests, subject to finalizing its Manual-mode semantics.

## Open Questions

The following items require resolution before implementation or user-facing
documentation can describe them as supported behavior:

1. Which component, if any, consumes `VSphereComponentScopedCredential` when
   CCO runs in Manual mode, and at what point in installation or day-2 operation?
2. Does `sourceSecretRef` remain part of the API? If so, what does it reference
   in a Manual-mode workflow, who validates it, and what namespace and data
   contract apply?
3. What validation, immutability, status, condition, and observability behavior
   belongs to the CRD? The current repository evidence does not establish named
   conditions or a CCO reconciliation contract in Manual mode.
4. What supported vSphere procedure or tooling produces and updates the exact
   target credential material after manifests are generated and during rotation?
5. What API approval PR and final API approver decision define the approved
   `openshift/api` contract?

## Test Plan

### Unit Tests

- Verify the final `VSphereComponentScopedCredential` schema, validation, and
  authorization rules after API review; do not test unapproved status or consumer
  semantics as though they were a contract.
- Verify that CCO in `credentialsMode: Manual` does not mint, copy, synchronize,
  rotate, or fall back to root credentials for component credential data.
- Verify any eventual CRD consumer against the approved Manual-mode contract.

### Integration / E2E Tests

- On a supported vSphere cluster with `TechPreviewNoUpgrade`, generate manifests,
  supply component-scoped credentials for the generated targets, and verify a
  successful Manual-mode installation without CCO credential management.
- Rotate one component credential using the supported administrator procedure;
  verify the component and its relevant vCenter activity before retiring the old
  identity, and verify other component credentials are unaffected.
- Exercise invalid, missing, or insufficient administrator-supplied credential
  material and verify that the affected component exposes its supported failure
  signals without CCO replacing the credentials.
- Verify that release changes to vSphere `CredentialsRequest` requirements are
  detected and handled through the documented Manual-mode upgrade procedure.
- Verify SNO behavior and that HyperShift / Hosted Control Planes and MicroShift
  are omitted from the supported test matrix.

## Graduation Criteria

### Dev Preview -> Tech Preview

- An approved `openshift/api` type/CRD change, API approver decision, and final
  Manual-mode consumer contract are linked from this enhancement.
- The CRD and implementation artifacts are correctly gated by
  `TechPreviewNoUpgrade`.
- Unit and integration coverage validates the approved API contract and confirms
  that CCO does not manage component credential data in Manual mode.
- E2E coverage demonstrates installation from generated manifests with
  administrator-supplied component credentials and a verified single-component
  rotation.
- Administrator and support documentation describes installation, privileged
  access, rotation, verification, and remediation without exposing credentials.

### Tech Preview -> GA

- The API has an approved compatibility and feature-gate graduation plan,
  including any version promotion required from Compatibility Level 4.
- Automated installation, upgrade, downgrade, and long-running credential
  rotation coverage demonstrates the supported Manual-mode behavior.
- At least one release cycle of supported use shows no unresolved credential
  lifecycle regressions.
- OpenShift documentation includes supported topologies, privilege boundaries,
  Manual-mode installation, day-2 rotation, verification, and remediation.

### Removing a deprecated feature

This is a new API. If it is replaced, a future enhancement must define migration
of CRs and credential-management procedures, feature gating, and the API
deprecation period before removal.

## Upgrade / Downgrade Strategy

While the feature is in `TechPreviewNoUpgrade`, minor-version upgrades are not
supported. Before any supported Manual-mode upgrade, an administrator reviews
the `CredentialsRequest` requirements in the target release, creates or updates
the required external vCenter identities and component credential material, and
performs the existing Manual-mode upgrade acknowledgement procedure. This
enhancement does not prescribe its exact annotation, command, or tooling.

Before disabling the feature, removing its manifests, or attempting a downgrade,
the administrator must follow the final API's documented removal procedure and
verify that every affected component has supported credentials. An older CCO
cannot be assumed to consume or reconcile the proposed CRD. Downgrade tests are
required before Tech Preview promotion.

## Version Skew Strategy

The CRD and any implementation that consumes it must be introduced and gated as
one feature-set unit. Consuming components continue to use their own
`CredentialsRequest` targets and do not depend on CCO copying credentials in
Manual mode. The final API design must define how an older component or CCO
behaves when a new CR is present; this proposal does not claim an unsupported
cross-component protocol.

## Operational Aspects of API Extensions

The API's final operational behavior is intentionally pending API review. The
proposal does not introduce a CCO webhook, Secret watch, target-Secret write,
or named CRD status condition in Manual mode. Administrators use the affected
component's supported health signals and relevant vCenter audit activity to
verify credential changes, without exposing credential data.

If the final CRD adds validation, a consumer, conditions, metrics, or alerts,
the implementation and user-facing documentation must specify its failure modes,
SLIs, support ownership, and recovery steps before Tech Preview promotion.

## Support Procedures

1. Identify the affected component and its generated or release-specific
   `CredentialsRequest` target; inspect metadata and health signals without
   exposing credential values.
2. Verify that the vCenter identity used for that component has the intended
   scope and that its audit activity corresponds to the component's operation.
3. For an installation failure or day-2 rotation failure, correct or replace the
   administrator-supplied credential material using the supported target
   procedure, then verify the affected component before retiring any previous
   identity.
4. Confirm that the cluster remains in `credentialsMode: Manual`; CCO is not
   expected to create, replace, or rotate component credentials.
5. If the final CRD contract defines status or conditions, use only those
   documented signals. Until then, do not infer CCO synchronization from CRD
   presence.
6. Escalate unresolved CRD-consumer, validation, or target-format questions to
   the API and CCO owners rather than inventing a Secret name or command.

## Implementation History

- 2026-08-17: Initial annotation-based enhancement proposal.
- 2026-09-11: Revised the proposal to the `VSphereComponentScopedCredential`
  CRD contract.
- 2026-09-14: Revised the proposal for a Manual-mode vSphere workflow in which
  administrators supply installation and day-2 component credentials; retained
  unresolved CRD consumer and status behavior as open questions.
- Target release: 5.1.0.
- Primary tracking: [SPLAT-2874](https://issues.redhat.com/browse/SPLAT-2874);
  CCO implementation: [SPLAT-2889](https://issues.redhat.com/browse/SPLAT-2889);
  parent initiative: [SPLAT-2724](https://issues.redhat.com/browse/SPLAT-2724).
