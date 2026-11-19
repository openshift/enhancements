---
title: featuregate-versioned-config
authors:
  - "@sanchezl"
reviewers:
  - "@JoelSpeed, for openshift/api conventions, please review the adapter pattern and feature gate types"
  - "@everettraven, for openshift/api conventions and feature gate declaration patterns"
approvers:
  - "@JoelSpeed"
api-approvers:
  - "None"
creation-date: 2026-08-04
last-updated: 2026-08-04
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2271
see-also:
  - "/enhancements/config/versioned-config-access.md"
  - "/enhancements/installer/feature-sets.md"
  - "/dev-guide/featuresets.md"
---

# FeatureGate Versioned Config

## Summary

The existing `FeatureGateAccess` interface in library-go is the original
implementation of the version-indexed, change-notifying configuration accessor
pattern. This enhancement refactors `FeatureGateAccess` to become a
backward-compatible adapter over the generic `VersionedConfigAccess[Features]`
interface introduced by the
[Versioned Config Access](versioned-config-access.md) framework.
The public interface, constructor signatures, and all behavioral contracts
remain unchanged. No consuming operator requires modification. This refactor
validates the generic framework in production using the most battle-tested
domain.

## Motivation

The `FeatureGateAccess` implementation in
`library-go/pkg/operator/configobserver/featuregates/simple_featuregate_reader.go`
contains approximately 300 lines of version-matching, startup synchronization,
and change-detection logic. This logic is not specific to feature gates; it
applies to any version-indexed configuration domain. By refactoring
`FeatureGateAccess` to delegate to the generic
`VersionedConfigAccess[Features]`, the domain-specific implementation shrinks
to a thin adapter (~50 lines), and the shared infrastructure gains its most
rigorous validation through the existing FeatureGate test suite.

### User Stories

* As a platform developer maintaining library-go, I want the FeatureGateAccess
  implementation to share infrastructure with TLS and PKI accessors, so that
  bug fixes to version matching or change detection apply to all domains
  automatically.

* As a platform developer consuming FeatureGateAccess in an operator, I want
  the refactor to be invisible to me (same interface, same constructors, same
  behavior), so that I do not need to modify my operator code.

* As a platform developer building the TLS or PKI accessor, I want to see the
  generic framework validated in production first (through the FeatureGate
  refactor), so that I have confidence the generic handles edge cases correctly.

### Goals

1. Refactor `FeatureGateAccess` to delegate to `VersionedConfigAccess[Features]`
   internally, preserving the public interface unchanged.
2. Validate the generic framework using the existing FeatureGate test suite
   without requiring new test cases.
3. Reduce the FeatureGate-specific implementation to a thin adapter layer.

### Non-Goals

1. Changing the FeatureGate CRD or its status shape. The FeatureGate CRD
   already has the correct version-indexed status structure.
2. Changing the `FeatureGateAccess` public interface or constructor signatures.
3. Modifying any consuming operator.
4. Adding new feature gates or changing feature gate behavior.

## Proposal

The `FeatureGateAccess` interface and its constructors (`NewFeatureGateAccess`,
`NewHardcodedFeatureGateAccess`) remain exactly as they are today. The internal
implementation changes from a standalone 300-line implementation to a thin
adapter that delegates to `VersionedConfigAccess[Features]`.

The `FeatureGate` query interface (which provides methods like
`Enabled(featureName)` with panic-on-unknown semantics) continues to wrap the
raw `Features` data. This is a clean example of the framework's design
principle: `T` is data, and query semantics are layered on top by the domain.

### Workflow Description

**platform developer** is an OpenShift engineer who consumes `FeatureGateAccess`
in an operator.

**library-go maintainer** is the engineer performing the refactor.

#### Refactor Process

1. The library-go maintainer implements `VersionedConfigAccess[Features]` in
   the new `pkg/operator/versionedconfig/` package (see
   [versioned-config-access.md](versioned-config-access.md)).
2. The library-go maintainer modifies `defaultFeatureGateAccess` to hold an
   inner `VersionedConfigAccess[Features]` and delegate all operations to it.
3. The library-go maintainer defines a `StatusExtractor[Features]` function
   (`extractFeaturesFromFeatureGate`) that reads `[]FeatureGateDetails` from
   FeatureGate CRD status and converts them to `[]VersionedEntry[Features]`.
4. The library-go maintainer defines an `EqualityFunc[Features]`
   (`featuresEqual`) that compares enabled/disabled feature lists.
5. All existing unit tests, integration tests, and e2e tests pass without
   modification.

#### No Impact on Consuming Operators

1. The platform developer continues to call `NewFeatureGateAccess(...)` with
   the same parameters.
2. The platform developer continues to call `Run(ctx)`,
   `InitialFeatureGatesObserved()`, and `CurrentFeatureGates()` as before.
3. The `FeatureGate` query interface returned by `CurrentFeatureGates()`
   continues to provide `Enabled(name)` with panic-on-unknown semantics.
4. No operator code changes are needed.

### API Extensions

None. The FeatureGate CRD status already has the correct version-indexed shape
(`[]FeatureGateDetails` keyed by `version`). No CRD changes are made.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No change. `FeatureGateAccess` already works correctly in Hypershift. The
refactor preserves the same `desiredVersion` parameter that allows management
and guest cluster operators to find their respective entries.

#### Standalone Clusters

Fully applicable. No behavioral change.

#### Single-node Deployments or MicroShift

**SNO**: No behavioral change.

**MicroShift**: Out of scope. MicroShift does not use `FeatureGateAccess`.

### Implementation Details/Notes/Constraints

#### Adapter Pattern

The refactored `FeatureGateAccess` is a thin adapter:

```go
// FeatureGateAccess interface is UNCHANGED. All existing methods preserved.
type FeatureGateAccess interface {
    SetChangeHandler(featureGateChangeHandlerFn FeatureGateChangeHandlerFunc)
    Run(ctx context.Context)
    InitialFeatureGatesObserved() <-chan struct{}
    CurrentFeatureGates() (FeatureGate, error)
    AreInitialFeatureGatesObserved() bool
}

// NewFeatureGateAccess signature is UNCHANGED.
func NewFeatureGateAccess(
    desiredVersion, missingVersionMarker string,
    clusterVersionInformer v1.ClusterVersionInformer,
    featureGateInformer v1.FeatureGateInformer,
    eventRecorder events.Recorder,
) FeatureGateAccess {
    // Internally delegates to:
    inner := versionedconfig.NewVersionedConfigAccess[Features](
        desiredVersion, missingVersionMarker,
        clusterVersionInformer.Informer(),
        featureGateInformer.Informer(),
        "cluster",
        extractFeaturesFromFeatureGate,
        featuresEqual,
        eventRecorder,
    )
    return &featureGateAdapter{inner: inner}
}
```

The `NewHardcodedFeatureGateAccess` constructor similarly wraps
`NewHardcodedConfigAccess[Features]`:

```go
func NewHardcodedFeatureGateAccess(
    enabled []configv1.FeatureGateName,
    disabled []configv1.FeatureGateName,
) FeatureGateAccess {
    features := Features{Enabled: enabled, Disabled: disabled}
    inner := versionedconfig.NewHardcodedConfigAccess[Features](features)
    return &featureGateAdapter{inner: inner}
}
```

#### T = Features

The data type for feature gates is:

```go
type Features struct {
    Enabled  []configv1.FeatureGateName
    Disabled []configv1.FeatureGateName
}
```

The `FeatureGate` query interface wraps this data to provide `Enabled(name)`
with panic-on-unknown semantics, exactly as it does today.

#### StatusExtractor

The `extractFeaturesFromFeatureGate` function reads `FeatureGateStatus` from
the FeatureGate CRD object and converts `[]FeatureGateDetails` (each keyed by
`version`) into `[]VersionedEntry[Features]`. This is a straightforward field
mapping with no business logic changes.

### Risks and Mitigations

**Risk: FeatureGateAccess refactor introduces regressions.**
Mitigation: The refactor is a pure internal restructuring. The public interface
(`FeatureGateAccess`, `NewFeatureGateAccess`, `NewHardcodedFeatureGateAccess`)
is unchanged. The same unit tests and integration tests apply. The refactor
occurs after the generic accessor is validated with its own test suite.

**Risk: Subtle behavioral differences between old and new implementations.**
Mitigation: Property-based tests can compare old and new implementations for
identical inputs to confirm behavioral equivalence. The existing integration
and e2e test suites provide additional coverage.

### Drawbacks

The refactor adds a dependency on the new `versionedconfig` package. If the
generic package has a bug, it affects FeatureGateAccess (and eventually TLS
and PKI). However, this is also a benefit: a bug fix in the generic package
fixes all consumers simultaneously.

## Design Details

### Test Plan

#### Unit Tests

- **Behavioral equivalence**: Property-based tests comparing old implementation
  output with new adapter output for identical inputs, covering version
  matching, change detection, and missing version behavior.
- **Adapter correctness**: Test that `featureGateAdapter` correctly translates
  between `VersionedConfigAccess[Features]` methods and `FeatureGateAccess`
  methods.
- **StatusExtractor**: Test that `extractFeaturesFromFeatureGate` correctly
  converts `FeatureGateStatus` entries to `VersionedEntry[Features]`.

#### Integration Tests

Run all existing FeatureGate integration tests against the refactored
implementation. No new test cases needed; the refactor must pass all existing
tests unchanged.

#### End-to-End Tests

Run all existing FeatureGate e2e tests. No new test cases needed.

### Graduation Criteria

This refactor is a pure internal library-go change with no API surface. It
ships as GA immediately, validated by the existing FeatureGate test suite.

#### Dev Preview -> Tech Preview

N/A. Ships as GA immediately (internal refactor).

#### Tech Preview -> GA

N/A. Ships as GA immediately (internal refactor).

#### Removing a deprecated feature

N/A. No features are deprecated.

### Upgrade / Downgrade Strategy

**No admin action required.** The refactor is internal to library-go. Operators
compiled against the new library-go behave identically to operators compiled
against the old library-go. There is no runtime state change, no CRD change,
and no configuration change.

On downgrade, operators compiled against the old library-go resume using the
standalone `FeatureGateAccess` implementation. There is no interaction between
old and new implementations because they both read the same FeatureGate CRD
status in the same way.

### Version Skew Strategy

No change from the current FeatureGateAccess behavior. The refactor preserves
the same version-matching logic. See
[versioned-config-access.md](versioned-config-access.md) for the general
version skew strategy.

### Operational Aspects of API Extensions

N/A. No API extensions are added or modified.

#### Failure Modes

No new failure modes. The refactored implementation has the same failure
characteristics as the current implementation:

- If the FeatureGate CRD status does not contain an entry for the operator's
  version, the operator blocks at startup until the entry appears.
- If the FeatureGate CRD is unavailable, the informer watch fails and the
  operator blocks at startup.

These failure modes exist today and are handled by the same mechanisms
(startup timeout, operator health monitoring).

#### Support Procedures

No new support procedures. The refactored implementation produces the same
log messages and events as the current implementation. Existing troubleshooting
documentation applies unchanged.

## Implementation History

N/A. This enhancement is newly proposed.

## Alternatives

### Leave FeatureGateAccess As-Is

The current standalone implementation works. The argument for refactoring is
not to fix a bug but to validate the generic framework and reduce long-term
maintenance burden. If the generic framework is not adopted for TLS and PKI,
the refactor has less value. However, both TLS and PKI are planned (see
[tls-versioned-config.md](tls-versioned-config.md) and
[pki-versioned-config.md](pki-versioned-config.md)), making the refactor
a worthwhile investment.

## Infrastructure Needed

None. All changes are to the existing openshift/library-go repository.
