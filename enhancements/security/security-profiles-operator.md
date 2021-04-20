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
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - [selinux-operator proposal](https://github.com/openshift/enhancements/pull/327)
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Security Profiles Operator Integration in OpenShift

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Let's integrate the [Security Profiles Operator](https://github.com/kubernetes-sigs/security-profiles-operator)
as part of the OpenShift platform in order to allow folks to:

* Easily install security profiles to secure their workloads
  (both SELinux and Seccomp)

* Manage the lifecycle of the aforementioned profiles (make sure
  that they get attached to the appropriate container as upgrades
  happen)

* Enable users to automatically generate profiles for their workloads

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

### Goals

* Integrate the Security Profiles Operator into OpenShift by having it
  installed by default via the Cluster Version Operator (CVO).

* Integrate the SPO into OpenShift's SCCs.

* Enable application developers to ship their operators with
  relevant security profiles (OLM integration)

### Non-Goals

* Developing of profiles for all OpenShift components. While having the OpenShift
  components locked down is a long-term goal and SPO might come with several profiles
  for some selected workloads, the intent is not to ship a very wide library of profiles
  with the initial OpenShift inclusion

## Proposal

The Security Profiles Operator should be installed by default in OpenShift
via the CVO as any other core component. As it is, its main function is to
install security policies and ensure that they're attached to the appropriate
pods which require them.

The installation is managed by a DaemonSet workload (Security Profiles Operator Daemon
or SPOD) that listens for security profiles (either `SeccompProfile` or
`SelinuxProfile`) and ensures that these are installed on the nodes.

The SPOD instance is configurable through a CRD which allows us to set where
it's scheduled and the capabilities that are enabled/disabled. For OpenShift,
SELinux would be enabled by default and the DaemonSet would be scheduled
everywhere.

`SeccompProfiles` are handled by simply persisting a file on a directory that
the Kubelet has access to. A `SeccompProfile` looks as follows:

```yaml
---
apiVersion: security-profiles-operator.x-k8s.io/v1alpha1
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
apiVersion: security-profiles-operator.x-k8s.io/v1alpha1
kind: SelinuxProfile
metadata:
  name: errorlogger
  namespace: my-namespace
spec:
  policy: |
    (blockinherit container)
    (allow process var_log_t ( dir ( open read getattr lock search ioctl add_name remove_name write ))) 
    (allow process var_log_t ( file ( getattr read write append ioctl lock map open create  ))) 
    (allow process var_log_t ( sock_file ( getattr read write append open  )))
```

It is mostly a wrapper around [SELinux's CIL language](https://github.com/SELinuxProject/cil)
with the main block and naming handled by the operator. Naming of policies
as implemented by the daemon is as follows `<object name>_<object namespace>`. Thus,
we're able to distinguish between policies installed in different namespaces.

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

The `SeccompProfile` objects rely on the [OCI Seccomp BPF Hook](https://github.com/containers/oci-seccomp-bpf-hook)
to do the actual recording. So we'd need to ensure this hook is available in RHCOS
before enabling this feature.

There isn't currently support for `SelinuxProfile` recording, but
the work is scheduled to start in the 4.9 cycle, and the intent
is to leverage [Udica](https://github.com/containers/udica) to generate
the profile.

### SCC integration

Security Context Constraints allow a user to restrict the security settings
that a workload may have. Given that the profile settings are in the pod's
`securityContext` it is imperative that we appropriately integrate with SCCs.

Currently, the only SCC that allows users to set the Seccomp and SELinux
settings for a pod is the privileged one. It looks as follows:

```bash
Name:                                           privileged
Priority:                                       <none>
Access:                                         
  Users:                                        system:admin,system:serviceaccount:openshift-infra:build-controller
  Groups:                                       system:cluster-admins,system:nodes,system:masters
Settings:                                       
  Allow Privileged:                             true
  Allow Privilege Escalation:                   true
  Default Add Capabilities:                     <none>
  Required Drop Capabilities:                   <none>
  Allowed Capabilities:                         *
  Allowed Seccomp Profiles:                     *
  Allowed Volume Types:                         *
  Allowed Flexvolumes:                          <all>
  Allowed Unsafe Sysctls:                       *
  Forbidden Sysctls:                            <none>
  Allow Host Network:                           true
  Allow Host Ports:                             true
  Allow Host PID:                               true
  Allow Host IPC:                               true
  Read Only Root Filesystem:                    false
  Run As User Strategy: RunAsAny                
    UID:                                        <none>
    UID Range Min:                              <none>
    UID Range Max:                              <none>
  SELinux Context Strategy: RunAsAny            
    User:                                       <none>
    Role:                                       <none>
    Type:                                       <none>
    Level:                                      <none>
  FSGroup Strategy: RunAsAny                    
    Ranges:                                     <none>
  Supplemental Groups Strategy: RunAsAny        
    Ranges:                                     <none>
```

This basicallly allows for setting all security settings for a pod, which
might not be what a user intends to allow.

To enable integration with the SPO and promote better security in workloads,
we could add another SCC called `measured` that would look similar to this:

```bash
Name:                                           measured
Priority:                                       <none>
Access:                                         
  Users:                                        <none>
  Groups:                                       <none>
Settings:                                       
  Allow Privileged:                             false
  Allow Privilege Escalation:                   false
  Default Add Capabilities:                     <none>
  Required Drop Capabilities:                   <none>
  Allowed Capabilities:                         *
  Allowed Seccomp Profiles:                     *
  Allowed Volume Types:                         *
  Allowed Flexvolumes:                          <all>
  Allowed Unsafe Sysctls:                       *
  Forbidden Sysctls:                            <none>
  Allow Host Network:                           true
  Allow Host Ports:                             true
  Allow Host PID:                               true
  Allow Host IPC:                               true
  Read Only Root Filesystem:                    false
  Run As User Strategy: RunAsAny                
    UID:                                        <none>
    UID Range Min:                              <none>
    UID Range Max:                              <none>
  SELinux Context Strategy: RunAsAny  (1)          
    User:                                       <none>
    Role:                                       <none>
    Type:                                       <none>
    Level:                                      <none>
  FSGroup Strategy: RunAsAny                    
    Ranges:                                     <none>
  Supplemental Groups Strategy: RunAsAny        
    Ranges:                                     <none>

```

(1) See the Open Questions section

With this profile, the intent is to disallow privileged containers,
while enabling the usage of custom Seccomp and SELinux profiles.

There needs to be extra validation in place in such a way that
using the `measured` SCC will also result in workloads
only being able to use profiles that are installed in the same
namespace as the workload.

This will prevent workloads trying to:

* Wrongly use profiles that are assumed to be "pre-installed" in the system.

* Share a profile accross namespaces which might lead to errors.

* Maliciously gain privileges by using a profile installed from another
  namespace.

### OLM integration

As part of OpenShift promoting security as part of the platform, application
developers need to be able to install Security Profiles when installing operators.

For this, a `securityProfiles` section could be added to the CSV definition as part
of the `install.spec` section. Following a similar pattern as permissions, it could
look as follows:

```yaml
...
  install:
    spec:
      clusterPermissions:
        - rules:
          - apiGroups:
            - ""
            resources:
            - nodes
            verbs:
            - get
            - list
            - watch
          serviceAccountName: some-sa
      securityProfiles:
        - seccomp:
            spec:
              defaultAction: "SCMP_ACT_LOG"
          serviceAccountName: some-sa
        - selinux:
            spec:
              policy: |
                (blockinherit container)
                (allow process var_log_t ( dir ( open read getattr lock search ioctl add_name remove_name write ))) 
                (allow process var_log_t ( file ( getattr read write append ioctl lock map open create  ))) 
                (allow process var_log_t ( sock_file ( getattr read write append open  ))
          serviceAccountName: some-sa
      deployments:
      - name: some-deployment
        spec:
          replicas: 1
      ...

```

Where SELinux and Seccomp profile definitions are linked to a
Service Account, similar to other objects in the CSV.

### User Stories

The personas it aims to help are documented in [the project's documentation](
https://github.com/kubernetes-sigs/security-profiles-operator/blob/master/doc/personas.md).

The user stories that are being covered are [also appropriately documented](
https://github.com/kubernetes-sigs/security-profiles-operator/blob/master/doc/user-stories.md).

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

#### OCI Seccomp BPF Hook

This hook is currently not available in RHCOS. While it shouldn't be enabled
by default, it would be good to make it available as an `extension` via the
MachineConfigOperator. This way, when using OpenShift for development purposes,
it would be possible for users to create `ProfileRecordings` for their workloads.

We should also disable the `ProfileRecording` feature by default as it should only
be used in development environments and not in production.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Open Questions [optional]

 > 1. Should we use `RunAsAny` or something else for the `measured` SCC?

 Something such as `RunAllowed` would help make it more explicit
 that one can only run the security profiles that are allowed
 (installed) for that namespace.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

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
