# Forward to Loki

## Release Sign-off Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
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
  - Must not allow distinct nodes with separate clocks to contribute to a single stream.

All logging meta-data will still be included in log records.
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

The default label set should be suitable for most deployments.
The user can configure a different set for specific needs.

The default label set is:

`log_type`: One of `application`, `infrastructure`, `audit`. Basic log categorization.

`cluster`: The cluster name, i.e. the DNS authority part of the cluster master URL.\
This name is DNS-unique, human readable, and known to all k8s API clients.
It is recommend as a cluster ID by [this discussion](https://github.com/kubernetes/kubernetes/issues/2292).
To print the name of the connected cluster:
```bash
oc config view -o jsonpath='{.clusters[].name}{"\n"}
```

`kubernetes.namespace_name`: namespace where the log originated.

`kubernetes.pod_name`: name of the pod where the log originated.

`kubernetes.host`: Host name of the cluster node where the log record originated.\
This is *always included* to guarantee ordered streams, even if the user configures a label set without it.

**Note:** `container_name` and `image_name` are *not* included in the defaults. They are not high-cardinality by themselves, but they are multipliers of `namespace/pod`, which is the highest cardinality we want to allow by default.

**Note:** We use names rather than UID for cluster, namespace and pod.
Names are more likely to be known and easier to use in queries.
Using both names *and* UIDs is redundant, they partition the data in approximately the same way.

UIDs (and all other meta-data) are still available in the log payload for filtering in queries.

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

- `labelKeys`: ([]string, default=_see [Default Loki Labels](#default-loki-labels))_ \
  A list of meta-data keys to replace the default labels.\
  Keys are translated to [label names][] as described in [Summary of Loki Labels](#summary-of-loki-labels)
  Example: `kubernetes.labels.foo` => `kubernetes_labels_foo`.\
  **Note**: `kubernetes.host` is *always* be included, even if not requested.
  It is required to ensure ordered label streams.
- `tenantKey`: (string, default=`"kubernetes.namespaceName"`) \
  Use the value of this meta-data key as the Loki tenant ID.\
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

I want logs from each namespace to be directed to separate Loki tenants, using Loki's "soft tenancy" model:

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

### Risks and Mitigation

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

Acceptable performance & passing stress tests.

### Upgrade / Downgrade Strategy

_**TODO** Migrating from Elasticsearch_

## Implementation History

None yet.

[label names]: https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
[data model]: https://github.com/openshift/origin-aggregated-logging/blob/master/docs/com.redhat.viaq-openshift-project.asciidoc
