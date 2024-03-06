---
title: microshift-router-configuration
authors:
  - "@pacevedom"
reviewers:
  - "@eslutsky"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@pliurh"
  - "@jerpeter1"
  - "@Miciah"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2024-01-08
last-updated: 2024-01-31
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1069
---

# MicroShift router default configuration options

## Summary
MicroShift's default router is created as part of the platform, but does not
allow configuring any of its specific parameters. For example, you can not
disable the router or change its listening ports.

In order to allow these operations and many more, a set of configuration options
is proposed.

## Motivation
MicroShift's default ingress router comes as part of the product and is always
deployed. As part of the start procedure, MicroShift will configure the router
with fixed parameters, such as being always enabled, and tied to ports 80 and
443. None of these are configurable for the user, and they also tie to how the
router is exposed in the cluster.

In order to allow more flexibility, the following configuration parameters shall
be added:
- Enable/Disable. Some use cases may be egress only, meaning they do not need
  an ingress controller. In this case the router pod should not be scheduled
  to save resources. There should be no rules associated to firewalld or
  iptables either for an improved security posture.
- Listening ports. Allow configuring ports other than 80 and 443 improves
  flexibility.
- Which IPs the router listens on. Some use cases may require the router to be
  reachable only from certain networks.

### User Stories
As a MicroShift user, I want to enable the default router with a configuration
option.

As a MicroShift user, I want to disable the default router and have all the
manifests automatically removed.

As a MicroShift user, I want to be able to configure the ports on which the
default router is listening.

As a MicroShift user, I want to be able to choose the IPs where the router is
listening.

As a MicroShift user, I want to be able to allow traffic to the router from
specific IP addresses.

### Goals
* Allow users to enable the router.
* Allow users to disable the router.
* Allow users to reach the router in the configured ports.
* Allow users to configure in which IPs the router listens.
* Allow users to allow traffic from specific IP addresses.
* Allow users to deny traffic to the router.
* Internal access from applications to the router must remain unchanged.

### Non-Goals
N/A

## Proposal
Each configuration option will get its own section with specifics and
details.
As a common baseline, a new top level section will be added to the
configuration:
```yaml
apiServer:
   ...
# new from here.
ingress:
  status: <Enabled|Disabled> # Defaults to Enabled.
  ports:
    http: <int> # Defaults to 80.
    https: <int> # Defaults to 443.
  expose: # Defaults to IPs in the host. Details below.
    hostnames:
    - <hostname>
    interfaces:
    - <NIC name>
    ipAddresses:
    - <IP address>
```

### Enable/disable the router
The following configuration is proposed:
```yaml
ingress:
  status: <Enabled|Disabled> # Defaults to Enabled.
```

With this option MicroShift will decide whether it should create the router
upon starting. This includes not just the pod, but all the associated resources
that come along with it: namespace, services, configmaps, etc.

When setting the option to `Disabled`, the next MicroShift restart will
delete all the default router related resources.

Note that any existing routes will stop being served and it will require an
application deploying an OpenShift router instance.

Setting the option to `Enabled` will deploy the router, while `Disabled` will
disable it. Default will be `Enabled`.

### Listening ports
The following configuration is proposed:
```yaml
ingress:
  ports:
    http: <int> # Defaults to 80.
    https: <int> # Defaults to 443.
```

MicroShift does not own the host, which means there might be other
applications running alongside and some ports may already be opened.
MicroShift must be able to accommodate such situations and be ready to listen
in other ports, hence the configuration options.

In order to allow this configuration and many other advantages, the
router service shall be changed so that it is exposed using `LoadBalancer`
type instead of using host ports. See the [Design Details section](#why-loadbalancer-service)
for more information.

#### Firewalling ports
Using `LoadBalancer` service type prevents the usage of firewalld to block
access to ports. Since this is a feature that users may require for auditing,
an alternative is provided.

The `NetworkPolicy` resource fits the needs for blocking traffic to services
and pods. Policies can also be configured to allow specific IP addresses and
provide a high degree of customization for network traffic. They use the CNI
instead of host level settings, providing greater flexibility and being self
contained within MicroShift.

Users in need of blocking traffic shall use the `NetworkPolicy` resource. A
section in the documentation will be added to describe how to do this.

### Configure IPs where the router listens
The following configuration is proposed:
```yaml
ingress:
  expose: # Defaults to IPs in the host. Details below.
    hostnames:
    - <hostname>
    interfaces:
    - <NIC name>
    ipAddresses:
    - <IP address>
```
As described in [this section](#using-loadbalancer-service-type), the use of
`LoadBalancer` makes ovnk configure iptables rules to expose the service
outside of the cluster.
Building on [LoadBalancer controller enhancement](loadbalancer-service-support.md),
the service is exposed using the node IP and configured ports.

All the IPs where the router is exposed need to be part of the `LoadBalancer`
service status definition. In order to provide more flexibility, the
configuration options are able to take IP addresses, hostnames and/or NIC
names.

MicroShift will default to listen to external IPs, the service IP and the
apiserver IP, as this was the previous behavior and it needs to keep it for
compatibility. Any advanced configuration, such as multiple interfaces, VLANs,
etc. will need user's configuration. Any user's configuration will override all
of the defaults.

The hostnames will be automatically resolved to their IP addresses by
MicroShift, same as the NIC names.

### Allow specific IP addresses
This may be seen as a special case of firewalling, but using IP addresses
instead of ports. Using the same `NetworkPolicy` resources, IPs may be
specified to allow traffic from them and reject it from any other source.

Please note this is a manual procedure for the user. Documentation will be
added on how to perform the required actions.

More information in the design details section.

### Workflow Description
**cluster admin** is a human user responsible for configuring a MicroShift
cluster.

1. The cluster admin adds specific configuration for the router prior to
   MicroShift's start.
2. After MicroShift started, the system will ingest the configuration and setup
   everything according to it.
3. The router will enabled/disabled, be exposed on the specified ports, on the
   specified IPs, allowing only specified IP addresses, all according to the
   cluster admin provided configuration. This includes the status in the
   LoadBalancer service type for the router.

### API Extensions
As described in the proposal, there is an entire new section in the configuration:
```yaml
ingress:
  status: <Enabled|Disabled> # Defaults to Enabled.
  ports:
    http: <int> # Defaults to 80.
    https: <int> # Defaults to 443.
  expose: # Defaults to IPs in the host. Details below.
    hostnames:
    - <hostname>
    interfaces:
    - <NIC name>
    ipAddresses:
    - <IP address>
```

For more information check each individual section.

### Topology Considerations
#### Hypershift / Hosted Control Planes
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is solely intended for MicroShift.

### Implementation Details/Notes/Constraints
The default router is composed of a bunch of assets part of the MicroShift
binary. These assets come from the rebase, copied from the original router in
[cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator).
There is already a [LoadBalancer service](https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/manifests/assets/router/service-cloud.yaml)
included, which MicroShift does not copy yet. Depending on the options
configured in MicroShift, this resource will need further customization done
by MicroShift's start process.

The rest of the changes imply only additions. The LoadBalancer controller needs
an expansion on its capabilities to include more IPs in the status and log any
changes to services. All the logic to control NetworkPolicy resources is also
new.

#### How config options change manifests
Each of the configuration options described above has a direct effect on the
manifests that MicroShift will apply after starting.

The `ingress.status` option will drive whether the router `Deployment` and the
`Service` get created.
```yaml
ingress:
  status: <Enabled|Disabled> # Defaults to Enabled.
```

The `ingress.policy.ports.http` and `ingress.policy.ports.https` will determine,
within the `Deployment` and the `Service`, which ports get configured to be
exposed.
```yaml
ingress:
  ports:
    http: <int> # Defaults to 80.
    https: <int> # Defaults to 443.
```

The `ingress.policy.expose` option contains lists of hostnames, NIC names and
IP addresses. MicroShift will translate them to IP addresses and then update
the `status.loadBalancer` field in the `Service`. Ovnk will pick up this field
to configure the iptables rules. This is described [here](#using-loadbalancer-service-type).
```yaml
ingress:
  expose: # Defaults to IPs in the host. Details below.
    hostnames:
    - <hostname>
    interfaces:
    - <NIC name>
    ipAddresses:
    - <IP address>
```

### Risks and Mitigations
Some of the features in this enhancement proposal rely on the
[LoadBalancer controller](loadbalancer-service-support.md).
Some of the features rely on NetworkPolicy resources, so the CNI needs
to support that.

Disabling the router requires a MicroShift restart with all the associated
consequences (apiserver downtime, etc.).

### Drawbacks
Some of the features depend on a non-agnostic CNI design. The `LoadBalancer`
service controller depends on ovnk to configure iptables rules so that the
service is correctly exposed. The same applies to `NetworkPolicy` resources,
which need the `LoadBalancer` service to be in place.

A different behavior should be expected if using a different CNI.

## Design Details
#### Why LoadBalancer service
In the current implementation router ports are fixed and can not be configured.
Back when there was no support for `LoadBalancer` service types, router was
forced to use a different way of getting exposed. Using ports 80 and 443 meant
that NodePort service types can not be used. Using host network would also
bind port 1936 to the host, which is used for internal metrics. The only option
that was left was using host ports.

Ports 80 and 443 are configured to use `hostPort` option. This will instruct
CRI to bind container ports 80 and 443 to the host in the form of iptables
rules, forwarding incoming traffic to the pod's IP address.

The following limitations apply to the current setup:
* Iptables rules bypass firewalld settings. This means any firewall
  configuration applying to host ports will be ignored. It is not possible
  to control incoming traffic to these ports.
* NetworkPolicy resources do not apply to host ports. These resources are
  able to control how traffic reaches which endpoints, including external
  sources.
* Any pod using this option is unable to scale within the same node.
* Port numbers are bound to the deployment, therefore any changes require
  a restart of the pod.

##### Using LoadBalancer service type
Common ways of exposing services outside of the cluster are using services of
types NodePort or LoadBalancer. In MicroShift, ports from the service are
picked up from ovnk (the CNI) and turned into iptables rules to forward them
to the service IP. In this case, default router should be exposed using a
LoadBalancer service type.

When using this kind of service ovnk will configure special iptables rules
that will forward traffic to the `LoadBalancer` service network IP. These rules
are also configured to use the node IP as destination address, as per
[LoadBalancer support](loadbalancer-service-support.md) feature.

#### Firewalling ports
Using LoadBalancer service will create special iptables rules with more
precedence than those from firewalld. This means a `LoadBalancer` service is
immune to such configurations. In order to firewall ports the user needs to
take action by either creating `NetworkPolicy` resources or disabling the
router.

Disabling the router might be disruptive, as not all traffic may come from
external sources. This procedure also requires a restart, which might not be
desirable/possible in all situations.

Using `NetworkPolicy` a user may control the traffic that is allowed into
the cluster. By creating a deny-all policy the router will still remain
operative for internal traffic, while rejecting all external traffic. An
example follows:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: block-router
  namespace: openshift-ingress
spec:
  ingress:
  - from:
    - ipBlock:
        cidr: 10.42.0.0/16
  podSelector:
    matchLabels:
      ingresscontroller.operator.openshift.io/deployment-ingresscontroller: default
  policyTypes:
  - Ingress
```
Assuming the pod network is configured to use CIDR `10.42.0.0/16`, the above
policy will block all traffic not coming from that network.

Any of these methods is equivalent to using firewalld to block traffic, but
using the CNI instead of a host level setting. This provides greater
flexibility, as it is self contained within MicroShift APIs.

If a user needs to block traffic to a port this needs a `NetworkPolicy`
resource like the one shown above. This is NOT automatically done by
MicroShift, user action is required.

#### Exposing the router
Relying on [LoadBalancer controller enhancement](loadbalancer-service-support.md)
means the services are updated by this component. However, ovnk only turns IP
addresses into iptables rules, it does not take Hostname, which is another
valid field for the service status.

Since the configuration uses NIC names and also host names, the controller
needs to translate these to their corresponding IP addresses, and then
configure them in the service. To keep up with the dynamic nature of IP
addresses and names, it needs to perform periodic checks and updates.

#### Allowing specific IP addresses
Allowing specific IP addresses is achieved through the use of `NetworkPolicy`
resources, as seen in previous examples. These policies include the allowed IP
addresses and ports in the spec, meaning they reject everything else that is
not included explicitly.

Due to MicroShift's networking setup, there is one more change required for
the `NetworkPolicy` to filter out based on the source IP.

As it can be seen in the iptables rules, traffic towards the router ports is
subject to DNAT to the router service IP and port. All services use
`externalTrafficPolicy: Cluster`, which will SNAT traffic's source IP to match
that of the node. This gets further NAT, as described here. By the time the
`NetworkPolicy` is evaluated the source IP has been replaced with
`100.64.0.0/16`, which will not produce matches against the configured IP in
the policy, and traffic will get rejected.

To overcome this, all LoadBalancer services in MicroShift that are subject to
get NetworkPolicy resources to control external traffic must set
`externalTrafficPolicy: Local`. This setting will not SNAT packets and the
source IP is preserved until NetworkPolicy checks run.

This setting, however, comes at the expense of an even load when having more
than one node, as traffic will not be sent to another node on the same port.
This is not an issue in single node, which is MicroShift's use case and design
principle.

This setting is also the default in OCP when deploying in cloud, having a load
balancer before the cluster. Manifests for both OCP and MicroShift are
compatible in this regard.

## Open Questions
N/A

## Test Plan
All of the changes listed here will be included in the current e2e scenario
testing harness in MicroShift.

## Graduation Criteria
Targeting GA for MicroShift 4.16 release.

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
In the previous implementation, using host ports, the router would listen on
any interface (except loopback). This included some of the internal IP
addresses from ovnk, such as `169.254.169.2`, which is the link local IP
address that ovnk uses in `br-ex` for internal handling of flows.

The approach described in this enhancement does not expose the router in such
IP, as it introduces conflicts between iptables rules and openflow.

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions

### Failure Modes
If the configured entries in `ingress.expose.ipAddresses` and
`ingress.expose.interfaces` do not exist in the node, MicroShift should fail
to start.

## Support Procedures
Additional logging is added to the LoadBalancer controller to show the ports
each service is using. Example:
```
Jan 24 14:37:14 microshift-dev microshift[8828]: microshift-loadbalancer-service-controller I0124 14:37:14.581388    8828 controller.go:127] Service openshift-ingress/router-internal-default using ports [80, 443]
```

## Implementation History
N/A

## Alternatives
- Instead of using a LoadBalancer service type, hostNetwork may be applied to
  the router pod. This approach does not allow NetworkPolicies to work with
  them, as they bypass networking. The metrics port would also get exposed on
  the host.
- Keep using hostPort. This approach does not allow NetworkPolicies either, for
  the same reasons as the hostNetwork alternative. Also some of the cons were
  listed in a previous section.
