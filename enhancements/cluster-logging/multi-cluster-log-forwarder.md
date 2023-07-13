---
title: multi-cluster-log-forwarder
authors:
- "@jcantril"
reviewers:
- "@alanconway, Red Hat Logging Architect"
approvers:
- "@alanconway"
api-approvers:
- "@alanconway"
creation-date: 2023-02-23
last-updated: 2023-07-12
tracking-link:
- https://issues.redhat.com/browse/LOG-1344
see-also:
-
replaces:
-
---

# Multi ClusterLogForwarder
## Summary

Log forwarding is functionally a "cluster singleton" where the operator explicitly only reconcilies a **ClusterLogForwarder** in the  namespace *openshift-logging* named *instance*.  This enhancement removes that restriction to allow administrators to define multiple instance of **ClusterLogForwarder** while retaining the legacy behavior.


## Motivation

### User Stories


* As an administrator of a Red Hat managed cluster, I want to RBAC my log forwarder configuration from customer admins so they can take ownership of their log forwarder needs without being able to modify mine.
* As an administrator of Hosted Control Planes, I want to deploy individual log forwarders to isolate audit log collection of each managed control plane.
* As an administrator adopting vector, I want to deploy it separately from my existing fluentd deployment so they can operate side-by-side and I can migrate my workloads.

### Goals

* Cluster administrators control which users are allowed to define log collection and which logs they are allowed to collect.
* Users with allowable permissions are able to specify additional log collection configurations
* Log forwarder deployments are isolated so they do not interfere with other log forwarder deployments
* Support ClusterLogForwarders simultaneously in legacy and multiple instance modes

### Non-Goals

* Introduction of the next version of logging APIs.
* Adding RBAC to the output destinations to restrict where logs can be forwarded

## Proposal

### Workflow Description
This proposal identifies two separate workflows in order to support the legacy deployment and allow additional deployments to meet the enhancement goals.  The legacy deployment will be familiar to users of ClusterLogForwarder 
prior to the implementation of this enhancement.  They should see no differences in the manner by which they use log forwarding.  The new workflow will require additional permissions to create new ClusterLogForwarders in order 
to limit the number of deployments for resource concerns.  Cluster administrators will need to explicitly allow additional deployments.

The workflows make the following assumptions:

* The **cluster-logging-operator** is deployed to the *openshift-logging* namespace
* The **cluster-logging-operator** is able to watch any namespace

#### Multiple-Instance Mode: Allowing multiple ClusterForwarder and ClusterLogging resources 

This workflow supports any ClusterLogForwarder except one named "instance" in the *openshift-logging namespace*.  The resource openshift-logging/instance is significant to supporting the legacy workflow.

**NOTE:** Vector is the only supported collector implementation in this mode.

**cluster administrator** is a user:

* responsible for maintaining the cluster
* able to bind cluster roles to serviceaccounts  
* that deploys the **cluster-logging-operator**

**namespace administrator** is a user:

* able to create a serviceaccount
* able to create a serviceaccount token
* manages a **ClusterLogForwarder** custom resource

The general workflow:

* The namespace administrator creates a service account to be used by a log collector.  The service account must additionally include a token if there is intent to write to log storage that depends upon a token for authentication.
* The cluster administrator binds cluster roles to the service account for the log types they are allowed to collect (e.g. audit, infrastructure).  Several roles are added to the operator manifest and look something like:

```yaml
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: collect-audit-logs
    rules:
    - apiGroups:
      - "logging.openshift.io"
      resources:
      - logs
      resourceNames:
      - audit
      verbs:
      - collect

```
This role allows collection of application logs and requires the namespace administor to bind the service account to the role like:

```text
    oc create clusterrolebinding kube-audit-log-collection --clusterrole=collect-audit-logs --serviceaccount=openshift-kube-apiserver:audit-collector-sa
```

* The namespace administrator creates a **ClusterLogForwarder** CR that references the serviceaccount and the inputs for which that serviceaccount is allowed to collect

```yaml
    apiVersion: "logging.openshift.io/v1"
    kind: ClusterLogForwarder
    metadata:
      name: audit-collector
      namespace: openshift-kube-apiserver
    spec:
      serviceAccount: audit-collector-sa
      pipelines:
       - inputRefs:
         - audit
         outputRefs:
         - loki
      outputs:
      - name: loki
        type: loki
        url: https://mycenteralizedserver.some.place
```

##### Use of ClusterLogging resource
This resource is optional in multiple instance mode and is a departure from the legacy mode where a ClusterLogging resource is always required with a ClusterLogForwarder. A namespace administrator must define a **ClusterLogging** CR named the same as the **ClusterLogForwarder** CR and in the same namespace when needing to spec collector resources or placement.

```yaml
    apiVersion: "logging.openshift.io/v1"
    kind: "ClusterLogging"
    metadata:
      name: audit-collector
      namespace: openshift-kube-apiserver
    spec:
      collection:
        type: "vector"
```

The relevent spec level fields for this CR in multiple instance mode are:

* managmentState
* collection

All other spec fields are ignored: logStore, visualization, curation, forwarder, collection.logs


##### Verification and Validations 
The operator will validate resources upon reconciliation of a **ClusterLogForwarder** and **ClusterLogging** CR.  Failure to meet any of the following conditions will stop the operator from deploying a collector and it will add error status to the resource.

* The **ClusterLogForwarder** CR defines a valid spec
* The serviceaccount defined in **ClusterLogForwarder** CR is bound to clusterroles that allow the input spec of the **ClusterLogForwarder** CR
* When a **ClusterLogging** CR is deployed that has a matching name and namespace to a **ClusterLogForwarder** CR it must only define a valid collection spec.

The previous example identifies a valid **ClusterLogForwarder** CR that specs audit logs forwarded to a loki stack.  The following is an example of a CR rejected by the operator because it specs collection of application logs but does not have the required role binding:

```yaml
    apiVersion: "logging.openshift.io/v1"
    kind: ClusterLogForwarder
    metadata:
      name: audit-collector
      namespace: openshift-kube-apiserver
    spec:
      pipelines:
       - inputRefs:
         - audit
         - application
         outputRefs:
         - loki
      outputs:
      - name: loki
        type: loki
        url: https://mycenteralizedserver.some.place
```


#### Legacy Mode: Allow only a single ClusterForwarder and ClusterLogging resource in openshift-logging

This workflow is the exising, legacy workflow.  It relies upon oppinionated resource names in an explicit namespace.  There are two variations to this workflow: administrator provides **ClusterLogging** CR with or without a **ClusterLogForwarder**.  This workflow continues to function as it has for previous releases of logging prior to the implementation of this proposal:

* **ClusterLogging** CR which specs collection and logstore results in a deployment that collects application and infrastructure logs and forwards to logging operator managed log store (e.g. loki, elasticsearch)
* **ClusterLogging** CR which specs at least collection and a **ClusterLogForwarder** CR which defines forwarding results in a deployment that at a minimum is a collector that forwards logs to the defined outputs

### API Extensions
None

### Implementation Details/Notes/Constraints

#### Log File Metric Exporter as a Separate Deployment

The cluster logging project provides a component to gather metrics about the volume of application logs being generated on each node in the cluster.  Prior to this enhancement this component was deployed as part of the collector pod.  This proposal will:

* move this component into a separate deployment from the collector
* introduce API to support configuring the component
* Explicitly only reconcile the object in the namespace *openshift-logging* named *instance*

```yaml
    apiVersion: "logging.openshift.io/v1alpha1"
    kind: LogFileMetricExporter
    metadata:
      name: instance 
      namespace: openshift-logging 
    spec:
      tolerations:
      resources:
       limits:
       requests:
```
* restrict the number of deployments to 1 as no more then one is required per cluster
* upgrade existing cluster logging deployments to separate the collector from the log-file-metric-exporter
* deploy a **LogFileMetricExporter** when there exists a **ClusterLogForwarding** or **ClusterLogging** in the namespace *openshift-logging* named *instance* and there is no **LogFileMetricExporter** CR

#### Metrics Dashboards

* Deploy singleton instance of the collector dashboard if **ClusterLogForwarder** count >= 1
* Refactor the dashboard to be agnostic of collector implementation and support multiple collector deployments


#### Metrics Alerts
* Deploy singleton instance of the alerts if **ClusterLogForwarder** count >= 1
* Refactor alerts to be agnostic of collector implementation and support multiple collector deployments


### Risks and Mitigations

* Are we properly supporting the app-sre?
* Are we properly supporting Hosted Control Planes?

### Drawbacks

## Design Details

### Open Questions [optional]

1. Is there any reason we need to support fluentd deployments for this feature given we consider fluentd deprecated?

### Test Plan
* Verify existing (legacy) deployments upgrade without regression
* Verify administrators can create legacy mode deployments as documented in logging 5.7 without regression

### Graduation Criteria
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature

### Upgrade / Downgrade Strategy


### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures


## Implementation History

## Alternatives


