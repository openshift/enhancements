---
title: machine-config-support-in-nto
authors:
  - "@jmencak"
reviewers:
  - "@MarSik"
  - "@sjug"
  - "@slintes"
  - "@yanirq"
  - "@zvonkok"
approvers:
  - "@MarSik"
  - "@sjug"
  - "@slintes"
  - "@yanirq"
  - "@zvonkok"

creation-date: 2020-03-26
last-updated: 2020-09-01
status: implemented
---

# Machine Config Support in NTO

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes adding the ability for admins to target MachineConfigPools
and create MachineConfigs through the Node Tuning Operator.  With this enhancement we
can take another step towards full support for tuned profiles that need `[bootloader]`
support on RHCOS.

## Motivation

In support of Telco 5G customers, partial tuned real-time profile support
already landed OpenShift.  However, a lot of node-management tasks still need to be
performed by the OCP administrator or an addon operator, so the current profile
delivery is not fully automated and performed from a centralized location.

Specialized profiles, such as cpu-partitioning or realtime exist on RHEL which
deliver specialized tuning.  These profiles perform calculations based on a
variable input and, among other things, set proper kernel boot parameters
to achieve requested tuning.

These tuned profiles are written with RHEL rather than RHCOS in mind, therefore
host modifications such as kernel boot parameters becomes problematic on RHCOS.
Support for MachineConfigPools/MachineConfigs in the Node Tuning Operator can 
overcome these issues on RHCOS.

### Goals

- [ ] The ability to target MachineConfigPools from the 
  [Tuned API](https://github.com/openshift/cluster-node-tuning-operator/blob/master/pkg/apis/tuned/v1/tuned_types.go)
  the appropriate MachineConfigs based on the calculated kernel boot parameters.
- [ ] The ability to roll-back the previous kernel settings on the removal of 
  rules that target a MachineConfigPool.
- [ ] Minimize the reboots of the cluster nodes to the necessary minimum.  The
  necessary minimum depends on the underlying Machine Config Operator handling
  of new MachineConfigs and should be a single reboot.
- [ ] In clusters installed using the 4.5 installer or higher, this new 
  functionality must not cause extra node reboots on the Node Tuning Operator
  upgrades.

### Non-Goals

- Does not support targetting MachineConfigPools on non-RHCOS machines.
- Does not protect the admins from the issues that might results using this 
  functionality when grouping machines of different hardware specifications 
  into the same MachineConfigPool.

## Proposal

### Risks and Mitigations

- Risk: Grouping nodes of different hardware specifications into the same
  MachineConfigPool and having tuned operands calculate different kernel
  parameters for two or more nodes.
  Mitigation: user documentation.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Testing should be thoroughly done at all levels, including unit, end-to-end, and
integration. The tests should be straightforward - setting kernel args in an
install or machineconfig, checking that they were applied correctly.

More specific testing requirements have been outlined above in the acceptance
criteria for the user stories.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Version Skew Strategy

Version skew should not have an impact on this feature.  The kernelArguments feature
is supported in MachineConfigs since OCP 4.3.

## Implementation History

See: https://github.com/openshift/cluster-node-tuning-operator/pull/119
