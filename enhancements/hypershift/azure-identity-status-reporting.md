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
last-updated: 2026-09-28
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/CNTRLPLANE-4491
see-also:
  - "/enhancements/hypershift/api-driven-azure-topology-and-private-connectivity.md"
  - "/enhancements/hypershift/self-managed-azure.md"
replaces: []
superseded-by: []
---

# Azure Identity Status Reporting for HyperShift

## Summary

Report Azure identities per role after the controller responsible for each role
has applied its identity-bearing Kubernetes resources. Each controller records
its observation on the corresponding `ControlPlaneComponent` (CPC); the
HyperShift Operator (HO) rolls these records up to
`HostedControlPlane.status.platform.azure.identities`, which is then copied to
`HostedCluster.status.platform.azure.identities`. The rollup gives ARO HCP one
cluster-level read while retaining the component and role that produced each
observation. Status describes an applied reference, not successful credential
use. Mount confirmation is a separate, later observation.

This is a provisional design for [CNTRLPLANE-4491](https://redhat.atlassian.net/browse/CNTRLPLANE-4491).
The current spec-mirroring implementation in
[CNTRLPLANE-4493](https://redhat.atlassian.net/browse/CNTRLPLANE-4493)
does not provide the applied-state guarantee proposed here and must be revised
before its status can be used as a rotation signal.

## Motivation

ARO HCP replaces managed identities by changing the requested credential
reference. Today it cannot see which control-plane and data-plane consumers
have applied the change, so it retains old credentials for a conservative
period. Components reconcile independently; a whole-spec mirror can show the
new value before a component's resource write succeeds.

The desired update path at implementation time is Clusters Service (CS) to the
ARO resource provider (RP) backend, then kube-applier to
`HostedCluster.spec`. HO propagates that desired state to
`HostedControlPlane.spec`. CS does not directly update HostedCluster in this
design. The RP backend reads HostedCluster status through its ReadDesire and
compares each reported role with the latest desired state and its own
identity-replacement plan.

This enhancement supports [OCPSTRAT-2151](https://issues.redhat.com/browse/OCPSTRAT-2151)
and [ARO-29197](https://issues.redhat.com/browse/ARO-29197).

### User Stories

1. As an ARO HCP operator, I can see the applied identity reference for each
   control-plane, data-plane, and service identity role, so I can detect partial
   or stalled replacement.
2. As an ARO HCP operator, I can distinguish resource application from CSI
   mounting and credential use, so I do not delete an old identity based on a
   premature signal.
3. As a self-managed Azure operator, I can see the client ID applied for each
   applicable workload identity role.

### Goals

1. Cover managed and workload identity modes, including control-plane roles,
   guest-cluster data-plane credential Secrets, and the Azure KMS identity.
2. Report each role only after its owner has successfully applied the resources
   that carry that role's identity reference. Preserve the last applied value
   when a later apply fails.
3. Expose the same per-role observations on HCP and HC, with source component,
   applied reference, and observed HCP generation. An absent or stale record is
   unknown, never successful.
4. Define the separate pod-mount observation needed by
   [CNTRLPLANE-4495](https://redhat.atlassian.net/browse/CNTRLPLANE-4495),
   including a version signal for same-name Key Vault updates where available.

### Non-Goals

1. Proving a workload loaded a mounted certificate, acquired a token, or
   successfully called Azure. Neither a Kubernetes resource write nor CSI
   `SecretProviderClassPodStatus` proves those events.
2. Authorizing old credential deletion from applied-resource status alone.
   RP cleanup requires a separate, documented safety policy and evidence for
   every affected consumer.
3. Reporting a worker-node rollout. The data-plane identities in this proposal
   are the IDs placed in guest-cluster operand credential Secrets by the hosted
   cluster config operator (HCCO), rather than a NodePool ignition change.

## Proposal

### Workflow Description

1. CS supplies the new desired identity to the RP backend. The backend writes
   its HostedCluster ApplyDesire; kube-applier updates HostedCluster spec.
2. HO updates HCP spec. Each owner applies the identity-bearing resources
   using the captured desired reference and HCP generation. A successful
   per-role apply updates that owner's CPC status. A failed or skipped role
   retains its previous applied observation.
3. HO validates each CPC's identity record against the current CPC object and
   copies the per-role records into HCP platform status. The existing
   HCP-to-HC status propagation publishes the rollup to HostedCluster.
4. RP compares every expected role's reported reference and
   `observedHCPGeneration` with the latest desired state. A match confirms
   resource application only. RP waits for separately defined mount and
   workload-use evidence, or a justified grace period, before old-identity
   cleanup.

The rollup is keyed by `component`, `scope`, and `role`; it is not a
single cluster-wide success boolean. An identity used by more than one
component has a record for each consuming component. RP requires all
affected consumers to converge. API review must fix the complete role
inventory and ownership before implementation.

### Reporting ownership

| Scope and role | Resource applied by | Source status |
| --- | --- | --- |
| Control plane: cloud provider, image registry, ingress, network, disk, file | CPO component reconciler | Respective CPC; storage has separate disk and file records |
| Control plane: control plane operator | HO | HO-reconciled control-plane-operator CPC |
| Control plane: node pool management | HO | HO-reconciled CAPZ CPC |
| Service: Azure KMS credentials for CPO in managed mode | HO | HO-reconciled control-plane-operator CPC |
| Service: Azure KMS credentials for kube-apiserver, including workload identity mode | CPO kube-apiserver/KMS component | Kube-apiserver CPC |
| Data plane: image registry, disk, file | HCCO guest-cluster credential reconciler | HCCO CPC, one record per guest Secret |
| Data plane: ingress in workload identity mode | HCCO guest-cluster credential reconciler | HCCO CPC |

The managed-Azure guest ingress Secret currently contains a placeholder client
ID and is not a real identity consumer, so it is omitted in that mode. The
data-plane client IDs are written to guest-cluster credential Secrets by HCCO;
they are not reported as applied merely because HO copied them to HCP spec.
HCCO already has a management-cluster client for HCP status; its CPC reporting
path requires implementation and RBAC review.

In managed mode the KMS identity is consumed by two
`SecretProviderClass` resources: HO applies the one for CPO, and the CPO
kube-apiserver component applies the other. Both records must converge
before RP considers the KMS reference applied. In workload identity mode,
the kube-apiserver component applies its token-minter configuration. The
identity comes from
`spec.secretEncryption.kms.azure.kms` in managed mode, or
`spec.secretEncryption.kms.azure.workloadIdentity` in workload identity mode.
[CNTRLPLANE-4494](https://redhat.atlassian.net/browse/CNTRLPLANE-4494)
implements its source record within the common status shape.

### API Extensions

Add a bounded `azureIdentities` status list to CPC and an `identities` list
under the existing `status.platform.azure` on HCP and HC. The latter two
use the same wire type. This supersedes the unmerged
`managedIdentities`/`workloadIdentities` spec-mirror status shape in
CNTRLPLANE-4493; it must not leave two conflicting "active" status fields.
The proposed shape is:

```yaml
kind: ControlPlaneComponent
metadata:
  name: cluster-storage-operator
status:
  azureIdentities:
  - scope: controlPlane
    role: disk
    authenticationMode: ManagedIdentities
    appliedReference:
      type: CredentialsSecretName
      value: disk-identity-v2
    observedHCPGeneration: 42
  - scope: controlPlane
    role: file
    authenticationMode: ManagedIdentities
    appliedReference:
      type: CredentialsSecretName
      value: file-identity-v2
    observedHCPGeneration: 42
```

```yaml
kind: HostedCluster
status:
  platform:
    azure:
      observedHCGeneration: 28
      currentHCPGeneration: 42
      identities:
      - scope: controlPlane
        role: disk
        component: cluster-storage-operator
        authenticationMode: ManagedIdentities
        appliedReference:
          type: CredentialsSecretName
          value: disk-identity-v2
        observedHCPGeneration: 42
      - scope: dataPlane
        role: disk
        component: hosted-cluster-config-operator
        authenticationMode: ManagedIdentities
        appliedReference:
          type: ClientID
          value: 00000000-0000-0000-0000-000000000000
        observedHCPGeneration: 42
```

`component`, `scope`, and `role` form the list key in the rollup; a CPC
owns only its declared roles. `component` identifies the source CPC.
A record has exactly
one nonempty applied reference. Control-plane managed identities and managed
KMS use `CredentialsSecretName`; data-plane managed identities and workload
identities use `ClientID`. The schema must validate supported combinations,
list uniqueness, and bounded size. Status contains no certificate or token.

A record is absent until the first successful apply. When a component is
disabled or a role removed, the source removes its record after the related
resources are removed; HO then removes it from the rollup. HO must discard
records from a deleted or recreated CPC, using the CPC UID it observed while
aggregating. It must not retain a rollup entry when its source disappears.

`observedHCPGeneration` identifies the HCP spec used for that particular
apply, and advances only after a successful resource write. HO publishes the
HCP's current generation in `currentHCPGeneration` and the HC generation
whose desired spec it has propagated in `observedHCGeneration`. RP compares
the latter with `HostedCluster.metadata.generation` and each role's
`observedHCPGeneration` with `currentHCPGeneration`. HO sets
`observedHCGeneration` only after it has confirmed that HCP spec contains
that HC generation's desired values. The CPC's general
`status.observedGeneration` can advance after a failed reconcile and must
not be used as a substitute. A generation mismatch is conservative even
when an unrelated HCP spec change leaves identity references unchanged.

### Pod mount and same-name rotation

[CNTRLPLANE-4495](https://redhat.atlassian.net/browse/CNTRLPLANE-4495)
adds an optional per-role mount observation, copied through the same rollup.
For managed control-plane roles, it may contain
`mountedReference`, `objectVersion`, `observedHCPGeneration`, and
`allCurrentPodsMountedAt`. The field is present only when at least one
expected live pod exists and every current consumer pod has a matching CSI
`SecretProviderClassPodStatus` with `mounted=true` and the expected object
ID and version. A changed pod set, a missing/stale pod status, or mixed
versions clears the completion observation. The implementation must map
roles to actual pod consumers and handle rolling updates and terminating pods.

The CSI API exposes object ID and version. This can reveal a same-name Key
Vault content update without querying Key Vault on every reconcile, but the
RP must know the expected new object version to distinguish it from the old
one. If it does not, same-name rotation remains `Unknown` for cleanup.
An unchanged `credentialsSecretName` is never evidence that new content
was mounted. Similarly, re-creating a managed identity under the same Azure
resource ID requires a changed client ID, credential reference, or version
known to RP; the resource ID alone is insufficient.

`allCurrentPodsMountedAt` is deliberately a mount-completion timestamp.
It must not be named `confirmedActiveAt` or described as proof of
application use. CNTRLPLANE-4495's proposed automatic removal of the
24-hour delay needs workload-use evidence or a separately reviewed cleanup
policy. Workload identity mode needs a distinct rollout and token-use design;
CSI pod status does not cover it. Data-plane guest Secrets likewise require
operand rollout/consumption evidence beyond their successful write.

### Topology Considerations

#### Hypershift / Hosted Control Planes

CPC, HCP, and HC status live in the management cluster. HCCO must reach
guest-cluster Secrets to apply data-plane credentials and then report its
result through the management-cluster CPC. HO performs the bounded rollup
and already copies HCP platform status to HC. RP already mirrors HC; a
complete HC rollup avoids per-cluster ReadDesires for every CPC. Status writes
occur only when a record, generation, or mount state changes, not on every
reconcile or CSI poll.

#### Standalone Clusters

Not applicable: this API is specific to HyperShift.

#### Single-node Deployments or MicroShift

No new node-level workload or MachineConfig change.

#### OpenShift Kubernetes Engine

The reporting path is independent of the guest cluster's operator set;
disabled components have no identity record.

### Implementation Details/Notes/Constraints

The component framework already passes HCP through its workload context.
Its apply result needs per-role success information, including for storage's
two identities. A component must capture the reference and HCP generation
used for its resource write, verify that generation before publishing, and
retry on change. A conflict retry must not substitute a newer, unapplied
spec value. Publish via the CPC status subresource while preserving existing
conditions and resource lists.

HO uses the same after-apply rule for the CPO and CAPZ CPCs. For HO-owned
roles, HO must first persist the updated HCP spec and read its generation
before applying resources and recording `observedHCPGeneration`. HCCO currently
returns a combined list of guest Secret reconciliation errors; it must return
or retain per-role outcomes so one failed Secret does not advance that role.
The HCCO status path must be wired to the appropriate CPC and must not infer
success from a general resources-controller condition.

HO is the single writer of the HCP Azure identity rollup. It watches relevant
CPC status changes, validates source ownership and generation, patches only
the Azure identity field with optimistic locking, and avoids no-op writes.
The existing CPO `reconcileAzurePlatformStatus` spec mirror must stop
writing the authoritative Azure identity field; concurrent writers or a
whole-spec mirror would violate the guarantee. HO must re-read HCP after its
status patch before propagating to HC, or allow the next reconcile to do so.
Update generated CRDs, clients, deepcopy code, API documentation, and
vendored API copies together.

### Risks and Mitigations

- **Applied resources are not running credentials.** RP treats applied status
  as one stage and keeps its existing cleanup protection until later evidence
  and policy are delivered.
- **Partial failure and stale rollup.** Each source record advances
  independently; source deletion invalidates its rollup entry. RP checks the
  full expected consumer set, reference, source, HC generation, and HCP
  generation.
- **Same-name updates lack a spec delta.** CSI object version may provide a
  later observation; without a known target version, cleanup remains blocked.
- **Guest-cluster data-plane writes can fail independently.** HCCO reports
  each Secret only after its own successful write.
- **Fleet-scale watches and writes.** HO watches bounded CPCs and performs
  change-only rollup writes; measure status and watch load before GA.

### Drawbacks

The design adds a CPC status extension, an HO aggregator, and an HCCO
management-cluster publication path. It also requires changing the current
spec-mirror implementation in CNTRLPLANE-4493. This cost buys one HC status
surface for RP while retaining per-component application evidence.

## Alternatives (Not Implemented)

### Copy the desired Azure spec into HCP and HC status

The current CNTRLPLANE-4493 implementation writes a spec mirror before the
component apply loop. It can advertise a new identity even if application
fails. Renaming that field to `desired` would make it useful for diagnostics,
but it cannot satisfy this enhancement's applied-state contract.

### Require RP to mirror every CPC

Direct CPC reads retain source details, but RP currently mirrors HC and only
one CPC for another purpose. Mirroring every identity-bearing CPC adds
per-cluster ReadDesires and makes the backend maintain the role inventory.
An HO rollup lets RP consume one HC status object.

### Use a single completion boolean or timestamp

A cluster can have only some identities updated. A single flag hides which
consumer is blocking replacement.

### Query Key Vault on every reconcile

Per-reconcile external calls would add latency and load and still would not
prove that a pod mounted or used the returned credential version. CSI mount
status is the appropriate source for mount observations.

## Jira Delivery Alignment

| Task | Required outcome under this enhancement |
| --- | --- |
| [CNTRLPLANE-4493](https://redhat.atlassian.net/browse/CNTRLPLANE-4493) | Replace the early CPO spec mirror with after-apply CPC records and HO HCP/HC rollup. Do not release the mirror as an applied or active signal. |
| [CNTRLPLANE-4494](https://redhat.atlassian.net/browse/CNTRLPLANE-4494) | Add the KMS role from `spec.secretEncryption.kms.azure` after its own resource apply, for both supported auth modes. |
| [CNTRLPLANE-4495](https://redhat.atlassian.net/browse/CNTRLPLANE-4495) | Report all-current-pods mount completion and CSI object version where verifiable; define a separate cleanup gate. |
| [CNTRLPLANE-4496](https://redhat.atlassian.net/browse/CNTRLPLANE-4496) | Test both auth modes, CP and DP, KMS, partial failure, HC propagation, rotation, and no-rollout regressions. |
| [CNTRLPLANE-4509](https://redhat.atlassian.net/browse/CNTRLPLANE-4509) | Resolve API location, ownership, status semantics, propagation, and same-name rotation limitations in this proposal. |

The task descriptions for 4493 and 4495 currently promise more than their
proposed mechanisms can prove. Their acceptance criteria should be updated
before the epic is considered complete.

## Open Questions

1. Confirm the exhaustive role-to-CPC mapping, including components that share
   an identity and the exact HCCO CPC name, against the implementation.
2. Decide the supported workload-use or grace-period evidence that lets RP
   delete an old identity after all affected pods have mounted a new one.
3. Establish whether RP can supply an expected CSI object version for
   same-name Key Vault rotations and same-resource-ID identity recreation.
4. Verify the HCCO management-cluster CPC status permissions and the HO CPC
   watch/aggregation path in the supported topology.

## Test Plan

- Unit tests for owner/role mapping, per-role success and failure, skipped or
  disabled roles, generation changes during apply, conflict retry, no-op
  status patches, and deleted/recreated CPCs.
- API serialization and validation tests for bounded unique consumer/role keys,
  reference types, authentication modes, absent fields, and older clients.
- Integration tests proving no status before resource apply; previous values
  remain on failure; HO aggregates only current CPC observations; HC receives
  the HCP rollup. Include HO-owned resources and HCCO guest Secrets.
- Azure end-to-end tests for managed and workload identity creation and
  replacement, at least one control-plane and one data-plane role, KMS where
  enabled, and status visible through RP's HC ReadDesire. Assert status does
  not advance on injected apply failure.
- For CNTRLPLANE-4495, test missing/stale CSI pod status, mixed object
  versions, new/terminating pods, same-name content changes, and removal of
  completion when the expected pod set changes.

## Graduation Criteria

### Dev Preview -> Tech Preview

API approval, reviewed role inventory, corrected CNTRLPLANE-4493 semantics,
and tests proving after-apply status for CP and DP.

### Tech Preview -> GA

End-to-end rotation and propagation coverage for both auth modes, KMS
coverage, version-skew and downgrade tests, measured rollup cost, and a
documented RP cleanup policy that does not equate application or mounting
with successful credential use.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

On upgrade, new fields remain absent until each owning controller applies
its identity resources. An old HCCO or CPO may leave some roles absent; RP
treats the set as incomplete. An old HO may not aggregate CPC status or copy
it to HC, so RP keeps its existing cleanup protection.

On downgrade, older controllers may stop updating these status fields, and
an older CRD schema may prune them. Consumers treat absent, old-generation,
or unsupported records as unknown. Compatibility tests must verify behavior
for supported version pairs. No ignition, MachineConfig, or NodePool
config-hash input changes are proposed.

## Version Skew Strategy

HO and CPO can run different versions because CPO comes from the hosted
release payload. The rollup must never infer that a missing producer
completed a role. RP requires the expected role set for the cluster's
authentication mode and capabilities, not merely all records that happen
to be present. The producer should expose a schema/capability version or
equivalent feature gate so RP can distinguish unsupported status from a
temporarily missing observation.

## Operational Aspects of API Extensions

These are status-only additions. CPC records are written only when a role's
applied reference or observed HCP generation changes. HO writes the HCP
rollup only when its content changes; normal HCP-to-HC propagation follows.
Status publication errors surface as reconciliation errors and leave the old
observation intact. Watches, write rates, and HC status size should be
measured at fleet scale before GA.

## Support Procedures

Read `HostedCluster.status.platform.azure.identities`, identify the role
and source component, and compare its reference and HCP generation with
the current desired state. If absent or stale, inspect the CPC's
`status.azureIdentities`, the owner controller's reconcile errors, and the
actual SecretProviderClass, ServiceAccount, Deployment, or guest credential
Secret. Inspect CSI pod status separately for mount completion and workload
telemetry separately for credential use.

## Infrastructure Needed

HO needs CPC watches and an HCP Azure status aggregator. HCCO needs a
management-cluster CPC status publication path. RP can use its existing HC
ReadDesire once the rollup is propagated; it does not need a ReadDesire for
each CPC. No new external service is required.
