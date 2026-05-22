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
  - "@JoelSpeed"
creation-date: 2026-04-15
last-updated: 2026-05-21
status: implementable
tracking-link:
  - https://redhat.atlassian.net/browse/CORS-4391
see-also:
  - "/enhancements/installer/gcp-private-clusters.md"
  - "/enhancements/installer/storage-class-encrypted.md"
replaces: []
superseded-by: []
---

# Customer-Managed KMS Keys for GCP Storage Encryption

## Summary

This enhancement enables OpenShift users to specify a customer-managed Cloud KMS key for encrypting **all cluster storage** on GCP, including:
1. Machine OS disks (control plane and compute nodes) - existing functionality
2. The bootstrap ignition GCS bucket (stores bootstrap machine ignition configs) - **new**
3. The internal image registry GCS bucket (stores container images) - **new**

By default, OS disks and GCS buckets use Google-managed encryption keys. This enhancement extends the existing `platform.gcp.defaultMachinePlatform.osDisk.encryptionKey.kmsKey` field to also apply to storage buckets, allowing customers with compliance, regulatory, or security requirements to use a single customer-managed KMS key for all cluster data encryption at rest.

**Implementation Approach:** This enhancement reuses the existing KMS key configuration from machine pool OS disk encryption rather than adding new install-config fields. This provides a simpler user experience (configure once, applies everywhere) and ensures consistent encryption across all cluster resources. The installer automatically grants required IAM permissions to Google-managed service accounts and cluster service accounts during installation.

## Key Findings (Updated 2026-05-21)

During analysis and design of this enhancement, several important discoveries were made that significantly simplify the implementation:

1. **Registry Operator Already Supports KMS Encryption**: The cluster-image-registry-operator has had full customer-managed KMS encryption support since 2019 (commit `8c08cc6805`). The `spec.storage.gcs.keyID` field in the ImageRegistry CR already exists and is fully implemented.

2. **No New Kubernetes API Changes Needed**: The ImageRegistry CR (`configs.imageregistry.operator.openshift.io/cluster`) already has the necessary field. We do not need to add fields to the Infrastructure CR or create new CRDs.

3. **No Feature Gates Required**: This is install-time configuration (not runtime cluster features), so feature gates are not appropriate. Install-config fields are naturally opt-in (omit = default behavior).

4. **Simplified IAM Permissions**: Based on how GCP KMS works with Cloud Storage, we only need to grant KMS decrypt permissions to specific service accounts (installer, bootstrap nodes, master nodes), not all worker nodes.

5. **Can Ship as GA**: Given the registry operator support has been production-proven for 7 years, this enhancement can ship directly as GA without a Tech Preview phase.

6. **Single-Key Approach Adopted** (2026-05-21): After analysis, the implementation uses a single KMS key for all cluster storage (OS disks, ignition bucket, registry bucket) rather than separate fields. This provides:
   - Simpler user experience (configure once)
   - Consistent security posture across all cluster resources
   - No new install-config fields (reuses existing `defaultMachinePlatform.osDisk.encryptionKey.kmsKey`)
   - Lower operational overhead (one key to manage)

7. **Automatic IAM Permission Grants**: The installer automatically grants required KMS permissions to Google-managed service accounts (Cloud Storage, Compute Engine) and the master node service account during the PreProvision phase. Users only need to grant the installer service account `roles/cloudkms.admin` on the KMS key.

**Implementation Scope**: This enhancement is now installer-only changes (no cross-repository coordination needed). The work involves:
- Reusing existing install-config fields (defaultMachinePlatform.osDisk.encryptionKey.kmsKey)
- Implementing validation
- Configuring bootstrap bucket encryption
- Populating the existing ImageRegistry CR field
- Automatically granting IAM permissions to Google-managed service accounts and master nodes

## Implementation Status (Updated 2026-05-21)

The implementation is complete on branch `CORS-4391-spike` and ready for review:

**Completed:**
- ✅ Bootstrap ignition bucket encryption (`pkg/asset/ignition/bootstrap/gcp/storage.go`)
  - Configures bucket with customer-managed KMS key during creation
  - Helper function `BuildKMSKeyName()` to format KMS key resource path
- ✅ ImageRegistry CR population (`pkg/asset/manifests/gcp/imageregistry.go`)
  - Generates ImageRegistry manifest when KMS key is configured
  - Populates `spec.storage.gcs.keyID` with KMS key resource path
- ✅ Automatic IAM permission grants (`pkg/infrastructure/gcp/clusterapi/iam.go`)
  - Grants Cloud Storage service account `cryptoKeyEncrypterDecrypter` role
  - Grants Compute Engine service account `cryptoKeyEncrypterDecrypter` role
  - Grants master node service account `cryptoKeyEncrypterDecrypter` role
  - All grants happen automatically during PreProvision phase
- ✅ Helper function in platform types (`pkg/types/gcp/platform.go`)
  - `GetStorageKMSKey()` extracts KMS key from defaultMachinePlatform for storage encryption
- ✅ KMS key validation (`pkg/asset/installconfig/gcp/validation.go`)
  - Validates KMS key reference format
  - Checks required fields (name, keyRing, location)
  - Validates against GCP naming conventions
- ✅ Comprehensive unit tests (see UNIT_TEST_SUMMARY.md)
  - 26 unit tests across 4 test files
  - All tests passing
  - Coverage for validation, bootstrap storage, ImageRegistry manifest generation
- ✅ Documentation (`docs/user/gcp/`)
  - IAM permissions requirements documented in `iam.md`
  - Install-config configuration documented in `customization.md`
  - Deprecated `kmsKeyServiceAccount` field (automatic grants make it unnecessary)

**Pending:**
- Integration tests with actual GCP cluster installation
- End-to-end testing with KMS-encrypted buckets
- User-facing documentation updates (install examples, troubleshooting)

**Files Changed:**
```
pkg/asset/ignition/bootstrap/gcp/storage.go          # Bootstrap bucket encryption
pkg/asset/ignition/bootstrap/gcp/storage_test.go     # Bootstrap tests
pkg/asset/manifests/gcp/imageregistry.go             # Registry CR generation
pkg/asset/manifests/gcp/imageregistry_test.go        # Registry tests
pkg/asset/manifests/openshift.go                     # Hook ImageRegistry into manifests
pkg/infrastructure/gcp/clusterapi/iam.go             # Automatic IAM grants
pkg/types/gcp/platform.go                            # GetStorageKMSKey() helper
pkg/types/gcp/machinepools.go                        # StorageEncryptionKeyReference type (unused)
pkg/asset/installconfig/gcp/validation.go            # Enhanced validation
docs/user/gcp/iam.md                                 # IAM documentation
docs/user/gcp/customization.md                       # Config documentation
```

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

#### Story 4: Consistent Encryption Across All Cluster Resources

As an operations engineer, I want to use a single customer-managed KMS key for all cluster storage (OS disks, bootstrap data, and registry data) so that I can implement consistent encryption policy, simplify key management, and ensure uniform security posture across all cluster infrastructure.

### Goals

- Enable users to specify a customer-managed Cloud KMS key that applies to all cluster storage:
  - Machine OS disks (existing functionality)
  - Bootstrap ignition GCS bucket (new)
  - Image registry GCS bucket (new)
- Automatically grant required IAM permissions to Google-managed service accounts and cluster service accounts
- Maintain backward compatibility with existing clusters using Google-managed encryption
- Validate KMS key references at install-config validation time
- Provide clear error messages when KMS permissions are insufficient
- Simplify user experience by reusing existing install-config field
- Document required IAM permissions for the installer service account

### Non-Goals

- Encrypting persistent volumes (PVs) with customer-managed keys - this is handled separately via StorageClass encryption
- Encrypting etcd data with customer-managed keys - this is handled at the encryption-at-rest layer
- Automatic KMS key creation or rotation - users must pre-create and manage their own keys
- Cross-project KMS key support in the initial implementation
- Encrypting other GCP resources (compute disks already support KMS via machinepool configuration)
- Day-2 migration of existing buckets from Google-managed to customer-managed encryption
- Support for external KMS providers (AWS KMS, Azure Key Vault, etc.)

## Proposal

**Implementation Approach (Adopted):**

Reuse the existing `platform.gcp.defaultMachinePlatform.osDisk.encryptionKey.kmsKey` field to enable customer-managed KMS encryption for **three purposes**:

1. OS disk encryption (existing functionality)
2. Bootstrap ignition bucket encryption (new)
3. Image registry storage bucket encryption (new)

This is a **single KMS key approach** where one customer-managed key is used for all cluster storage encryption needs. The field is optional - when omitted, resources use Google-managed encryption (current default behavior).

**Rationale for This Approach:**

- **Simplicity**: Users specify the KMS key once, and it applies consistently across all cluster storage
- **Security best practice**: Using a single customer-managed key is better than mixing customer-managed (OS disks) with Google-managed (storage buckets)
- **Existing API**: No new install-config fields needed - reuses existing, well-understood configuration
- **Consistency**: Aligns with how other platforms handle encryption keys (AWS, Azure use similar single-key approaches)
- **Minimal risk**: Same lifecycle and access patterns for all three uses (cluster infrastructure)

The implementation:
- Reuses existing `KMSKeyReference` type structure from machinepool disk encryption
- Validates KMS key references during install-config validation
- Configures bootstrap bucket encryption during ignition storage creation
- Passes registry KMS configuration to the cluster-image-registry-operator via ImageRegistry CR
- Automatically grants IAM permissions to Google-managed service accounts and master nodes

**Note:** The cluster-image-registry-operator has supported customer-managed KMS encryption for GCS buckets since 2019 (via the `spec.storage.gcs.keyID` field in the ImageRegistry CR). This enhancement makes that existing capability accessible through the installer.

**Alternative Considered:** Separate fields for ignition and registry (`platform.gcp.ignition.storage.encryptionKey` and `platform.gcp.registry.storage.encryptionKey`) were considered but rejected in favor of simplicity. See "Alternatives" section for details.

### Workflow Description

#### Installation Flow

**cluster administrator** is responsible for creating the Cloud KMS keys and deploying the cluster.

1. The cluster administrator creates a Cloud KMS key in their GCP project:
   ```bash
   gcloud kms keyrings create openshift-keyring --location us-east1
   gcloud kms keys create cluster-encryption-key --keyring openshift-keyring --location us-east1 --purpose encryption
   ```

2. The cluster administrator grants the installer service account IAM policy management permissions:
   ```bash
   # The installer needs permission to manage IAM policies on the KMS key
   # so it can automatically grant Google-managed service accounts access
   gcloud kms keys add-iam-policy-binding cluster-encryption-key \
     --keyring openshift-keyring \
     --location us-east1 \
     --member serviceAccount:installer@project.iam.gserviceaccount.com \
     --role roles/cloudkms.admin
   ```

3. The cluster administrator creates an install-config.yaml with the KMS key reference:
   ```yaml
   apiVersion: v1
   baseDomain: example.com
   metadata:
     name: my-cluster
   platform:
     gcp:
       projectID: my-project
       region: us-east1
       defaultMachinePlatform:
         osDisk:
           encryptionKey:
             kmsKey:
               name: cluster-encryption-key
               keyRing: openshift-keyring
               location: us-east1
   pullSecret: '{"auths": ...}'
   ```
   
   **Note:** This single KMS key will be used for encrypting:
   - All cluster machine OS disks (control plane and compute nodes)
   - The bootstrap ignition storage bucket
   - The image registry storage bucket

4. The installer validates the install-config:
   - Validates KMS key reference format
   - Checks that the key ring and key exist (optional warning if inaccessible)
   - Validates that location matches a valid GCP region/location

5. The installer automatically grants IAM permissions during the PreProvision phase:
   - Grants Cloud Storage service account `roles/cloudkms.cryptoKeyEncrypterDecrypter` on the KMS key
   - Grants Compute Engine service account `roles/cloudkms.cryptoKeyEncrypterDecrypter` on the KMS key
   - Grants master node service account `roles/cloudkms.cryptoKeyEncrypterDecrypter` on the KMS key

6. The installer creates the bootstrap ignition GCS bucket with the specified KMS key:
   - Sets `BucketAttrs.Encryption.DefaultKMSKeyName` to the full KMS key resource path
   - Bucket is created with customer-managed encryption from the start

7. The installer generates cluster manifests including the ImageRegistry CR (`configs.imageregistry.operator.openshift.io/cluster`) with `spec.storage.gcs.keyID` set to the KMS key

8. Bootstrap completes and the cluster-image-registry-operator starts

9. The registry operator reads the KMS key configuration from the ImageRegistry CR `spec.storage.gcs.keyID` field (this field has existed since 2019)

10. The registry operator creates the registry GCS bucket with customer-managed encryption using the specified KMS key

11. The cluster is fully operational with all storage (OS disks and GCS buckets) encrypted using the customer-managed key

#### Error Handling

**Insufficient IAM Policy Management Permissions**

1. If the installer service account lacks `cloudkms.cryptoKeys.setIamPolicy` permission, automatic IAM grants fail
2. The installer returns an error message: "Failed to grant KMS permissions: Permission denied. Ensure the installer service account has roles/cloudkms.admin on KMS key projects/{project}/locations/{location}/keyRings/{keyring}/cryptoKeys/{key}"
3. The administrator grants `roles/cloudkms.admin` (or a custom role with `setIamPolicy` permission) to the installer service account and retries

**KMS Key Does Not Exist**

1. If the referenced KMS key doesn't exist, validation may warn but installation will fail during bucket creation
2. The installer returns: "KMS key not found: projects/{project}/locations/{location}/keyRings/{keyring}/cryptoKeys/{key}"
3. The administrator creates the KMS key and retries the installation

**Registry Operator Cannot Access KMS Key** (Unlikely with automatic grants)

1. If automatic IAM grants failed but installation proceeded, registry bucket creation may fail
2. The cluster-image-registry-operator sets a degraded `StorageEncrypted` condition with reason "InvalidStorageConfiguration": "Permission denied for KMS key"
3. The administrator manually grants `roles/cloudkms.cryptoKeyEncrypterDecrypter` to the master service account (`{INFRA_ID}-m@{PROJECT_ID}.iam.gserviceaccount.com`) and the operator retries automatically

### API Extensions

This enhancement **does not add any new install-config fields**. It reuses the existing `platform.gcp.defaultMachinePlatform.osDisk.encryptionKey.kmsKey` field and extends its purpose to cover storage bucket encryption in addition to OS disk encryption.

#### Install Config - Existing Field Reused

The existing field in `pkg/types/gcp/machinepools.go`:

```go
// MachinePool stores the configuration for a machine pool installed on GCP.
type MachinePool struct {
    // ... other fields ...
    
    // OSDisk defines the storage for instance.
    // +optional
    OSDisk OSDisk `json:"osDisk"`
}

// OSDisk defines the disk for machines on GCP.
type OSDisk struct {
    // DiskType defines the type of disk.
    // The valid values are pd-standard, pd-ssd, pd-balanced, hyperdisk-balanced, and hyperdisk-throughput.
    // For control plane nodes, the default is pd-ssd. For compute nodes,
    // the default is pd-standard.
    // +optional
    DiskType string `json:"diskType,omitempty"`
    
    // DiskSizeGB defines the size of disk in GB.
    // The minimum is 16 for pd-standard and pd-balanced, 32 for hyperdisk-balanced,
    // and 500 for hyperdisk-throughput. The default is 128 for all types.
    // +kubebuilder:validation:Minimum=16
    // +optional
    DiskSizeGB int64 `json:"diskSizeGB,omitempty"`
    
    // EncryptionKey defines the KMS key to be used to encrypt the disk.
    // +optional
    EncryptionKey *DiskEncryptionKey `json:"encryptionKey,omitempty"`  // <-- EXISTING FIELD, NOW USED FOR STORAGE TOO
}

// DiskEncryptionKey defines the KMS key to be used to encrypt the disk.
type DiskEncryptionKey struct {
    // KMSKey is a reference to a KMS Key to use for the encryption.
    // +optional
    KMSKey *KMSKeyReference `json:"kmsKey,omitempty"`
    
    // KMSKeyServiceAccount is the service account being used for the
    // encryption request for the given KMS key. If absent, the Compute Engine
    // default service account is used.
    // DEPRECATED: This field is no longer needed. The installer automatically
    // grants the Compute Engine service account permission to use the KMS key.
    // +optional
    KMSKeyServiceAccount string `json:"kmsKeyServiceAccount,omitempty"`
}

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

#### New Helper Function Added

To support retrieving the KMS key for storage encryption, a new helper function was added to `pkg/types/gcp/platform.go`:

```go
// GetStorageKMSKey returns the KMS key to use for GCS bucket encryption.
// Returns the key from defaultMachinePlatform.osDisk.encryptionKey.kmsKey if configured,
// otherwise returns nil.
func GetStorageKMSKey(platform *Platform) *KMSKeyReference {
    if platform != nil &&
        platform.DefaultMachinePlatform != nil &&
        platform.DefaultMachinePlatform.OSDisk.EncryptionKey != nil &&
        platform.DefaultMachinePlatform.OSDisk.EncryptionKey.KMSKey != nil {
        return platform.DefaultMachinePlatform.OSDisk.EncryptionKey.KMSKey
    }
    return nil
}
```

#### ImageRegistry CR Changes (Existing API)

**No new API changes required.** The ImageRegistry CR (`configs.imageregistry.operator.openshift.io/cluster`) already has the `keyID` field in `spec.storage.gcs` since 2019:

```go
// From openshift/api/imageregistry/v1/types.go (existing code since 2019)
type ImageRegistryConfigStorageGCS struct {
    // bucket is the bucket name in which you want to store the registry's data.
    // Optional, will be generated if not provided.
    // +optional
    Bucket string `json:"bucket,omitempty"`
    
    // region is the GCS location in which your bucket exists.
    // Optional, will be set based on the installed GCS Region.
    // +optional
    Region string `json:"region,omitempty"`
    
    // projectID is the Project ID of the GCP project that this bucket should
    // be associated with.
    // +optional
    ProjectID string `json:"projectID,omitempty"`
    
    // keyID is the KMS key ID to use for encryption.
    // Optional, buckets are encrypted by default on GCP.
    // This allows for the use of a custom encryption key.
    // +optional
    KeyID string `json:"keyID,omitempty"`  // <-- EXISTING FIELD SINCE 2019
}
```

The installer will populate `spec.storage.gcs.keyID` in the ImageRegistry CR manifest, and the cluster-image-registry-operator will consume it as it already does today.

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

The image registry bucket is NOT created by the installer. It is created by the cluster-image-registry-operator after the cluster is up.

**Important Discovery:** The cluster-image-registry-operator has had full KMS encryption support since 2019 (commit `8c08cc6805` by Corey Daley, 2019-07-01). The implementation already exists in `pkg/storage/gcs/gcs.go`:

```go
// Existing code in cluster-image-registry-operator since 2019
if bucketCreated {
    if len(d.Config.KeyID) != 0 {
        _, err := bucket.Update(d.Context, gstorage.BucketAttrsToUpdate{
            Encryption: &gstorage.BucketEncryption{
                DefaultKMSKeyName: d.Config.KeyID,
            },
        })
        // ... error handling and status condition updates ...
    }
}
```

To enable KMS encryption for the registry bucket, the installer must:
1. Populate the `spec.storage.gcs.keyID` field in the ImageRegistry CR manifest
2. Format the KMS key as a full resource path: `projects/{project}/locations/{location}/keyRings/{keyring}/cryptoKeys/{key}`

The cluster-image-registry-operator will then:
1. Read the KMS key from `spec.storage.gcs.keyID` (as it already does today)
2. Create the registry GCS bucket with encryption configured
3. Set the bucket's `Encryption.DefaultKMSKeyName` to the provided KMS key
4. Update the `StorageEncrypted` condition based on success or failure

**No operator code changes are needed.** The installer just needs to populate the existing ImageRegistry CR field that the operator has supported for 7 years.

#### IAM Permissions Requirements

Based on how GCP KMS works with Cloud Storage, encryption keys are specified once at bucket creation. GCS automatically decrypts objects when accessed if the service account has the appropriate IAM permissions. **No runtime key passing is needed.**

**User-Provided Installer Service Account** needs:
```
roles/cloudkms.admin                        # To manage IAM policies on KMS keys (grants permissions to Google-managed accounts)
                                            # Or minimally: cloudkms.cryptoKeys.getIamPolicy + cloudkms.cryptoKeys.setIamPolicy
roles/storage.admin                         # To create and configure buckets
```

**The installer automatically grants these permissions during PreProvision:**

**Cloud Storage Service Account** (`service-{PROJECT_NUMBER}@gs-project-accounts.iam.gserviceaccount.com`):
```
roles/cloudkms.cryptoKeyEncrypterDecrypter  # To encrypt/decrypt bootstrap and registry buckets
```

**Compute Engine Service Account** (`service-{PROJECT_NUMBER}@compute-system.iam.gserviceaccount.com`):
```
roles/cloudkms.cryptoKeyEncrypterDecrypter  # To encrypt/decrypt OS disks
```

**Master Node Service Account** (created by installer as `{INFRA_ID}-m@{PROJECT_ID}.iam.gserviceaccount.com`):
```
roles/cloudkms.cryptoKeyEncrypterDecrypter  # To allow bootstrap and registry operator to access encrypted buckets
```

**Worker Node Service Accounts:**
```
NO KMS permissions needed                   # Workers pull images via HTTP from registry, not from GCS directly
```

The installer automatically grants these permissions in `pkg/infrastructure/gcp/clusterapi/iam.go` during the PreProvision phase, preventing users from needing to manually configure service account permissions. Users only need to grant the installer service account `roles/cloudkms.admin` (or equivalent) on the KMS key.

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

#### Risk: Insufficient IAM Policy Management Permissions

**Risk**: If the installer service account lacks `cloudkms.cryptoKeys.setIamPolicy` permission, automatic IAM grants will fail and installation will be blocked.

**Mitigation**:
- Provide clear documentation that installer service account needs `roles/cloudkms.admin` (or equivalent)
- Include detailed error messages that specify the exact permission missing
- Document the symptoms and resolution steps in troubleshooting guides
- The installer fails early during PreProvision (before creating any infrastructure), making it safe to retry

#### Risk: Automatic IAM Grants Fail Silently

**Risk**: If automatic IAM grants fail but don't block installation, the cluster may be created but registry or bootstrap may fail.

**Mitigation**:
- Automatic IAM grants happen during PreProvision phase, before any infrastructure is created
- Any failure in granting IAM permissions blocks the installation
- No partial state is created if grants fail
- Clear error messages guide users to the resolution

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

3. **Limited to GCP**: This is a platform-specific feature that doesn't apply to other clouds, though similar features could be added for AWS (S3 KMS) and Azure (Storage Account encryption).

4. **No Day-2 Migration**: Existing clusters cannot easily migrate from Google-managed to customer-managed encryption. Users must plan encryption strategy at install time.

## Alternatives (Not Implemented)

### Alternative 1: Separate Fields for Ignition and Registry

### Alternative 2: Separate Fields for Ignition and Registry (Original Proposal)

Add new structured fields to the GCP platform: `platform.gcp.ignition.storage.encryptionKey` and `platform.gcp.registry.storage.encryptionKey`.

**Pros**:
- Maximum flexibility - different keys for different purposes
- Aligns with security best practices of separating keys by data lifecycle
- Allows users to encrypt only one bucket type if desired
- Clear separation of concerns (ignition vs registry)
- Extensible for future ignition and registry settings

**Cons**:
- More complex API surface
- Requires users to configure multiple fields
- Creates operational overhead (multiple keys to manage, rotate, monitor)
- May lead to inconsistent security posture (some users might encrypt one but not the other)
- Adds new install-config schema when existing field can be reused

**Rejected because**: 
- The flexibility doesn't provide clear security or compliance value for most use cases
- Users who want customer-managed encryption typically want it for all cluster resources
- Additional API complexity and operational overhead outweigh the benefits
- The single-key approach (Alternative 1) better serves the common use case while being simpler

### Alternative 3: Automatic KMS Key Creation

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

### Alternative 4: Infrastructure CR for Registry Configuration

Initially proposed to add a new field to the Infrastructure CR to pass KMS configuration to the registry operator.

**Pros**:
- Infrastructure CR is the canonical source of platform configuration

**Cons**:
- Requires changes to openshift/api
- The ImageRegistry CR already has the `keyID` field (since 2019)
- Creates API duplication
- Infrastructure CR is not the right surface for registry-specific storage configuration

**Rejected because**: Analysis revealed that the ImageRegistry CR already has the necessary field (`spec.storage.gcs.keyID`) and the cluster-image-registry-operator has had full KMS support since 2019. No new API changes are needed.

### Alternative 5: Post-Installation Manual Configuration

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

~~1. **Should we support cross-project KMS keys in the initial implementation?**~~
   - Deferred to a future enhancement if needed

~~2. **Should we validate KMS key permissions during install-config validation?**~~
   - **Resolved**: Best-effort validation with warnings, not hard failures

~~3. **Should we support KMS key versioning/rotation?**~~
   - **Resolved**: GCS automatically uses the latest key version, no special handling needed

~~4. **How should we handle the cluster-image-registry-operator changes?**~~
   - **Resolved**: No operator changes needed. The operator has supported KMS encryption since 2019 via the `spec.storage.gcs.keyID` field in the ImageRegistry CR. The installer just needs to populate that existing field.

## Test Plan

### Unit Tests (Implemented)

Comprehensive unit tests have been implemented and are passing. See `UNIT_TEST_SUMMARY.md` in the installer repository for detailed results.

1. **KMS key validation tests** (`pkg/types/gcp/validation/kmskey_test.go`):
   - ✅ `TestValidateEncryptionKey` - 12 test cases:
     - Valid encryption keys (with/without project ID, with special characters)
     - Missing required fields (name, keyRing, location)
     - Invalid formats (special characters, spaces)
     - Encryption keys without KMS key
     - Service account combinations
   - ✅ `TestValidateMachinePoolWithEncryptionKey` - 3 test cases:
     - Valid machine pool with KMS encryption
     - Invalid KMS key names
     - Missing KMS location

2. **Bootstrap storage tests** (`pkg/asset/ignition/bootstrap/gcp/storage_test.go`):
   - ✅ `TestBuildKMSKeyName` - 5 test cases:
     - KMS key with all fields including project ID
     - KMS key without project ID (uses default)
     - KMS key with empty project ID (uses default)
     - KMS key with hyphens and underscores
     - KMS key in different region
   - ✅ `TestGetBootstrapStorageName` - 3 test cases:
     - Standard cluster ID, cluster ID with hyphens, short cluster ID

3. **ImageRegistry manifest tests** (`pkg/asset/manifests/imageregistry_test.go`):
   - ✅ `TestImageRegistryGenerate` - 5 test cases:
     - GCP with KMS encryption configured
     - GCP with KMS encryption and custom project ID
     - GCP without KMS encryption (no manifest generated)
     - GCP with DefaultMachinePlatform but no encryption key
     - AWS platform (no manifest generated)
   - ✅ `TestGenerateImageRegistryConfig` - 2 test cases:
     - Config generation with/without KMS encryption

4. **GCP manifest helper tests** (`pkg/asset/manifests/gcp/imageregistry_test.go`):
   - ✅ `TestBuildKMSKeyName` - 6 test cases:
     - KMS key formatting with various configurations
     - Project ID defaulting logic
     - Multi-region locations
     - Special characters in names

**Test Coverage:**
- ✅ KMS key required fields validation
- ✅ KMS key format validation (alphanumeric, hyphens, underscores)
- ✅ Integration with machine pool validation
- ✅ Edge cases (nil values, empty strings, special characters)
- ✅ Manifest generation when KMS key configured
- ✅ No manifest when KMS key not configured
- ✅ Custom project ID handling
- ✅ Non-GCP platforms (no manifest generated)
- ✅ YAML structure validation
- ✅ Bootstrap bucket naming and KMS key formatting
- ✅ Different regions and locations

**Running Tests:**
```bash
# All validation tests
go test ./pkg/types/gcp/validation

# All manifest tests
go test ./pkg/asset/manifests -run TestImageRegistry
go test ./pkg/asset/manifests/gcp

# All bootstrap storage tests
go test ./pkg/asset/ignition/bootstrap/gcp
```

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
   - Verify registry bucket is encrypted with customer key (operator support exists since 2019)
   - Verify cluster is functional
   - Verify images can be pushed and pulled from registry
   - Verify registry operator `StorageEncrypted` condition is True

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

### Shipping as GA

Given that:
- No new Kubernetes APIs are being added (install-config only)
- The cluster-image-registry-operator has supported KMS encryption since 2019 (production-proven)
- The feature is naturally opt-in via install-config (omit = default behavior)
- Similar install-time encryption features (AWS KMS, Azure disk encryption) shipped as GA
- GCP KMS is a mature, well-documented GCP service

**This enhancement can ship directly as GA without a Tech Preview phase.**

**GA Requirements:**
- Ability to utilize the enhancement end-to-end for both ignition and registry bucket encryption
- End user documentation available (install-config examples, IAM requirements, troubleshooting)
- Sufficient unit and integration test coverage
- End-to-end testing across multiple GCP regions
- Validation works correctly for all key reference formats
- IAM permission requirements clearly documented
- CI testing includes KMS encryption scenarios
- Support runbooks created for common failure scenarios
- Performance impact documented (minimal, < 10ms per operation)
- Validation that key rotation works seamlessly (GCS handles automatically)
- Compatibility verified with other GCP features (private clusters, shared VPC, etc.)

**Optional Tech Preview Approach:**
If desired for additional validation, the feature could be marked as "Tech Preview" in documentation only (no feature gate), then promoted to GA in the next release. However, this is not technically necessary given the existing registry operator support.

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

Not applicable.

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

2. **Registry Operator vs ImageRegistry CR**: The `spec.storage.gcs.keyID` field has existed in the ImageRegistry CR since 2019. All registry operator versions since then support customer-managed KMS encryption. No version skew concerns.

3. **Backward Compatibility**: If an older installer (without this enhancement) is used, it simply doesn't populate the `keyID` field, and the registry operator defaults to Google-managed encryption. This is the current behavior.

4. **Forward Compatibility**: If a newer installer populates `keyID` but an older registry operator is running (pre-2019), the field is ignored and Google-managed encryption is used. However, all currently supported OpenShift versions have registry operators that support KMS encryption.

## Operational Aspects of API Extensions

This enhancement does not add any new Kubernetes API extensions (no new CRDs, webhooks, aggregated API servers, or finalizers). It only modifies:
1. Installer install-config schema (input configuration, not a Kubernetes API)
2. ImageRegistry CR manifest generation (populates existing field)

### ImageRegistry CR Impact

**Field Used**: `spec.storage.gcs.keyID` (existing field since 2019)

**Impact on existing systems**:
- No impact on API throughput or availability
- No validation webhooks or admission controllers involved
- Field is optional and has existed for 7 years
- No finalizers or blocking operations
- Registry operator has supported this field since 2019

**Failure Modes**:
- If `keyID` is populated but KMS key doesn't exist: Registry operator degrades with `StorageEncrypted` condition showing "KMS key not found"
- If `keyID` contains invalid format: Registry operator degrades with condition showing format error
- If IAM permissions are missing: Registry operator degrades with condition showing "Permission denied"

## Support Procedures

### Detecting Failures

**Symptom**: Installer fails during PreProvision with "Permission denied" error when granting IAM policies

**Detection**:
- Error message: "Failed to grant KMS permissions: Permission denied"
- Logs show: "Error setting IAM policy on KMS key"

**Resolution**:
1. Check that KMS key exists:
   ```bash
   gcloud kms keys describe KEY_NAME --keyring=KEY_RING --location=LOCATION
   ```
2. Verify installer service account has IAM policy management permissions:
   ```bash
   gcloud kms keys get-iam-policy KEY_NAME --keyring=KEY_RING --location=LOCATION
   ```
3. Grant IAM policy management permissions to installer service account:
   ```bash
   gcloud kms keys add-iam-policy-binding KEY_NAME \
     --keyring=KEY_RING --location=LOCATION \
     --member=serviceAccount:INSTALLER_SA@PROJECT.iam.gserviceaccount.com \
     --role=roles/cloudkms.admin
   ```
   
   Or minimally (custom role):
   ```bash
   # Create custom role with just the needed permissions
   gcloud iam roles create kmsIamManager --project=PROJECT_ID \
     --permissions=cloudkms.cryptoKeys.getIamPolicy,cloudkms.cryptoKeys.setIamPolicy
   
   # Grant custom role
   gcloud kms keys add-iam-policy-binding KEY_NAME \
     --keyring=KEY_RING --location=LOCATION \
     --member=serviceAccount:INSTALLER_SA@PROJECT.iam.gserviceaccount.com \
     --role=projects/PROJECT_ID/roles/kmsIamManager
   ```

**Symptom**: Registry operator degraded after cluster installation

**Detection**:
- `oc get clusteroperator image-registry` shows Degraded=True
- Operator logs show: "Failed to create GCS bucket: Permission denied"
- Check ImageRegistry CR: `oc get configs.imageregistry.operator.openshift.io cluster -o yaml`
- Look for `StorageEncrypted` condition with status False and reason "InvalidStorageConfiguration"

**Resolution**:
1. Check that `spec.storage.gcs.keyID` is set correctly in ImageRegistry CR:
   ```bash
   oc get configs.imageregistry.operator.openshift.io cluster -o jsonpath='{.spec.storage.gcs.keyID}'
   ```
2. Verify KMS key exists:
   ```bash
   gcloud kms keys describe KEY_NAME --keyring=KEY_RING --location=LOCATION
   ```
3. Check if master service account has KMS permissions (should have been granted automatically):
   ```bash
   gcloud kms keys get-iam-policy KEY_NAME --keyring=KEY_RING --location=LOCATION | grep "{INFRA_ID}-m@"
   ```
4. If permissions are missing (automatic grant failed), manually grant:
   ```bash
   gcloud kms keys add-iam-policy-binding KEY_NAME \
     --keyring=KEY_RING --location=LOCATION \
     --member=serviceAccount:{INFRA_ID}-m@{PROJECT_ID}.iam.gserviceaccount.com \
     --role=roles/cloudkms.cryptoKeyEncrypterDecrypter
   ```
5. Operator will retry bucket creation automatically

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
   - GCP platform team
   - Documentation team

**Note:** No collaboration needed with cluster-image-registry-operator team or openshift/api maintainers - the registry operator already supports KMS encryption (since 2019) and no new API changes are required.

No new GitHub repositories or subprojects are needed.
