---
title: control-plane-machine-set-controller
authors:
  - "@JoelSpeed"
reviewers:
  - TBD
approvers:
  - "@sttts"
  - "@soltysh"
  - "@tkashem"
  - "@hasbro17"
api-approvers:
  - TBD
creation-date: 2022-01-11
last-updated: 2022-01-17
tracking-link:
  - TBD
replaces:
  - "[/enhancements/machine-api/control-plane-machinesets.md](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/control-plane-machinesets.md)"
  - "https://github.com/openshift/enhancements/pull/278"
  - "https://github.com/openshift/enhancements/pull/292"
---

# Control Plane Machine Set Controller

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

To enable automated management of vertical scaling and replacement of Control Plane Machines, this proposal introduces
a new resource and controller that will manage Control Plane Machine instances.

## Motivation

As OpenShift adoption increases and our managed services offerings become more popular, the manual effort required to
scale Control Plane Machines as clusters grow or shrink is becoming a significant drain on the SRE Platform team
managing OpenShift Dedicated (OSD), Red Hat OpenShift on AWS (ROSA), and Azure Red Hat OpenShift (ARO).

The procedure to resize a Control Plane today is lengthy and very involved. It takes a significant amount of time for
an OpenShift expert to perform. We also document this process for our end users, however due to the complexity of the
procedure, there is discomfort in recommending this procedure to end users who may not be as familiar with the product.

To ensure the long term usability of OpenShift, we must provide an automated way for users to scale their Control Plane
as and when their cluster needs additional capacity.

As HyperShift adoptions grows, HyperShift will solve the same issue by running hosted Control Planes within management
clusters. However, HyperShift is not a suitable product for all OpenShift requirements (for example the first management
cluster) and as such, we see that HA clusters will continue to be used and we must solve this problem to allow the
continued adoption of HA clusters.

### Goals

* Provide a "best practices" approach to declarative management of Control Plane Machines
* Allow users to "1-click" scale their Control Plane to large or smaller instances
* Allow the adoption of existing Control Planes into this new mechanism
* Provide a safe mechanism to perform the sensitive operation of scaling the Control Plane and provide adequate
  feedback to end users about progress of the operation
* Allow users to opt-out of Control Plane management should our mechanism not fit their needs
* Allow users to customise rolling update strategies based on the needs of their environments

### Non-Goals

* Allow horizontal scaling of the Control Plane (this may be required in the future, but today is considered
  unsupported by OpenShift)
* Management of the etcd cluster state (the etcd operator will handle this separately)
* Automatic adoption of existing clusters (we will provide instructions to allow users to opt-in for existing cluster)
* Management of anything that falls out of the scope of Machine API

## Proposal

A new CRD `ControlPlaneMachineSet` will be introduced into OpenShift and a respective `control-plane-machine-set-
operator` will be introduced as a new second level operator within OpenShift to perform the operations described in
this proposal.

The new CRD will define the specification for managing (creating, adopting, updating) Machines for the Control Plane.
The operator will be responsible for ensuring the desired number of Control Plane Machines are present within the
cluster as well as providing update mechanisms akin to those seen in a StatefulSets and Deployments to allow rollouts
of updated configuration to the Control Plane Machines.

### User Stories

- As a cluster administrator of OpenShift, I would like to be able to safely and automatically vertically resize my
  control plane as and when the demand on the control plane changes
- As a cluster administrator of OpenShift, I would like to be able to automatically recover failed Control Plane
  Machines
- As a cluster administrator of OpenShift, I would like to be able to make changes to the configuration of the
  underlying hardware of my control plane and have these changes safely applied using immutable hardware concepts
- As a cluster administrator of OpenShift, I would like to be able to control rollout of changes to my Control Plane
  Machines such that I can test changes with a single replica before applying the change to all replicas
- (Future work) As a cluster administrator of OpenShift with restricted hardware capacity, I would like to be able to   
  scale down my control plane before adding new Control Plane Machines with newer configuration, notably, my
  environment does not have capacity to add additional Machines during updates

### API Extensions

We will introduce a new `ControlPlaneMachineSet` CRD to the `machine.openshift.io/v1beta1` API group. It will be based
on the spec and status structures defined below.

```go
type ControlPlaneMachineSetSpec struct {
    // Replicas defines how many Control Plane Machines should be
    // created by this ControlPlaneMachineSet.
    // This field is immutable and cannot be changed after cluster
    // installation.
    // The ControlPlaneMachineSet only operates with 3 or 5 node control planes,
    // 3 and 5 are the only valid values for this field.
    // +kubebuilder:validation:Enum:=3;5
    // +kubebuilder:default:=3
    // +kubebuilder:validation:Required
    Replicas *int32 `json:"replicas"`

    // Strategy defines how the ControlPlaneMachineSet will update
    // Machines when it detects a change to the ProviderSpec.
    // +kubebuilder:default:={type: RollingUpdate}
    // +optional
    Strategy ControlPlaneMachineSetStrategy `json:"strategy,omitempty"`

    // FailureDomains is the list of failure domains (sometimes called
    // availability zones) in which the ControlPlaneMachineSet should balance
    // the Control Plane Machines.
    // This will be injected into the ProviderSpec in the
    // appropriate location for the particular provider.
    // This field is optional on platforms that do not require placement
    // information, eg OpenStack.
    // +kubebuilder:validation:MinLength:=1
    // +kubebuilder:validation:Required
    // +optional
    FailureDomains []string `json:"failureDomains,omitempty"`

    // Label selector for Machines. Existing Machines selected by this
    // selector will be the ones affected by this ControlPlaneMachineSet.
    // It must match the template's labels.
    // This field is considered immutable after creation of the resource.
    // +kubebuilder:validation:Required
    Selector metav1.LabelSelector `json:"selector"`

    // Template describes the Control Plane Machines that will be created
    // by this ControlPlaneMachineSet.
    // +kubebuilder:validation:Required
    Template ControlPlaneMachineSetTemplate `json:"template"`
}

type ControlPlaneMachineSetTemplate struct {
    // ObjectMeta is the standard object metadata
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
    // Labels are required to match the ControlPlaneMachineSet selector.
    // +kubebuilder:validation:Required
    ObjectMeta metav1.ObjectMeta `json:"metadata"`

    // Spec contains the desired configuration of the Control Plane Machines.
    // The ProviderSpec within contains platform specific details
    // for creating the Control Plane Machines.
    // The ProviderSe should be complete apart from the platform specific
    // failure domain field. This will be overriden when the Machines
    // are created based on the FailureDomains field.
    // +kubebuilder:validation:Required
    Spec machinev1beta1.MachineSpec `json:"spec"`
}

type ControlPlaneMachineSetStrategy struct {
    // Type defines the type of update strategy that should be
    // used when updating Machines owned by the ControlPlaneMachineSet.
    // Valid values are "RollingUpdate", "Recreate" and "OnDelete".
    // The current default value is "RollingUpdate".
    // +kubebuilder:default:="RollingUpdate"
    // +kubebuilder:validation:Enum:="RollingUpdate";"Recreate";"OnDelete"
    // +optional
    Type ControlPlaneMachineSetStrategyType `json:"type,omitempty"`

    // This is left as a struct to allow future rolling update
    // strategy configuration to be added later.
}

// ControlPlaneMachineSetStrategyType is an enumeration of different update strategies
// for the Control Plane Machines.
type ControlPlaneMachineSetStrategyType string

const (
    // RollingUpdate is the default update strategy type for a
    // ControlPlaneMachineSet. This will cause the ControlPlaneMachineSet to
    // first create a new Machine and wait for this to be Ready
    // before removing the Machine chosen for replacement.
    RollingUpdate ControlPlaneMachineSetStrategyType = "RollingUpdate"

    // Recreate causes the ControlPlaneMachineSet controller to first
    // remove a ControlPlaneMachine before creating its
    // replacement. This allows for scenarios with limited capacity
    // such as baremetal environments where additional capacity to
    // perform rolling updates is not available.
    Recreate ControlPlaneMachineSetStrategyType = "Recreate"

    // OnDelete causes the ControlPlaneMachineSet to only replace a
    // Machine once it has been marked for deletion. This strategy
    // makes the rollout of updated specifications into a manual
    // process. This allows users to test new configuration on
    // a single Machine without forcing the rollout of all of their
    // Control Plane Machines.
    OnDelete ControlPlaneMachineSetStrategyType = "OnDelete"
)

// ControlPlaneMachineSetStatus represents the status of the ControlPlaneMachineSet CRD.
type ControlPlaneMachineSetStatus struct {
    // ObservedGeneration is the most recent generation observed for this
    // ControlPlaneMachineSet. It corresponds to the ControlPlaneMachineSets's generation,
    // which is updated on mutation by the API Server.
    // +optional
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`

    // Replicas is the number of Control Plane Machines created by the
    // ControlPlaneMachineSet controller.
    // Note that during update operations this value may differ from the
    // desired replica count.
    Replicas int32 `json:"replicas"`

    // ReadyReplicas is the number of Control Plane Machines created by the
    // ControlPlaneMachineSet controller which are ready.
    ReadyReplicas int32 `json:"readyReplicas,omitempty"`

    // UpdatedReplicas is the number of non-terminated Control Plane Machines
    // created by the ControlPlaneMachineSet controller that have the desired
    // provider spec.
    UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

    // Conditions represents the observations of the ControlPlaneMachineSet's current state.
    // Known .status.conditions.type are: (TODO)
    // TODO: Identify different condition types/reasons that will be needed.
    // +patchMergeKey=type
    // +patchStrategy=merge
    // +listType=map
    // +listMapKey=type
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

### Implementation Details/Notes/Constraints

The ControlPlaneMachineSet controller aims to act similarly to the Kubernetes StatefulSet controller.
It will take the desired configuration, the `ProviderSpec`, and ensure that an appropriate number of Machines exist
within the cluster which match this specification.

It will be introduced as a new second level operator so that, if there are issues with its operation, it may report
this via a `ClusterOperator` and prevent upgrades or further disruption until the issues have been rectified.
Due to the nature of the actions performed by this controller, we believe it is important that it have its own
`ClusterOperator` in which to report its current status.

The `ControlPlaneMachineSet` CRD will be limited to a singleton within a standard OpenShift HA cluster, the only
allowed name will be `cluster`. This matches other high level CRD concepts such as the `Infrastructure` object.
Due to this limitation, the CRD will be cluster scoped.

The behaviour of such a controller is complex, and as such, various features of the controller and scenarios are
outlined in the details below.

#### Desired number of Machines

At present, the only source of the installed control plane size within OpenShift clusters exists within the
`cluster-config-v1` ConfigMap in the `kube-system` namespace.
This ConfigMap has been deprecated for some time and as such, should not be used for scale in new projects.

Due to this limitation, we will need to have a desired replicas field within the ControlPlaneMachineSet controller.
As we are not planning to tackle horizontal scaling of the control plane initially, we will implement a validating
webhook to deny changes to this value once set.

For new clusters, the installer will create (timeline TBD) the `ControlPlaneMachineSet` resource and will set the value
based on the install configuration.
For existing clusters, we will need to validate during creation of the `ControlPlaneMachineSet` resource that the
number of existing Control Plane Machines matches the replica count set within the `ControlPlaneMachineSet`.
This will prevent end users from attempting to horizontally scale their control plane during creation of the
`ControlPlaneMachineSet` resource.

In the future, once we have identified risks and issues with horizontal scale, and mitigated those, we will remove the
immutability restriction on the replica field to allow horizontal scaling of the control plane between 3 and 5 replicas
as per current support policy for control plane sizing.

#### Selecting Machines

The ControlPlaneMachineSet operator will use the selector defined within the CRD
to find Machines which it should consider to be within the set of control plane Machines it is responsible for managing.

This set should be the complete control plane and there should be a 1:1 mapping of Machines in this set to the control
plane nodes within the cluster.

If there are any control plane nodes (identified by the node role) which do not have a Machine, the operator will mark
itself degraded as this is an unexpected state.
No further action will be taken until the unknown node has either been removed from the cluster or a Machine has been
manually created for it.

#### Providing high availability/fault tolerance within the control plane

Typically within OpenShift, Machines are created within a MachineSet.
MachineSets have a single `ProviderSpec` which defines the configuration for the Machines created by the MachineSet.
The failure domain (sometimes called availability zone) is a part of this provider spec and as such, defines that all
Machines within the Machineset share a failure domain.

This is undesirable for Control Plane Machines as we wish to have them spread across multiple availability zones to
reduce the likelihood of datacenter level faults degrading the control plane.
To this end, the failure domains for the control plane will be set on the `ControlPlaneMachineSet` spec directly.

When creating Machines, the `ControlPlaneMachineSet` controller will balance the Machines across these failure domains
by injecting the desired failure domain for the new Machine into the provider spec based on the platform specific
failure domain field.

##### Failures domains

The `FailureDomains` field is expected to be populated by the user/installer based on the topology of the Control Plane
Machines. For example, we expect on AWS that this will contain a list of availability zones within a single region.
Note, we are explicitly not expecting users to add different regions to the the `FailureDomains` as we do not support
running OpenShift across multiple regions.

The following table denotes the field on each platform into which the list of failure domains will be mapped:


| Platform  | ProviderSpec Field          |
| --------- | --------------------------- |
| AWS       | .placement.availabilityZone |
| Azure     | .zone                       |
| GCP       | .zone                       |
| vSphere   | TBD (see below)             |
| OpenStack | .availabilityZone           |

Note that on some platforms (eg OpenStack) the failure domain field is optional, as such, the `FailureDomains` field
must also be optional.

###### Failure Domains on vSphere

As of OpenShift 4.10, there is no concept of a zone within vSphere and as such we would expect that the `FailureDomains`
would be omitted for vSphere.

However, [there is future work](https://github.com/openshift/enhancements/pull/918) to include zone support for vSphere
within 4.11. Once this is implemented, the Machine API provider for vSphere will understand the concept of zones,
defined within the `Infrastructure` resource.
The `ControlPlaneMachineSet` will then be able to balance Machines across the different zones by setting the new zone
information within the provider specs as it does on other platforms.

#### Ensuring Machines match the desired state

To ensure Machines are up to date with the desired configuration, the `ControlPlaneMachineSet` controller will leverage
a similar pattern to the workload controllers.
It will hash the template and compare the hashed result with the spec of the Machines within the managed set.
As we expect the failure domain to vary across the managed Machines, this will need to be omitted before the hash can
be calculated.

Should any Machine not match the desired hash, it will be updated based on the chosen update strategy.

##### The RollingUpdate update strategy

The RollingUpdate strategy will be the default. It mirrors the RollingUpdate strategy familiar to users from
Deployments. When a change is required to be rolled out, the Machine controller will create a new Control Plane
Machine, wait for this to become ready, and then remove the old Machine. It will repeat this process until all Machines
are up to date.
During this process the etcd membership will be protected by the mechanism described in a [separate enhancement](https://github.com/openshift/enhancements/pull/943),
in particular it isn't essential that the Control Plane Machines are updated in a rolling fashion for etcd sakes, though
to avoid potential issues with the spread of etcd members across availability zones during update, the
`ControlPlaneMachineSet` will perform a rolling update zone by zone.

At first, we will not allow any configuration of the RollingUpdate (as you might see in a Deployment) and will pin the
surge to 1 replica. We may wish to change this later once we have more data about the stability of this operator and
the etcd protection mechanism it relies on.

We expect this strategy to be used in most applications of the `ControlPlaneMachineSet`, in particular it can only not
be used in environments where capacity is very limited and a surge of any control plane capacity is unavailable.

##### The Recreate update strategy (FUTURE WORK)

Note: This strategy is not planned for implementation in the initial phase of this project. There are a number of
open questions that need to be ironed out and the use case is not immediately required with OpenShift.
The details of the strategy are left here as a basis of the future work.

The Recreate strategy mirrors the Recreate strategy familiar to users from Deployments.
When a change is required to be rolled out, the Machine controller will first remove a Control Plane Machine, wait for
its removal, and then create a new Control Plane Machine.

This strategy is intended to be used only in very resource constrained environments where there is no capacity to
introduce an extra temporary Control Plane Machine (preventing the RollingUpdate strategy).

At present, when using this strategy, the updates will need manual intervention. The etcd protection design will
prevent the Control Plane Machine from being removed until a replacement for the etcd member has been introduced into
the cluster. To allow for this use case, the end user can manually remove the etcd protection (via removing the Machine
deletion hook on the deleted Machine) allowing the rollout to proceed, the etcd operator will not re-add the
protection if the Machine is already marked for deletion.

This strategy introduces risk into the update of the Machines. While the old Machine is being drained/removed and the
new Machine is being provisioned, etcd quorum is at risk as a member has been removed from the cluster. In most
circumstances this would leave the cluster with just 2 etcd members.
This poses a similar risk to the etcd cluster as is present during a cluster upgrade when the Machine Config Daemon
reboots the Control Plane Machines, however, in this case, the duration of the member being down is expected to be
much longer than in the update process. For example baremetal clusters can take over an hour to reprovision a host.

There are a number of open questions related to this update strategy:
- Are we comfortable offering this strategy given the risks that it poses?
- What alternative can we provide to satisfy resource constrained environments if we do not implement this strategy?
- Should we teach the etcd operator to remove the protection mechanism when the `ControlPlaneMachineSet` is configured
  with this strategy?
- To minimise risk, should we allow this strategy only on certain platforms? (Eg disallow the strategy on public clouds)
- Do we want to name the update strategy in a way that highlights the risks associated, eg `RecreateUnsupported`?

##### The OnDelete update strategy

The OnDelete strategy mirrors the OnDelete strategy familiar to users from StatefulSets.
When a change is required, any logic for rolling out the Machine will be paused until the `ControlPlaneMachineSet`
controller observes a deleted Machine.
When a Machine is marked for deletion, it will create a new Machine based on the current spec.
It will then proceed with the replacement as normal.

This strategy will allow for explicit control for end users to decide when their Control Plane Machines are updated.
In particular it will also allow them to have different specs across their control plane.
While in the normal case this is discouraged, this could be used for short periods to test new configuration before
rolling the updated configuration to all Control Plane Machines.

Note that the OnDelete strategy is similar to the RollingUpdate strategy in that it needs to be able to add new
Machines to the cluster to function. It can be thought of as a slower, more controlled RollingUpdate strategy where the
user decides when each Control Plane Machine is replaced, and marks it for replacement by deleting it.
Otherwise the replacement is identical to the RollingUpdate strategy.

#### Protecting etcd quorum during update operations

With the introduction of the [etcd protection enhancement](https://github.com/openshift/enhancements/pull/943), the
ControlPlaneMachineSet does not need to observe anything etcd related during scaling operations.
In particular, it is the etcd operators responsibility to add Machine deletion hooks to prevent Control Plane Machines
from being removed until they are no longer needed.
When the `ControlPlaneMachineSet` controller observes that a newly created Machine is ready, it will delete the old
Control Plane Machine signalling to the etcd operator to switch the membership of the etcd cluster between the old and
new instances.
Once the membership has been switched, the Machine deletion hook will be removed on the old Machine, allowing it to be
removed by the Machine controller in the normal way.

#### Removing/disabling the ControlPlaneMachineSet

As some users may want to remove or disable the ControlPlaneMachineSet, a finalizer will be placed on the
`ControlPlaneMachineSet` to allow the controller to ensure a safe removal of the ControlPlaneMachineSet, while leaving
the Control Plane Machines in place.

Notably it will need to ensure that there are no owner references on the Machines pointing to the ControlPlaneMachineSet
instances. This will prevent the garbage collector from removing the Control Plane Machines when the
`ControlPlaneMachineSet` is deleted.

If users later wish to reinstall the `ControlPlaneMachineSet`, they are free to do so.

#### Installing a ControlPlaneMachineSet within an existing cluster

When adding a ControlPlaneMachineSet to a new cluster, the end user will need to define the `ControlPlaneMachineSet`
resource by copying the existing Control Plane Machine ProviderSpecs.
Once this is copied, they should remove the failure domain and add the desired failure domains to the `FailureDomains`
field within the ControlPlaneMachineSet spec.

To ensure adding a `ControlPlaneMachineSet` to the cluster is safe, we will need to ensure via a validating webhook
that the replica count defined in the spec is consistent with the actual size of the control plane within the cluster.

If no Control Plane Machines exist, or they are in a non-Running state, the operator will report degraded until this
issue is resolved. This creates a dependency for the `ControlPlaneMachineSet` operator on the Machine API. It will be
required to run at a higher run-level than Machine API.

In UPI or misconfigured clusters, a user adding a `ControlPlaneMachineSet` will result in a degraded cluster.
Users will need to remove the invalid ControlPlaneMachineSet resource to restore their cluster to a healthy state.

We do not recommend that UPI users attempt to adopt their control plane instances into Machine API due to the high
likelihood that they cannot create an accurate configuration to replicate the original control plane instances. This
limitation also limits the `ControlPlaneMachineSet` operator to an IPI only operator.

### Risks and Mitigations

#### etcd quorum must be preserved throughout scaling operations

As we are planning to scale up/down Control Plane Machines in an automated fashion, scaling operations will inevitably
effect the stability of the etcd cluster.
To prevent disruption, we have an [existing mechanism](https://github.com/openshift/enhancements/pull/943) that was
designed to allow the etcd operator to protect etcd quorum without other systems, such as Machine API, having any
knowledge of the state of the etcd cluster.

The protection mechanism is designed so that, even if a Machine is deleted, nothing will happen to the Machine (ie no
drain, no removal on the cloud provider) until the etcd operator allows the removal to proceed.
This prevents any data loss or quorum loss as the Machine will remain within the cluster until the etcd operator is
confident that it no longer needs the Machine.

The only time etcd quorum may be at risk is during the Recreate update strategy, this is highlighted in more detail
above.

#### Users may delete Control Plane Machines manually

If a user were to delete the Control Plane Machines using `oc`, `kubectl` or some other API call, the  
`ControlPlaneMachineSet` operator is designed in such a way that this should not pose a risk to the cluster.

The etcd protection mechanism will prevent removal of the Machines until they are no longer required for the etcd
quorum. The `ControlPlaneMachineSet` operator will, one by one, add new Control Plane Machines based on the existing
spec and wait for these to join the cluster. Once the new Machines have joined, they will replace the deleted Machines
as normal with the process outlined earlier in this document.

#### Machine Config may change during a rollout

There may be occasions where the Machine Config operator attempts to rollout new Machine Config during a Control Plane
scaling operation. We do not believe this will cause issue, but it may extend the time taken for the scaling operation
to take place.

While a scaling operation is in progress, etcd quorum is protected by the protection mechanism mentioned above as well
as the etcd quorum guard. The quorum guard will ensure that at most one etcd instance is interrupted at any time.
If the Machine Config operator needs to rollout an update, it will proceed in the usual manner while the etcd learner
process (part of scaling up a new etcd member) may suffer a delay due to the restart of the Control Plane Machines.

#### A user deletes the ControlPlaneMachineSet resource

Users will be allowed to remove `ControlPlaneMachineSet` resources should they so desire. This shouldn't pose a risk to
the control plane as the `ControlPlaneMachineSet` will orphan Machines it manages before it is removed from the
cluster.
More detail is available in the [Removing/disabling the ControlPlaneMachineSet](#Removingdisabling-the-ControlPlane)
notes.

#### The ControlPlaneMachineSet spec may not match the existing Machines

It is likely that in a number of scenarios, the spec attached to the `ControlPlaneMachineSet` for creating new Machines
may not match that of the existing Machines. In this case, we expect the `ControlPlaneMachineSet` operator will attempt
a rolling update
when it is created. Apart from refreshing the Machines potentially needlessly, this shouldn't have a negative effect on
the cluster assuming the new configuration is valid. If it is invalid, the cluster will degrade as the new Machines
created by the `ControlPlaneMachineSet` will fail to launch.

We have designed this operator with IPI style clusters in mind. We expect that the Machine objects within the cluster
should match the actual hosts on the infrastructure provider. In IPI clusters this is ensured by the installer.
Users may have, out of band, then modified these instances, but unfortunately we have no way to deal with this at
present. In these scenarios, when the Machines are replaced, they may not exactly match the previous iteration and
therefore may cause issues for the customer.
If a customer wishes to mitigate the chances of issues occurring, they may wish to manually control the roll out using
the OnDelete update strategy.

## Design Details

### Open Questions

Some open questions currently exist, but are called out in the Recreate Update Strategy notes.

1. How do we wish to introduce this feature into the cluster, should it be a Technical Preview in the first instance?
2. Do we want this to be an optional add-on (delivered via OLM) or do we want this to be a default operator for future
   clusters?

### Test Plan

As this project operates solely on resources within the Kubernetes environment, we will aim to leverage the controller
runtime [envtest](https://book.kubebuilder.io/reference/envtest.html) project to write an extensive integration test
suite for the main functionality of the new operator.

In particular, tests should include:
- Behaviour when there is no `ControlPlaneMachineSet` within the cluster
- Behaviour when a new `ControlPlaneMachineSet` is created
  - What happens with regard to the adoption of existing Control Plane Machines
  - What happens if the configuration differs from the existing Machines within the cluster (per strategy)
  - How does the ClusterOperator status present during the adoption
  - What happens if the name is wrong or there are multiple `ControlPlaneMachineSets`
  - How does the status get updated during this process
- Behaviour during a configuration change
  - How does the RollingUpdate strategy behave
  - How does the OnDelete strategy behave
  - How does the status get updated during rollouts
- Behaviour when a `ControlPlaneMachineSet` is removed
  - The effect on the Machines (removing owner references etc)
  - Removal of the finalizer

Notably, these tests will involve a large amount of simulation as we won't have a running Machine controller, nor will
we have an etcd operator adding Machine deletion hooks. The functions of these two operators will be simulated to
exercise the `ControlPlaneMachineSet` operator based on the behaviours that the operator relies on.

As this isn't a true E2E test, we will also duplicate a number of the above tests into an E2E suite that can run on a
real cluster. This E2E suite has the potential to be very disruptive, we expect that a sufficiently well written
integration suite should prevent this however.

We will ensure that the origin E2E suite is updated to include tests for this component to prevent regressions in either
the etcd operator or Machine API ecosystem from breaking the functionality provided by this operator.

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

This enhancement does not describe removing a feature of OpenShift, this section is not applicable.

### Upgrade / Downgrade Strategy

When the new operator is introduced into the cluster, it will not operate until a `ControlPlaneMachineSet` resource has
been created. This means we do not need to worry about upgrades during the introduction of this resource.

### Version Skew Strategy

The `ControlPlaneMachineSet` operator relies on the Machine API. This is a stable API within OpenShift and we are not
expecting changes that will cause version skew issues over the next handful of releases.

### Operational Aspects of API Extensions

We will introduce a new `ClusterOperator` for the `ControlPlaneMachineSet` operator. Within this we will introduce
conditions (TBD) which describe the state of the operator and the control plane which it manages.

This new `ClusterOperator` will be the key resource from which support should discover information if they believe the
`ControlPlaneMachineSet` to be degraded.

There should be no effect to existing operators or supportability of other components due to this enhancement, in
particular, this is strictly adding new functionality on top of an existing API, we do not expect it to impact the
functionality of other APIs.

#### Failure Modes

- A new Machine fails to scale up
  - In this scenario, we expect the operator to turn degraded to signal that there is an issue with the Control Plane
    Machines
  - We expect that the Machine will contain information identifying the issue with the launch request, which can be
    rectified by the user before they then delete the Machine, once deleted, the ControlPlaneMachineSet will attempt the
    scale up again
  - This process should be familiar from dealing with existing Failed Machines
  - A MachineHealthCheck targeting Control Plane Machines could automatically resolve this issue
- The `ControlPlaneMachineSet` webhook is not operational
  - This will prevent creation and update of ControlPlaneMachineSets
  - We expect these operations to be very infrequent within a cluster once established, so the impact should be
    minimal
  - The failure policy will be to fail closed, as such, when the webhook is down, no update operations will succeed
  - The Kube API server operator already detects failing webhooks and as such will identify the issue early
  - The ControlPlaneMachineSet is not a critical cluster operator. Everything done by the operator described here can be
    done manually to the machine(set) objects.

#### Support Procedures

TBD

## Implementation History

There is not current implementation of this enhancement, however, it will depend on the
[etcd quorum protection mechanism](https://github.com/openshift/enhancements/pull/943) which is currently being
implemented.

## Drawbacks

- Introduction of new CRDs complicates the user experience
  - Users will have to learn about the new CRD and how it can be used
  - Counter: The concepts are familiar from existing apps resources and CRDs
- We are making it easier for customers to put their clusters at risk
  - If a scale up fails, this will then need manual intervention, the cluster will become degraded at this point
  - Counter: The etcd protection mechanism should mean that the cluster is never at risk of losing quorum, clusters
    should be recoverable
- We will need confidence in the project before we can ship it
  - We need to decide how to ship the project, will we ship it as GA or TP in the first instance?

## Alternatives

Previous iterations of this enhancement linked in the metadata describe alternative ways we could implement this
feature.

### Layering MachineSets

There has been previous discussion about the use of MachineSets to create the Machines for the Control Plane.
In previous iterations of this enhancement they have been recommendations to either create one MachineSet per
availability zone or to have some `ControlPlaneMachineSet` like CRD create MachineSets and leverage these to create the
Machines.
This proposal deliberately omits MachineSets due to concern over the risks and drawbacks that leveraging MachineSets
poses in this situation.

Exposing MachineSets within this mechanism exposes risk in a number of ways:

- Users have the ability to scale the MachineSet
  - We do not intend to support horizontal scaling initially, there's no easy way to prevent users scaling the
    Control Plane MachineSets while not affecting workload MachineSets
  - If a user were to scale up the MachineSet, something would need to sit on top to scale the MachineSet back to the
    supported control plane replicas count. It is then difficult to ensure that the correct Machine is removed from
    the cluster without jeopardising the etcd quorum. If a user attempted to use GitOps to manage the MachineSet,
    this could cause issues as the higher level controller battles to scale the MachineSet.
- If no higher level object exists, difficult to ensure consistency across MachineSets
  - We promote to users that their control plane should be consistent across availability zones, with separate
    MachineSets, there is nothing to prevent users modifying the MachineSets between zones and having major
    differences. Collating these differences for support issues could prove difficult. Users may also spread their
    Machines in an undesirable manner.
  - Users will have the ability to have inconsistency while using the OnDelete update strategy with
    `ControlPlaneMachineSets`, but this should be easy to track due to the updated replicas count within the
    `ControlPlaneMachineSet` status. We may want to degrade the operator if there are discrepancies to encourage users
    to keep their control plane consistent.
- Users can delete intermediary MachineSets
  - In this case, if a user were to delete the Control Plane MachineSet, it is hard to define a safe way to leave the
    Control Plane Machine(s) behind without having to have very specific knowledge baked into the MachineSet
    controller
  - It becomes easier for users to mismanage their Control Plane Machines and put their cluster at risk
  - Previous enhancements have discussed the use of webhooks to prevent the deletion of Control Plane MachineSets,
    though these are not foolproof. The removal design of the `ControlPlaneMachineSet` (being operator based) should be
    more reliable than a webhook.

## Infrastructure Needed

For a clean separation of code, we will introduce the new operator in a new repository,
openshift/control-plane-machine-set-operator.
