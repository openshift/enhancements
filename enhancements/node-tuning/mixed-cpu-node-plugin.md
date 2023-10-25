---
title: mixed-cpu-node-plugin
authors:
  - "@Tal-or"
reviewers: 
  - "@MarSik"
  - "@yanirq"
  - "@ffromani"
  - "@jmencak"
  - "@mrunalp"
  - "@haircommander"
approvers: 
  - "@mrunalp"
  - "@rphillips"
api-approvers:
  - "@deads2k"
  - "@JoelSpeed"
creation-date: 2023-05-03
last-updated: 2023-06-14
tracking-link: 
  - https://issues.redhat.com/browse/CNF-7603
  - https://github.com/openshift/enhancements/pull/1421
  - https://github.com/openshift-kni/mixed-cpu-node-plugin
---

# mixed-cpu-node-plugin


## Summary

Resources management (particularly CPUs) in Kubernetes/OpenShift is limited and not flexible enough to cover all of
our customer use cases.
This enhancement introduces a runtime-level approach 
for extending CPU resources management on top of Kubernetes and OpenShift platforms.
With the existing CPU management design, a container can either request exclusive CPUs or shared CPUs,
while with this feature, it would be possible for container workload to request for both.

## Motivation

There is a growing interest from telco users in Kubernetes/OpenShift as a cloud management platform.
This proposal is motivated by the desire of optimizing the platform's capabilities for telco users,
such as cost reduction, power efficiency, higher pod density and sustainability.

The telco users have new use cases and requirements that especially related to resource management of the platform. 
In order to address some of those needs,
Kubelet CPU manager was created to allow users requesting for exclusive, isolated cpus.

But there are additional use cases that require more granular control of CPU management.
This enhancement aims to solve the following use case:
When the CPU manager allocates an exclusive set of CPUs for a given workloads, the workload shall still have the option 
to specify and access a set of shared CPUs, whether those shared CPUs are shared among containers in the 
same pod or containers in different ones.

By giving the users the flexibility to use exclusive CPUs only when it is an absolute necessity, we help them utilize
their existing compute resources in an optimal manner.

### User Stories

* As a developer who's building DPDK application,
I want to run my application in a container,
giving full exclusive access to cpu cores for the busy-loops polling threads,
but having the light-weight tasks like configuration processing and log printing not taking full cores,
in order to improve the application density and make more efficient use of the CPUs.
For that, my application container needs access to both shared and exclusive CPUs.

  
### Goals 
* Increase pod density, power efficiency, cost reduction and sustainability by optimizing CPUs resource management. 
* Provide a special-purpose at a runtime-level solution for containers in a pod to have the ability
to request and subsequently be allocated both exclusive and shared CPUs.
* Enable the telco-specific use case with a self-contained change,
    minimizing the changes to the platform

### Non-Goals
* Introducing a generic mechanism in the platform that does involve Kubelet and pod spec changes.

## Proposal

#### node-plugin implementation stages

To minimize the moving part and keep this feature as stable as possible
as an initial stage, the node-plugin would be implemented in CRI-O as a CRI-O hook
(similar to the existing performance hooks).

As a second stage, the plugin should be exported into a separate component, decoupled from CRI-O,
and be running as a separate
[NRI-plugin](https://github.com/containerd/nri).

#### plugin workflow

Container processes are restricted
to given [cpuset](https://man7.org/linux/man-pages/man7/cpuset.7.html) by their corresponding cgroup definition.
The kernel restricts the processes of a container to run at the CPU (core) ids specified under its
cgroup.

The plugin appends the shared cpus under the cgroup's `cpuset.cpus`.
Next, it modifies the following cgroup settings:
1. It increases the container's `cpu.cfs_quota_us` as a multiplication of shared cpus number and `cfs_period_us`.
2. It repeats step 1 for the container's parent (pod) cgroup `cpu.cfs_quota_us`.

Let's have a look at numeric example:
`cpu.shared` = `3,4`
Container has 2 exclusive cpus id: `5,6`
`cfs_period_us` = `100,000`
`cpu.cfs_quota_us` = `200,000` (number of exclusive cpus * `cfs_period_us`)

The plugin expands the [CPUs](https://github.com/containerd/nri/blob/main/pkg/api/api.pb.go#L2105)
to include the shared CPUs so the final CPU set looks like: `3,4,5,6`.
It also increases the container's `cpu.cfs_quota_us` from `200,000` to `400,000` according to the following formula:
`cpu.cfs_quota_us` = number_of(`cpu.shared`) * `cfs_period_us` + `cpu.cfs_quota_us`

It modifies the cgroup `kubepods.slice/kubepods-pod<pod-id>/crio-<container-id>.scope/cpu.cfs_quota_us` value
and `kubepods.slice/kubepods-pod<pod-id>/cpu.cfs_quota_us` value using the same formula.

For cgroup v2 the same changes apply but with the respective to the API changes in v2, For example,
cgroup v1 cpu.cfs_quota_us path:
`/sys/fs/cgroup/cpu,cpuacct/kubepods.slice/kubepods-pod<pod-id>/crio-<container-id>.scope/cpu.cfs_quota_us`
changes to:
`/sys/fs/cgroup/kubepods.slice/kubepods-pod<pod-id>/crio-<container-id>.scope/cpu.max`
The cpuset changes stays the same. 

#### Shared CPUs 
The Shared CPUs are configured via a [performance profile](https://github.com/openshift/cluster-node-tuning-operator/blob/master/docs/performanceprofile/performance_controller.md#performanceprofile) 
and are added to Kubelet [reservedSystemCpus](https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#explicitly-reserved-cpu-list).
With this addition, Kubelet's `reservedSystemCpus` will be composed of PerformanceProfile's 
`spec.cpu.reserved` + `spec.cpu.shared`.
We utilize Kubelet `reservedSystemCpus` because of CPU manager, which is not aware of the CPUs
lying in this pool, so it doesn't undo or changes the allocation logic performed by the plugin.
Therefore, the plugin assigns the shared CPUs to containers without racing/conflict with
the CPU manager behavior. 
More about this decision at the [Alternative](mixed-cpu-node-plugin.md#alternatives) section

When [management and workload partitioning](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md) is enabled,
the `reservedSystemCpus` are exclusive to management components (Kubelet/CRI-O) or pods that are labeled as management. 
With this proposal system reserved is no longer mapped to Kubelet's view of a system reserved.
Instead, there is an internal partition of `resrvedSystemCpus` in which the `reserved` cpus are dedicated
and exclusive to a management component ([same as today](https://github.com/openshift/cluster-node-tuning-operator/blob/master/pkg/performanceprofile/controller/performanceprofile/components/machineconfig/machineconfig.go#L592)).
While the `shared` cpus are dedicated for workloads which are asking for shared cpus.

The following shows the cpu layout and what processes can be running at what cpus:
![cpu layout](cpu_layout.jpg)
Please check the [Risks and Mitigations](mixed-cpu-node-plugin.md#risks-and-mitigations) for additional information.

There is a plan for a feature called [shared-partition](https://github.com/openshift/enhancements/pull/1421) 
that implements additional pool.
When shared-partition lands, Kubelet's cpu pool layout should look roughly like:
1. `reservedSystemCpus`
2. `guaranteedCpus` <- new pool
3. `sharedCpus` = All CPUs - `reservedSystemCpus` - `guaranteedCpus`

When shared-partition feature enabled, 
mixed-cpu-node-plugin allocates the shared cpus from the `sharedCpus` pool and not
from the `reservedSystemCpus` anymore.

The following table shows the shared-cpus origin in each flow:

| mixed-cpu | shared-partition | shared cpus origin                 |
|-----------|------------------|------------------------------------|
| Enable    | Disable          | Kubelet's `reservedSystemCPUs`     | 
| Disable   | Enable           | All cpus - `reserved` - `isolated` | 
| Enable    | Enable           | All cpus - `reserved` - `isolated` | 

#### Workload configuration
A workload that wants to access the shared cpu should request for `openshift.io/enabled-shared-cpus` under its spec.
The `openshift.io/enabled-shared-cpus` should be treated as a boolean value.
In other words, the resource request only uses as a hint that shared-cpus required for the container and
does not indicate any actual value.

For example:
```yaml
requests:
   openshift.io/enabled-shared-cpus: 1
```
Fine.

```yaml
requests:
   openshift.io/enabled-shared-cpus: 2
```
Wrong. An error will be returned to the user explaining how to fix the pod spec.

The reason for specifying a resource request, and not an annotation,
is to allow control on the number of workloads that can request for shared cpus.
By populating shared cpu resources as the number of maximum workloads.
If the number of shared cpu requests got exhausted, any new workload that requests for shared cpus request will move
to pending state by the scheduler.

Once workload request completed successfully it gets access to shared cpus, and two new environment variables
will be present under the container's environment variables:
`OPENSHIFT_SHARED_CPUS=<CPU-IDs>` = specifies the core ids of the shared cpus
`OPENSHIFT_ISOLATED_CPUS=<CPU-IDs>` = specifies the core ids of the isolated cpus
Those environment variables help the application's user to pin its processes/threads to the desired CPU set. 

#### Kubelet
Kubelet needs to be updated to advertise the `openshift.io/enabled-shared-cpus` resources 
as [Extended Resources](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#extended-resources).
Kubelet will look for a configuration file `/etc/kubernetes/openshift-shared-cpus`, to enable the resource advertisement.
The configuration file should be look like this (the number of resources might be varied)
``` json
{
  "shared-cpus": {
     "containersLimit": "15"
  }
}
```
`containersLimit` limit the number of containers that can access the shared cpus in parallel.
At first, this number will be static, and based on user feedback, we can make it configurable.

#### API Server Admission Hook
A new admission hook for [OpenShift Kubernetes API Server](https://github.com/openshift/kubernetes/tree/master/openshift-kube-apiserver/admission)
will be added for handling the following:
1. In case a user specifies more than a single `openshift.io/enabled-shared-cpus` resource, it rejects the pod request with an error explaining the user how to fix its pod spec.
2. It adds an annotation `cpu-shared.crio.io` that will be used to tell the runtime that a shared cpus were requested.
For every container requested for shared cpus, it adds and annotation with the following scheme:
`cpu-shared.crio.io/<container name>`
In addition, the `cpu-shared.crio.io` annotation needs 
to be added under the performance-runtime [allowed_annotation](https://github.com/openshift/cluster-node-tuning-operator/blob/master/assets/performanceprofile/configs/99-runtimes.conf#L20)   

#### CRI-O
CRI-O should update with a new performance hook to support the shared-cpu logic.
CRI-O should update with the configuration of the shared cpus.
```toml
[crio.runtime]
shared_cpuset = "0-1,50,51"
```

#### Cluster-Node-Tuning-Operator 
cluster-node-tuning-operator (NTO) will be modified to add new configuration settings to Kubelet and CRI-O and
update Kubelet's `resevedSystemCPUs`.

To activate NRI in container-runtime, NTO creates MachineConfig to enable NRI in 
the container-runtime configuration:
```yaml
[crio.nri]
enable_nri = true
```
NOTE: NRI enablement is not needed for the [first stage](mixed-cpu-node-plugin.md#node-plugin-implementation-stages)

### API
Extend PerformanceProfile with new `workloadHints` named `mixedCpus` in order to enable
the feature.

```yaml
workloadHints:
  mixedCpus: true
```

Specify the shared cpus under `cpus.shared`:

```yaml
cpu:
  shared: "2,3"
```

The CPU set value should follow the 
[cpuset](https://github.com/openshift/cluster-node-tuning-operator/blob/master/docs/performanceprofile/performance_profile.md#cpuset) conventions.
It defines a set of CPUs that can be allocated to guaranteed pods/containers that still require
non-exclusive ones.

The CPU set must not overlap with `spec.cpu.reserved` or `spec.cpu.isolated` and NTO
should return an error in case it does.

The reason for having a separate flag for enabling the feature, 
is because there is a coming feature named [shared-partition](https://github.com/openshift/enhancements/pull/1421)
that uses the `cpu.shared` value as well.
The features must be enabled independently, hence the `workloadHints` flag.

Both workloadHints and `cpu.shared` have to be specified to activate the feature.

The feature is optional and off by default.
The feature can be activated/deactivated on a running system.

PerformanceProfile example:
```yaml
apiVersion: performance.openshift.io/v2
kind: PerformanceProfile
metadata:
  name: example-performance-profile
spec:
  cpu:
    reserved: "0-1"
    isolated: "4-8"
    shared: "2-3"
  hugepages:
    defaultHugepagesSize: "1G"
    pages:
      - size: "1G"
        count: 2
        node: 0
  realTimeKernel:
    enabled: true
  workloadHints:
    mixedCpus: true
  nodeSelector:
    node-role.kubernetes.io/performance: "test"
```

Specify the `cpu.shared` and `mixedCpus: true` under `workloadHints` activates the feature,
signals NTO to update the `reservedSystemCpus` in Kubelet config,
reboots the nodes that are associated with the updated PerformanceProfile,
and deploys the mixed-cpu-node-plugin.
The logic of updating reserved-cpus pool is NTO's responsibility and would be implemented as part of this feature.

Remove the `cpu.shared` value,
or `mixedCpus: true` from `workloadHints` reverts the changes from Kubelet `reservedSystemCpus`,
reboots the nodes that are associated with the updated PerformanceProfile,
and removes the configuration files from Kubelet and CRI-O.

### Workflow Description

The premise is that OCP cluster is running, and there's an active performance-profile that already tuned the system.

1. The cluster administrator wants to support both exclusive and shared cpus for workloads running on the cluster.
2. The cluster administrator specifies the shared CPU ids in the PerformanceProfile `spec.cpu.shared` and toggels `mixedCpus: true` under the `WorkloadHints`.
3. The cluster administrator waits for MCO to kick in, update and reboot the nodes.
4. The cluster administrator waits for the node to come back from reboot.
5. The application administrator wants to deploy their DPDK application as a Guaranteed pod with shared CPUs.
6. The application administrator specifies a request for `openshift.io/enabled-shared-cpus: 1` under the pod's `spec.containers[].resources.requests`.
7. The application administrator is waiting for the DPDK pod to be `Running`.
8. The DPDK's app user/developer wants to run a light-weight task (threads) on shared cpus.
9. The DPDK's app user/developer should pin the light-weight threads to CPUs that have shown in the `OPENSHIFT_SHARED_CPUS` environment variable of the container's process.
10. The DPDK's app user/developer should pin the heavy-weight threads to CPUs that have shown in the `OPENSHIFT_ISOLATED_CPUS` environment variable of the container's process.

### API Extensions
A new admission hook in the Kubernetes API Server within OpenShift will 
mutate pod spec if more than a single `openshift.io/enabled-shared-cpus` was requested 
and annotate the pod with `cpu-shared.crio.io` annotation as describe at API Server Admission Hook [section](#api-server-admission-hook)

### Risks and Mitigations
#### Processes Boundaries 
* With this solution, the platform's housekeeping processes also runs on the shared CPUs.
but the intent is to allocate those cpus to workload’s light-weight tasks,
so having some latency is bearable.
A way of mitigating that is to use workload partitioning
to ensure the platform housekeeping processes don't run on the shared cpus.

* There is no risk that workload’s tasks will run on dedicated OCP’s housekeeping reserved cpus,
because only the shared cpus exposed via cgroups to the workload’s process.
In other words, platform's housekeeping processes can run on shared cpus dedicated to workload’s light-weight tasks,
but not the other way around.

#### cgroup v1 vs v2 considerations
This feature supports changes to cgroup configuration for pods or containers, at both v1 and v2.
In containers that are annotated with cpu load-balancing disabling and are asking for shared cpus, only the isolated cpus (guaranteed)
would have cpu load-balancing disabled.

### Drawbacks
The way of how user should ask for shared CPUs in the pod spec is through device request.
While this approach fits in the model of how the workload requires resources,
better integration would require more invasive changes which are out of scope now.
    
## Design Details
N/A

### Enabling Feature
A new feature gate will be defined for this feature (e.g. `MixedCPUAllocation`).

Multiple components affected by this feature:
* Cluster-Node-Tuning-Operator
* Kubelet
* Openshift Admission Webhook
* CRI-O

All code changes won't take effect when a feature gate is not enabled or the feature has not been activated.

### Test Plan
The node-plugin testing will be split into two phases:
1. Functionally - Tests that focus on the plugin's business logic.
Validation of cgroups settings, robustness (Kubelet restart/node reboot), scalability (In-place Resource Resizes).
Validate new API for workloads.

2. Deployment - Tests that focus on plugin deployment via NTO.
Verifying new API for PerformanceProfile, configuration files applied successfully,
Feature Gate enablement, run tests from the previous section while deployment done via NTO.

#### Unit testing
We will add unit-testing for the different components (especially CRI-O) to guarantee the basic functionality.

#### E2E testing
Validating a deployment process on NTO, verifying PerformanceProfile configuration and checking 
the functionality of the feature (running the e2e tests from mixed-cpu-node-plugin) 

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
N/A

## Alternatives
* Taint the nodes with `NoSchedule`, `NoExecute` in order to keep the node from scheduling/run on nodes when
the plugin is not ready (critical for reboot scenarios). 
This replaces the `openshift.io/enabled-shared-cpu` request and eliminate the need in device plugin API.

Pros:
1. Using an annotation for shared-cpus request which is more descriptive than the `openshift.io/enabled-shared-cpu` request
which is fitter for counted resources while here it's a static request.
2. simplifies the node-plugin by removing the device plugin implementation part.

Cons:
1. All infra pods would require toleration that matches the taint.
2. A mutation webhook should be added in order to add the toleration for the workloads.
(we can guide the user to add that manually together with the annotation, but webhook is preferred for 
minimizing human errors). 
Such a webhook makes this feature vendor lock-in
3. There should be a component that makes sure to add/remove the relevant taints from the node. (NTO?)
 
* Suggest a change to Kubelet CPU manager to support a static shared-cpu pool (not dynamic as of today).
A change such as this requires changes to Kubelet, scheduler and other supporting controllers as 
eviction manager, HPA (horizontal pod autoscaling), etc.
Considering the u/s velocity, current deadlines, and the number of open questions that have to be addressed,
the plugin solution has a bigger chance to be completed on time.

* Another option is a [KEP](https://github.com/kubernetes/enhancements/pull/3853) proposed u/s
that tries to provide more general solution by having a pluggable resource manager,
so users can implement their custom resource management behavior. 
While this KEP is still under discussion in the community, it might take quite some time to
reach maturity, which at this point in time cannot address our short-term needs.
