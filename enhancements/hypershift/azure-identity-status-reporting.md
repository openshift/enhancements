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
last-updated: 2026-09-24
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

This enhancement adds `status.platform.azure` to the `HostedCluster` and
`HostedControlPlane` resources in HyperShift. The new status field reflects the Azure
identity configuration that the control plane operator (CPO) is actively applying to
control plane components. The primary consumer is ARO HCP, which needs this signal to
determine when identity replacements have taken effect so that it can safely clean up
credentials associated with old managed identities.

## Motivation

ARO HCP supports day-2 managed identity replacement: when a control plane identity is
rotated, Clusters Service updates the `credentialsSecretName` fields in the
`HostedCluster` spec. The CPO reconciles those changes into `SecretProviderClass`
objects; the Secrets Store CSI driver then mounts the new certificates. Without a
status signal from HyperShift, ARO HCP cannot observe when the CPO has applied the
new configuration and must instead rely on conservative time-based delays (currently
24 hours) before cleaning up old identity credentials — even though the CPO typically
applies changes within one reconcile loop.

This enhancement is tracked in [OCPSTRAT-2151](https://issues.redhat.com/browse/OCPSTRAT-2151)
and unblocks [ARO-29197](https://issues.redhat.com/browse/ARO-29197).

### User Stories

#### Story 1: ARO HCP identity rotation clean-up

As an ARO HCP platform operator, I want to observe when the HyperShift control plane
operator has applied a new identity configuration to `SecretProviderClass` resources so
that I can safely clean up Key Vault credentials associated with the old managed identity
without relying on a conservative, time-based delay.

#### Story 2: Detecting stale identity configuration

As an ARO HCP operator or SRE, I want to compare `status.platform.azure` against
`spec.platform.azure.azureAuthenticationConfig` to detect clusters where identity
configuration changes have not yet been applied by the CPO, so that I can investigate
or alert on stuck reconciliation.

#### Story 3: Self-managed Azure workload identity visibility

As a self-managed Azure cluster operator, I want to observe which workload identity
client IDs the CPO has applied to `ServiceAccount` annotations for each control plane
component, so that I can verify that OIDC federation is configured correctly after
a cluster update.

### Goals

1. Report the active Azure identity configuration being applied by the CPO in
   `HostedCluster.Status.Platform.Azure` and `HostedControlPlane.Status.Platform.Azure`.
2. Cover both authentication modes: `ManagedIdentities` (ARO HCP) and `WorkloadIdentities`
   (self-managed Azure).
3. For `ManagedIdentities` mode: report the `credentialsSecretName` per control plane
   component (the operative rotation signal) and the MSI client IDs for data plane
   components.
4. For `WorkloadIdentities` mode: report the workload identity `clientID` per component.
5. Populate the status on every CPO reconcile using `statuspatching.PatchStatus` with
   optimistic locking, consistent with existing HCP status patching patterns.

### Non-Goals

1. **Pod-level confirmation.** The CPO can confirm it has updated `SecretProviderClass`
   objects but cannot confirm that pods are running with the new credentials. Pod-level
   confirmation (via `SecretProviderClassPodStatus`) is deferred to a follow-on
   enhancement (CNTRLPLANE-4495).
2. **Same-name Key Vault rotation detection.** If a Key Vault secret is replaced in-place
   (same `credentialsSecretName`, new certificate content), HyperShift cannot detect this
   without per-reconcile Key Vault API calls. This is not a valid scenario in the ARO HCP
   identity replacement protocol, which always uses a distinct `credentialsSecretName` per
   new identity.
3. **KMS managed identity.** The KMS identity is configured under
   `spec.secretEncryption.kms.azure.kms`, not under `spec.platform.azure.azureAuthenticationConfig`.
   Its status reporting is deferred (CNTRLPLANE-4494).
4. **Verifying that identities work against Azure APIs.** Out of scope.

## Proposal

### Workflow Description

**Actors:**
- **Clusters Service (CS):** Updates `HostedCluster.Spec.Platform.Azure.AzureAuthenticationConfig`
  to rotate managed identities.
- **HyperShift Operator (HO):** Creates/updates the `HostedControlPlane` from the
  `HostedCluster` spec.
- **Control Plane Operator (CPO):** Reconciles the `HostedControlPlane` and applies the
  identity configuration to `SecretProviderClass` (ManagedIdentities) or `ServiceAccount`
  (WorkloadIdentities) resources.
- **ARO HCP backend:** Reads `HostedCluster.Status.Platform.Azure` to determine when an
  identity rotation has been applied by the CPO.

**Rotation workflow:**

1. CS updates the `HostedCluster` spec with new `credentialsSecretName` values for the
   rotated components.
2. The HO propagates the updated spec to `HostedControlPlane`.
3. On the next reconcile, the CPO reads the new `credentialsSecretName` values, updates
   the corresponding `SecretProviderClass` objects, and then calls
   `reconcileAzurePlatformStatus` to mirror the active configuration into
   `HostedControlPlane.Status.Platform.Azure` via `statuspatching.PatchStatus`.
4. The HO copies `hcp.Status.Platform` to `hcluster.Status.Platform` — this already
   happens generically and requires no additional code.
5. ARO HCP reads `HostedCluster.Status.Platform.Azure.ManagedIdentities.ControlPlane`
   and compares each component's `credentialsSecretName` against what it wrote in step 1.
   When they match, the CPO has applied the rotation and ARO HCP can initiate clean-up
   of the old identity's credentials.

**Rotation contract:** The operative rotation signal is a `credentialsSecretName` change
in the spec. ARO HCP must use a new Key Vault secret name when rotating to a new identity;
the status will reflect it once the CPO has reconciled. Same-name Key Vault rotation (new
certificate uploaded to the same secret name) is handled transparently by the CSI driver
and is out of scope for this status signal.

### API Extensions

This enhancement adds five new types to the HyperShift API and one new field on
`PlatformStatus`.

#### New field on PlatformStatus

```go
type PlatformStatus struct {
    // aws contains platform-specific status for AWS
    // +optional
    AWS *AWSPlatformStatus `json:"aws,omitempty"`

    // azure contains platform-specific status for Azure, reflecting the identity
    // configuration currently applied by the control plane operator.
    // +optional
    Azure AzurePlatformStatus `json:"azure,omitzero,omitempty"`
}
```

#### New types

```go
// AzurePlatformStatus contains status specific to the Azure platform. It reflects
// the identity configuration that the control plane operator has applied to
// SecretProviderClass and ServiceAccount resources.
//
// Note: this reflects CPO-applied configuration, not pod-runtime state. Pods may
// continue to use old credentials for minutes after the CPO applies a new
// SecretProviderClass (CSI driver poll interval). Pod-level confirmation is a
// separate follow-on effort (CNTRLPLANE-4495).
//
// +kubebuilder:validation:MinProperties=1
type AzurePlatformStatus struct {
    // managedIdentities reflects the credential references currently applied
    // to control plane and data plane components. Populated when the Azure
    // authentication mode is ManagedIdentities.
    // +optional
    ManagedIdentities AzureManagedIdentitiesStatus `json:"managedIdentities,omitzero,omitempty"`

    // workloadIdentities reflects the client IDs of the federated workload identities
    // currently applied by the control plane operator. Populated when the Azure
    // authentication mode is WorkloadIdentities.
    // +optional
    WorkloadIdentities AzureWorkloadIdentitiesStatus `json:"workloadIdentities,omitzero,omitempty"`
}

// AzureManagedIdentitiesStatus reflects the active managed identity credential
// references for control plane and data plane components.
//
// +kubebuilder:validation:MinProperties=1
type AzureManagedIdentitiesStatus struct {
    // controlPlane contains the Key Vault credential secret names currently applied
    // to control plane components by the control plane operator.
    // +optional
    ControlPlane AzureControlPlaneManagedIdentitiesStatus `json:"controlPlane,omitzero,omitempty"`

    // dataPlane contains the MSI client IDs currently applied to data plane components
    // via the ignition configuration.
    // +optional
    DataPlane AzureDataPlaneManagedIdentitiesStatus `json:"dataPlane,omitzero,omitempty"`
}

// AzureControlPlaneManagedIdentitiesStatus reflects the active Key Vault credential
// secret name for each control plane component. Each field holds the
// credentialsSecretName that the CPO is currently using; it changes when an
// identity rotation is applied.
//
// +kubebuilder:validation:MinProperties=1
type AzureControlPlaneManagedIdentitiesStatus struct {
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    CloudProvider string `json:"cloudProvider,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    NodePoolManagement string `json:"nodePoolManagement,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    ControlPlaneOperator string `json:"controlPlaneOperator,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    ImageRegistry string `json:"imageRegistry,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    Ingress string `json:"ingress,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    Network string `json:"network,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    Disk string `json:"disk,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=127
    // +kubebuilder:validation:MinLength=1
    File string `json:"file,omitempty"`
}

// AzureDataPlaneManagedIdentitiesStatus reflects the active MSI client IDs for
// data plane managed identities. These values are passed into the ignition
// configuration applied to worker nodes.
//
// +kubebuilder:validation:MinProperties=1
type AzureDataPlaneManagedIdentitiesStatus struct {
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    ImageRegistryClientID string `json:"imageRegistryClientID,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    DiskClientID string `json:"diskClientID,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    FileClientID string `json:"fileClientID,omitempty"`
}

// AzureWorkloadIdentitiesStatus reflects the active client IDs for federated
// workload identities currently applied by the control plane operator.
//
// +kubebuilder:validation:MinProperties=1
type AzureWorkloadIdentitiesStatus struct {
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    CloudProvider string `json:"cloudProvider,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    NodePoolManagement string `json:"nodePoolManagement,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    ControlPlaneOperator string `json:"controlPlaneOperator,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    ImageRegistry string `json:"imageRegistry,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    Ingress string `json:"ingress,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    Network string `json:"network,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    Disk string `json:"disk,omitempty"`
    // +optional
    // +kubebuilder:validation:MaxLength=255
    // +kubebuilder:validation:MinLength=1
    File string `json:"file,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specific to HyperShift. The new status field is populated by the
CPO — which runs in the management cluster — based on what it has applied to
`SecretProviderClass` and `ServiceAccount` resources in the control plane namespace.
The field is propagated from `HostedControlPlane.Status.Platform` to
`HostedCluster.Status.Platform` by the HyperShift Operator's existing generic status
copy (`hcluster.Status.Platform = hcp.Status.Platform`); no additional HO code is
required.

The CPO uses `statuspatching.PatchStatus` with optimistic locking so that concurrent
status writers (CPO, HCCO, karpenter) do not silently overwrite each other's changes.

#### Standalone Clusters

Not applicable. This enhancement is exclusively for Azure-backed hosted control planes.

#### Single-node Deployments or MicroShift

Not applicable.

#### OpenShift Kubernetes Engine

Not applicable.

### Implementation Details/Notes/Constraints

**Reconciliation function:** A new `reconcileAzurePlatformStatus` method is added to
`HostedControlPlaneReconciler` in
`control-plane-operator/controllers/hostedcontrolplane/hostedcontrolplane_controller.go`.
It is called after `reconcileDefaultSecurityGroup` in the `update()` function, once per
reconcile loop. It performs a pure spec-mirror operation — no Azure API calls — and is
therefore safe to run on every reconcile and correct on conflict-retry.

**Idempotency:** Because the status mirrors spec directly, calling the function repeatedly
with the same spec produces the same status. `statuspatching.PatchStatus` skips the patch
if no change is detected.

**Auth mode switching:** The `AzureAuthenticationType` field is immutable once set, so
a cluster will always be either `ManagedIdentities` or `WorkloadIdentities`. Only one of
the two sub-fields will be populated; the other will be at its zero value and omitted from
serialization via `omitzero`.

**Generated artifacts:** Adding new API types requires regenerating:
- `api/hypershift/v1beta1/zz_generated.deepcopy.go`
- `client/applyconfiguration/hypershift/v1beta1/` (new apply-configuration builders)
- `docs/content/reference/api.md` and `docs/content/reference/aggregated-docs.md`
- CRD YAML manifests under `cmd/install/assets/crds/`

**No vendor drift:** The HyperShift main module vendors its own API module
(`replace github.com/openshift/hypershift/api => ./api`). The vendored copies of changed
files must be kept in sync with their sources.

### Risks and Mitigations

**Risk: Consumers treat status as pod-runtime confirmation.**
The status reflects CPO-applied `SecretProviderClass` configuration, not that pods are
running with the new certificate. Consumers must account for the CSI driver poll interval
(default: 2 minutes) and any pod restart lag.
*Mitigation:* The API comment and this enhancement document the semantic explicitly.
Pod-level confirmation (CNTRLPLANE-4495) provides the stronger guarantee.

**Risk: Status written before component reconciliation succeeds.**
The current call site places `reconcileAzurePlatformStatus` after `reconcileDefaultSecurityGroup`
but before the v2 component reconciliation loop. If a later component reconcile fails, the
status already reflects the new identity — a consumer may act on stale information.
*Mitigation:* The window is small (one reconcile loop) and the CPO will retry on the next
reconcile. For the initial release this is acceptable; a follow-on can gate the status
write on successful component reconciliation.

**Risk: Stale status during downgrade.**
If the operator is downgraded to a version that does not populate `status.platform.azure`,
the field will stop updating but retain its last value.
*Mitigation:* The field is purely informational and optional. Consumers must tolerate
stale status by cross-checking against spec or using a separate freshness signal.

### Drawbacks

- Adds new generated files and CRD schema complexity for a status-only feature.
- The "CPO-applied SPC" semantic is one level weaker than the "pods confirmed using new
  credential" semantic that consumers ultimately want. The stronger guarantee requires
  watching `SecretProviderClassPodStatus` objects (CNTRLPLANE-4495), which is more complex.

## Alternatives (Not Implemented)

### Report clientID instead of credentialsSecretName

`ManagedIdentity.clientID` exists in the spec but is optional and documented as
"mainly used for CI purposes." ARO HCP production clusters do not set this field today
(confirmed by the ARO team). Reporting it would result in empty status for all production
clusters. `credentialsSecretName` is always required and is the operative rotation signal
in the ARO HCP identity replacement protocol.

### ControlPlaneComponent CRD

Suggested in an [OCPSTRAT-2151 comment](https://issues.redhat.com/browse/OCPSTRAT-2151?focusedCommentId=18079965):
extend `ControlPlaneComponent.Status` with per-component identity information.
Rejected because `ControlPlaneComponent` is reconciled generically without per-component
identity knowledge. The CPO's component reconciliation framework has no access to which
`credentialsSecretName` was applied to a given component's `SecretProviderClass`.

### Single rollup status field

Instead of per-component granularity, expose a single boolean or timestamp indicating
"all identities applied." Rejected based on [OCPSTRAT-2151 feedback](https://issues.redhat.com/browse/OCPSTRAT-2151?focusedCommentId=18143688):
ARO HCP tracks individual managed identities by component and requires per-identity-role
granularity.

## Open Questions

1. **msi-dataplane auto-refresh:** The msi-dataplane library supports auto-refresh of
   credentials without pod restarts. This needs to be verified for each control plane
   component to confirm that identity rotation does not require a rolling restart of
   control plane pods after the CSI driver updates the mounted certificate.

2. **Call site placement:** Should `reconcileAzurePlatformStatus` be called after the
   v2 component reconciliation loop (so it only writes status when components have
   been successfully reconciled) rather than before it? This would tighten the semantic
   guarantee at the cost of slightly delayed status updates.

## Test Plan

- **Unit tests:** Table-driven tests for `reconcileAzurePlatformStatus` covering
  `ManagedIdentities` mode, `WorkloadIdentities` mode, non-Azure platform (no-op), and
  nil identity config (no panic).
- **E2e tests:** Verify that `status.platform.azure` is populated with the correct
  values after cluster creation on an Azure cluster. Must cover both `ManagedIdentities`
  (ARO HCP CI environment) and `WorkloadIdentities` (self-managed Azure CI environment).
  Tracked in CNTRLPLANE-4496.

## Graduation Criteria

### Dev Preview → Tech Preview

Not applicable — this feature does not require a feature gate. The status field is
additive and informational; consumers that do not read it are unaffected.

### Tech Preview → GA

The feature ships as GA from the first release. It is a purely additive, optional status
field with no behavioral impact on existing clusters.

## Upgrade / Downgrade Strategy

**Upgrade:** The new `status.platform.azure` field is populated by the first CPO version
that includes this enhancement. Prior to upgrade, the field is absent (zero value);
consumers that check it before upgrade will see an empty status, which they should treat
as "not yet available." No spec changes are required.

**Downgrade:** If the operator is downgraded, `status.platform.azure` stops being updated
but retains its last value (Kubernetes does not clear unknown status fields on downgrade).
Consumers should treat a stale status as equivalent to "not yet available" until the field
is refreshed.

No rollout or configuration changes are required on existing clusters for either direction.

## Version Skew Strategy

The CPO version is tied to the hosted cluster's OCP release image. The HyperShift Operator
and CPO may temporarily run different versions during an operator upgrade.

- **HO upgraded, CPO not yet upgraded:** The new `status.platform.azure` field exists in
  the CRD schema (deployed by HO) but the old CPO does not write it. Status is absent.
- **CPO upgraded, HO not yet upgraded:** The new CPO writes `status.platform.azure`; the
  old HO ignores unknown status fields and copies `hcp.Status.Platform` generically.

Both directions are safe. No action required during the skew window.

## Operational Aspects of API Extensions

The new types are purely status fields — they are only written by the CPO and read by
consumers. They add no admission webhooks, conversion webhooks, finalizers, or other
mechanisms that could affect API availability or cluster stability.

**Impact on SLIs:**
- No impact on API server throughput or latency.
- The CPO writes to `status.platform.azure` on every reconcile of the
  `HostedControlPlane`. Each write is a small JSON patch on an existing object with
  optimistic locking. Expected frequency: once per reconcile loop (typically every
  few minutes for a stable cluster).

**Failure modes:**
- If `statuspatching.PatchStatus` fails (e.g., conflict retry limit exceeded), the
  status update is skipped for that reconcile loop and retried on the next. The CPO
  continues reconciling other components normally.
- No cluster functionality depends on this status field. Failure to update it is
  operationally equivalent to the pre-enhancement state.

## Support Procedures

**Detecting a stale or missing status:**
- Check `kubectl get hcp <name> -n <ns> -o jsonpath='{.status.platform.azure}'`.
- If empty, the CPO has not applied the identity configuration yet, or the cluster
  is running an older CPO version.
- Compare `status.platform.azure.managedIdentities.controlPlane` against
  `spec.platform.azure.azureAuthenticationConfig.managedIdentities.controlPlane` to
  identify which components have not been updated.

**Disabling:**
- The feature cannot be disabled independently. Because it is a status-only field with
  no behavioral impact, disabling is not necessary.
- Removing the `Azure` field from `PlatformStatus` would be a breaking API change and
  is not supported.

## Infrastructure Needed

No new infrastructure is required. All changes are within the existing HyperShift
repository and the openshift/enhancements repository.
