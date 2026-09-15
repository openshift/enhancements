# TODO: CodeRabbit pair review for PR #2098

PR: https://github.com/openshift/enhancements/pull/2098  
Document: `enhancements/topologies/mutable-topology.md`  
Head reviewed: `4cc6fdd673fc8974057abdd5cf31a9999aefce05`

## Pair-review protocol

For each item, we will:

1. Read the relevant document sections and, where applicable, the controller/API implementation.
2. Separate the CodeRabbit claim from the proposed fix.
3. Identify the invariant, race, or contract that must hold.
4. Ask clarifying questions where the design is ambiguous.
5. Agree on the smallest correct documentation or code change.
6. Apply the change only after agreement.
7. Validate references, formatting, tests, and the resulting diff.
8. Record the disposition and evidence below.

## Review queue

- [x] **1. Remove stale `mastersSchedulable` contract** — rejected
  - Source: CodeRabbit review-body, outside diff, around lines 260–263.
  - Claim: The mapping section describes `mastersSchedulable` as a derived/maintained field even though the new contract uses `controlPlaneTopology`, `infrastructureTopology`, and `topologyTransitionStatus`.
  - Evidence to inspect: Infrastructure API definitions and the status contract in this document.
  - Questions to resolve: Is `mastersSchedulable` intentionally out of scope, or does it remain an existing status field that must be documented?
  - Intended disposition: likely documentation fix; confirm API evidence first.
  - Evidence so far: `InfrastructureStatus` in `openshift/api/config/v1/types_infrastructure.go` contains `controlPlaneTopology`, `infrastructureTopology`, and `topologyTransitionStatus`, but no `mastersSchedulable` field. `mastersSchedulable` exists instead on `SchedulerSpec` in `types_scheduling.go`, with an API default of `false`.
  - Disposition: Rejected. This is an outside-diff documentation comment, not a regression introduced by the PR. For the supported compact transition, `mastersSchedulable` is intentionally always `true` regardless of the transition, so the existing statement is retained.
  - Status: complete — rejected by user.

- [ ] **2. Define terminal versus retryable `Error` behavior**
  - Source: CodeRabbit inline comment at line 186; repeated in the later review at line 186 and also applying to lines 212 and 376.
  - Claim: The document simultaneously describes failed preconditions as retried on the next sync and `Error` as a persistent/terminal state requiring administrator action.
  - Evidence to inspect: Workflow steps 7–9, Failure Handling, orchestration, troubleshooting, recovery procedures, and the existing controller only as an implementation/convention reference. The controller/API changes described by this design are follow-up work after the enhancement merges.
  - Questions to resolve:
    - Which failures are transient and retried automatically?
    - Which failures are terminal and require changing or reverting the spec?
    - When is `topologyTransitionStatus=Error` written?
    - Are Warning Events emitted once, on every retry, or only on state changes?
    - What exactly constitutes an administrator-initiated retry?
  - Intended disposition: design/documentation change; define behavior before editing.
  - Decision so far: actual sync/API failures should use the standard Kubernetes/OpenShift rate-limited workqueue retry with backoff; no custom retry loop or bespoke backoff should be documented.
  - Code evidence/reference: the existing controller uses `WithSyncDegradedOnError(operatorClient)` and `ResyncEvery(time.Minute)`. The library-go base controller calls `Queue().AddRateLimited(key)` when `sync` returns an error and `Queue().Forget(key)` when it returns nil. These are implementation conventions to preserve in the follow-up implementation, not changes required in this documentation PR.
  - Important distinction still to resolve: failed preflight conditions currently update conditions and emit a Warning Event, then return `nil`; they therefore do not use rate-limited retries and instead depend on informer events plus the one-minute resync. This is different from an actual sync/API failure.
  - Pair-review direction: transient/retriable sync failures should use the standard rate-limited workqueue with backoff and should be represented explicitly by a status such as `RetryWithBackoff`; terminal/user-fixable failures remain distinct from this state.
  - Design consequence to resolve: using `RetryWithBackoff` requires deciding which validator failures return an error, when the status is persisted relative to returning that error, and whether the resulting controller `Degraded` condition is intended.
  - Agreed semantics: `RetryWithBackoff` represents only actual sync/API errors. It maps to `Progressing=True`, `Upgradeable=False`, and the standard controller `Degraded=True` path. The controller uses the standard rate-limited retry queue; no custom retry timer is introduced.
  - Agreed semantics: an unmet admission precondition is not a sync/API error and must not use rate-limited retry. The controller sets `topologyTransitionStatus=Error`, posts a failure Event, and returns `nil`; a later informer-driven sync after the precondition changes may retry admission.
  - Agreed semantics: precondition failures and sync/API failures are distinct. Precondition failures use `Error` and do not set `RetryWithBackoff` or the controller Degraded condition; actual sync/API errors use `RetryWithBackoff` and do.
  - Follow-up implementation issue: the current controller uses `ResyncEvery(time.Minute)`, so the future implementation must decide whether to remove/change that resync to meet the event-driven precondition retry contract. This is not a code change for the documentation PR.
  - Agreed semantics: retry state transitions and failure Event behavior should be documented explicitly. `Error` represents the latest failed admission attempt, and can transition to `Pending` once admission succeeds; it is not a rate-limited retry state.
  - Follow-up implementation detail: if the future API/status write itself fails, `RetryWithBackoff` cannot be persisted by that failed write; the returned error and queue backoff remain authoritative until a status update succeeds.
  - Status: documentation changes applied; awaiting user approval before moving to the next CodeRabbit comment.
  - Applied in `enhancements/topologies/mutable-topology.md`: precondition failures now become `Error` with a Warning Event and successful `sync()` return; actual sync/API failures are represented by `RetryWithBackoff` and standard rate-limited workqueue behavior; failure handling, status mappings, troubleshooting, and recovery text were aligned.

- [x] **3. Revalidate admission preconditions after the fresh Infrastructure read** — resolved by design clarification
  - Source: CodeRabbit inline comment at line 187.
  - Claim: The fresh Infrastructure read protects the spec race but does not recheck ClusterVersion, ClusterOperators, Nodes, or etcd before publishing `Pending`.
  - Evidence to inspect: The topology transition controller's admission sequence and event/watch behavior.
  - Questions to resolve:
    - Is the revalidation requirement implementable without making the admission protocol misleadingly atomic?
    - Should the document require a second full validation, or should it describe the known check/write race explicitly?
    - What happens if a recheck fails after `Upgradeable=False` has already been published?
  - Intended disposition: likely documentation and possibly implementation change; verify controller behavior first.
  - Decision: A second full validation pass is not required. Once the admission preconditions are observed true, they are expected to remain stable over the short admission window. The fresh Infrastructure read protects the spec race; the document now explicitly states that admission is not an atomic snapshot across Infrastructure, Nodes, etcd, and ClusterOperators, and that later changes are handled by normal informer-driven reconciliation.
  - Status: complete — documentation updated; awaiting user approval before moving to the next CodeRabbit comment.

- [x] **4. Make orchestration order match the upgrade gate** — resolved
  - Source: CodeRabbit inline comment at line 373, later review.
  - Claim: The workflow says `Progressing=True`/`Upgradeable=False` must be written before Infrastructure `Pending`, while the orchestration section says the reverse.
  - Evidence to inspect: Current controller status-update order and the two document sections.
  - Questions to resolve:
    - Is condition-first ordering the intended and implemented contract?
    - How should the document describe the non-atomic failure case if the first write succeeds and the second fails?
  - Intended disposition: documentation correction if the implementation confirms condition-first ordering.
  - Decision: Condition writes are always prioritized over Infrastructure status writes. The document now orders `Progressing=True`/`Upgradeable=False` before publishing Infrastructure `Pending`, and documents the non-atomic partial-write/retry behavior.
  - Status: complete — documentation updated; awaiting user approval before finalization.

## Resolved findings to verify before closing

- [x] Infrastructure Events were added to the troubleshooting command.
- [x] The soak period was moved to `Progressing.LastTransitionTime`.
- [x] The workflow section documents condition-first publication order.

## Finalization

- [x] Run formatting/Markdown validation or repository-prescribed checks (`git diff --check`).
- [x] Review the complete diff with the user.
- [x] Decide whether each CodeRabbit thread should receive an applied, declined, or follow-up response.
- [x] Receive explicit confirmation to commit; changes will remain local and will not be pushed.
