---
title: upgrade-without-registry
authors:
- "@jhernand"
reviewers:
- "@avishayt"
- "@danielerez"
- "@mrunalp"
- "@nmagnezi"
- "@oourfali"
approvers:
- "@sdodson"
- "@zaneb"
- "@LalatenduMohanty"
api-approvers:
- "@sdodson"
- "@zaneb"
- "@deads2k"
- "@JoelSpeed"
creation-date: 2023-06-29
last-updated: 2023-07-26
tracking-link:
- https://issues.redhat.com/browse/RFE-4482
see-also:
- https://github.com/openshift/api/pull/1548
- https://github.com/openshift/machine-config-operator/pull/3839
- https://issues.redhat.com/browse/OCPBUGS-13219
- https://github.com/openshift/cluster-network-operator/pull/1803
replaces: []
superseded-by: []
---

# Upgrade without registry

## Summary

Provide an automated mechanism to upgrade a cluster without requiring an image
registry server, and without requiring OpenShift knowledge for the technicians
performing the upgrades.

## Motivation

All these stories are in the context of disconnected clusters with limited
resources, both in the cluster itself and in the surrounding environment:

- The cluster is not connected to the Internet, or the bandwidth is very limited.
- It isn't possible to bring up additional machines, even temporarily.
- The resources of the cluster are limited, in particular in terms of CPU and memory.
- The technicians responsible for performing the upgrade have little or no OpenShift knowledge.

These clusters are usually installed at the customer site by a partner engineer
collaborating with customer technicians.

Eventually, the cluster will need to be upgraded, and then the technicians will
need tools that make the process as simple as possible, ideally requiring no
OpenShift knowledge.

When OpenShift knowledge is required the technician performing the upgrade will
have the support of the team of engineers that planned and vetted the upgrade.

### User Stories

#### Pre-load and pin upgrade images

As an engineer managing a cluster that has a low bandwidth and/or unreliable
connection to an image registry server I want to pin and pre-load all the
images required for the upgrade so that when I decide to actually perform the
upgrade there will be no need to contact that slow and/or unreliable registry
server.

#### Pre-load and pin custom images

As an engineer managing a cluster I want to be able to pin and pre-load custom
images required to upgrade my own applications.

#### Prepare an upgrade bundle

As an engineer managing a cluster I want to be able to assemble an upgrade
bundle that contains all the artifacts (container images and metadata) needed
to upgrade the OpenShift cluster and my own applications. I want to hand over
this upgrade bundle and the documentation explaining how to use it to the
technicians that will perform the upgrade in a suitable media, for example a
USB stick.

#### Include custom images in the upgrade bundle

As an engineer managing a cluster I want to be able to include in the upgrade
bundle the images required to upgrade my own workloads.

#### Explicitly allow vetted upgrade bundle

As an engineer managing a cluster I want be able to explicitly approve the use
of an upgrade bundle, so that only the bundle that I tested and vetted will be
applied to the cluster.

#### Upgrade a single-node or multi-node clusters using a bundle

As a technician with little or no OpenShift knowledge I want to be able to
upgrade a single-node or multi-node cluster using the upgrade bundle and its
documentation. I can't bring up any additional infrastructure at the cluster
site, in particular I can't bring up an image registry server, neither outside
of the cluster nor inside. I want to plug the USB stick provided by the
engineer in one of the nodes of the cluster and have the rest of the process
performed automatically.

### Goals

Provide an automated and documented mechanism that engineers managing a cluster
can use to pin and pre-load images in order to upgrade a cluster without
requiring a registry server.

### Non-Goals

It is not a goal to not require a registry server for other operations. For
example, installing a new workload will still require a registry server.

## Proposal

### Workflow Description

For all kinds of clusters, connected or disconnected, with or without an
available registry server:

1. The administrator of a cluster uses the OpenShift API to request an upgrade.

1. The upgrade infrastructure of the cluster ensures that all the images
required for the upgrade are pinned and pre-loaded in all the nodes of the
cluster. These images will be un-pinned during the next upgrade.

In addition, for the cases were the cluster is completely disconnected or it
isn't possible to use a registry server:

1. An engineer uses the `oc adm upgrade create bundle ...` tool described in
this enhancement to prepare the upgrade bundle containing all the artifacts
(container images and metadata) that are required to perform the upgrade plus
the images required for the custom workloads, and writes it to a USB stick (or
any other suitable media) that will be handed over to the technicians
responsible for performing the upgrades, together with documentation explaining
how to use it.

1. The technicians receive copies of the USB stick and the corresponding
documentation.

1. The technician goes to the cluster site and uses the upgrade bundle inside
the to perform the upgrade. The documentation will ask the technician to
plug the USB stick in one of the nodes of the cluster and provide simple
instructions to verify that the upgraded has been applied correctly. This step
is potentially repeated multiple times by the same technician for multiple
clusters using the same USB stick or copies of it.

Note that the upgrade bundle should not be specific for a particular cluster,
only for the OpenShift architecture and version. Technicians should be able to
use that package for any cluster with that architecture and version.

### API Extensions

There are no new object kinds introduced by this enhancement, but new fields
will be added to existing `ClusterVersion` and `ContainerRuntimeConfig` objects.

The new fields for the `ClusterVersion` object are defined in detail in the
in https://github.com/openshift/api/pull/1548.

The new fields for the `ContainerRuntimeConfig` object are defined in detail in
https://github.com/openshift/machine-config-operator/pull/3839.

### Implementation Details/Notes/Constraints

The proposed solution is based on pre-loading and pinning all required images
in all the nodes of the cluster before starting the upgrade, and ensuring that
no component requires access to a registry server during the upgrade. For this
to work the following changes are required:

1. No OpenShift component used during the upgrade should use the `Always` pull
policy, as that forces the kubelet and CRI-O to try to contact the registry
server even if the image is already available.

1. No OpenShift component should garbage collect the images required for the
upgrade. This is typically started by the kubelet instructing CRI-O to remove
images.

1. No OpenShift component should explicitly try to contact the registry server
without a fallback alternative.

1. The machine config operator needs to support image pinning, pre-loading and
reloading of the CRI-O service.

1. The cluster version operator needs to orchestrate the upgrade process.

1. The engineer that prepares the upgrade bundle needs a `oc adm upgrade create
bundle` tool to create it.

1. The technician that performs the upgrade needs documentation explaining how
to use the upgrade bundle.

#### Don't use the `Always` pull policy during the upgrade

Some OCP core components currently use the `Always` image pull policy during the
upgrade. As a result, the kubelet and CRI-O will try to contact the registry
server, even if the image is already available in the local storage of the
cluster. This blocks the upgrade.

The catalog operator uses the `Always` pull policy to pull catalog images. It
does so in order refresh catalog images that are specified with a tag. But it
also does it when it pulls catalog images that are specified with a digest. That
should be changed to use the `IfNotPresent` pull policy for catalog images that
are specified by digest.

Most OCP core components have been changed in the past to avoid this use of the
`Always` pull policy. Recently the OVN pre-puller has also been changed (see
this [bug](https://issues.redhat.com/browse/OCPBUGS-13219) for details). To
prevent bugs like this happening in the future and make the solution less
fragile we should have a test that gates the OpenShift release and that
verifies that the upgrade can be performed without a registry server. One way
to ensure this is to run in CI an admission hook that rejects/warns about any spec
that uses the `Always` pull policy.

It would also be useful to have another test that scans for use of this `Always`
pull policy in the source code.

We can control the image pull policy for the OCP payload, but not for
customer-specific images. The OCP upgrade may succeed, but the overall upgrade
process will still be seen as failed from the customer point of view. For
example, in the OpenShift appliance use case both the cluster and the customer
workloads are installed using a temporary registry server that is shutdown
after the installation is complete. If any of those workloads uses the `Always`
pull policy then the OCP upgrade would succeed, but the customer workloads will
not be able to start after the upgrade. To mitagate that risk when a bundle is
used the upgrade mechanism will check if there are pods using the `Always` pull
policy, and will emit a warning if there are any.

#### Don't try to contact the image registry server explicitly

Some OpenShift components explicitly try to contact the registry server without
a fallback alternative. These need to be changed so that they don't do it or so
that they have a fallback mechanism when the registry server isn't available.

For example, in OpenShift 4.1.13 the machine config operator runs the
equivalent of `skopeo inspect` in order to decide what kind of upgrade is in
progress. That fails if there is no registry server, even if the release image
has already been pulled. That needs to be changed so that contacting the
registry server is not required. A possible way to do that is to use the
equivalent of `crictl inspect` instead.

#### MCO should learn to pre-load and pin images

Starting with version 4.14 of OpenShift CRI-O will have the capability to pin
certain images (see [this](https://github.com/cri-o/cri-o/pull/6862) pull
request for details). That capability will be used to pin all the images
required for the upgrade, so that they aren't garbage collected by kubelet and
CRI-O.

Note that pinning images means that kubelet and CRI-O will not remove them, even
if they aren't in use. It is very important to make sure that there is enough
available space for these images, as otherwise the performance of the node may
degrade and it may stop functioning correctly if it runs out of space. The space
should be enough to accommodate tho releases (current running + candidate for
install) as well as workload images and buffer.

The changes to pin the images will be done in a
`/etc/crio/crio.conf.d/pin-upgrade.conf` file, something like this:

```toml
pinned_images=[
  "quay.io/openshift-release-dev/ocp-release@sha256:...",
  "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
  "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
  ...
]
```

The images need to be pre-loaded and the CRI-O service needs to be reloaded
when this configuration changes. To support that a new field will be added to
the `ContainerRuntimeConfig` object:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
  name: pin-upgrade
spec:
  containerRuntimeConfig:
    pinnedImages:
    - quay.io/openshift-release-dev/ocp-release@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    ...
```

When the new `pinnedImages` field is added or changed the machine config
operator will need to pre-load those images (with the equivalent of `crictl
pull`), create or update the corresponding
`/etc/crio/crio.conf.d/pin-upgrade.conf` file and ask CRI-O reload its
configuration (with the equivalent of `systemctl reload crio.service`).

If the `spec.containerRuntimeConfig.imagesDirectory` field is used then it
should contain a dump of a
[docker/distribution](https://github.com/distribution/distribution) registry
server. The machine config operator will first check if there is enough space
available in the node to copy the images to the `/var/lib/containers/storage`
directory. Images in the dump of a registry server are compressed, but in
`/var/lib/containers/storage` they are not, so the machine config operator will
check that there is at least twice the space space used by the registry server
dump. If there is not enough space it will be reported as an error conditions in
the `ClusterVersion` object.

If there is enough disk space then the machine config operator will start a
temporary embedded image registry server in each node of the cluster, listening
in a randomly selected local port, and using a self signed certificate, to
serve the images from the `/var/lib/upgrade/4.13.17-x86_64` directory. It will
then configure the node to trust the self signed certificate and configure
CRI-O to use that temporary image registry server as a mirror for the pinned
images, creating a `/etc/containers/registries.conf.d/pin-upgrade.conf` file
with a content similar to this:

```toml
[[registry]]
prefix = "quay.io/openshift-release-dev/ocp-release"
location = "quay.io/openshift-release-dev/ocp-release"

[[registry.mirror]]
location = "localhost:12345/openshift-release-dev/ocp-release"

[[registry]]
prefix = "quay.io/openshift-release-dev/ocp-v4.0-art-dev"
location = "quay.io/openshift-release-dev/ocp-v4.0-art-dev"

[[registry.mirror]]
location = "localhost:12345/openshift-release-dev/ocp-v4.0-art-dev"
```

The machine config operator will copy the images to the
`/var/lib/containers/storage` directory. To do that it will use the gRPC API of
CRI-O to run the equivalent of `crictl pull` for each of the images. When that
is completed the machine config operator will update the new
`status.pinnedImages` field of the rendered machine config:

```yaml
status:
  pinnedImages:
  - quay.io/openshift-release-dev/ocp-release@sha256:...
  - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  ...
```

We explored several alternatives for the format of the bundle, and using a dump
of a registry server to be the best one.

A plain file copy is feasible only if the target directory is empty and nothing
is using it. That is why we are suggesting to add full support to the
additionalimagestores setting in storage.conf. If that was usable (via
`ContainerRuntimeConfig`) then we could create such an additional directory, copy
the files there and reload CRI-O. The size of the (uncompressed) bundle for
this would be approximately 32 GiB, and it would be there for ever, at least
till the next upgrade, because those additional directories are read only.

It is also possible to copy the images using the equivalent of `skopeo copy
containers-storage:... containers-storage: ...`. That doesn't need need the
permanent additional directory, and doesn't need to shutdown or reload CRI-O,
but it does need those 32 GiB temporarily, while the copy is in progress.

Another possibility is to use the equivalent of skopeo copy `docker://...
containers-storage:...`. That is where the temporary registry comes in place.
The advantage is that the format used by the registry to store the images is
more efficient: it only needs 16 GiB for a release. That reduces the size of
the bundle and the space required in the node.

An improvement over that last possibility is to use the CRI-O gRPC API, the
equivalent of `circtl pull ...`. It doesn't improve performance or reduces the
required size, but it means that there is one less component needed (no need
for skopeo) and reduces the risks: CRI-O will be writing the images to the disk
itself, so no risk of format mismatch.

We aren't ruling out any of the above possibilities, consider them as
implementation details, but we think that the last one is the better overall.

#### CVO will need to orchestrate the upgrade activities

To initiate the upgrade the administrator of the cluster changes the
`ClusterVersion` object is changed like this:

```yaml
kind: ClusterVersion
metadata:
  name: version
spec:
  desiredUpdate:
    version: 4.14.5
```

The cluster operator will first ask the machine config operator to pin and
pre-load the release image, creating a `ContainerRuntimeConfig` object similar
to this:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
  name: pin-upgrade
spec:
  containerRuntimeConfig:
    pinnedImages:
    - quay.io/openshift-release-dev/ocp-release@sha256:...
```

The machine config operator will react to that pinning and pre-loading the
image in all the nodes of the cluster, as described in the previous section.
Once the image is pinned and pre-loaded the cluster version operator will
inspect it and find out the references to the payload images. It will then also
pin and pre-load those images, updating the `ContainerRuntimeConfig` object:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
  name: pin-upgrade
spec:
  containerRuntimeConfig:
    pinnedImages:
    - quay.io/openshift-release-dev/ocp-release@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    ...
```

Once those images are pinned and pre-loaded the upgrade will proceed as usual.

Note that this process will by default work using whatever registry servers are
configured in the cluster. When it isn't possible to use a registry server, the
administrator of the cluster will explicitly configure the cluster version
operator to use an upgrade bundle instead, setting the new
`spec.desiredUpdate.imageSource.sourceType` field to `Bundle` (the default will
be `Registry`):

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  desiredUpdate:
    imageSource:
      sourceType: Bundle
```

The technician that performs the upgrade will receive the USB stick containing
the upgrade bundle and will plug it in one of the nodes of the cluster.

The cluster version operator will be monitoring device events in all the nodes
of the cluster in order to detect when such an USB stick (or any other kind of
media containing an upgrade bundle) has been plugged. To control that a new
`spec.desiredUpdate.imageSource.bundle.detectionMechanism` field will be added
to the `ClusterVersion` object. The default value will be `Manual` so that this
monitoring will be disabled by default. When set to `Automatic` the cluster
version operator will create a new `bundle-monitor` daemon set that will
perform the actual monitoring. When any of the pods in this daemon set detects
a device containing a valid upgrade bundle it will update the status of the
`ClusterVersion` object indicating that the bundle is available.

For example, if the administrator of the cluster wants to enable automatic
detection of upgrade bundles she will add this to the `ClusterVersion` object:

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version
spec:
  desiredUpdate:
    imageSource:
      sourceType: Bundle
      bundle:
        detectionMechanism:
          mechanismType: Automatic
```

For situations where it isn't desirable to monitor device events, or it isn't
possible to plug the USB stick or any other kind of media, we will also add a
new `spec.desiredUpdate.imageSource.bundle.file` field to explicitly indicate
the location of the bundle. When this is used the value will be directly copied
to `status.desired.bundle.file` without creating the `bundle-monitor` daemon
set.

If the administrator wants to disable automatic detection of the upgrade bundle,
and wants to manually copy the file to one of the cluster nodes she will do
something like this:

```yaml
spec:
  desiredUpdate:
    imageSource:
      sourceType: Bundle
      bundle:
        detectionMechanism: Manual
        manual:
          file: /root/upgrade-4.13.7-x86_64.tar
```

If the administrator wants to disable automatic detection, but wants to use a
USB stick anyhow he will do something like this:

```yaml
spec:
  desiredUpdate:
    imageSource:
      sourceType: bundle
      bundle:
        detectionMechanism: Manual
        manual:
          file: /dev/sdb
```

When the `status.desired.bundle.file` field has been populated the cluster
version operator will start to replicate the bundle to the rest of the nodes of
the cluster. To do so it will first disable auto-scaling to ensure that no new
nodes are added to the cluster while this process is in progress. Then it will
start a new `bundle-server` daemon set. Each of the pods in this daemon set
will check if the bundle field exists and contains a valid bundle. If it does
then it will serve it via HTTP for the other nodes of the cluster. If it
doesn't exist then it will do nothing.

Simultaneously with the `bundle-server` daemon set the cluster version operator
will also start a new `bundle-extractor` batch job in each node of the cluster.
Each pod in these jobs will try to read the bundle file from the location
specified in `status.desired.bundle.file`. If that file doesn't exist it will
try to download it from the HTTP server of one of the pods of the
`bundle-server` daemon set; the first that responds with `HTTP 200 Ok`. Once it
has either the file or body of the HTTP response it will extract the contents
of the `metadata.json` file (will always be the first entry of the tarball) to
check that there is space in `/var/lib/upgrade` for the size indicated in the
`size` field. If there is not enough space then it will be reported as en error
condition in the `ClusterVersion` object and the upgrade process will be
aborted.

If the `spec.desiredUpdate.imageSource.bundle.digest` field has been set then
the `bundle-extractor` will calculate the digest of the bundle, and if it
doesn't match it will report it as an error condition in the `ClusterVersion`
object and the upgrade process will be aborted.

If there is enough space the `bundle-extractor` will extract the contents of
the bundle to the `/var/lib/upgrade/4.13.7-x86_64` directory.

Once the digest has been verified and the bundle has been completely extracted
the `bundle-extractor` will update the status of the `ClusterVersion` object to
indicate that the bundle is extracted in that node. This will be done via a new
set of conditions for each node, with types `Extracted` and `Loaded`. For
example, when `node0` and `node2` have completed the extraction of the bundle
but `node1` hasn't, the `ClusterVersion` status will look like this:

```yaml
status:
  desired:
    bundle:
      file: /dev/sdb
      nodes:
        node0:
          conditions:
          - type: Extracted
            status: True
          - type: Loaded
            status: False
          metadata: |
            {
              "version": "4.13.7",
              "arch": "x86_64",
              "release": "quay.io/openshift-release-dev/ocp-release@sha256:...",
              "images": [
                "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
                "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
                ...
              ]
            }
        node1:
          conditions:
          - type: Extracted
            status: False
          - type: Loaded
            status: False
        node2:
          conditions:
          - type: Extracted
            status: True
          - type: Loaded
            status: False
          metadata: |
            {
              "version": "4.13.7",
              "arch": "x86_64",
              "size": "16 GiB",
              "release": "quay.io/openshift-release-dev/ocp-release@sha256:...",
              "images": [
                "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
                "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
                ...
              ]
            }
```

At that point the `bundle-extractor` job will finish.

The `extracted` field for each node will indicate if the bundle extraction
process has been completed.

The `metadata` field for each node will contain the information from the
`metadata.json` file of the bundle. Note that the list of images may be too long
(approximately 180 images) to store it in the status of the `ClusterVersion`
object; it may be convenient to store them in separate configmaps, and have only
the references to those configmaps in the `ClusterVersion` status.

When all the nodes have the bundle extracted (the `Extracted` condition for all
nodes is `True`) the cluster version operator will delete the `bundle-server`
daemon set and verify that the metadata in all nodes (the content of the
`metadata` field) is the same. If there are differences they will be reported
as error conditions in the status of the `ClusterVersion` object and the
upgrade process will be aborted. This is intended to prevent accidents like
having two different USB sticks with different bundles plugged in two different
nodes.

Once the metadata has been validated the cluster version operator will ask the
machine config operator to pin and pre-load the images specified in the
metadata. To do so it will use the new `pinnedImages` and `imagesDirectory`
fields of the `ContainerRuntimeConfig` object:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
  name: pin-upgrade
spec:
  containerRuntimeConfig:
    pinnedImages:
    - quay.io/openshift-release-dev/ocp-release@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    imagesDirectory: /var/lib/upgrade/4.13.7-x86_64
```

The machine config operator will react to that pinning and pre-loading the
images in all the nodes of the cluster, as described in the previous section.

When the all the images are pinned and pre-loaded the cluster version operator
will check the `spec.desiredUpdate.schedule.scheduleType` field of the
`ClusterVersion` object. This field will be used to indicate if the upgrade has
to be started manually later or it if should wait till the user explicitly sets
it to `Immediate`. The default value for that will be `Immediate`. This is
intended for situations where the user wishes to prepare everything for the
upgrade in advance, but wants to perform the upgrade later.

When the value of the `spec.desiredUpdate.schedule.scheduleType` field is
`Immediate` the cluster version operator will trigger the regular upgrade
process.

When the upgrade has completed successfully the cluster version operator will
will start a new `bundle-cleaner` job in each node that will clean all the
artifacts potentially left around by other pieces of the upgrade.  In
particular it will remove the `/var/lib/upgrade/4.13.7-x86_64` directory
created by the `bundle-extractor`.

#### Tool to create the upgrade bundle

The upgrade bundle will be created by an engineer using a new `oc adm upgrade
create bundle` command. This engineer will first determine the target version
number, for example 4.13.7. Note that doing this will probably require access
to the upgrade service available in api.openshift.com. Finding that upgrade
version number is outside of the scope of this enhancement.

The engineer will then need internet access and a Linux machine where she can
run `oc adm upgrade create bundle`, for example:

```bash
$ oc adm upgrade create bundle \
--arch=x86_64 \
--version=4.13.7 \
--pull-secret=/my/pull/secret.txt \
--output=/my/bundle/dir
```

The `oc adm upgrade bundle` command will find the list of image references that
make up the release, doing the equivalent of this:

```bash
$ oc adm release info \
quay.io/openshift-release-dev/ocp-release:4.13.7-x86_64 -o json | \
jq '.references.spec.tags[].from.name'
```

In addition to the release images the tool will also support explicitly adding
custom images. For example:

```bash
$ oc adm upgrade create bundle \
--arch=x86_64 \
--version=4.13.7 \
--pull-secret=/my/pull/secret.txt \
--extra-image=quay.io/my-company/my-workload1 \
--extra-image=quay.io/my-company/my-workload2 \
...
--extra-images-file=/my-copany/my-workloads.txt
...
--output=/my/bundle/dir
```

This is intended for situations where the user wants to use the same upgrade
mechanism for her own images.

The `--extra-image` option will be used to add a single image, and it can be
repeated multiple times.

The `--extra-images-file` option will be used to add a collection of images
specified in a text file, one image per line.

The command will then bring up a temporary image registry server, embedded into
the tool, listening to a randomly selected local port and using a self signed
certificate. It will then start to copy the images found in the previous step to
the embedded registry server, using the equivalent of this for each image:

```bash
$ skopeo copy \
--src-auth-file=/my/pull.secret.txt \
--dst-cert-dir=/my/certs \
docker://quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:... \
docker://localhost:12345/openshift-release-dev/ocp-v4.0-art-dev@...
```

When all the images have been copied to the temporary image registry server it
will be shutdown.

The result will be a directory containing approximately 180 images and requires
16 GiB of space. The command will then create a `upgrade-4.13.7-x86_64.tar` tar
file containing that directory and a `metadata.json` file.

The `metadata.json` file will contain additional information, in particular the
architecture, the version, the size and the list of images:

```json
{
  "version": "4.13.7",
  "arch": "x86_64",
  "size": "16 GiB",
  "release": "quay.io/openshift-release-dev/ocp-release@sha256:...",
  "images": [
    "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
    "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
    ...
  ]
}
```

The `metadata.json` file will always be the first entry of the tar file, to
simplify operations that need the metadata but not the rest of the contents of
the tar file.

The command will also write a `upgrade-4.13.7-x86_64.sha256` file containing a
digest of the complete tar file. This digest is intended to protect the
integrity of the bundle. It can be checked with tools like `sha256sum`. It will
also be optionally used by the administrator of the cluster to ensure that the
right bundle is used in the right cluster. To do so the administrator of the
cluster will write it to the `desiredUpdate.imageSource.bundle.digest` field of
the `ClusterVersion` object. The cluster version operator will reject the
bundle if it doesn't match this digest.

The engineer will write the tar file to some kind of media and hand it over to
the technicians, together with the documentation explaining how to use it.

#### Documentation to use the bundle

This documentation shouldn't assume previous OpenShift knowledge, and should be
basic instructions to plug the USB stick containing the bundle and then check
if upgrade the upgrade succeeded or failed, using either the cluster console or
the `oc` tool.

### Risks and Mitigations

The proposed solution will require space to store the release bundle and all the
release images in all the nodes of the cluster, approximately 48 GiB in the
worst case. To mitigate that risks the components that will consume disk space
will check in advance if the required space is available.

### Drawbacks

This approach requires non trivial changes to the cluster version operator and,
to a lesser degree, to the machine config operator.

## Design Details

### Open Questions

None.

### Test Plan

We should have at least tests that verifies that the upgrade can be performed in
a fully disconnected environment, both for a single node cluster and a cluster
with multiple nodes. These tests should gate the OCP release.

It is desirable to have another test that scans the OCP components looking for
use of the `Always` pull policy. This should probably run for each pull request
of each OCP component, and prevent merging if it detects that the offending pull
policy is used. We should consider adding admission in CI for this.

### Graduation Criteria

The feature will ideally be introduced as `Dev Preview` in OpenShift 4.X,
moved to `Tech Preview` in 4.X+1 and declared `GA` in 4.X+2.

#### Dev Preview -> Tech Preview

- Ability to upgrade a single-node clusters using the `imageSource.sourceType:
Registry` mode (no bundle, just pinning and pre-loading of the images).

- Availability of the tests that verify the upgrade if single-node clusters.

- Availability of the tests that verify that no OCP component uses the `Always`
pull policy.

- Obtain positive feedback from at least one customer.

#### Tech Preview -> GA

- Ability to manually detect update bundles.

- Ability to upgrade single-node and multi-node clusters using the
`imageSource.sourceType: Bundle` mode (with a bundle and pinning and
pre-loading the images).

- Ability to create bundles using the `oc adm upgrade create bundle` command.

- Availability of the tests that verify the upgrade in single-node and
multi-node clusters.

- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

#### After GA

- Ability to automatically detect update bundles.

#### Removing a deprecated feature

Not applicable, no feature will be removed.

### Upgrade / Downgrade Strategy

There are no additional considerations for upgrade or downgrade. The same
considerations that apply to the cluster version operator in general will also
apply in this case.

### Version Skew Strategy

This feature will only be usable once the cluster version operator and the
machine config operator have been upgraded to support it. That upgrade will have
to be done by other means.

For subsequent upgrades we will ensure that the cluster version operator can
work with both the old and the new version of the machine config operator.

### Operational Aspects of API Extensions

Not applicable, there are no API extensions.

#### Failure Modes

#### Support Procedures

## Implementation History

There is an initial prototype exploring some of the implementation details
described here in this [https://github.com/jhernand/upgrade-tool](repository).

## Alternatives

The alternative to this is to make a registry server available, either outside
or inside the cluster.

An external
[https://cloud.redhat.com/blog/introducing-mirror-registry-for-red-hat-openshift](registry
server) is the currently supported solution for upgrades in disconnected
environments. It doesn't require any change, but it is not feasible in most of
the target environments due to the resource limitations described in the
[motivation](#motivation) section of this document.

An internal registry server, running in the cluster itself, is a feasible
alternative for clusters with multiple nodes. The registry server supported by
Red Hat is Quay. The disadvantages of Quay is that it requires additional
resources that are often not available in the target environments.

For single node clusters an internal registry server isn't an alternative
because it would need to be continuously available during and after the upgrade,
and that isn't possible if the registry server runs in the cluster itself.

## Infrastructure Needed

Infrastructure will be needed to run the tests described in the test plan above.
