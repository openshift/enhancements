---
title: content-filter
authors:
  - "@alanconway"
reviewers: 
  - "@jcantrill"
approvers:
  - "@jcantrill"
api-approvers: 
  - "@jcantrill"
creation-date: 2023-11-03
last-updated:  2024-02-15
tracking-link:
  - https://issues.redhat.com/browse/LOG-2155
see-also: []
replaces: []
superseded-by: []
---

# Content filters for log records

## Summary

Allow users to reduce the volume of log data by:
1. Dropping unwanted log records completely.
2. Pruning unwanted fields from log records.

The new prune/drop content filters use the same framework as the kube-api-audit filter.
This framework can be extended with new types of filters in future.

**NOTE**: Content filters are distinct from input selectors.
Input selectors select or ignore entire log _streams_ based on _source metadata_.
Content filters _edit_ log streams (remove and modify records) based on _record content_.

## Motivation

Collecting all logs from a cluster produces a large amount of data, which can be expensive to transport and store.
A lot of log data is low-value noise that does not need to be stored.

The ideal solution is to configure each application to generate only valuable log data, however:
- Not all applications can be tuned to remove noise while keeping important data.
- Configuration of applications is not always under the control of the cluster administrator.
- The value of logs varies depending on use: for example debugging a specific problem vs. monitoring overall health.

### User Stories

* As a logging administrator I want to drop log records by severity or other field values in the record.
* As a logging administrator I want to prune log records by removing pod annotations, or other uninteresting fields.

### Goals

The user can configure the ClusterLogForwarder to drop and prune log records according to their requirements.

### Non-Goals

No complex record transformations, for example adding new fields with computed values.
More complex filters MAY be added in future, but not as part of this enhancement.

## Proposal

### Workflow Description

**log administrator** is a human responsible for configuring log collection, storage and forwarding in a cluster.

1. The log administrator creates named `filter` sections in a `ClusterLogForwarder`
2. Pipelines that require filtering have a `filterRef` field with a list of filter names.
3. The collector edits log streams according to the filters before forwarding.

### API Extensions

A new `filters` section in the `ClusterLogForwarder` allows named filters to be defined.
This proposal defines the following filter types: prune, drop.

#### Prune filters

A "prune" filter removes fields from each record passing through the filter.

##### API

``` yaml
spec:
  filters:
  - name: ""      # User defined name used as a pipeline filterRef
    type: prune
    prune:
      in: []      # Array of field paths, remove fields in the array.
      notIn: []   # Array of field paths, remove all fields that are NOT in the array.
```

**Note**: `in` and `notIn` entries must match regex `^(\.[a-zA-Z0-9_]+|\."[^"]+")(\.[a-zA-Z0-9_]+|\."[^"]+")*$`

##### Examples

The following removes the `kubernetes.flat_labels` field and all other fields except `message` and the remaining `kubernetes` fields

``` yaml
  spec:
    filters:
    - name: foo
      type: prune
      prune:
        in: [.kubernetes.flat_labels]    # Prune the kubernetes.flat_labels sub-field.
        notIn: [.message, .kubernetes]   # Keep only the message and kubernetes fields.
    pipelines:
    - name: bar
    filterRefs: ["foo"]
```

#### Drop filters

##### API

A drop filter applies a sequence of tests to a log record and drops the record if any test passes.
Each test contains a sequence of conditions, all conditions must be true for the test to pass.

``` yaml
spec:
  filters:
  - name:             # User defined name used as a pipeline filterRef
    type: drop
    drop:
      - test:
        - field:      # path to the field to evaluate
          # Requires exactly one of the following conditions.
          matches:    # regular expression to match against the value of the field
          notMatches: # regular expression to not match against the value of the field
```

**Note**:
- If _all_ conditions in a test are true, the test passes.
- If _any_ test in the drop filter passes, the record is dropped.
- If there is an error evaluating a condition (e.g. a missing field), that condition evaluates to false.
  Evaluation continues as normal.
  `field` value must match regex `^(\.[a-zA-Z0-9_]+|\."[^"]+")(\.[a-zA-Z0-9_]+|\."[^"]+")*$`
  only one of `matches` or `notMatches` may be defined for a test

The drop filter is equivalent to a boolean OR of AND clauses. Any boolean expression can be reduced to this form.

##### Example

The following example keeps only log messages from the "very-important" kubernetes namespace which do not have a log level of 'warning', 'error' or 'critical'.

``` yaml
filters:
  - name: important
    type: drop
    drop:
      - tests:
        - field: .kubernetes.namespace_name
            notMatches: "very-important"  # Keep everything from this namespace.
        - field: .level # Keep unimportant levels
          matches: "warning|error|critical"
```

### Topology Considerations

Will function identically in all topologies.

#### Hypershift / Hosted Control Planes
No special considerations.

#### Standalone Clusters
No special considerations.

#### Single-node Deployments or MicroShift
No special considerations.

Possibly some additional CPU consumption if enabled (no measurements yet)
Feature is opt-in, no effect if not enabled.

### Implementation Details/Notes/Constraints
### Risks and Mitigations

- Performance impact, need to benchmark slowdown due to rule evaluation, but balance against reduction in data volume. 
- No new security issues.
- Extensions to ClusterLogForwarder will be reviewed as part of normal code review.

### Drawbacks

- Possible performance impact (negative and positive)
- User can damage log records in ways that make them invalid.

## Open Questions [optional]
### Field path

Need to document the rules for field paths, these are a subset of the JSON path spec.
In fact we will use the same subset as Vector does, but we should describe the rules explicitly
and not refer to vector docs, so we don't create assumptions in the API about the underlying collector.

All field paths must match the regex `^(\.[a-zA-Z0-9_]+|\."[^"]+")(\.[a-zA-Z0-9_]+|\."[^"]+")*$`

### Metrics

Vector provides metrics for records in/out of each vector node.
Records dropped by filters can be computed from this.
We should provide _brief_ hints and links to these metrics, but not duplicate vector docs.

## Test Plan
Not yet.

## Graduation Criteria
### Dev Preview -> Tech Preview
### Tech Preview -> GA
### Removing a deprecated feature
## Upgrade / Downgrade Strategy
## Version Skew Strategy
## Operational Aspects of API Extensions
## Support Procedures
## Alternatives

