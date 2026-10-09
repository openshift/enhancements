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
last-updated: 2026-09-30
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/MG-376
---

# Must-Gather Error Status Reporting

## Summary

This enhancement adds machine-readable error status reporting to `must-gather` by producing a versioned `/must-gather/status.json` file that records per-collector success, skip, degraded, or error outcomes with bounded messages, delivered in two phases: Phase 1 (must-gather repository) establishes the archive-level `status.json` contract usable immediately via `oc adm must-gather`, and Phase 2 (must-gather-operator repository) propagates Job lifecycle conditions and `status.json` results to the `MustGather` CR while preserving backward compatibility with older or custom images through a documented Job-based status fallback.

## Motivation

`must-gather` is intended to collect as much diagnostic information as possible even when individual collectors cannot complete. Today, the result of the overall command does not reliably communicate which collectors failed, which collectors were not applicable, or which collectors produced only partial output. A support engineer can receive an archive that appears successful while important directories are empty or incomplete.

The current orchestration starts multiple collectors in the background and waits for them as a group. The parent `gather` script calls `wait "${pids[@]}"`, which returns only the **last** waited PID's exit status; other collector failures are lost. The script ends with `sync` and effectively always exits `0`. Individual exit statuses and failure messages are not consistently surfaced to the user or to automation. Several collectors also intentionally suppress errors or use exit code `0` for an expected skip, making success, skip, and partial failure indistinguishable.

The Support Log Gather Operator must also propagate Job execution lifecycle and failure reasons (OOMKilled, timeout, image pull errors) to the `MustGather` CR. That Job-layer visibility is independent of collector-level reporting and is addressed in Phase 2.

### Current state of error signals

An audit of every collector script reveals that the vast majority do not produce catchable error signals today. Even if the parent wrapper is fixed to wait per-PID, most collectors suppress, swallow, or misclassify their own failures. This section documents the current state as the baseline that this enhancement must fix.

#### Parent orchestration problems

The parent `gather` script has two fundamental problems:

1. **Group wait loses exit codes.** `wait "${pids[@]}"` (line 185) returns only the exit status of the last PID in the list. If collectors 1-27 fail and collector 28 succeeds, the parent sees exit `0`.
2. **No exit code capture.** The parent does not store or inspect any collector's exit code. After `wait`, it runs `gather_aro` synchronously (line 189) and ends with `sync` (line 198). The overall gather process effectively always exits `0`.

#### Collector error signal audit

Out of 24 collector scripts, only 6 produce a usable non-zero exit code on failure. The remaining 18 always exit `0` regardless of what happened.

**Collectors that exit non-zero on failure (6 of 24):**

| Collector | Failure | Signal | Useful message? |
|-----------|---------|--------|-----------------|
| `gather_etcd` | No running etcd pod | `exit 1` | Yes: `"ERROR: No running etcd pods found"` to stderr |
| `gather_priority_and_fairness` | No kube-apiserver pods | `exit 1` | Yes: `"ERROR: No running kube-apiserver pods found"` to stderr |
| `gather_kas_startup_termination_logs` | Any `oc adm node-logs` or pipeline failure | Non-zero via `set -o errexit/pipefail` (lines 4–6) | No: errexit aborts script, raw `oc` error only |
| `gather_insights` | No running insights pod, `oc rsync` fails | Non-zero (unhandled crash) | No: raw `oc` error only, no collector-level message |
| `gather_olm` | `oc adm inspect` fails | Non-zero (last command's exit code propagates) | No: raw `oc` error only |
| `gather_podnetworkconnectivitycheck` | `oc get` fails (API unavailable, resource not found) | Non-zero (last command's exit code propagates) | No: raw `oc` error only |

**Collectors that always exit `0` on skip (silent, no distinguishing signal) (11 of 24):**

| Collector | Skip condition | Current code | Problem |
|-----------|---------------|-------------|---------|
| `gather_vsphere` | CSI driver absent | `exit 0`, no message | Skip indistinguishable from success |
| `gather_aro` | Not ARO cluster | `exit 0`, no message | Silent |
| `gather_osus` | No OSUS operator | `exit 0`, no message | Silent |
| `gather_windows_node_logs` | No Windows nodes | `exit 0`, no message | Silent |
| `gather_frrk8s` | Namespace absent | `exit` (bare, = 0), INFO to stdout | INFO mixed in stdout |
| `gather_olm_v1` | OLM v1 CRDs absent | `exit 0`, INFO to stdout | INFO mixed in stdout |
| `gather_priority_and_fairness` | Cluster version < 4.6 | `exit 0`, INFO to stdout | Mixed in stdout |
| `gather_metallb` | Operator absent via `get_operator_ns` | `exit 0`, INFO to stdout | `get_operator_ns` prints INFO then exits 0. Note: `gather_metallb` also appears in the error-swallowing table below for the distinct case where the operator IS present but the MetalLB CR is missing. |
| `gather_nmstate` | Operator absent via `get_operator_ns` | `exit 0`, INFO to stdout | Same |
| `gather_nfd` | Operator absent via `get_operator_ns` | `exit 0`, INFO to stdout | Same |
| `gather_sriov` | SR-IOV operator absent via `get_operator_ns` | `exit 0`, INFO to stdout | Same — `get_operator_ns "sriov-network-operator"` on line 5 |

**Collectors that always exit `0` after swallowing real errors (7 of 24):**

| Collector | Error condition | Suppression pattern | Problem |
|-----------|----------------|--------------------| --------|
| `gather_monitoring` | Any `prom_get` or `alertmanager_get` call fails | `\|\| true` on every call (8 calls) | Always exits `0`. Errors go to per-call `.stderr` files in the archive, but nothing reads them. |
| `gather_ppc` | Node gather fails (NTO image not found, daemonset timeout, per-node gather fails) | Errors logged to stdout, function uses `return`, script ends with `exit 0` | `exit 0` always. Multiple ERROR messages in stdout but no structured signal. |
| `gather_metallb` | MetalLB CR missing after partial inspect | `exit 0` (line 23) with message `"metallb not started"` to stdout | Partial collection looks like success. |
| `gather_network_logs_basics` | Resource inspect fails | `\|\| true` on every `oc adm inspect` call (line 18) | Always exits `0`. No artifact to detect failures. |
| `gather_olm_v1` | Individual CRD inspect fails | `\|\| { echo "WARNING: ..."; }` (line 36-38), continues loop, exits `0` | Warning in stdout only. |
| `gather_sriov` | `oc exec` commands to config-daemon pods fail | No error handling on any `oc exec` call; `2>/dev/null` on some (line 115) | Failures silently produce empty files. |
| `gather_istio` | Any `oc exec`, `oc get`, or `inspect` call fails inside the main loop | No error handling; `main` ends with `echo` so exit code is always `0`; `2>&1` redirects stderr into output files | Complex script (223 lines) with multiple unguarded `oc exec` calls in `getSynchronization()` and `getEnvoyConfigForPodsInNamespace()`. Intermediate failures produce truncated or empty files. No skip detection when Istio CRDs are absent — the script runs to completion collecting nothing. |

**Collectors with nested `wait` that lose child exit codes (8 of 24):**

| Collector | Children | Wait pattern | Lost |
|-----------|----------|-------------|------|
| `gather_etcd` | 5 parallel `ocp4etcdctl` PIDs | `wait "${PIDS[@]}"` (line 69) after `set +o errexit` (line 39) | 4 of 5 exit codes. Top-level exit may be 0 even if 4 sub-commands failed. |
| `gather_frrk8s` | Per-pod `gather_frr_logs` + reloader PIDs | `wait ${PIDS[@]}` (line 50) | All but last. |
| `gather_service_logs` | Per-service `collect_service_logs` PIDs | `wait "${PIDS[@]}"` (line 36) | All but last. |
| `gather_windows_node_logs` | Per-log-file `oc adm node-logs` PIDs | `wait ${PIDS[@]}` (line 31) | All but last. |
| `gather_sriov` | 9+ parallel `oc exec` / `oc cp` PIDs per pod | `wait "${PIDS[@]}"` (line 134) | All but last. |
| `gather_haproxy_config` | Per-pod `oc cp` of haproxy config files | `wait "${PIDS[@]}"` (line 24) | All but last. `echo` after `wait` forces exit `0`. |
| `gather_machineconfig_ondisk` | Per-degraded-node `oc cp` of MachineConfig files | `wait "${PIDS[@]}"` (line 29) | All but last. Additionally, two `oc cp` commands run per node (lines 18–19) but only the second PID is captured — the first is fire-and-forget. |
| `gather_machineconfigdaemon_termination_logs` | Per-node `oc cp` of MCD previous-logs | `wait "${PIDS[@]}"` (line 33) | All but last. Also has per-node silent skip with `INFO` to stdout when `previous-logs` folder absent (line 23) and `2>/dev/null` on the folder-existence check (line 17). |

**The `.stderr` artifact exception:**

`gather_monitoring` is the only collector that writes per-call `.stderr` files (for example `monitoring/prometheus/alertmanagers.stderr`). These are in the archive but nobody reads them programmatically. The wrapper can use non-empty `.stderr` files as a heuristic for `degraded`, but this only applies to this one collector.

#### Consequence

Without fixing the collectors themselves, a per-PID wrapper in the parent `gather` script would correctly detect failures from only 6 of 24 collectors. The remaining 18 would report `success` even when they skipped, partially failed, or completely swallowed errors. This is why Phase 1 of this enhancement must include **both** the wrapper infrastructure **and** collector-level instrumentation as a single deliverable.

### User Stories

* As a support engineer, I want to see which `must-gather` collectors succeeded, failed, or were skipped before inspecting the archive, so that I can understand the available diagnostic evidence immediately.

* As an SRE or on-call engineer, I want to know whether a failed collection is worth re-running, so that I can avoid blind repeated gathers and focus on the cluster condition that prevented collection.

* As a `must-gather` maintainer, I want collector failures to be recorded in a consistent format, so that recurring fragile collectors can be identified and fixed using production and CI evidence.

* As an automation or operator author, I want a machine-readable collection result, so that automation can distinguish a completed archive from an archive that completed only partially.

* As a cluster administrator using the Support Log Gather Operator, I want the `MustGather` CR to reflect Job lifecycle state and collection quality without manual pod triage.

* As a release and support stakeholder, I want the reporting behavior to be versioned, tested, and documented, so that it can be operated consistently across releases and during image/operator version skew.

### Goals

* Produce a valid, versioned `status.json` file at `/must-gather/status.json` in every successfully finalized default `must-gather` archive (Phase 1).
* Report a status for every top-level work unit launched by the main gather workflow (28 units; see below).
* Distinguish successful collection, expected non-applicability, partial/degraded collection, and collector failure.
* Preserve best-effort collection: one collector failure must not prevent unrelated collectors from running or prevent the archive from being finalized when finalization is possible.
* Capture non-zero top-level collector exits automatically and allow collectors to add richer error and skip messages explicitly.
* Aggregate nested collector failures where a collector launches its own child processes.
* Make the report useful to both humans inspecting an archive and automation consuming the archive or `MustGather` status.
* Keep older consumers and older/custom images usable when the report is absent or uses an older compatible schema.
* Provide tests and support procedures for interpreting the report (Phase 1).
* Propagate Job/Pod lifecycle conditions to `MustGather` CR to provide real-time Job state visibility (Phase 2).
* Consume `status.json` in the operator to expose bounded collection quality in `MustGather` status (Phase 2).

### Non-Goals

* Redesigning the diagnostic data collected by existing collectors.
* Making an expected absence of an optional operator, API, platform, or resource an error.
* Failing fast at the first collector error and abandoning the remaining collection work.
* Guaranteeing that every individual `oc` command inside every collector is represented as a separate status entry in the first implementation.
* Copying unrestricted raw collector `stderr` into a Kubernetes API object.
* Adding a remote reporting service, telemetry backend, or new always-running control-plane component.
* Treating the absence of `status.json` in an older or custom image as proof that collection failed.
* Changing the numeric process exit codes returned by collectors in v1. Collectors that currently exit `0` on skip or error will continue to exit `0`; status normalization happens in `status.json` only. Collectors will gain new `report_skip` / `report_error` calls to produce structured signals, but their exit codes are preserved for compatibility with callers that depend on current behavior.

## Proposal

The proposal introduces a collection-status contract with two complementary mechanisms:

1. A common reporting helper lets collectors explicitly report expected skips and meaningful errors at the point where they are detected.
2. A parent-level wrapper records the result of every top-level work unit, including collectors that have not yet adopted explicit reporting.

The parent workflow writes a versioned `status.json` report after all collectors and post-collection steps have either completed or reached a terminal condition. The report becomes the authoritative description of collection completeness when it is present. The archive remains best effort: collector failures are recorded and collection continues, while framework failures that prevent report finalization are distinguished from collector-level failures.

Phase 2 operator integration consumes the same contract. The full detailed report remains in the archive; the `MustGather` resource exposes a bounded aggregate and collector summary suitable for status inspection and automation.

### Two status layers

The operator status integration and this enhancement address two distinct problems. Both are required for complete visibility; neither replaces the other.

| Layer | What it reports | Source | Phase |
|-------|-----------------|--------|-------|
| **Job/Pod layer** | Job phase, container termination, OOMKilled, timeout, image pull errors | Job/Pod status and events | Phase 2 (operator) |
| **Collector layer** | Per-collector success, skip, degraded, error; overall archive completeness | `/must-gather/status.json` | Phase 1 (must-gather) + Phase 2 consumption |

#### Operator status requirements

| Requirement | Layer | Phase 1 | Phase 2 |
|-------------|-------|---------|---------|
| CR reflects real-time Job state (initialized, running, gathering, uploading, finished, failed) | Job/Pod | — | Job/Pod condition sync |
| Failed Job surfaces clear error details without manual pod triage | Job/Pod | — | Termination reason, exit code, events in conditions |
| Standard K8s condition conventions (`type`, `status`, `reason`, `message`, `lastTransitionTime`) | Job/Pod + collection | — | Conditions for lifecycle and collection quality |
| Collector failures visible when Job succeeds | Collector | `status.json` in archive | `collectionReport` in CR status |
| Unit and e2e tests for failure scenarios | Both | BATS + archive tests | Controller + integration tests |

### Implementation phases

One enhancement document covers the full design. Implementation is split into two sequential phases across two repositories. Phase 2 depends on a stable Phase 1 `status.json` schema (`schemaVersion: 1` finalized and tested in the default image).

#### Phase 1 — must-gather (this repository)

**Goal:** Every default `must-gather` archive contains a valid, versioned `/must-gather/status.json` that accurately describes collection completeness. Usable immediately by support engineers via `oc adm must-gather` without any operator changes.

| Area | Deliverables |
|------|-------------|
| Core infrastructure | `status_reporting.sh` with `report_error`, `report_skip`, init/finalize, atomic merge |
| Parent orchestration | Per-PID wrapper in `gather`; individual `wait` per work unit; cover all 28 top-level units |
| Path normalization | Standardize on `/must-gather/` for report and collector output where inconsistent today |
| Schema v1 | `status.json` with `schemaVersion`, collector outcomes, `framework` section, bounded messages |
| Collector instrumentation | Priority set: `get_operator_ns` callers, `gather_monitoring`, `gather_etcd`, `gather_ppc`, `gather_metallb`, nested-wait collectors |
| Exit semantics | `gather` exits `0` when archive finalizes; report `overallStatus` carries completeness truth |
| Tests | BATS for helpers/wrapper; fake-collector tests; archive validation; gather exit-0 regression |
| Docs | Support procedures for reading `status.json` from archives |

**Phase 1 incremental delivery (3 PRs):**

Phase 1 is delivered as three incremental pull requests. Each PR can be reviewed, tested, and merged independently. Later PRs depend on earlier PRs.

| PR | Scope | Contents | Risk |
|----|-------|----------|------|
| **PR 1: Infrastructure** | Framework and orchestration | Add `status_reporting.sh` (`report_error`, `report_skip`, `status_init`, `status_finalize`, atomic merge). Refactor `gather` to per-PID wait loop (replace group `wait`). Generate `status.json` with `schemaVersion: 1`. Add BATS unit tests for helpers. Clean up dead code (`resources+=()` on `gather` line 50). | Low — additive, no collector behavior changes. |
| **PR 2: Skip instrumentation** | Silent-skip visibility | Fix `get_operator_ns` to call `report_skip` (affects metallb, nmstate, nfd, sriov). Add `report_skip` to all silent-skip collectors (vsphere, aro, osus, windows, frrk8s, olm\_v1, priority\_and\_fairness, metallb CR-absent case as `report_error`). Add BATS tests for skip detection. | Low — one-line additions per collector, exit codes unchanged. |
| **PR 3: Error instrumentation** | Error-swallowing and nested-wait fixes | Fix `gather_monitoring` (remove unconditional-success suppression, add `report_error`). Fix `gather_etcd`, `gather_ppc`, `gather_network_logs_basics`, `gather_sriov`, `gather_olm_v1` error handling. Fix all nested-wait collectors (per-PID child wait). Add `gather_insights` basic error handling. Add BATS tests for error/degraded detection. | Medium — behavioral changes inside collectors, needs careful per-collector review. |

A fourth parallel effort (support documentation, error code registry, archive inspection guide) can proceed alongside PRs 2 and 3.

**Phase 1 graduation gate:** Default image emits valid `status.json` for all top-level work units on success, degraded, and collector-error runs where archive finalization succeeds. Schema v1 is frozen before Phase 2 begins.

**Out of scope for Phase 1:** `MustGather` CR changes, operator controller changes, Kubernetes API review.

#### Phase 2 — must-gather-operator (separate repository)

**Goal:** `MustGather` CR status reflects both Job execution lifecycle (to provide real-time Job state visibility) and collection quality from `status.json` when available.

| Area | Deliverables |
|------|-------------|
| Job lifecycle (Layer 1) | Standard K8s conditions: Pending, Running, Gathering, Uploading, Completed, Failed — synced from Job/Pod phase, exit codes, termination reasons |
| Report consumption (Layer 2) | Read `/must-gather/status.json` from gather output volume before cleanup/upload; parse and map to CR status |
| CR status extension | Bounded `status.collectionReport` with counts, `overallStatus`, affected collector names/reasons |
| Mapping rules | Job Failed → Failed condition regardless of report; Job Succeeded + report `degraded` → Completed with `CollectionDegraded` reason; Job Succeeded + report `error` → Completed with `CollectionFrameworkError` reason; missing/invalid report → Job-only fallback |
| Version skew | New image + old operator; old image + new operator; unsupported `schemaVersion` → graceful fallback |
| Tests | Controller unit tests; integration/e2e with operator-managed gather |
| API review | Condition types, reason strings, status field size limits, message redaction |

**Phase 2 dependency:** Requires Phase 1 `status.json` schema v1 merged and available in the default payload image (or test image). Operator development can begin against a test image while Phase 1 is in final review, but Phase 2 must not merge until the schema is stable.

**Backward compatibility during rollout:** An operator with Phase 2 changes running against an older image (no `status.json`) continues to use Job-only status. An image with Phase 1 changes running against an older operator simply adds `status.json` to the archive.

### Workflow Description

The actors are:

* The support engineer or administrator, who starts `must-gather` through `oc adm must-gather` or an operator-managed `MustGather` resource.
* The `must-gather` image, which runs the main gather workflow and collector scripts.
* The collector scripts, which collect platform, component, and resource diagnostics.
* The `must-gather-operator`, when used, which creates and observes the Job and publishes the result in the `MustGather` status.
* Automation or support tooling, which reads `status.json` from the archive or reads the aggregate `MustGather` status.

The normal workflow is:

1. The gather process creates the status-reporting workspace at `/must-gather/.status/` and records the start of the collection.
2. The main workflow starts its top-level work units. Each unit is assigned a stable name and a private result/status location under `/must-gather/.status/`.
3. Work units run in parallel where the current workflow already allows parallel collection.
4. A collector can call `report_skip` when its target is not present or not applicable. It can call `report_error` when it detects a failure while continuing to collect other data.
5. The parent waits for each work unit individually and records its original exit code, elapsed time, explicit events, and bounded diagnostic messages. It does not use `wait "${pids[@]}"` as a group.
6. Synchronous collectors and post-wait collection steps are also executed through the reporting path, so that they cannot silently fall outside the report.
7. The parent merges the per-collector results and atomically writes `/must-gather/status.json` (write to a temp file in `/must-gather/.status/`, then rename).
8. The archive is finalized and made available to the user. The user can inspect `status.json` before investigating individual directories.
9. For an operator-managed gather (Phase 2), the operator reads the report from the gather output volume before cleanup or upload and maps the aggregate result to the `MustGather` status. If the report is unavailable, the operator uses the compatibility fallback described below.

#### Top-level work units

The report must include an entry for every top-level work unit launched by the main `gather` workflow (28 units):

| # | Stable name | Type |
|---|-------------|------|
| 1 | `inspect_named_resources` | Background `oc adm inspect` (named resources) |
| 2 | `inspect_group_resources` | Background `oc adm inspect` (group resources) |
| 3 | `inspect_all_namespaces` | Background `oc adm inspect` (all-namespaces resources) |
| 4–26 | `gather_insights`, `gather_monitoring`, `gather_olm`, `gather_olm_v1`, `gather_priority_and_fairness`, `gather_etcd`, `gather_service_logs`, `gather_windows_node_logs`, `gather_haproxy_config`, `gather_kas_startup_termination_logs`, `gather_network_logs_basics`, `gather_metallb`, `gather_frrk8s`, `gather_nmstate`, `gather_nfd`, `gather_sriov`, `gather_podnetworkconnectivitycheck`, `gather_machineconfig_ondisk`, `gather_machineconfigdaemon_termination_logs`, `gather_vsphere`, `gather_ppc`, `gather_osus`, `gather_istio` | Parallel collector scripts |
| 27 | `gather_aro` | Synchronous post-wait collector |
| 28 | `compress_logs` | Post-wait framework step (when `COMPRESS_AFTER_GATHER=true`; otherwise `skipped`). Reported under `framework` in the JSON schema, not in the `collectors` array, because it is a post-processing step rather than a diagnostic collector. |

Standalone scripts not invoked by the main `gather` workflow (for example `gather_audit_logs`, `gather_core_dumps`) are out of scope for the top-level report in v1.

#### Collector outcomes

Each top-level work unit has one of the following outcomes:

* `success`: the unit completed with exit code `0` and reported no errors.
* `skipped`: the unit intentionally did not run or did not collect data because its target was not applicable. This outcome is not a failure.
* `degraded`: the unit completed and produced some output, but explicitly reported one or more non-fatal errors or incomplete sub-operations.
* `error`: the unit terminated unsuccessfully, returned an unexpected non-zero exit code, or reported a fatal error.

The original process exit code is always retained in the report. Status normalization happens in `status.json` only; collector process exit codes are **not changed in v1** to preserve compatibility with callers that depend on current skip behavior (for example `exit 0` from `get_operator_ns` when an operator is absent).

Normalization rules:

* `skipped`: explicit `report_skip`, or wrapper maps known skip patterns (for example `get_operator_ns` INFO message + exit `0`).
* `degraded`: explicit `report_error` with severity `warning` (the default), or wrapper detects bounded signals (for example non-empty `monitoring/**/*.stderr` files), and exit code is `0`.
* `error`: non-zero exit without skip mapping, or explicit `report_error` with severity `error` (fatal).
* Every scheduled work unit gets a wrapper entry; there is no `unknown` state.

#### Overall outcomes and exit semantics

The report has an overall status separate from individual collector outcomes:

* `success`: all scheduled work units completed successfully or were intentionally skipped.
* `degraded`: the archive and status report were finalized, but one or more work units reported `degraded` or `error`.
* `error`: the gather framework could not initialize or finalize the report, the archive could not be finalized, or a terminal timeout/cancellation prevented a trustworthy report from being written.

| Scenario | `overallStatus` | `gather` / Job exit |
|----------|-----------------|---------------------|
| All work units success or skipped | `success` | `0` |
| Archive finalized, any work unit `degraded` or `error` | `degraded` | `0` (report is the completeness signal) |
| Framework failure, archive not finalized, timeout before trustworthy report | `error` | non-zero |

There is no v1 concept of "required collectors" that flip overall status to `error`. Collector failures on a finalized archive are always `degraded`. This preserves the upstream must-gather best-effort contract and the existing e2e expectation that gather exits successfully when the archive is produced.

On timeout or cancellation, write a partial report when possible. The partial report must still include an entry for **every scheduled work unit** (the "no `unknown` state" rule applies even under timeout). Finished units keep their normal outcomes. Work units that were launched but not yet completed when the timeout fires receive `status: error`, `exitCode: null`, and a message with code `INTERRUPTED` (for example `"Collection interrupted before completion"`). Set the report-level fields `overallStatus: error`, `completedAt`, and `interrupted: true`.

#### Status report format

The initial report schema (`schemaVersion: 1`) is as follows. Field names are illustrative until reviewed by the owners of `must-gather` and `must-gather-operator`. Allowed values: `overallStatus` is one of `success`, `degraded`, `error`; collector `status` is one of `success`, `skipped`, `degraded`, `error`; message `severity` is `warning` or `error`; framework step `status` is one of `success`, `skipped`, `error`.

The following concrete example shows a **degraded** archive where `gather_etcd` failed while the remaining collectors succeeded and `compress_logs` was not enabled:

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

The report must be valid JSON even when multiple collectors finish concurrently. Collectors write private result fragments or event files under `/must-gather/.status/`; the parent performs the final merge and atomic rename to `/must-gather/status.json`. The implementation must not depend on unsynchronized concurrent writes to one shared JSON document.

Messages must be bounded and redacted: maximum 10 messages per work unit, 500 characters per message, no secrets or tokens. Messages use a stable `code` field for automation and support runbooks. The report describes the failure and points to an archive-relative artifact when more detail is available rather than copying unbounded command output. Align redaction goals with [must-gather-clean](https://github.com/openshift/must-gather-clean).

#### Operator status (Phase 2)

When operator integration is enabled, the operator preserves existing Job-based behavior as a fallback and adds report-derived information when `status.json` is available.

**Layer 1 — Job lifecycle conditions:**

Standard Kubernetes conditions synced from Job/Pod status: `Pending`, `Running`, `Gathering`, `Uploading`, `Completed`, `Failed`. Each condition uses `type`, `status`, `reason`, `message`, and `lastTransitionTime`. Sources include Job phase, Pod phase, container `terminated.reason`, `exitCode`, and cluster events (for example `OOMKilled`, `DeadlineExceeded`, `ImagePullBackOff`).

**Layer 2 — Collection quality (`status.collectionReport`):**

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

**Mapping rules:**

| Job outcome | Report | CR result |
|-------------|--------|-----------|
| Job Failed (OOM, timeout, image pull) | N/A or partial | `Failed` condition with Pod/Job reason — do not wait for `status.json` |
| Job Succeeded, no report | — | `Completed` + `reportAvailable: false` |
| Job Succeeded, report `success` | present | `Completed` |
| Job Succeeded, report `degraded` | present | `Completed` + `CollectionDegraded` reason |
| Job Succeeded, report `error` (framework) | present | `Completed` + `CollectionFrameworkError` reason — archive may still exist but report completeness is untrustworthy |
| Report parse fails | present but invalid | `Completed` + `reportAvailable: false` + parse error in condition message |

The detailed per-collector messages remain in the archive. The CR status contains only bounded, non-sensitive summary data and the names/statuses of affected collectors. The operator reads `status.json` from the gather output volume after the Job container terminates and before cleanup or upload.

### API Extensions (Phase 2)

This proposal modifies the `MustGather` status in Phase 2 only. The `must-gather` archive contract itself is not a Kubernetes API extension.

The proposed status addition is a bounded `collectionReport` summary as shown above. The existing `MustGather` status fields and Job lifecycle semantics must remain compatible. The proposal does not add admission webhooks, conversion webhooks, aggregated API servers, or finalizers. The new status fields are informational and must not block creation, deletion, or unrelated cluster workloads.

The API approver must review:

* Whether the detailed collector list belongs in the CR status or only in the archive.
* Stable names and allowed values for normalized statuses.
* Status size limits and message redaction.
* Conditions and reason strings for both Job lifecycle and collection quality.
* Behavior when the operator observes a report schema version it does not understand.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The report is generated in the same gather execution that collects diagnostics. Hosted control-plane and guest-cluster collection must identify the collector name and target in the same way as current output. The report must make it clear which collection context produced an outcome when a gather includes management-cluster and guest-cluster data.

The enhancement does not add a new management-cluster service or change the location of existing diagnostic data. Operator integration must be tested for the Job and output-volume topology used by hosted control planes before Phase 2 is considered complete.

#### Standalone Clusters

The feature is relevant to standalone clusters and should require no additional configuration. Collectors that are not applicable to a standalone deployment report `skipped` rather than `error`.

#### Single-node Deployments or MicroShift

The status framework adds only small per-run files and bounded in-memory metadata. It does not add a long-running workload or require additional cluster resources. Single-node and MicroShift-specific collectors must preserve their existing behavior; the reporting layer records their outcomes without requiring new configuration-file options.

#### OpenShift Kubernetes Engine

The archive-level report does not depend on features excluded from OKE. Any operator status integration must be checked against the OKE-supported `MustGather` workflow. If the operator is not part of the OKE offering, the archive-level behavior remains the applicable scope.

### Implementation Details/Notes/Constraints

#### Core reporting helpers (Phase 1)

Add a shared shell helper module `status_reporting.sh` that provides:

* `report_error <code> <message> [severity]` for bounded, human-readable error events. The optional `severity` argument is `warning` (non-fatal, default when omitted) or `error` (fatal). A `warning` event marks the collector `degraded` when it exits `0`; an `error` event marks the collector `error` regardless of exit code.
* `report_skip <code> <message>` for intentional non-applicability.
* Initialization and finalization of per-collector result data.
* Normalization of exit codes into the four collector outcomes in `status.json`.
* Atomic report generation and validation.

The helpers must fail safely. A reporting failure must not prevent unrelated diagnostic collection, but the parent must distinguish "collector data failed" from "the status report itself could not be finalized."

#### Parent orchestration (Phase 1)

Refactor the main gather workflow around a small wrapper that:

* Assigns each top-level work unit a stable name from the inventory above.
* Creates an isolated result location under `/must-gather/.status/`.
* Captures the unit's original exit code.
* Captures wrapper-visible `stderr` without replacing collector-specific artifacts.
* Waits for and processes each PID independently (not `wait "${pids[@]}"` as a group).
* Merges explicit events and automatic results.
* Includes synchronous collectors (`gather_aro`) and post-wait steps (`compress_logs`).
* Continues running unrelated collectors after a collector failure.

The wrapper cannot discover failures that a collector deliberately suppresses or redirects unless that collector participates in the reporting contract. Therefore the implementation must explicitly cover known patterns such as `|| true`, orphaned `.stderr` files, and nested PID waits. Phase 1 provides baseline top-level coverage, not complete command-level coverage.

#### Path normalization (Phase 1)

Collectors today use mixed output paths (`/must-gather`, `must-gather`, `../must-gather`). Phase 1 standardizes on `/must-gather/` for the status report and normalizes collector output paths where inconsistent. The report is always written to `/must-gather/status.json` alongside the existing `/must-gather/version` file.

#### Error propagation flow

The end-to-end path for a collector error from origin to consumer is:

1. A command inside a collector fails (for example `oc exec` returns non-zero, or a resource is not found).
2. **Detection:** Either the collector calls `report_error` / `report_skip` explicitly (instrumented collector), or the parent wrapper detects the failure via non-zero exit code or stderr heuristics (uninstrumented collector).
3. **Fragment write:** The reporting helper writes a JSON fragment to the collector's private location under `/must-gather/.status/<collector_name>/`.
4. **Exit:** The collector exits. The wrapper captures the exit code, elapsed time, and any fragment events.
5. **Merge:** After all work units complete, the parent merges every fragment into `/must-gather/status.json`, computes `overallStatus`, and atomically renames the file.
6. **Archive delivery:** `oc adm must-gather` rsyncs the archive including `status.json` to the user's local filesystem. Support engineers read it directly.
7. **Operator consumption (Phase 2):** The operator reads `status.json` from the gather output volume, maps overall and per-collector status to `MustGather` CR conditions and `collectionReport` fields.

Each collector owns its own internal error detection. The parent wrapper only sees the top-level exit code and any explicit `report_error` / `report_skip` calls. Failures that a collector suppresses (for example with `|| true`) are invisible to the wrapper unless the collector is instrumented to call `report_error`, or the wrapper applies a heuristic such as detecting non-empty `.stderr` artifact files.

#### Error responsibility model

| Responsibility | Owner |
|----------------|-------|
| Detecting that a target resource, operator, or API is absent and calling `report_skip` | The collector script |
| Detecting that an internal `oc` / `curl` / `exec` command failed and calling `report_error` with a meaningful code and message | The collector script |
| Detecting that nested child processes failed (for example `gather_etcd` internal PIDs) and aggregating them | The collector script |
| Capturing the top-level exit code of each work unit | The parent wrapper |
| Applying stderr/artifact heuristics for uninstrumented collectors | The parent wrapper |
| Merging fragments into `status.json` and computing `overallStatus` | The parent wrapper |
| Mapping `status.json` to CR conditions | The operator (Phase 2) |

#### Per-collector error mapping (Phase 1 priority set)

The following table maps known failure modes in the priority collectors to the expected `status.json` outcome. This serves as the specification for the first instrumentation pass.

| Collector | Current failure mode | Current behavior | Expected `status.json` outcome | Error code |
|-----------|---------------------|------------------|-------------------------------|------------|
| `gather_etcd` | No running etcd pod | `exit 1` | `error` | `ETCD_NO_RUNNING_POD` |
| `gather_etcd` | Individual etcdctl sub-commands fail after `set +o errexit` | Silent — `wait "${PIDS[@]}"` masks failures, top-level may exit 0 | `degraded` — collector must check each child PID and call `report_error` for failures | `ETCD_SUBCOMMAND_FAILED` |
| `gather_monitoring` | Any `prom_get` / `alertmanager_get` call fails | Suppressed by `\|\| true` — always exits 0 | `degraded` — detect non-empty `.stderr` files or instrument each call | `MONITORING_PROMETHEUS_ERROR`, `MONITORING_ALERTMANAGER_ERROR` |
| `gather_monitoring` | No prometheus pods found | `PROM_PODS` array is empty, calls fail silently | `error` — no useful data collected | `MONITORING_NO_PROMETHEUS_PODS` |
| `gather_metallb` | MetalLB operator absent | `get_operator_ns` prints INFO, `exit 0` | `skipped` — `get_operator_ns` calls `report_skip` | `METALLB_OPERATOR_NOT_FOUND` |
| `gather_metallb` | Operator present but MetalLB CR missing ("metallb not started") | Runs partial inspect, then `exit 0` | `degraded` — partial data collected but MetalLB not fully running | `METALLB_CR_NOT_FOUND` |
| `gather_vsphere` | vSphere CSI driver absent | `exit 0` | `skipped` — not a vSphere platform | `VSPHERE_CSI_NOT_FOUND` |
| `gather_ppc` | Errors logged inside `ppc_nodes()` function | `return` from function, `exit 0` at end | `degraded` or `error` — collector must call `report_error` on failure instead of only logging | `PPC_NODE_GATHER_FAILED` |
| `gather_frrk8s` | Namespace `openshift-frr-k8s` absent | Bare `exit` (code 0) | `skipped` | `FRRK8S_NAMESPACE_NOT_FOUND` |
| `gather_frrk8s` | Child `gather_frr_logs` PIDs fail | `wait ${PIDS[@]}` — last-PID-only | `degraded` — check each child PID | `FRRK8S_POD_LOG_FAILED` |
| `gather_nmstate` | NMState operator absent | `get_operator_ns` prints INFO, `exit 0` | `skipped` | `NMSTATE_OPERATOR_NOT_FOUND` |
| `gather_nfd` | NFD operator absent | `get_operator_ns` prints INFO, `exit 0` | `skipped` | `NFD_OPERATOR_NOT_FOUND` |
| `gather_olm_v1` | OLM v1 CRDs absent | `exit 0` | `skipped` | `OLMV1_NOT_INSTALLED` |
| `gather_olm_v1` | Individual CRD inspect fails | Logs WARNING, continues loop | `degraded` — collector should call `report_error` per failed CRD | `OLMV1_CRD_INSPECT_FAILED` |
| `gather_service_logs` | Invalid role selector | Prints error to stderr | `error` | `SERVICE_LOGS_INVALID_ROLE` |
| `gather_service_logs` | Child `collect_service_logs` PIDs fail | `wait "${PIDS[@]}"` — last-PID-only | `degraded` — check each child PID | `SERVICE_LOGS_COLLECTION_FAILED` |
| `gather_network_logs_basics` | Network resource inspect fails | Suppressed by `\|\| true` | `degraded` — detect failures from child PIDs or stderr | `NETWORK_RESOURCE_INSPECT_FAILED` |
| `gather_insights` | Insights operator pod not found or rsync fails | Unhandled — exits non-zero | `error` | `INSIGHTS_POD_NOT_FOUND` |
| `gather_insights` | Insights operator not running (no Running pod) | Empty `INSIGHTS_OPERATOR_POD` variable, `oc rsync` fails | `error` or `skipped` depending on whether operator is expected | `INSIGHTS_OPERATOR_NOT_RUNNING` |
| `inspect_named_resources` | `oc adm inspect` fails for some resources | Non-zero exit from `oc adm inspect` | `degraded` or `error` based on exit code | `INSPECT_NAMED_FAILED` |
| `inspect_group_resources` | Same | Same | Same | `INSPECT_GROUP_FAILED` |
| `inspect_all_namespaces` | Same | Same | Same | `INSPECT_ALLNS_FAILED` |
| `gather_sriov` | SR-IOV operator absent | `get_operator_ns` prints INFO, `exit 0` | `skipped` | `SRIOV_OPERATOR_NOT_FOUND` |
| `gather_aro` | ARO-specific collection fails | Runs synchronously after `wait`, exit code not captured | `error` — wrapper must capture exit code | `ARO_COLLECTION_FAILED` |
| `gather_olm` | `oc adm inspect` fails | Non-zero exit (last command propagates) | `error` — wrapper captures exit code automatically | `OLM_INSPECT_FAILED` |
| `gather_haproxy_config` | Child `oc cp` PIDs fail | `wait "${PIDS[@]}"` — last-PID-only, `echo` after wait forces exit `0` | `degraded` — per-PID check needed | `HAPROXY_CONFIG_COPY_FAILED` |
| `gather_kas_startup_termination_logs` | `oc adm node-logs` or pipeline failure | Non-zero via `set -o errexit/pipefail` | `error` — wrapper captures exit code automatically | `KAS_LOGS_COLLECTION_FAILED` |
| `gather_machineconfig_ondisk` | Child `oc cp` PIDs fail | `wait "${PIDS[@]}"` — last-PID-only; additionally, first of two `oc cp` per node (line 18) has no PID capture at all | `degraded` — per-PID check needed, fix PID capture for both `oc cp` commands | `MACHINECONFIG_ONDISK_COPY_FAILED` |
| `gather_machineconfigdaemon_termination_logs` | Child `oc cp` PIDs fail | `wait "${PIDS[@]}"` — last-PID-only | `degraded` — per-PID check needed | `MCD_LOGS_COPY_FAILED` |
| `gather_machineconfigdaemon_termination_logs` | `previous-logs` folder absent on node | `echo "INFO: ... skipping..."`, `2>/dev/null` on folder check | Per-node skip with no structured signal — wrapper sees exit `0` | `MCD_NO_PREVIOUS_LOGS` |
| `gather_podnetworkconnectivitycheck` | `oc get` fails | Non-zero exit (last command propagates) | `error` — wrapper captures exit code automatically | `PODNETCHECK_GET_FAILED` |
| `gather_istio` | Any `oc exec`, `oc get`, or `inspect` call fails | No error handling; `main` ends with `echo` so always exits `0` | `degraded` — requires per-function error detection | `ISTIO_COLLECTION_FAILED` |
| `gather_istio` | No Istio CRDs or operator pods present | `getCRDs` returns empty, loops don't execute, exits `0` | `skipped` — implicit skip with no signal | `ISTIO_NOT_INSTALLED` |
| `compress_logs` | Target directory missing | Logs WARNING, `return 0` | `skipped` — directory not present | `COMPRESS_DIR_NOT_FOUND` |

#### Error message guidelines

Messages in `status.json` must be actionable for a support engineer deciding whether to re-run collection. Each message should follow these rules:

* **State what happened**, not just that something failed. Bad: `"Collection failed"`. Good: `"No running etcd pods found in namespace openshift-etcd"`.
* **Include the resource or namespace** involved when safe to do so (no secrets, tokens, or customer-specific identifiers).
* **Indicate whether re-running might help.** If the failure is due to a missing optional component, say so. If it is a transient timeout, note that re-running may succeed.
* **Reference the archive artifact** where detailed output is available when the message itself is truncated. For example: `"Prometheus targets collection failed; see monitoring/prometheus/active-targets.stderr for details"`.
* **Do not include raw command output.** The message is bounded to 500 characters. Point to the artifact file instead.
* **Use the `code` field** for stable machine-readable identification. Codes should be `UPPER_SNAKE_CASE`, prefixed with the collector area (for example `ETCD_`, `MONITORING_`, `METALLB_`). New codes should be documented in a registry maintained alongside `status_reporting.sh`.

#### Known v1 coverage gaps

The following failure modes are **not** detected in v1 and are explicitly out of scope for the first implementation. They are documented here so that support engineers and maintainers know the boundary.

| Gap | Why it is not covered in v1 | Mitigation |
|-----|----------------------------|------------|
| Individual `oc adm inspect` sub-resource failures inside a single `oc adm inspect` invocation | `oc adm inspect` runs as a single process; internal per-resource failures are in its own logs, not exposed via exit code | Inspect the `oc adm inspect` logs in the archive; instrument in a future version if `oc adm inspect` gains structured error output |
| Individual `oc get` / `oc exec` failures inside uninstrumented collectors | Collector does not call `report_error`; wrapper only sees top-level exit code | Track uninstrumented collectors and instrument in follow-up releases |
| Failures hidden by `2>/dev/null` redirects | Output is discarded; no artifact to detect | Audit and replace `2>/dev/null` with stderr capture in follow-up instrumentation |
| Partial data inside a successful collector (for example `gather_olm` collects 9 of 10 CRDs) | Collector does not track per-resource success; exits 0 | Requires per-resource tracking inside the collector; planned for post-v1 |
| Disk-full or write-permission errors during collection | May cause silent truncation or missing files | Framework should check available disk space at init and report an error if below a threshold |
| Timeouts within nested `oc exec` calls (for example etcdctl hangs) | Only visible if the parent process is killed by signal | Instrument `timeout` wrapper around known long-running sub-commands in follow-up |

#### Collector instrumentation (Phase 1)

This enhancement must fix the broken error signals in existing collectors as part of Phase 1 delivery. The wrapper alone cannot produce useful `status.json` output without these fixes because 18 of 24 collectors currently exit `0` on all outcomes. The following specifies the required changes per collector.

##### Parent `gather` script fixes

| Current code | Fix |
|-------------|-----|
| `wait "${pids[@]}"` (line 185) — group wait, only last exit code | Replace with per-PID loop: `for pid in "${pids[@]}"; do wait "$pid"; capture exit code and map to collector name; done` |
| `gather_aro` (line 189) — synchronous, exit code not captured | Wrap in the same per-PID reporting path |
| `compress_logs` (line 194) — return value not captured | Wrap in reporting path; report `skipped` when `COMPRESS_AFTER_GATHER` is not set |
| No overall exit code logic | After all collectors: compute `overallStatus` from merged results. Exit `0` if archive finalized; exit non-zero only on framework failure. |

##### `common.sh` / `get_operator_ns` fix

Current: prints INFO and `exit 0` when operator absent. This kills the calling collector process with exit `0`, indistinguishable from success.

Fix: Call `report_skip` with code `<OPERATOR>_NOT_FOUND` and message `"<operator_name> not detected. Skipping."` before `exit 0`. This affects all callers: `gather_metallb`, `gather_nmstate`, `gather_nfd`, `gather_sriov`.

##### Silent-skip collectors — add `report_skip`

Each of these collectors exits `0` with no error signal when their target is absent. Add an explicit `report_skip` call before the early exit.

| Collector | Skip condition | Required change |
|-----------|---------------|----------------|
| `gather_vsphere` | CSI driver absent (line 9-11) | Add `report_skip "VSPHERE_CSI_NOT_FOUND" "vSphere CSI driver not installed; not a vSphere platform"` before `exit 0` |
| `gather_aro` | Not ARO (line 6-8) | Add `report_skip "ARO_NOT_DETECTED" "ARO CR not found; not an ARO cluster"` before `exit 0` |
| `gather_osus` | No OSUS operator (line 9-11) | Add `report_skip "OSUS_NOT_DETECTED" "Update service operator not found"` before `exit 0` |
| `gather_windows_node_logs` | No Windows nodes (line 19-21) | Add `report_skip "NO_WINDOWS_NODES" "No Windows nodes found in cluster"` before `exit 0` |
| `gather_frrk8s` | Namespace absent (line 32-34) | Add `report_skip "FRRK8S_NAMESPACE_NOT_FOUND" "Namespace openshift-frr-k8s not found"` before `exit` |
| `gather_olm_v1` | OLM v1 CRDs absent (line 25-28) | Add `report_skip "OLMV1_NOT_INSTALLED" "No OLM v1 CRDs detected"` before `exit 0` |
| `gather_priority_and_fairness` | Cluster version < 4.6 (line 34-37) | Add `report_skip "APF_VERSION_SKIP" "Cluster version ${VERSION} < 4.6"` before `exit 0` |
| `gather_metallb` | MetalLB CR absent after partial inspect (line 21-23) | Change to `report_error "METALLB_CR_NOT_FOUND" "metallb not started; operator namespace data collected but MetalLB CR missing"` before `exit 0`. This produces `degraded` status because the namespace inspect on line 17 already ran and collected partial data. |

##### Error-swallowing collectors — replace suppression with `report_error`

These collectors actively hide failures. The `|| true` pattern, `return` from functions, and `2>/dev/null` must be replaced with proper error detection.

**`gather_monitoring`:**

Current: every `prom_get` / `alertmanager_get` call uses `|| true` (lines 107-117). Failures are written to per-call `.stderr` files but never checked.

Fix:
1. Remove `|| true` from each call.
2. Wrap each call in a check: if the call fails, call `report_error "MONITORING_PROMETHEUS_ERROR" "Failed to get ${object} from prometheus; see monitoring/prometheus/${path}.stderr"` and continue.
3. After `init()`, check if `PROM_PODS` is empty. If so, call `report_error "MONITORING_NO_PROMETHEUS_PODS" "No prometheus pods found in openshift-monitoring"` with severity `error` (fatal for this collector).
4. The collector should still exit `0` to preserve best-effort behavior. The `report_error` calls produce a `degraded` status in `status.json`.

**`gather_ppc`:**

Current: `ppc_nodes()` function logs errors to stdout and uses `return` (line 62-63). The script ends with `exit 0` (line 214) regardless.

Fix:
1. In `ppc_nodes()`, replace `return` after NTO image failure with `report_error "PPC_NTO_IMAGE_NOT_FOUND" "Failed to identify container image with node tools; node-level data will be missing"` and `return`.
2. Replace `wait "${NODE_PIDS[@]}"` (line 170) with per-PID check. For each failed PID, call `report_error "PPC_NODE_GATHER_FAILED" "Node performance data collection failed for node ${node}"`.
3. Replace `wait "${ADM_PIDS[@]}"` (line 184) with per-PID check similarly.
4. Keep `exit 0` at end — the `report_error` calls produce `degraded`.

**`gather_network_logs_basics`:**

Current: `|| true` on every `oc adm inspect` in `gather_multus_data` (line 18). No error detection anywhere.

Fix:
1. Remove `|| true` from the `oc adm inspect` calls in `gather_multus_data`.
2. Wrap each call: on failure, call `report_error "NETWORK_RESOURCE_INSPECT_FAILED" "Failed to inspect ${resource}"` and continue.
3. For the nested `wait "${PIDS[@]}"` (line 190) and `wait "${PIDSDB[@]}"` (line 184), replace with per-PID check.

**`gather_olm_v1`:**

Current: CRD inspect failures logged as WARNING (line 37), loop continues, exits `0`.

Fix: Inside the `|| { ... }` block (line 36-38), add `report_error "OLMV1_CRD_INSPECT_FAILED" "Failed to collect ${crd}"` in addition to the existing WARNING log.

**`gather_sriov`:**

Current: No error handling on any `oc exec` call. Some use `2>/dev/null` (line 115). `wait "${PIDS[@]}"` (line 134) loses all but last exit code.

Fix:
1. Replace `wait "${PIDS[@]}"` with per-PID check. For each failure, call `report_error "SRIOV_POD_DATA_FAILED" "Failed to collect data from config-daemon pod ${CONFIG_DAEMON_POD}"`.
2. Replace `2>/dev/null` on line 115 with `2>"${MULTUS_LOG_PATH}.stderr"` so failures are captured.

##### Nested-wait collectors — fix per-PID aggregation

These collectors launch child processes and lose exit codes through group `wait`. Replace `wait "${PIDS[@]}"` with per-PID iteration.

**`gather_etcd`:**

Current: `set +o errexit` (line 39), 5 parallel `ocp4etcdctl` commands, `wait "${PIDS[@]}"` (line 69) — only last PID's exit code returned.

Fix:
1. Replace `wait "${PIDS[@]}"` with:

~~~bash
etcd_failures=0
for pid in "${PIDS[@]}"; do
    if ! wait "$pid"; then
        ((etcd_failures++))
    fi
done
if [ "$etcd_failures" -gt 0 ]; then
    report_error "ETCD_SUBCOMMAND_FAILED" "${etcd_failures} of ${#PIDS[@]} etcdctl sub-commands failed"
fi
~~~

**`gather_frrk8s`:**

Current: `wait ${PIDS[@]}` (line 50) — group wait.

Fix: Replace with per-PID loop. On failure, `report_error "FRRK8S_POD_LOG_FAILED" "Failed to collect FRR logs from pod"`.

**`gather_service_logs`:**

Current: `wait "${PIDS[@]}"` (line 36) — group wait.

Fix: Replace with per-PID loop. On failure, `report_error "SERVICE_LOGS_COLLECTION_FAILED" "Failed to collect service logs for one or more services"`.

**`gather_windows_node_logs`:**

Current: `wait ${PIDS[@]}` (line 31) — group wait.

Fix: Replace with per-PID loop. On failure, `report_error "WINDOWS_NODE_LOG_FAILED" "Failed to collect one or more Windows node log files"`.

##### `gather_insights` — add basic error handling

Current: no error handling at all. If `INSIGHTS_OPERATOR_POD` is empty, `oc rsync` fails with a raw error.

Fix:
1. After line 10, check if `INSIGHTS_OPERATOR_POD` is empty. If so, call `report_skip "INSIGHTS_OPERATOR_NOT_RUNNING" "No running insights operator pod found"` and `exit 0`.
2. Wrap the `oc rsync` call in an error check: on failure, call `report_error "INSIGHTS_RSYNC_FAILED" "Failed to rsync insights data from ${INSIGHTS_OPERATOR_POD}"`.

##### Collectors with natural exit-code propagation — add `report_error` messages

These collectors already exit non-zero on failure but lack structured messages. Add `report_error` calls so `status.json` contains actionable information rather than just a bare non-zero exit code.

**`gather_olm`:**

Current: single `oc adm inspect` call (line 6). Exit code propagates naturally. No error message.

Fix: Wrap the `oc adm inspect` call in an error check. On failure, call `report_error "OLM_INSPECT_FAILED" "Failed to inspect OLM resources across all namespaces"`.

**`gather_podnetworkconnectivitycheck`:**

Current: single `oc get` call (line 7). Exit code propagates naturally. No error message.

Fix: Wrap the `oc get` call in an error check. On failure, call `report_error "PODNETCHECK_GET_FAILED" "Failed to get podnetworkconnectivitychecks from openshift-network-diagnostics"`.

**`gather_kas_startup_termination_logs`:**

Current: uses `set -o errexit/pipefail` (lines 4–6). Exits non-zero on any pipeline failure. No structured message.

Fix: This collector already has the best error behavior of any collector. Add a trap to call `report_error "KAS_LOGS_COLLECTION_FAILED" "Failed to collect kube-apiserver startup/termination logs"` on `ERR` signal, so the exit code and a message both appear in `status.json`.

##### Additional nested-wait collectors — fix per-PID aggregation

**`gather_haproxy_config`:**

Current: `wait "${PIDS[@]}"` (line 24) — group wait, then `echo` (line 25) forces exit `0`.

Fix: Replace `wait "${PIDS[@]}"` with per-PID loop. For each failure, call `report_error "HAPROXY_CONFIG_COPY_FAILED" "Failed to copy haproxy config from pod ${POD} in ingress controller ${IC}"`.

**`gather_machineconfig_ondisk`:**

Current: Two `oc cp` commands per node (lines 18–19) run in background, but only the second PID is captured by `PIDS+=($!)` on line 20. The first `oc cp` (line 18) is a fire-and-forget process — its PID is never tracked. `wait "${PIDS[@]}"` (line 29) is also a group wait.

Fix:
1. Capture both PIDs per iteration: add `PIDS+=($!)` after line 18 as well as line 19.
2. Replace `wait "${PIDS[@]}"` with per-PID loop. For each failure, call `report_error "MACHINECONFIG_ONDISK_COPY_FAILED" "Failed to copy MachineConfig data from degraded node ${NODE}"`.

**`gather_machineconfigdaemon_termination_logs`:**

Current: `wait "${PIDS[@]}"` (line 33) — group wait. Per-node skip when `previous-logs` folder absent (line 22–24) with INFO to stdout but no structured signal. `2>/dev/null` on the folder-existence check (line 17).

Fix:
1. Replace `wait "${PIDS[@]}"` with per-PID loop. For each failure, call `report_error "MCD_LOGS_COPY_FAILED" "Failed to copy MCD termination logs from node ${NODE}"`.
2. In the per-node skip branch (line 22–24), the `INFO: ... skipping...` message is acceptable — this is a per-node skip within a collector, not a whole-collector skip. No `report_skip` needed here (the collector itself is not skipped).

##### `gather_istio` — deferred to follow-up

Current: 223-line script with no error handling, no skip detection, multiple unguarded `oc exec` calls, and `main` always exits `0`. The complexity of this collector (CRD iteration, per-namespace loops, envoy config dumps, synchronization collection) makes it the highest-risk instrumentation target.

v1 plan: The parent wrapper captures the top-level exit code (always `0`). A basic skip check at the start of `main()` can detect when no Istio CRDs and no operator pods exist and call `report_skip "ISTIO_NOT_INSTALLED" "No Istio, Sail Operator, or Gateway API CRDs detected"`. Full per-function error instrumentation is deferred to a follow-up release.

##### Instrumentation priority and ordering

The collector fixes should be implemented in this order:

1. **`common.sh` / `get_operator_ns`** — affects 4 callers immediately.
2. **Silent-skip collectors** (vsphere, aro, osus, windows, frrk8s, olm_v1, priority_and_fairness, metallb) — simple one-line additions, high impact for skip visibility.
3. **`gather_etcd`** — high-value collector, nested-wait fix straightforward.
4. **`gather_monitoring`** — most complex change, highest support impact.
5. **`gather_ppc`** — multiple error points, daemonset lifecycle.
6. **`gather_network_logs_basics`** — many parallel children.
7. **`gather_insights`** — simple fix for unhandled crash.
8. **`gather_sriov`, `gather_service_logs`, `gather_windows_node_logs`, `gather_frrk8s`** — nested-wait fixes.
9. **`gather_olm_v1`** — one-line addition in existing error block.
10. **`gather_olm`, `gather_podnetworkconnectivitycheck`, `gather_kas_startup_termination_logs`** — add `report_error` messages to collectors that already have good exit codes.
11. **`gather_haproxy_config`, `gather_machineconfig_ondisk`, `gather_machineconfigdaemon_termination_logs`** — nested-wait fixes for newly audited collectors.
12. **`gather_istio`** — basic skip detection only in v1; full instrumentation deferred.

Each instrumented collector must use error codes from the per-collector mapping table. New error codes must be added to the registry in `status_reporting.sh` with a short description before use.

#### Archive compatibility

`status.json` is additive and placed at `/must-gather/status.json`. Existing diagnostic directories and file names remain unchanged. Consumers must tolerate:

* An absent report from an older image.
* An absent report from a custom image that has not adopted the contract.
* A report with an older supported schema version.
* Unknown fields added by a newer compatible report version.

A report with an unsupported major `schemaVersion` must be treated as unavailable/unknown, not silently interpreted as a successful run.

Custom images passed through `oc adm must-gather --image` and `--all-images` are **recommended but not required** to produce `status.json`. Custom images without a report continue to use Job-only fallback in Phase 2.

#### Operator transport and lifecycle (Phase 2)

The operator must obtain the report from the gather output before the output is removed or made inaccessible. The implementation should use the existing gather output path and archive/upload lifecycle rather than introducing a separate network service. The exact read point, cleanup ordering, and fallback behavior must be covered by operator tests.

The operator must not make status parsing a prerequisite for preserving the archive. If parsing fails, it should record that report parsing was unavailable and fall back to the Job result while retaining the archive and operator logs.

#### Performance and resource use

The reporting layer should add only bounded per-collector metadata and small status files under `/must-gather/.status/`. It must not serialize collectors that are currently safe to run concurrently, and it must not add a new API request per collected resource. Any stderr capture must be size-limited and should preserve the existing collector artifact files where they are already part of the archive.

### Risks and Mitigations

* *False positives from expected conditions.* Optional components and platform-specific collectors may be absent by design. Use explicit `skipped` reporting and review skip reasons with support stakeholders.

* *Hidden failures remain hidden.* A top-level wrapper cannot see failures discarded by `|| true`, redirected to unexamined files, or masked by nested waits. Replace or instrument known suppression patterns and state the coverage boundary in the support documentation.

* *Concurrent report corruption.* Multiple collectors may finish at the same time. Use per-collector files under `/must-gather/.status/` and atomically create the final report.

* *Sensitive information in messages.* Collector errors may contain resource names, namespaces, endpoints, paths, or command output. Redact known sensitive values, cap message length/count, and keep detailed raw output in the existing access-controlled archive rather than the CR status.

* *Behavior changes for scripts that rely on exit code `0`.* Retain original exit codes in the report; normalize status in JSON only. Add tests for callers that depend on best-effort completion.

* *Operator and image version skew.* Treat the report as optional, version it, ignore unknown fields, and fall back to Job-based status when the report is absent or unsupported.

* *Status API growth.* Keep detailed messages out of the CR status and enforce bounded collector/count fields. The API approver must review the serialized size and field stability before Phase 2 implementation.

* *Inconsistent interpretation by support tooling.* Publish the status meanings and examples, and make the archive-level report the source of detailed truth.

Security review should include the must-gather maintainers, operator maintainers, and the reviewers responsible for support-facing diagnostic data. UX/support review should include the teams that consume must-gather archives during customer-case investigation.

### Drawbacks

* The collection framework becomes more complex and must maintain a reporting contract in addition to collecting data.
* Some collectors will need changes to replace error suppression and distinguish expected skips from successful collection.
* Support engineers may initially see more reported problems because failures that were previously silent become visible.
* Phase 2 introduces a status API compatibility obligation and requires coordination between repositories.
* Status messages and schema fields require long-term documentation and review when collectors are added or renamed.

## Alternatives (Not Implemented)

* *Only check the aggregate `wait` result.* This is insufficient because it does not identify the failing collector, distinguish skips, or expose failures hidden inside collectors.

* *Use only a wrapper around top-level exit codes.* This provides useful baseline coverage but does not detect failures suppressed by `|| true`, redirected stderr, or nested child-process waits. It is retained as one layer, not the complete solution.

* *Parse the archive after collection.* This delays failure detection, cannot reliably infer why data is missing, and is fragile for collectors whose valid output is legitimately empty.

* *Make any collector failure fail the entire gather command.* This would provide a simple signal but conflicts with the best-effort purpose of `must-gather`, risks losing useful data from collectors that did succeed, and breaks the upstream e2e expectation that gather exits successfully when the archive is produced.

* *Write only human-readable log messages.* Logs are useful for diagnosis but are difficult for automation to consume and do not provide a stable per-collector contract.

* *Add a new service or telemetry backend.* This would increase operational cost and would not help when the gather is running in a restricted or disconnected environment. The archive is already the natural delivery mechanism for the result.

* *Rely on Job exit code alone for operator status.* This satisfies the Job-level portion of operator status integration but cannot distinguish a successful Job with silent collector failures from a fully successful collection.

## Resolved Decisions

The following questions were resolved during enhancement review:

| # | Decision |
|---|----------|
| 1 | One enhancement document, **two implementation phases**: Phase 1 = must-gather repo; Phase 2 = must-gather-operator repo (blocked on Phase 1 schema stability). |
| 2 | Overall status is `degraded` (not `error`) whenever the report is finalized with collector failures. No required-collector list in v1. |
| 3 | `gather` and the Kubernetes Job exit `0` when the archive is finalized, even if collectors failed. The report carries completeness truth. |
| 4 | On timeout/cancellation, write a partial report when possible with `interrupted: true` and `overallStatus: error`. |
| 5 | No `unknown` state — every scheduled work unit gets a wrapper entry with status inferred from exit code and explicit events. |
| 6 | CR status uses bounded `collectionReport` plus standard K8s conditions. Final field names require API review in Phase 2. |
| 7 | Operator reads `status.json` after Job container termination and before volume cleanup or upload. |
| 8 | `status.json` is recommended for custom images but not required. Custom images without a report use Job-only fallback. |
| 9 | Message limits: max 10 messages per work unit, 500 characters each, stable `code` field, no secrets/tokens in CR status. |
| 10 | Prior analysis spikes are closed. This enhancement is the authoritative design. |
| 11 | This feature ships as **GA**. There is no Dev Preview or Tech Preview maturity stage. Phase 1 and Phase 2 are implementation delivery phases within the same GA track. |

## Test Plan

### Phase 1 — must-gather

* Add unit tests for `status_reporting.sh`, including success, skip, degraded, error, repeated events, message limits, redaction, and atomic finalization.
* Add wrapper tests using fake collectors that:
  * exit successfully;
  * exit with an error;
  * call `report_skip` and exit `0`;
  * report an error but continue and exit `0`;
  * write stderr;
  * finish concurrently;
  * fail before creating output;
  * launch nested child processes; and
  * are interrupted or time out.
* Verify that one failed collector does not prevent unrelated collectors from running.
* Verify that the final JSON is valid and contains all 28 top-level work units, including synchronous post-wait steps.
* Verify that the original exit code and bounded messages are retained.
* Verify that `gather` still exits `0` when a collector fails but the archive finalizes (regression guard for upstream e2e contract).
* Add regression tests for known error-suppression patterns in monitoring and other collectors.
* Add archive-level tests that check `/must-gather/status.json` and behavior when the report is missing or uses an older schema.
* Run the repository's existing formatting, shell lint, and test targets.

### Phase 2 — must-gather-operator

* Add controller tests for successful, degraded, failed, timeout, missing-report, parse-failure, and unsupported-schema cases.
* Add controller tests for Job Failed **without** `status.json` (OOMKilled, DeadlineExceeded, ImagePullBackOff).
* Verify that report parsing does not prevent archive preservation.
* Verify read-before-cleanup ordering for `status.json`.
* Add version skew tests: new image + old operator; old image + new operator.
* Add integration/e2e test for a representative operator-managed gather with controlled collector failure.
* Run operator-repository generation and validation checks when the API changes.

The test plan should be expanded with managed-service testing requirements when the enhancement is targeted to a release.

## Graduation Criteria

This feature ships as **GA**. There is no Dev Preview or Tech Preview maturity stage. Phase 1 (must-gather) and Phase 2 (must-gather-operator) are implementation delivery milestones on the same GA track.

This proposal is initially `provisional`. The following criteria define the path to implementable and GA.

### Provisional -> Implementable

* The archive-level schema, status meanings, exit behavior, and compatibility fallback have consensus.
* Must-gather owners, operator owners, support stakeholders, and API reviewers are identified.
* The implementation plan identifies which failures are covered by the wrapper and which require collector instrumentation.
* Security and message-redaction rules are agreed.
* Phase 1 and Phase 2 scope boundaries are agreed.

### Dev Preview -> Tech Preview

N/A. This feature is not released as Dev Preview or Tech Preview.

### Tech Preview -> GA

N/A. This feature graduates directly to GA.

**GA readiness — Phase 1 (must-gather):**

* The default must-gather image emits a valid versioned report for all 28 top-level work units.
* The report is preserved for success, degraded, and collector-error runs where archive finalization succeeds.
* Representative suppressed-error, skipped-collector, nested-process, and timeout cases are covered by tests.
* Support-facing documentation explains how to interpret the report and decide whether to re-run collection.
* Schema v1 is frozen.

**GA readiness — Phase 2 (must-gather-operator):**

* Operator handles Job lifecycle conditions as defined in the operator status requirements section above.
* Operator handles report-present, report-absent, and unsupported-schema cases without losing archives.
* End-to-end tests cover the operator-managed workflow.
* The report has bounded size and reviewed redaction behavior in CR status.
* Known high-volume error-suppression patterns have been instrumented or explicitly documented as outside the coverage boundary.
* User-facing documentation is available in the appropriate OpenShift documentation set.
* Compatibility behavior is tested across supported image/operator version combinations.
* Feedback is collected from engineers who use must-gather archives in support and incident workflows.

### Removing a deprecated feature

This enhancement does not deprecate or remove any existing features. The `status.json` report is purely additive. Existing archive contents, collector behavior, and Job exit semantics remain unchanged.

## Upgrade / Downgrade Strategy

The archive-level change is additive. Upgrading to an image that supports status reporting does not require a cluster migration or modification of existing diagnostic data. New archives contain `status.json`; existing archives remain readable without it.

A newer operator must tolerate an older image that does not produce `status.json` and must fall back to the existing Job-based result. An older operator must continue to run with a newer image even if it does not consume the report; the report remains an additional archive artifact.

Downgrading to an older must-gather image removes the additional report from new archives but does not invalidate previously generated archives. If the operator status API is extended in Phase 2, the status fields must be optional and the operator must preserve compatibility with objects created before the fields existed.

No persistent data migration is required. Any generated API or CRD changes must follow the normal compatibility and downgrade requirements for the operator repository.

## Version Skew Strategy

The report is versioned independently from the image build version through `schemaVersion`.

* Consumers must accept reports with the current supported schema version and ignore additive unknown fields.
* Consumers must not interpret an unsupported major schema version as success; they should report the detailed result as unavailable and use the compatibility fallback.
* A newer image may produce a report that an older operator does not understand. The older operator must retain existing Job-based behavior and must not block archive delivery.
* A newer operator may observe an older image with no report. It must not fabricate per-collector outcomes and must identify report-derived status as unavailable.
* The operator and archive consumers must use stable collector names. Renaming a collector requires a compatibility note and documentation update.
* The test plan must include at least one older-image/newer-operator and newer-image/older-operator combination.

## Operational Aspects of API Extensions (Phase 2)

The proposed API change is status-only. It does not add an admission path, conversion webhook, aggregated API server, finalizer, or synchronous dependency for ordinary cluster operations. A report parsing failure must not block creation or deletion of unrelated resources.

The operator should update the report summary when the gather Job reaches a terminal state rather than issuing one API update per collector. The serialized status must have bounded collector count and message sizes. Detailed diagnostic content remains in the archive.

The operator should expose clear conditions or reasons for:

* report available and overall collection successful;
* report available and collection degraded;
* report available with a terminal framework error;
* report unavailable because the image is old/custom; and
* report unreadable or schema-incompatible.

The failure mode for the status extension is graceful degradation to the existing Job-based status. It must not change cluster health or prevent the user from retrieving the archive. The operator team and API approver should define the exact conditions, reason strings, and support escalation path.

## Support Procedures

Support engineers should begin by checking `/must-gather/status.json` before interpreting individual collector directories.

* For overall `success`, proceed with normal archive inspection.
* For overall `degraded`, review the affected collectors and their messages, then determine whether the missing data is relevant to the customer issue. Re-run only when the failure condition is likely recoverable or the missing collector is required for the investigation.
* For a collector `skipped`, confirm that the target component or platform is not expected in the cluster. Do not treat normal non-applicability as a collection failure.
* For a collector `error`, inspect the referenced collector artifacts and bounded messages. If the failure is reproducible, report the collector and failure signature to the owning maintainers.
* For overall `error`, determine whether the archive was finalized and whether the failure was a timeout, cancellation, report-generation failure, or archive finalization failure. Use the gather log when `status.json` is absent or incomplete.
* For a missing report, record that the image is older, custom, unsupported, or failed before report initialization. Do not infer that every collector succeeded.

Operator-managed gathers (Phase 2) should expose the same aggregate interpretation in `MustGather` status. The archive remains the source of detailed messages. Support documentation should include examples of the common etcd, monitoring, and platform-not-applicable cases.

There is no API extension to disable for the archive-only path. For the Phase 2 operator status fields, removing or ignoring the fields must leave existing Job observation and archive retrieval functional.

## Infrastructure Needed [optional]

No new production infrastructure is required. The implementation uses the existing must-gather image, archive output, and operator Job lifecycle.

The following existing development and CI capabilities are sufficient:

* shell unit/integration tests for the reporting helpers and wrapper (Phase 1);
* controlled fake collectors for failure injection (Phase 1);
* archive validation tests (Phase 1); and
* operator controller tests and generated-manifest validation (Phase 2).

If later work adds fleet-wide reporting or metrics, that work should be proposed separately with its own data-retention, privacy, and operational requirements.
