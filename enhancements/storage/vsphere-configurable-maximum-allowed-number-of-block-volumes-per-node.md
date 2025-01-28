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
of block volumes that can be attached to a single vSphere node. This enhancement addresses the limitations of the current driver,
which relies on a static limit based on the number of SCSI controllers available on the vSphere node. 

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

## Motivation

### User Stories

- As a vSphere administrator, I want to configure the maximum number of volumes that can be attached to a node, so that
  I can optimize resource utilization and prevent oversubscription.
- As a cluster administrator, I want to ensure that the vSphere CSI driver operates within the limits imposed by the 
  underlying vSphere infrastructure.

### Goals

- Provide administrators with granular control over volume allocation on vSphere nodes.
- Improve resource utilization and prevent oversubscription.
- Ensure compatibility with existing vSphere infrastructure limitations.
- Maintain backward compatibility with existing deployments.

### Non-Goals

- Dynamically adjust the limit based on real-time resource usage.
- Implement per-namespace or per-workload volume limits.
- Modify the underlying vSphere VM configuration.

## Proposal

1. Enable Feature State Switch (FSS):

   - Use FSS of the vSphere driver to control the activation of the maximum volume limit functionality.
   - The operator will check for vSphere version 8 (`VCenterChecker`) and conditionally set higher volume limit if version 8 or higher is detected.

2. API for Maximum Volume Limit:

   - Introduce a new field `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode` in ClusterCSIDriver API to allow administrators to configure the desired maximum number of volumes per node.
   - The vSphere CSI operator will read the configured value from the API.

3. Update CSI Node Pods:

   - If the new `maxAllowedBlockVolumesPerNode` API field is set in ClusterCSIDriver the operator will inject the `MAX_VOLUMES_PER_NODE` environment variable into node pods using a DaemonSet hook.

4. Driver Behavior:

   - The vSphere CSI driver will continue to perform basic validation on the user-defined limit, allowing the new limit of 255 volumes per node only on vSphere versions higher than 8.
   - The driver will respect the configured limit when provisioning volumes.

### Workflow Description

1. Administrator Configures Limit:
   - The administrator creates or updates a ClusterCSIDriver object to specify the desired maximum number of volumes per node.
2. Operator Reads Configuration:
   - The vSphere CSI Operator monitors the configuration object for changes.
   - Upon detecting a change, the operator reads the configured limit value.
3. Operator Updates sets the new volume limit for DaemonSet:
   - The operator updates the DaemonSet for the vSphere CSI driver, injecting the `MAX_VOLUMES_PER_NODE` environment variable with the configured limit value into the driver node pods.
4. Driver Enforces Limit:
   - The vSphere CSI driver reads the `MAX_VOLUMES_PER_NODE` environment variable and uses the configured limit during volume provisioning requests.

### API Extensions

- New field in ClusterCSIDriver CRD:
  - A new CRD field will be introduced to represent the maximum volume limit configuration.
  - This CRD will contain a single new field (e.g., `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode`) to define the desired limit.
  - The CRD should be designed with appropriate validation rules to ensure valid values are provided.

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

- Possible risk of disabling CSI controller volume publish capability: The new field in ClusterCSIDriver for setting
  limits should default to higher value than 0 (59 is reasonable, to match vSphere version 7 limit)
- Impact on existing deployments: The default limit remains unchanged, minimizing disruption for existing deployments.

### Drawbacks

- Increased Complexity: Introducing a new CRD and operator logic adds complexity to the vSphere CSI driver ecosystem.
- Potential for Configuration Errors: Incorrectly configuring the maximum volume limit can lead to unexpected behavior or resource limitations.
- Limited Granularity: The current proposal provides a node-level limit. More fine-grained control (e.g., per-namespace or per-workload limits) would require further investigation and development.

## Open Questions [optional]

None.

## Test Plan

- E2E tests will be implemented to verify the correct propagation of the configured limit to the driver pods. These tests will only run on vSphere 8.

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
  the new `MAX_VOLUMES_PER_NODE` value if it's configured. If the new field is not configured, the operator will
  keep using its previous hardcoded value configured in DaemonSet (59).
-**Downgrades:** Downgrading to a version without this feature will result in the API field being ignored and the
  operator will revert to its previous hardcoded value configured in DaemonSet (59). If there is a higher count of
  attached volumes that the limit after downgrade, the vSphere CSI driver will not be able to attach new volumes and
  users will need to manually detach the extra volumes.

## Version Skew Strategy

There are no version skew concerns for this enhancement.

## Operational Aspects of API Extensions

- API extension does not pose any operational challenges.

## Support Procedures

* To check the status of the vSphere CSI operator, use the following command: `oc get deployments -n openshift-cluster-csi-drivers`.  Ensure that the operator is running and healthy, inspect logs.
* To inspect the `ClusterCSIDriver` CRs, use the following command: `oc get clustercsidriver/<driver_name> - yaml`.  Examine the `spec.driverConfig.vSphere.maxAllowedBlockVolumesPerNode` field.

## Alternatives

- We could conditionally set FSS with the operator based either on presence of the new field or feature gate enablement in OpenShift.
  This should not be necessary as the FSS in the driver only allows setting higher volume limit (255) per node.

## Infrastructure Needed [optional]

- Current infrastructure needed to support the enhancement is available for testing vSphere version 8.
