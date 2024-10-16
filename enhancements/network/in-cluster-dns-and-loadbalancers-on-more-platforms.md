---
title: in-cluster-dns-and-loadbalancers-on-more-platforms
authors:
  - "@mhrivnak"
  - "@eranco74"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@cybertron"
  - "@tsorya"
  - "@zaneb"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2024-08-26
last-updated: 2024-08-26
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
see-also:
  - "/enhancements/network/baremetal-networking.md"
replaces:
superseded-by:
---

# In-cluster DNS and load balancers on more platforms

## Summary

Multiple on-prem platform types, including `baremetal` and `openstack`,
[provide in-cluster implementations of network
services](https://github.com/openshift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md)
that are required in order to have a viable stand-alone cluster:

* CoreDNS for in-cluster DNS resolution
* haproxy with keepalived to provide in-cluster load balancers (ingress) for the API server and workloads

Continuing the work from that original [enhancement
proposal](https://github.com/openshift/enhancements/blob/master/enhancements/network/baremetal-networking.md),
those services should also be available for optional inclusion when installing
a cluster with the `external` or `none` platform types, which are likewise
often used in environments that lack a suitable alternative for DNS and/or load
balancers.

## Motivation

Provisioning and configuring a [DNS
system](https://docs.openshift.com/container-platform/4.16/installing/installing_platform_agnostic/installing-platform-agnostic.html#installation-dns-user-infra_installing-platform-agnostic)
and [load
balancers](https://docs.openshift.com/container-platform/4.16/installing/installing_platform_agnostic/installing-platform-agnostic.html#installation-load-balancing-user-infra_installing-platform-agnostic)
manually for an OpenShift cluster is a substantial burden on the user. In a
cloud environment that's not already supported by the OpenShift installer,
utilizing the native offerings requires additional work up-front, creates an
ongoing maintenance burden, and more monetary cost for infrastructure. And not
all cloud environments offer suitable options. On-prem, there may not exist
sufficient DNS and/or load balancer services nor additional infrastructure on
which to run them.

Many users end up deploying a cluster with platform type `baremetal` when all
they really want is to use the in-cluster network services. Instead, it should
be possible to utilize those in-cluster network services without them being
coupled to the baremetal platform type.

For example, the assisted-installer and the agent based installer are often
used to deploy clusters into “generic” environments where there is not an
opportunity to utilize an external DNS or load balancer solution. Thus the
assisted-installer sets the platform type as `baremetal` regardless of whether
the systems are actually running on bare metal or whether there is any intent
to use metal3 integrations. The resulting cluster has all of the appearances of
being bare metal, including a BareMetalHost resource for each Node, which can
be confusing to users. Even the web console’s Overview landing page shows
“Infrastructure provider: BareMetal” in addition to “N Nodes” and “N Bare Metal
Hosts”.

Single Node OpenShift uses platform type `none` and [requires the user to
configure DNS records
manually](https://docs.openshift.com/container-platform/4.16/installing/installing_sno/install-sno-installing-sno.html#install-sno-installing-sno-manually).
When using the assisted-installer, it configures dnsmasq in new SNO clusters as
a convenience. But it would be better for the internal DNS service to be a
native part of platform type `none` so that it is easily available to all
users, regardless of how they are installing SNO.

### User Stories

As a user deploying OpenShift in an environment that lacks a suitable DNS
and/or load balancer solution, and with no intent to utilize metal3-related
bare metal features, I want to utilize the in-cluster network services without
being forced to use the `baremetal` platform type.

As a user deploying OpenShift with the `external` platform type into an
environment of my choosing, I want the option to use the in-cluster network
services because they are easier to use than manually deploying, configuring
and managing the alternatives that may be natively available in the
environment.

As a user deploying Single Node OpenShift, I want the convenience of a
cluster-internal DNS solution.

As a user deploying OpenShift in a mixed environment, such as [virtualized
control plane nodes and bare metal worker
nodes](https://access.redhat.com/solutions/5376701), I am forced to select
platform type `none`, but I still want the option to use the in-cluster network
services.

As a developer enabling OpenShift on a new platform via the `external` platform
type, I want to get an OpenShift cluster up and running with as little friction
as possible so I can start adding integrations with features of the
environment.

### Goals

Enable stand-alone OpenShift clusters to be viable out-of-the-box in
environments that A) lack a suitable external DNS and/or load balancer
solution, and B) are not one of the platform types that already provide those
services in-cluster (`baremetal`, `openstack`, `vsphere`, and `ovirt`).

Allow users to opt-in for in-cluster DNS and load balancer services with
platform types `none` and `external`.

Stop requiring users to select the `baremetal` platform type when all they
really want is the in-cluster DNS and load balancer services.

Make it easy for Single Node OpenShift users to deploy the cluster-internal DNS
service.

### Non-Goals

The in-cluster network infrastructure has a limitation that it requires nodes
to be on the same subnet. This proposal does not seek to change or remove that
limitation.

## Proposal

The install-config.yaml platform section for both `external` and `none` will
include optional fields to deploy and configure coredns and/or the in-cluster
load balancers. Actual deployment and management will be handled the same way
it already is on other platforms.

### Workflow Description

A user or automation tool (such as the assisted-installer) that is editing
install-config.yaml prior to cluster installation will be able to:
* Enable internal DNS
* Provide VIPs that implicitly enable in-cluster load balancers

### API Extensions

In the `InstallConfig`, the sections for External and None platforms will have
new settings that:
* Enable internal DNS
* Provide VIPs that implicitly enable internal load balancers (See example under Implementation Details)

The `Infrastructure` API will add fields in the
[`PlatformSpec`](https://github.com/openshift/api/blob/ef419b6/config/v1/types_infrastructure.go#L272)
and
[`PlatformStatus`](https://github.com/openshift/api/blob/ef419b6/config/v1/types_infrastructure.go#L389)
that mirror the corresponding fields for baremetal, including:
* `APIServerInternalIPs` in Spec and Status
* `IngressIPs` in Spec and Status
* `LoadBalancer` in Status

Those fields will be added to the `External` platform Spec and Status. For the
`None` platform, a new Spec and Status section will need to be created.

### Topology Considerations

#### Hypershift / Hosted Control Planes

None

#### Standalone Clusters

The change is only relevant for standalone clusters.

#### Single-node Deployments or MicroShift

Single Node OpenShift benefits from this change as described above. Being a
single node, it does not need the loadbalancers, but it does require a DNS
solution.

Assisted-installer already deploys dnsmasq by default as a cluster-internal DNS
solution for SNO, which has been valuable and successful.

### Implementation Details/Notes/Constraints

The `InstallConfig` will gain new settings for `InClusterLoadBalancer` and
`InternalDNS`. They are shown below, added to the existing settings for the
External platform type.

```
type InClusterLoadBalancer struct {
    // APIVIPs contains the VIP(s) to use for internal API communication. In
    // dual stack clusters it contains an IPv4 and IPv6 address, otherwise only
    // one VIP
    //
    // +kubebuilder:validation:MaxItems=2
    // +kubebuilder:validation:UniqueItems=true
    // +kubebuilder:validation:Format=ip
    APIVIPs []string `json:"apiVIPs,omitempty"`

    // IngressVIPs contains the VIP(s) to use for ingress traffic. In dual stack
    // clusters it contains an IPv4 and IPv6 address, otherwise only one VIP
    //  
    // +kubebuilder:validation:MaxItems=2
    // +kubebuilder:validation:UniqueItems=true
    // +kubebuilder:validation:Format=ip
    IngressVIPs []string `json:"ingressVIPs,omitempty"`
}


// Platform stores configuration related to external cloud providers.
type Platform struct {
    // PlatformName holds the arbitrary string representing the infrastructure
    // provider name, expected to be set at the installation time. This field
    // is solely for informational and reporting purposes and is not expected
    // to be used for decision-making.
    // +kubebuilder:default:="Unknown"
    // +default="Unknown"
    // +kubebuilder:validation:XValidation:rule="oldSelf == 'Unknown' || self == oldSelf",message="platform name cannot be changed once set"
    // +optional
    PlatformName string `json:"platformName,omitempty"`

    // CloudControllerManager when set to external, this property will enable
    // an external cloud provider.
    // +kubebuilder:default:=""
    // +default=""
    // +kubebuilder:validation:Enum="";External
    // +optional
    CloudControllerManager CloudControllerManager `json:"cloudControllerManager,omitempty"`

    // InClusterLoadBalancer is an optional feature that uses haproxy and
    // keepalived as loadbalancers running in the cluster. Is is useful in
    // environments where it is not possible or desirable to use loadbalancers outside
    // of the cluster.
    // +optional
    InClusterLoadBalancer *InClusterLoadBalancer `json:"inClusterLoadBalancer,omitempty"`

    // InternalDNS, when set, activates a DNS service running inside the cluster
    // to provide DNS resolution internally. It is useful in environments where
    // it is not possible or desirable to manage the cluster's internal DNS
    // records in an external DNS system.
    // +kubebuilder:default:=""
    // +default=""
    // +kubebuilder:validation:Enum="";CoreDNS
    // +optional
    InternalDNS InternalDNS `json:"internalDNS,omitempty"`
}

type InternalDNSType string

const (
    // CoreDNS is the default service used to implement internal DNS within a cluster.
    CoreDNS InternalDNS = "CoreDNS"
)
```

### Risks and Mitigations

All of the components in question are already widely deployed in OpenShift
clusters.

### Drawbacks


## Open Questions [optional]


## Test Plan

**Note:** *Section not required until targeted at a release.*

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

### Dev Preview -> Tech Preview


### Tech Preview -> GA


### Removing a deprecated feature


## Upgrade / Downgrade Strategy

No change to how the components are upgraded and/or downgraded today.

## Version Skew Strategy

No change.

## Operational Aspects of API Extensions

This proposal will enable clusters installed in the future to have fewer CRDs,
since they'll be able to use in-cluster network services without having to
select the `baremetal` platform type. Thus, the unused CRDs from the
`baremetal` platform won't be present on those clusters.

## Support Procedures

No new support implications.

## Alternatives

### Move In-Cluster Network Settings Out of the Platform Spec

Instead of embedding the in-cluster network settings within the platform
specification, these settings could be moved to a separate, dedicated section
in the install-config.yaml. This approach would completely decouple the setup
of these in-cluster network services from platform-specific settings, allowing
greater flexibility in utilizing the network services on any platform type.

Pros:
* Decouples the in-cluster network services from platform-specific settings.
* Simplifies the platform specification and provides a clear, dedicated section for network services.
* De-duplicates settings that have been mirrored into `baremetal`, `openstack`, and `ovirt` platforms.

Cons:
* Introduces a new section in the configuration file, which may confuse users.
* May conflict with the settings for these network services that already exist on specific platforms, including baremetal and openstack.
* Would require guardrails to ensure they don’t get deployed with platforms that utilize other solutions, even if the user configures them for deployment in the install-config.

