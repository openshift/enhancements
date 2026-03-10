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
- Uses existing `config.openshift.io/v1/APIServer` resource `encryption.type` field to enable KMS mode
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

Users deploy KMS plugins manually on all control plane nodes as static pods or systemd units at a predefined socket path (`unix:///var/run/kmsplugin/kms.sock`).
Encryption controllers use the static endpoint in EncryptionConfiguration. KMS-to-KMS migrations are not supported in Tech Preview v1 since only one plugin can listen at the static socket path at a time.

**Tech Preview v2 (Managed Plugin Lifecycle):**

Users specify plugin-specific configuration for managed KMS provider types (e.g. Vault).
From the encryption controllers' perspective, the core logic remains the same; only the tracked fields change.

**Key changes in library-go:**
1. Add KMS mode constant to encryption state types
2. Track KMS configuration in encryption key secrets
3. Manage encryption key secrets with KMS configuration (actual keys are stored externally in KMS provider)
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

**keyController** manages encryption key lifecycle. Creates encryption key secrets in `openshift-config-managed` namespace. For KMS mode, creates secrets storing KMS configuration.

**stateController** generates EncryptionConfiguration for API server consumption. Implements distributed state machine ensuring all API servers converge to same revision.
For KMS mode, generates EncryptionConfiguration using the KMS configuration.

**migrationController** orchestrates resource re-encryption. Marks resources as migrated after rewriting in etcd. Works with all encryption modes including KMS.

**pruneController** prunes inactive encryption key secrets. Maintains N keys (currently 10) for rollback scenarios.

**conditionController** determines when controllers should act. Provides status conditions (`EncryptionInProgress`, `EncryptionCompleted`, `EncryptionDegraded`).

#### Steps for Enabling KMS Encryption (Tech Preview v1)

1. Cluster admin deploys KMS plugin on all control plane nodes (listening at `unix:///var/run/kmsplugin/kms.sock`) as static pod or systemd unit and updates the APIServer resource to enable KMS encryption.
To enable the apiservers to access the KMS plugin, the `/var/run/kmsplugin` directory is mounted as a hostPath volume in all the apiserver pods.
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   spec:
     encryption:
       type: KMS
   ```

2. keyController detects the new encryption mode.

3. keyController creates encryption key secret with KMS configuration:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: openshift-kube-apiserver-encryption-1
     namespace: openshift-config-managed
     annotations:
       encryption.apiserver.operator.openshift.io/mode: "kms"
   data:
     encryption.apiserver.operator.openshift.io-key: "<base64-encoded-kms-config>"
     # Contains base64-encoded structured data with KMS configuration:
     # - Tech Preview v1: Static endpoint path (unix:///var/run/kmsplugin/kms.sock)
     # - Tech Preview v2: Will also include key_id and other plugin-specific configuration for other kms provider types
   ```

4. stateController generates EncryptionConfiguration using the endpoint:
   ```yaml
   apiVersion: apiserver.config.k8s.io/v1
   kind: EncryptionConfiguration
   resources:
     - resources: [configmap]
       providers:
         - kms:
             name: configmap-1
             endpoint: unix:///var/run/kmsplugin/kms.sock
             apiVersion: v2
   ```

5. migrationController detects the new secret and initiates re-encryption (no code changes - works with any mode).

6. conditionController updates status conditions: `EncryptionInProgress`, then `EncryptionCompleted`.

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
- Tracking KMS configuration detects admin configuration changes
- Tracking key_id detects external key rotation

#### Variation: Migration Between Encryption Modes

**From aescbc to KMS:**
1. Admin deploys KMS plugin and updates APIServer: `type: KMS` with KMS configuration.
2. keyController creates KMS secret (empty data, with KMS configuration annotation).
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

**Current Behavior:**

The `encryption.type` field already supports the `KMS` value ([EncryptionType](https://github.com/openshift/api/blob/master/config/v1/types_apiserver.go#L214)), and the `KMSConfig` struct exists in the API.
These fields are gated by the `KMSEncryptionProvider` feature gate (DevPreviewNoUpgrade, TechPreviewNoUpgrade).
However, the encryption controllers do not implement KMS support. Enabling `KMSEncryptionProvider` feature gate and setting `type: KMS` have no effect - controllers ignore it and no encryption occurs.

**Tech Preview V1**

For Tech Preview v1, no new API fields are added to the APIServer resource.
Users simply set `encryption.type: KMS` ([EncryptionType](https://github.com/openshift/api/blob/6fb7fdae95fd20a36809d502cfc0e0459550d527/config/v1/types_apiserver.go#L214))
and deploy KMS plugins at the hardcoded endpoint `unix:///var/run/kmsplugin/kms.sock`. Current `KMSConfig` will not be used.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift has a parallel implementation that supports AESCBC and KMS without using the encryption controllers in library-go. 
Unifying the two implementations is out of scope for this enhancement.

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

**Unit Tests**:
- `key_controller_test.go`: KMS key creation, rotation detection, endpoint changes
- `migration_controller_test.go`: KMS migration scenarios
- `state_controller_test.go`: KMS state changes

**Integration Tests**:
- State transitions in encryption controllers in library-go
- Explore MOM framework for integration tests in apiserver operators (add tests if it makes sense)

**E2E Tests** (v1):
- Migration between identity ↔ KMS 

**E2E Tests** (v2):
- Full cluster with KMS encryption enabled
- Migration between encryption modes (aescbc → KMS, KMS → KMS)
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