---
title: pao-as-part-of-core-ocp
authors:
  - "@MarSik"
  - @yanirq
reviewers:
  - @fromanirh
  - @jmencak
  - @mrunalp
  - @dhellmann
  - @sttts
approvers:
  - @dhellmann
creation-date: 2021-08-12
last-updated: 2021-08-12
status: provisional
---

# PAO move into NTO to become part of core OCP

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The [Performance Addon Operator](https://github.com/openshift-kni/performance-addon-operators) is a component that makes it easier to configure an OCP cluster for low latency and real-time purposes [OCP docs](https://docs.openshift.com/container-platform/4.8/scalability_and_performance/cnf-performance-addon-operator-for-low-latency-nodes.html).
It is a high level orchestrator that consumes a [Performance Profile](https://github.com/openshift-kni/performance-addon-operators/blob/master/docs/interactions/performance-profile.yaml) and generates multiple manifests that are then processed by core OpenShift components like MCO and NTO.
We have a simplified interaction diagram here: https://github.com/openshift-kni/performance-addon-operators/blob/master/docs/interactions/diagram.png
A more in depth description of how a low latency tuned cluster works was presented at DevConf 2021: https://devconfcz2021.sched.com/event/gmJD/openshift-for-low-latency-and-real-time-workloads

The proposal in hand is to move the existing implementation of PAO under
[cluster node tuning operator (NTO)](https://github.com/openshift/cluster-node-tuning-operator) without adding new features.

## Motivation

The Telco team maintains its own build and release pipelines and releases via a [Telco specific errata](https://red.ht/3boqjeS) that however targets the OpenShift product. So from a customer perspective PAO is part of OCP.

- More and more customers are using PAO to configure their clusters for low latency or are being told to do so. The Telco team has so far managed to release PAO on the same day as OCP to maintain the illusion it is part of the product, but it costs a LOT of time to maintain the downstream build process.

- The OCP [gcp-rt](https://prow.ci.openshift.org/?type=periodic&job=*gcp-rt*) lane tests OCP deployment on top of GCP with the real-time kernel enabled, but it only works [thanks to some pieces of PAO tuning](https://github.com/openshift/release/pull/18870). However the tuning is a pre-rendered snippet, not linked to the operator, because the deployment does not know about PAO.

- PAO is closely tied to some OCP components
([Cluster Node Tuning Operator](https://github.com/openshift/cluster-node-tuning-operator))
and depends on the OCP version. So we release PAO for every OCP minor version (since 4.4) and together with some of the OCP Z-streams.
The upgrade procedure is tricky though, first OCP needs to be upgraded and then PAO via an OLM channel change.
This is needed to make sure the necessary functionality is present in the running NTO.
Some tuning changes and bug fixes in NTO/tuned as well as in RHCOS kernel are critical for the advertised functionality.

PAO does not depend on anything version specific with worker nodes.
NTO takes care of the few pieces that are needed with tuning and MCO installs the proper RT kernel.
PAO is tightly coupled to NTO/tuned version which is more tight than just OCP minor versions.
Some tuning changes and bug fixes in NTO/tuned as well as in RHCOS kernel are critical for the advertised functionality.

- The new [Workload partitioning](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md)
feature seeks to use PAO as the configuration tool.
However Workload partitioning needs to be enabled at install phase and PAO CRDs are not yet available at that time.
[A "hacky" procedure](https://github.com/openshift/enhancements/pull/753) is needed to overcome this.

- Pre-GA availability and testing is tricky in the current way of things. The 4.9 prod OLM index (for example) will not be populated until the operator is shipped.
At a GA date, all OCP operators are available earlier. PAO has its own separate quay.io bundle/index, but the separation is complicated for testing.

### Goals

We (the telco team) would like to make PAO part of OpenShift. There are multiple paths we can take to achieve this, I am going to describe the one we prefer: **PAO becoming a sub-controller in NTO.**

The initial plan is to keep all logic and custom resources intact, we want to start with simply moving the code over.

### Non-Goals

Logic changes or tighter integration with other MCO controllers are not expected to be part of this initial move. Just the delivery mechanism wrt OCP.

## Proposal

To be really clear here, the Telco team will handle all the necessary changes to both codebases, however we will need support in the form of PR reviews and design consultations.

### Existing Implementation

PAO is being developed under the OpenShift KNI organization (https://github.com/openshift-kni/performance-addon-operators), built by custom builder upstream and cPaaS downstream. PAO is installed via OLM as a second-day operator.

The current deliverables are:

- [performance-addon-operator](https://catalog.redhat.com/software/containers/openshift4/performance-addon-rhel8-operator/5e1f480fbed8bd66f81cbe23?container-tabs=overview&gti-tabs=registry-tokens)

  The operator contains the high level orchestration logic + a [tool to simplify generation of Performance Profiles](https://docs.openshift.com/container-platform/4.8/scalability_and_performance/cnf-create-performance-profiles.html). This tool can be executed locally on admin's laptop using `podman run --entrypoint performance-profile-creator performance-addon-operator:ver`

- [performance-addon-must-gather](https://catalog.redhat.com/software/containers/openshift4/performance-addon-operator-must-gather-rhel8/5ee0bfa4d70cc56cea83827f?container-tabs=overview&gti-tabs=registry-tokens)

  The must gather image [enhances](https://github.com/openshift-kni/performance-addon-operators/tree/master/must-gather) the main OCP one and [collects](https://docs.openshift.com/container-platform/4.8/scalability_and_performance/cnf-performance-addon-operator-for-low-latency-nodes.html#cnf-about-gathering-data_cnf-master) some latency tuning configuration and data that are needed for debugging.

- functional tests are also vendored into
[cnf-tests](https://catalog.redhat.com/software/containers/openshift4/cnf-tests-rhel8/5e9d81f65a134668769d560b?container-tabs=overview&gti-tabs=registry-tokens)
that can be used to
[validate customer deployments](https://docs.openshift.com/container-platform/4.8/scalability_and_performance/cnf-performance-addon-operator-for-low-latency-nodes.html#cnf-performing-end-to-end-tests-for-platform-verification_cnf-master) (this includes other aspects like SCTP, PTP, SR-IOV as well).

### Code

PAO code would be moved into the NTO repository as a separate controller. The Telco team would keep ownership as it has the necessary domain knowledge needed to implement the low latency tuning.

The code currently uses Operator SDK libraries in some of the tests and this might need to be changed.
Migrating the use of these libraries to [library-go](https://github.com/openshift/library-go) or stripping them to [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
would be options in hand.

PAO also implements a [conversion webhook](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#webhook-conversion)
The hook is currently installed via OLM mechanisms and this might need to be reimplemented or changed to use whatever CVO or ART support.
PAO must-gather will have the native support with additional arguments and will not require an additional image as currently exists in PAO original repository.

[cnf-tests](https://github.com/openshift-kni/cnf-features-deploy/) are NOT part of this move either, the Telco team will keep maintaining the validation suite in OpenShift KNI.


### CI

PAO contains both unit and functional tests implemented using Ginkgo.

Integration with NTO is currently TBD, please advise if anything special is needed.

One of the test suites (4_latency) implements an opinionated latency testing suite to get comparable results from customers and to prevent configuration / command line invocation mistakes. Keeping it in PAO/NTO or moving it into cnf-tests is TBD.

### Build pipeline

PAO would inherit the NTO pipeline and all the images will be released via ART.

### Releases

PAO should be shipped as part of core OCP after NTO will contain the migrated code of PAO.
PAO already obeys the OCP github PR rules and uses the same PR bot so nothing changes in the lifecycle.

### API stability

The `PerformanceProfile` CR uses an `performance.openshift.io/v2` API group which marks it as stable interface that needs to be maintained for about a year after official deprecation. We do not expect any change to the API as part of this proposal.

### Bugs

PAO bugs are currently tracked in The OpenShift Bugzilla component [Performance Addon Operator](https://bugzilla.redhat.com/buglist.cgi?component=Performance%20Addon%20Operator&list_id=12053428&product=OpenShift%20Container%20Platform)

This should be kept as-is, the Telco team will own all the bugs. We might need to move the component under NTO or rename it.

The bugzilla component is currently not part of OCP metrics and we can include it. TBD


### User Stories

Both the NTO and PAO functionality should be kept intact and no regressions are allowed. The user should not notice anything except the fact that OLM is no longer needed to enable the tuning functionality.

### API Extensions
The existing PAO API Extensions will be moved under the NTO repository.

### Implementation Details/Notes/Constraints

- Existing api compatibility practices:
PAO API has been treated as Tier 1 for multiple releases already (since 4.4).

- Enumerate list of performance profile owned objects:
MachineConfig, KubeletConfiguration, Tuned (see [diagram](https://github.com/openshift-kni/performance-addon-operators/blob/master/docs/interactions/diagram.png)).
All activity happening on the cluster or nodes is indirect via other components (MCO, NTO) reacting to changes in objects owned by PAO.

- Enumerate list of webhooks:
Validation and conversion webhook (TBD)

- Updates to openshift conformance test suite if included by default:
All basic checks are doable on all linux platforms except when real-time kernel is needed. That case used to be only supported on GCP (and bare metal).
Measuring real latency requires bare metal, but is not needed for conformance.


### Risks and Mitigations

- Since the performance profile API will be visible for users on all clusters there is a risk that a cluster admin might tune nodes improperly.
The risk is minor since addressing the performance profile means the admin should find this specific API and deliberately perform tunings on the cluster using it.
The same risk would be relevant for applying changes for machine and kubelet configurations that already exist in OCP clusters.
This will mitigated by clearly documenting that admins should proceed with caution.

## Design Details

### Open Questions

- CI: Integration with NTO is currently TBD.

- CI: One of the test suites (4_latency) implements an opinionated latency testing suite to get comparable results from customers and to prevent configuration / command line invocation mistakes. Keeping it in PAO/NTO or moving it into cnf-tests is TBD.

- Maintainability: There are two options:
  - Maintaining a separate copy of the code in the NTO repository.
  - Vendoring the current PAO code as a "library" until we can stop supporting the standalone mode (4.6+).
  From OCP perspective it makes little difference as it would just be part of NTO release process indeed.

- API definitions: They would need to go into openshift/api but at this moment NTO include its own API definitions under its own repository.
The NTO API definitions transition into openshift/api is TBD and should be considered to be done at the time of PAO/NTO merge.

- List of pertinent topology options (control plane and infrastructure) where this operator is pertinent:
The assumption is that the addition of PAO to NTO will not add additional requirements for installation at all when control plane topology is "external"
and the infrastructure topology is not pertinent to the function.
The resources leveraged by PAO, now introduced into NTO, are all managed externally when in this mode (e.g. node configuration, kubelet configuration).
PAO control plane component modifies the worker nodes (unless in single node mode where it affects masters).
NTO currently supports only one cloud installation with external control plane topology (IBM Cloud) with a dedicated manifest.

### Upgrade

PAO has been installed via OLM since 4.4. Once it moves to core we will have to figure out the upgrade procedure, because OLM will not remove the optional operator by itself.

Implementation options:
- Remove the relevant OLM artifacts (CSV,Subscription,Operator group) via the performance-addon controller and leave the PAO CRDs without the change.
- In case leader election supports such conditioning, is to keep using the same leadership election method and make sure the newest version always wins. The OLM operator will then stay on the cluster (can be release noted) as an inactive component.

### Test Plan

All tests already exist in OCP CI, the trigger will move to the NTO repository.

Continuous testing is done via [OpenShift CI jobs](https://github.com/openshift/release/tree/daaab71832999c072c60667955637fa3a535d4ba/ci-operator/config/openshift-kni/performance-addon-operators) as well as some downstream specific Jenkins instances (both virtual and bare metal).

### Graduation Criteria

The user facing feature is already considered stable and GA, just shipped as an optional operator.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

It is expected that the image will be kept up-to-date as part of regular OCP flows.

API changes are handled by a conversion webhook that PAO already contains to make upgrades transparent to users.

PAO depends on NTO and MCO functionality.

Downgrades should be handled the same way the cluster node tuning operator was handled for minor versions up until now.

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

PAO is an infrastructure orchestrator so the latest available version has to win. It generates and maintains objects for other controllers (MCO, NTO) that then apply them to nodes.

### Version Skew Strategy

NTO behavior will be kept so no addition strategy aspects on NTO counter parts.
When PAO code base will be moved and used inside NTO it will be version aligned with CRI-O and K8s at the time of release.
CI should cover the refactored component behavior with its counter parts.

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

The proposed change adds more code to the NTO component increasing the probability of bugs and possibly a higher resource usage.

However using Performance Profile is optional and so users that do not use this way of tuning should be not affected at all.

## Alternatives

### Alternative upgrade Implementation
Have the PAO copy in NTO disable itself when the standalone version is installed, updating the ClusterOperator resource with that reason.
Then the user can remove the old standalone version from their cluster.
This would be easier to implement reliably, since we would only need to look for the subscription or other indicator that there is a standalone copy installed.
We also would not need to deliver any changes in the standalone package, so it would not matter which version is installed in the cluster at the start of the upgrade

### PAO as a full OCP operator

The alternative approach is to move PAO into OCP as a fully standalone operator. This would have no impact on the MCO team and would be simpler in terms of code.

However there are disadvantages to this approach:

- OCP is already heavy on infrastructure resource consumption and an additional operator would make that worse
- implementing a conditional start is possible though, similar to what machine-operator uses, but negates the simpler on coding advantage

### PAO as a sub-controller of Machine Config Operator

PAO could also live inside MCO if NTO cannot accept it. The long term future might be to merge all of them together anyway. All other aspects of this design are the same for this approach.
Since a new MCO design is under consideration, it make more sense to go with NTO for now.
## Future

PAO currently uses Ignite heavily and since MCO is [looking at dropping the support](https://hackmd.io/UzgrWuCLQ-u9bMh_JhRKOA#Proposal), it would make it easier to adapt to this change if PAO could directly contribute to the layered Machine Image instead of describing the necessary config changes as an MachineConfig first.

## Infrastructure Needed [optional]

This is just a move of an existing project and all the infrastructure is already in use.