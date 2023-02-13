---
title: detection-and-reporting-of-stale-clusteroperators
authors:
  - "@mfojtik"
reviewers:
  - "@deads2k"
approvers:
  - "@tkashem"
creation-date: 2022-01-31
last-updated: 2023-02-13
---

# Detection and reporting of stale cluster operators

## Summary

In OpenShift 4, every active operator included in the core payload (cluster operator) updates its conditions in "
clusteroperator" resource. Conditions determine whether specific core components are Degraded or Available or if they
are making progress or if their state causes the cluster to be not upgradeable. These conditions are essential for
cluster administrators to determine the health of their clusters, and they are usually the first thing Red Hat support
is going to look at in case there is a problem with the cluster.

Unfortunately, in some edge cases, these conditions might not reflect the current state of the cluster. For example,
since they depend on the running operator, these conditions might become stale and outdated if the operator is not
running or cannot contact the API server. And while the API server might still work, the result
of `oc get clusteroperators` does not represent the cluster's current state.

Therefore, we need a detection mechanism to help detect in-active operators or operators that are not actively updating
the conditions.

## Motivation

In the past, there were multiple cases when either kubelet or CRI-O failed to run pods, either by KCM malfunctioning or
some other system error. However, the static pods succeeded in running, making the Kubernetes API server available and
accessible with `oc`. When support or engineers ran `oc get clusteroperators` against this cluster, they saw all
operators reporting available, non-degraded and upgradeable status, but the truth was that none of the operators that
provided these conditions were running.

### Goals

* Detect inactive cluster operators
* Provide more accurate condition messages and status

### Non-Goals

* Make a new operator that detects other operators status
* Create a heart-beat mechanism

## Proposal

To implement stale operator detection, we need a controller process that will run close to the Kubernetes API server,
preferably using a local connection and client-based certificate for authentication to avoid dependencies on working
auth and network when the cluster is unstable.

The controller will watch the clusteroperator resource and check the `status.[]conditions.lastTransitionTime` field for
Degraded, Available, Progressing, and Upgradeable conditions. Suppose the "lastTransitionTime" field is not updated for
10 minutes. In that case, the controller will change the human-readable condition message
to: `Operator checking for stale status, the active operator will reset this message: <original message>`.

After the message is changed, as it suggests, the active operators should reset this message back to the original
message, removing the "checking" part. This shows a healthy, active operator.

Suppose the operator is not active and does not reset the message back to the original after 10 minutes. In that case,
the controller will change the message again
to: `Operator has not updated this condition for more than 20 minutes, last known condition state was "<False/True>", original message: <original message>`
. In addition to changing the message, this time, the controller will also change the condition status from False or
True to **Unknown**.

Since the clusteroperator resource has multiple conditions in status, if **any** of these conditions are updated, we assume
the operator is still active and able to reflect the operand status into clusteroperator resource.

For cluster administrators, this means the operator is having a problem and is potentially down or unable to make
updates to conditions. In that case, an alert might be required to trigger a manual investigation of the current
operator state.

### API Extensions

*No API extensions required*

### Risks and Mitigations

* One risk here is that we find operators that are not correctly reconciling their clusteroperator conditions, which this controller makes obvious. These operators should be fixed as a result.
* This also creates (2 writes) * (number of cluster operators) every N minutes

## Design Details

### Test Plan

* This feature should be pared with an e2e invariant that ensures a minimum update time for the stale message. Let's take a guess at 30s to start and see how many fail.

### Operational Aspects of API Extensions

No API extension is needed to implement this proposal.

#### Failure Modes

#### Support Procedures

## Implementation History

## Drawbacks

## Alternatives

One alternative discussed was a new API field that will be added to clusteroperators.Status called "lastUpdateTime".
Instead of relying on
"lastTransitionTime" will introduce this field and use it as "heart-beat". This will require **all** operators to update
this field correctly and multiply the writes significantly.

Another alternative is to use lease inspection based on explicitly named leases in the .status.relatedResources for each
clusteroperator.
This alternative resolves the additional heartbeat problem and doesn't require a logic change since operators already 
provide .status.relatedResources.
Every operator would need updating to include their lease and the staleness checker navigates through and inspects leases.
This is preferred, but takes significantly longer to produce value.
The tradeoff of flapping message and 4.13 value versus not-flapping message and value in a nebulous future release is not
clean or obvious: staff-eng holds the keys, but will heavily weight the kube-apiserver team recommendation.