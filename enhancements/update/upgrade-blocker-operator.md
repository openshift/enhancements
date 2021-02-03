---
title: upgrade-blocker-operator
authors:
  - "@michaelguigno"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-02-03
last-updated: 2021-02-03
status: implementable
---

# Upgrade Blocker Operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add an extensible API and associated operator to block upgrades.

## Motivation

Sometimes clusters shouldn't be upgraded.  Currently, the only signal is
ClusterOperator statuses monitored by the CVO.  Cluster administrators need
to be able to optionally inform the cluster when upgrades aren't appropriate.

### Goals

* Define a new user-facing CRD and namespace to be consume by the new operator
* Create a new operator to signal whether or not upgrades can commence
* Define an override mechanism to allow upgrades anyway

### Non-Goals

* Limit the scope of what information might be used to block an upgrade
* Changes to the CVO other than respecting the new cluster operator

## Proposal

### User Stories

#### Story 1

As a cluster administrator, I need a pluggable system to inform the CVO when
upgrades/automatic upgrades should be prevented.  Data I use to make this
determination may originate inside or outside of the cluster, depending on
my organization's needs.

### Risks and Mitigations

#### Yet another operator

We could build some of this logic into the CVO directly instead of another
cluster operator.

#### People will want to block things other than upgrades

This is not addressed here.

## Design Details

### New CRD

A new CRD is created.  One or more instances (CRs) are to be created in a
system-specific namespace.  The CR should capture the who/what/why, as well
as severity.

The CRD should have a field in spec "preventUpgrades" with a boolean
value to clearly indicate if a given CR is block an upgrade or not.

Administrators can add an annotation to or adjust a different field in the spec
of object to 'silence' the block.

### New ClusterOperator

The new operator should watch/list the CRs in the specific namespace.  If any
objects in the namespace have "preventUpgrades: True" and a silence annotation
has not been applied, set ClusterOperator status upgradeable=False.

### New Controllers

End users can create controllers to add/remove/update CRs in the specific
namespace.  These controllers what for whatever conditions they choose, and
optionally add/remove/update the CRs to reflect their intent to block or
not block upgrades.

### Manually Add CRs

Cluster administrators don't need controllers to add/remove/update the CRs,
they can use whatever process they deem fit.  Manually creating or modifying
a CR would


### Open Questions [optional]

Figure out exact fields, namespace, and names of components.

Should OpenShift ship new operators/CRs by default or as add-on packages for
some well defined situations, or should only users consume this capability
and OpenShift component developers are expected to utilize current
ClusterOperator statuses to solve the same problem?

Should we try to implement some sort of system in the CRD to specify what type
of upgrades we are trying to prevent?  EG, Z is unblocked, Y is blocked.

Is this an API we should try to coordinate with upstream on, or do ourselves
downstream first?

## Implementation History

This is a new component.

## Drawbacks

Literally none.

## Alternatives

Teach CVO about metrics.  Teach CVO about something other than ClusterOperators
generally.
