---
title: manage-boot-images
authors:
  - "@djoshy"
reviewers: 
  - "@yuqi-zhang"
  - "@mrunal"
  - "@cgwalters, for rhcos context" 
  - "@joelspeed, for machine-api context" 
  - "@sdodson, for installer context"
approvers:
  - "@yuqi-zhang"
api-approvers: 
  - None
creation-date: 2023-10-16
last-updated: 2022-10-17
tracking-link:
  - https://issues.redhat.com/browse/MCO-589
see-also:
replaces: 
  - https://github.com/openshift/enhancements/pull/368
superseded-by: 
  - https://github.com/openshift/enhancements/pull/201
---

# Managing boot images via the MCO

## Summary

This is a proposal to manage bootimages via the `Machine Config Operator`(MCO), leveraging some of the [pre-work](https://github.com/openshift/installer/pull/4760) done as a result of the discussion in [#201](https://github.com/openshift/enhancements/pull/201). This feature will only target standalone OCP installs. It will also be user opt-in and is planned to be released behind a feature gate.

For Installer Provisioned Infrastructure(IPI) clusters, the end goal is to create a mechanism that can:
- update the boot images references in `MachineSets` to the latest in the payload image
- ensure stub ignition referenced in each `Machinesets` is in spec 3 format

For User Provisioned Infrastructure(UPI) clusters, this end goal is to create a document(KB or otherwise) that a cluster admin would follow to update their boot images.


## Motivation

Currently, bootimage references are [stored](https://github.com/openshift/installer/blob/1ca0848f0f8b2ca9758493afa26bf43ebcd70410/pkg/asset/machines/gcp/machines.go#L204C1-L204C1) in a `MachineSet` by the openshift installer during cluster bringup and is thereafter unmanaged. These boot image references are not updated on an upgrade, so any node scaled up using it will boot up with the original “install” bootimage. This has caused a myriad of issues during scale-up due to this version skew, when the nodes attempt the final pivot to the release payload image. Issues linked below:
- Afterburn [[1](https://issues.redhat.com/browse/OCPBUGS-7559)],[[2](https://issues.redhat.com/browse/OCPBUGS-4769)]
- podman [[1](https://issues.redhat.com/browse/OCPBUGS-9969)]
- skopeo [[1](https://issues.redhat.com/browse/OCPBUGS-3621)]

Additionally, the stub secret [referenced](https://github.com/openshift/installer/blob/1ca0848f0f8b2ca9758493afa26bf43ebcd70410/pkg/asset/machines/gcp/machines.go#L197) in the `MachineSet` is also unmanaged. This stub is used by the ignition binary in firstboot to auth and consume content from the `machine-config-server`(MCS). The content served includes the actual ignition configuration and the final pivot OS image. The ignition binary now does first boot provisioning based on this, then hands off to the `machine-config-daemon`(MCD) first boot service to do the final pivot. As 4.6 and up clusters only understood spec 3 ignition, and as the unmanaged ignition stub is only spec 2, this was now an incompatibility. This would prevent new nodes from joining a cluster that had been upgraded past 4.5, but was originally a 4.5 or lower at install time. Issue linked below:
- SAN [[1](https://issues.redhat.com/browse/OCPBUGS-1817)]


### User Stories

* As an Openshift engineer, having nodes boot up on an unsupported OCP version is a security liability. By having nodes directly boot on the release payload image, it helps me avoid tracking incompatibilities across OCP release versions and shore up technical debt(see issues linked above). 

* As a cluster administrator, having to keep track of a "boot" vs "live" image for a given cluster is not intuitive or user friendly. In the worst case scenario, I will have to reset a cluster(or do a lot of manual steps with rh-support in recovering the node) simply to be able to scale up nodes after an upgrade. If I'm managing an IPI cluster, once opted in, this feature will be a "switch on and forget" mechanism for me. If I'm managing a UPI cluster, this would provide me with documentation that I could follow after an upgrade to ensure my cluster has the latest bootimages.

### Goals

The MCO will take over management of the boot image references and the stub ignition. The installer is still responsible for creating the `MachineSet` at cluster bring-up of course, but once cluster installation is complete the MCO will ensure that boot images are in sync with the latest payload. From the user standpoint, this should cause less compatibility issues as nodes will no longer need to pivot to a different version of rhcos during node scaleup.

### Non-Goals

- The new subcontroller does not provide a solution for UPI as it does not use `MachineSets`. We plan to support a UPI solution via documentation that is based on this workflow.
- This is meant to be a user opt-in feature, and if the user wishes to keep their boot images static it will let them do so.
- This does not intend to solve [booting into custom pools](https://issues.redhat.com/browse/MCO-773). 
- This does not target Hypershift, as [it does not use machinesets](https://github.com/openshift/hypershift/blob/32309b12ae6c5d4952357f4ad17519cf2424805a/hypershift-operator/controllers/nodepool/nodepool_controller.go#L2168).

## Proposal

__Overview__

- The `machine-config-controller`(MCC) pod will gain a new sub-controller `machine_set_controller`(MSC) that monitors `MachineSet` changes and the `coreos-bootimages` [ConfigMap](https://github.com/openshift/installer/pull/4760).
- Before processing a MachineSet, the MSC will check for the existence of `io.openshift.mco-managed=true` annotation. If it is not present, the MSC will exit the reconciliation loop. This is how `MachineSets` are opted-in to this mechanism.
- Based on platform and arch type, the MSC will check if the boot images referenced in the `providerSpec` field of the `MachineSet` is the same as the one in the ConfigMap. Each platform(gcp, aws...and so on) does this differently, so this is a good opportunity to split the work up between platforms and see if the implementation is effective. The ConfigMap is considered to be the golden set of bootimage values, i.e. they will never go out of date.
- Next, it will check if the stub secret referenced is spec 3. If it is spec 2, the MSC will try create a new version of this secret by trying to translate it to spec 3. This step is platform/arch agnostic. Failure to up translate will cause a degrade and the sub-controller will exit without patching the `MachineSet`.
- Finally, if the MSC will attempt to patch the `MachineSet` if required. Failure to do so will cause a degrade. 
- Any other failures in the above steps will report an error; but degrades will only be in the specific cases mentioned above. Certain failures may also be as a result of an unsupported architecture or an unsupported platform. This is necessary because support for platforms will be phased in(and some platforms may not even desire this support)

__Rolling back__

The very first time a `MachineSet` is patched, the MSC will also backup the following via annotation to the `MachineSet`:
- `io.openshift.mco-pre-managed-image=` storing the original provider image reference
- `io.openshift.mco-pre-managed-secret=` storing the original stub secret

A roll back can be done by opting out the `MachineSet`, this will trigger the MSC to restore the MachineSet to "factory" values by using the annotations mentioned above.
This is an important mitigation in case things go wrong(invalid bootimage references, incorrect patching... etc).

__UPI__

For UPI, the proposal is to create platform specific documentation based on our implementation of the the above work. If this feature is
opted in on a UPI install, it is necessary to warn(degrade or some other way) the cluster admin to indicate that this functionally is essentially a no-op in the absence of machinesets.

### Workflow Description

- To enroll a `MachineSet` for boot image updates, the cluster admin should add an annotation `io.openshift.mco-managed=true` to the `MachineSet`.
- To un-enroll(and effectively rollback) the `MachineSet` from boot image updates, the cluster admin should remove the `io.openshift.mco-managed=true` annotation from the `MachineSet`.

#### Variation and form factor considerations [optional]

Any form factor using the MCO and `MachineSets` will be impacted by this proposal. So case by case:
- Standalone OpenShift: Yes, this is the main target form factor.
- microshift: No, as it does [not](https://github.com/openshift/microshift/blob/main/docs/contributor/enabled_apis.md) use `MachineSets`.
- Hypershift: No, Hypershift does not have this issue.

### API Extensions

We may have to make some changes to MCO CRDs for the opt-in feature.

### Implementation Details/Notes/Constraints [optional]

![Sub Controller Flow](manage_boot_images_flow.jpg)

![MachineSet Reconciliation Flow](manage_boot_images_reconcile_loop.jpg)

The implementation has a GCP specific POC here:
- https://github.com/openshift/machine-config-operator/pull/3980

Possible constraints:
- Ignition spec 2 to spec 3 is not deterministic. Some translations are unsupported and as a result not all stub secrets can be managed. In these cases, failure will be reported, and it will cause a cluster degrade.
- See Open questions below for some more possible constraints.

### Risks and Mitigations

The biggest risk in this enhancement would be delivering a bad boot image. To mitigate this, we have outlined a rollback option.

How will security be reviewed and by whom? TBD
This is a solution aimed at reducing usage of outdated artifacts and should not introduce any security concerns that do not currently exist. 

How will UX be reviewed and by whom? TBD 
The UX element involved include the user opt-in and opt-out, which is currently up for debate. 

### Drawbacks

TBD, based on the open questions below.

## Design Details

### Open Questions

- Should we have a like a global switch that opt-in all `MachineSets` for this mechanism?
- Somewhat related to above, would we also want to allow opting out without rolling back? This is for a situation for the customer would not want to update the boot images any longer, but would like to keep the current image instead of the "factory" after rolling back. Not sure if anyone would use this, but though it was worth considering.
- This proposal relies on the golden configmap having a target value for every platform/arch combination that we use today. I've [noticed](https://issues.redhat.com/browse/MCO-793) some cases like vsphere don't have a reference as it stands today. Why is that? Are there scenarios not requiring boot image updates?
- Heterogenous platform(nodes span across infra providers) concerns. Do such clusters exist? If they do, do they use `MachineSets`? The current proposal assumes the same platform across all nodes and uses the infra object to determine the cluster platform. It reports anror if there is a platform mismatch and will exit non-fatally.
- Hetergenous architecture concerns. I think these exist, but do they use `MachineSets`? The current proposal maps a `MachineSet` to an architecture, so this should not be a concern, but curious overall
- The user could have possibly modified the stub ignition used in first boot with sensitive information. While this sub controller could uptranslate them, this is manipulating user data in a certain way which the customer may not be comfortable with. Are we ok with this?
- What platforms do we want to support in GA? GCP was used in the PoC so I've added that, but is there an interest for certain platforms over others for the first release?

### Test Plan

In addition to unit tests, the enhancement will also ship with e2e tests, outlined [here](https://issues.redhat.com/browse/MCO-774).

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Support for GCP
- Unit & E2E tests
- Feedback from openshift teams
- [Good CI signal from autoscaling nodes](https://github.com/cgwalters/enhancements/blob/5505d7db7d69ffa1ee838be972c70b572d882891/enhancements/bootimages.md#test-plan) 


#### Tech Preview -> GA

- Feedback from interested customers
- UPI documentation based on IPI workflow for select platforms(vpshere + any others TBD)
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

In future releases, we can phase in support for remaining platforms as we gain confidence in the functionality. Priorty list for this is still TBD.

#### Removing a deprecated feature

This does not remove an existing feature.

### Upgrade / Downgrade Strategy

__Upgrade__

This mechanism is only active shortly after an upgrade, which is when the ConfigMap containing the bootimages are updated by the CVO manifest. It will also run during machineset edits but patching will only occur if there is a mismatch in bootimages.

__Downgrade__

- If the cluster is downgrading to a version that supports this feature, the boot images will track the downgraded version.
- If the cluster is downgrading to a version that does not support this feature, the boot images will not track to the downgraded version. So, it may be wise to opt-out of the feature prior to the downgrade if "normal(i.e. older) OCP behavior" is expected. 

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

TBD, based on how the opt-in feature would work.

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

TBD

## Alternatives

TBD
