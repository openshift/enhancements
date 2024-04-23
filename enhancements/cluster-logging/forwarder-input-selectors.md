---
title: forwarder-input-selectors
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
last-updated: 2024-03-07
tracking-link:
- https://issues.redhat.com/browse/LOG-2155
see-also:
-
replaces:
-
---


# Log Forwarding Input Slection using Kubernetes Metadata
## Summary


Cluster Logging defines a set of well known log sources in order to facilitate configuration of log collection and normalization.  Given customers are no longer bound to the data storage provided by cluster logging, this enhancement expands those definitions to allow specifying which logs are collected by using Kubernetes metadata.


Logs originate from six distinct sources and are logically grouped using the following definitions:


* **Application** are container logs from all namespaces across the cluster excluding infrastructure namespaces.  


* **Infrastructure** logs are:
  * container logs from namespaces: default, kube*, openshift*


* **Audit** are logs written to files on master nodes that include:
  * kubernetes API server
  * OpenShift API server
  * auditd
  * OVN


**NOTE**: **application**, **infrastructure**, and **audit** are reserved words to the **cluster-logging-operator** and continue to represent the previous definitions.


Administrators use these definitions to specify pipelines to normalize and route messages from the sources to outputs.


This enhancement allow administrators to define "named" inputs by expanding the previous definitions as follows:


* Named application:
  * Any name that is not reserved
  * Collect from any namespace including the ones for **infrastructure** container logs
* Named infrastructure:
  * Any name that is not reserved
  * Explicit source choices of: node, container
* Named audit:
  * Any name that is not reserved
  * Explicit source choices of: kubeAPI, openshiftAPI, auditd, ovn




## Motivation


### User Stories




* As an administrator of cluster logging, I want to only forward logs from a limited set of namespaces because I do not need the others
* As an administrator of cluster logging, I want to exclude logs from a limited set of namespaces because I do not need them
* As an administrator of cluster logging, I want to only forward logs from pods with a specific set of labels
* As an administrator of cluster logging, I want to exclude certain container logs from a pod because they are noisy and uninteresting to me
* As an administrator of cluster logging, I do not want to collect node logs because they are not of interest to me


### Goals


* Allow specifying which container logs are or are not collected using workload metadata (e.g. namespace, labels, container name)
* Allow specifying which source of infrastructure (i.e. node, container) or audit (i.e. kubernetes API, openshift API, auditd, ovn) logs are collected
* Reduce the CPU and memory load on the collector by configuring it to only process logs that are interesting to administrators
* Reduce the network usage when forwarding logs
* Reduce the resources required to store logs (e.g. size, cpu, memory)
* Reduce the cost to store logs


### Non-Goals


* Introduction of the next version of logging APIs.
* Allow administrators full access to the native collector configuration.


## Proposal


### Workflow Description


Administrators create an instance of **ClusterLogForwarder** which defines which logs to collect, how they are normalized, and where they are forwarded.  They can choose to explicitly collect logs from specific namespaces or from pods which have specific labels by defining a "named" input.  No other changes to the existing workflow are required.


### API Extensions


#### ClusterLogForwarder


Following are the additions to the InputSpec:

* Application Input
```yaml
    spec:
    - name: my-app
      application:
        namespaces: []           #deprecated: exact string or glob
        includes:
        - container:             #exact string or glob
          namespace:             #exact string or glob
        excludes:
        - container:             #exact string or glob
          namespace:             #exact string or glob
        selector:                #metav1.LabelSelector
          matchLabels: []
          matchExpressions:
          - key:
            operator:
            values: []
```

**NOTE:** *application.namespaces* field is deprecated.

```golang
   type Application struct {
     Namespaces        []string
     Includes          *NamespaceContainerGlob
     Excludes          *NamespaceContainerGlob
     Selector          *metav1.LabelSelector
   }


   type NamespaceContainerGlob struct {
     Namespace string
     Container string
   }
```

* Infrastructure Input
```yaml
    spec:
    - name: my-infra
      infrastructure:
        sources: ["node","container"]

```
```golang
   type Infrastructure struct {
     Sources     []string
   }

   const (
     InfrastructureSourceNode string      = "node"
     InfrastructureSourceContainer string = "container"
   )
```

* Audit Input
```yaml
    spec:
    - name: my-audit
      audit:
        sources: ["kubeAPI","openshiftAPI","auditd","ovn"]
```
```golang
   type Audit struct {
     Sources     []string
   }

   const (
     AuditSourceKube string      = "kubeAPI"
     AuditSourceOpenShift string = "openShiftAPI"
     AuditSourceAuditd string    = "auditd"
     AuditSourceOVN string       = "ovn"
   )
```

##### Verification and Validations 
The operator will validate resources upon reconciliation of a **ClusterLogForwarder**.  Failure to meet any of the following conditions will stop the operator from deploying a collector and it will add error status to the resource or be rejected before admission:


* The **ClusterLogForwarder** CR defines a valid spec
* Input spec fields that are "globs" (i.e. Namespace, container) match RE: '`^[a-zA-Z0-9\*]*$`'
* Input field 'selector' is a valid metav1.LabelSelector 
* Input enum fields accept only the values listed
* type "infrastructure" sources specs at least one value
* type "audit" sources specs at least one value


##### Examples
Following is an example of a **ClusterLogForwarder** that redefines "infrastructure" logs to include node logs and other namespaces outside of "openshift*" while dropping all istio container logs from any namespace:

```yaml
    apiVersion: "logging.openshift.io/v1"
    kind: ClusterLogForwarder
    metadata:
      name: infra-logs
      namespace: mycluster-infras
    spec:
      serviceAccountName: audit-collector-sa
      inputs:
      - name: my-infra-container-logs
        application:
          namespaces:
          - openshift*
          includes:
          - namespace: mycompany-infra*
          excludes:
          - container: istio*
      - name: my-node-logs
        infrastructure:
          sources: ["node"]
      pipelines:
       - inputRefs:
         - my-infra-container-logs
         - my-node-logs
         outputRefs:
         - default
```

### Implementation Details/Notes/Constraints


* The collector configuration will be restructured to dedicate a source for  each **ClusterLogForwarder** input


### Risks and Mitigations


* Are we able to provide enough test coverage to ensure we cover all the ways the configuration may change with this expanded offering


### Drawbacks


## Design Details


### Open Questions [optional]


### Test Plan


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
