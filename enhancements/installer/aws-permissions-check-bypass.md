---
title: aws-permissions-check-bypass
authors:
  - "@joelddiaz"
reviewers:
  - "@dgoodwin"
  - "@abhinavdahiya"
  - "@jeremyeder"
approvers:
  - TBD
creation-date: 2020-05-07
last-updated: 2020-05-15
status: provisional
see-also:
  - ""  
replaces:
  - ""
superseded-by:
  - ""
---

# AWS Permissions Check Bypass

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

Running the OpenShift installer in AWS accounts using Service Control Policies
(SCP) can result in errors indicating that the credentials provided do not have
sufficient permissions when in reality they do. This is a limitation with the
AWS IAM permissions simulation API.  Additionally, even without these
limitations it is also true that the AWS policy language is sophisticated enough
to restrict permissions in a way where is is difficult to perform acurate policy
simulations (eg restricting permissions based on VPC or source IP).

## Motivation

Presently, when installing OpenShift on AWS, the credentials provided to the
installer are queried/checked for appropriate permissions to verify that:
1. Can be used to complete an installation (the installer set of required permissions)
1. The in-cluster cloud-credential-operator (CCO) can successfully operate in
   either 'mint' or 'passthrough' mode.
If any of these conditions are not met, the install will fail and explain which
permissions were deemed to be missing.

When an AWS account is configured with Service Control Policies (SCP) the
permissions checking/simulation APIs can provide incorrect results depending on
the contents of the SCPs. Service Control Policies are typically used to deny
certain API calls unless a condition (or set of conditions) is met.  For
example, if user `openshift` has a policy attached that allows `ec2:*`, but the
SCP at the account level denies `ec2:*` unless the API calls are made against
region `us-east-1`, then the `openshift` user would receive errors when making
EC2 API calls outside of region `us-east-1`.

Attempting to validate whether user `openshift` has permissions to perform
`ec2:DescribeInstances` against region `us-east-1` will result in a
determination that the user `openshift` cannot succesfully perform the API call,
even though the actual `ec2:DescribeInstances` call works (as long as it is
against the allowed region).

In order to acomodate installing OpenShift in these environments, a way is
needed for the individual performing the installation to indicate that these
permissions checks should be skipped.

Additionally, a mechanism is needed to indicate to the cloud-credential-operator
to force it into either `mint` or `passthrough` mode, so that it too can avoid
attempting to validatate permissions.

### Goals

Enable successfull installation and operation of OpenShift in these AWS accounts
where the results of permissions simulations cannot be relied upon.

### Non-Goals

Not looking to write complex policy introspection to implement what should
already be performed by the AWS permissions simulation API.

## Proposal

### Installer
Introduce an install-config.yaml field that can be populated by users to
indicate that the installer should not concern itself with determining whether
the credentials provided are sufficient for installation and to affect the
in-cluster behavior of cloud-credential-operator.

Extend the install-config type in the installer repo:
```
type InstallConfig struct {
	// ForceCredentialsMode instructs the installer to not attempt to query
	// the cloud permissions before attempting installation. It also passes
	// down the desired credentials mode to the cloud-credential-operator
	// so that it too does not attempt to query permissions.
	// +optional
	ForceCredentialsMode cloudcredop.CloudCredentialsMode `json:"forceCredentialsMode,omitempty"`
}
```

The installer will then take the value defined in the `forceCredentialsMode`
field to populate the config for the cloud-credential-operator so that CCO knows
what mode it should be forced into.

### cloud-credential-operator
Formalize the constants in cloud-credential-operator repo to export:
```
type CloudCredentialsMode string

const (
	// MintMode is the annotation on the cluster's cloud credentials to
	// indicate that CCO should be creating users for each
	// CredentialsRequest.
	MintMode CloudCredentialsMode = "mint"

	// PassthroughMode is the annotation on the cluster's cloud credentials
	// to indicate that CCO should just copy over the cluster's cloud
	// credentials for each CredentialsRequest.
	PassthroughMode CloudCredentialsMode = "passthrough"
)
```

Introduce a config object to allow modifying the runtime behavior of CCO.

```
type CloudCredentialOperatorConfig struct {
	Spec CloudCredentialOperatorConfigSpec `json:"spec"`
}

type CloudCredentialOperatorConfigSpec struct {
	// ForceCredentialsMode will instruct CCO to skip any permissions
	// checking and assume the designated mode when reconciling
	// CredentialsRequests.
	// +optional
	ForceCredentialsMode CloudCredentialsMode `json:"forceCredentialsMode,omitempty"
}
```

### User Stories [optional]

#### Story 1

A user installing OpenShift in an AWS account subject to SCPs would run the
installer with the `forceCredentialsMode` field set appropriately. This will
generate manifests for CCO containing the CloudCredentialsConfig CR representing
the value in the install-config.yaml.

```
./openshift-install create install-config --dir my-aws-cluster
# edit generated install-config to add the `forceCredentialsMode: "mint"` field
./openshift-install create cluster --dir my-aws-cluster
```

This will cause the installer to skip any pre-flight permissions checks and lay
down the manifest for CCO to indicate that `mint` mode should be assumed:

```
apiVersion: v1
kind: CloudCredentialOperatorConfig
metadata:
  name: cloud-credential-operator-config
  namespace: openshift-cloud-credential-operator
spec:
  forceCredentialsMode: "mint"
```

### Implementation Details/Notes/Constraints [optional]

Bypassing these checks means that errors will be encountered at the moment the
API calls are attempted. For example a user with enough permissions to create
VPCs, Security Groups, and Route53 entries may error when setting up an Elastic
IP. Now depending on the permissions on the credentials, they may not be
sufficient to clean up what was created by the installer up until the failure.

Another error case is that the installer is able to complete its portion of the
bootsrapping, but the CCO and in-cluster AWS API users may fail to come up due
to lack of sufficient permissions provided to the CCO. These will show up as
operators unable to reach their installed=true/progressing=false state.

### Risks and Mitigations

Giving a user a way to avoid the dynamic permissions checking means that users
will need a reliable way to know which permissions are necessary to complete a
successfull installation (both for `mint` and `passthrough` modes). At present,
the list of permissions required for an installation are stored as a static list
of permissions in the installer code, and the permissions needed for `mint` and
`passthrough` mode are stored in the cloud-credential-operator repo. Publishing
and updating these lists should become part of the documentation effort for
OpenShift releases.

In-cluster users of cloud credentials (image-registry, ingress-operator,
machine-api-operator) will be exposed to situations where the credentials that
were requested via the CredentialsRequest CRs have not been validated in any way
when CCO bypasses the permissions verficiation before handing over credentials
to satisfy a CredentialsRequest. This is true for both modes of operator as it
is possible for CCO to have enough permissions to create a user, assign a policy
granting permissions, but the SCP might deny the API calls when they are
attempted, and for passthrough mode CCO is doing nothing more than copying the
contents of secrets around.

## Design Details

### Test Plan

Ideally, e2e coverage of installing OpenShift in an AWS account with SCP
permissions defined in a way that would otherwise fail without bypassing
permissions checking. Acceptably, simply running the installation with the
install-config `forceCredentialsMode` field defined to bypass permissions
checking.

We should also consider the case where OpenShift was installed in an AWS account
without SCPs defined, but the account is then migrated to an environment where
SCPs are now defined post-installation. CCO should be able to recover by an
admin defining the CloudCredentialOperatorConfig CR to force CCO into `mint` or
`passthrough` mode as appropriate. CCO will eventually settle into a functioning
state (assuming the credentials have sufficient permissions).

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

N/A. A running cluster whose AWS account is migrated to an organization that
does define SCPs will start to error.

During cluster runtime, enabling/disabling the bypassing of permissions
simulations can be controlled through the contents of the
CloudCredentialOperatorConfig CR.

### Version Skew Strategy

## Implementation History

## Drawbacks

Moving away from pre-flight permissions checks pushes out the time for when
someone attempting to install OpenShift will get feedback on failure.  The
pre-flight checks have not exposed OpenShift to needing to bubble up appropriate
information when certain types of cloud API errors are encountered.

## Alternatives

Working with AWS to enhance the permissions simulation API to cover these
complex permissions situations. 

Take the installer out of needing to worry about generating a manifest for CCO
and just allow the person installing the cluster to provide their own
CloudCredentialOperatorConfig manifests before starting the cluster creation
phase.

The proposed implementation makes global cross-cloud changes to the
install-config and the proposed changes only address AWS. It would be possible
to put the `forceCredentialsMode` field into a platform-specific section, and
the CloudCredentialOperatorConfig CRD could be modified to have
platform-specific overides as well.

## Infrastructure Needed [optional]

(For testing and for any ongoing e2e) A pair of AWS accounts where the root
account has the ability to set/modify SCP polcies, and a second child account to
be subject to the policies defined in the SCP.
