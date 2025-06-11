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

We want to allow Openshift admins to change `FSGroupChangePolicy` defaulting policies for certain namespaces so as pods in the given namespace can start faster.

We also want to allow Openshift admins to set `SELinuxChangePolicy` for certain namespaces, so as they can opt-out of potentially breaking changes being introduced via `SELinuxMount` feature upcoming in future k8s releases.

## Motivation

### Motivation for fsgroupchangepolicy defaulting

Currently it can take a long time for volume's permission to be changed to match pod's `fsGroup`. This is a well known problem
in Kubernetes and Openshift. See - https://access.redhat.com/solutions/6221251 for more details.

To solve this problem, we introduced `FSGroupChangePolicy` - https://github.com/kubernetes/kubernetes/blob/b5608aea94cfb54fea3a63e1d74235759d036c51/pkg/apis/core/types.go#L3977 , which
can be configured in a `PodSpec` and be set to `OnRootMismatch`, which instructs kubelet to stop recursively changing permissions of a volume, if top level directory permissions match as expected by kubelet.

This usually speeds up pod startup because kubelet does not have to recursively change permission of each file and directory in the volume. This mechanism relies on heuristic that gets applied when recursively changing permissions of a volume -  kubelet changes permissions of top level directory as a last step in the entire process, when it performs full recursive `chown` and `chmod` of the volume.

Having said that, `FSGroupChangePolicy` still defaults to `Always`, which means kubelet will always recursively change permissions. The reason for defaulting to `Always` is - mostly backward compatibility and if users bring in volumes that were used outside kubernetes. In these cases `OnRootMismatch` may not be a safe default.

But for most intent and purpose, it can be a reasonable default and hence this proposal allows admins to configure a namespace such that, all pods created in the namespace will default to `OnRootMismatch` policy if pod does not specify a policy of its own. If a pod specifies its own `FSGroupChangePolicy` that is still used as a default and will not be overridden.

### Goals

Allow Openshift admins to configure per-namespace default of `FSGroupChangePolicy`.

## Non-Goals

* Change Openshift default globally.

## Proposal

### Allow admins to opt-in to `FSGroupChangePolicy` via namespace annotation

We propose an annotation `openshift.io/fsgroup-change-policy` if that is set to `OnRootMismatch` in a namespace, then Openshift's SCC hooks start defaulting all pods created in that namespace with `OnRootMismatch` `FSGroupChangePolicy`.

```go
func getPodFsGroupChangePolicy(ns *corev1.Namespace) *api.PodFSGroupChangePolicy {
    fsGroupPolicy, ok := ns.Annotations[securityv1.OnRootMismatchFSGroupPolicy]
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

### Set default `FSGroupChangePolicy`in pods during admission:

We propose that, `CreatePodSecurityContext` function in sccmatching admission code to be enhanced, so as it can also start setting `FSGroupChangePolicy` if none are specified.


For example:

```go

// set the FSGroupChangePolicy if it is not set on the pod and the SCC has a policy
if sc.FSGroup() != nil && s.fsGroupChangePolicy != nil && sc.FSGroupChangePolicy() == nil {
    sc.SetFSGroupChangePolicy(s.fsGroupChangePolicy)
}
```

https://github.com/openshift/apiserver-library-go/blob/master/pkg/securitycontextconstraints/sccmatching/provider.go#L115

### Proof of concept

We have already implemented a proof-of-concept https://github.com/openshift/kubernetes/pull/2311/files, which works and does the job.

#### Graduation

##### Status during tech-preview

Since this feature requires explicit action by the admin, I am not *yet* proposing a feature-gate for this (although I am not opposed to the idea).
If required during TP phase, we can name the annotation in such a way that, it is clear to the user that this is a tech-preview feature. For example - `preview.openshift.io/fsgroup-change-policy`.

##### GA

This is relatively simple feature and hence, for GA we should be able to just rename the annotation. We could also implement this thing as GA from get-go.
