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
last-updated: 2026-05-29
tracking-link:
  - https://issues.redhat.com/browse/AUTOSCALE-192
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

* As an OpenShift user, I want all features of the Cluster Autoscaler continue
  to work as expected on Cluster API so that I do not experience a regression of
  functionality.

* As an OpenShift developer, I want to ensure that the Cluster Autoscaler has
  a consistent use pattern as we migrate from Machine API to Cluster API so that
  users are not confused by unexpected behaviors and workflows.

### Goals

* Enable Cluster Autoscaler to use Machine API and Cluster API resources
  depending on which are declared authoritative.
* Enable Cluster Autoscaler to use standalone Cluster API resources that
  do not have a Machine API representation.
* Enable Cluster Autoscaler Operator to select Cluster API resources
  in addition to Machine API resources as target for scaling.
* Add a conditions slice to the MachineAutoscaler resource status field
  to give improved error messages.

### Non-Goals

* Change the expected namespace of operation for the Cluster Autoscaler or
  Cluster Autoscaler Operator.
* Change the expected workflow for ClusterAutoscaler and MachineAutoscaler
  custom resources.

## Proposal

Update the Cluster Autoscaler to have a new provider specifically for OpenShift.
The OpenShift provider will enable scaling of Machine API and Cluster API resources
within a cluster. The OpenShift provider will be implemented to wrap the existing
Cluster API provider so that resources can be evaluated and processed by the proper
provider. In this manner, the OpenShift provider will respond to Machine API
authoritative resources and will utilize the Cluster API provider to respond to
Cluster API authoritative resources.

Revert the OpenShift Machine API specific changes that are carried on the Cluster API
provider. Currently (as of OpenShift 4.22) the Cluster Autoscaler provider for Cluster
API as used by OpenShift carries patches to allow for Machine API specific behavior.
With the creation of the OpenShift provider, the carried patches for Machine API
behavior will be migrated to the OpenShift provider and the Cluster API provider will
mirror the upstream implementation.

Update the Cluster Autoscaler Operator to deploy the OpenShift provider and to
recognize Cluster API resources. The Cluster Autoscaler Operator will need to
deploy the Cluster Autoscaler with the OpenShift provider enabled and the necessary
configuration for Cluster API. The operator will also need to be updated to
allow the specification of Cluster API MachineSet resources as targets for
MachineAutoscalers. The operator will also need to detect when the user has
specific a non-authoritative resource and have a feedback mechanism for users
to indicate the error.

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

Additionally, as Hypershift uses the Cluster API provider and that provider
will have the Machine API carry patches removed from it, the Cluster Autoscaler
on Hypershift will be using a more pure upstream version of the Cluster API
provider.

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

#### OpenShift Kubernetes Engine

For OpenShift Kubernetes Engine installations that utilize machine management through
Machine API and Cluster API, and where cluster autoscaling is provided by the Cluster
Autoscaler Operator, the changes described in this enhancement should work in a similar
manner as standalone OpenShift.

### Implementation Details/Notes/Constraints

The implementation changes for this enhancement will be done in the Cluster
Autoscaler and Cluster Autoscaler Operator projects. Most of the details are internal
changes to how Kubernetes resource data is applied and updated. The user-facing changes
will be confined to the MachineAutoscaler resource associated with the Cluster
Autoscaler Operator.

#### Cluster Autoscaler Changes

To ensure proper operation and reduce the complexity of integration, the Cluster
Autoscaler will gain a new provider specifically for OpenShift. The new provider
will wrap the Cluster API provider so that it can utilize existing logic and isolate
the Machine API specific behaviors.

Due to historical work on the Cluster Autoscaler and the sibling relationship of the
Machine API and Cluster API, the Cluster Autoscaler customization has been reduced to
a provider change, environment variables, and a few carried patches to express the Machine
API functionality. Reconfiguring the Cluster Autoscaler for Cluster API will mean that the
carried patches on the Cluster API provider can be removed in favor of a carry patch for the
OpenShift provider, this will result in a reduction of complexity for maintenance.

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

The Cluster Autoscaler Operator will only update the authoritative MachineSet resource.
In the case where a user specifies a non-authoritative resource as the `scaleTargetRef`,
this will be considered an error. The `status` field of the MachineAutoscaler will indicate
the type of error. For examples of error conditions, please see the section on MachineAutoscaler
Status Changes.

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

In cases where the user has specified a non-authoritative MachineSet, the condition would appear
as follows:

```yaml
status:
  conditions:
    - lastTransitionTime: 2025-01-01T00:00:00Z
      message: targetScaleRef is not the authoritative resource
      reason: MachineAutoscalerNonAuthoritativeTargetScaleRef
      status: "False"
      type: Ready
```

In this manner, errors in user configuration will be exposed through the MachineAutoscaler
resource to assist users in diagnosing failure states.

#### RBAC Updates

Both the Cluster Autoscaler and the Cluster Autoscaler Operator will need to have their roles
updated to allow reading and writing of Cluster API resources in the `openshift-cluster-api`
namespace.

### Risks and Mitigations

A primary risk associated with this change is misconfiguration of MachineAutoscaler
resources. When creating a MachineAutoscaler, the user is responsible for specifying
the target of scaling. As the Cluster Autoscaler Operator will now reject target
resources that are non-authoritative, users will need to learn to specify the
authoritative resource as the target, and they will need to update their processes to
ensure inspection of the `status` field on MachineAutoscalers.

#### Security

This change will allow users to specify MachineSet references that will exist in two
namespaces. Although the entry is not free form, and the reference will be validated
by the Cluster Autoscaler Operator, the OpenShift Cloud Infrastructure team has
reviewed this design for possible security vulnerabilities and considers the risk low.

Before releasing this change, it will be reviewed by product security for final analysis.

#### User Experience

This enhancement describes a small change in the user experience with most of the
user-facing work landing in the Cluster Autoscaler Operator. The changes introduced with
this enhancement follow the current user experience without introducing new concepts
for enabling autoscaling on any specific MachineSet resource.

The addition of conditions to the status will improve the user experience by giving
direct feedback on warning and errors that may arise during operation.

This change will require users to understand the authoritative and mirroring mechanisms
around the Cluster API migration to maximize their operational and maintenance activities.

These changes should be clear for users, but to help mitigate the risk of misinterpretation,
the user experience focused changes in this enhancement will be highlighted in the
OpenShift documentation.

### Drawbacks

A drawback of the approach defined in this enhancement is that it could cause
confusion among users. The change in API behavior introduced in this enhancement
will allow users to specify the MachineSet resource of their choice when
enabling autoscaling, but they will need to specify the authoritative resource.
Due to the mirrored nature of the Machine API and Cluster API MachineSets, users
may not immediately comprehend which resource they should specify, and what
workflow is recommended by Red Hat.

To reduce confusion, the changes described in this enhancement will be reviewed in
the OpenShift product documentation, with examples to clarfiy the intended usage.
A post on the OpenShift developer blog (https://developers.redhat.com/blog) to
describe this feature in greater depth could be added to any Cluster API change
post threads to help add informational material.

Due to the migration to Cluster API as the machine management interface for OpenShift,
the change to autoscaling will need to occur at some point. The changes described
in this enhancement represent the minimal changes necessary to ensure functionality.

As the behavior described in this enhancement is making an extension to current
workflows, without changing the ingestion API, the drawbacks to implementing this
enhancement are low. By contrast, to enable cluster autoscaling for Cluster API
resources _without_ this enhancement will require a redesign of how we express
autoscaling in OpenShift.

## Open Questions

1. Will we encounter race conditions when dealing with synchronizing the autoscaling
  metadata between authoritative and non-authoritative resources with the Cluster
  Autoscaler Operator and MachineSet sync controller?
  As the Cluster Autoscaler and Cluster Autoscaler Operator will only ever read from or
  write to the authoritative resource, this should not pose a problem.
2. Will we want to remove MachineAutoscalers that reference Cluster API MachineSets
  during a downgrade?
  (Answered on review by @JoelSpeed)
  As we don't support downgrades, this is not a concern for this enhancement. Since
  we won't automatically create the MachineAutoscaler changes during an upgrade,
  downgrading a failed upgrade shouldn't produce an issue.

## Test Plan

The current autoscaler end to end tests will be expanded to cover the new Cluster
API specific functionality. The business logic of our current cluster autoscaler
tests will continue to have relevance, the primary addition will be testing the
authoritative/non-authoritative mechanisms described in this enhancement.

Unit tests will be added to the Cluster Autoscaler Operator and Cluster Autoscaler
OpenShift provider where appropriate.

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
API resource, the Cluster Autoscaler Operator, and the Cluster Autoscaler.
As the Cluster Autoscaler Operator is the only controller that reconciles the
MachineAutoscaler resource, a skew in versions with other OpenShift components
represents a low risk for adverse behavior.

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

## Alternatives (Not Implemented)

### Cluster Autoscaler

#### Upgrade the Cluster API provider to recognize authoritative resources

The Cluster Autoscaler Cluster API provider could be updated to read both Machine API and
Cluster API MachineSets. This would require adding a patch to the autoscaler that will carry
the logic for reading and writing both API groups’ resources, and will be able to distinguish
between the authoritative and non-authoritative resources. This alternative requires carrying
a significantly complex patch on our fork of the autoscaler.

This alternative has been dismissed as a possible implementation due to the complexity
associated with carrying the extra patches on the Cluster Autoscaler Cluster API provider.

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
