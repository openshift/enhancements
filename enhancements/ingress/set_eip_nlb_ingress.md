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
last-updated: 2024-05-13
tracking-link:
  - "https://issues.redhat.com/browse/NE-1274"
see-also:
  - ""
replaces:
  - ""
superseded-by:
  - ""
---

# Set AWS EIP For NLB IngressController

## Summary

This enhancement allows a user to configure AWS Elastic IP(EIP) for an AWS Network Load Balancer(NLB) default or custom IngressController.
This is a feature request to enhance the IngressController API to be able to support static IPs during
- Custom NLB IngressController creation 
- Reconfiguration of the custom and default router to NLB with EIP allocation.

## Motivation

### User Stories

- As a cluster administrator, I want to provision EIPs and use them with an NLB IngressController.
- As a cluster administrator, I want to reconfigure default router to use EIPs.
- As a cluster administrator of OpenShift on AWS (ROSA), I want to use static IPs (and therefore AWS Elastic IPs) so that 
  I can configure appropriate firewall rules.
  I want the default AWS Load Balancer that they use (NLB) for their router to use these EIPs.
- As a cluster administrator who has purchased a block of public IPv4 addresses, I want to use these IP addresses, so 
  that I can avoid Amazon's new charge for public IP addresses.

### Goals
- Users are able to use EIP for a NLB `IngressController`.
- The load balancer of an existing `IngressController` can be reconfigured to use specific EIPs by updating the `IngressController` specification and recreating the load balancer as type NLB.

### Non-Goals

- Creation of EIPs in AWS.
- Monitoring or management of changes to EIPs in the AWS environment.
- Static IP usage with NLBs for OpenShift API server.
- Check the number of public subnets available in AWS environment and then compare number of subnets
  with the number of EIPs the user provides. Note - If subnets are provided in the IngressController CR
  we do compare the subnets with the number of eipAllocations provided in the IngressController CR. 

## Proposal

EIP = AWS Elastic IP, NLB = AWS Network LoadBalancer, AZ = Availability Zone 

This enhancement adds API fields in the IngressController specification
to set the AWS EIP for the NLB `IngressController`. The cluster ingress operator will then create 
the `IngressController` and get the EIP assigned to the service LoadBalancer with the annotation 
`service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
Note: `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` is a standard [Kubernetes service annotation](https://kubernetes.io/docs/reference/labels-annotations-taints/#service-beta-kubernetes-io-aws-load-balancer-eip-allocations)

### API Extensions
The IngressController API is extended by adding an optional parameter EIPAllocations of type []EIPAllocation to the AWSNetworkLoadBalancerParameters struct,
to manage the `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` on the LoadBalancer-type service.
As this feature is related to setting an annotation related to AWS we made this API specific to AWS by adding the configuration under `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws`.

- CEL to check if eipAllocations are allowed to set only when load balancer scope is `External`
```go
// LoadBalancerStrategy holds parameters for a load balancer.
// +kubebuilder:validation:XValidation:rule="!has(self.scope) || self.scope != 'Internal' || !has(self.providerParameters) || !has(self.providerParameters.aws) || !has(self.providerParameters.aws.networkLoadBalancer) || !has(self.providerParameters.aws.networkLoadBalancer.eipAllocations)",message="eipAllocations are forbidden when the scope is Internal."
type LoadBalancerStrategy struct {

```

- The API field for `eipAllocations` field in the `IngressController` CR needs to be set as follows
  in the file [operator/v1/types_ingress.go](https://github.com/openshift/api/blob/84047ef4a2ce54dc7f879b1382690079081128f1/operator/v1/types_ingress.go):
  Validations performed are as follows:
  - The number of subnets provided are matched with the eipAllocation provided.
  - A maximum of 10 EIP allocations are permitted.
  - Each EIP allocation must be unique.
  - Values for `eipAllocations` must begin with `eipalloc-` followed by exactly 17 hexadecimal (`[0-9a-fA-F]`) characters.
  - Min and Max length for the values in `eipAllocations` is 26 chars.

```go
// AWS Network load balancer. For Example: Setting AWS EIPs https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html
// +kubebuilder:validation:XValidation:rule=`has(self.subnets) && has(self.subnets.ids) && has(self.subnets.names) && has(self.eipAllocations) ? size(self.subnets.ids + self.subnets.names) == size(self.eipAllocations) : true`,message="number of subnets must be equal to number of eipAllocations"
// +kubebuilder:validation:XValidation:rule=`has(self.subnets) && has(self.subnets.ids) && !has(self.subnets.names) && has(self.eipAllocations) ? size(self.subnets.ids) == size(self.eipAllocations) : true`,message="number of subnets must be equal to number of eipAllocations"
// +kubebuilder:validation:XValidation:rule=`has(self.subnets) && has(self.subnets.names) && !has(self.subnets.ids) && has(self.eipAllocations) ? size(self.subnets.names) == size(self.eipAllocations) : true`,message="number of subnets must be equal to number of eipAllocations"
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
    //	+openshift:enable:FeatureGate=SetEIPForNLBIngressController
    // +optional
    // +listType=atomic
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

The administrator needs to know how many public subnets the AWS VPC ID which is tagged by cluster name after installation.

To locate the public subnets:

Navigate to your AWS Web Console for the account that contains the cluster
Go to Your VPCs under the Virtual Public Cloud section
Search for the VPC ID tagged with cluster name using the command oc get infrastructure cluster -o jsonpath='{.status.infrastructureName}'
Click the VPC ID link for the VPC
Click the link under Main route table and then check the column Explicit subnet associations which will show you the number of public subnets available for that VPC ID.
One can try these scripts present [here](https://github.com/frobware/haproxy-hacks/tree/master/NE-1398) as well.

**cluster administrator** is a human user responsible for deploying a
cluster. A cluster administrator also has the rights to modify the cluster level components.

#### Create an IngressController using IngressController custom resource and assign the EIP allocation to the Kubernetes load balancer service for this IngressController.
1. Cluster administrator creates an `IngressController`:
```yaml
 % cat custom-eip-cr.yaml
 apiVersion: operator.openshift.io/v1
 kind: IngressController
 metadata:
   creationTimestamp: null
   name: test
   namespace: openshift-ingress-operator
 spec:
   domain: test.apps.eip.devcluster.openshift.com
   endpointPublishingStrategy:
     loadBalancer:
       scope: External
       providerParameters:
         type: AWS
         aws:
           type: NLB
           networkLoadBalancer:
             eipAllocations:
               - eipalloc-0956fea34de4cb7ab
               - eipalloc-0e9a3077a70de050a
               - eipalloc-0b69fc4691f54cdd0
               - eipalloc-01e6ba6cbba1a391b
               - eipalloc-0e242df173f906112
     type: LoadBalancerService
```
```sh
% oc create -f custom-eip-cr.yaml
ingresscontroller.operator.openshift.io/test created
```
```sh
% oc get ingresscontrollers/test -n openshift-ingress-operator
NAME   AGE
test   118s
```

Ingress Operator will create an NLB `IngressController` called `test` with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
populated with the value from the eipAllocations field.

```yaml
% oc get ingresscontroller test -n openshift-ingress-operator -o yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
...
  finalizers:
  - ingresscontroller.operator.openshift.io/finalizer-ingresscontroller
...
  name: test
  namespace: openshift-ingress-operator
...
spec:
  clientTLS:
    clientCA:
      name: ""
    clientCertificatePolicy: ""
  domain: test.apps.eip.devcluster.openshift.com
  endpointPublishingStrategy:
    loadBalancer:
      dnsManagementPolicy: Managed
      providerParameters:
        aws:
          networkLoadBalancer:
            eipAllocations:
            - eipalloc-0956fea34de4cb7ab
            - eipalloc-0e9a3077a70de050a
            - eipalloc-0b69fc4691f54cdd0
            - eipalloc-01e6ba6cbba1a391b
            - eipalloc-0e242df173f906112
          type: NLB
        type: AWS
      scope: External
    type: LoadBalancerService
...
status:
...
  - lastTransitionTime: "2024-04-10T00:20:22Z"
    message: The LoadBalancer service is provisioned
    reason: LoadBalancerProvisioned
    status: "True"
    type: LoadBalancerReady
  - lastTransitionTime: "2024-04-10T00:20:17Z"
    message: LoadBalancer is not progressing
    reason: LoadBalancerNotProgressing
    status: "False"
    type: LoadBalancerProgressing
...
  - lastTransitionTime: "2024-04-10T00:20:50Z"
    status: "True"
    type: Available
  - lastTransitionTime: "2024-04-10T00:20:50Z"
    status: "False"
    type: Progressing
  - lastTransitionTime: "2024-04-10T00:20:50Z"
    status: "False"
    type: Degraded
...
  - lastTransitionTime: "2024-04-10T00:20:17Z"
    message: No evaluation condition is detected.
    reason: NoEvaluationCondition
    status: "False"
    type: EvaluationConditionsDetected
  domain: test.apps.eip.devcluster.openshift.com
  endpointPublishingStrategy:
    loadBalancer:
      dnsManagementPolicy: Managed
      providerParameters:
        aws:
          networkLoadBalancer:
            eipAllocations:
            - eipalloc-0956fea34de4cb7ab
            - eipalloc-0e9a3077a70de050a
            - eipalloc-0b69fc4691f54cdd0
            - eipalloc-01e6ba6cbba1a391b
            - eipalloc-0e242df173f906112
          type: NLB
        type: AWS
      scope: External
    type: LoadBalancerService
  observedGeneration: 2
  selector: ingresscontroller.operator.openshift.io/deployment-ingresscontroller=test
...
```

The router-test service will now show the updated service.beta.kubernetes.io/aws-load-balancer-eip-allocations annotation values.
```yaml
 % oc get svc router-test -n openshift-ingress -o yaml
 apiVersion: v1
 kind: Service
 metadata:
   annotations:
     service.beta.kubernetes.io/aws-load-balancer-eip-allocations: eipalloc-0956fea34de4cb7ab,eipalloc-0e9a3077a70de050a,eipalloc-0b69fc4691f54cdd0,eipalloc-01e6ba6cbba1a391b,eipalloc-0e242df173f906112
...
   name: router-test
   namespace: openshift-ingress
...
 spec:
...
   type: LoadBalancer
 status:
   loadBalancer:
     ingress:
       - hostname: a812fb12c893244bc87fdc257b7c0a45-e77e94821922b90d.elb.us-east-1.amazonaws.com
```

#### Cluster administrator updates eipAllocations field of the existing IngressController

AWS platform requires deleting and recreating a load balancer to change its EIPs.
This operation disrupts ingress traffic and may cause the load balancer's address
to change. Therefore, after changing EIPs on an existing `IngressController`, it changes its own status to `Progressing` to signal that the user must assist with the update by deleting the Kubernetes Service of type `LoadBalancer` object in order to propagate the changes.
Once the user performs this step, the operator recreates the load balancer with the new EIPs to complete the operation. By adding this safeguard, the disruption can be scheduled for a maintenance window, if needed.

In order to signal that the user must take action, the operator sets the Progressing status condition to True with a message that
provides instructions for how to effectuate the change:
```yaml
 % oc get ingresscontroller test -n openshift-ingress-operator -o yaml
 apiVersion: operator.openshift.io/v1
 kind: IngressController
 metadata:
...
   name: test
   namespace: openshift-ingress-operator
...
 spec:
...
   endpointPublishingStrategy:
     loadBalancer:
       dnsManagementPolicy: Managed
       providerParameters:
         aws:
           networkLoadBalancer:
             eipAllocations:
               - eipalloc-0387f99f5d4724c3e
               - eipalloc-0b09650c180c2abb6
               - eipalloc-0161deab2f05fe2fe
               - eipalloc-0ec5738e0e3808b8a
               - eipalloc-09d56b78479ac651d
           type: NLB
         type: AWS
       scope: External
     type: LoadBalancerService
 status:
   availableReplicas: 2
   conditions:
...
     - lastTransitionTime: "2024-04-10T00:20:22Z"
       message: The LoadBalancer service is provisioned
       reason: LoadBalancerProvisioned
       status: "True"
       type: LoadBalancerReady
     - lastTransitionTime: "2024-04-10T00:43:58Z"
       message: 'One or more managed resources are progressing: The IngressController
      eips were changed from [eipalloc-0956fea34de4cb7ab,eipalloc-0e9a3077a70de050a,eipalloc-0b69fc4691f54cdd0,eipalloc-01e6ba6cbba1a391b,eipalloc-0e242df173f906112]
      to [eipalloc-0387f99f5d4724c3e,eipalloc-0b09650c180c2abb6,eipalloc-0161deab2f05fe2fe,eipalloc-0ec5738e0e3808b8a,eipalloc-09d56b78479ac651d].  To
      effectuate this change, you must delete the service: `oc -n openshift-ingress
      delete svc/router-test`; the service load-balancer will then be deprovisioned
      and a new one created.  This will most likely cause the new load-balancer to
      have a different host name and IP address from the old one''s.  Alternatively,
      you can revert the change to the IngressController: `oc -n openshift-ingress-operator
      patch ingresscontrollers/test --type=merge --patch=''{"spec":{"endpointPublishingStrategy":{"loadBalancer":{"providerParameters":{"type":"AWS","aws":{"type":"NLB","eipAllocations":["eipalloc-0956fea34de4cb7ab","eipalloc-0e9a3077a70de050a","eipalloc-0b69fc4691f54cdd0","eipalloc-01e6ba6cbba1a391b","eipalloc-0e242df173f906112"]}}}}}}'''
       reason: OperandsProgressing
       status: "True"
       type: LoadBalancerProgressing
     - lastTransitionTime: "2024-04-10T00:20:50Z"
       status: "True"
       type: Available
     - lastTransitionTime: "2024-04-10T00:43:58Z"
       message: 'One or more status conditions indicate progressing: LoadBalancerProgressing=True
      (OperandsProgressing: One or more managed resources are progressing: The IngressController
      eips were changed from [eipalloc-0956fea34de4cb7ab,eipalloc-0e9a3077a70de050a,eipalloc-0b69fc4691f54cdd0,eipalloc-01e6ba6cbba1a391b,eipalloc-0e242df173f906112]
      to [eipalloc-0387f99f5d4724c3e,eipalloc-0b09650c180c2abb6,eipalloc-0161deab2f05fe2fe,eipalloc-0ec5738e0e3808b8a,eipalloc-09d56b78479ac651d].  To
      effectuate this change, you must delete the service: `oc -n openshift-ingress
      delete svc/router-test`; the service load-balancer will then be deprovisioned
      and a new one created.  This will most likely cause the new load-balancer to
      have a different host name and IP address from the old one''s.  Alternatively,
      you can revert the change to the IngressController:  `oc -n openshift-ingress-operator
      patch ingresscontrollers/test --type=merge --patch=''{"spec":{"endpointPublishingStrategy":{"loadBalancer":{"providerParameters":{"type":"AWS","aws":{"type":"NLB","eipAllocations":["eipalloc-0956fea34de4cb7ab","eipalloc-0e9a3077a70de050a","eipalloc-0b69fc4691f54cdd0","eipalloc-01e6ba6cbba1a391b","eipalloc-0e242df173f906112"]}}}}}}'''
       reason: IngressControllerProgressing
       status: "True"
       type: Progressing
     - lastTransitionTime: "2024-04-10T00:20:50Z"
       status: "False"
       type: Degraded
...
   domain: test.apps.eip.devcluster.openshift.com
   endpointPublishingStrategy:
     loadBalancer:
       dnsManagementPolicy: Managed
       providerParameters:
         aws:
           networkLoadBalancer:
             eipAllocations:
               - eipalloc-0956fea34de4cb7ab
               - eipalloc-0e9a3077a70de050a
               - eipalloc-0b69fc4691f54cdd0
               - eipalloc-01e6ba6cbba1a391b
               - eipalloc-0e242df173f906112
           type: NLB
         type: AWS
       scope: External
     type: LoadBalancerService
...

```

#### Cluster admin wants operator to automatically effectuate the Load Balancer service for updated IngressController eipAllocations 

The administrator can tell the Ingress Operator to complete the disruptive update of the service automatically
by annotating the IngressController with the annotation called `ingress.operator.openshift.io/auto-delete-load-balancer` 
and then changing the eipAllocations by using the following commands:

Annotate `IngressController` with `ingress.operator.openshift.io/auto-delete-load-balancer` with a blank value.
```sh
oc -n openshift-ingress-operator annotate ingresscontrollers/test ingress.operator.openshift.io/auto-delete-load-balancer=
```

Update the `IngressController` with the following command:

```sh
% oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge --patch='{"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService","loadBalancer":{"providerParameters":{"type":"AWS","aws":{"type":"NLB","nlbParameters":{"eipAllocations":[]}}}}}}}'
```

After the changes are saved, the operator will delete and create the load balancer service automatically.
```sh
% oc get svc router-test -n openshift-ingress 
NAME          TYPE           CLUSTER-IP       EXTERNAL-IP                                                                     PORT(S)                      AGE
router-test   LoadBalancer   172.30.230.109   a95d070adbbf045c9b356ed9db8d3cda-89fbf4dfc638408e.elb.us-east-1.amazonaws.com   80:30453/TCP,443:32704/TCP   77s
```

If you remove all EIPs, the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` is removed from the service as well.


#### Unmanaged EIPAllocation Annotation Migration Workflow

Migrating an unmanaged `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
service annotation to `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.networkLoadBalancerParameters.eipAllocations`
after upgrading to 4.y doesn't require a cluster admin to delete the LoadBalancer-type
service:

1. Cluster admin confirms that initially `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
   is set on the LoadBalancer-type service managed by the IngressController.
2. The cluster admin upgrades OpenShift to v4.y and the service annotation is not
   changed or removed. For more details see [section](#4-any-change-to-the-service-annotation-does-not-trigger-the-operator-to-manage-the-load-balancer-service)  
3. After upgrading, the IngressController will emit a `LoadBalancerProgressing` condition
   with `Status: True` because the spec's `EIPAllocations` (an empty slice `[]`) does not equal
   the current annotation.
4. In this case, the cluster admin must directly update
   `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.networkLoadBalancerParameters.eipAllocations` to the
   current value of the `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` service.
5. The Ingress Operator will resolve the `LoadBalancerProgressing` condition back to
   `Status: False` as long as `EIPAllocations` is equivalent to `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`.

#### Updating EIP in the default Classic IngressController after installation

We support updating the EIP in the default Classic `IngressController`.

Procedure:
Add the `ingress.operator.openshift.io/auto-delete-load-balancer` to the `IngressController`.
```sh
% oc -n openshift-ingress-operator annotate ingresscontrollers/default ingress.operator.openshift.io/auto-delete-load-balancer=
ingresscontroller.operator.openshift.io/default annotated
```

Add the following under spec:
```yaml
spec:
  endpointPublishingStrategy:
    loadBalancer:
      dnsManagementPolicy: Managed
      providerParameters:
        aws:
          networkLoadBalancer:
            eipAllocations:
            - eipalloc-0956fea34de4cb7ab
            - eipalloc-0e9a3077a70de050a
            - eipalloc-0b69fc4691f54cdd0
            - eipalloc-01e6ba6cbba1a391b
            - eipalloc-0e242df173f906112
          type: NLB
        type: AWS
      scope: External
    type: LoadBalancerService
```

Check if the router-default service was created with proper annotations.

```yaml
% oc get svc router-default -n openshift-ingress -o yaml         
apiVersion: v1
kind: Service
metadata:
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-eip-allocations: eipalloc-0956fea34de4cb7ab,eipalloc-0e9a3077a70de050a,eipalloc-0b69fc4691f54cdd0,eipalloc-01e6ba6cbba1a391b,eipalloc-0e242df173f906112
...
  type: LoadBalancer
status:
  loadBalancer:
    ingress:
    - hostname: a8491585620154d5992dcdecc06c5779-7938285b62f7bebe.elb.us-east-1.amazonaws.com
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

Currently, there is no unique consideration for making this change work with Hypershift.

#### Standalone Clusters

This proposal does not have any special implications for standalone clusters.

#### Single-node Deployments or MicroShift

This proposal does not have any special implications for single-node
deployments or MicroShift.

### Implementation Details/Notes/Constraints

The process in general works as follows:
- cluster-admin sets spec.
- desiredLoadBalancerService sets the annotation based on spec.
- syncIngressControllerStatus sets status based on the annotation.

If the cluster-admin manually sets the annotation, then in that case syncIngressControllerStatus will reflect it to status.
More details on the implementation are as follows:

#### 1. Create a new IngressController with EIPs:

   The admin sets the `eipAllocations` in the `IngressController` CR. The cluster ingress operator then creates a `IngressController` by:
   1. scaling a deployment for the `IngressController` CR which has the value of the `eipAllocations` from the
      ingress cluster object.
   2. then creating a service of service type load balancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`,
      which uses the value from the field `eipAllocations` of `IngressController` CR.  

#### 2. Update the EIP for an existing IngressController:
   ##### Effectuating EIP Updates using manual method:
   When the admin changes the eipAllocations field in the `IngressController` CR, the ingress operator sets the Load Balancer
   Progressing status as per [section](#cluster-administrator-updates-eipallocations-field-of-the-existing-ingress-controller).
   The cluster admin has to manually delete service in order to effectuate the change.
   Then step 1.2 is followed by the ingress operator.
   We have intentionally designed this feature to require manual deletion of the service for the following reasons:

   Reason #1: To mitigate risks associated with cluster admins providing an invalid annotation value.
   While the AWS Cloud Controller Manager (CCM) handles invalid annotations by producing errors and events, it is difficult 
   for the Ingress Operator to decipher these CCM events after the load balancer has been provisioned. 
   Therefore, the Ingress Operator can't indicate to a cluster admin that an IngressController's load balancer is in a 
   malfunctioning state (i.e., the service is not getting reconciled) while the service is already provisioned.

   When the IngressController is first created and the LoadBalancer-type service is not yet provisioned, 
   these same invalid `eipAllocation` values will prevent the load balancer from being provisioned. 
   The existing Ingress Operator logic will clearly indicate to the cluster admin that the load balancer failed to
   provision via the LoadBalancerReady status condition. In addition, the LoadBalancerReady condition will include the CCM 
   event logs that indicate to the cluster admin that the `eipAllocations` value is invalid (see Support Procedures for examples).
   Therefore, requiring manual service deletion provides a way for the Ingress Operator to produce a clear signal to the
   cluster admin that `eipAllocations` is invalid.

   See [Invalid EIPAllocation Annotation Values](#invalid-eipallocation-values) for examples of invalid `eipAllocation` annotation values.

   Reason #2: To mitigate upgrade compatibility issues.
   Note: Directly configuring the `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` on the LoadBalancer-type service is not
   supported and never has been. However, this enhancement is designed to prevent cluster disruption upon upgrading with this
   unsupported configuration.

   If the `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` annotation was previously configured and the cluster is upgraded,
   the default value [] for EIPAllocations will clear the annotation for existing IngressControllers, which would break cluster ingress.
   However, requiring the service to be deleted to effectuate a `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.networkLoadBalancer.eipAllocations`
   value different from the current value of the annotation prevents this automatic removal of the service annotation on upgrade.

   Note: Cluster admins upgrading with an unmanaged EIP annotation don't need to delete the service to propagate the `eipAllocation` values, 
   instead they can follow the [Unmanaged EIPAllocation Annotation Migration Workflow](#unmanaged-eipallocation-annotation-migration-workflow).

   Reason #3: The CCM Doesn't Reconcile NLB `eipAllocation` Updates after creation.
   The CCM doesn't reconcile `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` annotation updates to NLBs 
   once it has been created. However, requiring a service deletion will effectuate these changes.

   See [CCM Doesn't Reconcile NLB EIPAllocation Updates](#ccm-doesnt-reconcile-nlb-eipallocation-updates) for more details on why the CCM doesn't reconcile these updates.
 
   #####  Effectuating EIP Updates using automatic method:
   If the admin wants to detect the change in `eipAllocations` and update the LB service automatically,
   rather than explicitly deleting the Service after changes, they can add the  `ingress.operator.openshift.io/auto-delete-load-balancer` annotation.
   While simpler in design, this approach could result in several minutes of unexpected disruption to ingress traffic. A cluster admin might not anticipate that updating
   the Subnets field could cause disruption, leading to an unwelcome surprise.
   
### 3. Any change to the service annotation does not trigger the operator to manage the load balancer service.
  The `status.endpointPublishingStrategy.loadBalancer.providerParameters.aws.networkLoadBalancer.eipAllocations`
  will eventually reflect the configured `eipAllocation` value by mirroring the value of
  `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` on the service. There are
  two scenarios in which the `EIPAllocations` status won't be equal to the `EIPAllocations` spec:

  1. The cluster admin manually configured the `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
   annotation on the service, which is not supported and likely to cause issues.
  2. The cluster admin configured the IngressController's `EIPAllocations` spec, but hasn't
   effectuated the change by deleting the service (see [section](#cluster-administrator-updates-eipallocations-field-of-the-existing-ingress-controller)).

  This proposal's use of `status.endpointPublishingStrategy` is consistent with the approach in
  [LoadBalancer Allowed Source Ranges](/enhancements/ingress/lb-allowed-source-ranges.md),
  but diverges with the approach in [Ingress Mutable Publishing Scope](/enhancements/ingress/mutable-publishing-scope.md).
  The Ingress Mutable Publishing Scope design sets `status.endpointPublishingStrategy.loadBalancer.scope`
  equal to `spec.endpointPublishingStrategy.loadBalancer.scope`, which may not always reflect the actual scope of the
  load balancer. This proposal for `EIPs` ensures `status.endpointPublishingStrategy` reflects the _actual_ value
  (in our case, the value of `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` on the Service).

  In short, the operator won't manage an annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` [here](https://github.com/openshift/cluster-ingress-operator/blob/ddd1ee6dfb7e7c37d9525f48242baab55c7527fc/pkg/operator/controller/ingress/load_balancer_service.go#L214-#L225).
  Any change to the service annotation won't trigger the operator to check it's spec and then update the service annotation. So, the 
  status will always reflect the value present in the service annotation and not the values present in the IngressController Spec.


### Risks and Mitigations

#### Invalid EIPAllocation Values:
- Providing a right number of EIPs same as the number of public subnets as per AWS guidelines is an admin
responsibility. Details of the error message can be found [here](#error-messages-when-the-number-of-eip-provided-and-the-number-of-subnets-of-vpc-id-dont-match-are-mentioned-as-follows).
- Also providing existing available EIPs which are not assigned to any instance is important. Details of the error message can be found [here](#error-messages-for-invalid-eips-or-eips-not-present-in-the-subnet-of-the-vpc-are-mentioned-as-follows).
- If the above two points are not followed, the Kubernetes LoadBalancer service for the IngressController
  won't get assigned EIPs,  which will cause the LoadBalancer service to be in persistently pending state by the
  Cloud Controller Manager (CCM). The reason for the persistently pending state is posted to the status of the IngressController.
  As of now we are not able to identify any more risks which can affect other components of the EP.

#### CCM Doesn't Reconcile NLB eipAllocation Updates
Once a NLB is created by the CCM, any updates to the `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` annotation will be validated,
but will not be propagated to the NLB.
Requiring services to be recreated upon updating the `...aws.networkLoadBalancer.eipAllocations` field as outlined in [Effectuating EIPAllocation Updates](#effectuating-eip-updates-using-manual-method) will mitigate cluster admins from experiencing this issue.

###  Effectuating EIP Updates using automatic method
Please refer [subsection](#effectuating-eip-updates-using-automatic-method) for more details.

### Drawbacks

Requiring the cluster admin to delete the LoadBalancer-type
service leads to several minutes of ingress traffic disruption.
This enhancement brings additional engineering complexity for upgrade
scenarios because cluster admins have previously been allowed to directly
add this annotation on a service.
Debugging invalid EIP Allocations will be confusing for cluster admins and may
lead to extra support cases or bugs.
This design requires a cluster admin to check the IngressController's status after
updating `eipAllocations` for instructions on how to proceed.

## Open Questions

1. Shall we support updating EIP in the default Classic `IngressController` after installation ?
   Please refer to [section](#updating-eip-in-the-default-classic-ingresscontroller-after-installation).

2. What happens when a cluster-admin adds invalid EIPs ?

   The load balancer creation would not proceed and fail as per details mentioned in [subsection of Support Procedure](#error-messages-for-invalid-eips-or-eips-not-present-in-the-subnet-of-the-vpc-are-mentioned-as-follows)

3. What happens when a cluster-admin adds number of EIPs which are not equal to the number of public subnets for the VPC id tagged with cluster name.

   The load balancer creation would not proceed and fail as per details mentioned in [subsection of Support Procedure](#error-messages-when-the-number-of-eip-provided-and-the-number-of-subnets-of-vpc-id-dont-match-are-mentioned-as-follows)

4. What happens when a cluster-admin adds nothing under eipAllocations ?
  
   The load balancer would be created with no eipAllocations and the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` will be removed.
   For more check [this](#cluster-admin-wants-operator-to-automatically-effectuate-the-load-balancer-service-for-updated-ingress-controller-eipallocations-)
   AWS will randomly assign the NLB's IP addresses from the IP address pools available.

5. Can you assign `eipAllocations` to an `IngressController` with scope `Internal` ?

    No, eipAllocations can be provided only for an `IngressController` with scope `External`.
    The default IC is set to [External](https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/operator/operator.go#L476)
    However, we will validate the `IngressController` CR at API using CEL to check if `eipAllocations` are provided only when `scope` is 
    set to `External`.

## Test Plan

### This EP will be covered by unit tests in the following:
- cluster-ingress-operator

Unit tests as well as E2E tests will be added to the Ingress Operator
repository.

E2E tests will cover the following scenarios:

- Creating an IngressController with `eipAllocations` and observing
  `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` on the LoadBalancer-type Service.
- Updating an IngressController with new `eipAllocations`, deleting the
  LoadBalancer-type service, and observing `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
  (as described in [Updating an existing IngressController with new eipAllocations Workflow](#effectuating-eip-updates-using-manual-method)).
- Updating an IngressController with new `eipAllocations`, using `auto-delete-loadbalancer` annotation for the operator to automatically delete
  LoadBalancer-type service, and observing `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
  (as described in [Updating an existing IngressController with new eipAllocations Workflow](#effectuating-eip-updates-using-automatic-method)).
- Directly configuring `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` on the
  LoadBalancer-type service and setting `eipAllocations` on the IngressController while observing
  `LoadBalancerProgressing` transitioning back to `Status: False` (as described in
  [Unmanaged EIPAllocation Annotation Migration Workflow](#unmanaged-eipallocation-annotation-migration-workflow)).
- Creating a IngressController with the `ingress.operator.openshift.io/auto-delete-load-balancer`
  annotation, updating an IngressController with new `eipAllocations`, and observing the service
  get automatically deleted and recreated with `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
  configured to the new `eipAllocations`.

  
## Graduation Criteria

### Dev Preview -> Tech Preview

- N/A. This feature will be introduced as Tech Preview.

### Tech Preview -> GA

- The E2E tests should be consistently passing and a PR will be created
  to enable the feature gate by default.

### Removing a deprecated feature

- N/A. We do not plan to deprecate this feature.

## Upgrade / Downgrade Strategy

During upgrade, if there are any `IngressController` load balancer services which have `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
then the `IngressController` would make the `LoadBalancerProgressing` Status to `True` and update the message to either manually delete
and create the service or set the values of the `eipAllocations` in the `IngressController` to values in the service annotation.

Downgrading will not change the value of the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

- If `IngressController` CR is provided with incorrect `eipAllocations` or if the number of `eipAllocations` does
  not match the number of public subnets for VPC ID tagged with cluster name then the Kubernetes LoadBalancer service
  will be in "Pending" as an LB won't be created in AWS. The reasons for the failure can be found in the status of the 
  `IngressController`.

- In case of event of an issue because of this API the network edge team needs to be involved
  in.

## Support Procedures

CCM - Cloud  Controller Manager

- If the service is not getting updated with the EIP annotation and the kubernetes load balancer 
   service is in `Pending` state then check the `cloud-controller-manager` logs and `IngressController` status.

In the CCM logs search for the `IngressController` name and check if you find any error related EIP allocation failure.
```sh
  oc logs <ccm pod name> -n openshift-cloud-controller-manager
```

In the `IngressController` status, check the status for the following:

### Error messages for invalid EIPs or EIPs not present in the subnet of the VPC are mentioned as follows:
  We will check the status of the IngressController which will have the message `The service-controller component is reporting SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure load balancer: error creating load balancer: "AllocationIdNotFound:` for status type `LoadBalancerReady` and `Available` in the `IngressController` object as follows:
```yaml
  
  - lastTransitionTime: "2024-04-20T06:37:13Z"
  message: |-
   The service-controller component is reporting SyncLoadBalancerFailed events like:
    Error syncing load balancer: failed to ensure load balancer: error creating load balancer:
    "AllocationIdNotFound: The allocation IDs 'eipalloc-0dba74f5f9888a37a', 
    'eipalloc-0436f6a636851e1ce', 'eipalloc-034f3c45b2b341752', 'eipalloc-094ada3572e093006',
    'eipalloc-0eeb9f39681b1a015' do not exist (Service: AmazonEC2; Status Code: 400; Error Code:
    InvalidAllocationID.NotFound; Request ID: 50026879-8b24-4a0f-9606-87ac22f47692; 
    Proxy: null)\n\tstatus code: 400, request id: 740d20ff-ae98-44d6-b86e-0762a1cf9c20"
   The kube-controller-manager logs may contain more details.
  reason: SyncLoadBalancerFailed
  status: "False"
  type: LoadBalancerReady
- lastTransitionTime: "2024-04-20T06:36:43Z"
  message: LoadBalancer is not progressing
  reason: LoadBalancerNotProgressing
  status: "False"
  type: LoadBalancerProgressing
- lastTransitionTime: "2024-04-20T06:36:43Z"
  message: The DNS management policy is set to Unmanaged.
  reason: UnmanagedLoadBalancerDNS
  status: "False"
  type: DNSManaged
- lastTransitionTime: "2024-04-20T06:36:43Z"
  message: The wildcard record resource was not found.
  reason: RecordNotFound
  status: "False"
  type: DNSReady
- lastTransitionTime: "2024-04-20T06:37:16Z"
  message: |-
  One or more status conditions indicate unavailable: LoadBalancerReady=False 
  (SyncLoadBalancerFailed: The service-controller component is reporting SyncLoadBalancerFailed 
  events like: Error syncing load balancer: failed to ensure load balancer: error creating load 
  balancer: "AllocationIdNotFound: The allocation IDs 'eipalloc-0dba74f5f9888a37a', 
  'eipalloc-0436f6a636851e1ce', 'eipalloc-034f3c45b2b341752', 'eipalloc-094ada3572e093006',
   'eipalloc-0eeb9f39681b1a015' do not exist (Service: AmazonEC2; Status Code: 400; Error Code:
    InvalidAllocationID.NotFound; Request ID: 50026879-8b24-4a0f-9606-87ac22f47692;
     Proxy: null)\n\tstatus code: 400, request id: 740d20ff-ae98-44d6-b86e-0762a1cf9c20"
  The kube-controller-manager logs may contain more details.)
  reason: IngressControllerUnavailable
  status: "False"
  type: Available
- lastTransitionTime: "2024-04-20T06:37:16Z"

```

### Error messages when the number of EIP provided and the number of subnets of VPC ID don't match are mentioned as follows:
  We will check the status of the IngressController which will have the message `The service-controller component is reporting SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure load balancer: error creating load balancer: Must have same number of EIP AllocationIDs (4) and SubnetIDs (5)`
  for status type `LoadBalancerReady` and `Available` in the `IngressController` object as follows:
```yaml
- lastTransitionTime: "2024-04-20T06:40:43Z"
  message: |-
   The service-controller component is reporting SyncLoadBalancerFailed events like: 
    Error syncing load balancer: failed to ensure load balancer: 
    error creating load balancer: Must have same number of EIP AllocationIDs (4) and SubnetIDs (5)
   The kube-controller-manager logs may contain more details.
  reason: SyncLoadBalancerFailed
  status: "False"
  type: LoadBalancerReady
 - lastTransitionTime: "2024-04-20T06:40:43Z"
  message: LoadBalancer is not progressing
  reason: LoadBalancerNotProgressing
  status: "False"
  type: LoadBalancerProgressing
 - lastTransitionTime: "2024-04-20T06:40:43Z"
  message: The DNS management policy is set to Unmanaged.
  reason: UnmanagedLoadBalancerDNS
  status: "False"
  type: DNSManaged
 - lastTransitionTime: "2024-04-20T06:40:43Z"
  message: The wildcard record resource was not found.
  reason: RecordNotFound
  status: "False"
  type: DNSReady
 - lastTransitionTime: "2024-04-20T06:40:43Z"
  message: |-
   One or more status conditions indicate unavailable: DeploymentAvailable=False
    (DeploymentUnavailable: The deployment has Available status condition set to 
    False (reason: MinimumReplicasUnavailable) with message: Deployment does not 
    have minimum availability.), LoadBalancerReady=False (SyncLoadBalancerFailed:
    The service-controller component is reporting SyncLoadBalancerFailed events like:
    Error syncing load balancer: failed to ensure load balancer: error creating load 
    balancer: Must have same number of EIP AllocationIDs (4) and SubnetIDs (5)
   The kube-controller-manager logs may contain more details.)
  reason: IngressControllerUnavailable
  status: "False"
  type: Available
 - lastTransitionTime: "2024-04-20T06:40:43Z"
  message: |-
   One or more status conditions indicate progressing: DeploymentRollingOut=True
    (DeploymentRollingOut: Waiting for router deployment rollout to finish: 0 
    of 2 updated replica(s) are available...
   )
  reason: IngressControllerProgressing
  status: "True"
  type: Progressing
 - lastTransitionTime: "2024-04-20T06:40:43Z"
```

- Check the status of `IngressController` controller for LoadBalancer which is LoadBalancerReady, LoadBalancerProgressing, Available.

```sh
  oc get ingresscontroller <ingresscontroller name> -o yaml
 ```


## Alternatives

### Immutability

Another alternative is to make the `eipAllocations` field immutable. This would require a
cluster admin to delete and recreate the IngressController if they wanted to update
the `eipAllocations`.

The advantage of this approach is that the API Server would prevent any updates
to the `eipAllocations` fields, clearly indicating to the cluster admin that they would
need to recreate the IngressController. This is different from our current design,
as described in [Effectuating EIPAllocation Updates](#effectuating-eip-updates-using-manual-method),
which is more subtle. With the current design, a cluster admin would need to check the
IngressController's status after updating `eipAllocations` for instructions on how to proceed.

However, we didn't use the immutability design because of the following drawbacks:

1. **Not Just a Load Balancer**: The IngressController, which encompasses DNS, HaProxy routers, and more, extends
   beyond just a load balancer. Updating the eipAllocations on the Ingress Controller only needs to propagate to the
   LB-type service, not the other components. Deleting the entire IngressController just to change `eipAllocations` is excessive,
   considering that only the service itself requires deletion.
2. **Prior Art**: We have an established a precedent in the [Ingress Mutable Publishing Scope](/enhancements/ingress/mutable-publishing-scope.md)
   design, where cluster admins are allowed to mutate an IngressController API field, but must delete the Service to
   effectuate the changes.
3. **Future Possibilities**: Should the AWS CCM eventually supports reliably modifying subnets without deleting the
   LB-type service, we can then easily adapt the Ingress Operator to immediately effectuate the `eipAllocations` and eliminate
   the need for a cluster admin to delete the LB-type service.

## Infrastructure Needed 

This EP works in AWS environment as AWS EIPs work on AWS environment only.
