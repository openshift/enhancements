---
title: forward_to_azure_monitor_logs
authors:
  - "@vparfonov"
reviewers:
  - "@jcantrill"
  - "@alanconway"
approvers:
  - "@jcantrill"
  - "@alanconway"
api-approvers:
  - "@jcantrill"
  - "@alanconway"
creation-date: 2023-11-17
last-updated: 2023-11-17
status: implementable
tracking-link: 
  - https://issues.redhat.com/browse/LOG-4606
see-also:
  -
superseded-by:
---

# Forward to Azure Monitor Logs

## Release Sign-off Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary
[Azure Monitor Logs](https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-platform-logs)
Azure Monitor Logs is a comprehensive service provided by Microsoft Azure that enables the collection, analysis, and 
actioning of telemetry data across various Azure and on-premises resources. It facilitates advanced analytics,
offering profound insights into the performance and health of applications and workloads running on Azure.
This proposal extends the `ClusterLogForwarder` API with an output type for Azure Monitor Logs.


## Motivation
In this update, we aim to enhance our product's capabilities by enabling log forwarding to Azure Monitor Logs. 
By supporting this functionality, our users will be able to seamlessly integrate their application logs 
with Azure Monitor Logs, leveraging its powerful analytics and monitoring capabilities. This integration will empower
users to streamline their monitoring and troubleshooting processes, leading to better operational insights, 
faster issue resolution, and improved overall performance of their Azure-based applications and services.

### User Stories
As a product manager, I want to be able to forward logs from my OpenShift cluster to Azure Monitor Logs 
so that I can create dashboards and reports to track the performance and reliability of my applications and services.

As an administrator, I want to be able to forward logs from my OpenShift cluster to Azure Monitor Logs so that I can 
create alerts for security events or other critical incidents.

As a developer, I want to be able to forward logs from my OpenShift cluster to Azure Monitor Logs so that I can use
Azure Log Analytics to search and filter logs, and to create custom visualizations of my log data.

### Goals

* Enable log forwarding to Azure Monitor Logs via HTTP Data Collector API.
* Investigate enabling to support STS workflow when forwarding to Azure and if their additional changes required to Vector.

## Proposal
For enabling forwarding logs to the Azure Monitor Logs we need to: 
- register new Output Type in our API
- provide ability to generate valid configuration for collector according to user requirements
- release new Vector image with enabled "sinks-azure_monitor_logs" feature

### Proposed API

Add a new `azureMonitor` output type to the `ClusterLogForwarder` API:

```Go
type AzureMonitor struct {
  //CustomerId che unique identifier for the Log Analytics workspace.
  //https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-collector-api?tabs=powershell#request-uri-parameters
  CustomerId string `json:"customerId,omitempty"`
  
  //LogType The record type of the data being submitted. This field is distinct from the "OpenShift Logging LogType" 
  //and serves to provide context within the scope of the customer ID. It can only contain letters, numbers, and 
  //underscores (_), and must not exceed 100 characters.
  //https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-collector-api?tabs=powershell#request-headers
  LogType string `json:"logType,omitempty"`
  
  //AzureResourceId the Resource ID of the Azure resource the data should be associated with.
  //https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-collector-api?tabs=powershell#request-headers
  // +optional
  AzureResourceId string `json:"azureResourceId,omitempty"`
   
  //Host alternative host for dedicated Azure regions. (for example for China region)
  //https://docs.azure.cn/en-us/articles/guidance/developerdifferences#check-endpoints-in-azure
  // +optional
  Host string `json:"host,omitempty"`
}
```
Existing fields:

- `url`: Not used.
- `secret`: Azure credentials, the secret in `shared_key` field must contain keys primary or the secondary key 
   for the workspace that's making the request ([data-collector-api#authorization](https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-collector-api?tabs=powershell#authorization)).


### Workflow Description

#### Create secret with shared key:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mysecret
  namespace: openshift-logging
type: Opaque
data:
  shared_key: "my_shared_key"
```

#### I want to forward logs to Azure Monitor Logs instead of a local store
```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
spec:
  outputs:
  - name: azureLogAnalytics
    type: azureMonitor
    azureMonitor:
      customerId: "myCustomerID"
      logType: "MyLogType"
    secret:
       name: mysecret
  pipelines:
  - inputRefs: [application, infrastructure, audit]
    outputRefs: [azureLogAnalytics]
```

#### I want to forward logs to Azure Monitor Logs and associate the logs with some Azure resource
```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
spec:
  outputs:
  - name: azureLogAnalytics
    type: azureMonitor
    azureMonitor:
      customerId: "myCustomerID"
      logType: "MyLogType"
      azureResourceId: "/subscriptions/11111111-1111-1111-1111-111111111111/resourceGroups/otherResourceGroup/examplestorage"
    secret:
       name: mysecret
  pipelines:
  - inputRefs: [application, infrastructure, audit]
    outputRefs: [azureLogAnalytics]
```

#### I want to forward logs to Azure Monitor Logs in alternative host for dedicated Azure regions.
```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
spec:
  outputs:
  - name: azureLogAnalytics
    type: azureMonitor
    azureMonitor:
      customerId: "myCustomerID"
      logType: "MyLogType"
      host: "ods.opinsights.azure.acme"
    secret:
       name: mysecret
  pipelines:
  - inputRefs: [application, infrastructure, audit]
    outputRefs: [azureLogAnalytics]
```

#### I want to forward application and infrastructure logs to Azure Monitor Logs with dedicated log types (application and infrastructure)
```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
spec:
  outputs:
  
  - name: azureLogAnalyticsApp
    type: azureMonitor
    azureMonitor:
      customerId: "myCustomerID"
      logType: "application"
    secret:
      name: mysecret
 
  - name: azureLogAnalyticsInfra
    type: azureMonitor
    azureMonitor:
      customerId: "myCustomerID"
      logType: "infrastructure"
    secret:
      name: mysecret
    
  pipelines:
    - name: app-pipeline
      inputRefs: application
      outputRefs:
        - azureLogAnalyticsApp
    - name: infra-pipeline
      inputRefs: infrastructure
      outputRefs:
        - azureLogAnalyticsInfra
```    

### Implementation Details [optional]
Use the [Vector plugin](https://vector.dev/docs/reference/configuration/sinks/azure_monitor_logs) to publish log events
to the Azure Monitor Logs service
Plugin configuration settings:

The customer_id, log_type fields are required, while the azure_resource_id and host field is optional.

* customer_id : The unique identifier for the Log Analytics workspace.
* log_type : Specify the record type of the data that's being submitted. It can contain only letters, numbers, 
  and the underscore (_) character, and it can't exceed 100 characters. Each request to the Data Collector API 
  must include a LogType with the name for the record type. The suffix _CL is automatically appended to the name 
  you enter to distinguish it from other log types as a custom log. 
  For example, if you enter the name MyNewRecordType, Azure Monitor Logs creates a record with the type MyNewRecordType_CL. 
  This helps ensure that there are no conflicts between user-created type names and those shipped in current or future Microsoft solutions.
* azure_resource_id : The resource ID of the Azure resource that the data should be associated with. It populates the
  _ResourceId property and allows the data to be included in resource-context queries. If this field isn't specified, 
  the data won't be included in resource-context queries.
* host: Azure China differs from Azure global, so service endpoints must be changed. Default: ods.opinsights.azure.com


### Open Questions [optional]
1. How to test? Need to try https://mockoon.com/mock-samples/azurecom-operationalinsights-operationalinsights/#!
2. Investigate enabling to support STS workflow when forwarding to Azure and if their additional changes required to Vector.
   The Azure HTTP Data Collector API doesn't officially document STS token support. 

### Test Plan
- E2E tests: Need access to Azure accounts.
- Functional tests: using mock similar to Azure Log Analytics with https://mockoon.com/mock-samples/azurecom-operationalinsights-operationalinsights/#!

### Risks and Mitigations"
Vector plugin use deprecated HTTP Data Collector API to send log data to Azure Monitor Logs. 

Note:
The Azure Monitor Logs HTTP Data Collector API has been deprecated and will no longer be functional as of 9/14/2026. It's been replaced by the Logs ingestion API.
Vector not support Log ingestion API, yet.
https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-collector-api?tabs=powershell

### Drawbacks

## Design Details

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

### Non-Goals"

### API Extensions"
