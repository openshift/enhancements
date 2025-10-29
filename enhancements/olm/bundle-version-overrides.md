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
  - TBD
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

This enhancement introduces an optional `release` field to the registry+v1 Cluster Service Version (CSV) specification, providing a structured mechanism for future identification of release-level versioning within operator bundles. Operator Lifecycle Manager (OLM) will utilize this field when ordering multiple bundles are available for the same operator. 

## Motivation

Currently, FBC lacks a standardized mechanism to recognize the case where an operator is re-packaged rather than altered. Previous SQLite approaches exist using Freshmaker and imperative upgrade graph rebuild.  Implicit with this is the practice of encoding release version information using semver build metadata appended to version strings (e.g., `7.10.2-opr-2+0.1676475747.p`), which conflates the operator's semantic version with its build/release metadata. This is a violation of [the semver spec](https://www.semver.org) and not an appropriate approach given FBC's emphasis on semver as a primary ordering mechanism.  This new approach makes it possible to programmatically distinguish between operator version and release version information, enabling repackaging pathways independent of traditional upversioned republication approaches.

### User Stories

* As a cluster administrator, I want OLM to prioritize bundles with higher release versions when multiple same-version bundles are available, so that I always receive the most recent release of an operator for my environment.

* As an operator author, I want to able to repackage my operator without functional change separately from the semantic version, so that I can clearly communicate both the operator version and build/release information without mixing them in a single representation.

* As a cluster administrator, I want to see clean, readable version information when inspecting operator bundles and CSV resources, so that I can quickly understand what version of an operator is installed and what the corresponding release version is without implicit processing.

* As an OLM maintainer, I want a structured way to track release version information in bundle metadata, so that upgrade path resolution and bundle replacement logic can reliably determine whether a bundle should substitute for another based on release information.

* As a legacy bundle publisher using Freshmaker-style workflows, I want my existing `olm.substitutesFor` annotations with semver build metadata to be automatically interpreted, so that I can adopt this enhancement without modifying my existing publishing processes.

### Goals

* Add an optional `release` field to the CSV specification (omitted if unused to preserve line formats)
* Maintain backward compatibility with existing bundles that encode release information in semver build metadata (i.e. Freshmaker)
* Implement reusable comparators which leverage version and release version in a consistent, comprehensive manner
* Provide catalog template for easy adoption of the release field approach
# Define and enforce a naming scheme associated with the use of the release version to eliminate common errors and preemption relationships
* (Future) Enable OLM to prioritize bundles with higher release versions over those with lower or unspecified release versions during bundle selection

### Non-Goals

* Changing the core OLM bundle selection or upgrade resolution algorithms beyond adding release version comparison
* Deprecating or removing the ability to encode release version in semver build metadata (both approaches should coexist)
* Requiring changes to existing bundles or breaking compatibility with current bundle formats
* Modifying existing SQLite catalog schema or catalog source APIs (this is focused on FBC)
* Defining specific version numbering schemes or release version formats
* Modifying how FBC catalog sources or operator-registry databases are structured at the storage level
* Making the release field mandatory (it remains optional for backward compatibility)

## Proposal

### Workflow Description

When building an operator bundle, an operator author can now optionally specify a `release` field in addition to the `version` field in the CSV. This release field represents the bundle's release-level version, which is distinct from the semantic version.

**Determining Priority:** When OLM needs to select a bundle from multiple available versions, it will prioritize based on release version:
1. The semantic `version` field remains the primary selection criteria in the absence of release information, but where different;
2. Bundles with higher `release` values are ordered before bundles with lower `release` values
3. Bundles with `release` values are ordered before bundles without `release` values

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

**Backward Compatibility with Freshmaker-style Substitution:**

For backward compatibility, when a bundle contains the `olm.substitutesFor` annotation and the version string includes semver build metadata (e.g., `7.10.2-opr-2+0.1676475747.p`), the release information is automatically extracted from the build metadata and populated in the release field.

**Current State (Before):**

A Freshmaker-style bundle with version `7.10.2-opr-2+0.1676475747.p` is processed:
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

The parsing logic respects the legacy ability to encode release version implicitly in semver build metadata by detecting the presence of the `olm.substitutesFor` annotation in the bundle and extracting the build metadata (everything after the `+` in the semver string) as the release version.

**Bundle Ordering Priority (Future):**

When multiple bundles are available for the same package and version, OLM will use the following priority rules:

1. Compare versions, and if equal
1. Compare `release` values if both are present (bundles with higher release values win)
2. Prefer bundles with explicit `release` values over bundles without them (when `substitutesFor` is involved)

This enables scenarios where:
- Freshmaker has published multiple release builds of the same operator version
- OLM automatically selects the highest release version
- Operators with explicit release tracking have priority over those without

### Topology Considerations

#### Hypershift / Hosted Control Planes

The proposed change should have no specific impact to HCP. 

#### Standalone Clusters

The proposed change should have no specific impact to standalone clusters. 

#### Single-node Deployments or MicroShift

The proposed change should have no specific impact to SNO/MicroShift clusters. 

### API Extensions

This enhancement modifies the FBC (File-Based Catalog) package schema in operator-registry by adding an optional `release` field to the package value structure in the declcfg format. This is a pure additive change that does not modify the behavior of existing OLM components beyond adding release-based prioritization.

No CRDs, webhooks, or finalizers are added or modified by this enhancement. The change is confined to the metadata schema for how operator bundles are represented in FBC catalogs and operator-registry databases.

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

The implementation follows a hierarchical extraction strategy (as implemented in [operator-registry PR #1792](https://github.com/operator-framework/operator-registry/pull/1792)):

1. **Primary: Extract from CSV if present** - If the bundle's CSV contains an explicit release annotation (`metadata.annotations['operators.operatorframework.io/release']`), use that value
2. **Fallback: Parse from version build metadata** - If no explicit release annotation exists, check if the version string contains semver build metadata (the portion after `+`)
3. **Extract and validate** - Parse the build metadata using semver prerelease syntax validation to ensure it can be represented as a valid semver prerelease field
4. **Fatal error condition** - If the bundle contains an `olm.substitutesFor` annotation but the build metadata cannot be parsed as a valid release value, the processing fails with a clear error message

When processing bundles with semver build metadata:
- Extract the build metadata portion (everything after the `+`)
- Validate it can be represented as a semver prerelease field
- Populate the `release` field with the extracted and validated value
- Clean the `version` field by removing the build metadata

**Priority Comparison:**

The implementation uses semver's comparison methods for release values, as release values are validated to conform to semver prerelease syntax. This ensures consistent and predictable comparison behavior:
- Empty or unset release values are treated as lowest priority
- Non-empty release values are compared using semver's `PRVersion.Compare` method
- This provides proper ordering for numeric release schemes (e.g., `"0"`, `"1"`, `"2"`), timestamp-based releases (e.g., `"0.20250124"`), or alphanumeric identifiers
- Semver's validation rules ensure release values are well-formed and comparable

**Bundle Metadata:**

The CSV supports specifying the release field through the `operators.operatorframework.io/release` annotation. Operator authors can use this annotation in their CSV manifests:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: example-operator.v1.0.0
  annotations:
    operators.operatorframework.io.release: "2025.01.24.000000"
spec:
  version: 1.0.0
```

**operator-registry Tools:**

The `opm` command and operator-registry library are updated to:
* Parse and extract release information from bundles using the hierarchical extraction strategy
* Validate release values using semver prerelease syntax rules
* Render release fields in FBC catalog output
* Support both explicit CSV annotations and build metadata extraction
* Maintain backward compatibility with bundles lacking release information
* Add catalog template support for repackaging workflows

**Catalog Templates:**

The enhancement includes a new FBC catalog template `substitutesFor` that helps operator authors trivially adopt the release field approach and avoid common error scenarios when implementing Freshmaker-style bundle substitution workflows.

### Risks and Mitigations

**Risk:** Changes to the operator-framework API schema could break existing tools or scripts that parse bundle metadata.

**Mitigation:** The release field is optional and additive, ensuring full backward compatibility. Existing tooling that doesn't read the release field will continue to work unchanged. Only new tooling needs to be aware of the field. All extraction logic is non-breaking and only adds information.

**Risk:** Inconsistent behavior between bundles that explicitly set release vs those that use semver build metadata.

**Mitigation:** The implementation standardizes on the release field by automatically extracting build metadata from legacy Freshmaker-style bundles. This ensures consistent representation regardless of how the information was originally encoded, while allowing both approaches to coexist. The hierarchical extraction strategy (CSV annotation first, then build metadata) ensures predictable behavior.

**Risk:** Fatal errors when processing bundles with `olm.substitutesFor` annotations that have invalid build metadata.

**Mitigation:** The implementation validates that build metadata conforms to semver prerelease syntax. If validation fails for bundles that explicitly require release information (via `olm.substitutesFor`), the processing fails with a clear, actionable error message. This prevents silent failures and ensures bundle integrity.

**Risk:** Tools or UI components might display inconsistent version information if they read from old vs new fields.

**Mitigation:** All extraction and rendering logic is centralized in operator-registry tools, ensuring consistent behavior across the ecosystem. The `opm render` command and related tools will always normalize the representation.

**Risk:** String-based comparison of release values might not match operator expectations for numeric releases.

**Mitigation:** Documentation will provide guidance on using consistent release value formats. Operators can adopt conventions that work well with string comparison.

**Security Review:** No security implications. This is a pure metadata schema change that does not affect the runtime behavior of operators or access control.

**UX Review:** This improvement enhances UX by providing cleaner, more readable version strings without build metadata clutter, and enabling more predictable bundle selection in environments with multiple release builds.

### Drawbacks

* Adds another field to maintain in the API schema, though it's optional and backward compatible
* Requires coordination with documentation and downstream tooling to leverage the new field
* Creates a period where some bundles have explicit release fields and others continue to use semver build metadata (though the implementation bridges this gap automatically)
* String-based comparison may not match all release numbering schemes intuitively without careful formatting (but we feel this is a minimal edge case not borne out by catalog examination)

However, the benefits of cleaner separation of concerns, better programmatic access, improved human readability, and support for Freshmaker workflows significantly outweigh these relatively minor drawbacks. The optional nature of the field ensures no disruption to existing workflows.

## Alternatives (Not Implemented)

### Continue Using Semver Build Metadata Exclusively

**Alternative:** Keep the current approach of encoding release version in semver build metadata without adding an explicit field.

**Why Not:** This makes it difficult to programmatically extract release information, creates inconsistent version strings that are harder to read, doesn't provide a clear API for accessing this information, and doesn't enable reliable priority-based bundle selection.

### Use a Separate Metadata Field in Bundle Images

**Alternative:** Add release version as a separate annotation or metadata field in the bundle image rather than in the CSV schema.

**Why Not:** This would require changes to bundle image formats and tooling, making it a much larger scope change with more moving parts and compatibility concerns. It would also not be automatically reflected in catalog databases.

### Split Version into Major/Minor/Patch/Release Fields

**Alternative:** Redesign the version schema to have separate semantic version components (major, minor, patch) and release fields explicitly.

**Why Not:** This would be a breaking change to the CSV schema and require migration of all existing bundles, which is far beyond the scope of this enhancement.

### Make Release Field Mandatory

**Alternative:** Require all bundles to specify a release field.

**Why Not:** This would break backward compatibility and require updating all existing bundles in the ecosystem, creating significant disruption. The optional nature and backwards-compatibility path ensure smooth adoption.

## Open Questions

* Should we provide validation or normalization for release value formats to ensure consistent comparison? (Likely unnecessary - flexibility is valuable)
* Should we document recommended patterns for release value formatting (e.g., date-based vs numeric)? (Yes, in the implementation phase)
* Should operator-registry tools emit warnings when extracting release from build metadata? (Maybe for observability during migration period)

## Test Plan

This enhancement includes comprehensive testing across multiple levels:

**Unit Tests:**
* Verify release field parsing from CSV annotations
* Validate extraction logic for semver build metadata using the hierarchical strategy
* Test semver-based comparison logic using `PRVersion.Compare` for release values
* Verify validation of release values against semver prerelease syntax
* Test fatal error conditions when `olm.substitutesFor` is present but build metadata cannot be parsed
* Confirm that bundles without release fields continue to function correctly
* Confirm that `substitutesFor` catalog template generates valid FBC

**Integration Tests:**
* Process bundles with `olm.substitutesFor` annotations and verify release extraction from build metadata
* Test bundles with explicit CSV release annotations take precedence over build metadata
* Test `opm render` output with various release value formats
* Verify that bundles with explicit release values are properly represented
* Test the `substitutesFor` catalog template generation
* Confirm that validation failures produce clear error messages
* Confirm backward compatibility with legacy bundles lacking release information

**Priority Selection Tests (Future):**
* Create multiple bundles with different release values and verify OLM selects the highest
* Test scenarios where one bundle has a release and another does not
* Validate edge cases (empty strings, special characters, very long release values)

**Regression Tests:**
* Ensure existing bundle processing workflows continue to function
* Verify that catalog source queries return expected results
* Confirm that OLM upgrade resolution behaves correctly with mixed release information

**E2E Tests:**
* Install operators using bundles with release fields
* Test Freshmaker-style bundle substitution workflows
* Verify that bundle selection prioritizes higher releases correctly
* Confirm that dashboard/UI correctly displays release information when available

The test strategy focuses on ensuring both new functionality and backward compatibility work correctly.

## Graduation Criteria

### Dev Preview -> Tech Preview

- [ ] Release field is added to operator-framework API schema
- [ ] Extraction logic successfully parses semver build metadata from Freshmaker-style bundles
- [ ] `opm` tools can render bundles with both explicit and extracted release fields
- [ ] Unit tests demonstrate correct behavior for various release value formats
- [ ] Integration tests pass with bundles using `olm.substitutesFor` annotations
- [ ] Bundle selection prioritizes releases correctly in test environments
- [ ] Documentation updated to reflect the new optional field and usage patterns

### Tech Preview -> GA

- [ ] All tests passing including end-to-end scenarios with real operator bundles
- [ ] No regressions in existing bundle processing workflows
- [ ] Bundle selection in OLM correctly prioritizes higher release versions
- [ ] Freshmaker integration validated with multiple operator bundles
- [ ] Downstream consumers (Console, UI components) updated to handle and display release field
- [ ] Sufficient time for feedback from operators, maintainers, and catalog publishers
- [ ] User-facing documentation created in openshift-docs
- [ ] Feature available by default with stable API
- [ ] No critical issues reported during tech preview period

### Removing a deprecated feature

Freshmaker is only used with SQLite-based legacy catalogs, so it will never be encountered in the FBC future.

## Upgrade / Downgrade Strategy

### Upgrade

This enhancement is purely additive and requires no migration of existing bundles. When an upgraded version of operator-registry tools or OLM processes existing bundles:

* Bundles without release information continue to work as before
* Bundles with semver build metadata automatically have release information extracted and populated (for Freshmaker-style workflows)
* Bundles can be updated to use explicit release fields going forward
* No user action is required during upgrade
* Existing CSVs, CatalogSources, and Subscriptions continue to function without changes

**Migration Path for Operator Authors:**

Existing operator authors don't need to change anything. The enhancement provides automatic extraction for Freshmaker-style bundles and optional explicit specification for new bundles.

For those who want to start using explicit release values:
1. Add `operators.operatorframework.io.release` annotation to CSV
2. Rebuild and republish bundle
3. No changes needed to publishing infrastructure

### Downgrade

When downgrading to a version of operator-registry or OLM that doesn't support the release field:

* Bundles with explicit release fields will have them ignored (as optional fields)
* The version field remains intact and functional
* No data loss occurs, as the version field is preserved independently
* Bundle selection falls back to existing mechanisms
* No special handling required

**Downgrade is safe** because the release field is completely optional and removing it doesn't affect the core functionality of OLM or bundle management.

## Version Skew Strategy

The release field is entirely optional. Components that don't understand the field will simply ignore it. During version skew scenarios:

* Older operator-registry tools will process bundles with release fields but ignore the field
* Older OLM components will handle CSVs normally without accessing the release field
* Newer components can leverage the release field when available
* Bundle selection falls back to existing priority mechanisms in older components

**Cross-Version Compatibility:**

- Older OLM + Newer Bundles with Release: OLM ignores release field, uses existing selection logic
- Newer OLM + Older Bundles without Release: OLM uses existing selection logic
- Newer OLM + Newer Bundles with Release: OLM uses release-based prioritization
- Mixed environments: Each component handles bundles based on its own version

The design ensures that the enhancement can coexist with older components during upgrades without causing failures or inconsistent behavior.

## Operational Aspects of API Extensions

N/A - This enhancement does not add or modify CRDs, webhooks, finalizers, or other API extensions beyond the metadata schema changes in the operator-registry package representation.

The change is confined to how bundle metadata is represented and does not affect runtime API behavior.

## Support Procedures

This enhancement does not introduce new failure modes. The optional nature of the release field means that:

**Detection:** Standard bundle inspection tools can validate the presence and format of the release field if desired. The `opm render` command will display release fields when present.

**Impact:** Failure to parse or populate the release field does not affect bundle processing. The version field remains authoritative, and bundle selection falls back to existing mechanisms when release information is unavailable or unparseable.

**Remediation:** No remediation needed. If the release field is missing, invalid, or unparseable, bundles continue to use the version field as before. Bundle functionality is unaffected.

**Monitoring:** No new monitoring is required as this is purely a metadata enhancement. Bundle selection and installation proceed normally with or without release information.

**Documentation:** Support procedures remain unchanged. Troubleshooting bundle issues follows existing patterns regardless of the presence or absence of release fields.

