---
title: manila-csi-driver-integration
authors:
  - "@mfedosin"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2019-11-22
last-updated: 2019-11-22
status: implementable
---

# Manila CSI driver integration

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This document describes [Manila](https://docs.openstack.org/manila/latest/) [CSI](https://github.com/container-storage-interface/spec/blob/master/spec.md) driver integration to enable ReadWriteMany([RWX](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes)) volume access on OpenShift 4 on OpenStack.

## Motivation

[Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes), when running on OpenStack, are backed by Cinder, which doesn’t allow RWX access. Users have applications that require this access mode and want to run OpenShift on their OpenStack platforms. If we add Manila CSI driver support, users will be able to run more applications on OpenShift 4 on OpenStack.

### Goals

- Manila CSI driver is installed during OCP installation on OpenStack without any user action if Manila service is available.

- Automatic testing added.

### Non-Goals

- Implement the CSI driver - it's already done in upstream.

- Implement CSI tooling (snapshotter, resizer, cloner). It is available in OpenShift since 4.3.

## Proposal

Our main goal is to add RWX volume support in OpenShift 4 on OpenStack. So we are going to use Manila through the CSI driver available in upstream as a part of [cloud-provider-openstack](https://github.com/kubernetes/cloud-provider-openstack/tree/master/pkg/csi/manila) repo.

To maintain the lifecycle of the driver we want to implement an operator, that will handle all administrative tasks: deploy, restore, upgrade, healthchecks, and so on.

### Action plan

#### Find a testing/development platform that supports Manila (Done)

For testing and development we need a public OpenStack cloud with Manila support. So far MOC and PSI do not have this capability.
The cloud should be OSP 13+ based, and comply with the reference architecture.
There are two possibilities how we can accomplish this task:

- Add Manila support to either MOC or PSI.

- Find a completely new cloud, like next-gen RDO.

Actions:

- The first item on the list seems more likely. We need to contact the clouds admins and ask them to add Manila support.

#### Update cloud-provider-openstack for OpenShift (Done)

Currently the repo containing Manila CSI driver is forked, but seriously outdated https://github.com/openshift/cloud-provider-openstack
We need to fetch all the latest changes from upstream.
Actions:

- Add ShiftStack team to the Owners (Done: https://github.com/openshift/cloud-provider-openstack/pull/15)

- Prepare and merge a bump commit (Done: https://github.com/openshift/cloud-provider-openstack/pull/16)

#### Build Manila CSI driver image by OCP automation (Done)

To start using Manila CSI in OpenShift we need to build its image and make sure it is a part of the OpenShift release image. The driver image should be automatically tested before it becomes available.
The driver provides the Dockerfile, so we can reuse it to complete the task. Upstream image is already available in Quay in quay.io/k8scsi account.
Actions:

- Configure CI operator to build Manila CSI driver image.

The operator will run containerized and End-to-End tests and also push the resulting image in the OCP Quay account.

#### Test the driver manually (Done)

When all required components are built, we can manually deploy the driver with NFS backend and test how it works.
Actions:

- Manually deploy the driver on a cloud with Manila support

- Configure it to use NFS backend

- Go through the lifecycle of volume (create, update, mount, write data, delete, etc.)

#### Write Manila CSI driver operator (Done)

The operator should be able to create, configure and manage Manila CSI driver for Kubernetes and OpenShift. In other words, automate what has been done on the previous step.
Actions:

- Create a new repo in OpenShift’s github: github.com/openshift/csi-driver-manila-operator

- Implement the operator, using [OpenShift Operator SDK](https://docs.openshift.com/container-platform/4.1/applications/operator_sdk/osdk-getting-started.html)

##### CSI driver operator installation

Starting from OpenShift 4.5 the installation will include next steps:

- `cluster-version-operator` starts `openshift-cluster-storage-operator`.

- `openshift-cluster-storage-operator` checks if it runs on OpenStack, installs Manila CSI driver operator via OLM, starts it and monitors its status (i.e. monitors existence of Manila CSI driver operator ClusterOperator CR and reports its own `openshift-cluster-storage-operator` status based on it).

- Manila CSI driver operator checks if Manila service is available. If it's true then it starts, populates [configuration](https://github.com/kubernetes/cloud-provider-openstack/tree/master/manifests/manila-csi-plugin) and secrets for Manila CSI driver and runs the CSI driver (i.e. starts Deployment with the controller parts and DaemonSet with the node parts). Additionally it creates at least one non-default StorageClass that users can use in their RWX PVCs. If Manila service is not available, the operator does nothing.

- Manila CSI driver operator reports status of the driver in ClusterOperator CR.

In OpenShift <= 4.4 users will have to install the operator manually via OLM.

#### Add automatic testing

To make sure that everything works properly, we need the support of CI. If Manila is added to MOC, we just need to run a few tests in e2e-openstack pipeline. K8s already implements all necessary tests for CSI drivers, and they should run automatically if the testing system detects a CSI driver attached.
Actions:

- Run tests from https://github.com/openshift/origin/tree/master/vendor/k8s.io/kubernetes/test/e2e/storage/drivers against Manila CSI driver.

#### Document how to use the operator (Done)

Make sure users understand how to use the feature.

Actions:

- Write documentation for end users, that describes how they can create a persistent volume in Manila.

#### Build Manila CSI driver operator image by OCP automation (Done)

To start using Manila CSI in OpenShift we need to build its image and make sure it is a part of the OpenShift release image. The image should be automatically tested before it becomes available.
Dockerfile should be automatically generated by the SDK.
Actions:

- Configure CI operator to build Manila CSI driver operator image.

The operator will run containerized and End-to-End tests and also push the resulting image in the OCP Quay account.

#### Implement Secondary NIC support

By default in OSP Manila uses additional network for volumes. This means that in addition to the main network, we must connect worker nodes to the appropriate storage network to be able to successfully mount the volumes.

Actions:

- Add the ability to specify additional networks and security groups in the installer configuration.

- Document this feature.

Next fields will be added to OpenStack's MachinePool config:

- additionalNetworkIDs (optional list of strings): IDs of additional networks for machines.

- additionalSecurityGroupIDs (optional list of strings): IDs of additional security groups for machines.

Each ID is represented by a string in [UUID v4](https://en.wikipedia.org/wiki/Universally_unique_identifier#Version_4_(random)) format.

No changes are required for [cluster-api-provider-openstack](https://github.com/openshift/cluster-api-provider-openstack), as it already supports the necessary functionality. To add networks and security groups after installation or upgrade, users will need to modify related machine configurations in `OpenstackProviderSpec`:

- [securityGroups](https://github.com/openshift/cluster-api-provider-openstack/blob/master/pkg/apis/openstackproviderconfig/v1alpha1/types.go#L64-L65) for additional security groups.

- [networks](https://github.com/openshift/cluster-api-provider-openstack/blob/master/pkg/apis/openstackproviderconfig/v1alpha1/types.go#L54-L56) for additional networks.

### Risks and Mitigations

#### No clouds were found with Manila support

We won't be able to develop/test our work.

Severity: blocker
Likelihood: probably

#### Manila CSI driver doesn’t work properly

The driver has not been tested on either OSP or OCP. The team uses Devstack + K8s plugin for the development.

Severity: medium-high
Likelihood: probably

When testing on industrial clouds, we may face challenges, like bugs, issues with compatibility and so on. We will need to address them accordingly.

#### CSI tooling doesn’t work properly

CSI tooling was introduced in OpenShift in 4.3 by the storage team. So far there are no CSI drivers available, and there can be some problems with operability.

Severity: medium-high
Likelihood: low

#### OpenStack Manila is unstable on the dev cloud

This can affect our productivity, because we won’t be able to test our work.

Severity: depends on a situation
Likelihood: probably

## Design Details

### Test Plan

Kubernetes provides a set of RWX tests, which we are going to use them against Manila CSI driver. We will need to find a proper cloud that supports Manila and run these tests there.

### Graduation Criteria

#### Dev Preview

- Be able to deploy the driver and create a PV in Manila
- Gather feedback from developers
- Document how to use the feature

#### Dev Preview -> Tech Preview

- Be able to manage PVs in Manila
- End user documentation
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

It should be noted that Manila CSI driver operator is brand new and does not deprecate any existing component, so, at the moment there is nothing to upgrade from.

That said, with further upgrades we would ensure that any changes are backwards compatible.

We also control the life cycle of the CSI driver that this operator manages so we are able to prevent dependencies changing in an incompatible way.

### Version Skew Strategy

Version skew should not be an issue for Manila CSI driver operator, because it relies either on components that we control, like CSI tooling, or stable Kubernetes APIs (pods, configmaps, etc). Nothing depends on the operator either.

Since the operator will use operator-sdk, we can also use the OLM to manage the operator upgrades.

## Infrastructure Needed

For development and testing we need an OpenStack cloud that supports Manila.

## Links

1. [OpenStack Manila documentation](https://docs.openstack.org/manila/latest/)
2. [Manila CSI Driver code](https://github.com/kubernetes/cloud-provider-openstack/tree/master/pkg/csi/manila)
3. [Manila CSI Driver documentation](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/using-manila-csi-plugin.md)
4. [CSI Driver Manila Operator](https://github.com/openshift/csi-driver-manila-operator)
5. [CSI drivers installation enhancement proposal](https://github.com/openshift/enhancements/pull/139)
