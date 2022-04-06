---
title: Multi-container-structured-logging

authors:
  - "@alanconway"

reviewers:
  - "@lukas-vlcek"
  - "@jcantril"
  - "@vimalk78"

approvers:
  - "@jcantril"

creation-date: 2021-04-21

last-updated: 2021-09-14

status: provisional

see-also:
  - "forwarding-json-structured-logs.md"
---

# Multi-container structured logging

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

When JSON log records are forwarded to Elasticsearch, messages with different JSON formats must be directed to different indices.

The ClusterLogForwarder API can direct logs from different _Pods_ to different indices, using the `structuredTypeKey` and `structuredTypeName` fields.

This proposal extends `output.elasticsearch` to allow logs from different _containers_ within a single Pod to be sent to different indices using _annotations_.

## Motivation

- Sidecars are a common pattern in kubernetes and openshift clusters, which means there will be multiple containers in a Pod.
- Many popular sidecars (for example ISTIO) use JSON logging.
- The JSON log formats of sidecars and application containers may not be compatible, and must be separated to avoid index problems with Elasticsearch.
- K8s annotations are used to [https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/](attach arbitrary non-identifying metadata to objects)

### Goals

When JSON log records are forwarded to Elasticsearch, direct messages with different JSON formats to different Elasticsearch indices.

### Non-Goals

No other outputs will use this pattern initially, but may do in future if appropriate.

## Proposal

Per-container directives are specified as annotations like this:

> `containerType.logging.openshift.io/`*container-name*: "*format-type*"

1. Namespace prefix `containerType.logging.openshift.io/` indicates this is a logging directive for a named container.
2. Namespace suffix *container-name* is the name of the container.
3. Annotation value *format-type* is the name of the format to use for logs from thi container.

If the `ClusterLogForwarder.elasticsearch` also has structured type configuration, the type is chosen by the first of the following rules that applies:

1. `containerType` annotation if there is one that matches the container name.
2. `elasticsearch.structuredTypeKey` if the key is present and non-empty on a log record.
3. `elasticsearch.structuredTypeName` if specified.

If no type is identified by the above rules, the logging data is indexed as *unstructured*.

See also [forwarding-json-structured-logs.md](./forwarding-json-structured-logs.md)

### Example

An example will help.

``` yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
metadata:
  name: "instance"
spec:
  pipelines:
  - name: JSONToElasticsearch
    inputRefs: [ application ]
	outputRefs: [ default ]
	parse: json
```

This forwarder configuration parses and forwards JSON logs to the default Elasticsearch
instance. The format names are specified as Pod annotations, for example:


``` yaml
apiversion: apps/v1
kind: Pod
metadata:
  annotations:
	containerType.logging.openshift.io/application-foo: format-v1
	containerType.logging.openshift.io/sidecar-foo: format-v2
	containerType.logging.openshift.io/istio-sidecar: istio-format-v1
```

For this Pod:
- logs from a container named "application-foo" will be indexed under "format-v1"
- logs from a container named ""sidecar-foo"  will be indexed under "format-v2"
- logs from a container named "istio-sidecar" will be indexed under "istio-format-v1"
- logs from a container with any other name will be treated as _unstructured_.

### Details

There are no new entries for `ClusterLogForwarder` configuration.
The combination of `type: elasticsearch`, `parse: json` and a `containerType` annotation activates the feature.

This proposal is only for forwarding structured JSON logs to Elasticsearch, but the pattern described here may be useful in other situations.

### User Stories

#### Send JSON logs from containers in the same Pod to different Elasticsearch indices

As described above.

### API Extensions

Introduces new labels.

### Implementation Details

None.

### Risks and Mitigations

Potential to create too many indices if not used carefully.
This is somewhat mitigated because the user must create separate labels for each index, which makes accidentally creating large numbers of indices unlikely.

## Design Details

None

### Test Plan

- Unit test coverage to validate expected configuration for combinations of annotations and structuredType configuration.
- functional test with multi-container Pods
  - verify logs with an assigned type are indexed as expected.
  - verify logs with no type are indexed as _unstructured_.

### Graduation Criteria
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature
### Upgrade / Downgrade Strategy
### Version Skew Strategy
### Operational Aspects of API Extensions
#### Failure Modes
#### Support Procedures
## Implementation History
## Drawbacks

Additional complexity.

## Alternatives

None known.
