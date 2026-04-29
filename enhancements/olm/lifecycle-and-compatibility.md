---
title: lifecycle-and-compatibility
authors:
  - "@perdasilva"
reviewers:
  - "@joelanford, for OLM architecture and lifecycle-server design"
  - "@spadgett, for Console integration and frontend architecture"
approvers:
  - "@joelanford"
api-approvers:
  - None
creation-date: 2026-04-29
last-updated: 2026-04-29
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2618
status: provisional
see-also:
  - "/enhancements/olm/max-openshift-versions-for-operators.md"
---

# OLMv0 Operator Lifecycle and Compatibility

## Summary

This enhancement introduces operator lifecycle and platform compatibility visibility
for OLMv0-managed operators in OpenShift. A new lifecycle metadata extension schema
(`io.openshift.operators.lifecycles.v1alpha1`) is added to File-Based Catalogs (FBC)
to carry per-operator support phase, lifecycle timeline, and platform compatibility data.
Two new downstream-only components — a lifecycle-controller and a lifecycle-server —
are deployed to extract and serve this metadata. The lifecycle-controller runs in the
`openshift-operator-lifecycle-manager` namespace and manages lifecycle-server Deployments
alongside CatalogSource pods, initially limited to CatalogSources in the
`openshift-marketplace` namespace. The lifecycle-server exposes an internal HTTPS API
secured via TokenReview and SubjectAccessReview — callers must present a ServiceAccount
token with the requisite nonResourceURL RBAC to access lifecycle data. The OpenShift
Console is extended with a backend proxy endpoint and frontend UI columns ("Cluster
Compatibility" and "Support") on the installed operators table, giving platform engineers
immediate visibility into which operators are supported, approaching end-of-life, or
incompatible with the current cluster version.

## Motivation

Platform engineers managing OpenShift clusters lack a centralized view of operator lifecycle status. Today, users must consult external documentation for each operator to find support status, lifecycle duration, and platform compatibility — information that is critical for making update decisions and planning maintenance windows.

This creates operational risk:

- Operators may unknowingly be running in end-of-life status
- Platform upgrades may introduce compatibility issues with installed operators
- Maintenance windows are difficult to plan without visibility into lifecycle timelines

Without lifecycle visibility, customers risk running unsupported operator configurations, which leads to support escalations and delayed platform upgrades.

### User Stories

* As a platform engineer, I want to see the current support phase (e.g. Full Support, Maintenance, End of Life) of each installed operator in the OpenShift Console, so that I can identify operators that need attention before they become unsupported.

* As a platform engineer, I want to see how much time remains in the current lifecycle phase for each installed operator, so that I can plan maintenance windows and operator updates proactively.

* As a platform engineer, I want to see whether each installed operator version is compatible with the current cluster version, so that I can identify potential issues before or after a platform upgrade.

* As a cluster administrator, I want lifecycle and compatibility information to be available in disconnected environments, so that I have the same operational visibility regardless of cluster connectivity.

* As an operator author, I want to include lifecycle metadata in my operator's catalog entry, so that my users have visibility into the support status of my operator versions.

### Goals

1. Surface operator lifecycle phase (tech-preview, GA, maintenance, deprecated, end-of-life) per installed operator in the OpenShift Console.
2. Display remaining time in the current lifecycle phase for each installed operator.
3. Show platform compatibility assessment (installed operator version vs. current cluster version) in the Console.
4. Ensure backward compatibility: catalogs without lifecycle metadata continue to function normally with appropriate "no data" indicators in the UI.
5. Support disconnected environments without additional tooling changes.

### Non-Goals

1. Special-purpose CLI lifecycle commands (future phase).
2. Update availability enumeration or upgrade path recommendations (future phase).
3. Release notes and change previews (future phase).
4. Changes to existing OLMv0 controllers, Subscription API, or catalog operator code.
5. OLMv1-specific integration — this enhancement targets OLMv0 exclusively.
6. Defining the ground truth lifecycle data structure — that is owned by the ProdOps engineering team. However, defining the FBC schema (`io.openshift.operators.lifecycles.v1alpha1`) that carries this data within catalogs is a goal of this enhancement.

## Proposal

This proposal introduces three sets of changes across three repositories to deliver operator lifecycle visibility:

1. **operator-framework-olm**: A new lifecycle-controller and lifecycle-server that extract and serve lifecycle metadata from FBC catalogs.
2. **console-operator**: RBAC configuration granting the Console ServiceAccount access to the lifecycle-server API.
3. **console**: A backend proxy endpoint and frontend UI components to display lifecycle and compatibility data.

### Workflow Description

**platform engineer** is a human user who manages an OpenShift cluster and its installed operators.

**lifecycle-controller** is a controller that watches CatalogSource resources and manages lifecycle-server Deployment resources.

**lifecycle-server** is an HTTPS server that reads FBC catalog content from OCI volume mounts and serves lifecycle metadata.

**console backend** is the OpenShift Console server that proxies lifecycle API requests.

**console frontend** is the OpenShift Console UI that displays lifecycle information.

1. A catalog curator includes lifecycle metadata (using the `io.openshift.operators.lifecycles.v1alpha1` schema) in the FBC for the redhat-operators catalog during the catalog build process.
2. The lifecycle-controller watches for CatalogSource resources. When it detects a CatalogSource, it creates a lifecycle-server Deployment that mounts the catalog image as an OCI volume.
3. When the catalog image changes (detected by watching CatalogSource pods), the lifecycle-controller updates the lifecycle-server Deployment to reference the new image.
4. The lifecycle-server starts, walks the FBC content on its mounted volume, extracts entries matching `io.openshift.operators.lifecycles.*` schemas, and serves them over an HTTPS API.
5. A platform engineer navigates to the Installed Operators page in the OpenShift Console.
6. The console frontend requests lifecycle data for installed operators via the console backend proxy endpoint (`/api/olm/lifecycle/`).
7. The console backend authenticates with the lifecycle-server using its pod ServiceAccount token and forwards the request.
8. The lifecycle-server responds with lifecycle metadata for the requested packages.
9. The console frontend renders "Cluster Compatibility" and "Support" columns:
   - **Cluster Compatibility**: Shows whether the installed operator version is compatible with the current cluster version ("Compatible", "Incompatible", or "No data").
   - **Support**: Shows the current support phase with remaining time, using color-coded icons (green check for >12 months remaining, yellow warning for <=12 months remaining, "Self-support" when all phases have ended).

#### Error Handling

- If the lifecycle-server is unavailable, the console displays "No data" labels for both columns.
- If a catalog does not contain lifecycle metadata, the lifecycle-server returns empty results and the console displays "No data".
- Lifecycle data is cached in the console frontend with a 5-minute success TTL and 30-second error TTL, with request deduplication to avoid redundant API calls.

### API Extensions

This enhancement does not introduce new CRDs or modify existing Kubernetes API resources.

The lifecycle-server exposes an internal HTTPS API (not exposed outside the cluster):

- **Host**: `https://<catalogSourceName>-lifecycle-server.<catalogSourceNamespace>.svc:8443`
- **Path**: `/api/lifecycles/v1alpha1?packageNames=<commaSeparatedListOfPackageNames>`

The lifecycle-server authenticates callers via TokenReview and authorizes them via SubjectAccessReview on nonResourceURLs (`/api/*/lifecycles/*`). A ClusterRole and ClusterRoleBinding grant the Console ServiceAccount read access to these paths.

The console backend adds a proxy endpoint:

- **Path**: `/api/olm/lifecycle/`
- Validates catalog namespace and name inputs against strict regex to prevent SSRF.
- Forwards requests to the per-catalog lifecycle-server using the pod's ServiceAccount bearer token.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift support is not a goal at this stage. The lifecycle-controller and lifecycle-server run in the same namespace as CatalogSource pods. In HyperShift environments, CatalogSources are typically in the guest cluster, so the lifecycle components would deploy there as well. No special handling is required for the management cluster since these components only interact with CatalogSource resources and do not modify control plane components.

#### Standalone Clusters

This enhancement is fully relevant and functional on standalone clusters. It is the primary target topology.

#### Single-node Deployments or MicroShift

The lifecycle-controller and lifecycle-server add two additional pods per CatalogSource. On single-node OpenShift (SNO), this represents a modest increase in resource consumption. The lifecycle-server pods include resource requests to ensure appropriate scheduling.

This enhancement does not apply to MicroShift, which does not use OLM.

#### OpenShift Kubernetes Engine

This enhancement depends on the Console capability (for UI display) and OLM (for CatalogSource resources). OKE includes OLM but the Console operator is optional. If the Console capability is not enabled, the lifecycle-controller and lifecycle-server still deploy but the lifecycle data will not be visible to users through the Console.

### Implementation Details/Notes/Constraints

#### Lifecycle Metadata Schema

The lifecycle metadata uses FBC's extensibility mechanism. Each entry uses the schema `io.openshift.operators.lifecycles.v1alpha1` and contains:

- **Version**: Operator minor version (X.Y format)
- **Platform compatibility**: List of supported platforms and platform versions
- **Lifecycle phases**: Named phases ("Community", "Full Support", "Maintenance", "EUS-1", "EUS-2", "End of Life") with date ranges

The lifecycle-server treats this content opaquely — it extracts and serves entries matching `io.openshift.operators.lifecycles.*` schemas without parsing the internal structure. This means future schema changes (field-level within a version, or new schema versions) flow through without requiring lifecycle-server code changes.

#### Lifecycle Controller (operator-framework-olm)

The lifecycle-controller:

- Watches CatalogSource resources and reconciles lifecycle-server Deployments
- Uses server-side apply (SSA) for all owned resources
- Manages NetworkPolicy resources to restrict traffic to the lifecycle-server
- Configures TLS via controller-runtime-common for secure communication
- Supports leader election for HA deployments
- Detects catalog image changes by watching CatalogSource pods and updates the lifecycle-server Deployment accordingly

#### Lifecycle Server (operator-framework-olm)

The lifecycle-server:

- Serves FBC lifecycle content over HTTPS with TLS
- Mounts catalog images as OCI volumes
- Walks FBC subdirectories to find and serve lifecycle schema entries
- Authenticates callers via TokenReview and authorizes via SubjectAccessReview
- Includes health/readiness probes and resource requests
- Runs with restricted security context (read-only root filesystem, non-root, dropped capabilities)

#### Console Backend (console)

- Adds `/api/olm/lifecycle/` proxy endpoint
- Authenticates with lifecycle-server using the pod's ServiceAccount bearer token
- Validates catalog namespace and name inputs against strict regex patterns to prevent SSRF attacks

#### Console Frontend (console)

- Adds `useOperatorLifecycle` hook with in-memory caching (5-minute success / 30-second error TTL) and request deduplication
- Adds `ClusterCompatibilityStatus` and `SupportPhaseStatus` components using PatternFly Labels
- Version matching uses minor version fallback when lifecycle metadata versions (e.g. "2.16") don't exactly match the CSV spec version (e.g. "2.16.0")
- Support column shows:
  - Last support phase end date with green check icon (>12 months remaining) or yellow warning icon (<=12 months remaining)
  - Tooltip with current phase name (e.g. "Maintenance support", "Extended life cycle support")
  - "Self-support" when all phases have ended
  - "No data" when lifecycle info is unavailable
- All UI gated behind the `OPERATOR_LIFECYCLE_METADATA` feature flag

#### Console Operator (console-operator)

- Adds ClusterRole and ClusterRoleBinding granting the Console ServiceAccount read access to lifecycle-server nonResourceURL paths (`/api/*/lifecycles/*`)
- Manifest scoped to the Console capability

#### Deployment Manifests (operator-framework-olm)

- RBAC, NetworkPolicy, Service, and Deployment manifests for lifecycle-controller and lifecycle-server
- IBM Cloud managed variants included
- CRD manifests updated with lifecycle annotations
- Initially restricted to CatalogSources in the `openshift-marketplace` namespace (and potentially limited to only the `redhat-operators` CatalogSource)

### Risks and Mitigations

| Risk | Mitigation |
| :--- | :--- |
| New components increase operational surface area | The lifecycle-server is designed as a simple, stateless HTTP server with minimal failure modes. The lifecycle-controller can be scaled to zero in an emergency. |
| Lifecycle metadata may become stale or out-of-sync with PLCC | The ProdOps team is responsible for defining validation and synchronization tooling. Catalog builds will include validated lifecycle metadata. |
| SSRF via console proxy endpoint | Console backend validates catalog namespace and name inputs against strict regex patterns before forwarding requests. |
| Catalog image polling timing may cause briefly stale lifecycle data | The lifecycle-controller watches CatalogSource pods and updates the lifecycle-server Deployment when the catalog image changes. A small window of staleness is acceptable since lifecycle metadata changes infrequently. |
| Additional resource consumption on SNO | Lifecycle-server pods include appropriate resource requests. The pods are lightweight and stateless. |

### Drawbacks

- This approach introduces a new downstream-only component (lifecycle-controller + lifecycle-server) that serves a single purpose. This is acknowledged as immediate tech debt that will need to be reconciled when OLMv1 matures.
- The lifecycle-server operates out-of-band from the existing OLM architecture, meaning it duplicates some catalog image handling logic.
- The long-term desired approach is an OLMv1-centric catalog overhaul that would natively incorporate lifecycle metadata alongside other data model improvements (semver-based upgrade graphs, simplified disconnected mirroring, search facets, etc.). This pragmatic approach was chosen to meet the OCP 5.0 timeline.

## Alternatives (Not Implemented)

### Console-only changes (use OLMv1 ClusterCatalog data)

The Console already integrates with OLMv1's catalogd via the `api/v1/all` endpoint. Since both OLMv0 and OLMv1 use the same `registry.redhat.io/redhat/redhat-operator-index:vX.Y` image, lifecycle metadata added to the catalog would automatically be available through OLMv1's ClusterCatalog.

**Rejected because:**

1. Disconnected clusters may not have ClusterCatalogs enabled. Users relying on OLMv0 would reasonably expect lifecycle visibility without OLMv1 interaction.
2. OLMv1 catalogs may diverge from OLMv0 catalogs in the future.
3. Catalog image poll timing means CatalogSource and ClusterCatalog digests may never agree, potentially showing lifecycle data that is ahead of what OLMv0 can actually install.

### Extend OLMv0 GetPackage API

Extend the existing OLMv0 catalog pods' GetPackage gRPC API to include lifecycle metadata, and surface it through the Subscription status.

**Rejected because:**

- A fully correct implementation would require significant refactoring of OLMv0's multi-writer controllers, async catalog handling, and forcible status overwrites.
- Even a simple implementation carries risk of post-change bugs/regressions in the mature OLMv0 codebase.
- Syncing from a separate lifecycle database introduces risk that derivative data is out-of-sync.

### OLMv1-centric catalog overhaul

A complete revamp of FBC schemas to natively incorporate lifecycle, compatibility, semver-based upgrade graphs, and other improvements.

**Rejected because:**

- The BU needs lifecycle and compatibility information available in OCP 5.0, and the OLMv1-centric approach would take too long to deliver.
- The OLMv1-centric approach remains the long-term goal but has been deprioritized in favor of this pragmatic solution.

## Open Questions

1. **Pre-releases and Tech Preview versions:** Catalogs include version numbers that don't follow standard semver (e.g. "0.0.0-4.99.0-techpreview"). How should these be handled in lifecycle metadata validation that requires all minor versions to be represented?

2. **Concurrent Lifecycle Phases:** Do we need to account for operators having concurrent lifecycle phases? If so, how should concurrent phases be presented to users?

3. **Source system of record:** Where should product teams directly maintain their lifecycle information — PLCC or FBC?

4. **Console feature flag integration:** What is the correct approach for gating the lifecycle UI in the Console? How should the `OPERATOR_LIFECYCLE_METADATA` feature flag integrate with the feature gate being added for this enhancement? Needs alignment with the Console team.

## Test Plan

Testing covers all three components:

**operator-framework-olm:**
- Unit tests for lifecycle-controller reconciliation, including: CatalogSource mapping, catalog image change propagation, multi-CatalogSource management, Deployment field validation (security context, probes, resource requests, tolerations, node affinity), and error propagation.
- Unit tests for lifecycle-server: TLS configuration, FBC catalog building, HTTP handler concurrency safety, multiple API versions, and FBC subdirectory walking.
- Ginkgo e2e test suite covering: happy-path provisioning, cleanup on CatalogSource deletion, pod hardening validation, and independent multi-CatalogSource lifecycle management.

**console:**
- Frontend unit tests for the `useOperatorLifecycle` hook, `ClusterCompatibilityStatus`, and `SupportPhaseStatus` components.
- Tests for version matching logic (exact match and minor version fallback).

**console-operator:**
- Verification that RBAC manifests are correctly scoped to the Console capability.

**Integration testing:**
- End-to-end validation that lifecycle metadata flows from FBC catalog content through the lifecycle-server, console backend proxy, and into the console frontend UI.
- Validation that catalogs without lifecycle metadata display appropriate "No data" indicators.
- Disconnected environment validation.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Lifecycle-controller and lifecycle-server deploy successfully
- Console displays lifecycle and compatibility columns for installed operators
- At least 10 operators (with representation from all three lifecycle tiers) include lifecycle information that flows from FBC catalog contributions to Console UI views
- End-to-end functionality verified in connected and disconnected environments
- End user documentation for the feature

### Tech Preview -> GA

- Sufficient time for feedback from Tech Preview users
- 100% of operators in the redhat-operators catalog include lifecycle and compatibility metadata
- Load testing completed to ensure performance thresholds are not violated
- Upgrade and downgrade testing completed
- Telemetry in place to track operator lifecycle status across the fleet
- User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

This feature is new and not replacing a deprecated feature. If the OLMv1-centric approach supersedes this implementation in the future, the lifecycle-controller and lifecycle-server components would be removed along with the console proxy endpoint, and the console frontend would be updated to consume lifecycle data from the OLMv1 catalog API instead.

## Upgrade / Downgrade Strategy

**Upgrade:**
- On upgrade to a version containing this feature, the lifecycle-controller and lifecycle-server Deployments are created automatically.
- The lifecycle-controller begins watching CatalogSources in the `openshift-marketplace` namespace and provisioning lifecycle-server pods.
- No user action is required.
- Existing operator functionality is unaffected — no changes to OLMv0 controllers or Subscription API.

**Downgrade:**
- On downgrade, the lifecycle-controller and lifecycle-server Deployments are removed by CVO along with their associated RBAC, NetworkPolicy, and Service resources.
- The console will no longer display lifecycle columns (the feature flag will be absent).
- No manual cleanup steps are required.
- No data loss occurs since the lifecycle-server is stateless.

## Version Skew Strategy

The lifecycle-controller, lifecycle-server, console-operator RBAC, and console changes are all deployed as part of the same release payload. During a rolling upgrade:

- If the lifecycle-server is deployed before the console changes: The server is running but the console does not yet display the columns. No user-visible impact.
- If the console changes are deployed before the lifecycle-server: The console attempts to fetch lifecycle data, receives errors, and displays "No data" labels. This is the expected fallback behavior.
- The lifecycle-server is independent of OLMv0 controllers and does not participate in version skew with the catalog operator, subscription controller, or any other OLMv0 component.

## Operational Aspects of API Extensions

The lifecycle-server exposes an internal HTTPS API authenticated via TokenReview/SubjectAccessReview on nonResourceURLs.

**Health indicators:**
- Lifecycle-server pods include readiness and liveness probes
- The lifecycle-controller monitors CatalogSource resources and lifecycle-server Deployment status
- NetworkPolicy restricts traffic to the lifecycle-server to authorized sources

**Impact on existing SLIs:**
- The lifecycle-controller and lifecycle-server are independent of the OLMv0 control loop. They do not affect operator installation, updates, or any existing OLM functionality.
- The console proxy adds one additional HTTP call per installed-operators page load. Responses are cached (5-minute TTL) to minimize impact.
- Expected catalog sizes and lifecycle metadata volumes are small (kilobytes per catalog), so API throughput impact is negligible.

**Failure modes:**
- If the lifecycle-server is unavailable, the console displays "No data" — no impact on operator management functionality.
- If the lifecycle-controller fails, existing lifecycle-server pods continue to serve their last-known data. New CatalogSources will not get lifecycle-server pods until the controller recovers.
- If the console proxy endpoint fails, only lifecycle display is affected. All other console functionality remains operational.

**Escalation teams:**
- OLM team: lifecycle-controller and lifecycle-server issues
- Console team: proxy endpoint and frontend display issues

## Support Procedures

**Detecting failure:**
- If lifecycle data is not appearing in the console, check:
  - `oc get pods -n openshift-operator-lifecycle-manager -l app=lifecycle-controller` — controller pod should be running
  - `oc get pods -n <catalogSourceNamespace> -l app=<catalogSourceName>-lifecycle-server` — lifecycle-server pod should be running for each CatalogSource
  - Console pod logs for errors proxying to the lifecycle-server endpoint
  - Lifecycle-server pod logs for TLS, authentication, or FBC parsing errors

**Disabling the feature:**
- In an emergency, the lifecycle-controller Deployment can be scaled to zero. Existing lifecycle-server pods will continue serving cached data but will not be updated when catalog images change.

**Consequences of disabling:**
- Console will display "No data" for lifecycle and compatibility columns
- No impact on existing operator installation, updates, or cluster health
- No data loss — the lifecycle-server is stateless

**Recovery:**
- Scaling the lifecycle-controller back up will cause it to reconcile all CatalogSources and recreate any missing lifecycle-server pods
- Functionality resumes without manual intervention

## Infrastructure Needed

No new subprojects or repositories are needed. All changes are contained within existing repositories:

- `openshift/operator-framework-olm` — lifecycle-controller and lifecycle-server
- `openshift/console` — backend proxy and frontend UI
- `openshift/console-operator` — RBAC manifests
