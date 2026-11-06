---
title: add-ClusterImagePolicy-and-ImagePolicy-for-Signature-Verification
authors:
  - "@QiWang19"
  - "@aksjadha"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@saschagrunert, for API design, implementation details and graduation criteria"
  - "@mrunalp"
  - "@yuqi-zhang"
  - "@mtrmac"
  - "@wking"
  - "@ingvagabund"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@mrunalp"
  - "@JoelSpeed"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@mrunalp"
  - "@JoelSpeed"
creation-date: 2023-05-17
last-updated: 2026-10-13
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://redhat.atlassian.net/browse/OCPNODE-4756
---

# Add CRD ImagePolicy to config.openshift.io/v1

## Summary

This enhancement introduces ClusterImagePolicy and ImagePolicy CRDs to independently manage configurations at the cluster and namespace scopes. These CRDs support signature verification policies for trusted images, as well as deny-by-default policies to reject unsigned or unverified container images.

## Motivation

### User Stories

As an OpenShift user, I want to verify the container images signed using [sigstore](https://docs.sigstore.dev/about/overview/)
tools, so that I can utilize the increased security of my software supply chain.
As an OpenShift user, I want images that do not match an explicitly configured signature-verification policy to be rejected by default, so that only images satisfying an applicable verification policy are accepted.

### Goals

- ClusterImagePolicy is defined as cluster scoped CRD. ImagePolicy is defined as namespaced CRD.
- The user can create an ImagePolicy instance specifying the images/repositories to be verified and their policy. The MCO will write the configuration for signature verification. Once this is done, CRI-O can verify the images/repositories.
- MCO container runtime config controller watches ImagePolicy instance in different kubernetes namespaces and merges the instances for each namespace into a single [containers-policy.json](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md) in a predefined `/path/to/policies/<NAMESPACE>.json`.
- MCO container runtime config controller watches ClusterImagePolicy instance and merges the instances into a single [/etc/containers/policy.json](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md).
- The user can configure a ClusterImagePolicy to set the top-level default policy to reject images cluster-wide.
- The user can configure an ImagePolicy to set the top-level default policy to reject for that namespace.
- The user can revert the top-level default policy by deleting all configuration that requires default reject.
- CRI-O can verify Cosign signature signed images using configuration from ClusterImagePolicy and ImagePolicy by matching the namespace from the sandbox config on the `PullImage` RPC.

### Non-Goals

- Configuring policies for the images in the OCP payload is not within the scope of this enhancement.
- Providing a tool to mirror the signatures is out of the scope of this enhancement. In order to verify the signature, the disconnected users need to mirror signatures together with the application images.
- Grant the application administrator the ability to weaken cluster-scoped policies, to avoid expanding the set of administrators capable of increasing cluster exposure to vulnerable images.
- Grant the application administrator the ability to tighten cluster-scoped policies. This could be useful in the future, but we are deferring it to limit the amount of work needed for an initial implementation.
- Automatically enabling deny-by-default policies in existing or new clusters (administrators must explicitly create ClusterImagePolicy resources with `policyType: Reject` to enable this behavior).
- Creating default production ClusterImagePolicies with deny-by-default enabled (tracked separately in [OCPSTRAT-2761](https://redhat.atlassian.net/browse/OCPSTRAT-2761)).
- Providing emergency bypass mechanisms or Pod-level annotations to skip policy enforcement without updating ClusterImagePolicy or ImagePolicy resources.
- Supporting policy configuration at Pod or Deployment level through annotations or spec fields (policies apply only at cluster-wide or namespace-wide scope through CRDs).

## Proposal

### Workflow Description

**cluster administrator** Signature Verification Configuration Workflow:
1. The cluster administrator requests the addition of signature verification configurations at the cluster scope.
2. The cluster administrator writes the verification certification to the ClusterImagePolicy YAML file and creates a ClusterImagePolicy CR using `oc create -f imagepolicy.yaml`.
3. The cluster administrator can retrieve the merged cluster-scoped policies by checking `/etc/containers/policy.json` within the `99-<pool>-generated-registries` machine-config.
4. The cluster administrator has the option to delete the signature verification configuration by removing its ClusterImagePolicy instances.

**application administrator** Signature Verification Configuration Workflow:
1. The application administrator requests the addition of signature verification configurations at the namespace scope.
2. The application administrator writes the verification certification to the ImagePolicy YAML file and creates a ImagePolicy CR using `oc create -f imagepolicy.yaml`.
Please note that the application administrator cannot override cluster-scoped policies, as they are treated with higher priority. The [Implementation Details](#Update-container-runtime-config-controller-to-watch-ClusterImagePolicy-and-ImagePolicy) explains the conflict resolution rules.
3. The application administrator can retrieve the cluster override and merged policies by checking `<NAMESPACE>.json` within the `99-<pool>-generated-registries` machine-config.
4. The application administrator has the option to remove the signature verification configuration by deleting its ImagePolicy instances.

**Note:** The same workflows apply for deny-by-default policies: ClusterImagePolicy controls the cluster-wide default, while ImagePolicy controls the default for its namespace.

### API Extensions

#### Type definitions

Type definitions of ImagePolicy. ClusterImagePolicy is expected to have a similar structure to ImagePolicy.

```go
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImagePolicy holds namespace-wide configuration for image signature verification
//
// Compatibility level 1: Stable within a major release for a minimum of 12 months or 3 minor releases (whichever is longer).
// +openshift:compatibility-gen:level=1
type ImagePolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +kubebuilder:validation:Required
	Spec ImagePolicySpec `json:"spec"`
	// status contains the observed state of the resource.
	// +optional
	Status ImagePolicyStatus `json:"status,omitempty"`
}

// ImagePolicySpec is the specification of the ImagePolicy CRD.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="!has(self.appliesTo) ? (has(self.scopes) && size(self.scopes) > 0) : (self.appliesTo == 'Default' ? !has(self.scopes) : (has(self.scopes) && size(self.scopes) > 0))",message="scopes must be specified when appliesTo is omitted, and must be empty when appliesTo is Default"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="!has(self.appliesTo) || self.appliesTo != 'Default' || self.policy.rootOfTrust.policyType == 'Reject'",message="appliesTo Default requires policyType Reject"
type ImagePolicySpec struct {
	// +openshift:enable:FeatureGate=ImagePolicyDenyByDefault
	// appliesTo specifies whether this policy applies as a default for all images or to specific registry scopes.
	// Valid value is "Default".
	// When set to "Default", the policy acts as a fallback for images that do not match any registry-specific policies.
	// This maps to the policy.json "default" entry.
	// When omitted, the policy applies only to registries specified in the scopes field.
	// +optional
	// +kubebuilder:validation:Enum=Default
	AppliesTo AppliesTo `json:"appliesTo,omitempty"`
	// scopes defines the list of image identities assigned to a policy. Each item refers to a scope in a registry implementing the "Docker Registry HTTP API V2".
	// Scopes matching individual images are named Docker references in the fully expanded form, either using a tag or digest. For example, docker.io/library/busybox:latest (not busybox:latest).
	// More general scopes are prefixes of individual-image scopes, and specify a repository (by omitting the tag or digest), a repository
	// namespace, or a registry host (by only specifying the host name and possibly a port number) or a wildcard expression starting with *., for matching all subdomains (not including a port number).
	// Wildcards are only supported for subdomain matching, and may not be used in the middle of the host, i.e.  *.example.com is a valid case, but example*.*.com is not.
	// Please be aware that the scopes should not be nested under the repositories of OpenShift Container Platform (OCP) images.
	// If configured, the policies for OCP repositories will not be in effect.
	// For additional details about the format, please refer to the document explaining the docker transport field,
	// which can be found at: https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md#docker
	// +optional
	// +kubebuilder:validation:MaxItems=256
	// +listType=set
	Scopes []ImageScope `json:"scopes,omitempty"`
	// policy defines the verification policy for the items in the scopes list
	// +kubebuilder:validation:Required
	Policy Policy `json:"policy"`
}

// AppliesTo defines whether a policy applies to all images as a default fallback.
// When omitted, the policy applies to specific registries defined in the scopes field.
// +kubebuilder:validation:Enum=Default
type AppliesTo string

const (
	AppliesToDefault    AppliesTo = "Default"
)

// +kubebuilder:validation:XValidation:rule="size(self.split('/')[0].split('.')) == 1 ? self.split('/')[0].split('.')[0].split(':')[0] == 'localhost' : true",message="invalid image scope format, scope must contain a fully qualified domain name or 'localhost'"
// +kubebuilder:validation:XValidation:rule=`self.contains('*') ? self.matches('^\\*(?:\\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+$') : true`,message="invalid image scope with wildcard, a wildcard can only be at the start of the domain and is only supported for subdomain matching, not path matching"
// +kubebuilder:validation:XValidation:rule=`!self.contains('*') ? self.matches('^((((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:\\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+(?::[0-9]+)?)|(localhost(?::[0-9]+)?))(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?)(?::([\\w][\\w.-]{0,127}))?(?:@([A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}))?$') : true`,message="invalid repository namespace or image specification in the image scope"
// +kubebuilder:validation:MaxLength=512
type ImageScope string

// Policy defines the verification policy for the items in the scopes list.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="!(self.rootOfTrust.policyType in ['Reject', 'InsecureAcceptAnything']) || !has(self.signedIdentity)",message="signedIdentity must not be set when policyType is Reject or InsecureAcceptAnything"
type Policy struct {
	// rootOfTrust specifies the root of trust for the policy.
	// +kubebuilder:validation:Required
	RootOfTrust PolicyRootOfTrust `json:"rootOfTrust"`
	// signedIdentity specifies what image identity the signature claims about the image.
	// This field is only valid for signature verification policy types (PublicKey, FulcioCAWithRekor, PKI).
	// It must not be specified when policyType is Reject or InsecureAcceptAnything.
	// The required matchPolicy field specifies the approach used in the verification process to verify the identity in the signature and the actual image identity, the default matchPolicy is "MatchRepoDigestOrExact".
	// +optional
	SignedIdentity PolicyIdentity `json:"signedIdentity,omitempty"`
}

// PolicyRootOfTrust defines the root of trust based on the selected policyType.
// +union
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'PublicKey' ? has(self.publicKey) : !has(self.publicKey)",message="publicKey is required when policyType is PublicKey, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'FulcioCAWithRekor' ? has(self.fulcioCAWithRekor) : !has(self.fulcioCAWithRekor)",message="fulcioCAWithRekor is required when policyType is FulcioCAWithRekor, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'PKI' ? has(self.pki) : !has(self.pki)",message="pki is required when policyType is PKI, and forbidden otherwise"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="!(self.policyType in ['Reject', 'InsecureAcceptAnything']) || (!has(self.publicKey) && !has(self.fulcioCAWithRekor) && !has(self.pki))",message="publicKey, fulcioCAWithRekor, and pki must not be set when policyType is Reject or InsecureAcceptAnything"
type PolicyRootOfTrust struct {
	// policyType serves as the union's discriminator. Users are required to assign a value to this field, choosing one of the policy types.
	// "PublicKey" indicates that the policy relies on a sigstore publicKey and may optionally use a Rekor verification.
	// "FulcioCAWithRekor" indicates that the policy is based on the Fulcio certification and incorporates a Rekor verification.
	// "PKI" indicates that the policy relies on a PKI root of trust with optional intermediate certificates.
	// "Reject" indicates that all images matching the policy scopes will be rejected. This value is available only when the ImagePolicyDenyByDefault feature gate is enabled.
	// "InsecureAcceptAnything" indicates that all images matching the policy scopes will be accepted without verification. This value is available only when the ImagePolicyDenyByDefault feature gate is enabled.
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	PolicyType PolicyType `json:"policyType"`
	// publicKey defines the root of trust based on a sigstore public key.
	// +optional
	PublicKey *PublicKey `json:"publicKey,omitempty"`
	// fulcioCAWithRekor defines the root of trust based on the Fulcio certificate and the Rekor public key.
	// For more information about Fulcio and Rekor, please refer to the document at:
	// https://github.com/sigstore/fulcio and https://github.com/sigstore/rekor
	// +optional
	FulcioCAWithRekor *FulcioCAWithRekor `json:"fulcioCAWithRekor,omitempty"`

    // PKI defines the root of trust based on Root CA(s) and corresponding intermediate certificates.
    // +optional
    PKI *PKI `json:"pki,omitempty"`
}

// +kubebuilder:validation:Enum=PublicKey;FulcioCAWithRekor;PKI;Reject;InsecureAcceptAnything
// +openshift:validation:FeatureGateAwareEnum:featureGate=ImagePolicyDenyByDefault,enum=Reject;InsecureAcceptAnything
type PolicyType string

const (
	PublicKeyRootOfTrust         PolicyType = "PublicKey"
	FulcioCAWithRekorRootOfTrust PolicyType = "FulcioCAWithRekor"
	PKIRootOfTrust               PolicyType = "PKI"
	// +openshift:enable:FeatureGate=ImagePolicyDenyByDefault
	RejectPolicyType             PolicyType = "Reject"
	// +openshift:enable:FeatureGate=ImagePolicyDenyByDefault
)

// PublicKey defines the root of trust based on a sigstore public key.
type PublicKey struct {
	// keyData contains inline base64-encoded data for the PEM format public key.
	// KeyData must be at most 8192 characters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=8192
	KeyData []byte `json:"keyData"`
	// rekorKeyData contains inline base64-encoded data for the PEM format from the Rekor public key.
	// rekorKeyData must be at most 8192 characters.
	// +optional
	// +kubebuilder:validation:MaxLength=8192
	RekorKeyData []byte `json:"rekorKeyData,omitempty"`
}

// FulcioCAWithRekor defines the root of trust based on the Fulcio certificate and the Rekor public key.
type FulcioCAWithRekor struct {
	// fulcioCAData contains inline base64-encoded data for the PEM format fulcio CA.
	// fulcioCAData must be at most 8192 characters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=8192
	FulcioCAData []byte `json:"fulcioCAData"`
	// rekorKeyData contains inline base64-encoded data for the PEM format from the Rekor public key.
	// rekorKeyData must be at most 8192 characters.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=8192
	RekorKeyData []byte `json:"rekorKeyData"`
	// fulcioSubject specifies OIDC issuer and the email of the Fulcio authentication configuration.
	// +kubebuilder:validation:Required
	FulcioSubject PolicyFulcioSubject `json:"fulcioSubject,omitempty"`
}

// PKI defines the root of trust based on Root CA(s) and corresponding intermediate certificates.
type PKI struct {
	// CertificateAuthorityRootsData contains base64-encoded data of a certificate bundle PEM file, which contains one or more CA roots in the PEM format.
	// +required
	CertificateAuthorityRootsData []byte `json:"caRootsData,omitempty"`

	// CertificateAuthorityIntermediatesData base64-encoded data of a certificate bundle PEM file, which contains one or more intermediate certificates in the PEM format.
	// caIntermediatesData requires CertificateAuthorityRoots to be set.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == '' || self.CertificateAuthorityRootsData != ''", message="caIntermediatesData requires caRootsData to be set"
	CertificateAuthorityIntermediatesData []byte `json:"caIntermediatesData,omitempty"`

    // PKICertificateSubject defines the requirements imposed on the subject to which the certificate was issued.
	// +required
    // +kubebuilder:validation:XValidation:rule="self.CertificateAuthorityRootsData == '' || self.PKICertificateSubject != nil", message="pkiCertificateSubject must be set if caRootsData is specified"
	PKICertificateSubject *PKICertificateSubject `json:"pkiCertificateSubject,omitempty"`
}

// PKICertificateSubject defines the requirements imposed on the subject to which the certificate was issued.
type PKICertificateSubject struct {
	// Email address associated with the certificate identity.
	// +optional
  // +kubebuilder:validation:XValidation:rule="self.Email != '' || self.Hostname != ''", message="At least one of Email or Hostname must be set in pkiCertificateSubject"
	Email string `json:"email,omitempty"`

	// Hostname associated with the certificate identity.
	// +optional
	Hostname string `json:"hostname,omitempty"`
}

// PolicyFulcioSubject defines the OIDC issuer and the email of the Fulcio authentication configuration.
type PolicyFulcioSubject struct {
	// oidcIssuer contains the expected OIDC issuer. It will be verified that the Fulcio-issued certificate contains a (Fulcio-defined) certificate extension pointing at this OIDC issuer URL. When Fulcio issues certificates, it includes a value based on an URL inside the client-provided ID token.
	// Example: "https://expected.OIDC.issuer/"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="isURL(self)",message="oidcIssuer must be a valid URL"
	OIDCIssuer string `json:"oidcIssuer"`
	// signedEmail holds the email address the the Fulcio certificate is issued for.
	// Example: "expected-signing-user@example.com"
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule=`self.matches('^\\S+@\\S+$')`,message="invalid email address"
	SignedEmail string `json:"signedEmail"`
}

// PolicyIdentity defines image identity the signature claims about the image. When omitted, the default matchPolicy is "MatchRepoDigestOrExact".
// +kubebuilder:validation:XValidation:rule="(has(self.matchPolicy) && self.matchPolicy == 'ExactRepository') ? has(self.exactRepository) : !has(self.exactRepository)",message="exactRepository is required when matchPolicy is ExactRepository, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="(has(self.matchPolicy) && self.matchPolicy == 'RemapIdentity') ? has(self.remapIdentity) : !has(self.remapIdentity)",message="remapIdentity is required when matchPolicy is RemapIdentity, and forbidden otherwise"
// +union
type PolicyIdentity struct {
	// matchPolicy sets the type of matching to be used.
	// Valid values are "MatchRepoDigestOrExact", "MatchRepository", "ExactRepository", "RemapIdentity". When omitted, the default value is "MatchRepoDigestOrExact".
	// If set matchPolicy to ExactRepository, then the exactRepository must be specified.
	// If set matchPolicy to RemapIdentity, then the remapIdentity must be specified.
	// "MatchRepoDigestOrExact" means that the identity in the signature must be in the same repository as the image identity if the image identity is referenced by a digest. Otherwise, the identity in the signature must be the same as the image identity.
	// "MatchRepository" means that the identity in the signature must be in the same repository as the image identity.
	// "ExactRepository" means that the identity in the signature must be in the same repository as a specific identity specified by "repository".
	// "RemapIdentity" means that the signature must be in the same as the remapped image identity. Remapped image identity is obtained by replacing the "prefix" with the specified “signedPrefix” if the the image identity matches the specified remapPrefix.
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	MatchPolicy IdentityMatchPolicy `json:"matchPolicy"`
	// exactRepository is required if matchPolicy is set to "ExactRepository".
	// +optional
	PolicyMatchExactRepository *PolicyMatchExactRepository `json:"exactRepository,omitempty"`
	// remapIdentity is required if matchPolicy is set to "RemapIdentity".
	// +optional
	PolicyMatchRemapIdentity *PolicyMatchRemapIdentity `json:"remapIdentity,omitempty"`
}

// +kubebuilder:validation:MaxLength=512
// +kubebuilder:validation:XValidation:rule=`self.matches('.*:([\\w][\\w.-]{0,127})$')? self.matches('^(localhost:[0-9]+)$'): true`,message="invalid repository or prefix in the signedIdentity, should not include the tag or digest"
// +kubebuilder:validation:XValidation:rule=`self.matches('^(((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:\\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+(?::[0-9]+)?)|(localhost(?::[0-9]+)?))(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?$')`,message="invalid repository or prefix in the signedIdentity"
type IdentityRepositoryPrefix string

type PolicyMatchExactRepository struct {
	// repository is the reference of the image identity to be matched.
	// The value should be a repository name (by omitting the tag or digest) in a registry implementing the "Docker Registry HTTP API V2". For example, docker.io/library/busybox
	// +kubebuilder:validation:Required
	Repository IdentityRepositoryPrefix `json:"repository"`
}

type PolicyMatchRemapIdentity struct {
	// prefix is the prefix of the image identity to be matched.
	// If the image identity matches the specified prefix, that prefix is replaced by the specified “signedPrefix” (otherwise it is used as unchanged and no remapping takes place).
	// This useful when verifying signatures for a mirror of some other repository namespace that preserves the vendor’s repository structure.
	// The prefix and signedPrefix values can be either host[:port] values (matching exactly the same host[:port], string), repository namespaces,
	// or repositories (i.e. they must not contain tags/digests), and match as prefixes of the fully expanded form.
	// For example, docker.io/library/busybox (not busybox) to specify that single repository, or docker.io/library (not an empty string) to specify the parent namespace of docker.io/library/busybox.
	// +kubebuilder:validation:Required
	Prefix IdentityRepositoryPrefix `json:"prefix"`
	// signedPrefix is the prefix of the image identity to be matched in the signature. The format is the same as "prefix". The values can be either host[:port] values (matching exactly the same host[:port], string), repository namespaces,
	// or repositories (i.e. they must not contain tags/digests), and match as prefixes of the fully expanded form.
	// For example, docker.io/library/busybox (not busybox) to specify that single repository, or docker.io/library (not an empty string) to specify the parent namespace of docker.io/library/busybox.
	// +kubebuilder:validation:Required
	SignedPrefix IdentityRepositoryPrefix `json:"signedPrefix"`
}

// IdentityMatchPolicy defines the type of matching for "matchPolicy".
// +kubebuilder:validation:Enum=MatchRepoDigestOrExact;MatchRepository;ExactRepository;RemapIdentity
type IdentityMatchPolicy string

const (
	IdentityMatchPolicyMatchRepoDigestOrExact IdentityMatchPolicy = "MatchRepoDigestOrExact"
	IdentityMatchPolicyMatchRepository        IdentityMatchPolicy = "MatchRepository"
	IdentityMatchPolicyExactRepository        IdentityMatchPolicy = "ExactRepository"
	IdentityMatchPolicyRemapIdentity          IdentityMatchPolicy = "RemapIdentity"
)

// +k8s:deepcopy-gen=true
type ImagePolicyStatus struct {
	// conditions provide details on the status of this API Resource.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

### Feature Gate

The deny-by-default functionality is gated behind the `ImagePolicyDenyByDefault` feature gate, enabled in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade` feature sets.

#### Feature-Gated Enhancements

All new features are behind the `ImagePolicyDenyByDefault` feature gate:

- **appliesTo field** (`+openshift:enable:FeatureGate=ImagePolicyDenyByDefault`) - Controls whether policy is default or registry-specific
- **Reject enum value** (`+openshift:validation:FeatureGateAwareEnum:featureGate=ImagePolicyDenyByDefault`) - New policy types in the `PolicyType` enum
- **CEL validation rules** (`+openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault`) - Validation rules that reference feature-gated fields

#### Backward Compatibility

When the feature gate `ImagePolicyDenyByDefault` is not enabled, no user faced changes are expected on the clusters. When the feature gate `ImagePolicyDenyByDefault` is enabled, existing ClusterImagePolicy and ImagePolicy continue to work, and the new fields and enum values will be available for use in ClusterImagePolicy and ImagePolicy CRs.

### Implementation Details/Notes/Constraints [optional]

#### Update container runtime config controller to watch ClusterImagePolicy and ImagePolicy

Enhance the MCO container runtime config controller to manage ClusterImagePolicy and ImagePolicy CRs, and update the signature verification configurations:
- Retrieves all the ClusterImagePolicy and ImagePolicy instances on the cluster.
- Pre-validation:
  - Ensure that there are no scopes under the OCP image payload. If any are found, add an info-level log message to the machine config controller about the error encountered while adding an OCP image payload. Update the `status` of the CR to indicate that the policy will not be applied.
  - Check for conflicts between cluster scope and namespace scope policies. If the namespaced ImagePolicy scope is equal to or nests inside an existing cluster-scoped ClusterImagePolicy CR, do not deploy the namespaced policy.
  Update the `status` of both CRs and the machine config controller logs to indicate that the ClusterImagePolicy will be applied, while the non-global ImagePolicy will not be applied.
- Adds following configurations to machine configs
  - machine config `99-<pool>-generated-registries` adds [/etc/containers/registries.d/sigstore-registries.yaml](https://github.com/containers/image/blob/main/docs/containers-registries.d.5.md) to allow matching sigstore signatures for each image scope entry, for example:

    ```yaml
    docker:
      my-registry/image:
        use-sigstore-attachments: true
    ```

- Converts ClusterImagePolicy and ImagePolicy CRs to policy.json format:
  - **Scope mapping**: 
    - `appliesTo: Default` maps to the top-level `"default"` key in policy.json
    - `appliesTo` omitted with `scopes` field maps to `"transports": {"docker": {"<scope>": [...]}}` in policy.json
  - **PolicyType conversion**:
    - `policyType: PublicKey` → `{"type": "sigstoreSigned", "keyData": "...", ...}`
    - `policyType: FulcioCAWithRekor` → `{"type": "sigstoreSigned", "fulcio": {...}, ...}`
    - `policyType: PKI` → `{"type": "sigstoreSigned", ...}` with PKI configuration
    - `policyType: Reject` → `{"type": "reject"}`
    - `policyType: InsecureAcceptAnything` → `{"type": "insecureAcceptAnything"}`
- Merge rule for ClusterImagePolicy and ImagePolicy CRs:
  - when the syncHandler processing an image scope within a ClusterImagePolicy CR
    - the scope exists in an existing ClusterImagePolicy CR:
    append the policy to existing cluster policy; the policy will be written to `/etc/containers/policy.json` and each `\<NAMESPACE\>.json` if that namespace has an ImagePolicy CR that can be successfully rolled out.
    - if the scope is either equal to or a broader scope than one already present in an ImagePolicy CR: do not roll out the non-global policy in `<NAMESPACE>.json`. The policy for the cluster will be written to `/etc/containers/policy.json` and `<NAMESPACE>.json`;
    update the `status` of both CRs and machine config controller logs to indicate that the policy for cluster has been applied, while the non-global namespace policy is ignored.
    - if none of the above cases apply: the policy will be written to `/etc/containers/policy.json` and each `\<NAMESPACE\>.json` if that namespace has an ImagePolicy CR that can be successfully rolled out.
  - when the syncHandler processing default reject configuration within a ClusterImagePolicy CR:  
    - If at least one ClusterImagePolicy instance configures the default policy to `Reject`, writes `reject` to the top-level "default" entry in `/etc/containers/policy.json`.
    - Every generated namespace `\<NAMESPACE\>.json` will inherit the top level default from `/etc/containers/policy.json`
    - If an ImagePolicy with image scope using `PublicKey`, `FulcioCAWithRekor`, `PKI` exists, it will be rolled out according to the image scope merge rules above.    
  - when the syncHandler processing an image scope within an ImagePolicy CR
    - if the policy scope is equal to or nests inside an existing ClusterImagePolicy CR: do not roll out the non-global policy in `\<NAMESPACE\>.json`. the policy for the cluster will be written to `/etc/containers/policy.json` and `\<NAMESPACE\>.json`;
    update the `status` of both CRs and machine config controller logs to indicate that the policy for the cluster has been applied, while the non-global namespace policy is ignored.
    - the scope exists in another ImagePolicy CR:
    append the policy to existing policy; the policy will be written to `<NAMESPACE>.json`
    - if none of the above cases apply:
    the policy will be written to `/path/to/policies/\<NAMESPACE\>.json`
  - when the syncHandler processing default reject configuration within an ImagePolicy CR:
    - If a ClusterImagePolicy default Reject exists, keep Reject as top-level default in <NAMESPACE>.json
    - If no ClusterImagePolicy set default policy, write the ImagePolicy default `Reject` to the top-level "default" entry in  namespace file `\<NAMESPACE\>.json`. The namespace file retains inherited cluster image scope policies and any nonconflicting namespaced image scope policies.    
  - the policies will be coordinated with the base [/etc/containers/policy.json](https://github.com/openshift/machine-config-operator/blob/master/templates/master/01-master-container-runtime/_base/files/policy.yaml) file or the Image CR. The rollout will fail if the following validation with Image CR fails. In such a case, the error will be reported to the machine config logs.
    - if blockedRegistries exists, the clusterimagepolicy/imagepolicy image scopes must not equal to or nested under blockedRegistries
    - if allowedRegistries exists, the clusterimagepolicy/imagepolicy image scopes nested under the allowedRegistries. Signature verification will be enabled for these scopes.
    - if both blockedRegistries and ClusterImagePolicy with default Reject policy exists, the top-level default Reject policy will be written to `/etc/containers/policy.json` and `<NAMESPACE>.json` for all namespaces with ImagePolicy CRs.
    - if both allowedRegistries and ClusterImagePolicy with default Reject policy exists, the top-level default Reject policy will be written to `/etc/containers/policy.json` and `<NAMESPACE>.json` for all namespaces with ImagePolicy CRs.
    - if both blockedRegistries and ImagePolicy with default Reject policy exists, the effective top-level default policy of `/etc/containers/policy.json` is inherited from the above base file. The top-level default Reject policy will be written to `<NAMESPACE>.json` for the namespaces referenced by the ImagePolicy CR.
    - if both allowedRegistries and ImagePolicy with default Reject policy exists, the top-level default Reject policy will be written to `<NAMESPACE>.json` for all namespaces with ImagePolicy CRs.
    - When ClusterImagePolicy registry scope matches an Image CR allowed registry, the signature verification policy replaces the generated `insecureAcceptAnything` entry, enabling signature verification
  - the `/etc/containers/policy.json` holds the cluster wide policy. `\<NAMESPACE\>.json` holds the merged cluster override policy and namespaced policy.
- Image policies that are written to `/etc/containers/policy.json` will be rolled out by machine config `99-<pool>-generated-registries`.

#### Default Policy Precedence

- Cluster default `Reject` policy is written to `/etc/containers/policy.json` and inherited by every generated namespace policy.
- Multiple ImagePolicies with default `Reject` in the same namespace are allowed
- Multiple ClusterImagePolicies with default `Reject` are allowed and always compatible
- When a cluster default `Reject` exists, image-scoped signature verification policies from ImagePolicy instance are still applied, if they do not conflict with an equal or broader ClusterImagePolicy scope.


**Merge rules for same image scope in different CRs**

|                                                                                                                 	|process the policies from the CRs                |                                                                                    	|   	|   	|
|-----------------------------------------------------------------------------------------------------------------	|------------------------------------------------	|-----------------------------------------------------------------------------------	|---	|---	|
| same scope in different CRs                                                                                     	| ImagePolicy                                    	| ClusterImagePolicy                                                                	|   	|   	|
| ClusterImagePolicy ImagePolicy (scope in the ClusterImagePolicy is equal to or broader than in the ImagePolicy) 	| Do not deploy non-global policy for this scope 	| Write the cluster policy to `/etc/containers/policy.json`  and `<NAMESPACE>.json` 	|   	|   	|
| ClusterImagePolicy ClusterImagePolicy                                                                           	| N/A                                            	| Append the policy to existing `etc/containers/policy.json`                        	|   	|   	|
| ImagePolicy ImagePolicy                                                                                         	| append the policy to <NAMESPACE>.json          	| N/A                                                                               	|   	|   	|
**Merge rules for (Cluster)ImagePolicydefault Reject Configuration**

### Interaction with Image CR Registry Configuration

| Image CR configuration | ClusterImagePolicy default | ImagePolicy default | Effective `/etc/containers/policy.json` | Effective target `<NAMESPACE>.json` |
|---|---|---|---|---|
| Neither list | Not configured | Not configured | Default from the base file: `insecureAcceptAnything` | Inherits default `insecureAcceptAnything` |
| Neither list | Not configured | `Reject` | Default from the base file: `insecureAcceptAnything` | Default: `reject` |
| Neither list | `Reject` | Not configured | Default: `reject` | Inherits default `reject` |
| Neither list | `Reject` | `Reject` | Default: `reject` | Default: `reject` |
| `blockedRegistries` | Not configured | Not configured | Default from the base file: `insecureAcceptAnything` | Inherits default `insecureAcceptAnything` |
| `blockedRegistries` | Not configured | `Reject` | Default from the base file: `insecureAcceptAnything` | Default: `reject` |
| `blockedRegistries` | `Reject` | Not configured | Default: `reject` | Inherits default `reject` |
| `blockedRegistries` | `Reject` | `Reject` | Default: `reject` | Default: `reject` |
| `allowedRegistries` | Not configured | Not configured | Default: `reject`; allowed scopes: `insecureAcceptAnything` | Inherits default `reject` and the allowed-scope requirements |
| `allowedRegistries` | Not configured | `Reject` | Default: `reject`; allowed scopes: `insecureAcceptAnything` | Default: `reject`; inherits allowed-scope requirements |
| `allowedRegistries` | `Reject` | Not configured | Default: `reject`; allowed scopes: `insecureAcceptAnything` | Inherits default `reject` and the allowed-scope requirements |
| `allowedRegistries` | `Reject` | `Reject` | Default: `reject`; allowed scopes: `insecureAcceptAnything` | Default: `reject`; inherits allowed-scope requirements |

#### Example of ImagePolicy CRs
Example of ClusterImagePolicy and ImagePolicy.

```yaml
kind: ClusterImagePolicy
metadata:
  name: mypolicy-0
spec:
  scopes:
  - test0.com
  policy:
    rootoftrust:
      policyType: FulcioCAWithRekor
      fulcioCAWithRekor:
        fulciocadata: dGVzdC1jYS1kYXRhLWRhdGE=
        rekorkeydata: dGVzdC1yZWtvci1rZXktZGF0YQ==
    fulciosubject:
      oidcissuer: https://OIDC.example.com
      signedemail: test-user@example.com
    signedidentity:
      matchpolicy: RemapIdentity
      remapIdentity:
        prefix: test-remap-prefix
        signedPrefix: test-remap-signed-prefix
```

```yaml
kind: ClusterImagePolicy
metadata:
  name: mypolicy-1
spec:
  scopes:
  - test0.com	# this policy for test0.com and the policy from mypolicy-0 will be appended together
  - test1.com
  policy:
    rootoftrust:
      policyType: PublicKey
      publicKey:
        keydata: dGVzdC1rZXktZGF0YQ==
        rekorkeydata: dGVzdC1yZWtvci1rZXktZGF0YQ==
    signedidentity:
      matchpolicy: RemapIdentity
      remapIdentity:
        prefix: test-remap-prefix
        signedPrefix: test-remap-signed-prefix
```

```yaml
kind: ImagePolicy
metadata:
  name: mypolicy-2
  namespace: testnamespace
spec:
  scopes:
  - test0.com	# for test0.com, cluster policy will overwrite this policy
  - test2.com
  policy:
    rootoftrust:
      policyType: PublicKey
      publicKey:
        keydata: dGVzdC1rZXktZGF0YQ==
```
```yaml
kind: ImagePolicy
metadata:
  name: byopkipolicy # BYOPKI with Root CA
  namespace: testnamespace
spec:
  scopes:
  - test0.com	# for test0.com, cluster policy will overwrite this policy
  - test2.com
  policy:
    rootoftrust:
      policyType: PKI
      pki:
        caRootsData: dGVzdC1rZXktZGF0YQ==
        pkiCertificateSubject:
           email: test-user@test-domain.com
```
```yaml
kind: ImagePolicy
metadata:
  name: byopkipolicy   # BYOPKI with Root CA and Intermediate CA
  namespace: testnamespace
spec:
  scopes:
  - test0.com	# for test0.com, cluster policy will overwrite this policy
  - test2.com
  policy:
    rootoftrust:
      policyType: PKI
      pki:
        caRootsData: dGVzdC1rZXktZGF0YQ==
        caIntermediatesData: dGVzdC1rZXktZGF0YL==
        pkiCertificateSubject:
           email: test-user@test-domain.com
```

**Example: Deny-by-Default with Allowed Registries**

> **Note:** This example uses ClusterImagePolicy alone. If Image CR `allowedRegistries` is also configured, ClusterImagePolicy scopes must be nested inside the allowed registries. When both target the same scope, ClusterImagePolicy replaces Image CR's `insecureAcceptAnything`, enabling signature verification enforcement.

```yaml
# Allow Red Hat registries without verification - these policies are evaluated first by scope specificity
kind: ClusterImagePolicy
metadata:
  name: allow-redhat-registries
spec:
  scopes:
  - registry.redhat.io                                    # Images from Red Hat registry are allowed
  - quay.io/openshift-release-dev                         # OCP release images are allowed
  - image-registry.openshift-image-registry.svc:5000      # Internal registry images are allowed
  policy:
    rootOfTrust:
      policyType: InsecureAcceptAnything
---
# Reject all images that don't match more specific scopes above
kind: ClusterImagePolicy
metadata:
  name: default-reject
spec:
  appliesTo: Default  # Default policy - only evaluated when no registry scope matches
  policy:
    rootOfTrust:
      policyType: Reject  # Images from docker.io, ghcr.io, or any other registry will be rejected
```

**Example: Namespace-Scoped Deny-by-Default (Without Cluster-Wide Deny)**

> **Note:** This example assumes NO cluster-wide deny-by-default policy exists. If a ClusterImagePolicy with `appliesTo: Default` exists, **all** namespace-scoped ImagePolicies are ignored and marked Pending, regardless of their configuration. Similarly, if Image CR `allowedRegistries` is configured, namespace ImagePolicy scopes must nest inside the allowed registries.

```yaml
# Deny all images by default in the production namespace only
kind: ImagePolicy
metadata:
  name: default-reject
  namespace: production
spec:
  appliesTo: Default  # Default policy - reject all images in this namespace by default
  policy:
    rootOfTrust:
      policyType: Reject
---
# Allow only images from internal registry in production namespace
kind: ImagePolicy
metadata:
  name: allow-internal-registry
  namespace: production
spec:
  scopes:
  - image-registry.openshift-image-registry.svc:5000  # Internal registry allowed
  - quay.example.com/production                        # Company production registry
  policy:
    rootOfTrust:
      policyType: InsecureAcceptAnything
```

The deny-by-default ClusterImagePolicy example above will generate the following `/etc/containers/policy.json`:

```json
{
  "default": [
    {
      "type": "reject"
    }
  ],
  "transports": {
    "docker": {
      "registry.redhat.io": [
        {
          "type": "insecureAcceptAnything"
        }
      ],
      "quay.io/openshift-release-dev": [
        {
          "type": "insecureAcceptAnything"
        }
      ],
      "image-registry.openshift-image-registry.svc:5000": [
        {
          "type": "insecureAcceptAnything"
        }
      ]
    },
    "docker-daemon": {
      "": [
        {
          "type": "insecureAcceptAnything"
        }
      ]
    }
  }
}
```

For the `production` namespace with the namespace-scoped deny-by-default policy (assuming the `allow-redhat-registries` ClusterImagePolicy exists but no cluster-level `default-reject`), the following `/path/to/policies/production.json` will be generated:

```json
{
  "default": [
    {
      "type": "reject"
    }
  ],
  "transports": {
    "docker": {
      "registry.redhat.io": [
        {
          "type": "insecureAcceptAnything"
        }
      ],
      "quay.io/openshift-release-dev": [
        {
          "type": "insecureAcceptAnything"
        }
      ],
      "image-registry.openshift-image-registry.svc:5000": [
        {
          "type": "insecureAcceptAnything"
        }
      ],
      "quay.example.com/production": [
        {
          "type": "insecureAcceptAnything"
        }
      ]
    },
    "docker-daemon": {
      "": [
        {
          "type": "insecureAcceptAnything"
        }
      ]
    }
  }
}
```

**Explanation:**
- `"default": reject` - From the namespace-scoped `default-reject` ImagePolicy
- `registry.redhat.io`, `quay.io/openshift-release-dev` - From the cluster-scoped `allow-redhat-registries` ClusterImagePolicy
- `image-registry.openshift-image-registry.svc:5000` - From the cluster-scoped `allow-redhat-registries` ClusterImagePolicy. The namespace `allow-internal-registry` ImagePolicy also targets this scope, so it is marked `Pending` (cluster policy takes precedence per L435)
- `quay.example.com/production` - From the namespace-scoped `allow-internal-registry` ImagePolicy

Feedback from the container runtime config controller:

```yaml
- lastTransitionTime: "4321-03-07T11:21:39Z"
  message: Policy has scopes test0.com configured for both cluster scope non-global namespaces, only cluster scoped policy will be rolled out
  type: Pending
```

Apply the above CRs, if no Image CRs changes the policy.json. The below `/etc/containers/policy.json` will be rolled out. The condensed json string of the file will be updated to the `status.policyJSON` of `openshift-config` CR:

```json
{
  "default": [
    {
      "type": "insecureAcceptAnything"
    }
  ],
  "transports": {
    "docker": {
      "test0.com": [
        {
          "type": "sigstoreSigned",
          "fulcio": {
            "caData": "dGVzdC1jYS1kYXRhLWRhdGE=",
            "oidcIssuer": "https://OIDC.example.com",
            "subjectEmail": "test-user@example.com"
          },
          "rekorPublicKeyData": "dGVzdC1yZWtvci1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "remapIdentity",
            "prefix": "test-remap-prefix",
            "signedPrefix": "test-remap-signed-prefix"
          }
        },
        {
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "rekorPublicKeyData": "dGVzdC1yZWtvci1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "remapIdentity",
            "prefix": "test-remap-prefix",
            "signedPrefix": "test-remap-signed-prefix"
          }
        }
      ],
      "test1.com": [
        {
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "rekorPublicKeyData": "dGVzdC1yZWtvci1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "remapIdentity",
            "prefix": "test-remap-prefix",
            "signedPrefix": "test-remap-signed-prefix"
          }
        }
      ]
    },
    "docker-daemon": {
      "": [
        {
          "type": "insecureAcceptAnything"
        }
      ]
    }
  }
}
```

The merged cluster override policy and namespaced policy in the below `/path/to/policies/testnamespace.json`  will be rolled out.

```json
{
  "default": [
    {
      "type": "insecureAcceptAnything"
    }
  ],
  "transports": {
    "docker": {
      "test0.com": [
        {
          "type": "sigstoreSigned",
          "fulcio": {
            "caData": "dGVzdC1jYS1kYXRhLWRhdGE=",
            "oidcIssuer": "https://OIDC.example.com",
            "subjectEmail": "test-user@example.com"
          },
          "rekorPublicKeyData": "dGVzdC1yZWtvci1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "remapIdentity",
            "prefix": "test-remap-prefix",
            "signedPrefix": "test-remap-signed-prefix"
          }
        },
        {
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "rekorPublicKeyData": "dGVzdC1yZWtvci1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "remapIdentity",
            "prefix": "test-remap-prefix",
            "signedPrefix": "test-remap-signed-prefix"
          }
        }
      ],
      "test1.com": [
        {
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "rekorPublicKeyData": "dGVzdC1yZWtvci1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "remapIdentity",
            "prefix": "test-remap-prefix",
            "signedPrefix": "test-remap-signed-prefix"
          }
        }
      ],
      "test2.com": [
        {
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "matchRepoDigestOrExact"
          }
        }
      ]
    },
    "docker-daemon": {
      "": [
        {
          "type": "insecureAcceptAnything"
        }
      ]
    }
  }
}
```

### Risks and Mitigations

Risk: The ClusterImagePolicy and ImagePolicy policies overwrite

Mitigation: The [Implementation Details](#Update-container-runtime-config-controller-to-watch-ClusterImagePolicy-and-ImagePolicy) merge rule makes sure ImagePolicy for an image cannot override global definitions when merging ClusterImagePolicy and ImagePolicy.

Risk: Images already cached on a node are not re-checked against a newly created default policy when containers are created with `imagePullPolicy: IfNotPresent`.

Example: A node has `docker.io/library/busybox:1.36` cached. An administrator creates a ClusterImagePolicy with `appliesTo: Default` and `policyType: Reject`. A pod using `docker.io/library/busybox:1.36` with `imagePullPolicy: IfNotPresent` starts successfully on the node with the cached image but is rejected on nodes without the cached image.

Mitigation: When a ClusterImagePolicy with `appliesTo: Default` and `policyType: Reject` exists, the MCO writes a CRI-O drop-in configuration file `/etc/crio/crio.conf.d/<nn>-signature-policy.conf` containing `signature_policy = "/etc/containers/policy.json"`. This causes CRI-O to re-check cached images against the policy at container creation time, ensuring consistent policy enforcement regardless of whether the image is cached. This approach works because CRI-O's create-time policy check runs whenever the `signature_policy` option is set.

### Drawbacks

Creating a ClusterImagePolicy with `appliesTo: Default` disables **all** namespace-scoped ImagePolicies cluster-wide, even those that would tighten security. For example, if a cluster administrator creates a deny-by-default policy, namespaces cannot add their own signature verification requirements for specific registries. The only signal to namespace administrators is a `Pending` status condition on their ImagePolicy resources. This trade-off prioritizes preventing namespace administrators from weakening cluster-wide security policies over allowing them to strengthen their namespace's security posture.

### Topology Considerations

#### Hypershift / Hosted Control Planes

ClusterImagePolicy configuration for hosted clusters currently requires management cluster administrator privileges via `NodePool.spec.config`. Hosted cluster administrators cannot configure their own policies. Moving this configuration to `HostedCluster.spec.configuration` to enable hosted cluster administrators to manage policies is tracked in [CNTRLPLANE-531](https://issues.redhat.com/browse/CNTRLPLANE-531).

ImagePolicy (namespace-scoped) is not supported in HyperShift. The `openshift` ClusterImagePolicy is excluded on hosted clusters.

**HyperShift is unsupported for 5.1 Tech Preview.** Resolution of CNTRLPLANE-531 is a dependency for GA.

#### Single Node OpenShift (SNO)

Single Node OpenShift supports ClusterImagePolicy and ImagePolicy through the same MCO reconciliation path as multi-node clusters. Policy changes trigger a CRI-O reload, not a node reboot. The `openshift` ClusterImagePolicy is present.

A ClusterImagePolicy that configures `Reject` as the top-level default policy can block cluster upgrades if configured incorrectly. Since SNO has only one node, a blocked payload image leaves the node unable to upgrade with no fallback. Administrators must configure allow policies for OpenShift payload registries (`registry.redhat.io`, `quay.io/openshift-release-dev`) before enabling default Reject.

On SNO, nearly all OpenShift payload images are cached on the node after installation. A default Reject policy added post-install primarily affects new images not yet pulled (user workloads, external operator images, upgrade images), not the existing cached image set.

#### OpenShift Local (CRC)

OpenShift Local includes the ClusterImagePolicy and ImagePolicy CRDs and supports both policy types. However, it does not ship the `openshift` ClusterImagePolicy (the cluster is installed with `OPENSHIFT_INSTALL_EXPERIMENTAL_DISABLE_IMAGE_POLICY=true`, preventing the CVO from creating it). Users must manually create allow policies for OpenShift payload registries before enabling default Reject. The same upgrade risk constraints as SNO apply.

## Design Details

### Open Questions [optional]

### Test Plan

**Note:** *Section not required until targeted at a release.*

MCO container runtime config controller can add unit tests and e2e tests.
- unit test: 
  - verify the policies from ClusterImagePolicy and ImagePolicy instances merged correctly and the controller writes correct format policy.json
  - verify `policyType: Reject` and `policyType: InsecureAcceptAnything` generate correct policy.json output
  - verify `appliesTo: Default` field maps to `"default"` key in policy.json (not to transport scopes)
  - verify `appliesTo` omitted with `scopes` maps to transport-specific entries in policy.json
  - verify CEL validation rules:
    - `policyType: Reject` and `InsecureAcceptAnything` cannot have `publicKey/fulcioCAWithRekor/pki` or `signedIdentity`
    - when `appliesTo: Default`, `scopes` must be empty
    - when `appliesTo` is omitted, `scopes` must be specified
    - when `appliesTo: Default`, only `policyType: Reject` is allowed (PublicKey, FulcioCAWithRekor, PKI, and InsecureAcceptAnything are rejected)
- e2e test:
  - verify the MCO writes configuration to the correct location.
  - test demonstrating that an unsigned image in violation of the signature verification policy is rejected
  - test demonstrating that a Pod with image from docker.io fails when ClusterImagePolicy has `appliesTo: Default` with `policyType: Reject`
  - test demonstrating that a Pod with image from allowed registry succeeds when specific scope has `policyType: InsecureAcceptAnything` and default has `policyType: Reject`
  - test demonstrating that namespace ImagePolicy is ignored (marked Pending) when a ClusterImagePolicy with `appliesTo: Default` exists, even if the namespace policy would tighten security
  - test demonstrating that a namespace ImagePolicy with `appliesTo: Default` and `policyType: Reject` successfully rejects pods using images from the cluster's internal registry when no cluster-level default policy exists
  - test demonstrating that cached images are re-checked against a default Reject policy at container creation time: pull an image to a node, create a ClusterImagePolicy with `appliesTo: Default` and `policyType: Reject`, verify that creating a container with `imagePullPolicy: IfNotPresent` using the cached image is rejected (not allowed to start)
  - verify that the MCO writes the CRI-O drop-in configuration `/etc/crio/crio.conf.d/<nn>-signature-policy.conf` when a default Reject policy exists
  - test demonstrating that creating a ClusterImagePolicy with `appliesTo: Default` and `policyType: PublicKey` is rejected at admission with a CEL validation error
  - test demonstrating that when Image CR has `allowedRegistries` configured, a namespace ImagePolicy with `appliesTo: Default` and `policyType: InsecureAcceptAnything` is marked Pending and does not weaken the deny-by-default policy
  - test demonstrating that creating a ClusterImagePolicy with `appliesTo: Default` and `policyType: Reject` without allowing payload repositories causes the ClusterImagePolicy to be marked Pending with an error message indicating which repository would be blocked

**Note:** Tests using default Reject policies must include companion ClusterImagePolicies allowing payload repositories (extracted from the release image at runtime), `registry.redhat.io`, and the internal registry to prevent locking out the cluster.


### Graduation Criteria

#### OpenShift 5.1 (Current Phase)

The `ImagePolicyDenyByDefault` feature gate is available in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade` feature sets. This includes:
- The `appliesTo` field to specify default vs registry-specific policies
- The `Reject` and `InsecureAcceptAnything` policy types
- CEL validation rules that reference feature-gated fields

The base ClusterImagePolicy and ImagePolicy CRDs remain GA (config.openshift.io/v1) for signature verification use cases with the original three policy types (`PublicKey`, `FulcioCAWithRekor`, `PKI`).

#### Promotion to Default Feature Set

The `ImagePolicyDenyByDefault` feature gate can be promoted to the `Default` feature set when it meets the openshift/api promotion bar:
- At least five e2e tests tagged `[OCPFeatureGate:ImagePolicyDenyByDefault]`
- Each test runs 14+ times per platform (x86_64, aarch64, ppc64le, s390x)
- 95%+ pass rate across all platforms
- No known critical bugs or security issues

#### Before GA

The following items must be completed before the deny-by-default functionality can graduate to GA:

1. **Payload repository protection:** MCO validation that ensures ClusterImagePolicies with `appliesTo: Default` and `policyType: Reject` cannot block OpenShift payload repositories. When an `appliesTo: Default` with `policyType: Reject` policy exists, the MCO must check that the repositories of the release image, RHEL CoreOS, and RHEL CoreOS extensions (`ControllerConfig.Spec.ReleaseImage`, `BaseOSContainerImage`, `BaseOSExtensionsContainerImage`) are each covered by a non-`Reject` scope from some ClusterImagePolicy. If any payload repository is not covered, the MCO must not roll out the new `policy.json`, report the error in logs, and mark the ClusterImagePolicy as Pending with the uncovered repositories (e.g., `default Reject would block payload repo quay.io/openshift-release-dev/ocp-v4.0-art-dev; add a ClusterImagePolicy scope that covers it`). This validation follows the same pattern as the existing `blockedRegistries` check. Signed-release clusters pass automatically because the `openshift` ClusterImagePolicy covers these repositories. CI/QE clusters on unsigned releases and OKD clusters get a clear error instead of nodes that cannot pull payload images.

2. **Cached image re-checking:** MCO must write CRI-O configuration to ensure cached images are re-checked against default policies at container creation time.

3. **User documentation:** Complete user-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/) covering:
   - How to configure deny-by-default policies
   - Required allow-lists for OpenShift payload repositories
   - Behavior when cluster defaults disable namespace policies
   - Topology-specific limitations (HyperShift, OpenShift Local)

4. **Sufficient time for feedback:** Gather feedback from Tech Preview users on usability, performance impact, and edge cases.

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Upgrade expectations:
- In order to use ClusterImagePolicy and ImagePolicy, an existing cluster will have to make an upgrade to the version that the `ClusterImagePolicy` and `ImagePolicy` CRDs are available.
- If the existing cluster manages `/etc/containers/policy.json` via the MachineConfig, the configuration will continue to work if no new `ClusterImagePolicy` or `ImagePolicy` resources created during the upgrade. If `ImagePolicy` resources created and resulting `/path/to/policies/<NAMESPACE>.json` exists, CRI-O will use the config from `/path/to/policies/<NAMESPACE>.json`.
- **Deny-by-default support**:  In order to use deny-by-default functionality, an existing cluster will have to turn on the featuregate `ImagePolicyDenyByDefault`, existing ClusterImagePolicy and ImagePolicy CRs using `policyType: PublicKey`, `policyType: FulcioCAWithRekor`, or `policyType: PKI` with the `scopes` field continue to work without modification.

Downgrade expectations:
- Not applicable.
  If `N` does not support ClusterImagePolicy and ImagePolicy CRD, `N+1`supports ClusterImagePolicy and ImagePolicy. It is not expected that the CRDs related to the failure `N->N+1`. New resources should not be created during `N->N+1`.


### Version Skew Strategy

The implementation of `ClusterImagePolicy` and `ImagePolicy` is synchronized with CRI-O v1.28. To prevent version mismatches with CRI-O, the node team plans to target the release of CRI-O/MCO simultaneously. During an upgrade, the CRs will take effect only after both CRIO and MCO have been upgraded to versions that support the CRDs.

### Operational Aspects of API Extensions

#### Failure Modes

- CR field syntax error failure reported from CLI
- CR conflicting value failure: rollout failure according to the merge rules [Implementation Details](#Update-container-runtime-config-controller-to-watch-ClusterImagePolicy-and-ImagePolicy). The failure will be reported by machine config controller logs
- MCO rolling out configuration file failure reported by MCO
- If the signature validation fails, then CRI-O will report that in the same way as for any other pull failure. Further enhancements to the kubelet, CRI and CRI-O error handling can be achieved in future Kubernetes releases.

The above errors should not impact the overall cluster health since configuring policies for the images in the OCP payload is prohibited.
The OCP Node team is likely to be called upon in case of escalation with one of the failure modes.

#### Support Procedures

- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)
  - If the ClisterImagePolicy, ImagePolicy CR does not follow the syntax requirements, api-server will fail when creating the objects.
  - If the ClusterImagePolicy, ImagePolicy CR is not rolled out by MCO, machine-config-controller logs will shows error with the reason.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)
  - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.

- What consequences does it have on existing, running workloads?
  - No impact. CRI-O does not verify the signatures of container images while they are running.
  - Create/Update/Delete the ClusterImagePolicy and ImagePolicy resources do does not required node reboots. It follows the current [Reload Crio](https://github.com/openshift/machine-config-operator/blob/ff7ef2ec8ddbdf4f5758ee8f3ba3fea2d364e581/docs/MachineConfigDaemon.md?plain=1#L165) action.

- What consequences does it have for newly created workloads?
  - In order to verify signature for new pods, ClusterImagePolicy, ImagePolicy instances should be successfully rolled out before creating new workloads.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  - User can create another objects if previous objects failed to roll out.

## Implementation History

- [OCPNODE-1628: Sigstore Support - OpenShift Container Image Validation for cluster wide policies (Dev Preview)](https://issues.redhat.com/browse/OCPNODE-1628) epic will keep track of the ClusterImagePolicy implementation.
- [OCPNODE-2253: OpenShift Container Image Validation for namespaced policies](https://issues.redhat.com/browse/OCPNODE-2253) epic will keep track of the ImagePolicy implementation.
- [OCPNODE-4756: (Cluster)ImagePolicy for reject-by-default](https://redhat.atlassian.net/browse/OCPNODE-4756) epic tracks the deny-by-default policy extension by adding `reject` and `insecureAcceptAnything` to the existing `policyType` enum.

## Alternatives (Not Implemented)

Not applicable.

## Infrastructure Needed [optional]

- Registry proxies like registry.k8s.io are not natively usable: https://github.com/containers/image/issues/1952
Workaround: using remapIdentity
