---
title: samples-bootstrapped-as-removed
authors:
  - "@gabemontero"
reviewers:
  - "@bparees"
  - "@adambkaplan"
  - "@derekwaynecarr"
approvers:
  - "@bparees"
  - "@adambkaplan"
  - "@derekwaynecarr"
creation-date: 2020-01-06
last-updated: 2020-01-06
status: implementable
see-also:
replaces:
superseded-by:
---

# Bootstrapping the samples operator as "Removed" on x86 for non-standard installs

Since the advent of OpenShift 4.1 and the initial introduction of the samples operator,
the range of install scenarios where attempting to install samples out of the box is counter productive
has increased with both 4.2, as well as 4.3/4.4 (which have not yet GA'ed as of this writing).

In particular:
- initial disconnected/restricted network install deployments have proven cumbersome because 
if the cluster administrator does not address the needs of Managed samples within 2 hours the operator
marks itself `Degraded`.  The current choices the cluster administrator has to avoid this are, namely:

  -- mirroring images from registry.redhat.io to the mirrored registry used for install, and 
updating the samples registry on the operator's config object
  -- adding any unwanted imagestreams to the skipped list of the operator's config object, or
  -- marking the operator as Removed so that no imagestreams are created and no image imports are attempted.
- for IPv6 single stack based installs, since the terms based registry, registry.redhat.io, currently does not 
support IPv6, all samples aside from the Jenkins imagestreams (which are part of the install payload), will fail to
import out of the gate, where again, unless one of the options noted for disconnected/restricted network are
employed, the operator will make itself degraded after 2 hours.

## Release Signoff Checklist

- [/] Enhancement is `implementable`
- [/] Design details are appropriately documented from clear requirements
- [/] Test plan is defined
- [/] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary

In such scenarios where samples installation is doomed to fail out of the gate, it now is apparent to us that 
the samples operator should start off as `Removed`, with no samples content installed.

Those scenarios currently include
- disconnect / restricted network installs
- IPv6 single stack installs

## Motivation

With the mandate in 4.2 that the samples operator should mark itself `Degraded` if any imagestream imports
are still failing after 2 hours from the start of the install, cluster administrators have been caught off 
guard when the samples operator eventually marks itself `Degraded` in such cases where samples installation 
is doomed to fail without manual intervention from the cluster administrator.

In particular, the options available are:
- mirror desired imagestream images (where typically there are multiple versions per imagestream) to 
an available and functional registry; and mirror them in such a way such that the repository and image spec
match what is used on registry.redhat.io
- since there are many imagestreams, most likely add the imagestreams they are not concerned with to the 
skip list in the operator config object
- override the registry used by the samples operator in its config object
- or if you want to punt on samples entirely, mark the operator as `Removed`

### Goals

1) Alleviate out of the install concerns in scenarios where samples installation will fail out of the gate

2) Allow the cluster administrator time to decide what to do with samples in the cluster

### Non-Goals

Other usability improvements around installing samples from registries other than registry.redhat.io have 
been identified in Jira:

- regular expressions for further manipulation of the image reference in the imagestream
- modes where failed imagestreams are automatically added to the skip list (where the administrator then
automatically can have them added to the management set by fixing the import by mirroring the images, fixing
the registry in question, etc.)
- moving the samples operator out of the payload / CVO and making it an OLM operator, where the cluster
administrator controls which imagestreams are installed when the provision the OLM operator
- aggregating network topology details like disconnected/mirrored via our cluster config API (which was previously 
discussed and ultimately abandoned).

Bootstrapping as removed is considered the meets minimum, least impacting change to the samples operator 
that addresses the scenarios where out of the box imagestreams pointing to registry.redhat.io will fail.

## Proposal

With startup today, the samples operator bootstraps as `Managed`.  That bootstrapping can be made conditional,
and instead come up as `Removed` when the samples operator determines image stream imports will fail.

- Over a 3 minute window during initial install, the samples operator will attempt to create a TCP connection 
to `registry.redhat.io` and if one never succeeds, will bootstrap as `Removed`, with no samples installed.
- The clusteroperator `Reason` fields note the inability to access the terms based registry.

The cluster administrator is then free to investigate why the inability to access `registry.redhat.com` exists
and act accordingly. Options could include:

- Samples are left in `Removed`
- Images related to samples imagestreams are mirrored into an accessible registry, and then a) `samplesRegistry` is
updated to that accessible registry in the sample operator's config object, b) Imagestreams not mirrored are added
to the config's `skippedImagestreams` list, c) the operator is switched to `Managed`
- Whatever proxy or underlying network condition that prevents access to `registry.redhat.io` is addressed and then
the samples operator is switched back to `Managed`

### Risks and Mitigations

1) Misidentifying a cluster as tolerating `Managed` out of the box (i.e. failing to catch a case we are trying
to prevent with this enhancement)
2) Bootstrapping as `Removed` for a cluster where imagestreams are importable (i.e. breaking something that works today)

Also note, a fair amount of tests in `e2e-*-builds`, `e2e-aws-image-ecosystem`, `e2e-aws-jenkins`, and the subset of 
conformance tests in e2e-aws that leverage OpenShift Builds depend on the samples as a starting point.

For e2e environments for IPv6 and disconnected / restricted network, those tests will either have to be filtered 
out of the ginkgo query or reworked so that any needed builder/deployment images typically retrieved from `registry.redhat.io`
are either created or preloaded in the test cluster prior to their use in any extended test cases.

## Design Details

### Test Plan

Consider the following in developing a test plan for this enhancement:
- QE already had some disconnected CI and we've been working with them via bugzillas to get it set up.  This change
could remove the need for additional administration on their part to keep the cluster from having `Degraded` operators
- For IPv6, the samples operator e2e will be updated to inspect the test pod for IPv6 and adjust accordingly

### Graduation Criteria

No tech preview, beta, multiple maturity levels needed.

1) Samples install as they do traditionally in a connected IPv4 environment.
2) Samples start as `Removed` in the special case scenarios noted above.

Graduation will be achieved when this support GA's minimally in 4.4 and 4.3.z as part of IPv6 initiative. 

### Upgrade / Downgrade Strategy

Samples operator already identifies migration today, along with existing vs.
new installations.

The current plan is to only target new installations with these changes.

However, an implementation where for an existing install we 

- use our existing identification of whether image stream imports are failing
- and the techniques noted above to discern whether disconnected/restricted network install or IPv6 single stack ar
in play

could allow us to take an existing installation with a samples operator either in `Degraded` state or destined to 
be in `Degraded` state and adjust from `Managed` to `Removed` automatically.

This choice is noted here for now to facilitate discussion during the enhancement review process.

### Version Skew Strategy

N/A

## Implementation History

## Drawbacks

Working from `Removed` to a working `Managed` seems the easier / better path based on initial customer engagements with
disconnected / restricted network install.  But Murphy's Law would probably say somebody has gotten used to the 
existing out of the box behavior and would rather go from a broken `Managed` to functional `Managed` operator in
one fell swoop.

For example, as part of the multi-arch work, early adopters have requested that since samples exist out of the box for x86
they should as well for s390x or ppc64le.

## Alternatives

See the `Non-Goals` section for items that could also be classified as `Alternatives`.

## Infrastructure Needed [optional]

IPV6 and Disconnected environments for launching e2e's either from PRs out periodic tests.
