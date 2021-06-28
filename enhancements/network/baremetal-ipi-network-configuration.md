---
title: baremetal-ipi-network-configuration
authors:
- "@cybertron"
- "@hardys"
- "@zaneb"
reviewers:
- "@kirankt"
- "@dtantsur"
- "@zaneb"
approvers:
- "@trozet"
creation-date: 2021-05-21
last-updated: 2021-07-07
status: provisional|implementable

see-also:
- "/enhancements/host-network-configuration.md"
- "/enhancements/machine-config/mco-network-configuration.md"
- "/enhancements/machine-config/rhcos/static-networking-enhancements.md"
---

# Baremetal IPI Network Configuration

Describe user-facing API for day-1 network customizations in the IPI workflow,
with particular focus on baremetal where such configuration is a common
requirement.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently in the IPI flow, there is no way to provide day-1 network configuration
which is a common requirement, particularly for baremetal users.  We can build
on the [UPI static networking enhancements](rhcos/static-networking-enhancements.md)
to enable such configuration in the IPI flow.

## Motivation

Since the introduction of baremetal IPI, a very common user request is how
to configure day-1 networking, and in particular the following cases which are not currently possible:

* Deploy with OpenShift Machine network on a tagged (non-default) VLAN
* Deploy with OpenShift Machine network using static IPs (no DHCP)

In both cases, this configuration cannot be achieved via DHCP so some
means of providing the configuration to the OS is required.

In the UPI flow this is achieved by consuming user-provided NetworkManager
keyfiles, as an input to `coreos-install --copy-network`, but there is
no corresponding user interface at the openshift-install level.

Additionally, there are other networking configurations that would be useful
to configure via the same mechanism, even though it may be possible to
accomplish them in another way. For example:

* Deploy with OpenShift Machine network on a bond
* Deploy with OpenShift Machine network on a bridge
* Configure attributes of network interfaces such as bonding policies and MTUs

The proposed solutions should all be flexible enough to support these use
cases, but it is worth noting in case an alternative with a narrower scope
would be put forward.

### Goals

* Define API for day-1 network customizations
* Enable common on-premise network configurations (bond+vlan, static ips) via IPI

Initially these configurations will be one per host. If there is time, an
additional goal would be to provide a mechanism to apply a single config to all
nodes of a particular type. For example, one config for all masters and another
config for all workers.

### Non-Goals

* Platforms other than `baremetal`, although the aim is a solution which could be applied to other platforms in future if needed.
* Enabling kubernetes-nmstate by default for day-2 networking is discussed via
[another proposal](https://github.com/openshift/enhancements/pull/747)
* Provide a consistent (ideally common) user API for deployment and post-deployment configuration. Getting agreement on a common API for day-1 and day-2 has stalled due to lack of consensus around enabling kubernetes-nmstate APIs (which are the only API for day-2 currently) by default
* Configuration of the provisioning network. Users who don't want DHCP in their
deployment can use virtual media, and users who want explicit control over the
addresses used for provisioning can make the provisioning network unmanaged and
deploy their own DHCP infrastructure.

## Proposal

### User Stories

#### Story 1

As a baremetal IPI user, I want to deploy via PXE and achieve a highly
available Machine Network configuration in the most cost/space effective
way possible.

This means using two top-of-rack switches, and 2 NICS per host, with the
default VLAN being used for provisioning traffic, then a bond+VLAN configuration
is required for the controlplane network.

Currently this [is not possible](https://bugzilla.redhat.com/show_bug.cgi?id=1824331)
via the IPI flow, and existing ignition/MachineConfig APIs are not sufficient
due to the chicken/egg problem with accessing the MCS.

#### Story 2

As an on-premise IPI user, I wish to use static IPs for my controlplane network,
for reasons of network ownership or concerns over reliability I can't use DHCP
and therefore need to provide a static configuration for my primary network.

There is no way to provide [MachineSpecific Configuration in OpenShift](https://github.com/openshift/machine-config-operator/issues/1720) so I am
forced to use the UPI flow which is less automated and more prone to errors.

### Risks and Mitigations

In some existing workflows, kubernetes-nmstate is used to do network configuration on day-2. Using a different interface for day-1 introduces the potential for mismatches and configuration errors when making day-2 changes. It should be possible to mitigate this by generating the NetworkManager keyfiles using the nmstatecli, which will allow the same nmstate config to be used directly in the day-2 configuration.

An added benefit of writing nmstate configurations instead of NetworkManager
keyfiles directly is that if/when a day-1 nmstate-based interface becomes
available the config files can be directly inserted into the new interface.
However, this is not a complete solution because it still requires manually copying
configuration to multiple locations which introduces the potential for errors.

## Design Details

In the IPI flow day-1 network configuration is required in 2 different cases:

* Deployment of the controlplane hosts via terraform, using input provided to openshift-install
* Deployment of compute/worker hosts (during initial deployment and scale-out), via Machine API providers for each platform

In the sections below we will describe the user-facing API that contains network configuration, and the proposed integration for each of these cases.

### User-facing API

RHCOS already provides a mechanism to specify NetworkManager keyfiles during deployment of a new node. We need to expose that functionality during the IPI install process. There are a couple of options on how to do that:

* A new section in install-config.
* A secret that contains base64-encoded content for the keyfiles.

These are not mutually exclusive. If we implement the install-config option, we will still need to persist the configuration in a secret so it can be used for day-2. In fact, the initial implementation should likely be based on secrets, and then install-config integration can be added on top of that to improve the user experience. This way the install-config work won't block implementation of the feature.

The data provided by the user will need to have the following structure:
```yaml
[hostname]:
  [interface name]:
    [contents of NetworkManager key file for interface]
  [second interface name]:
    [contents of NetworkManager key file for second interface]
  etc...
```

For example:
```yaml
openshift-master-0:
  eth0:
    [connection]
    id=eth0
    uuid=18e0cec7-041c-4fb3-957c-a60c80dd9b85
    type=ethernet
    etc...
openshift-master-1:
  eth0:
    [connection]
    id=eth0
    etc...
```

In install-config this would look like:
```yaml
platform:
  baremetal:
    hosts:
      - name: openshift-master-0
        networkConfig:
        - interface: eth0
          data: |
            [connection]
            id=eth0
            uuid=18e0cec7-041c-4fb3-957c-a60c80dd9b85
            type=ethernet
            etc...
```

Because the initial implementation will be baremetal-specific, we can put the
network configuration data into the baremetal host field, which will allow easy
mapping to the machine in question.

### Processing user configuration

#### Deployment of the controlplane hosts via terraform

We will map the keyfiles to their appropriate BareMetalHost using the host field
of the baremetal install-config. The keyfiles will then be added to custom
images for each host built by Terraform and Ironic.

Since different configuration may be needed for each host (for example, when
deploying with static IPs), a Secret per host will be created. A possible
future optimization is to use a single secret for scenarios such as VLANs
where multiple hosts can consume the same configuration, but the initial
implementation will have a 1:1 Secret:BareMetalHost mapping.

#### Deployment of compute/worker hosts

BareMetalHost resources for workers will be created with the Secret containing the network data referenced in the `preprovisioningNetworkData` field defined in the Metal³ [image builder integration design](https://github.com/metal3-io/metal3-docs/blob/master/design/baremetal-operator/image-builder-integration.md#custom-agent-image-controller).
This will cause the baremetal-operator to create a PreprovisioningImage CRD and wait for it to become available before booting the IPA image.

An OpenShift-specific PreprovisioningImage controller will use the provided network data to build a CoreOS IPA image with the correct ignition configuration in place, and the baremetal-operator will then use this to boot the Host.

If not overridden, the contents of the same network data secret will be passed to the Ironic custom deploy step for CoreOS that installs the node.

### Test Plan

Support will be added to [dev-scripts](https://github.com/openshift-metal3/dev-scripts) for deploying the baremetal network without DHCP enabled. A CI job will populate install-config with the appropriate network configuration and verify that deployment works properly.

### Graduation Criteria

We expect to support this immediately on the baremetal IPI platform.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

There should be little impact on upgrades and downgrades. Nodes are deployed with network configuration baked into the image, which means it will remain over upgrades or downgrades. NetworkManager keyfiles are considered a stable interface so any version of NetworkManager should be able to parse them equally.

Any additions or deprecations in the keyfile interface would need to be handled per the NetworkManager policy.

### Version Skew Strategy

As this feature targets day-1 configuration there should be no version skew. Day-2 operation will be handled by other components which are outside the scope of this document.

## Implementation History

4.9: Initial implementation

## Drawbacks

Using NetworkManager keyfiles as the user interface for network configuration makes day-2 configuration more problematic. The only existing mechanism for modifying keyfiles after initial deployment is machine-configs, which will trigger a reboot of all affected nodes when changes are made. It also provides no validation or rollback functionality.

## Alternatives

### Use Kubernetes-NMState NodeNetworkConfigurationPolicy custom resources

If we were able to install the [NNCP CRD](https://nmstate.io/kubernetes-nmstate/user-guide/102-configuration)
at day-1 then we could use that as the configuration interface. This has the advantage of matching the configuration syntax and objects used for day-2 network configuration via the operator.

This is currently blocked on a resolution to [Enable Kubernetes NMstate by default for selected platforms](https://github.com/openshift/enhancements/pull/747). Without NMState content available at day-1 we do not have any way to process the NMState configuration to a format usable in initial deployment.
While we hope to eventually come up with a mechanism to make NMState available on day-1, we needed another option that did not make use of NMState in order to deliver the feature on time.

Once the keyfile implementation is done it should be simpler to layer NMState over that, so the two solutions are not exclusive of each other.

### Provide raw nmstate data

We could expose an interface to the user which accepts raw nmstate data (not a CR wrapper).  This would mean a common DSL to the NodeNetworkConfigurationPolicy resource, but not directly reusing that API, so there is risk of drift between the two APIs.

This does not solve the problem of the user needing to pass the same configuration in multiple formats, although it does ensure that the same underlying format is used, which somewhat reduces the opportunity for errors.

#### Create a net-new NMState Wrapper CR

The [assisted-service](https://github.com/openshift/assisted-service/blob/0b0e3677ae83799151d11f1267cbfa39bb0c6f2e/docs/hive-integration/crds/nmstate.yaml) has created a new NMState wrapper CR.

We probably want to avoid a proliferation of different CR wrappers for nmstate
data, but one option would be to convert that (or something similar) into a common
OpenShift API, could such an API be a superset of NNCP e.g also used for day-2?

This would mean we could at least use a common configuration format based on nmstate with minimal changes (or none if we make it _the_ way OpenShift users interact with nmstate), but unless the new API replaces NNCP there is still the risk of configuration drift between the day-1 and day-2 APIs.
