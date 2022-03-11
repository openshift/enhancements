---
title: Compact + Worker deployment as day0 operation
authors:
  - "@flaper87"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - TBD
creation-date: 2022-03-08
last-updated: 2022-03-08
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also: []
replaces: []
superseded-by: []
---

# Compact + Worker deployment as day0 operation

## Summary

This enhancement proposes to allow for deploying a 4-nodes cluster as a day 0
operation. The 4-nodes cluster would be composed by 3 schedulable master nodes
and a single worker, which provide users with a highly available cluster.

## Motivation

There are environments where only 4 nodes are available where 4-nodes clusters
are being deployed. We could serve OpenShift cluster administrators of these
environments better by allowing them to deploy a 4-nodes cluster as a day0
operation.

Today, in such environments, OpenShift cluster administrators can only deploy a
4-nodes clusters by first deploying a compact cluster (3 schedulable masters)
and then adding a host as a day 2 operation. This provides the same results as
aimed in this enhancement but with a poor and slower user experience.

Today, because of the current limitations in the installer, it takes
administrators significantly longer to achieve the desired topology, when only
4 nodes are available. Furthermore, by requiring more steps, we are making the
process more prone to user errors.

### Goals

List the specific goals of the proposal. How will we know that this has succeeded?

- Enable the installer to deploy a 4-nodes cluster composed by 3 schedulable
  masters and one worker.

### Non-Goals

- Define new infrastructure topologies

- Define new architectures

- Provide official support for the 4-nodes cluster deployments

## Proposal

- Set the InfrastructureTopology to have the same value as the
  ControlPlaneTopology when the number of workers is 1, instead of setting its
  value to SingleReplica as it's done today.

- Use the `ControlPlaneTopology` topology when the number of workers is 1,
  instead of the `SingleReplicaTopologyMode` as it's done today.

### User Stories

- As an OpenShift cluster administrator, I want to be able to install a 4-nodes
  cluster as a single installation step so that my cluster will be able to
  handle workloads that require replication.


### API Extensions

- Sets the `infrastructureTopology` field to the value of`ControlPlaneTopology` in the
  `config.openshift.io/v1/infrastructure` CR called `cluster`, which is created
  during the install process, when the number of workers is 1.

- Sets the `mastersSchedulable` field to true in the
  `config.openshift.io/v1/scheduler` CR called `cluster`, which is created
  during the install process, when the number of workers is 1.

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

This should not introduce newer risks other than the existing ones in a compact
cluster deployment.

## Design Details

### Open Questions [optional]

Nothing at the moment

### Test Plan


- Add unit tests to ensure the Infrastructure and Scheduler settings are properly set for the different combinations

- Add a periodic job (nightly?) job to run OpenShift tests on a 4-node cluster

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

TBD

### Upgrade / Downgrade Strategy

This enhancement does not have an impact on existing clusters as it's intended
to affect only day0 operations.

### Version Skew Strategy

Does not apply

### Operational Aspects of API Extensions


- Allows for a 4-nodes, highly available, cluster to be deployed as a day0
  task. It reduces the time required for such deployments to be created.

- Does not impact general deployment functionalities in a negative way.

- Allowing workloads on the master nodes may affect the master's health if the
  traffic and workloads are too high.

#### Failure Modes

Same as a compact cluster

#### Support Procedures

- The same support procedures used for a compact cluster should be used for a
  4-nodes cluster. I would appreciate help filling in this section with more
  details.


## Implementation History

N/A

## Drawbacks

A 4-nodes cluster (a.k.a 3+1) is generally not a recommended architecture. By
having schedulable masters users will be routing ingress traffic through the
masters as well as other workloads. Depending on the amount of traffic, and the
resources available to the cluster, this may result in degraded functionality
in the masters, which finally affects the general health of the cluster.

With this enhancement, we will be making it easier for users to create such a
4-nodes cluster deployments, which could make users believe it is supported or
recommended.

## Alternatives

- Don't set masters as schedulable automatically. Instead, cluster
  administrators will have to explicitly set masters as schedulable in the
  deployment configuration. The rest of the proposal would remain the same,
  switching the cluster to `ControlPlaneTopology` if there are 3 schedulable
  masters and 1 worker.

- Don't do anything and force users to do this in a 2 steps operation: first
  deploy a compact cluster and then add a worker node.

## Infrastructure Needed [optional]

N/A
