---
title: build-root-image-from-repository
authors:
  - @alvaroaleman
reviewers:
  - @marun
  - @sttts
approvers:
  - @stevekuznetsov
creation-date: 2020-09-28
last-updated: 2020-09-28
status: provisional
---


# Build root image from repository

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today, the image in which binaries are built by ci-operator (`build_root_image`) can be
configured by either specifying an imagestream or by specifying a Dockerfile in the
repository to build this image from. In the latler case, the Dockerfile is in the basebranch
of PRs. This means its not possible to do a PR that atomically changes both contents of the
repository and the `build_root_image`. This proposal proposes a way to enable changing both
code and the `build_root_image` atomically with one PR.


## Motivation

### Goals

* Allow changing both repository contents and `build_root` in a single PR

### Non-Goals

* Change anything about the concept of the `build_root` or how we build images
  for promotion
* Introduce any kind of automation that updates `build_root`

## Background

The ci-operator config has a `build_root` section that can either include an imagestreamtag
or a Dockerfile. If it is an Dockerfile, a build of that Dockerfile from the base branch of the
PR will be created. In both cases, the resulting image will be tagged into the `pipeline:root`
imagestreamtag.

The `pipeline:root` imagestreamtag in turn is used to:
* Build binaries via the `binary_build_command` and tag them into the `pipeline:bin` imagestreamtag
* Build test binaries via the `test_binary_build_commands` and tag them into the `pipeline:test-bin` imagestreamtag
* Build rpms via the `rpm_build_commands` and tag them into the `pipeline:rpms` imagestreamtag

All these tags in turn can then be refenced in tests.

Additionally, if the project promotes, there is one Dockerfile per promoted image. It usually duplicates
the `build_root` and `binary_build_commands` but copies the resulting binaries in an `ocp/$version:base`
image.

## Proposal

It is proposed to extend the existing `build_root` section in the ci-operator config
with a new `from_repository` boolean that will indicate of the image should be inferred
from the repository:
```
build_root:
	from_repository: true
```

This setting will be mutually exclusive with all other options in the `build_root` section.
If set, the `ci-operator-prowgen` cli will be changed to clone the repository into the ci
operator pod. This in turn allows the `ci-operator` to read the image to be used from the
repository.

The `build_root_image` must not be a raw docker pullspec but an imagestreamtag, because
we tag it into the `pipeline:root` imagestreamtag and have found that doing an import for
every job run causes performance issues in the underlying cluster.

TODO: Agree on which of the two options below is better.

There are two options to to get the imagestreamtag from:

### A new configfile in the repository

Very simple, but duplicates some bits from the Dockerfile used for promotion and hence risks
that what we test is not what we ship. That risk could be mitigated by introducing a check if
the two images are "semantically identical" which would probably just mean to make sure they
have the same golang version.

The file could be made extensible to allows us to use it for additional options if we end up
finding a need for that in the future.

### Inferring the image from the Dockerfile used for promotion

This is possible and avoids duplication, but it creates some challenges:

* More than one image can be specified for promotion. Which one do we use?
* For multistage-builds, we must make assumptions about which stages `FROM` is the one we should use - first? last?
* We must pass through and apply the replacements from the ci-operator config
* We will have to fail the build if the `FROM` includes a registry/is not an imagestreamtag

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.
