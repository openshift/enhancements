---
title: openstack-ipi-failure-domains
authors:
  - "@EmilienM"
  - "@mandre"
  - "@mdbooth"
reviewers:
  - "@sdodson"
  - "@patrickdillon"
approvers:
  - "@sdodson"
  - "@patrickdillon"
api-approvers:
  - "None"
creation-date: 2022-07-04
last-updated: 2022-07-05
tracking-link:
  - "https://issues.redhat.com/browse/OSASINFRA-2908"
see-also:
  - "/enhancements/installer/aws-customer-provided-subnets.md"
  - https://github.com/openshift/enhancements/pull/918
superseded-by:
  - "https://docs.google.com/document/d/1_AjhGEyK6vGBFSSqiLVtLt_qOq6o9t2dtKqq2eFr48Y/"
---

# Allow to stretch a cluster across multiple failure domains

## Summary

For organizations where service availability is a concern, networking architectures are usually more modern than what we can find in a traditional datacenter.
For example, the "Spine and Leaf" design is a recommended choice when deploying OpenStack at scale.
In this architecture, each leaf (lower-level access switch) is connected to multiple spines (upper-level core switch) in a full mesh. The latency becomes predictable, there is no more bottleneck and overall the resilience is improved.
Each leaf brings their own network fabrics that are usually using their own power units. Also each network fabric manages their its own Layer-2 domain, and therefore network subnets. Routing protocols are usually used so applications can communicate with each other in the datacenter, since they don't live within the same domain anymore.

We want to allow the OpenShift admins to make use of this topology when deploying via IPI and also provide higher resilience and performance for their clusters.

This enhancement proposes to support the deployment of OpenShift into multiple subnets from day 1, using IPI on OpenStack platform.

To accomplish this, we introduce an OpenStack platform scoped property combining compute and storage Availability Zones and subnets to represent Failure Domains. We allow referencing these failure domains in the Control Plane and Compute Machine Pools.

This feature will remove the limitation that we have today in BYON (Bring your own network) which is the support of a single primary subnet for the machines on day 1. We want to be able to deploy the cluster across multiple subnets and storage sources, where each resource live in a given Failure Domain.

## Motivation

### User Stories

- As an administrator, I would like to stretch my OpenShift control plane across multiple Failure Domains (x3) so I increase the cluster resilience and performance.
- As an administrator, I would like to deploy my OpenShift computes across multiple Failure Domains (>1) to increase the workload's resilience and performance.

### Goals

- The user will be able to define Failure Domains in the `install-config.yaml` file used by the OpenShift Installer.
- The user will have the ability to deploy the OpenShift control plane across multiple Failure Domains from day 1 using IPI.
- The user will have the ability to deploy the OpenShift computes across multiple Failure Domains from day 1 using IPI.

### Non-Goals

- Management of the network resources by the installer: the networks and subnets have to be pre-created in OpenStack Neutron.
- Dynamic routing: routing between the networks is managed by the network infrastructure.
- Control-Plane VIP (e.g. API, ingress) route advertisement is not discussed in this enhancement and will
  be solved by another proposal. If needed, more options will be added to the Failure Domain block to allow more advanced configurations; but this is out of scope for now.
- Validate the latency between the nodes. We expect all the availability zones to be located in the same datacenter. The latency between the nodes must remain acceptable for etcd to function correctly.

## Proposal

The Installer allows users to provide a list of Failure Domains which contain information about networking and storage. The Failure domains is defined as an OpenStack platform scoped property. Failure domains are referenced in OpenStack MachinePools.

The Installer validates the information given by the user and return an error if a resource doesn't exist or is incorrectly used.

The cluster will be deployed in these domains on day 1 via IPI.

Nodes will be provisioned in the different Failure Domains in a Round Robin fashion.

When using Failure Domains, the users will have to do it for both the control plane and the computes machine pools.

The infrastructure resources owned by the cluster continue to be clearly identifiable and distinct from other resources.

Destroying a cluster must make sure that no resources are deleted that didn't belong to the cluster.  Leaking resources is preferred over any possibility of deleting non-cluster resources.

### Workflow Description

- The OpenStack administrators will deploy OpenStack with multiple Availability Zones, so Nova and Cinder can respectfully deploy servers and volumes in the Failure Domain.
- The OpenStack administrators will create at least one Routed Provider Network, and then multiple subnets, where each zone has a least one subnet.
- The OpenShift administrators will identify the Failure Domains: their Availability Zones, subnets, etc. They'll decide which ones will be used for the masters, and the ones for the computes (failure domains can be shared).
- The OpenShift administrators will provide the right configuration in the `install-config.yaml` file and then deploy the cluster (detailed later in this document).
- (Out of scope for this enhancement, see "Non-Goals") The OpenShift administrators will make sure that the network infrastructure has a networking route to reach the OCP Control Plane VIPs.

### API Extensions

The Cloud team is currently working on adding support for [Control Plane Machine Set](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/control-plane-machine-set.md).
It is possible we'll want to update the Control Plane Machine Set API to match the OpenStack failure domains defined here.


### Implementation Details/Notes/Constraints

#### Types

```golang
package openstack

// Platform stores all the global configuration that all
// machinesets use.
type Platform struct {
    ...

    // FailureDomains configures failure domain information for the OpenStack platform
    // +optional
    FailureDomains *[]FailureDomain `json:"failureDomains,omitempty"`
}

// FailureDomain holds the information for a failure domain
type FailureDomain struct {
  // Name defines the name of the OpenStackPlatformFailureDomainSpec
  Name string `json:"name"`

  // ComputeZone is the compute zone on which the nodes belonging to the
  // failure domain must be provisioned.
  // If not specified, the nodes are provisioned in the OpenStack Nova default availabity zone.
  // +optional
  ComputeZone string `json:"computeZone,omitempty"`

  // StorageZone is the storage zone from where volumes should be provisioned
  // for the nodes belonging to the failure domain.
  // If not specified, volumes are provisioned from the default storage availabity zone.
  // +optional
  StorageZone string `json:"storageZone,omitempty"`

  // Subnet is the UUID of the OpenStack subnet nodes will be provisioned on
  Subnet string `json:"subnet"`
}

// MachinePool stores the configuration for a machine pool installed
// on OpenStack.
type MachinePool struct {
	...

	// FailureDomains is the list of failure domains where the instances should be deployed.
	// If no failure domains are provided, all instances will be deployed on OpenStack Nova default availability zone
	// +optional
	FailureDomains []string `json:"failureDomains,omitempty"`
}

```

#### Resource provided to the installer

The OpenShift administrators provide a list of `failureDomains` and their information:

```yaml
platform:
  openstack:
    ...
    failureDomains:
    - name: rack1
      computeZone: az1
      storageZone: cinder-az1
      subnet: d7ffd3d8-87e1-481f-a818-ff2f17787d40
    - name: rack2
      computeZone: az2
      storageZone: cinder-az2
      subnet: f584ce8c-bb54-422b-8e11-6a530be4c23e
    - ...
```
There might be additional options in the domains, like flavors or BGP peers, but for now we will cover the minimum.
We reserve ourselves the possibility to expand the definition of failure domains in the future.

Then they'll choose what Failure Domains to use when deploying the control plane and compute nodes by setting the `failureDomain` field in the machine pools:

```yaml
controlPlane:
  name: master
  platform:
    openstack:
      failureDomains:
      - rack1
      - rack2
      - rack3
  replicas: 3 // this will deploy 1 master in each domain
compute:
- name: worker
  platform:
    openstack:
      ...
      failureDomains:
      - rack1
      - rack2
      - rack3
      - rack4
      - rack5
      rootVolume:
        size: 30
        type: performance
  replicas: 10 // this will deploy 2 workers per rack
```

The installer creates a machineSet per failure domain passed to the compute machinePool.

The machineSet for the compute's `rack1` failure domain will look like the following:
```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  labels:
    machine.openshift.io/cluster-api-cluster: <infrastructure_ID>
    machine.openshift.io/cluster-api-machine-role: worker
    machine.openshift.io/cluster-api-machine-type: worker
  name: <infrastructure_ID>-worker-rack1
  namespace: openshift-machine-api
spec:
  replicas: 2
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-cluster: <infrastructure_ID>
      machine.openshift.io/cluster-api-machineset: <infrastructure_ID>-worker-rack1
  template:
    ...
    spec:
      metadata:
      providerSpec:
        value:
          apiVersion: openstackproviderconfig.openshift.io/v1alpha1
          cloudName: openstack
          flavor: <nova_flavor>
          kind: OpenstackProviderSpec
          networks:
            - subnets:
              - uuid: d7ffd3d8-87e1-481f-a818-ff2f17787d40
          availabilityZone: az1
          rootVolume:
            diskSize: 30
            volumeType: performance
            availabilityZone: cinder-az1
          ...
```

The installer will create the control plane machines in a way that they can easily be adopted by the [ControlPlane machineset](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/control-plane-machine-set.md).

When deploying nodes on separate subnets, the user will need to update the `machineNetwork` option to list all the possible CIDRs on which the machines can be:

```yaml
networking:
  ...
  machineNetwork:
  - cidr: 192.168.25.0/24
  - cidr: 192.168.26.0/24
  - cidr: 192.168.27.0/24
  - ...
```

 The `machineNetwork` is the source for creating the security groups rules during installation. There's a [known bug][bz-2095323] in the OpenStack implementation that only takes into account the first CIDR.

For now, `machinesSubnet` is required to be set when deploying nodes on separate Failure Domains, and is then used to identify where to create the API and Ingress ports.

The OpenShift administrator will be able to adjust the number of computes nodes for a specific Failure Domain, but only on day 2.

Also, the installer will provide some validations in order to avoid deployment errors:

- A maximum of 3 `failureDomains` can be provided to the control plane machine pool. OpenShift only supports 3 control plane nodes for now.
- Having `failureDomains` in machinepool without `machinesSubnet` is an error.
- machinepool `zones` for both compute and storage can't be used together with machinepool `failureDomains`.
- Check that `subnets` in `failureDomains` actually exist (the same validation as machinesSubnet). Note that we can't easily verify if the subnet can actually be used for a given availability zone. This is up to the OpenShift administrators to figure out which subnet they can use for which domain.
- Check that `failureDomain` subnets have a matching CIDR in `machineNetwork`.

### Risks and Mitigations

- UX will be reviewed by Field Engineers, partnering with a customer who needs this feature. Also our QE will help to review it.

### Drawbacks

- The networking resources are pre-created and managed by the OpenStack administrators, and therefore not managed by the OpenShift cluster. Changes in the networking infrastructure will have to be planned and accordingly updated in the cluster.
  This is not a new requirement by the way, this comes from the fact it relies on the BYON (bring-your-own-network) feature. If a network goes on maintenance, the machines connected to that network will have to be migrated or redeployed else where.
- Adding new parameters (especially when they contain sub-parameters) is never easy for the users but we'll make sure to choose easy names and that they're all documented.

## Design Details

### Open Questions

- What maximum latency and bandwidth should we require between the failure domains?
- Where should the failure domains be defined? Potential places would be the platform or the machine pool scope.
- How do we specify where to create the API and Ingress VIPs ports? Is it OK to reuse machinesSubnet for this purpose? Isn't it confusing?
- Should we discover the CIDR of the provided subnets and add them to MachineNetwork CIDR list automatically?

### Test Plan

Beside unit tests in the installer, when possible, we'll have to create a new CI job that will run in an infrastructure that has multiple Failure Domains. We'll use the `mecha` cloud for this purpose.
The CI job will deploy a cluster using an install-config.yaml that is defined in this enhancement and then run e2e tests to validate that the cluster is healthy and working as expected. We'll ensure we have tests in [openstack-test][openstack-test] to validate the nodes were scheduled on separate Failure Domains as expected.
 
### Graduation Criteria

This enhancement will follow standard graduation criteria.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Community Testing
- Sufficient time for feedback
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Not applicable. The proposed change only affects the installer, and thus new clusters. There is no upgrade implication.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

Not applicable.

#### Failure Modes

Not applicable.

#### Support Procedures

TBD

## Implementation History

- Prototype is [WIP](https://github.com/openshift/installer/pull/6061)

## Alternatives

- Provide a manual procedure to stretch a control plane with UPI and on day 2

## Infrastructure Needed

Not applicable.

[bz-2095323]: https://bugzilla.redhat.com/show_bug.cgi?id=2095323
[openstack-test]: https://github.com/openshift/openstack-test
