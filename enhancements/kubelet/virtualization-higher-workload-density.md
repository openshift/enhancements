---
title: virtualization-higher-workload-density
authors:
  - "@enp0s3"
  - "@iholder101"
  - "@fabiand"
reviewers:
  - "@rphilips"
  - "@sjenning"
approvers:
  - "@mrunalp"
api-approvers:
  - "@mrunalp"
creation-date: "2024-05-05"
last-updated: "2024-06-12"
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/CNV-28178"
see-also:
  - "https://github.com/kubernetes/enhancements/issues/2400"
status: implementable
---

# Virtualization higher workload density

## Summary

Fit more workloads onto a given node - achieve a higher workload
density - by overcommitting its memory resources. Due to timeline
needs a multi-phased approach is considered.

## Motivation

Today, OpenShift Virtualization is reserving memory (`requests.memory`)
according to the needs of the virtual machine and its infrastructure
(the VM related pod). However, usually an application within the virtual
machine does not utilize _all_ the memory _all_ the time. Instead,
only _sometimes_ there are memory spikes within the virtual machine.
And usually this is also true for the infrastucture part (the pod) of a VM:
Not all the memory is used all the time. Because of this assumption, in
the following we are not differentiating between the guest of a VM and the
infrastructure of a VM, instead we are just speaking collectively of a VM.

Now - Extrapolating this behavior from one to all virtual machines on a
given node leads to the observation that _on average_ much of the reserved memory is not utilized.
Moreover, the memory pages that are utilized can be classified to a frequently used (a.k.a working set) and inactive memory pages.
In case of memory pressure the inactive memory pages can be swapped out. 
From the cluster owner perspective, reserved but underutilized hardware resources - like memory in this case -
are a cost factor.

This proposal is about increasing the virtual machine density and thus
memory utilization per node, in order to reduce the cost per virtual machine.

> [!NOTE]
> CPU resources are already over-committed in OpenShift today, thus the
> limiting factor - usually - are memory resources.

### User Stories

* As an administrator, I want to be able to enable higher workload
  density for OpenShift Virtualization clusters so that I can increase
  the memory utilization.

### Goals

#### Functionality

* Fit more virtual machines onto a node once higher workload density
  is enabled
* Integrate well with [KSM] and [FPR]
* Protect the node from resource starvation upon high swapping activity.        

#### Usability

* Provide a boolean in the OpenShift Console for enabling higher-density.
  Do not require any additional workload or cluster level configuration
  besides the initial provisioning.
* Provide a method to deploy the requried configuration for safer
  memory overcommit, such as swap provisioning, tuning of system services, etc.

### Non-Goals

* Complete life-cycling of the [WASP](https://github.com/OpenShift-virtualization/wasp-agent) Agent. We are not intending to write
  an Operator for memory over commit for two reasons:
  * [Kubernetes SWAP] is close, writing a fully fledged operator seems
    to be no good use of developer resources
  * To simplify the transition from WASP to [Kubernetes SWAP]
* Allow swapping for VM pods only. We don't want to diverge from the upstream approach 
  since [Kubernetes SWAP] allows swapping for all pods associated with the burtsable QoS class.

## Proposal

A higher workload density is achieved by combining two mechanisms

1. Under-request memory resources for virtual machines by having the VM
   pods (`launcher`) `requests.memory` being smaller than the VMs `guest.memory`
   according to a configured ratio[^1]. This is owned by
   [KubeVirt and present today](https://kubevirt.io/user-guide/operations/node_overcommit/).
2. Compensate for the memory over-committment by using SWAP in order
   to extend the virtual memory. This is owned by the platform and not
   available today.

#### Applicability

We expect to mitigate the following situations

* Underutilized guest memory (as described above)
* Infrastructure related memory spikes in an over-committed environment
* Considerable amount of "cold memory allocations" - thus allocations
  which are rarely used

#### Scope

Following the upstream kubernetes approach every workload marked as burstable QoS would be able to swap.
There is no differentiation between the type of the workload: regular pod or a VM.
With that being said, swapping will be allowed by WASP (and later on by kube swap) for pods
with Burstable QoS class.

Among the VM workloads, VMs of high-performance configuration (NUMA affinity, CPU affinity, etc.) cannot overcommit.
Also, VMs with best-effort QoS class don't exist because requesting memory is mandatory for the VM spec. 
For VMs of Burstable Qos Class over-commited VM stability can be achieved during memory spikes by swapping out "cold" memory pages.

#### Timeline & Phases

| Phase                                                              | Target       | WASP Graduation   | Kube swap Graduation | Machine-level configuration deployment |
|--------------------------------------------------------------------|--------------|-------------------|----------------------|----------------------------------------|
| Phase 1 - Out-Of-Tree SWAP with WASP TP                            | mid 2024     | Tech-Preview      | Beta                 | Manual                                 |
| Phase 2 - WASP GA with swap-based evictions                        | end 2024     | GA                | Beta                 | Manual                                 |
| Phase 3 - Transition to Kubernetes SWAP. Limited to CNV-only users | mid-end 2025 | Start Deprecation | GA                   | Manual                                 |
| Phase 4 - Kubernetes SWAP for all OpenShift users                  | TBD          | Removed           | GA                   | Operator                               |

Because [Kubernetes SWAP] is currently in Beta and is only expected to GA within
Kubernetes releases 1.33-1.35 (discussion about its GA criterias is still ongoing).
this proposal is taking a four-phased approach in order to meet the timeline requirements.

* **Phase 1** - OpenShift Virtualization will provide an out-of-tree
  solution (WASP) to enable higher workload density and swap-based eviction mechanisms.
* **Phase 2** - WASP will include swap-based eviction mechanisms.
* **Phase 3** - OpenShift Virtualization will transition to [Kubernetes SWAP] (in-tree).
  OpenShift will [allow](#swap-ga-for-cnv-users-only) SWAP to be configured only if OpenShift Virtualization is installed on the cluster.
  In this phase, WASP deprecation will start.
* **Phase 4** - OpenShift will GA SWAP for every user, even if OpenShift Virtualization
  is not installed on the cluster. In this phase WASP will be removed, machine-level configuration
  will be managed by Machine Config Operator.

### Workflow Description

**cluster administrator** (Cluster Admin) is a human user responsible
for deploying and administering a cluster.

**virtual machine owner** (VM Owner) is a human user operating a
virtual machine in a cluster.

#### Workflow: Configuring higher workload density

1. The cluster admin is deploying a bare-metal OpenShift cluster
2. The cluster admin is deploying the OpenShift Virtualization Operator
3. The cluster admin is deploying the WASP Agent according to the
   [documentation](https://github.com/openshift-virtualization/wasp-agent/blob/main/docs/configuration.md)
4. The cluster admin is configuring OpenShift Virtualization for higher
   workload density via:

   a. the OpenShift Virtualization Console "Settings" page 
   b. [or `HCO` API](https://github.com/kubevirt/hyperconverged-cluster-operator/blob/main/docs/cluster-configuration.md#configure-higher-workload-density)

The cluster is now set up for higher workload density.

In phase 3, deploying the WASP agent will not be needed.

#### Workflow: Leveraging higher workload density

1. The VM Owner is creating a regular virtual machine and is launching it. The VM owner must not specify memory requests in the VM spec, but only the guest memory size.

### API Extensions

#### Phase 3
In phase 3 WASP will be replaced by kubernetes swap, which should be GA by then. With that being said it's requried to update the worker kubelet configuration with `memorySwap.swapBehavior`.
Following is a KubeletConfig example:
```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: custom-config
spec:
  machineConfigPoolSelector:
    matchLabels:
      custom-kubelet: enabled
  kubeletConfig:
    failSwapOn: false
    memorySwap:
      swapBehavior: LimitedSwap
```
More info about kubelet swap API can be found [here](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/2400-node-swap/README.md#api-changes)

#### Phase 4
In previous phases the machine-level confgiuration was deployed manually using machine configs. The transition to phase 4
will require re-deployment of the configuration by the operator.


### Topology Considerations

#### Hosted Control Planes: Hosting cluster with OCP Virt
No special consideration required since it's identical to the regular cluster case

#### Hosted Control Planes: Hosted cluster with OCP Virt
Since nested virt is not supported, the only topology that is supported is when the data plane resides on a bare metal.
In that case the abovementioned `KubeletCofnig` and `MachineConfig` should be injected into the `NodePool` CR.

#### Standalone Clusters

Standalone and regular clusters are the primary use-cases for
swap.

#### All-in-one deployments

Single-node Openshift (SNO), MicroShift and Compact Cluster deployments are out of scope of this
proposal, since enabling swap on control-plane nodes is not supported.

### Implementation Details/Notes/Constraints

##### Design

The design is driven by the following guiding principles:

* System services are more important than workloads. Because workload
  health depends on the health of system services.
* Try to stay aligned to upstream Kubernetes swap behavior in order
  to ease the transition to Kubernetes SWAP once available

###### WASP Agent

At its core the [wasp agent] is a `DaemonSet` delivering an [OCI Hook]
which is used to turn on swap for selected workloads.

Overall the agent is intended to align - and specifically not
conflict - with the upstream Kubernetes SWAP design and behavior in
order to simplify a transition. 

The agent uses an OCI Hook to enable swap via container cgroup. The hook script 
sets the containers cgroup `memory.swap.max=max`.

* **Tech Preview**
  * Limited to burstable QoS class pods.
  * Uses `UnlimitedSwap` approach. Using OCI hook each container is started with `memory.swap.max=max` in its cgroup.
* **General Availability**
  * Container starts with `UnlimitedSwap` as in TP, then reconciled to `LimitedSwap` by the wasp-agent limited swap controller.
    * For more info, refer to the upstream documentation on how to calculate [limited swap](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/2400-node-swap#steps-to-calculate-swap-limit). 
  * Added Limitation to non-high-priority pods.
  * Added swap-based evictions.
  * Added critical workload protection (same as kube swap).

![wasp](https://github.com/user-attachments/assets/314d387d-788a-4aa6-9275-c882f5f329aa)

**NOTE**: In WASP GA setting the swap limit isn't atomic, it can happen that a container application will start
to allocate memory before the limit is set by the wasp-agent.

###### Machine-level swap configuration

* Provisioning swap

Provisioning of swap is left to the cluster administrator.
The OCI hook itself is not making any assumption where the swap is located.

As long as there is no additional tooling available, the recommendation
is to use `MachineConfig` objects to provision swap on nodes.

The `MachineConfig` object would include the necessary scripts
in order to provision swap on a disk, partition, or in a file.

* Node service protection

All container workloads are run in the `kubepods.slice` cgroup.
All system services are run in the `system.slice` cgroup.

By default the `system.slice` is permitted to swap, however, system
services are critical for a node's health and in turn critical for the
workloads.

Without system services such as `kubelet` or `crio`, any container will
not be able to run well.

Thus, in order to protect the `system.slice` and ensure that the nodes
infrastructure health is prioritized over workload health, Machine Config is
used to configure `memory.swap.max` to zero on the system slice parent cgroup.
The configuration is done via systemd for the sake of consistency.

* Preventing SWAP traffic I/O saturation

One risk of heavy swapping is to saturate the disk bus with swap traffic,
potentially preventing other processes from performing I/O.

In order to ensure that system services are able to perform I/O, the
machine config is configuring `io.latency=50` for the `system.slice` in order
to ensure that its I/O requests are prioritized over any other slice.
This is, because by default, no other slice is configured to have
`io.latency` set.

* Node memory pressure handling

Dealing with memory pressure on a node is differentiating the TP fom GA.

** **Technology Preview** - `memory.high` is set on the `kubepods.slice`
  in order to force the node to swap, once the `kubepods.slice` memory is
  filling up. Only once swap is full, the system will cross `memory.high`
  and trigger soft evictions.

  * Pro
    * Simple to achieve.
  * Con
    * A lot of memory pressure has to be present in order to trigger
      soft eviction.
    * Once `memory.high` is reached, the whole `kubepods.slice` is throttled
      and cannot allocate memory, which might lead to applications crashing.

** **General Availability** - Memory-based soft eviction is going to
  be disabled, in favor of enabling swap-based hard evictions, based on new
  swap traffic and swap utilization eviction metrics.

  * Pro
    * Eviction on the basis of swap pressure, not only memory pressure.
    * [LLN] applies, because all pods share the nodes memory
  * Con
    * Swap-based evictions are made through a 3rd party container, which means
      it has to be done through an API-initiated eviction.

###### Hypervisor/OS level assistance 

Swap allows us to increase the
virtual address space. This is required in order to reduce the
likelihood for workloads to OOM significantly.

Besides _increasing_ the virtual address space, there are two
mechanisms which help to reduce the memory footprint of a workload.

They are not being discussed in depth here, but are mentioned, as they
help to reduce or avoid memory pressure, which in turn leads less usage
of swap.

The mechanisms are:
* _Free Page Reporting ([FPR])_ - A mechanism to free memory on the
  hypervisor side if it is not used by the guest anymore.
* _Kernel Samepage Merging ([KSM])_ - A mechanism to merge/deduplicate
  memory pages with the same content. This is particularly helpful
  in situations where similar guests are running on a hypervisor.
  It has however some security implications.

##### VM higher-density: Differences between Technology Preview vs GA

|                              | TP                  | GA                  |
|------------------------------|---------------------|---------------------|
| SWAP Provisioning            | MachineConfig       | MachineConfig       |
| SWAP Eligibility             | burstable QoS pods  | burstable QoS pods  |
| Node service protection      | Yes                 | Yes                 |
| I/O saturation protection    | Yes                 | Yes                 |
| Critical workload protection | No                  | Yes                 |
| Memory pressure handling     | Memory based        | Memory & Swap based |

### Risks and Mitigations

#### Phases 1-2

| Risk                                       | Mitigation                                                                                               |
|--------------------------------------------|----------------------------------------------------------------------------------------------------------|
| Miss details and introduce instability     | * Adjust overcommit ratio <br/> * Tweak eviction thresholds <br/> * Use de-scheduler to balance the load |

#### Phase 3

Swap is handled by upstream Kubernetes.
Wasp is starting deprecation process.

| Risk                                                      | Mitigation                                        |
|-----------------------------------------------------------|---------------------------------------------------|
| Swap-based evictions are based on API-initiated evictions | Also rely on kubelet-level memory-based evictions |

#### Phase 4

Wasp will be removed.
Upstream Kubernetes handles both swap and evictions.
Machine-level swap confguration handled by an OpenShift operator.

### Drawbacks

The major drawback and risk of the [WASP Agent] approach in phase 1 is
due to the lack of integration with Kubernetes. Its prone to
regressions due to changes in Kubernetes.

Thus phase 2 is critical in order to eliminate those risks and
drawbacks.

## Open Questions [optional]

None.

## Test Plan

The cluster under test has worker nodes with identical amount of RAM and disk size.
Memory overcommit is configured to 200%. There should be enough free space on the disk
in order to create the required file-based swap i.e. 8G of RAM and 200% overcommit require
at least (8G + 8G*SWAP_UTILIZATION_THRESHOLD_FACTOR)  free space on the root disk.

* Fill the cluster with dormant VM's until each worker node is overcommited. 
* Test the following scenarios: 
  * Node drain
  * VM live-migration
  * Cluster upgrade. 
* The expectation is to see that nodes are stable 
as well as the workloads.


## Graduation Criteria

### Dev Preview -> Tech Preview

Dev Preview is not planned.

### Tech Preview -> GA

Specific needs
- SWAP triggered eviction for workloads

Generic needs
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [OpenShift-docs](https://github.com/OpenShift/OpenShift-docs/)

#### Swap GA for CNV users only
In addition, swap is intended to become GA only for CNV users.

This will be achieved in the following way:
- The system will figure out if CNV is enabled by checking if the `hyperconvergeds.hco.kubevirt.io`
object and the `OpenShift-cnv` namespace exist.
- If CNV is enabled, do not emit an alert.
- Otherwise, emit an alert that says this cluster has swap enabled and it is not supported.
- Guard against non CNV users setting `LimitedSwap` in the KubeletConfig via MCO validation logic.

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy


### Phases 1-2
WASP update is done manually since it doesn't have an operator. The user must follow the 
downstream documentation of the relevant CNV release and re-run the deployment steps.

On OpenShift level no specific action needed, since all of the APIs used
by the WASP agent deliverables are stable (DaemonSet, OCI Hook, MachineConfig, KubeletConfig)

### Phase 3
Since kube swap should be GAed by then, the user will be advised to switch using kube swap instead of WASP.
Configuation of kube swap should be done manually, as described in the [API Extension](#api-extensions)
Machine-level configuration deployment will remain as-is.

### Phase 4
The user will have to switch using kube swap, since WASP will be removed.
Machine-level configuration should be re-deployed by the operator.

## Version Skew Strategy

The WASP Agent OCI hook is based on a stable OCI Hook API, thus few regressions are expected.
Furthermore, we expect to go through every minor version of OpenShift, reducing skew.

## Operational Aspects of API Extensions

### Worker nodes
The amount of RAM and the disk topology must be identical on all worker nodes.

### WASP deployment
As was mentioned in the non-goals, WASP doesn't have an operator. Therefore, the deployment
of WASP should be done manually according to the downstream [documentation](https://docs.openshift.com/container-platform/4.17/virt/post_installation_configuration/virt-configuring-higher-vm-workload-density.html).

## Support Procedures

Please refer to the **Verification** section in the wasp-agent deployment [documentation](https://docs.openshift.com/container-platform/4.17/virt/post_installation_configuration/virt-configuring-higher-vm-workload-density.html#virt-using-wasp-agent-to-configure-higher-vm-workload-density_virt-configuring-higher-vm-workload-density).

## Alternatives

1. [Kubernetes SWAP] - Will be used as soon as possible, but it was
   not available early enough in order to meet our timeline
   requirements.

## Infrastructure Needed [optional]

* OCP CI coverage for [WASP Agent] in order to preform regression
  testing.

[Kubernetes SWAP]: https://github.com/kubernetes/enhancements/issues/2400
[WASP Agent]: https://github.com/OpenShift-virtualization/wasp-agent
[OCI hook]: https://github.com/containers/common/blob/main/pkg/hooks/docs/oci-hooks.5.md
[LLN]: https://en.wikipedia.org/wiki/Law_of_large_numbers
[critical `priorityClass`es]: https://kubernetes.io/docs/tasks/administer-cluster/guaranteed-scheduling-critical-addon-pods/
[KSM]: https://issues.redhat.com/browse/CNV-23960
[FPR]: https://issues.redhat.com/browse/CNV-25921

[^1]: Because `requests.memory == guest.memory + additional_infra_overhead` in
      some cases it can happen that the pod's memory is not smaller than the VM's
      memory.
