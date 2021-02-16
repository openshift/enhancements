---
title: Enable tracking of SDK operators in OLM metrics
authors:
  - "@varshaprasad96"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-05-28
last-updated: 2020-08-13
status: implementable
---

# Enable tracking of SDK operators in OLM metrics

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA

## Summary

This enhancement proposes the procedure to collect metrics regarding the usage of Operator SDK among RedHat operators with the help of [SDK annotations][sdk_annotations] added to the operator image bundles.

## Motivation

Collecting metrics regarding the usage of Operator SDK among operator authors is important to measure the reachability of the tool. Since this is a static data for an operator throughout its lifecycle in the cluster, we intend to periodically extract the Operator SDK related information from the annotations present in operator image bundle, before it is deployed on cluster.

## Goals
The goal of this enhancement is to list the SDK stamps present in operator manifests which can be used to identify the use Operator SDK for building them, while scanning through the operator bundle images and propose a process to periodically extract the relevant data.

## Non-Goals
This proposal does not discuss:
1. The format of the report generated from the data collected through the proposed process.
2. The intervals at which we would collect data from bundle images.

## Proposal

The [proposal][sdk_metrics_proposal] lists the bundle resources in which stamps are added indicating the use of SDK to build an operator bundle image.

The proposed stamps have the following format in `bundle.dockerfile` and `annotations.yaml`:
1. operators.operatorframework.io.metrics.mediatype.v1 = “metrics+v1”
2. operators.operatorframework.io.metrics.builder = “operator-sdk-sdkversion”
3. operators.operatorframework.io.metrics.project_layout = “sdk_project_layout/layout_version”

In CSV and CRDs, they would have the following annotations in Object Metadata:
1. operators.operatorframework.io/builder = “operator-sdk-sdkversion”
2. operators.operatorframework.io/project_layout = “sdk_project_layout/layout_version”

For example, in case of memcached-operator having Kubebuilder layout, the labels can be written as:

```text
LABEL operators.operatorframework.io.metrics.mediatype.v1 = “metrics+v1”
LABEL operators.operatorframework.io.metrics.builder = “operator-sdk-v0.17.0”
LABEL operators.operatorframework.io.metrics.project_layout = “go.kubebuilder.io/v2.0.0”
```

The value of `project_layout` will represent the layout structure and version of SDK project for operators built using SDK after Kubebuilder integration. In case of operators using legacy SDK project structure, the value would represent the kind of operator (ie. `go`, `ansible` or `helm`)

With the help of the above mentioned stamps, the following data can be collected:
1. Number of Go/Ansible/Helm operators.
2. Version of Operator SDK/Ansible/Helm binary which is being used by the operators.
3. The SDK project layout and the plugin version used to build the operators.

As OLM would be moving towards CSV-less bundles, `bundle.dockerfile` and `annotations.yaml` also have SDK labels included in them for future use.

As the metrics we intend to collect on the operators are static, monitoring the operators in cluster and reporting it through the RedHat telemetry will not be required. Instead, in order to obtain this data, we could have a tool which periodically pulls the published catalog images from the image registry, extracts the bundle contents, parses the manifests and reports the data.

The logic to pull the bundle image from the image registry has already been implemented by the [bundle validation library][validate_bundle_image]. After the contents of the image bundle have been extracted, we could parse the manifests (`CSV`/`bundle.dockerfile`/`annotations.yaml`) to obtain the relevant metadata before the bundle is deployed on cluster.

### Use of opm tooling
With the help of `opm` tooling, the operator bundles can be extracted from bundle images.

`opm index export` takes in an index image and unpacks the bundle
specified by the `--package` flag. As mentioned in the
[proposal][extend_bundle_validation_pr] regrading the extension of
static bundle validation, with the implementation of making
`--package` flag optional, all the bundle images could be unpacked and
downloaded. This would enable us to parse the annotations from the
manifests present in the bundle and obtain relevant statistics.

**Notes**
1. Though most of the logic is implemented in the validation library, this process is not necessary an extension/additional feature to the existing tools as its use-case is not relevant to the users of OLM or SDK.
2. A similar tool to list the "bad" bundles in a catalog has been proposed - [Extending static bundle validation][extend_bundle_validation_pr].
2. The naming of SDK labels are generic. Hence, this is helpful to track operators with custom label values.

[sdk_metrics_proposal]: https://github.com/operator-framework/enhancements/pull/21
[cluster_monitoring]: https://github.com/openshift/cluster-monitoring-operator
[sdk_annotations]: https://github.com/operator-framework/enhancements/blob/master/enhancements/sdk-metrics.md
[validate_bundle_image]: https://github.com/operator-framework/operator-registry/blob/master/docs/design/operator-bundle.md#validate-bundle-image
[extend_bundle_validation_pr]: https://github.com/operator-framework/enhancements/pull/28
