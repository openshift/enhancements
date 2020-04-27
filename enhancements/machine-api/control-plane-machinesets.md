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
  - TBD

creation-date: 2020-04-02
last-updated: yyyy-mm-dd
status: implementable
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

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

For clarity in this doc we set the definition for "Control Plane" as "The collection of stateless and stateful processes which enable a Kubernetes cluster to meet minimum operational requirements". This includes: kube-apiserver, kube-controller-manager, kube-scheduler, kubelet and etcd.

This proposal outlines a solution for presenting the Control Plane compute as a single scalable resource (i.e machineSet) for each failure domain. Particularly:
- Proposes to let the installer create a MachineSet for each master Machine.
- Proposes to let the installer create a Machine Health Check resource to monitor the Control Plane Machines.

**Note:**
- This will enable to create a new CRD to manage the Control Plane as a single scalable resource on top of MachineSets, autoscaling and fully automated rolling upgrades if desired in the near future. This is out of the scope for this particular proposal.

- A separate proposal is created to let the installer create Machine Health Checking resources to monitor the worker Machines. This is out of the scope for this particular proposal.

## Motivation

The Control Plane is the most critical and sensitive entity of a running cluster. Today OCP Control Plane instances are "pets" and therefore fragile. There are multiple scenarios where adjusting the compute capacity which is backing the Control Plane components might be desirable either for resizing or repairing.

Currently there is nothing that automates or eases this task. The steps for the Control Plane to be resized in any manner or to recover from a tolerable failure (etcd quorum is not lost) are completely manual. Different teams are following different "Standard Operating Procedure" documents scattered around with manual steps resulting in loss of information, confusion and extra efforts for engineers and users.

### Goals

- To have a declarative mechanism to scale Control Plane compute resources.
- To have a declarative mechanism to ensure a given number of Control Plane compute resources is available at a time.
- To support default declarative self-healing and replacement of Control Plane compute resources.

### Non-Goals / Future work

- To integrate with any existing etcd topology e.g external clusters. Stacked etcd with dynamic member identities, local storage and the Cluster etcd Operator are an assumed invariant.
- To managed individual Control Plane components. Self hosted Control Plane components that are self managed by their operators is an assumed invariant:
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
  - This proposal is a necessary first step for Control Plane autoscaling. It focuses on settling on the primitives to abstract away the Control Plane as scalable resources. In a follow up RFE we will discuss how/when to auto scale the MachineSet resources as a single entity based on relevant cluster metrics e.g number of workers, number of etcd objects, etc.
- To support fully automated rolling upgrades for the Control Plane compute resources. Same reason as above.

## Proposal

Currently the installer chooses the failure domains out of a particular provider availability and it creates a Control Plane Machine resource for each of them.

This proposes two additional steps:
- To let the installer create MachineSets for each master Machine to be adopted.
- To let the installer create a Machine Health Check resource to monitor the Control Plane Machines.

The lifecycle of the compute resources still remains decoupled and orthogonal to the lifecycle and management of the Control Plane components hosted by the compute resources. All of these components, including etcd are expected to keep self managing themselves as the cluster shrink and expand the Control Plane compute resources.

### User Stories [optional]

#### Story 1
- As an operator running Installer Provider Infrastructure (IPI), I want flexibility to run [large or small clusters](https://kubernetes.io/docs/setup/best-practices/cluster-large/#size-of-master-and-master-components) so I need to have the ability to resize the control plane in a declarative, automated and seamless manner.

#### Story 2
- As an operator running User Provider Infrastructure (UPI), I want to expose my non-machine API machines and offer them to the Control Plane controller so I can have the ability to resize the control plane in a declarative, automated and seamless manner.

#### Story 3
- As an operator of an OCP Dedicated Managed Platform, I want to give users flexibility to add as many workers nodes as they want or to enable autoscaling on worker nodes so I need to have ability to resize the control plane instances in a declarative and seamless manner to react quickly to cluster growth.

#### Story 4
- As a SRE, I want to have consumable API primitives in place to resize Control Plane compute resources so I can develop upper level automation tooling atop. E.g Control Plane autoresizing to support a severe growth peak of the number of worker nodes.

#### Story 5
- As an operator, I want my machine API cluster infrastructure to be self healing so I want faulty nodes to be remediated automatically for safety scenarios. This includes having self healing Control Plane machines.

#### Story 6
- As a multi cluster operator, I want to have a universal user experience for managing the Control Plane in a declarative manner across any cloud provider, bare metal and any flavour of the product that have in common the topology assumed in this doc.

#### Story 7
- As a developer, I want to be able to deploy the smallest possible cluster to save costs, i.e one instance Control Plane and resize to more instances as I see fit.

### Implementation Details/Notes/Constraints [optional]

To satisfy the goals, motivation and stories above, this proposes to let the installer to create MachineSets to own the Machine objects for each failure domain and a Machine Health Check to monitor the Control Plane machines.

#### Bootstrapping
Currently during a regular IPI bootstrapping process the installer uses Terraform to create a bootstrapping instance and 3 master instances. Then it creates Machine resources to "adopt" the existing master instances.

Additionally:
- It will create MachineSets to adopt those machines by looking up known labels (Adopting behaviour already exists in machineSet logic).
	- `machine.openshift.io/cluster-api-machineset": <cluster_name>-<label>-<zone>-controlplane`
	- `machine.openshift.io/cluster-api-cluster":    clusterID`

#### Declarative horizontal scaling
- Each MachineSet can be scaled in/out by changing the replica number or via "scale" subresource.
- Out of band the Cluster etcd Operator is responsible for watching the existing nodes and taking care of the etcd operational aspects.
- During scaling in operations any machine deletion will always honour Pod Disruption Budgets (PBD). This gives etcd guard the chance to block a deletion that might be considered disruptive.

#### Declarative Vertical scaling
- With MachineSets in place this is semi-automated:
	- A particular provider property can be changed by any consumer in the MachineSet spec.
	- This won't affect existing Machines but it would apply for upcoming Machines.
	- Machine creation with the new spec property can be forced by scaling out or deleting existing machines, e.g `kubectl delete machine name`

#### Self healing (Machine Health Checking)
- Specific MHC details can be found [here](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/machine-health-checking.md)

- The Installer will create a Machine Health Checking resource targeting the Control Plane Machines.

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

-   Any machine deletion will always honour Pod Disruption Budgets (PBD). This gives etcd guard the chance to block a deletion that it considers disruptive.

TODO: Currently multiple MHC can target the same Machine. We should revisit a stronger mechanisim to prevent undesired MHC to target Control Plane Machines.

### Risks and Mitigations

During horizontal scaling operations there are multiple sensitive scenarios like scaling from 1 to 2. As soon as the etcd API is notified of the new member the cluster loses quorum until that new member starts and joins the cluster. This as well as any other operational aspect of etcd healthiness must be still handled by the [Cluster etcd Operator](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md#motivation).
The contract for the machine API is by honouring Pod Disruption Budgets (PBD), including those given by etcd guard.

## Design Details

### Test Plan

- Exhaustive unit testing.
- Exhaustive integration testing via [envTest](https://book.kubebuilder.io/reference/testing/envtest.html).
- E2e testing on the [machine API](https://github.com/openshift/cluster-api-actuator-pkg/tree/master/pkg) and origin e2e test suite. Given a running cluster:
  - Scale out.
    - Loop over machineSets for masters and increase replicas.
    - Wait for new nodes to go ready.
	- Scale in.
    - Loop over machineSets for masters and decrease replicas.
    - Wait for new nodes to go away. See exisisting one to remain ready.
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

### Graduation Criteria

All the resources leveraged in this proposal i.e MachineSet and Machine Health Check are already GA.
This proposal will be released in 4.N as long as:
- All the testing above is in place.
- The cluster etcd operator manages all the etcd operational aspects including scaling down.
- There is considerable internal usage and manual disruptive QE testing feedback.
- This is beta tested pre-release during a considerable period of time either internally or by an early adopter user through the dev preview builds.

### Upgrade / Downgrade Strategy

New IPI clusters deployed after the targeted release will run the MachineSets and MHC deployed by the installer out of the box.

For UPI clusters and existing IPI clusters this is opt-in. We should pursue cluster homogenous topology. To that end we should provide the exact steps and encourage users in any user facing doc to create the MachineSets and MHC for the Control Plane Machines.


### Version Skew Strategy

## Implementation History
- "https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191017-kubeadm-based-control-plane.md"
- "https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md"
- "https://github.com/openshift/machine-config-operator/blob/master/docs/etcd-quorum-guard.md"

## Drawbacks

## Alternatives

1. Higher level CRD and controller: The Control Plane scaling capabilities could be managed by a single scalable resource built atop MachineSets just like a deployment uses replica sets. This is explored here https://github.com/openshift/enhancements/pull/278. Instead this proposal focuses on the first step and primitives that will enable any farther abstraction.

2. Cluster etcd Operator could develop the capabilities to manage and scale machines. However this would break multiple design boundaries:
The Cluster etcd Operator has the ability to manage etcd members at Pod level without being necessarily tied to the lifecycle of Nodes or Machines.

There are UPI environments where the Machine API is not running the Master instances. Nevertheless we want the Cluster etcd Operator to manage the etcd membership consistently there.

3. Hive could develop the capabilities to manage and scale the Control Plane Machines.

However this would leave multiple scenarios uncovered where the Control Plane Machines would remain "pets" and it won't satisfy multiple of the stories mentioned above. Instead this proposal provides a reasonable abstraction to guarantee a certain level of consistent behaviour that Hive can still leverage as it sees fit.
