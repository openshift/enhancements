---
title: volume-data-source-validator
authors:
   - "@RomanBednar"
reviewers:
   - "@jsafrane"
approvers:
   - "@bbennett"
api-approvers:
   - "None"
creation-date: 2025-04-29
last-updated: 2025-04-29
tracking-link:
   - "https://issues.redhat.com/browse/STOR-1064"
see-also:
   - "None"
replaces:
   - "None"
superseded-by:
   - "None"
---

# Volume Data Source Validator

## Summary

This enhancement describes the onboarding of the Kubernetes upstream Volume Data Source Validator component to OpenShift.
The validator is a critical part of the Volume Populators feature, which allows pre-populating PersistentVolumes with
data from arbitrary sources. The validator component ensures that PersistentVolumeClaims (PVCs) referencing custom
populators via the `dataSourceRef` field are properly validated, providing clear error feedback to users.

## Motivation

Kubernetes 1.33 promoted the Volume Populators feature (feature gate `AnyVolumeDataSource`) to GA status. While the
feature will work without the validator component, users would miss important validation checks and explicit error
reporting. The validator serves three important functions:

1. Enables discoverability of registered populators
2. Provides warning events when PVCs reference populators that aren't registered
3. Exposes operational metrics for monitoring validation activities, including counts of valid, invalid, and error cases
   during data source validation

Without this validator, invalid references would silently fail, leading to PVCs remaining in a perpetual "Pending" state
without meaningful error feedback to users.

The registration is done by simply creating a `VolumePopulator` resource that references custom resources used by a 
volume populator, for example: 

```
kind: VolumePopulator
apiVersion: populator.storage.k8s.io/v1beta1
metadata:
  name: hello-populator
sourceKind:
  group: hello.example.com
  kind: Hello
 ```

### Goals

- Deploy the Volume Data Source Validator component in OpenShift clusters
- Install the `VolumePopulator` Custom Resource Definition (CRD)
- Enable validation of PVC `dataSourceRef` fields against registered populators
- Provide users with clear error messages for invalid populator references


### Non-Goals

- Implementing or managing specific volume populators
- Integrating validation status reporting with OpenShift's monitoring system
- Modifying how the Volume Populators feature works upstream

## Proposal

The Cluster Storage Operator (CSO) will be responsible for deploying the Volume Data Source Validator controller and
creating the required `VolumePopulator` CRD. The controller will be deployed in all 4.20 OpenShift clusters or newer.

### User Stories

#### Story 1: Using valid Volume Populators

A user creates a PVC with a `dataSourceRef` field pointing to a registered populator custom resource. The validator
confirms the reference is valid and does not emit any error events. The PVC is successfully populated with data.

#### Story 2: Attempting to use invalid Volume Populators

A user creates a PVC with a `dataSourceRef` field pointing to an unregistered populator custom resource. The validator
detects the invalid reference and adds warning events to the PVC, informing the user that the referenced populator is
not registered. This helps the user quickly identify and resolve the issue. Note that the validator does not actively
block the populator if it's not registered.

### Implementation Details

The implementation will involve:

1. Creating a fork of the [kubernetes-csi/volume-data-source-validator](https://github.com/kubernetes-csi/volume-data-source-validator) repository
2. Defining CI jobs in Prow
3. Onboarding the validator with ART.
4. Modifying the Cluster Storage Operator to:
   - Create the `VolumePopulator` CRD
   - Deploy the validator controller in the `openshift-cluster-storage-operator` namespace
   - Configure appropriate RBAC permissions for the validator component

### Risks and Mitigations

**Risk**: The validator could interfere with existing volume populator implementations.

**Mitigation**: Existing implementations that do not use the `VolumePopulator` CRD will continue to work as before, but
the validator will start to emit events after deployment. We will coordinate with teams that use or could use volume
populators to ensure smooth integration.

**Risk**: The VolumePopulator CRD deployment could conflict with CRDs in existing clusters.

**Mitigation**: The feature is relatively new so we don't expect wide adoption. We also checked metrics from existing 
clusters and have not found any online clusters with this CRD deployed.

**Risk**: The validator could cause additional resource overhead in the cluster.

**Mitigation**: The validator's resource footprint is minimal. It only actively processes PVCs with `dataSourceRef`
fields and maintains a small watch on `VolumePopulator` resources.

## Design Details

### Test Plan

Testing will involve:

1. Validator unit tests
2. E2E tests for validator functionality

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

- Ability to utilize the validator to validate PVC data sources
- Gathered feedback from users
- Addressing issues observed during tech preview

### Upgrade / Downgrade Strategy

On upgrade from a pre-validator version, the Cluster Storage Operator will deploy the validator components, PVCs with
`dataSourceRef` fields referencing custom populators will begin to be validated.

On downgrade, the validator components will be removed, and validation will no longer occur. This may result in PVCs
with invalid `dataSourceRef` fields silently failing rather than receiving explicit error messages.

### Version Skew Strategy

Version skew between the validator and the API server should not cause issues as the validator only reads PVC and
`VolumePopulator` resources without modifying their API fields.

## Implementation History

- 2025-04-29: Initial enhancement proposal

## Drawbacks

The main drawback is the introduction of another component to maintain and potential for increased complexity in the PVC
provisioning flow. However, these drawbacks are outweighed by the benefits of proper validation and clear error reporting.

## Alternatives

### Using OLM for Validator Deployment

An alternative approach would be to use the Operator Lifecycle Manager (OLM) to deploy the validator. This would require:

1. Creating an Operator for the validator
2. Publishing the Operator to the OpenShift OperatorHub
3. Making it a dependency for any Operator that implements volume populators

This approach was considered but rejected because:

1. It would require every Operator that implements populators to explicitly depend on the validator operator
2. It complicates the installation process for OLM operator developers
3. CSO is already responsible for other storage-related components, making it a natural fit for the validator

## Infrastructure Needed

- No additional infrastructure needs
