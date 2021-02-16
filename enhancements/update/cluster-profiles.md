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
last-updated: 2020-08-05
status: implementable
see-also:
  - "/enhancements/update/ibm-public-cloud-support.md"
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

#### Cluster profile annotation

The following annotation may be used to include manifests for a given profile:

```text
include.release.openshift.io/[identifier]=true
```

Manifests may support inclusion in multiple profiles by including as many of these annotations
as needed.

For items such as node selectors that need to vary based on a profile, different manifests
will need to be created to support each variation in the node selector. This feature will
not support including/excluding sections of a manifest. In order to avoid drift and
maintenance burden, components may use a templating solution such as kustomize to generate
the required manifests while keeping a single master source.

The current installation profile is called `self-managed-high-availability`. All current
manifests must specify it. Future profiles may choose.

#### Design Details

A cluster profile is specified to the CVO as an identifier an environment variable.

```text
CLUSTER_PROFILE=[identifier]
```

For a given cluster, only one cluster profile may be in effect.

The profile is a set-once property and cannot be changed once the cluster has started.

The cluster-version-operator picks the value for the cluster profile according to this order:
* if it is defined and not empty, the environment variable,
* otherwise, the default profile `self-managed-high-availability`.

Clusters in a version unaware of the cluster profile must upgrade to the `self-managed-high-availability` profile.

When upgrading, outgoing CVO will forward the cluster profile information to the incoming CVO with the environment variable.

`include.release.openshift.io/[identifier]=true` would make the CVO render this manifest only when `CLUSTER_PROFILE=[identifier]`
has been specified.

##### Usage

###### Without the installer

IBM Cloud and others platforms that are managing their own deployment of the CVO should pass the env. variable.
For instance, IBM Cloud already uses `EXCLUDE_MANIFESTS` env. variable. Cluster profile will be set like this env. variable.

Upgrade will have to preserve the initial cluster profile.

###### With the installer

This method will be used by CodeReady Containers and single-node production edge clusters.

Users must set `OPENSHIFT_INSTALL_EXPERIMENTAL_CLUSTER_PROFILE` env. variable in their shell before running the installer if they want to use non-default profile.

Example:
```console
$ OPENSHIFT_INSTALL_EXPERIMENTAL_CLUSTER_PROFILE=single-node-developer openshift-install create manifest
$ OPENSHIFT_INSTALL_EXPERIMENTAL_CLUSTER_PROFILE=single-node-developer openshift-install create cluster
```

##### Release phases

The `CLUSTER_PROFILE` env. variable will break the upgrade as this new template variable in CVO manifests is not known by outgoing CVO.
This requires to deploy cluster profile in 2 phases.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: bootstrap-cluster-version-operator
  namespace: openshift-cluster-version
...
    env:
      - name: CLUSTER_PROFILE
        value: "{{.ClusterProfile}}"
```

###### Phase 1

* Add the cluster profile `ClusterProfile` in the [`manifestRenderConfig`](https://github.com/openshift/cluster-version-operator/blob/b59561c40240d2a52048923b1b94ed7385cab957/pkg/payload/render.go#L104) object used to render all manifests (esp. CVO manifests).
* Read the env. variable and select manifests with the right include property `include.release.openshift.io/[identifier]=true`.
  It will default to `self-managed-high-availability` in this phase.

This will probably need to be backported.

###### Phase 2

* Add the cluster profile env. variable in CVO manifests,
* Operators can add manifests for the non-default profile.

## Implementation History

* Teach cluster-version operator about profiles, [cvo#404](https://github.com/openshift/cluster-version-operator/pull/404), in flight.

## Alternatives

A potential use for this enhancement would be to include/exclude certain operators based on
the hardware platform, as in the case of the baremetal operator.  However, the general problem
with this use for profiles is that it expands the scope of the templating the payload provides.
The current profile proposes exactly 2 variants decided at install time, and they impact
how the core operator works as well as how SLOs are deployed or included.
If we expanded this to include operators that are determined by infrastructure, then we're
potentially introducing a new variable (not just a new profile), since we very well may want
to deploy bare metal operator in a hypershift mode.
