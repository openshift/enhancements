---
title: split-rhcos-into-layers
authors:
  - "@jlebon"
  - "@cverna"
  - "@travier"
reviewers:
  - "@patrickdillon, for installer impact"
  - "@rphillips, for node impact"
  - "@joepvd, for ART impact"
  - "@yuqi-zhang, for MCO impact"
  - "@LorbusChris, for OKD impact"
  - "@zaneb, for agent installer impact"
  - "@sdodson, for overall architecture"
  - "@cgwalters, for overall architecture"
  - "@jupierce, for overall architecture"
approvers:
  - "@mrunalp"
api-approvers:
  - None
creation-date: 2024-06-07
last-updated: 2024-06-07
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1190
see-also:
  - "/enhancements/ocp-coreos-layering/ocp-coreos-layering.md"
---

# Split RHCOS into layers

## Summary

This enhancement describes improvements to the way RHEL CoreOS (RHCOS) is built
so that it will better align with image mode for RHEL, all while also providing
benefits on the OpenShift side. Currently, RHCOS is built as a single layer that
includes both RHEL and OCP content. This enhancement proposes splitting it into
three layers. Going from bottom to top:
1. the bootc layer; i.e. the base rhel-bootc image shared with image mode for RHEL (RHEL-versioned)
2. the CoreOS layer; i.e. coreos-installer, ignition, afterburn, scripts, etc... (RHEL-versioned)
3. the node layer; i.e. oc, kubelet, cri-o, etc... (OCP-versioned)

The terms "bootc layer", "CoreOS layer", and "node layer" will be used
throughout this enhancement to refer to these.

The details of this enhancement focus on doing the first split: creating the node
layer as distinct from the CoreOS layer (which will not yet be rebased on top of
a bootc layer). The two changes involved which most affect OCP are:
1. bootimages will no longer contain OCP components (e.g. kubelet, cri-o, etc...)
2. the `rhel-coreos` payload image will be built in Prow/Konflux (as any other)

This enhancement is based on initial sketches by @cgwalters in:
https://github.com/openshift/os/issues/799

## Motivation

Image mode for RHEL provides an opportunity to rethink how RHCOS is built
and allows us to share a lot more with other image mode variants. A key benefit
on the RHEL side is that we would have only one stream of RHCOS per RHEL release,
rather than one per OpenShift release. This greatly reduces the workload on the
CoreOS team. Another benefit is easier integration in the CI processes of
rhel-bootc and centos-bootc, as well as better shared documentation.

On the OCP side, the final node image would now be built as a layered
container image, just like most other OpenShift components are built. This should
allow simplifying CI and ART tooling related to RHCOS, much of which is bespoke.

For example, we would no longer have to rely solely on ART to sync RHCOS images
to CI; CI could build its own like it does other images. Layering also means
faster iteration on OCP packages (e.g. openshift-clients can keep churning fast
without incurring a full RHCOS rebuild each time). And it also opens the door to
more easily ship binaries in the OS without necessarily having to package them
as RPMs first.

Finally, keeping the CoreOS layer separate from the node layer provides major
benefits to OKD. First, because it makes it easier to share the derivation process
for building the node image (today, OKD maintains its own separate definitions).
Second, because it allows the sharing of CentOS Stream-based bootimages between
OCP and OKD (today, OKD uses FCOS bootimages). These changes would take OKD one
step closer to become a true upstream of OCP.

### User Stories

* As an OpenShift engineer, I want the openshift/os repo to work similarly to other OpenShift repos (with a `Containerfile`), so that CI can easily build and test a PR in a cluster and humans, too, can more easily build and test PRs or e.g. use Cluster Bot.

* As a CoreOS engineer, I want to only have a single stream of RHCOS per RHEL stream, so that I can greatly reduce the amount of streams to maintain (build, test, monitor for failures, flakes, etc...), reduce the number of bootimage bumps, and enforce more consistency across OCP releases that span the same RHEL version.

* As a CoreOS engineer, I want to build RHCOS on top of rhel-bootc, so that I can benefit from support by the wider RHEL organization on those images and collaborate on CI testing and fixing bugs.

* As an OpenShift customer, I want to be able to follow (most of) the image mode for RHEL documentation and use (most of) the same tooling surrounding it when customizing RHCOS, so that I have a consistent experience across image mode for RHEL and RHCOS and a clear understanding of how they relate to each other.

* As an ART engineer, I want to be able to build the OpenShift node image in a similar way to how the other OpenShift components are built, so that I can simplify my tooling and pipelines.

* As an ART engineer, I want to be able to build the OpenShift node image with hotfixes using the same container build pipeline used to build the official node image.

* As a RHEL kernel engineer, I want to be able to debug an OCP kernel bug the same way I do for image mode for RHEL, so that I don't have to learn different ways to do things.

* As an OKD contributor, I want to be able to use CentOS Stream-based boot images, so that I don't have to keep pivoting from Fedora CoreOS boot images into CentOS Stream everytime.

* As an OKD contributor, I want to be able to share node image building processes with OCP, so that I can reduce the maintenance load.

* (Longer-term) As a product manager, I want image mode for RHEL certification and support guarantees to automatically apply to OpenShift clusters.

### Goals

- Build RHCOS/SCOS on top of rhel-bootc/centos-bootc
- Share efforts, especially CI and docs, with the larger rhel/centos-bootc project
- Maintain a single RHCOS stream per RHEL stream
- Build the OpenShift node image as a layered image on top of RHCOS/SCOS
- Build the OpenShift node image using Prow (in CI) and Konflux (in prod)
- Introduce no additional reboots during cluster bootstrapping

### Non-Goals

- Change the cluster installation flow. It should remain the same whether IPI/UPI/AI/ABI/etc.
- Introduce cluster administrator-visible changes. This change should be transparent to
  admistrators. CoreOS layering instructions should keep working as is, but documentation
  should ideally be reworked to leverage rhel-bootc docs more.
- Remove Ignition. While image mode for RHEL lessens the need for Ignition, it remains a
  crucial part of the provisioning flow and will stick around for the foreseeable future.

## Proposal

The overall proposal consists of two parts. In the first part, we will focus on moving
the OCP-specific packages out of RHCOS and into its own layer (the node layer). In the second
part, we'll change RHCOS itself to become a layered image (the CoreOS layer) built on top
of the rhel-bootc work (the bootc layer).

As mentioned in the summary, this enhancement is focused on detailing the first part only.
Firstly, because it's the one we'll start with, and secondly, because it's the one that
will most impact OpenShift. The second part will require changes that are more contained
to the CoreOS and ART teams. That said, we felt it important to introduce them together
in this enhancement to provide a larger picture.

Diving into the first part then, at a high-level this is split into three phases.

#### Phase 1: Using RHEL/CentOS Stream-only bootimages

In this phase, the goal is to have the OpenShift installer use bootimages containing only
RHEL/CentOS Stream content.

To do this, we will start building two new CoreOS streams. In this context,
"CoreOS stream" refers to a stream of bootable container and disk images output
from the CoreOS Jenkins instance (these disk images for example are what end up
on the OpenShift mirrors eventually). These two new CoreOS streams will contain
only pure RHEL/CentOS Stream content (let's call these the "pure RHEL stream"
and "pure CentOS stream").

Once we have these bootimages, we can better start adapting components that will need it
to account for the lack of OpenShift components in the bootimages. Likely suspects here are
any components involved in the bringup of the cluster (installer, Assisted Installer, MCO, etc...).

Once all known issues have been resolved, the installer bootimage metadata will be updated
to point to these new images. At this point, the OpenShift node image (i.e. the
`rhel-coreos` image in the release payload) has not changed.

Similarly, the installer bootimage metadata for OKD will be updated to use the new bootimages
from the CentOS stream.

#### Phase 2: Switching `rhel-coreos` in CI

In this phase, the goal is to have the OpenShift node image be built in, and used by, CI.

To do this, we will configure the RHCOS pipeline to push the bootable container image output
from the pure RHEL stream to a development Quay repo. We will then create a new Prow CI job
which will build the OpenShift node image by layering the OpenShift components on top: RPM
packages, config files, etc... This CI node image is rebuilt everytime openshift/os changes
but we'll also have the CoreOS pipeline trigger it whenever it pushes a new base image.

After some testing, the Prow job will be configured to push to the `rhel-coreos` tag in the CI
imagestream. The ART job that syncs the legacy RHCOS container images to CI will be disabled.

(Ideally, we'd use a new tag for this here and in the release payload. E.g. `node` or `os`
would be more appropriate, but... there's a lot of work involved in doing this.)

#### Phase 3: Switching `rhel-coreos` in production

In this phase, the goal is to have the OpenShift node image be built in, and used by, production.

To do this, we will setup a Konflux pipeline to build the layered node image. After some
additional testing, this layered image will replace the current `rhel-coreos` image in the
production release payload.

### Workflow Description

This enhancement should have no immediate impact on cluster administrators. However, better
alignment with image mode for RHEL should result in better documentation and tooling down the line.

### API Extensions

Not applicable.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement does not directly impact HyperShift differently than standalone
cluster, but does open the door for follow-ups that would make HyperShift's life
easier (see "Re-organize MCO templates").

#### Standalone Clusters

This enhancement covers standalone clusters.

#### Single-node Deployments or MicroShift

Although SNO deployments differ in the details of their
[bootstrapping mechanism](https://github.com/openshift/enhancements/blob/master/enhancements/installer/single-node-installation-bootstrap-in-place.md),
the main issue faced there is the same as standalone clusters (that is, that
bootstrapping needs the kubelet to be able to run static pods and stand up the
temporary control plane) and it will be resolved in a similar way.

This enhancement does not directly affect MicroShift. However, it conceptually
makes RHCOS more similar to it by more clearly delineating the OS layer from the
OpenShift layer. It's currently not clear however if there's anything concrete
that can be shared otherwise.

### Implementation Details/Notes/Constraints

As mentioned in the proposal, the work is split into three phases. Here, we go into more
explicit implementation details for each phase.

##### Phase 1: Using RHEL/CentOS Stream-only bootimages

- In openshift/os, create a new variant of RHCOS which contains solely RHEL packages,
  i.e. today's RHCOS but without any OCP components.
  - Note this work has already been done: https://github.com/openshift/os/pull/1445
- In the RHCOS pipeline, start building this new stream variant. This will push the bootable
  container to quay.io/openshift-release-dev/ocp-v4.0-art-dev. Bootimages will also be built
  as usual.
- Use these bootimages to test changes to the installer boostrapping flow.
  Notably, bootstrapping currently requires the kubelet and cri-o to be present
  in order to bring up the temporary control plane. This will be adapted so that
  the installer first live-applies the node image, as if the node did a system
  update.
  - Note this work is already in progress:
    https://github.com/openshift/installer/pull/8742
  Other components will also need to be adapted (such as assisted-installer, but
  likely others too).
- Work with the teams of the affected components to adapt them.
- Open an openshift/installer PR which updates the bootimages. This will allow
  us to run more extensive CI tests on the PR.
- Raise awareness of this change to the OCP org to invite more testing. 
- Once all issues have been resolved, merge the PR. If new regressions are
  encountered, revert the PR and fix them.
- Once satisfied with the new bootimages, the RHCOS pipeline can stop building
  the old bootimages containing OpenShift components.

##### Phase 2: Switching `rhel-coreos` in CI

- Set up a new job in openshift/release which builds the layered OpenShift node image on
  top of the bootable container image from the new stream variant. This job should trigger
  whenever openshift/os changes.
- Make the CoreOS pipeline also trigger this job whenever it pushes new base images since
  currently CI doesn't rebuild on changed base images
- Open MCO and DTK PRs pointing to this layered image in CI (these are the only two components
  that explicitly reference `rhel-coreos`) as a way to verify CI is happy.
    - NOTE: There shouldn't be any major CI failures here. One key thing to understand is that
      we're changing *how* the node image is built, but not *what* its final contents are from
      a squashed image perspective. So this is mostly a sanity-check that they are indeed
      equivalent.
- Once any issues have been resolved, make the new job push to the `:rhel-coreos` tag of the
  CI imagestream. This simultaneously requires disabling ART's job which syncs RHCOS to that
  same tag.

##### Phase 3: Switching `rhel-coreos` in production

- Work with CRT to update the OCP release page to better handle the new layered approach.
    - It currently links to diffs in the RHCOS release browser. But now the
      release browser will only contain the RHEL diff, not the OCP diff in
      the node layer. Additionally, if temporary overrides are present in the
      node image for base packages like the kernel, we really want the release
      controller to be aware of this. Overall, the RPM diff should always be
      computed on the final node image. How to implement this remains to be
      discussed but we will need to close this gap before moving prod. (See
      "Open Questions" below).
- Work with ART to adjust how they determine if the RHCOS images are up to date.
  Currently, they expect both RHEL and OCP content in those images, but that content will
  be split across separate images now.
  - Changes to RHEL content should result in triggering the CoreOS pipeline that builds
    the rhel-coreos-base image (this implicitly will then also trigger a rebuild of both
    CI and prod node images)
  - Changes to OCP RPMs should result in triggering a node image rebuild for prod
    - ART can promote that prod node image to the CI imagestream. This awkwardly
      means that the node image in the CI imagestream is sometime built in CI
      and sometimes prod. This is an issue common to other OCP components. Down
      the line ART could instead also trigger a CI build whenever there's new
      OCP content.
- Work with ART to set up a Konflux pipeline to build the layered OpenShift node image. 
  - We want to make sure that this pipeline is flexible enough to support hotfix
    side container image builds for specific customer cases.
- Do a final set of tests to verify that CI is happy with this built image.
    - There should be no surprises here.
- Once any final issues have been resolved, switch over `rhel-coreos` in prod to come from
  the node image built by the Konflux pipeline.

### Follow-ups

#### Re-organize repos

With these changes in place, one follow-up worth investigating is to move the base RHCOS
definition out of openshift/os. This would better clarify the dividing line between the CoreOS
and node layers, by having their definitions live in separate repos.

The CoreOS layer definition at this point would probably better live somewhere under gitlab.com/redhat
alongside other rhel/centos-bootc projects. Then, openshift/os would be free to contain even more
OpenShift-specific things. For example, we might consider moving content currently in the MCO's
templates into openshift/os. or we might consider rewriting things currently written in bash
into a statically-typed language that can literally live in openshift/os and get compiled and
added as part of the derived (now multi-staged) layered build.

At a more practical level, this helps reduce git churn for both the RHCOS pipeline and the
OCP layered build.

#### Add more comprehensive CI

Another important follow-up would to be to hook up better CI testing to openshift/os. There is
currently no CI test on that repo which actually launches a cluster. The reason is that RHCOS
is just built differently. But now that the node image is simply yet another layered image build,
it fits perfectly in Prow's opinionated model of building the image and shoving it in the test
release payload with which to launch test cluster.

#### Re-organize MCO templates

Currently, there's a pile of templated files that live in the MCO. These
files configure systemd services and run extra logic as required by OCP and
conceptually have nothing to do with the MCO itself. It would then make sense to
move them to the openshift/os repo alongside the rest of the definition for the
node image.

In a first step, we would leave them in the MCO but work on "de-templatizing"
them by leveraging existing systemd directives as much as possible (e.g.
conditionalizing on platform can be done with `ignition.platform.id=azure`,
conditionalizing on OVN could be done with a generator or by having it pulled in
by another systemd unit, etc...).

Once they're fully de-templatized, we can move them to openshift/os so that they
are part of the node image proper.

This would considerably shrink the amount of templates that the MCO would
need to process, and as a result serve via the MCS and/or layer via on-cluster
layering.

#### Rework bootimage bumping flow

Long-term, it would make sense to move the bootimages definition out of the
installer and to the canonical place where the OCP <--> RHEL relationship is
defined: the node image (which references the rhel-coreos-base image in its
`FROM`).

This allows us to more easily maintain consistency across all the OCP releases
that share the same bootimages while streamlining the bootimage update process.
For more information, see this comment:

https://github.com/openshift/enhancements/pull/1637#discussion_r1806785754

### Risks and Mitigations

#### Versioning scheme

The versioning scheme for the base CoreOS container images, and their associated
bootimages, will change from e.g. 418.94.202409090849-0 to e.g. 9.4.20240909-0
since they'll no longer be versioned with OpenShift. (But note that the format
itself remains similar and, importantly, semver-compatible.)

The node image will inherit this versioning information from the base CoreOS
image instead of trying to reinstate the previous 418.94... versioning scheme.
The primary reason is that we can't anyway and even if we could it would be
inconsistent with the rest of OCP.

When building the layered node image, we can't easily come up with a good
version string from inside that process. It would need to be driven by the build
system so that e.g. it knows what the previous version string was and it can add
it to labels for example. AFAIK, there is no precedence for doing this for other
OCP components. Instead, the focus is on tying git commits to built images, and
all the same mechanisms there should work just the same on openshift/os as other
repos.

One important note here: we will still have the CVO version label for the
`machine-os` component, and that version will be the version of the CoreOS base
image we layered on. The whole point of the CVO version label AIUI is to surface
versioning information for embedded components that are versioned differently
from OCP, so this fits nicely here.

This change is likely to throw off anything that currently parses the version
string to for example derive the OCP and RHEL versions for which an RHCOS image
was built. Those will need to be adapted. The release browser as mentioned above
is a likely candidate.

Additionally, RHCOS artifacts on the official mirrors are currently versioned
by the OCP release (e.g. `rhcos-4.16.3-x86_64-live.x86_64.iso`) and kept under
OCP-versioned directories (e.g. `4.16.3/`). We don't want customers to have to
know which version of RHCOS bootimages to download for their target OCP version.
So to avoid confusing customers, we should still have OCP-versioned directories.
Ideally the mirror server could symlink/somehow dedupe the bootimages shared
across OCP versions but this is of course not a requirement.

As such, the versioning change should be transparent to end-users, but on the
off chance that a customer is relying on the current semantics, we should still
announce it in the GA release notes.

### /etc/os-release

Previously, the node image used a heavily customized `/etc/os-release`, mutating
some key fields inherited from RHEL. A major theme of this enhancement and more
largely of the Image Mode effort is re-emphasizing the fact that RHCOS simply
builds on top of RHEL Image Mode. As such, many fields in `/etc/os-release` are
now left untouched.

Important fields that **will be different**:

Key|Old Value|New Value
---|---|---
VERSION|419.94.202412101440-0|9.6.20250203-0 (Plow)
VERSION_ID|4.19|9.6
OSTREE_VERSION|419.94.202412101440-0|9.6.20250203-0
ID|rhcos|rhel

Important fields that **remain the same**:

Key|Value
---|---
NAME|Red Hat Enterprise Linux CoreOS
PRETTY_NAME|Red Hat Enterprise Linux CoreOS $VERSION
VARIANT|CoreOS
VARIANT_ID|coreos
OPENSHIFT_VERSION|4.19

#### Hidden dependencies

Given that we're effectively removing packages from the bootimages, we may
uncover hidden dependencies on the removed packages both within other components
of OpenShift and in end-user code.

Regarding other OCP components, the goal is to catch these dependencies during
CI testing of the installer PR that switches over to the new bootimages. We
need to raise awareness of the change to the whole OCP org and call for extra
testing. This needs to be a coordinated event with ART, CRT, and TRT. Of
course, even after merging, if any regression is found, we can easily revert the
PR to get back to green.

Regarding end-user code, although bootimages are by their very nature
transient (we boot, and shortly after pivot into the node image), there are
some situations where user-provided code may run (e.g. when manually extending
`bootstrap.ign` or `master.ign` or when scripting the live ISO). This code may
be relying on packages that are going away (e.g. some local-only `oc` command,
or grepping `--help` texts of the kubelet, etc...).

We shouldn't worry *too much* about breaking these use cases, but it still seems
worth highlighting in the GA release notes that these OCP packages are no longer
present.

### Drawbacks

- This will make it somewhat harder (or at the very least much more explicit) to
  fast-track RHEL fixes into specific OCP releases. Because everything beneath
  the node layer is RHEL-versioned and possibly shared across multiple OCP
  releases, any fast-tracks would need to happen by doing an RPM upgrade within
  the node layer. This is more awkward than just tagging packages into the
  the ART plashets as currently done, but makes us more thorough in our known
  deltas from RHEL. To do this, we would likely still just have to tag the RHEL
  packages into the ART plashet, but then in the layered build, we would also do
  e.g. `dnf update -y NetworkManager`.
  - We should do this as a last resort and instead work more closely with RHEL
    package maintainers to get fixes into the releases we need.

## Open Questions [optional]

- How to handle the early kernels we currently get from RHEL?
  - The early kernels are RHEL-versioned and it would make sense to have them be
    part of the CoreOS layer. (IOW, we always use the same early kernels for the set
    of OCP streams sharing the same RHEL version.)
  - Currently, this is done by tagging the kernel builds into a specific tag in
    Brew and those builds then being added to the ART plashets. To avoid having
    to source the (OCP-versioned) ART plashets during the CoreOS builds, ART
    could instead create RHEL-versioned plashets that we would add to the CoreOS
    builds alongside baseos and appstream.
- How to update the release controller to have valid diff?
  - As mentioned in phase 3, a requirement will be to adapt the release
    controller to provide or link to a meaninful diff that includes OCP content.
    Querying the rpmdb of any node image is expensive, so we need to cache this
    somehow.
    - Probably the easiest thing to do is to reuse the RHCOS release browser but
      point it at cached rpmdb generated from node images (and uploaded to S3?).
      This cache can be generated by e.g. a Jenkins job, or openshift/release
      service, etc...
    - Another approach is to "fake" a git repo (which is what `oc adm release
      info --changelog` works from) which just tracks rpmdb changes. But it
      still comes back to needing a service to pull down images and populate
      that git repo. Incidentally, if openshift/os used lockfiles, that would
      have naturally fit into this changelog model.

## Test Plan

This has been covered in the proposal and implementation sections.

This isn't an optional feature that's getting added; once we cut over to the
new model, it will be implicitly continuously tested by all OpenShift CI tests.

## Graduation Criteria

This is not a new user-facing (or even internal-facing) OpenShift API. However,
for completeness, let's consider "graduation" in this case as switching OCP over
to using the layered image in production.

For this to happen, the criteria are:
- all affected OpenShift components and processes have been adapted to deal with
  the new bootimages (which no longer have OpenShift packages)
- the OpenShift installer now uses the new bootimages
- the OpenShift node image can be built in Prow/Konflux
- the new artifacts are used as part of the CI streams and all tests pass
- the release controller differ is adapted to deal with split RHEL/OCP content
- ART change detection logic is adapted to deal with split RHEL/OCP content

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

Not applicable.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

On the client-side, the changes here affect mainly the bootstrapping phase. Once
the cluster is up, the nodes have already pivoted on the node image, as today.
So there are no day 2 concerns here.

## Version Skew Strategy

See above.

## Operational Aspects of API Extensions

Not applicable.

## Support Procedures

In general, support procedures will be exactly the same as they currently are for
OS-level issues. However, awareness that the node image is now a layered image will
make it easier for the support team to debug issues by looking in the right places.

## Alternatives (Not Implemented)

1. Taken to its extreme, one alternative of this proposal is to get rid of
RHCOS completely and just use the same bootimages as RHEL. This would have
major benefits as well as major ramifications. We would no longer have to
manage separate bootimages. But OpenShift would have to move from Ignition and
coreos-installer to cloud-init and Anaconda. That lift is currently considered
much too expensive. But if it were to become a desired goal, this enhancement
would still be a good first step.

2. Instead of building the node image in Prow/Konflux, we could keep building it
in RHCOS. This would be easier to fit in the current way things are set up (i.e.
ART consuming content from the RHCOS pipeline), but would still keep RHCOS as a
special snowflake among OpenShift components.
