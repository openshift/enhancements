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

Today, OpenShift test suites use a flat model with static string-label
assignment. This enhancement proposes migrating to a hierarchical suite
composition built on the OpenShift Tests Extension (OTE) APIs and driven
by key/value **test metadata tags** rather than opaque string labels.
A single ordered `Lifecycle` axis (`Draft → Informing → Blocking →
Stable`), together with a `Criticality` overlay, classifies every test
and determines which suite it lands in. The new model introduces a
structured test lifecycle,
explicit sharding for scheduling flexibility, and a minimal
conformance suite containing only core smoke tests. Lifecycle
transitions and shard balancing are driven by a scheduled automation
agent rather than manual toil, so tests do not languish in the
`Informing` state indefinitely. The goal is to improve test
manageability, enable predictable test graduation, and reduce
conformance suite footprint while increasing overall coverage through
better suite organization.

## Motivation

The current flat suite model in `openshift-tests` has grown organically
and presents several challenges: conformance suites are too large,
there is no formal lifecycle for test graduation, sharding is
implicit, and test assignment relies on fragile string-label pattern
matching. These problems slow down CI, make it difficult for component
teams to introduce and stabilize new tests, and create unpredictable
runtime behavior in constrained environments like vSphere.

The OTE framework (see [openshift-tests-extension](openshift-tests-extension.md))
provides the foundational APIs (`AddSuite`, `AddTag`, `AddLabel`,
`Informing()`) needed to build a richer suite hierarchy. In particular,
OTE supports key/value **tags** (`AddTag({"key":"value"})`, queried in
CEL as `test.tags.<key>=="<value>"`) in addition to string **labels**
(a set of text markers, queried as `test.labels.has("...")`). This
enhancement uses those tags as the structured test-metadata vocabulary
and defines how the OTE APIs should be used to organize suites, manage
test lifecycle, and balance shards.

A recurring problem with the manual model is that lifecycle transitions
require a human to notice that a test is eligible and then open a pull
request. In practice this rarely happens: `Informing` tests are left in
place indefinitely, in both good states (long since stable, never
promoted) and bad states (chronically failing, never removed). This
enhancement therefore makes an automation agent — not a human — the
primary driver of promotion, graduation, and shard balancing, closing
that gap from the start.

### User Stories

* As a member of the quality staff engineer team, I want a minimal
  conformance suite that contains only core smoke tests so that
  conformance runs are fast and focused on the most critical
  functionality.
* As an OpenShift product development engineer, I want a clear
  lifecycle for my tests (Draft → Informing → Blocking → Stable) so
  that I can stabilize new tests without risking release signal.
* As an OpenShift product development engineer, I want to classify my
  tests with structured key/value metadata (`Lifecycle`, `Criticality`)
  rather than opaque string labels so that suite membership is explicit
  and queryable.
* As a CI infrastructure engineer, I want explicitly sharded suites
  so that I can schedule jobs predictably in resource-constrained
  environments.
* As a component team lead, I want spot-check suites for features
  requiring uncommon cluster configurations so that specialized
  tests do not pollute the main conformance signal.
* As a member of the quality staff engineer team, I want shard runtimes
  balanced within 10% of mean so that CI pipelines complete in
  predictable and roughly equal time windows.
* As a member of the quality staff engineer team, I want an automation agent
  to detect promotion-eligible tests and imbalanced shards and open the
  corresponding pull requests so that lifecycle maintenance happens
  continuously without manual toil.

### Goals

1. Introduce a structured, key/value test-metadata vocabulary
   (a single ordered `Lifecycle` axis plus a `Criticality` overlay)
   expressed via OTE tags, and drive all suite membership from it.
2. Shrink `openshift/conformance/parallel/minimal` and
   `openshift/conformance/serial/minimal` to contain only core smoke
   tests (`Criticality: Core`).
3. Introduce `active` suites (`Lifecycle: Informing`/`Blocking`) for new
   and stabilizing tests with a clear graduation path to `stable` suites
   (`Lifecycle: Stable`).
4. Provide explicit sharding
   (`openshift/conformance/parallel/stable-01`,
   `openshift/conformance/parallel/stable-02`, etc.) instead of
   auto-sharding, enabling scheduling flexibility in constrained
   environments.
5. Define a test lifecycle flow (`Lifecycle: Draft`, `Informing`,
   `Blocking`, `Stable`) with clear pass-rate thresholds and duration
   expectations for each stage.
6. Introduce `spot-check/<feature>` suites for features requiring
   uncommon cluster configurations.
7. Establish a shard-balancing workflow using Sippy data to keep all
   shards within 10% of mean runtime.
8. Provide an automation agent (a scheduled prow job) that drives
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
2. The test author sets its `Lifecycle` — but only to `Draft` (not yet
   ready to run in any suite) or `Informing` (ready to run non-blocking).
   `Blocking` and `Stable` are not author-settable; they are reached only
   through measured promotion by the lifecycle agent, so a new test cannot
   skip stabilization by declaring itself blocking. Inline, `Informing`
   is declared with `ote.Tag("Lifecycle", "Informing")` and `Draft` with
   `ote.Tag("Lifecycle", "Draft")` (there is no `Draft()`
   helper — see [OTE API Extension: Inline Tags](#ote-api-extension-inline-tags)
   for why the helper set is left as-is). A `Draft` test therefore carries
   that one tag and no native lifecycle decorator. If a test carries no
   `Lifecycle` tag at all, the centralized normalization step writes
   `Lifecycle: Informing` (a normalization-time default written into source,
   **not** a CEL fallback — the suite qualifiers
   guard every `Lifecycle` read with `has(...)`, so an un-normalized test
   with no `Lifecycle` tag matches no suite rather than silently entering
   `active`; see [Test Metadata](#test-metadata)). This stage default also
   keeps such a test *non-fatal*: when normalization stamps the `Informing`
   stage, the same centralized path materializes `ote.Informing()` in the
   test source, overriding OTE's own native default of
   `blocking` (which would otherwise make an untagged test gate). Because
   omission normalizes to `Informing`, an author who wants `Draft` must
   say so explicitly with `ote.Tag("Lifecycle", "Draft")`. An
   `Informing`/`Blocking` test is selected into the `active` suites.
3. When the test first becomes non-`Draft` (i.e. it is `Informing`,
   whether set explicitly or by default), it is assigned an initial
   `Shard` before it enters CI, because the `active` qualifiers require a
   `Shard` value and mandatory-metadata validation rejects a *running*
   test without one (see [Mandatory Metadata](#mandatory-metadata)).
   **The author never sets `Shard`.** It is pure scheduling metadata owned
   entirely by automation: the metadata-normalization step assigns a
   deterministic initial `Shard` (from the registered `(active, mode)`
   shard set) to any non-`Draft` test that has no `Shard`, so the test is
   never dropped from every suite, and the balancing workflow rebalances
   it thereafter. A test still in `Draft` needs no `Shard`; it acquires
   one only as it leaves `Draft`. A `Shard` value written by hand is
   treated as a normalization/balancing concern, not an authoring
   decision.
4. Because the stage tag and the paired `Informing()` annotation must stay
   consistent, the centralized tagging path (the lifecycle agent) writes
   both into the test source together: when a test resolves to
   `Lifecycle: Informing` (explicitly or by default) the agent materializes
   the `Informing()` annotation alongside the tag, and removes it again on
   promotion to `Blocking`/`Stable`. The CI self-check that enforces
   mandatory metadata does not write; it *validates* agreement and fails any
   test where the tag and annotation disagree.
5. `Lifecycle: Informing` makes failures non-blocking during
   stabilization; this is expressed via the OTE `Informing()` annotation
   as well as the tag, which stay in sync.
6. The test is picked up in CI through the OTE extension binary
   discovery mechanism and runs in the `active` suite for at least the
   minimum stabilization window (2–3 sprints) before it is eligible for
   promotion. A `Lifecycle: Draft` test is excluded from every suite and
   does not run until its owner promotes it out of `Draft`.

#### Promoting a Test to Blocking

Promotion is performed by the lifecycle agent, not by hand.

1. On its schedule, the agent locates each `Lifecycle: Informing` test
   in the `active` suites (see [Locating Tests](#locating-tests)) and
   computes its pass rate over the
   [measurement window](#pass-rate-calculation).
2. If the test has been in `active` for at least the minimum
   stabilization window, has at least the minimum sample count of
   qualifying runs, and meets the `>= 99%` pass rate, the agent opens a
   pull request flipping the tag to `Lifecycle: Blocking` (and removing
   the corresponding `Informing()` annotation), making the test blocking
   within the `active` suite. The test stays in `active` (both
   `Informing` and `Blocking` tests are selected there).
3. A QSE engineer (or, after the response window, an architect or staff
   engineer) approves the pull request.
4. The test remains `Blocking` in `active` until GA + 1 release.

Promotion is one-way. Once a test is `Lifecycle: Blocking`, it is never
demoted back to `Informing` and it is never moved to a lower-signal
suite. If its pass rate later degrades, that is treated as a
**regression to be fixed**, not as grounds for quarantine or downgrade.
The agent surfaces the degradation (and the owning component is expected
to fix it) rather than relaxing the test's lifecycle state. See
[Pass-Rate Calculation](#pass-rate-calculation).

#### Graduating a Test to Stable

Graduation is also performed by the lifecycle agent.

1. After GA + 1 release, the agent selects the target stable shard
   suite based on current shard runtimes: a parallel test maps to an
   explicit `openshift/conformance/parallel/stable-NN` suite (e.g.
   `openshift/conformance/parallel/stable-01`) and a serial test maps to
   an explicit `openshift/conformance/serial/stable-NN` suite (e.g.
   `openshift/conformance/serial/stable-01`). The generic `stable-NN`
   form is never written into a suite definition; a concrete shard is
   always chosen. Because `active` and `stable` are **separate shard
   registries** (see [Shard Registry](#shard-registry)), the test's
   current `active` shard number does not carry over — the agent picks a
   `stable` shard from the registered `(stable, mode)` set, creating and
   registering a new stable shard suite + CI job first if none can absorb
   the test (register-then-assign, never the reverse). The `Shard` value
   is therefore (re)assigned to a *stable* shard during graduation.
2. The agent opens a single pull request that makes two coordinated tag
   edits together — flipping `Lifecycle` from `Blocking` to `Stable` and
   setting `Shard` to the chosen stable shard — so the test lands in a
   valid, registered `stable` shard in one atomic change. `Lifecycle` is
   single-valued, so at the instant the tag flips the test leaves the
   `active` selection and joins the `stable` selection; there is no window
   in which it belongs to both, and it is never left carrying a `Stable`
   lifecycle with an `active`-only shard. This consistency requirement is
   why every test must carry the required metadata tags (enforced in CI;
   see [Mandatory Metadata](#mandatory-metadata)) — an untagged test
   cannot be reliably moved between suites. See
   [Exclusive Membership and Rollout](#exclusive-membership-and-rollout).
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
APIs, in particular the key/value tag mechanism (`AddTag`, queried via
`test.tags.<key>`) as the structured metadata vocabulary.

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

#### Test Metadata

Suite membership is driven by structured key/value **metadata tags**
rather than opaque string labels. Tags are a first-class OTE feature:
they are set with `AddTag({"key":"value"})` and queried in CEL suite
qualifiers as `test.tags.<key>=="<value>"` (see
[openshift-tests-extension](openshift-tests-extension.md)). This
enhancement defines a single ordered lifecycle axis plus one overlay:

| Key | Valid values | Meaning |
|-----|--------------|---------|
| `Lifecycle` | `Draft`, `Informing`, `Blocking`, `Stable` | The one ordered axis describing where a test sits from creation to permanent graduation. `Draft` → in no suite; `Informing` → runs non-blocking in `active`; `Blocking` → runs and gates in `active`; `Stable` → graduated, gates permanently in `stable`. |
| `Criticality` | `Core` | The test is a core smoke test and additionally belongs to the `conformance/*/minimal` suites. |
| `Shard` | Zero-padded two-digit string, `01`–`NN` | Which shard the test runs in, scoped *within* a `(suite class, mode)` pair (suite class = `active` or `stable`) — parallel `stable`/`01` and serial `stable`/`01` are different suites. Mandatory for non-`Draft` tests; exactly one value, owned by automation and reassigned by the balancing workflow (see [Mandatory Metadata](#mandatory-metadata)). |

`Lifecycle` is the **single dimension** that drives lifecycle-suite
membership. There is deliberately no separate reliability or maturity
axis: a test's pass rate is a *measurement* (the input to the
`Informing → Blocking` transition), not a stored classification, and its
maturity is captured by the ordered progression itself. Suite membership
is therefore a pure function of `Lifecycle` (plus the `Shard` and
`Criticality` overlays).

**Execution mode (parallel vs serial) is not a metadata tag.** It is
derived from the existing `[Serial]` marker in the test name, consistent
with how `openshift-tests` already separates serial from parallel work.
Every suite qualifier combines a `[Serial]` predicate with its
member-selection predicate — serial suites require
`test.name.contains("[Serial]")` and parallel suites require
`!test.name.contains("[Serial]")`. What that member-selection predicate
is differs by suite: **lifecycle shard suites** (`active`/`stable`) select
on the `Lifecycle` value *and* the `Shard` tag, whereas the **minimal
suites** select on `Criticality: Core` and a non-`Draft` guard only —
they deliberately do *not* filter on a specific `Lifecycle` value or
`Shard`, so a `Core` test stays in minimal across every non-`Draft`
stage and every shard. Either way the mode predicate means a single test
cannot match both a parallel and a serial suite. No separate `Mode` tag
is introduced.

Rules governing the metadata:

- **`Lifecycle` is a single, ordered, one-way progression:**
  `Draft → Informing → Blocking → Stable`. A test carries exactly one
  value, so its lifecycle-suite membership is unambiguous and mutually
  exclusive.
  - `Draft` — the test is in **no** suite (not `active`, `stable`, or
    `minimal`) and does not run. It is a work-in-progress the owner has
    not yet released into CI.
  - `Informing` — runs in the `active` suites, failures non-blocking.
  - `Blocking` — runs in the `active` suites and gates.
  - `Stable` — graduated (after GA + 1); runs in the `stable` suites and
    gates permanently.
- **Suite membership derives from `Lifecycle`:** `active` selects
  `Informing` or `Blocking`; `stable` selects `Stable`. Suite qualifiers
  exclude `Draft` implicitly (it matches neither selection).
- **Default `Lifecycle` is `Informing`.** If a test carries no explicit
  `Lifecycle` tag, it is treated as `Lifecycle: Informing`. Tests are
  never automatically moved *out* of `Draft` — a test owner does that by
  hand when the test is ready.
- **Authors set the lifecycle entry point; the agent owns forward
  promotion.** A test author may set `Lifecycle` only to `Draft` (not yet
  ready) or `Informing` (ready to run, non-blocking), and owns the one
  transition the agent never performs: `Draft → Informing`. Authoring a
  test directly as `Blocking` or `Stable` is **not** allowed — those
  values are reached only through measured promotion by the lifecycle
  agent (`Informing → Blocking → Stable`), or seeded from historical
  pass-rate data by the bulk importer under the *same* rule (see
  [`lifecycleFor`](#mandatory-metadata)). This keeps promotion one-way and
  evidence-driven while still letting authors declare intent inline; there
  is no conflict, because authors and the agent never write the same edge.
  The metadata-enforcement check rejects a newly authored test whose
  `Lifecycle` is `Blocking` or `Stable`.
- **`active` holds `Informing` and `Blocking`; `stable` holds only
  `Stable`.** Because graduation only happens after a test has become
  `Blocking` and passed GA + 1, `stable` never contains `Informing`
  tests.
- **`Criticality: Core` is additive.** It marks a test as a core smoke
  test so it *also* appears in `conformance/*/minimal`. It does not
  replace the `Lifecycle` membership: a `Core` test still lives in an
  `active` or `stable` shard according to its `Lifecycle` (see
  [Minimal-Suite Membership](#minimal-suite-membership)).

The `Lifecycle` tag and the OTE `Informing()` annotation express the
same fact for the non-blocking case and are kept in sync: `Informing()`
present ⇔ `Lifecycle: Informing`; `Informing()` absent ⇔
`Lifecycle: Draft`, `Blocking`, or `Stable`. (A `Draft` test does not
run and must not carry `Informing()`; it simply has no annotation, like a
`Blocking` or `Stable` test.)

#### Suite Hierarchy

The new hierarchy is built on top of the OTE APIs, with each suite
selecting its members by metadata tag:

| Suite | Selects | Description |
|-------|---------|-------------|
| `openshift/conformance/parallel` | (existing, unchanged) | The existing aggregate parallel conformance suite. Left exactly as-is during bring-up; later becomes the graft target the matured new suites join via `Parents` (see [Exclusive Membership and Rollout](#exclusive-membership-and-rollout)) |
| `openshift/conformance/serial` | (existing, unchanged) | The existing aggregate serial conformance suite; same role |
| `openshift/conformance/parallel/minimal` | `Criticality: Core` | Core smoke tests only, parallel execution. Existing suite, defined in openshift/kubernetes; its selection is changed **in-place** from the upstream `[Conformance]` label to `Criticality: Core` (see naming note below) |
| `openshift/conformance/serial/minimal` | `Criticality: Core` | Core smoke tests only, serial execution; same in-place change |
| `openshift/conformance/parallel/active-01` | `Lifecycle: Informing`/`Blocking` | Recent features, parallel, shard 1 |
| `openshift/conformance/serial/active-01` | `Lifecycle: Informing`/`Blocking` | Recent features, serial, shard 1 |
| `openshift/conformance/parallel/stable-01` | `Lifecycle: Stable` | Stable tests, parallel, shard 1 |
| `openshift/conformance/parallel/stable-02` | `Lifecycle: Stable` | Stable tests, parallel, shard 2 |
| `openshift/conformance/serial/stable-01` | `Lifecycle: Stable` | Stable tests, serial, shard 1 |
| `openshift/spot-check/<feature>` | (own qualifier) | Uncommon cluster configs |

Every suite name keeps the `conformance` prefix so the naming stays
consistent with the existing conformance suites. Each shard suite's
qualifier combines the `Lifecycle` selection with the shard tag (see
[Shard Membership Qualifiers](#shard-membership-qualifiers)).

Two situations are handled differently, depending on whether the suite
already has a definition that can be edited in place.

**The `minimal` suites are edited in place.** `openshift/conformance/
parallel/minimal` and `.../serial/minimal` are defined in
openshift/kubernetes, so there is no second registration
and no name collision to worry about. The single authoritative
definition is changed directly: its selection moves from the upstream `[Conformance]`
label to `Criticality: Core`. Upstream `[Conformance]` tests continue to
qualify because the centralized import walk stamps `Criticality: Core`
onto every `[Conformance]` test (see
[Minimal-Suite Membership](#minimal-suite-membership) and
[Upstream and Externally-Sourced Tests](#upstream-and-externally-sourced-tests)),
and downstream tests qualify by carrying `Criticality: Core` directly.
Because this edits the one definition rather than registering a same-named
suite a second time, OTE's additive-union behavior does not come into play
for `minimal`.

**The new `active`/`stable` shard suites are brought up under new names.**
These have no existing equivalent, and they must not reuse a suite name
that already exists, because OTE combines all registrations of the same
suite name additively — it unions their qualifiers rather than replacing
them (see [openshift-tests-extension](openshift-tests-extension.md)). The
`active-NN`/`stable-NN` names are new, so they run in parallel with the
existing `openshift/conformance/parallel` / `.../serial` suites — whose
definitions are left exactly as they are — without interfering. Once the
new suites and their jobs mature, they are grafted into the existing
conformance suite via `Parents` (see below); no existing definition is
rewritten at any point.

`Parents` **is** the right tool for one specific, later step: once the new
suites and their jobs have matured, each shard suite can declare
`Parents: ["openshift/conformance/parallel"]` (respectively `.../serial`)
to graft its tests into the *existing* conformance suite — **without
touching the existing suite's definition at all.** See
[Exclusive Membership and Rollout](#exclusive-membership-and-rollout) for
the exact scope, and [Graduation Criteria](#graduation-criteria) for when
this happens.

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
  mechanically by the upstream `[Conformance]` label. A dedicated
  function walks the imported specs and adds `Criticality: Core` to
  every test carrying `[Conformance]`; the `conformance/*/minimal`
  suites then select on `Criticality: Core`. The mapping is applied
  centrally rather than judged test-by-test (see
  [Upstream and Externally-Sourced Tests](#upstream-and-externally-sourced-tests)).
- **Downstream (OpenShift-authored) tests**: there is no equivalent
  established signal. Deciding which downstream tests deserve
  `Criticality: Core` still requires evaluation, likely a manual review
  per component to begin with. Defining an objective, repeatable rule
  for this is tracked as an open question.

Note that a test is not confined to a single membership: a
`Criticality: Core` test also carries a `Lifecycle` value and so
participates in an `active` or `stable` shard. Minimal is an additional,
faster smoke selection layered on top of the lifecycle shard, not a
mutually exclusive alternative to it (see
[Upstream and Externally-Sourced Tests](#upstream-and-externally-sourced-tests)
for how this is expressed).

#### Test Lifecycle Flow

| Stage (`Lifecycle`) | Suite | Pass Rate Threshold | Duration |
|---------------------|-------|---------------------|----------|
| `Draft` | none (does not run) | N/A | Until owner releases it |
| `Informing` | `active` | Failures non-blocking | >= 2–3 sprints |
| `Blocking` | `active` | >= 99% over measurement window | Until GA + 1 release |
| `Stable` | `.../parallel/stable-01`, `.../parallel/stable-02`, `.../serial/stable-01`, … (explicit shard) | Governed by Component Readiness (~95%) | Permanent |
| Specialized (own qualifier) | `spot-check/<feature>` | N/A (own job) | Own cadence |

The four `Lifecycle` values are the single ordered lifecycle axis; each
row above is one value of that axis, not a separate dimension. `active`
selects `Informing`/`Blocking`; `stable` selects `Stable`.

The `Draft` stage is pre-CI: the test exists in source but is excluded
from every suite until its owner manually moves it to `Informing`. The
agent never advances a test out of `Draft`.

The `Stable` stage intentionally has no hard suite-level threshold:
once a test is in a `stable` shard, Component Readiness owns its health
signal at its configured tolerance (roughly 95%), rather than a fixed
`>= 99%` gate.

#### Suite Composition with OTE APIs

Tests are classified with metadata tags, and the lifecycle/shard suites
select on those tags via CEL qualifiers.

**Metadata lives with the test.** For OpenShift-authored tests, the
preferred and expected form is to declare the metadata *inline on the
`g.It`*, alongside the test itself, so a reader can see a test's
lifecycle, criticality, and shard directly at its definition without
consulting a separate mapping file. This relies on a small OTE API
extension described in
[OTE API Extension: Inline Tags](#ote-api-extension-inline-tags): an
`ote.Tag(key, value)` decorator plus a build-time step that promotes
these reserved-prefix `tag:key=value` labels into the spec's `Tags` map.
With it, every
metadata key is declared right on the `It`:

```go
// Everything is declared right on the It, so the metadata belongs to
// this specific test. ote.Tag(k, v) is the general form for any
// key/value tag. All of these are promoted into spec.Tags at build
// time (see OTE API Extension: Inline Tags), so the suite qualifiers
// can select on test.tags.<key>.
g.It("should do the thing",
    ote.Tag("Lifecycle", "Informing"),  // -> active, non-blocking
    ote.Tag("Criticality", "Core"),     // also -> conformance/*/minimal
    func() { ... },
)
// Note: Shard is deliberately NOT declared here. It is scheduling
// metadata owned by automation; the normalization step assigns the
// initial value and the balancing workflow reassigns it (see
// Mandatory Metadata). Authors only declare Lifecycle and, optionally,
// Criticality.
```

Because the metadata is attached to the individual test, **promotion is
per-test, not per-suite**: the lifecycle agent changes a single test's
`Lifecycle` when *that test* meets the criteria. For an inline-tagged
test, "changing the `Lifecycle`" is a source edit the agent lands as a
PR against the test's repository — e.g. replacing `ote.Informing()` with
`ote.Tag("Lifecycle", "Blocking")` on the `g.It` — not a mutation of a
separate mapping file; the build-time promotion then carries the new
value into `test.tags.Lifecycle`. For a bulk-imported test the equivalent
edit is to the rules table that drives the centralized `AddTag` walk.
Either way it is one value change for one test. Sibling tests for
the same feature are unaffected, and a newly added test starts at its
own `Lifecycle` (`Informing` by default) regardless of how long its
feature's other tests have been `Blocking` or `Stable`. There is no
notion of a whole feature suite being promoted as a unit.

The bulk, pattern-matching form below (`Select(NameContains(...))`) is
**reserved for externally-sourced test sets that cannot be edited
inline** — chiefly upstream Kubernetes tests imported through
`k8s-tests-ext` (see
[Upstream and Externally-Sourced Tests](#upstream-and-externally-sourced-tests)).
For those, the metadata is applied centrally from a rules table rather
than test-by-test. It is not the mechanism for OpenShift-authored tests,
precisely to avoid duplicating test names in a second file and letting
the two drift out of sync:

```go
// Tag tests with structured metadata. Tags are key=value pairs set via
// AddTag and queried in CEL as test.tags.<key>=="<value>". This bulk
// form is for imported specs we do not own and cannot annotate inline.
specs.Select(et.NameContains("[sig-foo] my test")).AddTag(map[string]string{
    "Lifecycle": "Informing", // non-blocking during stabilization -> active
})

// The active/stable suites are defined once (see Shard Membership
// Qualifiers) and select tests by their single Lifecycle value; a
// component does not move a test between them via Parents. Every
// lifecycle transition is expressed purely by changing that one tag:
//
//   Draft      -> Informing : Lifecycle Draft     -> Informing (owner)
//   Informing  -> Blocking  : Lifecycle Informing -> Blocking  (agent)
//   Blocking   -> Stable    : Lifecycle Blocking  -> Stable    (agent)
//
// Because Lifecycle carries exactly one value, flipping Blocking ->
// Stable atomically moves the test from active to stable; there is no
// window in which it is a member of both.
```

##### Exclusive Membership and Rollout

Lifecycle-suite membership is driven by the single-valued `Lifecycle`
tag (`Informing`/`Blocking` → `active`, `Stable` → `stable`), so a test
is a member of exactly one lifecycle suite at any point in time — the tag
cannot hold two values at once. Graduation is therefore expressed as a
single edit that changes `Lifecycle` from `Blocking` to `Stable`; there
is no window in which the test belongs to both suites. The lifecycle
agent changes exactly one `Lifecycle` value per transition PR. Reviewers
should reject any change that would leave a test carrying more than one
`Lifecycle` value.

OTE's `Parents` field exists so that a suite defined in one place can
advertise itself into another suite it does not own — a composition
mechanism that adds the child's tests to the parent *without editing the
parent's definition*. This enhancement uses `Parents` for **exactly one
purpose**, and forbids it for the others:

- **Used — grafting the matured shard suites into the existing
  conformance suite.** During bring-up the new suites run in parallel
  with the existing `openshift/conformance/parallel` / `.../serial`
  suites and do not touch them. Once the new suites and their jobs have
  proven out (see [Graduation Criteria](#graduation-criteria)), each
  shard suite declares `Parents: ["openshift/conformance/parallel"]`
  (respectively `.../serial`) so its tests flow into the established
  conformance signal. This is `Parents`' intended composition use, and
  its non-invasiveness — the existing suite definition is never edited —
  is exactly why it fits: promotion becomes a single additive change on
  the new side rather than a rewrite of the existing suite.
- **Not used — moving a test between lifecycle suites.** Every lifecycle
  transition is a single edit to the one-valued `Lifecycle` tag. Flipping
  `Blocking → Stable` atomically moves the test from the `active`
  selection to the `stable` selection; there is no window in which it
  belongs to both. Reviewers should reject any change that tries to move
  a test between lifecycle suites by adding a `Parents` advertisement
  instead of changing the tag.

##### Shard Membership Qualifiers

Each shard suite is defined by a qualifier that combines three
predicates: the `Lifecycle` selection, the execution-mode predicate
(`[Serial]` present or absent), and the `Shard` tag the balancing
workflow assigns. During bring-up **no shard suite declares `Parents`**;
`Parents` is reserved for grafting into the existing conformance suite at
maturity (see
[Exclusive Membership and Rollout](#exclusive-membership-and-rollout)):

```go
// Every qualifier that reads an optional tag guards it with has()
// first: in CEL, indexing test.tags with an absent key is an error, not
// false, so an unguarded read would fault the whole qualifier against
// any spec that lacks the key. Lifecycle and Shard are mandatory on
// non-Draft tests, but are guarded anyway so a not-yet-normalized spec
// is simply excluded rather than raising an evaluation error.
//
// Parallel stable shards: Stable, NOT serial, one per shard value.
ext.AddSuite(e.Suite{
    Name: "openshift/conformance/parallel/stable-01",
    Qualifiers: []string{
        `has(test.tags.Lifecycle) && test.tags.Lifecycle=="Stable" && ` +
            `!test.name.contains("[Serial]") && ` +
            `has(test.tags.Shard) && test.tags.Shard=="01"`,
    },
})
ext.AddSuite(e.Suite{
    Name: "openshift/conformance/parallel/stable-02",
    Qualifiers: []string{
        `has(test.tags.Lifecycle) && test.tags.Lifecycle=="Stable" && ` +
            `!test.name.contains("[Serial]") && ` +
            `has(test.tags.Shard) && test.tags.Shard=="02"`,
    },
})
// Serial stable shard: same Lifecycle/Shard space, but [Serial] tests.
// A Stable/01 test therefore lands here or in parallel/stable-01, never
// both, because the mode predicate is mutually exclusive.
ext.AddSuite(e.Suite{
    Name: "openshift/conformance/serial/stable-01",
    Qualifiers: []string{
        `has(test.tags.Lifecycle) && test.tags.Lifecycle=="Stable" && ` +
            `test.name.contains("[Serial]") && ` +
            `has(test.tags.Shard) && test.tags.Shard=="01"`,
    },
})
// The active shards accept both Informing and Blocking tests.
ext.AddSuite(e.Suite{
    Name: "openshift/conformance/parallel/active-01",
    Qualifiers: []string{
        `has(test.tags.Lifecycle) && ` +
            `(test.tags.Lifecycle=="Informing" || ` +
            `test.tags.Lifecycle=="Blocking") && ` +
            `!test.name.contains("[Serial]") && ` +
            `has(test.tags.Shard) && test.tags.Shard=="01"`,
    },
})
```

**At maturity**, the shard suites are grafted into the *existing*
aggregate with `Parents`, leaving that suite's definition untouched:

```go
// Promotion step (later): the matured new stable shards advertise
// themselves into the existing conformance suite. No edit to
// openshift/conformance/parallel's own definition is required.
ext.AddSuite(e.Suite{
    Name:    "openshift/conformance/parallel/stable-01",
    Parents: []string{"openshift/conformance/parallel"}, // graft in
    Qualifiers: []string{
        `has(test.tags.Lifecycle) && test.tags.Lifecycle=="Stable" && ` +
            `!test.name.contains("[Serial]") && ` +
            `has(test.tags.Shard) && test.tags.Shard=="01"`,
    },
})
```

A test carries exactly one `Shard` value and one execution mode, so it
belongs to exactly one shard within one `(suite class, mode)` pair
(suite class = `active` or `stable`).
Moving a test between shards means changing its single `Shard` value,
which keeps shard membership mutually exclusive. `Draft` tests match
neither the `active` nor the `stable` selection, so they are excluded
from every running suite regardless of any `Shard` tag they carry.

The `minimal` suites select on `Criticality: Core`, but must also carry
the execution-mode predicate and an explicit non-`Draft` guard so a
`Draft` core test (which belongs in no suite) cannot leak in:

```go
// These are the existing openshift/kubernetes minimal-suite definitions,
// edited in place: the selection changes from the upstream [Conformance]
// label to Criticality=="Core". Because we edit the one definition rather
// than registering a second same-named suite, OTE's additive union does
// not apply here.
//
// Note the has(...) guards: in CEL, indexing test.tags with an absent
// key is an error, not false. Criticality is present on only a small
// subset of tests, so every qualifier that reads an optional tag guards
// it with has() first. Lifecycle is mandatory on non-Draft tests, but is
// guarded too for uniformity and to stay correct against any not-yet-
// normalized spec.
ext.AddSuite(e.Suite{
    Name: "openshift/conformance/parallel/minimal",
    Qualifiers: []string{
        `has(test.tags.Criticality) && test.tags.Criticality=="Core" && ` +
            `has(test.tags.Lifecycle) && test.tags.Lifecycle!="Draft" && ` +
            `!test.name.contains("[Serial]")`,
    },
})
ext.AddSuite(e.Suite{
    Name: "openshift/conformance/serial/minimal",
    Qualifiers: []string{
        `has(test.tags.Criticality) && test.tags.Criticality=="Core" && ` +
            `has(test.tags.Lifecycle) && test.tags.Lifecycle!="Draft" && ` +
            `test.name.contains("[Serial]")`,
    },
})
```

##### Mandatory Metadata

Every **non-`Draft`** test must carry the metadata tags that determine
its suite and shard membership: a valid `Lifecycle` (the normalization
step writes `Informing` when absent — a normalization-time default, not a
CEL fallback) and a registered `Shard`. This is a hard requirement, not a
convention, for two reasons:

1. Lifecycle transitions and rebalancing change a test's `Lifecycle` and
   `Shard` tags; if a running test is untagged, the automation cannot
   reliably move it.
2. Membership must be deterministic — an untagged running test could
   silently fall out of every suite.

`Draft` is the deliberate exception. A `Draft` test is authored by hand
*before* any automation has run, and authors never set `Shard`, so a
freshly authored `Draft` test legitimately has **no `Shard` yet** — and
that is fine, because `Draft` tests are in no suite and do not run. The
`Shard` requirement therefore attaches at the moment a test leaves
`Draft`: the same metadata-normalization step that runs when an owner
promotes `Draft → Informing` assigns the initial `Shard` (see
[Adding a New Test](#adding-a-new-test) and [`shardFor`](#mandatory-metadata)).
Validation requires a `Shard` only for non-`Draft` tests.

To enforce this, a validation check runs in CI (in the extension binary
self-check and/or as a required presubmit) that **fails** if any
discovered test lacks the required metadata tags, or carries a value
outside the defined vocabulary (e.g. a `Lifecycle` other than `Draft`
/`Informing`/`Blocking`/`Stable`, or — for a non-`Draft` test — a
`Shard` that is not in the registered shard set for the test's
`(suite class, mode)`, see [Shard Registry](#shard-registry)). A `Draft`
test is exempt from the `Shard`-registry check: it is in no suite and
does not run, so any `Shard` it happens to carry is not validated against
the registry. Tests cannot merge without valid metadata. This guarantees
suite membership stays consistent across the lifecycle.

For large, externally-sourced test sets (e.g. upstream Kubernetes),
metadata is applied centrally in a dedicated function that walks the
specs and assigns tags from a rules table, rather than inline on each
test:

```go
// Centralized tagging for a bulk-imported test set, e.g. upstream k8s.
// Runs over all specs and assigns metadata tags from a rules table,
// instead of editing each test inline.
allSpecs.Walk(func(spec *et.ExtensionTestSpec) {
    // Upstream [Conformance] tests are core smoke tests: mark them so
    // they also land in conformance/*/minimal. The upstream marker is a
    // bracketed token in the test *name* ("[Conformance]"), not
    // necessarily a ginkgo Label, so match the name to avoid depending on
    // whether the importer surfaced it as a bracket-stripped label. Use a
    // word-bounded match ("[Conformance]"), never a bare substring, so
    // "[Conformance]" is matched but unrelated names are not.
    if strings.Contains(spec.Name, "[Conformance]") {
        spec.AddTag(map[string]string{"Criticality": "Core"})
    }
    // Every imported test still gets a lifecycle stage and a shard so it
    // participates in active/stable in addition to minimal.
    spec.AddTag(map[string]string{
        "Lifecycle": lifecycleFor(spec), // Informing / Blocking / Stable
        "Shard":     shardFor(spec),     // e.g. 01, always assigned
    })
})
```

Both tagging paths converge on the same destination — the spec's `Tags`
map — and therefore the same `test.tags.<key>` namespace the qualifiers
select on. The inline path (`ote.Tag`) writes reserved `tag:` labels that
the build-time step promotes into `Tags`; this centralized path calls
`AddTag` to write `Tags` directly. `AddTag` takes a map, so a single call
cannot carry a conflicting duplicate key, and it is the one authoritative
writer for imported specs (which are not edited inline), so the two paths
do not both tag the same test. Note `lifecycleFor` may return `Blocking`
here even though authors cannot set `Blocking` inline: the importer is not
an author — it applies the same evidence-based promotion rule the agent
uses, seeded from history (see [`lifecycleFor`](#mandatory-metadata)).

**`Tags["Lifecycle"]` is the source of truth for the stage; the native
`Lifecycle` field governs gating and is only ever *relaxed* toward it.**
Every suite qualifier reads `test.tags.<key>` and selects on the
four-stage `Tags["Lifecycle"]`; nothing selects on OTE's native two-value
`Lifecycle` field. That native field is not discarded — the executor still
uses it to decide whether a failure is fatal — so it is kept consistent
with the stage by a one-way, informing-only rule: a *running* `Informing`
test is relaxed to native `informing`, while `Blocking`/`Stable` and any
untagged test are left to the native default of `blocking` (a `Draft` test
never runs, so its gating value is moot and it carries no native
annotation). Crucially, this native annotation is
**materialized in the test source by the lifecycle agent**, not projected
at build time: the same centralized path that stamps a test's stage tag
also writes (or removes) `ote.Informing()` in the test's source in the
same change, so the gating decision is visible in the diff and greppable in
source rather than an invisible runtime transform (see
[OTE API Extension: Inline Tags](#ote-api-extension-inline-tags) and
[Mandatory Metadata](#mandatory-metadata)). An `Informing` test therefore
carries `ote.Informing()` and runs non-fatal, while a `Blocking`/`Stable`
test carries no native annotation and therefore stays `blocking` by
default — exactly the intended gating in both cases. Because the agent only
ever writes `informing` and never `blocking`, a test that gates today can
never be silently de-gated.

The dedicated `Criticality: Core` step above is the function referenced
in [Upstream and Externally-Sourced Tests](#upstream-and-externally-sourced-tests):
it is the single place that maps the upstream `[Conformance]` label onto
the new metadata vocabulary.

`lifecycleFor` and `shardFor` are deterministic so the initial placement
of a bulk-imported set is repeatable rather than ad hoc:

- **`lifecycleFor`**: these tests are not new — they have years of
  history — so the initial import should not blanket everything to
  `Informing` and re-run the whole stabilization clock. Instead it
  applies the *same* promotion rule the lifecycle agent uses ongoing,
  seeded from historical data: a test that already gates today as
  conformance, or that meets the `Blocking` bar over the historical
  window (sustained `>= 99%` pass rate with sufficient samples, per
  [Pass-Rate Calculation](#pass-rate-calculation)), imports directly as
  `Blocking`; only tests that fall below the bar, or lack enough
  historical data to judge, import as `Informing` and stabilize through
  the normal path. This avoids artificially demoting long-stable tests to
  non-blocking on day one, and it keeps the annotation and tag in sync via
  the same normalization as authored tests. (Graduation to `Stable` still
  goes through the agent after GA + 1; the importer does not seed
  `Stable` directly.)
- **`shardFor`**: assigns a starting `Shard` deterministically (e.g. a
  stable hash of the test name into the current shard count, or simply
  `01`), after which the balancing workflow redistributes shards to meet
  the runtime target. The invariant is not merely that every imported
  test receives *a* `Shard`, but that it receives one drawn from the
  **registered shard set** for its `(suite class, mode)` — see
  [Shard Registry](#shard-registry). Assigning a shard with no matching
  suite would drop the test from every running suite, so `shardFor` never
  invents a shard number.

These are migration defaults, not a permanent classification: once a
test is imported it follows the same lifecycle transitions and shard
rebalancing as any other test.

###### Shard Registry

`Shard`'s vocabulary (`01`–`NN`) is not open-ended: the set of shards
that actually exist is defined by an **authoritative registry** — the set
of `active`/`stable` shard suites that are registered via `AddSuite` and
that have a corresponding CI job.

The registry is keyed by **`(suite class, mode)`**, where the suite class
is `active` or `stable` — *not* by the raw `Lifecycle` value. This
matters because the `active` shard suites select both `Informing` and
`Blocking` (see [Shard Membership Qualifiers](#shard-membership-qualifiers)):
there is exactly one `active-NN` suite and one CI job per shard, and both
lifecycle values resolve to it. Keying the registry on the suite class
means an `Informing → Blocking` promotion does **not** change the
registry key or require a different shard suite/job — the test stays in
the same `active-NN` shard, and only its gating behavior changes. `NN` is
the number of registered shard suites for that `(suite class, mode)`,
bounded by the configurable per-suite maximum in
[Shard Balancing](#shard-balancing).

`Draft` is **outside** the registry entirely. A `Draft` test is in no
suite and does not execute, so it has no `(suite class, mode)` to resolve
to; its `Shard` is not validated against the registry (see the exemption
in [Mandatory Metadata](#mandatory-metadata)). Only `Informing`,
`Blocking`, and `Stable` tests are registry-checked.

The registry is what makes shard assignment safe:

- For a non-`Draft` test, `shardFor` and the balancing workflow may only
  assign a `Shard` value present in the registry for the test's
  `(suite class, mode)`. An assignment to an unregistered shard would
  match no suite, so the metadata-enforcement check treats an
  out-of-registry `Shard` as invalid (the same way it treats an
  out-of-vocabulary `Lifecycle`).
- A new shard becomes assignable **only after** its suite (`AddSuite`)
  and its CI job are registered. The balancing workflow therefore
  registers the suite/job first and assigns tests into the shard second,
  never the reverse. This keeps "a `Shard` value exists" and "a suite +
  job exist to run it" in lockstep. The suite lives in an extension
  binary while the CI job lives in `openshift/release`, so these two
  registrations are separate PRs and cannot land atomically. The ordering
  is a strict happens-before, not a transaction: the balancer must not
  assign any test to shard `NN` until *both* the `AddSuite` and the
  `openshift/release` job for `NN` have merged. Until then shard `NN` is
  simply not in the registry the balancer reads, so it cannot be chosen —
  a half-registered shard (suite merged, job not yet, or vice versa) is
  treated as not-yet-registered and never assigned to.

##### OTE API Extension: Inline Tags

The inline metadata form shown earlier (`ote.Tag(key, value)` on a
`g.It`) requires a small, backward-compatible extension to the
openshift-tests-extension library. This section specifies exactly what is
extended. The change is a natural generalization of a mechanism OTE
already has, and it is backward compatible — existing
`Informing()`/`Blocking()` callers keep working unchanged.

**What exists today.** Ginkgo only supports flat string `Label`
decorators; it has no key/value concept. OTE already encodes one such
value as a label and parses it back at build time, but it is deliberately
narrow: `ote.Informing()` / `ote.Blocking()` return
`ginkgo.Label("Lifecycle:informing")` (respectively `blocking`), and
`BuildExtensionTestSpecsFromOpenShiftGinkgoSuite` calls `GetLifecycle()`
to read that label into the spec's dedicated `Lifecycle` field. Two
properties of that field matter here and constrain the design:

- **It has exactly two values.** OTE's `Lifecycle` type is only
  `informing` or `blocking`; `GetLifecycle()` routes through
  `MustLifecycle()`, which **panics** on any other value. There is no
  `Draft` or `Stable`, and no `ote.Draft()` helper. So the two extra
  stages this enhancement needs (`Draft`, `Stable`) **cannot** be carried
  in the native `Lifecycle` field or its `Lifecycle:` label — doing so
  would crash the build.
- **It is load-bearing, not legacy.** The executor reads the native
  `Lifecycle` field to decide whether a test failure is fatal (`blocking`)
  or non-fatal (`informing`), and the value is serialized into the
  extension's JSON output that the runner and result pipeline consume. It
  therefore cannot be removed or repurposed in a single PR without
  breaking those consumers.

`ExtensionTestSpec` also carries a `Tags map[string]string` (key/value)
that the ginkgo build path does **not** populate today.

**Design consequence — two separate concepts.** Rather than force the
four-stage model into the two-value field (or make a breaking change to
OTE), this enhancement keeps them distinct:

- **`Tags["Lifecycle"]`** carries the full four-stage vocabulary
  (`Draft`/`Informing`/`Blocking`/`Stable`). This is what every suite
  qualifier selects on; it is authoritative for **membership**.
- **The native `Lifecycle` field** stays the two-value `informing` /
  `blocking` fatal-switch, unchanged. It is authoritative for **whether a
  failure fails the job**, and the **lifecycle agent keeps it consistent
  with the stage in one direction only**, materializing the annotation in
  the test source: it writes `ote.Informing()` for a running `Informing`
  test and leaves `Blocking`/`Stable` (and untagged tests) to the native
  `blocking` default, so a test that gates today is never silently
  de-gated. Tests that only use the pre-existing `Informing()`/
  `Blocking()` helpers keep gating exactly as before; to be *selected by
  the new suites* they must additionally carry a stage tag, which the
  centralized tagging path stamps during migration.

**What to add.** Deliberately small — one new decorator and one build
step. The existing `Informing()`/`Blocking()` helpers are **left exactly
as they are**; we do not add `Draft()`/`Stable()` look-alikes, because a
helper that resembles `Informing()` but behaves differently (tag only, no
native label) would invite a false assumption of symmetry. Stages other
than the two OTE already has are expressed with the generic `Tag()`.

1. A general decorator helper that encodes any key/value pair as a
   reserved-prefix `tag:key=value` label:

   ```go
   // In the OTE ginkgo helpers, next to Informing()/Blocking().
   // Metadata labels carry a reserved "tag:" prefix so they are
   // distinguishable from ordinary opaque ginkgo labels (e.g. a
   // Feature:Foo label is NOT metadata and must not become a tag).
   const tagLabelPrefix = "tag:"
   func Tag(key, value string) ginkgo.Labels {
       return ginkgo.Label(fmt.Sprintf("%s%s=%s", tagLabelPrefix, key, value))
   }
   ```

   The four stages are then written as: `ote.Tag("Lifecycle", "Draft")`,
   `ote.Tag("Lifecycle", "Informing")`, `ote.Tag("Lifecycle", "Blocking")`,
   `ote.Tag("Lifecycle", "Stable")`. The new suite qualifiers all select on
   `test.tags.Lifecycle`, so a test must carry a stage tag to participate;
   the pre-existing `ote.Informing()` / `ote.Blocking()` helpers keep
   working for native gating but do not by themselves place a test in the
   new suites. Existing tests are brought over by the centralized tagging
   path, which stamps the stage tag (and, when it promotes a test past a
   stale native label, drops the now-wrong `ote.Informing()` in the same
   change; see [Mandatory Metadata](#mandatory-metadata)).

2. A build-time step in `BuildExtensionTestSpecsFromOpenShiftGinkgoSuite`
   that promotes reserved-prefix labels into `Tags` (rejecting conflicting
   duplicates). The build step does **not** touch the native `Lifecycle`
   field — that field is written in the test source by the lifecycle agent
   (see below), so the build step only needs to surface the stage tag for
   suite selection:

   ```go
   // Promote ONLY reserved-prefix metadata labels into Tags so suite
   // qualifiers can select on test.tags.<key>. Ordinary labels (no "tag:"
   // prefix) are left untouched. A metadata key may repeat only if every
   // occurrence carries the same value; a conflicting duplicate (e.g. two
   // different Lifecycle values) is a build-time error, not last-writer-wins.
   for _, l := range spec.Labels() {
       raw, ok := strings.CutPrefix(l, tagLabelPrefix)
       if !ok {
           continue // not a metadata label
       }
       k, v, ok := strings.Cut(raw, "=")
       if !ok {
           return fmt.Errorf("malformed metadata label %q on %q", l, spec.Name)
       }
       if prev, seen := ets.Tags[k]; seen && prev != v {
           return fmt.Errorf("conflicting %q tag on %q: %q vs %q",
               k, spec.Name, prev, v)
       }
       ets.Tags[k] = v
   }
   ```

   The native `Lifecycle` field is kept consistent with the stage by the
   **lifecycle agent, which is the sole writer of that field**, and it
   writes it *into the test source* rather than as a runtime projection.
   The rule is one-way and informing-only: when the agent sets or updates a
   test's stage tag, it materializes the paired native annotation in the
   same change — for a running `Informing` test it ensures `ote.Informing()`
   is present; for `Blocking`/`Stable` it ensures `ote.Informing()` is
   *absent* so the test stays `blocking` by the native default. (A `Draft`
   test never runs, so it needs no native annotation.) Because the
   agent only ever writes `informing` (never `blocking`), the two systems'
   opposite defaults never collide: the new stage's default is `Informing`
   (non-gating) while OTE's native default is `blocking` (gating), and a
   test that is `blocking` today is never accidentally flipped. Materializing
   the annotation in source also handles a *promotion past a stale label* —
   a test that still carries `ote.Informing()` but whose new stage is
   `Blocking`/`Stable` — directly at the source: the same change that stamps
   the higher stage removes the now-wrong `ote.Informing()` (see
   [Mandatory Metadata](#mandatory-metadata)). A forgotten annotation
   therefore under-gates rather than over-gates — the safe direction — and
   the metadata-validation check flags any stage/annotation disagreement.
   An untagged legacy test the agent has not yet processed keeps whatever
   native annotation it has today, so nothing it does silently changes
   existing gating.

With these, `ote.Informing()`/`ote.Blocking()` are unchanged and keep
driving native gating; a legacy test joins the new suites only once the
centralized path stamps its stage tag. `Draft` and `Stable` are expressed
with `ote.Tag("Lifecycle", …)` and carry no native annotation: a `Draft`
test never runs, so its gating value is moot, and a `Stable` test is left
`blocking` by the native default. The lifecycle agent materializes
`ote.Informing()` in source only when a test enters a running suite as
`Informing` (and removes it again on promotion to `Blocking`/`Stable`).
`ote.Tag("Criticality",
"Core")` populates `Tags["Criticality"]`. (`Shard` is not declared inline — it is assigned
by automation — so it enters `Tags` through the centralized tagging path,
not this decorator; see [Mandatory Metadata](#mandatory-metadata).) All
suite qualifiers in this enhancement select on the key/value namespace
(`test.tags.<key>=="<value>"`) — never on the native two-value field —
while authors declare everything inline on the test. This work is tracked
in [Infrastructure Needed](#infrastructure-needed-optional).

To summarize, the concrete OTE changes are just two:

1. Add a `Tag(key, value)` decorator that emits a reserved-prefix
   `tag:key=value` ginkgo label. (No new lifecycle helpers — the four
   stages are written as `Tag("Lifecycle", …)`, and the existing
   `Informing()`/`Blocking()` are untouched.)
2. In `BuildExtensionTestSpecsFromOpenShiftGinkgoSuite`, add a build-time
   step that promotes reserved-prefix labels into `Tags` with a
   single-value conflict check. The build step does not write the native
   `Lifecycle` field at all.

No existing OTE type, helper, or serialized field is removed or changed
in meaning; the native `Lifecycle` field keeps its two values and its
executor semantics. The stage tag governs suite selection; the native
field governs gating, and the two stay consistent because the **lifecycle
agent is the sole writer of the native field** — it only ever relaxes that
field (to `informing`) and applies stage promotions at the source together
with removal of any stale native annotation, all materialized in the test
source rather than at build time.

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
- **Approval**: Because spot-check suites bring dedicated jobs and
  specialized cluster configs, adding one is not unchecked. The suite
  and its job configuration land through the normal config pull-request
  review, giving reviewers a predictable, agreed environment for the
  test rather than an ad-hoc one.
- **Longevity**: Spot-check suites are not expected to retire. Rather
  than being removed after a feature matures, a suite settles into its
  minimum run interval (see the maintenance-phase cadence above) and
  continues to run indefinitely, with its health tracked through
  Component Readiness (see
  [Risks and Mitigations](#risks-and-mitigations)). This keeps the
  total number of active configurations bounded through low-frequency
  steady-state execution rather than through eventual deletion.

#### Shard Balancing

- **Objective**: Maintain all shards within 10% of mean runtime.
- **Driver**: The lifecycle agent, on a schedule, performs the workflow
  below and opens the resulting pull requests. QSE engineers review
  them; they are not authored by hand.
- **Workflow**:
  1. Query Sippy for per-test and per-job runtimes.
  2. Move tests between shards by changing each test's single `Shard`
     tag value (one change to preserve exclusive shard membership). A
     test is only ever moved to a shard that is already in the registry
     (see [Shard Registry](#shard-registry)); the agent never assigns a
     `Shard` value that has no registered suite and job.
  3. Create new shards when existing ones cannot be rebalanced below
     runtime thresholds, up to a configurable per-suite maximum shard
     count. Creating a shard means **first** registering its suite
     (`AddSuite`) and its CI job — adding the shard to the registry — and
     **only then** assigning tests into it, so a `Shard` value never
     exists without a suite and job to run it. This ceiling bounds the
     number of shards (and therefore the number of prow jobs) a suite can
     spawn so that an unexpected influx of tests, or a bug in the
     balancing logic, cannot trigger unbounded shard creation. When the
     maximum is reached and shards still exceed the runtime threshold, the
     agent stops creating shards and instead flags the suite for human
     attention rather than silently degrading.

#### The Lifecycle Agent

The lifecycle agent is a scheduled prow job (weekly or per-sprint) that
removes routine suite maintenance from the human path. On each run it:

1. Enumerates the test inventory (see [Locating Tests](#locating-tests)).
2. For each `Lifecycle: Informing` test in `active`, evaluates promotion
   eligibility (stabilization window elapsed, sample count met, pass
   rate `>= 99%` over the measurement window) and opens a PR flipping
   `Lifecycle` to `Blocking` (and removing `Informing()`) when eligible.
3. For each `Lifecycle: Blocking` `active` test past GA + 1, opens a PR
   that flips `Lifecycle` from `Blocking` to `Stable` (moving it to the
   chosen explicit `stable` shard) and assigns its `Shard` tag in the
   same change.
4. Recomputes shard balance from Sippy and opens rebalancing PRs.

The agent never touches `Lifecycle: Draft` tests: advancing a test out
of `Draft` is a manual, owner-driven action. Promotion is one-way: the
agent never demotes a blocking test back to `Informing` and never moves
a test to a lower suite. A subsequent pass-rate degradation is a
regression to be fixed by the owning component, not something the agent
reverses. The agent also does not manage spot-check suites or jobs.

Precedent exists for this style of automated, PR-driven maintenance in
the TRT tooling ecosystem. All actions are proposals: the agent opens
pull requests, it does not merge them.

##### Locating Tests

Reliable automation requires knowing which tests are
`Lifecycle: Informing`, which are eligible for promotion, and —
importantly — where their source
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
once `Lifecycle: Blocking`, a degradation is handled as a regression to
be fixed, not as a trigger to relax the test's lifecycle state. The
specific numeric
parameters above are proposals and are called out in
[Open Questions](#open-questions-optional) for confirmation with TRT.

#### Test Requirements Checklist

Before assigning any test to a suite, ensure the following:

- Carries the required metadata tags: for a non-`Draft` test, a
  registered `Shard` value; plus `Criticality: Core` if it is a core
  smoke test. `Lifecycle` is normalized to `Informing` when absent and
  must otherwise be `Draft`, `Informing`, `Blocking`, or `Stable`. (`Draft`
  tests are not assigned to a suite and need no `Shard`.) This is enforced
  in CI (see [Mandatory Metadata](#mandatory-metadata)).
- Has a `[Jira:Component]` label for component ownership. This replaces
  the older `[sig-XYZ]`-based grouping for downstream, OpenShift-authored
  tests: new downstream tests should carry a Jira component rather than
  relying on a `[sig-XYZ]` label for ownership. Note that the `[sig-XYZ]`
  form still exists on upstream (Kubernetes) tests, which we do not
  intend to re-label; the deprecation applies to how we categorize our
  own downstream tests, not to the upstream tests we vendor.
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
label are mapped to `Criticality: Core` and so placed in the
`.../minimal` suites (on the order of a few hundred in parallel-minimal
and a couple dozen in serial-minimal); the remaining, non-`[Conformance]`
upstream tests are not `Core` and land in the broader
`openshift/conformance/parallel` (or `openshift/conformance/serial`)
suites rather than `.../minimal`.

The metadata mapping applied to upstream tests is, effectively:

| Test attributes | Metadata | Suite |
|-----------------|----------|-------|
| `[Serial]` + `[Conformance]` | `Criticality: Core` (serial) | `openshift/conformance/serial/minimal` (edited in place) |
| `[Serial]` (non-Conformance) | (no `Core`) | `openshift/conformance/serial` |
| `[Conformance]` (parallel) | `Criticality: Core` (parallel) | `openshift/conformance/parallel/minimal` (edited in place) |
| parallel, non-Conformance | (no `Core`) | `openshift/conformance/parallel` |

The Core-smoke rows land in the `.../minimal` suites immediately: those
are edited in place (selection changed to `Criticality: Core`), so a
stamped `[Conformance]` test qualifies as soon as the mapping runs. The
non-`Core` rows show the *existing* `openshift/conformance/parallel` /
`.../serial` aggregates; new-hierarchy membership (`active`/`stable`)
reaches those aggregates only at the maturity graft (see
[Exclusive Membership and Rollout](#exclusive-membership-and-rollout)),
and the existing aggregate definitions are never edited to get there.

Plan for these under the new hierarchy:

- **Upstream tests are added to `active`/`stable` in addition to
  `minimal`**, not instead of it. `Criticality: Core` (mapped from the
  `[Conformance]` label) is an additional, faster smoke layer; the same
  test also carries a `Lifecycle` value and so participates in a
  lifecycle shard. A test is therefore a member of both `minimal` and
  one of the `active`/`stable` shards at the same time.
- **Tagging is done centrally, not inline.** Because we do not want to
  edit vendored upstream test source, the `Criticality`/`Lifecycle`
  /`Shard` tags are assigned by a dedicated function that walks the
  imported specs and applies tags from a rules table (see the example in
  [Mandatory Metadata](#mandatory-metadata)). In particular, a single
  function adds `Criticality: Core` for every test carrying the upstream
  `[Conformance]` label. This mirrors how openshift/kubernetes already
  classifies these tests today — on `master` via the `k8s-tests-ext` OTE
  extension binary's centralized label and qualifier tables, and on
  release branches via the legacy `annotate.go`/`rules.go` mechanism.
- **We retain the ability to categorize.** Because everything flows
  through OTE tags and CEL qualifiers, upstream-sourced tests are ours
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
tag centrally (rather than owning inline) is the in-tree Kubernetes
set via the `hyperkube` extension; everything else is
OpenShift-authored and can be tagged at the source. (Caveat: this
reflects the current registry and two representative binaries verified
by source; it was not feasible to audit every one of the ~35 binaries,
so a component silently re-exporting an upstream suite cannot be
100% ruled out.)

#### Suites in Test Names

`openshift/origin` historically encodes suite membership in the test
name (e.g., bracketed `[Suite:...]` labels). Under this proposal, suite
membership is expressed through OTE metadata tags and CEL qualifiers, not
the test name. The `[Suite:...]` name labels are deprecated on the same
schedule as the other legacy string labels (see
[Removing a deprecated feature](#removing-a-deprecated-feature)); the
`[sig-XYZ]` and `[Jira:Component]` markers remain in the name because
they convey ownership/categorization rather than suite membership.

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

1. **Downstream `Criticality: Core` rule.** For upstream Kubernetes
   tests, `Criticality: Core` (and thus `minimal` membership) is mapped
   mechanically from the `[Conformance]` label. For downstream,
   OpenShift-authored tests there is no equivalent established signal, so
   which tests should carry `Criticality: Core` is not yet objectively
   defined and likely requires manual, per-component evaluation to start.
   Defining a repeatable rule is an open question (see
   [Minimal-Suite Membership](#minimal-suite-membership)).

   As a starting point for that discussion, the following candidate
   conditions could be used (individually or combined) to qualify a
   downstream test for `Criticality: Core`; they are illustrative, not
   yet agreed, and need a broader audience to refine:

   - Short runtime (e.g. test duration under a couple of minutes), so
     the `minimal` suite stays fast.
   - High blast radius: covers a GA feature whose breakage would render
     a large fraction of clusters unusable if the test were removed.
   - No external dependencies beyond the core Kubernetes API.
   - A demonstrated track record of stability (e.g. sustained `>= 99%`
     pass rate across multiple releases).
   - Explicit sign-off from an architect or equivalent reviewer.
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
- Validating that the `Lifecycle` metadata (and the paired `Informing()`
  annotation) correctly gates test blocking status, and that
  `Lifecycle: Draft` tests are excluded from every suite.
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
- The downstream `Criticality: Core` rule (see
  [Open Questions](#open-questions-optional)) is defined and applied, so
  the `conformance/*/minimal` suites hold only agreed smoke tests.
- The lifecycle agent is live and opening promotion, graduation, and
  rebalancing PRs; the shard-balancing workflow is exercised through it
  rather than by hand.
- The matured shard suites are grafted into the existing conformance
  suite via `Parents` (each shard suite declares
  `Parents: ["openshift/conformance/parallel"]` / `.../serial`), so their
  tests contribute to the established conformance signal without editing
  the existing suite definitions (see
  [Exclusive Membership and Rollout](#exclusive-membership-and-rollout)).
- Old flat suite labels are fully deprecated.

### Removing a deprecated feature

- Announce deprecation of old `[Suite:...]` string labels.
- Provide migration tooling to convert existing suite assignments to
  OTE API calls.
- Remove support for old labels after one full release cycle.

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
| OTE inline-tag API extension (`ote.Tag` decorator + build step promoting `tag:key=value`→`Tags`; native `Lifecycle` written in source by the lifecycle agent, not the build step) | Declare metadata inline on the `g.It` so qualifiers select on `test.tags.<key>` (see [OTE API Extension: Inline Tags](#ote-api-extension-inline-tags)) | TBD (QSE + Test Platform) | Before Dev Preview → Tech Preview |
| Parallel validation jobs in `openshift/release` | Run the new hierarchy alongside existing suites (non-gating) to compare coverage and runtime | TBD (QSE + Test Platform) | Must exist and be green before Dev Preview → Tech Preview |
| CI metadata-enforcement check | Fail any test that lacks its required metadata tags (`Shard`; `Lifecycle` defaults to `Informing`) or carries an out-of-vocabulary value, keeping suite membership consistent | TBD (QSE + Test Platform) | Required before Tech Preview → GA |
| Sippy queries for pass-rate and shard-runtime analysis | Feed the lifecycle agent's promotion and rebalancing decisions | TBD (TRT) | Required before the agent leaves dry-run |
| Test-source index / robust test database | Map each test to its owning repo and source so the agent can open PRs in the right place | TBD (TRT) | Required for full agent coverage; agent operates on the resolvable subset until then |
| Lifecycle agent prow job | Scheduled automation that opens lifecycle PRs | TBD (QSE) | Required before Tech Preview → GA |

If any required dependency cannot be provided, the corresponding
acceptance criterion (parallel-validation coverage, agent-driven
graduation) must be revised rather than silently skipped.
