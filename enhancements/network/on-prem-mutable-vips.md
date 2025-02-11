---
title: on-prem-mutable-vips
authors:
  - "@mkowalski"
reviewers:
  - "@JoelSpeed, to review API"
  - "@sinnykumari, to review MCO"
  - "@cybertron, to peer-review OPNET"
  - "@cgwalters"
approvers:
  - "@JoelSpeed"
api-approvers:
  - "@danwinship"
  - "@JoelSpeed"
creation-date: 2023-08-28
last-updated: 2023-08-28
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-178
  - https://issues.redhat.com/browse/OPNET-340
  - https://issues.redhat.com/browse/OPNET-80
see-also:
  - "/enhancements/on-prem-dual-stack-vips.md"
replaces:
superseded-by:
---

# On-Prem Mutable VIPs

## Summary

Originally the on-prem loadbalancer architecture supported only single-stack IPv4 or IPv6 deployments. Later on, after dual-stack support was added to the on-prem deployments, a work has been done to allow installing clusters with a pair of virtual IPs (one per IP stack).
This work however only covered installation-time, leaving clusters originally installed as single-stack and later converted to dual-stack out of scope.

This design is proposing a change which will allow adding a second pair of virtual IPs to clusters that became dual-stack only during their lifetime.

## Motivation

We have customers who installed clusters before dual-stack VIP feature existed. Currently they have no way of migrating because the new feature is avaialble only during initial installation of the cluster.

### User Stories

* As a deployer of a dual-stack OpenShift cluster, I want to access the API using both IPv4 and IPv6.

* As a deployer of a dual-stack OpenShift cluster, I want to access the Ingress using both IPv4 and IPv6.

### Goals

* Allow adding a second pair of virtual IPs to an already installed dual-stack cluster.

* Allow deleting a second pair of virtual IPs on an already installed dual-stack cluster.

### Non-Goals

* Modifying existing virtual IP configuration.

* Modifying virtual IP configuration after second pair of VIPs has been added. We only want to cover "create" and "delete" operation and only for the second pair of VIPs.

* Configuration of any VIPs beyond a second pair for the second IP stack. MetalLB is a better solution for creating arbitrary loadbalancers.

## Proposal

### Workflow Description

The proposed worklflow after implementing the feature would look as described below

1. Administrator of a dual-stack cluster with single-stack VIPs wants to add a second pair for the second IP stack configured.

1. Administrator edits the Infrastructure CR named `cluster` by changing the `spec.platformSpec.[*].apiServerInternalIPs` and `spec.platformSpec.[*].ingressIPs` fields. For dual-stack vips the sample `platformSpec` would look like shown below under "Sample platformSpec".

1. Cluster Network Operator picks the modification of the object and compares values with `spec.platformStatus.[*].apiServerInternalIPs` and `spec.platformStatus.[*].ingressIPs`.

1. After validating that the requested change is valid (i.e. conforms with the goals and non-goals as well as with the validations performed in o/installer), the change is propagated down to the keepalived template and configuration file.

1. After keepalived configuration is changed, the service is restarted or reloaded to apply the changes.

### Sample platformSpec

```yaml
platformSpec:
  baremetal:
    apiServerInternalIPs:
    - "192.0.2.100"
    - "2001:0DB8::100"
    ingressIPs:
    - "192.0.2.101"
    - "2001:0DB8::101"
```

### API Extensions

New fields will have to be added to the platform spec section for [baremetal](https://github.com/openshift/api/blob/938af62eda38e539488d6193e7af292e47a09a5e/config/v1/types_infrastructure.go#L694), [vsphere](https://github.com/openshift/api/blob/938af62eda38e539488d6193e7af292e47a09a5e/config/v1/types_infrastructure.go#L1089)
and [OpenStack](https://github.com/openshift/api/blob/938af62eda38e539488d6193e7af292e47a09a5e/config/v1/types_infrastructure.go#L772). In the first implementation we aim to cover only baremetal.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Change is not designed for Hypershift.

#### Standalone Clusters

No special considerations.

#### Single-node Deployments or MicroShift

No special considerations.

### Implementation Details/Notes/Constraints

Because of the expertise and the team owning this feature, we think the best place for implementing the logic is Cluster Network Operator. CNO however is not an owner of the `cluster` Infrastructure CR what at first sight may seem like a challenge.

Kubernetes API provides a solution for this and we can leverage Watches (with Predicate for optimization) to allow a controller inside CNO to watch an object it does not own. This in fact already happens inside CNO
in the [infrastructureconfig controller](https://github.com/openshift/cluster-network-operator/blob/2005bcd8c93de5bffc05c9c943b51386007f6b9a/pkg/controller/infrastructureconfig/infrastructureconfig_controller.go#L47). It was added as part of the initial dual-stack VIPs implementation. From the discussion with multiple teams back then it was decided that CNO is the best place to implement it.

The controller would get a logic implemented that validates if the change requested by user is valid as in the first implementation we only want to allow adding second pair of VIPs but forbid any modifications of the already configured ones.

The current values of `spec.platformStatus.[*]` are propagated during installation [by the installer](https://github.com/openshift/installer/blob/186cb916c388d29ed3f6ef4e71d9fda409f30bdf/pkg/asset/manifests/infrastructure.go#L160). After implementing this feature, the same function would also set `spec.platformSpec.[*]` so that the cluster is installed with a correct configuration from the very beginning.

When modifying `spec.platformSpec.[*]` the CNO controller would need to propagate this change down to the [keepalived templates managed by the Machine Config Operator](https://github.com/openshift/machine-config-operator/blob/2b31b5b58f0d7d9fe6c3e331e4b3f01c9a1bd00c/templates/common/on-prem/files/keepalived.yaml#L46-L49). As MCO's ControllerConfig
[already observes changes in the PlatformStatus](https://github.com/openshift/machine-config-operator/blob/5b821a279c88fee1cc1886a6cf1ec774891a2258/lib/resourcemerge/machineconfig.go#L100-L105), no additional changes are needed.

One of the tasks of CNO is validating if the change is correct (e.g. VIP needs to belong to the node network). To do so, CNO needs to access various network configuration parameters with `MachineNetworks` being one of them. To facilitate it, we store it as part of the `spec.platformStatus.[*]` and `spec.platformSpec.[*]`.

### Risks and Mitigations

Minimal risk. The dual-stack VIPs feature is already used. This is just adding an ability to add this feature to an already existing cluster. Because modifying and deleting VIPs is out of scope of this enhancement, the operations with the biggest potential of causing an issue do not need to be covered.

### Drawbacks

Because we do not implement Update operation, the main drawback is a scenario when user adds an incorrect address to be configured. We will only allow deleting a second entry, therefore user will need to first delete and then add again a correct pair of VIPs. Because fixing such a typo will require two reboots (one for deleting and one for adding),
enablement of this feature should be performed with some level of care by the end user.

## Design Details

This feature will require changes in a few different components:

* Installer - New fields will need to be populated during the installation. This is a safe part as installer is already populating `status` and now will populate `status` and `spec`.

* API - New fields will need to be added to the platform spec section of the infrastructure object.

* Cluster Network Operator - New object will need to be watched by one of its controllers. The core logic of validating the requested change will be implemented there.

* Machine Config Operator - Definition of the runtimecfg Pod will need to be rendered again when the VIP configuration changes.

* baremetal-runtimecfg - The code rendering the keepalived template belongs to this repo. As the enhancement does not touch the rendering part no big changes are expected in the runtimecfg. This component is used to re-render configuration in case something changes in the cluster (e.g. new nodes are added) so the fact we modify configuration is not a new feature from its perspective.
Small changes may be needed to acommodate to the new user scenario to provide good user experience.

### openshift/api

The new structure of platformSpec would look like this:

```yaml
platformSpec:
  properties:
    baremetal:
      description: BareMetal contains settings specific to the BareMetal platform.
      type: object
      properties:
        apiServerInternalIPs:
          description: apiServerInternalIPs are the IP addresses to...
          type: array
        ingressIPs:
          description: ingressIPs are the external IPs which...
          type: array
        machineNetworks:
          description: IP networks used to connect all the OpenShift cluster nodes.
          type: array
      ...
```

This format (i.e. usage of arrays) is already used by platformStatus so this does not introduce a new type nor schema.

### Cluster Network Operator

There already exists `infrastructureconfig_controller` inside CNO that watches for changes in `configv1.Infrastructure` for on-prem platforms. Because of this we do not need to create a new one nor change any fundaments, we only need to extend its logic.

Currently the controller already reconciles on the `CreateEvent` and `UpdateEvent` so we will implement a set of functions that compare and validate `platformSpec` with `platformStatus` and do what's needed.

### Machine Config Operator

[Keepalived templates managed by the Machine Config Operator](https://github.com/openshift/machine-config-operator/blob/2b31b5b58f0d7d9fe6c3e331e4b3f01c9a1bd00c/templates/common/on-prem/files/keepalived.yaml#L46-L49) currently use the `PlatformStatus` fields. CNO will be responsible for keeping `PlatformSpec` and `PlatformStatus` in sync.
As MCO [already grabs values from the latter](https://github.com/openshift/machine-config-operator/blob/2b31b5b58f0d7d9fe6c3e331e4b3f01c9a1bd00c/pkg/controller/template/render.go#L551-L575), no changes are needed.

It is important to note that Machine Config Operator triggers a reboot whenever configuration of the system changes. We already have a history of introducing changes into MCO and forcefully preventing reboot (i.e. single-stack to dual-stack conversion) but this proven to be problematic in a long run (e.g. https://issues.redhat.com/browse/OCPBUGS-15910) and is now being reverted.
Unless there is a strong push towards the mutable VIPs being rebootless, we should follow the default MCO behaviour and let it reboot.

Similarly to how it happens today, we are not covering a scenario of updating the PlatformStatus only after the change is rolled out to all the nodes. Today installer sets the PlatformStatus as soon as the configuration is desired and keeps it even if the keepalived ultimately did not apply the config. For simplicity we are keeping it that way, effectively meaning that PlatformStatus
and PlatformSpec will contain always the same content. The main reason is that implementing a continuous watch for keepalived runtime would require us to implement a new and relatively complicated controller that tracks the VIP configuration at runtime. Since we have not created one till now, it remains outside of the scope of this enhancement.

### baremetal-runtimecfg

Keepalived configuration is rendered based on the command-line parameters provided to the baremetal-runtimecfg. Those come from the Pod definition that is managed by the MCO. Because of that it is valid to say that baremetal-runtimecfg is oblivious to whether `PlatformSpec` or `PlatformStatus` stores the desired config.

## Test Plan

We will need to add a few tests that will perform the operation of adding a pair of VIPs to an already existing dual-stack clusters. Once done, we can reuse the already existing steps that test dual-stack clusters with dual-stack VIPs.

## Graduation Criteria
We do not anticipate needing a graduation process for this feature. The internal loadbalancer implementation has been around for a number of releases at this point and we are just extending it.

### Dev Preview -> Tech Preview

NA

### Tech Preview -> GA

NA

### Removing a deprecated feature

NA

## Upgrade / Downgrade Strategy

Upgrades and downgrades will be handled the same way they are for the current internal loadbalancer implementation. On upgrade the existing VIP configuration will be maintained. We will not automatically add additional VIPs to the cluster on upgrade. If an administrator of a dual-stack cluster wants to use the new functionality that will need to happen as a separate operation from the upgrade.

When upgrading a cluster to the version that supports this functionality we will need to propagate new `PlatformSpec` fields from the current `PlatformStatus` fields. We have experience with this as when introducing dual-stack VIPs
the [respective synchronization](https://github.com/openshift/cluster-network-operator/blob/2005bcd8c93de5bffc05c9c943b51386007f6b9a/pkg/controller/infrastructureconfig/sync_vips.go#L18-L22) has been implemented inside CNO.

## Version Skew Strategy

The keepalived configuration does not change between the version of the cluster and also when introducing this feature. There is no coordination needed and as mentioned above, enabling this functionality should be a separate operation from upgrading the cluster.

## Operational Aspects of API Extensions

NA

## Support Procedures

NA

## Alternatives

The original enhancement for dual-stack VIPs introduced an idea of creating a separate instance of keepalived for the second IP stack. It was mentioned as something with simpler code changes but following this path would mean we can have two fundamentally different architectures of OpenShift clusters running in the field.

Because we want clusters with dual-stack VIPs to not differ when they were installed like this from converted clusters, this is probably not the best idea.
