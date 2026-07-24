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
- [Downstream Podman](https://gitlab.cee.redhat.com/sustaining-engineering/container-tools/src-git/podman) (requires VPN)

## Summary

Starting with OCP 5.0, CRI-O is built from a downstream fork (`openshift/cri-o`) instead of from upstream `cri-o/cri-o`. Downstream CRI-O will vendor a downstream fork of [container-libs](https://github.com/podman-container-tools/container-libs) via Go module `replace` directives, ensuring CRI-O uses the same container library versions that ship with the target RHEL release.

## Motivation

container-libs (`podman-container-tools/container-libs`) is a monorepo consolidating three core Go libraries used across the container ecosystem (Podman, Buildah, CRI-O, Skopeo):

- **common** — shared configuration and utilities
- **storage** — container image and layer storage management
- **image** — container image and registry interaction

Each sub-library is versioned independently (e.g., `common/v0.68.1`), and the release cadence does not align with Kubernetes versions.

Previously, CRI-O was built directly from upstream `cri-o/cri-o` matching the OpenShift Kubernetes version. With OCP 5.0, we moved to a downstream fork (`openshift/cri-o`) carrying OpenShift-specific patches. Downstream CRI-O must vendor a downstream fork of container-libs to ensure consistency with RHEL's container tooling.

Upstream container-libs can introduce breaking changes not aligned with RHEL. For example, [common/v0.68.0](https://github.com/podman-container-tools/container-libs/releases/tag/common%2Fv0.68.0) overhauled configuration file lookup, ported `storage.conf`, `containers.conf`, `registries.conf`, and `policy.json` to a new unified parser, and added drop-in configuration support. Such changes could cause incompatibilities between the container runtime and RHEL host configuration files. A downstream fork ensures CRI-O uses the RHEL-validated container-libs version.

Each RHEL version ships a corresponding container-libs version. Since OCP releases target specific RHEL versions (e.g., OCP on RHEL 9.x vs. RHEL 10), the container-libs version vendored in CRI-O must match the target RHEL release.

TODO: Determine the exact RHEL-to-OCP version mapping with stakeholders.

A downstream container-libs fork will have its own release versioning distinct from upstream and is not expected to carry significant rebase work beyond bug fixes.

While this change does not affect user-facing features, it significantly impacts the CRI-O build and maintenance workflow — affecting dependency management, upstream rebases, and coordination between the Node and Container teams.

### User Stories

- As a CRI-O maintainer, I want downstream CRI-O to vendor the downstream container-libs so that the container libraries match those shipped with the target RHEL release.

- As an OpenShift release engineer, I want the vendoring relationship managed via Go module `replace` directives so the build process is straightforward and reproducible.

- As an OpenShift Node team member, I want a clear process for updating container-libs versions in downstream CRI-O so that upstream and RHEL bug fixes can be incorporated efficiently.

- As an SRE managing OpenShift clusters, I want CRI-O's container libraries to match those validated and shipped with RHEL for a consistent, supported container runtime stack.

### Goals

1. Downstream CRI-O (`openshift/cri-o`) vendors the downstream container-libs fork via Go module `replace` directives for all three sub-libraries (`common`, `storage`, `image`).

2. The vendored container-libs version aligns with the version shipped in the target RHEL release.

3. The vendoring mechanism is transparent and maintainable, requiring only `go.mod` changes without custom build tooling.

### Non-Goals

1. Establishing the downstream container-libs fork itself (repository creation, governance, CI) — that is a separate effort.

2. Changing how other OpenShift components (Podman, Buildah, Skopeo) vendor container-libs. These components also consume container-libs and may need the same approach in the future for consistency, but that is out of scope here.

3. Maintaining full upstream rebase parity in downstream container-libs — only bug fixes are expected.

## Proposal

Downstream CRI-O (`openshift/cri-o`) will use Go module `replace` directives in its `go.mod` to redirect the three container-libs module paths to the downstream fork.

The upstream CRI-O module dependencies reference:

- `github.com/podman-container-tools/container-libs/common`
- `github.com/podman-container-tools/container-libs/storage`
- `github.com/podman-container-tools/container-libs/image`

The downstream `go.mod` will add `replace` directives:

```go
replace (
    github.com/podman-container-tools/container-libs/common => <downstream-repo>/common <version>
    github.com/podman-container-tools/container-libs/storage => <downstream-repo>/storage <version>
    github.com/podman-container-tools/container-libs/image => <downstream-repo>/image <version>
)
```

TODO: Determine the downstream repository location.

### Workflow Description

**CRI-O downstream maintainer** is a developer responsible for maintaining `openshift/cri-o`.

1. A new downstream container-libs release is tagged, aligned with the target RHEL version.
2. The `replace` directives in `openshift/cri-o`'s `go.mod` are updated to reference the new version. TODO: Evaluate automating this step (e.g., a bot that opens a PR when a new downstream container-libs release is tagged). In the interim, a CRI-O downstream maintainer performs this manually.
3. The maintainer runs `go mod tidy` and `go mod vendor`.
4. The maintainer submits a PR to `openshift/cri-o` with the updated vendor directory and `go.mod`/`go.sum`.
5. CI validates the build and runs tests.
6. The PR is reviewed and merged.
7. ART automatically detects the change and builds a new CRI-O RPM.

#### Upstream CRI-O Rebase Workflow

When downstream CRI-O is rebased to a new upstream release (e.g., `release-1.36` to `release-1.37`):

1. Remove the existing downstream `replace` directives for container-libs.
2. Add new `replace` directives pointing to the container-libs version for the target RHEL release of the new OCP version.
3. Run `go mod tidy` and `go mod vendor`.
4. Standard PR and CI process follows.

#### Security Fix Workflow

When a CVE or security fix is needed in container-libs:

1. The fix is applied to the downstream container-libs fork (backported from upstream if applicable).
2. A new downstream container-libs version is tagged.
3. The container-libs version update workflow (above) is followed to pull the fix into downstream CRI-O.

TODO: Clarify the expected turnaround time for security fixes and whether there is a fast-track process.

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

TODO: Add downstream container-libs version to RHEL version mapping once determined.

TODO: Determine how we handle dualstream (OCP 5.0 - 5.2 will target both RHEL 9.8 and RHEL 10.2).

#### Versioning Scheme Change

Moving to a downstream fork changes CRI-O's versioning scheme. Previously, versions aligned with upstream (e.g., `1.36.x`). With the downstream fork, CRI-O follows the OpenShift NVR (Name-Version-Release) scheme, similar to kubelet:

```
openshift-5.0.0-202606091635.p2.gd8d517e.assembly.stream.el9
```

This affects tooling and processes that parse the CRI-O version string:

- `crictl version` output shows the NVR string instead of `1.36.x`.
- Monitoring dashboards, must-gather scripts, and alerting rules matching on CRI-O version patterns may need updates.
- Documentation and support procedures referencing CRI-O versions must account for the new scheme.

TODO: Confirm acceptability of this versioning scheme with stakeholders. Identify all tooling that parses CRI-O version strings and assess the impact.

#### Go Module Replace Directives

The `replace` directives are a standard Go modules feature that redirect module resolution at build time without changing import paths. This means:

- No source code changes in CRI-O are needed beyond `go.mod`.
- The `vendor/` directory will contain the downstream container-libs source.
- `go mod tidy` and `go mod vendor` handle the update.

#### Downstream container-libs Fork

TODO: Determine the downstream repository location.

The downstream fork:
- Will be owned by the Container team.
- Will be a proper Go module with its own `go.mod` and module path.
- Will have its own release versioning distinct from upstream.
- Is not expected to carry significant upstream rebase work beyond bug fixes.
- Each RHEL version will have a corresponding container-libs version.

TODO: Determine the RHEL version to container-libs version mapping.

### Risks and Mitigations

**Risk:** Downstream container-libs diverges significantly from upstream, making it difficult to incorporate upstream improvements.
**Mitigation:** The downstream fork is scoped to carry only bug fixes, limiting divergence. Periodic review of upstream changes ensures important fixes are not missed.

**Risk:** A container-libs update introduces a regression in downstream CRI-O.
**Mitigation:** Revert to the previous container-libs version via a rollback patch and release a new CRI-O build. TODO: Define the specific rollback process.

**Risk:** The `replace` directives conflict with other downstream patches in `openshift/cri-o` that also modify `go.mod`.
**Mitigation:** Go module `replace` directives are additive and well-understood. Conflicts are resolved through standard `go.mod` merge practices.

### Drawbacks

- Maintaining a downstream container-libs fork adds maintenance burden, even if limited to bug fixes.
- The `replace` directives diverge from upstream CRI-O's `go.mod`, requiring management during upstream rebases.

## Alternatives (Not Implemented)

### Vendor upstream container-libs directly

Continue vendoring upstream `podman-container-tools/container-libs` in downstream CRI-O. Rejected because it would not guarantee alignment with the RHEL-shipped container-libs version, risking inconsistencies in the container runtime stack.

### Use a Go module proxy

Configure a Go module proxy to serve the desired container-libs versions instead of maintaining a downstream fork. TODO: Explore feasibility around version pinning, authentication, and build infrastructure integration.

### Use RPM dependencies instead of Go vendoring

Link against container-libs via RPM dependencies at the system level instead of Go module `replace` directives. Rejected because container-libs are statically linked into the CRI-O binary via Go vendoring.

## Open Questions

1. What is the repository location for the downstream container-libs fork? Need to check with the Container team on repo location, accessibility, and release cycle.
2. What is the RHEL version to container-libs version mapping? Which RHEL version does each OCP release target (e.g., OCP 5.0 on RHEL 9.x vs. RHEL 10)?
3. Will other OpenShift components (Podman, Buildah, Skopeo) also switch to the downstream container-libs fork?
4. Can the container-libs version update in downstream CRI-O be automated (e.g., a bot that opens a PR on new downstream container-libs releases)?
5. Is the OpenShift NVR versioning scheme acceptable for CRI-O? What tooling (crictl, monitoring, must-gather) needs updates?
6. Is a Go module proxy a viable alternative to maintaining a downstream fork?
7. What is the expected turnaround time for security fixes flowing from upstream to downstream container-libs to downstream CRI-O?
8. What is the impact of a time gap between CRI-O adopting a newer container-libs version and other container tools (Podman, Buildah, Skopeo) on the same node still running an older version? Could this cause configuration or behavior inconsistencies?
9. How do we handle version skew between the CRI-O binary (with its vendored container-libs) and the node image during upgrades?

## Test Plan

No new tests are planned. Existing CI coverage validates CRI-O built with the downstream container-libs:

- **Pre-merge:** Pre-submit CI (build and e2e) and payload-jobs run against each PR to `openshift/cri-o`.
- **Post-merge:** Periodic CI jobs provide ongoing regression coverage.

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

## Version Skew Strategy

CRI-O is a node-level component. During upgrades, nodes may temporarily run different CRI-O versions. This is unchanged by this proposal — container-libs vendoring is internal to the CRI-O binary and does not affect inter-component communication or APIs.

## Operational Aspects of API Extensions

N/A — no API extensions.

## Support Procedures

This change does not introduce new failure modes visible to cluster administrators. If a CRI-O issue is traced to a container-libs regression:

1. Identify the container-libs version vendored in the CRI-O build (check `go.mod` in `openshift/cri-o` for the relevant release branch).
2. Compare with the upstream container-libs version to determine if the issue is downstream-specific.
3. Raise an issue with the Node team.
4. The Node team evaluates and, if the issue originates in container-libs, escalates to the Container team.

## Infrastructure Needed

- A downstream container-libs repository (TODO: determine location).
- CI configuration for `openshift/cri-o` to build and test with the downstream container-libs vendored.
