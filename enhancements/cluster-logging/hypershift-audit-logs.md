---
title: hypershift-audit-logs
authors:
  - "@alanconway"
reviewers:
  - "@jewzaam"
  - "@eparis"
approvers:
  - "@jewzaam"
api-approvers:
  - "@jewzaam"
creation-date: 2023-03-31
tracking-link:
  - https://issues.redhat.com/browse/LOG-3734
see-also:
  - /enhancements/cluster-logging/api-audit-log-policy.md
---

# Hypershift Audit Logs

## Summary

In a self contained cluster, users can forward application, infrastructure and audit logs.
In a hypershift hosted cluster, API server audit logs are not exposed in the hosted cluster or on the data plane.
They are generated in Hosted Control Plane (HCP) pods on a management cluster and forwarded via HTTP web-hook.

This proposal deploys the existing Cluster Logging Operator (CLO) on management and hosted clusters.
The CLO provides a ClusterLogForwarder resource (CLF) that collects and forwards logs.
On the hosted cluster, a CLF can be configured to receive audit logs from the management cluster
and forward them to customer-configured destinations.

On the management cluster, this proposal adds a "Hypershift Logging Operator" (HLO).
The HLO watches _data-plane_ CLF resources, when it sees one configured to receive audit logs it updates a _control_plane_ CLF to do the forwarding.
The control plane CLF can be configured to forward to additional supported outputs from the control plane directly.

**NOTE**: This proposal depends on [api-audit-log-policy](api-audit-log-policy.md)
Please read that proposal for details not covered here.

## Motivation

- Forward API server audit logs from the management cluster to the hosted cluster, based on hosted cluster configuration.
- Allow hosted cluster user to forward HCP audit logs in the same way as logs collected from the hosted cluster.
- Allow audit logs consumers with different requirements to get the subset logs they need.
- Replace the SD Splunk forwarder with the CLO which is more flexible and feature-rich.

### User Stories

#### As a service consumer, I want to forward selected API audit logs to my chosen destinations

- Have audit logs forwarded automatically from control-plane CLF to data-plane CLF.
- Configure API audit policy and forwarding on my data-plane CLF.
- Forward audit logs in the same way as logs collected from the hosted cluster.

#### As a service provider, I want to forward selected API audit logs to my chosen destinations

- configure the SD internal audit policy
- configure API audit policy and forwarding using name-spaced logging customer resources in the management cluster.
  - for example: Red Hat Service Delivery team forwards audit logs to internal Splunk destination.
- independent of user configuration.

### Goals

- Multiple audit policies can be defined in a management cluster, independent of hosted cluster policies.
- Multiple audit policies can be defined in a hosted cluster, independent of the management cluster policies.
  See [api-audit-log-policy](/api-audit-log-policy.md) for how the CLO will support this.
- Data plane (customer) requests audit log forwarding by modifying data-plane CLF configuration, forwarding from control plane starts automatically.

### Non-Goals

- This proposal only covers API audit logs. No change to application, infrastructure or node audit logs.

## Proposal

### Workflow Description

#### Hosted Cluster, data plane

1. Install the Cluster Logging Operator (CLO).
1. Create a `ClusterLogForwarder` with a HTTP input.
 - Receiver should be identified with a special input name e.g. `hypershiftAPIAudit`
 - Pipelines from the receiver may include an [api-audit-log-policy](/api-audit-log-policy.md) filter.
1. Forward logs to any outputs as desired.

#### Management Cluster, control plane

1. Install the Cluster Logging Operator (CLO).
1. Create a `ClusterLogForwarder` for each HCP to forward to desired outputs.
1. Install the new Hypershift Logging Operator (HLO) watching each HCP namespace.
   - The HLO watches CLF configurations _in the data plane_, not the control plane.
   - When the HLO detects a _data plane_ CLF requesting audit log forwarding, it updates
     the _control plane_ CLF for that HCP to forward audit logs via a HTTP output to the _data plane_ CLF.
   - The HLO should copy audit log policies from  _control plane_ CLF pipelines to the _data plane_,
     to forward only those logs that will also be forwarded by the data plane CLF.

See "API Extensions" and "Implementation Details" for more.

### API Extensions

None. The HLO watches existing resources: HCP on the control-plane, CLF on the data plane.

### Implementation Details

#### Hypershift Logging Operator

The _Hypershift Logging Operator_ (HLO) runs on the management cluster.

**NOTE**: 
- Unlike most operators, the HLO watches resources on both the control plane and the data plane.
- The HLO watches existing resources, it does not add any new custom resources.

Watches:

- Control-plane HCP namespace (created and destroyed.)
- Data-plane CLF resources corresponding to a HCP namespace.

Creates and updates:

- Control-plane CLF resources to add/remove HTTP forwarding as requested by the data-plane CLF.

#### Cluster Logging Operator

The CLO provides the following features (since 5.9) to support this use case:

- **Multiple instances**: Create a ClusterLogForwarder in each HCP namespace.
- **HTTP (aka webhook) input**: Act as a HTTP server to receive logs from the api-audit web hook.
- **API Audit policy filtering**: Provides detailed filter for API audit logs.

### Risks and Mitigations

Once API audit logs can be forwarded from control to data plane, the user has full control over
forwarding including authentication, networking issues etc. as they would on a stand-alone cluster.

Risks of future issues related to authentication and forwarding become part of the normal release
responsibility of the CLO, and should no longer require special actions to work with HCP.

### Drawbacks

## Design Details

### Open Questions

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

None.

### Upgrade / Downgrade Strategy

Upgrading: Existing customer `cl/instance` and `clf/instance` resources will not be modified.
To receive API audit logs, the customer must update their logging configuration as described above.
All logging features other than API audit logs will be unaffected.

Downgrading: removes new features but otherwise works as expected.

### Version Skew Strategy

Data and control plane may have different versions of CLO installed. Points of contact:

- HTTP input-to-output used to forward audit logs.
- API audit policy configuration.

As long as configuration for these two items remains compatible, the CLO versions can vary.
These are protected by normal backwards-compatibility rules between releases.

It is important to verify and test this compatibility with CLO and HCP releases,
so we can advertise the correct supported version ranges.

### Operational Aspects of API Extensions

The HLO operator watches resources on both control and data planes.
This is unusual but not unprecedented, SREs need to be aware to understand and fix problems.

- 2 new operators to run on management clusters: CLO and HLO.
- Custom Splunk forwarder will be removed.

**Note**: The custom Splunk forwarder MUST be removed and replaced with the CLO for all this to work.
For non-hypbershift clusters the Splunk exporter and the CLO can coexist by both scraping the API audit log files.
In hypershift there is no accessible log file, and only one process can act as webhook to intercept the API audit logs.

#### Failure Modes

- Incorrect data-plane CLF configuration for `hypershiftAPIAudit` input.

#### Support Procedures

Why don't I receive audit logs?
- Is the `hypershiftAPIAudit` input missing or incorrectly configured?

## Implementation History

None.

## Alternatives

None.
