---
title: dedicated-cpus-for-dpdk-vswitch
authors:
  - "@Tal-or"
reviewers:
  - "@yanirq"
  - "@MarSik"
  - "@jmencak"
  - "@mcoquelin"
approvers:
  - "@jmencak"
  - "@MarSik"
api-approvers:
  - "@MarSik"
creation-date: 2026-05-06
last-updated: 2026-05-19
status: implementable
tracking-link:
  - https://redhat.atlassian.net/browse/CNF-22582
  - https://redhat.atlassian.net/browse/RFE-8921
see-also:
  - "/enhancements/node-tuning/mixed-cpu-node-plugin.md"
  - "/enhancements/workload-partitioning/management-workload-partitioning.md"
replaces: []
superseded-by: []
---

# Dedicate CPU Resources for DPDK-based vSwitch/vRouter

## Summary

This enhancement extends the PerformanceProfile API to allow cluster administrators to dedicate
a set of CPUs exclusively for DPDK-based virtual switches (e.g., OVS-DPDK) or virtual routers
(e.g., OpenPErouter). The dedicated CPUs are fully isolated from system daemons, kernel services
(RCU callbacks, interrupts), and all Kubernetes-scheduled pod workloads, enabling infrastructure
networking and pod-to-pod communication to leverage DPDK-accelerated packet processing.

Two new API fields are introduced: `spec.cpu.dedicated` to define the dedicated CPU set, and
`spec.net.disableOvsDynamicPinning` to prevent OVN-Kubernetes from dynamically changing `ovs-vswitchd` and
`ovsdb-server` processes' CPU affinity.
When `dedicated` is set, the operator automatically configures kernel-level
isolation (`isolcpus=managed_irq`, `nohz_full`, `rcu_nocbs`), creates a separate cgroup slice named dedicatedcpus.slice 
with `cpuset.cpus.partition=isolated` to remove dedicated CPUs from the kernel scheduler's load
balancing domains, adds the dedicated CPUs to the irqbalance banned mask, and updates the
systemd CPU affinity to exclude them from host processes.

## Motivation

OVS-DPDK and similar userspace networking stacks use busy-loop polling threads (PMD threads) that
require exclusive, undisturbed access to CPU cores to achieve line-rate packet processing. Any
interference — from kernel interrupts, RCU callbacks, system daemons, or other workloads — causes
packet drops, jitter, and degraded throughput.

Today, the PerformanceProfile API provides `reserved` and `isolated` CPU sets. However, `reserved`
CPUs can still run Burstable and BestEffort QoS pods (Kubelet only excludes Guaranteed QoS pods),
and `isolated` CPUs are intended for application workloads scheduled by Kubelet. Neither category
provides the complete isolation needed for DPDK vSwitch processes that run outside of Kubernetes
pod scheduling.

As part of efforts to support OVS-DPDK natively in OpenShift and the OpenPErouter project, there is
a need for a CPU pool that is excluded from everything: OS daemons, kernel housekeeping, interrupt
handling, and all Kubernetes-scheduled workloads regardless of QoS class.

### User Stories

* As a cluster administrator deploying OVS-DPDK or OpenPErouter on OpenShift, I want to
  dedicate specific CPUs for the DPDK vSwitch so that infrastructure networking and pod-to-pod
  communication can leverage DPDK-accelerated packet processing without interference from OS
  daemons, kernel services, or pod workloads.

* As a cluster administrator, I want to disable OVN-Kubernetes dynamic OVS pinning so that
  dedicated CPUs remain fully isolated and are not affected by OVN-Kubernetes reassigning
  `ovs-vswitchd` and `ovsdb-server` processes onto them at runtime.

### Goals

- Provide a `dedicated` CPU set in the PerformanceProfile API that is fully excluded from Kubelet
  scheduling (all QoS classes), OS daemons, and kernel housekeeping.
- Automatically ban dedicated CPUs from irqbalance and configure `isolcpus=managed_irq`
  to prevent managed interrupt affinity on dedicated CPUs. Create a cgroup v2 partition
  (`cpuset.cpus.partition=isolated`) to remove dedicated CPUs from the kernel scheduler's load
  balancing domains.
- Provide the ability to disable OVN-Kubernetes dynamic OVS thread pinning independently
  of CPU dedication. This option is orthogonal to `dedicated` — OVS dynamic pinning and
  OVS-DPDK can coexist, and disabling dynamic pinning may not be desired in all `dedicated`
  CPU scenarios.
- Integrate with TuneD so that dedicated CPUs are added to `isolcpus` and receive the same
  kernel-level isolation as `isolated` CPU sets.

### Non-Goals

- Managing the lifecycle of OVS-DPDK processes themselves (PMD thread creation, DPDK EAL
  initialization). This is the responsibility of OVN-Kubernetes or the networking operator.

## Proposal

This proposal introduces two new fields to the PerformanceProfile API and corresponding changes
to the Node Tuning Operator controllers that generate Kubelet configuration, TuneD daemon
profiles, Tuned custom resources (`tuneds.tuned.openshift.io`), and MachineConfig resources.

### Prerequisites

The `dedicated` CPU feature requires either **Workload Partitioning** or the Kubelet
**`strict-cpu-reservation`** CPUManager policy option to be enabled on the node. Without one of
these, Burstable and BestEffort QoS pods can still be scheduled on dedicated CPUs through
kernel cpuset inheritance, breaking the isolation guarantee.

The PerformanceProfile controller can check the infrastructure mode to determine whether
Workload Partitioning is present and report an error condition on the PerformanceProfile status
when `dedicated` is set without WP or `strict-cpu-reservation`.

### New API Fields

1. **`spec.cpu.dedicated`** (`CPUSet`, optional) — A set of CPUs to be dedicated exclusively for
   infrastructure networking workloads such as DPDK vSwitch or vRouter. When set, the operator
   automatically configures full isolation for these CPUs:
   - **Kubelet**: Added to `ReservedSystemCPUs` (union with `reserved` CPUs) so that no pods of
     Garanteed QoS class are scheduled on them.
   - **Kernel boot parameters**: Added to `isolcpus=managed_irq` (prevents the kernel from
     auto-assigning managed interrupts to these CPUs), `nohz_full` (disables scheduler ticks
     when only one task is running), and `rcu_nocbs` (offloads RCU callbacks to housekeeping
     CPUs).
   - **Cgroup v2 partition**: A dedicated cgroup slice is created with `cpuset.cpus.exclusive`
     set to the dedicated CPUs and `cpuset.cpus.partition` set to `isolated`. This removes
     the dedicated CPUs from the kernel scheduler's load balancing domains, equivalent to the
     `isolcpus=domain` kernel parameter but applied at the cgroup level — avoiding conflicts
     with the existing `isolcpus` boot parameter used for isolated CPUs.
   - **TuneD**: Added to `isolated_cores` (union with `isolated` CPUs) so that kernel
     housekeeping is moved off these cores.
   - **Irqbalance**: Automatically added to the `IRQBALANCE_BANNED_CPUS` mask so that hardware
     interrupts are never routed to these CPUs.
   - **Systemd**: The systemd CPU affinity mask is updated to exclude dedicated CPUs, preventing
     host services (journald, NetworkManager, etc.) from running on them.

2. **`spec.net.disableOvsDynamicPinning`** (`*bool`, optional, default `false`) — Added to the
   existing `Net` struct alongside `userLevelNetworking` and `devices`. When set to `true`,
   the MachineConfig that triggers OVN-Kubernetes dynamic OVS thread pinning is not generated.
   This prevents OVN-Kubernetes from dynamically modifying OVS thread CPU affinity at runtime,
   which is necessary when CPU affinity is managed statically via the `dedicated` field.

### Workflow Description

**cluster administrator** is a human user responsible for configuring node performance profiles.

1. The cluster administrator identifies the CPUs on the target node(s) that should be dedicated
   to OVS-DPDK PMD threads. These are typically selected from the same NUMA node as the
   DPDK-bound NICs.

2. The cluster administrator creates or updates a PerformanceProfile CR with the new fields:

   Example topology: single-socket system, 4 cores, HT enabled (CPUs 0-3 physical, 4-7 HT
   siblings). The DPDK-bound NIC is on NUMA node 0.

   ```yaml
   apiVersion: performance.openshift.io/v2
   kind: PerformanceProfile
   metadata:
     name: dpdk-profile
   spec:
     cpu:
       reserved: "0,4"
       dedicated: "1,5"
       isolated: "2-3,6-7"
     net:
       disableOvsDynamicPinning: true
     nodeSelector:
       node-role.kubernetes.io/worker-dpdk: ""
   ```

   In this example, dedicated CPU 1 and its HT sibling 5 are reserved for OVS-DPDK PMD
   threads. Reserved CPU 0 and its sibling 4 handle system daemons. The remaining CPUs
   (2-3, 6-7) are isolated for application workloads.

3. The Node Tuning Operator reconciles the PerformanceProfile and generates:
   - A **KubeletConfig** with `ReservedSystemCPUs` set to the union of `reserved` and `dedicated`
     CPUs (`"0-1,4-5"` in this example), ensuring no pods are scheduled on dedicated CPUs.
   - A **TuneD profile** that:
     - Sets `isolated_cores` to the union of `isolated` and `dedicated` CPUs
       (`"1-3,5-7"` in this example), ensuring kernel housekeeping is moved off these cores.
     - Configures kernel boot parameters: `isolcpus=managed_irq:1-3,5-7`,
       `nohz_full=1-3,5-7`, `rcu_nocbs=1-3,5-7` for both isolated and dedicated CPUs.
     - Updates the systemd CPU affinity mask to exclude dedicated CPUs, confining all host
       services to reserved CPUs only.
   - A **cgroup v2 partition** (via a systemd slice or MachineConfig drop-in) that:
     - Creates a cgroup slice named `dedicatedcpus.slice` with `cpuset.cpus.exclusive=1,5` (the dedicated CPUs).
     - Sets `cpuset.cpus.partition=isolated` to remove the dedicated CPUs from the kernel
       scheduler's load balancing domains.
   - A **MachineConfig** that:
     - Does NOT include the OVS dynamic pinning trigger file (because `disableOvsDynamicPinning`
       is `true`).
     - Configures the irqbalance service with `IRQBALANCE_BANNED_CPUS` set to the hex mask of
       the dedicated CPUs.

4. The MachineConfigOperator applies the generated MachineConfig, which triggers a node reboot.

5. After reboot, the node is fully configured:
   - CPUs 0,4 are reserved for system daemons and Kubernetes system pods.
   - CPUs 1,5 are dedicated for OVS-DPDK — excluded from pod scheduling, kernel housekeeping,
     and interrupt handling.
   - CPUs 2-3,6-7 are isolated for application workloads (Guaranteed QoS pods).

6. OVN-Kubernetes (or the network operator) starts OVS-DPDK and pins PMD threads to the
   dedicated CPUs (1,5). Because dynamic pinning is disabled, OVN-Kubernetes will not modify
   these assignments at runtime.

### API Extensions

This enhancement modifies the `PerformanceProfile` CRD (`performance.openshift.io/v2`):

```go
type Net struct {
    // ... existing fields (Devices, UserLevelNetworking) ...

    // DisableOvsDynamicPinning when set to true, prevents OVN-Kubernetes
    // from dynamically adjusting OVS thread CPU affinity.
    // +optional
    DisableOvsDynamicPinning *bool `json:"disableOvsDynamicPinning,omitempty"`
}

type CPU struct {
    // ... existing fields (Isolated, Reserved, Shared, Offlined) ...

    // Dedicated defines a set of CPUs fully isolated from the operating system
	  // and Kubernetes scheduling, intended for exclusive use by user-space
	  // processes (for example, infrastructure networking workloads such as
	  // DPDK-based vSwitch or vRouter). WorkloadPartitioning or --strict-cpu-reservation
	  // kubelet CPUManager policy option are a prerequisite for this feature.
    // +optional
    Dedicated *CPUSet `json:"dedicated,omitempty"`
}
```

No new CRDs, webhooks, or aggregated API servers are introduced. The existing PerformanceProfile
validation webhook will be extended to validate:
- `dedicated` CPUs do not overlap with `reserved`, `isolated`, or `offlined` CPUs.
- When `dedicated` is set, either Workload Partitioning or Kubelet `strict-cpu-reservation`
  must be enabled (documented requirement; webhook enforcement deferred to a future iteration).

### Topology Considerations

#### Hypershift / Hosted Control Planes

This feature only affects the data plane (worker nodes). The PerformanceProfile is applied to
worker nodes via the NodePool's tuning configuration. No changes are required to management
cluster components. The Node Tuning Operator running in the hosted control plane already handles
PerformanceProfile reconciliation for guest cluster nodes.

#### Standalone Clusters

Fully applicable. This is the primary deployment model for telco RAN/edge use cases.

#### Single-node Deployments or MicroShift

For SNO, the feature is applicable but administrators must be careful not to starve control plane
components by dedicating too many CPUs. The `reserved` CPU set must be large enough for both
control plane workloads and system daemons.

MicroShift does not use the PerformanceProfile CRD or the Node Tuning Operator. Low-latency
tuning on MicroShift is achieved through host-level RHEL TuneD profiles
(`microshift-low-latency` RPM) and manual kubelet configuration. Achieving equivalent CPU
dedication on MicroShift would require manual host-level TuneD and kubelet configuration —
this is out of scope for this enhancement.

#### OpenShift Kubernetes Engine

This feature depends on the Node Tuning Operator and PerformanceProfile API which are part of
OCP, not OKE. This enhancement does not apply to OKE.

### Implementation Details/Notes/Constraints

#### Kubelet Configuration

The `dedicated` CPUs are added to Kubelet's `ReservedSystemCPUs`. The controller computes the
union of `reserved`, `dedicated`, and (if MixedCPUs is enabled) `shared` CPU sets:

```
ReservedSystemCPUs = Reserved ∪ Dedicated ∪ Shared
```

This ensures that the Kubelet CPU manager will not allocate any of these CPUs to pods,
regardless of QoS class. For complete exclusion of BestEffort and Burstable pods from dedicated
CPUs, either Workload Partitioning or the `strict-cpu-reservation` Kubelet static policy option
should be enabled.

#### TuneD Profile

The dedicated CPUs are added to TuneD's existing `isolated_cores` variable:

```
isolated_cores = Isolated ∪ Dedicated
```

TuneD computes reserved CPUs as the complement of `isolated_cores` (all online CPUs minus
`isolated_cores`), so adding dedicated CPUs to `isolated_cores` automatically excludes them
from the housekeeping domain.

The dedicated CPUs share the following kernel boot parameters with isolated CPUs via TuneD
(the parameters are applied to the union of isolated and dedicated CPU sets):
- `isolcpus=managed_irq:<isolated ∪ dedicated>` — prevents the kernel from automatically
  setting managed interrupt affinity to these CPUs.
- `nohz_full=<isolated ∪ dedicated>` — enables adaptive-ticks mode, suppressing scheduler timer
  interrupts when only one runnable task is on the CPU.
- `rcu_nocbs=<isolated ∪ dedicated>` — offloads RCU callback processing to housekeeping CPUs.

A new `DedicatedCpus` template variable is introduced for cases where the TuneD profile needs
to reference just the dedicated CPU set separately from isolated CPUs.

#### Cgroup v2 Partition Isolation

To achieve scheduler domain isolation for dedicated CPUs without using the `isolcpus=domain`
kernel boot parameter, a cgroup v2 separate slice with isolated partition is created for the dedicated CPUs.

The operator generates a systemd slice named `dedicatedcpus.slice` (via MachineConfig drop-in) that creates a cgroup with:
- `cpuset.cpus.exclusive` set to the dedicated CPU numbers.
- `cpuset.cpus.partition` set to `isolated`.

When `cpuset.cpus.partition=isolated` is set on a cgroup, the kernel removes those CPUs from
the scheduler's load balancing domains — the same effect as `isolcpus=domain`, but applied at
the cgroup level rather than at boot time. This approach is consistent with how OpenShift already
handles Guaranteed pods with integral CPUs when the `cpu-load-balancing.crio.io: disable`
annotation is used, but applied at the infrastructure level rather than the pod level.

This avoids the fundamental conflict with the `isolcpus` kernel parameter: since `isolcpus` can
only be specified once at boot, it is not possible to apply `domain` isolation to dedicated CPUs
without also applying it to isolated CPUs.

#### Systemd CPU Affinity

The TuneD profile updates the systemd CPU affinity mask by 
setting the `systemd.cpu_affinity` kernel command-line parameter, to exclude dedicated CPUs. 
This is done via the `[sysctl]` or `[systemd]` TuneD plugin,
similar to how the existing `cpu-partitioning`
TuneD profile confines system services to housekeeping CPUs 
(see [tuned cpu-partitioning profile](https://github.com/redhat-performance/tuned/blob/master/profiles/cpu-partitioning/tuned.conf#L28)
and the [openshift-node-performance template](https://github.com/openshift/cluster-node-tuning-operator/blob/main/assets/performanceprofile/tuned/openshift-node-performance#L141)).

By setting the systemd `CPUAffinity` to the reserved CPUs only, all systemd-managed services
(journald, NetworkManager, irqbalance, etc.) are confined to reserved CPUs and cannot run on
dedicated or isolated CPUs. This provides the host-process isolation that `isolcpus` alone
does not guarantee.

#### Irqbalance Configuration

When `dedicated` CPUs are set, their hex CPU mask is automatically computed and injected as
the `IRQBALANCE_BANNED_CPUS` environment variable into the irqbalance systemd service unit
via MachineConfig. The existing `clear-irqbalance-banned-cpus.sh` script is modified to:
- Accept the static banned CPU mask from the environment variable.
- Bitwise-AND the banned mask with `/proc/irq/default_smp_affinity` to exclude dedicated CPUs
  from interrupt handling.
- Use the banned mask as the base for CRI-O's irqbalance backup configuration instead of
  a hardcoded zero.

No separate API field is needed — irqbalance banning is an automatic consequence of setting
`dedicated` CPUs.

#### OVS Dynamic Pinning

When `disableOvsDynamicPinning` is `true`, the MachineConfig controller simply does not generate
the trigger file that OVN-Kubernetes watches to perform dynamic OVS thread pinning. This is a
clean opt-out — no runtime logic changes in OVN-Kubernetes are required.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Administrator dedicates too many CPUs, starving system daemons or Kubelet | Optional validation webhook warns if `reserved` CPU count drops below a safe minimum. Documentation provides guidance on CPU budget planning. |
| `dedicated` CPUs overlap with `isolated` or `reserved` | Validation webhook rejects overlapping CPU sets. |
| Feature is used without Workload Partitioning or `strict-cpu-reservation`, leaving Burstable/BestEffort pods able to run on dedicated CPUs via kernel cpuset inheritance | Documented prerequisite. Webhook enforcement deferred to a future iteration. |
| Disabling OVS dynamic pinning on a node where OVS-DPDK is not deployed leaves OVS without any CPU affinity management | Documentation clarifies that `disableOvsDynamicPinning` should only be set when static CPU dedication is configured for OVS-DPDK. |
| Interaction with CRI-O banned CPUs — CRI-O has its own mechanism for banning CPUs from container workloads | Initial implementation does not modify CRI-O banned CPUs. Integration will be evaluated as a follow-up based on testing. |

### Drawbacks

## Alternatives (Not Implemented)

### Automatically disable OVS dynamic pinning when `dedicated` is set

Instead of a separate `disableOvsDynamicPinning` flag, OVS dynamic pinning could be
automatically disabled whenever `dedicated` CPUs are configured. This was rejected because
the two features should remain orthogonal — there may be use cases where dedicated CPUs are
needed but dynamic OVS pinning should remain active (e.g., dedicating CPUs for non-networking purposes
which requires dynamic OVS pinning enabled).

### Separate `irqbalanceBanned` API field

An earlier design included a separate `spec.cpu.irqbalanceBanned` field to allow banning
arbitrary CPUs from irqbalance independently of CPU dedication. This was rejected because:
- Dedicated CPUs always need irqbalance banning — making it automatic reduces configuration
  burden and eliminates the risk of misconfiguration.
- A separate field adds API surface without a clear use case that differs from `dedicated`.

## Open Questions

1. Should the API field be named `dedicated` or something more descriptive like
   `infrastructureNetworking` or `dpdkCpus`? The current name is generic enough to support
   future use cases beyond DPDK but may be too vague.

## Test Plan

### Unit Tests

- Validation of CPU set non-overlapping constraints (`dedicated` vs `reserved` vs `isolated`).
- Kubelet config generation: verify `ReservedSystemCPUs` is the union of `reserved`, `dedicated`,
  and `shared`.
- TuneD profile generation: verify `isolated_cores` is the union of `isolated` and `dedicated`.
- Kernel boot parameters: verify `isolcpus=managed_irq`, `nohz_full`, and `rcu_nocbs`
  include dedicated CPUs.
- Cgroup v2 partition: verify a cgroup slice named dedicatedcpus.slice exists for dedicated CPUs with
  `cpuset.cpus.exclusive` set and `cpuset.cpus.partition=isolated`.
- Irqbalance banned CPU mask: verify dedicated CPUs are automatically included.
- Systemd CPU affinity: verify dedicated CPUs are excluded from the affinity mask.
- MachineConfig generation with and without `disableOvsDynamicPinning`.

### E2E Tests

- Apply a PerformanceProfile with `dedicated` and `disableOvsDynamicPinning` fields. Verify:
  - Kubelet's `ReservedSystemCPUs` includes dedicated CPUs.
  - TuneD profile's `isolated_cores` includes dedicated CPUs.
  - Kernel cmdline contains `isolcpus=managed_irq:<isolated ∪ dedicated>`,
    `nohz_full=<isolated ∪ dedicated>`, and `rcu_nocbs=<isolated ∪ dedicated>`.
  - A cgroup v2 partition exists for dedicated CPUs with `cpuset.cpus.exclusive` set and
    `cpuset.cpus.partition=isolated`.
  - The irqbalance service has `IRQBALANCE_BANNED_CPUS` set to the dedicated CPU mask.
  - Systemd CPU affinity excludes dedicated CPUs (`grep Cpus_allowed_list /proc/1/status`).
  - The OVS dynamic pinning trigger file is absent.
  - No pods (Guaranteed, Burstable, or BestEffort) are scheduled on dedicated CPUs.
  - Hardware interrupts are not routed to dedicated CPUs (`/proc/interrupts` verification).
  - No host processes are running on dedicated CPUs (`ps -eo pid,psr,comm` verification).

### Integration Tests

- Verify the interaction with Workload Partitioning: when both are enabled, management workloads
  are confined to their partitioned CPUs and do not leak into dedicated CPUs.
- Verify the interaction with MixedCPUs: when both `shared` and `dedicated` are set, the CPU
  sets are correctly unioned in Kubelet config.
- Verify that `IRQBALANCE_BANNED_CPUS` is handled correctly when isolated containers (using the
  `irq-load-balancing.crio.io: disable` annotation) are created and deleted: dedicated CPUs must
  remain banned at all times, while isolated container CPUs are dynamically added and removed from the `IRQBALANCE_BANNED_CPUS` 
  by CRI-O without affecting the dedicated CPU entries in the mask.

## Graduation Criteria

### Dev Preview -> Tech Preview

- All API fields implemented and functional.
- Unit tests and basic e2e tests passing in CI.
- Initial documentation available.

### Tech Preview -> GA

- Full e2e test coverage including interaction with Workload Partitioning and MixedCPUs.
- Upgrade testing completed.
- User-facing documentation merged in openshift-docs.
- Feedback from at least one customer deployment incorporated.

### Removing a deprecated feature

N/A — this is a new feature.

## Upgrade / Downgrade Strategy

### Upgrade

- The new API fields are optional with zero-value defaults. Existing PerformanceProfiles continue
  to work without modification after upgrade.
- No migration is required. The operator detects the presence of new fields and generates the
  appropriate configuration only when they are set.

### Downgrade

N/A

## Version Skew Strategy

- The Node Tuning Operator is the sole consumer of the new PerformanceProfile fields. There is no
  cross-component version skew concern within the operator itself.
- If the operator is upgraded but the CRD has not been updated yet, the new fields will not be
  present and the operator falls back to existing behavior.
- TuneD and irqbalance run on the node and are configured via MachineConfig — they do not need
  to be aware of the PerformanceProfile API version.

## Operational Aspects of API Extensions

- The PerformanceProfile CRD is extended with two new optional fields. No new CRDs, webhooks,
  or API servers are introduced.
- Expected usage: one PerformanceProfile per node role, typically once per
  MCP. No impact on API throughput or scalability.
- The existing `PerformanceProfileStatus` conditions (`Available`, `Degraded`, `Progressing`)
  continue to reflect the reconciliation state, including errors from invalid CPU set
  configurations.

### Failure Modes

- If `dedicated` CPUs are specified but the node does not have those CPU IDs, the TuneD daemon
  may fail to apply the profile and emit an error. If TuneD reports both `TunedDegraded=True`
  and `TunedProfileApplied=False`, the PerformanceProfile will report a `Degraded` condition
  with reason `TunedProfileDegraded`. Note: CPU IDs are not validated against the node at
  admission time — the failure is detected at TuneD profile application time.
- If the dedicated CPU mask produces an invalid irqbalance configuration, the irqbalance service
  may fail to start. The operator does not monitor irqbalance health — this failure would only
  be visible in node logs (`journalctl -u irqbalance`).

## Support Procedures

- **Symptoms**: If dedicated CPUs are not being isolated, check:
  - `kubelet` config: `cat /etc/kubernetes/kubelet.conf | grep reservedSystemCPUs`
  - TuneD active profile: `tuned-adm active` and inspect `isolated_cores` value.
  - Irqbalance: `systemctl show irqbalance | grep Environment` for `IRQBALANCE_BANNED_CPUS`.
  - `/proc/interrupts` to verify interrupt distribution avoids banned CPUs.
- **Disabling**: Remove the `dedicated` and `disableOvsDynamicPinning` fields from the
  PerformanceProfile. The operator will regenerate configurations and trigger a node reboot.

## Infrastructure Needed

No new infrastructure is needed. The existing CI infrastructure for the Node Tuning Operator
and PerformanceProfile e2e tests will be extended to cover the new fields.
