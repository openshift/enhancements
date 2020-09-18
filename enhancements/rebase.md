---
title: kubernetes-rebase
authors:
  - "@polynomial"
  - "@marun"
reviewers:
  - "@polynomial"
  - "@sttts"
approvers:
  - "@sttts"
creation-date: 2020-04-21
last-updated: 2020-09-17
---

# Rebasing openshift/kubernetes on kubernetes/kubernetes

## Relevance

This document provides instructions for rebasing release branches 4.5 and
prior. Rebasing for releases 4.6 and above is documented in the
[openshift/kubernetes](https://github.com/openshift/kubernetes/blob/master/REBASE.openshift.md)
repository.

## Motivation for this document

OpenShift is based on upstream Kubernetes. With every release of upstream Kubernetes,
it is necessary to create a new downstream release branch that adds
openshift-specific behavior to the upstream release branch. This document describes
the process for creating the downstream release branch.

## Getting started

Before the rebase you may:

- Read this document
- Make sure you can access the publishing bot’s logs
- Get familiar with tig (text-mode interface for git)
- Find the best tool for resolving merge conflicts
- Use diff3 conflict resolution strategy
   (https://blog.nilbus.com/take-the-pain-out-of-git-conflict-resolution-use-diff3/)
- Teach Git to remember how you’ve resolved a conflict so that the next time it can
  resolve it automatically (https://git-scm.com/book/en/v2/Git-Tools-Rerere)

## Preparing the local repo clone

Clone from a personal fork of kubernetes:

```
git clone https://github.com/<user id>/kubernetes
```

Enable push for personal fork:

```
git remote set-url --push origin git@github.com:<user id>/kubernetes.git
```

Add a remote for upstream and fetch its branches:

```
git remote add --fetch upstream https://github.com/kubernetes/kubernetes
```

Add a remote for the openshift fork and fetch its branches:

```
git remote add --fetch openshift https://github.com/openshift/kubernetes
```

## Creating a new local branch for the new rebase

The openshift branch name should have the form:

```
origin-<openshift version>-kubernetes-<kubernetes tag minus 'v' prefix>
```

For openshift version `4.5` and upstream tag `v1.18.2`, the branch
name would be `origin-4.5-kubernetes-1.18.2`.

Create a new branch from the upstream tag:

```
git checkout -b <name of new rebase branch> <upstream tag>
```

## Creating a local tracking branch for the previous rebase branch

To simplify access to commits made to the previous rebase branch,
create a local tracking branch:

```
git checkout --track openshift/<name of previous rebase branch>
```

## Creating a spreadsheet of carry commits from the previous release

Find the hash of the upstream tag (e.g. `v1.18.0-rc.1`) that the
previous rebase branch was based on:

```
git rev-list -1 <upstream tag>
```

The first commit in the previous rebase branch will be the one immediately following
the commit identified by the hash of the upstream tag:

```
git log --reverse --pretty=%H --ancestry-path <hash of upstream tag>..<name of previous rebase branch> | head -n 1
```

Using the hash of the first commit of the previous rebase branch,
generate a csv file containing the commits from the previous rebase
branch:

```
git log <first commit hash>..<name of previous rebase branch> \
 --pretty=format:'%H,https://github.com/openshift/kubernetes/commit/%H,,%s' | \
 sed 's#,UPSTREAM: \([0-9]*\):#https://github.com/kubernetes/kubernetes/pull/\1,UPSTREAM \1:#' > \
 <name of previous rebase branch>.csv
```

This csv file can be imported into a google sheets spreadsheet to
track the progress of picking commits to the new rebase branch. The
spreadsheet can also be a way of communicating with rebase
reviewers. For an example of how this communication, please see the
[the spreadsheet used for the 1.18
rebase](https://docs.google.com/spreadsheets/d/10KYptJkDB1z8_RYCQVBYDjdTlRfyoXILMa0Fg8tnNlY/edit).

## Updating godeps.json for glide

Assuming the local repo has the new rebase branch checked out:

```
go run k8s.io/publishing-bot/cmd/godeps-gen <(go list -m -json all | jq 'select(.Version!="v0.0.0")') > Godeps/Godeps.json
```

Commit the resulting file with the command as the commit message. The changes to go.mod
and go.sum made by `go run` can be reverted.

## Picking commits from the previous rebase branch to the new branch

Commits carried on rebase branches have commit messages prefixed as follows:

- `UPSTREAM <carry>`
  - A persistent carry that should probably be picked for the subsequent rebase branch.
  - In general, these commits are used to modify behavior for consistency or
    compatibility with openshift.
- `UPSTREAM <drop>`
  - A carry that should not be picked for the subsequent rebase branch.
  - In general, these commits are used to maintain the codebase in ways that are
    branch-specific, like the update of generated files or dependencies.
- `UPSTREAM 77870`
  - The number identifies a PR in upstream kubernetes
    (i.e. `https://github.com/kubernetes/kubernetes/pull/<pr id>`)
  - A commit with this message should only be picked into the subsequent rebase branch
    if the commits of the referenced PR are not included in the upstream branch.
  - To check if a given commit is included in the upstream branch, open the referenced
    upstream PR and check any of its commits for the release tag (e.g. `v.1.18.2`)
    targeted by the new rebase branch.
  - TODO(marun) include image from google doc

With these guidelines in mind, pick the appropriate commits from the previous rebase
branch into the new rebase branch. As per the example of previous rebase spreadsheets,
color each commit in the spreadsheet to indicate to reviewers whether or not a commit
was picked and the rationale for your choice.

Where it makes sense to do so, squash carried changes that are tightly coupled to
simplify future rebases. If the commit message of a carry does not conform to
expectations, feel free to revise and note the change in the spreadsheet row for the
commit.

## Updating dependencies

Once the commits are all picked from the previous rebase branch, each repo that those
commits depend on needs to be updated to depend on the targeted upstream tag targeted
by the rebase:

- https://github.com/openshift/api
- https://github.com/openshift/apiserver-library-go
- https://github.com/openshift/client-go
- https://github.com/openshift/library-go


Usually these repositories are updated in parallel by other team members.

Once the above repos have been updated to the target release, it will be necessary to
update go.mod to point to the appropriate revision of these repos by running
`hack/pin-dependency.sh` for each of them and then running `hack/update-vendor.sh` (as
per the [upstream
documentation](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/vendor.md#adding-or-updating-a-dependency)).

Make sure to commit the result of a vendoring update with `UPSTREAM: <drop>: bump(*)`.

## Cleaning up the code and updating generated files

Once the dependencies have been cleaned up, it's time to prepare the branch for a PR:

 - Clean up gofmt by running `hack/update-gofmt.sh`
   - Where possible, apply gofmt changes to carry commits.
 - Update generated files by running `make update`
   - This step depends on etcd being installed in the path, which can be accomplished
     by running `hack/install-etcd.sh`.

Make sure to commit these steps separately with prefixes of `UPSTREAM: <drop>:`.

## Building and testing

- Build the code with `make`
- Test the code with `make test`
  - This should pass locally but is expected to fail in the `unit` ci job due to
    dependency problems.
  - Where test failures are encountered and can't be trivially resolved, the
    spreadsheet can be used to to track those failures to their resolution. The
    example spreadsheet should have a sheet that demonstrates this tracking.
  - Where a test failure proves challenging to fix without specialized knowledge,
    make sure to coordinate with the team(s) responsible for areas of focus
    exhibiting test failure. If in doubt, ask for help!
- Verify the code with `make verify`

## Creating branches in target repositories

Before it will be possible to create a PR for the rebase branch, it will be necessary
to create the new branch in the `openshift/kubernetes` repos. This branch should
contain an unmodified version of Kubernetes at the upstream tag. Similarly, branches
will need to be created in all the staging repos to support publishing to those
repos. If you lack the permissions to perform these actions, ask Michal or Stefan to
perform them for you.

```shell
$ set -e; \
for R in staging/src/k8s.io/*; do \
  R=$(basename $R); \
  SHA=$(git ls-remote git@github.com:kubernetes/$R.git | grep 'kubernetes-1.18.2\^' | cut -f1); \
  echo "$R: $SHA" 1>&2; \
  echo curl -i -H "Authorization: token $TOKEN" -d "{\"ref\":\"refs/heads/origin-4.5-kubernetes-1.18.2\",\"sha\":\"$SHA\"}" https://api.github.com/repos/openshift/kubernetes-$R/git/refs; \
done)
```

## PR Checklists

In preparation for submitting a PR to the [openshift fork of
kubernetes](https://github.com/openshift/kubernetes), the following
should be true:

- [ ] The new rebase branch includes the relevant carries from the previous branch
- [ ] Dependencies have been updated and committed
- [ ] `make update` has been invoked and the results commited
- [ ] `make` executes without error (builds the code)
- [ ] The target rebase branch has been created in the `openshift/kubernetes` repo

Once a PR is submitted, the following steps can be tackled:

- [ ] The rebase branch has passed spreadsheet review
- [ ] `make verify` executes without error
- [ ] `make test` executes without error

### PR CI jobs

As of this writing, CI on `openshift/kubernetes` only runs two jobs: `verify` and
`unit`. They both have known issues that ensure failure, and and an experienced team
member will need to evaluate the job results to be able to identify which failures
are safe and which need further attention.
