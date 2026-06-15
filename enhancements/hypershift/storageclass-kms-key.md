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
cannot currently expose this capability, even though the underlying CSI operator
already supports it via `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN`
(established in [openshift/enhancements#1163](https://github.com/openshift/enhancements/pull/1163)).
HyperShift simply does not expose a creation-time knob in the `HostedCluster` API or
propagate it to the guest cluster.

Self-managed HyperShift on AWS has the same gap: operators cannot configure default
StorageClass encryption at cluster creation time.

### User Stories

- As a ROSA HCP cluster administrator, I want to specify a KMS key ARN when creating
  a cluster so that all PVCs provisioned by the default StorageClass are encrypted with
  my organization's key instead of the default AWS-managed key.

- As a ROSA HCP cluster administrator, I want to update the KMS encryption
  configuration on a running cluster by editing the `ClusterCSIDriver` resource
  directly in the guest cluster, so that I can rotate keys or change encryption
  settings without recreating the cluster.

- As a self-managed HyperShift operator or HyperShift developer on AWS, I want to
  specify a KMS key for the default StorageClass at cluster creation time via the
  `hcp` or `hypershift` CLI so that I can enforce encryption standards from day 1.

- As an OpenShift cluster administrator (standalone or HyperShift), I want the
  StorageClass controller to validate the configured KMS key and report a degraded
  condition if the key is invalid or the role lacks permissions, so I can diagnose encryption issues without
  waiting for PVC provisioning failures.

### Goals

- Add an optional `kmsKeyARN` field under
  `spec.operatorConfiguration.csiDriverConfig.aws` as a string field accepting
  KMS key ARNs and alias ARNs.
- Propagate the field at cluster creation from `HostedCluster` →
  `HostedControlPlane` → `ClusterCSIDriver` in the guest cluster (write-once,
  not continuously reconciled).
- Validate the KMS key in the aws-ebs-csi-driver-operator's StorageClass hook and
  report `StorageClassControllerDegraded` if the key is invalid or the role lacks
  permissions. This applies
  to both standalone and HyperShift clusters.
- Expose `--storage-volumes-kms-key` in the `hcp create cluster aws` command and the
  `hypershift create cluster aws` developer CLI.
- Apply identically to ROSA HCP and self-managed HyperShift on AWS.
- Preserve backward compatibility: clusters without the field continue to use
  AWS-managed encryption with no behavioral change.
- Preserve existing `ClusterCSIDriver` editability: cluster administrators can
  continue to modify `ClusterCSIDriver` directly in the guest cluster for day-2
  configuration changes, and the HCCO will not overwrite those changes.

### Non-Goals

- **ROSA CLI (`rosa create cluster`), Terraform, CAPI, and Hybrid Cloud Console
  integration** are out of scope for this enhancement. These are downstream concerns
  tracked separately by the ROSA product team, which consumes the upstream
  `HostedCluster` API to wire it into their clients.
- **Per-StorageClass KMS key granularity.** This targets only the default
  StorageClass. Users can still create custom StorageClasses with their own keys.
  Per-StorageClass granularity would require CSI operator changes, out of scope.
- **Day-2 key management via the HostedCluster API.** Day-2 key rotation and removal
  are performed directly on `ClusterCSIDriver` in the guest cluster. The HC field
  captures day-1 intent only.
- **Re-encrypting existing PVs.** The KMS key applies to newly created PVCs only.
  Existing volumes retain their original encryption.
- **NodePool root volume encryption.** Root volume encryption is configured separately
  on `NodePool.spec.platform.aws.rootVolume.encryptionKey` and is not affected by this
  enhancement.
- **Etcd encryption.** Etcd KMS key management is handled by the existing
  `ValidAWSKMSConfig` mechanism using a dedicated `AWSKMSRoleARN` role. This
  enhancement does not affect that code path.

## Proposal

When a customer sets `spec.operatorConfiguration.csiDriverConfig.aws.kmsKeyARN` on a
`HostedCluster` at creation time, the following propagation chain executes:

1. The HC controller mirrors the `operatorConfiguration` (including the new `aws`
   subfield) to `HostedControlPlane`.
2. The HCCO storage reconciliation writes
   `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` to the guest cluster via its
   guest cluster client. This is a **write-once operation**: if the `ClusterCSIDriver`
   resource already exists (has a `ResourceVersion`), the HCCO skips writing
   `DriverConfig`, preserving in-cluster modifications (like
   `ReconcileDefaultIngressController`, which returns early if the resource exists).
3. The **aws-ebs-csi-driver-operator** (in `openshift/csi-operator`) validates the
   KMS key in the existing `withKMSKeyHook` StorageClass hook. Before injecting
   `kmsKeyId` into the StorageClass, the hook calls `kms:Encrypt` with a test
   payload. If the call fails, the hook returns an error with the AWS SDK error
   code and message (using `smithy.APIError`, same pattern as the volume tags
   controller in the same package). The `WithSyncDegradedOnError` framework
   automatically sets `StorageClassControllerDegraded = True` with the error
   details in the condition message.
4. The CSO reads the `ClusterCSIDriver` value and configures the default StorageClass
   with `parameters.kmsKeyId`. New EBS volumes provisioned via the StorageClass carry
   the KMS encryption.

The cluster-storage-operator already reads
`ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` and configures the default
StorageClass accordingly. This was established in
[openshift/enhancements#1163](https://github.com/openshift/enhancements/pull/1163).
No CSO changes are needed.

### Workflow Description

#### Actors

- **Cluster administrator:** A human operator who creates or manages `HostedCluster`
  resources (ROSA HCP customer or self-managed HyperShift operator).
- **HC controller:** The HyperShift HostedCluster controller running on the management
  cluster.
- **HCCO:** The Hosted Cluster Config Operator running in the HCP namespace on the
  management cluster.
- **CSO:** The cluster-storage-operator, running in the HCP namespace on the
  management cluster.

#### Day-1: Cluster Creation with KMS Key

1. The cluster administrator runs:
   ```bash
   hcp create cluster aws \
     --storage-volumes-kms-key arn:aws:kms:us-east-1:123456789012:key/mrk-abc123 \
     ...
   ```
   or sets `spec.operatorConfiguration.csiDriverConfig.aws.kmsKeyARN` directly on the
   `HostedCluster` manifest.
2. The HC controller creates the `HostedControlPlane` with `operatorConfiguration`
   mirrored.
3. The HCCO writes `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` in the guest
   cluster (write-once: only on initial creation).
4. The aws-ebs-csi-driver-operator validates the key in the StorageClass hook:
   - The `withKMSKeyHook` reads `kmsKeyARN` from
     `ClusterCSIDriver.spec.driverConfig.aws`
   - Before injecting `kmsKeyId` into the StorageClass, the hook calls
     `kms.Encrypt` with the key ARN and a small test payload using the
     `ebs-cloud-credentials` (same IRSA credential path as the volume tags
     controller)
   - The validation result is cached by key ARN. Re-validation occurs only when
     the ARN changes or on a periodic interval (every 30 minutes, matching the
     `EBSVolumeTagsController`'s resync period), not on every StorageClass
     controller resync
   - If the call succeeds, the hook injects `kmsKeyId` into the StorageClass and
     returns nil
   - If the call fails, the hook extracts the AWS error code and message via
     `smithy.APIError` (e.g. `KMSKeyDisabled`, `AccessDeniedException`,
     `NotFoundException`) and returns an error. `WithSyncDegradedOnError` sets
     `StorageClassControllerDegraded = True` with a message like:
     `"error running hook function (index=1): KMS key arn:aws:kms:...:key/abc: KMSKeyDisabled: key is disabled"`
5. The CSO configures the default StorageClass with `parameters.kmsKeyId` set to the
   ARN.
6. New PVCs created from the default StorageClass produce EBS volumes encrypted with
   the customer's key.

#### Day-2: Key Rotation (In-Cluster)

Day-2 key rotation is performed by the cluster administrator directly on the
`ClusterCSIDriver` resource in the guest cluster:

```bash
oc patch clustercsidriver ebs.csi.aws.com --type merge \
  -p '{"spec":{"driverConfig":{"driverType":"AWS","aws":{"kmsKeyARN":"arn:aws:kms:us-east-1:123456789012:key/new-key"}}}}'
```

The CSO picks up the change and updates the default StorageClass. New PVCs use the
new key. Existing PVs retain encryption with the original key. The HCCO does not overwrite in-cluster `ClusterCSIDriver` after initial creation.

#### Day-2: Disabling KMS Encryption (In-Cluster)

The cluster administrator clears `DriverConfig` on the `ClusterCSIDriver` directly in
the guest cluster. The CSO reverts the default StorageClass to AWS-managed encryption.

### API Extensions

#### New field: `kmsKeyARN` on `AWSCSIDriverConfig`

A new field is added under `spec.operatorConfiguration.csiDriverConfig.aws` on both
`HostedCluster` and `HostedControlPlane`. This follows the ingress operator pattern
where platform-specific configuration is nested inside the operator's own config
(`ingressOperator.endpointPublishingStrategy.loadBalancer.providerParameters.aws`).
The `CSIDriverOperatorConfig` struct naturally extends to `azure`, `gcp` in the future.

New types:

```go
// CSIDriverOperatorConfig specifies configuration for CSI driver operators
// in the hosted cluster.
type CSIDriverOperatorConfig struct {
	// aws specifies configuration for the AWS EBS CSI driver operator.
	// +optional
	AWS *AWSCSIDriverConfig `json:"aws,omitempty"`
}

// AWSCSIDriverConfig specifies configuration for the AWS EBS CSI driver.
type AWSCSIDriverConfig struct {
	// kmsKeyARN sets the cluster default storage class to encrypt volumes with
	// a user-defined KMS key, rather than the default KMS key used by AWS.
	// The value may be either the ARN or Alias ARN of a KMS key.
	//
	// The ARN must follow the format:
	//   arn:<partition>:kms:<region>:<account-id>:(key|alias)/<key-id-or-alias>
	// where <partition> is the AWS partition (aws, aws-cn, aws-us-gov, aws-iso,
	// aws-iso-b, aws-iso-e, or aws-iso-f).
	//
	// This field is applied at cluster creation time only. Day-2 changes to
	// storage encryption should be made directly on the ClusterCSIDriver
	// resource in the guest cluster.
	//
	// The StorageARN role in AWSRolesRef must have kms:Encrypt (for validation),
	// kms:Decrypt, kms:GenerateDataKeyWithoutPlaintext, and kms:CreateGrant
	// permissions on the specified key.
	//
	// +optional
	// +kubebuilder:validation:MaxLength=2048
	// +openshift:validation:FeatureGateAwareXValidation:featureGate="",rule="matches(self, '^arn:(aws|aws-cn|aws-us-gov|aws-iso|aws-iso-b|aws-iso-e|aws-iso-f):kms:[a-z0-9-]+:[0-9]{12}:(key|alias)/.*$')",message="kmsKeyARN must be a valid AWS KMS key ARN in the format: arn:<partition>:kms:<region>:<account-id>:(key|alias)/<key-id-or-alias>"
	KMSKeyARN string `json:"kmsKeyARN,omitempty"`
}
```

Added to `OperatorConfiguration`:

```go
type OperatorConfiguration struct {
	// ... existing fields (clusterVersionOperator, clusterNetworkOperator, ingressOperator)

	// csiDriverConfig specifies configuration for CSI driver operators in the hosted cluster.
	// +optional
	CSIDriverConfig *CSIDriverOperatorConfig `json:"csiDriverConfig,omitempty"`
}
```

The CEL validation regex aligns with the downstream
`ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` field in
[openshift/api](https://github.com/openshift/api/blob/master/operator/v1/types_csi_cluster_driver.go),
including the full set of AWS partitions.

#### KMS Error Visibility

KMS key errors surface through the standard operator health chain. When the
StorageClass hook's `kms:Encrypt` probe fails, `StorageClassControllerDegraded`
is set with the specific AWS error code and message. This propagates through
the CSO to `ClusterOperator "storage"` Degraded, then through the CVO to
`ClusterVersion`, and in HyperShift to `ClusterVersionSucceeding` on the
`HostedCluster`. There is no dedicated KMS condition on the `HostedCluster`.

#### CLI flag: `--storage-volumes-kms-key`

A new flag is added to the AWS cluster creation command, consistent with the existing
`--root-volume-kms-key` naming convention:

```text
--storage-volumes-kms-key string
    AWS KMS key ARN (arn:...:key/...) or alias ARN (arn:...:alias/...) used
    to encrypt PVCs created by the default StorageClass at cluster creation.
    If omitted, PVCs use AWS-managed encryption. The StorageARN role must have
    kms:Encrypt, kms:Decrypt, kms:GenerateDataKeyWithoutPlaintext, and kms:CreateGrant
    permissions on the specified key. Day-2 key changes should be made directly
    on the ClusterCSIDriver resource in the guest cluster.
```

The flag is bound through the shared options mechanism so it is exposed in both the
HCP CLI (`hcp create cluster aws`) and the developer CLI
(`hypershift create cluster aws`) automatically.

### Topology Considerations

#### Hypershift / Hosted Control Planes

All control plane components (HC controller, HCCO, CSO, CSI driver operators) run on
the management cluster in the HCP namespace. The HCCO uses a dual-client architecture:
a management cluster client for reading `HostedControlPlane` spec, and a guest cluster
client (via an injected guest kubeconfig) for writing `ClusterCSIDriver` and cloud
credential secrets. CSI DaemonSets run on guest cluster worker nodes.

The `StorageARN` role is an IRSA-style (IAM Roles for Service Accounts) role that the
HCCO already provisions for storage credential management (`ebs-cloud-credentials`).
KMS validation reuses this existing IRSA credential acquisition path. No new roles
or permissions are introduced beyond what the feature already requires.

#### Standalone Clusters

The KMS key validation in the StorageClass hook applies to standalone clusters as
well. When `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` is set on a standalone
cluster, the same `kms:Encrypt` probe runs and `StorageClassControllerDegraded` fires
if the key is invalid or permissions are insufficient. The `HostedCluster` API field
is HyperShift-only.

#### Single-node Deployments or MicroShift

Not applicable.

#### OpenShift Kubernetes Engine (OKE)

Not applicable. Storage KMS configuration in standard OpenShift clusters is handled
via `ClusterCSIDriver` directly.

### Implementation Details/Notes/Constraints

#### IAM Permissions

The StorageARN role requires the following KMS permissions:
- `kms:Encrypt` (for key validation in the StorageClass hook)
- `kms:Decrypt` (for EBS volume attachment)
- `kms:GenerateDataKeyWithoutPlaintext` (for EBS volume creation)
- `kms:CreateGrant` (for EBS service principal access)

ROSA HCP clusters require these permissions in the
`ROSAAmazonEBSCSIDriverOperatorPolicy` AWS managed policy. Self-managed
HyperShift clusters require equivalent permissions on the StorageARN role.

#### Day-1-Only Reconciliation

The HCCO writes `kmsKeyARN` to
`ClusterCSIDriver` only on initial resource creation. If the `ClusterCSIDriver` already
exists (has a `ResourceVersion`), the reconcile function returns early without
modifying `DriverConfig`. This follows the ingress controller pattern
(`ReconcileDefaultIngressController` returns early when the resource exists).

#### KMS Key Validation

Validation is performed by the existing `withKMSKeyHook` in the
aws-ebs-csi-driver-operator (in `openshift/csi-operator`). The hook already reads
`kmsKeyARN` from `ClusterCSIDriver.spec.driverConfig.aws` and injects it into the
StorageClass. The enhancement extends this hook to call `kms:Encrypt` with a test
payload before injecting the key.

The hook builder (`withKMSKeyHook(c *clients.Clients)`) already captures the
`ClusterCSIDriver` informer lister via closure. The AWS config (region, IRSA
credentials via `ebs-cloud-credentials`) is captured the same way, following the
pattern established by the `EBSVolumeTagsController` in the same package
(`pkg/driver/aws-ebs/aws_ebs_tags_controller.go`), which uses
`stscreds.NewWebIdentityRoleProvider` with session expiry handling.

If the `kms.Encrypt` call fails, the hook extracts the AWS error code and message
via `smithy.APIError` (same pattern as the volume tags queue worker in
`pkg/driver/aws-ebs/aws_ebs_tags_queue_worker.go`) and returns an error like:
`"KMS key arn:aws:kms:...:key/abc: KMSKeyDisabled: key is disabled"`.
The StorageClass controller's `WithSyncDegradedOnError` wraps this and sets
`StorageClassControllerDegraded = True` with a message like:
`"error running hook function (index=1): KMS key arn:aws:kms:...:key/abc: KMSKeyDisabled: key is disabled"`.

To avoid unnecessary AWS API calls, the hook caches the validation result by key
ARN. Re-validation occurs only when the ARN changes or on a periodic interval
(every 30 minutes, matching the `EBSVolumeTagsController`'s resync period), not
on every StorageClass controller resync (which runs every minute). A cached
failure clears when the ARN changes, triggering immediate re-validation with the
new key.

#### Condition Propagation Chain

The `StorageClassControllerDegraded` condition propagates through the standard
operator health chain:
1. The CSO's `CSIDriverOperatorCRController.syncConditions()` copies it to the
   Storage operator CR as `AWSEBSCSIDriverOperatorCRDegraded`
2. The CSO's `ClusterOperatorStatusController` aggregates it into the
   `ClusterOperator "storage"` Degraded signal
3. The CVO aggregates into `ClusterVersion` conditions
4. In HyperShift, the HCCO bubbles `ClusterVersion` conditions to HCP status,
   and the HC controller copies them to `HostedCluster` as
   `ClusterVersionSucceeding`

Changes are required in `openshift/csi-operator` (extending the KMS hook with
validation). No HyperShift-side changes are needed for condition propagation.

#### Why the Driver Operator

 The driver operator is
the component closest to the actual KMS usage. It already reads `kmsKeyARN` from
`ClusterCSIDriver`, injects it into the StorageClass, and has the AWS SDK + IRSA
credential plumbing for making AWS API calls. Placing validation here means the probe
validates what the operator actually consumes, using the same credentials that the CSI
driver will use at volume creation time. Standalone (non-HyperShift) clusters get the
same validation automatically.

### Risks and Mitigations

#### Key Disabled After Volumes Encrypted

If the customer disables or deletes the KMS key in AWS after volumes are encrypted,
those volumes become inaccessible. HyperShift cannot prevent this.
*Mitigation:* `StorageClassControllerDegraded` fires on the next StorageClass controller
reconcile (every minute) with the specific AWS error code in the message. In
HyperShift, this surfaces as `ClusterVersionSucceeding = False`. Document the
key lifecycle responsibility.

#### IAM Role Misconfiguration

If the `StorageARN` role lacks the required KMS permissions on the key, the KMS probe fails and `StorageClassControllerDegraded = True` is set with the
AWS error code in the message.
*Mitigation:* The condition message includes the failing ARN and a remediation hint.
The HCCO still writes the ARN to `ClusterCSIDriver` (validation does not gate
reconciliation), so PVC
provisioning may fail at the CSI driver level until IAM permissions are corrected.

#### Existing Cluster Behavior on Upgrade

Clusters without `kmsKeyARN` must not experience any behavior change after upgrading
to a version containing this feature.
*Mitigation:* The field is optional with `omitempty`. When the field is absent, the StorageClass hook skips KMS validation and no
degraded condition is set. The write-once pattern (see Implementation Details) prevents any behavioral change.

#### Disrupting ClusterCSIDriver Editability

Cluster administrators and GitOps workflows may already rely on `ClusterCSIDriver`
being directly editable in the guest cluster. Continuously overwriting it from the
HC spec would be a disruptive change.
*Mitigation:* The write-once pattern (see Implementation Details) preserves this.

### Drawbacks

- Introduces a new operator entry in `OperatorConfiguration` (`csiDriverConfig`).
  Platform branching is inside the operator config, following the ingress operator
  pattern.
- The KMS probe in the StorageClass hook adds one AWS API call per StorageClass
  controller reconcile (every minute) when a KMS key is configured.

## Alternatives (Not Implemented)

#### Field on AWSPlatformSpec

Placing `storageKMSKeyARN` directly on the existing `AWSPlatformSpec` struct was the
initial design. Rejected because `AWSPlatformSpec` holds platform infrastructure
configuration (region, VPC, IAM roles), while this field configures an operator
(the CSI driver). The `operatorConfiguration` struct is where operator configs belong. OCP APIs are hard to modify after creation;
getting the nesting right before GA avoids a future deprecation cycle.

#### Continuous Reconciliation of ClusterCSIDriver

Continuously reconciling `ClusterCSIDriver.DriverConfig` from the HC spec (like
OAuth configuration) was considered. Rejected because `ClusterCSIDriver` is already
editable by cluster administrators in the guest cluster, and breaking that UX would
be disruptive. The ingress controller uses the same day-1-setup / day-2-admin-control model.

#### Validation in CPO

Validation could be placed alongside the existing etcd KMS validation in the CPO.
Rejected because CPO uses a different role (`AWSKMSRoleARN`) for etcd encryption, and
storage concerns belong with the storage operator stack (the
aws-ebs-csi-driver-operator's StorageClass hook). Splitting would
increase coordination surface and mix concerns across operators.

#### Propagation-Confirmation-Only Validation

Verify only that the ARN was written to `ClusterCSIDriver`, without calling KMS
`Encrypt`. Rejected because this approach cannot detect IAM permission issues until
the first volume creation failure, which is too late. The first sign would be failed PVCs.

## Open Questions

None at this time.

## Test Plan

### Envtest (CEL Validation)

YAML-driven envtest cases are mandatory for all CEL validation rules per HyperShift
project convention. Test cases cover `onCreate` and `onUpdate` scenarios across
multiple Kubernetes API server versions to verify ratcheting compatibility:

- Valid KMS key ARN accepted (e.g., `arn:aws:kms:us-east-1:123456789012:key/mrk-abc123`)
- Valid alias ARN accepted (e.g., `arn:aws:kms:us-east-1:123456789012:alias/my-key`)
- Invalid format rejected: missing `arn:` prefix
- Invalid format rejected: wrong partition (e.g., `arn:gcp:kms:...`)
- Invalid format rejected: wrong service (e.g., `arn:aws:s3:...`)
- Invalid format rejected: malformed key ID
- Invalid format rejected: value exceeding `MaxLength=2048`
- Regression: cluster created without `kmsKeyARN` is unaffected

### Unit Tests

Unit tests will cover the propagation chain (HC controller mirroring, HCCO storage
reconciliation with write-once semantics, condition bubble-up), KMS validation logic
(all condition states), and CLI flag wiring. Specific test cases:

- HC controller mirrors `operatorConfiguration.csiDriverConfig.aws` from HC to HCP
- HCCO writes `ClusterCSIDriver.DriverConfig.AWS.KMSKeyARN` on initial creation
- HCCO skips `DriverConfig` when `ClusterCSIDriver` already exists (write-once)
- StorageClass hook: valid key → no error, invalid key → error with AWS error code,
  no key → hook returns nil (no validation)
- CLI: `--storage-volumes-kms-key` flag parsed and wired to HC spec

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

- Unit test coverage for all propagation and validation paths.
- Envtest coverage for CEL validation across multiple Kubernetes versions.
- E2E test coverage for the full lifecycle (creation, write-once verification).
- E2E tests passing in CI for at least one release cycle without flakes.
- Upgrade testing completed (field present on upgrade, absent clusters unaffected).
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
customers. The write-once pattern means existing `ClusterCSIDriver` resources are not
modified on upgrade.

#### Failed Upgrade Rollback

Control plane downgrades are not supported in
HyperShift. If an N→N+1 upgrade fails mid-way, the `kmsKeyARN` field is not yet
active and has no effect on storage behavior. If `kmsKeyARN` was already configured
on a successfully upgraded cluster, the `ClusterCSIDriver` in the hosted cluster
retains its last-written `DriverConfig` throughout any subsequent upgrade attempts.

## Version Skew Strategy

All components in the propagation chain (HC controller, HCCO, CSO) run on the
management cluster in the HCP namespace and are versioned together with the
HyperShift release. There is no multi-version skew within the propagation chain.

The `ClusterCSIDriver` CRD in the guest cluster already includes
`spec.driverConfig.aws.kmsKeyARN` from the AWS EBS CSI driver. HyperShift writes
this field only at initial cluster creation, so guest clusters running older OCP
versions that do not recognize the field will ignore it gracefully (Kubernetes
unknown-field pruning applies at admission).

## Operational Aspects of API Extensions

#### SLIs The condition transition from `Unknown` to

`True` or `False` after cluster creation is the primary health indicator. The
condition reaches its terminal state within one HCCO reconcile interval during
cluster provisioning.

#### Impact on Existing SLIs

The StorageClass hook adds one KMS API call every 30 minutes when `kmsKeyARN` is
configured on `ClusterCSIDriver`, with immediate re-validation if the ARN changes.

#### Failure Modes

- If the KMS probe fails, `StorageClassControllerDegraded` is set but the
  `ClusterCSIDriver` field is still written (validation does not gate
  reconciliation). PVC provisioning may fail at the CSI driver level if the key
  is invalid or permissions are insufficient.
- No impact on existing workloads or control plane availability.

## Support Procedures

#### Detecting Failures

- Inspect `ClusterCSIDriver.status.conditions` for `StorageClassControllerDegraded`.
- In HyperShift, `ClusterVersionSucceeding = False` on the `HostedCluster` indicates
  an operator health issue. Check `ClusterOperator "storage"` for details.
- The Degraded condition message includes the failing ARN and AWS error code.

#### Diagnosing IAM Permission Errors

1. Verify the `StorageARN` role in `HostedCluster.spec.platform.aws.rolesRef.storageARN`.
2. Confirm the role's IAM policy includes `kms:Encrypt` (for validation),
   `kms:Decrypt`, `kms:GenerateDataKeyWithoutPlaintext`, and `kms:CreateGrant`
   for the key ARN.
3. Confirm the key policy allows the `StorageARN` role principal.

#### Diagnosing KMS Key Errors

1. Confirm the KMS key is enabled in the AWS console / CLI.
2. Confirm the key exists in the correct AWS region (matches the cluster region).
3. For alias ARNs: confirm the alias points to an enabled key.

#### Day-2 Storage Encryption Changes

Day-2 key rotation, key removal, or other `ClusterCSIDriver` changes are made
directly in the guest cluster by the cluster administrator. The HCCO does not
manage `ClusterCSIDriver.DriverConfig` after initial creation.

#### Graceful Failure

If the StorageClass hook's KMS validation fails, `StorageClassControllerDegraded`
is set but control plane provisioning continues. New PVC provisioning may fail at
the CSI driver level if the key is invalid or permissions are insufficient.

## Infrastructure Needed

No new subprojects, repositories, or testing infrastructure are required. The E2E
tests run in the existing HyperShift E2E CI jobs (`e2e-aws`, `e2e-aws-ovn`),
which already provision live AWS infrastructure with KMS access via the
`alias/hypershift-ci` key. The `StorageARN` role in the CI account must have
`kms:Encrypt`, `kms:Decrypt`, `kms:GenerateDataKeyWithoutPlaintext`, and
`kms:CreateGrant` permissions added for this key.

Changes are required in the following repositories:
- `openshift/csi-operator`: extend `withKMSKeyHook` in `pkg/driver/aws-ebs/aws_ebs.go`
  with `kms:Encrypt` validation, addition of `aws-sdk-go-v2/service/kms` dependency
- `openshift/hypershift`: API field, CLI flag, HCCO write-once propagation