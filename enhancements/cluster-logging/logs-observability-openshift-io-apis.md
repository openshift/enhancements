---
title: logs-observability-openshift-io-apis
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
creation-date: 2024-04-18
last-updated: 2023-04-18
tracking-link:
- https://issues.redhat.com/browse/OBSDA-550
see-also:
  - "/enhancements/cluster-logging-log-forwarding.md"
  - "/enhancements/forwarder-input-selectors.md"
  - "/enhancements/cluster-logging/multi-cluster-log-forwarder.md"
replaces: []
superseded-by: []
---


# observability.openshift.io/v1 API for Log Forwarding and Log Storage
## Summary

Logging for Red Hat OpenShift has evolved since its initial release in OpenShift 3.x from an on-cluster, highly opinionated offering to a more flexible log forwarding solution that supports multiple internal (e.g LokiStack, Elasticsearch) and externally managed log storage.  Given the original components (e.g. Elasticsearch, Fluentd) have been deprecated for various reasons, this enhancement introduces the next version of the APIs in order to formally drop support for those features as well as to generally provide an API to reflect the future direction of log storage and forwarding.

## Motivation

### User Stories

The next version of the APIs should continue to support the primary objectives of the project which are:

* Collect logs from various sources and services running on a cluster
* Normalize the logs to common format to include workload metadata (i.e. labels, namespace, name)
* Forward logs to storage of an administrator's choosing (e.g. LokiStack)
* Provide a Red Hat managed log storage solution
* Provide an interface to allow users to review logs from a Red Hat managed storage solution

The following user stories describe deployment scenarios to support these objectives:

* As an administrator, I want to deploy a complete operator managed logging solution that includes collection, storage, and visualization so I can evaluate log records while on the cluster
* As an administrator, I want to deploy an operator managed log collector only so that I can forward logs to an existing storage solution
* As an administrator, I want to deploy an operator managed instance of LokiStack and visualization

The administrator role is any user who has permissions to deploy the operator and the cluster-wide resources required to deploy the logging components.

### Goals

* Drop support for the **ClusterLogging** custom resource
* Drop support for **ElasticSearch**, **Kibana** custom resources and the **elasticsearch-operator**
* Drop support for Fluentd collector implementation, Red Hat managed Elastic stack (e.g. Elasticsearch, Kibana)
* Drop support in the **cluster-logging-operator** for **logging-view-plugin** management
* Support log forwarder API with minimal or no dependency upon reserved words (e.g. default)
* Support an API to spec a Red Hat managed LokiStack with the logging tenancy model
* Continue to allow deployment of a log forwarder to the output sinks of the administrators choosing
* Automated migration path from *ClusterLogForwarder.logging.openshift.io/v1* to *ClusterLogForwarder.observability.openshift.io/v1*

### Non-Goals

* "One click" deployment of a full logging stack as provided by **ClusterLogging** v1
* Complete backwards compatibility to **ClusterLogForwarder.logging.openshift.io/v1** v1


## Proposal


### Workflow Description

The following workflow describes the first user story which is a superset of the others and allows deployment of a full logging stack to collect and forward logs to a Red Hat managed log store.

**cluster administrator** is a human responsible for:

* Managing and deploying day 2 operators
* Managing and deploying an on-cluster LokiStack
* Managing and deploying a cluster-wide log forwarder

**cluster-observability-operator** is an operator responsible for:

* managing and deploying observability operands (e.g. LokiStack, ClusterLogForwarder, Tracing) and console plugins (e.g console-logging-plugin)

**loki-operator** is an operator responsible for managing a loki stack.

**cluster-logging-operator** is an operator responsible for managing log collection and forwarding.

The cluster administrator does the following:

1. Deploys the Red Hat **cluster-observability-operator**
1. Deploys the Red Hat **loki-operator**
1. Deploys an instance of **LokiStack** in the `openshift-logging` namespace
1. Deploys the Red Hat **cluster-logging-operator**
1. Creates a **ClusterLogForwarder** custom resource for the **LokiStack**

The **cluster-observability-operator**:
1. Deploys the console-logging-plugin for reading logs in the OpenShift console

The **loki-operator**:
1. Deploys the **LokiStack** for storing logs on-cluster

The **cluster-logging-operator**:

1. Deploys the log collector to forward logs to log storage in the `openshift-logging` namespace

### API Extensions

This API defines the following opinionated input sources which is a continuation of prior cluster logging versions:

* **application**: Logs of container workloads running in all namespaces except **default**, **openshift***, and **kube***
* **infrastructure**: journald logs from OpenShift nodes and container workloads running only in namespaces **default**, **openshift***, and **kube***
* **audit**: The logs from OpenShift nodes written to the node filesystem by: Kubernetes API server, OpenShift API server, Auditd, and OpenShift Virtual Network (OVN).

These are **reserved** words that represent input sources that can be referenced by a pipeline without an explicit input specification.

More explicit specification of **audit** and **infrastructure** logs is allowed by creating a named input of that type and specifiying at least one of the allowed sources.

This is a namespaced resource that follows the rules and [design](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-logging/multi-cluster-log-forwarder.md) described in the multi-ClusterLogForwarder proposal with the following exceptions:

* Drops the `legacy` mode described in the proposal.
* Moves collector specification to the **ClusterLogForwarder**

#### ClusterLogForwarer CustomResourceDefinition:

Following defines the next version of a ClusterLogForwarder. **Note:** The next version of this resources is part of a new API group to align log collection with
the objectives of Red Hat observability.  

```yaml
apiVersion: "observability.openshift.io/v1"
kind: ClusterLogForwarder
metadata:
  name:
spec:
  managementState:   #enum: Managed, Unmanaged
  serviceAccount:
    name:
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
      includes:
      - namespace:
        container:
      excludes:
      - namespace:
        container:
      tuning:
        ratelimitPerContainer:  #rate limit applied to each container selected by this input
          recordsPerSecond:    #int  (no multiplier, a each container only runs on one node at a time.)
    infrastructure:
      sources: []              #enum: node,container
    audit:
      sources: []              #enum: auditd,kubeAPI,openshiftAPI,ovn
    receiver:
      type:                    #enum: syslog,http
      port:
      http:
        format:                #enum: kubeAPIAudit , format of incoming data
      tls:
        ca:
          key:                 #the key in the resource
          configmap:
            name:                # the name of resource
          secret:
            name:                # the name of resource
        certificate:
          key:                   #the key in the resource
          configmap:
            name:                # the name of resource
          secret:
            name:                # the name of resource
        key:
          key:                   #the key in the resource
          secret:
            name:                # the name of resource
        keyPassphrase:
          key:                   #the key in the resource
          secret:
            name:                # the name of resource
  filters:
  - name:
    type:                      #enum: kubeAPIaudit, detectMultilineException, parse, openshiftLabels, drop, prune
    kubeAPIAudit:
    parse:
  pipelines:
    - inputRefs: []
      outputRefs: []
      filterRefs: []
  outputs:
  - name:
    type:                    #enum: azureMonitor,cloudwatch,elasticsearch,googleCloudLogging,http,kafka,loki,lokiStack,splunk,syslog
    tls:
      ca:
        key:                 #the key in the resource
        configmap:
          name:                # the name of resource
        secret:
          name:                # the name of resource
      certificate:
        key:                   #the key in the resource
        configmap:
          name:                # the name of resource
        secret:
          name:                # the name of resource
      key:
        key:                   #the key in the resource
        secret:
          name:                # the name of resource
      keyPassphrase:
        key:                   #the key in the resource
      insecureSkipVerify:      #bool
      securityProfile:         #openshiftv1.TLSSecurityProfile
    rateLimit:
      recordsPerSecond:  #int - document per-forwarder/per-node multiplier
    azureMonitor:
      customerId:
      logType:
      azureResourceId:
      host:
      authorization:
        sharedKey:
          key:
          secret:
            name:                # the name of resource
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        minRetryDuration:
        maxRetryDuration:
    cloudwatch:
      region:
      groupBy:         # enum.  should support templating?
      groupPrefix:     # should support templating?
      authorization:   # output specific auth keys
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
        minRetryDuration:
        maxRetryDuration:
    elasticsearch:
      url:
      version:
      index:           # templating? do we need structured key/name or is this good enough
      authorization:   # output specific auth keys
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
        minRetryDuration:
        maxRetryDuration:
    googleCloudLogging:
      ID:
        type:          #enum: billingAccount,folder,project,organization
        value:         
      logID:           # templating?
      authorization:   # output specific auth keys
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
        minRetryDuration:
        maxRetryDuration:
    http:
      url:
      headers:
      timeout:
      method:
      authorization:   # output specific auth keys
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
        minRetryDuration:
        maxRetryDuration:        
    kafka:
      url:
      topic:           #templating?
      brokers:
      authorization:   # output specific auth keys
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
    loki:
      url:
      tenant:                # templating?
      labelKeys:
      authorization:   # output specific auth keys
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
        minRetryDuration:
        maxRetryDuration:
     lokiStack:              # RH managed loki stack with RH tenant model
      target:
        name:
        namespace:
      labelKeys:
      authorization:
        token:
          key:
          secret:
            name:                # the name of resource
          serviceAccount:
            name:
        username:
          key:
          secret:
            name:                # the name of resource
        password:
          key:
          secret:
            name:                # the name of resource
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
        minRetryDuration:
        maxRetryDuration:
    splunk:
      url:
      index:           #templating?
      authorization:
        secret:              #the secret to search for keys
          name:
        # output specific auth keys
      tuning:
        delivery:            # enum: AtMostOnce, AtLeastOnce
        maxWrite:            # quantity (e.g. 500k)
        compression:         # enum of supported algos specific to the output
        minRetryDuration:
        maxRetryDuration:
    syslog:            #only supports RFC5424?
      url:
      severity:
      facility:
      trimPrefix:
      tagKey:         #templating?
      payloadKey:     #templating?
      addLogSource:
      appName:        #templating?
      procID:         #templating?
      msgID:          #templating?
status:
  conditions:     # []metav1.conditions
  inputs:         # []metav1.conditions
  outputs:        # []metav1.conditions
  filters:        # []metav1.conditions
  pipelines:      # []metav1.conditions
```

Example:

```yaml
apiVersion: "observability.openshift.io/v1"
kind: ClusterLogForwarder
metadata:
  name: log-collector
  namespace: acme-logging
spec:
  outputs:
  - name: rh-loki
    type: lokiStack
    service:
      namespace: openshift-logging
      name: rh-managed-loki
      authorization:
        resource:
          name: audit-collector-sa-token
        token:
          key: token
  inputs:
  - name: infra-container
    type: infrastructure
    infrastructure:
      sources: [container]
  serviceAccount:
    name: audit-collector-sa
  pipelines:
    - inputRefs:
      - infra-container
      - audit
      outputRefs:
      - rh-loki
```

This example:

* Deploys a log collector to the `acme-logging` namespace
* Expects the administrator to have created a service account named `audit-collector-sa` in that namespace
* Expects the administrator to have created a secret named `audit-collector-sa-token` in that namespace with a key named token that is a bearer token
* Expects the administrator to have bound the roles `collect-audit-logs`, `collect-infrastructure-logs` to the service account
* Expects the administrator created a **LokiStack** CR named `rh-managed-loki` in the `openshift-logging` namespace
* Collects all audit log sources and only infrastructure container logs and writes them to the Red Hat managed lokiStack

### Topology Considerations
#### Hypershift / Hosted Control Planes
#### Standalone Clusters
#### Single-node Deployments or MicroShift


### Implementation Details/Notes/Constraints [optional]

#### Log Storage

Deployment of log storage is a separate task of the administrator.  They deploy a custom resource to be managed by the **loki-operator**.  They will additionally specify forwarding logs to this storage by defining an output in the **ClusterLogForwarder**.  Deployment of Red Hat managed log storage is optional and not a requirement for log forwarding.

#### Log Visualization

The **cluster-observability-operator** will take ownership of the management of the **console-logging-plugin** which replaces the **log-view-plugin**.  This requires feature changes to the operator and the OpenShift console before being fully realized.  Earlier version of the **cluster-logging-operator** will be updated with logic (TBD) to recognize the **cluster-observability-operator** is able to deploy the plugin and will remove its own deployment in deference to the **cluster-observability-operator**.  Deployment of log visualization is optional and not a requirement for log forwarding.

#### Log Collection and Forwarding

*observability.openshift.io/v1* of the **ClusterLogForwarder** depends upon a **ServiceAccount** to which roles must be bound that allow elevated permissions (e.g. mounting node filesystem, collecting logs).

The Red Hat managed logstore is represented by a `lokiStack` output type defined without an URL
with the following assumptions:

* Named the same as a **LokiStack** CR deployed in the `openshift-logging` namespace
* Follows the logging tenant model

The **cluster-logging-operator** will:

* Internally migrate the **ClusterLogForwarder** to craft the URL to the **LokiStack**

#### Data Model

**ClusterLogForwarder** API allows for users to spec the format of data that is forwarded to an output.  Various models are provided to allow users to embrace industry trends (e.g. OTEL)
while also offering the capability to continue with the current model.  This will allow consumers to continue to use existing tooling while offering options for transitioning to other models
when they are ready.

##### ViaQ

The ViaQ model is the original data model that has been provided since the inception of OpenShift logging. The model has not been generally publicly documented until relatively recently.  It
can be verbose and was subject to subtle change causing issues for users because of the lack of documentation.  This enhancement document intends to rectify that.

###### V1

Refer to the following reference documentation for model details:

* [Container Logs](https://github.com/openshift/cluster-logging-operator/blob/release-5.9/docs/reference/datamodels/viaq/v1.adoc#viaq-data-model-for-containers)
* [Journald Node Logs](https://github.com/openshift/cluster-logging-operator/blob/release-5.9/docs/reference/datamodels/viaq/v1.adoc#viaq-data-model-for-journald)
* [Kubernetes & OpenShift API Events](https://github.com/openshift/cluster-logging-operator/blob/release-5.9/docs/reference/datamodels/viaq/v1.adoc#viaq-data-model-for-kubernetes-api-events)

###### V2

The progression of the ViaQ data model strives to be succinct by removing fields that have been reported by customers as extraneous.

Container log:
```yaml
model_version: v2.0
timestamp:
hostname:
severity:
kubernetes:
  container_image:
  container_name:
  pod_name:
  namespace_name:
  namespace_labels:  #map[string]string: underscore, dedotted, deslashed
  labels:            #map[string]string: underscore, dedotted, deslashed
  stream:            #enum: stdout,stderr
message:             #string: optional. only preset when structured is not
structured:          #map[string]: optional. only present when message is not
openshift:
  cluster_id:
  log_type:          #enum: application, infrastructure, audit
  log_source:        #journal, ovn, etc
  sequence:          #int: atomically increasing number during the life of the collector process to be used with the timestamp
  labels:            #map[string]string: additional labels added to the record defined on a pipeline 
```

Event Log:

```yaml
model_version: v2.0
timestamp:
hostname:
event:
  uid:
  object_ref_api_group:
  object_ref_api_version:
  object_ref_name:
  object_ref_resource:
  request_received_timestamp:
  response_status_code:
  stage:
  stage_timestamp:
  user_groups: []
  user_name:
  user_uid:
  user_agent:
  verb:
openshift:
  cluster_id:
  log_type:          #audit
  log_source:        #enum: kube,openshift,ovn,auditd
  labels:            #map[string]string: additional labels added to the record defined on a pipeline 
```
Journald Log:

```yaml
  model_version: v2.0
  timestamp:
  message:
  hostname:
  systemd:
    t:                  #map
    u:                  #map
  openshift:
    cluster_id:
    log_type:          #infrastructure
    log_source:        #journald
    labels:            #map[string]string: additional labels added to the record defined on a pipeline 
```

### Risks and Mitigations

#### User Experience

The product is no longer offering a "one-click" experience for deploying a full logging stack from collection to storage.  Given we started moving away from this experience when Loki was introduced, this should be low risk.  Many customers already have their own log storage solution so they are only making use of log forwarding.  Additionally, it is intended for the **cluster-observability-operator** to recognize the existance of the internally managed log storage and automatically deploy the view plugin.  This should reduce the burden of administrators.

#### Security

The risk of forwarding logs to unauthorized destinations remains as from previous releases.  This enhancement embraces the design from 
[multi cluster log forwarding](https://github.com/openshift/enhancements/blob/master/enhancements/cluster-logging/multi-cluster-log-forwarder.md) by requiring administrators to provide a 
service account with the proper permissions.  The permission scheme relies upon RBAC offered by the platform and places the control in the hands of administrators.

### Drawbacks

The largest drawback to implementing new APIs is the product continues to identify the
availability of technologies which are deprecated and will soon not be supported.  This will
continue to confuse consumers of logging and will require documentation and explanations of our technology decisions.  Furthermore, some customers will continue to delay the move to the newer technologies provided by Red Hat.

## Open Questions [optional]

## Test Plan

* Execute all existing tests for log collection, forwarding and storage with the exeception of tests specifically intended to test deprecated features (e.g. Elasticsearch).  Functionally, other tests are still applicable
* Execute a test to verify the flow defined for collecting, storing, and visualizing logs from an on-cluster, Red Hat operator managed LokiStack
* Execute a test to verify legacy deployments of logging are no longer managed by the **cluster-logging-operator** after upgrade.

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

This release:

* Intends to support the use-cases described within this proposal
* Intends to distibute *ClusterLogForwarder.observability.openshift.io/v1* of the APIs described within this proposal
* Drop support of *ClusterLogging.logging.openshift.io/v1* API
* Deprecate support of *ClusterLogForwarder.logging.openshift.io/v1* API
* Stop any feature development to support the *ClusterLogForwarder.logging.openshift.io/v1* API
* May support multiple data models (e.g OpenTelementry, VIAQ v2)

### Removing a deprecated feature

Upon GA release of this enhancement:

- The internally managed Elastic (e.g. Elasticsearch, Kibana) offering will no longer be available.
- The Fluentd collector implementation will no longer be available
- The *ClusterLogForwarder.logging.openshift.io/v1* is deprecated and intends to be removed after two z-stream releases after GA of this enhancement.
- The *ClusterLogging.logging.openshift.io/v1* will no longer be available

## Upgrade / Downgrade Strategy

The **cluster-logging-operator** will internally convert the *ClusterLogForwarder.logging.openshift.io/v1* resources to 
*ClusterLogForwarder.observability.openshift.io/v1* and identify the original resource as deprecated.  The operator will return an error for any resource 
that is unable to be converted, for example, a forwarder that is utilizing the FluendForward output type.  Once migrated, the operator will continue to reconcile it.  Log forwarders depending upon fluentd collectors will be re-deployed with vector collectors.  Fluentd deployments forwarding to fluentforward endpoints will be unsupported.

**Note:** No new features will be added to *ClusterLogForwarder.logging.openshift.io/v1*.  

**LokiStack** is unaffected by this proposal and not managed by the **cluster-logging-operator**

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives

Given most of the changes will result in an operator that manages only log collection and forwarding, we could release a new operator for that purpose only that provides only *ClusterLogForwarder.observability.openshift.io/v1* APIs

## Infrastructure Needed [optional]

