---
title: vsphere-storage-transfer-vmo
authors:
  - "@gnufied"
reviewers:
  - "@jsafrane"
  - "@dobsonj"
approvers:
  - "@jcpowermac"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-10-21
last-updated: 2026-10-21
status: implementable
tracking-link:
  - https://redhat.atlassian.net/browse/STOR-3166
see-also:
  - https://github.com/openshift/enhancements/pull/1961
replaces: []
superseded-by: []
---

# vSphere Storage Transfer with VMO

## Summary

Extend VMO with storage migration for vSphere CSI volumes as phase two of the
OpenShift cluster migration workflow. VMO will copy CNS volumes to datastores in
the destination vCenter environment and remap volumes, including topology,
without requiring cluster reinstallation or application-level migration.
Migration is coordinated per namespace: customers quiesce and resume workloads,
manage workload placement, and approve validation and optional source cleanup.

## Motivation

The cluster migration work tracked by
[OCPSTRAT-2677](https://redhat.atlassian.net/browse/OCPSTRAT-2677) covers moving
existing OpenShift 4.18+ clusters from vSphere/vCenter 7.x/8.x and VCF 5.x to
VCF9/VVF9, but does not cover vSphere CSI storage. Customers need a storage
migration path to complete these infrastructure upgrades while minimizing manual
work and downtime.

### User Stories

- As a cluster administrator, I want to migrate my cluster's vSphere CSI storage
  to a new vCenter environment so that I can upgrade the underlying infrastructure
  without reinstalling OpenShift or performing application-level migration.

### Goals

- Enable migration of vSphere CSI volumes to the destination vCenter environment.
- Preserve usable volume mappings, including topology, after migration.
- Minimize manual work and downtime during migration.

### Non-Goals

- Implement the infrastructure migration already covered by OCPSTRAT-2677.
- Implement cross-vCenter volume relocation using `CnsRelocateVolume` in this phase.
- Quiesce customer workloads or modify them to fit the target topology.
- Migrate raw block volumes or copy snapshot data.

## Proposal

VMO will copy CNS volumes to new datastores using
[`migtools/pvc-transfer`](https://github.com/migtools/pvc-transfer) as a library,
then remap the volumes, including topology. The copy mechanism should allow a
future replacement of `rsync` with `CnsRelocateVolume` when cross-vCenter volume
relocation becomes feasible.

### Workflow Description

Without being too verbose following is plan details about how CNS migration of volumes between vCenters will work using VMO:

1. When VMO has storage migration enabled via `VmwareCloudFoundationMigration.spec.migrateStorage` opt-in, user *must* start the migration by setting `VmwareCloudFoundationMigration.spec.state` to `Planning` . Setting it directly to `Running` will result in condition `PlanningRequired` on VCFM (short for `VmwareCloudFoundationMigration` ) object.  See section [Why VCFM must start in Planning phase](#why-vcfm-must-start-in-planning-phase-for-storage-migration).

2. At this stage customer is expected to specify `failureDomainMappings`, `storageClassMapping` if required in VCFM object. See API changes below for more information. This is required so as we can accurately map source and target vcenter deployment targets.

3. Once VMO is in `Planning` mode - VMO will create a inventory of vSphere-backed PVCs per namespace, resolves
   target failure domains and the volume mover, and reports blockers through a `StorageWorkloadMigration` object in each affected namespace. See API changes below for more details.

4. VMO only starts executing storage migration bits, after VMO has successfully configured multi-site and workers are successfully running in target vCenter. So for storage migration to start VMO must be in `TargetCapacityReady` state.
   To distinguish workers on different vCenters and facilitate namespace node
   selectors, VMO labels each source Node and MachineSet with
   `migration.openshift.io/site: <migration-uid>-source`, and each target Node
   and MachineSet with `migration.openshift.io/site: <migration-uid>-target`.

5. After the customer sets the migration to `Running`, VMO waits for ready, schedulable target capacity, prepares target volumes, and performs optional precopy. Each namespace then reaches `AwaitingQuiesce` and this state is recorded in that namespaces' `StorageWorkloadMigration` object. 

6. The customer stops all consumers, suspends mechanisms that could recreate writers, and sets `spec.actions.finalCopyApproved: true` in `StorageWorkloadMigration` object. VMO independently verifies quiescence and attachments before the authoritative final copy.

7. VMO copies every inventoried, supported vSphere-backed PVC in the namespace, releases transfer
   Pods and attachments, and rebinds the original PVC names to target PVs. At this point all the
   data from source vCenter should have been copied to target vCenter. VMO **does not**
   automatically resume customer workloads, because we do not *know* if scheduling constraints of
   existing workloads/operators match the new vCenter's failure-domain. That is why - VMO simply
   sets `StorageWorkloadMigration` object in `AwaitingPlacementUpdate` state.

8. At `AwaitingPlacementUpdate`, the customer adds the published target-site constraint to the Namespace's `openshift.io/node-selector`, preserving unrelated constraints. For example:


    ```yaml
    metadata:
     annotations:
       openshift.io/node-selector: >-
         migration.openshift.io/site=<migration-uid>-target
    ```
   

9. VMO validates placement before reporting `ReadyForResume` in `StorageWorkloadMigration` object, which means customer can at this point resume the workloads. 
   
10. The customer resumes workloads through their authoritative configuration. VMO checks target
    placement, volume handles, attachments, and readiness and set `StorageWorkloadMigration` to
    `WorkloadMigrationComplete`. At this point - VMO still needs to remove temporary namespace selectors
    which were added to support CNS volume movement, because we do not want to keep them in long
    term. But before we do that we must ask customer to validate everything and once customer sets
    `validationApproved`  in `StorageWorkloadMigration` object, VMO enters
    `StorageWorkloadMigration` object to `AwaitingPlacementCleanup`.

11. VMO will not automatically remove namespace selector that was added to customers namespaces and
    once `StorageWorkloadMigration` enters `AwaitingPlacementCleanup`  - customers are expected to
    remove those selectors from their namespaces. VMO will remove any such selector from Openshift
    system namespaces automatically. Once all such dependencies on labels added in step#4 are
    removed - VMO will automatically remove the label from namespaces and `Succeeded` phase in
    `StorageWorkloadMigration` object.

12. Removal of data from source vCenter is optional at this stage and will be targeted in next phase.


We execute these steps for every namespace that has pods/pvcs that are using vSphere CSI volumes.

#### Why VCFM must start in `Planning` phase for storage migration

The reason for doing this is - we want users to be accurately aware of what they are getting into. We do not want to do a lot of upfront work only to find migration is blocked for one or more reasons. We want to assess this even before new workers are created and 2nd vCenter is configured. 

Adding 2nd vCenter is a configuration which only had one failure-domain before could disrupt existing workloads and pod scheduling may break (because of PV's nodeAffinity) and hence it is important to validate all of storage migration before starting any work for infrastructure reconfiguration.


### API Extensions

#### VmwareCloudFoundationMigration changes

```yaml
spec:
  migrateStorage: <true|false>  # opt-in for enabling storage migration
  state: Planning
  storage:
    # Volume mover used to copy data (see "Volume movers"). Defaults to Auto.
    # this exists to support CnsRelocateVolume in future
    dataMigrationStrategy: <Auto|Rsync>
    # optional
    storageClassMapping: 
      - source: thin-csi
        target: thin-csi-2
    failureDomainMappings:
      - source: source-a
        target: target-a
      - source: source-b
        target: target-b
      - source: source-c
        target: target-c
```

* `storageClassMapping` exists to support the case of SCs with `datastoreurl` or to support vSphere volumes which were migrated from in-tree etc.
* `failureDomainMappings` support migration of one zonal cluster to another.
* A customer may override the mapped target failure domain for an individual source PVC by adding below annotation:

  ```yaml
  metadata:
    annotations:
      migration.openshift.io/target-failure-domain: target-b
  ```

If VMO can't map every vSphere PV/PVC in the cluster to a datastore in target vCenter VMO will block migration during planning phase.

#### StorageWorkloadMigration

During Planning, VMO creates one `StorageWorkloadMigration` in every namespace that contains a vSphere-backed PVC. Keeping it in the workload namespace lets it own the namespace's `VolumeCopy` objects; each `VolumeCopy` can in turn own its transfer Pods, temporary target PVC etc.

Example:

```yaml
apiVersion: migration.openshift.io/v1alpha1
kind: StorageWorkloadMigration
metadata:
  name: vcf-storage-<short-migration-uid>
  namespace: payments
spec:
  migrationRef:
    name: cluster
    uid: <migration-uid>
  actions:
    finalCopyApproved: false
    validationApproved: false
    sourceCleanupApproved: false
status:
  phase: Planning
  volumes: []
  consumers: []
  conditions: []
```

Customers write only `spec.actions`. VMO writes only `status`. VMO must never advance progress by changing the object's spec.

`status.phase` uses the following ordered values:

| Phase | Meaning |
| :---- | :---- |
| `Planning` | VMO inventories the namespace, resolves targets, and reports blockers. |
| `PreparingTargets` | The volume mover prepares and VMO verifies target volumes and target ownership. |
| `PreCopying` | The volume mover runs optional, non-authoritative seed copies. Skipped if the mover does not support precopy. |
| `AwaitingQuiesce` | VMO waits for the customer to stop all consumers and approve the final copy. |
| `FinalCopying` | VMO verifies quiescence and the volume mover runs the authoritative copies. |
| `CuttingOver` | The volume mover releases its attachments and VMO rebinds the original PVC names to the target PVs. |
| `AwaitingPlacementUpdate` | VMO waits for the customer to add the published target-site selector to the Namespace. |
| `ReadyForResume` | Storage and placement are ready and VMO asks the customer to resume the workloads. |
| `Validating` | VMO validates the resumed workloads while the customer performs application validation. |
| `WorkloadMigrationComplete` | Storage cutover and workload validation are complete; source compute retirement may proceed. |
| `AwaitingPlacementCleanup` | Source workers are gone and VMO waits for the customer to remove the temporary Namespace selector. |
| `CleaningPlacement` | VMO verifies the Namespace change and removes migration labels from Nodes and MachineSets. |
| `Succeeded` | Workload migration and all required cleanup are complete. |
| `Failed` | An unrecoverable error requires an explicit recovery action. Retriable waits remain in their current phase with conditions describing the problem. |

`status.conditions` reports readiness, blockers, retries, and customer actions within the current phase. A Planning blocker therefore leaves `phase: Planning` and sets an explanatory condition; it does not change the phase to `Blocked`.

Planning inventory records object identity, not names alone:

```yaml
status:
  volumes:
    - sourcePVC: {name: data-database-0, uid: <pvc-uid>}
      sourcePV:
        name: pvc-1234
        uid: <pv-uid>
        volumeHandle: <source-fcd-uuid>
      targetFailureDomain: target-a
  consumers:
    - apiVersion: apps/v1
      kind: StatefulSet
      name: database
      uid: <statefulset-uid>
```

VMO revalidates this inventory when the migration changes to `Running` and again before each disruptive transition. A new or replaced PVC or PV invalidates the affected plan and blocks progress until Planning is repeated.

`status.consumers` is informational. It lists the workload controllers observed consuming the
namespace's PVCs, so the customer knows what to stop and resume, and VMO knows which workloads to
check for readiness during `Validating`. A changed or replaced consumer does not block progress;
safety relies on PVC and PV identities and on the Pod and VolumeAttachment checks. Approval fields
never suppress identity, placement, attachment, or writer checks.


#### VolumeCopy


Since data migration can take long time, we track migration of each PVC via corresponding `VolumeCopy` object:

```go
type VolumeCopy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VolumeCopySpec   `json:"spec"`
	Status VolumeCopyStatus `json:"status,omitempty"`
}
```

`VolumeCopy.spec.method` records the volume mover resolved during Planning and is immutable, like `targetFailureDomain`. Mover-specific progress lives in a per-method field of `VolumeCopy.status` (for example `status.rsync`). The `VolumeCopy` objects have `ownerRef` set to `StorageWorkloadMigration` object of that namespace.

### Topology Considerations

#### Hypershift / Hosted Control Planes

At this point vSphere does not support Hypershift so this does not apply to this enhancement.

#### Standalone Clusters

Applies to OpenShift clusters on vSphere undergoing VMO migration. Detailed
placement rules are described below.

#### Single-node Deployments or MicroShift

vSphere although support single node Deployment, I am not sure if VMO will support migration of such clusters to another vCenter.

#### OpenShift Kubernetes Engine

TBD.

### Implementation Details/Notes/Constraints

#### Using VolumeMover to support pluggable data moving strategy

One of the key goals of this KEP is make actual mechanism of copying volumes pluggable. At some point if `CnsRelocateVolume` supports moving
volumes between vCenters, we would like to replace current rsync-based mechanism from `pvc-transfer` library with something like `CnsRelocateVolume`.

To this end both Rsync and `CnsRelocateVolume` will use `VolumeMover` interface to actually move the data.

```go
type VolumeMover interface {
	Method() migrationv1alpha1.VolumeCopyMethod
	Capabilities() MoverCapabilities

	// Validate runs during Planning and returns blockers. It never mutates anything.
	Validate(ctx context.Context, plan VolumePlan) []Blocker

	Prepare(ctx context.Context, vc *migrationv1alpha1.VolumeCopy) (StepResult, error)
	PreCopy(ctx context.Context, vc *migrationv1alpha1.VolumeCopy) (StepResult, error)
	FinalCopy(ctx context.Context, vc *migrationv1alpha1.VolumeCopy) (StepResult, error)
	// Release removes mover-owned Pods and attachments before cutover.
	Release(ctx context.Context, vc *migrationv1alpha1.VolumeCopy) (StepResult, error)
	// TargetVolume returns the target volume handle, capacity, and node affinity used by cutover.
	TargetVolume(ctx context.Context, vc *migrationv1alpha1.VolumeCopy) (TargetVolumeRef, error)
}

type MoverCapabilities struct {
	SupportsPreCopy bool // incremental, non-authoritative seed copies
	ConsumesSource  bool // the source FCD no longer exists after FinalCopy
	VolumeModes     []corev1.PersistentVolumeMode
	AccessModes     []corev1.PersistentVolumeAccessMode
}
```

When using `CnsRelocateVolume` , there isn't precopy, so after customer set VCFM object to `Running` from `Planning`, they should wait for `AwaitingQuiesce` state in `StorageWorkloadMigration` object and then take down their workloads (same as rsync flow) and then set `finalCopyApproved` in `StorageWorkloadMigration` object.

In this case though - a revert to older data will not be possible because `CnsRelocateVolume` deletes the data from source datastore.


### Risks and Mitigations

- The vSphere and OpenShift configuration matrix is large and difficult to test
  exhaustively.
- Recovering or resuming migration after errors may be difficult.

VMO records PVC/PV UIDs and volume handles, revalidates inventory when entering
`Running` and before disruptive transitions, and requires Planning again for new
or replaced volumes. It never adopts a same-name object with a different UID.
Customer approvals do not bypass identity, placement, attachment, or writer
checks. Source cleanup requires separate approval after validation and rollback
retention, unless explicit early cleanup acknowledgement applies.

### Drawbacks

The broad configuration matrix increases testing and support complexity.

## Alternatives (Not Implemented)

`CnsRelocateVolume` is a potential future alternative to `rsync` when moving CNS
volumes between vCenters becomes feasible.

## Open Questions [optional]

- What configuration and version combinations will be supported?
- What downtime is required for migration?

## Test Plan

I think we need to test following scenarios at very minimum:

1. Single zone OCP cluster migration from one vCenter to another.
2. Zonal OCP cluster migration with more than one datacenter ---> Zonal target vCenter with more than one datacenter
3. Single zone OCP cluster migration from one vCenter to another with RWX volumes.
4. Single zone ocp cluster with in-tree PVs --> Single zone target vCenter
5. Single zone OCP cluster with datastore URL storage classes --> Single zone target vCenter


We may have to test additional configurations such as:

1. Zonal OCP cluster migration with more than one datacenter ---> Single zone target vCenter
2. Zonal OCP cluster migration with more than one computer cluster ---> Zonal target vCenter with more than one compute cluster


At this stage, I don't know how much we will be able to automate via e2e. But we will try to automate as much as possible.

## Graduation Criteria

STOR-3166 requires passing end-to-end CI tests, completed documentation,
successfully executed QE scenarios, and merged engineering stories. Release
milestones beyond Tech Preview are TBD; the plan targets Tech Preview in
OpenShift 5.1.

### Dev Preview -> Tech Preview

TBD

### Tech Preview -> GA

TBD.

### Removing a deprecated feature

Not applicable

## Upgrade / Downgrade Strategy

Not applicable

## Version Skew Strategy

Not applicable

## Operational Aspects of API Extensions

`StorageWorkloadMigration.status` is the durable customer interface; Events are
supplemental. Conditions report blockers and retries within the current phase.
Retriable waits keep their phase; unrecoverable errors enter `Failed` and require
explicit recovery. Observed workload consumers are informational; safety checks
use PVC/PV identities, Pods, and VolumeAttachments. VMO observes customer-applied
Namespace selectors and never writes them itself.

### Monitoring and Observability

I think use case of monitoring of progress of migration from Customer perspective is covered by different states recorded in `StorageWorkloadMigration` and `VolumeCopy` objects. Those objects accurately tell the user how far the data migration has progressed and where it is stuck. I am not sure adding any prometheus metrics for user monitoring will improve the situation.

In general though I think we want to report metrics for clusters that have been migrated this way successfully and those that failed to migrate via telemetry. This will be useful to record and know. 

We should also report how PVs were migrated and how long the overall migration took place.

## Support Procedures

- Planning and target-preparation failures leave applications on source storage.
  Precopy failures leave the source authoritative and may be retried.
- Final-copy or cutover failures leave the namespace quiesced by default and
  preserve recorded source and target identities. Pausing preserves checkpoints,
  quiescence, and placement fences. A blocked namespace prevents source cleanup.
- Before target writes, rollback may restore source bindings and placement,
  except for source-consuming movers(Not implemented currently). After a workloads however writes to the target PVC such rollback will not be possible, so users must be informed about that.

## Infrastructure Needed [optional]

As according to Test Plan we will need access to more than one vCenter with following topologies:

1. Single zone vCenters with RWX volumes
2. Zonal vCenters with multiple datacenters

