---
title: Ignition Spec 2/3 dual support
authors:
  - "@yuqi-zhang"
reviewers:
  - "@ashcrow"
  - "@cgwalters"
  - "@crawford"
  - "@LorbusChris"
  - "@miabbott"
  - "@mrguitar"
  - "@runcom"
  - "@vrutkovs"
approvers:
creation-date: 2019-11-04
last-updated: 2019-11-19
status: **provisional**|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also: https://github.com/openshift/enhancements/pull/78
replaces:
superseded-by:
---

# Ignition Spec 2/3 dual support


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]


## Summary

This enhancement proposal aims to add dual Ignition specification version 2/3
(Ignition version 0/2) support to OpenShift 4.x, which currently only support
Ignition version 0 spec 2 for OS provisioning and machine config updates. We
aim to introduce a  method to switch all new and existing clusters to Ignition
spec version 3 at some version of the cluster, which will be performed by the
Machine-Config-Operator (Henceforth MCO). The switching will be non-breaking
for clusters that have no un-translatable configs (see below), and will have
a grace period for admins to intervene and transition otherwise. The OpenShift
installer and underlying control plane/bootstrap configuration, as well as RHEL
CoreOS (Henceforth RHCOS) package version will also be updated.

This overall migration will take part as a two-phase process:

Phase 1/OCP 4.4:
The objective of this phase is to allow users to apply machineconfigs with
Ignition spec 3 support to new and existing clusters. The breakdown is:

- a tool is created to translate between Ignition spec versions
- MCD gains the ability to process both spec 2 and spec 3 configs
- MCC gains the ability to translate from spec 3 configs to spec 2 configs

Some other work that would be nice to have in phase 1 to prepare for phase 2,
but not necessary to acheive the objective:

- MCC attempts to translate all machineconfigs with v2 Ignition config to v3
- MCC learns to generate v3 initial master/worker Ignition configs


Phase 2/OCP 4.X:
All new installs will be on spec 3 only. Existing clusters that have non spec 3
Ignition config machineconfigs will not be allowed to update to the new version.

- RHCOS bootimages switches to only accept Ignition spec 3 configs
- The OpenShift installer is updated to generate spec 3 configs
- Remaining MC* components generate spec 3 only
- MCO enforces that all configs are spec 3 before allowing the CVO to start the update


## Motivation

Ignition released v2.0.0 (stable) on Jun 3rd, 2019, which has an updated
specification format (Ignition spec version 3, henceforth “Spec 3”). This
change includes some important reworks for RHCOS provisioning, most
importantly the ability to specify and mount other filesystems and fixing
issues where Ignition v0 spec was not declarative. Particularly, this is
required to support having /var on a separate partition or disk, an important
requirement for security/compliance purposes in OCP. The existing version on
RHCOS systems (Ignition version v0.33) carries a spec version (spec version 2,
henceforth “Spec 2”) that is not compatible with Spec 3. Thus we would like to
update the Ignition version on RHCOS/Installer/MCO to make use of the changes.

This proposal will also allow closer alignment with OKD, as OKD will be based
on Fedora CoreOS (Henceforth FCoS), which is already on, and only supports,
Ignition spec 3. We want to do this in a way that can minimize deltas between
OKD and OCP.


### Goals

#### Phase 1
- [ ] A config translator is created to translate from Ignition spec 2 to spec 3
- [ ] A config translator is created to translate from Ignition spec 3 to spec 2
- [ ] The MCO is updated to support both Ignition spec 2 and 3, with:
  - [ ] The MCC accepting spec 3 configs
  - [ ] MCC rendering spec 3 configs to spec 2
- [ ] Create tests to apply spec 3 configs to spec 2 clusters successfully
- [ ] An alerting mechanism is put in place for outdated and incompatible/non-translatable configs
- [ ] Docs are updated to reflect the new config version

#### Phase 1 (optional)
- [ ] MCC attempts to translate all machineconfigs with v2 Ignition config to v3, leaving those which cannot be converted and warning the user of future changes
- [ ] MCC learns to generate v3 initial master/worker Ignition configs

#### Phase 2
- [ ] The MCO gains the ability to manage installer-generated stub master/worker Ignition configs (separate enhancement proposal)
- [ ] Ignition-dracut spec 2 and spec 3 diffs are aligned
- [ ] RHCOS bootimages switches to only accept Ignition spec 3 configs
- [ ] The OpenShift installer is updated to generate spec 3 configs
- [ ] The MCC gains the ability for users to provide necessary changes to update spec 2 to spec 3
- [ ] MCO enforces that all configs are spec 3 before allowing the CVO to start the update
- [ ] Further tests/docs are added


### Non-Goals

- Support FCoS/OKD directly through this change
- API support for MCO, namely switching to RawExt formatted machineconfig
  objects instead of explicitly referencing Ignition, is not considered as
  part of this proposal


## Proposal

This change is multi-component:


### Vendoring Changes

The MCO and Installer must change to go modules (currently dep) for vendoring
as Ignition v2 requires using go modules. To support both spec 2 and spec 3,
both Ignition versions must be vendored in for typing.


### Spec Translator

To handle both spec versions, as we need the ability to upgrade existing
clusters, we will create a translator between spec 2 and spec 3. This ensures
that a cluster only has one “desiredConfig” which will be translated to spec 3
when the MCO with dual support detects that the existing configuration of a
machine is on spec 2 (will happen only once for all existing and new nodes,
when the MCO with dual support is first deployed onto the cluster). This will
only be required as part of the MCO.

Note that there exists three types of spec 2 configs:

- Those that are directly translatable to spec 3. This is the case for all existing IPI configs.
- Those that we cannot translate with certainty, and requires user input for us to correctly translate

During phase 1 we should attempt to translate the existing config to detect
untranslatable configs. If the cluster can be directly translated to spec 3,
no action will be required. If not, we will need to alert the user that there
are untranslatable configs.

During phase two, we should fail updates unless the cluster is fully on spec
3 config. This effectively means that UPI clusters are at risk when upgrading
to an MCO with dual support. Mitigation methods are discussed below.

For backwards support (spec 3 to spec 2) the configs are 100% translatable.


### RHCOS

Phase 1:

No changes needed.

Phase 2:

The RHCOS bootimage needs to be updated to Ignition package v2.0+ . Required
dependencies are discussed below.


### Installer

Phase 1:

No changes needed.

Phase 2:

The bootstrap Ignition configs are updated to be spec 3. The stub Ignition
configs for master/worker nodes are updated to spec 3, and referece the correct
endpoint of the MCS which will serve Ignition spec 3 configs. RHCOS images pinned
by the installer will be updated in conjuction to ones with Ignition v2 (spec3).
All spec 2 references are stripped from the installer.


### MCO

The MCO and its subcomponents are the most affected by this change. The
aforementioned spec translator will be housed in the MCO. This means that the
MCO would need to simultaneously vendor both Ignition v0 and v2, and translate
existing between the spec versions as needed.

Phase 1:

The MCD has the capability to understand both spec 2 and spec 3 configs,
and lay down files as needed.

The MCC has the capability to translate spec 2 to spec 3 configs. If the
translation is completely successful, it will create final rendered spec 3
configs, and the MCD will be instructed to use the spec 3 configs to lay down
necessary files. Otherwise the MCD will continue using the existing
spec 2 configs, and apply new spec 2/spec 3 configs.

The MCS served configs generated by the MCC will be downtranslated by the
MCC to spec 2. The MCS is not updated to serve spec 3 configs yet.

Phase 2:

All machineconfigs are switched to spec 3. Spec 2 support will exist in the form
of served Ignition configs to new nodes joining the cluster that have Ignition
v0 in the bootimage. MCS will handle this with 2 endpoints.

The MCO will flat out reject spec 2 configs, and refuse to upgrade clusters
that have spec 2 bits.

The MCO will also validate upon seeing a spec 2 machineconfig applied to
it, after it has transitioned to phase 2 (pure spec 3), and rejects that
machineconfig.

The MCO should also add ability to reconcile broken spec 2 -> spec 3 updates,
after manual intervention from the user.


### User Stories (Post Phase 1)

**As the admin of an existing 4.x cluster on spec 2, I’d like to apply a spec 3 machineconfig once MCO has spec 3 support**

Acceptance criteria:

- The config is validated and applied successfully
- The user can see the new spec 3 config, and what it rendered down to


### User Stories (Post Phase 2)

**As the admin of an existing 4.x cluster on spec 2, I’d like to upgrade to the newest version and use Ignition spec 3**

Acceptance criteria:

- The update completes without user intervention, if all machine configs existing on the cluster can be directly translated to spec 3
- The user receives an alert, if the update is unable to complete due to untranslatable configs
- The user is able to recover the cluster and finish the update if the untranslatable configs are manually translated or remove, or roll back to the old version
- The user should have received notification that the update will be changing spec version, as well as received necessary documentation on how to recover a failed update
- CI tests are put in place to make sure the existing versions can be updated to the new payload

**As a user of openshift, I’d like to install from a spec 2 bootimage and immediately update to a spec 3 payload**

Acceptance criteria:

- Essentially the same as story 1

**As a user of OpenShift, I’d like to install a fresh Ignition spec 3 cluster**

Acceptance criteria:

- The workflow remains the same for an IPI cluster
- The workflow remains the same for a UPI cluster, minus custom specification changes
- The user should have good documentation, based on version, of how to set up user defined configs during install time

**As an admin of an existing spec 3 cluster, I’d like to apply a new machineconfig**

Acceptance criteria:

- The machineconfig is applied successfully, if the user has defined a correct spec 3 Ignition snippet
- The user is properly alerted if they attempt to apply a spec 2 config, and the machineconfig fails to apply
- The user is given necessary docs to remove the undesired spec 2 config and to translate it to a spec 3 config

**As an admin of an existing spec 3 cluster, I’d like to autoscale a new node**

Acceptance criteria:

- The bootimage boots correctly and pivots to align correctly to the rest of the cluster version


### Risks and Mitigations

**Failing to update a cluster**

The IPI configuration is fully translatable. UPI as well as user provided
configuration as day 2 operations are not workflows we can guarantee. For
some users they will simply fail to update the cluster to a new version. To
mitigate, we must allow the user to recover and/or reconcile, or at the bare
minimum have comprehensive documentation on what to do in this situation

**Failing to apply a spec 2 machineconfig that worked prior to the final update**

Users will likely be unhappy that there is such a large breaking change. In
other similar cases, e.g. for auto-deployed metal clusters, the served Ignition
configs must all be updated after a certain point of the bootimage to be able
to bring up new machines. To mitigate we should communicate this change well in
advance, and provide methods to translate Ignition configs. Failed
installation/alerting systems must clearly communicate the source of error in
this case, as well as how to mitigate.


## Design Details

The implemented changes for the various components can be separate, with the
caveat that Ignition spec 3 support for MCO must happen first (so that other
component changes can be tested in cluster). The MCO changes can be standalone,
as they serve to bring a spec 2 cluster to spec 3, or work as is on a spec 3
cluster.

**RHCOS details:**

RHCOS must change to use Ignition v2, which supports spec 3 configs. The actual
switching of the package is very easy on RHCOS. The building of Ignition v2,
however, presents two issues:

- The util-linux package is old on RHEL, without support of “lsblk -o PTUUID” which Ignition uses. This will have to be reworked in RHCOS, or the package must be bumped and rebuilt for rhel 8.1 or workaround as in https://github.com/coreos/Ignition-dracut/pull/133
- Ignition-dracut has seen significant deltas between the spec2x and master (spec 3) branches, especially for initramfs network management. There are also minor details such as targets that need to be checked for existing units. There exists a need to merge some spec2x bits back into 3x before RHCOS can move to 3x

This change will be phase 2 only.

**Installer details:**

The installer would only need to support either Ignition spec 2 or spec 3.
The installer today is responsible for generating:

- bootstrap Ignition config
- initial stub master config
- initial stub worker config

Most of the actual configs for master/workers are generated in the MCO via
templates, and served by the MCS. Thus the installer would need to, during
phase 2, generate spec 3 configs. For existing clusters, there exists a need
to update the stub master/worker configs, which today exists as secrets
unmanaged by any component in OpenShift. See
"Managing stub master/worker Ignition configs" section below.

At the time of writing this proposal, there exist FCOS/OKD branches for the
installer that are looking to move to spec 3, and has had success in installing
a cluster. This work can be integrated for OCP as well. The main issue remaining
is due to the necessity of moving to go modules as the vendoring method, there
as are failures in the Azure Terraform provider that seem to be incompatible
with this change.

**Managing stub master/worker Ignition configs**

Note: This will be also be a separate enhancement proposal.

Today in OpenShift, the installer generates stub Ignition configs for master
and worker nodes. These stub configs serve as initial configs given to
RHCOS bootimages for master/worker. They function to tell Ignition that actual
Ignition configs will be served at port 22623, under /config/master or
/config/worker, for Ignition to fetch during its run.

The Ignition stub config is then saved as `master-user-data` and `worker-user-data`
secrets in OpenShift. These stub configs are defined in a MachineSet, e.g.

```yaml
userDataSecret:
  name: worker-user-data
```

Which the MAO can interpret to fetch for the machine, when provisioning new
machines.

The issue today is that after installer creates these secrets, they are
effectively "forgotten". No componenent in OpenShift manages these secrets.
The only way to update these secrets would be if a user knows the name, and
manually changes it to another valid Ignition config.

When Ignition spec 3 bootimages come into the picture, there currently exists
no method to create a new MachineSet to referece new Ignition configs to serve
to these machines. A component of the OpenShift system (likely the MCO) thus
needs to create new secrets/update existing secrets to point to new stub
configs with spec 3. The MCS can then serve these spec 3 configs at different
directories at the same port, and it will be up to correctly defined
MachineSets to point it there.

**MCO details:**

A spec translator will first be implemented in the MCO, with the ability to
detect untranslatable configs. The MCO then should be updated to have support
for both Ignition V0S2 and V2S3. Since the MCO is responsible for the current
cluster nodes, it will be the only place at which spec translation is done.
The translation will happen when the version of MCO with dual support and
translator is first deployed; it will detect the existing config being spec 2,
generate a new renderedconfig based on a translator from spec2 to spec 3. If
this translation is successful, it will instruct the MCD that the spec 3 config
is now the complete renderedconfig of the system. Future spec 2 configs applied
to the system will undergo the same translation. After phase 2 happens, spec 2
configs detected will be rejected and an error thrown.

If the translation fails, the MCO will throw an alert to the admin that the
cluster machineconfig will soon be switching to spec 3, and there are existing
configs that are not translatable. If the admin takes no action, eventually the
cluster will fail to upgrade. In phase 2, the admin will see a failed update
with reason as "Unupgradeable".

The spec translator will also translate the existing bootimage configs served
to new nodes joining the cluster. The MCS will serve both configs at different
endpoints, and does not otherwise care which config needs to be served. Failed
spec 2 to spec 3 translations will also be handled as above, with warning
that at some point the cluster will refuse to upgrade.

The MCO should also add functionality to more easily reconcile broken
machineconfigs and Ignition specs being served, thus allowing a cluster admin
to correctly recover/abort a failed update to spec 3.


**Other notes:**

Spec 2 -> Spec 3 translation has not been fully implemented anywhere before.
There could be many edge cases we have not yet considered. There are other
potential difficulties such as serving the correct Ignition config. See above
section on risks and mitigations.

Starting from some version of OpenShift, likely v4.6, we can remove dual
support and be fully Ignition spec 3.

Kubernetes 1.16 onwards has support for CRD versioning:
https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definition-versioning/.
If we opt to delay this that is potentially an alternative method of
implementation.


### Test Plan

Extensive testing of all possible paths, especially those outlined in the
user stories, is critical to the success of this major update. The existing
CI infrastructure is a good start for upgrade testing. There should be
additional tests added, especially in the MCO repo, for edge cases as
described in the user stories, to ensure we never break this behaviour.
Many existing tests will also have to be updated given the spec change.


### Upgrade / Downgrade Strategy

The spec translation will happen as part of an upgrade, when the new MCO
is deployed. See above discussions on alerts during upgrade. For clusters
that are already on spec 3, future upgrades will proceed as usual, much
like what we have in spec 2.


### Graduation Criteria

This is a high risk change. Success of this change will require extensive
testing during upgrades. UPI clusters are especially at risk since there
are potentially situations we cannot reconcile with spec translations.
Some of the exact details need further fleshing out during implementation,
and potentially will be not feasible. Existing user workflow will be
disrupted, so communication of these changes will also be very important.


## Infrastructure Needed

None extra
