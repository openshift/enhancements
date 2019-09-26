# phone-home-support

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ]  -facing documentation is created in [openshift/docs]

## Summary

The purpose of this enhancement is to propose a method of communicating the health of installed operators through [Operator-Lifecycle-Manager (OLM)](https://github.com/operator-framework/operator-lifecycle-manager) to Telemeter.

Today, OLM provides Telemeter with information regarding `subscription syncs` on clusters but fails to communicate the health of installed operators. By communicating which versions of operators are consistently failing, OpenShift engineers and ISV partners could proactively address issues and improve the customer experience.

OLM will communicate the health of installed operators by searching for CatalogSourceVersions (CSV)s and reporting their name, phase, version, and reason fields.

The initial target for this enhancement is to support reporting installed operator health per version to Telemeter in OpenShift 4.2 and 4.3.

## References

- https://docs.google.com/document/d/16EOP2W97EEIfkQ0FAwz3btwtMmKNDBM0qXrnDhXfzsM/edit
- https://github.com/operator-framework/operator-lifecycle-manager/tree/master/pkg/metrics

## Motivation

The primary motivation for this enhancement is to enable OLM to collect information about the health per version of CSVs installed on the cluster and report these metrics to Telemeter. With this information, key insights regarding operator health could be delivered to both OpenShift Engineering as well as ISVs.

### Goals

- Reporting CSV health per version
- Backport reporting CSV health per version to 4.2

### Non-Goals

- Reporting CSV Status Conditions per version
- Integrating with Insights Operator in 4.3

## Proposal

### User Stories

_**Collect CSV health per version**_

As a cluster administrator, I want to be able to:
- Collect information surrounding the health per version of operators deployed on the cluster

so that I can:
- Understand which operators are consistently failing on my cluster.

**Acceptance Criteria**

- OLM's metric package is updated to include a new vector that collects information about CSV's installed on the cluster.
- OLM's  [Documentation](https://github.com/operator-framework/operator-lifecycle-manager/tree/master/doc/design) is updated to highlight metrics that it exposes.
- OLM is updated to include a series of unit and e2e tests to ensure that the feature is working as expected.

**Internal details**

This enhancement proposes that OLM's [metrics package](https://github.com/operator-framework/operator-lifecycle-manager/tree/master/pkg/metrics) be updated to export information about the name, phase, version, and reason of CSVs deployed on cluster as a representation of operator health.

Currently, information about `subscription syncs` are provided to OLM in the following format:

```
# HELP subscription_sync_total Monotonic count of subscription syncs
# TYPE subscription_sync_total counter
subscription_sync_total{installed="",name="akka-cluster-operator"} 10.0
subscription_sync_total{installed="akka-cluster-operator.v0.0.1",name="akka-cluster-operator"} 1.0
```

A similar metric could be created for CSVs:

```
# HELP csv_sync_total Monotonic count of csv syncs
# TYPE csv_sync_total counter
csv_sync_total{name="akka", phase="Succeeded", version="0.0.1", reason="InstallSucceeded"} 1.0
```


_**Report CSV health via Telemeter**_
As a OpenShift administrator, I want to be able to:
- See information about failing operators across all clusters using Telemeter.

so that I can:
- Identify which operators are health or unhealth and notify the author.

**Acceptance Criteria**
- Telemeter is configured to collect the Operator Health metrics exposed by OLM.

**Internal details**

In order to complete this work, an engineer will need to:

- Configure Telemeter to collect metrics about Operator Health. This task involves whitelisting the metrics exposed by OLM in Telemeter.

 
_**Telemeter Dashboard that displays operator health**_

As a OpenShift administrator, I want to be able to:
- Visit a Telemeter dashboard that makes it easy to identify which operators are frequently failing on a specific cluster or across all clusters.

so that I can:
- Investigate operators that are failing on OpenShift cluster and reach out to operator authors.

**Acceptance Criteria**

- A Telemeter dashboard exists which highlights insights collected by OLM, across individual and all clusters at https://telemeter-lts-dashboards.datahub.redhat.com/

**Internal details**

In order to complete this work, an engineer will need to:
- Review the existing Telemeter documentation available at https://gitlab.cee.redhat.com/service/dev-guidelines/blob/master/monitoring.md
- Create a new dashboard that allows users to view failing operators in a single cluster or across all cluster by following the steps outlined at https://telemeter-lts-dashboards.datahub.redhat.com/d/playground/playground 


_**Telemeter Alerts for failing OLM operators**_

As a OpenShift administrator, I want to be able to:
- Receive a notification when an operator is consistently reaching a failed state.

so that I can:
- Immediately address known issues.

**Acceptance Criteria**

- A new alert is created and shipped with OLM that alerts users about failing Operators and their version.
- The alert is viewable via Telemeter


### Risks and Mitigations

_**Risks**_

- This feature will give users the ability to generate new time series for Prometheus to scrape
- Users are not able to opt-out so there could be an issue with GDPR

_**Mitigations**_

- Once the insights operator becomes available in 4.3, we plan to report metrics about operator health through insights
- The OpenShift Monitoring team could implement a feature to ignore ServiceMonitors in the OLM namespace

## Design Details

### Test Plan

An e2e test case for this feature could consist of:

1. Creating a CSV that reaches a failed state
2. Creating a CSV that reaches a succeeded state
3. Checking that the metrics endpoint captures the information about the newly created CSVs

An integration test for this feature could consist of:

1. Creating a CSV that reaches a failed state
2. Creating a CSV that reaches a succeeded state
3. Checking that the metrics endpoint captures the information about the newly created CSVs
4. Confirming that this information is present in Telemeter

### Graduation Criteria

#### Dev Preview -> GA

- Ability to expose CSV Health information at the metrics endpoint
- Ability to view CSV Health information in a telemeter dashboard
- End user documentation
- Sufficient unit and e2e test coverage
- Gather feedback from users rather than just developers

### Upgrade / Downgrade Strategy

Upgrading or Downgrading this component should introduce no added complexity given that it only consists of serving additional metrics at an endpoint.

### Version Skew Strategy

Version skew will introduce no added complexity.

## Implementation History

- The subscription sync count [PR](https://github.com/operator-framework/operator-lifecycle-manager/pull/976)

## Drawbacks

Non-OpenShift clusters will not report metrics back to Telemeter, however these clusters will expose data to admins about the health of installed operators.

## Alternatives

As discussed earlier in this enhancement, operator health information should eventually be sent to the Insights Operator rather than Telemeter to prevent users from creating their own time series.

We had also discussed appending information about the CSV to the `subscription syncs` metric already available in Telemeter. Ultimately, it was decided that we should create a new metric in the interest of collecting the data of all CSVs, not just those tied to a subscription.
