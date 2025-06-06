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
solution for attaching devices not needing special initialization
(e.g. serial, USB, certain video devices) to containers. The goal of this enhancement is to
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
  in MicroShift

### Non-Goals

* Adapt Generic Device Plugin (e.g. create Operator) for OpenShift or other Kubernetes distribution

## Proposal

* Import Generic Device Plugin code into MicroShift binary
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
1. Kubelet updates the Node's status with new devices for scheduler to be aware of Node's capabilities
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
              mountPath: /dev/ttyACM0
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

Conversely, if a device path uses a glob pattern like `/dev/ttyACM*` and has
`count: 1` (default), and multiple devices match the pattern, requesting two serial
devices will result in both matching devices (e.g. `/dev/ttyACM0` and `/dev/ttyACM1`)
being mounted into the container. If both devices are attached to different Pods
then it's possible to use `mountPath` setting to make both devices appear as
`/dev/ttyACM0` in both containers.

#### Schema

Copied from Generic Device Plugin's codebase.

**DeviceSpec**
```go
// DeviceSpec defines a device that should be discovered and scheduled.
// DeviceSpec allows multiple host devices to be selected and scheduled fungibly under the same name.
// Furthermore, host devices can be composed into groups of device nodes that should be scheduled
// as an atomic unit.
type DeviceSpec struct {
	// Name is a unique string representing the kind of device this specification describes.
	Name string `json:"name"`
	// Groups is a list of groups of devices that should be scheduled under the same name.
	Groups []*Group `json:"groups"`
}
```

**Group**
```go
// Group represents a set of devices that should be grouped and mounted into a container together as one single meta-device.
type Group struct {
	// Paths is the list of devices of which the device group consists.
	// Paths can be globs, in which case each device matched by the path will be schedulable `Count` times.
	// When the paths have differing cardinalities, that is, the globs match different numbers of devices,
	// the cardinality of each path is capped at the lowest cardinality.
	Paths []*Path `json:"paths"`
	// USBSpecs is the list of USB specifications that this device group consists of.
	USBSpecs []*USBSpec `json:"usb"`
	// Count specifies how many times this group can be mounted concurrently.
	// When unspecified, Count defaults to 1.
	Count uint `json:"count,omitempty"`
}
```

**Path**
```go
// Path represents a file path that should be discovered.
type Path struct {
	// Path is the file path of a device in the host.
	Path string `json:"path"`
	// MountPath is the file path at which the host device should be mounted within the container.
	// When unspecified, MountPath defaults to the Path.
	MountPath string `json:"mountPath,omitempty"`
	// Permissions is the file-system permissions given to the mounted device.
	// Permissions apply only to mounts of type `Device`.
	// This can be one or more of:
	// * r - allows the container to read from the specified device.
	// * w - allows the container to write to the specified device.
	// * m - allows the container to create device files that do not yet exist.
	// When unspecified, Permissions defaults to mrw.
	Permissions string `json:"permissions,omitempty"`
	// ReadOnly specifies whether the path should be mounted read-only.
	// ReadOnly applies only to mounts of type `Mount`.
	ReadOnly bool `json:"readOnly,omitempty"`
	// Type describes what type of file-system node this Path represents and thus how it should be mounted.
	// When unspecified, Type defaults to Device.
	Type PathType `json:"type"`
	// Limit specifies up to how many times this device can be used in the group concurrently when other devices
	// in the group yield more matches.
	// For example, if one path in the group matches 5 devices and another matches 1 device but has a limit of 10,
	// then the group will provide 5 pairs of devices.
	// When unspecified, Limit defaults to 1.
	Limit uint `json:"limit,omitempty"`
}
```

**PathType**
```go
// PathType represents the kinds of file-system nodes that can be scheduled.
type PathType string

const (
	// DevicePathType represents a file-system device node and is mounted as a device.
	DevicePathType PathType = "Device"
	// MountPathType represents an ordinary file-system node and is bind-mounted.
	MountPathType PathType = "Mount"
)
```

**USBSpec**
```go
// USBSpec represents a USB device specification that should be discovered.
// A USB device must match exactly on all the given attributes to pass.
type USBSpec struct {
	// Vendor is the USB Vendor ID of the device to match on.
	// (Both of these get mangled to uint16 for processing - but you should use the hexadecimal representation.)
	Vendor USBID `json:"vendor"`
	// Product is the USB Product ID of the device to match on.
	Product USBID `json:"product"`
	// Serial is the serial number of the device to match on.
	Serial string `json:"serial"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

Enhancement is intended for MicroShift and it will extend MicroShift configuration.

### Implementation Details/Notes/Constraints

It was decided that initially the Generic Device Plugin will not be forked.
Instead we'll work with the upstream on contributing changes that would be useful to us.
Potentially but not necessarily:
- Switch to the `klog` to avoid importing another logging library by the MicroShift.
- Restructure `main` module into submodules for easier reuse within MicroShift
  (instead of copying the contents of `main`).

"Code import" approach was chosen to avoid the effort associated with
productizing a new image. The MicroShift team at this time does not own any
images and the RPMs are managed by ART. See "Alternatives" section for "container" approach.

- Importing the code as controller into the MicroShift binary means more or less
  replicating the functionality of the `main()` function.
- A duplicate of GDP's configuration structs may be used in MicroShift's 
  configuration code to make it more readable if needed.
- GDP will be an optional feature, disabled by default, but included within
  the MicroShift binary.
- MicroShift will expose the devices with `device.microshift.io/NAME` label
  (changed via configuration from `squat.ai/NAME`).

### Risks and Mitigations

Not forking means that time sensitive fixes (like security) will need to be vendor-patched.

Changes we propose might get rejected from the upstream. In such case we can
vendor-patch or create fork.

### Drawbacks

MicroShift team will need to manage the go.mod importing with all the
consequences of the Go modules versioning.

## Alternatives (Not Implemented)

### Forking

Generic Device Plugin fork would provide us a place to quickly patch time
sensitive issue (e.g. security, CVEs) however it would permanently add a
recurring maintenance effort for MicroShift team.

Trying to avoid that, team decided to skip forking and try upstream first approach,
and only if it's really necessary fork the project.

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

N/A

## Test Plan

Usually device plugins are tested using real hardware to make sure everything works.

We'll need to get creative and implement some kind of dummy serial device to
test communication between Pod utilizing the device and fake device
(in the form of a daemon on the system).

## Graduation Criteria

Generic Device Plugin for MicroShift will start as Tech Preview.

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

N/A

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

N/A
