---
title: bpfd-on-ocp
authors:
  - "@anfredette"
  - "@astoycos"
  - "@Billy99"
  - "@dave-tucker"
reviewers:
  - "@TBD"
approvers:
  - "@TBD"
api-approvers: 
  - "@TBD"
creation-date: 2023-02-21
last-updated: 2023-02-21
tracking-link:
see-also:
replaces:
superseded-by:
---

# BPF Support for OCP with bpfd

## Summary

This enhancement proposes the following:
- That the bpfd operator be included in OCP as a foundational operator.
- That the bpfd operator be used by internal Red Hat OCP applications and
  components for the loading and management of required BPF programs.
- That bpfd be recommended and supported for use by OCP partners and users to
  load and manage their BPF programs when running on OCP.

## Motivation

BPF is a powerful technology that OCP developers and customers alike are
increasingly trying to leverage. However, there are currently many challenges
with using BPF programs in Kubernetes-based systems.

[Enhancement Proposal
1133](https://github.com/openshift/enhancements/pull/1133), “Guidelines for the
Use of eBPF in OCP”, opened by Dan Winship last year, resulted in many
questions/concerns such as:

- How should multiple BPF programs share the same hook points? 
- How should BPF be utilized in OCP by the infrastructure team AND customers
  alike? 
- How can multiple infra operators manage BPF programs without duplicating
  effort?
- How does BPF in OCP affect cluster debugging?

Additional challenges with the proliferation of BPF in OCP include security,
visibility, debuggability, and lifecycle management of BPF programs in general.

Finally, without a common infrastructure for loading and managing BPF programs,
each developer needs to solve these problems independently, which results in
duplicated code and functionality.

[Bpfd](https://bpfd.netlify.app) is a system daemon that supports, loading,
unloading, modifying, and monitoring BPF programs. The solution for Kubernetes
uses an operator, built with the [Operator
SDK](https://sdk.operatorframework.io), that can deploy the bpfd daemon and a
bpfd agent to all the nodes in a cluster. The agent supports well-defined
Kubernetes APIs that can be used to manage the lifecycle of BPF programs across
a Kubernetes cluster.

The benefits of this solution include the following:
- Improved security because only the bpfd daemon, which can be tightly
  controlled, requires the privileges needed to load BPF programs, while access
  to the API can be controlled via standard RBAC methods. This could also allow
  us to remove CAP_BPF from the privileged container defaults.
- Improved ease of use and productivity because all developers use a standard
  framework for managing BPF programs which reduces duplicated efforts and the
  learning curve.
- Support for the coexistence of multiple BPF programs from multiple users.
- Improved visibility into what BPF programs are running on a system, which
  enhances the debuggability for developers, admins, and customer support.

### User Stories

1. As an OCP developer, I would like to have a consistent solution for loading,
   managing, and debugging across all of our BPF-enabled applications and
   components.

   Without a common solution, each application must solve many of these problems
   on its own, which leads to duplicated code, functionality, and/or different
   solutions. For example, we currently have three different OCP applications
   (Ingress Node Firewall, Network Observability, and ACS) that use different
   methods of loading and managing BPF programs. A common framework can also
   simplify the debugging of interactions between BPF applications and make it
   easier for developers to work on multiple projects.

2. As an OCP Developer/product manager, I would like to introduce functionality
   in a future release that uses BPF programs and have these new features
   interact in a predictable way with existing functionality, other operators,
   and local customizations.

   BPF allows low-level features to be programmed much more quickly than via
   traditional methods, e.g submitting kernel patches. Developers should be able
   to easily ship new functionality which makes use of BPF in Openshift without
   worrying about clobbering other developers who intend to make use of similar
   hook points and BPF programs. 

3. As an OCP Customer, I want to deploy BPF programs in a supported way. 

   As an OCP customer, I want to work around an OCP/RHEL bug by using a
   MachineConfig (or something similar) to deploy a small BPF program to every
   node in my cluster, without voiding my warranty.

   As an OCP customer, I want to add functionality to OCP/RHEL by using a
   MachineConfig (or something similar) to deploy a small (or not-so-small) BPF
   program to every node in my cluster, without voiding my warranty.

   (E.g., there is a customer who wants to use BPF to implement OVS-style SLB
   Bonding on a kernel bond interface, using BPF. The upstream Linux maintainer
   rejected a patch to implement it in the kernel driver directly, in part
   because it would be so easy to implement it with BPF.)

   As an OCP customer, I want to use existing probes from projects like bcc,
   bpftrace, etc. to provide insights into my infrastructure or manage and
   monitor workloads that I have deployed.

4. As an OCP Cluster Admin, I want to know ALL of the BPF programs deployed in
   my Cluster.

   As more and more kubernetes applications utilize BPF, cluster stability may
   ultimately be threatened. In order to mitigate this risk admins should be
   able to easily see all of the BPF programs loaded on a given cluster.

5. As an OCP Developer or support engineer I would like to easily utilize
   advanced BPF debugging tools. 

   Tools such as bpftrace provide unprecedented insight into the inner workings
   of the kernel, however, support for deploying and running those tools in OCP
   is very limited. 

6. As an OCP Developer, I need a centralized way to package and deploy BPF
   applications that follow secure supply chain best practices.

   Specifically, it should be simple for me to have fine-grained control over
   the versioning of both the user space and kernel space portions of my BPF
   applications. Additionally, shipping changes to both portions of the
   application should be straightforward and make use of existing
   infrastructure, e.g container registries. Lastly, I need to follow supply
   chain security best practices with BPF, e.g bytecode signing and integration
   with Sigstore

7. As an OCP Admin, I want to apply policy around BPF to control which users and
   applications can/cannot perform BPF operations. 

   Some BPF programs are critical to a functioning datapath while others may
   simply report data back to the user. Therefore policy regarding the actors
   who can load such programs should be explicit and easily integrated with OCP
   RBAC. 

8. As an OCP Developer/Admin I want to limit the container permissions required
   to load and run BPF programs. 

   In OCP today loading and managing BPF programs often requires that the pod be
   privileged and have both the CAP_BPF and CAP_NET_ADMIN capabilities. This is
   ultimately a major security concern and should be mitigated to ensure
   applications can load BPF programs without such privileges.

### Goals

The goal of this proposal is to provide OCP developers and customers with a
consistent and more secure experience using BPF. Whether they are consuming BPF
as a component part of OCP, from a partner product, or producing BPF themselves.

If we succeed in our goal, we will provide a solution, or at least the
infrastructure for a solution, for the user stories listed above. In particular:

- It will be simpler for OCP developers to use BPF in their work.
- Red Hat teams can collaborate on BPF development.
- OCP administrators will be able to easily discover what BPF has been loaded in
  their cluster, who loaded it, and for what purpose.
- Security will be improved because applications leveraging BPF will not need to
  run in privileged containers.

### Non-Goals

We do not want to mandate that customers ONLY use our method of working with BPF
in OpenShift.

We do not intend to provide any controls for blocking the loading of BPF, but we
may alert customers when incompatibilities are detected. Furthermore, we would
like to defer the enforcement of ANY policy to other products (i.e ACS).

## Proposal

As mentioned in the summary, this enhancement proposes the following:

- That the bpfd operator be included in OCP as a foundational operator.
- That the bpfd operator be used by internal Red Hat OCP applications and
  components for the - loading and management of required BPF programs.
- That bpfd be recommended and supported for use by OCP partners and users to
  load and manage their BPF programs when running on OCP

### Workflow Description

In general bpfd and the bpfd operator are designed to work with applications in
three different deployment models: 

1. **Manual Deployment** of a `bpfProgramConfig` by a cluster administrator with
   no accompanying application. 
2. **Manual Deployment** of a `bpfProgramConfig` by a cluster administrator with
   an accompanying application. 
3. **Dynamic Deployment** of a `bpfProgramConfig` where the application deploys
   and manages the BPF program while also interacting with it on a per-node
   basis. 

For all deployment scenarios, the application or a portion of it needs to be
deployed as a per-node agent with a mount-point linked to the bpfd-fs
`/run/bpfd/fs/`. Each application agent will then use the bpfd-helpers library
(still WIP) to fetch the node-specific `bpfProgram` which is used internally to
determine the absolute pin path of the maps on the node.

For Deployment scenario 3, the application would need to have a centralized
operator. In this case, the operator will use the bpfd-helpers library to manage
the `bpfProgramConfig` object and dynamically configure fields such as bpfd
bytecode location, attach points, priority (when applicable), direction, etc.

### API Extensions

The bpfd operator uses the following APIs:

#### BpfProgramConfig CRD

The [BpfProgramConfig
CRD](https://github.com/redhat-et/bpfd/blob/main/bpfd-operator/config/crd/bases/bpfd.io_bpfprogramconfigs.yaml)
is the bpfd K8s API object most relevant to users and can be used to understand
clusterwide state for a BPF program. It's designed to express how, and where BPF
programs are to be deployed within a Kubernetes cluster. An example
BpfProgramConfig which loads a basic xdp-pass program to all nodes can be seen
below:

```yaml
apiVersion: bpfd.io/v1alpha1
kind: BpfProgramConfig
metadata:
  labels:
    app.kubernetes.io/name: BpfProgramConfig
  name: xdp-pass-all-nodes
spec:
  ## Must correspond to image section name
  name: pass
  type: XDP
  # Select all nodes
  nodeselector: {}
  priority: 0
  attachpoint: 
    interface: eth0
  bytecode:
    imageurl: quay.io/bpfd-bytecode/xdp_pass:latest
```


#### BpfProgram CRD

The [BpfProgram
CRD](https://github.com/redhat-et/bpfd/blob/main/bpfd-operator/config/crd/bases/bpfd.io_bpfprograms.yaml)
is used internally by the bpfd-deployment to keep track of per node bpf program
state such as program UUIDs and map pin points, and to report node specific
errors back to the user. K8s users/controllers are only allowed to view these
objects, NOT create or edit them. Below is an example BpfProgram Object which
was automatically generated in response to the above BpfProgramConfig Object.

```yaml
apiVersion: bpfd.io/v1alpha1
  kind: BpfProgram
  metadata:
    creationTimestamp: "2022-12-07T22:41:29Z"
    finalizers:
    - bpfd.io.agent/finalizer
    generation: 2
    labels:
      owningConfig: xdp-pass-all-nodes
    name: xdp-pass-all-nodes-bpfd-deployment-worker2
    ownerReferences:
    - apiVersion: bpfd.io/v1alpha1
      blockOwnerDeletion: true
      controller: true
      kind: BpfProgramConfig
      name: xdp-pass-all-nodes
      uid: 6e3f5851-97b1-4772-906b-3ac69c6a4057
    resourceVersion: "1506"
    uid: 384d3d5c-e62b-4be3-9bf0-c6cf0e315acf
  spec:
    programs:
      bdeac6d3-4128-464e-9161-6010684eca27:
        attachpoint:
          interface: eth0
        maps: {}
  status:
    conditions:
    - lastTransitionTime: "2022-12-07T22:41:30Z"
      message: Successfully loaded BpfProgram
      reason: bpfdLoaded
      status: "True"
      type: Loaded
```

#### Bpfd Configmap

The [bpfd
configmap](https://github.com/redhat-et/bpfd/blob/main/bpfd-operator/bundle/manifests/bpfd-config_v1_configmap.yaml)
is used to configure cluster-wide parameters.

### Implementation Details/Notes/Constraints [optional]

The high-level design of the bpfd solution and how it interacts with the above
CRDs is depicted in the following diagram.

![bpfd On ocp](bpfd-on-ocp.jpg)

More information about bpfd and how it can be used for OCP can be found in the
following resources:

- Presentation: [What can bpfd do for
  OCP?](https://docs.google.com/presentation/d/1mRF13pBk-Ynt6Z-OQ2IRHPEcgSX81h0TgI9fzdQGP2o/edit?usp=sharing)
- [The bpfd website](https://bpfd.netlify.app/). In particular, the following
  pages are useful in understanding how bpfd and the bpfd operator can be used
  by OCP:
  - [How to Deploy bpfd on Kubernetes](https://bpfd.netlify.app/k8s-deployment)
  - [Example BPF Programs](https://bpfd.netlify.app/example-bpf)
- The [Ingress Node Firewall
  POC](https://github.com/openshift/ingress-node-firewall/pull/285) demonstrates
  exactly how the bpfd operator can be used with an existing OCP feature.

### Privileges

One of the major benefits of running bpfd in kubernetes is the ability to
separate concerns in relation to privileges. Traditionally containers required
CAP_BPF and CAP_NET_ADMIN, to load and attach BPF programs. With the
bpfd-operator bpfd needs to run as a privileged container in order to
load/unload BPF programs, However, applications leveraging BPF will not. Instead
they interact with bpfd via either a GRPC API or via the kubernetes
`bpfProgramConfig` CRD and let bpfd manage the loading/unloading on their
behalf.

In kubernetes the `bpfd-bpfprogramconfig-editor` and  `bpfd-bpfprogram-viewer`
RBAC cluster roles will be deployed via the bpfd operator for use by
applications.

### Risks and Mitigations

From Enhancement Proposal 1133:

- Continuing to allow arbitrary BPF without any oversight will likely eventually
  lead to buggy clusters, difficult-to-debug problems, and possibly security
  holes.
- OTOH adopting a too-strict policy toward customer use of BPF may turn some
  customers away (and would be awkward, messaging-wise: "BPF is great! You can't
  use it!").

### Drawbacks

TBD

## Design Details

TBD

### Open Questions [optional]

TBD

### Test Plan

TBD

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

TBD

### Upgrade / Downgrade Strategy

TBD

### Version Skew Strategy

TBD

### Operational Aspects of API Extensions

TBD

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

Initial proposal: 2023-02-21

## Alternatives

The current solution is for each application to choose/implement its own method
of managing BPF programs.

## Infrastructure Needed [optional]

TBD
