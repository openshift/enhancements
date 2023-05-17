---
title: scc-run-as-any
authors:
  - "@alanconway"
reviewers:
  - "@eparis"
  - "@bparees"
  - "@jcantrill"
approvers:
  - "@eparis"
api-approvers:
  - None
creation-date: 2023-05-17
last-updated:
tracking-link:
  - https://issues.redhat.com/browse/OCPBUGS-13375
---

# Consistent handling for SecurityContextConstraint RunAsAny strategies

## Summary

SecurityContextConstraints serve two distinct functions:

1. They _constrain_ Pod configurations that are allowed to run.
2. They _modify_ Pod configurations with values from outside the Pod based on strategies in the SCC.

From [the docs][scc]:
> The admission controller is aware of certain conditions in the security context constraints (SCCs) that trigger it to look up pre-allocated values from a namespace and populate the SC before processing the pod.

SCC strategies are ordered by "restrictiveness": if a Pod runs under SCC X, then it MUST run under any SCC that is _less restrictive_ than X.
This ordering is critical to correctly matching SCCs to Pods, and must be clear an consistent.

The `RunAsAny` strategy is documented as being the _least restrictive_ `runAsUser` strategy.
In other words, a Pod that runs under some other `runAsUser` strategy must also run under `RunAsAny`

This is not currently the case for pods with `runAsNonRoot:true`. They run correctly under `MustRunAsRange` but fail under `RunAsAny`.
If a Pod has no `runAsUser` and no image `User` setting then:
- Under `MustRunAsRange`, SCC processing "helps" by providing a default non-root UID from a predefined range.
- Under `RunAsAny`, SCC processing does not help, and assumes the Docker default of 0. The Pod is rejected by its own security context.

The proposal: For `runAsNonRoot:true` pods, `RunAsAny` should use the same defaulting rules as `MustRunAsRange` to provide a non-0 UID.

[scc]: https://docs.openshift.com/container-platform/4.12/authentication/managing-security-context-constraints.html

## Motivation

Major customer outrage, caused in this instance by the Cluster Logging Operator Pod failing.

- [Upstart Case 03508387 Review](https://docs.google.com/document/d/1AlVZHsWqykMELnJmE7Ei-wYCDAPEA966zPZnVDnkHIQ/edit)
- [OCPBUGS-13375 Logging Operator 5.7.0 fails with "CreateContainerConfigError" when there are certain custom SCCs in the cluster](https://issues.redhat.com/browse/OCPBUGS-13375)

Note the operator has been fixed to resolve the customer problem, but that does not resolve the larger problem that adding a custom SCC can break a cluster.

The operator does not express any preference for UID except that it not be root.
It has been running correctly in its current configuration for ages because it matched a standard SCC with `MustRunAsRange`.

`MustRunAsRange` does not simply constrain the UID, it _provides a valid default_ `runAsUser` field, if there isn't one otherwise specified.

To be strictly "less restrictive" `RunAsAny`, also needs to provide a valid runAsUser by default. 0 is a valid and sensible default with `runAsNonRoot:false`, but it is INVALID with `runAsNonRoot:true`.

The customer (correctly) assumed that a custom SCC with `RunAsAny` was safe because it is less restrictive.
Any pods already working in their cluster should continue to work.

This was not their experience.

**Note 1** A `runAsNonRoot` Pod _should_ have an explicit `runAsUser` or set the `User` property in the image. That is not the point of this proposal.
The point is that SCC processing must be consistent. We can't say "your pods are wrong" when a customer adds a _less restrictive_ SCC and Pods that used to work suddenly fail.
The user cannot control the match between SCC and Pod (that is the whole point of an SCC), so its up to us to ensure that the definition of "less restrictive" rigorously matches Pod behaviour.
Otherwise there is no safe way to use custom SCCs.

**Note 2:** If `MustRunAsRange` was strictly a constraint (i.e. failed if the Pod did not have an explicit UID or range matching the pre-allocated range) then this problem would not have arisen.
The logging operator would have failed during development and would have been updated with explicit UID configuration.
We can't do it both ways - either all strategies provide reasonable defaults, or none do.
At this point adding a default is better than taking one away.

### User Stories

Adding an SCC to my cluster should never break Pods that were working before.
If I follow the documented rules about what is more or less "restrictive", I expect that:
- Any pods match the new SCC (because it is more restrictive and/or higher-priority than the old one) should run as they did before.
- Any pods that run with an existing SCC, but can't run with the new SCC _should not be a match for its restrictions_

### Goals

Avoid unpleasant failures when customers try to write custom SCCs.

### Non-Goals

## Proposal

Change SCC `RunAsAny` behaviour as follows:
- No change with `runAsNonRoot: false`
- No change with Pods that explicitly select a non-0 UID (runAsUser, image properties etc.).
- No change with Pod that _explicitly_ select user 0 (runAsUser:0, User:"0" etc.) \
  (Such pods will fail with `runAsNonRoot:true`, but the reason is obvious from inspecting the Pod, and is independent of the SCCs available.)
- Otherwise, with `runAsNonRoot:true`, use the same defaulting rules as `MustRunAsRange` to pick a non-0 UID.

### Risks and Mitigations

Changes meaning of `RunAsAny` when combined with `runAsNonRoot: true` ONLY when there is no other configuration indicating the UID.

- The change provides a valid default for the case that currently fails.
- There is no change for any cases that currently work correctly.
- The default rules are the same as existing MustRunAsRange, already documented and requires no new configuration.

### Drawbacks
None I can see.

## Design Details

### Test Plan

Test the known issue, regression testing.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

No user visible changes except to resolve the current problem.
Should not have any other impact.

### Version Skew Strategy
### Operational Aspects of API Extensions
#### Failure Modes
#### Support Procedures
## Implementation History
## Alternatives
