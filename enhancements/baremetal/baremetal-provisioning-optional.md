---
title: baremetal-provisioning-optional
authors:
  - "@stbenjam"
reviewers:
  - "@dhellman"
  - "@hardys"
  - "@kirankt"
  - "@sadasu"
approvers:
  - "@hardys"
creation-date: 2020-05-21
last-updated: 2020-06-23
status: provisional
see-also:
  - "/enhancements/baremetal/baremetal-provisioning-config.md"
  - "/enhancements/installer/connected-assisted-installer.md"
---

# Baremetal Optional Provisioning Network

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today the baremetal IPI platform takes a "batteries included" approach
that has a dedicated provisioning network, HTTP caches, DHCP and TFTP
servers, fully managed provisioning with both PXE and virtual media, and
some level of hardware management.

In some situations it may be possible to remove the requirement for a
provisioning network entirely, in particular in the case where
virtual media or the assisted install mechanism is used as the
provisioning method.

Previously, we added a flag to the installer that lets a user disable
DHCP, this enhancement builds on that by making a configurable
provisioning network profile to allow for managed, unmanaged, and
disabled configurations.

## Motivation

In order to reduce the number of boolean flags in the baremetal
platform, the various options for the provisioning network will be
consolidated into a single enum field.

### Goals

The goal of this enhancement is to allow further customization of a
baremetal IPI deployment to permit disabling the provisioning network
entirely.

### Non-Goals

We do not intend to remove the static provisioning IP's at this time,
but instead they will need to be IP's available in the external network.

## Proposal

A new `ProvisioningNetwork` option will be added to the baremetal
installer platform, that features an enum of possible values:

  - `managed` (default): Fully managed provisioning networking including
     DHCP, TFTP, etc.

  - `unmanaged`: Provisioning network is still present and used, but
     user is responsible for managing DHCP. Virtual media provisioning
     is reccomended, but PXE is still available if required.

  - `disabled`: Provisioning network is fully disabled. User may only do
     virtual media based provisioning, or bring up the cluster using
     assisted installation. If using power management, BMC's must be
     accessible from the machine networks. User must provide 2 IP's on
     the external network that are used for the provisioning services.

The same field will be added to the Provisioning CRD, with the
provisioningDHCPExternal field removed.

### Provisioning Services Matrix

| Container             | Managed       | Unmanaged     | Disabled                |
|-----------------------|---------------|---------------|-------------------------|
| baremetal-operator    | X             | X             | X                       |
| dnsmasq               | X             | TFTP Only     |                         |
| httpd                 | X             | X             | X                       |
| ironic-api            | X             | X             | X                       |
| ironic-conductor      | X             | X             | X                       |
| ironic-inspector      | X             | X             | X                       |
| machine-os-downloader | X             | X             | X                       |
| static-ip-manager     | X             | X             | X                       |

### User Stories

- As a user, I want to be able to disable the provisioning network using
  an installer provisioningNetwork field.

- As a user, I want to be able to configure external DHCP using the new
  profile or the older boolean flag, with the older flag being deprecated.
  See https://github.com/openshift/installer/pull/2829 for a deprecation
  example.

- As a user, I want the provisioningNetwork field from the installer to
  be propogated to the cluster Provisioning custom resource.

### Implementation Details/Notes/Constraints

Today, we rely on two IP's for bootstrap and the cluster to host the
Ironic services. Without the dedicated provisioning network, these will
need to be static IP's on the external network.

We may be able to use the API VIP, however, both provisioning
infrastructures in the bootstrap and cluster can (and often are) online
at the same time. That is to say, the third control plane member is
still provisioning while the machine-api-operator is already busy
provisioning worker nodes. To remove the two IP's, we would need to
constrain this and not bring up the provisioning infrastructure until
the third control plane member is online.

### Risks and Mitigations

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Upgrades will need to handle upgrading the Provisioning CR from the
previous version to the latest.

### Version Skew Strategy

MAO/CBO will need to understand both versions of the Provisioning CR,
and be able to look at the older provisioningDHCPExternal field.

## Implementation History


## Drawbacks

This introduces yet another possible configuration for baremetal IPI,
which further increases the potential differences between baremetal IPI
clusters, and makes it harder to support.

## Alternatives

We originally considered and rejected the idea that we could just leave
fields blank to indicate a particular feature should be turned off. For
example a blank provisioning network CIDR would indicate to disable the
entire network. A blank DHCP range would indicate you wanted to use
external DHCP. However, both of these fields have non-empty defaults
today.

The goal of the baremetal IPI initiative from the beginning was to offer
a "batteries included approach" -- the defaults should work for a
majority, or at the very least some large plurality, of users. Removing
default values and requiring users to specify them makes the platform
more difficult to use.

If the majority of users will not want a provisioning network, then we
could opt to make that change instead.
