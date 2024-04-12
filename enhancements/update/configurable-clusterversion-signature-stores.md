---
title: configurable-clusterversion-signature-stores
authors:
  - "@PratikMahajan"
reviewers:
  - "@LalatenduMohanty"
  - "@wking"
approvers:
  - "@LalatenduMohanty"
api-approvers:
  - "@JoelSpeed"
creation-date: 2023-09-27
last-updated: 2023-11-22
tracking-link:
  - https://issues.redhat.com/browse/OTA-916
---

# ClusterVersion has option to replace default signature stores

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] Operational readiness criteria is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

CVO currently fetches the upstream urls for signature verification through [cluster-update-keys]. 
To increase flexibility and improve disconnected cluster user experience, we need to provide a way for the CVO to 
get the required signatures from custom signature stores or the OSUS running in the disconnected environment.


## Motivation

Today, in the disconnected environments, a user has to clone the signatures config map as well as the release images 
then apply the configMap to each cluster for the update to proceed successfully.
With [serve image signatures for disconnected environments], OSUS will be able to serve the cloned image signatures locally.

Currently, there is no way for the CVO to fetch the signatures through custom URIs. This enhancement defines an API change in the CVO
to accept a custom update-service URI which can be used to verify release signatures. 


### User Stories

* As a cluster administrator, I want the cluster's CVO to add new signature store URIs, so that I can have local release signature 
  verification in air gapped environments.

### Goals

* To verify the release image signature in disconnected environments using OSUS signatures URI.
* To replace existing signature stores with custom user-provided stores.

### Non-Goals

* This enhancement does not cover how OSUS running in the disconnected environment gets the signatures and if the 
signatures are validated during the mirroring process.

## Proposal

### Workflow Description

1. Cluster admin will have to add the verification URI to CVO.
2. CVO will reach the newly added URI to verify the release images.

### API Extensions

* CVO will grow a new field, an array, which will be populated with the image verification URI.
* This will be an array as it'll allow the admin to use multiple locally configured stores. 
* Existing default signature stores will be replaced with custom stores.
* Existing stores can be used along with custom stores by adding them manually.
* Admins will still be able to use the existing config maps for signature verification.
* CVO will check the release signatures in the local ConfigMap first and will reach in parallel to other signature stores.

This enhancement proposes to modify the [api/config/v1/types_cluster_version.go](https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go) as following,

A new field `SignatureStores` will be added to the [`ClusterVersionSpec`](https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go#L40-L96) structure:

```go
type ClusterVersionSpec struct {
	...
	// SignatureStores contains the upstream URIs to verify release signatures.
	// By default, CVO will use existing signature stores if this property is empty.
	// The CVO will check the release signatures in the local ConfigMaps first. It will search for a valid signature
	// in these stores in parallel only when local ConfigMaps did not include a valid signature.
	// Validation will fail if none of the signature stores reply with valid signature before timeout.
	// Setting SignatureStores will replace the default signature stores with custom signature stores.
	// Default stores can be used with custom signature stores by adding them manually.
	//
	// Items in this list should be a valid absolute http/https URI of an upstream signature store as per rfc1738.
	// +kubebuilder:validation:XValidation:rule="self.all(x, isURL(x))",message="signatureStores must contain only valid absolute URLs per the Go net/url standard"
	// +kubebuilder:validation:MaxItems=32
	// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
	// +listType=set
	// +optional
	SignatureStores []string `json:"signatureStores"`
}
```


### Risks and Mitigations

* Users will have to make sure that they've cloned all the signatures needed for image verification or use some of the 
existing stores along with custom stores as missing signatures will cause the verification to fail.
* We recommend admin to use [oc-mirror] clone all releases and release signatures as well building the graph-data 
container image with the signatures

### Drawbacks

N/A.

## Design Details

### Test Plan

* The CVO logic for image verification remains the same, it's just reaching a different signature store than preconfigured ones.
  Thus, explicit e2e tests are not needed for this enhancement.
* We'll be using unit tests to check if CVO's default stores are replaced with custom ones. 
* QE will be testing upgrading the cluster in a disconnected environment with custom signatures served using 
  OpenShift Update Service
* New periodics will be created testing the new feature against the most recent `candidate-4.y` Engineering Candidate releases, because those are the first point where we have CVO-trusted signatures to test with.
  The periodics will:
  1. Configure a custom signature store in ClusterVersion.
  1. Request the cluster update to a pinned older release.
  1. Confirm that the update request is rejected because no signature is found in the custom store.
  1. Add the target's signature to the custom store.
  1. Confirm that the update request is rejected because the version of the requested target is older than the Engineering Candidate being tested.

### Graduation Criteria

The plan is to introduce the first version of the new API behind the `TechPreviewNoUpgrade` feature gate, and later promote to GA.

#### Dev Preview -> Tech Preview

N/A. This is not expected to be released as Dev Preview.

#### Tech Preview -> GA

Once tech-preview periodics discussed in [the Test Plan section](#test-plan) are passing, the feature will be promoted to GA.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

No special consideration.

### Version Skew Strategy

No special consideration.

### Operational Aspects of API Extensions

N/A.

#### Failure Modes

No special consideration.

#### Support Procedures

No special consideration.

## Implementation History


## Alternatives

### Alternatives in proposed new API

#### Fetching custom signatures serially from signature stores

We are proposing fetching signatures parallelly in the proposed API, alternatively we can fetch signatures serially which 
will help us reduce the network load and also helps with not letting all signature stores know what version the cluster is 
trying to update to. 
A drawback of this approach is that if the 1st signature store does not contain the signature or is offline, it'll slow down 
the signature fetching and in extreme cases, we may timeout the overall fetch attempt and fail to find a signature at all.
With the current parallel approach, cluster will be able to fetch the signatures quickly which eliminates the timeout scenario 
with a small network tradeoff.

#### Extending existing stores instead of replacing the existing signature stores

In the current approach, we are replacing default signature stores.  
An alternative to this approach can be to add new signatures to the existing signature stores. 
This will allow admin to use the existing stores in addition to newly added custom stores

This can be achieved by adding the existing signature stores along with custom stores. By replacing the 
existing stores, we are giving more flexibility to the admin to select the signature stores they're 
comfortable with.
If default stores are blocked in the air-gapped environment, CVO error out on this stores constantly polluting the logs.


### Automatically add OSUS signature verification URI to CVO

For a cluster which has a OSUS instance running locally, we can use the OSUS URI to add or replace appropriate signature verification URI
to the cluster stores. The benefit of this alternative is that cluster-admin will not have to manually change the signature verification URI. 
But we might face issues where a cluster admin wants to use this feature to verify release images on multiple disconnected 
clusters that are connected to each other. In that case, the admin will have to have some way to modify the verification URI, which is why we're 
proposing manually adding stores.
With this alternative method, every cluster that needs the release image verification will have to host their own update service. 

[cluster-update-keys]: https://github.com/openshift/cluster-update-keys/blob/master/manifests.rhel/0000_90_cluster-update-keys_configmap.yaml#L4-L5
[serve image signatures for disconnected environments]: https://issues.redhat.com/browse/OTA-946
[oc-mirror]: https://github.com/openshift/oc-mirror
