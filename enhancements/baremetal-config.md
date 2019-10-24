---
title: Baremetal IPI Config Enhancement
authors:
  - "@sadasu"
reviewers:
  - "@deads2k"
  - "@hardys"
  - "@enxebre"
  - "@squeed"
  - "@abhinavdahiya"

approvers:
  - "@deads2k"

creation-date: 2019-10-23
last-updated: 2019-10-23
status: implemented
see-also:
  - "https://github.com/openshift/api/pull/480"  
replaces:
  - 
superseded-by:
  - 
---

# Config required for Baremetal IPI deployments

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

None.

## Summary

The configuration required to bring up a Baremetal IPI (metal3) cluster needs
to be availabe in a Config Resource. The installer aleady adds some configuration
to the Infrastructure Config CR. It is also aware of the additional config
required during the deployment of a metal3 cluster. This enhancement proposes
to extend the BareMetaPlatformStatus CR to hold these additional values needed
for a metal3 deployment.

## Motivation

Adding these additional config items to the BareMetalPlatformStatus CR, allows
versioning of these config items.

### Goals

The goal of this enhancement is to enable the installer to add vital config
values for a Baremetal IPI deployment to a CR. The MAO will then be able to
access these values when it kicks off a metal3 cluster.

Update lifecycle for bootimage is an unsolved problem for all platforms. For
more information, refer to : https://github.com/openshift/os/issues/381.

### Non-Goals

All config values except the RHCOSImageUrl are not expected to change after a
metal3 cluster is up. Handling changes to this value after install time has
not been explored and that is a non-goal at this time.

### Proposal

To support configuration of a Baremetal IPI (metal3) deployment, a set of new
config items are required to be made availabe via a Config Resource. The
BareMetalPlatformStatus is an ideal fit because it contains configuration
that is specific to a Baremetal deployment.

This proposal aims to add the following to BareMetalPlatformStatus:

1. ProvisioningInterface : This is the interface name on the underlying
baremetal server which would be physically connected to the provisioning
network.

2. ProvisioningIP : This is the IP address used to to bring up a NIC on
the baremetal server for provisioning. This value should not be in the
DHCP range and should not be used in the provisioning network for any
other purpose (should be a free IP address.) It is expected to be provided
in the format : 10.1.0.3.

3. ProvisioningNetworkCIDR : This is the provisioning network with its
CIDR. The ProvisioningIP and the ProvisioningDHCPRange are expected to
be within this network. The value for this config is expected to be
provided in the format : 10.1.0.0/24.

4. ProvisioningDHCPRange : This is a range of IP addresses from which
baremetal hosts can be assigned their IP address. This range should be
provided as two comma seperated IP addresses where the addresses represent
a contiguous range of IPs that are available to be allocated. It is expected
to be provided in the format : 10.1.0.10, 10.1.0.100.

5. CacheURL : This is a location where image required to bring up baremetal
hosts have been previously downloaded for faster image downloads within the
cluster. This is an optional parameter and is expected to be provided
in the format : http://111.111.111.1/images

6. RHCOSImageURL : This config is expected to be the location of the
official RHCOS release image. It is a required parameter and will be used
to deploy new nodes in the cluster.

### User Stories [optional]

This enhancement will allow users to have their Barametal IPI deployments
customizable to their specific hardware and network requirements.

This also allows IPI baremetal deployments to take place without manual
workarounds like creating a ConfigMap for the config (which is the
current approach we are using to test baremetal deployments.) 

The new config items would be set by the installer and will be used by
the machine-api-operator (MAO) to generate more config items that are
derivable from these basic parameters. Put together, these config
parameters are passed in as environment variables to the various containers
that make up a metal3 baremetal IPI deployment. 

## Design Details

### Test Plan

The test plan should involve making sure the openshift/installer generates
all configuration items within the BaremetalPlatformStatus when the platform
type is Baremetal.

MAO reads this configuration and uses these to derive additional configuration
required to bring up a metal3 cluster. E2e testing should make sure that MAO
is able to bring up a metal3 cluster using config from this enhanced
BaremetalPlatformStatus CR.

### Upgrade / Downgrade Strategy

Baremetal Platform type will be available for customers to use for the first
time in Openshift 4.3. And, when it is installed, it will always start as a
fresh baremetal installation at least in 4.3. There is no use case where a 4.2
installation would be upgraded to a 4.3 installation with Baremetal Platform
support enabled.

For the above reason, in this particular case, we would not have to make
additional provisions for an upgrade strategy.

Baremetal platform is experimental in 4.2 but, in the small chance that a user
might upgrade to 4.3 from 4.2 and also set the Platform type to Baremetal for
for the first time during the upgrade, our upgrade strategy would be to
support obtaining the config from both the ConfigMap and the
BareMetalPlatformStatus ConfigResource. In 4.3, the MAO would be implemented
to first look for the baremetal config in the CR and if that fails, it would
try to read it from the ConfigMap.
