---
title: pulling-and-exposing-sca-certs-from-ocm
authors:
  - "@tremes"
reviewers:
  - "@sbose78"
  - "@inecas"
  - "@petli-openshift"
  - "@bparees"
  - "@dhellman"
  - "@mfojtik"
  - "@adambkaplan"
approvers:
  - "@sbose78"
  - "@bparees"
  - "@dhellman"
creation-date: 2021-03-04
last-updated: 2021-08-10
status: implementable
see-also:
replaces:
superseded-by:
---

# Insights Operator pulling and exposing SCA certs from the OCM API

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement will enable the Insights Operator to pull the Simple Content Access certs for using RHEL subscription content
from the OCM (OpenShift Cluster Manager) API. The certificates will be exposed by the Insights Operator
in the OpenShift API to allow users to use them when consuming and building container images
on the platform.

## Motivation

Users could consume RHEL content and container images using the RHEL subscription in the OpenShift 3.x.
In the OpenShift 4, this is no longer possible because the Red Hat Enterprise Linux Core OS (RHCOS) does not
provide any attached subscription. This enhancement is to provide users the Simple Content Access (SCA) certs
from Red Hat Subscription Manager (RHSM).

### Goals

- Extend the Insights Operator config with an OCM API URL to be able to query the certificates
- Periodically pull the certificates from the OCM API and expose it in the OpenShift API
- This is an opt-in feature by a cluster user and might be moved to a different OCP component in the future

### Non-Goals

- Insights Operator providing any transformation or post-processing of the SCA certs pulled
  from the OCM API

## Proposal

### Why is it in the Insights Operator?

The Insights Operator is now the only OCP component that connects an OpenShift cluster to a Red Hat subscription experience (console.redhat.com APIs). The consumers of the SCA certs are builds, shared resources (such as the CSI driver) and any workload that requires access to the RHEL subscription content.

### User Stories

#### Consume SCA certs exposed in the API

As an OpenShift user
I want to consume SCA certs to be able to consume RHEL content and to build
corresponding container images.

### Risks and Mitigations

#### OCM API is down

Risk: OCM API is down or doesn't provide up to date data.

Risk: Insights Operator is unable to expose/update the data in the OpenShift API

Mitigation: The Insights Operator is marked as Degraded (in case the HTTP code is lower than 200
or greater than 399 and is not equal to 404, because HTTP 404 means that the organization didn't allow this feature).
At the same time, the Insights Operator must not be marked as Degraded in a disconnected cluster. The Insights Operator is marked as degraded after a number of unsuccessful exponential backoff steps.
The reason for marking the Insights Operator as Degraded is that we would like to gather data for such cases during the tech preview period. After the tech preview period, this should help with the fact that a cluster may have outdated or invalid entitlements.
This can be crucial for the workloads using these entitlements (in other words you want to know about a situation where you do not have valid entitlements before the upgrade, because all the Pods using the entitlements will fail after restart during the upgrade).

## Design Details

### Authorization

The Insights Operator is able to pull the certificates from the OCM API using the existing `cloud.openshift.com` token
available in the `pull-secret` (in the `openshift-config` namespace).

The Insights Operator must provide a cluster ID as an identifier of the cluster.

### SCA certs in API

The SCA certificate is available via the `etc-pki-entitlement` secret in the `openshift-config-managed` namespace.
The type of the `etc-pki-entitlement` secret is the `kubernetes.io/tls` with standard `tls.key` and `tls.crt` attributes.
The secret will be available for use in other namespaces by creating a cluster-scoped `Share` resource.
Cluster admin creates a `clusterrolebinding` to allow a service account access to the `Share` resource.

### Use of the SCA certs

- The SCA certificate can be mounted to a `Pod` as a CSI volume (where the volume attributes will reference the `Share` resource making the secret accessible)
- The SCA certificate can be mounted to a `Build` strategy as a CSI volume. The CSI driver is described in the [Share Secrets And ConfigMaps Across Namespaces via a CSI Driver](/enhancements/cluster-scope-secret-volumes/csi-driver-host-injections.md) enhancement and implemented in [https://github.com/openshift/csi-driver-shared-resource](https://github.com/openshift/csi-driver-shared-resource).

### Configuration
- The new configuration is tied to the Insights Operator, because the Insights Operator is currently the only component reading and using the `console.redhat.com` authentication token in the `pull-secret` in the `openshift-config` namespace, so if we decide to move this somewhere else, we must consider this fact.
- The new OCM API related config attributes are adjustable via `support` secret in the `openshift-config` namespace
- The Insights Operator respects the cluster wide proxy configuration (if any) so that it can reach the OCM API when behind a proxy
- Following attributes are configurable via the `support` secret:
  - OCM API endpoint
  - time period to query the OCM API
  - flag to disable this functionality

### Update period
- Insights Operator query the OCM API every 8 hours and downloads the full data provided
- The time period is configurable (via `support` secret) and can be changed by the cluster admin. Cluster admin can temporarily set a shorter time period to try to refresh the SCA certs
- The documentation will describe the steps how to pull the SCA certs and update the secret manually

### Test Plan

- `insights-operator-e2e-tests` suite can verify the SCA cert data
  is available
- Basic test of the validity of the SCA certs. Mount the `etc-pki-entitlement` secret and run e.g `yum install` in the container

### Graduation Criteria

This feature is planned as a technical preview in OCP 4.9 and is planned to go GA in 4.10.

#### Dev Preview -> Tech Preview
- opt-in feature (called `InsightsOperatorPullingSCA`) enabled with `TechPreviewNoUpgrade` feature set
- Insights Operator is able to download the certificates from OCM API and expose it in a cluster API
- Insights Operator is marked as degraded in case of the number of unsuccessful requests to the OCM API exceeds defined threshold
- basic functionality is tested
- this new functionality is documented in the Insights Operator documentation as tech preview

#### Tech Preview -> GA
- `Share` resource object is automatically created by the Insights Operator
- inform a cluster user about the error state (problem with pulling the certificates)
- the documentation is revisited and updated
- the use of SCA certs in the `Build` is documented in the Build API documentation
- the feature might be moved to a different OCP component

#### Removing a deprecated feature

The periodical data pulling can be easily disabled in the cluster configuration. Removing this feature will require updating the Insights operator code base and will remove the `etc-pki-entitlement` secret from the `openshift-config-managed` namespace.

### Upgrade / Downgrade Strategy

There is no upgrade/downgrade strategy needed.

### Version Skew Strategy

There is no Skew strategy needed. This work should have no impact on the upgrade. It doesn't require any coordinated behavior in the control plane. No other components will change.

There's no plan to change the OCM API in the near future. The certificate format is versioned, but note that the format is not checked by the Insights Operator.

## Implementation History

There are no other major milestones in the implementation history than the graduation criteria mentioned above.

## Drawbacks

The performance of the OCM API can be a possible drawback, but the certificates are cached in the OCM API server so the expected impact should be minimal.

## Alternatives

- Alternative is to implement this functionality in another control plane component/operator (e.g openshift-controller-manager).
- Another option is to create a new component/operator for this functionality. This would probably require the most effort and would require additional CPU and memory resources in a cluster.
- Current state, which is the manual addition of the SCA certs to cluster worker nodes. This is not very convenient because the SCA certs change regularly and the change requires node reboot.

