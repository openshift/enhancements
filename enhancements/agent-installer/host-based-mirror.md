---
title: host-based-mirror
authors:
  - "@bfournie"
reviewers:
  - "@andfasano"
  - "@patrickdillon"
  - "@romfreiman"
  - "@rwsu"
approvers:
  - "@zaneb"
api-approvers:
  - None
creation-date: 2023-02-03
last-updated: 2023-02-16
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/AGENT-262
---

# Host Based Mirror

## Summary

The agent based installer should have the ability to manage all container
images needed to boot a host in a disconnected environment without requiring
an external mirror registry residing on a separate server.

## Motivation

In a disconnected environment, a user must set up and maintain a local registry
on a separate server in order to provide container images to the hosts being
booted. In addition to the extra hardware, this step requires that the user be
familiar with both how to create the registry and the tools used to mirror the
images, e.g. [oc-mirror](https://github.com/openshift/oc-mirror).  The content
of the local registry must also be synchronized with the release image provided
in the manifests files and also eventually with the OLM operators required and
any additonal user-specific files.  Whenever there is an update to the release
image the local registry must also be updated.  Finally the user must add an
ImageContentSources section to the install-config.yaml to define the registry
sources and mirrors. This is error-prone and often results in misconfiguration
and failed installations.

This enhancement would remove the need for an additional server and
reduce the burden on the user when using the agent based installer in a
disconnected environment.

### User Stories

- As a user creating an initial Openshift cluster in a disconnected
  environment on Day 1, I do not want to create and manage a registry to
  provide images for the cluster. As I may not have access to a server to run
  a registry, I need to boot the hosts for the initial cluster with no
  additional servers.
- As a user with limited Openshift experience I want to use an ISO that
  fully contains all that is needed to create a cluster in a disconnected
  environment.

### Goals

- Remove the need to set up and maintain a local registry on a separate server
  in fully-disconnected environments.
- Work with all topologies supported by the agent based installer - SNO,
  Compact, HA.

### Non-Goals

- Replace the current method of using a local registry on a separate server
  in a disconnected environment, as that method will still be supported.
- Replace the current method in a connected environment where no local
  registry is required, that will still be supported.
- Reduce install times (since network access is greatly reduced it is expected
  to reduce install times, but it is not a goal of this feature).
- Fully support upgrades and updates, as this host-based mirror is intended
  for Day 1 only.
- Include the container images in the ISO.

## Proposal

### Workflow Description

Currently, to perform a disconnected installation, a user must add the
ImageContentSources section to install-config.yaml with the upstream sources
and corresponding mirror locations. With this proposal, this step is no longer
required, instead the container images will be made available on a local volume
on the hosts.

The container images will be stored in a tarfile created using the
`oc-mirror` command which can publish images to a tarfile using an
ImageSetConfiguration with the command -
`oc-mirror --config <ImageSetConfig> file://<dir>`
In order to facilitate creating an ImageSetConfiguration, a new command will be
added - `openshift-install agent create image-set-configuration`,
that will output an ImageSetConfiguration based on the release image.
The user can add additional content and operators to this file and then
generate the tarfile using the `oc-mirror` command as above.
For reference, a tarfile containing all of the container images for a release
is on the order of 18GB.

Creating the local volume and mounting the tarfile will be specific to the
implementation in order to allow the most flexibility with the host-based
registry. The work that was done to create a separate
partition for the [telco-ran-tools](https://github.com/openshift-kni/telco-ran-tools/blob/main/docs/partitioning.md)
may be leveraged. For test, it will be possible to simply `scp` the tarfile to
the host.

Any tools needed during the host boot process (i.e. registry, `oc-mirror` etc.)
must be included in the ignition when the ISO is created. This will utilize the
work done for the
[agent-tui](https://github.com/openshift/installer/pull/6777)
to include binaries in compressed cpio archive appended to initrd. Note that
the size of the binaries is limited by the the amount of space allocated
for the tmpfs ephemeral storage when booting the ISO.

During host boot, the presence of the container image tarfile will indicate
that a host-based mirror is to be used. The tarfile will be unpacked into
container storage and `/etc/container/registries.conf` will be set so that
all image downloads on the host, i.e. via `podman pull`, are made using this
mirror. The bootstrap process will function in the usual manner with no
external network required to run containers. No changes to pull secret or CA
certificates will be needed to access this mirror. Optionally, setting
[additionalimagestores](https://github.com/containers/storage/blob/main/storage.conf#L42-L45)
may be sufficient, with or without changes to `registries.conf`.

After the release image has been written to disk and bootstrap is complete, the
container images must be made available upon reboot to complete the
installation. It is envisioned that a method similar to the telco-ran-tools
[ignition config override](https://github.com/openshift-kni/telco-ran-tools/blob/main/docs/ztp-precaching.md#43-nodesignitionconfigoverride)
will be used. A script can be run using ignition config override to make the
images available, create a registry in the cluster, and publish the images
to the registry.

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

To create the tarfile of container images from within the openshift-installer
the best choice will be `oc-mirror`. The binary can be extracted from
the release image, it's well supported, and well documented. The method
to make this tarfile available to the booting host via a local volume
will depend on the particular implementation.

When the host boots, a service will run to make the container images
available before other services attempt to pull these images. There
are two potential paths, both with tradeoffs:

1. The service creates a container to run a registry, preferably
`docker-registry` which is in the release image.  `oc-mirror` is used to
publish the contents of the tarfile to the registry.  The
`/etc/containers/registries.conf` is set up to use this local registry so
all accesses to the container images use it.  In this case, both the
registry and `oc-mirror` binaries would need to be included in the ISO as
they are not part of CoreOS.

2. The service does not create a registry, instead it unpacks the
tarfile and pushes the container images to container storage using
`skopeo`. All pulls of the container images come directly from this
container storage, aka [pre-pulled images](https://kubernetes.io/docs/concepts/containers/images/#pre-pulled-images).
In this case the registry and `oc-mirror` binaries would not need to be
included.

For scenario 1, in a multi-node cluster configuration, e.g. 3 control
plane nodes, the registry would only be created on Node0. All other nodes would
have their registries.conf set to retrieve container images from this registry.
This is also true when the other nodes boot into the final image. This method
uses standard tools (`oc-mirror`, registry) but does require their inclusion in
the ISO. Since the registry is local it will not be necessary to provide a
pull-secret to access it. By design, `oc-mirror` can publish to a registry
directly.

For scenario 2, since `oc-mirror` cannot publish to the container storage
directly, additional scripting would be needed to transfer from the tarfile
to container storage. This is similar to what is done with `telco-ran-tools`
so some of that can be leveraged. With this method, each host will be
responsible for updating its container storage. Depending on how the tarfile
is made available in a local volume, this option will also require that it is
accessible on all the nodes, instead of just Node0 as in scenario 1.

### Risks and Mitigations

TODO

### Drawbacks

As this proposal does not provide for upgrades, we are relying on the
upgrade mechanism to be handled separately as a Day 2 operation.

## Design Details

### Open Questions [optional]

- Can the necessary container images be filtered in order to
  reduce the size of the container image tarfile?

- Should we create a registry on Node0 for all nodes to access or
  use container storage directly on each node?

- How to handle the pivot and reboot from the bootstrap to final
  image? Use a script with ignition config override as is done with
  `telco-ran-tools`, use the registry in a static pod, or another way?

- How to handle upgrades to the running cluster? Some ideas in the
  `Operator Driven Image Load` section in
  [OCP Orchestrated Disconnected Updates](https://docs.google.com/document/d/1ka24qHyZYR1K5d6Qxcp9jiCZZwbpw0UQWX2Vcn1JyWw/edit?usp=sharing)
  using an `oc-mirror` operator, although this is beyond the scope
  of this document.

### Test Plan

Tooling will be added to dev-scripts to create the tarfile and use
it when booting the image. The registry creation on the dev-scripts
machine will not be done, instead the images will be fetched from
the host.

An additional CI test will be added that will use the dev-scripts
setting for host-based mirroring.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

- Initial POC that uses a podman registry on Node0 to perform bootstrap and
write the final image. This includes changes both to the installer and to
dev-scripts. The tools aren't currently included in the image but are
scp'ed to Node0.
https://github.com/openshift-metal3/dev-scripts/pull/1509
https://github.com/openshift/installer/pull/6806

## Alternatives

TBD

## Infrastructure Needed [optional]

