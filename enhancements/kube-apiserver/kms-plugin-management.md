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
  - "@sjenning"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-01-28
last-updated: 2025-01-28
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-108"  # TP feature
  - "https://issues.redhat.com/browse/OCPSTRAT-1638" # GA feature
see-also:
  - "enhancements/kube-apiserver/kms-encryption-foundations.md"
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
replaces:
  - "https://github.com/openshift/enhancements/pull/1682"
superseded-by:
  - ""
---

# KMS Plugin Lifecycle Management

## Summary

Enable OpenShift to automatically manage the lifecycle of KMS (Key Management Service) plugins across multiple API servers. This enhancement provides a user-configurable interface to deploy, configure, and monitor KMS plugins as sidecar containers alongside kube-apiserver, openshift-apiserver, and oauth-apiserver pods. Support for multiple KMS providers (AWS KMS, HashiCorp Vault, Thales HSM) is included with provider-specific authentication and configuration.

## Motivation

KMS encryption requires KMS plugin pods to bridge communication between the kube-apiserver and external KMS providers. Managing these plugins manually is operationally complex and error-prone. OpenShift should handle plugin deployment, authentication, health monitoring, and lifecycle management automatically on behalf of users.

Different KMS providers have vastly different authentication models, deployment requirements, and operational characteristics. This enhancement provides a unified plugin management framework while accommodating provider-specific needs.

### User Stories

* As a cluster admin, I want to enable AWS KMS encryption by simply providing a key ARN in the APIServer config, so that OpenShift automatically deploys and manages the AWS KMS plugin for me
* As a cluster admin using HashiCorp Vault, I want OpenShift to handle Vault authentication (AppRole for TP, certificate-based for GA) and plugin deployment, so that I don't need to manually manage credentials or plugin containers
* As a cluster admin, I want to switch from one KMS provider to another (e.g., AWS KMS to Vault) by updating the APIServer configuration, so that OpenShift handles the plugin transition and data migration automatically
* As a cluster admin, I want to monitor KMS plugin health through standard OpenShift operators and alerts, so that I can detect and respond to KMS-related issues

### Goals

* Automatic KMS plugin deployment as sidecar containers in API server pods
* Support for multiple KMS providers with provider-specific configurations
* Credential management for KMS plugin authentication (IAM, AppRole, Cert, PKCS#11)
* Plugin health monitoring and integration with operator conditions
* Reactivity to configuration changes (automatic plugin updates)
* Support for Tech Preview (limited providers) and GA (full provider support) graduation

### Non-Goals

* Direct support for hardware security modules (HSMs) - supported via KMS plugins (Thales)
* KMS provider deployment or management (users manage their own AWS KMS, Vault, etc.)
* Encryption controller logic for key rotation (see [KMS Encryption Foundations](kms-encryption-foundations.md))
* Migration and recovery procedures (deferred to [KMS Migration and Recovery](kms-migration-recovery.md) for GA)
* Custom or user-provided KMS plugins (only officially supported providers)

## Proposal

Extend OpenShift's API server operators (kube-apiserver-operator, openshift-apiserver-operator, authentication-operator) to automatically inject KMS plugin sidecar containers when KMS encryption is configured. The plugin management framework is provider-agnostic at the infrastructure level, with provider-specific implementations for authentication, configuration, and deployment.

**Supported KMS Providers:**

| Provider | Tech Preview | GA | Primary Use Case |
|----------|--------------|-----|------------------|
| **AWS KMS** | ✅ Full support | ✅ Production-ready | Cloud-native AWS deployments |
| **HashiCorp Vault** | ⚠️ Beta (if Vault plugin available) | ✅ Production-ready | On-premises, multi-cloud, centralized KMS |
| **Thales CipherTrust** | ❌ Not supported | ✅ Production-ready | HSM integration, regulatory compliance |

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
           namespace: openshift  # Vault Enterprise namespace
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

#### KMS Plugin Deployment Architecture

KMS plugins are deployed as **sidecar containers** in API server pods. Each operator manages its own sidecar injection:

| API Server | Deployment Type | hostNetwork | Socket Volume | Credential Source |
|------------|-----------------|-------------|---------------|-------------------|
| kube-apiserver | Static Pod | ✅ true | hostPath | IAM (IMDS) or Secret |
| openshift-apiserver | Deployment | ❌ false | emptyDir | Secret (CCO) |
| oauth-apiserver | Deployment | ❌ false | emptyDir | Secret (CCO) |

#### Sidecar Injection Mechanism

TODO: Document sidecar injection implementation per operator

#### Provider-Specific Authentication

TODO: Document authentication mechanisms for each provider

#### Static Pod Limitations and Vault Auth

**Critical constraint**: Static pods (kube-apiserver) **cannot reference ServiceAccount objects** (Kubernetes limitation).

This has major implications for Vault authentication:

**Vault Kubernetes Auth Method - NOT VIABLE:**
- Requires ServiceAccount tokens mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`
- Static pods cannot have ServiceAccount tokens
- **Cannot be used for kube-apiserver**
- **CAN be used for openshift-apiserver and oauth-apiserver** (Deployments)

**Vault JWT Auth Method - NOT VIABLE:**
- Also requires ServiceAccount tokens
- Same static pod limitation applies

**Vault AppRole Auth Method - VIABLE (Tech Preview):**
- Uses static credentials (RoleID + SecretID)
- Can be mounted as Secret volumes in static pods
- **Security concerns**: Shared secret, manual rotation, bootstrap problem
- Acceptable for Tech Preview with documented limitations

**Vault Cert Auth Method - VIABLE (GA):**
- Uses TLS client certificates for authentication
- Certificates can be stored as files and mounted in static pods
- **Bootstrap flow**: AppRole → get cert from Vault PKI → use cert auth → rotate cert automatically
- Solves security concerns of AppRole-only
- **Recommended for GA**

#### AWS KMS Plugin Configuration

TODO: Migrate from current enhancement

#### Vault KMS Plugin Configuration

TODO: Document Vault plugin deployment and auth

#### Thales KMS Plugin Configuration

TODO: Document Thales/HSM plugin requirements

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

TODO: Define upgrade/downgrade procedures

## Version Skew Strategy

TODO: Define version skew handling

## Operational Aspects of API Extensions

TODO: Document operational impact

## Support Procedures

TODO: Define support runbooks per provider

## Infrastructure Needed

TODO: List infrastructure requirements (test KMS instances, HSMs, etc.)
