---
title: Dual-Stack Networking
authors:
  - "@danwinship"
reviewers:
  - "@dcbw"
  - "@russellb"
approvers:
  - "@knobunc"
creation-date: 2020-04-23
last-updated: 2020-04-23
status: implementable
---

# Dual-Stack Networking

This covers adding dual-stack networking support via ovn-kubernetes in OCP.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Customers need IPv6 support. Customers still need IPv4 support too
though. Dual-stack FTW!

At the moment we expect dual-stack to be more popular with customers
than single-stack IPv6, even for customers who mostly only want IPv6,
because in many cases you end up needing 99% IPv6 and 1% IPv4, and
single-stack IPv6 Kubernetes does not allow that. (You end up needing
hacks outside of Kubernetes's view, like Multus-based interfaces.)

## Motivation

Customers would like to be able to run workloads with both IPv4 and
IPv6 connectivity, without sacrificing any of the networking
functionality provided by Kubernetes. This will allow OpenShift to
work in a wider variety of network environments and serve a wider
variety of workloads.

### Goals

- All known upstream Kubernetes dual-stack blockers will be resolved.
  (eg, [the Service `ipFamily` problem] that blocked us from enabling
  the `IPv6DualStack` feature gate in 4.4) and dual-stack progresses
  to beta upstream.

- OVN-Kubernetes will support dual-stack networking according to the
  model upstream Kubernetes finalizes

- We enable the `IPv6DualStack` feature gate by default

- Other OCP components that need to be updated to be dual-stack aware
  will be updated. (It is not clear how much actual work is needed
  here. We are currently researching.)

- We will have CI/cluster-bot support for dual-stack clusters on
  platforms that we support for customers (ie, bare metal).

- Assuming that the necessary CloudProvider work is completed
  upstream, we will have CI/cluster-bot support for dual-stack
  clusters on at least one cloud platform. (If the bare-metal solution
  is not sufficiently useful/available, we might do some work to help
  complete the upstream CloudProvider work.)

[the Service `ipFamily` problem]: https://github.com/kubernetes/kubernetes/pull/86895

### Non-Goals

- Dual-stack support for openshift-sdn or Kuryr

- Officially supporting dual-stack for third-party CNI plugins

- Officially (ie, not just for CI/dev) supporting dual-stack on any
  platform except bare metal.

## Proposal

### Implementation Details/Notes/Constraints

#### Early CI

To avoid the problems we had with single-stack IPv6, we will get CI
set up early, to ensure that regressions do not occur in components we
are not directly paying attention to. Note that we do not expect to do
any work on a forked branch of OCP as we did for single-stack IPv6.

There is an existing `e2e-metal-ipi` job that tests single-stack IPv6.
We will add another flavor of this job that runs a dual-stack configuration.
The dev and test environment utilized by this CI job has already been
[updated to support dual-stack](https://github.com/openshift-metal3/dev-scripts/pull/1017),
so all that's left is to configure the same job to run with one additional configuration
item, `IP_STACK=v4v6`.

Being able to bring up a dual-stack cluster at all requires enabling
the `IPv6DualStack` feature gate in `kubelet`, `kube-apiserver`, and
`kube-controller-manager`. We tried to do this for all clusters in 4.4
but [ran into bugs] and had to revert it. The [upstream bugs] are not
yet fixed.

To work around this for early CI, we will just add a
`FeatureGate.config.openshift.io` object to the install manifests to
enable the feature gate. (If the upstream `ipFamiliy` problem is not
fixed soon we may also commit a "`<drop>`" patch to origin to work
around at least the most-obviously-buggy parts so we can get to work
on other dual-stack issues while the behavior of `ipFamily` is still
being figured out upstream.)

[ran into bugs]: https://bugzilla.redhat.com/show_bug.cgi?id=1794376
[upstream bugs]: https://github.com/kubernetes/kubernetes/pull/86895

#### Upstream Kubernetes

We need to continue watching/shepherding upstream [the dual-stack KEP]
and [dual-stack kubernetes PRs]. In theory, not much further work is
needed upstream for dual-stack on bare metal. (CloudProvider support
for IPv6/dual-stack is much poorer.)

There are currently very few dual-stack-specific e2e tests upstream
and we will probably need to create some more.

[the dual-stack KEP]: https://github.com/kubernetes/enhancements/pulls?q=is%3Apr+is%3Aopen+dual-stack
[dual-stack kubernetes PRs]: https://github.com/kubernetes/kubernetes/issues?q=is%3Aopen+DualStack

#### OVN-Kubernetes

Work is ongoing to add dual-stack support to OVN-Kubernetes.
([OVN-Kubernetes Dual-Stack Tracker]). Upstream OVN-Kubernetes uses CI
based on `kind` (Kubernetes-in-Docker) and we will have a dual-stack
`kind` environment ([kind PR], [ovn-kube PR]) for testing it against
dual-stack Kubernetes.

[OVN-Kubernetes Dual-Stack Tracker]: https://github.com/ovn-org/ovn-kubernetes/issues/1142
[kind PR]: https://github.com/kubernetes-sigs/kind/pull/692
[ovn-kube PR]: https://github.com/ovn-org/ovn-kubernetes/issues/1248

#### Installer

The installer already allows configuring dual-stack clusters on
bare metal (and Azure) even though they don't yet work, so no work
should be needed there until/unless we want to support additional
cloud platforms.

#### Other OCP Components

We believe that *most* components will not need any new work to
support dual-stack beyond what was done for single-stack IPv6. In
particular, most services are now listening for both IPv4 and IPv6
connections regardless of cluster configuration, and all code that
deals with IP addresses should be capable of dealing with either IPv4
or IPv6 addresses.

We are currently doing a survey of single-stack-IPv6-related changes
that were made in the 4.4 cycle to use as a starting point for
figuring out where further dual-stack changes may be needed. (One
known problem is with services that only listen on "localhost" rather
than on all interfaces; there's no trivial way to listen on both
`127.0.0.1` and `::1`.)

### Risks and Mitigations

#### Completion Risks

If upstream Kubernetes fails to finalize a workable dual-stack
approach then we would be unable to support dual stack in OCP. We are
monitoring and contributing to the upstream PRs, KEPs, etc, to make
sure that this doesn't happen.

#### Functional Risks

We will need to carefully ensure that the `IPv6DualStack` feature gate
does not change any functionality on single-stack IPv4 clusters before
enabling it for all clusters.

We need to make sure that our firewalling strategy handles dual-stack
clusters appropriately (not accidentally allowing more access than
expected on one IP family).

We need to make sure that NetworkPolicy and EgressFirewall provide the
expected protection in dual-stack clusters (eg not accidentally
allowing connections to be made over IPv6 that are blocked over IPv4).

If any components do their own filtering / access control of incoming
connections, we need to make sure they are properly dual-stack aware.

## Design Details

### Test Plan

Initially, ovn-kubernetes dual-stack testing will be done via the
upstream kubernetes e2e test suite using `kind`. We will probably want
to add more dual-stack-specific e2e test cases in upstream Kubernetes.

Additionally we will have a bare-metal dual-stack OCP CI job.
Initially (before the ovn-kubernetes work is completed) it will not be
expected to get very far, but this will allow us to monitor for
regressions in other components.

Eventually we will be able to make the periodic bare-metal job be
release-informing, and we can add it as a blocking job to
`openshift/ovn-kubernetes` and `cluster-network-operator`.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- You can successfully bring up a dual-stack OVN-Kubernetes bare metal
  cluster. (In particular, you can do this via cluster-bot.)

- We have a mostly-passing periodic CI job

#### Tech Preview -> GA

- TBD

### Upgrade / Downgrade Strategy

Although we do not allow most networking-configuration-related changes
to be made on a deployed cluster, we know we will need to support
customers migrating clusters from single-stack to dual-stack.
We will only support migrating from a single-stack configuration to a
dual-stack configuration which is a proper superset of it. (That is,
you cannot migrate from single-stack to dual-stack and also change the
IPv4 cluster network CIDR at the same time.)

Although upstream is worrying about the case of enabling the
IPv6DualStack feature gate and changing the network configuration to
dual-stack at the same time (such that some nodes are trying to be
dual-stack and other nodes don't understand what dual-stack means),
that would not be a plausible situation in OCP. (It would not be
possible to configure dual stack networking until after CNO had been
updated to a version that supported it, so having skew like this would
imply the administrator had changed the network configuration *in the
middle of an upgrade*.)

Customers will have to first upgrade their cluster to a version of OCP
that supports dual-stack, and *then* change their networking
configuration to be dual-stack. If some early adopter wants to switch
to dual stack in an existing cluster immediately as soon as it is
available, they would likely have to do two back to back drain and
reboot cycles.

### Version Skew Strategy

After the Service `ipFamily` problem is fixed, there should not be any
interesting differences between an OCP cluster with `IPv6DualStack`
disabled and one with it enabled but with a single-stack networking
configuration.

Since older/current versions of CNO will not allow configuring a
dual-stack cluster, we should not have to worry about version skew in
dual-stack clusters (provided that the administrator does not try to
enable dual-stack during an upgrade).

## Implementation History
