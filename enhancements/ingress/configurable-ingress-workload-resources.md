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
last-updated: 2026-11-20
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
an `IngressController`. Each target accepts a list of named resource
entries with optional request and limit quantities, covering CPU, memory,
ephemeral storage, hugepages, and extended resources such as
`qat.intel.com/cy`. Omission retains the operator's existing defaults
and does not introduce default limits. Setting the field is opt-in and
explicitly unsupported: the platform defaults remain the supported
configuration, and the administrator is responsible for choosing values
that suit the cluster. A scheduling failure that those values explain is
attributed to them through the `IngressController`'s `Degraded`
condition, a warning event, a metric, and an alert. This proposal
concerns resources of the router
pods, not resources of the Ingress Operator, and allocating a device does
not enable cryptographic acceleration.

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
  resource overrides through supported configuration and through status
  conditions, events, metrics, and alerts, so that a mis-sized limit can be
  detected and remediated
  without editing operator-managed Deployments.
* As a cluster administrator, I want to remove overrides and return to
  supported defaults, and to understand the scheduling and availability
  consequences before changing them.

### Goals

* Support requests and limits for both existing workload containers,
  independently, per `IngressController` and per replica.
* Support valid named Kubernetes resource entries rather than an
  allowlist containing only CPU and memory.
* Define partial configuration, reset, validation, and reconciliation
  behavior without changing omitted/default behavior.
* Define ownership of administrator-chosen values: configured values are
  unsupported, tailoring them to the cluster is the administrator's duty,
  and the result stays visible through status conditions, events, a metric,
  and an alert.
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

Each target is a list of entries keyed on `name`, where each entry has
an optional `request` and `limit`; names and quantities are validated by
the schema and CEL rules described below. Neither target is a pod-wide
total or a value copied to every container; `spec.resources` requires at
least one target (`MinProperties=1`), so an empty object is rejected
rather than treated as omitted. Either list can contain valid named extended
resources; their use does not imply that the selected application can
consume the allocation.

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
      - name: cpu
        request: "250m"
        limit: "1"
      - name: memory
        request: "384Mi"
        limit: "768Mi"
      - name: qat.intel.com/cy
        request: "1"
        limit: "1"
    router:
      - name: cpu
        request: "50m"
        limit: "200m"
      - name: memory
        request: "96Mi"
        limit: "192Mi"
```

Only HAProxy receives the QAT allocation in this example. Requests apply
per replica; this is not a shared pool across an IngressController's pods.
The [Envoy Gateway private-key-provider guide][envoy-qat] illustrates
`qat.intel.com/cy: '1'` in requests and limits, but separately requires
hardware, a device plugin, and a crypto provider.

The [API source][ingress-api] defines `IngressControllerResources` with
`router` and `haproxy` lists of `IngressControllerContainerResource`
entries, each with a required `name` and optional `request` and `limit`
quantities. Each list requires 1 to 8 entries keyed on `name`, and the
parent object requires at least one target. Names are restricted to
`cpu`, `memory`, `ephemeral-storage`, `hugepages-<size>`, or a
domain-qualified extended resource; reserved `kubernetes.io` domains and
the `requests.` prefix are rejected. Quantities must be positive,
non-zero, and at most 20 characters. The
[feature registration][resource-feature] enables
`IngressControllerResourceConfiguration` in `DevPreviewNoUpgrade` only,
with `bentito` as the contact; `claims` is not exposed.

The field documentation states the support boundary, and the proposal
adopts its wording:

* `spec.resources`: "The values set here are not validated against what the
  workload needs; setting them too low or too high can make the router
  unavailable, and Red Hat does not support issues caused by such values. See
  the request and limit fields for the specific risks of each. The cluster
  administrator is responsible for choosing appropriate values."
* `request`: "Issues caused by an inappropriate value are not supported, and
  the cluster administrator is responsible for choosing it; the platform
  default described on spec.resources is the supported configuration." The
  value is static: the platform never adjusts it as route count,
  configuration size or traffic grows.
* `limit`: setting a limit on a cluster component such as the router "is
  discouraged, the platform does not validate it against what the container
  needs, and the cluster administrator accepts responsibility for any
  unavailability it causes"; the platform default of no limit is the
  supported configuration.

Because this is schema documentation, `oc explain
ingresscontroller.spec.resources` shows it to administrators who enable
the gate.

#### Defaults, partial configuration, and reset

For each target, compute effective resources independently from its
existing operator baseline. Do not materialize baseline defaults into
the stored custom resource. This is the proposed contract:

| Configuration | Effective result |
| --- | --- |
| `spec.resources` omitted | Existing baselines for both targets |
| `spec.resources` present but empty | Rejected: at least one target is required |
| Target omitted | Existing baseline for that target |
| Target present with an empty list | Rejected: 1 to 8 entries are required |
| Entry with a request only | Override that request; limit remains unset (extended resources and hugepages require a limit) |
| Entry with a limit only | Use the limit as both effective request and limit |
| Entry with a request and limit | Use both; reject request greater than limit |
| Zero or negative quantity | Rejected |
| Remove an entry | Restore the baseline for that resource |
| Remove a target, or the entire `spec.resources` field | Restore the complete baseline for each removed target |

Limit-only behavior intentionally mirrors Kubernetes request defaulting
from a limit in the effective workload, not in CR storage. The API
stores the explicit limit without an inferred request; the operator must
resolve it before changing workloads, and the resolved request is visible
on the managed Deployment, not in status. It takes precedence over a baseline
request for that entry, so a limit below the baseline does not generate
request greater than limit. It also increases reserved capacity when the
limit exceeds the baseline; this consequence must be documented to users.
CPU/memory requests without limits remain supported. Limits omitted for
other entries remain unset unless a LimitRange in the router namespace
applies one.

Define an explicit null for an optional `request` or `limit` field as
equivalent to omission, using pruning/normalization where appropriate;
verify the generated schema and supported patch forms rather than
assuming Go pointers establish this behavior. This rule does not extend
to list elements: the lists are keyed on `name`, a null element has no
key, and Kubernetes does not prune null elements from arrays, so null
list elements must be rejected. Remove an entry by its `name` or by
replacing the list. Removing an explicit request while retaining its
limit invokes limit-only behavior, not unconditional reset to the
baseline. List-entry (`name`) deletion and server-side-apply ownership
must follow the final schema and receive dedicated tests.

Defaults remain owned by the operator's release, not frozen forever to
the values in the baseline table. Explicit overrides remain authoritative
across supported upgrades; omitted entries follow that release's defaults.

#### Validation

Validate CREATE and UPDATE at the IngressController API boundary before
invalid settings can produce an invalid Deployment. Quantity parsing
alone is insufficient. Use the [Kubernetes resource-management
rules][kubernetes-resources] as the baseline. The validation contract must
include:

* Kubernetes resource-name syntax and resource-specific rules, including
  the supported-resource allowlist and reserved names; do not interpret
  every slash-qualified name as a device.
* Quantity syntax, positive non-zero values of at most 20 characters,
  and applicable precision and alignment rules. CPU quantities below
  `1m` are accepted and rounded up by Kubernetes when the pod is
  admitted. Extended resources and hugepages must be whole numbers, and
  hugepages whole multiples of the page size; these checks use the
  quantity's integer representation, so an integral spelling such as
  `1536Mi` is required over an equivalent such as `1.5Gi`.
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

* Whole-number, positive quantities.
* Requests equal to limits where Kubernetes requires equality for
  non-overcommittable resources.
* Limit-only allocation is accepted, with a matching effective request.
* Request-only device entries are rejected under this proposed contract.
* Admission of a syntactically valid device resource must not depend on
  whether any node currently advertises it. Device availability,
  placement, and capacity are scheduler concerns, not CR schema checks.

Risks and the test plan reference these rules; they are not restated
there.

The rules are implemented with structural schema and CEL validation
supported by the target API server versions, without adding a new
admission webhook dependency. Importing `ResourceRequirements` does not
automatically give a CRD the equivalent of Pod admission validation, so
the rules are expressed explicitly.

#### Validation implementation status

The API enforces the rules above in the CRD schema and CEL: list keying
on `name`, 1 to 8 entries per target, at least one of `request` or
`limit` per entry, qualified-name syntax with the supported-resource
allowlist and reserved-domain rejection, positive non-zero quantities of
at most 20 characters, `request` less than or equal to `limit`, and the
extended-resource and hugepage whole-number, equality, and page-size
multiple rules. The whole-number checks test the quantity's integer
representation, so a page-aligned `limit: 1.5Gi` is rejected for
`hugepages-2Mi` while the equivalent `1536Mi` is accepted, and a
sub-milliCPU `cpu: 0.5m` is accepted and rounded up by Kubernetes at pod
admission. Tests must cover both hugepage spellings and `0.5m`. The
[API tests][resource-tests] are a YAML testsuite that
covers these valid and invalid cases at the schema boundary.

The API still defers the effective-baseline check to the consumer:
hugepage alignment to the declared page size is checked, but whether the
resolved page size is supported on the node, and whether the effective
configuration is usable for the workload, are not. Before any workload
change, the operator must validate the structural result of its own
resolution — for example a hugepage size that no eligible node advertises
— and retain the last valid workload; if none exists, it must not create
an invalid workload. It must not silently round quantities, and corrected
supported settings must resume reconciliation. This is a safeguard for
structural validity, not a judgement of fitness: the operator does not and
must not decide whether a value is appropriate for the workload, and it
enforces no minimum, because the requirement depends on route count,
configuration size, TLS, compression and traffic. Those choices belong to
the cluster administrator, as the field documentation states. These
safeguards are required, but they are not a claim that runtime support
exists.

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
2. Add schema/create/update/defaulting tests, including qualified resource
   names and disabled-gate behavior, without depending on live hardware.
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
  failure through Deployment/operator status, pod events, and the
  attribution signals described under Support boundary, without silently
  relaxing configuration.
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
  advise against limits on cluster components, and this proposal adds no
  default limits. The feature is nonetheless exposed as an explicitly
  unsupported opt-in: configured values are not supported, and the
  administrator owns them. Ownership is defined in the next subsection.
* **Crypto expectations:** allocation alone neither activates QAT in
  HAProxy/OpenSSL nor proves a compatible cryptographic or compliance
  configuration. Any such integration requires a separate design.

#### Support boundary for administrator-chosen values

Administrators already resize router pods by editing the managed
Deployment or by other unsupported means, and the operator reverts those
edits without recording the intent. This API replaces that invisible
workaround with a stored and validated value. It changes no
default, and the field is opt-in and confined to `DevPreviewNoUpgrade`,
so a cluster that does not set `spec.resources` behaves as it does today.

The supported configuration is to leave `spec.resources` unset: the
platform defaults described on that field are what Red Hat supports.
Any change to the field is unsupported, and the field documentation says
so in the terms quoted above. Ownership is split, and the API field
documentation states the split:

* The operator owns validation at the API boundary, propagation to both
  container paths, the per-release defaults, not creating an invalid
  workload, and surfacing the result through the signals described below.
* The cluster administrator owns the chosen values and the capacity they
  assume. A validly stored but unsuitable request or limit is an
  administrator error rather than an operator defect, and it is outside
  Red Hat support.

The administrator therefore has to size the values for the cluster the
IngressController runs on: node capacity, replica count, rollout surge
headroom, route count and configuration size, and expected traffic. The
values are static — the platform never re-evaluates them — so a
configuration that fits today can stop fitting as the cluster grows, and
revisiting it is the administrator's responsibility. No minimum is
enforced, because the requirement is workload-specific.

The project is therefore not responsible for a value that makes routes
fail, but the result stays visible and recoverable. The operator must not
suppress, round, or silently revert a configured value. A scheduling
failure that an override can explain is attributed to it through the
signals described in the next subsection. Failures the platform cannot
attribute remain visible through existing signals: unavailable replicas,
unschedulable pods, or a stalled rollout appear in Deployment status and
in the `IngressController` and `ingress` `ClusterOperator` conditions;
CPU throttling and OOM restarts appear in pod status, pod events, restart
counts, and container metrics, and the field documentation warns about
them. Correcting or removing the override is the recovery path, and the
condition message and gauge clear through normal reconciliation. No new
condition type, no new condition reason, and no new API status field is
introduced, and no restart heuristic is added.

Networking, API, hosted-control-plane, and security review is required
before implementation readiness; the reviewers listed in the frontmatter
must confirm that ownership.

#### Signals for resource misconfiguration

Because the platform does not judge whether a configured value fits the
cluster, the operator attributes the scheduling failures it can detect to
`spec.resources`. When `spec.resources` overrides at least one resource
and a pod of the current Deployment revision is pending with a
`PodScheduled=False` condition whose reason is `Unschedulable` and whose
message reports an `Insufficient` quantity for an overridden resource
name, the operator reports that pod. Pods from an earlier revision and
shortages of a resource that `spec.resources` does not set are ignored,
so an unrelated scheduling failure is not blamed on the override. The
operator adds no new condition type; it reuses existing signals and adds
one metric and one alert:

| Signal | Behavior |
| --- | --- |
| `Degraded` condition on the `IngressController` | When it is already `True`, a sentence is appended naming `spec.resources` as a possible cause and pointing at reducing the values or adding capacity. The condition type and reason are unchanged; existing deployment conditions already make an unavailable router `Degraded`. |
| Warning event `ResourceConfigurationUnschedulable` | Recorded on the `IngressController` with the pending pod and the scheduler message, only when the status actually changes, so a persistent problem does not emit an event on every status sync. |
| Gauge `ingress_controller_resource_configuration_unschedulable` | `1` while such a pod exists and `0` otherwise, labeled with the `IngressController` name. If listing pods fails, the last value is kept rather than published as `0`, so a transient error cannot look like recovery; the metric is deleted when the `IngressController` is deleted. |
| Alert `IngressControllerResourceConfigurationUnschedulable` | Fires at `warning` severity when the gauge has been `1` for 15 minutes. Its description says the values are administrator-chosen and not validated against capacity, matching the support boundary, and it does not claim that the override caused the shortage. |

The pod watch enqueues the owning `IngressController` when a pod becomes
unschedulable or its scheduling message changes, so a scheduler message
that changes from a taint failure to `Insufficient cpu` refreshes the
diagnosis without the pod leaving the unschedulable state.

These signals detect only scheduling failures caused by an overridden
resource. CPU throttling, OOM termination, and a limit that is too low for
the traffic served remain visible through pod status, pod events, restart
counts, container metrics, and the existing conditions, as the risks above
describe. Attribution is a diagnosis aid, not a fitness judgement: a
pod that fits but performs badly is not reported here, and the
administrator still owns the value.

### Drawbacks

Two similarly shaped targets under one field add API surface and support
obligations. Partial defaults plus limit-only behavior require careful
explanation. Allowing
limits increases the ways an administrator can disrupt a critical
workload. Generic resource entries require stronger validation than a fixed
CPU/memory structure and must evolve with supported Kubernetes semantics.

## Alternatives (Not Implemented)

* **Asymmetric pair:** `spec.resources` (HAProxy) alongside
  `spec.routerResources` is shorter, but `spec.resources` reads as a
  pod-wide or operator-wide setting; an admin setting it alone would
  leave the router container untouched.
* **Sibling names:** `spec.routerResources` and `spec.haproxyResources`
  keep the lists at the top level but scatter two similar-looking fields
  across the spec.
* **One list copied to both containers:** cannot express independent
  sizing and may reserve two scarce devices per replica. Reject it.
* **CPU/memory-only fields:** simpler schema, but cannot express named
  resources such as `qat.intel.com/cy`. Reject this restriction.
* **Generic container overrides or pod budgets:** expand the supported
  surface to unrelated containers or fail to express container-level
  device allocation. They are not the proposed scope.
* **Unmanaged Deployment edits or unsupported overrides:** lack a stable,
  validated contract and reconciliation guarantees. They are not a
  replacement for this API.
* **Existing signals only:** relying on `Degraded` and pod events without
  an attributed message, metric, or alert was considered. Rejected because
  the existing `Degraded` condition does not distinguish an
  administrator-chosen value from a platform or capacity defect, and a
  gauge lets the alert require a sustained, attributable failure instead of
  firing on every transient scheduling error.
* **Distinct `Degraded` reason instead of a metric and alert:** raised in
  review. It would add a new condition reason and a larger redesign of how
  `Degraded` is computed; it remains a reasonable follow-up rather than
  part of this change.
* **No resource overrides:** keep the API surface minimal and size nodes
  instead. Rejected: it leaves the unsupported Deployment-edit workaround
  in place, which the operator silently reverts, gives the operator no
  record of the intended values to validate or report, and cannot raise
  the router's own requests for scheduling or device allocation.

## Open Questions

1. Do HyperShift-hosted router pods and managed services support
   `spec.resources` in the Dev Preview? Schema acceptance alone does not
   establish runtime support. Owner: networking and hosted-control-plane
   reviewers.

## Test Plan

The current [API tests][resource-tests] are a YAML testsuite for
`IngressControllerResourceConfiguration`. They cover request/limit round
trips, list and quantity bounds including 20- and 21-character
boundaries, limit-only storage, integer and string quantity forms,
duplicate and oversized lists, invalid and unsupported names, reserved
domains and the `requests.` prefix, request greater than limit, and
extended-resource and hugepage rules. They do not prove effective
workload defaulting or operator runtime behavior.

The following remain future product acceptance tests:

* API schema and integration tests for CREATE/UPDATE, absent/empty/null
  inputs, partial entries, limit-only settings, entry removal,
  qualified resource names, serialization, and feature gate behavior.
* Rejection tests for malformed, zero, and negative quantities, invalid
  resource names, request greater than limit, fractional or unequal device
  allocation, request-only devices, and hugepage violations, per Validation.
  Check claims as unknown fields under Strict and Ignore, not as a supported API.
* Operator safeguard tests for admitted-but-invalid names/hugepages and
  invalid effective baselines: preserve the last valid workload, create no
  invalid workload when none exists, and recover after correction. Include
  `Degraded` on unschedulable pods and a stalled rollout. Cover the
  resource-unschedulable signals: only current-revision pods and
  `Insufficient` messages for an overridden resource count; the `Degraded`
  message names `spec.resources`; the event is recorded only when status
  changes; and the gauge reports `1` and `0`, keeps its last value when the
  pod list fails, and is removed with the `IngressController`.
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
resource entries, baseline preservation, and observable rollout failure and
recovery, including the attributed `Degraded` message, event, metric, and
alert. Provide end-user documentation and automated API/operator tests
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
back to historical single-container router layouts. No supported transition
removes a stored value: `DevPreviewNoUpgrade` cannot be left once enabled,
and `CustomNoUpgrade` keeps feature-gated fields in the schema, so only a
future field removal could discard values, and that must follow the
deprecation policy.

## Version Skew Strategy

An old controller cannot be assumed to honor new fields. Test supported
API/controller skew and upgrade ordering, distinguishing stored API values
from resources actually reconciled into pods. The feature must not be
advertised as effective where the controller ignores its configuration.
CRD pruning and defaulting must not silently remove stored settings. Define supported combinations before implementation;
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
metrics, the `ingress_controller_resource_configuration_unschedulable`
gauge and the `IngressControllerResourceConfigurationUnschedulable` alert,
route-update behavior, and ingress traffic success and latency.
Resource-caused failures that the existing deployment conditions observe
surface as `Degraded` on the IngressController and the `ingress`
ClusterOperator. The metric and alert add attribution for scheduling
failures of an overridden resource, and the alert is warning-level; this
proposal adds no new condition reason, restart heuristic, or automatic
recovery guarantee.

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

For Pending pods, first check whether the platform has already attributed
the failure to `spec.resources`, then inspect scheduler events, node
placement, device-plugin health and advertised resources, replica count,
and surge capacity:

```sh
oc describe ingresscontroller -n openshift-ingress-operator <name>
oc get events -n openshift-ingress-operator --field-selector reason=ResourceConfigurationUnschedulable
```

The `Degraded` condition carries the appended `spec.resources` sentence,
and the event carries the pending pod and the scheduler message. The
corresponding gauge is queried in the platform monitoring stack as
`ingress_controller_resource_configuration_unschedulable{name="<name>"} == 1`.
For unhealthy running pods, correlate CPU throttling, OOM/restarts, probe
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
[ingress-api]: https://github.com/openshift/api/blob/8b8462eddd2d8d0ddd4c4b73cf0bd0bfdecfcf07/operator/v1/types_ingresscontroller.go
[resource-feature]: https://github.com/openshift/api/blob/8b8462eddd2d8d0ddd4c4b73cf0bd0bfdecfcf07/features/features.go
[resource-tests]: https://github.com/openshift/api/blob/8b8462eddd2d8d0ddd4c4b73cf0bd0bfdecfcf07/operator/v1/tests/ingresscontrollers.operator.openshift.io/IngressControllerResourceConfiguration.yaml
[haproxy-launcher]: https://github.com/openshift/router/blob/12074b829811275700c93259973041f286beb0fe/images/router/haproxy/start-haproxy
[router-image]: https://github.com/openshift/router/blob/12074b829811275700c93259973041f286beb0fe/images/router/base/Dockerfile.ocp
[envoy-qat]: https://gateway.envoyproxy.io/docs/tasks/security/private-key-provider/
[kubernetes-resources]: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
[resource-conventions]: ../../CONVENTIONS.md#resources-and-limits
