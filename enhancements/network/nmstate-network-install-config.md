---
title: nmstate-network-install-config
authors:
  - "@cybertron"
reviewers:
  - "@mkowalski"
  - "someone from installer"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2024-10-18
last-updated: 2025-05-09
tracking-link:
  - https://issues.redhat.com/browse/OPNET-592
see-also:
  - "/enhancements/network/configure-ovs-alternative.md"
  - "/enhancements/network/baremetal-ipi-network-configuration.md"
replaces:
  - NA
superseded-by:
  - NA
---

# Install-config interface for NMState configuration

## Summary

In the configure-ovs alternative enhancement we created a new mechanism
for deployers to configure host networking with NMState. This is similar,
but complementary, to the baremetal day 1 networking feature. In order to
provide the basic functionality in a timely fashion, the interface used
for the previous feature was machine-configs. We would now like to put a
formal install-config interface over that to improve the user experience.

## Motivation

Host networking is becoming increasingly important with the rise of
virtualization workloads in OpenShift. We are also anticipating that many
new virtualization customers will have limited experience with advanced
host network configuration and want to make the experience as smooth as
possible. While the existing machine-config interface is functional, it
requires manual creation of base64-encoded content and machine-config
manifests. All of this could be handled in the installer instead, which
would be a much better interface.

### User Stories
* As an OpenShift Virtualization user, I want full control over the
  host networking setup of my nodes so I can run VM workloads on nodes
  with only a single interface.

* As an OpenShift deployer, I want to create br-ex with NMState so it
  can be managed on day 2 with Kubernetes-NMState.

### Goals

Provide an install-config interface to the existing NMState functionality.

### Non-Goals

While this is a component of the multi-interface configuration feature,
anything beyond creating the install-config interface for NMState data
will be handled in a separate enhancement that is dependent on this one.

This also does not replace the existing networkConfig functionality for
the baremetal platform. Because this feature requires enough network
connectivity to pull Ignition, the two configuration mechanisms are
complementary. The baremetal feature can be used to configure, for
example, a static IP on the primary interface to allow Ignition to be
pulled, then this feature can be used to do full configuration on the
node.

## Proposal

Add a new field to the networking section of install-config that allows
users to provide their NMState config for cluster nodes. This will look
something like the networkConfig field for the baremetal platform, but
will be platform-agnostic.

### Workflow Description

A user will write an appropriate NMState config for their network environment
and include that in their install-config. The NMState YAML they provide will
then be base64-encoded and placed in a machine-config manifest that will be
included in the cluster configuration. From there, the flow will be the same
as described in the configure-ovs-alternative feature.

### API Extensions

There will be no additions to the API for this feature. It is purely an
install-config wrapper around existing functionality.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This should be usable to configure host networking in any cluster deployed
using the installer. I do not anticipate needing anything Hypershift-specific
for it.

#### Standalone Clusters

Yes, standalone clusters are one of the primary motivators for this.

#### Single-node Deployments or MicroShift

It should have no effect on SNO. Once these configurations are applied
there is no ongoing cost in terms of CPU and memory.

I do not believe this would be especially useful for MicroShift either.
Since those devices are usually pre-configured, there wouldn't be much
need for deploy-time host network configuration from the installer.

### Implementation Details/Notes/Constraints

We are proposing to add a new field to the networking section of install-config
that would look something like the following:

```yaml
networking:
  networkType: OVNKubernetes
  machineNetwork:
  - cidr: 192.168.111.0/24
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  serviceNetwork:
  - 172.30.0.0/16
  # New part begins here
  hostConfig:
  - name: master-0
    networkConfig:
      interfaces:
      - type: ethernet
        name: eth0
        ...
  - name: master-1
    networkConfig:
      interfaces:
      - type: ethernet
        name: eth1
        ...
```

The content of the networkConfig fields will be used as input to the
configure-ovs-alternative feature.

### Risks and Mitigations

There is a potential for confusion because this would result in NMstate config
data existing in multiple different places in install-config. However, there's
no real way around that because some complex architectures necessitate a two
step process, so we'd need to have multiple NMState configs per node.

We intend to mitigate this confusion by providing extensive documentation of
the day one network configuration process in OpenShift. A preliminary,
unofficial version of this already exists here:
http://blog.nemebean.com/content/day-one-networking-openshift

### Drawbacks

Exposing the full NMState API this way allows users a lot of flexibility in
how they configure their host networking, including bad configurations that
may not work well or at all. However, this is really no different from any
other existing host networking mechanism. We have not historically been
that prescriptive when it comes to host networking, and while that does
occasionally cause problems, it hasn't been a huge issue up to now and it's
unlikely this feature will make it worse.

All of this configuration can already be achieved by writing nmconnection
files to the nodes anyway. This is actually a slight improvement in that
NMState will validate syntax before applying changes and will roll back
changes that fail its built-in checks.

## Open Questions [optional]

None

## Test Plan

We will be adding test jobs for the balance-slb bond mode which will exercise
this feature.

## Graduation Criteria

This is a new interface to a GA feature so there will not be a graduation process.

### Dev Preview -> Tech Preview

NA

### Tech Preview -> GA

NA

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

Not relevant

## Version Skew Strategy

There will be no version skew at install time.

## Operational Aspects of API Extensions

No API extensions.

## Support Procedures

The same as we would currently use for host networking issues (sosreports,
NetworkManager trace logs, etc.).

## Alternatives

There is a web app that can also be used to generate these configs in a more
user-friendly way: https://access.redhat.com/articles/7111145

However, that is not integrated with the product and is provided on a
best-effort support basis. While we should also investigate adding support
for this feature to the various deployment UIs that exist, even that would not
replace install-config integration for non-UI deployers.

We could also continue to use raw machine-configs as the interface for this
feature, but frankly that's a terrible option. Users hate it and we have to
do better.

## Infrastructure Needed [optional]

None
