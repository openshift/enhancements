---
title: must-gather-operator
authors:
  - "@swghosh"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@Prashanth684"
creation-date: 2025-08-14
last-updated: 2025-08-14
status: implementable
see-also:
  - "/enhancements/oc/must-gather.md"
tracking-link:
  - https://issues.redhat.com/browse/MG-5
  - https://issues.redhat.com/browse/OCPSTRAT-2259
---


# Must Gather Operator

## Summary

The Must Gather Operator is an OLM-installable operator deployed on an OpenShift cluster that closely integrates with the must-gather tool. This enhancement describes how the operator provides a user-configurable interface (a new CustomResource) to gather data, diagnostic information from a cluster using must-gather and upload it to a Red Hat support case.

## Motivation

The cli utility, oc adm must-gather can collect data from the cluster and dump the results into the local filesystem where the oc binary is running. Today, there is no means for users of OpenShift to trigger collection of a must-gather job that runs in the cluster asynchronously. Moreover, the oc cli requires cluster-admin privileges and is not feasible each time for cluster administrators to run the cli tool which hogs their local system and can often be a very long running time-consuming process. Also, there is no provision for a developer on the OpenShift cluster with a lesser permissions to capture a must-gather and forward it to Red Hat for any support.

### User Stories

- As an OpenShift administrator, I want to trigger collection of a must-gathers on the cluster
- As a non-privileged OpenShift user, I want to be able to collect a must-gather and automatically upload it to a Red Hat support case

### Goals

1. A day-2 operator that installs on top of the core OpenShift platform
2. Allow a CR on the cluster to trigger collection of a platform must-gather (pods, nodes, openshift objects, etc.)
3. Enable automated upload of must-gather results to Red Hat support cases
4. Provide role-based access control allowing users can trigger must-gather collection by creating a MustGather CR
5. Maintain compatibility with existing must-gather toolchain and image formats
6. Report status of the must-gather collection into the MustGather CR

### Non-Goals

1. Ability to collect information when a cluster installation had failed (day-0)
2. Collect a must-gather dump in the event of apisever being completely off
3. Different products or operators should be responsible for gathering for their own components from the operator (see https://github.com/advisories/GHSA-77c2-c35q-254w)
4. Reduce or skim the information collected by the must-gather script itself

## Proposal

The Must Gather Operator provides a Kubernetes-native way to collect diagnostic information from OpenShift clusters through a declarative API. The operator will introduces a new CustomResource `MustGather` allowing users to trigger collection (batch) jobs within the cluster, eliminating the need for cluster administrators to run the CLI must-gather tool everytime.

The operator is designed as a day-2 operator installable through OLM (Operator Lifecycle Manager) and operates within the cluster to:

1. Users create `MustGather` CustomResources which trigger collection jobs that run as pods within the cluster, allowing the collection process to continue without requiring the user persist a tedious long running local process.
2. The operator provides configurable RBAC that allows non-cluster-admin users to trigger must-gather collection for their permitted namespaces or cluster-scoped resources, depending on their roles.
4. The operator integrates with Red Hat Case Management APIs to automatically upload collected data to support cases, streamlining the support workflow for customers via SFTP.
5. The `MustGather` CR provides detailed status information about collection progress, completion, and any errors encountered during the process in the `.status` part.

The operator leverages the existing must-gather image format and `/usr/bin/gather` script convention, ensuring compatibility with the current ecosystem while providing a declarative interface for triggering data collections.

### Workflow Description

1. **Installation**: 
   - Cluster administrator installs the Must Gather Operator via OLM from OperatorHub

2. **Must-Gather Collection**:
   - User (with appropriate permissions) creates a `MustGather` CustomResource
   - User provides a reference to the service account to be used for the collection Job
   - User provides a reference to the secret to be used to authenticate to sftp.access.redhat.com

   - Operator creates a Kubernetes Job that has 2 containers: gather, upload
   - The gather pod runs the specific platform must-gather image
   - The upload container waits for the gather process to finish (via pgrep)
   - The upload container once ready, tars the gathered directory and uploads to Red Hat's SFTP server
   - Status is updated and propogated to the `MustGather.status` subresource

```mermaid
// TODO: diagram
```

### API Extensions

The operator introduces a new API group `mustgather.operator.openshift.io` with the following CustomResource:

#### MustGather Custom Resource

```go
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// 
// MustGather is the Schema for the mustgathers API
type MustGather struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MustGatherSpec   `json:"spec,omitempty"`
	Status MustGatherStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MustGatherList contains a list of MustGather
type MustGatherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MustGather `json:"items"`
}

// MustGatherSpec defines the desired state of MustGather
type MustGatherSpec struct {
	// The is of the case this must gather will be uploaded to
	// +kubebuilder:validation:Required
	CaseID string `json:"caseID"`

	// the secret container a username and password field to be used to authenticate with red hat case management systems
	// +kubebuilder:validation:Required
	CaseManagementAccountSecretRef corev1.LocalObjectReference `json:"caseManagementAccountSecretRef"`

	// the service account to use to run the must gather job pod, defaults to default
	// +kubebuilder:validation:Optional
	/* +kubebuilder:default:="{Name:default}" */
	ServiceAccountRef corev1.LocalObjectReference `json:"serviceAccountRef,omitempty"`

	// A flag to specify if audit logs must be collected
	// See documentation for further information.
	// +kubebuilder:default:=false
	Audit bool `json:"audit,omitempty"`

	// This represents the proxy configuration to be used. If left empty it will default to the cluster-level proxy configuration.
	// +kubebuilder:validation:Optional
	ProxyConfig ProxySpec `json:"proxyConfig,omitempty"`

	// A time limit for gather command to complete a floating point number with a suffix:
	// "s" for seconds, "m" for minutes, "h" for hours, or "d" for days.
	// Will default to no time limit.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Format=duration
	MustGatherTimeout metav1.Duration `json:"mustGatherTimeout,omitempty"`

	// A flag to specify if the upload user provided in the caseManagementAccountSecret is a RH internal user.
	// See documentation for further information.
	// +kubebuilder:default:=true
	InternalUser bool `json:"internalUser,omitempty"`
}

// +k8s:openapi-gen=true
type ProxySpec struct {
	// httpProxy is the URL of the proxy for HTTP requests.  Empty means unset and will not result in an env var.
	// +optional
	HTTPProxy string `json:"httpProxy,omitempty"`

	// httpsProxy is the URL of the proxy for HTTPS requests.  Empty means unset and will not result in an env var.
	// +optional
	HTTPSProxy string `json:"httpsProxy,omitempty"`

	// noProxy is the list of domains for which the proxy should not be used.  Empty means unset and will not result in an env var.
	// +optional
	NoProxy string `json:"noProxy,omitempty"`
}

// MustGatherStatus defines the observed state of MustGather
type MustGatherStatus struct {
	Status     string             `json:"status,omitempty"`
	LastUpdate metav1.Time        `json:"lastUpdate,omitempty"`
	Reason     string             `json:"reason,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
	Completed  bool               `json:"completed"`
}

```

### Implementation Details/Notes/Constraints

1. **Pod Creation**: The operator creates a collection Job which creates a pod with:
   - Local hostPath emptyDir PVC for output storage
   - ServiceAccount with necessary RBAC permissions

2. **Data Flow**: 
   - Each gather container writes to `/must-gather/<image-name>/`
   - The upload container creates a compressed tar archive before uploading via SFTP
   - PVC is removed once the pod gets recycled


### TODO: Add more granular details about status reconciliation flow.

#### Security Considerations

- **Privilege Escalation/RBAC**: 
- **Credential Management**: 
- **Data Validation**:

### Topology Considerations

None, as a day-2 operator dedicated OpenShift and Hosted Clusters are both treated equally although the amount of data collection by must-gather itself may vary.

## Implementation History

1. https://github.com/openshift/must-gather-operator/ was previously maintained by OSD SREs
2. Outdated old fork https://github.com/redhat-cop/must-gather-operator that still installs on OpenShift from the Community Catalog

## Alternatives

None

## Infrastructure Needed

While all the APIs provided by Red Hat for Case Management are available at access.redhat.com would be used by end-users and customers of OpenShift, for CI and local testing, use of access.stage.redhat.com: staging APIs requires Red Hat VPN connectivity on the cluster. Additionally, a new CI chain for triggering collection of must-gather from a CI cluster via the operator would be desirably helpful. 
