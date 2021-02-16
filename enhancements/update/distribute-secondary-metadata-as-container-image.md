---
title: cincinnati-graph-data-package-distribute-and-consume-in-a-container-image-repository
authors:
  - "@steveeJ"
reviewers:
  - @openshift/openshift-team-cincinnati-maintainers
  - @openshift/openshift-team-cincinnati
  - @openshift/team-cincinnati-operator
approvers:
  - @crawford
  - @LalatenduMohanty
  - @sdodson
creation-date: 2020-05-07
last-updated: 2020-05-07
status: provisional
see-also:
-
replaces: []
superseded-by: []
---

# cincinnati-graph-data: package, distribute, and consume in a container image repository

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [x] Graduation criteria
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

### Product name for the CPaaS integration
Which product name should this image be released under?
Further note: the repository for the graph-data images will not be semantically versioned.

## Summary
The [cincinnati-graph-data GitHub repository][cincinnati-graph-data] will be continuously packaged and released in form of a container image by the means of integration with CPaaS.
This image will be available for public consumption, and will be used by the publicly available Cincinnati instances operated by Red Hat.
The image can be transferred and served to Cincinnati instances operated by any party, including but not limited to network-restricted environments.
The source of the container image, namely the cincinnati-graph-data, will remain publicly available and its workflows unaffected by this enhancement.

## Motivation

### Goals
1. The container image will become the primary way of consuming and distributing the content of the cincinnati-graph-data repository.
2. As long as the primary release metadata will be packaged, distributed and consumed in form of container images as well, the transport mechanism required for data consumption by Cincinnati, as well as mirroring and serving its consumed content, will be solely [Docker Registry HTTP API V2][docker-registry-http-api-v2] protocol.


### Non-Goals
1. Changes to the GitHub repository workflow or data format.
2. Implementation details about the streamlining of the mirroring process for primary and secondary release metadata.

## Proposal

### User Stories

#### Content mirroring for Cincinnati deployments
A user might want to mirror all content which is required by Cincinnati.

1. The user sets up a container image registry in the target environment.
2. Optionally, if the target environment does not have internet access, the user downloads the release container images, the desired graph-data container image, and the images' verification signatures to a transfer medium.
3. The user pushes the release, graph-data, and signature images to the target container image registry
4. The user can now configure the Cincinnati instance in the target environment to consume the content from the available container image registry

### Implementation Details/Notes/Constraints

#### CPaaS Build configuration
1. Every change to the master branch of the cincinnati-graph-data results in a new container image containing the unmodified working tree files of the new revision.
  This will allow the plugin to continuously grab the latest version without being informed that a latest version exists.
  The image will contain a [LABEL](https://docs.docker.com/engine/reference/builder/#label) to reference the tag of the signature verification image, in the form `source-revision=$REVISION`.
  The image will be signed and its signature packaged as a separate container image.
  Packaging the signatures separately was chosen because as of the time of writing this enhancement, [Quay does not support Docker Content Trust](https://support.coreos.com/hc/en-us/articles/115000289114-Docker-Content-Trust-support).

2. The image will be signed by an official Red Hat key, and a container image including the signature for the latest image will be produced.

3. The signature verification image will be pushed to the same repository similarly, under the tag `signature-$REVISION`, where `$REVISION` is the full git revision of the graph-data which is contained.

4. The image will be pushed to a publicly available container image registry under a well-known repository name, and it's `latest` tag will be updated to the latest revision.

#### Plugin implementation
A new Cincinnati plugin will be implemented to fetch the container image from a configured container image repository.
The plugin will have the following configuration options:
* registry
* repository
* a tag or a digest. If a tag is specified, the plugin will check if the specified tag changed on each plugin execution, and download the content if that check is positive.
* (optional) An OpenGPG keyring in ASCII-armor representation to, which is used to validate the image signatures
* (optional) Credentials for authentication towards the container image registry
* ()

#### Deprecation of the Git(Hub) scraper plugin
The Git(Hub) scraper plugin will be deprecated in favor of the new plugin. Note that Cincinnati has support to use no cincinnati-graph-data scraper plugin at all, and can be provided the content to the filesystem directly by its operator.

#### Continous Release
The aim is to release the container images continuously as changes
come in to the repository.  Currently there is no support for incoming
webhooks or other means to detect changes on upstream repositories in
CPaaS. It [does support manually triggering the upsptream
poll](https://gitlab.sat.engineering.redhat.com/cpaas/documentation/-/blob/e95073a9d49c49b30dca0d3644889a714e2efe7b/users/midstream/index.adoc#user-content-poll-upstream-sources)
which makes it possible to create a post-submit job on the
repository's CI to trigger a rebuild in CPaaS.

### Risks and Mitigations

### Container image content integrity
The content of the cincinnati-graph-data repository may be altered by the process of being packaged by CPaaS and distributed via the container image registry.

A mitigation strategy is to sign the resulting image in the CPaaS pipeline, publish the signatures, and have Cincinnati verify the image after download.
This is described throughout the [implementation section](#implementation-detailsnotesconstraints).

### cincinnati-graph-data source repository integrity
Not strictly related to this enhancement, is the fact that we are not signing the commits in the source repository in the first place.
By this we effectively trust GitHub not to change the content.

OpenShift architects are aware of this risk and currently do not consider it sufficiently serious to want mitigation.
If we decide to mitigate in the future, having build tooling require commits to be signed by a hard-coded list of authorized maintainers would be one possible approach.


## Design Details

### Test Plan
**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria
**Note:** *Section not required until targeted at a release.*

- Maturity levels - `Developer Preview`, `Red Hat Staging`, `Red Hat Production`, `GA`

#### `Developer Preview`
- New Cincinnati plugin
- Container image registry managed by and dedicated to prow CI
- Integration tests using an image (to-be-determined: verifying against mocked signatures)
- CPaaS pipeline is setup up to automatically build and publish a container image for changes to the master branch of the graph-data repository

#### `Developer Preview` -> `Red Hat Staging`
- Update the Cincinnati deployment template to use the new plugin.

#### Red Hat Staging -> Red Hat Production
- New plugin is successfully deployed to production
- Adjust the Cincinnati Operator to use the new plugin

#### Red Hat Production -> GA
- Document the mirroring process for end-users
- Definitive positive feedback from the ACM team and their use-case

#### Removing a deprecated feature
*Not applicable*

### Upgrade / Downgrade Strategy
*Not applicable*

### Version Skew Strategy
*Not applicable*

## Implementation History

### Initial implementation
The initial implementation involves setting up the CPaaS pipeline, implementing a new Cincinnati plugin which downloads the published image and extracts it to disk for subsequent processing in the plugin chain.
It also involves adjusting the Cincinnati Operator to use the new plugin by default.

## Drawbacks

### Increase OCP release latency
The enhancement introduces the additional latency of the CPaaS pipeline into the OCP release workflow.
Assuming that the pipeline is fully automated, the latency introduced is expected to be at the order of minutes.
This seems to be an acceptable latency with regards to its worst-case scenario, where it's added on top of the duration of blocking an update path via the path of the repository.

## Alternatives
I want to clearly state that none of the alternatives mentioned below have all the same benefits as the enhancement described here.

* Instead of proceeding with this enhancement, we could continue to work with, or rather around, the current implementation of secondary metadata distribution in Cincinnati.

  Without any code changes, this would mean that we document the various ways how the cincinnati-graph-data GitHub repository can be mirrored and provided to Cincinnati, and how to reconfigure the plugin settings to directly read the data from disk.

  This alternative is currently implemented for Kubernetes deployments in the [Cincinnati Kubernetes operator](https://github.com/openshift/cincinnati-operator).
  It ships the graph-data using the [init container pattern](https://www.vinsguru.com/cloud-design-pattern-kubernetes-init-container-pattern/), which brings the graph-data local the machine which runs Cincinnati.
  The cincinnati-operator then configures Cincinnati to read the bare files from the filesystem, and to not scrape the graph-data from a remote server.


* With changing the GitHub scraper plugin to be compatible with the Git protocol instead of GitHub, we could propose to run a Git server for the Cincinnati environment, and document how to configure Cincinnati to scrape secondary metadata from a custom Git server.

## Infrastructure needed
* Public container image repository
* CPaaS build system with push access to the aforementioned container image repository

[cincinnati-graph-data]: https://github.com/openshift/cincinnati-graph-data
[docker-registry-http-api-v2]: https://docs.docker.com/registry/spec/api/
