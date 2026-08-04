---
title: test-suites
authors:
  - "@kenzhang"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2026-07-31
last-updated: 2026-07-31
tracking-link:
  - TBD
status: provisional
---

# Test Suites

## Summary

Today, OpenShift test suites use a flat model with static string-tag
assignment. This enhancement proposes migrating to a hierarchical,
label-driven suite composition built on the OpenShift Tests Extension
(OTE) APIs. The new model introduces a structured test lifecycle
(informing → blocking → graduated), explicit sharding for scheduling
flexibility, and a minimal conformance suite containing only core smoke
tests. The goal is to improve test manageability, enable predictable
test graduation, and reduce conformance suite footprint while
increasing overall coverage through better suite organization.

## Motivation

The current flat suite model in `openshift-tests` has grown organically
and presents several challenges: conformance suites are too large,
there is no formal lifecycle for test graduation, sharding is
implicit, and test assignment relies on fragile string-tag pattern
matching. These problems slow down CI, make it difficult for component
teams to introduce and stabilize new tests, and create unpredictable
runtime behavior in constrained environments like vSphere.

The OTE framework (see [openshift-tests-extension](openshift-tests-extension.md))
provides the foundational APIs (`AddSuite`, `AddLabel`, `Informing()`)
needed to build a richer suite hierarchy. This enhancement defines how
those APIs should be used to organize suites, manage test lifecycle,
and balance shards.

### User Stories

* As a member of the quality staff engineer, I want a minimal
  conformance suite that contains only core smoke tests so that
  conformance runs are fast and focused on the most critical
  functionality.
* As an OpenShift product development engineer, I want a clear
  lifecycle for my tests (informing → blocking → graduated) so that
  I can stabilize new tests without risking release signal.
* As a CI infrastructure engineer, I want explicitly sharded suites
  so that I can schedule jobs predictably in resource-constrained
  environments.
* As a component team lead, I want spot-check suites for features
  requiring uncommon cluster configurations so that specialized
  tests do not pollute the main conformance signal.
* As a member of the quality staff engineer, I want shard runtimes
  balanced within 10% of mean so that CI pipelines complete in
  predictable and roughly equal time windows.

### Goals

1. Shrink `openshift/conformance/parallel/minimal` and
   `openshift/conformance/serial/minimal` to contain only core smoke
   tests.
2. Introduce `active` suites for new and stabilizing tests with a
   clear graduation path to `stable` suites.
3. Provide explicit sharding (`stable-01`, `stable-02`, etc.) instead
   of auto-sharding, enabling scheduling flexibility in constrained
   environments.
4. Define a test lifecycle flow with clear pass-rate thresholds and
   duration expectations for each phase.
5. Introduce `spot-check/<feature>` suites for features requiring
   uncommon cluster configurations.
6. Establish a shard-balancing workflow using Sippy data to keep all
   shards within 10% of mean runtime.

### Non-Goals

1. Replacing or deprecating the OTE extension binary mechanism; this
   enhancement builds on top of it.
2. Changing how tests are authored at the individual test level (Ginkgo,
   custom frameworks, etc.).
3. Automating shard rebalancing; this enhancement defines the manual
   workflow only.
4. Defining CI job configurations for each suite; those are managed
   separately in `openshift/release`.

## Proposal

### Workflow Description

**test author** is a developer contributing tests for a component.

**QSE engineer** is a member of the quality staff engineer team responsible
for suite management and test graduation.

#### Adding a New Test

1. The test author writes a new test in their component repository
   using the OTE extension framework.
2. The test author defines a feature suite with `Parents` pointing to
   `openshift/active-01` (parallel) or `openshift/active-serial-01`
   (serial).
3. The test author marks the test as `Informing()` so failures are
   non-blocking during stabilization.
4. The test is picked up in CI through the OTE extension binary
   discovery mechanism and runs in the `active` suite for 2–3
   sprints.

#### Promoting a Test to Blocking

1. After 2–3 sprints, the QSE engineer reviews the test's pass rate.
2. If the pass rate is >= 99%, the QSE engineer removes the
   `Informing()` annotation, making the test blocking within the
   `active` suite.
3. The test remains blocking in `active` until GA + 1 release.

#### Graduating a Test to Stable

1. After GA + 1 release, the QSE engineer changes the feature suite's
   `Parents` from `openshift/active-*` to `openshift/stable-NN`.
2. The QSE engineer selects the appropriate shard based on current
   shard runtimes.
3. The test now runs permanently in the `stable` suite with a >= 99.5%
   pass-rate expectation.

#### Setting Up a Spot-Check Suite

1. The test author identifies tests requiring uncommon cluster
   configurations (e.g., etcd scaling, realtime nodes, external OIDC).
2. The test author creates a suite under `openshift/spot-check/<feature>`
   with dedicated CI jobs using specialized cluster configs.
3. During development, the spot-check job runs approximately 2x daily
   (~14+ runs/week) for pass-rate analysis.
4. Post-GA + 1 release, the frequency drops to approximately 1x/month.
5. Component Readiness flags are triggered if no pass occurs in 30
   days.

### API Extensions

None. This enhancement does not introduce new CRDs, webhooks, or other
API surface changes. It defines conventions for using existing OTE
APIs.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations. The suite hierarchy applies uniformly
regardless of control plane topology. 

#### Standalone Clusters

The suite hierarchy is fully relevant for standalone clusters. The
conformance/minimal, active, and stable suites are the primary suites
run against standalone clusters.

#### Single-node Deployments or MicroShift

No additional resource consumption is introduced by this proposal.

#### OpenShift Kubernetes Engine

No dependency on features excluded from OKE. The suite hierarchy
applies to OKE testing in the same way as OCP.

### Implementation Details/Notes/Constraints

#### Suite Hierarchy

The new hierarchy is built on top of the OTE APIs:

| Suite | Description |
|-------|-------------|
| `openshift/conformance/parallel/minimal` | Core smoke tests only, parallel execution |
| `openshift/conformance/serial/minimal` | Core smoke tests only, serial execution |
| `openshift/active-01` | Recent features, parallel, shard 1 |
| `openshift/active-serial-01` | Recent features, serial, shard 1 |
| `openshift/stable-01` | Graduated tests, parallel, shard 1 |
| `openshift/stable-02` | Graduated tests, parallel, shard 2 |
| `openshift/stable-serial-01` | Graduated tests, serial, shard 1 |
| `openshift/spot-check/<feature>` | Uncommon cluster configs |

#### Test Lifecycle Flow

| Phase | Suite | Pass Rate Threshold | Duration |
|-------|-------|---------------------|----------|
| New / Stabilizing | `active` with `Informing()` | Failures non-blocking | 2–3 sprints |
| Blocking | `active` (remove `Informing()`) | >= 99% | Until GA + 1 release |
| Graduated | `stable-NN` | >= 99.5% | Permanent |
| Specialized | `spot-check/<feature>` | N/A (own job) | Own cadence |

#### Suite Composition with OTE APIs

Feature suites compose into parent suites via the `Parents` field:

```go
// Define a feature suite that composes into active
ext.AddSuite(e.Suite{
    Name:    "mycomponent/feature-x",
    Parents: []string{"openshift/active-01"},
})

// Later, promote to stable by changing the parent
ext.AddSuite(e.Suite{
    Name:    "mycomponent/feature-x",
    Parents: []string{"openshift/stable-01"},
})

// Label tests for suite membership via CEL qualifiers
specs.Select(et.NameContains("[sig-foo] my test")).AddLabel("MY-FEATURE")

ext.AddSuite(e.Suite{
    Name:       "mycomponent/feature-x",
    Qualifiers: []string{`labels.exists(l, l=="MY-FEATURE")`},
    Parents:    []string{"openshift/active-01"},
})

// Mark new tests as non-blocking during stabilization
g.It("should do the thing", ote.Informing(), func() { ... })
```

#### Spot-Check Suite Details

- **Ownership**: Managed by the specific component team.
- **CI Configuration**: Dedicated jobs with specialized cluster
  configs.
- **Development Phase**: ~2x daily (~14+ runs/week) for pass rate
  analysis.
- **Maintenance Phase**: Post-GA + 1 release, frequency drops to
  ~1x/month.
- **Readiness**: Component Readiness flags are triggered if no pass
  occurs in 30 days.
- **Monitoring**: Monitortests collect data but avoid generating
  failure JUnits to prevent alerts on known specialized configs.

#### Shard Balancing

- **Objective**: Maintain all shards within 10% of mean runtime.
- **Workflow**:
  1. Use Sippy to query per-test and per-job runtimes.
  2. Move tests between shards by adjusting `SHARD-NN` labels.
  3. Create new shards when existing ones cannot be rebalanced below
     runtime thresholds.

#### Test Requirements Checklist

Before assigning any test to a suite, ensure the following:

- Has `[Jira:Component]` tag or `ci-test-mapping` entry.
- Has `[sig-XYZ]` tag.
- Includes appropriate `[FeatureGate:XYZ]` or `[Feature:XYZ]`
  annotations.
- For inclusion in standard parallel or serial suites, test duration
  is under 5 minutes (longer requires architect approval).
- Parallel tests are non-disruptive; serial tests successfully restore
  cluster state.
- Results are deterministic with a stable test name (no dynamic
  content).

### Risks and Mitigations

**Risk**: Component teams may resist shrinking conformance and
relocating tests to active/stable suites.
**Mitigation**: The migration can be phased, starting with new tests
using the new hierarchy while existing conformance tests are migrated
gradually. Clear documentation and tooling will ease the transition.

**Risk**: Explicit sharding may become unbalanced over time as tests
are added or removed.
**Mitigation**: A defined shard-balancing workflow using Sippy data,
with periodic reviews by QSE engineers.

**Risk**: Spot-check suites with low run frequency may go stale
without detection.
**Mitigation**: Component Readiness flags trigger if no pass occurs
in 30 days, ensuring stale suites are surfaced automatically.

### Drawbacks

- Explicit sharding requires manual maintenance compared to
  auto-sharding approaches.
- The migration from the flat suite model to the hierarchical model
  will require coordination across many component teams.
- Additional suite definitions increase the complexity of CI job
  configuration in `openshift/release`.

## Open Questions [optional]

1. What is the exact criteria for a test to remain in the minimal
   conformance suites versus being moved to `stable`?
2. Should shard-balancing be automated in the future, and if so, what
   tool should drive it?

## Test Plan

This enhancement is about test infrastructure organization rather
than product functionality. Validation will consist of:

- Verifying that the OTE APIs correctly compose the new suite
  hierarchy.
- Confirming that suite membership via CEL qualifiers produces the
  expected test lists.
- Validating that the `Informing()` lifecycle correctly gates test
  blocking status.
- Running the new suite hierarchy in a shadow CI configuration
  alongside existing suites to compare coverage and runtime.

## Graduation Criteria

### Dev Preview -> Tech Preview

- New suite hierarchy defined and available for opt-in use by
  component teams.
- At least one component team has migrated tests to the new hierarchy.
- Shadow CI jobs validate the new suites produce equivalent coverage.

### Tech Preview -> GA

- All component teams have migrated to the new suite hierarchy.
- Conformance/minimal suites contain only minimal smoke tests.
- Shard balancing workflow is documented and practiced by QSE.
- Old flat suite tags are fully deprecated.

### Removing a deprecated feature

- Announce deprecation of old `[Suite:...]` string tags.
- Provide migration tooling to convert existing suite assignments to
  OTE API calls.
- Remove support for old tags after one full release cycle.

## Upgrade / Downgrade Strategy

Not applicable. This enhancement affects CI test infrastructure only
and does not impact cluster upgrade or downgrade behavior.

## Version Skew Strategy

Not applicable. Suite definitions are part of test infrastructure and
do not participate in cluster version skew.

## Operational Aspects of API Extensions

Not applicable. No API extensions are introduced by this enhancement.

## Support Procedures

Not applicable. This enhancement does not affect production cluster
behavior or supportability.

## Alternatives (Not Implemented)

1. **Auto-sharding instead of explicit sharding**: Automatic sharding
   would reduce maintenance burden but does not provide the scheduling
   flexibility needed in constrained environments like vSphere where
   job parallelism must be carefully controlled.

2. **Keeping a large conformance suite**: Maintaining the current
   conformance suite size would avoid migration effort but continues
   to slow CI and dilute the signal from truly critical smoke tests.

3. **Using CI job configuration alone for suite organization**: Rather
   than defining suites in code via OTE APIs, suites could be managed
   purely through CI job configuration. This was rejected because it
   separates test organization from test definition, making it harder
   for component teams to manage their own test lifecycle.

## Infrastructure Needed [optional]

- Shadow CI jobs in `openshift/release` for validating the new suite
  hierarchy alongside existing suites during the migration period.
- Sippy queries for shard runtime analysis and balancing.
