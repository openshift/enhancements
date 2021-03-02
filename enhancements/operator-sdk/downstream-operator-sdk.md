---
title: downstream-operator-sdk
authors:
  - "@jmrodri"
reviewers:
  - "@joelanford"
  - "@asmacdo"
  - "@fabianvf"
  - "@camilamacedo86"
  - "@shawn-hurley"
  - "@gallettilance"
  - "@kevinrizza"
approvers:
  - "@joelanford"
  - "@asmacdo"
  - "@fabianvf"
creation-date: 2020-09-10
last-updated: 2020-03-02
status: implementable
see-also:
replaces:
superseded-by:

---

# Downstream Operator SDK

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Operator SDK currently ships the Ansible and Helm Operator base images
downstream. The purpose of this enhancement is to add a supported Operator SDK
base image as well as an operator-sdk binary. In addition, to add more
automation to syncing upstream with downstream.

## Motivation

Deliver a properly supported Operator SDK downstream to support OpenShift
developers.

### Goals

The primary goal is to deliver a new base image with the Operator SDK that
can be used by operator developers. Deliver an operator-sdk binary similar to
how oc is delivered to customers. Continue to deliver the Ansible and Helm
operator base image.

Another goal is to automate the syncing of upstream to downstream to make it
easier to bring in upstream releases.

### Non-Goals

We will not deliver an RPM based operator-sdk package as part of this. SDK
downstream scaffolding files to use downstream images this will be done when
Phase 2 plugins are done. For now folks will have to modify their operators to
pull from downstream images.

## Proposal

### User Stories

* Deliver an operator-sdk binary
  * As an operator developer, I would want the SDK that I use to support the supported OpenShift clusters.
  * As a Red Hat operator customer, I would want a supported operator-sdk binary.
* Deliver an operator-sdk base image
  * As an operator developer, I would want a supported Operator SDK base image to use to build my own operators.
  * As a Red Hat operator customer, I would want to have a released downstream Operator SDK base image to use to build my own operators.
  * Provide operator-lib downstream
* Create shell-script to help automate the process
  * As an operator-sdk developer, I would like a way to easily bring upstream SDK releases downstream for release with OpenShift
  * As an operator-sdk developer, I would like to run the upstream tests downstream in an OpenShift cluster
* Continue delivering Ansible and Helm operator base images
  * As an Ansible operator developer, I would want to continue having a released downstream Ansible operator image I could base my operator on.
  * As a Helm operator developer, I would want to continue having  a released downstream Helm operator image I could base my operator on.


### Implementation Details/Notes/Constraints

#### Repos

Mirror the upstream Operator SDK repos downstream in the openshift GitHub
organization. We will have a one to one upstream to downstream repo.

* operator-sdk - github.com/openshift/operator-sdk
* operator-lib - github.com/openshift/operator-lib
* ocp-release-operator-sdk - to be retired after the above repos are working

The downstream repo will contain a special _overlay_ branch that contains all of
the downstream-specific changes. The files to be contained in this overlay are
as follows:

* Dockerfiles to support CI
* Dockerfiles to build downstream images
* Vendor Go dependencies
* Ansible collections to support Ansible operator

We will pin the `downstream:master` to `upstream:master` and
_upstream_release_branches_ to _downstream_release_branches_. This means we will
manage the OpenShift release branches ourselves. So `release-x.y` will track a
specific upstream branch that was mirrored. Only tagged commits will ever be
brought into the release branches.

#### Syncing

Today, we sync `operator-framework/operator-sdk` into
`openshift/ocp-release-operator-sdk`. We sync it manually with `git merge`. The
`master` branch downstream currently tracks the latest OpenShift release. So
when moving to a new major SDK release downstream, the `master` branch sync is
frought with conflicts that can cause missing changes or just make for a very
tedious merge.

The idea behind the autosyncing strategy is that the downstream branches will
match the upstream branches 1 for 1. So downstream `master` will track upstream
`master`. Downstream `vx.y.z` will track upstream `vx.y.z`. So the syncs should
occur with minimal conflicts. These downstream branches will NOT contain any
special downstream fixes, they are strictly mirrors of the upstream.

The autosyncing will occur on a frequent basis configured in the
[configuration file][auto-config]. The frequency will be determined by our
needs.

We will manage our own OpenShift release branches. These branches will be a
combination of a tagged upstream branch plus the overlay branch.

There is an initial prototype of the syncing code already:

https://github.com/fabianvf/ocp-osdk-downstream-automation

#### Downstream Only Directory Structures

Today, we have a set of files scattered through the `ocp-release-operator-sdk`
repo which are NOT in the upstream. For example, `ci` and `release` directories.
Instead of having them in the root of the repo, we will create an `openshift`
directory to house all the files not part of the upstream except the `vendor`
directory which must be in the root since it contains the Golang dependencies.

Having them in a separate directory will also make it easier to do the [Overlay
Branch](#overlay-branch) alternative if the need arises.

The `openshift` directory will contain the following files and sub-directories:

* openshift # directory to contain downstream overlay
  * ci
    * dockerfiles
      * variety of Dockerfiles used to build images for running tests
  * prow.Makefile # Makefile used by CI
  * patches
    * contain the patch files that may be carried
  * release
    * ansible
      * ansible_collections # contains the sync ansible collections
      * Dockerfiles # various dockerfiles
    * helm
      * Dockerfiles # various dockerfiles
    * sdk
      * Dockerfiles # various dockerfiles
    * scorecard-test
      * Dockerfiles # various dockerfiles
    * scorecard-test-kuttl
      * Dockerfiles # various dockerfiles
* vendor
  * contains the dependencies for the project

#### Handling Patches

Typically we want to submit all bugs upstream, have them release then sync the
release downstream. But there will be times we need to fix a problem downstream
to resolve a bug that we can’t wait for an upstream release. The process here
will be to keep patches separate to avoid conflicts during syncing.

For fixes to vendored dependencies, have a patch in the patches directory
that will be applied to the vendor after we run go mod vendor.

For fixes to mainline code, have a patch in the patches directory that will be
applied before builds. In order to do this, we may have to create a downstream
`Makefile` that could include the upstream `Makefile` if we need to insert
patches step.

For either of the above, we will always submit the patches to the appropriate
upstream repos.

To generate the patches, the easiest approach is to use [`gendiff`][gendiff].
The general steps would be any file to be changed, copy it with a meaningful
extension and use `gendiff` to generate the patches to put in the `patch`
directory.

Let's assume `operator_installer.go` needs a fix downstream to fix a bug.

1. Make backup:
   * `cp operator_installer.go operator_installer.go.bugXXXXXX`
1. Make changes required to fix the problem:
   * `vim operator_installer.go`
1. Generate patch
   * cd ROOT_OF_PROJECT
   * `gendiff directory_of_file .bugXXXXXX > 00_bugXXXXXX.patch`
1. Add patch to patches directory.
   * `cp 00_bugXXXXXX.patch patches/`
   * `git add patches/00_bugXXXXXX.patch`
   * `git commit -m "Bug XXXXXX: fixing ..."`

Patches will be applied in order. The patches will live in the overlay branch.

### Risks and Mitigation

Moving forward with the automated syncing is a little high risk in the sense
that there are some unknown issues we may encounter. We will mitigate this risk
by keeping the existing `openshift/ocp-release-operator-sdk` repo until the
automated `openshift/operator-sdk` repo is working. Once the cut over is made we
can remove the `openshift/ocp-release-operator-sdk` repo.

In the event that the automated repo is not ready in time for 4.7, the backup
plan is to do a 4.7 release of OperatorSDK from the existing
`openshift/ocp-release-operator-sdk` repo.

## Design Details

### Test Plan

The existing set of Operator SDK upstream tests will be run downstream. Today
we run tests for the Ansible and Helm operators. A list of the upstream tests we
would run downstream:

* `test-sanity` # sanity checks like formatting and linters
* `test-unit`   # unit tests
* `test-e2e`    # e2e tests

The `test-links` target will *not* be run, since that is used to verify the
links in the upstream docs.

We will run the tests in the OpenShift CI cluster. We will *not* use
[kind][kind] downstream. There may need to be some patches maintained downstream
and/or some changes upstream to ensure the tests run in an OpenShift cluster.

Since OpenShift CI creates a new cluster for each CI run, this can get
expensive. We need to be good stewards of the CI clusters and ensure we are
running the tests sufficiently enough to have a good feel for the quality of the
syncs, but not so much that we are burning through CI resources. At this time we
don't have a solution.

### Graduation Criteria

Operator SDK will go out once the downstream builds work. The Operator SDK will
be downloadable from the [Red Hat Developer page][rh-dev-page].

### Upgrade / Downgrade Strategy

All Operator SDK releases have a migration guide on what users have to do to
upgrade their operators. These guides will likely be made available in the
official OpenShift documentation.

### Version Skew Strategy

* The operator-sdk repo will vendor its dependencies to keep consistent builds
* The ansible and helm repos will also vendor any Go dependencies
* The operator-lib repo will just be a mirror of the upstream

#### Branching

As described in the [Repos](#repos) and [Syncing](#syncing) sections, the
downstream repos will use a different branching strategy.

The downstream repos will contain mirrors of the upstream repos, an overlay
branch, and release branches that correspond to OpenShift releases.

* master - mirrors upstream master
* v1.1.x - mirrors upstream v1.1.x
* v1.0.x - mirrors upstream v1.0.x
* v0.19.x - mirrors upstream v0.19.x
* overlay - contains the downstream [overlay files][overlay-files]
* release-4.8 - SDK release + overlay for OpenShift 4.8
* release-4.7 - SDK release + overlay for OpenShift 4.7

The release branches will have specific tagged versions of the SDK only. We will
not pull in all commits from an upstream release branch unless they have been
tagged and released upstream.

The advantage of managing our own branches is it gives us flexibility in
keeping master in sync with upstream master so we always have the latest code
available downstream.

The disadvantage to this is that each release branch is not managed by ART.

## Implementation History

2020-10-13 - Rewrite proposal using the automation option.
2020-10-07 - Propose to automate the syncing of upstream to downstream.
2020-09-10 - Propose using the UPSTREAM-MERGE.sh script similar to Service
             Catalog releases.

## Drawbacks

* We would have to manage our own downstream release branches
* We would have to handle the automation of updates ourselves
* The special downstream branch is kind of janky

## Alternatives

### Monorepo
One alternative is to have a monorepo and sync all upstream repos to a single
repo. While this is a possible solution it would be more difficult to maintain
for the needs of the SDK team.

### Overlay Branch

Another idea that was floated was having the specific downstream files stored in
an overlay branch that would then be "overlayed" an upstream branch to create a
release branch. For example, release-4.9 would be created from
operator-sdk/v2.0.x plus the overlay branch. As of right now, there doesn't
seem to be a major need for having this over just a specific directory in
master.

### Semi-manual syncing of repos

Mirror the upstream Operator SDK repos downstream in the openshift GitHub
organization. We will have a one to one upstream to downstream repo.

* operator-sdk - github.com/openshift/operator-sdk
* operator-lib - github.com/openshift/operator-lib
* ocp-release-operator-sdk - to be retired after the above repos are working

The downstream repos will contain specific downstream changes which should be
additions and non-conflicting:

* Dockerfiles to support CI
* Dockerfiles to build downstream images
* Vendor Go dependencies
* Script to assist with syncing upstream to downstream
* Ansible collections to support Ansible operator

We would sync `operator-framework/operator-sdk` into `openshift/operator-sdk`
using a script, [UPSTREAM-MERGE.sh][upstream-merge]
which we have adapted from the current openshift/service-catalog.

The script is only maintained in master branch. Other branches should copy this
script into that branch in case newer changes have been made.

The `master` branch maps to a specific OpenShift release. We sync an upstream
minor or patch release we determined to ship with OpenShift. Once `master`
switches to a new release, we will reset master to the new release we plan to
sync for the next OpenShift release. The release branches will continue to be
updated when necessary the same way.

The process to update `master` will be as follows:

```console
cd operator-sdk
git checkout master
./UPSTREAM-MERGE.sh v1.0.0
# resolve any conflicts; script will put you in a new branch
git commit
git push origin NAME_OF_BRANCH
```

For updating a specific release branch the process will largely look the same:

```console
cd operator-sdk
git checkout release-4.6 # checkout specific branch to be updated
./UPSTREAM-MERGE.sh v0.19.5
# resolve any conflicts; script will put you in a new branch
git commit
git push origin NAME_OF_BRANCH
```

The following is a list of directories and files that we will be adde to
the repos:

* UPSTREAM-MERGE.sh # script used to sync the upstream to the downstream
* ci
  * dockerfiles
    * variety of Dockerfiles used to build images for running tests
  * tests
    * e2e test scripts
* prow.Makefile # Makefile used by CI
* patches
  * contain the patch files that may be carried
* release
  * ansible
    * ansible_collections # contains the sync ansible collections
    * Dockerfiles # various dockerfiles
  * helm
    * Dockerfiles # various dockerfiles
  * sdk
    * Dockerfiles # various dockerfiles
  * scorecard-test
    * Dockerfiles # various dockerfiles
  * scorecard-test-kuttl
    * Dockerfiles # various dockerfiles
* vendor
  * contains the dependencies for the project

## Infrastructure Needed

* New openshift/operator-lib repo
* New openshift/operator-sdk repo
* Retire openshift/ocp-release-operator-sdk repo

## Other resources

* [Adding CI Configuration for New Repositories][new-repos]
* [Red Hat Developer page][rh-dev-page]

[gendiff]: <https://linux.die.net/man/1/gendiff>
[overlay-files]: <#overlay-branch-directory-structures>
[upstream-merge]: <https://github.com/jmrodri/scripts/blob/master/UPSTREAM-MERGE.sh>
[new-repos]: <https://steps.ci.openshift.org/help/release#new-repos>
[rh-dev-page]: <https://developers.redhat.com/topics/kubernetes/operators>
[auto-config]: <https://github.com/fabianvf/ocp-osdk-downstream-automation/blob/master/deploy/cronjob.yaml#L40-L69>
[kind]: <https://kind.sigs.k8s.io/>
