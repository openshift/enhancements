---
title: kms-encryption-foundations
authors:
  - "@ardaguclu"
reviewers:
  - "@p0lyn0mial"
  - "@bertinatto" # for plugin lifecycle
  - "@flavianmissi" # for API alignment
approvers:
  - "@benluddy"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-12-03
last-updated: 2026-03-17
tracking-link:
  - "https://redhat.atlassian.net/browse/CNTRLPLANE-243"
see-also:
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
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

**Tech Preview v1 — Goals:**
- Support KMS v2 as a new encryption mode in existing encryption controllers
- Provider-agnostic implementation with minimal provider-specific code
- Migration between identity ↔ KMS

**Tech Preview v2 — Goals:**
- Split KMS configuration into kms-encryption-config, kms-provider-config, kms-secret-data, and kms-configmap-data
- Seamless migration between encryption modes (aescbc ↔ KMS, KMS ↔ KMS)
- Field-level comparison to distinguish migration-requiring vs. in-place changes
- Pre-flight checks before generating new encryption keys
- Credential/ConfigMap validation with degraded status reporting
- Periodic sync of referenced Secrets and ConfigMaps to all active key secrets
- KMS plugin deployment/lifecycle management (covered by a separate EP)
- API field definitions for KMS provider configuration in APIServer resource (covered by a [separate EP](https://github.com/openshift/enhancements/pull/1954))

**Tech Preview v3 — Goals:**
- Report current KMS encryption status to platform users (e.g., active KMS plugins, migration progress)
- Automatic `key_id` rotation detection
- KMS plugin health checks
- Removal of unused KMS plugins from EncryptionConfiguration after migration completes
- Support updating the KMS timeout field via `unsupportedConfigOverrides`

**GA — Goals:**
- Failure mode coverage (detection + mitigation for each):
  - Misconfiguration of the KMS plugin
  - Loss of access to the KMS service
  - Loss of credentials

### Non-Goals

- Implementing KMS plugins (provided by upstream Kubernetes/vendors)
- Recovery from KMS key loss (see [KMS Key Loss Considerations](#kms-key-loss-considerations) for details)

## Proposal

Extend the existing encryption controller framework in `openshift/library-go` to support KMS encryption in two phases:

**Tech Preview v1 (External Plugin Management):**

Users deploy KMS plugins manually on all control plane nodes as static pods or systemd units at a predefined socket path (`unix:///var/run/kmsplugin/kms.sock`).
Encryption controllers use the static endpoint in EncryptionConfiguration. KMS-to-KMS migrations are not supported in Tech Preview v1 since only one plugin can listen at the static socket path at a time.

**Tech Preview v2 (Managed Plugin Lifecycle):**

Users specify plugin-specific configuration for managed KMS provider types (e.g. Vault) via the APIServer resource (API fields covered by a separate EP).
Encryption controllers split the KMS configuration API into multiple parts stored atomically in encryption key secrets:

1. `kms-encryption-config` — structured Kubernetes KMS v2 provider configuration used to generate the EncryptionConfiguration provider entry (apiVersion: v2, name, endpoint, timeout)
2. `kms-provider-config` — serialized `KMSConfig` resource ([config.openshift.io/v1](https://github.com/openshift/api/blob/master/config/v1/types_kmsencryption.go)), giving consumers access to provider-specific configuration (image, vault-address, transit-mount, transit-key, etc.)
3. `kms-secret-{key}-{keyID}` — individual keys from the referenced Secret are stored as separate entries (e.g., `kms-secret-id-1`, `kms-secret-login-1`, `kms-secret-password-1` for Vault approle credentials)
4. `kms-configmap-{key}-{keyID}` — individual keys from the referenced ConfigMap are stored as separate entries (e.g., `kms-configmap-ca-1` for CA bundles)

   For example, an encryption-configuration secret with this layout:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: encryption-config-kube-apiserver-9
   data:
     kms-provider-config-1: |
       address: bar
       ...
     kms-secret-id-1: VALUE
     kms-secret-login-1: VALUE
     kms-secret-password-1: VALUE
     kms-configmap-ca-1: VALUE
   ```

Storing all related data in a single secret avoids race conditions caused by reading live, independently changing configuration.
In kas-o, the targetConfigController operates on live data and may generate a manifest based on the current sidecar configuration. However, this configuration can change before the RevisionController creates a revision. 
As a result, the generated manifest may no longer match the actual configuration state at the time the revision is created. Keeping all dependent configuration in a single secret ensures consistency and guarantees that both controllers operate on the same, atomic snapshot of data.

Additionally, consolidating the data in a single secret leverages existing revisioning and cleanup mechanisms.
The keyID is appended to the UDS path (`unix:///var/run/kmsplugin/kms-{keyID}.sock`) to ensure uniqueness among providers, enabling KMS-to-KMS migrations with multiple concurrent plugins.

**Key changes in library-go:**
1. Add KMS mode constant and track KMS configuration in encryption key secrets
2. Split configuration into kms-encryption-config, kms-provider-config, kms-secret-data, and kms-configmap-data; copy with keyID suffix to encryption-configuration secrets (Tech Preview v2)
3. Field-level comparison, credential/ConfigMap validation, and periodic sync of referenced resources to all active key secrets (Tech Preview v2)
4. Reuse existing migration controller (no changes needed)

### Workflow Description

#### Actors in the Workflow

**cluster admin** is a human user responsible for configuring and maintaining the cluster.

**KMS** is the external Key Management Service that stores and manages the Key Encryption Key (KEK).

**KMS plugin** is a gRPC service implementing Kubernetes KMS v2 API. In Tech Preview v1, it runs as a static pod on each control plane node. In Tech Preview v2, it runs as a sidecar container alongside with API Servers (kube-apiserver, oauth-apiserver, openshift-apiservers) managed by the APIServer operators. It communicates with the external KMS to encrypt/decrypt data encryption keys .

**API server operator** is the OpenShift operator (kube-apiserver-operator, openshift-apiserver-operator, or authentication-operator) managing API server deployments.

#### Encryption Controllers

**keyController** manages encryption key lifecycle. Creates encryption key secrets in `openshift-config-managed` namespace. For KMS mode, creates secrets storing KMS configuration.
For Tech Preview v2, also propagates updates from the API configuration, splits configuration into `kms-encryption-config`, `kms-provider-config`, `kms-secret-data`, and `kms-configmap-data`, performs field-level comparison, validates credential secrets, and periodically syncs referenced Secrets/ConfigMaps to all active key secrets.

**stateController** generates EncryptionConfiguration for API server consumption. Implements distributed state machine ensuring all API servers converge to same revision.
For KMS mode, generates EncryptionConfiguration using the KMS configuration.
For Tech Preview v2, also copies `kms-provider-config`, `kms-secret-data`, and `kms-configmap-data` with keyID suffix (e.g., `kms-provider-config-1`, `kms-secret-data-1`, `kms-configmap-data-1`) to the encryption-configuration secret.

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
     name: encryption-key-kube-apiserver-1
     namespace: openshift-config-managed
     annotations:
       encryption.apiserver.operator.openshift.io/mode: "KMS"
   data:
     encryption.apiserver.operator.openshift.io-key: "<base64-encoded-kms-encryption-config>"
     # Contains base64-encoded structured data with KMS configuration:
     # - Tech Preview v1: Static endpoint path (unix:///var/run/kmsplugin/kms.sock)
     # - Tech Preview v2: kms-encryption-config and kms-provider-config (see Tech Preview v2 section below)
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

#### Steps for Enabling KMS Encryption (Tech Preview v2)

1. Cluster admin configures KMS provider in the APIServer resource (API fields covered by a separate EP):
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   spec:
     encryption:
       type: KMS
      # Vault API specific fields
   ```

2. keyController detects the configuration, splits it into `kms-encryption-config`, `kms-provider-config`, `kms-secret-data`, and `kms-configmap-data`, and creates an encryption key secret:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: encryption-key-kube-apiserver-1
     namespace: openshift-config-managed
     annotations:
       encryption.apiserver.operator.openshift.io/mode: "KMS"
   type: Opaque
   data:
     kms-encryption-config: <base64-encoded kms-encryption-config>
     kms-provider-config: <base64-encoded sidecar container config>
     kms-secret-data: <base64-encoded credential data>
     kms-configmap-data: <base64-encoded configmap data>
   ```

3. stateController uses `kms-encryption-config` to generate the EncryptionConfiguration (with keyID in the endpoint and provider name):
   ```yaml
   apiVersion: apiserver.config.k8s.io/v1
   kind: EncryptionConfiguration
   resources:
     - resources:
         - secrets
       providers:
         - kms:
             apiVersion: v2
             name: kms-1_secrets
             endpoint: unix:///var/run/kmsplugin/kms-1.sock
             timeout: 10s
   ```

4. stateController copies `kms-provider-config`, `kms-secret-data`, and `kms-configmap-data` with keyID suffix to the encryption-configuration secret:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: encryption-config-kube-apiserver-9
     namespace: openshift-kube-apiserver
   type: Opaque
   data:
     encryption-config: <EncryptionConfiguration>
     kms-provider-config-1: <base64-encoded sidecar config for keyID 1>
     kms-secret-data-1: <base64-encoded credentials for keyID 1>
     kms-configmap-data-1: <base64-encoded configmap data for keyID 1>
   ```

5. The encryption-configuration secret is revisioned, triggering a new rollout. The respective operator configures sidecars accordingly.

6. migrationController initiates re-encryption (no code changes - works with any mode).

7. conditionController updates status conditions: `EncryptionInProgress`, then `EncryptionCompleted`.

For first-time KMS enablement, keyController runs pre-flight checks by deploying a pod with the KMS plugin to verify status and encrypt/decrypt capability before generating the first encryption key.

#### Variation: Updates Requiring Migration (Tech Preview v2)

If a field affecting the KEK is changed (**vault-address**, **vault-namespace**, **transit-key**, **transit-mount**), keyController creates a new encryption key secret with the next keyID (see [Preconditions for Configuration Changes](#preconditions-for-configuration-changes-tech-preview-v2) for invariants and pre-flight checks that apply before a new key is generated).

stateController generates an EncryptionConfiguration with both providers — new as write key, old as read key:

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - kms:
          apiVersion: v2
          name: kms-2_secrets
          endpoint: unix:///var/run/kmsplugin/kms-2.sock
          timeout: 10s
      - kms:
          apiVersion: v2
          name: kms-1_secrets
          endpoint: unix:///var/run/kmsplugin/kms-1.sock
          timeout: 10s
```

stateController copies kms-provider-config, kms-secret-data, and kms-configmap-data from both encryption key secrets into the encryption-configuration secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: encryption-config-kube-apiserver-9
  namespace: openshift-kube-apiserver
data:
  encryption-config: <EncryptionConfiguration>
  kms-provider-config-1: <base64-encoded sidecar config for keyID 1>
  kms-provider-config-2: <base64-encoded sidecar config for keyID 2>
  kms-secret-data-1: <base64-encoded credentials for keyID 1>
  kms-secret-data-2: <base64-encoded credentials for keyID 2>
  kms-configmap-data-1: <base64-encoded configmap data for keyID 1>
  kms-configmap-data-2: <base64-encoded configmap data for keyID 2>
```

Both providers run as separate sidecar containers with different unix domain sockets (kms-1.sock, kms-2.sock).

#### Variation: Updates Not Requiring Migration (Tech Preview v2)

Fields that only affect the container spec (e.g., image for CVE fixes) do not change the KEK:

1. keyController updates the existing encryption key secret in-place. No new secret is created.
2. stateController detects the change and triggers a new revision with the updated `kms-provider-config`.

Only the active provider receives the update. Older providers retain their original sidecar configuration as fallback.

#### Variation: Disabling KMS Encryption (Tech Preview v2)

When the user sets the encryption mode to identity, keyController creates a new encryption key secret for identity mode. The EncryptionConfiguration contains identity as write provider and the KMS plugin as read provider until migration completes.

After migration, the unused KMS plugin is removed from EncryptionConfiguration. This is important because leaving stale providers in EncryptionConfiguration means the API server will continue attempting to connect to the old KMS plugin at startup, blocking readiness if the plugin is no longer available. Status conditions notify the admin that the KMS plugin can be safely decommissioned. Backups encrypted with the previous KMS plugin are not restorable without access to that plugin. The removal mechanism is out of scope in Tech Preview v2.

#### Variation: Migration from KMS Plugin A to KMS Plugin B (Tech Preview v2)

keyController creates a new encryption key secret with the new plugin's configuration. stateController generates an EncryptionConfiguration with both providers — new as write key, old as read key. Both run as separate sidecars until migration completes. 

#### Variation: Migration Between KMS and Static Encryption (Tech Preview v2)

**From KMS to static encryption (aesgcm/aescbc):**
keyController creates a new encryption key secret for the static mode. EncryptionConfiguration contains static as write provider and KMS as read provider until migration completes. The KMS plugin must remain accessible during migration.

**From static encryption to KMS:**
keyController creates a new encryption key secret with KMS configuration. EncryptionConfiguration contains KMS as write key and static provider as read key.

#### Variation: KMS Plugin A to Identity to KMS Plugin A (Tech Preview v2)

Even with identical plugin configuration, keyController creates a new encryption key secret with the next keyID (e.g., keyID 3 vs original keyID 1). stateController generates an EncryptionConfiguration with kms-3 as write key, identity and kms-1 as read providers:

```yaml
apiVersion: apiserver.config.k8s.io/v1
kind: EncryptionConfiguration
resources:
  - resources:
      - secrets
    providers:
      - kms:
          apiVersion: v2
          name: kms-3_secrets
          endpoint: unix:///var/run/kmsplugin/kms-3.sock
          timeout: 10s
      - identity: {}
      - kms:
          apiVersion: v2
          name: kms-1_secrets
          endpoint: unix:///var/run/kmsplugin/kms-1.sock
          timeout: 10s
```

Both KMS providers run as separate sidecar containers without deduplication, maintaining full isolation.

#### Preconditions for Configuration Changes (Tech Preview v2)

**Invariants:**
1. Once an encryption key is generated, it must propagate through the entire state machine. Each key has a monotonically increasing ID that determines provider ordering in the EncryptionConfiguration.
2. Once a write key has been used by a single instance, it must be assumed to have encrypted data. The rollout must finish before proceeding to the next key.
3. The API configuration must resolve to the same encryption key instance.

**Pre-flight checks:** Before generating a new encryption key for migration-triggering changes, keyController deploys a pod with the KMS plugin to verify status and encrypt/decrypt capability. A new encryption key is only generated after pre-flight checks succeed. This prevents deadlocks where a misconfigured key (e.g., typo in transit-key) is deployed but non-functional, and the system cannot recover because the key must complete its cycle.

**Blocked operations during promotion:** keyController will not generate a new encryption key while the in-progress key is being promoted. If the admin overwrites the configuration (e.g., switches from KMS1 to KMS2 while KMS1 is still rolling out), the new key is not generated. To fix the in-progress configuration, admin must provide the same KMS configuration — this associates the fix with the existing encryption key.

**Recovery from incorrect configuration:**
- Migration-triggering fields: prevented by pre-flight checks (misconfiguration is caught before key generation).
- Non-migration fields (e.g., image): admin provides corrected configuration via APIServer resource. A new revision is created; older providers retain their original configuration as fallback.

#### Variation: KMS Key Rotation

When a KMS plugin rotates its `key_id` (KEK), this triggers neither a new encryption key secret nor a new revision. The mechanism for detecting and handling `key_id` rotation is under evaluation and not covered in this enhancement.

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

**Tech Preview V2**

API changes for Tech Preview v2 are covered by a separate EP. This EP assumes the API exists and describes only the encryption controller-side implementation. The API provides provider-specific fields (image, vault-address, vault-namespace, transit-key, transit-mount, etc.) that keyController splits into `kms-encryption-config` and `kms-provider-config`.

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

- `kms-encryption-config`, `kms-provider-config`, `kms-secret-data`, and `kms-configmap-data` are stored in the same encryption key secret for atomicity
- keyController uses provider-specific field-level comparison (not simple equality) to determine migration necessity
- UDS path convention: `unix:///var/run/kmsplugin/kms-{keyID}.sock` — keyID appended for uniqueness

### Risks and Mitigations

**Risk: KMS Plugin Unavailable During Controller Sync**
- **Impact:** Controllers cannot detect key rotation
- **Mitigation:** No mitigation in Tech Preview. GA will add health checks and expose it to cluster admin via operator conditions to degrade

**Risk: Race Condition Between EncryptionConfiguration and Sidecar Availability (Tech Preview v2)**
- **Impact:** KAS instance broken if sidecar configuration not yet available
- **Mitigation:** Atomic storage of both configs in same encryption key secret

**Risk: Invalid Credential Secret (Tech Preview v2)**
- **Impact:** KMS plugin cannot authenticate to external KMS
- **Mitigation:** keyController validates and goes degraded; old credentials continue to be used

**Risk: Configuration Change During Write Key Promotion (Tech Preview v2)**
- **Impact:** Conflict with in-progress state machine
- **Mitigation:** keyController blocks new encryption key generation during promotion

#### KMS Key Loss Considerations

If the KMS key (the KEK used to encrypt the cluster seed, which Kubernetes then uses to generate DEKs for encrypting cluster data) is deleted externally, all encrypted resources in etcd become unreadable.

Recovery from this situation would require deleting all resources that we are unable to decode and then recreating them from scratch. This process is costly and complex to implement (for example, all certificates would need to be reissued, the etcd cluster rebuilt, etc.), and is comparable in effort to implementing a full re-bootstrap. Additionally, the recovery flow would need to be covered by CI tests to catch potential regressions.

Moreover, the platform itself would not be able to recreate resources required by user workloads, since only users have the necessary knowledge about them. In practice, this means users must have their own mechanisms for restoring these resources.

On the Vault side, the key is stored in Vault's Transit secrets engine. By default, keys in Transit have `deletion_allowed` set to `false`. A Vault administrator would need to explicitly change this setting to `true` in order to allow key deletion. In general, standard best practices should be followed. This includes enforcing least-privilege access to sensitive API endpoints, such as those used for key deletion or key configuration updates. It is also recommended to periodically back up keys, so they can be restored if needed.

For these reasons, recovery from KMS key loss is a non-goal of this enhancement.

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
- Migration from KMS Plugin A to KMS Plugin B
- In-place update (image change without migration)
- KMS to identity and back to KMS (duplicate provider scenario)
- KMS to static encryption and vice versa
- Invalid credential secret handling (degraded state)
- Verify data re-encryption completes

## Graduation Criteria

### Dev Preview -> Tech Preview

None

### Tech Preview v1 -> Tech Preview v2

- KMS configuration splitting into kms-encryption-config, kms-provider-config, kms-secret-data, and kms-configmap-data with atomic storage in encryption key secrets
- Multiple concurrent KMS providers during migration with UDS path isolation
- Field-level comparison for migration-requiring vs. in-place configuration changes
- Pre-flight checks before generating new encryption keys
- Credential/ConfigMap validation with degraded status reporting
- Periodic sync of referenced Secrets and ConfigMaps to all active key secrets
- All migration scenarios validated (KMS-to-KMS, KMS-to-static, KMS-to-identity-to-KMS)

### Tech Preview v2 -> Tech Preview v3

- Report current KMS encryption status to platform users (e.g., active KMS plugins)
- Automatic `key_id` rotation detection
- KMS plugin health checks
- Feature parity with existing modes (monitoring, migration, key rotation)
- Removal of unused KMS plugins from EncryptionConfiguration after migration completes
- Support updating the KMS timeout field via `unsupportedConfigOverrides`

### Tech Preview -> GA

- Failure mode coverage: loss of access to KMS service
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

**Invalid Credential Secret:**
- keyController goes degraded, no changes propagated, old credentials continue to be used
- Detection: `EncryptionControllerDegraded=True`
- Recovery: Create/fix the credential secret; keyController resumes automatically

**Configuration Change During Write Key Promotion:**
- keyController will not generate a new encryption key during promotion — admin cannot overwrite the current configuration with a different provider
- Admin can fix in-progress config by providing the same KMS configuration (e.g., increase timeout)
- Detection: `EncryptionMigrationControllerProgressing=True`

**Configuration Updates During Migration:**
- Migration-triggering field misconfigurations are prevented by pre-flight checks (see Preconditions section)
- Older KMS plugins (read-only providers) cannot be updated; only the active (write) provider can be changed

**Non-Migration Update Fallback:**
- Only the active provider's sidecar config is updated; older providers retain their original configuration as fallback
- Detection: Revision rollout failure in operator status
- Recovery: Provide corrected configuration via APIServer resource

## Support Procedures

### Detecting KMS Rotation Issues
```bash
# Check encryption key secrets
oc get secrets -n openshift-config-managed -l encryption.apiserver.operator.openshift.io/component=encryption-key

# Check controller logs
oc logs -n openshift-kube-apiserver-operator deployment/kube-apiserver-operator | grep -i kms
```

### Inspecting Encryption Configuration (Tech Preview v2)
```bash
# Check encryption-configuration secrets for sidecar configs
oc get secrets -n openshift-kube-apiserver -l encryption.apiserver.operator.openshift.io/component -o yaml

# Check encryption key secrets
oc get secrets -n openshift-config-managed -l encryption.apiserver.operator.openshift.io/component=encryption-key -o yaml
```

### Disabling KMS Encryption

**Tech Preview v1:**
1. Update APIServer: `spec.encryption.type: "aescbc"`
2. Wait for migration to complete
3. Manually remove KMS plugin static pods from control plane nodes

**Tech Preview v2:**
1. Update APIServer: `spec.encryption.type: "aescbc"` (or `identity`)
2. keyController creates a new encryption key secret for the target mode
3. Migration proceeds automatically — KMS remains as read provider until migration completes
4. After migration, encryption controllers notify the cluster admin via status conditions that the KMS plugin can be safely decommissioned (GA)
5. Backups encrypted with the previous KMS plugin will not be restorable without access to that plugin

### Recovering from Invalid KMS Configuration (Tech Preview v2)

1. Check operator status: `oc get co kube-apiserver -o jsonpath='{.status.conditions}'`
2. If degraded due to missing credential secret: create/fix the secret. keyController resumes automatically.
3. If stuck during write key promotion: provide the same KMS configuration via APIServer resource.

**etcd Backup/Restore:**
- Before backup: Document KMS configuration, verify key availability
- Before restore: Verify KMS key accessible, credentials valid
- Critical: Deleting KMS key makes backups unrestorable

## Alternatives (Not Implemented)

### Alternative: Separate KMS-Specific Controllers

Instead of extending existing controllers, create new KMS-only controllers.

**Why not chosen:**
- Code duplication (migration logic, state management)
- More operational burden (additional monitoring, alerts)

### Alternative: Separate Secrets for EncryptionConfiguration and Sidecar Configuration

**Why not chosen:** Creates race conditions — EncryptionConfiguration could reference a KMS plugin before sidecar configuration is available.

### Alternative: Deduplication of KMS Plugin Instances During Migration

**Why not chosen:** Adds complexity to plugin lifecycle (must detect identical providers), breaks isolation, and complicates rollback scenarios.

## Infrastructure Needed

None - extends existing library-go code.