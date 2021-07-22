---
title: pulling-and-exposing-data-from-ocm
authors:
  - "@tremes"
reviewers:
  - "@sbose78"
  - "@inecas"
  - "@petli-openshift"
  - "@smarterclayton"
approvers:
  - "@sbose78"
  - "@smarterclayton"
creation-date: 2021-03-04
last-updated: 2021-03-09
status: implementable
see-also:
replaces:
superseded-by:
---

# Insights Operator pulling and exposing data from the OCM API

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement will enable the Insights Operator to pull the data (SCA certs)
from the OCM (OpenShift Cluster Manager) API. The data will be exposed by the Insights Operator
in the OpenShift API to allow users to use them when consuming and building container images
on the platform.

## Motivation

Users could consume RHEL content and container images using the RHEL subscription in the OpenShift 3.x.
In the OpenShift 4, this is no longer possible because the Red Hat Enterprise Linux Core OS (RHCOS) does not
provide any attached subscription. This enhancement is to provide users the Simple Content Access (SCA) certs
from Red Hat Subscription Manager (RHSM).

### Goals

- Extend the Insights Operator config with an OCM API URL to be able to query the data
- Periodically pull the data from the OCM API and expose it in the OpenShift API

### Non-Goals

- Insights Operator providing any transformation or post-processing of the data pulled
  from the OCM API

## Proposal

### User Stories

#### Consume SCA certs exposed in the API

As an OpenShift user
I want to consume SCA certs to be able to consume RHEL content and to build
corresponding container images.

### Risks and Mitigations

#### OCM API is down

Risk: OCM API is down or doesn't provide up to date data.

Risk: Insights Operator is unable to expose/update the data in the OpenShift API

Mitigation: Introduce a new state in the Insights Operator (e.g "SCADataDegraded") and
create a new alert based on this new state.

## Design Details

### Authorization

The Insights Operator is able to pull the data from the OCM API using the existing `cloud.openshift.com` token
available in the `pull-secret` (in the `openshift-config-managed` namespace).

The Insights Operator must provide a cluster ID as an identifier of the cluster.

### Data in API

The SCA certificate is available via the `etc-pki-entitlement` secret in the `openshift-config-managed` namespace.

### Update period
- Insights Operator query the OCM API every 8 hours and downloads the full data provided

### Test Plan

- `insights-operator-e2e-tests` suite can verify the SCA cert data
  is available
- Basic test of the validity of the SCA certs. Mount the `etc-pki-entitlement` secret and run e.g `yum install` in the container

### Graduation Criteria

This feature is planned as a technical preview in OCP 4.9 and is planned to go GA in 4.10.

#### Dev Preview -> Tech Preview
- Insights Operator is able to download the data from OCM API and expose it in a cluster API
- basic functionality is tested
- this new functionality is documented

#### Tech Preview -> GA
- ability to distinguish various error states - e.g organization doesn't have SCA allowed versus API returns 404
- inform a cluster user about the error state (problem with pulling the certificates)

#### Removing a deprecated feature

The periodical data pulling can be easily disabled in the cluster configuration. Removing this feature will require updating the Insights operator code base and will remove the `etc-pki-entitlement` secret from the `openshift-config-managed` namespace. 

### Upgrade / Downgrade Strategy

There is no upgrade/downgrade strategy needed.

### Version Skew Strategy

There is no Skew strategy needed. This work should have no impact on the upgrade. It doesn't require any coordinated behavior in the control plane. No other components will change.

## Implementation History

There are no other major milestones in the implementation history than the graduation criteria mentioned above.

## Drawbacks

There is no significant drawback.

## Alternatives

Only possible alternative is to implement this functionality in another control plane component/operator.
