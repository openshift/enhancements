---
title: cluster-logging-v2-apis
authors:
- "@jcantril"
reviewers:
- "@alanconway, Red Hat Logging Architect"
- "@xperimental"
- "@syedriko"
- "@cahartma"
approvers:
- "@alanconway"
api-approvers:
- "@alanconway"
creation-date: 2023-10-30
last-updated: 2023-10-30
tracking-link:
- https://issues.redhat.com/browse/OBSDA-550
see-also:
  - "/enhancements/cluster-logging-log-forwarding.md"
  - "/enhancements/forwarder-input-selectors.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---


# v2 API for Log Forwarding and Log Storage
## Summary

Logging for Red Hat OpenShift has evolved since its initial release in OpenShift 3.x from an on-cluster, highly opinionated offering to a more flexible log forwarding solution that supports multiple internal (e.g LokiStack, Elasticsearch) and externally managed log storage.  Given the original components (e.g. Elasticsearch, Fluentd) have been deprecated for various reasons, this enhancement introduces the next version of the APIs in order to formally drop support for those features as well as to generally provide an API to reflect the future direction of log storage and forwarding.

## Motivation

### User Stories

The next version of the APIs should continue to support the primary objectives of the project which are: 

* Collect logs from various sources and services running on a cluster
* Normalize the logs to common format to include workload metadata (i.e. labels, namespace, name)
* Forward logs to storage of an administrator's choosing (e.g. LokiStack)
* Provide a Red Hat managed storage solution
* Provide an interface to allow users to review logs from a Red Hat managed storage solution

The following user stories describe deployment scenarios to support these objectives:

* As an administrator, I want to deploy a complete operator managed logging solution that includes collection, storage, and visualization so I can evaluate log records while on the cluster
* As an administrator, I want to deploy an operator managed log collector only so that I can forward logs to an existing storage solution
* As an administrator, I want to deploy an operator managed instance of LokiStack and visualization

The administrator role is any user who has permissions to deploy the operator and the cluster-wide resources required to deploy the logging components.

### Goals

* Drop support for the **ClusterLogging** custom resource
* Drop support for **ElasticSearch**, **Kibana** custom resources and the **elasticsearch-operator**
* Drop support for Fluentd collector implementations, Red Hat managed Elastic stack (e.g. Elasticsearch, Kibana)
* Drop support in the **cluster-logging-operator** for **log-view-plugin** management 
* Support log forwarder API with minimal dependency upon reserved words (e.g. default)
* Support an API to spec a Red Hat managed LokiStack with the logging tenancy model
* Support an API to allow flexible deployment of the logging components: collector/forwarder, storage, visualization
* Continue to allow deployment of a log forwarder to the output sinks of the administrators choosing


### Non-Goals

* "One click" deployment of a full logging stack as provided by **ClusterLogging** v1
* Complete backwards compatibility to **ClusterLogForwarder** v1
* Automated migration path from v1 to v2

## Proposal

This is where we get down to the nitty gritty of what the proposal
actually is. Describe clearly what will be changed, including all of
the components that need to be modified and how they will be
different. Include the reason for each choice in the design and
implementation that is proposed here, and expand on reasons for not
choosing alternatives in the Alternatives section at the end of the
document.

### Workflow Description

The following workflow describes the first user story which is a superset of the others and allows deployment of a full logging stack to collect and forward logs to a Red Hat managed log store.

**cluster administrator** is a human responsible for:

* Managing and deploying day 2 operators
* Managing and deploying an on-cluster LokiStack
* Managing and deploying a cluster-wide log forwarder

**obervability-operator** is an operator responsible for:

* managing and deploying observability plugins (e.g log-view-plugin)

**loki-operator** is an operator responsible for managing a loki stack

**cluster-logging-operator** is an operator responsible for managing log collection and forwarding

The cluster administrator does the following:

1. Deploys the Red Hat **observability-operator**
1. Deploys the Red Hat **loki-operator**
1. Deploys an instance of **LokiStack** in the `openshift-logging` namespace
1. Deploys the Red Hat **cluster-logging-operator**
1. Creates a **ClusterLogForwarder** custom resource for the **LokiStack**

The **observability-operator**:
1. Deploys the logging-view-plugin for reading logs in the OpenShift console

The **loki-operator**:
1. Deploys the **LokiStack** for storing logs on-cluster

The **cluster-logging-operator**:

1. Deploys the log collector to forward logs to log storage in the `openshift-logging` namespace

### API Extensions

This API defines the following opinionated input sources which is a continuation of prior versions:

* **application**: Logs of container workloads running in all namespaces except **default**, **openshift***, and **kube*** 
* **infrastructure**: journald logs from OpenShift nodes and container workloads running only in namespaces **default**, **openshift***, and **kube***
* **audit**: The logs from OpenShift nodes written to the node filesystem by: Kubernetes API server, OpenShift API server, Auditd, and OpenShift Virtual Network (OVN).

These are **well-known** input sources that can be referenced by a pipeline without an explicit input specification.

Additional specification of **audit** and **infrastructure** logs is allowed by creating a named input of that type and specifiying at least one of the allowed sources.

#### CustomResourceDefinition:
```yaml
    apiVersion: "logging.openshift.io/v2"
    kind: ClusterLogForwarder
    metadata:
      name: 
    spec:
      serviceAccountName:
      namespace:         #namespace of deployment and resources
      collector:
        resources:       #corev1.ResourceRequirements
          limits:        #cpu, memory
          requests:
        nodeSelector:    #map[string]string
        tolerations:     #corev1.Toleration
      inputs:
      - name:
        type:                      #enum: application,infrastructure,audit
        application:
          selector:                #labelselector
          namespaces:
            include: []            #glob
            exclude: []            #glob
          containers:
            include: []            #glob
            exclude: []            #glob
          tuning:
            ratelimitDefault:      # for containers not mentioned in rateLimitByContainer
              recordsPerSecond:    # int 
            ratelimitByContainer:  # map[string]RateLimit (e.g ngnix: {recordsPerSecond: 20}) 
        infrastructure:
          sources: []              #enum: node,container
        audit:
          sources: []              #enum: auditd,kubeAPI,openshiftAPI,ovn
        receiver:  
      filters:
      - name:
        type:              #kubeapiaudit, detectmultiline, parse, labels
        kubeAPIAudit:
        parse:
      pipelines:
       - inputRefs: []
         outputRefs: []
         filterRefs: []
      outputs:
      - name: 
        type:                    #enum
        url:
        tls:
        secret:
        tuning:
          rateLimitDefault:
            recordsPerSecond:  #int - document per-forwarder/per-node multiplier
          delivery:         #AtMostOnce, AtLeastOnce
          maxWrite:         # quantity (e.g. 500k)
          compression:      # enum of supported algos specific to the output
          minRetryDuration:
          maxRetryDuration:
        cloudwatch:
          region:
          groupBy:         # enum.  should support templating?
          groupPrefix:     # should support templating?
        elasticsearch:
          version:
          index:           # templating? do we need structured key/name or is this good enough
          enableStructuredContainerLogs: #drop? we can do this now with custom app inputs
        googleCloudLogging:
          billingAccountID:
          organizationID:
          folderID:    # templating?
          projectID:  # templating?
          logID:      # templating?
        http:
          headers:
          timeout:
          method:
          schema:   #via,opentelemetry.  drop?  can't we do this with a filter
        kafka:
          topic:   #templating?
          brokers:
        lokiStack:
          tenantID:  #templating?
          labelKeys: 
        splunk:
          fields:
          index:  #templating?
        syslog:    #only supports RFC5424
          severity:
          facility:
          trimPrefix:
          tagKey:      #templating?
          payloadKey:   #templating?
          addLogSource:
          appName:  #templating?
          procID:  #templating?
          msgID:  #templating?
    status:
      conditions:    # []metav1.conditions
      inputs:        # map[string] metav1.conditions
      outputs:       # map[string] metav1.conditions
      filters:       # map[string] metav1.conditions
      pipelins:      # map[string] metav1.conditions      
```


Example:

```yaml
    apiVersion: "logging.openshift.io/v2"
    kind: ClusterLogForwarder
    metadata:
      name: log-collector
    spec:
      inputs:
      - name: infra-container
        type: infrastructure
        infrastructure:
          sources: [container]
      serviceAccount:
        name: audit-collector-sa
        namespace: acme-logging
      pipelines:
       - inputRefs:
         - infra-container
         - audit
         outputRefs:
         - rh-loki
      outputs:
      - name: rh-loki
        type: lokiStack
```

This example:

* Deploys a log collector to the 'acme-logging' namespace
* Expects the administrator to have created a service account named 'audit' in that namespace
* Expects the administrator to have bound the roles 'collect-audit-logs', 'collect-infrastructure-logs to the service account
* Expects the administrator created a **LokiStack** CR named 'rh-loki' in the 'openshift-logging' namespace
* Collects all audit log sources and only infrastructure container logs and writes them to the Red Hat managed lokiStack

### Implementation Details/Notes/Constraints [optional]

#### Log Storage

Deployment of log storage is a separate task of the administrator.  They deploy a custom resource to be managed by the **loki-operator**.  They will additionally specify forwarding logs to this storage by defining an output in the **ClusterLogForwarder**

#### Log Visualization

The **observability-operator** will take ownership of the management of the **log-view-plugin**.  This requires feature changes to the operator and the OpenShift console before being fully realized.  Both v1 and v2 of the API object will be provided by the operator during a transitional period until v2 achieves GA.  Administrators will create a **ClusterLogging** object to specify visualization until such time the **observability-operator** is available to provide the functionality.  The **cluster-logging-operator** will be updated with logic (TBD) to recognize the **observability-operator** is able to deploy the plugin and will remove its own deployment in deference to the **observability-operator**.

#### Log Collection and Forwarding

V2 of the **ClusterLogForwarder** is a cluster-wide resource.  It depends upon a **ServiceAccount** to which roles must be bound (e.g. mounting node filesystem, collecting logs).  Collectors will be deployed to the namespace of the **ServiceAccount** referenced in the spec.

The Red Hat managed logstore is represented by a 'lokiStack' output type defined without an URL
with the following assumptions:

* Named the same as a **LokiStack** CR deployed in the 'openshift-logging' namespace
* Follows the logging tenant model

The **cluster-logging-operator** will:

* Internally migrate the **ClusterLogForwarder** to craft the URL to the **LokiStack**

#### Data Model

Log forwarding will provide filters? to allow the selection of the normalization model??

##### Viaq V2
Sample:
```yaml
    model_version: v2.0
    timestamp:
    hostname:
    severity:
    kubernetes:
      container_id:
      container_image:
      host:
      pod_name:
      namespace_name:
      namespace_labels:  #map[string]string
      container_name:
      labels:  #map[string]string, underscore, dedoted, deslashed
    message:
    structured:  #map[string]
    openshift:
      cluster_id:
      log_type:
      log_source:  #journal, ovn, etc
      sequence:
```

##### OpenTelemetry
Sample:
```yaml
    ????
```


### Risks and Mitigations

#### User Experience
The product is no longer offering a "one-click" experience for deploying a full logging stack from collection to storage.  Given we started moving away from this experience when Loki was introduced, this should be low risk.  Many customers already have their own log storage solution so they are only making use of log forwarding.  Additionally, it is intended for the **observability-operator** to recognize the existing of the internally managed log storage and automatically deploy the view plugin.  This should reduce the burden of administrators

#### Security
The risk of forwarding logs to unauthorized destinations remains as from previous releases.  This enhancement embraces the design from multi cluster log forwarding by requiring administrators to provide a service account with the proper permissions.  The permission scheme relies upon RBAC offered by the platform and places the control in the hands of administrators.


### Drawbacks

The largest drawback to implementing new APIs is the product continues to identify the
availability of technologies which are deprecated and will soon not be supported.  This will
continue to confuse comsumers of logging and will require documentation and explainations of our technology decisions.  Furthermore, some customers will continue to delay the move to the newer technologies provided by Red Hat.


## Design Details

### Open Questions [optional]

1. How do we support the APIs side-by-side given OLM team advised us not to utilize webhooks

### Test Plan

* Exectue all existing tests for log collection, forwarding and storage with the exeception of tests specifically intended to test deprecated features (e.g. Elasticsearch).  Functionally, other other tests are still applicable

* Execute a test to verify the flow defined for collecting, storing, and visualizing logs from an on-cluster, Red Hat operator managed Loki Stack

### Graduation Criteria

#### Dev Preview Release

This release:

* Intends to support the use-cases described within this proposal
* Intends to distibute v2alpha1 of the APIs described within this proposal
* May introduce v2 of the VIAQ data model
* Allows v2alpha1 APIs to exist along side v1 APIs (i.e. **ClusterLogging**, **ClusterLogForwarder**)

#### GA Release

This release:

* Intends to support the use-cases described within this proposal
* Intends to distibute v2 of the APIs described within this proposal
* May support multiple data models (e.g OpenTelementry, VIAQ v2)
* Drop support of v1 APIs (i.e. **ClusterLogging**, **ClusterLogForwarder**)

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

- Ability to utilize the enhancement end to end
- Sufficient test coverage
- Sufficient time for feedback
- Gather feedback from users rather than just developers
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)


#### Removing a deprecated feature

Upon GA release of this enhancement:

- The internally managed Elastic (e.g. Elasticsearch, Kibana) offering will no longer be available.
- The Fluentd collector implementation will no longer be available

### Upgrade / Downgrade Strategy

There is no automated upgrade path between v1 and v2 of the APIs.  Administrators will migrate between the two versions.  This primary affects users of log forwarding as

* **LokiStack** is unaffected by this proposal and not managed by the **cluster-logging-operator**
* There is a migration path for log visualization which will ony require interaction if the **observability-operator** offers a custom resource

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Given most of the changes will result in an operator that manages only log collection and forwarding, we could release a new operator for that purpose only that provides only v2 log forwarding APIs

## Infrastructure Needed [optional]

