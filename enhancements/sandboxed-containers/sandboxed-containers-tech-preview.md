---
title: sandboxed-containers
authors:
  - "@ariel-adam"
  - "@fidencio"
  - "@jensfr"
reviewers:
  - "@cgwalters"
  - "@mrunalp"
approvers:
  - "@cgwalters"
  - "@mrunalp"
creation-date: 2020-06-07
last-updated: 2021-03-12
status: implemented
see-also:
  - "[OpenShift sandboxed containers Operator](https://github.com/openshift/sandboxed-containers-operator)"
  - "[CoreOS Extensions](https://github.com/openshift/enhancements/pull/317)"
  - "[QEMU Extension](https://github.com/openshift/machine-config-operator/pull/2376)"
replaces:
  - ""
superseded-by:
  - ""
---

# sandboxed containers

Sandboxed containers provides a container runtime using virtual machines,
and hardware virtualization technologies, providing powerful workload
isolation compared to the existing container solutions, with the same look
and feel provided by vanilla containers.

## Release Signoff Checklist
- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Sumary

Sandboxed containers will be integrated into OpenShift to provide the ability
to run kernel isolated containers.  Under the hood, the feature will rely on
[Kata Containers](https://katacontainers.io/), an Open Source project working
to provide a stronger workload isolation using hardware virtualization
technologies as an additional layer of defense.

  
## Motivation

The main motivation to bring sandboxed containers to OpenShift is to
provide the ability to handle the following use cases:
- Run privileged/untrusted workloads.
- Run workloads requiring administrative privileges above and beyond what is
  secure in a shared kernel environment (native linux OCI runtime - `runc`)
- Ensure kernel isolation for each workload.
- Ensure default resource containment through VM boundaries.
- Run any workload which requires:
  - Custom kernel tuning (sysctl, scheduler changes, cache tuning, etc).
  - Custom kernel modules (out of tree, special arguments, etc).
  - Exclusive access to hardware.
  - Root privileges.

### Goals

- Provide a way to enable running user workloads in sandboxed containers on
  an OpenShift cluster for additional isolation when necessary.
- Life-cycle management of the underlying sandboxed containers runtime.
- Provide a security policy to control which users can run what workloads
  in sandboxed containers in an OpenShift cluster.

### Non Goals

- Running workloads using a different kernel than the one used on the
  OpenShift node.
  - Although sandboxed containers are capable of supporting running a
    different kernel then the one used on the OpenShift node, such features
    will not be supported in order to simplify the development cycles and
    testing efforts of this project.
- Running OpenShift control plane and core operators on Sandboxed
  Containers.
  - These will continue to run on the native linux OCI runtime runc.

## Proposal

As part of our proposal, we'll cover a few user stories coming from the Telco industry.

### User Stories

#### Story 1 - Telco CNF deployments

As part of 5G/NFV telco deployments there is a gradual migration from physical
networking boxes to CNFs (container network functions). Some of these CNFs
require root access while others could potentially create a threat on the telco
cloud (as the kernel is shared in that case) by bypassing existing container
isolation mechanisms. To address these issues, a recommended way is to use a
sandboxed container runtime.

#### Story 2 - Devops deployments

Telcos are working hard to increase the rate of adding new capabilities to
their clouds for competing with OTTs (over the top companies such as Netflix,
Apple, Facebook etc…). This involves huge investments in devops tools and
processes. In order for devops to deploy new features on production
environments there is a need for high isolation of the deployed workloads in
order to control and revert such changes.

#### Story 3 - Hardware Vendors

Many hardware vendors require custom kernel parameters at boot, custom sysctl
variables, etc. They want to use containers as a convenient packaging format
for their entire application, but still need access to lower level tuning for
portions of their applications.

### Implementation Details/Notes/Constraints

For a cluster to be able to run workloads using sandboxed containers, the
following things have to happen:
- `kata-containers`, and its dependencies, have to be installed in the
  appropriate nodes.
- A runtime class has to be set up, in kubernetes.
- `CRI-O` has to be configured to use the installed `kata-containers` as
  the runtime for the newly configured runtime class.

And as a way to perform the lifecycle management (install, upgrade, and
uninstall) of the items mentioned above, the [OpenShift sandboxed containers
operator](https://github.com/openshift/sandboxed-containers-operator) was
created and is a crucial part of our project.  The operator can be used to
deploy sandboxed containers on all, or on a set, of the worker nodes of a
cluster.

Described below, you can see how the `KataConfig` CustomResourceDefinition (CRD),
implemented by the sandboxed containers operator, can be used for deployment:

- Deploying on all the worker nodes:
  ```yaml=
  apiVersion: kataconfiguration.openshift.io/v1
  kind: KataConfig
  metadata:
    name: example-kataconfig
  ```
- Deploying only on selected nodes:
  ```yaml=
  apiVersion: kataconfiguration.openshift.io/v1
  kind: KataConfig
  metadata:
    name: example-kataconfig
  spec:
    kataConfigPoolSelector:
      matchLabels:
         custom-kata1: test 
  ```

In the coming subsections, let's dive into the details of how the OpenShift
sandboxed containers operator covers each one of the points present above.

#### Installation of kata-containers dependencies and artifacts on the node

The following artifacts are installed in each one of the nodes where
sandboxed containers workloads are supported:

package         | dependency of   | rpm size |  installed size
----------------|-----------------|----------|-----------------
ipxe-roms-qemu  | qemu-kiwi       | 1.3M     |   2.58M
kata-containers |                 |  46M     | 130.72M
libpmem         | qemu-kiwi       |  79K     |    255K
pixman          | qemu-kiwi       | 258K     |    682K
qemu-kiwi       | kata-containers | 2.7M     |  12.08M
qemu-kvm-common | qemu-kiwi       | 919K     |   2.68M
seabios-bin     | qemu-kiwi       | 133K     |    384K
seavgabions-bin | qemu-kiwi       |  43K     |    210K
sgabios-bin     | qemu-kiwi       |  14K     |      4K

The desired way of installing those in the nodes is taking advantage of
[CoreOS Extensions](https://github.com/openshift/enhancements/pull/317),
which adds support for cluster operators (notably the MCO) to deploy
non-containerized software that is tested and versioned along with the OS,
but not installed by default.

**Pros:**
1. There's no extra work related to properly loading QEMU dependencies, as
   all the packages would be in installed in the same PATH its build was
   intent for.
2. QEMU RPMs have been tested / validated using an identical environment.
3. No additional work with regarding to re-packaging those in another form
   other than the RPMs which is the way they are consumed by other Layered
   Products.
4. We rely on an already existing mechanism to install all of the RPMs -
   QEMU, QEMU dependencies, and Kata Containers - which are not part of the
   base image.
5. There's no chance of hitting issues related to parallel installing
   packages, as the whole package installation is handled by the MCO.
6. Updates / Removal become trivial as they are also handled by the MCO.

**Cons:**
1. 51.5MB extra to be downloaded - coming from QEMU, QEMU dependencies, and
   Kata Containers - as part of the RHCOS Extensions, for all users relying
   on extensions; ~150MB installed for those using the sandboxed containers
   extension.

As an **alternative** to the above option, we could partially rely on [CoreOS
Extensions](https://github.com/openshift/enhancements/pull/317), but this time
only for installing `qemu-kiwi` and its dependencies.  The content of the
`kata-containers` RPM would then be copied to the cluster on
`/opt/sandboxed-containers/`.

**Pros:**
1. 5.5MB extra to be download - coming from QEMU and its dependencies - as
   part of the RHCOS Extensions, for all users relying on extensions; ~20MB
   installed for those using the QEMU extension.
2. There's no extra work related to properly loading QEMU dependencies, as
   all the packages would be in installed in the same PATH its build was
   intent for.
3. QEMU RPMs have been tested / validated using an identical environment.
4. No additional work with regarding to re-packaging those in another form
   other than the RPMs which is the way they are consumed by other Layered
   Products.
5. We rely on an already existing mechanism to install part of the RPMs, QEMU
   and its dependencies, which are not part of the base image.

**Cons:**
1. 130MB extra - coming from Kata Containers binaries - copied onto the host,
   for those who'd be using the Sandboxed Containers,
2. The sandboxed containers operator still does a work that, in theory,
   should be restricted to the MCO.
3. The sandboxed containers operator would have to implement update, upgrade,
   and downgrade logic, instead of relying on the MCO to do so.

#### Setting up the `kata` runtime class, in Kubernetes

Kubernetes provides support for [Runtime Class](https://kubernetes.io/docs/concepts/containers/runtime-class/),
a feature for selecting the container runtime configuration, which is used to run a Pod's containers.

A Runtime Class is created, and named as `kata`, in order to ease the
adoption for those already coming from using Kata Containers with
Kubernetes.

It's important to note that the Runtime Class support two features which are
quite important for sandboxed containers:
- Scheduling, which ensures Pods land only on nodes supporting the specific
  Runtime Class.
- Pod Overhead, which allows us to specify overhead resources that are
  associated to a running pod, such as `QEMU` and `virtiofs` overhead, in our
  specific case.

Example `kata` Runtime Class that gets created
```yaml=
apiVersion: node.k8s.io/v1
handler: kata
kind: RuntimeClass
metadata:
  name: kata
overhead:
  podFixed:
    cpu: 250m
    memory: 160Mi
scheduling:
  nodeSelector:
    node-role.kubernetes.io/worker: ""
```

#### Configuring CRI-O to support `kata` runtime

`CRI-O` supports drop-in configuration files, and we take advantage of that
feature to drop-in a kata-containers configuration file, which looks like:
```ini=
[crio.runtime.runtimes.kata]
  runtime_path = "/usr/bin/containerd-shim-kata-v2"
  runtime_type = "vm"
  runtime_root = "/run/vc"
  privileged_without_host_devices = true
```

This configuration is the glue between the installed RPMs and the Runtime
Class added in Kubernetes, specifying which container runtime that will be
used to run a workload using `kata` as its Runtime Class.

#### Risks and Mitigations

##### Metrics

While working on the integration of Kata Containers, which is used under the
hood by sandboxed containers, on to OpenShift, it's been realised that
metrics won't fully work as expected during the tech preview.

This happens because OpenShift uses `cAdvisor` for its monitoring tools, and
`cAdvisor` is not capable of collecting the **container** information of a
Kata Containers, which is running inside the a VM.  **pod** information,
though, which includes the `containerd-shim-kata-v2`, the  `QEMU`, and the
`virtiofs` processes, is properly collected.

OpenShift plans to switch  to `CRI stats` in the future, which is part of an
effort lead by Peter Hunt (@haircommander) and that can be seen
[here](https://github.com/kubernetes/enhancements/pull/2364).

##### Overhead

There is an overhead involved in the usage of sandboxed containers.
This is due to the usage of `Qemu` and `virtiofs` to run the sandboxed
containers.

The default overhead is specified in the `kata` Runtime Class and is described
below:
```yaml=
overhead:
  podFixed:
    cpu: 250m
    memory: 160Mi
```

## Design Details

### Open Questions

### Test Plan

sandboxed containers team is already in touch with OpenShift QE and is
currently performing tests related to:
- The operator features installability.
- Basic lifecycle of pods using sandboxed containers.
- Command-line tools integration.
- Logs, events, and metrics visiblity.
- Terminal availability.
- Scaling (both vertical and horizontal).
- CPU and memory restrictions.
- Pods behaviour based on node cordon/drain scenarios.

Apart from those, upstream Kata Containers project is already onboarded as part
of the OpenShift CI, which runs, daily, the `openshift-e2e` tests.  The same
set of tests, `openshift-e2e`, is planned to run against the downstream
releases.

### Graduation Criteria

#### Tech Preview -> GA

- Supported upgrade, downgrade, and scale.
- Sufficient time for feedback.
- Solid automated process for building sandboxed containers operators and
  everything it consumes.
- Solid user-facing documentation.
- Ability to gate users from using sandboxed containers (or from using native linux containers).
  - See [this](https://github.com/openshift/enhancements/pull/562) proposal for more details.

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature.
- Deprecate the feature.

### Upgrade / Downgrade Strategy

Considering that `kata-containers` and its dependencies will be part of CoreOS Extensions, the
upgrade / downgrade will be handled by the MCO.

### Version Skew Strategy

Considering that `kata-containers` and its dependencies will be part of CoreOS Extensions, the
version skew strategy will be handled by the MCO.

## Implementation History


## Drawbacks


## Alternatives


## Infrastructure Needed [optional
