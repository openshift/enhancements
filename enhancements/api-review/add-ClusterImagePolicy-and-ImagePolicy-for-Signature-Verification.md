---
title: add-ClusterImagePolicy-and-ImagePolicy-for-Signature-Verification
authors:
  - "@QiWang19"
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
last-updated: 2024-05-02
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-1628
---

# Add CRD ImagePolicy to config.openshift.io/v1alpha1

## Summary

This enhancement introduces ClusterImagePolicy and ImagePolicy CRDs to independently manage configurations at the cluster and namespace scopes. These CRDs support signature verification policies for trusted images, as well as deny-by-default policies to reject unsigned or unverified container images.

## Motivation

### User Stories

As an OpenShift user, I want to verify the container images signed using [sigstore](https://docs.sigstore.dev/about/overview/)
tools, so that I can utilize the increased security of my software supply chain.

### Goals

- ClusterImagePolicy is defined as cluster scoped CRD. ImagePolicy is defined as namespaced CRD.
- The user can create an ImagePolicy instance specifying the images/repositories to be verified and their policy. The MCO will write the configuration for signature verification. Once this is done, CRI-O can verify the images/repositories.
- MCO container runtime config controller watches ImagePolicy instance in different kubernetes namespaces and merges the instances for each namespace into a single [containers-policy.json](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md) in a predefined `/path/to/policies/<NAMESPACE>.json`.
- MCO container runtime config controller watches ClusterImagePolicy instance and merges the instances into a single [/etc/containers/policy.json](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md).
- CRI-O can verify Cosign signature signed images using configuration from ClusterImagePolicy and ImagePolicy by matching the namespace from the sandbox config on the `PullImage` RPC.
- Support deny-by-default policies where administrators can reject unsigned or unverified images by default, while explicitly allowing trusted registries.
- Support default scope (`"default"`) to specify fallback policy for images not matching any other scope, evaluated only when no more specific scope matches.

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

**Note:** The same workflows apply for deny-by-default policies by setting `policyType: Reject` with the special scope `"default"` (representing default policy for all images), or `policyType: InsecureAcceptAnything` to explicitly allow images without verification.

### API Extensions

#### Type definitions

Type definitions of ImagePolicy. ClusterImagePolicy is expected to have a similar structure to ImagePolicy.

```go
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ImagePolicy holds namespace-wide configuration for image signature verification
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
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
// +kubebuilder:validation:XValidation:rule="!self.scopes.exists(s, s == 'default') || size(self.scopes) == 1",message="default scope 'default' cannot be combined with other scopes in the same policy"
type ImagePolicySpec struct {
	// scopes defines the list of image identities assigned to a policy. Each item refers to a scope in a registry implementing the "Docker Registry HTTP API V2".
	// Scopes matching individual images are named Docker references in the fully expanded form, either using a tag or digest. For example, docker.io/library/busybox:latest (not busybox:latest).
	// More general scopes are prefixes of individual-image scopes, and specify a repository (by omitting the tag or digest), a repository
	// namespace, or a registry host (by only specifying the host name and possibly a port number) or a wildcard expression starting with *., for matching all subdomains (not including a port number).
	// Wildcards are only supported for subdomain matching, and may not be used in the middle of the host, i.e.  *.example.com is a valid case, but example*.*.com is not.
	// A special scope value "default" represents the default policy for images that do not match any other scopes. This default scope is only evaluated when no more specific scope matches the image.
	// The "default" scope cannot be combined with other scopes in the same policy.
	// Please be aware that the scopes should not be nested under the repositories of OpenShift Container Platform (OCP) images.
	// If configured, the policies for OCP repositories will not be in effect.
	// For additional details about the format, please refer to the document explaining the docker transport field,
	// which can be found at: https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md#docker
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxItems=256
	// +listType=set
	Scopes []ImageScope `json:"scopes"`
	// policy defines the verification policy for the items in the scopes list
	// +kubebuilder:validation:Required
	Policy Policy `json:"policy"`
}

// +kubebuilder:validation:XValidation:rule="self == 'default' || (size(self.split('/')[0].split('.')) == 1 ? self.split('/')[0].split('.')[0].split(':')[0] == 'localhost' : true)",message="invalid image scope format, scope must be 'default', contain a fully qualified domain name, or 'localhost'"
// +kubebuilder:validation:XValidation:rule=`self == 'default' || (self.contains('*') ? self.matches('^\\*(?:\\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+$') : true)`,message="invalid image scope with wildcard, a wildcard can only be at the start of the domain and is only supported for subdomain matching, not path matching"
// +kubebuilder:validation:XValidation:rule=`self == 'default' || (!self.contains('*') ? self.matches('^((((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:\\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+(?::[0-9]+)?)|(localhost(?::[0-9]+)?))(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?)(?::([\\w][\\w.-]{0,127}))?(?:@([A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*[:][[:xdigit:]]{32,}))?$') : true)`,message="invalid repository namespace or image specification in the image scope"
// +kubebuilder:validation:MaxLength=512
type ImageScope string

// Policy defines the verification policy for the items in the scopes list.
type Policy struct {
	// rootOfTrust specifies the root of trust for the policy.
	// +kubebuilder:validation:Required
	RootOfTrust PolicyRootOfTrust `json:"rootOfTrust"`
	// signedIdentity specifies what image identity the signature claims about the image. The required matchPolicy field specifies the approach used in the verification process to verify the identity in the signature and the actual image identity, the default matchPolicy is "MatchRepoDigestOrExact".
	// +optional
	SignedIdentity PolicyIdentity `json:"signedIdentity,omitempty"`
}

// PolicyRootOfTrust defines the root of trust based on the selected policyType.
// +union
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'PublicKey' ? has(self.publicKey) : !has(self.publicKey)",message="publicKey is required when policyType is PublicKey, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'FulcioCAWithRekor' ? has(self.fulcioCAWithRekor) : !has(self.fulcioCAWithRekor)",message="fulcioCAWithRekor is required when policyType is FulcioCAWithRekor, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'PKI' ? has(self.pki) : !has(self.pki)",message="pki is required when policyType is PKI, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="(self.policyType != 'Reject' && self.policyType != 'InsecureAcceptAnything') || (!has(self.publicKey) && !has(self.fulcioCAWithRekor) && !has(self.pki))",message="publicKey, fulcioCAWithRekor, and pki must not be set when policyType is Reject or InsecureAcceptAnything"
type PolicyRootOfTrust struct {
	// policyType serves as the union's discriminator. Users are required to assign a value to this field, choosing one of the policy types.
	// "PublicKey" indicates that the policy relies on a sigstore publicKey and may optionally use a Rekor verification.
	// "FulcioCAWithRekor" indicates that the policy is based on the Fulcio certification and incorporates a Rekor verification.
	// "PKI" indicates that the policy relies on a PKI root of trust with optional intermediate certificates.
	// "Reject" indicates that all images matching the policy scopes will be rejected.
	// "InsecureAcceptAnything" indicates that all images matching the policy scopes will be accepted without verification.
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
type PolicyType string

const (
	PublicKeyRootOfTrust         PolicyType = "PublicKey"
	FulcioCAWithRekorRootOfTrust PolicyType = "FulcioCAWithRekor"
	PKIRootOfTrust               PolicyType = "PKI"
	RejectPolicyType             PolicyType = "Reject"
	InsecureAcceptAnythingType   PolicyType = "InsecureAcceptAnything"
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

### Implementation Details/Notes/Constraints [optional]

#### Update container runtime config controller to watch ClusterImagePolicy and ImagePolicy

Enhance the MCO container runtime config controller to manage ClusterImagePolicy and ImagePolicy CRs, and update the signature verification configurations:
- Retrieves all the ClusterImagePolicy and ImagePolicy instances on the cluster.
- Pre-validation:
  - Ensure that there are no scopes under the OCP image payload. If any are found, add an info-level log message to the machine config controller about the error encountered while adding an OCP image payload. Update the `status` of the CR to indicate that the policy will not be applied.
  - Check for conflicts between cluster scope and namespace scope policies. If the namespaced ImagePolicy scope is equal to or nests inside an existing cluster-scoped ClusterImagePolicy CR, do not deploy the namespaced policy.
  Update the `status` of both CRs and the machine config controller logs to indicate that the ClusterImagePolicy will be applied, while the non-global ImagePolicy will not be applied.
- Adds following configurations to machine configs
  - machine config `99-<pool>-generated-registries` adds [/etc/containers/registries.d/*.yaml](https://github.com/containers/image/blob/main/docs/containers-registries.d.5.md) to allow matching sigstore signatures, for example:

    ```yaml
    docker:
      my-registry/image:
        use-sigstore-attachments: true
    ```

    Alternatively, we can enable it node wide:

    ```yaml
    default-docker:
      use-sigstore-attachments: true
    ```
- Converts ClusterImagePolicy and ImagePolicy CRs to policy.json format:
  - **Scope mapping**: 
    - Special scope `"default"` maps to the `"default"` key in policy.json
    - Regular scopes (e.g., `registry.redhat.io`, `docker.io`) map to `"transports": {"docker": {"<scope>": [...]}}` in policy.json
  - **PolicyType conversion**:
    - `policyType: PublicKey` → `{"type": "sigstoreSigned", "keyData": "...", ...}`
    - `policyType: FulcioCAWithRekor` → `{"type": "sigstoreSigned", "fulcio": {...}, ...}`
    - `policyType: PKI` → `{"type": "sigstoreSigned", ...}` with PKI configuration
    - `policyType: Reject` → `{"type": "reject"}`
    - `policyType: InsecureAcceptAnything` → `{"type": "insecureAcceptAnything"}`
- Special handling for default scope `"default"`:
  - When a ClusterImagePolicy uses scope `"default"`, it is treated as the broadest possible scope covering ALL registries and images
  - Any ImagePolicy scope (e.g., `quay.io`, `docker.io`, or any specific registry) is considered to nest inside the cluster's `"default"` scope
  - Therefore, ImagePolicy CRs cannot weaken a cluster-wide policy that uses `"default"` scope; the namespace policy will be ignored and the cluster policy takes precedence
  - This ensures application administrators cannot override cluster-wide deny-by-default policies, preserving the security boundary
- Merge rule for ClusterImagePolicy and ImagePolicy CRs:
  - when the syncHandler processing an image scope within a ClusterImagePolicy CR
    - the scope exists in an existing ClusterImagePolicy CR:
    append the policy to existing cluster policy; the policy will be written to `/etc/containers/policy.json` and each `\<NAMESPACE\>.json` if that namespace has an ImagePolicy CR that can be successfully rolled out.
    - if the scope is either equal to or a broader scope than one already present in an ImagePolicy CR: do not roll out the non-global policy in `<NAMESPACE>.json`. The policy for the cluster will be written to `/etc/containers/policy.json` and `<NAMESPACE>.json`;
    update the `status` of both CRs and machine config controller logs to indicate that the policy for cluster has been applied, while the non-global namespace policy is ignored.
    - if none of the above cases apply: the policy will be written to `/etc/containers/policy.json` and each `\<NAMESPACE\>.json` if that namespace has an ImagePolicy CR that can be successfully rolled out.
  - when the syncHandler processing an image scope within an ImagePolicy CR
    - if the policy scope is equal to or nests inside an existing ClusterImagePolicy CR: do not roll out the non-global policy in `\<NAMESPACE\>.json`. the policy for the cluster will be written to `/etc/containers/policy.json` and `\<NAMESPACE\>.json`;
    update the `status` of both CRs and machine config controller logs to indicate that the policy for the cluster has been applied, while the non-global namespace policy is ignored.
    - the scope exists in another ImagePolicy CR:
    append the policy to existing policy; the policy will be written to `<NAMESPACE>.json`
    - if none of the above cases apply:
    the policy will be written to `/path/to/policies/\<NAMESPACE\>.json`
  - the policies will be coordinated with the base [/etc/containers/policy.json](https://github.com/openshift/machine-config-operator/blob/master/templates/master/01-master-container-runtime/_base/files/policy.yaml) file or the Image CR, inheriting the `default` policy from them. The rollout will fail if the following validation with Image CR fails. In such a case, the error will be reported to the machine config logs.
    - Image CR `allowedRegistries` creates deny-by-default policy; ClusterImagePolicy registry scopes must nest inside allowed registries
    - Image CR `blockedRegistries` creates allow-by-default policy; ClusterImagePolicy registry scopes must NOT overlap with blocked registries
    - ClusterImagePolicy scope `"default"` is rejected when Image CR has `allowedRegistries` or `blockedRegistries` configured (Image CR controls the default policy)
    - When ClusterImagePolicy registry scope matches an Image CR allowed registry, ClusterImagePolicy replaces the `insecureAcceptAnything` entry, enabling signature verification or explicit rejection
  - the `/etc/containers/policy.json` holds the cluster wide policy. `\<NAMESPACE\>.json` holds the merged cluster override policy and namespaced policy.
- Image policies that are written to `/etc/containers/policy.json` will be rolled out by machine config `99-<pool>-generated-registries`.

### Policy Type Compatibility Validation

Multiple ClusterImagePolicy resources targeting the same scope must use compatible policy types. Signature verification types (PublicKey, FulcioCAWithRekor, PKI) are compatible and can coexist to support multiple signing authorities. The Reject and InsecureAcceptAnything types are only compatible with themselves.

The admission webhook validates policy type compatibility when creating or updating a ClusterImagePolicy:
- If the scope already exists in another ClusterImagePolicy with an incompatible policy type, the request is rejected
- Error: `Scope '<scope>' already has policy type '<existing-type>' in ClusterImagePolicy '<existing-name>'.`

The same validation applies to ImagePolicy resources within a namespace.

|                                                                                                                 	|process the policies from the CRs                |                                                                                    	|   	|   	|
|-----------------------------------------------------------------------------------------------------------------	|------------------------------------------------	|-----------------------------------------------------------------------------------	|---	|---	|
| same scope in different CRs                                                                                     	| ImagePolicy                                    	| ClusterImagePolicy                                                                	|   	|   	|
| ClusterImagePolicy ImagePolicy (scope in the ClusterImagePolicy is equal to or broader than in the ImagePolicy) 	| Do not deploy non-global policy for this scope 	| Write the cluster policy to `/etc/containers/policy.json`  and `<NAMESPACE>.json` 	|   	|   	|
| ClusterImagePolicy ClusterImagePolicy                                                                           	| N/A                                            	| Append the policy to existing `etc/containers/policy.json`                        	|   	|   	|
| ImagePolicy ImagePolicy                                                                                         	| append the policy to <NAMESPACE>.json          	| N/A                                                                               	|   	|   	|

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
  scopes:
  - "default"  # Default scope - only evaluated when no other scope matches
  policy:
    rootOfTrust:
      policyType: Reject  # Images from docker.io, ghcr.io, or any other registry will be rejected
```

**Example: Namespace-Scoped Deny-by-Default (Without Cluster-Wide Deny)**

> **Note:** This example assumes NO cluster-wide deny-by-default policy exists. If a ClusterImagePolicy with scope `"default"` exists, it takes precedence and namespace policies cannot weaken it. Similarly, if Image CR `allowedRegistries` is configured, namespace ImagePolicy scopes must nest inside the allowed registries.

```yaml
# Deny all images by default in the production namespace only
kind: ImagePolicy
metadata:
  name: default-reject
  namespace: production
spec:
  scopes:
  - "default"  # Default scope - reject all images in this namespace by default
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

For the `production` namespace with the namespace-scoped deny-by-default policy, the following `/path/to/policies/production.json` will be generated:

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

### Drawbacks

## Design Details

### Open Questions [optional]

### Test Plan

**Note:** *Section not required until targeted at a release.*

MCO container runtime config controller can add unit tests and e2e tests.
- unit test: 
  - verify the policies from ClusterImagePolicy and ImagePolicy instances merged correctly and the controller writes correct format policy.json
  - verify `policyType: Reject` and `policyType: InsecureAcceptAnything` generate correct policy.json output
  - verify default scope `"default"` maps to `"default"` key in policy.json
  - verify CEL validation rules (e.g., `policyType: Reject` cannot have `publicKey/fulcioCAWithRekor/pki`, default scope cannot be combined with other scopes)
- e2e test:
  - verify the MCO writes configuration to the correct location.
  - test demonstrating that an unsigned image in violation of the signature verification policy is rejected
  - test demonstrating that a Pod with image from docker.io fails when ClusterImagePolicy has `policyType: Reject` for `scopes: ["default"]`
  - test demonstrating that a Pod with image from allowed registry succeeds when specific scope has `policyType: InsecureAcceptAnything` and default has `policyType: Reject`


### Graduation Criteria

Before GA, validation will be implemented to make sure that scopes cannot be committed to an ImagePolicy that are under the scope of a ClusterImagePolicy, this will prevent conflicts between cluster scoped and namespace scoped policy and prevent namespaces attempting to override the global policy.

Additionally, it will be validated that neither an ImagePolicy nor ClusterImagePolicy sets a scope that could conflict with pulling from an OpenShift image payload repository.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Upgrade expectations:
- In order to use ClusterImagePolicy and ImagePolicy, an existing cluster will have to make an upgrade to the version that the `ClusterImagePolicy` and `ImagePolicy` CRDs are available.
- If the existing cluster manages `/etc/containers/policy.json` via the MachineConfig, the configuration will continue to work if no new `ClusterImagePolicy` or `ImagePolicy` resources created during the upgrade. If `ImagePolicy` resources created and resulting `/path/to/policies/<NAMESPACE>.json` exists, CRI-O will use the config from `/path/to/policies/<NAMESPACE>.json`.
- **Deny-by-default support**: The addition of `Reject` and `InsecureAcceptAnything` to the `policyType` enum is backward compatible. Existing ClusterImagePolicy and ImagePolicy CRs using `policyType: PublicKey`, `policyType: FulcioCAWithRekor`, or `policyType: PKI` continue to work without modification. The new policy types are only used when administrators explicitly create CRs with `policyType: Reject` or `policyType: InsecureAcceptAnything`.

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
