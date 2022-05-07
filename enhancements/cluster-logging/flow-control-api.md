---
title: flow-control-api
authors:
  - "@alanconway"
reviewers:
  - "@pmoogi"
  - "@eranra"
  - "@ajay"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2021-02-05
last-updated: 2022-01-20
tracking-link: https://issues.redhat.com/browse/LOG-1043
see-also:
status: provisional
see-also:
replaces:
superseded-by:
---

# Flow control API for Logging

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

"Flow control" refers to how the logging system behaves when logs are produced faster than they can be collected or forwarded.
A lack of planned flow control creates the following problems:

* Hard to predict and impossible to control the volume of logs.
* No control over which logs get lost.
* During an outage, log buffers build up without user control.
  This can cause long recovery times and high latency when the connection is restored.

This proposal defines an API to let cluster administrators to limit logging rates, or ignore some logs entirely.
Logs may still be lost if the collector cannot keep up, but administrators have more control over *what* is lost,
and more predictability of log rates.

## Motivation

Rate limits mean that:

* The cost and volume of logging can be predicted more accurately in advance.
* Noisy containers cannot produce unbounded log traffic and unfairly drown out other containers.
* Log streams that are not needed can be ignored, reducing the load on the logging infrastructure.
* High-value logs can be preferred over low-value logs by assigning higher rate limits.

### Goals

Control log rates and overflow policy at *two* points in the log forwarder:

* Output: controlling the flow rate *per destination* to selected outputs.
  * Limit the rate of outbound logs to match output network and storage capacity.
  * Controls *aggregated* (per-destination) output rate.
* Input: Controlling log flow rates *per container* from selected containers.
  * Limit the rate of log collection for selected groups of containers *per-container*.
  * Controls *individual* (per-container) collection throttling.

### Non-Goals

#### Limits in records, not bytes

The limits in this proposal are specified in "records".
A record corresponds to a single log entry - typically a single line in a log file.

Disk capacity and network bandwidth are specified in _bytes_.
For admins managing these resources, it would be easier if log rate limits were also in bytes.

Unfortunately most log collectors and similar tools use _records_ as the unit for counting.
This is because it is easy to count records internally in such tools, but the exact byte size of
the record as written to some output can vary due to transformations, different encodings etc.

Our API assumes that limits are expressed in records.
Users must estimate the average size of a record in their system to make byte-size predictions.
This is less convenient than giving byte limits, but still feasible.

#### Blocking flow control

This proposal does not include a `block` policy, which would back-pressure containers that exceed rate limits, forcing them to block on stout/std err and slow down to keep within the rate limit.
This may be added as a feature in future proposals.

## Proposal

### User Stories

#### Limit rate to remote Kafka output to 10,000,000 records/s to avoid saturating network link

``` yaml
  outputs:
    - name: offsite
	  type: kafka
	  limit:
        maxRecordsPerSecond: 10M
```

**Note**: flow rules applied to *outputs* specify a *per destination* limit.

#### Limit for default output to 10,000,000 records/s to respect local store limits

``` yaml
  outputs:
    - name: default
	  limit:
        maxRecordsPerSecond: 10M
```

**Note**: we need to add the ability to set a flow rule on the special "default" output.

#### Ignore (don't collect) logs from containers with certain labels

``` yaml
  inputs:
	- application:
		selector:
		  matchLabels: { boring: true }
	    limitPerContainer:
          maxRecordsPerSecond: 0
```

**Notes**
* Flow rules applied to *inputs* specify a *per container* limit.
* Inputs and input selectors are already part of the ClusterLogForwarder API.
* If multiple input limits apply to a container, the _lowest_ limit is applied.
  Example: the same container is selected by two inputs, one by namespace and one by label.

#### Set a per-container limit for containers in selected namespaces

``` yaml
  inputs:
	- application:
		namespaces: [ boring, tedious, tiresome ]
	    limitPerContainer:
		  maxRecordsPerSecond: 10
    - application:
		namespaces: [ important, exciting ]
	    limitPerContainer:
		  maxRecordsPerSecond: 1000
```

#### Set a per-container limit for containers with certain labels

``` yaml
  inputs:
	- application:
		selector:
		  matchLabels: { importance: low }
        limitPerContainer:
		  maxRecordsPerSecond: 10
    - application:
	    selector:
	  	  matchLabels: { importance: high }
        limitPerContainer:
		  maxRecordsPerSecond: 1000
```

### API Extensions

New API struct type `RateLimit` with fields:
- `maxRecordsPerSecond`: ([Quantity][Quantity], optional) Maximum log records per second allowed.\
  If the inbound rate exceeds this limit then the `policy` is applied to keep the outbound rate within the limit.
  - Absent (default): best effort, drop records only if the forwarder cannot keep up.
  - 0: means do not forward any logs - if possible the logs are not even collected.
  - greater than 0: is a limit in log records per second.
- `policy`: (enum: drop, default: drop) Placeholder for future policy extensions.\
  For the first iteration the only policy is `drop` and this field need not be specified.
  If the inbound flow exceeds the limit, logs are dropped.
  See Non-Goals for examples of possible future policy extensions.

`ClusterLogForwarder.input` new optional field:
- `perContainerLimit`: (RateLimit, optional) limit applied to _each container_ selected by this input.
  No container selected by this input can exceed this limit.

`ClusterLogForwarder.output` new optional field:
- `limit`: (RateLimit, optional) flow control limit to be applied _to the aggregated log flow to this output_.
  The total log flow to this output cannot exceed the limit.

[LabelSelector]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#labelselector-v1-meta
[quantity]: https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/
[memory]: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-memory

### Risks and Mitigations

Risks:
* Complexity added to forwarder.
* Performance impact of enforcing limits (beyond the limit itself)
* Well supported by vector, not clear if this can easily be implemented by fluentd.

Mitigations:
* Benchmarking of performance to verify impacts.
* Modular design to separate limit logic from other features - depends on Vector capabilities.

## Design Details
### Test Plan
TODO
### Graduation Criteria
TODO
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature
TODO
### Upgrade / Downgrade Strategy
TODO
### Version Skew Strategy
TODO
### Operational Aspects of API Extensions
TODO
#### Failure Modes
#### Support Procedures
## Implementation History
None
## Drawbacks
Complexity
## Alternatives
None proposed

