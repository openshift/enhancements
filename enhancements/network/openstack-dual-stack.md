---
title: openstack-dual-stack
authors:
  - "@maysamacedo"
reviewers:
  - "@mandre for SME review on OpenStack and general feedback"
  - "@cybertron for SME review on on-premise networking"
  - "@patrickdillon for SME review and approval on installer"
approvers:
  - "@mandre"
api-approvers:
  - "None"
creation-date: 2023-03-13
last-updated: 2023-03-13
tracking-link:
  - https://issues.redhat.com/browse/OCPBU-199
  - https://issues.redhat.com/browse/OSASINFRA-1938
---

# OpenStack dual-stack

Covers adding support of OpenShift dual-stack networking on OpenStack.

## Summary

Customers need support for OpenShift clusters running on OpenStack with IPv4 and IPv6.

## Motivation

Customers need dual-stack support to allow migration of workloads from IPv4 to IPv6
as they are running out of IPv4 addresses. This will allow OpenShift on OpenStack to
serve a wider variety of workloads, like the ones needed by telco.

### User Stories

* As an user of a IPI OpenShift cluster, I want to have ingress and egress working with dual-stack.
* As an user of a IPI OpenShift cluster, I want clusterIP services to be working with dual-stack.
* As an user of a IPI OpenShift cluster, I want LoadBalancer type services to be working with dual-stack.
* As an user of a IPI OpenShift cluster, I want to be able to convert a single stack cluster to dual-stack and the way back.

### Goals

- OpenStack VMs are configured with one dual-stack interface.
- The IPv6 Subnets can be configured with: Stateless, SLAAC or Stateful.
- The addresses on the VM's interface are configured with the same address
assigned by OpenStack to the port.
- Nodes are configured with both IPv4 and IPv6 addresses, without exposing
additional addresses, like storage interfaces.
- Pods are configured with both IPv4 and IPv6 addresses from the cidrs defined on the `clusterNetwork` or the Nodes Addresses.
- A pre-existing dual-stack network is used for the OpenShift cluster

### Non-Goals

- Support for OpenStack endpoints configured with dual-stack.
- Installer (IPI) handles creation of the dual-stack OpenStack Network and Subnets.

## Proposal

Customers will be able to specify the IPv4 and IPv6 Subnets
meant to be used in the `install-config.yaml`.

### Workflow Description

1. The customers will configure a network (tenant or provider) with IPv4 and
IPv6 subnets. The IPv6 address mode for the Subnet can be of any type:
SLAAC, Stateless or Stateful. Here is a sample of network and subnets creation:

```bash
$ openstack network create dual-stack
$ openstack subnet create --network dual-stack --subnet-range 10.0.0.0/16 ipv4-subnet
$ openstack subnet create --network dual-stack --ipv6-address-mode slaac --ipv6-ra-mode slaac --subnet-range 2001:db8:2222:5555::/64 --ip-version 6 ipv6-subnet
```

2. The customer will create API and Ingress Ports using the dual-stack network created in the
previous step. Here is a sample of ports creation:

```bash
$ openstack port create api --network dual-stack -f value -c fixed_ips
[{'subnet_id': 'eac94701-fd9c-4d7b-a0a6-292506e19ea9', 'ip_address': '10.0.0.239'}, {'subnet_id': '70728437-9708-42ec-bf4a-b844e3846190', 'ip_address': '2001:db8:2222:5555:f816:0000:0000:0000'}]
```

```bash
$ openstack port create ingress --network dual-stack -f value -c fixed_ips
[{'subnet_id': 'eac94701-fd9c-4d7b-a0a6-292506e19ea9', 'ip_address': '10.0.0.147'}, {'subnet_id': '70728437-9708-42ec-bf4a-b844e3846190', 'ip_address': '2001:db8:2222:5555:f816:1111:1111:1111'}]
```

3. The customer will write the `install-config.yaml` and add the details of the control plane port to the new field `controlPlanePort`, which contains Filters of ID and/or Name for the Network and Subnets. The `controlPlanePort` API definition is consolidated to look similar to `PortOpts` field in the Machine's `OpenstackProviderSpec`.

```yaml
platform:
  openstack:
    ingressVIPs: ['10.0.0.147', '2001:db8:2222:5555:f816:1111:1111:1111']
    apiVIPs: ['10.0.0.239','2001:db8:2222:5555:f816:0000:0000:0000']
    controlPlanePort:
      fixedIPs:
      - subnet:
          id: eac94701-fd9c-4d7b-a0a6-292506e19ea9
      - subnet:
          id: 70728437-9708-42ec-bf4a-b844e3846190
      network:
        name: hostonly
```

The generated Machine will look like the following:

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  creationTimestamp: null
  labels:
    machine.openshift.io/cluster-api-cluster: ostest-h8q6h
    machine.openshift.io/cluster-api-machine-role: master
    machine.openshift.io/cluster-api-machine-type: master
  name: ostest-h8q6h-master-0
  namespace: openshift-machine-api
spec:
  lifecycleHooks: {}
  metadata: {}
  providerSpec:
    value:
      apiVersion: machine.openshift.io/v1alpha1
      cloudName: openstack
      cloudsSecret:
        name: openstack-cloud-credentials
        namespace: openshift-machine-api
      flavor: m1.xlarge
      image: ostest-h8q6h-rhcos
      kind: OpenstackProviderSpec
      metadata:
        creationTimestamp: null
      networks:
      - filter:
          name: hostonly
        subnets:
        - filter:
            id: db333b56-1fb1-4fd6-a74c-65dfb9a49137
        - filter:
            id: f4e1fe8c-2230-44dc-b57d-c126a857261c
      primarySubnet: db333b56-1fb1-4fd6-a74c-65dfb9a49137
      securityGroups:
      - filter: {}
        name: ostest-h8q6h-master
      - filter: {}
        uuid: 32f60fe4-5260-4489-a36c-aaaa3fb24215
      serverGroupName: ostest-h8q6h-master
      serverMetadata:
        Name: ostest-h8q6h-master
        openshiftClusterID: ostest-h8q6h
      tags:
      - openshiftClusterID=ostest-h8q6h
      trunk: true
      userDataSecret:
        name: master-user-data
status: {}
```
4. The installer will validate the network, subnets and VIPs, add the security group to the api and ingress
ports and link the floating IPs to those Ports, when necessary. Also, create the additional IPv6 security group rules
and configured the servers with dual-stack addresses, add the dual-stack allowed address pairs and
include in the bootstrap node the following Network Manager configuration:

```ini
[connection]
type=ethernet
[ipv6]
method=auto
```
5. The machine-config-operator will configure the ethernet connection following the OpenStack [guidelines](https://docs.openstack.org/neutron/wallaby/admin/config-ipv6.html#configuring-interfaces-of-the-guest) to make sure the generated IPv6 addresses match the ones configured in OpenStack.
6. The cluster-cloud-controller-manager will enable the `CloudDualStackNodeIPs` feature gate for the openstack-cloud-controller-manager.

### API Extensions

The `machinesSubnet` field, which is available in the `install-config.yaml` under
`platform` and `openstack` will be deprecated in favor of a new field named `controlPlanePort`
to persist both IPv4 and IPv6 provided Subnets.

### Implementation Details/Notes/Constraints

The idea behind using bring your own network instead of basic IPI installation is to allow support
for a wider variety of ways on how to configure the Network and Subnet. As an example, the Ports have
to be pre-created by the user because when using SLAAC or Stateless the specification of fixed-ips during
Port creation is not allowed. UPI can also be an option in future releases.

### Risks and Mitigations

The support for dual-stack in Kubelet's node-ip setting is a [feature in alpha state in Kubernetes v1.27](https://kubernetes.io/docs/concepts/services-networking/dual-stack/#configure-ipv4-ipv6-dual-stack) and requires a [fix to merge](https://github.com/kubernetes/kubernetes/pull/118662).

### Drawbacks

NA

## Design Details

### Open Questions

### Test Plan

- Develop CI for basic dual-stack installation with Provider Network.

### Graduation Criteria

The Tech-Preview target is OCP 4.14 and GA would be 4.15.

#### Dev Preview -> Tech Preview

- Succesfully bring up a day 1 dual-stack OpenStack cluster without manual intervation
  of the user with exception of any documented requirements.
- End user documentation
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Sufficient time for feedback
- Migration from single-stack to dual-stack and the other way around
- Support IPv6 primary dual-stack deployments
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

NA

### Upgrade / Downgrade Strategy

It's not possible to upgrade directly from one version using single-stack to another version with dual-stack.
The customer would have to first upgrade to a version that has support for dual-stack, but maintaining the single-stack.
After upgrade the customer could follow [the conversion procedure to dual-stack](https://docs.openshift.com/container-platform/4.12/networking/ovn_kubernetes_network_provider/converting-to-dual-stack.html).

### Version Skew Strategy

NA

### Operational Aspects of API Extensions

NA

#### Failure Modes

NA

#### Support Procedures

## Implementation History

## Alternatives


