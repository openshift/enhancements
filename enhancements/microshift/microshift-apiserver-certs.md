---
title: microshift-apiserver-certs
authors:
  - "@pacevedom"
reviewers:
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@stlaz, Security specialist"
  - "@pmtk, MicroShift contributor"
  - "@copejon, MicroShift contributor"
  - "@deads2k, OpenShift architect"
  - "@benluddy, OpenShift API server team"
approvers:
  - "@dhellmann"
api-approvers:
  - N/A
creation-date: 2023-01-20
last-updated: 2023-01-20
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-716
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# MicroShift API server certificates
## Summary
This enhancement proposes a strategy to manage kube API server certificates
in all environments where MicroShift might be deployed, both externally
(accessible through kubeconfig) and internally (pods reaching the API server).

> Anytime the document mentions API server, it refers to kube API server.

MicroShift has a wide variety of different use cases when it comes to API
server connectivity, we can find environments where the cluster needs to be
reachable with the IP, with the hostname, by some other name having DNS, or
a combination of them.

## Motivation
As of today, MicroShift does not cover all authentication requirements. It
needs to be able to authenticate API server connections using the node IP
address, the hostname, or any other domain name with DNS resolution. It
shares certificate handling with OCP, which is a bit different in terms of
deployment.

Unlike OCP, MicroShift does not have a load balancer to access API server, and
users need to be able to reach it via IP address and/or the hostname.
Therefore, the IP and/or hostname need to be present in the certificates (CN or
SAN). Because of how the [certificate lookup](#current-certificate-strategy)
works for incoming connections, there can only be one certificate associated
to an IP address. Because of MicroShift network architecture, external and
internal clients appear to use the same server IP. Connections from both types
of clients would use the same certificate to authenticate the connection.

### User Stories
* As a MicroShift administrator/user, I want server TLS authentication using
the hostname.
* As a MicroShift administrator/user, I want server TLS authentication using
a name registered in a DNS.
* As a MicroShift administrator/user, I want server TLS authentication using
its IP address.
* As a MicroShift application developer, I want server TLS authentication using
the service name, `kubernetes`.
* As a MicroShift application developer, I want server TLS authentication using
the service IP.

### Goals
* Allow server TLS authentication using any combination of IP address and
domain names.
* Keep OpenShift alignment with trust domain separation.

### Non-Goals
N/A

## Proposal
MicroShift currently follows the OCP behavior and uses several serving
certificates for internal and external communication. Among them we can
find:
* API server external
* API server localhost
* API server service network

Each of these certificates is signed by a different CA.

Proposal suggests to have different networks for external and internal access.
By using different IPs to connect to API server the ambiguity when selecting
a valid certificate for a client disappears. External connections will use the
node external IP, while a pod should use an internal IP.

This enables the use of IPs in addition to hostnames, which is part of
MicroShift requirements.

### Current certificate strategy
#### How API server builds certificates configuration
The current strategy matches the one from OpenShift Container Platform (OCP),
where there are different trust domains depending on who wants to connect to
API server. Each client will get their corresponding certificate based on
the trust domain in which they are.

A description of how the API server configures certificates follows.

When creating the API server instance there is a [default certificate](https://github.com/openshift/microshift/blob/46f188c85b1da463b0d8f932ca521cb42dec3130/pkg/controllers/kube-apiserver.go#L136)
which is the serving service network. The rest of the certificates are
configured [here](https://github.com/openshift/microshift/blob/46f188c85b1da463b0d8f932ca521cb42dec3130/pkg/controllers/kube-apiserver.go#L184-L202)
along with the TLS versions and cipher suites. These go into an upstream
library called `dynamiccertificates`. If certificates have names on them (which
they do), the package [creates](https://github.com/kubernetes/apiserver/blob/1b6c1bf2dd8fa6448944178e27f73f6e097fe80b/pkg/server/dynamiccertificates/tlsconfig.go#L209)
a 1:1 mapping of names to certificates. There is one entry per name in each
certificate, including Common Name (CN) and Subject Alternative Names (SAN).
Note there can only be one certificate per name, as otherwise there would be
ambiguity.

#### How API server selects certificates for incoming connections
Everytime there is an incoming connection to the API server dynamiccertificates
package kicks in to select the certificate it should serve. If the client sends
SNI information (which means a name has been used, instead of an IP), then TLS
configuration follows normal flow. If SNI was not used, then it will start a
lookup in the table we described above using the destination IP for the
connection. This destination IP is one where API server is listening. If there
is no match (meaning that no certificate was configured with CN/SAN to include
the API server IP), then it resolves as if it were SNI, which will end up
returning the default certificate.

#### Connecting to API server
Here we can distinguish two use cases: external access and internal access.

External access will use the kubeconfig, which contains a `server` stanza in it
with the name (DNS or IP) and port of the cluster. As seen in previous section,
if it is a name it will find a matching certificate, and if its an IP it will
try looking it up in the mapping table, returning the default certificate if not
present. Note the name or IP must be in the external certificate, as there is
hostname validation on the client.

API server access typically uses [client-go](https://github.com/kubernetes/client-go).
If using [InClusterConfig](https://github.com/kubernetes/client-go/blob/27de641f7536dc606f237f681b2a4d8dbe6e34f9/rest/config.go#L511)
then it will not use the name for connecting. As we can see it uses the IP for
the `kubernetes` service, injected through the environment variables. This IP
is the service network IP, which later on gets translated to an endpoint:
```bash
$ KUBECONFIG=/var/lib/microshift/resources/kubeadmin/kubeconfig oc get svc kubernetes -o yaml
apiVersion: v1
kind: Service
metadata:
  creationTimestamp: "2022-12-20T15:57:23Z"
  labels:
    component: apiserver
    provider: kubernetes
  name: kubernetes
  namespace: default
  resourceVersion: "199"
  uid: 28e299a6-1578-4fd0-babd-660a1a86cdfb
spec:
  clusterIP: 10.43.0.1
  clusterIPs:
  - 10.43.0.1
  internalTrafficPolicy: Cluster
  ipFamilies:
  - IPv4
  ipFamilyPolicy: SingleStack
  ports:
  - name: https
    port: 443
    protocol: TCP
    targetPort: 6443
  sessionAffinity: None
  type: ClusterIP
status:
  loadBalancer: {}

$ KUBECONFIG=/var/lib/microshift/resources/kubeadmin/kubeconfig oc get endpoints kubernetes -o yaml
apiVersion: v1
kind: Endpoints
metadata:
  creationTimestamp: "2022-12-20T15:57:23Z"
  labels:
    endpointslice.kubernetes.io/skip-mirror: "true"
  name: kubernetes
  namespace: default
  resourceVersion: "201"
  uid: 2c3d37b8-9e04-4779-9947-6230450e2336
subsets:
- addresses:
  - ip: 192.168.122.117
  ports:
  - name: https
    port: 6443
    protocol: TCP
```
As we can see the endpoint is the node IP, which means that any connection to
the service IP will get translated to the node IP. When reaching API server the
destination IP will be that of the node, and if there is no match for that in
the map the default certificate is returned. This works because the service
network certificate includes the service IP for hostname validation.

#### Including the IP in the certificates
We have already seen that internal access always uses the node IP because of
the endpoints.

If we were to include the node IP in the service network certificate we would
get an immediate match in the mapping for pods, and the hostname verification
matches the service network IP. If the connection using the node IP as
destination does not come from a pod we run into trouble. The service network
certificate is signed by its own specific CA, so any external connection using
the IP will get the internal certificate and wont validate against the external
CA.

If we were to include the node IP in the external certificate we also get an
immediate match in the mapping, and the hostname verification also works ok.
All internal connections (those coming from pods through the service network
IP) will be broken when trying to validate hostname, as the service IP is
not included. Even if it was included, validation with the internal CA would
still fail.

Including the IP in the current situation means choosing what to lose:
internal or external connectivity. Since internal connectivity is mandatory,
the only option is to use names for external connectivity. MicroShift will run
in environments where hostname (or DNS) access may not be guaranteed. Customers
have given us the requirement of accessing the cluster via IP addresses.

### Using a different IP
To remove the ambiguity when selecting a certificate based on the destination
IP, which is the one used to connect to API server, there is a need to have
more than one network.

The external network is used for external connections, such as `oc` clients.
The internal network is used for internal connections, such as the ones from
pods in the cluster.

Choosing the internal network is a complex task, as it can collide with other
IP ranges coming from DHCP or other environment configurations. In order to
minimize risks, the API server service IP, which is always the [first one](https://github.com/kubernetes/kubernetes/blob/master/pkg/controlplane/services.go#L47)
from the service CIDR range, is selected by default.

Since API server listens in [all interfaces](https://github.com/openshift/microshift/blob/0f4e4e8d7cb9946a7af81550eb6a537d0f5b4e15/pkg/controllers/kube-apiserver.go#L185)
we only need to configure this IP in any interface. To make the solution not
dependent on CNI an already existing interface may be used, like loopback.
Upon start, MicroShift API server controller will first configure the service
network IP in the loopback interface with a 32-bit netmask, then launch the API
server.

By doing so, the API server service network resolves to a local IP (which
happens to be the same IP), while the external traffic still uses the external
IP, which is the node IP. This removes the ambiguity problem and allows the
external certificate to include the node IP in the SAN. The internal
certificate remains unchanged, as it already includes the service IP, which is
now a valid IP within the node.

From a pod's perspective, it is irrelevant whether the IP is local or not. All
a pod needs is to be able to reach the IP in order to connect to the API
server. This enables the possibility of adding more nodes to the cluster, but
keeping the external IP authentication the master node needs to have more than
one external IP.

### Configuring secondary IPs
In single-node like deployments the requirement is to have only one external
IP, requiring an internal IP like the one presented in this proposal. To allow
other kinds of deployments, such as 2+ node MicroShift (needed for CNCF
certification tests), the internal IP can not be local to a single node. By
introducing a new configuration option both use cases would fit into
MicroShift.

For single node the IP needs to be internal and it was already described how
to use the service CIDR to extract the API server IP. No additional
configuration is needed.

For more than one node the internal IPs are not enough, in this case there can
be any number of external IPs. Extra care should be taken when selecting which
ones belong to the external clients and which ones belong to the internal
clients, if we want to keep IP addresses in the certificates. The configuration
option shall be used to force an API server IP instead of the service network
default.

If the configuration option is used to force a non-default IP, loopback
interface would remain unchanged and no additional IPs will be added to it.

Options to have this configuration done include:
1. Manually configure IPs before MicroShift starts.
   While this may be a pre-requisite, the dynamic nature of IPs makes it
   unusable. IP addresses may change at any time, making it impractical to
   expect or require the interfaces to be manually/statically configured
   before starting. If in single node, it also needs to read MicroShift
   configuration to extract the service network CIDR.
1. Have a systemd unit do the configuration.
   This is the automatic way of doing the previous one. Introducing startup
   order, it also needs to read MicroShift configuration and react upon IP
   changes.
1. Have MicroShift configure the secondary IP.
   While MicroShift does not own the OS, all the available information to
   properly configure the interface address (if needed) lives here. It needs
   the service network CIDR, the API server IP, and the new configuration
   option.

### Workflow Description
> _Disclaimer:_ Workflows described here illustrate automatic processes. This
section is only informational about what happens when starting MicroShift.

**Start MicroShift with default configuration**
1. MicroShift service is assumed to be enabled.
1. When starting, MicroShift will read the service CIDR from the configuration. If not set, default to `10.43.0.0/16`.
1. Compute API service IP from the service CIDR provided. This will be the first IP from the range.
1. MicroShift API server controller will look for loopback interface assigned IPs.
1. If the API service IP is not assigned to the interface, do it with a `/32` netmask.
1. Keep startup process.
1. API server creates the endpointslices entry linked to the `kubernetes` service:
   ```shell
   $ KUBECONFIG=/var/lib/microshift/resources/kubeadmin/kubeconfig oc get endpointslices
   NAME         ADDRESSTYPE   PORTS   ENDPOINTS   AGE
   kubernetes   IPv4          6443    10.43.0.1   2d17h
   ```
1. Connecting to API server using external (node) IP yields external certificate.
   ```shell
   $ openssl s_client -connect 192.168.122.117:6443 -showcerts | openssl x509 -text -noout -in - | grep -C 1 "Alternative\|CN"
   Can't use SSL_get_servername
   depth=1 CN = kube-apiserver-external-signer
   verify error:num=19:self signed certificate in certificate chain
   verify return:1
   depth=1 CN = kube-apiserver-external-signer
   verify return:1
   depth=0 CN = 192.168.122.117
   verify return:1
        Signature Algorithm: sha256WithRSAEncryption
        Issuer: CN = kube-apiserver-external-signer
        Validity
   --
            Not After : Jan 27 16:46:37 2024 GMT
        Subject: CN = 192.168.122.117
        Subject Public Key Info:
   --
            X509v3 Subject Alternative Name:
                DNS:api.example.com, DNS:microshift-1, DNS:192.168.122.117, IP Address:192.168.122.117
   ```
1. Connecting to API server using internal IP yields internal certificate.
   ```shell
   $ openssl s_client -connect 10.43.0.1:6443 -showcerts | openssl x509 -text -noout -in - | grep -C 1 "Alternative\|CN"
   Can't use SSL_get_servername
   depth=1 CN = kube-apiserver-service-network-signer
   verify error:num=19:self signed certificate in certificate chain
   verify return:1
   depth=1 CN = kube-apiserver-service-network-signer
   verify return:1
   depth=0 CN = 10.43.0.1
   verify return:1
        Signature Algorithm: sha256WithRSAEncryption
        Issuer: CN = kube-apiserver-service-network-signer
        Validity
   --
            Not After : Jan 27 16:46:38 2024 GMT
        Subject: CN = 10.43.0.1
        Subject Public Key Info:
   --
            X509v3 Subject Alternative Name:
                DNS:api-int.example.com, DNS:api.example.com, DNS:kubernetes, DNS:kubernetes.default, DNS:kubernetes.default.svc, DNS:kubernetes.default.svc.cluster.local, DNS:openshift, DNS:openshift.default, DNS:openshift.default.svc, DNS:openshift.default.svc.cluster.local, DNS:10.43.0.1, IP Address:10.43.0.1
   ```

**Stopping MicroShift**
1. MicroShift service is stopped.
1. All running controllers start teardown process.
1. API server controller lists the IPs from the loopback interface looking for the API server service IP.
1. Remove the IP from the interface and keep shutting down all services.


### API Extensions
N/A

### Risks and Mitigations
* Internal IP change.
   In the event of having the internal IP changed, pods will lose connectivity
   towards API server for some time, until connections are renewed. There is
   a short span of time where the new and the old IPs are endpoints for the
   service IP. Pods should be able to recover by themselves by retrying or
   dying and waiting to be restarted.

* IP collisions.
   Choosing the correct internal IP can be challenging in certain types of
   deployments. If MicroShift is deployed in a way that its IP gets renewed,
   there might be network conditions where there are collisions. Special care
   must be taken to select an IP so as to avoid them. This also applies to the
   external IPs for MicroShift, so it is not something local to this proposal.

### Drawbacks
See risks above.

## Design Details
N/A

### Open Questions [optional]
N/A

### Test Plan
New e2e tests should be introduced to change:
* Hostname.
* External IP.
* SAN.

And verify connectivity and authentication is still working from any client,
external or internal.

### Graduation Criteria
#### Dev Preview -> Tech Preview
- Gather feedback from early adopters on possible networking issues.

#### Tech Preview -> GA
- Extensive testing on hostname/IP/SAN changes.

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy
N/A

### Version Skew Strategy
N/A

### Operational Aspects of API Extensions
N/A

#### Failure Modes
N/A

#### Support Procedures
N/A

## Implementation History
N/A

## Alternatives
* Merge certificates.
   If trust domains are merged into a single one then there will only be a
   single certificate for all possible connections. This means the certificate
   includes both the internal and external names, plus the internal and
   external IPs. It also means the certificate is signed by the same CA, for
   all clients. By doing so the strategy diverges from the one used in
   OpenShift.

* Have a single CA for all API server certificates.
   Signing all certificates with the same CA is not different than having a
   single certiticate. It would still need the node IP in the certificates,
   and we have already seen that this can only be done in the internal one,
   meaning it is also needed to add hostnames, rendering a single certificate.
   This approach diverges more significantly from the OCP approach.

* Use localhost for internal connectivity. 
   Using localhost as the node IP would allow internal pods use a different IP
   than what the external connectivity needs. This would conflict with the
   localhost certificate, which would require merging. It also invalidates
   having more than one node. As per the kubernetes docs, it is forbidden to
   use localhost IPs as endpoints for a service.
   [Reference](https://kubernetes.io/docs/concepts/services-networking/service/#custom-endpointslices)

## Infrastructure Needed [optional]
N/A
