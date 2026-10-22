---
title: azure-identity-status-reporting
authors:
  - "@muraee"
reviewers:
  - "@csrwng, for overall HyperShift architecture and status API design"
  - "@bryan-cox, for Azure platform implementation and ARO HCP identity model"
  - "@vismishr, for managed Azure (ARO HCP) identity configuration and CPO reconciliation"
  - "@machi1990, for ARO HCP identity replacement consumer requirements"
approvers:
  - "@csrwng"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-09-24
last-updated: 2026-10-22
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/CNTRLPLANE-4491
see-also:
  - "/enhancements/hypershift/api-driven-azure-topology-and-private-connectivity.md"
replaces: []
superseded-by: []
---

# Azure Identity Status Reporting for HyperShift

## Summary

Report the application of every ARO HCP managed identity configured through a
HostedCluster (HC): control-plane identities, data-plane identities, optional
Azure KMS, and the ACR pull identity. The controller that writes a consumer's
identity-bearing resources reports its last fully applied reference. Per-role
detail stays on ControlPlaneComponents (CPCs); the ACR worker attachment is
reported by each NodePool. HostedCluster has an aggregate
`AzureIdentitiesApplied` condition and no copy of the per-role list. The
condition means that HyperShift has applied the current references to the
specified Kubernetes resources. It does not prove that a workload used them.

This is a provisional design for [CNTRLPLANE-4491](https://redhat.atlassian.net/browse/CNTRLPLANE-4491).
The current spec mirror in
[CNTRLPLANE-4493](https://redhat.atlassian.net/browse/CNTRLPLANE-4493)
is not an applied-state signal and must be replaced before the RP consumes it
for identity replacement.

## Motivation

The ARO resource provider (RP) is taking ownership of all cluster managed
identities. The identities have different configuration paths, but RP should
use one rule for their HyperShift consumers: wait for positive evidence that
every applicable consumer has applied the requested reference. A single spec
mirror can advance before a Secret, SecretProviderClass, or AzureMachineTemplate
write succeeds. A single cluster-wide flag cannot diagnose which consumer is
behind. Status must also distinguish a completed write from a mounted or used
credential.

The RP is the intended writer of HC desired state through kube-applier. The
current Clusters Service (CS) implementation is evidence for the identity
inventory, not an enduring dependency of this API. CS currently maps eight
control-plane and three data-plane roles into
`spec.platform.azure.azureAuthenticationConfig.managedIdentities`, optional
KMS into `spec.secretEncryption.kms.azure.kms`, and ACR pull into
`spec.platform.azure.containerRegistry.credentials.managedIdentity.resourceID`.
CS can patch this HC field for ACR replacement, and HyperShift creates new
node templates. Existing VMs in InPlace NodePools do not acquire a changed
identity from that template update; that rollout is not complete replacement.

### User Stories

1. As an ARO HCP operator, I can inspect the last fully applied reference for
   every HC-configured managed identity consumer, so I can find a partial or
   stalled replacement.
2. As an ARO HCP operator, I can tell whether all applicable HyperShift
   consumers have applied the current HC configuration, so I can advance the
   replacement workflow without mistaking a desired spec for applied state.
3. As an on-call engineer, I can distinguish resource application from mount,
   node attachment, and credential use, so I do not remove an old identity on
   insufficient evidence.

### Goals

1. Cover all managed identities configured through the ARO HCP HC, including
   optional KMS and replaceable ACR pull identity, whether RP configured them
   through an operator map or a separate field.
2. Publish each role only after all resources that carry its reference have
   reconciled successfully; keep the previous complete observation on a
   partial failure.
3. Expose per-consumer detail without copying the full list to HCP and HC.
   Give RP a machine-readable aggregate result on HC.
4. Define the boundary between applied-resource status and later mount or
   workload-use evidence.

### Non-Goals

1. The ARO Service Managed Identity (SMI). CS uses it for service-side Azure
   operations and does not configure it in the HC CR. RP must track its own
   service-side operations; HyperShift cannot report its application.
2. Self-managed Azure workload identities. They use a different ownership and
   replacement flow; this proposal targets ARO HCP managed identities.
3. Proving certificate mount, token acquisition, Azure authorization, or image
   pull success from a Kubernetes resource write.
4. Authorizing old-identity deletion from `AzureIdentitiesApplied` alone. RP
   needs the consumer evidence and cleanup gate described below. Worker-node
   cleanup remains in its separate ARO workstream.

## Proposal

### Workflow Description

1. RP writes the desired HC spec. HO persists the corresponding HCP spec
   before attributing identity-resource writes to an HCP generation.
2. HO, CPO, and HCCO reconcile their respective resources. A producer
   advances the last complete state only after all identity-bearing writes
   for that role succeed. A failed write updates `lastAttempt` with a
   structured reason while retaining the previous complete state.
3. The NodePool controller reads the ACR resource ID from HC, reconciles the
   AzureMachineTemplate, and reports that ID on each NodePool only after CAPI
   reports that all desired Machines use the new template and are available.
   An updated template alone is insufficient; this does not verify the Azure
   VM's actual identity attachment through ARM. An InPlace NodePool whose
   existing Machines retain the old template reports
   `Failed/UnsupportedInPlaceIdentityChange` for a changed or removed ACR
   identity until those Machines are replaced or an attachment-update
   mechanism is delivered.
4. HO computes the expected consumer set from the current HC authentication
   mode, KMS and ACR configuration, enabled capabilities, and NodePools. It
   sets `AzureIdentitiesApplied` on HC to `True` only when each expected record
   matches the desired reference and the current generation fence. `False`
   means at least one known consumer is still applying or has failed;
   `Unknown` means a required producer or observation cannot be trusted.
5. RP reads the HC condition for cluster-wide progress. For an individual
   identity, RP reads the relevant CPC and, for ACR, NodePool records using
   the same scope/role/reference comparison. The condition message lists
   laggards for people; RP must never parse that free-text message. An RP that
   cannot read the detailed records can conservatively wait for the whole
   cluster condition, but cannot classify an individual role from it.

A record identifies a HyperShift consumer, not a distinct Azure resource.
One Azure identity reused by multiple roles must converge at every role and
consumer before RP considers its HyperShift application complete. RP matches
the configured ARM resource to the HC field(s) it supplied, then applies the
same comparison to all corresponding observations. No CS-specific control
plane, data plane, or service identity type changes the comparison rule.

### Expected roles, writers, and completion points

The following inventory is normative for ARO HCP. The named CPC is the status
owner; the two synthetic reporter CPCs have distinct names from real
components. A role's record proves successful reconciliation of **all**
listed resources with the recorded reference, not just that the producer read
HCP spec. If a resource already has the desired content, a successful read
and no-op reconcile counts as a write success.

| HC role and reference | Required resources and reporter |
| --- | --- |
| Control plane `cloudProvider` (`credentialsSecretName`) | CPO `azure-cloud-controller-manager` CPC: `managed-azure-cloud-provider` SecretProviderClass, `azure-cloud-config` Secret containing `AADMSIDataPlaneIdentityPath`, and CCM Deployment reference. The separate `azure-cloud-config` ConfigMap carries the ACR ID. |
| Control plane `controlPlaneOperator` (`credentialsSecretName`) | HO `azure-platform-credentials` CPC: `managed-azure-cpo` SecretProviderClass and CPO Deployment volume reference. |
| Control plane `nodePoolManagement` (`credentialsSecretName`) | HO `azure-platform-credentials` CPC: `managed-azure-nodepool-management` SecretProviderClass, AzureClusterIdentity credential path, and a completed CAPZ Deployment rollout with the expected CSI object version mounted on every current CAPZ pod. |
| Control plane `imageRegistry` (`credentialsSecretName`) | CPO `cluster-image-registry-operator` CPC: `managed-azure-image-registry` SecretProviderClass and operator Deployment reference, when enabled. |
| Control plane `ingress` (`credentialsSecretName`) | CPO `ingress-operator` CPC: `managed-azure-ingress` SecretProviderClass and operator Deployment reference, when enabled. |
| Control plane `network` (`credentialsSecretName`) | CPO `cluster-network-operator` CPC: `managed-azure-network` SecretProviderClass and network controller Deployment reference. |
| Control plane `disk` and `file` (separate `credentialsSecretName` values) | CPO `cluster-storage-operator` CPC: disk requires `azure-disk-csi-config` Secret and `managed-azure-disk-csi` SecretProviderClass; file requires `azure-file-csi-config` Secret and `managed-azure-file-csi` SecretProviderClass. Each also requires its workload reference. Disk success does not advance file. |
| Data plane `imageRegistry`, `disk`, `file` (separate client IDs) | HCCO `azure-guest-credentials` reporter CPC: respectively `openshift-image-registry/installer-cloud-credentials`, `openshift-cluster-csi-drivers/azure-disk-credentials`, and `openshift-cluster-csi-drivers/azure-file-credentials` guest Secrets. Registry is required only when enabled; disk and file are required. Managed-mode guest ingress is a placeholder and is excluded. |
| KMS (`spec.secretEncryption.kms.azure.kms.credentialsSecretName`) | HO `azure-platform-credentials` CPC: the shared `managed-azure-kms` SecretProviderClass, `azure-kms-config` Secret, and CPO Deployment KMS volume/mount reference. CPO `kube-apiserver` CPC: kube-apiserver workload/configuration reference. Required only when Azure KMS is configured. |
| ACR pull (`spec.platform.azure.containerRegistry.credentials.managedIdentity.resourceID`) | CPO `azure-cloud-controller-manager` CPC: `UserAssignedIdentityID` in the cloud config. Every NodePool reports the same resource ID after its AzureMachineTemplate and MachineDeployment rollout have converged. Required only when ACR pull identity is configured. |

`managed-azure-kms` is currently reconciled by both HO and CPO. This design
makes HO its sole writer; CPO verifies its presence and writes only the
kube-apiserver resources it owns. The HO KMS record advances after both the
shared SecretProviderClass and `azure-kms-config` Secret succeed. The CPO KMS
record advances after its own kube-apiserver write; these are distinct events.
This ownership change must ship with the reporter so two controllers do not
race on one object.

For a disabled capability or removed role, the former consumer stays in the
expected set until its owner removes the identity-bearing resources and
publishes a `Removed` record for the current generation. HO then stops
requiring the role. A deleted CPC or an empty record is not proof of teardown.
For ACR removal, the new machine rollout must complete without the old
identity before NodePool reports `Removed`. A deleting NodePool remains in the
expected set until its Machines are gone: the existing NodePool finalizer
calls `ensureMachineDeletion` before it is removed. HO tracks the deleting
NodePool by UID and ignores a same-name replacement until the prior UID's
Machine deletion is observed. If the finalizer contract changes, a durable
UID-keyed tombstone must preserve this fence. New NodePools enter the set
immediately and make the aggregate condition non-True until they converge.
A cluster with no NodePools has no worker attachment to check.

### API Extensions

Add an optional `credentialsSecretVersion` to the HC Azure managed-identity
reference (including KMS). It is the immutable Key Vault object version that
RP expects for `credentialsSecretName`, not a certificate value. RP must set
it for every new Key Vault-backed reference or replacement; a new version
changes HC generation. The field is optional for API compatibility, but a
name-only legacy reference is `Unknown/VersionUnavailable` until RP supplies
the expected version. The SecretProviderClass writer pins `objectVersion`
when the field is set. CP and KMS applied records then carry both the name and
expected version; the aggregator compares both. ACR ResourceID and data-plane
client-ID records do not acquire a Key Vault version.

Add a bounded `status.azureIdentities` list to CPC and a matching ACR record
to NodePool status. Do not add a role list to HCP or HC. Add the standard
`AzureIdentitiesApplied` condition to HC, with `observedGeneration` equal to
the HC generation evaluated. Example:

```yaml
kind: ControlPlaneComponent
metadata:
  name: cluster-storage-operator
status:
  azureIdentityReporter:
    schemaVersion: 1
    producerRevision: cpo-template-hash
  azureIdentities:
  - scope: controlPlane
    role: disk
    state: Applied
    reference:
      type: CredentialsSecretName
      value: disk-identity-v2
      keyVaultObjectVersion: 4d71...
    observedHCPGeneration: 42
    lastAttempt:
      hcpGeneration: 42
      phase: Succeeded
  - scope: controlPlane
    role: file
    state: Applied
    reference:
      type: CredentialsSecretName
      value: file-identity-v2
    observedHCPGeneration: 42
    lastAttempt:
      hcpGeneration: 42
      phase: Succeeded
```

```yaml
kind: NodePool
status:
  azureIdentityReporter:
    schemaVersion: 1
    producerRevision: ho-template-hash
  acrPullIdentity:
    state: Applied
    reference:
      type: ResourceID
      value: /subscriptions/.../userAssignedIdentities/acr-v2
    observedHCGeneration: 28
    lastAttempt:
      hcGeneration: 28
      phase: Succeeded
```

```yaml
kind: HostedCluster
status:
  conditions:
  - type: AzureIdentitiesApplied
    status: "False"
    reason: Applying
    observedGeneration: 28
    message: "Waiting for cluster-storage-operator/disk and NodePool workers-a"
```

CPC list keys are `(scope, role)` within the CPC; the CPC name supplies the
consumer identity. KMS uses `controlPlane/kms` on both its CPCs. ACR uses
`controlPlane/acrPull` for cloud config and `dataPlane/acrPull` on NodePools.
The ARO SMI does not create a service-scope role. HCCO has exactly one guest
Secret for each of its three managed-mode roles, so no Secret identifier is
needed in the key. An applied record has exactly one nonempty typed reference.
A removed record has no reference. Before first success, a keyed record has
`state: Pending`, no `reference` or successful-apply generation, and a
`lastAttempt` with current HCP generation (HC generation for NodePool) and
`phase: Pending`. On retry or failure, `lastAttempt` becomes `Pending` or
`Failed` with a structured reason while an earlier `Applied` or `Removed`
state and its reference/generation remain unchanged. HO maps a current
`Pending` attempt to `False/Applying` and a current `Failed` attempt to
`False/ApplyFailed`, even if the retained success matches the desired
reference. Missing or stale attempts remain `Unknown/StatusUnavailable`.
The schema validates enum
combinations, unique keys, and bounded list size. Status contains no
certificate, token, or secret value. The existing CNTRLPLANE-4493 spec-mirror
fields must not coexist as a second
"active" status signal.

`observedHCPGeneration` is the generation whose **entire role resource set**
last reconciled successfully. CPC's existing `status.observedGeneration` says
only that the component attempted the HCP generation; it advances even after
failure. It cannot fence a partial disk Secret/SecretProviderClass write or an
A -> B -> A rollback. The per-role field is therefore a safety fence rather
than a duplicate diagnostic. HO-owned writes currently precede HCP spec
persistence in CoreHCPChain; they must move after persistence and use the
persisted HCP generation before their records advance. A retry must re-read
spec and reapply all role resources, not relabel a previous success with the
new generation. NodePool uses the HC generation it read for the completed ACR
rollout. An unrelated HCP spec change conservatively requires another
successful no-op reconciliation before `True`.

The producer publishes `schemaVersion: 1` and the revision of its live
Deployment pod template with the record. HO verifies that revision against
the currently rolled-out producer Deployment and requires its Available
condition. A mismatch, unavailable Deployment, absent reporter marker, or
unsupported schema version makes the HC condition `Unknown`; old records
cannot survive a producer downgrade as positive evidence. HO applies the
same rule to HO, CPO, HCCO, and the NodePool reporter. If the reporter
revision cannot be verified, the record is unusable. No heartbeat or periodic
status write is needed when the producer revision and HC/HCP generation stay
unchanged. RP reads the resulting HC condition; detailed readers also check
reporter support before using a CPC or NodePool record.

The HC condition is `True/AsExpected` only for a complete applicable set,
`False/Applying` while current producers are progressing, `False/ApplyFailed`
when a required producer reports a current-generation role attempt as Failed,
and
`Unknown/StatusUnavailable` for absent, old, unsupported, or unverifiable
reporting. `Unknown/VersionUnavailable` identifies a Key Vault-backed role
whose expected immutable version is missing.
`False/UnsupportedInPlaceIdentityChange` identifies an ACR
replacement or removal that cannot update existing worker VMs. A `Removed`
record is required before a role is dropped from the
set. RP calls a non-converged role *pending* until its replacement operation
has waited 30 minutes without a new current-generation producer observation;
it then calls the role *stalled* for alerting. An explicit current-generation
`ApplyFailed` is stalled immediately. This time threshold affects alerts,
never cleanup. RP gets the start time from its replacement operation and
progress from structured CPC/NodePool status changes, not condition text.

### Key Vault version, CAPZ consumption, and cleanup

RP records the returned immutable Key Vault object version when it writes a
reference or replacement, then sends the name and version through HC. The
SPC pins that version. A same-name replacement without a new expected
version cannot be distinguished from the old credential and must not advance
to `Applied` or authorize cleanup. A legacy name-only reference remains
`Unknown/VersionUnavailable` until RP supplies the version. The version
field is an implementation prerequisite, not an open question for this
design.

CAPZ needs a stronger boundary than an AzureClusterIdentity or SPC write.
The pinned CAPZ credential cache keys UAMI credentials by authority host and
credential type; it does not include `credsPath` or invalidate on a path
change. The pinned MSI dataplane reload watches file `Write` events and uses
a six-hour fallback; it can reject a replacement whose certificate NotBefore
is not newer. HO therefore changes the CAPZ Deployment pod-template
annotation whenever the nodePoolManagement credential name or version
changes, after reconciling the SPC and AzureClusterIdentity. Fresh CAPZ pods
start with an empty credential cache. The HO record advances only after the
new Deployment revision is available and every current CAPZ pod's CSI
`SecretProviderClassPodStatus` shows the expected mounted object version.
A pod that cannot report its version leaves the role non-True. The rollout is
required even if an upstream CAPZ cache fix lands; the enhancement does not
depend on that fix. CAPZ should separately key or invalidate cached UAMI
credentials when the path or mounted credential version changes.

[CNTRLPLANE-4495](https://redhat.atlassian.net/browse/CNTRLPLANE-4495)
delivers mount/version evidence for the other CSI consumers and the RP
cleanup gate. Mount evidence does not prove token acquisition, Azure
authorization, or image pull. RP, as owner of
[ARO-29197](https://redhat.atlassian.net/browse/ARO-29197), must require
positive evidence that every current consumer has switched, including pod
rollout or a separately justified workload-use policy, before deleting an
old control-plane or data-plane identity. A changed `AzureIdentitiesApplied`
condition alone must never trigger cleanup. Worker-node identity cleanup is
tracked separately from ARO-29197; NodePool application still requires a
completed worker rollout and does not prove an image pull succeeded.

### Topology Considerations

#### Hypershift / Hosted Control Planes

CPC and NodePool detail and the HC condition live in the management cluster.
HCCO writes guest-cluster Secrets, then reports each successful write through
its management-cluster CPC. HO watches the bounded CPC and NodePool set and
updates only the HC condition when it changes. RP's existing HC ReadDesire
supplies the aggregate signal; role-level reads require additional CPC and
NodePool access. The HC message is for support, never a machine API.

#### Standalone Clusters

Not applicable: this API targets ARO HCP.

#### Single-node Deployments or MicroShift

No MachineConfig or ignition change is proposed.

#### OpenShift Kubernetes Engine

Disabled optional components are excluded only after their identity-bearing
resources have been removed and a current-generation `Removed` record exists.

### Implementation Details/Notes/Constraints

CPO publishes per-role results after its component resource apply loop.
Storage reports disk and file separately, and HCCO returns per-Secret outcomes
instead of only a combined error. HO creates
`azure-platform-credentials`; HO creates the `azure-guest-credentials` CPC and
grants HCCO write access only to its Azure identity status fields. Each
producer patches only its owned status fields with conflict retries. The
existing `hosted-cluster-config-operator` CPC is a real CPO component with
availability/version status and must keep participating in those rollups.
The current CPO
`reconcileAzurePlatformStatus` spec mirror stops writing the authoritative
Azure identity field. HO writes the HC aggregate condition after checking
producer revisions, current generations, references, and role applicability.
It never copies a CPC list to HCP or HC.

The NodePool controller already watches HC and generates a new
AzureMachineTemplate name when the ACR resource ID changes. Its new ACR
record advances only after CAPI reports all desired Machines use the intended
template and are available. Template creation, a changed MachineDeployment
reference, or a generic NodePool Ready condition alone is insufficient.
CAPZ `VMSpec.Parameters` returns no update parameters for an existing VM, and
the InPlace NodePool path only changes the MachineSet template reference.
Thus an InPlace NodePool with Machines still using the old template reports
`Failed/UnsupportedInPlaceIdentityChange` for ACR replacement or removal,
even if new Machines carry the target ID. The supported operator action is
to replace that pool with a Replace-mode pool, draining under the existing
node availability policy. Changing `UpgradeType` on an existing NodePool is
not supported. Automatic in-place VM identity updates or forced replacement
need separate design and tests before this reason can be cleared. The ACR
cloud-config CPC and every applicable NodePool must agree before HC reports
True. ACR removal follows the same rule with `Removed` records.

HO should surface a failed expected role by naming its source in the
human-readable HC condition message and setting the structured reason.
Producers must not clear a last complete record on a transient apply failure;
HO rejects it when its generation does not match. On partial success, the
resource set is mixed and no complete observation for the new generation
exists. RP does not infer that the prior record still describes every live
resource. Generated CRDs, clients, deepcopy code, API documentation, and
vendored copies change together during implementation.

The two synthetic reporter CPCs, `azure-platform-credentials` and
`azure-guest-credentials`, carry a
`hypershift.openshift.io/identity-report-only` label. CPO excludes labeled
reporters from its general CPC availability and control-plane-version
rollups, which currently require every listed CPC to publish `Available` and
`RolloutComplete` conditions and a release version. HO still includes them
in the Azure identity condition using their reporter schema and producer
revision. This prevents a status-only CPC from accidentally blocking a
control-plane upgrade.

### Risks and Mitigations

- **Applied is not used.** Keep RP cleanup protection until a separate policy
  accounts for mounts, workload use, and Azure propagation.
- **Partial writes or rollback.** Keep the last fully applied record, require
  a successful whole-role reconcile at the current generation, and hold HC
  non-True on mixed resources.
- **Producer downgrade.** Require a supported reporter schema and matching
  live Deployment revision before trusting retained status.
- **CAPZ stale credentials.** Restart its Deployment for a path or version
  change and require new pods with the expected mounted version before
  reporting nodePoolManagement applied. Retain a separate upstream cache fix.
- **Existing worker VMs.** Report a terminal InPlace ACR replacement failure;
  retain the old identity until a Replace-mode pool has converged and the
  separate worker cleanup gate has passed.
- **Role removal.** Require positive teardown evidence before excluding a
  consumer; a missing CPC or NodePool does not count.
- **Fleet scale.** Avoid HCP/HC per-role lists and no-op status writes; measure
  CPC, NodePool, and HC condition update rates before GA.

### Drawbacks

RP needs CPC and NodePool reads for per-identity detail, and implementation
adds producer-owned status fields and an HO aggregator. The aggregate HC
condition lets a consumer that only reads HC wait safely, though an unrelated
stalled role can delay its whole-cluster progress. NodePool status is a
necessary exception to CPC-only detail because worker VM attachment is
owned and observed by the NodePool controller.

## Alternatives (Not Implemented)

### Copy per-role status to HCP and HC

This duplicates the detailed data on two higher-level APIs and increases
status write and watch traffic. The HC condition is sufficient for an
aggregate read; RP reads producer status when it needs individual detail.

### Copy desired Azure spec into status

The current CNTRLPLANE-4493 mirror can advertise a new identity before any
resource apply. It is useful only as desired-state diagnostics and must not
be presented as applied or active state.

### Use only CPC observedGeneration and applied value

`status.observedGeneration` advances after a failed reconcile. A matching
old value after an A -> B -> A transition can hide a mixed resource set.
Per-role successful-apply generation prevents this false positive.

### Query Key Vault on every reconcile

External calls add latency and load and cannot show which pods mounted or
used the returned credential. Mount evidence belongs to the later phase.

## Jira Delivery Alignment

| Task | Required outcome under this enhancement |
| --- | --- |
| [CNTRLPLANE-4493](https://redhat.atlassian.net/browse/CNTRLPLANE-4493) | Replace the early CPO spec mirror with after-apply CPC records. Do not release the mirror as an applied signal. |
| [CNTRLPLANE-4494](https://redhat.atlassian.net/browse/CNTRLPLANE-4494) | Report KMS only after HO's shared SecretProviderClass, config Secret, and CPO Deployment mount, plus CPO's kube-apiserver resources, reconcile. |
| [CNTRLPLANE-4495](https://redhat.atlassian.net/browse/CNTRLPLANE-4495) | Deliver per-pod mount/version evidence for non-CAPZ CSI consumers and the RP cleanup gate; resource-applied status alone cannot satisfy ARO-29197 requirement 3. |
| [CNTRLPLANE-4496](https://redhat.atlassian.net/browse/CNTRLPLANE-4496) | Test CP, DP, KMS, CAPZ rollout/cache reset, ACR Replace and InPlace behavior, partial failure, teardown, downgrade, and RP visibility. |
| [CNTRLPLANE-4509](https://redhat.atlassian.net/browse/CNTRLPLANE-4509) | Deliver versioned HC Key Vault references, SPC `objectVersion` pinning, and same-name rotation status semantics before identity replacement implementation. |
| [CNTRLPLANE-4491](https://redhat.atlassian.net/browse/CNTRLPLANE-4491) | Align the epic acceptance criteria with CPC and NodePool detail plus the HC aggregate condition; remove the superseded HC/HCP `AzurePlatformStatus` list. |
| [ARO-29197](https://redhat.atlassian.net/browse/ARO-29197) | RP owns the final cleanup gate after evidence for all current CP/DP consumers; worker-node cleanup remains separately tracked. |

## Open Questions

1. Which workload-use evidence or grace period should RP require after every
   affected pod or machine has converged? ARO-29197 must resolve this before
   it deletes old identities; `AzureIdentitiesApplied` never answers it.

## Test Plan

- Unit tests for role inventory, capability gating, per-resource success and
  failure, disk/file independence, A -> B -> A rollback, and teardown.
- API validation tests for bounded unique records, reference types, states,
  producer schema, generations, and old-client compatibility.
- Integration tests with injected Secret/SecretProviderClass failures,
  HO-before-HCP ordering, shared KMS ownership, HCCO guest writes, reporter
  downgrade, and HC condition propagation.
- ACR tests for add, replacement, removal, zero and multiple NodePools,
  template creation versus completed CAPI rollout, InPlace terminal failure,
  Replace-mode pool replacement, deletion finalizer fencing, and stalled
  rollout.
- CAPZ tests for path and same-path version changes, Deployment restart and
  rollout, current-pod mounted-version evidence, failed mounts, and retained
  old cache behavior before restart. Verify a same-name change without a new
  expected version cannot reach Applied.
- Azure end-to-end tests for RP read of HC condition and per-role status;
  confirm none of these alone authorizes old-identity deletion.

## Graduation Criteria

### Dev Preview -> Tech Preview

API approval, role inventory and writer ownership, and tests that prove
report-after-apply for CP, DP, KMS, and ACR.

### Tech Preview -> GA

End-to-end replacement coverage, version-skew and downgrade tests, measured
status cost, and a documented RP cleanup policy. ACR worker replacement is
GA-ready only for proven Replace-mode rollout; InPlace remains an explicit
failure until a separate worker update mechanism is delivered.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

New fields remain absent until their producer successfully applies the
resources, except a keyed `Pending` attempt before first success. HO reports
`Unknown/StatusUnavailable` for an old or unsupported
producer; RP retains its existing cleanup protection. On downgrade, the
producer revision no longer matches the retained record and the aggregate
condition becomes Unknown. Older CRDs may prune new fields; absence is
Unknown. Compatibility tests cover supported version pairs.

## Version Skew Strategy

CPO follows the hosted release and can differ from HO. HO evaluates the
reporter schema and the currently rolled-out Deployment revision for each
source, plus current HC/HCP generations. A producer that cannot report is
Unknown, not silently omitted from the expected set. RP uses the HC condition
for aggregate compatibility and checks the same reporter fields on detailed
reads. It does not parse a condition message.

## Operational Aspects of API Extensions

CPC and NodePool records change only when the applied reference, state,
generation, or reporter revision changes. HO writes the HC condition only on
meaningful changes. A status write failure leaves the previous observation
intact and makes the aggregate condition non-True until retry. Watches and
status size must be measured at fleet scale. Measure (1) the time from an HC
generation change until all consumers report Applied, by role and cluster,
(2) the count and age of
current-generation `Applying`, `ApplyFailed`, `StatusUnavailable`, and
`UnsupportedInPlaceIdentityChange` records, and (3) CPC, NodePool, and HC
status writes per cluster per hour. Avoid polling writes when values do not
change. Alert on `ApplyFailed` or `UnsupportedInPlaceIdentityChange`
immediately; alert when a replacement has no new producer observation for
30 minutes; investigate unavailable reporter or mounted-version evidence
after 15 minutes. These are initial paging thresholds, to calibrate from
fleet measurements before GA, not cleanup deadlines.

HyperShift on-call owns missing reporters, reconciliation failures, CAPZ
rollout and NodePool status. ARO RP on-call owns desired version supply,
replacement orchestration, and the final cleanup gate. The Azure platform
maintainers own the upstream CAPZ cache follow-up. Failure to advance the
condition delays identity replacement and retains the old identity; it does
not make the cluster unavailable solely because a status reporter is late.

## Support Procedures

Inspect HC `AzureIdentitiesApplied` reason and message. For a lagging role,
read its CPC `status.azureIdentities` or the NodePool `status.acrPullIdentity`,
compare the typed reference and successful-apply generation with desired HC,
and inspect the named Secret, SecretProviderClass, cloud config, guest Secret,
or AzureMachineTemplate. Check mount and workload evidence separately.

## Infrastructure Needed

HO needs bounded CPC/NodePool watches and an HC condition reconciler. HCCO
needs management-cluster CPC status permission. RP needs CPC and NodePool
reads for per-identity progress; its existing HC read supplies the aggregate.
No new external service is required.
