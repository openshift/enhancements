---
title: csi-migration
authors:
  - "@fbertina"
reviewers:
  - "@openshift/storage”
approvers:
  - "@openshift/openshift-architects"
creation-date: 2020-07-29
last-updated: 2022-03-17
status: implementable
see-also: https://github.com/openshift/enhancements/pull/463
replaces:
superseded-by:
---

# Migration of in-tree volume plugins to CSI drivers

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We want to allow cluster administrators to seamlessly migrate volumes created using the in-tree storage plugin to their counterpart CSI drivers. It is important to achieve this goal before CSI Migration feature becomes GA in upstream. This also a requirement for supporting [out-of-tree cloud providers](https://github.com/openshift/enhancements/pull/463)

## Motivation

[CSI migration](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/625-csi-migration) is an upstream effort to migrate in-tree volume plugins to their counterpart CSI drivers. The feature was designed so that different storage plugins can independently graduate it according to their level of stability.

The feature is beta, but *disabled* by default, for most storage plugins since Kubernetes 1.17. However, that has changed in Kubernetes 1.21 (OCP 4.8), where the feature remained beta for OpenStack Cinder, but *enabled* by default. In Kubernetes 1.24 (OCP 4.11) the feature becomes GA.

In OCP we can optionally disable the CSI migration feature while it is still beta, however, that is no longer an option once CSI migration becomes GA. In order to avoid surprises, we want to allow cluster administrators to optionally enable the feature before it turns GA.

### Goals

Our goals are different throughout our support lifecycle in OCP.

For Tech Preview, we want to introduce a mechanism to allow switching CSI migration feature flags on and off across OCP components. It is important that this mechanism allows for a seamless migration path, without breaking existing volumes. In this phase, the default *StorageClass* will be the one with the in-tree provisioner.

For GA, existing in-tree volumes will be migrated to use the CSI driver. Once the migration has been enabled, users should not have to do any additional work. In this phase we will not support disabling CSI migration, and we should ensure that downgrades are not broken. In addition to that, the default *StorageClass* (SC) will preferably by the CSI one.

### Non-Goals

* Control the ordering in which OCP components will be upgraded or downgraded.
* Install or remove the CSI driver as the migration is enabled or disabled.

## Proposal

It is important to note that the order in which operators are downgraded in OCP violates [upstream version skew policy](https://kubernetes.io/docs/setup/release/version-skew-policy).
The policy states that a new Kubelet must never run with an older API server or Controller Manager. A direct consequence of this violation is the need to introduce a workaround to downgrade a cluster with CSI migration enabled.

That being said, we propose to add a carry-patch to the Attach Detach Controller in OCP that enables the migration of some storage plugins. In addition to that, we propose to utilize the existing **TechPreviewNoUpgrade** *FeatureSet* to switch the feature on and off during the Tech Preview phase.

Once CSI migration is GA, we will remove the carry-patch from OCP.

### Tech-preview

Before getting to our proposal, we need to describe some of the upstream requirements for using CSI Migration.

The CSI migration feature is hidden behind feature gates in Kubernetes. For instance, to enable the migration of an in-tree AWS EBS volume to its counterpart CSI driver, the cluster administrator should turn on these two feature gates: *CSIMigration* and *CSIMigrationAWS*. However, these flags must be enabled or disabled in a [specific order](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-migration.md#upgradedowngrade-migrateunmigrate-scenarios).

It is important to respect this ordering to avoid an undesired state of volumes. For instance, if the feature is enabled in the Kubelet before it is enabled in the Attach Detach Controller, volumes attached to nodes by the in-tree volume plugin cannot be detached by the CSI driver and will stay attached forever.
In the same vein, if the feature is disabled in the Attach Detach Controller before it is disabled in the Kubelet, volumes attached by the CSI driver cannot be detached by the in-tree volume plugin and will stay attached forever.

In summary, this is what upstream recommends:

* When the CSI migration is **enabled**, events should happen in this order:
  1. Enable the feature gate in all control-plane components.
  2. Once that's done, drain nodes one-by-one and start the Kubelet with the feature gate enabled.

* When the CSI migration is **disabled**, events should happen in this order:
  1. One-by-one, drain the nodes and start the Kubelet with the feature gate disabled.
  2. Once that's done, disable the feature gate in all control-plane components.

In order to keep the Attach Detach Controller and the Kubelet in sync regarding using the CSI driver or the in-tree plugin, upstream has a mechanism to keep the Attach Detach Controller informed about the status of the migration on nodes. Roughly speaking, Kubelet tracks this status via an annotation with the information for each migrated in-tree plugin on the node.

As a result, the Attach Detach Controller knows if the in-tree plugin has been migrated on the node. If the feature flags are enabled in Kubernetes Controller Manager **and** on the node, the Attach Detach Controller uses the CSI driver to attach volumes. Otherwise, it will revert to the in-tree plugin.

#### Upstream and OCP patches

In OCP, we can easily set those feature gates by using the [FeatureGate] (https://docs.openshift.com/container-platform/4.7/nodes/clusters/nodes-cluster-enabling-features.html) Custom Resource. OCP operators read this resource and restart their operands with the appropriate features enabled.
However, this approach alone is not acceptable for CSI migration because the feature flags might be switched across components in _any_ arbitrary order. Our solution is intended to make the Attach Detach Controller resilient to this issue.

That being said, **we propose to carry a patch in OCP's Attach Controller to force-enable the CSI Migration feature gates there**.

This carry-patch would only affect the Attach Detach Controller. Other parts of Kubernetes Controller Manager, such as the PersistentVolume Controller, would still obey the flags passed to the Kubernetes Controller Manager.
In order to do this the least invasive way possible, we will refactor the upstream code to allow the carry-patch to be minimal and self-contained. An existing PoC [is available](https://github.com/openshift/kubernetes/pull/601) to demonstrate how this would look like.

This patch would be carried over in OCP until all in-tree storage plugins are migrated to CSI, which should happen around 3 or 4 releases.
Initially we would start with Cinder and EBS (_CSIMigrationAWS_ and _CSIMigrationCinder_ flags), so that we are aligned with the goals of both upstream and [CCMO](https://github.com/openshift/enhancements/pull/463).
In subsequent OCP releases we would proceed with GCP, vSphere and Azure (_CSIMigrationGCE_, _CSIMigrationvSphere_ and _CSIMigrationAzureFile_), without a predefined order.

With this patch, when deciding about using either the CSI driver or the in-tree plugin, the Attach Detach Controller will **only** rely on the information propagated by the node. In other words, Attach Detach Controller will start considering which plugin to use (in-tree or CSI) on a node basis only.

Note: one might wonder why upstream does not implement this behavior already. The answer is that they do not have the same issues OCP has in regards to disabling feature flags in the wrong order across components.

#### Benefits

The biggest benefit of this approach is that we do not need to worry about the ordering in which components are restarted with the CSI migration flags switched on or off. Migration flags can be switched on or off across components in any order.

Another benefit is that this approach should not break downgrades once CSI Migration becomes GA. In OCP, a downgrade is fundamentally a regular upgrade to an older version.
That means that CVO downgrades components in the same order as it upgrades them: control-plane first, nodes later. This would impose an issue if the user downgraded from a version with CSI Migration enabled to a version with CSI Migration disabled. With the above patch in the downgraded version, that would not be a problem.

It is important to notice that, with this carry-patch in OCP, Attach Detach Controller will _not_ change its current behavior as long as nodes are not migrated to CSI, which is the default behavior in OCP 4.8.

#### Graduation

During the Tech Preview phase, users can enable and disable the CSI migration feature using the **TechPreviewNoUpgrade** *FeatureSet*. Among other things, this *FeatureSet* switches the complete [Cloud Migration](https://github.com/openshift/enhancements/pull/463) in OCP 4.8. Here is a configuration example:

```yaml
apiVersion: config.openshift.io/v1
kind: FeatureGate
metadata:
  name: cluster
spec:
  featureSet: TechPreviewNoUpgrade
```

Optionally, users can enable the CSI Migration *without* enabling the Cloud Migration by using the **CustomNoUpgrade** *FeatureSet*. Here is a configuration example to enable the migration for AWS EBS:

```yaml
apiVersion: config.openshift.io/v1
kind: FeatureGate
metadata:
  name: cluster
spec:
  featureSet: CustomNoUpgrade
  customNoUpgrade:
    enabled:
    - CSIMigrationAWS
```


#### Risks and Mitigations

A carry-patch means that OCP will be the only Kubernetes distribution exercising this code path, which can lead us to bugs that were not seen anywhere. However, we are confident that the patch is small and self-contained enough to be used.

### GA

Once CSI migration of a given storage plugin reaches GA in upstream, the associated feature gates will be enabled by default in OCP. As a result, we will remove the carry-patch we introduced in OCP 4.8. Also, users will not have to use the **TechPreviewNoUpgrade** *FeatureSet* anymore.

In addition to that, the CSI *StorageClass* (SC) will be set as default on new clusters, but we will not change the default SC on existing clusters. The main reason for that is that users might have set a different quota on the CSI driver SC. An extra benefit of this approach is that we will not override a default SC that was set by the user.

In order to achieve that, these are the main changes introduced in OCP 4.11 with regards to SCs:

* The *defaultstorageclasscontroller* from CSO will be completely removed and we will not create any new in-tree SCs.
  * SCs that were previously created by this controller will not be owned by any operator and may very well be deleted by the user.
* A new special-purpose controller will be added to the CSI Driver Operator.
  * This controller will make sure that a CSI SC always exists.
  * And it will set that SC as the default one when appropriate.
  * Also, it will make sure there is only **one** default SC.

In a more detailed way, this is how the new controller will operate and determine whether the CSI SC should be the default one:

* On every sync, this controller fetches all existing SCs.
* If there are **no** pre-existing **default** SCs in the cluster, then it creates a CSI SC **and** set it as the default.
  * Otherwise, create the CSI SC but do not make it default.
* If there are pre-existing SCs in the cluster and more than one is set as default, it will set an appropriate message in the CR asking the user to make amendments.

### Post-GA

CSI migration *FeatureSets* can be removed from OCP API **one** release after CSI migration becomes GA.

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

Each storage plugin will be migrated at their own paces and will have different support phases across OCP releases. For instance, EBS and Cinder migration would be Tech Preview and OCP 4.8 and GA in 4.9. GCP PD will most likely be Tech Preview in 4.9 and GA in 4.10.

Having said that, the items below only cover the initial targets for CSI Migration: AWS EBS and Cinder.

1. Tech Preview in 4.8:

* Introduce a new *FeatureSet* in openshift/api called `CSIMigration`.
* Make sure the *FeatureSet* used by [CCMO](https://github.com/openshift/enhancements/pull/463) contains the CSI migration feature flags enabled for the respective storage backend.
* Introduce an upstream patch to refactor the Attach Detach Controller in order to make our carry-patch smaller and self-contained.
* Introduce a carry-patch in OCP that enables CSI Migration for Cinder and EBS in Attach Detach Controller.

2. GA in 4.9:

* Update carry-patch to not force-enable CSI Migration for Cinder and EBS. Since the migration is enabled by default, there is no need to force-enable the migration.

Assuming the next storage plugin to be migrated is GCP PD, the steps below represent what needs to be done for that plugin:

1. Tech Preview in 4.9:

* Update the carry-patch in Attach Detach Controller to force-enable the CSI Migration for GCP PD.

2. GA in 4.11:

* Update the carry-patch in Attach Detach Controller to **not** force-enable the CSI Migration for GCP PD.
* Update CSO to stop syncing in-tree SC.

Once CSI Migration is GA for all storage plugins have been completely migrated to CSI, we can:

* Remove the carry-patch from OCP.
* Remove the `CSIMigration` _FeatureSet_ from openshift/api.


## Implementation History

Main events only, this is not a faithful history.

2020-07-29: Initial enhancement draft.
2021-01-28: Re-worked proposal to use 2 _FeatureSets_ with manual application by the user.
2021-03-05: Re-worked proposal to use carry-patch instead. Moved previous approach to "Alternatives" for reference.
2021-03-31: Swapped GCE migration for EBS: GCP will be Tech Preview in 4.9, EBS in 3.8.
2022-03-16: Added section for specifying default SC.
