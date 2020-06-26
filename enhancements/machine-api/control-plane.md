---
title: Managing Control Plane machines
authors:
  - enxebre
reviewers:
  - hexfusion
  - jeremyeder
  - abhinavdahiya
  - joelspeed
  - smarterclayton
  - derekwaynecarr
approvers:
  - hexfusion
  - jeremyeder
  - abhinavdahiya
  - joelspeed
  - smarterclayton
  - derekwaynecarr

creation-date: 2020-04-02
last-updated: yyyy-mm-dd
status: provisional
see-also:
  - https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191017-kubeadm-based-control-plane.md
  - https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md
  - https://github.com/openshift/enhancements/blob/master/enhancements/etcd/disaster-recovery-with-ceo.md
  - https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/auto-cert-recovery.md
  - https://github.com/openshift/machine-config-operator/blob/master/docs/etcd-quorum-guard.md
  - https://github.com/openshift/cluster-kube-scheduler-operator
  - https://github.com/openshift/cluster-kube-controller-manager-operator
  - https://github.com/openshift/cluster-openshift-controller-manager-operator
  - https://github.com/openshift/cluster-kube-apiserver-operator
  - https://github.com/openshift/cluster-openshift-apiserver-operator
replaces:
superseded-by:
---

# Managing Control Plane machines

## Release Signoff Checklist

- [ ] Enhancement is `provisional`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Glossary

Control Plane: The collection of stateless and stateful processes which enable a Kubernetes cluster to meet minimum operational requirements. This includes: kube-apiserver, kube-controller-manager, kube-scheduler, kubelet and etcd.

## Summary

This proposal outlines a first step towards providing a single entity to fully manage all the aspects of the Control Plane compute: 
  - Ensures that Control Plane Machines are recreated on a deletion request at any time.
  - Ensures that Control Plane Machines are auto repaired when a node goes unready (Machine Health Check).

Particularly:
- This introduces a new CRD `ControlPlane` which provides a simple single semantic/entity that adopts master machines and backs them with well known controllers (machineSet and MHC):
  - The ControlPlane controller creates and manages a MachineSet to back each Control Plane Machine that is found at any time.
  - The ControlPlane controller creates and manages a Machine Health Check resource to monitor the Control Plane Machines.

This proposal assumes that all etcd operational aspects are managed by the cluster-etcd-operator orthogonally in a safe manner while manipulating the compute resources.

The contract between the etcd operations and the compute resources is given by the PDBs that blocks machine's deletion. 
Depends on https://issues.redhat.com/browse/ETCD-74?jql=project%20%3D%20ETCD.

## Motivation

The Control Plane is the most critical and sensitive entity of a running cluster. Today OCP Control Plane instances are "pets" and therefore fragile. There are multiple scenarios where adjusting the compute capacity which is backing the Control Plane components might be desirable either for resizing or repairing.

Currently there is nothing that automates or eases this task. The steps for the Control Plane to be resized in any manner or to recover from a tolerable failure (a etcd quorum is not lost but a single node goes unready) are completely manual.

Different teams are following different "Standard Operating Procedure" documents scattered around with manual steps resulting in loss of information, confusion and extra efforts for engineers and users.

### Goals

- To have a declarative mechanism to ensure that existing Control Plane Machines are recreated on a deletion request at any time.
- To auto repair unhealthy Control Plane Nodes.

### Non-Goals / Future work

- To integrate with any existing etcd topology e.g external clusters. Stacked etcd with dynamic member identities, local storage and the Cluster etcd Operator are an assumed invariant.
- To manage individual Control Plane components. Self hosted Control Plane components that are self managed by their operators is an assumed invariant:
	- Rolling OS upgrades and config changes at the software layer are managed by the Machine Config Operator.
	- etcd Guard that ensures a PDB to honour quorum is managed by the Machine Config Operator.
	- Each individual Control Plane component i.e Kube API Server, Scheduler and controller manager are self hosted and managed by their operators.
	- The Cluster etcd Operator manages certificates, report healthiness and add etcd members as "Master" nodes join the cluster.
	- The Kubelet is tied to the OS and therefore is managed by the Machine Config Operator.
- To integrate with any Control Plane components topology e.g Remote hosted pod based Control Plane.
- Automated disaster recovery of a cluster that has lost quorum.
- To manage OS upgrades. This is managed by the Machine Config Operator.
- To manage configuration changes at the software layer. This is managed by the Machine Config Operator.
- To manage the life cycle of Control Plane components
- To automate the provisioning and decommission of the bootstrapping instance managed by the installer.
- To provide autoscaling support for the Control Plane.
  - This proposal is a necessary first step for enabling Control Plane autoscaling.
  - It focuses on settling on the primitives to abstract away the Control Plane as a single entity.
  - In a follow up RFE we will discuss how/when to auto scale the the controC Plane Machines based on relevant cluster metrics e.g number of workers, number of etcd objects, etc.
- To support fully automated rolling upgrades for the Control Plane compute resources. Same reason as above.

## Proposal

Currently the installer chooses the failure domains out of a particular provider availability and it creates a Control Plane Machine resource for each of them. This introduces a `ControlPlane` CRD and controller that will adopt and manage the lifecycle of those Machines. On new clusters the installer will instantiate a ControlPlane resource.

This is a first step towards the longer term goal of providing a single entity to fully manage all aspects of the controlPlane. This iteration proposes:
- A simple single semantic/entity that adopts master machines and backs them with well known controllers (machineSet and MHC).
- To keep the user facing API surface intentionally narrowed. See [#api-changes](#api-changes)

Although is out of the scope for the first implementation, to provide long term vision and aligment this sketchs how a possible second iteration could look like:
- Abstract the `failureDomain` semantic from providers to the core machine object.
- Introduce an `InfrastructureTemplate/providerSpec` reference and FailureDomains in the ControlPlane API.
- This would provide a single provider config to be reused and to be changed across any control plane machine.
- This would give the `ControlPlane` controller all the semantics it needs to fully automate vertical rolling upgrades across multiple failure domains while provider config changes would need to happen in one single place.

The lifecycle of the compute resources still remains decoupled and orthogonal to the lifecycle and management of the Control Plane components hosted by the compute resources. All of these components, including etcd are expected to keep self managing themselves as the cluster shrink and expand the Control Plane compute resources.

### User Stories [optional]

#### Story 1
- As an operator installer a new OCP cluster I want flexibility to run [large or small clusters](https://kubernetes.io/docs/setup/best-practices/cluster-large/#size-of-master-and-master-components) so I need the ability to vertically resize the control plane in a declarative, automated and seamless manner.

This proposal satisfies this by providing a semi-automated process to vertically resize Control Plane Machines by enforcing recreation.

#### Story 2
- As an operator running an existing OCP cluster, I want to have a seamless path for my Control Plane Machines to be adopted and become self managed.

This proposal enables this by providing the ControlPlane resource.

#### Story 3
- As an operator of an OCP Dedicated Managed Platform, I want to give users flexibility to add as many workers nodes as they want or to enable autoscaling on worker nodes so I need to have ability to resize the control plane instances in a declarative and seamless manner to react quickly to cluster growth.

This proposal enables this by providing a semi-automated vertical resizing process as described in "Declarative Vertical scaling".

#### Story 4
- As a SRE, I want to have consumable API primitives in place to resize Control Plane compute resources so I can develop upper level automation tooling atop. E.g Automate Story 3 to support a severe growth peak of the number of worker nodes.

#### Story 5
- As an operator, I want faulty nodes to be remediated automatically. This includes having self healing Control Plane machines.

#### Story 6
- As a multi cluster operator, I want to have a universal user experience for managing the Control Plane in a declarative manner across any cloud provider, bare metal and any flavour of the product that have in common the topology assumed in this doc.

### Implementation Details/Notes/Constraints [optional]

To satisfy the goals, motivation and stories above, this proposes to let the installer to create a ControlPlane object to adopt and manage the lifecycle of the Control Plane Machines.

The ControlPlane CRD will be exposed by the Machine API Operator (MAO) to the Cluster Version Operator (CVO).
The ControlPlane controller will be managed by the Machine API Operator.

#### Bootstrapping
Currently during a regular IPI bootstrapping process the installer uses Terraform to create a bootstrapping instance and 3 master instances. Then it creates Machine resources to "adopt" the existing master instances.

Additionally it will create a ControlPlane resource to manage the lifecycle of those Machines:
  - The ControlPlane controller will create MachineSets to adopt those machines by looking up known labels (Adopting behaviour already exists in machineSet logic).
  	- `machine.openshift.io/cluster-api-machineset": <cluster_name>-<label>-<zone>-controlplane`
  	- `machine.openshift.io/cluster-api-cluster":    clusterID`
  - The ControlPlane controller will create and manage a Machine Health Checking resource targeting the Control Plane Machines. It will keep a `maxUnhealthy` value non-disruptive for etcd quorum, i.e 1 out of 3. Specific MHC details can be found [here](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/machine-health-checking.md)

Example:
```
apiVersion: machine.openshift.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: controlPlane 
  namespace: openshift-machine-api
spec:
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machine-role: master
      machine.openshift.io/cluster-api-machine-type: master
  unhealthyConditions:
  - type:    "Ready"
    timeout: "300s"
    status: "False"
  - type:    "Ready"
    timeout: "300s"
    status: "Unknown"
  maxUnhealthy: "34%"
```

#### Declarative Vertical scaling
- This is semi-automated:
	- A particular provider property can be changed by any consumer in the MachineSet spec e.g `instanceType`.
	- Deleting the Machine will trigger the creation of a fresh one with new `instanceType`.
  - Any consumer might choose to automate this process as it sees fit.

#### Declarative horizontal scaling
- Out of scope:
  - We'll reserve the ability to scale horizontally for further iterations if required.
- For the initial implementation the ControlPlane controller will limit the underlying machineSet horizontal scale capabilities:
  - It will ensure the machineSet replicas is always 1 to enforce recreation of any of the adopted Machines.
  - If the machineSet replicas were to be modified out of band, the ControlPlane controller will set it back to 1 while enforcing a "newest" delete policy on the machineSet.


#### Autoscaling
- Out of scope:
  - This proposal sets the foundation for enabling vertical autoscaling. It enables any consumer to develop autoscaling atop the semi-automated process for "Declarative Vertical scaling" described above.
  - In a future proposal we plan to add vertical autoscaling ability on the ControlPlane resource.

#### Node Autorepair
- Any machine deletion will always honour and it will be blocked on Pod Disruption Budgets (PBD). This gives etcd guard the chance to block a deletion that it might consider to be disruptive for etcd quorum.
- Deletion operations triggered by the managed Machine Health Check will be limited to `maxUnhealthy` value, i.e 1 out of 3. 

#### API Changes

```
controlplane.openshift.machine.io/v1beta1

// ControlPlane is the Schema for the ControlPlane API.
type ControlPlane struct {
  metav1.TypeMeta   `json:",inline"`
  metav1.ObjectMeta `json:"metadata,omitempty"`

  Spec   ControlPlaneSpec   `json:"spec,omitempty"`
  Status ControlPlaneStatus `json:"status,omitempty"`
}

// ControlPlaneSpec defines the desired state of ControlPlaneSpec.
type ControlPlaneSpec struct {

  // Defaults to true.
  // This can be disable for complex scenarios where manual introspection is needed.
  EnableNodeAutorepair bool `json:"enableautorepair,omitempty"`

  // Out of scope.
  // EnableAutoscaling

  // Out of scope. Consider to enable a fully automated rolling upgrade for all control plane machines changing a single place.
  // This would possibly require externalizing the infra info and decoupling it from the failure domain definition e.g az.
  // InfrastructureTemplate
  // FailureDomains []string
}

type ControlPlaneStatus struct {
  // Total number of non-terminated machines targeted by this control plane
  // (their labels match the selector).
  // +optional
  Replicas int32 `json:"replicas,omitempty"`

  // Total number of fully running and ready control plane machines.
  // +optional
  ReadyReplicas int32 `json:"readyReplicas,omitempty"`

  // TODO
  // Conditions
}
```

### Risks and Mitigations

There are multiple sensitive scenarios like growing from 1 to 2. As soon as the etcd API is notified of the new member the cluster loses quorum until that new member starts and joins the cluster. This as well as any other operational aspect of etcd healthiness must be still handled by the [Cluster etcd Operator](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md#motivation).

The contract for the machine API is by honouring Pod Disruption Budgets (PBD), which includes the etcd guard PDB.

## Design Details

### Test Plan

- Exhaustive unit testing.
- Exhaustive integration testing via [envTest](https://book.kubebuilder.io/reference/testing/envtest.html).
- E2e testing on the [machine API](https://github.com/openshift/cluster-api-actuator-pkg/tree/master/pkg) and origin e2e test suite. Given a running cluster:
  - Vertical scaling forcing machine recreation.
    - Loop over machineSets for masters.
    - Change a providerSpecific property which defines the instance capacity.
    - Request all master machines to be deleted.
    - Wait for new machines to come up with new capacity. Wait for existing machines to go away while cluster remains healthy.
  - Simulate unhealthy nodes and remediation.
    - Force 2 master nodes out of 3 to go unhealthy, e.g kill kubelet.
    - Wait for machines get request for deletion by the MHC.
    - Wait for a new nodes to come up healthy.
    - Wait for old machines to go away.
  - Disruptive Machines deletion.
    - Delete all Control Plane Machines.
    - Wait for drain to block deletion.
    - Wait for new Machines to come up.
    - Wait for old Machines to go away.

### Graduation Criteria

This proposal will be released in 4.N as long as:
- All the testing above is in place.
- The cluster etcd operator aknowldege that it manages all the etcd operational aspects including [scaling down](https://issues.redhat.com/browse/ETCD-74?jql=project%20%3D%20ETCD).
- There is considerable internal usage and manual disruptive QE testing feedback.
- This is beta tested pre-release during a considerable period of time either internally or by an early adopter user through the dev preview builds.

### Upgrade / Downgrade Strategy

New IPI clusters deployed after the targeted release will run the ControlPlane resource deployed by the installer out of the box.

For UPI clusters and existing IPI clusters this is opt-in. As a user I can opt-in by creating a ControlPlane resource, i.e `kubectl create ControlPlane`.

### Version Skew Strategy

## Implementation History
- "https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191017-kubeadm-based-control-plane.md"
- "https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md"
- "https://github.com/openshift/machine-config-operator/blob/master/docs/etcd-quorum-guard.md"

## Drawbacks

## Alternatives

1. Let the installer to deploy only machineSets to adopt the Control Plane Machines. This would put too much burden on users to upgrade to self managed Control Plane Machines. Multiple manual error prone steps would need to be documented. Also this would give users a to high degree of flexibilty for horizontall scaling operations which might result confusing and unproductive.

Instead, the higher level ControlPlane resource provides a seamless upgrade path. Also it gates the underlying machineSets horizontal scaling capabilities to match our UX desires.

2. Cluster etcd Operator could develop the capabilities to manage and scale machines. However this would break multiple design boundaries:
The Cluster etcd Operator has the ability to manage etcd members at Pod level without being necessarily tied to the lifecycle of Nodes or Machines.

There are UPI environments where the Machine API is not running the Master instances. Nevertheless we want the Cluster etcd Operator to manage the etcd membership consistently there.

3. Hive could develop the capabilities to manage and scale the Control Plane Machines.

However this would leave multiple scenarios uncovered where the Control Plane Machines would remain "pets" and it won't satisfy multiple of the stories mentioned above. Instead this proposal provides a reasonable abstraction to guarantee a certain level of consistent behaviour that Hive can still leverage as it sees fit.
