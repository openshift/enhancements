---
title: externaldns-operator-aws-assume-role
authors:
  - "@gcs278"
reviewers:
  - "@Miciah"
  - "@candita"
approvers:
  - "@alebedev87"
api-approvers:
  - "None"
creation-date: 2023-09-06
last-updated: 2023-10-30
tracking-link:
  - https://issues.redhat.com/browse/NE-1299
  - https://issues.redhat.com/browse/OCPSTRAT-730
see-also:
  - "/enhancements/installer/aws-cross-account-dns-zone.md"
replaces:
superseded-by:
---

# ExternalDNS Operator AWS Assume Role

## Summary

[ExternalDNS Operator](https://github.com/openshift/external-dns-operator) allows you to deploy and manage [ExternalDNS](https://github.com/kubernetes-sigs/external-dns),
a cluster-internal component which makes Kubernetes resources discoverable through public DNS servers. This enhancement
extends the ExternalDNS Operator to support cross-account DNS zones in AWS by adding a new API field that lets users
specify (assume) an AWS IAM Role ARN to manage DNS records in a different AWS account.

## Motivation

Support for cross-account DNS hosted zones in AWS for OpenShift was introduced in [AWS Shared VPC with Cross-account DNS Zones](/enhancements/installer/aws-cross-account-dns-zone.md).
However, this enhancement only addressed DNS record management in the Ingress Operator and Installer, and did not
include support for optional operators that also handle DNS record creation, such as ExternalDNS Operator. As a result,
this document proposes a feature to add cross-account DNS hosted zone support in AWS to ExternalDNS Operator.

The advantages of enabling cross-account DNS hosted zones is that it enables OpenShift users to benefit from a shared
Virtual Private Cloud (VPC) architecture. Through VPC sharing, users have the ability to share resources, such as Route
53, with other AWS accounts within the same AWS Organization. See [Share your VPC with other accounts](https://docs.aws.amazon.com/vpc/latest/userguide/vpc-sharing.html)
for more information on the benefits of VPC sharing.

To reap the advantages of VPC sharing mentioned earlier, OpenShift currently offers support for sharing VPCs in AWS, as
illustrated in [Installing a cluster on AWS into an existing VPC](https://docs.openshift.com/container-platform/latest/installing/installing_aws/installing-aws-vpc.html).
This enhancement enables ExternalDNS Operator users to access these benefits offered by shared VPCs. By allowing
ExternalDNS Operator users to share Route 53 hosted zones, they can reduce cost and simplify design.

### User Stories

* As an OpenShift cluster admin, I want to configure ExternalDNS Operator to be able to assume another role to manage a
  pre-existing hosted zone.

### Goals

* Allow ExternalDNS Operator users to assume a different role in order to manage DNS records in a pre-existing hosted
  zone that doesn't belong to them.

### Non-Goals

* Support [IRSA](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/aws.md#iam-roles-for-service-accounts),
  [kiam](https://github.com/uswitch/kiam), or [kube2iam](https://github.com/jtblin/kube2iam) as mechanisms for assuming
  a different role.
* Support specifying of [ExternalID](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-user_externalid.html).
* Provide support for the automatic creation of DNS hosted zones by utilizing the assumed role ARN.
* Add AWS Security Token Service (STS) support for the ExternalDNS Operator.

## Proposal

Conveniently, ExternalDNS already supports the [`--aws-assume-role`](https://github.com/openshift/external-dns/blob/fe00b4b83c2263282a9068655e8e3fbbc167b653/docs/faq.md#can-external-dns-manageaddremove-records-in-a-hosted-zone-which-is-setup-in-different-aws-account)
argument, which uses the specified AWS role ARN when creating new DNS records. Therefore, to support cross-account DNS hosted
zones with the ExternalDNS Operator, this enhancement updates the [API](#api-extensions) to expose the `--aws-assume-role` argument for
the ExternalDNS binary.

### Workflow Description

**cluster admin** is a human user who has been granted **Account B IAM User**.

**shared vpc admin** is a human user with super-user-like privileges across multiple accounts.

1. **shared vpc admin** shares a VPC with Account B.
2. **shared vpc admin** creates a DNS hosted zone in Account A and associates it with the shared VPC.
3. **shared vpc admin** creates an IAM policy in Account A granting route53 and tagging permissions to the hosted zone
   (see [IAM Policies and Roles](/enhancements/installer/aws-cross-account-dns-zone.md#iam-policies-and-roles)).
4. **shared vpc admin** creates an IAM role in Account A with a [Trust Policy](/enhancements/installer/aws-cross-account-dns-zone.md#iam-policies-and-roles)
   granting Account B IAM User permission to assume it.
5. **shared vpc admin** attaches the IAM policy created in step 3 to the IAM role created in step 4.
6. **cluster admin** installs the cluster in Account B.
7. **cluster admin** installs ExternalDNS Operator in the new cluster.
8. **cluster admin** specifies the role from step 4 in a new ExternalDNS CR object.

### API Extensions

We extend the`ExternalDNS` API object by adding the struct `AssumeRole` to the `ExternalDNSAWSProviderOptions` struct
(`spec.provider.aws`). Refer to [`externaldns_types.go`](https://github.com/openshift/external-dns-operator/blob/main/api/v1beta1/externaldns_types.go)
for the existing API structure of the `ExternalDNS` object.

The ExternalDNS Operator's validation webhook will use the existing [`IsARN`](https://pkg.go.dev/github.com/aws/aws-sdk-go/service/s3/internal/arn#IsARN)
function to validate the `ARN` field in the `AssumeRole` struct (`spec.provider.aws.assumeRole.arn`). The approach
mentioned above differs from the design documented in the [API: DNS](/enhancements/installer/aws-cross-account-dns-zone.md#API-DNS)
section of the "AWS Shared VPC with Cross-account DNS Zones" enhancement, which employs CRD validation utilizing a regular
expression. Using the existing `IsARN` function provides more flexibility and reliability since it is maintained by AWS
themselves, which helps to mitigate potential issues with edge cases.

Furthermore, we introduce a default value for `spec.provider.aws.credentials.name`, providing OpenShift users the
ability to utilize the `AssumeRole` field without the necessity of specifying a `Credentials` field. For additional
information regarding this update, please refer to [The Required Credentials Issue](#the-required-credentials-issue).

```go
type ExternalDNSAWSProviderOptions struct {
    // Credentials is a reference to a secret containing
    // the following keys (with corresponding values):
    //
    // * aws_access_key_id
    // * aws_secret_access_key
    //
    // See
    // https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/aws.md
    // for more information.
    //
    // +kubebuilder:validation:Required
    // +kubebuilder:default:={"name":""}
    // +required
    Credentials SecretReference `json:"credentials"`

    // assumeRole is a reference to the IAM role that
    // ExternalDNS will be assuming in order to perform
    // any DNS updates.
    //
    // +kubebuilder:validation:Optional
    // +optional
    AssumeRole *ExternalDNSAWSAssumeRoleOptions `json:"assumeRole,omitempty"`
}

type ExternalDNSAWSAssumeRoleOptions struct {
    // arn is an IAM role ARN that the ExternalDNS
    // operator will assume when making DNS updates.
    //
    // +kubebuilder:validation:Required
    // +required
    ARN string `json:"arn,omitempty"`
}

```

### Implementation Details/Notes/Constraints

#### The Required Credentials Issue

Extending the v1beta1 `ExternalDNS` API object revealed a pre-existing issue with the `Credentials` API field in
`ExternalDNSAWSProviderOptions`. The problem is that `Credentials` is designated as `+required` to satisfy vanilla
Kubernetes use cases. On OpenShift, the ExternalDNS Operator generates a `CredentialRequest` automatically, eliminating
the need to manually specify `Credentials`. This wasn't yet exposed as an issue because there were no other fields in
`spec.provider.aws`, and `spec.provider.aws` is optional. The underlying [logic](https://github.com/openshift/external-dns-operator/blob/11e3b72b75d696b7419bce8360183373fc0725e1/api/v1beta1/externaldns_webhook.go#L132)
of the webhook is accurate; the issue lies with this required field in the `ExternalDNS` CRD.

However, there exists a simple workaround: we can apply a default value of an empty string `""` to the
`spec.provider.aws.credentials.name` field using the `+kubebuilder:default:={"name":""}` specifier on the `Credentials`
field. When `spec.provider.aws` is specified, but `spec.provider.aws.credentials` is not specified, the API server will default
`spec.provider.aws.credentials.name` to `""`.

For OpenShift clusters, this default satisfies the `+required` field for `Credentials` while the ExternalDNS Operator
logic [ignores](https://github.com/openshift/external-dns-operator/blob/bac092e98fe5f9065b75bd0fa21d5aeff9d00853/pkg/operator/controller/credentials-secret/controller.go#L259-L267)
the empty string and uses the `CredentialsRequest`.

Non-OpenShift clusters (who do not have the option to use a `CredentialsRequest`) will encounter a validation webhook
failure when the `Credentials` default of `""` is used due to the [validateProviderCredentials](https://github.com/openshift/external-dns-operator/blob/4cb22f19c5e6edcc6a37b73e17f25d996dbef9cd/api/v1beta1/externaldns_webhook.go#L132-L160)
logic. This failure is appropriate in this case because non-OpenShift users are required to specify valid `Credentials`.

In a future version of ExternalDNS Operator's API, we will resolve the requirement for the `Credentials` field through a
more appropriate solution as outlined in the alternative [Bumping the API](#bumping-the-api).

### Risks and Mitigations

#### Security

In terms of security concerns, the IAM role is not considered sensitive because AWS incorporates security measures to
limit the usage of a role ARN to a particular AWS account.

#### Lack of STS Support

Another drawback for this feature is that the ExternalDNS Operator currently lacks support for the AWS Security Token
Service (STS). Given that STS is not supported, and the majority of ROSA (Red Hat OpenShift Service on AWS) customers
rely on STS, this shared VPC feature is not usable for most managed OpenShift users. The addition of STS support for
ExternalDNS Operator is currently under evaluation. However, in the meantime, the decision has been made to move
forward with the implementation of shared VPC support.

### Drawbacks

A minor drawback is the added complexity in supporting cross-account DNS hosted zones for the ExternalDNS Operator. This
complexity involves handling AWS resource sharing, managing IAM policies and roles, and understanding shared VPC
architecture.

## Design Details

### Open Questions

N/A

### Test Plan

We will add E2E tests for the ExternalDNS Operator utilizing two AWS CI accounts to create DNS records in one (Account
A) and install the cluster in the other (Account B). This E2E test will leverage existing steps in the step registry
as created by [AWS Shared VPC with Cross-account DNS Zones](/enhancements/installer/aws-cross-account-dns-zone.md) such
as [ipi-aws-pre-shared-vpc-phz](https://steps.ci.openshift.org/chain/ipi-aws-pre-shared-vpc-phz) and [ipi-aws-post-shared-vpc-phz](https://steps.ci.openshift.org/chain/ipi-aws-post-shared-vpc-phz).
These steps set up a shared VPC cluster with access to a cross-account DNS private hosted zone. However, we may choose to use
a public hosted zone instead, as there is no functional distinction between the two, and it simplifies the E2E testing
process.

### Graduation Criteria

This update is an extension of the existing v1beta1 ExternalDNS Operator API and does not impact the graduation
criteria for the v1beta1 API.

The ExternalDNS Operator updates to support Shared VPC are out of payload and are therefore not aligned with
OpenShift releases. Initially, the plan was to release this enhancement alongside of OCP 4.14. However, because
ExternalDNS Operator lacks STS support (see [Lack of STS Support](#lack-of-sts-support)), the decision was made to
complete the implementation, but deprioritize the release of this feature until there is a need for a new ExternalDNS
Operator release due to other important updates.

There is no requirement for backporting this feature because users have the option to run a newer version of
ExternalDNS Operator, which includes this feature, in an older OpenShift cluster.

#### Dev Preview -> Tech Preview

Updates to the ExternalDNS Operator will be made available without being restricted by a feature gate. These updates
will be considered GA at the time of merging the feature.

#### Tech Preview -> GA

See [Dev Preview -> Tech Preview](#dev-preview---tech-preview).

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

* As outlined in [The Required Credentials Issue](#the-required-credentials-issue), when we bump the API from v1beta1,
  the conversion webhook will be required to handle the changes we make to the `Credentials` field. Nonetheless, in the
  initial implementation of this API, we will have no concern related to upgrading or downgrading.

### Version Skew Strategy

#### ExternalDNS Operator and OpenShift Container Platform Version Skew

The updates to the ExternalDNS Operator are decoupled from the standard release schedule of OpenShift Container
Platform. Consequently, in a shared VPC cluster using cross-account DNS record creation with the Ingress Operator, there
is a possibility of installing a version of the ExternalDNS Operator that lacks support for this same feature. However,
it is important to note that the ExternalDNS Operator and the installer or Ingress Operator do not rely on each other's 
logic. Hence, this mismatch does not pose a concern.

#### ExternalDNS Operator and ExternalDNS Version Skew

This feature adds an API field consumed by the ExternalDNS Operator that sets the `--aws-assume-role` flag in
ExternalDNS. Version discrepancies between the ExternalDNS Operator and ExternalDNS shouldn't be a concern, as the
`--aws-assume-role` flag already is supported in the existing ExternalDNS image we distribute.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

##### Role ARN Permissions Issues

If the role ARN is not configured with the appropriate permissions, ExternalDNS will error and be unable to generate
DNS records. Here is an example of a situation where a user forgot to apply all the needed permissions for their role
ARN:

1. The user applies the following `ExternalDNS` object with the bad role ARN:
```yaml
apiVersion: externaldns.olm.openshift.io/v1beta1
kind: ExternalDNS
metadata:
  name: example-fail
spec:
  provider:
    type: AWS
    aws:
      assumeRole:
        arn: arn:aws:iam::123456789012:role/user-rol1 # This Role lacks adequate permission
  source:
    hostnameAnnotation: Allow
    type: Service
```

2. The user annotates the `router-default` service to instruct ExternalDNS to create a DNS record for it:
```bash
oc annotate service -n openshift-ingress router-default external-dns.alpha.kubernetes.io/hostname=externaldns.$(oc get ingresses.config/cluster -o jsonpath={.spec.domain})
```

3. The user inspects the logs of the `example-fail` ExternalDNS controller and finds error messages related to the lack
   of permission:
```bash
oc logs -n external-dns-operator external-dns-example-fail-6c45fd8bfc-sjdj9
[...]
time="2023-10-30T18:00:41Z" level=info msg="Assuming role: arn:aws:iam::123456789012:role/user-rol1"
time="2023-10-30T18:00:41Z" level=debug msg="Refreshing zones list cache"
time="2023-10-30T18:00:42Z" level=error msg="records retrieval failed: failed to list hosted zones: AccessDenied: User: arn:aws:sts::123456789012:assumed-role/user-rol1/1698688841876440549 is not authorized to perform: route53:ListHostedZones because no identity-based policy allows the route53:ListHostedZones action\n\tstatus code: 403, request id: 73f968e4-8b8d-4aa0-929f-988da0c3e3d1"
```

In this particular case, the role `arn:aws:iam::123456789012:role/user-rol1` is lacking the `route53:ListHostedZones`
permission.

#### Support Procedures

As illustrated in [Failure Mode](#failure-modes), the failures associated with this feature can be identified by
examining the logs of the ExternalDNS controller instance located in the `external-dns-operator` namespace:
```bash
oc logs -n external-dns-operator external-dns-<instance_name>-<id>
```

The logs should be reviewed for permission errors linked to the role ARN specified in `spec.provider.aws.assumeRole.arn`
of the `ExternalDNS` object.

## Implementation History

* PR [openshift/external-dns-operator#195](https://github.com/openshift/external-dns-operator/pull/195) was merged on
  September 18, 2023. This PR contained the initial implementation without E2E tests.
* PR [openshift/release#42894](https://github.com/openshift/release/pull/42894) was merged on September 18, 2023. This
  PR contained the initial CI job implementation.
* PR [openshift/release#43517](https://github.com/openshift/release/pull/43517) was merged on September 20, 2023. This
  PR provided a fix for the `aws-provision-route53-private-hosted-zone` step, addressing a bug in the creation of a
  private hosted zone.
* PR [openshift/external-dns-operator#199](https://github.com/openshift/external-dns-operator/pull/199) was merged on
  October 26, 2023. This PR added a default value to the `spec.provider.aws.credentials.name` field (see
  [API Extensions](#api-extensions)).
* PR [openshift/external-dns-operator#198](https://github.com/openshift/external-dns-operator/pull/198) was merged on
  October 26, 2023. This PR added E2E tests for this feature.
* PR [openshift/release#44311](https://github.com/openshift/release/pull/44311) was merged on October 27, 2023. This PR
  modified the CI job to explicitly execute the shared VPC subset of E2E tests, which were seperated during the code
  review of the E2E tests.

## Alternatives

### Bumping the API

The appropriate fix for addressing the problem outlined in [The Required Credentials Issue](#the-required-credentials-issue)
would be as follows:
1. Update the CRD schema to make the existing `Credentials` field optional.
2. Create a new version of the API that makes the optional field a pointer (assuming `Credentials` unset and `AssumeRole`
   set is a valid combination).
3. Update new API to not allow zero value of `Credentials`.
4. Make the conversion webhook convert new/nil to old/zero-value and old/zero-value to new/nil, and new/zero-value to
   old/zero-value.

However, the concern with bumping the API solely for this update is that the process of bumping can be quite
time-consuming, and it doesn't align the timeline of this enhancement. Additionally, it would be more efficient to
bundle this fix with other API updates, such as the need to rebase for incorporating an upstream fix related to the
Infoblox view parameter (https://github.com/kubernetes-sigs/external-dns/pull/3301) at a later point.

### Assume Role Strategy

Assuming a different AWS role could be implemented without ExternalDNS's built in `--aws-assume-role` as demonstrated in
https://github.com/openshift/external-dns-operator/pull/191 by using kiam, IRSA, or kube2iam. These are alternate
methods for assuming a different AWS role, which require utilizing third-party software in addition to ExternalDNS
(kiam, kube2iam) or the STS support (IRSA: bound service account token). Therefore, this method was not chosen.

### External ID

We considered supporting configuration of an [external ID](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_create_for-user_externalid.html)
for the ExternalDNS Operator when using an assumed role. ExternalDNS provides support for an external ID via the [`--aws-assume-role-external-id`](https://github.com/kubernetes-sigs/external-dns/blob/22da9f231dbc6faa3a668b507a4a06823a129609/pkg/apis/externaldns/types.go#L460)
command line argument. AWS suggests the use of an external ID in specific scenarios to mitigate privilege escalation
such as the [confused deputy problem](https://en.wikipedia.org/wiki/Confused_deputy_problem). However, for this effort,
we opted not to include this functionality at the moment. The reason behind this decision is that there hasn't been a
specific customer need for it thus far, but we should revisit this at a later time.

## Infrastructure Needed

All development and testing of this functionality will require access to two separate AWS accounts.