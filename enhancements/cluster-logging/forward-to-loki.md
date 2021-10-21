---
title: forward_to_loki
authors:
  - "@alanconway"
reviewers:
  - "@jcantrill"
  - "@periklis"
approvers:
  - "@jcantrill"
creation-date: 2021-03-29
last-updated: 2021-07-13
status: implementable
see-also:
superseded-by:
---

# Forward to Loki

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Operational readiness criteria is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add a new output type to the `ClusterLogForwarder` API to forward logs to a Loki instance.
An important part of using Loki is choosing the right _Loki labels_ to define log streams.
We present a default set of Loki labels, and explain how the choice of labels affect performance and queries.

## Motivation

Loki will become our default log store in the near future.
There are also several use cases for forwarding to an external Loki instance.

### Goals

- Forward logs to a Loki instance
- Choose a good default set of Loki labels.
- Provide configuration to control the choice of Loki labels
- Describe how expected query patterns correspond to Loki labels and JSON payload

### Non-Goals

The following may be covered in future proposals, but are out of scope here:

- Query tools.
- Optimizing/reducing the JSON payload format.
- Alternate payload formats.
- Use of collectors other than `fluentd` (e.g. Promtail, FluentBit)

## Proposal

### Summary of Loki labels

Loki and Elasticsearch take different approaches to indexing and search.
Elasticsearch does content parsing and indexing _during ingest_ to optimize the _query_ path.
Loki optimizes the _ingest_ path by using simple key-value `labels` for initial indexing,
and deferring content parsing until _query_ time.

The benefit is faster ingest, which reduces the risk of dropping logs.
The trade-off is that need to carefully choose what to use as labels.

For more details:
- [Guide to labels in Loki](https://grafana.com/blog/2020/08/27/the-concise-guide-to-labels-in-loki/)
- [Out-of-order streams issue]([https://github.com/grafana/loki/issues/1544)

To summarize the key points:
- Each unique combination of labels defines an ordered Loki _stream_.
  - Too many streams degrades performance.
  - _Cardinality_ refers to the number of unique label combinations.
- The cardinality of `kubernetes.pod_name` was found to be
  - too high in clusters with _very_ rapid turn-over of short-lived pods.
  - acceptable in other (possibly more typical) clusters with high log data rates.
- Minimize the _total number_ of labels per stream
  - There is a limit of 30 max, fewer labels == better performance.
- Log streams must be _ordered by collection timestamp_.
  - Must not combine independent logs streams that may have out-of-order clocks.\
  **Note**: Grafana claim they will remove this restriction in future, but this design can cope with it.

All log record data will still be available as a JSON object in the Loki log payload.
Labels do not need to _identify the source of logs_, only to _partition the search space_.
At query time labels reduce the search space, then Loki uses log content to complete the search.

**Note**: Loki [label names][] must match the regex `[a-zA-Z_:][a-zA-Z0-9_:]*`.
Meta-data names are translated to Loki labels by replacing illegal characters with '_'.

Nested JSON objects are referenced by converting their JSON path to a label name.
For example, if `kubernetes.namespace_name` is a label, then this JSON log record:

``` json
{"kubernetes.namespace_name":"foo","kubernetes":{"labels":{"foo":"bar"}}}
```
would match this LogQL filter:

``` logql
{ kubernetes_namespace_name="foo" } | json | kubernetes_labels_foo == "bar"
```
### Default Loki Labels

By default the following log fields are translated to Loki labels and may be used in queries:

* `log_type`: Category of log, a string prefixed with `application`, `infrastructure` or `audit`.
* `kubernetes.namespace_name`: namespace where the log originated.
* `kubernetes.pod_name`: name of the pod where the log originated.
* `kubernetes.container_name`: name of the container within the pod.
* `kubernetes_host`: host name of the kubernetes node where the stream originated.

The default label set should be suitable for most deployments.
The user can configure a different set for specific needs.

Log streams _may_ have additional labels not mentioned here.
User queries should not rely on such labels, they may change without notice.

**Note:** Labels are used to partition the search space.
The full log meta-data is available in the JSON payload for filtering.

#### Implementation detail: labels for uniqueness

In the initial implementation, the following fields are _always_ translated to labels.
This is necessary to ensure ordered time-stamps in log streams.

* `kubernetes_host`: host name of the kubernetes node where the stream originates.
* `tag`: a value that distinguishes a unique stream on a host.

**Note**: The `tag` label is used _only_ to ensure ordered streams.
It should not be used in queries, its format may change in future.

**Note**: The set of additional labels to enforce uniqueness may change without notice.
Users should only rely on the listed default labels, or on labels explicitly mentioned
in a custom configuration.

### Proposed API

Add a new `loki` output type to the `ClusterLogForwarder` API:

``` yaml
- name: myLokiOutput
  type: loki

  url: ...
  secret: ...
```

`url` and `secret` have the usual meaning with regard to TLS and certificates.
The `secret` may also contain `username` and `password` fields for Loki.

The following optional output fields are Loki-specific:
* `tenentKey: (string, optional) \
   Tenet name (also known as org-id) to add to loki requests. See [Loki Multi-Tenancy](https://grafana.com/docs/loki/latest/operations/multi-tenancy/)
* `labelKeys`: ([]string, default=_see [Default Loki Labels](#default-loki-labels))_ \
  A list of meta-data keys to replace the default labels.\
  Keys are translated to [label names][] as described in [Summary of Loki Labels](#summary-of-loki-labels)
  Example: `kubernetes.labels.foo` => `kubernetes_labels_foo`.\
  At least these keys are supported:
  - `kubernetes.namespaceName`: Use the namespace name as the tenant ID.
  - `kubernetes.labels.<key>`: Use the string value of kubernetes label with key `<key>`.
  - `openshift.labels.<key>`: use the value of a label attached by the forwarder.

The full set of meta-data keys is listed in [data model][].

### Implementation Details

The output will be implemented using this fluentd plugin: https://grafana.com/docs/loki/latest/clients/fluentd/

Notes on plug-in configuration:

- Security: configured by the `output.secret` as usual.
- K8s labels as Loki labels: supported by the plug-in.
- Always include `kubernetes.host` to avoid avoid out-of-order streams.
- Tenant set from `output.tenantKey`
- Output format is `json`, serializes the fluentd record like other outputs.
- Static labels set as `extra_labels` to avoid extracting from each record.

### User Stories

#### Treat each namespace as a separate Loki tenant

I want logs from each namespace to be directed to separate Loki tenants.

``` yaml
- name: myLokiOutput
  type: loki
  url: ...
  secret: ...
  tenantKey: kubernetes.namespace_name
```

#### Query all logs from a namespace

``` logql
{ kubernetes_namespace_name="mynamespace"}
```

#### Query logs from a specific container in a named Pod

``` logql
{ kubernetes_namespace_name="mynamespace"" } |= kubernetes_pod_name == "mypod" |= kubernetes_container_name="mycontainer
```

#### Query logs from a labeled application

Using the default configuration:

``` logql
{ } |= kubernetes_labels_app == "myapp""
```

By adding `labelKeys: [ kubernetes.labels.app ]` to the Loki output configuration this
can be a faster label query:

``` logql
{ kubernetes_labels_app="myapp" }
```

#### Example Queries From The Field

These are real queries taken from use in the field and translated.
This deployment makes heavy use of the `app` and `deploymentconfig` kubernetes labels, so
we assume this configuration to speed up queries:

``` yaml
- name: myLokiOutput
  type: loki
  url: ...
  secret: ...
  labelKeys: [ kubernetes.labels.app, kubernetes.labels.deploymentconfig ]
```

``` logql
{cluster="dsaas",kubernetes_labels_app="f8notification"} |= 'level:"error"'

{cluster="dsaas",kubernetes_labels_app="keycloak"} |=  message:"*503 Service Unavailable*"

{cluster="rh-idev", kubernetes_labels_deploymentconfig="bayesian-pgbouncer"} !~ “client close request|coreapi tls=no|server_lifetime|new connection to server|client unexpected eof|LOG Stats|server idle timeout|unclean server”

{cluster="rh-idev", kubernetes_labels_deploymentconfig="wwwopenshifio"} |= message:"Connection reset by peer"
```


### Risks and Mitigations

#### Cardinality explosions for large scale clusters

We provide a set of labels that should perform well in most cases, but allow the user to tune the labels to manage unusual situations.
For example a CI test cluster that constantly creates and destroys randomly-named pods might want to omit the pod name if it becomes too high cardinality.

## Design Details

### Test Plan

- Unit and e2e tests
- Measure throughput/latency on busy cluster, should exceed Elasticsearch.
- Find breaking point for log loss in stress test, should exceed Elasticsearch.
- Measure query response on large data, must be acceptable compared to Elasticsearch.

### Graduation Criteria

- Performance benchmarks, must equal or outperform Elasticsearch store.

#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature

### Upgrade / Downgrade Strategy
### Version Skew Strategy

## Drawbacks

## Alternatives

## Implementation History

None.

[label names]: https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
[data model]: https://github.com/openshift/origin-aggregated-logging/blob/master/docs/com.redh
at.viaq-openshift-project.asciidoc
