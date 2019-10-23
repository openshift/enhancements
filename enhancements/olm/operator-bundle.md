# Operator Bundle

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary
This enhancement proposes standards and conventions for storing kubernetes manifests and `metadata` associated with an operator as container images in OCI-compliant container registries, and to associate metadata-only images with standard, runnable images.

## Motivation
There is no standard way to associate and transmit operator manifests and metadata between clusters, or to associate a set of manifests with one or more runnable container images.

Existing non-standard methods include:

* git repositories
    * see also, kustomize
* operator-registry directory “bundles”
* helm charts
* appregistry

We would like to be able to talk about a set of metadata and manifests, outside the context of a cluster, as representing a particular application or service (in this case, an operator).

By standardizing on a container format for this data, we get many other features for free, such as: identity, distribution, replication, deduplication, signing, and ingress.

### Goals
* Define a convention for storing operator manifests and metadata with container image.
* Build and push metadata using standard container tooling (e.g.docker cli)
* No union filesystem should be required to consume metadata
* Have a simple mechanism to apply a bundle to a kubernetes cluster

### Non-Goals
* Require OCI registries that support any non-standard media types
* Build on-cluster tooling to interact with bundles

## Proposal
We delineate the operator metadata from the operator manifests. The operator manifests refers to a set of kubernetes manifest(s) the defines the deployment and RBAC model of the operator. The operator metadata on the other hand are, but not limited to:
* Information that identifies the operator, it's name, version etc.
* Additional information that drives the UI: 
    * Icon
    * Example CR(s)
* Channel(s)
* API(s) provided and required. 
* Related images.


This enhancement proposal focuses on the following:
* A standard way to store and transmit manifests and metadata associated with an operator.
* An operator author can specify supporting metadata in a standard and structured manner.
* A single unique identifier that points to a particular version of an operator bundle (both metadata and manifests).

The following user stories discuss the User Experience.

---

### User Stories

#### Build, Push, Pull Operator Bundle
As an operator author, I would like to associate operator manifests and metadata with the container image of my operator.

The focus of this user story is to define a standard to store, transmit, inspect and retrieve operator manifests and metadata. The exact format of the metadata is outside of the scope of this story.

*Constraints*:
* An operator bundle (including both manifests and metadata) should be identifiable using a single versioned identifier. 
* For an operator The metadata can be downloaded independently of the manifest.

### Implementation Details/Notes/Constraints
* The initial implementation target will be Docker v2-2 `manifests`, `manifest-lists`, and docker client support, for maximum compatibility with existing tooling.
* We want the entire operator bundle to be identifiable and retrievable using the same identifier/URL.

#### Docker

##### Build, Push, Pull Operator Bundle Image
We use the following labels to annotate the operator bundle image.
* The label `operators.operatorframework.io.bundle.manifests.v1` reflects the path in the image to the directory that contains the operator manifests.
* The label `operators.operatorframework.io.bundle.metadata.v1` reflects the path in the image to the directory that contains metadata files about the bundle.
* The `manifests.v1` and `metadata.v1` labels imply the bundle type:
    * The value `manifests.v1` implies that this bundle contains operator manifests.
    * The value `metadata.v1` implies that this bundle has operator metadata.
* The label `operators.operatorframework.io.bundle.mediatype.v1` reflects the media type or format of the operator bundle. It could be helm charts, plain kubernetes manifests etc.
* The label `operators.operatorframework.io.bundle.package.v1` reflects the package name of the bundle.
* The label `operators.operatorframework.io.bundle.channels.v1` reflects the list of channels the bundle is subscribing to when added into an operator registry
* The label `operators.operatorframework.io.bundle.channel.default.v1` reflects the default channel an operator should be subscribed to when installed from a registry

The labels will also be put inside a YAML file, as shown below.

*annotations.yaml*
```yaml
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: "registry+v1"
  operators.operatorframework.io.bundle.manifests.v1: "path/to/manifests/"
  operators.operatorframework.io.bundle.metadata.v1: "path/to/metadata/"
  operators.operatorframework.io.bundle.package.v1: "$packageName"
  operators.operatorframework.io.bundle.channels.v1: "alpha,stable"
  operators.operatorframework.io.bundle.channel.default.v1: "stable"
```

*Notes:*
* In case of a mismatch, the `annotations.yaml` file is authoritative because on-cluster operator-registry that relies on these annotations has access to the yaml file only.
* The potential use case for the `LABELS` is - an external off-cluster tool can inspect the image to check the type of a given bundle image without downloading the content.

###### Format
We can use the following values for `mediatype`:
* `registry+v1`: Format used by [Operator Registry](https://github.com/operator-framework/operator-registry#manifest-format) to package an operator.
* `helm`: Can be used to package a helm chart inside an operator bundle.
* `plain`: Can be used to package plain k8s manifests inside an operator bundle.

An operator author can also specify the version of the format used inside the bundle. For example,
```yaml
operators.operatorframework.io.bundle.mediatype.v1: "registry+v1"
```

###### Graph Metadata
An additional point of concern is the fact that the current [operator-registry manifest format](https://github.com/operator-framework/operator-registry#manifest-format) also has a multi-version aggregation file `package.yaml` that describes data about the entire set of releases. Given that we are splitting up these manifests into separate objects, we now need a way to denormalize this data so that it applies only to an individual release. In order to do this, we will create labels on the image and include them in the `annotations.yaml` file. Previously the `package.yaml` file was defined like this:

*package.yaml*
```yaml
packageName: etcd
channels:
- name: alpha
  currentCSV: etcdoperator.v0.9.4
- name: stable
  currentCSV: etcdoperator.v0.9.2
defaultChannel: stable
```

The data here defined a package name to attach to all versions, a default channel that defines what channel a user should subscribe to if they are not opinionated about how they want to subscribe to updates, and information about the head of every channel. Now that this data needs to be defined *per release*, we will make the following changes:

1. Retain the Package Name
2. Instead of defining just the head of individual channels, each release version will explicitly define what channels it is subscribing to.
3. Retain the definition of the default channel. Always trust the latest version of the operator defining the default channel. If a non latest release is added to the index, ignore the default channel.

This results in set of labels that will applied to the image as well as added to the annotations.yaml file:

*annotations.yaml* additions for etcdoperator.v0.9.4
```yaml
operators.operatorframework.io.bundle.package.v1: "etcd"
operators.operatorframework.io.bundle.channels.v1: "alpha"
operators.operatorframework.io.bundle.channel.default.v1: "stable"
```

*annotations.yaml* additions for etcdoperator.v0.9.2
```yaml
operators.operatorframework.io.bundle.package.v1: "etcd"
operators.operatorframework.io.bundle.channels.v1: "alpha,stable"
operators.operatorframework.io.bundle.channel.default.v1: "stable"
```

###### Example of an Operator Bundle that uses Operator Registry Format
This example uses [Operator Registry Manifests](https://github.com/operator-framework/operator-registry#manifest-format) format to build an operator bundle image. The source directory of an operator registry bundle has the following layout.
```
$ tree test
test
├── testbackup.crd.yaml
├── testcluster.crd.yaml
├── testoperator.v0.2.0.clusterserviceversion.yaml
├── testrestore.crd.yaml
└── metadata
    └── annotations.yaml
```

`Dockerfile` for operator bundle
```
FROM scratch

# We are pushing an operator-registry bundle
# that has both metadata and manifests.
LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=etcd
LABEL operators.operatorframework.io.bundle.channels.v1=alpha,stable
LABEL operators.operatorframework.io.bundle.channel.default.v1=stable

ADD ./test/*.yaml /manifests/
ADD test/annotations.yaml /metadata/annotations.yaml
```

Below is the directory layout of the operator bundle inside the image.
```bash
$ tree
/
├── manifests
│   ├── testbackup.crd.yaml
│   ├── testcluster.crd.yaml
│   ├── testoperator.v0.1.0.clusterserviceversion.yaml
│   └── testrestore.crd.yaml
└── metadata
    └── annotations.yaml
```

*Notes:*
* The `/manifests` folder is expected to contain resources that can be applied to the cluster using standard tooling like `kubectl`.
* The `/metadata` folder is expected to contain resources that are not directly `apply`able. It can be used to store supporting metadata associated with the operator.
* The image is not runnable, it is built from `scratch`.


###### UX:
Build, Push and Pull an operator bundle image.
```
docker build -f Dockerfile -t quay.io/test/test-operator:v1 .
docker push quay.io/test/test-operator:v1
docker pull quay.io/test/test-operator:v1
```

A tool can inspect an operator bundle image to determine the bundle type and its format.
```bash
# inspect the format of the operator bundle.
docker image inspect quay.io/test/test-operator:v1 | \
jq '.[0].Config.Labels["operators.operatorframework.io.bundle.mediatype.v1"]'

"registry+v1"
```

### Verify, Run and Test

#### Generate Scaffolding
As an operator author I want to generate the scaffolding resources that are necessary to create an operator bundle. We provide the operator author with tooling to automatically generate the scaffolding.
```bash
$ tree test
test
├── 0.1.0
│   ├── testbackup.crd.yaml
│   ├── testcluster.crd.yaml
│   ├── testoperator.v0.1.0.clusterserviceversion.yaml
│   └── testrestore.crd.yaml

$ cd test

# the following command generates the necessary scaffolding.
$ operator-sdk bundle generate --directory /test/ --package test-operator --channels stable

# output:
#  - test/Dockerfile
#  - test/metadata/annotations.yaml
```

Once the scaffolding is generated the user can do a `docker build` to create an operator bundle image.

#### Validate an Operator Bundle
As an operator author I want to validate an operator bundle image so that I can ensure my operator runs as expected on a cluster.
```bash
$ operator-framework bundle validate --image=quay.io/test/test-operator:v1
```

The validate command will do the following:
* Make sure the image `label` and `annotations.yaml` are appropriately configured. If there is any mismatch, the tool should generate appropriate error message. 
* Verify that the format of the bundle is valid. If the bundle is of `registry` format, we should verify that the bundle conforms to operator-registry standards.

#### Run the Operator from the Bundle Image
As an operator author I want to run my operator directly from the bundle image. Once an operator is packaged into a bundle image, we want to give the author ability to run it using `olm` directly from the bundle image.
```bash
# The following creates an 'Operator' CR managed by olm.
cat <<EOF | kubectl apply -f -
apiVersion: operators.operatorframework.io/v2alpha1
kind: Operator
metadata:
  name: test-operator
spec:
  bundle:
    image: quay.io/test/test-operator:v1
EOF
```

Below is an example of how an operator bundle image can be unpacked to apply the manifests on a cluster.
```bash
$ docker save quay.io/test/test-operator:v1 -o bundle.tar
$ tar -xvf bundle.tar

$ tar -tf bundle.tar 
39d24aee3ad2e8720c12042d5b9ba52ce14a12ed72815a759b41b01b9a8dbc03/
39d24aee3ad2e8720c12042d5b9ba52ce14a12ed72815a759b41b01b9a8dbc03/VERSION
39d24aee3ad2e8720c12042d5b9ba52ce14a12ed72815a759b41b01b9a8dbc03/json
39d24aee3ad2e8720c12042d5b9ba52ce14a12ed72815a759b41b01b9a8dbc03/layer.tar
58b4c261195b83bc0b12b80b63f8e11fb97b5d369aea80ca7cc558793bb507a0.json
7b590145954570b3b3b52db41d4fa8950eefed80fd01c937fb3949b863fe0ede/
7b590145954570b3b3b52db41d4fa8950eefed80fd01c937fb3949b863fe0ede/VERSION
7b590145954570b3b3b52db41d4fa8950eefed80fd01c937fb3949b863fe0ede/json
7b590145954570b3b3b52db41d4fa8950eefed80fd01c937fb3949b863fe0ede/layer.tar
manifest.json
repositories

# list all the image layers
$ cat manifest.json  | jq  -r '.[0].Layers'
[
  "7b590145954570b3b3b52db41d4fa8950eefed80fd01c937fb3949b863fe0ede/layer.tar",
  "39d24aee3ad2e8720c12042d5b9ba52ce14a12ed72815a759b41b01b9a8dbc03/layer.tar"
]

# untar all the image layers, this will give us the content of the bundle.
$ cat manifest.json  | jq  -cr '.[0].Layers | .[]' | xargs -n1 tar -xvf
manifests/
manifests/testbackup.crd.yaml
manifests/testcluster.crd.yaml
manifests/testoperator.v0.9.2.clusterserviceversion.yaml
manifests/testrestore.crd.yaml
metadata/
metadata/annotations.yaml

# apply the manifests to a cluster.
$ kubectl apply -n test -f ./manifests
```
#### Run the Operator from the Bundle Folder
This applies to an `operator-registry` bundle. As an operator author I want to apply a bundle folder directly on the cluster so that:
* I can test my changes.
* I can iterate faster.

```bash
tree test
test
├── 0.1.0
│   ├── testbackup.crd.yaml
│   ├── testcluster.crd.yaml
│   ├── testoperator.v0.1.0.clusterserviceversion.yaml
│   └── testrestore.crd.yaml

$ kubectl -n test apply -f ./test/0.1.0
```

This should (re)install the operator from the bundle in the given namespace.
