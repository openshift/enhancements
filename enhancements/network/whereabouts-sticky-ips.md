---
title: whereabouts-sticky-ips
authors:
  - "@dougbtv"
reviewers:
  - "@s1061123"
  - "@zshi-redhat"
approvers:
  - "@s1061123"
  - "@zshi-redhat"
creation-date: 2020-08-05
last-updated: 2020-08-05
status: provisional
---


# Whereabouts "Sticky IP addresses"

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This feature implements functionality for Whereabouts where workloads are assigned an IP address, and when those workloads are terminated and restarted, are assigned the same IP address.

## Motivation

This functionality has been requested to provide reliable IP addresses for workloads that advertise their IP address in a method other than Kubernetes services, and where external components require that those same IP addresses be reachable regardless if the workload has been restarted.

This has a benefit for workloads for OpenShift Container Storage (which could utilize this functionality to better provide network isolation by using Whereabouts in tandem with Multus CNI)

### Goals

This should be considered complete if a given workload can be assigned an IP address, be restarted, and still be assigned the same IP address.

### Non-Goals

(No relevant non-goals)

## Proposal

### Implementation Details/Notes/Constraints 

Some upstream discussion regarding the design has already occured in [this upstream issue](https://github.com/dougbtv/whereabouts/issues/51).

Generally the proposed design is to create a new CRD that is specific to an individual IP address. This can be used with, or as an extension to data that exists for storing IP addresses individually in order to determine if they are used in overlapping ranges.

Each of these CR's would be labeled with a given subnet/range in order to query for a given subnet/range to determine allocations within that range.

For example:

```
kubectl get ip -l subnet_31=127.0.0.0
```

It is proposed that the method by which a IP address is determined for a given workload is by having a specific MAC address, in a fashion that mimics DHCP.

It is proposed that a pod may have its mac address assigned in a deterministic fashion by using [kubemacpool](https://github.com/k8snetworkplumbingwg/kubemacpool).

### Risks and Mitigations

* This may require additional logic in order to upgrade a refactored CRD and existing data as this will require refactoring the CRD(s) used for Whereabouts.

## Design Details

### Open Questions

 > 1. Decide on new CRD.
 > 2. Is kubemacpool available in OCP without OpenShift Virtualization (KubeVirt)?

### Test Plan

(TBD.)

#### Examples

### Upgrade / Downgrade Strategy

This may require that additional logic is added during an upgrade/downgrade to convert data due to a refactored CRD.