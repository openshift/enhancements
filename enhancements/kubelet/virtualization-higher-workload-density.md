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
density - by overcommitting it's memory resources. Due to timeline
needs a two-phased approach is considered.

## Motivation

Today, OpenShift Virtualization is reserving memory (`requests.memory`)
according to the needs of the virtual machine and it's infrastructure
(the VM related pod). However, usually an application within the virtual
machine does not utilize _all_ the memory _all_ the time. Instead,
only _sometimes_ there are memory spikes within the virtual machine.
And usually this is also true for the infrastucture part (the pod) of a VM:
Not all the memory is used all the time. Because of this assumption, in
the following we are not differentiating between the guest of a VM and the
infrastructure of a VM, instead we are just speaking colectively of a VM.

Now - Extrapolating this behavior from one to all virtual machines on a
given node leads to the observation that _on average_ there is no memory
ressure and often a rather low memory utilization - despite the fact that
much memory has been reserved.
Reserved but underutilized hardware resources - like memory in this case -
are a cost factor to cluster owners.

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

#### Timeline

* GA higher workload density in OpenShift Virtualization in 2024

#### Functionality

* Fit more virtual machines onto a node once higher workload density
  is enabled
* Integrate well with [KSM] and [FPR]
* **Technology Preview** - Enable higher density at all, limited
  support for stressed clusters
* **General Availability** - Improve handling of stressed clusters

#### Usability

* Provide a boolean in the OpenShift Console for enabling higher-density.
  Do not require any additional workload or cluster level configuration
  besides the initial provisioning.

### Non-Goals

* Complete life-cycling of the WASP Agent. We are not intending to write
  an Operator for memory over commit for two reasons:
  * [Kubernetes SWAP] is close, writing a fully fledged operator seems
    to be no good use of resources
  * To simplify the transition from WASP to [Kubernetes SWAP]

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

Memory over-committment, and as such swapping, will be initially limited to
virtual machines running in the burstable QoS class.
Virtual machines in the guaranteed QoS classes are not getting over
committed due to alignment with upstream Kubernetes. Virtual machines
will never be in the best-effort QoS because memory requests are
always set.

Later SWAP will be extended to burstable pods - by WASP as well as by
Kube swap.

#### Timeline & Phases

Because [Kubernetes SWAP] is currently in Beta only expect to GA with
Kube 1.32 (OpenShift 4.19, approx mid 2025) this proposal is taking a
two phased approach in order to meet the timeline requirements.

* **Phase 1** - OpenShift Virtualization will provide an out-of-tree
  solution to enable higher workload density.
* **Phase 2** - OpenShift Virtualization will transition to
  [Kubernetes SWAP] (in-tree) whenever this feature is generally
  available in OpenShift itself.

This enhnacement is focusing on Phase 1 and the transition to Phase 2.
Phase 2 is covered by the upstream [Kubernetes SWAP] enhnacements.

#### Phase 1 - Out-Of-Tree SWAP with WASP

[WASP Agent] is a component which is providing an [OCI hook] to enable
SWAP for selected containers hosting virtual machines.

### Workflow Description

**cluster administrator** (Cluster Admin) is a human user responsible
for deploying and administering a cluster.

**virtual machine owner** (VM Owner) is a human user operating a
virtual machine in a cluster.

#### Workflow: Configuring higher workload density

1. The cluster admin is deploying a bare-metal OpenShift cluster
2. The cluster admin is deploying the OpenShift Virtualization Operator
3. The cluster admin is deploying the WASP Agent according to the
   documentation

   a. The cluster admin is adding the `failOnSwap=false` flag to the
      kubelet configuration via a `KubeletConfig` CR, in order to ensure
      that the kubelet will start once swap has been rolled out.
   a. The cluster admin is calculating the amount of swap space to
      provision based on the amount of physical ram and overcommittment
      ratio
   b. The cluster admin is creating a `MachineConfig` for provisioning
      swap on worker nodes
   c. The cluster admin is deploying the [WASP Agent] DaemonSet

4. The cluster admin is configuring OpenShift Virtualization for higher
   workload density via

   a. the OpenShift Virtualization Console "Settings" page
   b. or `HCO` API

The cluster is now set up for higher workload density.

#### Workflow: Leveraging higher workload density

1. The VM Owner is creating a regular virtual machine and is launching it.

### API Extensions

Phase 1 does not require any Kubernetes, OpenShift, or OpenShift
Virtualization API changes.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The `MachineConfig` based swap provisioning will not work, as HCP does
not provide the `MachineConfig` APIs.

#### Standalone Clusters

Standalone regular, and compact clusters are the primary use-cases for
swap.

#### Single-node Deployments or MicroShift

Single-node and MicroShift deployments are out of scope of this
proposal.

### Implementation Details/Notes/Constraints

#### WASP Agent

At it's core the [wasp agent] is a `DaemonSet` delivering an [OCI Hook]
which is used to turn on swap for selected workloads.

Overall the agent is intended to align - and specifically not
conflict - with the upstream Kubernetes SWAP design and behavior in
order to simplify a transition.

##### Design

The design is driven by the following guiding principles:

* System services are more important than workloads. Because workload
  health depends on the health of system services.
* Try to stay aligned to upstream Kubernetes swap behavior in order
  to ease the transition to Kubernetes SWAP once available

###### Enabling swap

An OCI Hook to enable swap by setting the containers cgroup
`memory.swap.max=max`.

* **Technology Preview** - Limited to virt launcher pods
* **General Availability** - Limited to burstable QoS class pods

###### Provisioning swap

Provisioning of swap is left to the cluster administrator.
The hook itself is not making any assumption where the swap is located.

As long as there is no additional tooling available, the recommendation
is to use `MachineConfig` objects to provision swap on nodes.

The `MachineConfig` object would include the necessary scripts
in order to provision swap on a disk, partition, or in a file.

###### Node service protection

All container workloads are run in the `kubepods.slice` cgroup.
All system services are run in the `system.slice` cgroup.

By default the `system.slice` is permitted to swap, however, system
services are critical for a node's health and in turn critical for the
workloads.

Without system services such as `kubelet` or `crio`, any container will
not be able to run well.

Thus, in order to protect the `system.slice` and ensure that the nodes
infrastructure health is prioritized over workload health, the agent is
reconfiguring the `system.slice` and setting `memory.swap.max=0` to
prevent any system service within from swapping.

###### Preventing SWAP traffic I/O saturation

One risk of heavy swapping is to saturate the disk bus with swap traffic,
potentially preventing other processes from performing I/O.

In order to ensure that system services are able to perform I/O, the
agent is configuring `io.latency=50` for the `system.slice` in order
to ensure that it's I/O requests are prioritized over any other slice.
This is, because by default, no other slice is configured to have
`io.latency` set.

###### Critical workload protection

Even critical pod workloads are run in burstable QoS class pods, thus
at **General Availability** time they will be eligible to swap.
However, swapping can lead to increased latencies and response times.
For example, if a critical pod is depending on `LivenessProbe`s, then
these checks can start to fail, once the pod is starting to swap.

This is undesirable and can put a node or a cluster (i.e. if a critical
Operator is affected) at risk.

In order to prevent this problem, swap will be selectively disabled
for pod using the two well-known [critical `priorityClass`es]:

* `system-cluster-critical`
* `system-node-critical`

###### Node memory pressure handling

Dealing with memory pressure on a node is differentiating the TP fom GA.

* **Technology Preview** - `memory.high` is set on the `kubepods.slice`
  in order to force the node to swap, once the `kubepods.slice` memory is
  filling up. Only once swap is full, the system will cross `memory.high`
  and trigger soft evictions.

  * Pro
    * Simple to achieve.
  * Con
    * A lot of memory pressure has ot be present in order to trigger
      soft eviction.

* **General Availability** - Memory based soft and hard eviction is going to
  be disabled, in favor of enabling swap based hard evictions, based on new
  swap traffic and swap utilization eviction metrics.

  * Pro
    * Simple mental model. With memory only, memory eviction is used.
      With swap, swap eviction is used.
    * [LLN] applies, because all pods share the nodes memory
  * Con
    * If there are no burstable QoS pods on a node, then no swapping
      can take place, and no swap related signal will be triggered.
      Only way to remove pressure is cgroup level OOM.
      This is considered to be an edge case and highly unlikely.
      Prometheus alerts for this edge case will be added.

###### Node memory reduction

Swap (with wasp or kube swap) are mechanisms in order to increase the
virtual address space, this is required in order to reduce the
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

##### Differences between Technology Preview vs GA

|                              | TP            | GA                 |
|------------------------------|---------------|--------------------|
| SWAP Provisioning            | MachineConfig | MachineConfig      |
| SWAP Eligibility             | VM pods       | burstable QoS pods |
| Node service protection      | Yes           | Yes                |
| I/O saturation protection    | Yes           | Yes                |
| Critical workload protection | No            | Yes                |
| Memory pressure handling     | Memory based  | Swap based         |

### Risks and Mitigations

#### Phase 1

| Risk                                       | Mitigation             |
|--------------------------------------------|------------------------|
| Miss details and introduce instability     | Limit to VM pods       |

#### Phase 2

Handled by upstream Kubernetes.

### Drawbacks

The major drawback and risk of the [WASP Agent] approach in phase 1 is
due to the lack of integration with Kubernetes. It's prone to
regressions due to changes in Kubernetes.

Thus phase 2 is critical in order to eliminate those risks and
drawbacks.

## Open Questions [optional]

None.

## Test Plan

Add e2e tests for the WASP agent repository for regression testing against
OpenShift.

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
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

* On the cgroup level WASP agent supports only cgroups v2
* On OpenShift level no specific action needed, since all of the APIs used
  by the WASP agent deliverables are stable (DaemonSet, OCI Hook, MachineConfig, KubeletConfig)

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

The WASP Agent OCI hook is based on a stable OCI Hook API, thus few regressions are expected.
Furthermore we expect to go through every minor version of OpenShift, reducing skew.

## Operational Aspects of API Extensions

None

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

TBD

## Alternatives

1. [Kubernetes SWAP] - Will be used as soon as possible, but it was
   not available early enough in order to meet our timeline
   requirements.

## Infrastructure Needed [optional]

* OCP CI coverage for [WASP Agent] in order to preform regression
  testing.

[Kubernetes SWAP]: https://github.com/kubernetes/enhancements/issues/2400
[WASP Agent]: https://github.com/openshift-virtualization/wasp-agent
[OCI hook]: https://github.com/containers/common/blob/main/pkg/hooks/docs/oci-hooks.5.md
[LLN]: https://en.wikipedia.org/wiki/Law_of_large_numbers
[critical `priorityClass`es]: https://kubernetes.io/docs/tasks/administer-cluster/guaranteed-scheduling-critical-addon-pods/
[KSM]: https://issues.redhat.com/browse/CNV-23960
[FPR]: https://issues.redhat.com/browse/CNV-25921

[^1]: Because `requests.memory == guest.memory + additional_infra_overhead` in
      some cases it can happen that the pod's memory is not smaller than the VM's
      memory.
