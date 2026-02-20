---
title: kms-plugin-management
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
creation-date: 2025-11-28
last-updated: 2025-11-28
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-108"  # TP feature
  - "https://issues.redhat.com/browse/OCPSTRAT-1638" # GA feature
see-also:
  - "enhancements/kube-apiserver/kms-encryption-foundations.md"
  - "enhancements/kube-apiserver/kms-shim.md"
  - "enhancements/kube-apiserver/kms-migration-recovery.md"
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
replaces:
  - ""
superseded-by:
  - ""
---

# KMS Plugin Lifecycle Management

## Summary

Enable OpenShift to automatically manage the lifecycle of KMS (Key Management Service) plugins across multiple API servers. This enhancement provides a user-configurable interface to deploy, configure, and monitor KMS plugins as sidecar containers alongside kube-apiserver, openshift-apiserver, and oauth-apiserver pods. Support for multiple KMS providers (AWS KMS, HashiCorp Vault, Thales) is included with provider-specific authentication and configuration.

## Motivation

KMS encryption requires KMS plugin pods to bridge communication between the kube-apiserver and external KMS providers. Managing these plugins manually is operationally complex and error-prone. OpenShift should handle plugin deployment, authentication, health monitoring, and lifecycle management automatically on behalf of users.

Different KMS providers have vastly different authentication models, deployment requirements, and operational characteristics. This enhancement provides a unified plugin management framework while accommodating provider-specific needs.

### User Stories

* As a cluster admin, I want to enable KMS encryption by providing my KMS provider configuration in the APIServer config, so that OpenShift automatically deploys and manages the appropriate KMS plugin for me
* As a cluster admin, I want OpenShift to manage KMS plugin lifecycle on my behalf, so that I don't need to do any manual work when configuring KMS etcd encryption
* As a cluster admin, I want to easily understand the operations done by operators when managing the KMS plugin lifecycle via Conditions in the respective operator Status
* As a cluster admin, I want OpenShift to handle KMS authentication and credential management automatically, so that I don't need to manually provision credentials or manage plugin containers
* As a cluster admin, I want OpenShift to automatically detect when my KMS rotates encryption keys and handle the transition seamlessly, so that I don't need to manually intervene during key rotation
* As a cluster admin, I want to monitor KMS plugin health through standard OpenShift operator conditions and alerts, so that I can detect and respond to KMS-related issues

### Goals

* Fully automated KMS plugin lifecycle management, eliminating the need for manual plugin deployment, configuration, or updates
* Support for multiple KMS providers through a unified configuration model with provider-specific extensions
* Automatic credential provisioning and management for KMS authentication, eliminating manual credential setup
* Automatic detection and handling of KMS key rotation, including multi-key plugin configuration during migration
* Comprehensive monitoring of KMS plugin health, enabling proactive detection of encryption issues

### Non-Goals

* Direct support for hardware security modules (HSMs) - supported via KMS plugins (Thales)
* KMS provider deployment or management (users manage their own AWS KMS, Vault, etc.)
* Encryption controller logic for key rotation (see [KMS Encryption Foundations](kms-encryption-foundations.md))
* Migration between different KMS providers (deferred to [KMS Migration and Recovery](kms-migration-recovery.md) for GA)
* Recovery procedures for KMS key loss or temporary outages (deferred to [KMS Migration and Recovery](kms-migration-recovery.md) for GA)
* Custom or user-provided KMS plugins (only officially supported providers)

## Proposal

Extend OpenShift's API server operators (kube-apiserver-operator, openshift-apiserver-operator, authentication-operator) to automatically inject KMS plugin sidecar containers when KMS encryption is configured. The plugin management framework is provider-agnostic at the infrastructure level, with provider-specific implementations for authentication, configuration, and deployment.

**Supported KMS Providers:**

| Provider                | Tech Preview     | GA                  | Primary Use Case                            |
|-------------------------|------------------|---------------------|---------------------------------------------|
| **HashiCorp Vault**     | ⚠️ Beta          | ✅ Production-ready | On-premises, multi-cloud, centralized KMS   |
| **AWS KMS**             | ❌ Not supported | ✅ Production-ready | Clusters running on AWS infrastructure      |
| **Thales**              | ❌ Not supported | ✅ Production-ready | HSM integration, regulatory compliance      |
| **Azure KMS**           | ❌ Not supported | ✅ Production-ready | Clusters running on Azure infrastructure    |
| **GCP KMS**             | ❌ Not supported | ✅ Production-ready | Clusters running on GCP infrastructure      |

### Workflow Description

#### Roles

**cluster admin** is a human user responsible for configuring and maintaining the cluster.

**KMS** is the external Key Management Service (AWS KMS, HashiCorp Vault, Thales HSM) responsible for storing and rotating encryption keys.

**KMS Plugin** is a gRPC service implementing the Kubernetes KMS v2 API, deployed as a sidecar container.

**API Server Operator** is the OpenShift operator (kube-apiserver-operator, openshift-apiserver-operator, or authentication-operator) responsible for managing API server deployments.

#### Initial KMS Configuration (AWS KMS Example)

1. The cluster admin creates an encryption key (KEK) in AWS KMS
2. The cluster admin grants the OpenShift cluster access to the KMS key:
   - For kube-apiserver: Updates master node IAM role with KMS permissions
   - For openshift/oauth-apiserver: Ensures Cloud Credential Operator (CCO) can provision credentials
3. The cluster admin updates the APIServer configuration:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   metadata:
     name: cluster
   spec:
     encryption:
       type: KMS
       kms:
         type: AWS
         aws:
           keyARN: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
           region: us-east-1
   ```
4. The API server operators detect the configuration change
5. The operators inject AWS KMS plugin sidecar containers into API server pods
6. The KMS plugins start and communicate with AWS KMS
7. Encryption controllers detect the new KMS configuration and begin encryption (see [KMS Encryption Foundations](kms-encryption-foundations.md))
8. The cluster admin observes progress via `clusteroperator/kube-apiserver` conditions

#### Vault KMS Configuration (Tech Preview - AppRole)

1. The cluster admin deploys HashiCorp Vault (external to OpenShift)
2. The cluster admin creates an encryption key in Vault
3. The cluster admin configures AppRole authentication in Vault
4. The cluster admin creates Kubernetes secrets containing AppRole credentials:
   ```bash
   oc create secret generic vault-kms-credentials -n openshift-kube-apiserver \
     --from-literal=role-id=<role-id> \
     --from-literal=secret-id=<secret-id>
   ```
5. The cluster admin updates the APIServer configuration:
   ```yaml
   spec:
     encryption:
       type: KMS
       kms:
         type: Vault
         vault:
           vaultAddress: https://vault.example.com:8200
           keyPath: transit/keys/openshift-encryption
           authMethod: AppRole
           credentialsSecret:
             name: vault-kms-credentials
   ```
6. The operators inject Vault KMS plugin sidecars with AppRole credentials
7. Plugins authenticate to Vault and enable encryption

**Note:** AppRole is for Tech Preview only. GA will require certificate-based authentication (see Graduation Criteria).

#### Vault KMS Configuration (GA - Certificate Auth)

1. The cluster admin deploys Vault and configures PKI
2. The cluster admin configures initial AppRole credentials (bootstrap only)
3. OpenShift operators inject Vault KMS plugin sidecars
4. The KMS plugin uses AppRole to authenticate to Vault (first time only)
5. The plugin requests a client certificate from Vault PKI
6. The plugin stores the certificate and switches to certificate-based auth
7. The plugin automatically rotates certificates before expiration
8. AppRole credentials can be revoked after certificate issuance

This provides stronger security than AppRole-only while solving the bootstrap problem.

### API Extensions

This enhancement uses the KMS API types defined in [KMS Encryption Foundations](kms-encryption-foundations.md), which provides the foundational API for KMS encryption.

**For Tech Preview:**
- [KMS Encryption Foundations](kms-encryption-foundations.md) defines `KMSConfig` with only AWS support (`KMSProviderType` enum contains only `AWS`)
- [KMS Encryption Foundations](kms-encryption-foundations.md) defines `AWSKMSConfig` for AWS-specific configuration
- This enhancement focuses on managing the AWS KMS plugin lifecycle using that API

**For GA:**
- [KMS Encryption Foundations](kms-encryption-foundations.md) will extend the `KMSProviderType` enum to include `Vault` and `Thales`
- [KMS Encryption Foundations](kms-encryption-foundations.md) will add `VaultKMSConfig` and `ThalesKMSConfig` types
- This enhancement will document the provider-specific plugin management details

**Example API usage (defined in [KMS Encryption Foundations](kms-encryption-foundations.md)):**

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: KMS
    kms:
      type: AWS
      aws:
        keyARN: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
        region: us-east-1
```

For the complete API definitions, see the "API Extensions" section in [KMS Encryption Foundations](kms-encryption-foundations.md).

#### Provider-Specific Configuration Details (For Reference)

This section provides examples of provider-specific configurations that will be supported. The actual API types are defined in [KMS Encryption Foundations](kms-encryption-foundations.md).

**AWS KMS Configuration (Tech Preview - Supported):**
- `keyARN`: AWS KMS key ARN (required)
- `region`: AWS region (required)

**HashiCorp Vault Configuration (GA - Not in Tech Preview):**
- `vaultAddress`: Vault server URL
- `keyPath`: Path to encryption key in Vault
- `namespace`: Vault namespace (Enterprise only)
- `authMethod`: Authentication method (AppRole or Cert)
- `credentialsSecret`: Reference to secret containing auth credentials

**Thales CipherTrust Configuration (GA - Not in Tech Preview):**
- `p11LibraryPath`: Path to PKCS#11 library
- `keyLabel`: HSM key label
- `kekID`: Key Encryption Key ID
- `algorithm`: Encryption algorithm (rsa-oaep, aes-gcm)
- `credentialsSecret`: Reference to secret containing PKCS#11 PIN

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift, the control plane runs in a management cluster while workloads run in a guest cluster. KMS plugin management differs:

- **kube-apiserver**: Runs in management cluster, plugins deployed there
- **Credential access**: Management cluster must have access to KMS (network, IAM)
- **Plugin images**: Pulled in management cluster registry

No fundamental blockers, but credential provisioning may require manual setup in management cluster.

#### Standalone Clusters

This is the primary target for Tech Preview. All API servers and plugins run in the same cluster.

#### Single-node Deployments or MicroShift

Single-node deployments are supported. Resource consumption:
- Each API server pod adds one KMS plugin sidecar (~50MB memory, minimal CPU)
- Total: 3 sidecars for 3 API servers (kube-apiserver, openshift-apiserver, oauth-apiserver)

MicroShift may adopt this enhancement but will likely use file-based configuration instead of APIServer CR.

### Implementation Details/Notes/Constraints

This section is divided into two parts:
1. **Common Plugin Management Framework** - Shared infrastructure and mechanisms used by all KMS providers
2. **Provider-Specific Implementations** - Details specific to each KMS provider (Vault, AWS, Azure, GCP, Thales)

---

## Common Plugin Management Framework

The following components and mechanisms are shared across all KMS providers.

### Sidecar Deployment Architecture

KMS plugins are deployed as **sidecar containers** running alongside each API server. Each of the three API server operators manages its own KMS plugin sidecar instance.

**Deployment Model:**

| API Server          | Deployment Type | hostNetwork | Socket Volume | Operator                                 |
|---------------------|-----------------|-------------|---------------|------------------------------------------|
| kube-apiserver      | Static Pod      | true        | hostPath      | cluster-kube-apiserver-operator          |
| openshift-apiserver | Deployment      | false       | emptyDir      | cluster-openshift-apiserver-operator     |
| oauth-apiserver     | Deployment      | false       | emptyDir      | cluster-authentication-operator          |

**Key characteristics:**
- **One sidecar per API server pod**: Each kube-apiserver, openshift-apiserver, and oauth-apiserver pod gets its own KMS plugin sidecar
- **Pod-level isolation**: Socket volumes are pod-specific (hostPath for static pods, emptyDir for Deployments)
- **Provider-agnostic injection**: Same sidecar mechanism works for all providers (Vault, AWS, Azure, GCP, Thales)

### Shared library-go Components

The implementation leverages `library-go/pkg/operator/encryption/kms/` which provides shared code used by all three operators:

1. **Container Configuration**: `ContainerConfig` struct encapsulating KMS plugin container spec
2. **Volume Management**: Functions to create socket and credential volumes based on deployment type
3. **Pod Injection Logic**: `AddKMSPluginToPodSpec()` function for sidecar injection
4. **Socket Path Generation**: `GenerateUnixSocketPath()` builds paths based on KMS configuration hash

All three API server operators import and use these shared components, ensuring consistency.

### Configuration Detection and Reactivity

All operators watch the cluster-scoped `config.openshift.io/v1/APIServer` resource.

**Triggers for sidecar injection:**
- `spec.encryption.type` is set to `"KMS"`
- `spec.encryption.kms.type` specifies a supported provider (AWS, Vault, Azure, GCP, Thales)

**Reactivity to changes:**
1. **APIServer resource changes**: Configuration or provider updates trigger reconciliation
2. **Secret changes** (Deployment-based API servers): Credential updates trigger reconciliation
3. **Operator environment variables**: `KMS_PLUGIN_IMAGE` changes trigger rollout

**Update flow when KMS configuration changes:**
1. Operator detects APIServer configuration change
2. New deployment/static pod revision created with updated configuration
3. Rolling update replaces old pods with new ones
4. Old pods continue serving until new pods are ready
5. No socket conflicts (pod-level volume isolation)

### Socket Communication

All KMS plugins communicate with their respective API servers via Unix domain sockets using the Kubernetes KMS v2 gRPC API.

**Socket details:**
- **Base path**: `/var/run/kmsplugin/`
- **Socket naming**: `kms-<config-hash>.sock` (unique per configuration)
- **Volume name**: `kms-plugin-socket`
- **Protocol**: gRPC over Unix domain socket (KMS v2 API)
- **Permissions**: Socket owned by both API server and plugin container

**Why per-config sockets?**
- Supports multiple KMS configurations during migration (old + new plugins running simultaneously)
- Prevents conflicts during rolling updates
- Enables safe configuration changes without downtime

### Sidecar Injection Points

Each operator injects the KMS plugin sidecar at a specific point in its reconciliation loop:

**cluster-kube-apiserver-operator:**
- Injection point: `targetconfigcontroller.managePods()`
- Modifies static pod manifest before writing to pod ConfigMap
- Uses `hostPath` volume pointing to `/var/run/kmsplugin` on host

**cluster-openshift-apiserver-operator:**
- Injection point: `workload.manageOpenShiftAPIServerDeployment_v311_00_to_latest()`
- Modifies deployment spec after setting input hashes
- Uses `emptyDir` volume for socket isolation

**cluster-authentication-operator (oauth-apiserver):**
- Injection point: `workload.syncDeployment()`
- Modifies deployment spec after setting input hashes
- Uses `emptyDir` volume for socket isolation

### Plugin Image Management

KMS plugin container images are specified via environment variables on each operator:
- `KMS_PLUGIN_IMAGE` environment variable set on operator deployment
- For Tech Preview: Users may need to manually specify image
- For GA: Plugin images included in OpenShift release payload (automatic)

### Plugin Configuration Storage and Versioning

**Design Principle: Plugin Lifecycle Mirrors Encryption Configuration Lifecycle**

Since encryption configurations are stored as versioned secrets in the `openshift-config-managed` namespace, KMS plugin configurations follow the same pattern. This ensures that **API servers always use the newest plugin configuration**, even during rollback scenarios.

**Why this matters:**

When an API server rolls back to a previous revision, it must still be able to decrypt resources that were encrypted with newer keys. The current behavior for encryption configurations is:

- EncryptionConfig stored as secret: `encryption-config-<apiserver>-<revision>`
- During rollback: old API server revision reads the **newest** EncryptionConfig secret (not the previous revision's config)
- Reason: Must be able to decrypt resources encrypted with the new key

**Plugin configurations must adopt the same behavior:**

- Plugin config stored as ConfigMap: `kms-plugin-config-<apiserver>-<revision>`
- During rollback: old API server revision uses the **newest** plugin configuration
- Reason: Must be able to decrypt resources encrypted with the new key (plugin must have access to new key materials)

**Storage Mechanism:**

Each operator stores KMS plugin configurations as versioned ConfigMaps in the `openshift-config-managed` namespace:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kms-plugin-config-kube-apiserver-15
  namespace: openshift-config-managed
  labels:
    encryption.apiserver.operator.openshift.io/component: kube-apiserver
    encryption.apiserver.operator.openshift.io/revision: "15"
data:
  plugin-config.yaml: |
    # Sidecar container configuration
    containers:
    - name: aws-kms-plugin
      image: registry.redhat.io/openshift4/aws-kms-plugin:v4.17
      args:
      - --key=arn:aws:kms:us-east-1:123456789012:key/new-key-id
      - --region=us-east-1
      - --listen=/var/run/kmsplugin/kms-abc123.sock
      # ... full container spec
  key-id: "arn:aws:kms:us-east-1:123456789012:key/new-key-id"
```

**Retrieval During Reconciliation:**

When any operator reconciles its API server deployment (including after rollback), it:

1. Lists all plugin config ConfigMaps for its component
2. Sorts by revision number
3. Selects the **newest** (highest revision) regardless of API server revision
4. Applies that configuration to the sidecar container

```go
// In operator reconciliation loop
func (c *KMSPluginController) getNewestPluginConfig() (*PluginConfig, error) {
    // List all plugin config ConfigMaps
    cmList, err := c.configMapClient.List(
        "openshift-config-managed",
        metav1.ListOptions{
            LabelSelector: "encryption.apiserver.operator.openshift.io/component=" + c.component,
        },
    )

    // Sort by revision label, take newest
    sort.SliceStable(cmList.Items, func(i, j int) bool {
        revI, _ := strconv.Atoi(cmList.Items[i].Labels["encryption.apiserver.operator.openshift.io/revision"])
        revJ, _ := strconv.Atoi(cmList.Items[j].Labels["encryption.apiserver.operator.openshift.io/revision"])
        return revI < revJ
    })

    newest := cmList.Items[len(cmList.Items)-1]
    return parsePluginConfig(newest.Data["plugin-config.yaml"])
}

// Apply newest config to sidecar, regardless of API server revision
func (c *KMSPluginController) syncKMSPluginSidecar(ctx context.Context) error {
    newestConfig := c.getNewestPluginConfig()

    // Inject sidecar with newest plugin configuration
    return c.injectSidecar(newestConfig)
}
```

**Rollback Scenario Example:**

```
Timeline:
T=0:   API Server revision 10, KMS plugin config revision 10 (old key)
T=10:  Cluster admin updates KMS key
T=15:  API Server revision 11, KMS plugin config revision 11 (new key)
T=20:  Data migration in progress (old key → new key)
T=30:  Some resources encrypted with new key
T=35:  API Server rollback to revision 10 (due to unrelated issue)

What happens during rollback:
1. API server revision 10 pod starts
2. Operator reconciles: "I need to inject KMS plugin sidecar"
3. Operator calls getNewestPluginConfig()
   - Finds: plugin-config-kube-apiserver-10 (old key)
   - Finds: plugin-config-kube-apiserver-11 (new key)
   - Selects: revision 11 (newest)
4. Operator injects sidecar with NEW key configuration
5. API server revision 10 pod can now decrypt resources encrypted with new key ✓
6. Reads newest EncryptionConfig secret (per existing behavior)
7. EncryptionConfig references both old and new keys during migration
8. Rollback succeeds without data loss
```

**Benefits:**

- ✅ **Consistency**: Plugin lifecycle mirrors encryption config lifecycle
- ✅ **Rollback safety**: Old API server revisions can decrypt new data
- ✅ **Forward-only progression**: Plugin configs only move forward, never backward
- ✅ **Decoupled versioning**: API server revision ≠ plugin config revision

**During KEK Rotation:**

When a KEK changes, operators create new plugin config revisions:

```yaml
# Before rotation
kms-plugin-config-kube-apiserver-15:
  key-id: "old-key"

# During rotation (both configs exist)
kms-plugin-config-kube-apiserver-15:
  key-id: "old-key"
kms-plugin-config-kube-apiserver-16:
  key-id: "new-key"

# After rotation completes
kms-plugin-config-kube-apiserver-16:
  key-id: "new-key"
# (revision 15 pruned after migration completes)
```

All 3 API servers independently read the newest config, ensuring they can all decrypt resources encrypted with the new key.

**Cleanup:**

Old plugin config ConfigMaps are pruned using the same mechanism as encryption key secrets:
- Retained until data migration completes
- Pruned by the pruneController after all resources migrated
- Follows same retention policy as encryption configs (keep N recent revisions)

### KEK Rotation and Multi-Revision Deployment

When a KMS provider rotates the Key Encryption Key (KEK), OpenShift must support both the old and new keys simultaneously during the data re-encryption period. This is achieved by running **two complete revisions of each API server deployment/static pod** - one configured with the old KEK and one with the new KEK.

**Design Principle: Provider-Agnostic Approach**

While some KMS plugins (notably AWS KMS) support configuring multiple keys within a single plugin process, OpenShift adopts a **uniform multi-revision approach** for all providers:

- **Two full API server pods** run side-by-side during KEK rotation (each with its own KMS plugin sidecar)
- One pod configured with old KEK, one with new KEK
- Each pod uses a distinct Unix socket path for its plugin (based on config hash)
- Both pods remain active until data migration completes
- Old pod revision removed after migration finishes

**Why always use two pod revisions?**

1. **Provider uniformity**: Most KMS plugins (Vault, Azure, GCP, Thales) cannot handle multiple keys in one process
2. **Consistent behavior**: Users see the same rollout pattern regardless of KMS provider
3. **Simplified operators**: No provider-specific branching logic for single-plugin vs multi-plugin configurations
4. **Standard Kubernetes patterns**: Leverages existing rolling update mechanisms
5. **Easier testing**: Single code path for all providers

**Rotation Workflow:**

1. KMS rotates KEK externally (key materials change, `key_id` changes)
2. Operators detect `key_id` change via plugin Status gRPC call
3. **Operators create new plugin config ConfigMap** with new key configuration (see "Plugin Configuration Storage and Versioning" section)
   - New ConfigMap: `kms-plugin-config-<apiserver>-<new-revision>`
   - Contains sidecar spec with new key ARN/ID
   - Both old and new configs coexist during migration
4. Operators create new deployment/static pod revision with new KMS plugin configuration
   - Operator retrieves **newest** plugin config (the new revision)
   - Injects sidecar with new key configuration
5. Both pod revisions run simultaneously (old KEK + new KEK)
   - Old pod: reads newest plugin config → gets new key config ✓
   - New pod: reads newest plugin config → gets new key config ✓
   - **Both can decrypt resources encrypted with new key**
6. Encryption controllers (Enhancement A) migrate data from old KEK to new KEK
7. Once migration completes, operators remove old pod revision
8. Old plugin config ConfigMap pruned (see "Cleanup" in storage section)
9. Only new revision remains, ready for next rotation

**Socket Path Isolation:**

The per-config socket naming (`kms-<config-hash>.sock`) prevents conflicts between the two revisions:
- Old revision: `/var/run/kmsplugin/kms-abc123.sock`
- New revision: `/var/run/kmsplugin/kms-def456.sock`

The API server's `EncryptionConfiguration` (managed by stateController in Enhancement A) references both sockets during migration, with the new key as the write key and the old key available for decryption only.

**Rollback During Rotation:**

If an API server rollback occurs during KEK rotation, the "always use newest plugin config" pattern ensures data integrity:

```
Scenario: KEK rotation in progress, API server rollback occurs

Before rollback:
- API Server revision N+1 (new)
- Plugin config revision M+1 (new key)
- EncryptionConfig has both old and new keys
- Some resources encrypted with new key

During rollback:
- API server rolls back to revision N (old)
- Operator reconciles and retrieves newest plugin config (M+1)
- Injects sidecar with NEW key configuration
- Old API server revision can decrypt resources encrypted with new key ✓

Result: Rollback succeeds without data loss
```

This mirrors the existing behavior where API servers always read the newest EncryptionConfig secret, ensuring rollback safety.

---

## Provider-Specific Implementations

Each KMS provider has unique requirements for deployment, authentication, and configuration. This section details the provider-specific aspects.

### Vault KMS Plugin (Tech Preview / GA)

**Status:** Tech Preview (if Vault plugin ready), GA (production-ready)

TODO: Document Vault plugin deployment and auth

**Static Pod Constraint:**

**Critical limitation**: Static pods (kube-apiserver) **cannot reference ServiceAccount objects** (Kubernetes limitation).

This impacts Vault authentication options:

**Vault Kubernetes Auth Method - NOT VIABLE:**
- Requires ServiceAccount tokens at `/var/run/secrets/kubernetes.io/serviceaccount/token`
- Static pods cannot have ServiceAccount tokens
- **Cannot be used for kube-apiserver**
- **CAN be used for openshift-apiserver and oauth-apiserver** (Deployments)

**Vault JWT Auth Method - NOT VIABLE:**
- Also requires ServiceAccount tokens
- Same static pod limitation

**Vault AppRole Auth Method - VIABLE (Tech Preview):**
- Uses static credentials (RoleID + SecretID)
- Can be mounted as Secret volumes in static pods
- **Security concerns**: Shared secret, manual rotation, bootstrap problem
- Acceptable for Tech Preview with documented limitations

**Vault Cert Auth Method - VIABLE (GA):**
- Uses TLS client certificates for authentication
- Certificates stored as files, mounted in static pods
- **Bootstrap flow**: AppRole → get cert from Vault PKI → use cert auth → rotate cert automatically
- Solves AppRole security concerns
- **Recommended for GA**

### AWS KMS Plugin (Tech Preview / GA)

**Status:** Tech Preview (likely), GA (production-ready)

The AWS KMS plugin enables encryption using AWS Key Management Service.

**Credential Management:**

AWS KMS plugin authentication differs based on which API server it's running in:

| API Server          | Deployment Type | Credential Source | Authentication Method |
|---------------------|-----------------|-------------------|-----------------------|
| kube-apiserver      | Static Pod      | EC2 IMDS          | IAM Instance Profile  |
| openshift-apiserver | Deployment      | Secret (CCO)      | IAM User Credentials  |
| oauth-apiserver     | Deployment      | Secret (CCO)      | IAM User Credentials  |

**For kube-apiserver (Static Pod with hostNetwork: true):**

The KMS plugin sidecar accesses AWS credentials through the EC2 Instance Metadata Service (IMDS). The master node's IAM role must have the following KMS permissions:

```json
{
  "Effect": "Allow",
  "Action": [
    "kms:Encrypt",
    "kms:Decrypt",
    "kms:DescribeKey",
    "kms:GenerateDataKey"
  ],
  "Resource": "<kms-key-arn>"
}
```

**User responsibility:** Cluster admins must configure the master node IAM role with these permissions before enabling KMS encryption. Documentation and helper guidance will be provided.

**For openshift-apiserver and oauth-apiserver (Deployments with hostNetwork: false):**

These API servers cannot access IMDS directly (no hostNetwork), so they use AWS credentials from Kubernetes Secrets created by the Cloud Credential Operator (CCO).

The operators will include CredentialsRequest resources:
- `openshift-apiserver-kms-credentials-request.yaml`
- `oauth-apiserver-kms-credentials-request.yaml`

When CCO operates in **Mint mode**, it automatically:
1. Creates IAM users with KMS permissions
2. Provisions `kms-credentials` secret in each API server's namespace
3. Secret contains AWS access key ID and secret access key

The operators watch for these secrets and only inject the KMS plugin sidecar once credentials are available.

**Graceful Degradation:**

If KMS encryption is enabled but CCO credentials aren't ready yet:
1. Operator logs a warning indicating credentials are pending
2. Sidecar injection is skipped (return nil, not error)
3. API server deployment proceeds without KMS sidecar
4. On next reconciliation (when credentials available), sidecar is automatically injected

This prevents blocking API server rollouts while waiting for CCO to provision credentials.

**Configuration Example:**

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: KMS
    kms:
      type: AWS
      aws:
        keyARN: arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012
        region: us-east-1
```

**Known Limitations:**

- **AWS KMS key_id limitation**: AWS KMS does not change `key_id` when it rotates key materials internally. This means automatic rotation detection (via `key_id` changes) does not work for AWS-managed rotation. Users must manually update the `keyARN` in APIServer config to trigger rotation in OpenShift.
- **Workaround for GA**: Consider polling AWS KMS API directly to detect key version changes, or clearly document that users must update config for rotation

### Azure KMS Plugin (GA)

**Status:** GA only (not in Tech Preview)

TODO: Document Azure plugin deployment and authentication

**Expected authentication approach:**
- Managed Identity for kube-apiserver (if Azure supports workload identity)
- CCO-provisioned credentials for openshift-apiserver and oauth-apiserver

### GCP KMS Plugin (GA)

**Status:** GA only (not in Tech Preview)

TODO: Document GCP plugin deployment and authentication

**Expected authentication approach:**
- Workload Identity for kube-apiserver
- CCO-provisioned service account credentials for openshift-apiserver and oauth-apiserver

### Thales KMS Plugin (GA)

**Status:** GA (Tech Preview uncertain)

TODO: Document Thales/HSM plugin requirements

**Expected considerations:**
- PKCS#11 library integration
- HSM device access (network HSM vs local TPM)
- PIN management and security

#### KMS Plugin Health Monitoring

API server operators are responsible for monitoring KMS plugin health and surfacing plugin status to cluster administrators and to encryption controllers.

**Health Check Implementation:**

Each operator implements health checks for its KMS plugin sidecar:

1. **gRPC Status Endpoint Calls**
   - Periodically call the KMS plugin's Status gRPC endpoint (defined by KMS v2 API)
   - Parse the response to extract:
     - Plugin health status (healthy/unhealthy)
     - Current `key_id` (used by encryption controllers for rotation detection)
     - Plugin version and other metadata

2. **Health Check Frequency**
   - Default: Poll Status endpoint every 30 seconds
   - Configurable via operator environment variable (for tuning)
   - Exponential backoff on repeated failures

3. **Failure Detection**
   - Plugin process not running (container crashed)
   - Status endpoint unreachable (socket connection failed)
   - Status endpoint returns error response
   - Status call times out (after 10 seconds)

**Operator Condition Integration:**

Plugin health is reflected in operator conditions visible to administrators:

```yaml
status:
  conditions:
  - type: KMSPluginDegraded
    status: "False"
    reason: KMSPluginHealthy
    message: "KMS plugin is healthy and responding to Status calls"

  # When plugin is unhealthy:
  - type: KMSPluginDegraded
    status: "True"
    reason: KMSPluginUnhealthy
    message: "KMS plugin Status endpoint unreachable: connection refused"
```

**Metrics and Alerts:**

Operators expose metrics for monitoring:
- `kms_plugin_status_call_duration_seconds` - Histogram of Status call latency
- `kms_plugin_status_call_errors_total` - Counter of failed Status calls
- `kms_plugin_healthy` - Gauge (1 = healthy, 0 = unhealthy)

Alerts fire when plugin health degrades:
- `KMSPluginUnhealthy` - Plugin has been unhealthy for >5 minutes
- `KMSPluginStatusCallLatencyHigh` - Status calls taking >5 seconds

**Controller Precondition Integration:**

Operators provide a health check function to encryption controllers:

```go
// Provided by operator to library-go controllers
func kmsPluginHealthCheck(ctx context.Context) (bool, error) {
    // Check cached health status (updated by periodic Status polling)
    if !cachedPluginHealth.IsHealthy() {
        return false, fmt.Errorf("KMS plugin unhealthy: %s", cachedPluginHealth.Reason)
    }
    return true, nil
}

// Controllers use this as a precondition
controllers.NewKeyController(
    // ... other params ...
    preconditions: []PreconditionFunc{
        kmsPluginHealthCheck,  // Block controller sync if plugin unhealthy
        // ... other preconditions ...
    },
)
```

**Restart and Recovery Logic:**

When plugin health checks fail:

1. **Short-term failures (< 1 minute):**
   - Log warnings
   - Set operator condition to Degraded
   - Controllers block but cluster remains operational (kube-apiserver caches DEKs)

2. **Medium-term failures (1-5 minutes):**
   - Attempt container restart (if process crashed)
   - Check for configuration issues (invalid credentials, network problems)
   - Fire alerts

3. **Long-term failures (> 5 minutes):**
   - Operator condition remains Degraded
   - Alerts continue firing
   - Manual intervention required (see Support Procedures)

**Provider-Specific Health Considerations:**

Different KMS plugins may have provider-specific health indicators:

- **AWS KMS Plugin:** Check AWS credential validity, KMS API reachability
- **Vault KMS Plugin:** Check Vault token/cert expiration, Vault service health
- **Thales KMS Plugin:** Check HSM device connectivity, PKCS#11 library availability

**Tech Preview vs GA:**

- **Tech Preview:** Basic health checking (Status endpoint polling, operator conditions)
- **GA:** Full monitoring suite (metrics, alerts, automatic recovery, dashboard integration)

### Risks and Mitigations

TODO: Migrate from current enhancement and add provider-specific risks

### Drawbacks

TODO: Update from current enhancement

## Alternatives (Not Implemented)

TODO: Migrate alternatives section

## Open Questions

1. Should we support mixed authentication methods (e.g., Kubernetes auth for openshift-apiserver, AppRole for kube-apiserver)?
2. How do we handle Vault plugin beta availability? Make Vault support conditional on plugin release?
3. Thales HSM device access - how do control plane nodes access HSMs (network HSM vs USB vs embedded TPM)?

## Test Plan

TODO: Define test strategy for multi-provider support

## Graduation Criteria

### Tech Preview Acceptance Criteria

**AWS KMS Provider:**
- ✅ Full support with IAM authentication
- ✅ Sidecar deployment across all 3 API servers
- ✅ Automatic credential provisioning via CCO (openshift/oauth) and IMDS (kube-apiserver)
- ✅ Configuration via APIServer CR
- ✅ Basic monitoring and health checks

**Vault KMS Provider:**
- ⚠️ Best-effort support (depends on Vault plugin beta release)
- ⚠️ AppRole authentication only (security limitations documented)
- ⚠️ Manual credential setup required
- ⚠️ Marked as experimental, subject to change

**Thales KMS Provider:**
- ❌ Not supported in Tech Preview
- 📅 Deferred to GA

**Feature Gate:**
- Behind `KMSEncryptionProvider` feature gate
- Disabled by default

### Tech Preview → GA

**AWS KMS Provider:**
- ✅ Production-ready, full SLO coverage
- ✅ Load testing completed
- ✅ Monitoring, alerts, runbooks defined
- ✅ Documentation in openshift-docs

**Vault KMS Provider:**
- ✅ Certificate-based authentication required
- ✅ Automatic cert rotation by plugin
- ✅ AppRole used only for bootstrap
- ✅ Static pod auth limitations documented
- ✅ Vault plugin reaches GA release
- ✅ Production validation complete

**Thales KMS Provider:**
- ✅ HSM integration validated (network/USB/TPM)
- ✅ Secure PIN management strategy
- ✅ PKCS#11 library compatibility tested
- ✅ Device access requirements documented

**Feature Gate:**
- Removed (enabled by default)

## Upgrade / Downgrade Strategy

### Upgrade

**From version without KMS plugin management to version with:**

- No user action required if not using KMS encryption
- If KMS encryption enabled: Operators automatically deploy KMS plugin sidecars
- Plugin config ConfigMaps created in `openshift-config-managed` namespace
- Seamless transition during upgrade

**During upgrade:**

- KMS plugin sidecar images updated with operator upgrade
- Existing sidecars restarted with new image version
- No encryption downtime (rolling update, pod-level sidecar isolation)

**Plugin configuration behavior during upgrade:**

- Operators always read newest plugin config ConfigMap (see "Plugin Configuration Storage and Versioning" section)
- If upgrade creates new plugin config revision, all API servers (old and new) use newest config
- Ensures backward compatibility: old API server revisions can decrypt resources encrypted with new keys

### Downgrade

**From version with KMS plugin management to version without:**

- If KMS encryption enabled with managed plugins: **Cannot downgrade without migration**
- User must first migrate to different encryption provider or disable encryption
- Migration requires updating APIServer config and waiting for data re-encryption

**Procedure:**

1. Update APIServer config to use non-KMS encryption (type: aescbc) or disable encryption (type: identity)
2. Wait for migration to complete (all resources re-encrypted)
3. Downgrade OpenShift version
4. KMS plugin sidecars removed (not deployed by older operators)

**Important:** Plugin config ConfigMaps remain in `openshift-config-managed` namespace after downgrade. These are harmless but can be manually deleted if desired.

## Version Skew Strategy

### Operator Version Skew

During cluster upgrade, operators may be at different versions:
- kube-apiserver-operator upgraded, injects new plugin sidecar version
- openshift-apiserver-operator still on old version, injects old sidecar or no sidecar

**Impact:** Some API servers have new plugin sidecars, others have old sidecars

**Mitigation:**
- Plugin API (KMS v2) is stable across versions
- Old and new plugin versions can coexist
- Each API server operates independently with its own sidecar
- **Plugin config "always use newest" pattern ensures compatibility:**
  - Old operator reads newest plugin config ConfigMap
  - New operator reads newest plugin config ConfigMap
  - Both inject sidecars compatible with newest key configuration
  - No coordination required between operators

### API Server vs Plugin Version Skew

API server updated but KMS plugin unchanged (or vice versa).

**Impact:** Minimal - KMS v2 API is stable

**Mitigation:**
- Sidecars implement standard KMS v2 API (stable interface)
- API servers consume KMS v2 API (stable interface)
- No version coordination required
- Plugin config ConfigMaps versioned independently of API server revisions

### During KEK Rotation with Version Skew

**Scenario:** KEK rotation occurs while operators are at different versions

**Example:**
```
T=0:  kube-apiserver-operator at v4.17, creates plugin config revision 10 (old key)
T=10: Cluster upgrade begins
T=15: kube-apiserver-operator upgraded to v4.18
T=20: External KMS rotates key
T=25: kube-apiserver-operator (v4.18) detects rotation, creates plugin config revision 11 (new key)
T=30: openshift-apiserver-operator still at v4.17
```

**What happens:**

- kube-apiserver-operator (v4.18):
  - Reads newest plugin config → gets revision 11 (new key) ✓
  - Injects sidecar with new key
  - Works correctly

- openshift-apiserver-operator (v4.17):
  - Reads newest plugin config → gets revision 11 (new key) ✓
  - Injects sidecar with new key
  - Works correctly (KMS v2 API stable)

**Result:** Version skew has no impact. The "always use newest plugin config" pattern ensures all operators inject sidecars compatible with the current key configuration, regardless of operator version.

**No coordination needed between operators** - they independently converge on the newest plugin configuration.

## Operational Aspects of API Extensions

TODO: Document operational impact

## Support Procedures

TODO: Define support runbooks per provider

## Infrastructure Needed

TODO: List infrastructure requirements (test KMS instances, HSMs, etc.)
