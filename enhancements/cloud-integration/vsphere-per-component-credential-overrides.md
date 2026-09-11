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
  - "none"
creation-date: 2026-08-17
last-updated: 2026-09-11
tracking-link:
  - https://issues.redhat.com/browse/SPLAT-2874
  - https://issues.redhat.com/browse/SPLAT-2889
  - https://issues.redhat.com/browse/SPLAT-2724
see-also:
  - "/enhancements/cloud-integration/cloud-credentials.md"
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

This enhancement adds the cluster-scoped `cloudcredential.openshift.io/v1alpha1` `VSphereCredentialOverride` API for selecting a labeled Secret in `openshift-config` as the source credential for one vSphere `CredentialsRequest` target Secret. Today, vSphere clusters use a single shared credential (`kube-system/vsphere-creds`) for every component. The CRD lets authorized administrators configure distinct, lower-privilege credentials per component without storing credential bytes in the API object. The feature targets OpenShift 5.1.0 and is initially gated by `TechPreviewNoUpgrade`.

This is a reduced-scope first phase of the broader vSphere multi-account credential-management initiative. [SPLAT-2874](https://issues.redhat.com/browse/SPLAT-2874) is the primary tracking item; [SPLAT-2889](https://issues.redhat.com/browse/SPLAT-2889) tracks the CCO implementation, and [SPLAT-2724](https://issues.redhat.com/browse/SPLAT-2724) is the parent initiative.

## Motivation

vSphere clusters today operate with a single set of credentials shared across all OpenShift components that interact with the vCenter API. This means Machine API, the CSI driver, and Cloud Controller Manager all authenticate with the same identity. This shared-credential model creates several operational and security challenges:

- **Blast radius**: A compromised credential exposes all vSphere operations, not just the affected component.
- **Audit opacity**: vCenter audit logs cannot distinguish which OpenShift component performed an action when all components use the same identity.
- **Rotation risk**: Rotating the shared credential requires coordinated updates across all components simultaneously, increasing the risk of partial failures.

### User Stories

**Story 1 - Least-privilege credential separation**: As a cluster administrator, I want to assign distinct vSphere credentials to each OpenShift component so that each component operates with only the permissions it requires, reducing the blast radius of a credential compromise.

**Story 2 - Credential rotation without full cluster impact**: As a cluster administrator, I want to rotate one component's vSphere credential without affecting other components so that routine rotation is lower-risk and can be performed incrementally.

**Story 3 - Observable recovery**: As a cluster administrator, I want invalid or ambiguous override configuration to report a clear status without silently changing a component back to root credentials so that I can remediate the intended credential source safely.

### Goals

- Provide a cluster-scoped, structured API for mapping one vSphere `CredentialsRequest` target Secret to one source Secret in `openshift-config`.
- Validate mappings, protect the mapping target from mutation, and make invalid configuration observable through status conditions.
- Limit CCO source-Secret discovery to explicitly labeled Secrets and give only authorized administrators or delegated service accounts permission to create or update override resources and source Secrets.
- Preserve existing root-credential behavior for managed vSphere `CredentialsRequest` targets only when no override CR exists for that target.
- Reconcile create, update, deletion, and relevance-removal events through the existing `CredentialsRequest` work queue.

### Non-Goals

- Installer-native `install-config.yaml` support or automatic provisioning and rotation of per-component credentials.
- Applying or managing overrides when `credentialsMode: Manual` is configured.
- Extending this API to non-vSphere platforms.
- Supporting HyperShift / Hosted Control Planes or MicroShift in this phase.
- Changing the vSphere cloud-provider configuration (`cloud-provider-config` ConfigMap).

## Proposal

### Workflow Description

An authorized administrator creates a `VSphereCredentialOverride` after creating a source Secret in `openshift-config`. The source Secret has the label `cloudcredential.openshift.io/vsphere-override-source: "true"`; it is not selected by annotations. The Secret contains the complete vCenter credential data map required by the target, with non-empty matching `<vcenter-hostname>.username` and `<vcenter-hostname>.password` entries. Credential bytes are never placed in the override CR or in CCO logs.

```yaml
apiVersion: cloudcredential.openshift.io/v1alpha1
kind: VSphereCredentialOverride
metadata:
  name: openshift-machine-api-vsphere
spec:
  targetSecretRef:
    namespace: openshift-machine-api
    name: vsphere-cloud-credentials
  sourceSecretRef:
    name: vsphere-machine-api-creds
```

The API server validates the target reference at creation time against the inventory of vSphere `CredentialsRequest.spec.secretRef` values maintained by CCO. It rejects a target that does not exist or that names `kube-system/vsphere-creds`. CCO maintains that inventory as `CredentialsRequest` objects are created, updated, deleted, and at controller startup; the admission policy fails closed if that inventory is unavailable.

The CCO reconciler maps an override event to the `CredentialsRequest` whose `spec.secretRef` matches `targetSecretRef`. It then resolves exactly one applicable override:

1. In `credentialsMode: Manual`, CCO does not resolve, copy, or manage overrides or target Secrets. It reports `ManualMode` on existing overrides and leaves Manual-mode credential management unchanged.
2. In managed mode, no override CR for the target preserves the existing root path using `kube-system/vsphere-creds`.
3. Exactly one override causes CCO to read the named, labeled source Secret from `openshift-config`, validate its data, and synchronize that data to the target Secret.
4. A missing or invalid source Secret, a source Secret without the required label, or multiple overrides for the same target is fail-closed: CCO does not write root credentials or replacement data to that target. It records the applicable status condition and waits for remediation.
5. Deleting the only override CR requeues its target. In managed mode that is the no-override case, so the existing root path resumes. Deleting a source Secret while its override remains is not a no-override case and therefore remains fail-closed.

### API Extensions

This enhancement adds the cluster-scoped `VSphereCredentialOverride` CRD in the `cloudcredential.openshift.io/v1alpha1` API. The resource defines these structured fields:

| Field | Meaning |
|---|---|
| `spec.targetSecretRef.namespace` | Namespace of the vSphere `CredentialsRequest.spec.secretRef` target. |
| `spec.targetSecretRef.name` | Name of that target Secret. |
| `spec.sourceSecretRef.name` | Name of the source Secret in the fixed `openshift-config` namespace. |
| `status.conditions` | `Ready`, `SourceValid`, `TargetSynced`, and `Progressing` observations for this mapping. |
| `status.resolvedSource` | `Override` after a successful synchronization or `Unknown` otherwise; root use is not an override status. |

OpenAPI validation requires both references and valid Kubernetes names. A validating admission policy backed by the CCO-maintained vSphere target inventory verifies that `targetSecretRef` names an existing vSphere `CredentialsRequest` target and is not the root Secret. CEL validation makes `targetSecretRef` immutable (`self == oldSelf`); retargeting requires delete and recreate. It also rejects self-reference when the target is `openshift-config/<sourceSecretRef.name>`.

The controller validates source data at reconciliation time because Secret contents cannot be represented safely in this CRD. The source must exist, carry the dedicated label, and contain non-empty matching username/password entries for each vCenter key pair required by the vSphere credential format.

`v1alpha1` is Compatibility Level 4 and will be delivered only when the `TechPreviewNoUpgrade` feature set is enabled. The CRD, admission policy, and CCO manifests must carry the corresponding feature-set gating so the API is not installed or used outside that feature set.

This is a new OpenShift API and requires an `openshift/api` type/CRD change and API review before implementation proceeds. No API approval PR or API approver has been assigned at the time of this update; the real approved PR link is an explicit prerequisite and must be added when available.

### Topology Considerations

#### Standalone Clusters

Supported on managed vSphere standalone OpenShift clusters. CCO and the API operate in the cluster control plane; the feature does not add node workloads or depend on worker-node count.

#### HyperShift / Hosted Control Planes

<!-- The current template validator requires this legacy-cased heading string:
#### Hypershift / Hosted Control Planes
-->

Unsupported in this phase. HyperShift / Hosted Control Planes use a distinct management-cluster and hosted-control-plane credential topology. No CRD, policy, or source-Secret placement is defined for either cluster until a separate design establishes ownership, access boundaries, and upgrade behavior.

#### Single-node Deployments or MicroShift

OpenShift SNO is supported: the feature adds no node-level agent or SNO-specific resource requirement. MicroShift is unsupported because this CCO API and its feature-gated control-plane dependencies are not part of the MicroShift contract.

#### OpenShift Kubernetes Engine

There is no OKE-specific implementation. Availability is limited to supported vSphere OpenShift deployments that include the CCO vSphere actuator and the required feature set; product support for an OKE offering must be confirmed before enabling the feature there.

### Implementation Details/Notes/Constraints

**Changed components:**

- `openshift/api` — add the cluster-scoped `VSphereCredentialOverride` type, CRD generation, compatibility-level markers, feature-gate metadata, and the API-review approval.
- CCO API manifests — install the CRD and validating admission policy only under `TechPreviewNoUpgrade`.
- CCO credentials-request controller — maintain the vSphere target inventory, watch overrides and labeled source Secrets, serialize events through the `CredentialsRequest` queue, and update override status.
- CCO vSphere actuator — resolve the override only after validating its source and retain the existing root path only when there is no override.
- CCO RBAC and tests — scope CCO reads and verify both allowed and denied authoring paths.

**Authorization and Secret discovery:**

CCO uses the label selector `cloudcredential.openshift.io/vsphere-override-source=true` for its `openshift-config` Secret list and watch. Its RBAC grants only the required `get`, `list`, and `watch` access to Secrets in that namespace, with no source-Secret write permission; it separately retains only the target-Secret permissions required by existing `CredentialsRequest` reconciliation. Kubernetes RBAC cannot express a label selector, so the selector limits discovery while namespace-scoped least-privilege RBAC limits the API access boundary.

Only cluster administrators or explicitly delegated service accounts receive RBAC to create, update, or delete `VSphereCredentialOverride` resources and labeled source Secrets in `openshift-config`. Delegation must be implemented with narrowly scoped Roles/RoleBindings and documented as privileged credential-management access. It is not sufficient for a principal to have general application-namespace Secret permissions.

**Resolution algorithm:**

```text
reconcile CredentialsRequest:
    if credentialsMode is Manual:
        do not read source Secrets or write target Secrets
        set ManualMode on any matching override and return

    overrides = overrides whose targetSecretRef matches this request's secretRef
    if overrides is empty:
        use the existing root-credential path
        return
    if overrides has more than one item:
        set Ready=False, reason=DuplicateTarget on every conflicting override
        do not modify the target Secret
        return

    source = named Secret in openshift-config, selected with the dedicated label
    if source is missing:
        set Ready=False, SourceValid=False, reason=SourceSecretMissing
        do not modify the target Secret
        return
    if source lacks the label or has incomplete, empty, or unmatched data:
        set Ready=False, SourceValid=False, reason=SourceSecretInvalid
        do not modify the target Secret
        return

    synchronize source data to the target Secret
    set Ready=True, SourceValid=True, TargetSynced=True, reason=OverrideApplied
```

**Event handling and serialization:**

Override create, update, and delete events map to the `CredentialsRequest` that owns `targetSecretRef`, rather than directly writing a target Secret. Labeled source-Secret events first map to referencing overrides and then to their target `CredentialsRequest`s. This common queue serializes all writes for a target and avoids concurrent override/root updates.

The watches use `predicate.Funcs`, not `predicate.NewPredicateFuncs`. Their update handler accepts an event when either the old or new object is relevant to an override relationship. This preserves reconciliation when a source Secret loses the dedicated label or relevant metadata, and when an override is changed or removed. Delete events are also accepted so deletion of an override performs no-override cleanup and deletion or relevance-removal of a source is reported as fail-closed. The CRD replaces annotation-only target selection; there is no annotation compatibility path that could silently restore root credentials while an override CR still exists.

**Status and condition reasons:**

| Situation | `Ready` | Reason | Target behavior |
|---|---|---|---|
| Valid labeled source synchronized | `True` | `OverrideApplied` | Target contains the override data. |
| Source Secret does not exist | `False` | `SourceSecretMissing` | Target is left unchanged. |
| Source lacks the label or has invalid data | `False` | `SourceSecretInvalid` | Target is left unchanged. |
| More than one override targets a Secret | `False` | `DuplicateTarget` | Every conflicting target is left unchanged. |
| `credentialsMode: Manual` | `False` | `ManualMode` | CCO does not manage the override or target. |

`TargetSynced=SyncPending` may be reported while work is in progress and `TargetSynced=SyncBlocked` when the source or mapping is invalid. Condition messages identify resource names and remediation but never include Secret data.

### Risks and Mitigations

**Risk: An unauthorized principal directs credentials to another component.**
Mitigation: RBAC limits override and labeled-source Secret create/update/delete operations to cluster administrators or explicitly delegated credential-management service accounts. Admission verifies that a target is a real vSphere `CredentialsRequest` target and rejects the root Secret. Authorization tests cover both create and update denials for unauthorized principals.

**Risk: CCO reads unrelated credentials from the shared `openshift-config` namespace.**
Mitigation: CCO uses the dedicated source label for list/watch selectors and has namespace-scoped, read-only source-Secret RBAC. It does not log Secret data.

**Risk: Invalid source data or duplicate mappings silently escalates an affected component to root credentials.**
Mitigation: Missing, invalid, unlabeled, and ambiguous overrides fail closed, leave the target unchanged, and expose `SourceSecretMissing`, `SourceSecretInvalid`, or `DuplicateTarget` conditions. Only the absence of an override CR selects the normal root path.

**Risk: A source or override update is missed during removal.**
Mitigation: `predicate.Funcs` evaluates old and new objects, and all events map through the serialized `CredentialsRequest` queue. Tests cover relevance removal followed by deletion.

**Risk: Tech Preview APIs are used during a minor update.**
Mitigation: CRD, policy, and controller behavior are gated by `TechPreviewNoUpgrade`; the documented support posture prohibits minor upgrades while it is enabled.

### Drawbacks

- This introduces a feature-gated CRD, admission validation, source-Secret inventory, and status surface that must be maintained with the CCO.
- Fail-closed behavior can retain a component's prior target data until an administrator resolves a bad source or duplicate mapping; it deliberately favors preventing unexpected credential escalation over automatic fallback.
- Source Secret and CR authors require privileged access to a shared platform namespace, so delegation must be reviewed carefully.

## Alternatives (Not Implemented)

### Annotation-Based Secret Mapping

Selecting a target through annotations on arbitrary Secrets was rejected. It lacks a structured API, makes admission validation and immutability difficult, permits ambiguous matching, and makes relevance-removal handling error-prone. `VSphereCredentialOverride` provides an explicit, observable mapping instead.

### Installer-Native Configuration

Per-component credentials configured directly in `install-config.yaml` are deferred to a subsequent phase. This proposal focuses on the CCO-side API and day-2 management; installer integration must define its own source lifecycle and validation contract.

### Single Multi-Key Secret

A single Secret with multiple components' credentials was rejected because separate source Secrets provide better access separation, independent lifecycle management, and clearer audit trails.

### CredentialsRequest Field Extension

Adding an override reference to `CredentialsRequest.spec` was rejected because `CredentialsRequest` objects are owned by consuming components. A dedicated CRD keeps the user-authored mapping separate from those component-owned requests while allowing CCO to validate the target relationship.

## Open Questions

The CRD contract resolves the prior questions about duplicate mappings, status, and Manual mode. The remaining implementation prerequisite is assignment of an API approver and publication of the approved `openshift/api` change; this enhancement intentionally does not fabricate that approval or its PR URL.

## Test Plan

### Unit Tests

- Validate required reference fields, immutable `targetSecretRef`, self-reference rejection, and rejection of non-vSphere or root targets.
- Verify target-inventory maintenance for vSphere `CredentialsRequest` create, update, delete, and controller-startup rebuild; verify unavailable inventory denies override admission.
- Verify one valid labeled source synchronizes the target and reports `OverrideApplied`.
- Verify missing, unlabeled, incomplete, empty, and unmatched source data report `SourceSecretMissing` or `SourceSecretInvalid`, preserve the target, and never use root credentials while the CR exists.
- Verify duplicate target matches set `DuplicateTarget` on every conflicting override and leave the target unchanged.
- Verify no override CR uses the existing root path in managed mode.
- Verify `credentialsMode: Manual` performs no source read or target write and reports `ManualMode` for an existing override.
- Verify serialized queue mapping for override and source-Secret create/update/delete events.
- Verify `predicate.Funcs` accepts relevant old or new objects, including source-label or metadata removal and remove-then-delete; verify override deletion requeues root-path reconciliation.
- Verify allowed cluster-admin/delegated-service-account create and update operations, and denied create and update operations for unauthorized principals in `openshift-config`.

### Integration / E2E Tests

- On a vSphere cluster with `TechPreviewNoUpgrade`, create the source Secret and one valid override; verify only its `CredentialsRequest` target receives the override data and status becomes ready.
- Rotate the source data and verify the target is re-synchronized through the serialized queue without affecting other component targets.
- Exercise source deletion, invalid data, label removal, duplicate overrides, invalid target admission, and target immutability; verify conditions and no silent root fallback.
- Delete the only override in managed mode and verify the normal root path resumes only after the CR is absent.
- Use authorized and unauthorized identities to verify source-Secret and override CR create/update authorization.
- Verify Manual mode leaves override and target data unmanaged.
- Verify SNO behavior and that HyperShift / Hosted Control Planes and MicroShift are rejected or omitted from the supported test matrix.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Approved `openshift/api` type/CRD change and assigned API approver are linked from this enhancement.
- CRD, admission policy, and CCO manifests are correctly gated by `TechPreviewNoUpgrade`.
- Unit and integration coverage validates API rules, source-data validation, condition reasons, queue serialization, authorization denial, Manual mode, and fail-closed behavior.
- E2E coverage validates valid synchronization and all no-write failure paths on vSphere.
- Administrator and support documentation explains privileged authoring, source labels, conditions, and recovery.

### Tech Preview -> GA

- The API has an approved compatibility and feature-gate graduation plan, including any version promotion required from Compatibility Level 4.
- Automated conformance, upgrade, downgrade, and long-running credential-rotation coverage demonstrates safe behavior.
- At least one release cycle of supported use shows no unresolved credential-synchronization regressions.
- OpenShift documentation includes supported topologies, RBAC delegation, Manual-mode behavior, and fail-closed remediation.

### Removing a deprecated feature

This is a new API. If it is replaced, a future enhancement must define migration of CRs, source Secrets, feature gating, and the API deprecation period before removal.

## Upgrade / Downgrade Strategy

While the feature is in `TechPreviewNoUpgrade`, minor-version upgrades are not supported. Enabling the feature is opt-in and does not affect a managed target until an override CR is created. On clusters without an override CR, the existing root-credential behavior is unchanged.

Before disabling the feature, removing its manifests, or attempting a downgrade, an administrator must delete override CRs, verify that the root credential is valid for every affected component, and verify that targets have reconciled through the normal root path. An older CCO does not understand the CRD contract; remaining CRs and source Secrets are inert configuration and are not a supported substitute for that verification. The API-approval implementation must include downgrade tests before Tech Preview promotion.

## Version Skew Strategy

The CRD, admission policy, and CCO controller must be introduced and gated as one feature-set unit. The controller tolerates no CRD by retaining its existing behavior when the feature is disabled. Consuming components continue to read their `CredentialsRequest` target Secret and do not depend on the source API version. No independent cross-component protocol is introduced.

## Operational Aspects of API Extensions

**Failure modes:** A target that is invalid is denied at admission. After admission, missing, invalid, unlabeled, or ambiguous sources are reflected on the override's conditions and do not modify the target. An unavailable target inventory denies new override creation. In Manual mode, CCO does not manage the source, override application, or target.

**Monitoring:** Operators use `VSphereCredentialOverride.status.conditions`, `observedGeneration`, and `resolvedSource` to determine whether a target was synchronized. CCO logs and events identify the resource and condition reason but must not log credential bytes.

**Recovery:** Correct the source Secret data or label, delete duplicate override CRs, or delete the override CR when returning to the normal root path is intended. Reconciliation is event-driven; force-deleting the target Secret is not the prescribed recovery mechanism.

## Support Procedures

1. List and describe overrides without exposing Secret data: `oc get vspherecredentialoverrides` and `oc describe vspherecredentialoverride <name>`.
2. Confirm that `targetSecretRef` matches a vSphere `CredentialsRequest.spec.secretRef` and inspect the `Ready`, `SourceValid`, and `TargetSynced` reasons.
3. Confirm the named source Secret exists in `openshift-config` and carries `cloudcredential.openshift.io/vsphere-override-source=true`; inspect only metadata and required key names, never Secret values.
4. Confirm delegated access with `oc auth can-i` for both `VSphereCredentialOverride` and source-Secret create/update operations in `openshift-config`; unauthorized principals must be denied.
5. For `SourceSecretMissing`, `SourceSecretInvalid`, or `DuplicateTarget`, correct the reported configuration and wait for reconciliation. Delete the CR only when a deliberate return to the root path is desired.
6. For `ManualMode`, manage component credentials according to the existing Manual-mode procedure; CCO will not apply the override.

## Implementation History

- 2026-08-17: Initial annotation-based enhancement proposal.
- 2026-09-11: Revised the proposal to the `VSphereCredentialOverride` CRD contract, including fail-closed behavior, scoped source discovery, authorization, and Manual-mode decisions.
- Target release: 5.1.0.
- Primary tracking: [SPLAT-2874](https://issues.redhat.com/browse/SPLAT-2874); CCO implementation: [SPLAT-2889](https://issues.redhat.com/browse/SPLAT-2889); parent initiative: [SPLAT-2724](https://issues.redhat.com/browse/SPLAT-2724).
