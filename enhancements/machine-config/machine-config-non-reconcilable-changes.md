---
title: machine-config-non-reconcilable-changes
authors:
  - "@pablintino"
reviewers:
  - "@yuqi-zhang"
approvers:
  - "@yuqi-zhang"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-04-23
tracking-link:
  - https://issues.redhat.com/browse/MCO-1002
see-also:
replaces:
superseded-by:
---

# MachineConfig Non-Reconcilable Changes

## Summary

This enhancement describes the context around the MCO validation of 
MachineConfigs and why it will need to be partially skipped under certain, 
specific, circumstances. 

## Motivation

OCP OS configuration is driven by Ignition, that performs a one-shot 
configuration of the OS based on the Ignition spec of the MCO Pool each node
belong to. Once the user configure install-time parameters, the MCO will 
prevent any further changes to non-reconcilable fields. While this is 
generally useful for safety, it becomes problematic for any users who wishes 
to change install-time only parameters, such as disk partition schema. 
In the worst case, this would prevent scaleup of new nodes with any 
differences incompatible with existing MachineConfigs.

For these users, their only real option today would be to re-provision their 
cluster with new install-time configuration, which is costly and time 
consuming. We would like to introduce the ability for users in these scenarios 
to instruct the MCO to allow for unreconcilable MachineConfig changes to be 
applied, skipping existing nodes, and serve this configuration to new nodes 
joining the cluster. Invalid ignition is not considered in this case.

### User Stories

* As a cluster admin, I am adding new nodes to a long-standing cluster and I 
would like change the partitions schema for the new nodes.
* As a cluster admin, I am adding new nodes to a long-standing cluster and the 
new hardware has a different set of disks that requires different disks and 
filesystems sections.

### Goals

* Allow users to provide MCs with non-reconcilable fields for the use cases 
in which preserving the original install-time parameters values is not 
possible.

### Non-Goals

* Allow invalid Ignition/MachineConfig fields to be applied.
* Disable non-reconcilable MCs validation by default.

## Proposal

Update the MachineConfiguration CR by adding a new field to the spec that 
allows users to bypass validation for irreconcilable MachineConfig changes. 
The field will default to the current behavior that is to validate all 
rendered MCs.

The MachineConfig Controller and the MachingConfig MachineConfigDaemons will 
read in runtime the new field and if the value explicitly states that the 
validation should be skipped they will let the MachineConfig pass and get 
applied to the nodes.

MachineConfig daemons will continue to perform the already supported updates 
to nodes, no matter if the non-reconcilable validation is skipped or not.
Already existing nodes that receives only non-supported changes will skip the 
update and will be considered updated.

### Workflow Description

Each time a MachineConfig is changed a new rendered MachineConfig is created
for each pool associated to the changed MachineConfig.

Before the freshly created rendered MachineConfig passes to the 
MachineConfigDaemons the MCO performs an internal validation of it that can 
can be divided into three phases:

1. Parse the Ignition raw configuration.
2. Ensure the Ignition configuration is valid.
3. Ensure there are no changes to non-reconcilable fields.

The first two steps are self-explanatory and are not covered by this 
enhancement, as the MCO will always perform them. The third one, the 
validation of non-reconcilable fields, is the main target of this enhancement.

After the Ignition validation is done, the non-reconcilable fields validation 
is performed or skipped based on the proposed
`machineConfigurationValidationPolicy` field in the MachineConfiguration CR.
If the field is set to `Relaxed` the non-reconcilable fields validation is 
skipped, otherwise is performed.

The non-reconcilable MachineConfig validation remains as it is with this 
enhancement, as the [implementation](https://github.com/openshift/machine-config-operator/blob/e44d380686aee42f784a277236dbac49b083441e/pkg/controller/common/reconcile.go#L69) 
does not change with this enhancement.

After the validation checks are done the nodes of the MCP are updated to point 
to the new rendered MachineConfig as the desired one and the MCD starts to 
apply the requested changes.

The MCD only applies changes to the supported fields, any change to the 
MC out of supported ones is ignored.

After all the changes are applied, the MCD updates the Node annotations to 
point `machineconfiguration.openshift.io/currentConfig` to the updated one and 
`machineconfiguration.openshift.io/state` to `Done` if the update succeeds. 

### API Extensions

- Update the MachineConfiguration CRD to add an enumeration field, called 
`machineConfigurationValidationPolicy` that is used as the validation 
skipping toggle. The field does not set a default values to let the MCO pick 
what to do in the default case. The enumeration has only two values:
    - Strict: Validation is always performed. This is the value the MCO will
    use as default.
    - Relaxed: The validation of non reconcilable fields is skipped and only 
    the Ignition syntactic validation will be done. 

### Risks and Mitigations

By setting `machineConfigurationValidationPolicy` to `Relaxed` the customer 
acknowledges that providing MCs that make use of Ignition features out of the 
scope of the MCO will lead to cluster with nodes using different Ignition 
configurations.

### Drawbacks

None.

## Design Details

### Open Questions [optional]

None.

### Test Plan

MCO e2e tests and unit tests will cover this functionality.

### Graduation Criteria

This feature is behind the tech-preview FeatureGate in 4.20. 
Once it is tested by QE and users it can be GA'd since it should not impact 
daily usage of a cluster.

## Dev Preview -> Tech Preview

Not applicable. Feature introduced in Tech Preview. 

## Tech Preview -> GA

Bugs found by e2e tests and QE are .

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Upgrades or downgrades are not impacted by the presence or not of this feature.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

#### Failure Modes

If the non-reconcilable configuration validation is performed and it fails 
the MCO continues to report the failure as it is alraedy doing in the MCP, by
setting to the MCP the `RenderDegraded` condition to true.

If the configuration reaches the MCD and the non-reconcilable validation 
fails the MCN `UpdatePrepared` condition is updated with the details of the 
validation failure.

#### Support Procedures

None.

## Implementation History

Not applicable.

## Alternatives (Not Implemented)

Not applicable.
