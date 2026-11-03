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
last-updated: 2026-10-29
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
a shared SSH/SFTP client for preflight and upload. Preserve bundle naming/content, obfuscation workflows and logs, and the gather/upload arrangement;
fix proxy/trust divergence, CONNECT buffering, unbounded stalls, credential parsing, and false-success cleanup.
Include bounded pipelining, independent artifact verification, and phase diagnostics. These are intentional behavior
changes, including status reasons and metric semantics; the MustGather CRD schema remains unchanged.

## Motivation

Upload currently uses `sshpass`, OpenSSH `sftp`, Ncat, and a `socat` HTTPS helper, while preflight uses Go. Different
routing, authentication, TLS, and trust implementations let preflight approve a route upload does not use. Interactive
proxied SFTP can return 0 after failed `put` and delete the archive. Preflight can discard buffered SSH banners, its
five-second TCP timeout does not bound protocol setup, and upload has no stall limit. Shared Go code removes shell
transfer dependencies and makes these failures independently testable.

### User Stories

- As a support engineer, I want reliable completion and actionable diagnostics to resolve upload failures.
- As a cluster administrator, I want consistent proxy exclusions, authentication, and CA trust before and during upload.
- As an operator developer, I want one context-aware client with deterministic connection, transfer, and cleanup tests.
- As a QE engineer, I want independent checks of the current operation's exact artifact name and contents.

### Goals

- Preserve bundle and obfuscation compatibility while implementing archive creation and SFTP transfer in Go.
- Share route resolution, URL parsing, TLS/CONNECT, trust, buffering, SSH/SFTP setup, and cancellation between callers.
- Bound setup/stalls and Job duration; require acknowledged transfer completion before cleanup, with bounded write
  concurrency, measured throughput and resource use, and FIPS validation.
- Improve artifact verification and failure reporting, count preflight failures, and remove production shell upload
  dependencies while retaining polling/image tools.

### Non-Goals

- Changes to the gather image contract, `pgrep` polling, two-container arrangement, existing storage mounts, or
  ServiceAccount restrictions.
- New upload targets, resumable transfers, atomic remote publication, automatic remote deletion or rename, or upload-only
  in-Pod retries.
- New MustGather spec fields or changes to the gather-only meaning of `spec.mustGatherTimeout`.
- In-process obfuscation before safely returning library APIs and diagnostic writers exist; host-key distribution or
  changes to disabled SSH host-key verification.
- Older-Job launchers/migrations, image-pinning enforcement, or MicroShift support (the operator does not ship there).

## Proposal

Dispatch `upload` before controller-manager initialization, following the existing `obfuscate` subcommand pattern.
Controller and uploader adapters provide credentials/options and Kubernetes or filesystem inputs to one transport
package, without separate proxy implementations or dependencies on controller-runtime/MustGather API types.
Implementation contracts follow below.

### Workflow Description

1. For SFTP targets, validate the CR, credentials, proxy configuration, and [CA readiness](#shared-connection-policy),
   then run preflight. Pending injection requeues before Job creation; obfuscate-only Jobs need no SFTP connectivity.
2. Create the existing gather and conditional-upload Job with its configured deadline. Gather writes output; upload retains
   the outer `pgrep` wait. Obfuscate-source Jobs skip gather and invoke upload directly.
3. Run [obfuscation](#obfuscation), finish and copy its log even on failure. If it fails, exit non-zero before credential
   checks or archive/upload. Only after success check credentials: all-empty obfuscate-only execution skips
   archive/upload; partial credentials fail.
4. Write/close a local tar.gz, connect, and transfer under [transfer and pipelining](#transfer-and-pipelining).
   Report success and attempt archive removal only after that contract passes; earlier failures exit non-zero.
5. Reconcile terminal Job state into CR status. Stalls may fail an attempt for Job retry; the whole-Job ceiling ends
   the operation even when retries remain.

### API Extensions

No CRD schema, webhook, upload-target union, or finalizer changes. Spec immutability and gather timeout semantics stay
unchanged. New controls are operator flags recorded in new Jobs; existing condition types gain the reasons defined in
[failure reporting and metrics](#failure-reporting-and-metrics). Release notes must cover these observable changes.

### Topology Considerations

Operator availability is unchanged; the same upload/proxy/trust/timeout contracts apply wherever deployed. On hosted
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

Preserve password authentication, IPv4/IPv6 normalization, disabled host-key checking via
`ssh.InsecureIgnoreHostKey()`, shared process namespace, `restartPolicy: Never`, and `backoffLimit: 3`.

- Local filename: `${FILENAME_PREFIX}.tar.gz`; external remote path: `${caseid}_${FILENAME_PREFIX}.tar.gz`;
  internal remote path: `${username}/${caseid}_${FILENAME_PREFIX}.tar.gz`.
- Keep inputs `username`, `password`, `caseid`, `host`, `internal_user`, `must_gather_output`, `must_gather_upload`,
  `FILENAME_PREFIX`, `obfuscate`, `obfuscate_config`, and the proxy variables below. Credentials remain same-namespace
  SecretKeyRef inputs; do not replicate Secrets.
- Script input/workspace defaults remain `/must-gather-output` and `/must-gather-upload`; the template explicitly
  supplies `/must-gather`, `/must-gather-upload`, and `FILENAME_PREFIX`.

Proposed operator flags:

- `--sftp-connect-timeout=5s`: TCP establishment, including DNS, to the target or proxy.
- `--sftp-setup-timeout=30s`: one post-TCP budget for proxy TLS/CONNECT, SSH authentication, and SFTP initialization.
- `--sftp-transfer-stall-timeout=5m`: acknowledged-progress inactivity and separate remote-open/close bounds.
- `--must-gather-job-timeout=24h`: whole Job containing upload or obfuscation, including gather, polling, obfuscation,
  archiving, and retries.

Validate positive durations at startup; invalid uploader settings fail before transfer. Serialize the first three as
`SFTP_CONNECT_TIMEOUT`, `SFTP_SETUP_TIMEOUT`, and `SFTP_TRANSFER_STALL_TIMEOUT`; standalone upload uses the same defaults.
Convert the fourth to Job
`spec.activeDeadlineSeconds`, rounding up to seconds and checking representable bounds; do not set it on the Pod.
Validate proposed defaults against slow links/representative bundles. Operators can increase the ceiling; do not derive
it from gather-only `mustGatherTimeout`, which excludes other phases, scheduling, and repeated attempts.

Intentional changes are shared routing/exclusions, HTTPS preflight trust, retained tunnel bytes, decoded credentials,
new time bounds, fatal transfer errors, nonterminal CA readiness, and expanded failure reporting. Release notes must
identify these changes, proxy ports/authentication, and transfer-success criteria.

#### Shared Connection Policy

**Routing and parsing.** Both callers use the repository-pinned `httpproxy.FromEnvironment()`/`ProxyFunc()` with a
synthetic HTTPS URL for the normalized SFTP target, then HTTP fallback if no proxy is selected. Preserve resolver
errors and the same target/exclusions across evaluations; never bypass a proxy because parsing or connection failed.
Non-empty uppercase variables take precedence over lowercase. Honor `NO_PROXY`/`no_proxy` and implicit
localhost/loopback bypass. The controller forwards effective resolved settings as lowercase Job variables, resolving
case conflicts consistently. This intentionally changes upload's lowercase HTTPS preference and ignored exclusions.

Validate effective URLs before constructing `ProxyFunc`, whose pinned initialization can ignore parse errors. Use
`net/url.Parse` and `URL.User.Username()`/`Password()`; decode once, requiring percent-encoding where reserved userinfo
characters demand it. Allow HTTP/HTTPS and valid host/port; normalize scheme-less `host[:port]` to HTTP. Reject malformed
escapes, invalid ports, and unsupported schemes without leaking credentials. Explicit ports win; defaults are HTTP
3128 and HTTPS 3129, preserving preflight defaults and intentionally standardizing upload's missing HTTPS port.
The URL scheme controls transport, independently of variable name: `https_proxy=http://proxy:3128` means TCP CONNECT,
not TLS or implicit port 443.

**Transport and authentication.** HTTP proxies use TCP CONNECT; HTTPS proxies require verified TLS first, with proxy
hostname as TLS ServerName and chain/name verification. Never retry in plaintext. Support Basic and preserve HTTP
upload's Digest capability in both callers, preferring Digest when offered and crypto policy permits. Bound challenges
and authentication retries; fail prohibited Digest algorithms explicitly without bypassing FIPS policy. Basic-only would
lose [existing Ncat support](https://nmap.org/ncat/guide/ncat-proxy.html).

Build Basic authorization from decoded userinfo and base64-encode once. HTTP CONNECT credentials remain plaintext on
that hop; SSH inside the tunnel does not protect preceding headers. Apply the redaction rules below.

**Trust and readiness.** With `--trusted-ca-configmap`, preflight reads `ca-bundle.crt` from the effective CR-namespace
ConfigMap after existing replication; upload reads its mounted copy through
`SFTP_CA_BUNDLE_FILE=/etc/pki/tls/certs/ca-bundle.crt`. Use one cert-pool builder and that explicit bundle as trust
source. Only when no bundle is configured do both use release-image system roots; preflight needs no CA volume mount.
Using the effective copy aligns inputs without promising immutable snapshots or changing replication ownership. The
bundle must contain every root needed by the selected HTTPS proxy.

The shipped ConfigMap initially has no data and requests CNO injection via
`config.openshift.io/inject-trusted-cabundle="true"`. Missing/empty `ca-bundle.crt` in an injection-managed map is
pending readiness: return/requeue rather than block a reconcile worker, re-read, and start preflight only when usable.
Do not complete the CR, set terminal validation failure, emit terminal-failure events, increment error counters, or
consume the three network-validation attempts. ConfigMap NotFound/transient API reads also retry. Non-empty unusable
PEM, or missing/empty static data without the injection label, is invalid configuration. See
[certificate injection](https://docs.redhat.com/en/documentation/openshift_container_platform/4.12/html/networking/configuring-a-custom-pki).
Unavailable mounted trust after Job creation fails that attempt, retaining any archive under normal cancellation/retry/
Job-deadline rules. Configured trust never falls back to system roots, unverified TLS, or plaintext.

**CONNECT buffering.** Retain the response `bufio.Reader` in a `net.Conn` wrapper: reads drain buffered data, while
writes, deadlines, addresses, and close delegate to the underlying connection. Preserve an SSH banner arriving with
headers. On rejection/parse failure, release connection/response resources without masking the primary error; on
success, do not consume or close the tunnel through response-body cleanup.

Preflight remains point-in-time validation: environment, credentials, CA contents, network policy, and endpoint state
can change before upload.

#### Deadlines and Job Lifecycle

Use the earlier caller deadline/cancellation or phase bound. `ssh.ClientConfig.Timeout` bounds TCP establishment,
not [`ssh.NewClientConn` on an existing connection](https://pkg.go.dev/golang.org/x/crypto/ssh#ClientConfig).
After dialing, set/check the underlying setup deadline and use context-aware TLS handshake. Keep one budget through
CONNECT/SSH/SFTP; clear it after setup. Cancellation closes the connection to unblock operations. Stop all callbacks
and watchdog goroutines on completion.

Refresh sliding I/O deadlines after successful reads/writes, following the [net.Conn contract](https://pkg.go.dev/net#Conn),
and separately track acknowledged SFTP progress. Local reads, keepalives, rekeying, or unrelated SSH traffic cannot
refresh upload progress. Bound remote open and close separately by the stall interval. Expiry closes the connection,
fails the attempt, and retains the archive. Progressing transfers have no fixed total-duration limit below the Job
ceiling. Preflight has no transfer watchdog and retains at most three transient-error attempts; authentication,
invalid configuration, and certificate failures are nontransient. Never retry a canceled parent; CA readiness requeues
precede this network budget.

Use a signal-aware uploader context and `exec.CommandContext` for obfuscation. Network inactivity does not limit CPU/
filesystem work during obfuscation/archiving; those phases observe cancellation where supported and the Job ceiling.
Child-cancellation log finalization is best effort within Pod termination grace.

Set the Job deadline only when upload/obfuscation is present. Preserve gather's shell timeout and polling. The ceiling
covers all attempts, may stop progressing work, and prevents further retries even below backoff exhaustion; it is not an
upload estimate. Recognize true terminal `Failed`, including `DeadlineExceeded` when `Status.Failed <= backoffLimit`,
and terminal `Complete`, retaining count-based compatibility as needed. Neither an individual failed Pod nor early
`FailureTarget` alone permits terminal cleanup. Preserve one CR per operation and retention behavior. See
[Job deadlines and termination](https://kubernetes.io/docs/concepts/workloads/controllers/job/#job-termination-and-cleanup).

#### Archive Creation

Before connecting, stream a local tar.gz with `archive/tar` and `compress/gzip`, matching GNU
`tar --ignore-failed-read`: warn/skip unreadable input before its header; after declaring size, warn/zero-fill any
unreadable remainder so later members stay valid. Archive only declared bytes when files grow, warning about changes.
Distinguish source errors from fatal destination writes.

Strip leading `/` from absolute member names. Preserve contents, relevant permissions/ownership/timestamps, hard-link
relationships, symlink targets including dangling links, and FIFO headers without opening FIFOs. Compare semantic GNU
tar fixtures, not compressed bytes, gzip headers, or entry order. Check tar close, gzip close, buffered flush, and
file close in order; preserve the primary error while attempting cleanup. Destination/finalization errors prevent
upload. Stream recovery padding without whole-file buffering; retain existing workspace/PVC obfuscation mounts.

#### Transfer and Pipelining

Open the remote file write-only with create/truncate semantics. Success requires every write acknowledged without
error, total acknowledged bytes equal to the completed local archive's positive size, and successful remote close.
Local read, remote open/write/close, short write/count mismatch, cancellation, or timeout fails both direct and proxied
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

Use up to 64 `sftp.File.WriteAt` workers on distinct offsets, each chunk at most 32 KiB. Successful calls expose remote
acknowledgements for progress. Bound workers/buffer pool; stop producing on first failure, close the connection when
needed to unblock workers, join before remote close, and reuse buffers only after calls finish. Configure
`sftp.MaxPacketChecked(32768)` and `sftp.MaxConcurrentRequestsPerFile(64)`; enable `sftp.UseConcurrentWrites(true)`
for larger-write paths. The worker pool enforces the single-packet 64-request ceiling; unchecked packet limits are
unnecessary. Do not combine scheduling mechanisms and multiply concurrency.

The pinned [SFTP API](https://pkg.go.dev/github.com/pkg/sftp@v1.13.9) offers `ReadFromWithConcurrency` but no per-ack
progress callback; `WriteAt` avoids dependency changes and distinguishes progress from unrelated traffic. Check returned
errors/short writes. Nominal worker payload is 64 × 32 KiB = 2 MiB, not an RSS bound: producer/SSH/protocol buffers,
gzip, Go runtime, and the obfuscation child add memory. Measure throughput/CPU/peak RSS/time against OpenSSH; do not
assert unmeasured speedups or sequential-write slowdowns. FIPS validation must respect crypto policy and establishes
release compatibility, not new certification.

#### Obfuscation

Keep `/usr/local/bin/must-gather-operator obfuscate --input ... --output ... [--config ...] -v=3` as an
`exec.CommandContext` child without a shell: the library can reach `klog.Exitf`, preventing parent cleanup if called
in-process. Send stdout/stderr to one synchronized tee writer for console stdout and
`${must_gather_upload}/obfuscation.log`. Wait for exit/output completion, close the log, and copy it into
`cleaned/obfuscation.log` on success and failure. Preserve child failure status when available; explicitly handle
log setup/write/copy errors without changing parent global klog routing. Failure never proceeds to archive/upload.

Preserve gather/obfuscate/upload, obfuscate-only, and read-only source-PVC modes. Check credentials after successful
obfuscation: all `caseid`, `username`, and `password` empty means success without archive/upload; partial credentials
fail. The transfer watchdog does not impose a five-minute obfuscation limit.

#### Failure Reporting and Metrics

Keep `ReconcileError` with reasons `PreflightValidationFailed` (terminal pre-Job validation), `JobFailed` (terminal
workload failure), and `JobDeadlineExceeded` (Job ceiling). Use actionable sanitized messages, for example: "Unable to
validate SFTP connectivity; verify credentials, proxy settings, and trusted CA configuration, then check operator logs."
CA readiness follows the nonterminal rules in [connection policy](#shared-connection-policy).

Log classified cause/phase in operator logs for preflight and upload logs for runtime. Never log proxy userinfo,
authorization headers, passwords, or raw errors containing them, at any log level. Keep passwords, authorization,
credential-bearing URLs, and raw network details out of CR status/events; sanitize library errors before logging so
secrets cannot leak. Generic Job failure cannot identify the failed container; inspect container logs.

Count terminal failed operations, including preflight failures, in `must_gather_operator_must_gather_errors`; add
`must_gather_operator_must_gather_validation_errors` as its preflight subset. Keep
`must_gather_operator_must_gather_total` as successful Job creations: errors can exceed this denominator, so their ratio
is not a created-Job failure percentage. Update help text, documentation, dashboards, and alerts.

Count only persisted terminal-failure transitions, not validation attempts, Pod retries, or cleanup retries. Recognize
completed CRs, finish pending cleanup without recreating Jobs, and avoid repeated terminal events/metrics. Counters
reset with the operator and are not an exactly-once durable ledger across crash windows.

#### Integration and Packaging

Use these packages and test seams:

```text
pkg/sftpclient/       client.go, host.go, proxy.go, trust.go, timeout.go, errors.go, *_test.go
pkg/upload/           archive.go, transfer.go, *_test.go
pkg/mustgatherutil/   obfuscate.go, upload.go, upload_test.go
```

The first owns transport lifecycle, host normalization, and connection policy; the second owns archive recovery, transfer,
progress, and cleanup; `pkg/mustgatherutil/upload.go` orchestrates environment, subprocess, archive, and transfer while
`obfuscate.go` remains the child entrypoint. Inject dial, clock, SSH/SFTP, remote-file, and child-command interfaces;
retain controller adapter test seams without making controller globals shared-package state. Expose acknowledged
progress, errors, counts, close, cancellation, and resource ownership for tests; run the same policy suite on both
adapters.

In `main.go`, register upload before manager startup. Change final `uploadCommand`/`uploadCommandDirect` invocation to
`/usr/local/bin/must-gather-operator upload`; retain polling/shared process namespace, remove SSH directory and known_hosts
setup, and add configured timeout/CA environment and Job deadline.

Remove production `build/bin/upload`, `build/bin/https-proxy-connect-util`, and their directory-copy steps in
`build/Dockerfile`, `Dockerfile.openshift`, and `images/ci/Dockerfile.coverage`. Stop installing `sshpass`,
`nc`/nmap-ncat, `openssh-clients`, `wget`, `tar`, and `gzip` for upload; Go TLS also removes the `socat` helper need.
Keep the binary, obfuscation config, coverage setup/`findutils`, `procps` for polling, and `shadow-utils` for image setup.
Check built package inventories because base images may supply removed tools; the separate gather image keeps its tools.

Move `verifySFTPUpload` to a CI-built/published test-only image with an independently implemented client and archive tools.
Match documented routing, exclusions, parsing, authentication, and trust without reproducing the old defects; independent
implementation must detect production regressions.

With implementation, update the living designs: [upload](https://github.com/openshift/must-gather-operator/blob/master/docs/design/upload.md)
(transport/archive/obfuscation), [Job](https://github.com/openshift/must-gather-operator/blob/master/docs/design/job-template.md)
(command/environment/deadline), [controller](https://github.com/openshift/must-gather-operator/blob/master/docs/design/controller.md)
(trust/terminal status/metrics), and [API](https://github.com/openshift/must-gather-operator/blob/master/docs/design/api.md)
(status reasons/unchanged immutable spec), plus domain/architecture guidance. Update predicate/watch design/tests if
changed. Do not rewrite accepted ADR history;
the [two-container ADR](https://github.com/openshift/must-gather-operator/blob/master/docs/adrs/adr-0002-two-container-job.md)
still applies. Gather-completion redesign, marker/PID polling, and a new success gate remain independent.

### Risks and Mitigations

- Route/exclusion, port, decoding, auth, and TLS changes can expose configuration mistakes or bypass a formerly used
  proxy: test both adapters and document upgrade effects.
- Default bounds can terminate legitimate work: measure slow/large/obfuscation/retry cases and publish controls/results.
- Test delayed CA injection, immediate server consumption, and concurrent partial writes against their contracts above.
- Monitoring automation needs updated reasons/counter semantics; image tool removal requires the independent verifier
  and checks of every image variant. Ship tests, documentation, and release notes together.

### Drawbacks

Scope exceeds a runtime-only rewrite: preflight, Job terminal handling, metrics, and test infrastructure change.
Keep fixes shared and deviations explicit to avoid unrelated regressions. Content verification needs retaining fixtures;
transfer completion alone cannot prove downstream ingestion/durability.

## Alternatives (Not Implemented)

- **Strict parity first:** retains divergent routing, stalls, and false-success cleanup; fix these together while
  preserving other behavior.
- **Separate Go clients:** permits renewed routing/TLS/parsing/timeout drift; use one package with input adapters.
- **OpenSSH wrapper or dual-runtime feature flag:** retains removed dependencies and two production paths. Cut over
  after tests pass; obfuscation remains a child solely for its existing process-exit boundary.
- **Fixed upload duration or gather-derived deadline:** penalizes slow progress or ignores other phases; use phase/stall
  limits and an independent Job ceiling.
- **In-process obfuscation or changed gather gating:** unsafe library exits and lifecycle changes require separate work,
  including init-container restructuring and new success gating.
- **Direct archive streaming:** combines archive/transfer failures and changes staging/cleanup; retain the local archive
  for inspection and size comparison.

## Test Plan

### Unit Tests

Use injected clocks instead of multi-minute sleeps. Test:

- **Routing/auth:** no proxy, HTTP only, HTTPS only, both proxies, conflicting cases, exclusions, localhost/loopback, IPv4/IPv6 and ports, and
  consistent adapters and template forwarding. Include encoded delimiters/`@`, colon passwords, scheme-less inputs,
  omitted/explicit ports, malformed/unsupported URLs, Basic/Digest challenges, rejection, and redaction.
- **TLS/CONNECT:** identical PEM through both adapters, system roots when unconfigured, delayed empty/missing managed
  bundles, invalid static bundles and unusable non-empty PEM, trusted/untrusted chains, wrong hostname, and prohibited fallback.
  Cover combined/split CONNECT headers and SSH banner, rejection, and connection/response resource cleanup.
- **Bounds/progress:** stalled setup at each layer, cleared deadlines, earlier parent bounds, blocked-I/O cancellation,
  watchdog leaks, bounded open/close, and out-of-order acknowledgements. Slow progressing transfers beyond five minutes
  total succeed; unrelated SSH traffic without acknowledgements does not prevent expiry.
- **Transfer:** open denial, local read failure, disk-full/write/close errors, disconnect, short writes and byte-count mismatch, and sparse
  failures must be non-zero on all routes, retain the archive, and omit success. Immediate consumption after successful
  writes, byte count, and close must succeed with cleanup and no metadata requirement.
- **Archive:** GNU semantic fixtures for gather/cleaned roots, long paths, metadata, hard/dangling links, FIFO,
  unreadable/deleted inputs, short reads, growth, and destination/finalization faults.
- **Obfuscation/environment:** arguments, combined output, child exit/cancellation, log completion/copy on failure,
  setup/write/copy errors, three modes, defaults, empty/partial credentials, and ordering.
- **Controller/metrics:** configuration/deadline fields; terminal failure before backoff exhaustion; condition-only
  updates; completed CRs without Job recreation; sanitized reasons; persisted-transition counting, status conflicts,
  repeated events, and cleanup retries. CA pending must create no Job, consume zero network-validation attempts, and set
  no terminal status, completion, events, or counters; later population must permit validation/creation.

### Integration / E2E Tests

Use owned SFTP and HTTP/HTTPS/TLS fixtures with test credentials/certificates and deterministic read-only, write/quota,
close, metadata, and disconnect hooks. Tiny volumes/random kills alone are not reliable fault oracles. Keep failure
injection separate from live Red Hat smoke checks and confined to test environments.

Compare script/Go runs on identical fixture trees: names, normalized content/metadata, paths, and obfuscation logs.
Assert corrected outcomes separately from parity, including failed proxied `put` returning script exit 0/removal versus
Go non-zero/retention.

The independent verifier derives the current attempt's exact remote filename from Job `FILENAME_PREFIX`, including
internal-user directory; never regenerate names or match a case-ID substring. Handle retries and retain/inspect Job
data when default cleanup would remove it. Retaining servers must match local and remote size, download and validate gzip/tar,
and compare expected members/content, using fixture-specific expectations rather than universal `must-gather.log`.
Stale-artifact/corruption negatives must fail this verifier without imposing metadata permissions on production.

A separate server consumes/removes files immediately after successful close response: assert success/cleanup/no retry
when writes/counts passed. Conversely, write/close failure remains fatal even with apparent expected file size.
Live smoke checks require protocol and Job/CR success without persistent listing/Stat/download. Record case attachment
separately if an existing supported facility confirms it; neither absence nor a case-ID substring proves failure or
ingestion.

Populate initially empty managed CA data after reconciliation observes it: first no terminal failure/Job/error count,
then successful validation/creation. Test invalid static data separately, and mounted-trust loss after Job creation
under normal failure/cancellation/retry limits. Exercise gather/upload, obfuscate-only, gather/obfuscate/upload with and
without PVC, and read-only source PVC; cover direct/HTTP/HTTPS, exclusions, custom CA, and Job-deadline status.

Measure small/multi-GB bundles locally and at injected latency against OpenSSH: throughput, CPU, peak RSS, phase/elapsed
time, bounded memory growth, and no material unexplained regression. Validate release-image direct/proxy SSH/TLS/auth
paths on supported FIPS clusters without forbidden algorithms. Run required build/unit/lint/image/CI/QE checks on
production, OpenShift CI, and coverage images, including verifier operation after tool removal and secret-free errors.

## Graduation Criteria

Ship unconditionally in a normal operator release without feature gate or opt-in, explicitly documenting behavior changes.
Complete development validation and these release gates before cutover:

- Pass the [test plan](#test-plan): preserved artifacts/obfuscation and positive and negative cases for every intentional
  change, including shared policy/cancellation, slow progress/stalls, immediate consumption, and CA readiness/rejection.
- Confirm terminal deadline handling before backoff exhaustion and sanitized reasons/metric transitions.
- Record and review representative resource/throughput and supported FIPS results; pass required CI/QE checks.
- Remove production scripts/package installs consistently, publish the verifier image, and pass its artifact/fault checks.
- Update design, configuration, support, monitoring, and upgrade documentation with implementation.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

Remove the uploader/helper together with Go runtime/dependency cutover after these gates pass. No production
compatibility launcher remains; an older image used by comparison fixtures is test-only.

## Upgrade / Downgrade Strategy

Upgrade controller and independently configured `OPERATOR_IMAGE` to the matching release. No schema migration;
existing Jobs retain recorded commands/environment/deadlines. Document the intentional changes listed under
[configuration](#compatibility-and-configuration), validate proxy/trust, and increase the Job limit for legitimately
long workloads.

Downgrade controller/upload image together, remove unsupported flags, and restore compatible monitoring. New Jobs
return to script behavior and its known limitations. Existing commands are not rewritten; after aligning configuration,
create a new CR if another attempt is needed. See the skew limitations below: completion/data retention are not
guaranteed across mismatches.

## Version Skew Strategy

Match controller, `OPERATOR_IMAGE`, timeout environment, and verifier releases; common source/repository is insufficient.
Running Pods retain their images, but pending/replacement Pods with mutable tags may resolve another release: older
`/usr/local/bin/upload` commands or newer Go commands can be absent. Immutable refs stabilize recorded Jobs without
providing pinning enforcement or cross-release command guarantees.

Existing Jobs gain no new routing, timeouts, or deadlines through template updates. Controller preflight and a recorded
Job can use different release policies; alignment applies to new Jobs under matching releases and unchanged inputs.

## Operational Aspects of API Extensions

CR spec, schema, and immutability stay unchanged. Phase reasons, time bounds, terminal handling, and failure counters follow
[their contracts](#failure-reporting-and-metrics); ship release notes and monitoring migration. Failed attempts do not
increment terminal counters while the Job can retry. Metrics reset and never prove remote retention/processing.

## Support Procedures

### Detecting Issues

Use CR reasons/Job terminal state, operator preflight logs (possibly no Pod), and upload/container phase logs for setup,
stalls, byte-count/close, obfuscation, and archive faults. Pending CA injection is a nonterminal readiness wait, not bad
credentials/completion. For artifact inspection set existing `retainResourcesOnCompletion` before CR creation and
observe the [workspace lifetime limits](#transfer-and-pipelining). Successful transfer and later SFTP disappearance do
not independently establish downstream case ingestion.

### Troubleshooting

1. Inspect condition/Job reason and failing-phase/container logs; verify credential Secret keys and release alignment.
2. Check effective proxy variable precedence, exclusions, target/port, auth, and decoded credentials without printing
   secrets; follow [connection policy](#shared-connection-policy).
3. For HTTPS, inspect certificate name and effective CR-namespace/mounted `ca-bundle.crt`. Check CNO readiness for
   managed empty data or repair static configuration; never use insecure trust fallback.
4. Inspect remote open/write/close and acknowledged bytes. Disappearance after successful close can be normal; use
   retaining fixtures for content checks and supported case tooling for separate ingestion confirmation, not a retry
   triggered only by absence.
5. Distinguish connect/setup/progress/whole-Job limits before increasing them for proven slow work. The Job ceiling also
   stops retries. Inspect retained Pod/PVC artifacts promptly; retries may rerun gather, leave partial remote files, and
   cannot automatically resume.

### Disabling the Feature

No independent disable switch restores Bash. Increase limits through configuration or downgrade controller, image,
flags, and monitoring together under the upgrade/skew rules.

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
