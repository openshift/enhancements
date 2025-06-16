---
title: Setting Pod's storage security policies on per-namespace basis
authors:
  - "@gnufied"
reviewers:
  - "@openshift/storage"
approvers:
  - "@openshift/openshift-architects"
creation-date: 2025-05-22
last-updated: 2025-05-22
status: implementable
see-also:
replaces:
superseded-by:
---

# Defaulting of fsgroupChangePolicy and selinuxChangePolicy for pods in a particular namespace

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We want to allow Openshift admins to change `fsGroupChangePolicy` defaulting policies for certain namespaces so as pods in the given namespace can start faster.

We also want to allow Openshift admins to set `selinuxChangePolicy` for certain namespaces, so as they can opt-out of potentially breaking changes being introduced via `SELinuxMount` feature upcoming in future k8s releases.

## Motivation

### Motivation for fsgroupchangepolicy defaulting

Currently it can take a long time for volume's permission to be changed to match pod's `fsGroup`. This is a well known problem
in Kubernetes and Openshift. See - https://access.redhat.com/solutions/6221251 for more details.

To solve this problem, we introduced `fsGroupChangePolicy` - https://github.com/kubernetes/kubernetes/blob/b5608aea94cfb54fea3a63e1d74235759d036c51/pkg/apis/core/types.go#L3977 , which
can be configured in a `PodSpec` and be set to `OnRootMismatch`, which instructs kubelet to stop recursively changing permissions of a volume, if top level directory permissions match as expected by kubelet.

This usually speeds up pod startup because kubelet does not have to recursively change permission of each file and directory in the volume. This mechanism relies on heuristic that gets applied when recursively changing permissions of a volume -  kubelet changes permissions of top level directory as a last step in the entire process, when it performs full recursive `chown` and `chmod` of the volume.

Having said that, `fsGroupChangePolicy` still defaults to `Always`, which means kubelet will always recursively change permissions. The reason for defaulting to `Always` is - mostly backward compatibility and if users bring in volumes that were used outside kubernetes. In these cases `OnRootMismatch` may not be a safe default.

But for most intent and purpose, it can be a reasonable default and hence this proposal allows admins to configure a namespace such that, all pods created in the namespace will default to `OnRootMismatch` policy if pod does not specify a policy of its own. If a pod specifies its own `fsGroupChangePolicy` that is still used as a default and will not be overridden.

### Motivation for selinuxChangePolicy defaulting

https://github.com/kubernetes/enhancements/issues/1710 is bringing changes which can be breaking for certain pods which use different selinux contexts for same volume between different pods or same pod.

This will speed up time spent while recursively changing selinux policies of a volume.

By allowing cluster-admins to set namespace default of `selinuxChangePolicy` we want to give control over how these changes are applied to Openshift clusters, without explicitly  changing pods themselves.

### Goals

- Allow Openshift admins to configure per-namespace default of `fsGroupChangePolicy`.
- Allow Openshift admins to configure per-namespace default of `selinuxChangePolicy`.

## Non-Goals

* Change Openshift default globally.

## Proposal

### Allow admins to opt-in to `fsGroupChangePolicy` via namespace label

We propose a label `storage.openshift.io/fsgroup-change-policy` if that is set to `OnRootMismatch` in a namespace, then Openshift's SCC hooks start defaulting all pods created in that namespace with `OnRootMismatch` `FSGroupChangePolicy`.

```go
func getPodFsGroupChangePolicy(ns *corev1.Namespace) *api.PodFSGroupChangePolicy {
    fsGroupPolicy, ok := ns.Labels[securityv1.OnRootMismatchFSGroupPolicy]
    if !ok {
        return nil
    }
    if fsGroupPolicy == "OnRootMismatch" {
        onRootMismatchPolicy := api.FSGroupChangeOnRootMismatch
        return &onRootMismatchPolicy
    }
    return nil
}
```

### Allow admins to opt-in to `selinuxChangePolicy` via namespace label

Similar to mechanism proposed above,  we propose to use label `storage.openshift.io/selinux-change-policy` to define namespace wide `selinuxChangePolicy` if none are specified in a pod.

### Setting of `fsGroupChangePolicy` and `selinuxChangePolicy` during admission

We propose to add a new admission hook (which will be eventually replaced with MAP) which will trigger the defautling logic, similar to how SCC matching hooks are triggered.

### Eventual replacement with Mutating Admission policy

We want to replace admission plugin with Mutating Admission Policy once the feature becomes generally available. Once Mutating Admission Policy becomes GA, we propose to replace the admission hook with MAP on the lines of https://docs.google.com/document/d/13xa7imyvQzok3IJ66mheGqemQGhkNySl-He1h1iRuSM/edit?usp=sharing

This would serve similar purpose as admission hook we are carrying as go code.

### Proof of concept

We have already implemented a proof-of-concept https://github.com/openshift/kubernetes/pull/2311/files, which works and does the job. But this uses existing SCC admission hook.

#### Graduation

##### Status during tech-preview

During the initial development this feature will be behind a Openshift feature gate and will not be generally available.

##### GA

This is relatively simple feature and hence, for GA we should be able to just rename the annotation. We could also implement this thing as GA from get-go.
