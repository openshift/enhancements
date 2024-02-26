---
title: microshift-apiserver-custom-certs
authors:
  - "@eslutsky"
reviewers:
  - "@jerpeter, MicroShift contributor"
  - "@pacevedom, MicroShift contributor"
  - "@ggiguash, MicroShift contributor"
  - "@benluddy, OpenShift API server team"
  - "@standa, OpenShift Auth Team"

approvers:
  - "@jerpeter"
api-approvers:
  - N/A
creation-date: 2021-01-18
last-updated: 2023-01-18
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-2101
see-also:
  - microshift-apiserver-certs.md
replaces:
  - N/A
superseded-by:
  - N/A
---

# MicroShift API server custom external certificates
## Summary

This enhancement extends the Microshift apiserver Certs to allow the user to
provide additional custom certificates (e.g. signed by a 3rd party CA) for API server SSL handkshake and external authentication.


> Anytime the document mentions API server, it refers to kube API server.


## Motivation
Currently, users of Microshift cannot provide their own generated certificates,
Some customers have very strict requirements regarding TLS certs. There are frequently intermediate proxies which allow only TLS traffic with certs that are signed by organization owned CAs.

### User Stories
* As a MicroShift administrator, I want to be able to add  organization generated certificates. 
  - each certificate may contain:
    - Single Common Name containing The API server DNS/IPAddress or a wildcard entry (wildcard certificate).
    - Multiple Subject Alternative Names (SAN) containing the API server DNS/IPAddress.

* As a MicroShift administrator, I want to provide additional DNS names for each certificate.

### Goals
* Allow MicroShift administrator to configure Microshift with externally generated certificates and
domain names.

### Non-Goals
Automatic certificate rotation and integration with 3rd party cert issuing services.

## Proposal
Proposal suggests to provide Administrator means adding their own Certificates using microshift configuration file.

A new `apiServer` level section will be added to the configuration called `namedCertificates`:

```yaml
apiServer:
  namedCertificates:
  - certPath: ~/certs/api_fqdn_1.crt
    keyPath:  ~/certs/api_fqdn_1.key
  - certPath: ~/certs/api_fqdn_2.crt
    keyPath:  ~/certs/api_fqdn_2.key
    names:
    - api_fqdn_1     
    - *.apps.external.com

```  
For each provided certificate, the following configuration is proposed:
1. `names` - optional list of explicit DNS names (leading wildcards allowed).
   If no names are provided, the implicit names will be extracted from the certificates.
1. `certPath` -  certificate full path.
1. `keyPath` -   certificate key full path.

Certificate files will be read from their configured location by the Microshift service,
each certification file will be validated (see validation rules).

because we dont want to disrupt the internal API communication and make sure the internal certificate will be automaticly renewed,
Those certificate will extend the default  API server [external](https://github.com/openshift/microshift/blob/main/pkg/controllers/kube-apiserver.go#L194) certificate configuration.


### Kubeconfig Generation
Each configured certificate can contain multiple FQDN and wildcards values, for each unique FQDN address kubeconfig file will be generated on the filesystem.

Every generated `kubeconfig` for the custom certificates will omit the certificate-authority-data section,
therefore custom certificates will have to be validated against CAs in the RHEL Client trust store. 

each certificate will be examined and deteremined if its wildcard only.
certificate will be considered as a wildcard only if there are no FQDN Entries found.
the FQDN Address is searched in:
- Certificate CN
- Certificate Subject Alternative Name (SAN)
- names configuration value.

when a Certificate found as a wildcard only , host public IP Address will be placed inside its kubeconfig's `server` section.

Certain values that configured in `names` field can cause  internal API communication issues  ie:127.0.0.1,localhost therefore we must not allow them.

when the `names` field is not contained in the certificate (ie: included in the wildcard),

it will be served by the api-server:
```
> echo Q |   openssl s_client -connect example.com:6443 -showcerts 2>/dev/null |   openssl x509 -noout -subject -issuer -enddate  -ext subjectAltName

subject=C = il, ST = il, L = , O = , OU = , CN = 192.168.2.130
issuer=CN = 192.168.2.130
notAfter=May 27 14:45:13 2051 GMT
X509v3 Subject Alternative Name: 
    DNS:api.example.com, DNS:localhost.localdomain, DNS:192.168.2.130, IP Address:192.168.2.130
```

while kubeconfig on the client contains:
```
> cat kubeconfig | grep server

server: https://example.com:6443
```
but rejected by the oc client:
```
> oc get pods

tls: failed to verify certificate: x509: certificate is valid for api.example.com, localhost.localdomain, not test.com
```


### Workflow Description

#### Default API Certs
1. By default, when there is no namedCertificates configuration the behaviour will remain the same.

#### when custom namedCertificates configured
1. Device Administrator copies the certificated to MicroShift host
1. Device Administrator configures additonal CAs in the RHEL Client trust store on the client system.
1. Device Administrator configures `namedCertificates` in the Microshift configuration yaml file (/etc/microshift/config.yaml).
1. Device Administrator start/restarts MicroShift
1. During startup, Microshift will check and validate the certificates paths. in case of an error service will produce clear error log and if the file is missing it will exit.
1. Microshift passes the certificates to the tls-sni-cert-key as apiserver command line option preceding all the other certificates.
1. kube-apiserver picks up the certificates and start serving them on the configured SNI.

### Topology Considerations
#### Hypershift / Hosted Control Planes
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is solely intended for MicroShift.

### Implementation Details/Notes/Constraints
The certs will be prepended to the []configv1.NamedCertificate list before the api server is started. (it will be added to `-tls-sni-cert-key` flag)

this certification paths configuration and names will be prepended into the kube-apiserver `tls-sni-cert-key` command line flag.
when same SNI appear in the CN part of the provided certs,this certificate will take precedence over the `default` external-signer. [ref](https://github.com/kubernetes/kubernetes/blob/98358b8ce11b0c1878ae7aa1482668cb7a0b0e23/staging/src/k8s.io/apiserver/pkg/server/dynamiccertificates/named_certificates.go#L38)


### API Extensions
N/A

### Risks and Mitigations
* User provide certificate and it expired after some time.
  kube-api server will continue serving with an expired cert - similiar approach is taken by OpenShift.
  > users can mitigate this by using --insecure-skip-tls-verify client mode

* User provided expired Certificate and Microshift service started/restarted 
  Microshift will start with a warning in the logs,kube-api will continue serve with an expired cert - similiar approach is taken by OpenShift.
  > users can mitigate this by using --insecure-skip-tls-verify client mode

* Users might configure certs with the wrong names that doesnt match the certificate SAN,but openShift allows it so we will too.
### Drawbacks
* External certificate wont be automaticly renewed, so it requires manual Certificate rotation. 
  similiar approach is taken by OpenShift.


## Design Details
N/A

## Open Questions [optional]
N/A

## Test Plan
add e2e test that will:
- generate and sign certificates with custom ca.
- change the default Microshift configuration to use the newly generated certs.
- make sure system is functional using  generated external kubeconfig.
- intentionally configure invalid value in `names` (ie: 127.0.0.1,localhost) , Microshift should reject this configuration and exit with an error message.

## Graduation Criteria
### Dev Preview -> Tech Preview
- Gather feedback from early adopters on possible issues.

### Tech Preview -> GA
- Extensive testing with various certificates variations.
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
The provided certs value will be validated before is it passed to the api-server flag

This check will prevent Microshift service from starting:
1. certificates files exists in the disk and readable by microshift process.
1. certificates shouldnt override the internal local certificates which are managed internally.

This check display warning message at the log and service will be started:
1. certificates is expired.


## Support Procedures
Configured certs values to the tls-sni-cert-key TLS handshake command line flag which is passed to the kube-apiserver:

```shell
> journalctl -u microshift -b0 | grep tls-sni-cert-key

Jan 24 14:53:00 localhost.localdomain microshift[45313]: kube-apiserver I0124 14:53:00.649099   45313 flags.go:64] FLAG: --tls-sni-cert-key="[/home/eslutsky/dev/certs/server.crt,/home/eslutsky/dev/certs/server.key;/var/lib/microshift/certs/kube-apiserver-external-signer/kube-external-serving/server.crt,/var/lib/microshift/certs/kube-apiserver-external-signer/kube-external-serving/server.key;/var/lib/microshift/certs/kube-apiserver-localhost-signer/kube-apiserver-localhost-serving/server.crt,/var/lib/microshift/certs/kube-apiserver-localhost-signer/kube-apiserver-localhost-serving/server.key;/var/lib/microshift/certs/kube-apiserver-service-network-signer/kube-apiserver-service-network-serving/server.crt,/var/lib/microshift/certs/kube-apiserver-service-network-signer/kube-apiserver-service-network-serving/server.key
```

Connecting to API server using external SNI yields external certificate.
   ```shell
   $ openssl s_client -connect <SNI_ADDRESS>:6443 -showcerts | openssl x509 -text -noout -in - | grep -C 1 "Alternative\|CN"
   ```

## Implementation History
N/A

## Alternatives
- Use [cert-manager](https://cert-manager.io/docs/) for managing Microshift external custom certs
 which allow MicroShift administrators to rotate certificates automatically (e.g. via ACME)
 cert-manager is not supported on Microshift because it requires alot of internal API changes.

## Infrastructure Needed [optional]
N/A