---
title: cluster-autoscaler-integration-with-openshift-cluster-api
authors:
  - "@elmiko"
reviewers:
  - "@joelspeed"
  - "@nrb"
  - "@damdo"
approvers:
  - "@joelspeed"
api-approvers:
  - "@joelspeed"
creation-date: 2025-01-16
last-updated: 2025-01-16
tracking-link:
  - https://issues.redhat.com/browse/OCPCLOUD-2116
see-also:
  - https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/cluster-autoscaler-integration.md
  - https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/cluster-autoscaler-operator.md
  - https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/installing-cluster-api-components-in-ocp.md
  - https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/converting-machine-api-to-cluster-api.md
replaces: []
superseded-by: []
---

# Cluster Autoscaler Integration with Cluster API

## Summary

The [Kubernetes Cluster Autoscaler][cas-repo] is a tool for automating provisioning
of compute resources in an OpenShift clusters. It has been
[integrated in OpenShift][cas-ocp] since before the 4.X major release series.
With the migration of OpenShift's machine management components from Machine API to
Cluster API, there are several open questions that must be answered about how the
Cluster Autoscaler will integrate with Cluster API on OpenShift. This enhancemnt
describes how the Cluster Autoscaler, and its operator, will be modified to
interface with Cluster API resources and namespaces on OpenShift.

## Motivation

### User Stories

* As an OpenShift administrator, I want the Cluster Autoscaler to function
  as expected whether I am using the Machine API or Cluster API interface so
  that my users do not experience a loss of features or significant down time.

* As an OpenShift administrator, I want to utilize Cluster API as a single point
  of interaction when using the Cluster Autoscaler for monitoring infrastructure
  activity so that I can reduce the complexity of my automation and observability tooling.

* As an OpenShift user, I want to focus my attention on a single API for machine
  management so that I do not have to learn multiple interfaces for the same functionality.

* As an OpenShift developer, I want to ensure that all features of the Cluster
  Autoscaler continue to work as expected on Cluster API so that our users do
  not experience a regression of functionality.

* As an OpenShift developer, I want to ensure that the Cluster Autoscaler has
  a consistent use pattern as we migrate from Machine API to Cluster API so that
  users are not confused by unexpected behaviors and workflows.

### Goals

* Enable Cluster Autoscaler to use Cluster API resources instead of
  Machine API resources on OpenShift.
* Enable Cluster Autoscaler Operator to recognize Cluster API resource
  targets in addition to Machine API resources.
* Ensure mirroring of autoscaler specific metadata from Machine API
  resources to Cluster API resources.
* Add a conditions slice to the MachineAutoscaler resource status field.

### Non-Goals

* Change the expected namespace of operation for the Cluster Autoscaler or
  Cluster Autoscaler Operator.
* Change the expected workflow for ClusterAutoscaler and MachineAutoscaler
  custom resources.
* Enable the Cluster Autoscaler to manipulate both Machine API and Cluster
  API MachineSets.

## Proposal

Update the Cluster Autoscaler to recognize Cluster API resources instead of Machine API.
This change would bring our version of the autoscaler closer to the upstream
version and would allow us to drop some patches we are carrying. The Cluster
API MachineSet sync controller will be updated to recognize when the
Cluster Autoscaler has made a change to a Cluster API resource and then sync
the change to the corresponding Machine API resource, regardless of which resource
is authoritative.

Update the Cluster Autoscaler Operator to be namespace aware. This requires
changing the operator to recognize when it has a Machine API or Cluster API
reference in a MachineAutoscaler, and then use the appropriate namespace to
locate the resource. The Cluster API MachineSet sync controller will be updated
to ensure that when the Cluster Autoscaler Operator adds the autoscaling
annotations that they are copied to any related resources, regardless of which
is authoritative.

Update the Cluster API MachineSet sync controller to recognize the
scale-from-zero annotations and copy them from the Machine API resources to the
Cluster API resources. This ensures consistent data representation on both
the authoritative and non-authoritative records.

Leave the Cluster Autoscaler and Cluster Autoscaler Operator in the
openshift-machine-api namespace. This will continue the user experience for
managing autoscaling.

### Workflow Description

**cluster administrator** is a human user responsible for managing MachineSet,
ClusterAutoscaler, and MachineAutoscaler resources in a cluster.

1. The cluster administrator creates a Cluster API MachineSet named "scaling-set-1"
  intended for autoscaling.
2. The cluster administrator creates a ClusterAutoscaler resource to deploy
  the cluster autoscaler component in the cluster.
3. The cluster adiminstrator creates a MachineAutoscaler resource referencing the
  "scaling-set-1" MachineSet in the `scaleTargetRef` field.
4. The cluster administrator can see from logs and metadata on the MachineSet
  that it is being evaluated by the cluster autoscaler for autoscaling.

In general, the workflow for users should continue to meet the current expectations
for cluster autoscaler functionality. The main change to the previous workflows
is the inclusion of Cluster API MachineSets (e.g. MachineSets that have the
resource group `cluster.x-k8s.io`) in MachineAutoscaler resources.

### API Extensions

This enhancement will require a change to the `status` field of the MachineAutoscaler
API type. There is no expected change to the `spec` field, or the behavior that
users expect.

The Cluster Autoscaler Operator will be changed to include logic that can detect
the API group for any MachineSet that is referenced in the `scaleTargetRef` field
of MachineAutoscaler resources. The change will instruct the Operator to search
for records in the `openshift-cluster-api` namespace for resources with the
`cluster.x-k8s.io` group, and to search in the `openshift-machine-api` namespace
for resource with the `machine.openshift.io` group.

This change could be incorporated into the API by adding a field for namespace in
the `scaleTargetRef` structure, but given that OpenShift only allows MachineSet
resources to be in one of the two namespaces listed previously, this API change is
not necessary.

The `status` field of the MachineAutoscaler resource will gain a `conditions` field.
This field will be used to express the status of each MachineAutoscaler as it
pertains to normal operation and conflicts that might arise from user configuration
choices.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The changes in this enhancement are focused on OpenShift standalone
topologies. Hypershift, and the hosted control plane topologies based
on it, use Cluster API in a differnt configuration than standalone. As
such, these clusters have their Cluster Autoscaler configured in a
manner that does not depend on the Cluster Autoscaler Operator.

Please see the Hypershift enhancement on [Node lifecycle][hcp-nl] for more
information about the Cluster Autoscaler and its relationship to hosted
control plane topologies.

#### Standalone Clusters

The changes in this enhancement are specifically meant for standalone clusters.
These changes are not meant for deployment to Hypershift and hosted control plane
topologies as they do not use the Cluster Autosaler Operator, and have a different
configuration for the Cluster Autoscaler and Cluster API.

#### Single-node Deployments or MicroShift

The changes in this enhancement are not meant for single-node deployments. The
Cluster Autoscaler and Cluster Autoscaler Operator are not deployed in single-node
clusters, and as such this feature does not have relevance on that topology.

### Implementation Details/Notes/Constraints

The implementation changes for this enhancement will be done in the Cluster
Autoscaler, Cluster Autoscaler Operator, and Cluster API MachineSet Sync Controller
projects. Most of the details are internal changes to how Kubernetes resource data
is applied and updated. The user-facing changes will be confined to the MachineAutoscaler
resource associated with the Cluster Autoscaler Operator.

#### Cluster Autoscaler Changes

To ensure proper operation and reduce the complexity of integration, the Cluster
Autoscaler will be configured to interface with Cluster API MachineSet resources.
This change is primarily a change to the running configuration of the Cluster Autoscaler.

Due to historical work on the Cluster Autoscaler and the sibling relationship of the
Machine API and Cluster API, the Cluster Autoscaler customization has been reduced to
environment variables and a few carried patches to express the Machine API functionality.
Reconfiguring the Cluster Autoscaler for Cluster API will mean that some of the carried
patches can be removed, and a change in the deployment configuration.

#### MachineAutoscaler Spec Changes

The main implementation detail of this enhancement that users will interact with is
how the Cluster Autoscaler Operator will locate the MachineSet resources referenced by
the MachineAutoscaler resource.

Previously, only a Machine API MachineSet (i.e. a `MachineSet` kind in the
`machine.openshift.io` API group) would be valid target of the `.spec.scaleTargetRef`
field. After this enhancement is implemented, users may specify either a Machine
API MachineSet or a Cluster API MachineSet in the `.spec.scaleTargetRef` field.

For example, assume a cluster has the following MachineSet resources:

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  labels:
    machine.openshift.io/cluster-api-cluster: example-cluster
  name: example-cluster-worker-somezone-1
  namespace: openshift-machine-api
spec:
  ...
status:
  authoritativeAPI: MachineAPI
```

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  labels:
    machine.openshift.io/cluster-api-cluster: example-cluster
  name: example-cluster-worker-somezone-2
  namespace: openshift-machine-api
spec:
  ...
status:
  authoritativeAPI: ClusterAPI
```

A user could then create the following MachineAutoscaler resources:

```yaml
apiVersion: "autoscaling.openshift.io/v1beta1"
kind: "MachineAutoscaler"
metadata:
  name: "worker-somezone-1"
  namespace: "openshift-machine-api"
spec:
  minReplicas: 1
  maxReplicas: 12
  scaleTargetRef:
    apiVersion: machine.openshift.io/v1beta1
    kind: MachineSet
    name: example-cluster-worker-somezone-1
```

```yaml
apiVersion: "autoscaling.openshift.io/v1beta1"
kind: "MachineAutoscaler"
metadata:
  name: "worker-somezone-2"
  namespace: "openshift-machine-api"
spec:
  minReplicas: 1
  maxReplicas: 12
  scaleTargetRef:
    apiVersion: cluster.x-k8s.io/v1beta1
    kind: MachineSet
    name: example-cluster-worker-somezone-2
```

Note that the MachineAutoscaler named "worker-somezone-1" is targeting a Machine API
MachineSet while "worker-somezone-2" is targeting a Cluster API MachineSet. The
Cluster Autoscaler Operator will know by the `apiVersion` field whether to look
for the resource in the `openshift-machine-api` or `openshift-cluster-api` namespace
respectively.

To support existing user experiences and workflows, the user might also have created
the "worker-somezone-2" MachineAutoscaler using the Machine API reference as follows:

```yaml
apiVersion: "autoscaling.openshift.io/v1beta1"
kind: "MachineAutoscaler"
metadata:
  name: "worker-somezone-2"
  namespace: "openshift-machine-api"
spec:
  minReplicas: 1
  maxReplicas: 12
  scaleTargetRef:
    apiVersion: machine.openshift.io/v1beta1
    kind: MachineSet
    name: example-cluster-worker-somezone-2
```

The Cluster Autoscaler Operator will update the Machine API MachineSet resource, and
then the MachineSet sync controller will sync the change to the Cluster API MachineSet
resource. The sync controller will use the managed fields (i.e. `.metadata.managedFields`)
of the specified MachineSet to determine if the Cluster Autoscaler Operator made
changes to the annotations, and then replicate those appropriately. In this manner,
a user might specify an authoritative or non-authoritative MachineSet in the
`scaleTargetRef` and the sync controller will be able to properly mirror the
changes by detecting that the Cluster Autoscaler Operater has authored the changes.

#### MachineAutoscaler Status Changes

Another implementation detail of this enhancement that users will interact with is the
addition of Kubernetes conditions to the MachineAutoscaler status field
(i.e. `.status.conditions`).

Historically, the MachineAutoscaler resource has not contained information to aid users in
determining the state and health of that scaling group. With this enhancement however, it
is possible that users might encounter issues when interacting with non-authoritative
MachineSet resources. To improve the user experience, a conditions field will be added
to the MachineAutoscaler status field (i.e. `.status.conditions`).

There will be a single condition to indicate the ready status of the MachineAutoscaler. Under
normal operation this condition will appear similar to this example:

```yaml
status:
  conditions:
    - lastTransitionTime: 2025-01-01T00:00:00Z
      message: MachineAutoscaler ready for autoscaling
      reason: MachineAutoscalerReady
      status: "True"
      type: Ready
```

In a case where the user has specified the same MachineSet in multiple MachineAutoscaler resources,
the condition would appear as follows:

```yaml
status:
  conditions:
    - lastTransitionTime: 2025-01-01T00:00:00Z
      message: targetScaleRef has multiple MachineAutoscaler owners
      reason: MachineAutoscalerDuplicateTargetScaleRef
      status: "False"
      type: Ready
```

In this manner, errors in user configuration will be exposed through the MachineAutoscaler
resource to assist users in diagnosing error states.

#### MachineSet Sync Controller Changes

The behavior of the MachineSet sync controller is another focus of implementation detail.
Specifically, the MachineSet annotations and the replicas field of the spec
(i.e. `.spec.replicas`). As mentioned previously, the MachineSet sync controller will
use the managed fields metadata to know who, or what, has updated a field. Based on the field,
and the author of the update, the sync controller will either propagate the change
or synchronize with the authoritative resource.

The updates that the MachineSet sync controller will watch for fall into a few different
categories:

* `.metadata.annotations` changes will be synced from the authoritative resource to the
  non-authoritative except in these cases:
  * The Cluster Autoscaler Operator has added the minimum and maximum size annotations, and ownership
    annotation to a record. If the sync controller sees an update to these annotations on a
    non-authoritative resource originating from the Cluster Autoscaler Operator, it will copy
    that change to the authoritative resource if no MachineAutoscaler is referencing the
    authoritative resource.
  * A provider MachineSet controller has added the scale from zero annotations to a
    non-authoritative record. This occurs when the Cluster API resource is marked as
    authoritative but the Machine API resource is updated by the provider MachineSet controller.
    In these cases the scale from zero annotations will be copied to the non-authoritative
    Cluster API resource. The data from the MachineSet controller is only applied to
    Machine API resources currently.
* `.spec.replicas` changes will be synced from the Cluster API MachineSet to the Machine
  API MachineSet regardless of which is authoritative when the change originates from the
  Cluster Autoscaler. As the Cluster Autoscaler will be configured to operate against Cluster
  API resources only, there will be a need to identify when the Cluster Autoscaler has updated
  a non-authoritative Cluster API resource so that the authoritative resource can be updated.
  This will only occur when the sync controller observes and update to the replicas field from
  the Cluster Autoscaler.

### Risks and Mitigations

One of the possible risks associated with this implementation is conflicting
MachineAutoscaler resources; where a user has created two resources
that reference both the authoritative and non-authoritative MachineSet. This
could create a race condition where updating the minimum and maximum size
values will lead to an inaccurate update to both MachineSets.

To address the risk of possible race conditions on MachineAutoscalers we have a few
options:

* Only allow MachinceAutoscaler target scale references to specify the authoritative MachineSet.
  This would prevent users from using the non-authoritative references in their
  MachineAutoscalers, but _may_ be perceived as a regression in user experience. Enabling this
  would require the Cluster Autoscaler Operator to inspect the MachineAutoscaler target
  references to ensure that only the authoritative resources are referenced.
* Implement precedence rules within the Cluster Autoscaler Operator so that it can choose
  the proper MachineAutoscaler resource based on the authoritative reference. This would
  allow users to create multiple MachineAutoscalers which could reference the
  authoritative and non-authoritative MachineSet resources without creating a race
  condition on the MachineSet updates. Enabling this would require building the
  precedence logic into the Cluster Autoscaler Operator, and updating the user
  documentation to explain the precedence rules.

#### Security

This change will allow users to specify MachineSet references that will exist in two
namespaces. Although the entry is not free form, and the reference will be validated
by the Cluster Autoscaler Operator, the OpenShift Cloud Infrastructure team has
reviewed this design for possible security vulnerabilities and considers the risk low.

Before releasing this change, it will be reviewed by product security for final analysis.

#### User Experience

This enhancement describes a small change in the user experience with most of the
feature work landing in the Cluster Autoscaler Operator. The Cloud Infrastructure has
designed this change to follow the current user experience without introducing
new concepts for enabling autoscaling on any specific MachineSet resource.

The addition of conditions to the status will improve the user experience by giving
direct feedback on warning and errors that may arise during operation. The Cloud
Infrastructure team has designed this change to reduce confusion for users and to
provide a clear path for triage.

Outside of the Cluster Autoscaler Operator, users will need to understand that the
Cluster Autoscaler is now focused on Cluster API MachineSets instead of Machine API
MachineSets. This change will require users to understand the authoritative and
mirroring mechanisms around the Cluster API migration to maximize their operation
and maintenance activities.

These changes should be clear for users, but to help mitigate the risk of misinterpretation,
the user experience focused changes in this enhancement will be documented in the
official OpenShift documentation.

### Drawbacks

A drawback of the approach defined in this enhancement is that it could cause
confusion among users. The change in API behavior introduced in this enhancement
will allow users to specify the MachineSet resource of their choice when
enabling autoscaling. Due to the mirrored nature of the Machine API and Cluster API
MachineSets, users may not immediately comprehend which resource they should
specify, and what workflow is recommended by Red Hat.

To reduce confusion, the changes described in this enhancement will be reviewed in
the OpenShift product documentation, with examples to clarfiy the intended usage.
A post on the OpenShift developer blog (https://developers.redhat.com/blog) to
describe this feature in greater depth could be added to any Cluster API change
post threads to help add informational material.

Another approach to reducing confusion would be to allow only a single type of
MachineSet resource (Machine API or Cluster API) to be specified as a target for
autoscaling. This approach could work if the Cluster API resources are chosen as
the target, but would represent a hard shift in the current MachineAutoscaler
behavior and would require a conversion migration for all upgrades where cluster
autoscaler is in use.

As the behavior described in this enhancement is making an extension to current
workflows, without changing the ingestion API, the drawbacks to implementing this
enhancement are low. By contrast, to enable cluster autoscaling for Cluster API
resources _without_ this enhancement will require a redesign of how we express
autoscaling in OpenShift.

## Open Questions

1. Will we encounter race conditions when dealing with synchronizing the autoscaling
  metadata between authoritative and non-authoritative resources with the Cluster
  Autoscaler Operator and MachineSet sync controller?
2. Will we want to remove MachineAutoscalers that reference Cluster API MachineSets
  during a downgrade?

## Test Plan

The current autoscaler end to end tests will be expanded to cover the new Cluster
API specific functionality. The business logic of our current cluster autoscaler
tests will continue to have relevance, the primary addition will be testing the
authoritative/non-authoritative mechanisms described in this enhancement.

Unit tests will be added to the Cluster Autoscaler Operator and MachineSet sync
controller to exercise the precendence and mirroring logic.

## Graduation Criteria

The foundational technology that this enhancement is based on (cluster autoscaling) is
already considered to be "GA" in OpenShift. The changes described in this enhancement
will be dependent on the addition of Cluster API to OpenShift; the required
functionality is summared in these enhancements:

* [Cluster API Integration][capi-integ-enh]
* [Installing Cluster API components in OpenShift][capi-inst-enh]
* [Converting Machine API resources to Cluster API][capi-conv-enh]

The features described in this enhancement will follow graduation of their parent
enhancements, and will use a feature gate to ensure that the features are not
released prematurely.

### Dev Preview -> Tech Preview

- Ability to use either Machine API or Cluster API MachineSets with MachineAutoscalers
- Cluster Autoscaler uses only Cluster API MachineSet resources
- End user documentation
- Sufficient test coverage, end to end and unit
- Gather feedback from users rather than just developers
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Available by default
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feture
- Deprecate the feture

## Upgrade / Downgrade Strategy

For upgrades, the features described in this enhancement do not pose a risk for
users nor do they require conversions. Any MachineAutoscaler resources currently
referencing Machine API MachineSet resources will continue to work as expected
after upgrade.

Given that there will be no Cluster API MachineSet resources in the cluster prior
to upgrade, there could be no MachineAutoscaler referencing a Cluster API MachineSet.
This means that no upgrade conversion or migration is required for this scenario.

For downgrades, the features described in this enhancement might cause some
MachineAutoscalers to enter error states. If a MachineAutoscaler is referencing a
Cluster API MachineSet before a downgrade, and is not removed, it will cause
the Cluster Autoscaler Operator to report errors in its logs for those
MachineAutoscaler resources. Additionally, these MachineAutoscaler resources
will not be able to updated as the calls to update the previous scale target
may not be able to find the Cluster API MachineSet resource. In all other respects
a downgrade will be benign as the Cluster API resources will no longer be
reconciled by OpenShift operators, see
[Upgrade/Dowgrade Strategy of conversion enhancement][capi-conv-enh-ud].

## Version Skew Strategy

The changes described in this enhancement are limited to the MachineAutoscaler
API resource, the Cluster Autoscaler Operator, the Cluster Autoscaler, and the
MachineSet sync controller. As the Cluster Autoscaler Operator is the only controller
that reconciles the MachineAutoscaler resource, a skew in versions with other
OpenShift components represents a low risk for adverse behavior.

During an upgrade, configured MachineAutoscalers will continue to work as expected.
There is a possibility for a gap in autoscaling services when cluster conditions
have a skew with the Cluster Autoscaler. In scenarios where the Cluster Autoscaler is
skewed behind the version of the Cluster Autoscaler Operator, it is possible that
Cluster API MachineSets will not be processed by the Cluster Autoscaler. This condition
would not make the cluster unusable and would be cleared when the Cluster Autoscaler
is updated.

## Operational Aspects of API Extensions

The Cluster Autoscaler Operator will continue to be deployed in the `openshift-machine-api`
namespace. Previously it had only inspected resources (ClusterAutoscaler, MachineAutoscaler,
and MachineSet) in the same namespace. After this enhancement is implemented the
Cluster Autoscaler Operator will be able to inspect MachineSet resources in the
`openshift-cluster-api` namespace as well. The decision of which namespace to use when
inspecting records is determined by the API group version of the `targetScaleRef`
field in a MachineAutoscaler resource.

Changes to the Cluster Autoscaler will not have a marked change in user experience. The
expected workflows for operation, maintenance, and triage will continue to follow
proscribed patterns for OpenShift. The key difference is that the Cluster Autoscaler
will only integrate with Cluster API MachineSets after this enhancement is implemented.

The failure modes around the changes in this enhancement will be focused on the behavior
of the MachineAutoscaler API type and the Cluster Autoscaler. The most common failures
will come from incorrectly configured MachineAutoscaler resources: either referencing a
previously referenced MachineSet, or referencing a non-existent MachineSet. A less common
failure could be possible through user intervention that would overwrite the autoscaling
related API fields. In both of these cases, the effects on the cluster will be that some
autoscaling related activities might not work as expected. These failure modes will not
cause a disruption to holistic cluster usage.

## Support Procedures

The first point of failure when using the features described in this enhancement will be
the MachineAutoscaler resource. During a failure scenario, the expected symptoms include:

* The target MachineSet not being scaled by the cluster autoscaler.
* The target MachineSet not having the minimum and maximum scale annotations (`cluster-api-autoscaler-node-group-min-size` and `cluster-api-autoscaler-node-group-max-size`).
* The `Ready` condition on the MachineAutoscaler being set to `False`.

These symptoms will not affect crucial cluster operation, but may impact cluster autoscaling
capabilities for the MachineSets specified in the MachineAutoscaler resources.

To triage issues related to the symptoms, users should follow these steps:

1. Inspect the failing MachineAutoscaler's conditions in the status field. This will contain
  a detailed message about the failure.
1. Confirm the details of the `targetScaleRef` field in the failing MachineAutoscaler.
1. Search the logs for the Cluster Autoscaler Operator for warnings and errors related to the
  failing MachineAutoscaler.
1. Search the logs for the Cluster Autoscaler for warnings and errors related to the
  MachineSet targetted by the MachineAutoscaler.

It is safe to delete and recreate a failing MachineAutoscaler without disrupting cluster operation.

## Alternatives

### Cluster Autoscaler

#### Recognize Both Machine API and Cluster API Resource Groups

The Cluster Autoscaler could be updated to read both Machine API and Cluster API MachineSets.
This would require adding a patch to the autoscaler that will carry the logic for reading and
writing both API groups’ resources, and will be able to distinguish between the authoritative
and non-authoritative resources. This alternative requires carrying a significantly complex
patch on our fork of the autoscaler, some of this code might be shared with the Cluster API
MachineSet sync controller and could be extracted to a common library.

This alternative has been dismissed as a possible implementation due to the complexity
associated with carrying the extra patches on the Cluster Autoscaler, and the
likelyhood of race conditions with the MachineSet synchronization workflow.

#### Only Recognize Machine API Resource Groups

This is how we run the autoscaler today, it would require us to continue carrying our
Machine API patches indefinitely. This change would keep the autoscaler in its current state
and require the Cluster API MachineSet sync controller to recognize when the autoscaler has
changed the Machine API resources and propagate them accordingly. This possibility has a
large gap in that it will not be useful in situations where a Cluster API resource exists
without a corresponding Machine API resource.

This alternative has been dismissed as a possible implementation due to it not addressing
user workflows that only include Cluster API MachineSets without corresponding Machine API
MachineSets.

### Cluster Autoscaler Operator

#### No Change

Currently the operator does not discriminate between the API groups and versions that are
referenced in the MachineAutoscaler. This means that it should continue to function as
expected regardless of whether it references Machine API or Cluster API resources. A
downside to this approach is that the operator looks for resources in its own namespace
(currently openshift-machine-api), and the Cluster API related resources will be in a
different namespace. This means that there would need to be a Machine API MachineSet
equivalent for any Cluster API MachineSet that a user might want to scale. The Cluster
API MachineSet sync controller will need to be updated to ensure that when the Cluster
Autoscaler Operator adds the autoscaling annotations that they are copied to any related
resources, regardless of which is authoritative.

This alternative has been dismissed as a possible implementation due to the limitation
of requiring all resources to exist in the openshift-machine-api namespace. This
limitation will prevent user workflows where only Cluster API MachineSets exist.

#### Only Allow Machine API Targets

The Cluster Autoscaler Operator could be updated to only allow the use of Machine API
references as targets. This would mean restricting the functionality of the operator
to only recognize Machine API resources. The Cluster API MachineSet sync controller
would need to be updated to recognize the Machine API MachineSet changes and copy them
into the Cluster API resources regardless of which resource is authoritative.

This alternative has been dismissed as a possible implementation due to it not addressing
user workflows that only include Cluster API MachineSets without corresponding Machine API
MachineSets.

#### Only Allow Cluster API Targets

Similar to the previous alternative, the Cluster Autoscaler Operator could be updated
to only allow the use of Cluster API references as targets. This alternative would
require a similar level of change as the previous option with the notable exception
being the target of action, and allowing the operator to view resources in the
openshift-cluster-api namespace.

This alternative has been dismissed as a possible implementation due to the regression
in user experience that it would impose. This alternative would require all
MachineAutoscalers to convert their target references to use the Cluster API version of
any MachineSet to continue inclusion in autoscaling. This conversion could be done
through automation, but the regression in user experience is considered high enough
to dismiss this option.

### Namespaces and Resources

#### Migrate to the Cluster API Namespace

Migrate the Cluster Autoscaler and Cluster Autoscaler Operator to the new
openshift-cluster-api namespace. This change would require that we change the
deployment artifacts for the Cluster Autoscaler and Cluster Autoscaler Operator to the
new location. Due to the fact that much automation is built on top of the current locations
and namespaces, this possibility will require extensive documentation and migration
information for users.

This alternative has been dismissed as a possible implementation at this time due to
the transitive toil it would create in adjusting all the build and test workflows. This
may be reconsidered in the future but is not included in this enhancement.

#### Migrate Autoscaling to Its Own Namespace

Migrate the Cluster Autoscaler and Cluster Autoscaler Operator to their own namespace,
for example `openshift-cluster-autoscaling`. This option requires a similar level of effort
as moving to the openshift-cluster-api namespace but carries the possible advantage of
allowing us to better separate distinct controllers in OpenShift.

Similar to the previous alternative, this has been dismissed as a possible implementation
at this time due to the transitive toil it would create in adjusting all the build and
test workflows. This may be reconsidered in the future but is not included in this enhancement.


[cas-repo]: https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler
[cas-ocp]: https://docs.openshift.com/container-platform/4.17/machine_management/applying-autoscaling.html
[hcp-nl]: https://github.com/openshift/enhancements/blob/master/enhancements/hypershift/node-lifecycle.md
[capi-inst-enh]: https://github.com/openshift/enhancements/blob/master/enhancements/cluster-api/installing-cluster-api-components-in-ocp.md
[capi-integ-enh]: https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/cluster-api-integration.md
[capi-conv-enh]: https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/converting-machine-api-to-cluster-api.md
[capi-conv-enh-ud]: https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/converting-machine-api-to-cluster-api.md#upgrade--downgrade-strategy
