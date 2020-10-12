---
title: Machines user-data managed by the MCO
authors:
  - "@runcom"
reviewers:
  - "@yuqi-zhang"
  - "@cgwalters"
  - "@enxebre"
  - "@JoelSpeed"
approvers:
  - "@yuqi-zhang"
  - "@cgwalters"
  - "@enxebre"
  - "@JoelSpeed"
creation-date: 2020-06-09
last-updated: yyyy-mm-dd
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - "/enhancements/this-other-neat-thing.md"  
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Machines user-data managed by the MCO

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements

## Summary

Currently the installer creates [pointer Ignition configs][1] which end up as [user][2] [data][3] secrets [referenced by the installer-generated Machine(Sets)][4].  That makes things like the Ignition v2 (spec 3) transition tricky.
The key point is that the Secret holding the initial stub ignition is completely unmanaged after it's created by the installer.
In order to upgrade to the new Ignition spec, the MCO needs a way to update the Secret to the new Ignition spec.


[1]: https://github.com/openshift/installer/blob/093ca65398fe567bdf63322894496cebbe3d2ade/pkg/asset/ignition/machine/node.go#L30-L36
[2]: https://github.com/openshift/installer/blob/093ca65398fe567bdf63322894496cebbe3d2ade/pkg/asset/machines/master.go#L161-L170
[3]: https://github.com/openshift/installer/blob/093ca65398fe567bdf63322894496cebbe3d2ade/pkg/asset/machines/worker.go#L197-L205
[4]: https://github.com/openshift/installer/blame/093ca65398fe567bdf63322894496cebbe3d2ade/docs/user/aws/install_upi.md#L231-L232

## Motivation

The goal for this enhancement is to have the Secret that contains the stub ignition configuration managed by the MCO. The MCO will later be able to update the stub to a newer version of the spec during an upgrade.

### Goals

MCO will adopt the pointer ignition created by the installer and manage it's lifecycle inside the cluster. The installer is still responsible to create the initial version of the Secret as that is required to support UPI installation by generating the initial ignition configs until the installer provisions the control plane via MAO and not TF. The MCO takes ownership of the Secret containing the stub ignition once the cluster is up and running. When we'll update to the new Ignition version, the MCO can update the Secret.

The final goal for this enhancement is to be able to manage that Secret from the MCO during the ignition migration to the new spec.

If we don't manage that Secret, its content will be on an unsupported version of Ignition that can't provision new machines joining the cluster.

### Non-Goals

To make this simple and to be able to fit this into the release that will bring Ignition v2, we're not considering per-machine Secret(s) as highlighted here https://github.com/openshift/machine-config-operator/issues/683#issuecomment-493193902 and here https://github.com/openshift/machine-config-operator/issues/683#issuecomment-640602493

We don't want to diverge from the current implementation where the Installer still creates the Secret and stub ignition on installation otherwise we'll have to change far more things in order to keep supporting UPI installation that needs ignition configs before starting to install.

## Proposal

The proposal is pretty simple today:

- The installer keeps creating the Secret that holds the stub ignition config (at any given version) under a new name
- The secret will change name from `<role>-user-data` to `<role>-user-data-managed` which is going to be v3 at a later time before the release
- The name change is needed to keep supporting old bootimages that still reference the old ignition version
- The MCO takes over owning that Secret and its content once the cluster is up and running
- The MCO has a way to manage the lifecycle of the Secret containing the stub ignition


### Implementation Details/Notes/Constraints

The implementation has already started and there are two PRs that provide a working POC:

- https://github.com/openshift/machine-config-operator/pull/1792
- https://github.com/openshift/installer/pull/3730

The MCO PR simply learns how to create the content of the secret and once it's running, it goes ahead and takes ownership of the secret in the cluster (you can see we're using the new name ending in `-managed`). That allows us to upgrade the MCO to a newer Ignition version and make sure the secret's content is updated as well.
The installer PR is just a rename of the old user-data secret to something new that the MCO can manage. The main reason to the rename is ability to still have the old user-data secret around during an upgrade.

The whole implementation, related to the ignition upgrade will go as follows:

- installation (no impact)
    - everything starts with the new ignition version
    - installer creates the user-data with ignition v3
    - MCO is ignition v3 as well
    - scale up works as the new bootimages understand ignition v3

- upgrade (impacted)
    - everything is ignition v2, including bootimages
    - the user-data secret is also an ignition v2 snippet
    - the upgrade starts bringing the new MCO which understands ignition v3
    - the MCO creates the new managed secret containing an ignition v3 snippet and leaving around the old, unmanaged secret which is v2
    - scale up continues working as the Machine(Sets) still reference the unmanaged, v2 snippet
    - when we'll grow the ability to update bootimages, the new bootimages will understand v3 and they'll grab the managed secret which is v3

The last point above in the upgrade scenario is captured in https://github.com/openshift/enhancements/pull/201 - that enhancement is our long term solution to that problem. If there'd be a source of truth available for the cluster version bootimage the mao could use that by default and the mco could generate a userData secret with the right ign by machine creation request.


### Risks and Mitigations

This enhancement isn't about the migration to Ignition v3. Any risks and mitigations related to that won't be captured here. This is going to be a straightforward implementation during the development cycle leading to the final migration (which will obviously require more coordinations and/or ratcheting strategy). The only thing to note related to this enhacement is the need to coordinate 2 code changes in both MCO and installer, with the MCO one needed first.


## Drawbacks

We're not at a point where we can fully manage that Secret from the MCO as we don't run early enough to support UPI needing ignition configs. This drawbacks is going to be later caputured in a GitHub issue and/or enhacement here where we would like to have the installer use the MAO to provision the control plane.

## Alternatives

Most notable alternative is https://github.com/openshift/machine-config-operator/issues/683#issuecomment-493193902 but it's far from being something we could implement during one release as it requires changing how the MAO work.

