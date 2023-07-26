---
title: ovn-kubernetes-ns-ipsec
authors:
  - "@yuvalk"
reviewers:
  - TBD
  - "@dcbw"
  - "@trozet"
approvers:
  - TBD
  - "@dcbw"
  - "@trozet"
creation-date: 2023-07-26
last-updated: 2023-07-26
status: implemented
see-also:
  - "/enhancements/network/ovn-kubernetes-ipsec.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# OVN Kubernetes North-South IPsec

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The OVN Kubernetes network plugin uses [OVN](https://www.ovn.org) to instantiate
virtual networks for Kubernetes. These virtual networks use Geneve, a network
virtualization overlay encapsulation protocol, to tunnel traffic across the
underlay network between Kubernetes Nodes.

IPsec is a protocol suite that enables secure network communications between
IP endpoints by providing the following services:
- Confidentiality
- Authentication
- Data Integrity

OVN tunnel traffic is transported by physical routers and switches. These
physical devices could be untrusted (devices in public network) or might be
compromised. Enabling IPsec encryption for this tunnel traffic can prevent the
traffic data from being monitored and manipulated.

The scope of this work is to enable customers to encrypt traffic going in/out of a node. It will not encrypt traffic between cluster nodes (for that we have ovn-kubernetes-ipsec.md). However due the fact that both will use the same mechanims, n-s implementation will also touch the existing e-w.

## Motivation

Encryption services are recommended when traffic is traversing an untrusted
network. Encryption services may also be required for regulatory or compliance
reasons (e.g. FIPS compliance, FSI regulations, and more).

Use-Cases to consider:
a. Encapsulate all traffic - when cluster is placed remotely and need to communicate with services in a central DC (cloud or other), and last mile of communication is not controlled by owner (ie - public internet), we'd want to encapsulate all traffic in IPsec tunnel (as VPN). this includes even ssh traffic etc. cluster can be placed behind NAT and we might even want to encapsulate IPv6 inside IPv4 tunnel. this use case need to be available independently of kubelet.

b. CSI encryption - some regulation demand encryption in-transit and at-rest for certain types of data. IPsec tunnel can be used to encrypt data in-transit for cases where we cant use anything else (aka NetApp ONTAP). this use case user a policy to encrypt only toward certain endpoints.


### Goals

- Provide per host north-south IPsec encryption that encapsulate all traffic.
- Provide per host north-south IPsec encryption with parties external to the cluster.
- Said IPsec should be "stand alone" and start before kubelet

### Non-Goals

- RHEL nodes, support will be for RHCOS only see section below on RHEL nodes
- Shared Gateway Mode (SGW), only local GW mode will be supported
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
since the ipsec config file is now on the host, it doesnt really matter who start pluto as long as it's running. (ie - if customer configured NM-libreswan, and that kicks up way before ovn-ipsec container is started..)

3. Upgrade. special care needed for upgrade, as CNO upgrade before the node, which means we wont have the os extension available. to avoid haing to deal with it in 4.14 (see 4.15) - 
a. we'll keep 2 sets of daemonsets `ipsec` and `ipsec-pre414` each daemonset containers (init, ipsec, and liveness) will check whether libreswan is installed on the host and act accordingly so that only one set is actually handling ipsec at a time
b. this gives the added value of being able to mix nodes with n-s and without. still IMHO something undesireable in the future. we want to support a single implementation (ie - host based)

### 4.15

From 4.15 we'll add IPsec support to nmstate which will then become the only supported method to configure IPsec (either via nmstate on the host or through k8s-nmstate operator)

We also want to properly solve the upgrade issues in a manner that will allow us to keep a single ds ipsec implementation in the future. once we solve that, we should also apply the os-extension from CNO.

and lastly we want to add some telemetry around ipsec so we can gather usage statistics.
  - east-west: on/off
  - north-south: on/off

### Implementation Details/Notes/Constraints

### Risks and Mitigations

#### Upstreaming
this enhancement deals with OCP enablemnet only

#### Performance
IPsec without HW offloading is indeed cpu intensive. also, due to other existing kernel limitations we dont expect, even on modern CPUs to be able to do more than 6-7Gbps. Once offloading will be properly solved in RHEL we can revisit on how to enable for N-S IPsec, but at least, nothing we do blocks that option.

From scale perspective, this is a per-node feature.

#### Security Review
TBD

## Design Details
the exact fields to be supported in nmstate will be determined elsewhere

### Open Questions
how to properly solve upgrade issue

### Test Plan

https://docs.google.com/document/d/1LV87IrjTbG_0MOxfS5w4_T2pWQDMCQYookwjcVo86DU/edit#heading=h.ef9ztuvu6wrq

one note though is that to test n-s, test lane will need to include an external endpoint
and a ca to generate certs

### Graduation Criteria

Graduation criteria follows:

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Performance measurement

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

since CNO always upgrade first, only then the os, the idea is to enable the os extension during upgrade. then deploy the new daemonset.

### Version Skew Strategy

### RHEL Nodes appendix
Though RHEL nodes are not fully supported, customer using such nodes can manually install libreswan and create ipsec config.
While not tested, I believe this should work.
actually I think the E-W impl move to the host is covering RHEL nodes too (if libreswan will be installed on the node)

## Implementation History

## Drawbacks

N/A

## Alternatives

N/A

## Infrastructure Needed

N/A
