---
title: aws-load-balancer-type
authors:
  - "@miheer"
reviewers:
  - "@wking"  
  - "@Miciah"
  - "@frobware"
  - "@sdodson"
  - "@candace"
approvers:
  - "@wking"  
  - "@sdodson"
  - "@frobware"
  - "@Miciah"
  - "@candace"
api-approvers:
  - "@deads"
  - "@Miciah"
aws-approvers:
  - "@jstuever"
  - "@patrickdillon"
aws-reviewers:
  - "@jstuever"
  - "@patrickdillon"
installer-approvers:
  - "@jhixson74"
  - "@jstuever"
  - "@patrickdillon"
  - "@staebler"
  - "@sdodson"
  - "@smarterclayton"
installer-reviewers:
  - "@AnnaZivkovic"
  - "@barbacbd"
  - "@jhixson74"
  - "@jstuever"
  - "@kirankt"
  - "@patrickdillon"
  - "@r4f4"
  - "@rna-afk"
  - "@sadasu"
  - "@staebler"
tracking-link:
  - "https://issues.redhat.com/browse/NE-942"
creation-date: 2022-06-09
last-updated: 2019-06-09   
status: implementable
superseded-by:
  - ""
---

# Allow Users to specify Load Balancer Type during installation for AWS

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

With this enhancement, the user can specify the AWS ELB type to be either Classic or NLB in the install-config, and this preference is stored in the cluster ingress config object.
This preference is used to deploy the default ingress controller with the user-specified load-balancer type for OpenShift clusters
running on AWS.
The install-time preference is also used for user-created ingress controllers on which no preference for load-balancer type is specified.
This enhancement does not change the default load-balancer type, which is Classic ELB.

## Motivation

The default load-balancer type is Classic ELB. One can set the load-balancer type for the default ingress controller during installation [using custom installer manifests](https://docs.openshift.com/container-platform/4.10/networking/configuring_ingress_cluster_traffic/configuring-ingress-cluster-traffic-aws-network-load-balancer.html#nw-aws-nlb-new-cluster_configuring-ingress-cluster-traffic-aws-network-load-balancer).
However, without this enhancement, when the default IngressController CR is deleted, the ingress operator recreates it and uses a Classic ELB irrespective of the install-time setting.
This  behavior is expected without this enhancement as the ingress operator does not specify a load-balancer type, and Classic ELB is the default type for LoadBalancer-type services.
Once the CR is deleted, the operator cannot check what load-balancer type it specified before its deletion.
Thus in order for the operator to determine the user's preference, we need to store this information in the cluster ingress config object.

### Goals

- The cluster administrator can specify the load-balancer type for ingress on AWS to be either Classic or NLB during installation.
- When the default IngressController CR is deleted, it is recreated with the load-balancer type specified during installation.
- The cluster administrator's preference of load-balancer type for ingress controllers is persisted.

### Non-Goals

- This enhancement does not modify the behavior for the API LB.
- This enhancement does not change the default load-balancer type for ingress; the default remains "Classic".
- This enhancement does not add any configuration for any platform other than AWS.
- This enhancement does not affect user-created LoadBalancer-type services; it only affects the services that the ingress operator creates for ingress controllers.

## Proposal

We will be adding a field `lbType` in the install-config for the user to specify a load-balancer type, either NLB or Classic ELB.
If no value is set then Classic will be set in the ingress cluster config object.

### Workflow Description

The installer accepts an LB type from the user, and the installer stores this type in the following object:
`oc get ingress.config.openshift.io/cluster -o yaml`

The ingress operator fetches this object and uses the info in it in its reconciliation logic.  In particular,
the operator uses the LB type in this object if it needs to recreate the default ingress controller.

### User Stories

#### Story 1

As an admin I want to install a specific load balancer rather than the default Classic LB.

#### Story 2

As a cluster admin, I want the install-time load-balancer type to persist even if the default ingress controller object is deleted.

### API Extensions

### Implementation Details/Notes/Constraints

For the install config object, we will be adding a field called `lbType` to the [`aws.Platform` type definition](https://github.com/openshift/installer/blob/1fb1397635c89ff8b3645fed4c4c264e4119fa84/pkg/types/aws/platform.go#L7) that is used in the install config type definition.  
It will look as follows -
```go
// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {

  // LBType allows user to set a load balancer type.
  // When this field is set the default ingresscontroller will get created using the specified LBType.
  // If this field is not set then the default ingress controller of LBType Classic will be created.
  // Valid values are:
  //
  // * "Classic": A Classic Load Balancer that makes routing decisions at either
  //   the transport layer (TCP/SSL) or the application layer (HTTP/HTTPS). See
  //   the following for additional details:
  //
  //     https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html#clb
  //
  // * "NLB": A Network Load Balancer that makes routing decisions at the
  //   transport layer (TCP/SSL). See the following for additional details:
  //
  //     https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html#nlb
  // +optional
  LBType configv1.AWSLBType `json:"lbType,omitempty"`
```

In the cluster ingress config object, we will need to add an API field corresponding to the install config's `LBType` field to store and persist the information
of the desired LB type specified in the installer config API so that the ingress operator can refer to the LB type in this object whenever someone deletes the default ingress controller.
Currently, the default LB type, i.e. Classic, is used if someone deletes the default ingress controller object even if the desired LB type for the default ingress controller
was set to NLB at install-time as the ingress operator does not have a way to check the LB type as we don't store it.

The API will look as follows -
this is an excerpt from [the ingress config CRD](https://github.com/openshift/api/blob/49bdd1286f04e68eab3c92c8957695610bdc613d/config/v1/types_ingress.go#L29):
```go

type IngressSpec struct {
  // loadBalancer contains the load balancer details in general which are not only specific to the underlying infrastructure
  // provider of the current cluster and are required for Ingress Controller to work on OpenShift.
  // +optional
  LoadBalancer LoadBalancer `json:"loadBalancer,omitempty"`
}

type LoadBalancer struct {
  // platform holds configuration specific to the underlying
  // infrastructure provider for the ingress load balancers.
  // When omitted, this means the user has no opinion and the platform is left
  // to choose reasonable defaults. These defaults are subject to change over time.
  // +optional
  Platform IngressPlatformSpec `json:"platform,omitempty"`
}

// IngressPlatformSpec holds the desired state of Ingress specific to the underlying infrastructure provider
// of the current cluster. Since these are used at spec-level for the underlying cluster, it
// is supposed that only one of the spec structs is set.
// +union
type IngressPlatformSpec struct {
  // type is the underlying infrastructure provider for the cluster.
  // Allowed values are "AWS", "Azure", "BareMetal", "GCP", "Libvirt",
  // "OpenStack", "VSphere", "oVirt", "KubeVirt", "EquinixMetal", "PowerVS",
  // "AlibabaCloud", "Nutanix" and "None". Individual components may not support all platforms,
  // and must handle unrecognized platforms as None if they do not support that platform.
  //
  // +unionDiscriminator
  Type PlatformType `json:"type"`
  // aws contains settings specific to the Amazon Web Services infrastructure provider.
  // +optional
  AWS *AWSIngressSpec `json:"aws,omitempty"`
}

// AWSIngressSpec holds the desired state of the Ingress for Amazon Web Services infrastructure provider.
// This only includes fields that can be modified in the cluster.
// +union
type AWSIngressSpec struct {
  // type allows user to set a load balancer type.
  // When this field is set the default ingresscontroller will get created using the specified LBType.
  // If this field is not set then the default ingress controller of LBType Classic will be created.
  // Valid values are: 
  // * "Classic": A Classic Load Balancer that makes routing decisions at either
  //   the transport layer (TCP/SSL) or the application layer (HTTP/HTTPS). See
  //   the following for additional details:
  //   https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html#clb
  //
  // * "NLB": A Network Load Balancer that makes routing decisions at the
  //   transport layer (TCP/SSL). See the following for additional details:
  //   https://docs.aws.amazon.com/AmazonECS/latest/developerguide/load-balancer-types.html#nlb
  // +unionDiscriminator
  // +kubebuilder:validation:Enum:=NLB;Classic
  // +kubebuilder:validation:Required 
  Type AWSLBType `json:"type,omitempty"`
}

type AWSLBType string

const (
    // NLB is the Network Load Balancer Type of AWS. Using NLB one can set NLB load balancer type for the default ingress controller.
    NLB AWSLBType = "NLB"

    // Classic is the Classic Load Balancer Type of AWS. Using CLassic one can set Classic load balancer type for the default ingress controller.
    Classic AWSLBType = "Classic"
)
```

#### Resource provided to the installer

The users provide a load balancer type to the installer.

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  aws:
    region: us-west-2
    lbType: nlb
    subnets:
    - subnet-1
    - subnet-2
    - subnet-3
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```


#### Resources created by the installer

- The installer will create the cluster ingress config object with the load-balancer type defined in it.

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
       type: AWS
 status:
   componentRoutes:
```

- The cluster-ingress-operator will check the cluster ingress config object for the lbType and reconcile to create a default ingress controller.
- If the user-specified ingress controller CR does not specify LB type then the LB Type defined in the ingress config will be used for reconcilation
  by the ingress operator to create the LB.

#### Destroying cluster

- None


### Risks and Mitigations

- Users need to put either "NLB" or "Classic" as input or else the LB creation will fail.
- To mitigate this we need to validate the input.

### Drawbacks

None

## Design Details

### Test Plan

- Set the `lbType` field in the `install-config.yaml` either to "NLB" or "Classic" and see if the default ingress controller gets created with the desired LB type.
- Delete the default ingress controller CR and see if the default ingress controller will be created using the LB type mentioned during installation.
- Unit tests and E2E tests will be written.

### Graduation Criteria
 
- N/A

#### Dev Preview -> Tech Preview

- N/A

#### Tech Preview -> GA

- N/A

#### Removing a deprecated feature

None

### Upgrade / Downgrade Strategy

Upgrade won't have issues as far as the installer and OCP version(as ingress operator has the fix) have the feature.

Downgrade to a version of installer and OCP will create CLB/ELB by default.

### Version Skew Strategy

Not applicable.

### Operational Aspects of API Extensions

None

#### Failure Modes

Users need to put either "NLB" or "Classic" for lbType or omit the field or else the install-config will fail validation

#### Support Procedures

- To determine how the ingress operator is determining the default LB type check the cluster version or ingress operator version to make sure it is new enough to have the enhancement, using `oc version` or
  `oc get clusteroperators/ingress -o yaml`, and checking the LB type in the cluster ingress config, using `oc get ingress.config.openshift.io/cluster -o yaml`.
- To determine the LB type that the operator specified on the service, please check the annotations in `oc -n openshift-ingress get svc/router-default -o yaml`.
  - For LBType NLB please check if annotation `service.beta.kubernetes.io/aws-load-balancer-type: "nlb"` is set.
  - For LBType Classic annotation `service.beta.kubernetes.io/aws-load-balancer-type:` shall not be present as it is not required for Classic LB to have that annotation or have that annotation set in the service.
- If the LBType is not correctly set by installer in the cluster ingress config then please check the installer logs.
- If the LBType is not correctly set in the ingress controller CR then please check the ingress operator logs.
- If the LBType is not correctly set in the service object then please check the kube-controller-manager logs.
  
## Implementation History

None

## Alternatives

None