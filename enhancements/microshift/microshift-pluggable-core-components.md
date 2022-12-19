---
title: microshift-pluggable-core-components
authors:
  - majopela
reviewers:
  - "@copejon, MicroShift contributor"
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@dhellmann, MicroShift contributor"
  - "@oglok, MicroShift contributor"
  - "@zshi-redhat, MicroShift "
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2022-12-19
last-updated: 2022-12-19
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-599
---

# MicroShift Pluggable Core Components

## Summary

This enhancement proposes making some of the core components like CNI and CSI
optional and pluggable.

In the enhancement proposal we will describe an architecture which would allow
customers to install individual components as rpm packages, as well as some
alternative implementations.

## Motivation

The principal factor that gave birth to the idea of MicroShift was the need to bring
the OpenShift deployment model into the Edge where the power, memory, CPU and
over-the-wire budgets are reduced.

While the default configuration makes sense to most of our customers in the Device
Edge market, still our current CSI and CNI components use 32% of the memory footprint
and a significant CPU and power footprint. Making those components
pluggable would allow customers who don't require advanced network features or
persistent volumes to disable or switch those technologies and reduce the
energy, memory, CPU, disk/over-the-wire and boot time footprint.

In addition, upstream versions of MicroShift may need to ship with different CNI/CSI
options or image versions, and we are looking for ways to enable this.

This same implementation could potentially be applied to other components in
future enhancements.

### User Stories

As a MicroShift Deployment admin persona, I want to disable the CSI plugin since
my device has limited capacity and my application does not need PVs but would
benefit from the additional RAM and CPU.

As a MicroShift Deployment admin persona, I want to disable default CSI and CNI
plugins on my device to save RAM, CPU and disk footprint as my application
does not benefit from those features.

As an Upstream Developer, I need to run a different set of public available images
and potentially other configurations for the CSI/CNI components, I can release new
packages for those components to be tested and used upstream.

As an Project maintainer I want to avoid increasing coupling between the MicroShift
core and the CSI/CNI or other plugins.

As a Project mantainer I want to allow other teams to create new components
which can be installed to MicroShift without any need of coupling to the MicroShift
project or repository.

### Goals

* Allow networking and storage core components to be optionally installed.

* Allow networking and storage components to be replaced upstream for upstream
  versions of those components.

* Split the RPM packages to resemble those optional components.

* Factor out all CNI/CSI specific code from the MicroShift code, so existing and new plugins
  can be developed and mantained independently from MicroShift.

* As much as possible, use mechanisms familiar to K8s admins (for manifests) or Linux
  users (for host-config/RPMs), rather than inventing new mechanisms.

### Non-Goals

* Making DNS component optional.

* Making the certificate management component optional.

* Allowing the replacement of network or storage components post-install. Once
  the system has booted to a storage or network component, the component cannot
  be switched (but a full reset of the system `cleanup-all-microshift-data` and restart
  should work for developers).

* Adding code to MicroShift to avoid the install of two competing implementations at once,
  that should be handled at packaging level.

## Proposal

* Use modularization, so MicroShift no longer needs to carry plug-in specific
  code and plug-ins are self-contained.

* Make plug-in authors use a similar mechanism to deploy necessary manifests as end-users when they want
 to extend the core (--> a generalised/evolved form of the kustomizer running a plugin from MicroShift
 to generate the kustomize yaml).

* Make plug-in authors responsible for the OS-level aspects, in particular how to deal with systemd.

* Use Linux-native software package management mechanisms (here: RPM) to modularise plug-ins, allowing users to express which plug-in they want by explicitly installing / not installing an RPM.

### Proposal details

To allow modularity of the described core components the components are split across
multiple MicroShift subpackages, supporting at least the following use cases:

| Use case 	                                        | Packages installed in the RHEL4Edge image 	| Notes 	                            |
|--------------------------------------------------	|-----------------------------------------  	|-------	                            |
| The whole stack of components is needed         	| `microshift`, `microshift-networking-ovnk`, `microshift-storage-topolvm` | Being explicit about the components avoids unexpected changes to customers if the default components change; i.e. they would get an error if their component of choice got deprecated. |
| Only advanced networking is needed                | `microshift`, `microshift-networking-ovnk`  |                                    	|
| The bare minimum set of components is necessary   | `microshift`                             |                                   	|
| Only persistent volumes are necessary             | `microshift`, `microshift-storage-topolvm` |                                     |


The packages for networking and storage would be different in upstrean, containing
references to unrestricted (OKD) container images, removing the need for a pull secret.

Reuse the kustomize loader so system and user components use the mechanism,
extending its behaviour so binary plugins can be used in this content.


`/usr/lib/microshift/components.d/004-multus/plugin`
`/usr/lib/microshift/components.d/005-networking-ovnk/plugin`
`/usr/lib/microshift/components.d/010-storage-topolvm/plugin`

  MicroShift component manager would scan the `/usr/lib/microshift/components.d/`
  directory executing the component plugins in alphabetical order.

  `plugin` is an executable (in most cases a bash script will be the simplest
  solution), using go binaries would be an option when complex structures
  or connectivity back to the MicroShift API is required.

  Today for the CNI/CSI components there is no wait or readiness conditions,
  if this needed to be implemented in the future it could be implemented in
  the `plugin`.

The following changes would be introduced to packaging:

* Convert `microshift` into a `microshift-core`, which does not depend
  on `microshift-networking-ovnk`or `microshift-storage-topolvm`.

* The `microshift-networking-ovnk` package would include the necessary
  `/usr/lib/microshift/components.d/05-networking-ovnk/plugin` and collateral
  yaml files, including CRDs.

* The `microshift-storage-topolvm` package would include the necessary
  `/usr/lib/microshift/components.d/10-storage-topolvm/plugin` and any
   collateral yamls, including crds.

* The rebase script should be updated to extract those core components
  as and update the plugin or kustomize yamls. At some point each component
  could implement its own rebase script.

* Incompatible component rpms should be impossible to install together.
  i.e. `microshift-networking-bridge` plus `microshift-networking-ovnk`,
  this can be handled with Conflict spec clauses. (This is just an example,
  since microshift-networking-bridge would not be necessary as it's the
  default for container-networking)

* Systemd services of the then optional networking components (like `microshift-ovs-init`)
  should still start before the `microshift` systemd service, but `microshift` cannot
  depend on `microshift-ovs-init` anymore. This can be acomplished by using the
  Before=microshift clause in `microshift-ovs-init.service` and enabling this service
  during the `microshift-networking-ovnk` sub-package install.
  An alternative to this can be making `microshift-ovs-init` part of the plugin.

#### Plugin calling convention

The `plugin` binaries receive configuration details via environment variables, using the same variable format
MicroShift already receives (see MicroShift's howto_config.md):

| Environment Variable                    | Description                                                       |
|-----------------------------------------|-------------------------------------------------------------------|
| MICROSHIFT_CLUSTER_CLUSTERCIDR          | A block of IP addresses from which Pod IP addresses are allocated |
| MICROSHIFT_CLUSTER_SERVICECIDR          | A block of virtual IP addresses for Kubernetes services           |
| MICROSHIFT_CLUSTER_SERVICENODEPORTRANGE | The port range allowed for Kubernetes services of type NodePort   |
| MICROSHIFT_CLUSTER_URL                  | URL of the API server for the cluster.                            |
| MICROSHIFT_NODEIP                       | The IP address of the node, defaults to IP of the default route   |
| MICROSHIFT_SUBJECTALTNAMES              | Subject Alternative Names for apiserver certificates              |
| MICROSHIFT_LOGVLEVEL                    | Log verbosity (Normal, Debug, Trace, TraceAll)                    |
| MICROSHIFT_NODENAME                     | The name of the node, defaults to hostname                        |
| MICROSHIFT_BASEDOMAIN                   | Base DNS domain used to construct fully qualified router and API domain names. |

Plugins can read their own configuration files in case of needing more specific
configuration details, or this interface could be expanded in the future to
provide more generic data which plugins may need.

As input `plugin` will receive an argument command.

| Command                    | Description                                                       |
|-----------------------------------------|-------------------------------------------------------------------|
| api-version | Identifies the version of the calling convention for this plugin, this could be useful if we the implementation needed future changes |
| run | Initializes/validates the system and generates manifests to `stdout` |

As output it would generate an exit code: 
 * 0: Normal exit, everything was ok.
 * 1: Some system pre-condition is not met (i.e. 'br-ex' is missing for OVNk plugin)
 * 2: Configuration details are incorrect (env vars or additional files)

any manifests that MicroShift needs to apply are written to `stdout` in kustomize format.

Logging details and errors are written by the plugin to `stderr` and MicroShift will report that into
the MicroShift logging output for visibility.

MicroShift executes the plugin with the plugin directory as the current work directory, so it's simple
for the plugin to access any co-located files (yamls, default configurations, etc..).

NOTE to be discussed: We have talked about limiting permissions of the called plugin,
by using a non-root user, or equivalent (that would mean we need to setup another user
on the system, additionally the plugin could always generate a YAML for pods with elevated privileges).
Also in the end, if somebody is able to inject a malicious plugin, then is able to
inject any other malitious binary on the system, and we would be limiting what
plugins could do. (majopela: After thinking about it, I propose we don't limit this).


### Workflow Description

#### Deploying

The workflow for deployment is already described in the `Proposal details` section.

#### Upgrading

Upgrading optional components would behave in the same way as upgrading
components where the assets are embedded into the MicroShift binary.

There is an open question about how to handle migration of CRD based
content between versions. This problem is not handled or explored in MicroShift
yet. Could this be handled by one-shot pods, the one-shot pods could be defined to
be triggered only if an specific type of object/version existed, another
alternative to this would be to use upgrade-binaries optional in components
which are ran when necessary (similar to how CNIs work).

#### Configuring

Configuration of the optional components is done by package selection
when the images are created.

Specific component configuration could be provided to plugins via specific
files in `/etc` the plugins would be responsible for reading and parsing
those when necessary.

#### Deploying Applications

Applications are deployed as usual.

### API Extensions

None.

### Risks and Mitigations

The component combinations would need additional testing handled by
an increased testing matrix. Considering networking/storage to be independent
elements, probably an additional set of tests which cover `microshift-core`
together with `microshift-storage-topolvm` would be enough.

When running those tests we need to disable NetworkPolicy related testing.

### Drawbacks

We are creating a new API (the interface between MicroShift and the plugin).

## Design Details

### Open Questions

1. How do we handle CR upgrades when moving to newer versions of the APIs?,
   See the `Upgrading`section.

### Test Plan

We will run additional test matrix with `microshift-core` alone, in those
tests any OVN/NetworkPolicy tests would need to be disabled.

Additional unit tests should cover the loading of core components.

Additional unit tests should cover the loading of multiple `.d` style
directories with plugins.

## Implementation History

* [openshift/microshift](https://github.com/openshift/microshift)
* [Design guidelines](https://github.com/openshift/microshift/blob/main/docs/design.md)

## Alternatives

### Using kustomize yamls only for components

Plugins provide a fixed set of yamls to deploy the component in MicroShift
and we deploy it at boot.


Risks:

* This is limiting in terms of configuration details that plugins may need
  to inject or act on. This could be overcome by templating, but that would
  make the kustomize yamls hard to test outside of MicroShift.

### Using go plugins which microshift can load when available, intead of yamls.

[go/plugin](https://pkg.go.dev/plugin) could be used to create an interface
for loadable components, MicroShift would scan an spefic directory for .so files

Every plugin would contain it's own list of images, setup functionality,
validation hooks, etc.

The Risks here are:

* Binary code duplication: How do we handle shared code between MicroShift
  and the plugins, for example plugins would be expected to call kustomize
  libraries, but can't call back into code in MicroShift binary. This would
  force us replicate those libraries again statically in the plugins along
  with the kubernetes clients

* The inteface and calling convention needs to be maintained with versioning
  to avoid externally created plugins from breaking as we evolve the API.
  This is also true for yaml based components, but it's simpler to provide
  additive iterations without breaking backwards compatibility in that case.
  
* CGO_ENABLED needs to be enabled.

* Requires same version of go vendor dependencies.

The possible benefits are:

* Easier to handle validations of the system
* More flexible ability to read and handle configuration files.
* Upgrade of CRDs can potentially be done

### on/off components within MicroShift binary code

We could add flags to the MicroShift configuration which then can control
the component loading via flags.

Image versions would be switched at compile time for upstream versions
where necessary. Components which we could not ship upstream would be disabled
out from compilation via build -tags.

All the code relevant to the non-loaded componets would remain in the binary.

The possible benefits are:

* It's probably the simplest implementation.

Risks:

* It does not prevent coupling creep. Components would continue to couple with
  the MicroShift codebase in unexpected ways, as we already see today.

### External binaries with access back to the API

MicroShift would call on the external binaries with a kubeconfig, and use a
calling interface to request actions like setup/initialization/upgrades/etc.

Benefits:

* Fully decoupled plugin logic in external binary.

* The external binary could run independently in parallel in some cases.

Risks:

* Binary code duplication in disk, as the kubernetes and other related libraries
  need to be compiled again into another binary.

More details on this idea can be found here (option 3):
   https://docs.google.com/document/d/1MPTsf6KNMDdKfySkeVhrDRdm8GbphlRNDz_YrXErZjU/edit
