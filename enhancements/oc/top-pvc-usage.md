---
title: top-pvc-usage
authors:
- "@gmeghnag"
- "@tsmetana"
reviewers:
- "@dobsonj"
- "@ardaguclu"
approvers:
- "@ardaguclu"
creation-date: 2024-10-23
last-updated: 2024-10-23
status:
see-also:
replaces:
superseded-by:
---

# top pvc

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Provide a simple command to display a PersistentVolumeClaim capacity usage.

## Motivation

There already exist options like `oc adm top pod` that can display usage
statistic of the server resources. Adding an option to display also usage
of the PersistentVolumeClaims seems like a natural extension which provides
important information about the persistent storage available to the workloads.

The volume usage statistics are already available via the web console but we
should also have a way to get this through the CLI too.

### Goals

1. Provide a simple CLI option to display filesystem usage of
   PersistentVolumeClaims.
2. Display only the percentual usage for a given PersistentVolumeClaim or all
   PersistentVolumeClaims in a given namespace.

### Non-Goals

## Proposal

Implement `oc adm top persistentvolumeclaims` command that would show usage
statistic of the bound PersistentVolumeClaim like this:

```text
oc adm top pvc -n reproducer-pvc
NAMESPACE      NAME               USAGE(%) 
reproducer-pvc pvc-reproducer-pvc 98.28    
reproducer-pvc pvc-test-pvc       14.56   
```

### User Stories

#### Story 1

As an OCP project user, I want to see a list of all PVC's in my namespace and
their space consumption so I know if my application has enough storage space.

#### Story 2

As an OCP cluster admin, I want to see a list of all PVC's on the cluster and
their space consumption so I know if any components are running out of storage
space.

### Implementation Details/Notes/Constraints

There are the `kubelet_volume_stats_used_bytes` and
`kubelet_volume_stats_capacity_bytes` Prometheus metrics which can be used to
compute the volume used space percentage. This should also ensure consistency
with the PVC usage data presented by the web console.

For the initial implementation the output should display only the usage percent
value. This could be implemented by a single call to Prometheus API using PromQL
and would not need additional API calls.

The additional columns (e.g. volume capacity, absolute value of the free space)
might be added in later iterations and "hidden" behind an optional parameter to
maintain the backward compatibility.

### Risks and Mitigations

## Design Details

### Output Format

### Test Plan

There is an e2e test that makes sure the command always exits successfully and
that certain apsects of the content are always present.

### Graduation Criteria

### Upgrade / Downgrade Strategy

### Version Skew Strategy

The `oc` command must skew +/- one like normal commands.

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed
