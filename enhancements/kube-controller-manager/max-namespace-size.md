---
title: limit-resources-per-namespace
authors:
  - "@deads2k"
reviewers:
  - "@soltysh"
approvers:
  - "@mfojtik"
creation-date: 2020-01-31
last-updated: 2020-01-31
status: implementable
see-also:
replaces:
superseded-by:
---

# Limit Resources Per Namespace

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Create a new `namespace-size-policy-controller` in the existing [cluster-policy-controller](https://github.com/openshift/cluster-policy-controller/).
This controller will create quota objects in every namespace that limit the size of certain error-prone resources.
We will start with secrets and configmaps.
It will be created with a goal of protecting the cluster from accidental runaway resource creation.

## Motivation

Kuberentes clusters are vulnerable to runaway namespace conditions.
This is true regardless of how the cluster is hosted.
While the true concern is total size of objects in etcd, the immediate cause is usually an excessive number of resources in a single namespace.
We can easily carve off enforcement of total object count per namespace in a non-disruptive way that protects against common cases of errors.

### Goals

1. protect the cluster from accidental runaway resource creation.
2. prevent runaway growth instead of simply warning about large sizes

### Non-Goals

1. thwarting evil users trying to break the cluster

## Proposal

Create a resource quota in every namespace that looks roughly like
```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  generateName: namespace-size-policy-
  labels:
    "clusterpolicycontroller.openshift.io/namespace-size": "true"
spec:
    hard:
      count/secrets: "10000"
      count/configmaps: "10000"
```
There will be rules to ensure that quotas aren't likely to disrupt existing workloads and to allow cluster-admins to effectively bypass the limits.

Rules for how to create namespace-size-policy- quota resources
 1. check current number of secrets (or other resource)
 2. multiply that number by 100 and round down to the nearest power of 10. 
 3. the minimum value is 10,000.
 
Rules for how to update namespace-size-policy- quota resources
 1. if any controlled resource is not controlled, calculate the limit.
 2. don't reset any controlled resource's value.  Cluster-admins are allowed to change the limit if needed.
 
These rules allow a cluster-admin to allow an individual namespace to go past their normal limits.
These rules allow us to safely turn on this quota in existing customer clusters without breaking their current workloads.

We can turn on monitoring (and possibly alerting) for cases where quota is bumping against the configured limits.
This is generally useful because it indicates that the namespace cannot perform as it is expecting, regardless of which quota it is.

### Risks and Mitigations

Establishing quota in all namespaces where we used to have none is potentially unexpected.
By choosing a very high minimum value, ten thousand, we limit the impact to truly pathological cases while still protecting clusters.

## Design Details

### Test Plan

Add an e2e test that ensures the quota is created as expected.
Unit tests that ensure that the recalculation procedures and new resource protection limits work.

### Upgrade / Downgrade Strategy

On an upgrade, the quota will be created.

On downgrade, the quota will be enforced, but new quotas will not be created.

### Version Skew Strategy

Quota has existed for several years, this is not sensitive.

## Drawbacks


## Alternatives

1. Alerting. We could simply alert on large object counts.
   The alerting is not valid as clusters grow.
   Alerts don't prevent problems, they only call out risk. 
   Quota prevents the problems from developing, while allowing an escape hatch for valid use-cases.
