---
title: To Multi Arch
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

# Upgrade To Multi Arch

## Summary

This enhancement defines the extension of openshift/api to improve the communication between CVO and cluster operators and how they use the API to evaluate the progress of [migrating an existing single-architecture cluster to multi-architecture](https://docs.openshift.com/container-platform/4.16/updating/updating_a_cluster/migrating-to-multi-payload.html#migrating-to-multi-payload).


## Motivation

During a cluster upgrade, the Cluster Version Operator (CVO) downloads the desired release payload and applies the manifests of OpenShift operators to the cluster.
Each OpenShift operator reports the versions of its own and its operands via `ClusterOperator.status.versions` so that CVO can oversee the upgrade process of the operator.
When all of their versions are the desired version, CVO considers that the upgrade of the operator completes.

The command `oc adm upgrade --to-multi-arch` is used to upgrade the cluster from a single-architecture to a multi-architecture cluster *at the current version*.
Because `ClusterOperator.status.versions` does not provide information about the multi-arch of the operator's image and `--to-multi-arch` starts the upgrade targeting *the same version*, the upgrade of each operator finishes right away from the CVO's point of view. Without the ability to recognize the component's turning into multi-arch, CVO's evaluation of the upgrade progress is inaccurate . This enhancement is to define a proposal to address the issue.


### User Stories

* As a cluster admin, I want to check the upgrade progress of migrating a cluster to multi-architecture.


### Goals

Provide a proposal that addresses the issue about the upgrade progress during a cluster migration to multi-arch.

### Non-Goals


## Proposal

The proposal includes
- the extension of `ClusterOperator.status.versions` and `ClusterVersion.status.desired` in openshift/api to provide information about the architecture, 
- the implementation details on how an operator adopts the extension, and
- the implementation details on how CVO adopts the extension.

Any cluster operator, besides the existing fields `ClusterOperator.status.versions.version` and `ClusterOperator.status.versions.name`, may fill in the field `ClusterOperator.status.versions.architecture` with the value from the environment variable `OPERATOR_IMAGE_ARCHITECTURE` which are defined in the deployment of the operator like `OPERATOR_IMAGE_VERSION`. See [cluster-authentication-operator](https://github.com/openshift/cluster-authentication-operator/blob/0b7172707fcb3ba9880e8b100542700bd7d080d8/manifests/07_deployment.yaml#L69-L72) for example.

When loading the payload of a release, [CVO gets the architecture of the release from the metadata](https://github.com/openshift/cluster-version-operator/blob/455abd6acc3e69653e7506b1cef4cbdcd2512b92/pkg/payload/payload.go#L386) or "runtime". It can be used to [evaluate the upgrade progress of operators](https://github.com/openshift/cluster-node-tuning-operator/blob/890d5eefa95637750b5bfa94b00e2506100ce63c/pkg/operator/status.go#L30).


### API Extensions

We extend [`ClusterOperator.status.versions`](https://github.com/openshift/api/blob/82e082220d910f489d35880174e2eb90e21f5589/config/v1/types_cluster_operator.go#L81) with the following field:


```go
type OperandVersion struct {
  // Architecture indicates for which architecture of a particular operand image is built, such as "amd64", "arm64", "ppc64le", "s390x" "multi", etc.
  // +optional
  Architecture string `json:"architecture"`
}
```

and [`ClusterVersion.status.desired`](https://github.com/openshift/api/blob/82e082220d910f489d35880174e2eb90e21f5589/config/v1/types_cluster_version.go#L741) with:


```go
type Release struct {
	// Architecture is the architecture for which the release image is built, such as "amd64", "arm64", "ppc64le", "s390x" "multi", etc.
	// +optional
	Architecture string `json:"architecture"`
  ...
}
```

### Topology Considerations

TODO (hongkliu): I have no idea about how CVO works differently from Standalone Clusters.

### Implementation Details/Notes/Constraints

#### Operator's deployment

Each operator's deployment has an environment variable `OPERATOR_IMAGE_ARCHITECTURE` for the architecture of the operator image 
and `OPERAND_NAME_IMAGE_ARCHITECTURE` for the architecture of each operand that the operator manages. The values are vendored by ART that builds the OpenShift releases with the `oc adm release new` command.

```
env:
- name: OPERATOR_IMAGE_ARCHITECTURE
  value: UNDEFINED-ARCHITECTURE
```

Each operator takes the values of those environment variables to fill in the `architecture` field in `ClusterOperator.status.versions`.
The default value is the empty string if any of the environment variable is not defined so that operator can use the new field in the API without waiting for ART to actually define them.

A special operator: [machine-config-operator](https://github.com/openshift/machine-config-operator) (MCO).
It is special because it is the last operator to upgrade in a cluster upgrade and it usually takes longest for MCO to upgrade. With its adoption of the extended API, the ending point of migrating to multi-arch will become accurate which improves the overall UX the most.

The `oc adm release new` command will be modified to render the environment variables such as `OPERATOR_IMAGE_ARCHITECTURE` with expected values by replacing "UNDEFINED-ARCHITECTURE" with "amd64", "arm64", "multi", etc. See how the "versions" are [handled in the command](https://github.com/openshift/oc/blob/master/pkg/cli/admin/release/image_mapper.go#L342-L348).


A simple implementation is to ask ART to use `--metadata {"release.openshift.io/architecture": "<ARCHITECTURE>"}` in the command for all the cases. ART uses it only for "multi" today. Then `<ARCHITECTURE>` there or the empty string as the default value is used to replace "UNDEFINED-ARCHITECTURE" in the deployment manifest for each operator.

We will allows [other values than "Multi" and ""](https://github.com/openshift/api/blob/a1523024209f3122be9e9d53515da470c6a2458b/config/v1/types_cluster_version.go#L281) and modify the code [here](https://github.com/openshift/cluster-version-operator/blob/455abd6acc3e69653e7506b1cef4cbdcd2512b92/pkg/payload/payload.go#L388) in CVO to accept them.

We will also introduce utility functions `ArchitectureForOperandFromEnv`, `ArchitectureForOperatorFromEnv` and `ArchitectureForOperand`, like [the ones for the version](https://github.com/openshift/library-go/blob/4602d24d27bc94d0cbee0d4646eee867e808c5b4/pkg/operator/status/version.go#L72-L80) in the openshift/library-go. Operators can use those functions to get the architecture information.

#### CVO

CVO consumes the value of the `architecture` field from `ClusterOperator.status.versions` for each `ClusterOperator` on the cluster. An operator is considered upgraded to the `ClusterVersion.status.desired` if and only if for each element in `ClusterOperator.status.versions` the following two conditions are satisfied:

- The version matches, and
- The architecture matches or the `ClusterOperator.status.versions.architecture` is missing or the empty string. 

The 2nd condition above allows for a smooth procedure for all the operators to adopt the extended API. CVO can start to use the extended API without being blocked by any operator. For example, if cluster-authentication-operator decides not to use the API at the moment, the impact is that CVO considers the upgrade completes more quickly than it actually does which is the current behavior. As soon as cluster-authentication-operator becomes ready to use `architecture` and reports it in the `ClusterOperator.status`, the upgrade progress for the operator becomes accurate because CVO can use it to evaluate the progress. After all operators adopt `architecture`, the whole upgrade progress of migrating a cluster to multi-arch becomes accurate.

### Risks and Mitigations

As we allow ART, CVO, operators to adopt the extended API on its own pace. The upgrade progress of migrating to multi-arch only gets better. No risks are foreseen at the moment.

### Drawbacks

If no operator provides the architecture information, the upgrade procedure will be considered done very quickly which is as bad as the API was not extended.

## Open Questions [optional]

## Test Plan

- An e2e test which is executed in an optional pre-submit at the beginning until CVO and an operator (or MCO?) adopt the new API, exists for CVO that monitors the upgrade to multi-arch. It passes only when the expected events or logs with information about the architecture are observed.


## Graduation Criteria

TODO (hongkliu):

- Not sure if API maturity levels is relevant for this enhancement as `ClusterVersion` and `ClusterOperator` are with `VERSION=v1` already.
- The adoption of both CVO and MCO sounds like a milestone and is good enough for TechPreview.


## Alternatives

An alternative to the above "fail-open" evaluation on `ClusterOperator.status.version.architecture` by CVO is "fail-close", i.e., CVO does not move on until the operator reports the desired architecture.
CVO release the feature only after all the chosen operators adopt the API. We can start with MCO only and gradually add more when the other operators are ready.
CVO can also start with "fail-close" as proposed above in this enhancement and switch to "fail-close" only when all operators use the API.
