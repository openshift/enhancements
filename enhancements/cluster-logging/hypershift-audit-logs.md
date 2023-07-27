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
In a hypershift hosted cluster, API server audit logs are not exposed in the hosted cluster or on the data plane,
they are generated in Hosted Control Plane (HCP) pods on a management cluster.

This proposal deploys the existing Cluster Logging Operator (CLO) on management and hosted clusters.
The CLO supports both the cluster service provider and cluster service consumer personas by
permitting destinations to be configured on the hosted control plane (for service provider) and
the data plane (for service consumer) independently.  Each destination may independently filter
events forwarded to each destination by way of an optional Policy.

This proposal adds a "Hypershift Logging Operator" (HLO) on the management cluster.
The HLO combines data-plane and control-plane configuration on the control plane,
so that the CLO on the _management_ cluster can forward API audit logs for both personas.
The CLO on the _hosted_ cluster continues to forward other logs for the consumer persona as before.

**NOTE**: This proposal depends on [api-audit-log-policy](api-audit-log-policy.md)
Please read that proposal for details not covered here.

## Motivation

- Allow audit logs consumers with different requirements to get the subset logs they need.
- Forward API server audit logs from the management cluster, as they are not available in the hosted cluster.
- Replace the SD Splunk forwarder with the CLO which is more flexible and feature-rich.
- Separate configuration so that service consumer can request management cluster forwarding, but the service provider has final control.

### User Stories

#### As a service consumer, I want to forward selected API audit logs to my chosen destinations

- configure API audit policy and forwarding using hosted cluster resource.
- have the desired audit logs forwarded from the management cluster to my destination.

#### As a service provider, I want to forward selected API audit logs to my chosen destinations
- configure the SD internal audit policy
- configure API audit policy and forwarding using name-spaced logging customer resources in the management cluster.
  - for example: Red Hat Service Delivery team forwards audit logs to internal splunk destination.
- independent of user configuration.

### Goals

- Multiple audit policies can be defined in a management cluster, independent of hosted cluster policies.
- Multiple audit policies can be defined in a hosted cluster, independent of the management cluster policies.
  See [api-audit-log-policy](/api-audit-log-policy.md) for how the CLO will support this.
- Allow some forwarding in the _management_ cluster to be configured from the _hosted_ cluster.
- Keep direct control of management cluster forwarding on the management cluster.
  - hosted cluster has no direct access to control plane configuration.
  - data plane resources can request forwarding by the management cluster.
  - management cluster HLO automatically validates data-plane requests, and enables them if they are safe.

### Non-Goals

- This proposal only covers API audit logs. No change to application, infrastructure or node audit logs.
- Management cluster is not _required_ to honour all possible forwarding configurations allowed by CLF. \

## Proposal

### Workflow Description

#### Hosted Cluster, data plane

1. Create a `Policy.audit.k8s.io` resource with the desired policy
   (see [api-audit-log-policy](/api-audit-log-policy.md))
1. Create a `HypershiftLogForwarder` (HLF) resource with an `audit` input linked to the policy.
1. Create HLF `outputs` and `pipelines` to forward the resulting logs.
1. Optionally create a `ClusterLogForwarder` to forward other logs (not API-audit) as usual.

#### Management Cluster, control plane

On the _management_ cluster
1. Install the Cluster Logging Operator (CLO), watching all name-spaces.
1. Install the new Hypershift Logging Operator (HLO), watching all name-spaces.
1. In each new HCP name-space, create a `ClusterLogForwarderTemplate` resource. \
   This resource may be identical for all HCPs or customized per HCP.

See "API Extensions" and "Implementation Details" for more.

### API Extensions

#### HypershiftLogForwarder

The `HyperShiftLogForwarder` resource is created on the data plane (_hosted_ cluster),
but is reconciled by the HLO on the _management_ cluster.

This allows both service consumer and provider to create forwarding configuration,
without exposing service provider configuration to the consumer.

`HyperShiftLogForwarder` (HFL) is identical to the `ClusterLogForwarder` (CLO )API except that:
- Only the `audit` input is allowed, and only API server audit logs will be forwarded.
- Any pipeline or output features that cannot be used with an `audit` input are disabled.
- The HLF resource is created on the _hosted_ cluster, but describes forwarding for the HCP on the _management_ cluster.

#### ClusterLogForwarderTemplate

The `ClusterLogForwarderTemplate` is created in a HCP name-space on the _management_ cluster.
It contains a `ClusterLogForwarder` for the service provider.

Reconciled by the _Hypershift Logging Operator_ on the _management_ cluster.
- Watch `HyperShiftLogForwarder.spec` in the _hosted_ cluster:
  - Prefix input, output and pipeline names from the `HyperShiftLogForwarder` with "hosted_".
  - Append "hosted_" items to the `ClusterLogForwarderTemplate`
  - Create a new `ClusterLogForwarder` in the HCP containing the combined data.
- Watch `ClusterLogForwarder.status` in the HCP name-space
  - Propagate error conditions to the `HyperShiftLogForwarder.status` in the _hosted_ cluster (remove "hosted_" prefix).

### Implementation Details

#### Hypershift Logging Operator

The _Hypershift Logging Operator_ (HLO) runs on the management cluster.

**NOTE**: Unlike most operators, it watches resources on both the control plane and the data plane.

- Control plane 
  - HCP namespaces (goal is to enable logging in each of these)
  - `ClusterLogForwarderTemplate` (service provider logging configuration)
- Data plane
  - `HypershiftLogForwarder` (service consumer logging configuration)
  - `Policy.audit.k8s.io.` (service consumer audit policy)

Reconciliation:

- Validate `HypershiftLogForwarder` resource in the data plane
- Copy resources referred to by the HLF from data to control plane:
  - `Policy` resources for audit policies
  - `Secret` and `ConfigMap` resources for log outputs
- Copy/generate non-resource credentials required by the HLF
- Update `ClusterLogForwarder` resource in the HCP namespace (control plane) combining:
  - HLF configuration from the data plane (if present)
  - CLF template from the `ClusterLogForwarderTemplate` from the control plane (if present)

The result is a CLF resource in each HCP namespace that combines valid service consumer configuration (from data plane)
with service provider configuration (from the control plane).

The CLO manages these resources as normal, by deploying collectors to forward logs.

#### Cluster Logging Operator

Two new CLO features are needed:

- **Multiple instances**: The ClusterLogForwarder is currently a singleton. \
  To provide isolation between HCPs we need to deploy a forwarder per HCP namespace.
  This requires modifications to the CLO.
  See [LOG-1343 Multiple forwarders](https://issues.redhat.com/browse/LOG-1343).

- **HTTP (aka webhook) server**: The ClusterLogForwarder needs to be extended to act a a HTTP listener,
  so it can receive logs as a web hook from the API server in the HCP.
  See [LOG-3965 Collector to act as http server](https://issues.redhat.com/browse/LOG-3965)

### Risks and Mitigations

**NOTE**: See also "Risks and Mitigations" in [api-audit-log-policy](/api-audit-log-policy.md)

#### STS Authentication

CLO needs to refresh tokens from the HCP in order to communicate w/ CloudWatch.
Transplant the relevant code from the modified splunk-exporter used for Hypershift GA.

Tracked by:  [LOG-4029 Support STS Cloudwatch authentication for logging in Managed Clusters](https://issues.redhat.com/browse/LOG-4029)

### Drawbacks

## Design Details

### Open Questions

Need to ensure the management cluster can get credentials to connect to log outputs.
Credentials in Secrets are not a problem, but credentials that are magically auto-mounted in various ways may be tricky.
Need to review security requirements of existing outputs.

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

The CLO's installed on management and hosted clusters may be different versions.

The CLO on the _management_ cluster forwards
- All logs required by the provider, configured by `ClusterLogForwarderTemplate` on _management_ cluster
- API-audit logs _only_ for the customer, configured by `HypershiftLogForwarder` on _hosted_ cluster

The CLO on the _hosted_ cluster forwards
- All non-API-audit logs, configured by `ClusterLogForwarder` on the _hosted_ cluster.

If the hosted/managent CLO versions are different, the customer can
- Create `ClusterLogForwarder` based on the _hosted_ CLO version.
- Create `HypershiftLogForwarder` based on the _management_ CLO version.

This might be confusing but there is a clear separation of resources associated with each version.
- There's no situation where the customer is blocked from upgrading their hosted CLO.
- A customer could be blocked from sending API-audit logs (only) to a new type of log store if the SD CLO is old.

To manage version skew we will:
- minimize API change via normal API compatibility practices.
- identify supported version range, code the HLO to correct or reject configuration version mismatches in that range.

### Operational Aspects of API Extensions

The HLO operator watches resources on both control and data planes.
This is unusual but not unprecedented, SREs need to be aware to understand and fix problems.

- 2 new operators to run on management clusters: CLO and HLO.
- `ClusterLogForwarderTemplate` and `Policy` resource on the management cluster enables SRE Splunk forwarding as before.
- Custom Splunk forwarder will be removed.

**Note**: The custom splunk forwarder MUST be removed and replaced with the CLO for all this to work.
For non-hypbershift clusters the splunk exporter and the CLO can coexist by both scraping the API audit log files.
In hypershift there is no accessible log file, and only one process can act as webhook to intercept the API audit logs.

#### Failure Modes

- Invalid `HyperShiftLogForwarder.spec`: Indicate in `HyperShiftLogForwarder.status`
- Invalid policy reference or policy: Indicate in `HyperShiftLogForwarder.status`

#### Support Procedures

TBD

## Implementation History

None.

## Alternatives

The current work around solves the immediate problem (Cloudwatch yes-or-no), but is not a good long term solution:
- Requires a call to support to enable, can't be controlled from hosted cluster.
- Custom Cloudwatch client code hastily added to splunk exporter duplicates a CLO feature.
- Exporter was not intended to be a client, just a log filter.
- SD splunk forwarder duplicates Splunk output in the CLO.
- Was rushed to support 2 fixed outputs - Splunk and Cloudwatch
  - cannot easily extend to new output types, or more outputs.
