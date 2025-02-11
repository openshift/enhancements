---
title: ovs-dynamic-cpu-pinning
authors:
  - "@zeeke"
reviewers:
  - "@trozet"
  - "@MarSik"
  - "@ricky-rav"
  - "@cgoncalves"
approvers:
  - "@trozet"
api-approvers:
  - "None"
creation-date: 2023-03-31
last-updated: 2023-03-31
tracking-link:
  - https://issues.redhat.com/browse/CNF-5977
---

# OVS Dynamic CPU Pinning

## Summary

Allow OVS daemons to run on reserved and non-reserved CPUs if the network workload requires
cycles and non-reserved CPU are idle.

## Motivation

OVS runs on reserved CPUs when an OCP node is configured with [cpuManagerPolicy](https://docs.openshift.com/container-platform/4.12/scalability_and_performance/using-cpu-manager.html) set to `static`, and the kubelet is configured with [`reservedSystemCpus`](https://kubernetes.io/docs/tasks/administer-cluster/reserve-compute-resources/#explicitly-reserved-cpu-list).

Typically the users want to minimize the amount of reserved CPUs and leave 
as many available CPUs as possible for business-related workloads.

We have seen in some customer cases that it is possible to disrupt both the node
and the cluster when the networking load rises, because OVS is CPU bound by the 
limited number of system-reserved cores, and networking performance suffers.

### User Stories

> As an Openshift cluster administrator
> I want to maximize my network dataplane bandwidth to all available cores in a dynamically reserved CPU environment
> so that the cluster can endure a high network workload.

### Goals

Every cluster node that is configured with static CPU manager policy and
a low number of `reservedSystemCpus` does not experience any excessive latency 
increase while dealing with high network loads.

### Non-Goals

The logic used by Node Tuning Operator to decide if and when to enable the feature
are out of this enhancement's scope.

## Proposal

As of today, a set of CPUs can be reserved for system workload (i.e. other than Kubernetes workload)
by specifying the `--reserved-cpus` kubelet parameter (or `reservedSystemCpus` via configuration file).

The main idea to resolve the issue described above is to allow OVS 
to spread out to other cpus outside the reserved cpu pool. However 
a care must be taken to not interfere with cpus that are assigned 
to latency sensitive workloads.

### Workflow Description

The feature is triggered by [Node Tuning Operator](https://github.com/openshift/cluster-node-tuning-operator) (NTO) when it detects the 
performance conditions to turn it on. The logic NTO uses to activate this 
feature is out of this enhancement's scope. 
The following steps will be implemented to turn the feature on:
- NTO's daemonset creates a non-empty file in `/etc/openvswitch/enable_dynamic_cpu_affinity` 
  on every node where it wants to enable the pinning feature.
- The file is used by ovnkube-node pod to start a goroutine that constantly monitors 
  the affinity of itself and OVS daemons, aligning the latter if 
  needed (see [CPU affinity alignment](#cpu-affinity-alignment)). The file is the signal for ovnk to enable 
  the feature (see [Activation file](#activation-file))

```mermaid
sequenceDiagram
    Note right of NodeTuningOperator: via MachineConfig
  
    NodeTuningOperator->>ovs-vswitchd: set_slice
    NodeTuningOperator->>ovsdb-server: set_slice
    NodeTuningOperator->>/etc/openvswitch/enable_dynamic_cpu_affinity: fwrite('1')
    activate /etc/openvswitch/enable_dynamic_cpu_affinity
    ovnkube-node ->> /etc/openvswitch/enable_dynamic_cpu_affinity: fstat()
    
    activate ovnkube-node
    loop
    ovnkube-node ->> ovnkube-node: x = sched_getaffinity(os.getPid())
    ovnkube-node ->> ovs-vswitchd: sched_setaffinity(x)
    ovnkube-node ->> ovsdb-server: sched_setaffinity(x)
    ovnkube-node ->> ovsdb-server: sleep(10s)
    end

    deactivate ovnkube-node
    deactivate /etc/openvswitch/enable_dynamic_cpu_affinity
```

### API Extensions

N/A

### Implementation Details/Notes/Constraints

#### CPU affinity alignment

CPU affinity alignment is implemented using system calls SYS_SCHED_SETAFFINITY and SYS_SCHED_GETAFFINITY.
Pods from `ovnkube-node` daemonset run in the Burstable Quality of Service (QoS) class, hence their CPU 
affinities are managed by the [kubelet cpu manager](https://kubernetes.io/docs/tasks/administer-cluster/cpu-management-policies/) and it has access to all cpus that are not being used by guaranteed and pinned pods.

Note that the affinity alignment might take a few second to happen as the CPU reconciler will take a few 
seconds to update the affinity of the ovnkube-node container and then another few seconds until ovnkube-node notices and updates the affinity of OVS daemons. A newly started guaranteed and cpu pinned container might encounter latency spikes during this window.

#### Activation file

The file `/etc/openvswitch/enable_dynamic_cpu_affinity` on the host filesystem is used to 
activate the feature on ovnkube-node according to the following logic:
a. If the file is missing OR the file file is empty, then the feature is turned `OFF`.
b. If the file exists and its content is not an empty string, then the feature is turned `ON`.
c. `ovnkube-node` responds to the file modifications (deletion or content update)
  at runtime, without the need for a restart.

#### Must-Gather enhancement

As the feature is enabled via a file on the host filesystem, it's useful to
add `/etc/openvswitch/enable_dynamic_cpu_affinity` file contents to the [gather_network_logs](https://github.com/openshift/must-gather/blob/master/collection-scripts/gather_network_logs) script.

### Risks and Mitigations

|Risk|Mitigation|
|---|---|
|After NTO enabled the feature, a network performance decrease occurred in some cluster nodes.|The cluster administrator can turn off the feature on specific nodes by connecting through SSH and deleting the activation file. An overriding MachineConfig can be created to achieve the same goal.|


### Drawbacks

This enhancement breaks the hard division between `reservedSystemCPUs` list and CPUs designated
to the Kubernetes workload, for OVS service.

## Design Details

### Test Plan

Following scenario should be covered by automated tests:

_Given_ an OpenShift cluster with a node N with 10 CPUs (0,1,...,9), configured with 
- `cpuManagerPolicy = static`
- `reservedSystemCPUs = 0,1,2,3`

_And_ a running pod with `Guaranteed` QoS running on cpu 4,5

_When_ the administrator creates a the file `/etc/openvswitch/enable_dynamic_cpu_affinity` on N's host filesystem

_Then_ `ovs-switchd` and `ovsdb-server` processes on N has CPU affinity equal to `0,1,2,3,6,7,8,9`.


### Graduation Criteria
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature
### Upgrade / Downgrade Strategy
### Version Skew Strategy

### Operational Aspects of API Extensions

No API changes.

#### Failure Modes

No API changes.

#### Support Procedures

The following MachineConfig resource disable the feature
for every node with `role = worker`:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-disable-ovs-cpu-pinning
spec:
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - contents:
          # Empty file disable the feature
          source: data:text/plain;charset=utf-8;base64,
        mode: 0644
        path: /etc/openvswitch/enable_dynamic_cpu_affinity
        user: {}
```

## Implementation History

## Alternatives

N/A
