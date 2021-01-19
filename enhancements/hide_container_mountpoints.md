---
title: hide-container-mountpoints
authors:
  - "@lack"
reviewers:
  - "@haircommander"
  - "@mrunalp"
  - "@umohnani8"
approvers:
  - "@haircommander"
  - "@mrunalp"
  - "@umohnani8"
creation-date: 2021-01-18
last-updated: 2021-03-05
status: implementable
---

# Hide Container Mountponts

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The current implementation of Kubelet and CRI-O both use the top-level
namespace for all container and Kubelet mountpoints. However, moving these
container-specific mountpoints into a private namespace reduced systemd
overhead with no difference in functionality.

## Motivation

Systemd scans and re-scans mountpoints many times, adding a lot to the CPU
utilization of systemd and overall overhead of the host OS running OpenShift.
Changing systemd to reduce its scanning overhead is tracked in [BZ
1819868](https://bugzilla.redhat.com/show_bug.cgi?id=1819868), but we can work
around this exclusively within OpenShift. Using a separate mount namespace for
both CRI-O and Kubelet can completely segregate all container-specific mounts
away from any systemd or other host OS interaction whatsoever.

### Goals

- Mounts originating in CRI-O, OCI hooks, Kubelet, and container volumeMounts
  with `mountPropagation: Bidirectional` are no longer visible to systemd or
  the host OS
- Mounts originating in the host OS are still visible to CRI-O, OCI hooks,
  Kubelet, and container volumeMounts with `MountPropagation: HostToContainer`
  (or Bidirectional)
- Mounts originating in CRI-O, OCI hooks, Kubelet, and container volumeMounts
  with `mountPropagation: Bidirectional` are still visible to each other and
  container volumeMounts with `MountPropagation: HostToContainer`
- Restarting either `crio.service` or `kubelet.service` does not result in the
  mount visibility getting out-of-sync

### Non-Goals

- Fix systemd mountpoint scanning

## Proposal

We can create a separate mount namespace and cause both CRI-O and Kubelet to
launch within it to hide their many many mounts from systemd by creating:

- A service called `container-mount-namespace.service` which spawns a separate
  namespace and pins it to a well-known location.  We don't want to create the
  namespace in `crio.service` or `kubelet.service`, since if either one restarts they
  would lose each other's namespaces.

- An override file for each of `crio.service` and `kubelet.service` which wrap the
  original command under `nsenter` so they both use the mount namespace created
  by `container-mount-namespace.service`

With these in place, both Kubelet and CRI-O create their mounts in the new
shared (with each other) but private (from systemd) namespace.

This should be implemented in such a way that an administrator can disable it via
MCO, in case the original behavior is still desired for some use cases.

### User Stories

The end-user experience should not be affected in any way by this proposal, as
there is no outward API changes. There is some supportability change though,
since anyone attempting to inspect the CRI-O or Kubelet mountpoints externally
would need to be aware that these are now available in a different namespace
than the default top-level systemd mount namespace.

### Implementation Details/Notes/Constraints

A working proof-of-concept implementation based on
[MCO](https://github.com/openshift/enhancements/pull/github.com/openshift/machine-config-operator)
configuration is available
[here](https://github.com/lack/redhat-notes/tree/main/crio_unshare_mounts)

Here is the example `container-mount-namespace.service`:

    [Unit]
    Description=Manages a mount namespace that both kubelet and crio can use to share their container-specific mounts
    
    [Service]
    Type=oneshot
    RemainAfterExit=yes
    RuntimeDirectory=container-mount-namespace
    Environment=RUNTIME_DIRECTORY=%t/container-mount-namespace
    Environment=BIND_POINT=%t/container-mount-namespace/mnt
    ExecStartPre=bash -c "findmnt ${RUNTIME_DIRECTORY} || mount --make-unbindable --bind ${RUNTIME_DIRECTORY} ${RUNTIME_DIRECTORY}"
    ExecStartPre=touch ${BIND_POINT}
    ExecStart=unshare --mount=${BIND_POINT} --propagation slave
    ExecStop=umount -R ${RUNTIME_DIRECTORY}

This needs to be managed separately from either CRI-O or Kubelet to avoid the
namespaces getting out of sync if either of those services restart. This
example no longer uses the systemd `PrivateMounts=on` facility as a previous
proof-of-concept did, but uses `unshare` to create the namespace with slave
propagation and additionally pins the mount namespace to a filesystem location
(`/run/container-mount-namespace/mnt` in the example).

Both CRI-O and Kubelet can then find the associated namespace via
`nsenter` as follows:

    nsenter --mount=/run/container-mount-namespace/mnt $ORIGINAL_EXECSTART

This will also necessitate adding `Requires=container-mount-namespace.service`
and `After=container-mount-namespace.service` to both `crio.service` and
`kubelet.service` as well to ensure the namespace pin is available when they
start.

The proof-of-concept does this by injecting this override file for both
`crio.service` and `kubelet.service`, but a final implementation could do this by
editing the service files directly as well.

### Risks and Mitigations

The current OpenShift and Kubernetes implementations guarantee 3 things about
mountpoint visibility:
1. Mounts originating in the host OS are visible to CRI-O, OCI hooks, Kubelet,
   and container volumeMounts with `MountPropagation: HostToContainer` (or
   Bidirectional)
2. Mounts originating in CRI-O, OCI hooks, Kubelet, and container volumeMounts
   with `mountPropagation: Bidirectional` are visible to each other and
   container volumeMounts with `MountPropagation: HostToContainer`
3. Mounts originating in CRI-O, OCI hooks, Kubelet, and container volumeMounts
   with `mountPropagation: Bidirectional` are visible to the host OS

The first 2 guarantees are not changed by this proposal:
1. The new mount namespace uses 'slave' propagation, so any mounts originating
   in the host OS top-level mount namespace are still propagated down into the
   new 2nd-level namespace where they are visible to CRIO-O, OCI hooks,
   Kubelet, and container volumeMounts with `MountPropagation: HostToContainer`
   (or Bidirectional), just as before.
2. CRI-O, OCI hooks, Kubelet, and any containers created by CRI-O are all
   within the same 2nd-level namespace, so any mountpoints created by any of
   these entities are visible to all others within that same mount namespace.
   Additionally, any 3rd-level namespaces created below this point will have
   the same relationship with the 2nd-level namespace that they previously had
   with the higher-level namespace.

The 3rd guarantee is explicitly removed by this proposal.

This means that:
- Administrators who have connected to the host OS and want to inspect the
  mountpoints originating from CRI-O, OCI hooks, Kubelet, or containers will
  not be able to see them unless they explicitly enter the 2nd-level namespace.
- Any external or 3rd-party tools which run in the host mount namespace but
  expect to see mountpoints created by CRI-O, OCI hooks, Kubelet, or containers
  would need to be changed to enter the specific container namespace in order
  to see them.

We can mitigate this by adding a helper application to easily enter the right
mount namespace.  The proof-of-concept work adds a helper to do just this.

## Design Details

### Open Questions

Is the breaking of this "host OS sees CRI-O, Kubelet, and container
mountpoints" guarantee a deal-breaker?

The implementation could take a few different forms. The proof-of-concept work
that preceded this work was a set of
[MCO](https://github.com/openshift/enhancements/pull/github.com/openshift/machine-config-operator)
changes. This could be carried forward into the new implementation, or we could
approach it another way:
 > 1. Should these services and overrides be installed by MCO?
 > 2. If so, should they be part of the main `00-worker` profile, or in a
 >    separate `02-worker-container-mount-namespace` object?

For testing, the fact that there is a Kubernetes test that explicitly tests
that the container mountpoints are available to the parent operating system
implies that this may have been desirable to someone at some level at one time
in the past.
 > 1. What is the reason for the Kubernetes test? Is it okay to just skip or
 >    disable the test in OpenShift?
 > 2. Are there any external utilities or 3rd-party tools that assume they can
 >    have access to the CRI-O or Kubelet mountpoints in the top-level mount
 >    namespace?

### Test Plan

- Modify the existing Kubernetes e2e test that checks that all
  mountpoints are in the parent mount namespace to check:
  - All CRI-O and Kubelet mountpoints are visible only in the child
    namespace and not the parent namespace
  - Pod mountpoints in the 'master' 'slave' and 'private' categories still
    propagate to eachother and the child namespace as expected
  - Proof-of-concept e2e test changes
    [here](https://github.com/openshift/kubernetes/compare/master...lack:hide_container_mountpoint)
- Pass all other e2e tests at a similar rate.

## Implementation History

Initial proof-of-concept example is
[here](https://github.com/lack/redhat-notes/tree/main/crio_unshare_mounts).
It has MC objects that create:
- The new `container-mount-namespace.service` service
- Override files for both `crio.service` and `kubelet.service` which add the
  appropriate systemd dependencies upon the `container-mount-namespace.service`
  and wrap ExecStart inside of `nsenter`
- A convenience utility called `/usr/local/bin/nsenterCmns` which can be used by
  administrators or other software on the host to enter the new namespace.

It also passed e2e tests at a fairly high rate on a 4.6.4 cluster:

Parallel (Full output [here](https://raw.githubusercontent.com/lack/redhat-notes/main/crio_unshare_mounts/test_results/parallel.out))

    Flaky tests:
    
    [k8s.io] [sig-node] Events should be sent by kubelets and the scheduler about pods scheduling and running  [Conformance] [Suite:openshift/conformance/parallel/minimal] [Suite:k8s]
    [sig-network] Conntrack should be able to preserve UDP traffic when server pod cycles for a ClusterIP service [Suite:openshift/conformance/parallel] [Suite:k8s]
    
    Failing tests:
    
    [k8s.io] [sig-node] Mount propagation should propagate mounts to the host [Suite:openshift/conformance/parallel] [Suite:k8s]
    [sig-arch] Managed cluster should ensure control plane pods do not run in best-effort QoS [Suite:openshift/conformance/parallel]
    [sig-network] Networking should provide Internet connection for containers [Feature:Networking-IPv4] [Skipped:azure] [Suite:openshift/conformance/parallel] [Suite:k8s]
    
    error: 5 fail, 951 pass, 1457 skip (20m27s)

Serial (Full output [here](https://raw.githubusercontent.com/lack/redhat-notes/main/crio_unshare_mounts/test_results/serial.out))

    Failing tests:
    
    [sig-auth][Feature:OpenShiftAuthorization][Serial] authorization  TestAuthorizationResourceAccessReview should succeed [Suite:openshift/conformance/serial]
    [sig-cluster-lifecycle][Feature:Machines][Serial] Managed cluster should grow and decrease when scaling different machineSets simultaneously [Suite:openshift/conformance/serial]
    
    error: 2 fail, 58 pass, 227 skip (53m14s)

The only significant risk here is that of the `Mount propagation should
propagate mounts to the host` which is explictly and intentionally broken by
this proposal.  See /Risks and Mitigations/ above for an in-depth description
of the changes.

## Drawbacks

- Requires re-wrapping CRI-O and Kubelet services in bash and `nsenter`, and
  this is a little fragile. Could be mitigated by altering CRI-O and Kubelet to
  take the mount namespace PID and/or file on the command line. On the other
  hand, wrapping via `nsenter` could be reused upstream for all compatible
  container runtimes.
- If the namespace service restarts and then either CRI-O or Kubelet restarts,
  there will be a mismatch between the mount namespaces and containers will
  start to fail. Could be mitigated by changing the namespace service to NOT
  cleanup its pinned namespace but instead idempotently re-use a
  previously-created namespace.  However, given that the namespace service as
  implemented today is a systemd `oneshot` with no actual process that needs
  keeping alive, the risk of this terminating unexpectedly is very low.

## Alternatives

- Enhance systemd to support unsharing namespaces at the slice level, then put
  `crio.service` and `kubelet.service` in the same slice
- Alter both CRI-O and Kubelet executables to take a mount namespace via
  command line instead of requiring a re-wrap with `nsenter`
- Do this work upstream in Kubernetes as opposed to OpenShift

