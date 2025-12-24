---
title: accepted-risks
authors:
  - "@hongkailiu"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@wking"
  - "@JoelSpeed"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@PratikMahajan"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-06-11
last-updated: 2025-12-22
tracking-link:
  - https://issues.redhat.com/browse/OTA-1544
see-also:
  - "/enhancements/update/targeted-update-edge-blocking.md"
---

# accepted-risks

## Summary

A cluster admin can express accepted risks for a cluster so that when [a conditional update](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/updating_clusters/understanding-openshift-updates-1#conditional-updates-overview_understanding-update-channels-releases) of cluster is requested, it can be accepted only if the risks exposed to the conditional update are all accepted.

## Motivation

It is to reduce the time and effort for cluster administrators who manage many clusters, enabling them to pre-approve certain risks that they deem acceptable across some or all of their environments. Although the evaluation of risks cannot be avoided, the evaluation result may be reused for other clusters.

### User Stories

The enhancement is useful, e.g., in the following scenario:

- A cluster admin gets conditional updates of a cluster and evaluates the risks exposed to the conditional updates.
- After evaluation, the admin decides that some risks can be accepted on all or some of the managed clusters.
- The administrator configures relevant clusters to allow for updates, as long as any identified risks have all been accepted.

### Goals

- Reduce the overall effort of managing a great number of clusters and evaluating risks from a conditional update.

### Non-Goals

- It does not change which impact a specific update risk may bring to a cluster and how it is evaluated by a cluster administrator.

## Proposal

This enhancement proposes a way of specifying accepted risks regarding updating an OpenShift cluster.

### Workflow Description

The workflow and its implementation are based on [the API extension](#api-extensions) for the following scenario as the starting point: The cluster administrator decides that those risks `DualStackNeedsController`, `OldBootImagesPodmanMissingAuthFlag`, and `LeakedMachineConfigBlocksMCO` are acceptable and wants to tell the cluster version operator to accept an update if all its associated risks are among those three (otherwise block it).

The update to `4.18.16` is not recommended because of the risks `DualStackNeedsController` `OldBootImagesPodmanMissingAuthFlag`, and `RHELKernelHighLoadIOWait` while `4.18.17` contains the fixes of `DualStackNeedsController` and `RHELKernelHighLoadIOWait`. The only remained risk of `4.18.17` is `OldBootImagesPodmanMissingAuthFlag`.


#### Workflow

Provide a new (sub-)command `accept` of `oc adm upgrade`, e.g.,

> oc adm upgrade accept DualStackNeedsController,LeakedMachineConfigBlocksMCO,OldBootImagesPodmanMissingAuthFlag

 whose only result is to append the provided values to the field `.spec.desiredUpdate.accept` on the `clusterversion/version` manifest:

```console
$ oc clusterversion version -o yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: candidate-4.18
  clusterID: 1c182977-5663-428d-92a3-3d2bdf3fffb6
  desiredUpdate:
    acceptRisks:
    - name: DualStackNeedsController
    - name: LeakedMachineConfigBlocksMCO
    - name: OldBootImagesPodmanMissingAuthFlag
    force: false
    version: 4.18.15
```

Currently each item in `cv.spec.desiredUpdate.acceptRisks` has only one field `name` but it may have more such as `reason` in the future if needed.

Then the upgrade to `4.18.16` by the command `oc adm upgrade --to 4.18.16` will be blocked as only the first two of them are accepted by the administrator and to `4.18.17` by `oc adm upgrade --to 4.18.17` will be accepted.

The risk name like `OldBootImagesPodmanMissingAuthFlag` is unique cross all releases and thus the growing accepted risks over time should not cause any problems. For any reason, if it is desired to clear them out, we may run the following command:

> oc adm upgrade accept --clear

and

> oc adm upgrade accept -Foo

can be used to remove `Foo` from the accepted risks which is no-op if `Foo` is currently not in `cv.spec.desiredUpdate.acceptRisks`.

No arguments (either in form of `Foo` or `-Foo`) are provided to the command lead to an error.

The `--replace` in the following following command is to replace the current accept risks (instead of appending by default) with `RiskA` and `RiskB`:

> oc adm upgrade accept --replace=true RiskA,RiskB

Note that we have to modify the [`patchDesiredUpdate`](https://github.com/openshift/oc/blob/f9d98d644110d3413dc4862002395d0c6dfc1da7/pkg/cli/admin/upgrade/upgrade.go#L682-L696) function so that it does not clobber the existing `acceptedRisks` property.

#### Workflow: Alternative

Provide the existing `oc adm upgrade` command with a new and optional `--accept` option to update a cluster, i.e., it take effect only when performing an cluster update.

> oc adm upgrade --to 4.18.16  --accept DualStackNeedsController,LeakedMachineConfigBlocksMCO,OldBootImagesPodmanMissingAuthFlag


The result of `--accept` in the above command is to set up values for the field `.spec.desiredUpdate.acceptRisks` on the `clusterversion/version` manifest:

```console
$ oc clusterversion version -o yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: candidate-4.18
  clusterID: 1c182977-5663-428d-92a3-3d2bdf3fffb6
  desiredUpdate:
    acceptRisks:
    - name: DualStackNeedsController
    - name: LeakedMachineConfigBlocksMCO
    - name: OldBootImagesPodmanMissingAuthFlag
    force: false
    version: 4.18.16
```

Note that missing `--accept` in the above command means accepting no risks at all and all existing accepted risks specified in `cv.spec.desiredUpdate.acceptRisks` is going to be removed. A cluster admin who chooses to do GitOps on the ClusterVersion manifest should not use `oc adm upgrade` to perform a cluster update.

The cluster update to `4.18.16` is blocked. When the cluster is updated to `4.18.17`, e.g., by the following command:

> oc adm upgrade --to 4.18.17  --accept DualStackNeedsController,LeakedMachineConfigBlocksMCO,OldBootImagesPodmanMissingAuthFlag

the CVO accepts the upgrade to `4.18.17` is accepted.

#### Comparing Two Workflows:

Benefits of `oc adm upgrade --to 4.y.z --accept RiskA,RiskB`:

* No need for a new subcommand, so manipulating `cv.spec.desiredUpdate` stays consolidated.
* Encourages cluster-admins to build their own systems for managing acceptance information (e.g. GitOps), because while the new `cv.spec.desiredUpdate.acceptRisks` allows admins to store the risks they accept, it doesn't have space (`name` is the only field at the moment) for them to explain why they find those risks acceptable, and that seems like important information that a team of admins sharing risk-acceptance decision-making would want to have available.

The reasons that we did not go with this workflow: The reviews are in favor of an explicit way of manipulating the accepted risks, without intervening with the command that triggers a cluster upgrade.

Benefits of `oc adm upgrade accept [--replace] RiskA,RiskB`:

* Convenient appending, so an admin can roll forward with the things previous admins have already accepted, without having to worry about what those were.
* Convenient way to share accepted risk names via ClusterVersion, without having to build your own system to share those between multiple cluster-admins.

### API Extensions

This enhancement 
- adds a new field 'clusterversion.spec.desiredUpdate.acceptRisks': It contains
  the names of conditional update risks that are considered acceptable.
- moves `clusterversion.status.conditionalUpdates.risks` up and rename it as
  `clusterversion.status.conditionalUpdateRisks`. It contains all the risks
  for `clusterversion.status.conditionalUpdates`.
- adds a new field 'clusterversion.status.conditionalUpdates.riskNames': It
  contains the names of risks for the conditional update. We have the intension
  to deprecate `clusterversion.status.conditionalUpdates.risks` when the proposed
  TechPreview-gated feature is promoted. Even after promotion, we still have to populate
  the deprecated field until OpenShift 5 because it is a public v1 API.
- adds a new field 'clusterversion.status.conditionalUpdateRisks.conditions': It
  contains the observations of the conditional update risk's current status.

For example,

```console
$ oc clusterversion version -o yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  channel: candidate-4.18
  clusterID: 1c182977-5663-428d-92a3-3d2bdf3fffb6
  desiredUpdate:
    acceptedRisks:
    - name: DualStackNeedsController
    - name: LeakedMachineConfigBlocksMCO
    - name: OldBootImagesPodmanMissingAuthFlag
    force: false
    version: 4.18.15
status:
  conditionalUpdateRisks:  # include every risk in the conditional updates (moved up and renamed)
  - name: DualStackNeedsController
    message: Upgrade can get stuck on clusters that use multiple networks together with dual stack.
    url: https://issues.redhat.com/browse/SDN-3996
    matchingRules:
    - type: Always
    conditions:
    - status: True  # Apply=True if the risk is applied to the current cluster
      type: Applies
      reason: MatchingRule
      message: The matchingRules[0] matches
      lastTransitionTime: 2021-09-13T17:03:05Z
  - name: LeakedMachineConfigBlocksMCO
    message: MCO update might be blocked if some condition holds
    url: https://issues.redhat.com/browse/SDN-2212
    matchingRules:
    - type: PromQL
      promql:
        promql: prom_query
    conditions:
    - status: False
      type: Applies
      reason: NoMatchingRule
      message: None of the matching rules matches
      lastTransitionTime: 2021-09-13T17:03:05Z
  - name: DualStackNeedsController
    ...
  - name: RHELKernelHighLoadIOWait
    ...
  conditionalUpdates:
  - release:
      version: 4.18.16
      image: quay.io/openshift-release-dev/ocp-release@sha256:abc123
    riskNames:
    - DualStackNeedsController
    - OldBootImagesPodmanMissingAuthFlag
    - RHELKernelHighLoadIOWait
    risks:  # deprecated by riskNames
  - release:
      version: 4.18.17
    riskNames:
    - OldBootImagesPodmanMissingAuthFlag
    risks:  # deprecated by riskNames
  - release:
      version: 4.19.1
    riskNames:
    - LeakedMachineConfigBlocksMCO
    risks:  # deprecated by riskNames
```

For every risk appearing in `conditionalUpdateRisksconditionalUpdates`, there is an element in `conditionalUpdateRisks.conditionalUpdateRisks` with the same name. Its condition tells if the risk applies to the current cluster.

When a conditional update is accepted, the names of its associated risks are going to be merged into `clusterversion.status.history.acceptedRisks` which is an existing field before this enhancement. For example, CVO's acceptance of `4.18.17` leads to `OldBootImagesPodmanMissingAuthFlag` being a part of value of `clusterversion.status.history.acceptedRisks`: The wording might look like "The target release ... is exposed to the risks [OldBootImagesPodmanMissingAuthFlag] and accepted by CVO because all of them are considered acceptable".

As a future work, we might consider extending `clusterversion.status.history` with a new field `clusterversion.status.history.acceptedByAdmin`, instead of re-using `clusterversion.status.history.acceptedRisks`, e.g.,

```yaml
acceptedByAdmin:
- type: known-issue
  riskName: LeakedMachineConfigBlocksMCO
- type: warnings
  warnings:
  - warning-one
  - warning-two
```

where each `acceptedByAdmin.riskName` stands for a risk exposed to the update in the history.

When a conditional update is blocked, there is a condition in `clusterversion.status.conditions` with `ReleaseAccepted=False`, e.g.,

```yaml
- lastTransitionTime: 2022-10-11T14:16:13Z
  message: Payload loaded version="4.18.16" ... is blocked because it is exposed to unaccepted risks [RHELKernelHighLoadIOWait]
  reason: UnacceptedRisks
  status: False
  type: ReleaseAccepted
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

The current [OCPSTRAT-2118](https://issues.redhat.com/browse/OCPSTRAT-2118) covers only Standalone clusters and HyperShift will be a followup to it. We need to ensure that he work for [OCPSTRAT-2118](https://issues.redhat.com/browse/OCPSTRAT-2118) does not break HyperShift. This can be achieved by disabling the function on a HyperShift cluster, like we do on a TechPreview disabled standalone cluster.

#### Standalone Clusters

Is the change relevant for standalone clusters?

Yes, the changes of ClusterVersion and `oc adm upgrade` we're proposing here are directly applicable to standalone clusters without needing further integration work in other components.

#### Single-node Deployments or MicroShift

This proposal is applicable to single-node but not to MicroShift which lacks a ClusterVersion and CVO, and manages updates via RPMs.

### Implementation Details/Notes/Constraints

### Risks and Mitigations

### Drawbacks

## Alternatives (Not Implemented)

Currently more developers is in favor of Workflow 2 above. It is more likely we are going to implement it.

## Open Questions [optional]

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests? Yes.
- How will it be tested in isolation vs with other components? The e2e test will stay in CVO repo.
- What additional testing is necessary to support managed OpenShift service-based offerings? We will need to ensure HyperShift still works with or without the feature enabled.


## Graduation Criteria


### Dev Preview -> Tech Preview

N/A.

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

In order to unlock this feature, cluster admins need to set up the environment variable `OC_ENABLE_CMD_UPGRADE_ACCEPT_RISKS=true` when `oc` command is issued and the feature gate `ClusterUpdateAcceptedRisks` has to be enabled on the cluster. Any one of the these two is missing, the feature is disabled.

As long as the implementation from this enhancement stays as a TechPreview-gated feature, our end users do not see any UX difference. After the feature promotion, we will display a deprecation message to `clusterversion.status.conditionalUpdates.risks` to encourage our users to use 'clusterversion.status.conditionalUpdates.riskNames' instead. Since `clusterversion.status.conditionalUpdates.risks` is a public v1 API, we still have to populate it until the next major version: OpenShift 5.


## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Infrastructure Needed [optional]
