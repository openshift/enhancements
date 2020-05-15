---
title: cluster-version-operator-x.509-upstream-trust
authors:
  - "@wking"
reviewers:
  - "@LalatenduMohanty"
approvers:
  - "@bparees"
  - "@deads2k"
creation-date: 2020-05-12
last-updated: 2020-05-14
status: provisional
see-also:
  - "/enhancements/automated-service-ca-rotation.md"
  - "/enhancements/kube-apiserver/certificates.md"
  - "/enhancements/kube-apiserver/tls-config.md"
  - "/enhancements/proxy/global-cluster-egress-proxy.md"
  - "https://github.com/openshift/enhancements/pull/115"
---

# Cluster-version Operator X.509 Upstream Trust

This enhancement provides a mechanism for providing [the cluster-version operator][cluster-version-operator] (CVO) with an alternative trust-store and TLS configuration to use when connecting to [the update recommendation service][update-service] that overrides both the CVO's defaults and any [proxy configuration](../proxy/global-cluster-egress-proxy.md).

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

The CVO retrieves available updates from [an update recommendation service][update-service] serving [the Cincinnati protocol][cincinnati].
The CVO also retrieves release image signatures from [configured stores][update-keys] (and [other places](../oc/mirroring-release-image-signatures.md)) to establish trust for the recommended update targets.
Both of those may require HTTPS calls to external services.

In some cases, those HTTPS calls will use a [configured proxy](../proxy/global-cluster-egress-proxy.md).
And currently there is no mechanism for configuring additional HTTP(S) signature stores.
But the update recommendation service is configurable [with ClusterVersion's `upstream` property][api-upstream], and adminstrators may point that at internal services whose X.509 certificate is not signed by a certificate authority (CA) that the CVO trusts by default.

## Motivation

Communication to any HTTPS endpoint requires a few inputs:

* Target: this is usually a URI, in this case the existing [`upstream`][api-upstream] property.
* Security profile: this defines the acceptable versions, cyphers, etc. used in the negotiated TLS connection, and this enhancement adds `upstreamTLS.securityProfile` for this purpose.
* Target verification: this is a CA bundle, and this enhancement adds `upstreamTLS.trustedCA` for this purpose.
* Client verification: this can be a bearer token, client cert/key pair, challenge-response exchange, etc.
    It is [out of scope](#non-goals) for this enhancement.

This is true of communication to any endpoint by any client.
Not all endpoints imply the same level trust, and attempting to address multiple targets via a single security profile, target verification configuration, and client verification configuration broadens the risk of compromise.
One concrete example: a service serving CA bundle should not be trusted for names that are not services, because it would expose those non-service connections to an attacker who manages to compromise the service serving CA.

### Goals

Administrators will be able to configure the CVO to communicate with their configured [`upstream`][api-upstream], even if the TLS terminates with a service that:

* Uses an X.509 certificate signed by a CA that is not included in the CVO's default trust store.
* Uses TLS connection properties (version, ciphers, etc.) that are not accepted by the CVO's default TLS profile.

### Non-Goals

This enhancement does not address components outside the CVO.
[Enhancement 115][enhancement-115] proposed a general framework for configuring X.509 trust, but was [rejected][enhancement-115-rejection] in favor of per-component solutions.

This enhancement does not address signature retrieval.
Currently [the configured stores][update-keys] are hard-coded, so the CVO will only talk to an alternative TLS terminator when it is being [proxied](../proxy/global-cluster-egress-proxy.md).
The fact that the current proxy configuration does not clearly address proxies defined at the network-routing level, where `httpsProxy` may be empty but additional trust still needs to be injected is out of scope.

This enhancement does not address client authentication.
If needed, properties for client authentication may be added to this enhancement's `TLSConfig` structure in future enhancements.

## Proposal

### `TLSConfig`

A new config type `TLSConfig` will be added consuming the existing [`TLSSecurityProfile`][api-tls-security-profile] and [`ConfigMapNameReference`][api-config-map-name-reference].
The `securityProfile` property and its comment are copied from [the existing `APIServerSpec` consumer][api-server-tls-security-profile] with the leading `TLS` dropped because `TLSConfig` is already TLS-specific.
The `TrustedCA` property and its comment are based on [the existing `Proxy` consumer][api-proxy-trusted-ca].

```go
type TLSConfig {
	// securityProfile specifies settings for TLS connections for externally exposed servers.
	//
	// If unset, a default (which may change between releases) is chosen. Note that only Old and
	// Intermediate profiles are currently supported, and the maximum available MinTLSVersions
	// is VersionTLS12.
	// +optional
	SecurityProfile *TLSSecurityProfile `json:"securityProfile,omitempty"`

	// trustedCA is a reference to a ConfigMap containing a PEM-encoded X.509
	// CA certificate bundle.  The consuming operator is responsible for:
	//
	// 1. Reading and validating the certificate bundle from the
	//    required key "ca-bundle.crt".
	// 2. Optionally merging it with the system default trust bundle, as
	//    defined by the TLSConfig consumer.
	// 3. Feeding the resulting trust bundle to the appropriate operand, as
	//    defined by the TLSConfig consumer.  This may involve writing a
	//    ConfigMap in an operator-managed namespace that is then
	//    volume-mounted into operand pods.
	//
	// The namespace for the ConfigMap referenced by trustedCA is
	// "openshift-config". Here is an example ConfigMap (in yaml):
	//
	// apiVersion: v1
	// kind: ConfigMap
	// metadata:
	//  name: user-ca-bundle
	//  namespace: openshift-config
	//  data:
	//    ca-bundle.crt: |
	//      -----BEGIN CERTIFICATE-----
	//      Custom CA certificate bundle.
	//      -----END CERTIFICATE-----
	//
	// +optional
	TrustedCA *ConfigMapNameReference `json:"trustedCA,omitempty"`
}
```

### `upstreamTLS`

[`ClusterVersionSpec`][api-spec] will be extended to include the following new property:

```go
// upstreamTLS specifies settings for TLS connections to the upstream
// update service.  If trustedCA is set, it is used directly and not
// merged with the system default trust bundle.  If upstreamTLS is set,
// even if none of its constituent properties are set, it takes
// precedence over the proxy configuration.  To use the proxy
// configuration for connections to the upstream update service, unset
// upstreamTLS completely.
// +optional
UpstreamTLS *TLSConfig `json:"upstreamTLS,omitempty"
```

### User Stories

#### Local update recommendation service with non-standard certificate authority

Users running a local [update recommendation service][update-service] (such as [Cincinnati][cincinnati]) may use a serving certificate signed by an internal CA.
This enhancement will allow the CVO to successfully pull recommended updates from those services.
Connecting to the services over HTTP (without TLS) would be unwise, because an attacker who can man-in-the-middle the update recommendation service can suggest clusters do crazy things (e.g. "I see you're on 4.4.3.  I suggest you update to 4.1.0"), for which there are no CVO-side guards at the moment.
And in some cases (e.g. updating to fumbled releases which we initially expected to be healthy), no CVO-side guards are possible.
This enhancement enables trusted, secure connections to the update recommendation service without requiring users to provision a certificate signed by a default CA.

#### Transparent, network-level proxies

[rhbz#1773419][rhbz1773419] discusses issue when CVO upstream requests are not redirected via an `httpsProxy` setting, but are instead routed through a proxy via network-level configuration (e.g. all outgoing TCP traffic is routed through a proxy).
The CVO currently only injects the Proxy config's `trustedCA` data [when `httpsProxy` is non-empty][cluster-version-operator-trustedCA-vs-httpsProxy], so there is currently no mechanism for configuring the CVO's trusted CA store in this situation, short of forcing an explicit value into `httpsProxy`.
While it's possible that the Proxy semantics would encourage consumers to ingest `trustedCA` in the absence of `httpsProxy`, this enhancement allows for users to configure trusted CAs without going through the Proxy object.

### Implementation Details/Notes/Constraints

The network operator supports copying trust bundles between ConfigMaps based on labels on the target ConfigMaps.
But because we are not using the Proxy configuration, the CVO will need to grow its own implementation to retrieve data from the referenced ConfigMap in the `openshift-config` namespace.
Because it is the CVO retrieving and consuming the data, there is no need to copy a ConfigMap over to volume-mount into the CVO pod.
The CVO just needs to adapt its existing [retrieving][cluster-version-operator-get-trust-bundle] and [consuming][cluster-version-operator-use-tls-config] code to include the new properties at with a higher precedence than the existing Proxy `trustedCA` handling.

### Risks and Mitigations

It is possible that it is worth copying valid trust bundles into the `openshift-config-managed` namespace to guard against data loss if users write broken trust bundles into the `openshift-config` "input" namespace.
The CVO could also guard against broken data with an admission controller.
But because acccess to the upstream update recommendation service is not critical to immediate cluster functionality, it is probably fine to just allow users who write broken trust bundles to break `upstream` access until they fix the trust bundle.
There is no way that that broken data could impact their ability to recover by writing a valid trust bundle.

## Design Details

### Test Plan

CVO unit tests would stand up a dummy handler with a self-signed certificate and use fake ClusterVersion and ConfigMap clients to validate the enhancement behavior.
There would be no additional intergration tests covering this behavior.

### Graduation Criteria

This behavior would be born into GA in the first, supported release of OpenShift that shipped it.

### Version Skew Strategy

Downgrading to a version whose ClusterVersion custom resource definition did not support the new properties would clear the `upstreamTLS` data and break further access to the update recommendation service which required it.
Downgrading seems unlikely, and recovery seems straightforward enough (the CA ConfigMap would still be there, just restore the `upstreamTLS` values) that there is no mitigation strategy.

## Implementation History

None yet :).

## Alternatives

### `ConfigMapNameReference` without a pointer

The `*ConfigMapNameReference` pointer works around [Go's limitations on `omitempty` for `struct` types][go-11939].
While there are several `ConfigMapNameReference` properties in the API repository now and many are `+optional`, [the `Proxy` consumer][api-proxy-trusted-ca] is the only one that is nominally `omitempty`.
Having a working `omitempty` (via the pointer) will reduce distraction for users reading generated YAML (and other formats), which outweighs the small increase in complexity involved with pointer-aware code.

### Standardized X.509 configuration

A [standard framework][enhancement-115] would allow users to configure additional X.509 trust for their entire cluster and override it as necessary for specific components.
This seems like it would be more convenient for users whose cluster interacts with a number of services whose certificates are signed by an organization's default "internal CA" tooling.
But that approach was [rejected][enhancement-115-rejection] in favor of one-off solutions such as the one proposed in this enhancement.

### Inline trust bundles

Inline trust bundles, as done with [`externalServerURLs`][externalServerURLs] for `spokecores.nucleus.open-cluster-management.io`.
This would reduce the number of moving pieces (no need for an ConfigMap), but makes it harder to share a common trust bundle between different consuming components.
With the ConfigMap approach, users can drop their internal, corporate CA(s) into a single `openshift-config` ConfigMap, and point all of the components that should consume it at that single ConfigMap (assuming they all happen to agree on the [PEM-encoded][rfc-7468] [X.509][rfc-5280] format, `ca-bundle.crt` key, "and do not merge with the system default trust bundle" semantics.

[api-config-map-name-reference]: https://github.com/openshift/api/blob/944c57cb1477eaee4a94e3f82c8aab639fd0b94c/config/v1/types.go#L16-L23
[api-proxy-trusted-ca]: https://github.com/openshift/api/blob/944c57cb1477eaee4a94e3f82c8aab639fd0b94c/config/v1/types_proxy.go#L44-L69
[api-server-tls-security-profile]: https://github.com/openshift/api/blob/944c57cb1477eaee4a94e3f82c8aab639fd0b94c/config/v1/types_apiserver.go#L47-L53
[api-spec]: https://github.com/openshift/api/blob/944c57cb1477eaee4a94e3f82c8aab639fd0b94c/config/v1/types_cluster_version.go#L28-L73
[api-tls-security-profile]: https://github.com/openshift/api/blob/944c57cb1477eaee4a94e3f82c8aab639fd0b94c/config/v1/types_tlssecurityprofile.go
[api-upstream]: https://github.com/openshift/api/blob/944c57cb1477eaee4a94e3f82c8aab639fd0b94c/config/v1/types_cluster_version.go#L56-L60
[cincinnati]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md
[cluster-version-operator]: https://github.com/openshift/cluster-version-operator/
[cluster-version-operator-get-trust-bundle]: https://github.com/openshift/cluster-version-operator/blob/8240a9b3711fa6938129d06ee8c6957a8f3b6464/pkg/cvo/availableupdates.go#L232-L253
[cluster-version-operator-trustedCA-vs-httpsProxy]: https://github.com/openshift/cluster-version-operator/blob/8240a9b3711fa6938129d06ee8c6957a8f3b6464/pkg/cvo/availableupdates.go#L221-L226
[cluster-version-operator-use-tls-config]: https://github.com/openshift/cluster-version-operator/blob/8240a9b3711fa6938129d06ee8c6957a8f3b6464/pkg/cvo/availableupdates.go#L48-L55
[enhancement-115]: https://github.com/openshift/enhancements/pull/115
[enhancement-115-rejection]: https://github.com/openshift/enhancements/pull/115#issuecomment-580878147
[externalServerURLs]: https://github.com/open-cluster-management/api/pull/22/files#diff-13664523849861d62b79a83bb525f0e8R46
[go-11939]: https://github.com/golang/go/issues/11939
[rfc-5280]: https://tools.ietf.org/html/rfc5280
[rfc-7468]: https://tools.ietf.org/html/rfc7468
[rhbz1773419]:https://bugzilla.redhat.com/show_bug.cgi?id=1773419
[update-keys]: https://github.com/openshift/cluster-update-keys
[update-service]: https://docs.openshift.com/container-platform/4.4/updating/updating-cluster-between-minor.html#update-service-overview_updating-cluster-between-minor
