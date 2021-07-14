---
title: Fallback on Failing Revisions of Static Pods
authors:
- "@sttts"
  reviewers:
- "@p0lyn0mial"
- "@mfojtik"
- "@soltysh"
- "@marun"
- "@deads2k"
  approvers:
- "@mfojtik"
- "@soltysh"
- "@hexfusion"
creation-date: 2021-06-21
last-updated: 2021-07-14
status: implementable
# provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
# see-also:
#  - "/enhancements/this-other-neat-thing.md"
# replaces:
#  - "/enhancements/that-less-than-great-idea.md"
# superseded-by:
#  - "/enhancements/our-past-effort.md"
---

# Fallback on Failing Revisions of Static Pods

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Static pods operators use a revisioned pod manifest and revisioned configmap and secret manifests to roll out and start a new configuration of the operand in an atomic way.

Configuration can be bad in different ways. In HA OpenShift, the static pod operator will wait for operands on one node to start up and to become healthy and ready. The operator will stop rolling out bad revisions further.

In SNO, there is no other pod to ensure availability when a roll-out of a new revision happens. Hence, a bad revision can be fatal for the cluster, especially for the kube-apiserver and etcd.

This enhancement is describing a fallback mechanism that will make the operand to revert to the last healthy revision when the new revision fails, and how the operator will notice this event and how it will react.

The fallback mechanism is opt-in by the operator, and kube-apiserver and etcd operators (and potentially kcm and ks) have to consult the deployment topology in the infrastructure resource to decide.

## Motivation

In SNO, we only have one chance to start the new static pod for the kube-apiserver or etcd. When this fails, the cluster is bricked and there is no automatic recovery.

Bad configurations can have many reasons:
- **the pod manifest can be wrong**: e.g. by YAML syntax, not validating as a pod or some deeper semantical error:
    - **each one of the [23 different config observers of kube-apiserver]([https://](https://github.com/openshift/cluster-kube-apiserver-operator/blob/ce4170b4a040fc03603f62d686a6e9ea0cadde34/pkg/operator/configobservation/configobservercontroller/observe_config_controller.go#L119)) provided and owned by 9 different teams can lead to bad configuration**. E.g. a config observer adds a flag `--foo` which was removed in the current upstream release, but lacking test coverage did not show this in CI.
    - **source invalid data from other sources with incomplete validation**: e.g. the network mask given through some other config.openshift.io/v1 CR defined by the user is invalid.
- **some ConfigMap or Secret can be wrong**: e.g. the audit policy is a YAML file inside a ConfigMap. If it is invalid, the kube-apiserver will refuse startup. ConfigMaps and Secrets are synced into the operand namespace from many different sources, and then copied as files onto the master node file system by the installer pod.
- **some ConfigMap or Secret marked as optional but is actually required**: the installer will continue rolling out the pod after skipping the optional files. In HA OpenShift, this might not show up because the operator create quickly another revision when the ConfigMap or Secret show up. In SNO this race might be fatal.

It is impossible to guarantee that none of these ever happens because the test permutations are just infeasible to cover completely in CI, especially as many of these are disruptive for cluster behaviour and hence very expensive to test.

### Goals

- recover from a badly formatted or invalid pod manifest
- recover from bad config observer output
- recover from bad revisioned ConfigMaps and Secrets.

### Non-Goals

- recover from invalid non-revisioned certificates
- recover from missing or invalid files on the masters given by static paths outside of the `/etc/kubernetes` directory (e.g. some certs or kubeconfig the operand is consuming)
- optimize downtime of the API beyond in fallback case to O(60s). This mechanism is for the disaster case and only survival counts. If downtime is `O(5min)` or even more, this is completely ok.

## Proposal

We propose to add a `<operand>-startup-monitor` static pod that watches the operand pods for readiness (see below for details) and that is created by installer as another manifest in `/etc/kubernetes` and provides by the static pod controller the same way the operand pod manifest is created today.

If the operand pod does not start up in N minutes, e.g. due to:

- invalid pod manifest
- free port wait loop timeout
- crash-looping
- not answering on the expected port (connection refused)
- healthy but never readyz

but not in case of

- etcd not healthy

the task for startup-monitor is to fall back:

1. when detecting problems with the new revision, the startup-monitor will copy the pod-manifest of the `/etc/kubernetes/static-pods/last-known-good` link (or the previous revision if the link does not exist, or don't do anything if there is no previous revision as in bootstrapping) into `/etc/kubernetes`.

It will add annotations to the old revision pod manifest:

- `startup-monitor.static-pods.openshift.io/fallback-for-revision: <revision>`
- `startup-monitor.static-pods.openshift.io/fallback-reason: MachineReadableReason`
- `startup-monitor.static-pods.openshift.io/fallback-message: "Human readable, possibly multiline message"`

These will be copied into the mirror pod by kubelet and the operator knows that this is due to a problem to start up the new revision.

2. if the operand becomes ready, the startup-monitor will link the revision to `/etc/kubernetes/static-pods/last-known-good`, and then remove its own pod-manifest from `/etc/kubernetes`, and hence commit suicide.

As long as the startup-monitor notices an operand pod-manifest of a different revision (by checking the operator manifest in `/etc/kubernetes` and the revision annotation), it will do nothing and just wait, continuing to watch the operand pod-manifest. This is important to avoid races on startup, and avoid problems on downgrade before this mechanism was introduced.

### Readiness

The startup-monitor watches the mirror pod for readiness, i.e. by

1. watch `/var/log/kube-apiserver/termination.log` for the first start-up attempt (beware of the race of startup-monitor startup and kube-apiserver startup). Set Reason=**NeverStartedUp** when this times out.
2. watch `/var/log/kube-apiserver/termination.log` for more than one start-up attempt. Set Reason=**CrashLooping** if more than one is found and the monitor times out.
3. check https://localhost:6443/healthz. Set Reason=**Unhealthy** if this is red and the monitor times out.
4. check https://localhost:6443/readyz. Set Reason=**NotReady** if this is red and the monitor times out. Reason=EtcdUnhealthy if the etcd post-start-hook never finished. In all case: message should contain the unfinished post-start-hooks.
4. get the mirror pod from the localhost kube-apiserver
5. check the revision annotation is the expected one. Set Reason=**NotReady** if this is red and the monitor times out.
6. checking status.ready. Set Reason=**NotReady** if this is red and the monitor times out.

### Retries

The readiness procedure above has the risk of considering external reasons as fatal reasons for kube-apiserver readiness. E.g. etcd unstable or kubelet not updating the mirror pod. Both would led us to believe the new revision is broken. This is different in HA OpenShift because there is no time limit exists for a new static pod to start up. It can keep crash-looping for hours and eventually start-up to finish the roll-out.

In order to mitigate this difference in error behaviour, a failing revision in SNO will be retried to roll-out after N minutes, N=O(10), a reasonable time for the cluster to recover from a previous attempt. In the time in-between, the fallback mechanism will move the cluster back to the old revision, until the retry is started.

While retries are attempted, the operator will report Degraded=true.

Retries are not attempted for the following reasons:
- EtcdHealthy
- CrashLooping
- NeverStartedUp

### Coordination of Installer and Startup-Monitor

Startup-monitor and installer to non-atomic work, and hence we need to coordinate work between them to protect the following case:
an installer is in progress and wants to install a new revision, the current revision is not ready and we are about to fall back to the previous version. The installer writes the new file and we immediately overwrite it.

Hence, we will write a lock file to /var/lock/kube-apiserver-installer.lock, and make the installer and the startup-monitor readiness procedure to wait for it.

### Pruner

The pruner today does not know about the `last-known-good` symlink to the last successful revision. It must be extended to ignore it and the linked revision.

### User Stories

1. As a cluster admin I **don't want to brick my cluster because of an invalid input** (e.g. some field in a `config.openshift.io/v1` CR).
2. As a cluster admin I want to **get notice that a revision was not rolled out successfully**, e.g. by seeing the operator being degraded.
3. As a cluster admin I want to **see the termination message of failed revision**, e.g. in a condition message.
4. As a cluster admin I want that **every good configuration is eventually rolled out** and not delayed much longer than a temporary failure condition (e.g. etcd down) persists.

### Risks and Mitigations

## Design Details
- Why is it safe to go back to last-known-good?

  Handwaving ahead: ee don't really know what happens when a new kube-apiserver is deployed, but fails to get ready. It might be that some object got written to etcd with the broken config. Mathematically, we can only be sure that this is harmless if last-known-good revision is one behind the last revision (think about how we allow that in etcd encryption that we need more than one revision to phase out a read key). When going back 2 or more revisions this invariant breaks. But it is the best we can do as the alternative is be unavailable.

  Note: in etcd encryption controllers we wait until deployments settle. We won't make progress when the operator is in Progression=True state.

  TODO: double check that ^ is true
### Open Questions [optional]

### Test Plan

The usual e2e tests will verify the happy case.

For the error case, we have to inject the different types of errors into an operand, similarly as it is done for the installer in https://github.com/openshift/cluster-kube-apiserver-operator/blob/022057ed14a0f2b1eb98f7b5ccb76d100de011d6/pkg/operator/starter.go#L363):

- [ ] test case to recover from a badly formatted or invalid pod manifest:

  The bad manifest is rolled out, the API is unavailable, but recovers after 5min max by switching back to the old, save revision with reason NeverStartedUp. The operator does not retry, but stays degraded.

- [ ] test case to recover from bad config observer output

  The bad manifest is rolled out. The API is unavailable because the new kube-apiserver process does not start, but crash loops. I.e. the reason is CrashLooping. The operator does not retry, but stays degraded.

- [ ] test case to recover from bad revisioned ConfigMaps and Secrets.

  a) The manifest is rolled out, but a file in a configmap or secret (e.g. audit policy cannot be loaded. The API is unavailable because the new kube-apiserver process does not start, but crash loops. I.e. the reason is CrashLooping. The operator does not retry, but stays degraded.

  b) TODO: think about a case where the ConfigMap/Secret exists, the apiserver starts up, does not get ready.

- [ ] test case to recover from bad etcd smoke test

  The new kube-apiserver starts up, but the etcd post-start hook never finishes. The reason is EtcdUnhealthy. The operator does not retry.

- [ ] test case to recover from non-etcd post-start hook never finishing

  The new kube-apiserver starts up, but some non-etcd post-start hook never finishes. The reason is NotReady The operator does retry.

### Graduation Criteria

This will graduate directly to GA as 4.9 is the target release we have to hit.

We might implement this enhancement partially in 4.9 and complete it in 4.10.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

On upgrade, if that even matters for SNO pre-GA, the `last-known-good` symlink won't exist on first rollout. We will fall back to the previous revision then.

On downgrade, the startup-monitor might still exist in `/etc/kubernetes`. We will make it wait until it sees an operand pod-manifest of the same revision before doing anything (see proposal).

### Version Skew Strategy

Not relevant.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

## Alternatives

- we have talked about starting the new apiserver in parallel and to have some proxy or iptables mechanism to switch over only when the new revision is ready. This was rejected as we don't have the memory resources on nodes to run two apiservers in parallel. Instead, it was decided that 60s downtime of the API for the happy-case (= configuration is not bad) is acceptable. Hence, we changed direction to only cover the disaster recovery case that is described in this enhancement.


