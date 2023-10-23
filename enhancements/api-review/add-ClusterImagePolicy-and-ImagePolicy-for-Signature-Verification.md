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
last-updated: 2023-10-19
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-1628
---

# Add CRD ImagePolicy to config.openshift.io/v1alpha1

## Summary

This enhancement introduces ClusterImagePolicy and ImagePolicy CRDs to independently manage configurations at the cluster and namespace scopes.

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

### Non-Goals

- Configuring policies for the images in the OCP payload is not within the scope of this enhancement.
- Providing a tool to mirror the signatures is out of the scope of this enhancement. In order to verify the signature, the disconnected users need to mirror signatures together with the application images. 
- Grant the application administrator the ability to weaken cluster-scoped policies, to avoid expanding the set of administrators capable of increasing cluster exposure to vulnerable images.
- Grant the application administrator the ability to tighten cluster-scoped policies. This could be useful in the future, but we are deferring it to limit the amount of work needed for an initial implementation.

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
3. The application administrator can retrieve the cluster override and merged policies by checking `<NAMESPACE>.json` within the `99-<pool>-generated-imagepolicies` machine-config. 
4. The application administrator has the option to remove the signature verification configuration by deleting its ImagePolicy instances.

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
type ImagePolicySpec struct {
	// scopes defines the list of image identities assigned to a policy. Each item refers to a scope in a registry implementing the "Docker Registry HTTP API V2".
	// Scopes matching individual images are named Docker references in the fully expanded form, either using a tag or digest. For example, docker.io/library/busybox:latest (not busybox:latest).
	// More general scopes are prefixes of individual-image scopes, and specify a repository (by omitting the tag or digest), a repository
	// namespace, or a registry host (by only specifying the host name and possibly a port number) or a wildcard expression starting with *., for matching all subdomains (not including a port number).
	// Wildcards are only supported for subdomain matching, and may not be used in the middle of the host, i.e.  *.example.com is a valid case, but example*.*.com is not.
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

// +kubebuilder:validation:XValidation:rule="self.matches('^[a-zA-Z0-9-_+.*@:/]+$')",message="invalid
// image scope format, scope contained invalid characters, valid characters are [a-zA-Z0-9-*.@_]"
// +kubebuilder:validation:XValidation:rule="size(self.split('/')[0].split('.')) == 1 ? self.split('/')[0].split('.')[0].split(':')[0] == 'localhost' : true",message="invalid image scope format,
// scope must contain a fully qualified domain name or 'localhost'"
// +kubebuilder:validation:XValidation:rule=`self.contains('*') ? self.matches('^\\*(?:\\.(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9]))+$') : true`,message="invalid image scope with wildcard, a wildcard
// can only be at the start of the domain and does not contain subdomains"
// +kubebuilder:validation:MaxLength=512
type ImageScope string

// Policy defines the verification policy for the items in the scopes list.
// +kubebuilder:validation:XValidation:rule="(has(self.rootOfTrust) && has(self.rootOfTrust.policyType) && self.rootOfTrust.policyType == 'FulcioCAWithRekor') == has(self.fulcioSubject)",message="fulcioSubject must be set exactly when policyType is FulcioCAWithRekor"
type Policy struct {
	// rootOfTrust specifies the root of trust for the policy.
	// +kubebuilder:validation:Required
	RootOfTrust PolicyRootOfTrust `json:"rootOfTrust"`
	// fulcioSubject specifies OIDC issuer and the email of the Fulcio authentication configuration.
	// Required if rootOfTrust is based on Fulcio.
	// +optional
	FulcioSubject PolicyFulcioSubject `json:"fulcioSubject,omitempty"`
	// signedIdentity specifies what image identity the signature claims about the image.
	// +optional
	SignedIdentity PolicyIdentity `json:"signedIdentity,omitempty"`
}

// PolicyRootOfTrust defines the root of trust based on the selected policyType.
// +union
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'PublicKey' ? has(self.publicKey) : true",message="must set publicKey if policyType is PublicKey"
// +kubebuilder:validation:XValidation:rule="has(self.policyType) && self.policyType == 'FulcioCAWithRekor' ? has(self.fulcioCAWithRekor) : true",message="must set fulcioCAWithRekor if policyType is FulcioCAWithRekor"
type PolicyRootOfTrust struct {
	// policyType serves as the union's discriminator. Users are required to assign a value to this field, choosing one of the policy types that define the root of trust.
	// "PublicKey" indicates that the policy relies on a PGP publicKey and may optionally use a Rekor verification.
	// "FulcioCAWithRekor" indicates that the policy is based on the Fulcio certification and incorporates a Rekor verification.
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	PolicyType PolicyType `json:"policyType"`
	// PublicKey defines the root of trust based on a PGP public key.
	// +optional
	PublicKey *PublicKey `json:"publicKey,omitempty"`
	// FulcioCAWithRekor defines the root of trust based on the Fulcio certificate and the Rekor public key.
  // For more information about Fulcio and Rekor, please refer to the document at:
	// https://github.com/sigstore/fulcio and https://github.com/sigstore/rekor
	// +optional
	FulcioCAWithRekor *FulcioCAWithRekor `json:"fulcioCAWithRekor,omitempty"`
}

// +kubebuilder:validation:Enum=PublicKey;FulcioCAWithRekor
type PolicyType string

const (
	PublicKeyRootOfTrust         PolicyType = "PublicKey"
	FulcioCAWithRekorRootOfTrust PolicyType = "FulcioCAWithRekor"
)

// PublicKey defines the root of trust based on a PGP public key.
type PublicKey struct {
	// keyData contains inline base64 encoded data of the public key.
	// +kubebuilder:validation:Required
	KeyData string `json:"keyData"`
	// rekorKeyData contains inline base64 data of the Rekor public key.
	// +optional
	RekorKeyData string `json:"rekorKeyData,omitempty"`
}

// FulcioCAWithRekor defines the root of trust based on the Fulcio certificate and the Rekor public key.
type FulcioCAWithRekor struct {
	// fulcioCAData contains inline base64 data for the fulcio CA certificate.
	// +kubebuilder:validation:Required
	FulcioCAData string `json:"fulcioCAData"`
	// rekorKeyData contains inline base64 data of the Rekor public key.
	// +kubebuilder:validation:Required
	RekorKeyData string `json:"rekorKeyData"`
}

// PolicyFulcioSubject defines the OIDC issuer and the email of the Fulcio authentication configuration.
type PolicyFulcioSubject struct {
	// oidcIssuer contains the expected OIDC issuer.
	// Example: "https://expected.OIDC.issuer/"
	// +kubebuilder:validation:Required
	OIDCIssuer string `json:"oidcIssuer"`
	// signedEmail holds the email address the the certificate is issued for.
	// Example: "expected-signing-user@example.com"
	// +kubebuilder:validation:Required
	SignedEmail string `json:"signedEmail"`
}

// PolicyIdentity defines image identity the signature claims about the image. When omitted, the default matchPolicy is "MatchRepoDigestOrExact".
// +kubebuilder:validation:XValidation:rule="(has(self.matchPolicy) && self.matchPolicy == 'ExactRepository') ? has(self.exactRepository) : true",message="must set exactRepository if matchPolicy is ExactRepository"
// +kubebuilder:validation:XValidation:rule="(has(self.matchPolicy) && self.matchPolicy == 'RemapIdentity') ? has(self.remapIdentity) : true",message="must set remapIdentity if matchPolicy is RemapIdentity"
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

type PolicyMatchExactRepository struct {
	// repository is the reference of the image identity to be matched.
	// the value should be a repository (by omitting the tag or digest) in a registry implementing the "Docker Registry HTTP API V2". For example, docker.io/library/busybox
	// +kubebuilder:validation:Required
	Repository string `json:"repository"`
}

type PolicyMatchRemapIdentity struct {
	// prefix is the prefix of the image identity to be matched.
	// If the image identity matches the specified prefix, that prefix is replaced by the specified “signedPrefix” (otherwise it is used as unchanged and no remapping takes place).
	// The prefix and signedPrefix values can be either host[:port] values (matching exactly the same host[:port], string), repository namespaces,
	// or repositories (i.e. they must not contain tags/digests), and match as prefixes of the fully expanded form.
	// For example, docker.io/library/busybox (not busybox) to specify that single repository, or docker.io/library (not an empty string) to specify the parent namespace of docker.io/library/busybox.
	// +kubebuilder:validation:Required
	Prefix string `json:"prefix"`
	// signedPrefix is the prefix of the image identity to be matched in the signature. The format is the same as "prefix".
	// +kubebuilder:validation:Required
	SignedPrefix string `json:"signedPrefix"`
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
  - machine config `99-<pool>-generated-imagepolicies` adds [/etc/containers/registries.d/*.yaml](https://github.com/containers/image/blob/main/docs/containers-registries.d.5.md) to allow matching sigstore signatures, for example:

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
  - the policies will be coordinated with the base [/etc/containers/policy.json](https://github.com/openshift/machine-config-operator/blob/master/templates/master/01-master-container-runtime/_base/files/policy.yaml) file or the Image CR, inheriting the `default` policy from them.
  If the signature policy scope is also configured as `insecureAcceptAnything` or `reject`, the rollout will fail. In such a case, the error will be reported to the machine config logs.
  - the `/etc/containers/policy.json` holds the cluster wide policy. `\<NAMESPACE\>.json` holds the merged cluster override policy and namespaced policy.
- Image policies that are written to `/etc/containers/policy.json` will be rolled out by machine config `99-<pool>-generated-registries`. Merged policies that are written to `<NAMESPACE>.json` will be rolled out by machine config `99-<pool>-generated-imagepolicies`.
- Once an ImagePolicy or ClusterImagePolicy object gets created/updated, container runtime config controller will call the syncHandler, create machine config `99-<pool>-generated-imagepolicies` if the namespaced CR can successfully rollout.
Once the last ImagePolicy or ClusterImagePolicy object get deleted, container runtime config controller will delete the machine config `99-[pool]-generated-imagepolicies`. It then runs the syncHandler to coordinate policy updates with the Image CR.

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

Feedback from the container runtime config controller:

```yaml
- lastTransitionTime: "4321-03-07T11:21:39Z"
  message: Policy has scopes test0.com configured for both cluster scope non-global namespaces, only cluster scoped policy will be rolled out
  type: PolicyPending
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
- unit test: verify the policies from ClusterImagePolicy and ImagePolicy instances merged correctly and the controller writes correct format policy.json
- e2e test: 
  - verify the MCO writes configuration to the correct location. 
  - test demonstrating that an unsigned image in violation of the policy is rejected


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

[OCPNODE-1628: Sigstore Support - OpenShift Container Image Validation (Dev Preview)](https://issues.redhat.com/browse/OCPNODE-1628) epic will keep track of the implementation.

## Alternatives

Not applicable.

## Infrastructure Needed [optional]

- Registry proxies like registry.k8s.io are not natively usable: https://github.com/containers/image/issues/1952
Workaround: using remapIdentity
