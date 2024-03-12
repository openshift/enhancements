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
- Install time
- Custom NLB IngressController creation 
- Reconfiguration of the custom and default router to NLB with EIP allocation.

## Motivation

### User Stories

- As a cluster administrator, I want to provision EIPs and use them with an NLB IngressController.
- As a cluster administrator, I want to ensure EIPs are used with NLB on default router at install time.
- As a cluster administrator, I want to reconfigure default router to use EIPs.
- As a cluster administrator of OpenShift on AWS (ROSA), I want to use static IPs (and therefore AWS Elastic IPs) so that 
  I can configure appropriate firewall rules.
  I want the default AWS Load Balancer that they use (NLB) for their router to use these EIPs.
- As a cluster administrator who has purchased a block of public IPv4 addresses, I want to use these IP addresses, so 
  that I can avoid Amazon's new charge for public IP addresses.

### Goals
- Users are able to use EIP for a NLB `IngressController`.
- Users are able to create a NLB default `IngressController` during install time with the EIPs specified during install.
- The load balancer of an existing `IngressController` can be reconfigured to use specific EIPs by updating the `IngressController` specification and recreating the load balancer as type NLB.

### Non-Goals

- Creation of EIPs in AWS.
- Monitoring or management of changes to EIPs in the AWS environment.
- Static IP usage with NLBs for OpenShift API server.
- Check the number of public subnets available and then compare number of subnets
  with the number of EIPs the user provides.

## Proposal

EIP = AWS Elastic IP, NLB = AWS Network LoadBalancer, AZ = Availability Zone 

This enhancement adds API fields in the installer and the IngressController specification
to set the AWS EIP for the NLB `IngressController`. The cluster ingress operator will then create 
the `IngressController` and get the EIP assigned to the service LoadBalancer with the annotation 
`service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
Note: `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` is a standard [Kubernetes service annotation](https://kubernetes.io/docs/reference/labels-annotations-taints/#service-beta-kubernetes-io-aws-load-balancer-eip-allocations)

### API Extensions
- The first API extension for setting `eipAllocations` is in the installer [Platform](https://github.com/openshift/installer/blob/master/pkg/types/aws/platform.go) type, where the new field `NetworkLoadBalancerParameters` is added as an optional field.

```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
...

    // networkLoadBalancerParameters holds configuration parameters for an AWS
    // network load balancer. Present only if type is NLB.
    //
    // +optional
    NetworkLoadBalancerParameters *configv1.AWSNetworkLoadBalancerParameters `json:"nlbParameters,omitempty"`
}

```

- The installer will propagate `eipAllocations` from `install-config.yaml` to the cluster ingress configuration, which the ingress operator uses to create the default IngressController.  
  The following  definitions will be added in the file [`config/v1/types_ingress.go`](https://github.com/openshift/api/blob/master/config/v1/types_ingress.go) of the [`openshift/api`](https://github.com/openshift/api) GitHub repository:

```go
// AWSIngressSpec holds the desired state of the Ingress for Amazon Web Services infrastructure provider.
// This only includes fields that can be modified in the cluster.
// +kubebuilder:validation:XValidation:rule="self.type != 'Classic' || !has(self.nlbParameters)",message="Network load balancer parameters are allowed only when load balancer type is NLB."
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
    NetworkLoadBalancerParameters *AWSNetworkLoadBalancerParameters `json:"nlbParameters,omitempty"`
}

// AWSNetworkLoadBalancerParameters holds configuration parameters for an
// AWS Network load balancer. For Example: Setting AWS EIPs https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html
type AWSNetworkLoadBalancerParameters struct {
    // You can assign Elastic IP addresses to the Network Load Balancer by adding the following annotation.
    // The number of Allocation IDs must match the number of subnets that are used for the load balancer.
    // service.beta.kubernetes.io/aws-load-balancer-eip-allocations: eipalloc-xxxxxxxxxxxxxxxxx,eipalloc-yyyyyyyyyyyyyyyyy
    // https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html
    // https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.7/guide/service/annotations/#eip-allocations
    // +openshift:enable:FeatureGate=SetEIPForNLBIngressController 
    // The EIPs in the cluster config are only used when the operator creates the default IngressController. 
    // It isn't used as a default value when eipAllocations is not specified on the IngressController.
    EIPAllocations []EIPAllocations `json:"eipAllocations"`
}

type EIPAllocations string

```

- The API field for `eipAllocations` field in the `IngressController` CR needs to be set as follows
  in the file [operator/v1/types_ingress.go](https://github.com/openshift/api/blob/master/operator/v1/types_ingress.go):
```go
// AWSLoadBalancerParameters provides configuration settings that are
// specific to AWS load balancers.
// +kubebuilder:validation:XValidation:rule="self.type != 'Classic' || !has(self.nlbParameters)",message="Network load balancer parameters are allowed only when load balancer type is NLB."
// +kubebuilder:validation:XValidation:rule="self.type != 'NLB' || !has(self.classicLoadBalancer)",message="Classic load balancer parameters are allowed only when load balancer type is Classic."
// +union
type AWSLoadBalancerParameters struct {
    // networkLoadBalancerParameters holds configuration parameters for an AWS
    // network load balancer. Present only if type is NLB.
    //
    // +optional
    NetworkLoadBalancerParameters *AWSNetworkLoadBalancerParameters `json:"nlbParameters,omitempty"`	
}


// AWSNetworkLoadBalancerParameters holds configuration parameters for an
// AWS Network load balancer. For Example: Setting AWS EIPs https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html
type AWSNetworkLoadBalancerParameters struct {
    // You can assign Elastic IP addresses to the Network Load Balancer by adding the following annotation. 
    // The number of Allocation IDs must match the number of subnets that are used for the load balancer.
    // service.beta.kubernetes.io/aws-load-balancer-eip-allocations: eipalloc-xxxxxxxxxxxxxxxxx,eipalloc-yyyyyyyyyyyyyyyyy
    // https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html
    // https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.7/guide/service/annotations/#eip-allocations
    // +openshift:enable:FeatureGate=SetEIPForNLBIngressController
    // The EIPs in the cluster config are only used when the operator creates the default IngressController.
    // It isn't used as a default value when eipAllocations is not specified on the IngressController.
    EIPAllocations []configv1.EIPAllocations `json:"eipAllocations"`
}

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
    networkLoadBalancer:
      eipAllocations:
      - eipalloc-1111
      - eipalloc-2222
      - eipalloc-3333
      - eipalloc-4444
      - eipalloc-5555 
...
```  
Here, the administrator needs to know how many public subnets the `AWS VPC ID` which is tagged by `cluster name` after installation.
To locate the public subnets:
1. Navigate to your AWS Web Console for the account that contains the cluster
2. Go to `Your VPCs` under the `Virtual Public Cloud` section
3. Search for the VPC ID tagged with `cluster name` using the command `oc get infrastructure cluster -o jsonpath='{.status.infrastructureName}'`
4. Click the VPC ID link for the VPC
5. Click the link under `Main route table` and then check the column `Explicit subnet associations` which
   will show you the number of public subnets available for that VPC ID.

One can try these scripts present [here](https://github.com/frobware/haproxy-hacks/tree/master/NE-1398) as well.

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
         networkLoadBalancer:
           eipAllocations:
           - eipalloc-1111
           - eipalloc-2222
           - eipalloc-3333
           - eipalloc-4444
           - eipalloc-5555
       type: AWS
```
3. Ingress Operator will create the default `IngressController` with the service type load balancer having the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` with values from the field `eipAllocations`
      from the cluster ingress config object.

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
to change. Therefore, after changing EIPs on the an existing `IngressController`, it changes its own status to `Progressing` to signal that the user must assist with the update by deleting the Kubernetes Service of type `LoadBalancer` object in order to propagate the changes.
Once the user performs this step, the operator recreates the load balancer with the new EIPs to complete the operation.  By adding this safeguard, the disruption can be scheduled for a maintenance window, if needed.

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


#### Unmanaged Subnet Annotation Migration Workflow

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

#### Installer created VPCs and subnets

When the cluster admin uses installer created VPCs, they are not aware of what and how many public subnets would
be created by the `openshift-installer`. However, while providing the eipAllocations in the `install-config.yaml`
they should know the exact number of eipAllocations they may provide. So, they need to follow the workflow as follows:

1. Cluster admin wants to use EIPs, but isn't bringing their own VPC, so they need to know exactly how many EIPs to create.
2. Cluster admin should review https://aws.amazon.com/about-aws/global-infrastructure/regions_az/ to find out how many AZs are in the region they are installing into.
3. Cluster admin should use the same number of EIPs as AZs in their region.

Note: The validation for `install-config.yaml` will fail if the number of eipAllocations don't match the number of AZs
when installer created subnets are going to be used i.e when BYO Subnets are not used.

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

#### Changing the EIPs for the default IngressController

The `eipAllocations` mentioned in the ingress `cluster` object is not the default one. 
Refer question number 9 under [section](#open-questions). 
So, when ingress operator perform a polling loop, it checks for the existence of default `IngressController`.
If it is not present it creates one with the details like `eipAllocations` present in the ingress cluster object.

So, if one wants to change the eips for the default `IngressController` they need to change the ingress cluster 
objects `eipAllocations` and then delete the default IngressController.
Then wait for the polling loop of ingress operator to execute creation of the default `IngressController`
with the updated `eipAllocations` mentioned in the ingress `cluster` object. 

Note: Deleting and quickly recreating the default IngressController with updated EIPs can cause a race situation
if the operator's polling loop executes before cluster admins manual update of eipAllocations.

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

#### 2. Create a new IngressController with EIPs:
   
   The admin sets the `eipAllocations` in the `IngressController` CR. Then same process
   as per point 1.iii.b and 1.iii.c is followed by the ingress operator. 

#### 3. Add, delete, or change the EIP for an existing IngressController:
   
   When the admin changes the eipAllocations field in the `IngressController` CR, the ingress operator sets the Load Balancer
   Progressing status as per [section](#cluster-administrator-updates-eipallocations-field-of-the-existing-ingress-controller).
   The cluster admin has to manually delete service in order to effectuate the change.
   Then step 1.iii.c is followed by the ingress operator.

   If the admin wants to detect the change in `eipAllocations` and update the LB service automatically,
   rather than explicitly deleting the Service after changes, they can add the  `ingress.operator.openshift.io/auto-delete-load-balancer` annotation.
   
### 4. Any change to the service annotation does not trigger the operator to manage the load balancer service.
  The `status.endpointPublishingStrategy.loadBalancer.providerParameters.aws.networkLoadBalancer.eipAllocations`
  will eventually reflect the configured subnet value by mirroring the value of
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
  Any change to the service annotation won't trigger the operator to check it's spec and then update the service annotation.

### Risks and Mitigations

- Providing a right number of EIPs same as the number of public subnets as per AWS guidelines is an admin
responsibility.
- Also providing existing available EIPs which are not assigned to any instance is important.
- If the above two points are not followed, the Kubernetes LoadBalancer service for the IngressController
  won't get assigned EIPs,  which will cause the LoadBalancer service to be in persistently pending state by the
  Cloud Controller Manager (CCM). The reason for the persistently pending state is posted to the status of the IngressController.
  As of now we are not able to identify any more risks which can affect other components of the EP.

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
   Please refer to [section](#updating-eip-in-the-default-classic-ingress-controller-after-installation).

2. What happens when someone updates the `eipAllocations` directly in the load balancer service of an `IngressController` ?
  It is not supported to directly edit the `eipAllocations` for the load balancer service of an `IngressController`, and this will put the IngressController into an error state with the following status:
```yaml
  - lastTransitionTime: "2024-04-17T03:09:32Z"
    message: |-
      The service-controller component is reporting SyncLoadBalancerFailed events like: 
      Error syncing load balancer: failed to ensure load balancer: error creating load balancer: 
      "ResourceInUse: The allocation IDs are not available for use\n\tstatus code: 400, 
      request id: 58136c42-e1bd-4c1a-ac38-7078da2b1d95"
      The kube-controller-manager logs may contain more details.
    reason: SyncLoadBalancerFailed
    status: "False"
    type: LoadBalancerReady
  - lastTransitionTime: "2024-04-17T02:54:25Z"
    message: LoadBalancer is not progressing
    reason: LoadBalancerNotProgressing
    status: "False"
    type: LoadBalancerProgressing
...
...
  - lastTransitionTime: "2024-04-17T03:09:32Z"
    message: |-
      One or more status conditions indicate unavailable: LoadBalancerReady=False 
      (SyncLoadBalancerFailed: The service-controller component is reporting 
      SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure
      load balancer: error creating load balancer: "ResourceInUse: The allocation IDs 
      are not available for use\n\tstatus code: 400, request id: 58136c42-e1bd-4c1a-ac38-7078da2b1d95"
      The kube-controller-manager logs may contain more details.)
    reason: IngressControllerUnavailable
    status: "False"
    type: Available

```

3. Are there any pre-requisites for this EP ?
  Yes, please refer section `Pre-requisites` under [Workflow Description](#workflow-description)

4. What happens when the cluster-admin specified invalid EIPs in install-config.yaml at installation time (day 0) ?

   We would try to get the installer feasibly validate the `install-config.yaml` for day 0.
   - For validation for BYOB subnets:
  We will count if the number of cluster subnets provided by the user in the `install-config.yaml` were equal to 
  the number of the eip allocations provided in the install-config.yaml. We will need to add validations 
  in [pkg/asset/installconfig/aws/validation.go](https://github.com/openshift/installer/blob/master/pkg/asset/installconfig/aws/validation.go#L186)

  - For validation for  Installer Created Subnets:
  The number of EIPs must match number of AZs in region.

  - Validation for Invalid EIP which is generally when the EIP does not exist in the AWS environment or is not available.
    For Example: is assigned to some instance or when count of EIPs does not match to the number of subnets.

5. What happens when a cluster-admin adds invalid EIPs ?

  The load balancer creation would not proceed and fail as per details mentioned in [subsection of Test Plan](#test-when-ingress-controller-cr-has-been-provided-with-invalid-eipallocations-)

6. What happens when a cluster-admin adds number of EIPs which are not equal to the number of public subnets for the VPC id tagged with cluster name.

  The load balancer creation would not proceed and fail as per details mentioned in [subsection of Test Plan](#test-when-ingress-controller-cr-has-been-provided-with-number-of-eips-not-equal-to-the-number-of-public-subnets-for-vpc-tagged-with-your-cluster-name)

7. What happens when a cluster-admin adds nothing under eipAllocations ?
  
  The load balancer would be created with no eipAllocations and the service annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations` will be removed.
  For more check [this](#cluster-admin-wants-operator-to-automatically-effectuate-the-load-balancer-service-for-updated-ingress-controller-eipallocations-)
  AWS will randomly assign the NLB's IP addresses from the IP address pools available.

8. Why should a custom IngressController avoid using the EIPs from the cluster config ?

  The default IngressController cannot use the EIPs from the cluster config if a custom
  IngressController is using them. The expectation is that when the operator creates (or recreates) 
  the default IngressController, it uses any EIPs that are specified in the cluster config.
  Therefore, a custom IngressController should avoid using the EIPs from the cluster config
  so as to avoid preventing the default IngressController from using them.

  If the cluster-admin creates 2 custom IngressControllers, then only one of them could use the EIPs
  from the cluster config, even if the default IngressController weren't using them.  Using the cluster
  config in this case would mean the outcome would depend on which custom IngressController were created
  first. Avoiding using the cluster config for custom IngressControllers makes the behavior more predictable.

9. Are the eips mentioned in the cluster config default for the default `IngressController` ?  
   What happens if the cluster-admin deletes the default IngressController and then recreates it before 
   the polling loop of the operator does, and the cluster-admin doesn't specify eipAllocations?

  No. The eips in the cluster config are only used when the operator creates the default IngressController.
  If the default `IngressController` is not found and if the polling loop kicks in and detects it's absence it will
  create a default IC with the eips mentioned in the cluster config object of ingress.
  However, it isn't used as a default value when eipAllocations is not specified on the IngressController because
  the cluster-admin may delete the default IngressController and then recreate it with no eips or no already assigned eips
  or before the polling loop does.
  Because the operator only uses the cluster config to specify eipAllocations when the operator itself creates
  the default IngressController, the cluster-admin can always delete and recreate the default IngressController
  with eipAllocations deliberately left unspecified, so that the IngressController doesn't use the EIPs from the
  cluster config. Or the cluster-admin  can always delete and recreate the default IngressController with eipAllocations
  which are not already specified in the config ingress cluster object.

10. Can you assign `eipAllocations` to an `IngressController` with scope `Internal` ?
    No, eipAllocations can be provided only for an `IngressController` with scope `External`.
    The default IC is set to [External](https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/operator/operator.go#L476)
    However, we will validate the `IngressController` CR at API using CEL to check if `eipAllocations` are provided only when `scope` is 
    set to `External`.

## Test Plan

### This EP will be covered by unit tests in the following:
- cluster-ingress-operator
- installer

### This  EP will also be covered by end to end tests:
##### Create an IngressController using IngressController CR having eipAllocations.
  The end to end test will cover the scenario where user will set an `eipAllocations` field
  in the `IngressController` CR and then test if the `IngressController`
  with service type load balancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
  has value of `eipAllocations` field from the IngressController CR was created.
  We will check the status of IngressController for LoadBalancer which is `LoadBalancerReady`, `LoadBalancerProgressing`, `Available`
  and any errors in the cloud controller manager logs.

##### Update an IngressController eipAllocations using IngressController CR which already has eipAllocations set.
  The end to end test will also the cover the scenario where user updates the `eipAllocations` field
  in the `IngressController` CR and then check the status of `IngressController` for LoadBalancer which 
  is `LoadBalancerReady`, `LoadBalancerProgressing`, `Available` and the cloud controller manager logs.
  The `LoadBalancerProgressing` should display `True` and message `"'One or more status conditions indicate progressing: LoadBalancerProgressing=True
  (OperandsProgressing: One or more managed resources are progressing: The IngressController
  scope was changed from "eipalloc-0956fea34de4cb7ab,eipalloc-0e9a3077a70de050a,eipalloc-0b69fc4691f54cdd0,eipalloc-01e6ba6cbba1a391b,eipalloc-0e242df173f906112"
  to "eipalloc-0387f99f5d4724c3e,eipalloc-0b09650c180c2abb6,eipalloc-0161deab2f05fe2fe,eipalloc-0ec5738e0e3808b8a,eipalloc-09d56b78479ac651d".  To
  effectuate this change, you must delete the service: 'oc -n openshift-ingress
  delete svc/router-test'; the service load-balancer will then be deprovisioned
  and a new one created.  This will most likely cause the new load-balancer to
  have a different host name and IP address from the old one''s.  Alternatively,
  you can revert the change to the IngressController:  "'oc -n openshift-ingress-operator
  patch ingresscontrollers/test --type=merge --patch=''{"spec":{"endpointPublishingStrategy":{"loadBalancer":{"providerParameters":{"type":"AWS","aws":{"type":"NLB","eipAllocations:[]"}}}}}}'''"'`

##### Update an IngressController eipAllocations by adding auto-delete-load-balancer annotation in the IngressController CR which already has eipAllocations set.
  Test update with `auto-delete-load-balancer` annotation, where users sets `auto-delete-load-balancer` annotation
  on the `IngressController`, then updates the `eipAllocations` field
  in the `IngressController` CR and then test if the `IngressController`
  with service type load balancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-eip-allocations`
  has value of `eipAllocations` field from the `IngressController` CR was created.
  We will check the status of IngressController controller for LoadBalancer which is `LoadBalancerReady`, `LoadBalancerProgressing`, `Available`
  and any errors in the cloud controller manager logs.

##### Test when IngressController CR has been provided with invalid EIPAllocations. 
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

##### Test when IngressController CR has been provided with number of EIPs not equal to the number of public subnets for VPC tagged with your cluster name.
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
- Error messages for invalid eips or eips not present in the subnet of the VPC are mentioned [here](#test-when-ingress-controller-cr-has-been-provided-with-invalid-eipallocations-)
- Error messages when the number of EIP provided and the number of subnets of VPC ID don't match are mentioned [here](#test-when-ingress-controller-cr-has-been-provided-with-number-of-eips-not-equal-to-the-number-of-public-subnets-for-vpc-tagged-with-your-cluster-name).
- Check the status of `IngressController` controller for LoadBalancer which is LoadBalancerReady, LoadBalancerProgressing, Available.

```sh
  oc get ingresscontroller <ingresscontroller name> -o yaml
 ```


## Alternatives

N/A

## Infrastructure Needed 

This EP works in AWS environment as AWS EIPs work on AWS environment only.
