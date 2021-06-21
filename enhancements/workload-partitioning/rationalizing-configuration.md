---
title: rationalizing-configuration
authors:
  - "@dhellmann"
  - "@mrunalp"
  - "@browsell"
  - "@MarSik"
  - "@fromanirh"
reviewers:
  - "@deads2k"
  - "@sttts"
  - maintainers of PAO
  - maintainers of MCO
approvers:
  - "@derekwaynecarr"
creation-date: 2021-04-26
status: implementable
see-also:
  - "/enhancements/management-workload-partitioning.md"
---

# Rationalizing Configuration of Workload Partitioning

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The initial iteration of workload partitioning focused on a short path
to a minimum viable implementation. This enhancement describes the
loose ends for preparing the feature for GA at a high level, and
explains the set of other design documents that need to be written
separately during the next iteration.

## Motivation

The management workload partitioning feature introduced in OCP 4.8
enables pod workloads to be pinned to the kubelet reserved cpuset. The
feature is enabled at install time and requires the reserved set of
CPUs as input. That configuration is currently passed in via a
manually created manifest containing a machine config resource. The
set of CPUs used for management workload partitioning has to align
with the reserved set configured by the performance-addon-operator
(PAO) during day 2. For OCP 4.8, the onus is on the user to ensure
these align, there is no interlock.  The current implementation is
prone to user error because it has two entities configuring and
managing the same data.

The 4.8 implementation is also limited to single-node deployments. We
anticipate users of highly-available clusters also wanting to minimize
the overhead of the management components to take full advantage of
compute capacity for their own workloads.

This design document describes phase 2 of the implementation plan for
workload partitioning to improve the user experience when configuring
the feature and to extend support to multi-node clusters.

### Goals

1. Describe, at a high level, the remaining work to improve the user
   experience and expand support to multi-node deployments for
   architectural review and planning.
2. Document the division of responsibility between workload management
   in components in the OpenShift release payload and PAO.
3. List the other enhancements that need to be written, and their
   scopes.
4. Minimize the complexity of installing a "normal" cluster while
   still supporting this very special case.

### Non-Goals

1. Resolve all of the details for each of the next steps. We're not
   going to talk a lot about names, API fields, or implementation
   details here.
2. Describe an API for (re)configuring workload partitioning in a
   running cluster. We are going to continue to require the user to
   enable partitioning as part of deploying the cluster for another
   implementation phase.
3. Support for external control plane topologies.

## Proposal

### API

The initial implementation is limited to single-node deployments. To
support multi-node clusters, we need to solve a couple of concurrency
problems. The first is providing a way for the admission hook
responsible for mutating pod definitions to reliably know when the
feature should be enabled. The admission hook cannot store state on
its own, so we need a new API to indicate when the feature is
enabled.

We eventually want to extend the partitioning feature to support
non-management workloads to include workload types defined by cluster
admins. For now, however, we want to keep the interface as simple as
possible.  Therefore, the new API will only have status fields and
will only be used so that the admission hook (and other consumers) can
tell when workload partitioning is active.

To manage the risk associated with rolling out a complicated feature
like this, we are going to continue to require it to be enabled as
part of deploying the cluster. The API does not need to support
configuring the workload partitioning in a running cluster, so no
controller is needed, for now.

We propose that the API be owned by core OpenShift, and defined in the
`openshift/api` repository. A separate enhancement will be written to
describe the API in more detail.

### Owner for Enablement

Workload management needs two node configuration steps which overlap
with work PAO is doing today:

* Set reserved CPUs in kubelet config and enable the CPU manager
  static policy
* Set systemd affinity to match the reserved CPU set

PAO already has logic for managing CPU pools and configuring kubelet
and CRI-O. Since it already does a lot of what is needed to enable
workload partitioning, we propose to extend it to handle the
additional configuration currently being done manually by users,
including the machine config with settings for kubelet and CRI-O.

The effect of this decision is that the easiest way to use the
workload partitioning feature will be through the
performance-addon-operator. This means that the implementation of the
feature is split across a few components, which avoids duplicating a
lot of the work already done in the PAO.

Workload partitioning is currently enabled as part of installing a
cluster, and that will not change as part of the implementation of
this design. Enabling partitioning early in this way provides
predictable behavior from the beginning of the life-cycle, and avoids
extra reboots required to enable it in a running cluster (an important
consideration in environments with short, fixed maintenance
windows). To continue to support enabling the feature during
deployment, and to simplify the configuration, we propose to extend
the PAO with a `render` command, similar to the one in other
operators, to have it generate manifests for the OpenShift installer
as extra manifests. This avoids any need to change the OpenShift installer to
support this uncommon use case, and is consistent with the way
third-party networking solutions are installed.

The `render` command will have an option to enable the management
partition. When the option is given, the `render` command will create
a manifest for the new enablement API with status data that includes
the `management` workload type to enable partitioning (the details of
that API will be defined in a separate design document). In the
future, the PAO might enable partitioning for all of the types
mentioned in the input PerformanceProfiles, or based on some other
input.

For each PerformanceProfile, the PAO will render MachineConfig
manifests with higher precedence than the default ones created by the
installer during bootstrapping. These manifests will include all of
the details for configuring `kubelet`, CRI-O, and `tuned` to know
about and partition workloads, including the cpusets for each workload
type.

The PAO `render` command also generates a manifest with the CRD for
PerformanceProfile so bootstrapping does not block when trying to
install the PerformanceProfile manifests.

### Future Work

1. Write an enhancement to describe the API the admission hook will
   use to determine when workload partitioning is enabled.
2. Write an enhancement to describe the changes in
   performance-addon-operator to manage the kubelet and CRI-O
   configuration when workload partitioning is enabled, including the
   `render` command. See https://issues.redhat.com/browse/CNF-2164 to
   track that work.

### User Stories

#### Enabling and Configuring at the Same Time

In this workflow, the user provides all of the information needed to
fully partition the workloads from the very beginning.

1. User runs the installer to `create manifests` to get standard
   manifests without any partitioning.
2. The user creates PerformanceProfile manifests for each known
   MachineConfigPool and adds them to the installer inputs.
3. The user runs the PAO `render` command to read the
   PerformanceProfile manifests and create additional manifests, as
   described above.
4. User runs the installer to `create cluster`.
5. Installer bootstraps the cluster.
6. Bootstrapping runs the machine-config-operator (MCO) `render`
   command, which generates MachineConfig manifests with low
   precedence.
7. Bootstrapping uploads both sets of MachineConfig manifests, one
   after the other.
8. The machine-config-operator (MCO) applies MachineConfigs to nodes
   in the cluster.
9. Bootstrapping finishes and the cluster is launched.
10. If the MCO does not apply KubeletConfig in step 8, it must do it
    here.
11. The cluster has complete partitioning configuration for management
    workloads.
12. Kubelet starts and sees the config file enabling partitioning
    (delivered in the MachineConfig manifest generated by
    PAO). Kubelet advertises the workload resource type on the Node
    and mutates static pods with partitioning annotations.
13. The admission plugin uses the workload types on the new enablement
    API to decide when to mutate pods.
14. CRI-O sees pods with workload annotations and uses the resource
    request to set cpushares and cpuset.
15. On day 2 PAO, is installed and takes ownership of the
    PerformanceProfile CRs.

#### Enabling During Installation, Configuring Later

In this workflow, the user provides enough information to *enable*
workload partitioning but not enough to actually *configure* all nodes
to partition the workloads into specific CPUs.

1. User runs the installer to `create manifests` to get standard
   manifests without any partitioning.
2. The user runs the PAO `render` command without any
   PerformanceProfile manifests to create the additional manifests, as
   described above. The manifests for CRI-O do not include cpuset
   configuration, because there are no PerformanceProfiles.
3. User runs the installer to `create cluster`.
4. Installer bootstraps the cluster.
5. Bootstrapping runs the machine-config-operator (MCO) `render`
   command, which generates MachineConfig manifests with low
   precedence.
6. Bootstrapping uploads both sets of MachineConfig manifests, one
   after the other.
7. The machine-config-operator (MCO) applies MachineConfigs to nodes
   in the cluster.
8. Bootstrapping finishes and the cluster is launched.
9. If the MCO does not apply KubeletConfig in step 7, it must do it
   here.
11. The cluster has partial partitioning enabled for `management`
    workloads. Pods for management workloads are mutated, but not
    actually partitioned.
12. Kubelet starts and sees the config file enabling partitioning. It
    advertises the workload resource type on the Node and mutates
    static pods with partitioning annotations.
13. Admission plugin uses the workload types on the workloads CR to
    decide when to mutate pods.
14. CRI-O sees pods with workload annotations and uses the resource
    request to set cpushares but not cpuset. See the [Risks
    section](#risks-and-mitigations) for details of the effects this
    may have on the cluster.
15. User installs the performance-addon-operator into the cluster.
16. User creates PerformanceProfiles and adds them to the cluster.
17. PAO generates new MachineConfigs, adding the cpuset information
    from the PerformanceProfiles to the other partitioning info from
    the new enablement API.
18. MCO rolls out new MachineConfigs, rebooting nodes as it goes.
19. The cluster has complete partitioning configuration for management
    workloads and the management workloads are partitioned due to the
    nodes rebooting.
20. Kubelet starts and sees the config file enabling partitioning. It
    advertises the workload resource type on the Node and mutates
    static pods with partitioning annotations.
21. Admission plugin uses the workload types on the workloads CR to
    decide when to mutate pods.
22. CRI-O sees pods with workload annotations and uses the resource
    request to set cpushares and cpuset.

#### Enabling and Configuring Through the Assisted Installer Workflow

This section describes how a user will enable and configure workload
partitioning for clusters deployed using the assisted installer
workflow. We expect this workflow to be the most common approach used,
especially for bulk deployments.

1. The user creates PerformanceProfile manifests for each known
   MachineConfigPool name

   * The MachineConfig (MC) for the generic worker MachineConfigPool
     (MCP) that includes partitioning without cpusets
   * Other MCPs inherit from the worker MCP and include partitioning
     with cpusets

2. *something* runs the PAO `render` command to read the
   PerformanceProfile manifests and create additional manifests, as
   described above.

   * The user may perform this step, or an orchestration system
     managing the assisted installer workflow automatically may run
     it.

3. User generates a set of assisted installer CRs to deploy the
   cluster.
4. The assisted installer services start the cluster installation with
   the artifacts from steps 1-3 as input (PerformanceProfile CRD,
   PerformanceProfiles, MachineConfig, and enablement API).
5. The assisted installer (or hive?) invokes the installer to `create
   cluster`.
6. The installer bootstraps the cluster.
7. Bootstrapping runs the machine-config-operator (MCO) `render`
   command, which generates MachineConfig manifests with low
   precedence.
8. Bootstrapping uploads both sets of MachineConfig manifests, one
   after the other.
9. The machine-config-operator (MCO) applies MachineConfigs to nodes
   in the cluster.
10. Bootstrapping finishes and the cluster is launched.
11. If the MCO does not apply KubeletConfig in step 9, it must do it
    here.
12. The cluster has complete partitioning configuration for management
    workloads.
13. Kubelet starts and sees the config file enabling partitioning
    (delivered in the MachineConfig manifest generated by
    PAO). Kubelet advertises the workload resource type on the Node
    and mutates static pods with partitioning annotations.
14. The admission plugin uses the workload types on the new enablement
    API to decide when to mutate pods.
15. CRI-O sees pods with workload annotations and uses the resource
    request to set cpushares and cpuset.
16. On day 2 PAO, is installed and takes ownership of the
    PerformanceProfile CRs.

#### Day 2: Modify Reserved cpuset for a PerformanceProfile

In a cluster with partitioning enabled and fully configured, modifying
the reserved cpuset for a PerformanceProfile is safe because the node
is rebooted when the new MachineConfig is applied.

1. The user updates the cpuset in the PerformanceProfile(s).
2. PAO generates new MachineConfigs, adding the cpuset information
   from the PerformanceProfiles to the other partitioning info from
   the new enablement API.
3. The MCO rolls out new MachineConfigs, rebooting nodes as it goes.
4. The cluster has complete partitioning configuration for management
   workloads and the management workloads are partitioned due to the
   nodes rebooting.
5. Kubelet starts and sees the config file enabling partitioning. It
   advertises the workload resource type on the Node and mutates
   static pods with partitioning annotations.
6. Admission plugin uses the workload types on the workloads CR to
   decide when to mutate pods.
7. CRI-O sees pods with workload annotations and uses the resource
   request to set cpushares and cpuset.

#### Day 2: Add a New Node to an Existing MachineConfigPool

In a cluster with partitioning enabled and fully configured, adding a
new node to an existing MachineConfigPool is safe because the
MachineConfig will set up kubelet and CRI-O on the host correctly.

1. The MCO applies the MachineConfigs to the node.
2. Kubelet starts and sees the config file enabling partitioning. It
   advertises the workload resource type on the Node and mutates
   static pods with partitioning annotations.
3. CRI-O sees pods with workload annotations and uses the resource
   request to set cpushares and cpuset.

#### Day 2: Add a New MachineConfigPool

Adding a new MachineConfigPool is safe only if the MachineConfigPool
has a PerformanceProfile associated and the PAO is installed.

1. PAO reads PerformanceProfile to generate new MachineConfig for the
   pool with CRI-O and `kubelet` settings.
2. MachineConfigs are applied to appropriate nodes, rebooting them in
   the process.
3. Kubelet starts and sees the config file enabling partitioning. It
   advertises the workload resource type on the Node and mutates
   static pods with partitioning annotations.
4. CRI-O sees pods with workload annotations and uses the resource
   request to set cpushares and cpuset.

If the MachineConfigPool does not match a PerformanceProfile, there
will be no cpuset information and the PAO will generate MachineConfigs
with partitioning enabled but not tied to a cpuset. See the [Risks
section](#risks-and-mitigations) for details.

If the PAO is not installed, it will not generate the overriding
MachineConfigs for the new MachineConfigPool. See the [Risks
section](#risks-and-mitigations) for details.

### Risks and Mitigations

There is some risk of shipping a feature with part of the
implementation in the OpenShift release payload but the enabling tool
delivered separately. PAO is considered to be a "core" OpenShift
component, even though it is delivered separately. There is not a
separate SKU for it, for example. The PAO team has been working
closely with the node team on the implementation of this feature, so
we do not anticipate any issues delivering the finished work in this
way.

If a MachineConfigPool exists without a matching PerformanceProfile,
there will be no cpuset information and the PAO will generate
MachineConfigs with partitioning enabled but not tied to a cpuset.
This is somewhat safe, but may lead to unexpected behavior. The
mutated pods would float across the full cpuset in the same way as if
partitioning was not enabled and the cpushares would not be deducted
from available CPUs, potentially leading to over commit scenarios.

If the PAO is not installed into a cluster with workload partitioning
enabled and configured, adding a new MachineConfigPool can be unsafe.
The nodes in the pool will not enable partitioning at all. This may
not be safe, since kubelet will not advertise the workload resource
type but the admission plugin will mutate pods to require it. The
scheduler may refuse to place management workloads on the nodes in the
pool.

## Design Details

### Open Questions

1. What runs the PAO `render` command in the assisted installer
   workflow?
2. How hard do we need to work to ensure that the PAO version matches
   the release payload version for the cluster being deployed?
   What/who is responsible for that?
3. We need to ensure MCO render handles the KubeletConfig CR generated
   by PAO so kubelet has partitioning enabled without requiring an
   extra reboot.

### Test Plan

The other enhancements will provide details of the test plan(s)
needed.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

- https://issues.redhat.com/browse/OCPPLAN-6065
- https://issues.redhat.com/browse/CNF-2084

## Drawbacks

None

## Alternatives

We could move more of the PAO implementation into the release payload,
so that users who want the workload partitioning feature do not need
another component. This would either duplicate a lot of the existing
PAO work or make future deliver of PAO updates more complicated by
tying them to OpenShift releases.
