---
title: add-unqualified-search-registries
authors:
  - "@umohnani8"
reviewers:
  - "@mrunalp"
  - "@derekwaynecarr"
approvers:
  - "@mrunalp"
  - "@derekwaynecarr"
creation-date: 2020-06-16
last-updated: 2020-06-16
status: implementable
---

# Add UnqualifiedSearchRegistries to cluster wide Image CR

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary

Today, the cluster-wide Image CR (images.config.openshift.io cluster) doesn't have a way of setting the registry search order when pulling an image using short name.

With the move to v2 /etc/containers/registries.conf, there is a new
option called `unqualified-search-registries`. This is the list of registries that the container tools will check when pulling an image using its short name.

## Motivation

Today, when a user wants to pull images using short names, they have to create two `machineConfigs` (one for the master pool and one for the worker pool) that create a file under `/etc/containers/registries.conf.d` with the changes they want for `unqualified-search-registries`.

Users may have multiple internal registries that they use for pulling images. The DNS of these registries could change, and that would require changing the image spec to match this, which can be tedious. Using `unqualified-search-registries` to be able to configure the list of registries to search from allows the user to use short names for image pulls.

Users can currently configure `AllowedRegistries`, `BlockedRegistries`, and `InsecureRegistries` in the cluster wide Image CR. Adding the `unqualified-search-registries` option to this cluster wide Image CR will make short name configuration for users simple and have all the image related configurations in one CR.


### Goals

- Enable the user to be able to set `unqualified-search-registries` in the cluster wide Image CR.

### Non-Goals

- Builds and Imagestream Imports will not support the use of short
  names. There is an added complexity for these components to figure
  out the correct credentials needed for the short names and the
  security risks associated with using short names. Since we highly
  discourage short names, we are only adding support for
  `unqualified-search-registries` at the runtime level. Users can only
  use short names in their pod spec and only cri-o/podman will support
  it.

## Proposal

The Image API is extended by adding an optional `UnqualifiedSearchRegistries` field with type `[]string` to `RegistrySources`:

```go
// RegistrySources holds cluster-wide information about how to handle the registries config.
type RegistrySources struct {
    // ...

	// unqualifiedSearchRegistries are registries that will be searched when pulling images using short names.
	// +optional
	UnqualifiedSearchRegistries []string `json:"unqualifiedSearchRegistries,omitempty"`
}
```

The containerRuntimeConfig controller in the MCO already watches the cluster-wide images.config.openshift.io CR for the allowed, blocked, and insecure registries. It will now watch for `unqualified-search-registries` as well and update `/etc/containers/registries.conf.d` accordingly.

An example images.config.openshift.io CR will look like:
```yaml
apiVersion: config.openshift.io/v1
kind: Image
metadata:
  name: cluster
spec:
  registrySources:
    unqualifiedSearchRegistries:
    - "reg1.io"
    - "reg2.io"
    - "reg3.io"
```

### User Stories

#### As a user, I would like to use image short names when running my workloads

The user can set the `unqualified-search-registries` with a list of registries to check when pulling an image using short names.
The user can run `oc edit images.config.openshift.io cluster` and add `UnqualifiedSearchRegistries` under `RegistrySources`. Once this is done, the containerRuntimeConfig controller will roll out the changes to the nodes.

#### As a user, I would like to use multiple internal registries to pull my images

The user can use multiple internal registries to pull images with
short names without having to change the image spec every time that
the registries' DNS changes. This can be done by configuring the list
of `unqualified-search-registries` to reflect the changes in the
internal registries names by running `oc edit
images.config.openshift.io cluster` and add
`UnqualifiedSearchRegistries` under `RegistrySources`. Once this is
done, the containerRuntimeConfig controller will roll out the changes
to the nodes.

### Implementation Details/Notes/Constraints

Implementing this enhancement requires changes in:
- openshift/api
- openshift/machine-config-operator

This is what the `/etc/containers/registries.conf` file currently looks like on the nodes:

```toml
unqualified-search-registries = ['registry.access.redhat.com', 'docker.io']

[[registry]]
...
```

This is an example of the cluster wide images.config.openshift.io:

```yaml
apiVersion: config.openshift.io/v1
kind: Image
metadata:
  name: cluster
spec:
  registrySources:
    unqualifiedSearchRegistries:
    - "reg1.io"
    - "reg2.io"
    - "reg3.io"
```

The above Image CR will create a drop-in file at `/etc/containers/registries.conf.d` on each file, which will look like:

```toml
unqualified-search-registries = ['reg1.io', 'reg2.io', 'reg3.io']
```

Note: adding a drop-in file at `/etc/containers/registries.conf.d` completely overrides the default `unqualified-search-registries` list from `/etc/container/registries.conf`. This allows the user to set their list in their own priority order without it ever falling back to the default list.

The new list of `unqualified-search-registries` will be the list specified in the drop-in file at `/etc/containers/registries.conf.d`.
When a user runs a pod using an image short name, cri-o/podman/buildah will check `reg1.io`, `reg2.io`, and `reg3.io` for any images matching the short name.

Documentations: We will document that we heavily advise against using
this feature unless it is absolutely needed due to the security
risks. An example case would be when a user has multiple internal
registries whose DNS changes frequently, so image short name has to be
used in the image spec. We will also document that when you do this,
the whole list is overridden and there is no fall back to the default
list of `unqualified-search-registries`. We will also document that
the `unqualified-search-registries` list will not work with the builds
and imagestream imports components. It will only work with the pod
spec when using short names.

### Risks and Mitigations

The `unqualified-search-registries` field already exists in the `/etc/containers/registries.conf` file on all nodes in a cluster. This enhancement just adds the ability for the user to be able to configure this list. An API change for adding the `UnqualifiedSearchRegistry` field to the images.config.openshift.io CR is needed and updates to the MCO code to handle this new option will be needed..
CRI-O will be doing the image search and will be using the cluster wide pull secret to get the credentials for all the registries in the list.

We will document a big warning about the security risks of using short names. This includes the following:

- Users are subject to network and reistry-originating attacks when using external registries.
- If the pod pull secret contains docker.io credentials, these credentials will be sent to all the registries in the search list.
- Only the cluster-wide pull secret can be used for credentials and the namespaced docker.io credentials available to the kubelet.

Customers should only use this when they are using internal registries to reduce the possible security risks affecting them.

## Design Details

### Test Plan

Update the tests that are currently in the MCO to verify that `unqualified-search-registries` have been updated when the cluster wide Image CR is edited to configure this.

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

Upgrades and Downgrades should not be affected. The `unqualified-search-registries` field already exists in the `/etc/containers/registries.conf` file. We are just exposing this option to the user so that they can configure it through the cluster wide Image CR.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

## Alternatives
