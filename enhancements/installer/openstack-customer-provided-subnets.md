---
title: allow-customer-provisioned-aws-subnets
authors:
  - "@wking"
reviewers:
  - "@mandre"
  - "@pierreprinetti"
  - "@adduarte"
approvers:
  - "@abhinavdahiya"
  - "@sdodson"
creation-date: 2020-03-12
last-updated: 2020-03-12
status: planning
---

# 4.5: OpenStack: Allow Customer Provisioned Subnets

## Release Signoff Checklist

- [ x ] Enhancement is `implementable`
- [ x ] Design details are appropriately documented from clear requirements
- [ x ] Test plan is defined
- [ x ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
In many large organizations, IT groups are separated into teams. A common separation of duties lies between networking/WAN and application or enabling technology deployments. One group will provision or assign OpenStack accounts with core management technology. A second group will be responsible for provisioning networks complete with subnet & zone selection, routing, VPN connections, and ingress/egress rules from the company. Lastly, an applications team (or individual business units) are given a quota for other OpenStack resources to provision application-specific items (instances, containers, databases, pipelines, load balancers, etc.).

Another example of this split is found in companies that provide public access to their OpenStack resources, giving their tenants/customers quota on compute and storage resources but providing pre-defined networks for them.

In these scenarios, only OpenStack administrators will manage these networks, which will be generally provider networks, (as opposed to tenant networks), since they require physical network infrastructure configuration (typically VLANs). So when OpenStack tenants log in they will find them pre-created, shared with them and ready to use.

To accomplish this, our infrastructure needs an OpenStack account access that can be separated into "networking infrastructure" versus "application". Networking infrastructure includes elements related to the networks (networks, subnets, routing tables, internet gateways, NAT, VPN). Application items include OpenStack resources directly touching nodes within the cluster (LBs, security groups, Swift containers, nodes). This is a typical separation of responsibilities among IT groups and deployment/provisioning pipelines within large companies.

## Motivation

### Goals
As an administrator, I would like to create or reuse my own networks, subnets and routing schemes that I can deploy OpenShift to.

### Non-Goals
There are no changes in expectations regarding publicly addressable parts of the cluster.

## Proposal
- Installer allow users to provide a list of subnets that should be used for the cluster. Since there is an expectation that networking is being shared, the installer cannot modify the networking setup, but changes required to the shared resources like Tags that do not affect the behavior for other tenants of the network will be made.
- The installer validates a set of bare minimum assumptions about the networking setup
- Non-networking infrastructure resources that are specific to the cluster and resources owned by the cluster will be created by the installer. So resources like security groups, roles, RHCOS boot images, and ignition storage objects.
- The infrastructure resources owned by the cluster continue to be clearly identifiable and distinct from other resources.
- Destroying a cluster must make sure that no resources are deleted that didn't belong to the cluster. Leaking resources is preferred over any possibility of deleting non-cluster resources.


## User Stories
1. As an administrator of a private openstack cloud, I use provider networks for my openstack cluster, and would like to be able to install OpenShift in this environment.
2. As an OpenStack tenant, I want to be able to use pre-configured network resources so that I dont have to create new resources to install an OpenShift cluster.

## Implementation Details Notes/Constraints

### Installer Resources
The user provides a list of subnets to the installer in the openstack platform section of the install-config.yaml.

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  openstack:
    region: us-west-2
    subnets:
    - subnet-1
    - subnet-2
    - subnet-3
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

To make this work optimally, we should also enable customers to pass custom IP addresses to be used for our loadbalancing VIPs. This allows users who want to provision their own networks to set the CIDRs of the allowed address pairs that the VIPs can be drawn from.

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  openstack:
    region: us-west-2
    VIPAddresses:
      - "192.168.30.1/19"
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

### Subnets
- Subnets must all be a part of the same OpenStack cloud
- Each subnet must be able to be tagged, and have the capacity for at least one tag. This allows us to add the `kubernetes.io/cluster/.*:` shared tag to identify the subnet as part of the cluster. The k8s cloud provider code in kube-controller-manager uses this tag to find the subnets for the cluster to create LoadBalancer Services.
- The CIDR block for each one must be in MachineNetworks.
- The host running the installer needs to be able to reach the IP addresses assigned to the master nodes.
- No two public subnets or two private subnets can share a single availability zone, because that might be an error for future cloud provider load-balancer allocation.
- The installer should be able to support customer provided provider networks, as  well as tenant networks. 
- The subnet will have to allow for traffic to be routed to the VIPs that the installer uses for its dns solution. In IPI we currently manage this with an allowed address pair.

### Resources Created by the Installer
When the installer is passed a set of subnets, it will no longer create a network, a subnet on that network, the ports on that subnet, the routing, or the floating ips on those networks.  

The installer will continue to create:
- Images
- Security Groups
- Security Group Rules
- Images
- Volumes
- Nodes
- Boot Metadata

### Destroying the Cluster
We are operating under the assumption that the users that provisioned their custom subnets will also want to manually deprovision them. Therefore, no resources will be deleted in the custom network’s or subnets that are provided to the installer. We will remove any tags we added though.

All other resources will be destroyed normally.

### Limitations
Subnets must provide networking (ports and routers). At least for the initial implementation, we will not validate this assumption, will attempt to install the cluster regardless, and will fail after having created cluster-owned resources if the assumption is violated. Future work can iterate on pre-create validation for the networking assumptions, if they turn out to be a common tripping point.

## Risks and Mitigations
Deploying OpenShift clusters to pre-existing network has the side-effect of reduced the isolation of cluster services.
- ICMP ingress is allowed to entire network.
- TCP 22 ingress (SSH) is allowed to the entire network. We can restrict this to control plane and compute security groups, and require folks setting up a SSH bastion to either add one of those security groups to their bastion instance or alter our rules to allow their bastion.
- Control plane TCP 6443 ingress (Kubernetes API) is allowed to the entire network.
- Control plane TCP 22623 ingress (MCS) is allowed to the entire network. This should be restricted to the control plane and compute security groups, instead of the current logic to avoid leaking sensitive Ignition configs to non-cluster entities sharing the provider network.

## Design Details

### Test Plan
1. Proof of valid case: Update CI to use a UPI script to create a correct set of networks and subnets and pass them to the installer for an e2e installation. 
2. IF Validation: Proof of Invalid case: Create a test to create incorrect subnets and ensure that installer fails before trying to deploy infrastructure.

### Garduation Criteria
This enhancement will follow standard graduation criteria.

#### Dev Preview --> Tech Preview
- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview --> GA
- Community Testing
- Sufficient time for feedback
- Upgrade testing from 4.5 clusters utilizing this enhancement to later releases
- Downgrade and scale testing are not relevant to this enhancement

## Draw Backs
Customer owned networking components means the cluster cannot automatically change things about the networking during upgrade. This should be documented clearly so the customer is aware.
