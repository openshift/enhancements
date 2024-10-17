---
title: cvo-log-level-api
authors:
  - "@DavidHurta"
reviewers:
  - "@wking, for CVO aspects, ideally please look at the whole document"
  - "@LalatenduMohanty, for CVO aspects, ideally please look at the whole document"
  - "@petr-muller, for CVO aspects, ideally please look at the whole document"
  - "@csrwng, for HyperShift aspects, please look at the HyperShift section"
  - "@enxebre, for HyperShift aspects, please look at the HyperShift section"  
approvers:
  - "@wking"
api-approvers:
  - "@deads2k"
  - "@JoelSpeed"
creation-date: 2023-10-09
last-updated: 2024-10-01
tracking-link:
  - https://issues.redhat.com/browse/OTA-923
see-also:
  - "None"
replaces:
  - "None"
superseded-by:
  - "None"
---

# CVO Log Level API

## Summary

This enhancement proposes to create a new OpenShift API to provide a simple
method of dynamically changing the verbosity level of Cluster Version Operator's
logs. There will be four log levels available. The lowest level being the
current default log level being used by the Cluster Version Operator.

## Motivation

There is currently no way to easily change the verbosity of the [Cluster Version Operator (CVO)](https://github.com/openshift/cluster-version-operator)
logs in a live cluster.

It would be useful to provide functionality for the cluster administrators and
OpenShift engineers to easily modify the log level to a desired level using an API
[similarly as can be done for some other operators](https://github.com/openshift/api/blob/5852b58f4b1071fe85d9c49dff2667a9b2a74841/operator/v1/types.go#L67-L74).

### User Stories

* As an OpenShift administrator, I want to increase the log level of the CVO 
  from the default level to more easily troubleshoot any potential issues regarding
  the cluster or the CVO.
* As an OpenShift administrator, I want to decrease the log level of the CVO
  from a previously set higher log level to save storage space.
* As an OpenShift engineer, I want to increase the log level of the CVO for
  a CI run so that I can more easily troubleshoot any potential issues.

### Goals

* Add a user-facing API for controlling the verbosity of the CVO logs.

### Non-Goals

* Change the default logging verbosity of the Cluster Version Operator.
* Allow users to set a lower than the current default logging level.

## Proposal

This enhancement proposes to create a new `CustomResourceDefinition` (CRD) 
called `clusterversionoperators.operator.openshift.io`. The new type will be part 
of the [`github.com/openshift/api/operator/v1alpha1`][github.com/openshift/api/operator/v1alpha1]
package. A `ClusterVersionOperator` resource will be used to configure the 
CVO. The configuration, as of now, will only contain the knob to modify the CVO 
log level. A `ClusterVersionOperator` resource named `cluster` will be added to
the OCP payload. This resource will act as a singleton configuration resource
for the CVO. The CVO will dynamically change the verbosity of its logs based on a
value provided in the new resource. The CRD is described in more detail in the
**Implementation Details** section. A new CRD is created to better differentiate 
between the cluster version and the CVO configuration.

Four log levels will be available. The lowest level being the
current default log level being used by the Cluster Version Operator. The exact 
log levels are defined as per the existing enum [`LogLevel`][LogLevelType] that 
is used in the new `ClusterVersionOperator` CRD. See the **Implementation 
Details** section for more details.

[LogLevelType]: https://github.com/openshift/api/blob/f89ab92f1597eaed4de5b947c1781adde2bf42fb/operator/v1/types.go#L94-L110

### Workflow Description

Given a cluster administrator and a working cluster for which the administrator is responsible.

**Cluster administrator** is a human user responsible for managing the cluster.

1. The cluster administrator notices an issue in the cluster and chooses to
   troubleshoot the issue.
2. The cluster administrator, after some troubleshooting, notices that the logs
   of the Cluster Version Operator (CVO) might help.
3. The cluster administrator notices that the logs are not detailed enough to
   troubleshoot the issue.
4. The cluster administrator raises the log level from the default value to a
   more verbose level by simply modifying the new `ClusterVersionOperator`
   resource named `cluster` via the web console or by patching the resource by
   using the CLI.
5. The cluster administrator fixes the issue in the cluster.
6. The cluster administrator notices that the CVO outputs too many logs for the
   administrator's liking.
7. The cluster administrator lowers the log level of the CVO to the lowest 
   level, the default level.
8. The cluster administrator is now a happy cluster administrator.

### API Extensions

The enhancement proposes to create a new `ClusterVersionOperator` CRD. A new
`ClusterVersionOperator` resource will only impact the CVO logging level.

### Topology Considerations

#### Hypershift / Hosted Control Planes

A hosted CVO is located in the management cluster and accesses the hosted API
server. As the new CR will be part of the OCP payload, it will be applied to the
hosted cluster.

It is needed to address the fact that the hosted cluster API server will now 
provide access to the new CR that represents the CVO configuration. A 
hosted cluster administrator could set the log level of the hosted CVO in the
management cluster, thus affecting the storage space of the management cluster.
The `ClusterVersionOperator` `cluster` resource in the hosted cluster will be 
protected by a validating admission policy to ensure a hosted cluster
administrator cannot modify it and thus affect the management cluster ([as is 
currently being done for certain resources in HyperShift][kas/admissionpolicies]).

The `ClusterVersionOperator` `cluster` resource will be modifiable from the 
management cluster. To achieve that, the [`HostedCluster`][HostedClusterAPI] 
API will be extended, and its values for the `ClusterVersionOperator` 
configuration will be propagated to the hosted cluster.

The existing [`ClusterConfiguration`][ClusterConfigurationAPI] API could be 
considered to reference the `ClusterVersionOperator` API; however, its 
purpose is oriented around the configuration API (`github.com/openshift/api/config/v1`), 
meaning configuration for OCP components rather than just operators.
As such, a new API is proposed to extend the [`HostedCluster`][HostedClusterAPI]
API. A new `OperatorConfiguration` API where various APIs from the 
`github.com/openshift/api/operator/v1` package can be referenced, including 
the newly proposed `ClusterVersionOperator` API.

Starting in the alpha group, the following changes are proposed for the
[`api/hypershift/v1alpha1/hostedcluster_types.go`][api/hypershift/v1alpha1/hostedcluster_types.go] 
file.

[HostedClusterAPI]: https://hypershift-docs.netlify.app/reference/api/#hypershift.openshift.io/v1beta1.HostedCluster
[ClusterConfigurationAPI]: https://github.com/openshift/hypershift/blob/a0191dbda4ac75bd8ee19869d9a952aa508b3f2b/api/hypershift/v1beta1/hostedcluster_types.go#L2868
[api/hypershift/v1alpha1/hostedcluster_types.go]: https://github.com/openshift/hypershift/blob/main/api/hypershift/v1alpha1/hostedcluster_types.go
[kas/admissionpolicies]: https://github.com/OpenShift/hypershift/blob/36f2c22c033ecf80eb051aeace50e5bc3baa8001/control-plane-operator/hostedclusterconfigoperator/controllers/resources/kas/admissionpolicies.go#L55

```go
// OperatorConfiguration specifies configuration for individual OCP operators in the
// cluster, represented as embedded resources that correspond to the openshift
// operator API.
type OperatorConfiguration struct {
	// ClusterVersionOperator specifies the configuration for the Cluster Version Operator in the hosted cluster.
	//
	// +optional
	ClusterVersionOperator *v1alpha1.ClusterVersionOperatorSpec `json:"clusterVersionOperator,omitempty"`
}
```

```go
type HostedClusterSpec struct {
        ...
	// OperatorConfiguration specifies configuration for individual OCP operators in the
	// cluster, represented as embedded resources that correspond to the openshift
	// operator API.
	//
	// +optional
	OperatorConfiguration *OperatorConfiguration `json:"operatorConfiguration,omitempty"`
```

#### Standalone Clusters

In standalone clusters, cluster administrators will be able to specify the desired
CVO log level. No additional changes are needed.

#### Single-node Deployments or MicroShift

Same as standalone clusters.

### Implementation Details/Notes/Constraints

This enhancement proposes to create a new `operator/v1alpha1/types_clusterversion.go` 
file in the [OpenShift API repository](https://github.com/openshift/api). 
Meaning the new data types will be part of the [`github.com/openshift/api/operator/v1alpha1`][github.com/openshift/api/operator/v1alpha1]
package. A new feature-gated cluster scoped alpha configuration API resource 
will be defined to configure the CVO.

The types are inspired by the existing [`OperatorSpec`][OperatorSpec]
and [`OperatorStatus`][OperatorStatus]
structures and copy over some of the fields. Only the  
`OperatorLogLevel` field is introduced to the `spec` field as there is currently
no need to support the rest of the [`OperatorSpec`][OperatorSpec]
fields or any new fields. A similar case applies to the 
[`OperatorStatus`][OperatorStatus] fields. The `ObservedGeneration` field is 
used to give feedback regarding the last generation change the CVO has dealt
with. There is currently no need to support additional fields.

The `ClusterVersionOperator` CRD will be behind a new 
`ClusterVersionOperatorConfiguration` FeatureGate that will be used to 
control the development of the CVO configuration.

A `ClusterVersionOperator` resource named `cluster` will be added to
the OCP payload. This resource will act as a singleton configuration resource
for the CVO. Thus, validation using the CEL will be introduced to ensure only 
the `ClusterVersionOperator` resource named `cluster` exists. The 
[`release.openshift.io/create-only`][create-only] annotation will be used to 
ensure that the CVO will only create the resource as part of reconciling the 
payload and will not overwrite any user changes.

[github.com/openshift/api/operator/v1alpha1]: https://github.com/openshift/api/tree/master/operator/v1alpha1
[OperatorSpec]: https://github.com/openshift/api/blob/f89ab92f1597eaed4de5b947c1781adde2bf42fb/operator/v1/types.go#L54
[OperatorStatus]: https://github.com/openshift/api/blob/f89ab92f1597eaed4de5b947c1781adde2bf42fb/operator/v1/types.go#L112
[create-only]: https://github.com/openshift/cluster-version-operator/blob/e546515213c8681ca44c52f178401cd47ad07d11/lib/resourceapply/interface.go#L9-L13

The proposed contents of the new `operator/v1alpha1/types_clusterversion.go` file:

```go
package v1alpha1

import (
	operatorv1 "github.com/openshift/api/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterVersionOperator holds cluster-wide information about the Cluster Version Operator.
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +openshift:file-pattern=cvoRunLevel=0000_00,operatorName=cluster-version-operator,operatorOrdering=01
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterversionoperators,scope=Cluster
// +openshift:api-approved.openshift.io=https://github.com/openshift/api/pull/2044
// +openshift:enable:FeatureGate=ClusterVersionOperatorConfiguration
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message="ClusterVersionOperator is a singleton; the .metadata.name field must be 'cluster'"
type ClusterVersionOperator struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata"`

	// spec is the specification of the desired behavior of the Cluster Version Operator.
	// +kubebuilder:validation:Required
	Spec ClusterVersionOperatorSpec `json:"spec"`

	// status is the most recently observed status of the Cluster Version Operator.
	// +optional
	Status ClusterVersionOperatorStatus `json:"status"`
}

// ClusterVersionOperatorSpec is the specification of the desired behavior of the Cluster Version Operator.
type ClusterVersionOperatorSpec struct {
	// operatorLogLevel is an intent based logging for the operator itself.  It does not give fine grained control, but it is a
	// simple way to manage coarse grained logging choices that operators have to interpret for themselves.
	//
	// Valid values are: "Normal", "Debug", "Trace", "TraceAll".
	// Defaults to "Normal".
	// +optional
	// +kubebuilder:default=Normal
	OperatorLogLevel operatorv1.LogLevel `json:"operatorLogLevel,omitempty"`
}

// ClusterVersionOperatorStatus defines the observed status of the Cluster Version Operator.
type ClusterVersionOperatorStatus struct {
	// observedGeneration is the last generation change you've dealt with
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterVersionOperatorList is a collection of ClusterVersionOperators.
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
type ClusterVersionOperatorList struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata"`

	// Items is a list of ClusterVersionOperators.
	Items []ClusterVersionOperator `json:"items"`
}
```

### Risks and Mitigations

No risks are known.

No important logs will be lost due to this enhancement. The lowest settable 
log level, the `Normal` level, will represent the current default CVO log level.

### Drawbacks

No drawbacks are known.

## Open Questions [optional]

No open questions.

## Test Plan

* Unit tests
* E2E test(s) to ensure that the CVO correctly reconciles the new resource

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- Relative API stability
- Sufficient test coverage
- Gather feedback

### Tech Preview -> GA

- Sufficient time for feedback
- All tests are implemented
- Available by default
- API is stable
- User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

No existing feature will be deprecated or removed.

## Upgrade / Downgrade Strategy

Not applicable.

## Version Skew Strategy

The relevant component for this enhancement is the CVO. During a version skew, 
the default log level will be used.

## Operational Aspects of API Extensions

This enhancement proposes to create a new `ClusterVersionOperator` CRD.
A new CR will operationally impact only the CVO. It may increase or
decrease its logs. Impacting the storage.

The enhancement also proposes to extend the `HostedCluster` API and 
introduce a new admission policy in a hosted cluster. The HyperShift will 
simply propagate the values to the hosted cluster; operational aspects seem 
to be minimal. The new validating admission policy will enforce who can 
modify the hosted `ClusterVersionOperator`. Such a configuration is not 
expected to be modified often, and the admission itself will not be extremely 
complex; thus, the operational aspects seem to be minimal as well.

## Support Procedures

Not applicable.

## Alternatives

### The CVO state will be represented by the ClusterVersion resource

The CVO will be configurable by the existing `ClusterVersion` resource. The 
`spec` field of the `ClusterVersion` resource will grow CVO configuration.   

This alternative was not chosen due to it breaking the existing API consistency
to an extent. `ClusterVersion` will continue to configure the cluster 
version, and the new `ClusterVersionOperator` resource will configure the CVO.

### The ClusterVersionOperator CRD will be applied to the management cluster

As the hosted CVO is running in the management cluster, its configuration 
resource will also be applied to the management cluster. The CRD will be 
namespace scoped; one `ClusterVersionOperator` resource per one hosted 
control plane namespace. The HyperShift will not have to process the 
`ClusterVersionOperator` resource in the hosted cluster as there will be none. 
This will result in less logic for the HyperShift to overwrite any user 
changes, and the hosted CVO configuration will be maintained by the same API 
server that maintains the hosted CVO.

This alternative was not chosen, as it increases the overall complexity of the 
solution. The HyperShift would have to support multiple potentially
different versions of the CRD, as the HyperShift is able to handle multiple
versions of OCP clusters on the same management cluster. The hosted CVO would 
have to learn to process multiple API servers simultaneously and would be given 
additional network and API server access in the management cluster.

An alternative is to simply propagate the desired CVO configuration from 
the `HostedCluster` API to the hosted `ClusterVersionOperator` resource. 
This utilizes an existing pattern and makes the whole solution simpler and 
thus less prone to errors and easier to maintain.

### Do not introduce an option to dynamically modify the verbosity level of logs

The option for cluster administrators to choose the desired verbosity level will
not be introduced. This alternative was not chosen as the potential benefit of
the proposed change greatly outweighs the implementation cost.

## Infrastructure Needed [optional]

No new infrastructure is needed.