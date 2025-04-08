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

This proposal deploys the Cluster Logging Operator (CLO) on both management and hosted clusters.
The CLO provides a ClusterLogForwarder resource (CLF) that collects and forwards logs.

On the hosted cluster, the customer configures their own CLF for local log collection and forwarding.
With this proposal, the customer can also configure a HTTP receiver in their CLF, and annotate it to receive audit logs from the management cluster.
The customer can configure forwarding of audit logs using any of the CLF routing and forwarding features, just like local logs.

On the management cluster, this proposal adds a "Hypershift Logging Operator" (HLO), which is invisible to the customer.
The HLO watches _data-plane_ CLF resources.
When it sees one configured to receive audit logs, it updates a _control-plane_ CLF instance to do the forwarding via HTTP.

If required, the control plane CLF can forward audit logs to other locations using existing CLF features.
For example, in the case of managed ROSA it can forward to SD Splunk and/or directly to a customer CloudWatch account.

## Motivation

- Forward API server audit logs from the management cluster to the hosted cluster, based on _data-plane_ CLF configuration.
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

- This proposal only covers API audit logs.
  Support for application, infrastructure or node audit logs could be added but is not discussed here.

## Proposal

### Workflow Description

#### Hosted Cluster, data plane

1. Install the Cluster Logging Operator (CLO).
1. Create a `ClusterLogForwarder`, which MAY be configured to collect and forward local logs.
1. Create a HTTP receiver for audit logs from the HCP control plane.
1. Annotate the CLF so that HCP control plane will forward audit logs:
  - `logging.openshift.io/hypershift/forward-audit: <name-of-http-receiver>`
1. Pipelines from the receiver MAY include an [api-audit-log-policy](/api-audit-log-policy.md) filter.
1. Audit logs can be forwarded to any pipeline/output as desired.

#### Management Cluster, control plane

1. Install the Cluster Logging Operator (CLO) and Hypershift Log Operator (HLO)
1. Create a _control-plane_ CLF instance in each HCP
   - May forward logs to other locations, e.g. Splunk and/or CloudWatch for managed ROSA.
1. Hypershift Logging Operator (HLO) operates as follows:
   - Watches CLF resources in the _data plane_, not the control plane.
   - When a _data-plane_ CLF has `logging.openshift.io/hypershift/forward-audit` annotation.
     - Modify the corresponding _control plane_ CLF to add a HTTP output, configured to send to the _data plane_ receiver.

### API Extensions

- The HLO watches existing resources, it does not add any new custom resources.
- HLO must handle the case of a private hosted cluster, implying traffic to data plane must traverse private connection over konnectivity.

### Implementation Details

#### Hypershift Logging Operator

The _Hypershift Logging Operator_ (HLO) runs on the management cluster.

**NOTE**:
- Unlike most operators, the HLO watches resources on both the control plane and the data plane.
- The HLO watches existing resources, it does not add any new custom resources.

Watches:
- Data-plane CLF resources corresponding to a HCP namespace.

Creates and updates:
- Control-plane CLF resources to add/remove HTTP forwarding as requested by the data-plane CLF.

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

The HLO needs to be able to:
- read a subset of data plane CLF configuration: spec.inputs.receivers.http
- write a subset of control-plane CLO configuration: spec.outputs.http, spec.outputs.pipeline, spec.inputs.audit

If there are incompatible changes in these subsets, the HLO needs to be able to accommodate the range of supported versions.
Note the management-plane and data-plane version ranges do not have to be identical.

Data and control plane communicate via HTTP log forwarding protocol.
The CLO versions must be in a range that can use the same HTTP protocol.

### Operational Aspects of API Extensions

The HLO operator watches resources on both control and data planes.
This is unusual but not unprecedented, SREs need to be aware to understand and fix problems.

For managed ROSA:

- 2 new operators to run on management clusters: CLO and HLO.
- Webhook configuration for audit logs MUST be modified to forward to CLO instead of custom Splunk forwarder.

**Note**: For non-hypershift clusters the Splunk exporter and the CLO can coexist by both scraping the API audit log files.
In hypershift there is no accessible log file, and only one process can act as webhook to intercept the API audit logs.
The CLO can read the webhook and forward to Splunk as well as other destinations.

#### Failure Modes

- Incorrect data-plane CLF configuration for `hypershiftAPIAudit` input.

#### Support Procedures

Why don't I receive audit logs?
- Is the `hypershiftAPIAudit` input missing or incorrectly configured?

## Implementation History

None.

## Alternatives (Not Implemented)

None.
