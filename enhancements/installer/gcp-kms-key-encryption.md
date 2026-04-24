---
title: gcp-kms-key-encryption
authors:
  - "@barbacbd"
reviewers:
  - "@rochacbruno"
  - "@patrickdillon"
approvers:
  - "@patrickdillon"
  - "@sadasu"
api-approvers:
  - "@jspeed"
creation-date: 2026-04-15
last-updated: 2026-04-15
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-1671
see-also:
  - "/enhancements/installer/gcp-private-clusters.md"
  - "/enhancements/installer/storage-class-encrypted.md"
replaces: []
superseded-by: []
---

# Customer-Managed KMS Keys for GCP Storage Encryption

## Summary

This enhancement enables OpenShift users to specify customer-managed Cloud KMS keys for encrypting Google Cloud Storage (GCS) buckets used by the OpenShift installer and cluster infrastructure. Specifically, this applies to:
1. The bootstrap ignition GCS bucket (stores bootstrap machine ignition configs)
2. The internal image registry GCS bucket (stores container images)

By default, GCS buckets use Google-managed encryption keys. This enhancement allows customers with compliance, regulatory, or security requirements to bring their own KMS keys for data encryption at rest.

## Motivation

Many enterprise customers have strict security and compliance requirements that mandate the use of customer-managed encryption keys for all data stored in cloud storage services. While GCP provides encryption at rest by default using Google-managed keys, certain regulatory frameworks (HIPAA, PCI-DSS, FedRAMP, etc.) require customers to maintain control over encryption keys, including:

- Key rotation policies
- Access control and audit trails
- Key lifecycle management
- Geographic key residency requirements

Currently, the OpenShift installer does not provide a way to specify customer-managed KMS keys for GCS buckets, forcing customers to either:
1. Accept Google-managed encryption (non-compliant for some use cases)
2. Manually modify bucket encryption post-installation (complex, error-prone, may not cover bootstrap bucket)
3. Not use OpenShift on GCP (loss of platform choice)

### User Stories

#### Story 1: Financial Services Compliance

As a cloud architect at a financial institution, I need to deploy OpenShift clusters on GCP that comply with PCI-DSS requirements, which mandate customer-managed encryption keys for all data at rest, so that I can pass compliance audits and avoid regulatory penalties.

#### Story 2: Government Regulatory Requirements

As a platform engineer deploying OpenShift for a government agency, I need to use Cloud KMS keys with specific geographic residency (e.g., keys stored only in US regions) to meet FedRAMP and data sovereignty requirements, so that my cluster infrastructure meets federal compliance standards.

#### Story 3: Corporate Security Policy

As a security engineer at a large enterprise, I need to enforce a company-wide policy that all encryption keys are customer-managed with quarterly rotation, audit logging, and access controls tied to our IAM structure, so that I can maintain consistent security posture across all cloud deployments.

#### Story 4: Key Lifecycle Management

As an operations engineer, I want to use separate KMS keys for bootstrap data (temporary, high-privilege) and registry data (permanent, shared) so that I can implement different key rotation schedules and access policies appropriate to each bucket's lifecycle and security requirements.

### Goals

- Enable users to specify customer-managed Cloud KMS keys for the bootstrap ignition GCS bucket
- Enable users to specify customer-managed Cloud KMS keys for the image registry GCS bucket
- Support using a single KMS key for both buckets (simple use case)
- Support using separate KMS keys for each bucket (advanced security requirements)
- Maintain backward compatibility with existing clusters using Google-managed encryption
- Validate KMS key references at install-config validation time
- Document required IAM permissions for installer and cluster service accounts
- Provide clear error messages when KMS permissions are insufficient

### Non-Goals

- Encrypting persistent volumes (PVs) with customer-managed keys - this is handled separately via StorageClass encryption
- Encrypting etcd data with customer-managed keys - this is handled at the encryption-at-rest layer
- Automatic KMS key creation or rotation - users must pre-create and manage their own keys
- Cross-project KMS key support in the initial implementation
- Encrypting other GCP resources (compute disks already support KMS via machinepool configuration)
- Day-2 migration of existing buckets from Google-managed to customer-managed encryption
- Support for external KMS providers (AWS KMS, Azure Key Vault, etc.)

## Proposal

Add two optional fields to the GCP platform configuration in install-config.yaml that allow users to specify Cloud KMS keys for encrypting GCS buckets:

1. `ignitionStorageEncryptionKey` - KMS key for the bootstrap ignition bucket
2. `registryStorageEncryptionKey` - KMS key for the image registry bucket

Both fields are optional. When omitted, buckets are created with Google-managed encryption (current default behavior). When specified, the installer configures the buckets to use the provided customer-managed KMS keys.

The implementation will:
- Reuse existing `KMSKeyReference` type structure from machinepool disk encryption
- Validate KMS key references during install-config validation
- Configure bucket encryption during bootstrap ignition storage creation
- Pass registry KMS configuration to the cluster-image-registry-operator via Infrastructure CR
- Update IAM roles to include necessary KMS permissions

### Workflow Description

#### Installation Flow

**cluster administrator** is responsible for creating the Cloud KMS keys and deploying the cluster.

1. The cluster administrator creates one or more Cloud KMS keys in their GCP project:
   ```bash
   gcloud kms keyrings create openshift-keyring --location us-east1
   gcloud kms keys create ignition-key --keyring openshift-keyring --location us-east1 --purpose encryption
   gcloud kms keys create registry-key --keyring openshift-keyring --location us-east1 --purpose encryption
   ```

2. The cluster administrator grants the installer service account permissions to use the KMS keys:
   ```bash
   gcloud kms keys add-iam-policy-binding ignition-key \
     --keyring openshift-keyring \
     --location us-east1 \
     --member serviceAccount:installer@project.iam.gserviceaccount.com \
     --role roles/cloudkms.cryptoKeyEncrypterDecrypter
   ```

3. The cluster administrator creates an install-config.yaml with KMS key references:
   ```yaml
   apiVersion: v1
   baseDomain: example.com
   metadata:
     name: my-cluster
   platform:
     gcp:
       projectID: my-project
       region: us-east1
       ignitionStorageEncryptionKey:
         kmsKey:
           name: ignition-key
           keyRing: openshift-keyring
           location: us-east1
       registryStorageEncryptionKey:
         kmsKey:
           name: registry-key
           keyRing: openshift-keyring
           location: us-east1
   pullSecret: '{"auths": ...}'
   ```

4. The installer validates the install-config:
   - Validates KMS key reference format
   - Checks that the key ring and key exist (optional warning if inaccessible)
   - Validates that location matches a valid GCP region/location

5. The installer creates the bootstrap ignition GCS bucket with the specified KMS key:
   - Sets `BucketAttrs.Encryption.DefaultKMSKeyName` to the full KMS key resource path
   - Bucket is created with customer-managed encryption from the start

6. The installer generates cluster manifests including the Infrastructure CR with registry KMS configuration in `status.platformStatus.gcp`

7. Bootstrap completes and the cluster-image-registry-operator starts

8. The registry operator reads the KMS key configuration from the Infrastructure CR

9. The registry operator creates the registry GCS bucket with customer-managed encryption using the specified KMS key

10. The cluster is fully operational with all GCS buckets encrypted using customer-managed keys

#### Error Handling

**Insufficient KMS Permissions during Installation**

1. If the installer service account lacks KMS permissions, the GCS bucket creation will fail
2. The installer returns an error message: "Failed to create ignition storage bucket: Permission denied. Ensure the service account has roles/cloudkms.cryptoKeyEncrypterDecrypter on KMS key projects/{project}/locations/{location}/keyRings/{keyring}/cryptoKeys/{key}"
3. The administrator grants the missing permissions and retries the installation

**Registry Operator Cannot Access KMS Key**

1. If the master node service accounts lack KMS permissions, registry bucket creation fails
2. The cluster-image-registry-operator sets a degraded condition: "Failed to create registry storage: Permission denied for KMS key"
3. The administrator grants permissions to the master service account and the operator retries bucket creation

### API Extensions

This enhancement modifies the install-config API by adding new optional fields to the GCP platform configuration. These are not Kubernetes API extensions (CRDs, webhooks, etc.) but rather installer configuration schema changes.

#### Install Config Changes

Add to `pkg/types/gcp/platform.go`:

```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
    // ... existing fields ...
    
    // IgnitionStorageEncryptionKey defines the KMS key for encrypting the GCS bucket
    // used to store bootstrap ignition configs. When omitted, the bucket uses
    // Google-managed encryption.
    // +optional
    IgnitionStorageEncryptionKey *StorageEncryptionKeyReference `json:"ignitionStorageEncryptionKey,omitempty"`
    
    // RegistryStorageEncryptionKey defines the KMS key for encrypting the GCS bucket
    // used by the internal image registry. When omitted, the bucket uses
    // Google-managed encryption.
    // +optional
    RegistryStorageEncryptionKey *StorageEncryptionKeyReference `json:"registryStorageEncryptionKey,omitempty"`
}

// StorageEncryptionKeyReference describes the KMS key to use for GCS bucket encryption.
type StorageEncryptionKeyReference struct {
    // KMSKey is a reference to a Cloud KMS key to use for encryption.
    // +optional
    KMSKey *KMSKeyReference `json:"kmsKey,omitempty"`
}
```

This reuses the existing `KMSKeyReference` type already defined for disk encryption:

```go
// KMSKeyReference gathers required fields for looking up a GCP KMS Key
type KMSKeyReference struct {
    // Name is the name of the customer managed encryption key to be used for disk encryption.
    Name string `json:"name"`
    
    // KeyRing is the name of the KMS Key Ring which the KMS Key belongs to.
    KeyRing string `json:"keyRing"`
    
    // ProjectID is the ID of the Project in which the KMS Key Ring exists.
    // Defaults to the VM ProjectID if not set.
    // +optional
    ProjectID string `json:"projectID,omitempty"`
    
    // Location is the GCP location in which the Key Ring exists.
    Location string `json:"location"`
}
```

#### Infrastructure CR Changes (Upstream openshift/api)

Add to `config/v1/types_infrastructure.go` in the openshift/api repository:

```go
// GCPPlatformStatus holds the current status of the Google Cloud Platform infrastructure provider.
type GCPPlatformStatus struct {
    // ... existing fields ...
    
    // RegistryStorageKMSKey specifies the customer-managed KMS key for encrypting
    // the image registry GCS bucket. This is consumed by the cluster-image-registry-operator
    // when creating the registry storage bucket.
    // +optional
    RegistryStorageKMSKey *GCPKMSKeyReference `json:"registryStorageKMSKey,omitempty"`
}

// GCPKMSKeyReference describes a Cloud KMS key for GCS bucket encryption.
type GCPKMSKeyReference struct {
    // Name is the name of the customer managed encryption key.
    Name string `json:"name"`
    
    // KeyRing is the name of the KMS Key Ring which the KMS Key belongs to.
    KeyRing string `json:"keyRing"`
    
    // ProjectID is the ID of the Project in which the KMS Key Ring exists.
    ProjectID string `json:"projectID"`
    
    // Location is the GCP location in which the Key Ring exists.
    Location string `json:"location"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is not applicable to Hypershift deployments:
- Hypershift does not use the OpenShift installer's bootstrap process
- Hypershift control planes run in a management cluster, not on GCP infrastructure provisioned by the installer
- Image registry configuration in Hypershift follows a different model

If Hypershift deployments on GCP require customer-managed KMS for storage, that would be a separate enhancement to the Hypershift operator.

#### Standalone Clusters

This enhancement is fully applicable to standalone IPI (Installer Provisioned Infrastructure) clusters on GCP. It does not apply to UPI (User Provisioned Infrastructure) as users manage their own storage infrastructure.

#### Single-node Deployments or MicroShift

**Single-Node OpenShift (SNO):**
This enhancement applies to SNO deployments on GCP. The bootstrap ignition bucket is still created temporarily during installation, and if SNO clusters use the internal registry, the registry bucket would also use customer-managed encryption.

**MicroShift:**
MicroShift does not use the OpenShift installer and does not create GCS buckets for ignition or registry storage. This enhancement does not apply to MicroShift.

#### OpenShift Kubernetes Engine

OKE (OpenShift Kubernetes Engine) deployments that use the installer on GCP would benefit from this enhancement if they require customer-managed encryption for compliance. The feature works the same way as in full OCP deployments.

### Implementation Details/Notes/Constraints

#### Reusing Existing Patterns

GCP already has comprehensive KMS key support for machine pool OS disks via the `EncryptionKey` field in `pkg/types/gcp/machinepools.go`. This enhancement reuses:

1. **Type Structure**: The `KMSKeyReference` type with Name, KeyRing, ProjectID, and Location fields
2. **Validation Logic**: The `validatePlatformKMSKeys` function in `pkg/asset/installconfig/gcp/validation.go`
3. **Default Handling**: The pattern in `pkg/types/gcp/defaults/machinepool.go` for defaulting ProjectID

This provides consistency across the API and reduces implementation complexity.

#### Bootstrap Ignition Bucket Creation

The bootstrap ignition bucket is created in `pkg/asset/ignition/bootstrap/gcp/storage.go`. Currently, the `CreateStorage()` function creates a bucket with basic attributes:

```go
bucket := &storage.BucketAttrs{
    Location: location,
    Labels: labels,
    UniformBucketLevelAccess: storage.UniformBucketLevelAccess{
        Enabled: true,
    },
}
```

The enhancement will add encryption configuration when a KMS key is provided:

```go
if kmsKeyName != "" {
    bucket.Encryption = &storage.BucketEncryption{
        DefaultKMSKeyName: kmsKeyName,
    }
}
```

The KMS key name must be in the full resource format:
```
projects/{project}/locations/{location}/keyRings/{keyring}/cryptoKeys/{key}
```

#### Registry Bucket Creation

The image registry bucket is NOT created by the installer. It is created by the cluster-image-registry-operator after the cluster is up. The operator is part of the running cluster and creates the bucket based on the platform detected in the Infrastructure CR.

To enable KMS encryption for the registry bucket, the installer must:
1. Populate the `RegistryStorageKMSKey` field in the Infrastructure CR's `status.platformStatus.gcp` section
2. The field contains the full KMS key reference from the install config

The cluster-image-registry-operator will then:
1. Read the KMS key configuration from the Infrastructure CR (requires operator changes)
2. Create the registry GCS bucket with encryption configured
3. Set the bucket's `Encryption.DefaultKMSKeyName` to the provided KMS key

**Note:** This requires coordinated changes:
1. Enhancement to openshift/api to add the Infrastructure CR field (prerequisite)
2. Changes to the installer to populate the field (this enhancement)
3. Changes to cluster-image-registry-operator to consume the field (separate PR)

#### IAM Permissions Requirements

**Installer Service Account** needs:
```
roles/cloudkms.cryptoKeyEncrypterDecrypter  # On KMS keys
roles/storage.admin                          # To create and configure buckets
```

**Master Node Service Accounts** need:
```
roles/cloudkms.cryptoKeyDecrypter           # To decrypt ignition data
```

**Worker Node Service Accounts** need:
```
roles/cloudkms.cryptoKeyDecrypter           # To pull images from registry
```

The installer will need to update IAM role bindings in `pkg/infrastructure/gcp/clusterapi/iam.go` to include KMS permissions when KMS keys are configured.

#### Key Ring Validation

During install-config validation, the installer should:
1. **Format validation** (always): Ensure key ring and key names follow GCP naming conventions
2. **Existence check** (best effort): Call GCP API to verify the key ring and key exist
   - If the call fails due to permissions, log a warning but don't fail validation
   - This allows users with restrictive permissions to proceed, with errors caught later during bucket creation
3. **Location validation** (always): Ensure the location is a valid GCP region or multi-region (e.g., "us-east1", "us", "eu", "asia")

### Risks and Mitigations

#### Risk: KMS Key Deletion Causing Data Loss

**Risk**: If a customer deletes the KMS key used to encrypt the registry bucket, all data in that bucket becomes permanently inaccessible, effectively destroying all container images.

**Mitigation**:
- Document the criticality of KMS key lifecycle management
- Recommend using Cloud KMS key deletion protection (scheduled deletion with 24-hour minimum delay)
- Recommend regular backups of container images
- Consider adding a warning in the install-config validation about key deletion impact

#### Risk: Insufficient IAM Permissions

**Risk**: If the installer or node service accounts lack KMS permissions, installation will fail or the cluster will be degraded.

**Mitigation**:
- Provide clear documentation of required IAM roles
- Include detailed error messages that specify the exact permission missing
- Add a pre-flight permission check (optional) that validates KMS access before creating buckets
- Document the symptoms and resolution steps in troubleshooting guides

#### Risk: Performance Impact

**Risk**: Using customer-managed KMS keys may introduce latency for GCS operations (encrypt/decrypt operations on every read/write).

**Mitigation**:
- Document expected performance characteristics
- GCS encryption operations are handled by Google's infrastructure and typically add minimal latency (< 10ms)
- For most use cases, the performance impact is negligible compared to network latency
- Customers with extreme performance requirements can opt to use Google-managed encryption

#### Risk: Cross-Region Key Access

**Risk**: If KMS keys are in a different region than the GCS buckets, cross-region key access may increase latency or fail due to regional restrictions.

**Mitigation**:
- Recommend co-locating KMS keys in the same region as the cluster
- Validation should warn if key location differs from cluster region
- Document multi-region key considerations

#### Risk: Bootstrap Bucket Cleanup

**Risk**: The bootstrap ignition bucket is automatically deleted after cluster creation. If there are issues during bootstrap, the encrypted data may be needed for debugging but is destroyed.

**Mitigation**:
- The installer already preserves the bootstrap bucket on installation failure
- Document the importance of preserving bootstrap data for troubleshooting
- Consider adding an option to preserve the bootstrap bucket even on successful installation

### Drawbacks

1. **Increased Complexity**: Users must now manage KMS keys in addition to other infrastructure, adding operational overhead.

2. **Cost**: Customer-managed KMS keys have per-key monthly fees (~$1/key/month) plus per-operation costs. For large clusters with high registry usage, this could add non-trivial costs.

3. **Separate Operator Changes**: Full support requires coordinated changes to cluster-image-registry-operator, creating implementation dependencies across repositories.

4. **Limited to GCP**: This is a platform-specific feature that doesn't apply to other clouds, though similar features could be added for AWS (S3 KMS) and Azure (Storage Account encryption).

5. **No Day-2 Migration**: Existing clusters cannot easily migrate from Google-managed to customer-managed encryption. Users must plan encryption strategy at install time.

## Alternatives (Not Implemented)

### Alternative 1: Single KMS Key Field

Instead of separate fields for ignition and registry, use a single `storageEncryptionKMSKey` field that applies to all GCS buckets.

**Pros**:
- Simpler API
- Easier for users to configure
- Less configuration

**Cons**:
- Less flexible - cannot implement different key rotation policies for temporary vs permanent storage
- Doesn't align with security best practices of separating keys by purpose
- Cannot accommodate users who only want to encrypt one bucket type

**Rejected because**: Security best practices recommend separate keys for different purposes, especially when data has different lifecycles (bootstrap is temporary, registry is permanent).

### Alternative 2: Automatic KMS Key Creation

Have the installer automatically create KMS keys if not specified.

**Pros**:
- Easier user experience
- No pre-installation setup required

**Cons**:
- Requires additional installer permissions (kms.keyRings.create, kms.cryptoKeys.create)
- Doesn't support users' existing key management workflows
- Adds complexity to installer code
- Users may not want the installer creating persistent keys
- Inconsistent with other cloud platforms where users bring their own keys

**Rejected because**: Key management is a sensitive security area. Customers with compliance requirements typically have existing key management processes and don't want automated key creation.

### Alternative 3: ConfigMap for Registry Configuration

Instead of using the Infrastructure CR, create a ConfigMap in the openshift-image-registry namespace with KMS configuration.

**Pros**:
- No changes needed to openshift/api
- Simpler to implement in installer

**Cons**:
- ConfigMaps are not the standard way to pass platform configuration
- Inconsistent with how other platform settings are communicated
- Infrastructure CR is the canonical source of platform configuration
- Less discoverable for operators

**Rejected because**: Infrastructure CR is the established pattern for communicating platform configuration to in-cluster operators.

### Alternative 4: Post-Installation Manual Configuration

Document a process for users to manually configure KMS encryption after installation.

**Pros**:
- No installer changes needed
- Maximum flexibility

**Cons**:
- Complex and error-prone manual process
- Cannot encrypt the bootstrap ignition bucket (deleted before user can modify)
- Poor user experience
- Doesn't meet compliance requirements (some data was temporarily stored without customer-managed encryption)

**Rejected because**: This defeats the purpose of the feature and doesn't meet compliance requirements.

## Open Questions

1. **Should we support cross-project KMS keys in the initial implementation?**
   - This would allow using keys from a central "security" project
   - Requires additional IAM configuration and validation complexity
   - Can be deferred to a future enhancement

2. **Should we validate KMS key permissions during install-config validation?**
   - Pros: Fail fast with clear errors
   - Cons: Requires installer service account to have kms.cryptoKeys.getIamPolicy permission
   - Proposed: Make this an optional warning, not a hard failure

3. **Should we support KMS key versioning/rotation?**
   - GCS automatically uses the latest key version
   - No special handling needed in installer
   - Document that key rotation is handled by GCP automatically

4. **How should we handle the cluster-image-registry-operator changes?**
   - Option A: Hold this enhancement until operator PR is merged
   - Option B: Merge installer changes first, operator support comes in a follow-up release
   - Proposed: Coordinate with operator team, merge in the same release cycle

## Test Plan

### Unit Tests

1. **Type validation tests** (`pkg/types/gcp/validation/platform_test.go`):
   - Valid KMS key references are accepted
   - Invalid key ring names are rejected
   - Invalid key names are rejected
   - Invalid locations are rejected
   - Missing required fields are rejected

2. **Storage creation tests** (`pkg/asset/ignition/bootstrap/gcp/storage_test.go`):
   - Bucket creation with KMS key sets encryption correctly
   - Bucket creation without KMS key uses Google-managed encryption
   - KMS key reference is formatted correctly as full resource path

3. **Infrastructure manifest tests** (`pkg/asset/manifests/infrastructure_test.go`):
   - Registry KMS key is correctly populated in Infrastructure CR
   - ProjectID defaults are applied correctly

### Integration Tests

1. **Install-config validation integration test**:
   - Create install-config with valid KMS key references
   - Validation passes without errors
   - Create install-config with malformed KMS references
   - Validation fails with appropriate error messages

2. **Ignition bucket creation integration test**:
   - Run installer with ignition KMS key configured
   - Verify bucket is created with correct encryption settings
   - Verify GCS API shows customer-managed encryption enabled

3. **End-to-end installation test** (requires GCP credentials):
   - Pre-create KMS keys and grant permissions
   - Run full cluster installation with KMS keys configured
   - Verify bootstrap bucket is encrypted with customer key
   - Verify registry bucket is encrypted with customer key (once operator support is added)
   - Verify cluster is functional
   - Verify images can be pushed and pulled from registry

4. **IAM permission failure test**:
   - Run installation with insufficient KMS permissions
   - Verify installation fails with clear error message indicating missing permission

### Manual Testing

1. Test with different key ring locations (regional vs multi-regional)
2. Test with separate keys for ignition and registry
3. Test with single key for both buckets
4. Test backward compatibility (no KMS keys specified)
5. Test with cross-project KMS keys (if supported)

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end for ignition bucket encryption
- End user documentation available
- Sufficient unit and integration test coverage
- Gather feedback from early adopters
- Known limitations documented (e.g., registry support pending operator changes)
- Validation works correctly for all key reference formats
- IAM permission requirements clearly documented

### Tech Preview -> GA

- Registry operator support is implemented and tested
- End-to-end testing across multiple GCP regions
- Sufficient time for feedback from Tech Preview users
- Performance impact documented and acceptable
- User-facing documentation in openshift-docs
- CI testing includes KMS encryption scenarios
- Support runbooks created for common failure scenarios
- Load testing with registry under customer-managed encryption
- Validation that key rotation works seamlessly

**For GA, the following must be demonstrated:**
- End-to-end tests with both ignition and registry encryption
- No significant performance degradation
- Clear upgrade path for future enhancements (e.g., cross-project keys)
- Compatibility with other GCP features (private clusters, shared VPC, etc.)

### Removing a deprecated feature

Not applicable - this is a new feature.

## Upgrade / Downgrade Strategy

### Upgrade

**Upgrading TO a release with this feature:**
- Existing clusters continue to use Google-managed encryption (no change)
- New installations can opt-in to customer-managed KMS encryption
- No changes required to existing install-configs
- No impact on running clusters

**Upgrading FROM a cluster using this feature:**
- KMS-encrypted buckets remain encrypted with the same keys
- No special handling needed during upgrade
- The registry bucket continues to use customer-managed encryption
- Upgrade process does not modify bucket encryption settings

### Downgrade

**Downgrading TO a release without this feature:**
- Cluster continues to function normally
- Buckets remain encrypted with customer-managed keys
- Install-config fields are ignored by older installer version
- No data loss or functionality loss

**Risk**: If a downgrade is attempted and the cluster needs to recreate the bootstrap bucket (unlikely scenario), the older installer will create it with Google-managed encryption. This is acceptable as the bootstrap bucket is temporary.

## Version Skew Strategy

This enhancement has minimal version skew concerns:

1. **Installer vs Cluster**: The installer runs once during installation. Version skew between installer and running cluster does not apply.

2. **Registry Operator vs Infrastructure CR**: The registry operator must understand the new `RegistryStorageKMSKey` field. This is handled by:
   - Adding the field to openshift/api first
   - Registry operator changes to consume the field
   - Coordinating releases so both are available together

3. **Backward Compatibility**: Older registry operators ignore unknown fields in Infrastructure CR, so they will simply not use customer-managed encryption if the field is present. This degrades gracefully.

4. **Forward Compatibility**: Newer registry operators check for the field and use it if present, otherwise fall back to Google-managed encryption.

## Operational Aspects of API Extensions

This enhancement does not add any API extensions (CRDs, webhooks, aggregated API servers, or finalizers). It only modifies:
1. Installer install-config schema (input configuration)
2. Infrastructure CR status field (existing CR, new field)

### Infrastructure CR Impact

**Field Addition**: `status.platformStatus.gcp.registryStorageKMSKey`

**Impact on existing systems**:
- No impact on API throughput or availability
- No validation webhooks or admission controllers involved
- Field is optional and ignored by operators that don't support it
- No finalizers or blocking operations

**Failure Modes**:
- If field is populated but registry operator doesn't support it: Registry bucket created with Google-managed encryption (degraded functionality but no failure)
- If field contains invalid KMS reference: Registry operator degrades with condition indicating KMS key not found

## Support Procedures

### Detecting Failures

**Symptom**: Installer fails during bootstrap with "Permission denied" error

**Detection**:
- Error message: "Failed to create storage bucket: Permission denied"
- Logs show: "Error applying encryption configuration to bucket"

**Resolution**:
1. Check that KMS key exists:
   ```bash
   gcloud kms keys describe KEY_NAME --keyring=KEY_RING --location=LOCATION
   ```
2. Verify installer service account has permissions:
   ```bash
   gcloud kms keys get-iam-policy KEY_NAME --keyring=KEY_RING --location=LOCATION
   ```
3. Grant missing permissions:
   ```bash
   gcloud kms keys add-iam-policy-binding KEY_NAME \
     --keyring=KEY_RING --location=LOCATION \
     --member=serviceAccount:INSTALLER_SA@PROJECT.iam.gserviceaccount.com \
     --role=roles/cloudkms.cryptoKeyEncrypterDecrypter
   ```

**Symptom**: Registry operator degraded after cluster installation

**Detection**:
- `oc get clusteroperator image-registry` shows Degraded=True
- Operator logs show: "Failed to create GCS bucket: Permission denied"
- Condition message: "Unable to access KMS key for registry storage encryption"

**Resolution**:
1. Check master service account permissions on KMS key
2. Grant cloudkms.cryptoKeyEncrypterDecrypter role to master service accounts
3. Operator will retry bucket creation automatically

**Symptom**: Cannot pull images from registry after KMS key deletion

**Detection**:
- Image pulls fail with "Access Denied" errors
- Registry pods log: "Error reading from GCS: KMS key not found"
- GCS bucket is inaccessible

**Resolution**:
- If key is in scheduled deletion: Restore the key before deletion completes
- If key is permanently deleted: Data is unrecoverable, registry bucket must be recreated (data loss)

### Disabling the Feature

To disable customer-managed encryption for new installations:
1. Remove `ignitionStorageEncryptionKey` and `registryStorageEncryptionKey` from install-config.yaml
2. New buckets will use Google-managed encryption

For existing clusters:
- Cannot easily migrate from customer-managed to Google-managed encryption
- Bucket encryption configuration is immutable after creation
- Would require creating new buckets and migrating data

### Graceful Degradation

If customer-managed encryption fails:
- Bootstrap bucket creation will fail, blocking installation (expected behavior)
- Registry bucket creation will fail, but cluster remains functional without registry
- Operator will retry creation periodically
- Clear error messages guide users to resolution

## Infrastructure Needed

### Testing Infrastructure

1. **GCP Test Project**: A dedicated GCP project for CI testing with:
   - Pre-created KMS key rings and keys
   - Service accounts with appropriate KMS permissions
   - Quota for creating test clusters

2. **CI Pipeline Updates**: Modify existing GCP CI jobs to:
   - Create test KMS keys before running installer
   - Test installations with and without customer-managed encryption
   - Clean up KMS keys after test runs (or use scheduled deletion)

3. **Documentation Repository**: Space in openshift-docs for:
   - Install-config examples
   - IAM permission setup guides
   - Troubleshooting guides

### Development Infrastructure

1. **Development GCP Projects**: Engineers need access to GCP projects where they can:
   - Create and manage KMS keys
   - Deploy test clusters
   - Test IAM permission scenarios

2. **Code Review Collaboration**: Coordination with:
   - cluster-image-registry-operator maintainers
   - openshift/api maintainers
   - GCP platform team

No new GitHub repositories or subprojects are needed.
