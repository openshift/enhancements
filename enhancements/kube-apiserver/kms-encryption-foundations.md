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
last-updated: 2026-05-26
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
- Split KMS configuration into kms-encryption-config, kms-plugin-config, kms-plugin-secret data, and kms-plugin-configmap data
- Seamless migration between encryption modes (aescbc ↔ KMS, KMS ↔ KMS)
- Field-level comparison to distinguish migration-requiring vs. in-place changes
- Pre-flight checks before generating new encryption keys
- Credential/ConfigMap validation with degraded status reporting
- Periodic sync of referenced Secrets and ConfigMaps to all active key secrets
- KMS plugin deployment/lifecycle management (see [KMS Plugin Lifecycle Management](#kms-plugin-lifecycle-management-tech-preview-v2))
- API field definitions for KMS provider configuration in APIServer resource

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
- Allowing cluster admins to configure which resources they want to encrypt

## Proposal

Extend the existing encryption controller framework in `openshift/library-go` to support KMS encryption in two phases:

**Tech Preview v1 (External Plugin Management):**

Users deploy KMS plugins manually on all control plane nodes as static pods or systemd units at a predefined socket path (`unix:///var/run/kmsplugin/kms.sock`).
Encryption controllers use the static endpoint in EncryptionConfiguration. KMS-to-KMS migrations are not supported in Tech Preview v1 since only one plugin can listen at the static socket path at a time.

**Tech Preview v2 (Managed Plugin Lifecycle):**

Users specify plugin-specific configuration for managed KMS provider types (e.g. Vault) via the APIServer resource.
Encryption controllers split the KMS configuration API into multiple parts stored atomically in encryption key secrets:

1. `kms-encryption-config` — structured Kubernetes KMS v2 provider configuration used to generate the EncryptionConfiguration provider entry (apiVersion: v2, name, endpoint, timeout)
2. `kms-plugin-config` — serialized `KMSConfig` resource ([config.openshift.io/v1](https://github.com/openshift/api/blob/master/config/v1/types_kmsencryption.go)), giving consumers access to provider-specific configuration (image, vault-address, transit-mount, transit-key, etc.)
3. `kms-plugin-secret-{secretName}_{dataKey}` — individual keys from the referenced Secret are stored as separate entries, where `secretName` is the Kubernetes secret name and `dataKey` is the individual data key within that secret, separated by `_` (underscore is forbidden in Kubernetes resource names, preventing collisions). The underscore also disambiguates entries that would otherwise collide when concatenated: secret `vault-approle` with key `secret-role-id` produces `vault-approle_secret-role-id`, while secret `vault-approle-secret` with key `role-id` produces `vault-approle-secret_role-id` — without the separator both would yield `vault-approle-secret-role-id`. Only the specific data keys required by each provider type are carried; any other keys in the referenced secret are ignored. As a concrete example, Vault AppRole credentials produce `kms-plugin-secret-vault-approle-secret_role-id` and `kms-plugin-secret-vault-approle-secret_secret-id` (carrying only the `role-id` and `secret-id` keys).
4. `kms-plugin-configmap-{configMapName}_{dataKey}` — individual keys from the referenced ConfigMap are stored as separate entries, following the same `_` separator convention as secret data. For example, a Vault CA bundle ConfigMap produces `kms-plugin-configmap-vault-ca-bundle_ca-bundle.crt`.

   Credentials are stored as individual top-level keys rather than a single nested blob because the installer controller writes each `.data` key as a separate file on disk and the KMS plugin sidecar — a third-party binary — consumes credentials as individual files. Bundling them into one entry would require the sidecar to parse and extract them, which it cannot do.

   For example, an encryption-configuration secret with this layout:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: encryption-config-kube-apiserver-9
   data:
     kms-plugin-config-1: |
       address: bar
       ...
     kms-plugin-secret-vault-approle-secret_role-id-1: VALUE
     kms-plugin-secret-vault-approle-secret_secret-id-1: VALUE
     kms-plugin-configmap-vault-ca-bundle_ca-bundle.crt-1: VALUE
   ```

Storing all related data in a single secret avoids race conditions caused by reading live, independently changing configuration.
In kas-o, the targetConfigController operates on live data and may generate a manifest based on the current sidecar configuration. However, this configuration can change before the RevisionController creates a revision. 
As a result, the generated manifest may no longer match the actual configuration state at the time the revision is created. Keeping all dependent configuration in a single secret ensures consistency and guarantees that both controllers operate on the same, atomic snapshot of data.

Additionally, consolidating the data in a single secret leverages existing revisioning and cleanup mechanisms.
The keyID is appended to the UDS path (`unix:///var/run/kmsplugin/kms-{keyID}.sock`) to ensure uniqueness among providers, enabling KMS-to-KMS migrations with multiple concurrent plugins.

**Key changes in library-go:**
1. Add KMS mode constant and track KMS configuration in encryption key secrets
2. Split configuration into kms-encryption-config, kms-plugin-config, kms-plugin-secret data, and kms-plugin-configmap data; copy with keyID suffix to encryption-configuration secrets (Tech Preview v2)
3. Field-level comparison, credential/ConfigMap validation, and periodic sync of referenced resources to all active key secrets (Tech Preview v2)
4. Sidecar injection into API server pod specs from the encryption-configuration secret (Tech Preview v2)
5. Reuse existing migration controller (no changes needed)

### Workflow Description

#### Actors in the Workflow

**cluster admin** is a human user responsible for configuring and maintaining the cluster.

**KMS** is the external Key Management Service that stores and manages the Key Encryption Key (KEK).

**KMS plugin** is a gRPC service implementing Kubernetes KMS v2 API. In Tech Preview v1, it runs as a static pod on each control plane node. In Tech Preview v2, it runs as a sidecar container alongside with API Servers (kube-apiserver, oauth-apiserver, openshift-apiservers) managed by the APIServer operators. It communicates with the external KMS to encrypt/decrypt data encryption keys .

**API server operator** is the OpenShift operator (kube-apiserver-operator, openshift-apiserver-operator, or authentication-operator) managing API server deployments.

#### Encryption Controllers

**keyController** manages encryption key lifecycle. Creates encryption key secrets in `openshift-config-managed` namespace. For KMS mode, creates secrets storing KMS configuration.
For Tech Preview v2, also propagates updates from the API configuration, splits configuration into `kms-encryption-config`, `kms-plugin-config`, `kms-plugin-secret-{secretName}_{dataKey}`, and `kms-plugin-configmap-{configMapName}_{dataKey}`, performs field-level comparison, validates credential secrets, and periodically syncs referenced Secrets/ConfigMaps to all active key secrets.

**stateController** generates EncryptionConfiguration for API server consumption. Implements distributed state machine ensuring all API servers converge to same revision.
For KMS mode, generates EncryptionConfiguration using the KMS configuration.
For Tech Preview v2, also copies `kms-plugin-config`, `kms-plugin-secret-{secretName}_{dataKey}`, and `kms-plugin-configmap-{configMapName}_{dataKey}` with keyID suffix (e.g., `kms-plugin-config-1`, `kms-plugin-secret-vault-approle-secret_role-id-1`, `kms-plugin-configmap-vault-ca-bundle_ca-bundle.crt-1`) to the encryption-configuration secret.

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
     # - Tech Preview v2: kms-encryption-config and kms-plugin-config (see Tech Preview v2 section below)
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

1. Cluster admin configures KMS provider in the APIServer resource with Vault-specific configuration:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   spec:
     encryption:
       type: KMS
       kms:
         type: Vault
         vault:
           kmsPluginImage: registry.example.com/vault-plugin@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
           vaultAddress: https://vault.example.com:8200
           vaultNamespace: my-namespace
           tls:
             caBundle:
               name: vault-ca-bundle  # ConfigMap in openshift-config namespace
             serverName: vault.example.com
           authentication:
             type: AppRole
             appRole:
               secret:
                 name: vault-approle # Secret in openshift-config namespace with roleID and secretID keys
           transitMount: transit
           transitKey: my-encryption-key
   ```

2. keyController detects the configuration, fetches the referenced Secret from `openshift-config` namespace, validates that required data keys are present, and creates an encryption key secret containing `kms-encryption-config`, `kms-plugin-config`, individual `kms-plugin-secret-{secretName}_{dataKey}` entries, and individual `kms-plugin-configmap-{configMapName}_{dataKey}` entries:
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
     kms-plugin-config: <base64-encoded sidecar container config>
     kms-plugin-secret-vault-approle-secret_role-id: <base64-encoded role-id>
     kms-plugin-secret-vault-approle-secret_secret-id: <base64-encoded secret-id>
     kms-plugin-configmap-vault-ca-bundle_ca-bundle.crt: <base64-encoded CA bundle>
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

4. stateController copies `kms-plugin-config`, `kms-plugin-secret` entries, and `kms-plugin-configmap` entries with keyID suffix to the encryption-configuration secret:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: encryption-config-kube-apiserver-9
     namespace: openshift-kube-apiserver
   type: Opaque
   data:
     encryption-config: <EncryptionConfiguration>
     kms-plugin-config-1: <base64-encoded sidecar config for keyID 1>
     kms-plugin-secret-vault-approle-secret_role-id-1: <base64-encoded role-id for keyID 1>
     kms-plugin-secret-vault-approle-secret_secret-id-1: <base64-encoded secret-id for keyID 1>
     kms-plugin-configmap-vault-ca-bundle_ca-bundle.crt-1: <base64-encoded CA bundle for keyID 1>
   ```

5. The encryption-configuration secret is revisioned, triggering a new rollout. The respective operator configures sidecars accordingly (see [KMS Plugin Lifecycle Management](#kms-plugin-lifecycle-management-tech-preview-v2)).

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

stateController copies kms-plugin-config, kms-plugin-secret entries, and kms-plugin-configmap entries from both encryption key secrets into the encryption-configuration secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: encryption-config-kube-apiserver-9
  namespace: openshift-kube-apiserver
data:
  encryption-config: <EncryptionConfiguration>
  kms-plugin-config-1: <base64-encoded sidecar config for keyID 1>
  kms-plugin-config-2: <base64-encoded sidecar config for keyID 2>
  kms-plugin-secret-vault-approle-secret_role-id-1: <base64-encoded role-id for keyID 1>
  kms-plugin-secret-vault-approle-secret_secret-id-1: <base64-encoded secret-id for keyID 1>
  kms-plugin-secret-vault-approle-secret_role-id-2: <base64-encoded role-id for keyID 2>
  kms-plugin-secret-vault-approle-secret_secret-id-2: <base64-encoded secret-id for keyID 2>
  kms-plugin-configmap-vault-ca-bundle_ca-bundle.crt-1: <base64-encoded CA bundle for keyID 1>
  kms-plugin-configmap-vault-ca-bundle_ca-bundle.crt-2: <base64-encoded CA bundle for keyID 2>
```

Both providers run as separate sidecar containers with different unix domain sockets (kms-1.sock, kms-2.sock).

#### Variation: Updates Not Requiring Migration (Tech Preview v2)

Fields that only affect the container spec (e.g., image for CVE fixes) do not change the KEK:

1. keyController runs pre-flight checks to validate the new configuration (see [Pre-flight Checker](#pre-flight-checker-tech-preview-v2)).
2. keyController updates the existing encryption key secret in-place. No new secret is created.
3. stateController detects the change and triggers a new revision with the updated `kms-plugin-config`.

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
3. Once a write key has been read by a single instance, it must be assumed that it may be used to encrypt data. 
  Encryption keys for KMS are derived from the API configuration. 
  Because the configuration can change over time, a new key cannot be generated until the current rollout is fully completed.
3. The API configuration must resolve to the same encryption key instance.

**Pre-flight checks:** Before applying any configuration change, a dedicated controller deploys a pod with the KMS plugin to verify status and encrypt/decrypt capability. Configuration changes are only applied after pre-flight checks succeed. This prevents deadlocks where a misconfigured key (e.g., typo in transit-key) is deployed but non-functional, and the system cannot recover because the key must complete its cycle. See [Pre-flight Checker](#pre-flight-checker-tech-preview-v2) for the detailed mechanism.

**Blocked operations during promotion:** keyController will not generate a new encryption key while the in-progress key is being promoted. If the admin overwrites the configuration (e.g., switches from KMS1 to KMS2 while KMS1 is still rolling out), the new key is not generated. To fix the in-progress configuration, admin must provide the same KMS configuration — this associates the fix with the existing encryption key.

**Recovery from incorrect configuration:**
- Migration-triggering fields: prevented by pre-flight checks (misconfiguration is caught before key generation).
- Non-migration fields (e.g., image): prevented by pre-flight checks (misconfiguration is caught before the update is applied). Since no new revision is created, the existing configuration is preserved and the system continues to operate correctly.

#### Pre-flight Checker (Tech Preview v2)

The pre-flight checker validates KMS configuration before any configuration change is applied. The API allows admins to specify a KMS plugin image reference, and the API may add new fields over time in a backward-compatible way (e.g., a new field that maps to a new plugin flag). A new flag is expected to be supported for a range of image versions (say 1.X+), but we do not control which image version the admin provides: they might set a new field while referencing an older image (e.g., 1.X-2) that does not support the corresponding flag. Rather than maintaining a compatibility matrix between API field sets and image versions, we run the pre-flight checker unconditionally — the cost of an extra pod is acceptable compared to the risk of deploying an incompatible configuration.

The checker consists of two parts: a preflight binary that tests the KMS provider end-to-end via the plugin, and a controller that coordinates the check with the key-controller.

##### Preflight Binary

The preflight binary (`kms-preflight`) is shipped in the operator image. It runs on control plane nodes inside a one-shot pod that uses the [KMS plugin lifecycle management](#kms-plugin-lifecycle-management-tech-preview-v2) logic to attach a KMS plugin sidecar based on the current configuration. The binary connects to the plugin via the KMS v2 gRPC API and performs three checks:

1. **Status** — polls until the plugin reports `healthz=ok`.
2. **Encrypt** — encrypts a random payload.
3. **Decrypt** — decrypts the ciphertext and verifies the round-trip matches.

The pod uses readiness gates to post check results back to the controller. A ServiceAccount, Role, and RoleBinding are created so the pod can update its own status. These resources are cleaned up when the pod is removed.

After a successful check the preflight pod is kept for a short period (e.g., 1 hour) so that its logs can be inspected, then cleaned up by a subsequent sync.

##### KMS Preflight Controller

Like all other encryption controllers, a `KMSPreflightController` instance runs in each API server operator (cluster-kube-apiserver-operator, cluster-openshift-apiserver-operator, and cluster-authentication-operator). 
Each instance coordinates with its operator's key-controller using a hash-based handshake protocol:

1. The key-controller computes a hash of the current KMS configuration including the contents of the referenced Secrets/ConfigMaps and sets `EncryptionKMSPreflightRequired` (hash in the message).
2. The preflight controller reads that hash, deploys the preflight pod, and runs the checks.
3. On success, the preflight controller sets `EncryptionKMSPreflightSucceeded` (same hash in the message).
4. The key-controller waits for the two hashes to match before creating an encryption key.

This follows the same pattern as the revision and installer controllers.

**Why the handshake is necessary:**

Without this protocol a race can occur: preflight passes for config A, the key-controller starts creating a key for A, but config changes to B before the key is written. The preflight controller re-runs for B and overwrites status with hash B, leaving the key created for A inconsistent with status.

Each controller writing its own condition prevents this. If the config changes mid-flight, the key-controller posts a new hash and the preflight controller sees the mismatch and re-runs the check.

#### KMS Key Rotation

When the remote KMS rotates backing key material (for example, a new Vault Transit key version), the KMS v2 plugin reports a new opaque `key_id` in `Status` (called `kekId` below) and `Encrypt` responses. Cluster admins need etcd data re-encrypted under the new key without minting a new encryption key secret or extra static pod revisions for every external rotation.

Approach: Keep the existing encryption state machine, migration controller, and state controller unchanged. Add a new `EncryptionRotationController` per API server operand. It tracks convergence of the `kekId` across control plane nodes, records rotation progress on operator status, and triggers storage migration by adjusting two fields in the existing resources: 
- prune `StorageVersionMigration` objects per encrypted group resource 
- clear `migrated-*` annotations on the encryption-config secret

This allows us to keep the migration controller entirely unchanged.

##### Status API

We propose to add new fields for rotation and health status on the operand resource (not on `config.openshift.io/v1/APIServer`):

```yaml
apiVersion: operator.openshift.io/v1
kind: KubeAPIServer
metadata:
  name: cluster
status:
  encryptionStatus:
    healthReports: [...]
    keyRotationStatus: [...]   # max n-entries
  conditions:
    # interim — health controller today
    - type: KMSHealthReporter_master-0
      status: "True"
      reason: AsExpected
      message: '[{"kekID":"kek-9f2c","keyID":"2","status":"healthy","lastChecked":"..."},...]'
```

##### External components

- KMS health aggregation: (separate controller, in development - TODO rebase with Krzys PR) populates `KubeAPIServer.status.encryptionStatus.healthReports` from per-node plugin probes. Until that field is populated, the same data may appear in `status.conditions` (`type: KMSHealthReporter_<node>`, JSON in `message`). The rotation controller reads `healthReports` when present; it does not probe plugins or write health data.

- Pre-flight: ([Pre-flight Checker](#pre-flight-checker-tech-preview-v2)) validates KMS configuration before an encryption key is created. It will record the very first `kekId` it can observe from querying the plugin during its initial checks.

##### KEK convergence

For each plugin `keyId`, collect `kekId` from every healthy node reporting that `keyId`. We determine "converged" when all such nodes report the same `kekId`:

```text
keyId "1" → { master-0: kek-4a17, master-1: kek-4a17 }   ✓ converged
keyId "2" → { master-0: kek-9f2c, master-1: kek-7aa1 }   ✗ divergent
```

Rotation uses the `keyId` of the current write EncryptionKey KMS config, then requires that it to be converged before setting `discoveryTime` or considering `startRotation`.

We set a 5 minute convergence delay, allowing the apiservers to settle on the new KEK and invalidate their own internal caches. 

##### `keyRotationStatus`

Each entry tracks one rotation episode (ring buffer, capped at 10 (tbd) items):

| Field | Meaning |
|-------|---------|
| `kekId` | KEK identity for this episode |
| `discoveryTime` | When all nodes agree on this `kekId` for the `keyId`/plugin instance. Unset until per-`keyId` convergence. |
| `migrationStartTime` | When the operator started storage migration. Empty = not started. |
| `migrationFinishTime` | When migration completed (mirrored from secret `migrated-*` covering all encrypted GRs). Empty with `migrationStartTime` set means a rotation is in progress. |


##### `startRotation` (KEK change only)

This is not the same as initial provider migration, which is handled entirely by the migration controller on first enablement. `startRotation` runs only when:

1. A prior rotation episode has `migrationFinishTime` (e.g. the initial migration finished).
2. For the current write `keyId`, per-node health shows a `kekId` different from the last completed rotation entry.
3. That `kekId` is converged across all nodes for that `keyId`. This sets `discoveryTime` on a new `keyRotationStatus` entry and trims the list for retaining the last n episodes.
4. `discoveryTime` + convergence delay has elapsed.
5. Then: set `migrationStartTime` and call `PruneMigration` per encrypted GR, clear secret `migrated-*` annotations. This causes the migration controller to run.

A manual spike confirmed that deleting `StorageVersionMigration` CRs and clearing `migrated-resources` / `migrated-timestamp` on the encryption-config secret re-triggers migration without changes to `state.MigratedFor`, the state machine, or the migration controller.

##### End-to-end flow

**Initial provider migration**: Migration controller ensures SVM per GR. Rotation controller sets `discoveryTime` when health agrees on `kekId` (mirror only, never `startRotation`). Migration controller sets `migrated-*` on the secret. Rotation controller sets `migrationFinishTime` when annotations cover all encrypted GRs.

**KEK rotation**: Rotation controller sets `discoveryTime` when the new `kekId` converges → waits convergence delay → sets `migrationStartTime` → prunes SVM and clears migration annotations → migration controller re-encrypts → rotation controller sets `migrationFinishTime`.

### KMS Plugin Lifecycle Management (Tech Preview v2)

In Tech Preview v1, users manually deploy KMS plugins as static pods on each control plane node, communicating with the API server via a `hostPath` volume at `/var/run/kmsplugin`.
Tech Preview v2 replaces this with managed sidecar containers running in the same pod as the API server, using an `emptyDir` volume shared between the API server container and the sidecar(s).

#### Configuration Flow

Provider-specific configuration (container image, Vault address, transit key, etc.) reaches the encryption-configuration secret through the [encryption controller pipeline described above](#encryption-controllers). The API server operator then reads it and injects sidecar containers into the API server pod spec.

For kube-apiserver-operator, a revision post-check done in the RevisionController validates the revisioned `encryption-config` Secret against the revisioned `kube-apiserver-pod` ConfigMap in `openshift-kube-apiserver`. A revision is not marked ready until they are consistent, preventing inconsistent rollouts caused by races between `targetConfigController` and `RevisionController`. The other operators do not need this check because their workload sync controllers work with revisioned resources directly, for example, [openshift-apiserver workload sync](https://github.com/openshift/cluster-openshift-apiserver-operator/blob/main/pkg/operator/workload/workload_openshiftapiserver_v311_00_sync.go#L411).

#### Sidecar Injection

The sidecar injection logic is implemented in library-go and operates on a `pod.PodSpec`. Each API server operator calls it from the controller that manages its API server pod definition:

- **cluster-kube-apiserver-operator** calls it from `targetConfigController`, which builds the static pod definition stored in a ConfigMap.
- **cluster-openshift-apiserver-operator** and **cluster-authentication-operator** call it from their workload sync controllers, which reconcile the Deployment that runs their aggregated API server.

When KMS is enabled, the injection reads the encryption-configuration secret, extracts the KMS provider entries and their corresponding `kms-plugin-config-{keyID}`, and for each active provider: 

1. Builds a sidecar container using a provider-specific builder.
2. Appends it to the pod spec.
3. Adds a shared `emptyDir` volume mounted at `/var/run/kmsplugin` in both the API server container and the sidecar(s).

#### Provider Abstraction

Each KMS provider type has a sidecar builder that constructs the container spec from the provider configuration, credentials, and KMS endpoint. Currently, Vault is the only implemented provider. Adding a new provider requires implementing a sidecar builder and adding its configuration fields to the provider config union.

Credentials and ConfigMap data (`kms-plugin-secret-{secretName}_{dataKey}-{keyID}` and `kms-plugin-configmap-{configMapName}_{dataKey}-{keyID}`) are carried automatically by the encryption controllers through the encryption-configuration secret, so the sidecar builder can consume them without additional plumbing.

#### Multiple Concurrent Sidecars

During KMS-to-KMS migration, the encryption-configuration secret contains provider configs for all active keys. The operator creates a separate sidecar for each key, listening on its own unix domain socket (e.g., `kms-1.sock`, `kms-2.sock`).

#### Health Reporter Sidecar

When KMS encryption is enabled, a health reporter sidecar runs alongside every API server pod replica. The sidecar probes the colocated KMS plugin(s) and publishes the outcome to the owning operator's CR as a per-node condition. A separate aggregator controller picks up these conditions and emits one or more `ClusterOperator`-bound rollups (starting with `KMSPluginsDegraded`, see [Aggregator behavior](#aggregator-behavior)).

The sidecar's lifecycle (injection into the pod spec, image, mounts, RBAC) is managed by the same mechanism that handles KMS plugin sidecars; see [KMS Plugin Lifecycle Management](#kms-plugin-lifecycle-management-tech-preview-v2).

The reporter receives the set of UDS sockets to probe as flags at injection time. The `pluginlifecycle` package in library-go already enumerates the active KMS plugins from the encryption-config secret when it builds the pod spec, so passing the same socket paths into the reporter is essentially free. Plugin additions and removals always trigger a pod-spec change, which restarts the pod, so there is no live-discovery requirement.

##### Topology

One sidecar per API server pod replica, scaling with control-plane HA replica count:

- One per kube-apiserver static pod (typically 3 in HA, one per control-plane node)
- One per `openshift-oauth-apiserver` Deployment replica
- One per `openshift-apiserver` Deployment replica

During KMS-to-KMS migration, the same sidecar probes every active KMS plugin in its pod (see [Multiple Concurrent Sidecars](#multiple-concurrent-sidecars)) and reports their combined state in the Message field of its single per-node condition.

##### Probe contract

Each sidecar probes its colocated KMS plugin(s) over the local UDS at `unix:///var/run/kmsplugin/kms-{keyID}.sock` (the same socket path scheme described in [Sidecar Injection](#sidecar-injection)).

**Naming caveat.** `{keyID}` in the socket path is **not** an id of a cryptographic key. It is the id of the encryption key secret managed by the encryption controllers, a monotonically incrementing sequence number (a new one per key rotation). The KMS v2 plugin separately reports the id of the remote KEK it currently uses in its `StatusResponse.key_id`. This document keeps `keyID` for the socket-path id and `kekID` for the plugin-reported KEK. Conflating them will misbehave in any consumer that assumes `keyID` names a key.

##### Per-tick emission

Each probe produces one `PluginHealthCondition` (defined in [Message format](#message-format)) for the plugin it targeted. The sidecar collects one entry per colocated plugin into a `PluginHealthConditions` array and writes the minified JSON to the condition's `Message`. Each entry's `lastChecked` is the wall-clock time of that probe.

##### Destination

The sidecar writes one condition per pod replica to the owning operator's `*.operator.openshift.io/cluster` CR via Server-Side Apply (per-entry ownership via `+listType=map` on `OperatorStatus.Conditions`). The aggregator controller reads these conditions and emits the `KMSPluginsDegraded` rollup. See [KMS Plugin Health Conditions](#kms-plugin-health-conditions) for the exact naming, status mapping, and rollup behavior, and [KMS Health Reporter Connectivity](#kms-health-reporter-connectivity) for how the reporter authenticates and connects to perform the write.

```
within each apiserver pod (3 in HA):

  KMS plugin (kms-2.sock, write)  ──┐
  KMS plugin (kms-1.sock, read)   ──┤  UDS
                                    │
                                    ▼
                          health-reporter sidecar
                                    │
                                    │  SSA (per-node fieldManager)
                                    ▼
operator CR (kubeapiservers.operator.openshift.io/cluster):
  ├─ KMSHealthReporter_<nodeName>    ◄─ written by each per-pod reporter
  │      (one per node, multi-plugin state in Message)
  │
  └─ KMSPluginsDegraded              ◄─ written by aggregator controller
            │                            (reads the per-node entries above)
            │  matches _Degraded suffix
            ▼
  ClusterOperator: Degraded
```

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

In Tech Preview v2 we propose to extend the existing `KMSConfig`, adding
support to OCP-managed Vault KMS Plugin. The Vault KMS Plugin communicates
with Vault Enterprise to encrypt and decrypt resources. Users are expected to
fully configure and support Vault Enterprise themselves. OCP is responsible for
deploying and managing the Vault KMS Plugin.

Vault KMS Plugin configuration documentation can be found [here](https://github.com/hashicorp/web-unified-docs/blob/ab6191e4856b52a59a87fe0f17703671a7317ec6/content/vault/v1.21.x/content/docs/deploy/kubernetes/kms/configuration.mdx)

The `KMSConfig` currently supports configuring an AWS KMS Plugin through a union
discriminator. Since there is no current backing support for AWS, and no clients
using this API, we propose the removal of the `AWSKMSProvider` type and the
related `AWSKMSConfig`. We also propose removing the unused
`KMSEncryptionProvider` feature gate. We propose to keep the union
discriminator in preparation for future requests to support other KMS Plugins.

We propose to extend `KMSConfig` with `VaultKMSConfig`, also adding
`VaultKMSProvider` as new `KMSProviderType`.
We propose that the existing `KMSEncryption` feature gate be extended to include
the Vault KMS Plugin API.

The full structure of the proposed Vault KMS Plugin configuration API can be
found in [this pull request](https://github.com/openshift/api/pull/2805).

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

- `kms-encryption-config`, `kms-plugin-config`, `kms-plugin-secret-{secretName}_{dataKey}` entries, and `kms-plugin-configmap-{configMapName}_{dataKey}` entries are stored in the same encryption key secret for atomicity
- keyController uses provider-specific field-level comparison (not simple equality) to determine migration necessity
- UDS path convention: `unix:///var/run/kmsplugin/kms-{keyID}.sock` — keyID appended for uniqueness

#### KMS Health Reporter Connectivity

Short term, the reporter introduces no new credential. Every pod it runs in already mounts an admin-grade identity that can write the owning operator CR, and the reporter reuses it. The connection is split by pod type, because the kube-apiserver runs as a `hostNetwork` static pod while the aggregated API servers run as ordinary Deployments.

Reusing an admin-grade token is a deliberate short-term tradeoff. The trust boundary is the pod: the reporter is co-located with the API server process, and on the static pod with sidecars that already hold this exact `cluster-admin` token. A separate scoped credential short term would add a Role, a binding, and a token to rotate without shrinking the admin surface the pod already exposes.

##### Aggregated API servers (`openshift-apiserver`, `openshift-oauth-apiserver`)

The aggregated API servers run as Deployments in the pod network, each backed by a ServiceAccount (`openshift-apiserver-sa`, `oauth-apiserver-sa`). The reporter sidecar inherits that projected token with no extra wiring, exactly as the existing `openshift-apiserver-check-endpoints` sidecar does (it runs with no `--kubeconfig` and falls back to in-cluster config). Both SAs are bound to `cluster-admin`, so the token applies the per-node condition with no added RBAC.

##### kube-apiserver

The reporter reuses the kubeconfig `cert-syncer` already uses: it dials the local kube-apiserver at `https://localhost:6443`, the loopback endpoint every static-pod kubeconfig uses (`node-kubeconfigs`, `check-endpoints`), with the `localhost-recovery` token, a non-expiring legacy ServiceAccount token bound to `cluster-admin`. Because a static pod projects nothing automatically, the reporter must also mount the resources that kubeconfig references by path, including the serving CA it uses to verify the loopback connection.

Loopback is deliberate: not `kubernetes.default.svc` (does not resolve from a host-network static pod), and not the `KUBERNETES_SERVICE_HOST` ClusterIP. An unhealthy KMS plugin does not break this write, despite gating its own kube-apiserver's health: KMS gates `/readyz` and `/healthz` but not `/livez`, so the pod is dropped from the Service load balancer but never restarted and keeps serving on loopback. And the reporter writes an `operator.openshift.io` CR, which is not in the encrypted set (OpenShift encrypts only core `secrets` and `configmaps`), so the write uses the identity transformer and never calls KMS.

We lose the cross-node failover the ClusterIP would have offered. This is acceptable, because if a node's local kube-apiserver is itself down, the node's condition stops advancing `lastChecked` and the aggregator flips it to `Unknown` (see [Probe interval](#probe-interval)). Loopback also keeps the cluster network out of the write path: one fewer component that can fail between the reporter and its kube-apiserver.

##### Long term

The reporter moves to a dedicated, least-privilege identity, auto-rotated by the owning operator and scoped by RBAC to its single Server-Side Apply on the operator CR. The mechanism follows the same topology split:

- On the static pod, a managed client certificate whose CN names a dedicated identity, issued and rotated by the operator's existing certrotation, the same mechanism that already mints the scoped `check-endpoints` client cert.
- On the aggregated API servers, a dedicated ServiceAccount with a minimal Role, its bound token minted and rotated by the operator through the TokenRequest API (a plain projected volume cannot scope below the pod's own SA).

#### KMS Plugin Health Conditions

##### Naming convention

Each reporter sidecar writes one condition per pod replica to the owning operator's CR (`kubeapiservers.operator.openshift.io/cluster`, etc.), keyed by the node:

```
KMSHealthReporter_<nodeName>
```

The Type has no `_Available` or `_Degraded` suffix, so library-go's `StatusSyncer` ignores it and it does not propagate to the `ClusterOperator`. The aggregator controller consumes these conditions and emits the `KMSPluginsDegraded` rollup separately (see [Aggregator behavior](#aggregator-behavior)).

This is a **temporary mechanism**. Long term, we plan to add first-class status fields for KMS plugin health to the operator CR API, so this signal lives in a typed shape rather than a string-encoded condition. Until then, encoding it in `KMSHealthReporter_<nodeName>` avoids an API change and keeps the design reversible.

##### Status mapping

While this condition is a temporary solution (see [Naming convention](#naming-convention)), the `Status` and `Reason` are hardcoded to avoid library-go's `StatusSyncer` or other consumers reacting to per-pod transitions:

- `Status: True`
- `Reason: AsExpected`

All structured probe outcomes (per-plugin health, KEK ID, timestamps, error detail) live in the `Message` field and are parsed by the aggregator (see [Message format](#message-format) and [Aggregator behavior](#aggregator-behavior)).

##### Message format

The `Message` field carries the structured probe outcomes that the aggregator parses. It holds a single minified JSON array, one element per probed plugin:

```go
type PluginHealthConditions []PluginHealthCondition

type PluginHealthCondition struct {
    KeyID       string    `json:"keyID"`            // encryption-key-secret id from the socket path (kms-{keyID}.sock); not a cryptographic key
    KEKID       string    `json:"kekID,omitempty"`  // remote KEK id from the plugin's KMS v2 StatusResponse.key_id; omitted when the probe errors (no StatusResponse)
    Status      string    `json:"status"`           // healthy | unhealthy | error
    LastChecked time.Time `json:"lastChecked"`      // RFC 3339 timestamp of this probe
    Detail      string    `json:"detail,omitempty"` // error/health detail; omitted when healthy
}
```

##### Example

A three-node control plane during KMS-to-KMS migration. Each pod carries two plugins (six total across the cluster): `keyID=2` is the new key handling writes and reads, `keyID=1` is the previous key kept read-only to decrypt in-flight data. In this snapshot:

- `master-0`: both plugins healthy
- `master-1`: the new plugin (`id=2`) has a misconfigured cloud credential
- `master-2`: cannot reach either plugin

`Status` and `Reason` are uniform per the [Status mapping](#status-mapping); the actionable signal lives in `Message`:

```yaml
status:
  conditions:
    - type: KMSHealthReporter_master-0
      status: "True"
      reason: AsExpected
      message: '[{"kekID":"kek-9f2c","keyID":"2","status":"healthy","lastChecked":"2026-05-08T12:34:56Z"},{"kekID":"kek-4a17","keyID":"1","status":"healthy","lastChecked":"2026-05-08T12:34:56Z"}]'
    - type: KMSHealthReporter_master-1
      status: "True"
      reason: AsExpected
      message: '[{"kekID":"kek-9f2c","keyID":"2","status":"unhealthy","lastChecked":"2026-05-08T12:34:56Z","detail":"credential lacks decrypt permission"},{"kekID":"kek-4a17","keyID":"1","status":"healthy","lastChecked":"2026-05-08T12:34:56Z"}]'
    - type: KMSHealthReporter_master-2
      status: "True"
      reason: AsExpected
      message: '[{"keyID":"2","status":"error","lastChecked":"2026-05-08T12:34:56Z","detail":"connection refused"},{"keyID":"1","status":"error","lastChecked":"2026-05-08T12:34:56Z","detail":"connection refused"}]'
```

See [Aggregator behavior](#aggregator-behavior) for how these conditions roll up to the `ClusterOperator`.

##### Probe interval

Each reporter probes and emits on a fixed interval, **default 30 seconds**, passed as a sidecar flag at injection time (alongside the UDS socket paths). The exact value is not load-bearing: a probe cycle is `n` local UDS gRPC calls, one per `n` colocated plugins, followed by a single SSA write carrying all `n` results to the operator CR. Both are cheap, and tens-of-seconds detection latency is acceptable for KMS plugin health (key rotation and credential expiry are minutes-to-hours events).

Emission is unconditional and best-effort. The reporter writes its condition every tick even when nothing changed: the stale-reporter mitigation (see [Risks and Mitigations](#risks-and-mitigations)) relies on `lastChecked` advancing, so a write-on-change-only reporter would leave a healthy steady-state condition indistinguishable from a hung one. The reporter attempts the write every interval no matter the cluster state. If it cannot reach the kube-apiserver within the interval, it discards that result rather than queuing it: the reporter only ever needs its freshest probe on the CR, so once the next interval produces a result with a newer `lastChecked`, the un-written previous one is outdated and pointless to retry. A reporter that keeps failing to write stops advancing `lastChecked` and is caught by the staleness threshold.

The aggregator's staleness threshold is derived from the interval rather than configured independently: a condition whose `lastChecked` is older than `4 × interval` (120 s at the default) is treated as `Unknown`. Four intervals give enough data points that one or two dropped probes do not flip the rollup. Reporters apply small random jitter so replicas do not write the operator CR in lockstep.

##### Aggregator behavior

An aggregator controller reads the per-node `KMSHealthReporter_<nodeName>` conditions on the operator's CR and emits rollup conditions on the same CR. The first rollup is `KMSPluginsDegraded`; its `_Degraded` suffix routes it into the `ClusterOperator`'s `Degraded` slot via library-go's `StatusSyncer`. Additional rollups (e.g. `KMSPluginsAvailable`, `KMSPluginsProgressing`) may be added so the `ClusterOperator`'s `Available` and `Progressing` slots also reflect KMS plugin health. Each suffix maps to its matching `ClusterOperator` field via the same `StatusSyncer` convention, so each new type slots in without additional plumbing.

These rollup conditions (`KMSPluginsDegraded`, and any future `KMSPluginsAvailable` / `KMSPluginsProgressing`) are the admin-facing signal: they surface through `ClusterOperator` so that `oc get co kube-apiserver` is sufficient to learn KMS plugin health. The per-node `KMSHealthReporter_<nodeName>` conditions are plumbing for the aggregator and tooling, not intended for direct admin consumption.

The plan is to extend the existing [`conditionController`](https://github.com/openshift/library-go/blob/master/pkg/operator/encryption/controllers/condition_controller.go) in library-go's encryption controllers, which already emits the `Encrypted` condition on the same operator CR. It sits in the right call path (operator CR → ClusterOperator) and runs on the informer set the rollup needs. If extending it turns out to be a poor fit (conflicting sync triggers, unrelated dependencies that make the rollup hard to reason about), a dedicated controller will be introduced instead.

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

**Risk: Stale Reporter Conditions**
- **Impact:** A reporter that hangs leaves its last `KMSHealthReporter_<nodeName>` condition in etcd unchanged.
- **Mitigation:** Per-plugin `lastChecked` timestamps in Message expose staleness. The aggregator controller treats a condition whose `lastChecked` exceeds the freshness threshold (`4 × probe interval`; see [Probe interval](#probe-interval)) as effectively `Unknown`.

**Risk: Orphaned Conditions on encryption type change and node replacement**
- **Impact:** When KMS is disabled (e.g., switching to `aescbc`), reporter sidecars are removed. Without explicit cleanup, `KMSHealthReporter_<nodeName>` and `KMSPluginsDegraded` entries remain stale on the operator CR.
- **Mitigation:** The aggregator controller owns cleanup. It removes orphaned `KMSHealthReporter_<nodeName>` entries (when their owning sidecar is no longer present) and removes its own `KMSPluginsDegraded` entry on KMS disable.

**Risk: Cold-Start Window**
- **Impact:** KMS plugin starts first (KAS depends on it), KAS starts second, reporter starts last. During the window between KAS readiness and reporter readiness, no `KMSHealthReporter_<nodeName>` condition exists even though KMS is functional.
- **Mitigation:** Consumers must not infer "KMS broken" from condition absence; missing means "not yet observed". KMS plugin lifecycle and KAS startup do not depend on reporter conditions existing.

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

- KMS configuration splitting into kms-encryption-config, kms-plugin-config, kms-plugin-secret data, and kms-plugin-configmap data with atomic storage in encryption key secrets
- Multiple concurrent KMS providers during migration with UDS path isolation
- Field-level comparison for migration-requiring vs. in-place configuration changes
- Pre-flight checks before generating new encryption keys
- Credential/ConfigMap validation with degraded status reporting
- Periodic sync of referenced Secrets and ConfigMaps to all active key secrets
- KMS plugin sidecar lifecycle management via library-go injection into API server pod specs
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
- Operator conditions: `EncryptionControllerDegraded`, `EncryptionMigrationControllerProgressing`, plus per-node `KMSHealthReporter_<nodeName>` and the aggregated `KMSPluginsDegraded` (rolled into the `ClusterOperator`'s `Degraded` condition; see [Health Reporter Sidecar](#health-reporter-sidecar))
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

### Alternative: Exposing KMS Plugin Status Outside the Pod

The plugin's `Status` gRPC is reachable only over the in-pod UDS. Three projections outward were considered:

1. **`kube-rbac-proxy` in front of the plugin's `Status` RPC.** Adds a new exposed port on the kube-apiserver pod.
2. **Carry patch in kube-apiserver** to expose plugin status. Grows the kube-apiserver carry set we are trying to shrink.
3. **`kube-rbac-proxy` in front of the kube-apiserver pod.** No new port, but inserts a single point of failure in front of kube-apiserver.

**Chosen:** the in-pod [Health Reporter Sidecar](#health-reporter-sidecar) consumes `Status` locally and pushes the result to the operator CR.

## Infrastructure Needed

None - extends existing library-go code.
