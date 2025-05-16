
---
title: olmv1-webhook-support
authors:
  - perdasilva
reviewers:
  - thetechnick
  - joelanford
approvers:
  - joelanford
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2025-05-16
last-updated: 2025-05-19
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OPRUN-3862
---

# OLMv1: Support for bundles with Webhooks

## Summary

This enhancement proposes adding support for registry+v1 bundles that describe validating, mutating, and/or conversion 
webhooks in their ClusterServiceVersion to OLMv1. OLMv1 should manage the correct generation of webhook related 
bundle resources, e.g. ValidatingWebhookConfigurations, MutatingWebhookConfiguration, etc., but also use 
Openshift-ServiceCA to manage the TLS certificates for the webhook service(s).


## Motivation

To foster a thriving operator ecosystem and protect existing investments, 
OLM v1 aims to support operators packaged in the registry+v1 bundle format, including those with webhooks. 
By preserving compatibility with the current operator landscape, we ensure a smooth transition for operators and 
end-users to the OLM platform. This approach not only accelerates OLM adoption but also safeguards the stability and 
functionality of existing workloads on the OpenShift clusters.


### User Stories

- As an Operator author, I want to use Kubernetes' Dynamic Admission Control features so that I can fulfill particular requirements of my application.
- As an Operator author, I want to use a 'Webhook' conversion strategy in my CustomResourceDefinition(s) (CRD), so that users can be migrated to a newer version of the CRD.
- As an OLMv1 user, I want to use an Operator that includes webhooks, so that I can reap the benefits of that application on my cluster.
- As a cluster Admin, I want all services to be secured with TLS certificates, so that I promote a secure cluster environment and reduce the surface area for attacks and data leaks.

### Goals

- Users can rely on OLM v1 to manage operators packaged in registry+v1 bundle format, including those with webhooks.
- Operator authors can rely on OLM v1 to manage the lifecycle of webhooks included in their registry+v1 bundle-packaged operators without modifications.
- Users can rely on OLM v1 to detect webhook misconfigurations, such as webhooks reporting no available endpoints, and troubleshoot the underlying Service's Pods.

### Non-Goals

- Custom management of TLS certificates (as it was with OLMv0)

## Proposal

Update the OLMv1 registry+v1 to resource manifest process to:
1. Produce the appropriate `ValidatingWebhookConfiguration` resources for any [WebhookDescription](https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/clusterserviceversion_types.go#L178) with `Type` "ValidatingAdmissionWebhook" defined in the bundle's ClusterServiceVersion's [.spec.webhookdefinitions](https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/clusterserviceversion_types.go#L282) 
2. Produce the appropriate `MutatingWebhookConfiguration` resources for any [WebhookDescription](https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/clusterserviceversion_types.go#L178) with `Type` "MutatingAdmissionWebhook" defined in the bundle's ClusterServiceVersion's [.spec.webhookdefinitions](https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/clusterserviceversion_types.go#L282)
3. Appropriately update `.spec.conversion` for any bundle CRD referenced in the `.spec.conversionCRDs` for any [WebhookDescription](https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/clusterserviceversion_types.go#L178) with `Type` "ConversionWebhook" defined in the bundle's ClusterServiceVersion's [.spec.webhookdefinitions](https://github.com/operator-framework/api/blob/master/pkg/operators/v1alpha1/clusterserviceversion_types.go#L282)
4. Produce Service resources appropriately configured to match the webhook configuration specifications and to target the correct deployment described in the bundle's ClusterServiceVersion

Add support for Openshift-ServiceCA by ensuring:
1. The `service.beta.openshift.io/serving-cert-secret-name: <secret-name>` annotation is added to the generated webhook services
2. The `service.beta.openshift.io/inject-cabundle: true` is present on all generated `ValidatingWebhookConfiguration`, `MutatingWebhookConfiguration`, and CustomResourceDefinitions that are configured for a webhook conversion strategy
3. Updating webhook service backing deployment pod template specs to mount the tls configuration from the Openshift-ServiceCA generated `Secret`.

Ensure parity with OLMv0 behavior by:
1. Generating the `ValidatingWebhookConfiguration`, `MutatingWebhookConfiguration`, and `Service` resource with in the same way


### Workflow Description

There should be not workflow differences. User can install/upgrade bundles containing webhooks in the exact same way
as they would for bundles that don't. This enhancement presents a broadening of the content that is consumable by users
with no changes in user experience.

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

OLMv1 does not yet support Hypershift. Given that the bundle installation happens on the user cluster, I don't believe
there should be any specific Hypershift considerations. I.e. not resources will be installed in the central cluster.

#### Standalone Clusters

This change only affects the types of workloads that the customer will run. It should not have any major effect on 
OLMv1 itself.

#### Single-node Deployments or MicroShift

This change only affects the types of workloads that the customer will run. It should not have any major effect on
OLMv1 itself.

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

Because we are transpiling a ClusterServiceVersion into a set of coherent resources, there is a security implication that
well crafted bundles _could_ potentially be malicious in nature and could be used to subvert to host cluster. These
risks are mitigated by:
- Allowing cluster admins to configure the scope of RBAC they are comfortable with for a particular bundle
- Allowing cluster admins to control who gets write access to ClusterExtensions/ClusterCatalogs
- Allowing cluster admins to select the ClusterCatalogs from which content can be sourced

Additionally, Webhooks come with services, which could be a potential avenue for data leaks or security issues. All
communications between webhook components is secured via TLS certificates generated and rotated automatically by
Openshift-ServiceCA.

### Drawbacks

None that I could think of.

## Open Questions [optional]


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
- End-to-end tests
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

N/A:
- This enhancement depends on the presence of Openshift-ServiceCA, which should be the case for 4.20+
- No new resources are added as part of OLMv1 / only controller behavior is changed

:thinking: one possible situation that I don't know is relevant for this section:
1. User stamps out ClusterExtension for a bundle with webhooks
2. OLMv1 does not support it -> ClusterExtension reaches a terminal failed state
3. OLMv1 is upgraded to include webhook support
4. ClusterExtension reconciles and installs bundle with webhooks
5. Cluster is downgraded and OLMv1 returns to state without webhook support
6. ClusterExtension returns to terminal failed state but bundle + webhooks are still installed

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

## Alternatives (Not Implemented)

### Don't do it (and push it to registry+v2)

Not adding the support limits our ability to upgrade bundles across CRD versions due to the lack of conversion webhook
support. This will cut against a central operation of OLMv1 for the currently existing content. This would severly
limit the adoption of OLMv1. A new registry+v2 bundle format is still a while away.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
