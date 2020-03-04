---
title: mirroring-manifsts
authors:
  - "@wking"
  - "@jottofar"
reviewers:
  - "@jottofar"
  - "@LalatenduMohanty"
  - "@smarterclayton"
  - "@deads2k"
approvers:
  - "@LalatenduMohanty"
  - "@smarterclayton"
  - "@deads2k"
creation-date: 2020-01-24
last-updated: 2020-03-11
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

For disconnected clusters, `oc adm release mirror ...` already helps users copy the release image and its dependencies into their local mirror. As of 4.2 and 4.3, the following command:

```console
$ oc adm release mirror \
--from=quay.io/openshift-release-dev/ocp-release:4.3.0-x86_64 \
--to=registry.example.com/your/repository \
--to-release-image=registry.example.com/your/repository:your-tag \
```

will mirror the 4.3.0 release image and other images referenced by the release image into the mirror repository at registry.example.com.

This enhancment extends that command to also handle manifests, information that cannot be represented by container images, by either pushing them to disk or by applying them to a connected cluster.
The manifest application would begin after the image mirror had completed. For example, `oc` might produce ConfigMap manifests containing the release image signature which the cluster-version operator could use to [verify the mirrored release][cvo-config-map-signatures].

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

### Pushing manifests to disk for later application

As of 4.3, the following command:

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

This filesystem can then be applied to the local registry with the given `oc image mirror` command.

With this enhancement, the output format would gain an additional directory `manifests` under the base directory, a sibling to the current `v2`, containing manifest files that can be applied to the local cluster with:

```console
$ oc apply -Rf mirror/manifests
```

The `manifests` directory may or may not contain additional subdirectories, so the recursive `-R` should be given for future-proofing.

Users may also want to push container images directly to a mirror repository (as in the example command in [the *Summary*](#summary) while still pushing manifests to disk (because there is not yet a cluster into which those manifests can be pushed).
While you could address this use-case with the command above followed by an immediate `oc image mirror ...`, it is more convenient to have `oc adm release mirror ...` push to the registry directly.
Because setting `--to-dir` adjusts the image source, this enhancement extends the command with a new `--manifests-to-dir` that can be used to set a manifest output directory without setting a container image output directory.
When `--manifests-to-dir` is set, it takes precedence.
When `--manifests-to-dir` is unset but `--to-dir` is set, the manifest directory is `${TO_DIR}/manifests`, as described above.

### Applying manifests directly to the target cluster

This enhancement adds a new `--apply-manifests` option that when specified will apply manifests directly to the connected cluster rather than outputting them to disk.
When the `--apply-manifests` option is specified a user can also optionally specify `--overwrite` which would cause the apply to behave as the `oc apply --overwrite` does currently.
If the manifest exists on the cluster it will be updated.
If the manifest exists on the cluster and `--overwrite` is not specified a warning will be displayed that the manifest could not be created.
If `oc` encountered an error applying a manifest, it could optionally attempt to apply additional manifests or simply exit non-zero immediately.

### User Stories

#### Release-signature ConfigMaps

Currently a cluster upgrade can be accomplished on a cluster that does not have an active connection to the internet.
However manual steps are required to create a ConfigMap containing the signature data required for update image verification.
This enhancement will automatically create the ConfigMap from the image manifest files.

#### In-cluster Cincinnati

Clusters running within a disconnected network will run Cincinnati on premise to provide an upgrade experience much more like that found on a cluster running within a connected network.
The `oc adm release mirror ...` command is expected to be used to ease the installation of an on premise Cincinnati by mirroring the Cincinnati images.
With this enhancement any required Cincinnati manifests will also be mirrored.

#### Mirroring to a central registry in a fully air gapped environment

A fully air gapped environment is one in which your cluster nodes cannot access the internet.
For this reason you must mirror the images and manifests to a filesystem disconnected from that environment and then bring that host or removable media across that gap.
Our documnetation refers to this as disconnected mirroring.
It may also be the case that there are multiple clusters within the air gapped network.
In such a case it makes sense to configure a central registry in the air gapped environment from which every cluster can pull upgrade imagery.
Assuming the aforementioned use case and the use of removable media that will be sneakernetted across the gap, the steps for mirroring are:

1. Connect the removable media to a system connected to the internet.
2. Mirror the images and manifests to a directory on the removable media:
	```console
	$ oc adm release mirror --to-dir=<removable-media-path>/mirror quay.io/openshift-release-dev/ocp-release:4.3.0-x86_64
	```
3. The enhanced `oc adm release mirror ...` command will output the path, similar to the example below, to a configmap created containing the image signature.
	```console
	Configmap <removable-media-path>/mirror/manifests/signature-sha256-81154f5c03294534e1eaf0319bef7a601134f891689ccede5d705ef659aa8c92 created
	```
4. Run any required checks/scrubs on the removable media.
5. Connect the removable media to a host within the air gapped environment that has access to both the central registry and to any cluster requiring upgrade.
6. Upload the mirrored images to the central registry:
	```console
	$ oc image mirror --from-dir=<removable-media-path>/mirror file://openshift/release:4.3.0* REGISTRY/REPOSITORY
	```
7. Use oc to login to a given cluster to be upgraded.
8. Apply the the mirrored configmap, noted above, to the connected cluster for use by CVO for upgrade image verification:
	```console
	$ oc apply -Rf <removable-media-path>/mirror/manifests
	```
9. Perform the upgrade:
	```console
	$ oc adm upgrade --to-image REGISTRY/REPOSITORY/release@sha256:81154f5c03294534e1eaf0319bef7a601134f891689ccede5d705ef659aa8c92
	```

### Risks and Mitigations

One risk considered is that a user familiar with the current behavior of the `oc adm release mirror ...` command may inadvertently apply manifests to their cluster.
This has been addressed by requiring the explicit `--apply-manifests` argument to apply manifests to a cluster.

## Design Details

### Test Plan

The current e2e test, that makes sure the command always exits successfully and that certain apsects of the content are always present, will be modified to validate the new functionality as well.

### Graduation Criteria

### Upgrade / Downgrade Strategy

The image is included in the payload, but has no content running in a cluster to upgrade.

### Version Skew Strategy

The proposal for [applying manifests directly to the target cluster](#applying-manifests-directly-to-the-target-cluster) adds a new dependency on the cluster to which `oc` connects for Kubernetes activity.
The explicit `--apply-manifests` argument requires users who have been using the previous `oc adm release mirror` implementation (which did not push manifests) to explicitly opt in to the new functionality, so there is no chance that they accidentally push manifests into the wrong cluster.

## Implementation History

See https://github.com/openshift/oc/pull/343

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

[cvo-config-map-signatures]: https://github.com/openshift/cluster-version-operator/pull/279
[custom-resources]: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/
