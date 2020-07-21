---
title: Tuning Fluentd Output Peformance
authors:
  - "@alanconway"
reviewers:
approvers:
creation-date: 2020-05-27
status: provisional
see-also:[]
replaces:[]
superseded-by:[]
---

# Tuning Fluentd Output Peformance

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Allow defaults for selected fluentd output `<buffer>` fields to be specified globally
for logging, for performance-tuning purposes.

These settings are:
* *not* relevant to most users, default settings should give good general performance.
* only for *advanced* users with detailed knowledge of `fluentd` configuration and performance.
* only for performance tuning, they have no effect on functional aspects of logging.

These settings have weaker forward-compatibility guarantees than the rest of the API.
In future releases:
  - `fluentd` settings may be ignored, if the logging implementation changes.
  - valuesmay be overridden by (present or future) settings in other logging APIs.
  - new sections may be introduced for new implementations, they may not have equivalent settings to `fluentd`

The `clusterloggings.logging.openshift.io` API will have a new map field
`spec.forwarder.fluentd`. The `forwarder` field is for global tuning parameters
for fluentd output plugins.

## Motivation

### Goals

Ultimately we want great performance "out of the box" with no user
intervention. However, today we can't always predict/detect the best settings;
customers have had to adjust `fluentd` parameters to get good performance.

Furthermore, some requirements *can't* be predicted or detected.  Optimizing for
low latency often conflicts with optimizing for high throughput. We can't
predict or detect that a user wants to aggressively optimize for one or the
other.

Goals:
* Expose selected fluentd performance optimization parameters in the ClusterLogging API.
* Apply defaults from forwarder.fluentd to the underlying `fluentd` configuration.

We will be successful when:
* An administrator is able to set defaults for selected `fluentd` parameters.

### Non-Goals

* Not implying that logging will always use `fluentd` or use it in the same way. `forwarder.fluentd` applies to the current implementation, it may be ignored in future releases.
* Not allowing general `fluentd` configuration: we expose only a subset of performance tuning parameters. Users that want full control of `fluentd` need to go unmanaged.
* Not a generic performance tuning API: we may introduce such APIs in future, or introduce new `forwarder` sections. 

## Proposal

Field names are based on fluentd 1.0 `<buffer>` configuration section.
See the [fluentd documentation](https://docs.fluentd.org/configuration/buffer-section#buffering-parameters) for details.

Example below shows just the `forwarder` section of the ClusterLogging CR.

The field names are copied exactly from `fluentd` 1.0, including the use of "_"
rather than camelCase. This emphasizes that these settings are not a general
API, they set defaults directly for the underlying `fluentd`.

Note: two important fields were renamed by `fluentd` between versions 0.12 and 1.0:
* `queue_limit_length` -> `total_limit_size`
* `num_threads` -> `flush_thread_count`

```yaml
forwarder:
  fluentd:
    buffer:
      # Memory use
      chunk_limit_size: 1m
      total_limit_size: 32m             # Replaces 'queue_limit_length' in 0.12
      overflow_action: exception

      # Flushing output behavior
      flush_thread_count: 2              # Replaces 'num_threads' in 0.12
      flush_mode: interval
      flush_interval: 5s
      flush_at_shutdown: false

      # Retries
      retry_wait: 1                      # The wait interval for the first retry.
      retry_type: exponential_backoff    # Set 'periodic' for constant intervals.
      retry_max_interval: 300
```

Note: All these settings relate to the latency vs. throughput
trade-off. Optimizing for throughput favors batching to reduce network packet
count; bigger buffers and queues, delayed flushes and retries. Optimizing for
low latency favors sending data ASAP and *avoiding* build-up of batches; shorter
queues/buffers to minimize memory use, rapid flush and retry.

The CLO *may* override these settings if they would break functional behavior,
for example settings that are incompatible with a particular output type.

In future the CLO *may* introduce new APIs that can overlap with tuning
parameters in `forwarder`. How the overlap is resolved will be
decided if/when that happens.

### Implementation Details

The tuning values replace the environment variables that were previously used to
modify these defaults, for example `BUFFER_SIZE_LIMIT`.

### Risks and Mitigations

Risk: Users may set unreasonable values and break or slow down logging.

Mitigation: Document this is an advanced feature and the user is responsible for
any misbehavior if they use it incorrectly.

## Implementation History

Not yet started

## Drawbacks

* User can shoot self in foot.
* Possible future conflicts/confusion between the "recommended" way and the "tuning" way to accomplish a given result.

## Alternatives

* Use unmanaged logging, allows free-for-all hacking on fluentd configuration. Possible now.
* Design generic API for output tuning.
  - Still a goal but not close enough for existing users.
* Improve out-of-box performance for a wider range of use cases.
  - Still a goal but not close enough for existing users.
