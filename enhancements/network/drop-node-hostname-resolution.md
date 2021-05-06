---
title: drop-requirement-for-node-hostname-resolution
authors:
  - "@cybertron"
reviewers:
  - @yboaron
  - TBD
approvers:
  - TBD
creation-date: 2021-05-05
last-updated: 2021-05-05
status: implementable
see-also:
  - "/enhancements/network/baremetal-networking.md"
  - https://github.com/openshift/enhancements/pull/654
---

# Drop Requirement for Node Hostname Resolution

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Operational readiness criteria is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

When the initial baremetal IPI implementation was done, there was a requirement
for node hostnames to be resolvable from other nodes. Since that time there
have been changes in how bootstrapping happens (among other things) that seem
to have removed this requirement. In order to provide node name resolution,
baremetal IPI runs an independent coredns pod on each node that retrieves a
list of nodes and provides DNS records for them. Some deployments, particularly
resource-limited edge environments, do not want this additional service running
on their nodes. We are proposing to remove the requirement for node hostnames
to be resolvable so we can remove this service.

## Motivation

OpenShift nodes may be deployed in heavily resource-constrained environments.
In these cases, deployers want to limit the number of services consuming
resources. One such service is the coredns instance used to provide resolution
of node hostnames. If we do not need that resolution, then we can remove the
coredns service and save resources.

### Goals

Verify that node hostname resolution is not required so we can remove the
coredns instance deployed in on-prem IPI environments.

### Non-Goals

N/A

## Proposal

### User Stories

#### Story 1

As a deployer of OpenShift in Edge environments, I want the cluster to use as
few resources as possible so my applications have the maximum possible capacity
available to them.

### Implementation Details/Notes/Constraints [optional]

We've done deployments without node name resolution and everything we've
tested has worked correctly. This proposal is mostly to make sure we haven't
missed anything and to get general agreement that there won't be a future
feature that relies on being able to resolve nodes by name.

Even if we drop resolution of node hostnames, one record from coredns will
still need to be provided: api-int. After this change that record will be
provided by a dnsmasq service running on the host. While this does mean there
will still be a local DNS server, the dnsmasq implementation should be
significantly less resource intensive. In my local testing, a simple dnsmasq
instance uses around an order of magnitude less memory than a simple coredns
instance. Since we won't need some of the more advanced features of coredns
that we currently use, it makes sense to use the lighter weight service.

This proposal is strongly related to the
[ARO private DNS zone resource removal](https://github.com/openshift/enhancements/pull/654)
enhancement proposal. The motivations are different, which is the reason this
enhancement was written, but the end result is the same.

Also note that the associated baremetal-networking enhancement discusses the
use of mDNS to provide hostname resolution. This was recently removed and will
need to be updated regardless of the outcome of this discussion. For the
purposes of this enhancement, the main thing to consider is that coredns is
providing hostname resolution and using compute resources on the nodes to do
so. The specific mechanism it uses is not necessarily important.

### Risks and Mitigations

* There may be a component in OpenShift that relies on node hostname resolution
  that we haven't found. This proposal is the mitigation for that - we hope
  that by getting more eyes on the change any such oversights will be
  caught. Also, this is a fairly self-contained change so if we had to
  revert it because a problem was found that should not be an issue.

* The cloud platforms tend to get node name resolution for free from the
  cloud. It's possible a future change could introduce a dependency on that
  which would break if on-prem platforms are not providing the same
  functionality. This proposal is also the mitigation for that. If we have
  general agreement that node name resolution cannot be assumed then future
  designs will need to take that into account. We also have on-prem ci jobs
  running that would quickly catch any such problems.

* A deployer may have third-party services in their cluster that depend on
  node name resolution. It's debatable whether our internal DNS configuration
  should be considered a public interface, and in such use cases the name
  resolution could be provided by external DNS. Some baremetal IPI deployments
  are already doing that.

## Design Details

### Open Questions [optional]

No one I have talked to is aware of any current requirement for node hostname
resolution, but no one has been able to say that there definitely isn't one
either. Is there anyone who could give us a definitive answer on this?

### Test Plan

As this would become the default behavior for on-prem IPI deployments, it
would be covered by existing tests. The full test suite should continue to
pass after this change is made. This introduces no new behavior that would
need to be tested.

### Graduation Criteria

This is not strictly a feature and wouldn't go through a graduation process.
It's more of an RFC to confirm that we haven't missed anything. Once node
name resolution is removed this will become the supported configuration for
on-prem IPI deployments. Ideally, the change should be invisible to users,
so there is no need for a dev or tech preview phase.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

On upgrade, the coredns static pod will be removed and replaced with an
instance of dnsmasq running on the host that will only provide the api-int
record. We will need to resolve port conflicts between coredns and dnsmasq
via either ordering or retries. As part of that, we will need to ensure that
when the coredns pod is deleted it doesn't break the upgrade in such a way
that the dnsmasq service cannot start and take over DNS responsibilities.

On downgrade, the coredns pod would be recreated. It may be necessary to
manually stop dnsmasq to allow coredns to start. I am not aware of any
functionality in machine-config-operator that would allow us to automatically
stop a service on downgrade.

### Version Skew Strategy

This component does not interact with anything outside of the node it is
running on. If separate nodes end up at different levels, the only difference
will be what is providing internal DNS to a given node. It should not be a
problem to have one node at one version and another at a different version.

## Implementation History

## Drawbacks

As noted earlier, removing node hostname resolution from on-prem platforms
will introduce a difference in behavior from the cloud platforms. Since
efforts have been made to avoid reliance on DNS I don't see this as a major
problem, but it is less than ideal.

## Alternatives

* Keep node hostname resolution as it is today. This eliminates the skew from
on-prem to cloud platforms, but it requires cluster services to consume more
resources in edge deployments.

* Split the api-int and node hostname resolution so they can be managed
independently of one another. This would potentially allow a deployer to
disable node hostname resolution if they want to reclaim those resources, but
keep the needed api-int resolution. I don't favor this because it would require
running two services providing DNS in a regular cluster, and if node hostname
resolution is something that can be disabled without loss of functionality I
don't see any reason to have it enabled by default. The only way I could see
pursuing this option is if there _is_ a loss of some non-critical functionality
when disabling node hostname resolution that a deployer could choose to accept
in the interest of reducing cluster resource usage.
