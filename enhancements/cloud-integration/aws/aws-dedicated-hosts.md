---
title: Installing and Scaling Clusters on AWS EC2 Dedicated Hosts
authors:
  - "@faermanj"
  - "@rvanderp3"
reviewers:
  - "@nrb" # CAPA / CAPI
  - "@mtulio" # SPLAT
  - "@rvanderp3" # SPLAT
  - "@patrickdillon" # installer 
  - "@makentenza" # Product Manager
approvers: 
  - "@patrickdillon"
creation-date: 2025-05-05
last-updated: 2025-05-05
status: provisional
tracking-link: 
  - https://issues.redhat.com/browse/SPLAT-2138
see-also: {}
replaces: {}
superseded-by: {}
---

# Installing and Scaling Clusters on Dedicated Hosts in AWS

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposal outlines the work required to enable OpenShift to deploy and scale nodes onto pre-created dedicated hosts on AWS EC2. Dedicated hosts refer to the physical servers that users can allocate and assign instances to them, usually for licensing or security compliance. This leverages existing AWS infrastructure, focusing on the automated deployment process and integrating OpenShift’s node management capabilities with these hosts. The goal is to provide a simplified deployment and management experience for users who already have dedicated hosts set up.


## Motivation

This enhancement is necessary so that customers workloads that must run on dedicated hosts can be deployed to OpenShift.

### User Stories

1. As an administrator, I want to install OpenShift on a dedicated AWS host so my workloads are isolated from other tenants with identifiable physical compute resources.

2. As an administrator, I need to observe relevant conditions, events, and/or alerts to ensure I can diagnose deployments on dedicated hosts.

3. As an administrator, I want to scale my OpenShift cluster by adding or removing nodes on dedicated hosts with machine API and/or CAPI.

### Goals

- Enable OpenShift to install on pre-existing, dedicated AWS hosts.
- Enable OpenShift to scale nodes on pre-existing, dedicated AWS hosts.
- Provide meaningful conditions, events, and/or alerts for deployments on dedicated hosts.


### Non-Goals

- Automatically allocate, release or manage dedicated hosts.

## Proposal

Upstream support for dedicated hosts is being added [upstream](https://github.com/kubernetes-sigs/cluster-api-provider-aws/pull/5398). This PR will introduce support for dedicated hosts in the AWS provider. The OpenShift installer and machine management components([machine|cluster] API) will be updated to support dedicated hosts based on the upstream changes.

Changes to the OpenShift installer will include:
- Introduce fields for dedicated hosts in the [installer machinePool for AWS](https://github.com/openshift/installer/blob/main/pkg/types/aws/machinepool.go).
- Update the [manifest generation logic](https://github.com/openshift/installer/blob/main/pkg/infrastructure/aws/clusterapi/aws.go) to include dedicated hosts when generating manifests for CAPI.

Changes to the machine management components will include:
- Introduce fields for dedicated hosts in the [machine API](https://github.com/openshift/api/blob/master/machine/v1beta1/types_awsprovider.go#L12).
- Update the [machine reconciliation logic](https://github.com/openshift/machine-api-provider-aws/blob/main/pkg/actuators/machine/reconciler.go) to handle dedicated hosts when reconciling machines.

### Implementation Details/Notes/Constraints [optional]

This implementation should take pre-allocated host ids and pass it to the EC2 RunInstances API, optionally with host affinity setting.
Once that is working, we may consider adding automatic host allocation and release.

### Risks and Mitigations

Ensure that resource pruners (upstream and internal) are able to collect dedicated hosts that might leak from tests.


### API Extensions

In the upstream, fields are being introduced to add support for dedicated hosts.

```go
	// HostID specifies the Dedicated Host on which the instance should be launched.
	// +optional
	HostID *string `json:"hostId,omitempty"`

	// Affinity specifies the dedicated host affinity setting for the instance.
	// When affinity is set to Host, an instance launched onto a specific host always restarts on the same host if stopped.
	// +optional
	// +kubebuilder:validation:Enum:=Defailt;Host
	HostAffinity *string `json:"hostAffinity,omitempty"`
```

Likewise, the OpenShift API will be updated to include these fields in the [AWS machine provider spec](https://github.com/openshift/api/blob/master/machine/v1beta1/types_awsprovider.go). 

### Test Plan

Add the required dedicated-host test case to CAPA E2E suite.

### Topology Considerations

In general, this feature should be transparent to OpenShift components aside from honoring and/or propogating the dedicated host configuration
to [cluster|machine] API.

### Graduation Criteria

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- Sufficient time for feedback
- Available by default

## Infrastructure Needed [optional]

- Dedicated Hosts on EC2

