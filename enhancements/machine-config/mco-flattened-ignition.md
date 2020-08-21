---
title: MCO Support flattened ignition config
authors:
  - "@hardys"
  - "@celebdor"
reviewers:
  - "@celebdor"
  - "@cgwalters"
  - "@crawford"
  - "@kirankt"
  - "@runcom"
  - "@stbenjam"
approvers:
  - "@cgwalters"
  - "@crawford"
  - "@runcom"

creation-date: 2020-08-21
last-updated: 2020-09-11
status: implementable
---

# MCO Support flattened ignition config

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes adding an interface to the MCO such that a flattened
ignition config, that is a combination of the managed pointer config and the
MCO rendered config (inlined via a [data URL](https://tools.ietf.org/html/rfc2397))
is available.

## Motivation

In some situations it is necessary to perform network configuration before it
is possible to download the rendered config from the MCS in the ramdisk where
ignition evaluates the pointer config.

This leads to a chicken-egg problem, where a user could configure the network
via ignition, but ignition cannot apply any config until it has downloaded
any append/merge directives.

This situation is quite common in baremetal environments, e.g a bonded pair of nics
(where the native vlan is often used for PXE provisioning),
with the controlplane network configured via a VLAN on the same physical interface.

For UPI deployments, it may be possible to work around this situation when
using coreos-install following the [enhancements around static networking](https://github.com/openshift/enhancements/blob/master/enhancements/rhcos/static-networking-enhancements.md), but it may be a more consistent user
experience if all configuration was possible via ignition.

For baremetal IPI deployments currently the only workaround is to customize the
OS image, since the deploy toolchain currently does not use coreos-install,
or include any interface to drop additional files into /boot during deployment.

### Goals

 * Provide a solution to customer requirements for baremetal IPI around bond/VLAN
   and other configurations for controlplane networking
 * Avoid a platform-specific approach, which could be fragile and/or hard to maintain

### Non-Goals

The discussion of interfaces here only considers the existing interfaces, not
proposals around a future [declarative network configuration](https://github.com/openshift/enhancements/pull/399)
although we need to ensure the approach taken in each case is aligned.

### User Stories

#### Story 1

As a UPI user, I would like to perform network configuration in the same
way as other customization, e.g via MachineConfig manifests or ignition.

#### Story 2

As a baremetal IPI user, I need to deploy in an environment where network
configuration is required before access to the controlplane network is possible.

Specifically I wish to deploy in an environment where a bonded pair of nics
are used for the controlplane, on a non-default VLAN (a common configuration
for baremetal where resilience is required but you want to minimise the total
number of NICs needed).

## Design Details

The MCO currently maintains two ignition artifacts, a managed instance of the
pointer ignition config, stored in a Secret, and the rendered config stored
in a MachineConfig object (both per role, not per machine).

Only the rendered config is accessible via the MCS (although not currently from within
the cluster ref https://github.com/openshift/machine-config-operator/issues/1690)
so they may be consumed during deployment.

The proposal is to create a third managed artifact, which is the flattened
combination of the pointer and rendered ignition configs, so that when
required it may be consumed instead of requiring runtime download of the rendered
config via the pointer URL.

This additional artifact will be an additional MachineConfig resource, that is
regenerated every time the existing rendered config is updated by the machine
config render controller.  It will contain a combination of the pointer ignition
and the rendered config.

This new resource will be made available via a new API path in the MCS e.g
`config/master/flattened` (or alternatively a query string e.g `config/master?flattened=true`)

To work around the issue that the MCS is not accessible from the the cluster for
the baremetal machine-api actuator, that component will retrieve the rendered
config directly via the k8s API instead (TODO verify the RBAC for the machine
controller will permit this due to different namespaces).

### Test Plan

  * Ensure no regressions in existing deployments via existing e2e-metal-ipi CI coverage
  * Prove we can do deployments where the controlplane network requires configuration, such as that described in [previous bug reports](https://bugzilla.redhat.com/show_bug.cgi?id=1824331) - we can add a periodic CI job based on e2e-metal-ipi to test this.


### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Upgrade / Downgrade Strategy

This change only impacts the day1 deployment, so that deployment becomes
possible without workarounds, so there should be no upgrade impact.

### Version Skew Strategy

## Alternatives

### Native ignition support for early-network config

This was initially proposed via [an ignition issue](https://github.com/coreos/ignition/issues/979)
where we had some good discussion, but ultimately IIUC the idea of adding a new
interface for early network configuration to ignition was rejected.

### Implement config flattening in platform-specific repos

The ignition configuration is consumed at two points during deployment for IPI baremetal
deployments:

  * Via terraform (when the installer deploys the controlplane
    nodes using [terraform-provider-ironic](https://github.com/openshift-metal3/terraform-provider-ironic).
  * Workers are then deployed via the machine-api, using a
    [baremetal Cluster API Provider](https://github.com/openshift/cluster-api-provider-baremetal/)

The ironic terraform provider is designed to be generic, and thus we would prefer
not to add OS specific handling of the user-data there, and in addition [previous
prototying of a new provider](https://github.com/openshift-metal3/terraform-provider-openshift)
indicates that due to terraform limitations it is not possible to
pass the data between providers/resources in the way that would be required.

### Add support for injecting NM config to IPI

This might be possible using a recently added [ironic python agent feature](https://specs.openstack.org/openstack/ironic-specs/specs/approved/in-band-deploy-steps.html)
using that interface, it could be possible to inject network configuration in a
similar way to coreos-install.

However this is a very bleeding edge feature (not yet documented), and it means
maintaining a custom interface that would be different to both the proven
uncustomized OSP deploy ramdisk, and the coreos-deploy toolchain.

### Convert IPI to coreos-install based deployment

Long term, from an OpenShift perspective, this makes sense as there is overlap
between the deploy tooling.

However in the short/medium term, the IPI deployment components are based on
Ironic, and that is only really tested with the ironic deploy ramdisk
(upstream and downstream) - we need to investigate how IPI deployments may be
adapted to converge with the coreos-install based workflow, but this is likely
to require considerable planning and development effort to achieve, thus is not
implementable as an immediate solution.

### Customize images

Currently the only option for IPI based deployments is to customize the
OS image, then provide a locally cached copy of this custom image to the
installer via the `clusterOSImage` install-config option.

This is only a stopgap solution, it is inconvenient for the user, and
inflexible as it will only work if the configuration required is common
to every node in the cluster (or for multiple worker machinepools, you
would require a custom image per pool).
