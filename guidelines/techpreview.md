# What does it mean to be Tech Preview

* You can change APIs that are clearly indicated as tech preview, without following a deprecating or backwards compatibility process.
  * Note: you cannot change the type of an existing field w/o changing the apiversion.  Changing types has serialization
implications that can break existing code in significant ways.  Adding/removing/renaming fields is fine, if you need to change a
type, rev the apiversion.  (You do not have to offer migration from the old API since it was tech preview).  Old fields should be left commented in the file along with the release they were removed in so that we can determine when there is sufficient skew + storage migration to allow re-use of the field.
* You are not required to fix bugs customers uncover in your TP feature.  How much support you give to a customer who hits an
issue with the feature is up to you (but see below about doc+CEE training requirements).
* You do not have to provide an upgrade path from a customer using your TP feature to the GA version of your feature.
  * You must still support upgrading the cluster and your component, but it’s ok if the TP feature doesn’t work after the upgrade.
  * If you can’t support upgrading the cluster+component when this TP feature is enabled, you must actively block the upgrade by
setting the upgradeable=false condition on your ClusterOperator and utilizing the TechPreviewNoUpgrade feature gate.
* You still need to provide docs (which should make it clear the feature is tech preview)
* You still need to provide education to CEE about the feature



# Reasons to declare something Tech Preview

* You aren’t confident you got the API right and want flexibility to change it without having to deal with migrations
  * Bearing in mind the aforementioned restrictions on changing field types w/o revising the apiversion.
* You aren’t confident in the implementation quality (scalability, stability, etc) and do not want to have to support customers using it in production in ways the implementation cannot handle


# Downsides to declaring something Tech Preview
* Since your feature requires special action to enable, you won’t get default CI coverage.  You may need to introduce your own
CI job that enables the TP feature if you want automated coverage
* To date we have seen very few customers enabling feature gates (in part because they block upgrading that cluster) so if your
feature is behind the cluster feature gate, you are unlikely to get meaningful feedback from the field to help you evolve the
feature anyway.  It may be better to just hold the feature until it’s GA ready.


# Official process/mechanism for delivering a TP feature

1. Your feature must be disabled by default
1. Enabling it must require the admin enable a specific feature gate such as [TechPreviewNoUpgrade](https://docs.openshift.com/container-platform/4.1/nodes/clusters/nodes-cluster-enabling-features.html)
1. You need to list the feature in [this set](https://github.com/openshift/api/blob/bace76a807222b30bb9bfd4926826348156fb522/config/v1/types_feature.go#L117)
1. Your operator then needs to observe whether the feature gate is enabled and then, and only then, can it enable the feature
meaning installing any TP CRDs and performing the TP behavior.
1. optional:  you can have additional mechanisms for enabling the TP feature that are checked in addition to the cluster-scoped
feature gate mechanism, but you must have the feature gate mechanism.
1. optional:  if your feature gate is not enabled and the TP fields are populated by the user it is recommended that your
component should clear that data from the fields to avoid user confusion when they think they’ve configured the feature but
it’s not actually active/enabled.


## Following this process means

* No customer will be able use your feature by default
* If they do enable it, they cannot upgrade their cluster, so very few will use it.  You will not likely get much user feedback
on the feature if that is your goal.
* Your feature will not be available in CI clusters unless you create your own specific CI job that enables the featuregate so
you can test the feature.
* You must not install any CRDs related to the TP feature unless the TP is enabled.



# Unofficial Processes/Mechanisms

## Option 1

If you are confident in the API (or your TP Feature does not require API changes), and you are prepared to allow upgrades for
customers who enabled this feature (you do not have to keep the TP feature working during/after the upgrade, but you have to
allow the upgrade and not fail/crashloop/etc) you can take a more minimal approach to exposing your feature as TP by

1. Requiring the admin to set some field that contains TechPreview to true, such as “EnableTechPreviewFooBar=true” to enable your
feature
  1.1 For OLM operators where the entire operator is considered Tech Preview, clearly describing the operator as Tech Preview(or using a channel with TechPreview in the name) is sufficient as the admin must still take an explicit action to install/enable your operator.
1. Ensuring all the feature documentation and release notes are clear that the feature is tech preview

This meets the absolute minimum bar that no one uses a TP feature without at least their admin having made a deliberate choice to
turn it on.

## Option 2

If you are not confident in the API, and you are introducing the API as a new resource type:

1. put it in a “v1alpha1” or “v1beta1” version.
1. require a field named TechPreviewXXXX be set by the admin to enable the feature

In a future release you can remove or rename the TechPreview enablement field.  This will break/disable the feature for existing
customers who will need to re-apply the config using the GA API, unless you are enabling the feature by default in GA.

When you GA you can also move your new resource to v1 (along with any restructuring you want to do) and abandon any existing
v1alpha1/v1beta1 resources in place (you do not have to provide a migration path for them, your operator can ignore them, etc).

## Option 3

If you are not confident in the API and you are introducing new fields to an existing GA resource:

1. name the new fields with a TechPreview prefix
1. require a field named TechPreviewXXXX to be set by the admin to enable the feature

In a future release you can remove the TechPreview prefix and remove or rename the enablement field.  This will break/disable the
feature for existing customers who will need to re-apply the config using the GA API.

There may be some gotchas around the removal of fields from the API when those fields are populated in etcd?  (It might cause
some problems around validation that need to be explored/thought through)
