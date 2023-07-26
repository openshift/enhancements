---
title: shared-and-guaranteed-cpu-pools
authors:
  - "@browsell"
  - "@bwensley"
reviewers:
  - "@mrunalp"
  - "@MarSik"
  - "@yanirq"
  - "@ffromani"
  - "@jmencak"
  - "@Tal-or"
  - "@swatisehgal"
  - "@haircommander"
  - "@rphillips"
approvers:
  - "@mrunalp"
api-approvers:
  - "@mrunalp"
creation-date: 2023-06-08
last-updated: 2023-06-08
tracking-link:
  - https://issues.redhat.com/browse/CNF-8759
see-also:
  - "/enhancements/node-tuning/mixed-cpu-node-plugin.md"
---

# Shared and Guaranteed CPU Pools

## Summary

This enhancement describes an approach to split the single CPU pool currently used by kubernetes into two 
partitions (shared and guaranteed). The guaranteed partition will be used exclusively for containers with 
the guaranteed QoS that consume whole CPUs. This will allow special kernel configuration to be applied to 
this set of CPUs, to enable low latency applications (e.g. 5G RAN vDU), without affecting other applications 
running on the same server.

## Motivation

There are three workload categories on a 5G RAN vDU node:
* Workload 1: Platform services
  * These are the host services along with the OpenShift containerized platform services
  * Requirement is to isolate these services on a unique set of cores
  * This is addressed by [Management Workload Partitioning](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md) and reserved CPU partitioning
* Workload 2 - Application management and control plane pods - shared CPUs
  * These pods do not have stringent performance/latency requirements
  * Typically do not require dedicated CPUs, will request fractional CPUs with a limit or in some cases no limit
  * Burstable, BestEffort and fractional CPU Guaranteed QoS
  * These pods will be scheduled across kubelet's "defaultCpuSet"
  * "defaultCpuSet" = CPUs not used by the platform or pods with guaranteed CPUs
  * This is a dynamic cpuset that will re-size based the the creation/deletion of pods with guaranteed CPUs
* Workload 3 - Application user plane pods – L1/L2 function
  * These are the pods that carry the user plane traffic
  * These pods will use guaranteed CPUs along with additional isolation for example:
    * Disable CPU load balancing and CFS quota
    * Isolate interrupts from the allocated CPUs
    * Reduce timer ticks (nohz_full)
  * Generally a small number of pods consuming the bulk of the CPUs on the node

Although there are effectively two CPU pools from an application perspective, Kubernetes only knows about a 
single pool of CPUs (apart from the reserved CPU pool) and uses this pool for all containers. Kubernetes 
dynamically allocates CPUs from this single pool for guaranteed QoS containers (using whole CPUs) and 
updates the CPU affinity of the other containers to ensure they do not run on the guaranteed CPUs.

In order to meet the stringent latency, jitter and performance targets of workload 3, additional kernel 
tuning is required:
* Some kernel tuning can be done dynamically and applied at guaranteed container creation time.
* Unfortunately, some kernel tuning needs to be defined at boot time and is static meaning the same tuning is
applied to all application CPUs whether it is required or not.
* Some of these static tunings are problematic for workload 2, such as nohz_full and rcu callbacks. So we
need the tuning for workload 3 but it is problematic for workload 2.

This proposal provides an option for creating a separate pre-defined pool of CPUs for workload 2 (i.e. 
shared CPUs) and workload 3 (i.e guaranteed CPUs), which will allow the necessary kernel tuning to be 
applied to workload 3 without adversely affecting workload 2.

### User Stories

* As a telco service provider, I want to configure the set of CPUs running the L1/L2 functions of a vDU 
application for extremely low latency, without impacting the other application components running on the 
same server (on different CPUs).

### Goals

* Allow the configuration of separate pre-defined pools of shared and guaranteed CPUs, with different
kernel tuning applied to each pool.
* Automatically assign containers that use whole CPUs and are in a pod with the guaranteed QoS class to the
guaranteed CPUs.
* The selection of shared vs. guaranteed CPUs should be transparent to the user - no changes to the pod
spec should be required.
* The configuration should be optional and activated with a feature gate.
* Although the initial usecase for this enhancement is single-node deployments, it should also be usable
for multi-node deployments.

### Non-Goals

* This enhancement assumes the feature is activated as part of installing the cluster and cannot be
activated later. However, the sizing of the pools of shared and guaranteed CPUs can be modified (a reboot
will be required). Note: This limitation is due to the proposed use of extended resources to account for
the shared/guaranteed CPUs - if another approach is chosen, it may be possible to support activation of
the feature on an already configured cluster.
* The use of the shared and guaranteed CPU pools will be activated at the cluster level - it cannot be
configured for a subset of the nodes in a cluster. Note that different allocations of shared and
guaranteed CPUs would be possible on different nodes by having more than one PerformanceProfile and
grouping the nodes into different MachineConfigPools.

## Proposal

The [cluster-node-tuning-operator (NTO)](https://github.com/openshift/cluster-node-tuning-operator) will be
extended to add a shared CPUSet to the [Performance Profile CPU configuration](https://github.com/openshift/cluster-node-tuning-operator/blob/master/docs/performanceprofile/performance_profile.md#cpu)
in addition to the existing reserved and isolated CPUSets. For example:

```yaml
cpu:
  reserved: 0,1
  shared: 2-5
  isolated: 6-15
```
When the shared CPUSet is configured, NTO will:
* Continue to set the systemd.cpu_affinity to the reserved CPUSet.
* Apply kernel configuration for isolated CPUs (e.g. nohz_full) to only the isolated CPUSet.
* Update kubelet configuration to specify that the isolated CPUSet is to be used for guaranteed QoS
containers using whole CPUs.
* Update OpenShift Kubernetes API Server configuration to enable a new admission hook. Note: although this would
work in single-node deployments, we still need to work through how enablement will work in multi-node deployments
(waiting to decide on design approach before working that out).

The Kubelet configuration will be updated to allow a new CPUSet to be specified for guaranteed Qos containers
using whole CPUs. There are two options:
1. If this feature can be upstreamed, the [Kubelet Configuration](https://kubernetes.io/docs/reference/config-api/kubelet-config.v1beta1/) could be extended with a new `guaranteedCPUs` option.
2. If this cannot be upstreamed, a new configuration file could be created and read by kubelet on startup - similar
to the /etc/kubernetes/openshift-workload-pinning file created for The [Workload Partitioning](https://docs.openshift.com/container-platform/4.13/scalability_and_performance/ztp_far_edge/ztp-reference-cluster-configuration-for-vdu.html#ztp-sno-du-enabling-workload-partitioning_sno-configure-for-vdu) feature.

The Kubelet (mostly the [CPU manager](https://github.com/openshift/kubernetes/tree/master/pkg/kubelet/cm/cpumanager) and its static policy) would be updated to:
* Read and store the new `guaranteedCPUs` configuration.
* Remove the `guaranteedCPUs` from the `defaultCpuSet` so that shared containers are excluded from the `guaranteedCPUs` when their affinity is set.
* Extend the cpu_manager_state to include a new `guaranteedCpuSet` tracking the available CPUs.
* When a guaranteed QoS container using whole CPUs is created, allocate CPUs from the `guaranteedCpuSet` instead 
of the `defaultCpuSet` and set the CPU affinity to match.

The next piece is to let the Kubernetes Scheduler take the split shared and guaranteed CPUSets into account when
scheduling pods to a particular node. Kubernetes currently has a single 
[CPU resource](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-cpu) 
that can be specified for each container. A change is necessary to ensure the scheduler will not over-allocate 
shared or guaranteed CPUs on a node, resulting in a pod that would be scheduled but fail to run.

The solution is to introduce two new [Extended Resources](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#extended-resources):
* `openshift.io/shared-cpus`: equal to number CPUs in shared CPUSet
* `openshift.io/guaranteed-cpus`: equal to number of CPUs in isolated CPUSet

Kubelet will be updated to publish the number of millicores available for each of these resources, based on the
initial size of the `defaultCpuSet` and `guaranteedCpuSet` respectively.

The Kubernetes Scheduler will then manage the allocation of these new extended resources for each node and ensure that
the shared and guaranteed CPUs are not over-allocated. It would be unreasonable to expect the user to specify
these new extended resources for every container and to ensure that the resource matches the type of container
(shared vs. guaranteed). To automate this, an admission hook will be added to the 
[Kubernetes API Server in OpenShift](https://github.com/openshift/kubernetes/tree/master/openshift-kube-apiserver/admission), 
which will mutate each pod definition as it is created:
* For containers with guaranteed whole CPUs, add a new resource request/limit `openshift.io/guaranteed-cpus` 
equal to the number of CPUs requested in millicore units.
* For containers with non-guaranteed CPU requests (or guaranteed fractional CPU requests) add a new resource 
request/limit `openshift.io/shared-cpus` equal to the number of CPUs requested in millicore units. Note that 
one of the restrictions for extended resources is that the request/limit must both be specified and must match - in
the case of a container with differing CPU requests/limits, the CPU request value will be used - this will preserve
existing scheduler behaviour as only the CPU requests are used to choose the node a pod will run on.

Here are the CPU requests/limits from an example container that uses shared CPUs:
```yaml
requests:
  cpu: 200m
limits:
  cpu: 400m
```
This would be mutated as follows:
```yaml
requests:
  cpu: 200m
  openshift.io/shared-cpus: 200
limits:
  cpu: 400m
  openshift.io/shared-cpus: 200
```

Kubelet's cgroup creation/management code will continue to use the CPU requests/limits to configure the CFS shares/quota
for each container and will not require any modifications.

### Workflow Description

**cluster creator** is a human user responsible for deploying a cluster.

1. The cluster creator creates a Performance Profile for the NTO and specifies a shared CPU partition.
2. The cluster creator enables the new admission hook (TODO: how???). Note: This needs to be done
at the cluster level (not the node level) so can't be done by NTO. One option might be to apply a CR to update
the apiserver configuration, but that is still TBD.
3. The cluster creator then creates the cluster.
4. The NTO creates a machine config manifest to write a configuration file for kubelet to specify
the shared and isolated CPUs.
5. The kubelet starts up, finds the configuration file and initializes the `defaultCpuSet` and `guaranteedCpuSet` 
based on the shared/isolated CPUSets specified in the config file.
6. The kubelet advertises `openshift.io/shared-cpus` and `openshift.io/guaranteed-cpus` extended resources 
on the node based on the shared/isolated CPUSets specified in the config file.
7. Something schedules:
   * a pod with the `target.workload.openshift.io/management` annotation in a namespace 
with the `workload.openshift.io/allowed` management annotation. The admission hook ignores this pod as it will
be handled by the managementcpusoverride admission hook.
   * a pod with Burstable or BestEffort QoS. The admission hook modifies the pod,
adding `openshift.io/shared-cpus` requests/limits for each container, matching the CPU requests.
   * a pod with Guaranteed QoS. The admission hook modifies the pod as follows:
     * for any containers with whole CPU requests/limits it adds `openshift.io/guaranteed-cpus` requests/limits
     * for any containers with fractional CPU requests/limits it adds `openshift.io/shared-cpus` requests/limits
8. The scheduler sees the new pod and finds available `openshift.io/shared-cpus` and/or 
`openshift.io/guaranteed-cpus` resources on a node. The scheduler places the pod on the node.
9. Kubelet processes the pod as usual, but when a guaranteed QoS container using whole CPUs is created, it 
allocates CPUs from the `guaranteedCpuSet` instead of the `defaultCpuSet` and sets the CPU affinity to match.
10. Repeat steps 7-9 until all pods are running.

### API Extensions

A new admission hook in the Kubernetes API Server within OpenShift will mutate pods when they are created
to add `openshift.io/guaranteed-cpus` and/or `openshift.io/shared-cpus` requests/limits as described above.

Note that the existing [Management Workload Partitioning](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md) feature will be a dependency for this
feature. This will ensure that the API Server (and all other platform pods) with the 
`target.workload.openshift.io/management` annotation will be placed on the reserved CPU set, even if they are
started before the new admission hook is running.

### Implementation Details/Notes/Constraints

One constraint of this approach is that the feature scope is cluster wide. When the feature gate is
enabled, the new admission hook will mutate all incoming pods, adding requests/limits for the new
extended resources. This means that all nodes in the cluster must have a Performance Profile that
includes shared CPUs - without that, no pods would be scheduled on that node. Note that the isolated
CPUs partition would be optional and would only be required on nodes that were going to run
pods that had containers with whole CPU requests/limits.

### Risks and Mitigations

The current proposal will not be acceptable upstream (see Drawbacks below). Carrying these patches adds
the risk of breakages with new kubernetes versions, which would require additional time to address and
could impact release timelines.

### Drawbacks

The changes proposed to kubelet will require us to carry patches to the upstream version. The idea of
splitting the CPUs used by kubelet into a pre-defined shared and guaranteed pool might be considered
upstream, but the use of the extended resources to track these is unlikely to be accepted. The
alternative of adding a new first class resource (e.g. cpu-guaranteed) instead of extended resources
would have huge impacts to existing code and is unlikely to be accepted.

Another drawback is that the number of shared and guaranteed CPUs can only be changed with a reboot. However,
there is no alternative that avoids a reboot since much of the required kernel tuning is static and cannot
be changed at runtime.

## Design Details

### Scheduler Awareness

In the current proposal, the scheduler is aware of the new `openshift.io/shared-cpus` and
`openshift.io/guaranteed-cpus` resources and will ensure that a pod is only scheduled on a node with enough
of those resources available. However, this does not account for cases where the kubelet has a more
restrictive Topology Manager Policy (e.g. `single-numa-node` policy). In that case, it is possible that a pod
could be scheduled on a node that had enough total guaranteed CPUs (for example), but not enough guaranteed
CPUs on a single NUMA node. This would result in kubelet rejecting the pod with a Topology Affinity error.

Prior to this proposal, using the single-numa-node policy in kubelet’s Topology Manager can still result in
a Topology Affinity error when, for example, a container requests two guaranteed CPUs but the only two CPUs
available are on two different NUMA nodes. The scenario is the same with the enhancement - the Topology
Affinity error can occur if there are only two openshift.io/guaranteed-cpus available but they are on two
different NUMA nodes.

If the user wants to avoid this scenario, the solution will likely be the Topology Aware Scheduler (TAS)
which was created to address this scenario. This should work with a very small change to the kubelet - when
reporting the cpu resources available per NUMA node through the PodResources API, instead of reporting all
available CPUs, when this shared/guaranteed CPU feature is enabled, we would only report the available
guaranteed CPUs. This will ensure that the TAS will only schedule pods to nodes where there are enough
guaranteed CPUs available on the same NUMA node (when the single-numa-node policy is being used).

Note: In practice, this would only be an issue in multi-node deployments where there are pods that could
run on a selection of nodes. In a single node deployment, the user will configure the number of shared and 
guaranteed CPUs to match the workloads that they are planning on running on the node. The user would be 
aware of the NUMA restrictions imposed by using the `single-numa-node` policy and would ensure their 
configuration matched.

### Open Questions

#### Enabling Feature

How will this be enabled at the openshift apiserver level? It looks like you can create an admission hook
that is disabled by default and then enabled through a config option on the API server - could that mechanism
be used?

### Test Plan

**Note:** *Section not required until targeted at a release.*

TO DO 

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TO DO

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Enabling the feature after installation is not supported, so we do not need to address what happens if an older
cluster upgrades and then the feature is turned on.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

The new admission hook in the Kubernetes API Server within OpenShift will have the following impacts:
* When the feature is not active, there will be no impact as the hook will be disabled.
* When the feature is active, there will be a small amount of processing added only for the
admission of new pods - the limits/requests for each container will be examined and updated
with matching requests for the new extended resources.

#### Failure Modes

The new admission hook will do simple manipulations of the pod spec for new pods. A failure in the
admission hook could result in new pods not being admitted. The code will be simple though and
should be thoroughly covered with unit tests.

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

### Add new first class CPU resource

In order to support separate pre-defined pools of CPUs for workload 2 (i.e. shared CPUs) and 
workload 3 (i.e guaranteed CPUs), a new first class resource could be created. The existing
`cpu` resource would be used for shared CPUs and a new `cpu-guaranteed` resource would be used
for guaranteed CPUs.

The CPU requests/limits from an example container that used guaranteed CPUs would look like this:
```yaml
requests:
  cpu-guaranteed: 8
limits:
  cpu-guaranteed: 8
```

The NTO and kubelet changes for this option would be similar to the proposed option. Additional
kubelet changes would be required to the cgroup management code to use the cpu-guaranteed resource 
to calculate CFS shares/quotas/etc when necessary.

However, there would be significant impacts to other kubernetes components to handle the new
`cpu-guaranteed` resource in parallel to the existing `cpu` resource. This includes the apiserver,
scheduler, pod autoscaling and more.

Given that this change is being done for a very specific usecase, there is no chance the
upstream community would accept a change of this magnitude.

A variation of this alternative would be to add a new extended resource for the guaranteed CPUs 
(e.g. `openshift.io/guaranteed-cpus`) but continue to use the existing `cpu` resource for shared 
CPUs. This actually makes things worse as it would require just as many changes as would be
required to add the new first class resource, but most of the changes would have to be made
downstream and carried in patches indefinitely.

### Extend Workload Partitioning

The existing [Management Workload Partitioning](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md) 
could be extended to add support for a new `shared` partition. To place a pod in the `shared` 
partition, the user would annotate the namespace with `workload.openshift.io/allowed: shared` and 
then annotate each pod with `target.workload.openshift.io/management: {"effect": "PreferredDuringScheduling"}`. 

Here is the workflow:
1. The cluster creator creates a Performance Profile for the NTO and specifies a shared CPU partition.
2. The cluster creator then creates the cluster.
3. The NTO creates a machine config manifest to configure CRI-O to partition shared workloads.
4. The NTO creates a machine config manifest to write a configuration file for kubelet to specify
the shared CPUs.
5. The kubelet starts up, finds the configuration file and removes the shared CPUs from the `defaultCpuSet`,
adding them to the `reservedSystemCPUs` instead. 
6. The kubelet advertises `shared.workload.openshift.io/cores` extended resources on the node based on the 
number of shared CPUs from the config file.
7. Something schedules a pod with the `target.workload.openshift.io/shared` annotation in a namespace 
with the `workload.openshift.io/allowed: shared` annotation. 
   * The admission hook modifies the containers in the pod:
     * replacing the `cpu` requests with `shared.workload.openshift.io/cores` requests/limits
     * adding the `resources.workload.openshift.io/{container-name}: {"cpushares": <shares>, "cpulimits": <limits>}`
   annotation for CRI-O.
   * The scheduler sees the new pod and finds available `shared.workload.openshift.io/cores` resources on the node. 
   The scheduler places the pod on the node.
   * Kubelet does not do the cgroup configuration for the containers since it is a shared pod (based on the annotation).
   * CRI-O does the cgroup configuration for the containers in the pod based on the shares/limits annotated 
   against each container. Note: this is the same workflow currently used for guaranteed containers.
8. Something schedules a pod without the new `shared` annotations.
   * The pod is scheduled as usual based on the `cpu` requests.
   * Kubelet processes the pod as usual, allocating CPUs from the `defaultCpuSet`.
9. Repeat steps 7-8 until all pods are running.

This approach has several drawbacks:
* It requires all non-guaranteed QoS application pods to be annotated, which is going to be painful for users, 
especially when running third party components (e.g. operators).
* It requires the shared CPUs to be “hidden” in Kubelet's reserved CPU set (`reservedSystemCPUs`) to ensure kubelet 
doesn’t use these CPUs for guaranteed containers. This is an abuse of the intended use of the reserved CPU set
and could lead to future conflicts with Kubelet changes in this area.
* Some shared pods will have the Guaranteed QoS (because all `cpu` limits/requests match), but the containers in
the pod do not use whole CPUs. These containers would have their `cpu` resource removed, so the existing 
[QoS calculations](https://github.com/openshift/kubernetes/blob/master/pkg/apis/core/helper/qos/qos.go) would 
no longer work, which causes issues for various kubernetes components (e.g. evictions). One option would be 
to keep a very small (i.e. 2mc) `cpu` request/limit for these containers (done with the admission hook) - this 
would satisfy the QoS calculation, but is extremely hacky and likely to break other things.

This approach also has larger code impacts than the chosen solution - impacting the existing Workload Partioning code
and requiring CRI-O changes.

### Detached CPU pool

This option would allow separate pool(s) of CPUs to be configured to be used as shared and/or guaranteed CPUs.
Kubelet (and CPU Manager) would be configured to ignore these CPUs completely - they would be managed
exclusively by external components like a [Device Plugin](https://kubernetes.io/docs/concepts/extend-kubernetes/compute-storage-net/device-plugins/)
and [Node Resource Interface](https://github.com/containerd/nri). New extended resources would be published for
these pools and requested by containers that want to allocate CPUs from these pools. This is essentially the
solution implemented by the [Nokia CPU Pooler for Kubernetes](https://github.com/nokia/CPU-Pooler).

This approach has several drawbacks:
* It requires all containers using the new CPU pool(s) to be annotated, which is going to be painful for users, 
especially when running third party components (e.g. operators).
* Containers using the new CPU pool(s) will no longer have `cpu` requests, which breaks the existing QoS 
calculations (see above for implications).
* Since the new CPU pools are no longer managed by kubelet and CPU Manager, we lose existing features like
NUMA alignment and hyperthreading support, along with all the cgroup configuration. These features need to 
be re-implemented in the new component resulting in extra complexity and duplication.

This approach also has much larger code impact than the chosen solution.

### New Scheduler Plugin

This option would create a new [scheduler plugin](https://github.com/kubernetes-sigs/scheduler-plugins) that
would decide whether to admit each pod to a node based on the number of shared/guaranted CPUs in use on the
node and how many are required by the new pod. This could be done in a
[FilterPlugin](https://pkg.go.dev/k8s.io/kubernetes/pkg/scheduler/framework#FilterPlugin). The plugin would
be able to do these calculations purely based on the QoS of the pod and the `cpu` requests/limits for each
container.

The question would then be how to publish the number of shared/guaranteed CPUs available on each node:
* Using extended resources would no longer make sense, because pods would no longer be mutated to add
requests/limits for these resources. The user would no longer have an indication of how much of these
extended resources were in use (e.g. with the "oc describe node" command).
* A simple option would be to have kubelet add annotations to each node (instead of publishing extended
resources). The scheduler plugin could then use those annotations to know what was available on each
node. Something like:
  * openshift.io/shared-cpus: x
  * openshift.io/guaranteed-cpus: y

The next question would be how to show the user how many shared/guaranteed CPUs are in use on each node.
Without using extended resources, I can think of a couple options:
* We could add a new "oc" command to display the shared/guaranteed CPUs configured (by looking at the
annotations on each node) and the shared/guaranteed CPUs in use (by looking at the pods on each node and
their QoS class).
* We could patch the existing "oc describe node" command to add in this information (calculated in the
same way). But this feels ugly and it doesn't look like we actually patch the kubectl code today.

It doesn't feel like either of these options for showing the shared/guaranteed CPUs is going to be
acceptable. Additionally, this solution suffers from the Scheduler Awareness issue described in the
Open Questions section above.

### Enhance Topology Aware Scheduler

This option would enhance the [Topology Aware Scheduler](https://github.com/k8stopologyawareschedwg) (TAS) and
[Resource Topology Exporter](https://github.com/k8stopologyawareschedwg/resource-topology-exporter) (RTE) /
[Node Feature Discovery](https://github.com/k8stopologyawareschedwg/node-feature-discovery) (NFD) to support
shared/guaranteed CPUs.

At a high level:
* The [Node Resource Topology](https://github.com/k8stopologyawareschedwg/noderesourcetopology-api)
could be extended to track the shared/guaranteed CPUs available/allocated on each NUMA node.
* The TAS would be extended to make scheduling decisions based on the QoS of the pod and the `cpu`
requests/limits for each container.

However, tracking shared CPUs in the NRT doesn't make sense - shared CPUs are not NUMA node specific.
Containers using shared CPUs float across all the shared CPUs - shared CPUs are not allocated to specific
containers. This doesn’t map well to the NRT and the role of the TAS (the TAS is not involved in
scheduling decisions for shared CPUs).

The NRT could be updated to only track the guaranteed CPUs available on each NUMA node (instead of all
CPUs available) and that would allow the TAS to only schedule pods to workers where there are enough
guaranteed CPUs available on the same NUMA node (when the single-numa-node policy is being used). However,
that doesn't solve the problem of ensuring shared CPUs are not oversubscribed on a worker and doesn't
provide the end user with visibility into the number of shared CPUs available at any point in time.

Another concern with any solution that relies on TAS/NFD/RTE is that TAS/NFD/RTE can’t be deployed on
some SNO configurations (e.g. vRAN DU) due to resource constraints. We need a solution that can be used
in those configurations as well as larger configurations.

## Infrastructure Needed [optional]

N/A
