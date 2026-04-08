# Enhancement Index

**Last Updated**: 2026-04-08  

## Purpose

Index of OpenShift enhancement proposals organized by category.

## Location

All enhancements: [/enhancements/](../../enhancements/)

## By Category

### Machine Config

- [MCO RHCOS Layering](../../enhancements/machine-config/mco-rhcos-layering.md)
- [MCO Ignition Version Updates](../../enhancements/machine-config/mco-ignition-version.md)

### Networking

- [OVN-Kubernetes Default](../../enhancements/network/ovn-kubernetes-default.md)
- [Network Live Migration](../../enhancements/network/network-live-migration.md)
- [Dual-stack Support](../../enhancements/network/dual-stack.md)

### Cluster Lifecycle

- [Cluster Profiles](../../enhancements/update/cluster-profiles.md)
- [Update Recommendations](../../enhancements/update/update-recommendations.md)

### Storage

- [CSI Migration](../../enhancements/storage/csi-migration.md)
- [Generic Ephemeral Volumes](../../enhancements/storage/generic-ephemeral-volumes.md)

### Authentication

- [OIDC Support](../../enhancements/authentication/oidc-support.md)
- [Token Bound Service Accounts](../../enhancements/authentication/token-bound-service-accounts.md)

### Monitoring

- [User Workload Monitoring](../../enhancements/monitoring/user-workload-monitoring.md)
- [Monitoring Stack Upgrades](../../enhancements/monitoring/monitoring-stack-upgrades.md)

## By Status

### Implemented (GA)

Features in stable, production-ready state.

### Beta

Features supported but API may evolve.

### Alpha / Tech Preview

Experimental features, may change or be removed.

### Provisional

Proposals under review, not yet approved.

## By Release

### 4.16

- Feature A
- Feature B

### 4.15

- Feature C
- Feature D

### 4.14

- Feature E
- Feature F

## How to Use

### Finding Enhancement

**By feature name**: Search [/enhancements/](../../enhancements/) directory

**By component**: See [Repository Index](./repo-index.md), then check component's enhancements

**By status**: Filter GitHub issues/PRs in openshift/enhancements repo

### Reading Enhancement

Enhancements follow standard template:
- **Summary**: One-paragraph overview
- **Motivation**: Why this feature is needed
- **Proposal**: How it will work
- **Design Details**: Implementation specifics

See [Enhancement Process](../workflows/enhancement-process.md)

## Creating Enhancement

See [Enhancement Process](../workflows/enhancement-process.md) for full workflow.

Quick start:
1. Copy `enhancements/TEMPLATE.md`
2. Fill in all sections
3. Submit PR to openshift/enhancements
4. Address review feedback
5. Get approval from area owners

## See Also

- [Enhancement Repository](https://github.com/openshift/enhancements)
- [Enhancement Process](../workflows/enhancement-process.md)
- [Repository Index](./repo-index.md)
