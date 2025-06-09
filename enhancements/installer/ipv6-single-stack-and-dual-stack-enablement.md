---
title: ipv6-single-stack-and-dual-stack-enablement
authors:
- "@barbacbd"
- "@thvo"
- "@jhixon74"
- "@sadasu"
reviewers:
- "@patrickdillon, for Installer aspects"
- "@sadasu, for Installer aspects"
- "@thvo, for Installer AWS aspects"
- "@jhixson74, for Installer Azure aspects"
- "@barbacbd, for Installer GCP aspects"
approvers:
- "@patrickdillon"
- "@sadasu"
api-approvers:
- "@joelspeed"
creation-date: 2025-01-02
last-updated: 2025-01-03
tracking-link: # link to the tracking tickets 
- https://issues.redhat.com/browse/CORS-4158
- https://issues.redhat.com/browse/CORS-3526 # GCP
---

# IPv6 Single Stack and Dual Stack Enablement

## Summary

This enhancement adds the ability for a customer to enable IPv6 
stack for AWS, Azure, and GCP clusters. This feature will allow clusters to
consist of instances and other cluster resources that have IPv6 addresses. IPv6 
single stack clusters are supported in Azure. Dual Stack clusters are supported
in AWS and GCP.

## Motivation

### AWS

TODO

### Azure

TODO

### GCP 

A dual stack configuration provides flexibility, because cluster services 
can utilize IPv4 and IPv6 addresses. Cluster services can access services on
the internet that may only be reachable via IPv6 while also having the ability
to access internet services that are only accessible via IPv4 endpoints. 

By supporting both protocols, applications can be migrated to IPv6 endpoints
without disrupting current connections with IPv4 endpoints. This feature will
also assist in future-proofing applications. IPv4 addresses are being exhausted
and more applications/resources are forced to use IPv6 endpoints. The dual 
stack configuration will allow clusters to migrate as resource exhaustion 
continues.

**IPv6 _may_ provide increased security advantages over IPv4. Dual stack allows
the cluster to leverage these benefits.**

### Goals

#### AWS

TODO

#### Azure

TODO

#### DNS

- The ability to create multiple IPV4 (A) and IPV6 (AAAA) DNS Record Types per installation.

#### GCP 

- Enable GCP customers to create clusters using an IPv6 stack configuration.

### Non-Goals

None

### User Stories

- As an Openshift Installer developer, I would like to protect new API fields with a feature 
gate, so I can build features without concern that an incomplete feature would be exposed to customers.

- As a member of Openshift with customers and/or clients, I would like to explain the benefits of IPv6 stack
clusters. I want to explain that configuring IPv6 stack clusters will allow long term supportability and flexibility
of our product. 

- As a customer, I want to create a cluster that provides access to all possible endpoints now and in the future.

## Proposal

1. For the GCP cloud provider, the `platform` section of the install config should allow
the user to select IPv4_ONLY or IPv6_ONLY for the `stackType`. 

2. The install-config parameters for `machineNetwork` and `clusterNetwork` should be updated to allow IPv6 addresses.
_Note_: This may be applicable to GCP and Azure only.

3. Cluster Manifests need to be altered to include networking details for IPv6 single stack clusters. This will include the
cluster DNS manifest where the type of DNS Record(s) to be created are stored. When the user has selected to install using
a Dual Stack configuration, then two different DNS record types are required to be passed (A for IPv4 and AAAA for IPv6 records).

### Workflow Description

#### Example workflow for AWS feature

TODO

#### Example workflow for Azure feature

TODO

#### Example workflow for GCP feature

1. Developer add a new enumeration string `stackType` to the Openshift Installer. The new enumeration should have
equivalent values to the Openshift API enumeration. This is a Tech Preview feature set feature.
2. The User may now set the `stackType` in the install-config file. If the user leaves the `featureSet` field
blank, then `openshift-install` returns an `error: the TechPreviewNoUpgrade feature set must be enabled to use this field`
3. Installer validates the values for the enumeration as they must match one of the strings
4. Installer sets correct subnetwork stack type that is passed to the cluster api provider via the `ClusterSpec` structure
5. Cluster API Provider sets the correct stack type for the subnetworks in the cluster
6. The cluster resources are successfully created by the cluster api provider
7. Installer generates FeatureGate manifest
8. Installer emits warning message indicating the enabled feature set when provisioning infrastructure 
9. Cluster installs successfully with TechPreview cluster

## Operational Aspects of API Extensions

### API Extensions

#### Openshift


The DNS record type struct needs a new value for IPv6 records.

```go
const (
	// CNAMERecordType is an RFC 1035 CNAME record.
	CNAMERecordType DNSRecordType = "CNAME"

	// ARecordType is an RFC 1035 A record.
	ARecordType DNSRecordType = "A"

	// AAAARecordType is an RFC 3596 AAAA record that is used to map a domain name to an IPv6 address.
	AAAARecordType DNSRecordType = "AAAA"
)
```

The record types must then become a list. The list will be required for Dual Stack clusters where an IPv4 and IPv6 record must exist for
`*.apps` in the DNS Zones.


A new feature will be added to the Openshift API for AWS and GCP (it is not currently known if Azure will require this information). 
The feature will make use of the `stackType` variable. 

```go
    type StackType string

    const (
        // StackTypeIPV4 indicates that the network configuration is for single stack IPv4.
        StackTypeIPV4      StackType = "IPV4"
        // StackTypeDualStack indicates that the network configuration is for both IPv4 and IPv6 addresses.
        StackTypeDualStack StackType = "IPV4_IPV6" 
        // StackTypeIPV6 indicates that the network configuration is for single stack IPv6.
        StackTypeIPV6      StackType = "IPV6"
	)
```

#### Cluster API Provider GCP

**Currently, there is no active features or development in Cluster API GCP Provider for IPv6 or Dual Stack Support.**

##### Network/Subnets

The GCP cluster api provider should be updated to include the enumeration to control the stack type.

An example of the GCP changes; the following would be added to the GCPMachineSpec and the SubnetSpec

```go
	// stackType represents the stack type for the subnet.
	// Allowed values are IPV4_ONLY, IPV6_ONLY, IPV4_IPV6.
	//
	// When set to IPV4_ONLY, new VMs in this subnet will only be assigned IPv4 addresses.
	// When set to IPV6_ONLY, new VMs in this subnet will only be assigned IPv6 addresses.
	// When set to IPV4_IPV6, new VMs may be assigned an IPv4, IPv6, or both addresses.
	// +kubebuilder:validation:Enum=IPV4_ONLY;IPV4_IPV6;IPV6_ONLY
	// +kubebuilder:default=IPV4_ONLY
	// +optional
	StackType string `json:"stackType,omitempty"`
```

The user can set the stack type in the subnetwork structure to be created by the Google Compute API. The info is required for
each Machine to ensure that it is created and attached to the correct subnet and network.

The following variables should be added to the subnet spec too

```go
    // ExternalIpv6Prefix: The external IPv6 address range that is owned by this
    // subnetwork.
    ExternalIpv6Prefix string `json:"externalIpv6Prefix,omitempty"`

    // InternalIpv6Prefix: The internal IPv6 address range that is owned by this
    // subnetwork.
    InternalIpv6Prefix string `json:"internalIpv6Prefix,omitempty"`

    // Ipv6AccessType: The access type of IPv6 address this subnet holds. It's
    // immutable and can only be specified during creation or the first time the 
	//subnet is updated into IPV4_IPV6 dual stack.
    //
    // Possible values:
    //   "EXTERNAL" - VMs on this subnet will be assigned IPv6 addresses that are
    // accessible via the Internet, as well as the VPC network.
    //   "INTERNAL" - VMs on this subnet will be assigned IPv6 addresses that are
    // only accessible over the VPC network.
    Ipv6AccessType string `json:"ipv6AccessType,omitempty"`
```

The variables are taken directly from the subnet struct in GCP. These variables would be used to specify information about
the subnets that you wish to create. When these variables are not present the GCP provider will use default values.

```go
subnet := &compute.Subnetwork{
	Name:      subnetwork.Name,
	Region:    subnetwork.Region,
        ...
	StackType: subnetwork.StackType,      << Added feature
}

if subnetwork.StackType == "IPV4_ONLY" || subnetwork.StackType == "IPV4_IPV6" {
    secondaryIPRanges := []*compute.SubnetworkSecondaryRange{}
    for rangeName, secondaryCidrBlock := range subnetwork.SecondaryCidrBlocks {
        secondaryIPRanges = append(secondaryIPRanges, &compute.SubnetworkSecondaryRange{RangeName: rangeName, IpCidrRange: secondaryCidrBlock})
    }
    subnet.SecondaryIpRanges = secondaryIPRanges
    subnet.IpCidrRange = subnetwork.CidrBlock
}

if subnetwork.StackType == "IPV6_ONLY" || subnetwork.StackType == "IPV4_IPV6" {
    subnet.ExternalIpv6Prefix = subnetwork.ExternalIpv6Prefix
    subnet.InternalIpv6Prefix = subnetwork.InternalIpv6Prefix
    subnet.Ipv6AccessType = subnetwork.Ipv6AccessType
}
```

##### Firewalls

The firewall rules should be adjusted to follow the IPv4 and IPv6 addresses and ports found [here](https://cloud.google.com/load-balancing/docs/health-check-concepts#ip-ranges).

The IPv4 Addresses are the following:

```go
SourceRanges: []string{"35.191.0.0/16", "130.211.0.0/22"},
```

The IPv6 Addresses are the following:

```go
SourceRanges: []string{"2600:2d00:1:b029::/64", "2600:2d00:1:1::/64"}
```

_Note_: A firewall rule cannot have IPv4 and IPv6 Addresses, so multiple rules must be created for dual stack instances.

##### Machines

The Network and AccessConfig values for Machines must be edited based on the stack type.

```go
name := "External NAT"
configType := "ONE_TO_ONE_NAT"
if m.GCPMachine.Spec.StackType == "IPV6_ONLY" {
    name = "External IPv6"
    configType = "DIRECT_IPV6"
}

networkInterface.AccessConfigs = []*compute.AccessConfig{
{
    Type: configType,
    Name: name,
},
```

##### Load Balancers

TODO

### Topology Considerations

#### Hypershift / Hosted Control Planes

Dual stacks are currently supported by Hypershift.

#### Standalone Clusters

This features should cause no resource limitations for Standalone Clusters.

#### Single-node Deployments or MicroShift

This feature should cause no resources limitations in Single Node or MicroShift clusters.

### Implementation Details/Notes/Constraints

#### Install Config

##### AWS

TODO

##### Azure

TODO

##### GCP

The platform section, for GCP, will all contain a field for `stackType`. The `clusterNetwork` and 
`machineNetwork` values in the `networking` section will accept IPv6 Prefix strings.

```yaml
networking:
  clusterNetwork:
    - cidr: 10.128.0.0/14  <<< Accept IPv6 Prefix
      hostPrefix: 23
  machineNetwork:
    - cidr: 10.0.0.0/16  <<< Accept IPv6 Prefix
platform:
  gcp:
    projectID: example-project
    region: us-east1
    stackType: IPV6_ONLY
```

The default stack type will be set to `IPV4_ONLY` to provide backwards compatibility. In the Openshift Installer, the `types` package defines the install-config 
file, and it represents the API offered by the Installer. The API can be consumed by other projects, most 
notably HIVE. This feature should be included in the install-config to allow other parties to utilize the feature. 

### Risks and Mitigations

The ability to use both IPv4 and/or IPv6 addresses does not pose security risks as long as the firewall resources remain consistent. The access to 
the clusters are controlled by the traffic that can leave and enter, so controlling this information ensures
that the cluster security remains the same. See `Limitations`, below, for a current set of risks. 

Security will be reviewed by the Openshift Installer team, because the feature is created and managed by the team.
Specific team members included for each provider include:
- @patrickdillon - Installer
- @sadasu - Installer
- @barbacbd - GCP
- @thvo - AWS
- @jhixson74 - Azure

### Drawbacks

This feature is still considered in Preview. There are several limitations listed in the following section.

#### Limitations

##### AWS

TODO

##### Azure

TODO

##### GCP

- IPv6-only instances support only Debian and Ubuntu operating systems.
- IPv6-only instances don't support Compute Engine Internal DNS.
- VPC Network Peering and Cloud Interconnect VLAN attachments themselves can only be configured as dual-stack and not IPv6-only. However, when configured as dual-stack they are compatible with IPv6-only resources such as subnets and instances.
- For static routes, some next hop types don't support IPv6, and support differs between dual-stack and IPv6-only. For more information, see Next hops and features.
- For NAT64, Public NAT supports second generation or earlier VM instances and M3 VM instances. For more information, see Compute Engine terminology.
- Cloud DNS doesn't support IPv6 for inbound or outbound forwarding.
- IPv6-only support is limited to unmanaged instance group backends and protocol forwarding with IPv6-only target instances.
- Private Service Connect doesn't support IPv6-only subnets for the producer's NAT subnet.

###### Private Networks

If the Openshift Installer creates the VPC, there is no way for the user to enter the correct `machineNetwork` value in the
install config file.

For more information see [source](https://cloud.google.com/blog/products/networking/using-ipv6-unique-local-addresses-or-ula-in-google-cloud).

## Alternatives (Not Implemented)

The cluster remains the same. The cluster administrator can establish a cluster with IPv4 or IPv6 addresses.

## Upgrade / Downgrade Strategy

According to the cloud providers, the type of subnet(s) used in the cluster cannot change after creation. In the
event that the cluster was not established with an IPv6 stack, a new cluster must be established. 

It _may_ be possible to keep specific virtual machines (to prevent data loss) from a previous cluster. After the new
cluster is established with an IPv6 stack, the existing virtual machines could be added to the new cluster.

## Graduation Criteria

### Dev Preview -> Tech Preview

The feature will be marked in the Openshift Installer with a required feature set `techPreview`. The feature set
will require users/administrators explicitly note in the install configuration file that this feature is a 
technical preview and no upgrades are allowed.

### Tech Preview -> GA

The `techPreview` feature will be removed from the Openshift API.

The `techPreview` feature set will be removed from the Openshift Installer. The users/administrators will be able
to use the feature without the previously required feature set tag in the install configuration file.

### Removing a deprecated feature

No deprecated features to be removed.

## Version Skew Strategy

The feature should be released at a single time, so there should be no requirements for skewing the versions 
for release.

The feature does not require removing/deprecating other features, so there should be no requirements for skewing the
versions for release.

## Support Procedures

Support should be initialized with the Openshift Installer team. Noted in the document comments above, the Installer 
team member specializing in the cloud provider with the issue will act as the Point of Contact for the support case.

In the event that the support is resolved by the Installer team, the cloud provider teams and network teams should be
contacted. 

## Open Questions


## Test Plan

