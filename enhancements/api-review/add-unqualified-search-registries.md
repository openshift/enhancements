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

The user can use multiple internal registries to pull images with short names without having to change the image spec every time that the registries' DNS changes. This can be done by configuring the list of `unqualified-search-registries` to reflect the changes in the internal registries names by running `oc edit images.config.openshift.io cluster` and add `UnqualifiedSearchRegistries` under `RegistrySources`. Once this is done, the containerRuntimeConfig controller will roll out the changes to the nodes.

### Implementation Details/Notes/Constraints

Implementing this enhancement requires changes in:
- openshift/api
- openshift/machine-config-operator
- builds
- imagestream imports
- image-registry-pull-through

This is what the `/etc/containers/registries.conf` file currently looks like on the nodes:

```
unqualified-search-registries = ['registry.access.redhat.com', 'docker.io']

[[registry]]
...
```

This is an example of the cluster wide images.config.openshift.io:

```
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

```
unqualified-search-registries = ['reg1.io', 'reg2.io', 'reg3.io']
```

Note: adding a drop-in file at `/etc/containers/registries.conf.d` completely overrides the default `unqualified-search-registries` list from `/etc/container/registries.conf`. This allows the user to set their list in their own priority order without it ever falling back to the default list.

The new list of `unqualified-search-registries` will be the list specified in the drop-in file at `/etc/containers/registries.conf.d`.
When a user runs a pod using an image short name, cri-o/podman/buildah will check `reg1.io`, `reg2.io`, and `reg3.io` for any images matching the short name.

The shared package used by builds to create a registries.conf that matches what is on the node will also have to be updated to handle this new option. Imagestream imports and image-registry-pull-through currently use docker libraries, and handle the configuration of insecure, allowed, and blocked registries separately - we will have to do a similar thing to handle unqualified-search-registries until these components are able move over to using the containers/image library.

Documentations: We will document that we heavily advise against using this feature unless it is absolutely needed. An example case would be when a user has multiple internal registries whose DNS changes frequently, so image short name has to be used in the image spec. We will also document that when you do this, the whole list is overridden and there is no fall back to the default list of `unqualified-search-registries`.

### Risks and Mitigations

Need to ensure that all the components that deal with image management are updated to handle this new option so that they are all on the same page. The `unqualified-search-registries` field already exists in the `/etc/containers/registries.conf` file on all nodes in a cluster. This enhancement just adds the ability for the user to be able to configure this list. An API change for adding the `UnqualifiedSearchRegistry` field to the images.config.openshift.io CR is needed.
Builds, Imagestream imports, and image-registry-pull-through will have to handle `unqualified-search-registries` as well to ensure that we are consistent across all the components that deal with image pull and management.
Also, CRI-O will be doing the image search and will have access to the credentials for all the registries in the list.

## Design Details

### Test Plan

Update the tests that are currently in the MCO to verify that `unqualified-search-registries` have been updated when the cluster wide Image CR is edited to configure this.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

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


