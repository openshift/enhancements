---
title: api-tls-curves-config
authors:
  - richardsonnick
  - davidesalerno
reviewers:
  - dsalerno # OpenShift networking stack knowledge
approvers: 
  - JoelSpeed
api-approvers:
  - JoelSpeed
creation-date: 2025-11-19
last-updated: 2025-11-20
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/HPCASE-153
---

# OpenShift API TLS Curves Configuration 

## Summary

This enhancement adds the option to configure a list of supported TLS curves in the OpenShift API config server. This configuration mirrors the existing `ciphersuites` option in the OpenShift API config TLS settings.

## Motivation

As cryptographic standards evolve, there is a growing need to support Post-Quantum Cryptography (PQC) to protect against future threats. This enhancement contributes directly to the goal of enabling PQC support in OpenShift. It provides the mechanism to configure specific TLS curves in the OpenShift API, allowing administrators to explicitly enable PQC-ready curves such as ML-KEM. This ensures OpenShift clusters can be configured to meet emerging security compliance requirements and future-proof communications.

### User Stories

As an administrator, I want to explicitely set the supported TLS curves to ensure PQC readiness throughout OpenShift so that I can ensure the security of TLS communication in the era of quantum computing.

### Goals

To provide an interface that allows the setting of TLS curves to be used cluser wide.

This goal is part of the larger goal to:
 1. Provide the necessary knobs to specify a PQC ready TLS configuration in OpenShift.
 2. Improve the adaptability of the cluster's TLS configuration to provide support for the constantly evolving TLS landscape.

### Non-Goals

1. Overhauling the current process of TLS configuration in OpenShift. This change merely extends the current TLS options.

## Proposal

This proposal is to expose the ability to specify the TLS curves used in OpenShift components to the OpenShift administrator.
Currently, administrators can specify a custom TLS profile where they can specifically set which TLS ciphersuites and the minimum TLS version as opposed to using one of the preconfigured TLS profiles. Specifying the set of supported TLS curves will mirror this process of setting [supported ciphers and the minimum TLS version](https://github.com/openshift/api/blob/138912d4ee9944c989f593c51f15c41908155856/config/v1/types_tlssecurityprofile.go#L206). 

The current state of the OpenShift TLS stack uses a default set of curves with no way to specify them. This eases the burden on administators, however new quantum secure algorithms rely on a set of curves outside of the conventional default curves. For example, curves like [ML-KEM](https://www.ietf.org/archive/id/draft-connolly-tls-mlkem-key-agreement-05.html) provide a quantum safe mechanism for sharing secrets necessary for the TLS handshake, whereas curves like [X22519](https://datatracker.ietf.org/doc/html/rfc7748) (a commonly used conventional curve) are [weak against quantum computing](https://crypto.stackexchange.com/questions/59770/how-effective-is-quantum-computing-against-elliptic-curve-cryptography).

The ability to set curves explicitely will also make it possible to align our 
OpenShift TLS profiles to match the curves present in the [Mozilla TLS Profiles](https://wiki.mozilla.org/Security/Server_Side_TLS). 

This change will require working with OpenShift component owners to use this new field, however this is outside the scope of this proposal.

### Workflow Description

Administrators will use the [existing custom TLS security profile flow](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/security_and_compliance/tls-security-profiles#tls-profiles-ingress-configuring_tls-security-profiles) for setting the supported curves. 

Specifically administrators will use 

`oc edit IngressController default -n openshift-ingress-operator`

and edit the spec.tlsSecurityProfile field:

```
apiVersion: operator.openshift.io/v1
kind: IngressController
 ...
spec:
  tlsSecurityProfile:
    type: Custom 
    custom: 
      ciphers: 
      - ECDHE-RSA-CHACHA20-POLY1305
      minTLSVersion: VersionTLS13
      curves:
      - X25519MLKEM512
 ...
```

### API Extensions

- Adds a `curves` field to the `spec.tlsSecurityProfile` 
- The addition of this field should not affect existing API behaviour

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A


#### Standalone Clusters

N/A


#### Single-node Deployments or MicroShift

N/A


### Implementation Details/Notes/Constraints

#### Mismatching curves and ciphersuites
There is a case where the administrator could incorrectly specificy a set of ciphersuites
that do not work with each other. For example using an RSA ciphersuite with a ECDHE curve (such as TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 and P-256). The default behavior OpenSSL as well as go's crypto/tls (both used extensively in OpenShift) is to fail at **TLS handshake time**. . The TLS server instance will start normally, but when TLS clients attempt to handshake with the TLS server, the handshake will fail with a `handshake failure`

To avoid this scenario, OpenShift should implement validation to prevent known invalid combinations. A validation layer will be added to check for compatible combinations of curves and ciphersuites. If a known invalid combination is detected, the configuration will be rejected, informing the user of the incompatibility immediately rather than failing at runtime.

### Risks and Mitigations

OpenShift components could forego utilizing the curves set in the API config. However, this is a risk
that exists in the current TLS config flow. This change will require coordination with component owners
to comply with the new TLS config field.

### Drawbacks

N/A

## Alternatives (Not Implemented)

N/A

## Open Questions [optional]

N/A

## Test Plan

Utilize the `oc edit` and `oc describe` commands to verify that the API config server is exposing the correct list of curves.

Once components are onboarded to utilize these curves, the cluster will be scanned with the [tls-scanner tool](github.com/openshift/tls-scanner) to verify that TLS implemenations within OpenShift expose these curves as supported. It should also be verified that the TLS implementations will fallback to a default curve set when not specified.

### Dev Preview -> Tech Preview

- Ability to specify supported curves.

### Tech Preview -> GA

- Verify the general support for these curves using the [tls-scanner](github.com/openshift/tls-scanner)

### Removing a deprecated feature

N/A


## Upgrade / Downgrade Strategy

In openshift versions where the TLS curves are not specified, components will not specify the set of curves to be used to their underlying TLS implementations. The TLS implementation should fallback to a sensible default set of curves when not set. This should be verified during the component onboarding work as outlined in the test plan.


## Version Skew Strategy

By default, TLS implementations (openssl, golang, etc...) fallback to a sensible default when curves are not set. Currently, openshift components that do not set curves exhibit this behavior. This should be verified during component onboarding.

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A