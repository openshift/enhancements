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

- *Standard* Metal3 flow used in bare-metal IPI and [baremetal-operator][bmo].
  Uses the [Ironic agent][ipa] (also known as ironic-python-agent or
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
  [image-customization-controller][icc] (ICC). It will be updated to optionally
  accept additional Ignition configuration via a new annotation
  `baremetal.openshift.io/ignition-override-uri`.

- Additionally, ICC will recognize the label linking `BareMetalHost`s to
  `InfraEnv` resources from the assisted service. When such a label is present,
  ICC will wait for the Ignition override annotation to be provided.
  Otherwise, it will proceed with the image building.

- [Ironic agent][ironic-agent] currently contains a custom deployment plugin
  (*custom deploy method* in baremetal-operator terms, *custom deploy step* in
  Ironic terms) that invokes `coreos-installer` for the image installation.
  A new custom deploy method `start_assisted_install` will be added to start
  the assisted agent at the right moment.

- The assisted service (more specifically, BMAC) will stop explicitly disabling
  *inspection* on `BareMetalHost`s it manages, will start using a different
  process based on the  new `start_assisted_install` deploy method and stop
  rebooting the machine in the end of the process.

- BMAC will also set the `baremetal.openshift.io/ignition-override-uri`
  annotation with the URI of the assisted agent configuration on
  any `PreprovisioningImage` objects linked to `BareMetalHost` objects it
  manages. The assisted agent configuration will be updated to not start
  the agent automatically in the presence of the Ironic agent (can be detected
  e.g. by the presence of the `ironic-agent` systemd service).

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

All required API extensions are already present.

A new annotation will be recognized on `PreprovisioningImage` resources:
`baremetal.openshift.io/ignition-override-uri`.

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

### Minimal ISO

ICC is currently not capable of building a *mimimal ISO*, i.e. a CoreOS ISO
without the root filesystems. The minimal ISO support should be implemented as
part of this enhancement because
- it improves compatibility with BMCs that do not accept large ISOs,
- it reduces the resource footprint on the Ironic side.

#### Notes

- The new annotation with accept a URI instead of a full Ignition configuration
  or a link to a secret because the configuration can be quite large when
  many hosts are present.

- ICC will replicate the graceful wait period logic from the assisted service:
  the image will be built not earlier than 60 seconds from the creation
  of the `PreprovisioningImage` object. This is done for two reasons: to give
  the user enough time to apply all changes that may affect the build and to
  reduce the probability of a race between BMAC and ICC.

- Currently, BMO never deletes `PreprovisioningImage` resources. Since all
  Ignition configurations are stored in memory, it may cause a significant
  memory usage in the ZTP case. We will need to fix BMO to delete
  `PreprovisioningImage` resources when the `BareMetalHost` becomes
  `provisioned` or is deleted.

- The same process will be applied to `initrd` images for (i)PXE booting.

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

The image-customization-controller will need to be updated before the new flow
will be usable. Given that ZTP releases trail OpenShift releases (and are tied
to them), it should not become an issue. As a safeguard, BMAC will check
the version of cluster-baremetal-operator (with CVO) before using the new
procedure.

The version of CoreOS will always be the one shipped with the ZTP *hub*
cluster, not necessarily the same version we expect the *spoke* cluster to run
on. We think this is fine because MCO on the spoke will replace the ostree
before starting the host as a node anyway.

### Operational Aspects of API Extensions

#### Failure Modes

- When image building fails, this failure will be propagated to the
  baremetal-operator's `PreprovisioningImage` resource via conditions. The
  whole provisioning process will be paused until the failure is fixed.

#### Support Procedures

- The `image-customization-controller` logs will be the central place to
  triage issues related to image building. Such issues will be highlighted
  by a failure condition on the `PreprovisioningImage` resource.
- The Ironic ramdisk logs (published via a special container
  `ironic-ramdisk-logs`) will be useful to debug the coordination between
  the two agents.

## Implementation History

- [Updated image building
  process](https://github.com/openshift/image-customization-controller/pull/42)
- [Ironic agent
  plugin](https://github.com/openshift/ironic-agent-image/pull/40)

## Drawbacks

- The converged flow is more complex than each of the old flows.
- The image-customization-controller becomes aware of (although does not
  require) the assisted service.

## Alternatives

- Keep both flows separate, implement all required features (currently
  firmware/BIOS settings and iPXE boot) twice.

- The image-customization-controller could just ignore all CRs with an
  `InfraEnv` link, and a separate controller (probably running in the same
  process as BMAC) could reconcile those ones. A second place to vendor the
  controller code just adds opportunities for it to get out of sync. The
  image-customization-controller would still need to know about the `InfraEnv`
  API anyway so it could ignore hosts with the label.

- ICC could use the `InfraEnv` resources to ask the assisted service to build
  images. This would be a layering violation and cause a number of other
  issues, e.g. make the whole Ignition available to and changeable by users.

- Run both Ironic and assisted agents in parallel and implement a more
  sophisticated communication method between them. This would allow the
  assisted agent to register itself and run validations before the provisioning
  is requested. This approach would avoid the timeout issue described above,
  but also requires deeper changes to how BMAC works.

## Infrastructure Needed

None

[bmo]: https://github.com/metal3-io/baremetal-operator
[ipa]: https://docs.openstack.org/ironic-python-agent/latest/
[hardware managers]: https://docs.openstack.org/ironic-python-agent/latest/contributor/hardware_managers.html
[ironic-agent]: https://github.com/openshift/ironic-agent-image
[icc]: https://github.com/openshift/image-customization-controller
