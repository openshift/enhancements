---
title: kubernetes-rebase
authors:
  - "@fbertina"
reviewers:
  - "@soltysh"
approvers:
  - "@soltysh"
creation-date: 2023-06-15
last-updated: 2023-06-15
---

# Kubernetes Continuous Rebase

## Goal

The main goal of this proposal is to proactively identify and address
any potential issues that may arise during the upcoming rebase
process.

The desired outcome is to be able to land the rebase PR significantly
earlier in the process, potentially aligning with the release of the
upstream tag.

## Proposal

Currently, the rebase work is typically spread out over a period of 1
or 2 months. However, it can potentially be distributed throughout the
development cycle of Kubernetes. To achieve this, we could have an OCP
branch with a continually updated Kubernetes codebase, allowing most
of the work to be completed even before the rebase process begins.

The main approach involves applying our downstream patches against the
upstream master branch on a daily basis. It is expected that some
patches may fail to be applied multiple times during the development
cycle. However, as soon as such failures occur, we will receive
notifications, and the necessary fixes will be applied to the
downstream patches or the upstream code.

Implementing this approach brings several benefits:

1. The rebase process becomes less time-sensitive.
2. We receive early signals if an upstream change breaks OCP, enabling
   us to address the issue promptly either in the upstream code or on
   our side.
3. The rebase PR should be ready to be landed as soon as the upstream
   code becomes generally available (GA).

To implement this proposal, the following steps are required:

### Watcher

For each OCP release, we will designate a watcher to participate in
the process. Ideally, it should be the same person who will execute
the final rebase.

A watcher is responsible for ensuring that the remaining steps
outlined below are executed without errors.

Although some manual work is required, it should not occupy their
entire daily working time.

### A -next branch (optional)

For each of the dependencies listed below, a new branch called
`ocp-next` is created with their Kubernetes dependencies updated:

* openshift/api
* openshift/client-go
* openshift/library-go
* openshift/apiserver-library-go

Initially, this can be done manually on a weekly basis. In the future,
certain parts of this process can potentially be automated, requiring
manual intervention only when the automation fails.

This process should already uncover some future issues, requiring
fixes on unit tests or Makefiles for instance.

### CI Job

The goal of the CI job is to detect if our downstream patches create
any code conflicts when applied to the upstream code. In addition to
that, it will uncover potential issues with dependencies and
generated code.

In short, the new CI job will:

1. Take a series of downstream patches and apply them against the
   upstream code.
2. Pin the dependencies mentioned above to the HEAD of their
   respective `ocp-next` branches.
3. Update the auto-generated code and docs (i.e., `make update`).
4. Make sure the codebase is in a sane state by executing automated
   verification and testing with `make` (i.e., `test`, `verify`,
   `build`, etc.).
5. Commit and push the local changes to an `ocp-next` branch in a
   remote repository.
6. Update or create the Pull Request.

If the job fails to execute any of the steps above, the watcher is
responsible for fixing whatever is preventing the job from
succeeding. Examples of fixes include:

1. Making a code change to the downstream patch to address a code
   conflict.
2. Creating an upstream PR to correct any breaking change.
3. Creating a new downstream patch to rectify an incorrect assumption
   in our operators.

A prototype of this workflow is available
[here](https://github.com/bertinatto/ocp-next/blob/master/next.go).

### Open Questions

This proposal assumes that all downstream patches are located in a
specific directory, such as the `patches` directory in [this
prototype](https://github.com/bertinatto/ocp-next/tree/master/patches).

However, it is unclear how we can ensure that this directory remains
up-to-date with the latest patches imported into our
openshift/kubernetes fork.

Here are a few potential options to address this issue:

1. Establish the patches directory as the source of truth for all
   downstream patches. This would require teams to ensure that their
   patches are imported into this directory whenever they introduce a
   new carry patch. It may be beneficial to implement some automation
   to streamline this process.
2. Automate the process of listing and applying patches from the git
   log, as described
   [here](https://github.com/openshift/kubernetes/blob/master/REBASE.openshift.md#creating-a-spreadsheet-of-carry-commits-from-the-previous-release).
   In case the automation fails to cherry-pick a specific patch, it
   can then search for the patch in the patches directory. This is the
   approach taken by the tooling currently under development
   [here](https://github.com/soltysh/rebase).

## Conclusion

The proposed approach involves establishing an OCP branch with an
updated Kubernetes codebase, daily application of downstream patches,
and the setup of a CI job to detect code conflicts and and failures in
generated code.

The implementation of this proposal aims to improve the rebase process
and proactively address potential issues the are currently only
detected when the rebase process starts.

The ultimate goal is to land the rebase PR considerably early,
potentially aligning with the release of the upstream GA tag. This
will allow us to expose updated features and fixes from upstream to
our OCP teams considerably earlier than we do today.
