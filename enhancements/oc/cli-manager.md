---
title: cli-manager
authors:
  - "@sallyom"
  - "@deejross"
reviewers:
  - "@soltysh"
approvers:
  - "@soltysh"
creation-date: 2021-10-06
last-updated: 2021-10-06
status: implementable
---

# OpenShift CLI Manager

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal is describing the mechanism for how authors of a Command Line Interface (CLI) such as oc, kubectl, odo, istio, tekton, or knative,
can deliver tools to OpenShift clusters.  A feature is needed to manage various CLIs available for OpenShift and related services.  The goal is for
 a connected user to discover, install, and upgrade tools that are compatible with the current cluster version easily and from a single location.

Each component is responsible for building and publishing its artifacts and registering information regarding supplied binaries.
Currently, that location is [index of /pub/openshift-v4/clients](https://mirror.openshift.com/pub/openshift-v4/clients/),
and this makes it difficult for disconnected installations to mirror them.  

`oc` will retrieve binaries from images that package a CLI's artifacts. Currently, we provide disconnected environments 
cli-artifacts and oc download links that do not require anything outside the cluster.  The goal is to provide the same for other
binaries/tools/CLIs. Also, these artifacts will be accessible via `oc` as well as through the console via a fileserver.

`oc` will provide the mechanism to provide, list, install, and upgrade OpenShift supported CLIs compatible with   
a connected cluster. Through a ClusterCLI [Custom Resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/),
each CLI will provide necessary information about its provided artifacts, including its description and location within the image.  The ClusterCLI
for oc, helm, and odo custom resources as well as a CRD controller to manage these will serve as a reference consumer implementation for other CLIs to 
follow. Currently. the `cli-artifacts` image stream in the `openshift namespace` is shipped with the release payload. This is where the artifacts
for `oc` will be extracted from. If other CLIs choose to offer artifacts in this way, they can opt in by supplying an artifacts image to be
shipped with the release payload.

_For this proposal "ClusterCLI" is assumed the name of a CR that will store metadata and information about each CLI_

## Motivation

As more services are created on top of OpenShift, more CLIs are introduced to simplify interaction with these services.
Some current examples are oc, kubectl, odo, istio, tekton, and knative.  It is difficult for users to discover what tools exist, 
where to download them from and which version they should download.  We need to simplify as much as possible the interaction
of services on OpenShift.  We need a mechanism for providing and consuming tools that is simple to add on to as new CLIs are 
developed from a variety of sources - and this should be specific for each cluster and available with disconnected installs.

## Requirements:

1. No new form of binary distribution or binary creation will be proposed, because we have an existing structure at Red Hat.
RPMs or images are the only options, and images must be deployed by the RH pipeline via operators. This proposal is for delivering
CLIs via images, because this will enable offering CLIs offline through mirroring.
2. CLI owners must be able to easily distribute their binaries.
3. The version of the CLI a user is offered is appropriate for the version of the CLI controller installed on the cluster.
4. Arbitrary binaries not delivered by the in-cluster CRD controller are not important or relevant because of requirement 1 and 2.
    - Anyone who wants such tools can download them outside of this mechanism.
5. Users must continue to have the option to download binaries from either the central location where artifacts are published or from the console.
6. CLI author requirements:
    - Provide CLI image containing binaries
    - ClusterCLI Custom Resource that describes the binaries (location within the image, metadata, description)
    - Registering and managing a ClusterCLI Custom Resource through the CRD controller. 
7. `oc` changes:
    - Reading metadata from CR and extracting the binaries from the CRD controller's API to local disk (disconnected)
8. CRD controller for registering CRs with an API for listing, extracting and downloading tools
    - Krew-compatible index that both krew and oc can consume where the download links are for cluster-local resources

### Goals

Each component wishing to provide customers with their binaries will build and publish artifacts via an official channel to a central index.
Each component wishing to provide customers with binaries will create a ClusterCLI custom resource to provide an image name and the file path 
within that image for the binaries. The CLIs currently offered via console-cli-downloads will be included in the reference implementation of this enhancement.
`oc` will gather ClusterCLI from the cluster, and also the fileserver will offer the binaries via the console by mounting the binaries 
from the location/image in each custom resource.

Possible routes for supplying CLI binaries:    

1.  Central index that has links to where artifacts are stored.  This is what we have now with
[the ConsoleCLIDownload CRD](https://github.com/openshift/api/blob/master/console/v1/0000_10_consoleclidownload.crd.yaml).
Currently, a user can run `oc get consoleclidownloads oc-cli-downloads -o yaml` or `oc get consoleclidownloads odo-cli-downloads -o yaml` 
to get a download link for `odo` or `oc`.  A new `oc` command will also enable installing binaries on the user's system, 
in the $HOME directory, that are known compatible with a cluster's version.  The challenge is that we need a mechanism that works in disconnected environments.
Also, this has created a burden of maintenance for the console team.  Furthermore, we want to offer a mechanism that other CLI authors can leverage to deliver tools in-cluster. 
CLI-manager will replace the current consoleclidownloads mechanism and will extend its function.  CLIs will continue to be available from the console.

2.  Central repository where all artifacts are stored - [index of /pub/openshift-v4/clients](https://mirror.openshift.com/pub/openshift-v4/clients/)
currently is where crc, oc, ocp-dev-preview, ocp, odo, and serverless artifacts are published.  This is currently how OpenShift CLIs are published.  The ConsoleCLIDownloads
CRs reference this index.  The challenge is offering these in disconnected environments and automating the download and extraction of the artifacts.  

3.  Images for each CLI, with an extract mechanism for each CLI image, similar to how we currently 
`oc adm release extract --command oc` and `oc image extract`.   A Custom Resource for each CLI would provide information about each CLI and its image.
In disconnected, the images will be available through a mirrored local registry.  The logic for `oc image extract` can be re-used and extended for extracting the CLIs. 
    * CLI owners build images to package artifacts similar to [oc cli-artifacts](https://github.com/openshift/oc/blob/master/images/cli-artifacts/Dockerfile.rhel)
    * CLIs would be managed by users through extending the oc commands we currently have to extract `oc` and `openshift-install` binaries from the release payload.
    This would provide the function we need, but looking to the future there is an upstream effort we can use while also providing 
    a mechanism that the Kubernetes community as a whole can utilize.  This effort is `krew` noted below in Option 4. Also, users will continue to have the option of installing
    artifacts from the console downloads route (the route may be updated to be owned and managed by the cli-manager CRD controller). 

4.  CLIs could be installed in a similar manner to plugins through [krew](https://github.com/kubernetes-sigs/krew).  CLI images would serve the artifacts to enable disconnected downloads, and OpenShift CLIs will be available as 
[krew plugins](https://github.com/kubernetes-sigs/krew-index/tree/master/plugins).
    * Krew and Krew plugins are upstream projects that Kubernetes users are already familiar with
    * A Krew-compatible index can provide available CLIs and plugins for a cluster (similar to the current index) and can also provide an easy mechanism for installing through `oc tools`.  
    * An openshift-krew-index with supported artifacts could hold information about CLIs supported by OpenShift. However, with `krew index add https://someother-third-party-index` 
    we won't limit cluster-admins from adding their own index with whatever plugins they want.  Only the openshift-krew-index will be supported.

Initial implementation will be with Option 4. Each OpenShift CLI will create an image for one or more tools along with a ClusterCLI CR.

Users will continue to have the option to download binaries from either the central location where artifacts are published or from the console.  
This proposal adds the mechanism to install from images built from the same artifacts using `oc`. All options will offer the same binaries, 
ensured through sha256 signatures.  

For the initial implementation, supported CLIs will create ClusterCLI custom resources.  The CLI will be installed to a user's home directory with
the same logic as `oc image extract` and from each CLI's artifacts image (see oc, cli-artifacts image for reference).  
Through mirrored images and ImageContentSourcePolicy, CLIs will be available to disconnected environments. Currently, the console provides
oc binaries by running a webserver pod in the console namespace with a route.  The cli-manager will take over running this webserver, this will
be included in the CRD controller that will create the core ClusterCLI custom resources (oc, odo, and helm, the 3 CLIs currently available 
through the consoleclidownloads mechansism).

### Non-Goals

* `oc tools` will not build or serve the binaries.  It will know where to find them.
* This proposal is not concerned with _which_ binaries will be managed.  This proposal is meant to determine the mechanism only.  Consumers and publishers are clients of the mechanism. 
* `oc tools` will not create or update the ClusterCLI Custom Resources, that will be managed by individual CLIs. However, the CLIs that are 
currently available as `consoleclidownloads`, `oc, odo, and helm`, will be included in the reference implementation of this enhancement.

## Proposal

### User Stories

#### Story 1

As a user, I want a CLI manager for various CLIs available for OpenShift and related services so that I can discover, install and list them
The user will invoke the following commands:

* `oc tools list` will access the connected OpenShift cluster, if connected,  and will retrieve information about available CLIs (ClusterCLI CRs)
* `oc tools install odo` will install `odo` to a user's home directory, as a standalone binary.

##### example:
```
$ oc tools list
NAME                 DESC                        LATEST       INSTALLED
-----                -----                       -----        -----
kubectl              Kubernetes CLI              1.15         1.13
oc                   OpenShift CLI               4.4          4.3
odo                  OpenShift Developer CLI     1.0          Not Installed
kn                   Knative CLI                 1.0          Not Installed
tkn                  Tekton CLI                  1.0          Not Installed

```

#### Story 2

As a user, I want to access various CLIs from the OpenShift Console Web Terminal so that I can use the
services available in the cluster. The CLIs versions should match the those of the operators deployed 
on the cluster.

Currently, users can access `consoleclidownloads` custom resources for helm, oc, and odo either from oc with `oc get consoleclidownloads` or through
the `downloads -n openshift-console` route. This access will still be available, through the updated ClusterCLI custom resources. The ClusterCLI custom
resources can also be accessed through the console by searching for `ClusterCLI` as you would any other resource. The CLI versions that are compatible
with a given release payload will in this way be available to the Console Web Terminal. CLI artifacts will continue to be served via a webserver for users to download
from a route as they are today with the console downloads route. Today, only oc binaries are available with the console downloads route. The other consolclidownloads
custom resources offer the external links to the user. Other CLIs can provide an artifact image to be shipped with the release payload if they wish to offer binaries 
served in this way from an imagestream located within the cluster registry. The CLIS currently available as `consoleclidownloads`, `oc, helm, and odo`, will be
included in the reference implementation of this enhancement.

The deployment of the CRD controller will have an API accessible to the console for future CLI list and download integration in the future.

## Design Detals

Each CLI will provide an image.
Each CLI is responsible for creating a CR to hold metadata.  The CR will serve to deliver the metadata and description
of its deliverable binary.  Initially, CRs will be accessed using `oc tools` command, and `oc tools install <cli>` will extract the binary to a user's home directory.
Users will install OpenShift tools that are known compatible with each cluster version through `oc tools`.    

A CLI must provide a ClusterCLI CR.  The result of this proposal will be:
* ClusterCLI Custom Resource Definition
* an index with artifacts same as [index of /pub/openshift-v4/clients](https://mirror.openshift.com/pub/openshift-v4/clients/) with added listing of CLI images
* mechanism to manage CLIs via ClusterCLI CRs and CLI images
* a reference consumer implementation, a CRD controller to manage supported CLIs that will serve artifacts from CLI artifact image streams. Other CLIs (odo, tkn, kn)
can follow

For making the controller host a krew-compatible API, there are some challenges. A krew index is simply a Git repo, but replicating the output of a git repo from CRs
presents a few challenges as the git HTTP server protocol is not as straightforward as a traditional REST API serving JSON. The CRs would essentially need to be committed
to a temporary git repo which is then served via HTTP by `git-upload-pack`.

#### References
- [Krew: kubectl plugin manager](https://github.com/kubernetes-sigs/krew) - manages kubectl plugins from 
an [index](https://github.com/kubernetes-sigs/krew-index) of all known krew plugins.

- [Git HTTP server protocol](https://www.git-scm.com/docs/http-protocol) - How git servers (i.e. GitHub) work, this is required for krew index compatibilitty.
    
- ["Uc" PoC by Hiram](https://github.com/chirino/uc) - manages Kubernetes CLI clients with an online catalog of releases.  Installs to a user's
home directory, $HOME/.uc/cache  and when the cluster version does not match a known version, will install latest (well 'latest' known for uc oc
atm is 3.11)

_notes on macOS binaries_       
[Signing binaries for macOS Catalina](https://developer.apple.com/news/?id=09032019a)    
[related to above, Go toolchain issue with macOS Catalina](https://github.com/golang/go/issues/34986)    

## Version Skew Strategy

- CLIs are expected to be backwards compatible.  When working with multiple clusters, it's expected that CLI versions will work across cluster versions.
If this is not the case, CLI owners will provide that information in the Custom Resource description.  
- A cluster will supply known-good versions of supported CLIs through cli-manager for a particular cluster, but the expectation is that a CLI version
will be good for multiple cluster versions, and that it is rare for an admin of multiple clusters to require different versions of a single CLI.

## Infrastructure Needed

- CRD controller - for installing ClusterCLI CRD
- CRD and API [example](https://github.com/deejross/openshift-cli-manager/blob/main/config/crd/bases/config.openshift.io_clitools.yaml)
- ClusterCLI controller: [concept](https://github.com/deejross/openshift-cli-manager)
- ClusterCLI CR: [example](https://github.com/deejross/openshift-cli-manager/blob/main/config/samples/vault_clitool.yaml)
- each CLI publish an image to package artifacts [example](https://github.com/openshift/oc/blob/master/images/cli-artifacts/Dockerfile.rhel) 
