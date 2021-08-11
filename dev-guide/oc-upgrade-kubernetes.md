---
title: upgrading-oc-to-latest-version-of-kubernetes-with-gomod
authors:
  - "@soltysh"
reviewers:
  - "@damemi"
  - "@ingvagabund"
  - "@tnozicka"
  - "@sallyom"
approvers:
  - "@mfojtik"
creation-date: 2019-10-08
last-updated: 2021-04-07
status: implementable
see-also:
replaces:
superseded-by:
---

# Upgrading oc to latest version of Kubernetes with gomod

1. Explicitly opt into go modules, even though we are inside a `GOPATH` directory,
   otherwise the default auto mechanism turns on, which when it notices vendor
   directory will turn go modules off.

   ```bash
   export GO111MODULE=on
   ```

2. Get desired version of apimachinery, client-go, cli-runtime, kubectl and kubernetes (eg. 1.18.0).

3. Create branch `oc-A.B-kubernetes-X.Y.Z` (where A.B is the oc version, and X.Y.Z is the kubernetes version)
   in apimachinery, client-go, cli-runtime, kubectl and kubernetes and prime the repos with the basic
   state from k8s (see previous step).

   **Info**: `oc-A.B-kubernetes-X.Y.Z` branches are usually
   pre-created, so it's sufficient to just check the branches already
   exist. Also, all the branches contain the latest changes from
   corresponding k8s repositories (without our carry patches if there
   are any). I.e. just a clean sync with upstream repositories. We
   need those as a base for applying our patches and syncing those to
   oc repository at the end.

   Also, in case you are working with pre-release candidates, all the branches will have corresponding suffix in addition. E.g. `4.8-kubernetes-1.21.0-beta.1`.

4. For each checkout out repository (kubernetes/apimachiner|client-go|cli-runtime|kubectl), add the openshift git remote:
   ```bash
   $ cd local-checkout-of/k8s/<repository>
   $ git remote add openshift git@github.com:openshift/kubernetes-<repo>.git
   $ git fetch openshift
   ```

5. Pick carry patches. From each openshift/kubernetes-\<repository\>, `git log --no-merges --oneline openshift/oc-4.7-kubernetes-1.20.1`
   is a helpful query where `openshift` is the name of the git remote pointing to the last bumped (1.20 in this case) `openshift/kubernetes-repo`
   (replace versions from previous query accordingly). For the `UPSTREAM` commits, you need
   to verify what kind of patches were applied to the last kubernetes bump (1.20.0 here) and create a
   [spreadsheet similar to this](https://docs.google.com/spreadsheets/d/16s4lUbjKdY1yPuqqSoNeS5L53n_u1T0RZYLvwiWK8ak/edit?usp=sharing)
   to decide whether we still need a patch or not.

   **Info**: At the bottom of the spreadsheet, there's a tab for each repository.
   Each tab contains a list of commits that were available in the previous rebase.
   In order to get a new list of commits, run `git log --no-merges --oneline openshift/oc-4.7-kubernetes-1.20.1 | grep UPSTREAM`
   over each repository, copy paste the list to each tab and mark individual commits with proper colors.

6. For each repository (openshift/kubernetes-apimachinery, openshift/kubernetes-client-go, openshift/kubernetes-cli-runtime,
   openshift/kubernetes-kubectl), open A PR with the picked commits from the spreadsheet against the oc-A.B-kubernetes-X.Y.Z branch.

7. In oc repository:
   1. Edit the replace dependencies to point to the commits from the merged PRs from previous steps.
   2. Edit the replace dependencies for all other k8s.io/repos to point to latest release (`v0.21.0-beta.1` here).
      It's useful to add a commit for steps 1,2.
   3. Run `go mod tidy` _then_ `go mod vendor` and verify the changes before committing.
   3. Update kubectl version fields injected in Makefile (using `git describe --long --tags --abbrev=7` in kubernetes fork).
   4. Run `make` and `make test-unit` and fix whatever is broken.
8. Update this document with latest versions, spreadsheet, anything else to help the next bump go smoothly.

## Useful `gomod` commands

* `go mod init` creates a new module, initializing the go.mod file that describes it.
* `go build`, `go test`, and other package-building commands add new dependencies to go.mod as needed.
* `go list -m all` prints the current module’s dependencies.
* `go get` changes the required version of a dependency (or adds a new dependency).
* `go mod tidy` removes unused dependencies.  You should always run this _before_ `go mod vendor`.
* `go mod why -m` and/or `go mod graph` to learn about why a certain version was picked and how/where from

## FAQ

https://github.com/golang/go/wiki/Modules
