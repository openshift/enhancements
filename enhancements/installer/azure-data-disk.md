---
title: azure-data-disk
authors:
- "@jcpowermac"
reviewers:
- "@patrickdillon"
approvers:
- "@patrickdillon"
creation-date: 2025-04-22
last-updated: 2025-04-22
tracking-link:
- https://issues.redhat.com/browse/SPLAT-2133
see-also:
replaces:
superseded-by:
---

# Azure Multi Disk

## Summary

## Motivation

As the use of Kubernetes clusters grows, admins are needing more and more improvements to the VMs themselves to make sure they run as smoothly as possible.  The number of cores and memory continue to increase for each machine and this is causing the amount of workloads to increase on each virtual machine.  This growth is now causing the base VM image to not provide enough storage for OS needs.  In some cases, users just increase the size of the primary disk using the existing configuration options for machines; however, this does not allows for all desired configuration choices.  Admins are now wanting the ability to add additional disks to these VMs for things such as etcd storage, image storage, container runtime and even swap.

### User Stories

* As an OpenShift administrator, I want to be able to add additional disks to any of the azure VMs which are acting as a node so nodes can have additional disks for me to use to assign special case storage such as etcd data, swap, container images, etc.

### Goals

- Provide the ability on the machinepool to add additional disks that could be used for etcd, swap or user defined storage like containers. 

### Non-Goals

- Setup of the disk. That will be defined in an additional enhancement.

## Proposal


### Workflow Description


### API Extensions

This enhancement will be enhancing the installer's CRD / type used for the install-config.yaml.

#### Installer

The installer's install-config will be enhanced to allow the azure machine pools to define data disks.

```go
type MachinePool struct {
        ...
        // DataDisk specifies the parameters that are used to add one or more data disks to the machine.
        // +optional
        DataDisks []capz.DataDisk `json:"dataDisks,omitempty"`
}
```

Converting capz `DataDisk` to mapi `DataDisk`

```go
	dataDisks := make([]machineapi.DataDisk, 0, len(mpool.DataDisks))

	for _, disk := range mpool.DataDisks {
		dataDisk := machineapi.DataDisk{
			NameSuffix:     disk.NameSuffix,
			DiskSizeGB:     disk.DiskSizeGB,
			CachingType:    machineapi.CachingTypeOption(disk.CachingType),
			DeletionPolicy: machineapi.DiskDeletionPolicyTypeDelete,
		}

		if disk.Lun != nil {
			dataDisk.Lun = *disk.Lun
		}

		if disk.ManagedDisk != nil {
			dataDisk.ManagedDisk = machineapi.DataDiskManagedDiskParameters{
				StorageAccountType: machineapi.StorageAccountType(disk.ManagedDisk.StorageAccountType),
			}

			if disk.ManagedDisk.DiskEncryptionSet != nil {
				dataDisk.ManagedDisk.DiskEncryptionSet = (*machineapi.DiskEncryptionSetParameters)(disk.ManagedDisk.SecurityProfile.DiskEncryptionSet)
			}
		}

		dataDisks = append(dataDisks, dataDisk)
	}


```

capz machine spec change

```go
	for idx := int64(0); idx < total; idx++ {
		zone := mpool.Zones[int(idx)%len(mpool.Zones)]
		azureMachine := &capz.AzureMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s-%d", clusterID, in.Pool.Name, idx),
				Labels: map[string]string{
					"cluster.x-k8s.io/control-plane": "",
					"cluster.x-k8s.io/cluster-name":  clusterID,
				},
			},
			Spec: capz.AzureMachineSpec{
                                ...
				DataDisks:              mpool.DataDisks,
			},
		}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints


### Risks and Mitigations

This feature of allowing administrators to add new disks does not really introduce any risks.  The disks will be created and added to the VMs during the provisioning.  Once the VM is configured, the administrator can configure these disks to be used however they wish.  The assignment of these disks is out of scope for this feature.

### Drawbacks

N/A

## Open Questions [optional]


## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

- Installer allows configuration of data disks
- CI jobs for testing installation with data disks configured
- End user documentation, relative API stability
- Sufficient test coverage

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- User facing documentation created in OCP documentation
- E2E tests are added for testing compute nodes with data disks

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The upgrade / downgrade process is not being impacted by this feature.  No changes will need to be made if rolling back during a failed upgrade.

## Version Skew Strategy

N/A

## Support Procedures

N/A

## Alternatives

N/A

## Infrastructure Needed [optional]


## Alternatives (Not Implemented)"

n/a 

## Operational Aspects of API Extensions" 
N/A



