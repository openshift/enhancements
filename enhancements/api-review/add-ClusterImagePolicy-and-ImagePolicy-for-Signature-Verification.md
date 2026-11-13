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
last-updated: 2026-11-11
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://redhat.atlassian.net/browse/OCPNODE-4756
---

# Add CRD ImagePolicy to config.openshift.io/v1

## Summary

This enhancement introduces ClusterImagePolicy and ImagePolicy CRDs to independently manage configurations at the cluster and namespace scopes.
It also lets administrators configure a top-level `reject`
policy for images that do not match any registry scope, instead of leaving them
to the existing `insecureAcceptAnything` default.

## Motivation

### User Stories

As an OpenShift user, I want to verify the container images signed using [sigstore](https://docs.sigstore.dev/about/overview/)
tools, so that I can utilize the increased security of my software supply chain.

As an OpenShift user, I want images that do not match any explicitly configured registry scope to be rejected by default instead of accepted by default, so that an image is accepted only when it matches a scope with a signature-verification policy and satisfies that policy.

As an application administrator, I want to add namespace-scoped signature-verification requirements for image scopes that overlap cluster policies, without weakening the cluster requirements.

### Goals

- ClusterImagePolicy is defined as cluster scoped CRD. ImagePolicy is defined as namespaced CRD.
- The user can create an ImagePolicy instance specifying the images/repositories to be verified and their policy. The MCO will write the configuration for signature verification. Once this is done, CRI-O can verify the images/repositories.
- MCO container runtime config controller watches ImagePolicy instance in different kubernetes namespaces and merges the instances for each namespace into a single [containers-policy.json](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md) in a predefined `/etc/crio/policies/<NAMESPACE>.json`.
- MCO container runtime config controller watches ClusterImagePolicy instance and merges the instances into a single [/etc/containers/policy.json](https://github.com/containers/image/blob/main/docs/containers-policy.json.5.md).
- The user can configure a cluster-wide default that rejects images not matched by any scoped signature-verification policy.
- The user can configure a namespace default that rejects images not matched by any scoped signature-verification policy in that namespace.
- When a cluster default `Reject` exists, image-scoped signature verification policies from ImagePolicy can still be applied.
- The user can revert the top-level default policy by deleting all configuration that requires default reject.
- The user can add namespaced signature-verification requirements while still enforcing matching ClusterImagePolicy requirements, without weakening the ClusterImagePolicy.
- CRI-O can verify Cosign-signed images using configuration from ClusterImagePolicy and ImagePolicy by matching the namespace from the sandbox config on the `PullImage` RPC.

### Non-Goals

- Providing a tool to mirror the signatures is out of the scope of this enhancement. In order to verify the signature, the disconnected users need to mirror signatures together with the application images.
- Grant the application administrator the ability to weaken cluster-scoped policies, to avoid expanding the set of administrators capable of increasing cluster exposure to vulnerable images.
- Automatically enabling deny-by-default policies in existing or new clusters (administrators must explicitly create ClusterImagePolicy or ImagePolicy resources to enable this behavior).
- Restricting execution to only Red Hat-authorized signers in vital OpenShift namespaces (tracked separately in [OCPSTRAT-2761](https://redhat.atlassian.net/browse/OCPSTRAT-2761)).
- Providing emergency bypass mechanisms or Pod-level annotations to skip policy enforcement without updating ClusterImagePolicy or ImagePolicy resources.

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
3. The application administrator can retrieve the cluster override and merged policies by checking `/etc/crio/policies/<NAMESPACE>.json` within the `99-<pool>-generated-registries` machine-config.
4. The application administrator has the option to remove the signature verification configuration by deleting its ImagePolicy instances.
**Cluster Administrator** Deny by Default workflow:
1. The cluster administrator wants images that do not match any configured registry scope to be rejected cluster-wide. When an image matches a scope, it must satisfy that scope's signature-verification requirements.
2. The cluster administrator creates a ClusterImagePolicy CR with deny-by-default policy configuration.
3. The cluster administrator verifies that `/etc/containers/policy.json` in the `99-<pool>-generated-registries` machine-config contains top level default `"default": [{"type": "reject"}]`.

**Application Administrator** Deny by Default workflow:
1. The application administrator wants images in the namespace that do not match any configured registry scope to be rejected by default.
2. The application administrator creates a namespaced ImagePolicy CR with deny-by-default configuration.
3. The application administrator verifies that `/etc/crio/policies/<NAMESPACE>.json` in the `99-<pool>-generated-registries` machine-config contains `"default": [{"type": "reject"}]`.

**Application Administrator** Intersection for Nested Image Scopes workflow:
1. The application administrator wants to add signature-verification requirements for image scopes that overlap registry-scoped ClusterImagePolicies.
2. The application administrator enables intersection merging in an ImagePolicy. Enabling it in any ImagePolicy enables intersection merging for the entire namespace.
3. The application administrator verifies that `/etc/crio/policies/<NAMESPACE>.json` contains the ImagePolicy requirements together with those from the nearest enclosing or exact ClusterImagePolicy scope.

### API Extensions

`ClusterImagePolicy` is expected to have a similar structure to `ImagePolicy`. `intersectionMerge` is available only on `ImagePolicy`.

#### Type definitions

The full field descriptions are maintained in the OpenShift API repository. This section keeps a compact field sketch.

- [ImagePolicy types](https://github.com/openshift/api/blob/master/config/v1/types_image_policy.go)
- [ClusterImagePolicy types](https://github.com/openshift/api/blob/master/config/v1/types_cluster_image_policy.go)

```go
// ImagePolicySpec is the specification of the ImagePolicy CRD.
// +openshift:validation:FeatureGateAwareXValidation:featureGate="",rule="self.policy.rootOfTrust.policyType == 'Reject' || (has(self.scopes) && size(self.scopes) > 0)",message="scopes must be specified unless policyType is Reject"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="self.policy.rootOfTrust.policyType != 'Reject' || has(self.appliesTo)",message="policyType Reject requires appliesTo Default"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="!has(self.appliesTo) || self.appliesTo != 'Default' || self.policy.rootOfTrust.policyType == 'Reject'",message="appliesTo Default requires policyType Reject"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="!has(self.appliesTo) || self.appliesTo != 'Default' || !has(self.scopes)",message="scopes must be empty when appliesTo is Default"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyIntersectionMerge,rule="!has(self.intersectionMerge) || (has(self.scopes) && size(self.scopes) > 0)",message="intersectionMerge requires at least one scope"
// +openshift:validation:FeatureGateAwareXValidation:requiredFeatureGate=ImagePolicyDenyByDefault;ImagePolicyIntersectionMerge,rule="!has(self.intersectionMerge) || !has(self.appliesTo) || self.appliesTo != 'Default'",message="intersectionMerge is not supported with appliesTo Default"
type ImagePolicySpec struct {
	// appliesTo specifies whether this policy applies to images that do not match any registry-specific policy.
	// Allowed value: "Default".
	// +optional
	AppliesTo PolicyAppliesTo `json:"appliesTo,omitempty"`

	// scopes is an optional field that defines the list of image identities assigned to a policy.
	// +optional
	Scopes []ImageScope `json:"scopes,omitempty"`
	// intersectionMerge enables namespace-level intersection merging.
	// Allowed value: "Enabled".
	// +optional
	IntersectionMerge IntersectionMergePolicy `json:"intersectionMerge,omitempty"`
	// policy is a required field that contains configuration to allow scopes to be verified.
	// +required
	Policy ImageSigstoreVerificationPolicy `json:"policy"`
}

// ImageScope is a container image scope. It must contain between 1 and 512 characters.
type ImageScope string

// ImageSigstoreVerificationPolicy defines the verification policy for the items in the scopes list.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=ImagePolicyDenyByDefault,rule="self.rootOfTrust.policyType != 'Reject' || !has(self.signedIdentity)",message="signedIdentity must not be set when policyType is Reject"
type ImageSigstoreVerificationPolicy struct {
	// rootOfTrust is a required field that defines the root of trust for verifying image signatures during retrieval.
	// +required
	RootOfTrust PolicyRootOfTrust `json:"rootOfTrust"`
	// signedIdentity is an optional field that specifies what image identity the signature claims about the image.
	// +optional
	SignedIdentity *PolicyIdentity `json:"signedIdentity,omitempty"`
}

// PolicyRootOfTrust defines the root of trust based on the selected policyType.
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'PublicKey' ? has(self.publicKey) : !has(self.publicKey)",message="publicKey is required when policyType is PublicKey, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'FulcioCAWithRekor' ? has(self.fulcioCAWithRekor) : !has(self.fulcioCAWithRekor)",message="fulcioCAWithRekor is required when policyType is FulcioCAWithRekor, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'PKI' ? has(self.pki) : !has(self.pki)",message="pki is required when policyType is PKI, and forbidden otherwise"
type PolicyRootOfTrust struct {
	// policyType is a required field that specifies the type of the policy for verification.
	// Allowed values: "PublicKey", "FulcioCAWithRekor", "PKI", "Reject". Reject requires appliesTo "Default".
	// +required
	PolicyType PolicyType `json:"policyType"`
	// publicKey defines the root of trust configuration based on a sigstore public key.
	// +optional
	PublicKey *ImagePolicyPublicKeyRootOfTrust `json:"publicKey,omitempty"`
	// fulcioCAWithRekor defines the root of trust configuration based on the Fulcio certificate and the Rekor public key.
	// +optional
	FulcioCAWithRekor *ImagePolicyFulcioCAWithRekorRootOfTrust `json:"fulcioCAWithRekor,omitempty"`
	// pki defines the root of trust configuration based on Bring Your Own Public Key Infrastructure (BYOPKI) Root CA(s) and corresponding intermediate certificates.
	// +optional
	PKI *ImagePolicyPKIRootOfTrust `json:"pki,omitempty"`
}

// ImagePolicyPublicKeyRootOfTrust defines the root of trust based on a sigstore public key.
type ImagePolicyPublicKeyRootOfTrust struct {
	// keyData is a required field that contains inline base64-encoded data for the PEM format public key.
	// +required
	KeyData []byte `json:"keyData"`
	// rekorKeyData is an optional field that contains inline base64-encoded data for the PEM format from the Rekor public key.
	// +optional
	RekorKeyData []byte `json:"rekorKeyData,omitempty"`
}

// ImagePolicyFulcioCAWithRekorRootOfTrust defines the root of trust based on the Fulcio certificate and the Rekor public key.
type ImagePolicyFulcioCAWithRekorRootOfTrust struct {
	// fulcioCAData is a required field that contains inline base64-encoded data for the PEM format Fulcio CA.
	// +required
	FulcioCAData []byte `json:"fulcioCAData"`
	// rekorKeyData is a required field that contains inline base64-encoded data for the PEM format from the Rekor public key.
	// +required
	RekorKeyData []byte `json:"rekorKeyData"`
	// fulcioSubject is a required field that specifies OIDC issuer and the email of the Fulcio authentication configuration.
	// +required
	FulcioSubject PolicyFulcioSubject `json:"fulcioSubject"`
}

// PolicyFulcioSubject defines the OIDC issuer and the email of the Fulcio authentication configuration.
type PolicyFulcioSubject struct {
	// +kubebuilder:validation:XValidation:rule="isURL(self)",message="oidcIssuer must be a valid URL"
	// oidcIssuer is a required field that contains the expected OIDC issuer.
	// +required
	OIDCIssuer string `json:"oidcIssuer"`
	// +kubebuilder:validation:XValidation:rule=`self.matches('^\\S+@\\S+$')`,message="invalid email address"
	// signedEmail is a required field that holds the email address the Fulcio certificate is issued for.
	// +required
	SignedEmail string `json:"signedEmail"`
}

// ImagePolicyPKIRootOfTrust defines the root of trust based on Root CA(s) and corresponding intermediate certificates.
type ImagePolicyPKIRootOfTrust struct {
	// caRootsData contains base64-encoded data of a certificate bundle PEM file.
	// +required
	CertificateAuthorityRootsData []byte `json:"caRootsData"`
	// caIntermediatesData contains base64-encoded data of a certificate bundle PEM file.
	// +optional
	CertificateAuthorityIntermediatesData []byte `json:"caIntermediatesData,omitempty"`
	// pkiCertificateSubject defines the requirements imposed on the subject to which the certificate was issued.
	// +required
	PKICertificateSubject PKICertificateSubject `json:"pkiCertificateSubject"`
}

// PKICertificateSubject defines the requirements imposed on the subject to which the certificate was issued.
// +kubebuilder:validation:XValidation:rule="has(self.email) || has(self.hostname)", message="at least one of email or hostname must be set in pkiCertificateSubject"
type PKICertificateSubject struct {
	// +kubebuilder:validation:XValidation:rule=`self.matches('^\\S+@\\S+$')`,message="invalid email address"
	// email is the email address associated with the certificate identity.
	// +optional
	Email string `json:"email,omitempty"`
	// hostname is the hostname associated with the certificate identity.
	// +optional
	Hostname string `json:"hostname,omitempty"`
}

// PolicyIdentity defines image identity the signature claims about the image.
// +kubebuilder:validation:XValidation:rule="(has(self.matchPolicy) && self.matchPolicy == 'ExactRepository') ? has(self.exactRepository) : !has(self.exactRepository)",message="exactRepository is required when matchPolicy is ExactRepository, and forbidden otherwise"
// +kubebuilder:validation:XValidation:rule="(has(self.matchPolicy) && self.matchPolicy == 'RemapIdentity') ? has(self.remapIdentity) : !has(self.remapIdentity)",message="remapIdentity is required when matchPolicy is RemapIdentity, and forbidden otherwise"
type PolicyIdentity struct {
	// matchPolicy sets the type of matching to be used.
	// Allowed values: "MatchRepoDigestOrExact", "MatchRepository", "ExactRepository", "RemapIdentity".
	// The default is "MatchRepoDigestOrExact".
	// +required
	MatchPolicy IdentityMatchPolicy `json:"matchPolicy"`
	// exactRepository is required if matchPolicy is set to "ExactRepository".
	// +optional
	PolicyMatchExactRepository *PolicyMatchExactRepository `json:"exactRepository,omitempty"`
	// remapIdentity is required if matchPolicy is set to "RemapIdentity".
	// +optional
	PolicyMatchRemapIdentity *PolicyMatchRemapIdentity `json:"remapIdentity,omitempty"`
}

type PolicyMatchExactRepository struct {
	// repository is the reference of the image identity to be matched.
	// +required
	Repository IdentityRepositoryPrefix `json:"repository"`
}

type PolicyMatchRemapIdentity struct {
	// prefix is the prefix of the image identity to be matched.
	// +required
	Prefix IdentityRepositoryPrefix `json:"prefix"`
	// signedPrefix is the prefix of the image identity to be matched in the signature.
	// +required
	SignedPrefix IdentityRepositoryPrefix `json:"signedPrefix"`
}

// +kubebuilder:validation:XValidation:rule=`self.matches('.*:([\\w][\\w.-]{0,127})$')? self.matches('^(localhost:[0-9]+)$'): true`,message="invalid repository or prefix in the signedIdentity, should not include the tag or digest"
// +kubebuilder:validation:XValidation:rule=`self.matches('^(((?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])(?:\\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+(?::[0-9]+)?)|(localhost(?::[0-9]+)?))(?:(?:/[a-z0-9]+(?:(?:(?:[._]|__|[-]*)[a-z0-9]+)+)?)+)?$')`,message="invalid repository or prefix in the signedIdentity. The repository or prefix must starts with 'localhost' or a valid '.' separated domain. If contains registry paths, the path component names must start with at least one letter or number, with following parts able to be separated by one period, one or two underscore and multiple dashes."
type IdentityRepositoryPrefix string

// +kubebuilder:validation:Enum=MatchRepoDigestOrExact;MatchRepository;ExactRepository;RemapIdentity
type IdentityMatchPolicy string

const (
	IdentityMatchPolicyMatchRepoDigestOrExact IdentityMatchPolicy = "MatchRepoDigestOrExact"
	IdentityMatchPolicyMatchRepository        IdentityMatchPolicy = "MatchRepository"
	IdentityMatchPolicyExactRepository        IdentityMatchPolicy = "ExactRepository"
	IdentityMatchPolicyRemapIdentity          IdentityMatchPolicy = "RemapIdentity"
)

type ImagePolicyStatus struct {
	// conditions provide details on the status of this API Resource.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

### Feature Gate

`ImagePolicyDenyByDefault` is enabled in the `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade` feature sets.
`ImagePolicyIntersectionMerge` is enabled in the `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade` feature sets.

#### Feature-Gated Enhancements

The `ImagePolicyDenyByDefault` feature gate enables:
- The `appliesTo` field on ClusterImagePolicy and ImagePolicy.
- The `Reject` policy type.
- CEL validation for default policies.

The `ImagePolicyIntersectionMerge` feature gate enables:
- The `intersectionMerge` field on ImagePolicy.
- CEL validation for intersection merge configuration.

#### Backward Compatibility

When the feature gates are not enabled, existing ClusterImagePolicy and ImagePolicy behavior remains unchanged. Existing resources continue to work without modification.

### Implementation Details/Notes/Constraints [optional]

#### Update container runtime config controller to watch ClusterImagePolicy and ImagePolicy

Enhance the MCO container runtime config controller to manage ClusterImagePolicy and ImagePolicy CRs, and update the signature verification configurations:
- Retrieves all the ClusterImagePolicy and ImagePolicy instances on the cluster.
- Pre-validation:
  - If a cluster-wide default `Reject` policy exists, verify that the payload repositories from `ControllerConfig.Spec.ReleaseImage`, `BaseOSContainerImage`, and `BaseOSExtensionsContainerImage` are covered by a non-`Reject` requirement in the effective policy generated from ClusterImagePolicy scopes or Image CR `allowedRegistries`. If any repository is not covered, do not roll out the generated policy, mark the ClusterImagePolicy Pending, and report the uncovered repositories in the MCO logs. This validation follows the existing `blockedRegistries` validation pattern.
  - When intersection merging is not enabled for a namespace, do not deploy an ImagePolicy scope that equals or nests inside a registry-scoped ClusterImagePolicy scope. Non-conflicting scopes from the same ImagePolicy are still applied. Mark the ImagePolicy Pending only when at least one of its scopes is skipped for this reason.
- Adds following configurations to machine configs
  - machine config `99-<pool>-generated-registries` adds [/etc/containers/registries.d/sigstore-registries.yaml](https://github.com/containers/image/blob/main/docs/containers-registries.d.5.md) to allow matching sigstore signatures for each image scope entry, for example:

    ```yaml
    docker:
      my-registry/image:
        use-sigstore-attachments: true
    ```

- Image scope merge rules for ClusterImagePolicy and ImagePolicy CRs:
  - For an image scope in a ClusterImagePolicy CR:
    - If the same scope exists in multiple ClusterImagePolicies, append all requirements for that scope.
    - The merged cluster policy is written to `/etc/containers/policy.json`.
  - For an image scope in an ImagePolicy CR:
     - The generated namespace policy file `/etc/crio/policies/<NAMESPACE>.json` starts from the merged cluster policy file and appends namespace-scoped ImagePolicy requirements according to the merge rules below:
     - If the same scope exists in multiple ImagePolicies in the namespace, append all requirements for that scope.
     - If an ImagePolicy scope is disjoint from every ClusterImagePolicy scope, or is broader than a matching ClusterImagePolicy scope, add it to the namespace policy file.
     - If intersection is not enabled for the namespace, and an ImagePolicy scope is equal to or nests inside a registry-scoped ClusterImagePolicy scope, do not apply that scope to the namespace policy file. Mark the ImagePolicy `Status.Type: Pending` when at least one of its scopes conflicts. Non-conflicting scopes from the same ImagePolicy remain applied to the namespace policy file.
     - If intersection is enabled for the namespace and ImagePolicy and ClusterImagePolicy scopes overlap, combine the requirements from the nearest enclosing scope in each policy type according to the intersection table below. An exact scope match counts as enclosing. All combined requirements must be satisfied, and the ImagePolicy cannot weaken the ClusterImagePolicy requirements.
     - Intersection opt-in is a namespace-level mode: if any ImagePolicy in a namespace enables intersection, intersection applies uniformly to all ImagePolicies in that namespace. If no ImagePolicy enables intersection, the existing no-intersection conflict behavior applies.
- Top-level default policy merge rules:
  - Multiple ClusterImagePolicy resources with default `Reject` are compatible and produce a single top-level default `Reject` in `/etc/containers/policy.json`.
  - Multiple ImagePolicy resources with default `Reject` in the same namespace are compatible and produce a single top-level default `Reject` in `<NAMESPACE>.json`.
  - A cluster or namespace default `Reject` does not prevent image-scoped signature-verification ImagePolicies from being merged according to the image-scope merge rules above.
- Policies are coordinated with the base [/etc/containers/policy.json](https://github.com/openshift/machine-config-operator/blob/master/templates/master/01-master-container-runtime/_base/files/policy.yaml) file or the Image CR. If the following fails, rollout fails and the MCO reports the error in the machine config controller logs.
    - If `blockedRegistries` is configured, ClusterImagePolicy and ImagePolicy scopes must not equal or nest under any blocked registry.
    - If `allowedRegistries` is configured, ClusterImagePolicy and ImagePolicy scopes must nest under an allowed registry.
  - `/etc/containers/policy.json` holds the cluster-scoped policy. `\<NAMESPACE\>.json` holds the merged cluster and namespace-scoped policies permitted by the merge rules.
- Policies that are written to `/etc/containers/policy.json` and `/etc/crio/policies/<NAMESPACE>.json` will be rolled out by machine config `99-<pool>-generated-registries`.

**Image Scope Merge Without Intersection Opt-In**

| Scope relationship | ImagePolicy merge result | ClusterImagePolicy merge result |
|---|---|---|
| ImagePolicy scope equals or is nested inside a ClusterImagePolicy scope | Do not apply the conflicting ImagePolicy scope. Non-conflicting scopes from the same ImagePolicy remain applied | The ClusterImagePolicy requirement remains in `/etc/containers/policy.json` and is inherited by generated namespace policy files. |
| Same scope in multiple ClusterImagePolicies | N/A | Append all requirements for the same scope to `/etc/containers/policy.json`. |
| Same scope in multiple ImagePolicies in the same namespace | Append all requirements for the same scope to `<NAMESPACE>.json`. | N/A |

**Image Scope Merge With Intersection Opt-In**

When any ImagePolicy in a namespace enables intersection, all ImagePolicies in that namespace use the following rules. All combined requirements must be satisfied.

| Scope relationship | ImagePolicy merge result | ClusterImagePolicy merge result |
|---|---|---|
| ImagePolicy scope exactly matches a ClusterImagePolicy scope | Append all ImagePolicy requirements to the same namespace scope as the ClusterImagePolicy requirements. | Retain all ClusterImagePolicy requirements at that scope. |
| ImagePolicy scope is nested inside a ClusterImagePolicy scope | Copy all requirements from the nearest enclosing ClusterImagePolicy scope into the ImagePolicy scope, then append the ImagePolicy requirements. | Retain the ClusterImagePolicy requirements at the enclosing scope. |
| ImagePolicy scope is broader than a ClusterImagePolicy scope | Add the ImagePolicy requirements at the ImagePolicy scope. For each nested ClusterImagePolicy scope, add the nearest enclosing ImagePolicy requirements to that scope while retaining its ClusterImagePolicy requirements. | Retain the ClusterImagePolicy requirements at the nested scope. |
| ImagePolicy and ClusterImagePolicy scopes are disjoint | Add the ImagePolicy requirements at the ImagePolicy scope. | Leave the ClusterImagePolicy scope unchanged. |
| Same scope in multiple ClusterImagePolicies | N/A | Append all requirements for the same scope before combining them with ImagePolicy requirements. |
| Same scope in multiple ImagePolicies in the same namespace | Append all requirements for the same scope before combining them with ClusterImagePolicy requirements. | N/A |

**Default Reject and Image CR Registry Configuration**

| Image CR configuration | ClusterImagePolicy default Reject | ImagePolicy default Reject | Effective `/etc/containers/policy.json` | Effective target `<NAMESPACE>.json` |
|---|---|---|---|---|
| Neither list or `blockedRegistries` | Not configured | Not configured | Default from the base file: `insecureAcceptAnything` | Inherits default `insecureAcceptAnything` |
| Neither list or `blockedRegistries` | Not configured | Configured | Default from the base file: `insecureAcceptAnything` | Default: `reject` |
| Neither list or `blockedRegistries` | Configured | Not configured | Default: `reject` | Inherits default `reject` |
| Neither list or `blockedRegistries` | Configured | Configured | Default: `reject` | Default: `reject` |
| `allowedRegistries` | Configured or not configured | Configured or not configured | Default: `reject`; allowed scopes: `insecureAcceptAnything` | Default: `reject`; inherits the allowed-scope requirements |

`blockedRegistries` does not change the top-level default. Registry scopes remain subject to the allow/block validation rules described above.

#### Examples of ClusterImagePolicy and ImagePolicy

**Example: Basic Image Scope Merge Without Intersection**

This example uses the default namespace behavior with intersection merging disabled.

```yaml
kind: ClusterImagePolicy
metadata:
  name: mypolicy-0
spec:
  scopes:
  - test0.com
  policy:
    rootOfTrust:
      policyType: FulcioCAWithRekor
      fulcioCAWithRekor:
        fulcioCAData: dGVzdC1jYS1kYXRhLWRhdGE=
        rekorKeyData: dGVzdC1yZWtvci1rZXktZGF0YQ==
        fulcioSubject:
          oidcIssuer: https://OIDC.example.com
          signedEmail: test-user@example.com
    signedIdentity:
      matchPolicy: RemapIdentity
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
    rootOfTrust:
      policyType: PublicKey
      publicKey:
        keyData: dGVzdC1rZXktZGF0YQ==
        rekorKeyData: dGVzdC1yZWtvci1rZXktZGF0YQ==
    signedIdentity:
      matchPolicy: RemapIdentity
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
    rootOfTrust:
      policyType: PublicKey
      publicKey:
        keyData: dGVzdC1rZXktZGF0YQ==
```



Feedback from the container runtime config controller:

```yaml
- lastTransitionTime: "4321-03-07T11:21:39Z"
  message: Policy has scopes test0.com configured for both cluster scope non-global namespaces, only cluster scoped policy will be rolled out
  type: Pending
```

Apply the above CRs, if no Image CRs changes the policy.json. The below `/etc/containers/policy.json` will be rolled out.

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

The merged cluster override policy and namespaced policy in the below `/etc/crio/policies/testnamespace.json` will be rolled out.

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

**Example: Image Scope Intersection With a Nested ImagePolicy Scope**

Intersection is enabled for namespace `testnamespace`.

```yaml
kind: ClusterImagePolicy
metadata:
  name: cluster-team-policy
spec:
  scopes:
  - example.com/team
  policy:
    rootOfTrust:
      policyType: PublicKey
      publicKey:
        keyData: Q0xVU1URVJfS0VZX0E=
```

```yaml
kind: ImagePolicy
metadata:
  name: namespace-app-policy
  namespace: testnamespace
spec:
  intersectionMerge: Enabled
  scopes:
  - example.com/team/app
  policy:
    rootOfTrust:
      policyType: PKI
      pki:
        caRootsData: dGVzdC1rZXktZGF0YQ==
        caIntermediatesData: dGVzdC1rZXktZGF0YQ==
        pkiCertificateSubject:
          email: test-user@test-domain.com
```

The ClusterImagePolicy requirement remains at its original scope in `/etc/containers/policy.json`:

```json
"example.com/team": [
  {
    "type": "sigstoreSigned",
    "keyData": "Q0xVU1URVJfS0VZX0E=",
    "signedIdentity": {
      "type": "matchRepoDigestOrExact"
    }
  }
]
```

The generated `/etc/crio/policies/testnamespace.json` contains both requirements at the more-specific ImagePolicy scope:

```json
"example.com/team": [
  {
    "type": "sigstoreSigned",
    "keyData": "Q0xVU1URVJfS0VZX0E=",
    "signedIdentity": {
      "type": "matchRepoDigestOrExact"
    }
  }
],
"example.com/team/app": [
  {
    "type": "sigstoreSigned",
    "keyData": "Q0xVU1URVJfS0VZX0E=",
    "signedIdentity": {
      "type": "matchRepoDigestOrExact"
    }
  },
  {
    "type": "sigstoreSigned",
    "pki": {
      "caRootsData": "dGVzdC1rZXktZGF0YQ==",
      "caIntermediatesData": "dGVzdC1rZXktZGF0YQ==",
      "subjectEmail": "test-user@test-domain.com"
    },
    "signedIdentity": {
      "type": "matchRepoDigestOrExact"
    }
  }
]
```

**Example: Image Scope Intersection With a Broader ImagePolicy Scope**

Intersection is enabled for namespace `testnamespace`.

```yaml
kind: ImagePolicy
metadata:
  name: namespace-team-policy
  namespace: testnamespace
spec:
  intersectionMerge: Enabled
  scopes:
  - example.com
  policy:
    rootOfTrust:
      policyType: PublicKey
      publicKey:
        keyData: TkFNRVNQQUNFX0tFWV9C
```

Combined with `ClusterImagePolicy/cluster-team-policy` above, the generated `/etc/crio/policies/testnamespace.json` contains:

```json
"example.com": [
  {
    "type": "sigstoreSigned",
    "keyData": "TkFNRVNQQUNFX0tFWV9C",
    "signedIdentity": {
      "type": "matchRepoDigestOrExact"
    }
  }
],
"example.com/team": [
  {
    "type": "sigstoreSigned",
    "keyData": "Q0xVU1URVJfS0VZX0E=",
    "signedIdentity": {
      "type": "matchRepoDigestOrExact"
    }
  },
  {
    "type": "sigstoreSigned",
    "keyData": "TkFNRVNQQUNFX0tFWV9C",
    "signedIdentity": {
      "type": "matchRepoDigestOrExact"
    }
  }
]
```

The broader ImagePolicy requirement is copied into the more-specific ClusterImagePolicy scope because containers/image selects only the most-specific matching scope at runtime.

**Example: Cluster Default Reject With Registry Verification Policies**

```yaml
kind: ClusterImagePolicy
metadata:
  name: default-reject
spec:
  appliesTo: Default
  policy:
    rootOfTrust:
      policyType: Reject
---
kind: ClusterImagePolicy
metadata:
  name: verify-trusted-registries
spec:
  scopes:
  - registry.redhat.io
  - quay.io/openshift-release-dev
  - image-registry.openshift-image-registry.svc:5000
  policy:
    rootOfTrust:
      policyType: PublicKey
      publicKey:
        keyData: dGVzdC1rZXktZGF0YQ==
```

The generated `/etc/containers/policy.json` is:

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
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "matchRepoDigestOrExact"
          }
        }
      ],
      "quay.io/openshift-release-dev": [
        {
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "matchRepoDigestOrExact"
          }
        }
      ],
      "image-registry.openshift-image-registry.svc:5000": [
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

**Example: Cluster Default Reject With a Namespace-Scoped Signature Policy**

```yaml
kind: ClusterImagePolicy
metadata:
  name: default-reject
spec:
  appliesTo: Default
  policy:
    rootOfTrust:
      policyType: Reject
---
kind: ImagePolicy
metadata:
  name: application-signing-policy
  namespace: production
spec:
  scopes:
  - example.com/team/app
  policy:
    rootOfTrust:
      policyType: PublicKey
      publicKey:
        keyData: TkFNRVNQQUNFX0tFWV9C
```

The cluster policy contains the default Reject policy:

```json
{
  "default": [
    {
      "type": "reject"
    }
  ],
  "transports": {
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

The namespace policy contains the default Reject policy and the image-scoped signature-verification policy:

```json
{
  "default": [
    {
      "type": "reject"
    }
  ],
  "transports": {
    "docker": {
      "example.com/team/app": [
        {
          "type": "sigstoreSigned",
          "keyData": "TkFNRVNQQUNFX0tFWV9C",
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

**Example: Namespace Default Reject Without a Cluster-Wide Default**

```yaml
kind: ImagePolicy
metadata:
  name: default-reject
  namespace: production
spec:
  appliesTo: Default
  policy:
    rootOfTrust:
      policyType: Reject
---
kind: ImagePolicy
metadata:
  name: trusted-production-images
  namespace: production
spec:
  scopes:
  - image-registry.openshift-image-registry.svc:5000
  - quay.example.com/production
  policy:
    rootOfTrust:
      policyType: PublicKey
      publicKey:
        keyData: dGVzdC1rZXktZGF0YQ==
```

Because no ClusterImagePolicy default is configured, `/etc/containers/policy.json` retains the base default:

```json
{
  "default": [
    {
      "type": "insecureAcceptAnything"
    }
  ],
  "transports": {
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

The generated `/etc/crio/policies/production.json` contains:

```json
{
  "default": [
    {
      "type": "reject"
    }
  ],
  "transports": {
    "docker": {
      "image-registry.openshift-image-registry.svc:5000": [
        {
          "type": "sigstoreSigned",
          "keyData": "dGVzdC1rZXktZGF0YQ==",
          "signedIdentity": {
            "type": "matchRepoDigestOrExact"
          }
        }
      ],
      "quay.example.com/production": [
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

Risk: A cached image is not revalidated against newly added policy requirements when `imagePullPolicy: IfNotPresent`.

Mitigation: For namespaced policies, CRI-O selects `/etc/crio/policies/<NAMESPACE>.json` at both pull and container-creation time when the file exists, so cached images are revalidated against the namespace policy. Creation-time revalidation for cluster-wide policies is deferred beyond the initial implementation so the behavior and its impact on existing and upgraded clusters can be evaluated before deciding whether to implement it as a follow-up.

### Drawbacks

Creating a cluster default `Reject` can block all images that are not matched by a scoped verification policy. Administrators must configure trusted registry policies for payload and application images before enabling the default.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift manages node configuration through `NodePool.spec.config`, which references management-cluster ConfigMaps containing MCO-compatible manifests. At the referenced [HyperShift revision](https://github.com/openshift/hypershift/blob/f4540b4168b891692eb1c508a336bd4410179803/hypershift-operator/controllers/nodepool/config.go#L460), `ClusterImagePolicy` is accepted through this mechanism. The policy applies across guest namespaces on the configured nodes, but delivery remains NodePool-scoped: adding it to one NodePool does not configure other NodePools.

Namespaced `ImagePolicy` is not currently accepted by the HyperShift NodePool configuration validator. MCO can render `/etc/crio/policies/<NAMESPACE>.json`, but that generation support does not establish the complete HyperShift lifecycle: manifest validation, guest namespace preservation, raw-config generation, node rollout, and CRI-O enforcement must each be verified.

This enhancement does not add HyperShift support for `ImagePolicy` through `NodePool.spec.config`. Future support requires an explicit HyperShift configuration case, preservation of the guest-cluster namespace from the manifest, end-to-end enforcement tests, and coverage on every NodePool where workloads from that namespace can run. Creating `ImagePolicy` resources inside the hosted cluster is a separate lifecycle and is not assumed by this enhancement.

#### Single Node OpenShift (SNO)

SNO has the same policy behavior as other standalone OCP topologies. Its topology-specific impact is that no alternate node is available if a required payload pull fails.

#### OpenShift Local (CRC)

OpenShift Local includes the ClusterImagePolicy and ImagePolicy CRDs and supports both policy types. However, it does not ship the `openshift` ClusterImagePolicy (the cluster is installed with `OPENSHIFT_INSTALL_EXPERIMENTAL_DISABLE_IMAGE_POLICY=true`, preventing the CVO from creating it). Users must manually create allow policies for OpenShift payload registries before enabling default Reject. The same upgrade risk constraints as SNO apply.

## Design Details

### Open Questions [optional]

### Test Plan

#### Test Feature Gate: ImagePolicyIntersectionMerge

API validation tests (`openshift/api`):
- Verify CEL rejects `intersectionMerge: Enabled` when `scopes` is empty.
- Verify CEL rejects `intersectionMerge: Enabled` when combined with `appliesTo: Default`.

MCO unit tests:
- Verify exact, nested, broader, and disjoint ImagePolicy and ClusterImagePolicy scope combinations produce the expected namespace policy.
- Verify enabling `intersectionMerge` in any ImagePolicy enables it for all ImagePolicies in that namespace.

E2E tests tagged `[OCPFeatureGate:ImagePolicyIntersectionMerge]`:
- Verify an exact-scope intersection requires both the ImagePolicy and ClusterImagePolicy signatures.
- Verify a nested ImagePolicy requirement is materialized together with the nearest enclosing ClusterImagePolicy requirement.
- Verify a broader ImagePolicy requirement is enforced within a more-specific ClusterImagePolicy scope.
- Verify enabling intersection in one ImagePolicy applies it to all ImagePolicies in the namespace.
- Verify an image satisfying only one side of an intersection is rejected.

#### Test Feature Gate: ImagePolicyDenyByDefault

API validation tests (`openshift/api`):
- Verify CEL rejects `policyType: Reject` when `publicKey`, `fulcioCAWithRekor`, `pki`, or `signedIdentity` is set.
- Verify CEL rejects `appliesTo: Default` when `scopes` is non-empty.
- Verify CEL rejects omitted `appliesTo` when `scopes` is empty.
- Verify CEL rejects `appliesTo: Default` when `policyType` is not `Reject`.

MCO unit tests:
- Verify ClusterImagePolicy and ImagePolicy policies merge correctly and generate the expected `policy.json`.
- Verify a default Reject policy generates the top-level `default: reject` entry in global and namespace policy files.
- Verify a scoped policy generates transport-specific entries.

E2E tests tagged `[OCPFeatureGate:ImagePolicyDenyByDefault]`:
- Verify an image not covered by any scoped verification policy is rejected by a cluster default Reject.
- Verify that an image matching a scoped signature-verification policy is accepted under a cluster default Reject.
- Verify a namespace default Reject rejects images that do not match any scoped verification policy when no cluster-wide default Reject exists.
- Verify a scoped signature-verification policy is accepted under a namespace default Reject.
- Verify a ClusterImagePolicy with default Reject is Pending and the generated policy is not rolled out when payload repositories are not covered by non-Reject scopes.

#### Test Feature gate: SigstoreImageVerification (GA - OCP 4.21)

MCO container runtime config controller can add unit tests and e2e tests.
- unit test: verify the policies from ClusterImagePolicy and ImagePolicy instances merged correctly and the controller writes correct format policy.json
- e2e test:
  - verify the MCO writes configuration to the correct location.
  - test demonstrating that an unsigned image in violation of the policy is rejected


### Graduation Criteria

#### Dev Preview -> Tech Preview

**`ImagePolicyDenyByDefault` feature gate**
- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

**`ImagePolicyDenyByDefault` and `ImagePolicyIntersectionMerge` feature gates**
- Each gate has at least five corresponding e2e tests tagged `[OCPFeatureGate:<gate-name>]`
- Each test runs 14+ times per platform (x86_64, aarch64, ppc64le, s390x)
- 95%+ pass rate across all platforms
- No known critical bugs or security issues
- Complete user-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/).
- Gather sufficient feedback from Tech Preview users.

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Upgrade expectations:
- During an upgrade to a version that introduces these CRDs, fields, or required CRI-O support, do not create new ClusterImagePolicy or ImagePolicy resources until MCO and CRI-O have completed the upgrade on all nodes.
- In order to use ClusterImagePolicy and ImagePolicy, an existing cluster will have to make an upgrade to the version that the `ClusterImagePolicy` and `ImagePolicy` CRDs are available.
- If the existing cluster manages `/etc/containers/policy.json` via the MachineConfig, the configuration will continue to work if no new `ClusterImagePolicy` or `ImagePolicy` resources created during the upgrade. If `ImagePolicy` resources created and resulting `/path/to/policies/<NAMESPACE>.json` exists, CRI-O will use the config from `/path/to/policies/<NAMESPACE>.json`.
- **Feature-gated support**:
    - Enable `ImagePolicyDenyByDefault` to use
deny-by-default image policies.
    - Enable `ImagePolicyIntersectionMerge` to use
intersection merging.
    - Existing ClusterImagePolicy and ImagePolicy resources
continue to work without modification when these gates are disabled.

Downgrade expectations:
- If the target older version does not provide the ClusterImagePolicy and ImagePolicy CRDs, remove those resources before downgrading. After downgrade, the API support and configured policies are unavailable unless the cluster is upgraded again to a version that supports them.
- After `ImagePolicyDenyByDefault` or `ImagePolicyIntersectionMerge` is promoted beyond Tech Preview, a downgrade to a version that does not support the enabled fields requires removing or converting ClusterImagePolicy and ImagePolicy resources that use `appliesTo`, default `Reject`, or `intersectionMerge`.
- After downgrade, re-create policies using only fields supported by the target version. Deny-by-default and intersection merging remain unavailable until the cluster is upgraded to a version that provides and enables their feature gates.


### Version Skew Strategy

The base ClusterImagePolicy and ImagePolicy APIs require CRI-O 1.28 or later.

-The deny-by-default and intersection-merge fields require the corresponding feature gates, CRD schema, and MCO merge implementation to be available in the
same OCP release.

### Operational Aspects of API Extensions

#### Failure Modes

- CR field syntax error failure reported from CLI
- CR conflicting value failure: rollout failure according to the merge rules [Implementation Details](#Update-container-runtime-config-controller-to-watch-ClusterImagePolicy-and-ImagePolicy). The failure will be reported by machine config controller logs
- MCO rolling out configuration file failure reported by MCO
- If the signature validation fails, then CRI-O will report that in the same way as for any other pull failure. Further enhancements to the kubelet, CRI and CRI-O error handling can be achieved in future Kubernetes releases.

Errors that prevent the desired policy from being generated leave the previous policy active. A cluster default `Reject` that blocks payload repositories can prevent nodes from updating or joining and can therefore affect cluster health.
The OCP Node team is likely to be called upon in case of escalation with one of the failure modes.

#### Support Procedures

- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)
  - If a ClusterImagePolicy or ImagePolicy resource does not meet the API and CEL validation requirements, the API server rejects the create or update request.
  - If MCO does not roll out a ClusterImagePolicy or ImagePolicy resource, the machine-config-controller logs report the reason.

- Disabling or removing the CRD requires deleting the CR instances first. The customer loses the stored policy configuration.

- What consequences does it have on existing, running workloads?
  - Running containers are not interrupted, but containers recreated from cached images can be revalidated against namespace policy. New pulls can fail if they do not satisfy the effective policy.
  - Create/Update/Delete the ClusterImagePolicy and ImagePolicy resources does not require node reboots. It follows the current [Reload Crio](https://github.com/openshift/machine-config-operator/blob/ff7ef2ec8ddbdf4f5758ee8f3ba3fea2d364e581/docs/MachineConfigDaemon.md?plain=1#L165) action.

- What consequences does it have for newly created workloads?
  - In order to verify signature for new pods, ClusterImagePolicy, ImagePolicy instances should be successfully rolled out before creating new workloads.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  - Users can correct the invalid resource or create another valid policy. The previous generated policy remains active until a new valid policy is rolled out.

## Implementation History

- [OCPNODE-1628: Sigstore Support - OpenShift Container Image Validation for cluster wide policies (Dev Preview)](https://issues.redhat.com/browse/OCPNODE-1628) epic will keep track of the ClusterImagePolicy implementation.
- [OCPNODE-2253: OpenShift Container Image Validation for namespaced policies](https://issues.redhat.com/browse/OCPNODE-2253) epic will keep track of the ImagePolicy implementation.
- [OCPSTRAT-2073: GA for sigstore API(clusterimagepolicy, imagepolicy)](https://redhat.atlassian.net/browse/OCPSTRAT-2073)
- [OCPNODE-4756: (Cluster)ImagePolicy for reject-by-default](https://redhat.atlassian.net/browse/OCPNODE-4756) epic tracks the deny-by-default policy extension by adding `Reject` to the existing `policyType` enum.

## Alternatives (Not Implemented)

Not applicable.

## Infrastructure Needed [optional]

- Registry proxies like registry.k8s.io are not natively usable: https://github.com/containers/image/issues/1952
Workaround: using remapIdentity
