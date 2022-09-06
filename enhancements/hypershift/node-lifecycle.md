---
title: node-lifecycle

authors:
  - "@enxebre"
  
reviewers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"
  - "@yuqi-zhang"
  
approvers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"

api-approvers:
  - None

tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-5771

creation-date: 2022-07-22
last-updated: 2022-07-22
---

# Node lifecycle

## Summary

This proposal fleshes out the details for the current Node lifecycle solution for HyperShift form factor i.e. hosted control planes.
This includes automated Machine management, OS and config lifecycle.

## Glossary
- MCO - [Machine Config Operator](https://github.com/openshift/machine-config-operator)
- CAPI - [Cluster API](https://github.com/kubernetes-sigs/cluster-api)
- Control plane Namespace - The Namespace lifecycled by HyperShift in the management cluster for each guest cluster control plane.

## Motivation
HyperShift differs from standalone OCP in that the components that run in the Hosted Control Plane (HCP) exists in a logical and physical network different from the guest cluster Nodes.  
This split enables hiding Node management from the cluster admin end user persona.
Challenges include but not limited to:
- Provide the ability to manage Nodes securely in multi-tenancy scenario.
- Managing compute capacity (i.e. Machines) in a guest cluster from a different management cluster.
- Managing OS upgrades for a guest cluster from a different management cluster in a centralised fashion.
- Managing OS config for a guest cluster from a different management cluster in a centralised fashion.

### User Stories
- As a Service Provider I want to have the ability to manage compute capacity in the provider clusters dynamically via declarative API, so I can satisfy end user workloads compute demand:
  - Scaling.
  - Autoscaling.
  - Autorepair.
- As a Service Provider I want to have the ability upgrade the OS and Kube version of the Nodes in place for baremetal environments, so Hosts don't need to be setup every time.
- As a Service Provider I want to have the ability upgrade the OS and Kube version of the Nodes by recreating the instances, so Hosts are recycled regularly for security hygiene.
- As a Service Provider I want to have the ability to declaratively specify different pools of Nodes, so I can setup topologies tolerant to domain failures.
- As a Service Provider I want to have the ability to declaratively specify different pools of Nodes, so I can allocate workloads with concrete architecture or hardware demands.
- As a Service Provider I want to have the ability to upgrade the control plane independently of the data plane, so they can satisfy different compliance policies.
- As a cluster Admin I want to have the ability to manipulate Nodes with Kubernetes high level primitives e.g. taints, labels, etc. Without interacting the underlying infra.

### Goals
- Provide a consumable API for Node management that satisfies HyperShift form factor (multi-tenancy and control plane/data plane decoupling) supporting the following features:
  - Scaling.
  - Auto scaling.
  - Auto repair.
  - Auto approval.
  - OS upgrades.
  - Config updates.
  - Management/workload cluster network and infra separation.
  - Multi-tenancy.

### Non-Goals

## Proposal
### API
[NodePool](https://github.com/openshift/hypershift/blob/27c0a432bdc8d702f1cdb2a2f3f25e5ae6fbee7d/api/v1alpha1/nodepool_types.go) is the consumer facing API exposed for Node management.
Its reason to exist is to preserve Red Hat ability to satisfy consumer needs and evolve at our own pace while reusing and relying on battle proven technologies for the implementation details.

### Automated compute capacity management
#### Cluster API
Having a NodePool API creates the need for machinery to satisfy their intent. We choose to leverage [CAPI](https://github.com/kubernetes-sigs/cluster-api) as the NodePool implementation.
It's a project that we have been involved since early stages and contains the learnings from running the `machine.openshift.io` in production since OCP 4 release.
CAPI is now v1beta1 which gives us the API guarantees we require for customer facing APIs.
It solves the main problems intrinsic to the HyperShift form factor paradigm in this area: 
- Cloud agnostic Automated machine management.
- Awareness of management vs workload cluster.
- Ability to signal Externally managed infrastructure.
- Pluggable control plane implementation.
- Pluggable host bootstrapping implementation.

#### Implementation
The NodePool controller is meant to be a thin layer that proxies API input into CAPI resources and delegates as much as possible of any business logic implementation into CAPI controllers.
When a HostedCluster is created a CAPI Cluster and a HostedControlPlane are created as a representation in the control plane namespace. This Cluster CR satisfies the contract for any CAPI Machine scalable resource.

When a new NodePool is created:
- The NodePool controller will reconcile to create or update a "token" Secret which will serve as an index ID to access a server containing the Ignition payload for the .release specified in the NodePool (See section below to understand how the ignition payload is generated and served).
- The NodePool controller will reconcile to create or update an "userdata" Secret with an ignition URL and the "token" as a parameter.
- The NodePool controller will reconcile to create or update a MachineDeployment (`.upgradeType="Replace`) or a MachineSet (`.upgradeType="InPlace`).
- Replace:
  - When a new release version is specified in a NodePool, this results in a new userdata created and updated in the MachineDeployment which then triggers a rolling upgrade by deleting and creating new Machines while honouring maxUnavailable/maxSurge.
  - Any cloud provider specific change e.g. AWS instance type will trigger a rolling upgrade.
- InPlace:
  - When a new release version is specified in a NodePool, this results in a new userdata created and updated in the MachineSet which enables upcoming Machines to come up with the targeted version. Existing Machines are upgraded in place (See section below).
  - Any cloud provider specific change e.g. AWS instance type will only affect upcoming Machines.
- Scaling is delegated to MachineDeployment/MachineSet by reconciling `.replicas`.
- Autoscaling is delegated to the [Cluster API Autoscaler provider] (https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/clusterapi) by reconciling `.NodePoolAutoScaling` into MachineDeployment/MachineSet.
- AutoRepair is delegated to CAPI by reconciling to create or update opinionated MachineHealthCheck CRs.
- Bootstrapping CSR are handled by https://github.com/openshift/cluster-machine-approver.

### OS / kubelet and config management
In standalone OCP OS and config management is driven by the [MCO](https://github.com/openshift/machine-config-operator).
The HyperShift form factor prevents their original design from being suitable:
- ClusterScoped resources and single controller does not fit well in the single management cluster multi-tenancy model.
- Machine Config Server does not support authentication.

To overcome these limitations while reusing as much as possible of the existing MCO knowledge and learnings, some components are introduced:
- [Ignition Server](https://github.com/openshift/hypershift/tree/main/ignition-server).
- [Inplace Upgrader](https://github.com/openshift/hypershift/tree/27c0a432bdc8d702f1cdb2a2f3f25e5ae6fbee7d/control-plane-operator/hostedclusterconfigoperator/controllers/inplaceupgrader).
- `MachineConfig`, `KubeletConfig` and `ContainerRuntimeConfig` CRs can be embedded in ConfigMaps and associated to NodePools [via the `.config` the API know](https://github.com/openshift/hypershift/blob/main/api/v1alpha1/nodepool_types.go#L118-L127).

#### Ignition Server
Managed by the Control Plane Operator in the control plane namespace. 
At a high level the Ignition Server is an HTTP request multiplexer.
It serves Ignition payloads over the /ignition endpoint for a particular "Bearer $token" passed through the "Authorization" Header.

It runs a "token" Secret controller underneath.
This is the controller that generates and caches the Ignition payload for any pair of `.release` and `.config` input for each NodePool.
The TokenSecret controller watches "token" Secrets created by the NodePool controller and:
- Generates the Ignition payload for that `.release` and `.config` pair (See below for details).
- Stores the payload in memory indexed by "token" and exposes it to the Server.
- Sets a TTL for the token.
- After TTL/2 move the original token to rotated spot and create a new one. This results in both tokens for the same `.release` and `.config` pair coexisting during a TTL/2 duration. This gives in flight operations (e.g. Host booting with an userdata pointing to the rotated token) time to complete.
- Removes the token from memory after the TTL.

Payload generation:
- When generating a new payload is needed the TokenSecret controller fetches the MCO binaries for the given `.release` image.
- It runs mco controllers in bootstrap mode.
- It runs mcs in bootstrap mode and makes a local http query to get the payload served.
- It stores the payload by "token" for the Ignition server (So it can be consumed by new Machines i.e. scale up or MachineDeployment rolling upgrade operations).
- It stores the payload in a key of the token Secret (So it can be used by the Inplace upgrader i.e. upgrade of existing Machines belonging to a MachineSet).

Deletion:
- When a NodePool `.release` or `.config` is changed, a new token secret is created by the NodePool controller that corresponds to the new pair.
- The old token Secret is marked with an expiration date by the NodePool controller.
- Once the expiration date has passed, the old token Secret is deleted by the TokenSecret controller and its counterpart tokens in the Ignition Server memory are deleted. This strategy is done to allow in flight provisions that occurred in proximity to the nodePool upgrade to complete.

#### Inplace Upgrader
Managed by the HostedClusterConfigOperator (which is managed by the Control Plane Operator) in the control plane namespace.
The Inplace Upgrader is a controller that watches both MachineSets (management cluster) and Nodes (guest cluster) and orchestrates in place upgrades for a pool of Nodes honouring the following design invariants:
- Draining and any other step of the process but the upgrade itself are performed in a centralised fashion. This reduces the risk for high privilege Service Accounts permissions to be exposed in Nodes and therefore being vulnerable to containers escape.
- Prevent the cluster end user as much as possible from interfering in the product owned behaviour.

The overall workflow closely matches self-driving OCP's Machine-Config-Controller. The Inplace Upgrader is similar to the MCO node-controller and the Drainer is similar to the MCO drain-controller.

During the in place upgrade process as of today annotations are used to signal back and forward between NodePool<->MachineSets and between MachineSets<->Nodes.
This might in future evolve to a different form of contract e.g. CRD.

Workflow:
- The NodePool controller signals an upgrade (`.release` and `.config` pair change) via an annotation in the underlying MachineSet.
- The Inplace Upgrader watches this.
- It fetches all Nodes for the MachineSet, starting the upgrade process one by one while honouring maxUnavailable.
- The Inplace Upgrader creates update manifests, first creating a Secret in the guest cluster with the payload content propagated from the token Secret.
- The Inplace Upgrader then runs the machine-config-daemon (upgrader Pod) in the guest cluster targeted Node and mounts the secret with the payload.
- The machine-config-daemon pod processes the update, then signals back via Node annotations to request draining when needed.
- The Drainer, watching the Nodes, performs the drains for Nodes requesting drain, and signals completion via another Node annotation.
- The machine-config-daemon performs the upgrade in place, reboots the Node, and signal to uncordon the Node.
- The Drainer performs the uncordon and signals completion via annotation.
- The machine-config-daemon finally signals completion state via annotating the Node.
- The Inplace Upgrader watches the Node annotation and moves on appropriately, short-circuiting (in case of failure) or proceeding to the next Node, until all Nodes are at the targeted version. 

### Workflow Description
The [NodePool API](https://github.com/openshift/hypershift/blob/27c0a432bdc8d702f1cdb2a2f3f25e5ae6fbee7d/api/v1alpha1/nodepool_types.go) is the entrypoint and consumer facing API for any human or automation to interact with Node lifecycle features.
A consumer can express intent for scaling, autoscaling, autorepair or upgrades via API input.
Usually a [Service Provider](https://hypershift-docs.netlify.app/reference/concepts-and-personas/) would interact with this API directly to satisfy Service consumer demands.



### API Extensions
N/A.
### Risks and Mitigations
N/A.
### Drawbacks
N/A.
### Test Plan
N/A.
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
The following features are aimed to be supported and go through e2e automated testing before GA:
- Scaling.
- Autoscaling.
- Autorepair.
- In place upgrades.
  - Centralised draining and failure recovery.
- Replace upgrades.

#### Removing a deprecated feature
### Upgrade / Downgrade Strategy
N/A.
### Version Skew Strategy
N/A.
### Operational Aspects of API Extensions
N/A.
#### Failure Modes
#### Support Procedures

## Alternatives


## Design Details

### Graduation Criteria

## Implementation History
The initial version of this doc represents implementation as delivered via MCE tech preview.