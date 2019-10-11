---
title: Upgrading oc to latest version of Kubernetes with gomod
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
last-updated: 2019-10-08
status: provisional
see-also:
replaces:
superseded-by:
---

# Upgrading oc to latest version of Kubernetes with gomod

1. Explicitly opt into go modules, even though we are inside a `GOPATH` directory,
   otherwise the default auto mechanism turns on, which when it notices vendor
   directory will turn go modules off.
   ```
   export GO111MODULE=on
   ```
2. Get desired version of apimachinery, client-go, cli-runtime, kubectl and kubernetes (eg. 1.16.0).
3. Create branch `oc-A.B-kubernetes-X.Y.Z` (where A.B is the oc version, and X.Y.Z is the kubernetes version)
   in apimachinery, client-go, cli-runtime, kubectl and kubernetes and prime the repos with the basic
   state from k8s (see previous step).
4. Pick carry patches for appropriate repositories. `git log --no-merges --oneline v1.14.0..openshift/oc-4.2-kubernetes-1.14.0`
   is a handful query (don’t forget to replace versions from previous query accordingly). For those you just need
   to verify what kind of patches were applied to previous version, create a spreadsheet
   (see https://docs.google.com/spreadsheets/d/1LIqRpdSnyTkhD5Dhw8H5QDwd9U98APoZv4lUa5XAfa8/edit for example)
   and decide whether we still need or not a patch.
5. In kubernetes repository:
   1. Add replace dependency to openshift/api and openshift/client-go pointing at latest SHA from that repo, eg.
      ```
      github.com/openshift/api => github.com/openshift/api master
      ```
   2. Run `go list -m all` which will turn above into something like:
      ```
      github.com/openshift/api v3.9.1-0.20190822120857-58aab2885e38+incompatible // indirect
      ```
   3. Copy changes from apimachinery, client-go, cli-runtime and kubectl into staging directory,
      and use git add *.go because we care only about go files.
   4. Run `hack/update-vendor.sh` to pick up openshift dependencies
6. In oc repository:
   1. Update replace dependencies to point to these from previous step.
   2. Run `go mod vendor`, `go mod tidy` and verify the changes before committing.
   3. Update kubectl version fields injected in Makefile (using `git describe --long --tags --abbrev=7` in kubernetes fork).
   4. Run `make` and `make test-unit` and fix whatever is broken.

## Useful `gomod` commands

* `go mod init` creates a new module, initializing the go.mod file that describes it.
* `go build`, `go test`, and other package-building commands add new dependencies to go.mod as needed.
* `go list -m all` prints the current module’s dependencies.
* `go get` changes the required version of a dependency (or adds a new dependency).
* `go mod tidy` removes unused dependencies.
* `go mod why -m` and/or `go mod graph` to learn about why a certain version was picked and how/where from

## FAQ

https://github.com/golang/go/wiki/Modules

