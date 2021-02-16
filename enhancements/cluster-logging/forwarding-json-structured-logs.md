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

When applications write structured JSON logs, consumers want to access fields of JSON log entries for indexing and other purposes.
The current logging [data model][data_model] stores the log entry as a JSON *string*, not a JSON *object*.
Consumers can't access the log entry fields without a second JSON parse of this string.

The current implementation also 'flattens' labels to work around Elasticsearch limitations.

To illustrate, given a log entry `{"name":"fred","home":"bedrock"}` from a container with the label `app.kubernetes.io/name="flintstones"`. The current output record is:

```json
{
  "message":"{\"name\":\"fred\",\"home\":\"bedrock\"}",
  "kubernetes":{"labels":["app_kubernetes_io/name=flintstones", ...}},
  ... other metadata
}
```

This proposal will allow alternate structured forms of output record:

1. Enhanced record, compliant with the logging [data model][data_model]
```json
{
  "structured":{
    "name":"fred",
    "home":"bedrock",
  }",
  "kubernetes":{"labels":{"app.kubernetes.io/name": "flintstones", ...}},
  ... other metadata
}
```

2. Original record with no meta-data:
```json
{
  "name":"fred",
  "home":"bedrock",
}
```

This proposal describes

* extensions to the [data model][data_model]
* extensions to the `ClusterLogForwarder` API to configure JSON parsing and forwarding.
* how existing and future Elasticsearch stores can index structured records.
* a replacement for the [defunct MERGE_JSON_LOG][defunct_merge] feature.

**Note**: This proposal focuses on JSON, but the data model and API changes can also apply to structured encodings supported in future.

## Terminology

Logging terminology can have overlapping meanings, this document uses the following terms with specific meanings:

- *Consumer*: A destination for logs, identified by a forwarder `output`.
  The default consumer is the *Elasticsearch Log Store*
- *Entry*: a single log entry, usually a single line of text. May be JSON or other format.
- *Structured Log*: a log where each *entry* is formatted as a *JSON object*.
- *Record*: A key-value record including the *entry* and meta-data collected by the logging system.
  - *Internal record*: abstract record manipulated inside the forwarder.
  - *Output record*: concrete record encoded by an `ouput` for a consumer. Encodings include JSON and Rsyslog.
- *Schema*: Defines key names and value types in a structured entry.
   The forwarder associates *schema names* with logs, it does not store or process user schema.
- *Data Model*: The [schema][data_model] for the forwarder's own *record*.

## Motivation

### Goals

* Direct access to JSON log entries as JSON objects in log records for `fluentdForward`, `Elasticsearch` or any other JSON-parsing consumer.
* User can associate schema names with logs by category, namespace, application label, or any future forwarder `input` selection criteria.
* Elasticsearch indexing of JSON logs with multiple schema without index explosions.
* Upgrade path from today's implementation.

### Non-Goals

* General-purpose JSON queries and transformations.
* Recording or validating user-defined schema.
  The forwarder only identifies schema by name, all schema knowledge belongs to the user.

## Proposal

### Data model

Two new top-level fields added to the record [data model][data_model]:

* `structured` (type object): Original structured JSON log entry.
* `schema` (type string): Schema name assigned by user.

Relationship between `structured` and `message` fields:

* EXACTLY ONE of `message` or `structured` MUST be present.
* `schema` is OPTIONAL, defined and interpreted by the user.
* The presence or absence of the `structured` and `message` fields can be used to detect structured/unstructured records.

With an *Elasticsearch* store, `schema` is copied to the existing `viaq_index_name` field to set the index.
`viaq_index_name` will continue to be used for compatibility and upgrade, but will eventually be removed.

#### ClusterLogForwarder configuration

The elements of the CLF API are:

- *Input*: Selects logs, adds meta-data, creates _internal_ records.
- *Pipeline*: Transforms, filters and routes internal records.
- *Output*: Connects to consumers, encodes and sends _output_ records.

New fields:

`input.stuctured`: (string, default "JSON") Attempt to parse structured entries.
The only legal value is "JSON", others may be added in future.

An input record contains:

* `structured` field if `structured=JSON` *and* JSON parse succeeds.
* `message` field if `input.structured` is absent *or* JSON parse fails.

`input.schema`: (string, optional) User-defined schema name.
Adds a `schema` field to the internal record with a user-defined value.
Not used by the forwarder, included in output record for the consumer.

`pipeline.content`: (enum, default="Enhanced") Content type for messages

* "Enhanced":
  - if log entry parsed as JSON: meta-data + `strutured` object field.
  - else: meta-data + `message` string field.
* "Original": Original message string, no meta-data
  - if log entry parsed as JSON: use original log entry as output record, no meta-data.
  - else: `message` string field only, no meta-data
* "Unstructured": meta-data + `message` string field, regardless of JSON parse.

### User Stories

#### I want a replacement for the defunct MERGE_JSON_LOG feature

The "Structured" content format provides all the information formerly available via
[MERGE_JSON_LOG][defunct_merge]. Log entry field `x` is available as `.structured.x`,
instead of just `.x`

**NOTE**: We could provide a "Merge" content format to exactly reproduce the [MERGE_JSON_LOG][defunct_merge] feature.
We didn't do so because mixing [logging data-model][data-model] fields with user-defined fields can cause name clashes and invalid messages.
We could revisit this if there is sufficient user demand, but it would be at the user's risk.

```yaml
inputs:
- name: InputJSON
  application: {}
  structured: JSON
outputs:
- name: Output
  type: fluentdForward
pipelines:
- inputRefs: [ InputJSON ]
  outputRefs: [ Output ]
```

#### I want to index logs from applications with different schema in Elasticsearch

Imagine there are two web server implementations named "patchy" and "bulldog".
Each type of web server produces structured logs in a consistent format, but each uses a different format.
A hypothetical admin deploys both patchy and bulldog servers.

In both examples below, Elasticsearch will create separate indices for each schema "patchy and "bulldog".
See the [Elasticsearch notes](#es-notes) for more details.

**Note**: If there is only one JSON schema, the `schema` field is not necessary.

##### Example 1: Identify applications by namespace

Pods in namespaces "fred" and "bob" use the "patchy" schema, pods in namespaces "jill" and "jane" use the "bulldog" schema.

**Note**: the `namespaces` selector below is already implemented by the CLF.

```yaml
inputs:
- name: InputPatchy
  application:
    namespaces: [ fred, bob ]
  structured: JSON
  schema: patchy
- name: InputBulldog
  application:
    namespaces: [ jill, jane ]
  structured: JSON
  schema: bulldog
pipeline:
- inputRefs: [ InputPatchy, InputBulldog ]
  outputRefs: default
```

##### Example 2: Identify applications by labels

The applications using each schema must be identified by k8s labels.
Note this is often _already done_ as part of a Deployment.
Deployments usually label pods by the application they run, which determines their log format.

**Note**: the label `selector` below is defined by the [forwarder label selectors proposal][clf-labels]

```yaml
inputs:
- name: InputPatchy
  applications:
    selector:
      matchLabels: { app: patchy }
  structured: JSON
  schema: patchy
- name: InputBulldog
  applications:
    selector:
      matchLabels: { app: bulldog }
  structured: JSON
  schema: bulldog
pipelines:
- inputRefs: [ InputPatchy, InputBulldog ]
  outputRefs: default
```

### Implementation Details

#### Elasticsearch version-specific workarounds

The existing Elasticsearch logging store requires "flattening" of the `kubernetes.labels` map:

* Transform key:value object into array of "NAME=VALUE" strings.
* Replace '.' with '_' in label names.

This makes label keys difficult to use for a consumer.
There is no automatic way to reverse the process since '.' and '_' are both legal characters in label names.

The new forwarder will check the version of the Elasticsearch Operator (via labels or annotations, TBD).
Flattening labels will be enabled if the ELO is identified as a 'legacy' version.
Other output types, and future ELO versions based on the [pipeline proposal][es-pipeline] will receive labels unmodified.

#### Elasticsearch Indexing <a name="es-notes"></a>

To be efficient, Elasticsearch dynamic indexing needs records to contain a bounded set of key names with consistent value types.
Inconsistent types for the same key cause type mismatch errors.
Too many key names (including recursive keys in sub-objects) cause an "index explosion" and performance problems.
This leads to problems when inconsistent JSON records are stored in the same index.

The current data model uses `viaq_index_name` to select the ES index.
If `schema` is set, it is copied to `viaq_index_name` instead of a default index name.
Elasticsearch will then create a separate index for each schema.

Provided log entries are consistent with some fixed schema, it will be possible to search/index custom JSON messages in the *existing* Elasticsearch store implementation.

**Note**: It is up to the user to ensure that logs identified with a schema actually are consistent, otherwise index explosion and type confusion are still possible.

With the *new* [Elasticsearch pipeline proposal][es-pipeline] the store will pre-process records and limit indexing to be more robust.
* Safe handling of `kubernetes.labels`, so forwarder can turn off flattening/de-dotting behavior.
* Safe handling of `structured`, default limit on indexing depth to avoid problems with deeply nested JSON.
* Other possible optimizations and checks, outlined in [Elasticsearch pipeline proposal][es-pipeline]
* Eventually remove `viaq_index_name` and use `schema` directly.

The CLO will check the version of ELO that is deployed automatically configure the `default` output appropriately.

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
* Should we provide a "Merge" content format to exactly reproduce the [MERGE_JSON_LOG][defunct_merge] feature?

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
