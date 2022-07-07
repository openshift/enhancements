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
Each leaf brings their own network fabrics that are usually using their own power units. Also each network fabric manages their its own Layer-2 domain, and therefore network subnets. Routing protocols are usually used so applications can communicate each other in the datacenter, since they don't live within the same domain anymore.

In this enhancement, we would like to support the deployment of OpenShift into multiple subnets from day 1, using IPI.
To accomplish this, the OpenStack administrator will pre-create a Routed Provider Network and one or multiple subnets per leaf, which will be linked to an Availability Zone, here called Failure Domain for a better understanding.
Each Failure Domain will have its own network subnet and storage resources used by OpenShift.
This feature will remove the limitation that we have today in BYON (Bring your own network) which is the support of a single primary subnet for the machines on day 1. We want to be able to deploy the cluster across multiple subnets and storage sources, where each resource live in a given Failure Domain.

## Motivation

### User Stories

- As an administrator, I would like to stretch my OpenShift control plane across multiple Failure Domains (x3) so I increase the cluster resilience and performance.
- As an administrator, I would like to deploy my OpenShift workers across multiple Failure Domains (>1).

### Goals

- The user will be able to define Failure Domains in the `install-config.yaml` file used by the OpenShift Installer.
- The user will have the ability to deploy an OpenShift cluster across multiple availability zones which have their own networks and storage resources.

### Non-Goals

- The networks and subnets have to be pre-created in OpenStack Neutron.
- Routing between the networks is managed by the network infrastructure.
- Control-Plane VIP (e.g. API, ingress) route advertisement is not discussed in this enhancement and will
  be solved by another proposal. If needed, more options will be added to the Failure Domain block to allow more advanced configurations; but this is out of scope for now.

## Proposal

The Installer allow users to provide a list of Failure Domains which contain information about networking and storage.
The cluster will be deployed in these domains on day 1 via IPI.
The Installer will validate the information given by the user and return an error if a resource doesn't exist or
is incorrectly used.

The cluster will be deployed in the different Failure Domains, following the Round Robin process. The first master will be deployed in the first domain, and once we have used all domains, we come back to the first one, etc.

When using Failure Domains, the users will have to do it for both the masters and the workers.

The infrastructure resources owned by the cluster continue to be clearly identifiable and distinct from other resources.

Destroying a cluster must make sure that no resources are deleted that didn't belong to the cluster.  Leaking resources is preferred over any possibility of deleting non-cluster resources.

### Workflow Description

- The OpenStack administrators will deploy OpenStack with multiple Availability Zones, so Nova and Cinder can respectfully deploy servers and volumes in the Failure Domain.
- The OpenStack administrators will create at least one Routed Provider Network, and then multiple subnets, where each zone has a least one subnet.
- The OpenShift administrators will identify the Domain Failures: their availability zones, subnets, etc. They'll decide which ones will be used for the masters, and the ones for the workers (domains can be shared).
- The OpenShift administrators will provide the right configuration in the `install-config.yaml` file and then deploy the cluster (detailed later in this document).
- (Out of scope for this enhancement, see "Non-Goals") The OpenShift administrators will make sure that the network infrastructure has a networking route to reach the OCP Control Plane VIPs.

### API Extensions

Not applicable.


### Implementation Details/Notes/Constraints

#### Types

```golang
// OpenStackPlatformFailureDomainSpec holds the zone failure domain and
// the Neutron subnet of that failure domain.
type OpenStackPlatformFailureDomainSpec struct {
  // name defines the name of the OpenStackPlatformFailureDomainSpec
  Name        string `json:"name"`
  ComputeZone string `json:"computeZone"`
  StorageZone string `json:"storageZone"`
  Subnet      string `json:"subnet"`
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

Then they'll choose what Failure Domains to use when deploying the masters and workers directly in the machine pools:

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

The installer will create the control plane machines in a way that they can easily be adopted by the upcoming ControlPlane machineset.

When deploying nodes on separate subnets, the user will need to update the `machineNetwork` option to list all the possible CIDRs on which the machines can be. The `machineNetwork` is the source for creating the security groups rules during installation. There's a [bug][bz-2095323] in the OpenStack implementation that only takes into account the first CIDR that we'll need to fix.

However, once the bug will be fixed, the OpenShift administrators will have to provide a list of the CIDRs that will need access to the machine network:

```yaml
networking:
  ...
  machineNetwork:
  - cidr: 192.168.25.0/24
  - cidr: 192.168.26.0/24
  - cidr: 192.168.27.0/24
  - ...
```

For now, `machinesSubnet` is required to be set when deploying nodes on separate Failure Domains, and is then used to identify where to create the API and Ingress ports.

Also, the installer will provide some validations in order to avoid deployment errors:

- Having `failureDomains` in machinepool without `machinesSubnet` is an error.
- machinepool `zones` for both compute and storage can't be used together with machinepool `failureDomains`.
- Check that `subnets` in `failureDomains` actually exist (the same validation as machinesSubnet). Note that we can't easily verify if the subnet can actually be used for a given availability zone. This is up to the OpenShift administrators to figure out which subnet they can use for which domain.
- Check that `failureDomain` subnets have a matching CIDR in `machineNetwork`.

### Risks and Mitigations

- Backward compatibility will have to be maintained and the "old" way to define `zones` to remain available.
- UX will be reviewed by Field Engineers, partnering with a customer who needs this feature. Also our QE will
  help to review it.

### Drawbacks

- The networking resources are pre-created and managed by the OpenStack administrators, and therefore not managed by the OpenShift cluster. Changes in the networking infrastructure will have to be planned and accordingly updated in the cluster.
- Adding new parameters (especially when they contain sub-parameters) is never easy for the users but we'll make sure to choose easy names and that they're all documented.

## Design Details

### Open Questions

- Do we want to deprecate machinepool `zones` in favor of machinepool `failureDomains`?

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
