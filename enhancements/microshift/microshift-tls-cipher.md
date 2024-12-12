---
title: microshift-tls-cipher
authors:
  - "@pacevedom"
reviewers:
  - "@eslutsky"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2024-11-29
last-updated: 2024-11-29
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1070
---

# MicroShift TLS cipher configuration options

## Summary
MicroShift offers several endpoints for external connectivity out of the box,
such as the API server and the default router. These endpoints come with TLS
enabled, enhancing security in communications. As noted in the
[RFC 5246](https://datatracker.ietf.org/doc/html/rfc5246#appendix-A.5), TLS
uses one of the listed cipher suites in the ClientHello and ServerHello
messages. Cipher suites are the encryption algorithms used in all the
information exchange within a connection.Some of the TLS cipher suites,
however, are not considered secure anymore. In order to use the secure options
or even narrow the available suites that MicroShift should use, configuration
options need to be introduced for users and admins.

> Default router TLS configuration is handled in a different feature.

## Motivation
MicroShift's exposed endpoints (such as the default router and the API server)
are using TLS. In order to secure the API server only TLS 1.2 or above is
allowed.

Cipher suites are key to the security of TLS handshakes and information
exchange, and different versions of TLS use different cipher suites. Some TLS
versions (like 1.0 and 1.1) have been deprecated in favor of new ones, being
1.2 the most widespread. As with TLS versions, the cipher suites have also
evolved and some of them have been compromised and are now unsafe for use.
For these reasons, the possibility of restricting which cipher suites to use
becomes important for serving endpoints, as they allow administrators to
protect their APIs.

TLS 1.3 has had changes in cipher suites to simplify them, meaning the
available suites has been reduced to only a handful. Most cipher suites from
TLS 1.2 are incompatible with 1.3. Further details on this topic can be found
in the proposal section.

MicroShift API server is using TLS 1.2 as the minimum version, supporting also
1.3. Cluster admins who are using tools with TLS 1.2 or above may need to
comply with security policies restricting which cipher suites can be used.

### User Stories
As a MicroShift admin, I want to be able to configure TLS cipher suites for the
API endpoints.

As a MicroShift admin, I want to be able to configure the minimum TLS version
for the API endpoints.

### Goals
* Allow users to configure which TLS version and which TLS ciphers are used in
  API endpoints of MicroShift.

### Non-Goals
N/A

## Proposal
Introduce configuration options for API server cipher suites.
```yaml
apiServer:
  tls:
    # Defaults to the suites of the configured minVersion. When using TLS 1.3
    # the cipherSuites user input is ignored and replaced with TLS 1.3 default
    # suites.
    cipherSuites:
    - <cipher suite 1>
    - ...
    minVersion: <VersionTLS12|VersionTLS13> # Defaults to 1.2
```
The API server will use the configured minimum TLS version (which defaults to
1.2) and the cipher suites. Note that cipher suites may be unconfigured, in
which case it will automatically pick up the defaults for the configured
minimum version.

The default cipher suites for 1.2 are:
* TLS_AES_128_GCM_SHA256
* TLS_AES_256_GCM_SHA384
* TLS_CHACHA20_POLY1305_SHA256
* TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
* TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
* TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384
* TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384
* TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256
* TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256

The default cipher suites for 1.3 are:
* TLS_AES_128_GCM_SHA256
* TLS_AES_256_GCM_SHA384
* TLS_CHACHA20_POLY1305_SHA256

For simplicity, the API server cipher suites will also apply to internal
control plane components:
* API server
* Kubelet
* Kube controller manager
* Kube scheduler
* Etcd
* Route controller manager

Except for API server the rest of the components are internal to MicroShift
and require secure connections too.

IMPORTANT: Because of limitations in the Go implementation of TLS, using
version 1.3 does not allow to change the cipher suites. Whenever TLS 1.3 is
configured in `tls.minVersion`, `tls.cipherSuites` will take the default suites
for TLS 1.3, ignoring and overwriting whatever was configured.

### Workflow Description
**cluster admin** is a human user responsible for configuring a MicroShift
cluster.

1. The cluster admin adds specific configuration for API server cipher suites
   prior to MicroShift's start.
2. After MicroShift starts, all the control plane components (API server, kube
   controller manager, kube scheduler) will be configured with the specified
   cipher suites.
3. All clients connecting to API server must support the configured cipher
   suites or else connections will fail in TLS handshake phase.

### API Extensions
As described in the proposal section, there is one new configuration option:
```yaml
apiServer:
  tls:
    cipherSuites: # Defaults to the suites of the configured minVersion
    - <cipher suite 1>
    - ...
    minVersion: <VersionTLS12|VersionTLS13> # Defaults to 1.2
```

### Topology Considerations
#### Hypershift / Hosted Control Planes
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is solely intended for MicroShift.

### Implementation Details/Notes/Constraints
As mentioned in the proposal, there is one limitation in regards to TLS 1.3
that relates to Golang implementation.

[This issue](https://github.com/golang/go/issues/29349) brings up the question
about configuring them and the release notes for Golang state:
```
TLS 1.3 cipher suites are not configurable. All supported cipher suites are
safe, and if PreferServerCipherSuites is set in Config the preference order is
based on the available hardware.
```

So all of the settings for MicroShift about cipher suites will only apply to
TLS 1.2.

Practically speaking, this means any attempt to configure `tls.cipherSuites`
when having `tls.minVersion` set to 1.3 will result in MicroShift ignoring and
overwriting the contents to match the cipher suites that Golang will use. Note
that this is only for informational purposes, no cipher suites will be passed
on to the http servers, as they are ignored.

### Risks and Mitigations
N/A

### Drawbacks
In the event of having a compromised cipher suite in TLS 1.3 an admin would not
be able to configure the API server to not use it.
Golang restricts configuration in this regard, so the fix should come from the
language libraries themselves.

## Design Details
N/A

## Open Questions
N/A

## Test Plan
All of the changes listed here will be included in the current e2e scenario
testing harness in MicroShift.

## Graduation Criteria
Targeting GA for MicroShift 4.19 release.

### Dev Preview -> Tech Preview
- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage

### Tech Preview -> GA
- More testing (upgrade, downgrade)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
N/A

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions
N/A

### Failure Modes
* If `tls.minVersion` is configured to a value other than the supported ones,
  MicroShift will fail to start and display an error message.
* If `tls.cipherSuites` includes a suite that does not belong to the configured
  `tls.minVersion`, MicroShift will fail to start and display an error message.

## Support Procedures
N/A

## Implementation History
N/A

## Alternatives
N/A
