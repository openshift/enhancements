---
title: on-cluster-build-api
authors:
  - "@cdoern"
reviewers:
  - "@cheesesashimi"
  - "@dkhater-redhat"
  - "@inesqyx"
approvers:
  - "@sinnykumari"
  - "@cheesesashimi"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-02-26
last-updated: 2024-02-26
tracking-link:
  - https://issues.redhat.com/browse/MCO-665
---

# Track MCO Managed Builds and Images in the MachineOSImage and MachineOSBuild type

## Summary

This enhancement describes the user facing API for On-Cluster-Builds. Currently, there is no way to track the in-depth state of a build. The only mechanism the MCO has is a few additional MCP level condition types. Not only is state hard to follow but setup of an On-Cluster-Build system involves creating 2 secrets and 2 seprate configmaps referencing these secrets. 

With this API we aim to consolidate both the configuration of our image builds and the state pertaining to the build itself as well as the resulting image which the MCO then manages. a MachineOSBuild will in-turn cause the creation of a MachineOSImage. Since the MachineOSImage is created during an update process, we will also need to augment the MachineConfigNode object to be aware of these types of updates.

A MachineOSBuild will contain the image push and pull secrets for access to the registry and containerfile data for a user to specify custom data they want built into the image. The other fields will be set by the MCO mainly the name of the resulting machineOSImage. In the Status we will have conditions, the build start and end time, related failed/interrupted builds and why they failed, and the final pull spec of the created image.

a MachineOSImage will have the base image, the pull spec, the MCP the image is associated with and the rendered MachineConfig this image is based off of all in the spec. The status will contain an observed generation, conditions, an image age, a custom "rollout status" to tell the user where the image is being rolled out and how it is going, and finally an "image usage status" detailing the part of the lifecycle this image is in for garbage collection.

## Motivation

With On-Cluster-Builds approaching Tech-Preview and GA, the user experience is a huge topic. Our current workflow creates confusion at times and needs detailed instructions to be used. The aim here is to make the workflow as "OCP-Native" as possible and as centralized as possible. These builds and images have a complex management system behind the scenes which needs to be exposed or users to adequately manage their updates.

### User Stories

* As a cluster admin, when rolling out new OS images, I would like easily configure the On-Cluster-Build system.
* As a cluster damin, when rolling out new OS Images, I would like to see the status of an MCO managed build and detailed information about the resulting managed images.


### Goals

* Make the MachineOSBuild and MachineOSImage types that store config and track state.

### Non-Goals

* Replace the MCN or an other MCO object's functionality. We want to leverage our existing APIs.


## Proposal

Create the MachineOSBuild and MachineOSImage datatypes for storing on-cluster-build config and for tracking the state of builds, image age, and image health.

The MachineOSBuilder will own these types sine it manages the build and rollout process. The MCD might have some updating ability or at least retreival ability since it does much of the actual updating on the nodes. 

A MachineOSBuild will be where the user stores their registry credentials and their custom Containerfile content. The rest of the spec and of course the status will be used by the MCO to inform the user about their build.

The creation of one of these MachineOSBuild objects will be the trigger for a build. Once the build has been prepared, a machineOSImage object will be created to track the Image. If the build fails, the actual image build will be linked to the MachineOSBuild and the failure reason will be attached to it before triggering a rebuild.

a MachineOSBuild will have the following condition types for state reporting:  BuildPrepared, Building, BuildFailed, BuildInterrupted, BuildRestarted, and Built. Besides these, the status will report the build start and end times, as well as the Related (failed) builds, and the Final Image Pullspec.

a MachineOSImage will have the following conditions to track its rollout: RolledOut, RolloutPending, RolloutFailed. 
Each MachineOSImage will also have a column detailing what MCP it is associated with, its age, and what part of its lifecycle it is in. The available lifecycle types are: InUse, Stale, and Rotten.


### Workflow Description

An admin wants to bake some custom content into a pool. Or, an admin just wants to take advantage of the image based workflow and switch to using on-cluster-builds. This admin creates a MachineOSBuild targeted at MachineConfigPool worker. This pool needs to have the "layering-enabled" label on it to work. 

This will trigger the MachineOSBuilder to gather the sources for the build and set the new object to BuildPreparing. The image push and pullspec specified in the spec of the newly created MachineOSBuild object will be used to pull image sources and push the image to a resulting registry. The build start time will be set to the current time.

The Build will progress, and once we get past the validation and preparation phases, we will create a MachineOSImage to reference the attempted build. Its age will be 0 until the image actually exists. The MachineOSBuild will go through the Building Phase and end up on Built if it succeeds. Once this happens, the Age of the image will be instantiated and we will focus on Rolling out the image to the proper nodes.

The MachineOSImage will be in the RolloutPending status until all nodes in the pool are booted with the proper image. This will require the Daemon's help. The Daemon will most likely need to own this field on the Status. Or, we could add some watchers to the MachineOSBuilder to watch for this change. Once the image is rolled out, the usage of the MachineOSImage is set to InUse. If the image is ever replaced it is set to Stale. An image is rotten after it gets above a certain age. Once an image has two newer versions of it rolled out to a pool, an image is Rotten and will be garbage collected unless the user specified otherwise in the CR.

If the Build fails, the associated MachineOSImage and/or actual image pullspec will be kept in the RelatedImages struct in the status along with a reason for failure. the BuildCondition BuildFailed will also be true until the next Build is kicked off.

If the Build is interrupted by the user, that Build is also added the RelatedImages struct with reasoning. BuildInterrupted and BuildRestarted will be true until BuildPreparing is set to true in the subsequent build cycle.

### API Extensions

- Adds the MachineOSBuilds and MachineOSImages CRDs

### Risks and Mitigations

Current users of OCB might be familiar with the existing workflow. For the time being, those will also work. We can not immediately deprecate the configmap and MCP approach. We will need to phase these out over a few releases.


## Design Details

### Open Questions [optional]

None.

### Test Plan

MCO will add e2e tests to check the sanity of these structures and to make sure they properly trigger a build.

### Graduation Criteria

This will be TechPreview in 4.16 until Qe has had a chance to test upgrade paths between the old method and this new method of configuring On-Cluster-Builds.

## Dev Preview -> Tech Preview

Not applicable. Feature introduced in Tech Preview. 

## Tech Preview -> GA

Fix bugs associated with this new OCB API in 4.16 before GA.


### Upgrade / Downgrade Strategy

Between Upgrades, since the only method still exists, this should have minimal impact. Though, It will not hurt to add a migration strategy for users who have active configmaps. We can take the data in those MCO configmaps and create MachineOSBuilds out of them.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

#### Failure Modes

If the API is unreachable, we should fail. Similarly to how the Configmaps would be unreachable these new CRs will also be unreachable. This should be a fatal error.

#### Support Procedures

None.

## Implementation History

None.

## Alternatives

None.
