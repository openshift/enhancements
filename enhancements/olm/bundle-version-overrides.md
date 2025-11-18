---
title: bundle-version-overrides
authors:
  - "@grokspawn"
reviewers:
  - "@joelanford"
  - "@rashmigottapati"
  - "@trgeiger"
approvers:
  - "@joelanford"
api-approvers:
  - "@everettraven"
creation-date: 2025-10-24
last-updated: 2025-10-27
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-2232"
see-also:
  - "/https://docs.google.com/document/d/1tX0fXYuflTpTal6z0TbNkrQ7SAkYIKxQj4cPjlFAh4Q/"
  - "https://docs.google.com/document/d/14LNdxplPKB8mbfKLVvkHHsvboh4FEJOp0thzPNGgnzA/"
  - "https://github.com/operator-framework/api/pull/454"
  - "https://github.com/operator-framework/operator-registry/pull/1792"
replaces: []
superseded-by: []
---

# Bundle Version Overrides

## Summary

This enhancement adds an optional `release` field to the CSV specification, enabling structured release-level versioning within operator bundles. OLM will use this field when ordering multiple bundles for the same operator. 

## Motivation

FBC lacks a standardized mechanism for operator repackaging. Current approaches encode release information in semver build metadata (e.g., `7.10.2-opr-2+0.1676475747.p`), conflating operator version with build/release metadata. This violates [the semver spec](https://www.semver.org) and undermines FBC's reliance on semver ordering. This enhancement enables programmatic distinction between operator version and release version, supporting repackaging workflows without version changes.

### User Stories

* As a cluster administrator, I want OLM to prioritize bundles with higher release versions when multiple same-version bundles are available.
* As an operator author, I want to repackage operators without changing semantic version, clearly separating operator version from build/release information.
* As an OLM maintainer, I want structured release version tracking in bundle metadata for reliable upgrade path resolution.
* As a legacy bundle publisher, I want existing `olm.substitutesFor` annotations with semver build metadata automatically interpreted without modifying publishing processes.

### Goals

* Add optional `release` field to CSV specification
* Maintain backward compatibility with Freshmaker-style semver build metadata
* Implement consistent version/release comparators
* Provide catalog template for release field adoption
* (Future) Enable OLM to prioritize bundles by release version

### Non-Goals

* Changing core OLM algorithms beyond adding release comparison
* Deprecating semver build metadata approach (both coexist)
* Breaking compatibility with existing bundles
* Modifying SQLite catalog schema or APIs
* Making release field mandatory

## Proposal

### Workflow Description

Operator authors can optionally specify a `release` field alongside the `version` field in the CSV, representing release-level versioning distinct from semantic version.

**Priority Ordering:** When OLM selects between multiple bundles:
1. Semantic `version` field remains primary
2. Higher `release` values ordered before lower values
3. Bundles with `release` values ordered before those without

**Explicit Release Specification:**

An operator author builds a bundle with explicit release information:
```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: my-operator-v1.0.0-0.20250124000000
spec:
  version: 1.0.0
  release: 0.20250124000000
  # ... rest of CSV
```

This bundle, when rendered by `opm`, produces:
```yaml
schema: olm.bundle
name: amq-broker-operator.v
type: olm.bundle
...
properties:
   - type: olm.package
     value:
       packageName: my-operator
       version: 1.0.0
       release: 0.20250124000000
...
```

**Backward Compatibility:**

Bundles with `olm.substitutesFor` annotation and semver build metadata (e.g., `7.10.2-opr-2+0.1676475747.p`) automatically extract release information from build metadata.
```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
...
spec:
...
  version: 7.12.4-opr-1+0.1747217191.p
...
```

**Proposed State (After):**

The same bundle now populates both fields:
```yaml
schema: olm.bundle
name: amq-broker-rhel8.v7.10.2-opr-2+0.1676475747.p
type: olm.package
value:
  packageName: amq-broker-rhel8
  version: 7.10.2-opr-2
  release: 0.1676475747.p
```

Build metadata (everything after `+`) is extracted as release version when `olm.substitutesFor` annotation is present.

**Bundle Ordering Priority (Future):**

Priority rules for multiple bundles:
1. Compare versions first
2. If equal, compare `release` values (higher wins)
3. Prefer bundles with `release` over those without

### Topology Considerations

#### Hypershift / Hosted Control Planes

The proposed change should have no specific impact to HCP. 

#### Standalone Clusters

The proposed change should have no specific impact to standalone clusters. 

#### Single-node Deployments or MicroShift

The proposed change should have no specific impact to SNO/MicroShift clusters. 

### API Extensions

Adds optional `release` field to FBC package schema in operator-registry declcfg format. Pure additive change—no CRDs, webhooks, or finalizers modified. Change confined to bundle metadata schema in FBC catalogs and operator-registry databases.

### Implementation Details/Notes/Constraints

**Schema Changes:**

The package schema in operator-registry (FBC declcfg) is extended to include an optional `release` field:

```go
type Package struct {
    PackageName string `json:"packageName"`
    Version     string `json:"version"`
    Release     string `json:"release,omitzero"` // New optional field
}
```

**Extraction Logic:**

Hierarchical extraction strategy ([operator-registry PR #1792](https://github.com/operator-framework/operator-registry/pull/1792)):

1. Primary: Extract from CSV `spec.release` field
2. Fallback: Parse from version build metadata (portion after `+`)
3. Validate using semver prerelease syntax
4. Fatal error if `olm.substitutesFor` present but build metadata invalid

Build metadata processing: extract, validate, populate `release` field, clean `version` field.

**Priority Comparison:**

Reusable comparison methods consider both version and release fields. Release values validated as semver prerelease syntax. Empty values = lowest priority. Supports numeric, timestamp-based, or alphanumeric schemes.

**Bundle Metadata:**

CSV supports `spec.release` field:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: example-operator.v1.0.0
spec:
  version: 1.0.0
  release: 2025.01.24.000000
```

**operator-registry Tools:**

`opm` command and operator-registry library updated to:
* Parse and extract release using hierarchical strategy
* Validate release values using semver prerelease syntax
* Render release fields in FBC output
* Support explicit spec.release and build metadata extraction
* Maintain backward compatibility
* Add `substitutesFor` catalog template for repackaging workflows

### Implementation Phases

Three-phase implementation:

#### Phase 1: API and Operator Registry (operator-framework/api and operator-framework/operator-registry)

**Scope:**
- Add optional `release` field to CSV specification
- Implement release field extraction in operator-registry
- Update `opm` tooling for parsing and rendering
- Add `substitutesFor` catalog template

**Deliverables:**
- [ ] API schema update with `release` field in Package struct
- [ ] Hierarchical extraction logic (spec.release → build metadata)
- [ ] Semver prerelease validation
- [ ] Fatal error handling for invalid build metadata with `olm.substitutesFor`
- [ ] Reusable bundle version comparison library methods (version + release)
- [ ] `opm render` support for release field
- [ ] Catalog template for `substitutesFor` workflows
- [ ] Unit and integration tests

**Dependencies:** None

**Status:** In Progress - See PRs:
- https://github.com/operator-framework/api/pull/454
- https://github.com/operator-framework/operator-registry/pull/1792

#### Phase 2: OLMv1 (operator-framework/operator-controller)

**Scope:**
- Implement bundle selection prioritization by release
- Add feature gate for release-based ordering
- Leverage reusable bundle version comparison methods from Phase 1

**Deliverables:**
- [ ] Feature gate `ReleaseVersionPriority` (disabled by default)
- [ ] Bundle selection logic prioritizing higher release values
- [ ] E2E tests
- [ ] Feature gate documentation

**Dependencies:** Phase 1

**Feature Gate:** `ReleaseVersionPriority` (default: disabled)

**Status:** Pending

#### Phase 3: OLMv0 Integration

**Scope:**
- Ensure OLMv0 gracefully handles bundles with release fields
- Maintain backward compatibility (passive support only, no feature gate)
- Validate existing workflows unchanged

**Deliverables:**
- [ ] Verify OLMv0 catalog-operator ignores release field without errors
- [ ] Confirm CSVs with release processed normally
- [ ] Regression testing for Freshmaker-style bundles
- [ ] Documentation noting OLMv0 won't use release for prioritization

**Dependencies:** Phase 1

**Note:** Release field purely additive; OLMv0 safely ignores it. No feature gate support in OLMv0.

**Status:** Pending

#### Cross-Phase Considerations

**Testing Strategy:**
- Phase 1: parsing, extraction, validation
- Phase 2: selection logic and prioritization
- Phase 3: backward compatibility and regression
- All phases: integration tests with real bundles

**Rollout Strategy:**
- Phase 1: independent deployment (tools update)
- Phase 2: feature gate disabled by default
- Phase 3: validates alongside Phase 1/2
- Progressive enablement after sufficient testing

### Risks and Mitigations

**Risk:** API schema changes break existing tools parsing bundle metadata.
**Mitigation:** Release field optional and additive—full backward compatibility. Existing tooling unaffected.

**Risk:** Inconsistent behavior between explicit release vs semver build metadata.
**Mitigation:** Automatic extraction from Freshmaker-style bundles ensures consistent representation. Hierarchical extraction (spec.release → build metadata) provides predictable behavior.

**Risk:** Fatal errors processing `olm.substitutesFor` bundles with invalid build metadata.
**Mitigation:** Semver prerelease validation with clear error messages prevents silent failures.

**Risk:** Inconsistent version display across tools/UIs.
**Mitigation:** Extraction and rendering centralized in operator-registry. `opm render` normalizes representation.

**Risk:** String comparison doesn't match expectations for numeric releases.
**Mitigation:** Documentation provides guidance on consistent formats.

**Security Review:** No security implications—pure metadata schema change.

**UX Review:** Improves readability by separating version from build metadata; enables predictable bundle selection.

### Drawbacks

* Adds optional field to API schema
* Requires documentation and tooling coordination
* Mixed bundle formats during transition (automatic extraction bridges gap)
* String comparison may not intuitively match all numbering schemes

Benefits (cleaner separation, programmatic access, readability, Freshmaker support) outweigh drawbacks. Optional field ensures no workflow disruption.

## Alternatives (Not Implemented)

### Continue Using Semver Build Metadata Exclusively
**Why Not:** Difficult to extract programmatically, inconsistent version strings, no clear API, unreliable priority-based selection.

### Use a Separate Metadata Field in Bundle Images
**Why Not:** Larger scope, bundle image format changes, not reflected in catalog databases.

### Split Version into Major/Minor/Patch/Release Fields
**Why Not:** Breaking change requiring migration of all existing bundles.

### Make Release Field Mandatory
**Why Not:** Breaks backward compatibility, requires updating all existing bundles.

## Open Questions

* Validation/normalization for release formats? (Likely unnecessary—flexibility valuable)
* Document recommended formatting patterns? (Yes, during implementation)
* Emit warnings when extracting from build metadata? (Maybe for migration observability)

## Test Plan

**Unit Tests:**
* Release field parsing from CSV spec.release
* Hierarchical extraction logic (spec.release → build metadata)
* Reusable comparison methods for version + release
* Semver prerelease validation
* Fatal errors for invalid `olm.substitutesFor` build metadata
* Bundles without release fields function correctly
* `substitutesFor` catalog template generates valid FBC

**Integration Tests:**
* Release extraction from `olm.substitutesFor` bundles
* spec.release precedence over build metadata
* `opm render` with various release formats
* `substitutesFor` template generation
* Clear validation error messages
* Backward compatibility with legacy bundles

**Priority Selection Tests (Future):**
* OLM selects highest release value
* Bundles with/without release fields
* Edge cases (empty strings, special characters, long values)

**Regression Tests:**
* Existing workflows unchanged
* Catalog source queries correct
* OLM upgrade resolution with mixed release information

**E2E Tests:**
* Install operators with release fields
* Freshmaker-style substitution workflows
* Bundle selection prioritization
* Dashboard/UI display

## Graduation Criteria

### Dev Preview -> Tech Preview

**Phase 1 (API/Registry):**
- [ ] Release field in API schema
- [ ] Extraction logic parses Freshmaker-style build metadata
- [ ] `opm` renders explicit and extracted release fields
- [ ] Semver prerelease validation
- [ ] Fatal errors for invalid `olm.substitutesFor` metadata
- [ ] Tests pass
- [ ] `substitutesFor` catalog template available

**Phase 2 (OLMv1):**
- [ ] `ReleaseVersionPriority` feature gate implemented
- [ ] Bundle selection prioritizes by release (gate enabled)
- [ ] E2E tests demonstrate ordering
- [ ] Feature gate documentation

**Phase 3 (OLMv0):**
- [ ] OLMv0 compatibility verified
- [ ] Regression tests pass
- [ ] No breaking changes

**Documentation:**
- [ ] API changes
- [ ] Feature gate usage
- [ ] Migration guidance
- [ ] OLMv0 vs OLMv1 differences

### Tech Preview -> GA

**Phase 1 (API/Registry):**
- [ ] Tests pass across production catalogs
- [ ] No rendering/validation regressions
- [ ] Freshmaker integration validated
- [ ] Negligible performance overhead

**Phase 2 (OLMv1):**
- [ ] `ReleaseVersionPriority` enabled by default
- [ ] Bundle selection prioritizes correctly
- [ ] No critical issues
- [ ] User feedback incorporated
- [ ] Dashboard/Console displays release

**Phase 3 (OLMv0):**
- [ ] Production compatibility verified
- [ ] No issues with mixed OLMv0/v1

**Cross-Phase:**
- [ ] Minimum 2 releases for feedback
- [ ] User-facing documentation in openshift-docs
- [ ] Support procedures documented
- [ ] Stable API, enabled by default

### Removing a deprecated feature

Freshmaker is only used with SQLite-based legacy catalogs, so it will never be encountered in the FBC future.

## Upgrade / Downgrade Strategy

### Upgrade

Purely additive—no migration required:
* Bundles without release information work unchanged
* Build metadata automatically extracted for Freshmaker-style workflows
* Bundles can adopt explicit release fields
* No user action required
* Existing CSVs, CatalogSources, Subscriptions unchanged

**Migration Path:**
No changes required. For explicit release values:
1. Add `spec.release` to CSV
2. Rebuild and republish bundle
3. No publishing infrastructure changes

### Downgrade

Safe—release field completely optional:
* Explicit release fields ignored by older versions
* Version field intact and functional
* No data loss
* Bundle selection falls back to existing mechanisms
* No special handling required

## Version Skew Strategy

Release field entirely optional—unknown components ignore it.

**Cross-Version Compatibility:**
- Older OLM + Newer Bundles: Ignores release, uses existing logic
- Newer OLM + Older Bundles: Uses existing logic
- Newer OLM + Newer Bundles: Uses release-based prioritization
- Mixed environments: Components handle bundles per their version

No failures or inconsistent behavior during upgrades.

## Operational Aspects of API Extensions

N/A - No CRDs, webhooks, or finalizers added/modified. Change confined to bundle metadata schema; no runtime API impact.

## Support Procedures

No new failure modes. Release field optional.

**Detection:** Standard tools validate release field. `opm render` displays when present.

**Impact:** Parse failures don't affect bundle processing. Version field authoritative; selection falls back to existing mechanisms.

**Remediation:** None needed. Missing/invalid release fields handled gracefully—bundles use version field.

**Monitoring:** No new monitoring required. Bundle selection/installation proceed normally.

**Documentation:** Support procedures unchanged. Troubleshooting follows existing patterns.

