---
title: Store user ignition customizations in MachineConfig
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
  - "@yuqi-zhang"
approvers:
  - "@cgwalters"
  - "@crawford"
  - "@runcom"
  - "@yuqi-zhang"

creation-date: 2020-08-21
last-updated: 2020-12-03
status: implemented
---

# Store user ignition customizations in MachineConfig

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes changing the way installer ignition customizations
are stored, so that instead of storing the modified pointer ignition in a
Secret we include the user changes in a MachineConfig, such that the MCO
can manage it, and it is included in the MCO rendered config.

## Motivation

The installer supports user [modification of the pointer ignition config](https://github.com/openshift/installer/blob/master/docs/user/customization.md#os-customization-unvalidated) it generates.  While this interface is marked as unvalidated, we know from [previous bug reports](https://bugzilla.redhat.com/show_bug.cgi?id=1881703) that some users are using it.

This presents a problem for the plans to have the [MCO manage the pointer config](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/user-data-secret-managed.md)
because it uses a static template that does not consider any user customizations, and for that reason the work was [reverted](https://github.com/openshift/machine-config-operator/pull/2126).

Additionally, in some situations it is necessary to perform network configuration
before it is possible to download the rendered config from the MCS in the ramdisk
where ignition evaluates the pointer config.

This leads to a chicken-egg problem, where a user could configure the network
via ignition, but ignition cannot apply any config until it has downloaded
any append/merge directives.

We could solve that problem on some platforms (baremetal in particular) by
just providing the fully rendered config to each host, bypassing the pointer
config where config-drive size limits allow.  However this results in the [same
issue with losing any user customizations](https://bugzilla.redhat.com/show_bug.cgi?id=1833483) provided directly via the pointer config.

Discussions indicate we may want to deprecate/remove this ignition-config method of
customization and mandate MachineConfig manifests instead.  We could in the
meantime do that internally e.g have the installer detect any customization
to the pointer config, and inject a MachineConfig manifest containing that data
instead of writing it via the current user-data Secret.

### Goals

* Unblock the MCO managed pointer ignition work that was previously reverted

* Provide a solution to customer requirements for baremetal IPI around bond/VLAN
   and other configurations for controlplane networking

* Provide a means by which we might warn the user if we decide to deprecate customization via the pointer ignition config.

### Non-Goals

This work may give us some options wrt warnings around this interface but it
does not aim to formally deprecate the ignition-configs customization, that
may be separately discussed in a future enhancement.

The discussion of interfaces here and how they relate to network configuration
 only considers the existing interfaces, not proposals around a future
[declarative network configuration](https://github.com/openshift/enhancements/pull/399) although we need to ensure the approach taken in each case is aligned.

### User Stories

#### Story 1

As a user I would like my existing customizations to pointer ignition files
to work, but be warned if there is a preferable interface I should be using.

#### Story 2

As a baremetal IPI user, I need to deploy in an environment where network
configuration is required before access to the controlplane network is possible.

Specifically I wish to deploy in an environment where the controlplane is
on a non-default VLAN (a common configuration for baremetal).

Currently I cannot perform this configuration via ignition or MachineConfig,
because the pointer config requires network access prior to any configuration
being performed on the host.

## Design Details

If the installer can detect the case where the pointer config loaded at `create cluster` contains
user customizations we can create a new MachineConfig object to encapsulate this config,
instead of persisting it to the user-data secret directly.

This would be equivalent to the user creating those same customizations via a MachineConfig
manifests, and we could potentially warn users to guide them to that interface,
but avoid breaking any existing users performing customizations directly via
the pointer ignition.

This would mean we can potentially restore the [MCO managed pointer ignition](openshift/machine-config-operator#1792)
work, where the MCO will maintain a templated pointer config, and any user customizations
will be persisted in the existing rendered config.

This would also solve the issue for IPI baremetal without any [MCO API changes](https://github.com/openshift/enhancements/pull/467), since we could consume the existing rendered config directly.

### Test Plan

* Ensure no regressions in existing deployments via existing e2e-metal-ipi CI coverage
* Prove we can do deployments where the controlplane network requires configuration, such as that described in [previous bug reports](https://bugzilla.redhat.com/show_bug.cgi?id=1824331) - we can add a periodic CI job based on e2e-metal-ipi to test this.


### Upgrade / Downgrade Strategy

This change only impacts the day1 deployment, since the changes are in
the installer.

If the MCO managed pointer ignition config work is restored, the upgrade
strategy will be defined in https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/user-data-secret-managed.md

### Version Skew Strategy

## Alternatives

### Native ignition support for early-network config

This was initially proposed via [an ignition issue](https://github.com/coreos/ignition/issues/979)
where we had some good discussion, but ultimately IIUC the idea of adding a new
interface for early network configuration to ignition was rejected.

### Implement MCO support for a flattened ignition config

This was discussed in https://github.com/openshift/enhancements/pull/467 and
the solution documented here is derived from discussion on that PR.

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
