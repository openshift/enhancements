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

* Provide API configuration for Vault-specific parameters (authentication, TLS, Transit engine configuration)

### Non-Goals

* Deploying and managing the vault kms plugin workloads (will be covered by a future enhancement)
* Support Vault Community Edition (only Vault Enterprise is supported)
* Support for Vault authentication methods other than AppRole
* Automatic provisioning or configuration of Vault infrastructure
* Management of Vault Transit keys lifecycle (key creation, rotation policy configuration)

## Proposal

### API Extensions

We will extend the existing `KMSConfig` type in the APIServer configuration API to support Vault as a new KMS provider type. The implementation follows the same pattern established for AWS KMS provider.

#### Feature Gate

A new feature gate `KMSv2` will be introduced and enabled in `TechPreviewNoUpgrade`. This gate controls the availability of the Vault provider type in the `KMSProviderType` enum.

```go
FeatureGateKMSv2 = newFeatureGate("KMSv2").
    reportProblemsToJiraComponent("kube-apiserver").
    contactPerson("fmissi").
    productScope(ocpSpecific).
    enhancementPR("https://github.com/openshift/enhancements/pull/XXXX").
    enable(inTechPreviewNoUpgrade()).
    mustRegister()
```

Additionally, the existing `KMSEncryptionProvider` feature gate is extended to `TechPreviewNoUpgrade` (previously only `DevPreviewNoUpgrade`) to make the `kms` field available in Tech Preview.

#### Vault KMSConfig

The following config is extracted from [Vault KMS Plugin documentation](https://github.com/hashicorp/web-unified-docs/blob/ab6191e4856b52a59a87fe0f17703671a7317ec6/content/vault/v1.21.x/content/docs/deploy/kubernetes/kms/configuration.mdx).
At the time of writing, the plugin is in pre-alpha and the exposed configuration might be subject to changes.

```go
// VaultKMSConfig defines the KMS config specific to HashiCorp Vault KMS provider
type VaultKMSConfig struct {
	// vaultAddress specifies the address of the HashiCorp Vault instance.
	// The value must be a valid URL with scheme (http:// or https://) and can be between 1 and 256 characters.
	// Example: https://vault.example.com:8200
	//
	// +kubebuilder:validation:Pattern=`^https?://`
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:MinLength=1
	// +required
    VaultAddress string `json:"vaultAddress"`

    // vaultNamespace specifies the Vault namespace where the Transit secrets engine is mounted.
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

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

### Risks and Mitigations

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

