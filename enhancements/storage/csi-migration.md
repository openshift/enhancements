---
title: Migration of in-tree volume plugins to CSI drivers
authors:
  - "@fbertina"
reviewers:
  - "@openshift/storage ”
approvers:
  - "@openshift/openshift-architects"
creation-date: 2020-07-01
last-updated: 2020-07-01
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

CSI migration is going to tentatively be enabled by default in Kubernetes 1.22 (OCP 4.9). As a result, volumes from in-tree plugins will be migrated to their counterpart CSI driver by default.

In order to avoid surprises once the migration is enabled by default, we want to allow cluster administrators to optionally enable the feature earlier, preferably in OCP 4.8.

## Goals

For Tech Preview, we want to introduce a mechanism to switch certain feature gates on and off accross OCP components.

Once CSI migration is enabled by default in upstream, it will not be possible to disable it again. Therefore, given mechanism shall be disabled in OCP once CSI migration becomes GA in upstream.

## Non-Goals

* Control the ordering in which OCP components will be upgraded or downgraded.
* Install or remove the CSI driver as the migration is enabled or disabled.

## Proposal

The CSI migration feature is hidden behind feature gates in Kubernetes. For instance, to enable the migration of a in-tree AWS EBS volume to its counterpart CSI driver, the cluster administrator should enable these two feature gates:

* CSIMigration
* CSIMigrationAWS

CSI Migration requires that feature flags are enabled and disabled in a [specific order](https://github.com/kubernetes/community/blob/master/contributors/design-proposals/storage/csi-migration.md#upgradedowngrade-migrateunmigrate-scenarios). In summary:

* When the CSI migration is **enabled**, events should happen in this order:
  1. Enable the feature gate in all control-plane components.
  2. Once that's done, drain nodes one-by-one and start the kubelet with the feature gate enabled.

* When the CSI migration is **disabled**, events should happen in this order:
  1. One-by-one, drain the nodes and start the kubelet with the feature gate disabled.
  2. Once that's done, disable the feature gate in all control-plane components.

In order to enable and disable these feature gates in OCP, we propose two different approaches to be used at different times during the feature lifecycle.

The first approach is intended to be used once CSI migration is Tech Preview in OCP. The other approach should be used to once we graduate CSI migration to GA.

### Tech Preview

In OCP, *FeatureSets* can be used to aggregate one or more feature gates. Then, the resource [FeatureGate](https://github.com/openshift/api/blob/dca637550e8c80dc2fa5ff6653b43a3b5c6c810c/config/v1/types_feature.go#L9-L21) can be used to enable and disable *FeatureSets*.

With that in mind, we propose to:

1. Create two [new FeatureSets](https://github.com/openshift/api/blob/master/config/v1/types_feature.go#L25-L43) to support CSI migration: `CSIMigrationNode` and `CSIMigrationControlPlane`.
  1.1 Both *FeatureSets* contain the same feature gates: `CSIMigration`, `CSIMIgrationAWS`, `CSIMigrationGCE`, `CSIMigrationAzureDisk`, `CSIMigrationAzureFile`, `CSIMigrationvSphere`, `CSIMigrationOpenStack`.
  1.2 However, only control-plane operators will reacto to the `CSIMigrationControlPlane` *FeatureSet*. The machine-config-operator (MCO) will **ignore** it.
  1.3 On the other hand, only MCO will react to the `CSIMigrationNode` *FeatureSet*. Control-plane operators will ignore it.
2. To enable CSI Migration for any in-tree plugin, the cluster administrator should:
  2.1 Add the `CSIMigrationControlPlane` *FeatureSet* to the `featuregates/cluster` object:
  ```shell
  $ oc edit featuregates/cluster

  (...)
  spec:
    featureSet: CSIMigrationControlPlane
  ```
  2.2 Once all control-plane components have restart, add the `CSIMigrationNode` *FeatureSet* to the `featuresgates/cluster` object:
  ```shell
  $ oc edit featuregates/cluster
  (...)
  spec:
    featureSet: CSIMigrationControlPlane, CSIMigrationNode
  ```
3. To disable CSI migration, the cluster administrator should perform the same steps in the opposite order:
  3.1 Add the `CSIMigrationNode` *FeatureSet* to the `featuregates/cluster` object.
  3.2 Wait for all Nodes to be drained and restarted.
  3.3 Add the `CSIMigrationControlPlane` *FeatureSet* to the `featuregates/cluster` object.
4. It is the **responsability of the cluster administrator** to guarante the ordering described above is respected.

### GA

Once CSI migration reaches GA in upstream, the associated feature gates will be enabled by default and the features will not be optional anymore. As a result, CSI migration will be enabled by default in OCP as wel, and there will not be an option to disable it.

In addition that, the *FeatureSets* created to handle the Tech Preview feature will no longer be operatinal. In that case, those *FeatureSets* should be ignored by operators.

As for the required ordering described above, the [upgrade order](https://github.com/openshift/cluster-version-operator/blob/master/docs/dev/upgrades.md#generalized-ordering) performed by CVO during a cluster upgrade will take care of applying the feature gates in the correct order.

#### Limitations

The limitations of this approach lies on the downgrade process.

In OCP, a downgrade is fundamentally a regular upgrade to an older version. That means that CVO downgrades components in the same order as it upgrades them: control-plane first, nodes later.

However, as stated above, this ordering is **not** desired when **disabling** CSI migration, which is what is going to happen when the previous OCP version had CSI migration disabled.

To address this issue we propose to document a simple workaround:

1. Enable both `CSIMigrationNode`and `CSIMigrationControlplane` *FeatureSets*.
1. Downgrade.

As stated above, the *FeatureSets* `CSIMigrationNode`and `CSIMigrationControlplane` are not operational once CSI migration becomes GA. However, they will be carried over during the downgrade and they will be correctly applied once the system is downgraded.

### Risks and Mitigations

Although this approach does what we need, it has some drawbacks:

1. Having operators ignoring certain *FeatureSets* is not common and error-prone.
1. We need to patch many operators in order to ignore *FeatureSets*.
