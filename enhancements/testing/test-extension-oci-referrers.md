---
title: test-extension-oci-referrers
authors:
  - "@sdodson"
reviewers:
  - "@jupierce" # Test extension framework and release testing
  - "@stbenjam" # Origin test execution and extension adoption
approvers:
  - "@deads2k" # Approver of the original test extension enhancement
api-approvers:
  - None
creation-date: 2026-09-29
last-updated: 2026-09-29
status: provisional
tracking-link:
  - https://github.com/openshift/cluster-image-registry-operator/pull/1382
see-also:
  - "/enhancements/testing/openshift-tests-extension.md"
---

# Distribute OpenShift test extensions as OCI referrers

## Summary

Move compressed test extension binaries out of component runtime images and
publish them as OCI 1.1 referrers of those images. `openshift-tests` will find
the extension belonging to the exact component image in the release, download
and validate it, and continue to use the existing extension command protocol.
The migration includes release promotion, disconnected mirroring, retention,
and deletion of old extension artifacts.

## Motivation

The [original enhancement](openshift-tests-extension.md) places compressed
extension binaries inside component images so `openshift-tests` can extract
them. This couples a test-only, often large binary to the operator image used
at runtime. An image referrer can carry the binary in the same repository
without adding it to the runtime image layers. A
[registry operator proof of concept](https://github.com/openshift/cluster-image-registry-operator/pull/1382)
has published and retrieved such an artifact on Quay.

Separating the binary changes more than extraction. The artifact must survive
image promotion and mirroring, remain associated with the exact image digest,
have a trustworthy publisher, and be removed only after all supported releases
and rollback paths no longer need it.

### User Stories

- As a component developer, I want to publish tests with my component build
  without putting the test binary in the runtime image.
- As a release engineer, I want to know that every promoted component image
  has its required test extension at every registry destination.
- As a disconnected cluster tester, I want `openshift-tests` to retrieve the
  same tests from my mirror without reaching the source registry.
- As a registry operator, I want old extension artifacts removed after their
  supported releases expire, while retaining artifacts needed for rollback.

### Goals

- Preserve the existing test extension CLI, test IDs, and suite behavior.
- Bind one extension binary to the exact image manifest and architecture it
  tests, including after promotion or mirroring.
- Make missing, duplicate, or invalid artifacts visible as test setup errors.
- Keep release and mirror storage bounded with a safe retention process.
- Permit mixed releases while the consumer and publishers migrate.

### Non-Goals

- Change how tests run after an extension binary has been extracted.
- Make extension artifacts available to production workloads or add cluster
  APIs to configure them.
- Replace admission of non-payload extensions or existing registry security
  policy.
- Define a general retention policy for every OCI referrer type.

## Proposal

Each component build publishes its runtime image first. It then pushes a
compressed extension as an OCI image manifest whose `subject` is that image
manifest digest. The artifact lives in the same registry repository. It uses
the artifact type and filename annotation described in
[the extension contract draft](https://github.com/openshift-eng/openshift-tests-extension/pull/81).
The publisher records the subject, artifact, and blob digests as build output.

The release process promotes the image and its extension together and verifies
the relationship at the destination. Image copies alone do not imply referrer
copies: OCI referrers are queried by repository and subject digest. If image
conversion changes the subject digest, promotion must create a new artifact
manifest with the destination digest as its subject. The gzip blob can remain
byte-identical. Promotion must fail before release acceptance if a required
extension is absent or cannot be fetched.

`openshift-tests` discovers the artifact using the resolved component image
digest and the expected gzip filename. It verifies the artifact's subject,
type, layer media type, filename, size, and digests, then decompresses it and
checks CPU architecture as it does today. With no matching referrer, it uses
the existing in-image extraction path during migration. A listed but invalid
or inaccessible artifact is an error. Duplicate matches are an error.

The [Origin consumer draft](https://github.com/openshift/origin/pull/31686)
implements the initial lookup and legacy fallback. The
[registry operator publisher draft](https://github.com/openshift/cluster-image-registry-operator/pull/1382)
demonstrates one image and architecture. These are prototypes for the
cross-repository release workflow described here; they are not, by themselves,
the gate for removing in-image extensions from supported releases.

### Workflow Description

1. A component build produces a runtime image and an extension gzip from the
   same source revision. It pushes the image, resolves its digest, and attaches
   exactly one artifact for each advertised extension filename.
2. CI discovers the artifact by the subject digest, downloads it, checks its
   blob digest, and runs extension `info` and `list` before accepting the build.
3. Release promotion copies the image and all required extension artifacts to
   each release registry repository. It records source and destination digests
   and verifies discovery and execution at the destination.
4. Disconnected mirroring copies the same relationship into the mirror
   repository. The test runner uses the mirrored image reference and registry
   credentials, so it needs no connection to the source registry.
5. `openshift-tests` caches a validated binary by artifact digest. A newer
   artifact on the same subject cannot silently reuse stale cached bytes.
6. A release-aware retention job deletes an old extension manifest only after
   the subject is no longer referenced by any retained release or required
   rollback or mirror inventory. The registry reclaims its blobs when no
   remaining manifest references them.

### API Extensions

There are no Kubernetes API changes. The new interface is an OCI artifact
contract between component publishers, release tooling, mirrors, registries,
and `openshift-tests`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The artifact is fetched by the test runner from the registry containing the
component image selected for the hosted cluster under test. A management
cluster and guest cluster may use different mirrors and credentials; the
runner must use the image reference and auth for its chosen payload. No new
control plane process runs in either cluster.

#### Standalone Clusters

The standard payload image reference determines the artifact lookup. A mirror
used by the cluster or test job must contain the image and matching referrer.

#### Single-node Deployments or MicroShift

No resident process or cluster API is added. Tests still need scratch space
for the downloaded and decompressed binary. The runner should continue to
bound and report its disk use, especially on single-node systems.

#### OpenShift Kubernetes Engine

The distribution method applies only when the corresponding test extension
and `openshift-tests` run is available. It does not depend on OCP-only APIs.

### Implementation Details/Notes/Constraints

**Artifact format.** Use OCI image manifest media type, artifact type
`application/vnd.openshift.tests-extension.v1+gzip`, and one
`application/gzip` layer with `org.opencontainers.image.title` set to the
advertised gzip basename. The subject is the exact component image manifest
digest in the same repository. Multiple filenames may have separate
artifacts; two matching artifacts for the same subject and filename are an
error. An index needs platform-specific handling: the publisher attaches an
artifact to each platform image manifest, and the consumer selects the
platform manifest before lookup. The current single-platform POC does not
cover this requirement.

**Discovery and integrity.** The consumer uses the OCI Distribution 1.1
referrers endpoint, with the spec's referrers-tag fallback where needed. It
checks the fetched manifest's own `subject` rather than trusting only a
registry-provided list. It limits metadata and blob sizes, verifies digests,
and avoids following registry-supplied pagination URLs to unrelated hosts.
The test extension filename comes from the existing payload or ImageStream
registration; it is not used as an arbitrary download path.

**Promotion and mirrors.** The release workflow must preserve the destination
image manifest digest where possible. When it cannot, it must reattach the
artifact to the new destination digest and record both digests. Image and
extension publication must be treated as one release acceptance unit. Mirror
tooling must either copy referrers and their retention references or report
the release as unsupported for this format. A copy is verified by querying
the destination registry, not by assuming a successful image copy included
the artifact.

**Retention and garbage collection.** The
[OCI Distribution specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers)
defines referrer discovery and manifest deletion, but does not define a
portable cascade from deleting a subject to deleting its referrers (see the
[open specification discussion](https://github.com/opencontainers/distribution-spec/issues/378)).
[Quay has asynchronous garbage collection and blob expiration](https://docs.redhat.com/en/documentation/red_hat_quay/3.18/html/manage_red_hat_quay/garbage-collection).
The
release workflow therefore owns the retention decision; it must not rely on
an untagged artifact surviving solely because its subject is tagged, nor
assume the registry will remove it when the subject disappears. Each promoted
artifact receives a deterministic retention tag derived from the subject
digest and extension filename. The tag is a reachability anchor, while
referrer discovery remains the consumer interface.

The cleanup job operates from an inventory of promoted release and mirror
references, not tag age alone. It first computes a dry-run list of artifact
digests that are outside supported release, rollback, and mirror retention
windows. After a grace period, it removes only managed retention tags and
extension artifact manifests listed in that inventory. It never deletes a
subject image or a raw blob directly. It verifies the artifact type and
subject again before deletion, and lets each registry reclaim shared blobs
under its own garbage collector. A failed or incomplete inventory prevents
deletion. Disconnected mirror operators need the same inventory and a local
cleanup procedure; upstream cleanup cannot delete their copies.

**Trust.** Blob digest verification detects corruption, but it does not prove
that an artifact was published by the same party as the image. Extension
binaries can receive a privileged test kubeconfig. The release process must
allow only trusted component publishers to attach accepted artifacts and
record the expected artifact digest in trusted release metadata. The runner
should reject an unexpected digest, even if a later registry writer attaches
another syntactically valid referrer. The exact release metadata location and
verification mechanism require design review before removing in-image
binaries. The current consumer prototype verifies registry content but does
not implement this stronger release binding.

### Risks and Mitigations

- **Artifact loss or early collection:** retain artifacts through a managed
  tag and verify periodic retrieval for all supported releases and mirrors.
- **Unbounded storage:** collect only after release-aware retention checks;
  measure artifact bytes and deletion backlog per repository.
- **Mirror gaps:** gate release and disconnected testing on destination
  discovery and download, including registries with tag-schema fallback.
- **Malicious or ambiguous artifact:** restrict publishing, pin expected
  digests in trusted release metadata, and reject duplicate matches.
- **Platform mismatch:** test each architecture against its exact platform
  image manifest and keep the existing executable architecture check.
- **Availability and latency:** cache by artifact digest and emit a clear
  setup error identifying repository, subject digest, and filename.

### Drawbacks

This adds registry requests to extension startup and adds artifact support
requirements to promotion, mirroring, and cleanup tooling. It also separates
test bytes from the image digest, so the release process needs an explicit
integrity binding. These costs are material even though runtime images become
smaller.

## Alternatives (Not Implemented)

- Keep gzipped extensions in runtime images. This has no new release workflow
  but retains the runtime size and distribution cost.
- Push a standalone test image. This requires a second image reference and
  lifecycle for every component and does not directly bind it to the runtime
  image digest.
- Use only a conventional tag derived from the image digest. Such a tag can
  aid retention, but it does not supply the OCI subject relationship or
  standard referrer discovery.

## Open Questions [optional]

- Where should the trusted artifact digest inventory live in the release
  payload, and how should `openshift-tests` verify it?
- Which release and mirror tools will own atomic copy, retargeting, and the
  retention inventory? They need their own implementation PRs before rollout.
- What rollback period and mirror ownership data define the cleanup grace
  period? The answer must precede any automated deletion.
- Which registries and mirror workflows can preserve the required manifest
  media type and retention tag without rewriting the gzip blob?

## Test Plan

- Unit tests for subject, type, filename, digest, duplicate, cache, and
  fallback handling, including malicious pagination and malformed manifests.
- Registry integration tests for OCI 1.1 referrers, tag-schema fallback,
  private credentials, and destination digest changes.
- CI tests that promote and mirror a multi-architecture image, then run
  `info`, `list`, and a representative test from the mirrored artifact.
- Retention tests that protect supported and rollback releases, then delete
  an expired artifact without removing a shared blob or another referrer.
- An end-to-end release test proving both older in-image and newer referrer
  extensions work during the transition.

## Graduation Criteria

The format is initially provisional. Runtime-image removal is gated on
promotion, mirror, trust-binding, and retention tests passing for every
supported destination. A release must not require manual artifact repair.

### Dev Preview -> Tech Preview

Demonstrate an end-to-end build, promotion, mirror, and retrieval on one
component and all its supported architectures. Publish operational guidance
for inspecting and repairing an absent artifact.

### Tech Preview -> GA

Run the end-to-end release test by default, demonstrate downgrade and
disconnected operation, and operate safe cleanup for expired releases. Only
then remove the in-image binary from components migrated to this format.

### Removing a deprecated feature

Retire legacy in-image extraction only after all supported payloads and
non-payload extensions have migrated or have another supported retrieval
path. The mixed-mode fallback remains until that condition is met.

## Upgrade / Downgrade Strategy

Deploy the consumer first, while publishers continue to include the gzip in
images. Add artifact publication, release promotion, mirroring, and digest
binding next. Remove the gzip from each runtime image only after the oldest
supported test runner can read its referrer or the support policy excludes
that runner. During rollback, retain both the prior image and its artifact
through the rollback window. An old runner cannot retrieve a referrer-only
binary, so removal cannot precede this compatibility gate.

## Version Skew Strategy

New runners accept both formats. Old runners require the in-image gzip.
Referrers are keyed to the tested component digest, so an extension from a
newer component cannot silently replace one from an older payload. Non-payload
extensions continue to use the existing admission and discovery controls.

## Operational Aspects of API Extensions

There are no Kubernetes API extensions. Registry availability, credentials,
referrer support, and artifact retention become dependencies of test setup,
not of cluster operation.

## Support Procedures

On extraction failure, log the image reference, resolved subject digest,
expected filename and artifact digest, registry response, and whether legacy
fallback was attempted. Operators can query the destination referrers API,
fetch the artifact by digest, and compare it with the release inventory.
Repair is to republish the validated artifact in the destination repository
and restore its retention reference, then rerun the release gate. Cleanup logs
and dry-run inventories must make accidental deletion auditable.

## Infrastructure Needed [optional]

Release promotion and disconnected mirroring jobs need OCI referrer copy and
verification support. Release metadata and retention inventory storage need
owners. Registries used for release testing need OCI 1.1 referrer support or
the tag-schema fallback specified by the
[OCI Distribution specification](https://github.com/opencontainers/distribution-spec/blob/main/spec.md#listing-referrers).
