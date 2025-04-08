---
title: aws-lb-eip-selection
authors:
- "@miheer"
reviewers:
- "@gcs278"
- "@alebedev87"
- "@Miciah"
- "@mtulio"
- "@candita"
- "@frobware"
- "@rfredette"
approvers:
- "@patrickdillon"
- "@mtulio"
- "@r4f4"
- "@sadasu"
api-approvers:
- None
creation-date: 2024-09-26
last-updated: 2024-10-28
tracking-link:
  - https://issues.redhat.com/browse/CORS-3687
see-also:
  - ""
replaces:
  - ""
superseded-by:
---

# Installer LoadBalancer EIP Selection for AWS

## Summary

This enhancement extends the OpenShift Installer's install-config, enabling cluster admins to
configure EIPs for AWS NLB load balancer created for their `default` NLB IngressController at install time.

## Definitions and Terminology

### The `default` IngressController

This proposal will refer to the IngressController that gets created automatically during user provisioned installation and handles
the platform routes (console, auth, canary, etc.) as the `default` IngressController.

### CCM 
CCM refers to Cloud Controller Manager which calls [AWS LoadBalancer Controller](https://github.com/kubernetes-sigs/aws-load-balancer-controller) to create AWS LBs.

### Some more terminologies
EIP = AWS Elastic IP, NLB = AWS Network LoadBalancer, AZ = Availability Zone, LB = LoadBalancer


## Motivation

### User Stories
- As a cluster administrator using installer, I want to configure `default` NLB IngressController to use EIPs.
- As a cluster administrator of OpenShift on AWS (ROSA) using installer, I want to use static IPs (and therefore AWS Elastic IPs) so that
  I can configure appropriate firewall rules.
  I want the default AWS Load Balancer that they use (NLB) for their router to use these EIPs.
- As a cluster administrator who has purchased a block of public IPv4 addresses, I want to use these IP addresses, so
  that I can avoid Amazon's new charge for public IP addresses.

### Goals
- Users are able to use EIPs for a `default` NLB `IngressController` at install time.
- Add validation to the installer to prevent invalid EIP configurations.
  For example: Check for unassociated EIPs before passing to CCM.

### Non-Goals
- Creation of EIPs in AWS.
- Static IP usage with NLBs for OpenShift API server, DNS, Nat Gateways, LBs, Instances.
- To assign IPs from a Customer Owned IP (CoIP) Pool when using Outposts.
- Set default EIPs for user-created IngressControllers.

## Proposal
This enhancement adds API fields in the installer and the Ingress Config specification
to set the AWS EIP for the `default` NLB `IngressController`. The cluster ingress operator will then create
the `default` `IngressController` and get the EIP assigned to the service LoadBalancer with the annotation
`service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
Note: `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` is a standard [Kubernetes service annotation](https://kubernetes.io/docs/reference/labels-annotations-taints/#service-beta-kubernetes-io-aws-load-balancer-eip-allocations)

### API Extensions

#### Installer Updates
- The first API extension for setting `eipAllocations` is in the installer [Platform](https://github.com/openshift/installer/blob/master/pkg/types/aws/platform.go) type, where the new field `eipAllocations` is added as an optional field.

```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
...

    // eipAllocations contains Elastic IP (EIP) allocations for AWS resources 
    // within the cluster.
    //
    // +optional
    EIPAllocations *EIPAllocations `json:"eipAllocations,omitempty"`
}

// EIPAllocations contains Elastic IP (EIP) allocations for AWS resources 
// within the cluster.
type EIPAllocations struct {
    // ingressNetworkLoadBalancer is a list of IDs for Elastic IP (EIP) addresses that
    // are assigned to the default AWS NLB IngressController.
    // The following restrictions apply:
    //
    // eipAllocations can only be used with external scope, not internal.
    // An EIP can be allocated to only a single IngressController.
    // The number of EIP allocations must match the number of subnets that are used for the load balancer.
	// Each EIP allocation must be unique.
    // A maximum of 10 EIP allocations are permitted.
    //
    // See https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html for general
    // information about configuration, characteristics, and limitations of Elastic IP addresses.
    //
    // +openshift:enable:FeatureGate=InstallEIPForDefaultNLBIngressController
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="eipAllocations cannot contain duplicates"
    // +kubebuilder:validation:MaxItems=10
    IngressNetworkLoadBalancer []EIPAllocation `json:"ingressNetworkLoadBalancer"`
}


// EIPAllocation is an ID for an Elastic IP (EIP) address that can be allocated to an ELB in the AWS environment.
// Values must begin with `eipalloc-` followed by exactly 17 hexadecimal (`[0-9a-fA-F]`) characters.
// + Explanation of the regex `^eipalloc-[0-9a-fA-F]{17}$` for validating value of the EIPAllocation:
// + ^eipalloc- ensures the string starts with "eipalloc-".
// + [0-9a-fA-F]{17} matches exactly 17 hexadecimal characters (0-9, a-f, A-F).
// + $ ensures the string ends after the 17 hexadecimal characters.
// + Example of Valid and Invalid values:
// + eipalloc-1234567890abcdef1 is valid.
// + eipalloc-1234567890abcde is not valid (too short).
// + eipalloc-1234567890abcdefg is not valid (contains a non-hex character 'g').
// + Max length is calculated as follows:
// + eipalloc- = 9 chars and 17 hexadecimal chars after `-`
// + So, total is 17 + 9 = 26 chars required for value of an EIPAllocation.
//
// +kubebuilder:validation:MinLength=26
// +kubebuilder:validation:MaxLength=26
// +kubebuilder:validation:XValidation:rule=`self.startsWith('eipalloc-')`,message="eipAllocations should start with 'eipalloc-'"
// +kubebuilder:validation:XValidation:rule=`self.split("-", 2)[1].matches('[0-9a-fA-F]{17}$')`,message="eipAllocations must be 'eipalloc-' followed by exactly 17 hexadecimal characters (0-9, a-f, A-F)"
type EIPAllocation string

```

### Workflow Description

Pre-requisites: One must allocate an Elastic IP address from [Amazon's pool of public IPv4 addresses](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html#using-instance-addressing-eips-allocating),
or from a custom IP address pool that you have brought to your AWS account. For more information about bringing your own IP address range to your
AWS account, see [Bring your own IP addresses (BYOIP) in Amazon EC2](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-byoip.html)
which could be then used in the `ingressNetworkLoadBalancer` field of the install-config.
Note: The AWS account has some cap on the number of EIPs per region. By default, all AWS accounts have a quota of five (5) Elastic IP addresses per Region, because public (IPv4)
internet addresses are a scarce public resource. For more details please check [Elastic IP address quota](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html#using-instance-addressing-limit).
When this cap is hit you will get the following error: `Elastic IP address could not be allocated.
The maximum number of addresses has been reached.`
Elastic IP Allocation can only be done for internet-facing NLB.
If you are using an already available eipAllocation then it should be an `UnAssociated` one.

**cluster administrator** is a human user responsible for deploying a
cluster. A cluster administrator also has the rights to modify the cluster level components.

For installer-created-VPCs, a cluster admin can accurately predict how many EIPAllocations to bring so their cluster doesn't fail to install as follows as they might not 
know what public subnets will be created by the openshift-installer.

To illustrate further, the workflow could start like:

1. Cluster admin wants to use EIPs, but isn't bringing their own VPC, so they need to know exactly how many EIPs to create.
2. Cluster admin should review https://aws.amazon.com/about-aws/global-infrastructure/regions_az/ to find out how many AZs are in the region they are installing into.
3. Cluster admin should use the same number of EIPs as AZs in their region.

#### Configuring EIP for the default IngressController at installation time:
1. Cluster administrator creates `install-config.yaml` and adds their domain, lbType to NLB, AWS region, and EIP allocation ids.
```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  aws:
    region: <AWS region>
    lbType: NLB
    ingressNetworkLoadBalancer:
      eipAllocations:
      - eipalloc-1111
      - eipalloc-2222
      - eipalloc-3333
      - eipalloc-4444
      - eipalloc-5555 
...
```  

2. Installer creates a `default Ingress Controller CR` with the eipAllocations specified. 
3. Ingress Operator will create the `default` `IngressController` with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` with values from the field `eipAllocations`
   from the cluster ingress config object.

#### Updating the default IngressController's EIPs after installation:

The cluster-admin will need to update the eipAllocations in `IngressConfig` and then delete the `default IngressController`. 
The cluster-ingress-operator will create a new `default IngressController` with the newly updated eipAllocations.

### API Extensions

This proposal doesn't add any API extensions other than the new field proposed in
[IngressConfig API Updates](#ingressconfig-api-updates).

### Topology Considerations

#### Hypershift / Hosted Control Planes
Currently, there is no unique consideration for making this change work with Hypershift.

#### Standalone Clusters
This proposal does not have any special implications for standalone clusters.

#### Single-node Deployments or MicroShift
This proposal does not have any special implications for single-node
deployments or MicroShift.

### Implementation Details/Notes/Constraints

#### Set EIP through installer for the default IngressController:
    1. The admin sets the `eipAllocations` in the installer using the install-config.yaml.
    2. The installer then creates a default Ingress Controller resource with the `eipAllocations`.
    3. The cluster ingress operator watches the resource and then creates a `default` `IngressController` by:
        1. creating an `default` `IngressController` from the `default` `Ingress Controller CR` created by the installer.
        2. then scaling a deployment for the `IngressController` CR which has the value of the `eipAllocations`.
        3. then creating a service of service type load balancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`,
           which uses the value from the field `eipAllocations` of `IngressController` CR.

#### Field Validation

##### Validation on installer when installing in managed VPC (full-automated) based in the discovered zones used to create the cluster.
   Add a function to the installer code to  compare the number of Availability Zones in the region to the number of eipAllocations passed in the `install-config.yaml`.   This is required because the cluster will select one subnet per AZ, so the number of EIPs must be equal to the number of subnets.

##### The number of public subnets must match the number of `eipAllocations` in an unmanaged VPC (BYO VPC)
     We will be comparing if the number of public subnets added to the install-config matches with len(eipAllocations).

##### Ensure that EIPs exist and are not already assigned.
EIPs can be assigned to many resource types, like NAT gateways, load balancers, instances, etc. The attribute `associationId` will be set when the EIP is already associated.
To mitigate this we could add that validation, at least on installer, to provide quick-feedback (fail when validate install-config) to the user when the provided EIP is already associated to another resource.
It would be nice to have a validation before setting the annotation to CCM, keeping the operator Degraded before disrupting the service.

##### EIP Allocation Defaulting Mechanics for Ingress Controller
Traditionally, the Ingress Operator has populated default values from the Ingress Config into the `status`, 
making `status` effectively reflect the desired state of the IngressController. However, since 
`eipAllocations` in `status` represents the **actual** state, not the **desired** state, the default 
`eipAllocations` values must be set in the `spec` when the Ingress Operator initially admits the 
IngressController.
This approach is new. The Ingress Operator does not typically set default values in `spec` for load balancer
configurations if the user hasn’t explicitly provided them. While this defaulting pattern is more consistent
with Kubernetes conventions for `spec` and `status` (and is also our only option in this situation), it's
important to acknowledge that this inconsistency in defaulting behavior could cause confusion for users.

##### Validation to check if lbType was to NLB when eipAllocations were provided in the installer
EIPs can provided only for `NLB` type `IngressController` so, the installer will be check for the lbType set to NLB when `eipAllocations` are provided
in the `install-config.yaml`. We can't add a CEL in the Platform API type of the installer so a validation will need to be
added in the installer code.

### Risks and Mitigations

- Providing a right number of EIPs same as the number of public subnets as per AWS guidelines is an admin
  responsibility.
- Also providing existing available EIPs which are not assigned to any instance is important.
- If the above two points are not followed, the Kubernetes LoadBalancer service for the IngressController
  won't get assigned EIPs, which will cause the LoadBalancer service to be in persistently pending state by the
  Cloud Controller Manager (CCM). The reason for the persistently pending state is posted to the status of the IngressController.
  As of now we are not able to identify any more risks which can affect other components of the EP.

### Drawbacks


## Open Questions
- Q: As per [EP](https://github.com/openshift/enhancements/pull/1634), old subnets field will be deprecated. So, shall we skip the validation for checking
  number of `BYO Subnets` provided in the `install-config.yaml` with the number of eipAllocations ? Or shall we compare the old subnets field with the eipAllocations ?

- Q: Should we split Ingress Config into defaulting for the default IngressController and defaulting for user-created IngressControllers?
  Reference: https://docs.google.com/document/d/1Y-Z8gaKYv5dczbdwvXanbzBl-HjZ7zasRC0WaJGBWkA/edit?tab=t.0#heading=h.1196heeu6pjk

  API PR discussion thread: https://github.com/openshift/api/pull/2043#issuecomment-2440169902

  In this EP we no loner use Ingress Config to set `eipAllocations`.

## Test Plan

### Ingress Operator Testing

An E2E test will be created in the Ingress Operator repo to verify the functionality of the new [IngressConfig API](#ingressconfig-api-updates).
This test will follow a similar pattern established in the existing [TestAWSLBTypeDefaulting](https://github.com/openshift/cluster-ingress-operator/blob/4e621359cea8ef2ae8497101ee3daf9f474b4b66/test/e2e/operator_test.go#L1368) test.

### Installer Testing

E2E test(s) will also be added to the installer to verify functionality of the new [Installer API](#installer-updates).
These tests, typically written by QE, will follow existing patterns established for testing installer functionality in
the [openshift-tests-private](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/openshift-tests-private)
directory of the openshift/release repo. These installer tests are run as nightly CI jobs.


## Graduation Criteria

This feature will initially be released as Tech Preview only, behind the `TechPreviewNoUpgrade` feature gate.

### Dev Preview -> Tech Preview

N/A. This feature will be introduced as Tech Preview.

### Tech Preview -> GA

The E2E tests should be consistently passing and a PR will be created
to enable the feature gate by default.

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

No upgrade or downgrade concerns because all changes are compatible or in the installer.

## Version Skew Strategy

N/A. This enhancement will only be supported for new installations and therefore has no version skew concerns.

## Operational Aspects of API Extensions

N/A.

## Support Procedures

CCM - Cloud  Controller Manager

- If the service is not getting updated with the EIP annotation and the kubernetes load balancer
  service is in `Pending` state then check the `cloud-controller-manager` logs and `IngressController` status.

In the CCM logs search for the `IngressController` name and check if you find any error related EIP allocation failure.
```sh
  oc logs <ccm pod name> -n openshift-cloud-controller-manager
```

In the `IngressController` status, check the status for the following:
- Error messages for invalid eips or eips not present in the subnet of the VPC is `The service-controller component is reporting SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure load balancer: error creating load balancer: "AllocationIdNotFound:` for status type `LoadBalancerReady` and `Available`.
- Error messages when the number of EIP provided and the number of subnets of VPC ID don't match is `The service-controller component is reporting SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure load balancer: error creating load balancer: Must have same number of EIP AllocationIDs (4) and SubnetIDs (5)` for status type `LoadBalancerReady` and `Available`.
- Check the status of `IngressController` controller for LoadBalancer which is LoadBalancerReady, LoadBalancerProgressing, Available.

```sh
  oc get ingresscontroller <ingresscontroller name> -o yaml
```

## Alternatives (Not Implemented)

## Infrastructure Needed

Because EIPs are AWS objects, this proposal is valid only for the AWS environment.
