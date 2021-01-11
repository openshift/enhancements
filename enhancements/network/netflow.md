---
title: OpenShift Network Flows export support
authors:
  - "@rcarrillocruz"
reviewers:
  - "@russellb"
  - "@danwinship"
approvers:
  - TBD
creation-date: 2021-01-11
last-updated: 2020-01-11
status: provisional
---

# OpenShift Network Flows export support

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Collecting all traffic from a cluster at a packet level would be an enormous amount of data.
An alternative approach for analysis of network traffic is to capture only metadata about the traffic in a cluster.
This metadata would include information like the protocols, source and destination addresses, port numbers, and the amount of traffic.
Collecting this metadata becomes a more manageable approach to getting visibility into network activity across the cluster.

There are existing protocols for how this metadata is transmitted between systems, including NetFlow, IPFIX, and sFlow.
Open vSwitch (OVS) supports these protocols and OVS is a fundamental building block of the OVN-Kubernetes network type.

This proposal discusses how we start providing this functionality by allowing cluster administrators to enable the export of network flow metadata using one of these protocols.

## Motivation

* Cluster administrators need to be able to get network flows out of OpenShift to be consumed by their collectors.

### Goals

- Add support for exporting the network flows traffic in an OpenShift cluster by leveraging the NetFlow/sflow/IPFIX protocols supported by OVS.

### Non-Goals

- This document only applies to OVN-Kubernetes, not OpenShift SDN or any other other network plugin.
- This document will not discuss the flow logs store solutions. Typically, customers store the NetFlow data in a data warehouse system (e.g. ClickHouse) or search and analytics system (e.g. ElasticSearch).
  It is out of the scope of this document to discuss which option should be used and if we should manage that solution.
- This document will not discuss the aggregation of flow data with corresponding K8S pods. This will be discussed in other proposal and analyze various options
  we can leverage like bundle a collector that aggregate K8S data by querying API on the fly or SkyDive.
- This document will not discuss flow visualization or flow dashboards.

## Proposal

OVS supports NetFlow v5, IPFIX and sflow as export flows protocol. It also supports a list of collectors by specifying IP and port to send the flow data.
As such, we would support the three protocols and an arbitrary list of collectors.

The Cluster Network Operator (CNO) would expose:
* `exportNetworkFlows` optional dict to contain the options to export the network flows
  * `netflow` optional dict to specify that netflow protocol will be used.
    * list of strings specifying the IP and port (separated by colon)  of the collectors that will consume the flow data
  * `sflow` optional dict to specify that sflow protocol will be used.
    * list of strings specifying the IP and port (separated by colon)  of the collectors that will consume the flow data
  * `ipfix` optional dict to specify that ipfix protocol will be used.
    * list of strings specifying the IP and port (separated by colon)  of the collectors that will consume the flow data

This is an example of a *networks.operator.openshift.io* spec exporting flows on all protocols to two collectors on different ports:

```yaml
spec:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  defaultNetwork:
    ovnKubernetesConfig:
      genevePort: 6081
      mtu: 1400
    type: OVNKubernetes
  deployKubeProxy: false
  disableMultiNetwork: false
  disableNetworkDiagnostics: false
  logLevel: Normal
  managementState: Managed
  observedConfig: null
  operatorLogLevel: Normal
  serviceNetwork:
  - 172.30.0.0/16
  exportNetworkFlows:
    netflow:
    - 172.30.158.150:2056
    - 172.30.54.103:2056
    sflow:
    - 172.30.158.150:6343
    - 172.30.54.103:6343
    ipfix:
    - 172.30.158.150:2055
    - 172.30.54.103:2055
```

Under the covers, these options will make the CNO perform an ovs-vsctl command on the bridge br-int, which is the bridge the containers are connected to on OVN-Kubernetes, and its management port ovn-k8s-mp0.
As an example, if export flows were enabled per the above example this is the command that would be executed.

```bash
ovs-vsctl -- --id=@netflow create netflow targets=\[\"172.30.158.150:2056\",\"172.30.54.103:2056\"\] -- set bridge br-int netflow=@netflow
ovs-vsctl -- --id=@sflow create sflow agent=ovn-k8s-mp0 targets=\[\"172.30.158.150:6343\",\"172.30.54.103:6343\"\] header=128 sampling=64 polling=10 -- set bridge br-int sflow=@sflow
ovs-vsctl -- --id=@ipfix create ipfix targets=\[\"172.30.158.150:2055\",\"172.30.54.103:2055\"\] obs_domain_id=123 obs_point=456 sampling=1 -- set bridge br-int ipfix=@ipfix
```

### Test Plan

- Unit tests for the feature
- e2e tests covering the feature
- For generating flow data [GoFlow](https://github.com/cloudflare/goflow) would be used, as it supports all protocols and has a hub Docker image ready to use

### Graduation Criteria

From Tech Preview to GA

#### Tech Preview -> GA

- Ensure OpenShift can export network flows data off OVS to an endpoint

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History
