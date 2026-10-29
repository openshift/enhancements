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

Replace the Bash upload script (`build/bin/upload`) with a native Go subcommand (`must-gather-operator upload`) that
compresses gathered output and uploads it to the configured SFTP server. Extract a shared Go SFTP client for controller
preflight and upload, using the existing SSH/SFTP dependencies. Retain the existing obfuscation subprocess and remove
the shell transfer utilities from operator images.

This enhancement preserves archive naming and contents, remote paths, obfuscation modes and logs, credential-check
ordering, and the gather/upload container arrangement. It also fixes proxy routing and trust differences, lost CONNECT
tunnel bytes, unbounded network stalls, proxy credential parsing, and proxied transfer false success. Bounded SFTP
pipelining, stronger artifact verification, and clearer failure diagnostics are part of the same delivery.

These fixes intentionally change behavior. The proposal defines the changes below rather than treating inherited defects
as compatibility requirements. The MustGather CRD schema remains unchanged; new timeout settings belong to operator
configuration. Existing condition reasons and error-counter semantics change as documented under Failure Reporting and
Metrics.

## Motivation

The upload path currently uses `sshpass`, OpenSSH `sftp`, Ncat, and a `socat`-based HTTPS proxy helper. Controller
preflight already uses Go SSH/SFTP, but the two implementations disagree about proxy exclusions, TLS to HTTPS proxies,
authentication, and trust. A successful preflight can therefore test a different route from the eventual upload.

The current proxy branch runs interactive SFTP, where a failed `put` can still produce session exit 0 and trigger
archive removal. Preflight can discard an SSH banner buffered with CONNECT headers, and its five-second TCP timeout does
not bound all subsequent protocol setup. Upload has no explicit stall limit. Replacing the runtime provides an
opportunity to fix these problems in shared, testable code.

### User Stories

- As a support engineer, I want an upload to succeed only after the complete archive has been accepted, and I want
  useful diagnostics when it fails.
- As a cluster administrator, I want preflight and upload to apply the same proxy exclusions, authentication, and
  trusted CA configuration.
- As an operator developer, I want one context-aware connection implementation and deterministic tests for connection,
  transfer, and cleanup failures.
- As a QE engineer, I want verification to identify the current operation's exact artifact and check its contents,
  independently of the uploader.

### Goals

1. Implement archive creation and SFTP upload in Go while preserving the bundle format and existing supported
   obfuscation workflows.
2. Use a shared `pkg/sftpclient` package for route resolution, proxy parsing, trust, CONNECT, SSH, and SFTP setup in
   both preflight and upload.
3. Apply verified TLS for HTTPS proxies, honor proxy exclusions, preserve buffered tunnel bytes, and parse credentials
   with `net/url`.
4. Bound connection setup and transfer stalls, propagate cancellation, and add a configurable Job deadline as a final
   safety net.
5. Make remote open, write, close, and verification failures fatal on every route. Retain the local archive on failure
   and delete it only after confirmed transfer success.
6. Enable bounded SFTP write concurrency and validate throughput, resource use, and supported FIPS configurations.
7. Strengthen artifact verification and phase diagnostics, and count terminal preflight failures as well as terminal Job
   failures.
8. Remove production upload scripts and their OS dependencies from all operator image variants while retaining tools
   required for gather polling and image setup.

### Non-Goals

- Changing the gather image contract, gather-completion polling, or the two-container Job arrangement. The current
  `pgrep` loop remains in place.
- Adding upload target types, resumable transfers, atomic remote publication, or upload-only retries within a Pod.
- Adding a MustGather spec field or changing the gather-only meaning of `spec.mustGatherTimeout`.
- Moving obfuscation into the uploader process before the library exposes safely returning APIs and explicit diagnostic
  writers.
- Adding host-key distribution or changing the existing disabled SSH host-key verification policy.
- Providing compatibility launchers, migrating older Job commands, or enforcing image pinning across the cutover.
- MicroShift support, which is outside the operator's supported deployment scope.

## Proposal

Register `must-gather-operator upload` before controller-manager startup, following the existing `obfuscate` subcommand
pattern. Keep upload orchestration in `pkg/mustgatherutil/upload.go`, archive and transfer logic in `pkg/upload`, and
connection policy in `pkg/sftpclient`.

Both callers supply explicit connection options and credentials to the shared client. The transport package has no
dependency on controller-runtime or MustGather API types. The controller reads Kubernetes objects; the uploader reads
its environment and mounted files. Neither caller maintains a separate proxy implementation.

### Workflow Description

1. The controller validates the CR. When SFTP upload is configured, it validates credentials, proxy configuration, and
   the effective trusted CA bundle, then runs preflight through the shared client with the existing maximum of three
   transient-error attempts. Obfuscate-only Jobs do not require SFTP connectivity.
2. The controller creates the Job with the existing gather container and conditional upload container. Jobs with an
   upload/obfuscation container receive the configured whole-Job deadline.
3. The gather container writes its output. The upload container retains the existing `pgrep` polling before invoking the
   Go subcommand. Obfuscate-source Jobs invoke upload directly.
4. The uploader runs obfuscation when enabled, captures both child output streams, finishes the log, and copies it into
   cleaned output, including after child failure.
5. After successful obfuscation, it checks credentials. All credentials empty in obfuscate-only mode still means success
   without archive creation or upload; partial credentials fail.
6. It writes and closes the local tar.gz archive, then connects through the shared SFTP client and transfers the archive
   with bounded concurrent requests.
7. It checks every transfer result and remote close, verifies the remote size against the completed local archive, and
   reports success only after those checks pass.
8. It removes the local archive on success on a best-effort basis. Any earlier failure exits non-zero and skips that
   removal.
9. The controller observes terminal Job conditions and reports completion or phase-specific failure. A network stall can
   fail an attempt and permit Job retry; the whole-Job deadline ends the operation even when retries remain.

### API Extensions

No CRD schema, webhook, upload-target union, or finalizer changes are proposed. `spec.mustGatherTimeout` continues to
limit the gather command only. Timeout controls are operator flags, with values needed by upload recorded in each new
Job's environment.

Condition types remain unchanged, but the `ReconcileError` reason vocabulary expands to distinguish
`PreflightValidationFailed`, `JobFailed`, and `JobDeadlineExceeded`. These observable status changes and the expanded
error-counter meaning require release notes and monitoring updates even though no new schema field is added.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The operator and its gather/upload Jobs run as workloads on the guest cluster. Proxy routing, trusted CA configuration,
and SFTP connectivity use that cluster's configuration. This proposal introduces no management-cluster component or
cross-cluster connection.

#### Standalone Clusters

The operator and Jobs run in the same cluster. The shared connection policy and artifact requirements apply directly,
without any dependency on a hosted control plane.

#### Single-node Deployments or MicroShift

The uploader remains a short-lived container in the existing Job. Use bounded transfer buffers and streaming archive
I/O, and validate resource use on constrained deployments. No new persistent daemon is introduced.

MicroShift support remains outside scope because the must-gather-operator does not ship on MicroShift.

#### OpenShift Kubernetes Engine

This proposal does not change operator availability on OpenShift Kubernetes Engine. Where the operator is deployed, the
same upload, proxy, trust, and timeout requirements apply; no topology-specific service is added.

### Implementation Details/Notes/Constraints

#### Compatibility and Intentional Behavior Changes

Preserve the following behaviors:

- Local filename `${FILENAME_PREFIX}.tar.gz`; external remote path `${caseid}_${FILENAME_PREFIX}.tar.gz`; internal
  remote path `${username}/${caseid}_${FILENAME_PREFIX}.tar.gz`.
- Existing input/workspace defaults, template-provided filename prefix, IPv4/IPv6 target normalization, and
  password-based SFTP authentication.
- Equivalent tar members, contents, relevant metadata, links, and recoverable input-error behavior. Compressed bytes and
  archive member order need not match.
- Obfuscation before credential checks, all-credentials-empty success after obfuscation, partial-credential failure, and
  combined obfuscation logs.
- Existing storage mounts, ServiceAccount restrictions, conditional containers, `restartPolicy: Never`, and
  `backoffLimit: 3`.
- Disabled SSH host-key checking, implemented through `ssh.InsecureIgnoreHostKey()` without a known_hosts file.

The intentional changes are:

1. Upload honors `NO_PROXY` and loopback bypass and accepts uppercase proxy settings through the common resolver.
2. HTTPS proxy connections from preflight use verified TLS and the same configured CA bundle as upload.
3. Both callers retain tunnel bytes read ahead while parsing CONNECT.
4. Proxy URL credentials are decoded correctly, and malformed configuration fails explicitly instead of relying on shell
   parsing.
5. Setup and inactivity limits can fail previously unbounded operations. A whole-Job deadline can stop an otherwise
   progressing long operation.
6. Every transfer failure becomes non-zero on direct and proxied routes; failed uploads no longer enter success cleanup.
7. Remote metadata verification is required before success, so endpoints must allow the post-upload size check.
8. Preflight failures become visible in error metrics and phase-specific condition reasons.

Release notes must identify these changes, the proxy port defaults and authentication support, the new timeout settings,
and the remote metadata permission requirement.

#### Environment and Operator Configuration

Keep the current upload inputs: `username`, `password`, `caseid`, `host`, `internal_user`, `must_gather_output`,
`must_gather_upload`, `FILENAME_PREFIX`, `obfuscate`, and `obfuscate_config`. Credentials continue to use SecretKeyRef
in the CR namespace; no Secret replication is introduced.

The script defaults remain `/must-gather-output` for input and `/must-gather-upload` for workspace; the template
explicitly sets `/must-gather` and `/must-gather-upload`. The template continues to supply `FILENAME_PREFIX`. Proxy
variables retain their names, with the common resolver recognizing both cases.

Add these operator flags with proposed defaults:

- `--sftp-connect-timeout=5s`: TCP establishment, including name resolution, to the target or selected proxy.
- `--sftp-setup-timeout=30s`: protocol setup after TCP connection, covering proxy TLS/CONNECT, SSH authentication, and
  SFTP subsystem initialization together.
- `--sftp-transfer-stall-timeout=5m`: maximum interval without successful SFTP upload progress, plus bounds for remote
  file open, close, and verification operations.
- `--must-gather-job-timeout=24h`: whole-Job safety limit for Jobs containing upload/obfuscation, including gather,
  polling, obfuscation, archive creation, and retries.

All durations must be positive and validated at operator startup. Serialize the first three into `SFTP_CONNECT_TIMEOUT`,
`SFTP_SETUP_TIMEOUT`, and `SFTP_TRANSFER_STALL_TIMEOUT` for the uploader; standalone subcommand invocation uses the same
defaults. Invalid upload settings fail before transfer. Set Job `spec.activeDeadlineSeconds` from the fourth flag,
rounding up to whole seconds and checking representable bounds. Do not put the deadline on the Pod template.

These defaults are proposed starting values and must pass the slow-link and representative-bundle checks before release.
Operators can increase the Job limit for long operations without changing the CR schema. Do not derive it from
`mustGatherTimeout`: gather-only duration does not account for obfuscation, transfer, scheduling, or repeated attempts.

#### Shared Proxy Routing and URL Parsing

Use the repository-pinned `httpproxy.FromEnvironment()` configuration and `ProxyFunc()` in both callers. For the
normalized SFTP target, evaluate a synthetic HTTPS URL first, then HTTP as fallback when no proxy was selected. Both
evaluations must use the same target and exclusion settings. Preserve resolver errors; never fall back to a direct
connection because a selected proxy failed to parse or connect.

The pinned resolver gives non-empty uppercase variables precedence over lowercase ones and honors `NO_PROXY`/`no_proxy`,
including implicit localhost/loopback bypass. Before creating Jobs, the controller must resolve conflicts using that
same configuration and forward its effective values as lowercase variables. Forwarding and selection tests must
demonstrate that both callers choose the same route for unchanged configuration.

Validate the effective configured proxy URLs before constructing `ProxyFunc`; the pinned resolver can ignore parse
errors during initialization. Use `net/url.Parse` and `URL.User.Username()`/`Password()` for userinfo, accepting only
HTTP and HTTPS proxy schemes and a valid host/port. A scheme-less `host[:port]` value is normalized to HTTP, consistent
with the existing preflight environment contract. Reject malformed escapes, invalid ports, and unsupported schemes
without echoing credentials. Require reserved userinfo characters to be percent-encoded where needed; do not promise
that every literal reserved character is valid. Parse userinfo once; do not double-decode a credential when passing the
parsed URL between layers.

Default an omitted HTTP proxy port to 3128 and an omitted HTTPS proxy port to 3129, retaining the existing preflight
defaults and establishing one policy for both callers. An explicit port always wins. Standardize the HTTPS upload
missing-port behavior as an intentional change; do not infer port 443 from the URL scheme. The scheme controls TLS
independently of the variable name: `https_proxy=http://proxy:3128` still means TCP CONNECT.

The existing upload script uses lowercase HTTPS preference and ignores exclusions, so convergence is an intentional
routing change. Preflight success remains a point-in-time connectivity check: environment, credentials, CA contents,
network policy, and endpoint state can still change before the Pod uploads.

#### CONNECT Transport, Authentication, and Trust

For an HTTP proxy, establish TCP and send CONNECT. For an HTTPS proxy, complete a verified TLS handshake before sending
CONNECT or proxy credentials. Set TLS ServerName from the proxy hostname, verify its certificate chain and name, and
never retry in plaintext after a TLS failure.

Support Basic proxy authentication in both callers. Preserve the existing HTTP uploader's Digest capability in the
shared implementation, with bounded challenge handling and no unbounded authentication retries. Prefer Digest when
offered and supported by the active crypto policy; exercise Basic-only and Digest-only fixtures. Digest algorithms
prohibited by FIPS policy must fail clearly rather than bypass policy. The existing Ncat client supports both schemes,
so a Basic-only replacement would be an additional compatibility break. See the [Ncat proxy
guide](https://nmap.org/ncat/guide/ncat-proxy.html).

Construct Basic credentials from decoded userinfo and base64-encode once. Basic credentials on an HTTP proxy connection
are still plaintext at the proxy transport layer; only an HTTPS proxy URL adds TLS to that hop. SSH encryption begins
inside the established tunnel and does not protect preceding CONNECT headers. Do not log userinfo, authorization
headers, passwords, or raw errors containing them.

When `--trusted-ca-configmap` is configured, read `ca-bundle.crt` from the effective ConfigMap in the CR namespace after
the existing replication step. Preflight supplies those bytes in memory. Upload reads the mounted copy using a
template-provided `SFTP_CA_BUNDLE_FILE=/etc/pki/tls/certs/ca-bundle.crt`. Both use the same cert-pool builder: the
explicit bundle is the trust source, and missing, empty, or unusable configured data is an error. Without a configured
bundle, both use system roots from the release image. No operator CA volume mount is required for in-memory preflight
loading.

Reading the effective mounted ConfigMap, rather than independently reading a possibly newer source copy, aligns trust
inputs without promising immutable CA snapshots or changing replication ownership rules. Test both adapters with
identical PEM data and require the configured bundle to include every root needed by the selected HTTPS proxy.

Parse CONNECT responses with a retained `bufio.Reader`. Return a `net.Conn` wrapper whose reads continue through that
reader and whose writes, deadlines, addresses, and close delegate to the underlying connection. An SSH banner received
with the CONNECT response must remain available to the SSH handshake. On rejection or parse failure, close the
connection and any response resources without masking the primary error; on success, do not close a response body in a
way that consumes or closes the tunnel.

#### Timeout and Cancellation Policy

Use the earlier of caller cancellation/deadline and each configured phase bound. A TCP dial timeout alone is
insufficient: [`ssh.ClientConfig.Timeout`](https://pkg.go.dev/golang.org/x/crypto/ssh#ClientConfig) governs TCP
establishment, not `ssh.NewClientConn` on an existing connection.

After dialing, install the setup deadline on the underlying connection and use context-aware TLS handshake. Keep the
same setup budget through CONNECT, SSH, and SFTP initialization; check deadline-setting errors. Clear the setup deadline
when setup succeeds and switch to transfer operation limits. Cancellation must close the active connection so blocked
SSH/SFTP operations return, and all cancellation callbacks and watchdog goroutines must be stopped on completion.

Use a sliding connection I/O deadline to detect transport silence, refreshing it after successful reads or writes. Also
track successful SFTP write acknowledgements: unrelated SSH traffic, keepalives, or rekeying must not keep a stuck
upload alive indefinitely. The bounded `WriteAt` workers described below refresh upload progress only after successful
remote responses. Reading bytes from the local archive is not proof of remote progress.

Bound remote open, close, and Stat separately by the stall interval. If no acknowledged upload progress occurs within
that interval, close the connection, fail the upload, and keep the archive. There is no fixed per-transfer duration
limit below the Job ceiling: a slow transfer that continues to make progress may exceed the stall interval in total
duration. Connection I/O deadlines follow the [Go net.Conn contract](https://pkg.go.dev/net#Conn).

Preflight retains at most three attempts for transient errors; authentication, invalid configuration, and certificate
verification failures are not transient. Do not retry a canceled parent context. Preflight does not run the transfer
watchdog because it does not transfer a file.

The uploader uses a signal-aware context and `exec.CommandContext` for obfuscation. Network inactivity limits do not
apply to CPU or filesystem work during obfuscation or archiving. Those phases observe cancellation where supported and
are ultimately bounded by the Job deadline. Log finalization after child cancellation is best effort within the Pod
termination grace period.

#### Job Deadline and Terminal Status

Keep gather's shell timeout and polling unchanged. Add Job-level `activeDeadlineSeconds` only when upload/obfuscation is
present. The deadline covers the Job across attempts, so it can stop progressing work and prevents additional retries
once exceeded. This is a safety policy, not an upload-duration estimate. Kubernetes reports this as a failed Job with
reason `DeadlineExceeded`; see [Job termination and
deadlines](https://kubernetes.io/docs/concepts/workloads/controllers/job/#job-termination-and-cleanup).

Update reconciliation to recognize a true terminal Job `Failed` condition, including `DeadlineExceeded`, even when
`Status.Failed <= backoffLimit`. Use terminal `Complete` for success and retain compatibility with existing count-based
observations as needed. Do not treat an individual failed Pod as terminal while the Job can retry. Wait for terminal Job
state rather than cleaning up solely on the early `FailureTarget` condition.

Report `JobDeadlineExceeded` for deadline termination and `JobFailed` for other terminal Job failures. Preserve one CR
per operation and existing retention behavior. Tests must cover deadline failure with retries remaining, condition-only
status updates, and repeated reconciliation during cleanup.

#### Archive Handling

Use `archive/tar` and `compress/gzip` to write a local archive before connecting. Preserve GNU `tar
--ignore-failed-read` semantics: warn and skip unreadable inputs before header emission; after a header declares a size,
warn and zero-fill any unreadable remainder before continuing to later members. If a file grows, archive the declared
size and warn. Keep source-read failures distinct from archive destination-write failures.

Strip the leading slash from absolute input member names as GNU tar does. Preserve relevant permissions, ownership
metadata, timestamps, hard-link relationships, symlink targets including dangling links, and FIFO headers without
opening FIFOs for data. Compare semantic members against GNU tar fixtures; do not require identical gzip headers or
ordering.

Check tar close, gzip close, any buffered flush, and destination-file close, preserving the primary error while
attempting remaining cleanup. A destination or finalization failure prevents upload. Stream file data and recovery
padding rather than buffering whole files. Preserve existing workspace mount behavior, including the PVC-backed
obfuscation workspace where configured.

#### Transfer, Verification, and Cleanup

Open the remote path with write-only create/truncate semantics. Use the completed local archive's size as the expected
byte count. Every local read, remote open/write/close, cancellation, or timeout error is fatal on both direct and
proxied routes. Check each write result, total acknowledged bytes, and transfer error. Preserve the first error and
include secondary cleanup failures in sanitized diagnostics.

After successful transfer and remote close, call Stat on the exact remote path and require its size to equal the
expected positive local size. Missing, inaccessible, zero-length, or mismatched metadata fails verification. This is
stronger than a non-zero check, but size equality does not prove content integrity, detect all sparse-file holes, or
establish durable storage. Server acknowledgements and close are part of the success criteria; controlled tests
additionally download and compare content.

Confirm post-upload Stat support and permissions on the supported Red Hat endpoint before release. A write-only or
immediately consumed dropbox may not allow this contract. If the endpoint cannot support it, revise the verification
contract in this EP before implementation is accepted; do not silently skip verification or ship perpetual failures
after successful writes.

Delete the local archive only after all transfer and verification checks pass, and keep removal best effort as today.
Failed obfuscation occurs before archive creation; preserve its logs and any existing workspace artifacts rather than
claiming that a new archive exists. On transfer failure, leave the local archive and report the failure without a
success message. Partial remote files can remain and are not valid completed artifacts; automatic remote deletion or
rename is outside scope.

Archive retention refers to the current workspace. An emptyDir archive is lost when its Pod is removed, and default
completion cleanup can remove failed Pods. PVC-backed artifacts follow their volume lifetime.
`retainResourcesOnCompletion` enables post-completion inspection but does not make emptyDir durable across Pod
replacement. Job retry still reruns gather when present, with no resumable upload or extra in-Pod retry loop.

#### Bounded SFTP Pipelining and Resource Validation

Use up to 64 workers issuing `sftp.File.WriteAt` on distinct offsets in chunks no larger than 32 KiB. Each successful
chunk call returns after its SFTP response and can refresh acknowledged progress. Bound both the worker count and buffer
pool, stop producing work on the first failure, close the connection when needed to unblock workers, and join workers
before closing the remote file. Never reuse a chunk buffer until its call finishes.

Keep `sftp.MaxPacketChecked(32768)` and `sftp.MaxConcurrentRequestsPerFile(64)` explicit, and enable
`sftp.UseConcurrentWrites(true)` for any larger-write path. The worker pool itself enforces the 64-request ceiling for
these single-packet WriteAt calls. Use the checked packet-size option; an unchecked limit is unnecessary at 32 KiB. The
pinned [SFTP client API](https://pkg.go.dev/github.com/pkg/sftp@v1.13.9) documents the available controls and
concurrent-write error risks.

`ReadFromWithConcurrency` is a simpler alternative for pipelining, but the pinned version does not expose
per-acknowledgement progress. Choose the bounded WriteAt adapter so the watchdog can distinguish remote progress from
unrelated connection traffic without modifying the dependency. Do not combine both scheduling mechanisms and multiply
concurrency. Test returned errors and short writes; no library call makes silent failure impossible unless its result is
checked.

The nominal worker payload allowance is 64 times 32 KiB, or 2 MiB; this is not a bound on process RSS. Producer
buffering, SSH buffers, protocol bookkeeping, gzip, Go runtime, and the obfuscation child add memory. Measure peak
memory, CPU, throughput, and elapsed time on representative bundles and latency against the script baseline. Do not
claim an unmeasured speedup or fixed slowdown for sequential writes.

Any concurrent-write error fails the transfer regardless of the apparent remote length: later writes can leave holes
after earlier failures. A failed write must never be overridden by a later successful Stat. Test out-of-order responses,
partial failures, error propagation, and watchdog progress. Validate direct and supported HTTP/HTTPS proxy paths with
the release Go build in FIPS-enabled clusters, without enabling algorithms forbidden by policy. This validates the
release path; it does not establish a new certification claim.

#### Obfuscation

Retain `/usr/local/bin/must-gather-operator obfuscate` as a child process with the existing `--input`, `--output`,
optional `--config`, and `-v=3` arguments. The current library can terminate its process through `klog.Exitf`; direct
in-process use would prevent uploader cleanup and log handling on those paths.

Use `exec.CommandContext` without a shell. Send both child output streams to the same synchronized tee-style writer
targeting console stdout and `${must_gather_upload}/obfuscation.log`. Wait for child exit and output completion, close
the log, then copy it into `cleaned/obfuscation.log` on both successful and failed child exits. Preserve the child
failure status when available and never continue to archive/upload after obfuscation failure. Handle log setup, write,
and copy failures explicitly; do not change parent global klog routing.

Keep all three modes: gather/obfuscate/upload, obfuscate-only with all credentials empty, and obfuscation from a
read-only source PVC. Credential checks stay after successful obfuscation. The transfer stall watchdog starts only for
transfer and does not impose a five-minute obfuscation limit.

#### Failure Reporting and Metrics

Use existing `ReconcileError` condition type with `PreflightValidationFailed` for terminal pre-Job validation failure,
`JobFailed` for terminal workload failure, and `JobDeadlineExceeded` for the Job deadline. Keep messages actionable and
sanitized, for example: "Unable to validate SFTP connectivity; verify credentials, proxy settings, and trusted CA
configuration, then check operator logs."

Record protocol phase and classified cause in sanitized logs. Preflight failures belong in operator logs; runtime
failures belong in upload logs. Never expose passwords, proxy authorization, credential-bearing URLs, or raw network
details in CR status/events. Avoid dumping raw library errors into logs when they can include secrets. A generic Job
failure does not reliably identify which container failed; use container logs for that detail.

Expand `must_gather_operator_must_gather_errors` to count terminal failed operations, including preflight validation
failure. Add `must_gather_operator_must_gather_validation_errors` as the preflight subset. Keep
`must_gather_operator_must_gather_total` as successful Job creation count; errors can therefore exceed that denominator.
Update metric help text, documentation, dashboards, and alerts so they do not interpret this ratio as a failure
percentage of created Jobs.

Count at the persisted transition to terminal failure rather than every validation attempt, Pod retry, or cleanup retry.
Reconciliation must recognize an already completed CR, finish any pending cleanup without recreating its Job, and avoid
repeating terminal metrics or events. Test status-write conflicts and repeated events. These process-local Prometheus
counters are not an exactly-once durable ledger across operator crashes; document their reset and crash-window
limitations.

#### Package Layout and Test Seams

```text
pkg/sftpclient/
  client.go, host.go      # TCP, SSH, SFTP lifecycle and host normalization
  proxy.go, trust.go      # Common resolver, auth, TLS, CONNECT, CA pool
  timeout.go, errors.go   # Phase bounds, cancellation, classified errors
  *_test.go              # Same connection-policy suite for both adapters

pkg/upload/
  archive.go             # Streaming tar.gz with input-error recovery
  transfer.go            # Concurrent writes, progress, verification, cleanup
  *_test.go              # Archive fixtures and deterministic transfer faults

pkg/mustgatherutil/
  obfuscate.go           # Existing child entrypoint
  upload.go              # Environment, subprocess, archive, transfer orchestration
  upload_test.go
```

Use injectable dial, clock, SSH/SFTP, remote-file, and child-command interfaces rather than controller package globals
as shared package state. Keep existing controller test seams at its adapter boundary as appropriate. The transfer seam
must expose acknowledged remote progress, errors, close, and Stat; the connection seam must make cancellation and
resource ownership testable.

#### Entrypoint, Job Template, and Packaging

Dispatch `upload` in `main.go` before manager initialization. Change the final command in `uploadCommand` and
`uploadCommandDirect` to `/usr/local/bin/must-gather-operator upload`. Preserve the existing outer `pgrep` loop and
shared process namespace. Remove only the obsolete SSH directory/known_hosts setup prefix, and add the timeout and CA
environment plus Job-level deadline described above.

Remove `build/bin/upload`, `build/bin/https-proxy-connect-util`, and their directory-copy steps from `build/Dockerfile`,
`Dockerfile.openshift`, and `images/ci/Dockerfile.coverage`. Keep the operator binary, obfuscation config, and
coverage-specific setup. Stop installing `sshpass`, `nc`/nmap-ncat, `openssh-clients`, `wget`, `tar`, and `gzip` for
upload. Retain `procps` for `pgrep`, `shadow-utils` for image setup, and coverage `findutils`. TLS-to-proxy moves to Go
and no longer requires the helper or `socat`.

Check the resulting image package inventory rather than assuming that removing an install command removes packages
inherited from a base image. The gather image is separate and retains its own tools.

Move `verifySFTPUpload` to a test-only verification image with an independent client and required archive tools. Build
and publish that image through CI. Its route, trust, and authentication inputs must match the documented policy,
including exclusions; do not retain the old script's parsing and routing defects in the verifier. Keep client
implementation independent from production so it can detect production regressions.

#### Design Documentation Updates Required with Implementation

This EP proposes future behavior. Update the following living design documents with the implementation, without
rewriting accepted ADR history:

- [Upload and validation design](https://github.com/openshift/must-gather-operator/blob/master/docs/design/upload.md):
  shared transport, routing/authentication, trust adapters, buffered CONNECT, deadlines, transfer success and cleanup,
  and obfuscation subprocess.
- [Job template design](https://github.com/openshift/must-gather-operator/blob/master/docs/design/job-template.md): Go
  invocation, environment, whole-Job deadline alongside gather timeout, and removal of SSH filesystem setup.
- [Controller design](https://github.com/openshift/must-gather-operator/blob/master/docs/design/controller.md): terminal
  Job conditions, validation trust input, completed-CR handling, failure reasons, and metric transitions.
- [API design](https://github.com/openshift/must-gather-operator/blob/master/docs/design/api.md): observable status
  reason contract and unchanged spec schema/immutability. Update domain and architecture guidance to match the shipped
  behavior.

The [two-container
ADR](https://github.com/openshift/must-gather-operator/blob/master/docs/adrs/adr-0002-two-container-job.md) remains
applicable. Gather-completion redesign remains independent; this enhancement retains the existing polling and does not
introduce a marker/PID mechanism or success gate. If watch or predicate behavior changes during implementation, update
the corresponding design document and tests as well.

### Risks and Mitigations

- **Route changes:** honoring exclusions can bypass a proxy previously used by upload. Test both callers against the
  same routing matrix and document the upgrade effect.
- **Authentication and trust:** TLS verification, proper URL decoding, and common port defaults can expose previously
  accepted configuration mistakes. Cover custom roots, hostname rejection, Basic/Digest behavior, FIPS constraints, and
  credential redaction.
- **Timeouts:** defaults can terminate a legitimate slow phase or entire operation. Exercise slow progress, large
  bundles, obfuscation, and Job retries; publish the configuration controls and measured evidence.
- **Verification permissions:** a remote dropbox may deny Stat or download. Confirm production Stat support; use
  controlled endpoints for content verification and fail the release gate if the production success contract is
  unsupported.
- **Partial data:** a failed concurrent transfer can leave a partial remote file. Never report success after write/close
  errors and retain the local archive within its actual storage lifetime.
- **Monitoring changes:** expanded counters and new condition reasons can affect alerts and automation. Ship updated
  descriptions, tests, and release notes together.
- **Packaging:** removed operator tools can break E2E checks. Supply an independent verifier image and validate all
  image variants before cutover.

### Drawbacks

The change is larger than a runtime-only rewrite and touches preflight, Job failure recognition, metrics, and test
infrastructure. Connection fixes must remain shared and intentional differences must be tested explicitly, so the wider
scope does not hide unrelated regressions. Remote metadata verification adds a round trip and a server permission
requirement.

## Alternatives (Not Implemented)

### Strict Parity Followed by Separate Fixes

This would retain divergent proxy routes, unbounded stalls, and the known proxied false-success cleanup bug through the
runtime replacement. The selected scope fixes these defects together and preserves compatibility only for behavior not
intentionally changed.

### Separate Go Connection Implementations

Keeping distinct preflight and uploader dialers would allow routing, TLS, parsing, and timeout policy to diverge again.
Both callers instead use one package with adapters for Kubernetes and filesystem inputs.

### Thin Wrapper Around OpenSSH or a Dual-Runtime Feature Flag

Retaining shell transfer tools would defeat the dependency removal and require two production behavior paths. Cut over
once compatibility and changed-behavior tests pass. Keep the obfuscation child because it contains an existing
process-exit boundary rather than an external transfer tool dependency.

### Fixed Upload Duration or a Deadline Derived Only from Gather Timeout

A fixed per-transfer duration penalizes slow uploads that are making progress, while gather timeout does not estimate
obfuscation or transfer. Use setup bounds, acknowledged-progress stall detection, and an independently configured
whole-Job safety ceiling.

### In-Process Obfuscation or Changed Gather Gating

The current obfuscation library can exit the process. Keep it as a child. Init-container restructuring and new gather
success gating also change existing lifecycle semantics and remain separate work.

### Direct Archive Streaming to SFTP

Keeping a completed local archive separates archive failures from transfer failures and permits inspection or size
comparison. Direct streaming would change staging and cleanup semantics and is outside this enhancement.

## Decisions and Release Prerequisites

1. **Scope:** preserve the bundle and obfuscation contract while delivering the explicitly listed connection, transfer,
   timeout, performance, verification, and diagnostic fixes.
2. **Proxy policy:** both callers use the shared resolver and verified CONNECT implementation, including Basic and
   supported Digest authentication. Test the pinned dependency behavior rather than assuming current upstream defaults.
3. **Timeout configuration:** propose operator-level defaults of 5s connect, 30s setup, 5m stall, and 24h whole Job.
   Validate the defaults against representative workloads; no CR field is added.
4. **Remote verification:** require exact size after successful writes and close. Production endpoint Stat support is a
   release prerequisite; unsupported metadata access requires an explicit proposal revision.
5. **Obfuscation:** retain the subprocess, combined diagnostics, and credential ordering. No parent global logging
   changes.
6. **Cutover:** remove production scripts together with the Go runtime and verifier image. Older Job command
   compatibility remains unsupported.

## Test Plan

### Unit Tests

- **Shared routing:** no proxy, HTTP-only, HTTPS-only, both, conflicting variable cases, exclusions, localhost/loopback,
  IPv4/IPv6 and target ports. Assert adapter and shared policy agreement, including template forwarding.
- **Proxy parsing/authentication:** encoded delimiters, passwords containing colons, encoded `@`, scheme-less inputs,
  omitted/explicit ports, invalid URLs, unsupported schemes, Basic/Digest challenges, rejected credentials, and secret
  redaction.
- **TLS/trust:** identical CA input through both adapters, system roots without custom configuration, missing/invalid
  bundles, trusted/untrusted certificates, wrong proxy name, and no plaintext fallback after TLS failure.
- **CONNECT buffering:** response and SSH banner in one read, split headers, rejected responses, and ownership/cleanup
  of all connections.
- **Timeouts/cancellation:** each setup layer stalling, deadline clearing, earlier parent deadlines, cancellation during
  blocked I/O, and no leaked watchdogs. Use injected clocks instead of multi-minute sleeps.
- **Transfer progress:** slow acknowledged progress beyond five minutes of total duration succeeds; absent
  acknowledgements time out despite unrelated SSH traffic. Cover out-of-order concurrent responses and bounded
  open/close/Stat.
- **Transfer outcomes:** remote open denial, local read failure, disk-full/write/close errors, disconnect, short writes,
  missing/mismatched Stat, and concurrent sparse-file failure. Every error must be non-zero on all routes, retain the
  archive, and omit the success message.
- **Archives:** compare GNU tar semantic fixtures, including gather/cleaned roots, long paths, metadata, hard/dangling
  links, FIFO, unreadable/deleted files, short reads, growth, and finalization/destination failures.
- **Obfuscation/environment:** command arguments, combined output, child exit and cancellation, log completion/copy on
  failure, all three modes, defaults, all-empty/partial credentials, and credential-check ordering.
- **Controller/template/metrics:** deadline fields and configuration validation, Job terminal failure with retries
  remaining, completed CRs without Job recreation, sanitized reasons, terminal transition counting, status-update
  conflicts, and no repeated counts/events on cleanup retry.

### Integration / E2E Tests

Use controlled SFTP and HTTP/HTTPS proxy fixtures with test credentials and trusted certificates. Provide deterministic
remote read-only, write/quota, close, metadata, and disconnect fault controls. A tiny volume or random server kill alone
is not a reliable oracle for every fault; use an owned server implementation or test hooks where needed. Keep these
tests separate from live Red Hat endpoint smoke checks.

Run script and Go uploads against the same fixture tree and compare archive names, normalized contents/metadata, remote
paths, and obfuscation logs. For intentional changes, assert the corrected Go outcome separately: for example, the
script's proxied failed `put` may exit 0 and delete its archive, whereas Go must fail and retain it. Do not require
baseline equality for behaviors this EP changes.

Strengthen `verifySFTPUpload` to take the current attempt's expected filename from its Job `FILENAME_PREFIX`, including
the internal-user directory. Do not regenerate a name or match a case-ID substring. Account for retry attempts and
inspect retained Job data when default cleanup would remove it.

On controlled endpoints, require exact filename and local/remote size equality, download the artifact, validate
gzip/tar, and compare expected members and contents. Use fixture-specific expectations rather than assuming every custom
gather includes `must-gather.log`. On the live endpoint, check only operations supported by its permissions; never treat
a listing substring as artifact proof. Include stale-artifact and corrupted-archive negative tests.

Exercise normal gather/upload, obfuscate-only, gather/obfuscate/upload with and without PVC, and read-only source PVC
modes. Cover direct, HTTP, HTTPS, exclusion bypass, and custom CA configurations, plus timeout and Job deadline status
propagation.

Measure representative small and multi-GB bundles at local and injected network latency, recording throughput, CPU, peak
RSS, and phase duration against OpenSSH. The release must show bounded memory growth and no material unexplained
transfer regression. Run the release image on supported FIPS-enabled configurations and verify the target
SSH/TLS/authentication paths.

Run required repository build, unit, lint, image, CI, and QE checks. Validate production, OpenShift CI, and coverage
images; confirm the independent verifier works after production tool removal and that invalid configuration never leaks
credentials.

## Graduation Criteria

This ships in a normal operator release without a feature gate. It includes observable behavior changes and must not be
described as a transparent parity-only refactor.

### Dev Preview -> Tech Preview

No separate preview phase is proposed. Development validation must complete before the unconditional cutover.

### Tech Preview -> GA

- Preserved artifact and obfuscation compatibility tests pass, and every intentional change has a positive and negative
  regression test.
- Preflight and upload pass the same route, authentication, HTTPS trust, buffering, timeout, and cancellation suite.
- Slow progressing uploads work within the configured Job ceiling; stalled or failed transfers never report success or
  delete the local archive.
- Production endpoint post-upload Stat permissions and exact-size verification are confirmed.
- Job deadline failures complete the CR correctly before backoff exhaustion, with sanitized phase reasons and tested
  metric transitions.
- Representative throughput/resource results and supported FIPS upload/proxy validation are recorded and reviewed.
- The verifier checks operation-specific artifacts and controlled archive contents, with deterministic fault coverage.
- Production scripts and upload package installs are removed consistently from all three images; the test verifier image
  is built/published and E2E checks pass.
- Matching design, configuration, support, monitoring, and upgrade documentation are updated with the implementation.
  Required CI/QE checks pass.

### Removing a deprecated feature

Remove the Bash uploader and HTTPS helper in the cutover change after the above checks pass. Keep no production
compatibility launcher. Test-only comparison fixtures can invoke an older release image; its tools are not dependencies
of the new operator runtime.

## Upgrade / Downgrade Strategy

For upgrade, deploy the new controller with `OPERATOR_IMAGE` from the matching release containing the upload subcommand.
This image reference is configured independently of the controller image. No CR schema migration is required. Existing
Jobs keep their recorded command, environment, and deadline settings; this enhancement does not rewrite them.

Document the routing/exclusion change, corrected credential parsing, common proxy defaults, HTTPS certificate
validation, fatal transfer errors, remote Stat requirement, timeout defaults, condition reasons, and metrics. Validate
proxy/trust configuration and adjust the operator Job limit before running workloads that need longer than the proposed
default.

For downgrade, restore the earlier controller and matching upload image together, remove unsupported new operator flags,
and restore compatible monitoring expectations. New Jobs return to script behavior, including its known limitations.
Existing Go Job commands are not rewritten.

An already running Pod retains its image, but a pending or replacement Pod using a mutable tag can resolve another
release. An older Job invoking `/usr/local/bin/upload` can fail after the script is removed; a Go Job can fail against
an image lacking the subcommand. Completion and data retention across such skew are not guaranteed. After aligning
image/configuration, create a new CR for another operation.

## Version Skew Strategy

Sharing source or an image repository does not ensure controller/upload version alignment. Configure matching release
references for the controller, `OPERATOR_IMAGE`, timeout environment, and verifier image. Immutable references can
stabilize recorded Jobs, but pinning enforcement and cross-release command compatibility are outside scope.

Existing Jobs do not gain the new route policy, timeout values, or deadline through a template update. Preflight uses
the running controller's policy while an already recorded Job uses its own runtime/configuration; shared policy
alignment applies to new Jobs created by the matching release under unchanged inputs.

## Operational Aspects of API Extensions

The CR spec schema and immutability are unchanged. Existing condition types gain phase reasons, network failures have
new bounds, Job deadline handling changes, and terminal error metrics include preflight failures. Release notes and
monitoring migration must cover these observable contracts.

The existing creation counter remains a Job count. The error and validation subset counters describe terminal failures
observed by the running operator and can reset on restart. A failed attempt may retry before the CR is terminal; it must
not increment the terminal failure count yet. No metric is a durable proof that the remote service retained or processed
the artifact.

## Support Procedures

### Detecting Issues

Use operator logs for `PreflightValidationFailed`, where no upload Pod may exist. Use upload logs and Job/container
state for runtime failures. `JobDeadlineExceeded` identifies the whole-Job ceiling; phase logs distinguish connection
setup, transfer stall, remote verification, obfuscation, and archive failures.

For archive inspection, set the existing `retainResourcesOnCompletion` before creating the CR and account for emptyDir
versus PVC lifetime. Successful transfer confirms checked protocol operations and size, not remote durable storage or
support-case ingestion.

### Troubleshooting

1. Inspect CR condition reason and Job terminal condition, then the logs for the failing phase.
2. Verify credential Secret keys and the matching controller/upload release configuration.
3. Check effective proxy variables, uppercase precedence, exclusions, target/port, authentication scheme, and the
   decoded credential configuration without printing secrets.
4. For HTTPS proxies, check certificate name and the effective `ca-bundle.crt` in the CR namespace and upload mount. TLS
   verification never falls back to plaintext.
5. For verification failure, check exact remote path, expected byte count, post-upload Stat permissions, and whether the
   endpoint moves or consumes files immediately.
6. For timeout, distinguish TCP establishment, setup, transfer progress, and whole-Job limit. Increase limits only after
   identifying legitimate slow work; a Job deadline stops retries as well as the current attempt.
7. Inspect retained artifacts while the relevant Pod or PVC exists. A retry may rerun gather and leave a partial remote
   artifact; there is no automatic resume.

### Disabling the Feature

The Go runtime replaces the script unconditionally. Configuration can increase timeout limits, but there is no switch
back to the script within this release. Downgrade controller, upload image, operator flags, and monitoring together
subject to the older-Job limitations above.

## Infrastructure Needed

Reuse the repository's build/CI/QE pipeline and live endpoint smoke facilities. Add an independently implemented
verification image, controlled SFTP/proxy/TLS fixtures with deterministic failure hooks, and a reproducible
transfer/resource comparison job. Keep test credentials and failure injection confined to test environments. No new
production service is required.

## Implementation History

- 2026-09-30: Initial enhancement proposal (provisional).
- 2026-10-15: Aligned gather polling and image variants with the current implementation.
- 2026-10-28: Scoped an earlier revision to functional parity.
- 2026-10-29: Expanded the proposal to shared preflight/upload transport and the explicit proxy, buffering, timeout,
  failure handling, pipelining, verification, and diagnostic fixes. Preserved artifact and obfuscation compatibility and
  documented the intentional behavior changes.
