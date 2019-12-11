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
replacement CSI drivers can take over and none of the storage features get affected.

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

* Provide a way for AWS and GCE CSI driver installation . Users could either optionally install it or CSI driver could be installed
along with in-tree driver and users could use both.
* Install CSI provided storageclass along with in-tree StorageClass.
* *Install RHV CSI driver during installation of OCP 4.4 on RHV.* The driver is not optional. RHV is just another cloud provider for OCP.

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

Expected workflow as optional driver (using EBS as an example):
1. User finds EBS CSI driver in OLM and installs it.
2. EBS CSI driver is installed and it creates relevant storageclass that user can use to provision CSI EBS volumes.

Expected workflow as default driver:
1. CVO installs cluster-storage-operator.
2. cluster-storage-operator detects cloudprovider on which cluster is running(lets say EBS).
3. cluster-storage-operator creates a subscription for EBS CSI driver using redhat operator source.
3. cluster-storage-operator monitors progress of subscription and sets its own status as available when subscription is installed.

cluster-storage-operator ensures that there is always an subscription for CSI driver in given cloudprovider environment.


There are pros and cons to this approach and since this KEP is in its first iteration, we would like to seek feedback from other stakeholders.

Pros:
1. It is simple to create a opt-in installable driver operator.
2. Providing an optional UI for configuration via OLM is a plus.
3. Each driver's operator sits in its own repository and managing releases could be simpler.

Cons:
1. When these CSI drivers have to be installed by default, this approach causes a coupling between CVO managed cluster-storage-operator and driver's operator.
2. In general there are some concerns about OLM's lack of documentation and everyone's understanding of it.
3. Not sure about disconected cluster installation / update.

Open questions for OLM team:
1. How will disconnected installs work?
2. We need a way for a CSI driver operator to say version range of Openshift against which it is supported.
3. Are channel to which user is subscribed to automatically upgraded when Openshift version is bumped? For example: If we install an operator from 4.2 channel on OCP-4.2 and then upgrade to OCP-4.3, is subscription updated to use channel 4.3? Or this should be handled via `skipRange`?
4. Currently CVO operators can directly access cloudprovider configuration via configmap placed in `openshift-config` namespace, are we going to allow OLM operators to do the same? Do we need to do something to support CSI driver configuration?
5. There are some concerns about unknown unknowns which we may discover later on and requires faster turn around time from OLM team. These issues can become blocker issues for storage team but storage team may not have necessary technical know-how to fix them and hence will require help from OLM team.

### Installation via CVO

We are also considering if these 5 CSI drivers should be installed by default and managed via CVO. We are considering using cluster-storage-operator to detect
cloudprovider on which cluster is running and install necessary CSI driver by default. This does mean that, CSI driver is always installed along-side in-tree drivers.

It makes it simpler to enable these drivers by default when in-tree to CSI migration goes GA.

Expected workflow:
1. CVO installs cluster-storage-operator.
2. cluster-storage-operator checks on which cloud it is, let's say it's AWS.
3. cluster-storage-operator starts AWS EBS CSI driver operator and sets its cluster-storage-operator status to Available + Progressing. In future it could run also AWS EFS CSI driver operator.
4. AWS EBS CSI driver operator installs EBS CSI driver and creates some status CR (clusteroperators.config.openshift.io again?) that it's Available + Progressing.
5. AWS EBS CSI driver operator checks that the driver is fully installed, i.e. driver DaemonSet and Deployment have full set of replicas and sets its own status to Available.
6. cluster-storage-operator monitors status of AWS EBS CSI driver operator and sets its own status to Available.

User cannot uninstall the driver, therefore the AWS EBS CSI driver operator does not consume any CR (whose deletion would tell the operator to uninstall the driver).

Pros:
1. Simpler handling of in-tree to CSI migration.
2. Drivers are automatically installed and admin can monitor progress.

Cons:
1. If the CSI driver fails to deploy by the cluster-storage-operator, it will make the cluster-storage-operator degraded and as a result cluster will be degraded. This may not be optimal because initially we want CSI driver to be optional component.
2. Supporting future configuration of drivers is tricky. Currently cluster-storage-operator is not a configurable operator. But enabling and disabling
migration and making driver configurable will require designing CRDs which could support these functions.
3. cluster-storage-operator needs to give RBAC permissions to the CSI operators. I.e. it must have all the permissions already. They may be quite powerful (privileged pods, edit PVs/PVCs) and may be slightly different for each CSI driver. Collection of these RBAC rules may be complicated, as they may be in multiple repos.

### Installation with cloud providers

Since all cloud providers are moving from in-tree to external cloud controllers (and external repositories), it may have sense to have one overarching "cloud-operator".
The operator would check on which cloud it is and manage everything related to the cloud, i.e. install corresponding cloud controller and CSI driver.

It can be installed either by OLM or CVO, with the same pros/cons as noted above.

Extra pros:
1. Single operator managing everything related to clouds, single status showing health of the cloud part of a cluster.

Extra cons:
1. Coordination between cluster infrastructure (or whoever manages cloud controllers** and storage teams.

## Goals and required testing

### OCP-4.5
* We support installation of AWS & GCE CSI drivers alongside with in-tree drivers. User can use both drivers at the same time.
* We do not support enabling or disabling the feature gate at this point.

#### Testing
* CSI certification with driver and integration with openshift-tests.
* All sidecars and Openshift should run tests with forked sidecars and forker driver.

### OCP-4.6
* Add support for Azure and Cinder CSI drivers.
* We will allow users to enable or disable migration. Disabling the migration should not remove the driver.

#### Testing
* Requires testing harness to edit/create a CR to enable migration and wait for migration to complete - kinda open question.
* Will require a new prow job.
* Disable the migration and check if everything works. How is upstream going to test this?
* We should have scale tests that runs the tests at large scale (co-ordinate with perf team may be).
   - Looking for signal about CSI driver deployment working at scale.
   - Ensure that with somewhat increased number of watchers etc, things still keep working.

### OCP-4.7

* vSphere CSI driver installaion.

### OCP-4.8

* CSI drivers for their in-tree counterpart will become GA and CSI volumes will become default.
