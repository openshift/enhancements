---
title: dual-stack-enablement
authors:
- "@sadasu"
reviewers:
- "@patrickdillon, for Installer aspects"
- "@sadasu, for Installer AWS aspects"
- "@jhixson, for Installer Azure aspects"
- "@barbacbd, for Installer GCP aspects"
approvers:
- "@patrickdillon"
api-approvers:
- "@joelspeed"
creation-date: 2025-01-02
last-updated: 2025-01-03
tracking-link: # link to the tracking tickets 
- https://issues.redhat.com/browse/OCPSTRAT-886 # AWS
- https://issues.redhat.com/browse/OCPSTRAT-887 # Azure
- https://issues.redhat.com/browse/CORS-3526 # GCP
---

# Dual Stack Enablement

## Summary

This enhancement adds the ability for a customer to enable a dual
stack for AWS, GCP, and Azure clusters. This feature will allow clusters to
consist of instances that have IPv6 and IPv4 addresses. 

## Motivation

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

- Enable AWS, GCP, and Azure customers to enable dual stack configuration during
the installation of the cluster.
- Provide users with the ability to determine their stack type post installation.

### Non-Goals

None

### User Stories

- As an Openshift Installer developer, I would like to protect new API fields with a feature 
gate, so I can build features without concern that an incomplete feature would be exposed to customers.

- As a member of Openshift with customers and/or clients, I would like to explain the benefits of dual stack
clusters. I want to explain configuring dual stack clusters will allow long term supportability and flexibility
of our product. This could ensure data and cluster resources are not lost to future compatibility issues as IPv6
becomes the standard. 

- As a customer, I want to create a cluster that provides access to all possible endpoints now and in the future.

## Proposal

1. Openshift API should include a new variable `StackType` that allows the user to select between
single stacks and dual stacks.

2. For each cloud provider (AWS, GCP, and Azure), the `platform` section of the install config should allow
the user to select IPv4_ONLY, IPv6_ONLY, or Dual Stack for the `stackType`. 

3. The cluster api provider for each cloud (AWS, GCP, and Azure) should be updated to allow the correct stack type. 

4. Cluster Manifests need to be altered to include networking details for dual stack clusters.

### Workflow Description

#### Example workflow for feature

1. Developer adds a new enumeration string `stackType` to Openshift API. The new enumeration is tied to features 
for AWS, GCP, and Azure for the Openshift Installer. The features are set as a Teach Preview Feature Set.
2. Developer add a new enumeration string `stackType` to the Openshift Installer. The new enumeration should have
equivalent values to the Openshift API enumeration. This is a Tech Preview feature set feature.
3. The User may now set the `stackType` in the install configuration file. If the user leaves the `featureSet` field
blank, then `openshift-install` returns an `error: the TechPreviewNoUpgrade feature set must be enabled to use this field`
4. Installer validates the values for the enumeration as they must match one of the strings
5. Installer sets correct subnetwork stack type that is passed to the cluster api provider via the `ClusterSpec` structure
6. Cluster API Provider sets the correct stack type for the subnetworks in the cluster
7. The cluster resources are successfully created by the cluster api provider
8. Installer generates FeatureGate manifest
9. Installer emits warning message indicating the enabled feature set when provisioning infrastructure
10. Cluster installs successfully with TechPreview cluster

## Operational Aspects of API Extensions

### API Extensions

#### Openshift

A new feature will be added to the Openshift API for AWS, GCP, and Azure. The feature will make use of the `stackType`
variable. 

```go
	// StackType: The stack type for the subnet. If set to IPV4_ONLY, new VMs in
	// the subnet are assigned IPv4 addresses only. If set to IPV4_IPV6, new VMs in
	// the subnet can be assigned both IPv4 and IPv6 addresses. If not specified,
	// IPV4_ONLY is used. This field can be both set at resource creation time and
	// updated using patch.
	//
	// Possible values:
	//   "IPV4_IPV6" - New VMs in this subnet can have both IPv4 and IPv6
	// addresses.
	//   "IPV4_ONLY" - New VMs in this subnet will only be assigned IPv4 addresses.
	//   "IPV6_ONLY" - New VMs in this subnet will only be assigned IPv6 addresses.
	// +kubebuilder:validation:Enum=IPV4_ONLY;IPV4_IPV6;IPV6_ONLY
	// +kubebuilder:default=IPV4_ONLY
	// +optional
	StackType string `json:"stackType,omitempty"`
```

#### Cluster API Provider

AWS, GCP, and Azure cluster api providers should be updated to include the enumeration to control the stack type.

An example of the GCP changes:

```go
	// StackType: The stack type for the subnet. If set to IPV4_ONLY, new VMs in
	// the subnet are assigned IPv4 addresses only. If set to IPV4_IPV6, new VMs in
	// the subnet can be assigned both IPv4 and IPv6 addresses. If not specified,
	// IPV4_ONLY is used. This field can be both set at resource creation time and
	// updated using patch.
	//
	// Possible values:
	//   "IPV4_IPV6" - New VMs in this subnet can have both IPv4 and IPv6
	// addresses.
	//   "IPV4_ONLY" - New VMs in this subnet will only be assigned IPv4 addresses.
	//   "IPV6_ONLY" - New VMs in this subnet will only be assigned IPv6 addresses.
	// +kubebuilder:validation:Enum=IPV4_ONLY;IPV4_IPV6;IPV6_ONLY
	// +kubebuilder:default=IPV4_ONLY
	// +optional
	StackType string `json:"stackType,omitempty"`
```

The user can set the stack type in the subnetwork structure to be created by the Google Compute API.

```go
subnet := &compute.Subnetwork{
	Name:      subnetwork.Name,
	Region:    subnetwork.Region,
        ...
	StackType: subnetwork.StackType,      << Added feature
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

Dual stacks are currently supported by Hypershift.

#### Standalone Clusters

This features should cause no resource limitations for Standalone Clusters.

#### Single-node Deployments or MicroShift

This feature should cause no resources limitations in Single Node or MicroShift clusters.

### Implementation Details/Notes/Constraints

#### Install Config

The platform section (AWS, GCP, and Azure) will all contain a field for `stackType`.

```yaml
platform:
  gcp:
    projectID: example-project
    region: us-east1
    stackType: IPV4_IPV6
```

The default stack type will be set t `IPV4_ONLY` to provide backwards compatibility. To enable dual stacks, the above
configuration can be used for `stackType` (`IPV4_IPV6`). This feature will be gated on the three major cloud provider
platforms, and not available on the other platforms. 

In the Openshift Installer, the `types` package defines the install config file and it represents the API offered by
the Installer. The API can be consumed by other projects, most notably HIVE. This feature should be included in the
install config to allow other parties to utilize the feature. 

### Risks and Mitigations

Dual stacks are already supported by Kubernetes and Openshift products. The ability to use both IPv4 and
IPv6 addresses does not pose security risks as long as the firewall resources remain consistent. The access to 
the clusters are controlled by the traffic that can leave and enter, so controlling this information ensures
that the cluster security remains the same. 

Security will be reviewed by the Openshift Installer team, because the feature is created and managed by the team.
Specific team members included for each provider include:
- @patrickdillon - Installer
- @sadasu - AWS
- @jhixson - Azure
- @barbacbd - GCP

### Drawbacks

There are no immediate drawbacks to this new feature. The consideration could be made that this takes away the
simplicity of the design of the Openshift Installer. The Openshift Installer should provide an opinionated installation
process. The default case is currently set to IPv4 stacks only, and this feature will not change the default behavior. 

According to the cluster providers, the stack type cannot be altered post installation. This may pose a problem during
an upgrade process where a user/customer would like to change the stack type while keeping their cluster. 

## Alternatives

The cluster remains the same. The cluster administrator can establish a cluster with IPv4 or IPv6 addresses.

## Upgrade / Downgrade Strategy

According to the cloud providers, the type of subnet(s) used in the cluster cannot change after creation. In the
event that the cluster was not established with a dual stack, a new cluster must be established. 

It _may_ be possible to keep specific virtual machines (to prevent data loss) from a previous cluster. After the new
cluster is established with a dual stack, the existing virtual machines could be added to the new cluster.

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

