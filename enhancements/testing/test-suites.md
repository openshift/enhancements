---
title: test-suites
authors:
  - "@kenzhang"
reviewers:
  - "@dgoodwin"
  - "@jogeo"
  - "@miyadav"
approvers:
  - "@dgoodwin"
api-approvers:
  - None
creation-date: 2026-07-31
last-updated: 2026-11-04
tracking-link:
  - https://redhat.atlassian.net/browse/OCPQUAL-1
status: provisional
---

# Test Suites

## Summary

OpenShift test suites today are a flat model: suite membership is encoded
as `[Suite:...]` strings in test names, conformance is very large, and
each test separately declares whether its failures block (OTE's
`Informing()` / `Blocking()` lifecycle).

This enhancement replaces that with four ordered suites — `candidate`,
`active`, `stable`, and `minimal` — and makes **suite membership the only
lifecycle concept**. Each test declares the suites it belongs to directly
in its metadata. Whether a failure matters is a property of *where a
suite runs* (which jobs, whether Component Readiness covers it, whether
it gates payloads), not a per-test flag. New tests start in `candidate`,
and a scheduled lifecycle agent promotes them forward on measured
evidence by opening pull requests. Tests are never demoted.

## Motivation

- **Conformance is too large and too slow**, and payload and presubmit
  signal is diluted by tests that have no established track record.
- **Informing tests languish.** Promoting a test requires a human to
  notice it is eligible and open a PR. In practice this rarely happens:
  informing tests sit indefinitely, whether long since stable or
  chronically broken.
- **Two overlapping concepts.** Today a test has both a suite (in its
  name) and a lifecycle (`Informing()` / `Blocking()`), and the two
  interact implicitly. Changing a suite means renaming the test, which
  breaks its history.
- **Sharding is implicit**, which makes scheduling unpredictable in
  constrained environments such as vSphere.

### User Stories

- As a test author, I want to add a test without risk to release signal,
  and have it promoted automatically once it proves itself.
- As a test author, I want to see which suites my test runs in by reading
  its definition.
- As a release engineer, I want payloads gated only by tests with a
  proven record of stability across releases.
- As a developer, I want presubmits to run a small, fast, reliable set of
  core tests.
- As a CI infrastructure engineer, I want explicitly sharded suites with
  balanced runtimes so jobs schedule predictably.

### Goals

1. Make explicit, per-test suite membership the single mechanism for both
   organizing tests and expressing their maturity.
2. Define four suites with clear run locations and graduation criteria.
3. Shrink what runs on presubmits (`minimal`) and payloads (`stable` +
   `minimal`) to tests with proven stability.
4. Drive promotion and shard balancing with a scheduled agent that opens
   PRs, so promotion happens without manual toil.
5. Provide explicit, balanced shards.

### Non-Goals

1. Fully autonomous merging. The agent opens PRs; humans approve them.
2. Demotion. Neither the agent nor humans move a test to a lower suite
   (see [No Demotion](#no-demotion)).
3. Changing how individual tests are written (Ginkgo, etc.).
4. Defining every CI job; those live in `openshift/release`.

## Proposal

### The Suites

```text
candidate --(3 weeks, >=95%)--> active --(2 releases, >=98%)--> stable --(designated core)--> minimal
```

| Suite | Contains | Runs on | Component Readiness | Promotion out |
|-------|----------|---------|---------------------|---------------|
| `candidate` | New tests (the default). Equivalent to today's informing tests. | Dedicated periodic jobs only | No | Agent: at least 3 weeks in `candidate` (one full sprint), `>= 95%` pass rate |
| `active` | Tests graduated from `candidate`; existing flaky tests | Periodic jobs only | Yes | Agent: `>= 98%` pass rate, and at least 2 OCP releases since the test first appeared (added at any point in 5.0, eligible in 5.2) |
| `stable` | Tests with proven stability | Payloads and periodics | Yes | Human or agent designates the test as critical product functionality |
| `minimal` | Core cluster functionality; logically part of `stable` | Presubmits, payloads, periodics, and every `stable` run | Yes | — |

Each suite is split into parallel and serial forms, and `candidate`,
`active`, and `stable` are explicitly sharded, for example
`openshift/conformance/parallel/active-02` or
`openshift/conformance/serial/stable-01`.

`minimal` is part of `stable`: it is effectively the one stable "shard"
that also runs on presubmits. A test promoted to `minimal` *moves* out of
its stable shard instead of being listed in both, and any job that runs
`stable` also runs `minimal`, so core tests are never run twice and never
dropped from a stable run.

**Gating is per job, not per test.** A `candidate` failure cannot block
anything because `candidate` is not run on payloads or presubmits and is
not covered by Component Readiness. The agent is effectively the only
consumer of `candidate` results: it looks for tests to promote and for
new tests that are broken and whose owners should be told. Tests in the
new suites therefore do not need OTE's per-test `Informing()` /
`Blocking()` annotation.

### Workflow Description

**test author** writes a test in their component repository.
**lifecycle agent** is a scheduled prow job that promotes tests and
rebalances shards by opening PRs.
**approver** is a QSE engineer, or after a response window, an architect
or staff engineer, who approves agent PRs.

#### Adding a test

The author writes the test with no suite metadata. A test that matches
no registered suite is a `candidate`, placed in a candidate shard
automatically (see [Shards](#shards)). Nothing else is required:

```go
g.It("should do the thing", func() { ... })
```

The implicit default is temporary. Within a day or a week, the agent
opens a PR that writes the test's candidate shard into its source, so
any test's suites can be read from its definition:

```go
g.It("should do the thing",
    ote.Suites("openshift/conformance/parallel/candidate-03"),
    func() { ... },
)
```

#### Promotion

On each run, the agent evaluates every test against the criteria for its
current suite and opens a PR for each eligible test that rewrites its
suite list. Over its life, the test above changes like this:

```go
// candidate -> active (agent)
g.It("should do the thing",
    ote.Suites("openshift/conformance/parallel/active-02"),
    func() { ... },
)

// active -> stable (agent)
g.It("should do the thing",
    ote.Suites("openshift/conformance/parallel/stable-05"),
    func() { ... },
)

// stable -> minimal (human or agent designates it as core)
g.It("should do the thing",
    ote.Suites("openshift/conformance/parallel/minimal"),
    func() { ... },
)
```

Every transition is a single edit to a single test. Sibling tests for the
same feature are unaffected.

#### Override: straight to stable

A human may decide a new test should skip `candidate` and `active` and
opt into payload gating and regression coverage immediately, by authoring
it directly into a `stable` (or `minimal`) suite:

```go
g.It("should never lose etcd quorum during upgrade",
    ote.Suites("openshift/conformance/parallel/stable-03"),
    func() { ... },
)
```

This requires explicit approval (approver set TBD, see
[Open Questions](#open-questions-optional)). The agent reports any test
that first appeared outside `candidate` without an approved override.

#### No Demotion

Tests only move forward. If a test that was passing starts failing, that
is a regression. It is caught by Component Readiness (`active`,
`stable`, `minimal`) and, for `stable` and `minimal`, treated as a release
blocker under the existing process. It is not grounds to move the test
back to a lower suite. Neither the agent nor humans demote tests. An
escape hatch for exceptional cases may be needed someday; if it is added,
it will require heavy approvals and is out of scope here.

#### Tests stuck in candidate

Tests that never meet the bar to leave `candidate` are the same problem
as tests stuck in informing today, and the agent is the solution. It
identifies `candidate` tests that are failing, or that have stalled below
the promotion bar, and notifies the owning team. The notification
mechanism is an open question.

#### Dedicated suites

Some tests must run only in their own job, because their setup
reconfigures the shared cluster or they need an uncommon configuration
(etcd scaling, realtime nodes, external OIDC). They simply declare that
suite:

```go
g.It("should scale etcd from three to five members [Serial]",
    ote.Suites("openshift/etcd-scaling"),
    func() { ... },
)
```

A test in a dedicated suite is not in the lifecycle suites, and the agent
does not manage it. The owning team chooses the job's cadence and tier;
Component Readiness monitors it like any other job. Jobs that run too
rarely for normal regression analysis (roughly less than daily) are
marked as spot-check in the Sippy variant registry by developers.

### API Extensions

None. This enhancement adds no CRDs, webhooks, or API changes.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations; the suites apply regardless of topology.

#### Standalone Clusters

The suites are fully applicable to standalone clusters.

#### Single-node Deployments or MicroShift

No additional resource consumption.

#### OpenShift Kubernetes Engine

No dependency on features excluded from OKE.

### Implementation Details/Notes/Constraints

#### Declaring suites

A small, backward-compatible OTE addition: an `ote.Suites(names...)`
decorator that emits one reserved-prefix Ginkgo label per suite
(`suite:<name>`). Ginkgo labels are not part of the test name, so changing
a test's suites never changes its name or breaks its history, unlike
today's `[Suite:...]` name strings.

Origin defines the lifecycle suites in its static suite table. Because
membership is declared on the test, each suite's qualifier is a trivial
lookup that can be generated rather than hand-written:

```go
ginkgo.TestSuite{
    Name:       "openshift/conformance/parallel/stable-05",
    Qualifiers: []string{`test.labels.has("suite:openshift/conformance/parallel/stable-05")`},
}
```

Static suite qualifiers apply across all extension binaries, so tests
from any repository can join any registered suite.

#### Validation

Origin sees every extension's tests and every registered suite, so it
validates membership when it aggregates specs. A test is rejected if:

1. It names a suite that is not registered.
2. It is in more than one lifecycle suite (`candidate`, `active`,
   `stable`, or `minimal`) or shard.
3. It is in both a lifecycle suite and a dedicated suite.
4. Its suite's mode disagrees with its name: a `[Serial]` test in a
   parallel suite, or the reverse.

This is the only enforcement needed. The previous design derived these
guarantees from qualifiers combining several tags; with explicit
membership they become direct checks.

#### Shards

- **`active` and `stable`**: the agent writes the shard explicitly as part
  of the suite name, and rebalances by rewriting it. Target: every shard
  within 10% of mean runtime, using per-test runtimes from Sippy.
- **`candidate`**: assigned automatically, in memory, when origin
  aggregates specs from all extension binaries. Origin first evaluates
  every registered suite. A test that **matches no other registered
  suite** is placed in a `candidate-NN` shard for its mode (from
  `[Serial]`) by consistent hashing of its name. This keeps tests that
  legacy or special-purpose suites select by qualifier (for example,
  disruptive tests) out of `candidate`. New tests need no source edit to
  start running. The agent then makes the assignment explicit by writing
  the *same* shard origin chose into the test's source, so the test does
  not move. From then on, candidate shards are explicit and rebalanced
  like any other. Hashing only covers the window between merge and the
  agent's next run.
- **Registry**: a shard exists only when both its static suite in origin
  and its CI job in `openshift/release` have merged. The agent never
  assigns a test to an unregistered shard; it registers new shards first.
  A configurable per-suite maximum bounds the number of shards. When
  that maximum is reached and shards still exceed the runtime target,
  the agent flags the suite for human attention.

#### Promotion criteria

- **Measurement**: completed runs only. Aborted and infrastructure-failed
  runs are excluded, and flake attribution follows existing Sippy and
  Component Readiness definitions. A minimum sample count applies before
  any promotion.
- **`candidate` → `active`**: at least 3 weeks in `candidate` and a
  `>= 95%` pass rate.
- **`active` → `stable`**: a `>= 98%` pass rate, at least 2 OCP releases
  since first appearance (from Sippy's first-seen release, not stored
  metadata), and no per-variant regressions. Promotion uses Component
  Readiness's per-variant analysis so a test that passes on one platform
  but fails on another is not promoted. The exact use of per-variant data
  is an implementation detail of the agent.
- **`stable` → `minimal`**: a human or the agent designates the test as
  critical product functionality. A repeatable rule for this is an open
  question.

#### The lifecycle agent

A scheduled prow job that on each run:

1. Enumerates tests and their current suites, as resolved by origin.
2. Opens PRs that make implicit candidate membership explicit (daily or
   weekly; the other tasks can run less often).
3. Opens promotion PRs for eligible tests.
4. Opens shard rebalancing PRs, registering new shards first if needed.
5. Notifies owning teams about broken or stalled `candidate` tests.

The agent never merges. Promotion and rebalancing PRs follow a
response-window policy: the owning team has a defined number of days to
respond, after which an architect or staff engineer may approve. To open
PRs, the agent must map each test to its source repository; until a
complete test-source index exists, it operates on tests resolvable through
`ci-test-mapping` and OTE source metadata.

#### Upstream Kubernetes tests

In-tree Kubernetes e2e tests, run through the `k8s-tests-ext` extension in
openshift/kubernetes, are the only large test set whose source we do not
edit. Every other extension binary is OpenShift-authored and declares
suites inline. For upstream tests, `k8s-tests-ext` assigns suites
centrally from a rules table instead of inline. Tests carrying the
upstream `[Conformance]` label go to `minimal`; others are placed by
history (see [Migration](#migration)) and then follow the normal
lifecycle.

#### Job plan

| Suite | Presubmits | Payloads | Periodics |
|-------|-----------|----------|-----------|
| `candidate` | | | Dedicated candidate jobs |
| `active` | | | Yes |
| `stable` | | Yes | Yes |
| `minimal` | Yes | Yes (with `stable`) | Yes (with `stable`) |

Existing jobs keep their names. Because payload jobs run only tests that
have graduated on sustained pass-rate evidence, there is an opportunity
to reduce or eliminate aggregation for some payload jobs. That will be
evaluated during validation.

A possible follow-up: component-scoped presubmits that run `minimal` plus
the tests owned by the component under test. Each repository's component
is read from its root `OWNERS` file and matched against a `Component`
tag on tests. This is separable from this enhancement.

#### Migration

Existing tests are seeded from history instead of restarting everyone in
`candidate`:

- Tests marked `Informing()` today go to `candidate`.
- Tests that meet the `stable` bar historically, including today's gating
  conformance tests at `>= 98%`, go to `stable`.
- Upstream `[Conformance]` tests go to `minimal`. Downstream tests are
  designated core afterward, case by case.
- Everything else goes to `active`.

Seeding must complete before the candidate default is enabled. Otherwise
every not-yet-migrated test matches no suite and falls into `candidate`.

`[Suite:...]` name strings and the per-test `Informing()` /
`Blocking()` lifecycle are deprecated along with the old suites.

### Risks and Mitigations

**Risk**: Agent PRs stall waiting for review, recreating the backlog.
**Mitigation**: A response-window policy after which an architect or
staff engineer may approve.

**Risk**: A test passes in `candidate` jobs but fails on variants those
jobs do not cover.
**Mitigation**: `active` is covered by Component Readiness, so these
failures surface there before the test reaches payloads. `active` →
`stable` promotion uses per-variant analysis.

**Risk**: Tests accumulate in `candidate` indefinitely.
**Mitigation**: The agent notifies owners about broken and stalled
candidates. This is the same problem informing tests have today, now with
an owner.

**Risk**: Component teams resist losing payload and presubmit coverage
for their tests while the tests mature.
**Mitigation**: The straight-to-`stable` override, and history-based
migration seeding so existing stable tests keep gating from day one.

### Drawbacks

- New tests take at least two releases to gate payloads unless a human
  uses the override.
- Shard rebalancing creates a steady stream of PRs to review.
- Migration requires coordination across many component teams.

## Alternatives (Not Implemented)

1. **A `Lifecycle` tag with suites derived from tags** (the previous
   revision of this enhancement). Tests carried a four-value
   `Lifecycle` tag plus `Shard` and `Criticality` tags, and CEL
   qualifiers derived membership from them. Rejected because it kept two
   lifecycle concepts (the tag and OTE's native `Informing()`) that had
   to be kept in sync, and it encoded each test's single suite as several
   coordinates.
2. **Auto-sharding.** Less maintenance, but it does not give the
   scheduling control constrained environments such as vSphere need.
3. **Suites defined only in CI job configuration.** This separates test
   organization from test definition and makes it harder for component
   teams to manage their tests.

## Open Questions [optional]

1. How the agent notifies teams about broken or stalled `candidate`
   tests.
2. A repeatable rule for designating `minimal` (core) tests.
3. Pass-rate sample minimums and measurement windows, to be confirmed
   with TRT.
4. Who approves the straight-to-`stable` override.
5. Which variants the `candidate` jobs cover, and how often they run.
6. Whether payload jobs running `stable` + `minimal` can drop
   aggregation.
7. Naming for new `candidate` and `active` periodic jobs.

## Test Plan

- Verify origin's aggregation produces the expected membership for each
  suite, and rejects invalid membership.
- Run the new suites in non-gating jobs alongside existing jobs to
  compare coverage and runtime before cutover.
- Dry-run the agent against historical Sippy data and review its
  promotion and rebalancing decisions before it opens real PRs.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `ote.Suites`, origin aggregation and validation, and the new suites are
  available.
- Non-gating validation jobs run alongside existing jobs.
- The agent runs in dry-run mode.
- At least one component team has migrated.

### Tech Preview -> GA

- All component teams have migrated, and upstream tests are assigned
  centrally.
- The agent is live and opening promotion and rebalancing PRs.
- Payload and presubmit jobs run `stable` + `minimal` and `minimal`
  respectively.

### Removing a deprecated feature

- Announce deprecation of `[Suite:...]` name strings and the old suites.
- Remove them one full release after GA.

## Upgrade / Downgrade Strategy

Not applicable; CI test infrastructure only.

## Version Skew Strategy

Not applicable; CI test infrastructure only.

## Operational Aspects of API Extensions

Not applicable; no API extensions.

## Support Procedures

Not applicable; does not affect production clusters.

## Infrastructure Needed [optional]

| Dependency | Owner | Needed by |
|------------|-------|-----------|
| `ote.Suites` decorator | TBD (QSE + Test Platform) | Tech Preview |
| Origin aggregation: candidate default, shard hashing, validation | TBD (QSE + Test Platform) | Tech Preview |
| `candidate` periodic jobs and non-gating validation jobs | TBD (QSE + Test Platform) | Tech Preview |
| Sippy pass-rate, first-seen, runtime, and per-variant queries | TBD (TRT) | Before the agent leaves dry-run |
| Test-source index mapping tests to repositories | TBD (TRT) | Full agent coverage |
| Lifecycle agent prow job | TBD (QSE) | GA |
