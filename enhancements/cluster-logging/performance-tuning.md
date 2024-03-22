---
title: performance-tuning
authors:
  - "@alanconway"
reviewers:
  - "@jcantrill"
approvers:
  - "@jcantrill"
api-approvers:
  - "@jcantrill"
creation-date: 2023-12-15
last-updated: 2023-12-15
tracking-link:
  - https://issues.redhat.com/browse/OBSDA-549
see-also:
replaces:
superseded-by:
---

# Performance tuning

## Summary

A _performance tuning_ API to control performance, reliability and special protocol features of an output,
without exposing the complexity of the underlying collector configuration.

**Note**
- Only vector is be supported initial, there are no current plans to back-port to fluentd.
- Existing `output[].limits` rate limiting feature is separate from this proposal. The implementations may interact.

## Motivation

Performance and reliability tuning in the underlying collector configuration is complex and error prone.
We want to expose a sufficient set of controls for realistic use-cases,
but we don't want to expose the full configuration surface of Vector.

### User Stories

#### As a cluster logging administrator I want to minimize log loss.

``` yaml
outputs:
 new - name: minimize-log-loss
   tuning:
     delivery: AtLeastOnce
```

#### As a cluster logging administrator I want to maximize throughput.

``` yaml
outputs:
  - name: maximize-throughput
    tuning:
      delivery: AtMostOnce
```

#### As a cluster logging administrator I want to tweak features of a specific output.

``` yaml
outputs:
  - name: detailed-tweaks
    tuning:
      compression: "zlib"     # Enable protocol-specific compression.
      maxWrite: 10Mi          # Limit max size of data in a single remote write request.
      minRetryDuration: 100ms # Fast initial retry on connection error.
```

### Goals

- Accommodate users with different performance and reliability trade-offs.
- Allow users to tune output parameters that we cannot optimize automatically.
- Expose a simple, general purpose, collector-neutral tuning API
- Use vector's end-to-end acknowledgements for more efficient reliability.

### Non-Goals

- Not a full end-to-end delivery guarantee, we are limited by reliability of source and sink.
- Not exposing underlying vector configuration details.
- No rate limiting - already provided by `outputs[].limit` field.
- No plans to support fluentd.

## Proposal

### Workflow Description

**logging administrator** is a human user responsible for setting output tuning parameters as needed.
The logging administrator needs to tune log collection performance and reliability to suit local requirements.

### API Extensions

New field `ClusterLogForwarder.output[].tuning` with the following sub-fields:

#### delivery: AtMostOnce|AtLeastOnce|AtLeastOncePersistent

##### `AtMostOnce`

Logs may be lost if the collector restarts or other faults occur.
Logs will not be duplicated _by the collector_.
They may be duplicated in protocol exchanges with the source or sink.
This mode does no persistent storage and no acknowledgement book-keeping, so has the lowest CPU and disk use.

Use `AtMostOnce` when:
- It is acceptable to lose some logs due to restarts or faults (network outage, remote store failures)
- It is important to minimize memory, CPU, disk and network costs.
- The logging system is expected to run near capacity and is likely to be overloaded.

##### `AtLeastOnce`

Logs _read by the collector_ will not be lost _by the collector_ due to restarts or faults.
Logs may still be lost
- Before the collector reads them: the collector cannot keep up with incoming log rate, e.g. log file rotation rate.
- After the collector sends them: if the output protocol is unreliable, or the target store can drop logs.
- If rate limiting is enabled: logs may be dropped by the collector to enforce rate limits.

Use `AtLeastOnce` when:
- It is important to avoid log loss due to restarts or failures.
- The collector is properly resourced (memory, CPU, disk) for the maximum log throughput at expected peak loads.
- Log rates are not expected to exceed the collection rate in normal operation.

`AtLeastOnce` uses end-to-end acknowledgements without persistent buffering if possible.
Acknowledgements give similar reliability to persistent buffering but are more efficient.
A persistent buffer is used if acknowledgements are not available.

##### `AtLeastOncePersistent`

**NOTE**: This option may be omitted in the first version, until we are satisfied there are use cases.

Like `AtLeastOnce`, but forces the use of a persistent buffer, possibly in addition to acknowledgements.
Using buffer and acknowledgements together gives better reliability than buffering alone.

Use `AtLeastOncePersistent` when:
- You have a special situation where end-to-end acknowledgements alone do not work.
- You want to force the use of an extra-large buffer to work around large overload spikes.

**Note**: Very large buffers _cannot fix long-term overload_, they can only work-around temporary spikes.
The _long-term_ average throughput _must_ be within the collectors capacity, otherwise the system cannot "catch up".
Once the buffer fills, logs will be lost as if the buffer wasn't there,
and logs that are delivered will have high latency from waiting around in the buffer.

**Note**: The log collector _cannot_ guarantee fully reliable end-to-end delivery.
It has no control over the reliability or throughput of sources and sinks.
`AtMostOnce` takes advantage of reliability features at the source and sink, but the end-to-end result is only as good as the weakest link.

#### compression: string

Enable compression if supported
- Value indicates compression type (e.g. "zlib", "gzip"), valid values depend on the output type.
- Error if output does not support compression or does not recognize the value.

#### maxWriteBytes: byte measurement (100, 1K etc.) 

Limits the maximum bytes of data that will be sent in a single remote "write" or "send" operation.
Defaults are determined by the underlying sink.
#### minRetryDuration`: duration (1s, 200ms)

The first retry after a connection failure will happen after `minRetryDuration`
The delay between subseqent retries will increase up to `maxRetryDuration`
`minDelay` for the first attempt, delay increases up to `maxDelay` and then repeats.
- `maxDelay`: maximum delay between attempts.

#### maxRetryDuration`: duration (1s, 200ms)
See `minRetryDuration`.

### Topology Considerations
#### Hypershift / Hosted Control Planes
No special considerations.
#### Standalone Clusters
No special considerations.
#### Single-node Deployments or MicroShift
No special considerations.

### Implementation Details/Notes/Constraints

#### Causes of log loss

1. *Overload*: Logs are produced faster than they can be processed.
2. *Faults*: Collector restarts, network errors, remote store problems etc.

Log loss can be avoided for _temporary_ faults or overloads.
Persistent buffers and/or acknowledgements can store or re-send lost in-memory data.

_Sustained overload_ lasting long enough to exceed buffering capacity _will_ cause data loss.
The only remedy is to ensure that the log collector and store can keep up with the rate of log production.

#### Acknowledgements

[End-to-end acknowledgement](https://vector.dev/docs/about/under-the-hood/architecture/end-to-end-acknowledgements) 
means that source acknowledgements are _delayed_ until all relevant sinks have received the data.
After restart, the source can re-send data that did not reach all sinks - the source acts as a persistent buffer.

This is more efficient than duplicating the data again in a vector disk buffer,
but only works for sources that support acknowledgement and "at-least-once" reliable delivery.

Examples of acknowledgement sources:
- Kafka: Kafka protocol has acknowledgements and at-least-once delivery.
- HTTP: HTTP response can be used  to implement at-least-once.
  Not all HTTP clients do this, but REST clients for data streaming usually do.
- **File**: Vector's persistent "position" file can be used like an "acknowledgement" for file sources.
  Reading from the persisted position after restart is equivalent to at-least-once delivery.

#### Delivery policy implementation

`AtLeastOnce` always enables end-to-end persistence on sources and sinks that allow it.
If a source does not allow it, then `AtLeastOnce` implements a persistent buffer using the
same default size as Vector's default in-memory buffer.

``` pseudo-code
for each `AtLeastOnce` output:
  set sink.acknowledgement=true for attached sink(s).
  for each source, of each input, of each pipeline to the output:
    if source can participate in acknowledgement:
      set source.acknowledgement=true
    else
      set output.buffer=disk
```

`AtLeastOncePersistent` always enables disk buffering on the output.
It _also_ enables end-to-end persistence on sources and sinks that allow it.

Buffering and acknowledgement is more reliable than buffering alone.
Without acknowledgement records are dropped from the buffer as soon as they are sent,
with acknowledgement they are held until the remote acknowledges that the are safely stored.

**Note**: Rate limits set by `outputs[].limit` re still enforced with `AtLeastOnce*`, even though this
means deliberately dropping log records from the collector.
Review the existing limit code when implementing delivery policy so that the two work together properly.

### Risks and Mitigations

- Persistent buffering combined with multiple CLF instances creates potential problems with node disk space.
- Added complexity to forwarder.
- Support cost of customers abusing or misunderstanding the parameters.
- Increased customer demand for help "sizing" the logging stack: setting resources and predicting performance.

No new security risks are expected.

### Drawbacks
None.
## Test Plan
## Graduation Criteria
### Dev Preview -> Tech Preview
### Tech Preview -> GA
### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy
None special.
## Version Skew Strategy
None special.
## Operational Aspects of API Extensions
## Support Procedures
## Alternatives

Expose detailed vector configuration: fails our overall mission of simplified configuration.
