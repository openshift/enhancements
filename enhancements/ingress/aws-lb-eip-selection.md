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
api-approvers:
- None
creation-date: 2024-05-29
last-updated: 2024-09-04
tracking-link:
  - https://issues.redhat.com/browse/CORS-3440
see-also:
  - "enhancements/ingress/lb-subnet-selection-aws.md"
replaces:
  - "enhancements/installer/aws-customer-provided-subnets.md"
superseded-by:
---

# Installer LoadBalancer EIP Selection for AWS

## Summary

This enhancement extends the OpenShift Installer's install-config, enabling cluster admins to
configure EIPs for AWS NLB load balancer created for their default NLB IngressController at install time.
This proposal allows the install-time configuration of subnets for the `default` IngressController.

## Definitions and Terminology

### The `default` IngressController

This proposal will refer to the IngressController that gets created automatically during installation and handles
the platform routes (console, auth, canary, etc.) as the `default` IngressController.

### CCM 
CCM refers to Cloud Controller Manager which calls [AWS LoadBalancer Controller](https://github.com/kubernetes-sigs/aws-load-balancer-controller) to create AWS LBs.

### Some more terminologies
EIP = AWS Elastic IP, NLB = AWS Network LoadBalancer, AZ = Availability Zone, LB = LoadBalancer


## Motivation

### User Stories
- As a cluster administrator using installer, I want to configure default NLB IngressController to use EIPs.
- As a cluster administrator of OpenShift on AWS (ROSA) using installer, I want to use static IPs (and therefore AWS Elastic IPs) so that
  I can configure appropriate firewall rules.
  I want the default AWS Load Balancer that they use (NLB) for their router to use these EIPs.
- As a cluster administrator who has purchased a block of public IPv4 addresses, I want to use these IP addresses, so
  that I can avoid Amazon's new charge for public IP addresses.

### Goals
- Users are able to use EIPs for a default NLB `IngressController` at install time.
- Check for unassociated EIPs before passing to CCM.

### Non-Goals
- Creation of EIPs in AWS.
- Static IP usage with NLBs for OpenShift API server, DNS, Nat Gateways, LBs, Instances.
- To assign IPs from a Customer Owned IP (CoIP) Pool when using Outposts.

## Proposal
This enhancement adds API fields in the installer and the IngressController specification
to set the AWS EIP for the default NLB `IngressController`. The cluster ingress operator will then create
the default `IngressController` and get the EIP assigned to the service LoadBalancer with the annotation
`service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
Note: `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` is a standard [Kubernetes service annotation](https://kubernetes.io/docs/reference/labels-annotations-taints/#service-beta-kubernetes-io-aws-load-balancer-eip-allocations)

### API Extensions

#### Installer Updates
- The first API extension for setting `eipAllocations` is in the installer [Platform](https://github.com/openshift/installer/blob/master/pkg/types/aws/platform.go) type, where the new field `NetworkLoadBalancerParameters` is added as an optional field.

```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
...

    // eipAllocations holds eipAllocations for an default AWS
    // NLB IngressController.
    //
    // +optional
    EIPAllocations *EIPAllocations `json:"eipAllocations,omitempty"`
}

// EIPAllocations holds configuration parameters for an
// default AWS NLB IngressController. For Example: Setting AWS EIPs https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html
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

#### IngressConfig API Updates

- The installer will propagate `eipAllocations` from `install-config.yaml` to the cluster ingress configuration, which the ingress operator uses to create the default IngressController.  
  The following  definitions will be added in the file [`config/v1/types_ingress.go`](https://github.com/openshift/api/blob/master/config/v1/types_ingress.go) of the [`openshift/api`](https://github.com/openshift/api) GitHub repository:

```go
// AWSIngressSpec holds the desired state of the Ingress for Amazon Web Services infrastructure provider.
// This only includes fields that can be modified in the cluster.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=SetEIPForNLBIngressController,rule="self.type == 'NLB' ? true : !has(self.networkLoadBalancer)",message="Network Load Balancer parameters are allowed only when load balancer type is NLB."
// +union
type AWSIngressSpec struct {
...
    // +unionDiscriminator
    // +kubebuilder:validation:Enum:=NLB;Classic
    // +kubebuilder:validation:Required
    Type AWSLBType `json:"type,omitempty"`

    // networkLoadBalancerParameters holds configuration parameters for an AWS
    // network load balancer. Present only if type is NLB.
    //
    // +optional
    NetworkLoadBalancerParameters *AWSNetworkLoadBalancerParameters `json:"networkLoadBalancerParameters,omitempty"`
}

// AWSNetworkLoadBalancerParameters holds configuration parameters for an
// AWS Network load balancer. For Example: Setting AWS EIPs https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html
type AWSNetworkLoadBalancerParameters struct {
    // eipAllocations is a list of IDs for Elastic IP (EIP) addresses that
    // are assigned to the Network Load Balancer.
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
    // +openshift:validation:FeatureGateAwareXValidation:featureGate=InstallEIPForDefaultNLBIngressController,rule=`self.all(x, self.exists_one(y, x == y))`,message="eipAllocations cannot contain duplicates"
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="eipAllocations cannot contain duplicates"
    // +kubebuilder:validation:MaxItems=10
    EIPAllocations []EIPAllocation `json:"eipAllocations"`
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
which could be then used in the `eipAllocations` field of the IngressController CR.
Note: The AWS account has some cap on the number of EIPs per region. By default, all AWS accounts have a quota of five (5) Elastic IP addresses per Region, because public (IPv4)
internet addresses are a scarce public resource. For more details please check [Elastic IP address quota](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html#using-instance-addressing-limit).
When this cap is hit you will get the following error: `Elastic IP address could not be allocated.
The maximum number of addresses has been reached.`
Elastic IP Allocation can only be done for internet-facing NLB.
If you are using an already available eipAllocation then it should be an `UnAssociated` one.

For installer-created-VPCs, a cluster admin can accurately predict how many EIPAllocations to bring so their cluster doesn't fail to install as follows as they might not 
know what public subnets will be created by the openshift-installer.

To illustrate further, the workflow could start like:

Cluster admin wants to use EIPs, but isn't bringing their own VPC, so they need to know exactly how many EIPs to create.
Cluster admin should review https://aws.amazon.com/about-aws/global-infrastructure/regions_az/ to find out how many AZs are in the region they are installing into.
Cluster admin should use the same number of EIPs as AZs in their region.
Cluster admins probably know how many AZs they are installing into, but a workflow like this would provide a concrete example of how this would happen if they don't.

**cluster administrator** is a human user responsible for deploying a
cluster. A cluster administrator also has the rights to modify the cluster level components.

#### Set EIP using an installer:
1. Cluster administrator creates `install-config.yaml` and adds their domain, AWS region, and EIP allocation ids.
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

2. The installer will create the cluster ingress config object as follows:
```yaml
 oc edit ingress.config/cluster -o yaml
 ...
 apiVersion: config.openshift.io/v1
 kind: Ingress
 metadata:
...
   name: cluster
...
 spec:
   domain: apps.eip.devcluster.openshift.com
   loadBalancer:
     platform:
       aws:
         type: NLB
         eipAllocations:
           ingressNetworkLoadBalancer:
           - eipalloc-1234567890abcdef1
           - eipalloc-1234567890abcdef2
           - eipalloc-1234567890abcdef3
           - eipalloc-1234567890abcdef4
           - eipalloc-1234567890abcdef5
       type: AWS
```
3. Ingress Operator will create the default `IngressController` with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` with values from the field `eipAllocations`
   from the cluster ingress config object.

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

1. #### Set EIP through installer for the default IngressController:
    1. The admin sets the `eipAllocations` in the installer using the install-config.yaml.
    2. The installer then sets the ingress cluster object with the `eipAllocations`.
    3. The cluster ingress operator then creates a default `IngressController` by:
        1. creating an default `IngressController` CR.
        2. then scaling a deployment for the `IngressController` CR which has the value of the `eipAllocations` from the
           ingress cluster object.
        3. then creating a service of service type load balancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`,
           which uses the value from the field `eipAllocations` of `IngressController` CR.

2. #### Validation on installer when installing in managed VPC (full-automated) based in the discovered zones used to create the cluster.
     We will be comparing the number of Availability Zones in the region to the number of eipAllocations passed in the `install-config.yaml`.

3. #### Validation on installer when installing in unmanaged (BYO VPC)
     We will be comparing if the number of public subnets added to the install-config matches with len(eipAllocations).
     However, The problem is that the AWS CCM can select subnets that aren't provided in the BYO Subnets, see https://issues.redhat.com/browse/OCPBUGS-17432.
Here's one recommended option:

The Installer should count all LB subnets by predicting what subnets be chosen by the AWS CCM 
(i.e. any subnet without another cluster's kubernetes.io/cluster/<cluster-id> tag). 
We can call this Predicted LB Subnet Count.

We can examine the following scenarios:

##### BYO Subnet Count != EIPs Allocations && BYO Subnet Count == Predicted LB Subnet count && Predicted LB Subnet count != EIPs Allocations:

Throw a simple error that just says, EIP != Provided Subnets:
The number of EIP Allocations does not equal the number of provided Subnets, the cluster will fail.

##### BYO Subnet Count == EIPs Allocations && BYO Subnet Count != Predicted LB Subnet count && Predicted LB Subnet count != EIPs Allocations:

Throw a error that says something to the extend of that the LB will select more subnets and the cluster will fail:
Or though the number of EIP Allocations equals the number of provided Subnets, it does not equal the number of Subnets that will be selected by
the LoadBalancer and the cluster will fail, please review the kubernetes.io/cluster/<cluster-id> subnet tags are set correctly.

##### BYO Subnet Count != EIPs Allocations && BYO Subnet Count != Predicted LB Subnet count && Predicted LB Subnet count == EIPs Allocations:

This is an odd scenario. The user got the # of a EIPs == Predicted LB Subnet count, I suppose because they anticipated the AWS CCM's generous 
selection of subnets. This is valid scenario for no error message.

##### BYO Subnet Count == EIPs Allocations && BYO Subnet Count == Predicted LB Subnet count && Predicted LB Subnet count == EIPs Allocations:

Obvious valid scenario.

I think we need some feedback from installer-team, @patrickdillon or @mtulio or @sadasu on this type of validation. Should the installer team consider
the Predicted LB Subnets != BYO Subnet Count scenario as not valid? And possibly block future installs as a resolution
to https://issues.redhat.com/browse/OCPBUGS-17432? That would make EIP Allocation a lot easier, but not sure if that's realistic. 

4. #### Validation to check if EIPs are not already assigned to resources.
EIPs can be assigned to many resource types, like Nat Gateways, *LBs, Instances, etc. The attribute associationId will be set when the EIP is already associated.
To mitigate this we could add that validation, at least on installer, to provide quick-feedback (fail when validate install-config) to the user when the provided EIP is already associated to another resource.
It would be nice to have a validation before setting the annotation to CCM, keeping the operator degraded before disrupting the service.

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

## Alternatives

## Infrastructure Needed

This EP works in AWS environment as AWS EIPs work on AWS environment only.
