---
title: on-prem-configurable-lb-dns-stack
authors:
  - "@yboaron"
reviewers:
  - "@sttts"
  - "@deads2k"
  - "@celebdor"
  - "@bcrochet"
  - "@bnemec"
approvers:
  - "@"
creation-date: 2020-02-11
last-updated: 2020-02-11
status: implementable
---

# On-prem: configurable DNS/LB self-hosted stack

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In the current implementation,  the self-hosted DNS/LB stack runs under the MCO umbrella by default in all on-prem platforms and there's no option to configure/disable it.

Since the self-hosted LB doesn't support graceful node switchover (connections may break upon node reboot/shutdown) and some services are implemented using traffic (multicast) which may not be permitted on several costomers networks,  customers may prefer alternative Load Balancing and DNS.

This enhancement suggests a flexible configuration of the self-hosted stack to support the requirement of using alternative LB and DNS.

*Note:* as per the OCP plan to move API and Ingress to K8S SLB, this direction should be investigated deeper after MetalLb will be integrated.

## Motivation

Allow customers to adjust the self hosted DNS/LB stack according to the needs they really have instead of providing them in a generic way.

The next table illustrates the supported configuration possibilities:

| Component Name                                       | Current status         |  This proposal          |
| ---------------------------------------------------- |------------------------| ------------------------|
| Self-hosted LB and DNS                               |         V              |         V               |
| Self-hosted LB and DNS(without node names resolution)|         X              |         V               |
| external LB and DNS                                  |         X              |         V               |

### Goals

- Ability to use the self-hosted stack in all on-prem platforms.
- Ability to control the self-hosted stack configuration.
- Retrieve the self-hosted stack status through K8S API.
- Maintain a single code base/manifests for all the on-prem platforms.

### Non-Goals

- Define a new self-hosted stack for DNS or load balancing

## Proposal

The [Keepalived and infra DNS components](https://github.com/openshift/machine-config-operator/tree/master/manifests/on-prem) on the bootstrap node will keep running through the MCO.
However, the services on masters and workers nodes will be managed by both MCO and the new operator.

The following functionalities are expected as part of the new operator:

- Introduce a CRD used by the operator to manage the self-hosted stack.
- Fields in CR spec that control the self-hosted stack configuration.
- Fields in CR status that expose self-hosted stack status (e.g: which node holds the API-VIP).
- Run Coredns-MDNS, HAProxy and MDNS-publisher self-hosted stack components as static pods according to the CRD instance content.

## Risks and Mitigations

- Impact on on-prem platforms
- Upgrades/Downgrades

In order to mitigate the impact on on-prem platforms, in fresh deployments all the self-hosted LB, DNS, and VIP services will be enabled, therefore there is no need to rely on external services during deployment, also the on-prem platforms tests should be updated according to Test Plan section described below.

Additionally, a detailed document in which the supported LB, DNS migration processes will be issued.

## Design Details

### Open Questions [optional]

- Is `cluster-hosted-net-services-operator` an acceptable name?
- In which namespace the new operator components should be hosted?

### current implmentation

In current implementation, the [networking infra componenets](https://github.com/openshift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md) run as static pods through the MCO.

### Suggested design

To support early clustering requirements Keepalived will continue running as static pods through MCO, additionally, a new dnsmasq service (also deployed through MCO) will run in each node while HAProxy, CoreDNS-MDNS and MDNS-publisher will run as static pods through the new operator.

The following table reflects the suggested changes.

| Component Name       | Current implementation | Proposed implementation |
| ---------------------| -----------------------| ------------------------|
| Resolv.conf prepender|  NM dispatcher script  | NM dispatcher script    |
| Keepalived           |  Static pod            | Static pod              |
| HAProxy              |  Static pod            | Static pod              |
| CoreDNS-MDNS         |  Static pod            | Static pod              |
| mDNS-Publisher       |  Static pod            | Static pod              |
| dnsmasq              |   --                   | Systemd service         |

#### Self hosted DNS services

- The self hosted DNS functionality should be composed of the following components in each node:
  - A NetworkManager dispatcher script used to configure resolv.conf to point at the node's public IP address, see [self hosted DNS design](https://github.com/openshift/installer/blob/f6bb83784a835e1f4955ef41916793c0948fd4a1/docs/design/baremetal/networking-infrastructure.md#internal-dns) for more details.
  - Systemd service running dnsmasq
  - CoreDNS-mDNS pod

- The cluster-hosted-net-services-operator will be responsible only for the deployment of the CoreDNS pods while the NetworkManager dispatcher script and dnsmasq Systemd service will run through MCO.
- To avoid port conflict, Dnsmasq service should listen on DNS well-known port (53) and CoreDNS-MDNS should listen on other port (e.g: 5353).
- The new Dnsmasq service should be responsible to resolve 'api-int' (to API-VIP value provided in install-config) hostname when CoreDNS-MDNS pod isn't running on the node.
- CoreDNS-MDNS should register itself as an upstream server in dnsmasq [via dbus interface](https://github.com/imp/dnsmasq/blob/master/dbus/DBus-interface#L51) for ClusterName.DomainName

#### Self hosted API Loadbalancer

- The current self hosted LB implementation (based on Keepalived and HAProxy) doesn't support graceful switchover, which means connections will break upon shutdown of node holds the VIP.
- The self hosted API loadbalancer will run equally to the current mode.
- API should be available even when HAProxy pods don't run.
- Additional functionality aspects of the self-hosted Loadbalancer maybe further analyzed upon need in a separate enhancement.

#### Disaster recovery support

In order to support disaster recovery kubelet, kube-controller-manager and kube-scheduler should be able to communicate with kube-api even when all cluster-hosted-net-services-operator pods are shutdown.

The following steps will ensure that kube-api is available post disaster recovery:

- Dnsmasq service and Keepalived static pods will start running on master nodes
- API-VIP address will be assigned to one of the master nodes by Keepalived
- Dnsmasq should resolve api-int hostname to API-VIP address

The above steps should be applied also for cluster shutdown and resume use-case.

### cluster-hosted-net-services operator

The `cluster-hosted-net-services-operator` is a new operator whose operand is a controller for the cluster hosted DNS, LB and VIP services.

### Standard SLO Behavior

The cluster-hosted-net-services-operator is expected to follow the standard behavior of SLO, additionally, since the cluster-hosted-net-services-operator is applicable only to clusters running on on-prem ( 'baremetal', 'openstack', 'vsphere' or 'ovirt') infrastructure,
It should support also the expected behaviors for "not in use" SLOs similar to the [CBO operator](https://github.com/openshift/enhancements/blob/master/enhancements/baremetal/an-slo-for-baremetal.md#standard-slo-behaviors).

### cluster-hosted-net-services-operator details

The cluster-hosted-net-services-operator should:

- Be a new `openshift/cluster-hosted-net-services-operator` project.
- Publish an image called `cluster-hosted-net-services-operator`.
- Add a new `ClusterOperator` with an additional
  `Disabled` status for non on-prem platforms.
- Implement a controller reconciling a singleton instance representing the desired cluster hosted LB/DNS/VIP configuration.
- Do nothing except set `Disabled=true`, `Available=true`, and
  `Progressing=False` when the `Infrastructure` resource is a platform
  type other than openstack, baremetal, ovirt or vsphere.
- Update `ClusterOperator` DEGRADED field in accordance with the following healthchecks (in case self-hosted stack is enabled):
  - api-int name is resolvable in all nodes
  - kube-api is available via the VIP

#### CRD definition

Below is a possible option for a CRD instance of this operator:

```yaml
apiVersion: operator.openshift.io/v
kind: HostedNetServices
metadata:
  name: cluster
spec:
  dns: 
    nodesResolution: Enabled
    appsResolution: Enabled
    apiintIpAddress: 192.168.111.5 
  ingressHa: Enabled
status:
  apiServerInternalIpOwner: master-0
  ingressIpOwner: worker-1
```

With this sample manifest the cluster-hosted-net-services-operator should:

- Create DNS hosted service that will:

  - resolve node names and .apps wildcard record
  - resolve api-int to 192.168.111.5.
  - Run the self-hosted Loadbalancer for api only if apiintIpAddress is equal to API-VIP value provides in install-config file.
  - Provide high availability for default ingress.

From the status section we can see that master-0 node currently owns api-int VIP and worker-1 node owns ingress VIP. Additional spec and status fields may be introduced based on requirements.

#### Managing  the cluster hosted networking services

All the LB, DNS, and VIP components will be enabled by default in case of fresh deployment as today, while post-deployment the admin can control the services by editing the singleton CR.

##### API load balancer migration

Note: API LB migration is a significant infrastructure change that may cause an API outage for some time. There is no requirement for the process to take place without service disruption.

-- self-hosted to an external Load balancer

In order to migrate the self-hosted LB to an external Load balancer the admin should:

- Provide new IP address (!= API-VIP from install-config) pointing to external Load balancer front end.
- Edit apiintIpAddress value to use this IP address.

As a result of this change the cluster-hosted-net-services-operator should:

- First, verify that IP address is valid and kube-api is available using the new IP address.
- Update CoreDNS to resolve api-int to the new IP address
- Remove the HAProxy pods after graceful shutdown timeout
- Keepalived static pods will continue running

-- external to self-hosted Load balancer

To re-activate the self-hosted Load balancer the admin should set apiintIpAddress value to API-VIP address provided in install-config file and verify that external Load balancer is operational for graceful timeout.

Post this change the cluster-hosted-net-services-operator should run the same procedure described above and run the HAProxy pods.

### User Stories

#### As an admin I want to disable the self-hosted on-prem LB and DNS stack

#### As an admin I want to use the self-hosted on-prem LB and DNS stack also in UPI deployments

#### As an admin I want to retrieve the self-hosted stack status through K8S API

### Test Plan

The operator will be tested in the OpenShift CI via e2e tests triggered using the [e2e-metal-ipi](https://github.com/openshift/release/blob/master/ci-operator/step-registry/baremetalds/e2e/baremetalds-e2e-workflow.yaml) workflow defined in the OpenShift CI steps registry and also by e2e tests for openstack, ovirt and vsphere.

Additionally, the following tests should be added:

- Check each platform when external Load balancer is used
- Disruptive test that switch on/off the self-hosted components

### Upgrade / Downgrade Strategy

Prior to this proposal, the self-hosted stack implementation was running through the MCO.
The implementation of the self-hostd stack using the cluster-hosted-net-services operator might lead to cases during upgrade/downgrade in which two parallel components are running the self-hosted stack.

A. If the release includes the cluster-hosted-net-services operator that means MCO in this release shouldn't include the HAProxy, CoreDNS and MDNS-Publisher static pods.

B. The cluster-hosted-net-services operator will:

- Have lower priority than MCO, meaning it will be applied before MCO.
- Run the CoreDNS and HAProxy instances in each node following verification that the corresponding instance created by MCO isn't running anymore in this node (this should be applicable since both static and DaemonSet pod run on host's networking and listen on a well-known port)

A and B will ensure that :

- LB and DNS services will be available even when both MCO and cluster-hosted-net-services operator run the self-hosted stack.
- Following upgrade to a release includes cluster-hosted-net-services operator only HAProxy ,CoreDNS and MDNS-Publisher DaemonSet pods will run.

## Alternatives

Enabling/disabling of the self-hosted stack could be provided also by extending the monitor containers functionality.
Currently, the self-hosted stack is implemented by: Keepalived, HAProxy, MDNS-publisher and CoreDns static pods, each one of these pods includes a monitor container which is responsible for rendering its configuration.

It's possible to support enabling/disabling of the self-hosted stack in the following way:

- Addition of new fields to the [controllerconfig MCO CR](https://github.com/openshift/machine-config-operator/blob/master/manifests/controllerconfig.crd.yaml).
- The monitor container in each static pods should enable/disable the relevant functionality based on these fields content.

The next section summarizes the pros and cons of this method.

### Pros and Cons

Pros:

- Ability to disable LB/DNS services.
- Pods wo'nt consume actual CPU resources when service is disabled.
- No need for a separate operator.
- All the early clustering corner cases already covered and tested.
- Easier upgrade/downgrade path since all the components continue to run under the same operator (MCO).

Cons:

- Self-hosted stack configuration still coupled with platform.
- Not a typical operator controller implementation.
- Resources(memory/CPU) for static pods are allocated even if self-hosted stack is disabled.
- Is it acceptable to add these fields to the MCO CR ?
