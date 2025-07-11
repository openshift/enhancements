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
creation-date: 2025-07-07
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
 
The MCO exposes a new field, called `irreconcilableMCValidationOverrides` as 
part of the spec of the `MachineConfiguration` CR to let the user select which 
irreconcilable fields should be not considered for validation purposes. 

The full specification of the `irreconcilableMCValidationOverrides` is 
described as part of [API Extensions](#api-extensions) section.

### Status reporting

For troubleshooting purposes the MCO needs to report what kind of 
irreconcilable changes have been applied to a cluster and when the changes 
have occurred.
Reporting of irreconcilable changes has two main sides:

- A high level one, that consists of a summary of irreconcilable fields 
applied to the nodes of a MCP and gives users the ability to quickly know 
which deviations from the install-time parameters the nodes of a MCP have.

- A fine grain per-node tracking mechanism that reports when each MCD applied 
configurations that resulted in irreconcilable changes, the source and 
target rendered MC and the diff/deviation of each node from the MCP's rendered 
MC.

Given the implementation complexity of fine-grained tracking, this enhancement 
focuses on the summary approach. This provides immediate value for the most 
common troubleshooting scenarios, while per-node tracking can be developed in 
a future enhancement if there is a strong need.

The [Workflow Description section](#workflow-description) give a deeper 
explanation on the details of the summary.

If future troubleshooting needs justifies the need of fine grain tracking the 
enhancement, API and implementation will evolve in that direction.

### Workflow Description

Each time a MachineConfig is changed a new rendered MachineConfig is created
for each pool associated to the changed MachineConfig.

#### MachineConfigController early validation

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
is performed or skipped based on the `irreconcilableMCValidationOverrides` 
list in the MachineConfiguration CR.

```yaml
apiVersion: operator.openshit.io/v1
kind: MachineConfiguration
metadata:
    name: cluster
spec:
  # ...
  irreconcilableMCValidationOverrides:
    - storageDisks       # Allow changes to Ignition's storage disks section
    - storageFileSystems # Allow changes to Ignition's filesystem disks section
# ...
```

The MCO, given, for example, the previous 
`irreconcilableMCValidationOverrides`, ensures no field but `disks` and 
`filesystems` can be changed from the original install-time values and 
a MachineConfig that was not possible till this enhancement, like the one 
below can now progress:
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

All validation fields not listed in `irreconcilableMCValidationOverrides` will 
continue to be validated against reconcilable changes as always.

#### MachineConfigController irreconcilable changes summary report

Before the MachineConfigController updates the nodes to point to the new 
rendered configuration it creates the summary ConfigMap for the target pool, 
that will be later referenced by the MCNs of nodes that contain 
irreconcilable changes.

The content of the ConfigMap for the previous example is as follows:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: machine-config-irreconcilable-worker
  namespace: openshift-machine-config-operator
data:
  updates:
    - type: "storageDisks"
      count: 1
      lastUpdateTime: "2025-05-04T03:02:01Z"
    - type: "storageFileSystems"
      count: 1
      lastUpdateTime: "2025-01-02T03:04:05Z"
```
Where each item under the `updates` list contains:

- `type`: Type of irreconcilable change. Only the supported values of 
`irreconcilableMCValidationOverrides` are supported.

- `count`: The number of rendered MachineConfigs with irreconcilable changes 
to the section the item is associated the MCC has pushed to nodes after 
the associated field policy was set to to `Relaxed`.

- `lastUpdateTime`: The timestamp of the most recent irreconcilable change 
of this type applied to the pool.

#### MachineConfigDaemons update process

After the validation checks and the ConfigMap are done the nodes of the MCP 
are updated to point to the new rendered MachineConfig as the desired one and 
the MCD starts to apply the requested changes.

MachineConfig daemons will continue to perform the already supported updates 
to nodes, no matter if irreconcilable validations are skipped or not. Any 
change to the MachineConfig out of supported ones is ignored.

When the update process successfully finishes the MCD will report by performing 
two actions:

- Addition of a the reference to the ConfigMap that contains the report of the
irreconcilable changes for the pool under the new 
`irreconcilableChangesConfigMap` field of the MCN status section. If the node 
determines that there were no differences between its applied configuration 
and the target one (typically happening in nodes joining the cluster) the MCD
will leave the reference to the ConfigMap empty.

- Updated the `machineconfiguration.openshift.io/currentConfig` annotation to
point to the updated one and `machineconfiguration.openshift.io/state` to 
`Done`.

Once the `irreconcilableChangesConfigMap` field is set in a MachineConfigNode, 
it is never cleared.

### API Extensions

#### MachineConfiguration

Update the `MachineConfiguration` CRD to add a field that lets the user skip
individual irreconcilable fields validations called 
`irreconcilableMCValidationOverrides`.

```yaml
apiVersion: operator.openshift.io/v1
kind: MachineConfiguration
metadata:
    name: cluster
spec:
  # ... Already existing spec fields
  irreconcilableMCValidationOverrides:
    - StorageDisks       # Allow changes to Ignition's disks section
    - StorageFileSystems # Allow changes to Ignition's filesystem section
    - StorageRaid        # Allow changes to Ignition's raid  section
status:
  # ...
```

Based on customer use-cases the MCO has encountered over the years, we have 
determined that supporting irreconcilable changes for partition schema 
modifications addresses the primary operational need driving this enhancement. 
Customer requests consistently center around deploying nodes with different 
storage configurations than those used during initial cluster deployment.

This enhancement therefore focuses on supporting 
`irreconcilableMCValidationOverrides` for the specific fields required to 
modify partition schemas:

* StorageDisks
* StorageFileSystems
* StorageRaid

These three fields provide complete coverage for storage configuration 
scenarios while maintaining a focused scope that aligns with demonstrated 
customer needs. Additional fields such as `Passwd`, `StorageDirectories`, 
`StorageFiles` and `StorageLinks` are not included in this initial 
implementation as they fall outside the scope of partition schema changes and 
have not been identified as part of the relevant customer use cases.

The `MachineConfiguration` status section does not require any change.

#### MachineConfigNode

The `MachineConfigNode` CRD requires a change to add a new field under the 
status section to allow MCDs to report the update process results after 
applying a new rendered MachineConfig with this enhancement enabled ( 
`irreconcilableMCValidationOverrides` not empty).

As stated in the [Status Reporting](#status-reporting) section, all the nodes 
of a MCP share the same statistics, so to avoid storage missusage, specially 
critical in clusters with a high count of nodes, each MCN CR will just point
to the ConfigMap that holds the latest statistics about irreconcilable 
changes of the pool the MCN is part of.

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigNode
metadata:
  name: worker-0
spec:
  # Spec content. Remains unchanged.
status:
  irreconcilableChangesConfigMap:
    name: machine-config-irreconcilable-worker
  # ... Already existing status fields
```
Clusters that are not using this feature or nodes that have no config drift 
from install-time ones don't report the `irreconcilableChangesConfigMap` field.

The `MachineConfigNode` spec section does not require any change.

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

### Risks and Mitigations

By setting a field in `irreconcilableMCValidationOverrides` the customer 
acknowledges that providing MCs that make use of Ignition features out of the 
scope of the MCO will lead to cluster with nodes using different Ignition 
configurations.

### Drawbacks

- Tracking the divergence of node's configuration between the target render 
and the install-time configuration requires, without implementing the 
second point described in [Status reporting](#status-reporting), manual 
or script based analysis of all the available rendered MCs from the cluster 
(a must-gather package suffices). Both, manual and script based analysis can 
be time consuming and error prone.

## Design Details

### Open Questions [optional]

None.

## Test Plan

The enhancement will be covered by unit tests and e2e tests.
In particular, for e2e testing, the following scenarios will be added:

- Scale up with a mix of irreconcilable and supported fields: This test 
ensures that a cluster with this feature enabled is able to handle updates in 
both "old" and "new" nodes. New nodes should see the latest Ignition 
configuration while already existing nodes should apply MCD supported fields 
and should report irreconcilable changes applied in the irreconcilable 
irreconcilable report ConfigMap. New nodes MCN CRs should not report a 
reference to the status report ConfigMap as their configuration matches the 
latest rendered Ignition.

- Scale up with only irreconcilable fields: This test ensures that a cluster 
with this feature enabled is able to handle updates to new nodes without 
disturbing already existing ones. New nodes should see the latest Ignition 
configuration and their MCNs reference to the status report ConfigMap should 
be empty. Already existing nodes should be kept intact.

- Scale up with the feature turned on and feature disabling: This test
ensures that after making irreconcilable changes to the cluster, the 
`irreconcilableChangesConfigMap` field in the affected MCNs is preserved 
alongside the referenced `ConfigMap` even if the feature is disabled. 
The test also verifies that once the `irreconcilableMCValidationOverrides` 
configuration is removed, further irreconcilable changes are correctly rejected.

## Graduation Criteria

This feature is behind a new FeatureGate, called `NonReconcilableMC`, and part
of the tech-preview FeatureGate in 4.20. 
Once it is tested by QE and users it can be GA'd since it should not impact 
daily usage of a cluster.

### Dev Preview -> Tech Preview

Not applicable. Feature introduced in Tech Preview. 

### Tech Preview -> GA

- Bugs found by e2e tests and QE are solved and the dedicated e2e's are stable.
- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

Upgrades or downgrades are not impacted by the presence or not of this feature.

## Version Skew Strategy

Not applicable.

## Operational Aspects of API Extensions

#### Failure Modes

If the irreconcilable configuration validation is performed and it fails 
the MCO continues to report the failure as it is already doing in the MCP, by
setting to the MCP the `RenderDegraded` condition to true.

If the configuration reaches the MCD and the irreconcilable validation 
fails the MCN `UpdatePrepared` condition is updated with the details of the 
validation failure.

## Support Procedures

None.

## Implementation History

Not applicable.

## Alternatives (Not Implemented)

The main alternative to what this enhancement proposes is to modify the 
behavior of the custom MachineConfigPools to allow not inheriting from the 
built-in pools. That way, customers would be able to specify their own MCPs 
for a dedicated set of nodes without the need of dealing with the originally 
created built-in MCP.
While this alternative approach can be seen as a more refined solution that 
does not imply introducing the need of having nodes that are not fully aligned 
with the rendered configuration, the approach has several drawbacks:

- It would introduce a complex breaking change in the behavior of the MCPs as 
 customers (and operators like the compliance operator) have already built MCs 
 assuming the final applied MC is the compound of the base `worker` pool plus 
 the MCs of the custom MCP. Making custom MCPs independent from the base 
 built-in pool will require users to rewrite their  MCs. To  prevent this from 
 happening an option could be to introduce a  mechanism to select whether MCPs 
 inherit from the built-in pool or not. However, this would add unnecessary 
 complexity for customers who do not  require the use  cases targeted by this 
 enhancement.

- Changing the inheritance behavior of the MCPs, with or without a knob, 
requires a complex and time-consuming rework of the MCO's MCPs logic that is 
not justified by the use cases this enhancement targets.

- Compared to this proposal, changing the inheritance behavior would 
necessitate a more extensive rework of MCP testing to ensure the default 
behavior remains unchanged and to create tests specifically focusing on 
'bootable' MCPs.
