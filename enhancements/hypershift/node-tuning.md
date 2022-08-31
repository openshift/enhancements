---
title: node-tuning

authors:
  - "@dagrayvid"
  
reviewers:
  - "@jmencak"
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"
  
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
  - https://issues.redhat.com/browse/PSAP-742

creation-date: 2022-08-31
last-updated: 2022-08-31
---

# Node Tuning on HyperShift

## Summary

This enhancement describes how the [Node Tuning Operator (NTO)](https://github.com/openshift/cluster-node-tuning-operator) will run in HyperShift’s managed control plane to manage tuning of hosted cluster nodes via the containerized TuneD daemon.
The required changes can be summarized into the following three phases.

Phase 1: Enable NTO to be used for setting sysctls via TuneD and managing default tunings applied to hosted cluster nodes:
- HyperShift repo:
  - Enable deploying NTO in the control plane namespace via the control-plane-operator. NTO will have two kubeconfigs for accessing the hosted and management clusters.
  - Add `spec.tuningConfig` field to the NodePool API. Similarly to the `spec.config` field, it will be a list of references to ConfigMaps in which Tuned manifests are stored.
  - Reconcile `spec.tuningConfig` in the NodePool controller. Embed the Tuned configuration into a ConfigMap per NodePool in the control plane namespace.
- Node Tuning Operator repo:
  - NTO needs to support the option of having two kubeconfigs
  - NTO needs to know whether it is running in HyperShift (based on env variable)
  - When running in HyperShift, NTO needs to get Tuneds from the ConfigMap(s) in the hosted control-plane namespace and create the Tuned objects in the hosted cluster (reconciling the Tuneds in the Hosted Cluster to always match those in the ConfigMap(s))
  - NTO needs to create the operand (containerized TuneD daemon) and Profile objects in the Hosted Cluster.

Phase 2: Enable NTO to be used for setting kernel boot arguments calculated by TuneD on hosted cluster nodes
- HyperShift repo:
  - Watch and reconcile NTO-generated (by the operator) ConfigMaps containing MachineConfigs for setting kernel parameters.
- Node Tuning Operator repo:
  - Change NTO to embed generated MachineConfigs for setting kernel parameters into ConfigMaps with a specific label that will get picked up by the NodePool controller.
  - Change NTO to label these ConfigMaps by NodePool. 

Phase 3 (4.13+): Enable Performance Addon Operator / PerformanceProfile controller functionality.
In OCP 4.11 the Performance Addon Operator -- which is owned by the CNF team -- has been merged into the Node Tuning Operator, as a separate controller running under the same binary.

## Motivation
See the HyperShift [project goals](https://hypershift-docs.netlify.app/reference/goals-and-design-invariants/).
A guest cluster in HyperShift should ideally run only user workloads and it should decouple the control and data plane.
As much as possible, these goals should not come at the cost of giving up any existing OpenShift features.
In standalone OpenShift the cluster Node Tuning Operator (NTO) manages applying the default tuning for control-plane and worker nodes, as well as enabling users to apply custom TuneD profiles tailored to their workloads.
Due to changes in HyperShift environments like how MachineConfigs are created and managed, some changes are needed to NTO to enable these features on hosted clusters. Currently, in OCP 4.11, NTO is fully disabled on HyperShift, though the CRDs are created in the hosted cluster.

For some background on NTO architecture in standalone OCP and links to documentation, see the background section at the end of the document.


### User Stories
See [HyperShift Personas](https://hypershift-docs.netlify.app/reference/concepts-and-personas/) for definitions of some of the roles used below.
- As a Cluster Instance Admin / User, I want the cluster to run as few OCP infrastructure workloads as possible to have the cluster just for my applications.
- As a Cluster Instance Admin, I want the cluster to be as robust as possible, I should not be able to break it by my actions.
- As a Cluster Instance Admin / User, I don't want OCP infrastructure components running there to have elevated RBACs, so a compromised component cannot break my cluster (too much).
  - The containerized TuneD daemon must run with privileges on each worker node in the hosted cluster in order to apply sysctls and other tunings to the host OS.
- As a Cluster Service Consumer, I want to be able to apply custom TuneD profiles to my nodes in order to set custom node-level settings to improve the performance of the applications that my Cluster Instance User is running.
- As a Cluster Service Consumer, I want to be able to tune my Nodes with custom kernel boot parameters calculated by TuneD.
- As a Cluster Instance Admin and Cluster Service Consumer,  I don’t want the users of my hosted cluster to be able to apply custom node / kernel level tunings on hosted cluster nodes.


### Goals
- Run the Node Tuning Operator in the HyperShift management cluster hosted control plane namespaces. 
- Make the Tuned and Profile CR objects in the hosted cluster always subservient to configuration in the management cluster.
- Add a field to the NodePool API for defining the Tuned profiles that should be applied to the hosted nodes in that NodePool. The `spec.tuningConfig` field of the NodePool API will be a list of references to ConfigMaps containing Tuneds.

### Non-Goals
N/A?

## Proposal

The Node Tuning Operator will run in the hosted control plane namespace, alongside several other control plane components.
The NTO Deployment will be managed by the control-plane-operator, along with the NTO metrics Service and ServiceMonitor.
NTO will have two kubeconfigs, so that it can read / write objects in the management cluster (ConfigMaps) and manage the Tuneds, Profiles, and the operand DaemonSet in the hosted cluster as it does on standalone OCP.
The Tuned DaemonSet (containerized TuneD daemon) will continue to run on all worker Nodes as a privileged Pod as it does in standalone OCP.

Tuned CRD / CRs will be in the hosted cluster, but the CRs will be subservient to configuration in the management cluster. Profile CRDs and CRs (one per node) will also be in the hosted cluster.
We will add a `spec.tuningConfig` field to NodePool API, containing a list of object references.
This field will be used to specify a list of ConfigMaps containing Tuned object manifests (see API extensions).
The NodePool controller will reconcile these and create one-ConfigMap-per-NodePool in the control plane namespace containing the Tuned objects. (see Workflow Description below).

(Phase 2) The NodePool controller will watch and apply NTO-generated MachineConfigs.
NTO will create at most one MachineConfig per NodePool. These will be embedded in ConfigMaps and given a specific label by NTO. They will be created in the hosted control plane namespace.

### Workflow Description
(Phase 1) How Tuneds will be defined on the management cluster and applied to the hosted nodes
1. Cluster Service Consumer will create Tuned objects inside of ConfigMaps, under the key `tuning`. They reference these ConfigMap objects in the `spec.tuningConfig` section of the NodePool API.
2. The NodePool controller will propagate these Tuned manifests into a ConfigMap (one per NodePool) in the hosted control plane namespace, that NTO can look at.
3. NTO Operator will watch the ConfigMaps in the management cluster hosted control plane namespace that contain Tuned manifests (based on a label) and will sync the Tuneds in the hosted cluster to match the Tuned objects defined in these ConfigMaps.  Once NTO has created the Tuneds in the hosted cluster, they will be reconciled in the same way as they would be on standalone OCP.
    - NTO will ensure that the Tuneds in the hosted cluster are always in sync with those defined in the ConfigMaps, so any changes by hosted cluster users would be overwritten.
    - NTO will ensure that any Tuned profiles created by the admin (via ConfigMap) are only applied to Nodes in the NodePool in which the ConfigMap referenced.
4. The containerized TuneD daemon running on each Node will apply the TuneD profile that the operator calculates to match to that Node. The TuneD status will be reported to the Profile object (one per node). The NTO operator will update the ClusterOperator status according to the operator's own status, and the status from the Profile objects.

(Phase 2) How NTO components will generate and apply MachineConfigs:
1. The containerized TuneD daemon (operand) will calculate the kernel boot parameters that should be set on the node according to the admin defined TuneD profile (if any), and will write the kernel boot parameters to the Profile status for the Profile object corresponding to the node on which the daemon is running (same as standalone OCP). 
2. NTO will generate a MachineConfig for setting the calculated kernel boot parameters to the whole NodePool and will embed the MachineConfig into a ConfigMap. This ConfigMap will have a specific label to specify that it is an NTO-generated MachineConfig, and another label for the NodePool name that it corresponds to.
    - Similar to a limitation in standalone OCP with MachineConfigPools, at most one MachineConfig will be generated per NodePool by NTO. It will be documented that if users wish to apply different kernel boot parameters to Nodes, they cannot be in the same NodePool.
3. The NodePool controller will need to watch for these NTO generated ConfigMaps in the hosted control plane namespace and reconcile the corresponding NodePool if one is created / updated.
    - There will be at most one NTO-generated MachineConfig per NodePool, and they will only change based on admin changes to the MachineConfig or (sometimes) after a cluster upgrade has completed and the containerized TuneD daemon has recalculated the kernel boot parameters on the upgraded RHCOS.


### API Extensions
We will add a field to the NodePool `spec` called `tuningConfig`. This field will contain a list of object references to ConfigMaps. This field will be used to specify a list of ConfigMaps containing Tuned object manifests.

### Risks and Mitigations
- Since Tuned CRs will exist in the hosted cluster, an admin user of this cluster could modify the Tuned objects directly, giving them the ability to change the OS-level tunings.
  - We mitigate this issue by making the Node Tuning Operator sync these Tuneds with the ConfigMaps in the hosted control plane namespace before calculating the profile for the containerized TuneD daemon to apply.

### Drawbacks
N/A.

### Test Plan
N/A.

#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature
N/A as we are going straight to GA.

### Upgrade / Downgrade Strategy
In general, the upgrade will be very similar to standalone cluster, just the operator will not be started by CVO, but by control-plane-operator. 
In phase 3, NTO-generated MachineConfigs may change after an upgrade, but not during an upgrade. The MachineConfigs would change if the kernel or TuneD daemon version changed on the Node in a way that resulted in different calculated kernel boot parameters. This would not happen until NTO is upgraded and rolls out the containerized TuneD DaemonSet.

### Version Skew Strategy
N/A.

### Operational Aspects of API Extensions
N/A.

#### Failure Modes

#### Support Procedures

## Alternatives
A few alternative ideas were considered while planning the design outlined in this enhancement:
- Run the TuneD daemon directly on the hosted node OS rather than in a privileged container.
  - We did not pursue this option as it would entail significant changes to the NTO design. It does not seem to address the biggest challenges involved in enabling NTO on HyperShift such as how to move the TuneD configuration out of the hosted cluster and into the management cluster, how to get the TuneD profiles to the hosted nodes
- Run the NTO operator in the hosted cluster and allow hosted cluster admins to modify the hosted node OS-level settings. 
  - This would likely be the simplest approach from the NTO side, but is not in-line with the design goals of the HyperShift project.

## Design Details
The changes for phase 1 and phase 2 have already been merged:
- Phase 1:
  - NTO: https://github.com/openshift/cluster-node-tuning-operator/pull/390
  - HyperShift: https://github.com/openshift/hypershift/pull/1651
- Phase 2:
  - NTO: https://github.com/openshift/cluster-node-tuning-operator/pull/456
  - HyperShift: https://github.com/openshift/hypershift/pull/1729
- Minor follow-up changes:
  - https://github.com/openshift/cluster-node-tuning-operator/pull/452
  - https://github.com/openshift/cluster-node-tuning-operator/pull/481
  - https://github.com/openshift/cluster-node-tuning-operator/pull/491
  - https://github.com/openshift/hypershift/pull/1763
  - https://github.com/openshift/hypershift/pull/1802


## Background
### NTO architecture on standalone OCP
- CRDs:
  - Tuned CRD: defines a set of TuneD profiles, and some rules to map them to nodes based MachineConfig labels 
  - Profile CRD: one-per-node, contains information about which TuneD profile defined in some Tuned CR is being applied to which node, and the status of the application.
- Operator: reconciles Tuned CR’s, creates Profile CR’s one per node.
  - RBAC:
    - Tuneds: create, get, delete, list, update, watch, patch
    - Tuned/finalizers: update
    - Profiles: create, get, delete, list, update, watch, patch
- Operand TuneD daemonset: runs the TuneD daemon in a privileged Pod on every node which watches / reads the Tuned CRD and watches / reads / updates the Profile CRD. 
  - RBAC:
    - Tuneds: get, list, watch
    - Profiles: get,list,update,watch,patch
  - Look at the Tuned objects to get the set of potential profiles to apply
  - Check the Profile corresponding to the Node on which it is running to see which TuneD profile should be applied, as calculated by the operator.
  - Apply the correct TuneD profile to the Node.
  - Keeps the TuneD daemon running as is required by certain types of tuning. Keeps track of state and does rollback on profile change or shutdown.
  - Set some status information in the Profile CR like whether the profile was applied successfully, any TuneD errors or warnings during application, and the bootcmdline calculated by TuneD.


### Graduation Criteria

## Implementation History
