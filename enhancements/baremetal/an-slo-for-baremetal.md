---
title: an-slo-for-baremetal
authors:
  - "@markmc"
reviewers:
  - "@abhinavdahiya"
  - "@dhellmann"
  - "@enxebre"
  - "@eparis"
  - "@hardys"
  - "@sadasu"
  - "@smarterclayton"
  - "@stbenjam"
approvers:
  - TBD
creation-date: 2020-02-13
last-updated: 2020-02-21
status: provisional
see-also:
 - https://github.com/markmc/cluster-baremetal-operator
replaces:
superseded-by:
---

# An SLO for baremetal

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

1. How to handle upgrades?
2. Which release should this change target?

## Summary

The Bare Metal Operator provides bare metal machine management
capabilities needed for the Machine API provider on the `baremetal`
platform, and there is no equivalent for this component on other
platforms.

The Bare Metal Operator is currently managed by the Machine API
Operator in a fashion that requires the Machine API Operator to have
significant bare metal specific knowledge. This proposal outlines a
plan for the Bare Metal Operator to become a fully-fledged Second
Level Operator under the management of the Cluster Version Operator.

(To avoid likely confusion, please note that this proposal describes a
new project tentatively named Bare Metal Operator (BMO), implying that
the current baremetal-operator project will be renamed to reflect the
fact it is "just a controller").

## Motivation

In order to bring up a cluster using the baremetal platform:

1. The installer needs to capture bare metal host information and
   provisioning configuration for later use.
2. Something needs to install the CRDs for these bare metal specific
   resources created by the installer.
3. Something needs to launch a controller for these resources.

Currently (1) is achieved by the installer creating:

* A Provisioning resource manifest
* Manifests for BareMetalHost resources and their associated secrets

and these manifests are applied by the cluster-bootstrap component
towards the end of the cluster bootstrapping process.

This resource creation step does not succeed until the step (2)
completes - i.e. the relevant CRDs are applied - and this is currently
done by the Cluster Version Operator (CVO) as it applies the manifests
for the MAO.

Finally, (3) happens when the MAO detects that it is running on the
`baremetal` infrastructure platform and instantiates a relatively
complex Deployment including the BareMetalHost controller and various
containers running other components, all from the Metal3 project. The
configuration of this deployment is driven by the Provisioning
resource created by the installer in (1).

There are two problems emerging with this design:

* A sense that the MAO is responsible for a significant amount of bare
  metal specific tasks that should be declared outside of the scope of
  the MAO, particularly since it does not have equivalent
  responsibilities on any other platform.

* Expanding needs for bare metal specific manifests - for example, new
  CRDs used to drive new BMO capabilities - to be installed early in
  the cluster bring-up means introducing yet more bare metal specific
  concerns into the MAO.

Steps (2) and (3) are aspects of cluster bring-up which the CVO is
clearly well-suited. However, to date, it was understood that creating
a Second Level Operator (SLO) (in other words, a CVO-managed operator)
for bare metal would not make sense, since it implied a component
installed and running on clusters where it is not needed.

### Goals

Allow bare metal machine management capabilities to be fully
encapsulated in a new SLO.

Ensure that this new SLO has minimal impact on clusters not running on
the `baremetal` platform.

### Non-Goals

### Proposal

Recognizing that bare metal support warrants the creation of a new
"subsystem":

1. Create a new, OpenShift-specific project called the Bare Metal
   Operator.
2. This project should implement a new SLO which is responsible for
   installing bare metal specific CRDs and running the BareMetalHost
   controller and related Metal3 components.
3. The BMO should meet all of the standard expectations of an SLO,
   including for example keeping a ClusterOperator resource updated.
4. On infrastructure platforms other than `baremetal`, the BMO should
   following emerging patterns for "not in use" components, for
   example setting its cluster operator condition to `Disabled=true`,
   `Available=true`, and `Progressing=False`.

### Risks and Mitigations

(FIXME)

- Impact on non-baremetal platforms
- Implementation cost and complexity
- Upgrades
- API stability
- Additional bare metal specific functionality in future
- Upstream (Metal3) implications

## Design Details

The Bare Metal Operator is a new Second Level Operator (SLO) whose
operand is a controller for the BareMetalHost resource and associated
components from the Metal3 project. The below sections covers
different areas of the design of this new SLO.

### Standard SLO Behaviors

As an SLO, the BMO is expected to adhere to the standard expected
behaviours of SLO, including:

1. The BMO image should be tagged with
   `io.openshift.release.operator=true` and contain a `/manifests`
   directory with all of the manifests it requires the CVO to apply,
   along with an `image-references` file listing the images referenced
   by those manifests that need to be included in the OpenShift
   release image.
2. Implement a `ClusterOperator` resource, including updating its
   status field with the `Available/Progressing/Degraded` conditions,
   operator version number, and any `relatedObjects`.

While not required initially, other common SLO patterns can be
considered in future:

1. Implement an operator configuration resource, including
   `OperatorSpec` and `OperatorStatus` (as per in openshift/api#125)
2. Implement cluster-level configuration under `config.openshift.io`
   (as per openshift/api#124)
3. Expose a `/metrics` endpoint for Prometheus to be configured to
   scrape (via a `ServiceMontitor`) and define any relevant Prometheus
   alert rules based on those metrics.

### "Not In Use" SLO Behaviors

Unlike most other SLOs, the BMO is not applicable to all cluster
configurations. On clusters running on an infrastructure platform
other than `baremetal` it should adhere to the emerging expected
behaviors for "not in use" SLOs, including:

1. Setting its `ClusterOperator` condition to `Disabled=true`,
  `Available=true`, `Progressing=False` with appropriate messages.
2. User interfaces should convey this disabled state differently than
   a failure mode (e.g. by graying it out).

Currently, insights-operator is the only other example of an SLO
following this pattern of using a `Disabled` status. Other somewhat
similar cases following different patterns include:

- Image registry and samples have a `Removed` management state where
  `Degraded=False`, `Progressing=False`, `Available=True` with
  `Reason=currentlyUnmanaged`.
- The cluster credentials operator has a `disabled` config map setting
  that can be used to disable the operator and it then sets
  ClusterOperator status conditions to `Degraded=False`,
  `Progressing=False`, `Available=True` with
  `Reason=OperatorDisabledByAdmin` for all three conditions.

### BMO Details

The BMO will:

- Be a new `openshift/cluster-baremetal-operator` project.
- Publish an image called `cluster-baremetal-operator`.
- Use CVO run-level 30, so its manifests will be applied in parallel
  with the MAO.
- Add a new `baremetal` `ClusterOperator` with an additional
  `Disabled` status for non-baremetal platforms.
- Use a new namespace called `openshift-baremetal`.
- Install the `metal3.io` `Provisioning` (cluster-scoped) and
  `BareMetalHost` (namespaced) CRDs.
- Run under a new `openshift-baremetal/cluster-baremetal-operator`
  `ServiceAccount`.
- Be launched by a `openshift-baremetal/cluster-baremetal-operator`
  `Deployment`, copying much of the MAO pod spec in terms of
  `system-node-critical` priority class, running on masters, security
  context, resource requests, etc.
- Implement a controller reconciling the singleton
  `provisioning-configuration` cluster-scoped `Provisioning` resource
- Do nothing except set `Disabled=true`, `Available=true`, and
  `Progressing=False` when the `Infrastructure` resource a platform
  type other than `BareMetal`.
- Based on the values in the `Provisioning` resource, create a
  `metal3` `Deployment` and associated `metal3-mariadb-password` under
  the `openshift-baremetal` namespace. This is the same as the MAO
  currently creates under the `openshift-machine-api` namespace.

### Test Plan

### Graduation Criteria

### Upgrade / Downgrade Strategy


### Version Skew Strategy


## Implementation History


## Drawbacks


## Alternatives

### Continue with the BMO under the MAO

In order to fully encapsulate the responsibilities of the BMO in an
operator - and remove the bare metal specific code and manifests from
the MAO - the MAO coul add a generic operator management framework
for platform specific operators, and the BMO would integrate with this
framework.

This would involve a more generic mechanism where the MAO could
discover and apply any required manifests from BMO image would mean
the addition of operator management capabilities that look very much
like some of the CVO's capabilities.

Adding such a framework seems unnecessarily complex, when there will
only be a single user of this framework.

### Add platform awareness to the CVO

In order to reduce the impact of the BMO when running on non bare
metal platforms, the CVO could gain the ability to manage operators
that are platform-specific, meaning the BMO would move only be
installed and run when the CVO detects (via a
`io.openshift.release.platform` image label, for example) that this
operator is required on this platform.

While this may seem a minimal extension to the CVO's capabilities, we
want to avoid a trend where the CVO continues to gain more and more of
such conditional behavior.

### Use a CVO cluster profile

The [cluster profiles
enhancement](https://github.com/openshift/enhancements/pull/200) offer
a generic framework for conditions that affect how the CVO applies the
content in a release image. This framework could be used in this case
by creating a `baremetal` cluster profile, and the BMO would only be
installed when this profile is active.

As per the enhancement document, cluster profiles are being introduced
to initially handle two specific cases (hypershift, CRC) and there is
a desire to proceed cautiously and avoid using cluster profiles
extensively at this point. Also, the enhancement proposes that only a
single cluster profile can be activated at a time, and such a
`baremetal` profile is not something that would naturally be mutually
exclusive with other potential profiles.

Compared to proposed mechanism to reduce the impact of the BMO on non
bare metal platforms - i.e. the `Disabled` state - there are greater
potential downsides from jumping into using cluster profiles for this
at this early stage.

## References

- "/enhancements/baremetal/baremetal-provisioning-config.md"
- https://github.com/openshift/machine-api-operator/pull/302
- https://github.com/metal3-io/baremetal-operator/issues/227
- https://github.com/openshift/enhancements/pull/90
- https://github.com/openshift/enhancements/pull/102


