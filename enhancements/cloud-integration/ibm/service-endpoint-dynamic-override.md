---
title: service-endpoint-dynamic-override
authors:
  - jared-hayes-dev
  - cjschaef
  - jeffnowicki
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2024-11-05
last-updated: 2024-11-05
tracking-link:
  - https://issues.redhat.com/browse/OCPCLOUD-2694
see-also:
replaces:
superseded-by:
---

## Summary

IBM Cloud wishes to support overridng service endpoints for components post cluster creation. Currently you may define overrides [prior to creating the cluster](https://github.com/openshift/installer/blob/c0938914effb0f416d01f250ea021de0cea0d690/pkg/asset/manifests/ibmcloud/cloudproviderconfig.go#L80), but the process for updating the endpoints after the creation of a cluster is not officially supported. The desire is to allow a user to configure the infrastructure object spec for IBM Cloud to specify with a list of services and endpoints to override which will be dynamically updated and reflected in all dependent components.

## Motivation

Management of clusters neccesitates that users be able to update endpoints should requirements/upstream services change and IBM wishes to fully support this with an official path.

### User Stories

* As an Openshift cluster administrator, I want to update the service endpoints for my cluster so that I can utilize the new private IAM endpoint.
 
### Goals

* Provide an official path for IBM deployed clusters to update the infrastructure object with service endpoint override(s) that will propagate to all dependent components without further user intervention.

### Non-Goals



## Proposal

To realize this enhancement:


* Expand API definition to support defining services + endpoints within cloud provider spec for IBM
* Modify CCCMO so that changes are reconciled from infrastructure spec to status for IBM cloud provider and cloud config
* Update components CSI driver, ingress operator, MAPI, to pick up these changes and utilize new endpoints once they are set.

### Workflow Description

**cluster administrator** is a human user responsible managing an existing openshift custer deployed on IBM infrastructure.

1. The cluster administrator wishes to use private IBM Cloud endpoints
2. The cluster administrator identifies the services that they wish to update (ie IAM and resource controller) and identifies the endpoints for these services
3. The cluster administrator updates the infrastructure object to contain a list of overrides where each element is the name of the service and the endpoint to use for that service.
4. After a delay the cluster administrator observes this change in all dependent components.

**cccmo** is an operator responsible for watching updates to the infrastructure object and perforning updates once any value(s) are set. 

1. The cccmo reconciliation loop observes that the IBMCloudPlatform spec within the infrastructure object has been set,
2. The cccmo validates the endpoints and then updates the IBMCloudPlatformStatus and cloud config.


### API Extensions

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.


* API will be updated such that [IBMCloudPlatformSpec](https://github.com/openshift/api/blob/4c27e61e5554ea8506947d019770e5a04c3c4a36/config/v1/types_infrastructure.go#L1522) will have a field for `IBMCloudServiceEndpoints` similar to the existing field in [IBMCloudPlatformSpec](https://github.com/openshift/api/blob/4c27e61e5554ea8506947d019770e5a04c3c4a36/config/v1/types_infrastructure.go#L1549)
* CCCMO will be updated so that config sync controller via the IBM `CloudConfigTransformer` reads in endpoint settings within the spec of the infrastructure object and updates the corresponding infrastructure status and cloud config to reflect those set values.

### Risks and Mitigations

Users may cause service interruptions for their cluster should they define invalid overrides. This is mitigated by performing validation on the endpoint as there requirements are understood at time of implementation for IBM Cloud.

### Drawbacks

This change requires the cccmo manage and update the infrastructure object which is a new behavior for this operator. 

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