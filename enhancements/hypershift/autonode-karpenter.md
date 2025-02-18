---
title: autonode-via-karpenter
authors:
  - TBD
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# AutoNode via Karpenter

## Summary

Openshift AutoNode via by Karpenter is a feature that enables some of the data plane operational burden to be removed from the Service Consumer or the Cluster Admin Personas. It introduces first class support for heterogeneous compute that is dynamically adjusted in the most cost-effective manner within each particular cloud provider, based on the resource requirements of the workloads. The OS/Kubelet version is also owned by the Service which reserves the right to dictate data plane upgrade version and cadenece (e.g. upgrade to follow the control plane) while the Cluster Admin controls the disruption budget.


## Motivation

Heavy users of cloud seek to squeeze cost effectiveness. Additionally, present industry time with the proliferation of gen AI emphasizes the ongoing need for running heterogeneous workloads and provisioning cost effective mixed compute to allocate them.
Dynamic quota checks and computation of saving plans for cost calculation.

### User Stories

- As a Service Consumer/Cluster admin I want first class support in my cluster to express intent for single diverse pool of Nodes so I can focus on deploying my heterogeneous workloads.

- As a Service Consumer I want my dataplane to make provider native decissions for each particular Cloud Provider when running mixed instances with different capacity types, pricings and there’s quota failures so It can be as cost efficient as possible.

- As a Service Consumer I want my pool of Nodes to be tolerant to instance exhaustion so quota failures do not block node lifecycle like upgrades.

- As Service Consumer of another kubernetes based offering relying on Karpenter to allocate my workloads I want to migrate to Openshift while preserving that ability.


### Goals

- Provide a building block to enable AutoNode via Karpenter within offerings based on HCP topology

### Non-Goals

- Enable AutoNode via Karpenter for Standalone

## Proposal

The HostedCluster CRD will expose a knob for AutoNode settings.

When enabled this will install Karpenter CRDs (nodeclass, nodepool, nodeclaims) within the guest cluster.

When enabled this will cause a Karpenter instance to be lifecycled within the control plane namespace just like any other 
control plane component.

The release cadence of the karpenter image will be coupled with the OCP release cadence.
This let us align with existing SLAs for lifecycling control plane components.
Therefore we'll include the Karpenter provider image within the OCP payload. This will give us a predictable footprint for testing while having control over which features we want to expose and when.


### Workflow Description

A Service Consumer requires an Openshift cluster with autoNode

A HostedCluster API consumer e.g. ROSA set hostedCluster.autoNode... settings

A Cluster Admin can interact with Karpenter APIs within the guest cluster


### API Extensions

For a tech preview phase the feature will be exposed for consumers i.e. OCM as follows:

```
// We expose here internal configuration knobs that won't be exposed to the service.
type AutoNode struct {
  // provisioner is the implementation used for Node auto provisioning.
  // +required
  Provisioner *ProvisionerConfig `json:"provisionerConfig"`
}

// ProvisionerConfig is a enum specifying the strategy for auto managing Nodes.
type ProvisionerConfig struct {
  // name specifies the name of the provisioner to use.
  // +required
  // +kubebuilder:validation:Enum=Karpenter
  Name Provisioner `json:"name"`
  // karpenter specifies the configuration for the Karpenter provisioner.
  // +optional
  Karpenter *KarpenterConfig `json:"karpenter,omitempty"`
}

type KarpenterConfig struct {
  // platform specifies the platform-specific configuration for Karpenter.
  // +required
  Platform PlatformType `json:"platform"`
  // aws specifies the AWS-specific configuration for Karpenter.
  // +optional
  AWS *KarpenterAWSConfig `json:"aws,omitempty"`
}

type KarpenterAWSConfig struct {
  // arn specifies the ARN of the Karpenter provisioner.
  // +required
  RoleARN string `json:"roleARN"`
}
```

This API changes might be adjusted based on feedback gathered during tech-preview and it will be revisit before GA.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This proposal is HCP centric

#### Standalone Clusters

This proposal has no business use case for standalone. However the pieces of code that drive the feature in HCP are fairly isolated aiming for a potential future reusability.

#### Single-node Deployments or MicroShift

This proposal has no business use case for standalone.

### Implementation Details/Notes/Constraints

This feature is intended to be implemented in two phases.

An initial poc/tech preview phase where we expect to see the feature working e2e with OCM via ROSA as the consumer and a final GA phase.


### Risks and Mitigations


### Drawbacks


## Open Questions [optional]

## Test Plan

- Unit and e2e testing in openshift/hypershift where the core building block resides.
- Additional e2e testing driven by each consumer, e.g. ROSA.

## Graduation Criteria

This feature will be GAed only after there is tech preview ROSA environments where the feature can be exercised e2e

## Upgrade / Downgrade Strategy


## Version Skew Strategy


## Operational Aspects of API Extensions


## Support Procedures


## Alternatives


## Infrastructure Needed [optional]

