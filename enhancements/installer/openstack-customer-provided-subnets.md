---
title: support-provider-networks-and-custom-networks
authors:
  - "@egarcia"
reviewers:
  - "@mandre"
  - "@pierreprinetti"
  - "@adduarte"
approvers:
  - "@abhinavdahiya"
creation-date: 2020-03-12
last-updated: 2020-03-20
status: planning
---

# 4.5: OpenStack: Support Provider Networks and Custom Subnets

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
As an OpenStack user, I would like to deploy OpenShift onto pre-existing OpenStack networks, subnets and routing schemes.

### Non-Goals
There are no changes in expectations regarding publicly addressable parts of the cluster.

## Proposal
- Installer allow users to provide a subnet that should be used for the cluster. 
- In order to support provider networks in IPI, the subnet passed to the installer must meet these requirements:
  - have the capacity and ability for the installer to provision ports on the nodes subnet
  - have dhcp enabled
  - the installer must be able to send and recieve https traffic to nodes connected to ports on the nodes subnet
- In order to support provider networks in UPI, additional documentation will be provided to support this use case
- For those who do not want the installer to provision their networking resources, we will provide additional UPI documentation.
- The infrastructure resources owned by the cluster continue to be clearly identifiable and distinct from other resources.
- Destroying a cluster must make sure that no resources are deleted that didn't belong to the cluster. Leaking resources is preferred over any possibility of deleting non-cluster resources.


## User Stories
1. As an administrator of a private openstack cloud, I use provider networks for my openstack cluster, and would like to be able to install OpenShift in this environment.
2. As an OpenStack tenant, I want to be able to use pre-configured network resources so that I dont have to create new resources to install an OpenShift cluster.

## Implementation Details Notes/Constraints

### Installer Resources
If a user wants to use a pre-existing subnet, they can provide the installer with the ID of that subnet. The installer needs to have the authorization and quota to provision the necessary ports and resources on that subnet.

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  openstack:
    nodesSubnet: abcd-4321
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

Clusters with provider networks often do not use floating IPs, and may have flat networking schemes. In order to accound for this, we will have to make floating IPs in the cluster optional, as well as the external network. so the first change we will make to address this is to make the parameter `platform.openstack.lbFloatingIP` optional. When unset, the installer will not attempt to attach a floating IP to the API port. Likewise, we will have to make the variable `platform.openstack.externalNetwork` optional. When this is unset, the installer will no longer create and attach a floting IP to the bootstrap node.


To allow the customer to set up their own custom external access infrastructure, we will have to let them set the `apiVIP` and `ingressVIP` port IPs ahead of the install, so that they can be added to their routing/loadbalancing scheme before the installer is run.

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  openstack:
    apiVIP: "192.168.30.15"
    ingressVIP: "192.168.30.17"
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

### Nodes Subnet
- Nodes Subnet must all be a part of the same OpenStack cloud
- The subnet must be able to be tagged, and have the capacity for at least one tag. This allows us to add the `kubernetes.io/cluster/.*:` shared tag to identify the subnet as part of the cluster. The k8s cloud provider code in kube-controller-manager uses this tag to find the subnets for the cluster to create LoadBalancer Services.
- The CIDR block must be in MachineNetworks.
- The host running the installer needs to be able to send HTTP requests to the API.
- The installer should be able to support subnets on provider networks, as well as tenant networks.

### Resources Created by the Installer
When the installer is passed a set of subnets, it will no longer create a network, a subnet on that network, the ports on that subnet, the routing, or the floating ips on those networks.  

The installer will continue to create:
- Images
- Security Groups
- Security Group Rules
- Ports
- Volumes
- Nodes
- Boot Metadata

### Installs that need more control
For the use case where cluster operators do not want the installer creating any resources on their networks, we want them to use the UPI installer, since this usage pattern does not fit in to the IPI vision of the installer. To support this use case, we will provide additional UPI documentation.

### Destroying the Cluster
We are operating under the assumption that the users that provisioned their custom subnets will also want to manually deprovision them. Therefore, no resources will be deleted in the custom network’s or subnets that are provided to the installer. We will remove any tags we added though.

All other resources will be destroyed normally.

### Limitations
This does add a bit of complexity to the install process, so it is important that we have good validation and clear documentation to help users navigate this feature.

## Risks and Mitigations
Deploying OpenShift clusters to pre-existing network has the side-effect of reduced the isolation of cluster services.
- ICMP ingress is allowed to entire network.
- TCP 22 ingress (SSH) is allowed to the entire network. We can restrict this to control plane and compute security groups, and require folks setting up a SSH bastion to either add one of those security groups to their bastion instance or alter our rules to allow their bastion.
- Control plane TCP 6443 ingress (Kubernetes API) is allowed to the entire network.
- Control plane TCP 22623 ingress (MCS) is allowed to the entire network. This should be restricted to the control plane and compute security groups, instead of the current logic to avoid leaking sensitive Ignition configs to non-cluster entities sharing the provider network.

## Design Details

### Test Plan
1. e2e test: Update CI to use a UPI script to create a correct set of networks and subnets and pass them to the installer for an e2e installation
2. Unit tests for new parameters:
   1. `apiVIP` and `ingressVIP` must be valid IP addresses in the nodesSubnet CIDR
   2. `apiEndpoint` must be a valid IP address or URL
   3. `nodesSubnet` must be the uuid of a subnet that exists in the openstack cloud that the installer has access to
   4. When `externalNetwork` is set, then `lbFloatingIP` also needs to be set. This goes both ways.
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
This adds a bit of complexity to the user experience. It also does not support customers having full control over their networks in IPI, we are aware of other platforms going this route, but are not aware of an openstack IPI version of this use case at the time of writing.
