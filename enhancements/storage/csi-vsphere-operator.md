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

* We could provide manual instructions to the user about updating hardware version of their VMs. This won't be automatic but in some cases not even possible because some existing VMs can't be upgraded to HW version 15 in-place.
* We could update installer to set HW version 15 - when appropriate hypervisor version(6.7u3 or higher) is detected.
* For UPI install - we can additionally document required HW version while creating VMs.

#### StoragePolicy configuration

Currently while deploying Openshift a user can configure datastore used by OCP via install-config.yaml. vSphere CSI driver however can't use datastore directly and must be configured with vSphere storage policy. This can be done by an optional field in `Infrastructure` type of installer and if populated vsphere CSI driver operator will create storageClass with selected vSphere storage policy.

### Deployment strategy

vSphere CSI driver operator will be deployed by [cluster-storage-operator](https://github.com/openshift/cluster-storage-operator/)
