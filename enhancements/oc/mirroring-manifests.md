---
title: mirroring-manifsts
authors:
  - "@wking"
reviewers:
  - "@jottofar"
  - "@LalatenduMohanty"
  - "@smarterclayton"
approvers:
  - "@LalatenduMohanty"
  - "@smarterclayton"
creation-date: 2020-01-24
last-updated: 2020-01-24
status: provisional|implementable
---

# Mirroring Manifests

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

For disconnected clusters, `oc adm release mirror ...` already helps users copy the release image and its dependencies into their local mirror.
This enhancment extends that command to also create and apply manifests to their local cluster, to bring in information that cannot be represented by container images.
For example, `oc` might produce ConfigMap manifests containing the release image signature which the cluster-version operator could use to [verify the mirrored release][cvo-config-map-signatures].

## Motivation

There is a lot of information in the wider internet that may be useful for restricted-network clusters.
Some of that information can be represented by container images, such as the release image and the operator and other images referenced by the release image.
Some of that information cannot be represented by container images, including container signatures.
By extending `oc`'s release-mirror support to create and apply Kubernetes manifests, we provide a channel that can be used to gather arbitrary information in one place that the user can audit, transport into their restricted network, and apply to their local cluster.
As the need for additional information arises, additional manifests may be created, possibly using new [custom resources][custom-resources] known to the local cluster, without users needing to adjust their mirroring workflows.

### Goals

Users should be able to run a single `oc adm release mirror ...` command to gather all the external information needed to deploy that release inside their cluster, which may have restricted network access.

### Non-Goals

This proposal does not specify the types of manifests that are created.
Users who intend to audit the gathered manifests should extract the gathered types from the manifests themselves.

## Proposal

### Applying directly to the target cluster

As of 4.2 and 4.3:

```console
$ oc adm release mirror \
>   --from=quay.io/openshift-release-dev/ocp-release:4.3.0-x86_64 \
>   --to=registry.example.com/your/repository \
>   --to-release-image=registry.example.com/your/repository:your-tag \
>   --apply-manifests
```

will mirror the 4.3.0 release image and other images referenced by the release image into the mirror repository at registry.example.com.

This enhancement would also apply associated manifests to the cluster that `oc` connects to, with the manifest application beginning after the image mirror had completed.
This enhancement would also allow `oc apply`'s `--force-conflicts` and `--overwrite` options to allow users to manage conflicts with resources that already existed in the target cluster.
If `oc` encountered an error applying a manifest, it could optionally attempt to apply additional manifests before exiting non-zero, but would not be required to attempt any additional manifests.

### Pushing to disk

As of 4.3:

```console
$ oc adm release mirror --to-dir=mirror quay.io/openshift-release-dev/ocp-release:4.3.0-x86_64
...
Success
Update image:  openshift/release:4.3.0

To upload local images to a registry, run:

    oc image mirror --from-dir=mirror file://openshift/release:4.3.0* REGISTRY/REPOSITORY
```

creates an output filesystem like:

```console
$ tree mirror
mirror
└── v2
    └── openshift
        └── release
            ├── blobs
            │   ├── sha256:005337dc1f6870934cf5efaf443786149e6a71041a91566ce1e16e78bce511a9
            ...
            │   └── sha256:ffc132fefac522ec38b1b22ecd67a5173565efb3e8929e47ef7e0b4ee7920adf
            └── manifests
                ├── 4.3.0 -> mirror/v2/openshift/release/manifests/sha256:3a516480dfd68e0f87f702b4d7bdd6f6a0acfdac5cd2e9767b838ceede34d70d
                ...
                ├── 4.3.0-thanos -> mirror/v2/openshift/release/manifests/sha256:42ab3f59d3e769e88af89c30304b9cc77a76b66960747e2e1a6f8db097420858
                ├── sha256:00ec58112784d340179e045bc70399ac1bd509ae43c4411e934ced23d016b2a1
                ...
                └── sha256:ffa87c992c5a2c51290e6d67cb609f29893dd147c31cff639ff6785ae7a1cfe2

5 directories, 514 files
```

that can be applied to the local registry with the given `oc image mirror` command.

With this enhancement, the output format would gain an additional directory `manifests` as a sibling to the current `v2` containing manifest files that can be applied to the local cluster with:

```console
$ oc apply -Rf mirror/manifests
```

The `manifests` directory may or may not contain additional subdirectories, so the recursive `-R` should be given for future-proofing.

### Pushing to disk while pushing containers to the mirror registry

Users mirroring a release before creating a cluster may want to [push both container images and manifests to disk](#pushing-to-disk).
But they might also want to push container images directly to a mirror repository (as in [the *applying directly to the target cluster* case](#applying-directly-to-the-target-cluster) while still pushing manifests to disk (because there is not yet a cluster into which those manifests can be pushed).
While you could address this use-case with [the *pushing to disk* flow](#pushing-to-disk) followed by an immediate `oc image mirror ...`, it is more convenient to have `oc adm release mirror ...` push to the registry directly.
Because setting `--to-dir` adjusts the image source, this enhancement extends the command with a new `--manifests-to-dir` that can be used to set a manifest output directory without setting a container image output directory.
When `--manifests-to-dir` is set, it takes precedence.
When `--manifests-to-dir` is unset but `--to-dir` is set, the manifest directory is `${TO_DIR}/manifests`, as described in [the previous section](#pushing-to-disk).

### User Stories

#### Release-signature ConfigMaps

FIXME

#### In-cluster Cincinnati

FIXME

### Risks and Mitigations

FIXME

## Design Details

### Test Plan

FIXME

### Graduation Criteria

FIXME

### Upgrade / Downgrade Strategy

FIXME

### Version Skew Strategy

The proposal for [applying directly to the target cluster](#applying-directly-to-the-target-cluster) adds a new dependency on the cluster to which `oc` connects for Kubernetes activity.
The explicit `--apply-manifests` argument requires users who have been using the previous `oc adm release mirror` implementation (which did not push manifests) to explicitly opt in to the new functionality, so there is no change that they accidentally pushing manifests into the wrong cluster.
FIXME: check to see how this works if there are conflicts but neither `--force-conflicts` not `--overwrite` was set.

## Implementation History

FIXME

## Drawbacks

FIXME

## Alternatives

FIXME

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.

[cvo-config-map-signatures]: https://github.com/openshift/cluster-version-operator/pull/279
[custom-resource]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
