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

1. ~~How to handle upgrades?~~
2. Which release should this change target?

## Summary

The `cluster-baremetal-operator` component will provide bare metal
machine management capabilities needed for the Machine API provider on
the `baremetal` platform. There is no equivalent for this component on
other platforms.

This functionality is currently provided by `machine-api-operator`
directly via `baremetal-operator` from the Metal3 project, and related
components. This situation requires the Machine API Operator to have
significant bare metal specific knowledge.

This proposal outlines a plan for a new bare metal specific component
as a fully-fledged Second Level Operator under the management of the
Cluster Version Operator.

The new `cluster-baremetal-operator` will initially merely adopt the
existing bare metal specific components - including
`baremetal-operator` from `machine-api-operator`. However, we expect
that additional bare metal specific functionality will be added to
this new operator over time.

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
  CRDs used to drive new baremetal capabilities - to be installed
  early in the cluster bring-up means introducing yet more bare metal
  specific concerns into the MAO.

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

1. Create a new, OpenShift-specific project called
   `cluster-baremetal-operator` aka CBO.
2. This project should implement a new SLO which is responsible for
   installing bare metal specific CRDs and running the BareMetalHost
   controller and related Metal3 components.
3. The CBO should meet all of the standard expectations of an SLO,
   including for example keeping a ClusterOperator resource updated.
4. On infrastructure platforms other than `baremetal`, the CBO should
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

`cluster-baremetal-operator` is a new Second Level Operator (SLO)
whose operand is a controller for the BareMetalHost resource and
associated components from the Metal3 project. The below sections
covers different areas of the design of this new SLO.

### Standard SLO Behaviors

As an SLO, the BMO is expected to adhere to the standard expected
behaviours of SLO, including:

1. The CBO image should be tagged with
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

Unlike most other SLOs, CBO is not applicable to all cluster
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

### cluster-baremetal-operator Details

The CBO will:

- Be a new `openshift/cluster-baremetal-operator` project.
- Publish an image called `cluster-baremetal-operator`.
- Use CVO run-level 30, so its manifests will be applied in parallel
  with the MAO.
- Add a new `baremetal` `ClusterOperator` with an additional
  `Disabled` status for non-baremetal platforms.
- Use the existing `openshift-machine-api` namespace where the
  BareMetalHost resources are also located.
- Install the `metal3.io` [`Provisioning`
  (cluster-scoped)](https://github.com/openshift/machine-api-operator/blob/40cbead/install/0000_30_machine-api-operator_04_metal3provisioning.crd.yaml)
  and [`BareMetalHost`
  (namespaced)](https://github.com/openshift/machine-api-operator/blob/40cbead/install/0000_30_machine-api-operator_08_baremetalhost.crd.yaml)
  CRDs.
- Run under a new `openshift-machine-api/cluster-baremetal-operator`
  `ServiceAccount`.
- Be launched by a `openshift-machine-api/cluster-baremetal-operator`
  `Deployment`, copying much of the MAO pod spec in terms of
  `system-node-critical` priority class, running on masters, security
  context, resource requests, etc.
- Implement a controller reconciling the singleton
  `provisioning-configuration` cluster-scoped `Provisioning` resource
- Do nothing except set `Disabled=true`, `Available=true`, and
  `Progressing=False` when the `Infrastructure` resource is a platform
  type other than `BareMetal`.
- Based on the values in the `Provisioning` resource, create a
  `metal3` `Deployment` and associated `metal3-mariadb-password` under
  the `openshift-machine-api` namespace. This is the same as the MAO
  currently creates.

### Test Plan

### Graduation Criteria

### Upgrade / Downgrade / Version Skew Strategy

The key thing to consider for upgrades is how to smoothly transition
the `metal3` deployment in the `openshift-machine-api` namespace from
the MAO to the CBO.

We assume that the CBO will first appear in `4.N.0` and take over from
the MAO in that release. We can merge changes into `4.N-1.y` paving
the way for this and, if necessary, require an upgrade to given
`4.N-1.y` version before the upgrade to `4.N.0`. Since we only support
upgrading directly from `4.N-1` to `4.N`, we can remove all upgrade
handling code in `4.N+1`.

#### Prior Art

We will take inspiration from the
[Separate OAuth API-Resources](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/separate-oauth-resources.md)
enhancement

> Add code in 4.(n-1) to the cluster-openshift-apiserver-operator that
> prevents it from managing an apiservice if it is "claimed" by the
> cluster-authentication-operator via an annotation and if the
> cluster-authentication-operator is at least at 4.n.

In the related
[cluster-openshift-apiserver-operator PR](https://github.com/openshift/cluster-openshift-apiserver-operator/pull/294)
the details are clear - `cluster-authentication-operator` indicates
it is "at least at 4.n" by setting the `managingOAuthAPIServer` to
`True` in the status field of its `authentication` resource. Then
`cluster-authentication-operator` "claims" an `apiservice` by setting
the `authentication.operator.openshift.io/managed` annotation.

Presumably the "at least at 4.n" check is required in addition to the
annotation to handle a downgrade scenario - where the annotation is
set, but `cluster-authentication-operator` has been downgraded.

#### Design

We need the following changes to MAO:

1. Respect a "gate" allowing CBO to "claim" the resource.

   We will use [the existing `machine.openshift.io/owned` annotation](http://github.com/openshift/machine-api-operator/pull/424)
   on the `metal3` deployment to indicate that MAO is managing the
   resource. A new `baremetal.openshift.io/owned` annotation will
   indicate that CBO is in control. Only one of these annotations
   should be set on a resource but, for the avoidance of doubt, CBO
   takes precedence.

   Before CBO is be added to the `4.N-dev` release image, MAO will
   need to respect this gate - it must not attempt to manage the
   `metal3` deployment if CBO has claimed the resource. This MAO
   change will be backported to `4.N-1`.

2. An "at least at 4.N" downgrade check.

   In order to guard against a downgrade scenario where CBO had
   claimed the `metal3` deployment - but the cluster has since been
   downgraded and CBO (manually) removed - we need some way of
   checking CBO is actually still around. We will use the existence of
   a `baremetal` clusteroperator for this purpose. Users would need to
   manually remove this resource after a downgrade.

3. A change to remove `metal3` deployment from MAO.

   In `4.N-dev`, once the CBO has been added to the release, we can
   remove all awareness of the `metal3` deployment from MAO.

#### Upgrade/Downgrade Scenarios

In order to check our intuition about the above, we can exhaustively
consider all possible combinations of the upgrade starting and ending
point, and reverting back to the starting point.

| State | MAO     | CBO            |
| ----- | --------| -------------- |
| A     | ungated | none           |
| B     | gated   | none           |
| C     | gated   | claim resource |
| D     | none    | claim resource |

First is the scenario where we upgrade from a release where MAO's
management of `metal3` is ungated. We can't support upgrading from
this point to a release which includes CBO.

* A->B: new MAO checks gate before updating resource
* A->C: ~~new MAO checks gate before updating resource, CBO claims
  resource - problematic if old MAO is still running~~
* A->D: ~~new MAO ignores `metal3`, CBO claims resource - problematic
  if old MAO is still running~~

Next is from a release where MAO's management of the resource is
gated. All scenarios can be supported.

* B->C: CBO is introduced and claims the resource, old and new MAO
  respects the gate
* B->D: CBO is introduced and claims the resouce, old MAO respects the
  gate, new MAO ignores `metal3`

And the final trivial scenario:

* C->D: old and new CBO claim the resource, old MAO respects the gate,
  new MAO ignores `metal3`

What's clear is that we cannot support a transition to `C` or `D`
without first going through `B`. So if we assume that `C` or `D` is
what we ship as the `4.N.0` release, then users must upgrade to `B` in
`4.N-1.y` before upgrading to `>=4.N.0`. Ideally, we should ship `B`
in `4.N-1.0`.

Now, the downgrade scenarios:

* B->A: no CBO, new MAO ignores gate, old MAO respects gate
* C->B: old CBO claims resource, old and new MAO respects gate -
  manual intervention needed to delete old CBO
* C->A: ~~old CBO claims resource, old MAO respects gate, new MAO
  ignores gate~~
* D->C: old and new CBO claims resource, old MAO ignores `metal3`, new
  MAO respects gate
* D->B: old CBO claims resource, old MAO ignores `metal3`, new MAO
  respects gate - manual intervention needed to delete old CBO
* D->A: ~~old CBO claims resource, old MAO ignores `metal3`, new MAO
  ignores gate~~

And so we see the reverse of the upgrade restriction - you can only
downgrade to `B` from `A`.

## Implementation History


## Drawbacks


## Alternatives

### Continue with the BMO under the MAO

In order to fully encapsulate the responsibilities of the BMO in an
operator - and remove the bare metal specific code and manifests from
the MAO - the MAO could add a generic operator management framework
for platform specific operators, and the BMO would integrate with this
framework.

This would involve a more generic mechanism where the MAO could
discover and apply any required manifests from BMO image would mean
the addition of operator management capabilities that look very much
like some of the CVO's capabilities.

Adding such a framework seems unnecessarily complex, when there will
only be a single user of this framework.

### Add platform awareness to the CVO

In order to reduce the impact of the CBO when running on non bare
metal platforms, the CVO could gain the ability to manage operators
that are platform-specific, meaning the CBO would move only be
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
by creating a `baremetal` cluster profile, and the CBO would only be
installed when this profile is active.

As per the enhancement document, cluster profiles are being introduced
to initially handle two specific cases (hypershift, CRC) and there is
a desire to proceed cautiously and avoid using cluster profiles
extensively at this point. Also, the enhancement proposes that only a
single cluster profile can be activated at a time, and such a
`baremetal` profile is not something that would naturally be mutually
exclusive with other potential profiles.

Compared to proposed mechanism to reduce the impact of the CBO on non
bare metal platforms - i.e. the `Disabled` state - there are greater
potential downsides from jumping into using cluster profiles for this
at this early stage.

## References

- "/enhancements/baremetal/baremetal-provisioning-config.md"
- https://github.com/openshift/machine-api-operator/pull/302
- https://github.com/metal3-io/baremetal-operator/issues/227
- https://github.com/openshift/enhancements/pull/90
- https://github.com/openshift/enhancements/pull/102
