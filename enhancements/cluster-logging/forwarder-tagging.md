---
title: Forwarder Adds Labels to Outbound Messages
authors:
  - "@alanconway"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-06-01
status: provisional
see-also:
replaces:
superseded-by:
---

# Forwarder Adds Labels to Outbound Messages

## Release Signoff Checklist

- [X] Enhancement is implementable
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Labels are name:value pairs that can be attached to log messages.  Extend the
`ClusterLogForwarder` API so that it can attach a fixed set of labels to
outbound log messages.  For example: add a `datacenter-id` field to messages
forwarded to another data center.

Labels are associated with a `pipeline`, so different labels can be added based
on the `input` and `output` of the log message.  For example `audit` logs can be
labeled differently from `application` logs.

These labels are set by the _cluster administrator_, and apply regardless of
which application produced the logs.  To attach labels to logs from a specific
pod, namespace or application use kubernetes labels, which are also forwarded
with the log message.

## Motivation

### Goals

* Cluster admin can label outgoing messages at the forwarder.
* Set fixed labels based on `output` destination.
* Set fixed labels based on `input` origin, allow use of all present and future `input` filtering features.

### Non-Goals

* Labels are set at configuration, not computed based on message content.
* Does not replace or interfere with other labelling mechanisms. In particular, kubernetes labels are attached to messages as before.

## Proposal

Add a new field to the `ClusterLogForwarder` API: `spec.pipeline.labels`.
This is a map of name:value pairs to apply to each log record passing through the pipeline.

Labels are added to the normalized JSON record as a map field named `openshift.labels`, for example.

```json
{
  "message" : "2020-03-03 11:44:51,996 - SVTLogger - INFO",
  "@timestamp" : "2020-03-03T11:44:51.996384+00:00",
  "level" : "unknown",
  "hostname" : "ip-10-0-153-186.us-east-2.compute.internal",

  "openshift" : {
    "labels" : {
      "favouriteColor" : "blue",
      "datacenterId": "nunavik-12"
    }
  }

  "kubernetes" : {
    "labels" : {
      "run" : "myproject",
      "test" : "myproject"
    },
    "container_name" : "myproject",
    "namespace_name" : "logflatx",
    "pod_name" : "myproject-987vr",
    "container_image" : "docker.io/mffiedler/ocp-logtest:latest",
    "container_image_id" : "docker.io/mffi….,
    "pod_id" : "67667d28-13fe-4c89-aa44-06936279c399",
    "host" : "ip-10-0-153-186.us-east-2.compute.internal",
    "master_url" : "https://kubernetes.default.svc",
    "namespace_id" : "e8fb5826-94f7-48a6-ae92-354e4b779008"
  },
  "docker" : {
    "container_id" : "a2e6d10494f396a45e4a6e8f782a571cb7759cd45a3555c386976a1a9c62cf7c"
  },
}
```

### Alternate encodings for labels (stretch goal)

The output type may give additional options for encoding labels, for example:

```text
  - output
      type: syslog
      syslog:
        structuredData: [openshift.labels, kubernetes.labels]
        payloadKey: message
 ```

This encodes logging and kubernetes labels as STRUCTURED-DATA sections ([RFC 5424](https://tools.ietf.org/html/rfc5424#section-6.3)) with SD-ID `openshift.labels@2312` and `kubernetes.labels@2312`. The syslog payload is the unmodified original log string.

Note 2312 is the [IANA assigned enterprise number](https://www.iana.org/assignments/enterprise-numbers/enterprise-numbers) for Red Hat,
as required by [RFC 5424](https://tools.ietf.org/html/rfc5424#section-6.3.2)

### User Stories

#### As cluster admin I want to label all outgoing messages with a cluster-id

```text
pipeline:
  labels: { clusterId: C1234 }
  inputRefs: [application, infrastructure, audit]
  outputRefs: [somewhere]
```

#### As cluster admin I want to label each message type differently, but send all to the same output

```text
pipeline:
  labels: { logType: normal }
  inputRefs: [application]
  outputRefs: [somewhere]
pipeline:
  labels: { logType: special }
  inputRefs: [infrastructure]
  outputRefs: [somewhere]
pipeline:
  labels: { logType: secret }
  inputRefs: [audit]
  outputRefs: [somewhere]
```

### Risks and Mitigations

Risk: Excessive use of labels may bloat outgoing log traffic.

Mitigation: Cluster admins control use, with great power comes great responsibility.

## Design Details

Add `labels` map to the `pipeline` spec, each name-value pair from the labels
map is added as a label to each log message. Forwarder generates `fluentd`
configuration to add labels to each record going through the pipeline.

### Fluentd configuration examples

Example of generated fluentd configuration
```text
<label @SOME_PIPELINE>
  <filter **>
    @type record_transformer
    <record>
      openshift { "labels": { "foo": "bar" } }
    </record>
  </filter>
  ...
</label>
```

### Test Plan

- Unit tests using local log receivers.
- e2e test using a supported log receiver (e.g. syslog)

Test cases include:
- Fan in from multiple pipelines to a single output.
- Fan out from a single pipeline to multiple outputs.
- Forwarding all messages vs. from selected inputs.

### Graduation Criteria

TBD

### Upgrade / Downgrade Strategy

Additive feature:
- No impact on existing Forwarder CRs on upgrade
- Cannot downgrade a Forwarder CR that uses the feature to a version that does not support it.

### Version Skew Strategy

No change to existing Forwarder stragegy.

## Implementation History

TBD

## Drawbacks

Complicates the forwarder.

## Alternatives

None.

## Infrastructure Needed

Nothing new.
