---
title: component-selection-during-install
authors:
  - "@bparees"
reviewers:
  - "@staebler - install team - need agreement on install-config api updates and CVO config rendering"
  - "@LalatenduMohanty - ota/cvo team - need agreement on CVO resource filtering api and new behavior"
  - "@soltysh - oc adm release team - need agreement on how the list of valid capabilities will be generated and embedded in the release payload image."
approvers:
  - "@decarr - support for configurable CVO-managed content set feature"
  - "@sdodson - as the staff engineer most closely tied to install experience"
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

This enhancement proposes a mechanism for cluster installers to exclude one or more optional components
(capabilities) for their installation which will determine which payload components are/are not installed
in their cluster.  Core components are defined as the set of Second Level Operators managed by the Cluster
Version Operator which today cannot be disabled until after completing the install and editing a CVO
override, or editing the CVO overrides as part of rendering+editing manifests.  In addition, using CVO
overrides put the cluster into an unsupported and un-upgradeable state, making it insufficient as a solution.

The proposed UX is to make this a first class part of the install config api with the implementation
being arguments supplied to the CVO to filter out the user-selected manifests based on groupings called
capabilities.



## Motivation

There is an increasing desire to move away from "one size fits all" cluster installations, and
towards flexibility about what should/should not exist in a new cluster out of the box.  This can
be seen in efforts such as hypershift, single node, and code-ready-containers.  Each of these
efforts has done some amount of one-off work to enable their requirements.  This EP proposes a
mechanism that allows components to be disabled in a first class way that the installer exposes.

### Goals

* Admins can easily explicitly exclude specific "optional" components from their cluster, at install time.
* Admins can enable a previously excluded optional component, at runtime.
* Install wrappers like assisted-installer can define an install-config that excludes specific components
* Define an api that could be used in the future to exclude cluster capabilities based on things other than
CVO filtering, such as turning off a particular api for the cluster.

### Non-Goals

* Making control-plane critical components optional (k8s apiserver, openshift apiserver, openshift controller,
  networking, etc)
* Defining which components should be disable-able (this will be up to component teams to classify themselves
as `capabilities` or not)
* Providing a way to install OLM operators as part of the initial cluster install.  This EP is about making
the install experience around the existing CVO-based components more flexible, not adding new components to the
install experience.
* Allowing components to be disabled post-install.
* Eliminating or replacing cluster profiles
* Encoding logic in the installer itself about which components should be disabled under specific circumstances


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

* As a team using openshift for a specific use case such as edge deployments, I want to provide
an install experience that disables components that aren't needed for my use case.

### API Extensions

N/A - no api extensions are being introduced in this proposal

### Implementation Details/Notes/Constraints [optional]

The CVO already has the ability to respect annotations on resources, as can be seen
[here](https://github.com/openshift/cluster-kube-apiserver-operator/blob/c03c9edf5fddf4e3fb1bc6d7afcd2a2284ca03d8/manifests/0000_20_kube-apiserver-operator_06_deployment.yaml#L10) and leveraged [here](https://github.com/openshift/hypershift/blob/b98bc43467a73921891ede7d6477757b647f1985/control-plane-operator/controllers/hostedcontrolplane/cvo/reconcile.go#L152-L153).
This proposal consists of two parts:

1) Formalizing a concept of a "capability" annotation which allows a given resource to be excluded based
on installer input. For example the console related resources could be annotated as

```yaml
annotations:
  capability.openshift.io/name: "console"
```

The `oc adm release new` command will be updated to extract these annotations and embed a list of the valid
capabilities in the release payload image.  This list can be consulted by users to determine the set of
capabilities they can include/exclude when configuring the installation.

Note: Resources are enabled unless explicitly excluded by configuration.  If a component is defined
as a capability than it can be included or excluded depending upon the InstallConfig.capabilities API.
If the default is set to exclude i.e. `inclusionDefault: Exclude` then all capabilities will be excluded
unless specifically included. Similarly if the default is include then all capabilities will be
included in the install config unless excluded explicitly.  If `inclusionDefault` is not specified,
it is treated as (defaults to) `Include` which ensures backwards compatibility with the existing behavior
of always installing all components on the cluster.  In the future we may allow resources to be excluded
unless explicitly included, this can be done by introducing a new annotation such as 
`capability.openshift.io/defaultDisable`.  If we do so, we will need to define how that choice interacts
with the cluster scoped "default capabilities on/off" configuration setting.  For now, this EP takes the
position that individual components that exist in the payload cannot choose to be default-disabled.

2) Defining an [install config api](https://github.com/openshift/installer/blob/790048166067273d34f76bea4220fa395b1cce1b/pkg/types/installconfig.go#L70) field whereby the user can opt out of specific capabilities.

Example 1: include all capabilities by default, exclude some specific capabilities
```yaml
InstallConfig.capabilities
  inclusionDefault: Include
  exclude:
  - console
  - samples
```

Example 2: exclude all capabilities by default, include some specific capabilities.
```yaml
InstallConfig.capabilities
  inclusionDefault: Exclude
  include:
  - registry
```


To determine which resources ultimately get installed for a given cluster:

1) start with all the resources in the payload
2) remove any resources that don't match the configured [cluster profile](https://github.com/openshift/enhancements/blob/7da960b98b2835022823f569efa1e18e9e410696/enhancements/update/cluster-profiles.md)

From the remaining set of resources:
3) if any resource is part of an explicitly excluded capability, remove it
4) if any resource is part of an explicitly included capabilitity, keep it
5) if the inclusionDefault is Exclude, remove all remaining resources(i.e. not covered by (4)) that are part of a capability


The `inclusionDefault` field (carried through to the CVO) allows an Admin to choose what behavior they want for new
capabilities that might come as part of a future upgrade (either have the new capability installed during
the upgrade, or exclude it).  The `inclusionDefault` field itself will default to `Include`.

Specifying the same capability as an `include` and an `exclude` is an error condition.

Specifying a capability name that isn't represented in the payload is an error condition.  Admins who want to
pre-emptively exclude future capabilities can use `inclusionDefault: Exclude` and then explicitly include
anything they want the cluster to have.

Examples of candidate components to be treated as capabilities:

* console
* imageregistry
* samples
* cluster baremetal operator
* olm/marketplace
* csi-*
* insights
* monitoring
* ???



3) Pass the list of filtered annotations to the CVO.  This is distinct from overrides because overrides
put the cluster in an unsupported state.  Filtered annotations are supported for upgrades.  The filtered
components will be listed in the [ClusterVersion](https://github.com/openshift/api/blob/4436dc8be01e8dcd8b250e1b32bb0fbd64ba78ac/config/v1/types_cluster_version.go#L35) object:

```yaml
  spec:
    capabilities:
      inclusionDefault: [Include|Exclude]
      exclude:
      - capability1
      - capability2
      include:
      - capability3
      - capability4
```

The CVO will filter out(not apply/reconcile) resources that are annotated with key/value
`capability.openshift.io/name=$exclusions` and listed in the `exclude` section, or are not listed
and the `default` setting is `exclude`.

If a resource participates in multiple capabilities, it should specify all capabilities as plus-sign separated
values in the annotation(e.g. `capability.openshift.io/name=console+monitoring`).  The resource will only be 
included if all the capabilities on the resource are enabled.  This allows a component
to define a `ServiceMonitor` which is part of both the `Monitoring` capability and the `Foo` capability.  If
either of those capabilities are excluded, the `ServiceMonitor` should not be created.  (If Monitoring is excluded
then no ServiceMonitors should be created, if only Foo is excluded, then only Foo's ServiceMonitors should be
excluded while ServiceMonitors associated with other capabilities would not be).

Specifying the same capability as an `include` and an `exclude` is an error condition.  This avoids the need 
to define behavior for what to do when a user specifies the same capability in both, and update a status field
indicating what choice the CVO made.  To enforce this the CVO will need to evaluate the spec values and, if
they are valid, copy them into status.  If they are not valid, status must contain an indication of what is
invalid and why.  This also means the CVO must be driven off the status values, not the spec values.

Note: The CVO must wait until the ClusterVersion resource exists (created by the installer) before creating
any filterable resource(a resource with a capability annotation) to ensure there is no race condition in which
the CVO starts creating resources that should have been filtered, before it has the filter list.


Strawman of status api, this will be iterated on in api review.
```yaml
  status:
    capabilities:
      inclusionDefault: [Include|Exclude]
      exclude:
      - capability1
      - capability2
      include:
      - capability3
      - capability4
    conditions:
      - condition1
      - condition2
```

Conditions will reflect the validity/invalidity of the spec-requested configuration.
status.capabilities will reflect the current enforced capability set.

It may make sense to have a status.capabilities.implicit field to reflect capabilities which
were not explicitly requested by spec, but got pulled in due to the inclusionDefault, or due to a resource
moving between capabilities(see step 5 below).



4) Admin can remove an item from the excluded annotations list, but they cannot add an item to it.  If an
item is removed, the CVO will apply the previously filtered(skipped) resources to the cluster on the next reconciliation.
Adding an item to the filtered list is not supported because it requires the component be removed from the
running cluster which has more significant implications for how all traces of the component are removed.  The
CVO should reject admin edits/updates that add new entries to the exclusion list.  Again this requires the
introduction of a validating webhook for the ClusterVersion resource.

Similarly, the `inclusionDefault` cannot be changed from `include` to `exclude` after the cluster install is complete.

The currently configured capabilities settings for the CVO should be recorded in telemeter so we can understand
the configuration of a given cluster.

In the future, we might allow specific APIs to be disabled, such as the build api.  This could be done by
defining additional capability keywords that can be put into the install-config field being defined here,
which would drive the creation of cluster config that the apiserver+controllers used to disable that particular
api.

5) During upgrade, resources may move from being associated with one capability, to another, or move
from being part of the core(always installed) to being part of an optional capability.  In such cases,
the governing logic is that once a resource has been applied to a cluster, it must continue to be
applied to the cluster(unless it is removed from the payload entirely, of course).  Furthermore,
if any resource from a given capability is applied to a cluster, all other resources from that
capability must be applied to the cluster, even if the cluster configuration would otherwise filter
out that capability.  These are referred to as implicitly enabled capabilities and the purpose is to
avoid scenarios like this that would break cluster functionality:

```text
1) initially registry resources are part of core(not defined in a capability)
2) a cluster is installed, registry resources are applied to the cluster, admin chooses to 
disable optional capabilities by default.
3) a new version of OCP moves the registry resources into an optional capability
4a) during upgrade the registry resources are not applied to the cluster because the registry
capability is not enabled
OR
4b) during upgrade, the existing registry resources continue to be applied(because we have a rule
that says once a resource is applied, we will continue to reconcile it in future versions), but
new registry resources added to the payload are not applied because they are filtered due to the
registry capability not being enabled, resulting in critical registry resources not being applied
and the registry potentially breaking.
```

This scenario is avoided by specifying that once a resource is applied to a cluster, it will
always be reconciled in future versions, and that if any part of a capability is present,
the entire capability will be present.

This means the CVO must apply the following logic:

```text
1) for the new payload, find all resources that are filtered by the current cluster config
2) check if any of those resources are already applied to the cluster by explicilty
checking what is on the cluster already
3) for any resource that is being filtered, but already exists in the cluster apply that
resource and implicitly enable any capabilities it is a part of (can be reported in Status)
which may mean additional resources get applied.
```

Behavior Axioms:
1) Cluster admin can explicitly opt into, or out of, any capability at install time.
2) Cluster admin can indicate that all capabilities not explicitly opted into/out of(via (1)), should
   either be enabled, or not enabled.  This choice will also apply to new capabilities introduced in upgrades.
3a) Cluster admin cannot disable an enabled capability post-install.  They can be enabled post-install.  (it is
too difficult to safely/fully clean up the cluster when disabling a capability)
3b) Once the CVO has applied/reconciled a resource, it must continue to do so even if future upgrades
mean that resource would nominally be excluded (such as because the capability it is associated
with has changed to a capability that is disabled on the cluster).
4) Cluster Profiles take precedence over a resource's capability association and inclusion.  If the
   cluster's profile doesn't include a particular resource, enabling the capability won't add that
   resource to the cluster.  If the cluster profile does include it, it can still be excluded via
   the capabilities mechanism.
5) Individual capabilities/resources cannot specify whether they should be included or excluded
   by default.  If a resource is in the payload, it is included by default, unless the cluster
   admin has made the choice to exclude all "optional" content by default.
6) If any resource from a capability is applied to the cluster, the other resources in the
capability must also be applied to the cluster, regardless of the expressed configuration.  This
may result in a capability the admin explicitly disabled, being enabled as part of an upgrade.  E.g.
if a resource that was already applied to the cluster becomes part of a capability that the admin
had explicitly disabled.




### Risks and Mitigations

The primary risk is that teams understand how to use these new annotations and apply them
correctly to the full set of resources that make up their component.  Inconsistent or
partial labeling will result in inconsistent or partially deployed resources for a component.

Another risk is that this introduces more deployment configurations which might
have unforeseen consequences (e.g. not installing the imageregistry causes some
other component that assumes there is always an imageregistry or assumes the
presence of some CRD api that is installed with the imageregistry to break).

There was some discussion about the pros/cons of allowing each component to be enabled/disabled independent
of that component explicitly opting into a particular (presumably well tested) configuration/topology
[here](https://github.com/openshift/enhancements/pull/200#discussion_r375837903).  The position of this EP is that
we should only recommend the exclusion of fully independent "capability" components that are not depended on by
other components.  Further the assumption is that it will be reasonable to tell a customer who disabled
something and ended up with a non-functional cluster that their chosen exclusions are simply not supported
currently, or that they must accept the degraded functionality caused by the missing dependency, if they
intend to keep it disabled.

Since the only components/resources that can be filtered out of the installation are ones that are explicilty
annotated with `capability.openshift.io/name`, end-users will not be able to use this mechanism to filter
components/resources that we did not intend for them to be able to filter out.

## Design Details

### Answered Questions


1. Do we want to constrain this functionality to turning off individual capabilities?  We could
also use it to
  a) turn on/off groups of components as defined by "solutions" (e.g. a "headless" solution
  which might turn off the console but also some other capabilities).  This is what CLUSTER_PROFILES
  sort of enable, but there seems to be reluctance to expand the cluster profile use case to include
  these sorts of things.
  b) enable/disable specific configurations/topologies such as "HA", where components could contribute multiple
  deployment definitions for different configurations and then the installer/CVO would select the correct
  one based on the chosen install configuration (HA vs single node) instead of having components read/reconcile
  the infrastructure resource.

Current plan is to constrain this functionality to capability level controls.

2. How does the admin enable a component post-install if they change their mind about what components
they want enabled?  Do we need/want to allow this?

Turning on a component later is relatively easy (we expose a config resource for the CVO that defines
the filter, we allow the user to remove items from the filter, the CVO will apply the previously
filtered resources during the next reconciliation).

Turning off a component later is more problematic because while technically
[the CVO can delete resources](https://github.com/openshift/enhancements/blob/3db41c0ad1d7960ca66f79a60b962c9b1ec1753c/enhancements/update/object-removal-manifest-annotation.md)
that are annotated in a particular way, so it could use the same logic to "delete" resources that are now
matching a filter, just deleting the resources for the component isn't sufficient, as the component also
needs to clean itself up in case it created any additional resources on the cluster or contributed any
configuration.

Therefore we plan to support turning a component on, but not turning it off.

3. What are the implications for upgrades if a future upgrade would add a component or resource which would
have been filtered out during install time?  

There should be no implication here, the CVO has the list of annotations it will filter based on, if the
new resources match those annotations, the new resources will also be filtered(never applied to the cluster).

4. How prescriptive do we want to be about what can/can't be turned off?  Components need to opt into
this by annotating their resources, so it's not completely arbitrary.

This will need to be evaluated on a case by case basis as a component considers adding the annotation
to its resources that will allow it to be filtered out/disabled.

5. What if a user specifies a capability name to exclude that doesn't match anything?

The installer won't know it's not going to match anything, so it should do nothing.  The CVO
can tell it doesn't match anything, so it can provide a warning on the clusterversion resource, but
it should not be an error since a user might put a keyword into the list in anticipation of upgrading
to a new OCP version where the keyword will match (and thus exclude) a new capability, or perhaps
the keyword used to match a capability that no longer exists after an upgrade.

Along the same lines, if a user both "Excludes" and "Includes" a capability(if we ever add Include as
a thing you can do), that should be treated as an error/invalid configuration.

6. What to do(if anything) for components with interdependencies, to ensure a user doesn't break
enabled components by disabling a dependency?  Options include:

* Do nothing other than document dependencies so users know what not to turn off
* Don't even annotate dependency resources for filtering, so if something is a dependency it cannot be turned off
* Logic in the install or CVO that intelligently analyzes the filters the user has supplied and checks
  for dependency issues (least desirable solution imho).

Our current plan is to only annotate resources that are not depended upon (aka "leaf" components)

### Open Questions

1. What to do for components where disabling them has implications on other components or the way certain
apis behave.  Example: disabling the internal registry changes the behavior of imagestreams
(can't push to the imagestream anymore to push content to the internal registry) as well as the assumptions
made by tools like new-app/new-build (create imagestreams that push to the internal registry).

### Test Plan

1) Install clusters w/ the various add-on components included/excluded and confirm
that the cluster is functional but only running the expected add-ons.  One of these
tests should disable all optional components to ensure the cluster is functional.
This will necessitate disabling or modifying tests that depend on the disabled
component(s).

2) Upgrade a cluster to a new version that includes new resources that belong to
an addon that was included in the original install.  The new resources should be
created.

3) Upgrade a cluster to a new version that includes new resources that belong to
an addon that was excluded in the original install.  The new resources should *not* be
created.

4) After installing a cluster, enable additional addons.  The newly enabled addons should
be installed/reconciled by the CVO.

5) After installing a cluster, disable an addon.  The configuration change should be
rejected by the CVO.  Disabling a component post-install is not supported.



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
the CVO or not.  Once created, the CVO normally never deletes resources, so some manual
cleanup might be needed to achieve the desired state.  For downgrades this is probably
acceptable, for upgrades this could be a concern (resource A wasn't excluded in v1, but is
excluded in v2.  Clusters that upgrade from v1 to v2 will still have resource A, but clusters
installed at v2 will not have it).  Technically this situation can already arise today
if a resource is deleted from the payload between versions.  Since the CVO can actually
delete properly annotated resources, there may be an option here to make the CVO automatically
delete "now filtered" resources, but that brings us back to the aforementioned issues
with ensuring total cleanup of a filtered component.  Manual cleanup/release notes may be
the most comprehensive solution.



### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A - no api extensions are being introduced in this proposal

#### Failure Modes

N/A - no api extensions are being introduced in this proposal

#### Support Procedures

N/A - no api extensions are being introduced in this proposal

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
combinations of components to be enabled/disabled.  Direction has been set that cluster profiles should only be
used defining the standalone and hypershift form factors.

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

* Use clusteroverrides to exclude content.  The problem w/ this approach is it puts the cluster into an unsupported
and non-upgradeable state.

* Move "optional" CVO components to be OLM based and make it possible to install+upgrade OLM operators as though
they are part of the payload.  While this is part of our longterm roadmap, it has two major challenges.  First,
moving a component from the CVO to OLM is likely more engineering work and has more significant implications on
the distribution+testing of that component, than adding this filtering capability to the CVO, so it is an easier
win in the short term.  Secondly, OLM itself needs significant new functionality to be able to install+upgrade
OLM operators as part of the cluster lifecycle.  The long timeline it will take to build that capability makes
it necessary that we take a tactical approach in the short term to enable this capability at the CVO level.

* If we want components to be disabled by default (either existing components or new ones added in the future),
we can add their component names to a list of default-disabled components that the installer populates
the install-config with.  Users can then edit that disabled list in the install-config to remove those
components if they want them enabled, and add other components they'd like to disable.

## Infrastructure Needed

N/A