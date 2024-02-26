---
title: cloud-provider-azure-dependency-update-in-azure-csi-drivers
authors:
  - "@fbertina"
reviewers:
  - "@openshift/storage"
  - "@JoelSpeed"
approvers:
  - "@openshift/storage"
  - "@JoelSpeed"
api-approvers:
  - None
creation-date: 2024-02-26
last-updated: 2024-02-26
tracking-link:
  - https://issues.redhat.com/browse/STOR-1764
see-also:
  - None
replaces:
  - None
superseded-by:
  - None
---

# cloud-provider-azure dependency update for Azure CSI drivers

## Summary

Azure cloud logic from embedded in Azure CSI drivers rely on the
`cloud-provider-azure` dependency.  As a result of that, many bug
fixes to Azure CSI drivers go into that dependency.  However, for
older version of OCP, bumping that dependency results in too many code
changes, increasing the risk of introducing new issues on the CSI
drivers.  As a result, we need a way to address specific issues in
`cloud-provider-azure` without risking the overall stability of the
CSI drivers.

## Motivation


Some of the fixes for our Azure CSI drivers are implemented primarily
in the `cloud-provider-azure` dependency.This means that, once issues
are fixed in the dependency, we simply need to bump that dependency in
our CSI driver fork to get the bug fix.

This works well for recent versions of the CSI driver, however,
updating this dependency in older versions of OCP typically pulls in a
huge amount of code changes that are often unrelated to the fix being
addressed.

This is problematic because it increases the risk of introducing new
and unknown issues to our CSI drivers. In addition to that, it makes
it hard for QE to cover all the incoming changes in their test plan.

So far we've addressed this problem by directly patching the vendor
directory (see
[this](https://github.com/openshift/azure-disk-csi-driver/pull/60) and
[this](https://github.com/openshift/azure-disk-csi-driver/pull/62)),
however, this is not sustainable in the long term.

### User Stories

* As an OpenShift engineer,I want to fix issues in Azure CSI drivers
without merging additional code, so that I can be sure I am not
introducing other issues while doing so.

### Goals


The goal of this enhancement is to document a strategy for updating
the `cloud-provider-azure` dependency within our CSI drivers' forks in
older versions of OCP.

### Non-Goals

We will not cover other dependencies or forks.

## Proposal

We propose to have branches in `openshift/cloud-provider-azure`
repository that only contains fixes for the Azure CSI drivers. Those
branches are based off on the exact version that CSI drivers rely
on. Additional bug fixes will be added on top of it.

### Workflow Description

The workflow is better described with an example. Consider the
following problem:

1. QE discovers an issue with the `azure-disk-csi-driver` in OCP 4.12,
   with a bugfix being available in a newer `cloud-provider-azure`
   dependency.
1. In OCP 4.12, the driver depends on the version `v0.7.0` of
   `cloud-provider-azure`.
1. A bugfix is available in `v0.11.0`.
1. Bumping the dependency to that version brings in several unrelated
   code changes.

Our proposed solution:

1. In `openshift/cloud-provider-azure`, a Staff Engineer manually
   creates a branch called `csi-4.12-patches` based on the upstream
   `v0.7.0`.
   1. This only needs to be done once per OCP version, on demand.
1. The engineer working on the issue then creates a local branch off
   of `csi-4.12-patches` and then applies the fix for the given issue
   on top of it.
1. The engineer runs some basic code verification and unit tests.
1. The engineer submits a PR to `openshift/cloud-provider-azure`
   targeting the `csi-4.12-patches` branch.
1. Once the PR merges, the engineer can then bump the
   `cloud-provider-azure` dependency in the `azure-disk-csi-driver`
   repository using the [replace
   directive](https://go.dev/ref/mod#go-mod-file-replace) in go.mod
   file.

### API Extensions

NA.

### Topology Considerations

#### Hypershift / Hosted Control Planes

NA.

#### Standalone Clusters

NA.

#### Single-node Deployments or MicroShift

NA.

### Implementation Details/Notes/Constraints

NA.

### Risks and Mitigations

The biggest risk is to not have any CI in the CI branches in
`openshift/cloud-provider-azure`. This means no automation (`\lgtm`)
and no jobs. However, we expect to mitigate this risk by:

1. Having CI in the `openshift/azure-disk-csi-driver` PR that will
   bump the `cloud-provider-azure` dependency.
1. Running tests locally before submitting the PR to
   `openshift/cloud-provider-azure`.

Another possible action to completely remove this risk is to configure
our CI to act on the CSI branches of `openshift/cloud-provider-azure`
as well. However, we expect the changes there to be very rare, so we
believe the effort is not worth it at this point in time.

### Drawbacks

This whole effort might not pay off because we do not expect this to
happen pretty often, which can be an argument for us to keep patching
the vendor directory in `openshift/azure-disk-csi-driver`, as we have
been doing recently.

## Open Questions [optional]

NA.

## Test Plan

Changes to the CSI branches in `openshift/cloud-provider-azure` are
manually tested by the engineer.  Once the dependency is bumped in the
CSI driver, regular CI jobs will make sure everything is working well.

## Graduation Criteria

NA.

### Dev Preview -> Tech Preview

NA.

### Tech Preview -> GA

NA.

### Removing a deprecated feature

NA.

## Upgrade / Downgrade Strategy

NA.

## Version Skew Strategy

NA.

## Operational Aspects of API Extensions

NA.

## Support Procedures

NA.

## Alternatives

* Use Go Workspaces. There is an experimental PR
 [here](https://github.com/openshift/azure-disk-csi-driver/pull/77),
 however, this approach requires quite a lot of changes, which defeats
 the purpose.

* Patch the CSI driver's vendor directory directly, which is what we
have been doing. However, this is a bad solution because those changes
can be easily overwritten and lost.

## Infrastructure Needed [optional]

NA.
