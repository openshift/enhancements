---
title: precision-time-protocol
authors:
  - "@zshi-redhat"
  - "@SchSeba"
reviewers:
  - "@oglok"
  - "@fepan"
  - "@pliurh"
approvers:
  - TBD
creation-date: 2019-09-03
last-updated: 2020-08-12
status: implementable
---

# Precision Time Protocol

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

A proposal to enable Precision Time Protocol(PTP) on OpenShift.

The linuxptp package, an implementaion of PTP according to IEEE standard 1588
for Linux, can be installed on a node and configured to synchronize system
clock to remote PTP master clock.

## Motivation

NFV cRAN (Centralized Radio Access Networks) and vRAN (Virtual Radio Access
Networks) require PTP level accuracy in sub-microsecond range, associated
standards are based on PTP for time and phase synchronization.

### Goals

- Install linuxptp package on baremetal nodes
- Configure linuxptp services on baremetal nodes
- Support multiple interfaces on one node
- Support the nodes as a boundary clock

### Non-Goals

- Replace NTP
- Support PTP software timestamping
- Support PTP on platforms such as AWS, VMware, OpenStack

## Proposal

A new PTP DaemonSet is deployed in the cluster after OpenShift installation
finishes. This DaemonSet contains linuxptp package and interacts with two
Custom Resource Definitions, `NodePTPDev` and `NodePTPCfg`. `NodePTPDev`
custom resource is created for each node by PTP DaemonSet automatically,
it collects PTP capable device names and updates to status of `NodePTPDev`
custom resource. `NodePTPCfg` exposes configuration options for linuxptp
services such as ptp4l and phc2sys.

User deploys manifests that are necessary to run PTP DaemonSet which includes
resources such as namespace, role based access control, Custom Resource
Definition (`NodePTPDev` and `NodePTPCfg`), PTP DaemonSet.

PTP DaemonSet creates a custom resource of `NodePTPDev` type per each node and
updates PTP capable network device names from each OpenShift node to the status
field of this custom resource. User can refer to these interface names for
configuring linuxptp services with `NodePTPCfg`.

User configures linuxptp services and apply rules in `NodePTPCfg` custom
resource. Linuxptp services are configured via `profile` section in `NodePTPCfg`
and apply rules are configured via `recommend` section in `NodePTPCfg`.
Multiple `NodePTPCfg` custom resources can be created, PTP DaemonSet will merge
all the `NodePTPCfg` custom resources according to recommend definition and
apply all the profiles with the same priority that matches with the node 
using the node name or node label rules.

Upon receiving a notification event of `NodePTPCfg` creation or update,
PTP Daemon verify the correctness of `NodePTPCfg` custom resource and
apply the selected profile to each node. One or more profiles will be
applied to each node base on the highest priority.

### User Stories

#### Virtual Radio Access Networks

NFV vRAN workloads can run on OpenShift baremetal cluster.

### Implementation Details

The proposal introduces PTP as GA.

#### Node PTP Devices

Hardware PTP capability on network interface is detected with the following
command:

`ethtool -T <network-interface-name>`

This command shows whether a MAC supports hardware time stamping. Only devices
that contain below flags are listed as PTP capable device.

```
SOF_TIMESTAMPING_TX_HARDWARE
SOF_TIMESTAMPING_RX_HARDWARE
SOF_TIMESTAMPING_RAW_HARDWARE
```

#### PTP Clock Types

Clocks synchronized by PTP are organized under a hierachy of master-slave type.
The slaves are synchronized to their masters who can themselves be slaves to
other masters. When a clock has only one port, it can be master or slave, this
type of clock is called an `Ordinary Clock`, A clock with multiple ports can be
master on one port and slave on another, this type of clock is called a
`Boundary Clock`. The PTP Clock type supported in this proposal are both.

#### Node PTP Config

Multiple `NodePTPCfg` custom resources can be defined in the cluster,
also multiple porofiles can be applied to a specific node.
Every node will apply the highest priority profile if there are multiple profiles
with the same priority the node will apply all of them, this will support the high availability feature.
Each profile contains combination of `Interfaces`,  `ptp4lOpts`, `ptp4lConf` and `phc2sysOpts`.

#### PTP Network Transport Mode

PTP protocol supports three transport modes: IEEE 802.3, UDP IPv4 and UDP IPv6.
When using UDP network transport mode, PTP device requires IP address be
configured, however this is not supported by PTP daemon in this proposal, user
will need to configure it manually. `NodePTPCfg` might be extended in the future
to support necessary network configurations such as creating a bond interface
for PTP redundancy, assigning IP address for UDP transport mode.

#### PTP4L configuration file

Default ptp4l configuration file (ptp4l.conf) will be used when starting ptp4l
service or the user can specify a custom ptp4l configuration using the `ptp4lconf` CR field.
For every profile the operator will create a custom `ptp4l.conf` file
using the default configuration or the one provided in the CR,
then it will append the `uds-address` so the operator can start multiple `phc2sys` processes
pointing the uds files. User should not specify `-f ptp4l.conf` in `ptp4lOpts`, it is
automatically appended to `ptp4lOpts` by PTP Daemon.

#### Boundary Clock

Ptp4l supports to work as a boundary clock, to configure that using
the ptp-operator the user need to request more than one interface
in the `interfaces` field.
The operator will automaticly inject the `boundary_clock_jbod=1` into the ptp4l conf file.


#### Redundancy Options

##### PTP port redundancy

Ptp4l supports using bonding interface in active-backup mode. It uses the
active interface from bond as PTP clock and can switch to another active
interface in case of a failover. Creation of bonding interface(e.g. bond0)
is not supported in this proposal, to use bond interface as PTP device,
user needs to create it manually(for example, on RHEL nodes).

##### PTP and NTP redundancy

Chrony is deployed and enabled by default during OpenShift installation process.
This proposal doesn't support timemaster service configuration. Use of PTP will
require disabling NTP(Chrony) first. No redundancy is provided between NTP and
PTP in this proposal.

##### PTP on multiple ports

Ptp4l supports to be run multiple times on different interfaces.
It uses one of the interfaces as PTP clock and can switch to another active
interface in case of synchronization issue.


### Risks and Mitigations

In case of failure in linuxptp configuration, OpenShift nodes will be left as
no time source to sync, resulting in potential time disorder among nodes.
This can be mitigated by providing PTP and NTP redundancy via timemaster
service which automatically rolls back to use default NTP time source.
timemaster service configuration is not supported in this initial proposal.


PTP device working in UDP transport mode requires IP configuration on network
interface, if the network interface is also used by default openshift-sdn
plugin, it may destroy network connection established by openshift-sdn. This
should be documented as a risk.

## Design Details

### Test Plan

- Tests will not be conducted against real PTP grandmaster clock
- Functional tests will be implemented

### Graduation Criteria

Initial support for PTP will be Tech Preview

#### Tech Preview

- Linuxptp can be installed via container image
- Linuxptp services can be configured via CRDs
- End user documentation on how to interact with PTP DaemonSet
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Have an operator to manage PTP CRDs
- Support advanced configuration of linuxptp such as timemaster
- Support all PTP network transport modes
- Provide redundancy support for PTP, both port and time source redundancy
- Support configuration of all PTP Clock types
- Measure PTP time accuracy in real environment

### Upgrade / Downgrade Strategy

### Version Skew Strategy

PTP runs as a separate DaemonSet.

## Implementation History

### Version 4.3

Tech Preview

## Infrastructure Needed

This requires a github repo be created under openshift org to hold PTP
DaemonSet code. The name of this repo is `ptp-daemon`.
