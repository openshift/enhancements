---
title: AWS-IPv6-Dev-Env
authors:
  - "@russellb"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2019-10-24
last-updated: 2019-10-24
status: implementable
---

# aws-ipv6-dev-env

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

While Kubernetes itself includes support for IPv6, the OKD distribution does
not fully support it yet.  To enable developers across the many components that
make up OKD, this proposal is to add support for enabling IPv6 for an AWS
cluster.  This will also be used to create a CI job that can exercise IPv6
support across OKD.

## Motivation

Supporting IPv6 across OKD will require collaboration and fixes across many
components.  Many developers are already used to doing development against AWS
clusters, so supporting AWS as an IPv6 development environment should make it
easier for people to jump in to help debug and fix IPv6 issues.

### Goals

* Make creating an IPv6 development environment as easy as creating an AWS
  cluster, by just setting one additional option to turn it on.
* Support OKD in reaching a milestone of supporting a single stack IPv6 control
  plane (IPv6 only).

### Non-Goals

* Reaching full support for IPv6 on AWS.  While the work will get us closer,
  that is not the primary goal here.  The primary goal is to help make it
  easier for all teams to exercise IPv6.

## Proposal

To enable IPv6 in your AWS environment, a developer would set the following
environment variable before running the installer:

```bash
    export OPENSHIFT_INSTALL_AWS_USE_IPV6=”true”
```

This would be off by default.

### AWS Network Environment

AWS does not support a single-stack (IPv6 only) environment, but it does
support a dual-stack (IPv4 and IPv6) environment, so that’s what is enabled
here.  This is a summary of the changes to the network environment:

* The VPC has IPv6 enabled and a `/56` IPv6 CIDR will be allocated by AWS.
* Each Subnet will have an IPv6 `/64` subnet allocated to it.
* All IPv4 specific security group rules have corresponding IPv6 rules created.
* AWS Network Load Balancers (NLBs) do not support IPv6, so external API access
  is still over IPv4.  AWS does not have a TCP load balancer that supports
  IPv6, other than classic load balancers with EC2-Classic, and not EC2-VPC.
  AWS Application Load Balancers supposedly support IPv6, but that would
  require doing HTTPS load balancing for the API instead of just TCP load
  balancing, so we just use the IPv4 NLBs.  API access within the cluster is
  still exercising IPv6 when using its Service IP..
* IPv6 DNS records (AAAA) are created and the IPv4 (A) records are disabled,
  except for the API since the API is still accessed via an IPv4 only load
  balancer.
* IPv6 routing is configured.  Since all instances get global IPv6 addresses,
  NAT is not used from the instances out to the internet.  The current
  implementation uses security groups to block incoming traffic sent directly
  to any of the instances, but will move to using an egress-only internet
  gateway which will make this isolation more explicit.

### Node Addresses

Each AWS instance will receive both a private IPv4 address and a globally
routeable IPv6 address.

Kubelet is configured to use the IPv6 address for the Node object.

etcd and all other services running with host networking will be configured to
use the IPv6 address.

### Hack for IPv4 Access Where Necessary

There are some pods that still require IPv4 access on AWS to be functional.
For example, the CoreDNS pods must have IPv4 connectivity since the AWS DNS
server is only available via IPv4.  This also means we have to add a security
group rule allowing DNS traffic to our CoreDNS pods over the AWS network (they
use port 5353).

Another case where this hack is required is several pods that need to access
AWS APIs.  The AWS APIs are IPv4-only.

Since this is an AWS-IPv6 specific hack, it is currently centralized into one
place: ovn-kubernetes.  It will automatically add a second interface with IPv4
access to the set of affected pods.

### Install Configuration

Here is the suggested network configuration for `install-config.yaml`:

```yaml
networking:
  clusterNetwork:
  - cidr: fd01::/48
    hostPrefix: 64
  machineCIDR: 10.0.0.0/16
  networkType: OVNKubernetes
  serviceNetwork:
  - fd02::/112
```

Note that an IPv4 CIDR is still used for `machineCIDR` since AWS will provide a
dual-stack (IPv4 and IPv6) environment.  We must specify the IPv4 CIDR and AWS
will automatically allocate an IPv6 CIDR.

`OVNKubernetes` is the only `networkType` supported in this environment.

### User Stories

#### Story 1

As an OKD developer working on an arbitrary component and with little knowledge
about how to set up IPv6, I would like to easily create an IPv6 development
environment to debug, test, and develop IPv6 support in my component.

### Implementation Details/Notes/Constraints

AWS does not support IPv6 only.  It requires IPv4 and provides the option of
adding IPv6.  We will create a dual-stack environment, but disable IPv4
resources over time to exercise more and more of the IPv6 control plane for
OKD.

We can never block IPv4 completely on AWS.  At a minimum, IPv4 is required for
the AWS instances to reach the EC2 metadata API.

### Risks and Mitigations

It is important to not risk destabilizing normal AWS cluster support.  To
mitigate this risk, all of the IPv6 related additions will be off by default.

## Design Details

### Test Plan

A copy of an existing AWS CI job, but with the IPv6 control plane enabled, will
be created once enough IPv6 support is in place for a meaningful CI job to run.

### Graduation Criteria

Not applicable, since this is targeted at development only at the moment.

### Upgrade / Downgrade Strategy

Not applicable, since this is targeted at development only at the moment.

### Version Skew Strategy

Not applicable, since this is targeted at development only at the moment.

## Implementation History

A PR with an implementation and documentation can be found here:

* <https://github.com/openshift/installer/pull/2555>

## Drawbacks

The most important primary target for IPv6 is actually bare metal, so one could
argue that we should be focused on bare metal dev environments instead.  The
idea behind using AWS is to make it more immediately accessible to a larger
number of developers, including those that do not have the local resources to
set up a bare metal cluster.

## Alternatives

One alternative would be to focus on bare metal (or VMs simulating bare metal).
That still requires at least one large bare metal host capable of running
enough VMs for a cluster.
