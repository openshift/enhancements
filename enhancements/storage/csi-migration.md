---
title: csi-migration
authors:
  - "@fbertina"
reviewers:
  - "@openshift/storage”
approvers:
  - "@openshift/openshift-architects"
creation-date: 2020-07-29
last-updated: 2021-03-05
status: provisional
see-also: https://github.com/openshift/enhancements/pull/463
replaces:
superseded-by:
---

# Migration of in-tree volume plugins to CSI drivers

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We want to allow cluster administrators to seamlessly migrate volumes created using the in-tree storage plugin to their counterparts CSI drivers. It is important to achieve this goal before CSI Migration feature becomes GA in upstream. This also a requirement for supporting [out-of-tree cloud providers](https://github.com/openshift/enhancements/pull/463)

## Motivation

[CSI migration](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/625-csi-migration) is an upstream effort to migrate in-tree volume plugins to their counterpart CSI drivers. The feature is beta since Kubernetes 1.17, however, as of Kubernetes 1.20 it is still *disabled* by default.

That is going to change in Kubernetes 1.21 (OCP 4.8), where the feature will remain beta, but *enabled* by default. In Kubernetes 1.22 (OCP 4.9) the feature may become GA.

In OCP we can optionally disable the CSI migration feature while it is still beta, however, that will no longer be an option once CSI migration becomes GA. In order to avoid surprises once the migration is enabled by default in OCP, we want to allow cluster administrators to optionally enable the feature earlier, preferably in OCP 4.8.

### Goals

Our goals are different throughout our support lifecycle.

For Tech Preview, we want to introduce a mechanism to allow switching CSI migration feature flags on and off across OCP components. It is important that this mechanism allows for a seameless migration path, without breaking existing volumes.

For GA, existing in-tree volumes will be migrated to CSI and users should not have to do any additional work. In this phase we will not support disabling CSI migration and we do want to make sure we will not break downgrades.

### Non-Goals

* Control the ordering in which OCP components will be upgraded or downgraded.
* Install or remove the CSI driver as the migration is enabled or disabled.

## Proposal

We propose to add a carry-patch to Attacth Detach Controller in OCP that enables the migration of some storage plugins. Initially we would start with Cinder and GCP, so that we are aligned with the goals of [CCMO](https://github.com/openshift/enhancements/pull/463).

### Implementation Details/Notes/Constraints

Before getting to our proposal, we need to describe some of the upstream requirements for using CSI Migration.

The CSI migration feature is hidden behind feature gates in Kubernetes. For instance, to enable the migration of a in-tree AWS EBS volume to its counterpart CSI driver, the cluster administrator should turn on these two feature gates: *CSIMigration* and *CSIMigrationAWS*. However, these flags must be enabled or disabled in a [specific order](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-migration.md#upgradedowngrade-migrateunmigrate-scenarios).

It is important to respect this ordering to avoid an undesired state of volumes. For instance, if the feature is enabled in the Kubelet before it is enabled in the Attach Detach controller, volumes attached to nodes by the in-tree volume plugin cannot be detached by the CSI driver and will stay attached forever.
In the same vein, if the feature is disabled in the Attach Detach controller before it is disabled in the Kubelet, volumes attached by the CSI driver cannot be detached by the in-tree volume plugin and will stay attached forever.

In summary, this is what upstream recommends:

* When the CSI migration is **enabled**, events should happen in this order:
  1. Enable the feature gate in all control-plane components.
  2. Once that's done, drain nodes one-by-one and start the Kubelet with the feature gate enabled.

* When the CSI migration is **disabled**, events should happen in this order:
  1. One-by-one, drain the nodes and start the Kubelet with the feature gate disabled.
  2. Once that's done, disable the feature gate in all control-plane components.

In order to keep the Attach Detach Controller and the Kubelet in sync regarding using the CSI driver or the in-tree plugin, upstream has a mechanism to keep the Attach Detach Controller informed about the status of the migration on nodes. Roughly speaking, Kubelet propagates to an annotation the information for each migrated in-tree plugin on the node.

As a result, the Attach Detach Controller knows if the in-tree plugin has been migrated on the node. If the feature flags are enabled in Kube Controller Manager **and** on the node, the Attach Detach Controller uses the CSI driver to attach volumes. Otherwise, it will falls back to the in-tree plugin.

In OCP, we can easily set those feature gates by using the [FeatureGate] (https://docs.openshift.com/container-platform/4.7/nodes/clusters/nodes-cluster-enabling-features.html) Custom Resource. OCP operators read this resource and restart their operands with the appropriate features enabled.
However, this approach alone is not acceptable for CSI migration because the feature flags might be switched across components in _any_ arbitrary order.

That being said, we plan to submit an upstream patch that allows the Attach Detach Controller to have its own custom feature gates, independent from Kube Controller Manager.
In addition to that, we propose to add a carry-patch to Attach Controller in OCP that enables CSI Migration of some storage plugins. Initially we would start with Cinder and GCP, so that we are aligned with the goals of [CCMO](https://github.com/openshift/enhancements/pull/463).

That way, when deciding about using either the CSI driver or the in-tree plugin, the AttachDetch Controller will **only** rely on the information propagated by the node.
Other controllers from Kube Controller Manager, like the PV Controller, will still obey the flags passed to the Kube Controller Manager. In other words, Attach Detach Controller will start considering which plugin to use (in-tree or CSI) on a node basis only.

#### Benefits

The biggest benefit of this approach is that we do not need to worry about the ordering in which components are restarted with the CSI migration flags switched on or off. Migration flags can be switched on or off across components in any order.

Another benefit is that this approach should not break downgrades once CSI Migration becomes GA. In OCP, a downgrade is fundamentally a regular upgrade to an older version.
That means that CVO downgrades components in the same order as it upgrades them: connntrol-plane first, nodes later. This would impose an issue if the user downgraded from a version with CSI Migration enabled to a version with CSI Migration disabled. With the above patch in the downgraded version, that would not be a problem.

It is important to notice that, with this carry-patch in OCP, Attach Detach Controller will _not_ change its current behaviour as long as nodes are not migrated to CSI, which is the default behaviour in OCP 4.8.

#### Graduation

During Tech Preview in OCP 4.8, users can enable the CSI migration using a *FeatureSet*. It could be shared with [CCMO](https://github.com/openshift/enhancements/pull/463), but we may want to have a specific *FeatureSet* only for CSI Migration for users that want to enable CSI Migration without migrating to an external cloud provider.

Once CSI migration reaches GA in upstream, the associated feature gates will be enabled by default. As a result, it will not be necessary to use *FeatureSets* anymore.

### Risks and Mitigations

A carry-patch means that OCP will be the only Kubernetes distribution exercising this code path, which can lead us to bugs that were not seen anywhere. However, we are confident that the patch is small and self-contained enough to be used.

## Design Details

### Test Plan

#### E2E jobs

We want E2E jobs for all migrated plugins ready at **Tech Preview** time.

For each E2E job:

1. Install an OCP cluster.
1. Enable the `CSIMigration` _FeatureSet_.
1. Run E2E tests for in-tree volume plugins. This should use the CSI driver instead.
1. Disable the `FeatureSet`.
1. Once again, run E2E tests for in-tree volume plugins.

In addition to that, as a stretch goal, we want a separate job that:

1. Runs a `StatefulSet`.
1. Enables the migration _FeatureSets_.
1. Wait for all components to have the right feature flags.
1. Checks if the StatefulSet survives.

Once CSI migration is GA, we expect the regular upgrade jobs will cover upgrades from an OCP version with migration disabled to a version with migration enabled.

### Graduation Criteria

This is what needs to be done across different support phases:

1. Tech Preview in OCP 4.8:

* Introduce a new *FeatureSet* in openshift/api called `CSIMigration`.
* Make sure the *FeatureSet* used by [CCMO](https://github.com/openshift/enhancements/pull/463) contains the CSI migration feature flags enabled for the respective storage backened.
* Introduce an upstream patch that allows Attach Detach Controller to have its own custom feature gates, independent from Kube Controller Manager.
* Introduce a carry-patch in OCP that enables CSI Migration for Cinder and GCP PD in Attach Detach Controller.
* A PoC of both upstream and OCP patches [are available](https://github.com/openshift/kubernetes/pull/601).

2. GA in OCP 4.9

Nothing needs to be done, migration flags are enabled by default and cannot be disabled.

3. Post-GA in OCP 4.10

* Remove `CSIMigration` *FeatureSet* from openshift/api.

## Implementation History

Main events only, this is not a faithful history.

2020-07-29: Initial enhancement draft.
2021-01-28: Re-worked proposal to use 2 _FeatureSets_ with manual application by the user.
2021-03-05: Re-worked proposal to use carry-patch instead. Moved previous approach to "Alternatives" for reference.

## Alternatives

We have considered this alternative approach. It is NOT our preferable approach because it introduces many shortcomings:

* We put on the user the responsability of applying the *FeatureSets* in the correct order, which is very error-prone.
* We need to create 2 extra *FeatureSets* as opposed to 1.
* We need to patch MCO so that it ignores one of these *FeatureSets*.
* Once GA, downgrades to a previous version with CSI Migration disabled by default would break because OCP downgrades the control-plane before nodes.

### Tech Preview

1. Create two new FeatureSets: `CSIMigrationNode` and `CSIMigrationControlPlane`. Both *FeatureSets* will enable all CSI Migration feature flags.
   * The machine-config-operator (MCO) will **ignore** the `CSIMigrationControlPlane` **FeatureSet**.
   * On the other hand, all operators will react to the `CSIMigrationNode` *FeatureSet*, including control-plane operators.
2. To enable CSI Migration for any in-tree plugin, the cluster administrator should:
   * First, enable the CSI migration feature flags in all control-plane components by adding the `CSIMigrationControlPlane` *FeatureSet* to the `featuregates/cluster` object.
     - With the exception of MCO, all operators will recognize this *FeatureSet* and will initialize their operands with the associated feature gates.
   * Second, once all control-plane components have restarted, enable the CSI migration feature flags in the Kubelet:
     - Add the `CSIMigrationNode` *FeatureSet* to the `featuresgates/cluster` object, replacing the previous value (i.e., `CSIMigrationControlPlane`).
     - All operators will recognize the `CSIMigrationNode` *FeatureSet*, however, control-plane operators already applied the associated feature gates in the step above, so in practice only MCO will have work to do.
   * At this point, CSI migration is fully enabled in the cluster.
3. To disable CSI migration, the cluster administrator should perform the same steps in the opposite order:
   * In `featuregates/cluster` object, replace the `CSIMigrationNode` *FeatureSet* by `CSIMigrationControlPlane`.
   * Wait for all `CSINode` objects to have the annotation `storage.alpha.kubernetes.io/migrated-plugins` cleared. No storage plugins should be listed in this annotation.
   * Remove the `CSIMigrationControlPlane` *FeatureSet* from the `featuregates/cluster` object.
4. It is the **responsibility of the cluster administrator** to guarante the ordering described above is respected.

### GA

Once CSI migration reaches GA in upstream, the associated feature gates will be enabled by default and the features will not be optional anymore. As a result, CSI migration will be enabled by default in OCP as well, and there will not be an option to disable it.

In addition that, the *FeatureSets* created to handle the Tech Preview feature will no longer be operational because the feature flags they enable will already be enabled in the cluster.

It is important to note that the order in which operators are downgraded in OCP violates [upstream version skew policy](https://kubernetes.io/docs/setup/release/version-skew-policy).
The policy states that a new Kubelet must never run with an older API server or Controller Manager. A direct consequence of this violation is the need to introduce a workaround to downgrade a cluster with CSI migration enabled.

### Post-GA

CSI migration *FeatureSets* can be removed from OCP API **one** release after CSI migration becomes GA.
