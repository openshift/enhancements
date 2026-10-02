# Appendix: Go 1.26 Assessment

*OCP 4.22 — Go 1.25 → Go 1.26 (matching upstream Kube 1.35)*

See the [main enhancement](go-version-bump.md) for the repeatable process.

**Go 1.26 release notes**: [go.dev/doc/go1.26](https://go.dev/doc/go1.26)
(released February 2026)

## Upstream Kube Handling

Kube 1.35 bumped the **toolchain** to Go 1.26.5 via
[PR #140920](https://github.com/kubernetes/kubernetes/pull/140920), but kept
`go.mod` at `go 1.25.0` with `godebug default=go1.25` — the standard upstream
approach for release branches (KEP-3744). The protobuf deadcode fix was
cherry-picked in [PR #139666](https://github.com/kubernetes/kubernetes/pull/139666)
(from [#137451](https://github.com/kubernetes/kubernetes/pull/137451)).

**openshift/kubernetes** — the upstream `release-1.35` branch has the Go 1.26.5
bump and protobuf fix. The OCP `release-4.22` branch still needs the bump.

## What OpenShift Repos Should Do for release-4.22

Bump the **toolchain** (builder image, `.go-version`) but leave the `go.mod`
`go` directive unchanged. The Go 1.26 toolchain compiles the binary with
security fixes; because `go.mod` still declares the original Go version,
GODEBUG automatically preserves the original runtime behavior.

For each repo on its `release-4.22` branch:

1. Update Dockerfile builder image to Go 1.26 (e.g., `rhel-9-golang-1.26-openshift-*`)
2. Update `.go-version` to `1.26.x` (if present — only 6 repos have this file)
3. Pin `google.golang.org/protobuf` to `v1.36.12-0.20260120151049-f2248ac996af`
4. Run `go mod tidy` and `go mod vendor` (if vendoring)
   Note: the pinned protobuf revision declares `go 1.23`. For repos with a
   `go` directive below 1.23, `go mod tidy` will bump it to 1.23. This is
   expected and acceptable — these repos are already compiled with Go 1.25+.
5. **Do not** change the `go` directive in `go.mod` beyond what `go mod tidy`
   requires
6. **Do not** add a `toolchain` directive to `go.mod` — the toolchain is
   controlled by `.go-version` and the builder image

## Payload Repo Status (OCP 4.22.9, 2026-08-11)

Scanned 160 unique source repos on their `release-4.22` branches:

| Category | Count |
|----------|-------|
| No `go.mod` (non-Go) | 9 |
| Go repos, no protobuf dependency | 7 |
| Go repos, **needs protobuf fix** | **143** |
| Go repos, protobuf fix present | 1 |

The one repo with the fix is `openshift/ovn-kubernetes` (go.mod in
`go-controller/`). Note: `openshift/etcd` uses branch `openshift-4.22`;
`openshift/kubernetes-autoscaler` has go.mod in `cluster-autoscaler/`;
`openshift/insights-runtime-extractor` has go.mod in `exporter/`
— all three still need the fix.

Repos with `.go-version`: `openshift/kubernetes`, `openshift/coredns`,
`openshift/thanos`, `openshift/aws-pod-identity-webhook`,
`openshift/aws-encryption-provider`, `openshift/etcd`.

## Protobuf Deadcode Elimination Regression

Go 1.26 added `reflect.Value.Methods()`. Protobuf's `MethodByName("Methods")`
disables dead code elimination — a **build-time** issue triggered by the Go
1.26 linker regardless of the `go` directive. GODEBUG does not help.

| Binary | Go 1.25 | Go 1.26 (unfixed) | Increase |
|--------|---------|-------------------|----------|
| kube-apiserver | 75.6 MB | 90.5 MB | +20% |
| kubelet | 51.5 MB | 73.3 MB | +42% |
| kube-proxy | 38.5 MB | 62.4 MB | +62% |

**Fix**: Pin `google.golang.org/protobuf` to
`v1.36.12-0.20260120151049-f2248ac996af`
([CL 734280](https://go-review.googlesource.com/c/protobuf/+/734280)).

## GODEBUG Decision Table

Because the `go` directive is **not** being bumped, GODEBUG automatically
preserves the original runtime behavior for GODEBUG-gated changes. However,
changes controlled by `GOEXPERIMENT` (build-time) or with no opt-out are
active regardless.

| Change | GODEBUG opt-out | Decision |
|--------|----------------|----------|
| **Post-quantum TLS (ML-KEM)** enabled by default | `tlssecpmlkem=0` | **Not active** — gated by GODEBUG; `go` directive unchanged |
| **`net/url.Parse` strict colons** | `urlstrictcolons=0` | **Not active** — gated by GODEBUG |
| **Crypto ignores custom random** | `cryptocustomrand=1` | **Not active** — gated by GODEBUG |
| **Green Tea GC** default | `GOEXPERIMENT=nogreenteagc` | **Active** — build-time `GOEXPERIMENT`, not gated by GODEBUG. Opt-out removed in Go 1.27. Monitor for GC regressions |
| **HTTP 307 instead of 301** for ServeMux trailing slash | none | **Active** — unconditional, no GODEBUG opt-out. Audited: OCP health endpoints use exact paths; debug socket handles its own redirect. Low risk |

## FIPS Impact (Go 1.26)

Broader FIPS transition tracked under
[HPSTRAT-723](https://redhat.atlassian.net/browse/HPSTRAT-723).

- **FIPS module version is a build-time decision with runtime consequences.**
  The toolchain determines which `crypto/fips140` module version is embedded
  in the binary. A stricter module version can reject algorithms that
  previously worked under FIPS mode. GODEBUG does not gate this. Monitor
  CI FIPS profile results and upstream Go issues for FIPS-related regressions.

## GODEBUG Expiration Lookahead (Go 1.27)

Settings removed in Go 1.27 — will become active when the `go` directive
is eventually bumped past 1.27:

| Setting | OCP exposure |
|---------|--------------|
| `tlsrsakex` | RSA key exchange ciphers. Configurable via `--tls-cipher-suites`. |
| `tls10server` | TLS 1.0 server support. Affects clusters using `--tls-min-version=VersionTLS10`. |

## Go vet: Eventf Format Checks (Directive-Gated, Not Active)

Go 1.26 extended `go vet`'s printf-wrapper detection to interface methods,
newly flagging non-constant format strings in `Recorder.Eventf`-style calls.
This is **not active** for the toolchain-only bump — it fires only for files
declaring `go 1.26`, so the unchanged `go` directive preserves vet behavior.
Upstream fixed it in [PR #137456](https://github.com/kubernetes/kubernetes/pull/137456)
(backported to `release-1.35` as [#140030](https://github.com/kubernetes/kubernetes/pull/140030),
inherited by openshift/kubernetes). No action for OCP 4.22; only relevant if
the `go` directive is ever bumped to 1.26.

## What to Watch During Rollout

- **Binary size**: verify the protobuf fix restores pre-1.26 sizes (see
  Protobuf Deadcode section above for expected numbers)
- **GC behavior**: Green Tea GC is active (build-time `GOEXPERIMENT`, not
  gated by GODEBUG). Monitor for GC-related performance regressions
- **HTTP redirects**: ServeMux trailing-slash redirects changed from 301 to
  307 (unconditional, no opt-out). Audited in openshift/kubernetes — low risk;
  health endpoints use exact paths, debug socket handles its own redirect

## Per-Repo Rollout Guidance

Each of the 143 repos has a different layout. The steps in "What OpenShift
Repos Should Do" above are the checklist; the notes below cover what varies.

- **Dockerfile**: every repo has at least one Dockerfile (sometimes under
  `Dockerfile`, `Dockerfile.rhel`, `images/`, or `openshift/`). Find the
  `FROM` line that references the golang builder image and update it to the
  Go 1.26 equivalent (e.g., `rhel-9-golang-1.26-openshift-*`). The exact
  image name and path differ per repo.
- **Vendoring**: if the repo has a `vendor/` directory, run
  `go mod vendor` after updating protobuf. If it does not vendor, only
  `go mod tidy` is needed.
- **Branch**: most repos use `release-4.22`. Exceptions exist (e.g.,
  `openshift/etcd` uses `openshift-4.22`). Verify the branch name per repo.
- **Subdirectory go.mod**: some repos have `go.mod` in a subdirectory rather
  than the root (e.g., `openshift/ovn-kubernetes` → `go-controller/`,
  `openshift/kubernetes-autoscaler` → `cluster-autoscaler/`).
- **Protobuf skip**: the 7 Go repos with no protobuf dependency (see table
  above) can skip the protobuf pin step. All others need it.
- **`.go-version`**: only 6 repos have this file (listed above). For all
  other repos, the toolchain version is controlled solely by the Dockerfile
  builder image.

## Implementation Status

See the [main enhancement](go-version-bump.md) for the 6-phase process.
Currently in Phase 1 (Assessment). Phase 2 requires the protobuf fix across
143 repos. All 151 Go repos require the builder-image update.
