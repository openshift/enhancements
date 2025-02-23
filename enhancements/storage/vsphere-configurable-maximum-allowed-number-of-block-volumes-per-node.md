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
  - "@deads2k"
api-approvers:
  - "@deads2k"
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
be attached to a single node. This limit is inflcuened by the number of SCSI controllers available on the node.
By default, a node can have up to four SCSI controllers, each supporting up to 15 devices, allowing for a maximum of 60
volumes per node (59 + root volume).

However, vSphere version 8 increased the maximum number of volumes per node to 256 (255 + root volume). This enhancement
aims to leverage this increased limit and provide administrators with finer-grained control over volume allocation
allowing them to configure the maximum number of block volumes that can be attached to a single node.

Details about configuration maximums: https://configmax.broadcom.com/guest?vmwareproduct=vSphere&release=vSphere%208.0&categories=3-0
Volume limit configuration for vSphere storage plug-in: https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/container-storage-plugin/3-0/getting-started-with-vmware-vsphere-container-storage-plug-in-3-0/vsphere-container-storage-plug-in-concepts/configuration-maximums-for-vsphere-container-storage-plug-in.html

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
   - This field should default to the current maximum limit of 59 volumes per node for vSphere 7.
   - API will not allow `0` value to be set `MAX_VOLUMES_PER_NODE` or allow the field to be unset. This would lead to
     disabling the limit.
   - Allowed range of values should be 1 to 255. The maximum value matches vSphere 8 limit.

3. Update CSI Node Pods:

   - After reading the new `maxAllowedBlockVolumesPerNode` API field from ClusterCSIDriver the operator will inject the
     `MAX_VOLUMES_PER_NODE` environment variable into node pods using a DaemonSet hook.
   - Any value that is statically set for the `MAX_VOLUMES_PER_NODE` environment variable in DaemonSet asset file will
     be overwritten. If the variable is omitted in the asset, DaemonSet hook will add it and set its value to the
     default defined in ClusterCSIDriver API.

4. Driver Behavior:

   - The vSphere CSI driver needs to allow the higher limit with Feature State Switch FSS (`max-pvscsi-targets-per-vm`).
   - The switch is already enabled by default in versions shipped in OpenShift 4.19.
   - The driver will report the volume limit as usual in response to `NodeGetInfo` calls.

5. Documentation:

   - Update the vSphere CSI driver documentation to include information about the new feature and how to configure it.
   - Include a statement informing users of the current requirement of having a homogeneous cluster with all nodes
     running ESXi 8 or higher. Until this requirement is met, the limit set in maxAllowedBlockVolumesPerNode must not be
     increased to a higher value than 59.

### Workflow Description

1. Administrator configures the limit:
   - The administrator creates or updates a ClusterCSIDriver object to specify the desired maximum number of volumes per
     node using the new `maxAllowedBlockVolumesPerNode` API field.
2. Operator reads configuration:
   - The vSphere CSI Operator monitors the ClusterCSIDriver object for changes.
   - Upon detecting a change, the operator reads the configured limit value.
3. Operator updates the new limit for DaemonSet:
   - The operator updates the DaemonSet for the vSphere CSI driver, injecting the `MAX_VOLUMES_PER_NODE` environment
     variable with the configured limit value into the driver node pods on worker nodes.
4. Driver reflects the limit in deployments:
   - The vSphere CSI driver checks that `max-pvscsi-targets-per-vm` FSS is set to true and reflects the limit as
     `MAX_VOLUMES_PER_NODE` environment variable and uses the configured limit during
     volume provisioning requests.

### API Extensions

- New field in ClusterCSIDriver CRD:
  - A new CRD field will be introduced to represent the maximum volume limit configuration.
  - This CRD will contain a single new field (e.g., `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode`) to define
    the desired limit.
  - The field will default to the current maximum limit of 59 volumes per node for vSphere 7.
  - The CRD will validate the value fits within the defined range (1-255).

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

- Increased Complexity: Introducing a new CRD and operator logic adds complexity to the vSphere CSI driver ecosystem.
- Potential for Configuration Errors: Incorrectly configuring the volume limit can lead to unexpected behavior
  or pod scheduling failures.
- Limited Granularity: The current proposal provides a global node-level limit. More fine-grained control
  (e.g., per-namespace or per-workload limits) would require further investigation and development.

## Open Questions [optional]

None.

## Test Plan

- E2E tests will be implemented to verify the correct propagation of the configured limit to the driver pods.
  These tests will be executed only on vSphere 8.

## Graduation Criteria

- GA in 4.19.
- E2E tests are implemented and passing. 
- Documentation is updated.

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end

### Tech Preview -> GA

- E2E test coverage demonstrating stability.
- Available by default.
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/).

### Removing a deprecated feature

- No.

## Upgrade / Downgrade Strategy

- **Upgrades:** During an upgrade, the operator will apply the new API field value and update the driver DaemonSet with 
  the new `MAX_VOLUMES_PER_NODE` value. If the field is not set, default value (59) is used to match the current limit 
  for vSphere 7. So the limit will not change for existing deployments. 
-**Downgrades:** Downgrading to a version without this feature will result in the API field being ignored and the
  operator will revert to its previous hardcoded value configured in DaemonSet (59). If there is a higher count of
  attached volumes that the limit after downgrade, the vSphere CSI driver will not be able to attach new volumes and
  to nodes and users will need to manually detach the extra volumes.

## Version Skew Strategy

There are no version skew concerns for this enhancement.

## Operational Aspects of API Extensions

- API extension does not pose any operational challenges.

## Support Procedures

* To check the status of the vSphere CSI operator, use the following command:
  `oc get deployments -n openshift-cluster-csi-drivers`.  Ensure that the operator is running and healthy, inspect logs.
* To inspect the `ClusterCSIDriver` CRs, use the following command: `oc get clustercsidriver/csi.vsphere.vmware.com -o yaml`.
  Examine the `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode` field.

## Alternatives

- We considered adding version checks to the CSI operator to prevent users setting this value incorrectly for versions
  that do not support higher limits. In order to do this we would need to check vSphere/vCenter versions and ESXi for
  every node, probably with custom webhook for validating the limit value in ClusterCSIDriver. Due to the complexity
  and low demand we do not plan to add this logic in 4.19. We might expand this enhancement in the future if needed.

## Infrastructure Needed [optional]

- Current infrastructure needed to support the enhancement is available for testing vSphere version 8.
