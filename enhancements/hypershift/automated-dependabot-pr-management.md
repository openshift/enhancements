---
title: automated-dependabot-pr-management
authors:
  - "@bryan-cox"
reviewers:
  - TBD # HyperShift: dependency scope, repair behavior, and maintainer workflow.
  - TBD # Test Platform: Prow authorization, test triggering, and Tide merge gates.
  - TBD # Security: dependency exclusions, untrusted content, and credential isolation.
approvers:
  - TBD # Select one approver to coordinate review and establish consensus.
api-approvers:
  - None
creation-date: 2026-10-01
last-updated: 2026-10-01
status: provisional
tracking-link:
  - TBD # Add the issue tracking this enhancement.
see-also: []
replaces: []
superseded-by: []
---

# Automated Dependabot PR Management for HyperShift

## Summary

Chai auto-approves eligible Dependabot Go PRs limited to `go.mod`, `go.sum`, and vendored files. For other paths, including non-vendored Go code, Chai pings approvers in Slack for human review. Tests and verification gate Tide merges.

## Motivation

Maintainers repeatedly perform the same steps for dependency updates: inspect the changes, start tests, provide review labels, and record verification. Some updates also require regenerating vendored files before testing can succeed.

Automating this routine work can reduce maintainer effort and the time updates spend waiting for attention. The benefit depends on a narrow, understandable boundary between delegated work and changes that still require human judgment.

The proposal delegates review attestations only for eligible Go dependency updates. It retains existing test requirements and merge automation rather than introducing a second merge controller.

### User Stories

- As a HyperShift maintainer, I want routine dependency updates to progress without repeated manual commands, so that I can focus on changes that require engineering judgment.
- As a reviewer, I want Chai to notify me in Slack when an update needs human review, with an explanation, so that I can understand why automation stopped and what needs attention.
- As a HyperShift maintainer, I want Chai to replace Dependabot PRs that fail `verify` with validated repair PRs and notify approvers, so that I can review the fix rather than manually reconstruct the update.

### Goals

- Reduce manual shepherding of routine Go dependency PRs.
- Make eligibility decisions consistent, explainable, and auditable.
- Require human review whenever changes exceed the delegated scope or available evidence is insufficient.
- Preserve required testing, current-commit verification, and Tide's sole merge authority.

### Non-Goals

- Automating Kubernetes compatibility updates or security-sensitive exceptions to the agreed dependency policy.
- Approving application code, generated Go outside vendored directories, CRDs, workflows, container images, or non-Go dependency updates.
- Managing Renovate PRs, release-branch updates, or weekly consolidated dependency PRs in the initial pilot.
- Calling GitHub's merge API from Chai or replacing Tide.
- Monitoring dependency updates after merge, automatically detecting regressions, or automatically approving revert PRs.
- Changing HyperShift's runtime APIs, deployment model, or supported cluster topologies.

## Proposal

Use Chai to manage individual Dependabot Go dependency PRs targeting `main` in `openshift/hypershift`. Chai is the automation service responsible for classification, test coordination, and narrowly delegated review attestations.

An update enters the automated approval path only when both checks succeed:

- Its complete diff contains only allowed paths.
- The exception checks confirm that no Kubernetes compatibility or security-sensitive exclusion applies.

For example, a PR limited to vendored files still requires human review if it triggers a security-sensitive exclusion. Passing the path check does not bypass exclusions or required testing.

For eligible updates, Chai provides `approved` and `lgtm` after initial checks. Once the full required end-to-end suite passes on the current revision, it issues `/verified by e2e passing`. Tide alone merges once its requirements are met.

For eligible Go dependency bumps, passing the required end-to-end suite is sufficient pre-merge verification; no additional manual verification is required.

### Workflow Description

#### Actors and responsibilities

| Actor | Responsibility |
| --- | --- |
| Dependabot | Opens Go dependency update PRs. |
| Chai | Runs the automated workflow: evaluates eligibility, coordinates tests, records evidence, applies delegated attestations, notifies approvers, and handles retries and reconciliation. |
| Prow | Coordinates presubmit testing and processes supported review and test commands. |
| Tide | Merges PRs that satisfy the applicable labels, checks, and merge gates. |
| HyperShift maintainers and approvers | Own the policy and notification routing, respond to Slack review requests, authorize rollout, and handle post-merge recovery. |

`approved` represents delegated approval. `lgtm` represents delegated review. `verified` records a pre-merge testing attestation. Permission to start tests is separate from all three.

#### Eligible update

The automated workflow is:

1. Dependabot opens or updates a PR. Chai fetches its current commit ID, target branch, complete changed-file list, and full diff.
2. Chai checks the author, branch, allowed paths, and exception policies. It records the decision and claims processing ownership to prevent duplicate work.
3. Chai uses the supported test-trust path if needed and waits for initial checks, including `verify`. A failed `verify` on the original PR enters the repair workflow; otherwise, all initial checks must pass before approval.
4. Chai refetches the PR and requests `approved` and `lgtm` through the authorized repository-scoped mechanism. The configured workflow starts the required end-to-end suite after `lgtm`.
5. Chai waits for every job in the required end-to-end suite to pass for the current commit. It records job names, run links, tested revisions, and the policy version.
6. Chai confirms that the PR still has the tested revision and posts `/verified by e2e passing`.
7. Chai enables the final eligibility gate only while the evidence is current and automation remains enabled. Tide evaluates the applicable merge requirements and merges the PR.
8. Chai records the merge result and releases processing ownership. Its responsibility for the update ends at merge.

```mermaid
flowchart TD
    PR[Dependabot Go dependency PR] --> C{Complete diff and policy checks}
    C -->|Excluded or uncertain| H[Slack ping to approvers; human review]
    C -->|Eligible| I[Initial checks including verify]
    I -->|All pass| A[Authorized approved and lgtm]
    I -->|Original verify fails| R[Chai cherry-picks and repairs]
    R -->|Local validation passes| P["Open Chai PR, link and close original"]
    P --> H
    R -->|Repair or publication fails| F["Keep original open and held; request human help"]
    I -->|Other check fails| H
    A --> E[Required end-to-end suite]
    E -->|All pass| V["Chai posts /verified by e2e passing"]
    E -->|Any fails| H
    V --> G[Current evidence and eligibility gate]
    G --> T[Tide evaluates requirements and merges]
```

The configured test-trigger mechanism must be demonstrated before enabling automation. This proposal does not assume that adding `lgtm` already starts every required end-to-end job in the existing configuration.

#### Excluded, uncertain, or failed update

Chai provides no approval or verification for an excluded update. It pings the designated approvers in Slack to request human review. Any Go file outside an allowed vendored subtree requires human `lgtm`.

The notification targets approvers for the affected paths through maintainer-owned routing. It includes the PR link, evaluated commit ID, relevant changed paths, and the reasons the update needs human review.

Approvers review the PR and provide the required attestations through the normal GitHub/Prow workflow. A Slack reply is not an approval and does not make the PR eligible to merge.

Chai avoids duplicate pings for the same decision. If routing or delivery fails, it records the failure and retries the notification; the failure never causes automatic approval or verification.

API failures, incomplete diffs, unknown security results, and stale revisions block automated actions. Chai may retry data collection, but cannot infer eligibility from missing information.

Labels do not turn failed tests into passing evidence. Only a failed `verify` on the original Dependabot PR triggers replacement; other failures remain human-owned. Retests must produce fresh results for the current candidate.

#### Repair and replacement PRs

A failed `verify` check on an in-scope original Dependabot PR's current revision triggers this replacement path. Passing `verify` keeps the normal workflow; failures in other checks do not trigger replacement.

Chai keeps the original PR open and held while preparing the repair:

1. Chai creates a fresh branch from `main` in an isolated workspace and cherry-picks the pinned Dependabot update commits.
2. Chai fixes the `verify` failure and reruns `make verify` and plain `make test`. It uses `UPDATE=true make test` only when fixture regeneration is justified. Both validation commands must pass before publication.
3. Chai opens its own replacement PR. Its description links to the Dependabot PR and records the original failing revision, verification failure, repair changes, and local validation results.
4. Once the replacement PR is confirmed open, Chai links the two PRs and closes the original Dependabot PR as superseded. It does not wait for the replacement to merge before closing the original.
5. Chai pings the designated approvers in Slack with the replacement PR link, the reason for the repair, and the changes requiring review.

Every replacement PR is handed to human approvers, even if its diff remains within the original allowlist. Chai does not auto-approve these repair PRs or carry over the original PR's review labels or test results.

Reviewers assess the replacement's full diff, including generated assets and non-vendored Go changes. The replacement must pass its own required remote checks and normal human review before Tide merges it.

If repair, local validation, or publication fails, Chai leaves the original open and held and requests human help in Slack. It never closes the original without a confirmed replacement PR.

After the original is closed, failed checks or abandonment of the replacement require maintainer resolution. Closing the original marks it as superseded, not successfully merged.

### API Extensions

This enhancement introduces no OpenShift API extensions. It does not add or modify CRDs, webhooks, aggregated API servers, or finalizers. It uses existing repository and CI interfaces.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The automation manages the HyperShift source repository and runs in development or CI infrastructure. It adds no components, configuration, or resource requirements to management clusters or hosted control planes.

Dependency changes still require the agreed HyperShift test suite. Automating their review does not remove topology-specific testing or change supported management and guest cluster relationships.

#### Standalone Clusters

Standalone clusters receive no new runtime component or configuration. The enhancement changes a repository maintenance workflow, not how standalone clusters operate.

#### Single-node Deployments or MicroShift

The automation does not run on customer single-node or MicroShift deployments. It adds no CPU, memory, storage, or configuration requirements to either topology.

#### OpenShift Kubernetes Engine

The workflow operates outside customer clusters and has no dependency on OKE-specific runtime features. It does not change OKE behavior or configuration.

### Implementation Details/Notes/Constraints

#### Eligibility policy

The policy has two outcomes: **eligible (`NOMINAL`)** and **human review (`SPECIAL`)**. `SPECIAL` includes incomplete or uncertain evidence; it does not necessarily indicate a defect in the update.

Automatic approval requires an open original Dependabot PR with a verified author identity targeting `main`. Chai-authored repair PRs always enter human review, regardless of their changed paths.

The initial manifest allowlist covers the Go modules currently configured for Dependabot. Paths are repository-relative; vendored subtrees are allowed only under these module roots.

| Module root | Allowed manifests | Allowed vendored subtree |
| --- | --- | --- |
| Repository root | `go.mod`, `go.sum` | `vendor/**` |
| `api` | `api/go.mod`, `api/go.sum` | `api/vendor/**` |
| `hack/tools` | `hack/tools/go.mod`, `hack/tools/go.sum` | `hack/tools/vendor/**` |

Any other path requires human review, including `go.work`, another module's manifests, generated assets, documentation, and workflow files. Expanding the module roots requires an explicit policy change and maintainer review.

Classification must inspect both old and new paths for renames and all pages of the changed-file listing. It must detect truncation and missing diff content; an API limit or error cannot become an empty, eligible diff.

Kubernetes compatibility and security exclusions override the path allowlist. Maintainers must approve the dependency rules and security-review inputs before enabling automation. Unavailable or inconclusive checks require human review.

All changed module manifests must be examined for exclusions, including modules outside the allowlist. PR titles, Dependabot grouping, and a patch-version designation are not substitutes for examining the actual update.

#### Evidence and state

Each decision records the repository, PR number, verified author identity, base and head commit IDs, complete changed paths, policy version, exception results, reasons, and proposed or completed actions.

The idempotency key is `repository#PR#head_sha#policy_version`. Chai also records the test-suite configuration and tested base revision so changes to either cannot silently reuse incompatible evidence.

Chai refetches current state after delayed or duplicate events and before every privileged action. A head change invalidates the classification, pending actions, Chai-owned attestations, verification evidence, and eligibility gate.

Chai reconciles missed or delayed events and rechecks the current revision before issuing queued commands. Passing tests from an older revision cannot justify verification of a newer revision.

Initial checks gate `approved` and `lgtm`. The full required end-to-end suite gates `/verified by e2e passing`. Tide separately enforces all merge-required checks.

Missing, pending, failed, canceled, or skipped required jobs do not count as successful evidence. Chai must validate the expected job identities and revisions, not merely observe a green aggregate status.

Base-branch changes also affect the tested merge candidate. Prow and Tide owners must confirm how the current base or merge revision is tested and revalidated. Head-commit equality alone does not prove merge-candidate freshness.

#### Authorization boundaries

Test-trigger and review permissions are separate from merge authority. Chai is not added to broad root OWNERS aliases, and it receives no direct merge capability.

The privileged attestation path must enforce the approved repository and eligibility policy. Repository-scoped credentials alone cannot restrict writes by changed-file path; that restriction must exist at the trusted action boundary.

The supported path for `approved` and `lgtm` must enforce the required boundary. If existing Prow authorization cannot do so, the integration needs an explicitly reviewed policy-enforcing mechanism.

#### Verification

Chai is assumed to have permission to issue `/verified`. It posts `/verified by e2e passing` once all required e2e jobs pass on the current revision.

How that command is authorized or processed is outside this enhancement's scope.

The standardized reason is `e2e passing`; job names, run links, and tested revisions remain in the decision record.

#### Execution and credential isolation

PR diffs, dependency contents, test output, and vendored instruction files are untrusted data. They cannot change the eligibility policy, authorize actions, or supply instructions to the privileged writer.

Repair and validation workers receive no upstream or fork write credentials, GitHub App private keys, or writer credential helpers. Publication and attestation occur in a separate trusted component that revalidates the candidate.

Validation environments use only the test credentials they require. Their isolation and access to CI secrets need security review; removing GitHub write tokens alone does not make executing dependency code harmless.

#### Coordination with existing automation

Chai owns individual eligible Dependabot PRs, not weekly consolidated updates. The weekly triage job must skip claimed candidates or be disabled for the overlapping scope before enabling the workflow.

Ownership is recorded before work begins and is shared with the weekly job. The coordination mechanism must handle duplicate events, interrupted repairs, and abandoned claims without closing an original PR before its replacement exists.

Reusing the weekly job requires fail-closed file retrieval, coverage of all changed modules, and separation of validation from writer credentials. It does not receive Chai's attestation authority unchanged.

#### Merge gates and shutdown

Automated merge eligibility requires current policy and test evidence, the required labels and checks, and an enabled automation gate. A static label alone cannot establish current-commit eligibility.

Every Tide query capable of selecting a managed PR must honor equivalent gates. The current bot-specific and general queries overlap; adding a condition only to the bot query does not establish isolation.

A central enablement flag stops new test-trust grants, attestations, and eligibility decisions. Shutdown must also block already-attested open PRs, using an effective hold or the agreed merge-gate mechanism.

When disabled, Chai blocks pending candidates and invalidates queued actions. Credential revocation alone does not remove existing labels or cancel actions already accepted by another service.

The concrete gate, shutdown response time, and any already-dispatched merge behavior must be agreed and tested before enabling automation. Chai does not monitor or undo changes that have already merged.

### Risks and Mitigations

| Risk | Mitigation |
| --- | --- |
| Unsafe dependency content matches the path allowlist. | Keep explicit Kubernetes and security exclusions, review full diffs, and route inconclusive results to humans. |
| Incomplete or stale input is treated as eligible. | Verify complete listings and revisions, fail closed, and reconcile delayed events and commands. |
| Bot permissions exceed the delegated scope. | Enforce policy at the trusted writer, separate permissions, and avoid broad OWNERS membership. |
| Tests or dependency code expose credentials. | Isolate validation from writer credentials and review the remaining CI secret boundary. |
| Another Tide query bypasses eligibility or shutdown. | Review every matching query and test that held, stale, and disabled candidates cannot merge. |
| Repair hides additional changes or loses the original update. | Require human review of the full replacement diff, and close the original only after a validated replacement PR is open and linked. |
| Additional test runs increase CI cost or queue time. | Limit the pilot, measure test usage, and require maintainer approval before increasing throughput. |
| A regression appears after merge. | Use the normal human-owned investigation and revert process; pre-merge checks cannot eliminate every regression. |

HyperShift maintainers review the policy and maintainer experience. Test Platform owners review Prow permissions and merge behavior. Security reviewers assess content handling, permissions, and execution isolation.

### Drawbacks

- Chai's attestations replace a human review step for the delegated scope. A policy or evidence-validation defect can therefore have direct merge consequences.
- The service adds state, authorization, and coordination across several systems. Maintaining those integrations has an ongoing cost.
- Conservative exclusions and incomplete evidence will still require maintainer attention, limiting the achievable automation rate.
- Individual updates can consume more CI capacity than weekly consolidation. The pilot must establish whether reduced manual effort justifies that cost.

## Alternatives (Not Implemented)

### Continue fully manual review

Manual review preserves the existing workflow without another service. It does not reduce recurring shepherding of routine updates, although it remains the fallback and the required path for exceptions.

### Automate weekly consolidated updates

Consolidation can reduce test runs, but combines dependencies and complicates failure attribution, replacement review, and source-PR closure. The initial design instead operates on individual Dependabot PRs.

### Use GitHub auto-merge for Dependabot PRs

GitHub auto-merge can merge Dependabot PRs after required checks and reviews pass, without using GitHub's merge queue. It can wait for Prow checks, but only those required by GitHub—not every Prow job or Tide label.

As of 2026-10-01, HyperShift's `main` required-check list omits `ci/prow/e2e-*` contexts. Auto-merge would need equivalent e2e, approval, and verification gates in GitHub. This proposal retains Tide as the sole merger.

### Give Dependabot or Chai unrestricted review trust

Broad trust is simpler to configure but does not enforce the agreed file and exception boundaries. Test-trigger permission must not implicitly grant approval, verification, or merge authority.

### Apply verification labels or merge directly through GitHub

Chai issues the explicit `/verified by e2e passing` command rather than writing verification labels directly. Direct merging would introduce a second merger; Tide remains the sole merge authority.

## Open Questions

These implementation details remain to be resolved. Direct individual PRs, human-handled reverts, deferred Renovate support, and no post-merge monitor are settled boundaries, not open alternatives.

1. **Eligibility exceptions:** Which exact dependency and version rules identify Kubernetes compatibility updates, and which security signals or review criteria must succeed? Owners: HyperShift maintainers and Security.
2. **Prow permissions:** What is Chai's exact GitHub identity, and which mechanisms authorize its test-trigger, approval, and review actions within the agreed scope? Owners: repository access and Test Platform teams.
3. **Test contract:** Which initial and end-to-end jobs are mandatory, how does `lgtm` start the latter, and how is current-base testing established? Owners: HyperShift and Test Platform maintainers.
4. **Merge and shutdown gates:** How will eligibility be enforced across overlapping queries and stale commands, including an already-dispatched merge? What is the shutdown response target? Owners: HyperShift and Tide maintainers.
5. **Throughput limits:** Are the proposed initial limits below appropriate, and what evidence permits increasing them? Owners: HyperShift maintainers.
6. **Processing ownership:** What claim mechanism will Chai and the weekly job share, and how will claims be recovered after interruption? Owners: Chai and weekly-job maintainers.
7. **Approver notifications:** What Slack destination, path-to-approver routing, and notification retry policy should Chai use? Owners: HyperShift maintainers.

Reviewer identities, one coordinating approver, and the enhancement's tracking issue must also be supplied before submission. No existing prerequisite issue is assumed to track the complete proposal.

## Test Plan

### Policy and evidence tests

- Cover each allowed module, vendored Go files, disallowed module roots, `go.work`, non-Go lockfiles, generated assets, and Go files outside vendored subtrees.
- Cover both sides of renames, deletions, pagination, API limits, missing diff content, and unavailable exception results. Each incomplete or excluded case must produce no Chai attestation.
- Test Kubernetes and security overrides independently of path eligibility. Cover mixed dependency groups rather than relying on PR titles or update-type metadata.
- Test duplicate and out-of-order events, policy changes, head changes, base changes, and incorrect job identities or tested revisions.

### Authorization and workflow integration

- Use controlled canaries to prove the exact actor can trigger only the intended tests and provide `approved` and `lgtm` through the supported path.
- Demonstrate the two test stages. No approval precedes successful initial checks, and Chai issues `/verified by e2e passing` only after the full required end-to-end suite passes on the current revision.
- Verify that missing, pending, failed, canceled, skipped, or stale required e2e results prevent Chai from issuing the verification command.
- Change a PR's head while verification is queued. Confirm Chai does not issue the command using old test results and requires fresh results for the new revision. Test merge-candidate freshness when the target branch advances.
- Verify every applicable Tide query rejects a managed candidate with missing labels, stale evidence, a failed required check, a hold, or disabled automation.
- Verify an out-of-scope or excluded update triggers a Slack ping to the designated approvers with its PR link, revision, changed paths, and reasons, while receiving no Chai attestations.
- Verify duplicate events do not repeat the same ping, delivery failures remain visible and retryable, and Slack replies cannot supply approval or merge eligibility.

### Repair and execution isolation

- Verify replacement is triggered only by a failed `verify` on the current revision of the original Dependabot PR. Passing, pending, missing, or stale `verify` results and failures in other checks must not trigger it.
- Reproduce a `verify` failure, cherry-pick the pinned update, fix the failure, and verify the strict local validation sequence before publication.
- Verify Chai opens its own replacement PR, links and closes the original as superseded, and then pings the approvers in Slack. Closure must not depend on the replacement having already merged.
- Verify repair, validation, or publication failure leaves the original open and held. Never close it before the replacement PR is confirmed open.
- Verify all replacement PRs require human review, including replacements whose diffs remain within the allowlist. They must not inherit labels or test evidence and must pass their own remote checks.
- Verify a failed `verify` on a Chai-authored replacement does not recursively create another replacement.
- Verify workers cannot access writer tokens, private keys, or credential helpers, and that vendored instruction files cannot influence privileged decisions.
- Verify the weekly job respects ownership claims and fails closed when file retrieval is unavailable.

### Shutdown and recovery

- Disable automation before approval, during end-to-end testing, after verification, and while commands are queued. Confirm pending candidates are blocked across all applicable merge queries.
- Exercise the agreed handling of an already-dispatched merge and measure shutdown latency. The runbook must describe any limit on stopping work already in progress.
- Restart after missed events or worker interruption. Require current classification and evidence before restoring eligibility, without deleting human-owned review decisions.

## Graduation Criteria

This is repository automation, not a customer-facing OpenShift feature. The required maturity headings describe operational rollout milestones; they do not introduce a product feature gate or an OpenShift release dependency.

### Dev Preview -> Tech Preview

Before enabling the automated workflow:

- Policy tests and representative cases must cover each allowed module, excluded updates, and incomplete inputs, with no excluded case receiving Chai attestations.
- Maintainer-approved exception rules, test-suite definitions, and a documented comparison with human decisions.
- Passing authorization canaries, stale-event tests, repair tests, and merge-gate tests.
- Tested Chai shutdown and recovery behavior, with documented maintainer controls.
- Confirmed deconfliction with the weekly job and approved credential isolation.

The proposed initial limit is one in-flight PR and at most one automated merge per UTC day. Maintainers must approve or adjust these limits before enabling the workflow.

### Tech Preview -> GA

Expand throughput only after maintainers review classification accuracy, manual interventions, test cost, processing latency, and shutdown behavior. Any scope or throughput expansion requires explicit approval.

HyperShift maintainers own policy updates and integration configuration. Chai records decisions and handles workflow retries and reconciliation. Recovery instructions describe when human intervention is needed.

This milestone does not expand the proposal to Renovate, other branches, reverts, or post-merge monitoring. Those changes require a separate scope decision and review.

### Removing a deprecated feature

No customer-facing feature is deprecated. If the weekly job is disabled for overlapping PRs, document its new scope and rollback procedure before activating the replacement workflow.

## Upgrade / Downgrade Strategy

No cluster upgrade or downgrade behavior changes. Chai policy, test-contract, authorization, and merge-configuration changes are deployed with automation disabled until their integration checks pass.

Policy changes receive a new version and invalidate affected decisions. Do not carry old evidence into a policy or test contract that changes the candidate's requirements.

Rollback first blocks pending automated merges, then disables Chai's writes and restores the manual workflow. Maintain holds until a maintainer reconciles Chai-owned attestations, source/replacement relationships, and outstanding commands.

Do not remove unrelated human labels or resume the weekly job over claimed PRs without resolving ownership. Re-enabling automation requires fresh classification and compatible current-revision evidence.

## Version Skew Strategy

Chai depends on the deployed GitHub, Prow, and Tide behaviors, not on control-plane and worker version skew. Prow permissions and test-contract compatibility must be verified against those deployed services.

When an integration changes or its behavior is unknown, automated actions stop. Resume only after integration tests prove the required contract and stale-evidence handling.

## Operational Aspects of API Extensions

There are no OpenShift API extensions, so this proposal adds no API-server availability, latency, admission, or conversion dependencies. Repository automation health is handled through the support procedures below.

## Support Procedures

Each candidate's record must show its evaluated revision, policy decision, current stage, required jobs, evidence links, authorization results, ownership claim, and source/replacement relationship where applicable.

Chai tracks blocked decisions, failed commands, processing age, reconciliation lag, and CI usage. It reports problems requiring human intervention to maintainers. It does not monitor regressions after merge.

| Symptom | Diagnostic and recovery action |
| --- | --- |
| PR never becomes eligible | Read its reason codes; distinguish a policy exclusion from unavailable evidence. Retry data collection or hand the PR to maintainers. |
| Approvers do not receive a Slack ping | Check notification status and approver routing, then retry delivery. Keep the PR in the human-review lane without Chai attestations. |
| Initial or end-to-end tests do not start | Check the agreed job list, trigger configuration, actor authorization, and observed Prow response. Do not substitute labels for missing tests. |
| Chai cannot post the verification command | Inspect Chai's command-posting failure, then retry only after confirming that the required e2e results still match the current revision. |
| Evidence refers to an old revision | Keep the candidate blocked, invalidate stale actions, and rerun classification and the required tests. |
| Repair fails before a replacement opens | Keep the original open and held; inspect the failure and request human help. |
| A published replacement is stalled or abandoned | Inspect its checks, full diff, and source link. Maintainers decide how to proceed or whether to reopen the superseded original. |
| Chai and the weekly job both claim a PR | Hold the candidate and reconcile processing ownership before either workflow resumes. |
| Automation needs to stop | Disable new actions, apply the agreed merge block to pending candidates, and confirm that every matching Tide query honors it. |

Chai handles workflow retries and reconciliation. HyperShift maintainers handle policy exceptions and post-merge regressions; Test Platform maintainers handle CI integration failures.

Disablement affects pending repository work, not running customer workloads. Recovery preserves audit records and human review, and re-evaluates open candidates rather than replaying old attestations.

## Infrastructure Needed

- A Chai execution service with durable decision records, event reconciliation, and a centrally controlled automation-enable flag.
- A separately authorized writer and isolated repair/validation workers, with access reviewed by Security and repository owners.
- Prow and Tide configuration changes implementing the agreed test contract and merge gates.
- A shared processing-ownership mechanism or an explicit exclusion in the weekly job.
- Slack notification access and maintainer-owned routing to the designated approvers, with delivery records and retry handling.
- Controlled test PRs, decision records, and maintainer recovery instructions. No new customer-cluster infrastructure is required.

### References

- [OpenShift enhancement template](../../guidelines/enhancement_template.md)
- [HyperShift Dependabot configuration](https://github.com/openshift/hypershift/blob/main/.github/dependabot.yml)
- [HyperShift Prow plugin configuration](https://github.com/openshift/release/blob/main/core-services/prow/02_config/openshift/hypershift/_pluginconfig.yaml)
- [HyperShift Tide queries](https://github.com/openshift/release/blob/main/core-services/prow/02_config/openshift/hypershift/_prowconfig.yaml)
- [HyperShift main branch protection metadata](https://api.github.com/repos/openshift/hypershift/branches/main)
- [GitHub Dependabot automation guide](https://docs.github.com/en/code-security/tutorials/secure-your-dependencies/automate-dependabot-with-actions)
- [Existing dependency triage workflow](https://github.com/openshift/release/tree/main/ci-operator/step-registry/hypershift/dependabot-triage)
