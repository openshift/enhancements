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

This proposal provides cluster operators the ability to configure CoreDNS [plugins](https://coredns.io/plugins/) and
includes [forward](https://coredns.io/plugins/forward/) as the first plugin implementation.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

1. Is `operator.openshift.io` the best API group for the proposed `ForwardPlugin` type?
2. How are API changes handled for cluster downgrades?
3. Should IPv6 references for `Nameservers` be removed until OpenShift adds IPv6 support?
4. Is the cluster domain (i.e. cluster.local) an invalid `ForwardPlugin` domain?
5. Should the number of `ForwardPlugins` be restricted due to Gap 4?

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

Once CoreDNS starts and has parsed the configuration, it runs servers. Each server is defined by the zones it serves and
a listening port. In the above configuration, CoreDNS starts one server that manages all zones and listens on port 5353.
Each server has its own plugin chain represented within the server block stanza (i.e. `forward`). This proposal will
generate the ConfigMap with a `forward` plugin configuration based on user-provided values of type `ForwardPlugin`.

## Motivation

CoreDNS is responsible for resolving pod and service names for the cluster domain (i.e. `cluster.local`). Otherwise,
CoreDNS proxies the request to a resolver identified by `/etc/resolv.conf` on the corresponding node. Although this provides a consistent and reliable approach for name
resolution, it restricts how cluster operators manage DNS name queries.

### Goals

1. A well defined API for managing CoreDNS plugins.
2. The ability to configure the CoreDNS forwarding plugin.
2. A minimal API surface that can be expanded to support future DNS forwarding and plugin use cases.

### Non-Goals

1. Support every possible DNS forwarding use case.
2. Configure or manage external DNS providers.
3. Provide name query forwarding for other cluster services (i.e. container runtime).
4. Manage what plugins get loaded (i.e. plugin.cfg).

## Proposal

`PluginConfig` defines one or more CoreDNS plugins that can be configured. If a plugin is not defined, it will use a
default configuration provided by the cluster-dns-operator. A plugin is represented as a slice when multiple instances
of the plugin can be expressed within the DNS configuration.

```go
type PluginConfig struct {
    ForwardPlugins []ForwardPlugin `json:"forwardPlugin"`
    // additional future plugins
}
```

The proposed `ForwardPlugin` type defines a schema for associating one or more `Nameservers` to a DNS subdomain
identified by `Domain`. `Nameservers` are responsible for resolving name queries for `Domain`. `Policy` provides a
mechanism for distinguishing where to forward DNS messages when `Nameservers` consists of more than one nameserver:
```go
type ForwardPlugin struct {
    Domain string `json:"domain"`
    Nameservers []string `json:"nameservers"`
    Policy ForwardPolicy `json:"policy,omitempty"`
}

type ForwardPolicy string

const (
    ForwardPolicyRandom ForwardPolicy = "Random"
    ForwardPolicyRoundRobin ForwardPolicy = "RoundRobin"
    ForwardPolicySequential ForwardPolicy = "Sequential"
)
```

Each instance of CoreDNS performs health checking of `Nameservers`. If `Nameservers` consists of more than one
nameserver, `Policy` specifies the order of forwarder nameserver selection. When a healthy nameserver returns an error
during the exchange, another server is tried from `Nameservers` based on `Policy`. Each server is represented by an IP
address or IP address and port if the server listens on a port other than 53.

### User Stories

#### Story 1

As a customer with OpenShift running in AWS and connected to my data center by VPN, I want OpenShift DNS to resolve
name queries for our other internal devices using the DNS servers in our data center.

### Implementation Details/Notes/Constraints

If `ForwardPlugin` consists of more than one `ForwardPlugin`, longest suffix match will be used to determine the
`ForwardPlugin`. For example, if there are two `ForwardPlugins`, one for foo.com and one for a.foo.com, and the query is
for www.a.foo.com, it will be routed to the a.foo.com `ForwardPlugin`.

A maximum of 15 `Nameservers` is allowed per `ForwardPlugin`.

The cluster-dns-operator will generate the `forward` plugin configuration of `configmap/dns-default` based on
`ForwardPlugin` of `dnses.operator.openshift.io` instead of `forward` being statically defined. To achieve this:

1. The [default](https://github.com/openshift/cluster-dns-operator/blob/master/assets/dns/configmap.yaml) ConfigMap
asset and associated code from the [manifests pkg](https://github.com/openshift/cluster-dns-operator/blob/master//pkg/manifests/manifests.go#L70:6)
should be removed.

2. The [desiredDNSConfigMap](https://github.com/openshift/cluster-dns-operator/blob/master/pkg/operator/controller/controller_dns_configmap.go)
function must be modified to create a ConfigMap type with a `forward` configuration based on `ForwardPlugin`.
If `ForwardPlugin` is not present or an invalid `ForwardPlugin` is provided, the ConfigMap will contain
`forward . /etc/resolv.conf`. Otherwise, desiredDNSConfigMap will use the provided `ForwardPlugin` to construct the
forwarding configuration and use `/etc/resolv.conf` as a resolver of last resort. For example:

```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  pluginConfig:
    forwardPlugins:
    - domain: foo.com
      nameServers:
        - 1.2.3.4
        - 5.6.7.8:5353
      policy: RoundRobin
    - domain: bar.com
      nameServers:
        - 4.3.2.1
        - 8.7.6.5:5353
      policy: Sequential
```

The above `DNS` will produce the following `ConfigMap`:
```yaml
apiVersion: v1
data:
  Corefile: |
    foo.com:5353 {
        forward . 1.2.3.4 5.6.7.8:5353
        policy round_robin
    }
    bar.com:5353 {
        forward . 4.3.2.1 8.7.6.5:5353
        policy sequential
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
nameserver in `Nameservers` must be a valid IPv4 or IPv6 address. If the nameserver listens on a port other than 53,
a valid port number must be specified. A colon is used to separate the address and port, `IP:port` for IPv4 or
`[IP]:port` for IPv6.

#### Transport
UDP is used to transport DNS messages and `ForwardPlugin` health checks. Any UDP transport will automatically retry with
the equivalent TCP transport if the response is truncated (TC flag set in response).

#### Health Checking Details

Nameserver health checking is performed in-band. A health check is performed only when CoreDNS detects an error.
The check runs in a loop, every 0.5s, for as long as the forwarder reports unhealthy. Once healthy, CoreDNS stops
health checking until the next error. The health checks use a recursive DNS query (. IN NS) to get forwarder health.
Any response that is not a network error (REFUSED, NOTIMPL, SERVFAIL, etc) is taken as a healthy forwarder.
When all `Nameservers` are down CoreDNS assumes health checking as a mechanism has failed and will try to connect to a
random nameserver (which may or may not work).

### Risks and Mitigations

#### Gap 1
Forwarded name queries and nameserver health checks are insecure. This may allow a malicious actor to impersonate an
`ForwardPlugin` nameserver.

#### Mitigation 1
Add TLS support to secure forwarded name queries. Clearly document this insecurity in product documentation in the
meantime.

#### Gap 2

No mechanism for customizing DNS forwarding at install time.

#### Mitigation 2

Use the OpenShift Enhancement process to solicit feedback from the installer team.

#### Gap 3

Surfacing status for nameservers that fail health checks.

#### Mitigation 3

`coredns_forward_healthcheck_failure_count_total{to}` and `coredns_forward_healthcheck_broken_count_total{}` Prometheus
metrics are exported. Ensure these metrics are surfaced through the OpenShift monitoring stack.

#### Gap 4

An increased utilization of compute resources on each node due to the potential of running many `ForwardPlugins`, each
with up to 15 nameservers that are actively checked for health.

#### Mitigation 4

Test utilization using multiple `ForwardPlugins` with multiple `Nameservers`. Add a warning in the documentation that
adding a large number of forwarders may incur a performance penalty or hit memory limits.

## Design Details

### Test Plan

Implement the following end-to-end test in addition to unit tests:

- Create a `DNS` with a `ForwardPlugin` that uses a `Nameserver` to resolve a hostname from `Domain`.
- Start the dns-operator and check the logs for healthcheck failures for the `Nameserver`.
- Create a pod that performs an nslookup for a hostname in the cluster domain.
- Have the pod perform an nslookup for a hostname in `Domain`.
- If the nslookup succeeds, check if the nslookup server matches the `Nameserver`.
- Have the pod perform an nslookup for a hostname in `/etc/resolve.conf`.

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

Upgrades will continue to use the existing `forward . /etc/resolv.conf` forwarding configuration. Edit
 `dnses.operator.openshift.io` to modify the default forwarding behavior by adding a `ForwardPlugin`. For example:
 
```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  pluginConfig:
    forwardPlugins:
    - domain: foo.com
      nameServers:
        - 1.2.3.4
        - 5.6.7.8
```

### Version Skew Strategy

TODO

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

1. Additional resource utilization (i.e. memory) on each node required for the forward plugin, health checks, etc..

## Alternatives

Possible alternatives are listed [here](https://docs.google.com/document/d/17OEwYs9HuCeGtFKfJLwtk809C6MIjZmguce5-LQYxIM/edit?usp=sharing).

## Infrastructure Needed [optional]

TODO
