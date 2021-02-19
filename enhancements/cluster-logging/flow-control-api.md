---
title: flow-control-api
authors:
  - "@alanconway"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-02-05
status: provisional
see-also:
replaces:
superseded-by:
---

# Flow control API for Logging

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

If the log collector cannot keep up with the rate that logs are written, some logging data will be lost.
Presently the logging admin has little control over when or how logs get lost.

This proposal defines an API to let cluster administrators to limit logging rates, or ignore some logs entirely.
Logs will still be lost if the collector cannot keep up, but administrators have more control over *what* is lost.

Rate limits mean that:

* The cost and volume of logging can be predicted more accurately in advance.
* Noisy containers cannot produce unbounded log traffic and unfairly drown out other containers.
* High-value logs can be preferred over low-value logs by assigning higher rate limits.
* Log streams that are not needed can be ignored, which reduces the load on the logging infrastructure.

The API allows rates and policies to be applied _per-container_ by namespaces or pod labels.
New types of selector that are added in future should also be applicable.

## Motivation

### Goals

### Non-Goals

The following are *not* goals for this proposal but *may* be addressed in future proposals:

- `block` policy: containers that exceed their rate limit are back-pressured by leaving their stdout/stderr unread until they are within the target rate.
- combined rate limits: specify a *combined* rate limit, i.e. limit the sum of all container rates logging to the same pipeline. Dynamically adjust individual rate limits to give an approximately equal individual limit to each container.

## Proposal

Add a new CR `ClusterLogFlowControl`:

`spec`: []FlowControlRule

FlowControlRule: Match all criteria (AND)
- `logType`: (enum: application|infrastructure|audit, default: application)
- `namespaces`: ([]string, optional) policy applies to containers in any of these namespaces.
- `selector`: ([LabelSelector][LabelSelector])
- `maxByteRate`: ([Quantity][Quantity], default 0) bytes per second in [k8s resource notation][memory]
  Absent or <= 0 means no limit (other than the limits of the logging system).
  Use `policy: ignore` to ignore logs, not `maxByteRate: 0`.
- `policy`: (enum: ignore|drop|block, default: drop)
  - `ignore`: no logs are forwarded, if possible they are not even collected.
  - `drop`: drop data if necessary to respect `maxByteRate`, or if the logging system cannot keep up.

If multiple rules match a container the *last* matching rule applies.
You can specify general rules followed by more specific over-ride rules.
For example a default rule for a namespace, followed by an over-ride for pods with specific labels in the namespace.

[LabelSelector]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#labelselector-v1-meta
[quantity]: https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/
[memory]: (https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-memory)

### User Stories

#### Cluster admin wants to set global per-container default of 1024 bytes/second

``` yaml
- maxByteRate: 1024
```

#### Cluster admin wants to ignore logs from containers with certain labels

``` yaml
- selector:
    matchLabels: { boring: true }
  policy: ignore
```

#### Cluster admin wants to set a limit for containers in selected namespaces

``` yaml
- namespaces: [ slow, slower, slowest ]
  rateLimit: 512
- namespaces: [ important ]
  rateLimit: 5Ki
```

#### Cluster admin wants to set a limit for containers with certain labels

``` yaml
- selector:
    matchLabels: { importance: medium }
  maxByteRate: 1k
- selector:
    matchLabels: { importance: high }
  maxByteRate: 5k

```

#### Example of combining rules

- Set a cluster-wide default limit of 1024 bytes/sec.
- Set a lower limit of 512 bytes/sec for containers in namespaces "slow", "slower", "slowest".
- Set a higher limit of 5 kbytes/sec for *all* containers labelled `importance: high`, even in the "slow" namespaces.

``` yaml
- maxByteRate: 1024 # Cluster-wide default

- namespaces: [ slow, slower, slowest ]
  maxByteRate: 512

- selector:
    matchLabels: { importance: high }
  maxByteRate: 5k
```

### Implementation Details

Will combine the fluentd throttle plugin with the label/namespace routing plugin being introduced in
https://github.com/openshift/cluster-logging-operator/pull/865

May require some re-factoring of existing input-to-output routing logic, since we are introducing a second layer of input classification for throttlinga.
See Open Questions.

### Risks and Mitigations

## Design Details

### Open Questions

- It is logically cleaner to separate these rules into their own resource, but it may complicate the configuration of fluentd.
  Should we move these rules to ClusterLogForwarder pipelines so we have a single point for routing/throttling rather than 2 overlapping rule sets to resolve?

- Can we easily support namespace pattern matches?

### Test Plan

TODO

## Drawbacks

- Possible complexity combining routing and rate limiting rules.

## Alternatives

None proposed
