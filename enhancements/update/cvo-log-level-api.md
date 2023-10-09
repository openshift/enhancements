---
title: cvo-log-level-api
authors:
  - "@Davoska"
reviewers:
  - "@wking"
  - "@LalatenduMohanty"
  - "@petr-muller"
approvers:
  - "@LalatenduMohanty"
api-approvers:
  - "@deads2k"
  - "@bparees"
creation-date: 2023-10-09
last-updated: 2023-11-20
tracking-link:
  - https://issues.redhat.com/browse/OTA-1029
---

# CVO Log level API

## Summary

This enhancement describes the API changes needed to provide a simple way of dynamically changing the Cluster Version Operator's log level.

## Motivation

The Cluster Version Operator (CVO) logs a lot of useful information. However, there is currently no way to easily change the log level for the CVO.

It would be useful to provide functionality for the cluster administrators and OpenShift engineers to easily modify the log level to a desired level.

### User Stories

* As an OpenShift administrator, I want to increase the log level of the CVO to more easily troubleshoot any potential issues regarding the CVO.
* As an OpenShift administrator, I want to decrease the log level of the CVO to save up more storage space.
* As an OpenShift engineer, I want to decrease the log level of the CVO for shipped releases, so that the customers don't receive debug logs.
* As an OpenShift engineer, I want to increase the log level of the CVO for the CI runs, so that I can more easily troubleshoot any potential issues that occurred.

### Goals

Add a user-facing API for controlling the run-time verbosity of the [CVO](https://github.com/openshift/cluster-version-operator).

### Non-Goals

Change the default logging verbosity of the Cluster Version Operator in production OCP clusters.

## Proposal

This enhancement proposes to add the field `OperatorLogLevel` to the structure [`ClusterVersionSpec`](https://github.com/openshift/api/blob/c3f7566f6ef636bb7cf9549bf47112844285989e/config/v1/types_cluster_version.go#L40-L96) that is used in the structure [`ClusterVersion`](https://github.com/openshift/api/blob/c3f7566f6ef636bb7cf9549bf47112844285989e/config/v1/types_cluster_version.go#L18-L34) 
that represents the configuration for the CVO.
This new field is named exactly after the existing corresponding field [`OperatorLogLevel`](https://github.com/openshift/api/blob/36ce464529eb357673342c06be5886c5463cfc50/operator/v1/types.go#L64-L71) of type [`LogLevel`](https://github.com/openshift/api/blob/36ce464529eb357673342c06be5886c5463cfc50/operator/v1/types.go#L91-L107) that is used inside the structure [`OperatorSpec`](https://github.com/openshift/api/blob/36ce464529eb357673342c06be5886c5463cfc50/operator/v1/types.go#L49-L89)
by Cluster Operators. 
The type of the newly created `OperatorLogLevel` field of the CVO **cannot** be of the same type as above mentioned [`LogLevel`](https://github.com/openshift/api/blob/36ce464529eb357673342c06be5886c5463cfc50/operator/v1/types.go#L91-L107), which is defined in the `github.com/openshift/api/operator/v1` package.
Importing the `github.com/openshift/api/operator/v1` package from inside the `github.com/openshift/api/config/v1` package causes import cycles.
To ensure consistency across the operators and to not cause import cycles, this type will be redefined in the 
[api/config/v1/types_cluster_version.go](https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go) file.

This enhancement is thus proposing to modify the [api/config/v1/types_cluster_version.go](https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go) file as described by the following text.

A new field `OperatorLogLevel` will be added to the [`ClusterVersionSpec`](https://github.com/openshift/api/blob/c3f7566f6ef636bb7cf9549bf47112844285989e/config/v1/types_cluster_version.go#L40-L96) structure:

```go
type ClusterVersionSpec struct {
	...
	// operatorLogLevel controls the logging level of the cluster version operator.
	//
	// Valid values are: "Normal", "Debug", "Trace", "TraceAll".
	// Defaults to "Normal".
	// +optional
	// +kubebuilder:default=Normal
	OperatorLogLevel CVOLogLevel `json:"operatorLogLevel,omitempty"`
}
```

And a new enum will be defined:

```go
// +kubebuilder:validation:Enum="";Normal;Debug;Trace;TraceAll
type CVOLogLevel string

var (
	// Normal is the default.  Normal, working log information, everything is fine, but helpful notices for auditing or common operations.  In kube, this is probably glog=2.
	CVOLogLevelNormal CVOLogLevel = "Normal"

	// Debug is used when something went wrong.  Even common operations may be logged, and less helpful but more quantity of notices.  In kube, this is probably glog=4.
	CVOLogLevelDebug CVOLogLevel = "Debug"

	// Trace is used when something went really badly and even more verbose logs are needed.  Logging every function call as part of a common operation, to tracing execution of a query.  In kube, this is probably glog=6.
	CVOLogLevelTrace CVOLogLevel = "Trace"

	// TraceAll is used when something is broken at the level of API content/decoding.  It will dump complete body content.  If you turn this on in a production cluster
	// prepare from serious performance issues and massive amounts of logs.  In kube, this is probably glog=8.
	CVOLogLevelTraceAll CVOLogLevel = "TraceAll"
)
```

### Workflow Description

Given a cluster administrator and a working cluster for which the administrator is responsible.

**cluster administrator** is a human user responsible for managing the cluster.

1. The cluster administrator notices an issue in the cluster and chooses to troubleshoot the issue.
2. The cluster administrator after some troubleshooting notices that the logs of the Cluster Version Operator (CVO) might help.
3. The cluster administrator notices that the logs are not detailed enough to troubleshoot the issue, and the administrator raises the log level from the default value `Normal` to the level `Trace` by simply modifying the ClusterVersion object via the web console or by patching the resource by using the CLI.
4. The cluster administrator fixes the issue.
5. The cluster administrator notices that the CVO outputs too many logs for the administrator's liking.
6. The cluster administrator lowers the log level of the CVO to the level `Normal`.
7. The cluster administrator is now a happy cluster administrator.

#### Variation [optional]

N/A

### API Extensions

See the section [Proposal](#proposal) for the proposed API changes.

### Implementation Details/Notes/Constraints [optional]

The CVO needs to change the log level dynamically without redeploying itself. The CVO currently uses the package [`klog`](https://github.com/kubernetes/klog) for logging. This package does not provide a simple way of changing the log level dynamically. However, it is possible, and there already exists a function 
[`SetLogLevel`](https://github.com/openshift/library-go/blame/8477ec72b72554c01afff4df9c6a90c0e492ea87/pkg/operator/loglevel/util.go#L63-L103) in the [`openshift/library-go`](https://github.com/openshift/library-go) repository to remedy this issue.

#### Hypershift [optional]

The implementation of HyperShift needs to account for this change because updating the `github.com/openshift/api` module in HyperShit would currently provide a way for the administrator of a hosted cluster to set the log level of a CVO in a hosted control plane. The hosted cluster administrator could set an undesirable level.

However, this design proposes a desired feature that even HyperShift can potentially utilize, and thus HyperShift implementation should address the above-mentioned issue.

In HyperShift, this enhancement proposes to reconcile a fixed value of the new property as [HyperShift currently does for some other properties of the ClusterVersion object](https://github.com/openshift/hypershift/blob/90aa44d064f6fe476ba4a3f25973768cbdf05eb5/control-plane-operator/hostedclusterconfigoperator/controllers/resources/resources.go#L973-L979) of a hosted cluster. 
And potentially, in the future, the HyperShift may even dynamically utilize this property.

### Risks and Mitigations

No risks are known.

### Drawbacks

No drawbacks are known.

## Design Details

### Open Questions [optional]

No open questions.

### Test Plan

Unit tests will be written to test if setting `operatorLogLevel` sets the respective logging in the CVO.

### Graduation Criteria

This will be released directly to GA. The new field and its data type are equal to that of the [`OperatorLogLevel`](https://github.com/openshift/api/blob/1f9525271dda5b7a3db735ca1713ad7dc1a4a0ac/operator/v1/types.go#L74) field in the [`OperatorSpec`](https://github.com/openshift/api/blob/1f9525271dda5b7a3db735ca1713ad7dc1a4a0ac/operator/v1/types.go#L54) 
structure of the [`operator/v1`](https://github.com/openshift/api/tree/1f9525271dda5b7a3db735ca1713ad7dc1a4a0ac/operator/v1) 
package. This field in the [`operator/v1`](https://github.com/openshift/api/tree/1f9525271dda5b7a3db735ca1713ad7dc1a4a0ac/operator/v1) package has been stable for several years. 
Thus, the addition may be directly released to GA.

#### Dev Preview -> Tech Preview

This will be released directly to GA.

#### Tech Preview -> GA

This will be released directly to GA.

#### Removing a deprecated feature

No existing feature will be deprecated or removed.

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

The relevant components for this enhancement are the CVO and the ClusterVersion CRD. These components move closely together on updates and downgrades. During a version skew, the default log level will be used.

A newer CVO consuming an older ClusterVersion will receive an empty `OperatorLogLevel` field, and the CVO will continue using the default log level. An older CVO consuming newer ClusterVersion will not notice the `OperatorLogLevel` field, and the CVO will continue using the default log level.

### Operational Aspects of API Extensions

This enhancement proposes a minor addition to the already existing ClusterVersion CRD.
This new addition will operationally impact only the CVO.

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

Empty as of this moment.

## Alternatives

* The proposed solution doesn't strictly follow the DRY principle as the `LogLevel` type is copied over to the `github.com/openshift/api/config/v1` package to solve the import cycle. Another solution could be moving the data type `LogLevel` from its package `github.com/openshift/api/operator/v1` to a newly created package or to the `github.com/openshift/api/config/v1` package.
  * `github.com/openshift/api/operator/v1` package would include a modified definition for its type `LogLevel` so that no changes are required in other files.
	```go 
	type LogLevel configv1.LogLevel
	```
  * `github.com/openshift/api/config/v1` package would include the original definition of the `LogLevel`.
  * This solution doesn't create any import cycles and doesn't duplicate any code. However, it moves a stable definition of a type corresponding to its package where it's heavily being used to another less logically corresponding package only due to an importing issue.
* Don't provide a way to dynamically modify the log level of the CVO.
  * This results in abundant logging for some use cases.
* Raise the current log level of the CVO.
  * To not lose any information for any possible scenario, the CVO's log level could be raised.

## Infrastructure Needed [optional]

N/A
