---
title: operators-health-metric
authors:
  - "@sradco"
reviewers:
  - @simonpasquier
  - @jan--f
  - "@openshift/openshift-team-monitoring"
approvers:
  - @simonpasquier
  - @jan--f
  - "@openshift/openshift-team-monitoring"
creation-date: 2022-11-07
last-updated: 2022-11-27
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CNV-14693
status: new
---

# operators-health-metric

## Summary

This enhancement describes a generic way for operators, that use Prometheus as their monitoring tool, to report the operator health metric based on alerts.
This would be the recommended way to report the health metric and will not be enforced.

Currently, OLM only reports if the operator csv is in the `succeeded` state, by the `csv_succeeded` metric.
This value doesn't "know" if the operator is actually healthy or not.
In addition, there is no generic way to create the operators health metric, so it can differ in the way they it is implemented and calculated, which can lead to inconsistencies.

## Motivation

The health metric that an operator can report on itself, internally, is limited in scope and to real-time data.
We would like to be able to tell if there is a real issue with the operator that is based on the duration the issue exists.

For example:
If all operator api servers are down, we should not report the operator as unhealthy, since this can be automatically addressed by k8s.
This issue can be caused, for example, due to an operator upgrade.
Only if they are down for a period greater then X, then we should indicate there is an actual issue.

### User Stories

#### US1

As a user, I’d like to be able to check all my operators health status with a dedicated metric, so that it will help me with identifying issues in my environment.

#### US2

As an operator developer, I’d like to have a guidline for reporting an operator health, so that it will help to speed up my development process and allow me to easily track when issues accure and why.

### Goals

* Have a standard and recommended way for reporting operators health metrics, that can later be used to gather system wide operators health.
* Drive operators developers to invest in adding metrics and alerts to their operator, which will increase operator observability as a whole.
* Avoid code duplication and a clear understanding what impacts the operator health.

### Non-Goals

Out of scope are:

- Operators that do not want to add support for Prometheus based monitoring.

## Proposal

Use Prometheus alerts for calculating the operators health metric for each operator.
Operators determine when a functionality is lost or compromised and alerts the user once the evaluation time that was set for the alert has passed and the alert started firing.
This evaluation time period is important to determine if there is indeed an issue that Kubernetes was unable to resolve.
It also allows to easily examine why the operator is unhealthy.

This implementation gives an in-depth health analysis in comparison to an internal health metric.

Operators developers should work on analyzing their operator health indicators and translate them into Prometheus metrics and alerts.

Each alerts should include the following labels/annotations:
1. Operator name- The name of the operator that triggered the alert.It is required in order to be able to calculate the health for each operator separately.
Proposed name: `kubernetes_operator_part_of`. Label name is based on the  [Kubernetes Recommended Labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/#labels).
2. Health label - Indicates whether this alert impacts the operator health. It is required since not all alerts have an impact on the operator health.
Proposed name: `operator_health_impact`. Values: `critical` / `warning` / `none`.

If operator has at least 1 `critical` alert firing that relates to the operator health, it means that some important functionality is lost, then its health would become "Unhealthy"(Red).
If the operator has more that X `warning` alerts, we should consider if the operator should be considered as "At risk" (Yellow).

### Workflow Description

The user will be able to query the health metric and get the health of each operator, in Prometheus
and in the OpenShift UI. It can also be added to a dashboard in the OCP UI and Grafana.

The use of alerts would also allow to link them tho the specific alert thats that are impacting the operator health and
the users can use the alerts runbooks in order to resolve the issue.

#### Variation [optional]

### API Extensions

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

### Drawbacks

The health metric based on alerts is only relevant for operators that will integrate Prometheus,
but this has the most benefit for OpenShift certified operators.

## Design Details

### Open Questions [optional]

1. Why do we need a health metric if it doesn't help the community that don't use Prometheus.

Answer: It can be in addition to an internal real time metric. The benefit is that we can use it to report a more accurate health of the operator and also direct to the alerts that are affecting it and provide the runbooks to fix it.
As well as encouraging the OCP operators and partners to invest in alerts, which will result also in overall better operator monitoring and create an alignmnet on how the health metric is calculated.


### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives

We are working to add best practices for operators observability.
One of the recommendations would be to do a seperation between the metrics code and logic and the operator core code.
For internal health metric we can suggest best practices as well.
But this would still not solve the evaluation time factor that alerts help with.

## Infrastructure Needed [optional]
