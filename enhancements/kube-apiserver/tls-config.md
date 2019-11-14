---
title: tls-config-for-externally-facing-services
authors:
  - "@deads2k"
  - "@danehans"
reviewers:
  - "@danehans"
  - "@enj"
  - "@ironcladlou"
  - "@sttts"
approvers:
  - "@ironcladlou"
  - "@sttts"
creation-date: 2019-09-17
last-updated: 2019-09-17
status: implementable
see-also:
replaces:
superseded-by:
---

# TLS Config for Externally Facing Services

Cluster-admins need to be able to choose which cipher suites their kube-apiserver, oauth-server, and ingress controllers use.
This is a balance of corporate policy and compatibility with existing systems.  This should be a shared, top level
configuration amongst the affected components with a local override for ingress controllers to allow workloads to have a different 
level of TLS config restrictions than the infrastructure.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Cluster-admins need to be able to choose which cipher suites their kube-apiserver, oauth-server, and ingress controllers use.
This is a balance of corporate policy and compatibility with existing systems.  This should be a shared, top level
configuration amongst the affected components with a local override for ingress controllers to allow workloads to have a different 
level of TLS config restrictions than the infrastructure.

The idea is to create a set of standard profiles: old, intermediate, modern, and custom that can be used.  Old, intermediate, 
and modern are intent based.  We will update them as we see fit to stay up to date with standards as time goes on.  Custom
allows a user to specify exactly which options they want.

## Motivation

We want guardrails where possible to avoid letting a customer make choices that prevent future upgrades.  Lots of flexibility
is very easy to produce once, but very hard to maintain in an auto-updating world.  We will use profiles to constrain that choice.

### Goals

1. Easily configure a uniform level of TLSConfig (ciphers, tls versions, and DH key size) across kube-apiserver, oauth-server,
 and all ingress controllers. These are the only known exposed services today.
2. Allow overriding on a per-ingress-controller basis.  Workloads and infrastructure may have different requirements and we need to 
 support that use-case.

### Non-Goals

1. Make it easy to implement unusual configurations.
2. Make decisions about future restrictions like tls curves.

## Proposal

See https://github.com/openshift/api/pull/432 for the details of the API to be included, but at a high level it looks like this

```go
// TLSSecurityProfile defines the schema for a TLS security profile. This object
// is used by operators to apply TLS security settings to operands.
// +union
type TLSSecurityProfile struct {
	// type is one of Old, Intermediate, Modern or Custom. Custom provides
	// the ability to specify individual TLS security profile parameters.
	// Old, Intermediate and Modern are TLS security profiles based on:
	//
	// https://wiki.mozilla.org/Security/Server_Side_TLS#Recommended_configurations
	//
	// The profiles are intent based, so they may change over time as new ciphers are developed and existing ciphers
	// are found to be insecure.  Depending on precisely which ciphers are available to a process, the list may be
	// reduced.
	//
	// +unionDiscriminator
	// +optional
	Type TLSProfileType `json:"type"`
	// old is a TLS security profile based on:
	//
	// https://wiki.mozilla.org/Security/Server_Side_TLS#Old_backward_compatibility
	//
	// +optional
	// +nullable
	Old *OldTLSProfile `json:"old,omitempty"`
	// intermediate is a TLS security profile based on:
	//
	// https://wiki.mozilla.org/Security/Server_Side_TLS#Intermediate_compatibility_.28default.29
	//
	// +optional
	// +nullable
	Intermediate *IntermediateTLSProfile `json:"intermediate,omitempty"`
	// modern is a TLS security profile based on:
	//
	// https://wiki.mozilla.org/Security/Server_Side_TLS#Modern_compatibility
	//
	// +optional
	// +nullable
	Modern *ModernTLSProfile `json:"modern,omitempty"`
	// custom is a user-defined TLS security profile. Be extremely careful using a custom
	// profile as invalid configurations can be catastrophic. 
	//
	// +optional
	// +nullable
	Custom *CustomTLSProfile `json:"custom,omitempty"`
}

// OldTLSProfile is a TLS security profile based on:
// https://wiki.mozilla.org/Security/Server_Side_TLS#Old_backward_compatibility
type OldTLSProfile struct{}

// IntermediateTLSProfile is a TLS security profile based on:
// https://wiki.mozilla.org/Security/Server_Side_TLS#Intermediate_compatibility_.28default.29
type IntermediateTLSProfile struct{}

// ModernTLSProfile is a TLS security profile based on:
// https://wiki.mozilla.org/Security/Server_Side_TLS#Modern_compatibility
type ModernTLSProfile struct{}

// CustomTLSProfile is a user-defined TLS security profile. Be extremely careful
// using a custom TLS profile as invalid configurations can be catastrophic.
type CustomTLSProfile struct {
	TLSProfileSpec `json:",inline"`
}

// TLSProfileType defines a TLS security profile type.
type TLSProfileType string

const (
	// Old is a TLS security profile based on:
	// https://wiki.mozilla.org/Security/Server_Side_TLS#Old_backward_compatibility
	TLSProfileOldType TLSProfileType = "Old"
	// Intermediate is a TLS security profile based on:
	// https://wiki.mozilla.org/Security/Server_Side_TLS#Intermediate_compatibility_.28default.29
	TLSProfileIntermediateType TLSProfileType = "Intermediate"
	// Modern is a TLS security profile based on:
	// https://wiki.mozilla.org/Security/Server_Side_TLS#Modern_compatibility
	TLSProfileModernType TLSProfileType = "Modern"
	// Custom is a TLS security profile that allows for user-defined parameters.
	TLSProfileCustomType TLSProfileType = "Custom"
)

// TLSProfileSpec is the desired behavior of a TLSSecurityProfile.
type TLSProfileSpec struct {
	// ciphers is used to specify the cipher algorithms that are negotiated
	// during the TLS handshake.  Operators may remove entries their operands
	// do not support.  For example, to use 3DES  (yaml):
	//
	//   ciphers:
	//     - 3DES
	//
	Ciphers []string `json:"ciphers"`
	// tlsVersion is used to specify one or more versions of the TLS protocol
	// that is negotiated during the TLS handshake. For example, to use TLS
	// versions 1.1, 1.2 and 1.3 (yaml):
	//
	//   tlsVersion:
	//     minimumVersion: TLSv1.1
	//     maximumVersion: TLSv1.3
	//
	TLSVersion TLSVersion `json:"tlsVersion"`
	// dhParamSize sets the maximum size of the Diffie-Hellman parameters used for generating
	// the ephemeral/temporary Diffie-Hellman key in case of DHE key exchange. The final size
	// will try to match the size of the server's RSA (or DSA) key (e.g, a 2048 bits temporary
	// DH key for a 2048 bits RSA key), but will not exceed this maximum value.
	//
	// Available DH Parameter sizes are:
	//
	//   "2048": A Diffie-Hellman parameter of 2048 bits.
	//   "1024": A Diffie-Hellman parameter of 1024 bits.
	//
	// For example, to use a Diffie-Hellman parameter of 2048 bits (yaml):
	//
	//   dhParamSize: 2048
	//
	DHParamSize DHParamSize `json:"dhParamSize"`
}

// TLSVersion defines one or more versions of the TLS protocol that are negotiated
// during the TLS handshake.
type TLSVersion struct {
	// minimumVersion enforces use of the specified TLSProtocolVersion or newer
	// that are negotiated during the TLS handshake. minimumVersion must be lower
	// than or equal to maximumVersion.
	//
	// If unset and maximumVersion is set, minimumVersion will be set
	// to maximumVersion. If minimumVersion and maximumVersion are unset,
	// the minimum version is determined by the TLS security profile type.
	MinimumVersion TLSProtocolVersion `json:"minimumVersion"`
	// maximumVersion enforces use of the specified TLSProtocolVersion or older
	// that are negotiated during the TLS handshake. maximumVersion must be higher
	// than or equal to minimumVersion.
	//
	// If unset and minimumVersion is set, maximumVersion will be set
	// to minimumVersion. If minimumVersion and maximumVersion are unset,
	// the maximum version is determined by the TLS security profile type.
	MaximumVersion TLSProtocolVersion `json:"maximumVersion"`
}

// TLSProtocolVersion is a way to specify the protocol version used for TLS connections.
// Protocol versions are based on the following most common TLS configurations:
//
//   https://ssl-config.mozilla.org/
//
// Note that SSLv3.0 is not a supported protocol version due to well known
// vulnerabilities such as POODLE: https://en.wikipedia.org/wiki/POODLE
type TLSProtocolVersion string

const (
	// TLSv1.0 is version 1.0 of the TLS security protocol.
	VersionTLS10 TLSProtocolVersion = "TLSv1.0"
	// TLSv1.1 is version 1.1 of the TLS security protocol.
	VersionTLS11 TLSProtocolVersion = "TLSv1.1"
	// TLSv1.2 is version 1.2 of the TLS security protocol.
	VersionTLS12 TLSProtocolVersion = "TLSv1.2"
	// TLSv1.3 is version 1.3 of the TLS security protocol.
	VersionTLS13 TLSProtocolVersion = "TLSv1.3"
)

// DHParamSize sets the maximum size of the Diffie-Hellman parameters used for
// generating the ephemeral/temporary Diffie-Hellman key.
type DHParamSize string

const (
	// 1024 is a Diffie-Hellman parameter of 1024 bits.
	DHParamSize1024 DHParamSize = "1024"
	// 2048 is a Diffie-Hellman parameter of 2048 bits.
	DHParamSize2048 DHParamSize = "2048"
)

// TLSProfiles Contains a map of TLSProfileType names to TLSProfileSpec.
//
// NOTE: The caller needs to make sure to check that these constants are valid for their binary. Not all
// entries map to values for all binaries.  In the case of ties, the kube-apiserver wins.  Do not fail,
// just be sure to whitelist only and everything will be ok.
// TODO write the specifics of the ciphers, that's just too long to place in this KEP, it is in the openshift/api pull.
var TLSProfiles = map[TLSProfileType]*TLSProfileSpec{
	TLSProfileOldType: {
		Ciphers: []string{
		},
		TLSVersion: TLSVersion{
			MaximumVersion: VersionTLS13,
			MinimumVersion: VersionTLS10,
		},
		DHParamSize: DHParamSize1024,
	},
	TLSProfileIntermediateType: {
		Ciphers: []string{
		},
		TLSVersion: TLSVersion{
			MaximumVersion: VersionTLS13,
			MinimumVersion: VersionTLS12,
		},
		DHParamSize: DHParamSize2048,
	},
	TLSProfileModernType: {
		Ciphers: []string{
		},
		TLSVersion: TLSVersion{
			MaximumVersion: VersionTLS13,
			MinimumVersion: VersionTLS12,
		},
		DHParamSize: nil,
	},
}

``` 

The APIServer will get a new optional member like this:

```go
type APIServerSpec struct {
	// ...snip
	
	// tlsSecurityProfile specifies settings for TLS connections for externally exposed servers.
	//
	// If unset, a default (which may change between releases) is chosen.
	// +optional
	TLSSecurityProfile *TLSSecurityProfile `json:"tlsSecurityProfile,omitempty"`
}
```

The `TLSProfiles` map will be used to ensure that individual components honor the same meaning for the available
profiles.  This is congruent to how we coordinate features across operators.


### Implementation Details/Notes/Constraints 

Implementation has multiple steps.

1. Agree on and merge the API. (apiserver and network-edge must agree)
2. Update the kube-apiserver-operator to plumb the ciphers through. (apiserver)
3. Update the authentication-operator to plumb the ciphers through. (apiserver)
4. Update the ingress-operator to plumb the ciphers through. (network-edge)
5. IngressOperatorSpec needs an override built using the same profile API (network-edge)
6. Update the ingress-operator to allow the overriden the ciphers through. (network-edge)

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

Each operator can build an e2e CI test.

### Graduation Criteria

### Upgrade / Downgrade Strategy

On downgrade, the values will no longer be respected.

On upgrade, it is possible to see values you don't understand. Operators must correctly elide unknown ciphers.  Because
we are whitelist only, this is safe.  It can be tested by setting unknown values in a custom profile.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
