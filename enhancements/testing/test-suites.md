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
tests. Lifecycle transitions and shard balancing are driven by a
scheduled automation agent rather than manual toil, so tests do not
languish in the `informing` state indefinitely. The goal is to improve
test manageability, enable predictable test graduation, and reduce
conformance suite footprint while increasing overall coverage through
better suite organization.

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

A recurring problem with the manual model is that lifecycle transitions
require a human to notice that a test is eligible and then open a pull
request. In practice this rarely happens: `informing` tests are left in
place indefinitely, in both good states (long since stable, never
promoted) and bad states (chronically failing, never removed). This
enhancement therefore makes an automation agent — not a human — the
primary driver of promotion, graduation, and shard balancing, closing
that gap from the start.

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
* As a member of the quality staff engineer, I want an automation agent
  to detect promotion-eligible tests and imbalanced shards and open the
  corresponding pull requests so that lifecycle maintenance happens
  continuously without manual toil.

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
7. Provide an automation agent (a scheduled prow job) that drives
   promotion, graduation, and shard rebalancing by opening pull
   requests, keeping humans out of the routine path.

### Non-Goals

1. Replacing or deprecating the OTE extension binary mechanism; this
   enhancement builds on top of it.
2. Changing how tests are authored at the individual test level (Ginkgo,
   custom frameworks, etc.).
3. Fully autonomous merging. The automation agent opens pull requests;
   the PRs still require approval (see the response-window policy in
   [Risks and Mitigations](#risks-and-mitigations)). This enhancement
   does not remove human approval from the loop.
4. Defining CI job configurations for each suite; those are managed
   separately in `openshift/release`.

## Proposal

### Workflow Description

**test author** is a developer contributing tests for a component.

**lifecycle agent** is a scheduled prow job (see
[The Lifecycle Agent](#the-lifecycle-agent)) that scans the test
inventory, evaluates pass rates, and opens pull requests to promote,
graduate, rebalance, or flag tests. It replaces the manual QSE steps
described in earlier drafts of this proposal.

**QSE engineer** is a member of the quality staff engineer team who
reviews and approves the pull requests opened by the lifecycle agent
and handles the exceptions the agent escalates.

#### Adding a New Test

1. The test author writes a new test in their component repository
   using the OTE extension framework.
2. The test author defines a feature suite with `Parents` pointing to
   `openshift/active-01` (parallel) or `openshift/active-serial-01`
   (serial).
3. The test author marks the test as `Informing()` so failures are
   non-blocking during stabilization.
4. The test is picked up in CI through the OTE extension binary
   discovery mechanism and runs in the `active` suite for at least the
   minimum stabilization window (2–3 sprints) before it is eligible for
   promotion.

#### Promoting a Test to Blocking

Promotion is performed by the lifecycle agent, not by hand.

1. On its schedule, the agent locates each `Informing()` test in the
   `active` suites (see [Locating Tests](#locating-tests)) and computes
   its pass rate over the [measurement window](#pass-rate-calculation).
2. If the test has been in `active` for at least the minimum
   stabilization window, has at least the minimum sample count of
   qualifying runs, and meets the `>= 99%` pass rate, the agent opens a
   pull request removing the `Informing()` annotation, making the test
   blocking within the `active` suite.
3. A QSE engineer (or, after the response window, an architect or staff
   engineer) approves the pull request.
4. The test remains blocking in `active` until GA + 1 release.

Promotion is one-way. Once a test is made blocking, it is never
demoted back to `Informing()` and it is never moved to a lower-signal
suite. If its pass rate later degrades, that is treated as a
**regression to be fixed**, not as grounds for quarantine or downgrade.
The agent surfaces the degradation (and the owning component is expected
to fix it) rather than relaxing the test's lifecycle state. See
[Pass-Rate Calculation](#pass-rate-calculation).

#### Graduating a Test to Stable

Graduation is also performed by the lifecycle agent.

1. After GA + 1 release, the agent selects the target stable shard
   suite based on current shard runtimes: a parallel test maps to an
   explicit `openshift/stable-NN` suite (e.g. `openshift/stable-01`) and
   a serial test maps to an explicit `openshift/stable-serial-NN` suite
   (e.g. `openshift/stable-serial-01`). The generic `stable-NN` form is
   never written into a suite definition; a concrete shard is always
   chosen.
2. The agent opens a single pull request that **replaces** the feature
   suite's `active` parent with the chosen stable parent, and in the
   same change updates the test's shard/lifecycle label so that
   `Parents` and labels stay consistent. Because OTE merges `Parents`
   additively across extension binaries, the old advertisement must be
   removed in the same commit that adds the new one; otherwise the test
   would remain a member of both the `active` and `stable` suites. This
   consistency requirement is why every test must carry a suite/shard
   label (enforced in CI; see
   [Mandatory Labelling](#mandatory-labelling)) — an unlabelled test
   cannot be reliably moved between suites. See
   [Exclusive Membership](#exclusive-membership-and-rollout).
3. Once merged, the test runs permanently in the chosen stable shard.
   At this point the test is no longer gated by a hard suite pass-rate
   threshold; its health is governed by Component Readiness, which
   holds a roughly 95% pass rate (a promoted test near 100% may drift
   down by up to Component Readiness's tolerance, currently ~5%).

#### Setting Up a Spot-Check Suite

1. The test author identifies tests requiring uncommon cluster
   configurations (e.g., etcd scaling, realtime nodes, external OIDC).
2. The test author creates a suite under `openshift/spot-check/<feature>`
   with dedicated CI jobs using specialized cluster configs.
3. During development, the spot-check job runs frequently for pass-rate
   analysis (cadence chosen by the owning team; e.g., ~2x daily, or
   ~1x/week for expensive configs such as etcd-scaling).
4. Post-GA + 1 release, the frequency drops to approximately 1x/month.
5. Spot-check suite health is monitored by Component Readiness. The
   lifecycle agent does not manage spot-check suites or jobs.

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
| `openshift/conformance/parallel` | Aggregate parallel conformance; existing OTE parent that all parallel conformance tests roll up into |
| `openshift/conformance/serial` | Aggregate serial conformance |
| `openshift/conformance/parallel/minimal` | Core smoke tests only, parallel execution |
| `openshift/conformance/serial/minimal` | Core smoke tests only, serial execution |
| `openshift/active-01` | Recent features, parallel, shard 1 |
| `openshift/active-serial-01` | Recent features, serial, shard 1 |
| `openshift/stable-01` | Graduated tests, parallel, shard 1 |
| `openshift/stable-02` | Graduated tests, parallel, shard 2 |
| `openshift/stable-serial-01` | Graduated tests, serial, shard 1 |
| `openshift/spot-check/<feature>` | Uncommon cluster configs |

The `openshift/conformance/parallel` and `openshift/conformance/serial`
suites already exist in OTE as the aggregate roll-up parents (see
[openshift-tests-extension](openshift-tests-extension.md)). The
`.../minimal` suites introduced here are strict subsets used for the
fast smoke signal; a test in `minimal` is also reachable through the
broader `conformance/parallel` aggregate. Tests that are not core smoke
tests land in `active`/`stable`, not in a bare `conformance` suite.

The `<feature>` portion of a `spot-check/<feature>` name is
informational for human readers and for organizing the dedicated prow
jobs. It carries no OTE-level functionality. The lifecycle agent does
not manage spot-check suites or jobs; their health is monitored by
Component Readiness.

#### Minimal-Suite Membership

The precise rule for what belongs in `conformance/*/minimal` is **not
yet fully defined** and remains an open question (see
[Open Questions](#open-questions-optional)). Two cases differ:

- **Upstream Kubernetes tests**: minimal membership is selected
  mechanically by the upstream `[Conformance]` label. Tests carrying
  that label are placed in `conformance/*/minimal`; the mapping is
  applied centrally rather than judged test-by-test (see
  [Upstream and Externally-Sourced Tests](#upstream-and-externally-sourced-tests)).
- **Downstream (OpenShift-authored) tests**: there is no equivalent
  established signal. Deciding which downstream tests deserve to be
  labelled/treated as conformance-minimal still requires evaluation,
  likely a manual review per component to begin with. Defining an
  objective, repeatable rule for this is tracked as an open question.

Note that a test is not confined to a single membership: a minimal test
also participates in an `active` or `stable` shard. Minimal is an
additional, faster smoke selection layered on top of the lifecycle
shard, not a mutually exclusive alternative to it (see
[Upstream and Externally-Sourced Tests](#upstream-and-externally-sourced-tests)
for how this is expressed).

#### Test Lifecycle Flow

| Phase | Suite | Pass Rate Threshold | Duration |
|-------|-------|---------------------|----------|
| New / Stabilizing | `active` with `Informing()` | Failures non-blocking | >= 2–3 sprints |
| Blocking | `active` (remove `Informing()`) | >= 99% over measurement window | Until GA + 1 release |
| Graduated | `stable-01`, `stable-02`, `stable-serial-01`, … (explicit shard) | Governed by Component Readiness (~95%) | Permanent |
| Specialized | `spot-check/<feature>` | N/A (own job) | Own cadence |

The "Graduated" row intentionally has no hard suite-level threshold:
once a test is in a `stable` shard, Component Readiness owns its health
signal at its configured tolerance (roughly 95%), rather than a fixed
`>= 99.5%` gate.

#### Suite Composition with OTE APIs

Feature suites compose into parent suites via the `Parents` field:

```go
// Label tests for suite membership. Qualifiers use the documented
// OTE CEL contract: test.labels.has("...") / test.name.contains("...").
specs.Select(et.NameContains("[sig-foo] my test")).AddLabel("MY-FEATURE")

// Define a feature suite that composes into an active shard.
ext.AddSuite(e.Suite{
    Name:       "mycomponent/feature-x",
    Qualifiers: []string{`test.labels.has("MY-FEATURE")`},
    Parents:    []string{"openshift/active-01"},
})

// Graduating to stable is an EXCLUSIVE parent change: the active
// parent is removed in the SAME change that adds the stable parent.
// OTE merges Parents additively across extension binaries, so leaving
// both advertisements in place would make the test a member of both
// suites. A parallel test maps to an explicit stable-NN shard; a
// serial test maps to an explicit stable-serial-NN shard.
ext.AddSuite(e.Suite{
    Name:       "mycomponent/feature-x",
    Qualifiers: []string{`test.labels.has("MY-FEATURE")`},
    Parents:    []string{"openshift/stable-01"}, // was openshift/active-01
})

// Mark new tests as non-blocking during stabilization
g.It("should do the thing", ote.Informing(), func() { ... })
```

##### Exclusive Membership and Rollout

Because `Parents` is additive, both a promotion and a graduation are
expressed as a single edit that removes the previous parent and adds
the new one in the same commit. The lifecycle agent never opens a
pull request that adds a new parent without deleting the prior one, so
a test is a member of exactly one lifecycle shard (`active-*` or
`stable-*`) at any point in time. Reviewers should reject any suite
change that leaves a feature advertised under two lifecycle parents.

##### Shard Membership Qualifiers

Each stable shard suite is defined by a qualifier that consumes the
shard label the balancing workflow assigns, e.g.:

```go
ext.AddSuite(e.Suite{
    Name:       "openshift/stable-01",
    Qualifiers: []string{`test.labels.has("SHARD-01")`},
})
ext.AddSuite(e.Suite{
    Name:       "openshift/stable-02",
    Qualifiers: []string{`test.labels.has("SHARD-02")`},
})
```

A test carries exactly one `SHARD-NN` label, so it belongs to exactly
one stable shard. Moving a test between shards means swapping its single
`SHARD-NN` label (again, remove-and-add in the same change), which keeps
shard membership mutually exclusive.

##### Mandatory Labelling

Every test **must** carry the labels that determine its suite and shard
membership. This is a hard requirement, not a convention, for two
reasons:

1. Graduation and rebalancing change a test's `Parents` and its
   shard/lifecycle label together; if a test is unlabelled, the two can
   drift out of sync and the automation cannot reliably move it.
2. Membership must be deterministic — an unlabelled test could silently
   fall out of every shard.

To enforce this, a validation check runs in CI (in the extension binary
self-check and/or as a required presubmit) that **fails** if any
discovered test lacks the required suite/shard label. Tests cannot merge
without a label. This guarantees `Parents` and labels remain consistent
across the lifecycle.

For large, externally-sourced test sets (e.g. upstream Kubernetes),
labels are applied centrally in a dedicated function that walks the
specs and assigns labels from a rules table, rather than inline on each
test:

```go
// Centralized labelling for a bulk-imported test set, e.g. upstream k8s.
// Runs over all specs and assigns membership labels from a rules table,
// instead of editing each test inline.
allSpecs.Walk(func(spec *et.ExtensionTestSpec) {
    if spec.Labels.Has("Conformance") { // upstream [Conformance]
        spec.AddLabel("MINIMAL")
    }
    spec.AddLabel(shardFor(spec)) // e.g. SHARD-01, always assigned
})
```

#### Spot-Check Suite Details

- **Ownership**: Managed by the specific component team.
- **CI Configuration**: Dedicated jobs with specialized cluster
  configs.
- **Development Phase**: Frequent runs for pass-rate analysis; cadence
  is chosen by the owning team based on cost (e.g., ~2x daily for cheap
  configs, ~1x/week for expensive ones such as etcd-scaling, ideally
  paired with an automatic retry-on-failure mechanism once available).
- **Maintenance Phase**: Post-GA + 1 release, frequency drops to
  ~1x/month.
- **Health monitoring**: Spot-check suite health is monitored by
  Component Readiness. The lifecycle agent does not manage spot-check
  suites or jobs.
- **Monitoring**: Monitortests collect data but avoid generating
  failure JUnits to prevent alerts on known specialized configs.

#### Shard Balancing

- **Objective**: Maintain all shards within 10% of mean runtime.
- **Driver**: The lifecycle agent, on a schedule, performs the workflow
  below and opens the resulting pull requests. QSE engineers review
  them; they are not authored by hand.
- **Workflow**:
  1. Query Sippy for per-test and per-job runtimes.
  2. Move tests between shards by swapping each test's single
     `SHARD-NN` label (remove-and-add in one change to preserve
     exclusive shard membership).
  3. Create new shards when existing ones cannot be rebalanced below
     runtime thresholds.

#### The Lifecycle Agent

The lifecycle agent is a scheduled prow job (weekly or per-sprint) that
removes routine suite maintenance from the human path. On each run it:

1. Enumerates the test inventory (see [Locating Tests](#locating-tests)).
2. For each `Informing()` test in `active`, evaluates promotion
   eligibility (stabilization window elapsed, sample count met, pass
   rate `>= 99%` over the measurement window) and opens a PR removing
   `Informing()` when eligible.
3. For each blocking `active` test past GA + 1, opens a PR that performs
   the exclusive `active` → explicit `stable`/`stable-serial` shard
   parent change, updating the shard label in the same change.
4. Recomputes shard balance from Sippy and opens rebalancing PRs.

Promotion is one-way: the agent never demotes a blocking test back to
`Informing()` and never moves a test to a lower suite. A subsequent
pass-rate degradation is a regression to be fixed by the owning
component, not something the agent reverses. The agent also does not
manage spot-check suites or jobs.

Precedent exists for this style of automated, PR-driven maintenance in
the TRT tooling ecosystem. All actions are proposals: the agent opens
pull requests, it does not merge them.

##### Locating Tests

Reliable automation requires knowing which tests are `Informing()`,
which are eligible for promotion, and — importantly — where their source
lives so a PR can be opened against the right repository. Today the CI
signal knows a test's name and state but not necessarily its source
location. This enhancement depends on (and TRT is expected to provide) a
more robust test database that maps each test to its owning repository
and source, giving the agent a central index to scan. Until that index
exists, the agent can operate on the subset of tests whose source
location is already resolvable via `ci-test-mapping` / the OTE
`source` component.

##### Pass-Rate Calculation

To make promotion decisions repeatable, the pass rate is computed as:

- **Measurement window**: a rolling trailing window (proposed: 14 days,
  aligned to the release signal — to be finalized with TRT).
- **Minimum sample count**: a minimum number of qualifying runs in the
  window (proposed: 20) below which the agent takes no action and waits.
- **Run eligibility**: only completed runs count. Aborted or
  infrastructure-failed runs are excluded. Retries are counted per the
  standard CI convention (a test that passes on retry counts as the
  run's outcome for that job); flake attribution follows the existing
  Sippy/Component Readiness definitions rather than a new one.

The pass rate governs promotion only. It is not used to demote a test:
once blocking, a degradation is handled as a regression to be fixed, not
as a trigger to relax the test's lifecycle state. The specific numeric
parameters above are proposals and are called out in
[Open Questions](#open-questions-optional) for confirmation with TRT.

#### Test Requirements Checklist

Before assigning any test to a suite, ensure the following:

- Has a `[Jira:Component]` tag for component ownership. This replaces
  the older `[sig-XYZ]`-based grouping for downstream, OpenShift-authored
  tests: new downstream tests should carry a Jira component rather than
  relying on a `[sig-XYZ]` tag for ownership. Note that the `[sig-XYZ]`
  form still exists on upstream (Kubernetes) tests, which we do not
  intend to re-tag; the deprecation applies to how we categorize our own
  downstream tests, not to the upstream tests we vendor.
- Includes appropriate `[FeatureGate:XYZ]` or `[Feature:XYZ]`
  annotations.
- For inclusion in standard parallel or serial suites, test duration
  is under 5 minutes (longer requires architect approval).
- Parallel tests are non-disruptive; serial tests successfully restore
  cluster state.
- Results are deterministic with a stable test name (no dynamic
  content).

#### Upstream and Externally-Sourced Tests

Not all tests originate in repositories we control; the largest example
is the in-tree Kubernetes e2e tests carried by
[openshift/kubernetes](https://github.com/openshift/kubernetes) via the
`k8s-tests-ext` (hyperkube) extension binary. As a rough sense of scale,
a sampled `openshift/conformance/parallel` run selected several thousand
tests, the majority of which were attributed (via the OTE `source-binary`
property) to `k8s-tests-ext` — upstream Kubernetes is by far the single
largest source. The remainder came from `openshift-tests` (origin's own
tests) and a set of OpenShift-authored component `*-tests-ext` binaries
(e.g. `ovn-kubernetes-tests-ext`, `oc-tests-ext`). Exact counts vary by
release and platform, so these are illustrative rather than fixed.

Of the upstream tests, only those carrying the upstream `[Conformance]`
label are placed in the `.../minimal` suites (on the order of a few
hundred in parallel-minimal and a couple dozen in serial-minimal); the
remaining, non-`[Conformance]` upstream tests land in the broader
`openshift/conformance/parallel` (or `openshift/conformance/serial`)
suites rather than `.../minimal`.

The suite mapping currently applied to upstream tests is, effectively:

| Test attributes | Suite |
|-----------------|-------|
| `[Serial]` + `[Conformance]` | `openshift/conformance/serial/minimal` |
| `[Serial]` (non-Conformance) | `openshift/conformance/serial` |
| `[Conformance]` (parallel) | `openshift/conformance/parallel/minimal` |
| parallel, non-Conformance | `openshift/conformance/parallel` |

Plan for these under the new hierarchy:

- **Upstream tests are added to `active`/`stable` in addition to
  `minimal`**, not instead of it. Minimal membership (selected by the
  `[Conformance]` label) is an additional, faster smoke layer; the same
  test also participates in a lifecycle shard so it is covered by the
  `active`/`stable` signal. A test is therefore a member of both
  `minimal` and one of the `active`/`stable` shards at the same time.
- **Labelling is done centrally, not inline.** Because we do not want to
  edit vendored upstream test source, the shard/minimal labels are
  assigned by a dedicated function that walks the imported specs and
  applies labels from a rules table (see the example in
  [Mandatory Labelling](#mandatory-labelling)). This mirrors how
  openshift/kubernetes already classifies these tests today — on `master`
  via the `k8s-tests-ext` OTE extension binary's centralized label and
  qualifier tables, and on release branches via the legacy
  `annotate.go`/`rules.go` mechanism.
- **We retain the ability to categorize.** Because everything flows
  through OTE labels and CEL qualifiers, upstream-sourced tests are ours
  to place in the hierarchy regardless of where the source lives.

**Are there other upstream repos we ingest and cannot control?** No —
in-tree Kubernetes e2e is effectively the only large, genuinely-upstream
test source. `openshift-tests` no longer compiles component tests in;
under the OTE model it extracts and runs a hardcoded set of `*-tests-ext`
binaries from the release payload (the registry lives in
`openshift/origin`'s `pkg/test/extensions/provider.go`). Of the ~35
entries in that registry, exactly one is an upstream wrapper — the
`hyperkube` binary built from `openshift/kubernetes`
(`openshift-hack/cmd/k8s-tests-ext/`), which wraps `k8s.io/kubernetes/test/e2e/...`
(this import set already includes storage/CSI/`external`, so upstream CSI
tests are part of the same k8s source, not a separate one). Every other
entry is an OpenShift-owned `*-tests-ext` binary: origin's own tests plus
component binaries such as ovn-kubernetes, the machine-api / CAPI family,
OLM v0/v1, the etcd operator, and so on. Notably, even components that
wrap or fork upstream projects contribute **OpenShift-authored** tests
(for example, `openshift/ovn-kubernetes` ships `[ovn-kubernetes-ote]`
tests that live in its downstream `openshift/test/` tree, not vendored
ovn-org e2e). Because each extension declares a component identity via
`NewExtension(product, type, component)` — e.g.
`("openshift","payload","hyperkube")` versus
`("openshift","payload","ovn-kubernetes")` — upstream-derived tests are
distinguishable per Test ID, so OTE lets us classify them precisely. The
`source-binary` attribution seen in a sampled conformance run bears this
out: the only large upstream source is `k8s-tests-ext`; every other
`*-tests-ext` binary is OpenShift-authored.

The practical consequence for this proposal: the only test set we must
label centrally (rather than owning inline) is the in-tree Kubernetes
set via the `hyperkube` extension; everything else is
OpenShift-authored and can be labelled at the source. (Caveat: this
reflects the current registry and two representative binaries verified
by source; it was not feasible to audit every one of the ~35 binaries,
so a component silently re-exporting an upstream suite cannot be
100% ruled out.)

#### Suites in Test Names

`openshift/origin` historically encodes suite membership in the test
name (e.g., bracketed `[Suite:...]` tags). Under this proposal, suite
membership is expressed through OTE labels and CEL qualifiers, not the
test name. The `[Suite:...]` name tags are deprecated on the same
schedule as the other legacy string tags (see
[Removing a deprecated feature](#removing-a-deprecated-feature)); the
`[sig-XYZ]` and `[Jira:Component]` tags remain in the name because they
convey ownership/categorization rather than suite membership.

#### Job Plan and Cost

This section addresses what happens to the jobs that exist today and
where the new suites run.

**Which suites run where (intended model):**

- `conformance/*/minimal` runs on presubmits (PRs) and on payloads —
  the fast, high-frequency signal.
- `active` regressions are caught primarily through Component Readiness;
  `active` may additionally run on payloads (this is the more flexible
  case and is an open question).
- `stable` runs least frequently, with regressions caught through
  Component Readiness.

**Existing jobs (e.g., `aws-ovn`, `aws-ovn-serial`, `aws-ovn-upgrade`):**
a single job cannot run both the `active` and `stable` suites — they run
at different frequencies and serve different signals — so we do not try
to map an existing job onto both. Instead:

- The existing jobs are **kept as they are** (same names, same
  configuration) during migration, preserving their current signal and
  avoiding a mass rename.
- For each existing job we add **two new jobs**: one that runs the
  corresponding `active` suite and one that runs the corresponding
  `stable` suite, each at its own cadence (`active` more frequent,
  `stable` least frequent). For example, alongside `aws-ovn` we add an
  `active` variant and a `stable` variant; likewise for `aws-ovn-serial`
  (against the serial shards) and for the upgrade jobs.
- `minimal` continues to be used on its own for the fast
  presubmit/payload smoke jobs.

This keeps each job scoped to a single lifecycle suite while leaving the
existing jobs untouched. The exact naming of the new `active`/`stable`
variants is a follow-up detail; the preference remains to avoid renaming
any existing job (see [Open Questions](#open-questions-optional)).

**Cost:** this proposal is not expected to increase cost and may reduce
it, but the effect is uncertain. The most frequently run jobs will be
the minimal suites, which are shorter and faster; `active` runs less
often and `stable` less often still. The savings from smaller frequent
suites may be partly offset by the fixed per-job overhead of
provisioning a cluster (~40 minutes), especially if the reorganization
increases the number of distinct jobs. A quantitative before/after
comparison of total cluster-hours should be produced during validation
(see [Test Plan](#test-plan)).

### Risks and Mitigations

**Risk**: Component teams may resist shrinking conformance and
relocating tests to active/stable suites.
**Mitigation**: The migration can be phased, starting with new tests
using the new hierarchy while existing conformance tests are migrated
gradually. Clear documentation and tooling will ease the transition.

**Risk**: Explicit sharding may become unbalanced over time as tests
are added or removed, and would require ongoing manual maintenance.
**Mitigation**: The lifecycle agent handles the shard-rebalancing (and
promotion/graduation) pull requests automatically using Sippy data, so
no human authors these changes. QSE engineers only review the agent's
PRs.

**Risk**: Agent-authored PRs could stall waiting for team review,
recreating the very backlog the agent is meant to eliminate.
**Mitigation**: Adopt a response-window policy for these mechanical PRs.
Rebalancing and promotion PRs do not require deep team involvement;
teams have a defined number of days to respond, after which an architect
or staff engineer may approve and merge. This keeps the pipeline moving
while preserving a human approval step.

**Risk**: Spot-check suites with low run frequency may go stale
without detection.
**Mitigation**: Spot-check suite health is monitored by Component
Readiness rather than by the lifecycle agent; Component Readiness
surfaces suites that stop reporting results.

### Drawbacks

- Explicit sharding requires ongoing rebalancing. This is mitigated by
  the lifecycle agent (which authors the rebalancing PRs), but the PRs
  still must be approved, so some human review load remains.
- The migration from the flat suite model to the hierarchical model
  will require coordination across many component teams.
- Additional suite definitions increase the complexity of CI job
  configuration in `openshift/release`.

## Open Questions [optional]

Shard balancing and lifecycle transitions are **not** open questions:
this enhancement commits to the lifecycle agent driving them from the
start (see [The Lifecycle Agent](#the-lifecycle-agent)), rather than
deferring automation to a later phase.

Remaining open questions:

1. **Downstream minimal-suite membership rule.** For upstream Kubernetes
   tests, `minimal` is selected mechanically by the `[Conformance]`
   label. For downstream, OpenShift-authored tests there is no
   equivalent established signal, so which tests should be treated as
   conformance-minimal is not yet objectively defined and likely
   requires manual, per-component evaluation to start. Defining a
   repeatable rule is an open question (see
   [Minimal-Suite Membership](#minimal-suite-membership)).
2. Confirm the pass-rate parameters with TRT: the measurement window
   (proposed 14 days), the minimum sample count (proposed 20), and the
   exact retry/flake attribution the agent should adopt from Sippy /
   Component Readiness.
3. Naming for the new `active` and `stable` job variants added
   alongside each existing job (e.g. next to `aws-ovn`). Existing jobs
   are kept as-is (no mass rename); only the naming convention for the
   new variants needs to be agreed.
4. Should `active` run on payloads in addition to Component Readiness,
   or only through Component Readiness?

## Test Plan

This enhancement is about test infrastructure organization rather
than product functionality. Validation will consist of:

- Verifying that the OTE APIs correctly compose the new suite
  hierarchy.
- Confirming that suite membership via CEL qualifiers produces the
  expected test lists.
- Validating that the `Informing()` lifecycle correctly gates test
  blocking status.
- Running the new suite hierarchy in parallel with the existing suites
  (non-gating jobs added alongside today's jobs) to compare coverage and
  runtime before cutting over.
- Dry-running the lifecycle agent against historical Sippy data to
  confirm its promotion and rebalancing decisions match expectations
  before it is allowed to open real pull requests.

The parallel validation jobs and the Sippy queries they depend on are
**required** prerequisites, not optional. They are enumerated with owners
and availability gates in
[Infrastructure Needed](#infrastructure-needed-optional); the graduation
criteria below must not be evaluated until those dependencies are
available.

## Graduation Criteria

### Dev Preview -> Tech Preview

- New suite hierarchy defined and available for opt-in use by
  component teams.
- At least one component team has migrated tests to the new hierarchy.
- Required parallel validation jobs (see
  [Infrastructure Needed](#infrastructure-needed-optional)) are stood
  up and validate the new suites produce equivalent coverage.
- Lifecycle agent runs in dry-run mode (reporting decisions without
  opening PRs) so its behavior can be reviewed.

### Tech Preview -> GA

- All component teams have migrated to the new suite hierarchy.
- The downstream minimal-suite membership rule (see
  [Open Questions](#open-questions-optional)) is defined and applied, so
  the `conformance/*/minimal` suites hold only agreed smoke tests.
- The lifecycle agent is live and opening promotion, graduation, and
  rebalancing PRs; the shard-balancing workflow is exercised through it
  rather than by hand.
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

The items below are **required** dependencies for the graduation
criteria in this enhancement; the "[optional]" in the section title is
the template default and does not apply here. Each is listed with an
owner and an availability gate.

| Dependency | Purpose | Owner | Availability gate |
|------------|---------|-------|-------------------|
| Parallel validation jobs in `openshift/release` | Run the new hierarchy alongside existing suites (non-gating) to compare coverage and runtime | TBD (QSE + Test Platform) | Must exist and be green before Dev Preview → Tech Preview |
| CI labelling-enforcement check | Fail any test that lacks its required suite/shard label, keeping `Parents` and labels consistent | TBD (QSE + Test Platform) | Required before Tech Preview → GA |
| Sippy queries for pass-rate and shard-runtime analysis | Feed the lifecycle agent's promotion and rebalancing decisions | TBD (TRT) | Required before the agent leaves dry-run |
| Test-source index / robust test database | Map each test to its owning repo and source so the agent can open PRs in the right place | TBD (TRT) | Required for full agent coverage; agent operates on the resolvable subset until then |
| Lifecycle agent prow job | Scheduled automation that opens lifecycle PRs | TBD (QSE) | Required before Tech Preview → GA |

If any required dependency cannot be provided, the corresponding
acceptance criterion (parallel-validation coverage, agent-driven
graduation) must be revised rather than silently skipped.
