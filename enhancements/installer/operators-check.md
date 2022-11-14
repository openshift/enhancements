---
title: cluster-operator-installation-check
authors:
  - @patrickdillon
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @wking
  - @LalatenduMohanty
approvers:
  - @sdodson
  - @deads2k
  - @bparees
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2021-07-14
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - none
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - n/a
superseded-by:
  - n/a
---

# Cluster Operator Installation Check

## Summary

This enhancement proposes that the OpenShift Installer should check the status
of individual cluster operators and use those statuses as a threshold for 
determining whether an install is a success (exit 0) or failure (exit != 0).
Specifically, the Installer should check whether cluster operators have stopped
progressing for a certain amount of time. If the Installer sees that an operator
is Available=true but fails to enter a stable Progressing=false state, the Installer
will exit non-zero and log an error.

## Motivation

This enhancement allows the Installer to identify and catch a class of errors
which are currently going unchecked. Without this enhancement, the Installer can
(and does) declare clusters successfully installed when the cluster has failing components.
Adding a check for cluster operator stability will allow normal users and managed services teams
to more easily identify and troubleshoot issues when they encounter this class of failure. The change
will also help the Technical Release Team identify this class of problem in CI and triage
issues to the appropriate operator development team.

### User Stories

As an openshift-install user, I want the Installer to tell me when an operator never stops
progressing so that I can check whether the operator has an issue.

As a member of the technical release team, I want the Installer to exit non-zero when
an operator never stops progressing so that I can triage operator bugs.

### Goals

* Installer correctly identifies a failed cluster install where cluster operators are not stable
* Installer exits non-zero in those situations and delivers meaningful error message

### Non-Goals

* Installer handling other classes of errors, such as failure to provision machines by MAO

## Proposal

### Workflow Description

Cluster admin will begin installing cluster as usual. The Installation workflow will be:

1. Cluster initializes as usual
2. As usual, installer checks that cluster version is Available=true Progressing=False Degraded=False 
3. Installer checks status of each cluster operator for stability
4. If a cluster operator is not stable the installer logs a message and throws an error

### API Extensions

None

### Implementation Details/Notes/Constraints [optional]

The important detail is how we define _stable_. The definition proposed here is:

> If a cluster operator does not maintain Progressing=False for at least 30 seconds,
> during a five minute period it is unstable.

### Risks and Mitigations

One risk would be a false positive: the Installer identifies that a cluster
operator is unstable, but it turns out the operator is perfectly healthy;
the install was declared a failure but was actually successful. This risk
seems low and a risk that could be managed by monitoring these specific failures
in CI.

### Drawbacks

This enhancement has some significant potential drawbacks in terms of design:

A fundamental design principle of 4.x and the operator framework has been
delegating responsibility away from the Installer to allow clean separation
of concerns and better maintainbility. Without this enhancement, it is the
responsibility of the Cluster-Version Operator to determine whether given
Cluster Operators constitute a successful version. The idea of keeping the
cluster-version operator as the single responsible party is discussed in the
alternatives section.

Following the previous point, this enhancement could introduce a slippery slope.
Where before this enhancement, the Installer would simply query the CVO; now we
are adding additional conditional logic. Introducing this conditional logic leads
to the risk of further conditions, which could make the install process more brittle
and introduce the potential for false positives or other failures.

Does implementing this enhancement address symptoms of issues with operator status definitions?
Should operators themselves be setting Degraded=True when they don't meet this stability criterion?

As we have seen with other timeouts in the Installer, developers and users will want to change these.
We should define a process for refining our stability criteria. 

## Design Details

### Open Questions [optional]

1. What is the correct definition of a stable operator? More importantly, how can we
refine this definition?
2. Should this logic belong in the Installer, CVO, or another component?

### Test Plan

This code would go directly into the installer and be exercised by e2e tests.

### Graduation Criteria



#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The cluster-version operator can institute similar logic and trigger alerts when
operator stability is in question during an upgrade. See alternatives section.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

- The worst case failure more is that the Installer throws an error when there is not an actual
problem with a cluster. In this case, an admin would need to investigate an error or automation would
need to rerun an install. We would hope to eliminate this failures through monitoring CI.

#### Support Procedures

We should instruct users on how to debug or review their operators when this error occurs.

## Implementation History

N/A 

## Alternatives

We should seriously consider whether this logic should go into the cluster-version
operator rather than the installer. (Or if it should be in both, perhaps just the CVO
for upgrades.) Management of Cluster Operators has been the responsibility of the CVO
and the Installer should defer responsibility where possible to maintain separation
of concerns.
