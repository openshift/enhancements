---
title: forward-dns-over-tls
authors:
  - "@brandisher"
reviewers:
  - "@Miciah"
  - "@rfredette"
  - "@candita"
  - "@frobware"
  - "@knobunc"
approvers:
  - "@Miciah"
  - "@knobunc"
api-approvers:
  - "@deads2k"
  - "@JoelSpeed"
creation-date: 2021-12-08
last-updated: 2022-04-05
tracking-link:
    - https://issues.redhat.com/browse/NE-703
see-also:
    - "/enhancements/dns/plugins.md"
---

# Enable the ability to configure DNS-over-TLS for forwarded requests

## Summary

This enhancement adds optional configuration to enable DNS-over-TLS when 
either `spec.servers[].forwardPlugin` and/or `spec.upstreamResolvers` are 
configured in the cluster DNS spec.

## Motivation

In environments that are highly regulated or geographically dispersed there
may be a need to forward requests to upstream DNS resolvers. Because DNS is
unencrypted by default there are concerns in these environments around data
privacy and integrity. Currently, there is no way to configure DNS-over-TLS for
upstream resolvers so there is also no workaround to alleviate this security
concern. The expansion of the `UpstreamResolver` and `ForwardPlugin` APIs  for 
DNS request forwarding improves the current security posture for DNS on 
OpenShift.

### Goals

For both default and non-default zones (`spec.servers[]` and `spec.
UpstreamResolvers`) the goals are

1. Allow a cluster administrator to define a transport of either `Cleartext` 
   or `TLS` for forwarded zones.
2. Allow a cluster administrator to define a `ServerName` which matches the 
   server certificate of the upstream resolver(s) for forwarded zones.
3. Allow a cluster administrator to optionally configure a custom CA that 
   will serve as a verification CA for the server certificate of the 
   upstream resolver(s).

### Non-Goals

* This enhancement applies only to the CoreDNS Operator shipped as part of OpenShift. Anything outside that scope would be not applicable to this enhancement including the CoreDNS static pods managed by the `machine-config-operator`.
* This enhancement does not create or enable a general purpose DNS-over-TLS server.  In particular, enabling TLS for communication between in-cluster resolvers and CoreDNS is out of scope for the enhancement.  

## Proposal

This proposal covers the following changes:
* Addition of the `DNSTransport` type.
* Addition of the `DNSTransportConfig` type.
* Addition of the `DNSOverTLSConfig` type.
* Modification of the `UpstreamResolvers` API to include `DNSTransportConfig`
* Modification of the `ForwardPlugin` API to include `DNSTransportConfig`

Below is an abridged version of the proposed changes. The full set of 
changes can be found in the respective [openshift/api PR](https://github.com/openshift/api/pull/1110).

API Additions:
```go
// DNSTransport indicates what type of connection should be used.
// +kubebuilder:validation:Enum=TLS;Cleartext;""
type DNSTransport string

const (
    // TLSTransport indicates that TLS should be used for the connection.
    TLSTransport DNSTransport = "TLS"
    
    // CleartextTransport indicates that no encryption should be used for
    // the connection.
    CleartextTransport DNSTransport = "Cleartext"
)

// DNSTransportConfig groups related configuration parameters used for configuring
// forwarding to upstream resolvers that support DNS-over-TLS.
// +union
type DNSTransportConfig struct {
    // transport allows cluster administrators to opt-in to using a DNS-over-TLS
    // connection between cluster DNS and an upstream resolver(s). Configuring
    // TLS as the transport at this level without configuring a CABundle will
    // result in the system certificates being used to verify the serving
    // certificate of the upstream resolver(s).
    //
    // Possible values:
    // "" (empty) - This means no explicit choice has been made and the platform chooses the default which is subject
    // to change over time. The current default is "Cleartext".
    // "Cleartext" - Cluster admin specified cleartext option. This results in the same functionality
    // as an empty value but may be useful when a cluster admin wants to be more explicit about the transport,
    // or wants to switch from "TLS" to "Cleartext" explicitly.
    // "TLS" - This indicates that DNS queries should be sent over a TLS connection. If Transport is set to TLS,
    // you MUST also set ServerName. If a port is not included with the upstream IP, port 853 will be tried by default
    // per RFC 7858 section 3.1; https://datatracker.ietf.org/doc/html/rfc7858#section-3.1.
    //
    // +optional
    // +unionDiscriminator
    Transport DNSTransport `json:"transport,omitempty"`
    
    // tls contains the additional configuration options to use when Transport is set to "TLS".
    TLS *DNSOverTLSConfig `json:"tls,omitempty"`
}

// DNSOverTLSConfig describes optional DNSTransportConfig fields that should be captured.
type DNSOverTLSConfig struct {
    // serverName is the upstream server to connect to when forwarding DNS queries. This is required when Transport is
    // set to "TLS". ServerName will be validated against the DNS naming conventions in RFC 1123 and should match the
    // TLS certificate installed in the upstream resolver(s).
    //
    // + ---
    // + Inspired by the DNS1123 patterns in Kubernetes: https://github.com/kubernetes/kubernetes/blob/7c46f40bdf89a437ecdbc01df45e235b5f6d9745/staging/src/k8s.io/apimachinery/pkg/util/validation/validation.go#L178-L218
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`
    ServerName string `json:"serverName"`
    
    // caBundle references a ConfigMap that must contain either a single
    // CA Certificate or a CA Bundle. This allows cluster administrators to provide their
    // own CA or CA bundle for validating the certificate of upstream resolvers.
    //
    // 1. The configmap must contain a `ca-bundle.crt` key.
    // 2. The value must be a PEM encoded CA certificate or CA bundle.
    // 3. The administrator must create this configmap in the openshift-config namespace.
    // 4. The upstream server certificate must contain a Subject Alternative Name (SAN) that matches ServerName.
    //
    // +optional
    CABundle v1.ConfigMapNameReference `json:"caBundle,omitempty"`
}
```

`UpstreamResolvers` Modifications:
```go
type UpstreamResolvers struct {
  // <snip>

  // transportConfig is used to configure the transport type, server name, and optional custom CA or CA bundle to use
  // when forwarding DNS requests to an upstream resolver.
  //
  // The default value is "" (empty) which results in a standard cleartext connection being used when forwarding DNS
  // requests to an upstream resolver.
  //
  // +optional
  TransportConfig DNSTransportConfig `json:"transportConfig,omitempty"`
}
```

`ForwardPlugin` modifications:
```go
type ForwardPlugin struct {
  // <snip>

  // transportConfig is used to configure the transport type, server name, and optional custom CA or CA bundle to use
  // when forwarding DNS requests to an upstream resolver.
  //
  // The default value is "" (empty) which results in a standard cleartext connection being used when forwarding DNS
  // requests to an upstream resolver.
  //
  // +optional
  TransportConfig DNSTransportConfig `json:"transportConfig,omitempty"`
}
```

Because of the limitation in CoreDNS that you can only have one `forward` 
block per server and the TLS configuration for a forward block is global for the entire block,
DNS-over-TLS will be attempted for all upstreams configured for a zone by 
default. Failure to establish a TLS connection for an upstream will result in a 
log record indicating that a TLS connection could not be established.

A cluster administrator can set only `Transport` and `ServerName` to allow
the system certificates to be used for upstream resolver certificate
validation. They can additionally set `CABundle` to use their own CA (or
set of CAs) to validate upstream resolver certificates.

### User Stories

#### As a cluster administrator, I need to configure TLS for forwarded DNS requests so that security and integrity can be maintained

To satisfy this use case for the default zone, the cluster administrator can 
configure TLS like the following:

Example:
```yaml
spec:
  upstreamResolvers:
    transportConfig:
      transport: TLS
      tls:
        caBundle:
          name: mycacert
        serverName: upstream-tls
    upstreams:
      - type: Network
        address: 1.1.1.1
        port: 853
      - type: Network
        address: 2.2.2.2
        port: 5353
```

To satisfy this use case for non-default zones, the cluster administrator 
can configure TLS like the following:

Example:
```yaml
spec:
  servers:
    - name: foo-server
      zones:
        - foo.com
      forwardPlugin:
        policy: Random
        transportConfig:
          transport: TLS
          tls:
            caBundle:
              name: mycacert
            serverName: upstream-tls
        upstreams:
          - 1.1.1.1
          - 2.2.2.2:5353
```

### API Extensions

The `UpstreamResolvers` and `ForwardPlugin` APIs will be modified to allow 
for TLS configuration.

### Implementation Details/Notes/Constraints [optional]

Implementing this enhancement requires changes in the following repositories:

openshift/api
openshift/cluster-dns-operator

### Risks and Mitigations

**Risk 1:**
* Moving away from CoreDNS could impact the API

**Mitigations:**
* Ensure the API is flexible enough to handle reasonable digression from 
  what CoreDNS offers in the `forward` plugin.

**Security:** Given that this enhancement is inherently security related, 
security review of the approach will be preferred.

**UX:** Given that this enhancement is only accessible via configuration 
editing, no UX testing is required.

## Design Details

### Open Questions [optional]

Q. Should the cluster administrator supplied certificates be stored in the openshift-dns or openshift-dns-operator namespaces?

A. "The administrator should put the configmaps in the openshift-config namespace, and the operator should copy the configmaps from there to the openshift-dns namespace so the DNS pods can use them in volume mounts."

### Test Plan

A new e2e test will be added; TestDNSOverTLSForwarding. This can reuse 
most of the existing logic from the TestDNSForwarding e2e test. The 
implementation is intended to be relatively straightforward so no special 
testing considerations are known at this time aside from the need for an 
upstream resolver that has DNS-over-TLS enabled.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

On upgrade, the default configuration and existing forward configuration 
will work without issue.

On downgrade, this functionality will revert to sending plaintext 
DNS queries since the downgraded operator will have the older CRD 
and logic that does not include these new API fields. Cluster 
administrators should remove configuration specific to this enhancement 
before downgrade.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

* **Misconfiguration of the TLS certs**
  * Cluster administrators can revert the configuration.
  * CoreDNS will log an error if the TLS negotiation with the upstream
    resolver fails.

#### Support Procedures

* TLS will make packet captures difficult as all the information will be 
  encrypted. Support will require a tool to capture a pcap as well as 
  decrypt the pcap using a key (if available). We may also consider 
  something like [KeyLogWriter](https://pkg.go.dev/crypto/tls#example-Config-KeyLogWriter)
  but that would require an upstream change and be outside the scope of this 
  enhancement.

## Implementation History

This enhancement is being implemented in OpenShift 4.11.

### Drawbacks

This enhancement adds additional complexity to the `UpstreamResolver` and 
`ForwardPlugin` APIs. If the underlying DNS technology were to change in 
the future (away from CoreDNS) we would need to ensure that the new 
technology supported the same capabilities.

## Alternatives

This could potentially be done with systemd-resolved, however, that's not 
enabled by default in RHEL and would add a dependency into the core OS 
instead of relying on CoreDNS to serve user workloads.

## Infrastructure Needed [optional]

N/A
