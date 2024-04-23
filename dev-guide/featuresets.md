
<!-- TOC -->
* [Definition of terms](#definition-of-terms)
* [What are the benefits?](#what-are-the-benefits)
* [How to set it up?](#how-to-set-it-up)
* [When can I flip my feature to make it Accessible-by-default?](#when-can-i-flip-my-feature-to-make-it-accessible-by-default)
  * [I'd like to declare a feature Accessible-by-default.  What is the process?](#id-like-to-declare-a-feature-accessible-by-default-what-is-the-process)
    * [Automated regression protection](#automated-regression-protection)
    * [Obtaining QE sign-off](#obtaining-qe-sign-off)
* [Working with docs team](#working-with-docs-team)
* [Code](#code)
  * [openshift/api](#openshiftapi)
  * [openshift/your-repo](#openshiftyour-repo)
  * [Creating a TechPreview job for your repo](#creating-a-techpreview-job-for-your-repo)
* [How to create a cluster with TechPreviewNoUpgrade feature gates enabled](#how-to-create-a-cluster-with-techpreviewnoupgrade-feature-gates-enabled)
  * [Cluster Bot](#cluster-bot)
  * [Via the installer](#via-the-installer)
  * [Launching a ROSA cluster](#launching-a-rosa-cluster)
* [Steps to remove a feature gate](#steps-to-remove-a-feature-gate)
* [Examples](#examples)
  * [Adding TechPreviewNoUpgrade job to repo](#adding-techpreviewnoupgrade-job-to-repo)
  * [CSI via SCC](#csi-via-scc)
  * [CPU Partitioning](#cpu-partitioning)
  * [Gateway API](#gateway-api)
  * [Storage SharedResource CSI Driver](#storage-sharedresource-csi-driver)
* [FAQ](#faq)
  * [Can I merge an incomplete feature behind a feature gate?](#can-i-merge-an-incomplete-feature-behind-a-feature-gate)
  * [How can I use CustomNoUpgrade?](#how-can-i-use-customnoupgrade)
  * [I’m an SRE responsible for the uptime of a critical service.  I want to enable one single feature as Tech Preview for our customers.   How can I do that?](#im-an-sre-responsible-for-the-uptime-of-a-critical-service-i-want-to-enable-one-single-feature-as-tech-preview-for-our-customers-how-can-i-do-that)
    * [The longer answer](#the-longer-answer)
    * [Another way of thinking about the problem](#another-way-of-thinking-about-the-problem)
  * [Can a feature gate be perpetual (never removed)?](#can-a-feature-gate-be-perpetual--never-removed--)
  * [Will all FeatureGates eventually be enabled by default?](#will-all-featuregates-eventually-be-enabled-by-default)
  * [Should all feature gates eventually graduate to be an available option?](#should-all-feature-gates-eventually-graduate-to-be-an-available-option)
  * [What code changes should not be gated by feature gates?](#what-code-changes-should-not-be-gated-by-feature-gates)
  * [Should feature gates necessarily be tied to an API, i.e does it apply only when there are new APIs?](#should-feature-gates-necessarily-be-tied-to-an-api-ie-does-it-apply-only-when-there-are-new-apis)
  * [How do Feature Gates differ from configuration?](#how-do-feature-gates-differ-from-configuration)
* [Alternatives not chosen](#alternatives-not-chosen)
  * [One or more merges of on-by-default code](#one-or-more-merges-of-on-by-default-code)
  * [One or more merges of accessible-by-default code](#one-or-more-merges-of-accessible-by-default-code)
  * [Atomic merge of fully-implemented feature achieved by using feature branches across one or more repos to perform pre-merge testing and validation](#atomic-merge-of-fully-implemented-feature-achieved-by-using-feature-branches-across-one-or-more-repos-to-perform-pre-merge-testing-and-validation)
* [Deeper Technical Discussion](#deeper-technical-discussion)
  * [What happens to a feature gate once I’ve added it to OpenShift?](#what-happens-to-a-feature-gate-once-ive-added-it-to-openshift)
    * [Kube API server operator/Kube controller manager operator](#kube-api-server-operatorkube-controller-manager-operator)
    * [Machine config operator (Kubelet)](#machine-config-operator--kubelet-)
<!-- TOC -->

# Definition of terms
1. On-by-default - this feature is active and doing something without any additional configuration in a default install.
2. Accessible-by-default - this feature could be used in a default install, but it takes an additional step.
Things in this category must be upgradeable forever, backwards compatible for all future changes, and meet full support guarantees.
3. Inaccessible-by-default or TechPreviewNoUpgrade - this feature can only be used in clusters that have opted-in for preview features.
Upgrades are not supported.
Backwards compatibility (deserialization) of APIs is required.
Backwards compatibility of behavior is not required.
Support is offered, but can end with, “we’re sorry, that’s broken”.
   1. Tech Preview components may frequently use alpha-level APIs as they are evolving.
   Alpha APIs have no compatibility guarantees and can change any time from one API version to the next 
   (see [support tiers](https://docs.openshift.com/container-platform/4.13/rest_api/understanding-api-support-tiers.html) for full details).
   2. Within a single alpha API version, e.g. v1alpha1, it must always provide backwards compatibility at the deserialization level.
   Practically, this means you cannot remove fields, rename fields, change field types (e.g. go from string to int).
   This ensures other components can always CRUD these APIs without serialization errors.
   3. Across alpha API versions, e.g. going from v1alpha1 to v1alpha2, you are free to make any changes you wish.
   You can even remove an older alpha API version entirely when you introduce a newer one.
   4. It is ok to make acceptable breaking changes to a Tech Preview component as long as you never ever break the release payload tests.
   If you are making breaking changes that you know will cause CI tests to fail, you must:
      1. If possible, inform any other teams/owners so they are aware of the upcoming breaking changes
      2. Submit a PR to temporarily disable the tests that you plan to break
      3. File an OCPBUGS blocker bug tracking the need to reenable the disabled test
      4. Update dependent components to react to the breaking changes, or work with their owners to do so
      5. Submit a PR to reenable the disabled test

# What are the benefits?
1. Priority in the merge queue.
If your PR is feature gated as TechPreview, it can merge ahead of non-gated PRs, because it does not risk the stability of default installs or future upgrades.
2. Confidence of stability when moving to accessible-by-default.
TechPreview jobs run on every major cloud and test reliability can be shown in sippy.
3. “No-action” leaves us ready to release.
All features without proven reliability are not accessible-by-default in ECs, RCs, and releases.  
4. Enables cost-efficient statistical testing to provide deeper validation for race-conditions, flaky failure modes, and disruption budgets.
5. Creates an atomic signal (moving a feature gate from TechPreviewNoUpgrade to On-by-Default) indicating a new cluster capability is enabled.
This is useful as a tool for coordinating cross-functional teams, or specifying which new capabilities are available in an EC build.

# How to set it up?
1. Open PR to openshift/api adding a feature gate as TechPreview.
The threshold to merge is “enhancement is open”.
This can happen well ahead of implementation.
2. The PR to your-repo adding functionality includes vendoring openshift/API to get the feature gate constant.
This is your standard implementation PR.
3. Add tests and watch the reliability of those tests in sippy.
4. Open PR to openshift/api moving feature gate to accessible-by-default.
The threshold to merge includes referencing the reliability of existing tests.


# When can I flip my feature to make it Accessible-by-default?
Stable features can move to accessible-by-default at any time before release branching.
Exceptions are discouraged, but can be merged subject to staff-eng approval.
Exceptions are more likely to be granted for features with regression protection via e2e tests in origin.

It’s important to note that, even with exceptions, we only make features Accessible-by-default during new minor version releases.
This is not a technical limitation — it’s just how our support lifecycle works.

##  I'd like to declare a feature Accessible-by-default.  What is the process?
There are a few ways to go about this.
At a high level they involve proving that the new feature works on all supported architectures, cloud providers,
network types, and variants (FIPS, RT, cgroupsv2, crun, etc) and has not regressed any other part of OpenShift.

### Automated regression protection
Our preferred approach is to use OpenShift CI to prove that your feature is working and that no supported topologies have regressed.
Writing new e2e tests in openshift/origin is the only automated way to make this claim and ensure that another feature won’t come along and break yours at a later date.
TechPreview jobs run on AWS, Azure, GCP, and VSphere.

The textbook example of this process for a complex feature is demonstrated by Egli Hila as he works to ship [workload partitioning](https://github.com/openshift/api/pull/1443).
In his PR Egli is doing a few powerful things:
1. He’s created new tests in the E2E suite.
This is the most powerful automated regression protection we have.
2. He’s linked directly to the tests in sippy that aggregate results across all the jobs running in tech preview mode (AWS, Azure, GCP, VSphere).

### Obtaining QE sign-off
Since QE has the ability to fully test features behind feature gates we will allow features to merge that QE has declared done.

The final merges will still have to make it through our release gating CI, so we will have some confidence that no other
features have regressed even if no new tests have been added explicitly testing this feature.
We view that as a risk, but we know there are tradeoffs for trivial features that may be difficult to test in the E2E suite.

All this said, merging features without tests shouldn’t be the norm on OpenShift.  It will likely result in a conversation or two with staff engineers.
The conversation will go smoother if the results of E2E jobs that test the functionality and ensure that regressions in future Z and Y releases can be automatically detected are linked.

QE can give their sign-off for removing tech-preview by adding the qe-approved label to the PR. Bugs identified have to be communicated through the PR.

# Working with docs team

Features that are tech preview are marked in three places in the documentation collection:
* A technology preview note in the documentation for the feature
* In the Tech Preview tables in the [release notes](https://docs.openshift.com/container-platform/4.13/release_notes/ocp-4-13-release-notes.html#ocp-4-13-technology-preview)
* In the TechPreviewNoUpgrade list in the [feature gate docs](https://docs.openshift.com/container-platform/4.13/nodes/clusters/nodes-cluster-enabling-features.html#nodes-cluster-enabling-features-about_nodes-cluster-enabling)

The Tech Preview note is set during the initial development of the feature, and the Tech Preview tables and TechPreviewNoUpgrade list are updated between branch and GA.

To support the docs team in the completion of the Tech Preview/feature gate docs, the support status of each feature needs to be set before branch.

If the support status changes after branch, make sure that your writer or the documentation contact for the feature is
aware so that the docs team can ensure that the Tech Preview tables and TechPreviewNoUpgrade list are correct and add or remove the Tech Preview note from the feature docs.

# Code
## openshift/api
```go
// in config/v1/feature_gates.go
FeatureGateGatewayAPI = FeatureGateName("GatewayAPI")
  gateGatewayAPI        = FeatureGateDescription{
  FeatureGateAttributes: FeatureGateAttributes{
      Name: FeatureGateGatewayAPI,
  },
  OwningJiraComponent: "Routing",
  ResponsiblePerson:   "miciah",
  OwningProduct:       ocpSpecific,
}
```

```go
// in config/v1/types_feature.go
TechPreviewNoUpgrade: newDefaultFeatures().
    with(gateGatewayAPI).
```

## openshift/your-repo
```go
desiredVersion := config.OperatorReleaseVersion
missingVersion := "0.0.1-snapshot"

// By default, this will exit(0) if the featuregates change
featureGateAccessor := featuregates.NewFeatureGateAccess(
  desiredVersion, missingVersion,
  configInformers.Config().V1().ClusterVersions(),
  configInformers.Config().V1().FeatureGates(),
  eventRecorder,
)
go featureGateAccessor.Run(ctx)
go configInformers.Start(config.Stop)

select {
  case <-featureGateAccessor.InitialFeatureGatesObserved():
    featureGates, _ := featureGateAccessor.CurrentFeatureGates()
    klog.Infof("FeatureGates initialized: %v", featureGates.KnownFeatures())
  case <-time.After(1 * time.Minute):
    log.Error(nil, "timed out waiting for FeatureGate detection")
    return nil, fmt.Errorf("timed out waiting for FeatureGate detection")
}

featureGates, err := featureGateAccessor.CurrentFeatureGates()
if err != nil {
    return nil, err
}
// read featuregate read and usage to set a variable to pass to a controller
gatewayAPIEnabled := featureGates.Enabled(configv1.FeatureGateGatewayAPI)
```

## Creating a TechPreview job for your repo
```yaml
- as: e2e-aws-ovn-techpreview
  steps:
  cluster_profile: aws
  env:
  FEATURE_SET: TechPreviewNoUpgrade
  workflow: openshift-e2e-aws
```

[Example PR](https://github.com/openshift/release/pull/39516)


# How to create a cluster with TechPreviewNoUpgrade feature gates enabled
## Cluster Bot
It’s a variation

`launch 4.14 gcp,techpreview`

## Via the installer
Specify the `featureSet` field in the [install config](https://github.com/openshift/installer/blob/d50f489d9f0a44f0e7e4795a3699bfcc9015bdbe/pkg/types/installconfig.go#L206).

## Launching a ROSA cluster
ROSA cli contains a hidden flag for specifying TechPreviewNoUpgrade.

`rosa create cluster --properties "install-config:featureSet:TechPreviewNoUpgrade"`

# Steps to remove a feature gate
(aka make the feature stable)
1. Move the feature gate from the techpreview featureset to the default feature set in openshift/api
2. Once above is merged, your feature is now accessible by default
3. In the next release, go into whichever code has if featuregate.Enabled(myfeature) { do thing } and remove the if clause
4. Once all if clauses are gone, you can remove the feature gate from the default feature set (delete the feature gate) in openshift/api
5. Remove the featuregate from openshift/api

# Examples
## Adding TechPreviewNoUpgrade job to repo
[cluster-config-operator](https://github.com/openshift/release/pull/39516))

## CSI via SCC
1. [Promotion PR](https://github.com/openshift/api/pull/1402/files)
2. [Test reliability link](https://sippy.dptools.openshift.org/sippy-ng/tests/4.13/details?filters=%257B%2522items%2522%253A%255B%257B%2522columnField%2522%253A%2522variants%2522%252C%2522not%2522%253Afalse%252C%2522operatorValue%2522%253A%2522contains%2522%252C%2522value%2522%253A%2522serial%2522%257D%252C%257B%2522columnField%2522%253A%2522variants%2522%252C%2522operatorValue%2522%253A%2522contains%2522%252C%2522value%2522%253A%2522techpreview%2522%257D%252C%257B%2522columnField%2522%253A%2522name%2522%252C%2522operatorValue%2522%253A%2522contains%2522%252C%2522value%2522%253A%2522CSIInlineVolumeAdmission%2522%257D%255D%252C%2522linkOperator%2522%253A%2522and%2522%257D&sort=asc&sortField=current_working_percentage)

## CPU Partitioning
1. [Test reliability link](https://sippy.dptools.openshift.org/sippy-ng/tests/4.13?filters=%257B%2522items%2522%253A%255B%257B%2522columnField%2522%253A%2522current_runs%2522%252C%2522operatorValue%2522%253A%2522%253E%253D%2522%252C%2522value%2522%253A%25227%2522%257D%252C%257B%2522columnField%2522%253A%2522variants%2522%252C%2522not%2522%253Atrue%252C%2522operatorValue%2522%253A%2522contains%2522%252C%2522value%2522%253A%2522never-stable%2522%257D%252C%257B%2522columnField%2522%253A%2522variants%2522%252C%2522not%2522%253Atrue%252C%2522operatorValue%2522%253A%2522contains%2522%252C%2522value%2522%253A%2522aggregated%2522%257D%252C%257B%2522id%2522%253A99%252C%2522columnField%2522%253A%2522name%2522%252C%2522operatorValue%2522%253A%2522contains%2522%252C%2522value%2522%253A%2522CPU%2520Partitioning%2522%257D%255D%252C%2522linkOperator%2522%253A%2522and%2522%257D&sort=asc&sortField=current_working_percentage)

## Gateway API
1. [FeatureGate addition](https://github.com/openshift/api/pull/1452/files)
2. [Implementation](https://github.com/openshift/cluster-ingress-operator/blob/960d4104d25f60e077dedb08d423cb7fe1900c0c/pkg/operator/operator.go#L100-L131)

## Storage SharedResource CSI Driver
1. [FeatureGate addion](https://github.com/openshift/api/pull/982/files)
2. [Implementation](https://github.com/openshift/cluster-storage-operator/pull/368/commits/b8b52e12a93aad8edc743a137f1029dc6d76844c#diff-cb02d71cda13d1b6baf92847263391207bf0731e074197955b0493b8ea82feceR38-R62)


# FAQ
## Can I merge an incomplete feature behind a feature gate?
Yes! A feature doesn’t need to be complete to be merged behind a feature gate, but you should consider what the impact is on other components when deciding when/where to add the feature gate.

Enabling your feature gate in TechPreview clusters is the best choice for features that are not complete, but do-no-harm.
TechPreview jobs must continue to work (you can’t break other features), but not all features need to be fully complete.

## How can I use CustomNoUpgrade?
A rare minority of features may need to be developed in complete isolation.
This isn’t recommended and it’s unusual (deads cannot think of an example in 4.y), but it is possible.
If you are going down this path because you think someone will want to turn on just your feature in a cluster to test it,
know that as of writing this has not happened in the past four years.
We strongly recommend simply using TechPreviewNoUpgrade.

However, if you must, you may add the feature gate to openshift/api and not add it to an existing feature set (eg TechPreviewNoUpgrade).
By doing this, your feature will not be active in either default/stable clusters or tech preview clusters.
You can activate the feature by using a CustomNoUpgrade feature set once you have bootstrapped a cluster.
This will allow you to test your incomplete feature in isolation without any impact on others who are leveraging the tech preview workflows.

Enable a CustomNoUpgrade feature by editing the cluster FeatureGate as below:

```yaml
apiVersion: config.openshift.io/v1
kind: FeatureGate
metadata:
  name: cluster
spec:
  featureSet: CustomNoUpgrade
  customNoUpgrade:
    enabled:
    - <my feature gate name>
```

Even if you choose to start in CustomNoUpgrade, you must eventually go through TechPreviewNoUpgrade to demonstrate that
no harm is done with broader platform coverage and payload blocking inspection.


## I’m an SRE responsible for the uptime of a critical service.  I want to enable one single feature as Tech Preview for our customers.   How can I do that?
In short, this is something you can’t do and it’s by design.
We do have CustomNoUpgrade if that is absolutely what you want.
Please keep reading for the reasoning behind our decision not to make this an option for Tech Preview feature.
There is yet another alternative at the bottom.

### The longer answer
This question is usually asked for two reasons:
1. Other successful managed services used worldwide in mission critical environments have some ability to do this.
2. There’s a misunderstanding of our strategy for testing OpenShift.

We have experience with selectively enabling features in OpenShift v3 and it went very poorly.
Yes, we could enable a feature selectively and yes it would work in production—for a little while.

In the best case a subsequent feature would attempt to land and then realize they have not accounted for substantial
architectural differences that the selectively enabled feature had made.
In worse cases, the subsequent feature instead subtly broke the selective feature in ways that were extremely hard to detect and debug.

This is because the selectively enabled feature was created in a vacuum.
It was developed all alone, tested all alone and shipped all alone.
That model mirrored the level of coordination that was happening in our development organization.

Our distinction between On-by-default, Accessible-by-default and Inaccessible-by-default/TechPreviewNoUpgrade is a design
decision that gives us a tractable test matrix.
We know that when features land they are going to work together.
Just as important, it makes it impossible to develop in a vacuum.

When people say they want to enable a single Tech Preview feature what they really mean a few things:
1. “I really need feedback on this feature in an environment where the world won’t end if something goes wrong.”
2. “I don’t want to be broken by things I don’t care about.”

Our approach to Feature Gates is designed to address both of those at the same time.

### Another way of thinking about the problem
If an environment needs the utmost guarantee that a selective feature is going to work and that nothing “unimportant” is
going to cause problems, that is the very definition of Accessible-by-default.
The feature is there waiting for you, but it will only be used when an additional step is taken.

This is different from environments that simply need early feedback.
That’s when you want  Inaccessible-by-default/TechPreviewNoUpgrade.

## Can a feature gate be perpetual (never removed)?

FeatureGates should not be intended to be perpetual.
If a FeatureGate is being used to allow users to opt in/out of GA behavior, that's probably not a correct use of FeatureGates.
If a FeatureGate is being used to gate a Tech Preview feature, then the feature should eventually graduate to GA
(at which point the gate itself can be removed) or the feature should be removed entirely(failed to graduate to GA), not be perpetual.

Note that "feature is on/available" doesn't mean that the feature itself can't offer the user a configuration option that enables/disables/changes the behavior of the feature.
Some features will merely be accessible-by-default(as opposed to on-by-default).
These accessible-by-default features that are supported in the GA product, but additional steps are needed to turn them on.

The best example for how such an optional feature is tested would be to look at how we have supported SDN and OVN networking in OpenShift.
This is very expensive and not something we’re likely to do for every optional feature due to costs.

Another canonical example would be node-swap.
Even when this feature is GA, it will be necessary for a cluster-admin to provide additional configuration to indicate how much swap to use.

Even in cases where additional configuration is required, while the feature is TechPreview, those additional configuration
options must be inaccessible to avoid a cluster-admin accidentally using a TechPreview configuration option on a cluster they want fully supported.
This is why different CRD schemas are installed for TechPreview versus Default.

## Will all FeatureGates eventually be enabled by default?

Let’s first define what is meant by “enabled by default”.  For a FeatureGate to be Enabled by default means the Feature behind the
Gate is either always on-by-default (doing something once enabled) or always accessible-by-default
(additional configuration is available(required?) to use the feature).
We tend to refer to this as promoting the gate to GA.
The feature that the gate exposes may still allow the user to opt in, or out, of the functionality it provides, via some first class configuration api.
Thus you can have
1. Enabled in TechPreview - feature is accessible, but additional configuration is required to use it.
2. Enabled in TechPreview - feature is on, no additional configuration is required to use it
3. Enabled by Default  - feature is accessible-by-default and additional configuration is required to use it
4. Enabled by default - feature is on-by-default and no additional configuration is required to use it

The feature gate and the configuration are orthogonal.
In a feature requiring configuration, the feature gate controls whether the configuration is available to be set.
You can also have a feature that has no gate (because it was introduced without a gate or the gate was removed after the feature was promoted to GA).


All feature gates we introduce should either:
1. eventually be on by default meaning they are part of the default featureset (and we offer no good way to turn them off
so on by default is effectively just "on" unless we get into the nuances of the CustomNoUpgrade featuregate set)
2. the gating logic should be removed(making the feature always on/available)
3. the feature(and gate) should be removed (fails to graduate from Tech Preview to GA).

if someone is using a FeatureGate for something other than Tech Preview protection that would be a mistake.
If they are trying to let users opt in/out of a GA feature/behavior then they should use other configuration mechanisms
(e.g. A first class api), not FeatureGates, to allow users to turn on/off the behavior.

## Should all feature gates eventually graduate to be an available option?
This is a variation of the previous question, but also important to consider.
It’s certainly possible that the wide adoption of feature gates could result in lots of work in progress that’s never completed.

We don’t feel like there is enough risk right now to create any sort of policy around how long a feature can stay in TechPreviewNoUpgrade.
All the pieces are in place that would allow us to create a report to show "things in flight".
Perhaps in some future state we would limit the number of TechPreviewNoUpgrade features in OpenShift to ensure we’re finishing features before moving on to new ones.
That seems very far from our current reality though.

## What code changes should not be gated by feature gates?
The Kubernetes rebase is a good example of a large change that doesn’t really map well to feature gates.
The teams involved in landing these changes have worked with the TRT to create sufficient pre-merge signal to greatly
decrease the regression risk—effectively achieving the same goal as Feature Gates via another route.

Small features that take less than a sprint to fully develop, merge and test don’t really benefit from Feature Gates.
Those sorts of features haven’t been the biggest cause of our release risk.

## Should feature gates necessarily be tied to an API, i.e does it apply only when there are new APIs?
APIs provide a nice boundary, but Feature Gates can apply to non-API changes.  It's a balance of size/risk.

## How do Feature Gates differ from configuration?
It’s true that it’s possible to release a feature that’s hidden behind configuration.
The biggest difference is that our configuration is one of our APIs.
Configuration never goes away.
Feature Gates can and do disappear by design.

# Alternatives not chosen
Beyond using feature gates, there are other approaches to merge functionality:

## One or more merges of on-by-default code
1. This puts ECs, RCs, and releases at risk if there is problem with delivery. 
Aes-gcm is a good recent example.
2. For any feature requiring more than one PR, there is no way to gain reliable CI signal prior to having the code accessible-by-default.
3. Revert of multiple PRs is often/usually difficult
4. A single giant PR is unwieldy to review
5. If you’re small enough to have a reasonable-sized single PR, featuregates likely aren’t targeted for you, just make your one small PR.

## One or more merges of accessible-by-default code
1. This puts ECs, RCs, and releases at risk if there is problem with delivery.
Aes-gcm is a good recent example.
2. For any feature requiring more than one PR, there is no way to gain reliable CI signal prior to having the code accessible-by-default.
3. Revert of multiple PRs is often/usually difficult
If you’re small enough to have a reasonable-sized single PR, featuregates likely aren’t targeted for you, just make your one small PR.

## Atomic merge of fully-implemented feature achieved by using feature branches across one or more repos to perform pre-merge testing and validation
1. It’s impossible to gain CI signal from multiple PRs.  /payload doesn’t support it.
2. While developing those branches, you’ll have to be rebasing your PRs on other features landing in various repos
3. The payload doesn’t take into account your changes.  Remember, we have CI jobs for TechPreview clusters and we do require that to keep working.
4. With no way to gain CI signal cross our configurations, there either are not automated tests or we don’t know if they work consistently across the 100ish configurations.

# Deeper Technical Discussion
## What happens to a feature gate once I’ve added it to OpenShift?
Once a feature gate is added to the openshift/api repository.
The cluster-config-operator will start reporting the feature gate exists and whether it is enabled or disabled for a particular version.

Typically, most operators will not be affected by the addition of a new feature gate, but once added the feature gate is
available to be consumed by an operator, i.e. it is available for functionality to gate on the presence of the feature gate.

Some operators however, will act automatically upon the presence of a new feature gate, and this can have a negative impact on payload stability.
Consider the behaviour of the operators outlined below when a new feature gate is added, especially if the feature gate
could overlap with an upstream kubernetes feature gate.

### Kube API server operator/Kube controller manager operator
These operators use a [config observer](https://github.com/openshift/library-go/blob/be85f840097533832dea477f08ed10a574f36353/pkg/operator/configobserver/featuregates/observe_featuregates.go#LL20C8-L20C8)
defined in library-go to map the FeatureGate status through to a list of enabled and disabled features to be[passed directly to their operands](https://github.com/openshift/cluster-kube-controller-manager-operator/blob/1e15498f434994aa6676c146638b886e08c93aa5/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L125C1-L139).
Once a feature gate is present in the FeatureGate status, these operators will pick up the new feature gates and update the feature map in their observed config.
This triggers a rollout of new static pod configuration to update the pods on each of the control plane nodes.

The config observer does not filter the features observed from the FeatureGate status apart from a [blacklist](https://github.com/openshift/library-go/blob/be85f840097533832dea477f08ed10a574f36353/pkg/operator/configobserver/featuregates/observe_featuregates.go#L24)
passed in by the caller. Typically this blacklist should be empty.
This means that all OpenShift feature gates are passed directly to the Kube API server and Kube controller manager pods without filtering.
These processes discard unknown feature gates on startup and so adding a new feature gate is typically inert.
However, if you believe adding a feature gate will negatively impact one of these components, you must make sure to add
it to the blacklist in the correct operator to prevent it being passed to the component.

### Machine config operator (Kubelet)
Once a feature gate is present in the FeatureGate status, the [kubelet-config controller](https://github.com/openshift/machine-config-operator/tree/master/pkg/controller/kubelet-config)
in MCO updates a pair of MachineConfigs (`98-master-generated-kubelet` and `98-work-generated-kubelet`) based on the updated feature gates.

Since the Kubelet feature gates are processed in the `kubelet.conf` file, [generated](https://github.com/openshift/machine-config-operator/blob/c50386e0700f4a7e3f322e3fb37000e9c3e5feb1/pkg/controller/kubelet-config/kubelet_config_features.go#LL108C21-L108C54)
by this controller, it updates the list of [enabled and disabled features for the kubelet](https://github.com/openshift/machine-config-operator/blob/c50386e0700f4a7e3f322e3fb37000e9c3e5feb1/pkg/controller/kubelet-config/kubelet_config_controller.go#L370C29-L384)
whenever it observes a change to the feature gates.
This means that, without any further action, as soon as a feature gate is merged to openshift/api, it will be passed directly to Kubelet.

Kubelet on startup discards any feature gates that it isn’t aware of, so this action is typically inert, however,
if you believe your feature could negatively impact Kubelet, you must make sure to [exclude it](https://github.com/openshift/machine-config-operator/blob/c50386e0700f4a7e3f322e3fb37000e9c3e5feb1/pkg/controller/kubelet-config/kubelet_config_features.go#L26-L29)
from the features by marking it in MCO as an OpenShift only feature gate. This will prevent MCO from passing it directly to Kubelet.


