---
title: konnectivity-socks5-proxy
authors:
  - "@awgreene"
reviewers:
  - "@kevinrizza"
  - "@njhale"
approvers:
  - "@kevinrizza"
creation-date: 2021-09-21
last-updated: 2021-09-21
status: implementable
see-also:
  - "/enhancements/update/ibm-public-cloud-support.md"
replaces:
  -
superseded-by:
  -
---

# Konnectivity Socks5 Proxy

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The goal of this enhancement is to provide [HyperShift](https://github.com/openshift/hypershift/) control plane pods the ability to connect to services hosted on guest clusters.

## Motivation

The HyperShift solution deploys OpenShift clusters by hosting their control plane as pods in a central Management Cluster. This type of setup allows the hosting of many cluster control planes on a single Kubernetes cluster. Each Control Plane has a related Guest Cluster where user workloads run.

The [Operator Lifecycle Manager (OLM)](https://github.com/operator-framework/operator-lifecycle-manager) is a member of the control plane and runs within the Management Cluster. One requirement for OLM is to support Custom CatalogSources defined by users on the Guest Cluster.
To meet this requirement, OLM needs to have the ability to establish [GRPC](https://grpc.io/) connections with CatalogSource services running on the Guest Cluster.

As of today, OLM is unable to connect to CatalogSources defined on the Guest Cluster because:
- The Services defined on the guest clusters cannot be resolved by the DNS Service on the Management Cluster.
- OLM cannot connect to CatalogSources running on the Guest Cluster.

For OLM to support custom Catalog Sources it must be able to establish connections with CatalogSources running on the Guest Cluster.

### Goals

The primary goal of this enhancement is to propose a solution that allows services running on the control plane to:
- Resolve domain names using the DNS service on the Guest Cluster
- Tunnel requests to services running on the Guest Cluster

### Non-Goals

- N/A

## Proposal

This enhancement proposes the introduction of a [Socks5 proxy](https://en.wikipedia.org/wiki/SOCKS) that forwards requests from the control plane to the guest cluster.

The Control Plane already includes a [Konnectivity Server](https://kubernetes.io/docs/tasks/extend-kubernetes/setup-konnectivity/) which can be used to tunnel requests to IP addresses running on the Guest Cluster. Rather than requiring each component to support establishing a connection with the Konnectivity Server, this enhancement proposes the introduction of a Socks5 proxy that:
- Runs as a sidecar on containers
- Establishes a secure connection with the Konnectivity Server
- Resolves Domain Names from requests to Kubernetes services defined on the guest cluster
- Ultimately establishes a connection with the intended recipient on the guest cluster

### User Stories

As the owner of a service running on the Control plane, I would like the ability to route traffic to services running the guest cluster.

### Risks and Mitigations

- The proxy could be hijacked and used to route traffic to guest cluster. Proxy should not be exposed.
- Solution relies on open-source project that we do not maintain.

## Design Details

The Socks5 proxy could be created using [this open-source library](https://github.com/armon/go-socks5), which [supports connecting to the Konnectivity Service](https://github.com/armon/go-socks5/blob/e75332964ef517daa070d7c38a9466a0d687e0a5/socks5.go#L49-L50).
The connection will be established using the existing [konnectivity-client TLS Secret](https://github.com/openshift/hypershift/blob/bfecb01fe8aee63791ad182d8b533755cf94985e/control-plane-operator/controllers/hostedcontrolplane/hostedcontrolplane_controller.go#L1158-L1164).
The proxy will then need to resolve Domain Names by mapping the request to a service present on the guest cluster, which can be accomplshed by [defining a NameResolver](https://github.com/armon/go-socks5/blob/e75332964ef517daa070d7c38a9466a0d687e0a5/socks5.go#L29-L31) that will use a Kubernetes client to retrieve the cluster ip address of the service.
With the resolved domain name, the original request can be fulfilled.

### Test Plan

The introduction of an E2E test that confirms that a control plane service can connect with a service on the guest cluster.

### Graduation Criteria

#### Dev Preview -> Tech Preview

Not applicable, proxy work is simply enough to proceed directly to GA.

#### Tech Preview -> GA

Not applicable, proxy work is simply enough to proceed directly to GA.

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Proxy should be upgraded to new versions as they become available.

### Version Skew Strategy

Not applicable.
## Implementation History

- 09/21/2021: Introduced

## Drawbacks

The component using the proxy must be configured to route traffic to the socks5 proxy. Ideally, a generic solution such as [Istio](https://istio.io/) or [Linkerd](https://linkerd.io/) could handle this for us, but both solutions require their control plane for configuring IP tables.

## Alternatives

- None are apparent. I welcome alternative suggestions.

## Infrastructure Needed

The socks5 proxy binary could live within the [openshift/hypershift repository](https://github.com/openshift/hypershift/).
