---
title: disconnected-support
authors:
  - "@saifshaikh48"
reviewers:
  - "@openshift/openshift-team-windows-containers"
  - "@openshift/openshift-team-pod-autoscaling, for insight on MCO's registry configuration on Linux nodes"
approvers:
  - "@mrunalp" # Staff eng approval required - previously @aravindhp
api-approvers:
  - None
creation-date: 2023-12-11
last-updated: 2024-03-12
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-619"
  - "https://issues.redhat.com/browse/WINC-936"
see-also:
  - "https://github.com/openshift/enhancements/api-review/add-new-CRD-ImageDigestMirrorSet-and-ImageTagMirrorSet-to-config.openshift.io.md"
  - https://cloud.redhat.com/blog/is-your-operator-air-gap-friendly
---

# Windows Containers Support in Disconnected Environments

## Release Sign-off Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The goal of this enhancement is to support Windows Containers in environments with restricted networks where
hosts are intentionally impeded from reaching the internet, also known as disconnected or "air-gapped" clusters.
Currently, Windows nodes configured by the [Windows Machine Config Operator](https://github.com/openshift/windows-machine-config-operator) 
(WMCO) rely on `containerd`, the OpenShift-managed container runtime, to pull workload images. Also, the WMCO today
is required to make an external request outside the cluster's internal network to pull the [pause image](https://kubernetes.io/docs/concepts/windows/intro/#pause-container).
To support disconnected environments, all images need to be pulled from air-gapped mirror registries, whether that be 
the OpenShift internal image registry or other private registries.

There already exists a protocol for users to publish [registry mirroring configuration](https://docs.openshift.com/container-platform/4.14/openshift_images/image-configuration.html#images-configuration-registry-mirror_image-configuration),
namely `ImageDigestMirrorSet` (IDMS), `ImageTagMirrorSet` (ITMS) cluster resources. 
These are consumed by OpenShift components like the Machine Config Operator (MCO) to apply the settings to Linux
control-plane and worker nodes, Windows worker nodes do not currently consume or respect mirror registry settings when
pulling images. This effort will work to plug feature disparity by making the WMCO aware of mirror registry settings at
operator install time and reactive during its runtime.

## Motivation

The motivation here is to expand the Windows containers production use case, enabling users to add Windows nodes and run
workloads easily and successfully in disconnected clusters. This is an extremely important ask for customer environments
where Windows nodes must pull images from air-gapped registries for security reasons.
The added benefit here is that the changes proposed in this enhancement are not restricted to disconnected environments;
users in __connected__ clusters can take advantage of this mirror registry support. This would allow better performance
as images can be stored in registries closer to the production environment, reducing pull times for large 
Windows container images, in addition the security benefits of restricted networks.

### Goals

* Create a mechanism for WMCO to consume global mirror registry settings from existing platform resources (IDMS/ITMSes),
  such that it enables Windows nodes to pull images in disconnected environments
* Configure registry settings for the containerd runtime, a WMCO-managed component on Windows nodes 
* React to changes to the mirror registry settings during WMCO runtime
* Maintain normal functionality in connected clusters and clusters without any cluster-wide mirror registry configuration

### Non-Goals

* First-class support of [all OpenShift image registry setting configurations](https://docs.openshift.com/container-platform/4.14/openshift_images/image-configuration.html#images-configuration-file_image-configuration), including the `Image` resource
* Supporting clusters that use only `ImageContentSourcePolicy` resources. These were [deprecated in 4.13](https://docs.openshift.com/container-platform/4.14/release_notes/ocp-4-14-release-notes.html#ocp-4-14-deprecated-removed-features) 
and users should instead [migrate to IDMSes](https://docs.openshift.com/container-platform/4.14/openshift_images/image-configuration.html#images-configuration-registry-mirror-convert_image-configuration).
* Using [`ImageContentPolicy` resources](https://docs.openshift.com/container-platform/4.14/rest_api/config_apis/imagecontentpolicy-config-openshift-io-v1.html).
  This is not required for the MVP of supporting disconnected environments.
  For additonal context, rules defined in ImageContentPolicys are used to allow/deny specific registries or image
  references, something that is out-of-scope of this enhancement, which focuses on image mirroring.

## Proposal


### User Stories

<!--Story 1: Set `config_path` in containerd_conf.toml-->
As an OpenShift Windows admin, I want containerd to pick up CRI plugin config so that I can customize runtime settings.

<!--Story 2: WMCO generates hosts.toml files-->
As an OpenShift Windows admin, I would like the WMCO to create config files compatible with containerd, so that my
cluster's mirror settings are applied to Windows nodes.

<!--Story 3: WMCO copies over generated hosts.toml files to each Windows instance-->
As an OpenShift Windows admin, I want WMCO to copy over Windows-compatible container runtime settings onto my Windows
nodes, so my cluster would function when disconnected from primary registries.

User stories can also be found within the JIRA epic: [WINC-936](https://issues.redhat.com/browse/WINC-936)


### Implementation Details/Notes/Constraints

There are a couple pieces of work as part of this enhancement. All changes detailed in this enhancement proposal will be
limited to the Windows Machine Config Operator and its sub-component, Windows Instance Config Daemon (WICD).

First, WMCO will set the `config_path` variable in the containerd config file.
Next, the operator will generate registry config files based on cluster resources. 
And lastly, WMCO will place the generated files in the containerd config directory on each Windows instance.

WMCO will use the mirrors data in `ImageDigestMirrorSet` and `ImageTagMirrorSet` resources 
to create a set of [hosts.toml files](https://github.com/containerd/containerd/blob/main/docs/hosts.md#hoststoml-content-description---detail),
one for each registry mirror, that will be transferred to the Windows instance. WMCO will handle the file content 
creation and will copy over the files to each Windows node.

Since WMCO is an day 2 operator (optional, not part of the OCP payload), it will pick up the mirror configuration during
runtime regardless of whether they were set during cluster install time or at some point during the cluster's lifetime.

#### Containerd Config File Changes

WMCO will need to set the `config_path` variable in the containerd config file to tell it where to look for registry
config settings; it is currently empty in value. We should likely set it to `C:/k/containerd/registries` -- this is 
within our managed directories and ensures the directory contents are cleaned up during node deconfiguration.

#### Managing Containerd Registry Config Files

WMCO will generate the required registry config files based on the IDMS/ITMS resources in the cluster.
For new nodes, these files will be copied over to the instances on configuration, similar to how other config files
(kublet conf, bootstrap kubeconfig, files from ignition, etc) are treated today.

For existing nodes, the operator will react to updates to the cluster's mirror settings through a new controller
which will watch for events on IDMS/ITMS resources. This "registry controller" will generate and copy over the new
registry config files to each Windows instance when an event is detected.
We will fully replace the contents of the registry config directory on any change to the cluster's IDMS/ITMS resources; 
this way, out-of-date registry config files will be removed without an extra mechanism to track files WMCO previously created.

We can utilize [registry runtime-utils](https://github.com/openshift/runtime-utils/blob/master/pkg/registries/registries.go)
to easily process and merge the contents of the IDMS/ITMS resources, [as MCO does](https://github.com/openshift/machine-config-operator/blob/master/pkg/controller/container-runtime-config/helpers.go#L407). This will help us to
generate config file content from a unified object representing the cluster's mirror registry configuration state.

The directory to save the generated registry config files is defined by WMCO -- it should be the same as what it set for
the containerd_conf.toml's `plugins."io.containerd.grpc.v1.cri".registry.config_path`. 

The containerd daemon [picks up the registry configuration without requiring restart](https://github.com/containerd/containerd/blob/main/docs/hosts.md#registry-configuration---introduction) so no further action required.

#### Working example

Example: The base containerd_conf.toml has `config_path` set to `C:/k/containerd/registries` and the cluster contains 
the following resources
```yaml
apiVersion: config.openshift.io/v1 
kind: ImageDigestMirrorSet 
metadata:
  name: ubi9repo
spec:
  imageDigestMirrors: 
  - mirrors:
    - example.io/example/ubi-minimal
    - example.com/example2/ubi-minimal
    source: registry.access.redhat.com/ubi9/ubi-minimal 
    mirrorSourcePolicy: AllowContactingSource 
  - mirrors:
    - mirror.example.com
    source: registry.redhat.io
    mirrorSourcePolicy: NeverContactSource
```

```yaml
apiVersion: config.openshift.io/v1
kind: ImageTagMirrorSet
metadata:
  name: tag-mirror
spec:
  imageTagMirrors:
  - mirrors:
    - docker.io
    source: docker-mirror.internal
    mirrorSourcePolicy: AllowContactingSource
```

This would result in WMCO generating content for 3 files and transferring them onto each Windows instance.
The file structure would be as follows:
```powershell
$ tree $config_path
C:/k/containerd/registries/
|── registry.access.redhat.com
|   └── hosts.toml
|── mirror.example.com
|   └── hosts.toml
└── docker.io
    └── hosts.toml
```

And the content for each generated file would look like this: 
```powershell
$ cat "$config_path"/registry.access.redhat.com/host.toml
server = "https://registry.access.redhat.com" # default fallback server since "AllowContactingSource" mirrorSourcePolicy is set

[host."https://example.io/example/ubi-minimal"]
 capabilities = ["pull"]

[host."https://example.com/example2/ubi-minimal"] # secondary mirror
 capabilities = ["pull"]


$ cat "$config_path"/registry.redhat.io/host.toml
# "server" omitted since "NeverContactSource" mirrorSourcePolicy is set

[host."https://mirror.example.com"]
 capabilities = ["pull"]


$ cat "$config_path"/docker.io/host.toml
server = "https://docker.io"

[host."https://docker-mirror.internal"]
 capabilities = ["pull", "resolve"] # resolve tags
```

---

## Test Plan

In addition to unit testing individual WMCO/WICD packages and controllers, an e2e job will be added to the release repo
for WMCO's master/release-4.16 branches. A new CI workflow will be created leveraging 
[existing step-registry steps](https://github.com/openshift/release/tree/master/ci-operator/step-registry/aws/provision/vpc/disconnected),
which creates a disconnected cluster in AWS with hybrid-overlay networking. 
I propose AWS here instead of vSphere since we have 3+ existing jobs that run on vSphere and I don't want to overload the infra.
We'll also need to pre-load the images used in our e2e suite into the cluster's internal registry as part of job setup.
This workflow will be used to run the existing WMCO e2e test suite to ensure that WMCO runs as expected in disconnected 
environments with no regression. We will add a few test cases to explicitly check the state of registry settings on Windows
nodes.
QE should cover disconnected environments on all supported platforms when validating this feature.

### Workflow Description and Variations

**cluster creator** is a human user responsible for deploying a cluster.
**cluster administrator** is a human user responsible for managing cluster settings including network egress policies.

There are 4 different workflows that affect the disconnected use case.
1. A cluster creator brings up a new disconnected cluster and configures mirror registry settings at install time
2. A cluster administrator introduces new mirror registry settings or changes exisiting ones during runtime
   This would occur through the creation, deletion, or update IDMS/ITMS resources.
3. A cluster administrator transitions a connected cluster *with existing mirror registry settings* to a disconnected cluster
4. A cluster administrator transitions a connected cluster *without existing mirror registry settings* to a disconnected cluster


In cases 1, 2 & 3, Windows nodes can be joined to the cluster after altering registry settings,
which would result in WMCO applying mirror settings during initial node configuration. 

In case 2, for any existing Windows nodes, WMCO will react by updating each instance's state to consume the new registry config.

In case 3, existing Windows nodes will not be re-configured. Existing registry settings will stay applied.

In case 4, WMCO will take no action as there are no settings to configure. Windows nodes will not be able to pull images
until mirror registry settings are configured by the administrator.

### Risks and Mitigations

If node cleanup fails, instances may retain their containerd configuration. This could be a concern for BYOH instances
particularly. An event should be generated to alert cluster admins manual cleanup is required to remove WMCO-created files.

Also, WMCO has no way of detecting local changes to the containerd registry config directory on Windows nodes.
The changes will remain in effect until a new IDMS/ITMS change is made in the cluster or the node is reconfigured.

Another concern is if a user's mirror configuration has overlapping settings/clashes. An example of this would be
```yaml
kind: ImageDigestMirrorSet 
spec:
  imageDigestMirrors: 
  - mirrors:
    - registry.access.internal.com
    source: registry.redhat.io
  - mirrors:
    - mirror.example.com
    source: registry.redhat.io
```
In this case, the user has configured the same image source to be mirrored to two different mirror registries, which
results in ambigious pull instructions for the container rumtime. As on the Linux side, it is upon the user to ensure
there is no clash between mirrors across their IDMS and ITMS resources.

<!--
Containerd may not support all the fields available in IDMS/ITMS resources.
Unsupported fields will be ignored and will not impact containerd registry config/default behavior.
-->

### Drawbacks

The major drawback with this design is a scaling issue. WMCO will have to SSH into each node to update mirror registry
settings (i.e. copy new files onto the instance). This is will happen sequentially as each node will be reconciled
one at a time, something that will not scale well as the number of nodes increases, leading to long configuration times.
Re-configuration time will scale linearly with the number of Windows nodes in the cluster.

The only other drawback is the increased complexity of WMCO and the potential complexity of debugging customer cases
that involve a custom mirror registry setup, since it would be harder to set up an accurate replication environment.
This can be mitigated by proactively getting the development team, QE, and support folks familiar with the expected
behavior of Windows nodes/workloads in disconnected clusters.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A, not supported by WinC.

#### Standalone Clusters

N/A, not supported by WinC.

#### Single-node Deployments or MicroShift

N/A, not supported by WinC.

### API Extensions

N/A, as no CRDs, admission and conversion webhooks, aggregated API servers, or finalizers will be added or modified.
Only the WMCO will be extended which is an optional operator with its own lifecycle and SLO/SLAs, a
[tier 3 OpenShift API](https://docs.openshift.com/container-platform/4.12/rest_api/understanding-api-support-tiers.html#api-tiers_understanding-api-tiers).

## Operational Aspects of API Extensions

N/A

### Failure Modes

N/A

### Release Plan

The feature associated with this enhancement is targeted to land in the official Red Hat operator version of WMCO 10.16.0
within OpenShift 4.16 timeframe. The normal WMCO release process will be followed as the functionality described in this
enhancement is integrated into the product.

An Openshift docs update announcing Windows disconnected support will be required as part of GA.
The new docs should list the specific steps taken to configure Windows nodes to pull from mirror registries,
but linking to existing docs should be enough for overarching info such as mirroring images to disconnected environments,
creating image set configurations (IDMS/ITMS resouces), and authenticating with private registries
(e.g. [using image pull secrets](https://docs.openshift.com/container-platform/4.14/openshift_images/managing_images/using-image-pull-secrets.html)).
The docs should also call out the scaling limitations of the MVP approach -- WMCO will have to sequentially update each
Windows node when cluster mirror settings change, so reconfiguration times will scale linearly with the # of Windows
nodes in the cluster.
We should also have a post published on the OpenShift blog announcing this feature and showcasing how to use it.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

See Release Plan above.

### Removing a deprecated feature

N/A, as this is a new feature that does not supersede an existing one.

## Support Procedures

In general the support procedures for WMCO will remain the same.
Customers that need support with disconnected environments must upgrade to the minimum versions: OCP 4.16 and WMCO 10.16.0.

## Upgrade / Downgrade Strategy

The relevant upgrade path is from WMCO 10.15.z in OCP 4.15 to WMCO 10.16.0 in OCP 4.16. There will be no changes to the
current WMCO upgrade strategy. Once customers are on WMCO 10.16.0, they can [convert their connected cluster to a
disconnected one](https://docs.openshift.com/container-platform/4.14/post_installation_configuration/connected-to-disconnected.html#connected-to-disconnected-config-registry_connected-to-disconnected)
and the Windows nodes will be automatically updated by the operator to use the mirror settings to pull required images.

When deconfiguring Windows nodes, mirror settings will be cleared from the Windows instances. This involves undoing some node
config steps, which will be taken care of as part of removing the `containerd` Windows service.
This scenario will occur when upgrading both BYOH and Machine-backed Windows instances.

Downgrades are generally [not supported by OLM](https://github.com/operator-framework/operator-lifecycle-manager/issues/1177),
which manages WMCO. In case of breaking changes, please see the
[WMCO Upgrades](https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator-upgrades.md#risks-and-mitigations)
enhancement document for guidance.

## Version Skew Strategy

N/A. There will be no version skew since this work is all within 1 product sub-component (WMCO). 
The official Red Hat operator in OCP 4.16, WMCO version 10.16.0, will support Windows containers in disconnected
environments. Releases 10.15.z and below will not have support disconnected support.

## Implementation History

The implementation history can be tracked by following the associated work items in Jira and source code improvements in
the WMCO Github repo.

## Alternatives & Justification
 
For disconnected support, we could make the kubelet pause image location configurable. This is a nice to have, but may 
be out of scope of disconnected support as it could require a larger undertaking to make the containerd config file
configurable.

There are a few alternatives that are no longer viable.
Pre-pulling images, where user creates a Windows image with the pause image and other required images baked in, is not
possible anymore as we install and manage the container runtime.
Also, users cannot configure containerd's registries manually since WMCO will overwrite changes to the containerd
config file if there is some unrelated event that triggers node reconciliation.

## Open Questions

None currently.