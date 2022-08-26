---
title: csi-ebs-operator
authors:
  - "@gnufied"
reviewers:
  - "@jsafrane”
  - "@fbertina"
  - "@chuffman"
approvers:
  - "@..."
creation-date: 2020-10-05
last-updated: 2020-10-05
status: implementable
see-also: https://github.com/openshift/enhancements/blob/master/enhancements/storage/csi-driver-install.md
replaces:
superseded-by:
---

# CSI Driver operator for vSphere

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes deployment of vSphere CSI driver on Openshift as a default component.

## Motivation

* vSphere is a key cloud provider for OpenShift and is supported in Openshift 3.x and 4.x. We need it to be supported and available even if in-tree drivers have been removed.
* vSphere CSI driver provides new features such as - volume expansion, snapshotting and cloning which were previously unavailable with intree driver.

### Goals

* Create an operator to install the vSphere CSI driver.
* Publish the driver and operator via default OCP build pipeline.
* Enable creation of vSphere CSI storageclass, so as OCP users can start consuming vSphere CSI volumes without requiring further configuration.

### Non-Goals

* We don't intend to create a brand new driver with this KEP but merely want to deploy and configure upstream vSphere CSI driver - https://github.com/kubernetes-sigs/vsphere-csi-driver/


## Proposal

We propose that Openshift will ship with  vmware-vsphere-csi-driver-operator by default which is managed by [cluster-storage-operator](https://github.com/openshift/cluster-storage-operator/).

## Design Details

vSphere CSI driver has few dependencies on installer though and they are:

### Installer dependency

* vSphere CSI driver requires HW version 15 on VMs that make up OCP cluster. Currently the rhcos OVA file Red Hat ships has default HW version configured to 13 and hence VM version should be updated.
* Configuration of vSphere StorageClass requires knowledge of storage policy that was created in vCenter. Without this information - it is not possible to create a working storageClass for CSI driver.

#### HW version of OCP VMs

As mentioned above - vSphere CSI driver requires HW version 15 on all the VMs that make OCP cluster. Since, Openshift defaults to HW version 13 when a vSphere OCP cluster gets created - vSphere CSI driver isn't workable on the OCP cluster by default.

To solve this following alternatives were considered:

* We could provide manual instructions to the user about updating hardware version of their VMs. This won't be automatic but in some cases not even possible because some existing VMs can't be upgraded to HW version 15 in-place and hence this option is ruled out.
* We could update installer to set HW version 15 - when appropriate hypervisor version(6.7u3 or higher) is detected.
* For UPI install - we can additionally document required HW version while creating VMs.

### Deployment strategy

vSphere CSI driver operator will be deployed by [cluster-storage-operator](https://github.com/openshift/cluster-storage-operator/)

#### Operator deployment via cluster-storage-operator

The cluster-storage-operator will deploy all the resources necessary for creating the vSphere CSI driver operator.

1. vSphere CSI driver operator will be deployed in namespace `openshift-cluster-csi-drivers`.
2. A service account will be created for running the operator.
3. The operator will get RBAC rules necessary for running the operator and its operand (i.e the actual vSphere CSI driver).
4. A deployment will be created for starting the operator and will be managed by cluster-storage-operator.
5. A instance of `ClusterCSIDriver` will be created to faciliate managment of driver operator.  `ClusterCSIDriver` is already defined in - https://github.com/openshift/api/blob/master/operator/v1/types_csi_cluster_driver.go but needs to be expanded to include vSphere CSI driver.
6. cluster-storage-operator will request CVO to create required cloud-credentials for talking with vCenter API.

#### Driver deployment via vmware-vsphere-csi-driver-operator

The operator itself will be responsible for running the driver and all the required sidecars (attacher, provisioner etc).

1. The operator will assume namespace `openshift-cluster-csi-drivers` is already created for the driver.
2. A service account will be created for running the driver.
3. The operator will create RBAC rules necessary for running the driver and sidecars.
4. A deployment will be created and managed by operator to handle control-plane sidecars and controller-plane driver deployment with controller-side of CSI services.
5. A DaemonSet will be created and managed by operator to run node side of driver operations.
6. A `CSIDriver` object will be created to expose driver features to control-plane.
7. The driver operator will use and expose cloud-credentials created by CVO.

Most of the steps outlined above is common to all CSI driver operator and vSphere CSI driver is not unique among those aspects.

#### Additional consideration for vSphere

There are certain aspects of driver which require special handling:

##### StoragePolicy configuration

Currently while deploying Openshift a user can configure datastore used by OCP via install-config.yaml. vSphere CSI driver however can't use datastore directly and must be configured with vSphere storage policy.

To solve this problem vsphere CSI operator is going to create a storagePolicy by tagging selected datastore in the installer. This will require OCP to have expanded permissions of creating storagePolicies. After creating the storagePolicy, the vSphere CSI operator will also create corresponding storageclass.


##### Hardware and vCenter version handling

When vSphere CSI operator starts, using credentials provided by cluster-storage-operator, it will first verifiy vCenter version and HW version of VMs.
If vCenter version is not >= 6.7u3 or HW version is not 15 or greater on all VMs and this is a fresh install(i.e there is no `CSIDriver` object
already installed by this operator) - it will stop installation of vSphere CSI driver operator and periodically retry with exponential backoff.

Since vsphere-problem-detector also runs similar checks in 4.10 - our plan is to move those checks to CSI driver operator and if those checks fail
mark cluster as un-upgradeable. In 4.11 we are considering removal of vsphere-problem-detector altogether and brining all checks in CSI driver operator.

However, if additional VMs are added later into the cluster and they do not have HW version 15 or greater, Operator will mark itself as `degraded`. The vSphere CSI
driver operator will use presence of `CSIDriver` object with openshift annotation as indication that driver was installed previously.

###### Error handling during creation of storage policy and storage class

If CSI operator can not create storage Policy or storageClass for some reason:

Cluster will be degraded when:
- We can't talk to Kube API server and for some reason can't read secret or can't create storageclass etc.
- All errors that previously only marked cluster as "un-upgradeable" will be upgraded to *degrade* the cluster if we previously installed CSI driver (detected via presence of `CSIDriver` object).

Cluster will be marked as un-upgradable when:
- There are any vCenter related errors (connection refused, 503 errors or permission denied) or any of the checks fail (HW version, esxi verion checks).

In case operator is marked as un-upgradeable for some reason - detailed information will be added to `ClusterCSIDriver` object and an appropriate metric will be emitted.
It should be noted that even though cluster is marked as un-upgradeable, cluster is still fully available and `Available=True` will be set for all cluster objects
and cluster can still be upgraded between z-stream versions but upgrades to minor versions (such as `4.11`) will be blocked.

##### Presence of existing drivers in the cluster being upgraded

A customer may have installed vSphere driver from external sources. In 4.10 we will install the CSI driver operator but will not proceed with driver
install if an existing install of CSI driver is detected. The mechanism for detecting existing upstream driver will be:

1. Check if there is a `CSIDriver` object of type vSphere CSI driver.
2. Next we will check there is one or more `CSINode` objects with vSphere CSI driver type.

If we detect an existing driver present in the cluster then cluster will be marked "unupgradable". Following steps must be performed to migrate to red hat version of driver:

1. Cluster Admin will delete CSI driver Deployment object.
2. Cluster Admin will delete CSI driver Daemonset objects.
3. In last step Cluster Admin will delete `CSIDriver` and any existing configmap, secrets objects that were installed previously for functioning of CSI driver.

We will ensure that these steps are documented in 4.10 docs.

#### Configure CSI driver topology

A customer may configure a topology for the CSI driver by using following fields in `ClusterCSIDriver` object:

```go
// ClusterCSIDriverSpec is the desired behavior of CSI driver operator
type ClusterCSIDriverSpec struct {
    ...
    ...
    // driverConfig can be used to specify platform specific driver configuration.
    // When omitted, this means no opinion and the platform is left to choose reasonable
    // defaults. These defaults are subject to change over time.
    // +optional
    DriverConfig CSIDriverConfigSpec `json:"driverConfig"`
}

// CSIDriverConfigSpec defines configuration spec that can be
// used to optionally configure a specific CSI Driver.
// +union
type CSIDriverConfigSpec struct {
    // driverType indicates type of CSI driver for which the
    // driverConfig is being applied to.
    //
    // Valid values are:
    //
    // * vSphere
    //
    // Allows configuration of vsphere CSI driver topology.
    //
    // ---
    // Consumers should treat unknown values as a NO-OP.
    //
    // +kubebuilder:validation:Required
    // +unionDiscriminator
    DriverType CSIDriverType `json:"driverType"`

    // vsphere is used to configure the vsphere CSI driver.
    // +optional
    VSphere *VSphereCSIDriverConfigSpec `json:"vSphere,omitempty"`
}

// VSphereCSIDriverConfigSpec defines properties that
// can be configured for vsphere CSI driver.
type VSphereCSIDriverConfigSpec struct {
    // topologyCategories indicates tag categories with which
    // vcenter resources such as hostcluster or datacenter were tagged with.
    // If cluster Infrastructure object has a topology, values specified in
    // Infrastructure object will be used and modifications to topologyCategories
    // will be rejected.
    // +optional
    TopologyCategories []string `json:"topologyCategories,omitempty"`
}
```

Specifying topology as day-2 operation will not affect any of existing PVs in the cluster and they will remain with whatever topology configuration created with. Also changing the topology from a different value will not affect any of existing PVs. They will remain with whatever topology they were created with.

Specifying topology here will cause CSI driver to be deployed with topology configuration as specified in https://docs.vmware.com/en/VMware-vSphere-Container-Storage-Plug-in/2.0/vmware-vsphere-csp-getting-started/GUID-162E7582-723B-4A0F-A937-3ACE82EAFD31.html .

If customer had configured upstream CSI driver with an topology, they must ensure that they are using same values for CSI driver configuration here.

The intent of this change is compatible with changes introduced in - https://github.com/openshift/enhancements/pull/918 . If `driverConfig` is unspecified,
the CSI driver deployment in new clusters will default to platform specific defaults which will come from `Infrastructure` object if configured as defined in
enhancement#918.


The values specified in `Infrastructure` object will take precedence over user configuration (and user specified changes will be wiped away). Users can only
configure `TopologyCategories` categories if OCP platform configuration does not specify a topology and user would like to use one for CSI driver.

#### Disabling the operator

Currently disabling the operator is unsupported feature.

### API Extensions

The operator will use https://github.com/openshift/api/blob/master/operator/v1/types_csi_cluster_driver.go for operator configuration and managment.

### Operational Aspects of API Extensions

The `ClusterCSIDriver` type is used for operator installation and reporting any errors. Please see [CSI driver installation](csi-driver-install.md) for
more details.

### Test Plan

The operator will be tested via CSI driver e2e. We don't expect operator itself to have any e2e but it will have unit tests that validate specific behavior.

### User Stories

#### Story 1

### Implementation Details/Notes/Constraints

### Risks and Mitigations

* We don't yet know state of vSphere CSI driver. We need to start running e2e tests with vSphere driver as early as possible, so as we can determine how stable it is.
* We have some concerns about datastore not being supported in storageClass anymore. This means that in future when in-tree driver is removed, clusters without storagePolicy will become unupgradable.

#### Failure Modes

If operator is unable to install CSI driver or is failing for some reason, appropriate condition will be added to `ClusterCSIDriver` object.

#### Support Procedures

Supported by must-gather logs and metrics.

### Graduation Criteria

#### Dev Preview -> Tech Preview

There is no dev-preview phase.

#### Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed

* vmware-vsphere-csi-driver GitHub repository (forked from upstream).
* vmware-vsphere-csi-driver-operator GitHub repository.
* vmware-vsphere-csi-driver and vmware-vsphere-csi-driver-operator images.
