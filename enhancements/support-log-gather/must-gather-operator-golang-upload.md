---
title: must-gather-operator-golang-upload
authors:
  - "@neha037"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2026-09-30
last-updated: 2026-09-30
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/MG-386
see-also:
  - "/enhancements/support-log-gather/must-gather-operator.md"
  - "/enhancements/support-log-gather/operator-upload-targets.md"
  - "/enhancements/support-log-gather/must-gather-bundle-obfuscation.md"
---

# Must-Gather Operator: Go-Based Upload

## Summary

Replace the Bash upload script (`build/bin/upload`) with a native Go subcommand (`must-gather-operator upload`) that compresses gathered output and uploads it to the Red Hat SFTP server. The Go implementation reuses the SSH/SFTP and HTTP-proxy libraries already present in the operator binary for controller-side preflight validation (`golang.org/x/crypto/ssh`, `github.com/pkg/sftp`), eliminating the dependency on `sshpass`, `nc`, and the `https-proxy-connect-util` shell helper (which itself requires `socat` from the base image). This is a behavior-preserving, internal refactoring — the MustGather CRD API, the gather container, and the user-visible CR workflow are unchanged.

## Motivation

The upload phase of a must-gather Job currently runs as a Bash script (`build/bin/upload`) inside the operator image. This script shells out to `sshpass` and `sftp` (from `openssh-clients`) for SFTP upload, `nc` (nmap-ncat) for HTTP proxy tunneling, and the `https-proxy-connect-util` helper (which requires `socat` from the base image) for HTTPS proxy tunneling. At the same time, the controller's preflight validation (`controllers/mustgather/validation.go`) implements the same SFTP-connection, proxy-dialing, and host-normalization logic in Go using `golang.org/x/crypto/ssh` and `github.com/pkg/sftp`. The two implementations have no shared code, creating a drift risk: the controller may approve a connection that the script later handles differently (or vice-versa), particularly around proxy authentication, IPv6 bracket formatting, and error classification.

A specific example of this drift risk: the Bash script reads lowercase proxy env vars (`http_proxy`, `https_proxy`) and picks `https_proxy` first via shell default expansion (`${https_proxy:-${http_proxy}}`). The Go controller uses `httpproxy.FromEnvironment()` which reads both uppercase and lowercase variants and selects based on the request URL scheme. The Job template (`template.go`) injects lowercase vars. Today these happen to produce the same result, but a code change in either path could silently diverge.

The Bash script is also difficult to unit-test. Archive naming, internal-vs-external user remote paths, proxy URL parsing, and the obfuscate-without-credentials early-exit are all tested only through end-to-end runs. Moving this logic to Go makes each responsibility independently testable and gives operators structured, actionable error messages instead of raw shell exit codes.

### User Stories

* As a **support engineer**, I want upload failures to produce structured error messages (e.g. "Authentication failed: username or password is incorrect") so that I can guide customers to a fix without reading raw shell output.
* As an **operator developer**, I want the upload path and the controller's preflight validation to share the same SFTP/proxy client library so that a successful preflight check guarantees the upload will use the same connection parameters.
* As a **cluster operator**, I want the operator image to have fewer OS-level dependencies (`sshpass`, `openssh-clients`, `nc`) so that image CVE surface is reduced and I do not need to track patching of those packages.
* As a **QE engineer**, I want to unit-test archive naming, remote path construction, and proxy dial behavior without deploying a full cluster or standing up an SFTP server.

### Goals

1. **Behavioral parity** — the Go upload subcommand must produce identical outcomes to `build/bin/upload` for every supported scenario: standard upload, internal-user upload path, proxy (HTTP and HTTPS) with and without authentication, IPv6 host addresses, obfuscation before upload, obfuscation-only without SFTP credentials (exit 0), and `tar --ignore-failed-read` equivalent behavior.
2. **Shared SFTP/proxy client** — extract SFTP connection, proxy dialing, and host normalization into a shared package (`pkg/upload`) consumed by both the controller preflight and the upload subcommand.
3. **Unit-testable upload** — each upload responsibility (archive creation, remote path construction, SFTP put, proxy dial, obfuscation orchestration) is covered by unit tests with injectable dependencies.
4. **Remove OS-level upload dependencies** — stop installing `sshpass`, `nc` (nmap-ncat), `openssh-clients`, and `wget` in the operator image Dockerfile. With Go handling archiving as well, `tar` and `gzip` are also no longer required by the upload path (the gather container uses a different image). Retain `procps` (needed for the gather-wait `pgrep` loop which remains shell-based) and `shadow-utils`.
5. **Remove SSH filesystem setup from Job template** — the shell prefix that creates `~/.ssh/known_hosts` (`mkdir -p /tmp/must-gather-operator/.ssh; touch known_hosts; chmod ...`) is no longer needed because Go SSH uses `InsecureIgnoreHostKey()` in memory rather than a filesystem `known_hosts` file.

### Non-Goals

* Rewriting the gather container or modifying the must-gather image contract (`/usr/bin/gather` entrypoint, `/must-gather` output directory).
* Replacing the `pgrep`-based gather-wait loop in the upload container command. The shell wrapper that waits for the gather process to finish is retained as-is.
* Adding new upload target types (S3, HTTP, etc.). The union API (`spec.uploadTarget`) already supports extension via ADR-0003; this enhancement only rewrites the existing SFTP upload runtime.
* Changing the MustGather CRD API, field semantics, or validation rules.
* MicroShift support — the must-gather-operator does not ship on MicroShift.

## Proposal

Add a new `upload` subcommand to the `must-gather-operator` binary. The upload container command in the Job template switches from invoking the Bash script (`/usr/local/bin/upload`) to invoking the Go subcommand (`/usr/local/bin/must-gather-operator upload`). The gather-wait shell wrapper that polls via `pgrep` is retained as the outer command; only the final upload invocation changes.

The shared SFTP/proxy code currently in `controllers/mustgather/validation.go` is extracted into a reusable package (`pkg/upload`) that both the controller preflight and the upload subcommand import. This package provides:

- SSH/SFTP client construction with configurable host-key policy
- HTTP CONNECT proxy dialing (HTTP and HTTPS proxies, with and without authentication)
- Host address normalization (IPv6 bracketing, default port 22)
- Structured error classification

The upload subcommand reads the same environment variables that the Bash script reads today (`username`, `password`, `caseid`, `host`, `internal_user`, `http_proxy`, `https_proxy`, `no_proxy`, `must_gather_output`, `must_gather_upload`, `FILENAME_PREFIX`, `obfuscate`, `obfuscate_config`), preserving full backward compatibility with the Job template.

### Workflow Description

The user-facing workflow is unchanged. The internal Job execution changes as follows:

**cluster administrator** creates a MustGather CR with an `uploadTarget` of type SFTP.

1. The controller validates the CR, including SFTP preflight (using the shared `pkg/upload` client).
2. The controller creates a Job with a gather container and an upload container.
3. The gather container runs the must-gather image and writes output to `/must-gather`.
4. The upload container starts, runs the gather-wait shell wrapper (`pgrep` polling), and then invokes `must-gather-operator upload`.
5. The upload subcommand:
   a. Reads environment variables for credentials, host, case ID, proxy settings, and obfuscation flags.
   b. If obfuscation is enabled, runs obfuscation in-process (calling the same `must-gather-clean` library used by the `obfuscate` subcommand).
   c. If obfuscation completed but no SFTP credentials are provided, exits 0 (obfuscate-only mode).
   d. Creates a `.tar.gz` archive of the gather output using `archive/tar` and `compress/gzip`.
   e. Connects to the SFTP server (direct or through HTTP proxy) using the shared `pkg/upload` client.
   f. Uploads the archive to the remote path (`{caseid}_{filename}` or `{username}/{caseid}_{filename}` for internal users).
   g. Removes the local archive on success.
   h. Exits with 0 on success or non-zero on failure, with structured log output.
6. The controller observes Job completion/failure and updates MustGather status.

**Error handling:**

- If SFTP connection fails, the upload subcommand logs a classified error (same classification as `classifySFTPError` in `validation.go`) and exits non-zero.
- If archive creation fails (e.g. disk full), the subcommand logs the error and exits non-zero.
- The Job `backoffLimit: 3` retries the entire Pod on failure (existing behavior, unchanged).

### API Extensions

None. This enhancement does not modify the MustGather CRD, add webhooks, finalizers, or any other API extension. The `api-approvers` field is set to `None`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations. The must-gather-operator runs as a workload on the guest cluster. The upload subcommand operates identically regardless of whether the control plane is hosted or standalone.

#### Standalone Clusters

The change is relevant and transparent. Standalone clusters are the primary deployment model.

#### Single-node Deployments or MicroShift

The change does not affect resource consumption. The upload subcommand runs inside the same operator image that already ships the `obfuscate` subcommand. No new processes or persistent daemons are added.

MicroShift does not ship the must-gather-operator and is out of scope.

#### OpenShift Kubernetes Engine

OKE does not ship the must-gather-operator. No impact.

### Implementation Details/Notes/Constraints

#### Environment Variable Contract

The upload subcommand reads the same environment variables as the current Bash script. This is the full contract:

| Variable | Source | Required | Description |
|---|---|---|---|
| `username` | SecretKeyRef | Yes (unless obfuscate-only) | SFTP username |
| `password` | SecretKeyRef | Yes (unless obfuscate-only) | SFTP password |
| `caseid` | Literal | Yes (unless obfuscate-only) | Red Hat support case ID |
| `host` | Literal | No (default: `sftp.access.redhat.com`) | SFTP server hostname |
| `internal_user` | Literal | No (default: `false`) | Use internal upload path |
| `http_proxy` | Literal | No | HTTP proxy URL |
| `https_proxy` | Literal | No | HTTPS proxy URL |
| `no_proxy` | Literal | No | Proxy exclusion list |
| `must_gather_output` | Literal | No (script default: `/must-gather-output`; template always sets `/must-gather`) | Gather output directory |
| `must_gather_upload` | Literal | No (script default: `/must-gather-upload`; template always sets `/must-gather-upload`) | Upload workspace directory |
| `FILENAME_PREFIX` | Literal | Yes | Archive filename prefix (directory name) |
| `obfuscate` | Literal | No | Enable obfuscation (`true`/`false`) |
| `obfuscate_config` | Literal | No | Path to custom obfuscation config |

#### Behavioral Parity Checklist

The following behaviors of `build/bin/upload` must be replicated exactly:

1. **IPv6 host bracketing** — bare IPv6 addresses (e.g. `2001:db8::1`) are wrapped in brackets before dialing. The shared `normalizeHostAddress` in `validation.go` already handles this.
2. **Internal user remote path** — when `internal_user=true`, the remote filename is `{username}/{caseid}_{filename}` instead of `{caseid}_{filename}`.
3. **Archive creation** — `tar --ignore-failed-read` semantics: the Go `archive/tar` writer does not have a built-in equivalent. The implementation must catch `os.Open` and `io.Read` errors per-file, log a warning, and skip the file rather than aborting the entire archive. The archive must preserve the full directory path inside the tarball (e.g. `/must-gather/...`) because the Bash script uses absolute paths (`tar ... $must_gather_output/`).
4. **Obfuscate-without-credentials exit** — when `obfuscate=true` but `caseid`, `username`, and `password` are all empty, the subcommand must exit 0 after completing obfuscation. Note: when obfuscation is enabled but only *some* credentials are missing (partial config), the script exits 1 — the Go implementation must preserve this partial-vs-empty distinction.
5. **Obfuscation log preservation** — the Bash script copies the obfuscation log into the cleaned output directory (`cp obfuscation.log cleaned/obfuscation.log`) so it is included in the uploaded archive. The Go implementation must replicate this.
6. **Proxy routing** — the Bash script prefers `https_proxy` over `http_proxy` via `${https_proxy:-${http_proxy}}` and reads lowercase env var names. The Go implementation must read the same lowercase env vars injected by the Job template (`http_proxy`, `https_proxy`, `no_proxy`). The shared `httpproxy.FromEnvironment()` reads both cases and selects by request scheme, which produces equivalent behavior for the Job template's lowercase-only injection. HTTPS proxies use HTTP CONNECT (replacing `socat`/`https-proxy-connect-util`). HTTP proxies use HTTP CONNECT (replacing `nc --proxy`). Proxy authentication via URL userinfo is supported.
7. **Host key verification** — `StrictHostKeyChecking=no` is matched by `ssh.InsecureIgnoreHostKey()` (already used in `validation.go`).
8. **Archive cleanup** — the local `.tar.gz` file is removed after successful upload.

#### Package Layout

The SFTP/proxy/archive logic is extracted into `pkg/upload/`, a new package with no dependencies on controller-runtime or the MustGather API types. Both the controller preflight (`controllers/mustgather/validation.go`) and the upload subcommand (`pkg/mustgatherutil/upload.go`) import from it. This avoids circular imports because `pkg/upload/` depends only on stdlib, `golang.org/x/crypto/ssh`, `github.com/pkg/sftp`, and `golang.org/x/net/http/httpproxy`.

```text
pkg/upload/
├── sftp.go            # SFTPClient: dial (direct or via proxy), put file, close
├── sftp_test.go       #   Injectable net.Conn and ssh.ClientConn for unit tests
├── archive.go         # CreateArchive: streaming tar.gz with per-file error skip
├── archive_test.go    #   Tests: unreadable file skip, correct paths in tarball
├── proxy.go           # ProxyDial: HTTP CONNECT with Basic auth (extracted from validation.go)
├── proxy_test.go
├── host.go            # NormalizeHostAddress, ContainsPort, IPv6 bracketing
├── host_test.go
├── errors.go          # ClassifySFTPError, IsTransientError (extracted from validation.go)
└── errors_test.go

pkg/mustgatherutil/
├── obfuscate.go       # RunObfuscate (existing, unchanged)
├── upload.go          # RunUpload: env var parsing, obfuscation, archive, SFTP put
└── upload_test.go     # Unit tests for env parsing, remote path, obfuscate-skip logic
```

The `controllers/mustgather/validation.go` retains the `validateSFTPCredentials` and `validateSFTPWithRetry` functions (controller-specific orchestration with `logr.Logger`) but delegates to `pkg/upload.SFTPClient` for the actual dial/connect/verify. Helper functions (`normalizeHostAddress`, `proxyDialContext`, `classifySFTPError`, `buildSSHConfig`, `containsPort`, `getProxyURLForAddr`, `IsTransientError`) move to `pkg/upload/` and are exported.

#### Entrypoint

The upload subcommand is registered in `main.go` following the same pattern as `obfuscate`:

```go
func main() {
    if len(os.Args) > 1 && os.Args[1] == "obfuscate" {
        os.Exit(mustgatherutil.RunObfuscate(os.Args[2:]))
    }
    if len(os.Args) > 1 && os.Args[1] == "upload" {
        os.Exit(mustgatherutil.RunUpload(os.Args[2:]))
    }
    // ... existing controller setup ...
}
```

#### Job Template Change

In `controllers/mustgather/template.go`, the upload command constants change:

**Before:**
```go
uploadCommand       = "count=0\nuntil...\n/usr/local/bin/upload"
uploadCommandDirect = "/usr/local/bin/upload"
```

**After:**
```go
uploadCommand       = "count=0\nuntil...\n/usr/local/bin/must-gather-operator upload"
uploadCommandDirect = "/usr/local/bin/must-gather-operator upload"
```

The gather-wait shell wrapper (`pgrep` polling loop) is unchanged. Only the final invocation line changes.

Additionally, the SSH filesystem setup prefix in `getUploadContainer` is removed:

**Before:**
```go
uploadCommandWithSSH := fmt.Sprintf("mkdir -p %s; touch %s; chmod 700 %s; chmod 600 %s; %s",
    sshDir, knownHostsFile, sshDir, knownHostsFile, uploadCmd)
```

**After:**
The Go SSH client uses `ssh.InsecureIgnoreHostKey()` in memory and does not read `~/.ssh/known_hosts`, so this filesystem setup is unnecessary. The upload command is used directly without the SSH directory prefix. The `sshDir` and `knownHostsFile` constants are removed.

#### Dockerfile Changes

**Before:**
```dockerfile
RUN dnf install -y tar gzip openssh-clients wget shadow-utils procps sshpass nc && \
    dnf clean all

COPY --from=builder .../build/bin /usr/local/bin
```

**After:**
```dockerfile
RUN dnf install -y shadow-utils procps && \
    dnf clean all
```

The `build/bin/upload` and `build/bin/https-proxy-connect-util` scripts are no longer copied into the image. The following packages are removed:

| Package | Previous use | Replacement |
|---|---|---|
| `sshpass` | Password-based SFTP auth | `ssh.Password()` via `golang.org/x/crypto/ssh` |
| `openssh-clients` | `sftp` CLI binary | `github.com/pkg/sftp` Go library |
| `nc` (nmap-ncat) | HTTP proxy tunneling (`--proxy-type http`) | `proxyDialContext` in `pkg/upload/proxy.go` |
| `wget` | Not used by upload (legacy) | Removed |
| `tar`, `gzip` | `tar -caf` archive creation | `archive/tar` + `compress/gzip` in `pkg/upload/archive.go` |

Retained: `procps` (for `pgrep` in the gather-wait loop), `shadow-utils` (for user/group management).

Note: the gather container runs a *different* image (the must-gather image) and does not use the operator image's `tar`/`gzip`. The `socat` package (required by `https-proxy-connect-util` for HTTPS proxy tunneling) is provided by the base image and is no longer needed but is not explicitly installed — it stops being exercised when the helper script is removed.

#### Obfuscation

The upload subcommand calls obfuscation in-process using the same `must-gather-clean` library (`mgclean.Run`) that `RunObfuscate` in `pkg/mustgatherutil/obfuscate.go` uses, rather than shelling out to `must-gather-operator obfuscate` as a subprocess. This avoids subprocess management complexity (the Bash script uses a `set +e` subshell with exit-code capture via temp file) and provides direct error propagation.

The obfuscation log (`obfuscation.log`) must be copied into the cleaned output directory so it is included in the uploaded archive, matching the Bash script behavior (line 82: `cp "${OBFUSCATION_LOG}" "${must_gather_upload}/cleaned/obfuscation.log"`).

The three obfuscation modes are preserved:

1. **Gather + Obfuscate + Upload** — obfuscation runs on `/must-gather`, output goes to `/must-gather-upload/cleaned`, obfuscation log is copied into `cleaned/`, archive is created from cleaned output, then uploaded.
2. **Obfuscate-only (no SFTP credentials)** — obfuscation runs. If `caseid`, `username`, and `password` are all empty, the subcommand exits 0 without uploading. If only some are missing, the subcommand exits 1 (partial credential error).
3. **Obfuscate from source PVC** — obfuscation reads from the source PVC mount (read-only), writes cleaned output to emptyDir, then uploads.

### Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| **Behavioral drift from Bash script** | Upload failures in production that did not occur with the script | Comprehensive parity test suite covering every branch in `build/bin/upload`. Run side-by-side testing against a real SFTP server before removing the script. |
| **FIPS/BoringCrypto compatibility** | Go SSH implementation may not be FIPS-compliant in the same way as OpenSSH | The operator already builds with `FIPS_ENABLED=true` (Makefile line 1) and uses `golang.org/x/crypto/ssh` for preflight validation which performs a full SSH handshake and SFTP subsystem check. The upload subcommand adds only an SFTP `Put` call using the same library and crypto path. FIPS compliance is inherited. |
| **Large archive memory pressure** | Streaming tar/gzip in Go could use more memory than the shell `tar` command | Use streaming I/O (`io.Copy` with buffered writers) to avoid loading entire archives into memory. The archive is written directly to disk, matching the script's behavior. |
| **Proxy authentication edge cases** | Proxy URLs with special characters in passwords may be parsed differently | Use `net/url.Parse` for proxy URL handling (same as `validation.go`), which correctly handles URL-encoded credentials. The Bash script uses `sed`/`cut` to parse proxy URLs, which does not handle URL-encoded characters or passwords containing `@` or `:`. The Go implementation using `net/url.Parse` is strictly more correct, but any existing proxy configs that relied on the script's broken parsing could behave differently. Test with special characters in proxy passwords. |
| **Proxy env var case sensitivity** | Bash reads lowercase `http_proxy`; Go `httpproxy.FromEnvironment()` reads both cases | The Job template injects lowercase env vars. `httpproxy.FromEnvironment()` handles both cases. Validate that the Go upload reads the injected lowercase vars correctly by testing with only lowercase env vars set. |

### Drawbacks

- **Rewrite cost on the critical support path** — the upload phase is how customers get diagnostic data to Red Hat. A regression here directly impacts support case resolution time. The mitigation is thorough parity testing and a phased rollout.
- **Temporary dual maintenance** — during development, both the Bash script and Go subcommand exist in the codebase. The Bash script is removed only after the Go subcommand passes all parity tests and ships in a release.

## Alternatives (Not Implemented)

### Keep the Bash Script

The simplest option is to leave the Bash script as-is. This avoids any risk of behavioral regression. However, it perpetuates the SFTP/proxy code duplication between the script and `validation.go`, leaves the upload path untestable at the unit level, and maintains OS-level dependencies (`sshpass`, `nc`, `socat`) that enlarge the CVE surface of the operator image.

### Thin Go Wrapper That Shells Out to `sftp`/`sshpass`

A middle-ground approach would write the orchestration (env var parsing, archive naming, obfuscation gating) in Go but still shell out to `sshpass`/`sftp` for the actual SFTP transfer. This gains testability for the orchestration logic but does not unify the SFTP client with the controller's preflight validation, does not remove the OS-level dependencies, and adds the complexity of managing subprocess I/O from Go. Rejected because it provides insufficient benefit over the full Go implementation.

## Open Questions

1. **In-process vs subprocess obfuscation** — the plan calls for in-process obfuscation (calling `mgclean.Run` directly). The existing `RunObfuscate` in `pkg/mustgatherutil/obfuscate.go` calls this function successfully as an in-process library, so global state / panic risk is low. However, the Bash script currently captures obfuscation output via `tee` to produce an `obfuscation.log` that is included in the bundle. The Go in-process approach must replicate this log capture (e.g., by configuring klog output to write to both stderr and a file). Validate that `mgclean.Run` does not hold process-global state that conflicts with the upload subcommand's own logging during implementation.
2. **`tar --ignore-failed-read` fidelity** — Go's `archive/tar` has no equivalent flag. The implementation must manually catch `os.Open`, `os.Stat`, and `io.Read` errors per-file and skip with a warning. Edge cases to validate: symlinks to missing targets, named pipes, files deleted between directory walk and open (TOCTOU), and extremely long filenames.

## Test Plan

### Unit Tests

- **Archive creation**: verify tar.gz output contains expected files, handles unreadable files gracefully (skip with warning, not abort), produces correct filenames from `FILENAME_PREFIX`, and preserves the full directory path inside the tarball (e.g. `/must-gather/...`).
- **Archive edge cases**: symlinks to missing targets are skipped, named pipes are skipped, TOCTOU (file deleted between walk and open) is handled.
- **Remote path construction**: verify `{caseid}_{filename}` for external users and `{username}/{caseid}_{filename}` for internal users.
- **Proxy dialing**: test HTTP CONNECT proxy dial with and without authentication, HTTPS proxy dial, NO_PROXY exclusion, using injectable `net.Conn` for the proxy connection. Test both lowercase (matching Job template injection) and uppercase-only `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` configurations.
- **Host normalization**: test IPv4 with/without port, IPv6 bare/bracketed with/without port, default port injection.
- **Obfuscation modes**: test obfuscate+upload, obfuscate-only (all credentials empty → exit 0), partial credentials (some missing → exit 1), obfuscate from source PVC. Verify obfuscation log is copied into cleaned output directory.
- **Error classification**: verify structured error messages for auth failure, connection refused, DNS failure, timeout, connection reset, and SFTP subsystem errors.
- **Environment variable parsing**: verify all 13 env vars are read correctly, defaults are applied, and missing required vars produce clear errors with guidance.

### Integration / E2E Tests

- **SFTP upload parity**: run the Go upload subcommand against a test SFTP server and verify the uploaded file matches what the Bash script would produce (same filename, same archive contents, same remote path).
- **Proxy upload parity**: run the Go upload through an HTTP proxy and an HTTPS proxy, verifying the upload succeeds with the same proxy configurations that work with the Bash script.
- **Obfuscation + upload**: verify end-to-end flow with obfuscation enabled, comparing output to the Bash script flow.
- **Failure scenarios**: verify that SFTP auth failure, unreachable host, and proxy failure produce non-zero exit codes and structured log messages.

## Graduation Criteria

This enhancement is an internal refactoring that does not introduce a new user-facing feature or API. It ships as part of a normal operator release.

### Dev Preview -> Tech Preview

Not applicable — there is no feature gate or user opt-in. The refactoring is transparent to users.

### Tech Preview -> GA

- All parity tests pass against a real SFTP server (Red Hat staging environment).
- Proxy upload tests pass with both HTTP and HTTPS proxies (including authenticated proxies).
- Obfuscation + upload parity verified end-to-end, including obfuscation log preservation.
- Bash script (`build/bin/upload`) and `https-proxy-connect-util` removed from the image.
- OS-level packages (`sshpass`, `nc`, `openssh-clients`, `wget`, `tar`, `gzip`) removed from Dockerfile.
- SSH filesystem setup prefix removed from Job template.
- No regressions reported in operator CI or QE test suites for at least one full sprint.

### Removing a deprecated feature

The Bash upload script (`build/bin/upload`) and the HTTPS proxy helper (`build/bin/https-proxy-connect-util`) are removed from the repository and the operator image after the Go upload subcommand is validated and shipped.

## Upgrade / Downgrade Strategy

**Upgrade**: The operator image is replaced atomically. Jobs created by the old operator image continue to run with the old image (which still contains the Bash script). New Jobs created after the upgrade use the new image with the Go upload subcommand. No CR migration is required. No user action is needed.

**Downgrade**: Rolling back to a previous operator version restores the old image with the Bash script. Any in-flight Jobs created by the new operator will complete or fail with the new image. New Jobs after downgrade use the old image. No data loss or CR incompatibility.

## Version Skew Strategy

The operator binary and the Job template are shipped in the same container image. The upload subcommand and the Job template that invokes it are always at the same version. Version skew between these components is not possible.

The only potential skew is between a running Job Pod (using an older or newer image) and the controller (using the current image). This is the same situation that exists today with the Bash script and is handled by the Job's `restartPolicy: Never` and `backoffLimit: 3`.

## Operational Aspects of API Extensions

This enhancement does not introduce or modify any API extensions (CRDs, webhooks, aggregated API servers, finalizers). The MustGather CRD, its validation, and the controller's reconciliation logic are unchanged.

- **SLIs**: No change. The existing metrics (`must_gather_operator_must_gather_total`, `must_gather_operator_must_gather_errors`) continue to track Job creation and failure.
- **Failure modes**: Upload failures are surfaced through Job failure status and MustGather CR conditions, identical to today. The Go upload subcommand produces more structured log output, which improves debuggability but does not change the failure signaling mechanism.
- **Escalation**: Failures in the upload phase are escalated to the must-gather-operator team, same as today.

## Support Procedures

### Detecting Issues

- **Symptoms**: MustGather CR status shows a failed condition; the upload container logs show an error message.
- **Logs**: `oc logs <must-gather-job-pod> -c upload` shows structured error output from the Go upload subcommand. Error messages include classification (e.g. "Authentication failed", "Connection refused", "DNS resolution failed") and actionable guidance.
- **Metrics**: `must_gather_operator_must_gather_errors` counter is incremented on Job failure.

### Troubleshooting

1. Check the upload container logs for the classified error message: `oc logs <must-gather-job-pod> -c upload`.
2. Verify the secret referenced by `caseManagementAccountSecretRef` contains valid `username` and `password` keys.
3. If a proxy is configured, verify the proxy URL is correct and the proxy allows CONNECT to the SFTP host on port 22.
4. Check cluster-wide proxy settings (`oc get proxy cluster -o yaml`) if proxy environment variables are expected.
5. The controller's preflight SFTP validation uses the same shared `pkg/upload` client. If preflight succeeded but upload failed, check for transient network issues or proxy state changes between preflight and Job execution.

### Disabling the Feature

This is not a feature that can be disabled independently. The Go upload subcommand replaces the Bash script unconditionally. To revert to the Bash script, downgrade the operator to a version that ships the previous image.

## Infrastructure Needed

No new subprojects, repositories, or testing infrastructure are needed. The existing CI pipeline, SFTP test server (if available), and operator image build process are sufficient.

## Implementation History

- 2026-09-30: Initial enhancement proposal (provisional).
