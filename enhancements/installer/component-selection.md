---
title: component-selection-during-install
authors:
  - "@bparees"
reviewers:
  - "@decarr"
  - "@staebler"
approvers:
  - "@decarr"
creation-date: 2021-05-04
last-updated: 2021-05-04
status: provisional
---

# User Selectable Install Solutions

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes a mechanism for cluster installers to exclude one or more optional components for
their installation which will determine which payload components are/are not installed in their cluster.
Core components are defined as the set of Second Level Operators managed by the Cluster Version Operator
which today cannot be disabled until after completing the install and editing a CVO override, or editing
the CVO overrides as part of rendering+editing manifests.

The proposed UX is to make this a first class part of the install config api with the implementation
being arguments supplied to the CVO to filter the user-selected manifests.

## Motivation

There is an increasing desire to move away from "one size fits all" cluster installations, and
towards flexibility about what should/should not exist in a new cluster out of the box.  This can
be seen in efforts such as hypershift, single node, and code-ready-containers.  Each of these
efforts has done some amount of one-off work to enable their requirements.  This EP proposes a
mechanism that allows components to be disabled in a first class way that the installer exposes.

### Goals

* Users can easily explicitly exclude specific "optional" components from their cluster, at install time.

### Non-Goals

* Making control-plane critical components optional (k8s apiserver, openshift apiserver, openshift controller,
  networking, etc)
* Defining which components should be disable-able (this will be up to component teams to classify themselves
as `addons` or not)


## Proposal

### User Stories

* As a user creating a new cluster that will be managed programmatically, I do not want the additional
security exposure and resource overhead of running the web console.  I would like a way to install
a cluster that has no console out of the box, rather than having to disable it post-install or
modify rendered manifests in a way that requires deep understanding of the OCP components/resources.

* As a team scaffolding a managed service based on openshift, I want to minimize the footprint of my
clusters to the components I need for the service.

* As a user creating a cluster that will never run an image registry, I do not want the additional overhead
of running the image registry operator, or have to remove the default registry that is created.


### Implementation Details/Notes/Constraints [optional]

The CVO already has the ability to respect annotations on resources, as can be seen
[here](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c03c9edf5fddf4e3fb1bc6d7afcd2a2284ca03d8/manifests/0000_20_kube-apiserver-operator_06_deployment.yaml#L10) and leveraged [here](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/assets/cluster-version-operator/cluster-version-operator-deployment.yaml#L47-L48).
This proposal consists of two parts:

1) Formalizing a concept of an "addon" annotation which allows a given resource to be excluded based
on installer input. For example the console related resources could be annotated as

```yaml
annotations:
  addon.openshift.io/console: "true"
```

2) Defining an install config mechanism whereby the user can opt out of specific addons.

InstallConfig.ExcludeAddons
- console
- samples

Which resources ultimately get installed for a given cluster would be the set of resources encompassed
by the CLUSTER_PROFILE(if any), minus any resources explicitly excluded by the excluded addons configuration.

Examples of candidate components to be treated as addons:

* console
* imageregistry
* samples
* baremetal operator
* olm
* ???

### Risks and Mitigations

The primary risk is that teams understand how to use these new annotations and apply them
correctly to the full set of resources that make up their addon.  Inconsistent or
partial labeling will result in inconsistent or partially deployed resources.

Another risk is that this introduces more deployment configurations which might
have unforeseen consequences (e.g. not installing the imageregistry causes some
other component that assumes there is always an imageregistry or assumes the
presence of some CRD api that is installed with the imageregistry to break).

There was some discussion about the pros/cons of allowing each component to be enabled/disabled independent
of that component explicitly opting into a particular (presumably well tested) configuration/topology
[here](https://github.com/openshift/enhancements/pull/200#discussion_r375837903).  The position of this EP is that
we should only recommend the exclusion of fully independent "addon" components that are not depended on by
other components.  Further the assumption is that it will be reasonable to tell a customer who disabled
something and ended up with a non-functional cluster that their chosen exclusions are simply not supported
currently.


## Design Details

### Open Questions


1. Do we want to constrain this functionality to turning off individual components?  We could
also use it to
  a) turn on/off groups of components as defined by "solutions" (e.g. a "headless" solution
  which might turn off the console but also some other components).  This is what CLUSTER_PROFILES
  sort of enable, but there seems to be reluctance to expand the cluster profile use case to include
  these sorts of things.
  b) enable/disable specific configurations such as "HA", where components could contribute multiple
  deployment definitions for different configurations and then the installer/CVO would select the correct
  one based on the chosen install configuration (HA vs single node) instead of having components read/reconcile
  the infrastructure resource.

2. How does the admin enable a component post-install if they change their mind about what components
they want enabled?  Do we need/want to allow this?

3. What are the implications for upgrades if a future upgrade would add a component which would have
been filtered out during install time?  The install time choices need to be stored somewhere in
the cluster and used to filter applied resources during upgrades also.  My understanding is today
this is handled with CLUSTER_PROFILES and EXCLUDE_ANNOTATIONS by setting the env vars on the CVO
pod, but if we want to allow the set to be changed (see (2), we need a more first class config
resource that is admin editable)


### Test Plan

1) Install clusters w/ the various add-on components included/excluded and confirm
that the cluster is functional but only running the expected add-ons.

2) Upgrade a cluster to a new version that includes new resources that belong to
an addon that was included in the original install.  The new resources should be
created.

3) Upgrade a cluster to a new version that includes new resources that belong to
an addon that was excluded in the original install.  The new resources should *not* be
created.

4) After installing a cluster, change the set of addons that are included/excluded.
Newly included addon resources should be created, newly excluded ones should be
deleted.  (Do we want to support this?)



### Graduation Criteria

Would expect this to go directly to GA once a design is agreed upon/approved.

#### Dev Preview -> Tech Preview
N/A

#### Tech Preview -> GA
N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

For upgrades, any new resources should have the same exclusion filters applied by the CVO.
For downgrades, if downgrading below the version of the CVO that supports this logic
previously excluded components will get created on the cluster.  This is likely
not a concern since you can't downgrade below the version you started at, and if
you're using this feature that means you started at a version of the CVO that supports it.

If we allow enabling filters post-install, then we need to revisit the implications of
downgrades.

There is also some risk if a particular resource has different annotations in different
versions, then upgrading/downgrading could change whether that resource is excluded by
the CVO or not.  Once created, the CVO never deletes resources, so some manual cleanup
might be needed to achieve the desired state.  For downgrades this is probably acceptable,
for upgrades this could be a concern (resource A wasn't excluded in v1, but is excluded
in v2.  Clusters that upgrade from v1 to v2 will still have resource A, but clusters
installed at v2 will not have it).  Technically this situation can already arise today
if a resource is deleted from the payload between versions.


### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

The primary drawback is that this increases the matrix of cluster configurations/topologies and
the behavior that is expected from each permutation.

## Alternatives

* CVO already supports a CLUSTER_PROFILE env variable.  We could define specific profiles like "headless"
that disables the console.  CLUSTER_PROFILE isn't a great fit because the idea there is to define a relatively
small set of profiles to define specific sets of components to be included, not to allow a user to fully pick
and choose individual components.  We would have to define a large set of profiles to encompass all the possible
combinations of components to be enabled/disabled.

* CVO already supports an EXCLUDE_MANIFESTS env variable which is used to implement the ROKS deployment topology.
Unfortunately it only allows a single annotation to be specified, so even if we want to use it for this purpose
it needs to be extended to support multiple annotations so multiple individual components can be excluded
independently rather than requiring all components to be excluded to share a single common annotation.

Regardless we need a way to expose this configuration as a first class part of the install config provided by the
user creating the cluster, so at a minimum we need to add a mechanism to wire an install config value into
the CVO arguments and allow the CVO to consume more than a single annotation to exclude.


* Allow the installer to specify additional resources to `include` in addition to ones to `exclude`.  This has the challenge
of potentially conflicting with the specific set of resources that a cluster_profile defines.  There are some
components that should never be deployed in a particular cluster_profile and so we do not want to allow the user
to add them.  Examples would be resources that should only be created in standalone installs, not hypershift
managed ones, because hypershift has its own versions of those resources.


## Infrastructure Needed

N/A