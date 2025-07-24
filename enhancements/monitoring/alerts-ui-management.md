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
# Managing Alerts in the OpenShift Console

## Summary
Improve alert management in the OCP console to expose a unified view of the alerting rules (and associated alerts), provide flexible filtering capabilities and allow rules management without manual YAML edits.


A WIP reference backend/library implementation and demo are available at [machadovilaca/alerts-ui-management](https://github.com/machadovilaca/alerts-ui-management).

## Motivation
- While it's possible to customize built-in alerting rules, as described in the existing [alert overrides](https://github.com/openshift/enhancements/blob/
master/enhancements/monitoring/alert-overrides.md) proposal, with the `PrometheusRule` and `AlertRelabelConfig` CRDs, the process is cumbersome and error-prone. It requires creating YAML manifests manually and there's no practical way to verify the correctness of the configuration. Built-in alerting rules and alerts are still visible in the OCP console after they've been overridden.
- Some operational teams prefer an interactive console and API to manage alerts.
- A unified interface will help users create, clone, disable or silence alerts, view real‑time firing status and preserve changes across upgrades.

### Problem Statement
Administrators currently struggle with generic and unactionable alerts, which leads to excessive "noise" and insufficient detail. This issue stems from underlying logs that often do not provide enough information to take immediate action, leading to reduced efficiency, delayed problem resolution, and decreased user satisfaction. Customers have a strong desire for "smart" or de-duplicated alerts that provide detailed, actionable insights.

### User Stories

1. **Bulk disable Platform alerts that are not required**
   As a cluster administrator, I want to select and disable multiple Platform alerts in one action,
   so that I can permanently suppress non‑critical notifications and reduce noise.

2. **Disable a built-in alerting rule**
   As a cluster administrator, I want to disable an out-of-the-box alerting rule permanently, because it's not relevant for my environment (for instance, I don't want to see any alert about resource over-commitment because my cluster is only used for testing).

3. **Replace an existing platform alerting rule by a custom definition.**
   As a cluster admin, I want to disable existing built‑in alert, clone it and adjust its threshold and severity, and save it as a user‑defined rule, so that I can tailor default monitoring to suit my team’s SLAs without modifying upstream operators.

4. **Create a custom alerting rule**
   As a cluster admin/developer, I want to create an alerting rule using a form which allows me to specify mandatory fields (PromQL expression, name) and recommended fields (for duration, well-known labels/annotations such as severity, summary).

5. ***Clone an alert based on an existing alerting rule (platform or user-defined)**
   As a cluster admin/developer, I want to clone an alert and update it based on my organization needs.

7. **View silence status on active alerts** - Exists today. Keep Functionality as is.
   As a cluster admin, I want the “Active Alerts” list to show which alerts are currently silenced (and until when),
   so that I have clear visibility into which notifications are suppressed and why.

8. **Create an alerting rule from the Metrics > Observe page** - Currently not in scope
   AS a cluster admin I would like to be able to create an alert directly from the Metrics > Observe page,
   after I used it to tune the expression that I need.

## Goals
1. Add a Console UI for managing alerting rules (definitions) — create, edit, clone, disable/enable — without manual YAML edits to the `PrometheusRule` resource holding those rules.
2. Standardize `group` and `component` labels on alert rules to clearly surface priority and impact, to help administrators to understand what to address first.
3. CRUD operations on user‑defined alerts via UI and API.
4. Clone platform or user alerts to customize thresholds or scopes.
5. Disable/enable alerts by creating/updating entries in the `AlertRelabelConfig` CR.
6. Create/manage silences in Alertmanager and reflect this in the UI. - Already exists today.
7. Aggregate view of all alerting rules, showing definitions plus relabel context.
8. Aggregate view of all alerts, showing status (Pending, Firing, Silenced) and relabel context.
9. Enforce multi‑tenancy: restrict queries to user’s namespace or cluster scope.
10. Persist user changes through operator upgrades, without modifying existing operators or CRDs.
11. Stay fully GitOps/Argo CD compliant: managed resources must remain commit‑able and, when owned by a GitOps application, appear read-only.
---
The primary goal is to provide a comprehensive alerting management UI that directly addresses the problems identified through user feedback, research, and competitive analysis. The proposed features are intended to reduce alert noise, provide more actionable insights, and improve the overall user experience for monitoring and responding to issues.

## Proposed Features

#### User Interface
The user interface will be redesigned, with a new **Observe > Alerting** page that highlights new grouping and components functionality.

---

#### Alerts Tab

**Summary Section**
- The summary section will be expanded by default but can be collapsed.
- It will list each group's alerts by severity.
- A tooltip will be added to each card to explain the group's purpose.
- Clicking on a link within the summary will automatically filter the main table by the selected group and severity (e.g., clicking on the number of critical alerts under 'core' will filter the table by `Group: Cluster` and `Severity: Critical`).

**Table**
- The table will list all firing alert messages.
- It will have multiple sorting options: primary sort by **Severity**, secondary by **Group**, and tertiary by **Component**.
- If a user switches to a specific group tab, the **Component** will become the secondary sort.
- The table will be responsive; on smaller screens, some names will be shortened and the **State** column will be automatically hidden as needed.

**Comprehensive Filtering Capabilities**
- Filters will be available for **Groups**, **Component**, **Severity**, **State**, **Source**, and **Triggered date/time**.
- **Saved filters** will be stored per account and users will be able to save, edit, delete, and arrange the order of the menu.

**Column Management**
- The default columns will be **Alert name**, **Severity**, **Total** (if aggregated), **State**, **Group**, **Component**, and **Source**.
- Users can manage the view by adding columns like **Description** and **Start-End**.
- The **Alert name** and **Severity** columns cannot be hidden or reordered.

**Alert Rows**
- Alerts will be aggregated by name and severity by default.
- Expanding an alert will show all alerts with the same name.
- Individual alerts will be clickable to view full details.
- Individual alert columns will display **Alert name**, **Severity**, **State**, **Source**, and **Namespace**.
- A **Resource** column with a link to the node resource page will appear for node alerts.

**Actions**
- Actions will be available to **View alerting rule**, **Acknowledge an alert**, and **Silence alert**.
- Additional actions, such as **View logs**, **View metrics**, **See related incident**, and **Troubleshoot**, will be available if the corresponding operator is installed.
- A popover will be shown with a link to the Operator Hub if an operator is not installed.
- All actions will trigger a toast notification upon completion.

**Alert Details**
- Details will be presented in a side drawer, allowing users to navigate between alerts without losing context.
- The side drawer will include a **Details** tab and a **YAML** tab.

**Bulk Actions**
- Users will be able to **Silence selected alerts**.

**Kebab Menu Options**
- The menu will include options to **Generate a summary report** and **Generate a dashboard** (via Perses integration).

---

#### Management Tab
This tab will include sub-tabs for **Alert rules**, **Silence rules**, and **Manage groups**.

**Sub-tab: Alert Rules**

- The table will list **Alert rule name**, **Severity**, **State**, **Group**, **Component**, and **Source**.
- It will feature comprehensive filtering capabilities for **Search/Keywords**, **Component**, **Severity**, **State**, **Source**, **Notification receiver**, and the Prometheus rule that the alert is associated with.
- Users will be able to filter by when the alert was triggered (e.g., last hour, today, custom range).

**Create an Alert Rule**
- This will be a wizard-based process.
- Users can add new components, but groups will be non-editable initially.
- The "Append to" selection will determine which Kubernetes PrometheusRule Custom Resource will contain the alert definition.
- Users can search by namespace or rule name, or create a new Prometheus Rule.
- Expressions will have PromQL autocompletion and a graph showing the results.
- A toast notification will be presented after the action is completed.

**Edit an Alert Rule**
- For user-defined rules, the pre-populated wizard will allow editing all fields except the PrometheusRule selector.
- For platform-defined rules, the wizard will have limited editable fields.
- A toast notification will be presented after the action is completed.

**Alert Rule Actions**
- Users can **Disable/Enable** alert rules via a toggle switch or kebab menu.
- A verification modal will appear when disabling an alert rule.
- The kebab menu will also offer to **Duplicate** or **Delete** an alert rule, with a verification modal for deletion.
- Toast notifications will be presented after each action.

**Bulk Actions**
- Bulk actions will include **Disable**, **Edit labels**, **Edit component**, **Silence**, and **Delete**.

**Alert Rule Side Drawer**
- It will include **Details**, **Active alerts**, and **YAML** tabs.
- The **Details** tab will show the name, description, source, group, component, labels, severity, expression, and other information.
- The **Active alerts** tab will show a timeline chart and a list of active alerts.

**Column Management**
- By default, the table will show **Alert rule name**, **Severity**, **State**, **Group**, **Component**, and **Source**.
- Users can add or remove additional columns like **PrometheusRule**, **Created by**, **Last modified**, and **receivers**.
- The **Alert rule name** and **Severity** columns cannot be hidden or reordered.

**Sub-tab: Silence Rules**
- This tab will list all silence rules, whether active, pending, or expired.
- Users can create, expire, delete, or recreate silence rules individually or in bulk.
- The table will show the silence name, state, duration, number of firing alerts being silenced, and the creator.
- Users can create a new silence rule based on alert labels like name, severity, and namespace.
- A toast notification will be presented after the action is completed.

**Sub-tab: Alertmanager**
- This is not in the current scope.

**Sub-tab: Manage Groups**
- Default groups will include **Impact Group: Cluster** and **Impact Group: Namespace**.
- The Cluster group will include components like **control plane nodes**, **etcd**, **network**, and **api server**.
- The Namespace group will include components like **workload** and **worker node**.
- The ability for users to create additional groups and not rely on platform-created ones is a **Could-Have** feature.

## Non‑Goals
- Deep RBAC beyond native Kubernetes permissions.
- Operators reacting to user modifications (operator code remains unchanged).
- Full multi‑cluster federation (initial focus is single‑cluster).

## Related Enhancement Proposals
- https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/alert-overrides.md
- https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/user-workload-monitoring.md

## Proposal

### Architecture

```
+-----------------------+       OAuth        +---------------------------+
|   Web browser clients |<------------------>|   OpenShift Console UI    |
+-----------------------+                    +---------------------------+
                                                       ^
                                                       |
                                                       v
                                            +---------------------------+
                                            | Console backend / gateway |
                                            +---------------------------+
                                                       ^               ^
                                                       |                \
                                                       v                 \
                                            +---------------------------+ \
                                            |      Alerting API         |  \
                                            |   /api/v1/alerting        |   \
                                            +---------------------------+    \
                                (merges AlertRelabelConfig with Alerts/Rules) \
                                               ^           ^                   \
                                              /            |                    \
                                             v             v                     v
                  +-------------------------+  +-----------------------+  +----------------------+
                  | Kubernetes API Server   |  | Thanos/Prometheus API |  | Alertmanager         |
                  +-------------------------+  +-----------------------+  +----------------------+
                        ^          ^                       ^                          ^
                   CRUD |     CRU  |                       |                          |
                        v          v                       v                          v
               PrometheusRule   AlertRelabelConfig   Alerts/Rules            Silences/Notifications


   Note: Alerts delivery: Prometheus (post‑relabel via AlertRelabelConfig) ---> Alertmanager
```

Key flows:
- Web clients authenticate and access the OpenShift Console UI.
- Console calls the Alerting API to list/manage alerts and rules.
- Alerting API aggregates rule definitions and alert instances with `AlertRelabelConfig` to present a post‑relabel view (labels, disabled status, effective severity) in list/detail endpoints.
- Alerting API reads/writes `PrometheusRule` and `AlertRelabelConfig` via the Kubernetes API.
- Alerting API queries alerts/rules via Thanos/Prometheus.

Rationale for server‑side aggregation:
- Enforces RBAC and validation uniformly using the user’s bearer token forwarded by the console backend; no credentials are stored.
- Provides a consistent, post‑relabel “effective view” and write surface that merges `PrometheusRule` with `AlertRelabelConfig` (e.g., disables, severity changes).
- Improves performance by avoiding client fan‑out and doing normalization, filtering, and pagination on the server.

Note: This component can be implemented as part of the existing console backend/gateway rather than a standalone service; the requirement is server‑side aggregation, not a separate deployment.

### Proposed UI in OpenShift Console

See additional detailes in the [UX Design- Alerts management](https://docs.google.com/document/d/1bB7kg-W2lLq85Dmy530STMUWJFlNPFvg08Sayc-RwK8/edit?usp=sharing)

- **Management List**: show all alerting rules; filter/sort by name, severity, namespace, status, labels; saved searches
![Alerting -> Management](assets/alerts-management-ui.png)

- **Alerts View**: show current firing/pending instances, silence status, relabel context
![Alerting -> Alerts](assets/alerts-management-ui2.png)

- **Create/Edit Alert Form**: fields for Alert Name, Summary, Description, Duration, Severity, Labels, Annotations (runbook links), Group & Component labels
![Alerting -> Create new Alert Rule](assets/alerts-management-ui3.png)
![Alerting -> Update Platform Alert](assets/alerts-management-ui4.png)

- **Create/Edit Alert labels Form**: list common labels, Add/remove alert labels.
- **Silences List**: define matchers, duration, comment - Keep

### Additional Points to Consider
---

**RBAC**
- The approach is to filter alerts based on a user's namespace access.
- Administrators will have an "all namespaces" option.
- The UI will include a dropdown menu for users to select a specific namespace.
- The possibility of using "permissions by labels" is mentioned for further exploration.

**Terminology Alignments**
- There is a need for consistent terminology, such as Namespace vs. project and Alert rule vs. alert definition.

**Notifications Improvements**
- Future improvements could include notifications by group and component type, and notifications per team (RBAC related).

**Multi-cluster View**
- A separate design task (HPUX-795) is in progress for a multi-cluster alerting UI.
- The main difference is the scope of data presented, providing a centralized, aggregated view for a fleet of clusters compared to a single cluster's data.
- Key features for a multi-cluster view include a **Centralized View**, a **Cluster-Specific Context** filter, and enhanced **RBAC** for managing alerts across multiple clusters.

---

### Feature Prioritization

The features are prioritized using tags: **Must-Have**, **Should-Have**, **Could-Have**, and **Won't-Have**.

**Must-Have Features:**
- Tabs changes (Alerts, Management: Alert rules, Silence rules)
- Create user-defined alert rules (Alert rules definition and Metadata)
- Advanced filtering capabilities
- Bulk actions: disable, edit labels, edit component
- Duplicate and Delete alert rules
- Incident tab

**Should-Have Features:**
- Add components
- Saved filters
- Alert and alert rule side drawer
- Add "Resource" column for node alerts

**Could-Have Features:**
- Notifications (alertmanager receivers)
- PromQL expression autocompletion and graph
- "Save as draft" wizard
- Alert rule history
- Acknowledge alert
- Filter by triggered date/time
- Column management
- Additional alert action items (View logs, Troubleshoot, etc.)
- Generate a summary report
- Generate a dashboard
- Manage groups

**Won't-Have Features:**
- Alertmanager sub-tab

### API Endpoints

Base path: `/api/v1/alerting`

Rule identity

- A rule is uniquely identified by the tuple: `(namespace, prometheusrule, ruleName, specHash)`.
- Canonical opaque identifier used below as `ruleId`:
  - `/namespaces/{namespace}/prometheusrules/{prometheusrule}/rules/{ruleName}/specHash/{specHash}`.
    - `specHash` is server‑generated from the rule spec to ensure uniqueness and stability. It is computed as SHA‑256 (hex, lowercase) of the normalized fields:
      - `expr`: trimmed with consecutive whitespace collapsed
      - `for`: normalized to seconds
      - `labels`: Validate labels `key` and `value` meet Prometheus syntax and character rules; drop empty values; sort by key ascending; join as `key=value` lines separated by `\n`
      - Concatenate the three parts with `\n---\n` separators, then hash the UTF‑8 bytes
    - Clients must treat `specHash` as opaque (do not construct it). When `expr`/`for`/labels change, the server recomputes `specHash` and returns an updated `ruleId`.

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
| GET    | `/rules`                           | List all alert rule definitions **(platform and user)**, including `disabled` flags and relabel context. Supports the filter parameters listed below. | **Required** — merges `AlertRelabelConfig` with platform alerts.|
| GET    | `/rules/labels`                    | List common labels across selected rules across  **(platform and user)** alert rules. | **Required** — merges `AlertRelabelConfig` with platform alerts.|
| POST   | `/rules`                           | Create a new user‑defined alert rule. | Wraps K8s API for `PrometheusRule`. |
| GET    | `/rules/{ruleId}`                  | Fetch a single rule’s full definition and relabel context. | **Required** — merges `AlertRelabelConfig` with platform alert.|
| PATCH  | `/rules/{ruleId}`                  | Update a single rule (platform or user‑defined). | |
| DELETE | `/rules/{ruleId}`                  | Delete a user‑defined rule. | |
| POST   | `/rules`                           | Bulk update rules (e.g., severity/status/labels).  | |
| DELETE | `/rules`                           | Bulk delete user‑defined rules. | |

Filter parameters for `/rules`:

- `name`
- `group`
- `namespace`
- `component`
- `severity`
- `state` (one of: `enabled`, `disabled`) — rule enablement status. Note: “silenced” is not a rule state.
- `has_silenced_alerts` (boolean) - filter rules that currently have one or more associated alerts silenced. - TBD
- `source`
- `notification_receiver`
- Other alert labels (arbitrary `labelKey=labelValue` pairs)

**Health**

| Method | Path        | Description                 | Notes |
|--------|-------------|-----------------------------|-------|
| GET    | `/health`   | Basic health‑check endpoint |       |



### Data Model
- Store user alerts in `PrometheusRule`s in the specified project.
- Update platform alerts by writing relabel rules into the `AlertRelabelConfig` CR in `cluster-monitoring-config`.
- Use labels for disable/enable flags; rely on existing CRDs - Applies only for Platform alert rules.

### Migration
- Automated tool to detect existing `PrometheusRule` CRDs and import them into the new system
- Preserve existing labels, annotations and support rollback on errors

### GitOps / Argo CD Compliance
- All generated `PrometheusRule` and `AlertRelabelConfig` resources remain declarative and suitable for committing to Git

### Workflow Description

## Pain Points Addressed by this Design
- **Generic and Non-Actionable Alerts:** The design will provide more actions from an alert and link to resources like logs and Korrel8r for troubleshooting.
- **Alert Noise and Data Overload:** Grouping, advanced filters, and saved filters will help reduce noise and the need for repetitive filtering.
- **Missed Alarms or Missing Data:** Users will be able to create flexible alert definitions directly in the UI to monitor any data type, configure notifications, and link a runbook.

## Pain Points Not Directly Addressed
- High Resource Consumption of Observability Tools
- Lack of Unified Views (within a single-cluster scope)

## Risks & Mitigations
- **Performance**: split large rule sets into multiple CRs; benchmark reconciliation latency
- **Misconfiguration**: validate syntax; prevent disabling protected base alerts via exclusion list
- **Complexity**: sensible defaults; UI guidance; limit advanced options in MVP

## Graduation Criteria
- **Tech Preview**: basic CRUD, clone, disable/enable, silencing via UI/API; migration tool v1
- **GA**: best‑practice guidance in UI; multi‑namespace filtering; full test coverage; complete documentation

## Open Questions
1. **Per‑Alert vs. Single‑File**: Should each user‑defined alert reside in its own `PrometheusRule` file, or group all into one? A customer noted per‑alert files may simplify GitOps/Argo CD maintenance—does that hold true at scale? - Agreed that this would be up to the user to define.
2. **Read‑Only Detection**: Which annotations, labels or ownerReferences reliably indicate GitOps‑managed resources to render them read‑only in our UI?
3. **Concurrent Operator Updates**: How should we handle cases where upstream operators update their own `PrometheusRule` CRs—should we reconcile `AlertRelabelConfig` entries periodically? - [simonpasquier] Replied that, as of now, the cluster admin will need to detect the drift and update their customization.
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
- Existing URLs must remain operational across upgrades.
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