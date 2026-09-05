---
title: must-gather-operator-policy
authors:
  - "@swghosh"
reviewers:
  - "@shivprakahsmuley"
  - "@Prashanth684"
approvers:
  - "@Prashanth684"
api-approvers:
  - "@shivprakahsmuley"
  - "@Prashanth684"
creation-date: 2025-08-18
last-updated: 2025-10-09
tracking-link:
  - https://redhat.atlassian.net/browse/RFE-9011
  - https://redhat.atlassian.net/browse/OCPSTRAT-3733
  - https://redhat.atlassian.net/browse/MG-348
status: implementable
see-also:

---

# Must Gather Operator Managed Storage Lifecycle for Persistent Volumes

## Summary

Introduce a new cluster-scoped MustGatherPolicy CR that defines PVC profiles and cluster defaults. The existing namespace-scoped MustGather CR references or inherits the policy. A second controller reconciles the policy.

## Motivation

### User Stories

- cluster admins set policy
- users of namespace-scoped MustGather inherit fields set in the policy

- As a user, I want to collect a must-gather through Support Log Gather without having to create PersistentVolumeClaims manually.

### Goals

### Non-Goals

## Proposal

The MustGather reconciler looks up the resolved policy at reconcile time, provisions or reuses a PVC accordingly.

### API Extensions

New CR example: 

```
apiVersion: operator.openshift.io/v1alpha1
kind: MustGatherPolicy      # cluster-scoped
metadata:
  name: policy1
spec:
     storage:
        storageClassName: thin-csi
        size: 50Gi
        retentionPeriod: 7d
        fullThresholdPercent: 85
        expandIfAllowed: true
```

`api/v1/mustgather_types.go`

```go
type MustGatherSpec struct {
    // ...
    // storage is the storage configuration for persisting the collected must-gather tar archive.
	// If not specified, an ephemeral volume is used which will not persist
	// the tar archive on the cluster.
	// +optional
	Storage *Storage `json:"storage,omitempty"`
    
    // ...

    // inheritsPolicy allows to inherit an existing MustGatherPolicy
    // and apply fields present in the policy to be set without
    // explicitly being specified.
    InheritsPolicy *corev1.LocalObjectReference `json:"interitsPolicy,omitempty"`
}


```

### Implementation Details/Notes/Constraints

### Topology Considerations

## Implementation History

- [Must-Gather Operator: PVC destination for gathered data](./must-gather-operator-pvc-destination.md)

## Alternatives (Not Implemented)

## Infrastructure Needed

## Graduation Criteria

### Tech Preview -> GA

Consistently pass rate of 100% for all the e2e test cases over 1 release.

## Upgrade / Downgrade Strategy

## Operational Aspects of API Extensions

## Test Plan

<!--
### Risks and Mitigations

### Drawbacks

### Removing a deprecated feature

## Version Skew Strategy

## Support Procedures

-->
