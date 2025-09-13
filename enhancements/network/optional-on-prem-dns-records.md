---
title: optional-on-prem-dns-records
authors:
  - bnemec
reviewers:
  - mko # On-Prem Networking
  - everettraven # API
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

# Optional On-Prem DNS Records

## Summary

Allow deployers to disable the internal DNS records for api, api-int, and
ingress when external loadbalancers are used.

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

Add a configuration option for the internal DNS records so they can be disabled
when an external loadbalancer is in use. When this configuration is used, the
deployer will be wholly responsible for providing loadbalancer and DNS services
to the cluster.

Note that api and ingress are already required external records, so in reality
the only additional requirement here is api-int.

### Workflow Description

The deployer will configure external DNS and loadbalancing, similar to how
they would for a UPI deployment. They will then deploy an IPI cluster with
the loadbalancer set to UserManaged and DNS set to disabled.

### API Extensions

A new field will be added to control whether DNS records are deployed.

[Existing API](https://github.com/openshift/api/blob/674ad74beffcbdf6aa7a577bf23a269c24f92fe8/config/v1/types_infrastructure.go#L964)
[Existing Validations](https://github.com/openshift/installer/blob/97030df02861425054b980db72d31d36de1fcb20/pkg/types/validation/installconfig.go#L953)

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

An install-config snippet to use this new functionality would look like the
following:

```yaml
platform:
  baremetal:
    loadBalancer:
      type: UserManaged
    internalDNSRecords: Disabled
```

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

There is an alternative/variation we should consider here:

* Move these options to the networking object. The number of platforms using
  the internal loadbalancer and DNS infrastructure is increasing all the time,
  and at this point it is in use for more platforms than it isn't. Making these
  options platform-independent would significantly reduce the duplication
  across the different infrastructure objects.

  While this may not be strictly required for this specific feature, if we're
  going to move these options it should probably happen before we add more to
  the existing location as that will just make it more difficult to change
  later.

Other options were considered, but rejected for the following reasons:

* Add another type to the existing loadbalancer setting that would disable
  both the loadbalancer and DNS services. This was not favored because the
  DNS infrastructure is distinct from the loadbalancer and it would overload
  the meaning of the loadbalancer setting. Additionally, it could get quite
  messy if we add any more options to either the loadbalancer or DNS. We
  would have to enumerate every possible combination of options. With only
  two per service it wouldn't be too bad, but it could quickly escalate.

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

NA

## Test Plan

We will need to add the ability to configure api-int on the external DNS in
our external loadbalancer tests. Since we already have an api record, that
shouldn't be difficult.

## Graduation Criteria

We have a specific customer waiting for this feature who has agreed to test
it in tech preview. Graduation will be predicated on the feature addressing
their use case.

### Dev Preview -> Tech Preview

NA

### Tech Preview -> GA

Once the customer has verified that this addresses their use case it should be
ready for GA.

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

Currently this can only be set at install time, so it is not relevant for
upgrades and downgrades

## Version Skew Strategy

There will also be no version skew.

## Operational Aspects of API Extensions

Because they can only be set at deploy-time I don't expect a lot of impact here.
When debugging problems we'll need to look at the Infrastructure record to see
whether internal DNS records are in use.

## Support Procedures

DNS problems will now largely be the responsibility of the deployer rather
than the product. Support will be similar to UPI where both DNS and LB
services are managed outside the product.

## Infrastructure Needed [optional]

NA
