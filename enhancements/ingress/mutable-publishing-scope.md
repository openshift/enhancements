---
title: mutable-publishing-scope
authors:
  - "@Miciah"
reviewers:
  - "@candita"
  - "@frobware"
  - "@knobunc"
approvers:
  - "@frobware"
  - "@knobunc"
creation-date: 2021-08-23
last-updated: 2021-08-23
status: implementable
see-also:
replaces:
superseded-by:
---

# Ingress Mutable Publishing Scope

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement defines an approach for allowing users to modify the scope of a
service load-balancer for an IngressController that uses the LoadBalancerService
endpoint publishing strategy type.

## Motivation

Users sometimes wish to change the scope of the service load-balancer (SLB) of
an IngressController that uses the "LoadBalancerService" endpoint publishing
strategy type after the IngressController has been created.  Without this
enhancement, it is only possible to change the scope by deleting and recreating
the IngressController, which is disruptive and incompatible with some workflows.
This enhancement makes it possible to change the scope by updating the
IngressController's spec.  Then, on platforms that support changing the SLB's
scope without deleting and recreating it, the operator performs this update.
For platforms that do not support changing an SLB's scope _in situ_, this
enhancement guides the user through the process of deleting and recreating the
SLB.  When completing the process requires user action that would be disruptive,
the operator clearly communicates this fact and provides instructions for
canceling the operation if the user wishes to back out of the operation.  This
enhancement applies to the default IngressController as well as custom
IngressControllers.

### Goals

1. The cluster administrator can change the scope of the SLB of an IngressController with "LoadBalancerService" endpoint publishing strategy type.
2. If the cloud platform supports mutating scope on an SLB, the operator will perform this operation when the cluster administrator changes the configured scope.
3. If the cloud platform does not support mutating scope, the operator will advise the user how to complete the operation.

### Non-Goals

1. The operator does not delete an SLB for an IngressController that is not marked for deletion.

## Proposal

Some platforms (such as Azure and GCP) support changing the scope of a service
load-balancer between internal and external without deleting and recreating the
load balancer, by setting [a cloud-provider-specific
annotation](https://kubernetes.io/docs/concepts/services-networking/service/#internal-load-balancer)
on the Kubernetes Service object.  On these platforms, the operator merely sets
the annotation to the desired scope, and Kubernetes's service controller and
cloud-provider implementation complete the operation of changing the load
balancer's scope.

Other platforms (such as AWS) require deleting and recreating a load balancer to
change its scope.  This operation is disruptive: It interrupts ingress traffic
and may cause the load balancer's address to change.  On these platforms, the
operator signals that the user must delete the Kubernetes Service object.  Once
the user performs this step, the operator recreates the load balancer with the
desired scope to complete the operation.

In order to signal that the user must take action, the operator sets the
`Progressing` status condition to `True` with a message that indicates the
action that the user must take:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: custom
  namespace: openshift-ingress-operator
spec:
  replicas: 2
  domain: mycluster.com
  endpointPublishingStrategy:
    type: LoadBalancerService
    loadBalancer:
      scope: Internal
status:
  availableReplicas: 2
  conditions:
  - message: Deployment has minimum availability.
    reason: MinimumReplicasAvailable
    status: "True"
    type: Available
  - message: "The IngressController scope was changed from \"External\" to \"Internal\".  To effectuate this change, you must delete the service: `oc -n openshift-ingress delete svc/router-default`; the service load-balancer will then be deprovisioned and a new one created.  Alternatively, you can revert the change to the IngressController: `oc -n openshift-ingress-operator patch ingresscontrolle\
      rs/default --type=merge --patch='{\"spec\":{\"endpointPublishingStrategy\":{\"type\":\"LoadBalancerService\",\"loadBalancer\":{\"scope\":\"External\"}}}}'"
    reason: ScopeChanged
    status: "True"
    type: Progressing
```

Note that the IngressController remains available, and that the user can revert
the change by setting `spec.endpointPublishingStrategy.loadBalancer.scope` back
to its previous value.  That means the user must take one of two actions:

1. Delete the Service referenced in the status condition.
2. Revert the change to the IngressController.

If the user deletes the Service, the operator recreates it with the desired
scope.  Alternatively, if the user reverts the change, the `Progressing` status
condition changes back to `False`, and the state of the IngressController is as
it was before the user first changed the value of
`spec.endpointPublishingStrategy.loadBalancer.scope`.

In addition to deleting the Service explicitly, it is possible to annotate the
IngressController with the newly defined
`ingress.operator.openshift.io/auto-delete-load-balancer` annotation.  If the
operator observes that this annotation is set and that its
`spec.endpointPublishingStrategy.loadBalancer.scope` field has been changed, the
operator automatically deletes the Service if needed to complete a scope-change
operation.  This purpose of this annotation is to simplify automation: A tool
can set the annotation when it creates the IngressController or when it updates
`spec.endpointPublishingStrategy.loadBalancer.scope` to instruct the operator to
perform the operation automatically.  This annotation is not intended for
end-users to use directly.


### Validation

This enhancement adds no new API fields or values and requires no new API
validation.

### User Stories

#### As a cluster administrator, I want my default IngressController to be private (internal)

If the user installs a new cluster without specifying that the cluster is
"private", the default IngressController is created with "External" scope.  This
enhancement enables the cluster administrator to change the scope to "Internal"
as follows:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge --patch='{"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService","loadBalancer":{"scope":"Internal"}}}}'
```

The cluster administrator can check the status of the IngressController as
usual:

```shell
oc -n openshift-ingress-operator get ingresscontrollers/default -o yaml
```

The `Progressing` status condition will indicate whether the cluster
administrator must take further action.  The status condition may indicate that
the cluster administrator needs to delete the Service, like so:

```shell
oc -n openshift-ingress delete services/router-default
```

If the cluster administrator deletes the Service, the operator then recreates it
with internal scope.


#### As a cluster administrator, I want to make a private (internal) IngressController public (external)

The cluster administrator can change the scope of an IngressController to be
external (public) as follows:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/private --type=merge --patch='{"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService","loadBalancer":{"scope":"External"}}}}'
```

Again, the cluster administrator can check the IngressController's status
conditions and may need to delete a Service to complete the change in scope.

#### As a cluster administrator, I want to cancel a change to scope before it is completed

Suppose the cluster administrator has changed the scope of an IngressController
from "External" to "Internal", as follows:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/private --type=merge --patch='{"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService","loadBalancer":{"scope":"Internal"}}}}'
```

Now the cluster administrator realizes that the scope should not be changed at
this time, for example due to the disruption that changing the scope would
cause.  The cluster administrator can cancel the change by changing
`spec.endpointPublishingStrategy.loadBalancer.scope` back to its original value:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/private --type=merge --patch='{"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService","loadBalancer":{"scope":"External"}}}}'
```

#### As the provider of a managed service, I want to automate changing an IngressController's scope

A managed service can initiate the operation and tell the operator to complete
it automatically by annotating the IngressController and changing the scope,
using an API call that is equivalent to the following commands:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge --patch='{"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService","loadBalancer":{"scope":"Internal"}}}}'
oc -n openshift-ingress-operator annotate ingresscontrollers/default ingress.operator.openshift.io/auto-delete-load-balancer=
```

The operator will begin and complete the scope-change operation, including
deleting and recreating the service load-balancer if needed.

### API Extensions

This enhancement makes the following API changes:

- The semantics of the the IngressController API's `spec.endpointPublishingStrategy.loadBalancer.scope` field changes to allow mutation.
- A `Progressing` status condition is added to the set of status conditions reported in the IngressController API's `status.conditions` field.

### Implementation Details

The operator's ingress controller reconciles IngressController objects.  When an
IngressController is created, the controller sets the value of its
`status.endpointPublishingStrategy.loadBalancer.scope` field to the value of the
IngressController's `spec.endpointPublishingStrategy.loadBalancer.scope` field.
With this enhancement, the controller checks whether these two fields differ,
which would indicate that the user had changed the value of
`spec.endpointPublishingStrategy.loadBalancer.scope`.  If the controller
observes a modification to the spec field, the controller updates the status
field.

Additionally, if the operator is running on Azure or GCP, with this enhancement,
the controller updates [the cloud-provider-specific
annotation](https://kubernetes.io/docs/concepts/services-networking/service/#internal-load-balancer)
on the Kubernetes Service object for the IngressController.  Kubernetes's
service controller and cloud-provider implementation perform the necessary
changes to change the service load-balancer's scope, and no further action is
needed on the part of the user or the operator.

If the operator is running on AWS or IBM Cloud, the controller does not update
the Kubernetes Service's annotations.  The controller's status update logic then
observes that the Kubernetes Service does not have the expected annotations for
the scope that is specified in the IngressController object's status, whereupon
it sets the IngressController's `Progressing` status condition to `True` with a
message indicating that the cluster administrator must either revert the change
to the IngressController or delete the Service.

If the operator observes that the Service does not exist (which would be the
case once the user deletes the Service), the operator recreates the service with
the scope indicated in `status.endpointPublishingStrategy.loadBalancer.scope`.

Crucially, by default, the operator *never* deletes the Service as long as the
IngressController is not deleted; the operator requires that the user perform
the disruptive action of deleting the Service in order to complete the
operation.  The only exception is if the
`ingress.operator.openshift.io/auto-delete-load-balancer` annotation is set on
the IngressController, in which case the operator **does** delete the Service.

### Risks and Mitigations

It is important that the operator not perform a disruptive action (such as
deleting a service load-balancer) for an active IngressController.  For this
reason, the operator itself **never** deletes the Service for an
IngressController that has not itself been marked for deletion.

It is also important that the user understand that deleting a service
load-balancer is disruptive (specifically that it may interrupt ingress traffic
and may cause the load balancer's address to change).  For this reason, the
`Progressing` status condition clearly indicates that deleting the Service is
disruptive and provides an option for reverting back to the previous state
without causing disruption.

## Design Details

### Test Plan

The controller that manages the IngressController Deployment and related
resources has unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality.

The operator has end-to-end tests; for this enhancement, the following test is
added:

1. Create an IngressController with the "LoadBalancerService" endpoint publishing strategy type and "External" scope.
2. Verify that the operator creates a Service annotated for external scope.
3. Set the IngressController's endpoint publishing strategy's scope to "Internal".
4. If the operator is running on AWS and IBM Cloud, verify that the IngressController reports `Progressing=True`.
5. If the operator is running on Azure or GCP, verify that the Service is annotated for internal scope.
6. Set the IngressController's endpoint publishing strategy's scope to "External".
7. Verify that the IngressController reports `Progressing=False`.
8. Verify that the Service is annotated for external scope.

### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.  We do not plan to deprecate this feature.

### Upgrade / Downgrade Strategy

If the user modifies `spec.endpointPublishingStrategy.loadBalancer.scope` on a
version of OpenShift without this enhancement and upgrades to a version of
OpenShift with this enhancement, the operator annotates the Service or sets
`Progressing=True` on the IngressController as appropriate.  Thus the operator
may effectuate a latent scope change, but it does not delete the Service.

If the user changes the scope and then downgrades to a version without this
enhancement while the operator is reporting `Progressing=True`, the downgraded
operator does not take any action (other than possibly setting
`Progressing=False` when the downgraded operator recomputes status).

### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

Changing an IngressController's scope can cause disruption to ingress traffic.
In particular, deleting the service load-balancer (SLB) causes disruption to
traffic that the SLB is handling.  For this reason, the operator leaves it up
to the administrator to delete the SLB as necessary.

This enhancement adds a `Progressing` status condition to the IngressController
API to inform the administrator when the IngressController is in an intermediate
state.  If the operator requires user action to complete an operation, the
operator updates the IngressController's `Progressing` condition's status to
`True`, and the condition's message will instruct the user as to what steps to
take.  Once the operation is complete or canceled, then the operator updates the
`Progressing` condition's status to `False`.

#### Failure Modes

Possible failure modes include hitting cloud API quotas or getting other errors
from the cloud API when provisioning or updating an SLB.  Consequences of such
errors could be failure to effectuate a scope change on the SLB or disruption to
ingress traffic.  Assistance from the Network Edge team or the team that is
responsible for the affected cloud platform might be necessary to diagnose
errors.

#### Support Procedures

In general and in particular in the case of updating the scope of an SLB,
Kubernetes's service controller and cloud-provider implementation handle
creating or updating the SLB.  If there is a failure when updating the scope,
the service controller emits an event with reason "SyncLoadBalancerFailed".  The
ingress operator watches for such events and reports any so observed failures by
updating the IngressController's `LoadBalancerReady` status condition with
status `False` and ultimately updating the IngressController's `Available`
status condition with status `False` as well.  When reporting a
"SyncLoadBalancerFailed" failure, the `LoadBalancerReady` status condition's
message includes the error reported in the event.

Until the issue is resolved, the IngressController may be in the old state from
before the update or may be unavailable to handle any traffic until the update
or create is successful.

## Implementation History

* An earlier version of this enhancement that automatically deleted the
 IngressController to effectuate scope changes was made in
 [openshift/cluster-ingress-operator#472](https://github.com/openshift/cluster-ingress-operator/pull/472),
 merged on 2020-10-28.
* The earlier version of the enhancement was backported to OpenShift 4.6.4 in
  [openshift/cluster-ingress-operator#482](https://github.com/openshift/cluster-ingress-operator/pull/482),
  merged on 2020-11-19.
* The earlier version of the enhancement was found to cause several problems:
  * [BZ#1904582](https://bugzilla.redhat.com/show_bug.cgi?id=1904582), filed
    2020-12-04, reported that IngressControllers of clusters installed with
    OpenShift 4.1 and upgraded to OpenShift 4.6 were getting changed to internal
    scope.  This issue was fixed with
    [openshift/cluster-ingress-operator#502](https://github.com/openshift/cluster-ingress-operator/pull/502),
    merged 2020-12-04.
  * [BZ#1906267](https://bugzilla.redhat.com/show_bug.cgi?id=1906267), filed
    2020-12-10, reported that users had been directly setting the
    `spec.loadBalancerSourceRanges` field on the operator-manager Service, and
    that the original mutable-scope enhancement was reverting this change.  At
    this point, the decision was made to revert the feature entirely.
* [openshift/cluster-ingress-operator#514](https://github.com/openshift/cluster-ingress-operator/pull/514)
 reverted the original mutable-scope enhancement and merged on 2020-12-17.
* [openshift/cluster-ingress-operator#582](https://github.com/openshift/cluster-ingress-operator/pull/582)
  restores the original enhancement with modifications that (a) preserve user
  modifications to the operator-managed Service and (b) require the user to
  perform any required delete operation.

## Drawbacks

Mutable scope has a troubled history.  There is a risk that yet more unforeseen
complications exist.

## Alternatives

Users can manually delete and recreate IngressControllers.  This alternative
causes more disruption (i.e., more downtime) than just deleting a Service does.
