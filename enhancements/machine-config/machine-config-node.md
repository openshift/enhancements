---
title: machine-config-node
authors:
  - "@cdoern"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@sinnykumari" # MCO
  - "@yuqi-zhang" # MCO
approvers:
  - "@sinnykumari"
  - "@yuqi-zhang"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
creation-date: 2023-10-05
last-updated: 2023-10-20
tracking-link:
  - https://issues.redhat.com/browse/MCO-452
see-also:
replaces:
superseded-by:
---

# Track MCO State Related to Upgrades in the MachineConfigNode type

## Summary

This enhancement describes how Nodes and their upgrade processes should be aggregated by the MCO into a user facing object. The goal here is to allow customers and the MCO team to decipher our processes in a more verbose way, speeding up the debugging process and allowing for better customer engagement.

## Motivation

The MCO manages node upgrades but since we do not own the object or store much of their data in other ways, much of what occurs during an upgrade
is simply a black box operation that we currently report as "Updating" or "Updated". Users can debug into a specific node or look into the node spec for some of this information
but most of it simply lives in the MCO code rather than in data structures. We want to put these abstract "phases" of node operations as triggered by the MCO into a concrete data structure.

This feature is more tied to MCO procedures than the state reporting of the MachineConfigPool. We are designing this to fill the gap between what the MachineConfigPool currently reports and what is actually happening in the MCO pertaining to Node updates. One can view this as an API tied to MCO procedures. However, these objects are a way to track node update status and since the MCO owns the update code, it just so happens that a lot of these actions are tied to the MCO.

### User Stories

* As a cluster admin, during upgrades I would like to monitor the progress of individual node upgrades so that, if one of the nodes sticks during an upgrade, I can easily see where the node got stuck and why.
* As an OpenShift developer, I would like a place to observe the MCO's view of a Node so that I can easily identify if the MCO is related to an upgrade failure in CI.

### Goals

* Make a MachineConfigNode type that succinctly holds the upgrade data.
* Have API load be as minimal as possible but augment the proper objects as needed.
* aggregate as much MCO related data into easily accessible places as possible.

### Non-Goals

* Modify or remove existing API and status fields

## Proposal
Create a Datatype for tracking Node Upgrade Progression in the MCO as well as Operator Component Progression in the MCO.

Create a mechanism inside of the MCO to update the new MachineConfigNode API type
with data about a Node's progress during an upgrade and any errors that occur during those
processes.

The MCD owns this datatype. Inside of the MachineConfigDaemon there now is an "UpgradeMonitor" package which contains all of the logic to manage these MachineConfigNodes.
Since the Daemon owns all code related to updates (outside of a few drain controller functions), the MachineConfigDaemon has full ownership over these new objects.
During the process of an upgrade, the MachineConfigDaemon will call the upgrade monitor with key information related to OCP updates and it will update the spec and status of the MachineConfigNodes.

The MachineConfigOperator pod will manage the rollout of the initial objects and the CRD for MachineConfigNodes. The MachineConfigOperator pod manages all manifests and resources which the operator owns, making this the best place for resource management of the MachineConfigNode to live.

The data created by this MachineConfigNode would look like the following:

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   True      False             False              False                    False              False
ip-10-0-17-102.ec2.internal   True      False             False              False                    False              False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False              False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False              False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False              False
ip-10-0-6-214.ec2.internal    True      False             False              False                    False              False
```

as well as 

```console
$ oc get machineconfignodes -o wide
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED    RESUMED   UPDATECOMPATIBLE     UPDATEDFILESANDOS       DRAINEDNODE    CORDONEDNODE   REBOOTEDNODE   RELOADEDCRIO  UNCORDONEDNODE
ip-10-0-1-37.ec2.internal     True      False             False              False                    False              False      False                     False                False          False           False           False       False
ip-10-0-33-243.ec2.internal   True      False             False              False                    False              False      False                     False                False          False           False           False       False
ip-10-0-37-208.ec2.internal   True      False             False              False                    False              False      False                     False                False          False           False           False       False
ip-10-0-39-225.ec2.internal   False     False             False              True                     False              False      False                     False                False          False           True            False       False
ip-10-0-51-162.ec2.internal   True      False             False              False                    False              False      False                     False                False          False           False           False       False
ip-10-0-59-150.ec2.internal   False     False             False              True                     False              False      False                     False                False          False           True            False       False
```

Where each name represents a node. The statuses reported are created explicitly from MCO node annotations and MCO actions, no other operator actions are taken into account here. This allows us to get quite specific in what is occuring on the nodes.

```console
Name:         ip-10-0-12-194.ec2.internal
Namespace:    
Labels:       <none>
Annotations:  <none>
API Version:  machineconfiguration.openshift.io/v1alpha1
Kind:         MachineConfigNode
Metadata:
  Creation Timestamp:  2023-10-17T13:08:58Z
  Generation:          1
  Resource Version:    49443
  UID:                 4bd758ab-2187-413c-ac42-882e61761b1d
Spec:
  Node Ref:
    Name:         ip-10-0-12-194.ec2.internal
  Pool:
    Name:         master
  ConfigVersion:
    Desired: rendered-worker-823ff8dc2b33bf444709ed7cd2b9855b
Status:
  Conditions:
    Last Transition Time:  2023-10-17T13:09:02Z
    Message:               Node has completed update to config rendered-master-cf99e619747ab19165f11e3546c71f1e
    Reason:                NodeUpgradeComplete
    Status:                True
    Type:                  Updated
    Last Transition Time:  2023-10-17T13:09:02Z
    Message:               This node has not yet entered the UpdatePreparing phase
    Reason:                NotYetOccured
    Status:                False
  Config Version:
    Current:            rendered-worker-823ff8dc2b33bf444709ed7cd2b9855b
    Desired:            rendered-worker-823ff8dc2b33bf444709ed7cd2b9855b
  Health:               Healthy
  Most Recent Error:    
  Observed Generation:  3
```

The above struct gives us some helpful information about a node as it pertains to the MCO. First, all upgrade related events that have occured on the node from the most recent uprade process (no matter how small) appear in the `conditions`. Other than that, you can see the current and desired machineconfig indicating whether or not the node should be updating as well as whether or not the currently tracked update process held in `conditions` is updating to the expected MachineConfig. The Node reference and ObservedGeneration exist to let the user know some extra information on the object and how many times we have gone throgh some upgrade related changes.

The desired config found in the spec will get updated immediately when a new config is found on the node. However, the desired config found in the status will only get updated once the new config has been validated in the machine config daemon. In the current implementation this is done simply by checking what phase of the update we are in. If the update successfully gets past the "UpdatePrepared" phase, then the status can safely add the desired config. 

The states to be reported by this MachineConfigNode will roughly fall into the following:

#### Prepared phase
 - Stopping config drift monitor
 - Reconciling configs
#### Executed Phase
- Cordoned Node
- Drained Node
- Updated on disk state
- Updated OS
#### Post Config Action Phase
- Rebooting
- Closing daemon
- Node Reboot **OR** Reloading crio
#### Completed Phase
- Uncordoned
- Updating node state and metrics
#### Resumed Phase
- Start config drift monitor
#### Error states:
   - update stuck -- specifically degraded and/or failures in the functions. This is the phase that happens if we either error in any of the above stages
   - unavailable nodes... an obstacle has been hit, but its not the MCOs fault
     - Disk Pressure
   	 - Unschedulable

This loops until we are inDesiredConfig

More Paths that will update the node state:


### Workflow Description

When an upgrade is triggered by there being mismatch between a desired and current config or simply just a new MachineConfig being applied, the MachineConfigNodes for a specific pool will report the following processes (roughly)


Please note: the flow from False -> Unknown -> True and then back to False is still up for debate.

With the implementation the MCO is introducing in 4.15, The MCN objects are meant to track upgrade progression of nodes as impacted by the MCO. the general progression here is
- False == this phase has not started yet during the most recent upgrade process
- Unknown == this phase is either being executed or has errored. If the phase has errored `oc describe machineconfignodes` will display more information in the metav1.Conditions
- True == this phase is complete

The phases shown in `oc get machineconfignodes` are the parent phases. using `oc describe machineconfignodes` will reveal the parent and child phases. Within each parent phase there can be 0+ child phases that customers can use to see upgrade progression. 


```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   True      False             False              False              False              False
ip-10-0-17-102.ec2.internal   True      False             False              False              False              False
ip-10-0-2-232.ec2.internal    True      False             False              False              False              False
ip-10-0-59-251.ec2.internal   True      False             False              False              False              False
ip-10-0-59-56.ec2.internal    True      False             False              False              False              False
ip-10-0-6-214.ec2.internal    True      False             False              False              False              False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     Unkown            False              False              False              False
ip-10-0-17-102.ec2.internal   True      False             False              False              False              False
ip-10-0-2-232.ec2.internal    True      False             False              False              False              False
ip-10-0-59-251.ec2.internal   True      False             False              False              False              False
ip-10-0-59-56.ec2.internal    True      False             False              False              False              False
ip-10-0-6-214.ec2.internal    True      False             False              False              False              False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              False              False                    False              False
ip-10-0-17-102.ec2.internal   True      False             False              False                    False              False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False              False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False              False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False              False
ip-10-0-6-214.ec2.internal    True      False             False              False                   False              False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              Unknown            False                    False              False
ip-10-0-17-102.ec2.internal   True      False             False              False                    False              False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False              False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False              False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False              False
ip-10-0-6-214.ec2.internal    True      False             False              False                    False              False
```



```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              True               False                    False              False
ip-10-0-17-102.ec2.internal   True      False             False              False                    False              False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False              False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False              False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False              False
ip-10-0-6-214.ec2.internal    True      False             False              False                    False              False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              True               Unknown                 False              False
ip-10-0-17-102.ec2.internal   True      False             False              False                   False              False
ip-10-0-2-232.ec2.internal    True      False             False              False                   False              False
ip-10-0-59-251.ec2.internal   True      False             False              False                   False              False
ip-10-0-59-56.ec2.internal    True      False             False              False                   False              False
ip-10-0-6-214.ec2.internal    True      False             False              False                   False              False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              True               True                    False              False
ip-10-0-17-102.ec2.internal   True      False             False              False                   False              False
ip-10-0-2-232.ec2.internal    True      False             False              False                   False              False
ip-10-0-59-251.ec2.internal   True      False             False              False                   False              False
ip-10-0-59-56.ec2.internal    True      False             False              False                   False              False
ip-10-0-6-214.ec2.internal    True      False             False              False                   False              False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              True               True                    Unknown           False
ip-10-0-17-102.ec2.internal   True      False             False              False                    False             False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False             False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False             False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False             False
ip-10-0-6-214.ec2.internal    True      False             False              False                    False             False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              True               True                     True               False
ip-10-0-17-102.ec2.internal   True      False             False              False                    False              False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False              False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False              False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False              False
ip-10-0-6-214.ec2.internal    True      False             False              False                    False              False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              True               True                     True              Unknown
ip-10-0-17-102.ec2.internal   True      False             False              False                    False             False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False             False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False             False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False             False
ip-10-0-6-214.ec2.internal    True      False             False              False                    False             False
```

```console
$ oc get machineconfignodes
NAME                          UPDATED   UPDATEPREPARED   UPDATEEXECUTED   UPDATEPOSTACTIONCOMPLETED   UPDATECOMPLETED   RESUMED
ip-10-0-12-194.ec2.internal   False     True              True               True                     True              True
ip-10-0-17-102.ec2.internal   True      False             False              False                    False             False
ip-10-0-2-232.ec2.internal    True      False             False              False                    False             False
ip-10-0-59-251.ec2.internal   True      False             False              False                    False             False
ip-10-0-59-56.ec2.internal    True      False             False              False                    False             False
ip-10-0-6-214.ec2.internal    True      False             False              False                    False             False
```

The general process here goes as follows:

A state (and its respective child states) will generally go from False -> Unkown -> True -> False.
The states are mostly in the past tense. This is because processes like `Drained` are defined by completion primarily not the progress. So a user will have `UpdateExecuted` == Unknown and `Drained` == Unkown until the Drain actually completes.
However, the unknown phase will be accompanied by a message for how the drain is currently going or if the drain has gone wrong.

The condition transitions back to False once the update process is completed. Once Updated == true is the case, all previous states get set to false and their reasons/messages show the fact that this was the message from the previous update cycle.

and if you wanted to look at oc describe for more verbose details of other events in an upgrade cycle that happened on the node, you can do so using `oc describe machineconfignodes/<name>`
(output below is not correlated to the above example)

```console
Name:         ip-10-0-52-193.ec2.internal
Namespace:    
Labels:       <none>
Annotations:  <none>
API Version:  machineconfiguration.openshift.io/v1alpha1
Kind:         MachineConfigNode
Metadata:
  Creation Timestamp:  2023-11-22T20:05:18Z
  Generation:          1
  Resource Version:    190396
  UID:                 78526c4d-206c-4ec7-8ae1-c47aebcb79b3
Spec:
  Config Version:
    Desired:  NotYetSet
  Node:
    Name:  ip-10-0-52-193.ec2.internal
  Pool:
    Name:  worker
Status:
  Conditions:
    Last Transition Time:  2023-11-22T20:05:36Z
    Message:               Action during previous iteration: In desired config rendered-worker-7f183a799b1bce71eca1b49fa20c7261. Resumed normal operations.
    Reason:                Resumed
    Status:                False
    Type:                  Resumed
    Last Transition Time:  2023-11-22T20:08:46Z
    Message:               Update is Compatible.
    Reason:                UpdateCompatible
    Status:                True
    Type:                  UpdatePrepared
    Last Transition Time:  2023-11-22T20:08:48Z
    Message:               Draining Node as part of In progress upgrade phase
    Reason:                Drained
    Status:                Unknown
    Type:                  UpdateExecuted
    Last Transition Time:  2023-11-22T20:05:26Z
    Message:               This node has not yet entered the UpdatePostActionComplete phase
    Reason:                NotYetOccured
    Status:                False
    Type:                  UpdatePostActionComplete
    Last Transition Time:  2023-11-22T20:05:26Z
    Message:               This node has not yet entered the UpdateComplete phase
    Reason:                NotYetOccured
    Status:                False
    Type:                  UpdateComplete
    Last Transition Time:  2023-11-22T20:08:46Z
    Message:               Update Compatible. Post Cfg Actions [reboot]: Drain Required: true
    Reason:                UpdatePreparedUpdateCompatible
    Status:                True
    Type:                  UpdateCompatible
    Last Transition Time:  2023-11-22T20:08:48Z
    Message:               Draining node. The drain will not be complete until desired drainer drain-rendered-worker-3b7e2ff302f33d45a732e03564405ae7 matches current drainer uncordon-rendered-worker-7f183a799b1bce71eca1b49fa20c7261
    Reason:                UpdateExecutedDrained
    Status:                Unknown
    Type:                  Drained
    Last Transition Time:  2023-11-22T20:05:26Z
    Message:               This node has not yet entered the AppliedFilesAndOS phase
    Reason:                NotYetOccured
    Status:                False
    Type:                  AppliedFilesAndOS
    Last Transition Time:  2023-11-22T20:08:48Z
    Message:               Cordoned node. The node is reporting Unschedulable = true
    Reason:                UpdateExecutedCordoned
    Status:                True
    Type:                  Cordoned
    Last Transition Time:  2023-11-22T20:05:26Z
    Message:               This node has not yet entered the RebootedNode phase
    Reason:                NotYetOccured
    Status:                False
    Type:                  RebootedNode
    Last Transition Time:  2023-11-22T20:05:26Z
    Message:               This node has not yet entered the ReloadedCRIO phase
    Reason:                NotYetOccured
    Status:                False
    Type:                  ReloadedCRIO
    Last Transition Time:  2023-11-22T20:08:46Z
    Message:               Node ip-10-0-52-193.ec2.internal needs an update
    Reason:                Updated
    Status:                False
    Type:                  Updated
  Config Version:
    Current:            rendered-worker-7f183a799b1bce71eca1b49fa20c7261
    Desired:            rendered-worker-3b7e2ff302f33d45a732e03564405ae7
  Observed Generation:  2
Events:                 <none>
```

There are two levels of conditions in the MachineConfigNode type: the parent and the child condition. Parent conditions incude Updated, UpdatePrepared, UpdateExecuted, UpdatePostAction, UpdateComplete, and Resumed. These parent conditions track the overall arc of an upgrade. However, there are often multiple phases within these overarching ones. Therefore: UpdateCompatible, DrainedNode, ApplyingFilesAndOS, CordonedNode, RebootedNode, ReloadedCRIO, and UncordonedNode are the ChildrenPhases that occur during the larger ones. These don't always occur depending on the type of update. However, sometimes they all occur, leading to some confusion in the parent phases as to what happened. Adding thse children phases is meant to give the user more clsrity to the update at hand.

For example. When a CRIO reload is not required on a node, the condition will look like this: 

```console
    Last Transition Time:  2024-01-12T14:47:21Z
    Message:               This node has not yet entered the ReloadedCRIO phase
    Reason:                NotYetOccured
    Status:                False
    Type:                  ReloadedCRIO
```

or 

```console
    Last Transition Time:  2024-01-12T14:52:19Z
    Message:               Action during update to rendered-master-eef3a1a1a422b7ebd085b346042ca5a8: Upgrade required a CRIO reload. Completed this as the post update action.
    Reason:                UpdatePostActionCompleteReloadedCRIO
    Status:                False
    Type:                  ReloadedCRIO
```

The first option here indicates that this phase has never happened. The second one indicates that is has happend, just not during this update cycle. That is what the `Action during update to...` shows. That rendered config is not the one we are updating to currently.


The MCO in 4.15 is aiming to use these objects to improve the source of truth for MCP reporting. If a user is opted into TechPreview, the MachineConfigPools will pull their `Updated`, `Updating`, and `Degraded` statuses from the MCN objects rather than from the nodes themselves. There are a few reasons for this: 

1. Pulling the MCP state from the nodes in the pool results in reported behaviors that are outside of the MCOs control for example Cordoning and Draining. The MCO currently reports to Upgradeable=False in the CO when the node is Cordoned by an outside actor. This should only be the case if the MCO is attempting to Cordon and Drain a node. This also means that the Pool shows Updating=True which isn't the case.
2. The MCO goes Upgradeable=False when new nodes are added. This is because these new nodes are reporting that they are not ready when they haven't even joined the Pool correctly yet. Since the MCP directly pulls its status from the nodes, this causes all sorts of issues. The MCN object will wait until the node is all settled in the pool before using it for state reporting meaning the MCO should show Upgradeable=True


The MCN is meant to clarify state reporting for the users and disambiguate these edge cases.

### API Extensions

- Adds the MachineConfigNodes CRD

### Risks and Mitigations

There might be users who do not know to look here. We will mititagte this by reporting in the CO and/or the node status go to
and look at this object for MCO related node failures or progression.

Adding new MCD node states might have unintended consequences on the current flow which just has "Working" and "Done". Will make sure the
states I add are either only in between Working and Done OR no matter what we end up back in the "Done" state.

### Drawbacks

The only drawback is increased API usage in the MCO. However, any approach to increased state reporting will increase API calls. This is the most well thought approach to this. 

## Design Details

### Open Questions [optional]

None.

### Test Plan

MCO e2e tests and unit tests will cover this functionality.

### Graduation Criteria

This feature is behind the tech-preview FeatureGate in 4.15. Once it is tested by QE and users it can be GA'd since it should not impact daily usage of a cluster.

## Dev Preview -> Tech Preview

Not applicable. Feature introduced in Tech Preview. 

## Tech Preview -> GA

Bugs that QE finds get fixed and MCN is proven to handle pool status updating properly.

Also add support for the following features

1. https://issues.redhat.com/browse/MCO-1022
2. https://issues.redhat.com/browse/MCO-1023
3. https://issues.redhat.com/browse/MCO-1024
4. https://issues.redhat.com/browse/MCO-1025

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Between upgrades, this feature should not have a large impact. It will preform as it usually does and it will track node processes in both upgrades and downgrades. If a node is added or removed, the MachineConfigNode object will not report on it until the node is acknowledged by the MCO.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

#### Failure Modes

If the MachineConfigNode cannot be updated, an error is logged but the operator keeps functioning. If an error happens during an update it is represented in the MachineConfigNode clearly.

#### Support Procedures

None.

## Implementation History

As of Openshift 4.15, the feature went under the tech-preview FeatureGate. The basic functionality was implemented and provides users with a way to track node progressions during updates.
In 4.16, the UncordonedNode column was added to the -o wide output. Many bugs were fixed as well in this version with creation and deletion of nodes.

## Alternatives

Implementation wise, there has been discussion of who should own these objects: the MachineConfigDaemon or a new Controller called the MachineStateController. While either approach would work, there are positive and negative impacts to each.

The approach that was decided on for Openshift 4.15 was the MachineConfigDaemon approach. While some of the above proposal contradicts MCO sentiment regarding the size of the MachineConfigDaemon, there are future plans to separate the update functionality of the MachineConfigDaemon out of the everyday file operations and management of the MachineConfigDaemon. This makes the MachineConfigDaemon much mor favorable given that it has less overhead and apiserver impact.


## MachineStateController Approach

using a seprate controller allows for information consolidation and furthers the MCO's goals. However, it adds a level of information between the source and sink of this data.

1) Confused Deputy
  - An improper actor could potentially modify information on the node such that the MachinestateController thinks an upgrade event is going on but in reality it is not
    - Nothing "consumes" the MCN data type in this design. However, this is still a future concern and one that has been voiced by many teams.
2) Unecessary level of abstraction
   - the MCD is the source of the data so it should "own" the updating process of the MCN. This is true and fits into the idomatic processes of kubernetes. However, the goal of the MCO was to consolidate all state and metric reporting into a single location. It is a question of prioritizing data efficiency 

The summary of the two alternatives is as follows:

the MCD owning method furthers k8s and openshift api standards. The MachineStateController owning method furthers the MCO's goals in the next 6-12 months. It seems the balance we will strike is going to lean towards k8s standards.
