---
title: kms-encryption-foundations
authors:
  - "@ardaguclu"
  - "@flavianmissi"
reviewers:
  - "@ibihim"
  - "@sjenning"
  - "@tkashem"
approvers:
  - "@benluddy"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-12-03
last-updated: 2025-12-03
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-108"
see-also:
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
  - "[encrypt data at rest with KMS](https://github.com/openshift/enhancements/pull/1872)"
replaces:
  - "[KMS Encryption Provider for Etcd Secrets](https://github.com/openshift/enhancements/pull/1682/)"
---

# KMS Encryption Foundations

## Summary

Extend OpenShift encryption controllers to support external Key Management Services (KMS) alongside existing local encryption modes (aescbc, aesgcm). This allows encryption keys to be stored and managed outside the cluster for enhanced security.

This enhancement:
- Extends the `config.openshift.io/v1/APIServer` resource for KMS configuration
- Extends encryption controllers in `openshift/library-go` to support KMS as a new encryption mode
- Maintains feature parity with existing encryption modes (migration, monitoring, key rotation)
- Supports AWS KMS and Vault in Tech Preview (Thales in future iterations)

## Motivation

OpenShift currently manages AES keys locally for encrypting data at rest in etcd. KMS support enables integration with external key management systems where encryption keys are stored outside the cluster, protecting against attacks where control plane nodes are compromised.

### Goals

- Support KMS as a new encryption mode in existing encryption controllers
- Seamless migration between encryption modes (aescbc ↔ KMS)
- Provider-agnostic controller implementation with minimal provider-specific code
- Feature parity with existing modes (monitoring, migration, key rotation)

### Non-Goals

- Implementing KMS plugins (provided by upstream Kubernetes/vendors)
- KMS plugin deployment/lifecycle management (separate EP for Tech Preview)
- KMS plugin health checks (Tech Preview v2)
- Migration between different KMS providers (separate EP for GA)
- Recovery from KMS key loss (separate EP for GA)
- Automatic `key_id` rotation detection (Tech Preview v2)

## Proposal

Extend the existing encryption controller framework in `openshift/library-go` to support KMS encryption through hash-based change detection. The controllers calculate a hash of the KMS configuration to detect changes and trigger re-encryption, avoiding the need for external service dependencies.

**Key changes:**
1. Add KMS mode constant to encryption state types
2. Implement hash-based detection for KMS configuration changes
3. Manage empty encryption key secrets (actual keys in external KMS)
4. Reuse existing migration controller (no changes needed)

**Tech Preview v2 additions:**
- Poll KMS plugin Status endpoint for `key_id` changes in apiserver operators
- Store hash of `key_id` in data field of encryption key secrets
- Hash-based detection for external key rotation

### Workflow Description

#### Actors in the Workflow

**cluster admin** is a human user responsible for configuring and maintaining the cluster.

**KMS** is the external Key Management Service (AWS KMS, HashiCorp Vault, etc.) that stores and manages the Key Encryption Key (KEK).

**KMS plugin** is a gRPC service implementing Kubernetes KMS v2 API, running as a sidecar to API server pods. It communicates with the external KMS to encrypt/decrypt data encryption keys (DEKs).

**API server operator** is the OpenShift operator (kube-apiserver-operator, openshift-apiserver-operator, or authentication-operator) managing API server deployments.

#### Encryption Controllers

**keyController** manages encryption key lifecycle. Creates encryption key secrets in `openshift-config-managed` namespace. For KMS mode, creates empty secrets with KMS configuration hashes.

**stateController** generates EncryptionConfiguration for API server consumption. Implements distributed state machine ensuring all API servers converge to same revision. For KMS mode, generates configuration with deterministic Unix socket paths.

**migrationController** orchestrates resource re-encryption. Marks resources as migrated after rewriting in etcd. Works with all encryption modes including KMS.

**pruneController** prunes inactive encryption key secrets. Maintains N keys (currently 10) for rollback scenarios.

**conditionController** determines when controllers should act. Provides status conditions (`EncryptionInProgress`, `EncryptionCompleted`, `EncryptionDegraded`).

#### Steps for Enabling KMS Encryption

1. Cluster admin updates the APIServer resource:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   spec:
     encryption:
       type: kms
       kms:
         aws:
           region: us-east-1
           keyArn: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
   ```

2. keyController detects the new encryption mode and calculates hash of the KMS configuration.

3. keyController creates encryption key secret:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: openshift-kube-apiserver-encryption-1
     namespace: openshift-config-managed
     annotations:
       encryption.apiserver.operator.openshift.io/mode: "kms"
       encryption.apiserver.operator.openshift.io/kms-config-hash: "a1b2c3d4e5f67890"
   data:
     keys: ""  # Empty in Tech Preview - KEK stored in external KMS
              # In Tech Preview v2, will contain base64-encoded key_id hash
   ```

4. stateController generates EncryptionConfiguration with hash embedded in socket path:
   ```yaml
   apiVersion: apiserver.config.k8s.io/v1
   kind: EncryptionConfiguration
   resources:
     - resources: [configmap]
       providers:
         - kms:
             name: kms-a1b2c3d4e5f67890-configmap-1
             endpoint: unix:///var/run/kmsplugin/kms-a1b2c3d4e5f67890.socket
             apiVersion: v2
   ```
   The deterministic socket path allows KMS plugin lifecycle management to use the same path.

5. migrationController detects the new secret and initiates re-encryption (no code changes - works with any mode).

6. Resources are re-encrypted using KEK in external KMS via the KMS plugin.

7. conditionController updates status conditions: `EncryptionInProgress`, then `EncryptionCompleted`.

#### Variation: Configuration Changes (Key Rotation)

When cluster admin updates KMS configuration (e.g., new key ARN, different region):

1. keyController recalculates hash from updated APIServer resource.
2. Compares new hash with hash in most recent encryption key secret annotation.
3. If hashes differ:
   - Creates new encryption key secret with new hash
   - migrationController automatically triggers re-encryption
4. If hashes match: No action.

**Note:** Automatic weekly key rotation (used for aescbc/aesgcm) is disabled for KMS since rotation is triggered externally.

#### Variation: External KMS Key Rotation (Tech Preview v2)

When external KMS rotates the key internally (e.g., AWS KMS automatic rotation):

1. keyController polls KMS plugin Status endpoint for `key_id`.
2. Calculates hash of `key_id` and compares with hash in secret `Data` field.
3. If `key_id` hash differs:
   - Creates new encryption key secret with new `key_id` hash
   - migrationController automatically triggers re-encryption
4. If `key_id` hash matches: No action.

**Two hashes tracked:**
- `kmsConfigHash` (annotation) - Detects admin configuration changes
- `kmsKeyIDHash` (data field) - Detects external key rotation

Separate hashes handle scenarios where config changes without key rotation (updating Vault address) or key rotates without config changes (AWS automatic rotation).

#### Variation: Migration Between Encryption Modes

**From aescbc to KMS:**
1. Admin updates APIServer: `type: kms` with KMS configuration.
2. keyController creates KMS secret (empty data, with hash).
3. migrationController re-encrypts resources using external KMS.

**From KMS to aescbc:**
1. Admin updates APIServer: `type: aescbc`.
2. keyController creates aescbc secret (with actual key material).
3. migrationController re-encrypts resources using local AES key.

Migration controller reuses existing logic - no changes required.

### User Stories

- As a cluster admin, I want to enable KMS encryption by updating the APIServer resource, so I can declaratively configure encryption without manually managing keys.
- As a cluster admin, I want the same migration and monitoring experience for KMS as local encryption, so I don't need to learn new procedures.
- As a security admin, I want encryption keys stored outside the cluster, so compromised control plane nodes cannot access keys.

### API Extensions

**APIServer Resource** (`config.openshift.io/v1`):
- Extended with KMS configuration fields ([PR #2035](https://github.com/openshift/api/pull/2035) for AWS KMS)
- Vault KMS fields will be added after finalization

**Encryption Secret Annotations** (library-go):
```go
EncryptionSecretKMSConfigHash = "encryption.apiserver.operator.openshift.io/kms-config-hash"
```
Stores truncated hash (16 hex characters, 8 bytes) of KMS configuration for change detection.

**Encryption State Types** (library-go):
- `KeyState` struct: Add `KMSConfigHash` field
- Add `KMS` mode constant alongside `aescbc`, `aesgcm`, `identity`

### Topology Considerations

#### Hypershift / Hosted Control Planes

The library-go encryption controllers run in the management cluster as part of the hosted control plane operators.
KMS plugin health checks must account for the split architecture where plugins may run in different contexts than the controllers.

#### Standalone Clusters

This enhancement applies to standalone clusters. 
The controllers run in the cluster-kube-apiserver-operator, cluster-openshift-apiserver-operator, and cluster-authentication-operator.

#### Single-node Deployments or MicroShift

Resource consumption impact is minimal - the controllers already exist and are extended with KMS-specific logic.
Single-node deployments will see slightly increased CPU usage during key rotation detection (gRPC Status calls), but this is negligible.

MicroShift may adopt this enhancement if KMS encryption is desired, but the configuration mechanism may differ (file-based vs API resource).

### Implementation Details/Notes/Constraints

**Hash Calculation** (`pkg/operator/encryption/controllers/key_controller.go`):
```go
// Concatenate provider-specific fields
combined := aws.KeyARN + ":" + aws.Region
hash := sha256.Sum256([]byte(combined))
kmsConfigHash := hex.EncodeToString(hash[:])[:16]
```

> **Note:** The hash is truncated to 16 hex characters (8 bytes) to stay within Unix socket path length limits (typically 108 characters) while maintaining sufficient uniqueness for distinguishing different KMS configurations. This allows deterministic socket paths like `/var/run/kmsplugin/kms-a1b2c3d4e5f67890.socket`.

**Reverse Conversion** (stateController reads EncryptionConfiguration from API server pods):
1. Extract hash from socket path: `kms-a1b2c3d4e5f67890.socket` → `a1b2c3d4e5f67890`
2. Look up secret with matching `kms-config-hash` annotation
3. Reconstruct KeyState with original KMS configuration

### Risks and Mitigations

**Risk: KMS Plugin Unavailable During Controller Sync**
- **Impact:** Controllers cannot detect key rotation
- **Mitigation:** No mitigation in Tech Preview. Tech Preview v2 will add health checks and expose it to cluster admin via operator conditions

**Risk: etcd Backup Restoration Without KMS Key Access**
- **Impact:** Cannot decrypt data if KMS key deleted/unavailable/expired
- **Mitigation:** No mitigation in Tech Preview. Document KMS key retention requirements.

### Drawbacks

- Adds complexity to encryption controllers for KMS-specific logic
- AWS KMS requires config changes for rotation (not automatic)
- Dependency on KMS plugin health for controller operations (health checks in Tech Preview v2)

## Test Plan

**Unit Tests:**
- `key_controller_test.go`: KMS key creation, rotation detection, hash changes
- `migration_controller_test.go`: KMS migration scenarios
- `state_controller_test.go`: KMS state changes

**E2E Tests** (Future work):
- Full cluster with KMS encryption enabled
- Trigger external KMS key rotation
- Key rotation with real KMS plugin
- Migration between encryption modes (aescbc → KMS, KMS → identity)
- Verify data re-encryption completes
- Performance testing (time to migrate N secrets)

## Graduation Criteria

### Dev Preview -> Tech Preview

None

### Tech Preview -> GA

- Dynamic `key_id` fetching via KMS plugin Status endpoint
- Full support for key rotation, with automated data re-encryption
- Migration support between different KMS providers, with automated data re-encryption
- Health check preconditions (block operations when plugin unhealthy)
- Support for Thales KMS
- Comprehensive integration and E2E test coverage
- Production validation in multiple environments

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

**Upgrade:**

This feature is gated by TechPreviewNoUpgrade feature gate. Upgrades are not permitted in Tech Preview.

In GA, encryption controllers will handle upgrades seamlessly without requiring manual intervention.

**Downgrade:**

When KMS encryption is enabled and actively used, downgrade is not supported if the previous version lacks KMS support. The API server requires access to encryption keys to decrypt resources stored in etcd.

To downgrade:
1. Migrate from KMS to a supported encryption mode (aescbc or aesgcm or identity)
2. Wait for migration to complete
3. Proceed with cluster downgrade

## Version Skew Strategy

Encryption controllers run in operator pods (not nodes). Version skew concerns:
- **kube-apiserver:** Must support KMS v2 API (Kubernetes 1.27+)
- **library-go:** Operators must use same library-go version
- **KMS plugin:** Controllers don't interact directly (operators do)

No special handling required.

## Operational Aspects of API Extensions

**Monitoring:**
- Operator conditions: `EncryptionControllerDegraded`, `EncryptionMigrationControllerProgressing`, `KMSPluginDegraded`
- Metrics: `apiserver_storage_transformation_operations_total`, `apiserver_storage_transformation_duration_seconds`

**Impact:**
- API latency: +10-50ms per operation (KMS call required, mitigated by DEK caching)
- API throughput: <5% reduction under normal conditions

### Failure Modes

**KMS Plugin Unavailable:**
- New resource creation fails
- Existing resources readable (if DEKs remain cached in API server memory; cache clears on restart)
- Detection: `KMSPluginDegraded=True`
- Recovery: Plugin restart (automatic or manual)

**Invalid KMS Configuration:**
- Plugin fails to start
- Detection: Plugin container crash loops
- Recovery: Fix APIServer configuration

**Key Rotation Stuck:**
- Migration unable to complete
- Detection: `EncryptionMigrationControllerProgressing=True` for extended period
- Recovery: Check migration controller logs, verify KMS health

## Support Procedures

### Detecting KMS Rotation Issues
```bash
# Check encryption key secrets
oc get secrets -n openshift-config-managed -l encryption.apiserver.operator.openshift.io/component=encryption-key

# Check controller logs
oc logs -n openshift-kube-apiserver-operator deployment/kube-apiserver-operator | grep -i kms
```

### Disabling KMS Encryption

1. Update APIServer: `spec.encryption.type: "aescbc"`
2. Wait for migration to complete
3. KMS plugin pods removed by operators

**etcd Backup/Restore:**
- Before backup: Document KMS configuration, verify key availability
- Before restore: Verify KMS key accessible, credentials valid
- Critical: Deleting KMS key makes backups unrestorable

## Alternatives (Not Implemented)

### Alternative: Separate KMS-Specific Controllers

Instead of extending existing controllers, create new KMS-only controllers.

**Why not chosen:**
- Code duplication (migration logic, state management)
- User confusion (different controllers for different encryption types)
- More operational burden (additional monitoring, alerts)


## Infrastructure Needed

None - extends existing library-go code.