---
title: generic-device-plugin-for-microshift
authors:
  - pmtk
reviewers:
  - "@DanielFroehlich, MicroShift PM"
  - "@ggiguash, MicroShift contributor"
  - "@pacevedom, MicroShift contributor"
  - "@eslutsky, MicroShift contributor"
  - "@copejon, MicroShift contributor"
approvers:
  - "@jerpeter1, MicroShift Staff Engineer"
api-approvers:
  - none
creation-date: 2025-05-13
last-updated: 2025-05-13
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2031
---

# Generic Device Plugin for MicroShift

## Summary

[Generic Device Plugin](https://github.com/squat/generic-device-plugin) is a
straightforward device plugin for devices not needing special initialization
such as serial, USB, certain video devices. The goal of this enhancement is to
outline its adoption for MicroShift.

## Motivation

MicroShift is built for edge devices, which commonly use peripherals
like sensors and adapters.
Currently, MicroShift does not provide any solution to pass these peripherals
through to Pods without requiring elevated privileges.
We want to provide a solution for attaching simple devices (not requiring any special
initialization) to Pods without potentially compromising on security.

### User Stories

* As a MicroShift device administrator, I want to attach simple devices to Pods
  without elevating privileges (e.g. mounting `/dev`, adding extraneous capabilities).

### Goals

* Include functionality of
  [Generic Device Plugin](https://github.com/squat/generic-device-plugin)
  in MicroShift binary (as a controller)

### Non-Goals

* Create container images
* Adapt Generic Device Plugin (e.g. create Operator) for OpenShift

## Proposal

* Fork the [Generic Device Plugin](https://github.com/squat/generic-device-plugin)
  to `github.com/openshift` organization. Generic Device Plugin has Apache 2.0
  license and forking should not be a problem. Forked repository should include
  a reference to original repository.
* Modify fork to make it's more "code import friendly"
* Import Generic Device Plugin fork into MicroShift binary
* Integrate the Generic Device Plugin's configuration into MicroShift config
  and add a toggle option

### Workflow Description

**User** is a human user responsible for setting up and managing Edge Devices.

1. The user enables the Generic Device Plugin using MicroShift configuration
1. Optionally, the user modifies the configuration of Generic Device Plugin, 
   for example changes device groups and paths
1. The user (re)starts MicroShift
1. MicroShift starts the Generic Device Plugin controller which creates plugin
   goroutines for each device group.
1. Plugin goroutines detect the devices and report them with the Kubelet
1. Kubelet updates its status with new devices
1. User deploys a workload utilizing some device
   (could be at any point, it won't be scheduled/started until the device is available)

In order for workload to get the device attached, the **container** specification
must include the device in its limits. Below is an example of StatefulSet which
references a device in a container's limits.
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: WORKLOAD
  namespace: NAMESPACE
spec:
  template:
    spec:
      containers:
      - name: CONTAINER
        resources:
          limits:
            device.microshift.io/serial: "1"
```

### API Extensions

By including Generic Device Plugin (GDP) as code import, MicroShift configuration file
will grow to accommodate the GDP's settings.

Example config (based on default settings of GDP with changed `domain` and added `status`):
```yaml
genericDevicePlugin:
  status: { Enabled , Disabled }
  domain: device.microshift.io
  devices:
    - name: serial
      groups:
        - paths:
            - path: /dev/ttyUSB*
        - paths:
            - path: /dev/ttyACM*
    - name: video
      groups:
        - paths:
            - path: /dev/video0
    - name: fuse
      groups:
        - count: 10
          paths:
            - path: /dev/fuse
    - name: audio
      groups:
        - count: 10
          paths:
            - path: /dev/snd
    - name: capture
      groups:
        - paths:
            - path: /dev/snd/controlC0
            - path: /dev/snd/pcmC0D0c
```

Note:

Setting `count: 10` means that a device can be attached to up to 10 containers simultaneously.
It's important to note that this does not create 10 physical copies of the device.
If a Pod requests 4 allocations of this device, only a single instance of the
device (e.g. `/dev/snd`) will appear within the container.

Conversely, if a device path uses a glob pattern like `/dev/ACM*` and has
`count: 1` (default), and multiple devices match the pattern, requesting two serial
devices will result in both matching devices (e.g. `/dev/ACM0` and `/dev/ACM1`)
being mounted into the container.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

Enhancement is intended for MicroShift and it will extend MicroShift configuration.

### Implementation Details/Notes/Constraints

"Code import" approach was chosen to avoid the effort associated with
productizing a new image. The MicroShift team at this time does not own any
images and the RPMs are managed by ART. See "Alternatives" section for "container" approach.

- Following the fork, we will opt-in into the OpenShift CI to ensure it's in 
  sync and it's part of the branching automation.
- Code change to the fork should be delicate to avoid complicating
  synchronization with the upstream repository. Currently, planned changes include:
  - Switching to the `klog` to avoid importing another logging library by the MicroShift.
  - Possibly restructuring `main` module into submodules for easier reuse within MicroShift
    (instead of copying).
- Importing the code as controller into the MicroShift binary means more or less
  replicating the functionality of the `main()` function.
- A duplicate of GDP's configuration structs may be used in MicroShift's 
  configuration code to make it more readable if needed.
- GDP will be an optional feature, disabled by default, but included within
  the MicroShift binary.
- MicroShift will expose the devices with `device.microshift.io/NAME` label
  (changed via configuration from `squat.ai/NAME`).

### Risks and Mitigations

- Making changes to the fork may result in an additional effort when syncing
  upstream changes (e.g. resolving conflicts; unless we contribute the changes).
  Fortunately, GDP is not very complex product, nor it's subject to fast paced
  development because for its purpose it's mostly complete.

### Drawbacks

MicroShift team will need to manage the go.mod importing with all the
consequences of the Go modules versioning.

## Alternatives (Not Implemented)

### Container approach

An alternative to importing the code directly is the "container" approach.
It involves publishing the forked Generic Device Plugin as an container image
along with a manifest for MicroShift.
This might align better with Kubernetes principles (containerization) and could
offer broader reuse (but not necessarily by OpenShift right now), it presents
several drawbacks:
- Increased maintenance effort on MicroShift team:
  As GDP would likely not be a part of the OpenShift payload and MicroShift team
  gets help from ART in managing the RPMs. The MicroShift team does not have
  experience in productizing images.
- Size of the Generic Device Plugin image would likely be significantly larger
  than increase in the MicroShift binary size from code import. This is
  important consideration due to MicroShift's target environment.

### Other device plugin solution

Investigating this feature only three solutions were found:
- Generic Device Plugin (discussed in this enhancement)
- [Smarter Device Plugin](https://gitlab.com/arm-research/smarter/smarter-device-manager) (abandoned, last commit in Sept 2022)
- [Akri](https://docs.akri.sh/)

The Smarter Device Plugin was not considered as it is no longer actively maintained.

Akri, while an interesting approach that abstracts the Device Plugin framework,
was deemed too complex for our requirements. The MicroShift team concluded that
integrating Akri would significantly increase maintenance effort across
engineering, QE, and documentation.

Since the Generic Device Plugin (GDP) can be disabled via configuration, users
retain the flexibility to utilize upstream Akri if their needs require it.

## Open Questions [optional]

## Test Plan

Usually device plugins are tested using real hardware to make sure everything works.

We'll need to get creative and implement some kind of dummy serial device to
test communication between Pod utilizing the device and fake device
(in the form of a daemon on the system).

MicroShift's robust testing framework can host the integration tests, but it
would be good to implement a simple test (like successful compilation with the 
changes and start of MicroShift) in the GDP's fork repository.

## Graduation Criteria

Generic Device Plugin for MicroShift could start as GA.

However, if we conclude that we're uncertain about "code import" method, we
could publish GDP for MicroShift as Dev or Tech preview, gather the feedback,
and decide if we want to transition to the "container" approach.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- End-to-end tests
- End user documentation

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

Generic Device Plugin will be part of MicroShift binary so there's no need for
special consideration from user or technical point of view.

When downgrading MicroShift to a version without Generic Device Plugin,
the configuration fields of the new feature will be ignored by older binary.

## Version Skew Strategy

Generic Device Plugin fork will follow OpenShift's versioning and release schedule.

However, it might not be required for MicroShift to track the fork GDP's HEAD of a branch
to be up to date depending on the development velocity.
For example, if by 4.25 we discover we need to make a fix in GDP, unless there
were breaking changes between 4.20 and 4.25, we might only need to update the
GDP's main branch and `go.mod` in each of the MicroShift's affected branches.

## Operational Aspects of API Extensions

Disabled GDP will not take any resources - the controller will not run.

There are no resource consumption data for enabled embedded GDP
(might depend on the amount of device presets in the configuration).
The upstream manifest for GDP sets the following limits: 50m of CPU and 20Mi of memory.

## Support Procedures

If user created workload that is not starting, first step would be to `describe`
the Pod or parent object to get the reason - what's preventing from scheduling
or starting.
```sh
$ oc describe pod -n NAMESPACE POD
$ oc describe deployment -n NAMESPACE DEPLOY
$ oc describe statefulset -n NAMESPACE STATEFULSET
```

Other good place is inspecting kubelet's status for *Capacity*, *Allocatable*, and
*Allocated resources* to see if it's as expected.

The example below illustrates four types of devices available on a node with the
default config for Generic Device Plugin (see `API Extensions` section).
One serial device is already allocated, reflected by the `1` in the requests and limits under *Allocated resources*.
The value for capacity and allocatable devices indicate either:
- The count of the devices in a group, or
- The number of times a single device can be allocated.
  For instance, both `fuse` and `audio` devices might represent a single host
  device, each allowing up to 10 allocations for Pods.

```sh
$ oc describe node NODE
(...)
Capacity:
  cpu:                           12
  device.microshift.io/audio:    10
  device.microshift.io/capture:  0
  device.microshift.io/fuse:     10
  device.microshift.io/serial:   1
  device.microshift.io/video:    0
  ephemeral-storage:             941789904Ki
  hugepages-1Gi:                 0
  hugepages-2Mi:                 0
  memory:                        65300740Ki
  pods:                          250
Allocatable:
  cpu:                           12
  device.microshift.io/audio:    10
  device.microshift.io/capture:  0
  device.microshift.io/fuse:     10
  device.microshift.io/serial:   1
  device.microshift.io/video:    0
  ephemeral-storage:             941789904Ki
  hugepages-1Gi:                 0
  hugepages-2Mi:                 0
  memory:                        65300740Ki
  pods:                          250
(...)
Allocated resources:
  (Total limits may be over 100 percent, i.e., overcommitted.)
  Resource                      Requests     Limits
  --------                      --------     ------
  cpu                           535m (4%)    700m (5%)
  memory                        1457Mi (2%)  1100Mi (1%)
  ephemeral-storage             0 (0%)       0 (0%)
  hugepages-1Gi                 0 (0%)       0 (0%)
  hugepages-2Mi                 0 (0%)       0 (0%)
  device.microshift.io/audio    0            0
  device.microshift.io/capture  0            0
  device.microshift.io/fuse     0            0
  device.microshift.io/serial   1            1
  device.microshift.io/video    0            0
```

If the problem is suspected to originate in Scheduler, API Server, Kubelet, or
Generic Device Plugin, examine MicroShift's journal. Just like existing
MicroShift components include their name in the log (`kubelet`, `kube-scheduler`)
logs of the Generic Device Plugin will contain a `generic-device-plugin` marker.

## Infrastructure Needed [optional]

- Fork of https://github.com/squat/generic-device-plugin inside github.com/openshift organization.
