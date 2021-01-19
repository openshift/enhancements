---
title: aws-pod-identity
authors:
  - "@sjenning"
reviewers:
  - "@derekwaynecarr"
  - "@smarterclayton"
  - "@joelddiaz"
  - "@dgoodwin"
  - "@abhinavdahiya"
  - "@eparis"
  - "@cuppett"
  - "@marun"
approvers:
  - "@derekwaynecarr"
creation-date: 2020-03-26
last-updated: 2020-03-26
status: provisional
see-also:
  - ""
replaces:
  - ""
superseded-by:
  - ""
---

# Support Native (IAM-based) Pod Identity on AWS

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

### ServiceAccountIssuerDiscovery (RESOLVED)

#### Question

Should we enable the [`ServiceAccountIssuerDiscovery` feature gate](https://github.com/kubernetes/enhancements/issues/1393) (disabled by default, alpha 1.18)?

Pros:
- Eliminates the need for external hosting of the OIDC discovery and keys documents
- Eliminates the need for a controller to sync the keys when the bound
  ServiceAccount signer rotates

Cons:
- Requires enabling alpha feature
- Does not work if kube-apiserver is not publicly accessible from AWS STS

The alternative is to use a pubic S3 bucket to host the OIDC discovery endpoint

Pros:
- Works even with the apiserver is not accessible by AWS STS
  (disconnected/private cluster)

Cons:
- Requires a controller to reconcile the discovery documents as the service
  account issuer CA is rotated periodically.
- Requires the CCO to have additional privileges to create and write to a public
  S3 bucket.
- Conflicts with the upstream approach.
- Will require the creation of a new cluster config type to enable/disable this
  controller as customers in restricted environments may wish to disable this

#### Resolution

Because the apiserver is not always reachable by AWS STS, it was decided that it
would be better (i.e. work in all situations) to create and reconcile the OIDC
discovery endpoint ourselves as a public S3 bucket.

## Summary

When deployed on AWS and configured to use [bound service
tokens](https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/bound-sa-tokens.md),
Kubernetes pods can assume Roles managed by IAM via a binding created by a
ServiceAccount annotation.  AWS has a
[webhook](https://github.com/aws/amazon-eks-pod-identity-webhook/) they maintain
for EKS that does enables this ability.  This enhancement integrates the same
functionality into OpenShift.

The `cloud-credential-operator` will be responsible for managing this new
ability as it serves as an alternative credential _injection_ mechanism versus
the existing mechanism of injecting an IAM user access ID and key into a
secret.  The existing role/user _creation_ via a CredentialRequest CR is common
between the methods.

The new injection mechanism works by annotating a `ServiceAccount` with a IAM
role ARN the pod should assume. When a pod runs with the annotated
`ServiceAccount`, the webhook injects a environment variables and a projected
volume into the pod that contains the bound token and Role ARN that can be used
in AWS SDK client creation.  The AWS SDK searches for this token automatically.

This will be implemented in two phases.

### Phases

Phase 1 will achieve parity with the present-day EKS experience.  This includes
automatic configuration and deployment of the `aws-pod-identity-webhook` and the
creation of an OIDC discovery endpoint that AWS STS can use to validate cluster
created bound service account tokens.  Role creation in IAM and `ServiceAccount`
annotation will be left as an exercise to the user (as it is in EKS)

Phase 2 will make the Role creation and `ServiceAccount` annotation
automatic, just as the User creation and `Secret` injection are today.  This will
involve changing the API in the CredentialRequest CRD to allow references to
`ServiceAccounts` in addition to `Secerts` as an injection mechanism for the
requested credential.

## Motivation

This allows users to more cleanly express and manage pod identities in IAM when
running on AWS.  The use of bound tokens to assume roles (vs user id/key) is
preferred as these tokens can expire and be rotated.

### Goals

#### Phase 1

The OOTB experience with respect to IAM-based pod identity running OpenShift
on AWS is the same vs EKS.

1. The `aws-pod-identity-webook` is deployed and managed as part of the
   cluster infrastructure
1. The cluster creates and managed an OIDC discovery endpoint suitable for STS
  to do bounded service account token authentication against

#### Phase 2

1. An admin can create a IAM Role via `CredentialRequest` CR and inject (via
   annotation) to a `ServiceAccount`, just as they can create an IAM user and
   inject to a `Secret` currently.

### Non-Goals

1. Convert existing IAM user-based components (e.g. ingress, image-registry,
   machine-api) to use bound tokens.  The migration path will likely be manual
   if an admin for an existing cluster wishes to move solely to this method.  At
   some point in the future, we could migrate core components on fresh installs
   to use this method if we work out the ordering i.e. the webhook needs to be
   running and the IAM roles need to exist before any of the pods for these
   components are created and would require change and coordination with the
   operators that manage those components.
2. Registration of the OIDC provider in IAM.  EKS leaves this up to the user and
   it seems the safe thing to do is follow suit.  EKS _does_ create the OIDC
   endpoint that contains the discovery endpoint.  Note that there is
   a hard limit of 100 OIDC providers in IAM per account.  Leaving the OIDC
   registration as an exercise for the user allows them to choose which clusters
   have this enabled, allowing more that 100 clusters per account if so desired.
3. In a more generic form of #1, we are not looking to support, at this time,
   any workloads that could start before the webhook is active i.e. before
   cluster install is complete.

## Proposal

### Phase 1: EKS-Parity User Experience

Considerable functionality will be added to the `cloud-credential-operator`
(CCO).  This functionality is AWS-only and will only be active for clusters
deployed with `platform: aws`.

A new `aws-pod-identity-webhook` image will be added to the release payload
based on [this](https://github.com/openshift/aws-pod-identity-webhook) repo.

A new controller will be added to the CCO to create and reconcile the
`aws-pod-identity-webhook` deployment and its associated resources.

Another new controller will be added to the CCO to create and reconcile an OIDC
discovery endpoint that will contain the valid bounded service account signing
keys that allow AWS Simple Token Service (STS) to authenticate bounded service
account tokens created by the cluster.  These keys are contained in
`bound-sa-token-signing-certs` configmap in the `openshift-kube-apiserver`
namespace.  IAM permissions for the CCO will be expanded to include privileges
required for creating the discovery endpoint

### Phase 2: Automatic Role creation and ServiceAccount annotation

In terms of API changes, an additional `serviceAccountRef` field will be added
to the `CredentialsRequests` CR `spec` alongside the existing [`secretRef`
field](https://github.com/openshift/cloud-credential-operator/blob/9cc1c1abf898cfa08429d465da0f02d064bd89ee/manifests/00_v1_crd.yaml#L37-L42).
Usage of this field is an indication by the user that they wish to use this new
bound token injection mechanism.  The CCO will create a role, instead of a user,
in response and create a `ServiceAccount` with the appropriate role annotation.
A pod started with this `ServiceAccount` will have a token injected by the
mutating webhook.  A `serviceAccountRef` specified on a `CredentialsRequests`
when the platform is not AWS will result in an error reported in the
`CredentialsRequests` `status`.

There is also a small installer impact in that we need to change the
`serviceAccountIssuer` in the `Authentication` config CR day-1 e.g. at install
manifest render time

### Implementation Details/Notes/Constraints [optional]

#### OIDC Provider Creation and Reconciliation (Phase 1)
The name of the bucket and OIDC provider can be generated from the
`status.infrastructureName` in the `Infrastructure` global config.  The prefix
`sjenning-abcde` will be used in this document.

The following is a bash script that does what the OIDC controller will need to
do to create the `discovery.json` and `keys.json`.  The controller will need to
watch `bound-sa-token-signing-certs` for change and update `keys.json`
```bash
#!/bin/bash

set -xe

export S3_BUCKET=sjenning-abcde-oidc-provider
export AWS_REGION=us-west-1

# Extract the serviceaccount keypair from cluster
PKCS_KEY="sa-signer.pub"
oc get -n openshift-kube-apiserver configmap -ojson bound-sa-token-signing-certs | jq -r '.data["service-account-001.pub"]' > $PKCS_KEY

_bucket_name=$(aws s3api list-buckets  --query "Buckets[?Name=='$S3_BUCKET'].Name | [0]" --out text)
if [ $_bucket_name == "None" ]; then
    aws s3api create-bucket --bucket $S3_BUCKET --create-bucket-configuration LocationConstraint=$AWS_REGION
fi
echo "export S3_BUCKET=$S3_BUCKET"
export HOSTNAME=s3-$AWS_REGION.amazonaws.com
export ISSUER_HOSTPATH=$HOSTNAME/$S3_BUCKET

cat <<EOF > discovery.json
{
    "issuer": "https://$ISSUER_HOSTPATH/",
    "jwks_uri": "https://$ISSUER_HOSTPATH/keys.json",
    "authorization_endpoint": "urn:kubernetes:programmatic_authorization",
    "response_types_supported": [
        "id_token"
    ],
    "subject_types_supported": [
        "public"
    ],
    "id_token_signing_alg_values_supported": [
        "RS256"
    ],
    "claims_supported": [
        "sub",
        "iss"
    ]
}
EOF

./self-hosted -key $PKCS_KEY  | jq '.keys += [.keys[0]] | .keys[1].kid = ""' > keys.json

aws s3 cp --acl public-read ./discovery.json s3://$S3_BUCKET/.well-known/openid-configuration
aws s3 cp --acl public-read ./keys.json s3://$S3_BUCKET/keys.json

curl https://$ISSUER_HOSTPATH/.well-known/openid-configuration
curl https://$ISSUER_HOSTPATH/keys.json
```
The mentioned `self-hosted` binary is from
[here](https://github.com/openshift/aws-pod-identity-webhook/tree/master/hack/self-hosted)

#### Modifying initial `Authentication` global config (Phase 1)

In order for any of this to work, the `serviceAccountIssuer` must be set in the `Authentication` global config
```yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  annotations:
    release.openshift.io/create-only: "true"
  name: cluster
spec:
  serviceAccountIssuer: https://s3-us-west-1.amazonaws.com/sjenning-abcde-oidc-provider
status:
  integratedOAuthMetadata:
    name: oauth-openshift
```
Ideally, new 4.5 clusters deployed on platform `aws` would roll out with this set day-1.

For day-2 deployments, changing this results in a redeployment of
`kube-apiserver` pods to adjust the [underlying
options](https://github.com/openshift/aws-pod-identity-webhook/blob/master/SELF_HOSTED_SETUP.md#kubernetes-api-server-configuration).
The existing service account key file continues to be used so that existing
`ServiceAccount` tokens remain valid. Additional context can be found in the
[bounded token enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/bound-sa-tokens.md)

#### Modification to `CredentialsRequest` CRD (Phase 2)

Because the current `secretRef` in the CRD spec is a value and not a pointer, it
is not nullable. If it were nullable, we could add a union discriminator and
`serviceAccountRef` pointer field to the spec.

As it is, we need to deprecate the `spec.secretRef` field in favor of a new
union field named `storage` that contains pointer for a secret and
serviceaccount storage type selected by a discriminator.

`types.go`
```go
// CredentialsRequestSpec defines the desired state of CredentialsRequest
type CredentialsRequestSpec struct {
        // SecretRef points to the secret where the credentials should be stored once generated.
        // +optional
        SecretRef corev1.ObjectReference `json:"secretRef,omitempty"`

        // Storage specifies the type and location of a resource to which the credentials
        // information can be stored or linked.
        Storage Storage `json:"storage,omitempty"`

        // ProviderSpec contains the cloud provider specific credentials specification.
        ProviderSpec *runtime.RawExtension `json:"providerSpec,omitempty"`
}

const (
        SecretStorageType         = "Secret"
        ServiceAccountStorageType = "ServiceAccount"
)

// Storage defines the type and location of a resource to which the credentials
// information can be stored or linked.
// +union
type Storage struct {
        // StorageType contains the type of storage that should be used for the credentials.
        // +unionDiscriminator
        // +required
        Type string `json:"type"`

        // Secret specifies a secret in which to store the credentials.
        // +optional
        Secret *SecretStorage `json:"secret,omitempty"`

        // ServiceAccount specifies a serviceaccount to which the credentials will be linked.
        // This is currently only supported on AWS.
        // +optional
        ServiceAccount *ServiceAccountStorage `json:"serviceAccount,omitempty"`
}

// SecretStorage specifies a secret into which user credentials will be injected.
type SecretStorage struct {
        // Namespace specifies the namespace of the secret into which user credentials will be injected
        // +required
        Namespace string `json:"namespace"`

        // Name specifies the name of the secret into which user credentials will be injected
        // +required
        Name string `json:"name"`
}

// ServiceAccountStorage specifies a serviceaccount to which a role will be linked.
type ServiceAccountStorage struct {
        // Namespace specifies the namespace of the serviceaccount to which a role will be linked.
        // +required
        Namespace string `json:"namespace"`

        // Name specifies the name of the serviceaccount to which a role will be linked.
        // +required
        Name string `json:"name"`
}
```

If the `storage.type` field is not set, the type will be assume to be `Secret` and the name and namespace will be used from the deprecated `secretRef` field.  This satisfies backward compatibility.

We also avoid using `ObjectReference` type for the new fields as its use is [discouraged](https://github.com/kubernetes/api/blob/0cabc089cabafb6be30b40640bbc3ef966a5f00f/core/v1/types.go#L5042-L5058)

The AWS types will not need to be modified, but the use of the `User` field of the `AWSProviderStatus` type will be overloaded to store either the IAM User Name or the IAM Role Name depending on the storage type.

```go
// AWSStatus containes the status of the credentials request in AWS.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AWSProviderStatus struct {
        metav1.TypeMeta `json:",inline"`
        // User is the name of the User or Role created in AWS for these credentials.
        User string `json:"user"`
        // Policy is the name of the policy attached to the user in AWS.
        Policy string `json:"policy"`
}
```

#### `CredentialRequest` Example (Phase 2)

A `CredentialsRequest` CR like this
```yaml
apiVersion: cloudcredential.openshift.io/v1
kind: CredentialsRequest
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: openshift-image-registry
  namespace: openshift-cloud-credential-operator
spec:
  storage:
    serviceAccount:
      name: image-registry-sa
      namespace: openshift-image-registry
  providerSpec:
    apiVersion: cloudcredential.openshift.io/v1
    kind: AWSProviderSpec
    statementEntries:
    - effect: Allow
      action:
      - "s3:*"
      resource: "*"
```

Would result in the creation of an IAM role named
`sjenning-abcde-openshift-image-registry` with `policy.json`
```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "s3:*"
            ],
            "Resource": "*"
        }
    ]
}
```
and a `trust.json`
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::111122223333:oidc-provider/s3-us-west-1.amazonaws.com/sjenning-abcde-oidc-provider"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "__doc_comment": "scope the role to the service account (optional)",
        "StringEquals": {
          "s3-us-west-1.amazonaws.com/sjenning-abcde-oidc-provider:sub": "system:serviceaccount:openshift-image-registry:image-registry-sa"
        }
      }
    }
  ]
}

```
where `111122223333` is the ARN ID of the OIDC provider.

The controller then creates the `ServiceAccount`
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: image-registry-sa
  namespace: openshift-image-registry
  annotations:
    eks.amazonaws.com/role-arn: "arn:aws:iam::111122223333:role/sjenning-abcde-openshift-image-registry"
```

Any pod started with this `serviceAccountName` will have the token injected via
the pod identity webhook and be able to assume the IAM role annotated in the
`ServiceAccount`.

### Risks and Mitigations

Access to the new functionality is gated by two things: the platform being
`aws` and, once phase 2 is complete, the user specifying a `spec.storage.serviceAccount` in the `CredentialsRequests` CR.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
