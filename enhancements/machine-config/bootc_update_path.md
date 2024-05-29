---
title: bootc-update-path
authors:
  - "@umohnani8"
  - "@inesqyx"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@cheesesashimi"
  - "@mrunalp"
  - "@cgwalters"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@sinnykumari"
  - "@yuqi-zhang"
  - "@mrunalp"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2024-05-28
last-updated: 2024-05-28
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MCO-957
see-also:
replaces:
superseded-by:
---

# Bootc Update Path 

## Summary

This enhancement outlines an additional image update path added to the backend of the MCO. 
Keeping synced with CoreOS, MCO currently utilizes a rpm-ostree-based system to engage in base image specification and layering. By adding a bootc update path, MCO, in the future, will embrace the Image Mode for RHEL and become ready to support image-based upgrade process.

## Motivation

Currently the MCO uses rpm-ostree as its backend for layered OS updates. The introduction of bootc will necessitate another update pathway in the MCO. In older RHCOS versions we have a “non-layered” update path, then there is the rpm-ostree path, and the newly added bootc update path. Incorporating a bootc update path into MCO also opens the door for OCP to offer more rooms for customization through configmaps and bootable containers. 

### User Stories

* As an OpenShift cluster administrator, I would like my cluster to understand and to be able to target a new container image reference to boot, so that my customized changes are applied
* As an OpenShift user, I would like to use actual RHEL disk images on my nodes with any added customizations on it and have MCO boot/update into it.

### Goals

* Create a bootc update path in the MCO so that the MCO will support bootc compatible base disk images.
* Ensure that MCO is able to understand both rpm-ostree and bootc mutations
* Enable the user to install a cluster with bootc and transfer an old cluster to a bootc-enabled one 
* Have users not notice any difference in user experiences other than that more customization and mutation options are supported 
* Make bootc update path the default layered OS update path


### Non-Goals

* Refactor the MCD functions to have a unified update interface 
    - Description: Currently, there are mainly three update paths built in parallel within the MCO. They separately take care of non-image updates, image updates, and updates for pools that have opted in to On Cluster Layering. As a new bootc update path will be added in with the introduction of this enhancement, MCO is looking for a better way to better manage these four update paths who manage different types of update but also shares a lot of things in common (e.g. check reconcilability). Interest and Proposals in refactoring the MCD functions and creating a unified update interface has been raised several times in previous discussions.
    - Why non-goal: This is a valid request and will help clear up MCO’s update and enhance efficiency. This is non-goal for this enhancement because the update consolidation effort shall take place after a TP implementation of bootc switch and shall be followed up in a separate epic. 
* Bootc Day-2 Configuration Tools 
    - Description: Bootc has opened a door for disk image customization via configmap. Switching from rpm-ostree to bootc, MCO should not only make sure all the functionality remains but also proactively extending its support to make sure all the customization power brought in by bootc are directed to the user, allowing them to fully maximize these advantages.This will involve creating a new user-side API for fetching admin-defined configuration and pairing MCO with a bootc day-2 configuration tool for applying the customizations. Interest and Proposals in which has been raised several times in previous discussions.
    - Why non-goal: This is a valid request and will enhance user experience by increasing customization option.This is a non-goal for this enhancement because this request should be the next step after bootc switch happening. It will be announced and discussed in detail in a follow-up enhancement. 
* Splitting the RHCOS Image into layers will be announced in the other enhancement

## Proposal

MCO allows the users to specify changes in osImageURL Update / Extension Change / Kernel Argument Update / Kernel Type Switch and land the changes by selling out various rpm-ostree commands. With this enhancement, the image update path in the MCO will be further extended by adding support for bootc, which is already included in the RHCOS images. The new update path will be merged behind a featuregate, which will be removed once the feature becomes GA (graduation criteria discussed in Section Graduation Criteria) The goal is to ensure that all the functionality remains when we rip off the rpm-ostree backbone and have bootc in place. This aligns with our ultimate goal: Have users not notice any difference in user experiences other than that more customization and mutation options are supported. Though we are working towards maintaining as much functionality as possible, several known limitations are noticed and discussed in the workflow for the non-On-Cluster-Layering and On Cluster Layering path below.

Without On Cluster Layering: 
osImageURL Update: This change is bootc compatible and bootc enabled. MachineConfigDaemon will read the changes from the new Machine Config. Changes are deployed to the specified nodes by directly shelling out bootc commands. The implementation involves mimicking all the support we have for rpm-ostree and finding bootc equivalent. 

Kernel Argument Update: This change is bootc compatible, but not bootc enabled, meaning that it can only be done via rpm-ostree. Changes picked up by the MachineConfigDaemon will still be sent to the rpm-ostree based update path for landing. The mutated image will be bootc-compatible for further actions.

Extension Change & Kernel Type Switch: This change is bootc incompatible and bootc disabled. These two changes could only be done via rpm-ostree. Worse, these changes will also conflict with bootc, resulting in refusion to future bootc actions. As a result, this limitation can be identified as a regression for switching to bootc update path without On Cluster Layering. However, it can be solved if On Cluster Layering is enabled. The details of which will be discussed below. 

With On Cluster Layering:
On-Cluster Layering is the next step on MCO’s day-2 configuration journey with the change brought by Image Based RHEL. Upon its GA (planned for 4.17), it will solve the limitation discussed above and become the prerequisite for full bootc compatibility. The MachineOSBuilder will combine the base OS image shipped with OpenShift, any admin-provided custom content, and the MachineConfig content managed by the MCO into a new OCI image. This image will then be deployed to nodes of specified pool using bootc to facilitate a singular, transactional, update. Research has shown that no specific transition is needed as the OCI image built by On Cluster Layering (via Buildah) is by intrinsic bootc compatible. The implementation therefore falls on refactoring the rolling out process. The goal is to ensure that bootc is also able to roll out images built by OCL so that the functionality of OCL remains.

### Workflow Description

1. The cluster administrator wants to specify osImageURL Update / Kernel Argument Update / Extension Change / Kernel Type Switch to the OS on each of their cluster nodes 
2. The cluster administrator creates new Machine Configs for the changes they want to apply and deploys them to the cluster 
3. The Machine Config Daemon will read the desired change in the new config

| Type of update | OCL enabled? | Action |
| -------------  | ------------ | ------ |
| osImageURL Update | N | Enter the updateLayeredOS update path, where the system OS will be updated to the one specified in the new config by calling “bootc -switch …” |
| osImageURL Update | Y | Enter the updateClusterBuild update path, where the new changes will be built into a new image which will be rolled out onto the node |
| Kernel Argument Update | N | Enter the applyOSChanges update path, where the kernel args will be adjusted when new args are injected. The changes will be deployed by running “rpm-ostree kargs …” |
| Kernel Argument Update | Y | Enter the updateClusterBuild update path, where the new changes will be built into a new image which will be rolled out onto the node |
| Extension Change | N | Send out warning logs to notice the cluster admin that the change can not applied with bootc TP on but OCL off |
| Extension Change | Y | Enter the updateClusterBuild update path, where the new changes will be built into a new image which will be rolled out onto the node |
| Kernel Type Switch | N | Send out warning logs to notice the cluster admin that the change can not applied with bootc TP on but OCL off |
| Kernel Type Switch | Y | Enter the updateClusterBuild update path, where the new changes will be built into a new image which will be rolled out onto the node |



| zebra stripes | are neat      |    $1 |

### API Extensions

We don’t anticipate any user facing API extension changes as of now. We will update this section if something comes up during the smoke tests or implementation.

### Topology Considerations

#### Hypershift / Hosted Control Planes

None. Replacing the MCO install and update path to bootc should not change anything as it will follow the same workflow as rpm-ostree.

#### Standalone Clusters

None. Replacing the MCO install and update path to bootc should not change anything as it will follow the same workflow as rpm-ostree.

#### Single-node Deployments or MicroShift

None. Replacing the MCO install and update path to bootc should not change anything as it will follow the same workflow as rpm-ostree.

### Implementation Details/Notes/Constraints

Most of the details are highlighted in the sections above. Here are some things to keep in mind while implementing this enhancement:
    - Replace rpm-ostree calls with equivalent bootc calls in the MCD
    - Create an internal wrapper for bootc status reporting. Plan to switch to Go bindings once they are available for bootc

### Risks and Mitigations

None. Given that bootable container images are already supported in OCP, switching to use bootc instead of rpm-ostree for the update path shouldn’t be any different from what is currently done.

### Drawbacks

None

## Open Questions [optional]

* Is Go binding for bootc a necessity for adapting a bootc update path in the MCO? 
    - Discussion: Currently, bootc does not have go binding which will make it hard to read its status and deployment. It seems that we will need to create a Go library for client side interactions with bootc (similar to: https://github.com/coreos/rpmostree-client-go) Colin has suggested a way to auto-generate it (see comment in https://issues.redhat.com/browse/MCO-1026), which would be the most ideal solution. Workarounds should also be available, making go binding a non-goal item for this enhancement. 
    - Result: Go bindings for bootc is not a must and, as a result, will be outside of the scope of this enhancement and will not be implemented. The main purpose of creating go bindings for rpm-ostree/bootc is to have an easier way to parse the created json and to read the status of rpm-ostree/bootc. This can be done via creating a simple wrapper function inside of the MCO. It will make sense to separate it out to a standalone helper/library in the future when the demand raises, but is a non-goal for now.
* Certain config changes that the MCO watches don't trigger reboots, but it remains unknown if bootc can promise this too. 
    - Plan: Make a list of operations that you can do in the MCO that doesn’t require a reboot and ensure that bootc upholds that

## Test Plan

* Unit tests similar to what we have for rpm-ostree
* E2e tests similar to what we have for rpm-ostree
* Switch out the rpm-ostree backbone for bootc as shown in diagram below

## Graduation Criteria

We will extend this path behind a featuregate during the 4.17 timeframe. This will be GA iff (1) bootc can fully replace rpm-ostree in terms of functionality and becomes the main image update path within the MCO  (2) rpm-ostree is still kept but can be differentiated from the bootc update path to support image update for rpm-ostree mutated image / according to user’s preference. 

### Dev Preview -> Tech Preview

- Having CI testing for the feature

### Tech Preview -> GA

- Unit and E2E test coverage for the two criteria mentioned above 
- The GA of this enhancement is also dependent on (1) Openshift’s decision on whether bootc will be supported (2) bootc GA

### Removing a deprecated feature

None

## Upgrade / Downgrade Strategy

Currently, the MCO allows users to specify osImageURL Update / Extension Change / Kernel Argument Update / Kernel Type Switch for upgrade paths using rpm-ostree. With the bootc update path, users will still be able to set the same parameters and the MCO will use bootc instead to update the nodes to the new bootable image given.
This should not have a large impact as the MCO already supports upgrades via bootable images since 4.12 and the plan is to use bootc now instead of rpm-ostree to achieve the same thing.

## Version Skew Strategy

Not applicable.

Smooth transitioning from rpm-ostree to bootc is highly dependent on the base OS image’s compatibility with bootc. As RHCOS is also working towards an enhancement that takes care of bootc transitioning, there should be less of a concern from the MCO’s side to work with a bootc-enabled image. 

## Operational Aspects of API Extensions

None

## Support Procedures

Support for upgrades and handling failures will be the same as currently done for rpm-ostree. 
	
Failure mode: If the change applied by bootc is not in place, errors will be logged with the help of the bootc status wrapper in the Machine Config Daemon. 

## Alternatives

None

## Infrastructure Needed [optional]

None