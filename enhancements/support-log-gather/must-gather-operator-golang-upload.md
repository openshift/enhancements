---
title: must-gather-operator-golang-upload
authors:
  - "@neha037"
reviewers:
  - "@shivprakashmuley"
  - "@swghosh"
  - "@Prashanth684"
approvers:
  - "@Prashanth684"
api-approvers:
  - "None"
creation-date: 2026-09-30
last-updated: 2026-11-06
tracking-link:
  - https://issues.redhat.com/browse/MG-386
see-also:
  - "/enhancements/support-log-gather/must-gather-operator.md"
  - "/enhancements/support-log-gather/operator-upload-targets.md"
  - "/enhancements/support-log-gather/must-gather-bundle-obfuscation.md"
---

# Must-Gather Operator: Go-Based Upload

## Summary

Replace `build/bin/upload` with `must-gather-operator upload` for the configured SFTP server, using Go archives and
a shared SSH/SFTP client for preflight and upload. The upload command orchestrates optional obfuscation as a child
process in the same container before archiving and transfer, using existing configuration. Preserve bundle
naming/content, obfuscation workflows and logs, and the gather/upload arrangement; fix proxy/trust divergence,
CONNECT buffering, credential parsing, and false-success cleanup.
Include bounded pipelining, independent artifact verification, and phase diagnostics. These are intentional behavior
changes, including status reasons and metric semantics; the MustGather CRD schema remains unchanged.

## Motivation

Preflight can approve a route that upload then rejects, and a failed proxied upload can report success and delete the
local archive.

### User Stories

- As a support engineer, I want reliable completion and actionable diagnostics to resolve upload failures.
- As a cluster administrator, I want consistent proxy exclusions, authentication, and CA trust before and during upload.
- As an operator developer, I want one context-aware client with deterministic connection, transfer, and cleanup tests.
- As a QE engineer, I want independent checks of the current operation's exact artifact name and contents.

### Goals

- Preserve bundle and obfuscation compatibility while implementing archive creation and SFTP transfer in Go.
- Share route resolution, URL parsing, TLS/CONNECT, trust, buffering, SSH/SFTP setup, and cancellation between callers.
- Require acknowledged transfer completion before cleanup, with bounded write concurrency, measured throughput and
  resource use, and FIPS validation.
- Improve artifact verification and failure reporting, count preflight failures, and remove production shell upload
  dependencies while retaining polling/image tools.

### Non-Goals

- Changes to the gather image contract, `pgrep` polling, two-container arrangement, existing storage mounts, or
  ServiceAccount restrictions.
- New upload targets, resumable transfers, atomic remote publication, automatic remote deletion or rename, or upload-only
  in-Pod retries.
- New MustGather spec fields or changes to the gather-only meaning of `spec.mustGatherTimeout`.
- New connection/setup, upload-inactivity, or overall Job timeout controls.
- In-process obfuscation before safely returning library APIs and diagnostic writers exist; host-key distribution or
  changes to disabled SSH host-key verification.
- Older-Job launchers/migrations, image-pinning enforcement, or MicroShift-specific behavior.

## Proposal

Dispatch `upload` before controller-manager initialization, following the existing `obfuscate` subcommand pattern.
Controller and uploader adapters supply credentials, configuration, and trust inputs to one shared transport component.
Keep connection policy independent of Kubernetes API types so both callers follow the same rules.

### Workflow Description

1. When trusted CA is configured, resolve [CA readiness](#shared-connection-policy) before creating any Job. For SFTP
   targets, then validate the CR, credentials, and proxy configuration, and run preflight. Pending injection requeues
   with no terminal status. Obfuscate-only Jobs need no SFTP connectivity.
2. Create the existing gather and conditional-upload Job. Gather writes output; upload retains the outer `pgrep` wait.
   Obfuscate-source Jobs skip gather and invoke upload directly.
3. Run [obfuscation](#obfuscation). Failure stops before archive or upload.
4. Archive and transfer under [transfer and pipelining](#transfer-and-pipelining).
5. Reconcile terminal Job state into CR status under [Job lifecycle](#job-lifecycle). Preserve Job retries and
   resource retention.

### API Extensions

No CRD schema, webhook, upload-target union, or finalizer changes. Spec immutability and gather timeout semantics stay
unchanged. No new timeout flags or timeout environment variables are introduced. Status reasons and metrics change
as defined in [failure reporting](#failure-reporting-and-metrics). Release notes cover the intentional changes under
[configuration](#compatibility-and-configuration).

### Topology Considerations

Operator availability is unchanged; the same upload/proxy/trust contracts apply wherever deployed. On hosted
clusters, operator and Jobs run on the guest cluster using its proxy, CA, and network configuration. No management-cluster
component, cross-cluster connection, or topology-specific service is added.

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

Operator and Jobs run in the same cluster; no hosted-control-plane dependency is introduced.

#### Single-node Deployments or MicroShift

Keep the short-lived uploader, bounded buffers, and streaming I/O; measure constrained-deployment resource use. No
persistent daemon is added.

MicroShift: N/A

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

#### Compatibility and Configuration

Preserve password authentication, IPv4/IPv6 normalization, disabled SSH host-key checking, shared process namespace,
and the current Job retry policy.

- Local filename: `${FILENAME_PREFIX}.tar.gz`; external remote path: `${caseid}_${FILENAME_PREFIX}.tar.gz`;
  internal remote path: `${username}/${caseid}_${FILENAME_PREFIX}.tar.gz`.
- Keep inputs `username`, `password`, `caseid`, `host`, `internal_user`, `must_gather_output`, `must_gather_upload`,
  `FILENAME_PREFIX`, `obfuscate`, `obfuscate_config`, and the proxy variables below. Credentials remain same-namespace
  SecretKeyRef inputs; do not replicate Secrets.
- Script input/workspace defaults remain `/must-gather-output` and `/must-gather-upload`; the template explicitly
  supplies `/must-gather`, `/must-gather-upload`, and `FILENAME_PREFIX`.

`spec.mustGatherTimeout` limits only the gather command and defaults to no limit when unset. Preflight keeps its
current dial limit and per-reconcile retry behavior. That dial limit is not applied to upload. This EP adds no upload
connection, inactivity, or Job deadline.

Intentional changes are shared routing and exclusions, HTTPS preflight trust, retained tunnel bytes, decoded
credentials, fatal transfer errors, the CA and cancellation outcomes below, prohibited Digest without a Basic
fallback, and the status and metric contracts. Release notes identify these changes.

#### Shared Connection Policy

**Routing and parsing.** Both callers share environment-based proxy resolution for the normalized SFTP target,
evaluating HTTPS settings first, then HTTP fallback if no proxy is selected, with consistent exclusions throughout.
Preserve resolution errors; never bypass a proxy because parsing or connection failed.
Non-empty uppercase variables take precedence over lowercase. Honor `NO_PROXY`/`no_proxy` and implicit
localhost/loopback bypass. The controller forwards effective resolved settings as lowercase Job variables, resolving
case conflicts consistently. This intentionally changes upload's lowercase HTTPS preference and ignored exclusions.

Validate effective proxy URLs and decode credentials once, requiring percent-encoding for reserved userinfo characters.
Allow HTTP/HTTPS and valid host/port; normalize scheme-less `host[:port]` to HTTP. Reject malformed
escapes, invalid ports, and unsupported schemes without leaking credentials. Explicit ports win. A missing port uses
today's preflight default for both callers.
The URL scheme controls transport, independently of variable name: an `http` scheme means TCP CONNECT, not TLS, even
when the variable is `https_proxy`.

**Transport and authentication.** HTTP proxies use TCP CONNECT; HTTPS proxies require TLS first, verifying the proxy's
certificate chain and hostname. Never retry in plaintext. Support Basic, and add HTTP Digest for both callers; today both send Basic only.
Prefer Digest when the server offers an algorithm crypto policy permits. If every offered Digest algorithm is
prohibited, fail the connection and do not fall back to Basic. Use Basic only when Digest is not offered. Bound
challenge handling. Failing a prohibited Digest algorithm, instead of using it, is an intentional FIPS change.

**Trust and readiness.** Existing CA injection, ConfigMap distribution, mounts, and ownership remain unchanged.
With `--trusted-ca-configmap`, preflight reads the bundle from the effective CR-namespace ConfigMap through the
Kubernetes API; upload reads the mounted copy of that same bundle.
Both use the configured bundle as the trust source, containing all roots required by the selected HTTPS proxy.
Only when no bundle is configured do they use release-image system roots. Configured trust never falls back to
system roots, unverified TLS, or plaintext.

When `--trusted-ca-configmap` is set, this check runs before any Job, including gather-only. SFTP preflight runs only
after it, and only for SFTP targets. A ConfigMap is injection-managed when it has
`config.openshift.io/inject-trusted-cabundle=true`. A missing ConfigMap, a transient API read, or missing or empty
`ca-bundle.crt` on an injection-managed ConfigMap requeues with no completion, terminal status, event, or error
counter, and consumes none of the preflight attempts. A present static ConfigMap whose `ca-bundle.crt` is missing,
empty, or not usable PEM, and an injection-managed ConfigMap whose bundle is non-empty but not usable PEM, are
terminal. A missing Secret stays a non-terminal requeue. A Secret that exists with an empty `username` or `password`
is terminal. Unavailable mounted trust fails that upload attempt and retains the local archive.

**CONNECT buffering.** Preserve all tunnel data received alongside proxy response headers, including an early SSH
banner. Failed setup must release resources without masking the original error; successful setup must leave the
tunnel usable for SSH/SFTP.

Preflight remains point-in-time validation: environment, credentials, CA contents, network policy, and endpoint state
can change before upload.

#### Job Lifecycle

Preserve gather's existing shell timeout and polling, Job retries, and one CR per operation. Preflight keeps its
current per-reconcile retry behavior, and those attempts are not stored on the CR. Only a deadline exceeded and a
network timeout are retried, including a DNS lookup that times out. The exhausted retry completes the CR.
Authentication, certificate failure, connection refused, a definite DNS failure, and the terminal configuration
failures under [trust and readiness](#shared-connection-policy) complete the CR immediately. CA waits consume none of
the preflight attempts.

A canceled reconcile aborts preflight, does not retry that attempt, and does not complete the CR. The upload process
stops obfuscation, archive creation, connection, and transfer on cancellation, and does not upload after a canceled
archive. The process does not retry. Job retry behavior is unchanged: a non-zero Pod exit, including after kubelet
termination, still counts, and the replacement Pod reruns gather when gather is in the Job.

An inactive Job is terminal when Kubernetes reports `Complete` or `Failed`. `FailureTarget` alone does not finish the
CR, even if the failure count is already past the retry limit. When none of `Complete`, `Failed`, or `FailureTarget`
is present, today's success and failure counts decide, and only while the Job is inactive. One failed Pod is not
terminal.
A completed CR is recognized before the ServiceAccount check and the other pre-Job checks, so those checks cannot
rewrite it. It is not given a new Job. Pending cleanup of an already completed CR finishes without recreating the
Job. `retainResourcesOnCompletion` is unchanged.

#### Archive Creation

Create a complete local tar.gz before connecting, using streaming I/O with bounded memory and retaining existing
workspace and PVC mounts. Match current GNU tar semantics, including special files and unreadable or changing inputs,
and compare contents rather than gzip bytes or entry order. Archive-write or finalization failures must prevent upload.

#### Transfer and Pipelining

Open the remote file write-only with create/truncate semantics. Success requires every write acknowledged without
error, total acknowledged bytes equal to the completed local archive's positive size, and successful remote close.
Local read, remote open/write/close, short write/count mismatch, or cancellation fails both direct and proxied
routes; preserve the first error and sanitized secondary cleanup diagnostics. Apparent remote length cannot override
write errors: concurrent failures can leave holes.

Do not require production post-close Stat, listing, or readback: Red Hat removes case-prefixed files after case
attachment, creating a normal visibility race even with valid permissions. Protocol completion proves neither durable
storage nor case ingestion; [the Secure FTP guide](https://access.redhat.com/articles/5594481) describes consumption.
[Controlled tests](#integration--e2e-tests) must independently verify retained artifacts and successful immediate
consumption.

Only after success remove the local archive, best effort. Otherwise retain it and omit success messages. Failed
obfuscation precedes archive creation: preserve logs/existing workspace artifacts rather than promising a new archive.
Partial remote files may remain and are not completed artifacts. Default completion cleanup can remove failed Pods;
emptyDir data disappears with its Pod, PVC data follows volume lifetime. `retainResourcesOnCompletion` permits
inspection but cannot preserve emptyDir across replacement. Job retries rerun gather when present, without resume or
an extra in-Pod retry loop.

Use bounded concurrent SFTP writes and buffering to support efficient transfers without unbounded memory growth.
Use server acknowledgements and byte counts to establish transfer completion; stop transfer work on failure or
cancellation. Resource measurements must include the full process and obfuscation child, not just transfer buffers.
Validate throughput, CPU, peak RSS, and elapsed time against OpenSSH; do not assert unmeasured speedups. Validate
supported FIPS operation without bypassing crypto policy or claiming new certification.

#### Obfuscation

The `upload` command invokes the existing `obfuscate` subcommand as a child process in the same container when
obfuscation is enabled, using the existing MustGather configuration. No new flag or controller-managed step is
introduced. The process boundary protects uploader cleanup from the obfuscation library's process-exit behavior.

Preserve combined stdout/stderr in console stdout and `${must_gather_upload}/obfuscation.log`, and copy that log to
`cleaned/obfuscation.log` on success and failure. A failed child keeps its status even if the log copy also fails.
If that status is unavailable, or the child succeeds and the log cannot be persisted, exit non-zero and do not
archive or upload. Obfuscation failure stops before credential checks, archive creation, and upload.

Preserve gather/obfuscate/upload, obfuscate-only, and read-only source-PVC modes. Check credentials only after
obfuscation and log persistence succeed. All of `caseid`, `username`, and `password` empty on an obfuscate-only run
is success without archive or upload. Any other partial credential set fails before archive creation.

#### Failure Reporting and Metrics

`ReconcileError` stays the condition type. Terminal pre-Job validation uses `PreflightValidationFailed` instead of
`ValidationFailed`. Terminal Job failure, which sets no `ReconcileError` today, gains `JobFailed`. Non-terminal
errors, including a missing Secret and cleanup retries, keep `LastReconcileCycleFailed`. CA readiness sets no
`ReconcileError`.

`status.status` stays `Completed` or `Failed`, and `status.completed` is true for those results. Job success keeps
`MustGather Job pods succeeded`. Job failure keeps `MustGather Job pods failed`. Terminal pre-Job validation uses
one sanitized `status.reason`, also the condition message, that names the failing check: ServiceAccount, image,
credentials, proxy settings, trusted CA, or SFTP. SFTP is one reason for authentication, certificate failure,
connection refused, a definite DNS failure, the exhausted preflight timeout, a prohibited Digest algorithm, and any
proxy CONNECT failure, including proxy authentication failure. It carries no secrets, proxy userinfo,
host names, or dial details, and it does not distinguish reachable from unreachable. `proxy settings` names only a
rejected proxy URL. Emit one Warning
`ProcessingError` for each new terminal transition, and do not emit it again. CA readiness emits no event.

Exit 0 only for transfer success or for obfuscate-only success with all credentials empty. Otherwise exit non-zero.
Log the failing phase in operator logs for preflight and in container logs for upload. Never log proxy userinfo,
authorization headers, passwords, or raw errors containing them. Generic Job failure still cannot name the failed
container.

Increment `must_gather_operator_must_gather_errors` once per persisted terminal failure. Add
`must_gather_operator_must_gather_validation_errors` for the pre-Job subset only. Job failure increments the error
counter and not the validation counter. CA waits, a missing ConfigMap, a missing Secret, retries, Pod replacement,
reconcile cancellation, and cleanup retries increment neither. Keep
`must_gather_operator_must_gather_total` as successful Job creations, so errors can exceed it. Update help text,
documentation, dashboards, and alerts. Do not repeat a terminal event or counter. Counters reset with the operator
process.

#### Integration and Packaging

Separate shared connection policy/lifecycle, archive/transfer handling, and upload orchestration into independently
testable components. Controller and uploader adapters must pass the same connection-policy tests. Transfer tests
must cover acknowledgements, cancellation, and failure cleanup.

Jobs invoke `/usr/local/bin/must-gather-operator upload` after existing gather coordination, or directly for source-PVC
obfuscation. Preserve the shared process namespace, mounts, and existing configuration inputs; supply the shared
trust inputs. Remove obsolete SSH-directory/known-hosts setup.

Remove the production uploader and HTTPS helper scripts, and the upload-only tools they require, from production,
OpenShift CI, and coverage images. Retain the operator binary, obfuscation configuration, polling and image-setup
tools, and coverage support. Check built package inventories, including inherited packages; the gather image keeps its
tools.

Move artifact verification to a CI-built/published test-only image with an independently implemented client and archive
tools. It must follow the documented connection policy and detect production regressions independently.

With implementation, update the upload, Job, controller, and API design docs with the contracts that change, plus
domain and architecture guidance. Update predicate or watch design if it changes. Do not rewrite accepted ADR history;
the [two-container ADR](https://github.com/openshift/must-gather-operator/blob/master/docs/adrs/adr-0002-two-container-job.md)
still applies. Gather-completion redesign remains independent.

### Risks and Mitigations

- Route/exclusion, port, decoding, auth, and TLS changes can expose configuration mistakes or bypass a formerly used
  proxy: test both adapters and document upgrade effects.
- Existing timing limitations remain: stalled setup or upload and long-running obfuscation or archiving may require
  operational intervention. See [configuration](#compatibility-and-configuration).
- Test delayed CA injection, immediate server consumption, and concurrent partial writes against their contracts above.
- Monitoring automation needs updated reasons and counter semantics. Image tool removal requires the independent
  verifier and checks of every image variant. Ship tests, documentation, and release notes together.
- A mismatched replacement Pod is the case in [version skew](#version-skew-strategy). It is not repaired in place.

### Drawbacks

Scope exceeds a runtime-only rewrite: preflight, Job terminal handling, metrics, and test infrastructure change.
Content verification needs retained fixtures, and transfer completion cannot prove downstream ingestion. There is no
in-release switch back to the shell uploader. Rollback is the downgrade under [Disabling the Feature](#disabling-the-feature).

## Alternatives (Not Implemented)

- **Smallest change on the current path:** fix the known upload failures and leave preflight and upload as two
  implementations. Smaller blast radius, and the next routing or trust change can diverge again. Rejected: both
  callers need one testable policy.
- **Separate Go clients:** each caller keeps its own dial timing, and routing or trust can drift between them.
  Rejected for one package with input adapters. The preflight dial limit stays on preflight only.
- **Dual-runtime flag:** an in-release rollback that keeps two production paths. Rejected. Rollback is the
  downgrade in Drawbacks.
- **In-process obfuscation or changed gather gating:** unsafe library exits and a new success gate are separate work.
- **Direct archive streaming:** a failed upload leaves nothing local to compare. Rejected: the local archive stays
  until the success contract passes.

## Test Plan

### Unit Tests

Cover the following behaviors in deterministic unit tests:

- **Routing/auth:** no proxy, HTTP only, HTTPS only, both proxies, conflicting cases, exclusions, localhost and
  loopback, addresses and ports, encoded userinfo, scheme-less inputs, omitted and explicit ports, malformed URLs,
  redaction, permitted Digest, and prohibited Digest, under [Shared Connection Policy](#shared-connection-policy).
- **TLS/CONNECT:** identical PEM through both adapters, system roots when unconfigured, delayed empty or missing
  managed bundles, invalid static bundles, trusted and untrusted chains, wrong hostname, prohibited fallback,
  preserved CONNECT bytes including an early SSH banner, and connection cleanup, under
  [trust and readiness](#shared-connection-policy).
- **Timing/lifecycle:** preflight dial limit, gather timeout, exhausted preflight retry, canceled preflight, canceled
  archive creation, and a killed Pod whose replacement reruns gather, under
  [configuration](#compatibility-and-configuration) and [Job lifecycle](#job-lifecycle).
- **Transfer:** open denial, local read failure, write or close errors, disconnect, short writes, and sparse
  failures. Immediate consumption and apparent remote length follow [transfer and pipelining](#transfer-and-pipelining).
- **Archive:** semantic fixtures for the [archive outcome](#archive-creation), including special files, unreadable or
  changing inputs, and finalization faults.
- **Obfuscation/environment:** child exit and cancellation, log persistence, three modes, and empty versus partial
  credentials, under [obfuscation](#obfuscation).
- **Controller/metrics:** terminal Job conditions, a completed CR, a missing or empty injection-managed bundle, an
  unusable static bundle, `FailureTarget` without `Failed`, one SFTP reason for authentication and connection
  refused, an exhausted preflight timeout, and the counter cases in
  [failure reporting](#failure-reporting-and-metrics).

### Integration / E2E Tests

Use controlled SFTP and HTTP/HTTPS proxy servers with deterministic permission, quota, write/close, metadata, and
disconnect failures. Keep fault injection confined to test environments, separate from live Red Hat smoke checks.

Compare script/Go runs on identical fixture trees: names, normalized content/metadata, paths, and obfuscation logs.
Assert corrected outcomes separately from parity, including failed proxied `put` returning script exit 0/removal versus
Go non-zero/retention.

The independent verifier checks the current attempt's exact Job-recorded filename, including the internal-user
directory, and handles retries and Job-data retention. Retaining servers permit independent size, gzip/tar, and
expected-content checks; stale or corrupt artifacts must fail verification. Expectations follow each fixture's
contents rather than requiring a universal `must-gather.log`. Production needs no added metadata permissions.

Test immediate server consumption and write or close failures under [transfer and pipelining](#transfer-and-pipelining).
Live smoke checks require protocol and Job/CR success under that contract.

Exercise delayed managed-CA readiness followed by successful validation/Job creation, invalid static trust, and mounted
trust loss under normal retry/cancellation rules. Cover all three workflows, with/without PVC and read-only
source PVC, direct/HTTP/HTTPS routes, exclusions, custom CA, and terminal Job status.

Measure small/multi-GB bundles locally and at injected latency against OpenSSH: throughput, CPU, peak RSS, phase/elapsed
time, bounded memory growth, and no material unexplained regression. Validate release-image direct/proxy SSH/TLS/auth
paths on supported FIPS clusters, including a prohibited Digest case. Run
required build/unit/lint/image/CI/QE checks on
production, OpenShift CI, and coverage images, including verifier operation after tool removal and secret-free errors.

## Graduation Criteria

Ship in a normal operator release without a feature gate or opt-in, explicitly documenting behavior changes.
Complete development validation and these release gates before cutover:

- Pass the [test plan](#test-plan).
- Record and review representative resource and throughput results and supported FIPS results; pass required CI and QE checks.
- Remove the production uploader and its upload-only tools, publish the verifier image, and pass its artifact and fault checks.
- Update design, support, monitoring, and upgrade documentation. Release notes are the intentional changes under [configuration](#compatibility-and-configuration).

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

Remove the uploader/helper together with Go runtime/dependency cutover after these gates pass. No production
compatibility launcher remains; an older image used by comparison fixtures is test-only.

## Upgrade / Downgrade Strategy

Upgrade controller and independently configured `OPERATOR_IMAGE` to the matching release. No schema migration;
existing Jobs retain recorded commands/environment. CRs that are already complete keep their recorded status and
conditions, including historical `ValidationFailed`; they are not rewritten. A CR that has not yet created a Job is
validated with this policy on the next reconcile. Document the intentional changes listed under
[configuration](#compatibility-and-configuration) and validate proxy/trust; no new timeout configuration is required.

Downgrade controller/upload image together and restore compatible monitoring. New Jobs
return to script behavior and its known limitations. Existing commands are not rewritten; after aligning configuration,
create a new CR if another attempt is needed. See the skew limitations below: completion/data retention are not
guaranteed across mismatches.

## Version Skew Strategy

Match controller, `OPERATOR_IMAGE`, and verifier releases; common source/repository is insufficient.
Running Pods retain their images, but pending/replacement Pods with mutable tags may resolve another release: older
`/usr/local/bin/upload` commands or newer Go commands can be absent. Immutable refs stabilize recorded Jobs without
providing pinning enforcement or cross-release command guarantees.

Existing Jobs gain no new routing behavior through template updates. Controller preflight and a recorded
Job can use different release policies; alignment applies to new Jobs under matching releases and unchanged inputs.

## Operational Aspects of API Extensions

CR spec, schema, immutability, and existing timeout behavior stay unchanged. Operational changes are terminal handling
and [status/metric contracts](#failure-reporting-and-metrics). Release notes are the intentional changes under
[configuration](#compatibility-and-configuration). Metrics never prove remote retention or processing.

## Support Procedures

### Detecting Issues

Use `status.reason`, `ReconcileError` (`PreflightValidationFailed` or `JobFailed`), Job terminal state, and operator
or upload logs. Pending CA readiness has no terminal condition. For artifact inspection set existing
`retainResourcesOnCompletion` before CR creation and observe the [workspace lifetime limits](#transfer-and-pipelining).

### Troubleshooting

1. Inspect `status.reason`, the `ReconcileError` reason, and the failing container or operator log. Verify credential
   Secret keys and that controller and `OPERATOR_IMAGE` match.
2. Verify proxy routing, exclusions, ports, and authentication under the [connection policy](#shared-connection-policy)
   without exposing credentials.
3. For HTTPS, inspect certificate names and effective CR-namespace/mounted trust. Distinguish CNO readiness from invalid
   static configuration; never use insecure fallback.
4. Follow [transfer and pipelining](#transfer-and-pipelining) for success, partial remote files, and ingestion.
   Disappearance alone is not a reason to retry.
5. For collection timeouts inspect `spec.mustGatherTimeout`. For preflight timeouts follow
   [configuration](#compatibility-and-configuration). Inspect retained Pod or PVC artifacts under the
   [workspace lifetime limits](#transfer-and-pipelining).

### Disabling the Feature

No independent disable switch restores Bash. Downgrade controller, image, and monitoring together under the
upgrade/skew rules.

## Infrastructure Needed

Reuse build/CI/QE/live smoke facilities; add the independent verifier image, deterministic SFTP/proxy/TLS fault fixtures,
and reproducible resource/transfer comparison job. No new production service; test credentials/faults stay in tests.

## Implementation History

- 2026-09-30: Initial provisional proposal.
- 2026-10-15: Aligned gather polling and image variants with implementation.
- 2026-10-28: Scoped the earlier revision to functional parity.
- 2026-10-29: Included shared transport, proxy/buffering/timeouts, failure handling, pipelining, verification, and
  diagnostics while preserving artifact/obfuscation compatibility and documenting intentional changes.
- 2026-10-29: Corrected success to acknowledged bytes/remote close without post-upload metadata; defined nonterminal
  CA readiness and corresponding test/release/support requirements.
- 2026-11-06: Clarified upload-owned obfuscation and condensed implementation mechanics while retaining design contracts.
- 2026-11-06: Removed proposed timeout controls and Job deadlines, retaining existing gather and preflight timing behavior.
- 2026-11-06: Stated CA, cancellation, Digest, status, and Job-terminal outcomes once each.
- 2026-11-06: Named every terminal preflight check, kept SFTP as one reason, and recognized a completed CR before the ServiceAccount check.
- 2026-11-06: Condensed implementation mechanics to category-level outcomes. Tests, graduation, and support point at the owning sections.
