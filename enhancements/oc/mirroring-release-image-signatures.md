---
title: mirroring-release-image-signatures
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
last-updated: 2020-04-08
status: provisional|implementable
---

# Mirroring Release Image Signatures

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

For disconnected clusters, `oc adm release mirror ...` already helps users copy the release image and its dependencies into their local mirror.
This enhancment extends that command to also create and apply ConfigMap manifests containing the release image signature which the cluster-version operator could use to [verify the mirrored release][cvo-config-map-signatures].

## Motivation

Currently a cluster upgrade can be accomplished on a cluster that does not have an active connection to the internet.
However manual steps are required to create a ConfigMap containing the signature data required for update image verification.
This enhancement will automatically create the ConfigMap, so the user doesn't have to think about manual steps.

### Goals

Users should be able to run a single `oc adm release mirror ...` command to gather the release image, images referenced from the release image, and the release image signature, so they can more easily deploy that release inside their cluster, which may have restricted network access.

### Non-Goals

This proposal does not create a framework for gathering other information from the wider internet; it only addresses release image signatures.
If, in the future, capturing additional information is desired, it will be covered by follow-up enhancements.
Explicitly excluded from this enhancement is the generic mirroring behavior proposed in [enhancement 188][enhancement-188].

## Proposal

### Applying directly to the target cluster

As of 4.2 and 4.3:

```console
$ oc adm release mirror \
>   --from=quay.io/openshift-release-dev/ocp-release:4.3.0-x86_64 \
>   --to=registry.example.com/your/repository \
>   --to-release-image=registry.example.com/your/repository:your-tag
```

will mirror the 4.3.0 release image and other images referenced by the release image into the mirror repository at registry.example.com.

This enhancement adds an `--apply-release-image-signature` option.
When set, `oc` would also apply associated release image signature ConfigMap to the cluster that `oc` connects to, with the signature application occurring after the image mirror had completed.
This enhancement would also allow `oc apply`'s `--overwrite` option to allow users to manage conflicts with resources that already existed in the target cluster.
Users can also use the existing `--dry-run` option to get an overview of the update, although for detailed audits users should use [the *pushing to disk* flow](#pushing-to-disk).

If `oc` encounters an error applying a release image signature ConfigMap, it should warn the user and eventually exit non-zero.

### Pushing to disk

As of 4.3 (with [this change][relative-to-dir-value] to support relative `--to-dir` values):

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
                ├── 4.3.0 -> sha256:3a516480dfd68e0f87f702b4d7bdd6f6a0acfdac5cd2e9767b838ceede34d70d
                ...
                ├── 4.3.0-thanos -> sha256:42ab3f59d3e769e88af89c30304b9cc77a76b66960747e2e1a6f8db097420858
                ├── sha256:00ec58112784d340179e045bc70399ac1bd509ae43c4411e934ced23d016b2a1
                ...
                └── sha256:ffa87c992c5a2c51290e6d67cb609f29893dd147c31cff639ff6785ae7a1cfe2

5 directories, 514 files
```

This filesystem can then be applied to the local registry with the given `oc image mirror` command.

With this enhancement, the output format would gain an additional directory `config` under the base directory, a sibling to the current `v2`, containing configuration manifest files that can be applied to the local cluster.
For this enhancement, the `config` directory will only create a single file, whose filename will be `signature-${ALGORITHM}-${ENCODED_PREFIX}.yaml`, including the algorithm and first sixteen characters of the encoded portion of the release image [digest][].
The manifest file can be applied to the local cluster with:

```console
$ oc apply -f mirror/config/signature-sha256-81154f5c03294534.yaml
```

or similar (with the appropriate algorithm instead of `sha256` and encoded prefix instead of `81154f5c03294534`).

### Pushing the release image signature ConfigMap manifest to disk while pushing containers to the mirror registry

Users mirroring a release before creating a cluster may want to [push both container images and the release image signature ConfigMap manifest to disk](#pushing-to-disk).
But they might also want to push container images directly to a mirror repository (as in [the *applying directly to the target cluster* case](#applying-directly-to-the-target-cluster)) while still pushing the release image signature ConfigMap to disk (because there is not yet a cluster into which that manifest can be pushed).
While you could address this use-case with [the *pushing to disk* flow](#pushing-to-disk) followed by an immediate `oc image mirror ...`, it is more convenient to have `oc adm release mirror ...` push to the registry directly.
Because setting `--to-dir` adjusts the image source, this enhancement extends the command with a new `--release-image-signature-to-dir` that can be used to set a configuration manifest output directory without setting a container image output directory.
When `--release-image-signature-to-dir` is set, it takes precedence.
When `--release-image-signature-to-dir` is unset but `--to-dir` is set, the release image signature ConfigMap manifest directory is `${TO_DIR}/config`, as described in [the previous section](#pushing-to-disk).

### User Stories

##### Mirroring to a central registry in a fully air gapped environment

A fully air gapped environment is one in which your cluster nodes cannot access the internet.
For this reason you must mirror the images and configuration manifests to a filesystem disconnected from that environment and then bring that host or removable media across that gap.
Our documentation refers to this as disconnected mirroring.
It may also be the case that there are multiple clusters within the air gapped network.
In such a case it makes sense to configure a central registry in the air gapped environment from which every cluster can pull upgrade imagery.
Assuming the aforementioned use case and the use of removable media that will be sneakernetted across the gap, the steps for mirroring are:

1. Connect the removable media to a system connected to the internet.
1. Mirror the images and configuration manifests to a directory on the removable media, using [the `--to-dir` flow](#pushing-to-disk):
    ```console
    $ oc adm release mirror "--to-dir=${REMOVABLE_MEDIA_PATH}/mirror" quay.io/openshift-release-dev/ocp-release:4.3.0-x86_64
    ```
    The enhanced `mirror` command will output the path, similar to the example below, to the release image signature ConfigMap.
    ```console
    Configmap <removable-media-path>/mirror/config/signature-sha256-81154f5c03294534.yaml created
    ```
1. Run any required checks and scrubs on the removable media.
1. Connect the removable media to a host within the air gapped environment that has access to both the central registry and to any cluster requiring upgrade.
1. Upload the mirrored images to the central registry, as described in [the `--to-dir` flow](#pushing-to-disk):
    ```console
    $ oc image mirror "--from-dir=${REMOVABLE_MEDIA_PATH}/mirror" file://openshift/release:4.3.0* REGISTRY/REPOSITORY
    ```
1. Use `oc` to log in to a given cluster to be upgraded.
1. Apply the the mirrored release image signature ConfigMap to the connected cluster, as described in [the `--to-dir` flow](#pushing-to-disk):
    ```console
    $ oc apply -f mirror/config/signature-sha256-81154f5c03294534.yaml
    ```
1. Perform the upgrade:
    ```console
    $ oc adm upgrade --allow-explicit-upgrade --to-image REGISTRY/REPOSITORY@sha256:81154f5c03294534e1eaf0319bef7a601134f891689ccede5d705ef659aa8c92
    ```

### Risks and Mitigations

One risk considered is that a user familiar with the current behavior of the `oc adm release mirror ...` command may inadvertently apply configuration manifests to their cluster.
This has been addressed by requiring the explicit `--apply-release-image-signature` argument to apply configuration manifests to a cluster.

Another risk is that we will need to gather additional information from the wider internet, but Clayton feels like we are unlikely to ever have more than a handful of objects to capture all the information we need.
This has been mitigated by using very specific options such as `--apply-release-image-signature` and `--release-image-signature-to-dir` to manage the release image signature ConfigMap.
If `oc adm release mirror ...` learns about additional types in the future, it can add support for them with additional, specific, orthogonal options without confusion due to generic option names.

## Design Details

### Test Plan

[The current e2e test][release-mirror-test] mirrors and then creates a target cluster from that mirror.
It does not restrict access from the cluster to external networks, although work for that is [in flight][blackholed-proxy].
The initial test implementation will cover:

* Creating a cluster-under-test via the usual procedure, using [the *pushing to disk* flow](#pushing-to-disk) to provide an update, and then updating the cluster-under-test.
    Once restricted-network proxy CI support is available, this test will be ported to use it, to ensure that all required content is being mirrored via the *pushing to disk* flow.
* Similarly to the above, except using [the *applying directly to the target cluster* flow](#applying-directly-to-the-target-cluster), pushing both container images and configuration manifests into the cluster-under-test before updating it.

### Graduation Criteria

This feature will move straight to GA, because `oc` doesn't seem to have a history of a `--tech-preview` flag or similar to allow for graduated stability of new features.

The feature will follow normal CLI policy for two minor (4.y) releases of backwards compatibility.

### Upgrade / Downgrade Strategy

The proposal for [applying directly to the target cluster](#applying-directly-to-the-target-cluster) adds a new dependency on the cluster to which `oc` connects for Kubernetes activity.
The explicit `--apply-release-image-signature` argument requires users who have been using the previous `oc adm release mirror` implementation (which did not push manifests) to explicitly opt in to the new functionality, so there is no chance that they accidentally push configuration manifests into the wrong cluster.
Future changes to the `mirror` flow will be gated by similar flags, so existing users will need to opt in to new functionality instead of being surprised by changes after bumping their `oc`.
Changing to previous `oc` may result in the loss of functionalty, as has been the case for other `oc` features, but the failures will be "option `<whatever>` not recognized" fast-failures, not silent behavior changes.

### Version Skew Strategy

Besides the user <-> `oc` interaction discussed in the previous section, we should also consider the `oc` <-> cluster interaction.

If the user invokes an older `oc`, it may not create the release image signature ConfigMap expected by a newer cluster.
Even for matching versions, users using [the *pushing to disk* flow](#pushing-to-disk) may decide not to apply the release image signature ConfigMap manifest to the target cluster.
Cluster components that consume the release image signature ConfigMap should be robust in their absence.
Obviously, any behavior which depends on missing objects will not happen.
For example, the cluster-version operator will not be able to retrieve signatures from release image signature ConfigMaps if those ConfigMaps do not exist.

If the user invokes a newer `oc`, it may create additional configuration manifests not supported by an older cluster.
For example, a 4.5 `oc`, implementing this enhancement, might be used to [apply the release image signature ConfigMap](#applying-directly-to-the-target-cluster) to a 4.2 cluster whose cluster-version operator does not load signatures from ConfigMaps.
Users in that situation might be surprised that the cluster was unable to locate release image signatures, not realizing that the older cluster was ignoring the unknown-to-it signature ConfigMaps.

To mitigate these issues:

* New versions of `oc` should continue to create particular configuration manifests types (e.g. release image signature ConfigMaps), even if newer clusters grow support for alternative mechanisms.
* When pushing directly to a target cluster, `oc` might query the cluster to extract version information and warn about potential incompatibilies (e.g. "this cluster's version operator is too old to understand the release image signature ConfigMap I'm pushing" or, less usefully, "I'm a lot older than this cluster, and maybe a newer `oc` would be creating more manifests than I am").
    The downsides to this approach are:
    * It may be difficult to reliably determine if cluster components support a given object or not.
    * There's no way to implement it in [the *pushing to disk* flow](#pushing-to-disk).
* New versions of `oc` could create [custom resources][custom-resources] instead of using generic types such as ConfigMaps.
    This way, configuration manifest application would generate errors when applying manifests to clusters which were too old to understand them.
    But it is too late to use this approach for release image signatures, because the ConfigMap support has [already landed][cvo-config-map-signatures].

## Implementation History

See https://github.com/openshift/oc/pull/343

## Drawbacks

Requiring users to manually create and apply mirrored configuration manifests would give them a better idea of what was getting mirrored.
With this enhancement, users may choose to blindly apply the release image signature ConfigMap without inspecting it.
That's convenient when it works, but increases the chance that users are surprised by [version skew](#version-skew-strategy) if they are impacted by it.

The narrow release image signature scoping does not extend gracefully if we need to gather multiple types of information from the wider internet.
For example `--apply-release-image-signature --apply-cincinnati-operator --apply-image-content-source-policy ...` is a lot to type.

## Alternatives

Narrowly for release signatures, we have discussed serving signatures via the Cincinnati graph JSON.
However, even in that case, users would have to gather information from the wider internet (e.g. graph snapshots from Red Hat's Cincinnati service) to feed their in-cluster Cincinnati, so I don't see a way around something like this enhancement to facilitate that capture.

Generic mirroring behavior proposed in [enhancement 188][enhancement-188], which allows for gathering as many manifests as desired, but gives less fine-grained control over direcy application compared to this enhancement's narrow `--apply-release-image-signature`.

## Infrastructure Needed

This enhancement will not require additional infrastructure beyond that already used for CI, except for eventually consuming additional resources (e.g. long-lived VPCs) needed for restricted-network proxy CI as discussed in [the test plan](#test-plan).

[blackholed-proxy]: https://github.com/openshift/release/pull/5308
[Cincinnati]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md
[cincinnati-graph-data]: https://github.com/openshift/cincinnati-graph-data
[cvo-config-map-signatures]: https://github.com/openshift/cluster-version-operator/pull/279
[digest]: https://github.com/opencontainers/image-spec/blob/v1.0.1/descriptor.md#digests
[enhancement-188]: https://github.com/openshift/enhancements/pull/188
[relative-to-dir-value]: https://github.com/openshift/oc/pull/317
[release-mirror-test]: https://github.com/openshift/release/blob/25cf85f0fc5eccddb41bf4f9b8fcfa21ff1665da/ci-operator/templates/openshift/installer/cluster-launch-installer-e2e.yaml#L413
