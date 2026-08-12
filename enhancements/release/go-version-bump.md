---
title: go-version-bump
authors:
  - "@Prashanth684"
reviewers:
  - "@ashwindasr" # ART team, for builder image and RPM coordination
  - "@sdodson" # Staff engineer, for architectural review
  - "@aos-staff-engineers"
approvers:
  - "@sdodson"
api-approvers:
  - N/A
creation-date: 2026-08-11
last-updated: 2026-08-11
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/HPSTRAT-722
see-also:
  - https://github.com/kubernetes/enhancements/issues/3744
  - https://kubernetes.io/blog/2023/04/06/keeping-kubernetes-secure-with-updated-go-versions/
---

# Go Version Bump for OpenShift

## Summary

Starting with OCP 4.22, OpenShift will match upstream Kubernetes's practice of
bumping the Go toolchain version on release branches. This enhancement defines
the process for performing the same Go bump that upstream Kubernetes performed
on its release branch, and identifies the per-repo change scope across all
OpenShift repositories. FIPS-relevant changes are documented per bump; the
broader FIPS transition is covered by
[HPSTRAT-723](https://redhat.atlassian.net/browse/HPSTRAT-723).
Version-specific behavioral change analyses are maintained as appendices to
this document.

## Motivation

Go, Kubernetes, and OpenShift have different release cadences and support
lifetimes. Each depends on the one before it:

```text
  OpenShift  ──depends on──▶  Kubernetes  ──built with──▶  Go toolchain
```

When Go goes end-of-life before OpenShift does, a security gap opens.
Currently, Red Hat's Go Toolset team backports compiler CVE fixes to EOL'd Go
versions. Bumping the toolchain closes the remaining gap: it picks up
compiler and stdlib security fixes directly from upstream Go, and it allows
updating dependencies whose CVE fixes don't raise the Go minimum version.
Dependencies whose fixes do require a higher Go minimum remain blocked
until the `go` directive in `go.mod` is also bumped (not part of this
enhancement).

### The Lifecycle Mismatch

```text
  Go 1.N     |========12 months========|  EOL
  Go 1.N+1        |========12 months========|  EOL
  Go 1.N+2             |========12 months========|  EOL

  Kube 1.X   |===========14 months===========|  EOL

  OCP 4.Y                |================18-24+ months (EUS can extend further)================|  EOL
```

**Go:** releases a new minor version every 6 months. Only the latest two minor
versions receive security patches, giving each version approximately 12 months
of supported life.

**Kubernetes:** releases every ~4 months, with 14 months of patch support per
minor version.

**OpenShift:** maps to a Kubernetes release (e.g., OCP 4.22 maps to Kube 1.35),
but has 18 months of full support, plus maintenance support, and EUS releases
can stretch to 2+ years.

The gap for Kubernetes is short enough to manage with a single Go bump.
The gap for OpenShift is longer. This enhancement aligns with upstream
Kubernetes — performing the same Go bump that upstream Kube already performed
on its release branch. At this time additional bumps over the extended life
of OpenShift are not currently planned.

### What Changed Upstream: KEP-3744 and Go's GODEBUG Mechanism

Before Go 1.21, each new Go minor version could introduce breaking behavioral
changes with no opt-out. Real examples:

| Go version | Breaking change                              | Impact                       |
|------------|----------------------------------------------|------------------------------|
| Go 1.6     | HTTP/2 enabled by default                    | Broke proxies and middleware |
| Go 1.17    | Dropped x509 Common Name support             | Broke certificate validation |
| Go 1.18    | Disabled SHA-1 x509 certificates by default  | Broke legacy PKI setups      |

Starting with Go 1.21, Go introduced a formal backward compatibility contract
via GODEBUG: when `go.mod` declares `go 1.N`, the Go 1.N+1 toolchain
automatically preserves 1.N runtime behavior. Individual GODEBUG knobs
allow opting into or out of specific changes. Go guarantees a minimum 2-year
compatibility window.

```text
  ┌──────────────────────────────────────────────────────────────────┐
  │                     GODEBUG IN ACTION                           │
  │                                                                 │
  │   go.mod says:     go 1.N                                       │
  │   Compiled with:   Go 1.N+1 toolchain                           │
  │                                                                 │
  │   Result:          Binary gets Go 1.N+1 security fixes          │
  │                    but BEHAVES like Go 1.N at runtime           │
  │                                                                 │
  │   ┌─────────────────────────┐  ┌─────────────────────────┐     │
  │   │   Go 1.N+1 toolchain    │  │   Runtime behavior      │     │
  │   │                         │  │                         │     │
  │   │  • Security patches  ✓  │  │  • TLS defaults: 1.N   │     │
  │   │  • Bug fixes         ✓  │  │  • URL parsing:  1.N   │     │
  │   │  • Performance       ✓  │  │  • Crypto rand:  1.N   │     │
  │   │  • New std library   ✓  │  │  (GODEBUG-gated only)  │     │
  │   └─────────────────────────┘  └─────────────────────────┘     │
  └──────────────────────────────────────────────────────────────────┘
```

[KEP-3744](https://github.com/kubernetes/enhancements/issues/3744) formalized
this for Kubernetes: release branches can and should bump Go to stay on
supported versions, relying on GODEBUG to prevent behavioral regressions. This
has been GA since Kubernetes 1.27.

### User Stories

- As a cluster administrator, I want OpenShift release branches to receive
  Go toolchain bumps aligned with upstream Kubernetes so that the window
  during which security scanners flag known Go CVEs is reduced.
- As a security reviewer, I want Go toolchain bumps on release branches to
  follow the same cadence as upstream Kubernetes so that compiler and
  stdlib security fixes are picked up without requiring forks or backports.
- As a platform engineer maintaining an OpenShift operator, I want the
  Go toolchain on release branches to match upstream Kubernetes so that
  I can update dependencies whose CVE fixes require the newer toolchain.

### Goals

1. Define a documented process for bumping Go on an OCP release branch to
   match the upstream Kubernetes Go version.
2. For each bump, produce a version-specific appendix with behavioral
   changes, FIPS impact analysis, dependency issues, and open questions.
3. Document FIPS-relevant changes for each Go bump; the broader FIPS
   transition is covered by HPSTRAT-723.
4. Identify the per-repo change scope — which OpenShift repos are affected
   and what changes each one needs.
5. Validate each bump through CI and nightly rollout with no detectable
   regression.

### Non-Goals

1. Transitioning to native Go FIPS (Go's built-in FIPS 140-3 module).
   Covered by [HPSTRAT-723](https://redhat.atlassian.net/browse/HPSTRAT-723).
2. Going beyond upstream Kubernetes's Go bump cadence initially. Each OCP
   release will first perform the same bump upstream Kube performed, then
   evaluate if additional bumps are necessary.
3. Bumping Go for platform-agnostic or rolling-release-stream operators.

## Proposal

This enhancement defines three things:

1. A repeatable 6-phase process for any Go version bump
2. A FIPS review step within each bump (broader FIPS transition is HPSTRAT-723)
3. A per-repo change scope identifying what changes in each repository

Version-specific analysis (GODEBUG decisions, dependency issues, open
questions) is maintained as version specific appendices — one appendix
per Go version bump. See [Go 1.26 Assessment](go-version-bump-go126.md)
for the first instance.

### Repeatable Go Bump Process

This process applies to any Go minor version bump on an OCP release branch.

```text
  ┌────────────┐   ┌──────────────┐   ┌──────────┐   ┌────────────┐   ┌────────────┐   ┌──────────────┐
  │ ASSESSMENT │──▶│ DEPENDENCY   │──▶│ BUILDER  │──▶│ PR ROLLOUT │──▶│ VALIDATION │──▶│ RETROSPECTIVE│
  │            │   │ TRIAGE       │   │ PREP     │   │            │   │            │   │              │
  └────────────┘   └──────────────┘   └──────────┘   └────────────┘   └────────────┘   └──────────────┘
```

#### Phase 1: Assessment

Review the Go N+1 release notes and produce the version-specific appendix:

- [ ] Read the full Go N+1 release notes at `https://go.dev/doc/go1.N+1`
- [ ] Document behavioral changes and available GODEBUG/GOEXPERIMENT opt-outs
- [ ] Document FIPS-relevant changes (see FIPS Review Process below)
- [ ] Review upstream Kubernetes's handling of the same bump:
  - Check `kubernetes/kubernetes` for the Go bump PR and any related issues
  - Note any mitigations, workarounds, or disabled checks
- [ ] Write the version-specific appendix as a separate document

#### Phase 2: Dependency Triage

Upstream Kube typically encounters Go-version-related dependency issues first.
openshift/kubernetes inherits these fixes via cherry-picks from the upstream
release branch. Start by checking what upstream already fixed before
investigating independently.

- [ ] Check the upstream Kube release branch for dependency fixes related to
      the Go bump (cherry-picks, version pins, disabled checks)
- [ ] Verify that openshift/kubernetes has inherited those fixes
- [ ] Identify any additional dependencies that break with the new Go version
- [ ] Track upstream fixes; determine if tagged releases are available
- [ ] If fixes are not yet tagged upstream, pin to specific commits with
      pre-release versions in `go.mod`
- [ ] Enumerate all OpenShift repos that share the affected dependency
      (see Per-Repo Change Scope below)
- [ ] Validate fixes in representative repos before rolling out broadly

#### Phase 3: Builder Preparation

Coordinate with ART on builder images and RHEL Go RPMs:

- [ ] Confirm that RHEL provides the target Go version as an RPM
- [ ] Confirm that builder images are available for the target Go version
- [ ] Coordinate with ART on the timeline for switching builders
- [ ] Verify the Go bump does not break MicroShift RPM builds of shared
      components (e.g., etcd) — MicroShift's RPM build root may not have
      the newer Go version
      ([ART-20685](https://redhat.atlassian.net/browse/ART-20685))

#### Phase 4: PR Rollout

Open PRs against each repository (see Per-Repo Change Scope for the full list
of affected repos and what changes in each):

- [ ] Open PRs for openshift/kubernetes first (largest, most complex)
- [ ] Open PRs for all other affected repos with dependency fixes and version
      updates
- [ ] Ensure all PRs reference a tracking Jira

#### Phase 5: Validation

Roll out in two stages:

**Stage 1: CI repos (~1-2 weeks)**
- [ ] Switch CI repos to use the new Go builder image
- [ ] Run full test suites (unit, integration, e2e)
- [ ] Monitor for regressions: test failures, performance deltas

**Stage 2: Nightlies (~1-2 weeks)**
- [ ] Switch nightly builds to the new Go version
- [ ] Validate across the full product, not just unit/integration tests
- [ ] Monitor for TLS and FIPS-related regressions (see version-specific
      appendix for what to watch)
- [ ] Run CI jobs with FIPS cluster profiles enabled

z-stream delivery is not paused during validation. PRs from Phase 4 merge
incrementally; once a repo's PR merges, its subsequent builds use the new
Go version. Mixed-version builds are safe during the transition (see
Version Skew Strategy). If a regression is detected, the builder image is
reverted for the affected repo (see Remediation).

#### Phase 6: Retrospective

- [ ] Document issues encountered and how they were resolved
- [ ] Update this process with any new steps or checks discovered
- [ ] If applicable, write the forward plan for any subsequent bump
- [ ] Add retrospective notes to the version-specific appendix

### FIPS Review Process

Each Go bump may change the `crypto/fips140` module version and the FIPS
behavior of vendored dependencies (`crypto/tls`, `golang.org/x/crypto/ssh`,
etc.). As part of Phase 1 (Assessment), the OCP engineer should document
any FIPS-relevant changes in the version-specific appendix.

The broader FIPS transition — adopting native Go FIPS, managing FIPS module
validation, and eliminating Red Hat's custom FIPS patches — is covered by
[HPSTRAT-723](https://redhat.atlassian.net/browse/HPSTRAT-723) and is out of
scope for this enhancement.

### Per-Repo Change Scope

Each Go bump affects repos in the OCP payload in two ways:

1. **openshift/kubernetes** — mirrors upstream Kube's release-branch bump:
   toolchain updated (`.go-version`, builder images, `hack/lib/golang.sh`),
   `go.mod` `go` directive left unchanged so GODEBUG preserves original
   runtime behavior.
2. **All other Go repos** — update the Dockerfile builder image to the new Go
   version, update `.go-version` if present, apply any cross-cutting
   dependency fixes (`go.mod`, `go.sum`, `vendor/`). The `go` directive in
   `go.mod` is not changed — GODEBUG preserves the original runtime
   behavior automatically.

See the version-specific appendix for repo enumeration results.

### Workflow Description

The Go version bump workflow involves the following actors:

**OCP engineer** is the engineer driving the Go bump for the release.

**ART engineer** is responsible for builder images and build infrastructure.

**Staff engineer / approver** reviews the version-specific appendix and
approves the bump.

1. The OCP engineer reads the Go N+1 release notes and produces the
   version-specific appendix (Phase 1).
2. The OCP engineer builds openshift/kubernetes with the new Go version and
   identifies dependency issues (Phase 2).
3. The OCP engineer enumerates all affected repos in the OCP payload
   (Phase 2).
4. The OCP engineer coordinates with the ART engineer to confirm builder
   images are available (Phase 3).
5. The OCP engineer opens PRs against each affected repo (Phase 4). Each
   PR references this enhancement and the tracking Jira.
6. Each repo's owning team reviews and approves their PR. If a team does
   not review in a timely manner, staff engineers have authority to approve.
7. Review the FIPS impact section of the appendix.
8. CI repos are switched to the new builder. The OCP engineer monitors test
   results for 1-2 weeks (Phase 5, Stage 1).
9. Nightly builds are switched. The OCP engineer monitors for regressions
   for 1-2 weeks (Phase 5, Stage 2).
10. The OCP engineer writes the retrospective and updates this enhancement
    (Phase 6).

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

See the version-specific appendices for implementation details for each Go
bump. Generic constraints:

- **Cross-repo scope**: Dependency fixes must be applied to every OpenShift
  repo that shares the affected dependency, not just openshift/kubernetes.
- **Builder image availability**: Confirm with ART before starting the PR
  rollout phase.
- **GODEBUG directive**: The `go` directive in `go.mod` controls which Go
  version's runtime behavior the binary uses. Because the `go` directive is
  not bumped on release branches, GODEBUG automatically preserves the
  original runtime behavior — no explicit GODEBUG configuration is needed.
- **GODEBUG does not cover all changes**: `GOEXPERIMENT` defaults and some
  unconditional stdlib changes are active with the new toolchain regardless
  of the `go` directive. These are documented in the version-specific
  appendix.

### Risks and Mitigations

Because OpenShift follows upstream Kubernetes's Go bump, most issues surface
upstream first. The primary risk for OpenShift is ensuring those fixes are
propagated across all OCP repos.

| Risk | Mitigation |
|------|------------|
| Dependency fix from upstream not applied to all OCP repos | Enumerate all affected repos in Phase 2; automate PRs |
| Build-time toolchain change bypasses GODEBUG (e.g., binary size regression) | Check upstream for known issues; pin affected dependencies |
| FIPS module version change tightens crypto | Document per bump; coordinate with HPSTRAT-723 |
| Unforeseen behavioral change not caught by GODEBUG | GODEBUG covers most changes; revert builder image if needed |

### Drawbacks

This enhancement introduces net-new process overhead for each Go version bump:
cross-repo coordination across dozens of repositories, dependency triage that
may require upstream fixes, 2-4 weeks of validation, and FIPS review.

Automation can handle the mechanical work (opening PRs, updating version
strings), but cannot discover all risk — subtle dependency issues may require
human investigation.

The alternative (staying on EOL Go) avoids this overhead but leaves customers
with a growing backlog of unfixable CVEs and dependencies that cannot be updated.

## Alternatives (Not Implemented)

### Stay on EOL Go, fix CVEs reactively

OCP could remain on the GA Go version and address Go CVEs reactively through
forks, backports, and Red Hat's own Go builds.

**Advantages:**
- Maximum z-stream stability — no behavioral changes from Go bumps
- FIPS module version stays constant throughout the release lifecycle

**Disadvantages:**
- Growing backlog of unfixable Go CVEs that customers must triage
- Dependencies requiring newer Go minimums also become unfixable
- Reactive patching (forks, backports) does not scale
- Diverges from upstream Kubernetes practice (KEP-3744)

## Open Questions

N/A

## Test Plan

The following tests apply to every Go bump:

### CI and Nightly Validation

The primary validation mechanism is the existing CI and nightly pipeline:

- Run the complete CI test suite (unit, integration, e2e) with the new Go
  version
- Run nightly builds for 1-2 weeks and monitor for regressions
- Monitor test pass rates compared to pre-bump baselines

TLS handshake behavior is validated implicitly — CI e2e tests exercise
inter-component TLS (apiserver ↔ etcd, kubelet ↔ apiserver, etc.). There are
no dedicated cipher-suite-negotiation tests; regressions surface as connection
failures in existing e2e jobs.

### FIPS Mode Validation

FIPS validation lives in OCP CI infrastructure (FIPS cluster profiles in
origin, cluster-bot), not in individual repo test suites. If the FIPS module
version changed with the toolchain bump:

- Run CI jobs with FIPS cluster profiles enabled
- Monitor for SSH and TLS failures that indicate tightened crypto restrictions
- Coordinate with [HPSTRAT-723](https://redhat.atlassian.net/browse/HPSTRAT-723)
  owner if new FIPS enforcement modes need coverage

## Graduation Criteria

This is a process enhancement, not a feature with alpha/beta/GA stages. Each
Go bump graduates when:

1. **CI green**: All CI tests pass with the new Go version
2. **Nightlies green**: Nightly builds show no detectable regression for 1-2
   weeks
3. **Dependency fixes applied**: All affected repos have cross-cutting fixes
4. **FIPS validated**: FIPS-mode tests pass (if FIPS module version changed)
5. **Appendix complete**: The version-specific appendix is updated with
   retrospective learnings

### Dev Preview -> Tech Preview

N/A — process enhancement, not a feature.

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

All components within a given OCP release are built with the same Go version.
During upgrades, components temporarily run at different Go versions. This is
expected to be safe because:

1. The Go version does not affect the Kubernetes API wire format (protobuf/JSON)
2. GODEBUG preserves runtime behavior when the `go` directive is unchanged
3. TLS minimum version is explicitly configured (`DefaultTLSVersion()` returns
   `tls.VersionTLS12`) rather than relying on Go defaults

## Operational Aspects of API Extensions

N/A

## Support Procedures

### Detecting Go Behavioral Regressions

CI and nightly test pass rates are the primary detection mechanism. Specific
symptoms to watch for each bump are documented in the version-specific
appendix. No new metrics or alerts are introduced by this enhancement.

### Remediation

If a regression is traced to the Go toolchain bump:

1. Identify the specific Go change causing the issue
2. If urgent: revert the builder image to the previous Go version and rebuild
3. Otherwise: fix the code and release a z-stream update

GODEBUG already preserves old behavior for most changes. Individual GODEBUG
env var overrides are available for targeted fixes. For GOEXPERIMENT
regressions: rebuild with an explicit `GOEXPERIMENT` override or revert the
builder image.

## Infrastructure Needed

None. Existing CI, nightly build pipeline, and ART-managed builder images are
sufficient.

## References

| Reference | Link |
|-----------|------|
| KEP-3744: Stay on Supported Go Versions | https://github.com/kubernetes/enhancements/issues/3744 |
| Kubernetes blog: Keeping Kube Secure with Updated Go | https://kubernetes.io/blog/2023/04/06/keeping-kubernetes-secure-with-updated-go-versions/ |
| HPSTRAT-722: Use newer Golang versions for OCP | https://redhat.atlassian.net/browse/HPSTRAT-722 |

---

## Version-Specific Appendices

Each Go bump produces a version-specific appendix with behavioral changes,
FIPS impact analysis, dependency issues, and open questions.
Appendices are maintained as separate documents alongside this enhancement.

| Go bump | OCP release | Appendix |
|---------|-------------|----------|
| Go 1.25 → 1.26 | OCP 4.22 | [go-version-bump-go126.md](go-version-bump-go126.md) |
