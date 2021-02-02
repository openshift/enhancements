---
title: Migration of in-tree volume plugins to CSI drivers
authors:
  - "@fbertina"
reviewers:
  - "@openshift/storage ”
approvers:
  - "@openshift/openshift-architects"
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

We want to allow cluster administrators to seamlessly migrate in-tree volumes to their counterparts CSI drivers. It is important to achieve this goal before CSI Migration feature becomes GA in upstream.

## Motivation

CSI migration is going to be enabled by default as a beta feature in Kubernetes 1.21 (OCP 4.8). In Kubenertes 1.22 (OCP 4.9) the feature may become GA.

In OCP we can optionally disable beta features, however, that will no longer be an option once CSI migration becomes GA.

In order to avoid surprises once the migration is enabled by default in OCP, we want to allow cluster administrators to optionally enable the feature earlier, preferably in OCP 4.8.

## Goals

For Tech Preview, we want to introduce a mechanism to allow switching CSI migration feature flags on and off accross OCP components.

Due to upstream requirements, it is important that this mechanism allows for enabling the feature flags on control-plane components **before** the kubelet, and vice-versa.

Once CSI migration is enabled by default in upstream, it will not be possible to disable it again. Therefore, such mechanism shall be disabled in OCP once CSI migration becomes GA in upstream.

## Non-Goals

* Control the ordering in which OCP components will be upgraded or downgraded. We will leave this responsability to the user.
* Install or remove the CSI driver as the migration is enabled or disabled.

## Proposal

The CSI migration feature is hidden behind feature gates in Kubernetes. For instance, to enable the migration of a in-tree AWS EBS volume to its counterpart CSI driver, the cluster administrator should turn on these two feature gates:

* CSIMigration
* CSIMigrationAWS

Nevertheless, what makes things more complicated is the strict order in which those flags need to be switched across components. In other words, CSI Migration requires that feature flags are enabled or disabled in a [specific order](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-migration.md#upgradedowngrade-migrateunmigrate-scenarios).

It is important to respect this ordering to avoid an undesired state of the components of volumes. For instance, when enabling the feature, volumes attached to nodes by the in-tree volume plugin cannot be detached by the CSI driver and will stay attached forever.

In the same vein, when disabling the feature, volumes attached by the CSI driver cannot be detached by the in-tree volume plugin and will stay attached forever.

In summary:

* When the CSI migration is **enabled**, events should happen in this order:
  1. Enable the feature gate in all control-plane components.
  2. Once that's done, drain nodes one-by-one and start the kubelet with the feature gate enabled.

* When the CSI migration is **disabled**, events should happen in this order:
  1. One-by-one, drain the nodes and start the kubelet with the feature gate disabled.
  2. Once that's done, disable the feature gate in all control-plane components.

In order to achieve that, we propose two different approaches to be used at different times during the feature lifecycle.

The first approach is intended to be used for is Tech Preview in OCP. The other approach should be used to once we graduate CSI migration to GA.

### Tech Preview

In OCP, *FeatureSets* can be used to aggregate one or more feature gates. Then, the resource [FeatureGate](https://github.com/openshift/api/blob/dca637550e8c80dc2fa5ff6653b43a3b5c6c810c/config/v1/types_feature.go#L9-L21) can be used to enable and disable *FeatureSets*.

With that in mind, we propose to:

1. Create two [new FeatureSets](https://github.com/openshift/api/blob/master/config/v1/types_feature.go#L25-L43) to support CSI migration: `CSIMigrationNode` and `CSIMigrationControlPlane`.
   * Both *FeatureSets* will enable the **same** feature gates:
     - `CSIMigration`
     - `CSIMIgrationAWS`
     - `CSIMigrationGCE`
     - `CSIMigrationAzureDisk`
     - `CSIMigrationAzureFile`
     - `CSIMigrationvSphere`
     - `CSIMigrationOpenStack`
   * The machine-config-operator (MCO) will **ignore** the `CSIMigrationControlPlane` **FeatureSet**.
   * On the other hand, all operators will react to the `CSIMigrationNode` *FeatureSet*, including control-plane operators.
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
4. It is the **responsability of the cluster administrator** to guarante the ordering described above is respected.

### GA

Once CSI migration reaches GA in upstream, the associated feature gates will be enabled by default and the features will not be optional anymore. As a result, CSI migration will be enabled by default in OCP as wel, and there will not be an option to disable it.

In addition that, the *FeatureSets* created to handle the Tech Preview feature will no longer be operational because the feature flags they enable will already be enabled in the cluster.

As for the required ordering described above, the [upgrade order](https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/upgrades.md#generalized-ordering) performed by CVO during a cluster upgrade will take care of applying the feature gates in the correct order.

#### Limitations

The limitations of this approach lies on the downgrade process. In OCP, a downgrade is fundamentally a regular upgrade to an older version. That means that CVO downgrades components in the same order as it upgrades them: control-plane first, nodes later.

However, this ordering is **not** desired when **disabling** CSI migration, which is what is going to happen when the previous OCP version had CSI migration disabled.

It is important to note that the order in which operators are downgraded in OCP violates [upstream version skew policy](https://kubernetes.io/docs/setup/release/version-skew-policy).
The policy states that a new kubelet must never run with an older API server or Controller Manager. A direct consequence of this violation is the need to introduce a workaround to downgrade a cluster with CSI migration enabled.

That being said, to address this issue we propose to document a simple workaround:

1. Enable the `CSIMigrationNode` *FeatureSet*.
1. Downgrade.

As stated above, the CSI migration *FeatureSets* are not operational once CSI migration becomes GA. However, they will be carried over during the downgrade and they will be correctly applied once the system is downgraded.

### Post-GA

CSI migration *FeatureSets* can be removed from OCP API **one** release after CSI migration becomes GA.

### Risks and Mitigations

Although this three-phased approach does what we need, it has some drawbacks:

1. Having operators ignoring certain *FeatureSets* is not usual and is error-prone. Fortunately we only need to introduce the skipping in MCO.

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

Other operators, like the Kubernetes API Server Operator, may have their openshift/api library bumped. However, that is not strictly necessary as their operands don't need to enable the CSI migration feature flags.

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
1. Enable the feature gate. Don't worry about the order in which components will apply the feature flags because at this point there are no volumes created.
1. Wait until MCO and control-plane operators restart.
1. Run E2E tests for in-tree volume plugins.
1. Disable the feature gate. Again, don't worry about the ordering.
1. Run E2E tests again.

In addition to that, as a strech goal, we want a separate job that:

1. Runs a StatefulSet.
1. Enables the migration *FeatureSets* in the right order.
1. Wait for all components to have the right feature flags.
1. Checks if the StatefulSet survives.

Once CSI migration is GA, we expect the regular upgrade jobs will cover upgrades from an OCP version with migration disabled to a version with migration enabled.
