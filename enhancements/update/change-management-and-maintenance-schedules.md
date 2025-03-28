---
title: change-management-and-maintenance-schedules
authors:
  - "@jupierce"
reviewers: 
  - "@wking"
  - "@petr-muller"
  - "@sinnykumari"
  - "@jewzaam"
approvers: 
  - "@sdodson"
  - "@jharrington22"
  - "@yuqi-zhang"
  - "@sjenning"
api-approvers:
  - "@joelspeed"
creation-date: 2024-02-29
last-updated: 2024-11-05

tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1696
  - https://issues.redhat.com/browse/OCPSTRAT-1695
---

# Change Management and Maintenance Schedules

## Summary
Implement high level APIs for change management which allow
standalone and Hosted Control Plane (HCP) clusters a measure of configurable control
over when control-plane or worker-node configuration rollouts are initiated. 
As a primary mode of configuring change management, implement a strategy
called Maintenance Schedules which define reoccurring windows of time (and specifically
excluded times) in which potentially disruptive changes in configuration can be initiated. 

Material changes not permitted by change management configuration are left in a 
pending state until such time as they are permitted by the configuration. 

Change management enforcement _does not_ guarantee that all initiated
material changes are completed by the close of a permissive window (e.g. a worker-node
may still be draining or rebooting),but it does prevent _additional_ material 
changes from being initiated. 

A "material change" may vary by cluster profile and subsystem. For example, a 
control-plane update (all components and control-plane nodes updated) is implemented as
a single material change (e.g. the close of a scheduled permissive window
will not suspend its progress). In contrast, the rollout of worker-node updates is
more granular (you can consider it as many individual material changes) and  
the end of a permitted change window will prevent additional worker-node updates 
from being initiated.

Non-disruptive changes vital to the continued operation of the cluster (e.g. certificate rotation) 
are not considered material changes. Ignoring operational practicalities (e.g.
the need to fix critical bugs or update a cluster to supported software versions), 
it should be possible to safely leave changes pending indefinitely. That said,
Service Delivery and/or higher level management systems may choose to prevent
such problematic change management settings from being applied by using 
validating webhooks or admission policies.

## Definitions and Reference

### Recurrence Rules
RRULE, short for "Recurrence Rule", is an RFC https://icalendar.org/RFC-Specifications/iCalendar-RFC-5545/
commonly used to express reoccurring windows of time for calendar data interchange. Consider a calendar 
invite for a meeting that should occur on the last Friday of every month. RRULE can express 
this as `FREQ=MONTHLY;INTERVAL=1;BYDAY=-1FR`. 
Tool for generating / interpreting RRULES: https://jkbrzt.github.io/rrule/

RRULE served as an inspiration for the maintenance schedule recurrence rule definition schema, so 
a foundational understanding of RRULE may help users decipher maintenance schedules. Examples
of the schema:

**Example**: Every third day
As RRULE: `RRULE:FREQ=DAILY;INTERVAL=3`
Through enhancement schema:
```yaml
recurrence:
  frequency: Daily
  daily:
    interval: 3
```

**Example**: Every Saturday and Sunday
As RRULE: `FREQ=WEEKLY;BYDAY=SA,SU`
Through enhancement schema:
```yaml
recurrence:
  frequency: Weekly
  weekly:
    daysOfWeek:
    - Saturday
    - Sunday
    interval: 1
```

**Example**: The last Monday of each month
As RRULE: `FREQ=MONTHLY;BYDAY=MO;BYSETPOS=-1`
Through enhancement schema:
```yaml
recurrence:
  frequency: Monthly
  monthly:
    by: Day
    day:
      days:
      - daysOfWeek: Monday
        weekOfMonth: Last
      interval: 1
```

### Change Management Terminology
This document uses specialized terms to describe the key aspects of change management. 
It is worth internalizing the meaning of these terms before reviewing sections of the document.
- "Material Change". A longer definition is provided in the Summary, but, in short, any configuration
  change a platform operator wishes to apply which would necessitate the disruption, reboot, or replacement of one 
  or more nodes is considered a material change. For example, updating the control-plane version is 
  a material change as it requires rebooting master nodes. 
- "Permissive". When change management is in permissive state, reconciliation can initiate material changes
  on the cluster. Versions of OpenShift without change management can be considered to always have their
  resources in a permissive state. For example, changing a MachineConfig immediately begins the process
  of reconciling nodes - potentially draining and rebooting them and disrupting workloads.
- "Restrictive". The opposite of permissive. Reconciliation can be considered "paused" and pending material 
  changes will not be initiated.
- "Strategy". There are different change management strategies proposed. Each informs a different behavior
  for a controller to drive permissive and restrictive states. 
- "Maintenance Schedule" is one change management strategy. When enabled, based on a recurrence
  rule-like ([RRULE](https://icalendar.org/RFC-Specifications/iCalendar-RFC-5545/)) specification and exclusion periods, 
  a change management policy will be permissive according to the current datetime.

## Motivation
This enhancement is designed to improve user experience during the OpenShift 
upgrade process and other key operational moments when configuration updates
may result in material changes in cluster behavior and potential disruption 
for non-HA workloads.

The enhancement offers a direct operational tool to users while indirectly
supporting a longer term separation of control-plane and worker-node updates
for **Standalone** cluster profiles into distinct concepts and phases of managing 
an OpenShift cluster (HCP clusters already provide this distinction). The motivations
for both aspects will be covered, but a special focus will be made on the motivation
for separating Standalone control-plane and worker-node updates as, while not fully realized 
by this enhancement alone, ultimately provides additional business value helping to 
justify an investment in the new operational tool.

### Supporting the Eventual Separation of Control-Plane and Worker-Node Updates
One of the key value propositions of this proposal pre-supposes a successful
decomposition of the existing, fully self-managed, Standalone update process into two  
distinct phases as understood and controlled by the end-user: 
(1) control-plane update and (2) worker-node updates.

To some extent, Maintenance Schedules (a key strategy supported for change management) 
are a solution to a problem that will be created by this separation: there is a perception that it would also
double the operational burden for users updating a cluster (i.e. they have
two phases to initiate and monitor instead of just one). In short, implementing the
Maintenance Schedules concept allows users to succinctly express if and how
they wish to differentiate these phases.

Users well served by the fully self-managed update experience can avoid 
change management (i.e. not set an enforced maintenance schedule), specifying
that control-plane and worker node updates can take place at
any time. Users who need more control may choose to update their control-plane
regularly (e.g. to patch CVEs) with a permissive change management configuration
for the control-plane while using a tight maintenance schedule for worker-nodes
to only update during specific, low utilization, periods.

Since separating the node update phases is such an important driver for
Maintenance Schedules, their motivations are heavily intertwined. The remainder of this 
section, therefore, delves deeply into the motivation for this separation. 

#### The Case for Control-Plane and Worker-Node Separation
From an overall platform perspective, we believe it is important to drive a distinction
between updates of the control-plane and worker-nodes. Currently, an update is initiated
and OpenShift's ostensibly fully self-managed update mechanics take over (CVO laying
out new manifests, cluster operators rolling out new operands, etc.) culminating with 
worker-nodes being drained a rebooted by the machine-config-operator (MCO) to align
them with the version of OpenShift running on the control-plane.

This approach has proven extraordinarily successful in providing a fast and reliable 
control-plane update, but, in rare cases, the highly opinionated update process leads
to less than ideal outcomes. 

##### Node Update Separation to Address Problems in User Perception
Our success in making OpenShift control-plane updates reliable, exhaustive focus on quality aside,
is also made possible by the platform's exclusive ownership of the workloads that run on the control-plane 
nodes. Worker-nodes, on the other hand, run an endless variety of non-platform, user defined workloads - many of
which are not necessarily perfectly crafted. For example, workloads with pod disruption budgets (PDBs) that
prevent node drains and workloads which are not fundamentally HA (i.e. where draining part of the workload creates
disruption in the service it provides). 

Ultimately, we cannot solve the issue of problematic user workload configurations because
they are intentionally representable with Kubernetes APIs (e.g. it may be the user's actual intention to prevent a pod
from being drained, or it may be too expensive to make a workload fully HA). When confronted with 
problematic workloads, the current, fully self-managed, OpenShift update process can appear to the end-user
to be unreliable or slow. This is because the self-managed update process takes on the end-to-end responsibility
of updating the control-plane and worker-nodes. Given the automated and somewhat opaque nature of this
update, it is reasonable for users to expect that the process is hands-off and will complete in a timely 
manner regardless of their workloads.  

When this expectation is violated because of problematic user workloads, the update process is 
often called into question. For example, if an update appears stalled after 12 hours, a
user is likely to have a poor perception of the platform and open a support ticket before
successfully diagnosing an underlying undrainable workload.

By separating control-plane and worker-node updates into two distinct phases for an operator to consider,
we can more clearly communicate (1) the reliability and speeed of OpenShift control-plane updates and
(2) the shared responsibility, along with the end user, of successfully updating worker-nodes. 

As an analogy, when you are late to work because of delays in a subway system, you blame the subway system.
They own the infrastructure and schedules and have every reason to provide reliable and predictable transport.
If, instead, you are late to work because you step into a fully automated car that gets stuck in traffic, you blame the
traffic. The fully self-managed update process suggests to the end user that it is a subway -- subtly insulating
them from the fact that they might well hit traffic (problematic user workloads). Separating the update journey into
two parts - a subway portion (the control-plane) and a self-driving car portion (worker-nodes), we can quickly build the
user's intuition about their responsibilities in the latter part of the journey. For example, leaving earlier to
avoid traffic or staying at a hotel after the subway to optimize their departure for the car ride.

##### Node Update Separation to Improve Risk Mitigation Strategies
With any cluster update, there is risk --  software is changing and even subtle differences in behavior can cause
issues given an unlucky combination of factors. Teams responsible for cluster operations are familiar with these
risks and owe it to their stakeholders to minimize them where possible.

The current, fully self-managed, update process makes one obvious risk mitigation strategy
a relatively advanced strategy to employ: only updating the control-plane and leaving worker-nodes as-is.
It is possible by pausing machine config pools, but this is certainly not an intuitive step for users. Farther back 
in OpenShift 4's history, the strategy was not even safe to perform since it could lead to worker-node 
certificates expiring. 

By separating the control-plane and worker-node updates into two separate steps, we provide a clear
and intuitive method of deferring worker-node updates: not initiating them. Leaving this to the user's
discretion, within safe skew-bounds, gives them the flexibility to make the right choices for their
unique circumstances.

It should also be noted that Service Delivery does not permit customers to directly modify machine config
pools. This means that the existing machine config pool based pause is not directly available. It is 
not exposed via OCM either. This enhancement seeks to create high level abstractions supporting the 
separation of control-plane and worker-nodes that will be straight-forward and intuitive options
to expose through OCM.

#### Enhancing Operational Control
The preceding section delved deeply into a motivation for Change Management / Maintenance Schedules based on our desire to 
separate control-plane and worker-node updates without increasing operational burden on end-users. However,
Change Management, by providing control over exactly when updates & material changes to nodes in
the cluster can be initiated, provide value irrespective of this strategic direction. The benefit of
controlling exactly when changes are applied to critical systems is universally appreciated in enterprise 
software. 

Since these are such well established principles, the motivation can be summarized as helping
OpenShift meet industry standard expectations with respect to limiting potentially disruptive changes 
outside well planned time windows. 

It could be argued that rigorous and time sensitive management of OpenShift API resources 
(e.g. ClusterVersion, MachineConfigPool, HostedCluster, NodePool, etc.) could prevent
unplanned material changes, but Change Management / Maintenance Schedules introduce higher level, platform native, and 
intuitive guard rails. For example, consider the common pattern of a gitops configured OpenShift cluster.
If a user wants to introduce a change to a MachineConfig, it is simple to merge a change to the
resource without appreciating the fact that it will trigger a rolling reboot of nodes in the cluster.

Trying to merge this change at a particular time of day and/or trying to pause and unpause a 
MachineConfigPool to limit the impact of that merge to a particular time window requires
significant forethought by the user. Even with that forethought, if an enterprise wants 
changes to only be applied during weekends, additional custom mechanics would need
to be employed to ensure the change merged during the weekend without needing someone present.
Even this approach is unavailable to our managed services customers who are restricted
from modifying machine config pools directly.

Contrast this complexity with the user setting a Change Management / Maintenance Schedule 
on the cluster (or indirectly via OCM when Service Delivery exposes the option for managed clusters). 
The user is then free to merge configuration changes and gitops can apply those changes to OpenShift
resources, but material change to the cluster will not be initiated until a time permitted
by the Maintenance Schedule. Users do not require special insight into the implications of
configuring platform resources as the top-level Maintenance Schedule control will help ensure
that potentially disruptive changes are limited to well known time windows.

#### Reducing Service Delivery Operational Tooling
Service Delivery, operating Red Hat's Managed OpenShift offerings (OpenShift Dedicated (OSD), 
Red Hat OpenShift on AWS (ROSA) and Azure Red Hat OpenShift (ARO) ) is keenly aware of
the challenges motivating the Change Management / Maintenance Schedule concept. This is evidenced by their design
and implementation of tooling to fill the gaps in the platform the preceding sections
suggest exist.

Specifically, Service Delivery has developed UXs outside the platform which allow customers 
to define a preferred maintenance window. For example, when requesting an update, the user 
can specify the desired start time. This is honored by Service Delivery tooling (unless
there are reasons to supersede the customer's preference).

By acknowledging the need for scheduled maintenance in the platform, we reduce the need for Service
Delivery to develop and maintain custom tooling to manage the platform while 
simultaneously simplifying management for all customer facing similar challenges.

### User Stories
For readability, "cluster lifecycle administrator" is used repeatedly in the user stories. This 
term can apply to different roles depending on the cluster environment and profile. In general,
it is the person or team making most material changes to the cluster - including planning and
choosing when to enact phases of the OpenShift platform update.

For HCP, the role is called the [Cluster Service Consumer](https://hypershift-docs.netlify.app/reference/concepts-and-personas/#personas). For
Standalone clusters, this role would normally be filled by one or more `system:admin` users. There
may be several layers of abstraction between the cluster lifecycle administrator and changes being
actuated on the cluster (e.g. gitops, OCM, Hive, etc.), but the role will still be concerned with limiting
risks and disruption when rolling out changes to their environments.

> "As a cluster lifecycle administrator, I want to ensure any material changes to my cluster 
> (control-plane or worker-nodes) are only initiated during well known windows of low service 
> utilization to reduce the impact of any service disruption."

> "As a cluster lifecycle administrator, I want to ensure any material changes to my 
> control-plane are only initiated during well known windows of low service utilization to 
> reduce the impact of any service disruption."

> "As a cluster lifecycle administrator, I want to ensure that no material changes to my 
> cluster occur during a known date range even if it falls within our
> normal maintenance schedule due to an anticipated atypical usage (e.g. Black Friday)."

> "As a cluster lifecycle administrator, I want to restrict additional material changes from 
> taking place when it is no longer practical to monitor for service disruptions. For example,
> if a worker-node update is proving to be problematic during a valid permissive window, I would
> like to be able to pause that change manually so that the team will not have to work on the weekend."

> "As a cluster lifecycle administrator, I need to stop all material changes on my cluster
> quickly and indefinitely until I can understand a potential issue. I do not want to consider dates or
> timezones in this delay as they are not known and irrelevant to my immediate concern."

> "As a cluster lifecycle administrator, I want to ensure any material changes to my 
> control-plane are only initiated during well known windows of low service utilization to 
> reduce the impact of any service disruption. Furthermore, I want to ensure that material
> changes to my worker-nodes occur on a less frequent cadence because I know my workloads
> are not HA."

> "As an SRE, tasked with performing non-emergency corrective action, I want 
> to be able to apply a desired configuration (e.g. PID limit change) and have that change roll out 
> in a minimally disruptive way subject to the customer's configured maintenance schedule."

> "As an SRE, tasked with performing emergency corrective action, I want to be able to 
> quickly ignore a configured maintenance schedule, apply necessary changes, have them roll out immediately, 
> and restore the maintenance schedule to its previous configuration."

> "As a leader within the Service Delivery organization, tasked with performing emergency corrective action
> across our fleet, I want to be able to bypass and then restore customer maintenance schedules
> with minimal technical overhead."

> "As a cluster lifecycle administrator who is well served by a fully managed update without change management, 
> I want to be minimally inconvenienced by the introduction of change management / maintenance schedules."

> "As a cluster lifecycle administrator who is not well served by a fully managed update and needs exacting 
> control over when material changes occur on my cluster where opportunities do NOT arise at reoccurring intervals,
> I want to employ a change management strategy that defers material changes until I perform a manual action."

> "As a cluster lifecycle administrator, I want to easily determine the next time at which maintenance operations
> will be permitted to be initiated, based on the configured maintenance schedule, by looking at the 
> status of relevant API resources or metrics."

> "As a cluster lifecycle administrator, I want to easily determine whether there are material changes pending for
> my cluster, awaiting a permitted window based on the configured maintenance schedule, by looking at the 
> status of relevant API resources or metrics."

> "As a cluster lifecycle administrator, I want to easily determine whether a maintenance schedule is currently being
> enforced on my cluster by looking at the status of relevant API resources or metrics."

> "As a cluster lifecycle administrator, I want to be able to alert my operations team when changes are pending,
> when and the number of seconds to the next permitted window approaches, or when a maintenance schedule is not being
> enforced on my cluster."

> "As a cluster lifecycle administrator, I want to be able to diagnose why pending changes have not been applied
> if I expected them to be."

> "As a cluster administrator or privileged user familiar with OpenShift prior to the introduction of change management, 
> I want it to be clear when I am looking at the desired versus actual state of the system. For example, if I can see 
> the state of the clusterversion or a machineconfigpool, it should be straightforward to understand why I am 
> observing differences in the state of those resources compared to the state of the system."

### Goals

1. Indirectly support the strategic separation of control-plane and worker-node update phases for Standalone clusters by 
   supplying a change control mechanism that will allow both control-plane and worker-node updates to proceed at predictable 
   times without doubling operational overhead.
1. Empower OpenShift cluster lifecycle administrators with tools that simplify implementing industry standard notions 
   of maintenance windows.
1. Provide Service Delivery a platform native feature which will reduce the amount of custom tooling necessary to 
   provide maintenance windows for customers.
1. Deliver a consistent change management experience across all platforms and profiles (e.g. Standalone, ROSA, HCP).
1. Enable SRE to, when appropriate, make configuration changes on a customer cluster and have that change actually 
   take effect only when permitted by the customer's change management preferences.
1. Do not subvert expectations of customers well served by the existing fully self-managed cluster update.
1. Ensure the architectural space for enabling different change management strategies in the future.
1. Directly support the strategic separation of control-plane and worker-node update phases by empowering cluster 
   lifecycle administrators with change management strategies that provide them fine-grained control over exactly
   when and how worker-nodes are updated to a desired configuration even if no regular maintenance schedule is possible.
   
### Non-Goals

1. Allowing control-plane upgrades to be paused midway through an update. Control-plane updates are relatively rapid 
   and pausing will introduce unnecessary complexity and risk. 
1. Requiring the use of maintenance schedules for OpenShift upgrades (the changes should be compatible with various 
   upgrade methodologies – including being manually triggered).
1. Allowing Standalone worker-nodes to upgrade to a different payload version than the control-plane 
   (this is supported in HCP, but is not a goal for standalone).
1. Exposing maintenance schedule controls from the oc CLI. This may be a future goal but is not required by this enhancement.
1. Providing strict promises around the exact timing of upgrade processes. Maintenance schedules will be honored to a 
   reasonable extent (e.g. upgrade actions will only be initiated during a window), but long-running operations may 
   exceed the configured end of a maintenance schedule.
1. Implementing logic to defend against impractical maintenance schedules (e.g. if a customer configures a 1-second 
   maintenance schedule every year). Service Delivery may want to implement such logic to ensure upgrade progress can 
   be made.
1. Automatically initiating updates to `ClusterVersion`. This will still occur through external actors/orchestration. 
   Maintenance schedules simply give the assurance that changes to `ClusterVersion` will not result in material changes 
   until permitted by the defined maintenance schedules.

## Proposal

### Change Management Overview
We will establish two new custom Resource Definitions (CRDs) which allow cluster lifecycle
administrators to capture their requirements for when resource(s) associated with the CRDs can initiate material 
changes to a cluster. 
1. `ChangeManagementPolicy` - cluster scoped resource for management of internal changes within the cluster hosting the CRD.
2. `HostedChangeManagementPolicy` - namespaced resource, created on a management cluster, in the same namespace as a `HostedCluster`, to control changes to resources associated with that `HostedCluster`.

The semantics of the APIs are identical other than the type of cluster they are intended to influence. 

Resources subject to change management will include a `changeManagement` stanza allowing cluster
lifecycle administrators to reference defined `[Hosted]ChangeManagementPolicy` objects. Several existing resources in 
the OpenShift ecosystem will be updated to include support for change management to restrict when and how their 
associated controllers can initiate material changes to a cluster.
- HCP's `HostedCluster` can reference `HostedChangeManagementPolicy`. Honored by HyperShift Operator and supported by underlying CAPI providers.
- HCP's `NodePool` can reference `HostedChangeManagementPolicy`. Honored by HyperShift Operator and supported by underlying CAPI providers.
- Standalone's `ClusterVersion` can reference `ChangeManagementPolicy`. Honored by Cluster Version Operator.
- Standalone's `MachineConfigPool` can reference `ChangeManagementPolicy`. Honored by Machine Config Operator.

Changes that are not allowed to be initiated due to a change management policy will be 
called "pending". Controllers responsible for initiating pending changes will await a permissive window 
according to each resource's relevant `changeManagement` configuration.

In additional to "policies", different resource kinds may offer additional knobs in their `changeManagement`
stanzas to provide cluster lifecycle administrators additional control over the exact nature of 
the changes desired by the resource's associated controller. For example, in `MachineConfigPool`'s 
`changeManagement` stanza, a cluster lifecycle administrator will be able to (optionally) specify the 
exact rendered configuration the controller should be working towards (during the next permissive window) vs
the traditional OpenShift model where the "latest" rendered configuration is always the destination. 

### `[Hosted]ChangeManagementPolicy` Resource

```yaml
# HostedChangeManagementPolicy for hosted clusters (referenced by HostedCluster and NodePool)
# ChangeManagementPolicy for internal policies of a c cluster (referenced by CVO and MCP).
kind: "[Hosted]ChangeManagementPolicy"
metadata:

  # Only HostedChangeManagementPolicy are namespaced. This
  # field is NOT present for ChangeManagementPolicy.
  namespace: <hosted-cluster-namespace>
  
  # The name of the policy, which will be referenced by changeManagement stanzas.
  name: example-policy

spec:
  # Supported strategy overview:
  # Permissive:
  #   Always permissive - allows material changes. An administrator may use this value 
  #     to allow a number of resources referencing this policy to actuate material changes 
  #     without having to edit each separately. It could be used during an impromptu 
  #     change window or to help quickly drive out a critical security update.
  # Restrictive:
  #   Always restrictive - restricts material changes from being initiated.
  # MaintenanceSchedule: 
  #   A recurrence rule and other fields will be used to specify reoccurring permissive windows
  #   as well as any special exclusion periods.
  strategy: Permissive | Restrictive | MaintenanceSchedule

  # Difference strategies expose a configuration
  # stanza that further informs their behavior.
  maintenanceSchedule:    
    permit:
      ...
  
# A new ChangeManagementPolicy controller will update the status field so that
# other controllers, attempting to abide by change management, can easily 
# determine whether they can initiate material changes.
status:

  behavior:
    current:
      state: ChangesPaused
      # When the state started
      startTime: <datetime>
      endTime: <datetime>  # null if not expected to change
      reason: 'human readable..'
    next:  # next must not be null if current.endTime is set.
      state: ChangesUnpaused
      startTime: <datetime>
      endTime: <datetime>
      reason: 'human readable..'
    history:  # Includes up to 5 of the last transitions
      - strategy: MaintenanceSchedule
        state: ChangesUnpaused
        startTime: <datetime>
        endTime: <datetime>
      - strategy: MaintenanceSchedule
        mode: ChangesPaused
        startTime: <datetime>
        endTime: <datetime>

  conditions:
  # If a [Hosted]ChangeManagementPolicy has not calculated yet, it will not
  # have Ready=True. Resources referencing a ChangeManagementPolicy must
  # interpret Ready=False as a Restrictive policy until it is Ready=True.
  - type: Ready
    status: "True"
    reason: "AsExpected"
  # Indicates whether the policy is in a permissive state.
  # Must be False while not "Ready".
  # Message must provide detailed reason when False.
  - type: ChangesRestricted
    status: "True"
    reason: "MaintenanceSchedule"
    message: "Details on why..."
```

### Change Management Strategies

#### Maintenance Schedule Strategy
The strategy is configured by specifying a recurrence rule, identifying permissive dates during which material changes can be
initiated. The cluster lifecycle administrator can also exclude specific date ranges, during which 
the policy will request material changes to be restricted.

#### Restrictive Strategy
A policy using the restrictive strategy will always request material changes to be paused. This strategy is useful
when a cluster lifecycle administrator wants tight control over when material changes are initiated on the cluster
but cannot provide a maintenance schedule (e.g. viable windows are too unpredictable).

#### Permissive Strategy
A policy using the permissive strategy will always suggest a permissive window. A cluster lifecycle administrator
may want to toggle a `[Hosted]ChangeManagementPolicy` from the `Restrictive` to `Permissive` strategy, and back again,
as a means to implementing their own change management window mechanism.

### Resources Supporting Change Management

Resources which support a reference to a `[Hosted]ChangeManagementPolicy` are said to support change management.
Resources which support change management will implement a `spec.changeManagement` stanza. These stanzas
must support AT LEAST the following fields:

#### ChangeManagement Stanzas
```yaml
kind: ClusterVersion|MachineConfigPool|HostedCluster|NodePool
spec:
  changeManagement:
    #
    # ByPolicy - Policy is dynamic and defined by ChangeManagementPolicy object.
    # Permissive - All changes are permitted.
    # Restrictive - All changes are restricted.
    # PermissiveUntil - Changes are permitted until a specified datetime after which byPolicy applies if defined.
    #  If byPolicy is not set, falls back to Restrictive after expiration.
    # RestrictiveUntil - Changes are restricted until a specified datetime after which byPolicy applies.
    #  If byPolicy is not set, falls back to Permissive after expiration.
    strategy: ByPolicy | Permissive | Restrictive | PermissiveUntil | RestrictiveUntil
    
    # A reference to the [Hosted]ChangeManagementPolicy used to determine whether material 
    # changes can be initiated.
    # byPolicy will be preserved even if Permissive | Restrictive is configured. This
    # allows an administrator to quickly force an alternative strategy and then restore it to
    # ByPolicy without having to remember what the previously configured policy was.
    # Most k8s resources would delete the unused stanza, so this behavior is atypical.
    byPolicy: 
      # The name of the [Hosted]ChangeManagementPolicy. ClusterVersion and MachineConfigPool can
      # reference ChangeManagementPolicy. HostedCluster and NodePool can reference 
      # HostedChangeManagementPolicy in the HostedCluster's namespace.
      name: example-policy
    
    permissiveUntil: <datetime> # Only valid when state: PermissiveUntil 
    restrictiveUntil: <datetime> # Only valid when state: RestrictiveUntil    
```

At a given moment, a `changeManagement` stanza indicates to a controller responsible for a resource
whether changes should be restricted (no material changes should be initiated) or permitted (material changes can be initiated).
This can be specified directly with strategies like `Permissive` or `Restrictive` or informed indirectly by
referencing a separate `ChangeManagementPolicy` object.

#### Change Management Status & Conditions
Each resource which exposes `spec.changeManagement` must also expose change management status information
to explain its current impact. Common user interfaces for aggregating and displaying progress of these underlying 
resources (e.g. OpenShift web console) must be updated to share that status information with end users.

You may note that several of the fields in `status.changeManagement` for resources supporting change management
can be derived directly from `[Hosted]ChangeManagementPolicy.status`. This simplifies the work of each controller 
which supports change management - they simply need to observe `ChanageManagementPolicy.status`
and the `ChanageManagmentPolicy` controller does the heavy lifting of interpreting policy (e.g. interpreting recurrence
rule definitions).

```yaml
kind: ClusterVersion|MachineConfigPool|HostedCluster|NodePool
spec:
  ...
status:
  changeManagement:

    behavior:
      # Combines change management information from object's configuration
      # and any configured policy to reflect object specific status.
      current:
        state: Disabled
        startTime: 2025-03-15T12:34:56Z
        endTime: 2025-03-28T00:00:00Z
        reason: 'Change management disabledUntil set to 2025-03-28T00:00:00Z'
      next:  # See policy object for more details on this status information
        state: ChangesUnpaused
        startTime: <datetime>
        endTime: <datetime>
        reason: 'human readable..'
      history:  # See policy object for more details on this status information
        - strategy: MaintenanceSchedule
          state: ChangesUnpaused
          startTime: <datetime>
          endTime: <datetime>
        - strategy: MaintenanceSchedule
          mode: ChangesPaused
          startTime: <datetime>
          endTime: <datetime>

  conditions:
  - type: ChangesPaused
    status: "True"
    message: "Details on why..."
  - type: ChangesPending
    status: "True"  
    message: "Details on what..."
```

#### Change Management Metrics
Change management information will be made available through cluster metrics. Each resource
containing the `spec.changeManagement` stanza must expose the following metrics:
- Whether any change management strategy is enabled.
- Which change management strategy is enabled. This can be used to notify SRE when a cluster begins using a 
  non-standard strategy (e.g. during emergency corrective action).
- The approximate number of seconds until the next known permitted change window. See `change_management_next_change_eta` metric. 
  This might be used to notify an SRE team of an approaching permissive window.
- The approximate number of seconds until the current change window closes. See `change_management_permissive_remaining` metric.
- The last datetime at which changes were permitted (can be nil). See `change_management_last_change` metric (which 
  represents this as seconds instead of a datetime). This could be used to notify an SRE team if a cluster has not 
  had the opportunity to update for a non-compliant period.
- If changes are pending due to change management controls. When combined with other 
  metrics (`change_management_next_change_eta`, `change_management_permissive_remaining`), this can be used to 
  notify SRE when an upcoming permissive window is going to initiate changes and whether changes are still 
  pending as a permissive window closes.

### Enhanced MachineConfigPool Control
The MachineConfigOperator (MCO), like any other operator, works toward eventual consistency of the system state
with state of configured resources. This behavior offers a simple user experience wherein an administrator
can make a change to a `MachineConfig` and the MCO takes over rolling out that change. It will
1. Aggregate selected `MachineConfig` objects into a new "rendered" `MachineConfig` object.
1. Work through updating nodes to use the "latest" rendered `MachineConfig` associated with them. 

However, in business critical clusters, progress towards the "latest" rendered `MachineConfig` offers less control
than may be desired for some use cases. To address this, the `MachineConfigPool` resource will be extended
to include an option to declare which rendered `MachineConfig` the MachineConfigOperator should make
progress toward instead of the "latest".

```yaml
kind: MachineConfigPool
spec:
  machineConfig:
    # An administrator can identify the exact rendered MachineConfig
    # the MCO should progress towards for this MCP.
    name: <rendered machine configuration name>
    
    # ValidAfter defines when MCO will allow the use of a new
    # configuration. CanaryUpgrade or null means a node must successfully use the
    # configuration before new nodes are ignited with the
    # desiredConfiguration -- this is the historical behavior of the platform.
    # Immediate means no validation should be performed (new nodes will
    # immediately ignite with the configuration).
    validAfter: CanaryUpgrade | Immediate
```

### Projected ClusterVersion in HCP
A [Cluster Instance Admin](https://hypershift-docs.netlify.app/reference/concepts-and-personas/#personas) using a 
hosted cluster cannot directly update their cluster. When they query `ClusterVersion` on their cluster, they only
see a "projected" value. If they edit the projected value, it does not affect change on the cluster.

In order to update the hosted cluster, changes must be made to its associated HCP resources on the management cluster.
Since this proposal subjects those resources to change management control, this information must also be projected
into the `ClusterVersion` (including change management status information) of hosted clusters so that a Cluster 
Instance Admin can understand when material changes to their cluster will take place.

### Workflow Description

#### OCM HCP Standard Change Management Scenario

1. A [Cluster Service Consumer](https://hypershift-docs.netlify.app/reference/concepts-and-personas/#personas) requests an HCP cluster via OCM.
1. To comply with their company policy, the service consumer configures a maintenance schedule through OCM. 
1. Their first preference, no updates at all, is rejected by OCM policy, and they are referred to service 
   delivery documentation explaining minimum requirements. 
1. The user specifies a policy which permits control-plane changes to be initiated any time Saturday UTC on the control-plane.
1. To limit overall workload disruption, the user changes their worker-node policy to permit updates only on the **first** Saturday of each month.
1. OCM accepts the configuration.
1. OCM configures the HCP (HostedCluster/NodePool) resources via the Service Delivery Hive deployment to contain a `changeManagement` stanza 
   referring to newly created `ChanageManagementPolicy` objects using the `MaintenanceSchedule` strategy.
1. Hive creates/updates the associated HCP resources.
1. Company workloads are added to the new cluster and the cluster provides business value.
1. To leverage a new feature in OpenShift, the service consumer plans to update the minor version of the platform.
1. Via OCM, the service consumer requests the minor version update. They can do this at any time with confidence that the maintenance 
   schedule will be honored. They do so on Wednesday.
1. OCM (through various layers) updates the target release payload in the HCP HostedCluster and NodePool.
1. The HyperShift Operator detects the desired changes but recognizes that the `changeManagement` stanzas 
    of each resource precludes updates from being initiated.
1. Curious, the service consumer checks the projected ClusterVersion within the HostedCluster and reviews 
   its `status` stanza. It shows that changes are pending and the time of the next window in which changes 
   can be initiated.  
1. Separate metrics specific to change management indicate that changes are pending for both resources.
1. The non-Red Hat operations team has alerts setup to fire when changes are pending and the number of 
    seconds before the next permitted window is less than 2 days away.
1. These alerts fire after Thursday UTC 00:00 to inform the operations team that changes are about to be applied to the control-plane.
1. It is not the first week of the month, so there is no alert fired for the NodePool pending changes.
1. The operations team is comfortable with the changes being rolled out on the control-plane. 
1. On Saturday 00:00 UTC, the HyperShift operator initiates changes the control-plane update.
1. The update completes without issue.
1. Changes remain pending for the NodePool resource.
1. As the first Saturday of the month approaches, the operations alerts fire to inform the team of forthcoming changes.
1. The operations team realizes that a corporate team needs to use the cluster heavily during the weekend for a business critical deliverable.
1. The service consumer logs into OCM and adds an exclusion for the upcoming Saturday.
1. Interpreting the new exclusion, the metric for time remaining until a permitted window increases to count down to the following month's first Saturday.
1. A month passes and the pending cause the configured alerts to fire again.
1. The operations team is comfortable with the forthcoming changes.
1. The first Saturday of the month 00:00 UTC arrives. The HyperShift operator initiates the worker-node updates based 
   on the pending changes in the cluster NodePool.
1. The HCP cluster has a large number of worker nodes and draining and rebooting them is time-consuming. 
1. At 23:59 UTC Saturday night, 80% of worker-nodes have been updated. Since the maintenance schedule still permits 
   the initiation of material changes, another worker-node begins to be updated.
1. The update of this worker-node continues, but at 00:00 UTC Sunday, no further material changes are permitted by 
   the change management policy and the worker-node update process is effectively paused.
1. Because not all worker-nodes have been updated, changes are still reported as pending via metrics for 
   NodePool.  
1. The HCP cluster runs with worker-nodes at mixed versions throughout the month. The N-1 skew between the old kubelet versions and control-plane is supported.
1. On the next first Saturday of the month, the worker-nodes updates are completed.

#### OCM Standalone Standard Change Management Scenario

1. User interactions with OCM to configure a maintenance schedule are identical to [OCM HCP Standard Change Management Scenario](#ocm-hcp-standard-change-management-scenario).
   This scenario differs after OCM accepts the maintenance schedule configuration. Control-plane updates are permitted to be initiated to any Saturday UTC.
   Worker-nodes must wait until the first Saturday of the month.
1. OCM (through various layers) creates ChangeManagementPolicies and configures the ClusterVersion and 
   worker MachineConfigPool(s) (MCP) for the cluster with appropriate `changeManagement` stanzas.
1. Company workloads are added to the new cluster and the cluster provides business value.
1. To leverage a new feature in OpenShift, the service consumer plans to update the minor version of the platform.
1. Via OCM, the service consumer requests the minor version update. They can do this at any time with confidence that the maintenance 
   schedule will be honored. They do so on Wednesday.
1. OCM (through various layers) updates the ClusterVersion resource on the cluster indicating the new release payload in `desiredUpdate`.
1. The Cluster Version Operator (CVO) detects that its `changeManagement` stanza does not permit the initiation of the change.
1. The CVO sets a metric indicating that changes are pending for ClusterVersion. Irrespective of pending changes, the CVO also exposes a 
   metric indicating the number of seconds until the next window in which material changes can be initiated.
1. The MCO, irrespective of pending changes, exposes a metric for each MCP to indicate the number of seconds remaining until it is
    permitted to initiate changes to nodes in that MCP.
1. A privileged user on the cluster notices different options available for `changeManagement` in the ClusterVersion and MachineConfigPool
    resources. They try to set them but are prevented by an SRE validating admission controller. If they wish
    to change the settings, they must update them through OCM.
1. The privileged user does an `oc describe ...` on the resources. They can see that material changes are pending in ClusterVersion for 
    the control-plane and for worker machine config. They can also see the date and time that the next material change will be permitted.
    The MCP will not show a pending change at this time, but will show the next time at which material changes will be permitted.
1. The next Saturday is _not_ the first Saturday of the month. The CVO detects that material changes are permitted at 00:00 UTC and
    begins to apply manifests. This effectively initiates the control-plane update process, which is considered a single
    material change to the cluster.
1. The control-plane update succeeds. The CVO, having reconciled its state, unsets metrics suggesting changes are pending.
1. As part of updating cluster manifests, MachineConfigs have been modified. The MachineConfigOperator (MCO) re-renders a
    configuration for worker-nodes. However, because the MCP `chanageManagement` stanza precludes initiating material changes,
    it will not yet begin to update Machines with that desired configuration.
1. The MCO will set a metric indicating that desired changes are pending. 
1. `oc get -o=yaml/describe` will both provide status information indicating that changes are pending for the MCP and
    the time at which the next material changes can be initiated according to the maintenance schedule.
1. On the first Saturday of the next month, 00:00 UTC, the MCO determines that material changes are permitted.
    Based on limits like maxUnavailable, the MCO begins to annotate nodes with the desiredConfiguration. The 
    MachineConfigDaemon takes over from there, draining, and rebooting nodes into the updated release.
1. There are a large number of nodes in the cluster and this process continues for more than 24 hours. On Saturday
    23:59, the MCO applies a round of desired configurations annotations to Nodes. At 00:00 on Sunday, it detects
    that material changes can no longer be initiated, and pauses its activity. Node updates that have already
    been initiated continue beyond the maintenance schedule window.
1. Since not all nodes have been updated, the MCO continues to expose a metric informing the system of
    pending changes.
1. In the subsequent days, the cluster is scaled up to handle additional workload. The new nodes receive
    the most recent, desired configuration.    
1. On the first Saturday of the next month, the MCO resumes its work. In order to ensure that forward progress is
    made for all nodes, the MCO will update nodes in order of oldest rendered configuration to newest rendered configuration
    (among those nodes that do not already have the desired configuration). This ensures
    that even if the desired configuration has changed multiple times while maintenance was not permitted,
    no nodes are starved of updates. Consider the alternative where (a) worker-node updates required > 24h,
    (b) updates to nodes are performed alphabetically, and (c) MachineConfigs are frequently being changed
    during times when maintenance is not permitted. This strategy could leave nodes sorting last 
    lexicographically no opportunity to receive updates. This scenario would eventually leave those nodes
    more prone to version skew issues.
1. During this window of time, all node updates are initiated, and they complete successfully. 

#### Service Delivery Emergency Patch
1. SRE determines that a significant new CVE threatens the fleet.
1. A new OpenShift release in each z-stream fixes the problem. Only the control-plane needs to be updated
   to remediate the CVE.
1. SRE plans to override customer maintenance schedules in order to rapidly remediate the problem across the fleet.
1. The new OpenShift release(s) are configured across the fleet. Clusters with permissive change management 
   policies begin to apply the changes immediately.
1. Standlone clusters with `ClusterVersion` change management policies precluding updates are SRE's next focus.
1. During each region's evening hours, to limit disruption, SRE changes `ClusterVersion.spec.changeManagement` to the `PermissiveUntil` 
   strategy and sets `permissiveUntil` to the current UTC datetime+24h. Changes that were previously pending are can now be initiated for 
   24 hours. `PermissiveUntil` reverts to the originally configured change management policy after 24 hours.
1. Clusters which have alerts configured to fire when there is no change management policy in place 
   will do so.
1. SRE continues to monitor the rollout but does not need to remove `PermissiveUntil` since it will
   automatically revert to any configured policy in 24 hours.
1. Clusters with change management policies setup for their worker-nodes are not updated automatically after the
   control-plane update. MCPs will report pending changes, but the MachineConfigOperator will await a 
   permissive window for each MCP to apply the potentially disruptive update.

#### Service Delivery Immediate Remediation
1. A customer raises a ticket for a problem that is eventually determined to be caused by a worker-node system configuration.
1. SRE can address the issue with a system configuration file applied in a MachineConfig.
1. SRE creates the MachineConfig for the customer and provides the customer the option to either (a) wait until their
   configured maintenance schedule permits the material change from being initiated by the MachineConfigOperator
   or (b) having SRE override the maintenance schedule and permitting its immediate application.   
1. The customer chooses immediate application. 
1. SRE applies a change to the relevant worker-node resource's `changeManagement` stanza, setting `permissiveUntil` to
   a time 48 hours in the future. The configured change management policy is ignored for 48 hours as the system 
   initiates all necessary node changes to worker-nodes.
1. If unrelated changes were pending for the control-plane, they will remain pending throughout this process.

#### Service Delivery Deferred Remediation
1. A customer raises a ticket for a problem that is eventually determined to be caused by a worker-node system configuration.
1. SRE can address the issue with a system configuration file applied in a MachineConfig.
1. SRE creates the MachineConfig for the customer and provides the customer the option to either (a) wait until their
   configured maintenance schedule permits the material change from being initiated by the MachineConfigOperator
   or (b) modify change management to permit immediate application.   
1. The problem is not pervasive, so the customer chooses the deferred remediation. 
1. The change is initiated and nodes are rebooted during the next permissive window.


#### On-prem Standalone GitOps Change Management Scenario
1. An on-prem cluster is fully managed by gitops. As changes are committed to git, those changes are applied to cluster resources.
1. Configurable stanzas of the ClusterVersion and MachineConfigPool(s) resources are checked into git.
1. The cluster lifecycle administrator configures `spec.changeManagement` in both the ClusterVersion and worker MachineConfigPool
   in git. A policy using the MaintenanceSchedule strategy is chosen. The policy permits control-plane and worker-node updates only after
   19:00 UTC.
1. During the working day, users may contribute and merge changes to MachineConfigs or even the `desiredUpdate` of the
   ClusterVersion. These resources will be updated on the cluster in a timely manner via GitOps.
1. Despite the resource changes, neither the CVO nor MCO will begin to initiate the material changes on the cluster.
1. Privileged users, who may be curious as to the discrepancy between git and the cluster state, can use `oc get -o=yaml/describe`
   on the resources. They observe that changes are pending and the time at which changes will be initiated.
1. At 19:00 UTC, the pending changes begin to be initiated. This rollout abides by documented OpenShift constraints
   such as the MachineConfigPool `maxUnavailable` setting.
   
#### On-prem Standalone Manual Change Rollout Scenario
1. A small, business critical cluster is being run on-prem.
1. There are no reoccurring windows of time when the cluster lifecycle administrator can tolerate downtime.
   Instead, updates are negotiated and planned far in advance. 
1. The cluster workloads are not HA and unplanned drains are a business risk.
1. To prevent surprises, the cluster lifecycle administrator worker-node MCP to refer to a `ChanageManagementPolicy`
   using the `Restrictive` strategy (which never permits changes to be initiated).
1. Given the sensitivity of the operation, the lifecycle administrator wants to manually drain and reboot
   nodes to accomplish the update.
1. The cluster lifecycle administrator sends a company-wide notice about the period during which service may be disrupted.
1. The user determines the most recent rendered worker `MachineConfig`. 
1. They configure the `MachineConfigPool.spec.machineConfig.name` field to specific that exact configuration as the
   target configuration. They also set `MachineConfigPool.spec.machineConfig.validAfter` to `Immediate`.
   By bypassing normal validation, the MCO is being asked to ignite any new node with the specified `MachineConfig`
   even without first ensuring an existing node can use the configuration. At the same time, the `Restrictive`
   change management policy that is in place is telling the MCO that is it not permitted to initiate
   changes on existing nodes.
1. The MCO metric for the MCP indicating that changes are pending is set because not all nodes are running
   the most recently rendered configuration. Conceptually, it means, if change management was not enabled, 
   whether changes would be initiated.
1. The cluster lifecycle administrator scales in a new node. It receives the specified configuration. They
   validate the node's functionality.
1. Satisfied with the new machine configuration, the cluster lifecycle administrator begins manually draining
   and rebooting existing nodes on the cluster.
   Before the node reboots, it takes the opportunity to pivot to the `desiredConfig`. 
1. After updating all nodes, the cluster lifecycle administrator does not need make any additional 
   configuration changes. They can leave the `changeManagement` stanza in their MCP as-is.
   
#### On-prem Standalone Assisted Rollout Scenario
1. A large, business critical cluster is being run on-prem.
1. There are no reoccurring windows of time when the cluster lifecycle administrator can tolerate downtime.
   Instead, updates are negotiated and planned far in advance. 
1. The cluster workloads are not HA and unplanned drains are a business risk.
1. To prevent surprises, the cluster lifecycle administrator sets `changeManagement` on the worker MCP
   to refer to a `ChangeManagementPolicy` using the `Restictive` strategy.
1. Various cluster updates have been applied to the control-plane, but the cluster lifecycle administrator
   has not updated worker-nodes.
1. The MCO metric for the MCP indicating that changes are pending is set because not all nodes are running
   the desired `MachineConfig`.
1. The cluster lifecycle administrator wants to choose the exact `MachineConfig` to apply as well as the exact 
   times during which material changes will be made to worker-nodes. However, they do not want to manually 
   reboot nodes. To have the MCO assist them in the rollout of the change, they set
   `MachineConfigPool.spec.machineConfig.name=<selected rendered MachineConfig>` and
   `MachineConfigPool.spec.changeManagement.permissiveUntil=<tomorrow>` 
   to allow the MCO to begin initiating material changes and make progress towards the specified configuration.
   The MCO will not prune any `MachineConfig` referenced by an MCP's `machineConfig` stanza. 
1. The MCO begins to initiate worker-node updates. This rollout abides by documented OpenShift constraints
   such as the MachineConfigPool `maxUnavailable` setting. It also abides by currently rollout rules (i.e.
   new nodes igniting will receive an older configuration until at least one node has successfully applied 
   the latest rendered `MachineConfig`).
1. Though new rendered configurations may be created during this process (e.g. another control-plane update
   is applied while the worker-nodes are making progress), the MCO will ignore them since it is being asked
   to apply `MachineConfigPool.spec.machineConfig.name` as the desired configuration.

This scenario could also be achieved by toggling the strategy configured in a `ChangeManagementPolicy` 
referenced by their MCPs from `strategy: Restrictive` to `strategy: Permissive` and back again after their worker-nodes are
updated.
  
### API Extensions

The enhancement is API driven, as described in the overview. Two new CRDs are introduced:
- HostedChangeManagementPolicy
- ChangeManagementPolicy

Four existing APIs are modified in order to support change management policy definitions:
- HostedCluster
- NodePool
- ClusterVersion
- MachineConfigPool

Support for change management policies requires behavioral changes in controllers associated with
these resources to honor configured policies. 

### Topology Considerations

#### Hypershift / Hosted Control Planes

In the HCP topology, the `HostedCluster` and `NodePool` resources are enhanced to support the `spec.changeManagement`
stanza. 

#### Standalone Clusters

In the Standalone topology, the ClusterVersion and MachineConfigPool resources are enhanced to support the 
`spec.changeManagement` stanza.

#### Single-node Deployments or MicroShift

These toplogies do not have worker nodes. Only the ClusterVersion change management policy will be relevant.
There is no logical distinction between user workloads and control-plane workloads in this case. A control-plane
update will drain user workloads and will cause workload disruption.

Though disruption is inevitable, the maintenance schedule feature provides value in these toplogies by
giving the cluster lifecycle administrator high level controls on exactly when it occurs.

#### OCM Managed Profiles
OpenShift Cluster Manager (OCM) should expose a user interface allowing users to manage their change management policy.
Standard Fleet clusters will expose the option to configure the control-plane and worker-node `[Hosted]ChangeManagementPolicy`
objects with the `MaintenanceSchedule` strategy - including permit and exclude times.

- Service Delivery will reserve the right to override this policy for emergency corrective actions.
- Service Delivery should constrain permit & exclude configurations based on their internal policies. 
  For example, customers may be forced to enable permissive windows which amount to at least 6 hours a month.

### Implementation Details/Notes/Constraints

#### ChangeManagement Stanza
The `spec.changeManagement` stanza will be introduced into ClusterVersion and MachineConfigPool (for standalone profiles)
and HostedCluster and NodePool (for HCP profiles).

#### MaintenanceSchedule Strategy Configuration

When a `[Hosted]ChangeManagementPolicy` is defined to use the `MaintenanceSchedule` strategy,
a `maintenanceSchedule` stanza should also be provided to configure the strategy
(if it is not, the policy is functionally identical to `Restrictive`).

```yaml
kind: ChanageManagementPolicy
spec:
  strategy: MaintenanceSchedule
 
  maintenanceSchedule:

    # Specifies a reoccurring permissive window. 
    permit:  
      
      # If null, all dates are permitted and only exclude constrains permissive windows.
      recurrence: 
        # "frequency" is a discriminant for the unioned types which follow.
        frequency: Yearly | Monthly | Weekly | Daily
        
        # Required when frequency=Daily
        daily:
          interval: <int> # How many days should pass before the next occurrence 0 < x < 731

        # Required when frequency=Weekly
        weekly:
          daysOfWeek: 
          - Monday
          - Tuesday
          - Wednesday
          interval: <int> # How many weeks should pass before the next occurrence 0 < x < 27

        # Required when frequency=Monthly
        monthly:
          # "by" is a discriminant for the unioned types which follow.
          by: Day | Date
          date:
            datesOfMonth: 
            - <int> # List of dates 0 < x < 32
            interval: <int> # How many months should pass before the next occurrence 0 < x < 12
          day:
            days:
            - weekOfMonth: First | Second | Third | Fourth | Fifth | Last
              dayOfWeek: Monday | Tuesday | Wednesday | ... | Sunday
            interval: <int> # How many months should pass before the next occurrence 0 < x < 12

        # Required when frequency=Yearly
        yearly:
          # "by" is a discriminant for the unioned types which follow.
          by: Day | Date
          date:
            datesOfMonth: 
            - <int> # List of dates 0 < x < 32
            month: January | February | ... | December
          day:
            days:
            - weekOfMonth: First | Second | Third | Fourth | Fifth | Last
              dayOfWeek: Monday | Tuesday | Wednesday | ... | Sunday
            month: January | February | ... | December
      
      # Given the identification of a date by a recurrence rule, at what time (always UTC) can the 
      # permissive window begin. "00:00" if unset. Validated with a regex.
      startTime: <time-of-day|null>
      
      # Given the identification of a date by a recurrence rule, after what offset from the startTime should
      # the permissive window close. This can create permissive windows within days that are not
      # identified in the rule. For example, if "weekly on Saturday" is specified as a rule,
      # startTime="20:00", duration="8h" would permit material change initiation starting
      # each Saturday at 8pm and continuing through Sunday 4am (all times are UTC). The default
      # duration is 24:00-startTime (i.e. to the end of the day).
      # Value required to be > 0.
      duration: <duration|null>
      
    
    # Excluded date ranges override recurrence rule selections.
    exclude:
    # Dates should be specified in YYYY-MM-DD. Each date is excluded from 00:00 UTC for 24 hours.
    - fromDate: <date>
      # Non-inclusive until. If null, until defaults to the day after from (meaning a single day exclusion).
      untilDate: <date|null>
      # Provide additional detail for status when the cluster is within an exclusion period.
      reason: Optional human readable which will be included in status description.
```

**Recurrence Rule Interpretation**
The recurrence schema allows users to express recurring intervals of time without specifying start dates. For example:

```yaml
# Permit every 5th day
daily:
  interval: 5
```

A fixed start date must be assumed in order to calculate a deterministic set of days on which to permit
change. Such rules will be evaluated with a starting date of Jan 1st, 1970 00:00Z. 

If no `startTime` or `duration` is specified, any day selected by the recurrence rule will suggest a
permissive 24h window unless a date is in the `exclude` ranges.

**Overview of Interactions**
The MaintenanceSchedule strategy allows a cluster lifecycle administrator to express one of the following:

| permit | exclude | Enforcement State|
|--------|---------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `null` | `null`  | Restrictive indefinitely           |
| set    | `null`  | Permissive during reoccurring windows time. Restrictive at all other times.                                                                                                                  |
| set    | set     | Permissive during reoccurring windows time modulo excluded / restrictive date ranges. Restrictive at all other times.                                                                                                                                                                                                         |
| `null` | set     | Permissive except during excluded dates during which it is restrictive.                                                                                                                             |

#### CLI Assisted Rollout Scenarios
Once this proposal has been implemented, it is expected that `oc` will be enhanced to permit users to 
trigger assisted rollouts scenarios for worker-nodes (i.e. where they control the timing of a rollout
completely but progress is made using MCO automation). 

`oc adm update` will be enhanced with `worker-node` subverbs to initiate and pause MCO work. 
```sh
$ oc adm update worker-nodes start ...
$ oc adm update worker-nodes pause/resume ...
$ oc adm update worker-nodes rollback ...
```

These verbs can leverage `MachineConfigPool.spec.changeManagement` to achieve their goals. 
For example, if the MCP is as follows:

```yaml
kind: MachineConfigPool
spec:
  machineConfig:
    name: <desired rendered MachineConfig>
    validAfter: CanaryUpgrade
```

- `worker-nodes start` can set a target `spec.machineConfig.name` to initiate progress toward a new update's
  recently rendered `MachineConfig`. 
- `worker-nodes pause/resume` can toggle `spec.changeManagement` strategies between `Restrictive` and `Permissive` (or `ByPolicy` if `byPolicy` is set).
- `worker-nodes rollback` can restore a previous `spec.machineConfig.name` is backed up in an MCP annotation.

#### Manual Rollout Scenario

Cluster lifecycle administrators desiring still more control can initiate node updates & drains themselves.

```yaml
kind: MachineConfigPool
spec:
  changeManagement:
    strategy: Restrictive
  
  machineConfig:
    name: <desired rendered MachineConfig>
    validAfter: Immediate
```

The manual strategy requests no automated initiation of material updates. New and rebooting
nodes will only receive the desired configuration. From a metrics perspective, this strategy
is always restrictive.

#### Metrics

`change_management_change_pending`
Labels:
- kind=ClusterVersion|MachineConfigPool|HostedCluster|NodePool
- object=<object-name>
- system=<control-plane|worker-nodes>

Value: 
- `0`: no material changes are pending.
- `1`: changes are pending but being initiated.
- `2`: changes are pending and blocked based on this resource's change management policy.

`change_management_next_change_eta`
Labels:
- kind=ClusterVersion|MachineConfigPool|HostedCluster|NodePool
- object=<object-name>
- system=<control-plane|worker-nodes>

Value: 
- `-2`: Error determining or not yet ready to determine value (e.g. ChanageManagementPolicy Ready=False). 
- `-1`: Material changes are paused indefinitely (e.g. `Restrictive` strategy).  
- `0`: Material changes can be initiated now (e.g. change management is Permissive or inside machine schedule window). 
- `> 0`: The number seconds remaining until changes can be initiated.

`change_management_permissive_remaining`
Labels:
- kind=ClusterVersion|MachineConfigPool|HostedCluster|NodePool
- object=<object-name>
- system=<control-plane|worker-nodes>

Value: 
- `-2`: Error determining or not yet ready to determine value (e.g. ChanageManagementPolicy Ready=False).
- `-1`: Material changes are permitted indefinitely (e.g. `Permissive` strategy). 
- `0`: Material changes are not presently permitted (e.g. `Restrictive` strategy).
- `> 0`: The number seconds remaining in the current permissive change window. 

`change_management_last_change`
Labels:
- kind=ClusterVersion|MachineConfigPool|HostedCluster|NodePool
- object=<object-name>
- system=<control-plane|worker-nodes>

Value: 
- `-1`: Datetime unknown. 
- `0`: Material changes are currently permitted.
- `> 0`: The approximate number of seconds which have elapsed since the material changes were last permitted or initiated.

`change_management_strategy_enabled`
Labels:
- kind=ClusterVersion|MachineConfigPool|HostedCluster|NodePool
- object=<object-name>
- system=<control-plane|worker-nodes>
- strategy=MaintenanceSchedule|Permissive|Restrictive

Value: 
- `0`: Change management for this resource is not subject to this enabled strategy.
- `1`: Change management for this resource is subject to this enabled strategy.

#### Change Management Bypass Annotation
In some situations, it may be desirable for a MachineConfig to be applied regardless of the active change
management policy for a MachineConfigPool. In such cases, `machineconfiguration.openshift.io/bypass-change-management`
can be set to any non-empty string. The MCO will progress until MCPs which select annotated
MachineConfigs have all machines running with a desiredConfig containing that MachineConfig's current state.

This annotation will be present on `00-master` to ensure that, once the CVO updates the MachineConfig,
the remainder of the control-plane update will be treated as a single material change.

Rolling out critical machine config changes for worker nodes also is made easier with this annotation. 
Instead of, for example, trying to predict a `PermissiveUntil` date, an SRE/operations team can use this 
annotation to specify their goal with precision. Compare this with `PermissiveUntil`, which 
(a) an operations team would generally overestimate in order to ensure that updates complete and 
(b) may cause subsequent, non-critical, machine configuration changes to cause further workload disruptions
that are unwarranted.

### Special Handling

#### Change Management on Master MachineConfigPool
Changes to the master MachineConfigPool should only result in material changes to the control-plane
in standalone mode when a ChangeManagementPolicy referenced by the ClusterVersion is permissive.
To achieve this, the CVO will continuously reconcile to ensure that the `spec.changeManagement` stanza 
in the master MachineConfigPool always matches the values configured in ClusterVersion object.

When a non-CVO user (e.g. a human or gitops) modifies a MachineConfig that feeds into the master 
MachineConfigPool, this reconciliation ensures that the change will not result in material changes to the
master machines until the change management policy the CVO references is permissive.

Once the CVO initiates changes for the control-plane, all changes to the master machines must be completed, even if the permissive window
ends. This could create a small race condition -- imagine a user quickly toggles the CVO change management 
policy to permissive and then back to restrictive in ClusterVersion.
This would allow the CVO to roll out changes to cluster operators, but, by the time the MachineConfig
for the master MCP is updated, the permissive window could be restrictive again. This would leave the 
control-plane update only partially complete. 

To close this race condition, the CVO should annotate its master MachineConfig updates with 
`machineconfiguration.openshift.io/bypass-change-management`. This means that the CVO managed MachineConfig 
will work its way through to updating the master machines without being subject to change
management. 

Whether this annotation is present or not, once initiated, changes to the master machines should
continue without being subject to change management. This means that even non-CVO configured changes
to MachineConfig are rolled out consistently across the master machines.

#### Service Delivery Option Sanitization
It is obvious that the range of flexible options provided by change management configurations offers
can create risks for inexperienced cluster lifecycle administrators. For example, setting a 
standalone cluster to use the restrictive strategy and failing to trigger worker-node updates will
leave unpatched CVEs on worker-nodes much longer than necessary. It will also eventually lead to
the need to resolve version skew (Upgradeable=False will be reported by the API cluster operator).

Service Delivery understands that exposing the full range of options to cluster
lifecycle administrators could dramatically increase the overhead of managing their fleet. To
prevent this outcome, Service Delivery will only expose a subset of the change management
strategies. They will also implement sanitization of the configuration options a user can
supply to those strategies. For example, a simplified interface in OCM for building a
limited range of recurrence rules that are compliant with Service Delivery's update policies.

#### Node Disruption Policy
https://github.com/openshift/enhancements/pull/1525 describes the addition of `nodeDisruptionPolicy`
to `MachineConfiguration`. Through this configuration, an administrator can convey that
a configuration should not trigger a node to be rebooted / drained / etc.

Since arbitrary changes being introduced through MachineConfig, even with a potentially
non-disruptive nodeDisruptionPolicy may still materially impact the functionality of
a node, `nodeDisruptionPolicy` values cannot be used to bypass change management policy.  

### Risks and Mitigations

- Given the range of operators which must implement support for change management, inconsistent behavior or reporting 
  may make it difficult for users to navigate different profiles.
- Users familiar with the fully self-managed nature of OpenShift are confused by the lack of material changes be 
  initiated when change management constraints are active.
   - Mitigation: The introduction of change management will not change the behavior of existing clusters. 
     Users must make a configuration change.
- Users may put themselves at risk of CVEs by being too conservative with worker-node updates.
- Users leveraging change management may be more likely to reach unsupported kubelet skew configurations 
  vs fully self-managed cluster management.
  - Mitigation: For standalone, clusters will enter Upgradeable=False if another control-plane upgrade would
    cause unsupported skew. This should encourage cluster lifecycle administrators to update their worker-nodes.

### Drawbacks

The scope of the enhancement - cutting across several operators requires multiple, careful implementations. The enhancement
also touches code paths that have been refined for years which assume a fully self-managed cluster approach. Upsetting these
code paths may prove challenging. 

## Open Questions [optional]

1. Can the HyperShift Operator expose a metric for when changes are pending for a subset of worker nodes on the cluster if it can only interact via CAPI resources?

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
The API extensions will be made to existing, stable APIs. `changeManagement` is an optional
field in the resources which bear it and so do not break backwards compatibility.

The lack of a change management field implies the Permissive strategy - which ensures 
the existing, fully self-managed update behaviors are not constrained. That is,
until a change management strategy is configured, the behavior of existing clusters
will not be affected.

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- The `MachineConfigPool.spec.pause` can begin the deprecation process. Change Management strategies allow for a superset of its behaviors.
- We may consider deprecating `HostCluster.spec.pausedUntil`. HyperShift may consider retaining it with the semantics of pausing all reconciliation with CAPI resources vs just pausing material changes per the change management contract.

## Upgrade / Downgrade Strategy

Operators implementing support for change management will carry forward their
existing upgrade and downgrade strategies.

## Version Skew Strategy

Operators implementing change management for their resources will not face any
new _internal_ version skew complexities due to this enhancement, but change management 
does increase the odds of prolonged and larger differential kubelet version skew.

For example, particularly given long-lived Restrictive strategies, it 
becomes easier for a cluster lifecycle administrator to forget to update worker-nodes
along with updates to the control-plane. 

At some point, this will manifest as the kube-apiserver presenting as Upgradeable=False,
preventing future control-plane updates. To reduce the prevalence of this outcome,
the additional responsibilities of the cluster lifecycle administrator when 
employing change management strategies must be clearly documented along with SOPs
from recovering from skew issues.

HyperShift does not have any integrated skew mitigation strategy in place today. HostedCluster
and NodePool support independent release payloads being configured and a cluster lifecycle
administrator can trivially introduce problematic skew by editing these resources. HyperShift
documentation warns against this, but we should expect a moderate increase in the condition
being reported on non-managed clusters (OCM can prevent this situation from arising by
assessing telemetry for a cluster and preventing additional upgrades while worker-node
configurations are inconsistent with the API server). 

## Operational Aspects of API Extensions

The API extensions proposed by this enhancement should not substantially increase
the scope of work of operators implementing the change management support. The
operators will interact with the same underlying resources/CRDs but with
constraints around when changes can be initiated. As such, no significant _new_ 
operational aspects are expected to be introduced.

## Support Procedures

Change management problems created by faulty implementations will need to be resolved by 
analyzing operator logs. The operator responsible for a given resource will vary. Existing
support tooling like must-gather should capture the information necessary to understand
and fix issues. 

Change management problems where user expectations are not being met are designed to
be informed by the detailed `status` provided by the resources bearing the `changeManagement`
stanza in their `spec`.

## Alternatives

### Implement maintenance schedules via an external control system (e.g. ACM)
We do not have an offering in this space. ACM is presently devoted to cluster monitoring and does
not participate in cluster lifecycle.

### Do not separate control-plane and worker-node updates into separate phases
As separating control-plane and worker-node updates into separate phases is an important motivation for this
enhancement, we could abandon this strategic direction. Reasons motivating this separation are explained 
in depth in the motivation section.

### Separate control-plane and worker-node updates into separate phases, but do not implement the change control concept
As explained in the motivation section, there is a concern that implementing this separation without
maintenance schedules will double the perceived operational overhead of OpenShift updates. 

This also creates work for our Service Delivery team without any platform support.

### Separate control-plane and worker-node updates into separate phases, but implement a simpler MaintenanceSchedule strategy
We could implement change control without `permissiveUntil`, `Restrictive`, `exclude`, and perhaps more. However,
it is risky to impose a single opinionated workflow onto the wide variety of consumers of the platform. The workflows
described in this enhancement are not intended to be exotic or contrived but situations in which flexibility
in our configuration can achieve real world, reasonable goals. 

`permissiveUntil` is designed to support our Service Delivery team who, on occasion, will need
to be able to bypass configured change controls. The feature is easy to use, does not require 
deleting or restoring customer configuration (which may be error-prone), and can be safely 
"forgotten" after being set to a date in the future.

`Restrictive`, among other interesting possibilities, offers a cluster lifecycle administrator the ability
to stop a problematic update from unfolding further. You may have watched a 100 node
cluster roll out a bad configuration change without knowing exactly how to stop the damage
without causing further disruption. This is not a moment when you want to be figuring out how to format
a date string, calculating timezones, or copying around cluster configuration so that you can restore
it after you stop the bleeding.

### Implement change management, but only support a MaintenanceSchedule
Major enterprise users of our software do not update on a predictable, recurring window of time. Updates
require laborious certification processes and testing. Maintenance schedules will not serve these customers
well. However, these customers may still benefit significantly from the change management concept --
unexpected / disruptive worker node drains and reboots have bitten even experienced OpenShift operators
(e.g. a new MachineConfig being contributed via gitops).

Alternative strategies inform decision-making through metrics and provide facilities for fine-grained control 
over exactly when material change is rolled out to a cluster. 

The "assisted" rollout scenario described in this proposal is also specifically designed to provide a foundation for 
the forthcoming `oc adm update worker-nodes` verbs. After separating the control-plane and
worker-node update phases, these verbs are intended to provide cluster lifecycle administrators the 
ability to easily start, pause, cancel, and even rollback worker-node changes.

Making accommodations for these strategies should be a subset of the overall implementation
of the MaintenanceSchedule strategy and they will enable a foundation for a range of 
different personas not served by MaintenanceSchedule.

### Use CRON instead of Recurrence Rule
The CRON specification is typically used to describe when something should start and 
does not imply when things should end. CRON also cannot, in a standard way,
express common semantics like "The first Saturday of every month."
