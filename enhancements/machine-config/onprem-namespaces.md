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
last-updated: 2020-08-21
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

The services in question provide things like internal DNS resolution between
nodes and loadbalancing for API and ingress traffic. In cloud platforms these
would normally be provided by a service in the cloud, but for baremetal and
our other on-premise platforms there are no such services available.

### Goals

- Only create namespaces for networking services on deployments that need them.
- Use names that are consistent with the rest of OpenShift.

### Non-Goals

- Splitting the hosted network services into a separate operator. If we decide
  to go that direction it will require a separate enhancement.

## Proposal

Instead of creating the namespaces at install time as is currently done, add
them to the list of
[resources to be synced](https://github.com/openshift/machine-config-operator/blob/master/pkg/operator/sync.go)
in the machine-config-operator. This will allow us to add logic that considers
the platform before creating them. Additionally, we will need code to clean up
the old namespaces on all platforms. The deletion may be delayed by a cycle.
See the upgrade/downgrade section for more details on that.

For the naming, we propose `openshift-hosted-network-services` for all
platforms that use these components. There is no need for a platform-specific
name since only one platform can exist at a time anyway.

### User Stories [optional]

### Implementation Details/Notes/Constraints [optional]

On initial deployment, the static pods will run without a namespace for a
brief period of time until MCO has a chance to create the namespace. The
pods can actually run without a namespace indefinitely, but this causes
problems with resource tracking because the pods do not show up in the
API until the namespace exists. However, MCO should be able to create the
namespace early enough to avoid any resource issues.

On upgrade, new manifests for the static pods will be written by MCO and
kubelet will delete the old pod and create a new one using the new manifest
definition. Since the namespace is irrelevant to the functioning of the
static pods themselves, it won't matter if some pods are running in the old
namespace and some in the new for a brief period of time. Keepalived and
HAProxy should route traffic to avoid any outages that happen as a result of
this process.

We will also want to clean up the old namespaces. This can be done
unconditionally since the old namespaces will exist on all platforms and we
want to remove them everywhere. I think we should leave the old namespaces
for a release so rollback is possible in case of problems after an upgrade.
Once the old namespaces are removed I don't believe it will be possible to
safely downgrade as there will be nothing to recreate the old namespaces.

### Risks and Mitigations

Moving things around is always somewhat risky. I don't anticipate any
difficulty with that given the very limited interaction between our pods
and the namespace, but it's always possible there's a dependency I'm not
aware of.

This would also change the timing of when the namespace is created relative
to when the pods using the namespace are. I believe this is okay based on
this
[commit message](https://github.com/openshift/machine-config-operator/commit/b3ab7d8cfdbdd717bfefee62a56c6a1d0b03d657)
which suggests we were previously running these services without any namespace
at all and the only problem was that the resource usage by the pods was
not tracked correctly. In this proposal the pods would be running briefly
without a namespace until MCO has a chance to create it.

Once the old namespaces are removed, it will no longer be possible to downgrade
to a release that uses them, at least not without manual recreation of the
necessary namespace. We can mitigate that by leaving the old namespaces for a
release so rollback is possible.

## Design Details

### Open Questions [optional]

- Is `openshift-hosted-network-services` an acceptable name? There has also
been discussion of calling it `openshift-machine-config-something` since it
will be deployed by the MCO, but since these components are not part of MCO
that feels like an odd fit.

- Is a one release downgrade strategy sufficient? For example, 4.6 upgraded to
4.7 then downgraded to 4.6. I think we could make that work, but I'm not sure
other scenarios are possible.

### Test Plan

This would be covered by the same tests as the existing namespaces. Each
platform has an end-to-end test against MCO that will exercise the new
functionality.

Upgrades on baremetal are currently handled using one-off testing. It would
be good to investigate an automated upgrade job for baremetal and the other
platforms affected by this change.

### Graduation Criteria

This isn't a feature so graduation is not relevant. It's just fixing an
issue with the existing namespace handling.

### Upgrade / Downgrade Strategy

On upgrade, we will want to clean up the old namespace after moving the static
pods to the new one. Migration of the pods from old to new should happen
when MCO writes the new manifests pointing at the new namespace. Kubelet will
delete the old pods and start new ones based on the new manifest.

Downgrade may be tricky. The old version of MCO won't have logic to re-create
the old namespace, so I'm unsure how that will work.

One option to support some amount of downgrading would be to leave the old
namespaces for a release. That would allow a downgrade to happen in the
event of a problematic initial upgrade.

It's also possible we could just document that the namespaces need to be
recreated manually on downgrade. The static pods will still function anyway,
but they won't show up in the API or have their resource usage tracked
correctly until the namespace exists.

### Version Skew Strategy

Version skew should not be a major concern. The pods on different nodes
may be running in different namespaces for a period of time, but that
shouldn't affect their functionality.

## Implementation History

None

## Drawbacks

Changing how the namespaces work will complicate deployment of on-premise
services. The existing implementation works, despite the flaws.

## Alternatives

It's possible the installer could do something with these namespaces. It will
also have the necessary configuration data to correctly configure the new
namespace.

However, the installer doesn't have a good way (that I know of anyway) to deal
with the migration path on upgrade. Since MCO is responsible for deploying
these services, it makes more sense for it to own the namespace creation as
well.

