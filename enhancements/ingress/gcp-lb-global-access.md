---
title: gcp-internal-load-balancer-global-access
authors:
  - "@sgreene570"
reviewers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@miciah"
  - "@candita"
  - "@rfredette"
  - "@miheer"
approvers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@miciah"
creation-date: 2021-02-09
last-updated: 2021-02-12
status: implementable
---

# GCP Internal Ingress Load Balancer Global Access Option

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the user to enable
the "Global Access" option for Ingress Controllers exposed via an Internal Load Balancer
on GCP.

## Motivation

On GCP, Ingress Controllers that specify a `LoadBalancerScope` of `InternalLoadBalancer` are only externally accessible
when a client is in the same VPC network and Google Cloud Region as the Load Balancer. GCP exposes the
[Global Access](https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing#global_access) option specifically for
Internal Load Balancers via an annotation on the given LoadBalancer service. When enabled, the Global Access option allows clients from any
region within the cluster's VPC to connect to the Load Balancer.


### Goals

1. Expose the GCP Internal Load Balancer Global Access Option via a new field in the existing IngressController API `ProviderLoadBalancerParameters` type.


### Non-Goals

1. Expose a similar API field for other cloud providers (AWS, Azure, etc.) as GCP is the only provider that provides this configuration option.
2. Change the current default behavior (no Global Access) of Ingress Controllers exposed via Internal Load Balancers on GCP.

## Proposal

The IngressController API is extended by adding a `GCP` field of type `GCPLoadBalancerParameters` to the `ProviderLoadBalancerParameters` type.

```go
// ProviderLoadBalancerParameters holds desired load balancer information
// specific to the underlying infrastructure provider.
// +union
type ProviderLoadBalancerParameters struct {
        // <snip>

        // aws provides configuration settings that are specific to AWS
        // load balancers.
        //
        // If empty, defaults will be applied. See specific aws fields for
        // details about their defaults.
        //
        // +optional
        AWS *AWSLoadBalancerParameters `json:"aws,omitempty"`

        // gcp provides configuration settings that are specific to GCP
        // load balancers.
        //
        // If empty, defaults will be applied. See specific gcp fields for
        // details about their defaults.
        //
        // +optional
        GCP *GCPLoadBalancerParameters `json:"gcp,omitempty"`
}
```

Then, the new `GCPLoadBalancerParameters` type is created with a single field, `ClientAccess`.


```go
// GCPLoadBalancerParameters provides configuration settings that are
// specific to GCP load balancers.
type GCPLoadBalancerParameters struct {
        // clientAccess describes how client access is restricted for internal
        // load balancers.
        //
        // Valid values are:
        // * "Global": Specifying an internal load balancer with Global client access
        //   allows clients from any region within the VPC to communicate with the load
        //   balancer.
        //
        //     https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing#global_access
        //
        // * "Local": Specifying an internal load balancer with Local client access
        //   means only clients within the same region (and VPC) as the GCP load balancer
        //   can communicate with the load balancer. Note that this is the default behavior.
        //
        //     https://cloud.google.com/load-balancing/docs/internal#client_access
        //
        // +optional
        ClientAccess GCPClientAccess `json:"clientAccess"`
}
```

The following `GCPClientAccess` string constants are defined to support the new `ClientAccess` field.


```go

// GCPClientAccess describes how client access is restricted for internal
// load balancers.
// +kubebuilder:validation:Enum=Global;Local
type GCPClientAccess string

const (
        GlobalAccess GCPClientAccess = "Global"
        LocalAccess GCPClientAccess = "Local"
)
```

In addition to IngressController API extensions, changes to the Ingress Operator will need to be made.

On GCP, if an Ingress Controller has the
`Spec.LoadBalancer.ProviderParameters.GCP.ClientAccess` field set to `Global` or `Local` and `Spec.LoadBalancer.Scope` is set to `Internal`, the Ingress Operator
will need to apply the GCP Global Access Annotation (`networking.gke.io/internal-load-balancer-allow-global-access`) to the Ingress Controller's LoadBalancer service.

A value of `Global` for the `ClientAccess` field will require the Global Access annotation to have a value of `true`, while a value of `Local` for the `ClientAccess` field
will require the Global Access annotation to have a value of `false`.

In any case, if an Ingress Controller does not specify the `Spec.LoadBalancer.ProviderParameters.GCP.ClientAccess`
field, then the GCP Global Access annotation _will not_ be applied to the Ingress Controller's LoadBalancer service.
Note that not specifying the GCP Global Access LoadBalancer Service annotation leads to the same behavior as setting the annotation to `false`.

### User Stories

#### As an OpenShift cluster administrator using an Ingress Controller with an Internal Load Balancer on GCP, I wish to enable the "Global Access" option so that clients in a different region within the load balancer's VPC can reach cluster workloads

This option will be trivially configurable via the Ingress Controller API for any Ingress Controller.

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

Enabling the GCP Internal Load Balancer Global Access option may incur more hosting costs from the provider if cross-region Ingress
is used heavily.

Additionally, cluster administrators need to ensure that enabling the Global Access option does not over-expose an
Ingress Controller's front end within the given VPC.

## Design Details

### Open Questions

How often do cluster administrators want an internal load balancer on GCP that can communicate with clients in a different region?

### Test Plan

The Ingress Operator has existing unit tests to verify that the operator renders the correct service definition given an Ingress Controller's `Spec.LoadBalancer` field.
These unit test will be extended to verify that Ingress Operator applies the Global Access LoadBalancer service annotations for Ingress Controllers with
`Spec.LoadBalancer.ProviderParameters.GCP.ClientAccess` and `Spec.LoadBalancer.Scope` properly set.

The Ingress Operator also has existing e2e tests. These e2e tests will be extended to verify that Ingress Controllers created with the Global Access option enabled
successfully create a LoadBalancer service and function as intended. Note that any e2e test created for this enhancement would only run on GCP clusters. With that in mind,
additional CI jobs may need to be added to the Ingress Operator to accommodate this need.


An example of a proper e2e test is as follows:

1. Create an Ingress Controller with an Internal LoadBalancer and the `ClientAccess` field set to `Global`.
2. Ensure that the LoadBalancer service is created.
3. Ensure that the Ingress Controller works normally.
4. Modify the Ingress Controller's spec so that the `ClientAccess` field is set to `Local`.
5. Ensure that the LoadBalancer service is properly updated with the correct annotations.

Note that testing the actual behavior of the Global Access option from within GCP by using a client in a different region is most likely
not feasible given CI limitations, and most likely unnecessary. In other words, it is most likely safe to assume that GCP Internal Load Balancers
set with the proper Global Access annotations work as described by the [GCP documentation](https://cloud.google.com/kubernetes-engine/docs/how-to/internal-load-balancing#global_access).

### Graduation Criteria

N/A

### Upgrade / Downgrade Strategy

On upgrade, internal LoadBalancer services on GCP will not be updated with the Global Access annotation unless
an Ingress Controller's spec is updated to require so.

On downgrade, the Ingress Operator will ignore the new `GCPLoadBalancerParameters` field. However, in OCP 4.7, the Ingress Operator
does not remove annotations that it did not explicitly set on an Ingress Controller's LoadBalancer service. Downgrading from 4.8
(once this feature is implemented) to 4.7, a `ClientAccess` field of `Global` would effectively remain intact for an Ingress Controller.
This is not detrimental, and if this behavior is undesired, a cluster administrator could simply manually remove the GCP Global Access annotation
from a LoadBalancer service on downgrade.

### Version Skew Strategy

N/A

## Implementation History

None.

## Alternatives

A cluster administrator can already create a custom service that is not managed by the Cluster Ingress Operator. The administrator can then configure
these kinds of settings on the custom, administrator-managed LoadBalancer service.

## Infrastructure Needed

As mentioned, a new CI job for the Ingress Operator that exercises the Ingress Operators e2e tests on GCP may be needed to support this enhancement.
