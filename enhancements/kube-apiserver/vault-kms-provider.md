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
last-updated: 2026-03-04
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

Extend OpenShift's KMS encryption provider support to include HashiCorp Vault KMS provider, enabling users to leverage Vault's Transit secrets engine for encryption key management. This enhancement introduces the `KMSv2` feature gate and extends the APIServer configuration API to support Vault-specific configuration parameters.

## Motivation

### User Stories

### Goals

* Provide API configuration for Vault KMS

### Non-Goals

* Deploying and managing the vault kms plugin workloads (will be covered by a future enhancement)
* Verification of user-provided KMS Plugin Images (will be covered by a future enhancement)

## Proposal

### API Extensions

We will extend the existing `KMSConfig` type in the APIServer configuration
resource to support Vault as a new KMS provider type. The implementation
follows the same pattern established for the AWS KMS configuration.

#### Feature Gate

A new feature gate `KMSv2` will be introduced and enabled in
`TechPreviewNoUpgrade`. This gate controls the availability of the Vault
provider type in the `KMSProviderType` enum.

```go
FeatureGateKMSv2 = newFeatureGate("KMSv2").
    reportProblemsToJiraComponent("kube-apiserver").
    contactPerson("fmissi").
    productScope(ocpSpecific).
    enhancementPR("https://github.com/openshift/enhancements/pull/1954").
    enable(inTechPreviewNoUpgrade()).
    mustRegister()
```

Additionally, the existing `KMSEncryptionProvider` feature gate is extended to
`TechPreviewNoUpgrade` (previously only `DevPreviewNoUpgrade`) to make the
`kms` field available in Tech Preview.

#### Vault KMSConfig

The following config is extracted from [Vault KMS Plugin documentation](https://github.com/hashicorp/web-unified-docs/blob/ab6191e4856b52a59a87fe0f17703671a7317ec6/content/vault/v1.21.x/content/docs/deploy/kubernetes/kms/configuration.mdx).
At the time of writing, the plugin is in pre-alpha and the configuration
parameters might be subject to change.

```go
// VaultKMSConfig defines the KMS plugin configuration specific to Vault KMS
type VaultKMSConfig struct {
    // vaultKMSPluginImage specifies the container image for the HashiCorp Vault KMS plugin.
    // The image must be specified using a digest reference.
    //
    // For disconnected environments, mirror the image to your internal registry.
    //
    // Refer to the OpenShift documentation for a list of tested and compatible image digests
    // for your cluster version.
    //
    // +kubebuilder:validation:Pattern=`^([a-zA-Z0-9\-\.]+)(:[0-9]+)?/[a-zA-Z0-9\-\./]+@sha256:[a-f0-9]{64}$`
    // +required
    VaultKMSPluginImage string `json:"vaultKMSPluginImage"`

	// vaultAddress specifies the address of the Vault instance.
	// The value must be a valid URL with scheme (http:// or https://) and can be between 1 and 256 characters.
	// Example: https://vault.example.com:8200
	//
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:MinLength=1
	// +required
    VaultAddress string `json:"vaultAddress"`

    // vaultNamespace specifies the Vault namespace where the transit secrets engine is mounted.
    // You must set this parameter when using Vault Enterprise with namespaces enabled.
    // When this field is not set, no namespace is used.
    //
    // +kubebuilder:validation:MaxLength=128
    // +kubebuilder:validation:MinLength=1
    // +optional
    VaultNamespace string `json:"vaultNamespace,omitempty"`

    // tlsCAFile specifies the filesystem path to a PEM-encoded CA certificate bundle
    // used to verify the TLS connection to the Vault server.
    // When this field is not set, the system's trusted CA certificates are used.
    // Example: /etc/vault/ca.crt
    //
    // +kubebuilder:validation:MaxLength=512
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:XValidation:rule="self.matches('^/.*$')",message="tlsCAFile must be an absolute filesystem path starting with /"
    // +optional
    TLSCAFile string `json:"tlsCAFile,omitempty"`

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
    // +kubebuilder:validation:MaxLength=128
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:default="transit"
    // +optional
    TransitMount string `json:"transitMount,omitempty"`

    // transitKey specifies the name of the encryption key in Vault's Transit engine.
    // This key is used to encrypt and decrypt data.
    //
    // +kubebuilder:validation:MaxLength=128
    // +kubebuilder:validation:MinLength=1
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

#### KMSConfig Extension

The `KMSConfig` type is extended to include the Vault provider:

```diff
 type KMSConfig struct {
     // type defines the kind of platform for the KMS provider.
-    // Available provider types are AWS only.
+    // When the KMSv2 feature gate is enabled, Vault is also a supported provider type.
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
+    // +openshift:enable:FeatureGate=KMSv2
+    // +unionMember
+    // +optional
+    Vault *VaultKMSConfig `json:"vault,omitempty"`
 }
```

#### Example Configuration

First, create a secret in the `openshift-config` namespace containing the AppRole credentials:

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
        # Required: Vault server address
        vaultAddress: https://vault.example.com:8200

        # Required: AppRole authentication secret reference
        approleSecretRef:
          name: vault-approle

        # Required: Vault KMS plugin image (must use digest reference)
        # See OpenShift documentation for tested and compatible images for your cluster version
        vaultKMSPluginImage: registry.redhat.io/hashicorp/vault-kms-plugin@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef

        # Required: Transit engine key name
        transitKey: kubernetes-encryption

        # Optional: Vault Enterprise namespace
        vaultNamespace: admin/kubernetes

        # Optional: TLS configuration
        tlsCAFile: /etc/vault/ca.crt
        tlsServerName: vault.internal.example.com

        # Optional: Custom Transit mount (defaults to "transit")
        transitMount: transit
```

#### Vault KMS Plugin Image

The Vault KMS plugin is proprietary software developed by HashiCorp. Red Hat cannot ship closed-source software, so users must provide the container image reference for the plugin.

**Image Reference:**
Users must specify the full container image reference using a digest (not a tag). For example:
- `registry.redhat.io/hashicorp/vault-kms-plugin@sha256:abc123...`
- `docker.io/hashicorp/vault-kms-plugin@sha256:abc123...`
- `quay.io/hashicorp/vault-kms-plugin@sha256:abc123...`

**Why Digest References:**
- **Immutability**: Ensures the exact same image is always pulled
- **Security**: Prevents tag mutation attacks
- **Compatibility**: Clear mapping between image digests and tested plugin versions

**Tested Images:**
The OpenShift documentation maintains a compatibility matrix listing tested and compatible image digests for each OpenShift version. Users should refer to this documentation when selecting an image.

**Disconnected Environments:**
For disconnected or air-gapped environments, mirror the image to your internal registry and reference the mirrored location:

```yaml
# Mirror the image to your registry, then reference it
vault:
  vaultKMSPluginImage: mirror.registry.example.com/hashicorp/vault-kms-plugin@sha256:abc123...
```

Alternatively, use `ImageDigestMirrorSet` to transparently redirect image pulls to your mirror registry.

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

### Risks and Mitigations

## Alternatives (Not Implemented)

### Hardcoded Registry with Digest-Only Image Field

An alternative approach was considered where users would provide only the SHA256 digest (not the full image reference), and OpenShift would hardcode the registry path in the kube-apiserver operator code.

**Example configuration (not implemented)**
```yaml
vault:
  vaultKMSPluginImageDigest: sha256:abc123...
  # Operator would construct: registry.redhat.io/hashicorp/vault-kms-plugin@sha256:abc123...
```

**Advantages of this approach:**
- **Implicit trust**: Images pulled from a known Red Hat-managed registry
- **Clearer compatibility tracking**: Red Hat controls which images are available
- **Simpler API**: Shorter field, less for users to configure

**Why this approach was not implemented:**
- **Migration complexity**: Registry location changes require new OpenShift release
- **Registry dependency**: Requires HashiCorp to publish images to a specific Red Hat registry, or Red Hat to mirror Hashicorp image to its own registry
- **Partnership constraint**: Depends on formal agreement with HashiCorp for image publishing to Red Hat registries, or requires internal agreement between distinct parts of Red Hat

**Chosen approach:**
The implemented design uses a full image reference field (`vaultKMSPluginImage`) that accepts any registry, requiring only that the image be specified via digest. This approach:
- Works with any registry where HashiCorp publishes images
- Does not depend on specific Red Hat/HashiCorp publishing agreements
- Provides flexibility for users (test images, forks, alternative registries)

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

Not applicable - this is a new feature.

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

### Failure Modes

### Monitoring

### Support Procedures

## Infrastructure Needed

### Development and Testing

### Production Validation

