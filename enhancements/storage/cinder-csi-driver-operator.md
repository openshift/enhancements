---
title: csi-cinder-operator
authors:
  - "@mfedosin"
reviewers:
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
  - "@fbertina"
approvers:
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
  - "@fbertina"
creation-date: 2020-08-17
last-updated: 2020-10-20
status: implementable
see-also:
replaces:
superseded-by:
---

# CSI Operator for Openstack Cinder

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In the past storage drivers have been provided either in-tree or via external provisioners. With the move to advocating CSI as the way to deliver storage drivers, OpenShift needs to move the in-tree drivers (and others that our customers need) to use CSI. With the focus on operators in Red Hat OCP 4.x these CSI drivers should be installed in the form of an Operator.

## Motivation

- In-tree drivers will be removed from Kubernetes, we need to continue to make the drivers available and CSI is the way to do this.

- OpenStack is a key cloud provider for OpenShift and use of Cinder has been supported in both OpenShift 3.x and 4.x, we need to move this driver to be CSI, provided by an operator.

- CSI drivers (including OpenStack Cinder) will enable new storage features, not possible with in-tree volume plugins, like snapshots and volume cloning.

### Goals

- Create a cluster operator to install the OpenStack Cinder CSI driver.

- Package and ship downstream images for the operator and OpenStack Cinder driver.

- Integrate the operator with Cluster Storage Operator.

### Non-Goals

- Driver creation as it is available in [upstream](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/using-cinder-csi-plugin.md).

- Implement CSI tooling (snapshotter, resizer, cloner). It is available in OpenShift since 4.3.

## Proposal

OCP ships a new cluster operator called `openstack-cinder-csi-driver-operator`.

1. The operator installs the OpenStack Cinder CSI driver following the [recommended guidelines (currently work-in-progress)](https://github.com/openshift/enhancements/pull/139/files).
2. The operator deploys the all the objects required by the OpenStack Cinder CSI driver:
   2.1 A namespace called `openshift-cluster-csi-drivers`.
   2.2 Two ServiceAccounts: one for the Controller Service and other for the Node Service of the CSI driver.
   2.3 The RBAC rules to be used by the sidecar containers.
   2.4 The CSIDriver object representing the CSI driver.
   2.5 A Deployment that runs the driver's Controller Service.
   2.7 A DaemonSet that runs the driver's Node Service.
   2.8 A non-default StorageClass that uses the CSI driver as a provisioner.
3. The operator deploys all the CSI driver objects in the namespace `openshift-cluster-csi-drivers`.
   3.1 This is true regardless the namespace where the operator itself is deployed.
4. The operator itself is installed by [Cluster Storage Operator](https://github.com/openshift/cluster-storage-operator) for all OpenStack clouds.
   4.1 The operator should be deployed in the namespace `openshift-cluster-csi-drivers`.
   4.2 The CR which the operator reacts to is *non-namespaced* and is named `cinder.csi.openstack.org`.
   4.3 The CR belongs to `ClusterCSIDriver` CRD.
5. The operator leverages the OpenStack credentials by creating a credentials request to [Cluster Credential Operator](https://github.com/openshift/cloud-credential-operator).

**Note:** Cluster admin can choose if they're going to use stable & supported in-tree Cinder volume plugin or tech preview CSI (and get snapshots). In-tree will be the default one for OpenShift 4.7.

### Action plan

#### Build OpenStack Cinder CSI driver image by OCP automation

To start using OpenStack Cinder CSI in OpenShift we need to build its image and make sure it is a part of the OpenShift release image. The driver image should be automatically tested before it becomes available.
The driver provides the Dockerfile, so we can reuse it to complete the task. Upstream image is already available in Quay in quay.io/k8scsi account.

Actions:

- Configure CI operator to build OpenStack Cinder CSI driver image.

The operator will run containerized and End-to-End tests and also push the resulting image in the OCP Quay account.

#### Test the driver manually (Done)

When all required components are built, we can manually deploy the driver and test how it works.

Actions:

- Manually deploy the driver on OCP on an OpenStack cloud with availability zones and self-signed certificates support.

- Go through the lifecycle of volume (create, update, mount, write data, delete, etc.).

- Test availability zones support.

- Test self-signed certificates support.

#### Write OpenStack Cinder CSI driver operator

The operator should be able to create, configure and manage OpenStack Cinder CSI driver for OpenShift. In other words, automate what has been done on the previous step.

Actions:

- Create a new repo in OpenShift’s github: [github.com/openshift/openstack-cinder-csi-driver-operator](github.com/openshift/openstack-cinder-csi-driver-operator)

- Implement the operator, using [library-go](https://github.com/openshift/library-go) primitives.

##### CSI driver operator installation

Starting from OpenShift 4.7 the installation will include next steps:

- `cluster-version-operator` starts `openshift-cluster-storage-operator`.

- `openshift-cluster-storage-operator` checks if it runs on OpenStack, installs OpenStack Cinder CSI driver operator, starts it and monitors its status (i.e. monitors existence of the OpenStack Cinder CSI driver operator ClusterOperator CR and reports its own `openshift-cluster-storage-operator` status based on it).

- OpenStack Cinder CSI driver operator starts, populates
  [configuration](https://github.com/kubernetes/cloud-provider-openstack/tree/master/manifests/cinder-csi-plugin)
  and secrets for OpenStack Cinder CSI driver and runs the CSI driver
  (i.e. starts Deployment with the controller parts and DaemonSet with
  the node parts). Additionally it creates at least one non-default
  StorageClass that users can use in their PVCs.

- OpenStack Cinder CSI driver operator reports status of the driver in the ClusterOperator CR.

#### Add automatic testing

To make sure that everything works properly, we need the support of CI. Kubernetes already implements all necessary tests for CSI drivers, and they should run automatically if the testing system detects a CSI driver attached.
Actions:

- Run tests from https://github.com/openshift/origin/tree/master/vendor/k8s.io/kubernetes/test/e2e/storage/drivers against OpenStack Cinder CSI driver.

#### Document how to use the operator

Make sure users understand how to use the feature.

Actions:

- Write documentation for end users, that describes how they can create a persistent volume with OpenStack Cinder CSI driver.

#### Build OpenStack Cinder CSI driver operator image by OCP automation

To start using OpenStack Cinder CSI in OpenShift we need to build its image and make sure it is a part of the OpenShift release image. The image should be automatically tested before it becomes available.

Actions:

- Configure CI operator to build OpenStack Cinder CSI driver operator image.

The operator will run containerized and End-to-End tests and also push the resulting image in the OCP Quay account.

### Risks and Mitigations

#### OpenStack Cinder CSI driver doesn’t work properly

The driver has not been tested on either OSP or OCP. The team uses Devstack + K8s plugin for the development.

Severity: medium-high
Likelihood: probably

When testing on production clouds, we may face challenges, like bugs, issues with compatibility and so on. We will need to address them accordingly.

### Test Plan

- Both upstream and downstream CSI driver repositories will have a pre-submit job to run the Kubernetes external storage tests.

- The operator will have unit tests running as a pre-submit job.

- The operator will also have a pre-submit job to run the Kubernetes external storage tests against the CSI driver installed by it.

## Alternatives

Only one alternative is to keep the deprecated in-tree driver in OpenShift and support it with our own resources.

## Infrastructure Needed

Additional OpenStack cloud may be required to test how CSI Driver works with self-signed certificates. Our current CI doesn't allow this.

## Links

1. [OpenStack Cinder documentation](https://docs.openstack.org/cinder/latest/)
2. [OpenStack Cinder CSI Driver code](https://github.com/kubernetes/cloud-provider-openstack/tree/master/pkg/csi/cinder)
3. [OpenStack Cinder CSI Driver documentation](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/using-cinder-csi-plugin.md)
4. [CSI drivers installation enhancement proposal](https://github.com/openshift/enhancements/pull/139)
