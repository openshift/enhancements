---
title: ovn-kubernetes-ns-ipsec
authors:
  - "@yuvalk"
reviewers:
  - "@pperiyasamy" ## for 4.15 and upgrade parts
  - "@dcbw" ## for E-W interaction parts
  - "@trozet" ## overall
approvers:
  - "@trozet"
api-approvers:
  - "@JoelSpeed"
  - "@deads2k"
creation-date: 2023-07-26
last-updated: 2024-05-15
tracking-link:
  - https://issues.redhat.com/browse/SDN-4034
see-also:
  - "/enhancements/network/ovn-kubernetes-ipsec.md"
replaces:
  - NA
superseded-by:
  - NA
---


# OVN Kubernetes North-South IPsec

## Summary

The OVN Kubernetes network plugin uses [OVN](https://www.ovn.org) to instantiate
virtual networks for Kubernetes. These virtual networks use Geneve, a network
virtualization overlay encapsulation protocol, to tunnel traffic across the
underlay network between Kubernetes Nodes.

IPsec is a protocol suite that enables secure network communications between
IP endpoints by providing the following services
- Confidentiality
- Authentication
- Data Integrity

OVN tunnel traffic is transported by physical routers and switches. These
physical devices could be untrusted (devices in public network) or might be
compromised. Enabling IPsec encryption for this tunnel traffic can prevent the
traffic data from being monitored and manipulated.

The scope of this work is to enable customers to encrypt traffic going in/out of a node. It will not encrypt traffic between cluster nodes (for that we have ovn-kubernetes-ipsec.md). However due to the fact that both will use the same mechanims, n-s (north-south) implementation will also touch the existing e-w (east-west).

as part of this enhancement, we will also upgrade the CNO IPsec configuration API to a more discoverable configuration that supports both n-s and e-w.

## Motivation

Encryption services are recommended when traffic is traversing an untrusted
network. Encryption services may also be required for regulatory or compliance
reasons (e.g. FIPS compliance, FSI regulations, and more).

Use-Cases to consider:
a. Encapsulate all traffic - when cluster is placed remotely and need to communicate with services in a central DC (cloud or other), and last mile of communication is not controlled by owner (ie - public internet), we'd want to encapsulate all traffic in IPsec tunnel (as VPN). This includes even ssh traffic etc. clusters can be placed behind NAT and we might even want to encapsulate IPv6 inside the IPv4 tunnel. This use case needs to be available independently of kubelet.

b. CSI encryption - some regulation demand encryption in-transit and at-rest for certain types of data. IPsec tunnel can be used to encrypt data in-transit for cases where we cant use anything else (aka NetApp ONTAP). this use case user a policy to encrypt only toward certain endpoints.

### User Stories

1. as a user I want to define a host-2-net, catch all, IPsec tunnel between cluster nodes and an external VPN GW. should apply to all BM configurations (SNO, SNO+1, MNO)
2. as a user I want to define a host-2-host, IPsec transpot, to be able to encrypt data. for example from OCP Node to NetApp dyptshr device.
3. as a user I want to define a net-2-net, IPsec tunnel, between a OCP node and an external entity, possible another OCP cluster on a different network.

all stories should also cover NAT

### Goals

- Provide per host north-south IPsec encryption that encapsulate all traffic.
- Provide per host north-south IPsec encryption with parties external to the cluster.
- Said IPsec should be "stand alone" and start before kubelet

### Non-Goals

- RHEL nodes, support will be for RHCOS only see section below on RHEL nodes
- Will not provide key mgmt. ie - customer will need to manage keys on their own
- Will not support policy to encrypt traffic between cluster nodes

## Proposal

Generally the propsal is to make libreswan available on the host and have the customer manage it's configuration via nmstate and k8s-nmstate. 
For 4.14 TP, we'll simply use MCs with ipsec.config files.

Becuase we can only have one pluto daemon on the node, E-W will have to move to use the host part too

### 4.14

this have the following parts:
1. add libreswan and NM-libreswan to RHCOS as a new `ipsec` extension. We're adding both so that customer who need something that is not supported by the plugin can still achieve that by interacting directly with the lower libreswan layer.
this alone is enough to enable n-s IPsec. customer can use MC to deploy certs and config files.

2. But this is not enough. since there can be only one pluto daemon that listens on udp 500/4500, we also need to move E-W to the host. that is done by a series of changes to the ovn-ipsec daemonset:
a. use host `/etc/ipsec.conf.d/openshift.conf` instead of the container `/etc/ipsec.conf`
b. use hostPID so that pluto is actually "on the host". for that also mount relevant `/var/run` files.
c. move the SIGTERM trap to a pod lifecycle preStop, because kubelet does not send SIGTERM when hostPID=true
since the IPsec config file is now on the host, it doesnt really matter who start pluto as long as it's running. (ie - if customer configured NM-libreswan, and that kicks up way before ovn-ipsec container is started..)

3. Upgrade. special care needed for upgrade, as CNO upgrade before the node, which means we wont have the os extension available. to avoid having to deal with it in 4.14 (see 4.15) - 
a. we'll keep 2 sets of daemonsets `ipsec` and `ipsec-pre414` each daemonset containers (init, ipsec, and liveness) will check whether libreswan is installed on the host and act accordingly so that only one set is actually handling IPsec at a time
b. this gives the added value of being able to mix nodes with n-s and without. still IMHO something undesireable in the future. we want to support a single implementation (ie - host based)

In 4.14, the enablement of the os extension is done manually by customer using a MachineConfig.

### 4.15

From 4.15, the os-extension and IPsec service will be configured through a new network.operator.openshift.io API that will allow to enable/disable IPsec and configure the e-w behaviour. the new API include the following options:
Disabled = no ipsec. and is the default
External = CNO will apply the os-extension to get libreswan on the nodes, enabling N-S ipsec
Full = on top of external, CNO will also self-manage the E-W IPsec. E-W always imply also the possibility of user configurations for N-S. hence it is called `Full`.

North-South configuration will be supported through nmstate (either via nmstate on the host or through k8s-nmstate operator).

Upgrade will implement a state machine 

For north-south only (`ipsecMode: External`) case, cno just deploys IPsec MC extension and no IPsec daemonset get rolled out.
When both east-west and north-south enabled (`ipsecMode: Full`), then cno deploys IPsec MC extension and deploys host based
IPsec daemonset.
For the east-west configuration, IPsec MC extension with host networked pod is used by default, except for HyperShift deployments where container based IPsec is used.
For upgrade 4.14 -> 4.15 and IPsec for NS is already enabled, then CNO retains IPsec config for north-south traffic though IPSecExternal parameter not set.

and lastly we want to add some telemetry around IPsec so we can gather usage statistics. this will follow closely with the new API, but will take care of legacy modes too.

### Workflow Description

#### Enable IPsec
1. cluster admin enable N-S IPsec by modifying OVN config. can choose from `Full` or `External` (while default is `off`)
2. CNO will enable the ipsec os-extension and the ipsec.service on all the nodes.
3. If mode `Full`, CNO will also start the ipsec daemonset which will define ipsec policies for E-W traffic, see https://github.com/openshift/enhancements/blob/master/enhancements/network/ovn-kubernetes-ipsec.md for details on that.
4. from that point, cluster admin can define additional policies with k8s-nmstate operator or directly via MCs. certificate management is currently out-of-scope, but is possible via MCs too.

#### Upgrade
see 4.15 section above

### API Extensions

networks.operator.openshift.io ipsecConfig will be expanded to include IPsecMode string with 3 possible values (`Disabled`, `External`, `Full`)

to allow backwards compatibility we'll use some CIL "magic" so that if IPsecConfig exist with no mode value at all, it'll be treated as "Disabled" and on any new cluster, we'll explicitly set it to `Disabled`

### Topology Considerations

#### Hypershift / Hosted Control Planes

N-S IPsec is currently not supported for these topologies

#### Standalone Clusters

N-S IPsec will be fully supported on standalone clusters. no changes beyond what is already described is needed.

#### Single-node Deployments or MicroShift

SNOs will need to allocate additional full core for IPsec processing.

Microshift topology is not currently supported

### Drawbacks

- extra complication of an already complicated state-machine in CNO. especially around upgrades.
- sharing pluto daemon on the host, means user configurations might conflict with ovs and impact cluster availability
- We are not solving certificate management for users, which is extra complication for them

## Test Plan

Were adding more ipsec ci lanes, including upgrade.
All the existing CI lanes will be test in `Full` mode


## Graduation Criteria

### Dev Preview -> Tech Preview

- ability to enable ipsec on OCP Node (RHCOS) via os extension. this is N-S only, without E-W

### Tech Preview -> GA

- Ability to utilize the enhancement end to end
- End user documentation
- API stability
- ipsec mode included in telemetry data

### Removing a deprecated feature

- ipsec must be defined with a mode. old style, empty, definition `ipsecConfig: {}` will be reject by API server.

## Upgrade / Downgrade Strategy

See Prposal section with details

## Version Skew Strategy

See Proposal section with details. 
CNO is always upgraded first, and we will enhance it's upgrade logic to wait till the os layer is available before rolling out ipsec configuration on the host. while that is on-going, e-w ipsec, if enabled, will keep running in a container daemonset.

## Operational Aspects of API Extensions

none.

## Support Procedures

for E-W the main diff is that pluto logs are now on the host. and is still guarded with CNO (so `Full` mode and failing ipsec should result in degraded CNO)
for N-S (`External` mode) the main troubleshooting point is those said pluto logs.

## Alternatives

- Create a full CRD API for N-S IPsec tunnels within CNO. this would have generate much more work for the SDN team to support something that is unrelated to the SDN. probably first releases were limited because covering everything libreswan offers is difficult and takes time. while with the proposed approach, customers can enjoy anything libreswan have to offer.

### Implementation Details/Notes/Constraints

### Risks and Mitigations

#### Upstreaming
This enhancement deals with OCP enablemnet only

#### Performance
IPsec without HW offloading is indeed cpu intensive. also, due to other existing kernel limitations we dont expect, even on modern CPUs to be able to do more than 6-7Gbps. Once offloading will be properly solved in RHEL we can revisit on how to enable for N-S IPsec, but at least, nothing we do blocks that option.

From scale perspective, this is a per-node feature.

## Design Details
the exact fields to be supported in nmstate will be determined elsewhere

### Open Questions
none

### Test Plan

https://docs.google.com/document/d/1LV87IrjTbG_0MOxfS5w4_T2pWQDMCQYookwjcVo86DU/edit#heading=h.ef9ztuvu6wrq

one note though is that to test n-s, test lane will need to include an external endpoint
1. add libreswan and NM-libreswan to RHCOS as a new `ipsec` extension. We're adding both so that customer who need something that is not supported by the plugin can still achieve that by interacting directly with the lower libreswan layer.
this alone is enough to enable n-s IPsec. customer can use MC to deploy certs and config files.
