---
title: low-latency-workloads-on-microshift
authors:
  - "@pmtk"
reviewers:
  - "@sjug, Performance and Scalability expert"
  - "@DanielFroehlich, PM"
  - "@jogeo, QE lead"
  - "@eslutsky, working on MicroShift workload partitioning"
  - "@pacevedom, MicroShift lead"
approvers:
  - "@jerpeter1"
api-approvers:
  - "@jerpeter1"
creation-date: 2024-06-12
last-updated: 2024-06-12
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-2981
---

# Low Latency workloads on MicroShift

## Summary

This enhancement describes how low latency workloads will be supported on MicroShift hosts.


## Motivation

Some customers want to run latency sensitive workload like software defined PLCs.

Currently it's possible, but requires substantial amount of knowledge to correctly configure
all involved components whereas in OpenShift everything is abstracted using Node Tuning Operator's
PerformanceProfile CR.
Therefore, this enhancement focuses on making this configuration easy for customers by shipping
ready to use packages to kickstart customers' usage of low latency workloads.


### User Stories

* As a MicroShift administrator, I want to configure MicroShift host and all involved subsystems
  so that I can run low latency workloads.


### Goals

Provide guidance and example artifacts for configuring the system for low latency workload running on MicroShift:
- Prepare low latency TuneD profile for MicroShift
- Prepare necessary CRI-O configurations
- Allow configuration of Kubelet via MicroShift config
- Introduce a mechanism to automatically apply a tuned profile upon boot.
- Document how to create a new tuned profile for users wanting more control.


### Non-Goals

- Workload partitioning (i.e. pinning MicroShift control plane components) (see [OCPSTRAT-1068](https://issues.redhat.com/browse/OCPSTRAT-1068))
- Duplicate all capabilities of Node Tuning Operator


## Proposal

To ease configuration of the system for running low latency workloads on MicroShift following
parts need to be put in place:
- `microshift-low-latency` TuneD profile
- CRI-O configuration + Kubernetes' RuntimeClass
- Kubelet configuration (CPU, Memory, and Topology Managers and other)
- `microshift-tuned.service` to activate user selected TuneD profile on boot and reboot the host
  if the kernel args are changed.

New RPM will be created that will contain tuned profile, CRI-O configs, and mentioned systemd daemon.
We'll leverage existing know how of Performance and Scalability team expertise and look at
Node Tuning Operator capabilities.

To allow customization of supplied TuneD profile for specific system, this new profile will
include instruction to include file with variables, which can be overridden by the user.

All of this will be accompanied by step by step documentation on how to use this feature,
tweak values for specific system, and what are the possibilities and limitations.

Optionally, a new subcommand `microshift doctor low-latency` might be added to main
MicroShift binary to provide some verification checks if system configuration is matching
expectations according to our knowledge. It shall not configure system - only report potential problems.


### Workflow Description

Workflow consists of two parts:
1. system and MicroShift configuration
1. preparing Kubernetes manifests for low latency

#### System and MicroShift configuration

##### OSTree

1. User creates an osbuild blueprint:
   - (optional) User configures `[customizations.kernel]` in the blueprint if the values are known
     beforehand. This could prevent from necessary reboot after applying tuned profile.
   - (optional) User adds `kernel-rt` package to the blueprint
   - User adds `microshift-tuned.rpm` to the blueprint
   - User enables `microshift-tuned.service`
   - User supplies additional configs using blueprint:
     - /etc/tuned/microshift-low-latency-variables.conf
     - /etc/microshift/config.yaml to configure Kubelet
     - /etc/microshift/tuned.json to configure `microshift-tuned.service`
1. User builds the blueprint
1. User deploys the commit / installs the system.
1. System boots
1. `microshift-tuned.service` starts (after `tuned.service`, before `microshift.service`):
   - Saves current kernel args
   - Applies tuned `microshift-low-latency` profile
   - Verifies expected kernel args
     - ostree: `rpm-ostree kargs` or checking if new deployment was created[0]
     - rpm: `grubby`
   - If the current and expected kernel args are different, reboot the node
1. Host boots again, everything for low latency is in place,
   `microshift.service` can continue start up.

[0] changing kernel arguments on ostree system results in creating new deployment.

Example blueprint:

```toml
name = "microshift-low-latency"
version = "0.0.1"
modules = []
groups = []
distro = "rhel-94"

[[packages]]
name = "microshift"
version = "4.17.*"

[[packages]]
name = "microshift-tuned"
version = "4.17.*"

[[customizations.services]]
enabled = ["microshift", "microshift-tuned"]

[[customizations.kernel]]
append = "some already known kernel args"
name = "KERNEL-rt"

[[customizations.files]]
path = "/etc/tuned/microshift-low-latency-variables.conf"
data = """
isolated_cores=1-2
hugepagesDefaultSize = 2M
hugepages2M = 128
hugepages1G = 0
additionalArgs = ""
"""

[[customizations.files]]
path = "/etc/microshift/config.yaml"
data = """
kubelet:
  cpuManagerPolicy: static
  memoryManagerPolicy: Static
"""

[[customizations.files]]
path = "/etc/microshift/tuned.json"
data = """
{
  "auto_reboot_enabled": "true",
  "profile": "microshift-low-latency"
}
"""
```


##### bootc

1. User creates Containerfile that:
   - (optional) installs `kernel-rt`
   - installs `microshift-tuned.rpm`
   - enables `microshift-tuned.service`
   - adds following configs
     - /etc/tuned/microshift-low-latency-variables.conf
     - /etc/microshift/config.yaml to configure Kubelet
     - /etc/microshift/tuned.json to configure `microshift-tuned.service`
1. User builds the blueprint
1. User deploys the commit / installs the system.
1. System boots - rest is just like in OSTree flow

Example Containerfile:

```
FROM registry.redhat.io/rhel9/rhel-bootc:9.4

# ... MicroShift installation ...

RUN dnf install kernel-rt microshift-tuned
COPY microshift-low-latency-variables.conf /etc/tuned/microshift-low-latency-variables.conf
COPY microshift-config.yaml                /etc/microshift/config.yaml
COPY microshift-tuned.json                 /etc/microshift/tuned.json

RUN systemctl enable microshift-tuned.service
```

##### RPM

1. User installs `microshift-low-latency` RPM.
1. User creates following configs:
   - /etc/tuned/microshift-low-latency-variables.conf
   - /etc/microshift/config.yaml to configure Kubelet
   - /etc/microshift/tuned.json to configure `microshift-tuned.service`
1. user starts/enables `microshift-tuned.service`:
   - Saves current kernel args
   - Applies tuned `microshift-low-latency` profile
   - Verifies expected kernel args
     - ostree: `rpm-ostree kargs`
     - rpm: `grubby`
   - If the current and expected kernel args are different, reboots the node
1. Host boots again, everything for low latency is in place,
1. User starts/enables `microshift.service`


#### Preparing low latency workload

- Setting `.spec.runtimeClassName: microshift-low-latency` in Pod spec.
- Setting Pod's memory limit and memory request to the same value, and
  setting CPU limit and CPU request to the same value to ensure Pod has guaranteed QoS class.
- Use annotations to get desired behavior
  (unless link to a documentation is present, these annotations only take two values: enabled and disabled):
  - `cpu-load-balancing.crio.io: "disable"` - disable CPU load balancing for Pod 
    (only use with CPU Manager `static` policy and for Guaranteed QoS Pods using whole CPUs)
  - `cpu-quota.crio.io: "disable"` - disable Completely Fair Scheduler (CFS)
  - `irq-load-balancing.crio.io: "disable"` - disable interrupt processing
    (only use with CPU Manager `static` policy and for Guaranteed QoS Pods using whole CPUs)
  - `cpu-c-states.crio.io: "disable"` - disable C-states
    ([see doc for possible values](https://docs.openshift.com/container-platform/4.15/scalability_and_performance/low_latency_tuning/cnf-provisioning-low-latency-workloads.html#cnf-configuring-high-priority-workload-pods_cnf-provisioning-low-latency))
  - `cpu-freq-governor.crio.io: "<governor>"` - specify governor type for CPU Freq scaling (e.g. `performance`) 
    ([see doc for possible values](https://www.kernel.org/doc/Documentation/cpu-freq/governors.txt))


### API Extensions

Following API extensions are expected:
- A passthrough from MicroShift's config to Kubelet config.
- Variables file for TuneD profile to allow customization of the profile for specific host.


### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

Purely MicroShift enhancement.

### Implementation Details/Notes/Constraints

#### TuneD Profile

New `microshift-low-latency` tuned profile will be created and will include existing `cpu-partitioning` profile.

`/etc/tuned/microshift-low-latency-variables.conf` will be used by users to provide custom values for settings such as:
- isolated CPU set
- hugepage count (both 2M and 1G)
- additional kernel arguments

```ini
[main]
summary=Optimize for running low latency workloads on MicroShift
include=cpu-partitioning

[variables]
include=/etc/tuned/microshift-low-latency-variables.conf

[bootloader]
cmdline_microshift=+default_hugepagesz=${hugepagesDefaultSize} hugepagesz=2M hugepages=${hugepages2M} hugepagesz=1G hugepages=${hugepages1G}
cmdline_additionalArgs=+${additionalArgs}
```

```ini
### cpu-partitioning variables
#
# Core isolation
#
# The 'isolated_cores=' variable below controls which cores should be
# isolated. By default we reserve 1 core per socket for housekeeping
# and isolate the rest. But you can isolate any range as shown in the
# examples below. Just remember to keep only one isolated_cores= line.
#
# Examples:
# isolated_cores=2,4-7
# isolated_cores=2-23
#
# Reserve 1 core per socket for housekeeping, isolate the rest.
# Change this for a core list or range as shown above.
isolated_cores=${f:calc_isolated_cores:1}

# To disable the kernel load balancing in certain isolated CPUs:
# no_balance_cores=5-10

### microshift-low-latency variables
# Default hugepages size
hugepagesDefaultSize = 2M

# Amount of 2M hugepages
hugepages2M = 128

# Amount of 1G hugepages
hugepages1G = 0

# Additional kernel arguments
additionalArgs = ""
```

#### `microshift-tuned.service` configuration

Config file to specify which profile to re-apply each boot and if host should be rebooted if
the kargs before and after applying profile are mismatched.

```json
{
  "auto_reboot_enabled": "true",
  "profile": "microshift-low-latency"
}
```

#### CRI-O configuration

```ini
[crio.runtime.runtimes.high-performance]
runtime_path = "/bin/crun"
runtime_type = "oci"
runtime_root = "/bin/crun"
allowed_annotations = ["cpu-load-balancing.crio.io", "cpu-quota.crio.io", "irq-load-balancing.crio.io", "cpu-c-states.crio.io", "cpu-freq-governor.crio.io"]
```

#### Kubelet configuration 

Because of multitude of option in Kubelet configuration, a simple passthrough (copy paste)
will be implemented, rather than exposing every single little configuration variable.

```yaml
# /etc/microshift/config.yaml
kubelet:
  cpuManagerPolicy: static
  cpuManagerPolicyOptions:
    full-pcpus-only: "true"
  cpuManagerReconcilePeriod: 5s
  memoryManagerPolicy: Static
  topologyManagerPolicy: single-numa-node
  reservedSystemCPUs: 0,28-31
  reservedMemory:
  - limits:
      memory: 1100Mi
    numaNode: 0
  kubeReserved:
    memory: 500Mi
  systemReserved:
    memory: 500Mi
  evictionHard:
    imagefs.available: 15%
    memory.available: 100Mi
    nodefs.available: 10%
    nodefs.inodesFree: 5%
  evictionPressureTransitionPeriod: 0s
```
will be passed-through to kubelet config as:
```yaml
cpuManagerPolicy: static
cpuManagerPolicyOptions:
  full-pcpus-only: "true"
cpuManagerReconcilePeriod: 5s
memoryManagerPolicy: Static
topologyManagerPolicy: single-numa-node
reservedSystemCPUs: 0,28-31
reservedMemory:
- limits:
    memory: 1100Mi
  numaNode: 0
kubeReserved:
  memory: 500Mi
systemReserved:
  memory: 500Mi
evictionHard:
  imagefs.available: 15%
  memory.available: 100Mi
  nodefs.available: 10%
  nodefs.inodesFree: 5%
evictionPressureTransitionPeriod: 0s
```

#### Extra manifests

Connects Pod's `.spec.runtimeClassName` to CRI-O's runtime.
If Pod has `.spec.runtimeClassName: microshift-low-latency`,
it can use annotations specified in CRI-O config with `crio.runtime.runtimes.high-performance`.

```yaml
apiVersion: node.k8s.io/v1
handler: high-performance
kind: RuntimeClass
metadata:
  name: microshift-low-latency
```


### Risks and Mitigations

Biggest risk is system misconfiguration.
It is not known to author of the enhancement if there are configurations (like kernel params, sysctl, etc.)
that could brick the device, though it seems rather unlikely.
Even if kernel panic occurs after staging a deployment with new configuration,
thanks to greenboot functionality within the grub itself, the system will eventually rollback to
previous deployment.
Also, it is assumed that users are not pushing new image to production devices without prior verification on reference device.

It may happen that some users need to use TuneD plugins that are not handled by the profile we'll create.
In such case we may investigate if it's something generic enough to include, or we can instruct them
to create new profile that would include `microshift-low-latency` profile.

Systemd daemon we'll provide to enable TuneD profile should have a strict requirement before it
reboots the node, so it doesn't put it into a boot loop.
This pattern of reboot after booting affects the number of "effective" greenboot retries,
so customers might need to account for that by increasing the number of retries.


### Drawbacks

Approach described in this enhancement does not provide much of the NTO's functionality
due to the "static" nature of RPMs and packaged files (compared to NTO's dynamic templating),
but it must be noted that NTO is going beyond low latency.

One of the NTO's strengths is that it can create systemd units for runtime configuration
(such as offlining CPUs, setting hugepages per NUMA node, clearing IRQ balance banned CPUs,
setting RPS masks). Such dynamic actions are beyond capabilities of static files shipped via RPM.
If such features are required by users, we could ship such systemd units and they could be no-op
unless they're turned on in MicroShift's config. However, it is unknown to author of the enhancement
if these are integral part of the low latency.

## Open Questions [optional]

- Verify if osbuild blueprint can override a file from RPM 
  (variables.conf needs to exist for tuned profile, so it's nice to have some fallback)?
- ~~NTO runs tuned in non-daemon one shot mode using systemd unit.~~
  ~~Should we try doing the same or we want the tuned daemon to run continuously?~~
  > Let's stick to default RHEL behaviour. MicroShift doesn't own the OS.
- NTO's profile includes several other beside cpu-partitioning: 
  [openshift-node](https://github.com/redhat-performance/tuned/blob/master/profiles/openshift-node/tuned.conf)
  and [openshift](https://github.com/redhat-performance/tuned/blob/master/profiles/openshift/tuned.conf) - should we include them or incorporate their settings?
- NTO took an approach to duplicate many of the setting from included profiles - should we do the same?
  > Comment: Probably no need to do that. `cpu-partitioning` profile is not changed very often,
  > so the risk of breakage is low, but if they change something, we should get that automatically, right?
- Should we also provide NTO's systemd units for offlining CPUs, setting hugepages per NUMA node, clearing IRQ balance, setting RPS masks?

## Test Plan

Two aspect of testing:
- Configuration verification - making sure that what we ship configures what we need.
  Previously mentioned `microshift doctor low-latency` might be reference point.

- Runtime verification - making sure that performance is as expected.
  This might include using tools such as: hwlatdetect, oslat, cyclictest, rteval, and others.
  Some of the mentioned tools are already included in the `openshift-tests`.
  This step is highly dependent on the hardware, so we might need to long-term lease some hardware in
  Beaker to have consistent environment and results that can be compared between runs.

## Graduation Criteria

Feature is meant to be GA on first release.

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

Not applicable.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

Upgrade / downgrade strategy is not needed because there are almost no runtime components or configs
that would need migration.

User installs the RPM with TuneD profile and configures MicroShift (either manually,
using blueprint, or using image mode) and that exact configuration is applied on boot
and MicroShift start.

For the newly added section in MicroShift config, if it's present after downgrading to previous
MicroShift minor version, the section will be simply ignored because it's not represented in the Go structure.

## Version Skew Strategy

Potentially breaking changes to TuneD and CRI-O:
- Most likely only relevant when RHEL is updated to next major version.
  - To counter this we might want a job that runs performance testing on specified hardware
    to find regressions.
- We might introduce some CI job to keep us updated on changes to NTO's functionality related to TuneD.
  
Changes to Kubelet configuration:
- Breaking changes to currently used `kubelet.config.k8s.io/v1beta1` are not expected.
- Using new version of the `KubeletConfiguration` will require deliberate changes in MicroShift,
  so this aspect of MicroShift Config -> Kubelet Config will not go unnoticed.


## Operational Aspects of API Extensions

Kubelet configuration will be exposed in MicroShift config as a passthrough.


## Support Procedures

To find out any configuration issues:
- Documentation of edge cases and potential pitfalls discovered during implementation.
- `microshift doctor low-latency` command to verify if the pieces involved in tuning host for low latency
  are as expected according to developers' knowledge. Mainly comparing values between different
  config files, verifying that RT kernel is installed and booted, tuned profile is active, etc.

To discover any performance issues not related to missing configuration:
- Adapting some parts of OpenShift documentation.
- Referring user to [Red Hat Enterprise Linux for Real Time](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux_for_real_time/9) documentation.

## Alternatives

### Deploying Node Tuning Operator

Most of the functionality discussed in scope of this enhancement is already handled by Node Tuning
Operator (NTO). However incorporating it in the MicroShift is not the best way for couple reasons:
- NTO depends on Machine Config Operator which also is not supported on MicroShift,
- MicroShift takes different approach to host management than OpenShift,
- MicroShift being intended for edge devices aims to reduce runtime resource consumption and
  introducing operator is against this goal.


### Reusing NTO code

Instead of deploying NTO, its code could be partially incorporated in the MicroShift.
However this doesn't improve the operational aspects: MicroShift would transform a CR into TuneD,
CRI-O config, and kubelet configuration, which means it's still a controller, just running in
different binary and that doesn't help with runtime resource consumption.

Parts that depend on the MCO would need to be rewritten and maintained.

Other aspect is that NTO is highly generic, supporting many configuration options to mix and match
by the users, but this enhancement focuses solely on Low Latency.


### Providing users with upstream documentations on how to use TuneD and configure CRI-O 

This is least UX friendly way of providing the functionality.
Responsibility of dev team is to remove common hurdles from user's path so they make less mistakes
and want to continue using the product.


## Infrastructure Needed [optional]

Nothing.

## Appendix

### Mapping NTO's PerformanceProfile

NTO's PerformanceProfile is transformed into following artifacts (depending on CR's content):
- [tuned profiles](https://github.com/openshift/cluster-node-tuning-operator/tree/master/assets/performanceprofile/tuned)
- [runtime scripts ran using systemd units](https://github.com/openshift/cluster-node-tuning-operator/tree/master/assets/performanceprofile/scripts)
- [static config files, e.g. CRI-O, systemd slices, etc.](https://github.com/openshift/cluster-node-tuning-operator/tree/master/assets/performanceprofile/configs)


Following is PerformanceProfileSpec broken into pieces and documented how each value affects
Kubelet, CRI-O, Tuned, Sysctls, or MachineConfig.

- .CPU
  - .Reserved - CPU set not used for any container workloads initiated by kubelet. Used for cluster and OS housekeeping duties.
    > Relevant for workload partitioning, out of scope for low latency
    - KubeletConfig: .ReservedSystemCPUs, unless .MixedCPUsEnabled=true, then .ReservedSystemCPUs = .Reserved Union .Shared
    - CRI-O:
      - `assets/performanceprofile/configs/99-workload-pinning.conf`
      - `assets/performanceprofile/configs/99-runtimes.conf`: `infra_ctr_cpuset = "{{.ReservedCpus}}"`
    - Sysctl: `assets/performanceprofile/configs/99-default-rps-mask.conf`: RPS Mask == .Reserved
    - Kubernetes: `assets/performanceprofile/configs/openshift-workload-pinning`
    - Tuned: `/sys/devices/system/cpu/cpufreq/policy{{ <<CPU>> }}/scaling_max_freq={{$.ReservedCpuMaxFreq}}`

  - .Isolated - CPU set for the application container workloads. Should be used for low latency workload.
    > Relevant for low latency
    - Tuned: `isolated_cores={{.IsolatedCpus}}`
    - Tuned: `/sys/devices/system/cpu/cpufreq/policy{{ <<CPU>> }}/scaling_max_freq={{$.IsolatedCpuMaxFreq}}`
      > Impossible to do without dynamic templating (each CPU in .Isolated CPU set needs separate line)

  - .BalanceIsolated - toggles if the .Isolated CPU set is eligible for load balancing work loads.
     If `false`, Isolated CPU set is static, meaning workloads have to explicitly assign each thread
     to a specific CPU in order to work across multiple CPUs.
     - Tuned: true -> cmdline isolcpus=domain,managed_irq,${isolated_cores}, otherwise isolcpus=managed_irq,${isolated_cores}
       > Not implemented. Users can use `cpu-load-balancing.crio.io` annotation instead.

  - .Offlined - CPU set be unused and set offline.
    > Out of scope
    - Systemd: unit running `assets/performanceprofile/scripts/set-cpus-offline.sh`

  - .Shared - CPU set shared among guaranteed workloads needing additional CPUs which are not exclusive.
    > User configures in Kubelet config
    - KubeletConfig: if .MixedCPUsEnabled=true, then .ReservedSystemCPUs = .Reserved Union .Shared

- .HardwareTuning
  - Tuned: if !.PerPodPowerManagement, then cmdline =+ `intel_pstate=active`
    > cpu-partitioning sets `intel_pstate=disable`, if user wants different value they can use
    > `additionalArgs` in `microshift-low-latency-variables.conf` - in case of duplicated parameters,
    > last one takes precedence
  - .IsolatedCpuFreq (int) - defines a minimum frequency to be set across isolated cpus
    - Tuned: `/sys/devices/system/cpu/cpufreq/policy{{ <<CPU>> }}/scaling_max_freq={{$.IsolatedCpuMaxFreq}}`
      > Not doable without dynamic templating
  - .ReservedCpuFreq (int) - defines a minimum frequency to be set across reserved cpus
    - Tuned: `/sys/devices/system/cpu/cpufreq/policy{{ <<CPU>> }}/scaling_max_freq={{$.ReservedCpuMaxFreq}}`
      > Not doable without dynamic templating

- .HugePages
  - .DefaultHugePagesSize (string)
    - Tuned: cmdline =+ default_hugepagesz=%s
      > Handled
  - .Pages (slice)
    - .Size
      - Tuned: cmdline =+ hugepagesz=%s
        > Handled
    - .Count
      - Tuned: cmdline =+ hugepages=%d
        > Handled
    - .Node - NUMA node, if not provided, hugepages are set in kernel args
      - If provided, systemd unit running `assets/performanceprofile/scripts/hugepages-allocation.sh` - creates hugepages for specific NUMA on boot
        > Not supported.

- .MachineConfigLabel - map[string]string of labels to add to the MachineConfigs created by NTO.
- .MachineConfigPoolSelector - defines the MachineConfigPool label to use in the MachineConfigPoolSelector of resources like KubeletConfigs created by the operator.
- .NodeSelector - NodeSelector defines the Node label to use in the NodeSelectors of resources like Tuned created by the operator.

- .RealTimeKernel
  > RT is implied with low latency, so no explicit setting like this.
  - .Enabled - true = RT kernel should be installed
    - MachineConfig: .Spec.KernelType = `realtime`, otherwise `default`

- .AdditionalKernelArgs ([]string)
  > Supported
  - Tuned: cmdline += .AdditionalKernelArgs

- .NUMA
  > All of these settings are "exposed" as kubelet config for user to set themselves.
  - .TopologyPolicy (string), defaults to best-effort
    - Kubelet: .TopologyManagerPolicy.
      - If it's `restricted` or `single-numa-node` then also:
        - kubelet.MemoryManagerPolicy = `static`
        - kubelet.ReservedMemory
      - Also, if `single-numa-node`:
        - kubelet.CPUManagerPolicyOptions["full-pcpus-only"] = `true`

- .Net
  > Doing [net] per device would need templating.
  > Doing global [net] is possible, although "Reserved CPU Count"
  > suggests it's for control plane (workload partitioning) hence out of scope.
  - .UserLevelNetworking (bool, default false) - true -> sets either all or specified net devices queue size to the amount of reserved CPUs
    - Tuned: 
      - if .Device is empty, then:
        ```
        [net]
        channels=combined << ReserveCPUCount >>
        nf_conntrack_hashsize=131072
        ```
      - if .Device not empty, then: each device gets following entry in tuned profile:
        ```
        [netN]
        type=net
        devices_udev_regex=<< UDev Regex >>
        channels=combined << ReserveCPUCount >>
        nf_conntrack_hashsize=131072
        ```
  - .Device (slice)
    - .InterfaceName
    - .VendorID
    - .DeviceID

- .GloballyDisableIrqLoadBalancing (bool, default: false) - true: disable IRQ load balancing for the Isolated CPU set.
   false: allow the IRQs to be balanced across all CPUs. IRQs LB can be disabled per Pod CPUs by using `irq-load-balancing.crio.io` and `cpu-quota.crio.io` annotations
   ```
   [irqbalance]
   enabled=false
   ```
   > Not supported (though this is not difficult). Users can use `irq-load-balancing.crio.io: "disable"` annotation.

- .WorkloadHints
  - .HighPowerConsumption (bool)
    - Tuned: cmdline =+ `processor.max_cstate=1 intel_idle.max_cstage=0`
  - .RealTime (bool)
    - MachineConfig: if false, don't add "setRPSMask" systemd or RPS sysctls
      > Not a requirement. Sysctls can be handled with tuned (hardcoded), but systemd unit is out of scope.
    - Tuned: cmdline =+ `nohz_full=${isolated_cores} tsc=reliable nosoftlockup nmi_watchdog=0 mce=off skew_tick=1 rcutree.kthread_prio=11`
      > We can adapt some of kargs, but otherwise users can use `additionalArgs` variable.
  - .PerPodPowerManagement (bool)
    - Tuned: if true: cmdline += `intel_pstate=passive`
      > Users can use `additionalArgs` to override default cpu-partitioning's `intel_pstate=disable`
  - .MixedCPUs (bool) - enables mixed-cpu-node-plugin
    > Seems to be special kind of plugin ([repo](https://github.com/openshift-kni/mixed-cpu-node-plugin)).
    > Not present in MicroShift - not supported.
    - Used for validation: error if: .MixedCPUs == true && .CPU.Shared == "".
    - if true, then .ReservedSystemCPUs = .Reserved Union .Shared

Default values:
- Kubelet
  - .CPUManagerPolicy = `static` (K8s default: none)
  - .CPUManagerReconcilePeriod = 5s (K8s default: 10s)
  - .TopologyManagerPolicy = `best-effort` (K8s default: none)
  - .KubeReserved[`memory`] = 500Mi
  - .SystemReserved[`memory`] = 500Mi
  - .EvictionHard[`memory.available`] = 100Mi (same as Kubernetes default)
  - .EvictionHard[`nodefs.available`] = 10% (same as Kubernetes default)
  - .EvictionHard[`imagefs.available`] = 15% (same as Kubernetes default)
  - .EvictionHard[`nodefs.inodesFree`] = 5% (same as Kubernetes default)

- MachineConfig:
  - Also runs `assets/performanceprofile/scripts/clear-irqbalance-banned-cpus.sh`
    > Unsupported.
