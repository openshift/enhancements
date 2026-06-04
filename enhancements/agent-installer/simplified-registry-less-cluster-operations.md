---
title: simplified-registry-less-cluster-operations
authors:
  - "@andfasano"
reviewers:
  - "@danielerez"
  - "@oourfali"
  - "@bfournie"
  - "@rwsu"
  - "@pawanpinjarkar"
  - "@yuqi-zhang"
  - "@mrunalp"
  - "@sdodson"
approvers:
  - "@zaneb"
api-approvers:
  - "@joelspeed"
creation-date: 2025-07-07
last-updated: 2026-05-25
tracking-link: 
  - "https://issues.redhat.com/browse/VIRTSTRAT-60"
  - "https://redhat.atlassian.net/browse/OCPSTRAT-3001"
see-also: []
replaces: []
superseded-by: []
---

# Simplified registry-less cluster operations for disconnected environments

## Summary

This enhancement proposal outlines an opinionated workflow to simplify
on-premises cluster operations - such as installation, upgrade and node
expansion - by eliminating the need to set up and configure an external
registry mirror. This approach is specifically designed for disconnected
environments and leverages existing tools such as Agent-based Installer,
Assisted Installer and OpenShift Appliance.

## Motivation

One of the most recurring pain points when installing - and later managing -
an OpenShift cluster in a disconnected scenario has been the requirement to
setup an [additional registry](https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html/disconnected_environments/index)
within the same environment. This task involves not only deploying and
maintaining the registry service itself, but also mirroring the desired
OpenShift release payload - and optionally some additional OLM operators.
Such complexity made the initial deployment and further upgrades challenging
and time-consuming, often resulting in a frustrating process for less
experienced users. Additionally, this approach was not considered suitable for
resource-constrained environments - where the lack of dedicated hardware for
hosting an external registry made it impractical.

### User Stories

* As an OpenShift administrator, I want to install a cluster in a
  disconnected environment without setting up an external registry (and
  optionally a selected set of OLM operators)
* As an OpenShift administrator, I want to upgrade a cluster in a
  disconnected environment without setting up an external registry (and
  optionally a selected set of OLM operators)
* As an OpenShift administrator, I want to add a node to an existing cluster in
  a disconnected environment without setting up an external registry
* As an OpenShift administrator, after having deployed a cluster, I want to
  start using my own external registries for future upgrades

### Goals

* Ensure successful installation/upgrade/adding new nodes in disconnected
  environments with minimal requirements and no pre-existing registry
* Offer a simplified installation experience for disconnected environments by
  leveraging existing tools like the Agent-Based Installer, Assisted Installer
  and OpenShift Appliance
* Support a mechanism to allow the user, after having successfully installed
  the cluster, to opt-out from the feature and start using their own external
  registry for the cluster upgrade operations

### Non-Goals

* Allow the user to install/upgrade any OLM operators. The selection will be
  restricted to a specific subset.

## Proposal

From a broad perspective, this proposal consists of building and releasing an
extended RHCOS live ISO that includes a full OCP release payload (with a
selected subset of OLM operators) and all the necessary scripts/services to
support the various operations. In general, a local registry service will be
running on every control plane node, serving the OCP release payload included
in the ISO. Also, a new custom resource - managed by the Machine Config
Operator (MCO) - will be introduced to control and manage the internal release
payload added to the system.

### High-level proposal scenarios

#### Extended RHCOS ISO build and release

The foundation of the current proposal lies in the user's ability to
download a customized self-sufficient RHCOS live ISO, containing all the
necessary elements to support the cluster operations in a disconnected
environment.
This ISO will be prepared and built using the [OpenShift Appliance Builder](https://github.com/openshift/appliance),
a command line utility for building a disk image to orchestrate an OpenShift
installation. The Appliance builder will be enhanced with a new 
`build live-iso` command capable of generating an ISO artifact.
Since the extended ISO will contain not only a specific set of OCP release images,
but also additional images related to OLM operators (and other support images),
the Appliance builder will generate a signed release bundle image listing
all the included images.
The extended ISO builder will be integrated within the official Red Hat build and
release pipeline, so that a new extended RHCOS ISO could be published following
the same OpenShift release cadence, making it available via the 
[Red Hat Customer Portal](https://access.redhat.com), specifically through the
[Red Hat Hybrid Cloud Console](https://console.redhat.com/).

#### Installation

##### Bootstrap phase

Based on the same [Agent-based Installer](https://github.com/openshift/enhancements/blob/master/enhancements/agent-installer/automated-workflow-for-agent-based-installer.md) (ABI)
approach, the installation will be kicked off in an ephemeral environment
fully allocated in memory, as soon as each node is booted using the extended
ISO (bootstrap phase).
In addition to the usual ABI services configured to orchestrate the
installation, a local registry service will be created on each node, and the
content of the release payload from the mounted ISO will be used as its
backend source.
During this early phase, the main responsibility of the local registry will
be to serve any image pull request originating from the local services running
on the same node.

##### Installation phase (after the first reboot)

During the installation of each control plane node, the contents of the OCP
release payload will be copied from the ISO to the node's disk (in the format
of the registry's 'filesystem' backend) after the OS disk writing step - so
that it remains available not only after the first reboot, but also once the
installation is complete. Also, the registry container image itself will be
copied in the node additional image store for podman.
[Assisted Service](https://github.com/openshift/assisted-service/)
and [Assisted Installer](https://github.com/openshift/assisted-installer)
will be enhanced accordingly to take care of this step, and to ensure it
will be properly monitored during the installation process.

A new systemd service will be injected via MCO to run the registry container
for providing the release content stored on disk after the reboot (since the
previous ephemeral environment will not be available anymore).

To allow pulling images from the cluster, an ImageDigestMirrorSet
(IDMS) policy will be added to the cluster manifests - using the `api-int` 
DNS entry - so that the set of registries running within
the control plane nodes will transparently serve the release payload images
locally mirrored (plus any other cluster resources generated by oc-mirror
when mirroring the content during the ISO build process).

Finally, a new `InternalReleaseImage` custom resource, managed by MCO, will
also be added as a cluster manifest to keep track and manage the currently
stored internal release bundle.

A new MCC controller, along with a new MCD manager, will be added to properly
manage the `InternalReleaseImage` resource (and the related `MachineConfigNodes`).

#### Upgrade

Upgrading an existing lower version cluster will be performed via the same
extended ISO used for managing the installation. As soon as the user
attaches the extended ISO to one of the control planes, a temporary registry
will be created to serve its content, and the IRI resource will report in
its status the newly discovered release bundle identifier.
At this point the user can edit the existing IRI spec to add the new
release bundle identifier to trigger the copy process (from the temporary
registry to each local control plane node registry), managed by the Machine
Config Daemon (MCD). Once the copy has been completed on all the control plane
nodes, the user can start the usual offline upgrade process via the `oc adm
upgrade --allow-explicit-upgrade --to-image=<release-image>`.
At the end of the upgrade the user may decide to delete the previous release
entry from the IRI resource. In such case the MCD on each control plane node
will take care of removing the older release payload from the local registry.

#### Adding a new node

A node could be added to a target existing cluster using the `oc adm
node-image` [command](https://github.com/openshift/enhancements/blob/master/enhancements/oc/day2-add-nodes.md)
(currently, limited to workers). Even though the IDMS resource guarantees that
the `oc adm node-image create` could successfully generate the node ISO, it
won't be sufficient to support the joining process. For that, it will be
necessary to run the registry on a host network port accessible from outside
the cluster. In addition to that, the registry will be secured via a
pull-secret. Users will be required to add this port to the api-int load
balancer (when it is external to the cluster) and open firewalls to allow
this port to be accessible from all the nodes, as well as the registry port.

### Workflow Description

The various workflows are briefly summarized below, from the user's point of
view:

#### Installation

1. The user downloads the extended RHCOS ISO from the Red Hat Hybrid Cloud
   Console.
2. The user moves the extended RHCOS ISO into the disconnected environment.
3. The user boots all the nodes that will be part of the cluster using the
   downloaded ISO.
   (note: even though not strictly relevant for the current proposal,
   we'll assume that the deployment configuration details are already
   available in the installation environment - either provided statically or
   interactively by the user - such as the rendezvous node selection and cluster
   specifications).
4. The installation process starts automatically and runs unattended until
   completed.

#### Upgrade

1. The user downloads the extended RHCOS ISO from the Red Hat Customer Portal,
   by selecting a compatible newer version release for the target cluster.
2. The user moves the extended RHCOS ISO into the disconnected environment.
3. The user attaches the extended RHCOS ISO to one of the control plane nodes
   of the target cluster.
4. The IRI resource will report the newly detected release bundle
   identifier with a `Mounted` condition in its `status.releases` field.
5. The user edits the IRI resource by adding the release bundle identifier to
   the `spec.releases` field.
6. The user waits until the copy process of the new release image is completed
   for all the control plane nodes. The progress and status of the task can
   be monitored via the IRI `status` field.
7. Once completed, the IRI status will report also the specific release
   pullspec, so that the user can upgrade the cluster via the `oc adm upgrade
   --allow-explicit-upgrade --to-image=<release-pullspec>`

#### Post-upgrade optional steps

1. The user edits the IRI resource by removing the release bundle identifier
   not in use anymore.
2. All the images related to the specified release bundle will be removed
   from all the control plane nodes. The progress and status of the task
   can be monitored via the IRI `status` field.

#### Node expansion

1. The user runs the `oc adm node-image create` command from within the
   disconnected environment (with optionally some additional node
   configuration).
2. The user attaches the newly generated node ISO to the target machine(s),
   and boots it. The progress and status of the task can be monitored via the
   `oc adm node-image monitor` command.
3. The user waits until the CSRs are generated, and manually approves them
   via the `oc adm certificate approve` command.
   
### API Extensions

#### InternalReleaseImage custom resource

The `InternalReleaseImage` is a new singleton custom resource, named `cluster`,
controlled by MCO, used to keep track and manage the OCP release images stored
on the control plane nodes. In particular, it will be used for:

* Triggering a new release image copy task - as a preliminary upgrade step
* Delete from disk a release image version no longer in use - as an optional
  post-upgrade step
* Opt-out from the feature (by deleting the resource)

This is an example of what the CR will look like:

```
apiVersion: machineconfiguration.openshift.io/v1alpha1
kind: InternalReleaseImage
metadata:
  name: cluster      
spec: 
  releases:
    - name: ocp-release-bundle-4.18.0-x86_64
    - name: ocp-release-bundle-4.19.0-x86_64
status:
  conditions:
    - type: Degraded
      status: "False"
      reason: "AllReleasesAvailable"
      message: "All the release images are available"
      lastTransitionTime: "2024-11-01T07:00:00Z"
  releases:
    - name: ocp-release-bundle-4.18.0-x86_64
      image: example.com/example/openshift-release-dev@sha256:aa8795f7932441b30bb8bcfbbf05912875383fad1f2b3be08a22ec148d6860ff
      conditions:
      - type: Available
        status: "True"
        reason: "Available"
        message: "Release ocp-release-bundle-4.18.0-x86_64 is currently available"
        lastTransitionTime: "2024-11-01T07:00:00Z"
    - name: ocp-release-bundle-4.19.0-x86_64
      image: example.com/example/openshift-release-dev@sha256:d98795f7932441b30bb8bcfbbf05912875383fad1f2b3be08a22ec148d68607f
      conditions:
      - type: Available
        status: "True" 
        reason: "Available"
        message: "Release ocp-release-bundle-4.19.0-x86_64 is currently available"
        lastTransitionTime: "2024-12-01T09:00:00Z"
```

* The `spec.releases` contains a list of release objects, and it will be used
  to configure the desired release contents. Adding (or removing) an entry from
  the list will trigger an update on all the control plane nodes
* The `spec.releases.name` format is a string specifying the release bundle
  identifier. It must match the value discovered and reported in the 
  `status.releases` field
* The `status.conditions` field will be used to report the current global state
* The `status.releases` field will report the list of each currently managed release
* The `status.releases.name` is the identifier of the release bundle
* The `status.releases.image` is the OCP release image pullspec by digest for
  the current release bundle
* The `status.releases.conditions[]` field will be used to keep track of the activity
  performed on the various nodes

For the global IRI conditions, the following type will be defined:

* `Degraded`. When set, it may indicate a failure in the controller, or a problem in one (or more) release bundle.

For each release bundle, the following bundle condition types will be defined:

* `Mounted`. When true, it means that a valid ISO has been discovered and mounted on one of the cluster nodes.
* `Installing`.  When true, it means that a new release bundle is currently being copied on one (or more) cluster nodes, and not yet completed.
* `Available`. When true, it means that the release has been previously installed on all the cluster nodes, and it can be used.
* `Removing`.  When true, it means that a release deletion is in progress on one (or more) cluster nodes, and not yet completed.
* `Degraded`. When true, it means something has gone wrong (possibly on one or more cluster nodes).

##### MachineConfigNode InternalReleaseImage status

The `MachineConfigNode` CRD will be extended to report the per-node status, so
that it will be possible to track the current status/errors of each individual
node. The IRI resource will monitor all the MCN resources to update its status.

Below is a proposal example for the extended status field:

```
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigNode
...
status:
  internalReleaseImage:
    releases:
      - name: ocp-release-bundle-4.18.0-x86_64
        image: example.com/example/openshift-release-dev@sha256:aa8795f7932441b30bb8bcfbbf05912875383fad1f2b3be08a22ec148d6860ff
        conditions:
          - type: Mounted
            status: "False" 
            reason: "Mounted"
            message: ""  
            lastTransitionTime: "2024-12-01T08:04:21Z"
          - type: Available
            status: "True"               
            reason: "Available"
            message: "Release ocp-release-bundle-4.18.0-x86_64 is currently available on node master-0"
            lastTransitionTime: "2024-12-01T08:04:21Z"
          - type: Degraded
            status: "False"               
            reason: "Degraded"
            message: ""
            lastTransitionTime: "2024-12-01T08:04:21Z"
```

* The `status.internalReleaseImage.releases` field will be used to track the status of the release
  payloads stored in the related node
* The `status.internalReleaseImage.releases.name` will report the release bundle identifier string
* The `status.internalReleaseImage.releases.image` will report the OCP release image pullspec by digest
* The `status.internalReleaseImage.releases.conditions` field will be used to keep track of the activity
  performed for the specific release payload, and in particular to report any error

For each release bundle, the following bundle condition types will be defined:

* `Mounted`. When true, it means that a valid ISO has been mounted on the current node.
* `Installing`.  When true, it means that a new release bundle is currently being copied on the current node, and not yet completed.
* `Available`. When true, it means that the release has been previously installed on the current node, and it can be used.
* `Removing`.  When true, it means that a release deletion is in progress on the current node, and not yet completed.
* `Degraded`. When true, it means something has gone wrong in the current node.

Additionally, a new `StateProgress` enum value will be added to the MachineConfigNode conditions type:

* `InternalReleaseImageDegraded`. When true, it means the current node is not properly working (for one or more release bundles)


### Topology Considerations

Will function identically in all topologies.

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

Will support SNO deployments.

#### OpenShift Kubernetes Engine

This proposal does not depend on features excluded from OKE. All the
components involved - Agent-based Installer, Machine Config Operator,
ImageDigestMirrorSet and CRI-O - are part of the core platform available
in both OCP and OKE. The optional OLM operator mirroring included in the
extended ISO is not required for the core registry-less workflow.

### Implementation Details/Notes/Constraints

#### InternalReleaseImage custom resource

A new `InternalReleaseImageController` sub-controller of the
_machine-config-controller_ will be created to watch and manage the
`InternalReleaseImage` resource. The controller will take care of
updating the resource status.
In general, the IRI controller will watch all the available
MachineConfigNodes status to verify if a given action (adding/removing a
release bundle) has been fully completed, and to detect the presence
of a new mounted ISO.

A new `InternalReleaseImageManager` manager will be added to
_machine-config-daemon_ to handle per-node specific operations.
Every manager will report its own status to the related MachineConfigNode
resource.
Every manager will be responsible for watching the IRI resource, to detect
when adding/removing a release bundle is required.
Also, if a release bundle ISO is mounted on the related node, it will have
to report the ISO label string into its status.

##### Removal of a release from the InternalReleaseImage resource

The user can remove a release payload no longer in use by editing the
`spec.releases` field of the `InternalReleaseImage` resource to delete the
desired release entry value.

A ValidatingAdmissionPolicy will be added to prevent the user from removing
a release entry that is currently being used by the cluster. The policy will
perform a check against the `ClusterVersion` `version` object to allow (or not)
the update request.

##### Removal of InternalReleaseImage resource

The deletion of the IRI `cluster` resource will be handled by a finalizer,
and it will stop all the registry services running on the control plane nodes.
In addition, all the currently stored release payloads will be deleted from the
nodes disk, and all the previously created resources will be removed as well
(such as the IDMS ones) to complete the cleanup.

It won't be allowed to delete the resource if the current release is still
managed by the IRI resource. Also in this case, a ValidatingAdmissionPolicy
performing a check against the `ClusterVersion` `version` object will be used
(to verify that none of the IRI releases is currently being used by the
cluster).

This means that a user, to opt-out from the feature, will first have to
set up their own registry with the required mirrored content and then perform
a cluster upgrade. Once successfully completed, it will be possible to remove the
IRI resource.

Re-activation scenarios will not be supported.

#### Extended RHCOS ISO build

The release image and operators images will be stored inside the extended RHCOS
ISO in the storage format used by the [distribution/distribution](https://github.com/distribution/distribution)
registry’s filesystem storage plugin.
The Appliance builder will generate a new `release-bundle` (signed) image
containing a `bundle.json` listing all the included images digests (for the
release payload, OLM operators and any other additional support image), and
any other relevant oc-mirror artifacts generated during the build.
The new release bundle image will be published on the local IRI registry in the `/openshift/release-bundles/` repository.
The ISO must have a label in the format `ocp-release-bundle-<VER>-<ARCH>`.

#### Distribution/distribution registry OCP integration

The [distribution/distribution](https://github.com/distribution/distribution)
registry will be used to run the registry services on each control plane
node. The binary will be built and included in the same source as
[openshift/image-registry](https://github.com/openshift/image-registry) repo,
so that it will be available within an OCP release image payload and CVEs will
be handled without any additional overhead.

##### Registry security and maintenance

Every IRI registry running on each control plane node will be secured by a set of credentials
published in the `openshift-machine-config-operator/internal-release-image-registry-auth` secret,
and they will be included in the OpenShift global pull secret. The registries will also use a dedicated TLS certificate.
A mechanism to allow the user to rotate both the creds and TLS cert will be supported.

#### Installation - bootstrap phase

![alt bootstrap phase](simplified-operations-bootstrap.png)

Initially the registry image will be stored in the `/var/lib/iri-registry` folder.
An initial service (added by the Appliance builder in the ISO ignition) will
startup the registry at the very beginning of the boot phase. 
The `/etc/containers/registries.conf` will be modified to ensure that the
images will be pulled from the local registry, using `api-int` as the
registry host.

#### Installation - install phase

![alt install phase](simplified-operations-install.png)

A systemd service injected by MCO will ensure the registry will be available
after the reboot. The registry will have to run on a host network port on each
node to ensure it's accessible from the outside, specifically port 22625 (to be
[registered](https://github.com/openshift/enhancements/blob/master/dev-guide/host-port-registry.md)). 
An Assisted Service pre-flight validation will ensure that the port is open.

#### Upgrade

A udev rule, along with a systemd service injected by MCO will detect when an
extended RHCOS ISO is attached, and then mount it to launch a temporary registry
to serve its content, by looking for devices labelled as `ocp-release-bundle-*`.
This label will be reported as the bundle identifier in the MCN status (and then
in the IRI status). After the user modifies the IRI resource with the new release
bundle identifier, the new OCP release image payload will be copied on each node
registry using skopeo.

#### Storage requirements

The control plane nodes disks must have sufficient space to store the requested
release payloads. The recommendation is to allocate at least ~60Gb per release.
Thus, to fully support also the upgrade, a total of additional ~120GB will be
required.
Assisted Service will apply a pre-flight validation to ensure that the
control plane nodes will meet the new storage requirement. Additionally, it
should be properly documented in the prerequisites section.

### Risks and Mitigations

* Exposing the internal registry on a node port will allow it to be reached
  from outside the cluster. This behavior is similar to the one already
  adopted by the Machine Config Server (MCS).
  This is a new requirement that will need to be properly documented. In
  addition, to minimize the risks, the registry will have to be secured
  via a pull-secret.

### Drawbacks

* The extended RHCOS ISO size is pretty big, currently ~50GB with just a
  limited set of OLM operators. Including all the OLM operators is not
  feasible, and the current selected set may not be sufficient and/or
  effectively required by all the users.
* Due to the size, some hardware vendors may have difficulty loading
  such ISO, so a more extended testing may be required.
* The lifecycle of some OLM operators is different from the OCP one,
  thus assembling an extended ISO may be challenging due to the different release
  timings (between OCP and OLM operators).
* The upgrade path of some OLM operators may require including different
  versions of the same operator, thus impacting the final size of the
  extended ISO.
* Building and releasing the extended ISO via Konflux requires a non-trivial
  additional effort cost.

## Alternatives (Not Implemented)

* For building the ISO, we've explored also the usage of the [custom-coreos-disk-images](https://github.com/coreos/custom-coreos-disk-images)
  utility, to reuse the OCP layering feature. The approach was discarded as it
  appeared not feasible especially when dealing with such additional payload
  size (a standalone OCP release size is usually ~20GB)

* Instead of using a local registry on each node, we've also explored the
  possibility to pre-load all the required images into an additional read-only
  local container storage on each node. Unfortunately this approach wasn't
  feasible, as several pieces of the OpenShift ecosystem assume that a registry
  was always available in the environment. Removing such dependency across
  all the possible locations appeared to be particularly complex and
  challenging (and moreover no formal way to enforce it once removed)

## Test Plan

[dev-scripts](https://github.com/openshift-metal3/dev-scripts) will be enhanced
to support testing via the extended RHCOS ISO, and new jobs will be added in the
OpenShift CI to cover the sno/compact/ha topologies.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation
- Sufficient test coverage (using a disconnected environment)
- Gather feedback from users rather than just developers
- Integration with the official Red Hat build and release pipeline

### Tech Preview -> GA

- Complete e2e testing on installation, upgrade and node expansion
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A