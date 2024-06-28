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

The goal of workload paritioning is to be able to limit the amount of CPU usage of all control plane components. E.g. on a 8 core system, we can limit and gurantee that the control plane is using max 2 cores.

### User Stories

* As a MicroShift administrator, I want to configure MicroShift host and all involved subsystems
  so that I can isolate the control plane services to run on a restricted set of CPUs which reserves rest of the device CPU resources for user's own workloads .


### Goals

Provide guidance and example artifacts for configuring the system for workload partitioning running on MicroShift:
- provide means for configuring CPUSets for the control plane in Microshift.
- Ensure that Microshift controlled resources and services (API, etc.) respect the cpuset configuration and run exclusively on those CPUs.
- Document required Kubelet configuration via pass-through config
- Document required crio configuration .



  
### Non-Goals

- low latency workloads (see [OCPSTRAT-361 /etc/kubernetes/openshift-workload-pinning
](https://issues.redhat.com/browse/OCPSTRAT-361))


## Proposal

To ease configuration of the system for running workload partitioning on MicroShift following
parts need to be put in place:
- Kubelet configuration, microshift manages its own kubelet instance and configuration
- CRI-O configuration, is a doc-only

### Workflow Description

#### System and MicroShift configuration

##### OSTree
1. User supplies configuration files using blueprint
    -  /etc/microshift/config.yaml - passthrought to configure embedded Kubelet
    -  /etc/crio/crio.conf.d/20-microshift-wp.conf - crio configuration for WP
1. User builds the blueprint
1. User deploys the commit / installs the system.
1. System boots

Example blueprint:
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
  reservedSystemCPUs: 0,6,7
"""

[[customizations.files]]
path = "/etc/crio/crio.conf.d/20-microshift-wp.conf"
data = """
kubelet:
  [crio.runtime]
  infra_ctr_cpuset = "0,6,7"

  [crio.runtime.workloads.management]
  activation_annotation = "target.workload.openshift.io/management"
  annotation_prefix = "resources.workload.openshift.io"
  resources = { "cpushares" = 0, "cpuset" = "0,6,7" }
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
    - /etc/crio/crio.conf.d/20-microshift-wp.conf - configuration for crio 
    - /etc/microshift/config.yaml - passthrough configuration for kubelet 
    - /etc/kubernetes/openshift-workload-pinning - 
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


#### CRI-O configuration

add this drop-in to CRI-I with the following example:
```ini
[crio.runtime]
infra_ctr_cpuset = "0,6,7"

[crio.runtime.workloads.management]
activation_annotation = "target.workload.openshift.io/management"
annotation_prefix = "resources.workload.openshift.io"
resources = { "cpushares" = 0, "cpuset" = "0,6,7" }
```

#### Kubelet configuration 
```yaml
reservedSystemCPUs: 0,6,7
```

add to file /etc/kubernetes/openshift-workload-pinning:
```json
{

  "management": {

    "cpuset": "0,6,7"

  }

}
```
#### CRIO Admission Webhook 
Admission Webhook to update the Control Plane POD Annotations  for the CRIO
Introduce cpuset profiles to Microshift configuration


#### Microshift Control Plane cpu pinning
MicroShift runs as a single systemd unit. The main binary embeds as goroutines only those services strictly necessary to bring up a *minimal Kubernetes/OpenShift control and data plane*. 
each of those service should be pinned only to the configured CPUs.
the cpu will be derived from the reservedSystemCPUs configuration.


#### Extra manifests

TBD

### Risks and Mitigations

Biggest risk is system misconfiguration.
CPU starvation for Microshift Control plane will cause service outage.

### Drawbacks

Approach described in this enhancement does not provide much of the NTO's functionality
due to the "static" nature of RPMs and packaged files (compared to NTO's dynamic templating),
but it must be noted that NTO is going beyond Workload partitioning.

One of the NTO's strengths is that it can create systemd units for runtime configuration
(such as offlining CPUs, setting hugepages per NUMA node, clearing IRQ balance banned CPUs,
setting RPS masks). Such dynamic actions are beyond capabilities of static files shipped via RPM.
If such features are required by users, we could ship such systemd units and they could be no-op
unless they're turned on in MicroShift's config. However, it is unknown to author of the enhancement
if these are integral part of the low latency.

## Open Questions [optional]

TBD

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

TBD

## Version Skew Strategy

TBD


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

Other aspect is that NTO is highly generic, supporting many configuration options to mix and match
by the users, Responsibility of dev team is to remove common hurdles from user's path so they make less mistakes
and want to continue using the product.but this enhancement focuses solely on Low Latency.


### Providing users with upstream documentations on how to configure CRI-O 

This is least UX friendly way of providing the functionality.


## Infrastructure Needed [optional]

N/A
