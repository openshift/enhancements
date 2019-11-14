---
title: openshift-windows-worker-node-and-container-logging
authors:
  - "@ravisantoshgudimetla"
  - "@aravindhp"
reviewers:
  - "@crawford"
  - "@sdodson"
  - "@jcantrill"
  - "@richm" 
approvers:
  - "@crawford"
  - "@sdodson"
  - "@jcantrill"
  - "@richm"  
creation-date: 2019-09-10
last-updated: 2019-09-17
status: implementable
---

# OpenShift Windows Worker Node and Container Logging

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The intent of this enhancement is to enable logging for Windows worker nodes 
and pods running in an OpenShift cluster

## Motivation

Logging is a critical component for identifying issues with nodes, containers
running on the nodes. The main motivation behind this enhancement is to bring 
Windows node logging infrastructure on par with linux nodes

### Goals

As part of this enhancement, we plan to do the following:
* Deploy log collection infrastructure onto Windows nodes
* Collect node and pod logs
* Upgrade log collection infrastructure
* Forward logs and events from nodes and pods to log store

### Non-Goals

As part of this enhancement, we do not plan to support:
* Logging for Windows containers
  * Microsoft will provide us with the logging framework that enables Windows 
    containers to redirect logs to standard out from various sources like 
    ETW, Perf Counters etc
* Customization of log collection infrastructure
  * We plan to support fluentd only
  * Microsoft will provide us with a Windows fluentd binary or gem file


## Proposal

The main idea here is to deploy fluentd as a log collection agent onto Windows 
nodes after cluster logging operator is deployed onto the cluster. Fluentd has
support for Windows logging. We plan to leverage it and run fluentd as a 
Windows service.


### Implementation Details

We plan to create a Windows Node logging Ansible Playbook that can

* Download fluentd binary for Windows from a known location and transfers it to
  the Windows node
* Configure the container runtime to use fluentd as the logging driver
* Configure fluentd to communicate with elasticsearch data store in the same 
  format as logs emitted from linux worker nodes

The inputs to this playbook

* The ip address, port, certificate to connect the elasticsearch service
* URL from which fluentd Windows binary can be downloaded from

We can get the ipaddress and other details from various pods and secrets present
in the OpenShift cluster, once the cluster logging operator has been deployed.
If the certificates change, cluster admin should run the ansible playbook
again. 

Kubelet on Windows nodes logs to a file. So, we also need to configure 
fluentd's record transformer to ensure that logs collected from the kubelet log
file are in format understandable to elasticsearch

### Justification

Following are the reasons for having this ansible playbook approach instead of 
an operator driven approach:

* When we move to an operator based model, we would like to use the cluster 
  logging operator for managing the logging on Windows nodes instead of
  creating our own operator
* Cluster logging operator will undergo changes related to the log collection 
  API in the 4.3 timeframe and that uncertainity makes it difficult to introduce
  Windows specific changes

This design allows us to be future proof where in the ansible playbook can be 
converted to an operator.
 
### Risks and Mitigations

The main risk with this proposal are the following dependencies on Microsoft: 

* To provide us with fluentd binary
* Container logging works on Windows nodes
* Container runtime writes container logs in a format understandable by fluentd
  plugin

Mitigations:

* If fluentd binary is not shipped by Microsoft, we plan to install fluentd 
  from upstream which will result in Red Hat being responsible for security and 
  other fixes. 
* If Microsoft doesn't provide container logging framework on Windows nodes, we
  would just support node logging in 4.3 timeframe

## Design Details

### Test Plan

We plan to add e2e tests to ensure 

* Fluentd service is running on Windows node
* Windows nodes are forwarding logs properly to elasticsearch

### Graduation Criteria

This enhancement will start as GA

### Upgrade / Downgrade Strategy

We will support upgrades/downgrade of fluentd by publishing a new release of 
Windows Node logging playbook. An older release of the playbook can be used to 
downgrade.


## Implementation History

v1: Initial Proposal

## Drawbacks

The applications running in Windows containers may not always log to stdout
which is what container runtimes expect. Microsoft is working on tool that
is capable of collecting application metrics from disparate sources like 
ETW(Event Tracing for Windows), Perf Counter, Custom application logs and
route them to stdout for a container but this work is not production ready.
Given this limitation, the container logging on Windows nodes may not be
as good as their linux counterparts, causing a degraded experience to 
customers.


## Alternatives

An alternative approach would be to make changes to cluster logging operator to
ensure that it can manage fluentd DaemonSet pod on the Windows nodes

We are deciding to not go ahead with this approach considering the timeframe
and all the unknowns present in this project.

## Infrastructure Needed 

Windows worker nodes will available for the e2e tests to run against. The
existing openshift-ansible github repository will host the code being 
implemented as part of this feature.
