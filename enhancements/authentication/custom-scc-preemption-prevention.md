---
title: custom-scc-preemption-prevention
authors:
  - "@s-urbaniak"
reviewers:
  - "@stlaz"
  - "@ibihim"
approvers:
  - "@deads2k"
api-approvers:
  - "None"
creation-date: 2023-05-03
last-updated: 2023-05-03
tracking-link:
  - https://issues.redhat.com/browse/AUTH-132
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Preventing preemption of system SCCs by custom SCCs for core OpenShift components

## Summary

OpenShift ships SCCs by default that applies both to core OpenShift workloads
as well as to user provided workloads.

Users have the possibility to add custom SCCs to OpenShift.
These can impact core OpenShift workloads and, worst case, break out of the box components.

This enhancement proposal suggests a mechanism to be able to protect core OpenShift
out of the box workloads from side effects induced by custom SCCs.

## Motivation

In many cases the default out of the box SCCs are sufficient as they cover the most
common patterns for controlling permissions pods can have.
Additionally, the most recent enablement of Pod Security Admission also covers
an additional layer of protection next to OpenShift SCCs.

However, OpensShift historically does allow cluster administrators to add
additional custom SCCs on top of the default out of the box SCCs.
Custom SCCs are not considered different from the perspective of
the SCC admission plugin.

When a pod is being admitted, the admission plugin lists all SCCs available in the system, including all custom SCCs.
The list of SCCs is then sorted by:
- SCC priority in descending order
- SCCs with the same priority: SCC restriction score in descending order (most restrictive first, least restrictive last)
- SCCs with the same restriction score: lexical ordering of the SCC name in descending order

An SCC is considered applicable if the requesting user or the
pod's service account has `use` permissions against the `security.openshift.io.securitycontextconstraints` resource
for an SCC in the requested namespace.
If an SCC is applicable, the pod's manifest is potentially mutated according to the SCC logic.

One can see that custom SCCs can easily overrule existing out of the box SCCs
if the pod's service account or the requesting user has sufficient privileges to use any of them.

In that case, if a custom SCC has higher priority, is more restrictive, or it's name is lexically ordered before an out of the box SCC,
any OpenShift out of the box core pod will be mutated according to the custom SCC specified behavior.

This can lead to unknown mutations of core OpenShift workloads.
Worst case it can render OpenShift core workloads dysfunctional
if i.e. a custom SCC is more restrictive than a previously applicable less restrictive out of the box SCC that was applicable to that workload.

This proposal includes two suggested protection modes, a preferred "hard protection" and a "soft protection" variant
which are described below.

We need a mechanism to protect OpenShift core workloads such that they are not
subject to pod mutation due to user provided custom SCCs.

### User Stories

1. As a developer I want to make sure that my shipped workload
is guaranteed to be associated with an out of the box SCC.

2. As a developer I want to make sure that my shipped workload
is not preempted by custom user provided SCCs.

### Goals

1. Protect core OpenShift workloads from being preempted by user provided custom SCCs.

2. Be able to pin core OpenShift workloads to a concrete out of the box SCC.

3. Make this change generally available for end users.

### Non-Goals

## Proposal

### "hard protection" - Requesting a required SCC

This proposal suggests to implement a mechanism which enables OpenShift core workloads to enforce
an SCC to be pinned to a workload.
This is accomplished by setting an annotation named `openshift.io/required-scc`
whose value contains the name of the SCC that is supposed to be pinned.

Admission will fail if the `openshift.io/required-scc is being changed or added
in the pod specification directly in the case of update events.
Note that changing the annotation in the pod specification template in the underlying
deployment or daemonset manifest is valid as these are causing new pods to be created.

If the requested SCC does not exist in the system, admission fails with an error.

If the requested SCC does exist, any other SCC available in the system is ignored
and the requested SCC is the only one being considered for admission.

If the SCC is applicable to the workload, it gets/is used,
else admission fails with an error.

Pros:
- The required and pinned SCC is deterministic.
- No other SCC in the system is taken into account.

Cons:
- If the required SCC is not applicable, the workload fails to be created/admitted.
This can be detected in the existing e2e suite.
- All core OpenShift workloads need to set that annotation in their deployment artifacts
as part of the pod template specification.
This implies that, after the change merges, many repositories have to be touched.

An existing implementation is available at https://github.com/openshift/apiserver-library-go/pull/90.

### Workflow Description

Opting into this feature for core OpenShift workloads is straight-forward,
with just an annotation needed to be set in the deployment manifest.

### Variation (optional)

Another variation for both proposed suggestions is to accept a comma-separated list of SCCs.
This could be helpful in case additional SCCs are going to be introduced and the deployment manifest
needs to be compatible with both old and new applicable SCCs.

However, as adding a list further adds complexity, this suggestion is being omitted as an variation only
with the potential of being introduced as a backwards-compatible change in the future.

### API Extensions

A new annotation `openshift.io/required-scc` is introduced.

### Implementation Details/Notes/Constraints [optional]

N/A

### Risks and Mitigations

1. OpenShift workloads that are not applicable to the requested SCC might fail to be admitted.
Mitigation: The existing e2e test suite in OpenShift will give very early signals of failed admission attempts
as the workload will be unschedulable.

### Drawbacks

This proposal adds yet another additional complexity on top of the existing SCC logic
which also already interleaves with pod security admission.
The mitigation here is to reduce complexity by favoring the "hard protection" variant.

## Design Details

### Open Questions [optional]

N/A

### Test Plan

The existing OpenShift e2e test suite is assumed to be sufficient to verify correctness.
Further, an additional e2e test will be implemented that verifies that a more restrictive and highest priority
custom SCC will not preempt a required SCC for selected workloads.
If workloads opt into the annotation and force an SCC that is not applicable, admission will fail immediately.

### Graduation Criteria

The agreed consensus is to promote this proposal immediately to GA.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The added annotation to OpenShift is ignored during downgrades.
An upgrade strategy is not needed.

When workloads are rescheduled during upgrades, admission will apply the newly added annotation.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

Workloads that want to opt into custom SCC preemption prevention need to opt in
by setting the annotation on their pod manifests (be it deployments or other forms).

This implies that enabling this feature can potentially take more than one release
if the teams owning that workload don't have the possibility to add that annotation short-term.

#### Failure Modes

A wrong required SCC annotation that picks an SCC that is not applicable can cause the workload be rejected by the SCC admission plugin.

#### Support Procedures

The rejection reason of workloads can be retrieved from audit logs, generated by the SCC admission plugin.

## Implementation History

N/A

## Alternatives

### "soft protection" - Hinting a favored SCC

Another suggested approach is to use hinting instead.
Here, an annotation named `openshift.io/scc-hint` contains the hinted SCC name as its value.

If the hinted SCC does not exist in the system, and audit entry with the reason why it was not chosen will be generated.
In this case SCC admission continues with all remaining SCCs available in the system as described above.

If the hinted SCC does exist in the system it is being prepended to the sorted list describe above,
before the highest priority SCC.

This ensures that the hinted SCC is deterministically considered to be applied first
before any other SCC available in the system.

Pros:
- Less hard failure modes than the proposed "hard protection" mechanism.
- Less intrusive than the "hard protection" alternative
as the workload will continue to be asserted against the remaining SCCs available in the system.

Cons:
- If the hinted SCC is not applicable to the workload,
admission continues with asserting all remaining SCCs available in the system as described above.
- If the hinted SCC is not applicable, admission continues with asserting all the other SCCs available in the system,
potentially even custom SCCs which still can cause problems.
- All core OpenShift workloads need to set that annotation in their deployment artifacts.
This implies that, after the change merges, many repositories have to be touched.
- To verify that hinted SCCs are really being applied, an e2e test has to be written.
This can be done centrally.

An existing implementation is available at https://github.com/openshift/apiserver-library-go/pull/105.

This proposal favors the "hard protection" variant and continues to describe that one further on.

## Infrastructure Needed [optional]

N/A