---
title: pulling-and-updating-pull-secret-from-ocm
authors:
  - "@tremes"
reviewers:
  - "@mrunalp"
  - "@aravindhp"
  - "@inecas"
  - "@rawsyntax"
  - "@markturansky"
approvers:
  - "@mrunalp"
  - "@aravindhp" 
creation-date: 2021-12-01
last-updated: 2021-12-16
status: implementable
see-also:
  - "https://issues.redhat.com/browse/CCXDEV-6529"
  - "https://issues.redhat.com/browse/CCX-185"
---

# Insights Operator pulling and updating `pull-secret` from the OCM API

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created/updated in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement will enable the Insights Operator to pull the updated `pull-secret` (from the OCM API)
and that will enable cluster ownership transfer. Currently, the ownership transfer is a two step process
which involves initiating the transfer from the OpenShift Cluster Manager (OCM) API
and then updating the global `pull-secret` on the cluster with the new owner’s `pull-secret`.
If the `pull-secret` is not changed on the cluster within 5 days, the ownership transfer is canceled
and needs to be initiated again.
This process applies to all scenarios that require cluster ownership transferred across Red Hat accounts.

The intent is to simplify the cluster ownership process to a one step process.

## Motivation

The entry point to many OpenShift subscriptions is for a user to start with a 60 day trial from the OCM with personal credentials
and then migrate it over to a company or organization account. In addition, reports from the field indicate that customers in general when transferring ownership, either follow one of the steps in the process and do not follow through with the other step required to complete the transfer. This enhancement will make the transfer frictionless and provide a better user experience.


### Goals
The goal of this enhancement is to describe the API and the interaction between OCM and the cluster that is being transferred to update the `pull-secret` on that cluster:

- Extend the Insights Operator config with a new OCM API endpoint to be able to query the new `pull-secret` content
- Periodically (once a day) query the new endpoint and if there is a new or updated content then update the `pull-secret`.
  
### Non-Goals
- The enhancement will not describe the OCM user experience.
- The enhancement will not provide a generic way to pull information from OCM into the cluster.
- The process will not apply to disconnected clusters.
- This process will work only on the clusters where the feature was released as part of the Insights operator and will not be backported to older versions.

## Proposal

The Insights operator already communicates with the OCM API (see [Insights Operator pulling and exposing SCA certs from the OCM API](../insights/pulling-sca-certs-from-ocm.md). This is the proposed flow for updating the `pull-secret`:

1. OCM provides an API endpoint that the cluster can call to check the current global updated `pull-secret`.
2. User starts the migration process in the OCM by providing the `pull-secret` associated with the account to which the ownership will be transferred to.
   * The details of whether the user provides the pull secret as a means of verification, or whether a request/approval workflow is implemented, is left to the OCM/UI flow. What is pertinent here is that there is an endpoint available to a cluster entity to fetch the new `pull-secret`.
3. OCM validates the `pull-secret` for the new account and makes it available to the cluster for some time period i.e. the new API will now return the new `pull-secret` provided by the user.
4. The Insights operator calls this API periodically, fetches the updated `pull-secret` and compares it with the current global `pull-secret` on the cluster (only entries available in both pull-secrets are compared).
5. If there is a mismatch, the Insights operator applies the new `pull-secret`.
   * The updated `pull-secret` from the OCM API is an authoritative source. Insights operator replaces and adds (There should not be any new entries, but this may change in the future) the entries provided in the updated `pull-secret` from the OCM API (This means that the Insights operator cannot delete anything in the existing secret).
   * Note: MCO updating the `pull-secret` on the nodes will not result in node reboots. This will require an update of the documentation in https://docs.openshift.com/container-platform/4.9/updating/update-using-custom-machine-config-pools.html#update-using-custom-machine-config-pools-about_update-using-custom-machine-config-pools.
   We should emphasize the need of unpausing the machine config pools to succesfully apply the new `pull-secret` (credentials) to all the nodes.
6. The Insights operator itself starts using the new `pull-secret`
7. Cluster comes up with the new `pull-secret` and starts reporting back to OCM into the new account.

### User Stories

As an OCM user, I would like to transfer cluster ownership from my account to another user's account in one step.
- [CCXDEV-6529 Insights Operator pulling updated pull-secret to transfer unclassified cluster](https://issues.redhat.com/browse/CCXDEV-6529)
- [CCX-185 Transfer of Unclassified clusters - provide pull-secret](https://issues.redhat.com/browse/CCX-185)
### API Extensions

No API extensions.

### Risks and Mitigations

Risk: The OCM API is down or returning HTTP response code >= 500.

Mitigation: The Insights Operator is marked as degraded in such case.

Risk: The `pull-secret` does not work or is otherwise corrupted.

Mitigation: We must refer to the OCP documentation, which describes the manual fix procedure.

Risk: Users don't want to wait 24 hours for the pull secret to be updated on the cluster

Mitigation: User can trigger the update faster by deleting the Insights operator Pod (This needs to be documented and may change in the future - e.g we can introduce a new CRD to configure this).

## Design Details

### Authorization

The Insights Operator is able to pull the updated `pull-secret` content from the OCM API using the existing `cloud.openshift.com` token
available in the `pull-secret` (in the `openshift-config` namespace).

The Insights Operator must provide a cluster ID as an identifier of the cluster.

### OCM API

The OCM API provides an endpoint that will serve the updated `pull-secret` definition when the migration window is opened. The endpoint will be defined in a new `ClusterTransfer` API - more details are in https://gitlab.cee.redhat.com/service/uhc-account-manager/-/blob/master/docs/cluster_transfer_api.md.
The migration window will be opened for certain period of time (e.g 5 days) so the Insights Operator can safely update the secret.

When the migration window is closed, the OCM API returns another HTTP status code and the Insights operator does nothing in such case.

### Monitoring

The OCM API will return time information providing the remaining time until the end of the current migration window.
We decided to not introduce a new in-cluster alert because the main user interaction will happen in the OCM UI (where the alert for pending cluster transfers will be) and mainly because this alert would be displayed to the cluster owner and not to the recipient (who can accept the pending cluster transfer).

There is also plan to create an alert about pending `ClusterTransfer` on the OCM side - see https://issues.redhat.com/browse/SDB-2506,

### Configuration

The OCM API endpoint will be configurable in the Insights operator via the `support` secret (this is curenlty the only way to configure the Insights operator).

### Disconnected clusters

This feature will not work on disconnected clusters because there is no corresponding token (in the `pull-secret`) to communicate with Red Hat. We should emphasize this in https://docs.openshift.com/container-platform/4.9/support/remote_health_monitoring/opting-out-of-remote-health-reporting.html.


### Test Plan

- `insights-operator-e2e-tests` suite can verify that the Insights Operator is able to download and update the `pull-secret` content

### Graduation Criteria

There are no graduation criteria. This feature is planned as a fast track feature.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

There is no upgrade/downgrade strategy needed.

### Version Skew Strategy

The Insights operator expects the `pull-secret` in the `openshift-config` namespace. If there are any changes to the `pull-secret` location or content, then the update of the Insights operator will be required.

The OCM API is versioned and is expected to be backward compatible. A major update of the OCM API will probably require an update of the Insights operator.

### Operational Aspects of API Extensions

No API extension.

#### Failure Modes

There is no plan to provide direct confirmation of a successful `pull-secret` update. The problem is that the Insights operator is unable to provide such confirmation, because the update is followed up by the MachineConfig operator work. Some potential failures/problems are:  

- There are some paused `MachineConfigPools`, which failed to pick up the updated `pull-secret` definition. The old original secret will
still be valid for some period of time, but this can happen regardless of the period. We can create a new Insights rule (checking paused MCPs) to mitigate this.

- Attacker who can forge an X.509 cert and impersonate OCM (or who gains server-side access to OCM) recommends an invalid pull secret.
  This is basically same situation as described in [Risks and mitigations](#risks-and-mitigations) section above. There was a discussion about some pre-apply validation check of the new secret on the cluster side, but the output was that we can't do much about it (in the Insights operator) and we may revisit this in the future.

#### Support Procedures

A new Insights rule can be created based on the alerts described in the [Monitoring](#monitoring) section.

## Implementation History

There are no major milestones in the implementation history.

## Drawbacks

There should not be any significant drawbacks. The possible drawback is that the `pull-secret` update will not take place during the migration window (possible problems are described above). The original secret will continue to work and we will notify user (via alert as described above) and the procedure will need to be repeated.

## Alternatives

- In the future, we may have more flow of data from the OCM API to a cluster and we will revisit if we need a dedicated operator for such flows. However, we do not want to add an entirely new operator for the `pull-secret` flow today.

