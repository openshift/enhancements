---
title: install-ca-management
authors:
  - "@bparees"
reviewers:
  - "@sdodson"
  - "@abhinavdahiya"
  - "@adambkaplan"
  - "@dmage"
approvers:
  - "@sdodson"
  - "@crawford"
creation-date: 2020-08-03
last-updated: 2020-08-03
status: provisional
---

# Install CA Management


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Customers have a need to provide additional CAs to their cluster.  Ideally they can do this at 
install time so that everything works by the time the cluster comes up (if they do not, the 
cluster may install but things like imagestreams may not import, builds may not work, or other 
components may not function as expected).

Today the installer collects an “additionalTrustBundle”, and optionally proxy host names.

The additionalTrustBundle is put(not sure where exactly?) and consumed directly by some set of 
components such as the Machine Config Operator.  This ensures that the nodes can talk to the 
mirror registry in a disconnected install for example.

In addition if a proxy host is supplied, the bundle is put into a proxy configuration (CA+hosts) 
which many/most components consume.

However when only the bundle is supplied (because no proxy is in use) the bundle consumption is 
essentially ad hoc. This has the negative consequence that components which don’t directly consume 
it (from where?) may not function, particularly in a disconnected install where they need a CA to 
trust the mirror registry.  We see this problem w/ imagestream import which expects to get its CAs 
from an image config resource which references a configmap of bundles that are explicitly 
associated to registry hostnames.  Further complicating the situation is that the field is simply 
named `additionalTrustBundle` so the user intent is not clear, making it difficult for specific 
components to know whether they should be consuming this value or not.

The most direct consequences of this are that today if a customer does a disconnected install, 
their imagestream imports from the mirror registry will fail.  In particular this means the 
oauth-proxy imagestream, which is consumed by many OLM operators, will fail and thus those 
operators will fail to function, until/unless the admin explicitly configures the CA in the image 
config resource and then re-imports the imagestream.

While this could theoretically be solved by the imagestream component trusting the additional CA (
for all hosts?) explicitly, it would make more sense if we used the information the admin is 
providing us at install time to correctly configure the cluster as if they had done it themselves, 
rather than introducing additional sources of (potentially conflicting) configuration 
information.

## Motivation

See summary above, motivation is to make the install experience clearer with less potential for misconfiguration and conflicting settings.

### Goals

- Post install the cluster can not only pull images from an alternate registry, but imagestreams can import from it, builds can pull from it, and the internal registry can perform pullthrough against it
- Post install the registry CAs provided by the admin are populated into the configuration api that is intended for this purpose

### Non-Goals

- Refactoring the additionalTrustBundle field into many unique fields for different CA usages
- Addressing the current proxy CA behavior in which the additionalTrustBundle is sometimes populated into a proxy configuration and other times not (depending on whether a proxy host is supplied)
- Adding generic cluster configuration fields to the installer that are not required to bootstrap the cluster and ensure consistent use of cluster config apis


## Proposal

The installer should populate the 
[image config resource](https://github.com/openshift/api/blob/master/config/v1/types_image.go) in the same way the 
[product documentation](https://docs.openshift.com/container-platform/4.5/openshift_images/image-configuration.html#images-configuration-cas_image-configuration) 
recommends.  The installer 
can use the value of the additionalTrustBundle field to populate this api.  Similar to how the 
value is used for the proxy config when a proxy host is supplied, the value can be used to 
populate the image config resource when a mirror registry is in use.

In addition, the downstream components that potentially communicate with the mirror registry should add logic which confirms they can successfully access the mirror registry.  Specifically, for any registry defined in the image content source policy, the components should be able to access the registry using the supplied CAs.  If they cannot, they should mark themselves degraded.

Finally, if possible, the MCO mechanism that injects CAs into the nodes directly should be disabled when the image config api is populated, to ensure we do not have conflicting values being placed on the nodes.  The MCO mechanism should be a pure bootstrapping mechanism.  (That may already be the case today, I am not certain how that mechanism is implemented).

### User Stories

#### Story 1

As an administrator installing a new cluster, I do not want to have to take additional actions on day 2 of my cluster installation or risk forgetting to do so only to have surprising failures later when the cluster is in use.  

#### Story 2

As an administrator of a cluster I do not want nodes on my cluster being configured with a CA that is different from what I configured the cluster to trust via the product documented apis, so that I am not confused by my cluster's behavior relative to what the configuration appears to be.

### Implementation Details/Notes/Constraints

The main constraint is that nodes need this CA information before the [node-ca-daemon](https://github.com/openshift/cluster-image-registry-operator/blob/5eda706684c9ec69ae8be5745f0daf740fe947fe/bindata/nodecadaemon.yaml#L1) can be started (nodes need the CA to pull images, the node-ca-daemon is an image), so while the node-ca-daemon is the first class mechanism for getting CAs from the image config api to the nodes, it cannot be relied upon for purposes of bootstrapping the cluster.  Therefore the existing mechanism must continue to function at least through bootstrapping the cluster.  After that point, the CAs should come from the image config api.


### Risks and Mitigations

The main risk is that we get the bootstrapping/configuring logic/ordering incorrect and end up with clusters(and in particular nodes) that do not have the right CAs to trust the mirror registry.

Otherwise this EP largely amounts to having the installer automate steps that we already tell the cluster admin do today, so there is not substantial risk.

## Design Details

### Open Questions

1. What is the current install->mco->node CA flow and does it continue reconciling values post install?  Does it compete with the node-ca-daemon?

2. Where does the installer get the mirror registry hostname from so it knows whether or not to populate the image config resource?  (Presumably it has this information since it populates the image content source policy)


### Test Plan

Perform a disconnected install with a mirror registry that uses a custom CA.  Ensure that post install:
- the image config resource + associated CA configmap are properly configured
- nodes can pull images from the mirror
- imagestreams can import images from the mirror
- builds can pull images from the mirror
- the internal registry can perform image pullthrough against the mirror

Perform a standard (non-disconnected) install but supply an additionalTrustBundle to the installer.  Ensure that post install:
- the image config resource was not populated by the installer


### Graduation Criteria

This should move directly to GA state once implemented.  

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Drawbacks

Makes the installer responsible for additional cluster configuration logic.
The additionalTrustBundle field is ambiguous, this design further overloads it, increasing the ambiguity about exactly what the values provided there will be used for and when.


## Alternatives

Introduce new fields in the install config where CAs can be explicitly provided for separate purposes and deprecate the existing additionalTrustBundle field.  So far we have identified two types of CAs that are potentially needed:

1. CAs for the proxy (including MITM proxies)
2. CAs for the mirror registry

So this would not seem to be overly burdensome.
