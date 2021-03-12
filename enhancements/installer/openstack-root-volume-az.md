---
title: openstack-root-volumes-availability-zone
authors:
  - "@mfedosin"
reviewers:
  - "@mandre"
approvers:
  - "@mandre"
creation-date: 2021-3-9
last-updated: 2021-3-9
status: implementable
---

# OpenStack Root Volumes Availability Zones

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Numerous customers have requested the ability to run the OpenShift on OpenStack IPI installer with root volumes created in specific availability zones (AZs). This document describes how to achieve that during OpenShift initial installation, and as a day2 operation.

## Motivation

In the installer, we always create root volumes in the default Volume AZ, which may cause installation problems and performance issues when the cluster is deployed on a cloud with multiple Volume AZs. To fix it and give users more flexibility we want to add new parameters that allow them to choose desired root volumes AZs.

### Goals

- Extend the installer to support root volumes availability zones
- Add new options in Cluster API Provider OpenStack machine spec

### Non-Goals

- Implement availability zones for PVs as they are already supported by Cinder CSI driver
- Choose availability zone for the image registry volume when `PVC` backend is used

## Proposal

In order to implement this feature fully, the following changes must be made:

- Add support in Custer API Provider OpenStack (CAPO)
  - Add root volumes availability zones in the Cluster API Provider OpenStack machine spec.
  - Make sure that root volumes are created in the given AZs.
- Add support in the installer
  - Add new option in the install config's OpenStack machine pool section
  - Generate correct Terraform variables using these options
  - Generate correct Machine and Machineset assets

### Design Details

#### Cluster API Provider OpenStack

We add a new optional sting parameter `availabilityZone` in the machine spec. If it is set, then the root volume will be created in that availability zone. Here is the [implementation](https://github.com/openshift/cluster-api-provider-openstack/pull/168).

**Note:** Currently there is no validation that the zone really exists.

#### Installer

We add a new optional string array parameter `zones` to the `rootVolume` object in `openstack` machine pool.

```yaml
rootVolume:
  description: RootVolume defines the root volume for instances
    in the machine pool. The instances use ephemeral disks
    if not set.
  properties:
    size:
      description: Size defines the size of the volume in
        gibibytes (GiB). Required
      type: integer
    type:
      description: Type defines the type of the volume. Required
      type: string
    zones:
      description: Zones is the list of availability zones
        where the root volumes should be deployed. If no zones
        are provided, all instances will be deployed on OpenStack
        Cinder default availability zone
      items:
        type: string
      type: array
  required:
  - size
  - type
  type: object
```

Each item in the array corresponds to a Compute availability zone in `zones` section of `openstack` [machine pool](https://github.com/openshift/installer/blob/master/pkg/types/openstack/machinepool.go#L26-L29). Thus, Volume AZs will be assigned to machines in different Compute AZs in a circle.

#### Examples of a Compute machine pool with root volume AZs

##### Example #1 (Multiple Compute and Volume AZs)

```yaml
compute:
- name: worker
  platform:
    openstack:
      type: ml.large
      zones: ["ComputeAZ1", "ComputeAZ2", "ComputeAZ3"]
      rootVolume:
        size: 30
        type: performance
        zones: ["VolumeAZ1", "VolumeAZ2", "VolumeAZ3"]
  replicas: 3
```

In the given example the installer will generate 3 CAPO machine specs with next AZ pairs: `ComputeAZ1-VolumeAZ1`, `ComputeAZ2-VolumeAZ2`, `ComputeAZ3-VolumeAZ3`

##### Example #2 (Multiple Compute and just one Volume AZ)

```yaml
compute:
- name: worker
  platform:
    openstack:
      type: ml.large
      zones: ["ComputeAZ1", "ComputeAZ2", "ComputeAZ3"]
      rootVolume:
        size: 30
        type: performance
        zones: ["VolumeAZ"]
  replicas: 3
```

In the given example the installer will generate 3 CAPO machine specs with next AZ pairs: `ComputeAZ1-VolumeAZ`, `ComputeAZ2-VolumeAZ`, `ComputeAZ3-VolumeAZ`

**Note:** If either Compute or Volume AZs are omitted in the config, the default one (normally `nova`) will be used for installation.

### User Stories

As an enterprise OpenStack cluster administrator, I want to install the OpenShift IPI installer on root volumes in predefined availability zones. It is important in terms of stability when masters are in different racks, and also in terms of performance when masters and workers use different availability zones.

### Test Plan

Validations and testing manifest generation for this feature will be added to the installer to make sure that correct usage is enforced and that this feature does not hinder the usage of other features. To ensure GA readiness, it will be vetted by the QE team as well to make sure that it works with the following use cases: scale-in, scale-out and upgrades.

### Graduation Criteria

This enhancement will follow standard graduation criteria.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Community Testing
- Sufficient time for feedback
- Upgrade testing from 4.7 clusters utilizing this enhancement to later releases
- Downgrade and scale testing are not relevant to this enhancement
- E2E testing is not necessary for this feature

### Infrastructure Needs

- OpenStack cluster with multiple Volume Availability zones

#### Upgrade/Downgrade strategy

This feature will be released in 4.8, and will not be backported. In 4.8 after ugrade users will get an ability to additionally specify AZs for root volumes. Since it's an optional parameter in the CAPO machine spec, it doesn't break backward compatibility. After downgrade to 4.7 all created machine root volumes stay in the same AZs, but users won't be able to specify them anymore for new machines.
