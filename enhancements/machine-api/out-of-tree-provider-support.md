---
title: out-of-tree-provider-support
authors:
  - "@Danil-Grigorev"
  - "@Fedosin"
  - "@JoelSpeed"
reviewers:
  - "@enxebre"
  - "@JoelSpeed"
  - "@crawford"
  - "@derekwaynecarr"
  - "@eparis"
  - "@mrunalp"
  - "@sttts"
approvers:
  - "@enxebre"
  - "@JoelSpeed"
  - "@crawford"
  - "@derekwaynecarr"
  - "@enxebre"
  - "@eparis"
  - "@mrunalp"
  - "@sttts"
creation-date: 2020-08-31
last-updated: 2021-07-14
status: implementable
replaces:
superseded-by:
---

# Add support for out-of-tree cloud provider integration

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposal describes the migration of cloud platforms from the [deprecated](https://github.com/kubernetes/kubernetes/blob/222cae36ec667c2f12c8aaa73a86e3cf01d80f7e/cmd/kubelet/app/options/options.go#L389) [in-tree cloud providers](https://v1-18.docs.kubernetes.io/docs/concepts/cluster-administration/cloud-providers/)
to [Cloud Controller Manager](https://kubernetes.io/docs/concepts/architecture/cloud-controller/) services that implement the`external cloud provider` [interface](https://github.com/kubernetes/cloud-provider/blob/master/cloud.go).

## Motivation

Using Cloud Controller Managers (CCMs) is the Kubernetes [preferred way](https://kubernetes.io/blog/2019/04/17/the-future-of-cloud-providers-in-kubernetes/) to interact with the underlying cloud platform as it provides more flexibility and freedom for developers.
It replaces existing in-tree cloud providers, which have been deprecated and will be permanently removed approximately in Kubernetes `v1.24` (This aligns with OpenShift 4.11).
In-Tree providers are still used within OpenShift and we must start a smooth migration towards CCMs.

Another motivation is to be closer to upstream by helping develop Cloud Controller Managers for various platforms, which is benefiting both OpenShift and Kubernetes.

This change will help when adding support for other cloud platforms, starting with IBMCloud,
[Alibaba Cloud](https://github.com/kubernetes/cloud-provider-alibaba-cloud) and [Azure Stack Hub](https://kubernetes-sigs.github.io/cloud-provider-azure/install/configs/#azure-stack-configuration) as a sub-part of Azure [out-of-tree](https://github.com/kubernetes-sigs/cloud-provider-azure) cloud provider,
and later [Equinix Metal](https://github.com/equinix/cloud-provider-equinix-metal) and potentially others such as [Digital Ocean](https://github.com/digitalocean/digitalocean-cloud-controller-manager).

It is especially important to do this for OpenStack. By switching to the external cloud provider, many issues and limitations with the in-tree cloud provider are mitigated.
For example, the out-of-tree cloud provider no longer relies on the [Nova metadata service](https://docs.openstack.org/nova/latest/admin/metadata-service.html).
This would allow for OpenStack deployments on provider networks and at the edge.

### Goals

- Prepare OpenShift components to accommodate the CCMs instead of deprecated in-tree cloud providers.
- Provide the means to allow management of generic `CCM` component based on the platform specific upstream implementation.
- Define and implement upgrade and downgrade paths between the in-tree and out-of-tree cloud provider configurations.
- Ensure full feature parity between the in-tree and the out-of-tree providers before migration to ensure core Kubernetes cloud features (eg. Load Balancer Services) continue to operate as before.

### Non-Goals

- Force an immediate transition for all cloud providers from in-tree support to their out-of-tree counterparts.
- Exclude in-tree storage provider plugins usage and force transition to CSI storage implementation.
- [CSI driver migration](https://kubernetes.io/blog/2019/12/09/kubernetes-1-17-feature-csi-migration-beta/) is out of scope of this work.
- Support for multiple providers within a single cluster (Though careful operator design may allow for introduction of this in the future).
- Allow customers to bring their own CCM implementations - only CCM configurations included in the OpenShift payload will be supported.

## Proposal

Our main goal is to start using Cloud Controller Manager in OpenShift 4, and make a seamless transition for all currently supported providers that use the in-tree `cloud provider` interface: OpenStack, AWS, Azure, GCP and vSphere.

To maintain the lifecycle of the CCM we will implement a new cluster operator called `cluster-cloud-controller-manager-operator`.
This new operator will handle all administrative tasks for the CCM: deployment, restore, upgrade, and so on.

### User Stories

#### Story 1

As a cloud developer, I’d like to improve support for cloud features for OpenShift. I'd like to develop and test new cloud providers which do not ship with default Kubernetes distribution, and require support for external cloud controller manager.

#### Story 2

As a developer I'd like to implement, build and release fixes for cloud provider controllers independently from the Kubernetes core, and assume they will first land upstream and then will be carried over into OpenShift distribution with less effort.

#### Story 3

As an OpenShift developer responsible for the cloud controllers, I want to share more code with upstream Kubernetes in order to ease feature and bug fix contributions in both directions.

#### Story 4

We’d like to discuss technical details related to a specific cloud in a SIG meeting with people who are also involved into development in this domain,
and that way gain useful insights into the cloud infrastructure, improve the overall quality of our features, stay on top of the new features, and improve the relations with maintainers outside of our company, which nevertheless share with us a common goal.

### Implementation Details

The `cluster-cloud-controller-manager-operator` implementation will be hosted within its own OpenShift [repository](https://github.com/openshift/cluster-cloud-controller-manager-operator). The operator will manage `CCM` provisioning for supported cloud providers.

#### Operator resource management strategy

Every cloud manages its configuration differently, but shares a common overall structure.

1. The operator will be provisioned in `openshift-cloud-controller-manager-operator` namespace.
2. `CCCMO` will provision operands under the `openshift-cloud-controller-manager` namespace.
3. The pod running `CCM` controllers in one or more (Azure) containers will be managed by a `Deployment`.

*There are `4` common controller loops for every cloud provider, these are: `route`, `service`, `cloud-node` and `cloud-node-lifecycle`.*
*The Cloud-provider [interface](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/cloud-provider/cloud.go#L42-L69) does not require implementation for all of them, and for non-implemented methods the interface returns (`<interface>, false`), indicating that this sub-interface is not supported.*

*Notably only the `cloud-node` controller is required during installation time, as it is responsible for the `Node` initialization procedure. This controller is only operational in conjunction with `--cloud-provider=external` flag on KCM and Kubelet. Other controllers could operate day 2, and use cluster with `--cloud-provider=none` configuration.*

*The rest of the controller loops are provider dependent features, built under their consideration. An example is the [GCP](https://github.com/kubernetes/cloud-provider-gcp/blob/08a120e3a8936c200ef77d0e3384eb730ad6be77/cmd/gcp-controller-manager/loops.go#L45) certificate approval and node annotator controllers.*
*In OpenShift the [cluster-machine-approver](https://github.com/openshift/cluster-machine-approver) will take any additionally required responsibility to auto-approve `CSR` resources.*

`CCCMO` will:

- Create and manage the cloud-specific `Deployment`, based on the selected `platformStatus` in the `Infrastructure` resource.
  - This `Deployment` will run the `CCM` pods on the `control-plane` nodes, and will tolerate the `cloudprovider.kubernetes.io/uninitialized` and `node-role.kubernetes.io/master` taints.
  - This will run at least `2` replicas of the pods with anti-affinity settings to prevent scheduling on the same machine to allow some fault-tolerance if a machine were to fail. Note that since OpenShift typically deploys masters across multiple failure domains, this should provide zone by zone fault tolerance without having to rely on cloud provider scheduling information.
- Own a `ClusterOperator` resource, which will report the readiness of all workloads at post-install phase. The conditions reported will help other dependent components, such as [openshift-ingress](https://github.com/openshift/cluster-ingress-operator).
- Syncronize the `cloud-config` `ConfigMap` into `openshift-cloud-controller-manager` namespace for use in cloud providers which require it to be present.
- Be built with consideration for potential future implementations of hybrid OpenShift clusters which support multiple cloud providers simultaneously (ie ensure resources created for each provider by the operator do not clash with resources for other providers).
*Note: static resource creation is delegated to CVO (SA, RBAC, Namespaces, etc.)*

#### CVO management

The operator image will be built and included in every release payload.
It is expected to be deployed and running at Kubernetes operators level ([10-29](https://github.com/openshift/cluster-version-operator/blob/34010292c3abd2582b687e9c0ef76c5924998f39/docs/dev/operators.md#how-do-i-get-added-as-a-special-run-level)) right after the `kube-controller-manager-operator` at runlevel [25](https://github.com/openshift/cluster-kube-controller-manager-operator/tree/master/manifests).

Proposed `CCCMO` manifests runlevel is `26`.

#### Cloud provider management

Each cloud provider will be hosted under its own repository. These repositories will be initially bootstrapped by forking their upstream counterpart.

The `CVO` will be responsible for provisioning static resources, such as `ServiceAccounts`, `Roles/RoleBindings`, and `ServiceMonitors` for the operator. Then, the operator will  be responsible for constructing and creating cloud provider resources.

Forked provider repositories will also contain cloud-provider specific code, and a set of binaries for each of them to build and run. Each provider will be built inside its own image, which will be included in the release payload. The operator is responsible for choosing which provider image should be used in a cluster based on the platform specified in the `Infrastructure` resource.

#### CCM migration from KCM pod

Currently the cloud-specific controller loops are running inside the `kcm` pod.
It is ok to run in-tree and out-of-tree `CCM` simultaneously in a cluster for a short amount of time, or have a short amount of downtime between the moment in-tree plugin shuts down, and out-of-tree starts, but this gap should not be experienced by components, which rely on `CCM` functionality.

We plan to have co-ordination between `KCMO` and `CCCMO` to ensure no overlap of the running control loops. This will be implemented via conditions added to their respective `ClusterOperator` resources.

The following components depend on `CCM` provided functionality:

- `openshift-ingress` with `Route` resource management: non-operational without `CCM`.
- `kubelet`: can't initialize a new `Node` without `CCM`.
Consumes `cloudprovider.Interface` in order to collect `Node` [address](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/nodestatus/setters.go#L65), [populate](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/kubelet_node_status.go#L388-L435) `spec.ProviderID` and `Labels`,
and [collect](https://github.com/kubernetes/kubernetes/blob/master/cmd/kubelet/app/server.go#L960-L978) `Node` name.

*Note: `storage`: in-tree plugins including storage implementation will remain untouched by the migration. This controller will remain preserved by using the `--external-cloud-volume-plugin` flag on `kcm` pod.*
*Although `CCM` controller loops itself does not require HA setup as they does not store any state, the in-tree storage plugins are, and they will be removed together with the `CCM` from in-tree when it happens.*

*The ability to support out-of-tree provider storage in a form of a CSI plugin and perform migration from in-tree storage plugin to the CSI one is the next step. Migration process design to CSI is described [here](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-migration.md). This is task is not a scope of this proposal.*

#### Credentials management

The operator is expected to be integrated with the [cloud-credentials-operator](https://github.com/openshift/cloud-credential-operator), and request fine grained credentials for the cloud components. It will be `CCO`'s responsibility to decide, depending on the platform, which credentials to request.

*Bootstrap phase:*

- Initial configuration with static pods is expected to be using `ConfigMap` credentials, similar to what is [done](https://github.com/openshift/installer/blob/ba6d7fe087eada5a4fc260064c85a484a8c45aaf/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L162) in the `kcm` operator.
- Once it is supported, the static pods will also use the `CredentialsRequest` to get cloud credentials.

*Post-install phase:*

- Using a `CredentialsRequest` is the default option, every supported `CCM` within the `Deployment` will request its own set of credentials.
- A set of `CredentialsRequest` resources will be hosted under `CCCMO` repository, and created by the `CVO`, similarly to the [machine-api](https://github.com/openshift/machine-api-operator/blob/6f629682b791a6f4992b78218bfc6e41a32abbe9/install/0000_30_machine-api-operator_00_credentials-request.yaml) approach.

Depending on the cloud-provider, when it is supported, credentials management is deligated to the instance metadata endpoint to be acquired from. `AWS` and `Azure` are currently known to support this feature in some extent.

#### Resource changes

#### Kubelet

In OpenShift, `machine-config-operator` manages `kubelet` configuration.
Based on the [Infrastructure value](https://github.com/openshift/api/blob/master/config/v1/types_infrastructure.go) MCO currently sets the correct `cloud-config` value.
To support external providers, MCO should not set the `cloud-config` value anymore.

Additionally, it must set the value of the `cloud-provider` [flag](https://github.com/openshift/machine-config-operator/blob/2a51e49b0835bbd3ac60baa685299e86777b064f/templates/worker/01-worker-kubelet/_base/units/kubelet.service.yaml#L32) to `external`.
This informs `kubelet` that an out-of-tree provider will finish the `Node` initialization process.

While `kubelet` itself does not directly contact or interact with the `CCM`, formation of a functional `Node` in Kubernetes does rely on the `CCM`.
With in-tree providers, `kubelet` looks up Node IP Addresses, Instance IDs and other metadata directly from the cloud provider and places this information onto the `Node` object.

When using out-of-tree, the `kubelet` taints the `Node` with `node.cloudprovider.kubernetes.io/uninitialized: NoSchedule` and does not look up any information.
It is then the responsibility of the `CCM` to look up the Node IP Addresses, Instance IDs and other metadata and labels, place the information onto the `Node` object and then remove the taint from the `Node`.

The information previously set by `kubelet`, now `CCM`, is required to make a `Node` schedulable.
Without this information, the `scheduler` cannot schedule `Pods` that have any sort of affinity that relies on `Node` metadata (eg failure domain).

To ensure that cluster disaster recovery procedures can still operate smoothly, we will ensure that core control plane components and their operators tolerate the uninitialized taint, to prevent `CCM` blocking new control plane hosts being added if `CCM` is non-functional.
This will include, but is not limited to: Kube Controller Manager, Etcd, Kube API Server, Networking, Cluster Machine Approver.

#### Windows Machine Config Operator

##### Kubelet Changes

To generate the configuration for Windows nodes, `WMCO` reads the output of MCO's rendered configuration as a basis for its own config for Kubelet on the Windows node.
As we are making changes to the output of the Kubelet service in `MCO` (namely change the value of the `cloud-provider` flag), we will need to verify that `WMCO` reads this flag and copies its value to the Kubelet Windows service.

##### Node Initialization

On most platforms, `Node` initialization is handled centrally by the `CCM`, specifically the Cloud Node Manager (`CNM`) running within it.
However, On certain platforms (e.g. Azure), the `CNM` must be run on the `Node` itself, typically via a `DaemonSet`.
Since Red Hat cannot supply or support Windows container images, we cannot run a `DaemonSet` for the `CNM` targeted at Windows Nodes as we would do on Linux Nodes.
Instead, we must adapt the `WMCO` to, on these particular platforms, deploy a new Windows service that runs the `CNM` on the `Node`.
This pattern is already in place for other components that are required to run on the host (eg `CNI` and Kube-Proxy), so we will be able to reuse the existing pattern to add support for `CNM` on platforms that require a `CNM` per host.

It is worth noting that `WMCO` is deployed via `OLM` and so there might be a `CNM` image version skew during cluster upgrades via `CVO` as Windows nodes will be updated independently from the rest of the cluster. This may last for a period of time, especially if `WMCO` upgrades are set to be manually approved by the cluster administrator.

##### Example flag changes for kubelet

Current flag configuration for kubelet in AWS provider:

```bash=
...
--cloud-provider=aws
# No cloud-config due to use of instance metadata service for acquiring credentials, instance region, etc.
...
```

After transition flag configuration will contain values:

```bash=
...
--cloud-provider=external
...
```

Current flag configuration for kubelet in Azure provider:

```bash=
...
--cloud-provider=azure
--cloud-config=/etc/kubernetes/cloud.conf
...
```

After transition flag configuration will contain values:

```bash=
...
--cloud-provider=external
...
```

#### Kube Controller Manager and Kube API Server

In OpenShift, `kube-controller-manager` and `kube-apiserver` are managed by `cluster-kube-controller-manager-operator` and `cluster-kube-apiserver-operator` respectively.
When switching to out-of-tree providers, the `cloud-provider` option [should be removed](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/#running-cloud-controller-manager) from their deployments.

##### Kube Controller Manager changes to prolong support for in-tree storage

In the first phase of the transition, all cloud providers will still rely on the in-tree storage. `--external-cloud-volume-plugin=<cloud-provider>` flag set on `KCM` pod will preserve the in-tree storage controller loops. `--cloud-provider=external` will allow out-of-tree to do the job, and disable `service`, `route` and `cloud-node-lifecycle` controllers in `KCM`.

At the later phases, CSI support and CSI migration will be responsible for removing `--external-cloud-volume-plugin=<cloud-provider>` flag from the `KCM`.

With the `--external-cloud-volume-plugin` flag set, existing designs for KCM-CCM migration are being preserved. The storage will temporarily become a lone in-tree controller depending on the cloud, and could could be excluded by setting removing this flag.
The combination of both flags means that `KCM` will still initialize the `cloudprovider.Interface`, but this initialization will only be used by storage controllers.

##### Example flag changes for Kube Controller Manager

Current flag configuration for `kube-controller-manager` in Azure provider:

```bash=
...
--cloud-provider=azure
--cloud-config=/etc/kubernetes/cloud.conf
...
```

After transition flag configuration will contain values:

```bash=
...
--cloud-provider=external
--external-cloud-volume-plugin=azure
--cloud-config=/etc/kubernetes/cloud.conf
...
```

In the last example `--cloud-config` is still required for correct function of `--external-cloud-volume-plugin` preventing disabling of in-tree volumes in KCM. Production use of CSI and CSI migration for Azure will notify about `--external-cloud-volume-plugin` and `--cloud-config` flag removal.

#### Pre-release / Development Feature Gate

In order to achieve desired cluster configuration for cloud components during development and technical preview, we will add a new OpenShift [FeatureGate](https://docs.openshift.com/container-platform/4.5/nodes/clusters/nodes-cluster-enabling-features.html)
resource named `ExternalCloudProvider`, which will allow developers and customers to force their cluster to upgrade from in-tree to out-of-tree cloud providers.

This feature gate will be shipped with `TechPreviewNoUpgrade` `FeatureSet`, preventing upgrades.

The feature gate will be used to determine whether or not operators should migrate their components to the out-of-tree configuration.
If the feature gate is found, the operators for components such as `kube-apiserver`, `kube-controller-manager` and `kubelet` will update their component's configuration accordingly to allow hosting external `CCM` for current platform.

Once an out-of-tree provider implementation is ready to be released (GA), it will no longer depend on the feature gate, allowing production clusters to automatically upgrade to the out-of-tree implementation during 4.N to 4.N+1 upgrade.

Once all providers are moved out-of-tree, the feature gate will be safe to remove.

This feature gate is currently used in CI tasks, to test migration and installation stability.

Additionally, the FeatureGate will host required featureGates for CSI migration, as CCM migration and CSI migration should be started simultaniously. CSI migration has a preference, and could be started earlier.

**Note: At later stages this FeatureGate will be removed in favor of upstream one with similar effects, as noted in the upstream [enhancement](https://github.com/kubernetes/enhancements/pull/2443).**

#### Migration procedure

The process of the migration will be managed by corresponding operators. Communication will be done via `CloudControllerOwner` condition on the operator's `ClusterOperator` resource.

```yaml
conditions:
- type: CloudControllerOwner
  status: True
```

`status` will indicate the state of the ownership for this code. Setting it to `False` on one `ClusterOperator` will unblock resource provisioning for the other operator. Absent condition on operator evaluated as `CloudControllerOwner` `status: True` for KCM and Kubelet operators, and will block `CCCMO` from provisioning `CCM` resources.

The plan for migration is the following:

1)
    - `kubelet`
        1) Disable cloud loops in it's operands.
        2) Wait for migration to complete (new configs injected into nodes, `kubelet` service restarted and became healthy)
        3) Communicate the completion of the migration stage on `MCO` owned `ClusterOperator` resource for the`CCCMO` to pick up.
    - `kube-controller-manager`
        1) Configure flags according to [example](#Example-flag-changes-for-Kube-Controller-Manager) for `kube-controller-manager`, disabling cloud controller loops.
        2) Wait for migration to complete (static pod rolled out, `kube-controller-manager` restarted and became healthy)
        3) Communicate the completion of the migration stage on `KCMO` owned `ClusterOperator` resource for the`CCCMO` to pick up.
2) `cloud-controller-manager`
    1) Waits for preceding operators to report `CloudControllerOwner` condition.
    2) Starts provisioning out-of-tree resources.
3) Once necessary deployments are up and running, the migration is completed.

*Note:`kube-apiserver` migration is not important, as it currently depends on `cloud.Interface` only in establishing SSH node tunnelling functionality unused in `OpenShift` and [deprecated](https://github.com/kubernetes/kubernetes/blob/master/cmd/kube-apiserver/app/options/options.go#L197-L205) since 1.9*.

Before it is decided to make out-of-tree a default selection for the release, the first step will be initiated by the `FeatureGate` resource described above. In the designated release for switch from in-tree for a particular platform, the first step will be a part of default upgrade procedure.

Once the provider is moved to out-of-tree, the migration mechanism will be disabled. When all existing in-tree providers are moved to out-of-tree, the implementation will be safe to remove.

#### Bootstrap changes

One of the responsibilities of the initialisation process for Kubelet is to set the `Node`’s IP addresses within the status of the `Node` object. The remaining responsibilities are not important for bootstrapping, bar the removal of a taint which prevents workloads running on the `Node` until the initialisation has completed.

A second part of the bootstrap process for a new `Node`, is to initialise the `CNI` (networking). Typically in an OpenShift cluster, this is handled once the Networking Operator starts.
The Networking operator will create the `CNI` pods (typically OpenShift SDN), which schedule on the `Node`, use the `Node` IP addresses to create a `HostSubnet` resource within Kubernetes and then complete the initialisation process for the `CNI`, in doing so, marking the `Node` as ready and allowing the remaining workloads to start.

Before the `CNI` is initialized on a `Node`, in-cluster networking such as Service IPs, in particular the API server Service, will not work for any `Pod` on the `Node`. Additionally, any `Pod` that requires the Pod Networking implemented by `CNI`, cannot start.
For this reason, `Pods` such as the Networking Operator must use host networking and the “API Int” load balancer to contact the Kube API Server.

Because the `CCM` is taking over the responsibility of setting the `Node` IP addresses, `CCM` will become a prerequisite for networking to become functional within any Cluster. Because the `CNI` is not initialised, we must ensure that the `CCCMO` and `CCM` Pods tolerate the scenario where `CNI` is non-functional.

To do so, we must tolerate the not-ready taint for these pods and they must all run with host networking and use the API load balancer, rather than using the internal Service. This will ensure that the cluster can bootstrap successfully and recover from any disaster recovery scenario.

Our operator will become a prerequisite for the Network Operator. CCCMO will tolerate the `Node` `NotReady:NoSchedule` and `CCM` specific `Uninitialized` taints. CCCMO will start as the first operator on the control plane hosts, and be responsible for initializing `Nodes`, allowing other operators to start.

#### Metrics

In-tree `CCM` is responsible for exposing metrics measuring cloud API response failure rate and request duration. These metrics are not always preserved in the out-of-tree.

Metrics compatibility for providers:

- [AWS](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/legacy-cloud-providers/aws/aws_metrics.go) - [preserved](https://github.com/kubernetes/cloud-provider-aws/blob/master/pkg/providers/v1/aws_metrics.go).
- [GCP](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/legacy-cloud-providers/gce/metrics.go) - removed  in favor of `CSR` [metrics](https://github.com/kubernetes/cloud-provider-gcp/blob/708427eebf4b578936d762a2651bd3e3f46cca9f/pkg/csrmetrics/csrmetrics.go#L59-L84).
- [Azure](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/legacy-cloud-providers/azure/metrics/azure_metrics.go) - [removed](https://github.com/kubernetes-sigs/cloud-provider-azure/blob/e2a9a2bde8d6642a5c76063e2c6055b88b15c97a/pkg/version/prometheus/prometheus.go).
- [VSphere](https://github.com/kubernetes/legacy-cloud-providers/blob/master/vsphere/vclib/vsphere_metrics.go) - [preserved](https://github.com/kubernetes/cloud-provider-vsphere/blob/master/pkg/common/vclib/vsphere_metrics.go).
- [OpenStack](https://github.com/kubernetes/legacy-cloud-providers/blob/master/openstack/openstack_metrics.go) - [preserved](https://github.com/kubernetes/cloud-provider-openstack/blob/master/pkg/cloudprovider/providers/openstack/metrics/metrics.go).

`CCM` metrics will be collected by the by the [prometheus-operator](https://github.com/openshift/prometheus-operator) via `ServiceMonitor`. The `Service` endpoint will be secured with generated certificate in a `Secret` by setting the `service.beta.openshift.io/serving-cert-secret-name` annotation.

Exposed metrics is a subject for change in the future. Current values describing api request rate, throttling, etc. are no longer required, as out-of-tree plugins cache and reuse values requested once from cloud metadata by storing them on the `Node` object.

### Disaster Recovery

In the original `KCM` implementation, where the `CCM` is currently hosted, is using a [semi-automated](https://docs.openshift.com/container-platform/4.5/backup_and_restore/disaster_recovery/scenario-2-restoring-cluster-state.html) approach to disaster recovery, introducing the `forceRedeploymentReason` option in the managed CR, triggering the redeployment of the static pods, running the `KCM` code.

`CCM` approach is simpler, as we run the `CCCMO` which manages the `Deployment`. Re-populating etcd will revert the old image, and the previous version of the `CCCMO` code will re-apply and restart the old `CCM` configuration.

### Upgrade / Downgrade Strategy

A set of `cloud-controller-manager` pods will be running under a `Deployment` resource, provisioned by the operator. The leader-election will preserve the leadership during the cluster updates over cloud-specific loops.

Upgrades from previous versions of OpenShift will look like:

#### Post 4.9 (in-tree to out-of-tree per provider)

Each provider will be released independently as and when the providers are deemed stable enough.

Upgrade:

- New `cluster-kube-apiserver-operator` and `cluster-kube-controller-manager-operator` version updates the `kube-apiserver` and the `KCM` and removes the `--cloud-provider` flag. `cluster-kube-controller-manager-operator` in addition sets the `--external-cloud-volume-plugin=<some-platform>` flag on the `KCM` pods to ensure in-tree storage preservation.
- `cluster-cloud-controller-manager-operator` starts and creates out-of-tree `CCM` resources for the provider.
- `machine-config-operator` restart updates the `kubelet` with `--cloud-provider=external` option. At this point the `CCM` is running out-of-tree

Downgrade:

- `cluster-kube-apiserver-operator` and `cluster-kube-controller-manager-operator` restart updates the `kube-apiserver` and the `kube-controller-manager` with the `--cloud-provider=<some-platform>` option.
- `cluster-kube-controller-manager-operator` removes the `--external-cloud-volume-plugin=<some-platform>` flag from the `KCM` pods as `--cloud-provider=<some-platform>` superseeds it.
- `machine-config-operator` restart applies an old machine configuration, and cause `kubelet` to restart with `--cloud-provider=<some-platform>` option.

#### Providers, already running out-of-tree CCM (4.10+)

Upgrade:

- `cluster-cloud-controller-manager-operator` updates the state of resources for any in-tree provider defaulting to this since previous releases.

Downgrade:

- `cluster-cloud-controller-manager-operator` downgrades the state of resources for any in-tree provider defaulting to this since previous releases.

### Version Skew Strategy

See the upgrade/downgrade strategy.

### Action plan (for OpenStack)

To validate the changes described above, a single platform (OpenStack) will be migrated in the first instance.
The OpenStack out-of-tree provider supports more features than its in-tree counterpart,
crucially it no longer requires the Nova Metadata service which will expand the options for deploying OpenShift clusters on OpenStack.

While `CSI Migration` is not yet ready to allow existing clusters to be upgraded, the `CSI Driver` is in a state in which new clusters could be bootstrapped using `CCM` and `CSI`. This will be available as a Tech Preview initially.

#### Build OpenStack CCM image using OpenShift automation

To start using OpenStack CCM in OpenShift we need to build its image and make sure it is a part of the OpenShift release image.
The CCM image should be automatically tested before it becomes publicly available.

Actions:

- Configure CI operator to build OpenStack CCM image.

The CI operator will run an End-to-End test suite against the CCM and also push the resulting image into the OpenShift Quay account.

#### Test Cloud Controller Manager manually

When all required components are built, we can manually deploy the OpenStack CCM and test how it works.

Actions:

- Use development steps to install CCM on a working OpenShift cluster deployed on OpenStack with in-tree plugin.
- Check the CCM functionality is operational at this point.

**Note:** Example of a manual testing: https://asciinema.org/a/303399?speed=2

#### Test Cloud Controller Manager in CI

Automate OpenStack CCM testing in CI. Introduce a number of CI jobs for the provider.

Actions:

- Unit, local OpenStack CCM build.
- Release image build.
- Run cluster installation with out-of-tree OpenStack provider by default.
- Cluster upgrade from OpenStack in-tree to out-of-tree.
- Run existing e2e tests for the provider code, to confirm it's functionality.

#### Integrate the solution with OpenShift (Step 1)

Actions:

- `CCCMO` supports deploying OpenStack `CCM`.
- `KCM` runs only in-tree OpenStack storage implmentation.

#### Integrate the solution with OpenShift (Step 2)

Actions:

- Make sure the Cinder CSI driver is supported in OpenShift.
- Make sure the Cinder CSI migration is supported in OpenShift.
- `KCM` excludes the rest of cloud-controller loops by running CSI storage out-of-tree.

#### Examples for other cloud-controller-manager configurations

- AWS: https://github.com/kubernetes/cloud-provider-aws/. **Demo:** , to reproduce see: https://github.com/cloud-team-poc/cloud-provider-aws. [![asciicast](https://asciinema.org/a/9QSHP9ZRWvwWNoxEW2rEhLm5m.svg)](https://asciinema.org/a/9QSHP9ZRWvwWNoxEW2rEhLm5m)
- Azure: https://github.com/kubernetes-sigs/cloud-provider-azure
Azure separated the `CCM` and the "cloud-node-manager" (`CNM`) into two binaries: https://github.com/kubernetes-sigs/cloud-provider-azure/tree/master/cmd.
- GCP: https://github.com/kubernetes/cloud-provider-gcp
- vSphere: https://github.com/kubernetes/cloud-provider-vsphere
vSphere introduced an additional service to expose internal API server: https://github.com/kubernetes/cloud-provider-vsphere/blob/master/manifests/controller-manager/vsphere-cloud-controller-manager-ds.yaml#L64

### Risks and Mitigations

#### Specific CCM doesn’t work properly on OpenShift

The `CCMs` have not been tested on OCP yet. Unknown issues may arise.

Severity: medium-high

Likelihood: medium

The likelihood of the risk above varies per provider. None of the providers will be shipped without testing.

## Design Details

### Test Plan

**All following test plan details have a hard requirement - support for teach preview FeatureGate described in this [section](#Pre-release-/-Development-Feature-Gate). No development or testing could be done before the functionality is implemented.**

- Make sure the `CCM` implementation for "any" cloud is up and running in a bootstrap phase before or shortly after the `kubelet` is running on any `Node`.
- Ensure that transition between the in-tree and the out-of-tree will be handled by the established architecture with the new release payload, where all of the existing `Nodes` are operational during and after the upgrade time, and the bootstrap procedure for new ones is successful.
- Ensure that upgrade from a release running on the in-tree provider only to the out-of-tree succeeds, and the downgrade is also supported with proper CI tasks.
- Ensure that upgrade from a release running on the in-tree provider only to the out-of-tree does not disrupt in-tree storage functionality with proper CI tasks.
- Each provider should have a working CI implementation, which will assist in testing the provisioning and upgrades with out-of-tree support enabled.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech preview

- `CCCMO` handles cluster installation with out-of-tree `CCM` and defaulting to in-tree provider for storage for at least one cloud-provider.

#### Tech Preview -> GA

- `CCCMO` handles basic tasks, such as initial installation templates for bootstrap and provision all of supported in-tree providers after installation.

## FAQ

Q: Can we disaster-recover a cluster without CCM running?

- A: Yes we can, assuming the recovered state of the cluster has the `CCM` established properly, or it is still using in-tree implementation.

Q: What is non-functional in a cluster during bootstrapping until CCM is up?

- A: If CCM is not available, new nodes in the cluster will be left unschedulable with `node.cloudprovider.kubernetes.io/uninitialized` taint. This taint has `NoSchedule` effect, which will prevent most workloads from scheduling on the Node until CCM complete Node initialization.

Q: Who talks to CCM?

- A: CCM exposes health, readiness, metrics and config endpoints similar to most core Kubernetes controllers.
These endpoints are accessed by the process host's Kubelet to ensure the process is running smoothly and Prometheus to gather metrics.
No other components talk to the CCM directly. The CCM coordinates with other components by updating resources using the KAS similar to other controllers.

Q: CCM relation to other openshift components, such as SDN and storage? How a non-operational CCM will affect cluster health, which components will take a hit?

- A: A couple of components rely on CCM at different time of the cluster lifetime:
  - `kubelet` dependency requires removal of taints from newly created Nodes. Without CCM, new nodes will not become schedulable, existing nodes are unaffected.
  - CCMs manage `LoadBalancer` `Service` resources in all in-tree cloud implementations, except vSphere, so their creation will not be possible while the CCM is down.
  This will affect the [ingress-operator](https://github.com/openshift/cluster-ingress-operator).
  Cloud networking across `Nodes` is [disabled](https://github.com/openshift/cluster-kube-controller-manager-operator/blob/8be94db9da523152af2268bd7f891fe089a424eb/bindata/v4.1.0/config/defaultconfig.yaml#L8-L9) by default for all clouds, so the routes will not be provisioned for the Node by the `CCM` (which is done by `Routes()` method in the interface), and so not required for SDN functionality.
  - Storage CSI migration and CSI provider is a requirement for each cloud provider.

Q: Should we reuse the existing cloud provider config or generate a new one?

- A: CCM config is backward compatible with the in-tree cloud provider, therefore we can reuse it.

Q: How to migrate PVs created by the in-tree cloud provider?

- A: [CSIMigration](https://kubernetes.io/blog/2019/12/09/kubernetes-1-17-feature-csi-migration-beta/) is the preferred option. It graduates in GA in 1.19 (1.20).

Q: Does CCM provide an API? How is it hosted? HTTPS endpoint? Through load balancer?

- A: Not at the moment. The CCM vendors part of the code serving cloud features from the [cloud-provider](https://github.com/kubernetes/cloud-provider). CCM provides a secure TCP connection on port [10258](https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/cloud-provider/ports.go#L22) and it is only used for  metrics and readiness/health endpoint.

Q: What happens if the kcm leader has no access to ccm? Can it continue its work? Will it give up leadership?

- A: KCM and CCM perform independently after the initial migration process. KCM will be unaffected by CCM being down after the initial migration. The CCM cluster operator will be responsible for transitioning to Unavailable or Degraded in a similar situation.

Q: What are the thoughts about certificate management?

- A: At present, only the GCP provider has any CSR approval mechanism built in. This functionality is used to approve `NodeClient` and `NodeServer` certificates for Nodes on GCP.
This functionality should not be required as OpenShift handles certificate approval via the `cluster-machine-approver` component.

Q: Does every node need a CCM?

- A: No. The cluster only needs one active `CCM` at any time. A `Deployment` will manage the `CCM` pod and will have 2 replicas which will use leader election to nominate an active leader and maintain HA by scheduling on control-plane nodes located in the different regions.
Only in some scenarios (depending on the cloud provider implementation) like Azure `cloud-node-manager` has to run on all the `Nodes` due to 1:1 relation betwen `Node` and a `CNM` replica in their case.
[Source](https://kubernetes.io/docs/concepts/overview/components/#cloud-controller-manager) This assumption may change in the future, as `CCM` may run in worker nodes to determine the state of the instance.

Q: How metrics are affected by the CCM migration?

- A: Currently the `CCM` implementation is using the same set of metrics from the core repository, all existing metrics will still be available post migration.
The `CCMO` will be required to create a `ServiceMonitor` to instruct prometheus to collect metrics from the `CCM` pod. Exposed metrics will change in the future, as `KCM` metrics are redundant by design with `CCM` usage.

### Other notes

#### Removing a deprecated feature

Upstream is currently planning to remove support for the in-tree providers with kubernetes 1.24 (OpenShift 4.11).

The enhancement [describes](https://github.com/kubernetes/enhancements/blob/473d6a094f07e399079d1ce4698d6e52ddcc1567/keps/sig-cloud-provider/20190125-removing-in-tree-providers.md) steps towards this process.
Our goal is to have a working alternative for the in-tree providers ready to be used on-par with the old implementation before upstream remove this feature from core Kubernetes repository.

The `cloud-provider` flags will be allowed to set to `external`, defaulting to in 1.24 and locked on the value in 1.25 - [proposal](https://github.com/andrewsykim/enhancements/blob/49e40d65e106cf6ea05502656bf48cb0e67f4894/keps/sig-cloud-provider/2395-removing-in-tree-cloud-providers/README.md#phase-4---disabling-in-tree-providers), [discussion](https://github.com/kubernetes/kubernetes/pull/90408#issuecomment-670057176)
and [PR](https://github.com/kubernetes/kubernetes/pull/100136).

## Timeline

### Upstream

Follow the Kubernetes community discussions and implementation/releases

- vSphere support has graduated to beta in [v1.18](https://github.com/kubernetes/enhancements/issues/670) (Openshift 4.5)

- Azure support goes into beta in [v1.20](https://github.com/kubernetes/enhancements/issues/667) (Openshift 4.7)

### OpenShift

#### OCP 4.9

##### Technical preview

- AWS on out-of-tree `CCM` tech-preview. `CSI` is optional (via OLM) since [4.6](https://github.com/openshift/enhancements/blob/master/enhancements/storage/csi-driver-install.md#ocp-45-kubernetes-118).

##### General availability

- Azure - support for Azure Stack. Bootstrap and post-install phase supported by operator.
- OpenStack platform on out-of-tree `CCM` graduates as tech-preview.

#### OCP 4.10

- OpenStack upgrade and installation with the out-of-tree CCM by default. (GA)
- vSphere as a TP.
- GCP.

### Release boundaries

- Upper boundary for the out-of-tree `CCM` migration and a fresh installation is the in-tree plugins removal. It is estimated for [1.23](https://github.com/kubernetes/enhancements/pull/1665#issuecomment-703140901) (OCP 4.10).
- Lower boundary for the fresh cluster install is the `CSI` support for the provider - [timeline](https://github.com/openshift/enhancements/blob/master/enhancements/storage/csi-driver-install.md#timeline).
- Lower boundary for the upgrade - hard requirement is the `CSI Migration` for the provider. Initial release for that feature is OCP 4.8.

#### CSI support and migration timelines

CCM GA must go in sync with CSI migration GA, as CCM implies CSI migration and
we can't have in-tree volume plugin (GA) replaced by CSI driver which is beta / tech preview.

Current estimates:

- `AWS`, `GCE`, `OpenStack`, `Azure`: GA in 1.23 (4.10)
- `vSphere`: GA in 1.24 (4.11)

## Implementation History

- https://github.com/openshift/library-go/pull/993 - IsCloudProviderExternal helper method to use across OpenShift
- https://github.com/openshift/library-go/pull/895 - KCM and KAPI config observer support for --cloud-provider=external flag
- https://github.com/openshift/cluster-kube-controller-manager-operator/pull/450 - KCM support for --cloud-provider=external flag
- https://github.com/openshift/cluster-kube-apiserver-operator/pull/953 - KAPI support for --cloud-provider=external flag
- https://github.com/openshift/machine-config-operator/pull/2386 - MCO support for --cloud-provider=external flag in ignition
- https://github.com/openshift/cluster-cloud-controller-manager-operator/pull/9 - operator synchronization loop for provisioned resources
- https://github.com/openshift/cluster-cloud-controller-manager-operator/pull/15 - AWS cloud controller manager integration
- https://github.com/openshift/cluster-cloud-controller-manager-operator/pull/10 - OpenStack cloud controller manager integration
- https://github.com/openshift/cluster-cloud-controller-manager-operator/pull/42 - bootstrap pod implementation for AWS
- https://github.com/openshift/cluster-cloud-controller-manager-operator/pull/48 - bootstrap pod implementation for Azure
- https://github.com/openshift/cluster-cloud-controller-manager-operator/pull/45 - bootstrap render command implementation
- https://github.com/openshift/installer/pull/4947 - installer integration for bootstrap render

## Drawbacks

Requirement to separate CCM from Kubelet and KCM complicates bootstrap process. Currnent bootstrap machine components are platform independent and share common configuration. `CCM` in bootstrap requires configuration to communicate with the cloud api, making bootstrap dependent on the platform the cluster is deployed on.

## Alternatives

1. Continue development and support for in-tree cloud providers after exclusion from upstream as a carry patch.
2. Integrate external CCM with KCMO and proceed with support for new cloud providers this way, yet following described requirements for bootstrap and post install phases.

### Alternative for bootstrap changes

*Note: This is currently an alternative approach tested and confirmed functional on AWS and Azure cloud providers. Yet due to a more complicated design, it is here as an alternative to the current approach.*

Once an out-of-tree provider is released (GA), the `CCM` will be created as a static pod on the bootstrap node, to ensure swift removal of the `node.cloudprovider.kubernetes.io/uninitialized` taint from any newly created `Nodes`. Later stages, including cluster upgrades will be managed by an operator, which will ensure stability of the configuration, and will run the `CCM` in a `Deployment`.
Initial usage of the static pod is justified by the need for `CCM` to initialise `Nodes` before the `kube-scheduler` is able to schedule regular workloads (eg the operator for `CCM` managed by Deployment).

A static pod deployed on the bootstrap node will only run the `cloud-node` controller. This controller in particular, manages `Node` taint removal and all other `Node` related operations in `CCM`. Other controllers are not needed during bootstrap, and so can be excluded.

The bootstrap static pod for the cloud provider will be provisioned unconditionally on bootstrap nodes once the platform is supported by the `CCCMO` render implementation.
The constraint for enabling the `cloud-node` controller only on bootstrap will make sure this pod won't do anything for cloud providers which have not yet fully transitioned to `CCM`s, or don't have the `TechPreview` `FeatureGate` enabled from startup.

*Note: Render implementation used in bootstrap is currently a standard step for operators required to be a part of day 1 installation procedure. Our case is justified by reqirement to provision `control-plane` Nodes, and allow API server to be fully rolled-out there.*

The procedure would include the following:

1) `cluster-config-operator` render would fully populate `cloud-config` `ConfigMap` and store in a manifest on the bootstrap machine.
2) `CCCMO` would generate `CCM` manifest in form of a static pod, and provide all nessesary dependencies, such as `cloud-config` or `cloud-credentials` to use. `CCM` would have only `cloud-node` controller enabled.
3) All static pods created by render steps, including `CCM`, would be copied to static manifests folder for bootstrap `kubelet` to pick up.

`CCCMO` will provide a `render` implementation, similar to other operators, to generate initial manifests that allow deployment of `CCM` as a static pod on the bootstrap node.
This will be used with the `bootkube.sh` [script](https://github.com/openshift/installer/blob/master/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template).

At the initial stage of `CCM` installation, [installer](https://github.com/openshift/installer) creates the initial cloud-config `ConfigMap` and [cluster-config-operator](https://github.com/openshift/cluster-config-operator) could additionally populate (depending on platform) some values to the config.
Here are static contents of `cloud.conf` for OpenStack, which are generated by the installer:

Example for OpenStack:

```txt
[Global]
secret-name = openstack-credentials
secret-namespace = kube-system
```

Resources deployed on the bootstrap machine will be destroyed by the bootstrap cleanup process, after which operator provisioned CCM will take care of worker `Nodes` initialization.

## Infrastructure Needed

Additional infrastructure for OpenStack and vSphere may be required to test how CCM works with self-signed certificates. Current CI doesn't allow this.

Other platforms do not require additional infrastructure.

### New repositories

We need to forks next repositories with CCM implementations:

- [AWS](https://github.com/openshift/cloud-provider-aws) fork https://github.com/kubernetes/cloud-provider-aws
- [GCP](https://github.com/openshift/cloud-provider-gcp) fork https://github.com/kubernetess/cloud-provider-gcp
- [Azure](https://github.com/openshift/cloud-provider-azure) fork https://github.com/kubernetes-sigs/cloud-provider-azure
- [vSphere](https://github.com/openshift/cloud-provider-vsphere) fork https://github.com/kubernetes/cloud-provider-vsphere
- [Alibaba](https://github.com/openshift/cloud-provider-alibaba-cloud) fork https://github.com/kubernetes/cloud-provider-alibaba-cloud
- IBMCloud fork will be created from scratch.

OpenStack repo is already [cloned](https://github.com/openshift/cloud-provider-openstack) and the CCM image is shipped

Mandatory operator repository:

- [CCM Operator](https://github.com/openshift/cluster-cloud-controller-manager-operator)

## Additional Links

- [Kubernetes Cloud Controller Managers](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [Cloud Controller Manager Administration](https://kubernetes.io/docs/tasks/administer-cluster/running-cloud-controller/)
- [The Kubernetes Cloud Controller Manager](https://medium.com/@m.json/the-kubernetes-cloud-controller-manager-d440af0d2be5) article
https://hackmd.io/00IoVWBiSVm8mMByxerTPA#
- [CSI support](https://github.com/openshift/enhancements/blob/master/enhancements/storage/csi-driver-install.md#ocp-45-kubernetes-118)
- [CCM role in bootstrap process](https://docs.google.com/document/d/1yAczhHNJ4rDqVFFvyi7AZ27DEQdvx8DmLNbavIjrjn0)
