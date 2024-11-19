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
api-approvers:
- "@ardaguclu"
creation-date: 2024-10-21
last-updated: 2024-10-29
tracking-link:
- https://issues.redhat.com/browse/STOR-2040
see-also:
- https://issues.redhat.com/browse/STOR-1852
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

There already exist options like `oc adm top pod` that can display usage statistic of the server resources. Adding an option
to display also usage of the PersistentVolumeClaims seems like a natural extension which provides important information about
the persistent storage available to the workloads.

The volume usage statistics are already available via the web console but we should also have a way to get this through the CLI too.

### User Stories

#### Story 1

As an OCP project user, I want to see a list of all PVC's in my namespace and their space consumption so I know if my application has enough storage space.

#### Story 2

As an OCP cluster admin, I want to see a list of all PVC's on the cluster and their space consumption so I know if any components are running out of storage space.

### Goals

1. Provide a simple CLI option to display filesystem usage of PersistentVolumeClaims. 
2. Display only the percentual usage for a given PersistentVolumeClaim or all PersistentVolumeClaims in a given namespace.

### Non-Goals


## Proposal

Implement `oc adm top persistentvolumeclaims` command that would show usage statistic of the bound PersistentVolumeClaim like this
```text
oc adm top pvc -n reproducer-pvc
NAMESPACE      NAME               USAGE(%) 
reproducer-pvc pvc-reproducer-pvc 98.28    
reproducer-pvc pvc-test-pvc       14.56   
```

### Workflow Description

See the proposal section.

### API Extensions

N/A

### Topology Considerations

N/A

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

The feature should work with any cluster providing the required Prometheus
metrics.

#### Single-node Deployments or MicroShift

This change is in the CLI client and as such does not affect the cluster
requirements or footprint, however it will not work if the Prometheus
metrics are not available which might be the case for the small footprint
OpenShift variants (MicroShift).

### Implementation Details/Notes/Constraints

There are the `kubelet_volume_stats_used_bytes` and `kubelet_volume_stats_capacity_bytes` Prometheus metrics which can be used to compute the volume
used space percentage. This should also ensure consistency with the PVC usage data presented by the web console.

In case the Prometheus pods are down the command would fail:

```
$ oc adm top persistentvolumeclaims 
error: failed to get persistentvolumeclaims from Prometheus: unable to get /api/v1/query from URI in the openshift-monitoring/prometheus-k8s Route: prometheus-k8s-openshift-monitoring.apps.ocp1234.lab.local->GET status code=503
```

Additionally the users need at least the `ClusterRole/cluster-monitoring-view`
and the `get/list` verbs on `Route` objects in the `openshift-monitoring` namespace
permissions to run the command successfully. The necessary permissions can be set
by the cluster admin using

```
$ oc create clusterrole routes-view --verb=get,list --resource=routes -n openshift-monitoring
$ oc adm policy add-cluster-role-to-user routes-view <USER>
$ oc adm policy add-cluster-role-to-user cluster-monitoring-view <USER>
```

### Risks and Mitigations

The command output format should remain stable. This means any future changes
or improvements must be done in a backwards-compatible way in order to not to
break any existing workflows.

### Drawbacks

Since the feature requires the metrics server to be running, it is possible
for it to fail in case the Prometheus metrics server is not available.

## Test Plan

There is an e2e test that makes sure the command always exits successfully and that certain apsects of the content
are always present.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- The feature would be implemented in the upstream `kubectl` after the OpenShift TP release
- More testing (automated, e2e)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Graduation from TP to GA should happend after the upstream `kubectl` goes GA

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

The `oc` command must skew +/- one like normal commands.

## Operational Aspects of API Extensions

N/A

## Support Procedures

The feature is fairly isoloated and does not make changes to the cluster
itself. As such it should not cause any 

## Alternatives

It is possible to obtain the provided information already, e.g. by running
the `df` command on a mounted volume from a pod or querying PV, PVC and node
stats summary manually. This is however quite complicated and inconvenient.
