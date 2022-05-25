---
title: ebpf-in-ocp
authors:
  - "@danwinship"
reviewers:
  - "@TBD, for network observability"
  - "@TBD, for other existing eBPF use case X"
  - "@TBD, for other future eBPF use case Y"
  - "@dave-tucker, for bpfd"
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2022-05-25
last-updated: 2022-05-25
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
see-also:
replaces:
superseded-by:
---

# Guidelines for the Use of eBPF in OCP

## Summary

OCP developers and customers alike would like to use eBPF in OCP
clusters. At the present time, _some_ eBPF hooks are designed to be
"shareable", but others are not (and some are shareable only when used
in certain restricted ways). We need to create rules for how OCP will
use eBPF and how we expect others to use (or not use it) to prevent
conflicts later.

## Motivation

### User Stories

#### Existing Use Case Number 1

(Something about observability or tracing where people are already
using eBPF in OCP.)

#### Upcoming Network Observability Features

(Something about upcoming network observability features.)

#### Ensuring Network (and General) Supportability

As an OCP developer or support engineer, I would like to be able to
debug customer problems without having to worry that random
"unsanctioned" eBPF is messing things up in inexplicable ways.

#### Customer eBPF Bug Patching

As an OCP customer, I want to work around an OCP/RHEL bug by using a
`MachineConfig` (or something similar) to deploy a small eBPF program
to every node in my cluster, without voiding my warranty.

I might be OK with needing a support exception, since this is just to
work around a bug that should eventually be fixed.

#### Customer eBPF Feature Patching

As an OCP customer, I want to add functionality to OCP/RHEL by using a
`MachineConfig` (or something similar) to deploy a small (or
not-so-small) eBPF program to every node in my cluster, without
voiding my warranty.

I might be OK with needing a support exception.

(eg, concretely, there is a customer who wants to use eBPF to
implement [OVS-style SLB
Bonding](https://docs.openvswitch.org/en/latest/topics/bonding/#slb-bonding)
on a kernel bond interface, using eBPF. The upstream Linux maintainer
rejected a patch to implement it in the kernel driver directly, in
part because it would be so easy to implement it with eBPF.)

#### Cilium as a Supported Third-Party Network Plugin

As an OCP customer, I want to deploy an OCP cluster using Cilium as a
certified third-party network plugin, without worrying that any other
OCP components are going to interfere with it (or vice versa).

Cilium uses a wide variety of eBPF hooks, including FIXME.

#### eBPF From OLM

As a third-party developer of an operator that uses eBPF, I would like
to make my operator available via OLM and have it interoperate
smoothly with existing and future OCP releases.

#### Future eBPF Data Plane Functionality

As an OCP developer/product manager, I would like to introduce
functionality in a future release that makes use of the XDP and/or tc
eBPF hooks to modify data plane functionality, and have these new
features interact in a predictable way with existing networking
functionality, other operators, and local customizations.

#### Security

As a cluster administrator, I would like to make sure that no one is
using eBPF that I don't know about, because I am in a highly regulated
industry, and I can't have random people running mystery code in my
kernel, even if it's sandboxed.

### Goals

- Categorize the existing, near-term-planned, and long-term-envisioned
  uses of eBPF in OCP.

- Create a set of rules for using eBPF in OCP to ensure that different
  eBPF users do not clobber each other's programs and do not cause
  system-wide instability or un-debuggability.

  - We may need different rules for different kinds of eBPF
    programs.

### Non-Goals

-

## Proposal

TBD

### Workflow Description

N/A - not an end-user feature

### API Extensions

TBD - none expected

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

- Continuing to allow arbitrary eBPF without any oversight will likely
  eventually lead to buggy clusters, difficult-to-debug problems, and
  possibly security holes.

- OTOH adopting a too-strict policy toward customer use of eBPF may
  turn some customers away (and would be awkward, messaging-wise:
  "eBPF is great! You can't use it!").

  - We should look into what RHEL is doing to balance
    "supportability" vs "letting the customer do what they want"
    with eBPF.

### Drawbacks

None. We really need guidelines here.

## Design Details

### Open Questions [optional]

Many

- Consider requiring the use of
  [bpfd](https://github.com/redhat-et/bpfd) to coordinate multiple
  users of eBPF.

### Test Plan

TBD

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

TBD

### Version Skew Strategy

TBD

### Operational Aspects of API Extensions

TBD

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

- Initial proposal: 2022-05-25

## Alternatives

TBD

## Infrastructure Needed [optional]

TBD, none expected
