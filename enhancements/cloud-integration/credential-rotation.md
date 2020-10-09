---
title: cloud-credential-rotation
authors:
  - "@dgoodwin"
reviewers:
  - "@joelddiaz"
  - "@abhinavdahiya"
  - "@jsafrane"
  - "@dmage"
  - "@enxebre"
  - "@MaysaMacedo"
approvers:
  - "@sdodson"
  - "@derekwaynecarr"
creation-date: 2020-10-06
last-updated: 2020-10-08
status: provisional
---

# Cloud Credential Rotation

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Implement automatic credentials rotation in the cloud-credential-operator.
Allow users to configure a global expiry duration for their cloud credentials config,
after which the CCO will rotate new credentials in, and cleanup the old after a
suitable period of time.

## Motivation

Customers often have security policies that prohibit the use of long lived
cloud credentials. This enhancement provides a solution to a frequently
requested feature, will improve OpenShift security overall, and help meet the
requirements of more customers and thus generate more sales.

### Goals

  1. Support a global expiry time for CredentialsRequest credentials, defined in the CloudCredential config CR.
  1. Rotate expired credentials when expiry time has been reached.
  1. Preserve previous credentials for a period of time to ensure operators remain fully functional.
  1. All operators using CredentialsRequests able to successfully handle a Secret modification with new credentials without going degraded.

### Non-Goals

  1. Mint mode only, not applicable to Passthrough or Manual mode. (although the operator support for credentials changing will be universally useful in both of these modes for customers manually rotating their own credentials)
  1. AWS STS Token support. This is separate from this effort and not applicable to all mint capable cloud providers.

## Proposal

### User Stories

#### Story 1

As a cluster administrator, I would like to be able to define a credentials
expiration policy and have my OpenShift CredentailsRequests rotated
automatically.

#### Story 2

As a cluster administrator, I would like OpenShift operators to gracefully
detect new credentials and not go degraded during the transition.

### Implementation Details/Notes/Constraints [optional]

This implementation is only relevant for cloud actuators that support mint
mode. (presently AWS, Azure, and GCP) Expiration makes no sense for Passthrough
or Manual mode, where users can control credentials rotation themselves.

#### API Changes

Add CloudCredential.Spec.ExpireHours (int64) to represent the global setting
for how long credentials should live before being rotated. If this is specified
when the operator is *not* in mint mode, this should be reported as a Degraded
state. It is assumed that hours is the most reasonable granularity for
rotation.

Add CredentialsRequest.Status.MintTime timestamp to store the last time
credentials were minted. If this is unset and we are in mint mode, MintTime
will be assumed to be the target Secret CreationTimestamp. This path will be
used when ExpireHours if first enabled on a cluster.

Internally we will hardcode a GradePeriod that the old credentials should be kept functionalfor to help operators avoid degraded state. (10 minutes)

#### Controller Flow

Controllers will be updated to compare ExpireHours to MintTime and determine if
we're due for rotation. They should requeue CredentialsRequests with the
appropriate delay if not yet expired.

The rotation flow aims to record state at each step of the process and then
re-reconcile, allowing us to recover from failures at one state and re-try
without unexpected behaviour.

The flow for each reconcile loop will be as follows:

  1. If MintTime beyond ExpireHours, copy the current Secret.Data keys containing the credentials to key.old and write to etcd. the .old keys are recorded to ensure we can cleanup the old credentials after sufficient time has passed. Re-reconcile the CredentialsRequest.
  1. If MintTime beyond ExpireHours, and there are already .old keys with the same values as the normal keys, we should now mint new cloud provider credentials and store in the appropriate original Secret.Data keys. Add an additional Secret.Data.MineTime key for an atomic write of when we performed this operation.
    1. As with CCO minting today, if this Secret write fails, we have lost the credentials and a new set will be minted on next attempt. See risks and mitigations below.
  1. If Secret.Data.MintTime does not match the CredentialsRequest.Status.MintTime, copy it, write to etcd, and re-reconcile.
  1. If target secret has .old keys that do not match the normal keys, but we are not yet beyond MintTime + GracePeriod, re-reconcile for the grace period.
  1. If we reconcile and see a target secret with .old keys, and we are beyond MintTime + GracePeriod, deprovision the old credentials. Remove .old keys from the Secret and save. If the save fails, next reconcile will see the credentials already do not exist and should tolerate this, and attempt the save again.

#### Impact to Operators Using CredentialsRequests

All operators using CCO CredentialsRequests will need to ensure they are capable of smoothly handling an update to their credentials Secret without going degraded. The operator will ensure both the new and old credentials are operational for a short window of time.

This should have been the case all along, but has never been thoroughly tested or confirmed. This feature will expose any problems in this regard and thus we need to coordinate with each operator team to make sure they are ready for rotation. Doing so will also assist with manual credentials rotation that customers may try to make use of in passthrough or manual modes.

If the operator loads credentials from etcd whenever they are used, the operator will require no changes.

If the operator mounts the credentials Secret into a pod, and reads the mounted file contents whenver they are used, the operator will require no changes. However if the file is read only once when the operator starts, changes will be required. (either read when needed, or watch the Secret/file for changes and restart the pod) Note that when a secret is updated, that change will be reflected in the pod's filesystem, but it often takes a minute before this happens. Old credentials will be kept alive for a grace period per the above design.

If the operator sets environment variables for the credentials Secret, it may require changes. (unsure what the Kubernetes behavior is here)

### Risks and Mitigations

There are transactional concerns with this type of rotation where we're interacting with an external system, and may fail to record state in etcd after minting credentials. In theory this could result in leaked credentials we were never able to record and thus cannot clean up. However in this case the secret access key is likely lost in the ether as it was never stored anywhere, and thus has limited risk. This problem technically exists today even when minting a new credential, and has since the CCO was created. Cloud provider expiration could be used as a fallback to identify and clean up anything that slipped through the cracks.

Because each actuator has it's own Secret.Data key(s), and we're outside the current actuator interface Delete call, this enhancement will have to be implemented one cloud provider at a time. However once one is completed the others should fall into place quickly.

This automatic rotation *does not* apply to the top level admin credential in kube-system. Users wishing to rotate this credential can do so manually, but we do not create this credential and we do not ever try to delete it.

## Design Details

### Open Questions

None at this time.

### Test Plan

Testing will be via unit tests, and QE test plans.

Expiration times and rotation are too likely to flake in OpenShift e2e. Also we are presently targetting a min setting of hours. Using minutes would be difficult to test rotation without catching a followup rotation, and with the 10 minute window where both sets of creds should remain we'd be looking at a very long test.

QE test plan will effectively enable this setting with a low ExpiryHours, wait for this time to elapse, and verify that new credentials were minted, the old are cleaned up in the cloud provider, and the relevant operators never went degraded and are functioning with the new creds.

### Upgrade / Downgrade Strategy

Not applicable. This is new functionality and opt in.

It will be possible to enable this functionality after upgrading a cluster to an appropriate version simply by setting the new CloudCredential.Spec.ExpireHours.

Downgrade would not cause issues but naturally expiry would stop occurring.

### Version Skew Strategy

Not applicable, no version skew issues are expected.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

  * 2020-10-09: First draft of enhancement PR opened.

## Drawbacks

There is some overlap with the [AWS Pod Identity](./aws/aws-pod-identity.md)
feature being discussed for a future release. However Product Management has
confirmed that this rotation is much more desirable for more customers and thus
is a higher priority in the near term. It also spans all mint capable cloud
actuators and thus provides a better return with much less effort.

## Alternatives

A much simpler implementation could be completed quickly across all cloud
providers with mint support. In this solution we would just delete the current
credentials on expiry and re-reconcile to let the controllers replace them.
This avoids all the work around rotating the old out and the new in, and does
not require specific work for each actuator. However this approach means that
for a brief period of time the operators would have no working credentials, and
if something went wrong in the minting of new this state could persist for some
time. As such this approach is presently ruled out in favor of the more
complicated solution above, which will keep the operators functional.

