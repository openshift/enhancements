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

We want to allow cluster administrators to enable and disable the migration of in-tree volumes to their counterparts CSI drivers.

## Motivation

CSI migration is going to be enabled by default in Kubernetes 1.22 (OCP 4.9). As a result, volumes from in-tree plugins will be migrated to their counterpart CSI driver by default.

In order to avoid surprises once the migration is enabled by default, we want to allow cluster administrators to optionally enable the migration earlier, preferably in OCP 4.8.

## Goals

Introduce a mechanism to switch certain feature gates on and off accross OCP components *in a predefined order*.

* When the CSI migration is **enabled**, events should happen in this order:
  1. Enable the feature gate in all control-plane components.
  2. Once that's done, drain nodes one-by-one and start the kubelet with the feature gate enabled.

* When the CSI migration is **disabled**, events should happen in this order:
  1. One-by-one, drain the nodes and start the kubelet with the feature gate disabled.
  2. Once that's done, disable the feature gate in all control-plane components.

## Non-Goals

* Install or remove the CSI driver as the migration is enabled or disabled.

## Proposal

The CSI migration feature is hidden behind feature gates in Kubernetes. For instance, to enable the migration of a in-tree AWS EBS volume to its counterpart CSI driver, the cluster administrator should enable these 2 feature gates:

* CSIMigration
* CSIMigrationAWS

The resource [FeatureGate](https://github.com/openshift/api/blob/dca637550e8c80dc2fa5ff6653b43a3b5c6c810c/config/v1/types_feature.go#L9-L21) can be used to enable and disable feature gates in OCP.

### Proposal I (rejected): CustomNoUpgrade

The CustomNoUpgrade feature set can be used to enable the required feature gates. Here is an example:


```shell
$ oc edit featuregates/cluster
```

```yaml
(...)
spec:
  customNoUpgrade:
    enabled:
    - CSIMigration
    - CSIMigrationGCE
  featureSet: CustomNoUpgrade
```

Kubernetes components will restart with the features properly set.

This is **not an acceptable solution** because:

1. We can't control the order in which Kubernetes components will be restarted with the features set.
1. It's not possible to upgrade the cluster.

### Proposal II (rejected): New FeatureSet

We can create a new *FeatureSet* to enable or disable the CSI migration in a specific cloud. Each cloud platform will have its dedicated FeatureSet.

This is what should be done:

1. First, we create [a new FeatureSet](https://github.com/openshift/api/blob/master/config/v1/types_feature.go#L25-L43) called `CSIMigrationAWS`.
2. This FeatureSet contains contains 2 features gates enabled: `CSIMigration` and `CSIMigrationAWS`.
3. If a cluster administrator decides to enable the CSI Migration for AWS, they would add the FeatureSet to the `featuregates/cluster` object:
```shell
$ oc edit featuregates/cluster

(...)
spec:
  featureSet: CSIMigrationAWS
```
4. OCP will restart all components with both feature gates above enabled.
5. Once the cluster administrator decides to disable the CSI migration for AWS, they would undo the step 3 above.

Even though this solution allows the cluster to be upgraded, it's still not possible to control the order in each the components apply the feature gates.

### Proposal III (to be rejected): New FeatureSets and custom code

This is similar to the previous approach.

1. We create 2 *FeatureSets* to support CSI migration in AWS: `CSIMigrationAWSNode` and `CSIMigrationAWSControlPlane`.
  1.1 Only control-plane operators will understand the `CSIMigrationAWSControlPlane` *FeatureSet*. The kubelet will ignore it.
  1.2 Only the kubelet will react to the `CSIMigrationAWSNode` *FeatureSet*. Control-plane operators will ignore it.
2. Both FeatureSets above enable 2 feature gates: `CSIMigration` and `CSIMigrationAWS`.
3. To enable the CSI Migration for AWS, either the cluster administrator or an operator would:
  3.1 Add the `CSIMigrationAWSControlPlane` *FeatureSet* to the `featuregates/cluster` object.
  3.3 Once all control-plane components restarted, add the `CSIMigrationAWSNode` *FeatureSet* to the `featuregates/cluster` object.
4. In order to disable the migration, follow the opposite steps: first remove `CSIMigrationAWSNode`, then `CSIMigrationAWSControlplane`.

Although this approach does what we need, it has many drawbacks:

1. Having the control-plane or the kubelet ignoring certain FeatureSets is not common.
1. A new operator needs to be created.
1. We need to patch many operators in order to ignore *FeatureSets*.

### Risks and Mitigations

## Design Details

### Open Questions

There are a few things that we don't know yet:

1. Once we switch the feature gate using the `FeatureGate` resource, can we control the order that Kubernetes components will apply the change? If that's possible, we won't need the **Proposal III** above.
