---
title: aws-lb-subnet-selection
authors:
- "@gcs278"
reviewers:
- "@candita"
- "@frobware"
- "@rfredette"
- "@alebedev87"
- "@miheer"
- "@Miciah"
- "@mtulio"
approvers:
- "@patrickdillon"
api-approvers:
- "@JoelSpeed"
creation-date: 2024-05-29
last-updated: 2025-01-24
tracking-link:
  - https://issues.redhat.com/browse/CORS-3440
see-also:
  - "enhancements/ingress/lb-subnet-selection-aws.md"
replaces:
  - "enhancements/installer/aws-customer-provided-subnets.md"
superseded-by:
---

# Installer LoadBalancer Subnet Selection for AWS

## Summary

This proposal extends the OpenShift Installer's install-config for AWS to enable install-time configuration
of subnets for the `default` IngressController as well as the control plane load balancers and nodes.
It also fixes a common issue where load balancers would map to unintended subnets ([OCPBUGS-17432](https://issues.redhat.com/browse/OCPBUGS-17432)).

To achieve this, this enhancement deprecates the existing install-config field `platform.aws.subnets`
in favor of a more flexible and optional configuration field that handles specifying subnets for IngressControllers,
control plane load balancers, and the cluster nodes. These new subnet roles, referred to as
IngressControllerLB, ControlPlaneExternalLB, ControlPlaneInternalLB, ClusterNode, and EdgeNode subnets
are further defined in [Defining Subnet Roles](#defining-subnets-roles).

## Definitions and Terminology

### The `default` IngressController

This proposal will refer to the IngressController that gets created automatically during installation and handles
the platform routes (console, auth, canary, etc.) as the `default` IngressController. Note that
"default IngressController subnets" refers to the default subnets used by all IngressControllers, including
the `default` IngressController. This is different from "`default`-IngressController subnets", which specifically
means the subnets for the `default` IngressController.

### User-Created IngressControllers

A user-created IngressController is a custom IngressController that is created after installation (Day 2).
It represents all IngressControllers except for the `default` IngressController.

### Defining Subnets Roles

With the install-config now able to specify multiple ways a subnet could be used, it's important
to clarify what these uses are. We will define the function, purpose, or use of a subnet as a "role".
We use the concept of roles to recognize that a single subnet can simultaneously fulfill multiple roles.

Let's define the new subnet roles:

| Role                   | Description                                                                                                                                                                                                                                                                                                                                   | Allowed Subnet Scope                                        | 
|------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------|
| ClusterNode            | Subnets that host the cluster nodes, including both control plane and compute nodes in standard availability zones (excluding local zones).                                                                                                                                                                                                   | Private [^1]                                                |
| EdgeNode               | Subnets that host nodes running in AWS edge zones, Local Zones or Wavelength Zones. These subnets are exclusively used for hosting edge nodes and cannot attach to Control Plane or IngressController load balancers.                                                                                                                         | Public or Private                                           |
| Bootstrap              | Host the bootstrap resources such as a temporary machine booted to act as a temporary control plane whose sole purpose is launching the rest of the cluster ([reference](https://github.com/openshift/installer/blob/main/docs/user/overview.md#cluster-installation-process)).                                                               | Public or Private                                           |
| IngressControllerLB    | Subnets that host AWS load balancer created specifically for the `default` IngressController. This role does not include load balancers created from generic LoadBalancer-type Services. Defaulting for user-created IngressControllers is not included for this role; however, this role may be extended in the future include such support. | Public for External Clusters, Private for Internal Clusters |
| ControlPlaneExternalLB | Subnets that host the control plane's **external** load balancer that serves the Kubernetes API server. Only created for externally published clusters.                                                                                                                                                                                       | Public                                                      |
| ControlPlaneInternalLB | Subnets that host the control plane **internal** load balancers that serves the Kubernetes API server. Always created for all clusters.                                                                                                                                                                                                       | Private                                                     |
[^1]: Can be public for OPENSHIFT_INSTALL_AWS_PUBLIC_ONLY

_Note: These roles are defined only to establish a common language within the context of the install-config
and may not be applicable to other environments, situations, or APIs._

### Clarifying Subnet Roles Terminology

For simplicity, the term "<Role> subnets" (e.g., "ClusterNode subnets") refer to subnets that fulfill their respective
roles, but may also serve other roles. Additionally, this proposal will use "ControlPlaneLB subnets" to mean both
ControlPlaneExternalLB and ControlPlaneInternalLB subnets; however "ControlPlaneLB" is NOT an official subnet role
in the API.

When this proposal refers to a "IngressController subnet", it is generically referring to a subnet that associated with
an IngressController's load balancer, not specifically a subnet that was specified with the IngressControllerLB role
in the install-config, as these can differ. However, this proposal will avoid using the term "cluster subnet",
due to its ambiguity; it is unclear whether it refers to a ClusterNode, IngressControllerLB, or ControlPlaneLB subnet.

## Motivation

Cluster admins using AWS may have dedicated subnets for their load balancers due to
security reasons, architecture, or infrastructure constraints. They may be installing
a cluster into a VPC with multiple Availability Zones (AZs) containing untagged subnets, but
they wish to restrict their cluster to a single AZ.

Currently, cluster admins can configure their IngressController subnets after installation (Day 2) by configuring
the `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets`
or `...aws.networkLoadBalancer.subnets` fields (see [LoadBalancer Subnet Selection for AWS](/enhancements/ingress/lb-subnet-selection-aws.md)).
However, there is no way to explicitly configure the ControlPlaneLB subnets. Cluster admins also need a way to
configure both ControlPlaneLB and IngressControllerLB subnets at install time (Day 1) using the install-config.

Configuring subnets at install time is essential for cluster admins that want to
ensure their load balancer subnets are configured properly from the start. Additionally,
Service Delivery has use cases for ROSA Hosted Control Plane (HCP) that require the `default`
IngressController and ControlPlaneLB subnets to be configured during cluster installation.

### AWS Subnet Discovery

AWS subnet discovery is the mechanism used by the AWS Cloud Controller Manager (CCM) to determine the Elastic Load
Balancer (ELB) subnets created for LoadBalancer-type Services. According to [kubernetes/kubernetes#97431](https://github.com/kubernetes/kubernetes/pull/97431),
a subnet will **NOT** be selected if:

- The `kubernetes.io/cluster/<cluster-id>` tag contains the cluster ID of another cluster.
- The load balancer is external (internet facing) and the subnet is private.
- A subnet for the same AZ has already been chosen based on lexicographical order.
- The subnet type is a `wavelength` or `local`.

_Note: Subnet discovery, as defined here, applies only to IngressControllerLB subnets and not to ControlPlaneLB subnets.
While the Cluster API Provider AWS ([CAPA](https://github.com/kubernetes-sigs/cluster-api-provider-aws)) does some subnet
filtering, it relies on the list of [provided subnets](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/8f46a4d34d18cb090a4b48c9a9260cd3dffd9162/api/v1beta2/network_types.go#L340)
from the openshift-installer. As a result, ControlPlaneLB subnets are NOT impacted by [OCPBUGS-17432](https://issues.redhat.com/browse/OCPBUGS-17432)._

However, the current subnet discovery implementation has some limitations:

- It selects subnets that lack the `kubernetes.io/cluster/<cluster-id>` tag ([OCPBUGS-17432](https://issues.redhat.com/browse/OCPBUGS-17432))
- It doesn't support use cases such as [Dedicated Load Balancer Subnets User Story](#dedicated-load-balancer-subnets-user-story)

The goal of this enhancement is not to completely bypass or replace subnet discovery by hardcoding the
subnet selection. Subnet discovery offers a more flexible solution for dynamic AZ scaling (only for CLBs)
and aligns with standard workflows and expectations for AWS customers. Instead, this proposal's goal is to allow
users to choose between subnet discovery and explicit subnet selection while introducing new guardrails for
subnet discovery.

### User Stories

#### Dedicated Load Balancer Subnets User Story

_"As a cluster admin, I want to install a cluster into an existing VPC and
configure all of my load balancers to use dedicated subnets so that I can fulfill
my organization's requirements for security, observability, and network segmentation."_

Some enterprise customers have strict requirements to isolate subnets used for their
load balancers. They set up a self-managed VPC with dedicated subnets for load balancers
and require OpenShift to be installed into this configuration. Users may need to isolate
ControlPlaneLB subnets from IngressControllerLB subnets, resulting in multiple sets of dedicated subnets.

See [Specifying Manual Subnet Roles with BYO VPC during Installation Workflow](#specifying-manual-subnet-roles-with-byo-vpc-during-installation-workflow)
for the workflow for this user story.

#### Do Not Use Untagged Subnets User Stories

1. _"As a cluster admin, I want to install a cluster into an existing VPC that contains
other untagged subnets and I want my IngressControllers to be restricted to the cluster
subnets, so that my load balancers are not unnecessarily mapped to an AZ they aren't
intended for."_
2. _"As a cluster admin, I want to install a private cluster into an existing VPC that
contains other untagged subnets and I want my IngressControllers to be restricted to
private subnets, so that it is not exposed to the public Internet."_

When a cluster admin installs a cluster into an existing VPC with additional untagged
subnets that are selected by [AWS Subnet Discovery](#aws-subnet-discovery), IngressControllers may
inadvertently use these other subnets, which might be allocated to other cluster or purposes
(see related [RFE-2816](https://issues.redhat.com/browse/RFE-2816) and [OCPBUGS-17432](https://issues.redhat.com/browse/OCPBUGS-17432)).
Allowing load balancers to access unintended subnets can lead to bugs and pose security risks.

_Note: ControlPlaneLB subnets are NOT impacted by this user story because they do not use
[AWS Subnet Discovery](#aws-subnet-discovery)._

If explicitly specifying subnets, this use case will be addressed by the
[Specifying Manual Subnet Roles with BYO VPC during Installation Workflow](#specifying-manual-subnet-roles-with-byo-vpc-during-installation-workflow).
If using subnet discovery, this use case will be automatically resolved, as the [proposal](#proposal)
is to reject untagged subnets in new installations.

#### ROSA Installation User Story

_"As a ROSA Engineer, I want the ability to specify my load balancer subnets
in the install-config so that I can accommodate installer mechanisms (e.g., Hive) that
don’t support customizing installer manifests."_

Though users can currently [customize](https://github.com/openshift/installer/blob/master/docs/user/customization.md#kubernetes-customization-unvalidated)
the `default` IngressController and ControlPlaneLB subnets via installer manifests, this customization is not supported
for ROSA installations (Classic and HCP). This use case drives the need for an install-config API.

#### Public ClusterNode Subnets User Story

_"As an OpenShift Test Platform Engineer, I want the ability to assign my nodes to use public subnets
in the install-config so that I can use NAT-less gateways for significant cost savings in cases
where node security is not a concern (e.g, ephemeral clusters in CI)."_

Adapted from [openshift/installer#6342](https://github.com/openshift/installer/pull/6342).
This user story ensures that the functionality of the existing `OPENSHIFT_INSTALL_AWS_PUBLIC_ONLY`
environment variable is preserved. See
[Specifying Manual Subnet Roles with BYO VPC during Installation Workflow](#specifying-manual-subnet-roles-with-byo-vpc-during-installation-workflow)
for the workflow for this use case.

#### Cluster Nodes on Different Private Subnets User Story

_"As a cluster admin, I want to install an internal cluster into an existing VPC with AWS
Direct Connect where my cluster nodes use private subnets with non-routable IPs and my load
balancers use private subnets with routable IPs, so that I can conserve valuable routable IPs."_

AWS clusters that connect to a customer network via AWS Direct Connect (or similar mechanism)
require their load balancers to use internally routable IPs. However, internally routable IPs
may be limited, and placing cluster nodes (compute and worker) on the same subnet with routable
IPs as the load balancers unnecessarily consumes these limited IPs. Allowing customers to separate
non-routable and routable private subnets for compute nodes and load balancer saves valuable IPs.
See [RFE-4738](https://issues.redhat.com/browse/RFE-4738).

See [Specifying Manual Subnet Roles with BYO VPC during Installation Workflow](#specifying-manual-subnet-roles-with-byo-vpc-during-installation-workflow)
for the workflow for this use case.

### Goals

- Deprecate the `platform.aws.subnets` field in the install-config and add a
  new, more flexible subnet field for subnet-related configuration.
- Enable users to explicitly configure the subnets for the `default` IngressController on AWS
  through the install-config (i.e. IngressControllerLB subnets).
- Enable users to explicitly configure the subnets for the ControlPlaneLB subnets.
- Enable users to explicitly configure the subnets for the cluster nodes, both ClusterNode and EdgeNode.
- Enable users to explicitly configure the subnets for the bootstrap machine.
- Enable users to still use AWS Subnet Discovery for the `default` IngressController.
- Prevent installations into existing VPCs where AWS Subnet Discovery would select subnets that do not belong to the cluster.
- Support automatic subnet role selection (opinionated defaults) similar to the existing `platform.aws.subnets` behavior.
- Provide install-time validation for all user-provided subnets roles.
- Maintain support for configuring the `default` IngressController or ControlPlaneLB subnets via customizing the
  installer manifest.

### Non-Goals

- Extend support to platforms other than AWS.
- Support install-time subnet configuration for LoadBalancer-type Services that aren't associated with an IngressController.
- Support defaulting subnets for user-created IngressControllers to the IngressControllerLB subnet.
- Remove the `platform.aws.subnets` install-config field without requiring an install-config version bump.
- Split ClusterNode into ControlPlaneNodes and ComputeNode roles.
- Support mixing automatic and manually specified roles.

## Proposal

To enable cluster admins to specify subnets for IngressControllers and ControlPlaneLBs at install time, we will
deprecate the `platform.aws.subnets` field in the install-config and add a new `platform.aws.vpc.subnets` field,
allowing users to explicitly manage subnet roles. Additionally, the installer will reject untagged subnets when
using the new `platform.aws.vpc.subnets` field.

### InstallConfig API Proposal

The proposal to deprecate the `platform.aws.subnets` field in the install-config is driven by
its ambiguity, it does not distinguish between subnet roles. Additionally, `platform.aws.subnets` is a `[]string` and doesn't
allow for future expansion or adaptation for other subnet type, roles, or metadata information. The new 
install-config field that will enable this feature will be `platform.aws.vpc.subnets`.

#### Automatic vs. Manual Role Selection

Automatic role selection is when the installer, CAPA, AWS CCM, or other components automatically determine how the
provided subnets are used. Currently, the installer performs automatic role selection with the `platform.aws.subnets`
field, as it does not provide a way to override these decisions. The new `platform.aws.vpc.subnets` field will
continue to support automatic role selection for users who prefer a standard, opinionated default cluster subnet
config, but only if no roles are explicitly assigned to any subnet. For IngressControllers, using automatic role
selection enables the AWS CCM to use [AWS Subnet Discovery](#aws-subnet-discovery) as it did previously with
`platform.aws.subnets`. For ControlPlaneLBs and ClusterNodes, the subnets are selected based on whether they are
public or private and whether the cluster is configured as internal or external. Refer to the table below for more
details on auto-selection.

Manual role selection is when the cluster admin assigns roles to the provided subnets, and the installer
explicitly configures the subnets to be used according to the cluster admin's role selection. Every subnet must have
a role, and the roles labeled as "Required in Manual Mode" in the table below must be assigned at least one subnet:

| Role                   | Required in Manual Mode   | Auto-Selection Component | Auto-Selected Subnets Type[^1]                             |
|------------------------|---------------------------|--------------------------|------------------------------------------------------------|
| ClusterNode            | Yes                       | Installer                | Private[^2]                                                |
| EdgeNode               | No                        | Installer                | Local or Wavelength Zone                                   |
| Bootstrap              | Yes                       | Installer                | Public or Private                                          |
| IngressControllerLB    | Yes                       | AWS CCM                  | Public for External Cluster, Private for Internal Clusters |
| ControlPlaneExternalLB | Yes (if external cluster) | Installer (CAPA)         | Public                                                     |
| ControlPlaneInternalLB | Yes                       | Installer (CAPA)         | Private                                                    |
[^1]: Describes what type of subnets will be selected for this role in automatic role selection.
[^2]: Public for OPENSHIFT_INSTALL_AWS_PUBLIC_ONLY


### Rejecting Untagged Subnets Proposal

In BYO VPCs, untagged subnets are nuisance as the AWS CCM may select them, leading to various bugs, RFEs, and
support cases. Since this proposal introduces a new subnet install-config field, it's a great opportunity
to introduce a new behavior which will provide a better user experience. If `platform.aws.vpc.subnets`
is using automatic role selection (i.e. subnet discovery), the installer will inspect the VPC for untagged
subnets not already specified in `platform.aws.vpc.subnets` and reject the installation if any exist.

It's important to note that while the installer will reject additional untagged subnets during installation,
a cluster admin can still add untagged subnets after installation (Day 2), which may cause IngressControllers
to automatically use these new subnets. This proposal does not attempt to prevent this type of misconfiguration.

### Implementation Details/Notes/Constraints

As mentioned in [the Proposal section](#proposal), the install-config and the installer will be updated
to support propagating explicit subnet selection to the `default` IngressController, the control plane load balancers,
and nodes.

#### Installer Updates

The `platform.aws.subnets` field in the install-config will be deprecated and replaced with a new
field, `platform.aws.vpc.subnets`. Much like the old `subnets` field, the new `subnets` field indicates
a list of subnets in a pre-existing VPC, but also can optionally specify the roles that the subnet will fulfill in the
cluster. Additionally, since it is a struct and not a `[]string`, it provide the ability to be
expanded with additional subnet-related fields in the future.

The following install-config reflect the initial TechPreview release of this feature:

_Note: The install-config does not currently handle CEL validation, but the following suggestions
use CEL simply as guidelines for the desired validation that the installer needs to implement._ 

```go
// Platform stores all the global configuration that all machinesets
// use.
// +kubebuilder:validation:XValidation:rule=`!(has(self.subnets) && has(self.vpc.subnets))`,message="cannot use both subnets and vpc.subnets"
type Platform struct {
    //...
    // subnets specifies existing subnets (by ID) where cluster
    // resources will be created.  Leave unset to have the installer
    // create subnets in a new VPC on your behalf.
    //
    // Deprecated: Use platform.aws.vpc.subnets
    //
    // +optional
    Subnets []string `json:"subnets,omitempty"`

    // vpc specifies the VPC configuration.
    //
    // +optional
    VPC VPCSpec `json:"vpc,omitempty"`
}

// VPCSpec configures the VPC for the cluster.
type VPCSpec struct {
    // subnets defines the subnets in an existing VPC
    // and can optionally specify their intended roles for use by the installer.
    // If no roles are specified on any subnet, then the subnet roles
    // are decided automatically. In this case, the VPC must not contain
    // any subnets without the kubernetes.io/cluster/<cluster-id> tag.
    //
    // For manually specified subnet role selection, each subnet must have at
    // least one assigned role, and the ClusterNode, IngressControllerLB,
    // ControlPlaneExternalLB, and ControlPlaneInternalLB roles must be assigned
    // to at least one subnet. However, if the cluster scope is internal,
    // then ControlPlaneExternalLB is not required.
    // 
    // Leave this field unset to have the installer create subnets
    // in a new VPC on your behalf. subnets must contain unique IDs,
    // must not exceed a total of 40 subnets, and can include no more than
    // 10 subnets with the IngressController role.
    // subnets is only available in TechPreview.
    //
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x.id == y.id))`,message="subnets cannot contain duplicate IDs" 
    // +kubebuilder:validation:XValidation:rule=`self.exists(x, x.roles.exists(r, r.type == 'ClusterNode'))`,message="subnets must contain at least 1 subnet with the ClusterNode role"
    // +kubebuilder:validation:XValidation:rule=`self.filter(x, x.roles.exists(r, r.type == 'IngressControllerLB')).size() <= 10`,message="subnets must contain less than 10 subnets with the IngressControllerLB role"
    // +kubebuilder:validation:MaxLength=50
    Subnets []Subnet `json:"subnets,omitempty"`
}

type Subnet struct {
    // id specifies the subnet ID of an existing subnet.
    // The subnet id must start with "subnet-", consist only
    // of alphanumeric characters, and must be exactly 24
    // characters long.
    //
    // +required
    ID AWSSubnetID `json:"id"`

    // roles specifies the roles (aka functions)
    // that the subnet will provide in the cluster. If no roles are
    // specified on any subnet, then the subnet roles are decided
    // automatically. Each role must be unique.
    //
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="roles cannot contain duplicates"
    // +kubebuilder:validation:XValidation:rule=`!self.exists(r, r.type == 'EdgeNode') || self.size() == 1`,message="EdgeNode roles cannot be combined with any other roles"
    // +kubebuilder:validation:XValidation:rule=`self.filter(r, r.type == 'ControlPlaneExternalLB' || r.type == 'ControlPlaneInternalLB').size() < 2`,message="roles cannot contain both ControlPlaneExternalLB and ControlPlaneInternalLB"
    // +kubebuilder:validation:MaxLength=5
    Roles []SubnetRole `json:"roles"`
}

// AWSSubnetID is a reference to an AWS subnet ID.
// +kubebuilder:validation:MinLength=24
// +kubebuilder:validation:MaxLength=24
// +kubebuilder:validation:Pattern=`^subnet-[0-9A-Za-z]+$`
type AWSSubnetID string

// SubnetRole specifies the role (aka functions) that the subnet will provide in the cluster.
type SubnetRole struct {
    // type specifies the type of role (aka function)
    // that the subnet will provide in the cluster.
    // Role types include ClusterNode, EdgeNode, Bootstrap,
    // IngressControllerLB, ControlPlaneExternalLB, and
    // ControlPlaneInternalLB.
    //
    // +required
    Type SubnetRoleType `json:"type"`
}

// SubnetRoleName specifies the name of the roles (aka functions) that the subnet will provide in the cluster.
type SubnetRoleType string

const (
    // ClusterNodeSubnetRole specifies subnets that will be used as subnets for the
    // control plane and compute nodes.
    ClusterNodeSubnetRole SubnetRoleType = "ClusterNode"

    // EdgeNodeSubnetRole specifies subnets that will be used as edge subnets residing
    // in Local or Wavelength Zones for edge compute nodes.
    EdgeNodeSubnetRole SubnetRoleType = "EdgeNode"

    // BootstrapSubnetRole specifies subnets that will be used as subnets for the
    // bootstrap node used to create the cluster.
    BootstrapSubnetRole SubnetRoleType = "Bootstrap"

    // IngressControllerLBSubnetRole specifies subnets used by the default IngressController.
    IngressControllerLBSubnetRole SubnetRoleType = "IngressControllerLB"

    // ControlPlaneExternalLBSubnetRole specifies subnets used by the external control plane
    // load balancer that serves the Kubernetes API server.
    ControlPlaneExternalLBSubnetRole SubnetRoleType = "ControlPlaneExternalLB" 

    // ControlPlaneInternalLBSubnetRole specifies subnets used by the internal control plane
    // load balancer that serves the Kubernetes API server.
    ControlPlaneInternalLBSubnetRole SubnetRoleType = "ControlPlaneInternalLB"
)
```

The `platform.aws.vpc.subnets` field will be introduced under the TechPreview feature gate, and `platform.aws.subnets`
will be deprecated when `platform.aws.vpc.subnets` graduates to GA.

##### Installer Validation Rules

These validation rules only apply to the new `platform.aws.vpc.subnets` field. These validations aim to improve
user experience, but are not permanent and may be updated or removed to accommodate future use cases.

###### All or Nothing Subnet Roles Selection

This proposed API follows an "all or nothing" approach for roles: either all required roles (see
[Automatic vs. Manual Role Selection](#automatic-vs-manual-role-selection) for list) must be explicitly assigned
(manual role selection), or no roles must be assigned (automatic role selection). The installer must not allow a mix of
automatic and manual role assignments, as this could lead to confusing behavior. See [Allowing Mixed Automatic and Manual Subnet Selection Alternative](#allowing-mixed-automatic-and-manual-subnet-selection-alternative) for the reason behind this validation.

###### All Subnets Belong to the Same VPC Validation

The [existing validation](https://github.com/openshift/installer/blob/0d77aa8df5ddc68e926aa11da24a87981021b256/pkg/asset/installconfig/aws/subnet.go#L91)
for the deprecated `platform.aws.subnets` field ensuring all subnets belong to the same VPC
should apply to all subnets specified in `platform.aws.vpc.subnets`.

###### Multiple Load Balancer Subnets in the Same AZ Validation

The installer must reject multiple IngressControllerLB subnets in the same AZ as this will be
rejected by the AWS CCM.

###### The Old and New Subnets Fields Cannot be Specified Together Validation

Since `platform.aws.subnets` will be deprecated, the installer must not allow `platform.aws.vpc.subnets` and
`platform.aws.subnets` to both be specified.

###### Maximum of 10 IngressController Subnets Validation

Since the IngressController's API [only allows 10 subnets](https://github.com/openshift/api/blob/ee5bb7eaf6b6638d4e3b33ba4ff0834212cdb75d/operator/v1/types_ingress.go#L564),
the installer must not allow more than 10 IngressController subnets as well.

###### Consistent Cluster Scope with IngressControllerLB Subnets Validation

The installer must not allow **any** public IngressControllerLB subnets for private clusters (`installconfig.publish: Internal`)
as this is a security risk as discussed in the [Do Not Use Untagged Subnets User Stories](#do-not-use-untagged-subnets-user-stories) section.

Conversely, the installer must now allow **any** private IngressControllerLB subnets for public clusters
(`installconfig.publish: External`) as the public IngressController will not function with private subnets.

###### Consistent Cluster Scope with ControlPlaneLB Subnets Validation

The installer must not allow **any** public subnets to be specified with the ControlPlaneInternalLB role and must
not allow **any** private subnets to be specified with the ControlPlaneExternalLB role. As a result,
ControlPlaneInternalLB and ControlPlaneExternalLB are mutually exclusive roles.

Private clusters (`installconfig.publish: Internal`) should reject an install-config containing
any ControlPlaneExternalLB roles, as only an internal control plane load balancer will be created.

###### Reject BYO VPC Installations that Contain Untagged Subnets

As described in [Rejecting Untagged Subnets Proposal](#rejecting-untagged-subnets-proposal), the installer
must not allow installation into an existing VPC if it contains subnets lacking the `kubernetes.io/cluster/<cluster-id>`
tag when subnets roles are not specified. The error message should instruct the user to add a
`kubernetes.io/cluster/unmanaged` tag to any subnets they wish to exclude from this cluster installation or
configure the subnets with the Cluster or IngressController role in `platform.aws.vpc.subnets`.

###### Reject IngressControllers or ControlPlaneLB AZs that do not match ClusterNode AZs

When manual role selection is specified, the installer must ensure that the AZs for the IngressController
(defined by the IngressControllerLB subnets) and control plane (defined by the ControlPlaneExternalLB and
ControlPlaneInternal subnets) match the cluster AZs (defined by the ClusterNode subnets). The installer must
reject installations where the AZs are not equal.

AWS load balancers will NOT register a node located in an AZ that is not enabled. As a result, if the
nodes use an AZ that is not a load balancer AZ, the router pod might be scheduled to  node that the load balancer
cannot reach. Conversely, if the load balancer includes an AZ that is not a node AZ, there will be no nodes in that
AZ for the load balancer to register.

Additionally, the AZs provided in `controlPlane.platform.aws.zones` and `compute.platform.aws.zones` must be
AZs provided by the subnets in `platform.aws.vpc.subnets`. This validation exists for `platform.aws.subnets`
in [`validateMachinePool`](https://github.com/openshift/installer/blob/6fd2928b4f810c0592c042acb6b90028a8a3d6a6/pkg/asset/installconfig/aws/validation.go#L314).

###### Reject Duplicate AZs

The installer should ensure that each role does not contain multiple subnets from the same AZ, regardless of
automatic or manual role selection. This is implemented with `platform.aws.subnets` in the
`validateDuplicateSubnetZones` function.

###### Edge Subnet Restrictions

The installer must not allow any edge subnets (local or wavelength zone) to be specified with the ClusterNode,
ControlPlaneLB, or IngressControllerLB roles. Additionally, the EdgeNode role must be restricted to edge subnets only.
EdgeNode subnets can be public or private to support the existing [workflow](https://docs.openshift.com/container-platform/latest/installing/installing_aws/ipi/installing-aws-localzone.html#installing-with-edge-node-public_installing-aws-localzone).

##### Installer-Generated Manifests Updates

The installer will apply the specified IngressControllerLB subnets to the `default` IngressController's
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets` or
`...networkLoadBalancer.subnets` field (based on `platform.aws.lbType`) in the manifest generated by
the `generateDefaultIngressController` function.


##### Installer ControlPlaneLB Subnets Configuration Updates

The installer will configure the ControlPlaneInternalLB subnets in the `spec.controlPlaneLoadBalancer.subnets`
field of the CAPA AWSCluster object, while the ControlPlaneExternalLB subnets will be set in the
`spec.secondaryControlPlaneLoadBalancer.subnets` field.

##### Installer ClusterNode Subnets Configuration Updates

The installer will configure the ClusterNode subnets in each of the `spec.subnet` fields of the CAPA AWSMachine
objects for the control plane nodes and in the subnet field of the MachineSets for the compute nodes.

##### Installer Subnet Tagging

All public and private subnets specified in the `platform.aws.vpc.subnets` field, regardless of their role, must be
tagged with the `kubernetes.io/cluster/<cluster-id>: shared` tag. Currently, edge subnets are not tagged by the
installer and should continue to be excluded unless the resolution of [OCPBUGS-48827](https://issues.redhat.com/browse/OCPBUGS-48827)
indicate otherwise. These tags assist the AWS CCM in identifying which subnets to use
during [AWS Subnet Discovery](#aws-subnet-discovery) for both the cluster being installed and any other clusters
that may be installed within the same VPC.

##### Installer Subnet Deprecated API Conversion

As `platform.aws.subnets` is being deprecated, the existing upconversion process should convert it into
`platform.aws.vpc.subnets` to maintain backwards compatibility with the old API. Caution must be taken to
ensure that any new behaviors introduced in the new API, such as
[Reject BYO VPC Installations that Contain Untagged Subnets](#reject-byo-vpc-installations-that-contain-untagged-subnets),
are not inadvertently applied to the old API.

### Workflow Description

#### Specifying Manual Subnet Roles with BYO VPC during Installation Workflow

A cluster admin may prefer to specify the subnet roles to gain full control over their usage.
For example, they may want to specify the subnets for the `default` IngressController during
installation rather than relying on [AWS Subnet Discovery](#aws-subnet-discovery).
In order to do this, they must assign all the subnet roles:

1. Cluster admin creates an install-config with `platform.aws.vpc.subnets` specified
   with subnets that manually specify the subnet roles:
    ```yaml
    apiVersion: v1
    baseDomain: devcluster.openshift.com
    metadata:
      name: my-cluster
    #...
    platform:
      aws:
        region: us-east-1
        vpc:
          subnets:
          - id: subnet-0fcf8e0392f0910d0 # public / us-east-1a
            roles:
            - type: IngressControllerLB
            - type: ControlPlaneLB
            - type: Bootstrap
          - id: subnet-0fcf8e0392f0910d1 # private / us-east-1a
            roles:
            - type: ControlPlaneLB
            - type: ClusterNode
        lbType: Classic
    #...
    ```
2. The OpenShift Installer uses the subnets based on their specified roles, configuring the ControlPlaneExternalLB,
   ControlPlaneInternalLB, and ClusterNode subnets in the appropriate CAPA objects. For IngressControllerLB subnets,
   it populates the `default` IngressController manifest with them and installs the cluster into the existing VPC:
   ```yaml
   apiVersion: operator.openshift.io/v1
   kind: IngressController
   metadata:
     name: default
     namespace: openshift-ingress-operator
   spec:
     #...
     endpointPublishingStrategy:
       type: LoadBalancerService
       loadBalancer:
         scope: External
         providerParameters:
           type: AWS
           aws:
             type: Classic
             classicLoadBalancer:
               subnets:
                 ids:
                 - subnet-0fcf8e0392f0910d0
     #...
   ```
3. The ControlPlaneLBs, ClusterNodes, and the `default` IngressControllers are created using
   subnets assigned to their respective roles.

**Variations:**

- Dedicated IngressControllerLB and ControlPlaneLB subnets which enable cluster admins to isolate
  their load balancer subnets from the node subnets:
  ```yaml
  #...
      vpc:
        subnets:
        - id: subnet-0fcf8e0392f0910d4 # Private / AZ us-east-1a
          roles:
          - type: ClusterNode
        - id: subnet-0fcf8e0392f0910d5 # Public / AZ us-east-1a
          roles:
          - type: IngressControllerLB
          - type: Bootstrap
        - id: subnet-0fcf8e0392f0910d6 # Public / AZ us-east-1a
          roles:
          - type: ControlPlaneExternalLB
          - type: Bootstrap
        - id: subnet-0fcf8e0392f0910d7 # Private / AZ us-east-1a
          roles:
          - type: ControlPlaneInternalLB
  #...
  ```

- Dedicated IngressControllerLB and ControlPlaneLB subnets, but ControlPlaneInternalLB is shared with ClusterNode:
  ```yaml
  #...
      vpc:
        subnets:
        - id: subnet-0fcf8e0392f0910d4 # Private / AZ us-east-1a
          roles:
          - type: ClusterNode
          - type: ControlPlaneInternalLB
        - id: subnet-0fcf8e0392f0910d5 # Public / AZ us-east-1a
          roles:
          - type: IngressControllerLB
          - type: Bootstrap
        - id: subnet-0fcf8e0392f0910d6 # Public / AZ us-east-1a
          roles:
          - type: ControlPlaneExternalLB
          - type: Bootstrap
  #...
  ```
- ClusterNodes on public subnets so that they are externally accessible:
  ```yaml
  #...
      vpc:
        subnets:
        - id: subnet-0fcf8e0392f0910d4 # Private / AZ us-east-1a
          roles:
          - type: ControlPlaneInternalLB
        - id: subnet-0fcf8e0392f0910d5 # Public / AZ us-east-1a
          roles:
          - type: ClusterNode
          - type: IngressControllerLB
          - type: ControlPlaneExternalLB
          - type: Bootstrap
  #...
  ```
- Cluster installation with EdgeNodes:
  ```yaml
  #...
      vpc:
        subnets:
        - id: subnet-0fcf8e0392f0910d4 # Private / AZ us-east-1a
          roles:
          - type: ClusterNode
          - type: ControlPlaneInternalLB
        - id: subnet-0fcf8e0392f0910d5 # Public / AZ us-east-1a
          roles:
          - type: IngressControllerLB
          - type: ControlPlaneExternalLB
          - type: Bootstrap
        - id: subnet-0fcf8e0392f0910e0 # Private / Local Zone us-east-1-dfw-1a
          roles:
          - type: EdgeNode
  #...
  ```


#### Using Automatic Subnet Roles with BYO VPC during Installation Workflow

A cluster administrator may prefer to have automatically assigned subnet roles, similar to the behavior of
`platform.aws.subnets`, especially if they have no specific requirements for a unique or dedicated
subnet configuration.

1. Cluster admin creates an install-config with `platform.aws.vpc.subnets` specified
   with the subnet(s) while ensuring `platform.aws.vpc.subnets[].roles` is left empty:
    ```yaml
    apiVersion: v1
    baseDomain: devcluster.openshift.com
    metadata:
      name: my-cluster
    #...
    platform:
      aws:
        region: us-east-1
        vpc:
          subnets:
          - id: subnet-0fcf8e0392f0910d2 # Private / AZ us-east-1a
          - id: subnet-0fcf8e0392f0910d3 # Public / AZ us-east-1a
    #...
    ```
2. The OpenShift Installer installs the cluster into the VPC where the subnets exist and automatically determines
   the role or purpose of each subnet. For IngressControllers, no specific subnet is explicitly assigned so that
   the AWS CCM will use [AWS Subnet Discovery](#aws-subnet-discovery) when creating the load balancer for the
   `default` IngressController.

### API Extensions

This proposal doesn't add any API extensions.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is directly enabling the [XCMSTRAT-545](https://issues.redhat.com/browse/XCMSTRAT-545)
feature for ROSA Hosted Control Planes by allowing users to install a single AZ cluster into an
existing VPC with other untagged subnets. Users will be able to explicitly configure their subnets
at install time so that the `default` IngressController doesn't attach to additional subnets in other AZs.

#### Standalone Clusters

This proposal does not have any special implications for standalone clusters.

#### Single-node Deployments or MicroShift

This proposal does not have any special implications for single-node
deployments or MicroShift.

### Risks and Mitigations

- **Risk**: Using public IngressControllerLB subnets for private clusters.
  - **Mitigation**: We will reject private cluster installations using public IngressControllerLB subnets (see
    [Consistent Cluster Scope with IngressControllerLB Subnets Validation](#consistent-cluster-scope-with-ingresscontrollerlb-subnets-validation))
  - **Mitigation**: For private cluster installations using subnet discovery for IngressControllers, public subnets will be excluded from the 
    `default` IngressControllers, provided the subnets are correctly tagged.
- **Risk**: Using public ControlPlaneInternalLB subnets which would expose the internal control plane API load balancer.
  - **Mitigation**: We will reject using public ControlPlaneInternalLB subnets (see
    [Consistent Cluster Scope with ControlPlaneLB Subnets Validation](#consistent-cluster-scope-with-controlplanelb-subnets-validation)).

### Drawbacks

- Deprecating `platform.aws.subnets` will be painful for users.
- Distinguishing between BYO subnets/VPC and installer-created subnets/VPC may be confusing if we need
  to expand the `platform.aws.vpc.subnets` field in the future to accommodate installer-created
  subnets configuration.
  - For example, there may be a future desire to specify the IngressController role for installer-created subnets,
    and it is unclear how this API design would accommodate that.
  - One solution is to make `platform.aws.vpc.subnets[].id` optional, and if `platform.aws.vpc.subnets[].availabilityZone`
    is specified, then the installer would assume a managed VPC while using the provided subnet roles or configuration.
- The all-or-nothing approach in [Automatic vs. Manual Role Selection](#automatic-vs-manual-role-selection) prevents
  users from specifying only a subset of roles to be automatically determined.
  - For example, a cluster admin might want to isolate the ClusterNode subnet from the ControlPlaneInternalLB subnet,
    but leave the IngressControllerLB subnets unspecified to rely on subnet discovery for the `default`
    IngressController. This combination is not currently supported.
  - While it may be possible to support mixed automatic and manual role selection in the future, the semantics of
    that configuration remain unclear (see
    [Allowing Mixed Automatic and Manual Subnet Selection Alternative](#allowing-mixed-automatic-and-manual-subnet-selection-alternative)).

## Open Questions

- Q: Should the install-config design assume all IngressControllerLB subnets are ClusterNode subnets too?
  - A: No, we must support dedicated IngressController subnet use cases.
- Q: When using manual role selection, what subnet does the bootstrap node use? Do we need another role?
  - A: We do not want a separate role for the bootstrap node, as it is an implementation detail. It should
    function as it does with `platform.aws.subnets`, using the provided public subnets for external clusters
    and the provided private subnets for internal clusters.
- Q: Should we require the `OPENSHIFT_INSTALL_AWS_PUBLIC_ONLY` flag to be set when using `platform.aws.vpc.subnets`
  to assign ClusterNode roles to public subnets? Or should we disallow public ClusterNode subnets, and only enable
  `OPENSHIFT_INSTALL_AWS_PUBLIC_ONLY` functionality in automatic role selection mode? Or a combination of both?
  - A: We should disallow public ClusterNode and only allow when `OPENSHIFT_INSTALL_AWS_PUBLIC_ONLY` is set.

## Test Plan

### Installer Testing

E2E test(s) will also be added to the installer to verify functionality of the new [Installer API](#installer-updates).
These tests, typically written by QE, will follow existing patterns established for testing installer functionality in
the [openshift-tests-private](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/openshift-tests-private)
directory of the openshift/release repo. These installer tests are run as nightly CI jobs.

Suggested tests cases for the new `platform.aws.vpc.subnets` field:

- Manually specified roles where IngressControllerLB and ControlPlaneExternalLB share the same subnets, and ClusterNodes
  and ControlPlaneInternalLB share the same subnets.
- Manually specified roles where dedicated subnets are used for all roles (IngressControllerLB, ControlPlaneInternalLB,
  ControlPlaneExternalLB, and ClusterNode)
- Public subnets only (`OPENSHIFT_INSTALL_AWS_PUBLIC_ONLY`)
- Manually specified roles where EdgeNode is specified.

### Impact of Deprecation on Testing

Commands in the Step-Registry will need to be updated to reflect the install-config
migration from `platform.aws.subnets` to `platform.aws.vpc.subnets` including, but
not limited to:

- [aws-load-balancer-tag-vpc-subnets](https://steps.ci.openshift.org/reference/aws-load-balancer-tag-vpc-subnets)
- [ipi-conf-aws-proxy](https://steps.ci.openshift.org/reference/ipi-conf-aws-proxy).
- [ipi-conf-aws-custom-vpc](https://steps.ci.openshift.org/reference/ipi-conf-aws-custom-vpc)
- [ipi-conf-aws-blackholenetwork](https://steps.ci.openshift.org/reference/ipi-conf-aws-blackholenetwork)
- [ipi-conf-aws-edge-zone](https://steps.ci.openshift.org/reference/ipi-conf-aws-edge-zone)
- [ipi-conf-aws-publicsubnets](https://steps.ci.openshift.org/reference/ipi-conf-aws-publicsubnets)
- [ipi-conf-aws-sharednetwork](https://steps.ci.openshift.org/reference/ipi-conf-aws-sharednetwork)

QE and the Test Platform Team should be notified that `platform.aws.subnets` is being deprecated, so they can adjust their existing tests
accordingly.

## Graduation Criteria

This feature (including the deprecation of `Subnets` field) will initially be
released as Tech Preview only, behind the `TechPreviewNoUpgrade` feature gate.

### Dev Preview -> Tech Preview

N/A. This feature will be introduced as Tech Preview.

### Tech Preview -> GA

The E2E tests should be consistently passing and a PR will be created
to enable the feature gate by default.

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

No upgrade or downgrade concerns because all changes are compatible or in the installer.

## Version Skew Strategy

N/A. This enhancement will only be supported for new installations and therefore has no version skew concerns.

## Operational Aspects of API Extensions

N/A.

## Support Procedures

### Ingress Not Functional After Installing using IngressControllerLB Subnets

If the cluster installation with specified IngressControllerLB subnets was successful, but ingress is not working,
check the AWS CCM logs of the leader CCM pod for errors in provisioning the load balancer:

```bash
oc logs -n openshift-cloud-controller-manager aws-cloud-controller-manager-86b68698cd-wfhgz
```

## Alternatives

### Day 1 `default` IngressController Subnet Selection via Installer Manifests Alternative

Starting with 4.17, cluster admins are can configure the subnets of the `default` IngressController
at install time by using [custom installer manifests](https://github.com/openshift/installer/blob/master/docs/user/customization.md#kubernetes-customization-unvalidated).
Custom installer manifests offer an unvalidated method for adding or customizing Kubernetes objects that are
injected into the cluster. The workflow is as follows:

1. Cluster admin runs the openshift-installer command to create the manifests' directory:
   ```bash
   ./openshift-installer create manifests --dir="cluster-foo"
   ```
2. Cluster admin adds the YAML for the `default` IngressController into the manifests directory:
   ```bash
   cat << EOF > cluster-foo/manifests/default-ingresscontroller.yaml
   apiVersion: operator.openshift.io/v1
   kind: IngressController
   metadata:
     name: default
     namespace: openshift-ingress-operator
   spec:
     replicas: 2
     endpointPublishingStrategy:
       type: LoadBalancerService
       loadBalancer:
         scope: External
         providerParameters:
           type: AWS
           aws:
             type: NLB
             networkLoadBalancer:
               subnets:
                 ids:
                 - subnet-09d17e6367aea1305
   EOF
   ```
3. Cluster admin starts the installation with the custom `default` IngressController manifest:
   ```bash
   ./openshift-installer create cluster --dir="cluster-foo"
   ```
4. The cluster is provisioned with the `default` IngressController created, and the Ingress Operator
   will NOT overwrite or edit the IngressController.
   - NOTE: Customers do NOT have to explicitly provide `spec.domain` in the `default` IngressController
     manifest as the Ingress Operator will assume the default value from the Ingress Config.

However, this alternative has been deemed insufficient because:

- The ROSA installer currently lacks support for custom manifests.
- Custom installer manifests are unvalidated, whereas a new API in install-config will have validation.
- The mechanics of providing a `default` IngressController manifest is complicated and error-prone.

### User-Created IngressController Day 2 Subnet Defaulting

In addition to setting the `default` IngressController subnets, IngressControllerLB subnets could also serve as the
default for all user-created IngressControllers on Day 2. Previous revisions of this enhancement included this
type of defaulting, but we deemed it out-of-scope for the following reasons:

1. It requires more effort than initially anticipated.
   - Since the IngressController `subnets` field was implemented so that status reflects the real value, the
     defaulting would need to happen in spec, and that mechanism would need to be built.
   - Since the IngressController `subnets` field is split into `classicLoadBalancer` and `networkLoadBalancer`,
     switching between NLBs and Classic load balancer while maintaining the default subnets is tricky.
   - Since `platform.aws.vpc.subnets` specifies either public subnets for public clusters or private subnets
     for private clusters, and cluster admins can create IngressController on Day 2 with a different scope
     than the cluster, the Ingress Config must store both public and private IngressControllerLB subnets.
2. It's not a hard requirement for service delivery.
   - Day 2 subnet defaulting mostly provides a UX convenience.
3. It can be done later if needed.
   - The solution outlined below is compatible, allowing users to opt in to defaulting for user-created
     IngressControllers.

To implement defaulting for user-created IngressControllers, the subnet fields must first be added to the Ingress
Config CRD. These fields would be populated by the installer based on the IngressControllerLB subnet role, and the
Ingress Operator would use them to provide defaults for user-created IngressControllers:

```yaml
   apiVersion: config.openshift.io/v1
   kind: Ingress
   metadata:
     name: cluster
   spec:
     #...
     loadBalancer:
       platform:
         aws:
           classicLoadBalancer:
             subnets:
               ids:
               - subnet-0fcf8e0392f0910d0
               - subnet-0fcf8e0392f0910d1
           networkLoadBalancer:
             subnets:
               ids:
               - subnet-0fcf8e0392f0910d0
               - subnet-0fcf8e0392f0910d1
```

Next, the IngressController API will need to be updated to support explicitly selecting the source of the subnets by
adding the `subnetSelection` field to the `classicLoadBalancer` and `networkLoadBalancer` structs. This field will have
the following modes of operation:

- A) `AutodetectUsingTags`: Autodetect subnets using tags.
- B) `ExplicitlyEnumerated`: Inherit the subnets from the cluster Ingress Config on creation.
- C) `AutomaticallyEnumeratedFromClusterConfigOnCreate`: Inherit the subnets from the cluster Ingress Config on first reconciliation.

```yaml
kind: IngressController
spec:
  endpointPublishingStrategy:
    type: LoadBalancerService
    loadBalancer:
      providerParameters:
        type: AWS
        aws:
          type: Classic
          classicLoadBalancer:
            # Option (a); the list must be empty.
            subnetSelection: AutodetectUsingTags
            subnets:
              ids: []
---
            # ...
            # Option (b); cluster-admin specifies the list on create.
            subnetSelection: ExplicitlyEnumerated
            subnets:
              ids:
                - subnet-0fcf8e0392f0910d0
                - subnet-0fcf8e0392f0910d1
---
            # ...
            # Option (c); controller specifies the list on first reconciliation
            # based on the cluster config.
            subnetSelection: AutomaticallyEnumeratedFromClusterConfigOnCreate
            subnets:
              ids:
                - subnet-0fcf8e0392f0910d0
                - subnet-0fcf8e0392f0910d1
```

Adding `subnetSelection` will make the subnet API more verbose, enable compatibility for implementing defaulting
(option C) in the future, and enable user-created IngressControllers to use subnet discovery if the
cluster was installed with IngressControllerLB subnets explicitly defined.

### Override Subnet Discovery Alternative

One possible design for the install-config API is to require at least one IngressControllerLB subnet for all
new installations, effectively overriding AWS subnet discovery. This approach would bypass subnet discovery
entirely, as the default subnets would always be explicitly defined in the Ingress Config object.

However, as discussed in [AWS Subnet Discovery](#aws-subnet-discovery), subnet discovery offers a more flexible
solution for dynamic AZ scaling (only for CLBs) and aligns with standard workflows and expectations for AWS customers.
After extensive discussions, we have decided to maintain a path for users to continue use subnet discovery.

### Role First API Design Alternative

Alternatively, we could refactor the API to replace the single list of subnets structs in
`platform.aws.vpc.subnets` with separate role-centric lists:

```go
type VPCSpec struct {
    //...
    // subnets specifies the subnets configuration for
    // a cluster by specifying lists of subnets organized
    // by their role. Leave unset to have the installer
    // create subnets in a new VPC on your behalf.
    //
    // +optional
    Subnets Subnet `json:"subnet,omitempty"`
}

type Subnet struct {
    // clusterNodeSubnets specifies subnets that will be used as
    // ClusterNode role subnets.
    //
    // +required
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="clusterNodeSubnets cannot contain duplicates"
    ClusterNodeSubnets []AWSSubnetID `json:"clusterNodeSubnets,omitempty"`

    // ingressControllerLBSubnets specifies subnets that will be used as
    // IngressControllerLB subnets.
    //
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="ingressControllerLBSubnets cannot contain duplicates"
    IngressControllerLBSubnets []AWSSubnetID `json:"ingressControllerLBSubnets,omitempty"`

    //...etc
}
```

However, it's not clear how this API design would enable automatic subnet selection since a list of subnets is
no longer required.

### Allowing Mixed Automatic and Manual Subnet Selection Alternative

If there is a desire to allow some roles to be automatically configured while other to be manually configured,
we could remove the restriction to require all roles to be specified.

However, the semantics are unclear. Take this install-config example that lacks a IngressControllerLB subnet:

```yaml
#...
  vpc:
    subnets:
    - id: subnet-001 # Private / AZ us-east-1a
      roles:
      - type: ClusterNode
    - id: subnet-002 # Public / AZ us-east-1a
      roles:
      - type: ControlPlaneExternalLB
    - id: subnet-003 # Private / AZ us-east-1a
      roles:
      - type: ControlPlaneInternalLB
#...
```

Since no IngressControllerLB role is explicitly assigned, subnet discovery will use any subnet provided in
`platform.aws.vpc.subnets`. However, each subnet already has an explicit role assigned. It seems unexpected that
`subnet-002`, despite being designated by the user exclusively for the ControlPlaneExternalLB role, is also assigned to
IngressController load balancers.

Additionally, consider a scenario where some subnets have assigned roles, while others have none:

```yaml
#...
  vpc:
    subnets:
    - id: subnet-001 # Private / AZ us-east-1a
      roles:
      - type: ClusterNode
    - id: subnet-002 # Public / AZ us-east-1a
      roles:
      - type: ControlPlaneExternalLB
    - id: subnet-003 # Private / AZ us-east-1a
    - id: subnet-004 # Public / AZ us-east-1a
#...
```

How should the installer handle `subnet-003` and `subnet-004`? Should it treat these subnet as available for any role,
or should it limit their use to only the roles that are unassigned? Requiring all roles to be specified eliminates
these ambiguous semantics.

### Explicit API for Automatic and Manual Subnet Selection Alternative

If a more explicit API is desired to distinguish between automatic and manual subnet selection,
a field could be added to `platform.aws.vpc.subnets` to enable this functionality:

```go
type Subnets struct {
	// ...
    ID AWSSubnetID `json:"id"`

	// roleSelection indicates the method of role selection for subnets.
	// Automatic means the installer and other components decides the
	// role of the subnets, while Manual indicates the roles will be
	// manually configured.
	RoleSelection RoleSelection `json:"roleSelection"`

	// ...only can be set when RoleSelection is Manual.
	Roles []SubnetRole `json:"roles"`
}

// RoleSelection specifies the method of role selection for subnets.
type RoleSelection string

const (
    // AutomaticRoleSelection means the installer and other components automatically
    // decide the role of the subnets.
    AutomaticRoleSelection RoleSelection = "Automatic"

    // ManualRoleSelection means the roles will be manually configured.
    ManualRoleSelection RoleSelection = "Manual"
)
```

## Infrastructure Needed

N/A