---
title: openshift-validator
authors:
  - "@camilamacedo86"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-02-19
last-updated: 2021-03-10
status: implementable
see-also:
  - "/enhancements/olm/max-openshift-versions-for-operators.md"
  - new checks for OperatorHubValidator:https://github.com/operator-framework/enhancements/pull/65
---

# OpenShiftValidator

This proposal describes a new Validator to perform specific *linters* checks which are specifically to publish the operator bundle on the OpenShift Catalogue. The requirements are similar to the [OperatorHubValidator][operatorhubvalidator] implemented in [operator-framework/api][oper-api].

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently, [operator-framework/api][oper-api] centralizes the implementations of validator(s) & suite(s) of checks used by tools. For further information check [here](https://github.com/operator-framework/api/blob/master/pkg/validation/validation.go#L38-L46) the existing validators. Now see:

- `BundleValidator` checks if the operator bundle spec is according to spec. For further information see [Operator Bundle](https://github.com/operator-framework/operator-registry/blob/master/docs/design/operator-bundle.md) document.
- `OperatorHubValidator` checks if the operator is respecting the required criteria to be published in the [OperatorHub.io](https://operatorhub.io/).

The new `OpenShiftValidator` proposed here has the goal of perform checks which ensure the requirements to publish the operator bundle on the OpenShift Catalogue. The validator will raise issues or warnings accordingly. These checks will also be available to be used by the users via the tools such as SDK and consequently also called in the pipeline. Note, SDK provides the ability to optionally run validates using the `--select-optional` parameter to the bundle validate command.

We can utilize this feature allowing the `OpenShiftValidator` to be called:

```sh
$ operator-sdk bundle validate ./bundle --select-optional name=openshift`
```

The new validator will allow verifying the operator bundle configuration works for a specific OCP version where it is intend to be published:

```sh
$ operator-sdk bundle validate ./bundle \
    --select-optional name=openshift \
    --optional-values="ocp=4.8"
```

## Motivation

- Allow an operator developer or scheduled jobs a means to check required criteria for publishing an operator on OpenShift.
- Validate the operator bundle manifests and notify users and maintainers with warning and errors about what is not compliant to the OpenShift criteria
- Allow operator developers, CI jobs such as the pipeline/CVP, and admins of OLM catalogues audit and validate the bundles regarding the specific criteria for OpenShift versions, which are supported by the bundle or which the user is intend to distribute the operator.

### Goals

Perform static checks on the operator bundle manifests to avoid the scenarios where the operator bundle:

- is published but will not be installable
- is not prepared to work well on all OpenShift cluster versions and lacks any configuration to address its limitation
- is using deprecated or no longer supported K8S APIs
- is not following good practices and conventions

**NOTE** These motivations are similar to [Add new checks to the operatorhub validator][ep-operatorhub-checks], however, for OpenShift Catalog.

Also:
- [operator-framework/api][oper-api] with new checks in `OperatorHubValidator`
- SDK tool using [operator-framework/api][oper-api] and return errors or warnings regarding the new checks detailed in this proposal via the command `operator-sdk bundle validate ./bundle --select-optional name=operatorhub`
- SDK tool providing new argument `--optional-values` that allows pass a string map.
- Pipeline performing the new checks since it uses `operator-sdk bundle validate ./bundle --select-optional name=openshift`

### Non-Goals

- Add checks which are not specific to `OpenShift/OKD` and that could be part of `OperatorHubValidator`
- Provide a way for the users check specific criteria or perform custom checks

## Proposal

### User Stories

- As an operator developer, I'd like to use the SDK tool to validate my operator bundle before publishing on OpenShift Catalogue. Validating will allow me to quickly get its errors and warnings locally so that, I can provide the required fixes and improvements before trying to publish it.
- As an operator framework maintainer, I'd like to see CI checking the operator bundle criteria when a user pushes a Pull Request to add the operator bundle on the OpenShift so that, we can easily ensure a better quality of the projects before they are published.  
- As an operator developer or operator framework maintainer, I'd like to use the validator to check if my operator bundle will work on versions of OpenShift that I am intend to publish it to.
- As an operator framework maintainer, I'd like to see the `OpenshiftValidator` in [operator-framework/api][oper-api] so that, I can easily keep it maintained since the rules and criteria are centralized and then, I can ensure that all projects and tools are using the same. In this way, if something change I only need to update one project instead of many of them.

### Implementation Details/Notes/Constraints

#### OpenShift version supportability features

Before we describe the criteria for the validator we need to check some options available to specify the OpenShift and Kubernetes version(s) which the operator bundle will be supported by its configuration. Following a summary.

**Annotation maxOpenShiftVersion**

See [here](/enhancements/olm/max-openshift-versions-for-operators.md) a proposal for the operator bundles can use the following annotation:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  annotations:
    # Prevent upgrades to OpenShift Version 4.9
    operators.coreos.com/maxOpenShiftVersion: "4.8"
```

**OCP Labels in the image index**

By using the label `LABEL com.redhat.openshift.versions`, it is possible to define range criteria of OpenShift catalogue(s) where the operator bundle should be published, see:

```yaml
FROM scratch

LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=your-operator
LABEL operators.operatorframework.io.bundle.channels.v1=your-channel
LABEL operators.operatorframework.io.bundle.channel.default.v1=your-channel
LABEL com.redhat.openshift.versions: v4.5-v4.7

COPY manifests /manifests/
COPY metadata /metadata/
```

The versioning scheme available in the above OpenShift versions label accepts:

- Min version (e.g `v4.5`)
- Range (e.g `v4.5-v4.7`)
- A specific version (e.g `=v4.6`)

**Examples**

| Label | Description |
| ------ | ----- |
| `LABEL com.redhat.openshift.versions:v4.5` | It means valid for OCP `4.5` and later versions |
| `LABEL com.redhat.openshift.versions:=v4.5` | Valid **ONLY** for `4.5`
| `LABEL com.redhat.openshift.versions:v4.5-v4.7` | Valid **ONLY** for `4.5`, `4.6` and `4.7` |

However, note that nothing prevents a customer from taking a `4.6` catalog and installing it on a `4.8` ocp cluster (they may or may not actually be able to install the operators that exist in that catalog, of course).

**CSV spec for minimal k8s version**

The spec `minKubeVersion` describe what is the minimal version of Kubernetes that is supported by the operator bundle, see:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
...
spec:
  ... 
  minKubeVersion: 1.11.0
```

The above features can be used alone and/or combined.

#### Criteria valid which will not checked against to a specific OCP version

The motivation for the following check is to ensure that operators authors understand that when the `maxOpenShiftVersion` and `minKubeVersion` is not defined it means allow to distribute the operator bundle for any OpenShift Catalog version available and then, tha the operator projects has no limitations regarding the supported OCP/Kubernetes versions which probably is not the common scenario.

| Type | Description |
| ----- | ----- |
|warning|Check if we can found the `maxOpenShiftVersion` annotation or `minKubeVersion` spec in CSV. Then, if none of this options be filled raise a warning such as; `Your operator bundle is not specifying the OpenShift or Kubernetes version(s) which are supported by it. Ensure that it is installable in all possible provided options or configure it accordingly.`.

The motivation for the following checks is only to ensure that the syntax informed is valid and avoid issues usually caused by a typo mistake.

| Type | Description |
|  ----- | ----- |
| error| Check if `maxOpenShiftVersion` in CSV was informed then, if yes ensure that is valid. Return the message with the error(s) found.
| error| Check if the label `com.redhat.openshift.versions` is used in the index image, if yes then ensure that is a valid range/syntax. Return the message with the error(s) found.

The motivation for the following check is to ensure that versions informed are actually compatible which will not let the operator face problems to be installed or to be available for the desired criteria.

| Type | Description |
|  ----- | ----- |
| error | Check if `maxOpenShiftVersion` and `minKubeVersion` in CSV as the `com.redhat.openshift.versions` in the index image are informed then, check if them are a valid combination. Return the error accordingly inform what option combination is not valid as its values. (E.g If the `maxOpenShiftVersion` is = `4.5` and the label range is `LABEL com.redhat.openshift.versions:v4.5-v4.7` it is invalid or if `minKubeVersion` == `1.22` and the `maxOpenShiftVersion` is == `4.5` then it is invalid.)

#### Criteria valid which will be checked against to a specific OCP version

For the checks which should be done accordingly for some specific version(s) it will use the OCP version which has been provided to the validator via flag or via the bundle operator configuration. For example, if we run:

```sh
 operator-sdk bundle validate ./bundle \
     --select-optional name=openshift \
     --optional-values="ocp=4.8"
```

Then, it means that the check will validate the operator bundle configuration accordingly to the OCP version informed where it is intend to be published to. In this case, the validator will consider the OpenShift version `=4.8`.

If the `--optional-values="ocp=4.8"` not be used then, the validator will check the operator bundle configuration and do the checks considering the OCP version(s) which are supported by it. So, the validator will look for the values in the `maxOpenShiftVersion` and the `minKubeVersion` which are in the CSV and in the `com.redhat.openshift.versions` which is found in the index image.

- To check if the operator bundle is using a CRD API version which is deprecated or not supported we will need to see if the CRD manifests in the bundle are using `apiVersion: apiextensions.k8s.io/v1beta1`. (See [here](https://github.com/operator-framework/operator-sdk/blob/v1.4.2/testdata/go/v2/memcached-operator/bundle/manifests/cache.example.com_memcacheds.yaml#L1)). Note that an operator bundle can have Many CRD(s).

- To check if the operator bundle is using the Webhook API version which is deprecated we will need to see if the CSV has the spec `webhookdefinitions.<*>.v1beta1`. (e.g See [here](https://github.com/operator-framework/operator-sdk/blob/v1.4.2/testdata/go/v2/memcached-operator/bundle/manifests/memcached-operator.clusterserviceversion.yaml#L203-L205). Note that an operator bundle can have Many Webhook(s) and with more than one type and this field is a list of.

Note that the output message should clarify and informed when the API was deprecate and when was or will be removed and what version should be used instead. Also, the respective OCP version should also be provided in the message.

The error raised will be accordingly with the OCP version that we have, see:

| Conditional |Type |
| ------ | ----- |
| operator bundle supports OpenShift version which is using k8s => `1.22` and do not uses `apiextensions/v1` ÒR has [webhookdefinitions](https://github.com/operator-framework/operator-sdk/blob/v1.4.2/testdata/go/v2/memcached-operator/bundle/manifests/memcached-operator.clusterserviceversion.yaml#L203-L205) and do not have the API `v1` listed | error |
| operator bundle supports OpenShift version which is using k8s `=> 1.16 <= 1.22` and uses `apiextensions/v1beta1` ÒR `admissionregistration.k8s.io/v1beta1` | warning |

**Why we need to check if the api v1 is not used instead of only check if it has any CRD or Webhook with the deprecated `v1beta1`?**
		
The operator author might provide a solution that works for both scenarios which means that will work when installed in the previous version of `Kubernetes < 1.16` and also in the upper versions `=> 1.22`. However, for the operator works well in any Kubernetes version `>=1.22`, we know that it is mandatory it has the latest APIs versions `v1`. See [here](https://github.com/kubernetes-sigs/kubebuilder/blob/v3.0.0-beta.0/test/e2e/v3/plugin_cluster_test.go#L99-L101), for example, that in the e2e test for Kubebuilder, we check what is the cluster version to apply and test the API versions accordingly. And then, note that we can find both APIs in the webhooks fields of the CSV as well such as:

```yaml
  webhookdefinitions:
  - admissionReviewVersions:
    - v1beta1
    - v1   
```

**IMPORTANT**

> Kubernetes version `1.22` was not released until now which means that the deprecated APIs still supported for all clusters. Also, at the same way none OpenShift version which is using Kubernetes API `1.22` was released. Then, we cannot raise errors until it happens for the scenarios where we the `maxOpenShiftVersion` and the `minKubeVersion` which are in the CSV and in the `com.redhat.openshift.versions` which is found in the index image are not provided because we are assuming all possible versions.
> However, for the checks where the `--optional-values="ocp=4.X"` which will be using Kubernetes API `1.22` was used we know that the goal is to verify if the operator bundle will work òn `1.22` then, we can already raise the `error` result.

Then, it means that:
- `--optional-values="ocp=4.X"` flag with a value of an OpenShift version using K8S `=> 1.16 <= 1.22` the validator will return result as warning.
- `--optional-values="ocp=4.X"` flag with a value OpenShift version using K8S `=> 1.22` the validator will return result as error.
- `minKubeVersion >= 1.22` return the error result.
- `maxOpenShiftVersion >= OCP version with 1.22` return the error result.
- `com.redhat.openshift.versions` with a range which has an OCP version with 1.22` return the error result.

**Currently (OCP versions released with the Kubernetes `<1.22`)**
- `minKubeVersion` and `maxOpenShiftVersion` and `com.redhat.openshift.versions` empty/nil return the warning result.

**After OCP version using the `1.22` Kubernetes release**
- `minKubeVersion` and `maxOpenShiftVersion` and `com.redhat.openshift.versions` empty/nil return the error result.

In a scenario where none version has been provided and the operator bundle has not been using the provided features to define the OpenShift/Kubernetes versions which are supported by it then, it means that the bundle operator should work well in all available possible versions, indeed `>= 1.22` after its release roll out of course. The only check above which will return an error is that one which ensure that the APIs used are supported, however, for this scenario the validator should return **warning** instead.

Note that the OpenshiftValidator needs to be able to receive a string map and check if it contains any valid key for the specific check to address this requirement.

In the code implementation, it should be only one check which will raise error(s) and warning(s) as a respective message according to the OCP/K8S version used. See that the same checks with its specific outputs has been proposed to the `OperatorHubValidator` in the EP [Add new checks to the operatorhub validator][ep-operatorhub-checks]. Therefore, the code could be done without duplicate the rules and instead of that, used for both validators where only type and message changes if possible.

### Risks and Mitigations

- Same risks and/or mitigations described in [Add new checks to the operatorhub validator][ep-operatorhub-checks]

## Design Details

### Open Question

1. Will SDK be affected by this solution?

>The proposal describes the new Validator ideally implemented in [operator-framework/api][oper-api] which means that SDK would only need to provide its usage. However, see the [Alternative][#alternative] section. Some options could result in a high effort for SDK.

2. Ideally we should **not** like to address OpenShift specific features in OLM repository or [operator-framework/api][oper-api] because its shows "hurt" the community concepts then, it raises some extra questions:

a) How will we support OKD? Should not all OpenShift features and facilities are available for OKD as well or not? Will the OLM downstream version to be shipped with OKD? What version of OLM OKD uses currently?

>Currently, OLM for upstream is equals for downstream. Then, shows that idea here is OKD use the downstream version:
>OLM downstream targets OKD/OCP
>OLM upstream targets K8S
>And then, it is possible in the future we have features which are only available to OLM downstream and specifically only works with OCP/OKD.

b) Will the [operator-framework/api][oper-api] be also provided in a downstream repo? How will be the process to release the downstream version?

> Probably yes. However, we have no ETA for that. Not the ideal solution, but since this EP speaks about we create a new Validator shows that would be easy decouple that and move for another place if required. However, the big concern here shows create a precedence in the API to allow specific rules and implementations for OpenShift.

3. How much value allow users develop their own validators which only can be used by SDK feature bundle validation can bring to project and OF? Have we some example scenario(s) or requirements that would justify this effort?

>The best ideal solution proposed so far is `(Option C)Design and implement Pluggable Validator mechanism for SDK` if we think about address all possible concerns and bring flexibility. If the feature does not get provided, then be sure it will never be used. However, knowing that; the biggest part of the operator projects in the OCP catalogue are not indeed using the latest SDK, which means not using the SDK helpers to build the bundle then, all lead us to conclude that it does not show a high priority to address at the moment.
>Also, SDK provides via Scorecard a way for operators authors write specific checks which could address their needs. Note that we are looking here for common validations criteria that are valid to publish the operator in the options provided by OLM's default, which are;  OpenShift Catalog and/or to OperatorHub.io via [Add new checks to the operatorhub validator][ep-operatorhub-checks] so still valid also fit both in the API.

4. Have SDK any concern about show the specific validator for OpenShift as one option of the bundle validation feature? If yes, is SDK also planning have an [operator-lib](https://github.com/operator-framework/operator-lib) API specific for downstream to address the OpenShift specific helpers and needs that we know about (e.g see [here](https://github.com/operator-framework/operator-sdk/issues/2770))?

> For the same reasons, SDK would not be ideal add specific OpenShift requirements in SDK. However, if the items can be used by different vendors or services, or indeed be "optional" then shows that could be acceptable.

5. Knowing that:
- we have not specific implementations in the APIs raised above and provide in SDK or requirements/use cases currently that can let us confirm that it will grow a lot to starts to cause problems or any educated guess to know how long time it could take.
- if we decide to have a downstream repo for [operator-framework/api][oper-api] and need to keep maintained the feature in [ocp-release-operator-sdk] only.

a) How much extra effort will need to be spend to do the downstream releases and keep these repositories/solutions maintained? Is this effort justifiable by the concerns over use the upstream libs currently?

b) The mainly general concerns addressed via the Validators, *linter* checks, will be available via the new checks provided to OperatorHub when the EP [operator-framework/api: add new checks to the operatorhub validator](github.com/operator-framework/enhancements/pull/65) be implemented. In this way, besides this idea be very interesting for the [audit][audit-ep] idea, CVP/Pipeline, and operator authors which provide solutions for OCP Catalog, if we reach a consensus here to we would need to persuade an alternative solution which brings high effort to get this new Validator implemented and/or maintained then; how much value X effort will this proposal bring? Would still valid we try to address these requirements?

>This shows not exactly about the specific checks but more about the longer term version of the validation framework that allows these kinds of contributions.

## Drawbacks

We might use this scenario as use case to know how we will address this kind of requirement only for downstream in the future.

However, if we check that the effort/cost to address this needs in the possible agreed design will be too high which not justify its value at the moment then, we might decide to not move forward with now.

### Test Plan

The rules implemented should be covered by unit tests.

## Alternatives

### (Option A) Include the OpenShift Validator in upstream

The proposal above was defined considering the new Validator be implemented in [operator-framework/api][oper-api] and used by SDK upstream implementation.

**Pros**

- Lower effort/cost since it would be similar to the others as it usage
- Allow the Validator be used with OKD
- The Validator can be used by any solution
- Does not requires the usage/installation of a downstream SDK binary

**NOTE** SDK users and Pipeline uses the upstream binary and shows. However, SDK also provides a binary from downstream.

- Provide specific checks for OpenShift by default to help users from upstream and would like to publish their solutions on OperatorHub and OpenShift.
- Users will still get the latest releases and the bug fixes faster and will not be affected by any delay that might be faced to get the solutions from downstream.
- Provide and promote the helper to the users since they can check that it is available by using the `--list-optional` flag:

```sh
$ operator-sdk bundle validate --list-optional
NAME           LABELS                     DESCRIPTION
operatorhub    suite=operatorframework    OperatorHub.io metadata validation
               name=operatorhub          
```

**Cons**

- Provide specific OpenShift solutions in upstream might hurt the community
- OLM team would not like to add a code which is valid only for OpenShift unless we provide others vendor's validators as well in the public API. They are trying decouple the rules which are specific for OCP from OLM.
- In the future it might result an effort required to move any OpenShift rules for downstream repos and implementations which are only valid for it. Currently, it would be the only case scenario so far, however, the precedence can bring others ones.

### (Option B) Include the OpenShift Validator in downstream component for [operator-framework/api][oper-api] and use it in SDK upstream repository

**Pros**

- Mainly all pros of option A
- Solves the OLM team concern about add OpenShift checks in [operator-framework/api][oper-api]
  
**Cons**

- Effort required to keep a downstream repository for a downstream component and its releases [operator-framework/api][oper-api]
- SDK would need to probably use only the downstream import to avoid misleading
- It could cause delays for the SDK be able to apply the required changes of [operator-framework/api][oper-api] which can result ,for example, in an impact for Pipeline and users to get bug fixes or improvements.

### (Option C) Design and implement Pluggable Validator mechanism for SDK

Instead of adding the OpenshiftValidator to [operator-framework/api][oper-api], we would provide it via the custom "downstream plugin" and make it available only for SDK binary by:

- Implement an interface for the `operator-sdk bundle validate` command
- Then, we could have plugins that would respect this interface and be recognized by the SDK command
- The plugins would be such as go modules which would be downloaded in a directory. SDK command would be able to recognize and use the plugin.
- The plugin would live in the [sdk downstream repository][ocp-release-operator-sdk] or in a repository with specific solutions for OpenShift such as `http://github.com/operator-framework/openshift-plugins`

**Pros**

- Address an RFE that allow others to create their validators and use them with SDK.
- Does not affect the users and Pipeline since they will be still able to use the upstream binary and the downstream validator module
- Address the OLM team concerns about adding the validator in the upstream api. Also, address similar concerns, if they exist, for SDK CLI upstream.

**Cons**

- The Validator cannot be used outside of SDK. So, for example, it cannot be used by [audit proposal][audit-ep] if we decide not to make this solution leverage in SDK CLI (binary).
- Brings higher effort to implement since it requires the design and implementation of a new feature for SDK which we might have none other use cases/request which justify it.
- It does not allow the validator's re-usability via importing the code implementation to the other projects as option A or B.

**NOTE** If we decide to move forward with custom implementations directly in [ocp-release-operator-sdk][ocp-release-operator-sdk], it might start to be harder to keep maintained.

### (Option D) Implement the OpenShift validator via Scorecard

This solution would be [Writing Custom Scorecard Tests](https://sdk.operatorframework.io/docs/advanced-topics/scorecard/custom-tests/) and then, make they public for the common usage.

**Pros**

- Low effort compared to add the RFE to allow SDK works with any Validator provided respecting an interface
- Same other pros of the option C

**Cons**

- Same cons of the option C
- Requires a Kubernetes cluster running to be executed.

### (Option E) Api for dynamic validator using CUE

The idea of this solution would be an API that allows work with dynamic validator using CUE, which the tools could use. The tools would still need to use the API and recognize and allow users to use the custom validators to write with it. It means that each tool/command would require an interface to be implemented, such as described in `Option C` above. So, it would mean:

- Move forward with the proposal [Custom operator validation #43](https://github.com/operator-framework/enhancements/pull/43)
- Implement an interface for SDK tool `operator-sdk bundle validate` to recognize go module plugins using the CLU API. Otherwise, this option cannot be an offer and used via, for example;`$ operator-sdk bundle validate --list-optional`
- Then, we could have plugins that would respect this interface and be recognized by the SDK command
- The plugins would be such as go modules which would be downloaded in a directory. SDK command would be able to recognize and use the plugin.
- The plugin would live in the [sdk downstream repository][ocp-release-operator-sdk] or a repository with specific solutions for OpenShift such as `http://github.com/operator-framework/openshift-plugins`

Note that any tool also could have a "plugin" interface and use the CLU API suggested in [Custom operator validation #43](https://github.com/operator-framework/enhancements/pull/43).

**Pros**
- Address an RFE that allow others to create their validators and use them with SDK or other tools/solutions.
- Does not affect the users and Pipeline since they will be still able to use the upstream binary and the downstream validator module
- Address the OLM team concerns about adding the validator in the upstream api. Also, address similar concerns, if they exist, for SDK CLI upstream.

**Cons**

- For SDK offer it then, the "clu plugin validator" interface still required to be implemented for it
Any solution that wants to use this solution would need to use the API and define an implementation for the validators to be recognized and used by the specific solution, which means an effort must be spent for each case that cannot be centralized
- Brings higher effort to allows implementing this solution without use cases enough, which might justify it.

[oper-api]: https://github.com/operator-framework/api
[ocp-release-operator-sdk]: https://github.com/openshift/ocp-release-operator-sdk
[audit-ep]: https://github.com/operator-framework/enhancements/pull/66
[ep-operatorhub-checks]: https://github.com/operator-framework/enhancements/pull/65
[operator-lib]: https://github.com/operator-framework/operator-lib
