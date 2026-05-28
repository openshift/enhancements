---
title: olmv1-graphql-service
authors:
  - "@grokspawn"
reviewers:
  - "@everettraven, for catalogd architecture and storage layer expertise, please review the service/storage separation and handler integration"
  - "@tmshort, for OLMv1 e2e and feature gate considerations"
  - "@joelanford, for FBC schema evolution and GraphQL schema discovery approach"
approvers:
  - "@joelanford"
api-approvers:
  - None
creation-date: 2026-05-28
last-updated: 2026-05-28
status: implementable
tracking-link:
  - https://redhat.atlassian.net/browse/OPRUN-4042
see-also:
  - "/enhancements/olm/olmv1-deployment-configuration-api.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# OLMv1 Catalogd GraphQL Service Endpoint

## Summary

This enhancement adds a dynamic, schema-agnostic GraphQL query endpoint to catalogd's HTTP serving layer. The endpoint automatically discovers the structure of File-Based Catalog (FBC) data at ingestion time and generates a GraphQL schema without requiring manual type definitions. This gives catalog consumers server-side field selection, nested-object traversal, and introspection capabilities that the existing 'all' and 'metas' endpoints cannot provide — all behind a new feature gate.

## Motivation

Today, catalog consumers must either download the entire catalog via `/api/v1/all` and filter client-side, or use `/api/v1/metas` with limited schema/package/name query parameters. Neither supports field-level selection, nested-object traversal, or self-describing type introspection. The three default OCP catalogs (redhat-operators, community-operators, certified-operators) total roughly 60 MB and 6,000+ bundles; individual catalogs range from 9 to 31 MB. Icon data alone accounts for 5-8 MB per catalog (26-35% of community/certified payload, 98-99% of all `olm.package` bytes). Without server-side projection, every `/api/v1/all` call transfers this entire payload even when the consumer only needs package names or bundle versions. A traditional, handwritten GraphQL schema would solve this but create a maintenance burden: every FBC schema change would require code updates, tests, and a redeployment before new fields become queryable.

### User Stories

* As an operator author, I want to query only the significant atoms (for e.g. `name`, `package`, and `version` fields of bundles) in a catalog so that I can quickly information subsets without downloading the full catalog payload.

* As a catalogd maintainer, I want to be able to evolve FBC schema to fulfil as-yet-unknown roles/goals without having to make associated changes through the pipeline to facilitate structured queries.

* As a console user, I want to retrieve all `olm.package` entries without their `icon` blobs so that I can list available operators without transferring 5-8 MB of base64 icon data per catalog (icons constitute 98-99% of all `olm.package` bytes).

* As a CI pipeline, I want to issue a single GraphQL query that returns bundles alongside their channel memberships so that I can validate upgrade-graph correctness without multiple sequential HTTP calls.

* As a platform tooling developer, I want to introspect the catalog's GraphQL schema so that I can auto-generate client code and documentation without maintaining a separate schema definition.

### Goals

1. Provide a server-side query endpoint that supports field selection, nested-object traversal, and pagination for FBC catalog data.
2. Automatically adapt the GraphQL schema when FBC schemas evolve, requiring zero code changes in catalogd.
3. Maintain full backward compatibility with existing `/api/v1/all` and `/api/v1/metas` endpoints.
4. Gate the feature behind `GraphQLCatalogQueries` so it can be adopted incrementally.

### Non-Goals

1. Replacing or deprecating the existing `/api/v1/all` or `/api/v1/metas` endpoints.
2. Supporting GraphQL mutations or subscriptions.
3. Cross-catalog schema stitching (querying multiple catalogs in a single request).
4. Advanced filtering predicates (e.g., `where: {package: {eq: "foo"}}`) — basic `limit`/`offset` pagination is provided; richer filtering can be considered for future enhancement.

## Proposal

Add a `POST /catalogs/{catalog}/api/v1/graphql` endpoint to catalogd's HTTP service. The endpoint accepts a standard GraphQL JSON request body (`{"query": "..."}`) and returns a JSON response. The GraphQL schema is built dynamically by analyzing the FBC meta objects at catalog unpack time, cached per catalog, and invalidated on catalog update or deletion.

This requires four new internal packages layered on top of the existing storage:

| Package | Responsibility |
|---|---|
| `internal/catalogd/graphql` | Schema discovery engine: inspects FBC JSON blobs, infers field types, generates `graphql-go` types |
| `internal/catalogd/service` | `CachedGraphQLService`: schema caching, singleflight dedup, query execution |
| `internal/catalogd/server` | HTTP handlers for all three endpoints, extracted from the former monolithic `storage` package |
| `internal/catalogd/storage` | Pure storage operations, now implements a `CatalogStore` interface consumed by the server layer |

The refactoring also separates HTTP handling from storage for the existing endpoints, improving testability without changing their external behavior.

### Workflow Description

**catalog consumer** is a human or automated client that queries catalog content.

**cluster administrator** is a human who manages feature gates and cluster configuration.

1. The cluster administrator enables the `Tech Preview No Updates` feature for the OpenShift cluster.
2. Cluster-operator `cluster-olm-operator` enables the `GraphQLCatalogQueries` feature gate on the catalogd deployment (via `--feature-gates=GraphQLCatalogQueries=true`).
3. Catalogd unpacks a catalog image. During `Store()`, the storage layer pre-warms the GraphQL schema cache by parsing all FBC meta objects and building a `graphql-go` schema in-memory.
4. The catalog consumer sends a POST request to `/catalogs/{catalog-name}/api/v1/graphql` with a JSON body containing a GraphQL query, e.g.:
   ```json
   {"query": "{ olmpackages(limit: 10) { name defaultChannel } }"}
   ```
4. The server handler validates the request (POST-only, valid catalog name, body size ≤ 1 MB, query length ≤ 100 KB), retrieves the catalog filesystem, and delegates to the `CachedGraphQLService`.
5. The service returns the cached schema (or builds one via singleflight on cache miss), executes the query with `graphql.Do()`, and returns the result.
6. The handler writes the GraphQL JSON response to the client.

**Error handling:** 
- Invalid catalog names return 404. 
- Malformed JSON or empty queries return 400. 
- Schema discovery failures return 500 with a GraphQL error envelope. 
- If the feature gate is disabled, the route is not registered and requests return 404.

### API Extensions

No CRDs, webhooks, or aggregated API servers are introduced. The "only" change is a new HTTP endpoint on catalogd's existing serving port, gated behind a feature gate. The endpoint is internal to the cluster (served over the existing catalogd service with mTLS).

### Topology Considerations

#### Hypershift / Hosted Control Planes

No new impacts. Catalogd runs in the guest cluster's `olmv1-system` namespace. The GraphQL endpoint is served on the same in-cluster service as the existing endpoints. No management-cluster components are affected. HCP Control Plane hoisting is a separate feature and this service would be included in existing feature planning.

#### Standalone Clusters

Fully supported. The feature gate is available on standalone clusters and behaves identically.

#### Single-node Deployments or MicroShift

The GraphQL schema metadata cache (type definitions, field descriptors) adds approximately 100-300 KB per catalog. However, the pre-parsed objects cache — which stores the deserialized JSON for all FBC objects to avoid per-query `json.Unmarshal` — consumes memory roughly equal to the raw catalog size. For the three default OCP catalogs (~60 MB combined), this means up to ~60 MB of additional RSS when the feature gate is enabled. On a typical SNO, this is a meaningful but bounded cost; the cache is released immediately on catalog deletion and rebuilt only on catalog update. CPU cost is amortized: schema building and object parsing happen once per catalog unpack, not per query. MicroShift does not include OLMv1/catalogd, so there is no impact.

#### OpenShift Kubernetes Engine

OKE includes OLMv1 and catalogd. The feature gate defaults to disabled and does not change OKE behavior unless explicitly enabled.

### Implementation Details/Notes/Constraints

**Schema discovery algorithm:** For each FBC meta object, the discovery engine parses the JSON blob, groups objects by their `schema` field (e.g., `olm.package`, `olm.bundle`, `olm.channel`), and inspects field values to infer GraphQL types (String, Int, Float, Boolean, nested Object, Array). Fields present across multiple objects of the same schema are merged. Nested arrays of objects (e.g., `properties` on bundles) generate child GraphQL types.

**Field naming normalization:** FBC field names are sanitized to valid GraphQL identifiers — dots and hyphens become camelCase (e.g., `olm.gvk` → `olmGvk`), leading digits are prefixed with `field_`.  Colliding normalized fields are a fatal error for a given catalog. 

**Root query field naming:** Schema names are stripped of non-alphanumeric characters, lowercased, and pluralized with an `s` suffix. For example, `olm.bundle` → `olmbundles`, `olm.package` → `olmpackages`.

**Caching:** Built schemas are cached per catalog name. Cache invalidation occurs on `Store()` (catalog update) and `Delete()` (catalog removal). A `singleflight.Group` prevents duplicate schema builds when concurrent requests hit a cold cache.

**Pre-parsed objects:** FBC meta blobs are parsed once at schema-build time and stored alongside the schema. This eliminates per-query `json.Unmarshal` overhead at the cost of memory roughly equal to the raw blob size.

**Pagination:** Root query fields accept `limit` (default 100) and `offset` (default 0) arguments.

### Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Memory growth from caching parsed objects for very large catalogs | Pre-parsed object memory ≈ raw catalog size. The largest default OCP catalog (redhat-operators) is ~31 MB of FBC; all three defaults total ~60 MB. The cache is per-catalog and released on deletion. SNO deployments should be profiled during Tech Preview to confirm acceptable RSS overhead. |
| Unbounded query complexity (e.g., deeply nested selections) | Request body capped at 1 MB, query string capped at 100 KB. Future work may add query-depth limits. |
| Schema discovery produces incorrect types for polymorphic fields | Fields with mixed types across objects fall back to `String`. The `value` field in `properties` is serialized as a JSON string for deep nesting. |

### Drawbacks

The dynamic schema discovery approach trades type precision for zero-maintenance adaptability. Deeply nested or polymorphic fields (e.g., `properties[].value`) are serialized as JSON strings rather than fully typed, which limits the GraphQL introspection benefit for those fields. A future enhancement could add specialized type-union handling for well-known property types.

The pluralization strategy (append `s`) does not handle irregular English plurals, but FBC schema names (`olm.bundle`, `olm.package`, `olm.channel`) all pluralize naturally.

## Alternatives (Not Implemented)

**Traditional handwritten GraphQL schema:** Explicitly defining GraphQL types for each FBC schema provides full type safety and richer introspection but requires code changes, testing, and redeployment for every FBC schema update. This was rejected because the proposed alternative allows the opportunity to disintermediate FBC producers and consumers without continued maintenance work or perfect future knowledge.

**OData or REST query parameters:** Extending `/api/v1/metas` with field-selection and projection parameters would avoid adding a GraphQL dependency but would require inventing a bespoke query language and would not provide introspection or nested-object traversal.

**Client-side GraphQL (e.g., GraphQL-over-JSONL):** Serving raw JSONL and letting clients apply GraphQL queries locally avoids server-side schema building but does not reduce network transfer.

## Open Questions [optional]

1. Should the OCP feature gate be named `NewOLMCatalogdGraphQL` (following the `NewOLMCatalogdAPIV1Metas` pattern) or use a different convention?
2. Should query-depth or query-complexity limits be enforced at alpha, or deferred to beta?
3. The pre-parsed objects cache adds ~60 MB RSS for the three default OCP catalogs. On SNO (where memory is constrained), should the cache use a lazy-loading strategy (parse on first query, evict under memory pressure) instead of eager pre-warming? Or is ~60 MB acceptable given that catalogd already holds the raw catalog data on disk?

## Test Plan

**Unit tests (implemented):**
- Schema discovery from FBC meta objects (field type inference, nested structure detection, field name normalization) — `graphql/discovery_test.go` (307 lines)
- GraphQL schema building and query execution — `graphql/graphql_test.go` (381 lines)
- Service layer caching, singleflight, and invalidation — `service/graphql_service_test.go` (331 lines)
- HTTP handler routing, method enforcement, error handling — `server/handlers_test.go` (303 lines)

**OTE tests (drafted):**
- Per-catalog connectivity tests for `openshift-community-operators`, `openshift-certified-operators`, and `openshift-redhat-operators` verifying the endpoint responds to a `summary` introspection query.
- Query-scenario tests verifying field selection (packages without icon), nested property traversal (bundle properties), and cross-type queries (bundles + channels).
- All e2e tests gated on `[OCPFeatureGate:NewOLMCatalogdGraphQL]` and `[Skipped:Disconnected]`.

**Integration strategy:** Tests run in the existing OLMv1 CI jobs when the feature gate is enabled in the experimental manifest. The endpoint is exercised via in-cluster curl Jobs with ServiceAccount authentication.

## Graduation Criteria

### Dev Preview -> Tech Preview

**Note:** the feature is proposed as controlled by the `Tech Preview No Upgrades` flag so there is no distinct `Dev Preview` phase implemented.  Below points represent satisfied criteria in the current proposal.

- Feature gate `GraphQLCatalogQueries` available and functional when explicitly enabled.
- Unit and e2e test coverage for all three query patterns (field selection, nested traversal, multi-type queries).
- Documentation in the catalogd repository (`docs/howto/catalog-queries-graphql-endpoint.md`).
- Gather feedback from operator authors and tooling teams on query ergonomics and schema naming.

### Tech Preview -> GA

- Query-complexity or query-depth limits enforced to prevent abuse.
- Load testing with catalogs containing 5,000+ bundles (comparable to community-operators and operatorhubio at ~4,300-4,800 bundles today) to validate memory and latency bounds.
- Memory profiling on SNO with all three default OCP catalogs (~60 MB combined) to confirm pre-parsed-object cache RSS is acceptable.
- Feature gate defaults to enabled.
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/).
- OCP feature gate `NewOLMCatalogdGraphQL` defined in `openshift/api`.

### Removing a deprecated feature

N/A — this is a new feature.

## Upgrade / Downgrade Strategy

**Upgrade:** The feature gate defaults to disabled. Existing clusters upgrading to a version containing this change see no behavioral difference. Enabling the gate activates the endpoint; disabling it removes the route. No data migration is required — the GraphQL schema is derived entirely from the existing FBC catalog data at runtime.

**Downgrade:** Disabling the feature gate or downgrading to a version without GraphQL support removes the endpoint. No cleanup is needed — the in-memory schema cache is ephemeral. Clients that were using the GraphQL endpoint will receive 404 responses and must fall back to `/api/v1/all` or `/api/v1/metas`.

## Version Skew Strategy

The GraphQL endpoint is self-contained within the catalogd binary. It does not introduce new inter-component RPCs or shared state. During a rolling upgrade, the endpoint is either available (new version with gate enabled) or not (old version / gate disabled). There is no version-skew concern between catalogd and other OLMv1 components — the operator-controller and other consumers do not depend on the GraphQL endpoint.

## Operational Aspects of API Extensions

No CRDs, webhooks, or aggregated API servers are introduced.

The endpoint is an additional HTTP route on catalogd's existing HTTPS serving port. Operational impact:

- **SLIs:** The existing catalogd pod health checks (readiness/liveness probes) cover the serving port. No new SLIs are required; the endpoint's availability is implied by the pod's readiness. Query latency can be observed via standard HTTP metrics if catalogd exposes them.
- **Impact on existing SLIs:** Negligible. The schema cache is built asynchronously during catalog unpack (which already performs JSON parsing). Query execution uses the cached schema and pre-parsed objects — no additional disk I/O. The endpoint shares the same HTTP server and does not affect `/api/v1/all` or `/api/v1/metas` performance.
- **Failure modes:** If schema discovery fails for a catalog (e.g., severely malformed FBC data), the GraphQL endpoint returns a 500 with a descriptive error. The existing endpoints are unaffected. If the `graphql-go` library panics, Go's HTTP server recovers the goroutine and returns 500. No cluster-wide impact.
- **Escalation:** The OLM team (specifically the catalogd maintainers) would handle escalations related to this endpoint.

## Support Procedures

- **Detection:** If the GraphQL endpoint is misbehaving, the catalogd pod logs will show errors from the `graphql` or `service` packages (e.g., `"failed to build GraphQL schema"`, `"failed to execute query"`). The pod's readiness probe will continue to pass as long as the HTTP server is healthy.
- **Disabling:** Set `--feature-gates=GraphQLCatalogQueries=false` on the catalogd deployment and restart the pod. The route is removed immediately. No other consequences — existing endpoints continue serving. No data loss.
- **Impact on existing workloads:** None. No existing OLMv1 component (operator-controller, cluster extensions) uses the GraphQL endpoint. Disabling it affects only external consumers that opted in.
- **Impact on new workloads:** New workloads that depend on the GraphQL endpoint will receive 404 responses when the feature is disabled. They must fall back to the existing REST endpoints.
- **Graceful recovery:** Re-enabling the feature gate and restarting catalogd restores the endpoint. The schema cache is rebuilt from the existing catalog data. No state is lost.

## Infrastructure Needed [optional]

N/A — no new subprojects, repositories, or testing infrastructure are required. The implementation uses the existing `graphql-go/graphql` library (added as a Go module dependency).
