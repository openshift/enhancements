---
title: kms-encryption-foundations
authors:
  - "@ardaguclu"
reviewers:
  - "@flavianmissi"
  - "@ibihim"
approvers:
  - "@benluddy"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-12-03
last-updated: 2026-01-08
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

Extend OpenShift encryption controllers to support external Key Management Services (KMS v2) alongside existing local encryption modes (aescbc, aesgcm). 
This allows encryption keys to be stored and managed outside the cluster for enhanced security.

This enhancement:
- Extends the `config.openshift.io/v1/APIServer` resource for KMS configuration
- Extends encryption controllers in `openshift/library-go` to support KMS as a new encryption mode
- Maintains feature parity with existing encryption modes (migration, monitoring, key rotation)
- Provider-agnostic implementation supporting any KMS v2-compatible plugin

## Motivation

OpenShift currently manages AES keys locally for encrypting data at rest in etcd. 
KMS support enables integration with external key management systems where encryption keys are stored outside the cluster, protecting against attacks where control plane nodes are compromised.

### Goals

- Support KMS v2 as a new encryption mode in existing encryption controllers
- Seamless migration between encryption modes (aescbc ↔ KMS, KMS ↔ KMS)
- Provider-agnostic implementation with minimal provider-specific code
- Feature parity with existing modes (monitoring, migration, key rotation)

### Non-Goals

- Implementing KMS plugins (provided by upstream Kubernetes/vendors)
- KMS plugin deployment/lifecycle management
- KMS plugin health checks (Tech Preview v2)
- Recovery from KMS key loss (separate EP for GA)
- Automatic `key_id` rotation detection (Tech Preview v2)

## Proposal

Extend the existing encryption controller framework in `openshift/library-go` to support KMS encryption in two phases:

**Tech Preview v1 (External Plugin Management):**

Users deploy KMS plugins manually as static pods (e.g. `unix:///var/run/kmsplugin/my-plugin.sock`) and specify a plugin name (e.g. `my-plugin`). The plugin name is templated into a Unix socket path (`unix:///var/run/kmsplugin/<name>.sock`).
Encryption controllers track the endpoint and use it in EncryptionConfiguration. For KMS-to-KMS migrations, users update the plugin name, allowing both old and new plugins to run simultaneously during migration.

**Tech Preview v2 (Managed Plugin Lifecycle):**

Users specify plugin-specific configuration (details TBD). The ManualKMSConfig struct and Manual provider type will be dropped in favor of managed KMS provider types (e.g. Vault).
From the encryption controllers' perspective, the core logic remains the same; only the tracked fields change.

**Key changes in library-go:**
1. Add KMS mode constant to encryption state types
2. Track KMS configuration in encryption key secrets (v1: plugin name, v2: plugin-specific config)
3. Manage encryption key secrets with KMS metadata (actual keys are stored externally in KMS provider)
4. Detect configuration changes to trigger migration
5. Reuse existing migration controller (no changes needed)

**Additional Tech Preview v2 capabilities:**
- Poll KMS plugin Status endpoint for health checks and `key_id` changes to detect external key rotation

### Workflow Description

#### Actors in the Workflow

**cluster admin** is a human user responsible for configuring and maintaining the cluster.

**KMS** is the external Key Management Service that stores and manages the Key Encryption Key (KEK).

**KMS plugin** is a gRPC service implementing Kubernetes KMS v2 API, running as a static pod on each control plane node. It communicates with the external KMS to encrypt/decrypt data encryption keys (DEKs).

**API server operator** is the OpenShift operator (kube-apiserver-operator, openshift-apiserver-operator, or authentication-operator) managing API server deployments.

#### Encryption Controllers

**keyController** manages encryption key lifecycle. Creates encryption key secrets in `openshift-config-managed` namespace. For KMS mode, creates secrets storing KMS configuration metadata.

**stateController** generates EncryptionConfiguration for API server consumption. Implements distributed state machine ensuring all API servers converge to same revision.
For KMS mode, generates EncryptionConfiguration based on the KMS metadata given in APIServer resource.

**migrationController** orchestrates resource re-encryption. Marks resources as migrated after rewriting in etcd. Works with all encryption modes including KMS.

**pruneController** prunes inactive encryption key secrets. Maintains N keys (currently 10) for rollback scenarios.

**conditionController** determines when controllers should act. Provides status conditions (`EncryptionInProgress`, `EncryptionCompleted`, `EncryptionDegraded`).

#### Steps for Enabling KMS Encryption (Tech Preview v1)

1. Cluster admin deploys KMS plugin as static pod (listening at `unix:///var/run/kmsplugin/my-plugin.sock`) and updates the APIServer resource with the socket name:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   spec:
     encryption:
       type: kms
       kms:
         type: Manual
         manual:
           name: my-plugin
   ```

2. keyController detects the new encryption mode and reads the KMS metadata from APIServer resource.

3. keyController creates encryption key secret storing the endpoint:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: openshift-kube-apiserver-encryption-1
     namespace: openshift-config-managed
     annotations:
       encryption.apiserver.operator.openshift.io/mode: "kms"
       encryption.apiserver.operator.openshift.io/kms-name: "my-plugin"
   data:
     keys: ""  # Empty in Tech Preview - KEK stored in external KMS
              # In Tech Preview v2, will contain base64-encoded key_id
   ```

4. stateController generates EncryptionConfiguration using the user-provided endpoint:
   ```yaml
   apiVersion: apiserver.config.k8s.io/v1
   kind: EncryptionConfiguration
   resources:
     - resources: [configmap]
       providers:
         - kms:
             name: configmap-1
             endpoint: unix:///var/run/kmsplugin/my-plugin.sock
             apiVersion: v2
   ```

5. migrationController detects the new secret and initiates re-encryption (no code changes - works with any mode).

6. conditionController updates status conditions: `EncryptionInProgress`, then `EncryptionCompleted`.

#### Variation: Plugin Name Changes (KMS-to-KMS Migration, Tech Preview v1)

When cluster admin updates the KMS plugin name (e.g., switching to a different KMS plugin):

1. Admin deploys new KMS plugin at a different socket path (listening at `unix:///var/run/kmsplugin/my-new-plugin.sock`) and updates APIServer resource:
   ```yaml
   spec:
     encryption:
       type: kms
       kms:
         type: Manual
         manual:
           name: my-new-plugin
   ```

2. keyController reads the new KMS metadata from updated APIServer resource.
3. Compares new metadata with metadata in most recent encryption key secret annotation.
4. If metadata differs:
   - Creates new encryption key secret with new metadata
   - migrationController automatically triggers re-encryption
   - **Both old and new KMS plugins must remain running during migration**
5. If metadata matches: No action.

**Note:** Automatic weekly key rotation (used for aescbc/aesgcm) is disabled for KMS since rotation is triggered externally.

#### Variation: KMS Key Rotation (Tech Preview v2)

When external KMS rotates the key internally:

1. keyController polls KMS plugin Status endpoint for `key_id`.
2. Compares `key_id` with `key_id` stored in secret `Data` field.
3. If `key_id` differs:
   - Creates new encryption key secret with new `key_id`
   - migrationController automatically triggers re-encryption
4. If `key_id` matches: No action.

> **Note:** API server operators are not privileged and cannot directly communicate with KMS plugins running as static pods on control plane nodes. 
> Tech Preview v2 will require introducing a mechanism to poll KMS plugin Status endpoints for `key_id` changes and health monitoring, and expose this information to the operators.

**Two change detection mechanisms:**
- Tracking KMS metadata detects admin configuration changes
- Tracking key_id detects external key rotation

#### Variation: Migration Between Encryption Modes

**From aescbc to KMS:**
1. Admin deploys KMS plugin and updates APIServer: `type: kms` with KMS metadata.
2. keyController creates KMS secret (empty data, with KMS metadata annotation).
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

**APIServer Resource** ([config.openshift.io/v1](https://github.com/openshift/api/blob/master/config/v1/types_kmsencryption.go)):

**Tech Preview V1**

```go
// KMSConfig defines the configuration for the KMS instance
// that will be used with KMSEncryptionProvider encryption
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'Manual' ? (has(self.manual) && has(self.manual.name) && self.manual.name != '') : !has(self.manual)",message="manual config with non-empty name is required when kms provider type is Manual, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'AWS' ?  has(self.aws) : !has(self.aws)",message="aws config is required when kms provider type is AWS, and forbidden otherwise"
// +union
type KMSConfig struct {
// type defines the kind of platform for the KMS provider.
// Available provider types are AWS, Manual.
//
// +unionDiscriminator
// +required
Type KMSProviderType `json:"type"`

// manual defines the configuration for manually managed KMS plugins.
// The KMS plugin must be deployed as a static pod by the cluster admin.
//
// +unionMember
// +optional
Manual *ManualKMSConfig `json:"manual,omitempty"`

// aws defines the key config for using an AWS KMS instance
// for the encryption. The AWS KMS instance is managed
// by the user outside the purview of the control plane.
// Deprecated: There is no logic listening to this resource type, we plan to remove it in next release.
//
// +unionMember
// +optional
AWS *AWSKMSConfig `json:"aws,omitempty"`
}

// ManualKMSConfig defines the configuration for manually managed KMS plugins
type ManualKMSConfig struct {
// name specifies the KMS plugin name.
// This name is templated into the UNIX domain socket path: unix:///var/run/kmsplugin/<name>.sock
// and is between 1 and 80 characters in length.
// The KMS plugin must listen at this socket path.
// The name must be a safe socket filename and must not contain '/' or '..'.
//
// +kubebuilder:validation:MaxLength=80
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:XValidation:rule="!self.contains('/') && !self.contains('..')",message="name must be a safe socket filename (must not contain '/' or '..')"
// +optional
Name string `json:"name,omitempty"`
}
```

**Tech Preview V2**

In Tech Preview v2, Manual type support is dropped in favor of managed plugin lifecycle.
Vault is introduced as the first managed KMS provider type.
The ManualKMSConfig struct is removed entirely as manual plugin management is no longer supported.

```go
// KMSConfig defines the configuration for the KMS instance
// that will be used with KMSEncryptionProvider encryption
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'Vault' ?  has(self.vault) : !has(self.vault)",message="vault config is required when kms provider type is Vault, and forbidden otherwise"
// +union
type KMSConfig struct {
// type defines the kind of platform for the KMS provider.
// Valid values are "Vault".
//
// +unionDiscriminator
// +required
Type KMSProviderType `json:"type"`

// vault defines the configuration for using HashiCorp Vault as KMS provider.
//
// +unionMember
// +optional
Vault *VaultKMSConfig `json:"vault,omitempty"`
}
```

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

#### OpenShift Kubernetes Engine

This feature does not depend on the features that are excluded from the OKE product offering.

### Implementation Details/Notes/Constraints

### Risks and Mitigations

**Risk: KMS Plugin Unavailable During Controller Sync**
- **Impact:** Controllers cannot detect key rotation
- **Mitigation:** No mitigation in Tech Preview. Tech Preview v2 will add health checks and expose it to cluster admin via operator conditions to degrade

**Risk: etcd Backup Restoration Without KMS Key Access**
- **Impact:** Cannot decrypt data if KMS key deleted/unavailable/expired
- **Mitigation:** No mitigation in Tech Preview. Document KMS key retention requirements.

### Drawbacks

- Adds complexity to encryption controllers for KMS-specific logic
- Dependency on KMS plugin health for controller operations (health checks in Tech Preview v2)

## Test Plan

**Unit Tests:**
- `key_controller_test.go`: KMS key creation, rotation detection, endpoint changes
- `migration_controller_test.go`: KMS migration scenarios
- `state_controller_test.go`: KMS state changes

**E2E Tests** (Future work):
- Full cluster with KMS encryption enabled
- Migration between encryption modes (aescbc → KMS, KMS → KMS, KMS → identity)
- Verify data re-encryption completes

## Graduation Criteria

### Dev Preview -> Tech Preview

None

### Tech Preview -> GA

- Dynamic `key_id` fetching via KMS plugin Status endpoint
- Full support for key rotation, with automated data re-encryption
- Migration support between different KMS providers, with automated data re-encryption
- Health check preconditions (block operations when plugin unhealthy)
- Comprehensive integration and E2E test coverage
- Production validation in multiple environments

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

**Upgrade:**

This feature is gated by TechPreviewNoUpgrade feature gate. Upgrades are not permitted in Tech Preview.

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
- **KMS plugin:** No version skew concerns - plugins communicate with apiservers via the standardized KMS v2 API contract, ensuring compatibility regardless of plugin version

No special handling required.

## Operational Aspects of API Extensions

**Monitoring:**
- Operator conditions: `EncryptionControllerDegraded`, `EncryptionMigrationControllerProgressing`, `KMSPluginDegraded`
- Metrics: `apiserver_storage_transformation_operations_total`, `apiserver_storage_transformation_duration_seconds`

**Impact:**
- API latency: KMS call required, mitigated by DEK caching
- API throughput: minor reduction under normal conditions

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