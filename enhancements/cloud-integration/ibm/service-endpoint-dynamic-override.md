---
title: service-endpoint-dynamic-override
authors:
  - jared-hayes-dev
  - cjschaef
  - jeffnowicki
reviewers:
  - JoelSpeed
  - elmiko
approvers:
  - JoelSpeed
  - elmiko
api-approvers:
  - JoelSpeed
  - elmiko
creation-date: 2024-11-05
last-updated: 2024-11-05
tracking-link:
  - https://issues.redhat.com/browse/OCPCLOUD-2694
see-also:
replaces:
superseded-by:
---

# IBM Cloud Service Endpoint Override Support

## Summary

In controlled deployments in restricted network environments, support for configuring service endpoints is required. With IBM Cloud, [support](https://docs.openshift.com/container-platform/4.17/installing/installing_ibm_cloud/installing-ibm-cloud-restricted.html#access-to-ibm-service-endpoints_installing-ibm-cloud-restricted) has already been provided to specify desired service endpoints at install time. It is also desirable to be able to change the service endpoint configuration, post install. This enhancement will extend existing support and allow post install changes to be made to the service endpoint configuration.

## Motivation

IBM Cloud requires this enhancement for their control plane replatforming efforts. OpenShift IPI for IBM Cloud will be used to deploy a cluster with critical responsibility in our managed control plane. During the genesis phase of region bringup, existing service endpoints will be used while new regional service endpoints are brought up. Once the new regional service endpoints are available, the aforementioned cluster's service endpoint configuration will need to be updated.

### User Stories

* As an OpenShift cluster administrator, I want to update my cluster's current service endpoint configuration to point to new service endpoints so that I can comply with administrative requirements to directs component traffic through regional or private endpoints.
 
### Goals

* Provide an official path for IBM deployed clusters to update the infrastructure object with service endpoint override(s) that will propagate to all dependent components without further user intervention.

### Non-Goals
n/a


## Proposal

To realize this enhancement:

* Expand API definition to support defining services + endpoints within cloud provider spec for IBM
* Modify CCCMO so that changes are reconciled from infrastructure spec to status for IBM cloud provider and cloud config

### Workflow Description

**cluster administrator** is a human user responsible managing an existing openshift custer deployed on IBM infrastructure.

1. The cluster administrator wishes to use private IBM Cloud endpoints.
2. The cluster administrator identifies the services that they wish to update (ie IAM and resource controller) and identifies the endpoints for these services.
3. The cluster administrator updates the infrastructure object to contain a list of overrides where each element is the name of the service and the endpoint to use for that service. `oc edit infrastructure  -n default cluster`
4. Once the service endpoint override update has been processed/reconciled, components can act on the change (if applicable) and use in future operations (note: may need to be restarted to pick up the change).

**cccmo** is an operator responsible for watching updates to the infrastructure object and perforning updates once any value(s) are set. 

1. A user edits the IBMCloudPlatform spec withthin the infrastructure object with a list endpoint overrides. At admission time, basic validation of the provided URLs is performed including verifying the URL is valid, follows the IBM URL path spec, and is directing traffic using https.
2. The cccmo reconciliation loop observes that the IBMCloudPlatform spec within the infrastructure object has been set,
3. The cccmo validates the host can be reached and then updates the IBMCloudPlatformStatus and cloud config.


### API Extensions

* Extend `IBMCloudPlatformSpec` to contain service endpoint field that users may define as desired overrides.
* Given changes to the spec, the CCCMO reconciliation loop will read in the infrastructure spec, pick up the changes, and write forward the defined endpoint overrides to the `IBMCloudPlatformStatus` and cloud config. These changes are consumed by downstream resources such as the ingress operator for updating their behavior when making calls to IBM services.

```
type IBMCloudPlatformSpec struct {
	// serviceEndpoints is a list of custom endpoints which will override the default
	// service endpoints of an IBM Cloud service. These endpoints are consumed by
	// components within the cluster to reach the respective IBM Cloud Services.
	// +listType=map
	// +listMapKey=name
	// +optional
	ServiceEndpoints []IBMCloudServiceEndpoint `json:"serviceEndpoints,omitempty"`
}
```



### Topology Considerations

### Cluster wide proxy
Should a customer choose to enable the cluster wide proxy while using this feature, to ensure traffic is properly handled for the IBM service endpoints they will need to exclude these overrides from the proxy. Currently by default the proxy does not exclude IBM traffic so this will need to be configured on the customer's end when enabling the proxy by adding the relevant endpoints to the `noProxy` field.

#### Hypershift / Hosted Control Planes

n/a

#### Standalone Clusters

n/a

#### Single-node Deployments or MicroShift

n/a

### Implementation Details/Notes/Constraints

* API will be updated such that [IBMCloudPlatformSpec](https://github.com/openshift/api/blob/4c27e61e5554ea8506947d019770e5a04c3c4a36/config/v1/types_infrastructure.go#L1522) will have a field for `IBMCloudServiceEndpoints` similar to the existing field in [IBMCloudPlatformSpec](https://github.com/openshift/api/blob/4c27e61e5554ea8506947d019770e5a04c3c4a36/config/v1/types_infrastructure.go#L1549)
* CCCMO will be updated so that config sync controller via the IBM `CloudConfigTransformer` reads in endpoint settings within the spec of the infrastructure object and updates the corresponding infrastructure status and cloud config to reflect those set values.

### Risks and Mitigations

Users may cause service interruptions for their cluster should they define invalid overrides. This is mitigated by performing validation on the endpoint as there requirements are understood at time of implementation for IBM Cloud. 


### Drawbacks

n/a

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives