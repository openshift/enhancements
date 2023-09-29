---
title: pin-and-pre-load-images
authors:
- "@jhernand"
reviewers:
- "@avishayt"   # To ensure that this will be usable with the appliance.
- "@danielerez" # To ensure that this will be usable with the appliance.
- "@mrunalp"    # To ensure that this can be implemented with CRI-O and MCO.
- "@nmagnezi"   # To ensure that this will be usable with the appliance.
- "@oourfali"   # To ensure that this will be usable with the appliance.
approvers:
- "@sinnykumari"
- "@mrunalp"
api-approvers:
- "@sdodson"
- "@zaneb"
- "@deads2k"
- "@JoelSpeed"
creation-date: 2023-09-21
last-updated: 2023-09-21
tracking-link:
- https://issues.redhat.com/browse/RFE-4482
see-also:
- https://github.com/openshift/enhancements/pull/1432
- https://github.com/openshift/machine-config-operator/pull/3839
replaces: []
superseded-by: []
---

# Pin and pre-load images

## Summary

Provide an mechanism to pin and pre-load container images.

## Motivation

Slow and/or unreliable connections to the image registry servers interfere with
operations that require pulling images. For example, an upgrade may require
pulling more than one hundred images. Failures to pull those images cause
retries that interfere with the upgrade process and may eventually make it
fail. One way to improve that is to pull the images in advance, before they are
actually needed, and ensure that they aren't removed. Doing that provides a
more consistent upgrade time in those environments. That is important when
scheduling upgrades into maintenance windows, even if the upgrade might not
otherwise fail.

### User Stories

#### Pre-load and pin upgrade images

As the administrator of a cluster that has a low bandwidth and/or unreliable
connection to an image registry server I want to pin and pre-load all the
images required for the upgrade in advance, so that when I decide to actually
perform the upgrade there will be no need to contact that slow and/or
unreliable registry server and the upgrade will successfully complete in a
predictable time.

#### Pre-load and pin application images

As the administrator of a cluster that has a low bandwidth and/or unreliable
connection to an image registry server I want to pin and pre-load the images
required by my application in advance, so that when I decide to actually deploy
it there will be no need to contact that slow and/or unreliable registry server
and my application will successfully deploy in a predictable time.

### Goals

Provide a mechanism that cluster administrators can use to pin and pre-load
container images.

### Non-Goals

We wish to use the mechanism described in this enhancement to orchestrate
upgrades without a registry server. But that orchestration is not a goal
of this enhancement; it will be part of a separate enhancement based on
parts of https://github.com/openshift/enhancements/pull/1432.

## Proposal

### Workflow Description

1. The administrator of a cluster uses the `ContainerRuntimeConfig` object to
request that a set of container images are pinned and pre-loaded:

    ```yaml
    apiVersion: machineconfiguration.openshift.io/v1
    kind: ContainerRuntimeConfig
    metadata:
      name: ...
    spec:
      containerRuntimeConfig:
        pinnedImages:
        - quay.io/openshift-release-dev/ocp-release@sha256:...
        - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
        - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
        ...
    ```

1. The machine config operators ensures that all the images are pinned and
pulled in all the nodes of the cluster.

### API Extensions

There are no new object kinds introduced by this enhancement, but new fields
will be added to existing `ContainerRuntimeConfig` objects.

The new fields for the `ContainerRuntimeConfig` object are defined in detail in
https://github.com/openshift/machine-config-operator/pull/3839.

### Implementation Details/Notes/Constraints

Starting with version 4.14 of OpenShift CRI-O will have the capability to pin
certain images (see [this](https://github.com/cri-o/cri-o/pull/6862) pull
request for details). That capability will be used to pin all the images
required for the upgrade, so that they aren't garbage collected by kubelet and
CRI-O.

In addition when the CRI-O service is upgraded and restarted it removes all the
images. This used to be done by the `crio-wipe` service, but is now done
internally by CRI-O. It can be avoided setting the `version_file_persist`
configuration parameter to "", but that would affect all images, not just the
pinned ones. This behavior needs to be changed so that pinned images aren't
removed, regardless of the value of `version_file_persist`.

The changes to pin the images will be done in a `/etc/crio/crio.conf.d/pin.conf`
file, something like this:

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
  name: ...
spec:
  containerRuntimeConfig:
    pinnedImages:
    - quay.io/openshift-release-dev/ocp-release@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
    ...
```

When the new `pinnedImages` field is added or changed the machine config
operator will need to pull those images (with the equivalent of `crictl pull`),
create or update the corresponding `/etc/crio/crio.conf.d/pin.conf` file and ask
CRI-O reload its configuration (with the equivalent of `systemctl reload
crio.service`).

The machine config operator will then will use the gRPC API of CRI-O to run the
equivalent of `crictl pull` for each of the images. When that is completed the
machine config operator will update the new `status.pinnedImages` field of the
rendered machine config:

```yaml
status:
  pinnedImages:
  - quay.io/openshift-release-dev/ocp-release@sha256:...
  - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  - quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  ...
```

### Risks and Mitigations

None.

### Drawbacks

This approach requires non trivial changes to the machine config operator.

## Design Details

### Open Questions

None.

### Test Plan

We add a CI test that verifies that images are correctly pinned and pre-loaded.

### Graduation Criteria

The feature will ideally be introduced as `Dev Preview` in OpenShift 4.X,
moved to `Tech Preview` in 4.X+1 and declared `GA` in 4.X+2.

#### Dev Preview -> Tech Preview

- Availability of the CI test.

- Obtain positive feedback from at least one customer.

#### Tech Preview -> GA

- User facing documentation created in
[https://github.com/openshift/openshift-docs](openshift-docs).

#### Removing a deprecated feature

Not applicable, no feature will be removed.

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

Not applicable, there are no API extensions.

#### Failure Modes

#### Support Procedures

## Implementation History

There is an initial prototype exploring some of the implementation details
described here in this [https://github.com/jhernand/upgrade-tool](repository).

## Alternatives

The alternative to this is to manually pull the images in all the nodes of the
cluster, manually create the `/etc/crio/crio.conf.d/pin.conf` file and manually
reload the CRI-O service.

## Infrastructure Needed

Infrastructure will be needed to run the CI test described in the test plan
above.
