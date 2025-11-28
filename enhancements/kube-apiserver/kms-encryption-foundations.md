---
title: kms-encryption-foundations
authors:
  - "@ardaguclu"
  - "@dgrisonnet"
  - "@flavianmissi"
reviewers:
  - "@ibihim"
  - "@sjenning"
  - "@tkashem"
  - "@derekwaynecarr"
approvers:
  - "@benluddy"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-01-28
last-updated: 2025-01-28
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-108"
see-also:
  - "enhancements/kube-apiserver/kms-plugin-management.md"
  - "enhancements/kube-apiserver/kms-migration-recovery.md"
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
replaces:
  - ""
superseded-by:
  - ""
---

# KMS Encryption Foundations

## Summary

Provide the foundational support for Key Management Service (KMS) encryption in OpenShift by:
1. Extending the `config.openshift.io/v1/APIServer` resource to add KMS encryption configuration
1. Extending encryption controllers in `openshift/library-go` to support KMS encryption

This enhancement enables the existing encryption infrastructure (`keyController`, `stateController`, `migrationController`) to work with externally-managed encryption keys from KMS providers, while maintaining feature parity with local encryption modes (aescbc, aesgcm). For Tech Preview, only AWS KMS is supported; additional providers (Vault, Thales) will be added in GA.

## Motivation

OpenShift's existing encryption controllers manage local AES keys for encrypting data at rest in etcd. Adding KMS support to these controllers enables integration with external Key Management Systems (KMS) where encryption keys are stored and rotated outside the cluster. KMS encryption protects against attackers who gain access to control plane nodes, since the encryption keys are stored externally rather than on the nodes themselves.

The controller extensions are designed to minimize provider-specific logic. While some provider-specific code is necessary for configuration handling, the core controller logic for key rotation detection and migration remains provider-agnostic.

### User Stories

* As a cluster admin, I want encryption controllers to automatically detect when my external KMS rotates an encryption key, so that OpenShift can re-encrypt my data with the new key without manual intervention
* As a cluster admin, I want the same migration and monitoring experience for KMS encryption as I have with local AES encryption, so that I don't need to learn new operational procedures
* As a cluster admin, I want encryption controllers to verify KMS plugin health before performing operations, so that encryption/decryption failures don't impact cluster availability

### Goals

* Extend encryption controllers to support KMS as a new encryption mode
* Implement automatic key rotation detection based on KMS plugin-provided `key_id`
* Maintain empty encryption key secrets for KMS (keys stored externally)
* Ensure existing migration workflows work seamlessly with KMS
* Provide provider-agnostic controller implementation (no provider-specific logic)
* Maintain feature parity with existing encryption modes

### Non-Goals

* KMS plugin deployment and lifecycle management (see [KMS Plugin Management](kms-plugin-management.md))
* Provider-specific KMS configurations (AWS, Vault, Thales details are in [KMS Plugin Management](kms-plugin-management.md))
* Migration between different KMS providers (deferred to [KMS Migration and Recovery](kms-migration-recovery.md) for GA)
* Recovery from KMS key loss scenarios (deferred to [KMS Migration and Recovery](kms-migration-recovery.md) for GA)

## Proposal

Extend the existing encryption controller framework in `openshift/library-go` to support KMS encryption by:

1. Adding KMS as a new encryption mode in the `state` package
2. Implementing hash-based key rotation detection using KMS configuration and `key_id`
3. Managing empty encryption key secrets for KMS (actual keys stored in external KMS)
4. Extending controller preconditions to verify KMS plugin health
5. Ensuring migration controller works with KMS encryption transitions

The implementation maintains the existing controller architecture while adding KMS-specific logic where necessary.

### Workflow Description

#### Roles

**keyController** is the library-go controller responsible for creating and rotating encryption key secrets.

**KMS Plugin** is a gRPC service implementing the Kubernetes KMS v2 API, running as a sidecar to API server pods.

**External KMS** is the cloud or on-premises Key Management Service (e.g., AWS KMS, HashiCorp Vault) that stores and manages the Key Encryption Key (KEK).

#### Key Rotation Detection Workflow

1. The cluster admin configures a KMS provider in the APIServer config
2. The cluster admin configures automatic key rotation in their external KMS
3. The external KMS rotates the KEK (e.g., AWS KMS creates a new key version)
4. The KMS plugin detects the rotation and updates its Status response with a new `key_id`
5. The keyController polls the KMS plugin Status endpoint (via gRPC)
6. The keyController detects the `key_id` has changed
7. The keyController computes a new `kmsKeyIDHash` (combining config hash + new `key_id`)
8. The keyController creates a new encryption key secret with the updated hash
9. The migrationController detects the new secret and initiates data re-encryption
10. Resources are re-encrypted using the new KEK in the external KMS

#### KMS Configuration Change Workflow

1. The cluster admin updates the KMS configuration (e.g., changes Vault address or AWS key ARN)
2. The keyController detects the configuration change in APIServer config
3. The keyController computes a new `kmsConfigHash`
4. The keyController computes a new `kmsKeyIDHash` (new config hash + current `key_id`)
5. The keyController creates a new encryption key secret
6. The migrationController initiates re-encryption with the new configuration

### API Extensions

This enhancement extends the `config.openshift.io/v1/APIServer` resource to add KMS as a new encryption type. The API provides a foundation for KMS encryption that is extended in future releases to support additional KMS providers.

#### Encryption Type Extension

```diff
diff --git a/config/v1/types_apiserver.go b/config/v1/types_apiserver.go
index d815556d2..c9098024f 100644
--- a/config/v1/types_apiserver.go
+++ b/config/v1/types_apiserver.go
@@ -191,9 +194,23 @@ type APIServerEncryption struct {
        // +unionDiscriminator
        // +optional
        Type EncryptionType `json:"type,omitempty"`
+
+       // kms defines the configuration for external KMS encryption.
+       // When KMS encryption is enabled, sensitive resources are encrypted using keys managed by an
+       // externally configured KMS instance.
+       //
+       // The Key Management Service (KMS) instance provides symmetric encryption and is responsible for
+       // managing the lifecycle of encryption keys outside of the control plane.
+       //
+       // +openshift:enable:FeatureGate=KMSEncryptionProvider
+       // +unionMember
+       // +optional
+       KMS *KMSConfig `json:"kms,omitempty"`
```

```diff
@@ -208,6 +225,11 @@ const (
        // aesgcm refers to a type where AES-GCM with random nonce and a 32-byte key
        // is used to perform encryption at the datastore layer.
        EncryptionTypeAESGCM EncryptionType = "aesgcm"
+
+       // kms refers to a type of encryption where the encryption keys are managed
+       // outside the control plane in a Key Management Service instance.
+       // Encryption is still performed at the datastore layer.
+       EncryptionTypeKMS EncryptionType = "KMS"
 )
```

#### KMS Configuration Types

New file: `config/v1/types_kmsencryption.go`

```go
package v1

// KMSConfig defines the configuration for the KMS instance used with KMS encryption.
// The configuration is provider-specific and uses a union discriminator pattern to
// ensure only the appropriate provider configuration is set.
//
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'AWS' ? has(self.aws) : !has(self.aws)",message="aws config is required when kms provider type is AWS, and forbidden otherwise"
// +union
type KMSConfig struct {
	// type defines the KMS provider type.
	//
	// For Tech Preview, only AWS is supported.
	// Additional providers (Vault, Thales) will be added in GA.
	//
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	Type KMSProviderType `json:"type"`

	// aws defines the configuration for AWS KMS encryption.
	// The AWS KMS instance is managed by the user outside the control plane.
	//
	// +unionMember
	// +optional
	AWS *AWSKMSConfig `json:"aws,omitempty"`
}

// KMSProviderType defines the supported KMS provider types.
//
// For Tech Preview, only AWS is supported.
// +kubebuilder:validation:Enum=AWS
type KMSProviderType string

const (
	// AWSKMSProvider represents AWS Key Management Service
	AWSKMSProvider KMSProviderType = "AWS"
)

// AWSKMSConfig defines the configuration specific to AWS KMS provider.
type AWSKMSConfig struct {
	// keyARN specifies the Amazon Resource Name (ARN) of the AWS KMS key used for encryption.
	// The value must adhere to the format `arn:aws:kms:<region>:<account_id>:key/<key_id>`, where:
	// - `<region>` is the AWS region consisting of lowercase letters and hyphens followed by a number.
	// - `<account_id>` is a 12-digit numeric identifier for the AWS account.
	// - `<key_id>` is a unique identifier for the KMS key, consisting of lowercase hexadecimal characters and hyphens.
	//
	// Example: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self.matches('^arn:aws:kms:[a-z0-9-]+:[0-9]{12}:key/[a-f0-9-]+$') && self.size() <= 128",message="keyARN must follow the format `arn:aws:kms:<region>:<account_id>:key/<key_id>`"
	KeyARN string `json:"keyARN"`

	// region specifies the AWS region where the KMS instance exists.
	// The format is `<region-prefix>-<region-name>-<number>`, e.g., `us-east-1`.
	// Only lowercase letters, hyphens, and numbers are allowed.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self.matches('^[a-z]{2}-[a-z]+-[0-9]+$') && self.size() <= 64",message="region must be a valid AWS region format"
	Region string `json:"region"`
}
```

#### Graduation Path for Additional Providers

**Tech Preview:**
- `KMSProviderType` enum contains only `AWS`
- `KMSConfig` union has only `aws` field

**GA (Future Enhancement):**
The enum and union will be extended to support additional providers:

```go
// +kubebuilder:validation:Enum=AWS;Vault;Thales
type KMSProviderType string

const (
	AWSKMSProvider    KMSProviderType = "AWS"
	VaultKMSProvider  KMSProviderType = "Vault"    // Added in GA
	ThalesKMSProvider KMSProviderType = "Thales"   // Added in GA
)

type KMSConfig struct {
	Type   KMSProviderType `json:"type"`
	AWS    *AWSKMSConfig   `json:"aws,omitempty"`
	Vault  *VaultKMSConfig `json:"vault,omitempty"`   // Added in GA
	Thales *ThalesKMSConfig `json:"thales,omitempty"` // Added in GA
}
```

The provider-specific management details for Vault and Thales are documented in [KMS Plugin Management](kms-plugin-management.md).

### Topology Considerations

#### Hypershift / Hosted Control Planes

The library-go encryption controllers run in the management cluster as part of the hosted control plane operators. KMS plugin health checks must account for the split architecture where plugins may run in different contexts than the controllers.

#### Standalone Clusters

This enhancement applies to standalone clusters. The controllers run in the cluster-kube-apiserver-operator, cluster-openshift-apiserver-operator, and cluster-authentication-operator.

#### Single-node Deployments or MicroShift

Resource consumption impact is minimal - the controllers already exist and are extended with KMS-specific logic. Single-node deployments will see slightly increased CPU usage during key rotation detection (gRPC Status calls), but this is negligible.

MicroShift may adopt this enhancement if KMS encryption is desired, but the configuration mechanism may differ (file-based vs API resource).

### Implementation Details/Notes/Constraints

This section documents the implementation in `openshift/library-go` PR #2045.

#### Hash-Based Key Rotation Detection

The controllers track two separate hashes:

1. **kmsConfigHash** - Hash of the KMS configuration (provider type, endpoint, credentials reference)
   - Used to detect when admin changes KMS configuration
   - Stored in secret annotation: `encryption.apiserver.operator.openshift.io/kms-config-hash`

2. **kmsKeyIDHash** - Combined hash of config + `key_id` from KMS plugin
   - Used to detect when external KMS rotates the key
   - Stored in secret `Data` field (base64 encoded)
   - Computed as: `kms.ComputeKMSKeyHash(configHash, keyId)`

**Why two hashes?**
- Config changes may not change the key (e.g., updating Vault address but same key)
- Key rotation may not change config (e.g., AWS KMS rotates key materials, same ARN)
- Separating them allows proper handling of each scenario

#### Key Controller Changes

Modified functions in `pkg/operator/encryption/controllers/key_controller.go`:

```go
// New function type for getting KMS hashes (allows testing/mocking)
var kmsHashesGetterFunc func(ctx context.Context, kmsConfig *configv1.KMSConfig) (configHash string, keyIDHash []byte, err error)

// Extended to return KMS config
func (c *keyController) getCurrentModeAndExternalReason(ctx context.Context) (state.Mode, string, *configv1.KMSConfig, error)

// Extended to accept KMS hashes
func (c *keyController) generateKeySecret(keyID uint64, currentMode state.Mode, internalReason, externalReason string, kmsConfigHash string, kmsKeyIDHash []byte) (*corev1.Secret, error)

// Extended to check KMS key hash changes
func needsNewKey(grKeys state.GroupResourceState, currentMode state.Mode, externalReason string, encryptedGRs []schema.GroupResource, kmsKeyHash []byte) (uint64, string, bool)
```

Key rotation logic for KMS mode:
```go
if currentMode == state.KMS {
    if latestKey.Key.Secret != base64.StdEncoding.EncodeToString(kmsKeyHash) {
        // Either config changed or key_id rotated
        return latestKeyID, "kms-key-changed", true
    }
    // For KMS mode, NO time-based rotation
    // KMS keys are rotated externally by the KMS system
    return 0, "", false
}
```

#### Empty Encryption Key Secrets

For KMS mode, encryption key secrets do NOT contain actual key material (the KEK lives in external KMS). Instead:
- `Data["secrets"]` contains the base64-encoded `kmsKeyIDHash`
- Annotations contain metadata:
  - `encryption.apiserver.operator.openshift.io/mode: "kms"`
  - `encryption.apiserver.operator.openshift.io/kms-config-hash: "<configHash>"`
  - `encryption.apiserver.operator.openshift.io/internal-reason: "kms-key-changed"`

This allows reusing existing secret management logic while clearly indicating KMS mode.

#### Migration Controller Compatibility

No changes required to `migration_controller.go` - it already works with KMS because:
- It triggers on new encryption key secrets (regardless of mode)
- Migration uses the `EncryptionConfiguration` generated by `stateController`
- The actual encryption/decryption happens in kube-apiserver via KMS plugin

Test coverage added in `migration_controller_test.go` for KMS rotation scenarios.

#### Static vs Dynamic key_id (Tech Preview vs GA)

**Tech Preview Implementation:**
```go
func defaultGetKMSHashes(ctx context.Context, kmsConfig *configv1.KMSConfig) (string, []byte, error) {
    _, configHash, err := kms.GenerateUnixSocketPath(kmsConfig)
    if err != nil {
        return "", nil, fmt.Errorf("failed to generate KMS unix socket path: %w", err)
    }

    // TODO: Call KMS plugin Status gRPC endpoint to get actual key_id
    // For TP, use static key_id (AWS KMS doesn't rotate key_id anyway)
    keyId := "static-key-id"
    return configHash, kms.ComputeKMSKeyHash(configHash, keyId), nil
}
```

**GA Implementation (Future Work):**
- Call KMS plugin Status endpoint: `grpc.Dial(socketPath)` → `kmsv2.Status()`
- Extract `key_id` from response
- Implement retry/timeout logic for Status calls
- Handle plugin unavailability gracefully

**AWS KMS Special Case:**
The AWS KMS plugin does not change `key_id` when AWS rotates key materials. This is a known limitation. For AWS:
- Rotation triggered only by config changes (new key ARN)
- Automatic AWS key rotation does NOT trigger OpenShift re-encryption
- Users must update APIServer config with new key ARN to rotate

#### Controller Preconditions

The existing `preconditionsFulfilled` mechanism needs extension to check KMS plugin health:

**Current (unchanged):**
```go
type PreconditionFunc func(ctx context.Context) (bool, error)
```

**Future (GA):**
- Add KMS plugin health check precondition
- Call KMS plugin Status endpoint
- Verify plugin returns healthy status
- Block controller sync if plugin unavailable
- The health check implementation is provided by operators (see the "KMS Plugin Health Monitoring" section in [KMS Plugin Management](kms-plugin-management.md))

### Risks and Mitigations

#### Risk: KMS Plugin Unavailable During Controller Sync

**Mitigation:**
- Preconditions check plugin health before operations
- Controllers gracefully skip sync if plugin down
- Existing encryption continues working (kube-apiserver caches DEKs)

#### Risk: AWS KMS key_id Limitation

**Mitigation:**
- Document this limitation clearly
- Provide guidance: users must update APIServer config to rotate
- Consider future enhancement to poll AWS KMS directly

#### Risk: Performance Impact of Status Polling

**Mitigation:**
- Status calls are cheap (gRPC local Unix socket)
- Controllers already have rate limiting
- Cache `key_id` between syncs (only call Status if cache expired)

#### Risk: etcd Backup Restoration Without KMS Key Access

**Risk:**
When restoring an etcd backup, the cluster cannot decrypt data if the KMS key used during encryption is unavailable (deleted, different KMS instance, expired credentials, or key rotated past retention period).

**Impact:**
- **Data loss:** Resources encrypted with unavailable keys become permanently unrecoverable
- **Cluster inoperable:** API server may fail to start if critical resources cannot be decrypted
- **Partial recovery:** Only resources encrypted with still-available keys can be restored

**Mitigations:**

1. **KMS Key Deletion Grace Periods:**
   - Configure KMS to use deletion grace periods (e.g., AWS KMS 7-30 day pending deletion)
   - Ensure KMS keys are not permanently deleted until backup retention expires
   - Document minimum grace period = backup retention period

2. **Backup Procedure Documentation:**
   - Document KMS key dependencies in backup/restore runbooks
   - Include KMS key ID and configuration in backup metadata
   - Test restore procedures regularly to verify KMS key availability

3. **KMS Key Backup/Recovery:**
   - For on-premises KMS (Vault, Thales): Ensure KMS key material is backed up separately
   - For cloud KMS (AWS): Understand key recovery limitations (AWS does not export key material)
   - Consider key escrow strategies for critical environments (GA consideration)

4. **Cross-Region/Cross-Account Scenarios:**
   - Document KMS key access requirements for disaster recovery scenarios
   - Ensure backup restoration accounts/regions have access to original KMS keys
   - Consider multi-region key replication where supported by KMS provider

5. **Monitoring and Alerts:**
   - Alert on KMS key pending deletion (detect before permanent deletion)
   - Alert on KMS key access failures during backup operations
   - Track KMS key retention vs. backup retention alignment

6. **User Documentation:**
   - Clearly document in openshift-docs: "etcd backups depend on KMS key availability"
   - Provide restore procedures that verify KMS key access before attempting restore
   - Warn about consequences of KMS key deletion

**Testing Requirements:**
- E2E tests must validate backup/restore with KMS encryption
- Include failure scenarios (KMS key deleted, credentials expired)
- Document expected behavior and recovery procedures

### Drawbacks

- Adds complexity to encryption controllers for KMS-specific logic
- AWS KMS requires config changes for rotation (not automatic)
- Dependency on KMS plugin health for controller operations

## Alternatives (Not Implemented)

### Alternative: Separate KMS-Specific Controllers

Instead of extending existing controllers, create new KMS-only controllers.

**Why not chosen:**
- Code duplication (migration logic, state management)
- User confusion (different controllers for different encryption types)
- More operational burden (additional monitoring, alerts)

### Alternative: Time-Based Rotation for KMS

Continue weekly rotation even with KMS, generate new secrets periodically.

**Why not chosen:**
- KMS keys are rotated externally, not by OpenShift
- Unnecessary re-encryption burden
- Doesn't align with KMS operational model

## Open Questions

None - implementation is complete in PR #2045.

## Test Plan

**Unit Tests:** (Already in PR #2045)
- `key_controller_test.go`: KMS key creation, rotation detection, hash changes
- `migration_controller_test.go`: KMS migration scenarios

**Integration Tests:** (Future work)
- End-to-end KMS encryption workflow
- Key rotation with real KMS plugin
- Migration between encryption modes (aescbc → KMS, KMS → identity)

**E2E Tests:** (Future work)
- Full cluster with KMS encryption enabled
- Trigger external KMS key rotation
- Verify data re-encryption completes
- Performance testing (time to migrate N secrets)

## Graduation Criteria

### Tech Preview → GA

- **Dynamic key_id fetching:** Call KMS plugin Status endpoint (not static)
- **Health check preconditions:** Block operations when plugin unhealthy
- **AWS KMS workaround:** Document or implement solution for non-rotating key_id
- **Performance validation:** Ensure migration completes within SLOs
- **Comprehensive test coverage:** Integration and E2E tests passing
- **Production validation:** Run in multiple environments successfully

## Upgrade / Downgrade Strategy

**Upgrade:**
- PR #2045 code lands in library-go
- Operators import updated library-go version
- No user action required (controllers remain backward compatible)
- Existing aescbc/aesgcm encryption unaffected

**Downgrade:**
- If KMS encryption enabled, downgrade requires switching back to aescbc
- KMS-specific code paths are new, no risk to existing encryption

## Version Skew Strategy

The encryption controllers run in operator pods, not on nodes. Version skew concerns:

- **kube-apiserver version:** Must support KMS v2 API (Kubernetes 1.27+)
- **library-go version:** Operators must use same library-go version
- **KMS plugin version:** Controllers don't directly interact with plugins (operators do)

No special version skew handling required.

## Operational Aspects of API Extensions

This enhancement extends the `config.openshift.io/v1/APIServer` resource with new fields for KMS configuration. This is not a CRD, webhook, or aggregated API server - it's an extension to an existing core OpenShift API resource.

### Service Level Indicators (SLIs)

Administrators can monitor KMS encryption health through:

**Operator Conditions:**
- `cluster-kube-apiserver-operator` conditions:
  - `EncryptionControllerDegraded=False` - Controllers are functioning
  - `EncryptionMigrationControllerProgressing` - Migration status (key rotation)
  - `KMSPluginDegraded=False` - KMS plugin is healthy (see [KMS Plugin Management](kms-plugin-management.md))

**Metrics:**
- `apiserver_storage_transformation_operations_total` - Encryption/decryption operations
- `apiserver_storage_transformation_duration_seconds` - Latency of encryption operations
- KMS plugin health metrics (see [KMS Plugin Management](kms-plugin-management.md))

### Impact on Existing SLIs

**API Availability:**
- KMS encryption adds latency to resource creation/updates (external KMS call required)
- Expected impact: +10-50ms per operation (depends on KMS latency)
- Mitigation: DEK caching in kube-apiserver reduces calls to KMS

**API Throughput:**
- Minimal impact on read operations (decryption uses cached DEKs)
- Write operations may see slight throughput reduction due to KMS latency
- Expected: <5% throughput reduction under normal conditions

**Scalability:**
- KMS configuration is cluster-scoped (single `APIServer` resource)
- Expected use case: 1 KMS configuration per cluster
- No impact on scalability limits

### Failure Modes

**KMS Plugin Unavailable:**
- **Impact:** New resource creation fails, existing resources remain readable (DEKs cached)
- **Detection:** `KMSPluginDegraded=True` condition, alerts fire
- **Recovery:** Automatic (plugin restarts), or manual intervention (see Support Procedures)
- **Affected Teams:** API Server team, etcd team

**KMS Service Unavailable (External):**
- **Impact:** New DEK generation fails, encryption operations fail
- **Detection:** Increased encryption operation failures, KMS plugin health checks fail
- **Recovery:** Depends on external KMS (AWS, Vault, Thales) restoration
- **Affected Teams:** Customer infrastructure team

**Invalid KMS Configuration:**
- **Impact:** KMS plugin fails to start, encryption unavailable
- **Detection:** `KMSPluginDegraded=True`, plugin container crash loops
- **Recovery:** Fix APIServer configuration (credentials, endpoint, key ID)
- **Affected Teams:** Customer infrastructure team, API Server team

**Key Rotation Stuck:**
- **Impact:** Migration controller unable to re-encrypt resources
- **Detection:** `EncryptionMigrationControllerProgressing=True` for extended period
- **Recovery:** Check migration controller logs, verify KMS health
- **Affected Teams:** API Server team, etcd team

**etcd Backup Restoration Without KMS Access:**
- **Impact:** Restored cluster cannot decrypt etcd data if KMS key is unavailable or deleted
- **Detection:** API server fails to start or resource reads return decryption errors after restore
- **Recovery:**
  - **Best case:** Restore KMS key from backup/recovery (if KMS supports key recovery within grace period)
  - **Worst case:** Data loss - resources encrypted with lost key are unrecoverable
  - **Prevention:** Document KMS key dependencies in backup procedures, test restore procedures
- **Affected Teams:** etcd team, Customer infrastructure team, API Server team
- **Note:** This is why KMS key deletion grace periods are critical (see Risks and Mitigations)

### Measurement and Monitoring

**How to measure impact:**
- Prometheus queries for encryption operation latency percentiles (p50, p95, p99)
- Compare pre/post KMS enablement metrics
- Load testing before GA to establish SLOs

**When to measure:**
- Every release by QE (automated tests)
- Performance team review during GA graduation
- Customer escalations (if performance issues reported)

**Who measures:**
- **Dev/QE:** Automated CI tests, pre-release validation
- **Performance Team:** Load testing, SLO validation for GA
- **Site Reliability:** Production monitoring, SLI tracking

## Support Procedures

### Detecting KMS Rotation Issues

**Symptoms:**
- `EncryptionMigrationControllerProgressing` condition stuck at `True`
- Events in operator namespace: "migration in progress for KMS key rotation"
- No new encryption key secret created despite KMS key rotation

**Diagnosis:**
```bash
# Check if key controller detected rotation
oc get secrets -n openshift-config-managed -l encryption.apiserver.operator.openshift.io/component=encryption-key

# Check controller logs
oc logs -n openshift-kube-apiserver-operator deployment/kube-apiserver-operator | grep -i kms

# Verify KMS plugin Status response (if dynamic key_id implemented)
# Requires exec into API server pod and calling plugin gRPC endpoint
```

**Resolution:**
- If key_id not changing: Update KMS configuration in APIServer config
- If plugin unhealthy: Check plugin pod logs (see [KMS Plugin Management](kms-plugin-management.md))
- If stuck migration: Check migration controller logs

### Disabling KMS Encryption

To switch from KMS back to local encryption:
1. Update APIServer config: `spec.encryption.type: "aescbc"`
2. Wait for migration to complete
3. KMS plugin pods can be removed (handled by operators)

**Consequences:**
- Data re-encrypted with local AES keys
- Migration takes time proportional to data size
- Cluster remains available during migration

### etcd Backup and Restore with KMS Encryption

**Before Taking Backup:**
1. Document current KMS configuration:
   ```bash
   oc get apiserver cluster -o jsonpath='{.spec.encryption.kms}' | jq .
   ```
2. Record KMS key ID/ARN and provider details
3. Verify KMS key will remain available for backup retention period
4. Include KMS configuration in backup metadata

**Before Restoring Backup:**

1. **Verify KMS key availability:**
   ```bash
   # For AWS KMS - check key status
   aws kms describe-key --key-id <key-arn>
   # Key state should be "Enabled", not "PendingDeletion"

   # For Vault - verify key exists and is accessible
   vault read transit/keys/<key-name>
   ```

2. **Verify KMS credentials:**
   - Ensure restored cluster has access to same KMS instance
   - Verify IAM roles/credentials are valid for KMS access
   - Test KMS plugin can authenticate and call Status endpoint

3. **Restore etcd backup:**
   - Follow standard etcd restore procedures
   - Ensure KMS plugin pods start successfully
   - Verify API server can decrypt resources

**Troubleshooting Restore Failures:**

**Symptom:** API server fails to start after restore with KMS decryption errors

**Diagnosis:**
```bash
# Check API server logs for decryption errors
oc logs -n openshift-kube-apiserver kube-apiserver-<node> | grep -i "decrypt\|kms"

# Check KMS plugin health
oc get pods -n openshift-kube-apiserver -l app=kms-plugin

# Verify KMS key accessibility
# (provider-specific commands as shown above)
```

**Resolution:**
- **If KMS key deleted:** Check if within grace period, undelete if possible
- **If key expired/rotated:** Restore KMS key backup (Vault/Thales) or contact KMS admin
- **If credentials invalid:** Update credentials, restart KMS plugin pods
- **If unrecoverable:** Data encrypted with lost key is permanently lost (see Risks and Mitigations)

**Critical Warning:**
Deleting a KMS key used for encryption **will make etcd backups unrestorable**. Always ensure KMS key retention period ≥ backup retention period.

## Infrastructure Needed

None - this enhancement extends existing library-go code.
