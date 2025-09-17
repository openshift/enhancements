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
Once the user configures install-time parameters with the initial 
Ignition specification used at cluster install-time, the MCO will prevent any 
further changes to fields that MCDs do not support (known as irreconcilable 
fields), locking the user into an Ignition specification for the rest of the 
life of the cluster.

The exhaustive list of fields the MCD supports to be changed after the cluster 
is built can be found in the ["What can you change with machine configs?"](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/machine_configuration/machine-config-index#what-can-you-change-with-machine-configs) 
section of the official documentation.

While this is generally useful for safety, it becomes problematic for any 
users who wish to change install-time only parameters, such as disk 
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
* As a cluster administrator, I would like to be able to identify older 
machines that are not running my latest config, understand the difference 
between that and my latest config, and determine whether the nodes need to be 
replaced based on this difference

### Goals

* Allow users to provide MachineConfigs (MCs) with irreconcilable fields for 
use cases where preserving original install-time parameter values is not 
feasible.

### Non-Goals

* Allow invalid Ignition/MachineConfig fields to be applied.
* Disable irreconcilable MCs validation by default.

## Proposal

To enable users to use a different Ignition file than the one used at cluster 
install-time, the MCO will allow changes to MachineConfigs that existing nodes 
can ignore but new joining nodes can consume.

To achieve that, the enhancement adds a new field to the 
`MachineConfiguration` CR to let the user pick which irreconcilable fields 
can be pushed to the nodes without the MCO blocking the changes.

As a side effect of this feature, and given that Ignition runs once in CoreOS,
is that old existing nodes will never have the new Ignition changes applied 
(unless the changes are part of the fields the MCO MCD applies as day-2 
operations), adding troubleshooting complexity, as with the mechanisms the MCO 
has right now is not viable to know which node has which configuration. For 
this reason, the MCN CR is changed to report more information about the 
configurations applied to the node.

### Validation Policy Selection
 
The MCO exposes a new field, called `irreconcilableValidationOverrides` as 
part of the spec of the `MachineConfiguration` CR to let the user select which 
irreconcilable fields should be not considered for validation purposes. 

The full specification of the `irreconcilableValidationOverrides` is 
described as part of [API Extensions](#api-extensions) section.

### Status reporting

For troubleshooting purposes the MCO needs to report what kind of 
irreconcilable changes have been applied to a cluster and when the changes 
have occurred.

To do so, each node MCD takes care of computing the differences between the 
original Ignition configuration and the latest state after applying the 
target rendered configuration.

If this feature is enabled and there are irreconcilable differences between 
the applied and the target rendered configurations the MCD populates the 
`irreconcilableChanges` field of the MCN status section.

The [Workflow Description section](#workflow-description) gives a deeper 
explanation on the details of the summary.

If future troubleshooting needs justifies the need of fine grain tracking the 
enhancement, API and implementation will evolve in that direction.

### Workflow Description

Each time a MachineConfig is changed a new rendered MachineConfig is created
for each pool associated to the changed MachineConfig.

#### MachineConfigController early validation

Before the freshly created rendered MachineConfig passes to the 
MachineConfigDaemons the MCO performs an internal validation of it that can be 
divided into three phases:

1. Parse the Ignition raw configuration.
2. Ensure the Ignition configuration is valid.
3. Ensure there are no changes to irreconcilable fields.

The first two steps are self-explanatory and are not covered by this 
enhancement, as the MCO will always perform them. The third one, the 
validation of irreconcilable fields, is the main target of this enhancement.

After the Ignition validation is done, the per-field irreconcilable validation 
is performed or skipped based on the `irreconcilableValidationOverrides` 
content in the MachineConfiguration CR.

```yaml
apiVersion: operator.openshift.io/v1
kind: MachineConfiguration
metadata:
    name: cluster
spec:
  # ...
  irreconcilableValidationOverrides:
    storage:
      - Disks       # Allow changes to Ignition's storage disks section
      - FileSystems # Allow changes to Ignition's filesystem disks section
# ...
```

The MCO, given the previous `irreconcilableValidationOverrides` example, 
ensures no field but `disks` and `filesystems` can be changed from the 
original install-time values and a MachineConfig that was not possible till 
this enhancement, like the one below can now progress:

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

All validation fields not listed in `irreconcilableValidationOverrides` will 
continue to be validated against reconcilable changes as always.

#### MachineConfigDaemons update process

After the validation checks are done the nodes of the MCP are updated to point 
to the new rendered MachineConfig as the desired one and the MCD starts to 
apply the requested changes.

MachineConfig daemons will continue to perform the already supported updates 
to nodes, no matter if irreconcilable validations are skipped or not. Any 
change to the MachineConfig out of supported ones is ignored.

When the update process successfully finishes the MCD reports by performing 
two actions:

- Calculate the difference between the irreconcilable fields used at cluster 
installation time and after the rendered configuration apply. The difference 
result, as a per-field human readable string, is set in the 
`irreconcilableChanges` status field of the MCN.

- Update the `machineconfiguration.openshift.io/currentConfig` annotation to
point to the updated one and `machineconfiguration.openshift.io/state` to 
`Done`.

To compute the difference between install-time parameters and the latest 
rendered MachineConfig configuration, the MCD reads the 
`/etc/mcs-machine-config-content.json` file. This file is generated by the MCS 
and rendered by Ignition during each node's first boot and persists through 
upgrades unless a user manually removes it. By default, this enhancement 
will use that file. However, if the file is missing, the MCD will attempt to 
reconstruct the node's original state by searching for the earliest available 
MachineConfig to use as a base for generating the diff.

Nodes joining the cluster will never have irreconcilable differences between 
the rendered configuration and the initial Ignition parameters, as both will 
match. If at some point of the lifespan of a node this feature is enabled 
and the node observes irreconcilable differences, the `irreconcilableChanges` 
field will be set. After the field is added to the MCN it can only be cleared 
out if the non-reconcilable changes are reverted for the target rendered 
MachineConfig.

### API Extensions

#### MachineConfiguration

Update the `MachineConfiguration` CRD to add a field that lets the user skip
individual irreconcilable fields validations called 
`irreconcilableValidationOverrides`.

```yaml
apiVersion: operator.openshift.io/v1
kind: MachineConfiguration
metadata:
    name: cluster
spec:
  # ... Already existing spec fields
  irreconcilableValidationOverrides:
    storage:
    - Disks       # Allow changes to Ignition's disks section
    - FileSystems # Allow changes to Ignition's filesystem section
    - Raid        # Allow changes to Ignition's raid  section
status:
  # ...
```

Based on the customer use-cases the MCO has encountered over the years, we 
have determined that supporting irreconcilable changes for partition schema 
modifications will address the primary operational need driving this 
enhancement. 
Customer requests consistently center around deploying nodes with different 
storage configurations than those used during initial cluster deployment.

This enhancement therefore focuses on supporting 
`irreconcilableValidationOverrides` for the specific fields required to 
modify partition schemas, that are all storage related and are named:

* Disks
* FileSystems
* Raid

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
`irreconcilableValidationOverrides` not empty).

As stated in previous sections the MCD takes care of computing the difference 
between the state of its node after a new rendered configuration is applied 
and the target rendered configuration.

To provide that information to the user for troubleshooting purposes the MCN 
exposes a new string field that exposes the diff in a human-readable format.

As an example, based on the one provided in the 
[MachineConfigController early validation](#machineconfigcontroller-early-validation) 
section the MCD will fill the MCN with the following information assuming 
that the original install-time configuration was identical but only the 
`device` has changed.

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigNode
metadata:
  name: worker-0
spec:
  # Spec content. Remains unchanged.
status:
  irreconcilableChanges:
    - fieldPath: 'spec.config.storage.disks[0].device'
      diff: |-
          - "/dev/disk/by-path/pci-0000:3b:00.0-scsi-0:3:222:0"
          + "/dev/disk/by-path/pci-0000:3b:00.0-scsi-0:3:111:0"
  # ... Already existing status fields
```
Clusters that are not using this feature or nodes that have no configuration 
drift from install-time ones don't report the `irreconcilableChanges` field.

The `MachineConfigNode` spec section does not require any change.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature is not available in Hypershift topologies because Hypershift is 
not within the scope of the use cases that motivated this feature, and the 
CRs used to interact with it are not available due to the different MCO 
topology.

#### Standalone Clusters

No special considerations.

#### Single-node Deployments or MicroShift

While this feature can be enabled in single-node topologies, users will not be 
able to change the install-time configuration due to the fact that Ignition 
runs only once, meaning changes to MachineConfigs are only reflected on new 
nodes. For single-node topologies, the recommended approach to address 
configuration changes is to redeploy the cluster.

### Implementation Details/Notes/Constraints

#### MachineConfigController

The controller-side implementation is straightforward, requiring only an 
update to the well-known [isConfigReconcilable](https://github.com/openshift/machine-config-operator/blob/275ebe05779804733426ea781d7158386c367060/pkg/controller/common/reconcile.go#L69) 
function. This function will be modified to conditionally enable or disable 
its validation logic based on the content of the MachineConfiguration CR.

#### MachineConfigDaemon

The changes to perform in the MCD are all tied to the reporting side of this 
feature. To be able to fill the `irreconcilableChanges` with the applied MCs
when the feature is enabled the MCD requires the following changes:

- Generation of an alternative file to `/etc/mcs-machine-config-content.json`
in case it doesn't exist based on the earliest known MachineConfig the MCD
is able to retrieve. This file is only used in case the the MCS generated one 
is not available.
- Addition of a piece of logic that subtracts from the target rendered 
configuration the MCD supported changes applied in the run. The result of 
that subtraction will be compared against the `originalconfig` content 
and any difference will be reported in the `irreconcilableChanges` field of
the MCN status.

### Risks and Mitigations

By setting a field in `irreconcilableValidationOverrides` the customer 
acknowledges that providing MCs that make use of Ignition features out of the 
scope of the MCO will lead to cluster with nodes using different Ignition 
configurations.

### Drawbacks

- The main drawback of this approach is that it introduces the concept of 
configuration drift between what the user applies and the real configuration 
the node has, where ideally, in a kubernetes environment, what the user applies 
should be what each node uses.

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
and should report irreconcilable through `irreconcilableChanges`. New nodes 
MCN CRs should not differences as their configuration matches the latest 
rendered Ignition.

- Scale up with only irreconcilable fields: This test ensures that a cluster 
with this feature enabled is able to handle updates to new nodes without 
disturbing already existing ones. New nodes should see the latest Ignition 
configuration and their MCNs `irreconcilableChanges` should be empty while 
already existing nodes will start reporting differences but no changes 
should be applied.

- Scale up with the feature turned on and feature disabling: This test
ensures that after making irreconcilable changes to the cluster, the 
`irreconcilableChanges` field in the affected MCNs is preserved even if the 
feature is disabled. The test also verifies that once the 
`irreconcilableValidationOverrides` configuration is removed, further 
irreconcilable changes are correctly rejected.

- Scale up with the feature turned on revert the changes: This test
ensures that after making irreconcilable changes to the cluster, the 
`irreconcilableChanges` field can be reset by undoing the change. This test 
must ensure that after resetting the configuration the a freshly added node 
will start reporting `irreconcilableChanges` while the old ones have cleared 
out the field.

## Graduation Criteria

This feature is behind a new FeatureGate, called 
`NonReconcilableMachineConfig`, and part of the tech-preview FeatureGate in 
4.20. 
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
not justified by the use cases this enhancement targets. Without a change to 
the inheritance model users won't be able to have full control of the merge/
patch strategy and operations like removing existing configuration from the 
base MCs is not possible.

- Compared to this proposal, changing the inheritance behavior would 
necessitate a more extensive rework of MCP testing to ensure the default 
behavior remains unchanged and to create tests specifically focusing on 
'bootable' MCPs.
