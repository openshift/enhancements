---
title: protecting-etcd-quorum-during-control-plane-scaling
authors:
  - "@JoelSpeed"
reviewers:
  - "@hexfusion"
  - "@hasbro17"
approvers:
  - "@hexfusion"
creation-date: 2021-08-20
last-updated: 2021-11-18
status: implementable
see-also:
  - "[Machine Deletion Hooks](https://github.com/openshift/enhancements/pull/862)"
---

# Protecting etcd Quorum During Control Plane Scaling

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

To enable automation of Control Plane scaling activities, in particular vertical scaling of the Control Plane Machines,
we must implement a mechanism that protects etcd quorum and ensures the smoothest possible transition as new etcd
members are added and old members removed from the etcd cluster.

## Motivation

As Red Hat expands its managed services offerings, the ability to safely vertically scale the capacity of an
OpenShift Control Plane in some automated manner becomes imperative.

Currently, when a cluster starts to hit capacity limits on the Control Plane, a very involved manual process is
required to not only add new Machines to the cluster, but monitor and manage the etcd cluster to ensure that the quorum
is preserved throughout the operation.

This process is not sustainable and we must provide safety mechanisms on top of the existing etcd quorum guard to make
this process both easier and safer.

### Goals

* Provide the etcd operator with the ability to control when a Control Plane Machine is removed from the cluster
* Allow the etcd operator to prevent removal of etcd members until a replacement member has been promoted to a voting
  member
* Allow the etcd operator to remove an etcd member from the etcd cluster before the Machine is terminated to prevent a
  degraded etcd cluster
* Allow an escape hatch from the protection when surging the capacity with new Control Plane Machines is unavailable
  (for example in metal environments with limited capacity)


### Non-Goals

* Automation of scaling operations on Machines
* Horizontal scaling of the etcd cluster and Control Plane
* Providing these protection mechanisms when the Machine API is [not functional](#When-is-Machine-API-Functional)
* Recovering unhealthy etcd clusters

## Proposal

### User Stories

#### Story 1

As an operator of a managed OpenShift cluster, I want to be able to automate the scaling operations of Control Plane
Machines without having to manually ensure the safety of the etcd cluster.

#### Story 2

As a developer of OpenShift, I want to implement safety mechanisms that adhere to best practices for etcd scaling
operations to protect end users from potential quorum losses and data losses.

#### Story 3

As an end user of OpenShift, I want to be able to increase the size of my Control Plane without having to know the
intricacies of etcd and protecting its quorum.

### API Extensions

This enhancement does not introduce any new API extensions.

### Implementation Details/Notes/Constraints

#### Requirements for etcd safety

* The number of voting members should equal the desired number of Control Plane Machines (and this should be an odd
  number)
  * We will only deviate from this to add a replacement member
  * Once the new member is added, we should remove the old member as soon as possible to reduce the risk of degrading the
    cluster while having an even number of voting members
* Existing voting members should not be removed until their replacements have been promoted to voting members
  * By starting new etcd members as [learners](https://etcd.io/docs/v3.3/learning/learner/#raft-learner), we can ensure
    that the new member is fully "caught-up" and promotable to a full voting member before we start the removal process
    of the old member
  * This protects etcd from potential inconsistencies in its data if the cluster were to have some interruption shortly
    after a new member joins the cluster
  * This also ensures that we keep the full etcd data on disk on a minimum of the desired number of Control Plane
    Machines at all times

#### Protecting etcd during scaling operations

To ensure that the safety requirements described above are maintained during scaling operations,
the etcd operator will manage the etcd cluster in clusters with a
[functional Machine API](#When-is-Machine-API-Functional),
by leveraging [Machine Deletion Hooks](#What-are-Machine-Deletion-Hooks) to coordinate with the Machine API when it is,
and isn't safe, to remove Machines with voting members of the etcd cluster running on them.

##### Overview of protecion mechanism

To ensure the safety of the etcd cluster quorum, the etcd operator will leverage a pre-drain [Machine Deletion Hooks](#What-are-Machine-Deletion-Hook) to prevent the removal of any Control Plane Machine hosting a voting member of the etcd quorum.

```yaml
lifecycleHooks:
  preDrain:
  - name: EtcdQuorumOperator
    owner: clusteroperator/etcd
```

The etcd operator will apply the hook to a Machine resource once it identifies that the Machine hosts an etcd member.
The hook should be added before the member is promoted to ensure that there is no period where the member is a voting
member, while the machine is not protected by the deletion mechanism.

It will only remove the hook once it has identified that the Machine resource is being deleted and a replacement member
has been created. The removal of this hook will allow the Machine API to drain and terminate the Machine as it would
normally do.

In the case that a Machine is deleted before the member is promoted, the etcd operator is expected to not promote the
new member, and remove the deletion hook to allow the Machine to be removed from the cluster.
Once a Machine has been marked for deletion, if the hook is removed by some other entity, the etcd operator
is expected not to re-add the hook. This allows an escape hatch when manual intervention is required.

The etcd operator will ensure, based on the desired Control Plane replica count in the cluster `InstallConfig`
resource, that the etcd cluster has either the exact desired count of voting members, or during scaling/replacement
operations, at most 1 extra voting member.

The etcd operator will also leverage the etcd quorum guard to prevent voluntary disruptions of etcd members during the
process. By ensuring that the quorum guard PDB always has `minAvailable: (Num Current Control Plane Machines) - 1`,
this prevents draining of a healthy etcd member until a new member becomes healthy. This operation will prevent other
components in the cluster (eg. MCO) from disrupting the quorum of etcd during this operation.

Note: When the cluster is already degraded, the etcd operator is expected to report as degraded for admin intervention.
The etcd operator will not attempt to recover the cluster using the methods described in this enhancement.

##### Adoption of an existing Control Plane into the new mechanism

Note: This flow is the expectation for when a cluster is upgraded from 4.N-1 to 4.N where 4.N is the version where this
mechanism is introduced.

1. etcd operator fetches all Control Plane Machines
1. etcd operator identifies if the Control Plane Machines are in the `Running` phase
    a. If no Control Plane Machines are `Running`, assume that Machine API is non-functional and stop here
1. etcd operator identifies the voting members of the etcd cluster and maps these to the Control Plane Machines that
   host them
1. etcd operator adds a deletion hook to each of the Machines hosting a voting member of the etcd cluster

##### Operational flow during a node replacement (vertical scaling) operation

1. A Control Plane Machine is marked for deletion and a new Control Plane Machine is created
    a. The order of these operations does not matter
1. The etcd operator notices the new Machine and adjusts the etcd quorum guard appropriately
1. The Machine API creates the new host and the new Node joins the cluster
1. A new etcd member starts on the newly created Node
    a. This member is initially started as a learner member
1. The new etcd member syncs the full etcd state and becomes promotable
1. The etcd operator adds a deletion hook to the new Machine
1. The etcd operator promotes the new etcd member to a voting member
1. The etcd operator demotes the old etcd member, removing it from the cluster
1. The etcd operator removes the deletion hook from the old Machine
1. The Machine API now drains and removes the old Machine
1. etcd operator notices the removed Machine and adjusts the etcd quorum guard appropriately

##### Operational flow if a Control Plane Machine is deleted by a user

1. User deletes the Machine object
    a. This may also be some other component, for example this could be caused by a MachineHealthCheck
1. Machine API observes the etcd quorum pre-drain hook and waits for this to be removed before proceeding with the  
   Machine removal
1. etcd operator determines that removal of the etcd member on the deleted Machine would violate the desired replica
   count, takes no action
1. At some point, some user (or operator) creates a new Control Plane Machine
1. At this point, the remaining flow is as above, go to step 2 of
   [Operational flow during a resize operation](#Operational-flow-during-a-resize-operation)

##### Interaction with upgrades

While no scaling operations are occuring within the Control Plane set of Machines (ie. the number of Control Plane
Machines matches the desired count and none are in the process of being removed), the mechanism described in this
proposal will not interfere with the upgrade process and upgrades will proceed as normal.

While a scaling operation is occurring, the etcd quorum guard will prevent the draining of any of the Control Plane
Machines, until the new etcd member has been promoted and the old etcd member removed from the cluster.
This in turn means that updates caused during upgrades (for example changes to MachineConfig) will be blocked while the
scaling operation occurs.
This will delay the upgrade process, but should not block it indefinitely unless an issue occurs.

#### Additional Details

##### What are Machine Deletion Hooks?

[Machine Deletion Hooks](https://github.com/openshift/enhancements/pull/862) are a mechanism within the Machine API
that allow other operators to pause the Machine lifecycle in various places.
For example, once a Machine is marked for deletion, an operator may use a hook to prevent the Machine API from draining
a Node.

For the use case described in this document, we will leverage a pre-drain hook to pause the Machine removal until the
etcd member present on the Machine has been removed from the etcd quorum.

Once the member is removed from quorum, the etcd operator will remove the pre-drain hook,
which will signal to the Machine API that it is now safe to drain and terminate the instance as normal.

##### When is Machine API Functional?

We often refer to clusters in OpenShift as UPI and IPI.
However, there is no clear distinction, apart from during the install process between these two types of cluster.
Importantly, there should be no way to tell, after the cluster was installed, whether the cluster was created using UPI
or IPI.

Since in a UPI cluster, Control Plane Machines are typically unmanaged, the mechanisms described in this document will
not work in a UPI cluster. As there is no way for the etcd operator to determine whether a cluster is UPI or IPI, it
must instead determine whether or not the Machine API is functional.

For the purposes of this document, we define a functional Machine API as one which configured correctly such that if
required, it could create a new Machine.

Typically in UPI clusters, Machines and MachineSets are not present. This would represent a non-functional Machine API.

Typically in IPI clusters, Machines and MachineSets are created and after bootstrap, there are 6  Machines in the
`Running` phase, 3 Control Plane, 3 Worker. This would represent a functional Machine API.

###### Functional Machine API Scenarios

- The cluster was created using the IPI installation method.
  - The Control Plane Machine objects are created by the installer and linked to the existing hosts. The customer can
    use Machines API to create a new Control Plane host if required
- The cluster was created using the UPI installation method. The customer missed the instruction to remove the Machines
  and MachineSets from the manifests directory. The Control Plane Machines somehow became Running.
  - We have no evidence to suggest this is actually possible
  - Typically the installer creates Machines with specific names/tags. These are used by Machine API to identify the
    host and link it to the Machine. In UPI scenarios these names aren't mentioned in the documentation and the
    customer has free choice over the naming of their hosts
  - The Machine spec in this case will be half complete as the installer cannot fulfill all of the infrastructure
    information before the cluster is created
  - In this case, we have no way to determine whether creating new Machines from this spec will work
  - Assuming that the safety mechanism in this proposal is working as expected, there should be no risk to the Control
    Plane, but there may be additional work for the cluster administrator should they need to perform maintenance on
    the Control Plane.
  - We expect in this scenario that the administrator was not intending to use Machine API and as such, wouldn't try to
    use Machine API during this maintenance window, and as such, wouldn't actually run into any issues
- The cluster was created using the UPI installation method. The customer then configures Machine resources for their
  Control Plane Hosts
  - This process is undocumented, but theoretically possible
  - We expect in this case that the customer would configure the providerSpec to be accurate and test that new Machines
    can be created that work with their clusters.
  - In this scenario, the Machine API represents the configuration of an IPI cluster. Machines will be ready and we
    should be able to manage the Control Plane Machines as with an IPI cluster

In each of these scenarios, the presence of Machines in the `Running` phase signals that the Machine API is functional.

###### Non-Functional Machine API Scenarios

- The cluster was created using the UPI installation method. The customer correctly removed the Machines and
  MachineSets before installation.
  - There are no Machines in the cluster so Machine API cannot be functional
- The cluster was created using the UPI installation method. The customer missed the instruction to remove the Machines
  and MachineSets from the manifests directory.
  - This was a common scenario when vSphere was introduced as a new Machine API provider. The instruction was missed
    from the UPI documentation
  - In this scenario, the Machines for the Control Plane all siti in the `Provisioning` phase
  - The Machine objects created do not have a full configuration and as such, the Machine API fails to provision new
    Control Plane Machines
- The cluster is created using either the UPI or IPI installation method, but to an unsupported platform (eg. platform
  None)
  - In this case, as Machine API does not support the platform, the installer will not have generated and Machines/
    MachineSets

In each of the scenarios, either that are no Machines in the cluster or the Machines will be stuck in the
`Provisioning` phase. In particular the absence of any `Running` Machine signals that the Machine API is non-functional.

##### New metrics to export via the telemeter

TODO: Consult with SREs to identify metrics they might find useful for us to export

#### Example of vertically scaling a Control Plane Machine

To use this mechanism to vertically scale a Control Plane, the following procedure must be carried out by some user or
operator.

1. Identify a Controle Plane Machine for replacement (eg. `my-cluster-master-0`)
1. Determine a new name for the new Control Plane Machine that will not conflict with existing Control Plane Machines
   (eg `my-cluster-master-3`)
1. Take a copy of the Machine resource: `oc get machine -n openshift-machine-api my-cluster-master-0 -o yaml > my-
   cluster-master-3.yaml`
1. Modify the Machine YAML to update the `metadata.name` field and the size of the instance (eg. changing
   `spec.providerSpec.value.instanceType` from `c5.xlarge` to `c5.2xlarge`). The following fields will also need to be removed: `spec.providerID`, the entire `status` section, `metadata.generation`, `metadata.resourceVersion` and `metadata.uid`.
1. Create the new Machine: `oc create -f my-cluster-master-3.yaml`
1. Delete the old Machine: `oc delete machine -n openshift-machine-api my-cluster-master-0`
1. Wait until the process described in [Operational flow during a resize operation](#Operational-flow-during-a-resize-operation)
   has been completed. The end result of this is that the Machine `my-cluster-master-0` will be removed

### Risks and Mitigations

#### Blocking removal of Machines in environments with restricted capacity

This proposal assumes that when a Machine is to be replaced, that there is additional capacity available to allow the
new Machine to be created before the old Machine is removed. This may not be true in all environments, for example if
there is no quota left or in bare-metal environments with limited hardware available.

To ensure that we do not block users from replacing Control Plane Machines in these scenarios, we must allow a user to
remove the etcd quorum hook without it being replaced by the etcd operator.
When a Machine is marked for deletion, if the hook is removed, the etcd operator will not replace it. This will allow
users to override the mechanism and continue with their replacement in these scenarios.

In the future, if a new `ControlPlane` CRD is introduced, the etcd operator could observe the upgrade strategy on this
CR within the cluster and take appropriate action based on this.
We could allow users via this CRD to signal that they do not have capcity to burst during scaling operations and in
this case the etcd operator can remove the hooks as appropriate.

#### Other controllers may interfere with the mechanism

We know that some OpenShift users leverage GitOps mechanisms to manage Machines within OpenShift.
If these GitOps systems do not correctly handle server side changes (like additional annotations),
then they may remove the hooks after the etcd operator has added them.
In this case we expect a hot loop where the etcd and GitOps operators fight to add and remove the hook.
We must test this scenario and ensure that external systems aren't going to interfere with the mechanism.

## Design Details

### Open Questions

- Do we want to explicitly block upgrades while scaling operations are happening? What could go wrong if we don't?
- What metrics would be useful to expose from etcd operator and machine api to help monitor the progress of these
  operations?
- What metrics are we likely to want to pull back from customer clusters into CCX?

### Test Plan

We will need to develop a new E2E suite that exercises the replacement process outlined in this proposal.

In particular, we will need write the following into a test case:
- Bring up new cluster and check that all is healthy
- Delete a control plane machine and check that it does not get removed
- Check that etcd quorum is still intact - etcd operator should degrade when the cluster is degraded
- Create a new control plan machine to replace the deleted machine
- Monitor etcd to ensure that it does not degrade during replacement procedure
- Wait until old control plane machine is removed from the cluster

### Graduation Criteria

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

This proposal introduces a new internal OpenShift safety mechanism.
No features will be deprecated or removed during the implementation of this proposal.

### Upgrade / Downgrade Strategy

Note: For the purpose of this section, assume version 4.N is the version in which this feature is introduced.

#### Upgrading version 4.N-1 to version 4.N

When upgrading from version 4.N-1 to version 4.N, the etcd operator will introduce the new hooks onto the Machines.
These hooks take the form of annotations and as such can be added straight away without any new API rollout.

Once in place, the new hooks will be observed by the Machine API Controllers and the new mechanism will be active.

The process is described in more detail above in the [Adoption of existing an Control Plane into the new mechanism](#Adoption-of-existing-an-Control-Plane-into-the-new-mechanism) section.

#### Downgrading version 4.N to version 4.N-1

On dowgrades, the hooks added by the etcd operator will persist on the Machine object.
However, as soon as the Machine API Controllers are downgraded, the hooks will no longer be enforced.

We will time the release of the [Machine Deletion Hooks](https://github.com/openshift/enhancements/pull/862) feature in
Machine API such that it is only active from version 4.N, preventing the need for specific downgrade logic as part of
this proposal.

### Version Skew Strategy

We do not expect users to resize their Control Planes during upgrade operations and as such should see no version skews.

The mechanism will only be effective once the Machine API Controllers and etcd operator are upgraded,
however, neither depends on the other to be operational and as such, no issues should occur during the introduction of
this feature.

### Operational Aspects of API Extensions

This enhancement does not introduce any new API extensions.
Therefore no operational details are required.

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

No implementation of this proposal currently exists.

## Drawbacks

- This design means that the etcd operator has to understand a new API type, which adds complexity to the etcd operator
  and ties it to the Machine API.
  This may mean additional complexity if this mechanism were to be needed in the Centrally Managed Infrastructure
  project or if OpenShift were to migrate to Cluster API.

## Alternatives

### Use a separate component for this mechanism

Rather than embedding the described mechanism within the etcd operator itself, we could create a new component within
OpenShift that focuses explicitly on the coordination of the etcd cluster lifecycle and the Machine lifecycle.
This new component would then be able to be lifecylced separately and could potentially be adapted to leverage Cluster
API as an alternative if needed in the future.
Since it is not clear if this is an immediate need, we believe that the extra effort of creating a new component is not
necessary during the first iteration of this proposal, but the mechanism could be extracted into a separate component
in the future.

### Leverage a PDB to prevent disruptions

We could consider using a PDB (eg etcd-quorum-guard) to prevent the removal of Machines by blocking the Machine from
being drained. However, by blocking the Machine controller from draining a Machine, we also block other components from
draining the Machine. This would in turn block normal day to day operations such as upgrades, where normally, MCO will
drain a Machine and reboot it to apply updates.

Today, the etcd quorum guard protects the etcd cluster quorum by preventing more than 1 etcd member from being
disrupted at any one time.
These small interuptions for updates are tolerable as the etcd member should have a relatively small diff in its data
when it starts back up, meaning the cluster is degraded for only a short period.

When replacing the Machine, it is preferable to ensure that the replacement member is promotable before removing the
old member. This minimises the duration in which the etcd cluster could become degraded.
To prevent that member being removed, we would need a PDB that does not allow the MCO to drain nodes,
and as such, a PDB isn't a suitable mechanism for this use case.

## Infrastructure Needed

No additional infrastructure will be needed as a result of this proposal.
