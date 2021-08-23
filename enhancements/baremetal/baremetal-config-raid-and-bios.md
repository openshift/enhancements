---
title: Config-RAID-and-BIOS-for-Baremetal-IPI-deployments
authors:
  - "@fenggw-fnst"
  - "@Hellcatlk"
  - "@hs0210"
  - "@zhouhao3"
  - "@zhu1fei"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-05-18
last-updated: 2021-05-25
status: provisional
see-also:
  -
replaces:
  -
superseded-by:
  -
---

# Config RAID and BIOS for Baremetal IPI deployments

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes to add the support of RAID and BIOS configuration for baremetal IPI deployments.

## Motivation

With the new features of [RAID configuration](https://github.com/metal3-io/baremetal-operator/pull/292) and [BIOS configuration](https://github.com/metal3-io/baremetal-operator/pull/302), openshift already has the ability to configure RAID and BIOS for baremetal.

However, these features only work with a running cluster, users cannot configure desired RAID and BIOS during IPI deployments.

### Goals

- Allow users to manage the RAID and BIOS configuration for control plane hosts during installation.
- IPI deployments can handle the configuration of RAID and BIOS.

### Non-Goals

- Add new or vendor specific options regarding RAID and BIOS.

## Proposal

The configuration of RAID and BIOS is finally handed over to Ironic during the IPI deployments.
1. The user runs `create manifests` and then adds RAID and BIOS fields to the **BMH** created by the installer.
2. The interface to terraform in the installer code parses the BIOS and RAID fields.
3. [Terraform-provider-ironic](https://github.com/openshift-metal3/terraform-provider-ironic) gets the
configuration of RAID and BIOS for control plane hosts and then passes it to Ironic([Baremetal Operator(BMO)](https://github.com/metal3-io/baremetal-operator)
has supported the process of RAID and BIOS for worker nodes).

### User Stories

With the addition of this feature, the users can configure RAID and BIOS in IPI deployments.

### Implementation Details/Notes/Constraints

#### Add two fields

The user runs `create manifests` and then adds RAID and BIOS fields to the **BMH** created by the installer.

```yaml
spec:
  bmc:
    address: <bmc>://<ip-address>
    credentialsName: worker-0-bmc-secret
  bootMACAddress: mac-address
  raid: raid-config
  firmware: bios-config
  ...
```

RAID feature has been implemented in **BMO**, so the *raid* field here is the same as the [BMH](https://github.com/metal3-io/baremetal-operator/blob/399f5ef7ee3831014c1425250bc4fa49641a8709/config/crd/bases/metal3.io_baremetalhosts.yaml).
The *firmware* field is the same as the *spec.firmware* field in **BMH** which is been advancing by [#302](https://github.com/metal3-io/baremetal-operator/pull/302).

#### Process the fields in installer

For control plane hosts, transform the *raid* and *firmware* fields into json respectively and then write them into the
***terraform.baremetal.auto.tfvars.json*** file as the values of *raid_config* and *bios_settings* respectively.

#### Process the fields in terraform-provider-ironic

Add *raid_config* and *bios_settings* fields to terraform-provider-ironic API, transform them back to struct format, and then call the
methods in **BMO** to process the two fields to perform [manual cleaning](https://docs.openstack.org/ironic/latest/admin/cleaning.html#manual-cleaning).

#### Notes

When the IPI deployments completed, the **BMH** resources will still get persisted in the cluster, though control plane hosts have
RAID and BIOS field in their **BMH**, but the *externallyProvisioned* field's value is true, so the RAID and BIOS configurations are
invalid.

### Risks and Mitigations

TBD

## Design Details

### Test Plan

- Unit tests for determining the configuration of RAID and BIOS passed to Ironic meeting expectations.
- e2e tests for determining the configuration of RAID and BIOS configured during the IPI deployments.

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

TBD

### Upgrade / Downgrade Strategy

Older versions of the installer will ignore the *raid* and *firmware* fields.

### Version Skew Strategy

TBD

## Implementation History

- [Implementation on openshift/installer](https://github.com/hs0210/installer/tree/ipi-support-raid-bios)
- [Implementation on openshift-metal3/terraform-provider-ironic](https://github.com/hs0210/terraform-provider-ironic/tree/ipi-support-raid-bios)

## Drawbacks

This will increase the number of steps in IPI deployments and take longer.

## Alternatives

Add RAID and BIOS configuration by modifying the manifest files **~/clusterconfigs/openshift/99_openshift-cluster-api_hosts- \*.yaml** generated by executing `openshift-baremetal-install --dir ~/clusterconfigs create manifest`.

This approach takes advantage of already supported features, so it does not need to modify any source code, but it only works for worker nodes.
