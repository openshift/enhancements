---
title: KubeVirt-platform-provider
authors:
  - "@ravidbro"
reviewers:

approvers:

creation-date: 2020-07-14
last-updated: 2020-07-14
status: implementable
---

# KubeVirt-platform-provider

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [OpenShift/docs]

## Open Questions [optional]

## Summary

This document describes how [KubeVirt][kubevirt-website] becomes an infra provider for OpenShift.

`KubeVirt` is a virtualization platform running as an extension of Kubernetes.

 We want to create a tenant cluster on top of existing OpenShift/Kubernetes cluster by creating
 virtual machines by KubeVirt for every node in the tenant cluster (master and workers nodes)
 and other Openshift/Kubernetes resources to allow **users** (not admins) of the infra cluster
 to create a tenant cluster as it was an application running on the infra cluster.
 We will implement all the components needed for the installer and cluster-api-provider
 for the machine-api to allow day 2 operations of resizing the cluster.


## Motivation

- Achieve true multi-tenancy of OpenShift were each tenant has dedicated control plane 
 and has full control on its configuration. 

### Goals

- provide a way to install OpenShift on KubeVirt infrastructure using 
  the installer - an IPI installation. (1st day)
- implementing a cluster-api provider to provide scaling and managing the cluster
  nodes (used by IPI, and useful for UPI, and also node management/fencing) (2nd day)
- Provide multi-tenancy and isolation between the tenant clusters

### Non-Goals
- UPI implementation will be provided separately.

## Proposal

This provider enables the OpenShift Installer to provision VM resources in 
KubeVirt infrastructure, that will be used as worker and masters of the clusters. It 
will also create the bootstrap machine, and the configuration needed to get
the initial cluster running by supplying a DNS service and load balancing.

We want to approach deployment on OpenShift and KubeVirt as deployment on cloud similar to the
deployments we have on public clouds as AWS and GCP rather than virtualization platform in a way 
that the machine's network will be private, and the relevant endpoints will be exposed out of the
cluster with platform services as we can or pods deployed in the infrastructure cluster to supply the services
as DNS and Loadbalancing.

We see two main network options for deployment over KubeVirt:
- Deploy the tenant cluster on the pods network and use OpenShift services and routes
to provide DNS and Load-Balancing.
- Deploy the tenant cluster on a secondary network (using Multus) and provide DNS service and Load-Balancing
as the same way as other KNI networking deployments using HAProxy, CoreDNS and keepalived running on the
tenant cluster VMs. See the [baremetal ipi networking doc][baremetal-ipi-networking]

 

### Implementation Details/Notes/Constraints [optional]

1. Survey

    The installation starts and right after the user supplies their public ssh key,
and then choose `KubeVirt` the installation will ask for all the relevant details
of the installation: **kubeconfig** for the infrastructure OpenShift, **namespace**, **storageClass**, 
 **networkName (NAD)** and other KubeVirt specific attributes. 
The installer will validate it can communicate with the api, otherwise it will fail to proceed.

   With that the survey continues to the general cluster name, domain name, and 
the rest of the non-KubeVirt specific question.

2. Resource creation - terraform

   Terraform uses kubernetes provider to create:

    - DataVolume with RHCOS image

         *Note:* In disconnected environment the user will need to provide a local image that the installer
          can upload to the namespace.
    - secrets for the ignition configs of the VMs
    - 1 bootstrap machine
    - 3 masters
    
    When installing on pods network:
    - Services and routes for DNS and LB

3. Bootstrap

    The bootstrap VM has a huge Ignition config set using terraform as secrets and is visible
as secrets on the infra OpenShift. KubeVirt boots that VM with that content as ConfigDrive and the 
bootstraping begins when the `bootkube.service` systemd service starts.

    This process described more thoroughly in the [installer overview document][https://github.com/OpenShift/installer/blob/37b99d8c9a3878bac7e8a94b6b0113fad6ffb77a/docs/user/overview.md#cluster-installation-process]

4. Masters bootstrap

    Master VMs are booting using a stub Ignition config that are waiting early in
the Ignition service to load their Ignition config from a URL. That url is the
 `https://<internal-api-vip>/config/master` which is still not available until
the **bootstrap** VM is exposing it. It takes few minutes till it does.

    With the machine config available the masters pull their Ignition and boots up
joining the tenant cluster as masters and start scheduling pods.

5. Workers bootstrap

    After the masters and the control plane is up, we will scale the machineset to create workers
    by the machine-api-operator.


### Risks and Mitigations

- Network

    - Pods network option
        -(OCP gap) The ports 22623/22624 that are used by MCS are blocked on the 
        pods network and prevent from the nodes to pull ignition and updates.
        - (KubeVirt gap) Interface binding - Currently the only supported binding on the pods
        network is masquerade which means that all nodes are behind NAT, each VM
        behind the NAT of his own pod.
        - (OpenShift/KubeVirt gap) Static IP - Currently OpenShift assumes that node's IP addresses are static,
         and the VM egress IP is always the pod IP which is changing every time the VM restarts (and new pod is being created).
        
    - Secondary network option (MULTUS)
    
        - With this approach admin of the infra cluster will need to be involved in
    the creation of each new tenant cluster since NADs need to be created and
    probably also nmstate will need to be used to create the topology on the hosts.
    In this proposal, we will assume that admin created the namespace and all network resources 
    before running the installer, and the created networkName (NAD) will be the input for the installer.  

    
- Storage

    CSI driver for `KubeVirt` is not available yet.


## Design Details

- Namespaces

    For each tenant cluster we will create a namespace with the ClusterID.

    *Open question: should the namespace creation done by the user or by the installer.*

- Images

    - Option 1 - For each namespace we will create DataVolume (CDI CRD) with RHCOS image which will be cloned for
    each VM, masters and workers.
    - Option 2 - We will use URL and each VM will pull the image for itself without the need for cloning.
 
- Network

    ####Option 1 - Pods network
    - set cluster baseDomain as svc.cluster.local so services that we will create
    as LoadBalancer will have the expected FQDN <service-name>.<namespace>.svc.cluster.local
    - Create VMs with one interface on pods network.
    - Create headless services for each VM to create DNS records for internal communication
    between the nodes.
    - Create services for 'api' and 'api-int' as loadbalancers between the masters
    with MCS port (26223) and API server port (6443)
    - Set ingress domain name for default router as subdomain of the ingress domain
    of the infra OCP.
    - Create in the underlying OCP route for each route in the provisioned cluster with
    hostname value as the route on the provisioned cluster.\
    Alternatively, if the infra OCP support wildcard route then one route from
    type subdomain can be defined for all the routes to the provisioned cluster.
    - Isolation will be achieved by creation of network policies that will allow traffic
    only between VMs(pods) that are related to the same provisioned cluster.
    
    ####Option 2 - Secondary network (Multus)
    - Create VMs attached to the secondary network (NAD) that was configured.
    - Isolation achieved by the secondary network, it's up to the admin to decide how to create the
    secondary networks that can use different VLAN/VXLAN/etc. 
    
- Storage

    The VMs boot volumes will be PVs allocated from the infra cluster
    
    For PVs requested for pods running on the tenant cluster we have a few options:

    ####Option 1 - Direct storage CSI 
   The provisioned cluster will use CSI to attach storage using network to the VM guests.
   This can be OCS CSI driver to consume storage from OCS installed on the infra
   OpenShift as a tenant of OCS or any other external storage.
   
   ####Option 2 - KubeVirt CSI driver
   Develop CSI driver for KubeVirt platform.
   
   This provider should forward the request to the infra cluster to allocate PV
   from the infra cluster storageClass and attach it to the relevant VM where the PV will be exposed to the guest
   as block device that the driver will attach to the requested pods.

- Anti-affinity

   The VMs will be scheduled with anti-affinity rules between the masters and between the workers in a way
   that we will strive to spread the masters between the infra cluster nodes and same for the workers
   to reduce the risk that outage of one worker in the infra cluster will cause major failure to a tenant.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

TODO

[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

TODO
  - Announce deprecation and support policy of the existing feature
  - Deprecate the feature

### Upgrade / Downgrade Strategy

TODO
  If applicable, how will the component be upgraded and downgraded? Make sure this
  is in the test plan.


  Consider the following in developing an upgrade/downgrade strategy for this
  enhancement:
  - What changes (in invocations, configurations, API use, etc.) is an existing
    cluster required to make on upgrade in order to keep previous behavior?
  - What changes (in invocations, configurations, API use, etc.) is an existing
    cluster required to make on upgrade in order to make use of the enhancement?

### Version Skew Strategy

TODO
  What are the guarantees? Make sure this is in the test plan.

  Consider the following in developing a version skew strategy for this
  enhancement:
  - During an upgrade, we will always have skew among components, how will this impact your work?
  - Does this enhancement involve coordinating behavior in the control plane and
    in the kubelet? How does an n-2 kubelet without this feature available behave
    when this feature is used?
  - Will any other components on the node change? For example, changes to CSI, CRI
    or CNI may require updating that component before the kubelet.

## Implementation History

June 12th 2019 - Presented a fully working POC

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

- CI
Running and end-to-end job is a must for this feature to graduate, and it is a
non trivial task. oVirt is not a cloud solution and we need to provide a setup
for a job invocation. We started with deploying a static oVirt deployment on GCP
and it is working and able start an installation which is initiated from outside
of GCP. Now we need to make sure the CI job can do the same.
What's left is to make sure this instance network setup can work with the floating
IP's we assign to it (the DNS and API VIPS). Currently we assume we can make that
work because we control the dnsmasq inside VMs network.

What could go wrong?
- we may not be able to make the CI play nicely on time and we need as much help
  and guidance here. 
- multi ci jobs running in parallel will deploy 4 VMs on the infra, and I don't 
know how will it handle the traffic and disk pressure. My guess is that we should
minimize the load by not supporting parallel job invocations. Not sure its viable.


[baremetal-ipi-networking]: https://github.com/OpenShift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md
[kubevirt-website]: https://kubevirt.io/