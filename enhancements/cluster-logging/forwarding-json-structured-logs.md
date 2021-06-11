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

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement allows structured JSON log entries to be forwarded as JSON objects in JSON output records:

* Add an object field `structured` to log records, holds a JSON log entriy as an object.
* Extend `ClusterLogForwarder` input API to enable JSON parsing.
* Extend `ClusterLogForwarder` elasticsearch output API to direct structured logs to different indices.
* Replaces the [defunct MERGE_JSON_LOG][defunct_merge] feature.

For example, given this structured log entry:

``` json
{"level":"info","name":"fred","home":"bedrock"}
```

The current forwarded log record looks like this:

``` json
{"message":"{\"level\":\"info\",\"name\":\"fred\",\"home\":\"bedrock\"",
 "more fields..."}
```

The proposed new record with JSON parsing enabled looks like this:

``` json
{"structued": { "level": "info", "name": "fred", "home": "bedrock" },
 "more fields..."}
```

## Motivation

### Goals

* Direct access to JSON log entries as JSON objects
  * for `fluentdForward`, `Elasticsearch` or any other JSON-aware consumer.
* Elasticsearch outputs can direct logs with different JSON formats to different indices based on category, k8s label, or input selectors.
* Backwards compatible with today's implementation.

### Non-Goals

* Not general-purpose JSON queries and transformations.
* No improved flexibility of the ES output for external ES deployments.

## Proposal

### Data model

One top-level field added to the logging output record [data model][data_model]:

* `structured` (type object): Original structured JSON log entry.

Relationship between `structured` and `message` fields:

* Both `message` and `structured` fields *may* be present.
  * For first release, `message` is always present for backwards compatibility.
  * In future releases `message` may be removed when `structured` is present.
* If there is no structured data, the `structured` field will be missing or empty
* If `message` and `structured` are both present and non-empty, `message` MUST contain the JSON-quoted string equivalent of the `structured` value.

### ClusterLogForwarder configuration

New *pipeline* field:

* `parse`: (string, optional) Only legal value is "json" (there may be others in future).
   If set, attempt to parse log entries as JSON objects.

For each log entry; if `parse: json` is set _and_ the entry is valid JSON, the output record will include a `structured` field _equivalent_ to the JSON entry.
It may differ in field order and use of white-space.

### Output type elasticsearch

For most output types it is sufficient to enable `parse: json` to forward JSON data.
Elasticsearch is a special case; JSON records with _different formats_ must go to different indices, otherwise type conflicts and cardinality problems can occur.
The elasticsearch output can be configured with a "structured type" that is used to construct an index name.

New fields in  `output.elasticsearch`:
* `structuredTypeName`: (string, optional) the structured type, unless `structuredTypeKey` is set, and the key is present.
* `structuredTypeKey`: (string, optional) Use the value of this meta-data key (if present and non-empty)  as the structured type. These keys are supported:
  * `kubernetes.labels.<key>`: Use the string value of kubernetes label with key `<key>`.
  * `openshift.labels.<key>`: Use the string value of an openshift label with key `<key>` (see [forwarder-tagging.md](forwarder-tagging.md))

Notes:
* The Elasticsearch _index_ for structured records is formed by prepending "app-" to the structured type and appending "-write".
* Unstructured records are not sent to the structured index, they are indexed as usual in application, infrastructure or audit indices.
* If there is no non-empty structured type, forward an _unstructured_ record with no `structured` field.

It is important not to overload elasticsearch with too many indices.
Only use distinct structured types for distinct log _formats_, **not** for each application or namespace.
For example, most Apache applications use the same JSON log format and should use the same structured type, for example "LogApache".

Structured indices are created automatically by the managed default store.
In order to forward to an external Elasticsearch instance, indices must be created in advance.

### User Stories

#### I want to forward JSON logs to a remote destination that is not Elasticsearch

This example shows a remote `fluentd` but the same applies to any output other than Elasticsearch:

```yaml
outputs:
- name: myFluentd
  type: fluentdForward
  url: ...

pipelines:
- inputRefs: [ application ]
  outputRefs: myFluentd
  parse: json
```

#### I want to forward JSON logs to default Elasticsearch, using a k8s pod label to determine the structured type

For elasticsearch outputs, we must separate logs with different formats into different indices.
Lets assume that:

* Applications log in two structured JSON formats called "apache" and "google".
* User labels pods using those formats with `logFormat=apache` or `logFormat=google`

With the following forwarder configuration:

```yaml
outputDefaults:
- elasticsearch:
    structuredTypeKey: kubernetes.labels.logFormat
pipelines:
- inputRefs: [ application ]
  outputRefs: default
  parse: json
```

This structured log record will go to the index `app-apache-write`:

```json
{
  "structured":{"name":"fred","home":"bedrock"},
  "kubernetes":{"labels":{"logFormat": "apache", ...}}
!}
```

This structured log record will go to the index `app-google-write`:

```json
{
  "structured":{"name":"wilma","home":"bedrock"},
  "kubernetes":{"labels":{"logFormat": "google", ...}}
}
```

**Note**: Only _structured_ logs with a `logForward` label go to the `logForward` index.
All others go to the default application index as _unstructured_ records, including:

* Records with missing or empty `logFormat` label.
* Records that could not be parsed as JSON,  _even if_ they have a `logFormat` label.

#### I want a replacement for the defunct MERGE_JSON_LOG feature

Setting `parse: json` provides all the information formerly available via [MERGE_JSON_LOG][defunct_merge], but in a slightly different format.
For example a log entry field `name` is available `structured.name` in the forwarded records.

### Implementation Details

#### Fluentd parse rules

The `parse: json` attribute on a pipeline implies that each fluentd source feeding that pipeline must have JSON parsing enabled.
The attribute is not directly on the forwarder `input` because

* It makes it easier to apply JSON parsing to the default input types.
* It separates the logical roles of input (selecting log sources) and pipeline (processing logs records).

If a single input feeds structured and non-structured pipelines, use duplicate inputs or intermediate labels to get the correct behaviour on each.

#### Elasticsearch index creation

The managed Elasticsearch instance must create structured indices on-demand from the record key: `elasticsearch.index`
The forwarder prepends "app-" and appends "-write" to the structured type name, so that the index name follows managed Elasticsearch indexing rules.
An external unmanaged Elasticsearch must follow the same index naming pattern, and must pre-create or dynamically create indices.

**Note**: It is up to the user to ensure that logs sent to an index are consistent, otherwise index explosion and type confusion are still possible.

### Risks and Mitigations

Risks:
* Changes to data model and output format may impact users.
* Data model may have other issues to be addressed.

Mitigation:
* Data model changes are additive to avoid compatibility problems.
* Need full review of data model.

## Design Details

### Test Plan

* Unit/functional tests: verify forwarding to fluentd
* E2E tests: veryfy forwarding to Elasticsearch
* Integration tests with Elasticsearch

### Graduation Criteria

#### GA
* Fully tested
### Open Questions

* Need sync the [Elasticsearch pipeline proposal][es-pipeline] with this document, update both if needed.
* Exact requirements for flattening and de-dottting with existing ES. See [this PR comment](https://github.com/openshift/enhancements/pull/518#issuecomment-749564743)

#### Examples

See User Stories.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Both CLO and ELO must be upgraded before using the new features.

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
