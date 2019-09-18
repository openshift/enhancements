---
title: OpenShift Windows Worker Node Monitoring
authors:
  - "@ravisantoshgudimetla"
  - "@aravindhp"
reviewers:
  - "@crawford"
  - "@sdodson"
  - "@brancz"
  - "@s-urbaniak"
  - "@lilic"
approvers:
  - "@crawford"
  - "@sdodson"
  - "@brancz"
  - "@s-urbaniak"
  - "@lilic"
creation-date: 2019-09-17
last-updated: 2019-09-18
status: implementable
---

# monitoring-of-windows-worker-nodes

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

The intent of this enhancement is to enable monitoring of Windows worker nodes
in OpenShift cluster.

## Motivation

Monitoring is critical to identify issues with nodes, containers running on the
nodes. The main motivation behind this enhacement is to enable monitoring of
Windows nodes

### Goals

As part of this enhacement, we plan to do the following:
* Deploy Windows Management Instrumentation(WMI exporter) onto Windows nodes
* Configure Prometheus provisioned as part of OpenShift install to 
  collect data from Windows Nodes
* Upgrade WMI exporter on the Windows Nodes
* Leverage cluster-monitoring operator for setting up Prometheus, alert manager
  and other components

### Non-Goals

As part of this enhacement, we do not plan to do the following:
* Create a container image for WMI and let WMI run as a container
* Integrating WMI with cluster monitoring operator
* Ship Grafana dashboards for Windows Nodes

## Proposal

The main idea here is to run WMI exporter as a Windows Service and let 
Prometheus instance which was provisioned as part of OpenShift install to 
collect data from WMI exporter. Since WMI exporter cannot run as a container
on the Windows node, there by making it impossible to run it within OpenShift cluster, 
we need to ensure that Prometheus talks to WMI exporter as an external service.


### Implementation Details/Notes/Constraints [optional]

We plan to create a Windows Node Monitoring Ansible playbook that can
* Download WMI exporter Microsoft Installer from a well known location and 
transfers it to the Windows Node
* Run WMI msi on the Windows Node
* Ensure that WMI is running as a Windows Service on the node
* Create a namespace called `openshift-windows` with 
  `openshift.io/cluster-monitoring=true` label so that cluster monitoring
  stack will pick up the service monitor in the next steps.
* Create service, endpoint in `openshift-windows` pointing to WMI exporter 
  endpoint
* Create a service monitor in `openshift-windows` namespace for service
  created above

The inputs to this playbook
* The internal ip address of the Windows Node where we want the WMI exporter
  to run.
* URL from which WMI msi can be downloaded from

### Justification

The reason for having ansible playbook instead of operator with a container
image is that all Windows container images contains a Windows Kernel and 
Red Hat has a policy not to ship 3rd party kernels for support reasons. Please
look at [alternatives](#Alternatives) section

### Risks and Mitigations

The main risks with this proposal are as follows:
* We are depending on Microsoft to provide us supported version of WMI. As of 
  now, WMI is hosted at a repo managed by 
  [individual](https://github.com/martinlindhe/wmi_exporter).
  
  
Mitigation
* If Microsoft doesn't provide us with WMI, we would just go ahead with the 
  upstream repo and start using it.

## Design Details

### Test Plan

We plan to add e2e tests to ensure
* Service, endpoints and Service Monitors are properly created
* Prometheus is able to collect data from WMI exporter  

### Graduation Criteria

This enhacement will start as GA

### Upgrade / Downgrade Strategy

We will support upgrades/downgrades by publishing a new release of Windows Node
Monitoring playbook. An older release of the playbook can be used to downgrade.

## Implementation History

v1: Initial Proposal

## Drawbacks

Running WMI exporter as a Windows Service instead of running as a DaemonSet pod
makes it hard for the Prometheus to monitor. The limitation of not able to run
WMI exporter on Windows nodes as a pod is because of 
* Lack of previleged containers on Windows nodes 
* Windows doesn't write the proc, cpu and mem stats to single file

So, in order to overcome the limitation we're running WMI exporter as a Windows
service but that makes the monitoring of the Windows Nodes harder.
 

## Alternatives

An alternative approach would be write an operator which is responsible for 
installing WMI exporter to Windows nodes and ensures that it runs as a
Windows Service and distribute this operator as a Windows binary. The downsides
to this approach are 
* Once Windows and WMI exporter community reach a stage where WMI can run as a 
  container, we can leverage the cluster monitoring operator to create a 
  DaemonSet like node_exporter for linux and we wouldn't need operator to 
  manage it.
* The operator needs to get credentials with enough privileges to create 
  services, endpoint, service monitors

Considering the above downsides, timeframe and all the unknowns present in this
project, we have decided not to go with operator based model

In the ansible based approach, we'd run ansible playbook from the master nodes
which already has privilege to access the `openshift-monitoring` namespace. 

## Infrastructure Needed

Windows workers nodes will be available for the e2e tests to run against. The
existing openshift-ansible github repository will host the code being 
implemented as part of this feature.
