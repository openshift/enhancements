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
a KMS provider. This enhancement introduces the `VaultKMSPlugin` feature
gate and adds Vault-specific configuration fields to the KMSConfig API,
allowing the Vault KMS plugin to be configured declaratively.

When the `VaultKMSPlugin` feature gate is enabled, the existing `KMS`
encryption type is extended to support a `kms` configuration field with
provider-specific settings. This evolves the KMS encryption type from the basic
implementation (where plugins are manually deployed) to a fully managed
implementation where operators deploy and manage the Vault KMS plugin based on
API configuration.

## Motivation

### User Stories

- As a cluster admin, I want to configure Vault as a KMS provider in OCP so
  that I can encrypt resources using Vault Enterprise
- As a cluster admin using a multi-tenant Vault Enterprise, I need to specify
  which Vault namespace to use in OCP encryption config
- As a cluster admin with a private CA, I need to provide a custom CA bundle
  for Vault TLS connections

### Goals

* Introduce the `VaultKMSPlugin` feature gate to enable declarative
  configuration for operator-managed KMS plugin
* Extend the `KMSConfig` API type to support Vault as a provider type
* Define the structure for KMS plugin lifecycle management (details will be
  added in future iterations)
* Remove AWS api from KMSConfig (it's not functional and we have no current
  plan to support it)
* Remove `KMSEncryptionProvider` feature gate (it gates the AWS fields in the
  KMSConfig)

### Non-Goals

The following are explicitly out of scope for this enhancement:

* **Image signature verification and security validation** of user-provided KMS
  plugin images (critical security enhancement planned for future work)

## Proposal

This enhancement proposes adding API support for configuring OCP to encrypt
resources using HashiCorp Vault Enterprise via the Vault KMS plugin. The API
changes will be gated by the new `VaultKMSPlugin` feature gate.

### Workflow Description

This enhancement is API-only and does not introduce any runtime workflows. The
workflow consists solely of:

1. **Cluster administrator** creates necessary resources in the `openshift-config` namespace:
   - Secret containing Vault AppRole credentials (roleID and secretID)
   - Optional ConfigMap containing custom CA certificate bundle for Vault TLS verification

2. **Cluster administrator** configures the APIServer resource with Vault KMS settings:
   - Sets `spec.encryption.type: KMS`
   - Provides `spec.encryption.kms.type: Vault` with full Vault configuration

3. **API server** validates the configuration against OpenAPI schema and CEL validation rules

4. **Configuration is stored** but has no runtime effect (no operators act on it yet)

Future enhancements will add operator workflows to deploy and manage Vault KMS
plugins based on this configuration.

### API Extensions

We propose extending the existing `KMSConfig` type in the APIServer
configuration resource to support Vault as a new KMS provider type. The
implementation follows the same pattern established for the AWS KMS
configuration.

#### Feature Gates

This enhancement introduces a new feature gate `VaultKMSPlugin` to control
access to Vault KMS plugin configuration in the APIServer API.
Additionally, this enhancement proposes removing the `KMSEncryptionProvider`
feature gate, which is superseded by `KMSEncryption`, as well as the
`AWSConfig` in `KMSConfig`, which is managed by the `KMSEncryptionProvider` gate.

**VaultKMSPlugin:**

```go
FeatureGateVaultKMSPlugin = newFeatureGate("VaultKMSPlugin").
    reportProblemsToJiraComponent("kube-apiserver").
    contactPerson("fmissi").
    productScope(ocpSpecific).
    enhancementPR("https://github.com/openshift/enhancements/pull/1954").
    enable(inTechPreviewNoUpgrade()).
    mustRegister()
```

When enabled, this gate allows:
- The `kms` field in the `APIServerEncryption` struct
- The `Vault` value in the `KMSProviderType` enum
- The `vault` field in the `KMSConfig` struct

**Relationship to KMSEncryption:**

The `KMSEncryption` feature gate (from kms-encryption-foundations) enables the
`encryption.type: KMS` value and provides foundational KMS mode implementation
in library-go encryption controllers.

The `VaultKMSPlugin` feature gate extends the KMS encryption type by adding:
- The `kms` field in `APIServerEncryption` for plugin-specific configuration
- Support for Vault KMS plugin
- Operator-managed Vault KMS plugin lifecycle (deployment, health monitoring, updates)

When `VaultKMSPlugin` is enabled, users can specify `encryption.type: KMS`
with the `kms` field to configure Vault KMS. The previous basic KMS
implementation (without the `kms` field) remains available for backward
compatibility during the Tech Preview phase but will be deprecated before GA.

**Removing KMSEncryptionProvider:**

The `KMSEncryptionProvider` feature gate was introduced in [PR #1682](https://github.com/openshift/enhancements/pull/1682)
but was superseded by the `KMSEncryption` gate in [PR #1900](https://github.com/openshift/enhancements/pull/1900)
(kms-encryption-foundations). The old gate is non-functional and should be removed.
This enhancement proposes:

- Removing the `KMSEncryptionProvider` feature gate definition from
  `features/features.go`
- Updating API validation annotations to use only `KMSEncryption` (not
  `KMSEncryptionProvider`)
- Removing the `AWSConfig` type from APIServer config

Since `KMSEncryptionProvider` is in `TechPreviewNoUpgrade` and has never been
functional, this removal has no customer impact.

#### Vault KMSConfig

The following config is extracted from [Vault KMS Plugin documentation](https://github.com/hashicorp/web-unified-docs/blob/ab6191e4856b52a59a87fe0f17703671a7317ec6/content/vault/v1.21.x/content/docs/deploy/kubernetes/kms/configuration.mdx).
At the time of writing, the plugin is in pre-alpha and the configuration
parameters might be subject to change.

**Field Validation Limits and Error Messages:**

The API enforces the following validation limits on VaultKMSConfig fields, with
improved error messages using XValidation to provide actionable feedback:

- **kmsPluginImage** (75-512 chars): Minimum of 75 characters accounts for
  the shortest possible digest reference (`r/i@sha256:` + 64 hex characters).
  Maximum of 512 characters follows common Kubernetes practice for container
  image references and accommodates long registry hostnames and deep repository paths.
  Uses XValidation to provide a clear error message when users incorrectly use
  image tags instead of digests.
- **vaultAddress** (1-512 chars): Follows the established pattern in OpenShift
  API for service URLs, matching the OIDC `issuerURL` field limit (see
  [types_authentication.go:266](https://github.com/openshift/api/blob/master/config/v1/types_authentication.go#L266)).
  Sufficient for typical Vault server addresses with scheme, hostname, port,
  and optional short path. Uses XValidation to provide a clear error message
  when the URL scheme is missing.
- **vaultNamespace** (1-4096 chars): While Vault Enterprise has [no hard limit](https://developer.hashicorp.com/vault/docs/internals/limits)
  on namespace paths, we enforce a maximum of 4096 characters (matching Unix PATH_MAX)
  to prevent configuration errors and ensure compatibility with OpenShift's storage backend.
  This limit accommodates any reasonable namespace hierarchy while providing defense in depth.
- **tls.caBundle**: Optional reference to a ConfigMap containing CA certificates for
  TLS verification. Follows the standard OpenShift pattern for CA bundle references.
- **tls.serverName** (1-253 chars): [RFC 1035](https://www.rfc-editor.org/rfc/rfc1035)
  defines DNS FQDN maximum as 253 characters (255 octets minus 2 for encoding).
  This is the correct standard limit for hostnames.
- **transitMount** (1-1024 chars, default "transit"): While Vault has [no explicit limit](https://developer.hashicorp.com/vault/docs/internals/limits)
  on mount paths, we enforce a maximum of 1024 characters as a reasonable upper bound
  for mount path configurations. This prevents configuration errors and ensures
  compatibility with OpenShift's storage backend while supporting any legitimate mount path.
- **transitKey** (1-512 chars): While Vault Transit key names have [no documented hard limit](https://developer.hashicorp.com/vault/docs/internals/limits),
  we enforce a maximum of 512 characters as a reasonable upper bound for key identifiers.
  This prevents configuration errors and ensures compatibility with OpenShift's storage
  backend while supporting any legitimate key naming convention.

```go
// VaultKMSConfig defines the KMS plugin configuration specific to Vault KMS
type VaultKMSConfig struct {
    // kmsPluginImage specifies the container image for the HashiCorp Vault KMS plugin.
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
    // +kubebuilder:validation:XValidation:rule="self.matches('^([a-zA-Z0-9\\-\\.]+)(:[0-9]+)?/[a-zA-Z0-9\\-\\./]+@sha256:[a-f0-9]{64}$')",message="kmsPluginImage must be a valid image reference with a SHA256 digest (e.g., 'registry.example.com/vault-plugin@sha256:0123...abcd'). Use '@sha256:<64-character-hex-digest>' instead of image tags like ':latest' or ':v1.0.0'."
    // +kubebuilder:validation:MinLength=75
    // +kubebuilder:validation:MaxLength=512
    // +required
    KMSPluginImage string `json:"kmsPluginImage,omitempty"`

	// vaultAddress specifies the address of the HashiCorp Vault instance.
	// The value must be a valid URL with scheme (http:// or https://) and can be up to 512 characters.
	// Example: https://vault.example.com:8200
	//
	// +kubebuilder:validation:XValidation:rule="self.matches('^https?://')",message="vaultAddress must be a valid URL starting with 'http://' or 'https://' (e.g., 'https://vault.example.com:8200')."
	// +kubebuilder:validation:MaxLength=512
	// +kubebuilder:validation:MinLength=1
	// +required
    VaultAddress string `json:"vaultAddress,omitempty"`

    // vaultNamespace specifies the Vault namespace where the Transit secrets engine is mounted.
    // This is only applicable for Vault Enterprise installations.
    // The value can be between 1 and 4096 characters.
    // When this field is not set, no namespace is used.
    //
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=4096
    // +optional
    VaultNamespace string `json:"vaultNamespace,omitempty"`

    // tls contains the TLS configuration for connecting to the Vault server.
    // When this field is not set, system default TLS settings are used.
    // +optional
    TLS *VaultTLSConfig `json:"tls,omitempty"`

    // approleSecretRef references a secret in the openshift-config namespace containing
    // the AppRole credentials used to authenticate with Vault.
    // The secret must contain the following keys:
    //   - "roleID": The AppRole Role ID
    //   - "secretID": The AppRole Secret ID
    //
    // The namespace for the secret referenced by approleSecretRef is openshift-config.
    //
    // +required
    ApproleSecretRef SecretNameReference `json:"approleSecretRef,omitempty"`

    // transitMount specifies the mount path of the Vault Transit engine.
    // The value can be between 1 and 1024 characters.
    // When this field is not set, it defaults to "transit".
    //
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=1024
    // +kubebuilder:default="transit"
    // +optional
    TransitMount string `json:"transitMount,omitempty"`

    // transitKey specifies the name of the encryption key in Vault's Transit engine.
    // This key is used to encrypt and decrypt data.
    // The value must be between 1 and 512 characters.
    //
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=512
    // +required
    TransitKey string `json:"transitKey,omitempty"`
}

// VaultTLSConfig contains TLS configuration for connecting to Vault.
type VaultTLSConfig struct {
    // caBundle references a ConfigMap in the openshift-config namespace containing
    // the CA certificate bundle used to verify the TLS connection to the Vault server.
    // The ConfigMap must contain the CA bundle in the key "ca-bundle.crt".
    // When this field is not set, the system's trusted CA certificates are used.
    //
    // The namespace for the ConfigMap is openshift-config.
    //
    // Example ConfigMap:
    //   apiVersion: v1
    //   kind: ConfigMap
    //   metadata:
    //     name: vault-ca-bundle
    //     namespace: openshift-config
    //   data:
    //     ca-bundle.crt: |
    //       -----BEGIN CERTIFICATE-----
    //       ...
    //       -----END CERTIFICATE-----
    //
    // +optional
    CABundle ConfigMapNameReference `json:"caBundle,omitempty"`

    // serverName specifies the Server Name Indication (SNI) to use when connecting to Vault via TLS.
    // This is useful when the Vault server's hostname doesn't match its TLS certificate.
    // When this field is not set, the hostname from vaultAddress is used for SNI.
    //
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:MinLength=1
    // +optional
    ServerName string `json:"serverName,omitempty"`
}
```

#### APIServerEncryption Extension

When the `VaultKMSPlugin` feature gate is enabled, the `kms` field becomes
available in the `APIServerEncryption` type for provider-specific configuration.

The `kms` field is required when `type: KMS` and `VaultKMSPlugin` is enabled:

```diff
 // APIServerEncryption type
+// +openshift:validation:FeatureGateAwareXValidation:featureGate=VaultKMSPlugin,rule="has(self.type) && self.type == 'KMS' ?  has(self.kms) : !has(self.kms)",message="when encryption type is KMS, the kms field must be set with Vault KMS plugin configuration. Ensure the VaultKMSPlugin feature gate is enabled."
 type APIServerEncryption struct {
     Type EncryptionType `json:"type,omitempty"`
+
+    // kms defines the configuration needed to run the KMS provider plugin
+    // +openshift:enable:FeatureGate=VaultKMSPlugin
+    // +unionMember
+    // +optional
+    KMS *KMSConfig `json:"kms,omitempty"`
 }
```

#### KMSConfig Extension

The `KMSConfig` type is extended to include the Vault provider:

```diff
+// +openshift:validation:FeatureGateAwareXValidation:featureGate=VaultKMSPlugin,rule="has(self.type) && self.type == 'Vault' ? has(self.vault) : !has(self.vault)",message="vault config is required when kms provider type is Vault, and forbidden otherwise"
 type KMSConfig struct {
     // type defines the kind of platform for the KMS provider.
-    // Available provider types are AWS only.
+    // Valid values are:
+    // - "AWS": Amazon Web Services KMS (always available)
+    // - "Vault": HashiCorp Vault KMS (available when VaultKMSPlugin feature gate is enabled)
     //
     // +unionDiscriminator
     // +required
     Type KMSProviderType `json:"type"`

     // aws defines the key config for using an AWS KMS instance
     // for the encryption. The AWS KMS instance is managed
     // by the user outside the purview of the control plane.
+    // This field must be set when type is AWS, and must be unset otherwise.
     //
     // +unionMember
     // +optional
     AWS *AWSKMSConfig `json:"aws,omitempty"`
+
+    // vault defines the configuration for the Vault KMS plugin.
+    // The plugin connects to a Vault Enterprise server that is managed
+    // by the user outside the purview of the control plane.
+    // This field must be set when type is Vault, and must be unset otherwise.
+    //
+    // +openshift:enable:FeatureGate=VaultKMSPlugin
+    // +unionMember
+    // +optional
+    Vault *VaultKMSConfig `json:"vault,omitempty"`
 }
```

The `KMSProviderType` enum is also extended:

```diff
+// +openshift:validation:FeatureGateAwareEnum:featureGate="",enum=AWS
+// +openshift:validation:FeatureGateAwareEnum:featureGate=VaultKMSPlugin,enum=AWS;Vault
 type KMSProviderType string

 const (
     // AWSKMSProvider represents a supported KMS provider for use with AWS KMS
     AWSKMSProvider KMSProviderType = "AWS"
+
+    // VaultKMSProvider represents a supported KMS provider for use with HashiCorp Vault
+    VaultKMSProvider KMSProviderType = "Vault"
 )
```

#### Example Configuration

**Prerequisites:**
- The `VaultKMSPlugin` feature gate must be enabled (available in `TechPreviewNoUpgrade` feature set)

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
    type: KMS
    kms:
      type: Vault
      vault:
        vaultAddress: https://vault.example.com:8200
        approleSecretRef:
          name: vault-approle
        # Note: you must use digest reference
        kmsPluginImage: registry.redhat.io/hashicorp/vault-kms-plugin@sha256:a1b2c3d4e5f67890abcdef1234567890fedcba0987654321abcdef1234567890
        transitKey: kubernetes-encryption

        # optional
        vaultNamespace: admin/kubernetes

        # optional: TLS configuration
        tls:
          caBundle:
            name: vault-ca-bundle
          serverName: vault.internal.example.com

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
  kmsPluginImage: mirror.registry.example.com/hashicorp/vault-kms-plugin@sha256:abc123...
```

Alternatively, use `ImageDigestMirrorSet` to transparently redirect image pulls
to your mirror registry.

### KMS Plugin Lifecycle Management

This section outlines the operational aspects of deploying and managing Vault
KMS plugins based on the API configuration. Implementation details will be
determined in future iterations.

#### Plugin Deployment

How the kube-apiserver operator deploys Vault KMS plugin pods based on the API
configuration.

*Implementation details to be determined in future iterations.*

#### Configuration Management

How the operator translates the APIServer Vault KMS configuration into plugin
configuration and manages Secret/ConfigMap propagation.

*Implementation details to be determined in future iterations.*

#### Health Monitoring

Health checking, status reporting, and observability for KMS plugins.

*Implementation details to be determined in future iterations.*

#### Plugin Updates

Handling image digest changes and plugin version updates.

*Implementation details to be determined in future iterations.*

#### Failure Handling

Plugin pod failures, Vault connectivity issues, and fallback behavior.

*Implementation details to be determined in future iterations.*

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift currently has its own KMS implementation and API.

Future work may unify Hypershift to use the KMS configuration introduced here,
at which point this Vault KMS API would become available to Hypershift. That
unification is out of scope for this enhancement.

#### Standalone Clusters

This enhancement applies to standalone clusters.

#### Single-node Deployments or MicroShift

The Vault KMS plugin configuration can be set on SNO and MicroShift deployments.

We will go into more details when we update this enhancement with Vault KMS
plugin lifecycle management.

#### OpenShift Kubernetes Engine

This enhancement applies to OpenShift Kubernetes Engine (OKE). The API changes
are available in OKE as it shares the same API types.

We will go into more details when we update this enhancement with Vault KMS
plugin lifecycle management.

### Implementation Details/Notes/Constraints


### Risks and Mitigations

**Risk: Malicious or compromised KMS plugin image**

Users can specify arbitrary container images via the `kmsPluginImage`
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

### Drawbacks

**Increased API complexity:**
Adding Vault-specific configuration to the core `config.openshift.io/v1` API
increases the surface area of the cluster configuration API. The VaultKMSConfig
struct adds 7 top-level fields (with an additional nested VaultTLSConfig type
containing 2 fields), increasing the cognitive load for users and the
maintenance burden for the API.

*Overcome by:* The Vault configuration follows the same union type pattern used
throughout OpenShift for provider-specific configurations (e.g., OAuth identity
providers support GitHub, GitLab, Google, LDAP with provider-specific fields).
Feature gates ensure the Vault fields are only visible when explicitly enabled.

**Vendor-specific API in core config:**
Embedding HashiCorp Vault-specific fields (like `approleSecretRef`,
`transitMount`, `transitKey`) directly in the OpenShift core API creates a tight
coupling to a specific third-party product.

*Overcome by:* OpenShift already integrates third-party services via
provider-specific API fields (OAuth providers, cloud platforms, DNS providers).
The union type design with `KMSProviderType` discriminator keeps provider
configurations isolated. Alternative KMS providers can be added in the future
without affecting Vault configurations.

**Security risk from user-provided images:**
Allowing users to specify arbitrary container images for the KMS plugin creates
a potential attack vector. A malicious image could compromise encryption keys or
exfiltrate secrets from the control plane.

*Overcome by:* Digest-only image references prevent tag mutation attacks.
Feature gate keeps this in Tech Preview until image signature verification is
implemented. Documentation emphasizes security implications and the requirement
to trust image sources.

**Limited authentication flexibility:**
Supporting only AppRole authentication for Vault limits integration options for
users who have standardized on other Vault auth methods (Kubernetes, JWT, TLS
certificates).

*Overcome by:* AppRole is Vault's recommended authentication method for
applications and provides good security/usability balance. The API design allows
for additional auth methods to be added in future API versions as optional
fields alongside AppRole if there is demand.

## Alternatives (Not Implemented)

### Hardcoded Registry with Digest-Only Image Field

An alternative approach was considered where users would provide only the
sha256 digest (not the full image reference) of the Vault KMS Plugin image, and
OpenShift would hardcode the registry path in the kube-apiserver operator code.

**Example configuration (not implemented)**
```yaml
vault:
  kmsPluginImageDigest: sha256:abc123...
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
(`kmsPluginImage`) that accepts any registry, requiring only that the
image be specified via digest. This approach:
- Works with any registry where HashiCorp publishes images
- Does not depend on specific Red Hat/HashiCorp publishing agreements
- Provides flexibility for users (test images, forks, alternative registries)

## Test Plan

**API Validation Tests:**
- Verify that the `kms` field is rejected when `VaultKMSPlugin` feature gate is disabled
- Verify that `kms.vault` configuration is rejected when `VaultKMSPlugin` feature gate is disabled
- Verify that `kms` field is required when `type: KMS` and `VaultKMSPlugin` gate is enabled
- Verify field validation (character limits, regex patterns, required fields) for Vault KMS configuration

**Integration with Existing KMS Tests:**

Existing CI/CD tests using the basic `KMS` encryption type (from
kms-encryption-foundations) will need to be updated to use the `kms` field:

1. **Update test fixtures**: Add `kms` configuration with Vault provider settings
2. **Update QE test scenarios**: Ensure QE test environments properly configure the `kms` field

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

Not applicable - this is a new feature.

## Upgrade / Downgrade Strategy

**Upgrade:**
N/A

**Downgrade:**
N/A

**Evolution of KMS Encryption Type:**

This enhancement extends the existing `KMS` encryption type (enabled by the
`KMSEncryption` feature gate) by adding provider-specific configuration when the
`VaultKMSPlugin` feature gate is enabled.

Since both features are in `TechPreviewNoUpgrade`, there are no production
customer deployments. The impact is limited to internal test environments:

- **Internal CI/CD tests**: Tests using basic KMS will continue to work
- **QE test environments**: Can adopt the new `kms` field configuration
- **Development environments**: Can test Vault KMS with full configuration

For internal environments migrating to use the `kms` field:

1. Deploy the necessary configuration resources (Secrets, ConfigMaps) in
   `openshift-config`
2. Update the APIServer resource to add the `kms` configuration (keeping `type: KMS`)
3. The encryption controllers will configure the KMS plugin based on the API settings


## Version Skew Strategy

N/A - This enhancement only adds API fields. There are no version skew concerns
between components since no runtime implementation exists.

## Operational Aspects of API Extensions

### Failure Modes

N/A - This enhancement only adds API validation. The only "failure" is invalid
configuration being rejected by API validation, which is the expected behavior.

### Monitoring

N/A - No runtime components to monitor. API validation metrics are already
covered by standard API server metrics.

## Support Procedures

**Invalid configuration:**
If a user reports that their Vault KMS configuration is rejected:
1. Check API validation errors in the APIServer resource status or `oc` output
2. Verify all required fields are present and within validation limits
3. Verify the `VaultKMSPlugin` feature gate is enabled

**Configuration accepted but encryption not working:**
This is expected - the API-only enhancement accepts configuration but does not
implement plugin deployment. Future work will cover the necessary changes for
KMS encryption using Vault KMS Plugin.

## Infrastructure Needed

### Development and Testing

No special infrastructure needed. API validation can be tested with standard
unit tests and OpenAPI schema validation.

### Production Validation

N/A - This enhancement is API-only with no functional implementation to validate
in production environments.

