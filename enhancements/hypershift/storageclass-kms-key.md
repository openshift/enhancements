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
last-updated: 2026-10-15
status: implemented
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1679
see-also:
  - "/enhancements/storage/aws-ebs-csi-driver-sts.md"
replaces: []
superseded-by: []
---

# StorageClass KMS Key for AWS Hosted Control Planes

## Summary

This enhancement adds an optional `initialKMSKeyARN` field under
`spec.operatorConfiguration.csiDriverOperator.aws` on the HyperShift `HostedCluster`
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

This enhancement adds an optional `initialKMSKeyARN` field to the HostedCluster API under
`spec.operatorConfiguration.csiDriverOperator.aws`, allowing customers to specify a KMS
key ARN for encrypting the default StorageClass EBS volumes.

When a customer sets this field on a `HostedCluster` at creation time, the following
propagation chain executes:

1. The HostedCluster controller mirrors the `operatorConfiguration` (including the new `aws`
   subfield) to `HostedControlPlane`.
2. The HCCO creates the `ClusterCSIDriver` resource in the guest cluster with
   `spec.driverConfig.aws.kmsKeyARN` set. If the resource already exists (e.g.,
   on subsequent reconciles or after upgrade), the HCCO does not modify
   `DriverConfig`, preserving any in-cluster changes made by the administrator.
3. The CSO reads the `ClusterCSIDriver` value and configures the default StorageClass
   with `parameters.kmsKeyId`. New EBS volumes provisioned via the StorageClass carry
   the KMS encryption.

The CPOv2 component framework blocks the CSO from starting until the HCCO has
completed its first successful reconcile. This ordering is applied on the initial
rollout for all clusters (not only those with a KMS key), so that when
`initialKMSKeyARN` is set the HCCO creates `ClusterCSIDriver` with the key before
the CSO or its operands read it. See "Bootstrap Ordering" under Implementation Details.

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
     --initial-storage-volumes-kms-key arn:aws:kms:us-east-1:123456789012:key/mrk-abc123 \
     ...
   ```
   or sets `spec.operatorConfiguration.csiDriverOperator.aws.initialKMSKeyARN` directly on the
   `HostedCluster` manifest.
2. The HostedCluster controller creates the `HostedControlPlane` with `operatorConfiguration`
   mirrored.
3. The CPO reconciles the `HostedControlPlane`, deploys the HCCO, and gates the CSO
   on `ConfigOperatorReconciliationSucceeded` becoming True on the
   `HostedControlPlane` during the initial rollout, so the CSO is not deployed until
   the HCCO has reconciled.
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

#### New field: `initialKMSKeyARN` on `AWSCSIDriverConfig`

A new field is added under `spec.operatorConfiguration.csiDriverOperator.aws` on the
`HostedCluster`. This follows the ingress operator pattern
where platform-specific configuration is nested inside the operator's own config
(`ingressOperator.endpointPublishingStrategy.loadBalancer.providerParameters.aws`).
The `CSIDriverOperatorSpec` struct naturally extends to `azure`, `gcp` in the future.

`OperatorConfiguration` is a shared type referenced by both `HostedClusterSpec` and
`HostedControlPlaneSpec`, so the field is mirrored to `HostedControlPlane`
automatically. The spec-level CEL guards below live only on `HostedClusterSpec`
because that is the user-facing, user-editable surface; `HostedControlPlane` is
managed by the HostedCluster controller and is not edited directly.

New types:

```go
// CSIDriverOperatorSpec specifies configuration for the CSI driver operator
// running in the control plane. Platform-specific configuration is nested per platform.
// +kubebuilder:validation:MinProperties=1
type CSIDriverOperatorSpec struct {
	// aws configures the AWS EBS CSI driver.
	// It can only be set when spec.platform.type is AWS.
	//
	// When omitted, no AWS-specific CSI driver configuration is applied and the
	// default StorageClass uses the AWS account's default EBS encryption settings.
	// +optional
	AWS AWSCSIDriverConfig `json:"aws,omitzero"`
}

// AWSCSIDriverConfig specifies configuration for the AWS EBS CSI driver.
// +kubebuilder:validation:MinProperties=1
type AWSCSIDriverConfig struct {
	// initialKMSKeyARN is the ARN of an AWS KMS key used to encrypt volumes
	// created by the default StorageClass. When set, new PersistentVolumes
	// provisioned by the default StorageClass are encrypted with this key
	// instead of the AWS account's default EBS encryption key.
	//
	// When omitted, no KMS key is configured on the default StorageClass and
	// EBS volumes are encrypted with the AWS account's default EBS encryption
	// key.
	//
	// The value may be either the ARN or Alias ARN of a KMS key and must follow
	// the format arn:<partition>:kms:<region>:<account-id>:(key|alias)/<key-id-or-alias>,
	// where <partition> is one of aws, aws-cn, aws-us-gov, aws-iso, aws-iso-b,
	// aws-iso-e, or aws-iso-f; <region> is the AWS region; <account-id> is the
	// 12-digit AWS account identifier; and <key-id-or-alias> is the KMS key ID
	// or alias name. The key must be in the same region as the cluster
	// (spec.platform.aws.region).
	//
	// When set, must be between 1 and 2048 characters.
	//
	// This field can only be set when the HostedCluster is created and is
	// immutable afterwards. Day-2 changes to storage encryption must be made
	// directly on the ClusterCSIDriver resource in the hosted cluster.
	//
	// The IAM role in spec.platform.aws.rolesRef.storageARN must have
	// kms:Decrypt, kms:GenerateDataKeyWithoutPlaintext, and kms:CreateGrant
	// permissions on the specified key.
	//
	// +optional
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:XValidation:rule="self.matches('^arn:(aws|aws-cn|aws-us-gov|aws-iso|aws-iso-b|aws-iso-e|aws-iso-f):kms:[a-z0-9-]+:[0-9]{12}:(key|alias)/.+$')",message="initialKMSKeyARN must be a valid AWS KMS key ARN in the format: arn:<partition>:kms:<region>:<account-id>:(key|alias)/<key-id-or-alias>"
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="initialKMSKeyARN is immutable"
	InitialKMSKeyARN string `json:"initialKMSKeyARN,omitempty"`
}
```

Added to `OperatorConfiguration`:

```go
type OperatorConfiguration struct {
	// ... existing fields (clusterVersionOperator, clusterNetworkOperator, ingressOperator, ...)

	// csiDriverOperator configures the CSI drivers of the hosted cluster.
	// Settings are grouped by platform.
	//
	// When omitted, the CSI drivers use their default configuration.
	//
	// +optional
	CSIDriverOperator CSIDriverOperatorSpec `json:"csiDriverOperator,omitzero"`
}
```

Added to `HostedClusterSpec` (spec-level guards):

```go
// initialKMSKeyARN can only be set at creation and cannot be added or removed afterwards.
// +kubebuilder:validation:XValidation:rule="self.?operatorConfiguration.?csiDriverOperator.?aws.?initialKMSKeyARN.hasValue() == oldSelf.?operatorConfiguration.?csiDriverOperator.?aws.?initialKMSKeyARN.hasValue()",message="initialKMSKeyARN can only be set when the HostedCluster is created and cannot be added or removed afterwards"
// csiDriverOperator.aws may only be set on the AWS platform.
// +kubebuilder:validation:XValidation:rule="!self.?operatorConfiguration.?csiDriverOperator.?aws.hasValue() || self.platform.type == 'AWS'",message="csiDriverOperator.aws can only be set when platform.type is AWS"
// initialKMSKeyARN must reference a key in the cluster's region.
// +kubebuilder:validation:XValidation:rule="!self.?operatorConfiguration.?csiDriverOperator.?aws.?initialKMSKeyARN.hasValue() || !has(self.platform.aws) || self.operatorConfiguration.csiDriverOperator.aws.initialKMSKeyARN.split(':').size() < 4 || self.operatorConfiguration.csiDriverOperator.aws.initialKMSKeyARN.split(':')[3] == self.platform.aws.region",message="initialKMSKeyARN must be in the same region as the cluster (platform.aws.region)"
type HostedClusterSpec struct {
	// ...
```

The CEL validation regex aligns with the downstream
`ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` field in
[openshift/api](https://github.com/openshift/api/blob/master/operator/v1/types_csi_cluster_driver.go),
including the full set of AWS partitions.

The field is immutable once set and cannot be added or removed after creation. The
single presence-equality spec-level rule covers adding or removing `initialKMSKeyARN`
through any part of the parent chain in one update, using optional chaining so the
rule is inert when the parent structs are absent. The region rule guards the
ARN split against a malformed value (fewer than four colon-separated segments)
before indexing, so a malformed ARN is rejected by the format rule rather than
producing a CEL evaluation error. The platform rule restricts `csiDriverOperator.aws`
to the AWS platform.

#### KMS Error Visibility

If an invalid KMS key is configured, errors surface naturally during PVC
provisioning through the CSI driver. This is the same behavior as standalone OCP.
Proactive KMS key validation is planned as a separate feature (OCPSTRAT-3501).

#### CLI flag: `--initial-storage-volumes-kms-key`

A new flag is added to the AWS cluster creation command, consistent with the existing
`--root-volume-kms-key` naming convention:

```text
--initial-storage-volumes-kms-key string
    AWS KMS key ARN or alias ARN used to encrypt the EBS volumes provisioned by
    the default StorageClass. Applied once at cluster creation; if omitted, EBS
    volumes use AWS-managed encryption.
```

The flag value is validated client-side (ARN format and that the key region matches
the cluster region) before the HostedCluster is created. The flag is bound through the
shared options mechanism so it is exposed in both the HCP CLI (`hcp create cluster aws`)
and the developer CLI (`hypershift create cluster aws`) automatically.

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

The HCCO writes `ClusterCSIDriver.DriverConfig` only when it creates the resource
for the first time. If the resource already exists, the HCCO skips `DriverConfig`
entirely. This write-once guard prevents the HCCO from overwriting `DriverConfig`
after initial creation, preserving any in-cluster modifications made by the
administrator.

The write-once guard guarantees:
- Initial creation with `initialKMSKeyARN`: HCCO writes the field.
- Initial creation without `initialKMSKeyARN`: HCCO leaves `DriverConfig` unset.
- Day-2 admin key reconfiguration on `ClusterCSIDriver`: HCCO does not revert it.
- Day-2 admin clears `DriverConfig`: HCCO does not re-populate it.
- HCCO restart: if the resource exists, skip; if not, create. Both are correct.
- Upgrade of a cluster that never had `initialKMSKeyARN`: the resource already exists,
  `DriverConfig` is untouched.

#### Bootstrap Ordering: CSO Precondition on HCCO Reconciliation

The HCCO must write the KMS key to `ClusterCSIDriver` before the CSO creates the
default StorageClass. The CPO uses the CPOv2 component precondition mechanism to
block the CSO from starting until the HCCO has completed its first successful
reconcile. This works as follows:

1. The HCCO reconcile sets `ConfigOperatorReconciliationSucceeded=True` on the
   `HostedControlPlane` after each successful reconcile (including `reconcileStorage`).
2. The CSO component (`cluster-storage-operator`) declares a precondition on the
   `ConfigOperatorReconciliationSucceeded` HCP condition for all clusters,
   regardless of platform or whether `initialKMSKeyARN` is set. The precondition is
   attached with a gate that enforces it only while the CSO workload does not exist
   yet, so ordering is applied on the first rollout. The CPOv2 framework blocks the
   CSO's entire reconciliation (deployment creation, manifest application) until the
   precondition is satisfied, and reports `RolloutComplete=False` with a message
   that names the condition being waited on. Once the CSO workload exists, the gate
   returns false, so later updates and upgrades are not blocked by the condition.

A precondition differs from a component dependency: a dependency waits for another
component's readiness, while a precondition waits for a cluster-level HCP condition.
Gating the CSO on an HCP condition is the correct semantic here, and it keeps the
CSO's status message specific about what it is waiting for.

The resulting bootstrap sequence (when `initialKMSKeyARN` is set, the key flows
through; when unset, the same ordering holds and the `ClusterCSIDriver` is created
without a KMS key):

1. CVO installs the `ClusterCSIDriver` CRD in the guest cluster.
2. HCCO availability prober completes (its required guest-cluster CRDs are present).
3. HCCO reconciles storage, creates `ClusterCSIDriver` (with the KMS key when set),
   sets `ConfigOperatorReconciliationSucceeded=True` on the HostedControlPlane.
4. The CSO component's precondition is satisfied, and the CSO reconciliation proceeds.
5. CSO starts, finds `ClusterCSIDriver` already exists (with the KMS key when set;
   the CSO's `applyClusterCSIDriver` returns the existing object untouched).
6. CSO deploys the `aws-ebs-csi-driver-operator`.
7. The operator reads `ClusterCSIDriver` (with the KMS key already present when set)
   and creates the StorageClass correctly from the start.

The precondition is enforced for all clusters on the initial rollout, which delays
the CSO's first startup by the time the HCCO takes to complete its first reconcile.
After the CSO workload exists, the gate stops enforcing the precondition, so
subsequent updates and upgrades are never blocked by the condition. This keeps
ordering correct on first creation while avoiding any upgrade risk from a long-lived
`ConfigOperatorReconciliationSucceeded=False`.

### Risks and Mitigations

#### Invalid or Inaccessible KMS Key

If the KMS key ARN is syntactically valid but the key is disabled, deleted, or the
IAM role lacks the required permissions, PVC provisioning fails at the CSI driver
level with the AWS error surfaced in PVC events. This is the same behavior as
standalone OCP.
*Mitigation:* Document IAM permission requirements and key lifecycle responsibility.
Proactive key validation is planned as a separate feature (OCPSTRAT-3501).

### Drawbacks

- Introduces a new operator entry in `OperatorConfiguration` (`csiDriverOperator`).
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
overwriting admin changes. The write-once approach checks whether the resource already
exists rather than relying on user-modifiable metadata.

## Open Questions

None at this time.

## Test Plan

### Envtest (CEL Validation)

YAML-driven envtest cases are mandatory for all CEL validation rules per HyperShift
project convention. Test cases cover `onCreate` and `onUpdate` scenarios across the
Kubernetes API server versions in the envtest matrix (the OCP-mapped versions from
1.30 and the upstream Kubernetes versions through 1.37):

onCreate:
- Valid KMS key ARN accepted
- Valid alias ARN accepted
- Valid ARN accepted for each partition (`aws-cn`, `aws-us-gov`, `aws-iso`,
  `aws-iso-b`, `aws-iso-e`, `aws-iso-f`)
- Invalid prefix rejected
- Wrong partition rejected
- Wrong service rejected
- Empty key ID rejected
- Malformed account ID rejected
- `initialKMSKeyARN` region not matching the cluster region rejected
- `csiDriverOperator.aws` set on a non-AWS platform rejected
- Cluster without `csiDriverOperator` accepted

onUpdate:
- Changing `initialKMSKeyARN` value is rejected
- Removing `initialKMSKeyARN` is rejected
- Removing `csiDriverOperator` while `initialKMSKeyARN` was set is rejected
- Adding `initialKMSKeyARN` to a cluster created without it is rejected
- Removing `operatorConfiguration` while `initialKMSKeyARN` was set is rejected
- Updating an unrelated field on a cluster with `initialKMSKeyARN` set is accepted

### E2E Tests

An e2e test in the HyperShift e2e-v2 suite (`test/e2e/v2`) runs against a
pre-existing hosted cluster on live AWS infrastructure. The test reuses the existing
CI KMS key (`alias/hypershift-ci`) already provisioned in the CI AWS account.

The test is labeled `storage-kms` and is included in the `public` cluster group's
label filter. The public cluster is created with `--initial-storage-volumes-kms-key`
only when the create-guests harness sees `HYPERSHIFT_STORAGE_KMS_KEY_ALIAS` set
(which resolves the alias to an ARN). The test gates itself on several preconditions
and skips (rather than fails) when any is unmet: the platform is not AWS, the hosted
cluster release is below 5.1 (the release in which the propagation landed), no
`operatorConfiguration` or `initialKMSKeyARN` is configured on the hosted cluster, or
`AWS_GUEST_INFRA_CREDENTIALS_FILE` is not set for the encryption check.

The implemented scenario is day-1 key configuration:

1. **Day-1 key configuration:**
   - Create a `HostedCluster` with `operatorConfiguration.csiDriverOperator.aws.initialKMSKeyARN`
     set
   - Verify `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` and the default
     StorageClass `parameters.kmsKeyId` are set in the hosted cluster
   - Create a PVC using the default StorageClass, wait for it to bind
   - Verify the resulting EBS volume is encrypted with the specified KMS key via
     `ec2.DescribeVolumes`

Write-once semantics (an admin edit to `ClusterCSIDriver.spec.driverConfig`
persisting across HCCO reconciles) and the no-key regression path are covered by
the unit tests on the HCCO reconcile function; dedicated e2e scenarios for them are
follow-up work and are not yet part of the e2e suite.

## Graduation Criteria

### Dev Preview -> Tech Preview

This feature ships directly to GA. No Dev Preview or Tech Preview phase.

### Tech Preview -> GA

See above.

### GA

- Envtest coverage for CEL validation across multiple Kubernetes versions.
- E2E coverage for day-1 key configuration (creation and volume encryption). Write-once
  semantics and the no-key regression path are covered by HCCO reconcile unit tests;
  dedicated e2e scenarios for them are follow-up work.
- E2E tests passing in CI for at least one release cycle without flakes.
- User documentation merged in `openshift-docs` covering the
  `--initial-storage-volumes-kms-key` flag, day-1 encryption setup, and day-2 in-cluster
  key management via `ClusterCSIDriver`.
- No open blocking bugs.

### Removing a deprecated feature

Not applicable. This enhancement adds new capability; nothing is deprecated.

## Upgrade / Downgrade Strategy

#### Upgrade

The new field is optional with `omitempty`. Existing `HostedCluster`
objects gain the field on upgrade, defaulting to empty. No action is required from
customers. On upgrade, the `ClusterCSIDriver` resource already exists, so the HCCO
does not modify `DriverConfig`.

#### Failed Upgrade Rollback

Control plane downgrades are not supported in
HyperShift. If an N->N+1 upgrade fails mid-way, the `initialKMSKeyARN` field is not yet
active and has no effect on storage behavior. If `initialKMSKeyARN` was already configured
on a successfully upgraded cluster, the `ClusterCSIDriver` in the hosted cluster
retains its last-written `DriverConfig` throughout any subsequent upgrade attempts.

## Version Skew Strategy

If `initialKMSKeyARN` is set on a HostedCluster running a guest release that predates this
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
