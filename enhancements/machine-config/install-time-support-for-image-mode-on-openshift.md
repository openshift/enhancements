---
title: install-time-support-for-image-mode-on-openshift
authors:
  - "@umohnani8"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@mrunalp"
  - "@jlebon"
  - "@cheesesashimi"
  - "@yuqi-zhang"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@mrunalp"
  - "@jlebon"
  - "@cheesesashimi"
  - "@yuqi-zhang"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2025-07-25
last-updated: 2025-09-29
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MCO-1357
see-also:
  - "https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/on-cluster-layering.md"
replaces:
  - ""
superseded-by:
  - ""
---

# Install Time Support for Image Mode on OpenShift

## Summary

This enhancement proposes adding install time support for Image Mode on Openshift, enabling cluster admins to customize the operating system layer during initial cluster installation.
This feature allows users to build and deploy custom OS images containing additional software, drivers, and configurations from the very beginning of the cluster lifecycle, eliminating the need for post-installation customization and ensuring all nodes start with the desired configuration.

## Motivation

Image Mode on OpenShift became GA in OCP 4.18.21+ and has been well received as a post-install OS customization option. 

We want to bring that support to install time so cluster admins can customize and configure their OS from the beginning instead of having to do it post-install.

OpenShift's current approach to OS management provides excellent consistency and supportability through RHCOS (Red Hat CoreOS), but it requires cluster admins to cede certain configurability aspects to the platform. As workloads become more specialized there is an increasing need for OS-level customization that can be applied from install time. An obvious example today is how AI workloads require specific hardware drivers and configurations on nodes.

Some current challenges that Install Time Support For Image Mode on OpenShift would solve:
- Inability to customize OS images during initial cluster installation
- Post-installation customization complexity and timing issues
- Limited support for specialized hardware requirements during installation

### User Stories

#### Story 1: AI/ML Workload Deployment

As a cluster admin deploying AI workloads, I want to include specialized GPU drivers and AI frameworks in my OS image at install time, so that my cluster is ready for AI workloads from the moment it comes online, without requiring post-installation configuration.

#### Story 2: Regulated Environment Deployment

As a cluster admin in a regulated environment, I want to include security agents and compliance tools in my OS image during installation, so that my cluster meets compliance requirements from the moment it comes online, without requiring post-installation configuration.

#### Story 3: Edge Computing Deployment

As a cluster admin deploying edge computing solutions, I want to include edge-specific drivers and monitoring agents in my OS image during installation, so that my edge nodes are immediately functional without requiring network connectivity for post-installation customization.

### Goals

- Enable cluster admins to use Image Mode on OpenShift at install time
- Ensure all Machine Config Pools (control plane, worker, any custom pools) can all use custom OS images
- Ensure that the customizations are carried over with any day-2 updates
- Provide a declarative approach to OS customization through MachineOSConfig CRD

### Non-Goals

- Replacing the existing ignition-based configuration system entirely
- Supporting arbitrary OS distributions beyond RHCOS-based images
- Providing external CI/CD integration for image builds
- Enabling the custom OS container image to be built during bootstrap
- Enabling the use of a custom disk image at install time
- Reducing the number of reboots a node goes through (this is being handled in a separate enhancement)

## Proposal

### Workflow Description

The install time Image Mode on OpenShift workflow integrates with the existing OpenShift installation process in the following steps.

#### Step 1: Installation Configuration with Pre-Built Container Image

1. Cluster admin builds the customized OS container image and pushes it to a registry to be used at install time
2. Cluster admin adds the `pull secret` needed to pull the container image to the cluster's `global pull-secret`
3. Cluster admin runs `openshift-install create manifests` to generate the base Kubernetes manifests
4. Cluster admin places `MachineOSConfig` (MOSC) YAML files in the `<install-dir>/manifests/` directory for any MachineConfigPools they want to use the Image Mode workflow for
    - MOSC has annotation pointing to the pre-built container image (seeding the image to be used at install time)
    - MOSC has the custom Containerfile used when building the custom OS container image in step 1
    - MOSC has `pushSecret` set with the correct credentials needed to push any new images built post install to the registry
5. Cluster admin runs `openshift-install create cluster` which creates the cluster using the custom MOSC files added by the user

#### Step 2: Bootstrap Phase

1. Bootstrap MCO recognizes MachineOSConfig manifests and stores them for later API creation
2. Bootstrap MCO creates rendered MachineConfigs for each pool
3. Bootstrap MCO then does the following to inject the pre-built container image into the rendered MC
    - Maps MachineOSConfigs with pre-built annotations to their target pools
    - Validates pre-built image format (digest format required)
    - Sets `OSImageURL = preBuiltImage` by creating a custom MC called `10-prebuildimage-osimageurl-<pool>` for each pool that has a MachineOSConfig
4. Bootstrap MCO creates ignition files with rendered MCs containing `OSImageURL` pointing to the pre-built container image
5. The MCD first boot service detects the different `OSImageURL` value and uses `rpm-ostree rebase` to rebase the node to the custom pre-built container image
6. After the cluster bootstrap completes, MCO starts and begins post-bootstrap processing

#### Step 3: Post-Bootstrap Seeding Phase

1. Build controller detects the `machineconfiguration.openshift.io/pre-built-image` annotation on the newly created MachineOSConfig
2. Build controller triggers seeding workflow by calling `seedMachineOSConfigWithExistingImage()``
3. MCO creates a synthetic MachineOSBuild with:
    - Successful status referencing the seeded pre-built OS container image
    - `PreBuiltImage` label set to "true"
    - Proper metadata including MOSC name, DigestedImagePushSpec, and MCP reference
    - Success condition with reason "PreBuiltImageSeeded"
4. MCO updates MachineOSConfig status with `CurrentImagePullSpec` pointing to the pre-built image
    - This creates consistency between the actual node state (already running pre-built image) and the MCO API objects for future OCL workflows

#### Step 4: Post-Install Integration

1. Any future updates/changes will use the current post-install workflow to trigger a new image build and roll it out onto the nodes based on the MachineOSConfig created at install time

#### Notes and Restrictions:

- A pre-built custom OS container image is needed. Image build during bootstrap is not supported
- The Containerfile used to build the image needs to be added to the MachineOSConfig CRs created at install time
- The registry where the pre-built image is stored will need to be accessible to the cluster in future as any future builds will be pushed there and pulled from there when rolling out onto the nodes

### Topology Considerations

#### Hypershift / Hosted Control Planes

We currently don't have support for Hypershift with post-install Image Mode. Install time support for this will be added (if needed) once we have post-install support for it. This is currently not part of the implementation plan for this enhancement.

#### Standalone Clusters

Clusters in disconnected environments require special environment configurations such as a local package mirror, local image registry, etc.
Post-install Image Mode currently works for disconnected environments and should work in a similar manner for install time

#### Single-node Deployments or MicroShift

Single-Node OpenShift (SNO) currently works fine with post-install Image Mode and should continue to work as-is for install time support as well.

#### OpenShift Kubernetes Engine

OKE should not be affected by this.

### API Extensions

None

### Implementation Details/Notes/Constraints

#### Bootstrap Process Integration

The bootstrap MCO will be enhanced to:

- Parse MOSC configurations from the installation directory
- Update the MCD first boot service to use the seeded pre-built custom OS container image

#### Node Deployment Process

1. Nodes initially boot with stock RHCOS OS
2. MCD first boot service triggers `rpm-ostree rebase` or `bootc switch` to custom images
3. Nodes boot into custom OS container images

#### Post-install Integration

1. A "fake" MachineOSBuild is created for the seeded pre-built container image
2. This MOSB is set to successful state so the build controller is able to keep track of what needs to happen for any future updates or reverts
3. The MOSC added at install time is used to maintain the customizations across updates

#### MachineOSConfig (MOSC) Specifications

MOSC YAML files for both `master` and `worker` MachineConfigPools will be placed in the installation directory and parsed by the bootstrap MCO. Here is an example:

```yaml
# mosc-worker.yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineOSConfig
metadata:
  name: worker
  annotations:      
    # Key annotation that triggers hybrid workflow                
    machineconfiguration.openshift.io/pre-built-image: "registry.example.com/custom-rhcos-worker:latest@sha256:abc123def456..."
    # Added by build controller after seeding         
    machineconfiguration.openshift.io/current-machine-os-build: "worker-abc123"
spec:
  machineConfigPool:
    name: worker
  containerFile:
  - content: |
      FROM configs AS final
      RUN dnf install -y special-worker-drivers
      # Additional customizations
  imageBuilder:
    imageBuilderType: Job
  renderedImagePushSpec: registry.example.com/custom-rhcos-worker:latest
  renderedImagePushSecret:
    name: push-secret
```

Here is an example of the "fake" MachineOSBuild generated after the cluster has been installed:

```yaml
apiVersion: machineconfiguration.openshift.io/v1                                 
  kind: MachineOSBuild                                             
  metadata:                                           
    name: "worker-abc123"       
    labels:                          
      machineconfiguration.openshift.io/machineosconfig: "worker"
      machineconfiguration.openshift.io/target-machine-config-pool: "worker"
      # Special label marking this as synthetic       
      machineconfiguration.openshift.io/pre-built-image: "true"        
    annotations:         
      machineconfiguration.openshift.io/machine-os-config: "worker"
  spec:                        
    desiredConfig:                       
      name: "rendered-worker-abc123def"                 
    machineOSConfig:                
      name: "worker"              
    renderedImagePushspec: "registry.example.com/custom-rhcos-worker:latest"
  status:                        
    buildStart: "2024-01-15T10:29:55Z"                       
    buildEnd: "2024-01-15T10:30:00Z"                  
    # The pre-built image is set as the build result               
    digestedImagePushSpec: "registry.example.com/custom-rhcos-worker:latest@sha256:abc123def456..."
```


We will use the same MachineOSConfig and MachineOSBuild CRD definitions as already used for post-install customizations with some new annotations. The API for this can be found [here](https://github.com/openshift/api/blob/master/machineconfiguration/v1).

To know more about the post-install image mode workflow, please refer to the enhancement [here](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/on-cluster-layering.md).

### Risks and Mitigations

#### Failures During Installation

- Implement the same failure handling as existing installation failures
- Provide detailed guidelines on how to recover from failed installation based on the different errors
- Provide detailed logging and error reporting for image mode build failures
- Collect debug logs, metrics, and events for the install log bundle

#### Risk: Registry Connectivity Issues

- Validate registry connectivity and credentials during installation planning or bootstrap phase
- Provide clear error messages for registry-related failures
- Stop the install process as soon as we determine that we cannot connect to the registry

## Test Plan

#### Unit Tests

- Bootstrap MCO MOSC parsing logic
- Build controller bootstrap mode functionality
- Registry integration components

#### Integration Tests

- Complete install time image mode on OpenShift installation workflow
- Multiple machine config pool scenarios

#### E2E Tests

- Full cluster installation with custom OS images
- Upgrade scenarios with custom images
- Currently have a periodic job testing the Day 0 workflow

## Graduation Criteria

### Dev Preview -> Tech Preview

- Basic install time support for image mode on OpenShift functionality with external registry
- Support for control plane and worker node pools
- Documentation and examples

### Tech Preview -> GA

- Comprehensive failure handling and recovery
- Performance optimization
- Support for custom MachineConfigPools

### Removing a deprecated feature

None

## Upgrade / Downgrade Strategy

Install time Image Mode maintains compatibility with OpenShift's existing upgrade mechanisms:
- Custom OS images are preserved through cluster upgrades
- MOSC configurations are maintained and can be updated
- Rollback capabilities align with standard OpenShift procedures

## Version Skew Strategy

Install time Image Mode is designed to work within OpenShift's existing version skew policies:
- Custom OS images must be based on supported RHCOS versions
- Image builds use cluster-compatible base images
- Version compatibility is validated during the bootstrap process

## Operational Aspects of API Extensions

None

## Support Procedures

1. Clear documentation and tutorial on how to get started with this, highlighting all the resources needed such as pre-built image, Containerfile, MachineOSConfig yamls
2. Verbose bootstrap logs to help with debugging when failures occur
3. Add validations at bootstrap phase to verify the container image provided is valid and that the registry is accessible

## Implementation History

- 2025-07-25: Initial enhancement proposal
- 2025-XX-XX: Design review and approval
- 2025-XX-XX: Implementation begins

### Drawbacks

- Increased complexity in the installation process
- Additional resource requirements during bootstrap
- Dependency on external registry for MVP

## Alternatives (Not Implemented)

### Post-Installation Image Mode on OCP Only

Continue with the current approach of applying customizations after cluster installation. This approach is simpler but doesn't address the core need for install time customization.

### External Build Pipeline Integration

Integrate with external CI/CD systems for image builds before installation. This approach provides more flexibility but significantly increases complexity and dependencies.

### Ignition-Only Approach

Enhance the existing ignition system to support more complex customizations. This approach maintains current simplicity but has limitations for certain types of customizations.

## Infrastructure Needed

- Enhanced bootstrap MCO capabilities
- Additional testing infrastructure for image mode scenarios
- Documentation and example updates
