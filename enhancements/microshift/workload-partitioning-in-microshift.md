---
title: workload-partitioning-in-microshift
authors:
  - "@eslutsky"
reviewers:
  - "@sjug, Performance and Scalability expert"
  - "@DanielFroehlich, PM"
  - "@jogeo, QE lead"
  - "@pmtk, working on low latency workloads"
  - "@pacevedom, MicroShift lead"
approvers:
  - "@jerpeter1"
api-approvers:
  - "@jerpeter1"
creation-date: 2024-06-20
last-updated: 2024-06-20
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-409
---

# workload partitioning in Microshift

## Summary

This enhancement describes how workload partitioning will be supported on MicroShift hosts.


## Motivation

In constrained environments, management workloads, including the Microshift control plane, need to be configured to use fewer resources than they might by default in normal clusters.

The goal of workload partitioning is to be able to limit the amount of CPU usage of all control plane components. E.g. on a 8 core system, we can limit and guarantee that the control plane is using max 2 cores.

### User Stories

* As a MicroShift administrator, I want to configure MicroShift host and all involved subsystems
  so that I can isolate the control plane services to run on a restricted set of CPUs which reserves rest of the device CPU resources for user's own workloads .


### Goals

Provide guidance and example artifacts for configuring the system for workload partitioning running on MicroShift:
- Ensure that Microshift embedded goroutines (API, etc.) respect the cpuset configuration and run exclusively on specified CPUs.
- Isolate the control plane containers to run on a restricted set of CPUs.
- Ensure that application payloads that runs on MicroShift runs on Separate dedicated CPUs.


### Non-Goals

- low latency workloads (see [OCPSTRAT-361 Support low latency workload on MicroShift])

## Proposal
currently only ovs-vswitchd and ovsdb-server has CPUAffinity systemd configuration,
which are applied during rpm [installation](https://github.com/openshift/microshift/blob/faa5011fd96913fbb485789822cd77fdba66946f/packaging/rpm/microshift.spec#L286-L287)

the proposal is to extend this functionality to microshift and crio services.

to ease the configuration of the system for running workload partitioning on MicroShift  the following parts need to be put in place:
- crio configuration with cpuset and management annotation.
- systemd configuration with CPUAffinity setting for microshift and crio services.
  > setting systemd CPUAffinity for the microshift service will also propagate  to microshift-etcd.scope unit which is created at runtime.

- kubelet configuration to enable and configure CPU Manager for the workloads.

  
This will limit the cores allowed to run microshift control plane services, maximizing the CPU core for application payloads.

### Workflow Description

#### System and MicroShift configuration

##### OSTree
1. User supplies Microshift configuration  using blueprint
    -  /etc/microshift/config.yaml - embed Kubelet configuration through microshift config.
    - /etc/kubernetes/openshift-workload-pinning - kubelet configuration file.
1. User supplies CRIO configuration file using blueprint.
1. User specify CPUAffinity with systemd drop-in  configuration using blueprint,
   for those services:
    - ovs-vswitchd 
    - ovsdb-server
    - crio
    - Microshift 
1. User builds the blueprint
1. User deploys the commit / installs the system.
1. System boots

-  Example blueprint:

    ```
    name = "microshift-workload-partiontining"
    version = "0.0.1"
    modules = []
    groups = []
    distro = "rhel-94"

    [[packages]]
    name = "microshift"
    version = "4.17.*"

    [[customizations.services]]
    enabled = ["microshift"]

    [[customizations.files]]
    path = "/etc/microshift/config.yaml"
    data = """
    kubelet:
      reservedSystemCPUs: "0,6,7"
      cpuManagerPolicy: static
      cpuManagerPolicyOptions:
        full-pcpus-only: "true"
      cpuManagerReconcilePeriod: 5s
    """

    [[customizations.files]]
    path = "/etc/crio/crio.conf.d/20-microshift-wp.conf"
    data = """
    [crio.runtime]
    infra_ctr_cpuset = "0,6,7"

    [crio.runtime.workloads.management]
    activation_annotation = "target.workload.openshift.io/management"
    annotation_prefix = "resources.workload.openshift.io"
    resources = { "cpushares" = 0, "cpuset" = "0,6,7" }
    """

    [[customizations.files]]
    path = "/etc/systemd/system/ovs-vswitchd.service.d/microshift-cpuaffinity.conf"
    data = """
    [Service]
    CPUAffinity=0,6,7
    """

    [[customizations.files]]
    path = "/etc/systemd/system/ovsdb-server.service.d/microshift-cpuaffinity.conf"
    data = """
    [Service]
    CPUAffinity=0,6,7
    """

    [[customizations.files]]
    path = "/etc/systemd/system/crio.service.d/microshift-cpuaffinity.conf"
    data = """
    [Service]
    CPUAffinity=0,6,7
    """

    [[customizations.files]]
    path = "/etc/systemd/system/microshift.service.d/microshift-cpuaffinity.conf"
    data = """
    [Service]
    CPUAffinity=0,6,7
    """


    [[customizations.files]]
    path = "/etc/kubernetes/openshift-workload-pinning"
    data = """
    {
      "management": {
        "cpuset": "0,6,7"
      }
    }
    """
    ```

##### RPM
1. User creates configuration files with the workload partitioning configuration
1. reboot the host

### API Extensions

Following API extensions are expected:
- A passthrough from MicroShift's config to Kubelet config.


### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

Purely MicroShift enhancement.

### Implementation Details/Notes/Constraints

#### Kubelet configuration 
- instructs kubelet to modify the node resource with the capacity and allocatable CPUs.
  
  add to the file /etc/kubernetes/openshift-workload-pinning
  ```json
  {
    "management": {
      "cpuset": "0,6,7"
    }
  }
  ```
- microshift passthrough configuration for kubelet

  add to file /etc/microshift/config.yaml 
  ```yaml
  kubelet:
    reservedSystemCPUs: 0,6,7
    cpuManagerPolicy: static
    cpuManagerPolicyOptions:
      full-pcpus-only: "true"
    cpuManagerReconcilePeriod: 5s    
  ```
    > `reservedSystemCPUs` -  explicit cpuset for the system/microshift daemons as well as the interrupts/timers, so the rest CPUs on the system can be used exclusively for workloads.

    > `full-pcpus-only` - kubelet configuration setting the CPUManager policy option to `full-pcpus-only` this setting will  guarantee allocation of whole cores to a containers workload ([link](https://github.com/ffromani/enhancements/blob/master/keps/sig-node/2625-cpumanager-policies-thread-placement/README.md#implementation-strategy-of-full-pcpus-only-cpu-manager-policy-option)).
#### CRI-O configuration
- add this drop-in to CRI-I with the following example:
  ```ini
  [crio.runtime]
  infra_ctr_cpuset = "0,6,7"

  [crio.runtime.workloads.management]
  activation_annotation = "target.workload.openshift.io/management"
  annotation_prefix = "resources.workload.openshift.io"
  resources = { "cpushares" = 0, "cpuset" = "0,6,7" }
  ```

- add systemd drop-in for crio

  in the file /etc/systemd/system/crio.service.d/microshift-cpuaffinity.conf
  ```ini
  [Service]
  CPUAffinity=0,6,7
  ```
#### Microshift Control Plane cpu pinning
MicroShift runs as a single systemd unit. The main binary embeds as goroutines only those services strictly necessary to bring up a *minimal Kubernetes/OpenShift control and data plane*. 
microshift will be pinned using systemd [CPUAffinity](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/9/html/managing_monitoring_and_updating_the_kernel/assembly_using-systemd-to-manage-resources-used-by-applications_managing-monitoring-and-updating-the-kernel#proc_using-systemctl-command-to-set-limits-to-applications_assembly_using-systemd-to-manage-resources-used-by-applications) configuration option.

- add systemd drop-in file in /etc/systemd/system/microshift.service.d/microshift-cpuaffinity.conf:
  ```
  [Service]
  CPUAffinity=0,6,7
  ```

- add systemd drop-in for ovs-vswitchd which overwrite the default configuration

  in the file /etc/systemd/system/ovs-vswitchd.service.d/microshift-cpuaffinity.conf
  ```ini
  [Service]
  CPUAffinity=0,6,7
  ```

- add systemd drop-in for ovsdb-server which overwrite the default configuration

  in the file /etc/systemd/system/ovsdb-server.service.d/microshift-cpuaffinity.conf
  ```ini
  [Service]
  CPUAffinity=0,6,7
  ```

### Risks and Mitigations
Biggest risk is system misconfiguration.
CPU starvation for Microshift Control plane will cause service outage.

### Drawbacks
Manual configuration steps required on the system.
Approach described in this enhancement does not provide much of the NTO's functionality
One of the NTO's strengths is that it can create systemd units for runtime configuration.

## Open Questions [optional]

None

## Test Plan

## Graduation Criteria

Feature is meant to be GA on first release.

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

Not applicable.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions

Kubelet configuration will be exposed in MicroShift config as a passthrough.

## Support Procedures

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

### Providing users with upstream documentations on how to configure CRI-O 

This is least UX friendly way of providing the functionality.

## Infrastructure Needed [optional]

N/A
