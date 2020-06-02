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

We want certain CSI drivers such as AWS, GCE, Cinder, Azure and vSphere to be installable on OpenShift, so as
so as they can be used along-side in-tree drivers and when upstream enables migration flag for these volume types, their
replacement CSI drivers can take over and none of the storage features get affected.

## Motivation

Upstream Kubernetes is moving towards removing code of in-tree drivers and replacing them with their CSI counterpart. Our
current expectation is that - all in-tree drivers that depend on cloudprovider should be removed from core Kubernetes by 1.21.
This may not happen all at once and we expect migration for certain drivers to happen sooner.

This does mean that - OpenShift should be prepared to handle such migration. We have to iron out any bugs in driver themselves and
their interfacing with OpenShift. We need a way for users to use the CSI drivers and optionally enable migration from in-tree driver
to CSI driver. To support upstream design - we will also need a way for users to disable the migration and keep using in-tree driver, until
in-tree code is finally removed.

## Terminology

CSI migration - seamless migration of in-tree volume plugins (AWS EBS, GCE PD, OpenStack Cinder, vSphere, Azure Disk, Azure File) to their CSI counterparts. In a cluster with CSI migration enabled for a particular plugin, Kubernetes translates all in-tree volume plugin calls into CSI under the hood, without users changing their PVs / StorageClasses. This enables removal of the cloud specific code and whole cloud providers out of kubernetes/kubernetes.

## Goals

We would like to approach various goals of this KEP in different phases, because we do not expect to get everything right or done in phase-1.

### Phase-1 Goals

* Provide a way for AWS and Manila CSI driver installation. Users could either optionally install it or CSI driver could be installed
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

We are currently considering using CVO and cluster-storage-operator (CSO) for installation of CSI driver.

Each CSI driver is deployed via a dedicated operator (i.e. each CSI driver has its own), to accommodate differences
between the CSI drivers. For example, Manila CSI driver operator can detect if Manila service is present in underlying
OpenStack cloud and deploy the driver only when the service is present.

* Each CSI driver will provide its own CRD, `<backend>Driver` in `csi.openshift.io` API group. E.g. `AWSEBSDriver` or
  `ManilaDriver`. The CRD is non-namespaced - CSI drivers are global in the whole cluster.
* Each CRD will allow only a single instance, named `cluster`.
* Initially, the CRDs will be equal and provide only common
  [`OperatorSpec`](https://github.com/openshift/api/blob/95abe2d2f4223d5931e418bf8e4d3773d16b42c0/operator/v1/types.go#L48)
  and [`OperatorStatus`](https://github.com/openshift/api/blob/95abe2d2f4223d5931e418bf8e4d3773d16b42c0/operator/v1/types.go#L97).
  In the future, each CSI driver may introduce its own configuration fields.
* Each CSI driver runs in a dedicated namespace, typically `openshift-<backed>-csi-driver` (`openshift-aws-ebs-csi-driver`).
  * TODO: or run everything in cluster-storage namespace? OLM installs into openshift-aws-ebs-csi-driver, we would need to delete it before starting a new driver in cluster-storage. 
* TODO: where the operator runs? cluster-storage namespace?
* Each CSI driver uses cloud-credential-operator to obtain a role in the underlying cloud + its credentials to
  manipulate with the cloud storage API.

### Installation
1. During installation, CVO starts cluster-storage-operator (CSO).
2. CSO detects underlying cloud and starts operators for CSI driver(s) for the particular cloud.
   * I.e. CSO deploys corresponding CSI driver operators, incl. their RBAC, CRD, Deployment and finally a default CR.
3. The CSI driver operator obtains cluster credentials via cloud-credential-operator and deploys the CSI driver,
   i.e. creates its RBAC, Deployment, DaemonSet and CSIDriver objects. Progress of the deployment is reported in
   `CR.Status` of the operator CR.
4. CSO monitors CRs of the CSI drivers and reports back their status ("Progressing", "Degraded", "Available", ...) in
   CSO's ClusterOperator status.

### Upgrade from OLM-managed operator
In OCP 4.5, we released AWS EBS and Manila CSI drivers via OLM. It's too late to move them to CSO.
We designed their operators in a way that regardless where the CSI driver operator runs, the driver itself always runs
in openshift-aws-ebs-csi-driver namespace / openshift-manila-csi-driver.

1. During upgrade, CVO starts new 4.6 cluster-storage-operator (CSO).
2. 4.6 CSO detects that there is AWS / Manila driver installed by OLM.
3. CSO deletes Subscription of the operator. OLM removes the operator, but leaves the CRD, CR and the driver / operand running.
4. CSO runs the new CSI driver operator.
5. The CSI driver operator adopts the old operand - it reconciles the old CSI driver objects (Deployment, DaemonSet,
   RBAC, ...) with updated 4.6 content. I.e., all objects created by the 4.6 operator must have the same name
   as the objects created by the old 4.5 operator. 

In case there was no AWS / Manila CSI driver operator running during the update, CSO installs the corresponding operator
as during installation, so user has a CSI driver running after update.

### Un-installation

It is not possible to un-install a CSI driver / operator installed by cluster-storage-operator. Similarly to in-tree
volume plugins that can't be un-installed, OCP will provide a default set of CSI drivers after installation for each
cluster. Explicitly, deletion of a CSI driver CR will not result in CSI driver un-installation. Users that want to use
their own / upstream CSI drivers must set the operator `Unmanaged` and delete the CSI driver manually, just like with
any other cluster-scope OCP component.

### Documentation
We require 3rd party CSI driver vendors to ship their CSI drivers via OLM. In order to prove that OLM has necessary
capabilities, OCP will ship a sample CSI driver + CSI driver operator via OLM, together with a set of recommendations
how a CSI driver operator should work, as a separate enhancement.

Details about what CSI driver it is going to be is out of scope of 4.6.

## Timeline
Bases on current upstream plans & assumptions.

### OCP-4.5 (Kubernetes 1.18)
* We support installation of AWS EBS (tech preview) and Manila CSI (GA) drivers with in-tree drivers. User can use both
  drivers at the same time. The default storage class is for in-tree volume plugin (unless cluster admin changes it).
  * The driver is optional, not installed by default.
  * The driver is installed via OLM.
* The CR required for operator configuration will be created by the user.
* We do not support enabling or disabling CSI migration feature gate at this point.

#### Testing
* E2e test(s) with our CSI certification suite with driver and integration with openshift-tests.
* All sidecars and openshift/origin should run tests with forked OCP sidecars and forked OCP driver.

### OCP-4.6 (Kubernetes 1.19)
* AWS EBS and Manila drivers become installed by default in all clusters. User can use both drivers at the same time.
  The default storage class is for in-tree volume plugin (unless cluster admin changes it).
  * The drivers are managed by cluster-storage operator. If they were installed via OLM in a 4.5 cluster that's upgraded
    to 4.6, the CSI drivers are adopted as described above.

#### Testing
* Upgrade test from 4.5 with the driver installed via OLM.
  * This ensures that drivers installed via OLM are correctly adopted, without disturbing the cluster. 

### OCP-4.7 (Kubernetes 1.20)
* GCE, Azure and Cinder CSI drivers become installed by default (with non-default storage class).
* vSphere CSI driver installation (as optional).
  * Current vSphere driver is not backward compatible. We expect some changes and stabilization upstream. Our gut feeling is +/- Kubernetes 1.20 timeframe.
* We will allow users to enable or disable CSI migration. Disabling the migration should not remove the driver.
* Sample CSI driver operator + CSI driver is available on OperatorHub.

#### Testing
* Run e2e tests with CSI migration on.
  * Requires testing harness to edit/create a CR to enable migration and wait for migration to complete - kinda open question.
  * Will require a new prow job.
  * New test not available upstream: Run a StatefulSet with migration disabled (assuming it's the default). Enable the
    migration and check if the StatefulSet works. Disable the migration and check if it still works.
    * How is upstream going to test this?
* We should have scale tests that runs the tests at large scale (co-ordinate with perf team may be).
   * Looking for signal about CSI driver deployment working at scale.
   * Ensure that with somewhat increased number of watchers etc, things still keep working.
* Long-running e2e tests.
  * To make sure the CSI driver can sustain more than 1 hour of regular e2e tests.

### OCP-4.8 (Kubernetes 1.21)
Kubernetes 1.21 is current upstream target for removal of in-tree cloud providers and their volume plugins!

* CSI drivers for their in-tree counterparts will become GA and CSI volumes will become default.
* CSI migration is enabled and cannot be disabled (code for corresponding in-tree volume plugins and in-tree cloud providers doesn't event exist).


## Infrastructure needed

* ART must "adopt" existing AWS EBS and Manila CSI driver operator and drivers.
  * In 4.5, these operators are installed via OLM. We (storage team) are responsible for release + 4.5.z erratas.
    (No 4.5.z releases are planned, but we need to be prepared for serious errors or CVEs.) 
  * From 4.6, ART should manage these two operators + corresponding CSI drivers. These images become part of the
    release payload! All 4.6 and 4.6.z erratas are managed by ART.
  
## Alternatives considered

### Installation via OLM

We explored possibility of installing CSI driver operators via OLM. This has been proven to be difficult, not because
of OLM features, but because of the necessary infrastructure.

* OLM in CI clusters doesn't see the latest operators, it shows only one release old operators.
* Since OLM managed operators do not go through ART pipeline, the operators must be released via a separate errata.
  At the same time they refer to SHAs of CSI sidecar images, released in OCP erratas. It's not possible
  to rebuild CSI operator metadata with new SHAs when CSI sidecar changes, therefore release of the operator + OCP
  errata at the same time is challenging.
* Image mirroring for "offline" OCP installation does not count with some images provided by OLM.

For historical purposes, whole enhancement with installation through OLM:

We propose that - we provide each driver mentioned above as a separate operator which could be subscribed and installed via OLM UI. Each driver's operator
is responsible for its installation and release. The operator is responsible for creating storageclass that the driver provides.

The configuration of CSI driver can be done via OLM UI if required and CSI driver can access cloudprovider credentials
from OpenShift provided sources. Initially, when the CSI driver is an optional component, user must create the CR for the operator.
We expect operator configuration CR to be *cluster-scoped* rather than namespace scoped.

The reason for choosing cluster-scoped CRs are two fold:
1. CSI drivers logically span the entire cluster, so control of them in a single namespace doesn't make sense in a cluster and could lead to unnecessary conflicts. In addition, only a cluster-admin should be manipulating the set of CSI drivers, so cluster-scoped is a good choice in this case.
2. Having CRs cluster-scoped avoids unnecessary deletion and re-creation of CRs when cluster-storage-operator could potentially adopt an operator subscription.

User should be able to edit the CR and change log level, managementState and update credentials (if operator configuration CR is mechanism by which credentials are delivered to the CSI driver) required for talking to storage backend.

Installation via OLM however means that, when we want to enable these CSI drivers as default drivers,
they must be installed by default in OpenShift installs. We further propose that -
Cluster Storage Operator(https://github.com/openshift/cluster-storage-operator)
could create subscriptions for these driver operators when drivers have to be installed by default
and create CR for the driver with default parameters.

Expected workflow as optional driver (using EBS as an example):
1. User finds EBS CSI driver operator in OLM and installs it.
2. User creates creates a cluster-scoped CR for the operator, typically using console.
   We expect only few  configuration options for the CSI drivers themselves (`ManagementState`, `LogLevel`) - they don't have any options in-tree.
   User cannot create any other CR for the operator (ensured via CR name validation).
3. The operator install AWS EBS CSI driver.
4. While the operator is installed in a user provider namespace, the CSI driver should be installed in a namespace pre-defined(for example - `openshift-csi-ebs-driver`) in the operator.
5. EBS CSI driver is installed and it creates relevant storageclass that user can use to provision CSI EBS volumes.

When a CSI driver operator is in technical preview, we expect that the operator will be available from a `preview` channel. Moving to a `stable` channel once a driver reaches GA will require OpenShift admin to manually change subscribed channel from beta to stable. At this point we expect that, operator in GA state will simply **adopt** the resources(CRs) created by beta version of the operator.

##### Uninstallation of optional CSI driver operator.

Removing a CSI driver should be only possible if no workload is using the driver and hence we propose that - driver configuration CR
should have a finalizer which will be removed by the operator when csi-driver operator detects that no volume provisioned by this driver is in-use. Typically we expect following steps to happen in order for removal of the driver:

1. User should remove any pods that are using the CSI volumes.
2. User should delete all PV/PVCs provisioned by the CSI driver.
3. User should remove all snapshots provisioned by the CSI driver.
4. User deletes the CSI driver CR which in turn will cause operator to delete all resources created by the driver (daemonsets, storageclasses, deployments).
5. Finalizer is removed from CR and CSI driver CR is removed from api-server.
6. User can now safely uninstall the CSI driver operator by deleting the subscription from UI or from command line.

#### Expected workflow as default driver:

When these drivers become mandatory part of OpenShift cluster, we need to install them by default. This section in general only applies to drivers which want to be enabled by default in OpenShift installation.

1. CVO installs cluster-storage-operator.
2. cluster-storage-operator detects cloudprovider on which cluster is running (lets say EBS).
3. cluster-storage-operator creates a subscription for EBS CSI driver using redhat operator source.
4. cluster-storage-operator monitors progress of subscription and CR and sets its own status as available when the operator reports the driver is running via the CR.

cluster-storage-operator ensures that there is always an subscription for CSI driver in given cloudprovider environment. cluster-storage-operator will also update subscribed channel for the operator when creating the subscription.

There are pros and cons to this approach:

Pros:
1. It is simple to create a opt-in installable driver operator.
2. Providing an optional UI for configuration via OLM is a plus.
3. Each driver's operator sits in its own repository and managing releases could be simpler.

Cons:
1. When these CSI drivers have to be installed by default, this approach causes a coupling between CVO managed cluster-storage-operator and driver's operator.
2. Need to adopt operator installed by user during cluster update.

##### Upgrade from optional operator to a driver installed by default

When a CSI driver is moved from optional to installed by default, existing installation of CSI driver operator must be **adopted** by the CVO managed operator (in above case cluster-storage-operator).

1. CVO installs cluster-storage-operator.
2. cluster-storage-operator detects cloudprovider on which cluster is running(lets say EBS).
3. cluster-storage-operator searches for existing subscription to an operator with same name in all namespaces and if found it deletes it and re-creates the subscription in pre-defined namespace.
4. At this point if there are any changes in configuration CR between previously installed versions vs version shipped with new operator, they are reconciled.

#### Release

Since operators managed by OLM are not managed ART pipeline, there are some consequences.

1. CSI sidecar images, used by the operator, are part of OCP extras errata.
   Therefore at OCP 4.x release (or shortly after), we must rebuild the operator bundle image with sidecar SHAs that were just released by OCP and respin the operator errata.

   The operator will **not** be available at OCP GA date, it will be released later (few days?).
      * When the operator is optional, it's not an issue, cusomers can use in-tree volume plugins and experiment with CSI few days later.
      * When the operator is installed by default and an older version of the operator is available, it's not an issue.
        OCP installation will use the old version of the operator until the new version is available.
      * When the operator is installed by default and an older version of the operator is not available, **OCP cluster cannot be installed** until the operator errata is released.
          * The operator and OCP erratas should be synchronized and released at the same time.
            The operator bugs blocks OCP from release.
            This applies only to the first release of the operator, when there is no older version to use.
            It may not be an issue if we follow the timeline outlined below, but it's a very tight timeline!

2. There is no automation to rebuild operator bundle images when operand or sidecar image change.
   In addition we must sync dist-gits manually and create / respin erratas manually too:
   * During development & testing, we will update the operator / operand / sidecar SHAs only few times (say once per sprint).
   * After release, we won't release any z-streams of the operators, CSI drivers and/or SHAs of new sidecars unless absolutelly necessary (CVEs, urgent bugs).

3. Since there is no CI for images that we build ourselves (without ART), all our downstream images are tested only by QA.

#### Open questions:
1. How will disconnected installs work?

  A: This was partly answered by shawn. It is possible to use disconnected installs today via
     https://docs.openshift.com/container-platform/4.3/operators/olm-restricted-networks.html but index images should make it easier.
     This only covers manual installation of the operator *after* cluster installation.

     What about a driver being installed by default, during cluster installation?
     What / when is going to mirror the operator & driver images?
     The installation will time out when cluster-storage operator can't install a CSI driver via OLM.

     A: currently not possible.


3. When a CSI driver operator becomes installed by default and installed by cluster-storage-operator, all CI jobs must have the current version of the operator available in OLM in the cluster.
   Currently, all 4.5 CI clusters show only 4.4 operators.

Answered questions:

1. We need a way for a CSI driver operator to say version range of OpenShift against which it is supported.

  A: This is less of a problem with index images because all versions of operator is not available from same source.

2. Are channel to which user is subscribed to automatically upgraded when OpenShift version is bumped? For example: If we install an operator from 4.2 channel on OCP-4.2 and then upgrade to OCP-4.3, is subscription updated to use channel 4.3? Or this should be handled via `skipRange`?

  A: Channels aren't automatically upgraded on OCP upgrade but we will be using stable and beta channel names rather than version specific channels. As
     proposed above we expect that an operator installed from stable channel will adopt resources created by beta channel.

3. Currently CVO operators can directly access cloudprovider configuration via configmap placed in `openshift-config` namespace, are we going to allow OLM operators to do the same? Do we need to do something to support CSI driver configuration?

  A: CSI drivers should get cloud credentials using [CredentialsRequest](https://github.com/openshift/cloud-credential-operator).

4. Since most CSI operators has to be singletons, we need to stop users from installing the operator multiple times in different namespaces. Currently this is not possible.

  A. We use `AllNamespaces` install mode - an operator with this mode can be installed only once in a cluster. Still, user has possiblity to override the namespace where the operator actually runs, which complicates update from "optional" operator to "installed by default" (see Upgrade from optional operator to a driver installed by default)

6. When / how often are index images updated after operator release?
   Even when we synchronize operator & OCP errata release as suggested above, clusters may not be installable until OLM sees the operator in the index image.

   Answered on "Operator Framework (Eng + PM) sync call with Operator Teams" - index images are synced almost immediatelly after Errata is shipped (in under 1 hour).
   Similarly, staging index for QA is regenerated after new builds are added to an errata.

### Infrastructure needed

* CI: Ability to run OCP CI jobs with the latest operator images in OLM / OperatorHub.
* Mirror CSI driver operators when mirroring OCP release images (`oc adm release mirror`).
