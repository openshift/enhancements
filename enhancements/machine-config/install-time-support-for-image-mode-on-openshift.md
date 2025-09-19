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
last-updated: 2025-07-25
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MCO-1347
see-also:
  - "https://github.com/openshift/enhancements/pull/1515/files?short_path=9f0c5f1#diff-9f0c5f1adabad0dfbdb3c9a5b66e53d4fc6619274d7a4c260508d148de17f5c1"
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

Image Mode on OpenShift became GA in OCP 4.18.z and has been well received as a post-install OS customization option. 

We want to bring that support to install time so cluster admins can customize and configure their OS from the beginning instead of having to do it post-install.

OpenShift's current approach to OS management provides excellent consistency and supportability through RHCOS (Red Hat CoreOS), but it requires cluster admins to cede certain configurability aspects to the platform. As workloads become more specialized there is an increasing need for OS-level customization that can be applied from Day 0. An obvious example today is how AI workloads require specific hardware drivers and configurations on nodes.

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
- Support custom OS image builds for master, worker, and any other custom pools during bootstrap
- Maintain OpenShift's single-click upgrade experience maintaining customizations across upgrades
- Ensure control plane and worker nodes can all use custom OS images
- Provide a declarative approach to OS customization through MachineOSConfig CRD
- Support only external image registry for image builds

### Non-Goals

- Replacing the existing ignition-based configuration system entirely
- Supporting arbitrary OS distributions beyond RHCOS-based images
- Providing external CI/CD integration for image builds (builds occur on-cluster)
- Supporting internal image registry on bootstrap node

## Proposal

### Workflow Description

The install time Image Mode on OpenShift workflow integrates with the existing OpenShift installation process in the following steps.

#### Step 1: Installation Configuration

1. Cluster admin creates an installation directory and runs `openshift-install create install-config` to generate the initial install configuration
2. Cluster admin specifies image mode enablement in `install-config.yaml` with `imageMode.enabled: true`
3. Cluster admin runs `openshift-install create manifests` to generate the base Kubernetes manifests
4. Cluster admin places `MachineOSConfig` (MOSC) YAML files in the `<install-dir>/openshift/` directory for master and worker MachineConfigPools
5. Cluster admin creates and places secret YAML files for image registry authentication (push/pull credentials) in the `<install-dir>/openshift/` directory
6. Cluster admin runs `openshift-install create cluster` which automatically:
   - Creates ignition configs by base64 encoding all YAML files from `manifests/` and `openshift/` directories
   - Embeds MOSC and secret YAMLs in `bootstrap.ign` for bootstrap-time application
   - Initially creates standard `master.ign` and `worker.ign` files with stock RHCOS configuration

#### Step 2: Bootstrap Phase and Image Building

1. Bootstrap node boots using `bootstrap.ign` and Ignition extracts all embedded YAML files to `/opt/openshift/openshift/` directory
2. The `bootkube.service` systemd unit starts and executes `bootkube.sh` script
3. `bootkube.sh` copies manifests from `/opt/openshift/openshift/` to `/etc/kubernetes/bootstrap-manifests/`
4. `bootkube.sh` runs the `cluster-bootstrap` container which applies all manifests via `kubectl apply`, creating:
   - MachineOSConfig resources in the cluster's etcd
   - Registry authentication secrets in the cluster
5. Bootstrap Machine Config Operator (MCO) starts and reads the existing MOSC resources from etcd
6. Bootstrap MCO creates MachineOSBuild (MOSB) resources for each MOSC
7. Bootstrap MCO establishes connection with the external image registry using provided secrets
8. Bootstrap MCO creates build pod to build custom OS image for master MachineConfigPool
9. Built image is pushed to the external registry with the respective `renderedImagePushSpec` value
10. **Installer dynamically updates `master.ign` and `worker.ign`** to include systemd units that will perform image transitions:
    - Master ignition gets `custom-image-transition.service` with `rpm-ostree rebase/bootc switch <master-image-pullspec>`
    - Worker ignition gets `custom-image-transition.service` with `rpm-ostree rebase/bootc switch <worker-image-pullspec>`
11. Bootstrap node provisions master nodes using the **updated** `master.ign` (includes image transition service)

#### Step 3: Master Node Handoff and Worker Processing

1. Master nodes boot with stock RHCOS using `master.ign` and join the existing cluster
2. Master nodes read the existing cluster state from etcd, including previously applied MOSC resources
3. Production MCO on master nodes takes over from bootstrap MCO and continues processing
4. Production MCO processes the worker MOSC configuration that was applied during bootstrap
5. Master nodes (running production MCO) create MachineOSBuild resources for worker pools
6. Master nodes establish connection with external registry using the authentication secrets
7. Build pods on master nodes construct custom OS images for worker MachineConfigPools
8. Built worker images are pushed to the external registry

#### Step 4: Worker Node Deployment

1. **Boot 1**: Master nodes provision worker nodes using **updated** `worker.ign` containing:
   - Stock RHCOS base system  
   - `custom-image-transition.service` systemd unit configured with the built worker image pullspec
2. Worker nodes start with stock RHCOS, kubelet starts, and nodes join the cluster
3. **Boot 2**: The `custom-image-transition.service` runs automatically after kubelet is ready:
   - Executes `bootc switch <worker-image-pullspec>`
   - Automatically reboots the node via `&& reboot` in the ExecStart command
4. Worker nodes boot into their custom-built OS images and rejoin the cluster
5. Cluster is now fully operational with all nodes running custom OS images built during Day 0

#### Notes and Restrictions:

- **Dynamic ignition updates**: Installer must update `master.ign` and `worker.ign` with image `renderedImagePushSpec` from MOSC yamls
- **Embedded image transition**: Custom systemd units are embedded directly in ignition files rather than applied via MachineConfigs
- **Reduced boot cycles**: Each node boots only twice (stock RHCOS → custom image)
- **External registry requirement**: Bootstrap node requires connectivity to external registry for pushing and pulling built images
- **Image build completion dependency**: Master and worker node provisioning must wait for their respective image builds to complete
- **Built-in automation**: Image transition happens automatically via systemd units without MCO intervention post-boot

### Topology Considerations

#### Hypershift / Hosted Control Planes

We currently don't have support for Hypershift with post-install Image Mode. Install time support for this will be added (if needed) once we have post-install support for it. This is currently not part of the implementation plan for this enhancement.

#### Standalone Clusters

Clusters in disconnected environments require special environment configurations such as a local package mirror, local image registry, etc.
Post-install Image Mode currently works for disconnected environments and should work in a similar manner for install time

#### SNO or MicroShift

Single-Node OpenShift (SNO) currently works fine with post-install Image Mode and should continue to work as-is for install time support as well. However, there may be some efficiency improvements that can be made. For example, the final image could be written directly to the nodes’ filesystem before the handoff to rpm-ostree / bootc to avoid a registry roundtrip or a minimal registry should be started on the bootstrap node to host the built image. These efficiency gains would be best handled as a separate enhancement.

### API Extensions

#### Install Config Extensions

```yaml
# install-config.yaml
apiVersion: v1
kind: InstallConfig
metadata:
  name: my-cluster
platform:
  # ... existing platform config
imageMode:
  enabled: true
  # Future: additional image mode configurations
```

### Implementation Details/Notes/Constraints

#### Bootstrap Process Integration

The bootstrap MCO will be enhanced to:

- Parse MOSC configurations from the installation directory
- Create MachineOSBuild (MOSB) objects for each MOSC
- Execute image builds using the existing build controller with a bootstrap flag
- Manage image registry operations (external registry required for this enhancement)

#### Build Process

- Images are built after the first rendered MachineConfig is created
- Bootstrap node handles master node image builds
- Control Plane nodes handle worker node image builds
- External registry is required for MVP (future enhancement may include bootstrap-hosted registry)

#### Registry Management

For MVP, an external registry is required to host built images.

The bootstrap node will:
- Push control plane node images to the external registry
- Provide image pull specifications for node deployment
- Handle authentication through provided secrets
- Create MachineConfig to override `osImageURL` to point to the build image for control plane nodes

The control plane nodes will:
- Push worker node images to the external registry
- Handle authentication through provided secrets
- Create MachineConfig to override `osImageURL` to point to the build image for worker nodes

#### Node Deployment Process

1. Nodes initially boot with stock RHCOS OS
2. Ignition configurations trigger `rpm-ostree rebase` or `bootc switch` to custom images
3. Nodes boot into custom OS images

#### MachineOSConfig (MOSC) Specifications

MOSC YAML files for both `master` and `worker` MachineConfigPools will be placed in the installation directory and parsed by the bootstrap MCO:

```yaml
# mosc-master.yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineOSConfig
metadata:
  name: master
spec:
  machineConfigPool:
    name: master
  containerFile:
  - content: |
      FROM configs AS final
      RUN dnf install -y specialized-driver
      # Additional customizations
  imageBuilder:
    imageBuilderType: Job
  renderedImagePushSpec: registry.example.com/custom-rhcos-master:latest
  renderedImagePushSecret:
    name: push-secret
```

```yaml
# mosc-worker.yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineOSConfig
metadata:
  name: worker
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

We will use the same MachineOSConfig CRD definition as already used for post-install customizations. The API for this can be found [here](https://github.com/openshift/api/blob/master/machineconfiguration/v1/types_machineosconfig.go).

To know more about the post-install image mode workflow, please refer to the enhancement [here](https://github.com/openshift/enhancements/pull/1515/files?short_path=9f0c5f1#diff-9f0c5f1adabad0dfbdb3c9a5b66e53d4fc6619274d7a4c260508d148de17f5c1).

### Risks and Mitigations

#### Risk: Build Failures During Installation

- Implement the same failure handling as existing installation failures
- Provide detailed guidelines on how to recover from failed installation based on the different errors
- Provide detailed logging and error reporting for image mode build failures
- Collect debug logs, metrics, and events for the install log bundle

#### Risk: Registry Connectivity Issues

- Validate registry connectivity and credentials during installation planning or bootstrap phase
- Provide clear error messages for registry-related failures
- Stop the install process as soon as we determine that we cannot connect to the registry

#### Risk: Image Size and Performance Impact

- Implement build queuing (maximum 2 concurrent builds for MVP)
- Provide guidelines for optimal containerfile practices

### Test Plan

#### Unit Tests

- Bootstrap MCO MOSC parsing logic
- Build controller bootstrap mode functionality
- Registry integration components

#### Integration Tests

- Complete install time image mode on OpenShift installation workflow
- Multiple machine config pool scenarios
- Build failure and recovery scenarios

#### E2E Tests

- Full cluster installation with custom OS images
- Upgrade scenarios with custom images

### Graduation Criteria

#### Tech Preview

- Basic install time support for image mode on OpenShift functionality with external registry
- Support for control plane and worker node pools
- Documentation and examples

#### GA

- Production-ready registry management
- Comprehensive failure handling and recovery
- Performance optimization
- Support for custom MachineConfigPools

### Upgrade / Downgrade Strategy

Install time Image Mode maintains compatibility with OpenShift's existing upgrade mechanisms:
- Custom OS images are preserved through cluster upgrades
- MOSC configurations are maintained and can be updated
- Rollback capabilities align with standard OpenShift procedures

### Version Skew Strategy

Install time Image Mode is designed to work within OpenShift's existing version skew policies:
- Custom OS images must be based on supported RHCOS versions
- Image builds use cluster-compatible base images
- Version compatibility is validated during the build process

## Implementation History

- 2025-07-25: Initial enhancement proposal
- 2025-XX-XX: Design review and approval
- 2025-XX-XX: Implementation begins

## Drawbacks

- Increased complexity in the installation process
- Additional resource requirements during bootstrap
- Dependency on external registry for MVP
- Potential for longer installation times due to image builds

## Alternatives

### Post-Installation Image Mode on OCP Only

Continue with the current approach of applying customizations after cluster installation. This approach is simpler but doesn't address the core need for install time customization.

### External Build Pipeline Integration

Integrate with external CI/CD systems for image builds before installation. This approach provides more flexibility but significantly increases complexity and dependencies.

### Ignition-Only Approach

Enhance the existing ignition system to support more complex customizations. This approach maintains current simplicity but has limitations for certain types of customizations.

## Infrastructure Needed

- Enhanced bootstrap MCO capabilities
- Registry management components
- Additional testing infrastructure for image mode scenarios
- Documentation and example updates
