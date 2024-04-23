---
title: configurable-network-diagnostics-pod-placement
authors:
  - "@kyrtapz"
reviewers:
  - "@trozet"
  - "@jcaamano"
approvers:
  - "@trozet"
  - "@jcaamano"
api-approvers:
  - "@openshift/api-approvers"
creation-date: 2024-03-04
last-updated: 2024-03-04
tracking-link:
  - https://issues.redhat.com/browse/SDN-4433
see-also:
  - https://docs.openshift.com/container-platform/4.15/networking/verifying-connectivity-endpoint.html
---

# Configurable network diagnostics pod placement

## Summary

The network diagnostics feature performs connectivity health checks to services, endpoints, and 
load balancers. As part of that, the Cluster Network Operator (CNO) creates the 
`network-check-source` Deployment and the `network-check-target` DaemonSet.  
This enhancement allows cluster administrators to configure the pod placement for both 
`network-check-source` and `network-check-target`.

## Motivation

The network connectivity check controller is a component of CNO that manages 
`PodNetworkConnectivityCheck` objects in the `openshift-network-diagnostics` namespace.  
The `network-check-source` pod consumes the `PodNetworkConnectivityCheck` objects matching its 
name in `spec.sourcePod` and performs periodic connectivity tests to the target specified in 
the `spec.targetEndpoint` field. The `network-check-source` pod then updates the objects with 
the test results. To ensure connectivity health checks between nodes, the network connectivity 
check controller creates `PodNetworkConnectivityCheck` objects between the 
`network-check-source` pod and every `network-check-target` pod. Remaining connectivity checks 
include the following targets:
 - Kubernetes API server service 
 - Kubernetes API server endpoints 
 - OpenShift API server service 
 - OpenShift API server endpoints 
 - Internal and external API load balancers  

The network diagnostics feature provides informational data and does not impact cluster 
functionality.

In OpenShift 4.15 and prior, CNO creates the `network-check-source` Deployment and the 
`network-check-target` DaemonSet without providing the ability to affect the pod placement. The 
`network-check-source` Deployment has one replica and is scheduled on one of the linux worker 
nodes. The `network-check-target` DaemonSet creates a pod on every linux node as it tolerates 
all taints. Some cluster administrators require the ability to control the network diagnostics 
pod placement. For example, due to node-to-node connectivity restrictions, the network-check-source 
pod has to be scheduled only on a specific subset of nodes.

An additional requirement is to handle a scenario where no nodes are available to deploy the 
`network-check-source` pod. Instead of the CNO remaining in a `Progressing` state waiting for the 
deployment to be scheduled, it should detect and handle the use case without affecting the 
overall operator state.

### User Stories

* As an OpenShift cluster administrator, I want to ensure that the `network-check-source` pod is 
  only scheduled on an infrastructure node.
* As an OpenShift cluster administrator, I want to control the set of nodes 
  `network-check-target` and `network-check-source` pods are scheduled on.
* As an OpenShift cluster administrator, I want to scale the worker nodes down to zero without 
  permanently affecting the network operator conditions.
  
### Goals

* Enable OpenShift cluster administrators to control the pod placement of the 
  `network-check-source` Deployment and the `network-check-target` DaemonSet.
* Ensure that CNO handles a scenario where there are no available nodes for the 
  `network-check-source` Deployment without permanently affecting the operator conditions.

### Non-Goals

* Modifying the scope of the connectivity checks performed by the network diagnostics feature.
* Modifying other properties of the `network-check-source` Deployment and the 
  `network-check-target` DaemonSet.

## Proposal

This enhancement adds new API fields to the `network.config.openshift.io` Custom Resource 
Definition (CRD), that allow administrators to control the pod placement of the 
`network-check-source` Deployment and the `network-check-target` DaemonSet. During the 
reconciliation process for the mentioned resources, CNO will configure the relevant 
`tolerations` and `nodeSelector` fields.  
Additionally, introduce the `NetworkDiagnosticsAvailable` status condition that's going to reflect
whether network diagnotics are currently available. This is useful in a scenario where there are 
no nodes that can host the `network-check-source` Deployment. CNO will set the condition 
instead of putting the operator into a constant `Progressing` state. Considering that the 
network diagnostics feature is only informational it should not affect the `Available` and 
`Degraded` conditions.

### Workflow Description

The OpenShift cluster administrator can configure the pod placement of the 
`network-check-source` Deployment and the `network-check-target` DaemonSet by changing the `.
spec.networkDiagnostics.sourcePlacement` and `.spec.networkDiagnostics.targetPlacement` fields 
in the `cluster/network.config.openshift.io` object.  

1. The OpenShift cluster administrator modifies the `network.config.openshift.io` custom 
   resource, for example:
   ```
   apiVersion: config.openshift.io/v1
   kind: Network
   spec:
     ...
     networkDiagnostics:
       mode: All
       sourcePlacement:
         nodeSelector:
           kubernetes.io/os: linux
           node-role.kubernetes.io/infra: ""
       targetPlacement:
         nodeSelector:
           kubernetes.io/os: linux
         tolerations:
           - operator: Exists
      ```
1. CNO validates the new node placement requirements for the `network-check-source` Deployment.
   If there are no nodes matching the new requirements, CNO sets the `NetworkDiagnosticsAvailable` 
   status condition to false.
   There is no need to verify the node placement for `network-check-target` Daemonset.
   Even if it runs on no nodes, this might be intentional, and there are still platform 
   endpoints that will be verified.
1. The connectivity check controller detects the changes made to `network-check-source` and 
   `network-check-target` pods
   and reflects them in the `PodNetworkConnectivityCheck` objects in the 
   `openshift-network-diagnostics` namespace.

### API Extensions

The `network.operator.openshift.io` CRD already contains the `disableNetworkDiagnostics` field that
allows the administrator to disable the network diagnostics. With this enhancement the newly 
introduced `networkDiagnostics` takes precedence over `disableNetworkDiagnostics`. If
`networkDiagnostics` is not specified or is empty, and the `disableNetworkDiagnostics` flag in 
`network.operator.openshift.io`  is set to true, the network diagnostics feature will be disabled.

Once the new API lands in the default featureset, `disableNetworkDiagnostics` is going 
to be deprecated in favor of `networkDiagnostics`.

This enhancement adds the following fields to the spec of the `network.config.openshift.io` CRD:
```
type NetworkSpec struct {
...
	// networkDiagnostics defines network diagnostics configuration.
	//
	// Takes precedence over spec.disableNetworkDiagnostics in network.operator.openshift.io.
	// If networkDiagnostics is not specified or is empty,
	// and the spec.disableNetworkDiagnostics flag in network.operator.openshift.io is set to true,
	// the network diagnostics feature will be disabled.
	//
	// +optional
	// +openshift:enable:FeatureGate=NetworkDiagnosticsConfig
	NetworkDiagnostics NetworkDiagnostics `json:"networkDiagnostics"`
}

// NetworkDiagnosticsMode is an enumeration of the available network diagnostics modes
// Valid values are "", "All", "Disabled".
// +kubebuilder:validation:Enum:="";All;Disabled
type NetworkDiagnosticsMode string

const (
	// NetworkDiagnosticsNoOpinion means that the user has no opinion and the platform is left
	// to choose reasonable default. The current default is All and is a subject to change over time.
	NetworkDiagnosticsNoOpinion NetworkDiagnosticsMode = ""
	// NetworkDiagnosticsAll means that all network diagnostics checks are enabled
	NetworkDiagnosticsAll NetworkDiagnosticsMode = "All"
	// NetworkDiagnosticsDisabled means that network diagnostics is disabled
	NetworkDiagnosticsDisabled NetworkDiagnosticsMode = "Disabled"
)

// NetworkDiagnostics defines network diagnostics configuration
type NetworkDiagnostics struct {
	// mode controls the network diagnostics mode
	//
	// When omitted, this means the user has no opinion and the platform is left
	// to choose reasonable defaults. These defaults are subject to change over time.
	// The current default is All.
	//
	// +optional
	Mode NetworkDiagnosticsMode `json:"mode"`

	// sourcePlacement controls the scheduling of network diagnostics source deployment
	//
	// See NetworkDiagnosticsSourcePlacement for more details about default values.
	//
	// +optional
	SourcePlacement NetworkDiagnosticsSourcePlacement `json:"sourcePlacement"`

	// targetPlacement controls the scheduling of network diagnostics target daemonset
	//
	// See NetworkDiagnosticsTargetPlacement for more details about default values.
	//
	// +optional
	TargetPlacement NetworkDiagnosticsTargetPlacement `json:"targetPlacement"`
}

// NetworkDiagnosticsSourcePlacement defines node scheduling configuration network diagnostics source components
type NetworkDiagnosticsSourcePlacement struct {
	// nodeSelector is the node selector applied to network diagnostics components
	//
	// When omitted, this means the user has no opinion and the platform is left
	// to choose reasonable defaults. These defaults are subject to change over time.
	// The current default is `kubernetes.io/os: linux`.
	//
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`

	// tolerations is a list of tolerations applied to network diagnostics components
	//
	// When omitted, this means the user has no opinion and the platform is left
	// to choose reasonable defaults. These defaults are subject to change over time.
	// The current default is an empty list.
	//
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations"`
}

// NetworkDiagnosticsTargetPlacement defines node scheduling configuration network diagnostics target components
type NetworkDiagnosticsTargetPlacement struct {
	// nodeSelector is the node selector applied to network diagnostics components
	//
	// When omitted, this means the user has no opinion and the platform is left
	// to choose reasonable defaults. These defaults are subject to change over time.
	// The current default is `kubernetes.io/os: linux`.
	//
	// +optional
	NodeSelector map[string]string `json:"nodeSelector"`

	// tolerations is a list of tolerations applied to network diagnostics components
	//
	// When omitted, this means the user has no opinion and the platform is left
	// to choose reasonable defaults. These defaults are subject to change over time.
	// The current default is `- operator: "Exists"` which means that all taints are tolerated.
	//
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations"`
}
```
The fields will initially be added as tech preview and will move to default once they meet the 
graduation criteria.

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift, the network diagnostics components do not run in the controlplane so there are no 
special considerations.

#### Standalone Clusters

No special considerations.

#### Single-node Deployments or MicroShift

MicroShift doesn't run CNO and network diagnostics.  
No special considerations for single-node deployments.

### Implementation Details/Notes/Constraints

This enhancement requires changes to the `network.config.openshift.io` CRD and the CNO 
operator. CNO will apply the network diagnostics settings on every reconciliation. This means 
that they can be changed at any point during the cluster lifetime. 

### Risks and Mitigations

If there is no `network-check-source` pods running the network diagnostic feature is effectively 
disabled. In OpenShift 4.15 and prior, this would lead to CNO setting the `Progressing` condition to
`True` with a message stating that it is waiting for the deployment to succeed. This meant that CNO 
would stay in this state forever, which is unexpected and misleading to the user.   
This enhancement removes this behavior and to keep the discoverability, the CNO sets the 
`NetworkDiagnosticsAvailable` status condition instead.

### Drawbacks

Enabling the cluster administrator to affect the pod placement of the network diagnostic feature 
means that it can limit the connectivity health check data being gathered throughout the 
lifetime of the cluster.

## Open Questions [optional]

## Test Plan

- Introduce E2E tests that verify that changing the node selectors and tolerations for the 
  network diagnostics components are reflected in the deployed pods and PodNetworkConnectivityCheck 
  objects.
- Introduce E2E tests that ensure CNO handles a scenario where there are no available nodes for 
  the `network-check-source` Deployment without permanently affecting the operator conditions.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- A track record of passing E2Es in CI
- QE testing completed

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

The newly added API field is optional and should not affect upgrades. 

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

If the `PodNetworkConnectivityCheck` objects in the `openshift-network-diagnostics` namespace 
are not being updated this likely means that there is no `network-check-source` pod running. The 
user should check the `NetworkDiagnosticsAvailable` status condition in 
`cluster/network.config.openshift.io` and verify that there is a pod running in the 
`openshift-network-diagnostics` namespace.

## Alternatives

The only alternative to this enhancement is to disable the network diagnostics feature with the 
existing `.spec.disableNetworkDiagnostics` field in `network.operator.openshift.io`. This was 
not selected as it disables all the connectivity health checks that can be useful to the user.

## Infrastructure Needed [optional]

N/A
