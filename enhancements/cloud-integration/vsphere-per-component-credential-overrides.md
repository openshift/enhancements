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
  - None
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
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes a vSphere workflow that runs CCO in
`credentialsMode: Manual`. After `openshift-install create manifests`, an
administrator identifies the generated `CredentialsRequest` targets and supplies
least-privilege component-scoped credential material using each target's existing
contract. CCO does not mint, copy, synchronize, or rotate those credentials. The
administrator owns their day-2 lifecycle: adding credentials, rotating one
component at a time, verifying the affected component, and retiring superseded
vCenter identities. The feature targets OpenShift 5.1.0.

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
- Provide an administrator-owned installation procedure, beginning after
  `openshift-install create manifests`, for satisfying each relevant vSphere
  `CredentialsRequest` target with component-scoped credentials.
- Document a day-2 lifecycle for creating, rotating, and verifying component
  credentials without requiring CCO to mint, copy, or rotate them.
- Preserve the existing Manual-mode responsibility for administrators to review
  credential requirements when a release changes.

### Non-Goals

- Adding installer-native `install-config.yaml` fields, automatic credential
  provisioning, or automatic credential rotation.
- Having CCO mint, copy, synchronize, or otherwise reconcile component Secret
  data in `credentialsMode: Manual`.
- Extending this workflow to non-vSphere platforms.
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

Credential values are not written to logs, examples, support bundles, or
documentation.

#### Day-2 credential lifecycle

1. To add a component credential, the administrator identifies the component's
   existing `CredentialsRequest` target, creates an appropriately scoped vCenter
   identity, and supplies the credential material using the same supported target
   contract used at installation.
2. To rotate a credential, the administrator first prepares replacement vCenter
   credentials, updates only the affected component's credential material, and
   verifies that component before revoking the previous identity. The exact
   update operation is intentionally not prescribed here because this proposal
   does not establish a vSphere-specific Secret name, command, or replacement
   mechanism.
3. Verification includes confirming the component's normal health signals and
   relevant vCenter audit activity without exposing credential values.
4. When a release adds or changes a vSphere `CredentialsRequest`, the
   administrator reviews the new requirement and supplies or updates the
   component credential before allowing the normal Manual-mode upgrade process.

### API Extensions

Not applicable: this enhancement adds no extension. The workflow uses the
existing `CredentialsRequest` targets generated for the release and does not add
a new resource.

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
SNO-specific resource requirement. MicroShift is unsupported because the
Manual-mode workflow is not defined as part of the MicroShift contract.

#### OpenShift Kubernetes Engine

There is no OKE-specific implementation. Product support for an OKE offering
must be confirmed before this vSphere workflow is enabled there.

### Implementation Details/Notes/Constraints

**Changed components:**

- Installation and day-2 documentation — describe the administrator workflow
  that begins after manifest generation and uses the target contracts already
  produced for the release.
- Tests — cover the Manual-mode installation, rotation, verification, and
  upgrade procedures.

**Credential ownership and authorization:**

The administrator owns the external vCenter identities and the in-cluster
component credential material. Access to update component credential material
must be limited to cluster administrators or explicitly delegated
credential-management identities. Any delegation must be narrowly scoped and
documented as privileged access. This enhancement does not prescribe Secret
names, namespaces beyond what generated target references require, or a new RBAC
rule set.

**Manual-mode boundary:**

In Manual mode, the administrator—not CCO—creates, updates, rotates, and
verifies component credentials. CCO must not fall back to a root credential or
silently replace a component credential.

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

### Drawbacks

- Credential management moves administrative work outside CCO. Administrators
  must prepare credentials before installation and maintain them for day-2
  changes and upgrades.
- Component isolation increases the number of vCenter identities and credential
  updates to manage.

## Alternatives (Not Implemented)

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

## Open Questions

The following items require resolution before user-facing documentation can
describe them as supported behavior:

1. What supported vSphere procedure or tooling produces and updates the exact
   target credential material after manifests are generated and during rotation?
2. Which component health signals and vCenter audit records are the supported
   verification evidence for each component credential change?
3. What narrowly scoped delegation model is supported for administrators who
   manage component credential material?

## Test Plan

### Unit Tests

- Verify that CCO in `credentialsMode: Manual` does not mint, copy, synchronize,
  rotate, or fall back to root credentials for component credential data.

### Integration / E2E Tests

- On a supported vSphere cluster, generate manifests, supply component-scoped
  credentials for the generated targets, and verify a successful Manual-mode
  installation without CCO credential management.
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

- Unit and integration coverage confirms that CCO does not manage component
  credential data in Manual mode.
- E2E coverage demonstrates installation from generated manifests with
  administrator-supplied component credentials and a verified single-component
  rotation.
- Administrator and support documentation describes installation, privileged
  access, rotation, verification, and remediation without exposing credentials.

### Tech Preview -> GA

- Automated installation, upgrade, downgrade, and long-running credential
  rotation coverage demonstrates the supported Manual-mode behavior.
- At least one release cycle of supported use shows no unresolved credential
  lifecycle regressions.
- OpenShift documentation includes supported topologies, privilege boundaries,
  Manual-mode installation, day-2 rotation, verification, and remediation.

### Removing a deprecated feature

No new user-facing resource is introduced. If this workflow is replaced, a
future enhancement must define migration of credential-management procedures and
any applicable deprecation period before removal.

## Upgrade / Downgrade Strategy

This enhancement does not introduce a new release gate or change the existing
Manual-mode upgrade policy. Before a supported Manual-mode upgrade, an
administrator reviews the `CredentialsRequest` requirements in the target
release, creates or updates the required external vCenter identities and
component credential material, and performs the existing Manual-mode upgrade
acknowledgement procedure. This enhancement does not prescribe its exact
annotation, command, or tooling.

Before a downgrade, the administrator verifies that every affected component has
supported credentials and follows the documented Manual-mode procedure for the
target release. Downgrade tests are required before Tech Preview promotion.

## Version Skew Strategy

No new cross-component protocol is introduced. Consuming components continue to
use their own `CredentialsRequest` targets and do not depend on CCO copying
credentials in Manual mode. Release-specific target changes are handled by the
administrator as part of the documented Manual-mode upgrade procedure.

## Operational Aspects of API Extensions

Not applicable: this enhancement adds no extension. Administrators use the
affected component's supported health signals and relevant vCenter audit activity
to verify credential changes without exposing credential data.

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
5. Escalate unresolved target-format, verification, or delegation questions to
   the vSphere and CCO owners rather than inventing a Secret name or command.

## Implementation History

- 2026-08-17: Initial annotation-based enhancement proposal.
- 2026-09-11: Revised the proposal during design discussion.
- 2026-09-14: Removed the obsolete association design and documented the
  administrator-managed Manual-mode installation and day-2 credential workflow.
- Target release: 5.1.0.
- Primary tracking: [SPLAT-2874](https://issues.redhat.com/browse/SPLAT-2874);
  CCO implementation: [SPLAT-2889](https://issues.redhat.com/browse/SPLAT-2889);
  parent initiative: [SPLAT-2724](https://issues.redhat.com/browse/SPLAT-2724).
