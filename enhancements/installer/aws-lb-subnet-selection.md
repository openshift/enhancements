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
- None
creation-date: 2024-05-29
last-updated: 2024-07-02
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

This enhancement extends the OpenShift Installer's install-config, enabling cluster admins to
configure subnets for AWS load balancers created for their IngressControllers at install time.
This proposal allows the install-time configuration of subnets for the `default` IngressController
as well as the default subnets for any new IngressController.

This enhancement also deprecates the existing install-config field `platform.aws.subnets`
in favor of a more flexible configuration field that handles specifying subnets for both
IngressControllers and the cluster infrastructure. These new subnet roles, referred to as
IngressController-role subnets, cluster-role subnets, or dual-role subnets (for those serving both roles)
are further defined in [Defining Subnet Roles](#defining-subnets-roles).

## Definitions and Terminology

### The `default` IngressController

This proposal will refer to the IngressController that gets created automatically during installation and handles
the platform routes (console, auth, canary, etc.) as the `default` IngressController. Note that
"default IngressController subnets" refers to the default subnets used by all IngressControllers, including
the `default` IngressController. This is different from "`default` IngressController subnets", which specifically
means the subnets for the `default` IngressController.

### Defining Subnets Roles

With the install-config now able to specify two different ways a subnet could be used, it's important
to clarify what these uses are. We will define the function, purpose, or use of a subnet as a "role".
We use the concept of roles to recognize that a single subnet can simultaneously fulfill multiple roles.

Let's define the two subnet roles. The role associated with the existing `platform.aws.subnets` field
will be defined as the "cluster subnet role". Additionally, this enhancement proposes a new subnet
role for hosting IngressControllers, which will be defined as the "IngressController subnet role".

_Note: These roles are defined only to establish a common language within the context of the install-config
and may not be applicable to other environments, situations, or APIs._

**Cluster Subnet Role**

Subnets with this role hosts a majority of cluster infrastructure resources, including instances (nodes)
and the External and Internal API load balancers. Additionally, they serve important functions such
as locating the VPC for cluster installation and determining the Availability Zones for the cluster.

**IngressController Subnet Role**

Subnets with this role are designated as the default subnets for hosting AWS load balancers created
specifically for IngressControllers. This role does not include load balancers created from generic
LoadBalancer-type Services, but only those from IngressControllers.

### Clarifying Subnet Roles Terminology

For the sake of simplicity, we will refer to "IngressController-role subnets" as subnets that fulfill
at least the IngressController subnet role, but are not limited to it. Similarly, "cluster-role subnets"
will refer to subnets that fulfill at least the cluster subnet role, but are not limited to it.
If a subnet is known to fulfill both IngressController and cluster roles, we will refer to that as a "dual-role subnet"
to make it clear that it serves both purposes.

When this proposal refers to a "IngressController subnet", it is generically referring to a subnet that associated with
an IngressController's load balancer, not specifically a subnet that was specified with the IngressController-role
in the install-config, as these can differ. However, this proposal will avoid using the term "cluster subnet",
due to its ambiguity; a "cluster subnet" could refer to a subnet that exists in the same VPC, one that has
instances belonging to it, or one that carries the `kubernetes.io/cluster/<cluster-id>` tag of the cluster.

## Motivation

Cluster admins using AWS may have dedicated subnets for their load balancers due to
security reasons, architecture, or infrastructure constraints. They may be installing
a cluster into a VPC with multiple Availability Zones (AZs), but they wish to restrict
their cluster to a single AZ.

Currently, cluster admins can configure their load balancer subnets after installation by configuring
the `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.subnets` field
on the IngressController (see [LoadBalancer Subnet Selection for AWS](/enhancements/ingress/lb-subnet-selection-aws.md)).
Cluster admins also need a way to configure their IngressController default subnets at install time (Day 1).

Configuring subnets at install time is essential for cluster admins that want to
ensure their `default` IngressController is configured properly from the start. Additionally,
Service Delivery has use cases for ROSA Hosted Control Plane (HCP) that require `default`
IngressController subnets to be configured during cluster installation.

While cluster admins can influence the subnets their load balancers select by using specific subnet tags, not all
cluster admins have the ability to update these tags. According to [kubernetes/kubernetes#97431](https://github.com/kubernetes/kubernetes/pull/97431),
a subnet will NOT be selected for an AWS load balancer (including those created for IngressControllers) if:
- The `kubernetes.io/cluster/<cluster-id>` tag contains the cluster ID of another cluster.
- The load balancer is external and the subnet is private.

### User Stories

#### Day 1 Default IngressController Subnet Selection on AWS

_"As a cluster admin, when installing a cluster in AWS, I want to specify the
default subnet selection for my IngressControllers."_

This includes the default subnets for all IngressControllers as well as the subnets for the `default` IngressController.

See [Setting Default IngressController subnets during Installation Workflow](#setting-default-ingresscontroller-subnets-during-installation-workflow)
for the workflow for this user story.

#### Installing a Private Cluster with Private Subnets in AWS

_"As a cluster admin, when installing a private cluster in AWS, I want to
specify that the `default` IngressController should use private subnets, so
that it is not exposed to the public Internet."_

When a cluster admin installs a private cluster into a VPC also containing
public subnets lacking the proper tags, internal LoadBalancer-type services
may use the misconfigured public subnets, introducing
a security risk (see related [RFE-2816](https://issues.redhat.com/browse/RFE-2816) and
[OCPBUGS-17432](https://issues.redhat.com/browse/OCPBUGS-17432)).

As a workaround to this type of issue, cluster admins may want to specify the private
subnet list to ensure that the `default` IngressController exclusively uses the private
subnets. See [Setting Default IngressController subnets during Installation Workflow](#setting-default-ingresscontroller-subnets-during-installation-workflow)
for the workflow for this user story.

#### ROSA HCP: 1 MachinePool in 1 Availability Zone

_"As a user of ROSA HCP, I want to install a cluster with 1 MachinePool in 1
Availability Zone and have my `default` IngressController only mapped to the subnets
in that Availability Zone."_

Similar to [Day 1 Default IngressController Subnet Selection on AWS](#day-1-default-ingress-controller-subnet-selection-on-aws).

When a cluster is installed into a VPC containing other subnets without the
`kubernetes.io/cluster/<cluster-id>` tag in different AZs than the cluster-role subnets,
the `default` IngressController's load balancer may select those subnets.
This creates a security risk because the load balancer may select subnets that belong to other clusters. This
user story is derived from [OCM-5969](https://issues.redhat.com/browse/OCM-5969).

See [Setting Default IngressController subnets during Installation Workflow](#setting-default-ingresscontroller-subnets-during-installation-workflow)
for the workflow for this user story.

#### ROSA HCP: Adding a 2nd MachinePool

_"As a user of ROSA HCP, I want to install a cluster with 1 MachinePool in 1
AZ, then add a 2nd MachinePool in another AZ and update my IngressController's
subnets to map to the new AZ."_

A user installed a ROSA HCP cluster with 1 MachinePool in one AZ similar to
[ROSA HCP: 1 MachinePool in 1 Availability Zone](#rosa-hcp-1-machinepool-in-1-availability-zone),
but now they want to add another machine pool in a different AZ. The user
would need to adjust the existing default IngressController subnets by following
the workflow [Updating an existing IngressController with new Subnets Workflow](/enhancements/ingress/lb-subnet-selection-aws.md#updating-an-existing-ingresscontroller-with-new-subnets-workflow).

Additionally, if the user wants all future IngressControllers to use the new subnet by default, see
[Updating the Default Subnets for New IngressControllers Workflow](#updating-the-default-subnets-for-new-ingresscontrollers-workflow).

#### Dedicated IngressController Subnets for Security

_"As a cluster admin, I want to install a cluster with all IngressControllers
using a dedicated subnets in order to have load balancer VIPs on a dedicated VLAN for
applying ACLs."_

A cluster admin wants their all of their load balancers mapped to separate subnet(s), allowing
firewall ACL rules to specifically target the load balancer VLAN
(see https://access.redhat.com/support/cases/#/case/03054638). The dedicated subnet(s)
must be located in the same AZ(s) as the provided cluster-role subnets; otherwise,
the IngressController will not function properly.

See [Setting Default IngressController subnets during Installation Workflow](#setting-default-ingresscontroller-subnets-during-installation-workflow)
for the workflow for this user story.

### Goals

- Deprecate the `platform.aws.subnets` field in the install-config and replace with a
  more flexible subnet structure for subnet-related configuration.
- Add a new field in the install-config for default subnet selection for IngressControllers in AWS.
- Provide install-time validation for user provided IngressController-role subnets.
- Support configuring the `default` IngressController's subnets at install time for AWS.
- Support configuring default subnets for all new IngressControllers at install time for AWS.
- Support updating the default subnets for all new IngressControllers after installation for AWS.

### Non-Goals

- Extend support to platforms other than AWS.
- Automatically restrict IngressController subnet selection for private clusters to ensure the cluster stays private.
- Assume that bring-your-own (BYO) cluster-role subnets are also IngressController-role subnets.
- Assume that IngressController-role subnets are also cluster-role subnets.
- Support configuring subnets for LoadBalancer-type Services that aren't associated with an IngressController.

## Proposal

To enable cluster admins to specify subnets for IngressControllers at install time, we will need
to make the following API updates:

1. Deprecate the `platform.aws.subnets` field in the install-config and replace with
   `platform.aws.subnetsConfig`, which more intuitively manages multiple subnet roles.
2. Add the `spec.loadBalancer.platform.aws.subnets` field in the IngressConfig
   to encode the default subnets for IngressControllers.

The proposal to deprecate the `platform.aws.subnets` field in the install-config is driven by
its ambiguity, as the field name does not clearly distinguish between IngressController-role subnets and cluster-role
subnets, which can be different. Additionally, `platform.aws.subnets` is a `[]string` and doesn't
allow for future expansion or adaptation for other subnet type, roles, or metadata information.

The IngressConfig holds the default values that the Ingress Operator will use when creating
the `default` IngressController, as well as any new IngressController. The IngressConfig will be
populated by the installer, similar to how the [Allow Users to specify Load Balancer Type during installation for AWS](/enhancements/installer/aws-load-balancer-type.md)
enhancement introduced the `spec.loadBalancer.platform.aws.lbType` field.

The OpenShift Installer will be updated to support the new `platform.aws.subnetsConfig` field, and the
Ingress Operator will be updated to apply the `Subnets` field in the IngressConfig as the default subnets
for new IngressControllers.

### Implementation Details/Notes/Constraints

As mentioned in [the Proposal section](#proposal), the install-config and the IngressConfig will be updated
to support propagating default subnets to all new IngressControllers, including the `default`
IngressController. The Ingress Operator will also need to be updated to use the new IngressConfig
`Subnet` field.

#### Installer Updates

The `platform.aws.subnets` field in the install-config will be deprecated and replaced with a new
field, `platform.aws.subnetsConfig`. Much like `Subnets`, the new `SubnetsConfig` field indicates
a list of subnets in a pre-existing VPC, but also provide the role that the subnet will fulfill in the
cluster. Additionally, since it is a struct and not a `[]string`, it provide the ability to be
expanded with additional subnet-related fields in the future.

The following install-config reflect the initial TechPreview release of this feature:

```go
// Platform stores all the global configuration that all machinesets
// use.
// +kubebuilder:validation:XValidation:rule=`!(has(self.subnets) && has(self.subnetsConfig))`,message="cannot use both subnets and subnetsConfig"
type Platform struct {
    [...]
    // Subnets specifies existing subnets (by ID) where cluster
    // resources will be created.  Leave unset to have the installer
    // create subnets in a new VPC on your behalf.
    //
    // Deprecated in Tech Preview: Use subnetsConfig
    //
    // +optional
    Subnets []string `json:"subnets,omitempty"`

    // subnetsConfig specifies the subnet configuration for
    // a cluster by specifying a list of subnet ids with their
    // designated role. At least one Cluster role subnet must be
    // specified. If no IngressController role subnets are specified, the
    // IngressController's Load Balancer will automatically discover
    // its subnets based on the kubernetes.io/cluster/<cluster-id> tag,
    // whether it's public or private, and the availability zone.
    // Leave this field unset to have the installer create subnets
    // in a new VPC on your behalf.
    // subnetsConfig is only available in TechPreview.
    //
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x.id == y.id))`,message="subnetsConfig cannot contain duplicate IDs"
    // +kubebuilder:validation:XValidation:rule=`self.exists(x, x.roles.exists(r, r == 'Cluster'))`,message="subnetsConfig must contain at least 1 subnet with the Cluster role"
    // +kubebuilder:validation:XValidation:rule=`self.filter(x, x.roles.exists(r, r == 'Ingress')).size() <= 10`,message="subnetsConfig must contain less than 10 subnets with the Ingress role"
    SubnetsConfig []SubnetConfig `json:"subnetsConfig,omitempty"`
    }

type SubnetConfig struct {
    // ID specifies the subnet ID of an existing
    // subnet.
    //
    // +required
    ID AWSSubnetID `json:"id"`

    // Roles specifies the roles (aka functions) that the
    // subnet will provide in the cluster.
    //
    // +required
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="roles cannot contain duplicates"
    Roles []SubnetRole `json:"roles"`
}

// AWSSubnetID is a reference to an AWS subnet ID.
// +kubebuilder:validation:MinLength=24
// +kubebuilder:validation:MaxLength=24
// +kubebuilder:validation:Pattern=`^subnet-[0-9A-Za-z]+$`
type AWSSubnetID string

// SubnetRole specifies the roles (aka functions) that the subnet will provide in the cluster.
type SubnetRole string

const (
    // ClusterSubnetRole specifies subnets that will be used as subnets for the nodes,
    // API load balancers, and for locating the VPC and Availability Zones.
    SubnetRole ClusterSubnetRole = "Cluster"

    // IngressControllerSubnetRole specifies subnets that will be used as the default
    // subnets for IngressControllers.
    SubnetRole IngressControllerSubnetRole = "IngressController"
)
```

The `SubnetsConfig` field will be introduced under the TechPreview feature gate, and that the `Subnets` field
will be initially deprecated in TechPreview. Both the new `SubnetsConfig` field and the deprecation of `Subnets` field
will graduate to GA together. Upon graduation, the Go docs should be updated to remove the
`// subnetsConfig is only available in TechPreview.` and change `// Deprecated in Tech Preview: Use subnetsConfig`
to `// Deprecated: Use subnetsConfig`


##### Installer Validation Considerations

CEL validation enforces that at least one `SubnetConfig` has a `Cluster` role. This requirement exists
because if no `Cluster` role subnets are specified, the installer would have to create the `Cluster` role subnets
within the VPC associated with the `IngressController` role subnets. While this scenario could be valid, it is likely
to cause confusion. Cluster admins might not realize that even though they provided a valid `SubnetsConfig`
field, the installer will still create the `Cluster` role subnets during installation. This validation could
be relaxed in the future, provided there is a feature request to automatically create `Cluster` role subnets, but
manually specify `IngressController` role subnets.

The existing validation for the deprecated `Subnets` field should apply to all subnets (`IngressController`
and `Cluster`) specified in `SubnetsConfig`, such as the [existing validation](https://github.com/openshift/installer/blob/0d77aa8df5ddc68e926aa11da24a87981021b256/pkg/asset/installconfig/aws/subnet.go#L91)
that confirms all subnets are from the same VPC.

Since the AWS Cloud Controller Manager (CCM) will reject certain subnets, the installer will need to add
additional validation for `IngressController` role subnets:

- The installer should reject multiple `IngressController` role subnets in the same AZ as this will be rejected
  by the AWS CCM.

#### IngressConfig API Updates

The `spec.loadBalancer.platform.aws.subnets` field will be added to the IngressConfig
to encode the default subnets for all IngressControllers. This will be populated by installer
from the values provided in the new `platform.aws.subnetsConfig` install-config field. The `v1.AWSSubnets`
field is reused from the IngressController API to maintain commonality. With the new `subnetsConfig`
field, the install-config supports only subnet IDs, so subnet names will not be needed.

```go
// AWSIngressSpec holds the desired state of the Ingress for Amazon Web Services infrastructure provider.
// This only includes fields that can be modified in the cluster.
// +union
type AWSIngressSpec struct {
    [...]
    // Subnets specifies the default subnets for all IngressControllers,
    // including the default IngressController. This default will be overridden
    // if subnets are specified on the IngressController.
    // If omitted, IngressController subnets will be automatically discovered
    // based on the kubernetes.io/cluster/<cluster-id> tag, whether it's public
    // or private, and the availability zone.
    //
    // +optional
    // +openshift:enable:FeatureGate=IngressControllerLBSubnetsAWS
    Subnets []v1.AWSSubnets `json:"subnets,omitempty"`
}
```

#### Ingress Operator Updates

The Ingress Operator will be updated to consume the IngressConfig's `spec.loadBalancer.platform.aws.subnets`
field when creating IngressControllers. The `spec.loadBalancer.platform.aws.subnets` field will propagate
to the IngressController's `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.subnets`
field as long as `Subnets` haven't been defined on the IngressController (i.e. defaulting). This logic
will follow the existing patterns in the `setDefaultProviderParameters` function in the IngressOperator repo.

No validation for this new IngressConfig API will be added to the Ingress Operator as a part of this enhancement.

### Workflow Description

#### Setting Default IngressController subnets during Installation Workflow

Setting the default subnets for all IngressControllers (including the `default` IngressController)
during installation (Day 1):

1. Cluster admin creates an install-config with `platform.aws.subnetsConfig` specified
   with the subnet(s) while ensuring the `platform.aws.subnetsConfig[].roles` field is
   configured with `IngressController` and `Cluster` (also known as dual-role subnets):
    ```yaml
    apiVersion: v1
    baseDomain: devcluster.openshift.com
    metadata:
      name: my-cluster
    [...]
    platform:
      aws:
        region: us-east-2
        subnetsConfig:
        - id: subnet-0fcf8e0392f0910d0
          roles:
          - Ingress
          - Cluster
        - id: subnet-0fcf8e0392f0910d1
          roles:
          - Ingress
          - Cluster
    [...]
    ```
2. The OpenShift Installer installs the cluster into the VPC where the subnets exist, and populates the
   IngressConfig's `spec.loadBalancer.platform.aws.subnets` field:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Ingress
   metadata:
     name: my-cluster
   spec:
     loadBalancer:
       platform:
         aws:
           subnets:
           - id: subnet-0fcf8e0392f0910d0
           - id: subnet-0fcf8e0392f0910d1
   [...]
   ```
3. When the Ingress Operator starts, it uses the IngressConfig's `Subnets` value for the `default` IngressController
   as well as the default for all new IngressControllers:
   ```yaml
   apiVersion: operator.openshift.io/v1
   kind: IngressController
   metadata:
     name: default
     namespace: openshift-ingress-operator
   spec:
     [...]
     endpointPublishingStrategy:
       type: LoadBalancerService
       loadBalancer:
         scope: External
         providerParameters:
           type: AWS
           aws:
             type: Classic
             subnets:
               ids:
               - subnet-0fcf8e0392f0910d0
               - subnet-0fcf8e0392f0910d1
     [...]
   ```

#### Configuring Auto Subnet Selection for IngressControllers with BYO VPC during Installation Workflow

A cluster admin wants to provide cluster-role subnets to use an existing VPC, but still have the IngressController
automatically discover the subnets for its load balancer: 

1. Cluster admin creates an install-config with `platform.aws.subnetsConfig` specified
   with the subnet(s) while ensuring `platform.aws.subnetsConfig[].roles` is configured
   with only `Cluster`:
    ```yaml
    apiVersion: v1
    baseDomain: devcluster.openshift.com
    metadata:
      name: my-cluster
    [...]
    platform:
      aws:
        region: us-east-2
        subnetsConfig:
        - id: subnet-0fcf8e0392f0910d2
          roles:
          - Cluster
        - id: subnet-0fcf8e0392f0910d3
          roles:
          - Cluster
    [...]
    ```
2. The OpenShift Installer installs the cluster into the VPC where the cluster-role subnets exist, but does not
   configure any specific subnets for the `default` IngressController or set defaults for new IngressControllers.
3. When the `default` IngressController is created, the load balancer will automatically map to the appropriately
   tagged subnets in the VPC.

#### Specifying Dedicated IngressController-role Subnets During Installation with a BYO VPC Workflow

Specifying dedicated IngressController subnets enables cluster admins to separate
IngressController subnets from the node subnets (i.e. cluster-role subnets):

1. Cluster admin creates an install-config with `platform.aws.subnetsConfig`, specifying
   each subnet to have either the `IngressController` or `Cluster` role in `platform.aws.subnetsConfig.roles`, but
   not both. Note that each `IngressController` role subnet MUST have a corresponding `Cluster` role subnet in the
   same AZ; otherwise, the load balancer will be unable to route traffic into the cluster: 
    ```yaml
    apiVersion: v1
    baseDomain: devcluster.openshift.com
    metadata:
      name: my-cluster
    [...]
    platform:
      aws:
        region: us-east-2
        subnetsConfig:
        - id: subnet-0fcf8e0392f0910d4 # AZ us-east-1
          roles:
          - Cluster
        - id: subnet-0fcf8e0392f0910d5 # AZ us-east-1
          roles:
          - Ingress
    [...]
    ```
2. The OpenShift Installer installs the cluster into the VPC where the subnets exist, and populates the
   IngressConfig's `spec.loadBalancer.platform.aws.subnets` field.
3. When the Ingress Operator starts, it uses the IngressConfig's `Subnets` value for the `default` IngressController
   as well as the default for all new IngressControllers.
4. Since the IngressController subnet(s) are in the same AZ as the cluster-role subnets(s), ingress traffic is successfully
   routed from the load balancer to the nodes.

#### Updating the Default Subnets for New IngressControllers Workflow

When new IngressControllers are created, they will use the subnets specified at install time
unless otherwise specified. If a cluster admin wants to change this default subnet selection
after installation, they can follow these steps:

1. The cluster admin edits the IngressConfig via `oc edit ingresses.config.openshift.io cluster`.
2. The cluster admin updates the subnets in `spec.loadBalancer.platform.aws.subnets` to their
   desired default subnets.

Now all new IngressController will be created with the updated subnets specified in the IngressConfig's
`spec.loadBalancer.platform.aws.subnets`.

### API Extensions

This proposal doesn't add any API extensions other than the new field proposed in
[IngressConfig API Updates](#ingressconfig-api-updates).

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is directly enabling the https://issues.redhat.com/browse/XCMSTRAT-545
feature for ROSA Hosted Control Planes.

#### Standalone Clusters

This proposal does not have any special implications for standalone clusters.

#### Single-node Deployments or MicroShift

This proposal does not have any special implications for single-node
deployments or MicroShift.

### Risks and Mitigations

#### No IngressController-Role Subnet in the Same AZ as Cluster-Role Subnets Risk

If a provided IngressController-role subnet is not in the same AZ as any provided cluster-role subnet,
it will not be functional. A load balancer subnet must have adjacent subnets in the same AZ in order to route
traffic to the cluster's nodes. The risk is that a user might provide IngressController-role subnets that are all
in different AZs from their cluster-role subnets, leading to confusion about why their IngressController(s) are not
functioning as expected.

See [Open Questions](#open-questions) for the open question on whether we should mitigate this with
validation.

### Drawbacks

- Deprecating `platform.aws.subnets` will be painful for users.
- Distinguishing between BYO subnets/VPC and installer-created subnets/VPC may be confusing if we need
  to expand the `platform.aws.subnetsConfig` field in the future to accommodate installer-created
  subnets configuration.
  - For example, there may be a future desire to specify the IngressController role for installer-created subnets,
    and it is unclear how this API design would accommodate that.

## Open Questions

- Q: Should the install-config design assume all IngressController-role subnets are cluster-role subnets too?
  - This [customer case](https://access.redhat.com/support/cases/#/case/03054638) has a requirement to
    have load balancer VIPs created in a dedicated NLB subnet which is different from the cluster-role subnets.
  - Q: Is that customer case a valid scenario?
- Q: Should the installer validate IngressController-role subnets to ensure they provide connectivity to cluster-role
  subnets to mitigate [No IngressController-Role Subnet in the Same AZ as Cluster-Role Subnets Risk](#no-ingresscontroller-role-subnet-in-the-same-az-as-cluster-role-subnets-risk)?

## Test Plan

### Ingress Operator Testing

An E2E test will be created in the Ingress Operator repo to verify the functionality of the new [IngressConfig API](#ingressconfig-api-updates).
This test will follow a similar pattern established in the existing [TestAWSLBTypeDefaulting](https://github.com/openshift/cluster-ingress-operator/blob/4e621359cea8ef2ae8497101ee3daf9f474b4b66/test/e2e/operator_test.go#L1368) test.

### Installer Testing

E2E test(s) will also be added to the installer to verify functionality of the new [Installer API](#installer-updates).
These tests, typically written by QE, will follow existing patterns established for testing installer functionality in
the [openshift-tests-private](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/openshift-tests-private)
directory of the openshift/release repo. These installer tests are run as nightly CI jobs.

### Impact of Deprecation on Testing

Commands in the Step-Registry will need to be updated to reflect the install-config
migration from `platform.aws.subnets` to `platform.aws.subnetsConfig` including, but
not limited to:

- [aws-load-balancer-tag-vpc-subnets](https://steps.ci.openshift.org/reference/aws-load-balancer-tag-vpc-subnets)
- [ipi-conf-aws-proxy](https://steps.ci.openshift.org/reference/ipi-conf-aws-proxy).
- [ipi-conf-aws-custom-vpc](https://steps.ci.openshift.org/reference/ipi-conf-aws-custom-vpc)
- [ipi-conf-aws-blackholenetwork](https://steps.ci.openshift.org/reference/ipi-conf-aws-blackholenetwork)
- [ipi-conf-aws-edge-zone](https://steps.ci.openshift.org/reference/ipi-conf-aws-edge-zone)
- [ipi-conf-aws-publicsubnets](https://steps.ci.openshift.org/reference/ipi-conf-aws-publicsubnets)
- [ipi-conf-aws-sharednetwork](https://steps.ci.openshift.org/reference/ipi-conf-aws-sharednetwork)

QE will be notified that `platform.aws.subnets` is being deprecated, so they can adjust their existing tests
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

### Ingress Not Functional After Installing using IngressController-role Subnets

If the cluster installation with specified IngressController-role subnets was successful, but ingress is not working,
check the AWS CCM logs of the leader CCM pod for errors in provisioning the load balancer:

```bash
oc logs -n openshift-cloud-controller-manager aws-cloud-controller-manager-86b68698cd-wfhgz
```

## Alternatives

### Assume All Cluster-Role Subnets are IngressController-Role Subnets Alternative

If the installer enforced that all cluster-role subnets are also IngressController-Role subnets, as suggested in
the closed bug [OCPBUGS-17432](https://issues.redhat.com/browse/OCPBUGS-17432), then we could eliminate
the need for user-facing install-config API updates and the deprecation of the `platform.aws.subnets field`.
The installer would be updated to ensure that the values in `platform.aws.subnets` automatically propagates
to the IngressConfig's `spec.loadBalancer.platform.aws.subnets` field. This would make all cluster-role subnets
the default IngressController subnets.

While this would address the majority of customer use cases, this would invalidate the
[Dedicated IngressController Subnets for Security](#dedicated-ingresscontroller-subnets-for-security) use case
and could potentially be seen as a breaking change. Customers who previously relied on the selection of
subnets without the `kubernetes.io/cluster/<cluster-id>` tag (introduced by [kubernetes/kubernetes#97431](https://github.com/kubernetes/kubernetes/pull/97431)),
might find their future installs broken.

### Assume All Subnets are Cluster-Role Subnets Alternative

To simplify the install-config API and mitigate [No IngressController-Role Subnet in the Same AZ as Cluster-Role Subnets Risk](#no-ingresscontroller-role-subnet-in-the-same-az-as-cluster-role-subnets-risk),
we could make the assumption that any provided subnets are cluster-role subnets. This would invalidate the user story
[Dedicated IngressController Subnets for Security](#dedicated-ingresscontroller-subnets-for-security), but simplify the function of
providing subnets via the install-config API.

```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
    [...]
    // SubnetsConfig specifies the subnets configuration for
    // a cluster by specifying a list of subnets with their
    // desired role. Leave unset to have the installer
    // create subnets in a new VPC on your behalf.
    //
    // +optional
    SubnetsConfig []SubnetConfig `json:"subnetsConfig,omitempty"`
}

type SubnetConfig struct {
    // ID specifies the subnet ID of an existing
    // subnet.
    //
    // +required
    ID AWSSubnetID `json:"id,omitempty"`

    // AdditionalRoles specifies additional roles or functions
    // that the subnet will provide in the cluster. By default,
    // all subnets are cluster-role subnets.
    //
    // +optional
    AdditionalRoles []SubnetRole `json:"additionalRoles,omitempty"`
}

// AWSSubnetID is a reference to an AWS subnet ID.
type AWSSubnetID string

// SubnetRole describes the role or function the subnet will provide in the cluster.
type SubnetRole string

const (
    // IngressControllerSubnetRole specifies subnets that will be used as the default
    // subnets for IngressControllers.
    SubnetRole IngressControllerSubnetRole = "IngressController"
)
```

### Role First API Design Alternative

Alternatively, we could refactor the API to replace the single list of subnets structs in
`platform.aws.subnetsConfig[]` with separate role-centric lists: `platform.aws.subnetsConfig.clusterSubnets[]`
and `platform.aws.subnetsConfig.ingressControllerSubnets[]`:

```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
    [...]
    // subnetConfig specifies the subnets configuration for
    // a cluster by specifying lists of subnets organized
    // by their role. Leave unset to have the installer
    // create subnets in a new VPC on your behalf.
    //
    // +optional
    SubnetConfig SubnetConfig `json:"subnetConfig,omitempty"`
}

type SubnetConfig struct {
    // clusterSubnets specifies subnets that will be used as
    // cluster-role subnets.
    //
    // +required
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="clusterSubnets cannot contain duplicates"
    ClusterSubnets []AWSSubnetID `json:"clusterSubnets,omitempty"`

    // ingressControllerSubnets specifies subnets that will be used as
    // IngressController-role subnets.
    //
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="ingressControllerSubnets cannot contain duplicates"
    IngressControllerSubnets []AWSSubnetID `json:"ingressControllerSubnets,omitempty"`
}

// AWSSubnetID is a reference to an AWS subnet ID.
// +kubebuilder:validation:MinLength=24
// +kubebuilder:validation:MaxLength=24
// +kubebuilder:validation:Pattern=`^subnet-[0-9A-Za-z]+$`
type AWSSubnetID string
```

## Infrastructure Needed

N/A