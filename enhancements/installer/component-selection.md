---
title: component-selection
authors:
  - "@bparees"
reviewers:
  - "@staebler - install team - need agreement on install-config api updates and CVO config rendering"
  - "@LalatenduMohanty - ota/cvo team - need agreement on CVO resource filtering api and new behavior"
  - "@soltysh - oc adm release team - need agreement on how the list of valid capabilities will be generated and embedded in the release payload image."
  - "@wking"
approvers:
  - "@decarr - support for configurable CVO-managed content set feature"
  - "@sdodson - as the staff engineer most closely tied to install experience"
api-approvers:
  - "@deads2k"
creation-date: 2021-05-04
last-updated: 2022-05-10
tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-7589
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
* Install wrappers like assisted-installer can define an install-config that excludes specific components (by asking for components they do want, possibly an empty set).
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

### Implementation Details/Notes/Constraints

#### Capabilities

Capabilities must be [registered in the API][capability-registry].

* 4.11
  * baremetal
  * marketplace
  * openshift-samples
* Maybe later:
  * console
  * imageregistry
  * csi-*
  * insights
  * monitoring
  * ???

#### Capability sets

Capability sets must be [registered in the API][capability-set-registry].

* `None`: an empty set enabling no optional capabilities.
* `v4.11`: the recommended set of optional capabilities to enable for the 4.11 version of OpenShift.
  This list will remain the same no matter which version of OpenShift is installed.
* `vCurrent`: the recommended set of optional capabilities to enable for the cluster current version of OpenShift.
  For a 4.11 cluster, this matches `v4.11`.  We may add or remove capabilities from this set in 4.12 clusters.

Admins who want to pre-emptively exclude future capabilities can use `None` or `v4.11` or another `baselineCapabilitySet` which will not evolve with time.

We may decide to add additional sets like `v4.12` or `myGreatUseCase` in the future, but do not commit to doing so.

#### Manifest annotations

Formalizing a concept of a "capability" annotation gives the ability to exclude the given resource based
on installer input. For example the console related resources could be annotated as

```yaml
annotations:
  capability.openshift.io/name: console
```

If a resource participates in multiple capabilities, it should specify all capabilities as plus-sign separated
values in the annotation(e.g. `capability.openshift.io/name=console+monitoring`).  The resource will only be 
included if all the capabilities on the resource are enabled.  This allows a component
to define a `ServiceMonitor` which is part of both the `monitoring` capability and the `Foo` capability.  If
either of those capabilities are excluded, the `ServiceMonitor` should not be created.  (If `monitoring` is excluded
then no ServiceMonitors should be created, if only `Foo` is excluded, then only Foo's ServiceMonitors should be
excluded while ServiceMonitors associated with other capabilities would not be).

To determine which manifests are managed for a given cluster, the CVO will:

1. Start with all the manifests in the payload.
2. If TechPreviewNoUpgrade is not set, [remove all the manifests with `release.openshift.io/feature-set=TechPreviewNoUpgrade`](../update/cvo-techpreview-manifests.md).
3. Remove all the manifests that lack [`include.release.openshift.io/{profile}=true`](../update/cluster-profiles.md).
4. Remove all the manifests that declare a capability in `capability.openshift.io/name` which is not part of the current enabled capabilities.

#### Install configuration

Defining an [install config api](https://github.com/openshift/installer/blob/790048166067273d34f76bea4220fa395b1cce1b/pkg/types/installconfig.go#L70) field whereby the user can opt into specific capabilities.

```yaml
capabilities:
  baselineCapabilitySet: None
  additionalEnabledCapabilities:
  - openshift-samples
```

Specifying an unrecognized capability or capability set name is an error condition.

The installer will validate the pass the information through to the CVO for resource management, by setting [`spec.capabilities` in ClusterVersion](#resource-management).

#### Resource management

The CVO will grow new `capabilities` properties in `spec` and `status`.
This is distinct from `overrides` because `overrides` put the cluster in an unsupported state.

The `spec` property uses the same structure as [the install-config](#install-configuration):

```yaml
  spec:
    capabilities:
      baselineCapabilitySet: None
      additionalEnabledCapabilities:
      - openshift-samples
```

The CVO will calculate an effective status:

```yaml
  status:
    capabilities:
      enabledCapabilities:
      - openshift-samples
      knownCapabilities:
      - baremetal
      - marketplace
      - openshift-samples
```

`knownCapabilities` includes all known capabilities, regardless of whether they are enabled or not.
Removing `enabledCapabilities` from `knownCapabilities` will give the set of available but not currently enabled capabilities (e.g. web-console checkboxes for "do you want to enable these additional capabilities?").

`enabledCapabilities` will often align with the requested `spec.capabilities`.
But in some situations, `enabledCapabilities` may extend the requested capability set with additional entries, as discussed below.

Note: The CVO must wait until the ClusterVersion resource exists (created by the installer) before creating
any filterable resource (a resource with a capability annotation) to ensure there is no race condition in which
the CVO starts creating resources that should have been filtered, before it has the filter list.

##### Capabilities can be installed

Admins can adjust `spec.capabilities` to request additional capabilities, and the CVO will extend `enabledCapabilities` and begin reconciling [the newly-enabled manifests](#manifest-annotations).

##### Capabilities cannot be uninstalled

Admins can adjust `spec.capabilities` to stop requesting a capability, but the CVO will refuse, continue to hold that capability in `enabledCapabilities`, and continue to reconcile [the associated manifests](#manifest-annotations).
When the requested `spec.capabilities` diverge from `enabledCapabilities`, the CVO will set `ImplicitlyEnabledCapabilities=True` in ClusterVersion's `status.conditions` with a message pointing out the divergence.

Removing installed components is complicated, and we are deferring that for now.

We will [consider][admission-webhook] adding an admission webhook to improve the UX for admins who attempt to remove an enabled capability, by replacing "notice `ImplicitlyEnabledCapabilities=True`" with "have your PATCH fail with a 400".

###### Updates

During updates, resources may move from being associated with one capability, to another, or move
from being part of the core (always installed) to being part of an optional capability.  In such cases,
the governing logic is that once a resource has been applied to a cluster, it must continue to be
applied to the cluster (unless it is removed from the payload entirely, of course).  Furthermore,
if any resource from a given capability is applied to a cluster, all other resources from that
capability must be applied to the cluster, even if the cluster configuration would otherwise filter
out that capability.  These are referred to as implicitly enabled capabilities and the purpose is to
avoid scenarios that would break cluster functionality.  Once a resource is applied to a cluster, it will
always be reconciled in future versions, and that if any part of a capability is present,
the entire capability will be present.

This means the CVO must apply the following logic:

1. For the outgoing payload, find all manifests that are [currently included](#manifest-annotations).
2. For the new payload, find all the manifests that would be included, in the absence of capability filtering.
3. Match manifests using group, kind, namespace, and name.
4. Union all capabilities from matched manifests with the set of previously-enabled capabilities to compute the new `enabledCapabilities` set.

As in [the general case](#capabilities-cannot-be-uninstalled), when the requested `spec.capabilities` diverge from `enabledCapabilities`, the CVO will set `ImplicitlyEnabledCapabilities=True` in ClusterVersion's `status.conditions` with a message pointing out the divergence.

#### Axioms

1. Cluster admin can explicitly opt into, or out of, any capability at install time.
2. Cluster admin can enable a disabled capability post-install.
3. Cluster admin cannot disable an enabled capability post-install (it is too difficult to safely/fully clean up the cluster when disabling a capability).
4. Once the CVO has applied/reconciled a resource, it must continue to do so even if future upgrades mean that resource would nominally be excluded (such as because the capability it is associated with has changed to a capability that is disabled on the cluster).
  This may result in a capability the admin explicitly disabled, being enabled as part of an upgrade.
  E.g. if a resource that was already applied to the cluster becomes part of a capability that the admin had explicitly disabled.
5. Cluster profiles take precedence over a resource's capability association and inclusion.
  If the cluster profile doesn't include a particular resource, enabling the capability won't add that resource to the cluster.
  If the cluster profile does include it, it can still be excluded via the capabilities mechanism.
6. Individual capabilities can specify whether they should be included or excluded by default by adding or removing the capability from the `vCurrent` set.

#### Remote monitoring

The currently configured capabilities settings for the CVO will be recorded in a `cluster_version_capability` Telemetery metric and in Insights' ClusterVersion resource, so we can understand the configuration of a given cluster.

#### Possible future work

In the future, we might allow specific APIs to be disabled, such as the build api.  This could be done by
defining additional capability keywords that can be put into the install-config field being defined here,
which would drive the creation of cluster config that the apiserver+controllers used to disable that particular
api.

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

### Drawbacks

The primary drawback is that this increases the matrix of cluster configurations/topologies and
the behavior that is expected from each permutation.

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

The admin adjusts `spec.capabilities` to enable the additional capabilities (e.g. by appending to `additionalEnabledCapabilities)`.
The CVO will apply the previously filtered resources during the next reconciliation.

Turning off a component later is more problematic because while technically
[the CVO can delete resources](../update/object-removal-manifest-annotation.md)
that are annotated in a particular way, so it could use the same logic to "delete" resources that are now
matching a filter, just deleting the resources for the component isn't sufficient, as the component also
needs to clean itself up in case it created any additional resources on the cluster or contributed any
configuration.

Therefore we plan to support turning a component on, but not turning it off.

3. What are the implications for upgrades if a future upgrade would add a component or resource which would
have been filtered out during install time?  

There should be no implication here, the CVO has the list of annotations it will filter based on, if the
new resources match those annotations, the new resources will also be filtered (never applied to the cluster).

4. How prescriptive do we want to be about what can/can't be turned off?  Components need to opt into
this by annotating their resources, so it's not completely arbitrary.

This will need to be evaluated on a case by case basis as a component considers adding the annotation
to its resources that will allow it to be filtered out/disabled.

5. What if a user specifies a capability name to exclude that doesn't match anything?

The installer will compare it with the set of [registered capabilities][capability-registry] and fail validation.

The ClusterVersion CRD will explicitly enumerate known capabilities and capability sets, and the Kubernetes API server will 400 requests to set invalid values.

6. What to do(if anything) for components with interdependencies, to ensure a user doesn't break
enabled components by disabling a dependency?  Options include:

* Do nothing other than document dependencies so users know what not to turn off
* Don't even annotate dependency resources for filtering, so if something is a dependency it cannot be turned off
* Logic in the install or CVO that intelligently analyzes the filters the user has supplied and checks
  for dependency issues (least desirable solution imho).

Our current plan is to only annotate resources that are not depended upon (aka "leaf" components)

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

### How To Implement A New Capability

1. Select a name for the new capability, like `insights`.
2. Open a pull request adding `capability.openshift.io/name` annotations to the manifests that deliver the capability, like [this][bump-annotations].
    Some manifests should only be included if multiple capabilities are enabled, and in that case use the `+` delimiter between capability names, like `capability.openshift.io/name=insights+monitoring`.
3. In presubmit CI for the pull request, the cluster-version operator will [exclude manifests with unrecognized capabilities][cvo-exclude-unrecognized].
    Review presubmit results for your annotation pull request to ensure that:
    1. The capability is being completely removed, without leaving dangling resources that you forgot to annotate.
    2. CI suites which you expected to pass continue to pass.
        You may find that some test-cases assume or require your capability's presence, and they may need to grow logic to skip or alter the test conditions when your capability is not installed (like [this][bump-origin] or by adding a suitable regex to the `[Skipped:NoOptionalCapabilities]` [annotation rule][annotation-rules]).
    If your annotation addition spans multiple pull requests, either because the manifests being annotated span multiple repositories or because you also need to make test suite adjustments in other repositories), you may be able to [use cluster-bot][cluster-bot] to run tests on a release assembling multiple in-flight pull requests.
4. Introduce the new capablity name in the openshift/api repo, like [this][bump-api].
    1. If no [versioned capability set](https://github.com/openshift/api/blob/8324d657dee1d594a8a7768e5569fea6a8f887a9/config/v1/types_cluster_version.go#L257-L291) exists for the current OCP version under development, introduce one as part of your pull request.
5. Bump the openshift/api vendored dependency in the openshift/cluster-version-operator repo, like [this][bump-cvo-vendor], so the cluster-version operator will understand the new annotation and its `vCurrent` inclusion status.

    At this point, clusters installed with the `None` set will nominally not include the new capability, but if you had un-annotated manifests in previous versions, those will still be installed.
    Prereleases at this point may need to document this behavior ("this capability does not yet disable anything"), depending on the expected level of interest.
6. Land your annotation pull request(s), so that the capability related resources will be ignored when the new capability is disabled.
7. Bump the openshift/api vendored dependency in the openshift/installer repo, like [this][bump-installer-vendor], to allow folks to request the new capability via `additionalEnabledCapabilities` without the installer rejecting the unrecognized capability name.
8. Check the relevant `no-capabilities` periodic, like [this][periodic], to confirm that your change has not introduced any regressions.

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

* Allow the installer to specify additional resources to exclude in addition to ones to `additionalEnabledCapabilities`.
  We did not have a consensus argeement about how useful this would be, and because we can always add a property in this space later without breaking backwards compatibility, we're leaving it off for now.

* Use clusteroverrides to exclude content.  The problem w/ this approach is it puts the cluster into an unsupported
and non-upgradeable state.

* Move "optional" CVO components to be OLM based and make it possible to install+upgrade OLM operators as though
they are part of the payload.  While this is part of our longterm roadmap, it has two major challenges.  First,
moving a component from the CVO to OLM is likely more engineering work and has more significant implications on
the distribution+testing of that component, than adding this filtering capability to the CVO, so it is an easier
win in the short term.  Secondly, OLM itself needs significant new functionality to be able to install+upgrade
OLM operators as part of the cluster lifecycle.  The long timeline it will take to build that capability makes
it necessary that we take a tactical approach in the short term to enable this capability at the CVO level.

* Other installer UXes, such as a list of default-disabled components that the user can select from.
  The installer has all the capabilties information in [the][capability-registry] [registries][capability-set-registry], so we can build this sort of thing later if it seems useful.

## Infrastructure Needed

N/A

[admission-webhook]: https://issues.redhat.com/browse/OTA-575
[annotation-rules]: https://github.com/openshift/origin/blob/a86fa526218f3e5c5b8e101ebb78c287a6a4b215/test/extended/util/annotate/rules.go#L342-L348
[bump-annotations]: https://github.com/openshift/insights-operator/pull/646
[bump-api]: https://github.com/openshift/api/pull/1212
[bump-cvo-vendor]: https://github.com/openshift/cluster-version-operator/pull/737
[bump-installer-vendor]: https://github.com/openshift/installer/pull/5645
[bump-origin]: https://github.com/openshift/origin/pull/26998
[capability-registry]: https://github.com/openshift/api/blob/6f735e7109c87826edee3ca02a753071ecc933b9/config/v1/types_cluster_version.go#L231-L255
[capability-set-registry]: https://github.com/openshift/api/blob/6f735e7109c87826edee3ca02a753071ecc933b9/config/v1/types_cluster_version.go#L257-L291
[cluster-bot]: https://docs.ci.openshift.org/docs/how-tos/interact-with-running-jobs/#how-to-run-the-test-suites-outside-of-ci
[cvo-exclude-unrecognized]: https://github.com/openshift/library-go/blob/3c66b317b110fe1a94bb0c9a9fa9f7a46df29941/pkg/manifest/manifest.go#L198
[periodic]: https://testgrid.k8s.io/redhat-openshift-ocp-release-4.12-informing#periodic-ci-openshift-release-master-ci-4.12-e2e-aws-sdn-no-capabilities
