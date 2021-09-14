---
title: output-parameter-templates

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

# Output Parameter Templates

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Elasticsearch needs log messages with different formats to be directed to different indices.
The ClusterLogForwarder API can direct logs from diffeent _pods_ to different Elasticsearch indices, based on namespace or labels, using the `structuredTypeKey` field.

With sidecar services like ISTIO, it is common to have multiple _containers_ in a _single pod_ using different log formats.
This proposal adds a single new field `structuredTypePattern` to direct logs from different _containers_ to different indices.

For example, to generate an index of the form "label_value+container_name":

``` yaml
  elasticsearch:
    structuredTypePattern: '{.kubernetes.label.foo}+{.kubernetes.container_name}'
```

Note that if any field referenced in the template pattern is missing,
the message will be treated as an unstructured message as described in
[](forwarding-json-structured-logs.md)

The template pattern is [JSONpath](https://goessner.net/articles/JsonPath/).
JSONpath is used by [kubernetes](https://kubernetes.io/docs/reference/kubectl/jsonpath)
and [fluentd](https://docs.fluentd.org/plugin-helper-overview/api-plugin-helper-record_accessor)
and is relatively simple to learn.

This proposal is only about forwarding structured JSON logs to Elasticsearch, but the pattern described here may be useful in other situations.

## Motivation

Initial motivation comes from [forwarding JSON to Elasticsearch](forwarding-json-structured-logs.md).
The popularity of JSON logging means that an application container be deployed in a pod with a sidecar logging an incompatible JSON log format.
Adding `elasticsearch.structuredTypePattern` allows JSON logs from different containers to be sent to separate indices.

This feature can be used in any situation where per-log output parameters are derived from log record fields.
For example `cloudwatch.groupPattern`, `loki.tenantPattern`, `kafka.topicPattern` and so on.

### Goals

Implement `elasticsearch.structuredTypePattern` for the Elasticsearch output.

### Non-Goals

Other outputs may re-use this pattern, but are not required to complete this proposal.

## Proposal

Add field `output.elasticsearch.structuredTypePattern` of type string.
Compute the index name for each log record by applying the `structuredTypePattern` expression to the log record.

### User Stories

#### Send JSON logs from pods labeled app=myApp to separate indices based on container name

``` yaml
outputs:
  - name: myApp
    type: elasticsearch
	structuredTypePattern: '{.kubernetes.label.app.myApp}-{.kubernetes.container_name}'
```

Note: if the JSONpath expression is invalid, or if any of the fields it referrs to are missing,
log messages will be treated as _unstructued_ as described in [](forwarding-json-structured-logs.md)

### Implementation Details

Need to research available libraries and fluentd plugins.
We might implement a plugin similar to [fluentd record_transformer](https://docs.fluentd.org/filter/record_transformer) but using JSONpath, or there may be existing plugins we can re-use.

The following links are relevant:

- [JSONpath specification](https://goessner.net/articles/JsonPath/)
- [JSONpath in Kubernetes](https://kubernetes.io/docs/reference/kubectl/jsonpath/)
- [K8s Go library](https://pkg.go.dev/k8s.io/client-go/util/jsonpath)
- [Ruby library](https://github.com/joshbuddy/jsonpath)
- [LUA library](https://github.com/hy05190134/lua-jsonpath/blob/master/jsonpath.lua)

The implementation should keep in mind that we will use this pattern again.
Re-usable code should be packaged in re-usable libraries.

### Risks and Mitigations

- JSONpath may be confusing to users
  - It is used in common k8s tools and in other tools (fluentd)
  - Any alternative is likely to be equally or more complex.
- Potential to create too many indices if not used carefully.
  - We already have this problem with `structuredTypeKey` but it may be exacerbated.

## Design Details

### Test Plan

- Sufficient unit test coverage
- functional test with multi-container Pods, verify logs are indexed as expected.
- functional test for error cases: invalid JSONpath, missing fields in patterns.

### Graduation Criteria
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature

### Upgrade / Downgrade Strategy
### Version Skew Strategy
## Implementation History

## Drawbacks

Additional complexity.

## Alternatives

None known.
