---
title: operator-registry
authors:
  - "@kevinrizza"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2019-09-05
last-updated: 2019-10-04
status: provisional
---

# operator-registry

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The purpose of this enhancement is to create a first class storage mechanism for Operators in Kubernetes.

This implementation is designed with commonly used tools in mind (specifically, container images and registries) to provide a format of pushing and pulling yaml metadata needed to install and lifecycle Operators.

At a glance, the intention of this enhancement is to generate a format to define index containers that are equivalent to a "repository" and can be installed on a cluster. These indexes contain a set of references to already published Operators as well as a set of metadata that allows users to discover what can be installed on their cluster and how to install it (selecting a version, understanding what dependencies are required, etc.).

Additionally, once those index containers are defined, this enhancement also intends to drive the OperatorHub/OLM user experience with the metadata in those indexes.

## Motivation

The primary motivation for this enhancement is to replace the current implementation of streaming catalog data from app-registry bundles that exist in OpenShift. These bundles exist as base64 encoded file directories that are stored and referenced by an API. Currently, this is only implemented as a closed source API on Quay.io.

Additionally, creating a scaleable solution that fits the needs of operator developers, cluster owners, and development pipelines is important.

### Goals

  - Have index containers that drive on cluster catalog metadata.
  - Index containers define what Operators can be installed on cluster and how to get the manifests to install them.
  - Drive the Console (embedded Operator Hub) from the indexes
  - On clicking install / creating subscription pull down necessary operator manifest containers + all dependent manifest containers to drive metadata for installation
  - A way to serve that API so that existing apis can be driven by the metadata once it is unpacked
  - Index catalogs can stream updates from external source
  - Remove the need to constantly pull down all metadata associated with the entire catalog on each update to any element of the catalog -- this will also allow the update poll timer to be configurable

### Non-Goals

  - Tools to build the index images or operator manifest images
  - A definition for the operator manifest images or the content they contain

## Proposal

### User Stories

_**Index Container Format**_

As an operator author, I want to have a way of defining where my operator is released in an index of many operators:

- So that I can publish my operator to a wide audience
- So that I can define what my operator’s name and release information is

Acceptance Criteria:

A format is defined so that I can publish index container images to a container registry that are well defined and understood.

**Internal details**:

All the metadata that drives the on-cluster UI and install flow needs to be pulled by the cluster in order to display everything. Therefore it may be better to just build the index as a startable and queriable container that serves the content rather than a list of pointers to other content that needs to be pulled at runtime. There are some reasons to build the index as a simple list, largely stemming from a need for CI and build tools to build these indexes very rapidly. However, if the tools to generate the index are sufficiently well designed then bottlenecks can be avoided.

The format then looks something like the operator-registry database that exists today, but with only the metadata required to drive the UI, UX, and install resolution. The db schema will have a layer of "latest" for each operator name for all the metadata that drives the OpenShift console (descriptions, icon references, etc.). Then all operator image pointers will aggregate just the metadata that drives install and upgrade (version, channels, dependencies, owned crds). To add additional updates to the database, the database just needs to be inserted/batch updated with changes rather than rebuilt from scratch.

This database will be built by tooling that also needs to be created, but is not in scope of this enhancement.

---------------------

_**Serve Operator Database**_

As an Operator developer, I want my Operator to be consumed and made available to clusters.

- So that the metadata attached to my operator can be used to determine what is available to install on a cluster.

Acceptance Criteria:
I can create a registry database that hosts a history of my operator and other developer's operators. This database can be used by openshift clusters.

---------------------

_**Add Indexes to cluster**_

As an OpenShift administrator, I want to have a way of adding an operator index to my cluster

- So that I can make operators available to install onto my cluster

Acceptance Criteria:
When I add an operator index to my cluster, I am able to get information about the operators defined in that index.

Internal detail note:
Create an image type CatalogSource that, when added to a cluster with a reference to an index image, pulls the index manifest down and generates the operator metadata database for that operator.

---------------------

_**Drive Operator Installs and UX**_

As an OpenShift administrator, I want a way to install operators made available to me from my Operator index.
- So that I can view what operators are available to install
- So that I can view metadata about available operators (description, author, etc) on the OpenShift console
- So that I can select a given operator name, version, and channel to subscribe to

Acceptance Criteria:

When I install an operator index onto my cluster, I can use the data that is available on the cluster to drive operator installation and lifecycle.

---------------------

_**Polling Updates**_

As an OpenShift administrator, I want a way to configure a poll timer to ensure my index of operators is up to date and references all of the latest operator bundles.

- So that I can get updates over time
- So that I can know the frequency that those updates will appear

Acceptance Criteria:
When I set the update frequency on an operator index, OLM will poll and ensure that if there is a new version of the index that those changes are pulled and propagated into the cluster.

Internal details:
Polling process should be reviewed by quay team to ensure we scale on quay (although these indexes should work on ANY container image registry)

### Implementation Details

The goal of this enhancement is to replace the app-registry implementation in a way that is usable by operator developers and CI pipeline developers. To do this we will build an index image that is defined from a set of operator bundle images that are released independently and optimized in a way that can be productized by operator authors and pipeline developers.

This implementation makes the assumption that a separate process is built that generates container images that contain individual operator bundles for each version of an operator. The intent of the index image is to aggregate these images that are out of scope of this enhancement. Once that image is built, the image itself is immutable. In order to create a new image, the tooling described here will run off cluster. Once the image is loaded onto the cluster, the only way to load new content is to pull down a new version of that image (either manually or by an automated poll).

The final implementation goal of this enhancement involves this set of steps:

1. Use a reference to a container image (also referred to here as the operator index) rather than app-registry namespaces to expose installable operators on a cluster
2. Build an operator-registry database that serves the data required by OLM to drive install and UI workflows
3. Have a way to optimize the build workflow of operator-registry databases (which currently drive OLM's workflows) from previous versions of the database plus new content (ex. new operator at new version) so that the database need not be built from scratch every time they are created.

To start, let's define what the operator-registry does today. It:

1. Takes a set of manifest files and outputs a database file.
2. Takes a database file and serves a grpc api that serves content from that db
3. Takes a configmap reference, builds a db and serves a grpc api that serves content from that db
4. Takes an app-registry path/namespace reference, builds a db and serves a grpc api that serves content from that db

Below are a set of implementation steps that will add new features to accomplish our goals:

*Update the Operator Registry to insert incremental updates*

Add create db, delete, and batch insert APIs to the model layer of operator-registry.

Add a new set of operator registry commands to utilize those new APIs:

`operator-registry create`
    - inputs: none
    - outputs: empty operator registry database
    
`operator-registry add`
    - inputs: $operatorBundleImagePath, $dbFile
    - outputs: updated database file
    ex: `operator-registry add quay.io/community-operators/foo:sha256@abcd123 example.db`
    
`operator-registry add --batch`
    - inputs: $operatorBundleImagePath, $dbFile
    - outputs: updated database file
    ex: `operator-registry add "quay.io/community-operators/foo:sha256@abcd123,quay.io/community-operators/bar:sha256@defg456" example.db`
    
`operator-registry delete`
    - inputs: $operatorName, $dbFile
    - outputs: updated database file without $operatorName included
    ex: `operator-registry delete bar example.db`

`operator-registry delete-latest`
    - inputs: $operatorName, $dbFile
    - outputs: updated database file without latest version of $operatorName included
    ex: `operator-registry delete-latest foo example.db`

`operator-registry manifest`
    - inputs: $dbFile
    - outputs: file with list of bundle images the index was built on
    ex: `operator-registry manifest example.db`

As a point of context, these operations all output new database files. The intent of creating these commands is to allow the creation of new container images based on historical context of previous versions. These commands will be wrapped in tooling (outside the scope of this enhancement) that will be run as part of build environments or for local development. We are not creating a way for these commands to be run from inside the context of a cluster.

*Reference non latest versions of bundles by image digests*

Currently the operator-registry database has a table to the effect of:

OperatorBundle
    -name: foo
    -bundle: "{$jsonblob}"
    
We will add a field to this table that will reference the bundle image path and, when the operator is not the latest version of the default channel, will not include the bundle blob itself:

OperatorBundle
    -name: foo
    -bundle: NULL
    -bundlePath: "quay.io/community-operators/foo:sha256@abcd123"
    
Additionally, we will update the grpc API that OLM currently uses to get the bundle that if it attempts to pull the non latest version will return null. In that case, it will make a second query to get the bundlePath. Then we will create a pod with that image that will serve the bundle data by writing the data to a configmap that can be read by OLM.

In order to serve that manifest image, we cannot pull the image directly. Instead we will:

1. Create a pod spec that uses an init container to inject a binary into the manifest image.
2. Run the manifest image with that binary that writes to a configmap in a well known location
3. Read the configmap data
4. On return kill the pod
5. Delete the configmap

pod spec:
```
apiVersion: v1
kind: Pod
metadata:
  name: example
spec:
  containers:
  - name: operator-manifest
    image: quay.io/community-operators/foo@sha256:abc123
    command: ['operator-registry', 'serve' '-manifestdir "./manifests/"']
  initContainers:
  - name: init-manifest
    image: quay.io/operator-framework/init-operator-manifest:latest
    command: ['cp', 'operator-registry', '.']
```

We will also need to update operator-registry to include a new `serve` command that knows how to parse the manifest/ folder in the operator bundle image. This command will write to a configmap that OLM will use to get the bundle data.

*Reproducibility*

In order to satisfy the ability to recreate the index as a source of truth the `operator-registry manifest` command can be used. This command, when run against a particular database, will return a list of image SHAs for all of the bundle images the database was built from. This allows the command that built the index to be source controlled so that the index is reproducible. If a user needs to reproduce the index, they can simply run this command to get the json object, then pipe that list to the `operator-registry add` command to regenerate the index from scratch with an empty database.

Also of note, there is the possible concern that the index is not fully reproducible because building the index requires pulling the bundle images it is built upon. If those bundles are gone, then the index cannot be easily recreated. It is also *possible* to commit the sqlite database file to source control, which would allow the index to be rebuilt without the need to pull the bundle images. However, the value of such an image is extremely limited given that the bundles are still required at runtime, so the recommended approach for CI systems is to use `operator-registry manifest` if needed.

*DB Versioning*

Since we are now calling this database schema a first class releasable object, we also need to be able to make changes to the schema over time. Along with that, we will need to be able to migrate to newer schema versions over time. To get started with that, we will update the existing operator-registry database to include a table that defines the current version of the database schema. At that point, the DB format is versioned with schemas.

To support this, we will also add a version to the built operator-registry binary tooling (which currently exists in an unversioned state). Going forward, this tooling will support a set of different versions and handle migrations as needed to the latest schema that the tool's version supports. To go along with this, the runtime GRPC API that serves data to OLM will be versioned with the rest of the tooling (by the nature of the space it exists in) but will remain backwards compatible for API calls.

Any large changes we wish to make (i.e. want to deprecate the current grpc api because we've changed how dependency resolution works) will be indicated via new CatalogSource types (current types are grpc, and configmap/internal). We will also provide a migration strategy along with any such changes.

*Make CatalogSource type for this image*

Finally, once these operator registry images are servable, we will update the catalog source to be able to download these images and serve them.

Additionally, we will update the spec to create a polling field that allows the index to be updated automatically on a configurable poll timer.

```
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: example-catalog
  namespace: olm
spec:
  sourceType: grpc
  image: quay.io/operator-framework/community-operators:
  poll:
    interval: 1m
```

Some open questions about the actual poll implementation: In openshift, for many of the the same reasons that OLM cannot directly pull the operator bundle image, OLM cannot query for new shas on a given tag directly.

In order to accomplish this, we will need to create a deployment process that has an implicit understanding of when new images are available. The initial implementation of this poller will leverage the existing method of building CatalogSources. That is, CatalogSources currently directly generate a pod. We will generate a new pod with an ImagePullPolicy of Always on the scheduled timer and then examine the SHA of the pods image. If the SHA does not match the current pod attached to the CatalogSource, we know that the image is newer and will point the CatalogSource to the new pod.

This concept is also fairly related to ImageStreams in OpenShift. Our current plan is to not leverage that feature given that OLM is a component that is also targeted at plain vanilla kubernetes clusters.

*Operator Bundle Metadata*

Currently, when operator manifests are stored in app registry, every version of the operator is included in each release. In addition to each set of versioned manifests, this blob also contains an aggregate file called `package.yaml` that defines a package name, a default channel to subscribe to, and the head of every channel which defines the set of channels that can be subscribed to. Because no such aggreate concept can exist now that each operator version will exist in a separate operator image, we need to define a set of convetions for the registry to continue to build that index. For now, we will attempt to mitigate this issue by providing a similar set of metadata in every operator bundle image as a set of annotations:

`packageName`: Provides the same function as before. The package name is used to uniquely tie that set of bundles to the update graph.

`channels`: A set of channels that the particular bundle explicitly subscribes to. Since each channel is now an explicit choice, the channel graph is no longer an implicit decision with just the head of the channel defined.

`defaultChannel`: Still defines the default channel that a subscription attaches to. Since this concept still applies to the entire package in aggregate in the index, we will explicitly update this value in the database *only* when the operator being added is the latest version. Otherwise this field will be ignored.

This disassociation is a fairly complex problem that, ideally, will require more significant changes to the way that OLM understands and handles update graphs (for example, the use of the `replaces` field). In a future enhancement, a more comprehensive solution to this set of problems will be discussed. For now, this is an intermediate attempt to retain existing functionality.

#### Outcomes

The outcome of all of this implementation ends up defining two distinct user workflows.

The developer/CI tool workflow: Building operator indexes:
  - Users will be able to build operator indexes that essentially define a new type of catalog to drive the use of operators on cluster. Rather than building these catalogs from the full history of all operators, they will build indexes by addition (or subtraction if needed).
  - Users will use a URI to an operator bundle image that contains a single version of an operator (the implementation details of these are somewhat outside the scope of this enhancement, but to start will essentially contain a manifest directory of the CSV and CRD content of the given operator version) and as a result generate a new index image that contains all the contents of the previous index PLUS the bundle that was specified.
  - This process will drive CI workflows that generate new content available on clusters.

The on cluster workflow:
  - Users will be able to add indexes of operators to their cluster that will contain just enough information to drive the UI of OpenShift and the UX of creating operator subscriptions. They will do this by creating a CatalogSource pointing to that built image.
  - When that catalog is created, OLM's catalog operator will create a pod that runs the index. This index image runs what is essentially a slimmed down operator-registry database that can still drive the same user experience of "what is on the cluster" as is. It will be aggregated by the package server and display all the required content that exists today.
  - When the user clicks "Install" from the operator-hub page or creates a subscription from the terminal, normally OLM queries the operator-registry grpc API for the entire "bundle" field, which is just a serialized JSON string containing all the yaml that was defined for that operator. If the version is not the latest version, that query will return null.
  - If the API returns null, OLM will ask for the bundle image URI from the operator-registry database and create a deployment with that image in order to get the manifests from that bundle. In order to do this, we will use an init container to push a binary into the container that can understand where the manifests are (in a well known path) and push all of those files into a configmap as strings.
  - OLM will read that configmap and create a CSV cr to drive the installation of the operator.


#### CI story

Not scoped out inside this enhancement is the fact that a higher order CLI tool needs to be built that encapsulates the functionality of these new `operator-registry` in order to build the index images. Such a tool will encapsulate the image build process along the lines of:

1. Generate index scratch

2. Generate index from previous index

3. Delete entire operator from index

4. Delete latest operator bundle version from index

In all cases, such a tool will need to generate container images as output based on a set of input commands. Essentially it will generate a dockerfile/image specification, and then run podman/docker build to generate that image. Example dockerfile for an add command:

```
FROM quay.io/redhat/community-operators:latest as builder

COPY exampledb.db exampledb.db

FROM quay.io/operator-registry-builder/builder:latest
COPY /build/bin/operator-registry /operator-registry

RUN operator-registry add "quay.io/community-operators/foo:sha256@abcd123,quay.io/community-operators/bar:sha256@defg456" exampledb.db

ENTRYPOINT ["/registry-server"]
CMD ["--database", "/exampledb.db"]
```

In this way, the primary way that a developer or CI pipeline will interact with these indexes will be to try to "add" a newly built bundle to an index, with the output being a net new index. That index can be stored, passed around, and applied to clusters as needed.

### Risks and Mitigations

Concern:

The current CI story around building index images has the expectation that operator-registry will be able to pull images to get manifest content. It appears as though certain build systems (ex. brew) may not have access to external registries. In that case, the operator-registry command will most likely not be able to fetch images directly.

Mitigation:

We may need to be able to give that CI tooling an option to build the database file itself and commit it to source control. Then when updating the index we would need a way to add manifest files directly to that existing database (through some other version of the `add` command). This is not a recommended method of interacting with the database (given that it will solidify the sqlite db as an API which comes with an entire other set of challenges) but it is a possible workaround for ci system limitations that we may need to explore.

Concern:

Upstream community cannot come up with agreement about what an operator bundle image looks like. 

Mitigation:

In that event, we will do a subset of this work. Instead of the operator-registry taking an image, it will take a local directory that has a set of yaml for the bundle. Additionally, we will not add the imagePath optimization to the registry schema.

## Design Details

### Test Plan

Update operator-registry tests to ensure new APIs and DB update commands work as expected.
Additionally, add e2e tests with mocked index containers to ensure that those workflows work as expected.

### Graduation Criteria

##### Dev Preview -> GA

Ability to build indexes
Ability to apply them to a cluster
Ability to drive UI/UX with indexes

##### Removing a deprecated feature

n/a

### Upgrade / Downgrade Strategy

Net new feature for now. In a future enhancement, we will attempt to enhance this feature further as a way of replacing default operator sources with default index catalog sources.

### Version Skew Strategy

See *DB Versioning* section in implementation details.

## Implementation History

n/a

## Drawbacks

## Alternatives

Alternative described in the risks and management section.

Another alternative is to continue to use app-registry to serve this content (not ideal from a community adoption standpoint).
