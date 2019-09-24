---
title: Kernel Arguments - Day 1 Support
authors:
  - "@ericavonb"
reviewers:
  - "@cgwalters"
  - "@crawford"
  - "@imcleod"
  - "@runcom"
approvers:
  - "@cgwalters"
  - "@crawford"
  - "@imcleod"
  - "@runcom"

creation-date: 2019-09-12
last-updated: 2019-09-23
status: **provisional**|implementable|implemented|deferred|rejected|withdrawn|replaced
---

# Kernel Args - Day 1 Support

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This enhancement proposes adding the ability for admins to configure custom
kernel arguments during initial RHCOS node configuration. With this enhancement,
both OS upgrade to the latest [machine-os-content](https://github.com/openshift/machine-config-operator/blob/master/docs/OSUpgrades.md#os-updates)
and custom kernel args can be applied to a node during the first boot of the
machine, followed by a single reboot.

## Motivation

Kernel arguments for RHCOS can be set today using a MachineConfig as a
[`day-2` operation](https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfiguration.md#kernelarguments),
i.e. after cluster install, incurring an additional reboot to apply. Even if the
kernel args are configured at [installation time](https://github.com/openshift/installer/blob/master/docs/user/customization.md#install-time-customization-for-machine-configuration),
the node will still incur a double reboot to apply the args. Furthermore, that
reboot will not be coordinated by the [MachineConfigController](https://github.com/openshift/machine-config-operator/blob/master/docs/MachineConfigController.md#updatecontroller),
resulting in possible cluster downtime.

We can provide a better experience by allowing kernel arguments to be set as a
`day-1` operation, when a node is being initialized with the current OS and
cluster version.


### Goals

- [ ] Cluster admins can define custom kernel arguments for RHCOS nodes during
  cluster installation, without rebooting the machine twice.
- [ ] kernel args
  can be configured via MachineConfigs and will be applied to new nodes joining
  the cluster, without inclurring an extra reboot.
- [ ] In clusters installed using the 4.3 installer or higher, nodes with kernel
  arguments configured via MachineConfigs have these kernel args applied with
  the update without incurring an extra reboot.

### Non-Goals

- Does not support defining custom kernel arguments on non-RHCOS machines.
- Does not support applying kernel args on first boot for nodes in clusters
  installed using a 4.1 or earlier installer.
- Does not protect against setting kernel arguments that may affect the
  performance of the node or cluster.
- Does not take into account kernel args set manually on the node when applying
  kernel args from the MachineConfigs. Any kernel args manually set or unset
  will be overwritten on the next update.

## Proposal

#### Part 1
Add kernel args to install-config and process them in the MCO bootstrap. This
should be a straight-forward translation from the install-config into
MachineConfig resources.

#### Part 2
For the nuts and bolts, there are a couple ways we could achieve no-extra-reboot
kernel args support.

##### Method 1 - append to `etc/pivot/kernel-args`

- Update the MachineConfigServer to parse kernel args in the MachineConfigs and
  append them in the `/etc/pivot/kernel-args` file within the ignition it
  serves, similar to how hyperthreading can be disabled in 4.2.
- Update the MachineConfigDaemon to compare k-args set on MachineConfigs with
  those set via rpm-ostree to determine if there are changes requiring a reboot.

Pros:
- Requires no additional packaging in RHCOS or other tricky methods to update
  what's available on the host.
- Allows new nodes in clusters with 4.2 bootimages to have k-args set without
  the additional reboot.
- Brings the MCD closer to the kubernetes model of inspecting current state and
  reconciling desired spec with that.

Cons:
- Further special-cases kernel args as state passed to pivot. Any other config
  that should be set before os pivot would need to be specially handled as well.
- Determining the current state from the host rather than the current config
  could get tricky, and is different from how other config fields are handled.
  See https://github.com/openshift/machine-config-operator/pull/245


##### Method 2 - MCD-as-pivot packaged in RHCOS

- Installer in 4.2 ships machine-config-daemon package in RHCOS host which handles [early pivot](https://github.com/openshift/machine-config-operator/pull/859).
- [MCS inverts MachineConfig](https://github.com/openshift/machine-config-operator/pull/868) containing non-ignition part (like osImageURL,kernelArguments) during first boot in /etc/ignition-machine-config-encapsulated.json on hosts
- During the first boot, MCD is patched to parse the ignition-machine-config-encapsulated.json available on the host to read and set the k-args if specified.
- machine-config-dameon package in RHCOS is updated so that it has required changes on the host during early boot.
- MCS is updated so that MCD first boot service runs to process kargs
- As a result, we get kargs applied on RHCOS nodes during first boot without incurring extra reboot.

Pros:
- Moves us to a more flexible model where the MCD can be the component managing
  the host.
- Removes any drift between how k-args are handled on day-1 or day-2 by using
  the same code paths.

Cons:
- Requires latest MCD package with necessary fixes in RHCOS shipped with installer, any further changes needs [updating bootimage](https://github.com/openshift/os/issues/381) which is currently not in place.
- Will only work on clusters with bootimages at 4.3 or higher, i.e. clusters
  installed at 4.1 or 4.2 and upgraded will not have k-args applied without a
  double reboot on new nodes spun up.
- Running the MCD as a binary removes all the advantages we get from containers.


##### Method 3 - MCD-as-pivot fed in through ignition

- Same as method 2 except rather than packaging the MCD in RHCOS, feed it to the
  hosts via their ignition file.
- The MCS would need to extract the MCD through the os-container place it in the
  ignition files it serves.

Pros:
- Same as method 2
- Works for all versions of bootimages, including 4.1 and 4.2

Cons:
- Significantly more complexity in the MCS
- Additional work to fit into a tight release schedule



4. Package the MachineConfigDaemon in RHCOSRun the MCD during boot where `pivot` runs today to process kernel args along
   with the pivot to the latest machine-os-content.

#### Recommendation

We believe starting with method 2 would be the best idea for 4.3 given the
limited timeline. And since method 3 builds on top of method 2, we move in the
right direction and can continue onto method 3 in further releases.


### User Stories

#### Story 1

Kernel arguments can be set in my install config and applied to nodes as part of
the cluster installation.

Acceptance Criteria
- CI tests ensure kernel args can be configured at install without ill effects
  on other components
- CI tests ensure kernel args configured at install incur only one reboot
- CI tests ensure when no k-args are set via the installer, the MCD should do
  nothing.
- The installer validates user provided configs and rejects improperly formatted
  ones.
- The MCD validates user configured k-args in MCs for correctness and safety in
  a best-effort manner.
  - openapi schema on machineconfig CRD
  - custom admission validation if necessary
  - input validation during processing in controllers
- The installer reports failures related to setting the kernel args to the user.
- MCC takes k-args status into account when determining the node health and
  reboot status.
- A user can understand the status of k-arg configs via cluster resource objects
  (nodes, MCs, MCPs).
- MCD reports the status of k-args on its host to the node, MC, and/or other
  related objects.
- A user can reasonably debug k-arg configuration via installer logs and
  artifacts, MC* logs, metrics, and events
- kernel arg configuration is limited to cluster administrators and properly
  secured.
- Documentation outlines kernel arg configuration during install and guides
  users through how to properly use install config and MachineConfig objects to
  configure them.
- Kernel arg configuration by the MCD during install works the same on all
  supported platforms.
- Other teams within Red Hat affected by these changes are informed and
  communicated with through the implementation and release. These may include
  but are not limited to: the Installer team, node-tuning-operator teams, RHCOS
  teams, docs, and ART.


#### Story 2

Nodes with kernel arguments configured via MachineConfigs can be updated during
cluster upgrades and have those kernel args applied without incurring an extra
reboot on clusters installed at 4.3 or higher.

Acceptance Criteria

- Cluster upgrades from 4.3.x to 4.3.x+1 transition nodes to the MCD-as-pivot without
cluster distruption.
- Node updates are coordinated correctly by the MCO, taking kernel argument
  application or failures into account for node health.
- Cluster upgrades with nodes running pre-4.3 RHCOS have kernel arguments
  applied after they are upgraded to a more recent OS version with kernel args
- Upgrades call be rolled back on failure (?)

Similar to previous story:
- CI tests ensure configured kernel args are applied to upgraded nodes during a
  cluster upgrade, without ill effects on other components
- Configured kernel args are applied to upgraded nodes during a cluster upgrade
  without incurring any extra reboots
- MCD reports the status of k-args on the host to the node and/or MC objects
- MCC takes k-args status into account when determining the node health and
  reboot status
- A user can understand the status of k-arg configs via cluster resource objects
  (nodes, MCs, MCPs)
- A user can reasonably debug k-arg configuration via MC* logs, metrics, and events
- k-arg configuration is limited to cluster administrators and properly secured
- No k-arg configuration is done by the MCD on non-RHCOS and non-managed RHCOS
  nodes.
- Documentation outlines the changes to how k-args are applied and guides users
  through how to properly use MCs to configure them and check that k-args
  previously configured are being applied correctly
- k-arg configuration by the MCD works the same on all supported platforms

#### Story 3

New nodes added to clusters first installed at 4.3 or higher can be configured
with kernel args via MachineConfigs without incurring an extra reboot.

Acceptance Criteria
- CI tests ensure kernel args can be configured on new nodes added to a cluster
  without ill effects on other components.
- CI tests ensure kernel args configured on new nodes incur no extra reboot.
- CI tests ensure when no k-args are set, the MCD should do nothing on new nodes.
- Validation on user provided configs reject improperly formatted configs
- User configured k-args in MCs are validated for correctness and safety in a
  best-effort manner.
- MCD reports the status of k-args on the host to the node and/or MC objects
- MCC takes k-args status into account when determining the node health and
  reboot status.
- A user can understand the status of kernel arg configs via cluster resource
  objects (nodes, MCs, MCPs).
- A user can reasonably debug k-arg configuration via MC* logs, metrics, and
  events.
- Kernel arg configuration is limited to cluster administrators and properly
  secured.
- No kernel arg configuration is done by the MCD on non-RHCOS and non-managed
  RHCOS nodes.
- Documentation outlines kernel arg configuration and guides users through how
  to properly use MCs to configure them.
- Kernel arg configuration by the MCD works the same on all supported platforms


### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they releate.

### Risks and Mitigations

1. Risk: non-recoverable clusters due to bad kernel arg configuration
   Mitigation: best-effort validation, careful documentation with warnings. We
   may want to consider restricting customization of kernel args with specific
   values needed by the system (e.g. ostree=..)
2. Risk: Drift between RHCOS and other node types
   Mitigation: check in with other OS's (e.g. FCOS) on how they might support
   a similar feature. TODO check on what this would mean for the
   windows-machine-config-operator.
3. Risk: users with write permissions on MachineConfigs can set sensitive kernel
   args, compromising the security of the system. For example, a user could turn
   off audit logging via a MachineConfig using another user or service account's
   credentials thus evading detection for any further actions.
   Mitigation: strict RBAC on MachineConfig resources. We may wish to consider
   more fine-grained permission scoping by creating a new resource type, similar
   to `kubeletconfigs` and `containerruntimeconfigs`. We can also look into
   restricting users from setting certain security sensitive kernel args.
4. Risk: failures related to k-args may be harder to debug
   Mitigation: careful error reporting, logging, integration with debugging
   tools such as must-gather. TODO think about roll-back capabilities.
5. Risk: overlapping configuration fields may lead to unexpected behavior (e.g.
   FIPS mode, hyperthreading/nosmt)
   Mitigation: validation where both are set, clear and deterministic
   compositions.

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

As the ability to set kernel arguments via MachineConfigs will be added in 4.2,
the API does not change with this proposal. The install-config API would have
the addition of kernel args which can be matured according to standard procedure.

### Upgrade / Downgrade Strategy

Since upgrades happen as a "day-2" operation, they should not be affected by
rolling out these changes. See discussion about bootimage versions affecting the
ability to utilize this feature in the methods discussed above.


### Version Skew Strategy

Version skew should not have an impact on this feature. The change would only
apply to new nodes. If a new node is added during an ugrade (not recommended),
it could get either the old or new version, meaning it may or may not require
an extra reboot to apply the kernel args.

The version skew between the bootimages and the upgraded cluster is the largest
concern since users may be suprised that their clusters still double reboot new
nodes despite being upgraded. Good documentation will be required.


## Implementation History

This feature is a continuation of adding kernel-args configuration to the
machine-config-operator. Work was done in 4.2 to allow kernel-arg configuration
as a "day-2" operator.

See discusion in this issue: https://github.com/openshift/machine-config-operator/issues/798
Former epic: https://jira.coreos.com/browse/PROD-1084
