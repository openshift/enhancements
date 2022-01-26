---
title: secure-shortname-image-pulls
authors:
  - "@umohnani8"
reviewers:
  - "@mrunalp"
approvers:
  - "@mrunalp"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - N/A
creation-date: 2022-01-26
last-updated: 2022-01-26
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/RUN-1134
see-also:
replaces:
superseded-by:
---

# Secure Shortname Image Pulls

## Summary

Pulling images via shortnames is not very secure. There is a possibility of
spoofing where the image is pulled from as it goes through the list of `unqualified-search-registries`
checking for a match and pulls the image from the first location it is found in. With this method
there is no guarantee that the image is actually being pulled from the correct source.

We have made this more secure by adding support for an alias table that has a list of shortname images
pointing to their fully qualified image name. This alias table is checked for a match whenever a pull
via shortname is attempted, which guarantees that the image will only be pulled from the location specified
in the alias table.

## Motivation

We are always trying to make OpenShift more secure. While we do highly discourage the usage of
shortnames in favour of fully qualified names, there are users who still prefer using shortnames
so we have updated the pull logic to be more secure than just checking a list of registries and pulling
from the first location where a match is found.

So, to make the shortname use-case more secure, the c/image library was updated to check an
alias table when available. An alias table is a list of image names with shortnames pointing to their
corresponding fully qualified image names. We can use this new feature to ensure that any Red Hat and OpenShift
images that are referred to by short name will always resolve to our official locations, hence
guaranteeing that those images will be official.

### Goals

- Secure where the official Red Hat and OpenShift images are pulled from when using shortnames
- The alias table list will be maintained by us and be specific to the OpenShift use-case
- The MCO will be used to lay down the alias table

### Non-Goals

- This is not encouraging the use of shortnames. It is instead focused on making the few use-cases
  that are out there more secure

## Proposal

Enable the shortname alias logic in the cri-o code so that when an image is being pulled and
referred to by shortname, cri-o will check the alias table first and if there is a match it
will pull the image from that location. If there is no match it will fallback to check the
`unqualified-search-registries` list in order and pull the image from the first location where
a match is found - note, this is what is done currently.

We will maintain the alias table shipped on the OpenShift node via the MCO. We will create an alias
table for the official Red Hat and OCP images based on a subset of the aliases in the existing tables
that are shipped in RHEL. We are doing this as the tables shipped with RHEL have a lot of images from
the community which are not required in the OpenShift use-case.

Note: This proposal is a security enhancement around the shortnames use case and is not an API change.

### User Stories

#### As a user, I would like to use image short names when running my workloads

When the user uses shortname for official Red Hat and OpenShift images that have a matching alias in the
alias table, the image will be pulled from the matching location specified there. This will ensure
the security of where these images are being pulled from reducing the risk of spoofing.

### API Extensions

No API changes are needed for this proposal.

### Implementation Details/Notes/Constraints [optional]

Implementing this enhancement requires changes in:

- cri-o/cri-o
- openshift/machine-config-operator

Update the cri-o code to enable the shortname alias logic to `permissive` mode. This will ensure
that when an image is referred to by shortname, it will check the alias table first, if there is no
match, it will then try all the registries in the `unqualified-search-registries` list in order.

Documentation: We will document that while we highly discourage the use of shortnames, there is a way to have
more secure shortname access for official Red Hat and OpenShift images.

### Risks and Mitigations

There are no risks involved here as the alias table will be maintained by us in the MCO repo and will only contain aliases for official
Red Hat and OpenShift images.

## Design Details

### Open Questions [optional]

Question 1: Would image name resolution with mirrors when configured be okay when tags are enabled?

  Shortname with alias will work with mirrors if any mirrors are configured.

  Example:

  If shortname `myimage` has a fully qualified image name match in the alias table `myreg.io/repo/myimage` and
  there is a mirror configuration where the source is `myreg.io/repo` and mirror is `mirror.io/repo`. The image
  pull code will resolve the shortname first with the alias table and with the fully qualified image name it found
  there, it will do the mirror magic and pull the image from `mirror.io/repo`.

  Note: currently, the mirror configuration only works with digests, but there is WIP to enable tags as well where this
  use case may end up being more common.

Question 2: Do we want to allow users to modify the alias table with images for their use case?

### Test Plan

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

Not an API change, so graduation process is not needed.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Upgrade and Downgrade should not be affected.

### Version Skew Strategy

Version skew should not be affected by this.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

- The only failure mode is the image will fail to pull if a matching alias doesn't exist and the image does
not exist in any of the registries in the `unqualified-search-registries` list. This is the same as the failure
mode of the current method.
- The node and/or container engines team will most likely be called upon during an escalations.

#### Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  The cri-o logs will have information on why any image pulls failed.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    No consequences on the cluster health.

  - What consequences does it have on existing, running workloads?

    No consequences on existing, running workloads.

  - What consequences does it have for newly created workloads?

    If the image being used in the new workload is referred to by shortname, the alias table will be checked
    for a matching fully qualified name. If there is no match in the alias table, it will fallback to check
    all the registries in the `unqualified-search-registries` list.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  This will not affect the way the functionality fails as it falls back to the current method if no match is
  found in the alias table.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]
