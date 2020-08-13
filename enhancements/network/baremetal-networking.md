---
title: in-cluster-network-infrastructure
authors:
  - "@jcpowermac"
  - "@yboaron"
reviewers:
  - "@abhinavdahiya"
approvers:
  - "@abhinavdahiya"
creation-date: 2019-12-10
last-updated:  2019-12-12
status: implemented
see-also:
  - "https://github.com/openshift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md"
  - "https://github.com/openshift/installer/blob/master/docs/design/openstack/networking-infrastructure.md"
  - "https://github.com/openshift/enhancements/pull/61"
replaces:
  - ""
superseded-by:
  - ""
---

# In Cluster Network Infrastructure

[comment]: <> (or internal network services, or internal networking infrastructure or non-cloud network{,ing} {services,infrastructure})

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Many customers still have on-premise infrastructure which includes bare metal, VMware vSphere,
OpenStack and Red Hat RHV.  These customers would like to use the IPI installation approach while utilizing
their existing environments. This enhancement would provide networking infrastructure including DNS
and load balancing required for OpenShift to compute environments that do not provide such services.

## Motivation

### Goals

Install an IPI OpenShift cluster on various on-premise non-cloud platforms that
provides internal DNS and load balancing that is minimally required for OpenShift
cluster to run properly. OpenShift installation will still require DHCP
and DNS entries for api and the apps wildcard url.

The minimal requirements includes:
* Internal DNS:
  - hostname resolution for masters and workers nodes.
  - `api-int` hostname resolution.
* Highly available load-balancing API access for internal clients.
* Highly available access for default ingress. 

### Non-Goals

## Proposal

In cluster network infrastructure automates a number
of capabilities that are handled on other platforms by cloud infrastructure services.
These capabilities include:
* Highly available load-balanced api access
* Highly available ingress access
* Internal DNS support

The assets needed for these capabilites implementation are rednered by the [MCO](https://github.com/openshift/machine-config-operator) and the [baremetal-runtimecfg](https://github.com/openshift/baremetal-runtimecfg) is used significantly within the manifests and templates of the machine-config-operator project.

The `baremetal-runtimecfg` is further described in a section below.
VIPs (Virtual IP) and keepalived are used to provide high availability.


### Virtual IP addresses and Keepalived

#### Keepalived

[keepalived](https://keepalived.org) provides high-availablity for services by using a single
"virtual" ip address (VIP) that can failover between multiple hosts using
[VRRP](https://www.haproxy.com/documentation/hapee/1-5r2/configuration/vrrp/#understanding-vrrp)

#### Virtual IP addresses 

A VIP (Virtual IP) is used to provide failover of the service across the relevant machines
(including the bootstrap instance).
Two VIPs are supported:
* api-vip
* ingress-vip

##### api-vip

The api-vip is used for API communication and it is provided by the user
via the `install-config.yaml` [parameter](https://github.com/openshift/installer/blob/master/pkg/types/baremetal/platform.go#L86-L89)
or `openshift-installer` terminal prompts. 

The api-vip can be either private or public IPv4/IPv6 address.
The external `api.$cluster_name.$base-domain` DNS record should point to api-vip.

NOTE: This needs clarification. While we could assume the api-vip could be a public-facing internet address it seems that it more likely to be configured to a private address.

##### ingress-vip

The ingress-vip is used for ingress access and it also provided by the user via the `install-config.yaml` [parameter](https://github.com/openshift/installer/blob/master/pkg/types/baremetal/platform.go#L91-L94)
or `openshift-installer` terminal prompts.

The ingress-vip can be either private or public IPv4/IPv6 address, the external wildcard `*.apps.$cluster_name.$base-domain` DNS record should point to ingress-vip.

### Highly available load-balanced API access

Access to the Kubernetes API (port 6443) from clients both external
and internal to the cluster should be highly available load-balanced across control
plane machines.

#### API high availability

The api-vip first resides on the bootstrap instance.
This `keepalived` [instance](https://github.com/openshift/machine-config-operator/blob/master/manifests/baremetal/keepalived.yaml)
runs as a [static pod](https://kubernetes.io/docs/tasks/administer-cluster/static-pod/) and the
[relevant assets](https://github.com/openshift/machine-config-operator/blob/master/manifests/baremetal/keepalived.conf.tmpl#L7)
are rendered by the Machine Config Operator.

The control plane [pod](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-keepalived.yaml)
is configured via the
[baremetal-keepalived-keepalived.yaml](https://github.com/openshift/machine-config-operator/blob/master/templates/master/00-master/baremetal/files/baremetal-keepalived-keepalived.yaml)
ignition template.
The control plane keepalived configuration uses service checks to either add or remove points to the instance weight.
Currently the service checks include:
- curl locally the api on port 6443 and check local HAProxy instance health endpoint 

The VIP will move to one of the control plane nodes, but only after the
bootstrap process has completed and the bootstrap instance is stopped. This happens
because the `keepalived` [instances](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-keepalived.yaml)
on control plane machines are [configured](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-keepalived.yaml)
with a lower [VRRP](https://en.wikipedia.org/wiki/Virtual_Router_Redundancy_Protocol)
priority. This ensures that the API on the control plane nodes is fully
functional before the api-vip moves.

#### API load-balancing

Once the api-vip has moved to one of the control plane nodes, traffic sent from clients to this VIP first hits an `haproxy` load balancer running on that control plane node.
These [instances](https://github.com/openshift/machine-config-operator/blob/master/templates/master/00-master/baremetal/files/baremetal-haproxy.yaml)
of `haproxy` are [configured](https://github.com/openshift/machine-config-operator/blob/master/templates/master/00-master/baremetal/files/baremetal-haproxy-haproxy.yaml)
to load balance the API traffic across all of the control plane nodes.
The [runtimecfg-haproxy-monitor](https://github.com/openshift/baremetal-runtimecfg/blob/master/pkg/monitor/monitor.go) is used for rendering of the haproxy cfg file.

### Highly available ingress access

The ingress-vip will always reside on a node running an Ingress controller.
This ensures that we provide high availability for ingress by default.
The [configuration](https://github.com/openshift/machine-config-operator/blob/master/templates/worker/00-worker/baremetal/files/baremetal-keepalived-keepalived.yaml)
of this mechanism used to determine which nodes are running an ingress controller
is that `keepalived` will try to reach the local `haproxy` stats port number
using `curl`.

### Internal DNS

Externally resolvable DNS records are required for:

* `api.$cluster_name.$base-domain` -
* `*.apps.$cluster_name.$base_domain` -

These records are used externally and internally for the cluster.

In addition, internally resolvable DNS records are required for:

* `api-int.$cluster_name.$base-domain` -
* `$node_hostname.$cluster_name.$base-domain` -

In cluster networking infrastructure, the goal is is to automate as much of the
DNS requirements internal to the cluster as possible, leaving only a
small amount of public DNS configuration to be implemented by the user
before starting the installation process.

In a non-cloud environment, we do not know the IP addresses of all hosts in
advance.  Those will come from an organization’s DHCP server.  Further, we can
not rely on being able to program an organization’s DNS infrastructure in all
cases.  We address these challenges in the following way:

1. Self host some DNS infrastructure to provide DNS resolution for records only
   needed internal to the cluster. In case a request can't be resolved it should be forwarded 
   to the upstream DNS servers. A CoreDNS instance (detailed described below) is used to provide this capability.    
2. Make use of mDNS (Multicast DNS) to dynamically discover the addresses of
   hosts that we must resolve records for.
3. Update `/etc/resolv.conf` in [control plane and compute nodes](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/NetworkManager-resolv-prepender.yaml) to forward requests to the self hosted DNS described in `1` above. 

**NOTE**:
**Docs**: As indicated above Multicast DNS is being used.  The implications of
this are if a customer has a multiple subnet cluster installation
the physical network switches will need to be configured to forward
the multicast packets beyond the subnet boundary.

#### CoreDNS

CoreDNS instance runs as a static pod on [bootstrap](https://github.com/openshift/machine-config-operator/blob/master/manifests/baremetal/coredns.yaml) and
[control plane and compute](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-coredns.yaml)
nodes.
The configuration of CoreDNS for [bootstrap](https://github.com/openshift/machine-config-operator/blob/master/manifests/baremetal/coredns-corefile.tmpl) and [control plane and compute nodes](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-coredns-corefile.yaml) includes the following:

1. Enable `mdns` plugin to perform DNS lookups based on discoverable information from mDNS. the `mdns` plugin is decribed below.
2. `api-int` hostname resolution, the CoreDNS configured during [bootstrap phase](https://github.com/openshift/machine-config-operator/blob/master/manifests/baremetal/coredns-corefile.tmpl#L9) and [after that](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-coredns-corefile.yaml#L13) to resolve the `api-int` hostname to api-vip address.

##### CoreDNS mdns plugin

https://github.com/openshift/coredns-mdns/

The `mdns` plugin for `coredns` was developed to resolve DNS requests based on information received from mDNS.
This plugin will resolve the `$node_hostname` records.
The IP addresses that the `$node_hostname` host records resolve to comes from the
mDNS advertisement sent out by the `mdns-publisher` on that node.

#### mdns-publisher

https://github.com/openshift/mdns-publisher

The `mdns-publisher` [pod](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-mdns-publisher.yaml)
is configured with `hostNetwork: true` providing the IP address
and hostname of the RHCOS instance.

The [baremetal-runtimecfg](https://github.com/openshift/baremetal-runtimecfg)
renders the `mdns-publisher` [configuration](https://github.com/openshift/machine-config-operator/blob/master/templates/master/00-master/baremetal/files/baremetal-mdns-config.yaml).
Replacing `.NonVirtualIP`, `.Cluster.Name` and `.ShortHostname`.

The `mdns-publisher` is the component that runs on each host to make itself
discoverable by other hosts in the cluster.  Both control plane hosts and worker nodes 
advertise `$node_hostname` names.  

`mdns-publisher` does not run on the bootstrap node, as there is no need for any
other host to discover the IP address that the bootstrap instance gets from DHCP.


#### DNS Resolution in control plane and compute nodes

As mentioned above, the node's IP address is [prepend](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/NetworkManager-resolv-prepender.yaml#L27-#L32) to `/etc/resolv.conf`, with this change every DNS request at node level will be forwarded to the local CoreDNS instance.

CoreDNS should resolve the internal records (api-int and cluster node names), as per other requests it should forward them to upstream DNS servers configured in CoreDNS config file. The baremetal-runtimecfg is responsible to [retrieve](https://github.com/openshift/baremetal-runtimecfg/blob/master/pkg/config/node.go#L68-#L97) and render the upstream DNS server list in CoreDNS config file.

### baremetal-runtimecfg

[baremetal-runtimecfg](https://github.com/openshift/baremetal-runtimecfg) is used for rendering the manifests and templates of the machine-config-operator project, it also supports runtime update of the configuration templates (e.g: update HAProxy config incase a new control plane node added to cluster) based on the current system status.

The next capabilities are supported by the `baremetal-runtimecfg`:
- `renders templates` using [values provided at command line](https://github.com/openshift/baremetal-runtimecfg/blob/master/cmd/runtimecfg/runtimecfg.go), parameters retrieved from the cluster (e.g: control plane nodes) and
  - api-vip
  - ingress-vip
- `haproxy monitor`: verify that the API is reachable through haproxy and haproxy config is synced with the cluster.
  It is used from a [side-car](https://github.com/openshift/machine-config-operator/blob/master/templates/master/00-master/baremetal/files/baremetal-haproxy.yaml#L107-#L134) to the haproxy pod.
  - https://github.com/openshift/baremetal-runtimecfg/blob/master/cmd/monitor/monitor.go
  - https://github.com/openshift/baremetal-runtimecfg/blob/master/pkg/monitor/monitor.go
- `keepalived monitor`: monitors that VRRP interface in keepalived config is set to the correct interface. it is also used from a [side-car](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-keepalived.yaml#L89-#L113) to the keepalived pod.
  - https://github.com/openshift/baremetal-runtimecfg/blob/master/cmd/dynkeepalived/dynkeepalived.go
  - https://github.com/openshift/baremetal-runtimecfg/blob/master/pkg/monitor/dynkeepalived.go
- `coredns monitor`: verify that [forward list](https://github.com/openshift/baremetal-runtimecfg/blob/master/pkg/monitor/dynkeepalived.go) in CoreDNS config is synced with `/etc/resolv.conf`, also used from a [side-car](https://github.com/openshift/machine-config-operator/blob/master/templates/common/baremetal/files/baremetal-coredns.yaml#L87-#L113) to coredns pod
  - https://github.com/openshift/baremetal-runtimecfg/blob/master/cmd/corednsmonitor/corednsmonitor.go
  - https://github.com/openshift/baremetal-runtimecfg/blob/master/pkg/monitor/corednsmonitor.go


### Implementation Details/Notes/Constraints

This has already been implemented for baremetal, Ovirt, vSphere and OpenStack.

### Risks and Mitigations

- This network service design has not been verified to be resilient or performant.
- mDNS could have potential security implications

## Design Details

### Test Plan

Testing has already been implemented for baremetal, OpenStack and Ovirt.

- https://github.com/openshift/release/blob/master/ci-operator/templates/openshift/installer/cluster-launch-installer-metal-e2e.yaml
- https://github.com/openshift/release/blob/master/ci-operator/templates/openshift/installer/cluster-launch-installer-openstack-e2e.yaml
- https://github.com/openshift/release/blob/master/ci-operator/templates/openshift/installer/cluster-launch-installer-ovirt-e2e.yaml

With the addition of vSphere IPI the testing implementation will depend on the location
for the testing infrastructure.  If VMware Cloud on AWS is used additional AWS Route53 and ELBs
will be needed for internet-facing access to the API.  If Packet is to be used
only Route53 will be needed to access the API.  The other potential issue
will be determining the IP addresses for the VIPs but reusing the existing
IPAM server might be an option.

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

- Bare Metal - 7/2019
- OpenStack - 7/2019
- Ovirt - 10/2019

## Drawbacks

- Currently only provides a single default VIP for Ingress
- Bootstrap will maintain its role as keepalived master until some intervention which in our case is destroying the bootstrap node.

## Alternatives

Unknown


The basis and significant portions for this proposal was taken from existing
[documentation](https://github.com/openshift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md).
