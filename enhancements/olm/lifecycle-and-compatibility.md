---
title: lifecycle-and-compatibility
authors:
  - "@perdasilva"
reviewers:
  - "@joelanford, for OLM architecture and ExperimentalListPackageCustomSchemas design"
  - "@spadgett, for Console integration and frontend architecture"
approvers:
  - "@joelanford"
api-approvers:
  - None
creation-date: 2026-04-29
last-updated: 2026-06-01
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
A new `ExperimentalListPackageCustomSchemas` gRPC endpoint is added to `opm serve` via
a new `ExperimentalRegistry` gRPC service — since CatalogSource catalog pods already run
`opm serve`, they automatically serve custom FBC schemas (including lifecycle metadata)
through this endpoint. The OpenShift Console backend
acts as a gRPC client, calling `ExperimentalListPackageCustomSchemas` directly on the CatalogSource catalog pods
and exposing the results over HTTP. The Console frontend adds "Cluster Compatibility" and
"Support" columns on the installed operators table, giving platform engineers immediate
visibility into which operators are supported, approaching end-of-life, or incompatible
with the current cluster version.

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
7. Defining a stable, general purpose, and documented lifecycle API. For this EP, the lifecycle FBC schema and the Console's use of `ExperimentalListPackageCustomSchemas` are strictly internal implementation details, not a public API surface.
8. Support for MicroShift.
9. Support for HyperShift / Hosted Control Planes — the Console backend's network access to CatalogSource catalog pods in HCP topologies requires further investigation. HyperShift support may be addressed in a future iteration.

## Proposal

This proposal introduces two sets of changes across two repositories to deliver operator lifecycle visibility:

1. **operator-registry**: A new `ExperimentalListPackageCustomSchemas` gRPC endpoint on a new `ExperimentalRegistry` gRPC service in `opm serve` that serves custom FBC schema blobs (including lifecycle metadata) from the existing CatalogSource catalog pods.
2. **console**: A backend gRPC client that queries CatalogSource catalog pods for lifecycle metadata and exposes it over HTTP, plus frontend UI components to display lifecycle and compatibility data.

### Workflow Description

**platform engineer** is a human user who manages an OpenShift cluster and its installed operators.

**CatalogSource catalog pod** is the existing pod running `opm serve` that serves operator catalog content over gRPC, including custom FBC schemas via the `ExperimentalListPackageCustomSchemas` endpoint.

**console backend** is the OpenShift Console server that acts as a gRPC client to CatalogSource catalog pods.

**console frontend** is the OpenShift Console UI that displays lifecycle information.

1. Lifecycle metadata is centrally managed by Red Hat product management in the Product Lifecycle and Compatibility Center (PLCC) — individual operator teams do not author or maintain this data. The Red Hat catalog pipeline pulls lifecycle data from PLCC and injects it into the FBC for Red Hat operator catalogs during the catalog build process, validating that it conforms to the `io.openshift.operators.lifecycles.v1alpha1` schema. The catalog pipeline is responsible for ensuring that each `(schema, packageName)` pair has exactly one lifecycle metadata blob — duplicates must be prevented at build time.
2. The existing CatalogSource catalog pods (running `opm serve`) automatically index custom FBC schemas — including lifecycle metadata — during cache build. The lifecycle data is served via the `ExperimentalListPackageCustomSchemas` gRPC endpoint on the catalog pod's `ExperimentalRegistry` gRPC service (port 50051).
3. A platform engineer navigates to the Installed Operators page in the OpenShift Console.
4. The console frontend requests lifecycle data for installed operators via the console backend endpoint (`/api/olm/lifecycle/{catalogNamespace}/{catalogName}/{packageName}`).
5. The console backend dials the CatalogSource catalog pod's gRPC service at `{catalogName}.{catalogNamespace}.svc:50051` and calls `ExperimentalListPackageCustomSchemas` with the lifecycle schema and package name filters.
6. The catalog pod streams lifecycle metadata for the requested package.
7. The console backend marshals the protobuf response to JSON and returns it over HTTP.
8. The console frontend renders "Cluster Compatibility" and "Support" columns:
   - **Cluster Compatibility**: Shows whether the installed operator version is compatible with the current cluster version ("Compatible", "Incompatible", or "No data").
   - **Support**: Shows the current support phase with remaining time, using color-coded icons (green check for >12 months remaining, yellow warning for <=12 months remaining, "Self-support" when all phases have ended).

#### Error Handling

- If the CatalogSource catalog pod is unavailable or does not support `ExperimentalListPackageCustomSchemas`, the console displays "No data" labels for both columns.
- If a catalog does not contain lifecycle metadata, the `ExperimentalListPackageCustomSchemas` endpoint returns empty results and the console displays "No data".
- If the endpoint returns multiple lifecycle metadata documents for the same package (indicating a catalog build error), the console logs an error and displays "No data" for that operator rather than attempting to merge or choose between conflicting documents.
- Lifecycle data is cached in the console frontend with a 5-minute success TTL and 30-second error TTL, with request deduplication to avoid redundant API calls.

### API Extensions

This enhancement does not introduce new CRDs or modify existing Kubernetes API resources.

The lifecycle FBC metadata follows the schema:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "io.openshift.operators.lifecycles.v1alpha1",
  "title": "Operator Lifecycle",
  "description": "Defines the lifecycle phases and platform compatibility for an OpenShift operator package.",
  "type": "object",
  "required": ["package", "schema", "versions"],
  "additionalProperties": false,
  "properties": {
    "package": {
      "type": "string",
      "description": "The operator package name."
    },
    "schema": {
      "type": "string",
      "const": "io.openshift.operators.lifecycles.v1alpha1",
      "description": "Schema identifier for this lifecycle document."
    },
    "versions": {
      "type": "array",
      "description": "List of operator versions with their lifecycle phases.",
      "minItems": 1,
      "items": {
        "type": "object",
        "required": ["name", "phases"],
        "additionalProperties": false,
        "properties": {
          "name": {
            "type": "string",
            "description": "The operator version identifier."
          },
          "phases": {
            "type": "array",
            "description": "List of lifecycle phases for this version.",
            "minItems": 1,
            "items": {
              "type": "object",
              "required": ["name", "startDate", "endDate"],
              "additionalProperties": false,
              "properties": {
                "name": {
                  "type": "string",
                  "description": "The lifecycle phase name.",
                  "examples": [
                    "Full support",
                    "Maintenance support",
                    "Extended update support",
                    "Extended update support Term 2",
                    "Extended life cycle support (ELS) add-on"
                  ]
                },
                "startDate": {
                  "type": "string",
                  "format": "date",
                  "description": "Start date of this phase (inclusive)."
                },
                "endDate": {
                  "type": "string",
                  "format": "date",
                  "description": "End date of this phase (inclusive)."
                }
              }
            }
          },
          "platformCompatibility": {
            "type": "array",
            "description": "Platforms and their versions that this operator version is compatible with.",
            "items": {
              "type": "object",
              "required": ["name", "versions"],
              "additionalProperties": false,
              "properties": {
                "name": {
                  "type": "string",
                  "description": "The platform name.",
                  "examples": ["openshift"]
                },
                "versions": {
                  "type": "array",
                  "description": "Compatible platform versions.",
                  "minItems": 1,
                  "items": {
                    "type": "string"
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
```

**Root fields:**

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `schema` | string | yes | Always `io.openshift.operators.lifecycles.v1alpha1`. Identifies this blob type within FBC. |
| `package` | string | yes | The OLM catalog package name (e.g., `aws-efs-csi-driver-operator`). Must match the operator's package in the catalog. |
| `versions` | array | yes | List of version entries. Each entry describes one minor release of the operator. |

**Version fields:**

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Version identifier in `MAJOR.MINOR` format (e.g., `4.12`, `1.5`). |
| `phases` | array | yes | Ordered list of lifecycle phases for this version. Must be contiguous (no gaps or overlaps). |
| `platformCompatibility` | array | no | Platforms this version is compatible with. Omitted if no compatibility data is available. |

**Phase fields:**

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Phase name (e.g., `Full support`, `Maintenance support`). |
| `startDate` | string | yes | Start date in `YYYY-MM-DD` format. |
| `endDate` | string | yes | End date in `YYYY-MM-DD` format. Must be strictly after `startDate`. |

Phases within a version are ordered chronologically and must be contiguous: the start date of phase N must be exactly one day after the end date of phase N-1. There must be no gaps or overlaps between adjacent phases.

**Platform compatibility fields:**

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `name` | string | yes | Platform identifier (e.g., `openshift`). |
| `versions` | array | yes | List of platform versions this operator version is compatible with, each in `MAJOR.MINOR` format. |

The `platformCompatibility` structure is designed to support multiple platforms. Currently, only `openshift` is populated.

**Example:**

```yaml
schema: io.openshift.operators.lifecycles.v1alpha1
package: aws-efs-csi-driver-operator
versions:
  - name: "4.12"
    phases:
      - name: Full support
        startDate: "2023-01-17"
        endDate: "2023-08-17"
      - name: Maintenance support
        startDate: "2023-08-18"
        endDate: "2024-07-17"
      - name: Extended update support
        startDate: "2024-07-18"
        endDate: "2025-01-17"
      - name: Extended update support Term 2
        startDate: "2025-01-18"
        endDate: "2026-01-17"
      - name: Extended update support Term 3
        startDate: "2026-01-18"
        endDate: "2027-01-17"
    platformCompatibility:
      - name: openshift
        versions:
          - "4.12"
  - name: "4.17"
    phases:
      - name: Full support
        startDate: "2024-10-01"
        endDate: "2025-05-25"
      - name: Maintenance support
        startDate: "2025-05-26"
        endDate: "2026-04-01"
    platformCompatibility:
      - name: openshift
        versions:
          - "4.17"
```

The `ExperimentalListPackageCustomSchemas` gRPC endpoint is served by the existing CatalogSource catalog pods (running `opm serve`):

- **Service**: `ExperimentalRegistry` (new OPM gRPC service, separate from the existing `Registry` service)
- **Endpoint**: `rpc ExperimentalListPackageCustomSchemas(ExperimentalListPackageCustomSchemasRequest) returns (stream google.protobuf.Struct)`
- **Address**: `{catalogName}.{catalogNamespace}.svc:50051`
- **Required metadata**: Clients must send the `x-acknowledge-experimental: true` gRPC metadata header. Without this header, the endpoint silently returns an empty stream (no error) to ensure callers explicitly acknowledge the experimental nature of the API.
- **Request fields**:
  - `schema` (string, **required**): FBC schema to query (e.g. `io.openshift.operators.lifecycles.v1alpha1`). Returns `InvalidArgument` if empty.
  - `packageName` (string, optional): package name to scope the query. When provided, only blobs for that package are returned. When empty, returns blobs stored without a package association.
- **Response**: server-side stream of `google.protobuf.Struct` messages, each containing a raw FBC blob as a JSON-like structure. Multiple blobs may be returned for the same `(schema, packageName)` pair if the catalog contains more than one matching entry.
- **Error codes**: `InvalidArgument` if `schema` is empty or fails input validation (must match `^[a-zA-Z0-9][a-zA-Z0-9._-]*$`); `Unimplemented` if the store does not support custom schema queries; `Internal` for unmarshalling or other server-side errors.

The console backend adds an HTTP endpoint that acts as a gRPC client:

- **Path**: `/api/olm/lifecycle/{catalogNamespace}/{catalogName}/{packageName}`
- Validates catalog namespace and name inputs against strict Kubernetes DNS name regex to prevent injection attacks.
- Dials the CatalogSource catalog pod's gRPC service and calls `ExperimentalListPackageCustomSchemas` with the lifecycle schema and package name filters.
- Marshals protobuf `Struct` responses to JSON and returns them over HTTP.
- Handles gRPC `Unimplemented` errors (from older opm versions) by returning HTTP 503, which the frontend treats as "No data".

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is out of scope for this enhancement. The Console backend needs network access to the CatalogSource catalog pods' gRPC service, and the specifics of how this works in HCP topologies (where the control plane and data plane run in separate clusters) have not yet been resolved. HyperShift support may be addressed in a future iteration once the standalone cluster implementation is validated.

#### Standalone Clusters

This enhancement is fully relevant and functional on standalone clusters. It is the primary target topology.

#### Single-node Deployments or MicroShift

No new pods are introduced — the `ExperimentalListPackageCustomSchemas` endpoint is served by the existing CatalogSource catalog pods. There is no additional resource consumption beyond the negligible cost of indexing custom FBC schemas during cache build.

This enhancement does not apply to MicroShift.

#### OpenShift Kubernetes Engine

This enhancement depends on the Console capability (for UI display) and OLM (for CatalogSource resources). OKE includes OLM but the Console operator is optional. If the Console capability is not enabled, the `ExperimentalListPackageCustomSchemas` endpoint is still available on catalog pods but lifecycle data will not be visible to users.

### Implementation Details/Notes/Constraints

#### Lifecycle Metadata Schema

The lifecycle metadata uses FBC's extensibility mechanism.
Each FBC lifecycle blob carries the data of each operator using the `io.openshift.operators.lifecycles.v1alpha1` schema.
The data include the available package versions, the lifecycle phases for each version, and the platforms each version is compatible with.
See [API Extensions](#api-extensions) for the full schema definition, field descriptions, and examples.

The `ExperimentalListPackageCustomSchemas` endpoint treats this content opaquely — it serves entries matching the requested schema filter without parsing the internal structure. This means future schema changes (field-level within a version, or new schema versions) flow through without requiring any server-side code changes.

#### ExperimentalListPackageCustomSchemas gRPC Endpoint (operator-registry)

A new `ExperimentalListPackageCustomSchemas` RPC is added as part of a new `ExperimentalRegistry` gRPC service in `opm serve`, separate from the existing `Registry` service:

- During cache build, `opm serve` walks the FBC content and indexes all non-standard schemas (anything other than `olm.package`, `olm.channel`, `olm.bundle`, `olm.deprecation`) as custom schema metadata, keyed by `(schema, packageName)`. Custom schema blobs without a `package` field are stored with an empty `packageName` key. Blobs are content-addressed using FNV64a hashing, which naturally deduplicates identical blobs stored under the same `(schema, packageName)` pair.
- Clients must send the `x-acknowledge-experimental: true` gRPC metadata header. Without this header, the endpoint silently returns an empty stream. This forces callers to explicitly acknowledge the experimental nature of the API.
- The `ExperimentalListPackageCustomSchemas` endpoint requires `schema` (returns `InvalidArgument` if empty) and accepts an optional `packageName`. It streams all matching blobs as `google.protobuf.Struct` messages.
- The server-side implementation uses a duck-typed `customSchemaQuerier` interface on the cache store. If the store does not implement this interface, the endpoint returns gRPC `Unimplemented`, providing graceful degradation for older opm versions. Cache-layer `ValidationError`s (e.g. input validation failures in schema or packageName) are converted to gRPC `InvalidArgument`.
- Meta keys are validated against the pattern `^[a-zA-Z0-9][a-zA-Z0-9._-]*$` — they must start with an alphanumeric character and may only contain alphanumeric characters, dots, underscores, and dashes. A dedicated `ValidationError` type is used to distinguish validation failures from other errors.
- Two storage backends are supported: a JSON filesystem backend (stores blobs as `{schema}/{packageName}/{fnv64a-hash}.json` files under the cache directory) and a pogreb key-value store backend (stores blobs under a single `metas/{schema}/{packageName}` key as concatenated length-prefixed proto binary blobs, with each blob preceded by a 4-byte big-endian uint32 length header). The deprecated SQLite backend does not implement the `customSchemaQuerier` interface and returns `Unimplemented`.

#### Console Backend (console)

- Adds `/api/olm/lifecycle/{catalogNamespace}/{catalogName}/{packageName}` HTTP endpoint
- Acts as a gRPC client: dials the CatalogSource catalog pod at `{catalogName}.{catalogNamespace}.svc:50051` and calls `ExperimentalListPackageCustomSchemas` with the `x-acknowledge-experimental: true` metadata header, `schema=io.openshift.operators.lifecycles.v1alpha1`, and the requested `packageName`
- Marshals the streamed protobuf `Struct` responses to JSON using `protojson.Marshal` and returns them over HTTP
- Validates catalog namespace and name inputs against strict Kubernetes DNS name regex (`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`) to prevent injection attacks
- Handles gRPC errors gracefully: `Unimplemented` and `Unavailable` return HTTP 503; other gRPC errors return HTTP 502
- If the gRPC stream returns more than one document for a package, the Console backend logs an error and returns an error response — the frontend displays "No data" for that operator. The catalog pipeline is the authoritative source and is expected to ensure exactly one lifecycle metadata blob per package; duplicates indicate a catalog build issue.
- Uses insecure gRPC transport credentials for the catalog pod connection — this is consistent with how OLMv0 controllers already connect to catalog pods (intra-cluster traffic over the pod network). The Console HTTP endpoint itself remains behind the existing Console authentication middleware.

#### Console Frontend (console)

- Adds `useOperatorLifecycle` hook with in-memory caching (5-minute success / 30-second error TTL) and request deduplication
- Adds `ClusterCompatibilityStatus` and `SupportPhaseStatus` components using PatternFly Labels
- Version matching uses minor version fallback when lifecycle metadata versions (e.g. "2.16") don't exactly match the CSV spec version (e.g. "2.16.0")
- Support column shows:
  - The current lifecycle phase name if within the support period
  - "Self-support" when all phases have ended
  - "No data" when lifecycle info is unavailable
- All UI gated behind the `OLMLifecycleAndCompatibility` feature gate

### Risks and Mitigations

| Risk                                                                                  | Mitigation                                                                                                                                                                                                                                                                                                                                                                                                               |
|:--------------------------------------------------------------------------------------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Lifecycle metadata may become stale or out-of-sync with PLCC                          | The ProdOps team is responsible for defining validation and synchronization tooling. Catalog builds will include validated lifecycle metadata.                                                                                                                                                                                                                                                                           |
| Injection attacks via console lifecycle endpoint                                      | Console backend validates catalog namespace and name inputs against strict Kubernetes DNS name regex before dialing gRPC.                                                                                                                                                                                                                                                                                                |
| Extends the gRPC API surface of OPM serve                                             | The `ExperimentalListPackageCustomSchemas` endpoint is generic (not lifecycle-specific), returns opaque FBC blobs, and gracefully returns `Unimplemented` on older opm versions. It does not increase the lifecycle-specific API surface.                                                                                                                                                                                            |
| CatalogSource pods running older opm versions won't support `ExperimentalListPackageCustomSchemas`        | Console backend handles gRPC `Unimplemented` error and returns HTTP 503. The frontend treats this as "No data" and displays graceful fallback labels.                                                                                                                                                                                                                                                                    |
| Duplicate lifecycle metadata blobs in catalog for the same package                            | The catalog pipeline is responsible for ensuring exactly one lifecycle blob per `(schema, packageName)`. The Console treats duplicates as a catalog build error — it logs the issue and displays "No data" for the operator.                                                                                                                                                                                             |
| Cache digest mismatch between new and older opm versions                                     | The new opm version stores additional custom schema entries in the cache, which changes the on-disk state. If an older opm version reads this cache, the digest could theoretically differ. This is mitigated by the fact that the cache digest is calculated over all keys in the pogreb database or all files in the cache filesystem — older opm versions will compute the correct digest for the cache they observe. |

### Drawbacks

- This approach extends the gRPC API surface of `opm serve` with the `ExperimentalListPackageCustomSchemas` endpoint. Although the endpoint is generic and not lifecycle-specific, it is a new API that needs to be maintained.
- The long-term desired approach is an OLMv1-centric catalog overhaul that would natively incorporate lifecycle metadata alongside other data model improvements (semver-based upgrade graphs, simplified disconnected mirroring, search facets, etc.). This pragmatic approach was chosen to meet the OCP 5.0 timeline.

## Alternatives (Not Implemented)

### Dedicated lifecycle-controller and lifecycle-server

Instead of adding a gRPC endpoint to the existing catalog pods, deploy two new downstream-only components in `operator-framework-olm`: a lifecycle-controller that watches CatalogSource resources and manages per-CatalogSource lifecycle-server Deployments, and a lifecycle-server that mounts the catalog image as an OCI volume, walks the FBC content, and serves lifecycle metadata over an internal HTTPS API (authenticated via TokenReview/SubjectAccessReview). The Console would proxy requests to the lifecycle-server instead of calling gRPC on the catalog pods directly.

**Not adopted because:**

- Introduces new downstream-only components (lifecycle-controller + lifecycle-server) that serve a single purpose — acknowledged tech debt that would need reconciliation when OLMv1 matures.
- Duplicates catalog image handling logic, since the lifecycle-server must independently mount and parse the same catalog image that the CatalogSource pod already serves.
- Adds operational surface area: a new controller pod plus one lifecycle-server pod per CatalogSource, with associated RBAC, NetworkPolicy, Service, and Deployment manifests.
- Does not work with CatalogSources of gRPC type with a custom address, backed by ConfigMaps, or of *internal* type.
- More complex HyperShift/HCP story since the lifecycle-server pods need to run alongside CatalogSource pods.

**Note:** this approach remains a fallback option if the `ExperimentalListPackageCustomSchemas` approach proves insufficient during Tech Preview validation. See [Graduation Criteria](#graduation-criteria).

### Console-only changes (use OLMv1 ClusterCatalog data)

The Console already integrates with OLMv1's catalogd via the `api/v1/all` endpoint. Since both OLMv0 and OLMv1 use the same `registry.redhat.io/redhat/redhat-operator-index:vX.Y` image, lifecycle metadata added to the catalog would automatically be available through OLMv1's ClusterCatalog.

**Rejected because:**

1. Disconnected clusters may not have ClusterCatalogs enabled. Users relying on OLMv0 would reasonably expect lifecycle visibility without OLMv1 interaction.
2. OLMv1 catalogs may diverge from OLMv0 catalogs in the future.
3. Catalog image poll timing means CatalogSource and ClusterCatalog digests may never agree, potentially showing lifecycle data that is ahead of what OLMv0 can actually install.

### Extend OLMv0 GRPC APIs with lifecycle-specific endpoint

Extend the existing OLMv0 catalog pods' gRPC API to include a lifecycle-specific endpoint (e.g. `GetLifecycleMetadata`).

**Rejected because:**

- A lifecycle-specific endpoint increases the domain-specific API surface of OPM serve, making it harder to maintain long-term. The adopted `ExperimentalListPackageCustomSchemas` approach is generic — it serves arbitrary custom FBC schema blobs without knowledge of lifecycle semantics, keeping the API surface minimal and reusable for other custom schemas in the future.

### Extend OLMv0 GetPackage API

Extend the existing OLMv0 catalog pods' GetPackage gRPC API to include lifecycle metadata, and surface it through the Subscription status.

**Rejected because:**

- A fully correct implementation would require significant refactoring of OLMv0's multi-writer controllers, async catalog handling, and forcible status overwrites.
- Even a simple implementation carries risk of post-change bugs/regressions in the mature OLMv0 codebase.
- Syncing from a separate lifecycle database introduces risk that derivative data is out-of-sync.
- Changes to OLMv0 carry serious risks due to possible unintended side effects.

### Use FBC bundle properties and Console-side parsing

Instead of using the `ExperimentalListPackageCustomSchemas` gRPC endpoint, lifecycle metadata could be added as an opaque [bundle property](https://github.com/operator-framework/operator-registry/blob/677f3ea1240ab76f5cc5958520b142591f6c20e2/alpha/property/property.go#L15-L18) in the existing FBC schema. The Console already [pulls bundle/CSV information](https://github.com/openshift/console/blob/1fea0064885a01e46ce60b659b5057798e56e76f/pkg/olm/catalog.go#L72) to build an internal cache, so it could parse these custom properties directly — avoiding any new gRPC endpoints.

**Rejected because:**

- This approach couples the Console directly to an internal, non-public property schema in FBC bundles, making it harder to evolve the lifecycle metadata format independently.
- Bundle properties are per-bundle, while lifecycle metadata is per-package/per-version — embedding it in bundles creates redundancy and potential consistency issues across the upgrade graph.
- The `ExperimentalListPackageCustomSchemas` approach provides a single, cacheable API endpoint that decouples the Console from the details of how lifecycle data is stored and structured in the catalog, making it easier to transition to the OLMv1-native solution in the future.
- As described in the [Workflow Description](#workflow-description), lifecycle metadata is not authored by individual operator teams in their bundle contributions — it is centrally managed by Red Hat product management in PLCC and injected at catalog build time. Using bundle properties would either require operator teams to maintain data they don't own, or require the catalog build pipeline to mutate bundle content to inject externally-owned metadata.

### OLMv1-centric catalog overhaul / registry+v2

A complete revamp of FBC schemas to natively incorporate lifecycle, compatibility, semver-based upgrade graphs, and other improvements.

**Rejected because:**

- The BU needs lifecycle and compatibility information available in OCP 5.0, and the OLMv1-centric approach would take too long to deliver.
- The OLMv1-centric approach remains the long-term goal but has been deprioritized in favor of this pragmatic solution.

## Open Questions

1. **Pre-releases and Tech Preview versions:** Catalogs include version numbers that don't follow standard semver (e.g. "0.0.0-4.99.0-techpreview"). How should these be handled in lifecycle metadata validation that requires all minor versions to be represented?

2. ~~**Concurrent Lifecycle Phases:** Do we need to account for operators having concurrent lifecycle phases? If so, how should concurrent phases be presented to users?~~ **Resolved:** Concurrent phases are disallowed in this EP. Phases within a version must be contiguous and non-overlapping.

3. ~~**Source system of record:** Where should product teams directly maintain their lifecycle information — PLCC or FBC?~~ **Resolved:** PLCC is the source of record. Lifecycle metadata is centrally managed by Red Hat product management and injected into FBC during catalog builds.

4. ~~**Console feature flag integration:** What is the correct approach for gating the lifecycle UI in the Console? How should the `OPERATOR_LIFECYCLE_METADATA` feature flag integrate with the feature gate being added for this enhancement? Needs alignment with the Console team.~~ **Resolved:** The Console lifecycle UI is gated behind the `OLMLifecycleAndCompatibility` feature gate. OLMv0 does not support feature gates, so the `ExperimentalListPackageCustomSchemas` gRPC endpoint is always available on catalog pods; only the Console UI consumption is gated.

5. **ListPackageCustomSchemas validation for GA:** The `ExperimentalListPackageCustomSchemas` gRPC endpoint approach needs practical validation during Tech Preview. Key areas to validate include: performance under load with large catalogs, graceful degradation with mixed opm versions across CatalogSource pods, and operational simplicity compared to the dedicated lifecycle-controller/server alternative. If validation reveals issues, the lifecycle-controller/server approach (described in [Alternatives](#alternatives-not-implemented)) can be used as a fallback.

## Test Plan

Testing covers two components:

**operator-registry:**
- Unit tests for `ExperimentalListPackageCustomSchemas` server implementation: streaming multiple blobs, empty results for nonexistent packages, `InvalidArgument` when schema is missing, `ValidationError` handling, handling of stores that do not implement the `customSchemaQuerier` interface (returns `Unimplemented`), and silent empty stream when the `x-acknowledge-experimental` header is missing.
- Unit tests for cache custom schema indexing: FBC walking, custom schema storage and retrieval, metaKey validation (path traversal prevention), storage of packageless blobs with empty packageName, and multiple blobs per `(schema, packageName)` pair.
- Unit tests for both JSON filesystem and pogreb storage backends: `PutMeta` content-addressed storage (FNV64a hashing), `SendMetas` iteration and length-prefixed proto binary decoding (pogreb), and correct handling of missing directories or keys.

**console:**
- Backend unit tests for the lifecycle gRPC handler: catalog pod address formatting, Kubernetes name validation (rejecting invalid patterns like domains with dots, uppercase characters, length violations), gRPC error code mapping (`Unimplemented` → 503, `Unavailable` → 503, other → 502).
- Frontend unit tests for the `useOperatorLifecycle` hook, `ClusterCompatibilityStatus`, and `SupportPhaseStatus` components.
- Tests for version matching logic (exact match and minor version fallback).

**Integration testing:**
- End-to-end validation that lifecycle metadata flows from FBC catalog content through the CatalogSource catalog pod's `ExperimentalListPackageCustomSchemas` gRPC endpoint, console backend gRPC client, and into the console frontend UI.
- Validation that catalogs without lifecycle metadata (or running older opm versions) display appropriate "No data" indicators.
- Disconnected environment validation.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `ExperimentalListPackageCustomSchemas` gRPC endpoint available on CatalogSource catalog pods
- Console displays lifecycle and compatibility columns for installed operators
- End-to-end functionality verified in connected and disconnected environments

### Tech Preview -> GA

- Product Management is happy to go GA with the functionality
- `ExperimentalListPackageCustomSchemas` approach validated in practice — performance under load, mixed opm version compatibility, and operational simplicity confirmed. Decision made on whether to continue with `ExperimentalListPackageCustomSchemas` or fall back to the dedicated lifecycle-controller/server alternative (see [Alternatives](#alternatives-not-implemented))
- 50% of operators + some selected operators in the redhat-operators catalog include lifecycle and compatibility metadata
- Load testing completed to ensure performance thresholds are not violated
- Upgrade and downgrade testing completed
- At least 10 operators (with representation from all three lifecycle tiers) include lifecycle information that flows from FBC catalog contributions to Console UI views
- Telemetry in place to track operator lifecycle status across the fleet
- User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/) — only expected documentation is on the Console side to explain the new columns. No customer facing docs for the lifecycle metadata API, or how the backend works, since they are internal details and not for general user consumption.

### Removing a deprecated feature

This feature is new and not replacing a deprecated feature. If the OLMv1-centric approach supersedes this implementation in the future, the `ExperimentalListPackageCustomSchemas` endpoint would remain available (it is generic) but the console frontend would be updated to consume lifecycle data from the OLMv1 catalog API instead.

## Upgrade / Downgrade Strategy

**Upgrade:**
- On upgrade to a version containing this feature, CatalogSource catalog pods automatically gain the `ExperimentalListPackageCustomSchemas` gRPC endpoint when the updated `opm` binary is deployed. No new Deployments or pods are created.
- The Console backend begins querying catalog pods for lifecycle metadata.
- No user action is required.
- Existing operator functionality is unaffected — no changes to OLMv0 controllers or Subscription API.

**Downgrade:**
- On downgrade, the Console feature flag is absent, so the lifecycle columns are not rendered and no gRPC calls to catalog pods are made.
- Catalog pods revert to an opm version without `ExperimentalListPackageCustomSchemas`, but this is transparent since nothing queries the endpoint.
- No manual cleanup steps are required.
- No data loss occurs since the catalog pod serves data from its existing FBC cache.

## Version Skew Strategy

The opm binary (in CatalogSource catalog pods) and console changes are all deployed as part of the same release payload. During a rolling upgrade:

- If the catalog pod is updated before the console changes: The `ExperimentalListPackageCustomSchemas` endpoint is available but not consumed. No user-visible impact.
- If the console changes are deployed before the catalog pod update: The Console calls `ExperimentalListPackageCustomSchemas`, receives gRPC `Unimplemented`, and displays "No data" labels. This is the expected fallback behavior.
- The `ExperimentalListPackageCustomSchemas` endpoint is independent of OLMv0 controllers and does not participate in version skew with the catalog operator, subscription controller, or any other OLMv0 component.
- If the CatalogSource catalog image is updated, the catalog pod rebuilds its cache and the new lifecycle metadata is immediately available through `ExperimentalListPackageCustomSchemas`.

## Operational Aspects of API Extensions

The `ExperimentalListPackageCustomSchemas` gRPC endpoint is served by the existing CatalogSource catalog pods. No new API extensions or CRDs are introduced.

**Health indicators:**
- CatalogSource catalog pods already include readiness and liveness probes. The `ExperimentalListPackageCustomSchemas` endpoint is served as part of the existing gRPC service and does not require additional health checks.

**Impact on existing SLIs:**
- The `ExperimentalListPackageCustomSchemas` endpoint is a lightweight addition to the existing gRPC service. It does not affect operator installation, updates, or any existing OLM functionality.
- The console backend adds one gRPC call per installed-operators page load. Responses are cached (5-minute success TTL, 30-second error TTL) to minimize impact.
- Custom schema indexing during cache build adds negligible overhead — lifecycle metadata volumes are small (kilobytes per catalog).

**Failure modes:**
- If the CatalogSource catalog pod is unavailable, the Console displays "No data" for lifecycle columns — no impact on operator management functionality.
- If the catalog pod runs an older opm version without `ExperimentalListPackageCustomSchemas`, the Console receives gRPC `Unimplemented` and displays "No data".
- If the Console backend gRPC client fails, only lifecycle display is affected. All other Console functionality remains operational.

**Escalation teams:**
- OLM team: `ExperimentalListPackageCustomSchemas` endpoint and CatalogSource catalog pod issues
- Console team: gRPC client, HTTP endpoint, and frontend display issues
- Layered products / PLCC team: lifecycle metadata content issues (incorrect dates, missing operators, stale data from the Product Lifecycle and Compatibility Center)
- Konflux operator pipeline team: issues related to injection of lifecycle data into catalogs during the catalog build process

## Support Procedures

**Detecting failure:**
- If lifecycle data is not appearing in the console, check:
  - `oc get pods -n <catalogSourceNamespace>` — CatalogSource catalog pod should be running
  - Console pod logs for gRPC connection errors or HTTP 503/502 responses from the lifecycle endpoint
  - CatalogSource catalog pod logs for cache build errors or custom schema indexing issues

**Disabling the feature:**
- The Console lifecycle UI is gated behind the `OLMLifecycleAndCompatibility` feature gate. When the feature gate is not enabled, the lifecycle columns are not rendered and no gRPC calls to catalog pods are made.
- The `ExperimentalListPackageCustomSchemas` endpoint remains available on catalog pods regardless of the feature gate state. OLMv0 does not have support for feature gates, so the experimental gRPC API is always present once the updated `opm` binary is deployed. The feature gate only controls the Console UI that consumes this endpoint.
- The `ExperimentalListPackageCustomSchemas` endpoint is not consumed by any component other than the Console, so its presence on catalog pods when the feature gate is disabled has no functional impact.

**Consequences of not enabling:**
- Console displays the installed operators table without lifecycle and compatibility columns
- No impact on existing operator installation, updates, or cluster health

**Recovery:**
- Enabling the `OLMLifecycleAndCompatibility` feature gate activates the lifecycle columns in the Console
- No manual intervention required — the `ExperimentalListPackageCustomSchemas` endpoint is always available on catalog pods running the updated opm

## Infrastructure Needed

No new subprojects or repositories are needed. All changes are contained within existing repositories:

- `operator-framework/operator-registry` — `ExperimentalListPackageCustomSchemas` gRPC endpoint on a new `ExperimentalRegistry` gRPC service
- `openshift/console` — backend gRPC client and frontend UI
