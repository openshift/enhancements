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
api-approvers:
  - None
creation-date: 2021-01-18
last-updated: 2021-05-19
tracking-link:
  - https://issues.redhat.com/browse/CNF-5326
---

# Hide Container Mountpoints

## Summary

The current implementation of Kubelet and CRI-O both use the top-level
namespace for all container and Kubelet mountpoints. However, moving these
container-specific mountpoints into a private namespace reduced systemd
overhead with no difference in functionality.

## Motivation

systemd scans and re-scans mountpoints many times, adding a lot to the CPU
utilization of systemd and overall overhead of the host OS running OpenShift.
Changing systemd to reduce its scanning overhead is tracked in [BZ
1819868](https://bugzilla.redhat.com/show_bug.cgi?id=1819868), but we can work
around this exclusively within OpenShift. Using a separate mount namespace for
both CRI-O and Kubelet can completely segregate all container-specific mounts
away from any systemd or other host OS interaction whatsoever.

### User Stories

As an OpenShift system administrator, I want systemd to consume less resources
so that I can run more workloads on my system.

As an OpenShift system administrator, I want to disable the
mount-namespace-hiding feature so that I can fall back to the previous system
behavior.

As an OpenShift developer or support engineer, I want to inspect
kubernetes-specific mountpoints as part of debugging issues.

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

- Reduce systemd mountpoint scanning overhead

## Proposal

### Workflow Description

Generally speaking, the end-user experience should not be affected in any way
by this proposal, as there is no outward API changes. There is some
supportability difference, since anyone attempting to inspect the CRI-O or
Kubelet mountpoints externally would need to be aware that these are now
available in a different namespace than the default top-level systemd mount
namespace.

For Tech Preview, the feature must be enabled by adding a MachineConfig which
enables the new `kubens.service` systemd unit which drives the whole feature.
For GA, the feature would be enabled by default, and may be disabled by adding
a MachineConfig that disables the `kubens.service` systemd unit.

For any containers running in the system, there should be no observable
difference in the behavior of the system.

For any administrative shells or processes running outside of containers on the
host, the Kubernetes-specific mountpoints will no longer be visible by default.
Entering the new mount namespace via the `kubensenter` script will make these
mountpoints visible again.

### API Extensions

No API changes required.

The existing APIs available within MachineConfig objects can be used to
enable/disable the `kubens.service` which in turn enables/disables this
feature.

### Implementation Details/Notes/Constraints

We will create a separate mount namespace and cause both CRI-O and Kubelet
to launch within it to hide their many many mounts from systemd by:

- Selecting a well-known location to pin a Kubernetes-specific mount namespace:
  `/run/kubens/mnt`

- Adding a systemd service called `kubens.service` which spawns a separate
  namespace and pins it to this well-known location.
  - This will pin the namespace in `/run/kubens/mnt`
  - This will also create an environment file in `/run/kubens/env` which sets
    `$KUBENSMNT` if the service is running and the namespace is pinned.
  - A drop-in can be added to change the namespace location.
  - We don't want to create the namespace in `crio.service` or
    `kubelet.service`, since if either one restarts they would lose each
    other's namespaces.
  - Implemented in this way, disabling the `kubens.service` (and restarting
    both Kubelet and CRI-O) fully disables this proposed feature, falling back
    to the current not-hidden mode of operation.

- Adding a mechanism to CRI-O to enter a pre-existing mount namespace if set in
  `$KUBENSMNT`
  - If the path does not exist, or does not point to a mount namespace
    bindmount, CRI-O will run in its parent's mount namespace and log a warning
    that the requested namespace was not joined.

- Adding a mechanism to Kubelet to enter a pre-existing mount namespace if
  set in `$KUBENSMNT`
  - If the path does not exist, or does not point to a mount namespace
    bindmount, Kubelet will run in its parent's mount namespace and log a
    warning that the requested namespace was not joined.

- A convenience wrapper to enter this well-known mount namespace,
  `kubensenter` for other tools, administrative and support actions which
  need access to this namespace.
  - This will operate identically to `nsenter` except that it defaults to
    entering this well-known namespace location (if present), or running the
    command in the current mount namespace if the namespace is not pinned.

- An update to the node's MOTD that details how to deal with these namespace
  changes:
  - oc debug already starts inside the kubernetes mount namespace, and should
    give instructions on how to enter the default systemd mount namespace.
  - SSH login will start in the systemd mount namespace, and should give
    instructions on how to enter the kubernetes mount namespace.

- Both the new systemd service and convenience wrapper will be installed
  as part of Kubelet.

With this proposal in place, both Kubelet and CRI-O create their mounts in
the new shared (with each other) but private (from systemd) namespace, and
this feature can be easily enabled/disabled by enabling/disabling a single
systemd service.

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
  - This could mean that security scanning solutions that expect to see
    Kubernetes mounts will no longer see them.  They can be modified to join
    the new mount namespace.  We have verified that StackRox is not
    substantially affected by this change.

We will mitigate this by adding a helper application to easily enter the right
mount namespace, and adding an easy mechanism to disable this feature and
fallback to the original mode of operation.

### Drawbacks

If the namespace service restarts and then either CRI-O or Kubelet restarts,
there will be a mismatch between the mount namespaces and containers will start
to fail. Could be mitigated by changing the namespace service to NOT cleanup
its pinned namespace but instead idempotently re-use a previously-created
namespace.  However, given that the namespace service as implemented today is a
systemd `oneshot` with no actual process that needs keeping alive, the risk of
this terminating unexpectedly is very low.

Hiding the Kubernetes mounts from systemd may confuse administrators and
support personnel who are used to seeing them.

## Design Details

### Test Plan

- With the feature enabled:
  - Ensure that running 'mount' in default mount namespace does not show any of
    the Kubernetes-specific mountpoints.
  - Ensure that entering the mount namespace and running 'mount' shows all the
    Kubernetes-specific mountpoints.
  - All existing e2e tests at a similar rate.
- With the feature disabled:
  - Ensure that running 'mount' in default mount namespace shows all of the
    Kubernetes-specific mountpoints.
  - All existing e2e tests at a similar rate.

### Graduation Criteria

The main graduation consideration for this feature is when it is enabled by
default.

This feature is already in Dev Preview, with the current MachineConfig-based
proof-of-concept solution part of the telco-specific DU profile installed by
ZTP, and also available
[here](https://github.com/openshift-kni/cnf-features-deploy/tree/master/feature-configs/deploy/container-mount-namespace)

#### Dev Preview -> Tech Preview

- Reimplement according to this proposal, but the feature is disabled by
  default
- Add a CI lane that runs all current e2e tests with this feature enabled
- Update the ZTP DU profile to use the new mechanism instead of the current
  MachineConfig-based proof-of-concept
- User-facing documentation:
  - Feature overview (what it is and what happens when it's enabled)
  - How to enable the feature
  - How to inspect the container mounts when the feature is enabled
  - How to set up a system service to enter the mount namespace

#### Tech Preview -> GA

- Enable the feature by default
- Remove the CI lane that enables the feature, as it is enabled by default
- User-facing documentation changes:
  - Mention the feature is on by default
  - Change "how to enable" instructions to "how to disable"

#### Removing a deprecated feature

Not applicable.

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable. The mount namespace is fully-contained and isolated within each
node of a cluster. There is no impact of having the feature enabled on some
nodes and disabled on others.

### Operational Aspects of API Extensions

Not applicable; no API extensions; but there are operational impacts of this
change, detailed in the 'Risks and Mitigations' section above.

#### Failure Modes

- If either the Kubelet or CRI-O services end up in different namespaces from
  one another, containers started by CRI-O will not see mounts made by Kubelet,
  such as secrets or configmaps.

- If the namespace is not configured correctly to allow mounts from the OS to
  be shared into Kubelet or CRI-O, system mountpoints will not be visible to
  either Kubelet or CRI-O or runing containers.

#### Support Procedures

When this feature is enabled, a shell on a node will not have visibility of the
Kubernetes mountpoints.  The container started by `oc debug` will by default
start inside the kubernetes mount namespace.

- To start a shell or execute a command within the container mount namespace,
  execute the `kubensenter` script.

- To start a shell within the top-level systemd mount namespace, run `nsenter
  -t 1 -m`

- To disable this feature, inject a MachineConfig that disables the
  `kubens.service`:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-custom-disable-kubens-worker
spec:
  config:
    ignition:
      version: 2.2.0
    systemd:
      units:
      - enabled: false
        name: kubens.service
```

## Implementation History

This proposal differs from the original proof-of-concept by:
- Moving the responsibility of entering the namespace to the tools that
  run in it (CRI-O and Kubelet), instead of a fragile systemd
  ExecStart-patching drop-in
- Building in the simple off/on switch of enabling/disabling a single
  systemd service, instead of having it tied to a monolithic MachineConfig
  object.

### Initial proof-of-concept

Original work [here](https://github.com/lack/redhat-notes/tree/main/crio_unshare_mounts).

It has MC objects that create:
- The new `container-mount-namespace.service` service
- Override files for both `crio.service` and `kubelet.service` which add the
  appropriate systemd dependencies upon the `container-mount-namespace.service`
  and wrap ExecStart inside of `nsenter`
- A convenience utility called `/usr/local/bin/nsenterCmns` which can be used by
  administrators or other software on the host to enter the new namespace.

It also passed e2e tests at a fairly high rate on a 4.6.4 cluster.

### Dev Preview

This was then productized as a dev preview for the Telco RAN installations
[here](https://github.com/openshift-kni/cnf-features-deploy/tree/master/feature-configs/deploy/container-mount-namespace).
It uses the same MachineConfig-based drop-in mechanism as the original
proof-of-concept.

This is installed and enabled by the ZTP DU profile, and is used in production
on many Telco customers' systems, both for SingleNode OpenShift and standard
clusters, with no reported issues.

## Alternatives

- Enhance systemd to support unsharing namespaces at the slice level, then put
  `crio.service` and `kubelet.service` in the same slice

- We will also begin a KEP to support this same behavior in the stock upstream
  kubernetes.
