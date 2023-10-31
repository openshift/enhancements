---
title: IPTables Deprecation Alerter
authors:
  - "@danwinship"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2023-10-20
last-updated: 2023-10-20
tracking-link: 
  - https://issues.redhat.com/browse/SDN-4114
---

# IPTables Deprecation Alerter

## Summary

IPTables is deprecated in RHEL 9 and will no longer be available in
RHEL 10. We need to warn customers who are using iptables in their
pods that they will need to migrate to newer APIs by the time OCP
moves to RHEL 10 in a few years.

## Motivation

### User Stories

As an OpenShift user, I don't want to upgrade to OCP 4.[REDACTED] and
discover that my pods don't work any more.

### Goals

- Customers using pods that use iptables rules are warned about the
  fact that iptables is being deprecated and that those pods will
  (eventually) need to be updated.

- Customers using third-party software that uses iptables in their OCP
  cluster are warned about the fact that this software is using
  iptables, so they can talk to their third-party vendor to make sure
  they have a plan to address this.

- We discover any stray usage of iptables in OpenShift components that
  we hadn't been aware of.

### Non-Goals

- Anything beyond just notifications.

- At least for the first version we do not plan to detect and warn
  about host-network-namespace iptables usage.

## Proposal

At some point in the lifecycle of (hopefully) every pod-network pod,
we will run some piece of software that checks for iptables rules in
the network namespace of the pod, and if it finds any, it will notify
the customer in some way.

Eventually we also want to try to detect host-network iptables rules,
but this is trickier because (a) both ovn-kubernetes and openshift-sdn
currently create numerous rules in the host network namespace, which
we would have to know how to ignore, (b) other OCP components also
currently create iptables rules in the host network namespace (e.g.,
the gcp-routes script in MCO), (c) if we find iptables rules in the
host network namespace, we have no way of figuring out who created
them anyway (unless we catch them at the time of creation).

### Open Questions

#### What exactly do we detect?

This is pretty easy: if there are any iptables rules in a pod network
namespace, then there's a problem.

(Actually, currently both openshift-sdn and ovn-kubernetes create a
few iptables rules in every pod network namespace, but there are PRs
open to fix that. ([openshift/ovn-kubernetes #1946], [openshift/sdn
#581]. However, to avoid problems during upgrade, we will need to make
sure that either (a) the checker is not run when there are still
running pods created by an older OCP, or (b) the checker ignores the
rules created by old versions of ovn-kubernetes/openshift-sdn.)

We can also limit the check to pods which have at least one container
which is `privileged` or has `CAP_NET_ADMIN`, since other pods would
not be able to create iptables rules.

[openshift/ovn-kubernetes #1946]: https://github.com/openshift/ovn-kubernetes/pull/1946
[openshift/sdn #581]: https://github.com/openshift/sdn/pull/581

#### At what point in the pod lifecycle do we do the detection?

There are a few possibilities here (and this also ties into the "how"
question below):

  1. **Asynchronously, after the pod first becomes Ready**. We don't want
     to check any earlier than that, because if we did we might check
     before the pod has created the rules. (We assume it's unlikely a
     pod would create iptables rules after it is already fully up and
     running.)

  2. **Synchronously, when the pod is deleted**. (In particular, we
     could hook into the CNI deletion process at some point.) This
     would catch iptables rules created at any point in the pod
     lifecycle, but would mean that we didn't emit any
     notification/warning about the pod until after it was gone, which
     seems wrong.

  3. **Synchronously, at the moment an iptables rule is created**, by
     using an eBPF trace or something similar to catch the kernel
     syscall.

  4. **Asynchronously, via some periodic check of all pods in the
     cluster**. This requires the least integration with other
     components, but potentially misses short-lived pods.

The eBPF solution feels architecturally nice, but it gets more
complicated the more you look at it. In particular, since RHEL uses
`iptables-nft`, we would need to hook into the nftables API, not
iptables, and we would need to distinguish "real" nftables rules
(which are fine) from `iptables-nft` nftables rules (which are bad).
Additionally, we might need to be able to recognize and ignore the
sdn- and ovn-k-created iptables rules. Also, since the network
observability operator is still optional, we would have to have our
own loader, userspace component, etc, as well.

#### Where does the detector run from and how?

The different "when"s have different possibilities, but:

  1. For the "asynchronously after ready" and "synchronously on
     delete" cases, we could patch some component involved in pod
     lifecycle, such as kubelet, cri-o, multus, or ovn-kubernetes (and
     openshift-sdn). In all such cases though, we'd have to carry a
     local patch against some upstream component.

  2. For the eBPF case, we'd presumably deploy a DaemonSet that would
     install the eBPF program and also run a daemon to receive
     notifications from it.

  3. For the periodic check case, we would need to periodically run
     something on every node that looked into the network namespace of
     every pod-network pod on the node.

       - This could be deployed as a DaemonSet, but it would have to
         be `privileged`. (Ideally we'd like to deploy something that
         combined the properties of a CronJob and a DaemonSet, but no
         such object exists.)

       - Alternatively it could be run as a systemd service deployed
         via a MachineConfig. That feels more secure to me? We may
         need to give it some sort of k8s credentials though,
         depending on how we decide to alert the user.

#### How do we alert the customer when we find iptables-using pods?

We want the notifications to be (a) noticeable, and (b) clearly
associated with specific pods. Given (b), metrics seem wrong, and it
seems like Events would be a better fit.

I'm not sure if we need to do anything more to call the admin's
attention to the generated events?

### Workflow Description

Mostly N/A, though we need to figure out exactly what way(s) the
notifications are presented to the user.

### API Extensions

None

### Risks and Mitigations

Discussed above in Open Questions

### Drawbacks

In general, we feel that it is important to warn customers about the
impending deprecation, and so _some_ solution is needed.

## Design Details

TBD

### Test Plan

There are two parts to this:

  1. We will need to add an e2e test to ensure that the alerter is
     working.

  2. We will need to modify the CI report-generating scripts to
     analyze the outputs of the iptables alerter during each CI run,
     filter out any expected results from our e2e test, and report the
     rest as some sort of warning (and eventually, as an error), so
     that we are alerted to any unexpected usage of iptables by our
     own pods.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A. The feature is not expected to go through Dev Preview.

#### Tech Preview -> GA

N/A. The feature is not expected to go through Tech Preview.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

As mentioned above, at least the first release will need to deal with
the existence of pods created by older versions of OCP which contain
iptables rules created by ovn-kubernetes or openshift-sdn.

Later releases should not have to deal with this or any other skew
issues.

### Operational Aspects of API Extensions

N/A

#### Failure Modes
#### Support Procedures

## Implementation History

- Initial proposal: 2023-10-31

## Alternatives

(Move unused options here after deciding how to implement.)
