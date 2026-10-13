---
title: must-gather-adapter-for-agentic-ols
authors:
  - "@spatidar"
  - "@shivprakashmuley"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-07-16
last-updated: 2026-08-12
status: provisional
tracking-link:
  - "https://redhat.atlassian.net/browse/OAPE-688"
---

# Must-Gather Adapter for Agentic OLS

## Summary

Automate root-cause analysis (RCA) of must-gather diagnostic data so operators no longer need to manually download and interpret large diagnostic bundles. After a successful must-gather collection, an optional integration with agentic OLS runs the IntelliAide RCA pipeline in a sandboxed agent and surfaces results as an AnalysisResult.

The analysis agent does not require cluster-admin access: it reads diagnostic data only through structured MCP tool calls served by openshift-mcp-server, not direct PVC or API access.

**Phase 1 (this proposal):** must-gather-operator orchestrates gather → MCP access layer → Lightspeed agent → AnalysisResult, using a pragmatic PoC integration (see "Phased Approach").

**Phase 2 (future):** declarative MCP lifecycle (MCPServer CR, DiagnosticArchive), stronger per-collection authorization, and reduced custom infrastructure in must-gather-operator (see "Future Direction" and "Gaps and Blockers Today").

## Motivation

Customers use MustGather to collect cluster diagnostics when something goes wrong. Today that workflow typically ends at collection: data is stored or uploaded, and a human expert must download, navigate, and interpret it.

Agentic OLS can run automated RCA (IntelliAide) in a security-constrained sandbox. That sandbox cannot safely be given broad cluster access or direct access to diagnostic storage in another namespace.

**Why MCP:** A dedicated MCP server mounts the collected diagnostic data and exposes it as scoped, read-only tool calls over HTTP—the same protocol agentic OLS already uses for tools. This keeps the agent sandbox least-privileged while still enabling automated analysis.

### User Stories

- As a cluster administrator, I want must-gather collection to optionally trigger automated RCA so I receive an analysis report without manually reviewing raw logs.
- As a SRE, I want the analysis agent to run with least privilege and never require cluster-admin to read diagnostics.
- As a support engineer, I want RCA output as a cluster API object (AnalysisResult) for automation and case workflows.
- As an administrator running multiple collections, I want each analysis scoped to the intended collection only—not other tenants' or other runs' data.
- As a platform admin, I want this capability behind a feature gate until the integration is stable.

### Goals

1. After a successful must-gather collection with agentic analysis enabled, IntelliAide RCA runs automatically (High-priority pass in v1) with no human intervention.
2. The analysis agent never needs cluster-admin privileges—it reads data only through MCP.
3. **Collection isolation:** Each analysis is scoped to a single must-gather collection. The agent must not enumerate or query across unrelated collections (e.g. broad "list all must-gathers" access is out of scope). v1 enforces this by binding the agent to one path in the analysis request and serializing shared MCP access when multiple collections share storage.
4. A single shared MCP server serves collections on the same PVC via path isolation; concurrent analyses are serialized when they would conflict.
5. The feature is opt-in (defaults off) and gated behind a cluster **FeatureGate** before Tech Preview graduation.
6. No changes required to the Lightspeed agentic platform APIs beyond consuming existing Proposal/AnalysisResult (or AgenticRun successor).
7. The user retains responsibility for PVC lifecycle (creation, sizing, cleanup).

### Non-Goals

1. Changes to the Lightspeed agentic operator, sandbox, or core agent behavior.
2. Changes to the openshift-mcp-server toolset (this feature consumes the existing `openshift/mustgather` toolset).
3. Remediation execution (we produce an AnalysisResult; execution is a Lightspeed concern).
4. Multi-namespace storage/MCP layout in v1 (MustGather, PVC, and MCP server must be in the operator namespace).
5. Multiple concurrent PVCs per shared MCP server in v1.
6. Custom must-gather images for user workload namespaces (separate concern).

## Design Overview

```
User → MustGather CR → gather Job → diagnostic data on storage
                              ↓
                    MCP access layer (openshift-mcp-server)
                              ↓
              Proposal → agent sandbox (IntelliAide) → AnalysisResult
```

| Component | Role |
|-----------|------|
| **must-gather-operator** | Triggers gather; after success, enables MCP access and creates a Lightspeed analysis request |
| **openshift-mcp-server** | Serves read-only `mustgather_*` tools over the collected archive |
| **agentic OLS** | Runs IntelliAide in a sandbox; consumes MCP URL from the analysis request |
| **IntelliAide skills** | RCA pipeline (extract → analyze → report) |

Data flow: gather writes to storage → MCP server reads that storage → agent is assigned **one** collection path and calls MCP for that archive only → AnalysisResult is written.

## Phased Approach

### Phase 1 — Must-Gather Adapter for Agentic OLS (this EP)

**Goal:** End-to-end automated RCA after must-gather, with minimal new platform APIs.

| Area | Phase 1 approach |
|------|------------------|
| MCP pod lifecycle | must-gather-operator ensures a shared MCP server (Deployment/Service) |
| Storage | User-provided PVC; operator mounts it for MCP; collection path passed to agent in analysis request |
| Analysis trigger | Lightspeed Proposal (or successor AgenticRun) with IntelliAide skills |
| Authorization | Namespace isolation + one active collection at a time on shared MCP (see "Security Model") |
| Feature exposure | Opt-in field on MustGather; **gated behind cluster FeatureGate** before Tech Preview |
| API surface | One optional boolean on existing MustGather CR |

### Phase 2 — Platform-aligned lifecycle (future)

**Goal:** Replace bespoke MCP Deployment management with shared platform patterns.

| Area | Phase 2 direction |
|------|-------------------|
| MCP lifecycle | MCPServer CR reconciled by MCP Lifecycle Operator |
| Collection metadata | DiagnosticArchive CR (path, ownership, auth boundary) |
| Authorization | Token impersonation + RBAC per collection |
| Storage lifecycle | Coordinated PVC/directory lifecycle (upstream + operator) |

Phase 2 depends on upstream gaps in "Gaps and Blockers Today"; Phase 1 does not block on them.

## Gaps and Blockers Today

| Gap | Why it matters | Phase 1 workaround | Phase 2 fix |
|-----|----------------|-------------------|-------------|
| **MCPServer CR has no PVC storage type** | Cannot declare PVC mount on MCPServer CR | Operator creates Deployment with PVC directly | Upstream: PVC in `spec.config.storage` |
| **No DiagnosticArchive CR** | Collection path only in operator logic / analysis request | Runtime path resolution + prompt | New CR + RBAC |
| **Per-collection auth** | Shared MCP could expose wrong collection if misconfigured | Serialize analyses; scope agent to one path per request | Impersonation + DiagnosticArchive |
| **Catalog MCP image / toolset** | Image must include `openshift/mustgather` toolset | Pin `MCP_SERVER_IMAGE` explicitly until catalog validated | Validated catalog tag |

Detailed implementation notes and per-repository impact are tracked in the companion design document for this feature.

## Proposal

Add an optional boolean field to the MustGather CR. When enabled and storage is configured, after a successful gather Job the operator:

1. **Ensures an MCP access layer** — a shared openshift-mcp-server instance with only the `openshift/mustgather` toolset, mounting the user's PVC read-only at `/data`.
2. **Creates a Lightspeed analysis request** — a Proposal CR (or AgenticRun successor) with the MCP URL, IntelliAide skill reference, and instructions scoped to **one** collection path on that volume.

If agentic OLS is not installed, or MCP/analysis setup fails, must-gather collection still completes normally (best-effort integration).

### Workflow

1. Administrator creates PVC and a MustGather CR with storage and agentic analysis enabled.
2. Operator runs gather Job; output is written to the PVC under a per-run subdirectory.
3. On successful completion, operator ensures the shared MCP server is running with the PVC mounted.
4. Operator creates a Lightspeed analysis request pointing the agent at the MCP server and the specific collection path.
5. Agentic OLS spawns a sandbox; the agent runs IntelliAide (High-priority pass in v1) via MCP tool calls.
6. AnalysisResult is produced for the administrator to retrieve.

**Error handling:** Lightspeed absent → skip analysis, gather succeeds. MCP or Proposal failure → logged, gather succeeds. Gather failure → no analysis path triggered.

### API Extensions

One optional boolean field on `MustGatherSpec` (default `false`). When enabled, the operator requires configured storage so diagnostic data is available to the MCP layer.

```go
type MustGatherSpec struct {
    // ... existing fields ...

    // AgenticDebuggingEnabled, when true, automatically creates a Lightspeed
    // Proposal CR for agentic root-cause analysis after a successful
    // must-gather collection. Requires storage to be configured so the
    // MCP layer can serve the collected data to the analysis agent.
    // +kubebuilder:default:=false
    // +optional
    AgenticDebuggingEnabled *bool `json:"agenticDebuggingEnabled,omitempty"`
}
```

*Validation note:* Strict CRD CEL tying this field to a specific storage shape is deferred to avoid locking the API before storage options are finalized for GA. The operator enforces the storage requirement at reconcile time; admission/webhook validation may be added in Tech Preview if the storage contract stabilizes.

Example CR:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: debug-cluster-issue
  namespace: must-gather-operator
spec:
  serviceAccountName: must-gather-admin
  agenticDebuggingEnabled: true
  storage:
    type: PersistentVolume
    persistentVolume:
      claim:
        name: must-gather-pvc
      subPath: collections
```

No new CRDs in Phase 1. The operator creates a Proposal in `openshift-lightspeed` via an unstructured client.

**Feature gating:** The field and post-gather agentic behavior will be behind a cluster **FeatureGate** (TechPreview), consistent with other volatile OpenShift APIs—not only an operator environment variable.

### Topology Considerations

**Primary target:** standalone OpenShift clusters with agentic OLS installed. All components (operator, MCP server, Lightspeed) run on the same cluster. Hypershift/management-cluster deployments follow the same pattern on the management cluster.

**Not supported:** MicroShift and OpenShift Kubernetes Engine (agentic OLS not available). On SNO, MCP and agent sandbox pods add modest resource pressure (MCP requests ~128Mi/50m CPU).

## Security Model

**Principle:** The analysis agent is untrusted relative to cluster data; it only receives MCP tool access scoped to one diagnostic collection at a time.

| Layer | Phase 1 | Phase 2 |
|-------|---------|---------|
| Agent privileges | Sandboxed; no cluster-admin | Same |
| Data access | MCP read-only tools only | Same |
| MCP server cluster API | Disabled (`--cluster-provider disabled`) | Same |
| MCP pod identity | Dedicated `must-gather-mcp` SA, no RBAC, `automountServiceAccountToken: false` | Same |
| Network | In-namespace Service reachability | + optional hardening |
| Collection boundary | One path per analysis request; shared MCP serialized per active analysis | RBAC on DiagnosticArchive via token impersonation |

**Explicit non-capability:** Generic cross-collection queries (e.g. "fetch from all must-gathers on this volume") are not supported and must not be exposed to the agent.

When multiple MustGather CRs share a PVC, the operator serializes MCP access so a second analysis does not start until the first completes or is judged stale—preventing one agent from reading another collection's data mid-run.

## Implementation Notes (Phase 1)

- **MCP image:** `openshift-mcp-server` with `--toolsets openshift/mustgather`, `--cluster-provider disabled`, `--stateless`. Image reference configurable via `MCP_SERVER_IMAGE` on the operator until a catalog tag including `openshift/mustgather` is validated.
- **Shared MCP server:** One Deployment/Service per operator namespace; collections distinguished by path on the PVC.
- **Namespace:** MustGather CR, PVC, and MCP server must reside in the operator namespace in v1.
- **PVC lifecycle:** User-managed; operator does not create or delete PVCs.
- **Lightspeed detection:** Operator checks for Proposal CRD; skips gracefully if absent.
- **IntelliAide adapter:** Skills in `intelliaide-lightspeed-skills` bridge IntelliAide's filesystem-oriented pipeline to MCP tool calls via a client-side adapter (implementation detail outside this EP).
- **Analysis depth:** v1 runs High-priority pass only; Medium/Low deferred.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Lightspeed not installed | API discovery check; skip analysis; gather completes |
| MCP image missing `openshift/mustgather` toolset | Pin `MCP_SERVER_IMAGE`; verify toolset before setting operator default |
| Cross-collection data exposure on shared MCP | Collection path scoping in analysis request + serialized MCP access |
| Agentic API volatility | FeatureGate before Tech Preview |
| Hardcoded MCP URL (PoC) | Build URL from operator namespace at request-creation time (follow-up) |

### Drawbacks

- Using full `openshift-mcp-server` for one toolset may seem heavy, but avoids a custom server.
- Phase 1 uses hand-built Deployment/Service rather than MCPServer CR until upstream PVC support exists.

## Future Direction

Phase 1 uses a hand-built MCP Deployment/Service managed by must-gather-operator. The longer-term direction delegates MCP pod lifecycle to the **MCP Lifecycle Operator** via a declarative **`MCPServer` CR**, and introduces a **`DiagnosticArchive` CR** to record collection path and ownership (replacing implicit path resolution today).

**Confirmed blocker:** `MCPServer.spec.config.storage[].source.type` (developer-preview API) supports only `ConfigMap`, `Secret`, and `EmptyDir`—not `PersistentVolumeClaim`. Until upstream adds PVC storage, an MCPServer CR cannot express the mount this feature depends on. Phase 1 proceeds without waiting on this.

Post-v1 token impersonation (agent token forwarded; MCP authorizes per DiagnosticArchive) depends on DiagnosticArchive and stronger platform lifecycle—documented in Phase 2 above, not required for Phase 1 graduation.

## Alternatives (Not Implemented)

1. **Per-CR MCP server** — rejected: resource waste and lifecycle complexity vs. shared server with path isolation.
2. **Mount PVC directly in agent sandbox** — rejected: PVCs are namespace-scoped; sandbox runs in `openshift-lightspeed`.
3. **Custom lightweight MCP server** — rejected: `openshift-mcp-server` already provides production-quality `openshift/mustgather` tooling.
4. **Start MCP at operator boot** — rejected: PVC data does not exist until after gather completes.

## Open Questions

1. **Multi-PVC concurrent serving:** Is serialized access on one shared MCP sufficient for v1, or should Phase 2 invest in per-PVC MCP instances?
2. **User-supplied analysis query:** Should MustGather accept an optional query string to narrow RCA scope (vs. fixed comprehensive analysis)?
3. **MCP server idle lifecycle:** Should the shared MCP server scale down when no analysis is active?
4. **IntelliAide analysis depth:** Is High-priority-only acceptable for Tech Preview, or must all three passes run before GA?

## Test Plan

**Unit tests:** MCP server ensure logic (idempotency, PVC-swap guard), Proposal creation and path resolution, storage required when agentic analysis enabled.

**Integration tests:** MCP/Proposal created when enabled; skipped when disabled or Lightspeed absent; second MustGather defers when shared MCP is busy.

**End-to-end:** MustGather → gather → MCP → Proposal → IntelliAide → AnalysisResult. Negative: no storage when enabled; Lightspeed absent.

## Graduation Criteria

### Dev Preview → Tech Preview

- `agenticDebuggingEnabled` field in MustGather CRD
- Cluster FeatureGate implemented and documented
- End-to-end flow validated (High-priority IntelliAide pass)
- Single-PVC / serialized MCP access documented
- Graceful behavior when Lightspeed absent
- Proposal/AgenticRun CR must not be created until the MCP Deployment reports Ready and its Service has at least one endpoint

### Tech Preview → GA

- Feature gate removed for general availability
- Multi-collection scenarios tested; upgrade/downgrade validated
- Validated catalog MCP image with `openshift/mustgather`
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Upgrade / Downgrade Strategy

- **Upgrade:** Field defaults to `false`; existing CRs unchanged. MCP server created only when a new CR enables agentic analysis.
- **Downgrade:** Older operator ignores the field. Leftover MCP Deployment/Service may remain; manual cleanup acceptable. No data loss on PVC.

## Version Skew Strategy

Self-contained in must-gather-operator. Proposal creation uses dynamic GVR; missing or changed Lightspeed APIs fail gracefully without blocking gather. MCP server version is operator-configured only.

## Operational Aspects

- **SLI impact:** None for existing gather/upload workflows; optional field only.
- **Failure modes:** MCP or Proposal failures are logged; gather completes. Agent/sandbox failures are Lightspeed-owned.
- **Escalation:** must-gather-operator team (MCP, Proposal); Lightspeed team (sandbox, AnalysisResult).

## Support Procedures

- Operator logs: `failed to ensure MCP server`, `failed to create IntelliAide proposal`
- `oc get deployment must-gather-mcp -n <operator-ns>`
- `oc get proposal -n openshift-lightspeed`
- Disable: set field false on new CRs or turn off FeatureGate

## Infrastructure Needed

Implementation in `openshift/must-gather-operator`. MCP server image from `openshift/openshift-mcp-server`. IntelliAide skills in `openshift/intelliaide-lightspeed-skills`. No new operator subprojects required for Phase 1.
