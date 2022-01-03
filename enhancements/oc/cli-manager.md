---
title: cli-manager
authors:
  - "@sallyom"
  - "@deejross"
reviewers:
  - "@soltysh"
  - "@jwmatthews"
approvers:
  - "@soltysh"
  - "@sferich888"
  - "@deads2k"
  - "@spadgett"
api-approvers:
  - "@deads2k"
  - "@soltysh"
creation-date: 2021-10-06
last-updated: 2021-10-18
tracking-link:
  - https://issues.redhat.com/browse/WRKLDS-169
see-also:
  - "/enhancements/installer/component-selection.md"
---

# OpenShift CLI Manager

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal is describing the mechanism for how authors of a Command Line Interface (CLI) tool such as `oc`, `kubectl`, `odo`, `istio`, `tekton`, or `knative`,
can deliver tools to OpenShift clusters in disconnected environments.  A feature is needed to manage various CLIs available for OpenShift and related services. The goal is for
a connected user to discover, install, and upgrade tools that are compatible with the current cluster version easily and from a single location.

`krew` is an upstream project to distribute CLI tools (plugins) to Kubernetes users today.
It works by reading a Git repository of files describing the plugins,
and providing download links to them for various different OS and architecture combinations.
Since those download links in the default index are Internet-facing, a Git and file server would need to be
setup by customers to create their own custom index for use in disconnected environments.

In order to avoid creating a new protocol and tool for this functionality, this proposal aims to leverage the `krew` project and a new custom index feature provided by `krew` in the form of a new Controller. The index will be managed by cluster-admins using
[Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/) (or CRs). An image registry will be used to host images that contain the binaries.

By leveraging an image registry, which is an existing dependency for OpenShift, and allowing the index to be managed
within an OpenShift cluster in the form of CRs, we can remove the need for customers to create additional supporting infrastructure (Git and file servers) in order to use this functionality.

_For this proposal "Plugin" is the assumed name of the CR that will store metadata and information about each krew plugin, and "Controller" will refer to the Custom Resource Definition (CRD) controller_

## Motivation

As more services are created on top of OpenShift, more CLIs and plugins are introduced to simplify interaction with these services.
Some current examples are `oc`, `kubectl`, `odo`, `istio`, `tekton`, and `knative`.  It is difficult for users to discover what tools exist,
where to download them from and which version they should download. We should simplify as much as possible the interaction
of services on OpenShift. We need a mechanism for providing and consuming tools that is simple to add on to as new plugins are
developed from a variety of sources - and this should be specific for each cluster and available with disconnected installs.

### Goals

* No new form of binary distribution or binary creation will be proposed, because we have an existing structure at Red Hat.
RPMs or images are the only options, and images must be deployed by the RH pipeline via operators. This proposal is for delivering
plugins via images, because this will enable offering plugins offline through an existing image registry.
* Plugin owners must be able to easily distribute their binaries
* Allow cluster-admins to control which plugins are offered to users
  * This proposal is not concerned with _which_ plugins will be managed, as that is decided by cluster-admins
* Controller for registering CRs with an API for listing, extracting and downloading plugins
  * An API that generates an index that `krew` can consume via its custom index feature
  * The index and binaries will served from a single route and service within the cluster
* The `krew` client package will be vendored into `oc` for usage as `oc krew install <plugin>`

### Non-Goals

* The Controller will not build or package plugins
* No recommending/limiting which plugins are served by the API
* `krew` will not create or update the Plugin CRs, those will be managed by cluster-admins

## Proposal

Each component wishing to provide customers with their plugins will build and publish images via a trusted image registry
and create a Plugin CR to provide an image name and the file path within that image for the binaries.
Clients (i.e. `krew`) will read the index and download binaries from the Controller. The Controller is responsible for building the index from CRs, for pulling the images,
extracting the binaries, and serving them to clients.

* `krew` and `krew` plugins are upstream projects that Kubernetes users are already familiar with
* A `krew`-compatible custom index can provide available plugins for a cluster in disconnected environments
* The index will be served by a Controller with its contents managed by cluster-admins via CRs
* The binaries will also be served by the Controller that will pull images from a trusted image registry and extract the binaries
* With `krew index add https://someother-third-party-index` we won't limit users from adding their own index with whatever plugins they want
* There are two possible approaches for this new controller:
  1. Controller should be installed by default:
  * Pros for being installed by default:
    * Available day one for all clusters
    * All binaries can be served using single unified mechanism
    * No user setup required to use in disconnected environments
    * Replace existing, temporary download mechanism
  * Cons:
    * Additional controller running by default
  2. Controller being optional:
  * Pros for being optionally installed:
    * Less resource utilization
  * Cons:
    * Another operator to manage and mirror for disconnected environments
    * Separate mechanism for built-in and third-party binaries
    * Missing plugin CRs for optional elements installed before controller is installed
    * Won't be able to replace existing, temporary system used by console which is serving some binaries
* Given above arguments it is reasonable to include the controller by default
* As `krew` itself is a `kubectl` plugin, it can be invoked using either using `kubectl krew` or `oc krew`\
* `krew` functionality baked into `oc` by default

Existing methods of downloading binaries (i.e. the console) will not be affected by this proposal. For the initial implementation, supported plugins will create Plugin CRs. The plugins will be downloaded and installed using `krew`.
This transition should be transparent for consumers.

### User Stories

#### Story 1

As a user, I want a CLI manager for various CLIs and plugins available for Kubernetes/OpenShift and related services so that I can discover, install, and list them. If `odo` was made available by a cluster-admin, I could install it using:
* `oc krew install odo`

I could then interact with `odo`:
* `oc odo --help`

##### Example
```text
$ oc krew search odo
NAME                 DESC                        LATEST       INSTALLED
-----                -----                       -----        -----
odo                  OpenShift Developer CLI     1.0          Not Installed
```

#### Story 2

As owner of a CLI or plugin, I want to publish it to users of the cluster. I need to create a Plugin CR for my tool, or provide the required information about my tool
to a cluster-admin for the creation of a CR:
* Name
* Short description
* Long description
* Caveats
* Homepage
* Version
* Platform/architecture
* The image:tag (and registry credentials if required)
* The paths within the image where the binary for the given platform/architecture can be found

### API Extensions

A new CRD will be generated based on the types:
* API: `config.openshift.io/v1`
* Kind: `Plugin`

```go
// PluginSpec defines the desired state of Plugin.
type PluginSpec struct {
	// ShortDescription of the plugin.
	// +required
	ShortDescription string `json:"shortDescription"`

	// Description of the plugin.
	// +optional
	Description string `json:"description,omitempty"`

	// Caveats of using the plugin.
	// +optional
	Caveats string `json:"caveats,omitempty"`

	// Homepage of the plugin.
	// +optional
	Homepage string `json:"homepage,omitempty"`

	// Version of the plugin.
	// +required
	Version string `json:"version"`

	// Platforms the plugin supports.
	// +required
	Platforms []PluginPlatform `json:"platforms"`
}

// PluginPlatform defines per-OS and per-Arch binaries for the given plugin.
type PluginPlatform struct {
	// Platform for the given binary (i.e. linux/amd64, darwin/amd64, windows/amd64).
	// +required
	Platform string `json:"platform"`

	// Image containing plugin.
	// +required
	Image string `json:"image"`

	// ImagePullSecret to use when connecting to an image registry that requires authentication.
	// +optional
	ImagePullSecret string `json:"imagePullSecret,omitempty"`

	// Files is a list of file locations within the image that need to be extracted.
	// +required
	Files []FileLocation `json:"files"`

	// Bin specifies the path to the plugin executable.
	// The path is relative to the root of the installation folder.
	// The binary will be linked after all FileOperations are executed.
	// +required
	Bin string `json:"bin"`
}

// FileLocation specifies a file copying operation from plugin archive to the
// installation directory.
type FileLocation struct {
	// From is the absolute file path within the image to copy from.
	// Directories and wildcards are not currently supported.
	// +required
	From string `json:"from"`

	// To is the relative path within the root of the installation folder to place the file.
	// +required
	To string `json:"to"`
}
```

### Risks and Mitigations

Distributing undesirable binaries is always a risk. Some mitigations include requiring cluster-admins to maintain the index, and the verification of downloaded
binaries using SHA256 hashes. Cluster-admins are responsible for publishing only trusted binaries.

## Design Details

Each plugin will provide an image.
Each plugin is responsible for creating a CR to hold metadata.  The CR will serve to deliver the metadata and description
of its deliverable binary. The Controller will use CRs to generate an index for the `oc krew search` command, and `oc krew install <name>` will download the binary from the Controller.
Users will install OpenShift tools that are known compatible with each cluster version through `oc krew`.

A plugin must provide a Plugin CR. The result of this proposal will be:
* Plugin Custom Resource Definition compatible with `krew`
* Must work with image registries that require image pull secrets
* Use `krew` manage plugins made available via CRs
* A Controller to manage plugins that will serve binaries from images
* Controller should be optional, but enabled by default as it will be replacing the existing mechanism which was intented to be temporary
  * See: https://github.com/openshift//enhancements/installer/component-selection.md
* Indexes that should be included by default:
  * Red Hat supported plugins
  * Third-party ISV certified plugins
  * Community (upstream) `krew` plugins

The controller should consider the following:
* Use cluster-scoped image pull secret by default
* Respect ICSP rules when pulling images
* Allow specifying certificates and CAs for image registries
* Allow specifying of proxy configuration for image registries

### Test Plan

**Note:** Section not required until targeted at a release.

### Graduation Criteria

**Note:** Section not required until targeted at a release.

#### Dev Preview -> Tech Preview

**Note:** Section not required until targeted at a release.

#### Tech Preview -> GA

**Note:** Section not required until targeted at a release.

#### Removing a deprecated feature

**Note:** Section not required until targeted at a release.

### Upgrade / Downgrade Strategy

**Note:** Section not required until targeted at a release.

### Version Skew Strategy

* Plugins are expected to be backwards compatible. When working with multiple clusters, it's expected that plugin versions will work across cluster versions
  * If this is not the case, plugin owners will provide that information in the CR description

### Operational Aspects of API Extensions

New CRD for plugins, should not affect existing SLIs.

#### Failure Modes

If controller is not running, and `krew` is configured to use the custom index hosted by the controller, a connection failure will occur.

#### Support Procedures

If a connection failure occurs when using the custom index controller, ensure it is running, exposed, and that `krew` is configured to use the correct URL.

## Implementation History

* 2019-12-03 - Originally proposed by @sallyom: https://github.com/openshift/enhancements/pull/137
* 2021-10-06 - Modified and reproposed by @deejross
* 2021-10-18 - Reworked to use upstream `krew` as the plugin installer instead of adding functionality to `oc`

## Drawbacks

Being that `krew` is for distributing `kubectl` plugins rather than generic CLIs, one drawback is how a non-`kubectl` plugin
is executed after being installed. The binary will always be prefixed with `kubectl-`, so for example, `odo` would be `kubectl-odo`.
This is how `kubectl` plugins work. You can either execute it with that name, or through `kubectl` or `oc` as though it were a
`kubectl` plugin. For example: `kubectl odo` or `oc odo`.
Most modern shells include an alias feature that could be used to mitigate this, either as a future enhancement or documentation example.

One major drawback is that Windows users will require administrative access to their machines to install plugins. See [this issue](https://github.com/kubernetes-sigs/krew/issues/378) for more information.

## Alternatives

* An addition to `oc plugin` was originally proposed and prototyped, but once `krew`-compatibility was successfully implemented in the prototype, we could leverage upstream efforts instead of creating something new
* ["Uc" PoC by Hiram](https://github.com/chirino/uc) - manages Kubernetes CLI clients with an online catalog of releases.
  * Installs to a user's home directory, $HOME/.uc/cache  and when the cluster version does not match a known version, will install latest
  * 'latest' known for uc oc atm is 3.11
  * No activity since 2019

## Infrastructure Needed

* Controller: [concept](https://github.com/deejross/openshift-cli-manager)
* Custom Resource: [example](https://github.com/deejross/openshift-cli-manager/blob/main/config/samples/vault_clitool.yaml)
* Each plugin will publish an image to package binaries [example](https://github.com/openshift/oc/blob/master/images/cli-artifacts/Dockerfile.rhel)

## Followup

* Plugin signature validation
* Prevent non-krew plugins from being distributed
* Alias functionality in modern shells could potentially remove the need to prefix plugin commands with `kubectl` or `oc`
* While this initial proposal is only meant for plugins, general CLI distribution is something we want to address eventually
* Console integration

## References

* [Krew: kubectl plugin manager](https://github.com/kubernetes-sigs/krew)
  * Manages kubectl plugins from an [index](https://github.com/kubernetes-sigs/krew-index) of all known `krew` plugins.

Notes on macOS binaries:

* [Signing binaries for macOS Catalina](https://developer.apple.com/news/?id=09032019a)
* [related to above, Go toolchain issue with macOS Catalina](https://github.com/golang/go/issues/34986)


