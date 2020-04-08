
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

This proposal outlines a solution for declaratively managing as a single entity the compute resources that host the OCP Control Plane components. It introduces scaling and self-healing capabilities for Control Plane compute resources while honouring inviolable Etcd expectations and without disrupting the lifecycle of Control plane components.

## Motivation

The Control Plane is the most critical and sensitive entity of a running cluster. Today OCP Control Plane instances are "pets" and therefore fragile. There are multiple scenarios where adjusting the compute capacity which is backing the Control Plane components might be desirable either for resizing or repairing. 
Currently there is nothing that automates or eases this task. The steps for the Control Plane to be resized in any manner or to recover from a tolerable failure are completely manual. Different teams are following different "Standard Operating Procedure" documents scattered around with manual steps resulting in loss of information, confusion and extra efforts for engineers and users.

### Goals

- To have a declarative mechanism to manage the Control Plane as a single entity.
- To support declarative safe horizontal scaling towards an odd number of replicas for the control plane compute resources.
- To support declarative safe vertical scaling for the control plane compute resources.
- To support declarative safe self-healing and replacement of Control Plane compute resources when the healthy resources are above n/2.
- To support even spread of compute resources across multiple failure domains.

### Non-Goals

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

## Proposal

This proposes an ad-hoc CRD and controller for declaratively managing as a single entity the compute resources that host the OCP Control Plane components.

This controller differs from a regular machineSet in that: 
- It ensures that scaling operations are non disruptive for etcd:
	- It scales one resource at a time.
	- It let scaling operations proceed only when all etcd members are healthy and all the owned machines have a backed ready node.
- It ensure even spread of compute resources across failure domains.
- It removes etcd membership for voluntary machine disruptions (Question: can rather etcd operator somehow handle this?).
- It owns and ensure safe values for a MHC that monitor its machines.

This entity is decoupled and orthogonal to the lifecycle and management of the Control Plane components that it hosts. All of these components are expected to keep self managing themselves as the cluster shrink and expand the Control Plane compute resources.

### User Stories [optional]

#### Story 1
- As an operator running Installer Provider Infrastructure (IPI), I want flexibility to run [large or small clusters](https://kubernetes.io/docs/setup/best-practices/cluster-large/#size-of-master-and-master-components) so I need to have the ability to resize the control plane in a declarative, automated and seamless manner.

#### Story 2
- As an operator running User Provider Infrastructure (UPI), I want to expose my non-machine API machines and offer them to the Control Plane controller so I can have the ability to resize the control plane in a declarative, automated and seamless manner.

#### Story 3
- As an operator of an OCP Dedicated Managed Platform, I want to give users flexibility to add as many workers nodes as they want or to enable autoscaling on woker nodes so I need to have ability to resize the control plane instances in a declarative, automated and seamless manner to react quickly to cluster growth.

#### Story 4
- As a SRE, I want to have consumable API primitives in place to resize Control Plane compute resources so I can develop upper level automation tooling atop. E.g Control Plane autoresizing to support a severe growth peak of the number of worker nodes.

#### Story 5
- As an operator, I want my machine API cluster infrastructure to be self healing so I want faulty nodes to be remediated automatically for safety scenarios.

#### Story 6
- As a multi cluster operator, I want to have a universal user experience for managing the Control Plane in a declarative manner across any cloud provider, bare metal and any flavour of the product that have in common the topology assumed in this doc.

#### Story 7
- As a developer, I want to be able to deploy the smallest possible cluster to save costs, i.e one instance Control Plane.

### Implementation Details/Notes/Constraints [optional]

To satisfy the goals, motivation and stories above this propose a new CRD and controller that manages the Control Plane as a single entity which supports the following features:

#### Declarative horizontal scaling
##### Scale out
1. The controller always reconciles towards expected number of replicas. This must be an odd number.
2. Fetch all existing control plane Machine resources by ownerRef. Adopt any other machine having a targeted label e.g `node-role.kubernetes.io/master`.
3. Compare with expected replicas number. If expected is higher than current then:
4. Check all owned machines have a backed ready node.
5. Check all etcd members for all owned machines are healthy via Cluster etcd Operator status signalling.
6. If (NOT all etcd members are healthy OR NOT all owned machines have a backed ready node) then controller short circuits here, log, update status and requeue. Else:
7. Choose a failure domain.
8. Create new Machine object with a templated spec. Go to 1.
9. Cluster etcd Operator watches the new node. It runs a new etcd pod on it.

##### Scale in
1. The controller always reconciles towards expected number of replicas. This must be an odd number.
2. Fetch all existing control plane Machine resources by ownerRef. Adopt any other machine having a targeted label e.g `node-role.kubernetes.io/master`.
3. Compare with expected replicas number. If expected is lower than current then:
4. Check all owned machines have a backed ready node.
5. Check all etcd members for all owned machines are healthy via Cluster etcd Operator status signalling.
6. If (NOT all etcd members are healthy OR NOT all owned machines have a backed ready node) then controller short circuits here, log, update status and requeue. Else:
7. Pick oldest machine in more populated failure domain.
8. Remove etcd member.
9. Delete machine. Go to 1. (Race between the node going away and Cluster etcd Operator re-adding the member?)

#### Declarative Vertical scaling (scale out + scale in)
1. Watch changes to the Control Plane provideSpec
2. Fetch all existing control plane Machine resources by ownerRef. Adopt any other machine having a targeted label.
3. Fetch all machines with old providerSpec.
4. If any machine has old providerSpec then signal controller as "needs upgrade".
5. If any machine has "replaced" annotation then trigger scale in workflow (starting in 4) and requeue. Else:
6. Pick oldest machine in more populated failure domain.
7. Trigger scale out workflow (starting in 4). Set "replaced" annotation. Requeue.
This is effectively a rolling upgrade with maxUnavailable 0 and maxSurge 1.

#### Self healing (MHC + reconciling)
1. The Controller should always reconcile by removing etcd members for voluntary Machine disruptions, i.e machine deletion.
2. At creation time, unless indicated otherwise `EnableAutorepair=true` will be default. The controller will create a Machine Health Checking resource with an ownerRef which will monitor the Control Plane Machines. This will request unhealthy machines to be deleted. Related https://github.com/openshift/machine-api-operator/pull/543.
3. The controller will ensure the `maxUnhealthy` value in the MHC resource is set a to known integer to prevent farther remediation from happening during scenarios where quorum could be violated. E.g 1 for cluster with 3 members or 2 for a cluster with 5 members.

#### Bootstrapping (scale out)
Currently during a regular IPI bootstrapping process the installer uses Terraform to create a bootstrapping instance and 3 master instances. Then it creates Machine resources to "adopt" the existing master instances. In the past etcd quorum needed to be reached between the three of them before having storage available for the control plane to run self hosted and so for the CVO to run its payload.

The Cluster [etcd Operator](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md) introduced support for a single member etcd cluster to be available quickly on the bootstrapping machine. This lets the CVO to be deployed much faster while new etcd members are added organically as masters come up.

This proposes dropping terraform for creating masters instances in favour of letting the installer to define a Control Plane resource that scales from zero to 3 replicas as soon as the CVO runs the Machine API Operator. Alternatively if this happen to not be doable because of chicken-egg issues, we could keep the current workflow and include an additional step to create the Control Plane resource which would just adopt existing Master machines.

#### API
```
type ControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ControlPlaneSpec   `json:"spec,omitempty"`
	Status ControlPlaneStatus `json:"status,omitempty"`
}

type ControlPlaneSpec struct {
	// Number of desired machines. Defaults to 1.
	// Only odd numbers are permitted.
	Replicas *int32 `json:"replicas,omitempty"`

	// EnableAutorepair defines if a Machine Health Checking resource
	// should be created.
	// This will autorepair faulty machine/nodes in scenarios where quorum would not be violated.
	// e.g 1 unhealthy out of 3.
	// Defaults to true.
	EnableAutorepair bool `json:"enableautorepair,omitempty"`
	
	// ProviderSpec details Provider-specific configuration to use during node creation.
	ProviderSpec ProviderSpec `json:"providerSpec"`
}

// ProviderSpec defines the configuration to use during node creation.
type ProviderSpec struct {

	// Value is an inlined, serialized representation of the resource
	// configuration. It is recommended that providers maintain their own
	// versioned API types that should be serialized/deserialized from this
	// field.
	Value *runtime.RawExtension `json:"value,omitempty"`
}

type ControlPlaneStatus struct {
	// Total number of non-terminated machines targeted by this control plane
	// (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// Total number of non-terminated machines targeted by this control plane
	// that have the desired template spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// Total number of fully running and ready Control Plane Machines.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// Total number of unavailable machines targeted by this control plane.
	// This is the total number of machines that are still required for
	// the deployment to have 100% available capacity. They may either
	// be machines that are running but not yet ready or machines
	// that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`

	// FailureReason indicates that there is a terminal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	// +optional
	FailureReason errors.ControlPlaneStatusError `json:"failureReason,omitempty"`

	// ErrorMessage indicates that there is a terminal problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

type ControlPlaneStatusError string
```

### Risks and Mitigations

During horizontal scaling operations there are sensitive scenarios like scaling from 1 to 2. As soon as the etcd API is notified of the new member the cluster loses quorum until that new member starts and joins the cluster. This must be still handled by the [Cluster etcd Operator](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md#motivation) while the Control Plane controller should honour and short-circuit when it meets the etcd unhealthiness criteria as described in the workflows above.

There are multiple components and operators involved in the lifecycle management of the Control Plane. This in addition to its complex nature makes it difficult to predict the holistic behaviour for failure scenarios. To mitigate risks the Control Plane controller will short-circuit preventing any scaling operation until its healthiness criteria is met again.

## Design Details

### Test Plan

- Exhaustive unit testing.
- Exhaustive integration testing via [envTest](https://book.kubebuilder.io/reference/testing/envtest.html).
- E2e testing on the machine API [e2e test suite](https://github.com/openshift/cluster-api-actuator-pkg/tree/master/pkg):
	- Scale from 3 to 5.
	- Scale from 5 to 3.
	- Vertical scaling Rolling upgrade.
	- Simulate 1 unhealthy node and remediation.
- E2e testing on the origin e2e test suite.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

This controller will go GA in 4.N as long as:
- All the testing above is in place.
- There is considerable internal usage and manual disruptive QE testing feedback.
- This is beta tested pre-release during a considerable period of time either internally or by an early adopter user through the dev preview builds.

Otherwise this would go "Tech Preview" under a feature gate first until enough real use feedback is developed. The risk though is that there might be a very scarce use of feature gated features in the field as to develop the feedback we need.

### Upgrade / Downgrade Strategy

During the cluster upgrade for the targeted release the Machine API Operator (MAO) will let the CVO to instantiate the new CRD `controlPlane` and it will run the backing controller making this functionality opt-in for existing clusters. The user can create an instance of the new CRD if they choose to do so.

New IPI clusters deployed after the targeted release will run the `controlPlane` instance deployed by the installer out of the box.

### Version Skew Strategy

## Implementation History
- "https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20191017-kubeadm-based-control-plane.md"
- "https://github.com/openshift/enhancements/blob/master/enhancements/etcd/cluster-etcd-operator.md"
- "https://github.com/openshift/machine-config-operator/blob/master/docs/etcd-quorum-guard.md"

## Drawbacks

## Alternatives

1. Use only existing resources for managing the Control Plane:

This wouldn't be enough to satisfy the user stories in this proposal.

Scenario 1: Scale out compute from 3 to 5. 2 etcd member healthy and 1 unhealthy out of 3.

- With only machineSet + MHC:
	- New machines start to come up.
	- As soon as the etcd API is notified of a new member( total 4, healthy 2) the cluster
	loses quorum until that new member starts and joins the cluster.
	- There's no automation mechanism for Machines to spread evenly across failure domains.

- With upper level controller:
	- If (NOT all etcd members are healthy OR NOT all owned machines have a backed ready node) then controller short circuits here, log, update status and requeue. Else:
	- Scale up one machine at a time. Go to 1.

Scenario 2: 4 etcd member healthy and 1 unhealthy out of 5. Scale in compute from 5 to 3.

- With only machineSet + MHC:
	- Machines start to get deleted.
	- etcd peer membership is not removed. 
	- etcd guard blocks on drain before losing quorum. etcd remains degraded.
	- There's no automation mechanism for Machines to spread evenly across failure domains.

- With upper level controller:
	- If (NOT all etcd members are healthy OR NOT all owned machines have a backed ready node) then controller short circuits here, log, update status and requeue. Else:
	- Machines start to get deleted.
	- remove etcd member. Quorum membership remains consistent.

Scenario 3: self-healing

- With only machineSet + MHC:
	- All burden is on a consumer of the APIs to set safe inputs.

- With upper level controller:
	- Self healing is signalled with a bool via API.
	- The Controller own all the details

2. Same approach but different details: The Control Plane controller scaling logic could be built atop machineSets just like a deployment uses replica sets.

The scenarios to be supported in this proposal are very specific. The flexibility that managing machineSets would provide is not needed. 

The Control Plane is critical and we want to favour control over flexibility here, therefore this proposes an ad-hoc controller logic to avoid unnecessary layers of complexity.

3. Cluster etcd Operator could develop the capabilities to manage and scale machines. However this would break multiple design boundaries:
The Cluster etcd Operator has the ability to manage etcd members at Pod level without being necessarily tied to the lifecycle of Nodes or Machines.

There are UPI environments where the Machine API is not running the Master instances. Nevertheless we want the Cluster etcd Operator to manage the etcd membership consistently there.

4. Hive could develop the capabilities to manage and scale the Control Plane Machines.

However this would leave multiple scenarios uncovered where the Control Plane Machines would remain "pets" and it won't satisfy multiple of the stories mentioned above. Instead this proposal provides a reasonable abstraction to guarantee a certain level of consistent behaviour that Hive can still leverage as it sees fit.
