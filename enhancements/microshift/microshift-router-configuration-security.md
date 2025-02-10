---
title: microshift-router-configuration-security
authors:
  - "@eslutsky"
reviewers:
  - "@pacevedom"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@pliurh"
  - "@Miciah"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2024-09-23
last-updated: 2024-10-07
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-4091
---

# MicroShift router security configuration options

## Summary
MicroShift's default router is created as part of the platform, but does not
allow configuring any of its specific parameters. For example, you cannot
specify the custom Certificate for the ingress router, or custom TLS Ciphers Security profile.

In order to allow these operations and many more, a set of configuration options
is proposed.

## Motivation

Microshift Customers need a way to override the default Ingress Controller security configuration  similar as OpenShift does.


### User Stories

#### adding custom SSL Certificates

User wants to  configure organization trusted SSL certificate that will be served to the application users.

#### rotating/renewing custom SSL Certificates (day2)
User wants to  rotate/renew organization trusted SSL certificate that will be served to the application users (day 2).

#### enabling client certificates
User want to  enable client TLS  and client certificate policy (mTLS) .

### Goals
Allow users to configure the additional HAProxy/Router security customization parameters, see Proposal table for details.


### Non-Goals
N/A

## Proposal

Microshift don't use ingress [operator](https://github.com/openshift/cluster-ingress-operator) , all the customization performed through  configuration file.
the configuration will propagate to the router deployment [manifest](https://github.com/openshift/microshift/blob/aea40ae1ee66dc697996c309268be1939b018f56/assets/components/openshift-router/deployment.yaml) Environment variables.

see the table below for the proposed configuration changes:

| new configuration                               | description                                                                                                                                                                                                                                                                                                                                                                                                                                                          | default                       |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------- |
| ingress.certificateSecret                       | name of the secret in the openshift-ingress namespace that will be contain the ingress certificate. <br>                                                                                                                                                                                                                                                                                                                                                             | router-certs-default          |
| ingress.tlsSecurityProfile.type                 | type is one of Old, Intermediate, Modern or Custom. Custom provides the ability to specify individual TLS security profile parameters<br><br>see more [info](https://docs.openshift.com/container-platform/4.17/security/tls-security-profiles.html#tls-profiles-understanding_tls-security-profiles)<br>                                                                                                                                                            | Intermediate                  |
| ingress.tlsSecurityProfile.Custom.ciphers       | lists the allowed cipher suites that the Will be accepted and served when tlsSecurityProfile.type is set to  custom<br>                                                                                                                                                                                                                                                                                                                                              | Default for the selected type |
| ingress.tlsSecurityProfile.Custom.minTLSVersion | only when tlsSecurityProfile.type is set to  custom<br>MinVersion specifies which TLS version is the minimum version of TLS.<br>Allowed values: VersionTLS12, VersionTLS13.                                                                                                                                                                                                                                                                                          | Default for the selected type |
| ingress.clientTLS.allowedSubjectPatterns        | allowedSubjectPatterns specifies a list of regular expressions that should be matched against the distinguished name on a valid client certificate to filter requests. The regular expressions must use PCRE syntax. If this list is empty, no filtering is performed. If the list is nonempty, then at least one pattern must match a client certificate’s distinguished name or else the ingress controller rejects the certificate and denies the connection.<br> | None                          |
| ingress.clientTLS.clientCA                      | clientCA specifies a configmap containing the PEM-encoded CA certificate bundle that should be used to verify a client’s certificate. The administrator must create this configmap in the openshift-config namespace.                                                                                                                                                                                                                                                | None                          |
| ingress.clientTLS.clientCertificatePolicy       | clientCertificatePolicy specifies whether the ingress controller requires clients to provide certificates. This field accepts the values "Required" or "Optional".                                                                                                                                                                                                                                                                                                   | Optional                      |
| ingress.routeAdmission.wildcardPolicy           | WildcardPolicyAllowed indicates routes with any wildcard policy are admitted by the ingress controller.                                                                                                                                                                                                                                                                                                                                                              | WildcardsDisallowed           |
|                                                 |                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |                               |

for clientTLS implementation details see the original [enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/client-tls.md#implementing-basic-support-for-client-certificate-verification).

see full [ocp](https://docs.openshift.com/container-platform/4.17/networking/ingress-operator.html) configuration reference.


### Workflow Description
***configuring custom certificates for ingress***
**cluster admin** is a human user responsible for configuring a MicroShift
cluster.

1. Cluster admin  creates  a kustomized  yaml that contains the wildcard certificate chain and key under selected name.
2. Cluster admin adds  `ingress.certificateSecret` configuration for the router providing the new secret name. 
3. Cluster admin start or restart the Microshift service.
4. After MicroShift started, the system will ingest the configuration and setup
   everything according to it.
   
***configuring other Security options***

1. The cluster admin adds specific configuration for the router prior to
   MicroShift's start.
2. After MicroShift started, the system will ingest the configuration and setup
   everything according to it.

### API Extensions
As described in the proposal, there is an entire new section in the configuration:
```yaml
ingress:
    certificateSecret: router-certs-custom
    tlsSecurityProfile: 
        type: Custom
        custom:
            ciphers:
            - ECDHE-ECDSA-CHACHA20-POLY1305
            - ECDHE-RSA-CHACHA20-POLY1305
            - ECDHE-RSA-AES128-GCM-SHA256
            - ECDHE-ECDSA-AES128-GCM-SHA256
            minTLSVersion: VersionTLS12
    clientTLS:
        allowedSubjectPatterns: ^/CN=example.com/ST=NC/C=US/O=Security/OU=OpenShift$
        clientCA: ca-config-map
        clientCertificatePolicy: Required
    routeAdmission:
        wildcardPolicy: WildcardPolicyAllowed
    
```

For more information check each individual section.

#### Hypershift / Hosted Control Planes
N/A
### Topology Considerations
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is solely intended for MicroShift.

### Implementation Details/Notes/Constraints
The default router is composed of a bunch of assets that are embedded as part
of the MicroShift binary. These assets come from the rebase, copied from the
original router in [cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator).

####  Enabling  wildcard routes
The default behavior of the Ingress Controller is to admit routes with a wildcard policy of `None`, the setting value will configure to the  environment variable ROUTER_ALLOW_WILDCARD_ROUTES in the ingress  deployment.

#### enabling clientTLS/mTLS 
If the user does not specify `ingress.clientTLS`, then client TLS is not enabled,
which means that the Microshift Routes does not request client certificates on
TLS connections.  If `ingress.clientTLS` is specified, then the Microshift Routes (Edge/reencrypt)
does request client certificates, and `ingress.clientTLS.clientCertificatePolicy`
must be specified to indicate whether the IngressController should reject
clients that do not provide valid certificates.

The required `ClientCA` field specifies a reference to a ConfigMap in the same namespace as the router deployment, containing a CA certificate bundle.

Additionally, the optional `AllowedSubjectPatterns` field can be used to specify a list of patterns. If this field is defined, the IngressController will reject any client certificate that does not match at least one of the specified patterns.

Finally, the optional `AllowedSubjectPatterns` field may be used to specify a
list of patterns.  If the field is specified, then the IngressController rejects
any client certificate that does not match at least one of the provided
patterns.

The user must create the ConfigMap containing the CA bundle in the `openshift-ingress` namespace (`ingress.ClientCA`). This ConfigMap is mounted to the router deployment as a read-only volume. MicroShift provides the `ROUTER_MUTUAL_TLS_AUTH_CA` environment variable, which contains the full path to the CA certificate file.

Once the OpenShift router starts, the following actions occur (on the router):

- The certificates specified in `ROUTER_MUTUAL_TLS_AUTH_CA` are parsed.
- CRLs (Certificate Revocation Lists) are downloaded from any CRL distribution points found and written to a file, which is then supplied to HAProxy for mTLS certificate validation.
- Client certificates are validated against the CRL.
- Requests with invalid client certificates, including revoked certificates, are rejected.

Certificate revocation list (CRL) handling was introduced in the router starting from version [4.14](https://github.com/openshift/router/pull/472).
#### Day 2 Replacing/renewing certificates
applying the new secret using  kustomize yaml  will be enough for renewing certificate on a running system with minimal downtime
- using built-in k8s functionality the new secret will be transparently  mounted into the router container
	 > *When a volume contains data from a Secret, and that Secret is updated, Kubernetes tracks this and updates the data in the volume, using an eventually-consistent approach*.  ([source](https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets-as-files-from-a-pod)   )
- openshift router will detect the updated certificate  and gracefully reload the haproxy ([source](https://github.com/openshift/router/blob/master/pkg/router/template/router.go#L453-L454))

#### Customizing TLSSecurity - Ciphers
MicroShift currently generates an Ingress deployment with TLS ciphers that support both TLS 1.2 and TLS 1.3, following [**Intermediate**](https://wiki.mozilla.org/Security/Server_Side_TLS#Intermediate_compatibility_.28recommended.29) compatibility guidelines.

Similar to OpenShift Ingress Operators, we want to allow users to select their own TLS Security Profile, offering the following options: **Old, Intermediate, Modern, and Custom**.

The Ingress deployment is controlled by three environment variables:

- **ROUTER_CIPHER** – Specifies ciphers compatible with TLS versions earlier than 1.3.
- **ROUTER_CIPHERSUITES** – Defines ciphers used exclusively for TLS 1.3 sockets.
- **SSL_MIN_VERSION** - Minimum allowed SSL version.

If a user selects the **Modern** TLS profile, TLS 1.2 ciphers will be disabled, and only TLS 1.3 will be active with its recommended ciphers, leaving `ROUTER_CIPHERS` empty.
For the **Custom** TLS profile, users must provide a complete list of supported ciphers. MicroShift will determine whether each cipher should apply to TLS 1.3 or earlier versions.


#### How config options change manifests
Each of the configuration options described above has a direct effect on the
manifests that MicroShift will apply after starting.  
see the full Implementation details in the [router-configuration](microshift-router-configuration.md) enhancement.

see the [table](#proposal) in the proposal above for all the new configuration options.



### Risks and Mitigations
* User provide certificate and it expired after some time.
  ingress will continue serving with an expired cert - similiar approach is taken by OpenShift.

- Incorrect ingress configuration can cause network  disruption and unpredictable behavior , the documentation should warn about dangers of changing the defaults. 

### Drawbacks

- External certificate wont be automaticly renewed, so it requires manual Certificate rotation.

## Open Questions
N/A

## Test Plan
all of of the configuration changes listed here will be included in the current e2e scenario
testing harness in Microshift, verifying that its applied to the ingress deployment pods.
testing ingress functionality is out of scope .

## Graduation Criteria
Not applicable

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

When upgrading from 4.17 or earlier to 4.18, the new configuration fields will remain
unset, causing the existing defaults to be used.

When downgrading from 4.18 to 4.17 or earlier, the  self-generated certificate values will
be user.


## Version Skew Strategy
N/A

## Operational Aspects of API Extensions

### Failure Modes
N/A

## Support Procedures
### showing which certificate serving  the users

- from Microshift host:
```bash
echo Q |   openssl s_client -connect 10.44.0.1:443 -showcerts  2>/dev/null |  openssl x509 -noout -subject -issuer -dates -enddate -ext subjectAltName
```
- inside the running router pod shell:
```bash
echo "show ssl cert /var/lib/haproxy/router/certs/default.pem" | socat stdio /var/lib/haproxy/run/haproxy.sock

```
## Implementation History
Implementation [PR](https://github.com/openshift/microshift/pull/4474/) for Micorshift
## Alternatives
N/A