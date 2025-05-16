
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

Considerations:
- This enhancement depends on the presence of Openshift-ServiceCA, which should be the case for 4.20+
- No new resources are added as part of OLMv1 / only controller behavior is changed

Possible downgrade issue scenario:
1. User stamps out ClusterExtension for a bundle with webhooks
2. OLMv1 does not support it -> ClusterExtension reaches a terminal failed state reporting the error
3. OLMv1 is upgraded to include webhook support
4. ClusterExtension re-reconciles and installs bundle with webhooks and reaches a terminal successful state (i.e. resources are created on the cluster)
5. Cluster is downgraded and OLMv1 returns to a state without webhook support

At this point, there are two scenarios:
1. OLM resolves to a bundle with webhook resources -> the ClusterExtension reaches a terminal failed state and does NOT make any further changes to the cluster. This means that the resources installed before downgrade still persist and will be unmanged.
2. OLM resolves to a bundle without webhook resources -> OLM will effectively rollback the installation to the resolved version.

Note: I'm writing this as the current state as of 2025.06.02. I'm raising this issue with the team as I feel we should make a decision regarding rollbacks and, specifically,
auto-rollbacks after a cluster downgrade. This issue should be clarified before this enhancement goes gets merged/goes GA.

## Version Skew Strategy

This feature has a dependency on Openshift-ServiceCA operator and the annotations it provides to manage generation
and injection of webhook service TLS certificates. Changes in Openshift-ServiceCA in how it expects users to 
interface with it, and or configure the certificates may impact this feature. Such breaking changes ought to be caught
by e2e tests and can be addressed before reaching customers.

Another consideration are the versions of the underlying Kubernetes webhook related resources (e.g. ValidatingWebhookConfiguration, etc.)
and CustomResourceDefinition (i.e. where/how conversion webhooks get configured). These changes ought to also be caught 
by our e2e test suite.

## Operational Aspects of API Extensions

### Impact of API Extensions (CRDs, Admission/Conversion Webhooks, Aggregated API Servers, Finalizers)

**ClusterServiceVersion (CSV) as the Source of Truth:**
  *   **Architectural Impact:** OLM acts as a lifecycle manager for these API extensions, translating operator intent (in the CSV) into live Kubernetes resources (`ValidatingWebhookConfiguration`, `MutatingWebhookConfiguration`, `CustomResourceDefinition.spec.conversion`, etc.).
  *   **Operational Impact:**
    *   Operators (via their CSVs) directly influence core API behavior.
    *   Debugging API issues might now require inspecting OLM, package bundle contents, and the resulting webhook configurations, in addition to the webhook services, deployments and pods themselves.
    *   Upgrades/rollbacks of operators managing webhooks become more critical, as a faulty webhook can impact the cluster. OLM needs robust mechanisms to handle this.

**CRDs (with Conversion Webhooks):**
  *   **Architectural Impact:** Conversion webhooks allow these CRDs to evolve their schema over time (e.g., v1alpha1 -> v1beta1 -> v1) without breaking existing clients or stored data. The API server calls the conversion webhook to translate objects between stored and requested versions. OLM will manage the `spec.conversion` section of the CRDs in the bundle.
  *   **Operational Impact:**
    *   If a conversion webhook is unavailable or faulty, clients requesting a different version than the one stored in etcd will fail. This can render CRs unreadable or unwritable for certain API versions, potentially halting controllers that rely on those versions.
    *   Care must be taken during CRD updates to ensure conversion paths are always valid.
    *   Storage in etcd is always for *one* version (the storage version). All other versions are converted on the fly.
    *   An unavailable conversion webhook also causes the core garbage collector controller to fail because it will try to fetch objects that need to pass through the conversion webhook to perform its garbage collection function. 

**Admission Webhooks (Validating & Mutating):**
  *   **Architectural Impact:** These intercept requests to the Kubernetes API server *before* an object is persisted.
    *   **Mutating:** Can modify objects. Called first.
    *   **Validating:** Can accept or reject objects based on custom policies. Called after mutations.
  *   **Operational Impact:**
    *   A misbehaving or unavailable admission webhook (especially with `failurePolicy: Fail`) can block creates, updates, or deletes of specific resources system-wide or in specific namespaces.
    *   They add latency to API requests.
    *   Debugging involves checking API server logs, webhook logs, and the webhook configuration.
    *   OLM will manage the `ValidatingWebhookConfiguration` and `MutatingWebhookConfiguration` objects, CA bundles will be injected by `openshift-service-ca`.

**Finalizers:**
  *   **Architectural Impact:** Finalizers are keys on resources that signal to controllers that pre-deletion cleanup logic is required. The API server will prevent deletion of an object until all its finalizers are removed.
  *   **Operational Impact:**
    *   If a controller responsible for removing a finalizer is down or buggy, resources can get stuck in a "Terminating" state, blocking deletion. This is a common issue.
    *   Webhooks (especially validating ones) might *enforce* the presence or absence of finalizers, or mutating webhooks might add them.

### Conversion/Admission Webhooks Metrics/Alerting

**Metrics**

* apiserver admission webhook related metrics to monitor latency, calls, rejections, errors, and failures
* apiserver conversion webhook related metrics to monitor request counts, response code counts (e.g. 5xx)
* webhook deployment/pod related metrics to monitor service health and readiness
* openshift-service-ca metrics to monitor certificate related issues
* controller_runtime_webhook_requests_total and controller_runtime_webhook_latency_seconds (if webhooks are built with controller-runtime, which many are)

**Alerts:**

* admission/conversion webhook availability
* admission/conversion webhook high latency
* CA bundle not present in admission or conversion webhook configurations 


### Impact on Existing SLIs

**API Availability:**

*   **Admission/Conversion Webhooks with `failurePolicy: Fail`:** If such a webhook is unavailable or misconfigured, it can **block all API operations** (CREATE, UPDATE, DELETE, CONNECT for admission; GET/LIST/WATCH for conversion) for resources it targets. This is a **major availability risk**. OLM stamping these out means operator bugs can now cause cluster-wide API outages for specific resources.
  *   Example: Fails creation of `ConfigMap` in the system when the webhook is not available.
*   **Dependency on Service Network (SDN):** Webhook services are typically exposed via Kubernetes services, meaning the API server needs to reach them over the cluster network. SDN issues can make webhooks unreachable, leading to API unavailability for affected resources if `failurePolicy: Fail`.
  *   Example: Adds a dependency on the SDN service network for all resources, risking API availability in case of SDN issues.
*   **Dependency on `openshift-service-ca`:** If `openshift-service-ca` has issues rotating or injecting CAs, new webhooks might not become trusted, or existing ones might fail after certificate rotation, leading to API unavailability.

**API Throughput & Latency:**

*   **Increased Latency:** Every admission webhook in the chain for a given request adds latency. Mutating webhooks are called serially, then validating webhooks serially. A slow webhook (e.g., >100ms) can significantly degrade overall API performance. If an operator installs a poorly performing webhook, it affects all matching requests.
*   **Conversion Webhook Latency:** Called on GET/LIST/WATCH if version conversion is needed. Slow conversion webhooks make reading objects slow.
*   **Increased Kube-APIServer Load:** More processing for each request, more outgoing network calls from API server to webhooks. This can increase CPU/memory usage on API servers.

**Scalability:**

  *   The number of webhook configurations and the rate of requests they intercept can impact the scalability of the kube-apiserver.
  *   The scalability of the webhook services themselves becomes a factor. If a webhook service can't handle the load from the API server, it becomes a bottleneck.
  *   Example: If 100 operators each install a webhook for ConfigMaps, every ConfigMap operation now invokes 100 webhooks (potentially). This is an extreme, but illustrates the multiplicative effect. `objectSelector` and `namespaceSelector` can mitigate this, but OLM needs to ensure operators use them judiciously.
  *   Example: Expected use-cases require less than 1000 instances of the CRD, not impacting general API throughput.

Note: The use of `namespaceSelector` is discouraged for multi-tenancy use cases. The purpose of that field is to ensure that namespaces that are critical to the functionality of the cluster can be excluded from webhook interception (and avoid potential availability concerns).


### Possible Failure Modes of API Extensions

**Webhook Pod/Service Unavailability:**

*   Pod crashes, OOMKilled, not scheduled, image pull error.
*   Service misconfigured (selector doesn't match pods, wrong port).
*   Network policy blocks API server access to webhook service.
*   Webhook deployment scaled to zero.

**Webhook Logic Errors:**

*   Webhook panics or returns an internal server error (500).
*   Webhook times out (API server has a timeout, typically a few seconds, for webhook calls).
*   Webhook returns an invalid admission/conversion response (e.g., malformed JSON).
*   Mutating webhook creates an invalid object (e.g., breaks schema, removes required fields).
*   Conversion webhook fails to convert an object or produces an invalidly converted object.
*   Infinite loop (e.g. a mutating webhook re-triggers itself by modifying fields that cause it to be called again without a condition to stop).

**Configuration Issues (in `Validating/MutatingWebhookConfiguration`, `CRD.spec.conversion`):**

*   Incorrect `service` reference (name, namespace, port, path).
*   `caBundle` missing, expired, or doesn't match the webhook server's certificate (OLM managing this with ServiceCA greatly mitigates this for the initial setup).
*   `failurePolicy: Fail` set inappropriately for a non-critical or unstable webhook.
*   `rules` too broad, causing the webhook to intercept unintended resources.
*   `objectSelector` or `namespaceSelector` misconfigured.
*   `timeoutSeconds` too short or too long.
*   For CRD conversion: `conversionReviewVersions` in the webhook doesn't match what the API server sends or what the CRD lists.

**`openshift-service-ca` Failures:**

*   Service CA controller fails to inject `caBundle` into webhook configs.
*   Service CA controller fails to sign new certificates for webhook services.
*   Problems with the CA certificate itself (e.g., expiry, though this is managed).

**Network Issues:**

*   SDN problems preventing kube-apiserver from reaching the webhook service endpoint.
*   DNS resolution failure for the webhook service name from the API server.

**Resource Exhaustion:**

*   Kube-apiserver overwhelmed by too many webhook calls or slow webhooks.
*   Webhook pod itself runs out of CPU/memory due to high request volume or inefficient code.

### OCP Teams Likely to be Called Upon in Case of Escalation

1. OpenShift API Server Team
2. OLM Team
3. Networking Team
4. Service CA Team
5. Node Team
6. Logging / Monitoring Teams
7. Layered Product Team


## Support Procedures

If there are problems with the webhooks provided by the installed package, these ought to be detectable by 
admission/conversion webhook related metrics provided by the kube api server. Since these are arbitrary
webhook services provided by the operator authors, there's no one-size fits all support procedure.

As it is now, admins would need to either remove the extension with the offending resources (which could
potentially lead to data loss), or scale down CVO, scale down OLM, and remove the offending resources (as OLM
would put them back if it detects resource drift).

Note: As of this writing I'm bringing up this issue with the OLM team to see if we can add a knob to the ClusterExtension
CR to pause reconciliation.

## Alternatives (Not Implemented)

### Don't do it (and push it to registry+v2)

Not adding the support limits our ability to upgrade bundles across CRD versions due to the lack of conversion webhook
support. This will cut against a central operation of OLMv1 for the currently existing content. This would severly
limit the adoption of OLMv1. A new registry+v2 bundle format is still a while away.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
