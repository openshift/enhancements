---

## title: must-gather-agentic-debugging

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
last-updated: 2026-07-16
status: provisional
tracking-link:
- [https://redhat.atlassian.net/browse/OAPE-688](https://redhat.atlassian.net/browse/OAPE-688)

# Must-Gather Agentic Debugging via MCP Server

## Summary

Integrate the must-gather-operator with OpenShift Lightspeed to enable automated root-cause analysis (RCA) on must-gather diagnostic bundles. When a user sets `agenticDebuggingEnabled: true` on a MustGather CR, the operator - after a successful gather Job - deploys a shared MCP server (using the `openshift-mcp-server` image with the `openshift/mustgather` toolset) that mounts the same PVC, and creates a Lightspeed Proposal CR. The Lightspeed agentic platform spawns a sandboxed agent that connects to the MCP server, fetches diagnostic data through structured MCP tool calls, and runs the IntelliAide RCA pipeline - producing a structured AnalysisResult without requiring cluster-admin privileges for the analysis agent.

## Motivation

When customers experience cluster issues, they create MustGather CRs to collect diagnostic data. Today this pipeline ends at data collection: the bundle is written to a PVC or uploaded via SFTP, and a human SRE must manually download, navigate, and analyze the data. This is slow, error-prone, and requires deep OpenShift expertise.

IntelliAide is a multi-stage RCA pipeline that can automate this analysis. It will run in a sandboxed agent pod in `openshift-lightspeed` with limited permissions. It cannot directly access the must-gather data on a PVC in the `must-gather-operator` namespace (PVCs are namespace-scoped), and giving it cluster-admin to re-collect data defeats the purpose of a security-constrained sandbox.

A Model Context Protocol (MCP) server bridges this gap: it mounts the PVC in the operator namespace and exposes the data as structured, read-only tool calls over HTTP - a protocol the Lightspeed agent sandbox already supports.

### User Stories

- As an OpenShift cluster administrator, I want to set `agenticDebuggingEnabled: true` on my MustGather CR so that after diagnostic data is collected, an AI agent automatically analyzes it and produces a root-cause analysis report - without me having to manually sift through logs and events.
- As an SRE at a large enterprise, I want the analysis agent to never require cluster-admin privileges so that automated diagnostics do not introduce security risks or privilege escalation.
- As a Red Hat support engineer, I want RCA results surfaced as an AnalysisResult CR on the cluster so that I can programmatically retrieve them or integrate them into case management workflows.
- As a cluster administrator managing multiple must-gather collections on the same PVC, I want each collection analyzed independently so that different failure scenarios are diagnosed without interference.
- As an operator administrator, I want the agentic debugging feature gated behind a TechPreview flag so that I can control rollout and opt in only when my cluster has the Lightspeed agentic platform installed.

### Goals

1. After a successful must-gather collection with `agenticDebuggingEnabled: true`, the agent automatically runs IntelliAide RCA - no human intervention required
2. The agent never needs cluster-admin privileges - it reads data only through MCP
3. A single shared MCP server serves multiple MustGather collections via subPath isolation on the PVC
4. The feature is opt-in (`agenticDebuggingEnabled` defaults to `false`) and gated behind TechPreview for initial releases
5. No changes required to the Lightspeed agentic platform - we use its existing Proposal/AnalysisResult APIs
6. The operator is idempotent - concurrent MustGather completions do not race on MCP server creation
7. The user retains full responsibility for PVC lifecycle management (creation, sizing, cleanup)

### Non-Goals

1. Changes to the Lightspeed agentic operator, sandbox, or agent behavior
2. Changes to the openshift-mcp-server toolset itself
3. Remediation execution (we produce an AnalysisResult; execution is a Lightspeed concern)
4. Multi-namespace PVC access (MCP server runs in operator namespace, mounts PVC from same namespace only)
5. Custom must-gather images for user workload namespaces (separate concern of `origin-must-gather`)
6. Multiple PVC support in the initial implementation (single PVC per operator namespace is the initial scope)

## Proposal

Add an `agenticDebuggingEnabled` boolean field to the MustGather CR spec. When set to `true` (and storage is configured), the operator performs two additional steps after a successful gather Job completes:

1. **Ensure shared MCP server** - creates (or verifies existence of) a Deployment and Service for the `openshift-mcp-server` container image, configured with only the `openshift/mustgather` toolset and mounting the user's PVC at `/data`.
2. **Create Lightspeed Proposal** - creates a `Proposal` CR in the `openshift-lightspeed` namespace containing the MCP server URL, IntelliAide skill configuration, and a prompt instructing the agent to run the IntelliAide pipeline against the specific must-gather collection path.

The MCP server container image is the `openshift-mcp-server` published via Red Hat's ART/Konflux pipeline. Only the `openshift/mustgather` toolset is enabled to keep the footprint minimal.

### Workflow Description

**Cluster administrator** is the human user responsible for creating MustGather CRs.

**Must-gather-operator** is the controller managing the MustGather lifecycle.

**Lightspeed agentic operator** is the controller managing Proposals and agent sandboxes.

#### Primary Flow: Gather + Analyze

1. The cluster administrator creates a PVC in the `must-gather-operator` namespace.
2. The cluster administrator creates a MustGather CR with `agenticDebuggingEnabled: true` and `storage.persistentVolume.claim.name` referencing the PVC.
3. CRD validation verifies that `storage` is configured (rejects the CR otherwise).
4. The must-gather-operator creates a gather Job. The Job runs `oc adm must-gather` and writes output to the PVC under `{subPath}/{podName}/`.
5. The gather Job completes successfully. The operator calls `handleJobCompletion()`.
6. The operator checks `agenticDebuggingEnabled == true`.
7. The operator validates that OpenShift Lightspeed is installed (checks API discovery for the Proposal CRD). If not installed, logs and skips - does not block MustGather completion.
8. `ensureMCPServer()` creates (or confirms existence of) the `must-gather-mcp` Deployment + Service. The Deployment mounts the PVC at `/data` as read-only.
9. `resolveMustGatherDataPath()` determines the specific collection path: `{subPath}/{podName}`.
10. `createIntelliAideProposal()` creates a Proposal CR in `openshift-lightspeed` with the MCP server URL and IntelliAide instructions.
11. The Lightspeed agentic operator processes the Proposal, spawns a sandboxed agent pod.
12. The agent connects to `http://must-gather-mcp.must-gather-operator.svc:8080/mcp`.
13. The agent calls `mustgather_use("/data/{subPath}/{podName}")` to select the archive.
14. The agent runs the IntelliAide pipeline (extract → select → fetch via MCP → analyze → RCA).
15. The agent writes an AnalysisResult CR.
16. The cluster administrator retrieves the AnalysisResult.

#### Error Cases

- **Lightspeed not installed**: Operator logs info and skips Proposal creation. MustGather completes normally.
- **MCP server creation fails**: Operator logs error and continues (best-effort). Proposal may be created but agent cannot connect.
- **Gather Job fails/times out**: No MCP or Proposal flow is triggered.
- **Agent sandbox crashes** (missing LLMProvider, etc.): Lightspeed operator concern; no impact on must-gather-operator.

### API Extensions

One new field added to the existing `MustGatherSpec` in the `mustgathers.operator.openshift.io` CRD:

```go
type MustGatherSpec struct {
    // ... existing fields ...

    // AgenticDebuggingEnabled, when true, automatically creates a Lightspeed
    // Proposal CR for agentic root-cause analysis after a successful
    // must-gather collection. Requires storage to be configured (PVC) so the
    // shared MCP server can serve the collected data to the analysis agent.
    // +kubebuilder:default:=false
    // +optional
    AgenticDebuggingEnabled *bool `json:"agenticDebuggingEnabled,omitempty"`
}
```

CRD validation rule (CEL):

```
rule: "!(has(self.agenticDebuggingEnabled) && self.agenticDebuggingEnabled && !has(self.storage))"
message: "storage is required when agenticDebuggingEnabled is true (MCP server needs PVC-backed data)"
```

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

No new CRDs are introduced. The operator creates resources in other namespaces (Proposal CR in `openshift-lightspeed`) using an unstructured client with dynamic GVR.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations. The must-gather-operator runs in the management cluster. The MCP server and Lightspeed components also run in the management cluster. No guest cluster components are affected.

#### Standalone Clusters

This is the primary target topology. All components (operator, MCP server, Lightspeed) run on the same cluster.

#### Single-node Deployments or MicroShift

On SNO, the MCP server pod and agent sandbox pod add resource pressure. The MCP server requests are minimal (128Mi memory, 50m CPU). The agent sandbox resource usage depends on the Lightspeed configuration. This should be documented as a consideration for resource-constrained environments.

MicroShift: Not applicable. OpenShift Lightspeed is not available on MicroShift.

#### OpenShift Kubernetes Engine

This feature depends on OpenShift Lightspeed (agentic), which is not part of OKE. The `agenticDebuggingEnabled` field can be set but will be a no-op if Lightspeed is not installed (the operator gracefully skips).

### Implementation Details/Notes/Constraints

#### MCP Server Image

The MCP server uses the `openshift-mcp-server` container image published via Red Hat's build pipelines. Two image sources are available:


| Environment                  | Image                                                              | Notes                                                                                                                                           |
| ---------------------------- | ------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Production (Red Hat catalog) | `registry.redhat.io/openshift-mcp-beta/openshift-mcp-server-rhel9` | Certified, validated via Red Hat pipelines. Currently in beta.                                                                                  |
| CI (latest builds)           | `registry.ci.openshift.org/ocp/5.0:openshift-mcp-server`           | Requires CI registry credentials ([docs](https://docs.ci.openshift.org/how-tos/use-registries-in-build-farm/#summary-of-available-registries)). |


The image reference is configurable via the `MCP_SERVER_IMAGE` environment variable on the operator Deployment. The default will be set to the Red Hat catalog image once it moves out of beta.

The MCP server is configured with:

```
args: ["--port", "8080", "--toolsets", "openshift/mustgather",
       "--cluster-provider", "disabled", "--stateless"]
```

- `--toolsets openshift/mustgather` limits the exposed toolset to only must-gather analysis, keeping the footprint minimal.
- `--cluster-provider disabled` ensures no live cluster API queries.
- `--stateless` prepares for future stateless operation.

#### Shared MCP Server Design

A single MCP server Deployment (`must-gather-mcp`) serves all MustGather collections on the same PVC. Each collection is identified by its path (`{subPath}/{podName}`), which is passed to the agent via the Proposal prompt.

The initial implementation supports a single PVC. Multiple PVCs with multiple MustGather CRs referencing different PVCs is deferred (see Open Questions). The shared server approach avoids resource waste from per-CR server pods.

#### Idempotency and Concurrency

`ensureMCPServer()` uses a get-then-create pattern with explicit `AlreadyExists` handling. If two MustGather Jobs complete simultaneously, both reconcilers attempt to create the Deployment - the second receives `AlreadyExists` and treats it as success.

#### Feature Gating

For the initial release, this feature will be gated behind a TechPreview mechanism. The operator will check a feature gate (environment variable or cluster FeatureGate resource) before processing `agenticDebuggingEnabled: true`. If the gate is off, the field is ignored.

#### Lightspeed Detection

The operator detects OpenShift Lightspeed presence by querying API discovery for the Proposal CRD (`agentic.openshift.io/v1alpha1`). If the CRD is not found, the operator skips Proposal creation gracefully. This avoids hard coupling and allows the must-gather-operator to function normally on clusters without Lightspeed.

#### PVC Lifecycle

The operator does not create or delete PVCs. The user is responsible for PVC creation before creating a MustGather CR and cleanup after analysis is complete. This is consistent with the existing `storage` behavior in the operator.

### Risks and Mitigations


| Risk                                                                   | Impact                                          | Mitigation                                                                                                                       |
| ---------------------------------------------------------------------- | ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Lightspeed not installed but user sets `agenticDebuggingEnabled: true` | Proposal creation fails silently                | Operator checks API discovery and logs info message; MustGather still completes normally                                         |
| MCP server image not pullable (registry auth, image not published)     | MCP pod in ImagePullBackOff                     | Image confirmed: `registry.redhat.io/openshift-mcp-beta/openshift-mcp-server-rhel9`; configurable via `MCP_SERVER_IMAGE` env var |
| Concurrent MustGather completions race on MCP Deployment creation      | Transient AlreadyExists errors                  | `ensureMCPServer()` explicitly handles AlreadyExists as success                                                                  |
| origin-must-gather skips user namespaces                               | User workload failures not in bundle            | Document limitation; support `imageStreamRef` for custom gather images                                                           |
| openshift-mcp-server footprint is large for this use case              | Over-engineering concern                        | Only enable the `openshift/mustgather` toolset; consider a thinner alternative if footprint is unacceptable                      |
| MCP server PVC can only mount from operator namespace                  | Cannot serve data from PVCs in other namespaces | Acceptable for initial design; document as limitation                                                                            |


### Drawbacks

- The `openshift-mcp-server` is a full-featured MCP server; using it only for the `openshift/mustgather` toolset may be considered over-engineering. However, it avoids writing and maintaining a custom server.
- The temp cache in the agent sandbox duplicates some data from the PVC. Future optimization via MCP resource URIs or IntelliAide changes could eliminate this.

## Alternatives (Not Implemented)

### Alternative 1: Per-CR MCP Server (one pod per MustGather)

Each MustGather CR gets its own MCP server pod.

Resource waste (most collections are analyzed once then deleted), complex lifecycle management (who deletes the per-CR MCP pod?), no benefit over a shared server with subPath isolation.

### Alternative 2: Mount PVC Directly in Agent Sandbox

Skip the MCP server; mount the PVC directly into the agent sandbox pod.

PVCs are namespace-scoped - the PVC lives in `must-gather-operator`, the sandbox runs in `openshift-lightspeed`. Cross-namespace PVC mounts are not supported in Kubernetes.

### Alternative 3: Build a Custom Lightweight MCP Server

Write a minimal Go service that only exposes must-gather file access.

The `openshift-mcp-server` already has a production-quality `openshift/mustgather` toolset with proper archive parsing, namespace queries, pod log extraction, and event filtering. The selected-toolset approach (`--toolsets openshift/mustgather`) keeps the deployment lightweight without reimplementing.

### Alternative 5: Trigger MCP Server at Operator Boot

Start the MCP server when the operator starts, not lazily on Job completion.

The PVC and subPath don't exist until a MustGather CR with storage is reconciled. Lazy creation ensures data exists before the server starts.

## Open Questions

1. **Multiple PVCs / Multiple MustGather CRs**: If different MustGather CRs reference different PVCs, the shared MCP server can only mount one PVC at a time. Should we support PVC hot-swapping, multiple MCP Deployments, or mandate a single PVC? Initial approach: single PVC support only. Multiple PVCs deferred to a future iteration.
2. **Feature gate mechanism**: Should the TechPreview gate be an environment variable on the operator Deployment, or should it integrate with the cluster-level `FeatureGate` resource?
3. **MCP server footprint**: Is the openshift-mcp-server image acceptable for this use case, or should we consider a thinner alternative? Can the `openshift/mustgather` toolset will be enabled to minimize footprint.

## Test Plan

**Unit tests:**

- `mcp_server.go`: Deployment/Service creation, idempotent handling of AlreadyExists, PVC claim name update logic
- `proposal.go`: Proposal CR structure validation, dynamic path resolution, graceful Lightspeed-not-installed handling
- `mustgather_types.go`: CRD validation - storage required when `agenticDebuggingEnabled` is true

**Integration tests:**

- Operator creates MCP Deployment + Service when MustGather CR has `agenticDebuggingEnabled: true` and storage configured
- Operator skips MCP/Proposal when `agenticDebuggingEnabled: false`
- Operator skips Proposal creation when Lightspeed CRD is not present

**End-to-End tests:**

- MustGather CR → gather Job → PVC data → MCP server running → Proposal created → agent sandbox launched → MCP tools called → IntelliAide pipeline executes → AnalysisResult produced
- Negative: MustGather CR without storage + `agenticDebuggingEnabled: true` is rejected at admission
- Negative: Lightspeed not installed - MustGather completes, no Proposal created

## Graduation Criteria

### Dev Preview -> Tech Preview

- `agenticDebuggingEnabled` field available in MustGather CRD
- Feature gated behind TechPreview flag
- End-to-end flow works: gather → MCP server → Proposal → agent → AnalysisResult
- Single PVC support validated
- Operator gracefully handles missing Lightspeed
- Documentation of prerequisites and limitations

### Tech Preview -> GA

- Feature gate removed; `agenticDebuggingEnabled` is generally available
- Multiple PVC / multiple CR scenarios tested and documented
- Upgrade/downgrade tested
- Performance benchmarked (time from Job completion to AnalysisResult)
- User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- MCP server image published and validated via official pipelines
- Feedback from users incorporated

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

- **Upgrade**: Existing MustGather CRs continue to work unchanged. The `agenticDebuggingEnabled` field defaults to `false` (disabled), so no behavioral change occurs for existing users. The MCP server Deployment is only created when a new CR with `agenticDebuggingEnabled: true` is reconciled.
- **Downgrade**:
  - If only the operator is downgraded (CRD still includes `agenticDebuggingEnabled`), the older operator version ignores the field and collection proceeds without analysis.
  - If the CRD is also downgraded, the field may be pruned from existing CRs. No data migration needed.
  - The MCP server Deployment/Service created by the newer operator will remain but serve no new requests. Manual cleanup is acceptable.
  - No data loss occurs; must-gather data on the PVC is unaffected.

## Version Skew Strategy

The agentic debugging feature is self-contained within the must-gather-operator. The only external interaction is creating a Proposal CR in `openshift-lightspeed` - this uses an unstructured client with a dynamic GVR, so version skew with the Lightspeed operator is handled gracefully (if the CRD doesn't exist or has a different shape, creation fails and the operator logs the error without blocking MustGather completion).

The MCP server is deployed by the operator and runs the same version as configured in the operator's environment. No coordination with other component versions is required.

## Operational Aspects of API Extensions

The `agenticDebuggingEnabled` field is an optional addition to the existing `mustgathers.operator.openshift.io` CRD. No new webhooks, finalizers, aggregated API servers, or additional CRDs are introduced.

- **Impact on existing SLIs**: None. The field is optional and only triggers additional behavior on newly created CRs that explicitly enable it. Existing CRs and the operator's core gather/upload functionality are unaffected.
- **Failure modes**:
  - MCP Deployment creation fails → logged, MustGather still completes
  - Proposal creation fails → logged, MustGather still completes
  - MCP server pod crashes → Proposal exists but agent cannot connect; Lightspeed operator handles timeout
  - Agent sandbox fails → AnalysisResult not produced; no impact on must-gather-operator
- **Escalation teams**: Must-gather operator team for MCP server and Proposal creation issues. Lightspeed team for agent sandbox and AnalysisResult issues.

## Support Procedures

- **Detecting issues**:
  - Check operator logs for `"failed to ensure MCP server"` or `"failed to create IntelliAide proposal"` messages
  - Check `oc get deployment must-gather-mcp -n must-gather-operator` - should be `1/1 Ready` after first agentic MustGather
  - Check `oc get proposal -n openshift-lightspeed` for the Proposal corresponding to the MustGather name
  - Check agent sandbox pod logs in `openshift-lightspeed` for MCP connection errors
- **Disabling**: Set `agenticDebuggingEnabled: false` on new CRs, or remove the TechPreview feature gate to disable cluster-wide. The MCP server Deployment can be manually deleted if no longer needed.
- **Consequences of disabling**: Must-gather collection continues normally. No RCA is produced. No data loss.
- **Graceful failure**: All agentic debugging steps are best-effort. Failures in MCP server creation or Proposal creation do not block MustGather completion or resource cleanup.

## Infrastructure Needed

No new subprojects or repos are required. The implementation lives in the existing `openshift/must-gather-operator` repository. The MCP server uses the existing `openshift/openshift-mcp-server` container image.