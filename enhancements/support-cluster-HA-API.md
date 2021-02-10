---
title: support-cluster-HA-API
authors:
  - "@varshaprasad96"
reviewers:
  - "@jmrodri"
  - "@ecordell"
  - "@estroz"
approvers:
  - "@jmrodri"
  - "@ecordell"
  - "@estroz"
creation-date: 2021-02-01
status: implementable
---

# Support for Cluster High-availability Mode API

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA

## Summary

The goal of this enhancement is to describe the support for [cluster high-availability mode API][enhancement_cluster_operators] introduced by OpenShift. The enhancement proposes to introduce necessary helper functions which will enable users to identify if the OpenShift cluster can support high availability deployment mode of their operator or not.

## Motivation

OpenShift intends to provide an API to expose the high-availability expectations to operators in a cluster. The API will accomplish this by embedding additional fields in the [informational resource][informational_resource]. Providing the necessary helper functions which utilize the OpenShift API to expose the intended cluster topology, will enable users to identify the mode of operator deployment which the cluster can support. Operator authors can leverage this functionality and configure their operators to react accordingly.

### Goals:

- List the helper functions that read the OpenShift informational resource and return appropriate values to expose the high availability mode that the cluster can support.
- Discuss on the appropriate location of the helper functions.

### Non-Goals:

- Discuss how OpenShift determines if the cluster can support HA/non-HA mode.
- Discuss the behavior of operators in HA/non-HA mode.

#### Story 1

As an Operator author, I would like to know if the cluster can support HA or non-HA modes, so that I can configure my operator's behavior accordingly.

## Design Details

As an operator author it is helpful to know the cluster topology so that one can modify the behavior of operands. The library where this functionality resides, will provide users with a `GetClusterConfig` function that returns a `ClusterConfig` struct.

The `ClusterConfig` struct can be defined to be:

```go
import (
    ...
    apiv1 "github.com/openshift/api/config/v1"
    ...
)

// ClusterConfig contains the configuration information of the
// cluster.
type ClusterConfig struct {
    // ControlPlaneTopology contains the expectations for operands that 
    // run on control plane nodes.
    ControlPlaneTopology apiv1.TopologyMode

    // InfrastructureTopology contains the expectations for infrastructre
    // services.
    InfrastructureTopology apiv1.TopologyMode
}
```

The `ClusterConfig` contains the high availability expectations for control plane and infrastructure nodes. The `apiv1.TopologyMode` is defined by OpenShift API and refers to the [topology mode][api_topology_mode_pr] in which the cluster is running. It can have the following two values:

1. `ControlPlaneTopology` - refers to the expectations for operands that normally run on [control plane][control_plane] (or master) nodes.
2. `InfrastructureTopology` - refers to the expectations for infrastructure services that do not run on control plane nodes, usually indicated by a node selector for a `role` value other than `master`.

The possible values for both can either be `HighlyAvailable` or `SingleReplica`. More details on when the cluster is considered to support highly available or single replica deployments can be found [here][enhancement_cluster_operators].

**Note**
In future, the `ClusterConfig` can be extended to store additional information on cluster topology. 

The helper function will return the `ClusterConfig` mentioned above by reading the `infrastructure.config.openshift.io` crd present in cluster. The user will be expected to provide the location of the `kubeconfig` file, with which an existing  cluster can be accessed using [Go client][upstream_go_client].

```go
// ErrAPINotAvailable is the error returned when the infrastructure CRD
// is not found in cluster. This means that this could be a non-openshift cluster.
var ErrAPINotAvailable = fmt.Errorf("infrastructure crd cannot be found in cluster")

// GetClusterConfig will return the HA expectations of the cluster
func GetClusterConfig(kubeconfigPath string) (ClusterConfig, error) {
    // Implementation will involve reading the required fields
    // in status subresource of `infrastructure.config.openshift.io`
    // crd. If the crd is not present this will return an 
    // `ErrAPINotAvailable` error.
}
```

**NOTE:**
Operators cannot modify the informational resources present in OpenShift clusters. The function can only be used to retrieve information about the high availability expectations of the cluster and not to modify it.

### Scenarios:

1. Operator author tries to fetch the `ClusterConfig` for a non-openshift cluster:

In this case, an `ErrAPINotAvailable` error is returned. The error can be wrapped, such that users can utilize the `errors.Is` method to identify this specific case.

2. Operator author tries to use this function with older versions of OCP clusters which do not have HA expectations embedded in CRD:

In this case, the error returned will be in the same format as stated in previous scenario but would specifically mention that the requested information resource cannot be found in CRD.

### Location of the helper functions:

- Library-go:

[OpenShift/library-go][library_go] contains helpers which deal with OpenShift-specific operator logic. Though the helpers defined in the proposal can be used on non-openshift clusters, they will always return a `ErrAPINotAvailable` error. Since this is intended to support operator developers using OpenShift, library-go seems to be the suitable location.

### Alternatives:

It was earlier discussed on having these helper functions in [`operator-framework/operator-lib`][operator_lib]. In future they can be used to make decisions on the scaffolding of operator project based on cluster topology. However, it would not be useful for consumers of upstream [Operator SDK][upstream_sdk] who largely use vanilla Kubernetes clusters.

As this functionality is targeted for OpenShift users, it can be integrated with downstream SDK ([ocp-release-operator-sdk][downstream_sdk]) in future. It can also be used in a `openshift-specific plugin` implementation, which Operator SDK intends to introduce in future to support openshift specific features.

[informational_resource]: https://docs.openshift.com/container-platform/4.6/installing/install_config/customizations.html#informational-resources_customizations
[control_plane]: https://docs.openshift.com/container-platform/4.1/architecture/control-plane.html#defining-masters_control-plane
[enhancement_cluster_operators]: https://github.com/openshift/enhancements/blob/master/enhancements/cluster-high-availability-mode-api.md#cluster-operators
[operator_lib]: https://github.com/operator-framework/operator-lib
[openshift-api]: https://github.com/openshift/api
[api_topology_mode_pr]: https://github.com/openshift/api/pull/827
[upstream_go_client]: https://kubernetes.io/docs/tasks/administer-cluster/access-cluster-api/#go-client
[downstream_sdk]: https://github.com/openshift/ocp-release-operator-sdk
[upstream_sdk]: https://github.com/operator-framework/operator-sdk
[library_go]: https://github.com/openshift/library-go