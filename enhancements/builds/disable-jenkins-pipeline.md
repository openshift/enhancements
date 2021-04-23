---
title: disable-jenkins-pipeline
authors:
  - "@adambkaplan"
reviewers:
  - "@gabemontero"
  - "@akram"
approvers:
  - "@bparees"
  - "@sbose78"
  - "@derekwaynecarr"
creation-date: 2021-04-21
last-updated: 2021-08-25
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# Disable Jenkins Pipeline Strategy By Default

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenShift 3.0 included a way to invoke a Jenkins pipeline from an OpenShift build.
The status of the build on the OpenShift cluster would reflect the state of the pipeline in Jenkins.
This was the first feature to support continuous integration/delivery processes on OpenShift.

As OpenShift and Kubernetes have evolved, so has cloud-native CI/CD.
Tekton and its downstream distribution - OpenShift Pipelines - provide a way of running CI/CD processes natively on Kubernetes.
The Jenkins pipeline strategy was officially deprecated in OpenShift 4.1, but at that time there was no meaningful replacement.
We now have that meaningful replacement with the general availability of OpenShift Pipelines.

To discourage use of the Jenkins pipeline strategy, this proposal will disable runs of Jenkins Pipeline builds by default.
Clusters which rely on the Jenkins pipeline strategy can enable it through cluster's build controller configuration API (`build.config.openshift.io/cluster`).

## Motivation

Distributing and supporting Jenkins directly has always been challenging for OpenShift.
Jenkins is based in Java and is extended by a convoluted ecosystem of plugins, which are a constant source of bugs and CVEs.
Jenkins itself is not native to Kubernetes and does not use a Java runtime that has been optimized for cloud-native environments (i.e. Quarkus).
As a result, it has poor resilience when run on Kubernetes.

With the GA release of OpenShift Pipelines, we would like to decouple Jenkins from OpenShift.
Requiring additional configuration to enable the JenkinsPipeline build strategy is a step in this direction.
In the future this configuration option can be deprecated, thereby disabling the JenkinsPipeline build strategy for all clusters.
Full deprecation and removal should be addressed in a separate enhancement proposal.

### Goals

* Disable the Jenkins pipeline build strategy by default.

### Non-Goals

* Document how JenkinsPipeline strategy builds can be migrated off of OpenShift.
* Migrate Jenkins users to OpenShift Pipelines/Tekton.
* Permanently disable the JenkinsPipeline strategy.
* Deprecate the `Custom` build strategy.

## Proposal

### User Stories

As a developer using Jenkins on OpenShift
I want to continue using my Jenkinsfiles
So that I can continue to use Jenkins for my CI/CD processes

As an OpenShift product manager or SRE
I want to know which clusters have the JenkinsPipeline strategy enabled
So that I know which clusters rely on OpenShift's Jenkins integration

### Implementation Details/Notes/Constraints [optional]

#### Disabled Build Strategy Behavior

The Jenkins Pipeline build strategy will be disabled by default for new OpenShift clusters.
When a strategy is disabled, OpenShift will allow Build and BuildConfig objects to reference the disabled strategy.
However, build controller will immediately fail Build objects that reference a disabled strategy.
The build's status will clearly indicate that the disabled strategy caused the build to fail.
The failed build should also record a Kubernetes "Warning" event, indicating that the build failed because the build strategy is disabled.

The code implementing the Jenkins Pipeline build strategy will not be removed.
Rather, it will be invoked only if the build controller is explicitly configured to enable JenkinsPipeline build strategies via controller configuration.

In keeping with alerting best practices, builds that fail in this fashion should _not_ trigger a Prometheus alert.

#### Explicitly Enable the Jenkins Pipeline Strategy

The Jenkins Pipeline strategy can be re-enabled through a new option in the cluster-wide Build Controller configuration:

```yaml
kind: Build
apiVersion: config.openshift.io/v1
metadata:
  name: cluster
spec:
  ...
  enableDeprecatedStrategy:
    jenkinsPipeline: true
```

The operator for openshift-controller-manager will be enhanced to read this setting.
If this setting is set to `true`, an appropriate configuration option will be passed to the build controller's configuration (in openshift-controller-manager).
The build controller will then enable the strategy and allow JenkinsPipline builds to proceed as normal.

The structure of this API allows other strategies to be deprecated in the future.
For example, we may deprecate the `Custom` build strategy and ask those users to try Tekton Tasks via OpenShift Pipelines.

#### Bootstrap Behavior

When the `jenkinsPipeline` option is introduced, the openshift-controller-manager-operator will bootstrap the value by querying all BuildConfig and Build objects to see if any utilize the JenkinsPipeline strategy.
If any BuildConfig or Build uses the JenkinsPipline strategy, the `jenkinsPipeline` option will be set to `true`.
This check will be a part of the operator's normal reconciliation loop.
However, a status condition will indicate if this bootstrap check has been completed.
If the bootstrap check has been completed, it should not be run again.
These protections will ensure that we do not overwrite the `jenkinsPipeline` value if it is set as a day 2 action.

#### oc <new|start>-build

`oc new-build` should print a warning message if the generated/referenced BuildConfig uses the Jenkins pipeline strategy.
This message should indicate that the cluster must have the `jenkinsPipeline` option enabled.
Similarly, `oc start-build` should print a warning message if a build was started with the Jenkins pipeline strategy indicating that the cluster must have the `jenkinsPipeline` option enabled.

#### Web Console

As is the case today, users will be able to deploy Jenkins on OpenShift by instantiating one of our provided Templates.
Existing console integrations with Jenkins should remain in place as long as the Jenkins Pipeline strategy can be run on a fully supported cluster (i.e upgrades are allowed).

#### Telemetry / Operational Readiness

Openshift-controller-manager-operator will expose a gauge metric which indicates if the JenkinsPipelineStrategy has been enabled - `openshift_build_jenkins_pipeline_enabled`.
If enabled, the value will be set to 1.
This metric will then be exported to OpenShift telemetry so fleet operators can identify which clusters have Jenkins enabled (ex - `openshift:build_jenkins_pipeline_enabled:sum`).

An Insights rule should also be proposed - a cluster should be flagged if:

* The cluster has any JenkinsPipeline builds (`openshift:build_by_strategy:sum{strategy="jenkinspipeline"} > 0`)
* The cluster has JenkinsPipeline disabled (`openshift:build_jenkins_pipeline_enabled:sum == 0`)

A dashboard could also be constructed using the queries above, which can enable proactive support.

The bootstrap behavior will ensure that most clusters which rely on the JenkinsPipeline build strategy will have it enabled automatically.

#### Documentation

Documentation related to the Jenkins Pipeline build strategy will need to inform admins that the `jenkinsPipeline` option to be enabled for builds to succeed.

Examples for Jenkins usage in openshift/origin should be removed.
Tests which utilized this example deployment should instead instantiate one of the Jenkins templates installed via the Samples operator.

### Risks and Mitigations

**Risk**: Jenkins pipeline builds fail on cluster upgrade, requiring manual intervention.

*Mitigation*: openshift-controller-manager-operator will include bootstrap logic to detect if the cluster uses Jenkins Pipeline builds and automatically set the enableDeprecatedJenkinsPipeline=true build controller configuration.

**Risk**: Bootstrap logic can overwrite the cluster admin's desired spec.

*Mitigation*: Bootstrap logic uses a status condition to check if it should be run.

## Design Details

### Open Questions [optional]

None.

### Test Plan

CI already has an existing test suite for the JenkinsPipeline build strategy.
Setup for this suite will need to enable the Jenkins Pipeline build strategy before any tests run.
A separate test will need to run against a standard OpenShift cluster that verifies JenkinsPipeline builds fail immediately.

Tests for the `oc` command line will need to be updated as follows:

* JenkinsPipeline builds started via `oc start-build` should issue a warning.
* JenkinsPipeline BuildConfigs created via `oc new-build` should issue a warning.

### Graduation Criteria

#### Dev Preview -> Tech Preview

Not required - this continues the deprecation of a GA feature.

#### Tech Preview -> GA

Not required - this continues the deprecation of a GA feature.

#### Removing a deprecated feature

The Jenkins pipeline strategy has been officially deprecated since OCP 4.1.
The APIs that reference it are [Tier 1 APIs](https://docs.openshift.com/container-platform/4.8/rest_api/understanding-api-support-tiers.html#api-deprecation-policy_understanding-api-tiers),
and have already met the established criteria for alering/removing behavior.
No fields are removed in this proposal - instead this proposal modifies the default behavior of the cluster.
This proposal also does not permanently disable the strategy - rather it discourages its use by requiring admins to enable a feature through cluster configuration.
This will be documented in all places where Jenkins is referenced.

Eventually we would like to permanently disable this strategy, however under current usage this many never occur.
If in the event we do decide to permanently disable the JenkinsPipeline build strategy, we could do so as follows:

* In OCP 4.N we announce the deprecation of the `jenkinsPipeline` config option.
* In OCP 4.N+2 we add an upgrade ack for 4.N+3 - `ack-4.<N+2>-build-jenkins-pipeline-removed`.
  The controller configuration API is considered Tier 1, therefore we need to wait three releases to remove deprecated behavior.
  See the [Upgrade acks proposal](../update/upgrades-blocking-on-ack.md) for more information.
* In OCP 4.N+3 the `jenkinsPipeline` config option will be ignored (always interpreted as `false`), and build controller never enables the Jenkins pipeline strategy.

### Upgrade / Downgrade Strategy

On upgrade, the `jenkinsPipeline` configuration will default to false.
The bootstrap logic in openshift-controller-manager-operator will then set this value to `true` if any BuildConfig or Build references the JenkinsPipeline strategy.
This will ensure clusters that may rely on Jenkins pipeline builds can continue to run these builds.

On downgrade, clusters that set the `jenkinsPipeline` field will retain this value in etcd unless the cluster's build controller configuration is altered in another fashion.
The operator's boostrap logic status condition will be retained, which means this logic will not be invoked when the cluster is re-upgraded.
Therefore, cluster admins may need to re-set the `jenkinsPipeline` field after they re-upgrade.

### Version Skew Strategy

Jenkins pipeline strategy builds will NOT be gated by this new configuration until openshift-controller-manager is fully upgraded, reads the new controller configuration, and takes over leader election.
Until that happens, Jenkins pipeline builds should be able to run as expected.

## Implementation History

2021-04-23: Provisional enhancement draft.
2021-07-16: Update enhancement to disable via feature gates.
2021-07-20: Split out removal of Jenkins from the playload.
2021-08-26: Use build controller configuration instead of feature gates.

## Drawbacks

This proposal aims to disable behavior that previously functions by default.
Jenkins remains a critical part of many customer development processes - at one point Jenkins was the most widely downloaded container image from the Red Hat container catalog.
Requiring clusters to enable a configuration option could spark confusion and concern amongst end customers.

Using the cluster's build controller configuration API means that we are extending a Tier 1 API, which has strong protections for future deprecations.
Long term we would like to decouple Jenkins from OpenShift, which implies that the new `jenkinsPipeline` option will need to be deprecated.
It can take up to a year to deprecate a Tier 1 API feature, which does not include the time needed to support the deprecated feature in previous releases.
The downside of this is outweighed by the flexibility this configuration option provides.
Cluster admins will be able to enable JenkinsPipeline builds while potentially taking advantage of other features, such as tech preview features and high performance features.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

### Completely disable the Jenkins Pipeline build strategy

The first draft of this proposal sought to disable the Jenkins Pipeline build strategy in its entirety.
Not only would we disable the Jenkins pipeline strategy, we would also remove Jenkins from the OCP payload and remove a lot of functionality from the Jenkins sync plugin.
This proved untenable in the near term because we did not have sufficient telemetry to determine the full impact of this revocation.
After obtaining preliminary data from telemetry, about 8% of clusters which run builds rely on the Jenkins Pipeline strategy.
Some of these clusters are extremely reliant on the strategy, with over 1,000 Build objects that reference the JenkinsPipeline build strategy.

### Create a "DeprecatedFeatures" feature set

Another draft of this proposal used OpenShifts feature gate/feature set API to disable the JenkinsPipeline strategy.
Cluster admins would be able to turn this on by enabling a new feature set - `DeprecatedFeatures`.

This proved untenable because feature sets cannot be combined.
For instance, customers that want to run Jenkins pipeline strategy builds could not also enable the `LatencySensitive` feature set.
Customers could work around this with their own custom feature set, but doing so:

* Prevents clusters from upgrading in all use cases.
* Is known to be problematic and catasrophically error prone.
  Clusters have failed completely if a feature gate is misspelled.

Current telemetry indicates that a small number of JenkinsPipeline build users also enable a feature set.

### Keep Jenkins Pipeline strategy enabled by default

We could introduce the feature gate (`BuildJenkinsPipelineStrategy`) but not create a separate feature set.
The feature gate would then be added to the default set of features enabled by OpenShift.

This is akin to keeping free parking in a city plagued with congestion.
Deprecations necessarily require change in behavior, and keeping the JenkinsPipeline build strategy in the default set of features does not prompt behavior change.
There is little value to introducing a feature gate if we do not intend to eventually disable said feature gate.
Requiring admins to enable a feature set will hopefully prompt behavior change, as the feature set will signal that the Jenkins integration will eventually be removed.

### Keep the status quo

We can also do nothing and keep the JenkinsPipeline build strategy.
This would continue our prior strategy of making Jenkins a special application in OpenShift.
This directly contraticts our current posture with respect to Jenkins.

## Infrastructure Needed [optional]

No new infrastructure is expected.
