---
title: host-level-openvswitch
authors:
  - "@squeed"
  - "@JacobTanenbaum"
reviewers:
  - "@danwinship"
  - "@smarterclayton"
  - "@crawford"
  - "@phoracek"
approvers:
  - TBD
creation-date: 2019-12-11
last-updated: 2019-12-11
status: provisional

---

# openvswitch as a systemd service


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenVSwitch (OVS) is a core component to OpenShift networking providers (openshift-sdn, OVN-Kubernetes). This enhancement proposes running OVS as a SystemD service. Currently, it is deployed as a DaemonSet.

However, we must find a way to deploy OVS such that it matches administrators' expectations. That means it should, as much as possible, conform to existing Kubernetes semantics. In other words, the API should be the source of truth, for both desired state and current status.

We propose a "sidecar" DaemonSet that ensures OVS matches a configured state. It also reflects OVS' status back in to the API and exposes logs.

## Motivation

Firstly, running OVS as a SystemD unit matches the technical reality:
1. Only one ovs-vswitchd process can run on a node. Multitenancy is built in to the daemon, not the kernel.
2. ovs-vswitchd is tightly coupled, protocol-wise, with the kernel
3. ovs-vswitchd is in the critical path for networking. Stopping it blocks **all new flows**.

Thus, the proposed solution has the following advantages:
1. Simplifies the burden on OVS maintainers by shrinking the test matrix of kernel-daemon versions.
2. Removes a known source of packet loss during upgrades
3. Allows OVS to be shared between multiple networking components (openshift-sdn, CNV).

### Goals & Requirements

1. OVS is run in a SystemD unit provided by the base OS, but is still manageable as if it were a Kubernetes Pod:
  - kubectl logs works
  - Liveness- and Readiness- probes reflect the unit’s state
  - Note: it is a non-goal that deleting the sidecar Pod stops OVS
2. OVS is stable and more available than the current state.

### Non-Goals

1. Provide a generic solution for managing SystemD units via Kubernetes
2. Reconcile all possible configuration changes (e.g. stopping / disabling OVS).

## Proposal

There is a pod that acts as a “sidecar” or “babysitter” for the openvswitch systemd unit. It needs to

- Ensure the OVS unit is enabled and started
- Provide a Liveness probe
- Provide a Readiness probe
- Apply any desired OVS configuration (loglevel, memory usage, etc)
- Print log messages to stdout

Configuration for OVS is always presented as a set of key-value pairs that are inserted in to a configuration database. If needed, we should allow custom configuration to be expressed as a ConfigMap, which is reconciled to the database.


### Implementation Details/Notes/Constraints [optional]

As a prototype, this can be implemented with just a few shell scripts. Ideally, it should be ultimately implemented as a go program that talks to the SystemD DBus API directly.


### Risks and Mitigations

The biggest risk is that OVS is not running when we need it. The nice dependency management and ordering provided by SystemD doesn’t help us here. It could happen that, for whatever reason, the OVS consumer is scheduled and the OVS sidecar is not. One simple mitigation is PodAffinity. Or, we could include the babysitter process as a container inside ovn-kubernetes node daemonset, rather than a separate daemonset.

The second risk is one of adopting existing clusters. How we migrate from containerized to non-containerized openvswitch will require careful planning.

Not to be discounted is administrative churn. We disrupted many administrators’ workflows by moving OVS from a systemd service to a container. Now we’re reversing that decision - albeit for good reason. The mitigation is to try and merge expectations.

## Design Details

### Test Plan

Continuously test making new connections while upgrading ovn-kubernetes. When this is satisfactory, add a similar test to CI suite
Manually perturb OVS and ensure the status is reflected and/or corrected

### Graduation Criteria

This will start with being the default in ovn-kubernetes, so we do not need to solve the existing-nodes problem. Then, we will need to determine how to safely convert existing openshift-sdn clusters with containerized openvswitch.

### Upgrade / Downgrade Strategy

Upgrades of openvswitch itself will now be handled by the usual RHCOS update process, so that becomes significantly simplified. Instead, the question of upgrades becomes how version skew is handled.

Downgrading is also mostly uninteresting, *except* for the case where we may downgrade back to containerized openvswitch. We will need to carefully design the daemonset (of **older versions**) such that they either stop or tolerate a host-level openvswitch.

### Version Skew Strategy

There are three components at play here, all of which can have version skew:

1. The kernel vSwitch
2. The ovs-vswitchd daemon
3. The ovs clients (openshift-sdn, ovn-kubernetes, others)

The protocol between ovs clients and ovs-vswitchd is standardized (OpenFlow v1.3) and has version negotiation. The protocol between ovs-vswitchd and the kernel vSwitch is stable by the nature of the kernel ABI guarantees, but does not otherwise have any explicit versioning.

The proposed change pins the kernel and ovs-vswitchd together. This actually **removes** the most serious risk of version-skew-caused bugs, as the two are tested as a unit. The risk of version skew affecting communication between ovs clients and the ovs-vswitchd daemon is much lower, as the protocol is versioned.

## Implementation History

- *v3.10*: openvswitch switched from host-level to containerized as part of openshift-sdn.
- *v4.4* (proposed): ovn-kubernetes uses host-level openvswitch
- *v4.5* (proposed): openshift-sdn migrates to host-level openvswitch

## Drawbacks

The biggest drawback is that we lose visibility into the openvswitch processes. If they are somehow misconfigured or not running correctly, then we may not necessarily detect this.

This proposal does not cleanly provide for multiple consumers of OpenVSwitch. It is expected that, in the near future, other node-level components would like to consume it. They may make the assumption that ovs is always enabled, which is currently true but may not always be. A mechanism for requesting generic systemd units would more easily fit this.

## Alternatives

We can continue using our existing containerized openvswitch. However, we’ve seen that this causes connection disruption that is unlikely to be tolerated.

We could design a more generic “systemd bridge layer” that manages arbitrary units. Then, all consumers of openvswitch, etc. could independently request activation. This approach may naturally follow as progress is made.

We could have the SDN processes enable openvswitch by writing the correct files to disk via the MCO. The disadvantage is that the unit’s status (logs, liveness) will not be reflected in the API.


