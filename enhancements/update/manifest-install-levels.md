---
title: manifest-install-levels
authors:
  - "@wking"
reviewers:
  - "@LalatenduMohanty"
approvers:
  - TBD
creation-date: 2020-09-12
last-updated: 2020-12-15
status: implementable
---

# Manifest Install Levels

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

[Several operators][extra-manifests] pass manifests [like these][extra-manifests-example] to the cluster-version operator which are nice-to-have, [create-only][] defaults, but not critical to bootstrap completion.
That's fine, and makes life in the post-install clusters easier.
However, when a user feeds the installer their own alternative content, [cluster-bootstrap][] will [race][] the cluster-version operator to push the content into the cluster.
Sometimes cluster-bootstrap wins, and the user-provided manifest ends up in-cluster.
But sometimes the cluster-version operator wins, and the user-provided content is forgotten when the create-only default ends up in-cluster.
This enhancement adds the ability to delay non-critical manifests to later in the installation, ensuring the cluster-version operator will always lose the race and cluster-bootstrap will push the user-provided manifest into the cluster.

## Motivation

### Goals

* Respect manifests that users feed the installer, even if they target a resource that is also backed by a release image, create-only manifest.

### Non-Goals

* Completely ordering installation.
    The cluster-version operator used to order manifests during installation, but we [dropped that][parallel-install] for quicker installs.
    This enhancement should restore enough install-time ordering to avoid the races, but not so much that it significantly delays installs.

## Terminology

There are several types of manifests in play during installation:

* Release image manifests, which second-level operators feed into release images and which are managed in-cluster by the cluster-version operator.
    These may be inspected with `oc adm release extract --to manifests "${RELEASE_IMAGE_PULLSPEC}"`.

    * Create-only release image manifests, which have [the `release.openshift.io/create-only` annotation set to `true`][create-only].
        This enhancement is about the timing of these manifests.
    * Manifests which are not create-only.
        This enhancement has nothing to do with these manifests.

* Cluster-bootstrap manifests, which are fed into the cluster via Ignition configuration and applied by [cluster-bootstrap][].
    This enhancement proposes no changes to the handling of these manifests.

## Proposal

This enhancement defines a new manifest annotation, `release.openshift.io/install-level`, which allows manifests to be ordered by run-level during installation.
The default value is `0`, and currently the only other allowed value is `1`.

The cluster-version operator has [three reconciliation phases][manifest-graph]:

* Install phase, which lasts until all release image manifests have been reconciled.
    This enhancement is about the timing of these manifests.
* Update phase, which occurs after installation if the desired release image has not yet been fully reconciled.
    This enhancement has nothing to do with this phase.
* Reconcile phase, which occurs when neither the installation nor the update phase applies.
    This enhancement has nothing to do with this phase.

The cluster-version operator will only consider the new annotation during the install phase, and will ignore it for all other phases.

To guard against errors in release image manifests, the install-phase cluster-version operator will add the following manifests guards:

* If the new `release.openshift.io/install-level` annotation is present:

    * [the create-only annotation][create-only] must also be present.
    * The value of the `release.openshift.io/install-level` annotation must be `0` or `1`.

If a manifest fails any install-phase, valid-manifest guards, the cluster-version operator will complain about the invalid value in a `Failing=True` ClusterVersion condition and not apply any release image manifests.
That ensures that installs will fail with an understandable explanation, making it very unlikely that accidentally mistyped values make it through CI and into released images.

During the install phase, the cluster-version operator will ensure all manifests in phase *n* are reconciled before attempting to apply any manifests from phase *n*+1.
Within a phase, manifests will still be applied in parallel without blocking other manifests within that same phase.

Some component manifests are required to satisfy bootstrap completion, which [today is][bootstrap-complete]:

* Production pods for:
    * `openshift-cluster-version/cluster-version-operator`
    * `openshift-kube-apiserver/kube-apiserver`
    * `openshift-kube-controller-manager/kube-controller-manager`
    * `openshift-kube-scheduler/openshift-kube-scheduler`
* A bootstraped etcd cluster.

Manifests which are required for bootstrap completion must remain in the default level `0`, ideally by leaving the new annotation unset.

At least one manifest which is fairly slow, ideally always landing after bootstrap removal, must remain in the default level `0`, ideally by leaving the new annotation unset.
For example, the monitoring ClusterOperator always takes a while, especially when it is scheduled to compute nodes.
FIXME: but when we allow monitoring to be scheduled on the control plane, is it _always_ after bootstrap removal?  On the other hand, all we really need to beat is cluster-bootstrap's manifest-pushing phase, which completes well before its pod-waiting phase.

Manifests which are not required for bootstrap completion, but which are `create-only` defaults for user-facing configuration must be placed in level `1` to avoid racing cluster-bootstrap.

Other manifests should leave the new annotation unset.

### User Stories

#### Custom Network Configuration

Per [rhbz#1832759][rhbz-1832759], CRC sets [a custom MTU][crc-custom-MTU] via [a `create-manifests` flow][crc-create-manifests].
This currently races [an empty, default Network manifest][extra-manifests-network].
Once that network manifests grows an `release.openshift.io/install-level: 1` annotation, the user-provided manifest will always be pushed into the cluster.

### Implementation Details/Notes/Constraints

Currently during installation, the cluster-version operator treats manifests as "reconciled" after _attempting_ to push them into the cluster, regardless of whether that push succeeds.
With this enhancement, it will continue that behavior within an install level, but will then need to wait on successful reconciliation before entering a subsequent install level.
For consistency, and in order to avoid failed reconiliation after claiming a successful install, it should also wait on successful reconciliation for manifests in the final install level before claiming install completion.

### Risks and Mitigations

FIXME
Something about folks typoing values.
Something about folks accidentally picking the wrong level.
Something about the integer values and (not) painting ourselves into a corner.

There is a low risk that related manifests maintained in separate repositories would need to be shifted between different install levels in an atomic pivot.
To unblock that case in CI, either:

* The failing CI jobs may be temporarily ignored with `/override ...` to land one of the locking PRs.
    This is simple, but would break CI across the organization until the remaining pull requests landed to complete the pivot.
* The related manifests could be copied to a single repository unchanged, removed from their original repositories, pivoted while in the single repository, and then optionally moved back to their original repositories.

## Design Details

### Test Plan

FIXME

### Graduation Criteria

This will be released directly to GA.

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

The new annotation only affects the install phase, so it has no impact on upgrades or downgrades.

### Version Skew Strategy

The new annotation only affects the install phase, and the set of installation manifests and the cluster-version operator which applies them are extracted from the same release image, so there is no version skew exposure for this enhancement post release.

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Splitting installation into levels may slow installation if heavy objects are moved to later levels.
But since the primary focus is moving manifests that are create-only default configuration, level `1` should apply very quickly and not add much time.

## Alternatives

An alternative to splitting install manifests into levels would be to teach the cluster-version operator to detect bootstrap completion and use that to unblock the create-only, default manifests.
However, the mechanism for declaring bootstrap completion [has changed before][bootstrap-complete-config-map], and using a mechanism that is purely internal to the cluster-version operator allows the installer to continue to pivot that implementation without worrying about breaking other consumers.

[bootstrap-complete]: https://github.com/openshift/installer/blob/0a4cc6c428c9d1aaf11d7f05002ab7637cdb872f/data/data/bootstrap/files/usr/local/bin/bootkube.sh.template#L342-L355
[bootstrap-complete-config-map]: https://github.com/openshift/installer/pull/1645
[cluster-bootstrap]: https://github.com/openshift/cluster-bootstrap/
[crc-create-manifests]: https://github.com/code-ready/snc/blob/a7f77519a24715129981209d2f06a2927d738db3/snc.sh#L353-L365
[crc-custom-mtu]: https://github.com/code-ready/snc/blob/a7f77519a24715129981209d2f06a2927d738db3/cluster-network-03-config.yaml#L9
[create-only]: https://github.com/openshift/cluster-version-operator/blob/6d56c655ea16f6faee4b65ffef43dcd912657bc6/docs/dev/operators.md#what-if-i-only-want-the-cvo-to-create-my-resource-but-never-update-it
[extra-manifests-example]: https://github.com/openshift/cluster-config-operator/pull/34
[extra-manifests-network]: https://github.com/openshift/cluster-config-operator/pull/36
[extra-manifests]: https://bugzilla.redhat.com/show_bug.cgi?id=1832759#c29
[manifest-graph]: https://github.com/openshift/cluster-version-operator/blob/6d56c655ea16f6faee4b65ffef43dcd912657bc6/docs/user/reconciliation.md#manifest-graph
[parallel-install]: https://github.com/openshift/cluster-version-operator/pull/136
[race]: https://bugzilla.redhat.com/show_bug.cgi?id=1832759#c28
[rhbz-1832759]: https://bugzilla.redhat.com/show_bug.cgi?id=1832759
