---
title: provisioning-request-enablement
authors:
  - "@elmiko"
reviewers: # Useful for reviewers to know how the Cluster Autoscaler works in OpenShift
  - "@joelspeed"
  - "@derekwaynecarr"
  - "@enxebre"
  - "@kannon92"
  - "@mrunalp"
approvers:
  - "@joelspeed"
  - "@enxebre"
api-approvers:
  - "@joelspeed"
creation-date: 2025-02-10
last-updated: 2025-02-10
tracking-link:
  - https://issues.redhat.com/browse/OCPCLOUD-2704
see-also: []
replaces: []
superseded-by: []
---

# ProvisioningRequest Enablement

## Summary

The Kubernetes resource `provisioningrequests.autoscaling.x-k8s.io`, known by the kind
`ProvisioningRequest`, is an API to request additional capacity that a user would like
to provision within the cluster. The Kubernetes Cluster Autoscaler can reconcile
ProvisioningRequests to inform its scaling decisions so that it can create nodes in
expectation of a user workload. This enhancement is about enabling the use of
ProvisioningRequest resources in OpenShift.

## Motivation

The ProvisioningRequest enables users to inform the Cluster Autoscaler that a workload will
be expected, and to begin creating nodes for the workload. Under normal operation, the
Cluster Autoscaler will not begin creating nodes until the pods of the workload are marked
as pending. The ability to create nodes ahead of an impending workload is useful when
scheduling batch style workloads where the size of upcoming workloads is known ahead of
time. Advanced scheduling applications, notably Kueue, have begun to integrate
ProvisioningRequests into their logic to improve efficiency and reduce downtime.

Job queueing and batch scheduling style workloads are popular in the Kubernetes community.
Having integrated support for ProvisioningRequests on OpenShift will give users of advanced
scheduling application the best experience by delivering resources in an efficient manner.

### User Stories


**cluster administrator** is a human user responsible for deploying cluster-wide changes
for users of the cluster.

**application administrator** is a human user responsible for deploying a namespaced application
in a cluster.

Stories:
* As an application admnistrator, I would like the Cluster Autoscaler to respect ProvisioningRequests
  so that Kueue workloads can have infrastructure resources available when the workload is scheduled.
  Having the ProvisioningRequest type available and the Cluster Autoscaler configured to reconcile
  that resource will solve my problem.
* As a cluster admnistrator who is writing automation and integrating new applications,
  I would like to write and use tools that will create more cluster resources based on predictive resource
  requests from my users. Being able to use ProvisioningRequests with the Cluster Autoscaler will
  allow me to achieve my goals.

### Goals

* Include the `provisioningrequests.autoscaling.x-k8s.io` type in OpenShift.
* Configure the Cluster Autoscaler to enable the controller for ProvisioningRequests.

### Non-Goals

* Rename the ProvisioningRequest API type, group, or version.
* Add an API field to the ClusterAutoscaler resource to allow configuration of the ProvisioningRequest controller.

## Proposal

1. Add the ProvisioningRequest custom resource definition to the manifest payload for the Cluster Autoscaler Operator.
2. Modify the Cluster Autoscaler Operator to enable the `--enable-provisioning-requests` flag of the Cluster Autoscaler.
3. Update the Cluster Autoscaler RBAC to allow reading and writing of ProvisioningRequests.

### Workflow Description

This feature is exposing an API resource type and does not provide the user with additional
controls for configuring how the resource is consumed. There are two general workflows
that users will encounter:

**cluster administrator** is a human user responsible for deploying cluster-wide changes
for users of the cluster.

**application administrator** is a human user responsible for deploying an application in
a cluster.

**Example workflow 1**

1. The cluster administrator configures the Cluster Autoscaler by creating a ClusterAutoscaler
  resource and one or more MachineAutoscaler resources.
2. The application administrator creates a manifest for a ProvisioningRequest resource in any
  namespace and applies it with the `oc` command line tool.
3. The Cluster Autoscaler reconciles the ProvisioningRequest and begins creating the
  requested cluster capacity.

**Example workflow 2**

1. The cluster administrator configures the Cluster Autoscaler by creating a ClusterAutoscaler
  resource and one or more MachineAutoscaler resources.
2. The cluster administrator installs and configures Kueue for cluster users, with support
  for ProvisioningRequest enabled.
3. The application administrator submits their workload to Kueue for scheduling.
4. At the appropriate time, Kueue creates a ProvisioningRequest resource in any
  namespace to represent the workload, and then the Cluster Autoscaler reconciles the resource
  and begins to create the requested cluster capacity.

### API Extensions

- Adds the Kubernetes custom resource definition for `provisioningrequests.autoscaling.x-k8s.io`
  to all OpenShift installations where the Cluster Autoscaler Operator is also installed.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift clusters utilize the Cluster Autoscaler by deploying it through a Hypershift controller
instead of the Cluster Autoscaler Operator. As such, this API change should be deployed by a component
associated with Hypershift to ensure it is available. For Hypershift, this enhancement
proposes the following changes:

* The hosted control plane operator will install the `provisioningrequests.autoscaling.x-k8s.io`
  resource definition in guest cluster by including a payload manifest for the Cluster Version
  Operator to install.
* The hosted control plane operator will always enable the `--enable-provisioing-requests`
  flag when deploying the Cluster Autoscaler.
* Update the Cluster Autoscaler RBAC to allow modification of ProvisioningRequests.

#### Standalone Clusters

Standalone clusters utilize the Cluster Autoscaler Operator to deploy and operate the Cluster
Autoscaler. This operator currently installs resource definitions for ClusterAutoscaler and
MachineAutoscaler resource types. This enhancements proposes the following changes:

* The Cluster Autoscaler Operator will install the `provisioningrequests.autoscaling.x-k8s.io`
  resource definition in its installation manifests.
* The Cluster Autoscaler Operator will always enable the `--enable-provisioing-requests`
  flag when deploying the Cluster Autoscaler.
* Update the Cluster Autoscaler RBAC to allow modification of ProvisioningRequests.

#### Single-node Deployments or MicroShift

This change does not affect single-node deployments and Microshift as they do not
install the Cluster Autoscaler Operator.

### Implementation Details/Notes/Constraints

The main implementation detail for this enhancement is the inclusion of the
ProvisioningRequest custom resource definition.

Currently, the Cluster Autoscaler Operator contains custom resource definitions for
the ClusterAutoscaler and MachineAutoscaler types in its payload manifests. Following
in this pattern, the `Makefile` for the Cluster Autoscaler Operator will be modified to
include a target for rendering the ProvisioningRequest manifest from vendored files
containing the Cluster Autoscaler API package. The resulting custom resource definition
manifest will be included in the payload manifests for the operator to be deployed by
the Cluster Version Operator.

The second implementation detail worth noting is the change in default configuration
for the Cluster Autoscaler. In order to properly reconcile ProvisioningRequests in
a cluster, the Cluster Autoscaler must be started with the command line flag
`--enable-provisioning-requests=true`. The Cluster Autoscaler Operator will be
updated to include this flag as a default. There will not be a corresponding
API field to allow user configuration of this flag.

### Risks and Mitigations

This change is generally low risk but there are a few concerns to consider:

1. Unauthorized creation of infrastructure resources. Because the ProvisioningRequest
  resource will lead to infrastructure resource creation, it must have a proper role and
  permissions for users to create. This will follow the same role-based authentication
  control pattern as other Kubernetes resources, but is worth noting as creation of
  ProvisioningRequests in any namespace could lead to the creation of infrastructure
  resources.

### Drawbacks

One drawback to this approach is the inclusion of ProvisioningRequest as a supported type
on core OpenShift. This adds complexity to OpenShift not only in operation but in support
as well. It is entirely possible that users could install the ProvisioningRequest
custom resource definition on their own.

The trade-offs for having the user install the ProvisioningRequest definition on their own
come in two areas: user experience, and Cluster Autoscaler configuration.

The user experience for managing custom resource definitions that interact with Kubernetes
components, such as the Cluster Autoscaler, requires users to understand the version
changes and differences. Users need to understand when the project versions have changed
to ensure that they are installing the correct versions of an API resource. Further, they
will need to manually migrate on upagrades of the underlying platform.

In addition to installing the ProvisioningRequest, users will need to configure the Cluster
Autoscaler to reconcile the resources as well. This will require the addition of a new
field in the ClusterAutoscaler resource on OpenShift.

## Open Questions

1. Do we want to create a field in the ClusterAutoscaler resource to allow disabling of the
  ProvisioningRequest?

## Test Plan

* Unit tests in openshift/cluster-autoscaler-operator to confirm deployment configuration.
* End-to-end tests openshift/cluster-api-actuator-pkg to confirm the ProvisioningRequest
  behavior with Cluster Autoscaler.
* Integration tests with Kueue to confirm that the Cluster Autoscaler will react to Kueue's
  ProvisioningRequest creation.
* Test upgrade to confirm that feature is available.
* Test downgrade to confirm that feature is disabled.

## Graduation Criteria

This feature is currently released as stable in the Kubernetes community. As such, its
graduation within OpenShift will be accelerated. This feature will be created behind
the `TechPreviewNoUpgrade` feature gate. The feature gate will be promoted once testing
has confirmed that the feature does not introduce regressions, and after the feature
has been made available for internal feedback following review.

### Dev Preview -> Tech Preview

- This feature will start in Tech Preview based on internal conversations and
  the maturity of the upstream integrations.
- Feature gate for API changes and Cluster Autoscaler behavior

### Tech Preview -> GA

- Unit testing passing
- End-to-end testing passing
- Upgrade/downgrade testing
- Sufficient time for feedback
- Available by default
- Conduct integration testing with Kueue
- Conduct load and scale testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

To ensure proper behavior of the Cluster Autoscaler during and after upgrades, it
will be necessary to ensure that the command line flag for enabling ProvisioningRequest
support is added, and that it is restarted. The Cluster Autoscaler Operator is responsible
for managing the lifecycle of the Cluster Autoscaler and it will need to restart the
Cluster Autoscaler after an upgrade to change the Deployment configurations.

Likewise, when downgrading OpenShift to a version before ProvisioningRequests are
enabled, the command line flag will need to be disabled.

Upgrade expectations:
- ProvisioningRequest resource definition will be available after upgrading to OpenShift
  version 4.19.0 or greater.
- When upgrading from OpenShift versions less than 4.19.0 to a version 4.19.0 or greater,
  the Cluster Autoscaler will be restarted to add the command line flag that enables
  ProvisioningRequest support.

Downgrade expectations:
- When downgrading to OpenShift versions less than 4.19.0 from a version equal to or
  greater than 4.19.0, the ProvisioningRequest resource definition will be removed.
- When downgrading to OpenShift versions less than 4.19.0 from a version equal to or
  greater than 4.19.0, the Cluster Autoscaler will be restarted to remove the command
  line flag that enables ProvisioningRequest support.

## Version Skew Strategy

Version skews of plus or minus one version are expected during upgrades and downgrades,
and should not negatively affect a cluster's operation.

In the rare case of a version skew beyond the recommended Kubernetes policy of plus or
minus two versions, the Cluster Autoscaler might be affected but should be the only
affected component. The Cluster Autoscaler is the only component in the OpenShift core
payload that will react to ProvisioningRequest resources. In the event of a version
skew that goes beyond the recommended policy, the Cluster Autoscaler may become
unresponsive to ProvisioningRequest resources.

## Operational Aspects of API Extensions

* The primary API impact of this enhancement is the addition of the
  `provisioningrequest.autoscaling.x-k8s.io` type, also known as `ProvisioningRequests`, to
  the OpenShift API server.
* There will not be an admission controller or webhook associated with the
  ProvisioningRequest installed by default.
* The Cluster Autoscaler is the only component in the OpenShift payload that will react to
  ProvisioningRequests.
* This change should not have an impact on the Kubernetes control plane controllers.

## Support Procedures

* If the Cluster Autoscaler is not running, inspect the logs for the cluster-autoscaler-operator
  in the `openshift-machine-api` namespace. This operator controls the lifecycle of the Cluster
  Autoscaler and it may have failed to start due to an error in command line flags or mismatched
  version.
  * Additionally, inspect the logs for the cluster-autoscaler in the `openshift-machine-api`
    namespace. It might have captured an error log about the nature of the failure. If a command
    line flag error is the root cause, it will be captured in this log.
* If the Cluster Autoscaler is not creating new nodes in response to a ProvisioningRequest,
  inspect the logs for the Cluster Autoscaler in the `openshift-machine-api` namespace. If the
  Cluster Autoscaler is not able to create nodes, or is not able to read the ProvisioningRequest,
  it will be reported in these logs.
  * Inspect the permissions associated with the service account for the Cluster Autoscaler to ensure
    that it had read and write ProvisioningRequest resources.
  * Inspect the custom resource definitions in the cluster to ensure that
    `provisioningrequests.autoscaling.x-k8s.io` is present.

* Failure modes for the features described in this enhancement will be focused on the Cluster
  Autoscaler and its behavior. If ProvisioningRequests are not available, or the Cluster
  Autosacler is not creating resources in response to ProvisioningRequests, it will not affect
  the core functionality of the cluster. The worst affect on the cluster will be the inability
  to create nodes in response to ProvisioningRequests. Nodes will continue to be created by
  the Cluster Autoscaler in response to pending pods.

## Alternatives (Not Implemented)

This feature is stable in the Kubernetes community and as such does not have a reasonable
alternative for implementation.

The choice of how to integrate this feature with OpenShift does have at least one alternative
option:

Instead of packaging the ProvisioningRequest resource definition with the OpenShift payload,
the user could be responsible for providing the custom resource definition. This would also
require the addition of an API field on the ClusterAutoscaler to allow users to configure
the Cluster Autoscaler to use ProvisioningRequests.

This alternative would make for a simpler development effort by Red Hat, but would leave
large parts of the integration to the user's responsibility. This option was considered
during the design process, but was disregarded due to the complication and lack of support
it creates for users.
