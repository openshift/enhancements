---
title: vault-kms-provider
authors:
  - "@flavianmissi"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on.
  - "@benluddy, for overall architecture and API design"
  - "@ardaguclu, for overall architecture and API design"
  - "@p0lyn0mial, for overall architecture and API design"
  - "@ibihim, for authentication and security aspects"
approvers:
  - "@benluddy"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-02-27
last-updated: 2026-03-18
tracking-link:
  - "https://issues.redhat.com/browse/CNTRLPLANE-2711"
see-also:
  - "enhancements/kube-apiserver/kms-encryption-foundations.md"
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
replaces: []
superseded-by: []
---

# HashiCorp Vault KMS Provider

## Summary

Extend the OpenShift APIServer configuration API to support HashiCorp Vault as
a KMS provider type. This enhancement introduces the `ManagedKMSProvider`
feature gate, the `ManagedKMS` encryption type, and adds Vault-specific
configuration fields (VaultKMSConfig) to the KMSConfig API, allowing the Vault
KMS provider to be configured declaratively alongside the existing AWS KMS
provider.

The `ManagedKMS` encryption type distinguishes operator-managed KMS deployments
from the simple/unmanaged `KMS` type (Tech Preview v1) where users manually
deploy plugins. This allows the API changes to land independently of the
library-go implementation. Eventually, the unmanaged `KMS` type will be removed
and `ManagedKMS` will be renamed to `KMS`.

## Motivation

### User Stories

### Goals

* Introduce the `ManagedKMSProvider` feature gate to control access to
  operator-managed KMS provider configuration
* Extend the `KMSConfig` API type to support Vault as a provider type
* Define Vault-specific configuration fields (VaultKMSConfig)

### Non-Goals

The following are explicitly out of scope for this enhancement and will be
covered by separate future enhancements:

* Deploying and managing Vault KMS plugin workloads (operators managing plugin
  pods/containers)
* **Image signature verification and security validation** of user-provided KMS
  plugin images (critical security enhancement planned for future work)
* Pre-flight validation and compatibility checks for KMS plugin images
* Plugin lifecycle management (updates, health monitoring, etc.)
* End-to-end functional implementation (this enhancement provides API only)

## Proposal

This enhancement proposes adding API support for configuring HashiCorp Vault as
a KMS provider. The API changes will be gated by the new `ManagedKMSProvider`
feature gate, but the implementation of the operators that act on this
configuration is out of scope for this enhancement.

### API Extensions

We propose extending the existing `KMSConfig` type in the APIServer
configuration resource to support Vault as a new KMS provider type. The
implementation follows the same pattern established for the AWS KMS
configuration.

#### Feature Gates

This enhancement introduces a new feature gate `ManagedKMSProvider` to control
access to Vault KMS provider configuration in the APIServer API. This gate
works in conjunction with the `KMSEncryption` feature gate from
[kms-encryption-foundations.md](./kms-encryption-foundations.md).

Additionally, this enhancement proposes removing the
`KMSEncryptionProvider` feature gate, which is superseded by `KMSEncryption`.

**ManagedKMSProvider:**

```go
FeatureGateManagedKMSProviders = newFeatureGate("ManagedKMSProvider").
    reportProblemsToJiraComponent("kube-apiserver").
    contactPerson("fmissi").
    productScope(ocpSpecific).
    enhancementPR("https://github.com/openshift/enhancements/pull/1954").
    enable(inTechPreviewNoUpgrade()).
    mustRegister()
```

When enabled, this gate allows:
- The `Vault` value in the `KMSProviderType` enum
- The `vault` field in the `KMSConfig` struct

The gate is named "ManagedKMSProvider" because it represents the overall
capability for OpenShift-managed KMS provider deployments, which is the end
goal across multiple enhancements. While this enhancement only adds the Vault
API configuration, future work may add other managed provider types. The
complete OpenShift-managed deployment functionality will be gated under this
same feature gate.

**Relationship to KMSEncryption and the ManagedKMS Encryption Type:**

This enhancement introduces the `ManagedKMS` encryption type to distinguish
operator-managed KMS deployments (where operators deploy and configure KMS
plugins based on API configuration) from the unmanaged KMS mode (where users
manually deploy plugins at a static socket path).

The `KMSEncryption` feature gate (from kms-encryption-foundations) provides:
- The `encryption.type: KMS` support (unmanaged KMS - Tech Preview v1)
- Foundational KMS mode implementation in library-go encryption controllers
  (hardcoded static endpoint)

The `ManagedKMSProvider` feature gate (this enhancement) provides:
- The `encryption.type: ManagedKMS` support (operator-managed KMS - Tech Preview v2)
- The `kms` field in APIServerEncryption with provider-specific configuration
- Vault as a KMS provider type

**Migration Path:**

The `KMS` encryption type (unmanaged) is intended as a temporary
implementation for Tech Preview v1. Once operators can deploy and manage KMS
plugins based on API configuration (plugin deployment and lifecycle management
functionality is implemented):

1. The unmanaged `KMS` encryption type will be deprecated and eventually removed
2. The `ManagedKMS` encryption type will be renamed to `KMS` to become the
   standard KMS implementation
3. Internal CI/CD tests, QE environments, and dev clusters using `type: KMS`
   (unmanaged) will need to migrate to `type: ManagedKMS` (managed) before the
   removal

Since both features are in `TechPreviewNoUpgrade`, there will be no production
customer impact. This allows the API changes in this enhancement to land
independently of the library-go implementation changes in
[PR #1960](https://github.com/openshift/enhancements/pull/1960), while
providing a clear migration path for internal test infrastructure

**Removing KMSEncryptionProvider:**

The `KMSEncryptionProvider` feature gate was introduced in [PR #1682](https://github.com/openshift/enhancements/pull/1682)
but was superseded by the `KMSEncryption` gate in [PR #1900](https://github.com/openshift/enhancements/pull/1900)
(kms-encryption-foundations). The old gate is non-functional and should be removed.
This enhancement proposes:

- Removing the `KMSEncryptionProvider` feature gate definition from
  `features/features.go`
- Updating API validation annotations to use only `KMSEncryption` (not
  `KMSEncryptionProvider`)

Since `KMSEncryptionProvider` is in `TechPreviewNoUpgrade` and has never been
functional, this removal has no customer impact.

#### Vault KMSConfig

The following config is extracted from [Vault KMS Plugin documentation](https://github.com/hashicorp/web-unified-docs/blob/ab6191e4856b52a59a87fe0f17703671a7317ec6/content/vault/v1.21.x/content/docs/deploy/kubernetes/kms/configuration.mdx).
At the time of writing, the plugin is in pre-alpha and the configuration
parameters might be subject to change.

**Field Validation Limits:**

The API enforces the following validation limits on VaultKMSConfig fields:

- **vaultKMSPluginImage** (75-512 chars): Minimum of 75 characters accounts for
  the shortest possible digest reference (`r/i@sha256:` + 64 hex characters).
  Maximum of 512 characters follows common Kubernetes practice for container
  image references and accommodates long registry hostnames and deep repository paths.
- **vaultAddress** (1-512 chars): Follows the established pattern in OpenShift
  API for service URLs, matching the OIDC `issuerURL` field limit (see
  [types_authentication.go:266](https://github.com/openshift/api/blob/master/config/v1/types_authentication.go#L266)).
  Sufficient for typical Vault server addresses with scheme, hostname, port,
  and optional short path.
- **vaultNamespace** (no limit): Vault Enterprise has [no hard limit](https://developer.hashicorp.com/vault/docs/internals/limits)
  on namespace paths (limited only by storage backend). We do not impose an
  artificial limit to avoid restricting valid Vault configurations.
- **tlsServerName** (1-253 chars): [RFC 1035](https://www.rfc-editor.org/rfc/rfc1035)
  defines DNS FQDN maximum as 253 characters (255 octets minus 2 for encoding).
  This is the correct standard limit for hostnames.
- **transitMount** (no limit): Vault has [no explicit limit](https://developer.hashicorp.com/vault/docs/internals/limits)
  on mount paths (constrained by storage entry size). We do not impose an
  artificial limit to allow any mount path that Vault accepts.
- **transitKey** (no limit): Vault Transit key names have [no documented hard limit](https://developer.hashicorp.com/vault/docs/internals/limits).
  We do not impose an artificial limit to allow any key naming convention that
  Vault accepts.

```go
// VaultKMSConfig defines the KMS plugin configuration specific to Vault KMS
type VaultKMSConfig struct {
    // vaultKMSPluginImage specifies the container image for the HashiCorp Vault KMS plugin.
    // The image must be specified using a digest reference (not a tag).
    //
    // Consult the OpenShift documentation for compatible plugin versions with your cluster version,
    // then obtain the image digest for that version from HashiCorp's container registry.
    //
    // For disconnected environments, mirror the plugin image to an accessible registry and
    // reference the mirrored location with its digest.
    //
    // The minimum length is 75 characters (e.g., "r/i@sha256:" + 64 hex characters).
    // The maximum length is 512 characters to accommodate long registry names and repository paths.
    //
    // +kubebuilder:validation:Pattern=`^([a-zA-Z0-9\-\.]+)(:[0-9]+)?/[a-zA-Z0-9\-\./]+@sha256:[a-f0-9]{64}$`
    // +kubebuilder:validation:MinLength=75
    // +kubebuilder:validation:MaxLength=512
    // +required
    VaultKMSPluginImage string `json:"vaultKMSPluginImage"`

	// vaultAddress specifies the address of the Vault instance.
	// The value must be a valid URL with scheme (http:// or https://) and can be between 1 and 512 characters.
	// Example: https://vault.example.com:8200
	//
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:MinLength=1
	// +required
    VaultAddress string `json:"vaultAddress"`

    // vaultNamespace specifies the Vault namespace where the transit secrets engine is mounted.
    // You must set this parameter when using Vault Enterprise with namespaces enabled.
    // When this field is not set, no namespace is used.
    //
    // +optional
    VaultNamespace string `json:"vaultNamespace,omitempty"`

    // tlsCA is a reference to a ConfigMap in the openshift-config namespace containing
    // the CA certificate bundle used to verify the TLS connection to the Vault server.
    // The ConfigMap must contain the CA bundle in the key "ca-bundle.crt".
    // When this field is not set, the system's trusted CA certificates are used.
    //
    // +optional
    TLSCA ConfigMapNameReference `json:"tlsCA,omitempty"`

    // tlsServerName specifies the Server Name Indication (SNI) to use when connecting to Vault via TLS.
    // This is useful when the Vault server's hostname doesn't match its TLS certificate.
    // When this field is not set, no SNI value is sent during the TLS connection.
    //
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:MinLength=1
    // +optional
    TLSServerName string `json:"tlsServerName,omitempty"`

    // tlsVerify controls whether the KMS plugin verifies the Vault server's TLS certificate.
    // Valid values are:
    // - "Verify": (default) TLS certificate verification is enabled. This is the secure option and should be used in production.
    // - "SkipVerify": TLS certificate verification is skipped. This option is insecure and should only be used in development or testing environments.
    // When this field is not set, it defaults to "Verify".
    //
    // +kubebuilder:validation:Enum=Verify;SkipVerify
    // +default="Verify"
    // +optional
    TLSVerify VaultTLSVerifyMode `json:"tlsVerify,omitempty"`

    // approleSecretRef references a secret in the openshift-config namespace containing
    // the AppRole credentials used to authenticate with Vault.
    // The secret must contain the following keys:
    //   - "roleID": The AppRole Role ID
    //   - "secretID": The AppRole Secret ID
    //
    // +required
    ApproleSecretRef corev1.LocalObjectReference `json:"approleSecretRef"`

    // transitMount specifies the mount path of the Vault Transit engine.
    // When this field is not set, it defaults to "transit".
    //
    // +kubebuilder:default="transit"
    // +optional
    TransitMount string `json:"transitMount,omitempty"`

    // transitKey specifies the name of the encryption key in Vault's Transit engine.
    // This key is used to encrypt and decrypt data.
    //
    // +required
    TransitKey string `json:"transitKey"`
}

// VaultTLSVerifyMode defines the TLS certificate verification mode for Vault connections
type VaultTLSVerifyMode string

const (
    // VaultTLSVerify enables TLS certificate verification (secure, recommended for production)
    VaultTLSVerify VaultTLSVerifyMode = "Verify"

    // VaultTLSSkipVerify disables TLS certificate verification (insecure, only for development/testing)
    VaultTLSSkipVerify VaultTLSVerifyMode = "SkipVerify"
)
```

#### EncryptionType Extension

The `EncryptionType` enum is extended to include the `ManagedKMS` value:

```diff
 type EncryptionType string

 const (
     EncryptionTypeIdentity EncryptionType = "identity"
     EncryptionTypeAESCBC   EncryptionType = "aescbc"
     EncryptionTypeAESGCM   EncryptionType = "aesgcm"
-    EncryptionTypeKMS      EncryptionType = "KMS"
+    EncryptionTypeKMS      EncryptionType = "KMS"        // Simple/unmanaged KMS (Tech Preview v1)
+    EncryptionTypeManagedKMS EncryptionType = "ManagedKMS" // Operator-managed KMS (Tech Preview v2+)
 )
```

The feature gate validation is updated to allow `ManagedKMS` when the `ManagedKMSProvider` gate is enabled:

```diff
 // APIServerEncryption validation
-// +openshift:validation:FeatureGateAwareEnum:featureGate="",enum="";identity;aescbc;aesgcm
-// +openshift:validation:FeatureGateAwareEnum:featureGate=KMSEncryption,enum="";identity;aescbc;aesgcm;KMS
+// +openshift:validation:FeatureGateAwareEnum:featureGate="",enum="";identity;aescbc;aesgcm
+// +openshift:validation:FeatureGateAwareEnum:featureGate=KMSEncryption,enum="";identity;aescbc;aesgcm;KMS
+// +openshift:validation:FeatureGateAwareEnum:featureGate=ManagedKMSProvider,enum="";identity;aescbc;aesgcm;ManagedKMS
```

Additionally, the `kms` field is required when `type: ManagedKMS`:

```diff
 // APIServerEncryption validation
+// +openshift:validation:FeatureGateAwareXValidation:featureGate=ManagedKMSProvider,rule="has(self.type) && self.type == 'ManagedKMS' ?  has(self.kms) : !has(self.kms)",message="kms config is required when encryption type is ManagedKMS, and forbidden otherwise"
```

#### KMSConfig Extension

The `KMSConfig` type is extended to include the Vault provider:

```diff
 type KMSConfig struct {
     // type defines the kind of platform for the KMS provider.
-    // Available provider types are AWS only.
+    // When the ManagedKMSProvider feature gate is enabled, Vault is also a supported provider type.
     //
     // +unionDiscriminator
     // +required
     Type KMSProviderType `json:"type"`

     // aws defines the key config for using an AWS KMS instance
     // for the encryption. The AWS KMS instance is managed
     // by the user outside the purview of the control plane.
     //
     // +unionMember
     // +optional
     AWS *AWSKMSConfig `json:"aws,omitempty"`
+
+    // vault defines the key config for using a HashiCorp Vault KMS instance
+    // for encryption. The Vault KMS instance is managed by the user outside
+    // the purview of the control plane.
+    //
+    // +openshift:enable:FeatureGate=ManagedKMSProvider
+    // +unionMember
+    // +optional
+    Vault *VaultKMSConfig `json:"vault,omitempty"`
 }
```

#### Example Configuration

**Prerequisites:**
- The `KMSEncryption` feature gate must be enabled (provides base KMS support)
- The `ManagedKMSProvider` feature gate must be enabled (enables Vault provider type)
- Both gates are available in `TechPreviewNoUpgrade` feature set

First, create the required resources in the `openshift-config` namespace.

Create a Secret containing the AppRole credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vault-approle
  namespace: openshift-config
type: Opaque
stringData:
  roleID: "kubernetes-kms"
  secretID: "your-secret-id-here"
```

Optional: create a ConfigMap containing the CA certificate bundle for verifying Vault's TLS certificate:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: vault-ca-bundle
  namespace: openshift-config
data:
  ca-bundle.crt: |
    -----BEGIN CERTIFICATE-----
    MIIDXTCCAkWgAwIBAgIJAKJ... (your CA certificate)
    -----END CERTIFICATE-----
```

Then, configure the APIServer to use Vault KMS:

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: ManagedKMS
    kms:
      type: Vault
      vault:
        vaultAddress: https://vault.example.com:8200
        approleSecretRef:
          name: vault-approle
        # Note: you must use digest reference
        vaultKMSPluginImage: registry.redhat.io/hashicorp/vault-kms-plugin@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef
        transitKey: kubernetes-encryption

        # optional
        vaultNamespace: admin/kubernetes

        # optional
        tlsCA:
          name: vault-ca-bundle
        tlsServerName: vault.internal.example.com

        # optional: Custom Transit mount (defaults to "transit")
        transitMount: transit
```

**This configuration is API-only in this enhancement**

Applying the above configuration will allow the API server to accept and
validate the Vault KMS configuration, but it will not result in functional
Vault KMS encryption. No operator will deploy or manage the Vault KMS plugin
based on this configuration until future enhancements implement that
functionality.

#### Vault KMS Plugin Image

The Vault KMS plugin is proprietary software developed by HashiCorp. Red Hat
cannot ship closed-source software, so users must provide the container image
reference for the plugin.

**Image Reference:**
Users must specify the full container image reference using a digest (not a tag). For example:
- `registry.redhat.io/hashicorp/vault-kms-plugin@sha256:abc123...`
- `docker.io/hashicorp/vault-kms-plugin@sha256:abc123...`
- `quay.io/hashicorp/vault-kms-plugin@sha256:abc123...`

**Why Digest References:**
- **Immutability**: Ensures the exact same image is always pulled
- **Security**: Prevents tag mutation attacks
- **Auditability**: Clear tracking of which plugin version is deployed

**Security Considerations:**

This API allows users to specify arbitrary container images that will run in
the control plane with access to encryption keys. A malicious or compromised
image could compromise the entire cluster. Users are responsible for ensuring
images come from trusted sources.

See the **Risks and Mitigations** section for detailed security analysis and
planned enhancements.

**Disconnected Environments:**
For disconnected or air-gapped environments, users must mirror the image to an
appropriate registry and reference the mirrored location:

```yaml
# Mirror the image to your registry, then reference it
vault:
  vaultKMSPluginImage: mirror.registry.example.com/hashicorp/vault-kms-plugin@sha256:abc123...
```

Alternatively, use `ImageDigestMirrorSet` to transparently redirect image pulls
to your mirror registry.

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

### Risks and Mitigations

**Risk: Malicious or compromised KMS plugin image**

Users can specify arbitrary container images via the `vaultKMSPluginImage`
field. A malicious or compromised image could:
- Access encryption keys and decrypt sensitive cluster data
- Exfiltrate secrets and credentials from the control plane
- Consume excessive resources (CPU, memory) to degrade control plane performance
- Exploit container runtime vulnerabilities to access the control plane node

*Impact:* **Critical** - Complete cluster compromise possible

*Mitigation (Current):*
- API validation enforces digest-only references (prevents tag mutation)

*Mitigation (Planned - Future Enhancement):*
- **Image signature verification**: Validate images are signed by a trusted
  authority before deployment
- Images run in the control plane with restricted RBAC (limits blast radius)
- Documentation emphasizes image source verification
- Users are responsible for validating image authenticity

This is a **known limitation** of the current API-only enhancement. Image
signature verification is planned for a future enhancement before the feature
progresses beyond Tech Preview.

---

**Risk: No runtime validation of plugin behavior**

The API accepts any image digest without validating the image actually contains a functional Vault KMS plugin.

*Impact:* **Medium** - Cluster encryption may fail silently or incorrectly

*Mitigation (Planned - Future Enhancement):*
- Runtime health checks and monitoring when operators deploy plugins
- Operator conditions reflecting plugin status
- Pre-flight checks for plugin image function

---

**Risk: AppRole credentials exposure**

AppRole credentials stored in Secrets could be accessed by users with elevated privileges.

*Impact:* **High** - Unauthorized access to Vault encryption keys

*Mitigation:*
- Secrets stored in `openshift-config` namespace (restricted access)
- Standard Secret encryption at rest applies
- Users must implement Vault policies limiting AppRole permissions
- Future: Consider more secure authentication methods

## Alternatives (Not Implemented)

### Hardcoded Registry with Digest-Only Image Field

An alternative approach was considered where users would provide only the
sha256 digest (not the full image reference) of the Vault KMS Plugin image, and
OpenShift would hardcode the registry path in the kube-apiserver operator code.

**Example configuration (not implemented)**
```yaml
vault:
  vaultKMSPluginImageDigest: sha256:abc123...
  # Operator would construct: registry.redhat.io/hashicorp/vault-kms-plugin@sha256:abc123...
```

**Advantages of this approach:**
- **Implicit trust**: Images pulled from a known Red Hat-managed registry
- **Clearer compatibility tracking**: Red Hat controls which images are available

**Why this approach was not implemented:**
- **Migration complexity**: Registry location changes require new OpenShift
  release
- **Registry dependency**: Requires HashiCorp to publish images to a specific
  Red Hat registry, or Red Hat to mirror Hashicorp image to its own registry
- **Partnership constraint**: Depends on formal agreement with HashiCorp for
  image publishing to Red Hat registries, or requires internal agreement
  between distinct parts of Red Hat

**Chosen approach:**
The implemented design uses a full image reference field
(`vaultKMSPluginImage`) that accepts any registry, requiring only that the
image be specified via digest. This approach:
- Works with any registry where HashiCorp publishes images
- Does not depend on specific Red Hat/HashiCorp publishing agreements
- Provides flexibility for users (test images, forks, alternative registries)

## Test Plan

**API Validation Tests:**
- Verify that `ManagedKMS` encryption type is rejected when `ManagedKMSProvider` feature gate is disabled
- Verify that `kms.vault` configuration is rejected when `ManagedKMSProvider` feature gate is disabled
- Verify that `kms` field is required when `type: ManagedKMS` and `ManagedKMSProvider` gate is enabled
- Verify field validation (character limits, regex patterns, required fields)

**Migration from Unmanaged KMS Tests:**

Existing CI/CD tests using the unmanaged `KMS` encryption type (from
kms-encryption-foundations) will need to be updated:

1. **Update test fixtures**: Change `type: KMS` to `type: ManagedKMS` and add appropriate `kms` configuration
2. **Add migration test**: Verify that changing from `type: KMS` to `type: ManagedKMS` triggers re-encryption
3. **Update QE test scenarios**: Ensure QE test environments use `ManagedKMS` with proper Vault configuration

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

Not applicable - this is a new feature.

## Upgrade / Downgrade Strategy

**Upgrade:**

This feature is gated by the `TechPreviewNoUpgrade` feature set. Upgrades are
not permitted in Tech Preview.

**Migration from Unmanaged to Managed KMS:**

Since both features are in `TechPreviewNoUpgrade`, there are no production
customer deployments using the unmanaged `KMS` encryption type. The migration
impact is limited to:

- **Internal CI/CD tests**: Automated tests using the unmanaged `KMS` type will
  need to be updated to use `ManagedKMS` with appropriate configuration
- **QE test environments**: Quality engineering test clusters will need to
  migrate their test scenarios
- **Development environments**: Developer clusters testing KMS functionality

For these internal test environments, the migration steps are:

1. Deploy the necessary configuration resources (Secrets, ConfigMaps) in
   `openshift-config`
2. Update the APIServer resource to change `type: KMS` to `type: ManagedKMS`
   and add the `kms` configuration
3. The encryption controllers will detect the mode change and trigger
   re-encryption
4. Wait for migration to complete

When the unmanaged `KMS` type is eventually removed and `ManagedKMS` is renamed
to `KMS`, there will be no customer impact since both features will
have graduated together and customers will only see the final `KMS` type with
managed deployment.

**Downgrade:**


## Version Skew Strategy

## Operational Aspects of API Extensions

### Failure Modes

### Monitoring

### Support Procedures

## Infrastructure Needed

### Development and Testing

### Production Validation

