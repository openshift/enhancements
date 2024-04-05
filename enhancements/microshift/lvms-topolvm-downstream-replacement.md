---
title: lvms-topolvm-downstream-replacement
authors:
  - jakobmoellerdev
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "DanielFroehlich, PM"
  - "copejon, MicroShift Owner of LVMS"
  - "jogeo, QE lead"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - jerpeter1
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2024-03-25
last-updated: 2024-03-25
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPEDGE-919
replaces: []
superseded-by: []
---

# Replacing upstream TopoLVM with a minified version of LVMS in MicroShift

## Summary

This enhancement describes the proposed strategy to move from repackaged 
upstream TopoLVM to a minified version of LVMS in MicroShift starting with 4.17.

## Motivation

Based on [OCPEDGE-830](https://issues.redhat.com/browse/OCPEDGE-830) 
we believe that we can provide a sufficient alternative to the TopoLVM 
upstream with LVMS while maintaining full backwards compatibility 
within MicroShift.

We want to create a RFE that encompasses this change and allows 
the MicroShift team to review / acknowledge / veto the proposal 
based on our findings and suggestions.

### User Stories

As an edge device administrator, I want to make the most of my resources
and do not want to use various CSI sidecars that introduce unnecessary
memory and CPU overhead. I want to use a single, lightweight solution
that is easy to maintain and provides the same functionality as the
current solution.

As a MicroShift user using the TopoLVM storage driver, I want to ensure
that my previously created and used storage classes, deviceClasses and PVCs
are still usable & upgradeable after the change to the downstream driver.

As a MicroShift PM, I want to ensure that LVMS means the same feature set
to the end user in MicroShift as it does in OpenShift.

As a LVMS maintainer, I do not want to keep maintaining the bundled TopoLVM
images only for MicroShift. I want it to switch so that maintenance and 
support can be done solely based on the LVMS project.

As a LVMS maintainer, I want to ensure that the LVMS project is used
in as many places as possible to ensure that it is well tested and
maintained.

### Goals

* Switch from using repackaged registry.redhat.io/lvms4/topolvm-rhel9 images
  to using a minified version of LVMS in MicroShift based on registry.redhat.io/lvms4/lvms4-lvms-operator-bundle.
* Switch from using CSI Sidecars to using the LVMS Deployment optimized for Edge.

### Non-Goals

* Ensuring that LVMCluster works for all use cases in MicroShift.
  Instead, we will focus on allowing custom deviceClasses to be used 
  outside the LVMCluster Custom Resource.
* Integrating all usual MicroShift TopoLVM use cases into LVMS. (think of LVM RAID, etc.)
  Instead, we will rebrand our documentation and put forward best practices 
  for using LVMS in MicroShift based on LVMCluster with an extended documentation
  for specialised use cases.
* Forcing MicroShift users to migrate to LVMS CustomResources. We will allow
  the use of the old deviceClass mechanism without a migration path and permanently
  allow custom deviceClass management.

## Proposal

With 4.16 and the [controller consolitation initative in LVMS](https://issues.redhat.com/browse/OCPEDGE-689)
we no longer use any of our own upstream images like TopoLVM or the CSI Sidecars. 
We would like to introduce a conformant way to rebase MicroShift based on LVMS, not TopoLVM in the future.
This will allow us to no longer ship topolvm as a component,
but embed it into LVMS fully as well as maintain LVMS as the only
storage driver in MicroShift and OpenShift we have to support for lvm2 support.

### Workflow Description

1. Edge device administrator deploys a host with MicroShift 4.15 or 4.16
   installed, and maintains a storageClass, and deviceClass as per 
   [the documentation reference](https://github.com/openshift/microshift/blob/main/docs/contributor/storage/configuration.md).
2. Software runs, time passes.
3. Edge device administrator updates the host to run MicroShift 4.17.
4. Edge device restarts.
5. MicroShift restarts.
6. MicroShift removes the old TopoLVM deployment without removing the CRDs.
   All existing storageClasses and deviceClasses are still present.
7. MicroShift installs LVMS with a LVMCluster CustomResource.
   * The LVMCluster is configured with `.spec.configurationPolicy=ExternalConfiguration`
     which will trigger a deployment of the driver which can use the existing
     deviceClasses and storageClasses.
   * If LVMS does not detect a lvmd.yaml file (at runtime), it will error out,
     as an external configuration is now mandatory for correct configuration.
     Note that if the lvmd.yaml file is not present when starting MicroShift, we should
     [keep the current behavior of not enabling LVMS and skip deploying it from the assets.](https://github.com/openshift/microshift/blob/main/pkg/components/storage.go#L83)
   * When External Configuration is not used, then not specifying any deviceClasses
     will result in rejection of the LVMCluster specification.
   * Note that while it is eventually planned to decouple the CSI stack from MicroShifts
     core resources, this enhancement only targets the migration of the existing solution,
     not the adoption of LVMS as an external component out-of-tree.
8. MicroShift continues to run, and LVMS starts the vg-manager DaemonSet with all CSI bindings included.
9. All existing configurations continue to work, with the following changes to log destinations
   * Errors of TopoLVM in the node will now be written to the vg-manager Pod for all node-relevant logs and 
     to the lvms-controller Pod for all CSI controller logs.
   * Errors of LVMCluster reconciliation will be written to the lvms-controller Pod and Events
     that are keyed to the LVMCluster resource.

### API Extensions

MicroShift will now introduce the `LVMCluster`, `LVMVolumeGroup` and `LVMVolumeGroupNodeStatus` CustomResources.
All existing documentation of LVMS on the CR usages applies and can be cross-referenced if users
want a simplified setup of their storage devices.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

This enhancement only applies to MicroShift.

### Implementation Details/Notes/Constraints

#### Enablement of configuration of no deviceClasses in LVMS

Currently, LVMS does not allow for the configuration of no deviceClasses in the LVMCluster.
This is a constraint that we will have to work around by allowing the user to specify
an empty deviceClass in the LVMCluster and then coordinating that with the vg-manager.
We have a prototype implementation that can be pushed into GA in time for the 4.17 release based on
[OCPEDGE-830](https://issues.redhat.com//browse/OCPEDGE-830) that proves this is possible.

#### Switching from TopoLVM to LVMS

The switch from TopoLVM to LVMS will be done in a single release. We will support
upgrading from 4.16 to 4.17, and from 4.15 to 4.17 (2 y-streams).

The switch will be done by removing the TopoLVM deployment and installing the LVMS
deployment. The LVMS deployment will be configured to use the existing deviceClasses
and storageClasses. There will be leftover images from the TopoLVM deployment that
will be removed during regular garbage collection cycles of the container runtime.

There should not be other regressions in the upgrade process, as the LVMS migration
is transparent to users.

### Risks and Mitigations

There is some risk that the LVMS deployment will not work with the existing
deviceClasses and storageClasses. We will mitigate this risk by testing the
upgrade process in CI and by providing a rollback mechanism in case of failure.
Specific test scenarios that are not handled in the generic LVMS use cases should especially involve
[Software RAIDs introduced with LVM Raid](https://github.com/topolvm/topolvm/blob/2597dc75821009230af3be4067d377eef4bbe0a4/docs/lvmd.md?plain=1#L28-L36) with custom create options
This is because in LVMS, these are not supported by default and require a custom deviceClass
and are now enabled for the first time within LVMS through the MicroShift deployment.

Additionally, we will provide documentation on how to correctly set up an empty
LVMCluster and how to run debugging procedures in case of failure.
Any issues discovered during this process will be remediated with custom adjustments.

### Drawbacks

The main draw back of this change is that it might cause issues during upgrade / downgrade of MicroShift. 
However, we plan to circumvent this by carefully testing the LVMS upgrade/downgrade in MicroShift through 
greenboot and the E2E test suite. Additionally, we will make sure that a rollback is possible even in case
of upgrade/downgrade failure through our usual asset recreation.
Since this is a one-time change, we believe that the benefits of moving to LVMS outweigh this potential issue
after testing accordingly.

Additionally, the change will require some additional testing to ensure that
the upgrade process works as expected. From now on we will need to issue rebase
updates for the LVMS deployment in MicroShift and that will require adjustments

## Test Plan

We will add an automated test to CI to upgrade the MicroShift version from 4.15/4.16 to 4.17 anyhow.
In this test, we will verify that the upgrade process works as expected and that the LVMS
deployment is correctly configured to use the existing deviceClasses and storageClasses.

The MicroShift Storage Tests will run regressions and additional tests to ensure that the
upgrade process works as expected.

Specifically for LVMS, we will introduce a new set of tests that will verify that the
LVMCluster is correctly configured and that the vg-manager is running as expected.

Additionally, we will provide documentation on how to use the new LVMS deployment so we will
also test that the documentation is correct and that users can follow it to set up their storage manually.

Last but not least, we will add test cases to the MicroShift Storage Tests that will verify that
LVMCluster can also be used without ExternalConfiguration and that a regular LVMS deployment would work
as expected. LVMCluster as of today is not used within MicroShift, but can be used instead of a custom
lvmd.yaml file to configure the storage driver. 

In comparison to the lvmd.yaml file, the LVMCluster owns the full configuration of the volume group 
and the logical volumes and can be used to configure the storage driver in a more declarative way. 
This is especially useful for MicroShift users that want to extend the storage driver with custom configurations. 
However, this is not a requirement for the upgrade process described in this proposal,
and we can test these cases separately since the upgrade will only initialize an empty LVMCluster.

Since Volume Groups can now be handled by LVMCluster, one might also want to consider introduction of
setting up MicroShift Installations via LVMCluster. While in the past, the lvmd.yaml file was used to
configure the storage driver, this EP does not want to break this process so it will not target moving
from lvmd.yaml to LVMCluster as part of the initial setup process.


## Graduation Criteria

### Dev Preview -> Tech Preview

Regression passes in CI for the upgrade process from 4.15 to 4.17.

### Tech Preview -> GA

- We will GA within the 4.17 release and there is no need to introduce Tech Preview for this feature.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The mechanics of upgrade and rollback for MicroShift do not change as
part of this work.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Failure Modes

N/A

## Implementation History

* https://github.com/openshift/lvm-operator/pull/576

## Alternatives

We could continously maintain the TopoLVM images in MicroShift, but this would
require additional effort and would not be in line with our goal to reduce the
number of images we maintain in LVMS.

Additionally, we could also consider not switching to LVMS at all, but this would
force us into a state where we cannot optimize from LVMS edge-optimised deployment
and would require us to maintain the TopoLVM images for the foreseeable future as well.