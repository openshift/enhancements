---
title: single-node-developer-cluster-profile
authors:
  - "@rkukura"
  - "@guillaumerose"
reviewers:
  - "@cfergeau"
  - "@deads2k"
  - "@enxebre"
  - "@hexfusion"
  - "@LalatenduMohanty"
  - "@marun"
  - "@mfojtik"
  - "@sgreene570"
  - "@soltysh"
  - "@sspeiche"
  - "@sttts"
  - "@wking"
  - all OCP group leads
approvers:
  - "@derekwaynecarr"
  - "@smarterclayton"
creation-date: 2020-09-18
last-updated: 2020-11-20
status: implementable
see-also:
  - "/enhancements/update/cluster-profiles.md"
  - "/enhancements/single-node-production-edge-cluster-profile.md"
replaces:
superseded-by:
---

# Single Node Developer Cluster Profile

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add a new 'single-node-developer' [cluster
profile](https://github.com/openshift/enhancements/blob/master/enhancements/update/cluster-profiles.md)
that defines the set of OpenShift payload components, and the
configuration thereof, applicable to single-node resource-constrained
non-production OCP clusters that support the development of OpenShift
applications to be deployed for production on other OCP clusters, such
as those using the default 'self-managed-high-availability'
profile. The 'single-node-developer' profile will be defined for and
utilized in producing the Code Ready Containers (CRC) product that
runs in a single VM on a developer’s workstation or laptop. This
profile is not intended or supported for any kind of production
deployment.

In a cluster deployed using the 'single-node-developer' profile, a
single node serves both as the cluster’s control plane and as a worker
node. The profile will include all the OpenShift payload components
needed for application development, such as the web console, image
registry, operator hub and many others. In order to minimize system
resource requirements, the profile will exclude production-oriented
features such as high availability of the control plane components,
machine upgradability, and advanced telemetry and logging.

This 'single-node-developer' profile will result in a new topology of
OpenShift components to be officially supported for its intended usage
in the CRC product. A new CI job will verify that a cluster deployed
using this profile passes the subset of the Kubernetes and OpenShift
conformance tests that are applicable to this topology.

The CRC team remains ultimately responsible for the CRC product, and
thus for the contents of the 'single-node-developer' cluster
profile. Once the profile is introduced, all teams developing
OpenShift components will be aware of how their components are
configured in this profile. For many components, this will be simply a
matter of whether the component is included or excluded, with little
or no impact on the component itself. For other components,
specialized configurations and/or features may be used in this
profile, possibly requiring development and ongoing maintenance
efforts. Some such specialized component configurations might be
shared with other cluster profiles that have similar requirements,
such as the '[single-node-production-edge'
profile](https://github.com/openshift/enhancements/pull/504).  Going
forward, as the components making up OpenShift evolve, and as CRC
product requirements evolve, the CRC team and various OpenShift
component teams will need to collaborate to ensure the
'single-node-developer' profile continues to provide the desired CRC
user experience.


## Motivation

The CRC product’s VM image is currently built using [Single Node
Cluster (SNC)](https://github.com/code-ready/snc) tooling. The snc.sh
shell script runs the OpenShift installer to deploy a running cluster
into a libvirt/KVM environment. One VM serves as the bootstrap node,
while another serves as the combined master/worker node. Between
installation steps, and after the installer completes, snc.sh makes
various modifications to the cluster, such as configuring etcd and
other services to run with a single replica, tuning CPU, storage, and
networking details, and deleting unneeded components. Once the cluster
is properly configured and has stabilized, the createdisk.sh shell
script takes a snapshot of the master/worker node VM and turns that
into the VM image that is installed for users by the CRC installer.

The current SNC image build process, described above, defines the CRC
cluster configuration via day-2 disablement. It starts with a fully
supportable cluster, then makes a set of changes to disable unneeded
functionality and reduce resource requirements for the CRC use
case. Because these day-2 configuration changes are not visible to
OpenShift component developers, CRC is often broken as components
evolve, and debugging CRC-specific issues is very difficult. This
exclusion-based approach also does not accommodate components
providing features specific to CRC, limiting the ability to optimize
resource usage and functionality for the CRC use case.

By adding a 'single-node-developer' cluster profile, the CRC cluster
configuration will be defined by explicit inclusion rather than day-2
disablement. This will make each component's configuration in CRC more
broadly visible, thereby reducing CRC's brittleness, enabling
CRC-specific optimizations, and improving CRC's supportability.

### Goals

* Reduce the brittleness of the SNC image build process and of the CRC
  product. By moving control of each component’s inclusion and
  configuration in single node developer clusters from the SNC tooling
  to the component itself, changes to individual components will be
  much less likely to require corresponding changes to the SNC
  tooling. Component developers will become more aware of how the
  component is configured in these clusters, and more likely to
  consider the implications of the changes on these clusters. The
  'single-node-developer' profile conformance CI job will expedite
  detecting and resolving any problems that are introduced by
  component changes.

* Enable better control and optimization of each component for single
  node developer clusters, further reducing CRC's resource
  requirements. This can range from controlling whether or not the
  component is included in the cluster, through setting configuration
  details, all the way to adding functionality specific to single node
  developer clusters. As new components or component capabilities are
  added to OpenShift, the component and CRC teams can work together to
  determine whether the capability is worth the cost in single node
  developer clusters before deciding to include the new functionality
  in the profile.

* Improve the supportability of the CRC product. The increased
  visibility of how components are utilized in single node developer
  clusters should reduce the likelihood of OpenShift component
  developers introducing CRC-specific problems, and reduce the effort
  required to understand and resolve problems when they do occur. A
  component developer may be able to reproduce and debug a
  CRC-specific customer issue using a deployment of the
  'single-node-developer' profile in their familiar development
  environment, avoiding the need to use the SNC tooling and/or setup
  an actual CRC-based environment for debugging purposes. Any
  specialized component features specific to the
  'single-node-developer' profile should also be covered by the
  component's unit tests, easing debugging and preventing issue
  regressions.

* Ensure that CRC is, and remains, fit-for-purpose - i.e. that
  applications developed using CRC will run as expected when deployed
  on production OpenShift clusters. The 'single-node-developer'
  profile conformance CI job will validate that the profile's topology
  passes a well-defined set of Kubernetes and OpenShift conformance
  tests. Specific tests in the suite that are not compatible with the
  topology will be documented and skipped.

### Non-Goals

* Single node clusters supportable for production or critical task
  workloads.  The cluster profile described in this enhancement
  proposal is not intended to support any form of production cluster
  deployment. Single node clusters for non-developer use cases, such
  as for edge computing, would require distinct cluster profiles with
  independent selection and configuration of components specific to
  those use cases.

* Changes to the operator hub experience in CRC. This enhancement is
  focused exclusively on the OpenShift payload operators managed by
  the Cluster Version Operator (CVO). Additional operators can be
  installed and managed via the Operator Lifecycle Manager (OLM). Some
  of those OLM-managed operators are likely incompatible with the
  'single-node-developer' profile due to missing dependencies or other
  topological issues, but are still available to install in CRC via
  the operator hub. Ideally these would not be visible in the default
  catalog of available operators on CRC. This issue is present prior
  to this enhancement, and should be addressed via a separate
  enhancement.

* A new CRC installer. Although it should simplify the SNC tooling
  used for CRC, this enhancement proposal does not change the basic
  approach to installing CRC or producing a VM image for the CRC
  installer. By making the definition of the content and configuration
  of the components in a single node developer cluster more orthogonal
  to the tooling that deploys the cluster, this enhancement could
  enable distinct future enhancements to improve the build and/or
  install process used by CRC.

* A new OpenShift installer. This enhancement proposal does not add a
  new OpenShift installer or involve changes, beyond the ability to
  specify the cluster profile, to the standard OpenShift installer
  currently used by the SNC tooling to build CRC installation
  images. Continued evolution of the existing installer, SNC tooling
  and other CRC components is expected.

* Functional changes for OpenShift. This enhancement proposal does not
  result in any functional changes to clusters that do not utilize the
  'single-node-developer' cluster profile.

## Proposal

The 'single-node-developer' cluster profile will be implemented in two
phases. The first phase, which is targeted for completion for the OCP
4.7 release, introduces the new cluster profile, and enables some
simplification of the SNC image build process by utilizing the profile
to control which components are included in the cluster. The second
phase, which might begin during the OCP 4.7 development cycle but will
continue afterwards, leverages the profile to optimize the resource
utilization of the components included in the cluster, and may further
simplify the SNC image build process.

### Phase One

In the initial phase, the following annotation will be added to the
manifest files for all CVO-managed components (operators, deployments,
etc.) that are to be included in single node developer clusters:

```text
include.release.openshift.io/single-node-developer: "true"
```

This annotation will need to be added to the manifests for all the
OpenShift components that are not currently removed by the snc.sh
script during the CRC build process. The components that are currently
removed, and thus should not need the annotation added, include:

* any unneeded components running in the openshift-monitoring namespace
* most components running in the openshift-insights namespace
* machine-api-operator
* machine-config-operator (may need to run once, requiring the annotation)
* openshift-cloud-credential-operator
* openshift-cluster-storage-operator

Note that OpenShift monitoring functionality is currently disabled by
the SNC tooling due to its subtantial memory requirements, but can be
re-enabled by CRC users that need it and have sufficient resources to
run it. Monitoring-related components that are needed in this use case
will be included in the profile during phase one, but will continue to
be disabled by SNC, until their resource requirements can be reduced
during phase two.

Additional components that are not currently removed by the SNC
tooling might be also be determined to be unnecessary, and thus not
require the annotation. Possibilities include:

* openshift-cluster-node-tuning-operator
* openshift-multus

As an example, the current default manifest for the console downloads
deployment begins with:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: openshift-console
  name: downloads
  annotations:
    include.release.openshift.io/self-managed-high-availability: "true"
spec:
  replicas: 2
  selector:
    # ...
```

After phase one, the same manifest will also apply to the
'single-node-developer' profile:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: openshift-console
  name: downloads
  annotations:
    include.release.openshift.io/self-managed-high-availability: "true"
    include.release.openshift.io/single-node-developer: "true"
spec:
  replicas: 2
  selector:
    # ...
```

In order to document that they have been purposely excluded from the
profile, the following annotation, which is ignored by the CVO, will
be added to any manifests not included in the profile if other
manifests in the same repository are included:

```text
include.release.openshift.io/single-node-developer: "false"
```

Once PRs adding these annotations have been merged to all the
necessary component repositories, the snc.sh script will be updated to
tell the OpenShift installer to use the 'single-node-developer'
cluster profile, and to remove the code that deletes the components
that are not included in the profile. At the conclusion of this
initial phase, the somewhat simplified SNC image build tooling should
produce a cluster equivalent to that currently produced, and all
existing CRC tests should pass. In addition, the
'single-node-developer' profile conformance CI job will be enabled at
this point, as described below.

From this point forward, any new component being added to OpenShift
will not be included in CRC unless a manifest with the
'single-node-developer' profile annotation specifying a value of
"true" is included in the new component. The CRC team will need to
engage with the OpenShift component team to help decide whether or not
the new component should be included in the profile, and if so,
whether it needs any special configuration for CRC. Once a decision
that the component should be included has been reached, the
appropriate annotations should be added to the new component's
manifests.

### Phase Two

In the second phase, various OpenShift components that are included in
single node developer clusters will be updated to use specialized
manifests in the 'single-node-developer' profile rather than the same
manifest used for default clusters. These specialized manifests will
configure the components for use in single node developer clusters,
replacing the patching currently done post-deployment by the CRC build
process, and potentially enabling additional reductions in resource
requirements.

Some of the manifest specializations expected to be included in this
phase are shown in the following table:


| Operator | Change |
| :--- | :--- |
| ingress | reduce the number of replicas of controller pod |
| console | reduce the number of replicas of console pod |
| console | reduce the number of replicas of downloads pod |
| authentication | reduce the number of replicas of oauth pod |
| lifecycle-manager | reduce the number of replicas of packageserver pod |
| etcd | have a configuration with useUnsupportedUnsafeNonHANonProductionUnstableEtcd option |
| monitoring | configure to reduce resource requirements |
| all | reduce memory and CPU requests so that we can downsize the VM |

The last item in particular is somewhat open ended. More detail can be
added to this enhancement as component optimization details are worked
out, or the more significant of these optimizations can be managed as
separate enhancements.

Continuing the example from phase one, we want to specialize the
console downloads deployment to use a single replica, so the original
manifest will become:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: openshift-console
  name: downloads
  annotations:
    include.release.openshift.io/self-managed-high-availability: "true"
    include.release.openshift.io/single-node-developer: "false"
spec:
  replicas: 2
  selector:
    # ...
```

and a new specialized manifest will be added to reduce the number of
replicas from 2 to 1:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: openshift-console
  name: downloads
  annotations:
    include.release.openshift.io/self-managed-high-availability: "false"
    include.release.openshift.io/single-node-developer: "true"
spec:
  replicas: 1
  selector:
    # ...
```

It is expected that most manifests specialized for single node
developer clusters will only differ slightly from the corresponding
manifests for default clusters. To avoid unintentional divergence,
some [templating
solution](https://github.com/openshift/cloud-credential-operator/pull/210)
should be used in the component build process to generate the
specialized manifests from the default manifests.

In the example above, only a minor change to an existing configuration
item was needed. In some other cases, a new configuration item will
need to be defined, along with changes to the logic of the
component. In such cases, the CRC team will work with the component
team to plan, implement, test, and maintain the needed changes. Entire
specialized manifests, or any individual new configuration items they
utilize, may be shared with other cluster profiles such as
'single-node-production-edge' that have similar requirements.

### User Stories

The initial direct user of the 'single-node-developer' cluster profile
is the SNC image build process, which is currently used by CRC as well
as some additional projects. If any of these indirect users did not
need the actual VM image provided by SNC, they could switch to using
the new cluster profile directly via the standard OpenShift installer.

#### Story 1

At the end of phase one, the SNC image build process will be slightly
simpler, and should run a bit faster. The resulting CRC cluster that
end users deploy should be unchanged.

#### Story 2

As phase two progresses with merges to various components, the SNC
image build process will become even simpler and quicker, and the
resulting CRC cluster should require less system resources.

### Implementation Details/Notes/Constraints

Phase one is pretty straightforward - simply adding an annotation to
the existing manifest files in the various repositories for components
to be included in single node developer clusters.

Phase two is more of an open ended process leveraging the new profile
to optimize the configuration of the individual components for single
node developer clusters. In general, each component will be addressed
independently and possibly multiple times, first to eliminate the
day-2 patching by SNC, and then to enable additional
optimizations. During implementation of this phase, it is expected
that patterns will emerge when common approaches apply across
different components.

### Risks and Mitigations

One risk is that all parts of the implementation of the cluster
profiles
[enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/update/cluster-profiles.md)
do not get merged in a timely manner. Implementing the
'single-node-developer' cluster profile requires the ability to
specify a non-default cluster profile to the OpenShift installer, and
for this to result in only the manifests annotated for the specified
cluster profile to be processed by the resulting cluster. CRC team
members have proposed CVO patches as part of that enhancement, and
will continue to participate in the review process, but progress
getting the implementation merged has been slow so far. Having this
enhancement as an additional use case for the cluster profile
mechanism hopefully will help clarify how that feature is being used
and expedite its implementation.

Another risk is that it will take too long to get the initial
annotations merged to all the needed OpenShift component repositories
during the first phase of development. Until all needed annotations
are merged, the SNC tooling cannot be updated to make use of the
'single-node-developer' cluster profile, as needed components would be
excluded from the cluster. The review process for this enhancement
should help raise awareness across the various teams that will need to
review and merge these patches.

A final risk is that new components will be added to OpenShift after
the completion of phase one that should be included in single node
developer clusters, but will not have the necessary annotation
included in their manifests due to lack of awareness or concern for
the CRC product. CRC team members will be available to collaborate
with component developers regarding whether a component should be
included in the 'single-node-developer' cluster profile, and if so,
how it might be configured to minimize resource requirements. If the
component is not included in the profile, its likely that conformance
tests for that component will also need to be skipped in the
'single-node-developer' conformance CI job.

## Design Details

### Open Questions

One currently open question is whether and how to allow a CRC user to
enable a component that the 'single-node-developer' profile excludes.

### Test Plan

Individual OpenShift components should not require any additional test
cases during phase one, as the annotation for the single node
developer cluster will be applied to existing manifest files that are
already tested. Once these annotations are all merged and the SNC
tooling is updated to have the OpenShift installer use the new cluster
profile, the existing SNC tests and CRC product tests will verify that
nothing has changed in the resulting cluster.

Once phase one of the profile implementation is complete, a new
OpenShift CI job will be added to run the Kubernetes and OpenShift
conformance test suites on a cluster deployed using the
'single-node-developer' profile. Tests that require components
disabled in the profile will be skipped. Note that these conformance
tests are run without the additional changes made by the SNC image
build tooling, so they do not necessarily reflect the actual
conformance of the CRC product. This CI job will initially run
periodically. Once it has been shown to work reliably, it could be
triggered automatically when PRs are submitted, at least for certain
OpenShift components.

As part of this CI job, or as a separate job, the memory utilization
of a 'single-node-developer' cluster will also be checked, so that the
impact of changes to OpenShift components, including changes made as
part of phase two of this enhancement, can be tracked.

The component testing required during the second phase will depend on
what changes are made to each individual component. If a specialized
manifest is added for a component, an appropriate subset of existing
tests for that component could be run using the new manifest (or the
configuration values it specifies), but testing the manifest via the
periodic conformance CI job may be sufficient. If new configuration
items or features are added to a component, those items will require
new component-level tests.

As phase two proceeds, the set of conformance tests skipped in the
'single-node-developer' conformance CI job may also need to be
adjusted. As phase two manifest specializations replace and eliminate
changes currently made by the SNC image build tooling, these test
results will more closely reflect the conformance of the actual CRC
product.

### Graduation Criteria

This enhancement does not add or change any APIs or other user-visible
features, so maturity levels and deprecation policies are not
relevant.

Graduation from phase one will happen when the SNC tooling is updated
to use the new cluster profile. This should not result in any visible
changes to users of the CRC product or to other users of SNC.

As phase two is an open ended process of optimizing individual
components, these changes will become available to SNC users,
including CRC, as they are merged.

### Upgrade / Downgrade Strategy

The presence of the 'single-node-developer' cluster profile,
annotations for the profile on existing manifest files, and new
specialized manifests annotated just for this profile will have no
effect on clusters using any other cluster profile, including the
default profile. Therefore, there should be no upgrade or downgrade
concerns for any clusters not using the 'single-node-developer'
profile.

CRC does not support in-place upgrade or downgrade of single node
developer clusters between OCP releases, so there are no related
requirements for the 'single-node-developer' profile.

### Version Skew Strategy

No component changes are foreseen as part of this enhancement that
would require handling version skew.

## Implementation History

TBD

## Drawbacks

The main drawback of the proposed enhancement is that there may turn
out to be multiple slightly different ideas of what constitutes a
single node developer cluster. This could result in the proposed
'single-node-developer' profile becoming a compromise that is not
ideal for CRC or for any other of its use cases. But discouraging
reuse (i.e. naming this the code-ready-containers profile) might
result in a proliferation of similar but distinct cluster profiles
each requiring independent maintenance and testing overhead. We hope
to find a balance between these extremes, where the
'single-node-developer' profile content is driven by the requirements
of the CRC product, but is useful as-is in other cases as well. If
this doesn’t turn out to be possible, additional profiles might
utilize many of the same specialized manifest files as the
'single-node-developer' profile, minimizing the amount of additional
maintenance and testing overhead.

Another drawback is that any new cluster profile imposes a new
supported topology on all components in the OpenShift payload,
including all future platform extensions. Component teams will need to
be aware of which profiles their component is included in, and will
need to avoid introducing dependencies on other components that might
be excluded from those profiles. Component teams will need to provide
ongoing maintenance and support for any specialized configurations or
features utilized in any profile. Note that somewhat equivalent
responsibility for a single node developer topology is currently being
borne by the CRC team in order to update the SNC tooling as changes
are made in OpenShift components. The component teams themselves are
much better positioned to control how changes they make impact all
supported cluster profiles.

## Alternatives

An alternative approach would be to start with some new installer that
is intended for minimal single node deployments such as the proposed
[Singe Node
Installation](https://github.com/openshift/enhancements/pull/440)
enhancement or the upstream [kind](https://kind.sigs.k8s.io/) project,
and build up to where it resembles OpenShift closely enough to support
development of applications for deployment on actual OpenShift
clusters. This approach would risk never being quite close enough to
OpenShift, and could turn out to take more effort and time than the
proposed approach to get to the point where it could replace CRC. Even
if a new installer were used, decisions about which components to
include for developers would need to be made, and specialized features
and configurations would still be required for certain components in
order to minimize resource requirements for single node
clusters. Therefore, much of the work involved in implementing the
proposed approach would still be necessary, and might be accomplished
via the cluster profile manifest annotations being proposed in this
enhancement. Since the cluster content defined in the profile is
orthogonal to the CRC installer implementation, implementing the
proposed approach now should not preclude development of alternative
installation approaches for CRC in the future.

Another alternative approach, at least for some of the phase two
optimization work, would be to define an
[InstallType](https://github.com/openshift/installer/pull/4209) for
CRC, and add code to components to change their behavior when the CRC
InstallType is being used. This approach could be used in conjunction
with the proposed cluster profile, where the profile determines which
components are included, and the InstallType determines how they
behave. A downside of this approach is that it hardwires CRC-specific
special cases into the components. Defining individual configuration
items in the components' manifests, as proposed here, allows any such
optimizations to be easily reused in other profiles, or to be disabled
for CRC by updating the manifest. The profile proposed here is focused
on defining the content and configuration of the cluster, rather than
means by which the cluster is installed, allowing features or
optimizations to be used and/or tested in contexts other than an
actual CRC VM image.

## Infrastructure Needed

Additional CI resources may be needed to run the
'single-node-developer' profile conformance suite CI job, but no new
types of infrastructure will be needed.
