---
title: lb-subnet-selection-aws
authors:
  - "@gcs278"
reviewers:
  - "@candita"
  - "@frobware"
  - "@rfredette"
  - "@alebedev87"
  - "@JoelSpeed, for review regarding CCM"
approvers:
  - "@Miciah"
api-approvers:
  - "@deads2k"
creation-date: 2024-03-06
last-updated: 2024-03-06
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/NE-705
see-also:
replaces:
superseded-by:
---

# LoadBalancer Subnet Selection for AWS

## Summary

This enhancement extends the IngressController API to allow a user to specify
custom subnets for LoadBalancer-type services for AWS. By default, AWS
auto-discovers the subnets and has its own logic for tie breaking if there
are multiple subnets per availability zone. This enhancement will configure
the `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation on the
LoadBalance-type service which will manually configure the subnets for
each availability zone.

it consist in selecting only those subnets which: 1) belong to the VPC of the cluster, 2) belong to the cluster (have kubernetes.io/cluster/{cluster-name} tag), 3) have public or internal ELB role (tagged with kubernetes.io/role/elb or kubernetes.io/role/internal-elb)

## Motivation

Cluster admins on AWS may have dedicated subnets for their load balancers due to
security reasons or infrastructure constraints. Currently, cluster admins can manually
add the `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation to the service.
However, this approach is considered bad practice since the service is managed
by the Ingress Operator. Additionally, the annotation must be added after the
load balancer-type service has already been created by the Ingress Operator. This
means that an Ingress Controller may initially be deployed in a broken state.
Cluster admins need an API for the Ingress Controller which will populate this
annotation simultaneously with the creation of LoadBalancer-type Services.

### User Stories

#### Day 2 Load Balancer Subnet Selection on AWS

_"As a cluster admin, when configuring an IngressController in AWS, I want to
specify the subnet selection for the LoadBalancer-type Service on Day 2."_

The cluster admin can modify the IngressController to specify a list of subnets,
one for each availability zone, for the LoadBalancer-type Service. This is available as a
Day-2 operation to the cluster admin.

#### Load Balancer Subnet Selection Migration on AWS

_"As a cluster admin, I want to migrate an IngressController load balancer-type
service, currently using an unmanaged service.beta.kubernetes.io/aws-load-balancer-subnets
annotation, to using the new IngressController API field so that the annotation is now
managed."_

A cluster admin manually set the `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation,
and now wishes to OpenShift v4.17 to use new IngressController API field, `spec.TBD`.

1. Cluster admin confirms that initially `service.beta.kubernetes.io/aws-load-balancer-subnets` is set on the load
   balancer-type service managed by the IngressController.
2. The cluster admin upgrades OpenShift to v4.17 and the annotation is not removed or changed.
3. The cluster admin sets `spec.TBD` on the IngressController to the same value as their annotation,
   and the annotation is unchanged, but now is managed by the Ingress Controller.
4. From now on, the cluster admin uses `spec.TBD` to adjust the ingress controller subnets.

### Goals

- Introduce a new field in the IngressController API for subnet selection in AWS for Day 2 operations.
- Support migration from an unmanaged `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation to using the
  new API field.

### Non-Goals

- Support an install-time (Day 1) option for subnet selection of the default ingress controller.
- Extend support to platforms other than AWS.

## Proposal

The `IngressController` API is extended by adding an optional parameter `Subnets`
of type `[]string` to the `AWSLoadBalancerParameters` struct, to manage the
`service.beta.kubernetes.io/aws-load-balancer-subnets` annotation on the load
balancer-type service.

The `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation is used
for both Classic Load Balancers (CLBs) and Network Load Balancers (NLBs), hence
adding it to `AWSLoadBalancerParameters` enable common configuration for both.

While AWS, GCP, and Azure provide annotations for subnet configuration, AWS's
annotation accepts for multiple subnets, whereas GCP and Azure only permit a
single subnet. For this reason, we made this API specific to AWS by adding
the configuration in `AWSLoadBalancerParameters`.

```go
// AWSLoadBalancerParameters provides configuration settings that are
// specific to AWS load balancers.
// +union
type AWSLoadBalancerParameters struct {
  [...]

  // subnets specifies the list of subnets for the load balancer to
  // route traffic to. The values can be either a subnetID or subnetName.
  // Each subnet should be from a different availability zones, otherwise
  // the AWS controller logic will break the tie based on the role tag,
  // the cluster tag, and/or lexicographic order.
  // 
  // When omitted, the subnets will be auto-discovered per availability zone.
  //
  // +optional
  Subnets []string `json:"subnets,omitempty"`
}
```

### Workflow Description

1. A cluster admin, who wants to specify the subnets for a load balancer, creates an IngressController with
  `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.subnets` specified.
2. The ingress operator reconciles the LoadBalancer Service and sets the
   `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation with the subnets provided by the user.

### API Extensions

This proposal will modify the `IngressController` API by adding a new
field called `Subnets` of type `[]string` to the `AWSLoadBalancerParameters`
struct type.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift & HCP impact TBD

#### Standalone Clusters

Standalone Clusters impact TBD

#### Single-node Deployments or MicroShift

This proposal does not have any special implications for single-node
deployments or MicroShift.

### Implementation Details/Notes/Constraints

The value set for `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.subnets`
will be used to set the `service.beta.kubernetes.io/aws-load-balancer-subnets`
annotation on the LoadBalancer Service created by the ingress controller.

If `service.beta.kubernetes.io/aws-load-balancer-subnets` is set and `Subnets` is empty,
the controller won't remove or clear the `service.beta.kubernetes.io/aws-load-balancer-subnets`
annotation. This avoids breaking existing users that have manually configured the
`service.beta.kubernetes.io/aws-load-balancer-subnets` annotation when they upgrade to
an OpenShift version that supports the `Subnet` field.

However, `service.beta.kubernetes.io/aws-load-balancer-subnets` will be
removed when detecting a transition to an empty value for `Subnet`. This
maintains compatibility with upgrades while enabling the ingress operator
to fully manage this field. This is unlike the [LoadBalancer Allowed Source Ranges](/enhancements/ingress/lb-allowed-source-ranges.md)
enhancement which will never remove the `spec.loadBalancerSourceRanges` field
from the service when the `spec.endpointPublishingStrategy.loadBalancer.allowedSourceRanges`
field is cleared. This leads to a less-than-desirable user experience,
requiring the cluster admin to manually clear the `spec.loadBalancerSourceRanges`
field on the service.

// TODO: Revise this, I think we need to add a new status field to detect
//       transition as `status.endpointPublishingStrategy` will show effective
//       values and not indicate the transition.
To detect the transition of `Subnet`, we will leverage the fact that the
`status.endpointPublishingStrategy` field on the `IngressController`
currently gets automatically populated with the effective fields for
`EndpointPublishingStrategy`. 

### Risks and Mitigations

#### Invalid Subnets

If a cluster admin provides an invalid subnet, the AWS Cloud Controller Manager (CCM)
will not reconcile the service:

- If a service is created with an invalid subnet, the load balancer will not be
  created.
- If an existing service has an invalid subnet added, any future updates to the service
  will remain unreconciled until the invalid subnet is fixed. 

The logs from the CCM will appear as follows:

```shell
$ oc logs -n openshift-cloud-controller-manager aws-cloud-controller-manager-7f6bd55cdb-fkglk
[...]
E0327 22:10:30.548205       1 aws.go:4319] Error listing subnets in VPC: "expected to find 1, but found 0 subnets"
E0327 22:10:30.548290       1 controller.go:298] error processing service default/test (retrying with exponential backoff): failed to ensure load balancer: expected to find 1, but found 0 subnets
```

Adding an invalid subnet to an existing Ingress Controller can lead to confusing
scenarios. Without immediate indication, a cluster administrator might mistakenly
believe that adding the subnet was successful, only to discover that future
updates to the service will not be reconciled.

#### Upgrade Compatibility Risk

As mentioned in [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints),
a cluster admin may have configured `service.beta.kubernetes.io/aws-load-balancer-subnets`
directly on the load balancer-type service. The Ingress Operator doesn't remove or
reconcile this service annotation. However, there is a risk that clusters with this
services annotation set directly, may break on upgrades because the Ingress Operator
now manages the annotation and removes it.

This is mitigated by the solution outlined in
[Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints). TBD.

### Drawbacks

This enhancement brings additional engineering complexity
for upgrade scenarios because cluster admins have previously
been allowed to directly add this annotation on a service.

Debugging invalid subnets will be confusing for cluster
admins and may lead to extra support cases or bugs.

## Open Questions

- Do we need to configure subnets for default ingress controller at cluster installation (i.e. Day1 API)?

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
