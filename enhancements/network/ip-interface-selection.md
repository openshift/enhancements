---
title: ip-interface-selection
authors:
  - "@cybertron"
reviewers:
  - "@jcaamano"
  - "@tsorya"
  - "@flaper87"
approvers:
  - "@danwinship"
  - "@trozet"
api-approvers:
  - "None"
creation-date: 2022-07-07
last-updated: 2022-09-06
tracking-link:
  - https://issues.redhat.com/browse/OPNET-134
see-also:
  - https://github.com/openshift/baremetal-runtimecfg/issues/119
replaces:
superseded-by:
---

# Host IP and Interface Selection

## Summary

As OpenShift is deployed in increasingly complex networking environments, we
have gotten many requests for more control over which interface is used for
the primary node IP. We provided a basic mechanism for this with
[KUBELET_NODEIP_HINT](https://github.com/openshift/machine-config-operator/pull/2888)
but as users have started to exercise that some significant limitations have
come to light.

## Motivation

Some users want to have a great deal of control over how their network traffic
is routed. Because we use the default route for interface and IP selection,
in some cases they are not able to route traffic the way they want.

### User Stories

As a deployer, I want my cluster traffic to stay on an isolated network with
no external gateway. External traffic will travel on a different interface
that is managed by a service like MetalLB.

### Goals

Ensure that all host networked services on a node have consistent interface
and IP selection.

### Non-Goals

Support for platforms that do not use the nodeip-configuration service today.

Complete support for multiple nics, with full control over what traffic
gets routed where. However, this work should be able to serve as the basis
for a broader multi-nic feature so we should avoid designs that would limit
future work in this area.

## Proposal

The following is a possibly incomplete list of places that we do IP/interface
selection. Ideally these should all use a single mechanism to do that so they
are all consistent with each other.

- Node IP (Kubelet and CRIO)
- configure-ovs
- resolv-prepender
- Keepalived
- Etcd

In some cases (resolv-prepender) it may not matter if the IP selected is
consistent with the other cases, but Node IP, configure-ovs, and Keepalived all
need to match because their functionality depends on it. I'm less familiar with
the requirements for Etcd, but it seems likely that should match as well. In
general, it seems best if all IP selection logic comes from one place, whether
it's strictly required or not.

### Workflow Description

At deployment time the cluster administrator will include a manifest that sets
KUBELET_NODEIP_HINT appropriately. The nodeip-configuration service (which
will now be set as a dependency for all other services that need IP/interface
selection) will use that value to determine the desired IP and interface for
all services on the node. It will write the results of this selection to a
well-known location which the other services will consume. This way, we don't
need to duplicate the selection logic to multiple places. It will happen once
and be reused as necessary.

#### Example

- resolv-prepender has to run before any other node ip selection can take
  place. Without resolv.conf populated the nodeip-configuration service cannot
  pull the runtimecfg image.
  - Note: This is not relevant for UPI deployments.
- nodeip-configuration runs and selects one or more IPs. It writes them to
  the Kubelet and CRIO configuration files (this is the existing behavior).
- nodeip-configuration also writes the following files (new behavior):
  - /run/nodeip-configuration/primary-ip
  - /run/nodeip-configuration/ipv4
  - /run/nodeip-configuration/ipv6
- When configure-ovs runs, it looks for the primary-ip file and bridges the
  interface associated with that IP. It may also look at the ipv4 and ipv6
  addresses to determine which IP versions should be present on the bridge.
  If the nodeip-configuration files are not found, the logic will remain as
  it is today.
- When keepalived runs it will read the IP from the primary-ip file and use
  the associated interface for VRRP traffic.
  - Note: When [dual stack VIP support](https://github.com/openshift/enhancements/blob/master/enhancements/network/on-prem-dual-stack-vips.md)
    is implemented it will need to look at the appropriate ipvX file for each
    VIP. Until then, it should always use the primary IP.

#### Variation [optional]

Moved discussion of KUBELET_NODEIP_HINT to open questions.

### API Extensions

NA

### Implementation Details/Notes/Constraints [optional]

Currently configure-ovs runs before nodeip-configuration. In this design we
would need to reverse that. There are currently no dependencies between the
two services that would prevent such a change.

As noted above, we want to make sure we don't do anything that would further
complicate the implementation of a higher level multi-nic feature. The current
design should not be a problem for that. For example, if at some point we add
a feature allowing deployers to specify that they want cluster traffic on
eth0, external traffic on eth1, and storage traffic on eth2, that feature
would simply need to appropriately populate the KUBELET_NODEIP_HINT file that
would be directly created by the deployer in the current design. By providing
a common interface to configure host-networked services, this should actually
simplify any such future enhancements.

Initially, nodeip-configuration as also going to write an interface file so
services would not have to figure out the correct interface to use for a given
IP. However, this is problematic for use cases like OVNKubernetes because on
initial boot the IP will be on the interface itself, while on subsequent boots
the address will be on br-ex. Because the IP may move after
nodeip-configuration runs, we don't want to persist the interface.

### Risks and Mitigations

- There is some risk to changing the order of critical system services like
  nodeip-configuration and configure-ovs. This will not affect deployments
  that do not use OVNKubernetes as their CNI, but since we intend that to be
  the default going forward it is a significant concern.

  We intend to mitigate this risk by first merging the order change without
  any additional changes included in this design. This way, if any races
  between services are found once the change is being tested more broadly
  it will be easy to revert.

  We will also test the ordering change as much as possible before merging
  it, but it's unlikely we can exercise it to the same degree that running
  across hundreds of CI jobs per day will.

- Currently all host services are expected to listen on the same IP and
  interface. If at some point in the future we need host services listening
  on multiple different interfaces, this may not work. However, because we
  are centralizing all IP selection logic in nodeip-configuration, it should
  be possible to extend that to handle multiple interfaces if necessary.

### Drawbacks

This design only considers host networking, and it's likely that in the future
we will want a broader feature that provides an interface to configure pod
traffic routing as well. However, if/when such a feature is implemented it
should be able to use the same configuration interface for host services
that deployers would use directly after this is implemented.

Additionally, there are already ways to [implement traffic steering for pod
networking.](https://youtu.be/EpbUWwjadYM) We may at some point want to
integrate them more closely, but host networking is currently a much bigger
pain point and worth addressing on its own.

## Design Details

### Open Questions [optional]

- Currently this only applies to UPI clusters deployed using platform None.
  Do we need something similar for IPI?
  - Answer: Yes. In IPI deployments the VIP is always used for node ip
    selection. In an environment where the VIP subnet is isolated this could
    result in the node IP and the interface selected by configure-ovs not
    matching.

- In UPI deployments it is also possible to set the Node IP by manually writing
  configuration files for Kubelet and CRIO. Trying to look for all possible
  ways a user may have configured those services seems complex and error-prone.
  Can we just require them to use this mechanism if they want custom IP
  selection?
  - Answer: We probably don't need an answer before implementing this. The
    changes described here shouldn't affect any existing deployments, it will
    only allow some deployments that are currently having issues the the ip
    selection behavior to be successful. It's probably reasonable to require
    anyone who is struggling with this to use our interface though.

- The interface file written by nodeip-configuration will no longer be valid
  once configure-ovs moves the IP to br-ex. Should we re-run
  nodeip-configuration at that point to update it, should be leave the
  interface file alone, or should we just not write the interface at all and
  use the IP address(es) exclusively for interface selection?
  - Answer: Just don't persist the interface name. As long the IP used to get
    the interface is consistent between services we should get the behavior
    we need. Also, this option is more flexible for future features, like the
    ability to run dual stack clusters with ipv4 and ipv6 on different
    interfaces.

- We may want to rename KUBELET_NODEIP_HINT to reflect the fact that it will
  now affect more than just Kubelet. Is just NODEIP_HINT acceptable? How do we
  handle backward compatibility with the KUBELET version of the name to avoid
  breaking existing users of that functionality? I'm not sure how much logic
  we can inject in the systemd service.
  - Answer: NODEIP_HINT should be fine. Even the existing KUBELET_NODEIP_HINT
    is taken from a [file named nodeip-configuration](https://github.com/openshift/machine-config-operator/blob/3cf95882fa8e967d9dca1495a5dcde659d75ba46/templates/common/_base/units/nodeip-configuration.service.yaml#L39)
    We can maintain compatibility using bash variable magic, e.g.
    `${NODEIP_HINT:-$KUBELET_NODEIP_HINT}`.

### Test Plan

In general this will be covered by existing e2e tests. However, we should
expand coverage of the nodeip-configuration and configure-ovs components as
they have proven to have a lot of edge cases.

At this point I'm not sure exactly what a more-targeted set of tests for
host networking would look like though. We likely cannot stand up the large
variety of networking environments needed to run e2e tests with all of the
possible architectures, so we need something more like a unit or functional
test.

Given that this work will not make the testing situation any worse, it's
possible we could defer the testing improvements to followup work.

### Graduation Criteria

NA

#### Dev Preview -> Tech Preview

NA

#### Tech Preview -> GA

NA

#### Removing a deprecated feature

NA

### Upgrade / Downgrade Strategy

NA

### Version Skew Strategy

From version to version the selection process must remain consistent in order
to avoid IPs and interfaces changing. As a result, version skew should not be
a problem.

### Operational Aspects of API Extensions

NA

#### Failure Modes

NA

#### Support Procedures

This should not drastically affect support. It will still be possible to
determine what address/interface a service is using by looking at its config
and/or logs. The only change will be where that comes from and that the new
files can also be consulted to check the behavior of nodeip-configuration.

## Implementation History

KUBELET_NODEIP_HINT was implemented in 4.11 and backported to older releases
to improve the user experience where they needed to override the default logic
in nodeip-configuration.

## Alternatives

* Leave things basically as they are, but teach services like configure-ovs and
  keepalived to understand KUBELET_NODEIP_HINT so they all select the same IP
  and interface. This would result in some level of duplicated logic and likely
  lead to more problems down the road.

* Instead of making the ip selection logic smarter, document a mechanism for
  users to be explicit about the desired network layout. In theory this is
  already possible, but it only helps with configure-ovs. It's still possible
  other services may select incorrect ips. This is also a rather complex way
  to support specifying an interface for configure-ovs.

## Infrastructure Needed [optional]

NA
