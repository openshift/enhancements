---
title: storage-class-encrypted
authors:
  - "@patrickdillon"
reviewers:
  - "@jhixson74, installer"
  - "@jsafrane, storage"
  - "@bertinatto, storage"
  - "@trilokgeer, storage"
approvers:
  - "@jsafrane"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@joelspeed"
creation-date: 2022-06-22
last-updated: 2022-12-09
tracking-link:
  - https://issues.redhat.com/browse/CORS-2049
  - https://issues.redhat.com/browse/OCPPLAN-9307
---

# Default Storage Class with Encrypted Keys 

## Summary

When user-managed encryption keys are provided at install time, the default storage
class should be set to utilize those keys. Presently, only root volumes for nodes are
encrypted with those keys--but users would expect the persistent storage volumes to
be encrypted as well. This enhancement proposes a solution for AWS, GCP, & Azure.

## Motivation

The primary motivation is to enable managed services to encrypt storage volumes with
customer-provided keys automatically as part of the installation process without requiring
day-2 operations. The behavior of automatically setting the default storage class is
sensible for individual (non-managed-service) users as well; setting the default will
reduce their day-2 burden and decrease the opportunity for user error.

### User Stories

When I install a cluster, I want to provide user-managed encryption keys in the install-config
so that the default storage class will be set to provision volumes with those keys.

### Goals

* Default storage class is set to use user-provided keys at cluster creation for public clouds (AWS, GCP, Azure)

### Non-Goals
* Other clouds besides AWS, GCP, & Azure are out of scope


## Proposal

### Workflow Description

1. Cluster creator specifies (aws|gcp|azure) user-managed encryption keys 
in `defaultMachinePlatform` of install config.
2. Installer creates a manifest for `ClusterCSIDriver` which includes encryption keys in spec.
3. Cluster-storage-operator/CSI drivers use installer-created `ClusterCSIDriver`
to create default storage class using user-encrypted keys.


### API Extensions

This enhancement extends the [`ClusterCSIDriver` API object](https://github.com/openshift/api/blob/efeef9d83325a0ba12a636e5efd7d9414036f9f7/operator/v1/types_csi_cluster_driver.go).

AWS, Azure & GCP will be added as new valid values for the `CSIDriverType` union discriminator
in the CSIDriverConfigSpec:

```go
// CSIDriverType indicates type of CSI driver being configured.
// +kubebuilder:validation:Enum=AWS;Azure;GCP;vSphere
type CSIDriverType string

const (
  AWSDriverType     CSIDriverType = "AWS"
  AzureDriverType   CSIDriverType = "Azure"
  GCPDriverType     CSIDriverType = "GCP"
  VSphereDriverType CSIDriverType = "vSphere" // existing platform
)

// CSIDriverConfigSpec defines configuration spec that can be
// used to optionally configure a specific CSI Driver.
// +union
type CSIDriverConfigSpec struct {
  // driverType indicates type of CSI driver for which the
  // driverConfig is being applied to.
  // Valid values are: AWS, Azure, GCP, vSphere.
  // Consumers should treat unknown values as a NO-OP.
  // +kubebuilder:validation:Required
  // +unionDiscriminator
  DriverType CSIDriverType `json:"driverType"`

  // AWS is used to configure the AWS CSI driver.
  // +optional
  AWS *AWSCSIDriverConfigSpec `json:"aws,omitempty"`

  // Azure is used to configure the Azure CSI driver.
  // +optional
  Azure *AzureCSIDriverConfigSpec `json:"azure,omitempty"`

  // GCP is used to configure the GCP CSI driver.
  // +optional
  GCP *GCPCSIDriverConfigSpec `json:"gcp,omitempty"`

  ... exisiting vsphere field ... 
}
```

 and fields for the encryption keys will be added
to platform-specific config specs:

#### AWS

```go
// AWSCSIDriverConfigSpec defines properties that can be configured for the AWS CSI driver.
type AWSCSIDriverConfigSpec struct {
    // kmsKeyARN sets the cluster default storage class to encrypt volumes with a user-defined KMS key,
    // rather than the default KMS key used by AWS.
    // The value may be either the ARN or Alias ARN of a KMS key.
    // +kubebuilder:validation:Pattern:=arn:(aws|aws-cn|aws-us-gov):kms:[a-z0-9]+(-[a-z0-9]+)*:[0-9]{12}:(key|alias)/.*
    // +optional
    KMSKeyARN string `json:"kmsKeyARN,omitempty"`
}
```

#### Azure

```go
// AzureDiskEncryptionSet defines the configuration for a disk encryption set.
type AzureDiskEncryptionSet struct {
  // subscriptionID defines the Azure subscription that contains the disk encryption set.
  // When omitted, the subscription from the authenticating credentials of the CSI driver
  // will be used.
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:Pattern:=^[a-z0-9]{8}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{12}$
  SubscriptionID string `json:"subscriptionID"`
  
  // resourceGroup defines the Azure resource group that contains the disk encryption set.
  // When omitted, the cluster resource group from the cluster infrastructure object
  // will be used.
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:Pattern=^[-\w\._\(\)]+$
  ResourceGroup string `json:"resourceGroup"`
  
  //name is the name of the disk encryption set that will be set on the default storage class.
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:MaxLength:=80
  // +kubebuilder:validation:Pattern:=[a-zA-Z0-9_-]
  Name string `json:"name"`
}

// AzureCSIDriverConfigSpec defines properties that can be configured for the Azure CSI driver.
type AzureCSIDriverConfigSpec struct{
  // diskEncryptionSet sets the cluster default storage class to encrypt volumes with a
  // customer-managed encryption set, rather than the default platform-managed keys.
  // +optional
  DiskEncryptionSet *AzureDiskEncryptionSet `json:"diskEncryptionSet,omitempty"`
}

```

#### GCP

```go
// GCPKMSKeyReference gathers required fields for looking up a GCP KMS Key
type GCPKMSKeyReference struct {
  // name is the name of the customer-managed encryption key to be used for disk encryption.
  // The value should correspond to an existing KMS key
  // and input should match the regular expression: [a-zA-Z0-9_-]{1,63}.
  // +kubebuilder:validation:Pattern:=^[a-zA-Z0-9_-]$
  // +kubebuilder:validation:MinLength:=1
  // +kubebuilder:validation:MaxLength:=63
  // +kubebuilder:validation:Required
  Name string `json:"name"`
  
  // keyRing is the name of the KMS Key Ring which the KMS Key belongs to.
  // The value should correspond to an existing KMS key ring
  // and input should match the regular expression: [a-zA-Z0-9_-]{1,63}.
  // +kubebuilder:validation:Pattern:=^[a-zA-Z0-9_-]$
  // +kubebuilder:validation:MinLength:=1
  // +kubebuilder:validation:MaxLength:=63
  // +kubebuilder:validation:Required
  KeyRing string `json:"keyRing"`
  
  // projectID is the ID of the Project in which the KMS Key Ring exists.
  // It must be 6 to 30 lowercase letters, digits, or hyphens.
  // It must start with a letter. Trailing hyphens are prohibited.
  // Defaults to the cluster project defined in the cluster infrastructure object, if not set.
  // +kubebuilder:validation:Pattern:=^[a-z][a-z0-9-]+[a-z0-9]$
  // +kubebuilder:validation:MinLength:=6
  // +kubebuilder:validation:MaxLength:=30
  // +optional
  ProjectID string `json:"projectID,omitempty"`
  
  // location is the GCP location in which the Key Ring exists.
  // The input must match an existing GCP location, or "global".
  // Defaults to global, if not set.
  // +kubebuilder:validation:Pattern:=[a-zA-Z0-9_-]
  // +optional
  Location string `json:"location,omitempty"`
}

// GCPCSIDriverConfigSpec defines properties that can be configured for the GCP CSI driver.
type GCPCSIDriverConfigSpec struct {
    // kmsKey sets the cluster default storage class to encrypt volumes with customer-supplied
    // encryption keys, rather than the default keys managed by GCP. 
    // +optional
    KMSKey *GCPKMSKeyReference `json:"kmsKey,omitempty"`
}
```

#### Install Config

Inputs corresponding to the API extensions already exist in the Installer install config: 
[AWS](https://github.com/openshift/installer/blob/master/pkg/types/aws/machinepool.go#L100), 
[GCP](https://github.com/openshift/installer/blob/master/pkg/types/gcp/machinepools.go#L73), &
[Azure](https://github.com/openshift/installer/blob/master/pkg/types/azure/disk.go#L36).

### Implementation Details/Notes/Constraints

The Installer will create a `ClusterCSIDriver` manifest when users have defined
encryption keys on the `defaultMachinePlatform` (`installconfig.platform.(aws|azure|gcp).defaultMachinePlatform`). The Installer will only conditionally create the `ClusterCSIDriver`
manifest, the default resource should be created by the cluster-storage operator in its absence.

1. User adds encryption keys to the `defaultMachinePlatform` in the install config.
2. Installer checks if `defaultMachinePlatform` has encryption keys defined, sets them in the `ClusterCSIDriver` manifest.
3. `ClusterCSIDriver` object is created during install.
4. cluster-storage-operator/CSI driver monitors `ClusterCSIDriver` and creates default storage class
accordingly


### Risks and Mitigations

No risks foreseen. 

### Drawbacks

These tasks could be handled day-2 by a user. The largest drawback would be in terms of the added complexity required to
implement this feature when that could be handled day 2 by a user. The complexity of this task is quite small and seems
reasonable, so this drawback does not seem like a reason to block this enhancement.

## Design Details

### Open Questions 

None

### Test Plan

If desired, presubmit and/or periodic jobs could follow this pattern:
1. In pre-steps, create or use existing keys to populate install config.
2. Create cluster
3. Add a step to ensure default storage class uses specified keys.

### Graduation Criteria

#### Dev Preview -> Tech Preview


#### Tech Preview -> GA


**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature


### Upgrade / Downgrade Strategy

After upgrade, users should be able to utilize the new API fields to have the cluster-storage-operator or CSI driver
set the default storage class. Presumably, the operator/driver would do nothing if the fields are unset; so the
upgrade should be safe.

### Version Skew Strategy

Version skew should not be an issue for this enhancement.

### Operational Aspects of API Extensions

- Sets default storage class when populated on cluster infrastructure object

#### Failure Modes

- If bad data is entered into the cluster infrastructure object for this use case, the operator/driver should fail
to set the default storage class. 

#### Support Procedures

- The cluster-storage-operator/driver should validate the existence/availability of the keys
- Failure of validation should appear in logs and stop further action

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Instead of using the existing fields in the machinepools of the install config, it would be possible to define a new
top-level field to define a default storage class. I have decided against this idea, because it has a broader scope
and this proposal does not preclude the future introduction of such a field. 

### API Alternatives

The original enhancement suggested adding these fields to the cluster infrastructure object. These fields are only
being used to set the default storage class, therefore they should be added to the more specific `ClusterCSIDriver`
config rather than the more general cluster infrastrcture object.

## Infrastructure Needed

Users and testers will need to be able to create or gain access to user-managed encryption keys in AWS, GCP, & Azure.