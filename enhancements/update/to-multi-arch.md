---
title: Duration of Migration To Multi Arch
authors:
  - "@hongkailiu"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2024-10-01
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OTA-962
see-also:
  
replaces:

superseded-by:

---

# Duration of Migration To Multi Arch

## Summary

This enhancement provides a proposal to improve Cluster Version Operation's evaluation on the progress of [migrating an existing single-architecture cluster to multi-architecture](https://docs.openshift.com/container-platform/4.17/updating/updating_a_cluster/migrating-to-multi-payload.html).


## Motivation

During a cluster upgrade, the Cluster Version Operator (CVO) downloads the desired release payload and applies the manifests of OpenShift operators to the cluster.
Each OpenShift operator reports the versions of its own and its operands via `ClusterOperator.status.versions` so that CVO can oversee the upgrade process of the operator.
When all of their versions are the desired version, CVO considers that the upgrade of the operator completes.

The command `oc adm upgrade --to-multi-arch` is used to migrate a cluster from a single-architecture to a multi-architecture cluster. It is implemented via a cluster upgrade by switching the underlying images to the ones from the multi-arch payload *at the current version*.
Because `ClusterOperator.status.versions` contains the information only about the operator's version number at the moment and `--to-multi-arch` starts the upgrade targeting *the same version*, the upgrade of each operator finishes right away from the CVO's point of view. Without the ability to recognize the component's turning into multi-arch, CVO's evaluation of the upgrade progress is inaccurate, i.e., the operators' upgrade is still ongoing when CVO claims complete. This enhancement is to define a proposal to address the issue.


### User Stories

* As a cluster admin, I want CVO to claim that migrating a cluster to multi-architecture is complete correctly.


### Goals

This enhancement is to provide a proposal that addresses the issue about the upgrade progress during a cluster migration to multi-arch. The proposal has impact only after the migration to multi-arch starts.

### Non-Goals

Whether or not 'oc adm upgrade' is executed succesfully or the requested upgrade is accepted is not in the scope of this enhancement. So it is _not_ required to have test cases for this enhancement to cover (although they are valid in general):
- upgrade from multi-arch to single-arch,
- upgrade from multi-arch to multi-arch, or
- upgrade a cluster in a cloud provider where multi-arch is not supported to multi-arch.

The `oc adm upgrade status` might be useful to monitor the migration progress to multi-arch but the output of the command does not block this enhancement and its implementation. The `status` command is designed for cluster upgrades but migration is not an upgrade conceptually and is implemented through an upgrade.

## Proposal

The proposal includes the steps that an OpenShift cluster operator may take to provide information about the upgrade in additional to the existing names and version numbers for the components that the operator manages.

* Add a few elements into the existing list `ClusterOperator.status.versions` in the `ClusterOperator` manifest. Those new `versions` contains the image pull specifications for the operator and OPTIONALLY the managed operands that the operator chooses to track. Although the version stays the same, the image pull specification are different if a cluster becomes multi-arch.

* Populate the `ClusterOperator.status.versions` including these new elements above.

The CVO has already implemented the way that it checks if each element in the `ClusterVersion.status.versions` from the payload's has the counterpart in the actual one on the cluster.

There is no need to extend the APIs in `openshift/api`, or modify the CVO implementation.

### Workflow Description

This enhancement changes no workflow.

### API Extensions

None.

### Topology Considerations

#### Hypershift / Hosted Control Planes

[Hosted control planes enables the decoupling of updates between the control plane and the data plane](https://docs.openshift.com/container-platform/4.17/hosted_control_planes/hcp-updating.html#hcp-get-upgrade-versions_hcp-updating):
* Update a control plane: set up `HostedCluster.spec.release`, and
* Update a data plane: set up `NodePool.spec.release`,
where the value is [the image pull specification of an OCP release payload image](https://hypershift-docs.netlify.app/reference/api/#hypershift.openshift.io/v1beta1.Release).

For the upgrade status:
* `HostedCluster.status.version` shows the status and `HostedCluster.status.payloadArch` indicates the arch.
* `NodePool.status.version` shows the version number and `NodePool.status.conditions` shows the progress.

The difference with standalone clusters:
* The image in `HostedCluster.spec.release` does not need to be with the same version number to migrate to multi-arch. It is not clear for `NodePool.spec.release`.
* Is `ClusterVersion/version` still precise?
* Is `ClusterOperator.status` still precise?

TODO (hongkliu): Update the answers to ^^^ from the HyperShift team.

CVO might have to watch `HostedCluster.status` and `NodePool.status` to monitoring the upgrade progress if `ClusterVersion.status` and `ClusterOperator.status` are no longer updated on a hosted cluster.

#### Standalone Clusters
The proposal targets standalone clusters.

#### Single-node Deployments or MicroShift
The proposal should apply to SNO as CVO treats it the same as a standalone cluster.
The version of MicroShift is not managed by [dnf](https://docs.redhat.com/fr/documentation/red_hat_build_of_microshift/4.17/html-single/installing_with_an_rpm_package/index#microshift-install-rpm), not CVO and thus MicroShift does not hit the issue described here.

### Implementation Details/Notes/Constraints

The changes in this section are also reflected in [clusteroperator.md](../../dev-guide/cluster-version-operator/dev/clusteroperator.md).

Each operator creates a pull request that includes:

- in the `ClusterVersion` manifest:

  - a new element with `name=operator-image` and `version=placeholder.url.oc.will.replace.this.org/placeholdernamespace:<operator-tag-name>` where `<operator-tag-name>` is the name of the tag for the operator in [image-references](./operators.md#how-do-i-ensure-the-right-images-get-used-by-my-manifests).
  - OPTIONALLY a new element with `name=operand-image` and `version=placeholder.url.oc.will.replace.this.org/placeholdernamespace:<operand-tag-name>` where `<operand-tag-name>` is the name of the tag for an operator's operand in [image-references](./operators.md#how-do-i-ensure-the-right-images-get-used-by-my-manifests).


- in the `deployment` manifest, environment variables or flags with a placeholder for an image pull specification that will be replaced when the payload is built.

- in the operator's implementation, the operator takes the values of the environment variables or the flags and use them to populate the `clusteroperatror.status.versions`.

A special operator: [machine-config-operator](https://github.com/openshift/machine-config-operator) (MCO).
It is special because it is the last operator to upgrade in a cluster upgrade and it usually takes longest for MCO to upgrade. With the new populated elements in `clusteroperator.status.versions`, the ending point of migrating to multi-arch will become accurate which improves the overall UX the most.
The pull request [MCO#4637](https://github.com/openshift/machine-config-operator/pull/4637) is created for the purpose.
Other operators may take the pull request as an example when it is ready to report the image pull specifications in the operator's `.status.versions`.

### Risks and Mitigations

As we allow operators to modify their implementation on its own pace. The upgrade progress of migrating to multi-arch only gets better when more operators take the actions. No risks are foreseen at the moment.

### Drawbacks

All OpenShift cluster operators have to modify their implementation to make the whole migration process accurate.

## Open Questions [optional]

## Test Plan

An e2e test which is executed in an optional pre-submit at the beginning until CVO and an operator (or MCO?) adopt the new API, exists for CVO that monitors the upgrade to multi-arch. It passes only when the expected `clusterversion/version`, events or logs with information about the migration are observed. This might require the modification on CVO.

The initial cluster for the e2e test may parameterized by its architecture. It must pass for `amd64` or `arm64`. Other architectures are nice to be included but optional depending on testing resources.

## Graduation Criteria
Not applicable; this enhancement just describes how multiple cluster components can use existing APIs to address a problem, there is nothing to graduate.

### Dev Preview -> Tech Preview
### Tech Preview -> GA
### Removing a deprecated feature

## Upgrade / Downgrade Strategy
Not applicable; the multi-arch migration does not change the version number.

TODO (hongkliu): The rescue path in case migration failed for any reason. This might not be directly relevant to this enhancement though.

## Version Skew Strategy

Each operator decides when to make the changes proposed in this enhancement and has no dependence on other operators' versions.

## Operational Aspects of API Extensions
Not applicable; this enhancement makes no API Extensions.

## Support Procedures

Migration to multi-arch is implemented through a cluster upgrade. A bug that might be introduced unexpectedly by this enhancement could lead to failures on upgrading an cluster and then the migration that gets blocked.

## Alternatives

An alternative to the above approach is extending `ClusterOperator.status.version` in `openshift.api` with a `architecture` or `image` field.
Then operators produce the new field that is consumed by CVO.
This approach probably is going to take more effort to achieve.