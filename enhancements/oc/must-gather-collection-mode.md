---
title: must-gather-collection-mode
authors:
  - "@ardaguclu"
reviewers:
  - "@ingvagabund"
approvers: 
  - "@deads2k"
api-approvers: 
  - "@deads2k"
creation-date: 2024-11-22
last-updated: 2024-11-22
tracking-link: 
  - https://issues.redhat.com/browse/OCPBUGS-37344
see-also:
  - "/enhancements/oc/must-gather.md"
  - "/enhancements/oc/inspect.md"
---

# must-gather: Collection Mode

## Summary

This proposal introduces a new flag (i.e. `--collection-mode`) in must-gather targeting to large clusters by skipping some
logs to take less time and storage size. In addition to that, this proposal introduces a new `--node-selector` flag
in oc adm inspect command to only collect the daemonset pods running on the given node selector.

## Motivation

must-gather, due to its nature, aims to collect every log in the cluster in best effort to provide extensive insights.
However, this comes with a drawback that on large clusters (e.g. clusters whose node count is greater than 20), completion
of the must-gather takes excessive time and storage which eventually hurts the usability (or worse, collection failure). As maintainers, we are usually
under the pressure of two opposite sides; adding more and more logs for better troubleshooting experience, cutting some logs
for short completion duration and less storage size. It is hard to find the optimum balance in regard to the default behavior.

As a result, we need to find a mitigation plan for large clusters by skipping some logs that are marked as less critical
meanwhile preserving the default behavior and expectations.

### User Stories

#### Story 1

As a cluster administrator maintaining 250 nodes of cluster, I want to have a mechanism to collect the logs quickly and efficiently. 
Besides, I'm fine skipping some logs and providing them separately. Because currently must-gather takes 6 hours that even may end up with a failure.

#### Story 2

As a cluster administrator maintaining 250 nodes of clusters. I usually need to troubleshoot networking
that are directly associated to the daemonset pod logs running on workers nodes. So that, I would like to collect everything without skipping
any log (accepting the long time and a risk of collection failure).

### Goals

1. Introducing a new `--collection-mode` in must-gather that will be used on large clusters.
2. Introducing a new `--node-selector` in inspect command that will be used to collect daemonsets logs only on the given node selector.

### Non-Goals

1. Changing the default behavior of must-gather and inspect.

## Proposal

### `oc adm must-gather`

There is a new flag in must-gather, namely `--collection-mode`. This flag's default value is set to `medium` that
represents the current behavior [1][2]. Its type is string rather than boolean because in the future, we may want to have different modes
such as "extensive" (collect everything that was skipped previously due to the time and size constraints, etc.).
Once user invokes the oc adm must-gather command with `--collection-mode=limited`,
must-gather command will export an environment variable `COLLECTION_MODE=limited` into its collection pod.

Since there are multiple must-gather images and none of them (apart from the default must-gather in here) does not
adopt this flag, `--collection-mode` will be marked as hidden.

### `oc adm inspect`

There is a new flag in inspect command, namely `--node-selector`. If this flag is empty, every log is collected to preserve
the default behavior. Once this flag is set, only the daemonset pod logs whose running on the given node selector will be collected and
the rest will be ignored. If ignored ones are necessary at some point during the troubleshooting, customer can run this command
with different node selectors separately (e.g. `--node-selector=!node-role.kubernetes.io/control-plane`).

### `must-gather` script

Default must-gather's gathering script checks the existence of `COLLECTION_MODE` environment variable and if it exists
and is set to `limited`, script passes `--node-selector=node-role.kubernetes.io/control-plane` in every "oc adm inspect" invocation.
This ensures that only the daemonset pod logs running on control plane are collected.

In the future, we can skip more resources based on this `COLLECTION_MODE=limited` (or add more resources based on different collection modes).
Skipping daemonset logs running on workers can be considered as a first attempt for the limited mode as it seems like the overt one.

### must-gather Images

Based on the adoption rate of the `COLLECTION_MODE` environment variable by other must-gather images, we can decide again
marking the flag in must-gather as visible.

[1] from must-gather point of view, current behavior represents [this gathering script](https://github.com/openshift/must-gather/blob/release-4.18/collection-scripts/gather)

[2] from oc adm inspect point of view, current behavior represents collecting [built-in resources](https://github.com/openshift/oc/blob/9e568296a9b2f774e9321825e2b0c314ee9df566/pkg/cli/admin/inspect/namespace.go#L22-L33), cluster operators, CRDs, webhooks, namespaces that are associated via related objects chain. 

### Workflow Description

There is no change in default behavior. However, on large clusters that is the typical usage of must-gather;

```shell
 oc adm must-gather --collection-mode=limited
```

If it is decided that the logs of daemonsets running on worker nodes are essential, additionally this command is an example;

```shell
 oc adm inspect namespace openshift-multus --node-selector='node-role.kubernetes.io/worker'
```

### API Extensions

There is no API related change.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No impact

#### Standalone Clusters

No impact

#### Single-node Deployments or MicroShift

No impact

### Implementation Details/Notes/Constraints

`--collection-mode` flag will be invisible and that would be difficult to find and use it without any prior knowledge.
People may complain about the excessive duration but in reality, there is a way we just don't to expose them.

### Risks and Mitigations

Some logs are skipped in _limited_ mode, even though they are essential. That causes the less usability of limited mode.
Because it is an additional requirement and back and forth to collect more data with a separate command.

### Drawbacks

This brings about an inevitable but slightly maintenance burden as it introduces new flags and environment variables.
We have to skip the collection of some logs due to the constraints we have and the drawback is some logs are not essential 
for some clusters but contrarily these logs could be very essential for some clusters. This proposal eliminates 
some logs which may end up that limited mode can't be usable for some clusters.

## Open Questions [optional]

None

## Test Plan

We have several pinpoints in our CI testing the must-gather's default behavior. That can assure that this change does not
break it.

## Graduation Criteria

### Dev Preview -> Tech Preview

In this case, we can merge Dev Preview and Tech Preview and we can have this;

* `--collection-mode` flag will be hidden in must-gather, but it is triggerable if you know the flag
* `--node-selector` flag will be visible but marked as experimental

### Tech Preview -> GA

* After the adoption of `--collection-mode` flag by other must-gather images, we can mark this flag as visible for general use
* `--node-selector` will be marked as GA by removing the experimental

### Removing a deprecated feature

None

## Upgrade / Downgrade Strategy

None

## Version Skew Strategy

* oc that is used to invoke must-gather is old, regardless of must-gather is new or old; no issue as the environment variable will not be set.
* oc that is used to invoke must-gather is new, must-gather script is old; no issue, environment variable will be ignored
* oc that is used to invoke must-gather is new, must-gather script is new, oc in must-gather script is old; since the old oc in must-gather does not know `--node-selector`, it will fail
* oc that is used to invoke must-gather is new, must-gather script is old, oc in must-gather script is new; since must-gather script does not pass `--node-selector`, no issue

Only the 3rd case gets an error.

## Operational Aspects of API Extensions

No API changes

## Support Procedures

Collection fails with an error and default collection behavior can be used.

## Alternatives

Previously without exposing any flags, we tested this rule as default behavior;

```markdown
Do not collect daemonset pods logs running on worker nodes, if the cluster's worker node count is greater than 18
```

However, people whose are responsible from the large clusters do not strongly embrace this rule and push towards decreasing worker node threshold to fewer values  
to include the mid/small clusters (because cluster size is subjective topic based on the number of nodes). 
Additionally, people whose are responsible from the network or MCO components do not embrace this rule as they strictly rely on the daemonset logs we skip as default and
now they have to ask for an additional command execution.

## Infrastructure Needed [optional]

Not applicable
