---
title: static-ip-addresses-from-dhcp
authors:
  - "@cybertron"
reviewers:
  - "@celebdor"
  - "@bcrochet"
  - "@yboaron"
  - "@dcbw"
  - "@knobunc"
approvers:
  - "@shardy"
creation-date: 2020-10-28
last-updated: 2021-02-04
status: implemented
---

# Static IP Addresses from DHCP

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [X] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In some OpenShift deployments, particularly Edge deployments, DHCP may not
always be available for various reasons. To allow these deployments to
function if they are not able to access the DHCP server, we have a request
to configure the address(es) received from DHCP as static IPs on the
deployed nodes. This would only be done if the lease received has an
infinite length, so there is a reasonable expectation that it won't be
reassigned.

## Motivation

There are two primary use cases being addressed by this:

1. Deployers on baremetal don't want an outage of their DHCP server to cascade
   into an outage of their OpenShift cluster.

2. Edge deployments may not have access to a DHCP server due to the limited
   capacity in those environments.

### Goals

A DHCP server will initially be required for deployment, but after that the
cluster should function with or without the DHCP server.

### Non-Goals

Fully static deployments with no DHCP at any point. That will require
significantly larger architectural changes.

## Proposal

When we receive a DHCP lease with an infinite expiration time, create a static
configuration on the node that reflects the values provided by the DHCP server.

### User Stories [optional]

#### Story 1

As the administrator of a baremetal cluster, I do not want a DHCP server outage
to cascade into an outage of any other services.

#### Story 2

As the deployer of an Edge environment, my nodes may have limited or no
connectivity back to a central DHCP server. This should not affect their
functionality.

### Implementation Details/Notes/Constraints [optional]

We will add a NetworkManager dispatcher script that looks at each DHCP lease.
If the lease duration is infinite, the script will take the values provided
through DHCP and configure them statically on the node using nmcli. This
means the interface will not be configured via DHCP from then on, but since
we don't allow changing IPs on nodes anyway that shouldn't be a problem.

### Risks and Mitigations

If an address that was statically assigned in this way is reassigned by the
DHCP server, it could cause a conflict. There are two mitigations to this:
First, the requirement for the lease time to be infinite makes it unlikely
the server would reassign it in the course of normal operation, and second,
some DHCP servers have conflict detection that will prevent them from
reassigning an address that is active on the network.

Static configuration of the node's networking may make future changes more
difficult. In general we don't support changing the IP address of a node
so that isn't a concern at this time, but if routes or DNS search domains
are provided by DHCP it won't be possible to update those either after
initial deployment. Making such changes after deployment would require use
of a different mechanism.

There may be deployers who do not want their DHCP addresses configured
statically, but do want to use infinite DHCP leases. This could be worked
around by using extremely long leases that are not technically infinite,
but in practice would never expire.

## Design Details

### Open Questions

### Test Plan

This can be tested in virt environments by configuring libvirt to provide
infinite leases to the cluster nodes. This will be best implemented as a
periodic job, as it will need to test things like node reboots without the
DHCP server active, which would not be useful tests in most situations.
Additionally, the current test infrastructure only allows for setting the
lease time for all of the nodes at once, and since we need to test both
code paths, separate jobs will be required. Since this functionality has
few dependencies on other OpenShift components, it is likely not necessary
to run the infinite lease job on every proposed change.

### Graduation Criteria

This will be supported immediately.

### Upgrade / Downgrade Strategy

This behavior would largely have an effect at deployment time. For clusters
with existing DHCP addresses that are not infinite leases there will be no
change. A cluster with infinite DHCP leases would get them configured
statically on upgrade. After that the addresses would be static unless
the deployer made a change to the system configuration. If a cluster is
using infinite DHCP leases and does *not* want them to be statically
configured, the leases would need to be changed as discussed above.

On downgrade, the behavior would remain the same. If the nodes were using
DHCP before the downgrade, they will continue to. If they had been statically
configured, that will also be persistent.

### Version Skew Strategy

Version skew should not be a problem. If some nodes are configured with DHCP and
some with static addresses the end result should be the same. If it is not, that
is a bug to be fixed.

## Implementation History

Implemented in 4.7.

## Drawbacks

See the risks and mitigations section above. It addresses the concerns I am
aware of.

## Alternatives

In IPv6 deployments, SLAAC addressing could be used instead of DHCPv6 to provide
stable addresses without the need for a DHCP server. However, this does not help
with IPv4 deployments, and some deployers have existing DHCPv6 infrastructure they
would like to continue to use.
