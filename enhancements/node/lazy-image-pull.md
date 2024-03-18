---
title: lazy-image-pull
authors:
  - @harche
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @mrunalp
  - @rphillips
  - @sairameshv
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - @mrunalp
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - @mrunalp
creation-date: 2024-03-18
last-updated: 2024-03-18
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-2204
---

# Enable Lazy Image Pulling Support in OpenShift


## Summary

This enhancement proposes integrating support for lazy image pulling in OpenShift using [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter). This will provide the ability to lazy-pull compatible container images, significantly reducing container startup times and potentially decreasing network strain for the specific workloads that can benefit from it.

## Motivation

* **Faster Serverless:** - Serverless end points need to serve the cold requests as fast as possible. Lazy-pulling of the images allow the serverlesss end point to start serving the user request without getting blocked on the entire container image to get downloaded on the host.
* **Improved User Experience:** Large images often lead to extended pod startup times. Lazy-pulling drastically reduces the initial delay, providing a smoother user experience.
* **Potential Benefits for Large Images:** If the container image is shared between various containers which utilize only part of the image during their execution, Lazy image pulling speeds up such container startup by downloading only the relavent bits from those images.

### User Stories


* As a cluster administrator, I want to serve serverless endpoints on my cluster as fast as possible by not waiting for the entire container image to download.
* As a developer, I want to deploy applications packaged in large container images while minimizing the impact of image size on pod startup time.
* As a cluster administrator, I want to reduce network bandwidth usage in my OpenShift environment, especially when dealing with large container images.

# Goals

* Modify `c/storage` to support drop-in config
* Package upstream [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter) for the installation on RHCOS
* Enable lazy image pulling in Openshift using [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter) which uses [eStargz](https://github.com/containerd/stargz-snapshotter/blob/main/docs/estargz.md) image format.
* Support lazy image pulling only on the worker nodes (or subset of worker nodes).

# Non-Goals

* Enabling lazy image pulling on the control plane nodes.
* Replacement of the standard OCI image format; `eStargz` will be an additional supported format.
* Alterations to the image build process; developers can use standard OCI build tools, such as buildah to build and convert images to `eStargz` format.

## Proposal

In order to use `eStargz` images, we need to have [Stargz Store plugin](https://github.com/containerd/stargz-snapshotter/blob/aaa46a75dd97e401025f82630c9d3d4e41c9f670/docs/INSTALL.md#install-stargz-store-for-cri-opodman-with-systemd) installed on the node and with a configuration for container storage library. Existing MCO [node controller](https://github.com/openshift/machine-config-operator/blob/53bbe70847393935e8f2c3f83c739fbb477cb84d/pkg/controller/node/node_controller.go#L75) for `node.config` object can be used to roll out this configuration to all or [set of specific worker nodes](https://github.com/openshift/machine-config-operator/blob/release-4.16/docs/custom-pools.md).

### Workflow Description

* A cluster administrator will create or edit the [node.config](https://github.com/openshift/api/blob/610cbc77dbab208d5364bba8982385bd148025c1/config/v1/types_node.go#L19) object to enable lazy image pulling support for all worker nodes by, `oc edit nodes.config -o yaml`
```yaml
apiVersion: config.openshift.io/v1
kind: Node
metadata:
  name: cluster
  spec:
    lazyImagePools:
      - "worker"
```
or for specific machine config pool(s) by,
```yaml
apiVersion: config.openshift.io/v1
kind: Node
metadata:
  name: cluster
  spec:
    lazyImagePools:
      - "serverless-custom-pool1"
      - "serverless-custom-pool2"
```
*  Existing MCO [node controller](https://github.com/openshift/machine-config-operator/blob/53bbe70847393935e8f2c3f83c739fbb477cb84d/pkg/controller/node/node_controller.go#L75) for `node.config` object, which is responsible for reconciling any changes to that object, would recongize the user's intent to enable the support for lazy image pulling.
* Before rolling out the configuration to the specified machine config pools, node controller will validate that they are not inherited from the `master` pool.
* Node controller then provisions a machine config consisting of required changes that would be rolled out to specified machine config pool(s).

### API Extensions

Extend existing [NodeSpec](https://github.com/openshift/api/blob/610cbc77dbab208d5364bba8982385bd148025c1/config/v1/types_node.go#L36) to hold the user specified machine config pool(s).
```golang=
type NodeSpec struct {
    LazyImagePools []string `json:"lazyImagePools,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

## Package Stargz Store plugin

Package the Stargz Store plugin located at, https://github.com/containerd/stargz-snapshotter so that it can get installed on RHCOS

## Drop-in config support for c/storage
A configuration for `additionallayerstores` needs to be added in `/etc/containers/storage.conf.d/estargz.conf` on the node as follows,
```toml=
[storage.options]
additionallayerstores = ["/var/lib/stargz-store/store:ref"]
```
As of now, that `c/storage` configuration file does not support drop-in config. The entire content of that file is [hard-coded in MCO](https://github.com/openshift/machine-config-operator/blob/release-4.16/templates/common/_base/files/container-storage.yaml). Without the drop-in config support, we risk overriding that critical file and potentially losing custom configuration that might be already present in there. A Request for adding drop-in config support for applying similar custom configurations to `c/storage` [already exists](https://github.com/containers/storage/issues/1306). Hence, this proposal advocates adding support for drop-in config in `c/storage`.



## Starz Store systemd service

Startz Store service needs to be running on the host _before_ crio service starts. MCO needs to drop relavent systemd unit file on the targeted nodes.

```toml=
[Unit]
Description=Stargz Store plugin
After=network.target
Before=crio.service

[Service]
Type=notify
Environment=HOME=/home/core
ExecStart=/usr/local/bin/stargz-store --log-level=debug --config=/etc/stargz-store/config.toml /var/lib/stargz-store/store
ExecStopPost=umount /var/lib/stargz-store/store
Restart=always
RestartSec=1

[Install]
WantedBy=multi-user.target
```

### Risks and Mitigations

## Enhancing c/storage Configuration Management

Even in the absence of drop-in configuration support within `c/storage`, the current [hard-coded](https://github.com/openshift/machine-config-operator/blob/release-4.16/templates/common/_base/files/container-storage.yaml) `/etc/containers/storage.conf` file can be updated by the MCO. However, the integration of drop-in configuration support would offer a more secure and robust method for managing this configuration file. Should the drop-in configuration feature not be implemented in `c/storage`, the MCO has the capability to read the existing, hard-coded `storage.conf` and appropriately inject the necessary `additionallayerstores` settings.

### Drawbacks

#### Resource Consumption with FUSE

The stargz-snapshotter employs a FUSE filesystem, which can lead to increased resource usage on the node during intensive local disk I/O operations. It’s important to note that this does not affect containers performing heavy I/O with local volume mounts, as they will not experience performance degradation.


## Test Plan

#### OpenShift End-to-End Testing

Write comprehensive e2e tests to confirm the accurate pulling and execution of containers utilizing eStargz images.

#### Performance Benchmarking

Benchmarks comparing the performance of eStargz images against traditional image usage across diverse operational scenarios.


## Graduation Criteria
#### Dev Preview
* Introduce support for lazy image pulling per machine config pool in Openshift

## Upgrade / Downgrade Strategy

* Downgrade expectations - In the event of a cluster downgrade to a version that does not support eStargz image format, containers utilizing such image format will get downloaded again and their startup times would be comparable to the non-eStargz equivalent container image. Even in the absence of stargz-snapshotter, those container will start but without any performance gains.

## Version Skew Strategy

#### During an upgrade, we will always have skew among components, how will this impact your work?

During the upgrade, if the container that uses eStargz image format gets scheduled on the node that doesn't yet support it, the container will start without any performance gains associated with eStargz image format.

#### Does this enhancement involve coordinating behavior in the control plane and in the kubelet?

No

#### How does an n-2 kubelet without this feature available behave when this feature is used?

If the n-2 kubelet is running on the host without the required `additionallayerstores` configuration for container storage library then the container that use eStargz image format will start without any associated performance gains.

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

If MCO fails to roll out required machine config to enable this feature, the container images that use eStargz format will not observe improved start up times associated with their eStargz image.

#### Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes and add them as reviewers to this enhancement.

OCP Node Team

## Support Procedures

### Detect the failure modes in a support situation, describe possible symptoms (events, metrics, alerts, which log output in which component)

* If MCO fails to roll out required machine config to enable this feature, the container images that use eStargz format will not observe any associated performance gains in their start up time.
* If the workload is going to do heavy local disk IO, then this feature may contribute to higher resource memory and cpu usage on the node due to the use of FUSE filesystem.
* Useful metrics to observe container start up time,  `kubelet_pod_start_sli_duration_seconds`, `kubelet_pod_start_duration_seconds` abd `kubelet_runtime_operations_duration_seconds`.

### Disable the API extension (e.g. remove MutatingWebhookConfiguration xyz, remove APIService foo)

#### What consequences does it have on the cluster health?

Containers that use eStargz image format will observe no associated performance gains in their start up time.

#### What consequences does it have on existing, running workloads?

MCO will remove the required configuration in the container storage library on the host, which would result in container images using eStargz format getting downloaded like regular OCI container images.

#### What consequences does it have for newly created workloads?

MCO will remove the required configuration in the container storage library on the host, which would result in container images using eStargz format getting downloaded like regular OCI container images.

### Does functionality fail gracefully and will work resume when re-enabled without risking consistency?

Yes

## Alternatives

1. [Seekable OCI images](https://aws.amazon.com/blogs/containers/under-the-hood-lazy-loading-container-images-with-seekable-oci-and-aws-fargate/) - Only works with [containerd](https://github.com/awslabs/soci-snapshotter/blob/main/docs/install.md).
