---
title: alignment-with-image-mode-rhel-for-the-mco

authors:
  - "@umohnani8"
  - "@inesqyx"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@cheesesashimi"
  - "@yuqi-zhang"
  - "@jlebon"
  - "@travier"
  - "@mrunalp"
  - "@cgwalters"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@yuqi-zhang"
  - "@mrunalp"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2024-05-28
last-updated: 2024-07-24
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
see-also:
replaces:
superseded-by:
---

# Bootc Update Path 

## Summary

This enhancement outlines a three-phase plan to fully adapt an image-based update path and make it the default for the MCO. Keeping synced with CoreOS, MCO currently utilizes an rpm-ostree-based system to engage in base image specification and local package layering. At the same time, the machine config daemon (MCD) also monitors updates by reading machine configs and writing the configuration to each node individually. Moving forward, we will prepare the MCO to be able to automatically produce a derived image from the specified OS image which contains all of the files and configurations from a given MachineConfig as well as a cluster admins custom modifications whenever the base OS image changes, push the image to a user specified container registry, and reboot each node to use the new image using a “bootc switch”. 

## Motivation

In older RHCOS versions we have a “non-layered” update path, then there is the rpm-ostree path. Now with Image Mode RHEL, there is a strong call for a full image-based update path. Partnering with CoreOS, MCO is also committing to its own mission to ensure OCP uses image mode and can benefit from image mode. A key advantage of converging to image mode is that layered OS images will become the source of truth for configuration that the nodes are booted to, rolled back to, and applied with. This will efficiently prevent nodes from degrading due to config drift. Keeping changes into separate image layers will also make image inspection, testing, and scanning easier. We can foresee that, down the journey, there will be more room for customization as well through bootc’s configmap support.

### User Stories

* As an OpenShift cluster administrator I would like MCO to leverage image-mode RHEL to boot/update my nodes
* As an Openshift administrator, I would like to reset a config I have applied without manual deletion that may cause node degradation, but via an image rollback.
* As an OpenShift cluster administrator, I would like my cluster to understand and to be able to (automatically) target a new Openshift/RHCOS container image reference to boot for upgrade.
* As an Openshift support engineer, I would like to easily reproduce the settings and issues my client runs into by booting according to their image queue. 

### Goals

* Future Day-2 configuration owned by the MCO will, by default, involve building container images from config specification and layer the image by bootc switch. 
* Enable the user to upgrade their cluster to a rpm-ostree-based image-mode cluster and to a bootc-based image-mode cluster seamlessly 

### Non-Goals

* On Cluster Layering [Discussed in detail in https://github.com/openshift/enhancements/pull/1515]
* Bootc Day-2 Configuration Tools: new bootc features like the [planned configmap support](https://github.com/containers/bootc/issues/22) are of interest to the MCO but not in scope in this initial work to align with image-mode for RHEL.
* Splitting the RHCOS Image into layers [Discussed in detail in https://github.com/openshift/enhancements/pull/1637]

## Proposal

The efforts needed to fully align the MCO with Image Mode RHEL in 4.19 or 4.20 can be divided into three phases, each building upon the previous one. In the section below, we include a detailed description and dependencies for each phase. 

### Phase 1: On Cluster Layering GA

On-Cluster Layering [Discussed in detail in https://github.com/openshift/enhancements/pull/1515] introduces a seamless image-based configuration flow with details summarized as follows: “perform an on-cluster build (OCB) to combine the base OS image shipped with OpenShift, any admin-provided custom content, and the MachineConfig content managed by the MCO. This image is then deployed to all of the cluster nodes using rpm-ostree (and later, bootc) to facilitate a singular, transactional, update”

On-Cluster Layering is already shipped as a tech preview version in 4.16. Currently, we are targeting 4.18 for its GA. In this phase, we are aiming that, by the time On Cluster Layering GA, it will support most day-2 configurations specified by machine config and ship the change as a container image layer, which include osImageURL specification (DONE), extensions and kernel changes (PLANNED) and file writing to mutable local file system directories (DONE). Another important GA criteria for On Cluster Layering and Phase 1 is that, currently, On Cluster Layering is only a day-2 configuration tool for Openshift, with the workflow being that you install a cluster and then opt in to it. We need to figure out an On Cluster Layering day-1 configuration story so that users can install a cluster with it.

### Phase 2: Move to Layered Update Path by Default 

After On Clustering GA, we will gradually deprecate the current configuration method of direct file writing to disk and move to layered update path (Phase 1) by default. At the end of this phase, OCP nodes will be configured in a full image-based way and will be built by layering day-2 configuration on top of the node image and rolled out by On Cluster Layering discussed in Phase 1. Some of the gaps we observed here are:
  * "Default" MCO managed configuration moved to /usr and shipped as a container image layer: Includes all MCO specific config rendered to /usr + MachineConfigs which are rendered to /etc remain in /etc where an equivalent /usr path is not available.
  * MachineConfigs that write to `/var` (this includes things like SSH keys under `/home`, which is really a symlink to `/var/home`) will need to keep being written by the MCO directly. In cases where we own the MachineConfig, we can look at whether there are `/usr` or `/etc` alternatives (or e.g. `tmpfiles.d` dropins) we can use, but in the general case and for customers, anything written to `/var` can't be part of the layered image build since ostree by design ignores `/var` content when rolling out the update.
  * Address functionality gap between bootc and rpm-ostree: figure out a story for kernel argument changes as a container image layer/label
    - There is an update in support for this by bootc here https://containers.github.io/bootc/building/kernel-arguments.html.
    - The MCO team did a spike investigation on how to support kernel args with bootc and the findings and planned solution can be found in this [jira card](https://issues.redhat.com/browse/MCO-1244).
  * Carve a path for changes made to nodes without applying MachineConfig like changes made by other OCP operators (e.g. SRIO-V) or MCO cert writing.

### Phase 3: Deprecate rpm-ostree and Move Layered Image Update Path to Bootc 

Upon completion of the above, the last step is to update the image rollout mechanism. In the foreseeable future, we will be expecting a dual support RHCOS where rpm-ostree will be gradually deprecated and transition its responsibility to bootc. Following this trend, MCO will also enable image deployment by both “rpm-ostree rebase” and “bootc switch” and, in the end, settle down on “bootc switch” for image rollout.

### Workflow Description

1. The cluster administrator wants to add new configuration to the OS on each of their cluster nodes.
2. The cluster administrator specifies base OS image pull spec & secret, base OS extension pull spec, rendered image pull spec & secret, rendered image push spec & secret and custom content through a MachineOSConfig object.
3. The cluster administrator specifies machine configuration through MachineConfig object.
4. The Machine Config Controller will read the MachineConfig, check reconcilability and render a rendered-MachineConfig. 
5. The BuildController detects a build request from new MachineOSConfig/MachineConfig creation and creates a build. A new MachineOSBuild object will be created to track build progress and mark targeted rendered-MachineConfig.
6. The built image will be pushed to the rendered image push spec specified in the MachineOSConfig object (Step 2).
7. The MachineConfigDaemon will then drain the node and call “bootc switch” to roll the newly built image to the node, and reboot the node upon completion.
8. If required, cluster administrator can roll back to previous image by deleting the MachineOSConfig/MAchineConfig object that was created to trigger the build (Step 5).

### API Extensions

[TBD] We don’t anticipate any user facing API extension changes as of now. We will update this section if something comes up during the smoke tests, implementation or future planning.

### Topology Considerations

#### Hypershift / Hosted Control Planes

[TBD] We don’t anticipate any impact on Hypershift as of now. We will update this section if something comes up during the smoke tests, implementation or future planning.

#### Standalone Clusters

[TBD] We don’t anticipate any impact on Standalone Clusters as of now. We will update this section if something comes up during the smoke tests, implementation or future planning.

#### Single-node Deployments or MicroShift

[TBD] We don’t anticipate any impact on Single-node Deployments or MicroShift as of now. We will update this section if something comes up during the smoke tests, implementation or future planning.

### Implementation Details/Notes/Constraints

Phase 1: Please refer to On Cluster Layering enhancement for more details  
Phase 2: [TBD] 
Phase 3:
  * Setup bootc go-bindings for status reading and command execution 
  * Replace rpm-ostree rebase by bootc switch for image rollout 

### Risks and Mitigations

[TBD]

### Drawbacks

[TBD]

## Open Questions [optional]

* Is Go binding for bootc a necessity for adapting a bootc update path in the MCO? 
    - Q: Currently, bootc does not have go binding which will make it hard to read its status and deployment. It seems that we will need to create a Go library for client side interactions with bootc (similar to: https://github.com/coreos/rpmostree-client-go) Colin has suggested a way to auto-generate it (see comment in https://issues.redhat.com/browse/MCO-1026), which would be the most ideal solution. Workarounds should also be available, making go binding a non-goal item for this enhancement. 
    - A: Go bindings for bootc is not a must and, as a result, will be outside of the scope of this enhancement and will not be implemented. The main purpose of creating go bindings for rpm-ostree/bootc is to have an easier way to parse the created json and to read the status of rpm-ostree/bootc. This can be done via creating a simple wrapper function inside of the MCO. It will make sense to separate it out to a standalone helper/library in the future when the demand raises, but is a non-goal for now.

* Discussion question for gaps highlighted in phase 2 
    - Q: A problem with the current model is that configs that involve file writing to local file system directories are not respected by “rpm-ostree rebase” / “bootc switch”. For example, if we write an SSH key to “/home/core/.ssh/authorized_keys'' directory during an OS image build and use rpm-ostree to rebase onto the newly built OS image, that file will not actually be there when the node reboots. Will this be handled/considered in the future planning of RHCOS and bootc? 
    - A: 
        - rpm-ostree/bootc does not touch /home (right now, might in the future with config maps)
        - Bootc is working on an enhancement[https://containers.github.io/bootc/building/guidance.html?highlight=configma#configuration] to support dynamically-injected ConfigMaps. This enhancement will help handle this case by directly mounting configuration to the disk so there won’t be the need to update and re-deploy the image.
        - For the time being, the MCO will have to keep some of the file writing workflow in place. In the future, plan to move these files to /usr or /etc or use systemd-tmpfiles dropins.

* Questions raised alongside “moving to layered update path by default”:
    - Q: 
        - Should all config be built into the image? 
        - What will the update path look like if certain config changes are not baked into the image because (1) We normally don’t write them through MachineConfig (e.g. cert writings) (2) Less reboot is favored by Node Disruption Policies?
        - Certain config changes that the MCO watches don't trigger reboots, but it remains unknown if “moving to layered update path by default” can promise this too.
    - A: 
        - Everything rebootless should not be in the image, unless we have live image changes. It is better for those changes to be in writable directories on the disk. Need to keep in mind the priority order of /usr vs /etc.
        - MCO would have to apply its own internal configs first then the MachineConfigs.
        - [Actionable item] Get a list of files managed by the MCO to further refine this.
        - In the future, see if bootc can handle this instead based on the decision and implementation around https://github.com/containers/bootc/issues/76.

## Test Plan

* Phase 1: Please refer to On Cluster Layering enhancement for more details  
* Phase 2: TBD 
* Phase 3 : Bootc Switch Over 
    - Develop an e2e test suite and run payloads with bootc: Tests should target the TestOSImageURL, TestKernelArguments, TestKernelType and TestExtensions. Results will be used to discuss whether bootc is ready to perform rpm-ostree’s tasks.
    - Create a bootc experimentation sandbox by having the BootcNodeManagement FG: When the FG is on, the rpm-ostree commands will be replaced by equivalent bootc commands to manually test the capability of bootc in non-layered machine configuration workflow. Switch back to rpm-ostree as the default option when not compatible. This experimentation prepares for Scenario 1 in the following section. 
    - Test bootc with On Cluster Layering: When the BootcNodeManagement and OnClusterBuild FG is on, On Cluster Layering will be the default path for machine configuration, accompanied by bootc which is responsible for image roll out. This test prepares for Scenario 2 in the following section.   


## Graduation Criteria

### Dev Preview -> Tech Preview

There are three cases we should consider and try to support under tech preview. These can be viewed as stages in which we add more and more support for layered update path and bootc.
  * Stage 1: We don’t have layered update path as the default, but we start to provide rpm-ostree + bootc dual support (likely in 4.18)
  * Stage 2: We now have layered update path as the default and we still have rpm-ostree + bootc dual support (likely in 4.19)
  * Stage 3: We have completed the switch over to layered update path + bootc for image management + dnf for package management (likely in  4.20)

### Tech Preview -> GA

* Unit and E2E test coverage for the two criteria mentioned above 
* The GA of this enhancement is also dependent on:
    - Bootc GA
    - On Cluster Layering GA 
    - Move to Layered Update Path by Default GA
    - Move to dnf Interface for RHCOS Package Install 
    - Local Package Layering on Bootc-based RHCOS 
* The GA of this enhancement will work with a dual support RHCOS 

### Removing a deprecated feature

Once layer update path is the default with bootc, we should be able to deprecate rpm-ostree.

## Upgrade / Downgrade Strategy

[TBD]

## Version Skew Strategy

[TBD]

## Operational Aspects of API Extensions

None

## Support Procedures

[TBD]

## Alternatives

None

## Infrastructure Needed [optional]

None