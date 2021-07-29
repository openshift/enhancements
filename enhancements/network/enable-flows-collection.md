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
of network Flows' tracing from the user side, as well as the status retrieval.
It describes the user stories to switch on/off the flows both from the command-line
interface and the web console, and depicts the implementation details required in
the existing OVN-Kubernetes component, as well as the new components that should
be created.

## Motivation

Flows (NetFlow, IPFIX) emission, collection and processing may have an overall
impact in the system performance. By this reason, we need to allow users to
switch on and off the flows' tracing feature.

It is also important to allow the users to know the status of the current flows
tracing (on or off).

### Goals

Both via Command-Line Interface (CLI) and Graphical User Interface (UI):

* To provide to the users a simple mechanism to enable and disable flows emission and 
  collection.
* To provide to the users a simple mechanism to retrieve the current status of the flows'
  emission.

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

To enable Netflow emission:

```
$ oc flows start netflow
```

To enable IPFIX emission:

```
$ oc flows start ipfix
```

To disable any flow emission:

```
$ oc flows stop
```

For simplicity purposes, we do not allow emitting both netflow and IPFIX at the
same time.

To retrieve the status of the flows' emission, it could be shown by node:

```
$ oc get flows
NAME    STATUS      TYPE     ENDPOINT
node-a  emitting    netflow  http://10.1.2.3:30001
node-b  emitting    netflow  http://10.1.2.4:30001
node-c  emitting    netflow  http://10.1.2.5:30001
```

The command should return error if you try to enable flows in any cluster operator
different from `OVNKubernetes`.

```
$ oc flows start netflow
OpenShiftSDN does not support flows tracing.
```

### Console-based flow status and retrieval

If the cluster uses `OVNKubernetes` CNI type, in the side panel of the Openshift 
Console, an "Observability" or "Monitoring" entry would appear inside the
"Networking" group.

This would lead to a page where you can enable/disable the Network flows, and
see their status. When the flows' emission is disabled, it would just show an
"Enable flows" dropdown list, where you can select one of the following
entries:

```
                 +-----------+-+                            
 Flows emission: |Disabled   |V|                            
                 |NetFlow    +-+                            
                 |IPfix        |                            
                 +-------------+                            
```

Once the emission has been enabled, the GUI would show a table with the
status for each node, analogue to the CLI use case:

```
                       +-----------+-+
       Flows emission: |Netflow    |V|
                       +-----------+-+

                                
    +------------------------------------------------+
    |  NAME       STATUS      ENDPOINT               |
    +------------------------------------------------+
    |  node-a     emitting    http://10.1.2.3:30001  |
    |  node-b     emitting    http://10.4.5.6:30001  | 
    |  node-c     emitting    http://10.7.8.9:30001  |
    +------------------------------------------------+
```

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

Given the aforementioned details, following subsections depicts the work to do,
grouped in different tasks.

#### Task 1: modify OVN-Kubernetes configuration to discover automatically the collector address.

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
    netFlow:
      collectors:
        - 192.168.1.99:2056
```

However, we need to set this address dynamically to ensure that each pod will forward
the flows to the collector in its same node.

We need to extend the previous configuration to allow replacing the `collectors` property
by one of the following properties:

* `nodePort: (int)`: if set, the OVN will discover the node IP and submit the flows to the
  port indicated by such property. E.g. `nodePort: 30023`.
* `selector: (map)`: if set, the OVN will fetch for any `NodePort` matching
  the labels passed as argument, and use the discovered host:ip that matches the node and
  labels. For example:
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

#### Task 3: create a Flow operator to manage the flows status

After this task is finished:
* The operator can communicate with OVN-Kubernetes pods to patch their configuration.
* There is an exposed API to perform the various operations: enable, disable, get status...
* You can operate flows through the `oc` CLI command: enable, disable, get status...

This task could involve setting new RBAC permissions to allow interaction between the
Flow operator and the OVF-Kubernetes pods.

#### Task 4: enable the Flows' operation in the Console GUI

As described in the above [console-based flow status and retrieval](#console-based-flow-status-and-retrieval)
section.

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
  * deploying a cluster with netflow collection/processing
  * enabling netflow/ipfix and verify that it is collected
  * disabling flows and verify that it stopped being collected
* Performance tests
  * We need to evaluate the impact of enabling flow tracing in a cluster's
    resources. Mainly Network, CPU and Memory.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

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
