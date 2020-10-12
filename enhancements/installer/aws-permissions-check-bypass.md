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

## Open Questions

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
against the allowed region). This false negative result occurs because AWS
blocks calls to their global endpoints (eg aws.amazonaws.com), and IAM has no
region endpoints so the permissions simulation calls (which are IAM calls) fail.

A false positive can also manifest. An SCP policy that denies
`ec2:DescribeInstances` to any user named `openshift` can be defined. In this
case the validation will return a result indicating that the call is allowed,
but when the `openshift` user tries to make the call, it ultimately is denied.

In the first example, the installation would be halted even though the install
could have succeeded. In the second, the install would be allowed to proceed and
fail after pre-flight checks.

In order to accommodate installing OpenShift in these environments, a way is
needed for the individual performing the installation to indicate that these
permissions checks should be skipped.

Additionally, a mechanism is needed to indicate to the cloud-credential-operator
to force it into either `mint` or `passthrough` mode, so that it too can avoid
attempting to validatate permissions.

While adding the new mechanism to allow specifying `mint` or `passthrough`,
extend the idea to allow indicating that CCO should be in the disabled/manual
mode for the disconnected VPC case (where the IAM API is unavailable) or when
the user simply does not want CCO to be processing CredentialsRequests (the user
will provide credentials manually).  This will allow deprecating the current
process of creating a ConfigMap to put CCO in the disabled/manual mode.

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
type credentialsMode string

const (
	credentialsModeMint credentialsMode = "mint"
	credentialsModePassthrough credentialsMode = "passthrough"
	credentialsModeManual credentialsMode = "manual"
)

type InstallConfig struct {
	// CredentialsMode is used to explicitly set the mode with which
	// CredentialsRequests are satisfied.
	//
	// If this field is set, then the installer will not attemp to query the
	// cloud permissions before attempting installation. If the field is not
	// set or empty, then the installer will perform its normal verification
	// that the credentials provided are sufficient to perform an
	// installation.
	//
	// There are three possible values for this field, but the valid values
	// are dependent upon the platform being used.
	// "mint": create new credentials with a subset of the overall
	// permissions for each CredentialsRequest
	// "passthrough": copy the credentials with all of the overall
	// permissions for each CredentialsRequest
	// "manual": CredentialsRequests must be handles manually by the user.
	//
	// For each of the following platforms, the field can be set to the
	// specified values. For all other platforms, the field must not be set.
	// AWS: "mint", "passthrough", "manual"
	// Azure: "mint", "passthrough"
	// GCP: "mint", "passthrough"
	// +optional
	CredentialsMode credentialsMode `json:"credentialsMode,omitempty"`
}
```

The installer will make available the user's install-config as a ConfigMap that
the cloud-credential-operator can then use to affect CCO runtime behavior.

### cloud-credential-operator
Formalize the constants in cloud-credential-operator repo to define the
acceptable credentials (matching the definitions in the installer):
```
type CredentialsMode string

const (
	// ModeMint indicates that CCO should be creating users for each
	// CredentialsRequest.
	ModeMint CredentialsMode = "mint"

	// ModePassthrough indicates that CCO should just copy over the cluster's cloud
	// credentials for each CredentialsRequest.
	ModePassthrough CredentialsMode = "passthrough"

	// ModeManual indicates that CCO should not process CredentialsRequests.
	// This results in CCO not creating the Secrets to satisfy a
	// CredentialsRequest. CCO will simply calculate metrics and report
	// those, but a CredentialsRequest will never move to
	// status.provisioned: True.
	// Disconnected VPCs (where the IAM endpoints are not available) can use
	// this to allow a user to pre-provision credentials and place them in
	// the install-time manifests so that the cluster installation can succeed.
	// This can also be used if a user does not want powerful credentials to
	// be left residing in the cluster.
	ModeManual CredentialsMode = "manual"
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
	ForceCredentialsMode CredentialsMode `json:"forceCredentialsMode,omitempty"

	// NOTE: also migrate existing fields in the CCO configmap used to
	// disable CCO into this new config object.
}

Since the CCO runs as a bootstrap Pod, the process for rendering the CCO pieces
will use the contents of the install-config ConfigMap to build the runtime
configuration to ensure CCO runs in the desired mode.
```

### User Stories

#### Story 1

A user installing OpenShift in an AWS account subject to SCPs would run the
installer with the `forceCredentialsMode` field set appropriately. This will
generate manifests for CCO containing the CloudCredentialsConfig CR representing
the value in the install-config.yaml.

```
./openshift-install create install-config --dir my-aws-cluster
# edit generated install-config to add the `credentialsMode: "mint"` field
./openshift-install create cluster --dir my-aws-cluster
```

This will cause the installer to skip any pre-flight permissions checks and lay
down the manifest for CCO to indicate that `mint` mode should be assumed:

```
apiVersion: v1
kind: CloudCredentialOperatorConfig
metadata:
  name: cluster
  namespace: openshift-cloud-credential-operator
spec:
  forceCredentialsMode: "mint"
```

### Implementation Details/Notes/Constraints

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

None

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

## Infrastructure Needed

(For testing and for any ongoing e2e) A pair of AWS accounts where the root
account has the ability to set/modify SCP polcies, and a second child account to
be subject to the policies defined in the SCP.
