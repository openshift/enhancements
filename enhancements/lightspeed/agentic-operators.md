---
title: lightspeed-agentic-operators
authors:
  - "@harche"
reviewers:
  - "@everettraven"
  - "@wking"
  - "@mrunalp"
approvers:
  - "@mrunalp"
api-approvers:
  - TBD
creation-date: 2026-05-08
last-updated: 2026-05-08
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3095
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Lightspeed Agentic Operators

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenShift Lightspeed Agentic Operators introduce autonomous, policy-driven AI agents that diagnose, remediate, and verify cluster issues through a declarative CRD-based workflow. The platform defines three core custom resources — `LLMProvider`, `Agent`, and `Proposal` — under the `agentic.openshift.io/v1alpha1` API group, along with supporting resources for approval policy, approval tracking, and step results. Each Proposal follows a run-to-completion lifecycle (Analysis → Execution → Verification) with configurable approval gates, dynamic RBAC scoping, sandboxed agent execution, and automatic retry with escalation. The feature is gated behind the `LightspeedAgents` feature gate, targeting Tech Preview in OCP 5.0.

## Motivation

OpenShift clusters generate alerts, security violations, and operational events that require expert diagnosis and remediation. Today, Site Reliability Engineers (SREs) must manually triage each event — reading logs, querying metrics, identifying root causes, and applying fixes. This process is time-consuming, error-prone, and does not scale with fleet size.

Large Language Models (LLMs) with tool-use capabilities can now perform multi-step diagnostic and remediation workflows when given appropriate cluster access and guardrails. However, giving an LLM direct cluster-admin access is unacceptable from a security standpoint. A platform is needed that:

- Provides a declarative, auditable interface for AI-driven operations
- Enforces least-privilege access through dynamic, per-task RBAC
- Requires human approval at configurable checkpoints
- Sandboxes agent execution to prevent uncontrolled mutations
- Supports multiple LLM backends without vendor lock-in

### User Stories

* As an SRE, I want the platform to automatically diagnose a crash-looping pod and propose a remediation, so that I can review and approve the fix instead of manually investigating.

* As a cluster administrator, I want alerts from AlertManager to automatically trigger diagnostic proposals, so that issues are investigated immediately without waiting for human triage.

* As a security engineer, I want ACS policy violations to generate remediation proposals scoped to the affected namespace, so that compliance issues are addressed promptly with least-privilege access.

* As a platform team lead, I want to configure approval policies that require manual approval for execution but auto-approve analysis, so that agents can investigate freely while mutations require human sign-off.

* As an SRE managing a fleet, I want the platform to retry failed remediations with enriched context and escalate to a senior agent when retries are exhausted, so that complex issues are handled without manual re-triage.

* As a cluster administrator, I want to review the agent's proposed RBAC permissions before execution begins, so that I can verify the agent will not receive excessive access.

* As an operations team, I want to monitor agent proposal lifecycles through standard Kubernetes conditions and metrics, so that I can integrate with existing monitoring and alerting infrastructure.

### Goals

1. Ship the Lightspeed agentic platform as Tech Preview in OCP 5.0 behind the `LightspeedAgents` feature gate (enabled in `DevPreviewNoUpgrade` and `TechPreviewNoUpgrade` feature sets).
2. Provide a CRD-based interface (`Proposal`) for AI-driven diagnostic and remediation workflows with configurable approval gates.
3. Enforce least-privilege access through dynamic, per-proposal RBAC generation validated by the operator before agent execution.
4. Support multiple LLM providers (Anthropic, Google Cloud Vertex AI, OpenAI, Azure OpenAI, AWS Bedrock) through the `LLMProvider` CRD.
5. Enable adapter-driven proposal creation from AlertManager webhooks, ACS violation webhooks, and manual triggers.
6. Provide a console plugin for proposal lifecycle management (list, detail, approve/deny, revision feedback, escalation, sandbox log streaming).

### Non-Goals

1. GA readiness in OCP 5.0 — this is a Tech Preview feature that will iterate based on feedback.
2. Replacing existing monitoring, alerting, or incident management systems — the platform complements them by acting on their signals.
3. Providing a general-purpose LLM chat interface — the existing OpenShift Lightspeed chat serves that role.

## Proposal

The Lightspeed agentic platform consists of four components deployed in the `openshift-lightspeed` namespace:

1. **[Agentic Controller](https://github.com/openshift/lightspeed-agentic-operator)** — A set of Kubernetes controllers imported into the existing [Lightspeed operator](https://github.com/openshift/lightspeed-operator), activated when the `LightspeedAgents` feature gate is enabled. The controllers reconcile `Proposal`, `Agent`, `LLMProvider`, `ProposalApproval`, and `ApprovalPolicy` CRDs. They manage the proposal lifecycle, create dynamic RBAC, launch sandboxed agent pods, and record results. The agentic controller code is developed separately in [`lightspeed-agentic-operator`](https://github.com/openshift/lightspeed-agentic-operator) and imported as a Go library.

2. **[Agent Sandbox](https://github.com/openshift/lightspeed-agentic-sandbox)** — An ephemeral pod launched per workflow step (analysis, execution, verification) that runs an LLM-powered agent with tool-use capabilities. Skills (OCI image volumes) and MCP servers provide the agent's toolset. The sandbox is destroyed after the step completes.

3. **[Console Plugin](https://github.com/openshift/lightspeed-agentic-console)** — A dynamic plugin for the OpenShift console that provides proposal list and detail pages, approve/deny actions, revision feedback, escalation triggers, and real-time sandbox log streaming.

4. **[Skills Images](https://github.com/openshift/lightspeed-skills)** — OCI images containing Claude Code skills (Prometheus querying, cluster operations, RBAC security analysis, Red Hat support integration, etc.) mounted as read-only volumes in agent sandboxes.

### Workflow Description

**Cluster administrator** configures the platform through a three-step setup:

1. Creates `LLMProvider` resources to configure LLM backend connectivity and credentials.
2. Creates `Agent` resources to define agent tiers (e.g., "default", "smart", "fast") pairing a provider with a model and runtime settings. A "default" agent must exist.
3. Optionally creates an `ApprovalPolicy` (cluster-scoped singleton) to configure which workflow steps require manual approval vs. auto-approval, and sets retry limits. If no `ApprovalPolicy` exists, all steps default to manual approval.

**Adapter** (AlertManager webhook, ACS violation webhook, or manual user) creates a `Proposal` resource.

**Operator** reconciles the Proposal through its lifecycle:

1. **Pending** — Proposal is created. Operator validates references (Agent, LLMProvider, secrets) and creates a `ProposalApproval` resource.

2. **Analyzing** — Operator checks approval policy for the analysis stage. If auto-approved or manually approved via `ProposalApproval`, the operator launches an analysis sandbox pod. The agent runs with cluster-wide read access to enable holistic diagnosis — reading logs, metrics, events, and resource state across namespaces. This broad read scope is necessary because root-cause analysis frequently spans multiple namespaces and cluster-scoped resources. The agent produces a structured `AnalysisResult` containing: diagnosis, proposed remediation options, requested RBAC permissions for execution, and a verification plan.

3. **Proposed** — Analysis is complete. The user reviews the analysis result in the console, including the requested RBAC permissions. The user may:
   - **Approve** — Select a remediation option and approve execution via `ProposalApproval`.
   - **Deny** — Deny the proposal (terminal state).
   - **Revise** — Set `spec.revisionFeedback` to re-run analysis with additional context and user guidance.
   - **Escalate** — Manually trigger escalation.

4. **Executing** — Operator creates a per-proposal ServiceAccount with scoped Role/RoleBinding (namespace-scoped) or ClusterRole/ClusterRoleBinding (cluster-scoped) based on the RBAC permissions requested in the analysis result. The user reviews these permissions as part of the approval step. The operator then launches an execution sandbox pod with the granted permissions. The agent applies the approved remediation.

5. **Verifying** — Operator launches a verification sandbox pod. The agent checks that the remediation was effective by running the verification plan from the analysis step. If verification fails and retry attempts remain, the operator re-runs execution with enriched context (previous failure details appended). After `maxAttempts` retries are exhausted, the proposal transitions to escalation.

6. **Completed** — Verification passed. The operator cleans up RBAC resources (ServiceAccount, Role, RoleBinding). The proposal is in a terminal state.

**Alternative terminal states:**
- **Denied** — User denied a step.
- **Failed** — System error (agent crash, timeout, invalid configuration).
- **Escalated** — Retries exhausted; operator creates an escalation result with failure history for senior review.

#### Approval Flow

The `ApprovalPolicy` (cluster-scoped singleton named "cluster") configures default approval behavior:

```yaml
apiVersion: agentic.openshift.io/v1alpha1
kind: ApprovalPolicy
metadata:
  name: cluster
spec:
  stages:
    - name: Analysis
      approval: Automatic
    - name: Execution
      approval: Manual
    - name: Verification
      approval: Automatic
  maxAttempts: 2
  maxConcurrentProposals: 5
```

Each `ProposalApproval` resource (created 1:1 with each Proposal) tracks per-stage decisions. Users approve or deny stages by patching the `ProposalApproval` resource (or via the console UI).

#### Assisted and Advisory Workflows

Not all proposals require execution. The workflow shape is configured inline on the Proposal spec:

- **Full remediation**: `analysis` + `execution` + `verification` — agent diagnoses, fixes, and verifies.
- **Assisted**: `analysis` + `verification` (no `execution`) — agent diagnoses, user applies fix manually, agent verifies.
- **Advisory**: `analysis` only — agent diagnoses and provides recommendations without any execution or verification.

### API Extensions

This enhancement introduces 6 CRDs and 4 result CRDs under the `agentic.openshift.io/v1alpha1` API group. All are new resources; no existing OCP API surfaces are modified.

#### Core CRDs

**LLMProvider** (cluster-scoped) — Manages LLM backend connectivity:
```yaml
apiVersion: agentic.openshift.io/v1alpha1
kind: LLMProvider
metadata:
  name: vertex-claude
spec:
  type: GoogleCloudVertex
  googleCloudVertex:
    credentialsSecret:
      name: gcp-credentials
    projectID: my-gcp-project
    region: us-central1
  model: claude-opus-4-6
```

Supported provider types: `Anthropic`, `GoogleCloudVertex`, `OpenAI`, `AzureOpenAI`, `AWSBedrock`.

**Agent** (cluster-scoped) — Defines agent tiers pairing a provider with runtime settings:
```yaml
apiVersion: agentic.openshift.io/v1alpha1
kind: Agent
metadata:
  name: default
spec:
  llmProvider:
    name: vertex-claude
  model: claude-opus-4-6
  maxTurns: 100
  timeouts:
    analysisSeconds: 600
    executionSeconds: 900
    verificationSeconds: 300
```

**Proposal** (namespaced) — Primary unit of work:
```yaml
apiVersion: agentic.openshift.io/v1alpha1
kind: Proposal
metadata:
  name: fix-crashloop
  namespace: production
spec:
  request: "Pod nginx-abc is crash looping with OOMKilled"
  targetNamespaces:
    - production
  tools:
    skills:
      - image: registry.redhat.io/openshift-lightspeed/lightspeed-skills:latest
  analysis:
    agent: default
  execution:
    agent: default
  verification:
    agent: default
```

**ProposalApproval** (namespaced) — Per-stage approval tracking (created by operator, patched by user):
```yaml
apiVersion: agentic.openshift.io/v1alpha1
kind: ProposalApproval
metadata:
  name: fix-crashloop
spec:
  stages:
    - type: Analysis
      decision: Approved
    - type: Execution
      decision: Approved
      execution:
        option: 0
        maxAttempts: 2
```

**ApprovalPolicy** (cluster-scoped singleton) — Default approval behavior (see Approval Flow above).

#### Result CRDs (namespaced, operator-managed)

- **AnalysisResult** — Structured analysis output: diagnosis, remediation options, RBAC requests, verification plan.
- **ExecutionResult** — Actions taken, inline verification, outcome.
- **VerificationResult** — Verification checks and results.
- **EscalationResult** — Escalation summary and failure history.

All result CRDs are owned by their parent Proposal via owner references for garbage collection.

#### Impact on Existing APIs

None. All CRDs are new and additive. The operator does not modify any existing OpenShift or Kubernetes resources beyond creating standard RBAC resources (ServiceAccount, Role, RoleBinding, ClusterRole, ClusterRoleBinding) scoped to each proposal's needs.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The agentic operator runs in the management cluster control plane. Proposals target workloads in the hosted cluster's data plane. The operator creates RBAC resources in the hosted cluster via the management cluster's kubeconfig. No HostedCluster API changes are required. Skills and agent sandboxes run in the management cluster.

This integration is not targeted for Tech Preview; Hypershift support will be evaluated for GA.

#### Standalone Clusters

Fully supported. This is the primary deployment model. The operator, agent sandboxes, and console plugin all run in the `openshift-lightspeed` namespace.

#### Single-node Deployments or MicroShift

**SNO**: Supported. Agent sandboxes are ephemeral and short-lived. The `maxConcurrentProposals` setting in `ApprovalPolicy` can be used to limit concurrent sandboxes.

**MicroShift**: Not applicable. MicroShift does not include the OpenShift console or the Lightspeed operator.

#### OpenShift Kubernetes Engine

Not applicable. The Lightspeed operator is not included in OKE.

### Implementation Details/Notes/Constraints

**Kubernetes Version Requirement**: Skills are mounted as OCI image volumes, which requires Kubernetes 1.34+ (OpenShift 4.21+). This feature is GA in Kubernetes 1.34.

**LLM Credential Isolation**: LLM provider credentials are stored in Kubernetes Secrets and injected into sandbox pods as environment variables or file mounts. They are never logged or included in Proposal status.

**Sandbox Pod Lifecycle**: Each workflow step launches a separate pod. The pod runs to completion and is not restarted. The operator monitors pod status and records results in the corresponding Result CRD. Pods are cleaned up when the proposal reaches a terminal state.

**Skills as OCI Images**: Agent capabilities (Prometheus querying, cluster operations, RBAC analysis, etc.) are packaged as OCI images and mounted as read-only volumes. This allows versioned, auditable skill delivery without modifying the agent container image.

**MCP Server Integration**: Agents can call external Model Context Protocol (MCP) servers for additional tool access. MCP server authentication supports Kubernetes Secrets, ServiceAccount tokens, and client-provided headers.

### Risks and Mitigations

**Risk: Analysis agents have cluster-wide read access.**
Mitigation: In Tech Preview, the platform is targeted at cluster administrators who already have broad cluster access. Analysis requires a holistic view of cluster state (logs, metrics, events, resources across namespaces) to perform effective root-cause diagnosis. The analysis sandbox is strictly read-only — it cannot create, modify, or delete any resources. Write access is only granted during the execution step, where RBAC is scoped per-proposal and requires explicit approval. Future releases may introduce finer-grained read scoping based on user feedback.

**Risk: LLM produces incorrect remediation.**
Mitigation: Human approval gates before execution. The analysis step produces a structured diagnosis with confidence levels, risk assessment, and rollback plans. The user reviews the requested RBAC permissions as part of the approval step before any execution occurs.

**Risk: Agent exceeds granted RBAC permissions.**
Mitigation: Dynamic RBAC creates minimal ServiceAccount/Role/RoleBinding per proposal based on what the analysis agent requested. The agent sandbox pod uses this ServiceAccount and cannot escalate privileges. RBAC resources are scoped to the proposal's target namespaces and cleaned up when the proposal reaches a terminal state.

**Risk: LLM API costs are unbounded.**
Mitigation: Per-step timeouts and `maxTurns` limits prevent runaway LLM usage. `maxConcurrentProposals` limits parallel execution. Cost tracking is recorded in step results. Additionally, the default `ApprovalPolicy` requires manual approval for all steps — no agent runs without explicit human authorization unless the administrator opts in to auto-approval.

**Risk: Sandbox pod consumes excessive cluster resources.**
Mitigation: Pods have resource requests/limits. `maxConcurrentProposals` in `ApprovalPolicy` limits concurrent sandboxes. Pods are ephemeral and cleaned up at terminal state.

### Drawbacks

Placeholder — to be filled in before submitting.

## Alternatives (Not Implemented)

Placeholder — to be filled in before submitting.

## Open Questions

Placeholder — to be filled in before submitting.

## Test Plan

Testing will cover:

- **Unit tests**: Controller reconciliation logic, RBAC generation, phase derivation, approval policy evaluation, sandbox template generation.
- **Integration tests**: End-to-end proposal lifecycle with a mock LLM backend. Covers all workflow patterns (full, assisted, advisory), approval flows (auto, manual, deny), retry/escalation, and RBAC creation/cleanup.
- **e2e tests**: Placeholder — to be filled in before submitting.
- **Security tests**: RBAC boundary validation — agents cannot access resources outside granted scope. Sandbox pod cannot escalate privileges. LLM credentials are not leaked in status or logs.

## Graduation Criteria

### Dev Preview -> Tech Preview

- End-to-end proposal lifecycle functional (analysis, execution, verification, retry, escalation)
- Console plugin with proposal management (list, detail, approve/deny, revision feedback, log streaming)
- AlertManager and ACS adapter integration
- Dynamic RBAC generation and cleanup
- Multiple LLM provider support (at least Anthropic via Vertex AI and Bedrock)
- Unit and integration test coverage
- Feature gated behind `LightspeedAgents` in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`

### Tech Preview -> GA

- Stability: no regressions across two consecutive OCP releases
- Load testing: concurrent proposal handling under realistic alert volumes
- Upgrade/downgrade testing
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Telemetry: proposal success/failure rates, LLM cost tracking
- Feedback from internal and external Tech Preview users
- Security audit of RBAC generation and sandbox isolation
- API review and approval from `#forum-api-review`

### Removing a deprecated feature

Not applicable for initial release.

## Upgrade / Downgrade Strategy

**Upgrade**: When the `LightspeedAgents` feature gate is enabled (by selecting `TechPreviewNoUpgrade` or `DevPreviewNoUpgrade` feature set), the Lightspeed operator enables its agentic controller and installs the agentic CRDs. No existing cluster state is modified.

**Downgrade**: When downgrading to a version without the feature gate, the agentic controller is disabled. CRD instances (Proposals, Agents, etc.) remain in etcd but are inert without the controller. Administrators should delete CRD instances before downgrading.

**Version skew**: The operator is the sole controller for all agentic CRDs. There is no version skew concern between components because the operator, console plugin, and agent sandbox are deployed as a unit from the same OLM bundle.

## Version Skew Strategy

The Lightspeed operator is a layered operator installed via OLM, not part of the OCP release payload. All agentic components (operator with agentic controller, console plugin, agent sandbox image, skills images) are versioned together within the operator's OLM bundle. The agentic controller is the sole reconciler for agentic CRDs, so there is no cross-component version skew during normal operation.

During an upgrade, OLM updates the Lightspeed operator Deployment first, followed by the console plugin. In-flight proposals continue with the old operator version until the new operator pod is ready. The new operator picks up existing proposals and continues reconciliation. No special handling is required because proposal state is fully captured in CRD status conditions.

## Operational Aspects of API Extensions

**Health indicators**:
- Operator Deployment health (replicas ready)
- `Agent` status condition `Ready=True` indicates all referenced resources exist
- `Proposal` status conditions track per-step progress (`Analyzed`, `Executed`, `Verified`)
- Operator exposes standard controller-runtime metrics (reconcile duration, queue depth, error rate)

**Impact on existing SLIs**:
- Minimal. The agentic CRDs are independent resources that do not intercept or modify existing API request paths.
- Expected scale: tens to low hundreds of active proposals per cluster. API throughput impact is negligible. Agent sandbox pods are lightweight — they run a Python SDK process with CLI tools (oc, kubectl, claude) and make outbound LLM API calls. There is no GPU or heavy compute requirement on the sandbox pod itself; all inference happens on the remote LLM endpoint or in-cluster model server. This makes sandboxes cheap to schedule and scale.
- RBAC resources (ServiceAccount, Role, RoleBinding) are created and deleted per proposal. At steady state, only active proposals have associated RBAC resources.

**Failure modes**:
- **Operator crash**: Proposals pause in their current phase. No mutations occur without the controller running. Recovery is automatic on operator restart; reconciliation resumes from the last recorded condition.
- **LLM API unavailable**: Sandbox pods fail with timeout. Proposals enter `Failed` state. No cluster mutations occur because execution requires a successful analysis first.
- **Sandbox pod OOM/crash**: Step fails. Controller records failure in the corresponding Result CRD. If retries remain, re-execution is attempted. Otherwise, escalation is triggered.

**Escalation teams**: Lightspeed team (Jira component: `Lightspeed`).

## Support Procedures

**Detecting failures**:
- Check operator pod logs in `openshift-lightspeed` namespace for reconciliation errors.
- Check `Proposal` status conditions for `Failed` or `Escalated` states.
- Check `AnalysisResult`, `ExecutionResult`, `VerificationResult` status for `failureReason`.
- Operator metrics: `controller_runtime_reconcile_errors_total{controller="proposal"}`.

**Disabling the feature**:
- Scale the operator Deployment to 0 replicas. All proposals pause; no new mutations occur. Running sandbox pods are not terminated but will not be monitored.
- To fully remove: delete all Proposal resources, then uninstall the operator. RBAC resources are cleaned up by the operator's finalizer during Proposal deletion.
- Disabling does not affect existing workloads. The operator does not modify workload resources except through approved proposal execution.

**Recovery**:
- Scale the operator back to 1 replica. Reconciliation resumes automatically from the last recorded state.
- Proposals in terminal states (Completed, Failed, Denied, Escalated) are not re-processed.
- In-flight proposals resume from their current phase.

## Infrastructure Needed

- The agentic controller is developed in [`openshift/lightspeed-agentic-operator`](https://github.com/openshift/lightspeed-agentic-operator) and imported as a Go library into the existing [`openshift/lightspeed-operator`](https://github.com/openshift/lightspeed-operator)
- OCI image registry space for skills images
- CI infrastructure for e2e tests with LLM API access (mock backend for CI, real backend for periodic jobs)
