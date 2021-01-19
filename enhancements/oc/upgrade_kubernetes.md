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
last-updated: 2020-04-02
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

   Also, in case you are working with pre-release candidates, all the branches will have corresponding suffix in addition. E.g. `oc-4.5-kubernetes-1.18.0-beta.2`.

4. Open a PR in openshift/release to add this new branch of openshift/kubernetes, [similar to this](https://github.com/openshift/release/pull/7582).
   Usually, you can copy the last version's `release/ci-operator/config/openshift/kubernetes/*` file to a new file that reflects the new branch name,
   verify the go version used, then `make jobs` to generate the new job.

5. Pick carry patches. From each openshift/kubernetes-repository, `git log --no-merges --oneline openshift/oc-4.5-kubernetes-1.18.0`
   is a handful query where `openshift` is the name of the remote pointing to `openshift/kubernetes-repo`
   (replace versions from previous query accordingly). For the `UPSTREAM` commits, you need
   to verify what kind of patches were applied to the last kubernetes bump (1.17.0 here) and create a
   [spreadsheet similar to this](https://docs.google.com/spreadsheets/d/1caKr7-FGn14H2P8UZ6K7wrjcqwQIRuEA_nQR8X4iKJ0/edit#gid=1354624919)
   to decide whether we still need a patch or not.

   **Info**: At the bottom of the spreadsheet, there's a tab for each repository.
   Each tab contains a list of commits that were available in the previous rebase.
   In order to get a new list, run `git log --no-merges --oneline openshift/oc-4.5-kubernetes-1.18.0 | grep UPSTREAM`
   over each repository, copy paste the list to each tab and mark individual commits with proper colors.

6. For each repository (apimachinery, client-go, cli-runtime, kubectl), open A PR with the picked commits from the
   spreadsheet against the oc-A.B-kubernetes-X.Y.Z branch.

7. In openshift/kubernetes repository, check out the new oc-A.B-kubernetes-X.Y.X branch and:
   1. Add the replace dependency for openshift/api and openshift/client-go pointing at latest SHA from that repo, eg.
      ```text
      github.com/openshift/api => github.com/openshift/api master
      ```
   2. Run `go list -m all` which will turn above line in go.mod into something like:
      ```text
      github.com/openshift/api v3.9.1-0.20190822120857-58aab2885e38+incompatible // indirect
      ```
   3. Copy and paste the changes from apimachinery, client-go, cli-runtime and kubectl PRs above into kubernetes/staging/src/k8s.io/ directory,
      and use git add *.go because we care only about go files.  This is a manual step, You can use curl like so for each file to copy file
      changes from your PRs above:
      ```console
      cd staging/src/k8s.io/repo
      curl -o path/to/file https://raw.githubusercontent.com/.....
      ```
   4. Run `hack/update-vendor.sh` to pick up openshift dependencies
   5. Commit the changes, then open a PR against the openshift/kubernetes oc-A.B-kubernetes-X.Y.Z branch.
      Confirm the openshift/release change from step 4 is merged and the unit test is triggered in your PR.
8. In oc repository:
   1. Edit the replace dependencies to point to the commits from the merged PRs from previous steps.
   2. Edit the replace dependencies for all other k8s.io/repos to point to latest release (`release-1.18` here).
      It's useful to add a commit for steps 1,2.
   3. Run `go mod tidy` _then_ `go mod vendor` and verify the changes before committing.
   3. Update kubectl version fields injected in Makefile (using `git describe --long --tags --abbrev=7` in kubernetes fork).
   4. Run `make` and `make test-unit` and fix whatever is broken.
9. Update this document with latest versions, spreadsheet, anything else to help the next bump go smoothly.

## Useful `gomod` commands

* `go mod init` creates a new module, initializing the go.mod file that describes it.
* `go build`, `go test`, and other package-building commands add new dependencies to go.mod as needed.
* `go list -m all` prints the current module’s dependencies.
* `go get` changes the required version of a dependency (or adds a new dependency).
* `go mod tidy` removes unused dependencies.  You should always run this _before_ `go mod vendor`.
* `go mod why -m` and/or `go mod graph` to learn about why a certain version was picked and how/where from

## FAQ

https://github.com/golang/go/wiki/Modules
