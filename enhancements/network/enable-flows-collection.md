---
title: neat-enhancement-idea
authors:
  - "@mariomac"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-07-29
last-updated: 2021-07-29
status: implementable
see-also:
  - "/enhancements/network/netflow.md"
replaces:
superseded-by:
---

# Enable flows collection

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This document defines the steps required to facilitate the enablement and collection
of network flows' tracing from the user side, as well as the status retrieval.
It describes the user stories to switch on/off the flows both from the command-line
interface and the web console, and depicts the implementation details required in
the existing OVN-Kubernetes component, as well as the new components that should
be created. We consider IPFix as the unique protocol to be supported by now.

## Motivation

Flows (IPFIX) emission, collection and processing may have an overall
impact in the system performance. By this reason, we need to allow users to
switch on and off the flows' tracing feature.

It is also important to allow the users to know the status of the current flows
tracing (on or off).

### Goals

Both via Command-Line Interface (CLI) and Graphical User Interface (UI):

* To provide the users with a simple mechanism to enable and disable flows emission and
  collection.
* To provide the users with a simple mechanism to retrieve the current status of the flows'
  emission.
* To provide the users with a mean to retrieve and set the flows' cache setting,
  which can play an important role to mitigate potential issues with a too high ingestion rate.

### Non-Goals

To discuss the architecture of the flow collection and processing pipeline. This
has been already discussed and defined by the Network Observability Team
[by another channel](https://docs.google.com/document/d/1kzNfTLXhMolu8VTcH0m9hmvFluN9yKMy7TSNIXJVTrA/edit?usp=sharing).

This document does not define the work required to configure and deploy the
flow collection and storage pipeline.

## Proposal

### User Stories

#### CLI-based flow enablement and status retrieval

(The following verbs and commands are not definitive but just illustrative examples).

To enable IPFix emission:

```text
$ oc flows start
```

To enable IPFIX emission:

```text
$ oc flows start
```

To disable any flow emission:

```text
$ oc flows stop
```

For simplicity purposes, we only allow emitting IPFix samples, but we might support other protocols
(e.g. Netflow) in the future.

To retrieve the status of the flows' emission, it could be shown by node:

```text
$ oc get flows
NAME    STATUS      TYPE     ENDPOINT
node-a  emitting    ipfix    http://10.1.2.3:30001
node-b  emitting    ipfix    http://10.1.2.4:30001
node-c  emitting    ipfix    http://10.1.2.5:30001
```

The command should return error if you try to enable flows in any cluster operator
different from `OVNKubernetes`.

```text
$ oc flows start
OpenShiftSDN does not support flows tracing.
```

#### Console-based flow status and retrieval

If the cluster uses `OVNKubernetes` CNI type, in the side panel of the Openshift
Console, an "Observability" or "Monitoring" entry would appear inside the
"Networking" group.

This would lead to a page where you can enable/disable the Network flows, and
see their status. When the flows' emission is disabled, it would just show an
"Enable flows" dropdown list, where you can select one of the following
entries:

```text
                 +-----------+-+                            
 Flows emission: |Disabled   |V|                            
                 |           +-+                            
                 |IPFix        |                            
                 +-------------+                            
```

Once the emission has been enabled, the GUI would show a table with the
status for each node, analogue to the CLI use case:

```text
                       +-----------+-+
       Flows emission: |IPFix      |V|
                       +-----------+-+

                                
    +------------------------------------------------+
    |  NAME       STATUS      ENDPOINT               |
    +------------------------------------------------+
    |  node-a     emitting    http://10.1.2.3:30001  |
    |  node-b     emitting    http://10.4.5.6:30001  | 
    |  node-c     emitting    http://10.7.8.9:30001  |
    +------------------------------------------------+
```

#### Tuning flows' emission parameters

Some setups might lead to an overcongestion of the cluster resources such as the network or the
flows' ingestion/processing/storage pipeline (e.g. because the size and frequency of reported
flows is too high).

To minimize it, the user should be able to tune up the flows' cache mechanism in order to control
the number of records sent to collector.

According to the [OpenVSwitch configuration options](https://www.openvswitch.org/support/dist-docs/ovs-vswitchd.conf.db.5.html),
there are multiple configuration options that could be exposed to the user:

* `cache_active_timeout`: optional integer, in range 0 to 4,200
  The maximum period in seconds for which an IPFIX flow record  is
  cached  and  aggregated  before  being  sent.  If not specified,
  defaults to 0. If 0, caching is disabled.

* `cache_max_flows`: optional integer, in range 0 to 4,294,967,295
  The maximum number of IPFIX flow records that can be cached at a
  time.  If  not  specified,  defaults to 0. If 0, caching is disabled.

* `sampling`: optional integer, in range 1 to 4,294,967,295
  The  rate  at  which  packets should be sampled and sent to each
  target collector. If not specified, defaults to 400, which means
  one  out of 400 packets, on average, will be sent to each target
  collector.

Other arguments might be considered, such as `enable-input-sampling`, `enable-output-sampling`,
or `enable-tunnel-sampling`.

The configuration should be also exposed in the Console UI, along with the current
emission status (see previous subsection).

### Implementation Details/Notes/Constraints [optional]

In order to ensure an optimal usage of the network, we should restrict flow's
emission-collection traffic to the local node.

According to the [architectural description of the flows' emitter/collectors pipeline](
https://docs.google.com/document/d/1kzNfTLXhMolu8VTcH0m9hmvFluN9yKMy7TSNIXJVTrA/edit#heading=h.kulf936ct32p),
the collector will be deployed as a DaemonSet, to ensure that there will be an
instance of the collector for each node.

Exposing the collector endpoint as a Kubernetes ClusterIP Service would cause the traffic
to be distributed across all the nodes (unless you enable the `ServiceInternalTrafficPolicy`
cluster-wide feature gate, which at this moment is in alpha status, only available
for the latest Kubernetes 1.21 version).

To limit the flows' traffic to internal nodes, each collector should expose a node
port, so each OVN instance only has to know the node IP and Port (which might be
fixed or discovered).

From the user-side configuration of the IPFix cache and sampling, we should use a `ConfigMap` and
expose it to OVN-Kubernetes with read permissions.

Given the aforementioned details, following subsections depicts the work to do,
grouped in different tasks.

#### Task 1: modify OVN-Kubernetes configuration to discover automatically the collector address

[Current `Network` operator configuration allows setting a static host:port address](
https://docs.okd.io/latest/networking/ovn_kubernetes_network_provider/tracking-network-flows.html#nw-network-flows-object_tracking-network-flows).
This information is forwarded to each OVN instance, on each cluster node. For example:

```yaml
apiVersion: operator.openshift.io/v1
kind: Network
metadata:
  name: cluster
spec:
  exportNetworkFlows:
    collectors:
      - 192.168.1.99:2056
```

However, we need to set this address dynamically to ensure that each pod will forward
the flows to the collector in its same node.

We need to extend the previous configuration to allow replacing the `collectors` property
by one of the following properties:

* `nodePort: (int)`: if set, the OVN will discover the node IP and submit the flows to the
  port indicated by such property. E.g. `nodePort: 30023`.
* `selector: (map)`: if set, the OVN will search for any `Service` with `type: NodePort`
  matching the labels passed as argument, then extract the service's `NodePort` and use
  it alongside with the node IP. Configuration example:
  ```yaml
  selector:
    role: flows-collector
  ```

#### Task 2: implement new discovery options in OVN-Kubernetes

Before [OVN-Kubernetes invokes OpenVSwitch to enable flows](https://github.com/ovn-org/ovn-kubernetes/commit/ecc4047f5d0e732c267fccb2eabd64b5894b9f9a#diff-9c3b7372332fd16d60dba539a94dd0f819d262d1fbabd1e5d9c09e47da5af0ffR119-R128),
if the network provider is configured for discovery, it needs to fetch the host/port and pass it
to OpenVSwitch, in the `targets=` argument.

If, by any reason, the Collector host:port can't be discovered (e.g. because the collector is not
properly deployed). The error status should be visible for the customer (e.g. in the `oc get flows`
table).

The simplest way to expose this error status would be exposing it as a metadata annotation, while
the Pod is kept in the `Running` status (to avoid a wrong observability configuration to interrupt
the normal network operation).

#### Task 3: tune up flows enablement in OVN-Kubernetes

If OVN-Kubernetes has access to the [performance-tuning configmap](#tuning-flows-emission-parameters),
it should pass the contained arguments [when it enables the flows](https://github.com/ovn-org/ovn-kubernetes/commit/ecc4047f5d0e732c267fccb2eabd64b5894b9f9a#diff-9c3b7372332fd16d60dba539a94dd0f819d262d1fbabd1e5d9c09e47da5af0ffR167).

#### Task 4: create a Flow operator to manage the flows status

After this task is finished:
* The operator can communicate with OVN-Kubernetes pods to patch their configuration.
* There is an exposed API to perform the various operations: enable, disable, get status...
* You can operate flows through the `oc` CLI command: enable, disable, get status...
* The operator should be able to detect changes in the [performance-tuning configmap](#tuning-flows-emission-parameters)
  and restart the OVN-Kubernetes emission with the new arguments.
This task could involve setting new RBAC permissions to allow interaction between the
Flow operator and the OVN-Kubernetes pods.

#### Task 5: enable the Flows' operation in the Console GUI

As described in the above [console-based flow status and retrieval](#console-based-flow-status-and-retrieval)
section. It should also visualize the [configuration options for tuning IPFix performance](#tuning-flows-emission-parameters).

The features should be implemented within a Console plugin.

### Risks and Mitigations

To be discussed.

## Design Details

### Open Questions [optional]

[User stories](#user-stories) need to be reviewed by the UX team.

### Test Plan

* Unit tests for all the components and GUI.
* Integration tests for all the interactions between different components:
  - Flows' operator <-> OVN-Kubernetes
  - UI <-> Flows' operator
* End-to-end tests:
  * deploying a cluster with IPFix collection/processing
  * enabling IPFix and verify that it is collected
  * disabling flows and verify that it stopped being collected
* Performance tests
  * We need to evaluate the impact of enabling flow tracing in a cluster's
    resources. Mainly Network, CPU and Memory.
  * We need to evaluate the effect of the different sampling and cache configuration options
    in the overall performance impact.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

The customer would need to upgrade the OVN-Kubernetes operator, in order to support
the collector host:port discovery.

Also, they would need to Install the flows operator and the console plugin. For example,
from the OperatorHub or a Helm chart.

### Version Skew Strategy

If the OVN-Kubernetes updated operator is released with Openshift 4.10, we would need
to set this Openshift version as a requirement to install the operator.

We don't envision other issues with respect to version skew.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

To be discussed.

## Alternatives

Currently, OVN-Kubernetes can be configured to export flows to any third-party
application. Customer could just manually install FluentD or GoFlow and a
storage/UI component like ElasticSearch+Kibana or Loki+Grafana.

## Infrastructure Needed [optional]

Code repositories for:
* Console plugin
* Flows operator

Testing infrastructure:
* Probably we'd need and end-to-end environment with all the components installed,
  allowing setting up flows in order to evaluate both the performance and the
  correct behavior.
