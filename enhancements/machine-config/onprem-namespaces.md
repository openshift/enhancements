---
title: namespaces-for-on-prem-networking-services
authors:
  - "@cybertron"
reviewers:
  - "@smarterclayton"
  - "@celebdor"
  - "@mandre"
  - "@rgolangh"
  - "@patrickdillon"
  - "@kikisdeliveryservice"
approvers:
  - Someone from MCO
  - Someone from architecture
creation-date: 2020-07-24
last-updated: 2020-07-27
status: implementable
see-also:
replaces:
superseded-by:
---

# Namespaces for On-Prem Networking Services

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

When support for on-premise deployments on baremetal, OpenStack, OVirt, and
VSphere was added, some additional networking services were needed to provide
functionality that would come from cloud services on other platforms. These
services were added in namespaces called openshift-kni-infra,
openshift-openstack-infra, etc. There are two problems with this: 1) The
namespaces are created in all deployments, including on platforms other
than the one that uses the namespace and 2) The openshift-x-infra name scheme
does not match the existing pattern for OpenShift namespaces.

## Motivation

The existence of namespaces not related to a given deployment is confusing to
users. Additionally, the inconsistent naming of the namespaces is a further
possible point of confusion.

### Goals

- Only create namespaces for networking services on deployments that need them.
- Use names that are consistent with the rest of OpenShift.

### Non-Goals

## Proposal

Instead of creating the namespaces at install time as is currently done, add
them to the list of
[resources to be synced](https://github.com/openshift/machine-config-operator/blob/master/pkg/operator/sync.go)
in the machine-config-operator. This will allow us to add logic that considers
the platform before creating them.

For the naming, we propose `openshift-hosted-network-services` for all
platforms that use these components. There is no need for a platform-specific
name since only one platform can exist at a time anyway.

### User Stories [optional]

### Implementation Details/Notes/Constraints [optional]

The services in question provide things like internal DNS resolution between
nodes and loadbalancing for API and ingress traffic. In cloud platforms these
would normally be provided by a service in the cloud, but for baremetal and
our other on-premise platforms there are no such services available.

### Risks and Mitigations

The only significant risk should be the migration from the existing names
to the new ones. We are already creating namespaces for these services.
This proposal only changes how we do that and what those namespaces are
called.

## Design Details

### Open Questions [optional]

Is `openshift-hosted-network-services` an acceptable name? There has also been
discussion of calling it `openshift-machine-config-something` since it will be
deployed by the MCO, but since these components are not part of MCO that feels
like an odd fit.

### Test Plan

This would be covered by the same tests as the existing namespaces. Each
platform has an end-to-end test against MCO that will exercise the new
functionality.

### Graduation Criteria

This isn't a feature so graduation is not relevant. It's just fixing an
issue with the existing namespace handling.

### Upgrade / Downgrade Strategy

On upgrade, we will want to clean up the old namespace after moving the static
pods to the new one.

Downgrade may be tricky. The old version of MCO won't have logic to re-create
the old namespace, so I'm unsure how that will work.

### Version Skew Strategy

Version skew should not be a major concern. The pods may be running in
different namespaces for a period of time, but that shouldn't affect their
functionality.

## Implementation History

None

## Drawbacks

Changing how the namespaces work will complicate deployment of on-premise
services. The existing implementation works, despite the flaws.

## Alternatives

It's possible the installer could do something with these namespaces. It will
also have the necessary configuration data to correctly configure the new
namespace. Since MCO is responsible for deploying these services, it most
likely makes more sense for it to own the namespace creation as well.

