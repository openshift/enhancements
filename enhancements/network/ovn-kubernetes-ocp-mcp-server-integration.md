---
title: ovn-kubernetes-ocp-mcp-server-integration
authors:
  - "@arkadeepsen"
reviewers:
  - "@tssurya, OVN-Kubernetes networking ownership and OVN/OVS troubleshooting tool semantics"
  - "@Cali0707, integration implementation in openshift-mcp-server"
  - "@matzew, integration implementation in openshift-mcp-server"
approvers:
  - "@tssurya"
api-approvers:
  - "None"
creation-date: 2026-05-08
last-updated: 2026-06-12
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/CORENET-7091
see-also:
  - https://github.com/ovn-kubernetes/ovn-kubernetes-mcp
  - https://github.com/containers/kubernetes-mcp-server
  - https://github.com/openshift/openshift-mcp-server
replaces:
  - NA
superseded-by:
  - NA
---

# OVN-Kubernetes MCP tools in OpenShift MCP server

## Summary

This enhancement proposes making the already-implemented OVN-Kubernetes MCP tools from [ovn-kubernetes-mcp](https://github.com/ovn-kubernetes/ovn-kubernetes-mcp) available **directly in** [openshift-mcp-server](https://github.com/openshift/openshift-mcp-server) by importing the same reusable packages and wiring execution through that server’s in-cluster primitives (pod exec, node-level debugging, and related paths). The intent is to reuse the upstream OVN troubleshooting implementations in ovn-kubernetes-mcp inside the OpenShift product MCP server, rather than forking or re-implementing equivalent tool logic.

## Motivation

OVN-Kubernetes operators and support engineers often need Northbound and Southbound database views (`ovn-nbctl`, `ovn-sbctl`, traces, logical flows), OVS bridge and OpenFlow inspection (`ovs-ofctl` and related helpers), host-oriented diagnostics, and packet or kernel-level capture workflows while investigating connectivity and routing. These tools are already implemented in ovn-kubernetes-mcp, but OpenShift users benefit from consuming them via a **single MCP server** that shares authentication, tool governance, and documentation with the rest of the platform troubleshooting surface.

The primary motivation for adding these tools to **openshift-mcp-server** is to give customers **a single MCP server** for OpenShift troubleshooting: the same server and configuration they use for broader Kubernetes and platform operations can also expose OVN-Kubernetes diagnostics, instead of **deploying and maintaining a separate MCP server** for OVN-Kubernetes, with its own endpoints, credentials, access policies, and day-two operations.

By integrating into the existing openshift-mcp-server, OpenShift avoids funding a parallel pipeline for a standalone OVN MCP offering with separate build and release pipelines, container image lifecycle and CVE response, supportability and documentation matrices, security review surfaces, and client onboarding for another server identity. OVN-Kubernetes tooling then inherits the same packaging, upgrade, and support expectations of openshift-mcp-server, which is materially cheaper to own than treating OVN-Kubernetes diagnostics as a second shipped product.

### User Stories

- As a cluster administrator or platform engineer, I want OVN-Kubernetes MCP troubleshooting tools in the same MCP server I already use for Kubernetes resources, so that I do not have to deploy, operate, or manage authentication for a second MCP server dedicated only to OVN-Kubernetes.
- As a support engineer, I want MCP clients to expose the full ovn-kubernetes-mcp troubleshooting surface that openshift-mcp-server imports—NB/SB inspection and related `ovn-*` workflows (including `get`, `lflow-list`, `trace` where those tools apply), OVS bridge and OpenFlow helpers, and **`kernel`** / **`network-tools`** host and capture tooling—so that assisted troubleshooting matches how other cluster operations are automated without switching servers or credentials mid-incident.
- As an OpenShift contributor, I want OVN-Kubernetes MCP logic to live in the ovn-kubernetes-mcp repository as reusable packages, so that behavior stays aligned with the upstream OVN-Kubernetes community.

### Goals

- Add an `ovn-kubernetes` toolset to openshift-mcp-server that reuses the existing OVN MCP tool implementations from ovn-kubernetes-mcp, rather than re-implementing equivalent functionality.
- Enable openshift-mcp-server to run in-cluster troubleshooting for this toolset: OVN/OVS commands via existing pod-exec into suitable pods, and **`kernel`** / **`network-tools`** flows via whatever node-level debugging or host access path those upstream handlers require, implemented **as part of this same integration** (expect refactoring in **ovn-kubernetes-mcp** and **kubernetes-mcp-server**/**openshift-mcp-server** so execution is delegated cleanly to the host).
- Import the full handler sets from ovn-kubernetes-mcp **`ovn`**, **`ovs`**, **`kernel`**, and **`network-tools`** into openshift-mcp-server’s OVN-Kubernetes tool registration, subject only to exclusions in Non-Goals.
- Ship the toolset to OpenShift users in openshift-mcp-server product builds (versioning and packaging follow that repository’s release process).

### Non-Goals

- Full parity with every tool category shipped by the standalone ovn-kubernetes-mcp binary (for example must-gather, sosreport) where those require separate dependencies, images, or product workflows outside this MCP integration.
- New Kubernetes or OpenShift APIs, CRDs, operators, or cluster-side agents solely for this feature.
- Replacing existing CLI-based troubleshooting; MCP tools are an additional interface.

## Proposal

### Workflow Description

1. An operator configures MCP clients (for example Cursor, other MCP hosts) to use openshift-mcp-server with a kubeconfig that can reach the target cluster and satisfies RBAC for pod read and pod exec where policies allow.
2. The user or automated agent invokes OVN/OVS troubleshooting tools with the Kubernetes namespace and name of a pod that runs OVN utilities (for example an `ovnkube-node` pod), and selects the desired database or command parameters (for example `nbdb` or `sbdb` where applicable).
3. The toolset handler invokes the imported OVN/OVS tool implementation and delegates in-cluster command execution to openshift-mcp-server’s existing pod-exec capability.
4. The server runs the appropriate `ovn-nbctl` / `ovn-sbctl` / `ovs-*` command inside the target pod; stdout/stderr are returned through the MCP tool response.
5. Results are returned to the MCP client as structured or textual output according to the tool schema.

For **`kernel`** and **`network-tools`** tools, the operator or agent selects parameters appropriate to those handlers (for example node or debug-pod targeting as defined by the tool schema); openshift-mcp-server carries out the node-level execution path implemented for this integration, analogous to step 3–5 for pod-exec.

### API Extensions

None. This work adds MCP tools only and does not extend the OpenShift or Kubernetes API surface.

### Topology Considerations

The OVN-Kubernetes toolset delegates to openshift-mcp-server's existing pod-exec and node-debug paths and does not add cluster-access mechanisms or deployment models beyond what that server already supports. The subsections below document topology-specific constraints on **which cluster API** the server must reach when running these tools—constraints inherited from those execution paths, not new logic in the OVN-K tool implementations.

#### Hypershift / Hosted Control Planes

The OVN-K toolset inherits openshift-mcp-server's existing cluster-targeting behavior and does not introduce HyperShift-specific logic.

In HyperShift, OVN-K components are split across clusters: **ovnkube-node** pods (per-node OVN NB/SB databases, northd, and ovn-controller) run on hosted-cluster worker nodes, while a lightweight **ovnkube-control-plane** runs on the management cluster. All troubleshooting targets for this toolset—pod exec into ovnkube-node and node-debug for `kernel` / `network-tools`—therefore require the MCP server to reach the **hosted cluster** API, not the management cluster; nothing in this toolset targets the management-cluster control plane.

Operators satisfy that requirement the same way as for any openshift-mcp-server toolset that acts on workload-cluster resources: configure the server against the hosted cluster (for example in-cluster on the hosted cluster, or a kubeconfig whose context points at the hosted cluster when invoking tools).

#### Standalone Clusters

On standalone clusters, the OVN-Kubernetes toolset executes against pods and nodes on the same cluster the MCP server's API client reaches.

#### Single-node Deployments or MicroShift

Relevant wherever OVN-Kubernetes runs and the user can identify a suitable pod; resource footprint is limited to occasional exec sessions initiated by MCP clients. MicroShift applicability follows whether openshift-mcp-server is supported for that distribution (product-specific packaging is outside this design).

#### OpenShift Kubernetes Engine

No dependency on OCP-only APIs for this integration. Tool availability still depends on shipping openshift-mcp-server builds that include the `ovn-kubernetes` toolset and on RBAC policy configured by the operator.

### Implementation Details/Notes/Constraints

**Importing upstream tools into openshift-mcp-server.** The OVN troubleshooting MCP tools already exist in ovn-kubernetes-mcp. The integration approach for openshift-mcp-server is to add an `ovn-kubernetes` toolset that reuses those implementations as imported packages and exposes them through openshift-mcp-server’s tool registration.

**Command execution strategy.** OVN/OVS tools run commands inside OVN-Kubernetes pods via openshift-mcp-server’s pod exec. **`kernel`** and **`network-tools`** handlers use the node-level execution contract wired up in the same integration (for example debug pod or node-targeted exec, as the upstream packages require). Imported libraries should delegate all cluster I/O to openshift-mcp-server rather than opening separate Kubernetes client connections. Expect **refactoring in ovn-kubernetes-mcp and kubernetes-mcp-server/openshift-mcp-server** so each category uses a clear, single host-supplied execution path per invocation.

**Scope.** All troubleshooting tools under ovn-kubernetes-mcp **`ovn`**, **`ovs`**, **`kernel`**, and **`network-tools`** belong to this effort (NB/SB inspection, logical flows, OVN trace, OVS bridge and OpenFlow helpers, kernel-oriented diagnostics, and **`network-tools`**-style capture where applicable). Other ovn-kubernetes-mcp surfaces (must-gather, sosreport, and similar) remain out of scope unless separately agreed; see Non-Goals.

**Split of work:** openshift-mcp-server decides how each capability is exposed to MCP users (tool names and parameters). ovn-kubernetes-mcp keeps handler logic that validates inputs, builds command lines, and defines execution contracts; openshift-mcp-server integrates by calling those libraries and supplying pod exec, node-level debugging, or other supported cluster operations against the target cluster.

```text
      +------------------------------------------------------------+
      |                   openshift-mcp-server                     |
      +------------------------------------------------------------+
      |                                                            |
      |     +-----------------------------------------------+      |
      |     |        ovn-kubernetes MCP tool handlers       |      |
      |     +-----------+-----------------------------------+      |
      |                 |                ^                         |
      |          calls  |                |  parsed response        |
      |                 v                |                         |
      |        +-------------------------+-------------------+     |
      |        |  ovn-kubernetes-mcp (imported Go packages)  |     |
      |        +---------------------------------------------+     |
      |        |            tool handler logic               |     |
      |        +-+---------+-------+-----------------------+-+     |
      |          | OVN/OVS |       | kernel / network-tools|       |
      |          +-+-------+       +----------+------------+       |
      |            |   ^                      |    ^               |
      |      calls |   | stdout/        calls |    | stdout/       |
      |            v   | stderr               v    | stderr        |
      |       +--------+----+              +-------+-----+         |
      |       |   pod-exec  |              | node-debug  |         |
      |       | (Kubernetes |              | (OpenShift  |         |
      |       |   client)   |              |   client)   |         |
      |       +-------------+              +-------------+         |
      |                                                            |
      +------------------------------------------------------------+
```

### Risks and Mitigations

- **RBAC and privilege:** Pod exec and node-level debugging are sensitive. Mitigation: reuse openshift-mcp-server permission models for `pods/exec`, node-scoped operations, and any debug-pod workflows; document required roles; keep tools read-only where possible.
- **Data sensitivity:** OVN database dumps can expose topology and workloads. Mitigation: treat tool output like other diagnostic data; warn in tool descriptions; rely on cluster RBAC and MCP deployment guidance.
- **Wrong pod or namespace:** Users may target non-OVN pods. Mitigation: clear parameter descriptions; surface stderr from exec failures.

### Drawbacks

- Requires maintaining a dependency from openshift-mcp-server on ovn-kubernetes-mcp (version alignment and API stability).
- The integrated surface spans pod exec and node-level paths; a serious failure or unclear errors in one category can erode trust in the whole toolset until mitigations and docs catch up.

## Alternatives (Not Implemented)

- **Add the OVN toolset to kubernetes-mcp-server first, then rely on downstream sync into openshift-mcp-server:** Not chosen for this enhancement because landing the integration directly in openshift-mcp-server to ship on product cadence avoids gating on upstream kubernetes-mcp-server acceptance, release, and fork sync timing. Several factors also argue against landing these tools in upstream kubernetes-mcp-server. The tools which are part of `kernel` and `network-tools` in OVN-Kubernetes MCP server depend on node-debug functionality, and currently there are no immediate plans of adding node-debug to the kubernetes-mcp-server. In addition, upstream kubernetes-mcp-server is likely to remain CNI-agnostic, while these tools are a strong fit for openshift-mcp-server because most OpenShift customers use OVN-Kubernetes as the CNI. Finally, [ovn-kubernetes-mcp](https://github.com/ovn-kubernetes/ovn-kubernetes-mcp) already exists as a dedicated upstream project, so duplicating the same tools in kubernetes-mcp-server would create two upstream homes for the same surface, which is undesirable. The import-and-delegate pattern remains the same; a future upstream integration could still reduce long-term duplication if both codebases converge.
- **Fork OVN tool logic into openshift-mcp-server only:** Rejected because it duplicates effort and diverges from upstream ovn-kubernetes-mcp.
- **Keep only the standalone ovn-kubernetes-mcp binary for OpenShift:** Rejected because it splits credentials, RBAC documentation, and user experience from the unified MCP server.
- **Have ovn-kubernetes-mcp open its own REST/exec connections inside openshift-mcp-server:** Rejected because it duplicates openshift-mcp-server’s existing **pod exec** (Kubernetes client) and **node debug** (OpenShift client) behavior for in-cluster command execution, and splits security review surfaces for both pod-scoped and node-level troubleshooting paths.

## Open Questions

- How to structure mcpchecker suites or task labels so OVN/OVS, **`kernel`**, and **`network-tools`** coverage stays maintainable under openshift-mcp-server’s pass-rate gates (or equivalent CI evaluation), given differing cluster prerequisites?
- Whether openshift-mcp-server needs product-specific enablement flags or documentation for clusters where pod exec is restricted.

## Test Plan

- **Unit tests:** Ensure imported tool implementations can be exercised without requiring a live cluster (for example by substituting test doubles for in-cluster command execution and validating command construction and output handling), including **`kernel`** and **`network-tools`** handlers where feasible.
- **Integration:** Validate the `ovn-kubernetes` toolset end to end in openshift-mcp-server: pod-exec paths for OVN/OVS, and node-level paths for **`kernel`** / **`network-tools`** as implemented for this integration.
- **Manual:** Run MCP tool calls against a cluster with OVN-Kubernetes installed, verifying OVN/OVS output for a known `ovnkube-node` pod and representative **`kernel`** / **`network-tools`** scenarios supported by the cluster.

### Tool acceptance mechanism (mcpchecker evals)

openshift-mcp-server should evaluate the new toolset with the [mcpchecker](https://github.com/mcpchecker/mcpchecker) framework: scenario tasks under `evals/tasks/**`, suite-based selection with labels (for example `metadata.labels.suite: <suite-name>`), and a `label-selector` per suite in CI. Document concrete pass-rate thresholds (for example **≥ 80%** task pass rate and **≥ 80%** assertion pass rate if matching common mcpchecker gates), workflow filenames, and result baselines in openshift-mcp-server when the eval suite is added.

For OVN-Kubernetes tool acceptance, add a dedicated suite (for example `suite: ovn-kubernetes`) with tasks that exercise imported tools—including OVN/OVS, **`kernel`**, and **`network-tools`** where the eval environment supports them—against a cluster where OVN-Kubernetes is present. Task metadata may use labels or sub-suites so CI can select or skip scenarios with stricter prerequisites while keeping pass-rate expectations consistent with other toolsets evaluated in openshift-mcp-server.

## Graduation Criteria

This enhancement targets **General Availability** of the integrated OVN-Kubernetes MCP toolset in **OpenShift 5.0**, delivered through **openshift-mcp-server** product builds. **Dev Preview** and **Tech Preview** milestones preceding 5.0 must satisfy the criteria in the following subsections. At each stage, the feature is expected to demonstrate end-to-end validation on representative topologies, published RBAC guidance for administrators (including pod exec and node debug), and stakeholder agreement on the supported tool inventory.

### Dev Preview -> Tech Preview

- Imported OVN-Kubernetes MCP tools (OVN/OVS, **`kernel`**, **`network-tools`**) usable end to end against representative clusters where RBAC and cluster policy allow the required pod and node-level operations.
- Clear documentation for namespace/pod selection, node or debug-pod selection where applicable, and permissions.
- mcpchecker evaluation pass rate meets **≥ 80%** for task pass rate (and assertions), aligning with openshift-mcp-server’s chosen CI gate.

### Tech Preview -> GA (target: OpenShift 5.0)

- Agreed supported tool list with networking and support stakeholders, covering the full integrated surface (OVN/OVS, **`kernel`**, **`network-tools`**) and documented limitations where cluster policy blocks certain operations.
- Sufficient soak time and feedback; openshift-docs updates if the feature is user-facing in product documentation.
- mcpchecker evaluation pass rate meets **≥ 95%** for task pass rate (and assertions) for the `ovn-kubernetes` suite.
- The OVN-Kubernetes eval suite is stable across representative topologies and does not regress existing suites (diff vs baseline tracked under `evals/results/*-latest.json` or the equivalent path openshift-mcp-server uses for eval baselines).

### Removing a deprecated feature

- Announce deprecation of overlapping standalone-only workflows if any are superseded; maintain compatibility until a documented removal release.

## Upgrade / Downgrade Strategy

No cluster upgrade steps are required: changes are confined to MCP server binaries and their dependencies. Upgrading or downgrading the MCP server image or package restores previous tool availability if the toolset is absent in an older build.

## Version Skew Strategy

**openshift-mcp-server** must remain compatible with the cluster APIs it uses for in-cluster execution. **OVN/OVS** tools depend on Kubernetes **pod get** and **pods/exec** subresources, as today. **`kernel`** and **`network-tools`** tools depend on whatever **node debug** (and related node- or debug-workload) APIs the OpenShift client path requires in the target release. Operators should run an MCP server build matched to the OpenShift (or Kubernetes) version of the cluster under troubleshooting; mixing a much newer or older MCP server against the API server is unsupported.

No new coordinated skew is introduced between MCP server releases and **kubelet** or **CNI** beyond ordinary OpenShift expectations: command execution still runs inside existing OVN-Kubernetes pods or on nodes via product-supported debug mechanisms, not via a new in-cluster agent shipped by this enhancement.

## Operational Aspects of API Extensions

Not applicable. There are no CRDs, admission webhooks, aggregated API servers, or finalizers introduced by this enhancement.

## Support Procedures

- **Symptoms (pod exec / OVN-OVS):** Tool errors such as `Forbidden` or `cannot exec into a container` usually indicate RBAC or pod state issues. Verify RoleBindings (or ClusterRoleBindings) that grant `pods/exec` (and `pods/get` where required), confirm the target pod is **Running**, and that the namespace and pod name match an OVN-Kubernetes utility pod (for example `ovnkube-node`). Wrong targets often show command or `ovn-*` / `ovs-*` failures in stderr rather than API `Forbidden`.

- **Symptoms (node debug / `kernel` / `network-tools`):** Failures for node-level tools often appear as `Forbidden` on node or debug-related APIs, timeouts waiting for a debug pod, or errors that the node is not schedulable or not found. Verify the kubeconfig context reaches the intended cluster (management versus hosted, for HyperShift). Confirm RBAC and any product policy allow **node debug** workflows openshift-mcp-server uses (OpenShift client / node-debug path). Check that the named node exists, is ready, and that cluster policy permits ephemeral debug workloads on that node if the handler creates or uses them. Restricted environments (pod exec disabled, debug pods blocked, or SCC constraints) may block only this category while OVN/OVS pod-exec tools still work.

- **Logs:** API server **audit logs** may record `pods/exec` and node- or debug-related API calls according to cluster policy. **openshift-mcp-server** logs should show handler errors, including which execution path failed (pod exec versus node debug). For node-debug failures, correlate MCP server timestamps with events on the target node and any debug pod namespace the integration uses.

- **Disable:** Disable or unregister the `ovn-kubernetes` toolset in MCP deployment configuration (exact mechanism depends on openshift-mcp-server packaging); no cluster-side toggle is defined here. Disabling the whole MCP server removes all toolsets, including OVN-Kubernetes; there is no per-path cluster toggle for pod exec versus node debug in this enhancement.

## Infrastructure Needed [optional]

No new production or platform infrastructure is required to ship or run this feature. It does not introduce cluster-side agents, operators, CRDs, or cloud resources; operators continue to run **openshift-mcp-server** against an existing cluster API with appropriate RBAC for pod exec and node debug.

For **implementation and CI**, openshift-mcp-server (and ovn-kubernetes-mcp) need the same class of resources already used to validate in-cluster MCP tools: jobs or environments that can reach a cluster with **OVN-Kubernetes** installed, suitable `ovnkube-node` (or equivalent) targets for pod-exec scenarios, and policy that allows **node debug** where **`kernel`** / **`network-tools`** evals or integration tests run. That is an extension of openshift-mcp-server’s existing test and mcpchecker pipelines, not a separate infrastructure program.
