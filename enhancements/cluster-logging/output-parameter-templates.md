---
title: output-parameter-templates

authors:
  - "@alanconway"

reviewers:
  - "@ewolinetz"
  - "@lukas-vlcek"
  - "@jcantril"
  - "@sichvoge"
  - "@vimalk78"

approvers:
  - "@jcantril"

creation-date: 2021-04-21

last-updated:

status: provisional

see-also:
  - "/enhancements/cluster-logging/forwarding-json-structured-logs.md"
---

# Output Parameter Templates

## Release Sign-off Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The ClusterLogForwarder.Outputs API can already set some output parameters from log record fields.

For example, to set the Elasticsearch index for each log record to the value of kubernetes label "foo" on that record:

``` yaml
  elasticsearch:
    indexKey: kubernetes.label.foo
```

This proposal allows multiple record fields to be _combined_ into an output value.

For example, to set an index of the form "label_value+container_name":

``` yaml
  elasticsearch:
    indexPattern: '{.kubernetes.label.foo}+{.kubernetes.container_name}'
```

The template pattern is [JSONpath](https://goessner.net/articles/JsonPath/).
JSONpath is used by [kubernetes](https://kubernetes.io/docs/reference/kubectl/jsonpath)
and [fluetnd](https://docs.fluentd.org/plugin-helper-overview/api-plugin-helper-record_accessor)
and is relatively simple to learn.


## Motivation

Initial motivation comes from [forwarding JSON to Elasticsearch](forwarding-json-structured-logs.md).
The popularity of JSON logging means that an application container be deployed in a pod with a sidecar logging an incompatible JSON log format.
Adding `elasticsearch.indexPattern` allows JSON logs from different containers to be sent to separate indices.

This feature can/will be used in any situation where per-log output parameters are derived from log record fields.
For example `cloudwatch.groupPattern`, `loki.tenantPattern`, `kafka.topicPattern` and so on.

### Goals

Implement elasticsearch.indexPattern for the Elasticsearch output.

### Non-Goals

Other outputs will follow, but are not required to complete this proposal.

## Proposal

Add field `output.elasticsearch.indexPattern` of type string.
Compute the index name for each log record by applying the `indexPattern` expression to the log record.

### User Stories

#### Send JSON logs from pods labeled app=myApp to separate indices based on container name

``` yaml
outputs:
  - name: myApp
    type: elasticsearch
	indexPattern: '{.kubernetes.label.app.myApp}-{.kubernetes.container_name}'
```

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

### Risks and Mitigation

- JSONpath may be confusing to users
  - It is used in common k8s tools and in other tools (fluentd)
  - Any alternative is likely to be equally or more complex.
- Invalid JSONpath may cause log loss
  - CLO must validate JSONpath expressions and error on invalid expressions.
  - Provide some tools to help test expressions on realistic log records before use?
- Allows who can influence log content can influence the output parameters.\
  Example for label-based indexing, anyone who can set labels can create indices
  - We already have this problem with `indexKey` but it may be exacerbated.

## Design Details

### Open Questions [optional]

Consider other template languages:
- JSONpath is widely supported: used by kubernetes and fluentd, has libraries in Go, ruby, C, LUA, python etc.
- k8s also uses Go templates, but they are less well known and supported outside of Go, and arguably more awkward to construct.

Allow templates in existing fields or only in new `xxxPattern` fields?
- Risks of confusing JSONpath as plain string or vice versa?
- Need to mark JSONpath specially?

Do we need safeguards for accidental/malicious creation of excessive indices?
- Should this be implemented by the Elasticsearch operator or forwarder?

### Test Plan

- Sufficient unit test coverage
- E2E test `elasticsearch.indexPattern` with multi-container Pods
  - Verify logs are indexed as expected.

## Drawbacks

Additional complexity.

## Alternatives

None known.
