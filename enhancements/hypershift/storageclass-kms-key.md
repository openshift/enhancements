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
  directly in the guest cluster, so that I can change keys or modify encryption
  settings without recreating the cluster.

- As a self-managed HyperShift operator or HyperShift developer on AWS, I want to
  specify a KMS key for the default StorageClass at cluster creation time via the
  `hcp` or `hypershift` CLI so that I can enforce encryption standards from day 1.

### Goals

- Add an optional `kmsKeyARN` field under
  `spec.operatorConfiguration.csiDriverConfig.aws` as a string field accepting
  KMS key ARNs and alias ARNs.
- Propagate the field at cluster creation from `HostedCluster` ->
  `HostedControlPlane` -> `ClusterCSIDriver` in the guest cluster (set-once,
  not continuously reconciled).
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
- **Day-2 key management via the HostedCluster API.** Day-2 key reconfiguration and removal
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
   guest cluster client. This is a **set-once operation** guarded by an annotation
   on the `ClusterCSIDriver` resource
   (`hypershift.openshift.io/storage-driver-config-applied`). The HCCO sets this
   annotation on its first storage reconciliation pass, regardless of whether
   `kmsKeyARN` is configured. On subsequent reconciles, the annotation's presence
   tells the HCCO to skip `DriverConfig` entirely, preserving any in-cluster
   modifications made by the administrator.
3. The CSO reads the `ClusterCSIDriver` value and configures the default StorageClass
   with `parameters.kmsKeyId`. New EBS volumes provisioned via the StorageClass carry
   the KMS encryption.

When `kmsKeyARN` is set, the CPOv2 component framework blocks the CSO from starting
until the HCCO has completed its first successful reconcile. This ensures the HCCO
creates `ClusterCSIDriver` with the KMS key before the CSO or its operands read it.
See "Bootstrap Ordering" under Implementation Details.

The cluster-storage-operator already reads
`ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` and configures the default
StorageClass accordingly. This was established in
[openshift/enhancements#1163](https://github.com/openshift/enhancements/pull/1163).
No changes to the cluster-storage-operator codebase are needed. The CSO's startup
ordering is configured in the HyperShift CPOv2 component registration.

### Workflow Description

#### Actors

- **Cluster administrator:** A human operator who creates or manages `HostedCluster`
  resources (ROSA HCP customer or self-managed HyperShift operator).
- **HC controller:** The HyperShift HostedCluster controller, part of the HyperShift
  operator running in the `hypershift` namespace on the management cluster.
- **CPO:** The Control Plane Operator, a per-HCP deployment running in the HCP
  namespace on the management cluster. Manages ~40 control plane components via the
  CPOv2 component framework, including deployment creation, dependency ordering, and
  status reporting.
- **HCCO:** The Hosted Cluster Config Operator, a subcommand of the CPO binary (from
  the guest release payload), running in the HCP namespace on the management cluster.
- **CSO:** The cluster-storage-operator (from the guest release payload), running in
  the HCP namespace on the management cluster.

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
   cluster and sets the `hypershift.openshift.io/storage-driver-config-applied`
   annotation. Subsequent reconciles see the annotation and skip `DriverConfig`.
4. The CSO configures the default StorageClass with `parameters.kmsKeyId` set to the
   ARN.
5. New PVCs created from the default StorageClass produce EBS volumes encrypted with
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
touch `ClusterCSIDriver.DriverConfig` after the annotation is set.

#### Day-2: Disabling KMS Encryption (In-Cluster)

The cluster administrator clears `DriverConfig` on the `ClusterCSIDriver` directly in
the guest cluster. The CSO reverts the default StorageClass to AWS-managed encryption.
The HCCO does not re-populate the field because the annotation is already present.

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
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.aws) || has(self.aws)",message="aws is immutable once set and cannot be removed"
type CSIDriverOperatorConfig struct {
	// aws specifies configuration for the AWS EBS CSI driver operator.
	// +optional
	AWS *AWSCSIDriverConfig `json:"aws,omitempty"`
}

// AWSCSIDriverConfig specifies configuration for the AWS EBS CSI driver.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.kmsKeyARN) || has(self.kmsKeyARN)",message="kmsKeyARN is immutable once set and cannot be removed"
type AWSCSIDriverConfig struct {
	// kmsKeyARN is the ARN of an AWS KMS key used to encrypt the default
	// StorageClass volumes in the guest cluster. When set, the HCCO configures
	// ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN on the guest cluster's
	// ebs.csi.aws.com ClusterCSIDriver resource, which causes the
	// cluster-storage-operator to set the kmsKeyId parameter on the default
	// StorageClass.
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
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.csiDriverConfig) || has(self.csiDriverConfig)",message="csiDriverConfig is immutable once set and cannot be removed"
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

The immutability rules follow the `HCPEtcdBackupS3.kmsKeyARN` precedent in
`api/hypershift/v1beta1/etcdbackup_types.go`: a "cannot be removed" guard on each
parent struct (`OperatorConfiguration`, `CSIDriverOperatorConfig`,
`AWSCSIDriverConfig`) and a `self == oldSelf` rule on the field itself. The parent
guards prevent the two-step bypass where a user nils an optional parent struct to
circumvent the child's immutability rule (documented in `api/AGENTS.md`).

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

The HC controller runs in the `hypershift` namespace as part of the HyperShift
operator. The HCCO, CSO, and CSI driver operators run in the per-cluster HCP namespace
on the management cluster; their binaries come from the guest cluster's release
payload. The HCCO uses a dual-client architecture: a management cluster client for
reading `HostedControlPlane` spec, and a guest cluster client (via an injected guest
kubeconfig) for writing `ClusterCSIDriver` and cloud credential secrets. CSI
DaemonSets run on guest cluster worker nodes.

The `StorageARN` role is an IRSA-style (IAM Roles for Service Accounts) role that the
HCCO already provisions for storage credential management (`ebs-cloud-credentials`).
No new roles or permissions are introduced beyond what the feature already requires.

#### Standalone Clusters

Not applicable. This enhancement targets only `HostedCluster` resources managed by
HyperShift. The `ClusterCSIDriver.spec.driverConfig.aws.kmsKeyARN` field already
exists in standalone OCP.

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

#### Set-Once Reconciliation

The HCCO uses an annotation on the `ClusterCSIDriver` resource to ensure
`DriverConfig` is written at most once. On each storage reconciliation pass, the
HCCO checks for the `hypershift.openshift.io/storage-driver-config-applied`
annotation on the `ebs.csi.aws.com` `ClusterCSIDriver`:

- **Annotation absent:** The HCCO writes `DriverConfig.AWS.KMSKeyARN` (if
  `kmsKeyARN` is set in the HCP spec) and sets the annotation. If `kmsKeyARN` is
  not set, the HCCO sets the annotation without writing `DriverConfig`, committing
  the fact that the initial storage configuration pass has completed.
- **Annotation present:** The HCCO skips `DriverConfig` entirely. This preserves
  any in-cluster modifications made by the administrator.

This approach avoids the race condition that affects the `ResourceVersion` check
used by the ingress controller pattern. For `IngressController`, the HCCO creates
the resource before any other controller, so `CreateOrUpdate` hits the Create path
where `ResourceVersion` is empty. For `ClusterCSIDriver`, both the HCCO and the
cluster-storage-operator attempt to create it during cluster bootstrap, and the
creation order is not deterministic. The annotation check is independent of
creation ordering.

The set-once guard ensures:
- Initial creation with `kmsKeyARN`: HCCO writes the field and sets the annotation.
- Initial creation without `kmsKeyARN`: HCCO sets the annotation without writing.
- Day-2 admin key reconfiguration on `ClusterCSIDriver`: HCCO does not revert it
  (annotation present).
- Day-2 admin clears `DriverConfig`: HCCO does not re-populate it
  (annotation present).
- HCCO crash before annotation is set: on restart, the HCCO retries
  (annotation absent, write is idempotent).
- Upgrade of a cluster that never had `kmsKeyARN`: the new HCCO sees no annotation,
  sets it without writing `DriverConfig` (since `kmsKeyARN` is empty in the HCP
  spec), and commits that the initial pass is complete.

#### Bootstrap Ordering: CSO Conditional Dependency on HCCO

During cluster bootstrap, both the HCCO and the cluster-storage-operator (CSO) create
the `ClusterCSIDriver` resource in the guest cluster. The CSO starts faster because
its availability prober init container checks 1 CRD (`Storage`), while the HCCO's
checks 18 CRDs. Without coordination, the CSO creates `ClusterCSIDriver` with empty
`DriverConfig` before the HCCO writes the KMS key. The `aws-ebs-csi-driver-operator`
(deployed by the CSO) reads `ClusterCSIDriver` via an informer and creates the default
StorageClass. If it reads the empty version, the StorageClass is briefly created
without the KMS key.

The `aws-ebs-csi-driver-operator` watches `ClusterCSIDriver` and re-reconciles the
StorageClass when the HCCO updates it, so the StorageClass converges. However, the
brief window between first creation and update is architecturally undesirable.

When `kmsKeyARN` is set, the CPO uses the CPOv2 component dependency framework to
block the CSO from starting until the HCCO has completed its first successful
reconcile. This works as follows:

1. The HCCO component (`hosted-cluster-config-operator`) registers a custom operands
   rollout check that reads the existing `ConfigOperatorReconciliationSucceeded`
   condition on the `HostedControlPlane`. This condition is set to True by the HCCO
   after each successful reconcile (including `reconcileStorage`). The HCCO's
   `ControlPlaneComponent` CR reports `RolloutComplete=False` until this condition
   is True.
2. The CSO component (`cluster-storage-operator`) conditionally declares the HCCO
   as a dependency when `kmsKeyARN` is set in the HCP spec. The CPOv2 framework
   blocks the CSO's entire reconciliation (deployment creation, manifest
   application) until the HCCO's `ControlPlaneComponent` CR reports both
   `Available=True` and `RolloutComplete=True`.

The resulting bootstrap sequence when `kmsKeyARN` is set:

1. CVO installs the `ClusterCSIDriver` CRD in the guest cluster.
2. HCCO availability prober completes (all 18 CRDs present).
3. HCCO reconciles storage, creates `ClusterCSIDriver` with the KMS key and
   annotation, sets `ConfigOperatorReconciliationSucceeded=True` on the HCP.
4. CPO sees the HCCO's `ControlPlaneComponent` CR has `RolloutComplete=True`.
5. CPO creates the CSO deployment (dependency satisfied).
6. CSO starts, finds `ClusterCSIDriver` already exists with the KMS key (the CSO's
   `applyClusterCSIDriver` returns the existing object untouched).
7. CSO deploys the `aws-ebs-csi-driver-operator`.
8. The operator reads `ClusterCSIDriver` with the KMS key already present and creates
   the StorageClass correctly from the start.

When `kmsKeyARN` is not set, the CSO has no dependency on the HCCO and starts
independently. No behavior change for clusters without KMS encryption.

The conditional dependency adds approximately 30-60 seconds to the CSO's startup time
(the time for the HCCO to complete its first reconcile). This is acceptable: the
tradeoff is correctness over a small bootstrap delay for a single operator.

### Risks and Mitigations

#### Key Disabled After Volumes Encrypted

If the customer disables or deletes the KMS key in AWS after volumes are encrypted,
those volumes become inaccessible. HyperShift cannot prevent this.
*Mitigation:* The CSI driver will fail to attach/use the volume, surfacing the AWS
error during PVC provisioning. Document the key lifecycle responsibility.

#### IAM Role Misconfiguration

If the `StorageARN` role lacks the required KMS permissions on the key, PVC
provisioning fails at the CSI driver level with the AWS error.
*Mitigation:* The HCCO still writes the ARN to `ClusterCSIDriver`. IAM permission
errors surface during PVC provisioning. Document required permissions.

#### Existing Cluster Behavior on Upgrade

Clusters without `kmsKeyARN` must not experience any behavior change after upgrading
to a version containing this feature.
*Mitigation:* The field is optional with `omitempty`. On upgrade, the new HCCO's
first storage reconciliation sees no annotation on `ClusterCSIDriver`, sets it
without writing `DriverConfig` (since `kmsKeyARN` is empty), and never touches
`DriverConfig` again. Existing `ClusterCSIDriver` configuration is preserved.

#### Disrupting ClusterCSIDriver Editability

Cluster administrators and GitOps workflows may already rely on `ClusterCSIDriver`
being directly editable in the guest cluster. Continuously overwriting it from the
HC spec would be a disruptive change.
*Mitigation:* The set-once guard (see Implementation Details) ensures the HCCO
never touches `DriverConfig` after the annotation is set.

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

Continuously reconciling `ClusterCSIDriver.DriverConfig` from the HC spec (like
OAuth configuration) was considered. Rejected because `ClusterCSIDriver` is already
editable by cluster administrators in the guest cluster, and breaking that UX would
be disruptive. The ingress controller uses the same day-1-setup / day-2-admin-control
model.

#### Version History Guard

Using `hcp.Status.ControlPlaneVersion.History` to determine day-1 vs day-2 was
considered: if no `Completed` entry exists in the history, the cluster is in its
initial rollout and the HCCO writes `DriverConfig`. Rejected because the `Completed`
entry is set when all component deployments are rolled out at the target version, not
when the HCCO has finished its reconciliation. The HCCO deployment can be marked as
rolled out before the HCCO has run its first successful reconcile loop, creating a
window where `Completed` is set but `kmsKeyARN` has not been written. The annotation
approach avoids this timing gap by directly tracking whether the write has occurred.

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
- Immutability: updating `kmsKeyARN` to a different value is rejected
- Immutability: removing `kmsKeyARN` once set is rejected
- Immutability: removing `csiDriverConfig.aws` once `kmsKeyARN` is set is rejected
- Immutability: removing `csiDriverConfig` once set is rejected

### Unit Tests

Unit tests cover the propagation chain (HC controller mirroring, HCCO storage
reconciliation with set-once guard), and CLI flag wiring. Specific test cases:

- HC controller mirrors `operatorConfiguration.csiDriverConfig.aws` from HC to HCP
- HCCO writes `ClusterCSIDriver.DriverConfig.AWS.KMSKeyARN` when annotation is absent
  and `kmsKeyARN` is set in HCP spec
- HCCO sets annotation without writing `DriverConfig` when annotation is absent and
  `kmsKeyARN` is not set in HCP spec
- HCCO skips `DriverConfig` when annotation is present, even if admin cleared it
- HCCO retries on restart when annotation is absent (crash recovery)
- CLI: `--storage-volumes-kms-key` flag parsed and wired to HC spec
- CSO component includes HCCO as a dependency when `kmsKeyARN` is set in HCP spec
- CSO component omits HCCO dependency when `kmsKeyARN` is not set
- HCCO custom operands check returns false when
  `ConfigOperatorReconciliationSucceeded` is absent or False, true when True

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
   - Verify the `hypershift.openshift.io/storage-driver-config-applied` annotation
     is present on `ClusterCSIDriver`
   - Create a PVC using the default StorageClass, wait for it to bind
   - Verify the resulting EBS volume is encrypted with the specified KMS key via
     `ec2.DescribeVolumes` (following the existing `KMSRootVolumeTest` pattern)

2. **Set-once semantics:**
   - Verify that modifying `ClusterCSIDriver.spec.driverConfig` directly in the guest
     cluster persists across HCCO reconcile cycles (the HCCO does not revert it)

3. **Regression (no key configured):**
   - On a hosted cluster where `kmsKeyARN` was never set, verify PVCs use
     AWS-managed encryption
   - Verify the `hypershift.openshift.io/storage-driver-config-applied` annotation
     is still present on `ClusterCSIDriver`

## Graduation Criteria

### Dev Preview -> Tech Preview

This feature ships directly to GA. No Dev Preview or Tech Preview phase.

### Tech Preview -> GA

See above.

### GA

- Unit test coverage for all propagation paths.
- Envtest coverage for CEL validation across multiple Kubernetes versions.
- E2E test coverage for the full lifecycle (creation, set-once verification).
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
customers. On upgrade, the new HCCO's first storage reconciliation sees no annotation
on `ClusterCSIDriver`, sets the annotation without writing `DriverConfig` (since
`kmsKeyARN` is empty), and never touches `DriverConfig` again.

#### Failed Upgrade Rollback

Control plane downgrades are not supported in
HyperShift. If an N->N+1 upgrade fails mid-way, the `kmsKeyARN` field is not yet
active and has no effect on storage behavior. If `kmsKeyARN` was already configured
on a successfully upgraded cluster, the `ClusterCSIDriver` in the hosted cluster
retains its last-written `DriverConfig` throughout any subsequent upgrade attempts.

## Version Skew Strategy

The HC controller runs as part of the HyperShift operator on the management cluster.
Its binary comes from the HO image, which is installed independently of any guest
release. The HCCO and CSO binaries come from the guest cluster's release payload and
run in the per-cluster HCP namespace on the management cluster.

There is a version skew surface between the HO (which accepts and mirrors the
`kmsKeyARN` field) and the HCCO (which writes it to `ClusterCSIDriver`). On older
guest releases whose HCCO predates this feature, the field in the HCP spec is unused:
the older HCCO does not have the propagation code, so `ClusterCSIDriver` is not
configured with the key. The HO accepts the field regardless of guest version because
the `HostedCluster` CRD is managed by the HO.

The `ClusterCSIDriver` CRD in the guest cluster already includes
`spec.driverConfig.aws.kmsKeyARN` (since 4.17). For guest clusters older than 4.17,
the field on `ClusterCSIDriver` would not exist, but the HCCO from those releases
does not have the propagation code either, so no write is attempted.

## Operational Aspects of API Extensions

#### SLIs

The primary indicator is successful PVC provisioning with the configured KMS key.
If the key is invalid, PVC provisioning failures surface the issue.

#### Impact on Existing SLIs

No additional AWS API calls are introduced by this feature. The HCCO writes the
field once during the first storage reconciliation pass.

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
- Verify the `hypershift.openshift.io/storage-driver-config-applied` annotation
  is present on `ClusterCSIDriver` to confirm the HCCO completed its initial pass.

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
touch `ClusterCSIDriver.DriverConfig` after setting the annotation.

#### Graceful Failure

If the configured KMS key is invalid, control plane provisioning continues
normally. PVC provisioning may fail at the CSI driver level if the key is invalid
or permissions are insufficient. This is the same behavior as standalone OCP.

## Infrastructure Needed

No new subprojects, repositories, or testing infrastructure are required. The E2E
tests run in the existing HyperShift E2E CI jobs (`e2e-aws`, `e2e-aws-ovn`),
which already provision live AWS infrastructure with KMS access via the
`alias/hypershift-ci` key. The `StorageARN` role in the CI account must have
`kms:Decrypt`, `kms:GenerateDataKeyWithoutPlaintext`, and `kms:CreateGrant`
permissions added for this key.

Changes are required in `openshift/hypershift` only: API field, CLI flag, HCCO
set-once propagation. Proactive KMS key validation is planned as a separate
feature (OCPSTRAT-3501).
