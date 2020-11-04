---
title: Supporting out-of-tree drivers on OpenShift
  - "@zvonkok"
reviewers:
  - "@ashcrow"
  - "@darkmuggle"
  - "@cgwalter"
approvers:
  - "@ashcrow"
  - "@cgwalters" 
  - "@darkmuggle"
creation-date: 2020-04-03
last-updated: 2020-07-21
status: provisional
see-also:
  - "/enhancements/TODO.md"
replaces:
  - "/enhancements/TODO.md"
superseded-by:
  - "/enhancements/TODO.md"
---

# Supporting out-of-tree drivers on OpenShift

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

1. Are users willing to provide precompiled DriverContainers (We have already some commitments)?  This helps immensely in disconnected environments and only validated combinations of driver + kernel would be e.g. provided. Source instal should be the fallback solution

## Summary

OpenShift will support out-of-tree and third-party kernel drivers and the support software for the underlying operating systems via containers.

## Terminology

### Day 0, Day 1, Day 2, ...
The terms Day 0, Day 1, Day 2 refer to different phases of the software life cycle. There are different interpretations what Day <Z> means. In the context of OpenShift: 
* Day 1 are all operations involved to install an OpenShift cluster 
* Day 2 are all operations involved after an OpenShift cluster is installed 

In this enhancement we are solely concentrating on Day 2 operations.

### DriverContainers

DriverContainers are used more and more in cloud-native environments, especially when run on pure container operating systems to deliver hardware drivers to the host. Driver containers are more than a delivery mechanism for the driver itself, as they extend the kernel stack beyond the out-of-box software and hardware features of a specific kernel. Additionally, a driver container can handle the configuration of modules, and start userland services. 
 
DriverContainers work on various container capable Linux distributions.
 
With DriverContainers the host stays always "clean", and does not classh with different library versions or binaries on the host. Prototyping is far more easier, updates are done by pulling a new container with the loading and unloading done by the DriverContainer with checks on /proc and /sys and other files to make sure that all traces are removed).

## Current Solutions

Here are the current solutions in use today.

### Special Resource Operator
 
For any day-2 management of kernel modules we can leverage the Special Resource Operator (SRO) features. SRO was written in such a way that it is highly customizable and configurable to any hardware accelerator or out-of-tree kernel module. 

A detailed description of SRO and its inner workings are described in the following two blog posts:

* [https://red.ht/2JQuNwB](https://red.ht/2JQuNwB)
* [https://red.ht/34ubzq3](https://red.ht/34ubzq3)
 
SRO supports full lifecycle management of an accelerator stack but it can also be used as a stripped down version to manage e.g only one kernel module. Furthermore SRO can handle multiple kernel modules from different vendors and is able to model a dependency between those too.
 
Another important feature is the ability to consume build artifacts from other kernel modules to build a more sophisticated DriverContainer. SRO is capable of delivering out-of-tree drivers and supporting software stacks for kernel features and hardware that is not shipped as part of the standard Fedora/RHEL distribution.

Ideally, SRO would pull a prebuilt DriverContainer with precompiled drivers from the vendor. Any module updates (and downgrades) will be delivered by container. 

The Special Resource Operator (SRO) is currently only available in OperatorHub. SRO has proven in the past to be the template for enabling hardware when on OpenShift. Its capabilities to handle several DriverContainers with only one copy of SRO running makes it a preferable solution to tackle kmods on OpenShift.

SRO is going to be a core-component of OpenShift and delivered/managed by CVO. Here is an example how one can use SRO + KVC to deliver a simple kernel module via container in a OpenShift cluster: [https://bit.ly/2EAlLEF](https://bit.ly/2EAlLEF)


### kmods-via-containers (KVC)

[kmods-via-containers](https://github.com/kmods-via-containers/) is a framework for building and delivering kernel modules via containers. The implementation for this framework was inspired by the work done by Joe Doss on [atomic-wireguard](https://github.com/jdoss/atomic-wireguard).
This framework relies on 3 independently developed pieces.

1. [The kmods-via-containers code/config](https://github.com/kmods-via-containers/kmods-via-containers)

Delivers the stencil code and configuration files for building and delivering kmods via containers. It also delivers a service `kmods-via-containers@.service` that can be instantiated for each instance of the KVC framework.

2. The kernel module code that needs to be compiled

This repo represents the kernel module code that contains the source code for building the kernel module. This repo can be delivered by vendors and generally knows *nothing* about containers. Most importantly, if someone wanted to deliver this kernel module via the KVC framework, the owners of the code don't need to be consulted. The project provides an [example kmod repo](https://github.com/kmods-via-containers/simple-kmod).

3. A KVC framework repo for the kernel module to be delivered

This repo defines a container build configuration as well as a library, userspace tools, and config files that need to be created on the host system. This repo does not have to be developed by the owner of the kernel module that is wanted to be delivered.

It must define a few functions in the bash library:

* `build_kmods()`
  * Performs the kernel module container build
* `load_kmods()`
  * Loads the kernel module(s)
* `unload_kmods()`
  * Unloads the kernel module(s)
* `wrapper()`
  * A wrapper function for userspace utilities

Customers can hook in their procedures on how to build, load, unload etc their kernel modules. We are providing only an interface not the actual "complicated" implementation of those steps (facade pattern). Customers can then use any tool(s) (akmods, dkms, ..) they need to build their modules. 

[This repo](https://github.com/kmods-via-containers/kvc-simple-kmod) houses an example using `simple-kmod`.

## Motivation

In OpenShift v3.x out-of-tree drivers could be easily installed on the nodes, since the node was a full RHEL node with a subscription and needed tools were installed easily with yum. 

In OpenShift v4.x this changed with the introduction of RHCOS. There are currently two different documented ways to enable out-of-tree drivers. One way is using a SRO based operator and the other is using kmods-via-containers. 

We want to come up with a unified solution that works for our customers across RHCOS and RHEL. The solution should also help customers that are currently on RHEL7 to consider moving to RHCOS which is fully managed and easier to support in OpenShift.
 
#### Day-2 DriverContainer Management OpenShift
 
More and more customers/partners want to enable hardware and or software on OpenShift that need kernel drivers, which are currently not (and may never be) upstream and out-of-tree. 

We want to provide a unified way to support out-of-tree kernel drivers (multiple) on OpenShift day-2. It has to work for classical (RHEL7, 8, Fedora) and container based operating systems (RHCOS, FCOS) in the same way. 

Many customers/partners are using [dkms](https://github.com/dell/dkms)/akmods as the solution to build, rebuild modules on kernel changes. Adopting dkms/akmods is not a workable solution for OpenShift/OKD; we need to create and own acceptable build and delivery mechanism for RHEL, Fedora, and OpenShift. 
 
#### Fill the gap of providing drivers that are not yet, or will never be, upstream

For the DriverContainer we need to cover several stages of driver packaging. 

* **source repository or archive** The driver is available as source code, is not packaged, and/or is required to be set up before the cluster is available; this is where KVC can help
* **kmod-{vendor}-src.rpm** The next step is a source RPM package that can be recompiled with rpmbuild. This is also the base for akmods, dkms
* **precompiled-{vendor}-{kernelversion}.rpm** Precompiled RPMS this is the wish thinking to have in the future. DriverContainers could be easily built just by using RPMs. 

Some drivers will *never* be upstreamed and can be in any of the states described above. The proposed solutions needs to handle drivers in any state and build by "any" tool. 

The compilation of kernel modules was always anticipated to be the fallback solution when dealing with kernel modules. Some kernel modules will always be out-of-tree and are not going to be included in the near future. Some kernel modules are out-of-tree but we are working with the vendors on upstreaming them to the mainline kernel. 

### Goals


* A unified way to deploy out-of-tree drivers on OpenShift 4.x on all supported Red Hat Operating Systems
* The solution should avoid rebuilds on every node and allow for distribution of drivers on a cluster using the cluster registry
* A solution for day-2 kernel modules
* Support upgrades of OpenShift for multiple kernel module providers
* Hierarchical initialization of kernel modules (modeling dependencies)
* Handle dependencies between kernel modules (depmod) in tree and out of tree
* Should support disconnected and proxy environments
* Support heterogeneous cluster: 
  * OpenShift with RHEL7, 8 and RHCOS 

### Non-Goals

* The solution is not a replacement for the traditional way of delivering kernel modules, customer/partners should be aware that we prefer they deliver the drivers upstream
* We are not providing a way how to build the drivers this is business logic of a specific vendor, we are providing the interface to hook into specific stages of a DriverContainer
* Extending customer support for third-party modules or implications of said modules.

## Proposal

The SRO pattern showed how to enable hardware and the complete hardware accelerator stack on OpenShift. The heavy-lifting was the management of the DriverContainer. Approximately 5% of the logic behind SRO was used for deploying the remaining parts aka stack. 

Based on SRO we are going to create a new operator that will solely be used for DriverContainer management providing out of tree drivers on OpenShift. 

SRO can be seen as the upstream project where we test new features and enable more hardware accelerators where this new operator will be the downstream operator shipped by OpenShift for out-of-tree drivers. 

The new version of SRO will have an API update and hence called SROv2. 


#### Combining both approaches

For managing the module in a container we are going to use KVC as the framework of choice. Targeting RHCOS solves the problem also for RHEL7 and RHEL8. The management of those KVC containers aka DriverContainers are managed by SROv2. 

#### Day-2 DriverContainer Management OpenShift
 

For any day-2 kernel module management or delivery we propose using SROv2 as the building block on OpenShift. 

We will run a single copy of the SROv2 as part of OpenShift that is able to handle multiple kernel module drivers using the following proposed CR below.

The following section will cover three kernel module instantiations (1) A single kernel module (2) multiple kernel modules with build artifacts (3) full-stack enablement. 

There are three main parts involved in the enablement of a kernel module. We have a specific (1) set of meta information needed for each kernel module, a (2) set of manifests to deploy a DriverContainer and lastly (3) a framework running inside the container for managing the kernel module (dkms like functions). 

The following section will walk one through the enablement of the different use-case scenarios. After deploying the operator the first step is to create an instance of a special-resource. Following are some example CRs how to one would instantiate SROv2 to manage a kernel module or hardware driver. 

#### Example CR for a single kernel module #1 

```
apiVersion: sro.openshift.io/v1alpha1
kind: SpecialResource
metadata:
  name: <vendor>-<kmod>
spec:
  driverContainer:
  - git: 
      ref: "release-4.3"
      uri: "https://gitlab.com/<vendor>/<kmod>.git"
```


The second example shows the combined capabilities of SROv2 for dealing with multiple driver containers and artifacts. On the other side SROv2 can also be used in a minimalistic form where we only deploy a simple kmod. The example CR above would create only one DriverContainer from the git repository provided. For each kernel module one would provide one CR with the needed information. 

#### Example CR for a hardware vendor (all settings) #2

```
apiVersion: sro.openshift.io/v1alpha1
kind: SpecialResource
metadata:
  name: <vendor>-<hardware>
spec:
  metadata:
    namespace: <vendor>-<driver>
  environment:
  - key: "key_id"
    value: "ACCESS_KEY_ID"
  - key: "access_key"
    value: "SECRET_ACCESS_KEY"
  driverContainer:
    source: 
      git: 
        ref: "master"
        uri: "https://gitlab.com/<vendor>/driver.git"
    buildArgs:
    - name: "DRIVER_VERSION"
      value: "440.64.00"
    - name: "USE_SPECIFIC_DRIVER_FEATURE"
      value: "True"
    runArgs:
    - name: "LINK_TYPE_P1" # 1st Port
      value: "2"  #Ethernet
    - name: "LINK_TYPE_P2" # 2nd Port
      value: "2"  #Ethernet
    artifacts:
      hostPaths:
      - sourcePath: "/run/<vendor>/usr/src/<artifact>"
        destinationDir: "/usr/src/"
      images:
      - name: "<vendor>-{{.KernelVersion}}:latest"
        kind: ImageStreamTag
        namespace: "<vendor>-<hardware>"
        pullSecret: "vendor-secret"
        paths:
        - sourcePath: "/usr/src/<vendor>/<artifact>
          destinationDir: "/usr/src/"
      claims:
      - name: "<vendor>-pvc"
      mountPath: "/usr/src/<vendor>-<internal>"
  node: 
    selector: "feature.../pci-<VENDOR_ID>.present"
  dependsOn:
    - name: <CR_NAME_VENDOR_ID_SRO>
    - name: <CR_NAME_VENDOR_ID_KJI>
```

Since SROv2 will manage several special resources in different namespaces, hence the CRD will have cluster scope. The SROv2 can take care of creating and deleting of the namespace for the specialresource. Otherwise one would have an manual step in creating the new namespace before creating the CR for a specialresource, which make cleanup of a special resource easy, just by deleting the namespace. If there is no spec.metadata.namespace supplied SROv2 will set the namespace to the CR name per default to separate each resources. 

With the above information SROv2 is capable of deducing all needed information to build and manage a DriverContainer. All manifests in SROv2 are templates that are templatized during reconciliation with runtime and meta information. 

```
----------------------------- SNIP ---------------------------------
metadata:
  name: <vendor>-<hardware>
spec:
  metadata:
    namespace: <vendor>-<hardware>
  environment:
  - key: "key_id"
    value: "ACCESS_KEY_ID"
  - key: "access_key"
    value: "SECRET_ACCESS_KEY"
  driverContainer:
    source: 
      git: 
        ref: "master"
        uri: "https://gitlab.com/<vendor>/driver.git"
----------------------------- SNIP ---------------------------------
```

The name e.g. is used to prefix all resources (Pod, DameonSet, RBAC, ServiceAccount, Namespace, etc) created for this very specific **{vendor}-{hardware}**. The DriverContainer section expects optionally the git repository from a vendor. This repository has all tools and scripts to build the kernel module. The base image for a DriverContainer is an UBI7,8 with the KVC (kmod-via-containers) framework installed. Simpler builds can be accomplished by including the Dockerfile into the Build YAML.

KVC provides hooks to build, load, unload the kernel modules and a wrapper for userspace utilities. We might extend the number of hooks to have a similar interface as dkms. 

The environment section can be used to provide a arbitrary set of key value pairs that can be later templatized for any kind of information needed in the enablement stack.

```
----------------------------- SNIP ---------------------------------
    buildArgs:
    - name: "DRIVER_VERSION"
      value: "440.64.00"
    - name: "USE_SPECIFIC_DRIVER_FEATURE"
      value: "True"
----------------------------- SNIP ---------------------------------
```

Another important field is the build arguments. We have often seen incompatibility between workloads and driver versions. Selecting a specific version is sometimes the only way to have a workload successfully running on OpenShift or BareMetal. This field can also be used by an administrator to upgrade or downgrade a kernel module due to CVEs, bug fixes or incompatibility. Some drivers have also some flags to enable or disable specific features of a driver. 

```
----------------------------- SNIP ---------------------------------
    runArgs:
    - name: "LINK_TYPE_P1" # 1st Port
      value: "2"  #Ethernet
    - name: "LINK_TYPE_P2" # 2nd Port
      value: "2"  #Ethernet
----------------------------- SNIP ---------------------------------
```

Run arguments can be used to provide configuration settings for the driver. Some hardware accelerators e.g. need to change specific attributes that are only available after the DriverContainer is executed. 

```
----------------------------- SNIP ---------------------------------
    artifacts:
      hostPaths:
      - sourcePath: "/run/<vendor>/usr/src/<artifact>"
        destinationDir: "/usr/src/"
      images:
      - name: "<vendor>-{{.KernelVersion}}:latest"
      kind: ImageStreamTag
      namespace: "<vendor>-<hardware>"
      pullSecret: "vendor-secret"
      paths:
      - sourcePath: "/usr/src/<vendor>/<artifact>
        destinationDir: "/usr/src/"
      claims:
      - name: "<vendor>-pvc"
      mountPath: "/usr/src/<vendor>-<internal>"
----------------------------- SNIP ---------------------------------
```
The next section is used to tell SROv2 where to find build artifacts from other drivers. Some drivers need e.g. symbol information from kernel modules, header files or the complete driver sources to be built successfully. We are providing two ways for these artifacts to be consumed. (1) Some vendors expose the build artifacts in a hostPath. The DriverContainer with KVC needs a hook for preparing the sources, which means it would copy from sourcePath on the host to the destinationDir in the DriverContainer. (2) The other way to get build artifacts is to use an DriverContainer image that is already built do get the needed artifacts (We are assuming here that the vendor is not exposing any artifacts to the host). We can leverage those images in a multi-stage build for the DriverContainer. 
```
----------------------------- SNIP ---------------------------------
  node: 
    selector: "feature.../pci-<VENDOR_ID>.present"
----------------------------- SNIP ---------------------------------
```

The next section is used to filter the nodes on which a kernel module or driver should be deployed on. It makes no sense to deploy drivers on nodes where the hardware is not available. Furthermore this can also be used to target even subsets of special nodes either by creating labels manually or leveraging NFDs hook functionality. 

To retrieve the correct image we are using SROv2s templating to inject the correct runtime information, here we are using **{{.KernelVersion}}** as a unique identifier for DriverContainer images. 

For the case when no external or internal repository is available or in a disconnected environment, SROv2 can consume also sources from a PVC. This makes it easy to provide SROv2 with packages or artifacts that are only available offline.
```
----------------------------- SNIP ---------------------------------
  dependsOn:
    - name: <CR_NAME_VENDOR_ID_SROv2>
      imageReference: "true"
    - name: <CR_NAME_VENDOR_ID_KJI>
----------------------------- SNIP ---------------------------------
```
There are kernel-modules that are relying on symbols that another kernel-module exports which is also handled by SROv2. We can model this dependency by the dependsOn tag. Multiple SROv2 CR names can be provided that have to be done (all states ready) first before the current CR can be kicked off. CRs with now dependsOn tag can be executed/created/handled simultaneously. 

Users should usually deploy only the top-level CR ans SROv2 will take care of instantiating the dependencies. There is no need to create all the CRs in the dependency, SROv2 will take care of it. 

If special resource *A* uses a container image from another special resource *B* e.g using it as a base container for a build, SROv2 will setup the correct RBAC rules to make this work. 


```
----------------------------- SNIP ---------------------------------
    buildArgs:
    - name: "KVER"
      value: "{{.KernelVersion}}" (1)
    - name: "KMODVER"
      value: "SRO"
----------------------------- SNIP ---------------------------------
```

One can also use template variables in the CR that are correctly templatized by SROv2 in the final manifest. DKOM does a 2 pass templatizing, 1st pass is to inject the variable intot the manifest and the second pass it templatize this given variable. Even if we do not know the runtime information beforehand of an cluster we can use it in a CR. 

 
#### DriverContainer Manifests

The third part of enablement are the manifests for the DriverContainer. SROv2 provides a set of predefined manifests that are completely templatized and SROv2 updates each tag with runtime and meta information. They can be used for any kernel module. Each Pod has a ConfigMap as an entrypoint, this way custom commands or modification can be easily added to any container running with SROv2. See [https://red.ht/34ubzq3](https://red.ht/34ubzq3) for a complete list of annotations and template parameters. 

To ensure that a DriverContainer is successfully running SROv2 provides several annotations to steer the behaviour of deployment. We can enforce an ordered startup of different stages. If the drivers are not loaded it makes no sense to startup e.g. a DevicePlugin it will simply fail and all other dependent resources as well. 

DriverContainers can be annotated and are telling SROv2 to wait for full deployment of DaemonSets or Pods. SROv2 watches the status of the resources. Some DriverContainers can be in a running state but are executing some scripts before being fully operational. SROv2 provides a special annotation for the manifest to look for a specific regex in the container logs to match before declaring a DriverContainer as operational. This way we can guarantee that drivers are loaded and subsequent resources are running successfully. 

#### Supporting Disconnected Environments

SROv2 will first try to pull a DriverContainer. If the DriverContainer does not exist, SROv2 will kick off a BuildConfig to build the DriverContainer on the cluster. Administrators could build a DriverContainer upfront and push it to an internal registry. If is able to pull it, it will ignore the BuildConfig and try to deploy another DriverContainer if specified (ImageContentSourcePolicy). 

### Operator Metrics & Alerts

Like DevicePlugins the new operator should provide metrics and alerts on the status of the DriverContainers. Alerts could be used for update, installation or runtime problems. Metrics could expose resource consumption, because some of the DriverContainers are also shipping daemons and helper tools that are needed to enable the hardware. 

### User Stories [optional]

#### Story 1: Day-2 DriverContainer Kernel Module

As a vendor of kernel extensions I want a procedure to build modules on OpenShift with all relevant dependencies. Before loading the module I may need to do some housekeeping and start helper binaries and daemons. This procedure should enable an easy way to interact with the module and startup and teardown of the entity delivering the kernel extension. I should also be able to run several instances of the kernel extension (A/B testing, stable and unstable testing). 
 
#### Story 2: Multiple DriverContainers

As an administrator I want to enable a hardware stack to enable a specific functionality with several kernel modules. These kernel modules may have an order and the procedure enabling them needs to expose a way to model this dependency. It may even be the case that a specific module needs kernel modules loaded that are already installed on the node. The intended clusters are either behind a proxy or completely disconnected and hence the anticipated procedure has to work in these environments too. 

#### Story 3: Day-2 DriverContainer Accelerator

As a vendor of a hardware accelerator I want a procedure to enable the accelerator on OpenShift no matter which underlying OS is running on the nodes. The life-cycle of the drivers should be fully managed with the ability to upgrade and downgrade drivers for the accelerator. It should support all kernel versions (major, minor, z-stream) and handle driver errors gracefully. Uninstalling the drivers should not leave any trace of the previous installation (keep the node as clean as possible). The driver will not be upstreamed to the mainline kernel, which means it will always be out-of-tree. 

#### Story 4: Multiple Driver Containers with Artifacts

As an administrator I want to enable a specific vendor stack. I need to build kernel modules that are dependent on each other during the build and at the time of loading. Specific build artifacts need to be available during the build not for loading. These artifacts can be available in another DriverContainer or extracted during runtime. 


### Implementation Details/Notes/Constraints

DriverContainers need at least the following packages: 
- kernel-devel-$(uname -r)
- kernel-headers-$(uname -r)
- kernel-core-$(uname -r)

Kernel-core is needed for running `depmod` inside the DriverContainer to resolve all symbols and load the dependent modules. The DriverContainer is not installing the kmods on the host and hence the already installed modules from the host are missing in the container. 

These packages can be installed from different sources, SRO can currently handle all three:
- from base repository
- from EUS repository
- from machine-os-content (missing kernel-core)

[BuildConfigs do not support volume mounts](https://issues.redhat.com/browse/DEVEXP-17), cannot be used for artifacts on a hostPath. If we have all artifacts stored in a container image, BuildConfig can/could be leveraged for building. Where no build artifacts are needed a BuildConfig is a first choice, because of the functionality it provides (triggers, source, output, etc).
One of the most important things, we have no issues with SELinux, because we are interacting with the same SELinux contexts (container_file_t). Accessing host devices, libraries, or binaries from a container breaks the containment
as we have to allow containers to access host labels. 

For DriverContainers to build the kernel modules we need entitlements first. The e2e story is described here: [https://bit.ly/2XjZq5D](https://bit.ly/2XjZq5D)
We need to provide an interface for vendors to hook in their business logic of building the drivers
For prebuilt containers pullable from a vendor's repository we're going to use a ImageContentSourcePolicy, currently only pulling by digest works, we cannot pull by label now. We need to accommodate this in the naming scheme of a DriverContainer.
If a DriverContainer needs additional RPM packages in a proxy environment we need to update the configuration for dnf and rhsm. 
 
Steps to enable proxy setting for yum, dnf, rhsm in a container 

```
$ sudo vim /etc/dnf/dnf.conf
# Add
proxy=http://proxyserver:port

$ sudo vim /etc/yum.conf
# Add
proxy=http://proxyserver:port

$ sudo vi /etc/rhsm/rhsm.conf
# Configure
proxy_hostname = proxy.example.com
proxy_port = 8080
# user name for authenticating to an http proxy, if needed
proxy_user =

# password for basic http proxy auth, if needed
proxy_password =
```



### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For example, consider both security and how this will impact the larger OKD ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

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


### Upgrade / Downgrade Strategy


#### Special Resource Driver Updates 

Initially, the NFD operator labels the host with the kernel version (e.g. 4.1.2). The SRO reads this kernel version and creates a DaemonSet with a NodeSelector targeting this kernel version and a corresponding image to pull. With the pci-10de label, the DriverContainer will only land on GPU nodes. 

This way we ensure that an image is pulled and placed only on the node where the kernel matches. It is the responsibility of the DriverContainer image builder to name the image the correct way.

If an administrator updates the CR with a new driver version will taint the node rebuild the DriverContainer and un-taint the node with the new drivers running on a node. 

#### Updates in OpenShift

Updates in OpenShift can happen in two ways: 
1. Only the payload (operators and needed parts on top of the OS) is updated
2. The payload and the OS are simultaneously updated. 

The first case is "easy" the new version of the operator will reconcile the expected state and verify that all parts of the special resource stack are working and then "do" nothing. 

For the second case, the new operator will reconcile the expected state and see that there is a mismatch regarding the kernel version of the DriverContainer and the updated Node. It will try to pull the new image with the correct kernel version. If the correct DriverContainer cannot be pulled, will update the BuildConfig with the right kernel version and OpenShift will reinitiate the build since we have the ConfigChange trigger as described above. 

Besides the ConfigChange trigger, we also added the ImageChange trigger, which is important when the base image is updated due to CVE or other bug fixes. For this to happen automatically we are leveraging ImageStreams of OpenShift, an ImageStream is a collection of tags that gets automatically updated with the latest tags. It is like a container repository that represents a virtual view of related images.

To be always up to date another possibility would be to register a github/gitlab webhook so every time the DriverContainer code changes a new container could be built. One has just to make sure that the webhook is triggered on a specific release branch, it is not advisable to monitor a fast moving branch (e.g. master) that would trigger frequent builds. 

#### Special Resource Driver Downgrade

Having a look at the example CRs above we can see that one can provide a driver version for a specific hardware aka DriverContainer. will take care of updating the BuildConfig and DriverContainer manifests. Tainting the node as **specialresource.openshift.io/downgrade=true:NoExecute** will evade all running Pods and the DriverContainer can be restarted. When the DriverContainer is again up and running, the node can be un-tainted to allow workloads to be scheduled on the node again. 
 
 
#### Update proactive DriverContainers

A preferable workflow for updates could also be to be proactive on updates. When OpenShift is updated we would need a mechanism (notification, hook for updates) to provide the kernel version of the next update to before attempting the upgrade and rebooting the nodes (needinfo installer/CVO team) .

This way DriverContainers are prebuild and potential problems can be examined before the update completes (e.g. no drivers for newer kernels, build errors, etc). 

Another major point is to know the underlying RHEL version with major and minor number. Many drivers have dependencies on RHEL8.0 or RHEL8.1 etc. Currently there is no easy way to find out if RHCOS is based on RHEL8.0 or RHEL8.1 (OpenShift 4.3 e.g changes from 8.0 to 8.1 depending on the z-Stream)
 
#### Exception Handling

If there is no prebuilt DriverContainer and no source git repository is provided to build the DriverContainer the current behaviour is to wait until one of these prerequisites are fulfilled. Either a DriverContainer is pushed to a registry known to or a new updated CR is created. The current status is exposed in the status field of the special resource. 
To prevent such a state, before an update happens the user/administrator should know the kernel version upfront. We need an most obvious way to expose the kernel version.

Even with the kernel version exposed it is hard to know if an update will break the cluster. There are several constraints of the drivers and how they tie to a kernel version. 

You have one single source of drivers that can be compiled on all major RHEL versions. Here it does not matter which kernel version we are running. Here we can assume that drivers work for all 3.xx.yy and 4.xx.yyy kernels.

One could have drivers that are only dependent on the major RHEL versions. We would need to consider "only" upgrades from one major to the other. Here drivers are sensitive going from one major kernel version to the other. 

Another case is where drivers are also sensitive to minor version changes which means they are driver changes for any kernel version. 
 
#### DCI - Distributed CI Environment (RHEL) 
 


### Version Skew Strategy

In some use-case scenarios relies on NFD labels, which are used as node selectors for deploying the DriverContainers. NFD labels are not changed during updates. The specific label is an input parameter for the CR of a hardware. 

NFD labels are integral parts of the node, if a label is not discovered then the hardware is not available and hence not intended to be used as a deployment target for DriverContainers. 
