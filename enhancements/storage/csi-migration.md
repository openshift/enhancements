---
title: csi-migration
authors:
  - "@fbertina"
reviewers:
  - "@openshift/storage ”
approvers:
  - "@openshift/openshift-architects"
  - "@darkmuggle"
creation-date: 2020-07-01
last-updated: 2021-01-28
status: provisional
see-also:
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

We want to allow cluster administrators to seamlessly migrate volumes created using
the in-tree storage plugin to their counterparts CSI drivers. It is important to
achieve this goal before CSI Migration feature becomes GA in upstream.

## Motivation

[CSI migration](https://github.com/kubernetes/enhancements/tree/master/keps/sig-storage/625-csi-migration)
is an upstream effort to migrate in-tree volume plugins to their counterpart CSI
drivers. The feature is beta since Kubernetes 1.17, however, as of Kubernetes 1.20
it is still *disabled* by default.

That is going to change in Kubernetes 1.21 (OCP 4.8), where the feature will remain
beta, but *enabled* by default. In Kubernetes 1.22 (OCP 4.9) the feature may become GA.

In OCP we can optionally disable the CSI migration feature while it is still beta,
however, that will no longer be an option once CSI migration becomes GA.

In order to avoid surprises once the migration is enabled by default in OCP, we want
to allow cluster administrators to optionally enable the feature earlier, preferably
in OCP 4.8.

### Goals

Our goals are different throughout our support lifecycle.

For Tech Preview, we want to introduce a mechanism to allow switching CSI migration
feature flags on and off across OCP components. Due to upstream requirements
described in (the design proposal)[https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-migration.md#upgradedowngrade-migrateun-migrate],
it is important that this mechanism allows for switching the feature
flags in the the correct order.

In other words, when enabling CSI migration, control-plane components should have
their feature flags enabled before the kubelet. The opposite order applies when
disabling CSI migration.

For GA, we will not support disabling CSI migration. Existing in-tree volumes will
be migrated to CSI and users should not have to do any additional work. We do want
to make sure we will not break downgrades should the user decide to do that.

### Non-Goals

* Control the ordering in which OCP components will be upgraded or downgraded.
* Install or remove the CSI driver as the migration is enabled or disabled.

## Proposal

### Requirements

The CSI migration feature is hidden behind feature gates in Kubernetes. For instance,
to enable the migration of a in-tree AWS EBS volume to its counterpart CSI driver,
the cluster administrator should turn on these two feature gates: *CSIMigration*,
and *CSIMigrationAWS*.

In OCP, we can easily set those feature gates by using the [FeatureGate]
(https://docs.openshift.com/container-platform/4.7/nodes/clusters/nodes-cluster-enabling-features.html)
Custom Resource. OCP operators read this resource and restart their operands with
the appropriate features enabled. However, this approach alone is not acceptable
for CSI migration because the feature flags might be switched across components
in an arbitrary order.

This is not acceptable because CSI Migration requires that feature flags are enabled
or disabled in a [specific order](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-migration.md#upgradedowngrade-migrateunmigrate-scenarios).

It is important to respect this ordering to avoid an undesired state of volumes.
For instance, if the feature is enabled in the kubelet before it is enabled in the
AttachDetach controller, volumes attached to nodes by the in-tree volume plugin
cannot be detached by the CSI driver and will stay attached forever.

In the same vein, if the feature is disabled in the AttachDetach controller before
it is disabled in the kubelet, volumes attached by the CSI driver cannot be detached
by the in-tree volume plugin and will stay attached forever.

In summary, this is what upstream recommends:

* When the CSI migration is **enabled**, events should happen in this order:
  1. Enable the feature gate in all control-plane components.
  2. Once that's done, drain nodes one-by-one and start the kubelet with the
  feature gate enabled.

* When the CSI migration is **disabled**, events should happen in this order:
  1. One-by-one, drain the nodes and start the kubelet with the feature gate disabled.
  2. Once that's done, disable the feature gate in all control-plane components.

In addition to that, upstream has a mechanism to keep the AttachDetach Controller
informed about the status of the migration on the nodes. Roughly speaking,
Kubelet propagates to an annotation the information for each potentially migrated
in-tree plugin on the node.

As a result, the AttachDetach Controller knows if the in-tree plugin has been
migrated on the Node. If the feature flags are enabled in KCM and on the Node,
the AttachDetach Controller uses the CSI driver to attach volumes. Otherwise,
it will falls back to the in-tree plugin.

### OCP

That being said, we propose to add a carry-patch to OCP that allows the AttachDetach
Controller to ignore the CSI Migration feature flags passed to Kube Controller Manager.
That way, when deciding about using either the CSI driver or the in-tree plugin,
the AttachDetch Controller will **only** rely on the information propagated by the Node.

If the feature flags are disabled on the Node, the AttachDetach Controller will never use
the CSI driver to attach or detach volumes, which makes this a safe approach.

The biggest benefit of this approach is that we do not need to worry about the ordering
in which components are restarted with the CSI migration flags switched on or off.

Another benefit is that this approach should not break downgrades once CSI Migration
becomes GA. In OCP, a downgrade is fundamentally a regular upgrade to an older version.
That means that CVO downgrades components in the same order as it upgrades them: control-plane
first, nodes later. This would impose an issue if the user downgraded from a version with CSI
Migration enabled to a version with CSI Migration disabled. With the above patch in the downgraded
version, that would not be a problem.

Once this patch is merged, during Tech Preview users can enable the CSI migration using a *FeatureSet*.
It could be shared with [CCMO](https://github.com/openshift/enhancements/pull/463), but we may want
to have a specific *FeatureSet* only for CSI Migration.

Once CSI migration reaches GA in upstream, the associated feature gates will be enabled by default.
As a result, it will not be necessary to use *FeatureSets* anymore.

## Alternatives

We have considered this alternative approach. It is NOT our preferable approach because
it introduces many shortcomings:

* It is error prone.
* We put on the user the responsability of applying the *FeatureSets* in the correct order.
* We need to create 2 extra *FeatureSets* as opposed to 1.
* We need to patch MCO so that it ignores one of these *FeatureSets*.

### Tech Preview

1. Create two new FeatureSets: `CSIMigrationNode` and `CSIMigrationControlPlane`.
* Both *FeatureSets* will enable the **same** feature gates:
- `CSIMigration`
- `CSIMIgrationAWS`
- `CSIMigrationGCE`
- `CSIMigrationAzureDisk`
- `CSIMigrationAzureFile`
- `CSIMigrationvSphere`
- `CSIMigrationOpenStack`
* The machine-config-operator (MCO) will **ignore** the `CSIMigrationControlPlane` **FeatureSet**.
* On the other hand, all operators will react to the `CSIMigrationNode` *FeatureSet*,
  including control-plane operators.
2. To enable CSI Migration for any in-tree plugin, the cluster administrator should:
* First, enable the CSI migration feature flags in all control-plane components:
- Add the `CSIMigrationControlPlane` *FeatureSet* to the `featuregates/cluster` object:
       ```shell
       $ oc edit featuregates/cluster
       (...)
       spec:
         featureSet: CSIMigrationControlPlane
       ```
       - With the exception of MCO, all operators will recognize this *FeatureSet* and will initialize their operands with the associated feature gates.
	   * Second, once all control-plane components have restarted, enable the CSI migration feature flags in the kubelet:
       - Add the `CSIMigrationNode` *FeatureSet* to the `featuresgates/cluster` object, replacing the previous value (i.e., `CSIMigrationControlPlane`):
       ```shell
       $ oc edit featuregates/cluster
       (...)
       spec:
         featureSet: CSIMigrationNode
       ```
       - All operators will recognize the `CSIMigrationNode` *FeatureSet*, however, control-plane operators already applied the associated feature gates in the step above, so in practice only MCO will have work to do.
	   * At this point, CSI migration is fully enabled in the cluster.
	   3. To disable CSI migration, the cluster administrator should perform the same steps in the opposite order:
	   * In `featuregates/cluster` object, replace the `CSIMigrationNode` *FeatureSet* by `CSIMigrationControlPlane`.
	   * Wait for all `CSINode` objects to have the annotation `storage.alpha.kubernetes.io/migrated-plugins` cleared. No storage plugins should be listed in this annotation.
	   * Remove the `CSIMigrationControlPlane` *FeatureSet* from the `featuregates/cluster` object.
	   4. It is the **responsibility of the cluster administrator** to guarante the ordering described above is respected.

### GA

Once CSI migration reaches GA in upstream, the associated feature gates will be
enabled by default and the features will not be optional anymore. As a result,
CSI migration will be enabled by default in OCP as well, and there will not be
an option to disable it.

In addition that, the *FeatureSets* created to handle the Tech Preview feature
will no longer be operational because the feature flags they enable will already
be enabled in the cluster.

As for the required ordering described above, the [upgrade order]
(https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/upgrades.md#generalized-ordering)
performed by CVO during a cluster upgrade will take care of starting
components in the desired order.

In this phase, CSI migration feature gates are enabled by default in all
components, so restarting control-plane components before the kubelet
is enough to guarantee a smooth feature enablement.

#### Limitations

The limitations of this approach lies on the downgrade process. In OCP, a downgrade
is fundamentally a regular upgrade to an older version. That means that CVO downgrades
components in the same order as it upgrades them: control-plane first, nodes later.

However, this ordering is **not** desired when **disabling** CSI migration, which is
what is going to happen when the previous OCP version had CSI migration disabled.

It is important to note that the order in which operators are downgraded in OCP violates
[upstream version skew policy](https://kubernetes.io/docs/setup/release/version-skew-policy).
The policy states that a new kubelet must never run with an older API server or Controller Manager.
A direct consequence of this violation is the need to introduce a workaround to downgrade a
cluster with CSI migration enabled.

That being said, to address this issue we propose to document a simple workaround:

1. Enable the `CSIMigrationNode` *FeatureSet*.
1. Downgrade.

As stated above, the CSI migration *FeatureSets* are not operational once CSI migration
becomes GA. However, they will be carried over during the downgrade and they will be
correctly applied once the system is downgraded.

### Post-GA

CSI migration *FeatureSets* can be removed from OCP API **one** release after CSI
migration becomes GA.

### Risks and Mitigations

Although this three-phased approach does what we need, it has some drawbacks:

1. For Tech Preview, users might enable or disable *FeatureSets* in the wrong
order, causing issues with attaching or detaching existing volumes. We plan to
address this through documentation.

## Design Details

This is what needs to be done across different support phases:

1. Tech Preview:

* openshift/api
* Introduce two new *FeatureSets*: `CSIMigrationNode` and `CSIMigrationControlPlane`
* Machine Config Operator
* Bump openshift/api in order to get the *FeatureSets* above
* Introduce a patch to ignore the `CSIMigrationControlPlane` *FeatureSet*
* Kubernetes Scheduler Operator
* Bump openshift/api in order to get the *FeatureSets* above
* Kubernetes Controller Manager Operator
* Bump openshift/api in order to get the *FeatureSets* above

Other operators, like the Kubernetes API Server Operator, may have their
openshift/api library bumped. However, that is not strictly necessary as
their operands don't need to enable the CSI migration feature flags.

2. GA

Nothing needs to be done.

3. GA + 1 release

* openshift/api
* Remove CSI migration *FeatureSets*: `CSIMigrationNode` and `CSIMigrationControlPlane`
* Machine Config Operator
* Bump openshift/api
* Remove the skip added for Tech Preview
* Other operators
* Bump openshift/api

### Test Plan

#### E2E jobs

We want E2E jobs for all migrated plugins ready at **Tech Preview** time.

For each E2E job:

1. Install an OCP cluster.
1. Enable the feature gate in the right order. Even though it is a fresh cluster,
   we need to respect the order because [volumes might be created in CI]
   (https://github.com/openshift/release/blob/master/ci-operator/step-registry/ipi/install/monitoringpvc/ipi-install-monitoringpvc-ref.yaml).
1. Run E2E tests for in-tree volume plugins.
1. Disable the feature gate in the right order.
1. Once again, run E2E tests for in-tree volume plugins.

In addition to that, as a stretch goal, we want a separate job that:

1. Runs a StatefulSet.
1. Enables the migration *FeatureSets* in the right order.
1. Wait for all components to have the right feature flags.
1. Checks if the StatefulSet survives.

Once CSI migration is GA, we expect the regular upgrade jobs will cover upgrades
from an OCP version with migration disabled to a version with migration enabled.
