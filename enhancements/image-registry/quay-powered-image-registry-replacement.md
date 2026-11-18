---
title: quay-powered-image-registry-replacement
authors:
  - jbpratt
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-08-13
last-updated: 2026-08-13
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/PROJQUAY-12512
see-also:
  - https://redhat.atlassian.net/browse/PROJQUAY-9734
  - https://redhat.atlassian.net/browse/OCPSTRAT-2775
  - https://redhat.atlassian.net/browse/IR-499
replaces:
  - TBD
superseded-by:
  - TBD
---

# Quay Powered Image Registry

## Summary

Replace the deprecated `openshift/image-registry` with a new in-cluster
registry built on Quay (backed by distribution/distribution v3). The new
registry integrates with the ImageStream API, OAuth, and existing RBAC systems
to provide a drop-in replacement which is deployed through the existing
`openshift/cluster-image-registry-operator`.

## Motivation

The OpenShift Image Registry has been deprecated since OCP 4.19 and disabled by
default. The formal decision doc (linked in IR-499) lays out why.

However, many existing and newer capabilities depend on an in-cluster
registry being available. Image Mode needs a registry for the layered RHCOS
images. Builds push output to ImageStreamTags. `oc` and the Console assume a
registry is available.

Quay's new in-cluster registry, built on distribution, will share code between
the new mirror-registry, the downstream Quay product, as well as Quay.io.

### User Stories

- As a developer, I want to `podman push` my newly built images without
  configuring external registry credentials
- As a developer, I want my pods to authenticate with their service account
  tokens and push output to `ImageStreamTags`
- As a cluster administrator, I want to be able to enforce namespace-scoped
  access control to limit what images users can push and pull
- As a cluster administrator, I want the registry to be operational after
  install without manual configuration
- As a cluster administrator, I want to be able to upgrade from existing
  image-registry deployments and be able to roll back if needed.

### Goals

- OCI Distribution Spec v1.1 conformant
- Drop-in replacement via the existing operator
- Full ImageStream integration
- Auth via TokenReview and SubjectAccessReview
- Feature gate opt-in and rollback

### Non-Goals

- Replacing the bootstrap/IRI registry used during cluster installation
- Providing a multi-tenant hosted registry service (that remains Quay.io)
- Deprecating the ImageStream API, it will be used as is
- Shipping Quay-specific features (a web UI, robot accounts, etc.). The scope
  matches what openshift/image-registry ships today.

## Proposal

The Quay team will ship a new registry implementation built on the CNCF
distribution/distribution project which will serve as a drop-in replacement
inside the existing operator framework. A feature gate
(`QuayIntegratedRegistry`) will signal to the operator to deploy the new image
which will look and act the same as the previous image. The storage format is
identical, so upgrades and rollbacks are instant with no migration.

### Workflow Description

**Developer** is a human user who pushes and pulls images via podman, oc, or
other client tooling

**Cluster administrator** is a human who enables the registry capability,
configures storage, and manages image lifecycle

**cluster-image-registry-operator** is the operator that reconciles
`configs.imageregistry.operator.openshift.io` and deploys the registry pods

#### Push

1. Developer (or pod) authenticates to the registry which will extract the
   token and validate it via a `TokenReview`
2. The registry checks authorization via a `SubjectAccessReview` in the target
   namespace
3. Blobs are uploaded via standard OCI chunked upload flow, writing directly to
   storage
4. Manifests are then written to storage and an `Image` CR is created, tagging
   the image into the target `ImageStream`, creating it if it does not exist
5. If quota is exceeded (`LimitRange` and `ResourceQuota`) for blob size or
   image/tag count, then the push is rejected with a 403

#### Pull

1. Developer (or pod) authenticates and is authorized via a
   `SubjectAccessReview`, requiring the `get` verb on `imagestreams/layers`
2. The registry resolves the tag or digest by walking the `ImageStream` status
   for the matching `Image` CR
3. If the image is local, the registry serves the manifest and blobs from
   storage
4. If the image is a remote reference, the registry pulls from the source
   registry and caches it locally

#### Tag/Catalog Listing

1. Developer lists tags, the registry enumerates the `ImageStream.status.tags`
   with the user's identity and returns a list
2. Developer lists the catalog, the registry enumerates all `ImageStream` CRs
   across all namespaces

#### Image Pruning

1. Cluster administrator runs `oc adm prune images` or the ImagePruner
   `CronJob` fires
2. `oc` filters against the K8s API to determine unreferenced `Image` CRs and
   deletes them
3. `oc` calls the DELETE blobs endpoint on the registry for each orphaned blob,
   deleting them from storage

#### Enablement

1. Cluster administrator enables the `QuayIntegratedRegistry` feature gate
2. cluster-image-registry-operator detects the gate and switches the
   container image deployed from the legacy image to the new image, rolling out
   the deployment
3. If the pod fails readiness, the administrator disables the gate to roll
   back. No data migration is needed, Distribution's storage layout is used

### API Extensions

No new CRDs or modifications to existing schemas are needed. We will consume
existing APIs:

- `image.openshift.io/v1` - Image, ImageStream, ImageStreamMapping,
  ImageStreamLayers
- `authentication.k8s.io/v1` - TokenReview
- `authorization.k8s.io/v1` - SubjectAccessReview

The existing `configs.imageregistry.operator.openshift.io` CRD is used as-is to
configure the registry. No new fields are required at Tech Preview.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The registry runs on the data plane, so no special Hypershift work should be
needed.

#### Standalone Clusters

Primary target topology, all features described are expected to work

#### Single-node Deployments or MicroShift

Single-node should continue functioning as it does today

#### OpenShift Kubernetes Engine

No OKE-specific changes are required

### Implementation Details/Notes/Constraints

#### Feature Gate

The feature gate `QuayIntegratedRegistry` is added to the
`TechPreviewNoUpgrade` feature set in `openshift/api`. When the gate is
enabled, the `cluster-image-registry-operator` switches the registry deployment
to use the Quay container image instead of the legacy image.

#### Operator Integration

The operator reads the `IMAGE` environment variable and uses that as the
container image deployed. Configuration is handled through the `REGISTRY_*` env
vars that distribution v3 handles natively and custom parsing will be introduced
for the OpenShift-specific options. The operator handles mounting the storage
root, the TLS cert, and the SA token.

#### ImageStream Integration

`ImageStreams` continue to be the backing of every registry operation:

- Push: Build an `Image` CR with the full manifest and map it to an
  `ImageStream`
- Pull: Resolve the tag and digest via the `ImageStream` resource
- Delete: Determine if the digest is still referenced in the `ImageStream`
- List: Walk the `ImageStreams` across all namespaces via the K8s API

#### Authentication

Standard Docker V2 challenge-response: a client hits the `/v2/` endpoint and
receives a 401 to authenticate against the token endpoint. The client sends the
token, it is validated against a `TokenReview` then a `SubjectAccessReview`.
Existing `image-puller` and `image-builder` roles should continue to work.

#### Storage

Filesystem and cloud backends are supported (S3 with AWS SDK v2, Azure, GCS,
Swift). Upstream distribution drivers are used where possible

### Risks and Mitigations

Image Registry internals are largely undocumented, any existing integration
tests will be used to validate behavior.

Feature gate rollback puts data at risk, this is mitigated by the same storage
format being used on disk

### Drawbacks

This adds a second registry implementation to the release payload for the Tech
Preview period. Once the feature gate graduates to GA, the legacy image can be
safely removed

## Alternatives (Not Implemented)

- Keep the legacy registry: this increases complexity for the maintainers from
  CVE fixes to feature delivery with the hope of a shared Quay core behind the
  various product registries
- Use an external registry: while feasible, this introduces barriers and goes
  against the "batteries included" offering that is OpenShift

## Open Questions [optional]

1. Is it possible to implement an in-process garbage collection mechanism to
   replace the existing image pruning process? Is there history behind the
   existing mechanism?
2. Are there any existing performance issues reported with the legacy
   image-registry implementation that should be taken into account?

## Test Plan

- Passing OCI conformance suite
- Passing existing `image-registry` integration suites
- Upgrade and downgrade testing in CI
- Scale testing of 2000+ nodes

## Graduation Criteria

### Tech Preview

- Feature gate `QuayIntegratedRegistry` in `TechPreviewNoUpgrade`
- OCI conformance tests passing
- Push/pull/delete with OpenShift OAuth and SA token auth
- ImageStream integration
- Pull-through proxy support
- Adherence to mirror policies
- Image signatures storage (OCI 1.1 Referrers)
- Quota enforcement
- Multiple storage drivers provided (AWS, GCP, Azure, Filesystem, etc.)
- Upgrade/downgrade tests stable

### GA

- Documentation updated
- Scale validation at 2000+ nodes
- ...

### Removing a deprecated feature

Once the feature gate graduates, the legacy image-registry can be removed from
the payload along with the feature gate

## Upgrade / Downgrade Strategy

Upgrade (legacy to new): Enabling the feature gate will trigger a redeploy of
the new Quay image which is compatible with the legacy image. No data migration
is required.

Downgrade (new to legacy): Disabling the feature gate will trigger a revert of
the image, back to the legacy image. Images pushed while it was enabled will
remain as is.

## Version Skew Strategy

All APIs used are stable so there are no version skew concerns

## Operational Aspects of API Extensions

No new API extensions are being introduced, we will consume existing APIs.

## Support Procedures

- On start failure: checking operator conditions for common issues like PVC
  not bound or mount failures
- On push failure: analyzing the pod logs for auth or write errors
- On pull failure: validating the `ImageStream` exists with the requested tag
  and checking the pod logs for storage read errors
- On rollback: Disable the feature gate and the operator redeploys the legacy
  image

## Infrastructure Needed [optional]

No new infrastructure should be needed. The existing operator machinery and
namespace will be reused
