---
title: cache-tuning
authors:
  - "@brandisher"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@jerpeter1"
  - "@Miciah"
  - "@frobware"
approvers:
  - "@Miciah"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@soltysh"
  - "@JoelSpeed"
creation-date: 2022-06-30
last-updated: 2022-06-30
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/NE-757
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Cache Tuning

## Summary

The purpose of this enhancement is to provide an API in the cluster-dns-operator which will allow cluster 
admins to configure durations for successful (positive) and unsuccessful (negative) caching of DNS query responses. 
The scope of the configured durations are for all configured server blocks in CoreDNS' Corefile including the 
default zone.

## Motivation

Customers running their own DNS resolvers et al want the option to configure OpenShift's DNS caching so that the 
load on upstream resolvers can be reduced.

### User Stories

- As a cluster admin, I want the ability to configure a maximum time-to-live duration for successful DNS queries so 
  that I can reduce the load on my DNS infrastructure.
- As a cluster admin, I want the ability to configure a maximum time-to-live duration for unsuccessful DNS queries so 
  that I can reduce the load on my DNS infrastructure.

### Goals

- Cluster admins should see a reduction in load for upstream resolvers.
- Provide the ability to tune the positive/negative caching done by CoreDNS. In the end, have full caching based on 
  TTLs provided by upstream DNS servers. This means not artificially reducing the TTL in Core DNS.
- Core DNS does not override TTL (respect TTLs as provided by other DNS servers)

### Non-Goals

- This enhancement does not apply to the infrastructure DNS and is only intended to enable caching for the CoreDNS 
  managed by the cluster-dns-operator.
- This enhancement is not intended to enable configuration of additional caching parameters like `prefetch`, 
  `serve_stale`, or configuring of cache capacity.

## Proposal

This enhancement proposal will add a new type of `DNSCache` which will contain two `metav1.Duration` fields; 
`positiveTTL` and `negativeTTL`. `DNSSpec` will get a new field of `cache` which holds this configuration.


### Workflow Description

1. `oc edit dns.operator.openshift.io/default`
2. Modify the caching values
    ```yaml
    spec:
      cache:
        successTTL: 1h
        denialTTL: 0.5h10m
    ```
3. Save changes.

#### Variation [optional]

N/A

### API Extensions

```go
// DNSCache defines the fields for configuring DNS caching.
type DNSCache struct {
	// positiveTTL is optional and specifies the amount of time that a positive response should be cached.
	//
	// If configured, it must be a value of 1s (1 second) or greater up to a theoretical maximum of several years.
	// If not configured, the value will be 0 (zero) and OpenShift will use a default value of 900 seconds unless noted
	// otherwise in the respective Corefile for your version of OpenShift. The default value of 900 seconds is subject
	// to change. This field expects an unsigned duration string of decimal numbers, each with optional fraction and a
	// unit suffix, e.g. "100s", "1m30s". Valid time units are "s", "m", and "h".
	// +kubebuilder:validation:Pattern=^(0|([0-9]+(\.[0-9]+)?(s|m|h))+)$
	// +kubebuilder:validation:Type:=string
	// +optional
	PositiveTTL metav1.Duration `json:"positiveTTL,omitempty"`

	// negativeTTL is optional and specifies the amount of time that a negative response should be cached.
	//
	// If configured, it must be a value of 1s (1 second) or greater up to a theoretical maximum of several years.
	// If not configured, the value will be 0 (zero) and OpenShift will use a default value of 30 seconds unless noted
	// otherwise in the respective Corefile for your version of OpenShift. The default value of 30 seconds is subject
	// to change. This field expects an unsigned duration string of decimal numbers, each with optional fraction and a
	// unit suffix, e.g. "100s", "1m30s". Valid time units are "s", "m", and "h".
	// +kubebuilder:validation:Pattern=^(0|([0-9]+(\.[0-9]+)?(s|m|h))+)$
	// +kubebuilder:validation:Type:=string
	// +optional
	NegativeTTL metav1.Duration `json:"negativeTTL,omitempty"`
}
```

```go
// DNSSpec is the specification of the desired behavior of the DNS.
type DNSSpec struct {
	// <snip>

	// cache describes the caching configuration that applies to all server blocks listed in the Corefile.
	// This field allows a cluster admin to optionally configure:
	// * positiveTTL which is a duration for which positive responses should be cached.
	// * negativeTTL which is a duration for which negative responses should be cached.
	// If this is not configured, OpenShift will configure positive and negative caching with a default value that is
	// subject to change. At the time of writing, the default positiveTTL is 900 seconds and the default negativeTTL is
	// 30 seconds or as noted in the respective Corefile for your version of OpenShift.
	// +optional
	Cache DNSCache `json:"cache,omitempty"`
}
```

### Implementation Details/Notes/Constraints [optional]

Changes will be needed in the following repositories:
- openshift/api [PR](https://github.com/openshift/api/pull/1214)
- openshift/cluster-dns-operator [PR](https://github.com/openshift/cluster-dns-operator/pull/335)

#### Notes

**If someone decides to add an API to configure minTTL in CoreDNS**

The `cache` plugin will cache both positive and negative responses handled by the `kubernetes` plugin. Coincidentally, 
the current CoreDNS default minimum TTL for the `cache` plugin is `5s` which is the same as the default `ttl` for the 
`kubernetes` plugin. This allows `cluster.local` lookups to be cached for the expected amount of time. If we choose 
to allow changing the `minTTL`  in the `cache` plugin in the future we'll need to assess how this will affect critical
cluster services. Here are some examples of where setting a `minTTL` could affect services/pods.

* If you set a positive `minTTL` to `22s`, successful `cluster.local` queries will be cached for `22s` instead of `5s`.
This would make services/pods appear to be available even though they are not available.
* If you set a negative `minTTL` to `7s`, failed `cluster.local` queries will be caches for `7s` instead of `5s`. This
would make services/pods appear to be unavailable even though they are available.

The net effect is that adjusting `minTTL` for either the positive or negative case could result in traffic being sent to
services/pods that no longer exist or exist but in the negative response cache.

### Risks and Mitigations

* **Risk 1**: If we move away from CoreDNS in the future, the API could be impacted
  * **Mitigation:** Ensure the API is flexible enough to handle reasonable divergence from what CoreDNS offers in the 
`cache` plugin.
* **Risk 2**: Setting TTLs to low values could lead to increased load on upstream resolvers and cause DoS.
  * **Mitigation**: Cluster admins can revert the changes to restore normal function.

### Drawbacks

No notable drawbacks aside from a slight increase in support complexity.

## Design Details

### Open Questions [optional]

Q. Will CoreDNS honor high TTL values such as 604800 (= 1 week)?
A. Yes. CoreDNS uses `time.Duration` for TTLs which enables using extremely long durations. We'll be using `metav1.
Duration` for better user experience and converting the string unit (e.g. `1h`) to the respective number of seconds 
for usage in the Corefile. We do this because the CoreDNS Corefile uses seconds as the unit for TTL values but it would
be cumbersome for a user to need manually calculate the number of seconds for longer or unique timeframes.

### Test Plan

#### Testing TTL settings
1. Deploy an upstream resolver.
2. Modify the cache settings to have a low `successTTL` (e.g. `1m`).
3. Issue a known successful DNS query for the associated zone once to ensure the record is cached.
4. Issue the query a second time to ensure that it's served from the cache.
5. Wait until the TTL period has expired and then issue the query again to ensure that it's not served from the cache.

#### Testing TTL Passthrough
1. Deploy an upstream resolver that can respond with a specific TTL for a zone. Ensure that this TTL is higher than 
   the default for CoreDNS.
2. Issue a known successful query for the associated zone.
3. Wait until the cluster DNS TTL has expired and then issue the query again to ensure that it's survived the 
   cluster DNS cache settings.

### Graduation Criteria

#### Dev Preview -> Tech Preview

This will go directly to GA.

#### Tech Preview -> GA

This will go directly to GA. It will include product documentation changes and e2e tests.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Upgrade expectations:
- When upgrading to versions where the cache API _is not_ available, the current default in the CoreDNS Corefile will 
  apply normally.
- When upgrading to versions where the cache API _is_ available, no interruption will occur as the CoreDNS Corefile 
  cache settings will apply by default even if the API is not configured.
- When upgrading from a version with the cache API to another version with the cache API, the configuration will be 
  retained across upgrades.

Downgrade expectations:
- When downgrading to versions where the cache API _is not_ available, the cluster admin will remove the caching 
  settings when downgrading.
- When downgrading to versions where the cache API _is_ available, the configuration settings should be retained 
  across downgrades.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

Since allowing for adjustment of the CoreDNS cache settings is primarily a judgement call based on the environment 
the cluster is running in, we do not expect a change in operator conditions as a result of configuring caching. 
Modifying cache settings could have an effect on the collected CoreDNS metrics and as a result the alerts associated 
with those metrics, but they would be localized to a particular environment and would need to be triaged accordingly.

#### Failure Modes

* TTLs set too low
  * This could cause cache churn and put excessive load on CoreDNS.
* TTLs set too high
  * This could cause stale data to be served back to clients.

#### Support Procedures

Apply standard DNS troubleshooting methods to support this feature. See [Failure Modes](#failure-modes) for potential
issues customers could run into.

## Implementation History

This enhancement is being implemented in OpenShift 4.12.

## Alternatives

One potential alternative would be to do this in the infrastructure CoreDNS but that could run the risk of 
overloading a core functionality of the cluster and could make failure modes more impactful.

## Infrastructure Needed [optional]

N/A
