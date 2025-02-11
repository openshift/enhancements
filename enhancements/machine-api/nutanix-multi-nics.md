---
title: nutanix-multi-nics
authors:
  - "@yanhua121"
reviewers:
  - "@JoelSpeed"
  - "@patrickdillon"
approvers:
  - "@JoelSpeed"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-11-05
last-updated: 2024-11-08
tracking-link:
  - https://issues.redhat.com/browse/CORS-3741
---

# Nutanix: Multi-NICs for OCP Cluster Nodes

## Summary

Ability to install OpenShift on Nutanix with nodes having multiple NICs (multiple subnets) from IPI and for autoscaling with MachineSets.

## Motivation

Requested by customers:
- Everest Digital
- Unacle B.V

### Goals

- Allow users to configure multiple subnets for Nutanix pltform in the install-config.yaml file at cluster installation using IPI or UPI.
- Allow users to configure multiple subnets via Machine/MachineSet CRs' Nutanix providerSpec to add/scale worker nodes.
- Allow smooth cluster upgrade from older OCP versions. 

### Non-Goals

## Proposal

### User Stories

As an OpenShift user, I wish to deploy clusters that allow infrastructure and worker nodes with multi-NICs support. This may be to support secondary storage networking, such as Nutanix CSI, or to support other applications with segmented network requirements.

### API Extensions

Currently, the “subnets” fields in both Machine/MachineSet’s Nutanix providerSpec and Nutanix FailureDomain are already array type. The only change for the api is to relax the validation rule for the “subnets” fields to allow multiple values and to ensure no duplication values are configured.

We will add a featue gate "NutanixMultiSubnets" (DevPreviewNoUpgrade, TechPreviewNoUpgrade) for this feature. After QE testing completes, we will add the feature gate to the "Default" feature set.

```go
// NutanixPlatformSpec holds the desired state of the Nutanix infrastructure provider.
// This only includes fields that can be modified in the cluster.
type NutanixPlatformSpec struct {
  ...

  // failureDomains configures failure domains information for the Nutanix platform.
  // When set, the failure domains defined here may be used to spread Machines across
  // prism element clusters to improve fault tolerance of the cluster.
  // +openshift:validation:FeatureGateAwareMaxItems:featureGate=NutanixMultiSubnets,maxItems=32
  // +listType=map
  // +listMapKey=name
  // +optional
  FailureDomains []NutanixFailureDomain `json:"failureDomains"`
}

// NutanixFailureDomain configures failure domain information for the Nutanix platform.
type NutanixFailureDomain struct {
  ...

  // subnets holds a list of identifiers (one or more) of the cluster's network subnets
  // If the feature gate NutanixMultiSubnets is enabled, up to 32 subnets may be configured.
  // for the Machine's VM to connect to. The subnet identifiers (uuid or name) can be
  // obtained from the Prism Central console or using the prism_central API.
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:MinItems=1
  // +openshift:validation:FeatureGateAwareMaxItems:featureGate="",maxItems=1
  // +openshift:validation:FeatureGateAwareMaxItems:featureGate=NutanixMultiSubnets,maxItems=32
  // +openshift:validation:FeatureGateAwareXValidation:featureGate=NutanixMultiSubnets,rule="self.all(x, self.exists_one(y, x == y))",message="each subnet must be unique"
  // +listType=atomic
  Subnets []NutanixResourceIdentifier `json:"subnets"`
}
```

### Implementation Details/Notes/Constraints

The installer should allow more than one subnets to be configured in the install-config.yaml. And pass that configuration to the installer generated Machine/MachineSet manifests and the corresponding Nutanix-CAPI manifests when running the installer to create an OCP cluster. Note that the Nutanix-CAPI already supports multi-subnets. Since in the OCP installer's current code base, the mutli-subnets as array type is already handled properly when generating the nutanix-capi manifests and also the MAPI manifests. This time, the installer change is only to relax the validation (previously limit to only one subnet) to allow more than one (up to maximum of 32) subnets and no duplicates to configure. 

The Machine validation webhook should check the Nutanix providerSpec’s “subnets” field to allow more than one item and make sure there are no duplicates.
The nutanix machine controller should allow more than one item in the NutanixMachineProviderConfig’s “subnets” field, and use this configured subnets value when creating a new VM for the Machine node. 

### Workflow Description

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Risks and Mitigations

### Drawbacks

## Test Plan

- QE will test this feature
  - IPI installation, configure more than one valid items for platform.nutanix.subnetUUIDs, and for platform.nutanix.failureDomains.subnetUUIDs in the install-config.yaml to create an Nutanix OCP cluster. Also verify that the subnetUUID values must unique, and the maximum number of items cannot exceed 32. 
  - After cluster is created, deploy a new Machine or MachineSet by configuring its spec.providerSpec.subnets with more than one items. And verify that each subnets value must be unique, and the maximum number of items cannot exceed 32. Also test the same by modify the Infrastruecture CR with its field platformSpec.nutanix.failureDomains.subnets.
  - Test the newly added  max limit of platformSpec.nutanix.failureDomains items (maximum=32) of the Infrastructure CRD.

- Will add an e2e test case for this feature
Add a nutanix-e2e test case by configuring two subnets in the install-config.yaml, and after verify that the subnets are available in each of the cluster nodes.

## Graduation Criteria

The Test Plan completes.

### Dev Preview -> Tech Preview

### Tech Preview -> GA

After QE test is done. We can add the feature gate "NutanixMultiSubnets" to the Default feature set, for the 4.18 GA.

### Removing a deprecated feature

## Upgrade / Downgrade Strategy 

To upgrade an existing OCP (prior to 4.18) Nutanix cluster to 4.18, there is nothing to worry about this feature. Because prior to 4.18, the “subnets” field of the Nutanix providerSpec in the Machine/MachineSet/ControlPlaneMachineSet CRs and in the each of Nutanix FailureDomains of the Infrastructure CR should only have one and exactly one item. And this is supported in 4.18.

A new limit is imposed, limiting the failure domains within the nutanix platform spec to 32 items. Any cluster that has more than 32 failure domains will be considered unsupported and will need to reduce their failure domain count to 32 or fewer before they can update their failure domain configuration.

To downgrade an existing 4.18 OCP Nutanix cluster to a prior version, if any of the Machine/MachineSet/ControlPlaneMachineSet CRs and the Nutanix FailureDomains of the Infrastructure CR configures more than one “subnets”, it will fail with validation errors.

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives

