---
title: aws-load-balancer-operator 
authors:
  - "@arjunrn"
reviewers:
  - "@Miciah"
  - "@alebedev87"
approvers:
  - "@Miciah"
api-approvers:
  - TBD
creation-date: 2022-01-27
last-updated: 2022-01-27
tracking-link:
  - https://issues.redhat.com/browse/CFEPLAN-39
see-also:
  - "/enhancements/ingress/transition-ingress-from-beta-to-stable.md"
replaces:
superseded-by:
---

# AWS Load Balancer Operator

## Summary

Users would like to use [Application Load
Balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/introduction.html) (ALBs)
for their Ingress resources so that they can use [Amazon Certificate
Manager](https://aws.amazon.com/certificate-manager/) (ACM) to manage
certificates for the Ingress domains and also attach [Web Application
Firewalls](https://aws.amazon.com/waf/) (WAFs) to the ALB for security and traffic
shaping. The OpenShift [router](https://github.com/openshift/router) uses a classic ELB or an NLB, which
passes through all traffic to the router. TLS termination is done on the router or backend pod.
A WAF cannot be attached to the ELB or NLB, and the router cannot be configured to
perform TLS termination at the ELB or NLB.

## Background

### AWS Load Balancing
AWS has three kinds of load balancers:
1. **Classic Load Balancer:** This is a Level 4 load balancer with some Level 7
    features. It does not have integration with
    AWS’s newer services.
2. **Network Load Balancer:** This is also a Level 4 load balancer but has been
    optimized for performance and cost compared to the Classic Load Balancer.
3. **Application Load Balancer:** This is a Level 7 load balancer which can
    route requests based on the HTTP path and host. It also integrates with other
    AWS services like DDoS protection and WAF.

### OpenShift Router
In OCP/OKD, Ingress resources are serviced by the OpenShift
[router](https://github.com/openshift/router). Routers are created by the
[cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator/) (CIO)
for every
_[IngressController](https://docs.openshift.com/container-platform/4.9/networking/configuring_ingress_cluster_traffic/configuring-ingress-cluster-traffic-ingress-controller.html)_
resource. By default on AWS, the CIO creates a Service of type
[LoadBalancer](https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer).
The in-tree Service controller in turn provisions a classic Elastic Load
Balancer with a TCP listener. The CIO then creates a DNS entry for the subdomain
specified in the IngressController resource with the load balancer created in
the previous step as the target. 

### aws-load-balancer-controller
AWS maintains an out-of-tree controller with support for ALBs for Ingress and
NLBs for Service type resources. The in-tree Service controller only provisions [classic
load
balancers](https://docs.aws.amazon.com/elasticloadbalancing/latest/classic/introduction.html)
and
[Network](https://docs.aws.amazon.com/elasticloadbalancing/latest/network/introduction.html)
Load Balancers. The out-of-tree controller supports integration of Kubernetes
resources with more of the AWS services like WAFs and the DDoS prevent service
[Shield](https://docs.aws.amazon.com/waf/latest/developerguide/ddos-overview.html).
It also enables traffic routing from the load balancer directly to the pods of
an Ingress or Service if the cluster uses the [AWS VPC Container Networking
Interface](https://github.com/aws/amazon-vpc-cni-k8s). In addition it also
introduces 2 Custom Resources namely
[`IngressClassParams`](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.3/guide/ingress/ingress_class/#ingressclassparams)
and
[`TargetGroupBindings`](https://kubernetes-sigs.github.io/aws-load-balancer-controller/v2.3/guide/targetgroupbinding/targetgroupbinding/).
_IngressClassParams_ is used for specifying custom parameters for Ingresses of a
certain class.  The controller uses _TargetGroupBinding_ internally and it can
also be used by users to manage the load balancer externally. The
aws-load-balancer will henceforth be referred to as **“lb-controller”** for the
sake of brevity.

## Motivation

Implement an operator, tentatively called `aws-load-balancer-operator`, which deploys and manages an instance of
lb-controller. The operator will be distributed through Operator Hub. The initial version of the operator will only
support OCP/OKD. But in the future the operator may be extended to work on any Kubernetes compliant cluster.

### Goals
* Implement operator which can be installed and used in OCP/OKD.

### Non-Goals
* Implementing support for ALB for OpenShift Routes
* Support for multiple installations of the controller i.e. multiple custom
  resources.
* Support for `ip` mode for Ingress resources
* Support for NLB backed Services through the lb-controller.
* Support for the AWS VPC CNI plugin.

## Proposal

### User Stories

#### TLS Termination and DDoS protection

The user wants to route traffic for the domain example.com to pods of Service
example-service and have TLS termination and DDoS protection on the load
balancer. The user deploys the operator and creates an instance of the
resource `AWSLoadBalancerController` as follows:

```yaml
kind: AWSLoadBalancerController
group: networking.openshift.io/v1beta1
metadata:
  name: cluster
spec:
  subnetTagging: auto
  ingressClass: tls-termination
```

Then they create an `IngressClass` resource as follows:

```yaml
apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata:
  name: tls-termination
spec:
  controller: ingress.k8s.aws/alb
```

Then they create an ingress resource with the following schema:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: example
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/certificate-arn: arn:aws:acm:us-west-2:xxxxx
    alb.ingress.kubernetes.io/shield-advanced-protection: true
spec:
  ingressClassName: tls-termination
  rules:
  - host: example.com
    http:
        paths:
          - path: /
            pathType: Exact
            backend:
              service:
                name: example-service
                port:
                  number: 80
```

The controller then creates an application load balancer with an HTTPS listener.
The ALB only has an HTTPS listener because a certificate has been specified
explicitly through the annotations. If the certificate does not exist, the load
balancer is not created. It also enables AWS Shield on the Elastic IP associated
with the ALB.

#### Multiple Ingress through single ALB

The user wants to route traffic to services example-1, example-2 and example-3
as parts of the domain example.com and wants all the endpoints served through a
single ALB. The user create an `AWSLoadBalancerController` like in the previous example and
then creates 3 Ingress resources as follows:

```yaml
kind: Ingress
metadata:
  name: example-1
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/group.name: example
    alb.ingress.kubernetes.io/group.order: "1"
spec:
  ingressClass: alb
  rules:
  - host: example.com
    http:
        paths:
        - path: /blog
          backend:
            service:
              name: example-1
              port:
                number: 80
—--
kind: Ingress
metadata:
  name: example-2
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/group.name: example
    alb.ingress.kubernetes.io/group.order: "2"
spec:
  ingressClass: alb
  rules:
  - host: example.com
    http:
        paths:
        - path: /store
          backend:
            service:
              name: example-2
              port:
                number: 80
—--
kind: Ingress
metadata:
  name: example-3
  annotations:
    alb.ingress.kubernetes.io/scheme: internet-facing
    alb.ingress.kubernetes.io/group.name: example
    alb.ingress.kubernetes.io/group.order: "3"
spec:
  ingressClass: alb
  rules:
  - host: example.com
    http:
        paths:
        - path: /
          backend:
            service:
              name: example-3
              port:
                number: 80
```

In this example the annotations `alb.ingress.kubernetes.io/group.name` specifies
which Ingress Group the Ingresses belong to and the annotation
`alb.ingress.kubernetes.io/group.order` specifies the order in the group which
will be used for path matching. All Ingresses which belong to the same group are
attached to the same ALB.

### API Extensions

This operator uses a cluster-scoped Custom Resource to configure the
lb-controller instance. The resource has the following _Spec_:

```golang
type AWSLoadBalancerControllerSpec struct {
  SubnetTagging SubnetTaggingPolicy
  // +optional
  AdditionalResourceTags map[string]string // Default AWS Tags that will be applied to all AWS resources managed by this controller (default [])
  // +optional
  IngressClass string   // indicates the class of ingress for which the controller should provision ALBs
  // +optional
  Config *DeploymentConfig
  // +optional
  EnabledAddons []AWSAddon // indicates which AWS addons should be disabled.
}

type AWSAddon string

const (
  AWSAddonShield AWSAddon = "AWSShield"
  AWSAddonWAFv1 AWSAddon = "AWSWAFv1"
  AWSAddonWAFv2 AWSAddon = "AWSWAFv2"
)

type DeploymentConfig struct {
  // +optional
  Replicas int
}

type SubnetTaggingPolicy string
const (
    AutoSubnetTaggingPolicy SubnetTaggingPolicy = "Auto"
    ManualSubnetTaggingPolicy SubnetTaggingPolicy = "Manual"
)
```


### Implementation Details/Notes/Constraints

The operator only reconciles a single instance of the custom resource because
only [one
instance](https://github.com/kubernetes-sigs/aws-load-balancer-controller/issues/2185)
of the lb-controller can be run in a cluster. There can be multiple replicas
with the same configuration with leader-election enabled. But multiple
controllers cannot be started with different configurations.

The lb-controller also requires that the subnets where the load balancers are
provisioned have certain resource tags present on them. The operator can detect
the subnets and tag them or the user can do this. This can be enabled by setting
the `SubnetTagging` to `AutoSubnetTaggingPolicy`.

The user can also specify the Ingress class which the lb-controller will
reconcile. This could default to **_“alb”_**. This value is required because if
an ingress class value is not passed to the controller it attempts to reconcile
all Ingress resources which don’t have an ingress class annotation or where the
ingress class field is empty. Passing the value of “alb” does not restrict the
number of ingress classes which the controller can reconcile. This is described
further in the [Parallel operation of OpenShift
Router](#parallel-operation-of-the-openshift-router-and-lb-controller) and
lb-controller section.

The _DeploymentConfig_ field contains any configuration required for the
deployment. The number of replicas is the only field for now. But in future
versions other fields from the
[PodSpec](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#podspec-v1-core)
could also be exposed here.

Finally, WAFs and DDoS prevention configurations are attached to the load
balancer through annotations specified on the Ingress resource. But users might
want to limit which features are available and the `EnabledAddons` array can be
used to specify which features/addons have to be enabled. By default, all the
addons will be enabled. They are disabled by setting the addon CLI flag to
false. When an addon which was previously enabled is disabled the controller
does not remove the existing addon attachment from the provisioned load
balancers.

The operator will also create a
[CredentialRequest](https://docs.openshift.com/container-platform/4.9/rest_api/security_apis/credentialsrequest-cloudcredential-openshift-io-v1.html#credentialsrequest-cloudcredential-openshift-io-v1)
on behalf of the lb-controller and mount the minted credentials in the
_Deployment_ of the controller. This means that the operator will only work on
OCP/OKD but this is sufficient for the initial release.

The lb-controller requires a validating and mutating webhook for correct operation. The operator will have to create the
webhooks along with the controller deployment. The webhook can be registered with a CA bundle which is used to verify
the identity of webhook by the API server.
The [service-ca controller](https://docs.openshift.com/container-platform/4.9/security/certificate_types_descriptions/service-ca-certificates.html)
can be used to generate certificates and have them injected into the webhook configurations. The operator will also
watch the secret with the certificates so that when the ceritificate is re-newed the pods of the deployment will be also
updated so that they start using the new certificates.

### Risks and Mitigations

#### Restricting features in the lb-controller

Some changes will have to be made in the lb-controller to ensure that the users
only use features which are supported on OCP and only those features which
target Ingress resources. These changes may also be contributed back upstream
since it may benefit other users.

1. The annotation `alb.ingress.kubernetes.io/target-type` can be set to the
value **_ip_** or **_instance_**. When the value is set to _instance_ the
created load balancer will be configured to route traffic through a NodePort
service. The _ip_ value only works when the cluster uses the Amazon VPC CNI and
hence will not work on OCP/OKD clusters. The controller has to be modified so
that this annotation is ignored and the controller always uses routing through
_NodePort_ services.
2. The _lb-controller_ has support for _Service_ type resources as
well. When the annotation `service.beta.kubernetes.io/aws-load-balancer-type:
"external"` is specified on a _Service_ resource of type _LoadBalancer_ the
in-tree controller ignores this resource and the lb-controller instead
provisions a Network Load Balancer with the correct configuration. Since we want
to restrict the features supported by the controller to only Ingresses the
lb-controller will have to be modified so that it does not reconcile any Service
resources.

#### Parallel operation of the OpenShift router and lb-controller

The OpenShift router manages ingresses that don’t have any ingress class value.
This is the expected behavior currently and the lb-controller should not attempt
to reconcile Ingresses which don’t have an ingress class set. If the user wants
to switch over to using the lb-controller as default they can create an
_IngressClass_ resource with the annotation
`ingressclass.kubernetes.io/is-default-class` set to _true_ and the
`spec.controller` value should be _ingress.k8s.aws/alb._ The user can also
create multiple `IngressClass` resources with different configurations and have
the `spec.controller` set to _ingress.k8s.aws/alb._ All Ingresses which belong
to these classes will be reconciled by the lb-controller.

## Design Details

### Open Questions [optional]

### Test Plan

Other than standard unit testing the operator will have end-to-end tests for some common usage scenarios:

1. Test which ensures that sufficient replicas are running and up-to-date.
2. Test for Ingress with addon annotations but with the addons disabled.
3. Test for Ingress with non-matching Ingress class. Verify that no load balancer is created.
4. Test for when subnets are tagged manually. Verify that the load balancer is created only in the manually tagged subnet.

### Graduation Criteria

#### Dev Preview -> Tech Preview
TBD

#### Tech Preview -> GA
TBD

#### Removing a deprecated feature
NA

### Upgrade / Downgrade Strategy
NA

### Version Skew Strategy
NA

### Operational Aspects of API Extensions

The new Custom Resource will not affect any existing operations of the cluster. However the webhooks which have to be
created for the lb-controller will impact the availability and performance of Ingress resource _create_
and _update_ operations. These webhooks are light-weight and do not perform any complex validation. So the performance
impact should be minimal. Availability of the webhook is critical because if the webhook is unavailable operations on
the Ingress resource will not be possible.

#### Failure Modes

1. Operator is unable to determine the private and public subnets in the cluster. In this case the operator will be
   marked as __Degraded__ in the status.
2. lb-controller pods are not running. This would mean that the webhook is unavailable and all operations on _Ingress_
   , _IngressClass_ and _IngressGroup_ would fail. The failure would include a message indicating that the webhook is
   unavailable and the user would have to remediate this by examining the status of the lb-controller.

#### Support Procedures
TBD

## Implementation History
TBD

## Drawbacks

Since we are reusing an existing upstream controller and restricting which of its features are enabled, some upstream
documentation would not be applicable for this operator.

## Alternatives

The existing OpenShift router could be modified to provision ALBs for Ingresses
which specify it through an annotation. The disadvantage would that the router
would be replicating a lot of the functionality which is present in the
lb-controller.
