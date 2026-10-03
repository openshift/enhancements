---
title: downstream-container-libs-vendoring-for-cri-o
authors:
  - "@bitoku"
reviewers:
  - TODO: Identify reviewers
approvers:
  - TODO: Identify approvers
api-approvers:
  - None
creation-date: 2026-07-23
last-updated: 2026-07-23
status: informational
tracking-link:
  - TODO: Add tracking link
see-also: []
replaces: []
superseded-by: []
---

# Downstream container-libs Vendoring for CRI-O

## Useful Links

- [Upstream CRI-O](https://github.com/cri-o/cri-o)
- [Downstream CRI-O](https://github.com/openshift/cri-o)
- [Upstream container-libs](https://github.com/podman-container-tools/container-libs)
- [CRI-O RPM repository for OCP 4.x](https://pkgs.devel.redhat.com/cgit/rpms/cri-o/) (requires VPN)
- [Downstream container-libs](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs) (requires VPN)
- [Downstream Podman](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/podman) (requires VPN)

## Summary

Starting with OCP 5.0, CRI-O is built from a downstream fork (`openshift/cri-o`) instead of from upstream `cri-o/cri-o`. Downstream CRI-O will vendor a downstream fork of [container-libs](https://github.com/podman-container-tools/container-libs) via Go module `replace` directives, ensuring CRI-O uses the same container library versions that ship with the target RHEL release.

## Motivation

container-libs (`podman-container-tools/container-libs`) is a monorepo consolidating three core Go libraries used across the container ecosystem (Podman, Buildah, CRI-O, Skopeo):

- **common** — shared configuration and utilities
- **storage** — container image and layer storage management
- **image** — container image and registry interaction

Each sub-library is versioned independently (e.g., `common/v0.68.1`), and the release cadence does not align with Kubernetes versions. Upstream Podman, upstream CRI-O, and the downstream RHEL/OCP builds all follow different release timelines, making synchronization an ongoing challenge.

The need to keep container-libs synchronized across container tools (Podman, Buildah, Skopeo, CRI-O) has been a requirement for multiple OpenShift releases, dating back to an incident where Podman and CRI-O running different container-libs versions caused production issues. This requirement has been complicated by CRI-O builds being pulled from upstream into the downstream build system, and by the downstream container-libs fork being hosted on an internal GitLab instance (gitlab.cee) that is not accessible from public repositories.

Previously, CRI-O was built directly from upstream `cri-o/cri-o` matching the OpenShift Kubernetes version. With OCP 5.0, we moved to a downstream fork (`openshift/cri-o`) carrying OpenShift-specific patches. Downstream CRI-O must vendor a downstream fork of container-libs to ensure consistency with RHEL's container tooling.

Each RHEL version ships a corresponding container-libs version. Since OCP releases target specific RHEL versions (e.g., OCP on RHEL 9.x vs. RHEL 10), the container-libs version vendored in CRI-O must match the target RHEL release.

A downstream container-libs fork will have its own release versioning distinct from upstream and is not expected to carry significant rebase work beyond bug fixes.

While this change does not affect user-facing features, it significantly impacts the CRI-O build and maintenance workflow — affecting dependency management, upstream rebases, and coordination between the Node and Container teams.

### User Stories

- As a Node team member, I want downstream CRI-O to vendor the downstream container-libs so that the container libraries match those shipped with the target RHEL release, avoiding incompatibilities between the container runtime and RHEL host configuration files (e.g., `storage.conf`, `containers.conf`, `registries.conf`, `policy.json`).

- As a Node team member or release engineer, I want the vendoring relationship managed via Go module `replace` directives so the build process is straightforward and reproducible, with no custom build tooling required beyond standard `go mod tidy` and `go mod vendor`.

- As a Node team member, I want a clear process for updating container-libs versions in downstream CRI-O so that when RHEL ships a new container-libs version, I know exactly which steps to follow — update `replace` directives, run `go mod tidy`/`go mod vendor`, submit a PR — and can do so without deep knowledge of the container-libs internals.

- As a Node team member who needs a container-libs feature or fix backported into CRI-O, I want a clear process to get the change into the downstream container-libs fork and subsequently into downstream CRI-O, so that fixes (e.g., a bug fix in `image` or `storage`) do not have to wait for a full upstream rebase cycle.

- As an SRE or support engineer managing clusters, I want CRI-O's container libraries to match those validated and shipped with RHEL so that the container runtime stack behaves consistently with RHEL's Podman, Buildah, and Skopeo — reducing the risk of configuration drift or unexpected behavior differences between container tools on the same node.

### Goals

1. Downstream CRI-O (`openshift/cri-o`) vendors the downstream container-libs fork via Go module `replace` directives for all three sub-libraries (`common`, `storage`, `image`).

2. The vendored container-libs version aligns with the version shipped in the target RHEL release.

3. The vendoring mechanism is transparent and maintainable.

### Non-Goals

1. Establishing the downstream container-libs fork itself (repository creation, governance, CI) — that is a separate effort.

2. Changing how other OpenShift components (Podman, Buildah, Skopeo) vendor container-libs. These components also consume container-libs and may need the same approach in the future for consistency, but that is out of scope here.

3. Maintaining full upstream rebase parity in downstream container-libs — only targeted bug fixes and approved backports are expected.

## Proposal

Downstream CRI-O (`openshift/cri-o`) will use Go module `replace` directives in its `go.mod` to redirect the three container-libs module paths to the downstream fork:

```go
replace (
    go.podman.io/common => gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs/common <version>
    go.podman.io/storage => gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs/storage <version>
    go.podman.io/image/v5 => gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs/image/v5 <version>
)
```

The `<version>` placeholders in the `replace` directives should be replaced with the specific version that corresponds to the target RHEL release (i.e., the same version that RHEL's Podman vendors for that release).

The upstream CRI-O module dependencies reference three container-libs submodules. The downstream fork preserves the same directory structure and tag conventions:

| Upstream module path | Example tag |
|---------------------|-------------|
| `go.podman.io/common` | `common/v0.67.1` |
| `go.podman.io/storage` | `storage/v1.62.0` |
| `go.podman.io/image/v5` | `image/v5.39.2` |

Note: `image` uses the `/v5` major version suffix per Go module conventions. The existing `go.podman.io/...` import paths in CRI-O source code remain unchanged; only the `replace` directives redirect resolution.

### Workflow Description

**OpenShift Node Team member** is a developer responsible for maintaining `openshift/cri-o`.

#### Scenario 1: Routine container-libs Update

When the Container team releases a new downstream container-libs version on the [downstream container-libs fork](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs), aligned with a RHEL release:

1. A Prow job detects the new downstream container-libs release, updates the `replace` directives in `openshift/cri-o`'s `go.mod`, runs `go mod tidy` and `go mod vendor`, and opens a PR with the updated vendor directory and `go.mod`/`go.sum`. The Prow job has access to `gitlab.cee` to pull the downstream container-libs source.
2. CI validates the build and runs tests, including a presubmit check that the vendored container-libs version matches the version shipped in Podman for the target RHEL release.
3. The PR is reviewed and merged.
4. ART automatically detects the change and builds a new CRI-O RPM.

#### Scenario 2: Upstream CRI-O Rebase

When downstream CRI-O is rebased to a new upstream release (e.g., `release-1.36` to `release-1.37`):

The container-libs `replace` directives are maintained as a carry patch. The rebase bot reapplies it automatically. If a conflict arises (e.g., upstream changed module paths), the maintainer resolves it and updates the `replace` directives to match the new OCP version's target RHEL release. CI validates that `go.sum`, `vendor/`, and `vendor/modules.txt` are consistent after the rebase.

#### Scenario 3: Security Fix

**Non-embargoed CVEs:** Same as Scenario 4 (Bug Fix / Feature Backport). The fix flows through the downstream container-libs fork and into `openshift/cri-o` via Scenario 1.

**Embargoed CVEs:** The fix cannot be pushed to any public repository (including a GitHub mirror of container-libs, if used) until the embargo is lifted. The process differs from the standard workflow:

During embargo:

1. The fix is developed in a private fork/branch of container-libs (see Open Question 3 for the exact location).
2. In `openshift-priv/cri-o` (the private mirror of `openshift/cri-o`), `go.mod` is updated with a `replace` directive pointing to the private fork containing the fix.
3. ART builds an embargoed CRI-O image from `openshift-priv/cri-o`.
4. The embargoed build is tested through the private pipeline (private Quay, private release controller, QE testing).

At embargo lift:

1. The public PR is merged into `podman-container-tools/container-libs`.
2. The fix is applied to the [downstream container-libs fork](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs) and a new version is tagged by the Container team.
3. The temporary `replace` directive in `openshift-priv/cri-o` is removed.
4. `openshift/cri-o` is updated via Scenario 1 to vendor the now-public downstream container-libs version.
5. `/publicize` on `openshift-priv/cri-o` merges the changes to public `openshift/cri-o`.
6. ART mirrors images to public Quay and the release goes public.

Note: Access to `openshift-priv` repos is restricted. The engineer handling the CVE must be assigned as the REP (Responsible Engineering Party) and request temporary access through Product Security.

Expected turnaround times for security fixes (based on Red Hat Product Security internal targets):

| Severity | Target |
|----------|--------|
| Critical | 5 business days |
| Important | 21-28 calendar days (45 days for OCP container image rebuilds) |
| Moderate | 57 calendar days |

For container-libs specifically, if the upstream containers maintainers need to develop and release the fix first, the clock does not start until the fix is available to vendor. The full chain for a container-libs CVE is: upstream fix → downstream container-libs fork → CRI-O vendor update → ART rebuild, which may be longer than for components where the fix is entirely within OpenShift's control.


#### Scenario 4: Bug Fix / Feature Backport

When a non-security fix or feature from upstream container-libs is needed in downstream CRI-O:

1. The Node team member identifies the upstream commit(s) needed.
2. The Node team member forks the [downstream container-libs repository](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs), cherry-picks the commit(s) onto the appropriate branch, and opens a Merge Request against the downstream repository.
3. The Container team reviews, merges, and tags a new downstream container-libs version.
4. Scenario 1 is followed to pull the change into downstream CRI-O.

Note: upstream container-libs does not accept backport branches from external teams (see [PR #984](https://github.com/podman-container-tools/container-libs/pull/984)). All backports must go through the downstream repository.

### API Extensions

None. No CRDs, webhooks, aggregated API servers, or finalizers are involved.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No impact. This is a build-time vendoring change. CRI-O runs on worker nodes regardless of control plane topology.

#### Standalone Clusters

No impact beyond the CRI-O binary shipping with updated container-libs.

#### Single-node Deployments or MicroShift

No impact. The change is limited to which container-libs version is compiled into the CRI-O binary.

#### OpenShift Kubernetes Engine

No impact. CRI-O is part of the base platform available to both OCP and OKE.

### Implementation Details/Notes/Constraints

#### Version Mapping

Starting with OCP 5.0:

| OCP Version | openshift/cri-o branch | Upstream cri-o/cri-o branch |
|-------------|------------------------|-----------------------------|
| 5.0         | main (then release-5.0 after branching) | release-1.36 |
| 5.1         | main (then release-5.1 after branching) | release-1.37 |

The upstream CRI-O release branch tracked by `openshift/cri-o` is determined by the Node team for each OCP release. When downstream branching occurs, a release-specific branch is created from `main`.

Currently, the container-libs version for a given RHEL release is determined by checking the Podman spec file (`podman.spec`) in the corresponding dist-git branch, which specifies the upstream branch and commit:

```spec
%global import_path github.com/containers/podman
%global branch v5.4-rhel
%global commit0 52090f82124c248a84ce5d8a97ae11c56e26755c
```

The container-libs version vendored in that Podman commit is the version CRI-O should match.

The following table shows the RHEL version to container-libs (common, image, storage) version mapping. The Podman versions are based on the [Container-tools: Component Version Consolidation](https://docs.google.com/document/d/1RA-xEIVDNayvPiYUrGnp6mSCt11VvBEsZxTt7yYnmUI/edit?usp=sharing) proposal except where noted.

| RHEL | Podman | common | image | storage | OCP (reference) |
|------|--------|--------|-------|---------|-----------------|
| rhel-9.2.0.z | 5.4.0 | v0.62.3 | v5.34.3 | v1.57.2 | 4.13, 4.14, 4.15 |
| rhel-9.4.z | 5.4.0 | v0.62.3 | v5.34.3 | v1.57.2 | 4.16 |
| rhel-9.6.z | 5.4.0 | v0.62.3 | v5.34.3 | v1.57.2 | 4.19, 4.20, 4.21 |
| rhel-9.8.z | 5.8.2† | v0.67.1 | v5.39.2 | v1.62.0 | 4.22, 4.23 |
| rhel-10.2.z | 5.8.2† | v0.67.1 | v5.39.2 | v1.62.0 | 4.21, 4.22 |

† Podman version not listed in the consolidation proposal. May change with OCP z-stream updates. To be confirmed with the Container-tools team.

Note: OCP z-stream updates may shift the underlying RHEL minor version (e.g., OCP 4.21.0 ships on RHEL 10.1 while later z-streams ship on RHEL 10.2). The RHEL version is identifiable by the `.el` suffix on RPM packages (e.g., `.el9_8`, `.el10_2`).

For the full container-tools version consolidation plan (including RHEL 8.x, OpenStack, golang, Skopeo, and Buildah mappings), see the [Container-tools: Component Version Consolidation](https://docs.google.com/document/d/1RA-xEIVDNayvPiYUrGnp6mSCt11VvBEsZxTt7yYnmUI/edit?usp=sharing).

#### Go Module Replace Directives

The `replace` directives are a standard Go modules feature that redirect module resolution at build time without changing import paths. This means:

- No source code changes in CRI-O are needed beyond `go.mod`.
- The `vendor/` directory will contain the downstream container-libs source.
- `go mod tidy` and `go mod vendor` handle the update.

#### CI Access to gitlab.cee

Prow CI jobs for `openshift/cri-o` require network access to `gitlab.cee.redhat.com` when vendoring the downstream container-libs source. This is achieved by adding the `intranet` capability to the ci-operator configuration for `openshift/cri-o`. The `intranet` capability schedules CI pods on clusters with Red Hat intranet access, allowing `go mod` operations to reach `gitlab.cee`. The build environment sets `GOPRIVATE=gitlab.cee.redhat.com/*` to prevent Go from routing requests to the public module proxy or checksum database.

Note: No additional Git credentials are required. `gitlab.cee.redhat.com` allows anonymous read access from within the Red Hat intranet. The `intranet` capability provides network routing and `GOPRIVATE` bypasses the public module proxy. Validated in [openshift/release#82557](https://github.com/openshift/release/pull/82557).

#### Downstream container-libs Repository

The downstream repository is hosted at [gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs). It is an RHEL src-git repository that mirrors upstream `podman-container-tools/container-libs` (GitHub → GitLab, one-way sync). Protected branches (`main`, `podman-6.0`, `podman-5.8`, etc.) are automatically synced from upstream and cannot be modified directly.

### Risks and Mitigations

**Risk:** Downstream container-libs diverges significantly from upstream, making it difficult to incorporate upstream improvements.
**Mitigation:** The downstream fork is scoped to carry only bug fixes, limiting divergence. Periodic review of upstream changes ensures important fixes are not missed.

**Risk:** A container-libs update introduces a regression in downstream CRI-O.
**Mitigation:** This risk exists today with upstream vendoring. Revert to the previous container-libs version by updating the `replace` directives in `go.mod` to pin the previous version, running `go mod tidy` and `go mod vendor`, and releasing a new CRI-O build via ART. If a storage format issue arises, existing CRI-O upgrade procedures apply.

**Risk:** The `replace` directives conflict with other downstream patches in `openshift/cri-o` that also modify `go.mod`.
**Mitigation:** Go module `replace` directives are additive and well-understood. Conflicts are resolved through standard `go.mod` merge practices.

**Risk:** If a mirroring approach is used to bridge `gitlab.cee` to GitHub, the mirror can break silently when one side gets too far out of sync. GitLab mirroring has no built-in notification mechanism for sync failures — a human must manually check for breakage.
**Mitigation:** Implement monitoring for the sync job (e.g., alerting on sync age or failure). Enforce the mirror as read-only on the GitHub side to prevent bidirectional desync. Test the mirroring setup on non-production repositories before adopting it for container-libs.

**Risk:** Embargoed CVE fixes cannot be pushed to a public mirror until the embargo is lifted, creating a window where downstream CRI-O builds with the fix cannot be tested through public CI.
**Mitigation:** During embargoes, the fix is applied and tested through internal build and CI processes (see Scenario 3: Security Fix for the detailed embargo workflow). The public mirror (if used) and `openshift/cri-o` are updated once the embargo is lifted.

**Risk:** Release timing misalignment between upstream Podman, upstream CRI-O, and downstream RHEL/OCP builds causes container-libs version drift across container tools on the same node.
**Mitigation:** A CI check or `gitlab.cee` pipeline can alert when the container-libs version in CRI-O diverges from the version shipped in Podman for the same RHEL release, providing early warning before the drift causes runtime issues.

**Risk:** Container-libs versions diverge across RHEL major version boundaries during dual-stream periods (e.g., OCP targeting both RHEL 10 and RHEL 11), requiring `openshift/cri-o` to support two different container-libs versions from a single branch.
**Mitigation:** The RHEL team is responsible for maintaining container-libs version compatibility across dual-stream RHEL versions. No special handling is expected in `openshift/cri-o` — a single set of `replace` directives should work for both RHEL version RPM builds.

**Risk:** Moving to a downstream fork changes CRI-O's versioning scheme from upstream-aligned (e.g., `1.36.x`) to the OpenShift NVR (Name-Version-Release) scheme (e.g., `openshift-5.0.0-202606091635.p2.gd8d517e.assembly.stream.el9`). Tooling and processes that parse the CRI-O version string — `crictl version` output, monitoring dashboards, must-gather scripts, alerting rules, documentation, and support procedures — may break or produce incorrect results.
**Mitigation:** Audit all tooling that parses CRI-O version strings and update to handle the NVR format.


### Drawbacks

- Maintaining a downstream container-libs fork adds maintenance burden, even if limited to bug fixes.
- The `replace` directives diverge from upstream CRI-O's `go.mod`, requiring management during upstream rebases.
- The downstream container-libs fork lives on internal GitLab (`gitlab.cee`), which is not accessible from public GitHub repositories. Bridging this gap — whether through mirroring, build-time patching, or another mechanism — adds infrastructure complexity and introduces failure modes around synchronization and embargoed security fixes.

## Alternatives (Not Implemented)

### Vendor upstream container-libs directly

Continue vendoring upstream `podman-container-tools/container-libs` in downstream CRI-O without a downstream fork.

Pros:
- No downstream fork to maintain — eliminates the maintenance burden of a separate container-libs repository.
- No accessibility constraint — upstream container-libs is publicly available, so `replace` directives and CI work without mirroring or patching.
- Simpler upstream rebases — no `replace` directive management when rebasing CRI-O to a new upstream release.

Cons:
- No guarantee of alignment with the RHEL-shipped container-libs version, risking inconsistencies between CRI-O and other container tools (Podman, Buildah, Skopeo) on the same node.
- Upstream container-libs releases may include changes not yet validated for the target RHEL version.
- Bug fixes and backports needed for RHEL must go through upstream first, which may not align with RHEL release timelines.

### Use a Go module proxy

Configure a Go module proxy (via `GOPROXY`) to serve the desired container-libs versions to CRI-O builds, instead of maintaining a downstream fork with `replace` directives.

Pros:
- No `replace` directives in `go.mod` — the proxy transparently serves the right version, simplifying upstream rebases.
- Solves the `gitlab.cee` accessibility problem without mirroring infrastructure.

Cons:
- Does not eliminate the downstream fork — the core requirement is serving *patched* container-libs, so a forked source is still needed. The proxy becomes an additional layer rather than a replacement.
- Introduces a single point of failure — if the proxy is unavailable, builds break. The vendored-source approach is self-contained.
- Requires authentication and `GOPROXY` configuration across all build environments (Prow, Brew/OSBS, developer workstations).
- Complicates embargoed CVE handling — the proxy must enforce access control to prevent serving patched versions before disclosure.
- No such proxy exists today — building and operating one is significant infrastructure investment for a problem that `replace` directives already solve.

### Apply an internal go.mod patch during RPM builds

Instead of maintaining `replace` directives in the public `openshift/cri-o` repository, keep upstream container-libs references in the public `go.mod` and maintain a patch on `gitlab.cee` that adds the `replace` directives pointing to the internal fork. The patch is applied during RPM builds (in the dist-git prep step).

Pros:
- No public mirroring infrastructure needed — avoids the complexity and fragility of syncing between `gitlab.cee` and GitHub.
- Embargoed CVE fixes stay internal — no risk of leaking fixes to a public mirror before disclosure.
- Simple to implement — a single patch file on `gitlab.cee`, applied during the existing dist-git prep step.
- A `gitlab.cee` pipeline can alert when the container-libs version in CRI-O drifts from the version in Podman's src-git.

Cons:
- CI on GitHub tests against upstream container-libs, not downstream — behavioral differences between upstream and downstream would not be caught until RPM builds.
- The patch must be kept in sync with changes to `go.mod` in `openshift/cri-o` — upstream rebases or dependency updates can silently break the patch.
- Adds an invisible build-time dependency that is not reflected in the public repository, making the build harder to reproduce externally.

## Open Questions

1. What is the RHEL version to container-libs version mapping? 
- Addressed in the Version Mapping section. Post-consolidation values for rhel-9.2/9.4 and actual values for rhel-9.8/10.2 to be confirmed with the Container-tools team.
2. What is the expected turnaround time for security fixes?
- SLA targets documented in Scenario 3. To be confirmed with the security response team.
3. What is the exact location where embargoed container-libs fixes are developed (gitlab.cee downstream fork, a private GitHub fork, or upstream private branch)?
4. How does dual-stream (e.g., OCP targeting both RHEL 10 and RHEL 11) work with a single `go.mod` — does each RHEL target get a separate RPM build with target-specific `replace` directives, or are they expected to use the same container-libs version? 
- Note: dual-stream is an existing OCP mechanism, not introduced by this enhancement.
5. Where should backports and bug fixes be applied in the downstream container-libs repository? The protected branches (`main`, `podman-5.8`, `podman-6.0`) are mirrored from upstream and cannot be modified directly. Does the Container team maintain separate RHEL-specific branches (e.g., `v5.8-rhel`) for downstream patches? What branch/tag should CRI-O's `replace` directives target?

## Test Plan

CI coverage will validate CRI-O built with the downstream container-libs:

- **Pre-merge:** Pre-submit CI (build and e2e) and payload-jobs run against each PR to `openshift/cri-o`.
- **Post-merge:** Periodic CI jobs provide ongoing regression coverage.

### Detecting container-libs Version Divergence

A presubmit Prow job on `openshift/cri-o` checks that the container-libs version vendored in CRI-O matches the version RHEL ships in Podman. The job compares the container-libs module versions in CRI-O's `go.mod` against Podman's `go.mod` on the same RHEL release. The Prow job has access to `gitlab.cee` to retrieve the Podman source for comparison.

Pros:
- Simple to implement — a script comparing version strings in `go.mod` files.
- Fast execution — no runtime dependencies or cluster resources needed.
- Catches divergence early, before it causes runtime issues.

Cons:
- May produce false positives — version differences do not always indicate behavioral incompatibilities.
- OCP release will be blocked until the container-libs version is updated.

### Sync Drift Detection via Internal Pipeline

Complement the CI test above with a `gitlab.cee` pipeline that monitors the container-libs version used in CRI-O's vendor directory against the version in Podman's src-git for the same RHEL release. This pipeline can alert when drift occurs, even outside of CI runs, providing early warning before release-timing misalignment causes issues.

## Graduation Criteria

This is an informational enhancement describing a build-process change. No feature gates or graduation milestones apply. The change is effective once downstream CRI-O is built with the downstream container-libs `replace` directives.

### Dev Preview -> Tech Preview

N/A — build-process change, not a user-facing feature.

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

Transparent to cluster upgrades and downgrades. Each OCP release ships a CRI-O binary with its vendored dependencies bundled at build time. Upgrading or downgrading OCP replaces the CRI-O binary entirely. No cluster-level configuration changes are required.

### RHEL Version Switch (Dual Stream)

When a cluster transitions from one RHEL version to another (e.g., RHEL 10 to RHEL 11) within the same OCP version, nodes are reprovisioned with a new RHCOS image. The new image includes the CRI-O RPM built for the target RHEL version, with the corresponding container-libs vendored. Reprovisioning replaces the OS image — including the CRI-O binary and default configuration files. However, custom node configurations delivered via MachineConfig (e.g., custom `storage.conf` or `containers.conf`) persist across reprovisioning. If a new container-libs version changes config file format or semantics, those custom configs may need to be updated.

During rolling reprovisioning, the cluster temporarily has nodes running CRI-O built with different container-libs versions (e.g., some nodes on RHEL 10's container-libs, others on RHEL 11's). This is safe because the CRI gRPC API between kubelet and CRI-O is unchanged by the container-libs version — the difference is internal to the CRI-O binary.

## Version Skew Strategy

CRI-O is a node-level component. During upgrades, nodes may temporarily run different CRI-O versions. This is unchanged by this proposal — container-libs vendoring is internal to the CRI-O binary and does not affect inter-component communication or APIs.

During dual-stream RHEL transitions, nodes may run CRI-O binaries built with different container-libs versions. This does not introduce inter-node version skew concerns because container-libs is statically linked into CRI-O and does not affect inter-component APIs. On-disk container storage formats managed by the `storage` library are local to each node; if a container-libs update changes the on-disk format, the impact is limited to the upgraded node and handled by CRI-O's existing storage migration logic.

## Operational Aspects of API Extensions

N/A — no API extensions.

## Support Procedures

This change does not introduce new failure modes visible to cluster administrators. If a CRI-O issue is traced to a container-libs regression:

1. Identify the container-libs version vendored in the CRI-O build (check `go.mod` in `openshift/cri-o` for the relevant release branch).
2. Compare with the upstream container-libs version to determine if the issue is downstream-specific.
3. Raise an issue with the Node team.
4. The Node team evaluates and, if the issue originates in container-libs, escalates to the Container team.

## Infrastructure Needed

- The downstream container-libs repository at [gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/container-libs).
- The `intranet` capability added to `openshift/cri-o` ci-operator configuration, granting CI jobs network access to `gitlab.cee.redhat.com` for pulling downstream container-libs source during vendoring.
- A Prow job that monitors downstream container-libs releases, runs `go mod tidy`/`go mod vendor`, and opens PRs to `openshift/cri-o` with the updated vendored source.
- A presubmit Prow job on `openshift/cri-o` that validates the vendored container-libs version matches the version shipped in Podman for the target RHEL release.
