---
title: Cluster Profiles
authors:
  - "@csrwng"
reviewers:
  - "@abhinavdahiya"
  - "@crawford"
  - "@sdodson"
  - "@derekwaynecarr"
  - "@smarterclayton"
approvers:
  - "@derekwaynecarr"
  - "@smarterclayton"
creation-date: 2020-02-04
last-updated: 2019-02-04
status: implementable
see-also:
replaces:
superseded-by:
---

# cluster-profiles

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Cluster profiles are a way to support different deployment models for OpenShift clusters. 
A profile is an identifier that the Cluster Version Operator uses to determine
which manifests to apply. Operators can be excluded completely or can have different
manifests for each supported profile.

## Motivation

To support different a deployment model in which not all operators rendered by
the CVO by default are needed.  Every usage of a Cluster Profile MUST have a corresponding
enhancement describing its usage.  

The initial target for this enhancement is to improve the IBM Cloud managed service offering
to support alternative manifests for clusters that do not self-host the control plane.  Other
scenarios may use this solution (such as Code Ready Containers) pending future enhancements.

Alternative manfiests for any OpenShift release component must link to an enhancement that
defines the profile type and intended usage.

### Goals

- Equip the CVO to include/exclude manifests based on a matching identifier (profile)

### Non-Goals

- Define which manifests should be excluded/included for each use case

## Proposal

### User Stories

#### Story 1
As a user, I can create a cluster in which manifests for control plane operators are
not applied by the CVO.

#### Story 2
As a user, I can create a cluster in which node selectors for certain operators target
worker nodes instead of master nodes.

### Design

A cluster profile is specified to the CVO as an identifier in an environment
variable. For a given cluster, only one CVO profile may be in effect.

NOTE: The mechanism by which the environment variable is set on the CVO deployment is 
out of the scope of this design.

```
CLUSTER_PROFILE=[identifier]
```
This environment variable would have to be specified in the CVO deployment. When
no `CLUSTER_PROFILE=[identifier]` variable is specified, the `default` cluster profile
is in effect.

The following annotation may be used to include manifests for a given profile:

```
include.release.openshift.io/[identifier]=true
```
This would make the CVO render this manifest only when `CLUSTER_PROFILE=[identifier]`
has been specified. 

Manifests may support inclusion in multiple profiles by including as many of these annotations
as needed.

For items such as node selectors that need to vary based on a profile, different manifests
will need to be created to support each variation in the node selector. This feature will
not support including/excluding sections of a manifest. In order to avoid drift and 
maintenance burden, components may use a templating solution such as kustomize to generate
the required manifests while keeping a single master source.

## Alternatives

A potential use for this enhancement would be to include/exclude certain operators based on
the hardware platform, as in the case of the baremetal operator.  However, the general problem 
with this use for profiles is that it expands the scope of the templating the payload provides. 
The current profile proposes exactly 2 variants decided at install time, and they impact 
how the core operator works as well as how SLOs are deployed or included. 
If we expanded this to include operators that are determined by infrastructure, then we're 
potentially introducing a new variable (not just a new profile), since we very well may want 
to deploy bare metal operator in a hypershift mode.
