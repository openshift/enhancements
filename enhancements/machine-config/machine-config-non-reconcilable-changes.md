---
title: machine-config-irreconcilable-changes
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

# MachineConfig Irreconcilable Changes

## Summary

This enhancement provides context for the MCO's MachineConfig validation 
process and explains why it will need to be selectively bypassed under 
specific circumstances.

## Motivation

OCP OS configuration is driven by Ignition, which performs a one-shot 
configuration of the OS based on the Ignition specification associated with 
the MachineConfigPool (MCP) that each node belongs to.
Once the user configure install-time parameters with the initial 
Ignition specification used at cluster install-time, the MCO will prevent any 
further changes to fields MCDs do not support, known as irreconcilable fields, 
locking the user into an Ignition specification for the rest of the life of 
the cluster.

The exhaustive list of fields the MCD supports to be changed after the cluster 
is built can be found in the ["What can you change with machine configs?"](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/machine_configuration/machine-config-index#what-can-you-change-with-machine-configs) 
section of the official documentation.

While this is generally useful for safety, it becomes problematic for any 
users who wishes to change install-time only parameters, such as disk 
partition schema. In the worst case, this would prevent scale up of new nodes 
with any differences incompatible with existing MachineConfigs.

For these users, their only real option today would be to re-provision their 
cluster with new install-time configuration, which is costly and time 
consuming. We would like to introduce the ability for users in these scenarios 
to instruct the MCO to allow for irreconcilable MachineConfig changes to be 
applied, skipping existing nodes, and serve this configuration to new nodes 
joining the cluster. This proposal does not address or permit invalid Ignition 
configurations.

### User Stories

* As a cluster admin, I am adding new nodes to a long-standing cluster and I 
would like change the partitions schema for the new nodes.
* As a cluster admin, I am adding new nodes to a long-standing cluster and the 
new hardware has a different set of disks that requires different disks and 
filesystems sections.
* As a cluster admin, I am adding new nodes to a long-standing cluster and the 
new setup requires a dedicated new user.
* As a cluster admin, I am adding new nodes to a long-standing cluster and the 
new setup requires symlinks.

### Goals

* Allow users to provide MachineConfigs (MCs) with irreconcilable fields for 
use cases where preserving original install-time parameter values is not 
feasible.

### Non-Goals

* Allow invalid Ignition/MachineConfig fields to be applied.
* Disable irreconcilable MCs validation by default.

## Proposal

To enable users to consume a different Ignition file than the one used at 
cluster install-time, the MCO will allow changes to MachineConfigs that 
existing nodes can ignore, but new joining nodes can consume.

To achieve that, the enhancement adds a new field to the 
`MachineConfiguration` CR to let the user pick which irreconcilable fields 
can be pushed to the nodes without the MCO blocking the changes.

As a side effect of this feature, and given that Ignition runs once in CoreOs,
is that old existing nodes will never have the new Ignition changes applied 
(unless the changes are part of the fields the MCO MCD applies as day-2 
operations), adding troubleshooting complexity, as with the mechanisms the MCO 
has right now is not viable to know which node has which configuration. For 
this reason, the MCN CR is changed to report more information about the 
configurations applied to the node.

### Validation Policy Selection
 
The MCO exposes a new field, called `machineConfigurationValidationPolicy` 
as part of the spec of the `MachineConfiguration` CR to let the user select 
which irreconcilable fields should be not considered for validation purposes. 
The field, in turn, is composed by enum based fields that matches each 
possible irreconcilable field the MCO considers. The user, by setting each 
field to `Strict` or `Relaxed` is able to skip the validation the MCO performs.

The full specification of the `machineConfigurationValidationPolicy` is 
described as part of [API Extensions](#api-extensions) section.

### Status reporting

The MCO exposes a new field as part of the MCN CR status of each node that 
contains a list of structures, called `MachineConfigUpdateReport`, that 
exposes a history of the rendered configurations applied and the deviation 
between the desired configuration and what the MCD applied to each node.
The `MachineConfigUpdateReport` is formed by:
+ The name of the current and desired rendered MCs.
+ A reference to a `ConfigMap` that contains `diffs` between both 
MachineConfigs.
+ A summary of the MCD supported fields that where updated.

The full specification of the `MachineConfigUpdateReport` is described as part 
of [API Extensions](#api-extensions) section.

### Workflow Description

Each time a MachineConfig is changed a new rendered MachineConfig is created
for each pool associated to the changed MachineConfig.

Before the freshly created rendered MachineConfig passes to the 
MachineConfigDaemons the MCO performs an internal validation of it that can 
can be divided into three phases:

1. Parse the Ignition raw configuration.
2. Ensure the Ignition configuration is valid.
3. Ensure there are no changes to irreconcilable fields.

The first two steps are self-explanatory and are not covered by this 
enhancement, as the MCO will always perform them. The third one, the 
validation of irreconcilable fields, is the main target of this enhancement.

After the Ignition validation is done, the per-field irreconcilable validation 
is performed or skipped based on the `machineConfigurationValidationPolicy` 
field in the MachineConfiguration CR.
Each validated irreconcilable field has its own individual policy field:

```yaml
apiVersion: operator.openshit.io/v1
kind: MachineConfiguration
metadata:
    name: cluster
spec:
  # ...
  machineConfigurationValidationPolicy:
    fields:
      passwd: Strict
      storageDisks: Relaxed
      storageFileSystems: Relaxed
      storageRaid: Strict
      storageDirectories: Strict
      storageFiles: Strict
# ...
```

The MCO, given, for example, the previous 
`machineConfigurationValidationPolicy`, ensures no field but `disks` and 
`filesystems` can be changed from the original install-time values and 
a MachineConfig that was not possible till this enhancement, like the one below 
can now progress:
```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-var-partition
spec:
  config:
    ignition:
      version: 3.5.0
    storage:
      disks:
        - device: /dev/disk/by-path/pci-0000:3b:00.0-scsi-0:3:111:0
          partitions:
            - label: var
              sizeMiB: 0
              startMiB: 0
          wipeTable: true
      filesystems:
        - device: /dev/disk/by-partlabel/var
          format: xfs
          path: /var
```

If a field for a validation is set to `Relaxed` the validation is skipped, 
otherwise is performed.

After the validation checks are done the nodes of the MCP are updated to point 
to the new rendered MachineConfig as the desired one and the MCD starts to 
apply the requested changes.

MachineConfig daemons will continue to perform the already supported updates 
to nodes, no matter if irreconcilable validations are skipped or not. Any 
change to the MachineConfig out of supported ones is ignored.

When the update process successfully finishes the MCD will report by performing 
two actions: 
- Addition of a `MachineConfigUpdateReport` to the `updateReports` status 
field of the MCN associated to its node, reflecting the applied changes. If 
the feature this enhancement describes is not in use MCDs do not report the 
changes in MCNs to reflect that all nodes reached the desired configuration. 
As soon as this feature is enabled and a node diverges from the current state 
it will report a new `MachineConfigUpdateReport` on each update.

- Updated the `machineconfiguration.openshift.io/currentConfig` annotation to
point to the updated one and `machineconfiguration.openshift.io/state` to 
`Done`.

If at some point the user sets all the existing 
`machineConfigurationValidationPolicy` fields to `Strict` (or entirely remove 
the policy) the `updateReports` list of each node is cleared to express that 
there are no differences between the desired configuration and the 
configuration of each node if and only if the diff between the current 
configuration of the node and the desired one is zero.

### API Extensions

#### MachineConfiguration

Update the `MachineConfiguration` CRD to add a field that lets the user skip
individual irreconcilable fields validations called 
`machineConfigurationValidationPolicy`. To ensure the field can evolve in the 
future with common, not per-field, settings, the individual policy fields are
nested under the 'fields' section:

```yaml
apiVersion: operator.openshi t.io/v1
kind: MachineConfiguration
metadata:
    name: cluster
spec:
  logLevel: Normal
  managementState: Managed
  operatorLogLevel: Normal
  machineConfigurationValidationPolicy:
    fields:
      passwd: Strict
      storageDisks: Strict
      storageFileSystems: Relaxed
      storageRaid: Strict
      storageDirectories: Strict
      storageFiles: Strict
status:
  # ...
```

Each enumeration field under `machineConfigurationValidationPolicy.fields` has
two possible values:
  - Strict: Validation of the field is always performed. This is the value the 
    MCO will use as default.
  - Relaxed: The irreconcilable field validation is skipped.

The `MachineConfiguration` status section does not require  any change.

#### MachineConfigNode

The `MachineConfigNode` CRD requires a change to add a new field under the 
status section to allow MCDs to report the update process result after 
applying a new rendered MachineConfig with this enhancement enabled (one or
more fields under `machineConfigurationValidationPolicy` set to `Relaxed`).

Based on the [Status Reporting](#status-reporting) section, the information 
required to troubleshoot clusters that takes advantage of this enhancement is:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigNode
metadata:
  name: worker-0
spec:
  # Spec content. Remains unchanged.
status:
  updateReports:
    - currentMachineConfig: rendered-worker-0bd73c31ddc08248955203a8da514eaf
      desiredMachineConfig: rendered-worker-3b024f8ff47b7604867aba90c8061cce
      diffsConfigMap:
        name: machine-config-daemon-update-diff-d1061d365f13b9d799cc9b0a92e4ecd5
      changed:
        passwd: "Changed"
	      files: "Unchanged"
        units: "Unchanged"
        os: "Unchanged"
        kernelArgs: "Unchanged"
        kernelType: "Unchanged"
  # ... Already existing status fields
```

The reports mission is to save a record of what was applied on each update to 
each node for troubleshooting purposes, thus the information a report contains:
- `currentMachineConfig`: Name of the current configuration when the MCD ran 
the update process.
- `desiredMachineConfig`: Name of the desired configuration when the MCD 
ran the update process. The state of the node may differ from this 
configuration if irreconcilable or MCD not managed fields are part of the 
configuration.
- `diffsConfigMap`: The diff between the current and the desired rendered 
MachineConfigs at the time of the update.
- `changed`: Contains a summary of the supported MCD fields that required 
update.

The `MachineConfigNode` spec section does not require any change.

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

The enhancement will be covered by unit tests and e2e tests.
In particular, for e2e testing, the following scenarios will be added:
- Scale up with a mix of irreconcilable and supported fields: This test 
ensures that a cluster with this feature enabled is able to handle updates in 
both "old" and "new" nodes. New nodes should see the latest Ignition 
configuration while already existing nodes should apply MCD supported fields 
and should report differences between the current configuration and the 
desired one.
- Scale up with only irreconcilable fields: This test ensures that a cluster 
with this feature enabled is able to handle updates to new nodes without 
disturbing already existing ones. New nodes should see the latest Ignition 
configuration while already existing nodes should not report updates in any 
of the MCD supported fields. The diff between the current and the desired 
configuration in old nodes should match all the changes applied to new nodes.
- Scale up with the feature turned on and feature disabling: This test has is 
divided into two sub-tests:
    - Irreconcilable changes rolled back before validations are turned on: In 
    this scenario `updateReports` will be cleared as the machines will match 
    the latest rendered MC, no matter if the machine were or not added after 
    the test introduced irreconcilable changes.

    - Irreconcilable changes not rolled back before validations are turned on: 
    In this scenario `updateReports` will continue reporting differences in 
    "old" nodes even after disabling the feature.

### Graduation Criteria

This feature is behind a new FeatureGate, called NonReconcilableMC, and part
of the tech-preview FeatureGate in 4.20. 
Once it is tested by QE and users it can be GA'd since it should not impact 
daily usage of a cluster.

## Dev Preview -> Tech Preview

Not applicable. Feature introduced in Tech Preview. 

## Tech Preview -> GA

- Bugs found by e2e tests and QE are solved and the dedicated e2e's are stable.
- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Upgrades or downgrades are not impacted by the presence or not of this feature.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

#### Failure Modes

If the irreconcilable configuration validation is performed and it fails 
the MCO continues to report the failure as it is alraedy doing in the MCP, by
setting to the MCP the `RenderDegraded` condition to true.

If the configuration reaches the MCD and the irreconcilable validation 
fails the MCN `UpdatePrepared` condition is updated with the details of the 
validation failure.

#### Support Procedures

None.

## Implementation History

Not applicable.

## Alternatives (Not Implemented)

The main alternative to the proposal this enhancement proposes is to modify 
the behavior of the custom MachineConfigPools to allow not inheriting from the 
worker pool. That way, customers would be able to specify their own MCPs for 
a dedicated set of nodes without the need of dealing with the originally 
created worker MCP.
While this approach can be seen as the clean way of solving the uses cases 
this enhancement addresses is has some drawbacks that need to be mentioned:
- It would introduce a complex breaking change in the behavior of the MCPs. A 
custom MCP 'bootable' approach could introduce a mechanism to select whether 
MCPs inherit from the worker pool. However, this would add unnecessary 
complexity for customers who do not require the use cases targeted by this 
enhancement.
- Changing the inheritance behavior of the MCPs, with or without a knob, 
requires a complex and time-consuming rework of the MCO's MCPs logic that is 
not justified by the use cases this enhancement targets.
- Compared to this proposal, changing the inheritance behavior would 
necessitate a more extensive rework of MCP testing to ensure the default 
behavior remains unchanged and to create tests specifically focusing on 
'bootable' MCPs.
