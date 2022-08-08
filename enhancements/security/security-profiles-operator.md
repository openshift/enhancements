---
title: security-profiles-operator
authors:
  - "@jaormx"
  - "@jhrozek"
  - "@saschagrunert"
reviewers:
  - "@ashcrow"
  - "@mrunalp"
  - "@sttts"
  - "@mrunalp"
  - "@shawn-hurley"
  - "@dmesser"
approvers:
  - TBD
creation-date: 2012-04-20
last-updated: 2022-08-08
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - [Make Security Profiles Operator a part of OpenShift](https://issues.redhat.com/browse/CMP-1091)
status: implemented
replaces:
  - [selinux-operator proposal](https://github.com/openshift/enhancements/pull/327)
---

# Security Profiles Operator Integration in OpenShift

## Summary

Let's integrate the [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator)
as an optional operator of the OpenShift platform in order to allow folks to:

* Easily install security profiles to secure their workloads
  (both SELinux and Seccomp)

* Manage the lifecycle of the aforementioned profiles (make sure
  that they get attached to the appropriate container as upgrades
  happen)

* Enable users to automatically generate profiles for their workloads

At some later point:

* Provide predefined profiles for existing default workloads in OpenShift


## Motivation

OpenShift already has several security controls enabled by default. Amongst them
are the following:

* SELinux to restrict what resources can containers interact with

* Seccomp to restrict the system calls that containers are allowed to do

* Security Context Constraints to control what security knobs and handles are
  service accounts able to tweak in their workloads.

The default SELinux and Seccomp profiles work great for restricted workloads.
However, once the workload needs a little more permissions, setting these
security settings becomes non-trivial. In face of this, developers often
respond by turning off these controls and running their workloads as privileged
containers.

The [Security Profiles Operator (SPO)](https://github.com/kubernetes-sigs/security-profiles-operator)
is an upstream project aiming to ease installation and development of security
profiles in Kubernetes. By using these profiles, developers would have a more
granular way of setting permissions for their workloads, hopefully eliminating
the need to run privileged containers

### User Stories

 * As an application SRE, I would like to deploy a Seccomp profile
   and an SELinux profile to secure my application.

 * As an application SRE, I need to be able to see the state of
   the installed profile(s) and verify that it has indeed been installed on
   the system.

 * As an application SRE, I need to easily be able to link a security
   profile to a pod or set of pods.

 * As an application developer, I would like to be able to automatically
   generate initial security profiles that are specific to the application.

Note that we also maintain a more complete list of [user stories upstream](https://github.com/kubernetes-sigs/security-profiles-operator/blob/main/doc/user-stories.md)

### Goals

* Enable application developers to ship their applications with
  relevant security profiles

* Enable application developers to easily record the security profiles
  for their application.

### Non-Goals

* Integrate the Security Profiles Operator into OpenShift by having it
  installed by default via the Cluster Version Operator (CVO). While this is
  a long-term goal, it is not what we're trying to do in the first iteration.

* Developing of profiles for all OpenShift components. While having the OpenShift
  components locked down is a long-term goal and SPO might come with several profiles
  for some selected workloads, the intent is not to ship a very wide library of profiles
  with the initial OpenShift inclusion

## Proposal

The Security Profiles Operator should be available for installation as an
add-on. Its main function is to install security policies and ensure that
they're attached to the appropriate workloads which require them. Additionally,
the Security Profiles Operator enables recording of SELinux and seccomp
profiles, which can be used e.g. in CI runs.

The installation is managed by a DaemonSet workload (Security Profiles Operator Daemon
or SPOD) that listens for security profiles (either `SeccompProfile` or
`SelinuxProfile`) and ensures that these are installed on the nodes.

The SPOD instance is configurable through a CRD which allows us to set where
it's scheduled and the capabilities that are enabled/disabled. For OpenShift,
SELinux would be enabled by default and the DaemonSet would be scheduled
everywhere.

### Workflow Description

Please see the upstream [documentation](https://github.com/kubernetes-sigs/security-profiles-operator/blob/main/installation-usage.md)

### API Extensions

`SeccompProfiles` are handled by simply persisting a file on a directory that
the Kubelet has access to. A `SeccompProfile` looks as follows:

```yaml
---
apiVersion: security-profiles-operator.x-k8s.io/v1beta1
kind: SeccompProfile
metadata:
  name: profile-complain-block-high-risk
  annotations:
    description: "Enables complain mode whilst blocking high-risk syscalls. Some essential syscalls are allowed to decrease log noise."
spec:
  defaultAction: SCMP_ACT_LOG
  architectures:
  - SCMP_ARCH_X86_64
  syscalls:
  - action: SCMP_ACT_ALLOW
    names:
    - exit
    - exit_group
    - futex
    - nanosleep

  - action: SCMP_ACT_ERRNO
    names:
    - acct
    - add_key
    - bpf
    - clock_adjtime
    - clock_settime
    - create_module
    - delete_module
    - finit_module
    - get_kernel_syms
    - get_mempolicy
    - init_module
    - ioperm
    - iopl
    - kcmp
    - kexec_file_load
    - kexec_load
    - keyctl
    - lookup_dcookie
    - mbind
    - mount
    - move_pages
    - name_to_handle_at
    - nfsservctl
    - open_by_handle_at
    - perf_event_open
    - personality
    - pivot_root
    - process_vm_readv
    - process_vm_writev
    - ptrace
    - query_module
    - quotactl
    - reboot
    - request_key
    - set_mempolicy
    - setns
    - settimeofday
    - stime
    - swapoff
    - swapon
    - _sysctl
    - sysfs
    - umount2
    - umount
    - unshare
    - uselib
    - userfaultfd
    - ustat
    - vm86old
    - vm86
```

As seen, the object is simply a wrapper around the JSON formatting, which
allows us to do extra validations on the objects, thus ensuring easily that
it's appropriately formatted.

`SelinuxProfiles` require extra work, as to install a SELinux policy
one has to parse the policy, compile it to the main system's policy
and then install it in the system (which keeps it in memory). To do this
an extra component called [selinuxd](https://github.com/JAORMX/selinuxd) has
been developed. The intent is that selinuxd allows one to simply drop a file
on a pre-determined directory to install a SELinux policy, and that policy will
get picked up and installed by the daemon. Thus, the SPOD instance simply
places the policy into a directory, the selinuxd instance picks it up
and handles the installation and tracking of policies.

A `SelinuxProfile` object looks as follows:

```yaml
---
apiVersion: security-profiles-operator.x-k8s.io/v1alpha2
kind: SelinuxProfile
metadata:
  name: errorlogger
spec:
  inherit:
    - name: container
  allow:
    var_log_t:
      dir:
        - open
        - read
        - getattr
        - lock
        - search
        - ioctl
        - add_name
        - remove_name
        - write
      file:
        - getattr
        - read
        - write
        - append
        - ioctl
        - lock
        - map
        - open
        - create
      sock_file:
        - getattr
        - read
        - write
        - append
        - open
```

It is mostly a more user-friendly wrapper around [SELinux's CIL
language](https://github.com/SELinuxProject/cil) with the conversion
to CIL, which is used underneath, handled by the operator. Naming of
policies as implemented by the daemon is as follows `<object name>_<object
namespace>`. Thus, we're able to distinguish between policies installed
in different namespaces.

To ease usability and allow for developers to easily bind profiles to workloads,
an object called `ProfileBinding` was created. The intent is for the binding to
declare what profile goes to a ceratain image. The binding is then enforced by a
webhook which mutates the pod definition to add the appropriate profile
to its security context.

The `ProfileBinding` object looks as follows:

```yaml
---
apiVersion: security-profiles-operator.x-k8s.io/v1alpha1
kind: ProfileBinding
metadata:
  name: profile-binding
spec:
  profileRef:
    kind: SeccompProfile
    name: profile-allow-unsafe
  image: nginx:1.19.1
```

This would help manage security profiles and match them to workloads
accross the application's lifecycle. Where one could upgrade their
workload, and create a binding that matches the new application's image
tag.

Finally, to aide developers in writing profiles, there is a `ProfileRecording`
object. The object would look as follows:

```yaml
---
apiVersion: security-profiles-operator.x-k8s.io/v1alpha1
kind: ProfileRecording
metadata:
  # The name of the Recording is the same as the resulting `SeccompProfile` CRD
  # after reconciliation.
  name: test-recording
spec:
  kind: SeccompProfile
  podSelector:
    matchLabels:
      app: alpine
```

The operator automates the recording of profiles, and, when the
workload is done, it's able to output a ready-to-use profile
with the privileges that the application needs.

The `ProfileRecording` objects rely tailing the `audit.log` for both
recording of SELinux policies and Seccomp policies. Additionaly, when recording
is to be used, a recording webhook must be enabled that injects a special
Seccomp or Selinux profile which actually enables the recording itself.

### Implementation Details/Notes/Constraints

#### selinuxd

Selinuxd is currently deployed as a container alongside the SPOD instance.
In order to ensure that SELinux policies are installed early on when a
node boots, it would be ideal to have selinuxd run directly on the node
itself (either as a systemd unit or as a static container) as opposed to
it running on a container managed by the Kubelet.

On the other hand, the repository namespace selinuxd is hosted is currently
a personal one. It would also be ideal to move that somewhere more appropriate.
(The coreos namespace maybe?)

### Risks and Mitigations

Recording of profiles and binding profiles to images both require mutating
webhooks. Having a webhook always brings a degree of risk depending on the failure
policies and what namespaces do the webhooks listen to. We need to balance
usability (upstream enables webhooks always, on all namespaces) vs security or
stability (probably by not enabling the webhooks at all).

The `selinuxd` deamon is performance heavy.

### Drawbacks

See Risks and Mitigations, as long as the operator is optional, I can't
thing of anything else to add here.

## Design Details

### Open Questions

### Test Plan

There is already a nice e2e test suite upstream, moreover RH QE have been
involved for some time and have developed a test plan as well.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to this should be
  identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Currently, people need to use the Machine Config Operator to install such profiles.
In order to do this, people need administrative access (or at least access to
MachineConfig object creation) in order to simply install a policy. For
SELinux policies, one has to not only drop a file on the node, but also drop a systemd
unit that would handle the installation. Finally, the installation of policies would
always trigger a restart of the cluster, which makes it very difficult and tedious
to use when simply trying to develop profiles.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
