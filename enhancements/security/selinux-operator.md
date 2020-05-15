---
title: selinux-opertor
authors:
  - "@jaormx"
reviewers:
  - "@cgwalters"
  - "@mrogers950"
  - "@ashcrow"
  - "@jhrozek"
approvers:
  - "@ashcrow"
creation-date: 2020-05-15
last-updated: 2020-05-15
status: provisional
---

# Cluster-wide SELinux module management

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]


## Summary

This enhancement describes a new security component for OpenShift. The intent
is to provide both infra developers and customers with a way for them to
install SELinux modules for their workloads in an easy and cluster-aware
fashion. The proposed selinux-operator provides a way to install modules in a
kubernetes-namespaced manner, and so it would be possible to also enforce RBAC
on them (define who can install modules and what pods could take them
into use).

## Motivation

Currently, workloads that need to interact with the underlying nodes in any way
need to resort to using privileged containers. Using such a flag, while
allowed, effectively turns off SELinux labeling support by assigning the
container(s) the `spc_t` type domain, which acts similarly to an unconfined
domain; thus giving the container too much privilege. This is usually
unnecessary and could be avoided by having a custom SELinux policy for that
container that would allow it to just mount the files it needs to mount and
thus limit the access to the host.

Given that installing custom policies is non-trivial, most developers just opt
to using privileged containers and accept the risk. As of today (May 15th,
2020) a stock installation of OpenShift ships with around 150 privileged
containers.

As a workaround, some projects such as Kubevirt have opted to have a privileged
container that installs such a custom policy. However, instead of asking people
to do this, it would be good to support a mechanism to help folks install
SELinux modules. Better yet if we could provide such a feature in a
cluster-aware fashion.

### Goals

Provide a way for deployers to install, remove and audit SELinux policies for
containers in the cluster.

### Non-Goals

Provide a way to automate writing SELinux policies. While this is important too
and would be useful, this is out of scope for this particular enhancement. Such
an initiative is being handled by the [Udica](https://github.com/containers/udica)
project and a Proof-Of-Concept to integrate a more automated workflow into
OpenShift is being handled separately in
[selinux-policy-helper-operator](https://github.com/JAORMX/selinux-policy-helper-operator)

This also doesn't address how these SELinux policies are distributed with
operators. This should be handled instead in an OLM proposal.

## Proposal

In a nutshell, the idea is to have a dedicated operator that handles the
installation and removal of SELinux policies.

The implementation is currently
[in the following repository](https://github.com/JAORMX/selinux-operator),
however, if accepted, it shall be moved to the OpenShift organization in
GitHub.

The proposal is the following:

* The operator will expose a CRD that represents SELinux policies. This Custom
  Resource shall be called `SelinuxPolicy`. The specification of this CRD is
  defined in a subsequent section.

* If the `SelinuxPolicy` is created and set to be applied. It'll spawn a
  `DaemonSet` in the background that installs this policy in each node in the
  cluster.

* Once the `SelinuxPolicy` is no longer needed (e.g. if a workload is being
  removed), the operator will uninstall the policy from the cluster. This is
  currently done with finalizers and a pre-stop hook.

* `SelinuxPolicy` objects are namespaced objects, so only users with access to
  a certain project can view and edit them. On the other hand, only `Pods` in
  the same namespace as the `SelinuxPolicy` objects shall be able to use them.
  This is enforced with an Admission Webhook. This prevents random workloads
  from taking privileges that were not intended for them since they can't use
  policies that are not enabled in their namesapces.


### User Stories


#### Module installation

As a system administrator I want to install a custom SELinux policy so I can
run my workload without it using privileged containers.

#### Module removal

As a system administrator I want to remove the custom SELinux policy once I
no longer run the workload in the cluster.

#### Auditing

As a system administrator I want to know what custom policies are installed all
over the cluster.

#### Security (RBAC)

As a system administrator, I want to enforce that only certain users can create
policies in certain namespaces.

This way, only trusted users would be able to install policies in the system.

#### Security (namespace separation)

As a system administrator, I want to make sure that workloads can only use
approved policies.

This way, an attacker can't take into use a policy from a namespace that's not
accessible.

### API Specification

This operator would expose a CRD that would look as follows:

```
apiVersion: selinux.openshift.io/v1alpha1
kind: SelinuxPolicy
metadata:
  name: virt-launcher
  namespace: my-namespace
spec:
  apply: true
  policy: |
    (blockinherit container)
    (typeattributeset sandbox_net_domain (process))
    (allow process kernel_t (system (module_request)))
    (allow process mtrr_device_t (file (write)))
    (allow process self (tun_socket (relabelfrom)))
    (allow process self (tun_socket (relabelto)))
    (allow process self (tun_socket (attach_queue)))
    (allow process sysfs_t (file (write)))
    (allow process tmp_t (dir (write add_name open getattr setattr read link search remove_name reparent lock ioctl)))
    (allow process tmp_t (file (setattr open read write create getattr append ioctl lock)))
    (allow process container_share_t (dir (write add_name)))
    (allow process container_share_t (file (create setattr write )))
    (allow process container_var_run_t (dir (write add_name open getattr setattr read link search remove_name reparent lock ioctl)))
    (allow process container_var_run_t (file (setattr open read write create getattr append ioctl lock)))
status:
  usage: virt-launcher_my-namespace.process
  state: INSTALLED
```

Where:

* `spec.apply`: Is a flag that allows us to tell the operator if the policy is to be
  applied (installed) or not. Having this flag allows policy writers and system
  administrators to verify the policy before taking it into use in the system.
  Automated tools such as the aforementioned `selinux-policy-helper-operator`
  would generate policies with this flag set to `false`.
* `spec.policy`: Contains the actual lines of policy in
  [CIL language](https://github.com/SELinuxProject/cil/wiki). The decision for
  this language is because `Udica` already writes policies in such a language,
  and it's an easier to read format than the typical SELinux type enforcement
  (te) language.
* `status.usage`: This string indicates how the user is supposed to use this
  policy in their `Pod` templates. The format is as follows:
  `<policy name>_<policy namespace>.process>`
* `status.state`: This represents the current status of the policy in the
  cluster. The accepted states are: `PENDING`, `IN-PROGRESS`, `INSTALLED`,
  `ERROR`. A typical reason why a policy would end up in an ERROR state would
  be that it has errors in the syntax.


### Implementation Details/Notes/Constraints

* Installing policies may take some seconds for each of the clusters, so pods
  that initially want to take them into use might fail if they get scheduled
  before the policy is installed. This needs to be figured out and possibly
  addressed (if the container runtime engine doesn't retry). We might also need
  a way to ensure that the pod is only run when the policy is installed.

* Currently, the implementation runs a `DaemonSet` that installs the policy in
  all the nodes in the cluster. We might want to limit this by exposing a
  `nodeSelector` to the CRD.

* Given that this uses a custom operator and a custom resource, integration
  with `Pods` and standard kubernetes resources doesn't necessarily use the
  same names (e.g. why the `usage` section was needed in the first place). In
  the future, we could consider adding a section to the `seLinuxOptions`
  section for `Pods` that includes the name of the resource. This would be a
  usability enhancement.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

* Basic functionality
  1. Install selinux-operator.
  2. Ensure operator roll-out, check for running `DaemonSet` pods.
  3. Install a `SelinuxPolicy` object in an arbitrary namespace
  4. Ensure a pod in that arbitrary namespace can take that policy into use.
* Cluster upgrade testing


### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

### Upgrade / Downgrade Strategy

We are working closely with the SELinux maintainers, any changes in the CIL
implementation or the way modules in general are installed should be handled by
operator upgrades with minimal user intervention.

### Version Skew Strategy

The operator is intended to be the sole controller of its operand resources
(configmaps, daemonSets), so there should not be version skew issues.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in
`Implementation History`.

## Alternatives

### Docs

Officially document that folks should install their own policies via a
privileged container, as KubeVirt does.

### MachineConfig

One option is to deliver such policies through `Machineconfig` objects. This
would require shipping a MachineConfig object that would embed the policy in an
ignition config, and a systemd unit that would install the policy.

While this option would be tempting due to the fact that it requires no extra
components for OpenShift, there are several reasons why I opted not to take
this route. 

One of them is that SELinux policies are not just files that we persist on the
host and they get used (as would be the case with Seccomp), there needs to be
further automation and tracking of these: One has to trigger an installation
command, and if you want to remove it, gotta trigger an uninstall command too.
So in order for this to work, we not only need to provide the file, but we also
need a systemd unit to install it.

On the other hand, this proposal is beyond just installing the policies in a
node. The idea is to namespace them and is able to enforce RBAC on them, so we
want to be careful who can create and use these. Some policies might be fairly
harmless while others might give the containers a lot of privilege, so it
would be ideal for admins to be able to be able to enforce who is able to use
policies and who can't. Installing them via MachineConfigs would just make the
policies usable throughout the whole cluster.

There is also the factor that creating a MachineConfig triggers a node restart
which would not be ideal if you want to take a policy into use immediately.

Let's look at the following example: We want to install an operator from
OperatorHub. This operator has been hardened and it comes with a read-to-use
SELinux policy for its operand. We just want to install it and use it
immediately. If it would use MachineConfig to distribute the policy it would
trigger a restart, and if you at some point decide to uninstall the operator
(because you have a different solution somewhere) this makes it really hard to
track what policies do you have in your system. So you really want a way to be
able to track these and remove them when they're no longer needed.

Writing policies without the ready-to-use blocks of policy that the operator
provides (which come from the **Udica** project) would also mean that the
overall policies that have to be provided either have to be a lot longer than
the ones you write with the operator, or it means that the **Udica** package
needs to be included in RHCOS (so we would need yet anther package in the OS).

Finally, UX. SELinux is normally seen as a very complicated technology. This
aims to simplify things by exposing it as an object in a readable format, as
opposed to having to decode a url-encoded string from a MachineConfig. There is
also a helper operator that automatically generates policies if you request
so https://github.com/JAORMX/selinux-policy-helper-operator.

## References

* [Current repository](https://github.com/JAORMX/selinux-operator)

* [DevConf talk](https://youtu.be/iMO6rwA-i_s)
