---
title: configurable-ingress-workload-resources
authors:
  - "@davidesalerno"
reviewers:
  # Ingress
  - "@Miciah"
  - "@bentito"
  # QE
  - "@melvinjoseph86"
approvers:
  - "@Miciah"
api-approvers:
  - "@JoelSpeed"
  - "@everettraven"
creation-date: 2026-10-23
last-updated: 2026-10-28
status: provisional
tracking-link:
  # Epic
  - "https://redhat.atlassian.net/browse/NE-727"
  # User Story
  - "https://issues.redhat.com/browse/NE-2926"
see-also:
  - "/enhancements/ingress/select-haproxy-version.md"
  - "/enhancements/ingress/haproxy-thread-tuning.md"
replaces:
superseded-by:
---

# Configurable Ingress Workload Resources

## Summary

Allow cluster administrators to configure resource requests and limits
independently for the router/controller and HAProxy workloads managed by
an `IngressController`. Each target accepts maps of Kubernetes resource
names to quantities, including CPU, memory, and extended resources such
as `qat.intel.com/cy`. Omission retains the operator's existing defaults
and does not introduce default limits. This proposal concerns resources
of the router pods, not resources of the Ingress Operator, and allocating
a device does not enable cryptographic acceleration.

## Motivation

Traffic processing and route configuration have different resource
requirements. Administrators need to size both workloads without editing
operator-managed Deployments, and need to express device allocations
without an API limited to CPU and memory. The API must make the resource
target unambiguous and preserve existing behavior for unconfigured clusters.

### User Stories

* As a cluster administrator, I want to set CPU and memory requests and
  limits for HAProxy and the router/controller independently, so that
  traffic volume and configuration churn can be sized separately.
* As a cluster administrator, I want to request a valid named extended
  resource for a specific workload without allocating it to every
  container in the pod.
* As a cluster administrator, I want to observe and correct workload
  resource overrides through supported configuration and existing ingress
  health signals, so that a mis-sized limit can be detected and remediated
  without editing operator-managed Deployments.
* As a cluster administrator, I want to remove overrides and return to
  supported defaults, and to understand the scheduling and availability
  consequences before changing them.

### Goals

* Support requests and limits for both existing workload containers,
  independently, per `IngressController` and per replica.
* Support valid Kubernetes resource-name-to-quantity entries rather than
  an allowlist containing only CPU and memory.
* Define partial configuration, reset, validation, and reconciliation
  behavior without changing omitted/default behavior.
* Make device-allocation, security, and rollout risks explicit.

### Non-Goals

* Configure resources of the Ingress Operator itself, the one-shot init
  container, the optional syslog container, or arbitrary injected containers.
* Add containers or pods, or assign independent limits to processes
  sharing a container.
* Implement this API or its controller in this documentation change.
* Install device plugins, enable a HAProxy/OpenSSL QAT provider or engine,
  promise accelerated TLS, or configure Envoy Gateway.
* Expose DRA resource claims, pod-level resource budgets, or in-place
  pod resizing through this API.

## Proposal

### Workflow Description

A cluster administrator selects an `IngressController` and configures
resource overrides for either or both workloads. The API validates the
configuration, and the Ingress Operator reconciles the corresponding
Deployment pod template. Existing Deployment rollout mechanisms replace
pods. The administrator observes rollout progress, scheduling events,
resource settings, and ingress health. Removing overrides restores the
operator-managed baseline for the affected keys.

The current topology is established by the pinned public
[router Deployment asset][router-deployment] and its
[construction and update logic][deployment-code]:

| Workload | Existing pod entry | Default CPU request | Default memory request |
| --- | --- | --- | --- |
| Router/controller | `containers[name=router]` | `25m` | `64Mi` |
| HAProxy | `initContainers[name=haproxy]`, `restartPolicy: Always` | `100m` | `256Mi` |

HAProxy is a native restartable sidecar in the same pod as the router,
not a separate pod and not an ordinary `containers` entry. The pod shares
a process namespace and HAProxy configuration volume. A separate
`init-router` init container copies initial configuration; an optional
syslog container can also be added. Neither is a resource target here.
The [HAProxy launcher][haproxy-launcher] starts the HAProxy process;
the [router base image][router-image] supplies `openshift-router`.
Resource settings apply to each target container as a whole, including
its processes, not to each process independently.

The table describes the inspected public source baseline, not every
historical release or every admitted pod. Neither target has explicit
CPU/memory limits in that asset. Namespace policies and admission may
affect the final pod. No container split is proposed by this enhancement.

### API Extensions

The [API PR #3134 draft][api-pr] adds optional `spec.resources` to
`operator.openshift.io/v1` `IngressControllerSpec`, with two optional targets.
This describes a draft API, not operator support or availability:

| Proposed field | Target |
| --- | --- |
| `spec.resources.router` | Router/controller only |
| `spec.resources.haproxy` | HAProxy data plane only |

Each target exposes `requests` and `limits` maps with free-form keys
intended as resource names and `resource.Quantity` values; key syntax is
not yet validated by the draft (see below). Neither target is a
pod-wide total or a value copied to every container; setting
`spec.resources` alone, without a target, configures nothing. Both maps
can contain valid named extended resources, on either target; their use
does not imply that the selected application can consume the allocation.

The following is a **proposed configuration**:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: example
  namespace: openshift-ingress-operator
spec:
  resources:
    haproxy:
      requests:
        cpu: "250m"
        memory: "384Mi"
        qat.intel.com/cy: "1"
      limits:
        cpu: "1"
        memory: "768Mi"
        qat.intel.com/cy: "1"
    router:
      requests:
        cpu: "50m"
        memory: "96Mi"
      limits:
        cpu: "200m"
        memory: "192Mi"
```

Only HAProxy receives the QAT allocation in this example. Requests apply
per replica; this is not a shared pool across an IngressController's pods.
The [Envoy Gateway private-key-provider guide][envoy-qat] illustrates
`qat.intel.com/cy: '1'` in requests and limits, but separately requires
hardware, a device plugin, and a crypto provider.

The [draft source][ingress-api] uses focused
`IngressControllerResourceRequirements` with only
`requests` and `limits`, both `corev1.ResourceList`, not
`corev1.ResourceRequirements`. Each map permits at most eight entries;
quantity representations must parse, be nonnegative, and have at most
64 characters. Generic resource keys are retained, not reduced to CPU/memory.
The [feature registration][resource-feature] enables
`IngressControllerResourceConfiguration` in `DevPreviewNoUpgrade` and
`TechPreviewNoUpgrade`, not `Default` or `OKD`. These draft choices remain
subject to API review; `claims` is not exposed.

#### Defaults, partial configuration, and reset

For each target, compute effective resources independently from its
existing operator baseline. Do not materialize baseline defaults into
the stored custom resource. This is the proposed contract:

| Configuration | Effective result |
| --- | --- |
| Parent omitted or empty object | Existing target baseline, no new limits |
| Empty requests or limits map | No overrides from that map |
| Request for one resource only | Override that key; retain other baseline requests |
| Explicit limit without an explicit request for that resource | Use the limit as both effective request and limit |
| Limit for one key while another key keeps a baseline request | Effective request for the limited key equals the limit; baseline requests for other keys are untouched; reject if the resulting pair violates extended-resource equality |
| Explicit request and limit | Use both; reject request greater than limit |
| Explicit zero request | Keep zero where Kubernetes permits it; do not treat as omission |
| Remove an override key | Recompute that key using remaining explicit values and baseline |
| Remove a target, or the entire `spec.resources` field | Restore the complete baseline for each removed target |

Limit-only behavior intentionally mirrors Kubernetes request defaulting
from a limit in the effective workload, not in CR storage. The draft API
stores the explicit limit without an inferred request; the operator must
resolve it before changing workloads. It takes precedence over a baseline
request for that key, so a limit below the baseline does not generate
request greater than limit. It also increases reserved capacity when the
limit exceeds the baseline; this consequence must be documented to users.
CPU/memory requests without limits remain supported. Limits omitted for
other keys remain unset unless a separate admission policy changes the pod.

Define explicit null as equivalent to omission for these optional fields
and maps, using pruning/normalization where appropriate; verify the
generated schema and supported patch forms rather than assuming Go
pointers establish this behavior. Removing an explicit request while
retaining its limit invokes limit-only behavior, not unconditional reset
to the baseline. Map-key deletion and server-side-apply ownership must
follow the final schema and receive dedicated tests.

Defaults remain owned by the operator's release, not frozen forever to
the values in the baseline table. Explicit overrides remain authoritative
across supported upgrades; omitted keys follow that release's defaults.

#### Validation

Validate CREATE and UPDATE at the IngressController API boundary before
invalid settings can produce an invalid Deployment. Quantity parsing
alone is insufficient. Use the [Kubernetes resource-management
rules][kubernetes-resources] as the baseline. The validation contract must
include:

* Kubernetes resource-name syntax and resource-specific rules, including
  reserved names; do not interpret every slash-qualified name as a device.
* Quantity syntax, nonnegative values, and applicable precision and
  alignment rules for CPU, memory, ephemeral storage, and hugepages.
* Explicit request less than or equal to limit, with equality for
  non-overcommittable resources where Kubernetes requires it.
* Resource-specific hugepage requirements and equality rules rather than
  applying device-name rules to hugepage resources.
* No claims API: strict field validation rejects unknown fields such as
  `claims`; `fieldValidation=Ignore` can prune them rather than reject the
  object. Pruning does not mean the operator supports the supplied setting.
* Equivalent valid quantity spellings must retain their meaning through
  serialization.

#### Extended resources

Extended-resource (device) entries follow one set of rules, collected
here:

* Whole-number, nonnegative quantities.
* Requests equal to limits where Kubernetes requires equality for
  non-overcommittable resources.
* Limit-only allocation is accepted, with a matching effective request.
* Request-only device entries are rejected under this proposed contract.
* Admission of a syntactically valid device resource must not depend on
  whether any node currently advertises it. Device availability,
  placement, and capacity are scheduler concerns, not CR schema checks.

Risks and the test plan reference these rules; they are not restated
there.

Prefer structural schema and CEL validation supported by the target API
server versions, without adding a new admission webhook dependency.
Importing `ResourceRequirements` does not automatically give a CRD all
Pod admission validation. API review must establish how quantity comparison,
resource-name rules, effective defaulting, and semantic validation will
be implemented and tested. Any remaining parity gaps must be resolved
before this proposal is considered implementable.

#### Draft implementation status and unresolved validation

The draft CEL rules bound maps and quantities, compare explicit request/limit
pairs, and use slash-qualified keys for whole-number/equal-pair checks.
They do not establish full Kubernetes resource-name or resource-specific
validation. The [API integration tests][resource-tests] positively round-trip
`storage`, reserved names, `example.com/bad+name`, overlong keys,
`hugepages-invalid`, request-only hugepages, and misaligned hugepage pairs.
These are admission gaps, not supported workload configurations.

The API comments explicitly defer hugepage alignment and resolved-baseline
validation to the consumer. Before any workload change, the operator must
validate the complete effective configuration, report invalid or unsupported
settings through existing conditions, and retain the last valid workload.
If none exists, it must not create an invalid workload. It must not silently
round quantities; corrected supported settings must resume reconciliation.
These are required safeguards, not a claim that runtime support exists.

Full CREATE/UPDATE semantic validation specified above remains the desired
acceptance criterion and an unresolved API-review blocker. Consumer-side
safeguards do not fulfill or replace that admission requirement.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Where the Ingress Operator manages guest-cluster router pods, the API
targets those pods and their node/device capacity, not management-cluster
control-plane components. Confirm resource ownership, admission effects,
and rollout capacity for supported hosted deployments with their owners.
This proposal does not tune the management cluster's Ingress Operator.

#### Standalone Clusters

Settings apply per shard and per replica. Administrators must account for
requests, eligible nodes, and rollout surge; resources sufficient for the
steady-state replica count may not be sufficient for an update. Existing
replica count and placement APIs are not changed by this proposal.

#### Single-node Deployments or MicroShift

On a single-node cluster there is no second node to absorb rollout or
device demand. Restrictive limits or a stalled single-replica rollout can
interrupt all ingress for the affected shard. No high-availability or
automatic replica-adjustment guarantee is added. MicroShift applicability
and any corresponding configuration surface require confirmation with
its maintainers; this proposal does not introduce a MicroShift API.

#### OpenShift Kubernetes Engine

The proposal does not require an additional Gateway API implementation
or a new optional operator. Where the same IngressController-managed
router workload is provided, the same resource-targeting semantics are
intended. Confirm product-specific support and device-plugin availability
before claiming additional supported hardware or acceleration modes.

### Implementation Details/Notes/Constraints

A subsequent product implementation would:

1. Add the reviewed optional API fields, documentation, validation, and
   feature gate in `openshift/api`; regenerate CRD, OpenAPI, deepcopy,
   and other affected generated artifacts using that repository's tooling.
2. Add schema/create/update/defaulting tests, including qualified map
   keys and disabled-gate behavior, without depending on live hardware.
3. Consume the API in the Ingress Operator, compute effective resources
   independently, and assign them to the existing named targets. Update
   both ordinary and restartable-init-container reconciliation paths.
4. Test hashing, updates, canonical quantities, and reset behavior so
   equivalent desired resources do not cause repeated rollouts.
5. Document sizing, limit-only reservations, device capacity, rollout,
   troubleshooting, and the absence of implied crypto enablement.

The inspected Deployment logic already includes resource requirements in
container hashing and copying, including init-container updates. This
is evidence of existing consumers, not proof the new API is implemented
or that no reconciliation changes or tests will be needed.

Use pod-template rolling updates, not a promise of live resource resizing.
The operator must not drop requested devices, substitute resources, or
change another container's budget to make a rollout succeed. Manual edits
to the managed Deployment are not the supported configuration mechanism.

### Risks and Mitigations

* **Availability:** CPU limits can throttle traffic handling and route
  updates; memory limits can cause OOM termination. Preserve existing
  requests and no-limit defaults. Document user opt-in limits and recovery
  before enabling the feature for general use.
* **Scheduling:** extended resources cannot be overcommitted (see the
  extended-resources rules under Validation). Capacity,
  node placement, restartable-sidecar scheduling accounting, one-shot
  init-container requirements, and surge can stall rollout. Report the
  failure through existing Deployment/operator status and pod events,
  without silently relaxing configuration.
* **Admission and QoS:** ResourceQuota, LimitRange, and management-workload
  admission may reject or mutate pods. Compare CR, desired Deployment,
  and admitted pods separately. QoS is determined by the whole pod,
  including other containers; no Guaranteed QoS promise is made.
* **Security:** allocation can expose devices to the chosen container.
  Keep existing SCC, RBAC, service accounts, security contexts, and token
  isolation unchanged. Do not add host-device mounts or deploy a device
  plugin. Request networking, API, and device-security review. Allocation
  must not bypass compatibility, security, or FIPS eligibility checks.
* **Supportability:** [OpenShift conventions][resource-conventions]
  advise against limits on cluster components. User-configured limits
  are an explicit policy tradeoff requiring review, not new default
  limits or an assertion that arbitrary settings are safe.
* **Crypto expectations:** allocation alone neither activates QAT in
  HAProxy/OpenSSL nor proves a compatible cryptographic or compliance
  configuration. Any such integration requires a separate design.

Networking, API, hosted-control-plane, and security review is required
before implementation readiness; the reviewers listed in the frontmatter
must confirm that ownership.

### Drawbacks

Two similarly shaped targets under one field add API surface and support
obligations. Partial defaults plus limit-only behavior require careful
explanation. Allowing
limits increases the ways an administrator can disrupt a critical
workload. Generic resource maps require stronger validation than a fixed
CPU/memory structure and must evolve with supported Kubernetes semantics.

## Alternatives (Not Implemented)

* **Asymmetric pair:** `spec.resources` (HAProxy) alongside
  `spec.routerResources` is shorter, but `spec.resources` reads as a
  pod-wide or operator-wide setting; an admin setting it alone would
  leave the router container untouched.
* **Sibling names:** `spec.routerResources` and `spec.haproxyResources`
  keep the maps at the top level but scatter two similar-looking fields
  across the spec.
* **One map copied to both containers:** cannot express independent
  sizing and may reserve two scarce devices per replica. Reject it.
* **CPU/memory-only fields:** simpler schema, but cannot express named
  resources such as `qat.intel.com/cy`. Reject this restriction.
* **Generic container overrides or pod budgets:** expand the supported
  surface to unrelated containers or fail to express container-level
  device allocation. They are not the proposed scope.
* **Unmanaged Deployment edits or unsupported overrides:** lack a stable,
  validated contract and reconciliation guarantees. They are not a
  replacement for this API.

## Open Questions

1. Review the draft `spec.resources.router`/`haproxy` focused type, eight-entry
   maps, 64-character quantity bound, and preview gate choices; confirm null
   handling and map ownership rather than reopening representation as undecided.
2. Resolve the full-admission-validation blocker, including resource names,
   hugepages, and effective-baseline checks, without substituting consumer
   safeguards for the desired API contract. Approve effective limit-only
   resolution without persisted requests and test the operator safeguards.
3. Reconcile this EP's no-silent-loss requirement with the API comment that
   disabling the gate may prune fields and reset overrides. Existing tests
   cover schema filtering, not preservation of previously configured objects
   across transitions. Preservation policy and transition tests remain unresolved.
4. Resolve user opt-in limits, supported preview platforms, and hosted or
   managed-service constraints. Confirm whether support covers both default
   and non-default IngressControllers; API schema acceptance alone does not
   establish runtime support.

## Test Plan

The current [API tests][resource-tests] cover explicit storage/round trips,
map/quantity bounds, explicit-pair CEL rejection, unknown-field strict
rejection versus Ignore pruning, and preview-schema filtering. Their positive
invalid-name/hugepage cases document gaps; they are not full-validation or
operator-runtime acceptance evidence. Limit-only storage tests do not prove
effective workload defaulting, and schema filtering does not prove preservation.

The following remain future product acceptance tests:

* API schema and integration tests for CREATE/UPDATE, absent/empty/null
  inputs, partial maps, limit-only settings, explicit zero, removal,
  qualified resource keys, serialization, and feature gate behavior.
* Rejection tests for malformed/negative quantities, invalid resource
  names, request greater than limit, fractional or unequal device
  allocation, request-only devices, and hugepage violations, per Validation.
  Check claims as unknown fields under Strict and Ignore, not as a supported API.
* Operator safeguard tests for admitted-but-invalid names/hugepages and
  invalid effective baselines: preserve the last valid workload, create no
  invalid workload when none exists, report conditions, and recover after
  correction. Test configured-object gate transitions against the reviewed
  preservation policy.
* Operator unit tests for independent target resources, baseline defaults,
  ordinary and restartable-init-container paths, optional syslog isolation,
  resource hashing/copying, and quantity-equivalent idempotence.
* End-to-end tests for CPU/memory requests and limits on both targets,
  resetting overrides, multiple shards, placement, device exhaustion,
  rollout surge, and recovery. QAT allocation tests require prepared
  hardware and a device plugin; generic schema tests do not.
* Representative traffic and route-churn tests, including authorized
  isolated throttling/OOM tests; supported upgrades with both omitted
  and explicit settings; supported standalone, single-node, and hosted
  topologies. Cover external certificates, HAProxy version selection,
  thread tuning, admission policies, and optional logging regressions.

Use the project's required feature-gate and component test labels once
those names are assigned. Agree availability/performance criteria and
platform coverage before using results as graduation evidence.

For this documentation-only change, executable checks are Markdown
lint with the repository configuration, template-heading checks, metadata
validation, and full diff/content review. Discovery in `hack/find_changed.sh` uses committed history;
an uncommitted review patch needs explicit file-level heading/metadata
checks rather than treating a wrapper's no-files message as success.
Later patch delivery must also pass `git apply --check` on a pristine
checkout of its exact recorded base.

## Graduation Criteria

This is provisional and targets no release yet. Resolve open API and
metadata questions, obtain the required reviews, and agree feature-gate
and promotion policy before marking it implementable.

### Dev Preview -> Tech Preview

Demonstrate independent sizing of both workloads, validated generic
resource maps, baseline preservation, and observable rollout failure and
recovery. Provide end-user documentation and automated API/operator tests
with the required labels and agreed supported-platform coverage.

### Tech Preview -> GA

Require sustained CI evidence under the applicable feature-promotion
policy, supported upgrade/skew tests, scale and representative workload
tests, review of security and limit-related availability risks, and
documented sizing/support procedures. Publish user-facing documentation
in `openshift-docs`, including QAT allocation versus crypto enablement.

### Removing a deprecated feature

No feature removal is proposed. Any future removal or API replacement
must follow compatibility and deprecation policy; removing a gate or
field must not silently discard stored resource settings.

## Upgrade / Downgrade Strategy

Preserve existing behavior for objects without overrides. On a supported
upgrade, keep explicit resource values while omitted keys follow the new
operator baseline; verify no needless rollout when effective resources
are unchanged. Roll out the feature-gated API schema before relying on a
controller that consumes it, following the supported release lifecycle.

Before a supported transition that cannot honor the new settings, define
how operators detect and handle configured objects and how administrators
can reset them. Do not promise arbitrary OpenShift downgrades or migration
back to historical single-container router layouts. Gate-disable and
field-preservation behavior must be specified and tested before promotion.

## Version Skew Strategy

An old controller cannot be assumed to honor new fields. Test supported
API/controller skew and upgrade ordering, distinguishing stored API values
from resources actually reconciled into pods. The feature must not be
advertised as effective where the controller ignores its configuration.
CRD pruning/defaulting and feature-gate transitions must not silently
remove stored settings. Define supported combinations before implementation;
this draft does not assert compatibility for unsupported skew.

## Operational Aspects of API Extensions

This adds optional configuration fields, not a new API resource, webhook,
or per-request data-plane API call. Existing CRD availability is unchanged
if validation is implemented through the supported schema/CEL mechanisms.
The validation cost and schema compatibility need API integration tests.

The main availability risk is the resulting pod configuration: scheduling
failure, CPU throttling, OOM termination, or slower route propagation.
Observe IngressController/ClusterOperator conditions, Deployment progress
and available replicas, pod readiness/events/restarts, container resource
metrics, route-update behavior, and ingress traffic success and latency.
Use existing signals where available; this proposal invents no new metric,
condition reason, or automatic recovery guarantee.

Networking owns the proposed reconciliation path; API and platform owners
must review validation and admission/skew effects, and device specialists
must review allocation security.

## Support Procedures

Compare the stored IngressController, managed Deployment, and admitted
pods, inspecting both `containers` and `initContainers` and their status
lists. Useful existing read-only commands include:

```sh
oc get ingresscontroller -n openshift-ingress-operator <name> -o yaml
oc get deployment -n openshift-ingress router-<name> -o yaml
oc describe pod -n openshift-ingress <pod>
oc get clusteroperator ingress -o yaml
```

For Pending pods, inspect scheduler events, node placement, device-plugin
health and advertised resources, replica count, and surge capacity. For
unhealthy running pods, correlate CPU throttling, OOM/restarts, probe
failures, controller logs, and traffic or route-update symptoms. Check
namespace quota, LimitRange, and admission mutations separately.

After implementation, the supported recovery is to correct the relevant
IngressController overrides or remove them to restore defaults, then
observe reconciliation and rollout. This is not an immediately available
mutation command. A reset may itself need scheduling headroom or time
for existing connections to drain; it is not guaranteed instantaneous.
Do not delete the IngressController CRD, disable unrelated webhooks, or
edit managed Deployments as the routine reset mechanism. Valid settings
should reconcile normally once capacity or admission problems are fixed,
without the operator silently discarding the configured intent.

No TLS acceleration after successful device allocation is not by itself
a resource-configuration failure. Check the separately supported crypto
integration, if any.

## Infrastructure Needed

No new infrastructure is required to review this proposal. Future generic
API/controller tests need no accelerator hardware; future QAT allocation
and exhaustion tests require an existing compatible device plugin and
hardware capacity. Hardware qualification, crypto integration, CI ownership,
and managed-service rollout agreements are separate readiness work.

[router-deployment]: https://github.com/openshift/cluster-ingress-operator/blob/16b5a9ff93247e7d6914948b8a5e2ce35dfa5088/pkg/manifests/assets/router/deployment.yaml
[deployment-code]: https://github.com/openshift/cluster-ingress-operator/blob/16b5a9ff93247e7d6914948b8a5e2ce35dfa5088/pkg/operator/controller/ingress/deployment.go
[api-pr]: https://github.com/openshift/api/pull/3134
[ingress-api]: https://github.com/openshift/api/blob/61663703a76509cfeedff89f90b5b2f7d0c8e134/operator/v1/types_ingresscontroller.go
[resource-feature]: https://github.com/openshift/api/blob/61663703a76509cfeedff89f90b5b2f7d0c8e134/features/features.go
[resource-tests]: https://github.com/openshift/api/blob/61663703a76509cfeedff89f90b5b2f7d0c8e134/tests/ingresscontroller_resources_test.go
[haproxy-launcher]: https://github.com/openshift/router/blob/12074b829811275700c93259973041f286beb0fe/images/router/haproxy/start-haproxy
[router-image]: https://github.com/openshift/router/blob/12074b829811275700c93259973041f286beb0fe/images/router/base/Dockerfile.ocp
[envoy-qat]: https://gateway.envoyproxy.io/docs/tasks/security/private-key-provider/
[kubernetes-resources]: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
[resource-conventions]: ../../CONVENTIONS.md#resources-and-limits
