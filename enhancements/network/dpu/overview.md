---
title: DPU Support in OCP (Overview)
authors:
  - "@danwinship"
reviewers:
  - "@derekwaynecarr"
  - "@dcbw"
  - "@zshi-redhat"
approvers:
  - "@derekwaynecarr"
  - "@dcbw"
creation-date: 2021-04-15
last-updated: 2021-09-15
status: provisional
---

# DPU Support in OCP (Overview)

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

As servers have greater and greater networking needs, hardware vendors
have created increasingly more complex and powerful network cards. The
latest iteration of this is "DPUs" (Data Processing Units) that
include an entire ARM system running Linux on the NIC. With such a
NIC, we can offload nearly all network processing from the host
computer onto the DPU, freeing up host CPU resources for customer
workloads (and allowing faster data transfer by ensuring the network
stack doesn't need to compete for CPU with the customer workload).

However, doing this requires installing and managing an appropriate
software stack on all of the NICs in the cluster. This software will
need to be kept in sync with the software on the host PC (eg, as new
functionality is added to OCP). The obvious answer to this problem is
to run OpenShift on the NIC as well as on the host.

The initial target of this work is the [NVIDIA BlueField-2 DPU].
However, similar products are being worked on by other vendors which
we will eventually want to support, so the solution should not be too
specialized to this one product.

[NVIDIA BlueField-2 DPU]: https://www.nvidia.com/en-us/networking/products/data-processing-unit/

## Glossary

- **Smart NIC** - a vaguely-defined term for an enterprise-class NIC
  with extensive offload capabilities, but not necessarily with the
  ability to run arbitrary user software. For example, the NVIDIA
  Mellanox ConnectX-5 and ConnectX-6.

- **DPU** / **Data Processing Unit** - a Smart NIC with a full CPU,
  RAM, storage, etc, running a full operating system, able to offload
  network processing and potentially other tasks from its host system.
  For example, the NVIDIA BlueField-2.

  (Some vendors other than NVIDIA are using the terms "Smart NIC" and
  "DPU" slightly differently, but for now at least, this is the way we
  are defining these terms.)

- **DPU NIC** - refers to the DPU and its CPU, OS, etc (as opposed to
  the CPU, OS, etc, of the x86 server that the DPU is installed in). A
  **DPU NIC node** or **DPU NIC worker** is an OCP worker node
  running on a DPU NIC.

- **DPU Host** - refers to an x86 server (and its CPU, OS, etc) which
  contains a DPU. A **DPU Host node** or **DPU Host worker** is an OCP
  worker node running on a DPU Host.

- **Two-Cluster Design** - any architecture for deploying an OCP
  cluster with (some) nodes containing DPUs, where the DPUs are nodes
  in a second OCP cluster. (For purposes of this term, the
  architecture still counts as "two-cluster" even if HyperShift is
  involved and there are actually three clusters.)

- **Infra Cluster** - in a Two-Cluster Design, the OCP cluster which
  contains the DPU NIC workers. (It may also include master nodes
  and/or non-DPU workers, depending on the particular details of the
  two-cluster design.)

- **Tenant Cluster** - in a Two-Cluster Design, the OCP cluster
  containing the DPU Host workers. (It may also include master nodes
  and/or workers without DPUs.)

## Motivation

### User Stories

#### Network Offload

As a cluster administrator, I would like to offload network processing
from the host CPU to the NIC, to increase throughput and to free up
host CPU cycles for workloads rather than network processing.

In particular:

- We can move OpenFlow processing and (eventually) IPsec encryption
  from the DPU Host CPU to the DPU. (These would be fully offloaded to
  custom silicon in the DPU, not simply run as processes on the
  ARM-based DPU.)

- We can move all of the node-side OVS, OVN, and OVN Kubernetes
  daemons from the DPU Host CPU to the DPU NIC CPU. (The host side
  would retain only a CNI plugin binary.)

- Is there interest in moving DNS or routers off the host CPU?

- ...

#### Security Offload

As a cluster administrator, I would like to use a DPU to monitor its
host node. The NIC would run in an isolated mode, where the host OS
has no control over it. Possibly even the cluster administrators of
the host's OCP cluster would have no control over the NIC.

#### Storage Offload

(FIXME: Details? NVIDIA talks about this case in their marketing
materials but I haven't heard as much about it in an OCP context and
maybe this isn't an important case for us?)

The BlueField-2 cards have M.2 NVMe ports and can have directly
attached SSDs.

### Goals

- Design a system for provisioning and maintaining a RHEL-based
  software stack on DPUs to support features such as "[Smart NIC
  OVN Offload]".

  - Although this enhancement is currently written to assume that
    this means running OCP on the NICs, this is not yet cast in
    stone. Some other approaches are discussed under "Alternatives"
    and one of those could potentially replace the OCP approach.

  - Although not part of the initial design, we assume some
    customers may be interested taking advantage of the NIC/host
    separation to run additional security and monitoring
    functionality on the NICs in the future. Thus, we are interested
    in designs with more isolation between the NIC and the host,
    even though that is not necessarily interesting for the primary
    network-offload use case.

- Design an appropriate architecture for an environment containing a
  mix of x86_64 hosts with "dumb" NICs, x86_64 hosts with DPUs, and
  the ARM-based DPUs themselves. (As discussed below, such an
  environment may involve one or more OCP clusters.)

- Define how OCP installs and upgrades will work in such an
  environment.

[Smart NIC OVN Offload]: https://github.com/openshift/enhancements/pull/732

### Non-Goals

- Support for network plugins other than OVN Kubernetes. The
  architecture designed here may not be generically applicable and is
  not guaranteed to be usable by other network plugins even if they
  independently implement their own BlueField, etc, support.

- Support for NICs that are called "Smart NICs" or "DPUs" but which
  don't meet our requirements (below).

### Supported Hardware

We want to support DPUs that are vaguely "BlueField-2-ish". eg:

  1. The NIC has a sufficiently-powerful ARM processor (or,
     theoretically, a processor of some other architecture that RHCOS
     supports), and sufficient RAM, storage, etc, to run as an OCP
     node.

       - Current BlueField-2 specs mention "Up to 8 Armv8 A72 cores"
         and "8GB / 16GB / 32GB of on-board DDR4". It is possible we
         would only support a subset of SKUs.

       - The terms "Smart NIC" and "DPU" are also being used to refer
         to NICs that just have programmable ASICs or FPGAs, rather
         than a full computing environment. These are out of scope.

  2. The NIC has a Baseboard Management Controller that supports IPMI,
     Redfish, or some other protocol [supported by the
     baremetal-operator], to control and provision it independently of
     its host.

  3. The NIC implements SR-IOV in such a way that we can connect
     pods on the host directly to an OVN bridge on the NIC.

  4. All of the above can be done without running any proprietary
     drivers or third-party software on either the host's or the NIC's
     RHCOS installation.

[supported by the baremetal-operator]: https://github.com/metal3-io/baremetal-operator/blob/master/docs/api.md#bmc

## Proposal

The details will be figured out in further enhancements.

### Risks and Mitigations

...

## Design Details

### Cluster Architecture

The expected (tenant) cluster architecture at this time is:

- Three master nodes, which do not contain DPUs.
- Some number of worker nodes (maybe 0, but probably not) that do not contain DPUs.
- A medium-to-large number of workers that do contain DPUs.

(The initial Developer Preview will require at least 1 non-DPU worker
node.)

## Open Questions

### API Extensions

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

- 2021-04-19: Initial proposal
- 2021-09-14: Simplified to be just an overview

## Drawbacks

## Alternatives

### Running RHCOS But Not OCP on the NIC

Rather than running a full OpenShift installation on the DPU, we could
instead provision a simple RHCOS system from the node, and have each
node manage its own DPU's software and configuration directly.
(Indeed, this is how the proof of concept for OVN offload was
implemented.)

However, we expect customers to be interested in the greater degree of
security provided by having the DPU NIC software be entirely isolated
from the DPU Host software. Eventually, this can evolve into a system
like AWS's "Nitro", where not just network processing, but also other
security and cluster monitoring functionality is handled by the DPU,
entirely outside the control of the host CPU.

In a HyperShift environment, it is possible that the DPU NICs could be
provisioned (with plain RHCOS) by something outside the control of the
"tenant" cluster. (As far as I can tell, the complete architecture for
bare-metal HyperShift has not been worked out yet, so it is not clear
exactly how provisioning even of normal nodes will work there.)

## Infrastructure
