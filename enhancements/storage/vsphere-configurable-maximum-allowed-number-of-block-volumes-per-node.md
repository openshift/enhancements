---
title: vsphere-configurable-maximum-allowed-number-of-block-volumes-per-node
authors:
  - "@rbednar"
reviewers:
  - "@jsafrane"
  - "@gnufied"
  - "@deads2k"
approvers:
  - "@jsafrane"
  - "@gnufied"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-01-31
last-updated: 2025-01-31
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1829
see-also:
  - "None"
replaces:
  - "None"
superseded-by:
  - "None"
---

# vSphere configurable maximum allowed number of block volumes per node

This document proposes an enhancement to the vSphere CSI driver to allow administrators to configure the maximum number
of block volumes that can be attached to a single vSphere node. This enhancement addresses the limitations of the
current driver, which currently relies on a static limit that can not be changed by cluster administrators.

## Summary

The vSphere CSI driver for vSphere version 7 uses a constant to determine the maximum number of block volumes that can
be attached to a single node. This limit is influenced by the number of SCSI controllers available on the node.
By default, a node can have up to four SCSI controllers, each supporting up to 15 devices, allowing for a maximum of 60
volumes per node (59 + root volume).

However, vSphere version 8 increased the maximum number of volumes per node to 256 (255 + root volume). This enhancement
aims to leverage this increased limit and provide administrators with finer-grained control over volume allocation
allowing them to configure the maximum number of block volumes that can be attached to a single node.

Details about configuration maximums: https://configmax.broadcom.com/guest?vmwareproduct=vSphere&release=vSphere%208.0&categories=3-0
Volume limit configuration for vSphere storage plug-in: https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/container-storage-plugin/3-0/getting-started-with-vmware-vsphere-container-storage-plug-in-3-0/vsphere-container-storage-plug-in-concepts/configuration-maximums-for-vsphere-container-storage-plug-in.html
Knowledge base article with node requirements: https://knowledge.broadcom.com/external/article/301189/prerequisites-and-limitations-when-using.html

## Motivation

### User Stories

- As a vSphere administrator, I want to configure the maximum number of volumes that can be attached to a node, so that
  I can optimize resource utilization and prevent oversubscription.
- As a cluster administrator, I want to ensure that the vSphere CSI driver operates within the limits imposed by the 
  underlying vSphere infrastructure.

### Goals

- Provide administrators with control over volume allocation limit on vSphere nodes.
- Improve resource utilization and prevent oversubscription.
- Ensure compatibility with existing vSphere infrastructure limitations.
- Maintain backward compatibility with existing deployments.

### Non-Goals

- Support heterogeneous environments with different ESXi versions on nodes that form OpenShift cluster.
- Dynamically adjust the limit based on real-time resource usage.
- Implement per-namespace or per-workload volume limits.
- Modify the underlying vSphere VM configuration.

## Proposal

1. Driver Feature State Switch (FSS):

   - Use FSS (`max-pvscsi-targets-per-vm`) of the vSphere driver to control the activation of the maximum volume limit
     functionality.
   - No changes needed, the feature is enabled by default.

2. API for Maximum Volume Limit:

   - Introduce a new field `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode` in ClusterCSIDriver API to allow
     administrators to configure the desired maximum number of volumes per node.
   - This field should not have a default value and the actual default will be set by the operator to the current
     maximum limit of 59 volumes per node which matches limit for vSphere 7.
   - API will not allow `0` value to be set or allow the field to be unset. This would lead to
     disabling the limit.
   - Allowed range of values should be 1 to 255. The maximum value matches vSphere 8 limit.

3. Update CSI Pods with hooks:

   - After reading the new `maxAllowedBlockVolumesPerNode` API field from ClusterCSIDriver the operator will inject the
     `MAX_VOLUMES_PER_NODE` environment variable into all pods using a DaemonSet and Deployment hooks.
   - Any value that is statically set for the `MAX_VOLUMES_PER_NODE` environment variable in asset files will
     be overwritten. If the variable is omitted in the asset, hooks will add it and set it its value found in 
     `maxAllowedBlockVolumesPerNode` field of ClusterCSIDriver. If the field is not set, the default value will be 59
     to match vSphere 7 limit.

4. Operator behavior:

   - The operator will check ESXi versions on all nodes in the cluster. Setting `maxAllowedBlockVolumesPerNode` to a
     higher value than 59 while not having ESXi version 8 or higher on all nodes will result in cluster degradation.

5. Driver Behavior:

   - The vSphere CSI driver needs to allow the higher limit with Feature State Switch FSS (`max-pvscsi-targets-per-vm`).
   - The switch is already enabled by default in versions shipped in OpenShift 4.19.
   - The driver will report the volume limit as usual in response to `NodeGetInfo` calls.

6. Documentation:

   - Update the vSphere CSI driver documentation to include information about the new feature and how to configure it.
     However, at the time of writing we don't have any official vSphere documentation to refer to that would explain how
     to configure vSphere to support 256 volumes per node.
   - Include a statement informing users of the current requirement of having a homogeneous cluster with all nodes
     running ESXi 8 or higher. Until this requirement is met, the limit set in `maxAllowedBlockVolumesPerNode` must not
     be increased to a higher value than 59. If higher value is set regardless of this requirement the cluster will
     degrade.
   - Currently, there is no Distributed Resource scheduler (DRS) validation in place in the vSphere to make sure we do
     not end up having more VMs with 256 disks on the same host so users might exceed the limit of 2048 Virtual Disks
     per Host. This is a known limitation of vSphere, and we need note this in documentation to make users aware of this
     potential risk.

### Workflow Description

1. Administrator configures the limit:
   - The administrator creates or updates a ClusterCSIDriver object to specify the desired maximum number of volumes per
     node using the new `maxAllowedBlockVolumesPerNode` API field.
2. Operator reads configuration:
   - The vSphere CSI Operator monitors the ClusterCSIDriver object for changes.
   - Upon detecting a change, the operator reads the configured limit value.
3. Operator updates the new limit for DaemonSet and Deployment:
   - The operator updates the pods of vSphere CSI driver, injecting the `MAX_VOLUMES_PER_NODE` environment
     variable with the configured limit value into the driver node pods on worker nodes.

### API Extensions

- New field in ClusterCSIDriver CRD:
  - A new CRD field will be introduced to represent the maximum volume limit configuration.
  - This CRD will contain a single new field (e.g., `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode`) to define
    the desired limit.
  - The API will validate the value fits within the defined range (1-255).

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations for Hypershift. The configuration and behavior of the vSphere CSI driver with respect to the
maximum volume limit will remain consistent across standalone and managed clusters.

#### Standalone Clusters

This enhancement is fully applicable to standalone OpenShift clusters.

#### Single-node Deployments or MicroShift

No unique considerations for MicroShift. The configuration and behavior of the vSphere CSI driver with respect to the
maximum volume limit will remain consistent across standalone and SNO/MicroShift clusters.

### Implementation Details/Notes/Constraints

One of the possible future constraints might be increasing the limit with newer vSphere versions. However, we expect the
limit to be increasing rather than decreasing and making the API validation more relaxed is possible.

### Risks and Mitigations

- None.

### Drawbacks

- Increased Complexity: Introducing a new CRD field and operator logic adds complexity to the vSphere CSI driver ecosystem.
- Missing vSphere documentation: At the time of writing we don't have a clear statement or documentation to refer to
  that would well describe all the necessary details and limitations of this feature. See Documentation in
  the Proposal section for details.
- Limited Granularity: The current proposal provides a global node-level limit. More fine-grained control
  (e.g., per-namespace or per-workload limits) would require further investigation and development.

## Open Questions [optional]

None.

## Test Plan

- E2E tests will be implemented to verify the correct propagation of the configured limit to the driver pods.
  These tests will be executed only on vSphere 8.

## Graduation Criteria

- TechPreview in 4.19.

### Dev Preview -> Tech Preview

- No Dev Preview phase.

### Tech Preview -> GA

- E2E test coverage demonstrating stability.
- Available by default.
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/).
- We have to wait for VMware to GA this feature and document the configuration on vCenter side.

### Removing a deprecated feature

- No.

## Upgrade / Downgrade Strategy

- **Upgrades:** During an upgrade, the operator will apply the new API field value and update the driver pods with 
  the new `MAX_VOLUMES_PER_NODE` value. If the field is not set, default value (59) is used to match the current limit 
  for vSphere 7. So the limit will not change for existing deployments unless the user explicitly sets it.
- **Downgrades:** Downgrading to a version without this feature will result in the API field being ignored and the
  operator will revert to its previous hardcoded value configured in DaemonSet (59). If there is a higher count of
  attached volumes than the limit after downgrade, the vSphere CSI driver will not be able to attach new volumes to
  nodes and users will need to manually detach the extra volumes.

## Version Skew Strategy

There are no version skew concerns for this enhancement.

## Operational Aspects of API Extensions

- API extension does not pose any operational challenges.

## Support Procedures

* To check the status of the vSphere CSI operator, use the following command:
  `oc get deployments -n openshift-cluster-csi-drivers`.  Ensure that the operator is running and healthy, inspect logs.
* To inspect the `ClusterCSIDriver` CRs, use the following command: `oc get clustercsidriver/csi.vsphere.vmware.com -o yaml`.
  Examine the `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode` field.

## Alternatives (Not Implemented)

We considered several approaches to handle environments with mixed ESXi versions:

1. **Cluster Degradation (Selected Approach)**: 
   - We will degrade the cluster if the user-specified limit exceeds what's supported by the underlying infrastructure.
   - This requires checking the ClusterCSIDriver configuration against actual node capabilities in the `check_nodes.go` implementation.
   - The error messages will be specific about the incompatibility.
   - Documentation will clearly state that increased limits are not supported on environments containing ESXi 7.x hosts.

2. **Warning-Only Approach**:
   - Allow any user-specified limit (up to 255) regardless of ESXi versions in the cluster.
   - Emit metrics and alerts when incompatible configurations are detected.
   - This approach would result in application pods getting stuck in ContainerCreating state when scheduled to ESXi 7.0 nodes that exceed the 59 attachment limit.
   - This option was rejected as it would lead to poor user experience with difficult-to-diagnose failures.

3. **Dynamic Limit Adjustment**:
   - Have the DaemonSet controller ignore user-specified limits that exceed cluster capabilities and automatically switch to a supportable limit.
   - This option is technically complex as it would require:
     - Delaying CSI driver startup until all version checks complete
     - Implementing a DaemonSet hook to perform full cluster scans for ESXi versions (expensive operation)
     - Duplicating node checks already being performed elsewhere
   - This approach was rejected due to implementation complexity.

4. **Driver-Level Detection**:
   - Add code to the DaemonSet pod that would detect limits from BIOS or OS and consider that when reporting attachment capabilities.
   - This would require modifications to the driver code itself, which would be better implemented by VMware.
   - This approach was rejected as it would depend on upstream changes that historically have been slow to implement.

## Infrastructure Needed [optional]

- Current infrastructure needed to support the enhancement is available for testing vSphere version 8.
- To test the feature we need to a nested vSphere environment and set `pvscsiCtrlr256DiskSupportEnabled` in
  vCenter config to allow the higher volume attachment limit.
