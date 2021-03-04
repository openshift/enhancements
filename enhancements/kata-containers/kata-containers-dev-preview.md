---
title: kata-containers-dev-preview
authors:
  - "@aadam"
reviewers:
  - "@mpatel"
approvers:
  - "@mpatel"
creation-date: 2020-06-07
last-updated: 2020-06-07
status: implementable
see-also:
  - TBD
replaces:
  - TBD
superseded-by:
  - TBD
---

# Kata Containers

## Summary

[Kata Containers](https://katacontainers.io/) is an open source project developing a container runtime using virtual machines and providing the same look and feel as vanilla containers.
By leveraging hardware virtualization technologies, Kata Containers provides powerful workload isolation compared to existing container solutions.

We will be integrating Kata Containers into OpenShift to provide the ability to run kernel isolated containers for any workload which requires:
- custom kernel tuning (sysctl, scheduler changes, cache tuning, etc);
- custom kernel modules (out of tree, special arguments, etc);
- exclusive access to hardware;
- root privileges;
- any other administrative privileges above and beyond what is secure in a shared kernel environment (regular runc).

It should be noted that Kata Containers differ from Kubevirt:

- Kubevirt aims to run **VM images** on OpenShift providing the VM with the look and feel of a virtual machine running as a legacy VM (CRD defining a Virtual Machine in Kubernetes)
- Kata Containers aim to run **container images** on OpenShift in an isolated manner using virtualization tools (uses Runtime Class resource to choose Kata only for specific workloads which require kernel level isolation)

## Motivation
If we take telcos for example, there are a number of reasons they require isolated workloads:

1. **Telco CNF deployments** - As part of 5G/NFV telco deployments there is a gradual migration from physical networking boxes to CNFs (container network functions). Some of these CNFs require root access while others could potentially create a threat on the operators cloud bypassing existing container isolation mechanisms. For addressing those issues a hardened isolated container solution is required
2. **Devops deployments** - Telcos are working hard to increase the rate of adding new capabilities to their clouds for competing with OTTs (over the top companies such as Netflix, Apple, Facebook etc…).
                            This involves huge investments in devops tools and processes. In order for devops to deploy new features on production environments there is a need for high isolation of the deployed workloads in order to control and revert such changes.
3. **Hardware Vendors** - many hardware vendors require custom kernel parameters at boot, custom sysctl variables, etc. They want to use containers as a convenient packaging format for their entire application, but still need access to lower level tuning for portions of their applications


## Terminology
- **Kata artifacts** - includes the upstream kata project components and RHEL-virt artifacts including QEMU
- **RuntimeClass** - see https://kubernetes.io/docs/concepts/containers/runtime-class/


## Enhancement Goals
- Provide a way to enable running user workloads on kata containers on an OpenShift cluster.
- Clusters with kata container workloads should support upgrades.
- Add security policy to control which users can run what workloads on kata in an OpenShift cluster.


## Enhancement Non-Goals
- Although kata containers are capable of supporting running a different kernel then the one used on the OpenShift node, such features will not be supported. This is in order to simplify the development cycles and testing efforts of this project.
- Running OpenShift control plane and core operators on kata. These will continue to run on runc.


## Kata containers architecture

For details on the kata architecture see https://github.com/kata-containers/documentation/blob/master/design/architecture.md

## Proposal - main development efforts
### Overview
Kubernetes provides support for RuntimeClasses. RuntimeClass is a feature for selecting the container runtime configuration. The container runtime configuration is used to run a Pod’s containers.

CRI-O today comes out of the box with a runc as the default runtime. CRI-O also supports RuntimeClasses and using this configuration, it will support a KataContainers runtime as well.

The 2 fundamental problems we need to address for getting kata to work with OpenShift are:

1. Getting the kata artifacts on to an RHCOS node
2. Deploying kata and configuring CRI-O to use it as the runtime

For addressing the first issue of packaging and installing kata in openshift we will introduce 2 approaches, a short term and a long term.

For addressing the second issue of deploying and configuring CRI-O we will use an operator.

The next sections detailed the 2 solutions.

### KataContainers Packaging
#### Short term (preview release): Use container images to deliver RPMs and install using rpm-ostree
In this option we build a container image containing RHEL AV RPMs (such as QEMU) and kata RPMs. The kata operator will create a pod based on this image and then mount the host filesystem and install the RPMs on the filesystem (kata operators will be described in a later section).

**Pros:**
1. The RPMs can be installed only by the users who would be interested in using the kata runtime
2. There's no extra work related to properly loading qemu-kiwi dependencies, as all the packages would be in installed in the same PATH its build was intent for
3. QEMU RPMs have been tested / validated using an identical environment
4. No additional work with regarding to re-packaging those in another form other than the RPMs which is the way they are consumed by other Layered Products

**Cons:**
1. 20 MB extra, installed in the host, for those who'd be using kata runtime
2. Updates / Removal may be more complicated than having the RPMs as part of machine-os-content

#### Long term: Use RHCOS extensions (qemu-kiwi and dependencies only)
In this option we build upon the RHCOS extension approach planned for OCP4.6.

**What is the qemu-kiwi RHCOS extension?**

This will add additional software onto the host - but this software will still be versioned with the host (included as part of the OpenShift release payload) and upgraded with the cluster.

The additional RPMs required for qemu-kiwi and its dependencies are:
* qemu-kiwi
* qemu-kvm-common
* pxe-roms-qemu
* libpmem
* pixman
* seabios-bin
* seavgabions-bin
* sgabios-bin

See additional details about RHCOS extensions in general in: https://github.com/openshift/enhancements/pull/317

**So What do we want to do with this?**

We start by creating a container image with only the kata upstream components as static binaries (no RHEL-AV components). The kata operator will pull down this image and mount it in the host mount namespace, i.e. without running it as a container. We then use the RHCOS extensions for deploying the qemu-kiwi RHEL-AV RPMs.

**Pros:**
1. RHEL-AV depends on libraries and other tools which are all available as RPMs. Turning this into a static image instead of RPMs creates a huge single binary which we avoid with extensions
2. RHEL-AV has been tested / validated as RPMs and not a single binary which means we saves the extra overhead
3. RHEL-AV RPMs can be installed only by the customers who would be interested in using the kata runtime. QEMU (and its dependencies) get updated as part of the machine-os-content updates
4. There's no extra work related to properly loading qemu-kiwi dependencies, as all the packages would be installed in the same PATH its build in
5. Better control of the versions that are installed both for qemu (specific version for a given host) and for the dependencies (tracked by rpm)

**Cons:**
1. ~20 mb extra, installed in the host, for those who'd be using kata runtime
2. machine-os-content would still carry this until there is a separate machine-os-content- extensions container



### KataContainers Operator development
The goal is to develop a kata operator that can be used to manage the entire lifecycle of kata components in an OpenShift cluster. We will have a controller that watches for a kata **custom resource (CR)** and a daemon-set that acts upon changes to the (CR) and do the required work on the worker nodes (install, uninstall update,...).

Via the CR it will be possible to select a subset of worker nodes.

For deploying binaries on the host the current idea is to use a container image that can be mounted on the host level.  

The goal is to be able to install the operator via OperatorHub.

The Operator will:
- Create a crio drop-in config file via machine config objects;
- Automatically select the payload image that contains correct version of the kata binaries for the given version of the OpenShift;
- Configure a `RuntimeClass` called `kata` which can be used to deploy workload that uses kata runtime;
- Create a machine-config object to enable the `qemu-kiwi` RHCOS extension.

The `RuntimeClass` and payload image names will be visible in the CR status.

#### KataContainers Operator Goals
- Create an API which supports installation of Kata Runtime on all or selected worker nodes
- Configure CRI-O to use Kata Runtime on those worker nodes
- Installation of the runtimeClass on the cluster, as well as of the required components for the runtime to be controlled by the orchestration layer.
- Updates the Kata runtime
- Uninstall Kata Runtime and reconfigure CRI-O to not use it.

#### KataContainers Operator Non-Goals
To keep the Kata Operator's goal to the lifecycle management of the Kata Runtime, it will only support installation configuration of the Kata Runtime. This operator will not interact with any runtime configuration, such as Pod Annoations supported by Kata.

#### Proposal
The following new API is proposed `kataconfiguration.openshift.io/v1`

```go
// Kataconfig is the Schema for the kataconfigs API
type Kataconfig struct {
  metav1.TypeMeta   `json:",inline"`
  metav1.ObjectMeta `json:"metadata,omitempty"`

  Spec   KataconfigSpec   `json:"spec,omitempty"`
  Status KataconfigStatus `json:"status,omitempty"`
}

// KataconfigSpec defines the desired state of Kataconfig
type KataconfigSpec struct {
  // KataConfigPoolSelector is used to filer the worker nodes
  // if not specified, all worker nodes are selected
  // +optional
  KataConfigPoolSelector *metav1.LabelSelector
  
  Config KataInstallConfig `json:"config"`
}

type KataInstallConfig struct {
}

// KataconfigStatus defines the observed state of KataConfig
type KataconfigStatus struct {
  // Name of the runtime class
  runtimeClassName string `json:"runtimeClassName"`

  // +required
  kataImage string `json:"kataImage"`

  // TotalNodesCounts is the total number of worker nodes targeted by this CR
  TotalNodesCount int `json:"totalNodesCount"`

  // CompletedNodesCounts is the number of nodes that have successfully completed the given operation
  CompletedNodesCount int `json:"CompletedNodesCount"`

  // InProgressNodesCounts is the number of nodes still in progress in completing the given operation
  InProgressNodesCount int `json:"inProgressNodesCount"`

  // FailedNodes is the list of worker nodes failed to complete the given operation
  FailedNodes []KataFailedNodes `json:"failedNodes"`

  // Conditions represents the latest available observations of current state.
  Conditions []KataStatusCondition `json:"conditions"`
}

type KataFailedNodes struct {
  // Name is the worker node name
  Name string `json:"name"`

  // Error returned by the daemon running on the worker node
  Error string `json:"error"`
}

// KataStatusCondition contains condition information for Status
type KataStatusCondition struct {
  // type specifies the state of the operator's reconciliation functionality.
  Type KataConfigStatusConditionType `json:"type"`

  // status of the condition, one of True, False, Unknown.
  Status corev1.ConditionStatus `json:"status"`

  // lastTransitionTime is the time of the last update to the current status object.
  // +nullable
  LastTransitionTime metav1.Time `json:"lastTransitionTime"`
}

type KataConfigStatusConditionType string

const (
  // KataStatusInProgress means the kata controller is currently running.
  KataStatusInProgress KataConfigStatusConditionType = "KataStatusInProgress"

  // KataStatusCompleted means the kata controller has completed reconciliation.
  KataStatusCompleted KataConfigStatusConditionType = "KataStatusCompleted"

  // KataStatusFailed means the kata controller is failing.
  KataStatusFailed KataConfigStatusConditionType = "KataStatusFailed"
)

One of the ways Administrators can interact with the Kata Operator by providing a yaml file to the standard oc or kubectl command.
apiVersion: kataconfiguration.openshift.io/v1
kind: KataConfig
metadata:
  name: install-kata-1.0
spec:
  kataConfigPoolSelector:
    matchLabels:
      install-kata: kata-1.0
```

### OpenShift kata testing

- Kubernetes E2E tests, matching the ones ran by CRI-O:
https://github.com/cri-o/cri-o/blob/master/contrib/test/integration/e2e.yml#L14
- OpenShift "conformance/parallel" tests
- Get sample workloads from customers and add tests to CI if we are missing coverage for those workloads to ensure we support the workloads and don’t regress.

### OpenShift kata CI

- Leverage existing openshift CI infrastructure for e2e tests on kata-containers upstream

### Scale and performance

These comparisons to be performed by targeted microbenchmarks. Instrumentation is WIP:
- Rate of Kata pod creation/deletion comparison with standard runtime
- Network traffic throughput/latency
- Storage throughput/latency
- CPU utilization
- Memory consumption/overhead

