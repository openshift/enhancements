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

OCP ships with a vsphere-csi-driver-operator by default which is managed by [cluster-storage-operator](https://github.com/openshift/cluster-storage-operator/).
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
6. cluster-storage-operator will create required cloud-credentials for talking with vCenter API.

#### Driver deployment via vsphere-csi-driver-operator.

The operator itself will be responsible for running the driver and all the required sidecars (attacher, provisioner etc).

1. The operator will create namespace `openshift-cluster-csi-drivers` to deploy the driver and sidecars.
2. A service account will be created for running the driver.
3. The operator will create RBAC rules necessary for running the driver and sidecars.
4. A deployment will be created and managed by operator to handle control-plane sidecars and controller-plane driver deployment with controller-side of CSI services.
5. A DaemonSet will be created and managed by operator to run node side of driver operations.
6. A `CSIDriver` object will be created to expose driver features to control-plane.
7. The driver operator will use and expose cloud-credentials created by cluster-storage-operator.

Most of the steps outlined above is common to all CSI driver operator and vSphere CSI driver is not unique among those aspects.

#### Additional consideration for vSphere

There are certain aspects of driver which require special handling:

##### StoragePolicy configuration

Currently while deploying Openshift a user can configure datastore used by OCP via install-config.yaml. vSphere CSI driver however can't use datastore directly and must be configured with vSphere storage policy. 

To solve this problem vsphere CSI operator is going to create a storagePolicy by tagging selected datastore in the installer. This will require OCP to have expanded permissions of creating storagePolicies.

If CSI operator can not create storage Policy for some reason:

- The operator will continue installing CSI driver without creating storageClass. The operation of creating storagePolicy and storageClass will be however retried expecting permissions to be fixed or transient errors to go away.
- It is expected that user will manually create a storageClass to consume vSphere CSI volumes if CSI operator can not create storagePolicy and storageClass.
- CSI operator will mark itself as degraded but CSO itself will not be degraded and vSphere CSI operator's degraded status must be handled in a different way in CSO.
- Create a metric if storagePolicy and storageClass creation fails in 4.7.

##### Hardware and vCenter version handling

When vSphere CSI operator starts, using credentials provided by cluster-storage-operator, it will first verifiy vCenter version and HW version of VMs. If vCenter version is not 6.7u3 or greater and HW version is not 15 or greater on all VMs - it will mark itself as degraded but continue with driver installation. If additional VMs are added later into the cluster and they do not have HW version 15 or greater, Operator will mark itself as `degraded` and nodes which don't have right HW version will have annotation `vsphere.driver.status.csi.openshift.io: degraded` added to them.

Additionally vSphere CSI operator will report HW version of VMs that make up the cluster as a metric.

##### Presence of existing drivers in the cluster being upgraded

A customer may install vSphere driver from external sources. In 4.7 we will install the CSI driver operator but will not proceed with driver install if an existing install of CSI driver is detected. The operator will be marked as degraded but it will not degrade overall CSO status.

Since VMWare will ship its own vSphere CSI driver operator via OLM, it should be possible to disable default cluster-storage-operator managed operator(i.e this operator). Currently we are planning to let user set `ManagementState: Removed` in `ClusterCSIDriver` CR, which will cause operand to be removed(i.e the CSI driver) but operator will still be running. This will allow user to use external vSphere CSI operatrs.

#### Disabling the operator

#### API

The operator will use https://github.com/openshift/api/blob/master/operator/v1/types_csi_cluster_driver.go for operator configuration and managment.

### User stories

#### Story 1

### Implementation Details/Notes/Constraints

### Risks and Mitigations

* We don't yet know state of vSphere CSI driver. We need to start running e2e tests with vSphere driver as early as possible, so as we can determine how stable it is.
* We have some concerns about datastore not being supported in storageClass anymore. This means that in future when in-tree driver is removed, clusters without storagePolicy will become unupgradable.

### Test plan

* We plan to enable e2e for vSphere CSI driver.

### Graduation Criteria

There is no dev-preview phase.

##### Tech Preview

##### Tech Preview -> GA

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed

* vsphere-csi-driver GitHub repository (forked from upstream).
* vsphere-csi-driver-operator GitHub repository.
* vsphere-csi-driver and vsphere-csi-driver-operator images.
