---
title: CSI driver installation for in-tree drivers
authors:
  - "@gnufied"
  - “@jsafrane”
reviewers:
  - "@eparis”
  - "@shawn-hurley"
approvers:
  - "@eparis"
  - "@knobunc"
  - "@shawn-hurley"
creation-date: 2019-11-04
last-updated: 2020-03-10
status: implementable
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

## Terminology

CSI migration - seamless migration of in-tree volume plugins (AWS EBS, GCE PD, OpenStack Cinder, vSphere, Azure Disk, Azure File) to their CSI counterparts. In a cluster with CSI migration enabled for a particular plugin, Kubernetes translates all in-tree volume plugin calls into CSI under the hood, without users changing their PVs / StorageClasses. This enables removal of the cloud specific code and whole cloud providers out of kubernetes/kubernetes.

## Goals

We would like to approach various goals of this KEP in different phases, because we do not expect to get everything right or done in phase-1.

### Phase-1 Goals

* Provide a way for AWS and GCE CSI driver installation . Users could either optionally install it or CSI driver could be installed
along with in-tree driver and users could use both.
* Install CSI provided storageclass along with in-tree StorageClass.

### Phase-2 Goals

* Support installation of GCE, Cinder, vSphere, AzureDisk CSI drivers.
* Provide a way for users to enable in-tree to CSI migration. It basically replaces whole in-tree volume plugin with a less tested CSI implementation. While it's not a strict requirement, we'd like to have the migration optional for at least one release.
* Provide a way for users to disable in-tree to CSI migration.

### Phase-3 Goals

* Enable CSI drivers as default drivers and CSI provided storageclass as default storageclass.
* Optionally allow users to configure CSI driver install.

## Non-Goals

* This KEP does not attempt to design installation of third-party or partner CSI drivers.
* Design of CSI migration enablement.
  * A new operator will be needed to enable alpha features on API server, controller-manager and kubelets in the right order and with proper node draining before switching the gates.
  * Right now it's implementation detail, this enhancement focuses on CSI driver installation. Proper enhancement will be created for CSI migration.

## Proposal

We are currently considering using OLM for installation of CSI driver.

### Installation via OLM

We propose that - we provide each driver mentioned above as a separate operator which could be subscribed and installed via OLM UI. Each driver's operator
is responsible for its installation and release. The operator is responsible for creating storageclass that the driver provides.

The configuration of CSI driver can be done via OLM UI if required and CSI driver can access cloudprovider credentials
from Openshift provided sources.The CR that is responsible for driver configuration can be installed by the operator
itself optionally or by the user. We expect operator configuration CR to be *cluster-scoped* rather than namespace scoped.

The reason for choosing cluster-scoped CRs are two fold:
1. CSI drivers logically span the entire cluster, so control of them in a single namespace doesn't make sense in a cluster and could lead to unnecessary conflicts. In addition, only a cluster-admin should be manipulating the set of CSI drivers, so cluster-scoped is a good choice in this case.
2. Having CRs cluster-scoped avoids unnecessary deletion and re-creation of CRs when cluster-storage-operator could potentially adopt an operator subscription.

User should be able to edit the CR and change log level, managementState and update credentials(if operator configuration CR is mechanism by which credentials are delivered to the CSI driver) required for talking to storage backend.

Installation via OLM however means that, when we want to enable these CSI drivers as default drivers,
they must be installed by default in Openshift installs. We further propose that -
Cluster Storage Operator(https://github.com/openshift/cluster-storage-operator)
could create subscriptions for these driver operators when drivers have to be installed by default.

Expected workflow as optional driver (using EBS as an example):
1. User finds EBS CSI driver operator in OLM and installs it.
2. The operator automatically creates a cluster-scoped CR for itself and install AWS EBS CSI driver with default settings. User can configure basic operator features as log level or ManagementState in the CR. We expect only few (if any) configuration options for the CSI drivers themselves - they don't have any options in-tree. User cannot create any other CR for the operator (ensured via webhooks).
4. While the operator is installed in a user provider namespace, the CSI driver should be installed in a namespace pre-defined(for example - `openshift-csi-ebs-driver`) in the operator.
3. EBS CSI driver is installed and it creates relevant storageclass that user can use to provision CSI EBS volumes.

When a CSI driver operator is in technical preview, we expect that the operator will be available from a `beta` channel. Moving to a `stable` channel once a driver reaches GA will require Openshift admin to manually change subscribed channel from beta to stable. At this point we expect that, operator in GA state will simply **adopt** the resources(CRs) created by beta version of the operator.


#### Uninstallation of optional CSI driver operator.

Removing a CSI driver should be a safe operation and only possible if no workload is using the driver and hence we propose that - driver configuration CR
should have a finalizer which will be removed by the operator when csi-driver operator detects that no volume provisioned by this driver is in-use. Typically we expect following steps to happen in order for removal of the driver:

1. User should remove any pods that are using the CSI volumes.
2. User should delete all PV/PVCs provisioned by the CSI driver.
3. User should remove all snapshots provisioned by the CSI driver.
4. User deletes the CSI driver CR which in turn will cause operator to delete all resources created by the driver (daemonsets, storageclasses, deployments)
5. Finalizer is removed from CR and CSI driver CR is removed from api-server.
6. User can now safely uninstall the CSI driver operator by deleting the subscription from UI or from command line.

### Expected workflow as default driver:

When these drivers become mandatory part of Openshift cluster, we need to install them by default. This section in general only applies to drivers which want to be enabled by default in Openshift installation.

1. CVO installs cluster-storage-operator.
2. cluster-storage-operator detects cloudprovider on which cluster is running(lets say EBS).
3. cluster-storage-operator creates a subscription for EBS CSI driver using redhat operator source.
4. cluster-storage-operator monitors progress of subscription and sets its own status as available when subscription is installed.

cluster-storage-operator ensures that there is always an subscription for CSI driver in given cloudprovider environment. cluster-storage-operator will also update subscribed channel for the operator when creating the subscription.


There are pros and cons to this approach:

Pros:
1. It is simple to create a opt-in installable driver operator.
2. Providing an optional UI for configuration via OLM is a plus.
3. Each driver's operator sits in its own repository and managing releases could be simpler.

Cons:
1. When these CSI drivers have to be installed by default, this approach causes a coupling between CVO managed cluster-storage-operator and driver's operator.
2. Need to adopt operator installed by user during cluster update.

#### Upgrade from optional operator to a driver installed by default

When a CSI driver is moved from optional to a mandatory one, existing installation of CSI driver operator must be **adopted** by the CVO managed operator (in above case cluster-storage-operator).

1. CVO installs cluster-storage-operator.
2. cluster-storage-operator detects cloudprovider on which cluster is running(lets say EBS).
3. cluster-storage-operator searches for existing subscription to an operator with same name in all namespaces and if found it deletes and re-creates the subscription in pre-defined namespace.
4. At this point if there are any changes in configuration CR between previously installed versions vs version shipped with new operator, it must be reconciled.


### Open questions for OLM team:
1. How will disconnected installs work?

  A: This was partly answered by shawn. It is possible to use disconnected installs today via
     https://docs.openshift.com/container-platform/4.3/operators/olm-restricted-networks.html but index images should make it easier.


2. We need a way for a CSI driver operator to say version range of Openshift against which it is supported.

  A: This is less of a problem with index images because all versions of operator is not available from same source.

3. Are channel to which user is subscribed to automatically upgraded when Openshift version is bumped? For example: If we install an operator from 4.2 channel on OCP-4.2 and then upgrade to OCP-4.3, is subscription updated to use channel 4.3? Or this should be handled via `skipRange`?

  A: Channels aren't automatically upgraded on OCP upgrade but we will be using stable and beta channel names rather than version specific channels.As
     proposed above we expect that an operator installed from stable channel will adopt resources created by beta channel.

4. Currently CVO operators can directly access cloudprovider configuration via configmap placed in `openshift-config` namespace, are we going to allow OLM operators to do the same? Do we need to do something to support CSI driver configuration?

  A: This is still an open question. Currently in 4.6 we will have cluster-storage-operator create the subscription but this is being tracked via RFE - https://issues.redhat.com/browse/RFE-664.

5. Since most CSI operators has to be singletons,we need to stop users from installing the operator multiple times in different namespaces. Currently this is not possible.

  A. This is still an open question and work to address this is being tracked via https://issues.redhat.com/browse/RFE-660.


## Timeline
Bases on current upstream plans & assumptions.

### OCP-4.5 (Kubernetes 1.18)
* We support installation of AWS CSI driver alongside with in-tree drivers. User can use both drivers at the same time. The default storage class is for in-treee volume plugin (unless cluster admin changes it).
* The CR required for operator configuration will be created by the user.
* We do not support enabling or disabling CSI migration feature gate at this point.

#### Testing
* E2e test(s) with our CSI certification suite with driver and integration with openshift-tests.
* All sidecars and openshift/origin should run tests with forked OCP sidecars and forked OCP driver.

### OCP-4.6 (Kubernetes 1.19)
* Add support for Azure and Cinder CSI drivers.
* We will allow users to enable or disable CSI migration. Disabling the migration should not remove the driver.

#### Testing
* Run e2e tests with CSI migration on.
  * Requires testing harness to edit/create a CR to enable migration and wait for migration to complete - kinda open question.
  * Will require a new prow job.
  * New test not available upstream: Run a StatefulSet with migration disabled (assuming it's the default). Enable the migration and check if the StatefulSet works. Disable the migration and check if it still works.
    * How is upstream going to test this?
* We should have scale tests that runs the tests at large scale (co-ordinate with perf team may be).
   * Looking for signal about CSI driver deployment working at scale.
   * Ensure that with somewhat increased number of watchers etc, things still keep working.
* Long-running e2e tests.
  * To make sure the CSI driver can sustain more than 1 hour of regular e2e tests.

### OCP-4.7 (Kubernetes 1.20)

* vSphere CSI driver installation.
  * Current vSphere driver is not backward compatible. We expect some changes and stabilization upstream. Our gut feeling is +/- Kubernetes 1.20 timeframe.

### OCP-4.8 (Kubernetes 1.21)
Kubernetes 1.21 is current upstream target for removal of in-tree cloud providers and their volume plugins!

* CSI drivers for their in-tree counterpart will become GA and CSI volumes will become default.
* CSI migration is enabled and cannot be disabled (code for corresponding in-tree volume plugins and in-tree cloud providers doesn't event exist).
