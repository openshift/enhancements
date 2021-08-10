---
title: machine-deletion-hooks
authors:
  - "@JoelSpeed"
  - "@michaelgugino"
reviewers:
  - "@enxebre"
  - "@elmiko"
  - "@ademicev"
  - "@hexfusion"
approvers:
  - "@elmiko"
  - "@hexfusion"
  - "@deads2k"
creation-date: 2021-08-10
last-updated: 2021-09-28
status: implementable
see-also:
  - [Cluster API Upstream Equivalent](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20200602-machine-deletion-phase-hooks.md)
---

# Machine Deletion Hooks

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Glossary

### Lifecycle Hook
A specific point in a machine's reconciliation lifecycle where execution of
normal machine-controller behaviour is paused or modified.

### Deletion Phase
Describes when a machine has been marked for deletion but is still present
in the API.  Various actions happen during this phase, such as draining a node,
deleting an instance from a cloud provider, and deleting the node object.

### Hook Implementing Controller (HIC)
The Hook Implementing Controller describes a controller, other than the
machine controller, that adds, removes, and/or responds to a particular
lifecycle hook. Each lifecycle hook should have a single HIC, but an HIC
can optionally manage one or more hooks.

## Summary

Defines a set of annotations that can be applied to a Machine which can be used
to delay the actions taken by the Machine controller once a Machine has been marked
for deletion. These annotations are optional and may be applied during Machine
creation, sometime after Machine creation by a user, or sometime after Machine
creation by another controller or application, up until the Machine has been marked
for deletion.

## Motivation

Allow custom and 3rd party components to easily interact with a Machine or
related resources while that Machine's reconciliation is temporarily paused.
This pause in reconciliation will allow these custom components to take action
after a Machine has been marked for deletion, but prior to the Machine being
drained and/or associated instance terminated.

In particular, this will allow the etcd Operator to prevent a Control Plane
Machine from being drained/removed until the etcd Operator has had a chance
to synchronise the etcd data to the replacement Machine.

This could also be used by other components such as Storage to allow them to
ensure safe detachment of volumes before the Machine is deleted.

### Goals

- Define an initial set of hook points for the deletion phase.
- Define an initial set and form of related annotations.
- Define basic expectations for a controller or process that responds to a
lifecycle hook.

### Non-Goals

- Create an exhaustive list of hooks; we can add more over time.
- Create new machine phases.
- Create a mechanism to signal what lifecycle point a machine is at currently.
- Dictate implementation of controllers that respond to the hooks.
- Implement ordering in the machine controller.
- Require anyone to use these hooks for normal machine operations, these are
strictly optional and for custom integrations only.

## Proposal

- Utilize annotations to implement lifecycle hooks.
- Each lifecycle point can have 0 or more hooks.
- Hooks do not enforce ordering.
- Hooks found during machine reconciliation effectively pause reconciliation
until all hooks for that lifecycle point are removed from a machine's annotations.

### User Stories

#### Story 1
(pre-terminate) As an operator, I would like to have the ability to perform
different actions between the time a Machine is marked deleted in the API and
the time the Machine is deleted from the infrastructure provider.

For example, when replacing a Control Plane Machine, ensure a new Control
Plane Machine has been successfully created and joined to the cluster before
removing the instance of the deleted Machine. This might be useful in case
there are disruptions during replacement and we need the disk of the existing
instance to perform some disaster recovery operation.  This will also prevent
prolonged periods of having one fewer Control Plane host in the event the
replacement instance does not come up in a timely manner.

#### Story 2
(pre-terminate) As an operator, I would like to have the ability to perform
some action between the time that a Machine has been drained, but before it
is removed from the infrastructure provider.

For example, when storage is attached to the Machine due to some pod placement,
an operator may want to check that the storage is detached before allowing the
removal of the machine from the infrastructure provider.

Alternatively, once Nodes are drained, an additional delay may need to be added
to allow the Node's log exporter daemon to synchronise all logs to the
centralised logging system. A pre-terminate hook could allow a log operator to
ensure the main workloads are removed and no longer adding to the log backlog
before waiting for the log exporter to catch up on the synchronisation process,
ensuring all application logs are captured.

#### Story 3
(pre-drain) As an operator, I want the ability to utilize my own draining
controller instead of the logic built into the machine controller. This will
allow me better flexibility and control over the lifecycle of workloads on each
node.

For example this would allow me to prioritise moving particular mission critical
applications first to ensure that service interruptions are minimised in cases
where cluster capacity is limited. Ordering is not supported today in drain
libraries and as such, this would require a custom drain provider.

### Implementation Details/Notes/Constraints

For each defined lifecycle point, one or more hooks may be applied as an annotation
to the machine object. These annotations will pause reconciliation of a Machine object
until all hooks are resolved for that lifecycle point.
The hooks should be managed by a Hook Implementing Controller or other external
application, or manually created and removed by an administrator.

#### Lifecycle Points

##### pre-drain

`pre-drain.delete.hook.machine.cluster.x-k8s.io`

Hooks defined at this point will prevent the machine controller from draining a node
after the machine object has been marked for deletion until the hooks are removed.

##### pre-terminate

`pre-terminate.delete.hook.machine.cluster.x-k8s.io`

Hooks defined at this point will prevent the machine-controller from
removing/terminating the instance in the cloud provider until the hooks are
removed.

"pre-terminate" has been chosen over "pre-delete" because "terminate" is more
easily associated with an instance being removed from the cloud or
infrastructure, whereas "delete" is ambiguous as to the actual state of the
Machine in its lifecycle.

#### Annotation Form
```yaml
<lifecycle-point>.delete.hook.machine.cluster-api.x-k8s.io/<hook-name>: <owner/creator>
```

##### lifecycle-point
This is the point in the lifecycle of reconciling a machine the annotation will
have effect and pause the machine-controller.

##### hook-name
Each hook should have a unique and descriptive name that describes in 1-3 words
what the intent/reason for the hook is.
Each hook name should be unique and managed by a single entity.

##### owner (Optional)
Some information about who created or is otherwise in charge of managing the annotation.
This might be a controller or a username to indicate an administrator applied the hook directly.

##### Annotation Examples

These examples are all hypothetical to illustrate what form annotations should
take. The names of each hook and the respective controllers are fictional.

pre-drain.hook.machine.cluster-api.x-k8s.io/migrate-important-app: my-app-migration-controller

pre-terminate.hook.machine.cluster-api.x-k8s.io/backup-files: my-backup-controller

pre-terminate.hook.machine.cluster-api.x-k8s.io/wait-for-storage-detach: my-custom-storage-detach-controller

#### Changes to machine-controller

The machine controller should check for the existence of 1 or more hooks at
specific points (lifecycle-points) during reconciliation.  If a hook matching
the lifecycle-point is discovered, the machine-controller should stop
reconciling the machine.

##### Reconciliation

When a Hook Implementing Controller updates the Machine, reconciliation will be
triggered, and the Machine will continue reconciling as normal, unless another
hook is still present; there is no need to 'fail' the reconciliation to
enforce requeuing.

When all hooks for a given lifecycle-point are removed, reconciliation
will continue as normal.

##### Preventing additions after Machine deletion

Adding new hooks once the deletion process has been initiated may not behave as
expected, for example if a pre-drain hook is added after the Machine has already
been terminated.

To prevent potentially confusing behaviour, once a Machine has been marked for deletion,
it is expected that hook implementing controllers should not attempt to add new deletion
hooks. The hook implementing controller is expected to detect the deletion in progress
and perform any clean up logic necessary.

To prevent potential issues here, and to ensure users do not accidentally add hooks,
we will leverage the existing Machine API webhooks to prevent new annotations of the
deletion hook format from being added to Machines after they have been marked for
deletion.

##### Hook failure

The machine controller should not timeout or otherwise consider the lifecycle
hook as 'failed.'  Only the Hook Implementing Controller (or the end user in
extenuating circumstances) may decide to remove a particular lifecycle hook
to allow the machine controller to progress past the corresponding lifecycle-point.

The Machine status will contain conditions identifying why the Machine controller
has not progressed in removing the Machine. It is expected that hook implementing
controllers should signal to users when they are having issues via some mechanism
(eg. their `ClusterOperator` status) and that the conditions on the Machine should
identify the components which the user should look at in order to determine any
underlying issues.

For example, the condition will contain the owner of the hook blocking the removal,
the user should look towards the owner's own reporting to identify issues.
Any OpenShift operator experiencing issues should already have a mechanism in which
it can report errors and having these errors follow the normal mechanisms should
place the failure reporting closer to the issue, as opposed to placing it directly
onto the Machine.

We also encourage hook implementing controllers to use Event objects to add detail
about the operations they are performing. For example if the hook implementing
controller is waiting on some side effect, it should periodically send an Event,
attached to the Machine object, notifying the user of delay. Events are already
displayed in the OpenShift console and in `oc describe` and as such should be
relatively easy for the end user to find.

##### Hook ordering

The machine controller will not attempt to enforce any ordering of hooks.
No ordering should be expected by the machine controller.

Hook Implementing Controllers may choose to provide a mechanism to allow
ordering amongst themselves via whatever means HICs determine.
Examples could be using CRDs external to the Machine API, gRPC communications,
or additional annotations on the Machine or other objects.

#### Hook Implementing Controller Design

Hook Implementing Controller is the component that manages a particular
lifecycle hook.

##### Hook Implementing Controllers must

* Watch Machine objects and determine when an appropriate action must be taken.
* After completing the desired hook action, remove the hook annotation.

##### Hook Implementing Controllers may

* Watch Machine objects and add a hook annotation as desired by the cluster
administrator.
* Coordinate with other Hook Implementing Controllers through any means
possible, such as using common annotations, CRDs, etc. For example, one hook
controller could set an annotation indicating it has finished its work, and
another hook controller could wait for the presence of the annotation before
proceeding.

#### Determining when to take action

A Hook Implementing Controller should watch Machines and determine when is the
best time to take action.

For example, if an HIC manages a lifecycle hook at the pre-drain lifecycle-point,
then that controller should take action immediately after a Machine has a
DeletionTimestamp or enters the "Deleting" phase.

Fine-tuned coordination is not possible at this time; eg, it's not
possible to execute a pre-terminate hook only after a node has been drained.
This is reserved for future work.

##### Failure Mode

It is entirely up to the Hook Implementing Controller to determine when it is
prudent to remove a particular lifecycle hook. Some controllers may want to
'give up' after a certain time period, and others may want to block indefinitely.
Cluster operators should consider the characteristics of each controller before
utilizing them in their clusters.

#### Conditions

To ensure that users have visibility into why a Machine has not been removed yet,
we will add new conditions to the status of the Machine.

##### Drainable

The drainable condition will reflect whether, when deleted, the Machine would be
able to be drained. If any pre-drain hooks are present on the Machine, the
condition will be marked false.

##### Terminable

The terminable condition will reflect whether, when deleted, the Machine would
be able to be terminated. If any pre-terminate hooks are present on the Machine,
the condition will be marked false.

### Risks and Mitigations

* Annotation keys must conform to length limits: https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/#syntax-and-character-set
* Requires well-behaved controllers and admins to keep things running
smoothly. Would be easy to disrupt machines with poor configuration.
* Troubleshooting problems may increase in complexity, but this is
mitigated mostly by the fact that these hooks are opt-in. Operators
will or should know they are consuming these hooks, but a future proliferation
of the cluster-api could result in these components being bundled as a
complete solution that operators just consume. To this end, we should
update any troubleshooting guides to check these hook points where possible.
* Users may add hooks which end up indefinitely blocking removal of Machines.
We already have a [MachineNotYetDeleted](https://github.com/openshift/machine-api-operator/blob/master/docs/user/Alerts.md#machinenotyetdeleted)
alert that will warn users when a Machine removal is blocked for an extended
period. This should continue to function and notify users when a removal has
been blocked, at which point we expect manual intervention to check the Machine.

## Design Details

### Test Plan

* E2E testing will be added to the cluster-api-actuator-pkg (Machine API E2E suite)
to ensure that the hooks behave as expected

### Graduation Criteria

This feature is not large enough to require graduation and will be considered GA from
initial release.

#### Dev Preview -> Tech Preview

This feature will be released straight to GA.

#### Tech Preview -> GA

This feature will be released straight to GA.

#### Removing a deprecated feature

This proposal does not involve removing any deprecated features.

### Upgrade / Downgrade Strategy

As this feature is opt-in, there is no need to account for upgrades.

Any new Hook Implementing Controller will be able to add the hooks at
any point and they will be observed when the updated Machine controller
is deployed via the CVO.

On downgrades, HICs may wish to remove the annotations, but they are not
harmful if left behind.

### Version Skew Strategy

We do not expect any issues with version skew as this is a new feature.

## Implementation History

* This has already been implemented in Cluster API

## Drawbacks

* If misused, this could prevent Machines from being deleted and lead to an increase in support
cases to determine if it is safe to override the feature.

## Alternatives

### Custom Machine Controller
Require advanced users to fork and customize.
This can already be done if someone chooses, so not much of a solution.

### Finalizers
We define additional finalizers, but this really only implies the deletion lifecycle point.

A misbehaving controller that accidentally removes finalizers could have undesirable effects,
such as the removal of the Machine before it has been terminated.
A misbehaving controller may also remove too many deletion hooks, but in this case, this
wouldn't result in the Machine resource being removed from etcd and therefore prevents costly
errors such as leaked cloud instances.

The proposal today effectively extends the finalizer concept to allow two stages of finalization
(pre and post drain) in conjunction with the existing Machine finalizer.

A limitation of finalizers is the 63 character limit. With this limit, it would be difficult to
provide information such as the owner of the deletion hook. By leveraging annotations which have
a larger length limit, we can store more information about the hook within the API.

If we were to leverage finalizers for this purpose, there are a few different ways this could work,
as set out below.

#### Machine API waits on all Finalizers

In this scenario, the Machine API would not start draining/removing a Machine until all finalizers
other than its own one have been removed. This would allow future controllers to manage the removal
of the Machine without having Machine API have to know anything about their specific implementation.
However, there could be a scenario where another controller also needs to wait for other finalizers
to be removed, in such a case, unless the Machine API or this new controller are aware of each other
there would be a deadlock created.

Relying on all finalizers being removed before draining/removing could become fragile in the future
due to the above, and as such is not a preferred solution.

#### Machine API waits on some Finalizers

In this scenario, Machine API would have a list of known finalizers that would prevent it from taking
certain actions. For example a known etcd-quorum finalizer could prevent the Machine controller from
draining the Machine.

This creates a tight coupling between the Machine controller and other components within OpenShift.
The upstream solution today, matches the proposal set out in this document. As we are experimenting
with Cluster API in HyperShift, Central Machine Management and for future IPI clusters, it is
preferred to not create a tight coupling with these components and to copy the upstream solution to
allow these mechanisms to be transparently honoured by both MAPI and CAPI going forward.

#### Machine API waits on Finalizers based on an annotation

In this scenario, the blocking operator would use an annotation to instruct the Machine controller
of a particular finalizer it should wait on. This solution allows the third party operator to opt
in to having their cleanup logic block the removal of the Machine, without having to have the
Machine controller aware of the finalizer ahead of time.

As this approach includes the use of annotations to prevent the Machine controller from taking any
action, it is very similar to the approach described in the proposal above.
Since any deletion hook annotation prevents the Machine controller from removing its own finalizer,
this secondary annotation + finalizer combination doesn't add much value over the existing proposal.
As it also means we deviate from the upstream solution, and may have to account for this in the
future with additional complexity, this is not a preferred solution.

### Status Field
Harder for users to modify or set hooks during machine creation.
How would a user remove a hook if a controller that is supposed to remove it is misbehaving?
We’d probably need an annotation like ‘skip-hook-xyz’ or similar and that seems redundant
to just using annotations in the first place.

### Spec Field
We probably don’t want other controllers dynamically adding and removing spec fields on an object.
It’s not very declarative to utilize spec fields in that way.

### CRDs
Seems like we’d need to sync information to and from a CR.
There are different approaches to CRDs (1-to-1 mapping Machine to CR, match labels,
  present/absent vs status fields) that each have their own drawbacks and are more
complex to define and configure.
