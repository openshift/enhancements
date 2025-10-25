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

Fit more workloads onto a given node to achieve a higher workload
density by overcommitting its memory resources. Due to timeline
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
* Integrate well with [Kernel Samepage Merging] and [Free Page Reporting]
* Protect the node from resource starvation upon high swapping activity.        

#### Applicability

We expect to mitigate the following situations

* Underutilized guest memory (as described above)
* Infrastructure related memory spikes in an over-committed environment
* Considerable amount of "cold memory allocations" - thus allocations
  which are rarely used
* 
#### Usability

* Provide a boolean in the OpenShift Console for enabling higher-density.
  Do not require any additional workload or cluster level configuration
  besides the initial provisioning.
* Provide a method to deploy the requried configuration for safer
  memory overcommit, such as swap provisioning, tuning of system services, etc.
* Provide a maintainable delivery method for new releases of the proposal.

### Non-Goals

* Allow swapping for VM pods only. We don't want to diverge from the upstream approach 
  since [Kubernetes SWAP] allows swapping for all pods associated with the burtsable QoS class.
* Swap Operator graduation. This will be discussed in a dedicated proposal.
* Allow choosing how to enable swap for pods: wasp-agent or kube swap. That is an implementation detail, hence it should be 
  transparent to the end user.
* SNO: MicroShift and Compact Cluster deployments are out of scope of this
  proposal, since enabling swap on control-plane nodes is not supported.
* HCP: Hosted cluster with OCP Virt, since nested virt is not supported.

## Proposal

A higher workload density is achieved by combining two mechanisms

1. Under-request memory resources for virtual machines by having the VM
   pods (`launcher`) `requests.memory` being smaller than the VMs `guest.memory`
   according to a configured ratio[^1]. This is owned by
   [KubeVirt and present today](https://kubevirt.io/user-guide/operations/node_overcommit/).
2. Compensate for the memory over-committment by using SWAP in order
   to extend the virtual memory. This is owned by the platform and not
   available today.

The proposal suggests four delivery phases from timeline perspective. We will refer to these
phases while discussing the components.

### Scope

Following the upstream kubernetes approach every workload marked as burstable QoS would be able to swap.
There is no differentiation between the type of the workload: regular pod or a VM.
With that being said, swapping will be allowed for pods with Burstable QoS class.

Among the VM workloads, VMs of high-performance configuration (NUMA affinity, CPU affinity, etc.) cannot overcommit.
Also, VMs with best-effort QoS class don't exist because requesting memory is mandatory for the VM spec. 
For VMs of Burstable Qos class over-commited VM stability can be achieved during memory spikes by swapping out "cold" memory pages.

### WASP Agent

Overall the agent is intended to align - and specifically not
conflict - with the upstream [Kubernetes SWAP design](https://kubernetes.io/blog/2023/08/24/swap-linux-beta/) and behavior in
order to simplify a transition. One extra functionality that the agent
currently provides is the swap-based evictions.

The agent uses [OCI Hook] to enable swap via container cgroup. The hook script
sets the containers cgroup `memory.swap.max=max`.

Wasp-agent image is deployed as a Daemonset. In the first and the second phase the Daemonset is deployed
manually as part of the [deployment guide](https://github.com/openshift-virtualization/wasp-agent/blob/main/docs/configuration.md) while in the third and fourth phases it will be deployed by the swap operator.

* **Tech Preview**
    * Limited to burstable QoS class pods.
    * Uses `UnlimitedSwap` approach. Using OCI hook each container is started with `memory.swap.max=max` in its cgroup.
* **General Availability**
    * Container starts with `UnlimitedSwap` as in TP, then reconciled to `LimitedSwap` by the wasp-agent limited swap controller.
        * For more info, refer to the upstream documentation on how to calculate [limited swap](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/2400-node-swap#steps-to-calculate-swap-limit).
    * Added swap-based evictions.
    * Added critical workload protection.

![wasp](https://github.com/user-attachments/assets/314d387d-788a-4aa6-9275-c882f5f329aa)

**NOTE**: In WASP GA setting the swap limit isn't atomic, it can happen that a container application will start
to allocate memory before the limit is set by the wasp-agent limited swap controller.

### Swap Operator

The operator will be responsible for the following tasks:
* Deployment and reconciliation of the WASP agent resources: Daemonset, service-account, RBAC and SCC.
* Deployment and reconciliation of the swap related Machine Configs and Kubelet Configs.
* Transparent switching from WASP agent to kubernetes swap once the latter will graduate to GA in upstream.
* Provide a solution for swap storage provision in case of different disk sizes on worker nodes.

Kubelet configuration auditing for swap will be out of the swap operator scope since
the swap operator is not a built-in Openshift operator. 
The graduation of the operator from CNV to OLM can be discussed in a dedicated proposal.

### Timeline & Phases

| Phase                                                                          | Target       |   
|--------------------------------------------------------------------------------|--------------|
| Phase 1 - Out-Of-Tree SWAP with WASP TP                                        | mid 2024     |
| Phase 2 - WASP GA with swap-based evictions                                    | end 2024     |
| Phase 3 - Transition to swap operator. Limited to CNV-only users               | mid-end 2025 |
| Phase 4 - Limits removed. All Openshift users allowed to use the swap operator | TBD          |

* **Phase 1 (Done)** - OpenShift Virtualization will provide an out-of-tree
  solution (WASP) to enable higher workload density and swap.
* **Phase 2 (Done)** - WASP will include swap-based eviction mechanisms.
* **Phase 3 (In Progress)** - OpenShift Virtualization will transition to swap operator.
  OpenShift will [allow](#swap-ga-for-cnv-users-only) SWAP to be configured only if the swap operator is installed on the cluster.
* **Phase 4 (TBD)** - OpenShift will allow every user to opt-in for swap. Swap operator will become stand-alone operator deployed by OLM.

### Workflow Description

**cluster administrator** (Cluster Admin) is a human user responsible
for deploying and administering a cluster.

**virtual machine owner** (VM Owner) is a human user operating a
virtual machine in a cluster.

#### Workflow: Configuring higher workload density

**Phases 1-2**
* The cluster admin is deploying a bare-metal OpenShift cluster
* The cluster admin is deploying the OpenShift Virtualization Operator
* The cluster admin is enabling swap on Openshift according to the
   [documentation](https://github.com/openshift-virtualization/wasp-agent/blob/main/docs/configuration.md)
* The cluster admin is configuring OpenShift Virtualization for VM memory overcommit via:
    * the OpenShift Virtualization Console "Settings" page 
    * [or `HCO` API](https://github.com/kubevirt/hyperconverged-cluster-operator/blob/main/docs/cluster-configuration.md#configure-higher-workload-density)

**Phase 3**
* The cluster admin is deploying a bare-metal OpenShift cluster.
* The cluster admin is deploying the OpenShift Virtualization Operator.
* The cluster admin is configuring the swap operator.
* The cluster admin is configuring OpenShift Virtualization for VM memory overcommit via:
   * The OpenShift Virtualization Console "Settings" page
   * [or `HCO` API](https://github.com/kubevirt/hyperconverged-cluster-operator/blob/main/docs/cluster-configuration.md#configure-higher-workload-density)

**Phase 4**
* The cluster admin is deploying a bare-metal OpenShift cluster.
* The cluster admin is deploying the swap operator.
* The cluster admin is configuring the swap operator.
* The cluster admin is deploying the OpenShift Virtualization Operator.
* The cluster admin is configuring OpenShift Virtualization for VM memory overcommit via:
   * The OpenShift Virtualization Console "Settings" page
   * [or `HCO` API](https://github.com/kubevirt/hyperconverged-cluster-operator/blob/main/docs/cluster-configuration.md#configure-higher-workload-density)


The cluster is now set up for higher workload density.

#### Workflow: Leveraging higher workload density

1. The VM Owner is creating a regular virtual machine and is launching it. The VM owner must not specify memory requests in the VM spec, but only the guest memory size.

### API Extensions

#### Phases 1-2
No API extension. The [documented](https://github.com/openshift-virtualization/wasp-agent/blob/main/docs/configuration.md) procedure relies on existing released APIs from MCO and CNV.

#### Phases 3-4
For existing users from previous phases the transition is transparent. The swap operator will reconcile the legacy artifacts.
For new users in these phases the swap operator API should be used. The API will be discussed in a dedicated EP. 

### Topology Considerations

#### Hypershift / Hosted Control Planes

See [Non-Goals](#non-goals)

#### Single-node Deployments or MicroShift

See [Non-Goals](#non-goals)

#### Standalone Clusters

Standalone and regular clusters are the primary use-cases for
swap.

## Graduation Criteria
See [Timeline & Phases](#timeline--phases)

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
N/A

### Removing a deprecated feature
N/A

### Implementation Details/Notes/Constraints

The design is driven by the following guiding principles:

* System services are more important than workloads. Because workload
  health depends on the health of system services.
* Try to stay aligned to upstream Kubernetes swap behavior in order
  to ease the transition to Kubernetes SWAP once available

#### Swap-based evictions 

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

Swap-based evictions are integrated into the wasp-agent and deployed alltogether as single image.

#### Machine-level swap configuration

The configuration consists of multiple subjects. These subjects are not covered
by the upstream swap KEP. It's considered by the proposal as an add-on for optimal
user experience with swap in Openshift. In phase one and phase two the configuration is deployed
manually as part of the [deployment guide](https://github.com/openshift-virtualization/wasp-agent/blob/main/docs/configuration.md) while in phase three and phase four it will be deployed by the swap operator.

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

By default, the `system.slice` is permitted to swap, however system
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

#### Hypervisor/OS level assistance 

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

#### VM higher-density: Differences between Technology Preview vs GA

|                              | TP                  | GA                  |
|------------------------------|---------------------|---------------------|
| SWAP Provisioning            | MachineConfig       | MachineConfig       |
| SWAP Eligibility             | burstable QoS pods  | burstable QoS pods  |
| Node service protection      | Yes                 | Yes                 |
| I/O saturation protection    | Yes                 | Yes                 |
| Critical workload protection | No                  | Yes                 |
| Memory pressure handling     | Memory based        | Memory & Swap based |

### Risks and Mitigations

#### Phase 1

| Risk                                       | Mitigation                                                                                      |
|--------------------------------------------|-------------------------------------------------------------------------------------------------|
| Miss details and introduce instability     | * Adjust overcommit ratio <br/> * Tweak kubelet soft-eviction thresholds <br/> * Upgrade to GA <br/> |


#### Phases 2-3

| Risk                                                      | Mitigation                                        |
|-----------------------------------------------------------|---------------------------------------------------|
| Swap-based evictions are based on API-initiated evictions | Also rely on kubelet-level memory-based evictions |
| New CVEs found                                            | Upgrade to released CNV z-stream                  |

#### Phase 4

| Risk                                                      | Mitigation                                        |
|-----------------------------------------------------------|---------------------------------------------------|
| Swap-based evictions are based on API-initiated evictions | Also rely on kubelet-level memory-based evictions |
| New CVEs found                                            | Upgrade to released swap operator z-stream        |



### Drawbacks

The major drawback and risk of the [WASP Agent] approach in phase 1 is
due to the lack of integration with Kubernetes. It's prone to
regressions due to changes in Kubernetes.

Thus phase 2 is critical in order to eliminate those risks and
drawbacks.

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


## Upgrade / Downgrade Strategy

### Phase 1 <-> Phase 2
Straight-forward. No API changes.
 
### Phase 2 <-> Phase 3
Since swap operator is backward-compatible, the upgrade to phase 3 is straight-forward. 
Downgrading is safe as well since when swap operator will be removed it won't remove the legacy operands.

### Phase 3 <-> Phase 4
Straight-forward. No API changes.

## Version Skew Strategy

The WASP Agent OCI hook is based on a stable OCI Hook API, thus few regressions are expected.
Furthermore, we expect to go through every minor version of OpenShift, reducing skew.

## Operational Aspects of API Extensions

### Worker nodes
The amount of RAM and the disk topology must be identical on all worker nodes.

## Support Procedures

Please refer to the **Verification** section in the wasp-agent deployment [documentation](https://docs.openshift.com/container-platform/4.17/virt/post_installation_configuration/virt-configuring-higher-vm-workload-density.html#virt-using-wasp-agent-to-configure-higher-vm-workload-density_virt-configuring-higher-vm-workload-density).

## Alternatives (Not Implemented)

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
[Free Page Reporting]: https://issues.redhat.com/browse/CNV-25921
[Kernel Samepage Merging]: https://issues.redhat.com/browse/CNV-23960

[^1]: Because `requests.memory == guest.memory + additional_infra_overhead` in
      some cases it can happen that the pod's memory is not smaller than the VM's
      memory.
