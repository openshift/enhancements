---
title: forwarding-json-structured-logs

authors:
  - "@alanconway"

reviewers:
  - "@ewolinetz"
  - "@lukas-vlcek"
  - "@jcantril"
  - "@sichvoge"

approvers:
  - "@jcantril"

creation-date: 2020-10-27

status: provisional

see-also:
  - [Elasticsearch Pipeline Processing](cluster-logging-es-pipeline-processing.md)
  - [Forwarder Label Selector](forwarder-label-selector.md)
---

# Forwarding JSON Structured Logs

## Release Sign-off Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement will allow structured JSON log entries to be forwarded as JSON objects in JSON output records.

The current logging [data model][data_model] stores the log entry as a JSON *string*, not a JSON *object*.
Consumers can't access the log entry fields without a second JSON parse of this string.

The current implementation also 'flattens' labels to work around Elasticsearch limitations.

To illustrate, given a log entry `{"name":"fred","home":"bedrock"}` from a container with the label `app.kubernetes.io/name="flintstones"`.
The current output record looks like:

```json
{
  "message":"{\"name\":\"fred\",\"home\":\"bedrock\"}",
  "kubernetes":{"flat_labels":["app_kubernetes_io/name=flintstones", ...]},
  ... other metadata
}
```

This proposal enables an alternate form of output record including a `structured` object field for JSON log entries.
For Elasticsearch outputs, new configuration is added to determine the index to use.

```json
{
  "structured":{
    "name":"fred",
    "home":"bedrock",
  }",
  "kubernetes":{"labels":{"app.kubernetes.io/name": "flintstones", ...}},
  ...
}
```

This proposal describes

* extensions to the logging [data model][data_model] - `structured` field.
* extensions to the `ClusterLogForwarder` API to configure JSON parsing and forwarding.
* extensions to the `ClusterLogForwarder` API to select an index.
* indexing structured records in current and future Elasticsearch stores.
* replacing the [defunct MERGE_JSON_LOG][defunct_merge] feature.

**Note**: This proposal focuses on JSON, but the data model and API changes can apply to other structured formats that may be supported in future.

## Terminology

Logging terminology can have overlapping meanings, this document uses the following terms with specific meanings:

- *Consumer*: A destination for logs, identified by a forwarder `output`.
  The default consumer is the *Elasticsearch Log Store*
- *Entry*: a single log entry, usually a single line of text. May be JSON or other format.
- *Structured Log*: a log where each *entry* is formatted as a *JSON object*.
- *Record*: A key-value record including the *entry* and meta-data collected by the logging system.
- *Data Model*: [Description][data_model] of the forwarder's own *record* format.

Elasticsearch *Index*: an index receives a stream of logs with similar JSON format.

## Motivation

### Goals

* Direct access to JSON log entries as JSON objects for `fluentdForward`, `Elasticsearch` or any other JSON-aware consumer.
* Elasticsearch outputs can associate index names with log records by category, namespace, k8s label, or`input` selection criteria.
* Upgrade path from today's implementation.

### Non-Goals

* General-purpose JSON queries and transformations.

## Proposal

### Data model

One new top-level fields added to the logging output record [data model][data_model]:

* `structured` (type object): Original structured JSON log entry.

Relationship between `structured` and `message` fields:

* `message` field is always present for backwards compatibility (a future proposal will address filtering out unwanted fields in the record)
* Both `message` and `structured` fields may be present.
* If there is no structured data, the `structured` field will be missing or empty
* If `message` and `structured` are both present and non-empty, `message` MUST contain the JSON-quoted string equivalent of the `structured` value.

*Elasticsearch* outputs MAY specify special index names for records, see below.

#### ClusterLogForwarder configuration

New *pipeline* field:

* `parse`: (string, optional) Only legal value is "json" (there may be others in future).
   If set, attempt to parse log entries as JSON objects.

For each log entry; if `parse: json` is set _and_ the entry is valid JSON, the output record will include a `structured` field _equivalent_ to the JSON entry.
It may differ in field order and use of white-space.

All record will have a `message` field for backwards compatibility.

#### Output  configuration for type:elasticsearch

New fields for output type `elasticsearch`
* `indexKey`: (string, optional) Use value of meta-data key as `index` value, if present.
  These keys are supported:
  - `kubernetes.namespaceName`: Use the namespace name as the index name.
  - `kubernetes.labels.<key>`: Use the string value of kubernetes label with key `<key>`.
  - `openshift.labels.<key>`: Use the string value of an openshift label with key `<key>` (see [](forwarder-tagging.md))
  - *other keys may be added in future*
* `indexName`: (string, optional)
  * If `indexKey` is not set, or the key is missing, use `indexName` as the `index` value.
  * If `indexKey` is set and the key is present, that takes precedence over `indexName`

**Note:** The Elasticsearch output will _delete_ the `structured` field if the rules above do not yield a non-empty index name.
This is to avoid sending randomly structured JSON to shared indices.

Indices are created automatically on-demand by co-operation between the forwarder and the managed default store.

#### Default output configuration

To allow index settings on the default elasticsearch output, there is a new section in the ClusterLogForwarder spec:

* outputDefaults (type map, optional):
  Each map under outputDefaults MUST have a `type` field.
  All other fields are used as defaults for any outputs of the same `type` _including_ the `default` output

### User Stories

#### I want the  default Elasticsearch to index by k8s label on the source pod

```yaml
outputDefaults:
- type: elasticsearch
  indexKey: kubernetes.labels.myIndex

pipelines:
- inputRefs: [ application ]
  outputRefs: default
  structured: json
```

For example, this log record will go to the index `flintstones`:

```json
{
  "structured":{
    "name":"fred",
    "home":"bedrock",
  }",
  "kubernetes":{"labels":{"myIndex": "flintstones", ...}},
  ...
}
```

Logs from pods with no `myIndex` label will go to the `app` index like any unstructured `message` record.

```json
{
  "message":"{\"name\":\"fred\",\"home\":\"bedrock\"}",
  ... other metadata
}
```

#### I want input selector to set the elasticsearch index

Pipelines can set an `openshift` label on records from input selectors.
This is used the same way as the kubernetes label in the previous example:

```yaml
outputDefaults:
- type: elasticsearch
  indexKey: openshift.labels.myIndex

inputs:
- name: InputItchy
  application:
    namespaces: [ fred, bob ]
- name: InputScratchy
  application:
    namespaces: [ jill, jane ]

pipeline:
- inputRefs: [ InputItchy ]
  outputRefs: [ default ]
  structured: json
  labels: { myIndex: itchy }
- inputRefs: [ InputScratchy ]
  outputRefs: [ default ]
  structured: json
  labels: {myIndex: scratchy }
```

Would produce records like:

```json
{
  "structured":{
    "name":"fred",
    "home":"bedrock",
  }",
  "openshift": {"labels":{ "myIndex":"itchy" } }
  "kubernetes": {"namespace_name":"fred", ...}},
  ...
}

{
  "structured":{
    "name":"fred",
    "home":"bedrock",
  }",
  "openshift": {"labels":{"myIndex":"scratchy" } }
  "kubernetes":{"namespace_name":"jill", ...},
  ...
}
```

#### I want a replacement for the defunct MERGE_JSON_LOG feature

Setting `parse: json` provides all the information formerly available via [MERGE_JSON_LOG][defunct_merge].

```yaml
outputs:
- name: OutputJSON
  type: fluentdForward
pipelines:
- inputRefs: [ application ]
  outputRefs: [ OutputJSON ]
  structured: json
```

Would produce records like:

```yaml
{
  "structured":{
    "name":"fred",
    "home":"bedrock",
  }",
  ...
}
```

**Note**:

* *Not a drop-in replacement* - log entry field `home` is available `structured.home`, not just `home`
* With the Elasticsearch store you should not forward all JSON logs to the same index, see the other use cases.

### Implementation Details

#### Fluentd parse rules

The `parse: json` attribute on a pipeline implies that each fluentd source feeding that pipeline must have JSON parsing enabled.
The attribute is not directly on the forwarder `input` because

* It makes it easier to apply JSON parsing to the default input types.
* It separates the logical roles of input (selecting log sources) and pipeline (processing logs records).

If a single input feeds structured and non-structured pipelines, use duplicate inputs or intermediate labels to get the correct behaviour on each.

#### Elasticsearch output rules

The Elasticsearch output needs to take extra measures:

##### Flattening the `kubernetes.labels` map

* Transform key:value object into array of "NAME=VALUE" strings.
* Replace '.' with '_' in label names.
* Use field name `flat_labels` instead of `labels`

This makes label keys difficult to use for a consumer.
There is no automatic way to reverse the process since '.' and '_' are both legal characters in label names.

The new forwarder will check the version of the Elasticsearch Operator (via labels or annotations, TBD).
Flattening to `flat_labels` will be enabled if the ELO is identified as a 'legacy' version.
Other output types, and future ELO versions based on the [pipeline proposal][es-pipeline] will receive a field named `labels` with the unmodified labels.

##### Elasticsearch Indexing <a name="es-notes"></a>

The current implementation relies on a fixed set of rollover indices and aliases being set up on Elasticsearch.

This proposal requires we dynamically create rollover aliases and indices on-demand.

This can be done from fluentd using the viaq plug-in, or in an Elsaticsearch pipeline.
We should implement this in fluetnd as part of implementing this proposal:

- Creating from fluentd is more robust to upgrades, it will work with existing Elasticsearch deployments.
- Longer term Elasticsearch should take this on, but that depends on the future plans for Elasticsearch.

This decision may be reviewed based on changes to Elasticsearch capabilities and timing of releases.

If index creation is handled by the forwarder, then this proposal should work with the *existing* Elasticsearch store implementation.

**Note**: It is up to the user to ensure that logs sent to an index are consistent, otherwise index explosion and type confusion are still possible.

With the *new* [Elasticsearch pipeline proposal][es-pipeline] the store will pre-process records and limit indexing to be more robust.
* Safe handling of `kubernetes.labels`, so forwarder can turn off flattening/de-dotting behavior.
* Safe handling of `structured`, default limit on indexing depth to avoid problems with deeply nested JSON.
* Other possible optimizations and checks, outlined in [Elasticsearch pipeline proposal][es-pipeline]

The CLO should check the version of ELO that is deployed and automatically configure the `default` output appropriately.

### Risks and Mitigation

Risks:
* Changes to data model and output format may impact users.
* Data model may have other issues to be addressed.

Mitigation:
* Data model changes are additive to avoid compatibility problems.
* Need full review of data model.

## Design Details

### Open Questions

* Need sync the [Elasticsearch pipeline proposal][es-pipeline] with this document, update both if needed.
* Exact requirements for flattening and de-dottting with existing ES. See [this PR comment](https://github.com/openshift/enhancements/pull/518#issuecomment-749564743)
* Sidecar injection: multi-container pods, especially those with injected sidecar images,
  may have different log formats for each container.
* Future enhancement proposals may add more capabilities to edit/filter a structured record for output.

#### Examples

See User Stories.

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

CLO and ELO can be upgraded separately, in any order.
The CLO checks the ELO version and configures its `default` output accordingly.

CLO first:
1. Upgrade CLO: recognizes old version of ELO, runs in 'legacy' described [above](#es-notes).
2. Upgrade ELO: CLO recognizes new version of ELO, disables legacy workarounds.

ELO first:
1. Upgrade ELO: CLO continues to send old format records, new ELO is backwards compatible, no change in ELO indexing.
2. Upgrade CLO: New CLO sends structured records, new ELO knows how to index them. Described [above](#es-notes).

See [Elasticsearch Implementation Notes](#es-notes)

### Version Skew Strategy

## Implementation History

## Drawbacks

* Additional complexity in the API.
* Additional implementation complexity.

## Alternatives

Unmanaged mode.

## References

[es-pipeline]: ./cluster-logging-es-pipeline-processing.md "Elasticsearch Pipeline Processing"
[clf-labels]: ./forwarder-label-selector.md "Forwarder Label Selector"
[es-issue]: https://github.com/openshift/origin-aggregated-logging/issues/1492
[defunct_merge]: https://docs.openshift.com/container-platform/4.1/logging/config/efk-logging-fluentd.html#efk-logging-fluentd-json_efk-logging-fluentd "Defunct `MERGE_JSON_LOG` feature"
[data_model]: https://github.com/openshift/origin-aggregated-logging/blob/master/docs/com.redhat.viaq-openshift-project.asciidoc
[k8s_labels]: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels
[recommended_labels]: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/ "Recommended k8s labels"
[jsonpath]: https://goessner.net/articles/JsonPath/
