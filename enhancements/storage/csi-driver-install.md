---
title: CSI driver installation for in-tree drivers
authors:
  - "@gnufied"
  - “@jsafrane”
reviewers:
  - "@eparis”
approvers:
  - TBD
  - "@..."
creation-date: 2019-11-04
last-updated: 2019-11-04
status: provisional
see-also:
replaces:
superseded-by:
---

# Installation of CSI drivers that replace in-tree drivers

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/**

## Summary

We want certain CSI drivers such as AWS, GCE, Cinder, Azure and vSphere to be installable on Openshift, so as
they can be used along-side in-tree drivers and when upstream enables migration flag for these volume types, their
replacement CSI drivers can take over and none of storage features get affected.

## Motivation

Upstream Kubernetes is moving towards removing code of in-tree drivers and replacing them with their CSI counterpart. Our
current expectation is that - all in-tree drivers that depend on cloudprovider should be removed from core Kubernetes by 1.21.
This may not happen all at once and we expect migration for certain drivers to happen sooner.

This does mean that - Openshift should be prepared to handle such migration. We have to iron out any bugs in driver themselves and
their interfacing with Openshift. We need a way for users to use the CSI drivers and optionally enable migration from in-tree driver
to CSI driver. To support upstream design - we will also need a way for users to disable the migration and keep using in-tree driver, until
in-tree code is finally removed.

## Goals

We would like to approach various goals of this KEP in different phases, because we do not expect to get everything right or done in phase-1.

### Phase-1 Goals

* Provide a way for AWS CSI driver installation starting from Openshift-4.4. Users could either optionally install it or CSI driver could be installed
along with in-tree driver and users could use both.
* Install CSI provided storageclass along with in-tree StorageClass.

### Phase-2 Goals

* Support installation of GCE, Cinder, vSphere, AzureDisk CSI drivers.
* Provide a way for users to enable in-tree to CSI migration.
* Provide a way for users to disable in-tree to CSI migration.

### Phase-3 Goals

* Enable CSI drivers as default drivers and CSI provided storageclass as default storageclass.
* Optionally allow users to configure CSI driver install.

## Non-Goals

* This KEP does not attempt to design installation of third-party or partner CSI drivers.

## Proposal

We are currently considering two options for installation of CSI driver.

### Installation via OLM

We propose that - we provide each driver mentioned above as a separate operator which could be subscribed and installed via OLM UI. Each driver's operator
is responsible for its installation and release. The operator is responsible for creating storageclass that the driver provides.

The configuration of CSI driver can be done via OLM UI if required and CSI driver can access cloudprovider credentials from Openshift provided sources.

Installation via OLM however means that, when we want to enable these CSI drivers as default drivers, they must be installed by default in Openshift installs.
We further propose that - Cluster Storage Operator(https://github.com/openshift/cluster-storage-operator) could create subscriptions for these driver operators when drivers have to be installed by default.

There are pros and cons to this approach and since this KEP is in its first iteration, we would like to seek feedback from other stakeholders.

Pros:
1. It is simple to create a opt-in installable driver operator.
2. Providing an optional UI for configuration via OLM is a plus.
3. Each driver's operator sits in its own repository and managing releases could be simpler.

Cons:
1. When these CSI drivers have to be installed by default, this approach causes a coupling between CVO managed cluster-storage-operator and driver's operator.
2. In general there are some concerns about OLM's lack of documentation and everyone's understanding of it.

### Installation via CVO

We are also considering if these 5 CSI drivers should be installed by default and managed via CVO. We are considering using cluster-storage-operator to detect
cloudprovider on which cluster is running and install necessary CSI driver by default. This does mean that, CSI driver is always installed along-side in-tree drivers.

It makes it simpler to enable these drivers by default when in-tree to CSI migration goes GA.

Pros:
1. Simpler handling of in-tree to CSI migration.
2. Drivers are automatically installed and admin can monitor progress.

Cons:
1. If the CSI driver fails to deploy by the cluster-storage-operator, it will make the cluster-storage-operator degraded and as a result cluster will be degraded. This may not be optimal because initially we want CSI driver to be optional component.
2. Supporting future configuration of drivers is tricky. Currently cluster-storage-operator is not a configurable operator. But enabling and disabling
migration and making driver configurable will require desiging CRDs which could support these functions.
