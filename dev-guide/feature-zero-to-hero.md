# OpenShift Feature Development Zero-to-Hero Guide

_Guidance on how to contribute a feature to OpenShift from beginning to end_

> [!NOTE]
>While this document is intended to help guide you through the OpenShift feature development process, this document may not yet fully document everything necessary for a new feature.
>If you use this document to guide development of your feature and identify some knowledge gaps, please feel free to propose changes to the document and tag or reach out to the OpenShift API reviewers in [`#forum-api-review`](https://redhat.enterprise.slack.com/archives/CE4L0F143)

## Proposing Your New Feature

### Writing an OpenShift Enhancement

OpenShift follows a document-driven feature development process and we take
heavy inspiration from the Kubernetes Enhancement Proposal process.

All new OpenShift features are required to go through the OpenShift Enhancement
Proposal process.

Once you are confident a feature is worth implementing, following the process outlined in the OpenShift Enhancement Template is a great place to start.

All OpenShift Enhancements live in https://github.com/openshift/enhancements.

The goal of an OpenShift Enhancement is to ensure that proposed changes have been effectively communicated to and agreed on by appropriate stakeholders:
- The motivations for implementing this feature. This includes, but is not limited to:
  - Goals and non-goals of the feature
  - User stories
- The proposed approach for implementing the feature. This includes, but is not limited to:
  - How we expect users to use the feature
  - API additions/changes necessary to use the feature
  - How it impacts different cluster topologies and whether or not it will work on those different cluster topologies
  - Risks/Drawbacks of implementing the feature and their mitigations
  - Alternative approaches
  - How upgrades/downgrades and version skew is handled
- The plan for testing the feature. This is a high-level strategy for how we can verify in CI that this feature is working as expected.
- The signals we will use to identify feature maturity and thus graduation of the feature to being enabled by default.
- The procedures for diagnosing problems with the feature during support scenarios

It is important to note that for the graduation of features, **ALL** new OpenShift features must start out as being disabled by default. We will go over this in more detail in the “Creating a new Feature Gate” section further down in this document.

### Getting Your Enhancement Reviewed/Approved

In the OpenShift Enhancement Template, there are 3 sections for specifying the people responsible for reviewing and approving your enhancement proposal.

#### Reviewers

Reviewers are subject matter experts generally responsible for reviewing the technical details of your enhancement proposal to make sure that they make sense. When specifying reviewers of your enhancement proposal, include a comment explaining the reviewer’s domain expertise and the sections of the enhancement you’d like their input on.

In general, reviewers should be from your own team, any affected teams, and other applicable stakeholders.

As an example, see: https://github.com/openshift/enhancements/blob/c0d057000d5ca5d00ae47a57bd501a861983e325/enhancements/authentication/adding-uid-and-extra-claim-mapping-configuration-options-for-external-oidc.md?plain=1#L5-L6 

#### Approvers

Approvers are the people responsible for ensuring that the enhancement proposal has received reviews from appropriate subject matter experts. They are also responsible for determining when consensus has been reached so that the enhancement proposal can be merged and start being implemented.

Generally, it is preferred to have a single approver to prevent confusion for who is ultimately responsible for approval. Approvers are usually team leads and/or staff engineers.

#### API Approvers

API Approvers are the subject matter experts responsible for reviewing and approving any proposed API changes. This can be adding new APIs or modifying existing APIs that are shipped with OpenShift.

Explicit API reviews here help to ensure:
- Consistent user experiences across OpenShift APIs
- Best practices for API design are being followed (API reviewers review lots of APIs and have deep knowledge of how API decisions have aged across OpenShift)

The canonical list of API Approvers can be found in https://github.com/openshift/api/blob/master/OWNERS under the approvers section.

Please use the [`#forum-api-review`](https://redhat.enterprise.slack.com/archives/CE4L0F143) Slack channel to request an API review so that the API review team can assign a reviewer.

## Implementing Your New Feature

This section is a high-level overview of some of the common things that any new feature will likely need to do during the implementation process and is not prescriptive as to how features are implemented end-to-end.

### Creating a New Feature Gate

As mentioned at the end of the “Writing an OpenShift Enhancement Proposal” section, ALL OpenShift features must start out being disabled by default and therefore are not intended for use in production.

We achieve this through the use of feature gates within feature sets. The 4 feature sets are:
- **CustomNoUpgrade (CNU)** - allows the enabling or disabling of any feature. Turning this feature set on IS NOT SUPPORTED, CANNOT BE UNDONE, and PREVENTS UPGRADES. Because of its nature, this setting cannot be validated. If you have any typos or accidentally apply invalid combinations your cluster may fail in an unrecoverable way. Customization done here is additive to the Default set.
- **DevPreviewNoUpgrade (DPNU)** - turns on dev preview features that are not part of the normal supported platform. Turning this feature set on CANNOT BE UNDONE and PREVENTS UPGRADES.
- **TechPreviewNoUpgrade (TPNU)** - turns on tech preview features that are not part of the normal supported platform. Turning this feature set on CANNOT BE UNDONE and PREVENTS UPGRADES.
- **Default** - features that are enabled by default and are fully supported in production environments.

The inherent ordering to feature sets here is `DevPreviewNoUpgrade > TechPreviewNoUpgrade > Default`.
Simply put, all Default features are inherently enabled in `TechPreviewNoUpgrade` and all `TechPreviewNoUpgrade` features are inherently enabled in `DevPreviewNoUpgrade`.

Most new features will start in either the `DevPreviewNoUpgrade` or `TechPreviewNoUpgrade` feature sets.
While there is not a requirement to start in one or the other it is recommended to begin feature development in `DevPreviewNoUpgrade` to prevent introducing regressions to features currently enabled in `TechPreviewNoUpgrade`.
TRT will revert any changes that cause a regression in `TechPreviewNoUpgrade`, but will not revert changes made in `DevPreviewNoUpgrade`.

To add a new OpenShift feature gate, you add the feature gate to https://github.com/openshift/api/blob/master/features/features.go using the builder pattern.

As an example, see: https://github.com/openshift/api/blob/8a46f746f2cf87624651e6e8a85421b49bef3b6e/features/features.go#L472-L478

For more information, see https://github.com/openshift/api?tab=readme-ov-file#adding-new-featuregates 

New feature gates must specify:
- A Jira component (to file bugs/issues against)
- A contact person responsible for the feature
- An OpenShift Enhancement Pull Request. This PR should be merged and related to the feature.
- The feature sets in which this feature is enabled by default.
- The “owning product”. For most features this will be OpenShift (`ocpSpecific`). The only time this will be different will be when it is an upstream Kubernetes feature.

For more information on the benefits of feature gates and other things to consider, see https://github.com/openshift/enhancements/blob/master/dev-guide/featuresets.md 

### Using Your New Feature Gate

OpenShift follows the operator and operand pattern.
Operators are responsible for watching feature gate configurations and updating their operands with corresponding configuration options.
Operands should not be concerned with reading OpenShift feature gates.

To use your new feature gate in an OpenShift operator to gate new functionality, you can use the following Go packages:
- https://pkg.go.dev/github.com/openshift/library-go@v0.0.0-20250919173008-7fa221ceac52/pkg/operator/configobserver/featuregates 
- https://pkg.go.dev/github.com/openshift/api/features

#### Creating a Feature Gate Accessor

The first step to using feature gates in a controller is to create a feature gate accessor.
This is an asynchronous process that reacts to changes in the `featuregates.config.openshift.io/v1` resource with the name `cluster`.
The accessor is responsible for determining the feature gate states based on the currently selected feature set.

As an example, see: https://github.com/openshift/cluster-authentication-operator/blob/ed0d09e6a99743a14b1a48cf131e3e9125c86bf7/pkg/operator/replacement_starter.go#L348-L360 

#### Accessing Feature Gate State

Once you’ve created a feature gate accessor, you can use it to get the current state of feature gates on an OpenShift cluster.

As an example, see: https://github.com/openshift/cluster-authentication-operator/blob/ed0d09e6a99743a14b1a48cf131e3e9125c86bf7/pkg/operator/starter.go#L747-L753

Once you’ve gotten the current state of feature gates, you can check whether or not particular features are enabled.

As an example, see: https://github.com/openshift/cluster-authentication-operator/blob/ed0d09e6a99743a14b1a48cf131e3e9125c86bf7/pkg/operator/starter.go#L755-L757

### Creating a New OpenShift API Kind

All new OpenShift APIs must start as an unstable API.
This means that it starts as `v1alpha1`.

All OpenShift APIs should be created in https://github.com/openshift/api.

All OpenShift APIs follow the [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md) with some explicit deviations outlined in the [OpenShift API Conventions](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md#openshift-api-conventions).

New APIs should be created in an appropriate directory for construction of the group name.
The group name pattern followed is `{directory}.openshift.io/{version}`.

As an example, see: https://github.com/openshift/api/tree/master/example

For additional information, see: https://github.com/openshift/api?tab=readme-ov-file#defining-new-apis

### Updating an Existing OpenShift API

When updating an existing OpenShift API, the [Kubernetes](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md) and [OpenShift](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md#openshift-api-conventions) API conventions should be followed.

All new fields and validations must be added behind a feature gate. This is done by using feature gate markers. The current set of feature gate markers are:
- `+openshift:enable:FeatureGate={featureGateName}` - Used to only include a field in CRDs for feature sets where the feature gate is enabled by default. Example: https://github.com/openshift/api/blob/8a46f746f2cf87624651e6e8a85421b49bef3b6e/example/v1/types_stable.go#L40
- `+openshift:validation:FeatureGateAwareEnum` - Used to only include an enum validation for the field/type in CRDs for feature sets where the feature gate is enabled by default. Example: https://github.com/openshift/api/blob/8a46f746f2cf87624651e6e8a85421b49bef3b6e/example/v1/types_stable.go#L122-L123
- `+openshift:validation:FeatureGateAwareMaxItems` - Used to only include a maxItems validation for the field/type in CRDs for feature sets where the feature gate is enabled by default. Example: https://github.com/openshift/api/blob/8a46f746f2cf87624651e6e8a85421b49bef3b6e/example/v1/types_stable.go#L82-L83
- `+openshift:validation:FeatureGateAwareXValidation` - Used to only include a CEL validation in CRDs for feature sets where the feature gate is enabled by default. Example: https://github.com/openshift/api/blob/8a46f746f2cf87624651e6e8a85421b49bef3b6e/example/v1/types_stable.go#L35-L36

> [!NOTE]
>If there does not exist a `+openshift:validation:FeatureGateAware...` marker for your specific use case, one can be implemented in our [fork of controller-tools](https://github.com/openshift/kubernetes-sigs-controller-tools).
>Please reach out to the API review team for any help here.

### Testing Your API Changes

We require integration testing for all API changes to ensure that any validations are working as expected and as a way of catching regressions to these validations when making future changes.

For more guidance on testing your API changes see: https://github.com/openshift/api/tree/master?tab=readme-ov-file#defining-api-validation-tests

### Making Your Unstable API Stable

Once you feel that your unstable API (`v1alphaN`) is ready to become stable, you’ll need to go through the process of promoting your API version from `v1alphaN` → `v1`.

Today, this often requires some co-ordination to get simultaneous merges done across the openshift/api, openshift/client-go, openshift/origin, and your component repositories.

An example of an API version promotion for a Machine Config Operator (MCO) API:
- openshift/api PR: https://github.com/openshift/api/pull/2255
- openshift/origin PR: https://github.com/openshift/origin/pull/29701
- openshift/machine-config-operator PR: https://github.com/openshift/machine-config-operator/pull/4992
- openshift/client-go PR: https://github.com/openshift/client-go/pull/320

## Promoting Your New Feature to be Accessible by Default

Stable features can be promoted any time before release branching.
Exceptions are possible to backport features to z-streams, but this is rare and requires going through the [SBAR process](https://docs.google.com/presentation/d/1djF3MaC7rgKFC3_8SPelRcBUB825vx0FO8tz_HwzeWM/edit?usp=sharing) to receive approval.

Once your feature has been promoted as accessible by default for at least one minor version release, it is encouraged to remove your feature gate and any unstable API version code.

### Promotion Requirements

In order to promote your feature to the Default feature set, you must prove that the feature works on **ALL** supported architectures, cloud providers/platforms, network types, and variants.
Your feature **must not** regress any other part of OpenShift.

We use automated regression testing, called Component Readiness, to ensure that features are ready for promotion and that promoted features do not get regressed by future changes.
We will go into more detail on how to set up automated regression testing in another section.

It is highly recommended that once you have identified that you’d like to pursue feature promotion you open a WIP PR (a PR with the prefix `WIP:` in the title) against openshift/api.

There is a pre-submit job that runs on all feature promotion PRs to verify whether or not feature promotion requirements have been met.
You can use the results of that job to get a signal as to what requirements you still need to meet for promotion.
You can re-trigger the job by commenting `/test verify-feature-promotion` on the PR.
Additionally, you can see what tests will be picked up by looking at `https://sippy.dptools.openshift.org/sippy-ng/feature_gates/{openshiftRelease}/{featureGateName}`.

Example of a feature promotion PR: https://github.com/openshift/api/pull/2454

#### Testing Requirements

The current testing requirements are:
- At least 5 tests per feature
- All tests must be run at least 7 times per week
- All tests must be run at least 14 times per supported platform
- All of the above should be in place no less than 14 days before branch cut for any feature hoping to be enabled by default in that release.
- All tests must pass at least 95% of the time.
- Your tests will be run in both `TechPreviewNoUpgrade` and `Default` Prow job variants (tests will be skipped in stable jobs until promoted, but this ensures coverage continues once promoted).
- All tests are run on all supported platforms. The canonical list of these can be seen [here](https://github.com/openshift/api/blob/8a46f746f2cf87624651e6e8a85421b49bef3b6e/tools/codegen/cmd/featuregate-test-analyzer.go#L332-L383). Currently, these are:

| Provider | Topology | Architecture | Network Stack |
| -------- | -------- | ------------ | ------------- |
| AWS | HA | amd64 | default | 
| AWS | Single | amd64 | default |
| Azure | HA | amd64 | default |
| GCP | HA | amd 64 | default |
| vSphere | HA | amd64 | default  |
| Baremetal | HA | amd64 | IPv4 |
| Baremetal | HA | amd64 | IPv6 |
| Baremetal | HA | amd64 | Dual |

> [!NOTE]
> All baremetal IPv6 jobs run your tests against a disconnected cluster. Your tests must be able to handle this or be exempt from running against a disconnected cluster.


##### Exceptions

Exceptions to these testing requirements are discouraged, but possible.
The easiest exception to receive is if your feature is explicitly not supported on one of these platforms (most likely due to a technical constraint).

In order to receive an exception to these testing requirements when your feature is supported on the platforms not being covered, you must follow the [SBAR process](https://docs.google.com/presentation/d/1djF3MaC7rgKFC3_8SPelRcBUB825vx0FO8tz_HwzeWM/edit?usp=sharing) and include a plan for how the feature owner will provide feature regression protection equivalent to the automated regression testing.

The plan must take into account:
- Which data sources regression data will be pulled from
- The required frequency and quantity of tests mentioned in the promotion requirements
- How regression probability will be calculated

All features promoted through an exception are required to present weekly regression probability for each z-stream and the current development branch until automated regression testing has been implemented.

For an example of how this weekly regression probability analysis can be gathered and presented see:
- [Aggregated OCL Test Results](https://docs.google.com/spreadsheets/d/1mfaK1b-XYOWFsMTDf06cvBhwU5uOkgRHUFp0KzI3qDc/edit?gid=1635329499#gid=1635329499)
- [Data extraction from QE CI: A journey](https://docs.google.com/document/d/1bg1nlZYbEqtRU8lL4p1OOTrDxAyoj_MLEZLBIbiV1Cc/edit?tab=t.0#heading=h.t3zt329hl4se)
- https://gitlab.cee.redhat.com/zzlotnik/openshift-ci-stuff (requires VPN)

If you are having a hard time getting sufficient runs “naturally”, you can use gangway-cli to trigger additional runs of your Prow jobs.
A handy shell script for running multiple jobs in succession with some configuration options can be found here: https://gist.github.com/everettraven/7348f12b9e758dbdd76a9423b2450eed (edit to suit your needs).
Keep in mind that there is rate limiting that is enforced when using gangway-cli.
For more information on gangway-cli and the Prow REST endpoint, see https://docs.ci.openshift.org/docs/how-tos/triggering-prowjobs-via-rest/ .

## Testing Your New Feature

All OpenShift E2E tests must report into Sippy (and eventually into Component Readiness).
Historically, these are defined in https://github.com/openshift/origin.
There is now a new way to define tests outside of openshift/origin called OpenShift Test Extensions (OTE).
For more details on OTE, see: https://github.com/openshift-eng/openshift-tests-extension and [Integration Guide for OpenShift Tests Extension](https://docs.google.com/document/d/1cFZj9QdzW8hbHc3H0Nce-2xrJMtpDJrwAse9H7hLiWk/edit?usp=sharing).

For now, this section will focus on the openshift/origin approach.

### Determining the Types of Your Tests

In OpenShift, we have a few different “types” of tests.
These follow the Kubernetes test kinds.
For additional information on test types, see: https://github.com/openshift/origin/tree/main/test/extended#test-labels

The type of tests you need to write will have an impact on the work you need to do to get your tests running and reporting into Sippy.

If your tests are disruptive and/or slow, see the guidance in the “Considerations for Disruptive/Slow Tests” section.
Otherwise, continue with the “Writing Your Tests” section.

### Considerations for Disruptive/Slow Tests

Today, there is no default test suite to run disruptive and slow tests like there is for serial and parallel tests.
This means that to add a new set of tests that may cause disruption in the cluster or run slowly you’ll need to add a new test suite.

All new test suites must be added to https://github.com/openshift/origin/blob/main/pkg/testsuites/standard_suites.go .
There should be plenty of examples in this file to give you a starting point for creating your own test suite.

If your test suite will be disruptive, you must set the `ClusterStabilityDuringTest: ginkgo.Disruptive` option on your test suite.
This tells our monitoring test suite, that always runs, that when running this suite of tests it should expect cluster disruption to occur and should not flag issues related to cluster disruption.
An example of setting this option on a test suite can be seen here: https://github.com/openshift/origin/blob/6d8c2d04532e89ebfe1924baee1ca222beed7799/pkg/testsuites/standard_suites.go#L443 

Today, there are some monitoring tests that always run that may not make sense to run during disruptive test suites.
It may be necessary to disable specific monitoring tests that do not make sense here.
This must be done with caution and is a case-by-case consideration.
It is highly recommended that you consult with TRT on whether or not it makes sense to disable any of the monitoring tests when running your test suite.
This will need to be in your Prow job configuration like https://github.com/openshift/release/blob/5fdc35daff8145aa2f6b505cd4a3fdd6e81b4664/ci-operator/config/openshift/cluster-authentication-operator/openshift-cluster-authentication-operator-release-4.21__periodics.yaml#L59 .

When adding a new test suite, there will be no Prow jobs that will run your new test suite by default.
You must add and maintain these Prow jobs for your new test suite.
While it is encouraged to have a mixture of periodic and presubmit jobs, the bare minimum is that your tests must run at least 7 times per week.
This can be accomplished with a periodic job that runs once daily.
For feature promotion, it is expected that both `TechPreviewNoUpgrade` and `Default` variants of these jobs exist so that after promotion your feature continues to be tested.

Prow jobs are configured in https://github.com/openshift/release and documentation for creating new Prow jobs can be found at https://docs.ci.openshift.org/docs/ .

As an example of periodic jobs running a specific test suite for each supported platform, see https://github.com/openshift/release/blob/master/ci-operator/config/openshift/cluster-authentication-operator/openshift-cluster-authentication-operator-release-4.21__periodics.yaml

> [!NOTE]
>The above example uses regular expressions to run a subset of tests for each job due to a long setup and teardown cycle for each test spec.
>Avoid doing this unless you **must** shard your tests this way.

Some naming gotchas to consider are present in https://github.com/openshift/sippy/blob/main/pkg/variantregistry/ocp.go - you’ll want to avoid using any naming conventions here that result in the jobs being hidden from component readiness.

While you are still working on your tests, it makes it easiest to run your tests in a CI-like environment when you have an existing payload job you can run against your test implementation pull requests using the `/payload-job` Prow command.

It is highly recommended that you create your new test suite with a skeleton for your tests implemented that runs a singular trivial test that passes.
Without at least one test implemented for the test suite, rehearsals on your new Prow job will fail making it more difficult to get merged.

Once you’ve got a skeleton test suite merged into openshift/origin, create a new Prow periodic job for each supported platform that will run your new test suite.
You should set the interval value for each job to some arbitrarily high number (such as a month) so that it won’t be automatically triggered until you have fully implemented your tests.

When the new Prow jobs have merged, you will be able to run `/payload-job {jobNames…}` on your test implementation PRs to trigger runs of your test suite with the changes in your PR.

Ideally, your feature will have both presubmit and periodic tests.
Once your tests are implemented, ensure you also add presubmit jobs for components that are affected by your feature to catch potential regressions earlier in the development cycle.

For additional resources on creating Prow jobs to run your tests, see https://docs.ci.openshift.org/docs/

### Writing Your Tests

Once you’ve decided on the types of tests you’ll need for your feature, it is time to write them.

All new OpenShift component tests go in an appropriate directory under https://github.com/openshift/origin/tree/main/test/extended .
If an appropriate directory does not exist, create it.
When creating a new directory, this is a new Go package that needs to be “underscore” imported in the https://github.com/openshift/origin/blob/main/test/extended/include.go file.
This ensures that your tests are properly included in generated testing files.

All tests must be appropriately labeled.
This includes, but is not limited to:
- A label that denotes the Jira component regressions should be assigned to. This generally follows the pattern of `[Jira:”Component Name”]`
- If applicable, a test suite label. This is for denoting that tests should only be run as a part of a specific test suite and follows the pattern `[Suite: {suiteName}]`. If no suite label is present the test will be included in the standard conformance suite.
- If the test must run serially, the `[Serial]` label. If this label is not present, it will be assumed to be a test that can run in parallel with other tests.
- If the test runs slowly, the `[Slow]` label.
- If the test is disruptive, the `[Disruptive]` label. Disruptive tests are inherently serial tests as well.
- An OpenShift feature gate label. This follows the format of `[OCPFeatureGate:{FeatureGateName}]` where `FeatureGateName` is the name of your feature gate. This signals that the test should only be run against a cluster where the feature gate is enabled.

An example of using these labels can be seen in https://github.com/openshift/origin/blob/main/test/extended/authentication/oidc.go

Once you’ve written your tests and have a PR against openshift/origin, it is recommended that you run a payload job that will run your new tests in the appropriate CI environments to ensure they run as expected.

#### Considerations for tests that require use of a specific OCI image

For tests that require the use of a specific OCI image, there are some additional steps you must go through to ensure that the image is mirrored to an OpenShift e2e image registry.
This ensures that all of our tests are consistently repeatable and not prone to failures from external factors.
For disconnected tests, these images will be mirrored to a local image registry on the cluster-under-test.

To add an allowed image, follow the steps outlined in https://github.com/openshift/origin/blob/main/test/extended/util/image/README.md.

An example of doing this can be found in https://github.com/openshift/origin/pull/30221 .

#### Considerations for testing against disconnected clusters

When interacting with disconnected clusters within your tests, it is important to note that the only way to reach the cluster is through a proxy.

For most tests this should not be a problem as all programmatically created Kubernetes clients will automatically use the proxy.

For tests that require testing a different connection method, like hitting a route exposed by the disconnected cluster with a raw HTTP client, you must read and respect the `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables.

### Once Your Tests Merge
Inevitably once your tests merge and go live in a large number of jobs running hundreds of times a day, potentially in many configurations and platforms you may not have anticipated, unexpected problems can arise. This can surface in the main Component Readiness view for a release, which means it is implicitly a release blocker. It is best to monitor your new tests behavior in Sippy by visiting the main page and navigating to `Release > Tests > search for your test name(s)`. Use those test analysis pages to see how it’s performing globally in CI.

An example of a test analysis page can be found [here](https://sippy.dptools.openshift.org/sippy-ng/tests/4.21/analysis?test=%5Bsig-auth%5D%5BSuite%3Aopenshift%2Fauth%2Fexternal-oidc%5D%5BSerial%5D%5BSlow%5D%5BDisruptive%5D%20%5BOCPFeatureGate%3AExternalOIDCWithUIDAndExtraClaimMappings%5D%20external%20IdP%20is%20configured%20with%20invalid%20specified%20UID%20or%20Extra%20claim%20mappings%20should%20reject%20admission%20when%20UID%20claim%20expression%20is%20not%20compilable%20CEL&filters=%7B%22items%22%3A%5B%7B%22columnField%22%3A%22name%22%2C%22operatorValue%22%3A%22equals%22%2C%22value%22%3A%22%5Bsig-auth%5D%5BSuite%3Aopenshift%2Fauth%2Fexternal-oidc%5D%5BSerial%5D%5BSlow%5D%5BDisruptive%5D%20%5BOCPFeatureGate%3AExternalOIDCWithUIDAndExtraClaimMappings%5D%20external%20IdP%20is%20configured%20with%20invalid%20specified%20UID%20or%20Extra%20claim%20mappings%20should%20reject%20admission%20when%20UID%20claim%20expression%20is%20not%20compilable%20CEL%22%7D%2C%7B%22columnField%22%3A%22variants%22%2C%22not%22%3Atrue%2C%22operatorValue%22%3A%22contains%22%2C%22value%22%3A%22never-stable%22%7D%2C%7B%22columnField%22%3A%22variants%22%2C%22not%22%3Atrue%2C%22operatorValue%22%3A%22contains%22%2C%22value%22%3A%22aggregated%22%7D%5D%2C%22linkOperator%22%3A%22and%22%7D).
