---
title: optional-vips-with-extlb
authors:
  - bnemec
reviewers:
  - mko # On-Prem Networking
approvers:
  - knobunc
api-approvers:
  - JoelSpeed
creation-date: 2025-05-21
last-updated: 2025-05-21
tracking-link:
  - https://issues.redhat.com/browse/OPNET-678
see-also:
  - "/enhancements/network/external-lb-vips.md"
replaces:
  - NA
superseded-by:
  - NA
---

# Optional VIPs with External Loadbalancer

## Summary

Stop requiring VIP configuration when an external loadbalancer is in use.
This will allow external loadbalancer customers to use their own DNS as well,
effectively disabling most of the internal DNS and LB infrastructure.

## Motivation

Previously when an external loadbalancer was configured we still used the VIP
fields of install-config to create DNS records in the internal DNS
infrastructure pointing at the external loadbalancer. However, some more
advanced loadbalancers also use DNS loadbalancing to further distribute
traffic. This does not work if we hard-code a single loadbalancer endpoint
into our internal DNS.

### User Stories

* As an OpenShift deployer I want to use an advanced external loadbalancer
  to distribute traffic to my cluster. This loadbalancer uses multiple DNS
  targets in order to reduce load on a single endpoint.

### Goals

Make it possible to disable the internal DNS records for api, api-int, and
ingress in on-prem IPI deployments to allow use of external DNS to manage
those, even for internal cluster traffic.

### Non-Goals

Disabling the internal DNS infrastructure entirely. There are two reasons
we're not doing this:

* It also provides records for each host in the cluster, although it's not
  clear this is actually necessary anymore.
* It would require significant re-working of the resolv-prepender behavior,
  and that component has historically been rather fragile. Adding complexity
  in that area is more likely to cause problems and would run counter to past
  efforts to simplify it.

## Proposal

When an external loadbalancer is configured we will stop requiring VIPs to be
specified for creating DNS records. If VIPs are not provided, we will not
configure any DNS records for the api, api-int, and ingress addresses. The
deployer will then be wholly responsible for providing those records.

Note that api and ingress are already required external records, so in reality
the only addition here is api-int.

### Workflow Description

The deployer will configure external DNS and loadbalancing, similar to how
they would for a UPI deployment. They will then deploy an IPI cluster with
the loadbalancer set to UserManaged and no VIPs configured.

### API Extensions

The VIP fields will no longer be mandatory for all on-prem IPI deployments.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No Hypershift impacts

#### Standalone Clusters

Yes, this is expected to be used primarily with standalone clusters.

#### Single-node Deployments or MicroShift

SNO is not supported for on-prem IPI deployments, so no impacts there.

I don't believe this will impact MicroShift either, but even if it did it would
reduce resources requirements slightly.

### Implementation Details/Notes/Constraints

A configuration to use this new functionality would look like the following:

```yaml
platform:
  baremetal:
    loadBalancer:
      type: UserManaged
```

Note the lack of the apiVIPs and ingressVIPs fields, which previously would
have been mandatory with this configuration.

### Risks and Mitigations

Not having internal DNS records for mandatory names slightly increases the
chances of misconfiguration, but as most of these records are already
required in the external DNS infrastructure it's not particularly concerning.

### Drawbacks

It will add complexity to the coredns configuration for on-prem IPI, but less
than any other option we've come up with. It is also yet another possible
variation in the networking configuration for on-prem deployments. That matrix
is already impossibly large so it won't move the needle noticeably though.

## Alternatives (Not Implemented)

Two other options were considered, but rejected for the following reasons:

* Support multiple VIPs in the coredns configuration. This would have
  significantly increased the complexity of our coredns configuration
  and would have introduced the possibility of variance between how DNS
  resolves internally or externally. Additionally, multiple backends
  behind a single DNS name is already supported by the external infrastructure
  so at best we'd be duplicating existing functionality.

* Remove coredns completely. This has a few problems. As noted earlier, it
  would require changing the resolv-prepender service to handle the absence
  of a local DNS server, and that's a much riskier change. It would also
  eliminate name-based resolution between nodes.

## Open Questions [optional]

How will the validation for these fields work? Currently the VIP fields are
always required, but with this feature they will only be mandatory if the
external loadbalancer option is not set. I don't have enough experience with
the API to know how that will work. Worst-case, we can add a validation to
the installer though.

How does this interact with the existing VSphere UPI configuration? The VIPs
are already optional on that platform (which is how UPI is deployed), so we
may need to rework some logic around that to account for the loadbalancer
parameter as well. Alternatively we can just not support this on VSphere,
but that may be undesirable.

## Test Plan

We will need to add the ability to configure api-int on the external DNS in
our external loadbalancer tests. Since we already have an api record, that
shouldn't be difficult.

## Graduation Criteria

This is a variation on an existing supported deployment scenario, so we don't
plan to pursue a graduation process. It will be GA from initial implementation.

### Dev Preview -> Tech Preview

NA

### Tech Preview -> GA

NA

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

Currently this can only be set at install time, so it is not relevant for
upgrades and downgrades

## Version Skew Strategy

There will also be no version skew.

## Operational Aspects of API Extensions

It will now be possible for the VIP fields in the on-prem Infrastructure
record to be empty. We largely already handle this because of VSphere
UPI, but it may now happen in circumstances where it previously wouldn't.

## Support Procedures

DNS problems will now largely be the responsibility of the deployer rather
than the product. Support will be similar to UPI where both DNS and LB
services are managed outside the product.

## Infrastructure Needed [optional]

NA
