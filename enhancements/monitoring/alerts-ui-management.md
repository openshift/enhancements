---
title: alerts-ui-managment
authors:
  - "@sradco"
reviewers:
  - "@jan--f"
  - "@jgbernalp"
approvers:
  - "@jan--f"
  - "@jgbernalp"
api-approvers:
  - TBD
creation-date: 2025-07-24
last-updated: 2025-07-30
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
---
# New APIs to support Alerts UI Management in OpenShift

## Summary
Provide a user‑friendly UI and REST API for defining, viewing, editing, disabling/enabling and silencing Prometheus alerts without manual YAML edits, reducing alert fatigue and improving operational efficiency.
Platform alerts will be overriden leveraging the  `AlertRelabelConfig` CR (in `cluster-monitoring-config`) rather than editing their original AlertingRules in the `PrometheusRule`.

## Motivation
- While it's possible to customize built-in alerting rules with the `AlertingRule` + `AlertRelabelConfig` CRDs, the process is cumbersome and error-prone. It requires creating YAML manifests manually and there's no practical way to verify the correctness of the configuration.
- Some operational teams prefer an interactive console and API to manage alerts safely, guided by best practices.
- A unified interface will help users create, clone, disable or silence alerts, view real‑time firing status and preserve changes across upgrades.

### User Stories

1. **Bulk disable during maintenance**
   As a cluster administrator, I want to select and disable multiple alerts in one action,
   so that I can suppress non‑critical notifications and reduce noise.

2. **Single‑alert toggle from alert details**
   As a cluster administrator, I want to disable an out-of-the-box alerting rule permanently, because it's not relevant for my environment (for instance, I don't want to see any alert about resource over-commitment because my cluster is only used for testing).

3. **Replace an existing platform alerting rule by a custom definition.**
   As a cluster admin, I want to disable existing built‑in alert, clone it and adjust its threshold and severity, and save it as a user‑defined rule, so that I can tailor default monitoring to suit my team’s SLAs without modifying upstream operators.

4. **Create alert a new custom alert**
   As a cluster admin/developer, I want to write or paste a PromQL expression in a “Create Alert” form, specify duration and severity, and save it, so that I can turn any ad‑hoc query I used during troubleshooting into a managed alert.

5. **Clone an alert base on platform or user-defined alerting rule**
   As a cluster admin/developer, I want to clone an alert and update it based on my organization needs.

7. **View silence status on active alerts** - Exists today. Keep Functionality as is.
   As a cluster admin, I want the “Active Alerts” list to show which alerts are currently silenced (and until when),
   so that I have clear visibility into which notifications are suppressed and why.

8. **Create an alerting rule from the Metrics > Observe page** - Currently not in scope
   AS a cluster admin I would like to be able to create an alert directly from the Metrics > Observe page,
   after I used it to tune the expression that I need.

## Goals
1. CRUD operations on user‑defined alerts via UI and API.
2. Clone platform or user alerts to customize thresholds or scopes.
3. Disable/enable alerts by creating/updating entries in the `AlertRelabelConfig` CR.
4. Create/manage silences in Alertmanager and reflect this in the UI. - Already exists today.
5. Aggregate view of all alerting rules, showing definitions plus relabel context.
6. Aggregate view of all alerts, showing status (Pending, Firing, Silenced) and relabel context.
7. Enforce multi‑tenancy: restrict queries to user’s namespace or cluster scope.
8. Persist user changes through operator upgrades, without modifying existing operators or CRDs.
9. Stay fully GitOps/Argo CD compliant: managed resources must remain commit‑able and, when owned by a GitOps application, appear read‑only.

## Non‑Goals
- Deep RBAC beyond native Kubernetes permissions.
- Operators reacting to user modifications (operator code remains unchanged).
- Full multi‑cluster federation (initial focus is single‑cluster).

## Related Enhancement Proposals
- https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/alert-overrides.md
- https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/user-workload-monitoring.md

## Proposal

### 1. API Endpoints

Base path: `/api/v1/alerting`

Rule identity

- A rule is uniquely identified by the tuple: `(namespace, prometheusrule, ruleName, severity)`.
- Canonical opaque identifier used below as `ruleId`:
  - `/namespaces/{namespace}/prometheusrules/{prometheusrule}/rules/{ruleName}/severities/{severity}`.

**Alerts (instances)**

| Method | Path                      | Description                                                                                         | Notes |
|--------|---------------------------|-----------------------------------------------------------------------------------------------------|-------|
| GET    | `/alerts`                 | List alert instances (Pending / Firing / Silenced) with post‑relabel labels. Supports the filter parameters listed below. | **Required** — merges `AlertRelabelConfig` with platform alerts. |

Filter parameters for `/alerts`:

- `name`
- `group`
- `component`
- `severity`
- `state` (one of: `firing`, `pending`, `silenced`)
- `source` (platform, user-defined)
- `notification_receiver`
- `triggered_since`
- Other alert labels (arbitrary `labelKey=labelValue` pairs)

**Rules (definitions)**

| Method | Path                                                  | Description                                                                                              | Notes |
|--------|-------------------------------------------------------|----------------------------------------------------------------------------------------------------------|-------|
| GET    | `/rules`                           | List all alert rule definitions (platform and user), including `disabled` flags and relabel context. Supports the filter parameters listed below. | **Required** — merges `AlertRelabelConfig` with platform alerts.|
| GET    | `/rules/labels`                    | List common labels across selected rules. | |
| POST   | `/rules`                           | Create a new user‑defined alert rule. | **Required** — Wraps K8s API for `PrometheusRule`. |
| GET    | `/rules/{ruleId}`                  | Fetch a single rule’s full definition and relabel context. | |
| PATCH  | `/rules/{ruleId}`                  | Update a single rule (platform or user‑defined). | |
| DELETE | `/rules/{ruleId}`                  | Delete a user‑defined rule. | |
| GET    | `/rules/{ruleId}/labels`           | List labels (including relabel context) for the specified rule. | |
| POST   | `/rules`                           | Bulk update rules (e.g., severity/status/labels).  | |
| DELETE | `/rules`                           | Bulk delete user‑defined  rules. | |

Filter parameters for `/rules`:

- `name`
- `group`
- `namespace`
- `component`
- `severity`
- `state` (one of: `enabled`, `disabled`, `silenced`)
- `source`
- `notification_receiver`
- Other alert labels (arbitrary `labelKey=labelValue` pairs)

**Health**

| Method | Path        | Description                 | Notes |
|--------|-------------|-----------------------------|-------|
| GET    | `/health`   | Basic health‑check endpoint |       |

### 2. UI Components
- **Management List**: show all alerting rules; filter/sort by name, severity, namespace, status, labels; saved searches
- **Create/Edit Alert Form**: fields for Alert Name, Summary, Description, Duration, Severity, Labels, Annotations (runbook links), Group & Component labels
- **Create/Edit Alert labels Form**: list common labels, Add/remove alert labels.
- **Alerts View**: show current firing/pending instances, silence status, relabel context
- **Silencing Panel**: define matchers, duration, comment - Keep

### 3. Data Model
- Store user alerts in a dedicated `PrometheusRule` under `openshift-user-workload-monitoring`
- Update platform alerts by writing relabel rules into the `AlertRelabelConfig` CR in `cluster-monitoring-config`
- Use labels for disable/enable flags; rely on existing CRDs
- Validate PromQL expressions and enforce namespace/cluster scope

### 4. Migration
- Automated tool to detect existing `PrometheusRule` CRDs and import them into the new system
- Preserve existing labels, annotations and support rollback on errors

### 5. GitOps / Argo CD Compliance
- All generated `PrometheusRule` and `AlertRelabelConfig` resources remain declarative and suitable for committing to Git
- If a resource is owned by an Argo CD/GitOps application (via ownerReferences or annotations), the UI/API surfaces it as read‑only
- Prevent out‑of‑band edits by ensuring all changes go through the same reconciliation loop

### 6. Workflow Description

## Risks & Mitigations
- **Performance**: split large rule sets into multiple CRs; benchmark reconciliation latency
- **Misconfiguration**: validate syntax; prevent disabling protected base alerts via exclusion list
- **Complexity**: sensible defaults; UI guidance; limit advanced options in MVP

## Graduation Criteria
- **Tech Preview**: basic CRUD, clone, disable/enable, silencing via UI/API; migration tool v1
- **GA**: best‑practice guidance in UI; multi‑namespace filtering; full test coverage; complete documentation

## Open Questions
1. **Per‑Alert vs. Single‑File**: Should each user‑defined alert reside in its own `PrometheusRule` file, or group all into one? A customer noted per‑alert files may simplify GitOps/Argo CD maintenance—does that hold true at scale?
2. **Read‑Only Detection**: Which annotations, labels or ownerReferences reliably indicate GitOps‑managed resources to render them read‑only in our UI?
3. **Concurrent Operator Updates**: How should we handle cases where upstream operators update their own `PrometheusRule` CRs—should we reconcile `AlertRelabelConfig` entries periodically?
4. **Multi‑Cluster Extension**: What API or data‑model changes will be needed to support an ACM‑driven, multi‑cluster alerts view in future phases?
5. **Change History**: What should we provide for alerts change history?
6. Do we need a dedicated API for updating labels in Bulk?

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
