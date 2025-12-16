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

Extend OpenShift encryption controllers to support external Key Management Services (KMS v2) alongside existing local encryption modes (aescbc, aesgcm). This allows encryption keys to be stored and managed outside the cluster for enhanced security.

This enhancement:
- Extends the `config.openshift.io/v1/APIServer` resource for KMS configuration
- Extends encryption controllers in `openshift/library-go` to support KMS as a new encryption mode
- Maintains feature parity with existing encryption modes (migration, monitoring, key rotation)
- Provider-agnostic implementation supporting any KMS v2-compatible plugin

## Motivation

OpenShift currently manages AES keys locally for encrypting data at rest in etcd. KMS support enables integration with external key management systems where encryption keys are stored outside the cluster, protecting against attacks where control plane nodes are compromised.

### Goals

- Support KMS v2 as a new encryption mode in existing encryption controllers
- Seamless migration between encryption modes (aescbc ↔ KMS, KMS ↔ KMS)
- Provider-agnostic controller implementation with minimal provider-specific code
- Feature parity with existing modes (monitoring, migration, key rotation)

### Non-Goals

- Implementing KMS plugins (provided by upstream Kubernetes/vendors)
- KMS plugin deployment/lifecycle management
- KMS plugin health checks (Tech Preview v2)
- Recovery from KMS key loss (separate EP for GA)
- Automatic `key_id` rotation detection (Tech Preview v2)
- API Servers communicate with KMS v2 Plugins via abstract unix sockets (i.e. unix:///@foo) (require `hostNetwork=true` which openshift-apiserver and oauth-apiserver do not have)

## Proposal

Extend the existing encryption controller framework in `openshift/library-go` to support KMS encryption using user-provided unix socket paths. Users specify the socket path where their KMS plugin listens, and encryption controllers use this path directly in the EncryptionConfiguration. For KMS-to-KMS migrations, users update to a new socket path, allowing both old and new plugins to run simultaneously during migration.

**Key changes:**
1. Add KMS mode constant to encryption state types
2. Track unix socket paths in encryption key secrets
3. Manage encryption key secrets with socket path metadata (actual keys in external KMS)
4. Reuse existing migration controller (no changes needed)

**Tech Preview v2 additions:**
- Poll KMS plugin Status endpoint for `key_id` changes
- Store `key_id` in data field of encryption key secrets
- Detect external key rotation via `key_id` comparison

### Workflow Description

#### Actors in the Workflow

**cluster admin** is a human user responsible for configuring and maintaining the cluster.

**KMS** is the external Key Management Service that stores and manages the Key Encryption Key (KEK).

**KMS plugin** is a gRPC service implementing Kubernetes KMS v2 API, running as a static pod on each control plane node. It communicates with the external KMS to encrypt/decrypt data encryption keys (DEKs).

**API server operator** is the OpenShift operator (kube-apiserver-operator, openshift-apiserver-operator, or authentication-operator) managing API server deployments.

#### Encryption Controllers

**keyController** manages encryption key lifecycle. Creates encryption key secrets in `openshift-config-managed` namespace. For KMS mode, creates secrets storing the unix socket path.

**stateController** generates EncryptionConfiguration for API server consumption. Implements distributed state machine ensuring all API servers converge to same revision. For KMS mode, generates configuration with user-provided unix socket paths.

**migrationController** orchestrates resource re-encryption. Marks resources as migrated after rewriting in etcd. Works with all encryption modes including KMS.

**pruneController** prunes inactive encryption key secrets. Maintains N keys (currently 10) for rollback scenarios.

**conditionController** determines when controllers should act. Provides status conditions (`EncryptionInProgress`, `EncryptionCompleted`, `EncryptionDegraded`).

#### Steps for Enabling KMS Encryption

1. Cluster admin deploys KMS plugin and updates the APIServer resource with the socket endpoint:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   spec:
     encryption:
       type: kms
       kms:
         type: External
         endpoint: unix:///var/run/kmsplugin/socket.sock
   ```

2. keyController detects the new encryption mode and reads the endpoint from KMS configuration.

3. keyController creates encryption key secret storing the endpoint:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: openshift-kube-apiserver-encryption-1
     namespace: openshift-config-managed
     annotations:
       encryption.apiserver.operator.openshift.io/mode: "kms"
       encryption.apiserver.operator.openshift.io/kms-endpoint: "unix:///var/run/kmsplugin/socket.sock"
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
             endpoint: unix:///var/run/kmsplugin/socket.sock
             apiVersion: v2
   ```

5. migrationController detects the new secret and initiates re-encryption (no code changes - works with any mode).

6. Resources are re-encrypted via the KMS plugin. The KMS plugin communicates with the external KMS to encrypt/decrypt a SEED using the KEK. The SEED, combined with a random value, generates the actual encryption keys used for resource encryption (KMS v2 protocol).

7. conditionController updates status conditions: `EncryptionInProgress`, then `EncryptionCompleted`.

#### Variation: Endpoint Changes (KMS-to-KMS Migration)

When cluster admin updates the KMS endpoint (e.g., switching to a different KMS plugin):

1. Admin deploys new KMS plugin at a different socket path and updates APIServer resource:
   ```yaml
   spec:
     encryption:
       type: kms
       kms:
         type: External
         endpoint: unix:///var/run/kmsplugin/socket-new.sock  # Changed from socket.sock
   ```

2. keyController reads the new endpoint from updated APIServer resource.
3. Compares new endpoint with endpoint in most recent encryption key secret annotation.
4. If endpoints differ:
   - Creates new encryption key secret with new endpoint
   - migrationController automatically triggers re-encryption
   - **Both old and new KMS plugins must remain running during migration**
5. If endpoints match: No action.

**Note:** Automatic weekly key rotation (used for aescbc/aesgcm) is disabled for KMS since rotation is triggered externally.

#### Variation: External KMS Key Rotation (Tech Preview v2)

When external KMS rotates the key internally:

1. keyController polls KMS plugin Status endpoint for `key_id`.
2. Compares `key_id` with `key_id` stored in secret `Data` field.
3. If `key_id` differs:
   - Creates new encryption key secret with new `key_id`
   - migrationController automatically triggers re-encryption
4. If `key_id` matches: No action.

> **Note:** API server operators are not privileged and cannot directly communicate with KMS plugins running as static pods on control plane nodes. Tech Preview v2 will require introducing a mechanism to poll KMS plugin Status endpoints for `key_id` changes and health monitoring, and expose this information to the operators.

**Two change detection mechanisms:**
- `kms-endpoint` (annotation) - Detects admin configuration changes (endpoint updates)
- `key_id` (data field) - Detects external key rotation

Separate tracking handles scenarios where endpoint changes (switching KMS plugins) or key rotates within the same KMS.

#### Variation: Migration Between Encryption Modes

**From aescbc to KMS:**
1. Admin deploys KMS plugin and updates APIServer: `type: kms` with endpoint.
2. keyController creates KMS secret (empty data, with endpoint annotation).
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
- Extended with KMS configuration fields ([PR #2622](https://github.com/openshift/api/pull/2622))

```go
type KMSProviderType string

const (
    ExternalKMSProvider KMSProviderType = "External"
)

// KMSConfig defines the configuration for the KMS instance
// that will be used with KMSEncryptionProvider encryption
type KMSConfig struct {
// managementModel defines how KMS plugins are managed.
// Valid values are "External".
// When set to External, encryption keys are managed by a user-deployed
// KMS plugin that communicates via UNIX domain socket using KMS V2 API.
//
// +kubebuilder:validation:Enum=External
// +kubebuilder:default=External
// +optional
ManagementModel ManagementModel `json:"managementModel,omitempty"`

// endpoint specifies the UNIX domain socket endpoint for communicating with the external KMS plugin.
// The endpoint must follow the format "unix:///path".
// Abstract Linux sockets (i.e. "unix:///@abstractname") are not supported.
//
// +kubebuilder:validation:MaxLength=120
// +kubebuilder:validation:MinLength=9
// +kubebuilder:validation:XValidation:rule="self.matches('^unix:///[^@ ][^ ]*$')",message="endpoint must follow the format 'unix:///path'"
// +required
Endpoint string `json:"endpoint,omitempty"`
}
```

> **Breaking Change:** This removes the AWS-specific fields (region, keyArn, etc.) from earlier iterations of KMSConfig. This is acceptable because: (1) no controllers are consuming these fields, (2) the feature is gated behind the KMSEncryptionProvider feature gate in Tech Preview, which provides no stability guarantees.

**Encryption Secret Annotations** (library-go):
```go
EncryptionSecretKMSEndpoint = "encryption.apiserver.operator.openshift.io/kms-endpoint"
```
Stores the unix socket endpoint for the KMS plugin.

**Encryption State Types** (library-go):
- `KeyState` struct: Add `KMSEndpoint` field
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

**Reverse Conversion** (stateController reads EncryptionConfiguration from API server pods):
1. Extract endpoint from EncryptionConfiguration: `unix:///var/run/kmsplugin/socket.sock`
2. Look up secret with matching `kms-endpoint` annotation
3. Reconstruct KeyState with the endpoint

### Risks and Mitigations

**Risk: KMS Plugin Unavailable During Controller Sync**
- **Impact:** Controllers cannot detect key rotation
- **Mitigation:** No mitigation in Tech Preview. Tech Preview v2 will add health checks and expose it to cluster admin via operator conditions

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
- Trigger external KMS key rotation
- Key rotation with real KMS plugin
- Migration between encryption modes (aescbc → KMS, KMS → KMS, KMS → identity)
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
- Comprehensive integration and E2E test coverage
- Production validation in multiple environments

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

**Upgrade:**

This feature is gated by TechPreviewNoUpgrade feature gate. Upgrades are not permitted in Tech Preview.

> **Important:** KMS plugins run as independent static pods with no ordering guarantees relative to kube-apiserver startup. If kube-apiserver starts before the KMS plugin is ready, it will fail to decrypt resources encrypted with KMS and will crash-loop until the plugin becomes available. This is expected behavior - the kube-apiserver will automatically recover once the KMS plugin is running and accessible at the configured endpoint.

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