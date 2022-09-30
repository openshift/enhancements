---
title: lb-allowed-source-ranges
authors:
  - "@suleymanakbas91"
reviewers:
  - "@jerpeter1"
  - "@knobunc"
  - "@Miciah"
  - "@candita"
  - "@frobware"
  - "@rfredette"
  - "@gcs278"
  - "@deads2k"
approvers:
  - "@frobware"
  - "@Miciah"
api-approvers:
  - "@deads2k"
creation-date: 2022-07-08
last-updated: 2022-09-30
tracking-link:
  - "https://issues.redhat.com/browse/NE-555"
see-also:
replaces:
superseded-by:
---

# LoadBalancer Allowed Source Ranges

## Summary

This enhancement extends the IngressController API to allow the user to specify a 
list of IP address ranges for an IngressController to which access to the LoadBalancer Service 
should be restricted when `endpointPublishingStrategy` is `LoadBalancerService`. 
By default, all source addresses are allowed (which is equivalent to
specifying "0.0.0.0/0" for IPv4 and "::/0" for IPv6). Currently, the only way for the customers to limit 
this is by setting the `.Spec.LoadBalancerSourceRanges` field or 
`service.beta.kubernetes.io/load-balancer-source-ranges` annotation for the LoadBalancer Service
in front of the router in cloud environments. This is not desired as we prefer
the customers to use the APIs and to not directly modify the managed resources.

## Motivation

Customer security teams are concerned that the public ELB (created in 
the installation process by default) is allowing all inbound traffic on 0.0.0.0 
on ports 80 and 443, so they wish to restrict it. 

Customers have been setting `.Spec.LoadBalancerSourceRanges` field or
`service.beta.kubernetes.io/load-balancer-source-ranges` annotation for the LoadBalancer 
Service themselves, something that we did not plan to support but also did not 
actively reject. We recognize that users consider it now to be an existing feature 
while we prefer they use APIs and not directly modify managed resources.

The main motivation of this enhancement is to allow the customers to enable 
and configure `.Spec.LoadBalancerSourceRanges` through a new API.

### User Stories

#### As a cluster admin, when configuring an IngressController, I want to limit access to the load balancer to a specified list of IP address ranges on the cloud provider for this ingress

The cluster admin has the option to modify the IngressController to specify 
a list of IP address ranges for an IngressController to which access to 
the load balancer should be restricted. This is available as a Day-2 operation 
to the cluster admin.

#### As a cluster administrator, I want to migrate a cluster from using the service.beta.kubernetes.io/load-balancer-source-ranges annotation to using the new IngressController API field

The cluster admin set the `service.beta.kubernetes.io/load-balancer-source-ranges` annotation
in previous versions, and now she wants to migrate to the new IngressController API field.
She makes sure that initially `service.beta.kubernetes.io/load-balancer-source-ranges` is set 
and `spec.loadBalancerSourceRanges` is unset. She upgrades OpenShift to v4.12. She sets 
`spec.endpointPublishingStrategy.loadBalancer.allowedSourceRanges` on the IngressController.
The ingress operator then sets `spec.loadBalancerSourceRanges` based on `AllowedSourceRanges`
and clears `service.beta.kubernetes.io/load-balancer-source-ranges` service annotation.

### Goals

Enable the configuration of a range for a load balancer via the `IngressController.Spec`, 
specifically via the `LoadBalancerStrategy`, with a new parameter `AllowedSourceRanges`, 
which represents a list of IP address ranges. This is then used by the operator to set
`.Spec.LoadBalancerSourceRanges` of the LoadBalancer Service in front of the router 
in cloud environments.

Enable the migration of users who set the `service.beta.kubernetes.io/load-balancer-source-ranges`
annotation to the new IngressController API field.

### Non-Goals

This enhancement does not change the behavior or API for LoadBalancer-type services 
in general (that is, services that aren't managed by the ingress operator).

## Proposal

The IngressController API is extended by adding an optional field `AllowedSourceRanges` 
of type `[]CIDR` to `LoadBalancerStrategy`. If this field is empty, all source addresses 
are allowed (which is equivalent to specifying "0.0.0.0/0" for IPv4 and "::/0" for IPv6).

[openshift/api#1222](https://github.com/openshift/api/pull/1222) contains the literal API 
changes that we're proposing for this EP.

### Workflow Description

1. A cluster admin who wants to limit access to the load balancer to a list of IP address ranges
of their choice sets the `AllowedSourceRanges` field. 
2. The ingress operator reconciles the LoadBalancer Service and sets the `.Spec.LoadBalancerSourceRanges`
based on `AllowedSourceRanges`.

#### Variation [optional]

If a cluster admin already set `service.beta.kubernetes.io/load-balancer-source-ranges` annotation,
the workflow differs to be a migration workflow.

1. A cluster admin ensures that `service.beta.kubernetes.io/load-balancer-source-ranges` is set and 
`.Spec.LoadBalancerSourceRanges` is unset.
2. She upgrades her cluster to OpenShift v4.12.
3. She sets `AllowedSourceRanges` on the IngressController.
4. The ingress operator then sets `Spec.LoadBalancerSourceRanges` based on `AllowedSourceRanges` 
and clears `service.beta.kubernetes.io/load-balancer-source-ranges` service annotation.

### API Extensions

This proposal will modify the `IngressController` API by adding a new
field called `AllowedSourceRanges` to the `LoadBalancerStrategy` 
struct type and a new type `CIDR` that is a string in CIDR notation.

[openshift/api#1222](https://github.com/openshift/api/pull/1222) contains the literal API changes that we're
proposing for this EP.

### Implementation Details/Notes/Constraints [optional]

The value set for `AllowedSourceRanges` will be used to set `.Spec.LoadBalancerSourceRanges` 
of the LoadBalancer Service in front of the router in cloud environments.

If `.Spec.LoadBalancerSourceRanges` is set and `AllowedSourceRanges` is empty, 
the controller won't set `.Spec.LoadBalancerSourceRanges` 
on the service to prevent unintentional overwriting during an upgrade to OpenShift v4.12. 
This will result in marking the cluster as `Progressing=True` and `EvaluationConditionsDetected` 
as `.Spec.LoadBalancerSourceRanges` and `AllowedSourceRanges` do not match. 
`EvaluationConditionsDetected` will be used to evaluate how many customer clusters will be affected
if we block upgrades at a later release.
Based on this information, a future OpenShift release may block upgrades when `.Spec.LoadBalancerSourceRanges` 
and `AllowedSourceRanges` do not match, and in a release subsequent to that, `AllowedSourceRanges` 
will fully own and overwrite `.Spec.LoadBalancerSourceRanges`, also when it is empty.

`AllowedSourceRanges` in the IngressController's status will reflect the effective value by looking at the `.Spec.LoadBalancerSourceRanges`
field and the `service.beta.kubernetes.io/load-balancer-source-ranges` annotation as the annotation
takes precedence over the field.

The following table aims to show all the cases: 

| IngressController spec.allowedSourceRanges | Service annotation  beta.kubernetes.io/load-balancer-source-ranges | Service spec.loadBalancerSourceRanges | Effective value on the service (reflected in allowedSourceRanges in status) | Ingress Operator action                          | progressing status                               |
|--------------------------------------------|--------------------------------------------------------------------|---------------------------------------|-----------------------------------------------------------------------------|--------------------------------------------------|--------------------------------------------------|
| []                                         | []                                                                 | [foo]                                 | [foo]                                                                       | nothing                                          | true -- ingress.spec does not match service.spec |
| [bar]                                      | []                                                                 | [foo]                                 | [foo]                                                                       | Writes service.spec=[bar]                        | false                                            |
| [bar]                                      | []                                                                 | []                                    | []                                                                          | Writes service.spec=[bar]                        | false                                            |
| []                                         | [cow]                                                              | [foo]                                 | [foo]                                                                       | nothing                                          | true -- clear service annotation                 |
| [bar]                                      | [cow]                                                              | [foo]                                 | [foo]                                                                       | Writes service.spec=[bar], clears the annotation | false                                            |
| []                                         | [foo]                                                              | [foo]                                 | [foo]                                                                       | nothing                                          | true -- clear service annotation                 |
| [bar]                                      | [foo]                                                              | [foo]                                 | [foo]                                                                       | Writes service.spec=[bar], clears the annotation | false                                            |
| []                                         | [foo]                                                              | []                                    | [foo]                                                                       | nothing                                          | true -- clear service annotation                 |
| [bar]                                      | [foo]                                                              | []                                    | [foo]                                                                       | Writes service.spec=[bar], clears the annotation | false                                            |
| [foo]                                      | [cow]                                                              | [foo]                                 | [foo]                                                                       | Clears the annotation                            | false                                            |

### Risks and Mitigations

One risk of this proposal is that customers may have already set the
`.Spec.LoadBalancerSourceRanges` field of the LoadBalancer Service in a previous version
and this configuration gets overwritten during an upgrade. 
This risk is mitigated by not setting `.Spec.LoadBalancerSourceRanges` when `AllowedSourceRanges`
is empty.

### Drawbacks

This enhancement may bring additional complexity for the customers who are 
used to modify directly the `Spec.LoadBalancerSourceRanges` of the LoadBalancer 
Service for the same purposes. 

## Design Details

### Open Questions [optional]

1. On Kubernetes API reference, the field 
[`loadBalancerSourceRanges`](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.24/#service-v1-core) 
has the warning "This field will be ignored if the cloud-provider does not support the feature.".
However, there is no information about which platforms support this. Is there any platform
OpenShift runs on which does not support this feature?

A. I could only find this statement from Kubernetes
[v1.18 website repository](https://github.com/kubernetes/website/blob/dev-1.18/content/en/docs/tasks/access-application-cluster/configure-cloud-provider-firewall.md): 
"This feature is currently supported on Google Compute Engine, Google Kubernetes Engine, 
AWS Elastic Kubernetes Service, Azure Kubernetes Service, and IBM Cloud Kubernetes Service". 
The current version of the document does not mention `.Spec.LoadBalancerSourceRanges` anymore.

### Test Plan

The operator that manages the IngressController Deployment and related
resources has unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality.  

The operator has end-to-end tests; for this enhancement, a test is added that verifies
the following scenarios:

1. Adding/deleting/changing `AllowedSourceRanges` on the IngressController and 
observing `.Spec.LoadBalancerSourceRanges` on the LoadBalancer Service.
2. Adding `service.beta.kubernetes.io/load-balancer-source-ranges` annotation to 
a LoadBalancer Service and observing cluster's progressing and `EvaluationConditionsDetected` statuses.
3. Setting `.Spec.LoadBalancerSourceRanges` on the LoadBalancer Service 
when `AllowedSourceRanges` is empty and observing cluster's progressing and `EvaluationConditionsDetected` statuses.

This enhancement adds an upgrade test in openshift/origin to make sure that 
the migration workflow explained under **Variation** above works seamlessly.
In detail, it implements 
[Test](https://github.com/kubernetes/kubernetes/blob/master/test/e2e/upgrades/upgrade.go#L46) 
interface that includes the following steps:

1. In `Setup`, create an IngressController that requests an LB, 
and set the `service.beta.kubernetes.io/load-balancer-source-ranges` annotation
on the resulting LoadBalancer-type service.
2. In `Test`, periodically check the service to verify that nothing updates the annotation
or the `Spec.LoadBalancerSourceRanges` field on the service during the upgrade.
3. In `Test`, verify again when the upgrade is done that nothing has updated the annotation
or `Spec.LoadBalancerSourceRanges`, and verify that the operator has set `Progressing=True`.
4. Then set `AllowedSourceRanges`, remove the annotation, and finally verify that the operator 
has set `Progressing=False` and `Spec.LoadBalancerSourceRanges` accordingly.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A. This feature will go directly to GA.

#### Tech Preview -> GA

N/A. This feature will go directly to GA.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The `AllowedSourceRanges` field is empty by default, so it will be empty on an upgraded cluster, until the cluster admin 
sets it after the upgrade. If the cluster admin sets `AllowedSourceRanges` and then downgrades the cluster, 
the `AllowedSourceRanges` field will vanish from the CRD; however, the downgraded operator would not update or 
remove the `Spec.LoadBalancerSourceRanges` field value and would not update or remove the `service.beta.kubernetes.io/load-balancer-source-ranges`
annotation.

Additionally, the cluster will be marked as `EvaluationConditionsDetected` on the following cases. This will give an idea of
how many customer clusters will be affected if we block upgrades at a later release.
- If `service.beta.kubernetes.io/load-balancer-source-ranges` annotation is set 
to guide the cluster admin towards using the IngressController API and deprecate
use of the service annotation for ingress.
- If `Spec.LoadBalancerSourceRanges` on the service is nonempty and `AllowedSourceRanges` on the 
IngressController is empty. This can be the case when the cluster admin has configuration in 
`Spec.LoadBalancerSourceRanges` prior to upgrading to OpenShift 4.12 that is not overwritten 
by `AllowedSourceRanges` as it is empty in the beginning.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

This enhancement is being implemented in OpenShift 4.12.

## Alternatives

This can be done (is already being done by some customers) by directly 
setting `.Spec.LoadBalancerSourceRanges` or by adding 
`service.beta.kubernetes.io/load-balancer-source-ranges` annotation
to the LoadBalancer Service. However, we prefer the customers use the APIs 
and not directly modify managed resources as these can be overwritten and lead 
to unexpected behavior.

Another alternative would be moving to a private cluster, but this would not
be an option for many customers who just want to filter the incoming traffic.

## Infrastructure Needed [optional]

N/A
