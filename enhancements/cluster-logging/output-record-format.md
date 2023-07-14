---
title: output-record-format
authors:
  - "@alanconway"
reviewers:
  - "@jatinsu"
  - "@jcantril"
approvers:
  - "@jcantril"
api-approvers:
  - "@jcantril"
creation-date: 2023-07-14
last-updated: 2023-07-14
tracking-link:
  - "https://issues.redhat.com/browse/LOG-2827"
see-also:
replaces:
superseded-by:
---

# Output Record Format

## Summary

The logging operator produces records using the "ViaQ" data model and labelling scheme.
Users requesting more flexibility in the output format:

- Reduced log size by excluding some metadata fields.
- Alternate data models, for example OpenTelemetry.
- Alternate encodings, for example forwarding original log text without additional meta-data.
  In future other encodings might be requested, for example GRPC protobuf.

This enhancement proposes API extensions to configure these options.

## Motivation

### User Stories

- As a user I want to reduced log size by excluding unwanated metadata fields.
- As a user I want to send logs to systems that use an alternate data model, e.g. OpenTelemetry.
- As a user I want to send logs to systems that use an alternate encoding, e.g. GRPC.

### Goals

- Reduce log size by removing unwanted metadata.
- Alternate data models such as open telemetry.
- Alternate encodings such as raw text (and future options like protobuf)

### Non-Goals

- Arbitrary transformations of log data and fields.
- User-defined data models or schema -  only pre-defined schema are supported.

## Proposal

### Workflow Description

- Logging administrator configures output format fields on the ClusterLogForwarder.output spec.
- Logs are generated in the configured format for each output.

### API Extensions

Example ClusterLogForwarder

``` yaml
spec:
  outputs:
    - name: SendOTEL
	  encoding: JSON
      schema: OpenTelemetry
      fields: [Standard, -k8s.pod.labels, -k8s.namespace.labels]
```

The above example requests _JSON_ encoding using the _OpenTelemetry_ data model.
Records will include a standard set of fields, but will exclude pod and namepsace labels.
#### New Fields
##### encoding (enum, default "JSON")

Name of an encoding, the initial enum values are:
- JSON: Encode each log record as a JSON object.
- None: Forward the _original unmodified log text_. There is no added metadata or encoding.
  Note that `schema` and `fields` are ignored with  `encoding: None`.

In future, other encodings _may_ be added, for example:
- Protobuf: Binary GRPC protobuf encoding.
- Syslog: to encode log fields as syslog USER-DATA sections.

##### schema (enum, default "ViaQ")

Name of a schema or data-model, initial enum values:
- ViaQ: The existing ViaQ encoding used by the logging operator.
- OpenTelemetry: Use the [OpenTelemetry semantic conventions]

[OpenTelemetry semantic conventions]: https://opentelemetry.io/docs/specs/semconv/

##### fields (array, default ["All"])

Elements of the `fields` array can be:
- Path name of a field to include, e.g. `k8s.pod.name` or `kubernetes.pod_name`.
  Path names use the naming conventions selected by the `schema` field
- Path name of a field to exclude, preceeded by the `-` character. E.g. `-kubernetes.namespace_labels`
- One of the group names `Minimal`, `Standard`, `All` (capitalized)
  - Minimal: Selects the minimum useful set of fields.
  - Standard: Selects the recommended set of fields for most applications.
  - All: Selects all available fields.

A field is selected if it is listed or is a member of a listed group AND
it is not listed with a preceding `-` character.

### Risks and Mitigations

- Allows the user to exclude meta-data that may subsequently hinder debugging.
- Changes to output format may cause confusion in downstream log destinations.
- The openshift console will only work with the default `{encoding: json, schema: ViaQ}`
  It _may_ work with reduced field sets, but some features may be disabled (needs testing)
- Existing Elasticsearch stores will only work with defaults (reduced field sets need testing)
- Some log types (API audit and Event logs) are already encoded differently

### Drawbacks

See Risks.

## Design Details

### Open Questions

- Need to define the appropriate fields for Minimal and Standard groups.
- TODO define the interaction with existing JSON structured logs and the `parse: JSON` feature.
  Note special encoding of API audit and k8s Event logs, bring this into a standard form.

### Test Plan
TODO

### Graduation Criteria
TODO

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature
None.

### Upgrade / Downgrade Strategy
None special.

### Version Skew Strategy
None special.

### Operational Aspects of API Extensions

None. Minor extension of existing CRD.

#### Failure Modes
None special.

#### Support Procedures
None special.

## Implementation History
None.

## Alternatives
None.
