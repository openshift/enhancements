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
last-updated: 2026-10-29
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

Report the application of ARO HCP managed identities configured through a
HostedCluster (HC). Each control-plane component observes its applied
identity resources and records per-role results on its existing
ControlPlaneComponent (CPC). HCCO records data-plane results on
HostedControlPlane (HCP). HC exposes an aggregate `AzureIdentitiesApplied`
condition. Individual reporting is required for every in-scope identity
consumer: its desired reference comes from HC/HCP spec, and its status
exposes the last fully applied reference and current reconciliation outcome.
The HC aggregate supplements these records. These observations describe the
configured resources and completion points below; they do not prove that a
workload used a credential.

The ACR/NodePool design below remains provisional. The open scope decision is
whether RP should observe worker VM identity attachment directly through ARM,
or whether HyperShift should expose that evidence. This revision does not
settle that decision or assume that a separate status effort has an owner.

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

1. As an ARO HCP operator, I can compare the desired and last fully applied
   references and inspect the current outcome for each in-scope managed
   identity consumer separately, so I can identify completed, partial, and
   stalled replacements even when another role has not converged.
2. As an ARO HCP operator, I can tell whether all applicable HyperShift
   consumers have applied the current HC configuration, so I can advance the
   replacement workflow without mistaking a desired spec for applied state.
3. As an on-call engineer, I can distinguish resource application from mount,
   node attachment, and credential use, so I do not remove an old identity on
   insufficient evidence.

### Goals

1. Cover control-plane and data-plane managed identities configured through
   the ARO HCP HC, including optional KMS. Resolve ACR's reporting surface
   before finalizing its provisional scope.
2. Publish success only after observing all resources that carry a role's
   reference. On partial failure, update the condition and retain the last
   fully applied reference for diagnostics.
3. Preserve individual reporting for control-plane and data-plane identity
   consumers, including KMS. Each role exposes its last fully applied
   reference and a current condition that RP compares with the desired
   reference in spec. Keep control-plane detail on existing CPCs and
   data-plane detail on HCP, with an additional aggregate result on HC.
   Each role has one status owner; higher-level APIs do not mirror another
   owner's list.
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
   needs the consumer evidence and cleanup gate described below. The ACR
   worker attachment and cleanup scope remains an open decision.

## Proposal

### Workflow Description

1. RP writes the desired HC spec. HO persists the corresponding HCP spec
   before reconciling identity resources against that HCP generation.
2. HO, CPO, and HCCO reconcile their respective resources. The component's
   CPC reconciler observes the complete control-plane role resource set,
   including resources written by another controller. HCCO observes each
   guest Secret it reconciles and writes that role's result to HCP status.
   Each role has an `Applied` condition for the generation evaluated and an
   optional `lastAppliedReference` retained for diagnostics after failure.
3. In the provisional ACR path, the NodePool controller reads the ACR
   resource ID from HC, reconciles the AzureMachineTemplate, and reports
   `Applied=True` only after CAPI reports that all desired Machines use the
   new template and are available. An updated template alone is
   insufficient; this does not verify actual Azure VM attachment through
   ARM. An InPlace pool whose existing Machines retain the old template
   reports `Applied=False`, reason `UnsupportedInPlaceIdentityChange`, until
   the Machines are replaced or an attachment-update mechanism is delivered.
4. HO computes the expected consumers from the current HC authentication
   mode, KMS and ACR configuration, enabled capabilities, and NodePools. It
   correlates HC and HCP generations as described below. HC reports
   `AzureIdentitiesApplied=True` only when every applicable role has a
   trustworthy current-generation success with the expected reference, or
   positive evidence of requested removal.
5. RP reads the HC condition for cluster-wide progress. For individual
   identities it reads CPC control-plane records, HCP data-plane records,
   and the provisional ACR NodePool records using the same reference and
   condition rules. The HC message names laggards for people; RP never
   parses it. RP must also satisfy the compatibility and cleanup checks
   below before retiring an identity.

A record identifies a HyperShift consumer, not a distinct Azure resource.
One Azure identity reused by multiple roles must converge at every role and
consumer before RP considers its HyperShift application complete. RP matches
the configured ARM resource to the HC field(s) it supplied, then applies the
same comparison to all corresponding observations. No CS-specific control
plane, data plane, or service identity type changes the comparison rule.

### Expected roles, writers, and completion points

The following inventory defines the control-plane and data-plane observers;
the ACR rows describe the provisional HyperShift option. All named CPCs
represent existing workloads. Their existing component reconcilers are the
sole writers of their CPC status, including identity observations. Creating a
status-only CPC, opting a CPC out of normal rollups, or granting HCCO access
to another controller's CPC status is unnecessary.

A role reports success only after observing **all** listed resources with the
expected reference. A successful write response or a fresh read of matching
resources can establish application. Reading desired HCP spec alone cannot.
Cross-controller resources must be read from applied objects; lagging writes
leave the condition non-True. Required-resource changes enqueue the observer.

| HC role and reference | Required resources and observer |
| --- | --- |
| Control plane `cloudProvider` (`credentialsSecretName`) | CPO observes on `azure-cloud-controller-manager` CPC: `managed-azure-cloud-provider` SecretProviderClass, `azure-cloud-config` Secret containing `AADMSIDataPlaneIdentityPath`, and CCM Deployment reference. The separate `azure-cloud-config` ConfigMap carries the ACR ID. |
| Control plane `controlPlaneOperator` (`credentialsSecretName`) | HO observes on `control-plane-operator` CPC: `managed-azure-cpo` SecretProviderClass and CPO Deployment volume reference. |
| Control plane `nodePoolManagement` (`credentialsSecretName`) | HO observes on `capi-provider` CPC: `managed-azure-nodepool-management` SecretProviderClass, AzureClusterIdentity credential path, and a completed CAPZ Deployment rollout with the expected CSI object version mounted on every current CAPZ pod. |
| Control plane `imageRegistry` (`credentialsSecretName`) | CPO observes on `cluster-image-registry-operator` CPC: `managed-azure-image-registry` SecretProviderClass and operator Deployment reference, when enabled. |
| Control plane `ingress` (`credentialsSecretName`) | CPO observes on `ingress-operator` CPC: `managed-azure-ingress` SecretProviderClass and operator Deployment reference, when enabled. |
| Control plane `network` (`credentialsSecretName`) | CPO observes on `cluster-network-operator` CPC: `managed-azure-network` SecretProviderClass and network controller Deployment reference. |
| Control plane `disk` and `file` (separate `credentialsSecretName` values) | CPO observes on `cluster-storage-operator` CPC: disk requires `azure-disk-csi-config` Secret and `managed-azure-disk-csi` SecretProviderClass; file requires `azure-file-csi-config` Secret and `managed-azure-file-csi` SecretProviderClass. Each also requires its workload reference. Disk success does not advance file. |
| Data plane `imageRegistry`, `disk`, `file` (separate client IDs) | HCCO observes on HCP `status.azureIdentities`: respectively `openshift-image-registry/installer-cloud-credentials`, `openshift-cluster-csi-drivers/azure-disk-credentials`, and `openshift-cluster-csi-drivers/azure-file-credentials` guest Secrets. Registry is required only when enabled; disk and file are required. Managed-mode guest ingress is a placeholder and is excluded. |
| KMS (`spec.secretEncryption.kms.azure.kms.credentialsSecretName`) | CPO observes on `kube-apiserver` CPC: the shared `managed-azure-kms` SecretProviderClass, `azure-kms-config` Secret, CPO Deployment KMS volume/mount reference, and kube-apiserver workload/configuration reference. Required only when Azure KMS is configured. |
| ACR pull (`spec.platform.azure.containerRegistry.credentials.managedIdentity.resourceID`), provisional | CPO observes `UserAssignedIdentityID` in cloud config on `azure-cloud-controller-manager` CPC. Every NodePool reports the same resource ID after its AzureMachineTemplate and MachineDeployment rollout have converged. Required only when ACR pull identity is configured or being removed. |

`managed-azure-kms` is currently reconciled by both HO and CPO. Collapsing its
status onto `kube-apiserver` does not transfer resource ownership. Both
resource writers must use the persisted HCP reference, including the pinned
version, and reconcile identical SPC content. The CPO observer checks the
entire KMS resource set, including the HO-managed CPO mount, before reporting
success. Supported version-skew tests must verify that neither writer
reverts the other's pinned version. A new HO with a CPO that still writes
an unpinned SPC is not a supported combination for identity replacement;
the CPO fix must be backported or the hosted release upgraded before enabling
replacement. A transient matching observation cannot make that pair safe.

HCP data-plane records are HCCO's original observations of guest resources,
not copies of CPC status or desired spec. Individual reporting is required
for registry, disk, and file: RP must distinguish each role's progress and
last fully applied client ID. One aggregate condition could gate an operation
that waits for all three roles together, but would not provide the required
individual observations. HCP therefore has at most three data-plane records;
HC remains the only cluster-wide aggregate. When one Azure identity is reused
by several roles, RP requires success for every consumer of that identity;
a successful role does not imply that the other consumers have switched.

A role that was never enabled is outside the expected set. Existing
capability immutability rules are unchanged. For a supported removal, keep
the former consumer in the set until its observer verifies teardown and sets
`Applied=True`, reason `Removed`, for the current generation. A deleted CPC
or an absent record is not proof of teardown. If the existing component
lifecycle deletes a CPC before that observation can be published, support
for that removal requires a teardown/status ordering change; HO must remain
non-True until positive evidence exists. This does not introduce support for
otherwise immutable spec transitions.

In the provisional ACR path, removal requires a completed machine rollout
without the old identity. A deleting NodePool remains in the expected set
until its Machines are gone: the existing NodePool finalizer calls
`ensureMachineDeletion` before it is removed. HO tracks the deleting NodePool
by UID and ignores a same-name replacement until the prior UID's Machine
deletion is observed. If the finalizer contract changes, a durable UID-keyed
tombstone must preserve this fence. New NodePools enter the set immediately
and make HC non-True until they converge. A cluster with no NodePools has no
worker attachment to check.

### API Extensions

Add an optional `credentialsSecretVersion` to the HC Azure managed-identity
reference (including KMS), and propagate it to HCP. It is the immutable Key
Vault object version RP expects for `credentialsSecretName`, not a
certificate value. RP must set it for every new Key Vault-backed reference
or replacement; changing the version changes HC generation. The field is
optional for API compatibility, but a legacy name-only reference remains
`Unknown/VersionUnavailable` until RP supplies the version. The SPC writer
maps `credentialsSecretVersion` to the CSI provider's existing `objectVersion`
field. Status uses `credentialsSecretVersion` too. Client IDs and ACR
resource IDs do not have a Key Vault version.

The completion check also compares the applied SPC's Key Vault location and
`objectEncoding` with the values derived from the persisted HCP, including
`managedIdentitiesKeyVault`. Name/version equality alone is insufficient.
A change to these inputs requires a new current-generation observation of
every affected role even if its identity reference is unchanged. They remain
configuration in spec rather than duplicated identity-reference members.

Add a bounded `status.azureIdentities` map list to CPC for control-plane
roles and to HCP for HCCO's data-plane roles. The provisional NodePool
`status.acrPullIdentity` has the same observation fields. HC adds the
standard `AzureIdentitiesApplied` condition without a per-role list.

Each role has standard `metav1.Condition` entries keyed by type, initially
with one type, `Applied`. There is no separate `state`, `lastAttempt`, or
successful-apply generation. `lastAppliedReference` is optional and changes
only when the complete resource set has been observed with that reference.
It remains available after a failure or removal as historical information.
It must never be interpreted as the current state without checking the
current `Applied` condition. Example, with disk complete and file failing:

```yaml
kind: ControlPlaneComponent
metadata:
  name: cluster-storage-operator
status:
  observedGeneration: 42
  azureIdentities:
  - scope: ControlPlane
    role: Disk
    lastAppliedReference:
      type: CredentialsSecretName
      credentialsSecretName: disk-identity-v2
      credentialsSecretVersion: 4d71...
    conditions:
    - type: Applied
      status: "True"
      reason: AsExpected
      observedGeneration: 42
      lastTransitionTime: "2026-10-29T10:00:00Z"
      message: All required disk identity resources match
  - scope: ControlPlane
    role: File
    lastAppliedReference:
      type: CredentialsSecretName
      credentialsSecretName: file-identity-v1
      credentialsSecretVersion: 29ab...
    conditions:
    - type: Applied
      status: "False"
      reason: ApplyFailed
      observedGeneration: 42
      lastTransitionTime: "2026-10-29T10:00:00Z"
      message: Failed to update the file identity SecretProviderClass
```

```yaml
kind: HostedControlPlane
status:
  azureIdentities:
  - scope: DataPlane
    role: Disk
    lastAppliedReference:
      type: ClientID
      clientID: 00000000-0000-0000-0000-000000000001
    conditions:
    - type: Applied
      status: "True"
      reason: AsExpected
      observedGeneration: 42
      lastTransitionTime: "2026-10-29T10:00:00Z"
      message: Guest disk credentials Secret matches
```

```yaml
kind: NodePool
metadata:
  generation: 7
status:
  acrPullIdentity:
    observedHCGeneration: 28
    lastAppliedReference:
      type: ResourceID
      resourceID: /subscriptions/.../userAssignedIdentities/acr-v2
    conditions:
    - type: Applied
      status: "True"
      reason: AsExpected
      observedGeneration: 7
      lastTransitionTime: "2026-10-29T10:00:00Z"
      message: All desired Machines use the expected template and are available
```

```yaml
kind: HostedCluster
status:
  conditions:
  - type: AzureIdentitiesApplied
    status: "False"
    reason: ApplyFailed
    observedGeneration: 28
    lastTransitionTime: "2026-10-29T10:00:00Z"
    message: cluster-storage-operator/File could not apply its identity
```

List keys are `(scope, role)`. Scope values are `ControlPlane` and
`DataPlane`; role values are the PascalCase forms of the inventory names,
including `KMS` and `ACRPull`. Each CPC accepts only its assigned roles; HCP
accepts only `DataPlane/ImageRegistry`, `DataPlane/Disk`, and `DataPlane/File`.
The largest CPC list is the two storage roles or the two CCM roles. HCP has
at most three records. The SMI creates no service-scope record.

The reference is a discriminated union: `CredentialsSecretName` requires
`credentialsSecretName` and `credentialsSecretVersion`; `ClientID` requires
`clientID`; `ResourceID` requires `resourceID`. Exactly the matching member
is populated, and the version is allowed only with `CredentialsSecretName`.
Validation rejects empty values, duplicate keys, unsupported scope/role
pairs, and more than one condition of a given type. Status contains no
certificate, token, or secret value. CNTRLPLANE-4493's spec-mirror fields must
not coexist as a second authoritative applied-state signal.

#### Condition transitions

`Applied` describes whether the desired role configuration, including a
requested removal, has been observed at its defined completion point.
Every outcome carries the generation actually evaluated, including failures.

| Observation for the evaluated generation | Applied status / reason | Last applied reference |
| --- | --- | --- |
| All required resources match the desired reference and completion criteria | `True / AsExpected` | Set to the observed reference |
| Requested removal is positively observed across all required resources | `True / Removed` | Retain the previous reference as history, if any |
| First reconciliation or a known incomplete write/rollout | `False / Applying` | Absent before first success; otherwise retained |
| A required resource apply fails | `False / ApplyFailed` | Retained; may no longer describe every live resource |
| InPlace ACR replacement/removal cannot update existing Machines | `False / UnsupportedInPlaceIdentityChange` | Retained; retrying the same unsupported operation does not imply progress |
| The observer cannot read a required resource or establish completion | `Unknown / ObservationUnavailable` | Retained |
| A Key Vault-backed desired reference lacks its immutable version | `Unknown / VersionUnavailable` | Retained |

`ApplyFailed` is retryable. `UnsupportedInPlaceIdentityChange` persists until
the pool is replaced or a supported attachment-update mechanism is used;
it is a condition reason, not a terminal state in a second state machine.
An absent condition means no usable observation. `lastTransitionTime`
changes only when the condition status changes and is not a progress counter.

#### HC and HCP generation correlation

CPC and HCP role conditions use the generation of the HCP they evaluated,
consistent with the existing CPC `status.observedGeneration` convention.
The provisional NodePool role condition uses the NodePool generation, with
`observedHCGeneration` recording the HC input evaluated in the same attempt.
This also invalidates success when a NodePool changes without an HC change.
The HC generation field is an input correlation field, not a separate
successful-apply generation. HC's aggregate uses HC generation. These
source-object semantics must be stated in the API field comments.

For each aggregate reconciliation, HO:

1. Reads HC at UID/generation `(C, c)` and persists its desired HCP
   projection, including managed identities, Key Vault versions, KMS, ACR,
   authentication mode, and capabilities. It captures the returned HCP
   UID/generation `(P, p)`. HC and HCP generation numbers need not be equal.
2. Verifies the applied HCP projection still corresponds to that HC
   snapshot. CPC records must belong to HCP UID `P`; HCCO observations come
   from that same HCP. Both need `Applied.observedGeneration == p`. NodePool
   records must belong to HC UID `C`, use `observedHCGeneration == c`, and
   have an Applied condition for the current NodePool generation.
3. Requires `Applied=True/AsExpected` **and** equality between the desired typed
   reference and `lastAppliedReference`. For removal, requires
   `Applied=True/Removed` and desired absence instead of reference equality.
   A condition from another generation is unusable even if its reference
   matches. A failed current-generation condition cannot be masked by a
   retained matching reference.
4. Rechecks the HC/HCP snapshots and applicable consumer membership before
   publishing. If they changed, it retries instead of attributing the result
   to a newer HC generation. It writes the HC condition with
   `observedGeneration: c` using optimistic concurrency. Consumers compare
   that generation with the current HC before trusting the result.

There is no persisted assumption that HC generation 28 always maps to HCP
42; the mapping is established from the persisted projection on each pass.
An HC change that leaves the HCP projection unchanged can reuse the same
HCP generation. An unrelated HCP spec change conservatively requires a new
whole-role observation. Writers and observers must use persisted HCP state;
HO must not tag writes made from an unpersisted HC projection with an old
HCP generation.

CPC `status.observedGeneration` correctly advances after an attempted
reconcile, including a failed one. Success is expressed by the per-role
condition. During A -> B -> A, the old A observation cannot satisfy the new
HCP generation; the observer must inspect the complete resource set again.
The separate role records exist for disk/file independence and retained
last-good diagnostics, not to redefine `observedGeneration` as success.

#### Reporting compatibility and aggregate result

Use standard API versioning and per-role `Applied` conditions. No reporter
schema marker, controller revision, or observer Deployment lookup is part of
a role record or the aggregation algorithm. Each condition describes the
resources observed for its source generation; `lastAppliedReference` keeps
that role's last complete reference. Individual reporting is unchanged.

Identity replacement and cleanup may consume this API only with a deployed
HO/CPO/HCCO combination that supports the required reporting and immutable
Key Vault versions. Establish those supported combinations through release
qualification before enabling RP consumption. A supporting controller
upgrade does not by itself invalidate an observation of unchanged resources;
when the desired HCP generation changes, each role must observe it again.
The normal component conditions continue to report workload health and
rollout progress independently of the identity conditions.

HO reports `Unknown/VersionUnavailable` when a required immutable version is
missing, then `Unknown/StatusUnavailable` if any expected observation is
absent, stale, or unreadable. A success whose reference or removal reason
contradicts the desired configuration is also unusable. When all required
sources are usable and current, precedence is
`False/UnsupportedInPlaceIdentityChange`, `False/ApplyFailed`,
`False/Applying`, then `True/AsExpected`. Individual records retain their
own reasons even when another role determines the aggregate reason.

Generation checks do not detect a controller rollback that stops reporting
without changing the desired spec. Such a controller can leave an old True
condition intact, including on HC if HO loses aggregation support. Before
any exceptional rollback outside the qualified supporting versions, RP
must suspend identity replacement and cleanup. Recovery requires verifying
supporting controllers and fresh observations before resuming; retained True
conditions alone are insufficient. This proposal does not add automatic
invalidation for that rollback scenario. Agreement on this operational
limitation and recovery procedure remains an explicit review question.

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
start with an empty credential cache. HO's `capi-provider` CPC observer sets
`Applied=True` only after the new Deployment revision is available, no
old-revision CAPZ pods remain, and every current CAPZ pod's CSI
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
condition alone must never trigger cleanup. ARO-29197 excludes worker-node
identity cleanup. The owner and reporting surface for that work are still
being confirmed. In the provisional HyperShift path, NodePool application
requires a completed worker rollout and does not prove actual ARM attachment
or image pull success.

### Topology Considerations

#### Hypershift / Hosted Control Planes

CPC control-plane detail, HCP data-plane detail, provisional NodePool detail,
and the HC condition live in the management cluster. HCCO observes guest
Secrets through its guest client and patches its own HCP status fields
through its management client. HO watches these sources and updates the HC
condition on meaningful changes. RP's existing HC ReadDesire supplies the
aggregate; per-role reads require CPC, HCP, and provisional NodePool access.
The HC message is for support, never a machine API.

#### Standalone Clusters

Not applicable: this API targets ARO HCP.

#### Single-node Deployments or MicroShift

No MachineConfig or ignition change is proposed.

#### OpenShift Kubernetes Engine

Never-enabled optional components are excluded. Existing capability
immutability is unchanged. Any supported removal needs positive teardown
evidence at the current generation before its consumer leaves the set.

### Implementation Details/Notes/Constraints

Extend the existing component reconcilers with role-specific observation
hooks. HO owns observations on `control-plane-operator` and `capi-provider`;
CPO owns the other listed CPCs. A hook compares applied resources against the
persisted HCP reference and sets the role condition after checking the full
resource set. Observation is repeated after relevant Secret, SPC, workload,
or mount-status changes, even when HCP generation has not changed.

Of the managed Azure identity resources, `azure-kms-config` is currently
written by the critical `SecretEncryptionSync` step before `CoreHCPChain`.
The CPO, node-pool-management, and KMS SPC writes already follow HCP
persistence in `reconcilePlatformSpecific`. Move only the managed Azure KMS
configuration write after successful HCP persistence and derive it from that
persisted spec; retain its critical error handling before dependent work.
Other encryption modes keep their existing ordering. Initial cluster creation
must write the Secret without waiting for kube-apiserver readiness, since
kube-apiserver needs it to start. On upgrade, a failed write preserves the
existing Secret, leaves KMS non-True, and is retried. Tests must cover creation
and upgrades with injected failures at HCP persistence and Secret writes.

Storage reports disk and file independently. HCCO returns per-Secret
outcomes and patches only its data-plane HCP records with conflict retries,
preserving HCP status maintained by other controllers. HCCO never patches a
CPC. The existing `hosted-cluster-config-operator` CPC continues normal
workload availability/version reporting. All existing CPCs remain in the
normal rollups; no opt-out label or synthetic component is added.

The current CPO `reconcileAzurePlatformStatus` spec mirror stops writing the
authoritative Azure identity field. HO aggregates the current CPC/HCP and
provisional NodePool observations using the documented generation mapping,
references, and applicability within the qualified supporting versions. It
does not copy control-plane records to HCP or any per-role records to HC.

The provisional NodePool path already watches HC and creates a new
AzureMachineTemplate name when the ACR resource ID changes. Its new record
reports success only after CAPI reports all desired Machines on the intended
template and available. Template creation, a changed MachineDeployment
reference, or a generic NodePool Ready condition alone is insufficient.
CAPZ `VMSpec.Parameters` returns no update parameters for an existing VM,
and the InPlace path only changes the MachineSet template reference.
`Applied=False/UnsupportedInPlaceIdentityChange` therefore persists while
existing Machines retain the old template. The operator can replace that
pool with a Replace-mode pool under the existing node availability policy;
changing `UpgradeType` on an existing pool is unsupported. Automatic
in-place VM updates require separate design and tests. ACR cloud config and
all applicable NodePools must agree before HC reports True, with positive
removal observations for unset identities.

A partial resource update sets a non-True condition at the evaluated
generation while retaining `lastAppliedReference`. The historical reference
does not imply every live resource still uses it. Status update failures
leave prior data intact; readers reject stale generations.
Same-generation resource drift is detected by watches and reconciliation,
so this status remains an asynchronous observation, not an atomic guarantee
against subsequent resource changes. RP must serialize replacements and
apply its final cleanup policy.

Generated CRDs, clients, deepcopy code, API documentation, and vendored
copies change together during implementation.

### Risks and Mitigations

- **Applied is not used.** Keep RP cleanup protection until a separate policy
  accounts for mounts, workload use, and Azure propagation.
- **Partial writes or rollback.** Retain last-good references for diagnostics;
  require current-generation `Applied=True` after a whole-role observation.
- **Rollback below reporting support.** Suspend RP replacement and cleanup
  before the rollback. Same-generation retained status cannot detect this
  case; recovery requires operational verification before resuming.
- **CAPZ stale credentials.** Restart for a path or version change, wait for
  old pods to exit, and verify each new pod's mounted version. Retain a
  separate upstream cache fix.
- **Existing worker VMs.** In the provisional ACR path, report the unsupported
  InPlace operation explicitly and retain the old identity until the worker
  workflow has trustworthy attachment and cleanup evidence.
- **Role removal.** Require positive teardown evidence; a missing CPC or
  NodePool is insufficient. Do not imply support for immutable transitions.
- **Fleet scale.** Bound CPC and HCP records, avoid mirrored lists and no-op
  status writes, and measure watch/update rates before GA.

### Drawbacks

RP needs CPC and HCP reads for per-identity detail and NodePool reads if the
HyperShift ACR path is adopted. Implementation adds component observation
hooks, HCCO-owned data-plane records, and an HO aggregator. An HC-only reader
can wait for cluster-wide convergence, but one unrelated stalled role delays
its progress. Exceptional rollback below reporting support needs an
operational suspension and recovery procedure; the API cannot automatically
detect retained success from a controller that no longer updates it.

## Alternatives (Not Implemented)

### Copy per-role status to HCP and HC

Mirroring all control-plane records onto HCP and HC duplicates observations
and increases status write/watch traffic. HCP stores only HCCO's original
data-plane observations; HC stores only the aggregate condition.

### Copy desired Azure spec into status

The current CNTRLPLANE-4493 mirror can advertise a new identity before any
resource apply. It is useful only as desired-state diagnostics and must not
be presented as applied or active state.

### Use only CPC observedGeneration and applied value

`status.observedGeneration` means the component processed a generation,
including a failed reconciliation. Pairing it with a current-generation
success condition is the correct way to distinguish attempted from applied.
This proposal uses that convention. A single component-wide result still
cannot distinguish disk success from file failure or preserve each role's
last-good reference. Those requirements justify per-role records; no bespoke
successful-apply generation field is needed.

### Use one data-plane condition on HCP

A single current-generation condition could gate an operation that waits
for all guest Secrets to match. It cannot expose individual role progress
or retain each role's last applied client ID. Individual identity-consumer
reporting is a requirement, so HCCO writes at most three per-role
observations on HCP. An alternative API shape must preserve those individual
observations, including the case where disk has converged but file has not.

### Track the observing controller's build in each role

Per-record controller revisions, schema markers, and Deployment lookups add
API and lifecycle machinery to every observation. They are unnecessary for
individual reporting or generation checks during qualified upgrades that
retain reporting support. A Deployment template hash also repeats across
an A -> B -> A build change, so it is not a complete invalidation mechanism.
This design uses standard conditions and explicitly documents the operational
limitation for exceptional rollback below reporting support. This choice
does not remove the CAPZ consumer rollout and mounted-version checks.

### Query Key Vault on every reconcile

External calls add latency and load and cannot show which pods mounted or
used the returned credential. Mount evidence belongs to the later phase.

## Jira Delivery Alignment

| Task | Required outcome under this enhancement |
| --- | --- |
| [CNTRLPLANE-4493](https://redhat.atlassian.net/browse/CNTRLPLANE-4493) | Replace the early CPO spec mirror with observations on existing workload CPCs and HCCO-owned data-plane HCP records. Use per-role Applied conditions; do not release the mirror as an applied signal. |
| [CNTRLPLANE-4494](https://redhat.atlassian.net/browse/CNTRLPLANE-4494) | Observe the full KMS resource set on kube-apiserver CPC, including shared SPC, config Secret, and CPO mount. Verify shared writers preserve the pinned version during supported skew. |
| [CNTRLPLANE-4495](https://redhat.atlassian.net/browse/CNTRLPLANE-4495) | Deliver per-pod mount/version evidence for non-CAPZ CSI consumers and the RP cleanup gate; resource-applied status alone cannot satisfy ARO-29197 requirement 3. |
| [CNTRLPLANE-4496](https://redhat.atlassian.net/browse/CNTRLPLANE-4496) | Test CP, DP, KMS, CAPZ rollout/cache reset, generation correlation, partial failure, teardown, supported upgrades/skew, and RP visibility. Finalize ACR coverage after its scope decision. |
| [CNTRLPLANE-4509](https://redhat.atlassian.net/browse/CNTRLPLANE-4509) | Deliver versioned HC Key Vault references, SPC `objectVersion` pinning, and same-name rotation status semantics before identity replacement implementation. |
| [CNTRLPLANE-4491](https://redhat.atlassian.net/browse/CNTRLPLANE-4491) | Align acceptance criteria with existing CPC observations, HCCO-owned HCP detail, and HC aggregation; replace the superseded HC/HCP spec mirror. Confirm the ACR reporting scope. |
| [ARO-29197](https://redhat.atlassian.net/browse/ARO-29197) | RP owns the final cleanup gate after evidence for all current CP/DP consumers. Worker-node cleanup is excluded; its ownership and reporting plan remain to be confirmed. |

## Open Questions

1. Which workload-use evidence or grace period should RP require after every
   affected pod or machine has converged? ARO-29197 must resolve this before
   it deletes old identities; `AzureIdentitiesApplied` never answers it.
2. For ACR, can production RP credentials enumerate all relevant worker VMs
   and verify that the new identity is attached and the old one can safely
   be retired? If RP can own this observation, should the provisional
   NodePool status/API be removed from this enhancement? Otherwise, should
   HyperShift expose actual attachment evidence here or in a separate
   enhancement? Confirm ownership and tracking with Todd and Bryan.
3. Does the API review accept HCCO-owned per-role observations on HCP as the
   representation of required individual data-plane reporting? The API shape
   is open for agreement; individual progress and last applied references
   must be preserved.
4. Can Bryan and the RP owners accept operational suspension and recovery
   for an exceptional rollback below reporting support, instead of an API
   mechanism that automatically invalidates retained status? Agree how
   recovery verifies fresh observations before re-enabling cleanup.
5. For supported role removals, which paths can delete their CPC before a
   positive removal observation is consumed? Resolve the teardown/status
   ordering or a durable evidence location before enabling those paths.
   Immutable capability changes are not supported removal operations.

## Test Plan

- Unit tests for inventory, capability gating, observed applied resources,
  disk/file independence, current-generation failures, A -> B -> A identity
  changes, historical-reference retention, and positive removal evidence.
- API validation tests for bounded map lists, condition types, typed
  references, scope/role ownership, version naming, and old-client behavior.
- Aggregator tests for HC/HCP generations that differ, HC changes that leave
  HCP unchanged, unrelated HCP changes, stale reference matches, object UID
  replacement, concurrent spec changes, and condition-reason precedence.
- Integration tests with injected Secret/SPC failures, persisted-HCP
  ordering, cross-controller KMS observation and version pinning, HCCO guest
  reads/HCP patches, Key Vault location/encoding changes, KMS Secret ordering
  during creation and upgrade, and untouched CPC availability/version rollups.
- Supported-upgrade and skew tests with both controllers retaining reporting,
  missing or pruned status fields, and independently advancing role conditions.
  Verify that missing observations remain Unknown and that legacy KMS writers
  cannot be qualified for replacement while they can undo version pinning.
  Exercise RP's rollback suspension/recovery procedure once it is agreed;
  do not assert automatic stale-status invalidation from generation alone.
- Provisional ACR tests for add/replacement/removal, zero and multiple pools,
  template creation versus completed CAPI rollout, NodePool-only spec
  changes, unsupported InPlace changes, Replace-mode pool replacement,
  deletion finalizer fencing, and
  stalled rollout. Final attachment tests depend on the ACR scope decision.
- CAPZ tests for path and same-path version changes, Deployment restart,
  old-pod termination, current-pod mounted versions, failed/missing CSI status,
  missing CRDs or read permissions, and retained old cache before restart.
  A same-name reference without a new expected version cannot report True.
- Azure end-to-end tests for RP reads of HC aggregation and per-role CPC/HCP
  detail. Verify that disk success remains individually observable while
  file is still applying or failing, that each role retains its own last
  fully applied reference, and that a reused identity requires observations
  from all of its consumers. None of these observations alone authorizes
  old-identity deletion.

## Graduation Criteria

### Dev Preview -> Tech Preview

API approval, role inventory and observer ownership, and tests that prove
observation of CP, DP, and KMS resources. Resolve the ACR scope and reporting
owner before accepting its provisional API and associated tests.

### Tech Preview -> GA

End-to-end replacement coverage, supported-upgrade and version-skew tests,
measured status cost, and a documented RP cleanup policy. Agree and validate
the operational suspension/recovery procedure for exceptional rollback below
reporting support. If the HyperShift ACR path is selected, complete its
attachment-evidence design and end-to-end tests;
CAPI template convergence alone does not prove actual ARM attachment.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

New records appear when their observer first evaluates the role, including
non-True conditions before first success. Missing records or fields produce
`Unknown/StatusUnavailable` at the aggregator. Older CRDs may prune new
fields; absence is not success. Deploy compatible CRDs and supporting
HO/CPO/HCCO versions before enabling RP consumption.

Normal supported upgrades retain reporting. A desired HCP generation change
requires a new per-role observation; a restart or controller build change
with unchanged desired resources does not alone invalidate a prior
observation. No new controller-revision field or API heartbeat is introduced.

An exceptional rollback below reporting support can preserve stale True
conditions. RP must suspend replacement and cleanup before that rollback,
including when HO itself loses aggregation support. Recovery must verify
supporting versions and fresh observations before resuming. This is an
operational precondition, not an automatic guarantee supplied by this API;
its recovery procedure remains subject to reviewer and RP agreement.

## Version Skew Strategy

CPO follows the hosted release and can differ from HO. Qualify the supported
HO/CPO/HCCO combinations for reporting and pinned SPC versions. For those
combinations, HO uses current source generations, reference equality, and
the persisted HC/HCP correlation. An expected role without a usable current
condition remains Unknown and stays in the expected set. RP applies the
same checks when reading individual records; it does not parse messages.

Both KMS writers must preserve identical versioned SPC content. An older CPO
that can undo pinning requires a backport or hosted-release upgrade before
replacement is enabled. Merely observing the new HO's write once is
insufficient. Version-skew tests must verify continued agreement across
reconciliation by both writers.

## Operational Aspects of API Extensions

CPC and HCP records, and provisional NodePool records, change only when
references, conditions, or evaluated generations change.
HO updates HC only on meaningful changes. Avoid periodic no-op status writes.
Measure (1) time from a desired generation change to each role's success
(`AsExpected` for application, `Removed` for removal) and to HC aggregate
convergence, (2) counts and ages of non-True role conditions, and (3) CPC,
HCP, NodePool, and HC status writes per cluster per hour. A condition's lastTransitionTime
is not the timestamp of every attempt; RP uses its replacement operation
state and observed changes when deciding that progress has stalled.
HyperShift maintainers measure status cost and convergence in fleet tests;
RP owners define and calibrate alert thresholds before GA. These thresholds
are operational policy rather than HyperShift API or cleanup deadlines.

HyperShift on-call owns missing observations, reconciliation failures, and
CAPZ rollout; the provisional NodePool path also requires rollout support.
ARO RP on-call owns desired version supply, replacement orchestration,
alerting policy, and the final cleanup gate. Alert timing is RP operational
policy, not an API guarantee or cleanup deadline. Azure platform maintainers
own the upstream CAPZ cache follow-up. Delayed status retains the old
identity; a late observer alone does not make the cluster unavailable.

## Support Procedures

Inspect HC `AzureIdentitiesApplied` reason and message. Read the named CPC's
`status.azureIdentities`, HCP's data-plane records, or provisional NodePool
`status.acrPullIdentity`. Check the current condition, its source generation,
the expected typed reference, and the configured supporting versions. A
historical `lastAppliedReference` is not success. Inspect the role's Secret, SPC, cloud
config, guest Secret, or AzureMachineTemplate and check mount/use evidence
separately.

## Infrastructure Needed

- HO needs bounded CPC, HCP, and provisional NodePool watches and an HC
  condition reconciler. Component observers need read/watch access to their
  listed Secrets, SPCs, workloads, and other completion resources.
- CAPZ observation depends on the Secrets Store CSI driver's
  `SecretProviderClass` and `SecretProviderClassPodStatus` CRDs in the
  management cluster. Register their types and configure HO get/list/watch
  RBAC for `secretproviderclasses` and `secretproviderclasspodstatuses` in
  `secrets-store.csi.x-k8s.io`, plus the relevant Deployments, ReplicaSets,
  and Pods. Map mount-status events to the owning CAPZ role. Missing CRDs,
  denied reads, or absent per-pod evidence must produce Unknown rather than
  success or an unhandled controller startup failure.
- HCCO uses guest Secret reads and management-cluster HCP status updates;
  extend its existing HCP status patch path while preserving other writers'
  fields. No CPC status permission or field-level RBAC is introduced.
- RP needs CPC/HCP and provisional NodePool reads for individual progress;
  its existing HC read supplies the aggregate. Release and deployment
  procedures must establish reporting support before enabling consumption
  and suspend replacement/cleanup before an exceptional rollback below it.
  No observer Deployment reads or controller-revision injection are added
  for identity status validation.

No new external service is required. These watches and RBAC changes are
implementation prerequisites, including the CAPZ mounted-version dependency.
