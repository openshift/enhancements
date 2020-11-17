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
creation-date: 2020-11-13
last-updated: 2020-11-13
status: implementable
see-also:
  - "/enhancements/cloud-integration/aws/aws-pod-identity.md"
---

# AWS STS Token Support

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add support for using time limited AWS Security Token Service credentials in OpenShift Installation and all core CVO managed operators. This enhancement will avoid the need to use long lived traditional AWS access key ID / secret access key credentials at any point during the lifecycle of the cluster. Support will require manual setup, but once complete tokens will be automatically rotated and made available to affected OpenShift operators.

## Motivation

Better align with AWS best practices in using short, time limited credentials within our operators.

### Goals

List the specific goals of the proposal. How will we know that this has succeeded?

  * OpenShift can be installed with an STS credential.
  * All core CVO managed OpenShift operators using cloud credentials (approximately 5) can now use STS tokens rotated on a regular basis.
  * OpenShift can be installed without requiring, generating or using any long lived credentials. (aws_access_key_id / aws_secret_access_key)

### Non-Goals

  * Changing the default install method on AWS. We will continue to use long lived credentials by default to preserve the simplest path to success possible. In future, if we proceed to later phases of STS support where CCO automates the setup fully, we may be able to make this the default.
  * Leveraging the Cloud Credential Operator to automate this setup required for using STS.
  * Upgrading pre-existing clusters to use STS. For now this is for new installs only.

## Proposal

### Implementation Details/Notes/Constraints

#### Cloud Credential Operator Changes

The Cloud Credential Operator will be modified to include a new `credentials` Secret key in the target secrets for each `CredentialsRequest`. This `credentials` Secret key will contain a standard AWS credentials file:

```
[default]
aws_secret_access_key = REDACTED
aws_access_key_id = REDACTED
```

All pre-existing Secrets will be updated to include this new `credentials` key, and this change will be backported to aid in the upgrade to 4.7, where operators may run before the CCO would have had a chance to add the new key.

The CCO will be modified to set Upgradeable=False for Manual mode users who have not yet updated their Secrets to contain a `credentials` key. This change will need to be backported to 4.6 as well. Alternatively, we could backport a change for these users where CCO automatically generates this new field and includes it, as we have the access key ID and secret access key already in those Secrets, however this may interfere with how manual mode users are deploying/reconciling their Secrets. As such it is probably safest to require them to do it themselves.

#### Operator Changes

All CVO managed operators (~5) that use CredentialsRequests will be updated to use the new `credentials` Secret key. Because this file can also contain a reference to a file with an AWS STS token, operators will then be able to build AWS clients without worrying about what type of authentication they are using.

All CVO managed operators will also now mount in a [Kubernetes Projected Service Account Token](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#service-account-token-volume-projection) to a consistent file location on disk. (/var/run/secrets/openshift/serviceaccount/token)

```yaml
      - name: bound-sa-token
        projected:
          defaultMode: 420
          sources:
          - serviceAccountToken:
              audience: openshift
              expirationSeconds: 3600
              path: token
```

openshift-install will be modified to accept user provided service account signing keys and configure the Kubernetes APIServer to use them.

For each `CredentialsRequest` the user will need to manually create the relevant Secret and role
Users will have to manually provision the IAM roles, policies, and Secrets for each `CredentialsRequest`. This process will also require the name of the `ServiceAccount` that the pod consuming the credentials uses, which is typically created by the operator itself. However, we do not presently have the `ServiceAccount` name modeled in the `CredentialsRequest` API, and would like to defer doing this until we have more real world data and experience before making a permanent API commitment. As such, credentials will be documented as a list policy permissions, service account names, and target secrets, which the user will have to iterate:

  1. Create an IAM Role with no policy attached, tagged with `kubernetes.io/cluster/[infraid]=owned` so it can be cleaned up on deprovision. (type = web identity, identity provider created by the admin manually)
  1. Attach an in-line policy to the role which matches the permissions in the `CredentialsRequest` or documentation.
  1. Ensure the trust relationship for the Role points to the cluster’s AWS IAM OIDC ARN with conditions to limit which `ServiceAccount` can assume this role.
     ```json
{
	"Version": "2012-10-17",
		"Statement": [
		{
			"Effect": "Allow",
			"Principal": {
				"Federated": "<OIDC_IDENTITY_PROVIDER_ARN>"
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": {
				"StringEquals": {
					"<OIDC_PROVIDER_URL_without_https://>:sub": "system:serviceaccount:<CredReq spec.Namespace>:<CredReq .spec.ServiceAccountName>"
				}
			}
		}
		]
}
```
  1. Create a secret containing a valid AWS credentials file
     ```yaml
apiVersion: v1
stringData:
  credentials: |-
    [default]
    role_arn=[ AWS Role ARN created for CredentialsRequest ]
    web_identity_token_file=/var/run/cloudcredentials/token
kind: Secret
metadata:
  name: [ CredReq .spec.SecretName ]
  namespace: [ CredReq .spec.Namespace ]
type: Opaque
```

Cluster install workflow becomes:

  1. Create an S3 bucket to publish OIDC keys.
  1. Generate and publish OIDC keys.
  1. Create an AWS IAM identity provider.
  1. Configure their local AWS credentials to use STS.
  1. `openshift-install create install-config`
  1. Provide their OIDC keys in a manifest, which will be used to configure the Kube APIServer to be used to sign bound service account keys so they are trusted by AWS.
  1. Modify `spec.credentialsMode` in the generated install-config.yaml to set CCO to manual mode. (this mechanism already exists)
  1. Generate secret yaml containing an AWS credentials file which configures the client to use STS, and references to token file mounted in. (see above)
  1. Include secret yaml definitions generated in the steps above.
  1. `openshift-install create cluster`

The OpenShift cluster will now install without ever using a long live credential. Kubernetes will handle the rotation of the tokens via the projected service account, which are then trusted by AWS via the OIDC / identity provider trust relationship established by the user manually before installing the cluster.

A [tool has been written](https://github.com/sjenning/sts-preflight) that may be available to help users automate some of the initial AWS setup.

Documentation will be provided on this install process, as well as rotating the OIDC signing keys.

#### Future Phases

In the future we may explore automating the OIDC setup and credentials secrets with CCO, instead of placing it in manual mode. However we are unsure if this will proceed and thus will leave this for a future enhancement or addition to this document.

The `CredentialsRequest` API should eventually be updated to include a reference to the ServiceAccount to configure in the policy, so this information no longer needs to be duplicated and maintained in documentation.


### Risks and Mitigations

This implementation does not use the AWS pod identity webhook we began deploying in 4.6, meaning our approach differs from EKS.

Non-CVO managed third-party or OLM operators may be using `CredentialsRequests`. As per any use of CCO manual mode, these requests will not be fulfilled automatically. They will have the flexibility to do this with STS, or traditional credentials, depending on what the operator supports.

## Design Details

### Test Plan

It is likely infeasible to automate testing around a manual CCO mode with non-trivial setup steps. For the most part we will need to rely on manual testing by engineering and QE test plans. Future phases where we may automate more of this could change this.

### Upgrade / Downgrade Strategy

Because this feature relies on CCO in manual mode, we will continue using existing mechanisms in CCO for setting upgradeable=false when we want to alert the user that they need to take action before attempting an upgrade. We presently support setting upgradeable false if a new credential has been added in the next minor release, but we do not support checking for modified permissions. This will likely be handled as a separate feature someday for both STS and traditional credentials.

