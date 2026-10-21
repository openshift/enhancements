---
title: must-gather-error-status-reporting
authors:
  - "@neha037"
reviewers:
  - "@shivprakashmuley"
  - "@swghosh"
  - "@Prashanth684"
approvers:
  - "@Prashanth684"
api-approvers:
  - "@Prashanth684"
creation-date: 2026-09-17
last-updated: 2026-10-21
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/MG-376
---

# Must-Gather Error Status Reporting

## Summary

This enhancement adds machine-readable error status reporting to `must-gather` by producing a versioned `/must-gather/status.json` that records per-collector success, skip, degraded, or error outcomes with bounded messages. Phase 1 (must-gather) establishes the archive-level contract for immediate use via `oc adm must-gather`. Phase 2 (must-gather-operator) propagates Job lifecycle conditions and `status.json` results to the `MustGather` CR, with Job-based fallback for older or custom images.

## Motivation

`must-gather` collects as much diagnostic data as possible even when individual collectors fail. Today the overall result does not reliably show which collectors failed, were not applicable, or produced only partial output. Support can receive an archive that looks successful while important directories are empty or incomplete.

The parent `gather` script starts collectors in the background and uses `wait "${pids[@]}"`, which returns only the last PID's exit status. Other failures are lost. The process ends with `sync` and effectively always exits `0`. Many collectors also suppress errors or exit `0` on expected skips, so success, skip, and partial failure are indistinguishable.

The Support Log Gather Operator must also surface Job lifecycle and failure reasons (OOMKilled, timeout, image pull) on the `MustGather` CR. That Job-layer visibility is independent of collector-level reporting and is Phase 2.

### Current baseline

An audit of the collector scripts shows:

* **Parent:** group `wait` loses all but the last exit code; no per-collector exit capture.
* **Collectors (24):** only 6 exit non-zero on failure; 11 silent-skip with exit `0`; 7 swallow real errors (`|| true`, `return`, `2>/dev/null`); 8 nest `wait` and lose child exit codes.
* **Consequence:** a per-PID wrapper alone would correctly detect failures from only 6 of 24 collectors. Phase 1 must deliver **both** wrapper infrastructure and collector instrumentation.

### User Stories

* As a support engineer, I want to see which collectors succeeded, failed, or were skipped before inspecting the archive.
* As an SRE, I want to know whether a failed collection is worth re-running.
* As a maintainer, I want consistent failure records to identify fragile collectors from production and CI evidence.
* As an automation or operator author, I want a machine-readable result that distinguishes complete from partial archives.
* As a cluster administrator using the Support Log Gather Operator, I want the `MustGather` CR to reflect Job lifecycle and collection quality without pod triage.
* As a release stakeholder, I want versioned, tested, documented behavior across image/operator skew.

### Goals

* Produce valid, versioned `/must-gather/status.json` in every successfully finalized default archive (Phase 1).
* Report status for every top-level work unit (28 units).
* Distinguish success, expected skip, degraded/partial, and collector failure.
* Preserve best-effort collection: one failure must not stop unrelated collectors or block archive finalization when possible.
* Capture non-zero top-level exits automatically; allow explicit `report_skip` / `report_error`.
* Aggregate nested child-process failures inside collectors.
* Serve both human archive inspection and automation / `MustGather` status.
* Keep older consumers and older/custom images usable when the report is absent or older-compatible.
* Tests and support procedures for interpreting the report (Phase 1).
* Propagate Job/Pod lifecycle to `MustGather` CR; consume `status.json` for bounded collection quality (Phase 2).

### Non-Goals

* Redesigning diagnostic data collected by existing collectors.
* Treating expected absence of optional operators/APIs/platforms as an error.
* Failing fast and abandoning remaining collection work.
* Guaranteeing every inner `oc` command as its own status entry in v1.
* Copying unrestricted raw collector stderr into a Kubernetes API object.
* Adding remote telemetry or a new always-running control-plane component.
* Treating absence of `status.json` (older/custom image) as proof that collection failed.
* Changing numeric collector process exit codes in v1. Normalization happens in `status.json` only; `report_skip` / `report_error` add structured signals while preserving current exit codes.

## Proposal

Two complementary mechanisms:

1. Common helpers let collectors report expected skips and meaningful errors at detection points.
2. A parent wrapper records every top-level work unit, including collectors that have not yet adopted explicit reporting.

After all units reach a terminal condition, the parent writes versioned `status.json`. When present, the report is the authoritative description of collection completeness. The archive remains best effort. Phase 2 consumes the same contract; the full report stays in the archive while the CR exposes a bounded aggregate.


### Two status layers

| Layer | What it reports | Source | Phase |
|-------|-----------------|--------|-------|
| **Job/Pod** | Job phase, termination, OOMKilled, timeout, image pull | Job/Pod status and events | Phase 2 |
| **Collector** | Per-collector success/skip/degraded/error; overall completeness | `/must-gather/status.json` | Phase 1 + Phase 2 consumption |

| Requirement | Layer | Phase 1 | Phase 2 |
|-------------|-------|---------|---------|
| CR reflects real-time Job state | Job/Pod | — | Condition sync |
| Failed Job surfaces clear error details | Job/Pod | — | Termination reason, exit, events |
| Standard K8s conditions | Both | — | Lifecycle + collection quality |
| Collector failures visible when Job succeeds | Collector | `status.json` | `collectionReport` |
| Tests for failure scenarios | Both | BATS + archive | Controller + integration |

### Implementation phases

One enhancement; two sequential phases across two repositories. Phase 2 depends on a stable Phase 1 schema (`schemaVersion: 1` finalized and tested).

#### Phase 1 — must-gather

**Goal:** Every default archive contains valid `/must-gather/status.json`. Usable via `oc adm must-gather` with no operator changes.

| Area | Deliverables |
|------|-------------|
| Core | `status_reporting.sh`: `report_error`, `report_skip`, init/finalize, atomic merge |
| Parent | Per-PID wait in `gather`; all 28 top-level units |
| Paths | Standardize `/must-gather/` for report and inconsistent collector outputs |
| Schema v1 | `schemaVersion`, collector outcomes, `framework`, bounded messages |
| Instrumentation | Priority set: `get_operator_ns` callers, monitoring, etcd, ppc, metallb, nested-wait collectors |
| Exit | `gather` exits `0` when archive finalizes; `overallStatus` carries completeness |
| Tests | BATS helpers/wrapper; fake collectors; archive validation; exit-0 regression; CI lint against group `wait` without per-PID handling |
| Docs | Support procedures for reading `status.json` |

**Phase 1 incremental delivery (3 PRs):**

| PR | Scope | Contents | Risk |
|----|-------|----------|------|
| **PR 1: Infrastructure** | Framework | `status_reporting.sh`, per-PID wait, `status.json` `schemaVersion: 1`, BATS for helpers, dead-code cleanup | Low — additive |
| **PR 2: Skip instrumentation** | Silent skips | `get_operator_ns` → `report_skip`; silent-skip collectors; BATS for skip detection | Low — exit codes unchanged |
| **PR 3: Error instrumentation** | Swallow + nested wait | monitoring, etcd, ppc, network_logs_basics, sriov, olm_v1, insights; nested-wait per-PID; BATS for error/degraded | Medium — collector behavior |

PR 1 does not change individual collectors. Support docs / error-code registry can proceed in parallel with PRs 2–3.

**Graduation gate:** Default image emits valid `status.json` for all top-level units on success, degraded, and collector-error runs where finalization succeeds. Schema v1 frozen before Phase 2.

**Out of scope for Phase 1:** `MustGather` CR, operator controller, Kubernetes API review.

#### Phase 2 — must-gather-operator

**Goal:** CR reflects Job lifecycle and collection quality from `status.json` when available.

| Area | Deliverables |
|------|-------------|
| Job lifecycle | Conditions: Pending, Running, Gathering, Uploading, Completed, Failed |
| Report consumption | Read `status.json` from gather output before cleanup/upload |
| CR status | Bounded `status.collectionReport` (counts, `overallStatus`, affected collectors) |
| Mapping | Job Failed → Failed; Succeeded + `degraded` → Completed/`CollectionDegraded`; Succeeded + report `error` → Completed/`CollectionFrameworkError`; missing/invalid → Job-only fallback |
| Skew | New/old image×operator; unsupported `schemaVersion` → graceful fallback |
| Tests / API review | Controller + e2e; conditions, reasons, size limits, redaction |

**Dependency:** Phase 1 schema v1 merged (or available in a test image). Operator work may start against a test image; must not merge until the schema is stable.

**Rollout:** New operator + old image → Job-only status. New image + old operator → `status.json` in archive only.

### Workflow Description

Actors: support engineer / admin (`oc adm must-gather` or operator-managed `MustGather`); must-gather image; collector scripts; must-gather-operator (Phase 2); automation reading archive or CR status.

1. Create `/must-gather/.status/` and record collection start.
2. Start top-level work units; each gets a stable name and private result location under `/must-gather/.status/`.
3. Run units in parallel where the workflow already allows it.
4. Collectors call `report_skip` / `report_error` when they detect non-applicability or failure.
5. Parent waits per PID (not group `wait`), records exit code, elapsed time, events, bounded messages.
6. Synchronous and post-wait steps (`gather_aro`, `compress_logs`) go through the same reporting path.
7. Merge fragments; atomically write `/must-gather/status.json` (temp under `.status/`, then rename).
8. Finalize archive; users can inspect `status.json` first.
9. Phase 2: operator reads the report before cleanup/upload; maps aggregate to CR status, or uses Job fallback.

#### Top-level work units

Every top-level unit launched by main `gather` (28 units):

| # | Stable name | Type |
|---|-------------|------|
| 1 | `inspect_named_resources` | Background `oc adm inspect` (named) |
| 2 | `inspect_group_resources` | Background `oc adm inspect` (group) |
| 3 | `inspect_all_namespaces` | Background `oc adm inspect` (all-namespaces) |
| 4–26 | `gather_insights`, `gather_monitoring`, `gather_olm`, `gather_olm_v1`, `gather_priority_and_fairness`, `gather_etcd`, `gather_service_logs`, `gather_windows_node_logs`, `gather_haproxy_config`, `gather_kas_startup_termination_logs`, `gather_network_logs_basics`, `gather_metallb`, `gather_frrk8s`, `gather_nmstate`, `gather_nfd`, `gather_sriov`, `gather_podnetworkconnectivitycheck`, `gather_machineconfig_ondisk`, `gather_machineconfigdaemon_termination_logs`, `gather_vsphere`, `gather_ppc`, `gather_osus`, `gather_istio` | Parallel collectors |
| 27 | `gather_aro` | Synchronous post-wait |
| 28 | `compress_logs` | Post-wait framework step (`skipped` unless `COMPRESS_AFTER_GATHER=true`). Reported under `framework`, not `collectors`. |

Standalone scripts not invoked by main `gather` (e.g. `gather_audit_logs`, `gather_core_dumps`) are out of scope for the top-level report in v1.

#### Collector outcomes

Each top-level unit has one outcome:

* `success` — exit `0`, no errors reported.
* `skipped` — intentionally not applicable (not a failure).
* `degraded` — some output produced, but non-fatal errors or incomplete sub-operations.
* `error` — unsuccessful termination, unexpected non-zero exit, or fatal reported error.

Original process exit codes are always retained. Status normalization is in `status.json` only; collector exit codes are **not changed in v1**.

Normalization:

* `skipped` — explicit `report_skip`, or wrapper maps known skip patterns (e.g. `get_operator_ns` INFO + exit `0`).
* `degraded` — `report_error` severity `warning` (default), or bounded heuristics (e.g. non-empty `monitoring/**/*.stderr`), and exit `0`.
* `error` — non-zero exit without skip mapping, or `report_error` severity `error`.
* Every scheduled unit gets a wrapper entry; no `unknown` state.

#### Overall outcomes and exit semantics

Overall status is separate from per-collector outcomes:

* `success` — all units succeeded or were intentionally skipped.
* `degraded` — archive and report finalized, but one or more units reported `degraded` or `error`.
* `error` — framework could not init/finalize the report, archive could not be finalized, or timeout/cancellation prevented a trustworthy report.

| Scenario | `overallStatus` | `gather` / Job exit |
|----------|-----------------|---------------------|
| All success or skipped | `success` | `0` |
| Archive finalized, any unit `degraded` or `error` | `degraded` | `0` (report is the completeness signal) |
| Timeout/cancellation (`interrupted: true`) | `error` | non-zero |
| Framework failure, archive not finalized | `error` | non-zero |

No v1 "required collectors" list. Collector failures on a finalized archive are always `overallStatus: degraded`, preserving best-effort collection and existing e2e exit-0 expectations.

**Exception:** When `interrupted: true`, overall status is always `error` even if a partial archive exists. Phase 2 maps that to `CollectionFrameworkError`.

On timeout/cancellation, write a partial report when possible. Include an entry for **every scheduled unit**. Finished units keep normal outcomes; launched-but-incomplete units get `status: error`, `exitCode: null`, code `INTERRUPTED`. Set `overallStatus: error`, `completedAt`, `interrupted: true`.

#### Status report format

`schemaVersion: 1` (field names illustrative until owner review). Allowed values: `overallStatus` ∈ {`success`,`degraded`,`error`}; collector `status` ∈ {`success`,`skipped`,`degraded`,`error`}; message `severity` ∈ {`warning`,`error`}; framework step `status` ∈ {`success`,`skipped`,`error`}.

Example — degraded archive (`gather_etcd` failed; `compress_logs` not enabled):

~~~json
{
  "schemaVersion": 1,
  "mustGatherVersion": "4.18.0-202609170001",
  "startedAt": "2026-09-17T09:55:00Z",
  "completedAt": "2026-09-17T09:58:42Z",
  "interrupted": false,
  "overallStatus": "degraded",
  "collectors": [
    {
      "name": "gather_etcd",
      "status": "error",
      "exitCode": 1,
      "startedAt": "2026-09-17T09:55:01Z",
      "completedAt": "2026-09-17T09:55:03Z",
      "durationSeconds": 2,
      "messages": [
        {
          "severity": "error",
          "code": "ETCD_NO_RUNNING_POD",
          "message": "No running etcd pods found in namespace openshift-etcd"
        }
      ],
      "artifacts": ["etcd_info"]
    }
  ],
  "framework": {
    "compress_logs": {
      "status": "skipped",
      "exitCode": null
    }
  }
}
~~~

Collectors write private fragments under `/must-gather/.status/`; the parent merges and atomically renames to `/must-gather/status.json`. No unsynchronized concurrent writes to one shared JSON document.

Messages: max 10 per unit, 500 characters each, no secrets/tokens; stable `code` for automation. Point to archive-relative artifacts rather than copying unbounded command output. Align redaction with [must-gather-clean](https://github.com/openshift/must-gather-clean).

#### Operator status (Phase 2)

Preserve Job-based behavior as fallback; add report-derived data when `status.json` is available.

**Layer 1 — Job lifecycle conditions:** `Pending`, `Running`, `Gathering`, `Uploading`, `Completed`, `Failed` with standard condition fields. Sources: Job/Pod phase, `terminated.reason`, `exitCode`, events (`OOMKilled`, `DeadlineExceeded`, `ImagePullBackOff`).

**Layer 2 — Collection quality:**

~~~yaml
status:
  conditions:
    - type: Completed
      status: "True"
      reason: CollectionDegraded
      message: "Archive available; 2 collectors reported errors"
      lastTransitionTime: "2026-09-17T10:00:00Z"
  collectionReport:
    schemaVersion: 1
    reportAvailable: true
    overallStatus: degraded
    counts:
      success: 20
      skipped: 5
      degraded: 1
      error: 0
    affectedCollectors:
      - name: gather_monitoring
        status: degraded
        reason: "Prometheus collection was incomplete"
~~~

| Job outcome | Report | CR result |
|-------------|--------|-----------|
| Job Failed (OOM, timeout, image pull) | N/A or partial | `Failed` with Pod/Job reason — do not wait for `status.json` |
| Job Succeeded, no report | — | `Completed` + `reportAvailable: false` |
| Job Succeeded, report `success` | present | `Completed` |
| Job Succeeded, report `degraded` | present | `Completed` + `CollectionDegraded` |
| Job Succeeded, report `error` (framework) | present | `Completed` + `CollectionFrameworkError` |
| Report parse fails | invalid | `Completed` + `reportAvailable: false` + parse error in message |

Detailed per-collector messages remain in the archive. CR status holds bounded, non-sensitive summary only. Read after Job container terminates and before cleanup/upload.

### API Extensions (Phase 2)

Phase 1 does not modify any Kubernetes API. Phase 2 adds bounded informational `collectionReport` to `MustGather` status (no webhooks, finalizers, or aggregated API servers). API approver reviews: collector list placement, stable names/values, size limits/redaction, condition/reason strings, unknown schema versions.

### Topology Considerations

Report is generated inside the same gather execution; no topology-specific config.

* **Hypershift / HCP:** Identify management vs guest collection context. No new management-cluster service. Test operator Job/volume topology before Phase 2 completion.
* **Standalone:** Non-applicable collectors report `skipped`, not `error`.
* **SNO / MicroShift:** No new long-running workload; reporting records outcomes only.
* **OKE:** Archive report does not depend on OKE-excluded features; verify operator path against OKE-supported workflow.

### Implementation Details/Notes/Constraints

#### Core reporting helpers (Phase 1)

`status_reporting.sh` provides:

* `report_error <code> <message> [severity]` — `warning` (default, → `degraded` if exit `0`) or `error` (fatal → collector `error`).
* `report_skip <code> <message>` — intentional non-applicability.
* Init/finalize of per-collector result data; normalize exits into four outcomes; atomic report generation and validation.

Reporting failures must not block unrelated collection. The parent must distinguish "collector data failed" from "status report could not be finalized."

#### Parent orchestration (Phase 1)

Wrapper assigns stable names, creates `/must-gather/.status/<name>/`, captures exit code and wrapper-visible stderr (without replacing collector artifacts), waits per PID, merges events, includes `gather_aro` and `compress_logs`, and continues after collector failures.

The wrapper cannot see failures suppressed inside a collector unless that collector reports explicitly or a known heuristic applies. Phase 1 provides baseline top-level coverage, not complete command-level coverage.

**Responsibility split:** Collectors detect absent targets (`report_skip`), internal failures (`report_error`), and nested child failures. Parent captures top-level exits, applies heuristics for uninstrumented collectors, merges `status.json`, computes `overallStatus`. Operator (Phase 2) maps report → CR.

#### Path normalization (Phase 1)

Standardize on `/must-gather/` for the report (alongside `/must-gather/version`) and normalize inconsistent collector output paths where needed.

#### Phase 1 instrumentation

Phase 1 instruments collectors so `status.json` is accurate beyond the wrapper’s top-level exit codes:

* Fix `get_operator_ns` and silent-skip collectors to call `report_skip`.
* Replace error-swallowing and nested group-`wait` patterns with `report_error` / per-PID aggregation (priority: monitoring, etcd, ppc, network logs, and other high-traffic collectors).
* Add structured messages where exits already propagate.
* `gather_istio`: basic skip detection in v1; full instrumentation deferred.

Per-command coverage inside every collector is not required in v1. Known gaps (inspect sub-failures, `2>/dev/null`, hangs) remain documented as follow-up.

#### Error message guidelines

Actionable for re-run decisions: what happened, which resource/namespace (no secrets), whether re-run might help, archive artifact pointer. Bounds: 500 chars, max 10/unit. Codes: `UPPER_SNAKE_CASE` prefixed by area (`ETCD_`, `MONITORING_`), registered beside `status_reporting.sh`.

#### Archive and custom-image compatibility

`status.json` is additive at `/must-gather/status.json`. Diagnostic layout unchanged. Consumers must tolerate: absent report (older/custom image); older supported schema; unknown additive fields. Unsupported major `schemaVersion` → treat as unavailable, not success.

Custom images (`--image`, `--all-images`) are **recommended but not required** to produce `status.json`. Phase 1 changes only the default image entrypoint/collectors/helper — no new env, mounts, or entrypoint contract for custom images. Missing report means the contract was not adopted, not that collection failed (Resolved Decision #8). Phase 2: Job lifecycle still works; `reportAvailable: false`; no cross-image aggregation in v1. Optional adopt path: vendor `status_reporting.sh`, use `schemaVersion: 1`.

#### Operator transport and lifecycle (Phase 2)

Read report from existing gather output before cleanup/upload. Parsing must not be a prerequisite for preserving the archive; on parse failure, record unavailability and fall back to Job result.

#### Performance and resource use

Bounded per-collector metadata under `/must-gather/.status/` only. Must not serialize currently concurrent collectors or add a new API request per resource. Stderr capture must be size-limited.

### Risks and Mitigations

* *False positives from expected conditions.* Explicit `skipped`; review skip reasons with support.
* *Hidden failures remain hidden.* Instrument known suppression; document coverage boundary.
* *Concurrent report corruption.* Per-collector files + atomic final rename.
* *Sensitive information in messages.* Redact, bound length/count; keep raw detail in the archive, not CR status.
* *Callers relying on exit `0`.* Retain exit codes; normalize in JSON; test best-effort completion.
* *Operator/image skew.* Optional versioned report; Job fallback.
* *Status API growth.* Bounded CR fields; API review before Phase 2.
* *Inconsistent tooling interpretation.* Publish meanings; archive report is detailed source of truth.

Security and UX/support review: must-gather maintainers, operator maintainers, support-facing diagnostic owners.

### Drawbacks

* Framework complexity and a long-lived reporting contract.
* Collectors need changes to distinguish skips from success and stop swallowing errors.
* Support may see more reported problems as formerly silent failures become visible.
* Phase 2 adds status API compatibility and cross-repo coordination.
* Schema/fields need ongoing documentation when collectors are added or renamed.

## Alternatives (Not Implemented)

* *Only aggregate `wait`.* Does not identify the failing collector or expose hidden failures.
* *Wrapper-only around top-level exits.* Useful baseline but misses `|| true`, redirected stderr, nested waits — retained as one layer, not the full solution.
* *Parse the archive after collection.* Delayed, unreliable for legitimately empty output.
* *Any collector failure fails the whole gather.* Conflicts with best-effort purpose and e2e exit-0 expectations.
* *Human-readable logs only.* Hard for automation; no stable per-collector contract.
* *New service or telemetry backend.* Higher cost; unhelpful in disconnected environments.
* *Job exit code alone for operator status.* Cannot distinguish successful Job with silent collector failures from full success.

## Resolved Decisions

| # | Decision |
|---|----------|
| 1 | One enhancement, **two phases**: Phase 1 = must-gather; Phase 2 = must-gather-operator (blocked on Phase 1 schema stability). |
| 2 | Overall `degraded` (not `error`) when the report is finalized with collector failures. No required-collector list in v1. |
| 3 | `gather` and Job exit `0` when the archive is finalized; the report carries completeness truth. |
| 4 | On timeout/cancellation, partial report with `interrupted: true` and `overallStatus: error`. |
| 5 | No `unknown` state — every scheduled unit gets a wrapper entry. |
| 6 | CR status: bounded `collectionReport` + standard K8s conditions. Final field names require Phase 2 API review. |
| 7 | Operator reads `status.json` after Job container termination and before cleanup/upload. |
| 8 | `status.json` recommended for custom images but not required; missing report → Job-only fallback. |
| 9 | Message limits: max 10/unit, 500 chars, stable `code`, no secrets/tokens in CR status. |
| 10 | Prior analysis spikes are closed. This enhancement is the authoritative design. |
| 11 | Ships as **GA** (no Dev Preview / Tech Preview). Phase 1 and Phase 2 are delivery milestones on the same GA track. |

## Test Plan

### Phase 1 — must-gather

* Unit tests for `status_reporting.sh`: success, skip, degraded, error, repeated events, message limits, redaction, atomic finalization.
* Wrapper tests with fake collectors: success; error exit; `report_skip` + exit `0`; report error + continue exit `0`; stderr; concurrent finish; fail before output; nested children; interrupt/timeout.
* One failed collector does not prevent unrelated collectors.
* Final JSON valid with all 28 top-level units (including post-wait steps).
* Original exit codes and bounded messages retained.
* `gather` exits `0` when a collector fails but the archive finalizes (e2e contract regression).
* Regression tests for known suppression patterns (e.g. monitoring).
* Archive-level tests for present/missing/older-schema reports.
* Existing formatting, shell lint, and test targets.

### Phase 2 — must-gather-operator

* Controller tests: success, degraded, failed, timeout, missing report, parse failure, unsupported schema.
* Job Failed without `status.json` (OOMKilled, DeadlineExceeded, ImagePullBackOff).
* Parse failure does not prevent archive preservation; read-before-cleanup ordering.
* Version skew: new image + old operator; old image + new operator.
* Integration/e2e for operator-managed gather with controlled collector failure.
* Operator generation/validation when the API changes.

Expand with managed-service requirements when targeted to a release.

## Graduation Criteria

Ships as **GA** (no Dev Preview / Tech Preview). Phase 1 and Phase 2 are delivery milestones on the same GA track. Initially `provisional`.

### Provisional -> Implementable

* Consensus on schema, status meanings, exit behavior, compatibility fallback.
* Owners identified (must-gather, operator, support, API).
* Clear which failures need wrapper vs collector instrumentation.
* Security and redaction rules agreed; Phase 1/2 boundaries agreed.

### GA readiness — Phase 1

* Default image emits valid report for all 28 top-level units.
* Report preserved for success, degraded, and collector-error runs where finalization succeeds.
* Tests cover suppressed-error, skip, nested-process, and timeout cases.
* Support docs explain interpretation and re-run decisions.
* Schema v1 frozen.

### GA readiness — Phase 2

* Job lifecycle conditions as specified; report-present/absent/unsupported-schema without losing archives.
* E2e for operator-managed workflow; bounded CR size and reviewed redaction.
* High-volume suppression patterns instrumented or documented as out of coverage.
* User-facing docs; skew tests across supported combinations.
* Feedback from support/incident archive consumers.

### Removing a deprecated feature

No deprecations. `status.json` is additive. Archive contents, collector behavior, and Job exit semantics remain unchanged.

## Upgrade / Downgrade Strategy

Additive archive change; no cluster migration. Newer operator + older image → Job fallback. Older operator + newer image → report is an unused archive artifact. Downgrade drops the report from new archives only. Phase 2 status fields must be optional. No persistent data migration.

## Version Skew Strategy

`schemaVersion` is independent of image build version.

* Accept current schema; ignore additive unknown fields.
* Unsupported major version → unavailable + Job fallback; never treat as success.
* Older operator must not block archive delivery for a newer report.
* Newer operator must not fabricate per-collector outcomes for an older image without a report.
* Stable collector names; renames need compatibility notes.
* Tests include older-image/newer-operator and newer-image/older-operator.

## Operational Aspects of API Extensions (Phase 2)

Status-only change: no admission, conversion webhook, aggregated API, finalizer, or synchronous dependency for ordinary operations. Update summary when the Job reaches terminal state (not per collector). Bounded serialized size. Clear reasons for: report success; degraded; framework error; unavailable (old/custom); unreadable/incompatible. Failure mode: graceful degradation to Job-based status without blocking archive retrieval.

## Support Procedures

Check `/must-gather/status.json` before diving into collector directories.

* `success` — normal inspection.
* `degraded` — review affected collectors/messages; re-run only if recoverable or required for the case.
* Collector `skipped` — confirm target not expected; not a collection failure.
* Collector `error` — inspect artifacts and messages; report signature to owners if reproducible.
* Overall `error` — determine timeout, cancellation, report-generation, or finalization failure; use gather log if report absent/incomplete.
* Missing report — older/custom/unsupported image or failed before init; do not infer all collectors succeeded.

Phase 2 exposes the same aggregate in CR status; archive remains the detailed source. Include examples for etcd, monitoring, and platform-not-applicable. No disable switch for archive-only path; ignoring Phase 2 fields must leave Job observation and archive retrieval working.

## Infrastructure Needed [optional]

No new production infrastructure. Existing image, archive output, and operator Job lifecycle suffice: shell unit/integration tests and fake collectors (Phase 1); archive validation (Phase 1); operator controller tests and manifest validation (Phase 2). Fleet-wide reporting/metrics would be a separate proposal.
