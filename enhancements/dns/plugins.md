---
title: configurable-dns-plugins
authors:
  - "@dhansen"
reviewers:
  - "@ironcladlou"
  - "@Miciah"
  - "@frobware"
approvers:
  - "@knobunc"
  - "@ironcladlou"
creation-date: 2019-09-27
last-updated: 2019-10-10
status: implementable
see-also: 
replaces:
superseded-by:
---

# Configurable DNS Plugins

This proposal provides cluster admins the ability to configure CoreDNS [plugins](https://coredns.io/plugins/) and
includes [forward](https://coredns.io/plugins/forward/) as the first plugin implementation.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

1. How are API changes handled for cluster downgrades?
2. Should the number of `Servers` be restricted due to Gap 4?

## Summary

DNS name resolution for services and pods is currently provided by an instance of [CoreDNS](https://coredns.io) that
runs on each node in the cluster. The [cluster-dns-operator](https://github.com/openshift/cluster-dns-operator) manages
the CoreDNS configuration with a statically-defined ConfigMap:

```bash
$ oc get configmap/dns-default -n openshift-dns -o yaml
apiVersion: v1
data:
  Corefile: |
    .:5353 {
        errors
        health
        kubernetes cluster.local in-addr.arpa ip6.arpa {
            pods insecure
            upstream
            fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf
        cache 30
        reload
    }
kind: ConfigMap
metadata:
  labels:
    dns.operator.openshift.io/owning-dns: default
  name: dns-default
  namespace: openshift-dns
  <SNIP>
```

Once CoreDNS starts and has parsed the configuration, it runs one or more servers. Each server is defined by the zones
it serves and a listening port. In the above configuration, CoreDNS starts one server that manages all zones and listens
on port 5353. Each server has its own plugin chain represented within the server block stanza (i.e. forward). This
proposal describes an API for (a) configuring additional zones for specific subdomains and (b) specifying custom
upstream nameservers that will be used to resolve names beneath these subdomains.

## Motivation

CoreDNS is responsible for resolving pod and service names for the cluster domain (i.e. `cluster.local`). Otherwise,
CoreDNS proxies the request to a resolver identified by `/etc/resolv.conf` on the corresponding node. Although this
provides a consistent and reliable approach for name resolution, it restricts how cluster operators manage DNS name
queries.

### Goals

1. A well defined API for managing CoreDNS plugins.
2. The ability to configure the CoreDNS forwarding plugin.
2. A minimal API surface that can be expanded to support future DNS forwarding and plugin use cases.

### Non-Goals

1. Support every possible DNS forwarding use case.
2. Configure or manage external DNS providers.
3. Provide name query forwarding for other cluster services (i.e. container runtime).
4. Manage what plugins get loaded (i.e. plugin.cfg).
5. Specifications for any CoreDNS plugins other than forward.

## Proposal

`Server` defines the schema for a server that runs per instance of CoreDNS. The `Name` field is required and specifies a
unique name for the `Server`. `Zones` is required and specifies the subdomains the server is authoritative for.

`Server` may contain several CoreDNS plugins in the future. This proposal includes `ForwardPlugin` as the first plugin
implementation. `ForwardPlugin` provides options for configuring the forward plugin configuration. A `Server` listens on
port `5353`, which can not be configured at this time. If no `Server` is specified, the cluster-dns-operator will
generate the ConfigMap referenced in the [Summary](#summary) section.

```go
type Server struct {
    Name string `json:"name"`
    Zones []string `json:"zones"`
    ForwardPlugin ForwardPlugin `json:"forwardPlugin"`
    // additional future plugins
}
```

`ForwardPlugin` defines a schema for configuring the CoreDNS forward plugin. `Upstreams` is a list of resolvers to
forward name queries for subdomains specified by `Zones`. `Upstreams` are randomized when more than 1 upstream is
specified. Each instance of CoreDNS performs health checking of `Upstreams`. When a healthy upstream returns an error
during the exchange, another resolver is tried from `Upstreams`. Each upstream is represented by an IP address or IP
address and port if the upstream listens on a port other than 53.
```go
type ForwardPlugin struct {
    Upstreams []string `json:"upstreams"`
}
```

`Servers` is added to `DNSSpec` since multiple servers can run per CoreDNS instance.
```go
type DNSSpec struct {
    Servers []Server `json:"servers,omitempty"`
}
```

### User Stories

#### Story 1

As a customer with OpenShift running in AWS and connected to my data center by VPN, I want OpenShift DNS to resolve
name queries for our other internal devices using the DNS servers in our data center.

### Implementation Details/Notes/Constraints

If `Servers` consists of more than one `Server`, longest suffix match will be used to determine the `Server`. For
example, if there are two `Servers`, one for "foo.com" and another for "a.foo.com", and the name query is for
"www.a.foo.com", it will be routed to the `Server` with `Zone` "a.foo.com".

A maximum of 15 `Upstreams` is allowed per `ForwardPlugin`.

`Name` must comply with the [rfc6335](https://tools.ietf.org/rfc/rfc6335.txt) Service Name Syntax.

`Zones` must conform to the definition of a subdomain in [rfc1123](https://tools.ietf.org/html/rfc1123). The cluster
domain (i.e. cluster.local) is an invalid subdomain for `Zones`.

The cluster-dns-operator will prepend the configuration of `Servers` to `configmap/dns-default`, instead of the entire
ConfigMap being statically defined. To achieve this:

1. The [default](https://github.com/openshift/cluster-dns-operator/blob/master/assets/dns/configmap.yaml) ConfigMap
asset and associated code from the [manifests pkg](https://github.com/openshift/cluster-dns-operator/blob/master//pkg/manifests/manifests.go#L70:6)
should be removed.

2. The [desiredDNSConfigMap](https://github.com/openshift/cluster-dns-operator/blob/master/pkg/operator/controller/controller_dns_configmap.go)
function must be modified to create a ConfigMap with additional server configuration blocks based on `Server`. If
`Server` is undefined or an invalid, the ConfigMap will only contain the default server. Otherwise,
`desiredDNSConfigMap` will use the provided `Servers` to construct additional server configuration blocks, each with a
forwarding configuration and use `/etc/resolv.conf` as a resolver of last resort. For example:

```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  servers:
  - name: foo-server
    zones:
      - foo.com
    forwardPlugin:
      upstreams:
        - 1.1.1.1
        - 2.2.2.2:5353
  - name: bar-server
    zones:
      - bar.com
      - example.com
    forwardPlugin:
      upstreams:
        - 3.3.3.3
        - 4.4.4.4:5454
```

The above `DNS` will produce the following `ConfigMap`:
```yaml
apiVersion: v1
data:
  Corefile: |
    foo.com:5353 {
        forward . 1.1.1.1 2.2.2.2:5353
    }
    bar.com:5353 example.com:5353 {
        forward . 3.3.3.3 4.4.4.4:5454
    }
    .:5353 {
        errors
        health
        kubernetes cluster.local in-addr.arpa ip6.arpa {
            pods insecure
            upstream
            fallthrough in-addr.arpa ip6.arpa
        }
        prometheus :9153
        forward . /etc/resolv.conf {
            policy sequential
        }
        cache 30
        reload
    }
kind: ConfigMap
metadata:
  labels:
    dns.operator.openshift.io/owning-dns: default
  name: dns-default
  namespace: openshift-dns
```
#### Updating
Changes to `ForwardPlugin` will trigger a [rolling update](https://kubernetes.io/docs/tasks/manage-daemon/update-daemon-set/#performing-a-rolling-update)
of the CoreDNS DaemonSet.

#### Validation
`Domain` must conform to the [RFC 1123](https://tools.ietf.org/html/rfc1123#page-13) definition of a subdomain. Each
upstream in `Upstreams` must be a valid IPv4 address. If the upstream listens on a port other than 53,
a valid port number must be specified. A colon is used to separate the address and port, `IP:port` for IPv4.

#### Transport
UDP is used to transport DNS messages and `ForwardPlugin` health checks. Any UDP transport will automatically retry with
the equivalent TCP transport if the response is truncated (TC flag set in response).

#### Health Checking Details

Nameserver health checking is performed in-band. A health check is performed only when CoreDNS detects an error.
The check runs in a loop, every 0.5s, for as long as the forwarder reports unhealthy. Once healthy, CoreDNS stops
health checking until the next error. The health checks use a recursive DNS query (. IN NS) to get forwarder health.
Any response that is not a network error (REFUSED, NOTIMPL, SERVFAIL, etc) is taken as a healthy forwarder.
When all `Upstreams` are down CoreDNS assumes health checking as a mechanism has failed and will try to connect to a
random upstream (which may or may not work).

### Risks and Mitigations

#### Gap 1
Forwarded name queries and upstream health checks are insecure. This may allow a malicious actor to impersonate an
`ForwardPlugin` upstream.

#### Mitigation 1
Add TLS support to secure forwarded name queries. Clearly document this insecurity in product documentation in the
meantime.

#### Gap 2

No mechanism for customizing DNS forwarding at install time.

#### Mitigation 2

Use the OpenShift Enhancement process to solicit feedback from the installer team.

#### Gap 3

Surfacing status for upstreams that fail health checks.

#### Mitigation 3

`coredns_forward_healthcheck_failure_count_total{to}` and `coredns_forward_healthcheck_broken_count_total{}` Prometheus
metrics are exported. Ensure these metrics are surfaced through the OpenShift monitoring stack.

#### Gap 4

An increased utilization of compute resources on each node due to the potential of running many `Servers`, each with up
to 15 upstreams being actively checked for health.

#### Mitigation 4

Test utilization using multiple `Servers` with multiple `Upstreams`. Include a warning in the documentation that states
"adding a large number of forwarders may incur a performance penalty or hit memory limits". It's also possible for the
operator to increase the amount of resources (i.e. memory) requested based on the number of `Servers`.

## Design Details

### Test Plan

Implement the following end-to-end test in addition to unit tests:

- Create a `DNS` with `Servers` that uses an `Upstream` to resolve a hostname from `Zones`.
- Start the dns-operator and check the logs for healthcheck failures for the `Upstream`.
- Create a pod that performs an nslookup for a hostname in the cluster domain.
- Have the pod perform an nslookup for a hostname in `Zones`.
- If the nslookup succeeds, check if the nslookup server matches the `Upstream`.
- Have the pod perform an nslookup for a hostname in `/etc/resolve.conf`.

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

Upgrades will continue to use the existing `forward . /etc/resolv.conf` forwarding configuration. Edit
 `dnses.operator.openshift.io` to modify the default forwarding behavior by adding `Servers`. For example:
 
```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  servers:
  - name: foo-server
    zones:
      - foo.com
    forwardPlugin:
      upstreams:
        - 1.1.1.1
        - 2.2.2.2:5353
```

### Version Skew Strategy

TODO

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

1. Additional resource utilization (i.e. memory) on each node required for the forward plugin, health checks, etc..

## Alternatives

Possible alternatives are listed [here](https://docs.google.com/document/d/17OEwYs9HuCeGtFKfJLwtk809C6MIjZmguce5-LQYxIM/edit?usp=sharing).

## Infrastructure Needed

TODO
