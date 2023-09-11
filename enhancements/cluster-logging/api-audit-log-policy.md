---
title: api-audit-log-policy
authors:
  - "@alanconway"
reviewers:
  - "@jewzaam"
  - "@eparis"
approvers:
  - "@jewzaam"
api-approvers:
  - "@jewzaam"
creation-date: 2023-03-28
tracking-link:
  - https://issues.redhat.com/browse/OBSDA-344
  - https://issues.redhat.com/browse/LOG-3982
see-also:
  - /enhancements/cluster-logging/hypershift-audit-logs.md
---

# API Server Audit Log Policy

## Summary

The Kube and OpenShift API servers generate a lot of log data, with very large individual records.
Much of this data has little or no value for security auditing.
It is not feasible to forward the unfiltered data to most external log storage systems.
Audit events may exceed record size limits, and the total volume may be too expensive to store.

Kubernetes defines an [audit policy configuration][k8s-auditing] to filter audit events,
Red Hat Service Delivery filters API audit logs using  https://gitlab.cee.redhat.com/service/splunk-audit-exporter
which implements the k8s audit policy. This exporter is not available to customers.

The goals of this proposal are:
- Make the Kubernetes audit policy available for customer log forwarding.
- Allow customers and Service Delivery to configure _independent_ policies and destinations for audit logs.
- Make it possible for OSD and Customers to both use the CLO for log forwarding.

This proposal applies to all types of OpenShift cluster.
There is a [separate proposal with additional details for hypershift clusters](hypershift-audit-logs.md)

[k8s-auditing]: https://kubernetes.io/docs/tasks/debug/debug-cluster/audit

## Motivation

### User Stories

#### As a cluster owner, I want to store and/or forward selected API audit logs to my chosen destinations

- configure an event logging policy to match my security auditing needs.
- store and/or forward the resulting events in any destination supported by the `ClusterLogForwarder`.

#### As a cluster manager, I want selected audit logs to be forwarded to my preferred log store

- store/forward audit logs for internal use according to cluster manager policies, to my preferred log store.
- independent of the users desired policy and destinations.

### Goals

#### Fine grained control

The [Kubernetes Policy][k8s-auditing] provides detailed tuning, and has been well tested by the Kubernetes community and Service Delivery.

#### Multiple separate audit policies

For managed clusters, the cluster manager and cluster owner may have different auditing needs.
third party cluster auditing tools may introduce yet other auditing needs.

To support _multiple_ independent audit log streams, we cannot configure filtering in the API server directly.
Instead the API server will write complete logs (or some super-set of all desired logs)
and the `ClusterLogForwarder` will apply a separate policy filter for each consumer.

### Non-Goals

- No help with semantics of audit events or design of audit policies.
- No Hypershift support in this proposal -  [see separate hypershift proposal](hypershift-audit-logs.md)

## Proposal

### Workflow Description

Cluster admin will
1. Create a `Policy.audit.k8s.io` resource with the desired policy.
2. Create an `audit` input in the `ClusterLogForwarder` linked to the policy.
3. Create a `ClusterLogForwarder.pipeline` as normal to forward the resulting logs.

### API Extensions

#### New resource `Policy.audit.k8s.io`

See [Kubernetes Documentation][k8s-auditing] for an example.

#### Extensions to `ClusterLogForwarder.logging.openshift.io`

Add a new `filters` section to the `ClusterLogForwarder` resource.
A filter can contain an audit policy. Activate the filter by referring to it from a pipeline.
The same filter can be attached to multiple pipelines.

For example:

``` yaml
  filters:
    - name: my-policy
      type: apiAudit # Other types of filter will be added in future.
      apiAudit:
	    # Audit policy as defined by https://kubernetes.io/docs/tasks/debug/debug-cluster/audit
        omitStages:
          - "RequestReceived"
        rules:
          - level: RequestResponse
            resources:
            - group: ""
              resources: ["pods"]
  pipelines:
    - inputRefs: [application, infrastructure, audit]
      filterRefs: [my-policy]
      outputRefs: [default]
```

### Implementation Details

The policy is compiled into a VRL (Vector Remap Language) transformation as part of the vector log forwarding configuration.
The transformation drops or edits API audit events according to the policy.
Logs which are not API audit events are passed on unmodified.

### Risks and Mitigations

None.

#### Data security

No new sensitive data is exposed.
The `ClusterLogForwarder` can already forward API server audit logs on OSD, ROSA classic and on-premise clusters.

#### Run-time resource costs

OSD and ROSA classic have been doing this type of filtering on managed clusters for years, no new problems are expected.

#### Compliance and legal risks

Incorrectly filtering out important audit events may be a compliance/legal problem.
The customer creates their own filtering configuration, it needs to be clear that it is their responsibility to do it correctly.

#### Roll-out

Existing customers using the CLO add-on will be unaffected.
The new features will be activated only when the customer modifies their configuration as described above.

The existing SRE Splunk exporter SHOULD be replaced by separate CL/CLF resources in a separate namespace as soon as possible.
Depends on [LOG-1343 Multiple forwarders](https://issues.redhat.com/browse/LOG-1343).

However, the Splunk exporter MAY continue to run alongside customer CLO deployments using these new features, if necessary.
Both can scrape from the same audit log files simultaneously, and perform separate filtering.

**NOTE**: this is not true for Hypershift, [see hypershift proposal](hypershift-audit-logs.md)

### Drawbacks

- Increased scope for user error in policies.
- Possible additional CPU cost.

## Design Details

None.

### Open Questions

None.

### Test Plan

- unit tests for each type of rule.
- port tests from the splunk-audit-exporter repository.
- compatibility test to ensure it produces the same result as splunk-audit-exporter.

### Graduation Criteria

None.

#### Dev Preview -> Tech Preview

None.

#### Tech Preview -> GA

None.

#### Removing a deprecated feature

None.

### Upgrade / Downgrade Strategy

Nothing new.

### Version Skew Strategy

Nothing new.

### Operational Aspects of API Extensions

Nothing new.

#### Failure Modes

- Invalid Policy: indicate in `ClusterLogForwarder.status`

#### Support Procedures

Nothing new.

## Implementation History

None.

## Alternatives

- Improve filtering at the API server instead of `ClusterLogForwarder`
  - does not meet the requirement of multiple independent filters.
- Use existing Openshift Audit Policy resource
  - ongoing customer attempts suggest this is not sufficient
  - user cannot change this policy in managed clusters - reserved by SD.
- Forward unfiltered audit logs
  - multiple customers report this is unacceptable.
- Introduce some other mechanism to forward audit logs
  - this was the approach of SD (custom splunk forwarder)
  - it breaks down when logs need to be forwarded elsewhere (e.g. Cloudwatch)
  - it scatters and duplicates features in the logging operator rather than improving those features in one place

Note: there is an existing [APIServer.config.openshift.io resource][openshift-config] resource that configures policy based on authenticated groups. 
This is insufficient for some customers:
- lacks control by _verb_ - the most important filter of all is to remove "read only" events.
- lacks control for of chatty k8s/openshift services that don't belong to distinct groups.

[openshift-config]: https://docs.openshift.com/container-platform/4.12/security/audit-log-policy-config.html
