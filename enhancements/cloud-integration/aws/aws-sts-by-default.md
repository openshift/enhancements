---
title: aws-sts-support
authors:
  - "@dgoodwin"
  - "@joelddiaz"
reviewers:
  - "@joelddiaz"
  - "@sjenning"
  - "@derekwaynecarr"
  - "@sdodson"
  - "@abhinavdahiya"
approvers:
  - "@derekwaynecarr"
creation-date: 2021-02-01
last-updated: 2021-02-01
status: implementable
see-also:
  - "/enhancements/cloud-integration/aws/aws-sts-support.md"
---

# AWS STS As Default

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Start having OpenShift installations on AWS default to using STS for cloud
credentials. This gets a cluster to rely on a web of trust involving AWS and
the cluster's OIDC provider.
There will be pre-installaion steps expected from users to prepare an AWS account to run in this mode. Upgrades will involve steps external to the cluster when new cloud Roles/permissions are required, or when existing Roles/permissions need changes.


## Motivation

Better align with AWS best practices in using short, time limited credentials within our operators.

### Goals

List the specific goals of the proposal. How will we know that this has succeeded?

* OpenShift can be run with short-lived credentials.
* OpenShift can be installed without requiring, generating or using any long lived credentials. (aws_access_key_id / aws_secret_access_key). Only an IAM Role (with sufficient permissions to complete an installation) and the ability to AssumeRole() into that Role.
* There will no longer be a Secret containing highly-privileged credentials in kube-system/aws-creds.

### Non-Goals

* Extending the OpenShift installer to create the pre-requisite AWS resources.
* Upgrading pre-existing clusters to use STS. For now this is for new installs only.

## Proposal

Leverage the capabilities for OpenShift to run on AWS with temporary credentials (via STS), and make it the default mode of installation for AWS. This involves significant changes to the current installation workflow where the installer is provided powerful-enough creds so that the cluster can create an IAM User for each component requiring AWS API access.

A user would be expected to create a number of AWS and OpenShift installer resources before launching the installation.

### Cloud resources
To allow a cluster to be installed for use with temporary credentials, a few AWS resources need to exist.

#### AWS IAM Identity Provider
An AWS IAM Identity Provider of type OpenID Connect that points to an OIDC config URL that matches the ServiceAccount signing keys that will be handed to the installer/cluster.
```
[jdiaz@minigoomba sts-preflight (master %=)]$ aws iam get-open-id-connect-provider --open-id-connect-provider-arn arn:aws:iam::125931421481:oidc-provider/s3.us-east-1.amazonaws.com/jdiaz-stsfull-installer
{
    "Url": "s3.us-east-1.amazonaws.com/jdiaz-stsfull-installer",
    "ClientIDList": [
        "openshift"
    ],
    "ThumbprintList": [
        "a9d53002e97e00e043244f3d170d6f4c414104fd"
    ],
    "CreateDate": "2021-01-06T15:54:11.150Z"
}
```
This IAM Identity Provider does not need to be unique to the cluster, as a pre-existing Identity Provider can be used and shared across clusters.

#### HTTPS endpoint with OpenID configuration
An HTTPS endpoint (typically an S3 bucket) containing the OpenID Connect configuration that lists the key(s) the cluster will use to sign ServiceAccounts.
```
[jdiaz@minigoomba sts-preflight (master %=)]$ aws s3api list-objects-v2 --bucket jdiaz-stsfull-installer
{
    "Contents": [
        {
            "Key": ".well-known/openid-configuration",
            "LastModified": "2021-01-06T15:54:11.000Z",
            "ETag": "\"27646b660203fa53f0e2802602d88957\"",
            "Size": 469,
            "StorageClass": "STANDARD"
        },
        {
            "Key": "keys.json",
            "LastModified": "2021-01-06T15:54:11.000Z",
            "ETag": "\"bbd0a94535f87f977c2eaf0a995dbeab\"",
            "Size": 917,
            "StorageClass": "STANDARD"
        }
    ]
}
```
There is a one-to-one relationship between the AWS IAM Identity Provider and the HTTPS OpenID configuration endpoint, so sharing AWS IAM Identity Providers across clusters will also share the HTTPS OpenID configuration.

#### IAM Role(s)
One IAM Role for each in-cluster component granting the permissions as defined in each component's CredentialsRequest (from the manifest)
```
[jdiaz@minigoomba sts-preflight (master %=)]$ aws iam list-roles | grep RoleName | grep jdiaz-stsfull
            "RoleName": "jdiaz-stsfull-installer",  <--- Optional: only used to perform the install, no in-cluster user of this Role
            "RoleName": "jdiaz-stsfull-openshift-cloud-credential-operator-cloud-credenti",
            "RoleName": "jdiaz-stsfull-openshift-cluster-csi-drivers-ebs-cloud-credential",
            "RoleName": "jdiaz-stsfull-openshift-image-registry-installer-cloud-credentia",
            "RoleName": "jdiaz-stsfull-openshift-ingress-operator-cloud-credentials",
            "RoleName": "jdiaz-stsfull-openshift-machine-api-aws-cloud-credentials",
```

### Installer manifests

Before launching the installer, several manifests need to be prepared for the installer.

#### Kube Secrets with AWS config
Kube Secrets need to be defined for each in-cluster component requiring cloud credentials.
```
[jdiaz@minigoomba _output (master %=)]$ ls manifests/*cred*
manifests/openshift-cloud-credential-operator-cloud-credential-operator-iam-ro-creds-credentials.yaml
manifests/openshift-cloud-credential-operator-cloud-credential-operator-s3-creds-credentials.yaml
manifests/openshift-cluster-csi-drivers-ebs-cloud-credentials-credentials.yaml
manifests/openshift-image-registry-installer-cloud-credentials-credentials.yaml
manifests/openshift-ingress-operator-cloud-credentials-credentials.yaml
manifests/openshift-machine-api-aws-cloud-credentials-credentials.yaml
```

These Secrets will hold an AWS config file that references the IAM Role that the component will AssumeRole() into when making AWS API calls. The Name/Namespace for each Secret is defined by each component's CredentialsRequest CR in their respective manifest payloads.
```
[jdiaz@minigoomba _output (master %=)]$ cat manifests/openshift-machine-api-aws-cloud-credentials-credentials.yaml 
apiVersion: v1
stringData:
  credentials: |-
    [default]
    role_arn = arn:aws:iam::123456789012:role/jdiaz-stsfull-openshift-machine-api-aws-cloud-credentials
    web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
kind: Secret
metadata:
  name: aws-cloud-credentials
  namespace: openshift-machine-api
type: Opaque
```

#### Custom Authentication CR
A custom authentication.config.openshift.io CR with the `.spec.serviceAccountIssuer` field set to match the OpenID Connect configuration URL.
```
[jdiaz@minigoomba _output (master %=)]$ cat manifests/cluster-authentication-02-config.yaml 
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  name: cluster
spec:
  serviceAccountIssuer: https://s3.us-east-1.amazonaws.com/jdiaz-stsfull-installer # Sme URL that the IAM Identity Provider has been configured with
```

#### ServiceAccount signing key
The private key side of the public key published to the OIDC config URL that the cluster will use to sign ServiceAccounts.
```
[jdiaz@minigoomba _output (master %=)]$ cat tls/bound-service-account-signing-key.key 
-----BEGIN RSA PRIVATE KEY-----
PRIVATE KEY DATA HERE
-----END RSA PRIVATE KEY-----
```

### Tooling

While a user is free to create these objects independently, tooling will exist to simplify creating and updating these objects.

#### AWS Resource generation

Extend the `oc` CLI to allow fetching a release image and generating templates of the various AWS resources that need to exist prior to installing/upgrading the cluster. `oc adm release extract --cloud=aws --iam-roles $RELEASE_IMAGE` will be used to extract the CredentialsRequests objects from the release image to generate templates of the IAM Roles that need to be created for each in-cluster component needing to make AWS API calls.

```
{
    "Path": "/",
    "RoleName": "openshift-machine-api-aws-cloud-credentials",
    "AssumeRolePolicyDocument": {
            "Version": "2012-10-17",
            "Statement": [
                {
                    "Effect": "Allow",
                    "Principal": {
                        "Federated": "INSERT ARN OF AWS IAM IDENTITY PROVIDER HERE"
                    },
                    "Action": "sts:AssumeRoleWithWebIdentity",
                    "Condition": {
                        "StringEquals": {
                            "INSERT OPENID URL HERE:sub": "system:serviceaccount:openshift-machine-api:machine-api-controllers"
                        }
                    }
                }
            ]
    },
    "Description": "OpenShift IAM Role for openshift-machine-api-aws-cloud-credentials",
    "MaxSessionDuration": 0,
    "PermissionsBoundary": "",
    "Tags": [
        {
            "Key": "",
            "Value": ""
        }
    ]
}
```

And the Policy that needs to be attached to the IAM Role.
```
{
    "RoleName": "openshift-machine-api-aws-cloud-credentials",
    "PolicyName": "openshift-machine-api-aws-cloud-credentials",
    "PolicyDocument": {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Action": [
                    "ec2:CreateTags",
                    "ec2:DescribeAvailabilityZones",
                    "ec2:DescribeDhcpOptions",
                    "ec2:DescribeImages",
                    "ec2:DescribeInstances",
	...
                 ],
             }
         ],
         "Resource": "*"
     }
}
```

Once the fields are appropriately filled out, the objects can be created in AWS to satisfy the need for the AWS IAM Roles to exist (or be updated) prior to installation/upgrade.

#### Additional tooling

A new tool will be created that can perform the pre-installation/upgrade tasks that are necessary when installing/upgrading a cluster running in STS mode.

##### OpenID Connect setup
Tooling will be created/modified (from developer-only tooling found at https://github.com/sjenning/sts-preflight) upload an OpenID Connect (OIDC) configuration to an S3 bucket, and creating an AWS IAM Identity Provider that trusts identities from the OIDC provider (it will be able to take a pre-existing public/private key pair or generate one from scratch if needed).

A command like `tool-name-tbd create identity-provider --use-public-key ./path/to/public/key` can be used to create an AWS IAM Identity Provider, an S3 bucket holding an OpenID configuration tree using a pre-generated public/private key pair.

##### AWS IAM Role creation and updating
Tooling will be created to take a list of CredentialsRequests (from a release image) and create/update IAM Roles with appropriate permissions as needed. This can be used before installation, or to prepare IAM Roles as needed before a cluster upgrade.

First fetch the CredentialsRequests `oc adm release extract --cloud=aws --credentials-requests --to=./credrequests/` followed by processing of the files with `tool-name-tbd create iam-roles --credentials-requests ./path/to/credrequests --aws-iam-id-provider-arn=ARN_OF_AWS_IAM_IDENTITY_PROVIDER --openid-config-url=URL_WITH_OPENID_CONFIGURATION`.

This will create/update an IAM Role which restricts the Role to only the appropriate OpenShift ServiceAccounts, and a policy/permissions list as defined in the CredentialsRequest `.spec`.

##### OpenShift installer manifests generation
Tooling will be created to take the ServiceAccount private signing key, the list of CredentialsRequests for a given OpenShift release image, and the OpenID Connect configuration URL to generate the objects that need to be passed to the installer.

`tool-name-tbd create installer-manifests --credentials-requests ./path/to/credrequests --openid-config-url=URL_WITH_OPENID_CONFIGURATION --private-key-file ./path/to/service/account/private/key` will be used to generate the Secrets, the Authentication.config.openshift.io CR (with issuerURL set), and the ServiceAccount signing key file needed before launching an installation.

The Secret manifests generated can be used to create any necessary Secrets before a cluster upgrade if the version being upgraded to includes new components calling the AWS API.

## User Stories

### User who wants to create AWS resources themselves

### User who wants to review AWS resources before creation

### User who wants tooling to create/update AWS resources

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

## Design Details

### Open Questions [optional]

### Test Plan

#### CI Account Setup

  1. Create a single shared AWS IAM OIDC provider in CI account. Make sure this and related resources are skipped from pruning.
  1. Create IAM roles for all the operators using the OIDC provider. Make sure these roles are also skipped from pruning (these will need to be updated whenever a new component needing cloud API accces is introduced to the product, or whenever changes in permissions are made to existing components).

#### Add Another Multi-Step Workflow

  1. Add a ipi-conf step to set the credentialMode: Manual
  1. Store the Bound Service Account Singning key in CI cluster as secret.
  1. Use CI steps to inject this credential to one of the step see https://docs.ci.openshift.org/docs/architecture/step-registry/#injecting-custom-credentials
  1. Create a step that adds following items to the $SHARED_DIR/manifests such that the install step will add those files before running create cluster.
    * manifests/cluster-authentication-02-config.yaml (static)
    * tls/bound-service-account-signing-key.key (from the credential)
    * manifests/secret-{*for each operator*} (static) (new Secret manifests whenever a new cluster component requires cloud API access)
    * see https://github.com/openshift/release/blob/ed755e202de5d04dbeb72ec91670f13e9a722022/ci-operator/step-registry/ipi/install/install/ipi-install-install-commands.sh#L45-L49
  1. This should test OpenShift clusters components using STS for AWS authentication and running the same e2e.


### Graduation Criteria

### Upgrade / Downgrade Strategy

Because the cluster credentials/permissions are controlled external to the cluster, the procedure for upgrading a cluster changes significantly. The expectation from a user running a cluster in Manual mode is that before a (non-z-stream) upgrade, they should fetch the CredentialsRequest manifests from the release they will be upgrading to and ensure that any new/changed permission requirements are made to the IAM Users/Roles as needed. Once the release-to-be-upgraded-to's CredentialsRequests have been reviewed/reconciled the user signals that they are ready for upgrade by annotating the cloud-credential-operator's CR (`cloudcredential.operator.openshift.io`).

This is a significant departure from the `oc adm upgrade` workflow that exists when the cluster is in non-Manual credentials mode. CCO will mark itself Upgradeable=False when set to Manual mode, and will clear it when the annotation is seen.

### Version Skew Strategy

## Implementation History

## Drawbacks

### Cluster in "Manual" credentials mode

Because the in-cluster components (specificaly the cloud-credential-operator) cannot create the necessary AWS IAM resources, the cluster must be placed in "Manual" credentials mode (accomplished by setting `credentialsMode: Manual` in the `install-config.yaml`). This changes the default operating mode of the cluster:
* All credentials/permissions configuration is performed outside of the cluster
* Misconfiguration needs to be repaired outside of the cluster

### OpenShift Dedicated (OSD)



## Alternatives

## Infrastructure Needed [optional]
