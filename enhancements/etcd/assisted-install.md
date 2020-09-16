---
title: support-assisted-installer

authors:
  - "@hexfusion"
reviewers:
  - "@deads2k"
  - "@skolicha"
  - "@ironcladou"
approvers:
  - TBD
creation-date: 2020-09-16
last-updated: 2020-11-12
status: provisional
---

# add cluster-etcd-operator support for assisted installer

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
The assisted installer was designed to utilize three bare-metal machines to
provision the control-plane. In order to do this after bootstrap is complete
the bootstrap node is repurposed into the third master node. The assisted
installer requires etcd to maintain a quorum of two for a short period of
time until it reprovisions the bootstrap node as the third master.

## Motivation
Currently the `cluster-etcd-operator` does not tolerate less than 3 master
nodes during install time.

Reasons outlined for cluster with less than 3 master nodes as unsafe non HA.
1. Violates operational best practices for etcd - unstable
1. Allows non-HA configurations which we cannot support in production - unsafe and non-HA
1. Allows a situation where we can get stuck unable to re-achieve quorum, resulting in cluster-death - unsafe, non-HA, non-production, unstable
1. The combination of all these things makes the situation unsupportable. 

To allow toleration of this check we provided an unsupportedConfigOverrides
`useUnsupportedUnsafeNonHANonProductionUnstableEtcd`. As assisted installer is
now moving to GA a supported toleration is required.

## Goals

1.  Provide a supported solution for assisted installer that will allow
    `cluster-etcd-operator` to tolerate scaling etcd on 2 master replicas.

## Non-Goals

1.  Add support for less than 3 master node clusters
1.  Support the current unsupportedConfigOverrides flag

## Currently Supported Functionality 
Currently `cluster-etcd-operator` and OpenShift only supports clusters with
three master nodes as install complete.

### EnvVarController
Currently the `EnvVarController` ensures that all the data required for the
etcd static pods exists. A condition exists within the `getEtcdEnvVars`[1]  function which checks if we have the expected master node count or if
`useUnsupportedUnsafeNonHANonProductionUnstableEtcd` is populated otherwise
return error which results in the controller being degraded and no static-pod
spec installed on the master node for etcd.

[1] https://github.com/openshift/cluster-etcd-operator/blob/be2b8d97ca048baecb051d288971a9f6db411457/pkg/etcdenvvar/etcd_env.go#L71

## Proposal
The following changes are proposed.

  - etcd-operator changes: https://github.com/openshift/cluster-etcd-operator/pull/449

### Design Details
In order for the operator to tolerate an unsafe install configuration all
efforts must be taken to minimize the risk of tolerating user defined unsafe
cluster. For example to save on cost of cluster user chooses 2 master replicas instead of 3.

1.  Observe `assisted-install-bootstrap.complete` file in /opt/openshift directory of bootstrap node.
1.  Write annotation to operand config during render.
1.  Observe the annotation as a condition to tolerate scaling etcd with less than three nodes.
1.  Observe install status and only tolerate less than 3 masters during the time
    of bootstrap complete until install complete. After install status is `complete`
    the operator will go into a degraded state until the cluster has 3 or more master nodes.

### bootstrap configmap (no change)
The `bootstrap` configmap in the `kube-system` namespace is where the value for
`install status` is persisted. The design will observe this value as a
condition. After install status is complete the `cluster-etcd-operator` will no longer
tolerate less than 3 master nodes and will go into a degraded state. Once the
master count. If the configmap is missing or status is invalid the operator will
go degraded. 

bootstrap configmap
```
apiVersion: v1
kind: ConfigMap
metadata:
  creationTimestamp: "2020-09-16T12:23:33Z"
  name: bootstrap
  namespace: kube-system
data:
  status: complete
```

### Extend checks in EnvVarController
Currently, the `EnvVarController` ensures that all the data required for the
etcd static pods exists. A condition exists within `getEtcdEnvVars` which checks
if we have the expected master node count otherwise returns error. When the operator observes the annotation it will tolerate scaling up etcd with 2 master nodes instead of 3. 
#### Reads
1.  `oc get cm -n kube-system bootstrap`
1.  `oc get cm -n openshift-etcd config`

## User Stories

## Test Plan

Testing of the feature will be done using the e2e-metal-assisted CI job and periodic tests.

## Graduation Criteria

This enhancement will follow standard graduation criteria.

## Drawbacks

