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

If the log collector cannot keep up with the rate that logs are written, some logging data will be lost.
Lack of  flow control these problems:
* Hard to predict and impossible to control the volume of logs.
* No control over which logs get lost.
* During an outage, log buffers build up without user control.
  This can cause long recovery times and very high latency when the connection is restored.

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

The following are *not* goals for this proposal but *may* be addressed in future proposals:

- `priority` rules: extends the `drop` policy so that lower-priority logs are dropped before higher-priority ones if possible.
- `block` policy (back-pressure): containers that exceed their rate limit are back-pressured, forcing them to block on stout/std err and slow down to keep within the target rate.\
  **Note**: This will impact application performance, is only appropriate if log loss is a bigger problem than a slow application.

Examples of these possible future policies:

``` yaml
limits:
  - name: future-priority
    policy: drop
    priority:
    - { level: critical }         # Prefer critical logs from anywhere
    - { namespaces: [ importantApp, veryImportantApp } # Next prefer logs from these NS.
    - TODO syntax, re-use input selector syntax and features???

  - name: future-backpressure
    policy: block                 # Best-effort with back-pressure.

  - name: future-backpressure-limit
    maxRate: 1000Kbi              # Rate limit with back-pressure.
    policy: block
```

## Proposal

### User Stories

#### Limit rate to remote Kafka output to 1GiB/s to avoid saturating network link

``` yaml
  limits:
	- name: oneGig
	  maxBytesPerSecond: 1Gi
  outputs:
    - name: kafka
	  type: kafka
	  ... details
	  limitRef: oneGig
```

**Note**: flow rules applied to *outputs* specify a *per destination* limit.

#### Limit for default output to 10GiB/s to respect local store limits

``` yaml
  limits:
	- name: oneGig
	  maxBytesPerSecond: 10Gi
  outputs:
    - name: default
	  limitRef: oneGig
```

**Note**: we need to add the ability to set a flow rule on the special "default" output.

#### Ignore (don't collect) logs from containers with certain labels

``` yaml
  limits:
    - name: ignore
      maxBytesPerSecond: 0
  inputs:
	- application:
		selector:
		  matchLabels: { boring: true }
	    perContainerLimitRef: ignore
```

**Notes**
* Flow rules applied to *inputs* specify a *per container* limit.
* Inputs and input selectors are already part of the ClusterLogForwarder API.
* If multiple input limits apply to a container, the first limit listed in the `limits` list applies.\
  Example: the same container is selected by two inputs, one by namespace and one by label.

#### Set a per-container limit for containers in selected namespaces

``` yaml
  limits:
    - name: slow
      maxBytesPerSecond: 1Ki
    - name: fast
      maxBytesPerSecond: 10Ki
  inputs:
	- application:
		namespaces: [ boring, tedious, tiresome ]
	    limitRef: slow
    - application:
		namespaces: [ important, exciting ]
		perContainerLimitRef: fast
```

#### Set a per-container limit for containers with certain labels

``` yaml
  limits:
    - name: slow
      maxBytesPerSecond: 1Ki
    - name: fast
      maxBytesPerSecond: 10Ki
  inputs:
	- application:
		selector:
		  matchLabels: { importance: low }
	 perContainerLimitRef: slow
    - application:
	    selector:
	  	  matchLabels: { importance: high }
	  perContainerLimitRef: fast
```

### API Extensions

Extend the `ClusterLogForwarder` API with an optional `limits` field (list of `limit`)

The `limit` type has fields:

- `name`: Name used to identify this limit.
- `maxRecordsPerSecond`: ([Quantity][Quantity], optional) Maximum number of log records per second rate allowed. If the inbound rate exceeds this limit then the `policy` is applied to keep the outbound rate within the limit.
  - Absent (default) means 'best effort' - go as fast as possible, drop records if forwarder cannot keep up.
  - 0 means do not forward any logs - if possible the logs are not even collected.
  - > 0 is a limit in log records per second (usually a log record corresponds to a line of log output)
- `policy`: (enum: drop, default: drop)\
  Placeholder for future policy extensions.
  For the first iteration the only policy is `drop` and this field need not be specified.
  If the inbound flow exceeds the limit, logs are dropped.
  See Non-Goals below for examples of possible future policy extensions.

The forwarder `input` and `output` types gets a new optional field
- `limitRef`: (limit, optional) to apply a flow control limit.

[LabelSelector]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#labelselector-v1-meta
[quantity]: https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/
[memory]: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-memory

### Risks and Mitigations

Risks:
* Complexity added to forwarder.
* Performance impact of enforcing limits (beyond the limit itself)

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

