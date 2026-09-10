---
title: storageclass-kms-key
authors:
  - "@devguyio"
reviewers:
  - "@csrwng, for HyperShift architecture and API design"
  - "@muraee, for HyperShift AWS platform and propagation chain"
  - "@celebdor, for HyperShift platform and storage integration"
  - "@jsafrane, for storage operator and ClusterCSIDriver"
  - "@joshbranham, for managed services and ROSA integration"
  - "@JoelSpeed, for API conventions and review"
  - "@everettraven, for API conventions and review"
approvers:
  - "@csrwng"
  - "@enxebre"
api-approvers:
  - "@enxebre"
  - "@JoelSpeed"
  - "@everettraven"
creation-date: 2026-06-09
last-updated: 2026-07-13
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1679
see-also:
  - "/enhancements/storage/aws-ebs-csi-driver-sts.md"
replaces: []
superseded-by: []
---

# StorageClass KMS Key for AWS Hosted Control Planes

## Summary

This enhancement adds an optional `kmsKeyARN` field under
`spec.operatorConfiguration.csiDriverConfig.aws` on the HyperShift `HostedCluster`
API. When set at cluster creation time, the Hosted Cluster Config Operator (HCCO)
propagates the key ARN to the `ClusterCSIDriver` resource in the guest cluster,
causing the cluster-storage-operator to configure the default StorageClass to encrypt
new EBS volumes with the customer-specified AWS KMS key. This closes a parity gap
between ROSA classic and ROSA Hosted Control Planes (HCP) for storage encryption.

The field is a **day-1 knob**: it configures the initial default StorageClass
encryption at cluster creation. Day-2 changes to storage encryption are made
directly on the `ClusterCSIDriver` resource in the guest cluster by the cluster
administrator, following the same pattern as the default ingress controller
configuration.

## Motivation

ROSA classic clusters allow customers to configure KMS encryption for volumes created
by the default StorageClass via `rosa create cluster --kms-key-arn`. ROSA HCP clusters
does not expose this capability, even though the underlying CSI operator
already supports it via `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN`
(established in the enhancement [`Default Storage Class with Encrypted Keys`](../installer/storage-class-encrypted.md)). HyperShift simply does not expose a creation-time knob in the `HostedCluster` API or
propagate it to the guest cluster.

Self-managed HyperShift on AWS has the same gap: operators cannot configure default
StorageClass encryption at cluster creation time.

### User Stories

- As a ROSA HCP cluster administrator, I want to specify a KMS key ARN when creating
  a cluster so that all PVCs provisioned by the default StorageClass are encrypted with
  my organization's key instead of the default AWS-managed key.

- As a ROSA HCP cluster administrator, I want to update the KMS encryption
  configuration on a running cluster by editing the `ClusterCSIDriver` resource
  directly in the guest cluster, so that I can change keys or modify encryption
  settings without recreating the cluster.

- As a self-managed HyperShift operator or HyperShift developer on AWS, I want to
  specify a KMS key for the default StorageClass at cluster creation time via the
  `hcp` or `hypershift` CLI so that I can enforce encryption standards from day 1.

### Goals

- Allow HyperShift users to specify a KMS key for encrypting the default
  StorageClass EBS volumes at cluster creation time.
- Propagate the configured key from the HostedCluster API to the guest cluster's
  CSI driver configuration (write-once on initial resource creation, not continuously reconciled).
- Expose a CLI flag for specifying the KMS key in the `hcp create cluster aws`
  and `hypershift create cluster aws` commands.
- Apply identically to ROSA HCP and self-managed HyperShift on AWS.
- Preserve backward compatibility: clusters without the field continue to use
  AWS-managed encryption with no behavioral change.
- Preserve existing in-cluster storage configuration: cluster administrators can
  continue to modify guest cluster storage settings directly for day-2
  configuration changes without interference from the management cluster.

### Non-Goals

- **ROSA CLI (`rosa create cluster`), Terraform, CAPI, and Hybrid Cloud Console
  integration** are out of scope for this enhancement. These are downstream concerns
  tracked separately by the ROSA product team, which consumes the upstream
  `HostedCluster` API to wire it into their clients.
- **Per-StorageClass KMS key granularity.** This targets only the default
  StorageClass. Users can still create custom StorageClasses with their own keys.
  Per-StorageClass granularity would require CSI operator changes, out of scope.
- **Day-2 key management via the HostedCluster API.** Day-2 key reconfiguration and removal
  are performed directly on `ClusterCSIDriver` in the guest cluster. The HostedCluster field
  captures day-1 intent only.
- **Re-encrypting existing PVs.** The KMS key applies to newly created PVCs only.
  Existing volumes retain their original encryption.
- **NodePool root volume encryption.** Root volume encryption is configured separately
  on `NodePool.spec.platform.aws.rootVolume.encryptionKey` and is not affected by this
  enhancement.

## Proposal

This enhancement adds an optional `kmsKeyARN` field to the HostedCluster API under
`spec.operatorConfiguration.csiDriverConfig.aws`, allowing customers to specify a KMS
key ARN for encrypting the default StorageClass EBS volumes.

When a customer sets this field on a `HostedCluster` at creation time, the following
propagation chain executes:

1. The HostedCluster controller mirrors the `operatorConfiguration` (including the new `aws`
   subfield) to `HostedControlPlane`.
2. The HCCO storage reconciliation uses `CreateOrUpdate` on the `ClusterCSIDriver`
   resource in the guest cluster. On the **Create path** (initial resource creation),
   the HCCO writes `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN`. On the
   **Update path** (resource already exists), the HCCO skips `DriverConfig` entirely,
   preserving any in-cluster modifications made by the administrator.
3. The CSO reads the `ClusterCSIDriver` value and configures the default StorageClass
   with `parameters.kmsKeyId`. New EBS volumes provisioned via the StorageClass carry
   the KMS encryption.

When `kmsKeyARN` is set, the CPOv2 component framework blocks the CSO from starting
until the HCCO has completed its first successful reconcile. This guarantees the HCCO
creates `ClusterCSIDriver` with the KMS key before the CSO or its operands read it.
See "Bootstrap Ordering" under Implementation Details.

The cluster-storage-operator already reads
`ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` and configures the default
StorageClass accordingly as established in the enhancement [`Default Storage Class with Encrypted Keys`](../installer/storage-class-encrypted.md).
No changes to the cluster-storage-operator codebase are needed. The CSO's startup
ordering is configured in the HyperShift CPOv2 component registration.

### Workflow Description

#### Actors

- **Cluster administrator:** A human operator who creates or manages `HostedCluster`
  resources (ROSA HCP customer or self-managed HyperShift operator).
- **HostedCluster controller:** The HyperShift HostedCluster controller, part of the HyperShift
  operator running in the `hypershift` namespace on the management cluster.
- **CPO:** The Control Plane Operator, a per-HostedControlPlane deployment running in the HostedControlPlane
  namespace on the management cluster. Manages ~40 control plane components via the
  CPOv2 component framework, including deployment creation, dependency ordering, and
  status reporting.
- **HCCO:** The Hosted Cluster Config Operator, a control plane component managed by
  the CPO that reconciles resources on the guest cluster. Versioned in lockstep with
  the guest OCP version, running in the HostedControlPlane namespace on the management cluster.
- **CSO:** The cluster-storage-operator (from the guest release payload), running in
  the HostedControlPlane namespace on the management cluster.

#### Day-1: Cluster Creation with KMS Key

1. The cluster administrator runs:
   ```bash
   hcp create cluster aws \
     --storage-volumes-kms-key arn:aws:kms:us-east-1:123456789012:key/mrk-abc123 \
     ...
   ```
   or sets `spec.operatorConfiguration.csiDriverConfig.aws.kmsKeyARN` directly on the
   `HostedCluster` manifest.
2. The HostedCluster controller creates the `HostedControlPlane` with `operatorConfiguration`
   mirrored.
3. The CPO reconciles the `HostedControlPlane`, sees that `kmsKeyARN` is set,
   deploys the HCCO, and registers a conditional dependency so that the CSO
   will not get deployed until `ConfigOperatorReconciliationSucceeded` is True
   on the `HostedControlPlane`.
4. The HCCO creates `ClusterCSIDriver` with `spec.driverConfig.aws.kmsKeyARN` set.
   On subsequent reconciles, the resource already exists, so the HCCO skips
   `DriverConfig`.
5. The CPO sees the condition is satisfied and deploys the CSO. The CSO configures
   the default StorageClass with `parameters.kmsKeyId` set to the ARN.
6. New PVCs created from the default StorageClass produce EBS volumes encrypted with
   the customer's key.

If an invalid KMS key is configured, errors surface naturally during PVC
provisioning through the CSI driver. This is the same behavior as standalone OCP.

#### Day-2: Key Reconfiguration (In-Cluster)

Day-2 key reconfiguration is performed by the cluster administrator directly on the
`ClusterCSIDriver` resource in the guest cluster:

```bash
oc patch clustercsidriver ebs.csi.aws.com --type merge \
  -p '{"spec":{"driverConfig":{"driverType":"AWS","aws":{"kmsKeyARN":"arn:aws:kms:us-east-1:123456789012:key/new-key"}}}}'
```

The CSO picks up the change and updates the default StorageClass. New PVCs use the
new key. Existing PVs retain encryption with the original key. The HCCO does not
touch `ClusterCSIDriver.DriverConfig` after the initial creation.

#### Day-2: Disabling KMS Encryption (In-Cluster)

The cluster administrator clears `DriverConfig` on the `ClusterCSIDriver` directly in
the guest cluster. The CSO reverts the default StorageClass to AWS-managed encryption.
The HCCO does not re-populate the field because the resource already exists.

### API Extensions

#### New field: `kmsKeyARN` on `AWSCSIDriverConfig`

A new field is added under `spec.operatorConfiguration.csiDriverConfig.aws` on both
`HostedCluster` and `HostedControlPlane`. This follows the ingress operator pattern
where platform-specific configuration is nested inside the operator's own config
(`ingressOperator.endpointPublishingStrategy.loadBalancer.providerParameters.aws`).
The `CSIDriverOperatorConfig` struct naturally extends to `azure`, `gcp` in the future.

New types:

```go
// CSIDriverOperatorConfig specifies configuration for the CSI driver operator
// in the hosted cluster.
// Once the aws field is set, it cannot be removed.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.aws) || has(self.aws)",message="aws is immutable once set and cannot be removed"
type CSIDriverOperatorConfig struct {
	// aws specifies configuration for the AWS EBS CSI driver operator.
	// +optional
	AWS AWSCSIDriverConfig `json:"aws,omitzero,omitempty"`
}

// AWSCSIDriverConfig specifies configuration for the AWS EBS CSI driver.
// Once kmsKeyARN is set, it cannot be removed from this struct.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.kmsKeyARN) || has(self.kmsKeyARN)",message="kmsKeyARN cannot be removed once set"
type AWSCSIDriverConfig struct {
	// kmsKeyARN is the ARN of an AWS KMS key used to encrypt volumes
	// created by the default StorageClass. When set, new PersistentVolumes
	// provisioned by the default StorageClass are encrypted with this key
	// instead of the AWS account's default EBS encryption key.
	//
	// When omitted, no KMS encryption is configured on the default StorageClass.
	// EBS volumes use the AWS account's default encryption settings.
	//
	// The value may be either the ARN or Alias ARN of a KMS key in the format:
	//   arn:<partition>:kms:<region>:<account-id>:(key|alias)/<resource-id>
	//
	// When set, must be between 1 and 2048 characters.
	//
	// This field is applied at cluster creation time only and is immutable
	// once set. Day-2 changes to storage encryption should be made directly
	// on the ClusterCSIDriver resource in the guest cluster.
	//
	// The StorageARN role in AWSRolesRef must have kms:Decrypt,
	// kms:GenerateDataKeyWithoutPlaintext, and kms:CreateGrant
	// permissions on the specified key.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:XValidation:rule="matches(self, '^arn:(aws|aws-cn|aws-us-gov|aws-iso|aws-iso-b|aws-iso-e|aws-iso-f):kms:[a-z0-9-]+:[0-9]{12}:(key|alias)/.+$')",message="kmsKeyARN must be a valid AWS KMS key ARN in the format: arn:<partition>:kms:<region>:<account-id>:(key|alias)/<key-id-or-alias>"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="kmsKeyARN is immutable"
	KMSKeyARN string `json:"kmsKeyARN,omitempty"`
}
```

Added to `OperatorConfiguration`:

```go
// Once the csiDriverConfig field is set, it cannot be removed.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.csiDriverConfig) || has(self.csiDriverConfig)",message="csiDriverConfig is immutable once set and cannot be removed"
type OperatorConfiguration struct {
	// ... existing fields (clusterVersionOperator, clusterNetworkOperator, ingressOperator)

	// csiDriverConfig specifies configuration for the CSI driver operator in the hosted cluster.
	// Once set, it cannot be removed.
	// +optional
	CSIDriverConfig CSIDriverOperatorConfig `json:"csiDriverConfig,omitzero,omitempty"`
}
```

Added to `HostedClusterSpec` (day-1-only guards):

```go
// +kubebuilder:validation:XValidation:rule="!has(self.operatorConfiguration) || !has(self.operatorConfiguration.csiDriverConfig) || !has(self.operatorConfiguration.csiDriverConfig.aws) || !has(self.operatorConfiguration.csiDriverConfig.aws.kmsKeyARN) || (has(oldSelf.operatorConfiguration) && has(oldSelf.operatorConfiguration.csiDriverConfig) && has(oldSelf.operatorConfiguration.csiDriverConfig.aws) && has(oldSelf.operatorConfiguration.csiDriverConfig.aws.kmsKeyARN))",message="kmsKeyARN cannot be added after creation"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.operatorConfiguration) || !has(oldSelf.operatorConfiguration.csiDriverConfig) || !has(oldSelf.operatorConfiguration.csiDriverConfig.aws) || !has(oldSelf.operatorConfiguration.csiDriverConfig.aws.kmsKeyARN) || has(self.operatorConfiguration)",message="operatorConfiguration cannot be removed when kmsKeyARN is set"
type HostedClusterSpec struct {
	// ...
```

The CEL validation regex aligns with the downstream
`ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` field in
[openshift/api](https://github.com/openshift/api/blob/master/operator/v1/types_csi_cluster_driver.go),
including the full set of AWS partitions.

The immutability rules follow the `HCPEtcdBackupS3.kmsKeyARN` and
`infraID`/`clusterID` precedents: `kmsKeyARN` is immutable once set, cannot be
added after creation, and parent structs cannot be removed to circumvent the
child's immutability. Spec-level guards cover the case where the entire parent
chain is added or removed in one update (nested transition rules are skipped
when the parent struct is absent from either the old or new object).

#### KMS Error Visibility

If an invalid KMS key is configured, errors surface naturally during PVC
provisioning through the CSI driver. This is the same behavior as standalone OCP.
Proactive KMS key validation is planned as a separate feature (OCPSTRAT-3501).

#### CLI flag: `--storage-volumes-kms-key`

A new flag is added to the AWS cluster creation command, consistent with the existing
`--root-volume-kms-key` naming convention:

```text
--storage-volumes-kms-key string
    AWS KMS key ARN (arn:...:key/...) or alias ARN (arn:...:alias/...) used
    to encrypt PVCs created by the default StorageClass at cluster creation.
    If omitted, PVCs use AWS-managed encryption. The StorageARN role must have
    kms:Decrypt, kms:GenerateDataKeyWithoutPlaintext, and kms:CreateGrant
    permissions on the specified key. Day-2 key reconfiguration should be done directly
    on the ClusterCSIDriver resource in the guest cluster.
```

The flag is bound through the shared options mechanism so it is exposed in both the
HCP CLI (`hcp create cluster aws`) and the developer CLI
(`hypershift create cluster aws`) automatically.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement adds day-1 KMS encryption support for HyperShift hosted clusters.
The KMS key is set at cluster creation time and propagated to the guest cluster's
default StorageClass. Day-2 key reconfiguration is performed directly on the
`ClusterCSIDriver` resource in the guest cluster, not through the HostedCluster API.

No new IAM roles or permissions are introduced; the existing StorageARN role
(`ebs-cloud-credentials`) requires `kms:Decrypt`,
`kms:GenerateDataKeyWithoutPlaintext`, and `kms:CreateGrant` on the specified key.

#### Standalone Clusters

Not applicable. Standalone OCP clusters configure `ClusterCSIDriver` directly.

#### Single-node Deployments or MicroShift

Not applicable.

#### OpenShift Kubernetes Engine (OKE)

Not applicable. Storage KMS configuration in standard OpenShift clusters is handled
via `ClusterCSIDriver` directly.

### Implementation Details/Notes/Constraints

#### IAM Permissions

The StorageARN role requires the following KMS permissions:
- `kms:Decrypt` (for EBS volume attachment)
- `kms:GenerateDataKeyWithoutPlaintext` (for EBS volume creation)
- `kms:CreateGrant` (for EBS service principal access)

ROSA HCP clusters require these permissions in the
`ROSAAmazonEBSCSIDriverOperatorPolicy` AWS managed policy. Self-managed
HyperShift clusters require equivalent permissions on the StorageARN role.

#### Write-Once Reconciliation

The HCCO writes `ClusterCSIDriver.DriverConfig` only on the initial resource
creation. The HCCO uses `CreateOrUpdate` on the `ClusterCSIDriver` resource. Inside
the mutate function, it checks `driver.CreationTimestamp.IsZero()`:

- **Create path** (`CreationTimestamp` is zero): the resource does not yet exist.
  The HCCO writes `DriverConfig.AWS.KMSKeyARN` if configured. If `kmsKeyARN` is
  empty, `DriverConfig` is left unset.
- **Update path** (`CreationTimestamp` is non-zero): the resource already exists.
  The HCCO skips `DriverConfig` entirely, preserving any in-cluster modifications
  made by the administrator.

The write-once guard guarantees:
- Initial creation with `kmsKeyARN`: HCCO writes the field.
- Initial creation without `kmsKeyARN`: HCCO leaves `DriverConfig` unset.
- Day-2 admin key reconfiguration on `ClusterCSIDriver`: HCCO does not revert it.
- Day-2 admin clears `DriverConfig`: HCCO does not re-populate it.
- HCCO crash before `CreateOrUpdate` completes: on restart, the resource either
  exists (update path, skip) or doesn't (create path, retry). Both are correct.
- Upgrade of a cluster that never had `kmsKeyARN`: the resource already exists,
  HCCO hits the update path, `DriverConfig` is untouched.

Unlike an annotation-based guard, `CreationTimestamp` is a server-set field that
cannot be modified or deleted by users. The CSO conditional dependency (below)
guarantees the HCCO creates the resource first when `kmsKeyARN` is set.

#### Bootstrap Ordering: CSO Conditional Dependency on HCCO

When `kmsKeyARN` is set, the HCCO must write the KMS key to `ClusterCSIDriver`
before the CSO creates the default StorageClass. The CPO uses the CPOv2 component
dependency framework to block the CSO from starting until the HCCO has completed
its first successful reconcile. This works as follows:

1. The HCCO component (`hosted-cluster-config-operator`) registers a custom operands
   rollout check that reads the existing `ConfigOperatorReconciliationSucceeded`
   condition on the `HostedControlPlane`. This condition is set to True by the HCCO
   after each successful reconcile (including `reconcileStorage`). The HCCO's
   `ControlPlaneComponent` CR reports `RolloutComplete=False` until this condition
   is True.
2. The CSO component (`cluster-storage-operator`) conditionally declares the HCCO
   as a dependency when `kmsKeyARN` is set in the HostedControlPlane spec. The CPOv2 framework
   blocks the CSO's entire reconciliation (deployment creation, manifest
   application) until the HCCO's `ControlPlaneComponent` CR reports both
   `Available=True` and `RolloutComplete=True`.

The resulting bootstrap sequence when `kmsKeyARN` is set:

1. CVO installs the `ClusterCSIDriver` CRD in the guest cluster.
2. HCCO availability prober completes (all 18 CRDs present).
3. HCCO reconciles storage, creates `ClusterCSIDriver` with the KMS key,
   sets `ConfigOperatorReconciliationSucceeded=True` on the HostedControlPlane.
4. The CSO component's dependency check passes (HCCO's `ControlPlaneComponent` CR
   reports `RolloutComplete=True`), and the CSO reconciliation proceeds.
5. CSO starts, finds `ClusterCSIDriver` already exists with the KMS key (the CSO's
   `applyClusterCSIDriver` returns the existing object untouched).
6. CSO deploys the `aws-ebs-csi-driver-operator`.
7. The operator reads `ClusterCSIDriver` with the KMS key already present and creates
   the StorageClass correctly from the start.

When `kmsKeyARN` is not set, the CSO has no dependency on the HCCO and starts
independently. No behavior change for clusters without KMS encryption.

The conditional dependency adds approximately 30-60 seconds to the CSO's startup time
(the time for the HCCO to complete its first reconcile). This is acceptable: the
tradeoff is correctness over a small bootstrap delay for a single operator.

### Risks and Mitigations

#### Invalid or Inaccessible KMS Key

If the KMS key ARN is syntactically valid but the key is disabled, deleted, or the
IAM role lacks the required permissions, PVC provisioning fails at the CSI driver
level with the AWS error surfaced in PVC events. This is the same behavior as
standalone OCP.
*Mitigation:* Document IAM permission requirements and key lifecycle responsibility.
Proactive key validation is planned as a separate feature (OCPSTRAT-3501).

### Drawbacks

- Introduces a new operator entry in `OperatorConfiguration` (`csiDriverConfig`).
  Platform branching is inside the operator config, following the ingress operator
  pattern.

## Alternatives (Not Implemented)

#### Field on AWSPlatformSpec

Placing `storageKMSKeyARN` directly on the existing `AWSPlatformSpec` struct was the
initial design. Rejected because `AWSPlatformSpec` holds platform infrastructure
configuration (region, VPC, IAM roles), while this field configures an operator
(the CSI driver). The `operatorConfiguration` struct is where operator configs belong.
OCP APIs are hard to modify after creation; getting the nesting right before GA avoids
a future deprecation cycle.

#### Continuous Reconciliation of ClusterCSIDriver

Continuously reconciling `ClusterCSIDriver.DriverConfig` from the HostedCluster spec (like
OAuth configuration) was considered. Rejected because `ClusterCSIDriver` is already
editable by cluster administrators in the guest cluster, and breaking that UX would
be disruptive. The ingress controller uses the same day-1-setup / day-2-admin-control
model.

#### Annotation-Based Set-Once Guard

Using an annotation on `ClusterCSIDriver` to track whether the initial write occurred
was considered. Rejected because annotations are user-facing metadata that can be
deleted, which would cause the HCCO to re-write `DriverConfig` on the next reconcile,
overwriting admin changes. The `CreationTimestamp`-based approach uses a server-set
field that cannot be tampered with.

## Open Questions

None at this time.

## Test Plan

### Envtest (CEL Validation)

YAML-driven envtest cases are mandatory for all CEL validation rules per HyperShift
project convention. Test cases cover `onCreate` and `onUpdate` scenarios across
multiple Kubernetes API server versions (1.31-1.35):

onCreate:
- Valid KMS key ARN accepted
- Valid alias ARN accepted
- Valid `aws-cn` partition ARN accepted
- Invalid prefix rejected
- Wrong partition rejected
- Wrong service rejected
- Empty key ID rejected
- Cluster without `csiDriverConfig` accepted

onUpdate:
- Changing `kmsKeyARN` value is rejected
- Removing `kmsKeyARN` is rejected
- Removing `aws` while `kmsKeyARN` was set is rejected
- Removing `csiDriverConfig` while it was set is rejected
- Adding `kmsKeyARN` to a cluster created without it is rejected
- Removing `operatorConfiguration` while `kmsKeyARN` was set is rejected

### E2E Tests

E2E tests will be added to the HyperShift E2E test suite, which runs against a
pre-existing hosted cluster on live AWS infrastructure. The test
reuses the existing CI KMS key (`alias/hypershift-ci`) already provisioned in the
CI AWS account.

Tests run in the `e2e-aws` presubmit and `e2e-aws-ovn` periodic CI jobs, which use
the `hypershift` cluster profile with Boskos-managed AWS account leasing.

Test scenarios:

1. **Day-1 key configuration:**
   - Create a `HostedCluster` with `operatorConfiguration.csiDriverConfig.aws.kmsKeyARN`
     set
   - Verify `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` is set in the hosted
     cluster
   - Create a PVC using the default StorageClass, wait for it to bind
   - Verify the resulting EBS volume is encrypted with the specified KMS key via
     `ec2.DescribeVolumes` (following the existing `KMSRootVolumeTest` pattern)

2. **Write-once semantics:**
   - Verify that modifying `ClusterCSIDriver.spec.driverConfig` directly in the guest
     cluster persists across HCCO reconcile cycles (the HCCO does not revert it)

3. **Regression (no key configured):**
   - On a hosted cluster where `kmsKeyARN` was never set, verify PVCs use
     AWS-managed encryption

## Graduation Criteria

### Dev Preview -> Tech Preview

This feature ships directly to GA. No Dev Preview or Tech Preview phase.

### Tech Preview -> GA

See above.

### GA

- Envtest coverage for CEL validation across multiple Kubernetes versions.
- E2E test coverage for the full lifecycle (creation, write-once verification).
- E2E tests passing in CI for at least one release cycle without flakes.
- User documentation merged in `openshift-docs` covering the
  `--storage-volumes-kms-key` flag, day-1 encryption setup, and day-2 in-cluster
  key management via `ClusterCSIDriver`.
- No open blocking bugs.

### Removing a deprecated feature

Not applicable. This enhancement adds new capability; nothing is deprecated.

## Upgrade / Downgrade Strategy

#### Upgrade

The new field is optional with `omitempty`. Existing `HostedCluster`
objects gain the field on upgrade, defaulting to empty. No action is required from
customers. On upgrade, the `ClusterCSIDriver` resource already exists, so the HCCO
hits the update path and leaves `DriverConfig` untouched.

#### Failed Upgrade Rollback

Control plane downgrades are not supported in
HyperShift. If an N->N+1 upgrade fails mid-way, the `kmsKeyARN` field is not yet
active and has no effect on storage behavior. If `kmsKeyARN` was already configured
on a successfully upgraded cluster, the `ClusterCSIDriver` in the hosted cluster
retains its last-written `DriverConfig` throughout any subsequent upgrade attempts.

## Version Skew Strategy

If `kmsKeyARN` is set on a HostedCluster running a guest release that predates this
feature, the field is accepted by the CRD but has no effect: the older HCCO does not
have the propagation code.

## Operational Aspects of API Extensions

#### SLIs

The primary indicator is successful PVC provisioning with the configured KMS key.
If the key is invalid, PVC provisioning failures surface the issue.

#### Impact on Existing SLIs

No additional AWS API calls are introduced by this feature. The HCCO writes the
field once during the initial `ClusterCSIDriver` creation.

#### Failure Modes

- If the configured KMS key is invalid or the IAM role lacks permissions, PVC
  provisioning fails at the CSI driver level with the AWS error. The
  `ClusterCSIDriver` field is still written by the HCCO.
- No impact on existing workloads or control plane availability.

## Support Procedures

#### Detecting Failures

- If PVCs from the default StorageClass fail to provision, check the PVC events
  for AWS error codes related to KMS key access.
- Verify `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` was written correctly
  by inspecting the guest cluster resource.

#### Diagnosing IAM Permission Errors

1. Verify the `StorageARN` role in `HostedCluster.spec.platform.aws.rolesRef.storageARN`.
2. Confirm the role's IAM policy includes `kms:Decrypt`,
   `kms:GenerateDataKeyWithoutPlaintext`, and `kms:CreateGrant`
   for the key ARN.
3. Confirm the key policy allows the `StorageARN` role principal.

#### Diagnosing KMS Key Errors

1. Confirm the KMS key is enabled in the AWS console / CLI.
2. Confirm the key exists in the correct AWS region (matches the cluster region).
3. For alias ARNs: confirm the alias points to an enabled key.

#### Day-2 Storage Encryption Changes

Day-2 key reconfiguration, key removal, or other `ClusterCSIDriver` changes are made
directly in the guest cluster by the cluster administrator. The HCCO does not
touch `ClusterCSIDriver.DriverConfig` after the initial creation.

#### Graceful Failure

If the configured KMS key is invalid, control plane provisioning continues
normally. PVC provisioning may fail at the CSI driver level if the key is invalid
or permissions are insufficient. This is the same behavior as standalone OCP.

## Infrastructure Needed

None. Changes are in `openshift/hypershift` only. E2E tests use existing CI jobs
and the `alias/hypershift-ci` KMS key already provisioned in the CI account.
