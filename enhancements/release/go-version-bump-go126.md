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

The [OpenShift Kubernetes update](https://github.com/openshift/kubernetes/pull/2773)
merged to `release-4.22` in September 2026. It uses the upstream protobuf
pseudo-version; later downstream repos generally use the tagged `v1.36.12`.

## What OpenShift Repos Should Do for release-4.22

Bump the **toolchain** used by the relevant builds. Preserve existing `go`
and `toolchain` directives where compatible. Keeping the `go` directive
unchanged retains the old defaults for behavior changes controlled by
GODEBUG. Other build-time or runtime changes may still apply.

For each affected source repo, verify the branch first. Central CI and ART
configuration may be maintained on different branches.

1. Update the Go 1.25 builder references actually used by Dockerfiles, CI,
   and production builds. Check `.go-version` and other version selectors
   where present.
2. Where a module uses `google.golang.org/protobuf`, update it to the
   appropriate fixed version. Downstream 4.22 PRs generally use `v1.36.12`;
   OpenShift Kubernetes follows upstream's pseudo-version. Do not add a pin
   to a module that does not use protobuf.
3. Use the repository's own dependency update and verification procedure.
   Some repos have several modules or a `go.work` workspace; some do not
   commit a vendor tree. Do not assume `go mod vendor` covers every repo.
4. Review any `go` directive change caused by the dependency's minimum Go
   version and explain it in the PR. Do not add or change a `toolchain`
   directive merely to match the builder image.

## Protobuf Deadcode Elimination Regression

Go 1.26 added `reflect.Value.Methods()`. Protobuf's `MethodByName("Methods")`
disables dead code elimination — a **build-time** issue triggered by the Go
1.26 linker regardless of the `go` directive. GODEBUG does not help.

| Binary | Go 1.25 | Go 1.26 (unfixed) | Increase |
|--------|---------|-------------------|----------|
| kube-apiserver | 75.6 MB | 90.5 MB | +20% |
| kubelet | 51.5 MB | 73.3 MB | +42% |
| kube-proxy | 38.5 MB | 62.4 MB | +62% |

**Fix**: Use the protobuf fix from
[CL 734280](https://go-review.googlesource.com/c/protobuf/+/734280).
Kubernetes carries it as
`v1.36.12-0.20260120151049-f2248ac996af`; downstream 4.22 repos generally
use the tagged `v1.36.12`.

## GODEBUG Decision Table

Modules with a pre-1.26 `go` directive keep the old defaults for behavior
changes controlled by GODEBUG. Check any module whose directive changes.
Changes controlled by `GOEXPERIMENT` or with no opt-out are active regardless.

| Change | GODEBUG opt-out | Decision |
|--------|----------------|----------|
| **Post-quantum TLS (ML-KEM)** enabled by default | `tlssecpmlkem=0` | Not active with a pre-1.26 `go` directive |
| **`net/url.Parse` strict colons** | `urlstrictcolons=0` | Not active with a pre-1.26 `go` directive |
| **Crypto ignores custom random** | `cryptocustomrand=1` | Not active with a pre-1.26 `go` directive |
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

## Go vet and Lint

Check format/type errors and `Eventf`-style printf wrappers when running vet
or lint with the new toolchain. The upstream `Eventf` fix was backported to
`release-1.35` in [PR #140030](https://github.com/kubernetes/kubernetes/pull/140030).
Other repos can still have findings. A linter built with an older Go version
may also fail while analyzing Go 1.26; compare the required lint target on
unchanged and updated source before attributing the failure to this bump.

## What to Watch During Rollout

- **Binary size**: verify the protobuf fix restores pre-1.26 sizes (see
  Protobuf Deadcode section above for expected numbers)
- **GC behavior**: Green Tea GC is active (build-time `GOEXPERIMENT`, not
  gated by GODEBUG). Monitor for GC-related performance regressions
- **HTTP redirects**: ServeMux trailing-slash redirects changed from 301 to
  307 (unconditional, no opt-out). Audited in openshift/kubernetes — low risk;
  health endpoints use exact paths, debug socket handles its own redirect
