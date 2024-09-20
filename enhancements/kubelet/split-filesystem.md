---
title: split-filesystem
authors:
  - kannon92
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - rphillips
  - haircommander
  - harche
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - mrunal
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2024-07-30
last-updated: 2024-07-30
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-2461
see-also:
  - "https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4191-split-image-filesystem/README.md"
---

# Split Filesystem

## Summary

Upstream Kubernetes has released [KEP-4191](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4191-split-image-filesystem/README.md).
This feature aims to allow one to separate the read-only layers (images) from the writeable layer of a container.
Upstream Kubernetes focused on allowing Kubelet garbage collection and eviction to work if the filesystem is split.

This enhancement focuses on enablement in Openshift.

## Motivation

See [KEP Motivation](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4191-split-image-filesystem/README.md#motivation)

### Definitions

### User Stories

#### Story 1

As an Openshift admin, I want to store images in a separate filesystem from ephemeral storage and the writeable layer.
The images can be on a read-only filesystem while ephemeral storage and the writeable layers can live on a writeable filesystem.

#### Story 2

As an Openshift admin, I want to store images on a dedicated disk that multiple nodes can share.

### Goals

- Enable ability to split filesystem in openshift
- Automate the setup of this feature to avoid user errors

### Non-Goals

- We will not support a separate disk for the entire container runtime filesystem due
to a lack of interest from customer requests.

- Customers will not have the ability to change the location of the image cache.

## Proposal

### Background

The main way to enable a split filesystem case is add `imageStore` in the container storage configuration.

See [container storage](https://github.com/containers/storage/blob/main/docs/containers-storage.conf.5.md).

```
imagestore="" The image storage path (the default is assumed to be the same as graphroot). Path of the imagestore, which is different from graphroot. By default, images in the storage library are stored in the graphroot. If imagestore is provided, newly pulled images will be stored in the imagestore location. All other storage continues to be stored in the graphroot. When using the overlay driver, images previously stored in the graphroot remain accessible. Internally, the storage library mounts graphroot as an additionalImageStore to allow this behavior.
```

Container storage is configured in openshift by adding this [file](https://github.com/openshift/machine-config-operator/blob/master/templates/common/_base/files/container-storage.yaml) to /etc/containers/storage.conf.

Container Runtime Config allows one to change the overlay size of storage. Other fields of this file are kept the same as the template.

### Dev Preview

#### Sketch of what happens when this feature is enabled

- A user specifies in the container runtime config that they want to split the filesystem (use this feature)
  - Feature gate is set for kubelet
  - container storage uses image store (hard coded to /var/lib/images)
  - /var/lib/images is relabeled to match the same selinux labels as /var/lib/container/storage.

#### Manual Enablement of Feature

In the developer preview, a user can run the following steps to enable this feature.

We will further refine the user APIs and interfaces for tech preview.

##### Feature gate

User can set the `KubeletSeparateDiskGC` feature gate in `CustomNoUpgrade.

```yaml
apiVersion: config.openshift.io/v1
kind: FeatureGate
metadata:
  name: cluster
spec:
  featureSet: "CustomNoUpgrade"
  customNoUpgrade:
    enabled:
      - KubeletSeparateDiskGC
```

#### Storage Configuration

One could use a butane config as follows:

```storage.bu
variant: openshift
version: 4.17.0
metadata:
  name: 40-storage-override
  labels:
    machineconfiguration.openshift.io/role: worker
storage:
  files:
  - path: /etc/containers/storage.conf
    mode: 0644
    overwrite: true
    contents:
      inline: |
        [storage]
        
        # Default Storage Driver
        driver = "overlay"
        
        runroot = "/var/containers/storage"
        graphroot = "/var/lib/containers/storage"
        imageroot = "/var/lib/images"
```

And then run `butane storage.bu -o storage.yaml

Applying storage.yaml will apply this machine config to your workers.

#### Prequisites for Filesystem

User must have a disk attached to the node and `imageroot` path is mounted on this disk.

#### Remove all old images

Since the image cache has changed locations, all the old images left over should be removed.

Simplest option is to remove the images on each node that this feature was enabled.

#### Checking if feature is enabled on a node

One can use `crictl imagefsinfo` to see if the filesystem is split. This will show imageFilesystems and containerFilesystems.

If they are split you would need a different mount in containerFilesystems.

#### Feature Gate

Since we are enabling only for Dev Preview, everything will be configured via MachineConfigs or other OpenShift APIs already created.

In tech preview, we will add feature gates in openshift/api.

### Tech Preview

Details will be filled out more for tech preview once dev preview has been released.


### GA

TBD

### Implementation Details/Notes/Constraints

## Open Questions

- Installer does not support creating openshift clusters with multiple disk.

- How does one delete all images and containers once the container runtime config is changed?
  - crictl on all images and containers on each node?

### Risks and Mitigations

In tech preview, we will automate many of the steps to mitigate problems.

### Drawbacks

Day 2 addition of disks in Openshift does not have the best user experience.

Adding a disk requires applying MachineConfigs which would trigger a reboot of the node.

It is not currently possible (as of Dev preview) to add disks on creation of the node.

## Test Plan

We have upstream tests where we run the conformance tests of node-e2e with this feature enabled.

We could follow a similar idea.

## Graduation Criteria

### Dev Preview Criteria

- Manual steps for enabling via MachineConfigs
- Blog post walking through this feature.

### Dev Preview -> Tech Preview

- Tech Preview will include the API changes.
- Telemetry will be included to verify if user is using this feature
- Installer support

### Tech Preview -> GA

Will update once we are ready to promote.

### Removing a deprecated feature

NA

## Operational Aspects of API Extensions

NA

## Upgrade / Downgrade Strategy

### Upgrade from feature off to feature on

Let's say scenario a does not have this feature enabled and CRI-O is not configured.

Let's say scenario b has this feature enabled.

If one wants to upgrade from scenario a to scenario b, cri-o will repull all images.
This is because the cache of the images will not be located in the same location and could cause some problems.

On a reboot of the node, all existing services will repull their images.

### Upgrade from feature on to feature on

Upgrades where the feature enablement stays the same should have no impact.

### Downgrading from feature on to feature off

Images will be pulled to `graphroot` location on downgrade. All images in imagestore will no longer be tracked and are effectively orphaned.
The disk will be safe to prune or unmount in a manual step.

### Workflow Description

NA

### API Extensions

NA

### Topology Considerations

NA

#### Hypershift / Hosted Control Planes

NA

#### Standalone Clusters

NA

#### Single-node Deployments or MicroShift

NA

## Version Skew Strategy

The support for this feature was merged into CRI in 4.15. However, this feature is only supported for 4.18 and above.

This is due to an issue in container/storage around the imagestore implementation. This feature was not backported to 4.17.

## Support Procedures

A common problem with changing container storage location is selinux permission denied errors.

If a container is failing due to the following error:

```bash
 error while loading shared libraries: /lib64/libc.so.6: cannot apply additional memory protection after relocation: Permission denied
```

One needs to check the labels for the imagestore and verify relabeling work. If not, a relabel is necessary.

## Alternatives

NA
