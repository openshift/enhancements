---
title: lazy-image-pull
authors:
  - "@harche"
  - "@ktock"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@mrunalp"
  - "@rphillips"
  - "@sairameshv"
  - "@giuseppe"
  - "@haircommander"
  - "@mtrmac"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@mrunalp"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@mrunalp"
creation-date: 2024-03-18
last-updated: 2024-03-18
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-2204
---

# Enable Lazy Image Pulling Support in OpenShift


## Summary

This enhancement proposes integrating support for lazy image pulling in OpenShift using [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter). This will provide the ability to lazy-pull compatible container images, significantly reducing container startup times for the specific workloads that can benefit from it.

## Motivation

1. Serverless end points need to serve the cold start requests as fast as possible. Lazy-pulling of the images allow the serverlesss end point to start serving the user request without getting blocked on the entire container image to get downloaded on the host.
2. Large images often lead to extended pod startup times. lazy pulling images drastically reduces the initial delay, providing a smoother user experience.
3. If the container image is shared between various containers which utilize only part of the image during their execution, lazy image pulling speeds up such container startup by downloading only the relavent bits from those images.

### User Stories


* As a cluster administrator, I want to serve serverless endpoints on my cluster as fast as possible by not waiting for the entire container image to download.
* As a developer, I want to deploy applications packaged in large container images while minimizing the impact of image size on pod startup time.

### Goals

* Package upstream [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter) for the installation on RHCOS
* Enable lazy image pulling in Openshift using [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter) with [zstd:chunked](https://github.com/containers/storage/pull/775) image format.
* Add documenatation to easily create `zstd:chunked` images from scratch as well as converting existing images to `zstd:chunked` format.
* Add support for drop-in config in [container storage config](https://github.com/containers/storage/blob/main/pkg/config/config.go)

### Non-Goals

* Enabling lazy image pulling on the control plane nodes.
* Support for lazy image pull format other than `zstd:chunked`

## Proposal

In order to lazy pull images, we need to have [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter/blob/aaa46a75dd97e401025f82630c9d3d4e41c9f670/docs/INSTALL.md#install-stargz-store-for-cri-opodman-with-systemd) and [its authentication helper](https://github.com/containers/image/pull/2417) installed on the node as well as `additionallayerstores` configuration for container storage library. [ContainerRuntimeConfig](https://github.com/openshift/machine-config-operator/blob/release-4.16/docs/ContainerRuntimeConfigDesign.md) will be used to deploy the required configuration on the target nodes.


### Workflow Description

* A cluster administrator will create or edit the [ContainerRuntimeConfig](https://github.com/openshift/machine-config-operator/blob/release-4.16/docs/ContainerRuntimeConfigDesign.md) object to enable lazy image pulling support by,
```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
 name: lazy-image-pull
spec:
 machineConfigPoolSelector:
   matchLabels:
     pools.operator.machineconfiguration.openshift.io/worker: ""
 containerRuntimeConfig:
     lazyImagePull:
        mode: "Enabled"
```
### API Extensions

Extend existing [ContainerRuntimeConfiguration](https://github.com/openshift/api/blob/ff84c2c732279b16baccf08c7dfc9ff8719c4807/machineconfiguration/v1/types.go#L749C6-L749C35) to hold the required configuration for lazy image pulling.

```golang
// ContainerRuntimeConfiguration defines the tuneables of the container storage
type ContainerRuntimeConfiguration struct {
   // +kubebuilder:validation:Optional
   LazyImagePullConfig *LazyImagePullConfiguration `json:"lazyImagePullConfig,omitempty"`
}
```

```golang
type LazyImagePullConfiguration struct {
   // +kubebuilder:validation:Required
   Mode LazyImagePullMode `json:"mode"`
}

type LazyImagePullMode string

const (
   // LazyImagePullEnabled enables lazy image pulling.
   LazyImagePullEnabled LazyImagePullMode = "Enabled"

   // LazyImagePullDisabled disables lazy image pulling.
   LazyImagePullDisabled LazyImagePullMode = "Disabled"
)
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

MCO config above is needed to enable lazy pulling on HyperShift node pools.

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

MicroShift doesn't have MCO. Thus, to enable this on MicroShift node, users need to manually install Stargz Store and configure c/storage as described in the following sections.

### Implementation Details/Notes/Constraints

## Package Stargz Store plugin

Package the Stargz Store plugin located at, https://github.com/containerd/stargz-snapshotter so that it can get installed on RHCOS


## Starz Store systemd service

Startz Store service needs to be running on the host _before_ crio service starts. MCO needs to drop relavent systemd unit file on the targeted nodes.

```toml=
[Unit]
Description=Stargz Store plugin
Before=crio.service

[Service]
Type=notify
ExecStart=/usr/bin/stargz-store --log-level=debug --config=/etc/stargz-store/config.toml /var/lib/stargz-store/store
ExecStopPost=umount /var/lib/stargz-store/store
Restart=always
RestartSec=1

[Install]
WantedBy=crio.service
```
## Drop-in config support for c/storage
A configuration for `additionallayerstores` needs to be added in `/etc/containers/storage.conf.d/lazy-image-pull.conf` on the node as follows,
```toml=
[storage.options]
additionallayerstores = ["/var/lib/stargz-store/store:ref"]
```
As of now, that `c/storage` configuration file does not support drop-in config. The entire content of that file is [hard-coded in MCO](https://github.com/openshift/machine-config-operator/blob/release-4.16/templates/common/_base/files/container-storage.yaml). Without the drop-in config support, we risk overriding that critical file and potentially losing custom configuration that might be already present in there. A feature request for adding drop-in config support for applying similar custom configurations to `c/storage` [already exists](https://github.com/containers/storage/issues/1306). Hence, this proposal advocates adding support for drop-in config in `c/storage`.

### Risks and Mitigations

## Enhancing c/storage

Even in the absence of drop-in configuration support within `c/storage`, the current [hard-coded](https://github.com/openshift/machine-config-operator/blob/release-4.16/templates/common/_base/files/container-storage.yaml) `/etc/containers/storage.conf` file can be updated by the MCO. However, the integration of drop-in configuration support would offer a more secure and robust method for managing this configuration file. Should the drop-in configuration feature not be implemented in `c/storage`, the MCO has the capability to read the existing, hard-coded `storage.conf` and appropriately inject the necessary `additionallayerstores` settings.

### Drawbacks

#### Resource Consumption with FUSE

The stargz-snapshotter employs a FUSE filesystem, which can lead to increased resource usage on the node during intensive local disk I/O operations. It’s important to note that this does not affect containers performing heavy I/O with local volume mounts, as they will not experience performance degradation. Although in near future this might get mitigated by [FUSE-Passthrough](https://www.phoronix.com/news/Linux-6.9-FUSE-Passthrough)


## Test Plan

#### OpenShift End-to-End Testing

Write comprehensive e2e tests to confirm the accurate lazy pulling and execution of containers utilizing `zstd:chunked` images.

#### Performance Benchmarking

Benchmarks comparing the performance of layz pulling `zstd:chunked` images against traditional image usage across diverse operational scenarios.


## Graduation Criteria

### Dev Preview -> Tech Preview
* Introduce support for lazy image pulling per machine config pool in Openshift
* Drop-in configuration support in `c/storage`
* E2e tests in Openshift CI

### Tech Preview -> GA
* CI test stable for at least one release
* User facing documentation created in openshift-docs

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

When an upgrade or downgrade happens with incompatible changes in the store configuration (e.g. enabling/disabling Additional Layer Store), CRI-O handles this by rebuilding its image storage.
Rebuilding of the image storage is done following the existing `clean_shutdown_file` feature of CRI-O.
Concretely, CRI-O checks if layers in the storage are damaged and repairs the storage if there are damaged layers.

CRI-O detects incompatibe changes of storage configuration and repaires its storage using the following subcommand.

```
crio check-layer-state <layer_integrity_state_marker> <layer_integrity_state_file>
```

This subcommand runs as a separated service before CRI-O starts as a container engine.

When this subcommand starts for the first time on the node, it creates a file (call it "state file") at the location specified by `layer_integrity_state_file` with the contents of `layer_integrity_state_marker` (call it "state marker").
Everytime this subcommand starts, it checks that `layer_integrity_state_marker` exactly matches the contents of the state file at `layer_integrity_state_file`.
If it doesn't match, it treats this as an incompatible change and starts repairing the storage as mentioned above.

MCO adds this subcommand as a separated service running before CRI-O starts.
MCO populate the state marker with configurations whose changes trigger repairing of the storage, e.g. `version=1;store-format=1;ALS="estargz:/var/"`.
The state marker must be an empty string if lazy pulling is disabled and must not be empty if lazy pulling is enabled.

This subcommand doesn't run if CRI-O is downgraded to a version that doesn't support lazy pulling and state files.
Thus, downgrading requires manual wiping of images on the node.

## Version Skew Strategy

#### During an upgrade, we will always have skew among components, how will this impact your work?

N/A

#### Does this enhancement involve coordinating behavior in the control plane and in the kubelet?

No

#### How does an n-2 kubelet without this feature available behave when this feature is used?

If the n-2 kubelet is running on the host without the required `additionallayerstores` configuration for container storage library then the container that use `zstd:chunked` image format will start without any associated performance gains.

#### Will any other components on the node change?

Yes, configuration for the `c/storage` library used by CRI-O.

## Operational Aspects of API Extensions

#### For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level Indicators) an administrator or support can use to determine the health of the API extensions

metrics `kubelet_pod_start_sli_duration_seconds`, `kubelet_pod_start_duration_seconds` abd `kubelet_runtime_operations_duration_seconds` can be used to gauge the performance of the container start up time.

#### What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput, API availability)

Container that use large images but initiate the execution with a relatively small binary (e.g. using `ping` from fedora:39 container image) in that image should see faster start up time.

#### How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review this enhancement)

Container start up time can be measured by an e2e CI test to validate the expected speed up.

#### Describe the possible failure modes of the API extensions.

MCO could encounter failures in rolling out the required machine config to enable this feature.

#### Describe how a failure or behaviour of the extension will impact the overall cluster health (e.g. which kube-controller-manager functionality will stop working), especially regarding stability, availability, performance and security.

If MCO fails to roll out required machine config to enable this feature, the container images that use `zstd:chunked` format will not observe improved start up times associated with their `zstd:chunked` image.

#### Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes and add them as reviewers to this enhancement.

OCP Node Team

## Support Procedures

### Detect the failure modes in a support situation, describe possible symptoms (events, metrics, alerts, which log output in which component)

* If MCO fails to roll out required machine config to enable this feature, the container images that use `zstd:chunked` format will not observe any associated performance gains in their start up time.
* If the workload is going to do heavy local disk IO, then this feature may contribute to higher resource usage on the node due to the use of FUSE filesystem.
* Useful metrics to observe container start up time,  `kubelet_pod_start_sli_duration_seconds`, `kubelet_pod_start_duration_seconds` abd `kubelet_runtime_operations_duration_seconds`.

### Disable the API extension (e.g. remove MutatingWebhookConfiguration xyz, remove APIService foo)

#### What consequences does it have on the cluster health?

MCO will reboot the nodes to reflect the configuration change.

#### What consequences does it have on existing, running workloads?

MCO will remove the required configuration in the container storage library on the host and reboot the node.
`crio check-layer-state` service rebuilds the image storage by removing damaged images by the configuration change (as described in "Upgrade / Downgrade Strategy" section).
When containers run from the removed images next time, they will be pulled again without lazy pulling.

#### What consequences does it have for newly created workloads?

If the workload image was never lazily pulled, then there won't be any impact. 
But, if the backing container image was ever lazily pulled then this image will be removed by `crio check-layer-state` as described above.
Creating this workload will trigger the pulling of that image without lazy pulling.

### Does functionality fail gracefully and will work resume when re-enabled without risking consistency?

Yes

## Alternatives

1. [Seekable OCI images](https://aws.amazon.com/blogs/containers/under-the-hood-lazy-loading-container-images-with-seekable-oci-and-aws-fargate/) - Only works with [containerd](https://github.com/awslabs/soci-snapshotter/blob/main/docs/install.md).
