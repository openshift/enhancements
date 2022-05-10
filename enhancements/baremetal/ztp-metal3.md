---
title: converge-ztp-and-metal3-flows
authors:
  - "@dtantsur"
reviewers:
  - "@flaper87"
approvers:
  - "@hardys"
  - "@zaneb"
creation-date: 2022-03-14
last-updated: 2022-03-24
tracking-link:
  - https://issues.redhat.com/browse/METAL-10
  - https://issues.redhat.com/browse/METAL-192
status: implementable
---

# Converge ZTP and Metal3 flows

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes converging two somewhat different bare-metal
provisioning flows in OpenShift:

- *Standard* Metal3 flow used in bare-metal IPI and [baremetal-operator][bmo]
  (BMO). Uses the [Ironic agent][ipa] (also known as ironic-python-agent or
  simply IPA) for writing the CoreOS image.

- ZTP (Zero-Touch Provisioning) flow using the so-called *live ISO* Ironic and
  baremetal-operator feature, the assisted service and the assisted agent.

## Motivation

Having two different provisioning flows comes with significant maintenance and
development costs:

- Some features, namely firmware and RAID configuration, require using the
  Ironic agent, which is not present in the *live ISO* flow.

- Supporting (i)PXE boot with the *live ISO* flow requires creating new API
  in baremetal-operator specifically for this purpose.

- In the *live ISO* flow, it is not possible to notify Ironic (and thus
  baremetal-operator) when the actual installation finishes. From the
  perspective of Metal3 components, the installation is finished once the ISO
  is connected. As a result, Ironic cannot disconnect the ISO from the
  machine's BMC, nor can it update the BMC's boot device settings properly.

- The *live ISO* flow disables Ironic *inspection* process and has to simulate
  it by updating the `BareMetalHost` based on the assisted service's inspection
  process. This creates an unnessesary coupling between the assisted service
  and the baremetal-operator's hardware inventory format (which, in turn, is
  derived from the Ironic's inventory format).

- The *standard* flow is highly [extensible][hardware managers] via the
  [ironic-agent-image][ironic-agent] container image. This extensibility does
  not apply to the *live ISO* flow since it does not use the Ironic agent.

- Any different path through the `BareMetalHost` state machine makes it
  exponentially more difficult for upstream developers to reason about the
  effects of any changes. In addition, the *live iso* flow is not tested
  upstream by the Metal3 project.

### Goals

- ZTP follows the *standard* Metal3 flow instead of *live ISO*.
- (i)PXE boot, firwmare and RAID settings work for ZTP without introducing
  special API in Metal3 components.
- Continue to allow non-ZTP assisted deployments without depending on Metal3.
- Not depend on assisted components in the core metal platform

### Non-Goals

- Converging two agent implementations (Ironic and assisted) into a single one.
- Supporting per-host network configuration through the BMH
  PreprovisioningNetworkData API in the ZTP case.
- Performing firmware config changes only after the assisted service runs host
  validations.

## Proposal

- Building CoreOS images for the *standard* flow is handled by
  [image-customization-controller][icc] (ICC). ICC will start ignoring all
  `PreprovisioningImage`s that have the `infraenvs.agent-install.openshift.io`
  label (copied from the corresponding `BareMetalHost` by BMO).

- A new controller will be created as part of the assisted service to
  handle `PreprovisioningImage`s that do have the InfraEnv label. Its exact
  implementation will be defined by the assisted team. The assisted agent
  configuration will be updated to not start the agent automatically in the
  presence of the Ironic agent.

- To boot CoreOS via network, three artifacts are required: the kernel, the
  initramfs and the root filesystem. In the current ICC implementation, only
  the initramfs is a dynamically generated artifact, while the kernel and the
  rootfs are taken from the payload on the Metal3 pod start-up and used for
  all nodes. Since ZTP supports installing clusters of a different version and
  even architecture from the hub cluster, we will add separate fields
  to the `PreprovisioningImage`'s `Status`:

  `KernelURL` to store the matching kernel.

  `ExtraKernelParams` to provide a link to the root file system. BMO will be
  updated to pass the parameters to the node.

- [Ironic agent][ironic-agent] currently contains a custom deployment plugin
  (*custom deploy method* in baremetal-operator terms, *custom deploy step* in
  Ironic terms) that invokes `coreos-installer` for the image installation.
  A new custom deploy method `start_assisted_install` will be added to start
  the assisted agent when BMO requests deployment.

- The assisted service (more specifically, BMAC) will stop explicitly disabling
  *inspection* on `BareMetalHost`s it manages, will start using a different
  process based on the new `start_assisted_install` deploy method and stop
  rebooting the machine in the end of the process.

### User Stories

This change itself does not add new user stories, but rather enables existing
features to work for ZTP:

- As an operator, I want to ensure that all OpenShift nodes are deployed with
  known firmware settings.

- As an operator of hardware that does not support virtual media, I want to
  deploy OpenShift nodes using iPXE network boot.

- As an operator, I want OpenShift to ensure that the deployment ISO is
  correctly disconnected on installation finish and cannot be accidentally
  booted into.

### API Extensions

The `PreprovisioningImage`'s `Status` will be updated with two new fields:

- `KernelURL` - the URL of a kernel image to use with (i)PXE boot.

- `ExtraKernelParams` - additional kernel parameters to pass when booting
  using (i)PXE.

### Implementation Details/Notes/Constraints

#### Coordination between agents

The first agent to start will be the Ironic agent since it is responsible
for coordinating the whole flow and (indirectly through Ironic) updating the
`BareMetalHost`'s status. *Inspection* and additional configuration (RAID etc)
will be handled by the Ironic agent alone.

BMAC will be able to stop synchronizing the inventory to `BareMetalHost`
objects since this task will be handled by Ironic and baremetal-operator.

Currently, the provisioning is initiated by BMAC (the assisted component
responsible for interactions with baremetal-operator) by applying the following
change to the `BareMetalHost`'s spec:

```yaml
Image:
  URL: url/of/assisted/agent.iso
  DiskFormat: live-iso
```

After this enhancement, BMAC will instead apply:

```yaml
CustomDeploy:
  Method: start_assisted_install
```

This instructs Ironic to find and execute a *custom deploy step* called
`start_assisted_install`, which will be provided by an OpenShift specific
Ironic agent plugin. This step will use D-Bus to order systemd to start the
assisted agent and watch its status.

If the assisted agent exits successfully, the Ironic agent will report success
to Ironic and baremetal-operator. Otherwise, a failure will be recorded.

The `CustomDeploy` field can be set from the very beginning as well. Then BMO
will conduct inspection, apply firmware settings (if any) and prepare for
the installation immediately.

### Ironic changes

Two small tweaks will be required in Ironic:

- Allow deploy steps without a timeout, so that Ironic does not time out
  the `start_assisted_install` deploy steps.

- Allow accepting additional kernel parameters instead of overriding the
  parameters completely.

### Risks and Mitigations

- More containers will be downloaded on the ramdisk start-up. We'll need to
  account for more RAM and a longer start-up time.

- Users will be responsible for correctly linking `BareMetalHost`s with
  `InfraEnv` resources.

## Design Details

### Open Questions

- The assisted mode of operation is to boot the assisted agent and wait for
  instructions from the service (and indirectly - from the user). During this
  time, the `BareMetalHost` state will be `provisioning`, and timeouts may
  apply. We may need a way to tell Ironic to never time out this operation,
  e.g. via custom timeouts per deploy step.

- Is it possible (although rare) that the assisted agent restarts during its
  normal operation. The Ironic agent may misinterpret that as a successful
  exit. In the future, we may need to develop a more robust means of
  communication between the two agents.

### Test Plan

- Building the new ISO will be covered by the existing ZTP e2e tests.
- A new ZTP job will need to be created for network booting.

### Graduation Criteria

This work will be GA immediately since there is no phasing possible or planned.
Only ZTP will be affected. The new behavior will be fully opt-in from the
assisted service's (more specifically, BMAC) point of view.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

This change does not introduce new resources, so upgrading or downgrading them
is not a concern.

For the new flow to apply, it must be explicitly requested through the
`BareMetalHost` API (by setting `CustomDeploy` instead of `Image`). Otherwise,
the old flow is used.

There are no plans to remove the code that supports the *live ISO* flow from
Ironic, Metal3 or OpenShift.

### Version Skew Strategy

The baremetal-operator will need to be updated before the new flow will be
usable with iPXE. Given that ZTP releases trail OpenShift releases (and are
tied to them), it should not become an issue. As a safeguard, BMAC will check
the version of cluster-baremetal-operator (with CVO) before using the new
procedure.

ZTP may be used to install clusters with an older version of OpenShift. In this
case, the new Ironic agent image must still be used, otherwise the
`start_assisted_agent` step will not be present.

### Operational Aspects of API Extensions

#### Failure Modes

- When image building fails, this failure will be propagated to the
  baremetal-operator's `PreprovisioningImage` resource via conditions. The
  whole provisioning process will be paused until the failure is fixed.

#### Support Procedures

- The new controller logs will be the central place to
  triage issues related to image building. Such issues will be highlighted
  by a failure condition on the `PreprovisioningImage` resource.
- The Ironic ramdisk logs (published via a special container
  `ironic-ramdisk-logs`) will be useful to debug the coordination between
  the two agents.

## Implementation History

- [Ironic agent
  plugin](https://github.com/openshift/ironic-agent-image/pull/40)

## Drawbacks

- The converged flow is more complex than each of the old flows.

## Alternatives

- Keep both flows separate, implement all required features (currently
  firmware (BIOS) settings and iPXE boot, potentially hardware RAID and
  firmware upgrades) twice.

- ICC could use the `InfraEnv` resources to ask the assisted service to build
  images. This would be a layering violation and cause a number of other
  issues, e.g. make the whole Ignition available to and changeable by users.

- Run both Ironic and assisted agents in parallel and implement a more
  sophisticated communication method between them. This would allow the
  assisted agent to register itself and run validations before the provisioning
  is requested. This approach would avoid the timeout issue described above,
  but also requires deeper changes to how BMAC works.

- The assisted agent could imitate the Ironic agent by providing a compatible
  API and making the required internal calls. This will require the assisted
  team to implement an internal API that has limited backward compatibility
  promises. Additionally, it will prevent the ZTP flow from benefiting from
  any future changes to the Ironic agent. Finally, the iPXE case will still
  need solving.

## Infrastructure Needed

None

[bmo]: https://github.com/metal3-io/baremetal-operator
[ipa]: https://docs.openstack.org/ironic-python-agent/latest/
[hardware managers]: https://docs.openstack.org/ironic-python-agent/latest/contributor/hardware_managers.html
[ironic-agent]: https://github.com/openshift/ironic-agent-image
[icc]: https://github.com/openshift/image-customization-controller
