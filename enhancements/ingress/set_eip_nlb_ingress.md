---
title: set_eip_nlb_ingress
authors:
  - miheer
reviewers:
  - Miciah
  - frobware
  - gspence
  - candace
approvers:
  - Miciah
  - frobware
  - gspence
api-approvers:
  - joel
  - deads
creation-date: 2024-03-06
last-updated: 2024-03-22
tracking-link:
  - "https://issues.redhat.com/browse/NE-1274"
see-also:
  - ""
replaces:
  - ""
superseded-by:
  - ""
---

# Set AWS EIP For NLB Ingress Controller

## Summary

This enhancement allow user to set AWS EIP for the NLB default or custom ingress controller.
This is a feature request to enhance the IngressController API to be able to support static IPs during
- Install time
- Custom NLB ingress controller creation 
- Reconfiguration of the router.

## Motivation

### User Stories

- As an administrator, I want to provision EIPs and use them with an NLB IngressController.
- As a user or admin, I want to ensure EIPs are used with NLB on default router at install time.
- As a user, I want to reconfigure default router to use EIPs.
- As a user of OpenShift on AWS (ROSA), I want to use static IPs (and therefore AWS Elastic IPs) so that 
  I can configure appropriate firewall rules.
  I want the default AWS Load Balancer that they use (NLB) for their router to use these EIPs.

### Goals
- Users are able to use EIP for a NLB Ingress Controller.
- Users are able to create a NLB default Ingress Controller during install time with the EIPs specified during install.
- Any existing ingress controller should be able to be reconfigured to use EIPs. 

### Non-Goals

- Creation of EIPs in AWS.
- Keep a track of changes in the EIP in the AWS environment.
- Static IP usage with NLBs for API server.

## Proposal

In this enhancement we would be adding API fields in the installer and the ingress controller CRD
to set the AWS EIP for the NLB Ingress Controller. The cluster ingress operator will then create 
the ingress controller and get the EIP assigned to the service LoadBalancer with the annotation 
`service.beta.kubernetes.io/aws-load-balancer-eip-allocations`

### Workflow Description

**cluster administrator** is a human user responsible for deploying a
cluster. A cluster administrator also has the rights to modify the cluster level components.

#### Set EIP using an installer:
1. Cluster administrator creates adds the following in the install-config.yaml
```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  aws:
    region: us-west-2
    lbType: NLB
      networkLoadBalancer:
        eip-allocations: eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted> 
    subnets:
    - subnet-1
    - subnet-2
    - subnet-3
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```  
2. The installer will create the cluster ingress config object as follows:
```yaml
 oc edit ingress.config/cluster -o yaml
 # Please edit the object below. Lines beginning with a '#' will be ignored,
 # and an empty file will abort the edit. If an error occurs while saving this file will be
 # reopened with the relevant failures.
 #
 apiVersion: config.openshift.io/v1
 kind: Ingress
 metadata:
   creationTimestamp: "2022-08-30T01:03:17Z"
   generation: 11
   name: cluster
   resourceVersion: "60888"
   uid: de89cc95-449e-4ae5-a837-f4171fe61e36
 spec:
   domain: apps.testnlbapi.devcluster.openshift.com
   loadBalancer:
     platform:
       aws:
         type: NLB
           networkLoadBalancer:
             eip-allocations: eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>
       type: AWS
 status:
   componentRoutes:
```
3. Ingress Operator will create the default ingress controller with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`.

#### Create an Ingress Controller using Ingress Controller CR and assign the EIP allocation mentioned in the Ingress Controller CR.
1. Cluster administrator creates an Ingress Controller:
```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  creationTimestamp: null
  name: test
  namespace: openshift-ingress-operator
spec:
  endpointPublishingStrategy:
    loadBalancer:
      scope: External
      providerParameters:
        type: AWS
        aws:
          type: NLB
          networkLoadBalancer:
            eip-allocations: eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>,eipalloc-<redacted>
    type: LoadBalancerService
```

Ingress Operator will create an NLB Ingress Controller called `test` with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
populated with the value from the eip-allocations field.

#### Cluster administrator adds eip-allocations field the existing ingress controller

Ingress Operator will create an NLB Ingress Controller with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
populated with the value from the eip-allocations field.

#### Cluster administrator updates eip-allocations field the existing ingress controller

Ingress Operator will update an NLB Ingress Controller with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
populated with the value from the eip-allocations field.

### API Extensions
- The API for setting `eip-allocations` field in the installer-config will look as follows in the file `pkg/types/aws/platform.go`:

```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
...
    // https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html#nlb 
    // +optional 
    LBType configv1.AWSLBType `json:"lbType,omitempty"`

    // networkLoadBalancerParameters holds configuration parameters for an AWS
    // network load balancer. Present only if type is NLB.
    //
    // +optional
    NetworkLoadBalancerParameters *configv1.AWSNetworkLoadBalancerParameters `json:"networkLoadBalancer,omitempty"`
}

```

- The API for the `eip-allocations` field in the ingress cluster object to store the value of the
  field `eip-allocations` from the installer which the ingress operator uses to create the ingress controller
  will look as follows in the file `config/v1/types_ingress.go`:

```go
// AWSIngressSpec holds the desired state of the Ingress for Amazon Web Services infrastructure provider.
// This only includes fields that can be modified in the cluster.
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
    NetworkLoadBalancerParameters *AWSNetworkLoadBalancerParameters `json:"networkLoadBalancer,omitempty"`
}

// AWSNetworkLoadBalancerParameters holds configuration parameters for an
// AWS Network load balancer.
type AWSNetworkLoadBalancerParameters struct {
    EIPAllocations EIPAllocations `json:"eip-allocations,omitempty"`
}

type EIPAllocations string

```

- The API field for `eip-allocations` field in the Ingress Controller CR needs to be set as follows
  in the file `operator/v1/types_ingress.go`:
```go
// AWSNetworkLoadBalancerParameters holds configuration parameters for an
// AWS Network load balancer.
type AWSNetworkLoadBalancerParameters struct {
    EIPAllocations EIPAllocations `json:"eip-allocations,omitempty"`        
}

type EIPAllocations string

```

### Topology Considerations

#### Hypershift / Hosted Control Planes

Currently, there is no unique consideration for making this change work with Hypershift.

#### Standalone Clusters

Is the change relevant for standalone clusters?

#### Single-node Deployments or MicroShift

This proposal does not affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory.

This proposal does not affect MicroShift. The proposal
adds configuration options through API resources and the installer however, it is not
required for any of those behaviors to be exposed to MicroShift admins through the
configuration file for MicroShift.

### Implementation Details/Notes/Constraints

#### 1. Set EIP through installer for the default ingress controller:
   
   a. The admin sets the `eip-allocations` in the installer using the install-config.yaml.
   
   b. The installer then sets the ingress cluster object with the `eip-allocations`.
   
   c. The cluster ingress operator then creates a default ingress controller by:

     i.   creating an default ingress controller CR
     
     ii.  then scaling a deployment for the Ingress Controller CR which has the value of the `eip-allocations`.
     
     iii. then creating a service with service type load balancer with annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
          which uses the value from the field `eip-allocations` of ingress controller CR.

#### 2. Set EIP through the Ingress Controller CRD:
   
   The admin sets the `eip-allocations` in the Ingress Controller CR. Then same process
   as per point 1.c.ii and 1.c.iii is followed by the ingress operator. 

#### 3. Update the existing EIP for the IngressController:
   
   When the admin changes the eip-allocations field in the Ingress Controller CR, the watch on the
   `eip-allocations` field by the ingress operator scales a new deployment for the Ingress Controller CR.
   Then step 1.c.iii is followed by the ingress operator.

   
### Risks and Mitigations

Providing a right range of EIP as per AWS guidelines is an admin responsibility.
If they are not provided the router won't get assigned an IP which can cause communication outage.
As of now we are not able to identify any more risks which can affect other components of the EP.

### Drawbacks

Currently, there are not any drawbacks as such for this EP.

## Open Questions

1. Shall we support updating EIP in the NLB Ingress Controller ?

Yes, we will support updating the EIP in the custom ingress controller.

## Test Plan

- This EP will be covered by unit tests.

- This  EP will also be covered by end to end tests.
  
  The end to end test will cover the scenario where user will set an `eip-allocations` field
  in the Ingress Controller CR and then test if the Ingress Controller
  with service type load balancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
  has value of `eip-allocations` field from the Ingress Controller CR was created.

  The end to end test will also the cover the scenario where user updates the `eip-allocations` field
  in the Ingress Controller CR and then test if the Ingress Controller
  with service type load balancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
  has value of `eip-allocations` field from the Ingress Controller CR was updated.
  

## Graduation Criteria

### Dev Preview -> Tech Preview

- N/A

### Tech Preview -> GA

- N/A

### Removing a deprecated feature

- N/A

## Upgrade / Downgrade Strategy

During upgrade, user should be able to populate the eip_allocations_field via installer

During downgrade, the EIPs won't be allocated to the ingress controller's load balancer service.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

- If the Ingress Operator can't scale an Ingress Controller with `eip-allocations` then
  there can be an outage as all the communications to the router will fail.

- In case of event of an outage because of this API the network edge team needs to be involved
  in.

## Support Procedures

- If the service is not getting updated with the EIP annotation check the `kube-controller-manager` logs.
  ```sh
  oc logs <kcm pod name> -n openshift-kube-controller-manager
  ```
- If router pods fail to run then check the following:
   - `Ingress Controller` logs.
  ```sh
  oc logs <ingress pod name> -n openshift-ingress
  ```  
   - `Ingress Operator` logs.
  ```sh
  oc logs <ingress operator pod> -n openshift-ingress-operator
  ```

## Alternatives

N/A

## Infrastructure Needed 

This EP works in AWS environment as AWS EIPs work on AWS environment only.
