---
title: configurable-dns-loadbalancerservice
authors:
  - "@thejasn"
reviewers:
  - "@alebedev87"
  - "@sherine-k"
  - "@brandisher"
approvers:
  - "@Miciah"
api-approvers:
  - "@Miciah"
creation-date: 2022-05-10
last-updated: 2022-05-10
tracking-link:
  - https://issues.redhat.com/browse/CFEPLAN-58
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Configurable DNS Management for LoadBalancerService Ingress Controllers

## Summary

The ingress operator currently assumes that it's going to create and
manage DNS using the required cloud provider integration. But in some
circumstances, the required DNS zone may be different from the cluster
DNS zone and may not even be hosted in the cloud provider. In such scenarios,
the operator reports a degraded state after provisioning the required
resources in the cloud provider because the provided DNS can't be created.

This enhancement aims to provide the end-user the ability to completely disable
DNS management on an Ingress Controller.

## Motivation

End users need to have an option to manage DNS on their own instead of relying
on the `cluster-ingress-operator`. This is quite a common requirement where
1. Customers may need to deploy ingress controllers on domains hosted outside of
   cloud providers.
2. Partners may need to deploy ingress controllers for domains from their own 
   customers (with domains not hosted in the same cloud provider).

With no support for such scenarios currently, customers end up with having the
`cluster-ingress-operator` in degraded state. The solution is to add support
in the operator to be able to optionally manage DNS record lifecycle. 

### User Stories

#### As a cluster admin, when configuring an ingresscontroller, I want to specify whether a DNS record should or should not be created for this ingress 

The cluster admin has the option to modify/create ingresscontroller to 
specify whether a DNS record should or should not be created. This is 
available as a Day-2 operation to the cluster admin.

#### As a cluster admin, when updating an ingresscontroller from managed DNS to unmanaged DNS, should reflect the same on the previous DNS Record

Upon updating `.loadbalancer.dnsManagementPolicy` to `Unmanaged` the older DNS records must 
denote the same using the `dnsManagementPolicy` field. The cluster admin can 
choose to retain or delete the DNS record at his discretion. 

### Goals

- Provide the ability to opt out of DNS management by the `cluster-ingress-operator`.
- Recover from degraded state of the `cluster-ingress-operator` during upgrades
  from OCP 3.x to 4.x where DNS was being managed externally.

### Non-Goals

- Provide a day-0 solution for cluster installations involving external DNS
  management by the customer.
- Traffic management during DNS management policy migration from managed to unmanaged.

## Proposal

Currently, there is partial support provided by the `installer` (in UPI installations)
to disable the `cluster-ingress-operator` from managing the DNS cluster-wide as documented
[here](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones). 

The proposed solution adds support for more fine-grained control over specific
ingresscontrollers on how they manage the wildcard DNS records associated with them.
Introduce `dnsManagementPolicy` to indicate current state of DNS management,
- `Managed`: It is the default state and behaves the same as the existing
  implementation. The ingresscontroller manages create/update/delete actions
  on the *DNSRecord* CR and the actual DNS records on the DNS provider.
- `Unmanaged`: In this state, the ingresscontroller will not create/update/delete
  the *DNSRecord* associated with it, nor does it create/update/delete
  the actual DNS record on the cloud provider. This responsibility entirely falls
  on the cluster admin.

### Workflow Description

This feature is designed as Day-2 solution and is geared towards cluster
admins. It mainly applies to scenarios where specific ingresscontrollers
need to be configured to not manage DNS records associated with them.

__Note__: This feature is only supported on new or non-default ingresscontrollers.
The default ingresscontroller can be modified/updated at the discretion of
the cluster admin. 

#### Modifying/Updating an ingresscontroller

The ingresscontroller will need to be deleted and recreated with the updated domain
present in if required domain is _not_ the same as cluster domain configured at
*ingress.config.openshift.io/cluster* `.spec.domain`. If not needed, the workflow
defined below is sufficient,
- The new domain that needs to be associated with the ingresscontroller
  must be created prior to making any changes.
- The ingresscontroller must be edited to set `.loadbalancer.dnsManagementPolicy` to `Unmanaged`.
- This will trigger a reconcile of the controller, resulting in updating
  the following conditions on the ingress operator
  - `DNSManaged` condition will be set to false and reason updated to
    UnmanagedDNS.
- The DNSRecord will also be updated as part of the same reconcile, 
  `.spec.dnsManagementPolicy` will be set to `Unmanaged` and the following conditions
  will be updated,
  - `DNSReady` condition will be set to true and reason updated to
    UnmanagedDNS.
- Post successfully updating the ingresscontroller, the associated *DNSRecord*
  must be deleted at the discretion of the cluster admin.
  - Since it is in the `Unmanaged` state, the complete clean-up of resources
    is the responsibility of the cluster admin.
  - The finalizer on the *DNSRecord* isn't deleted by the operator when in `Unmanaged`
    state, so the cluster admin needs to manually delete the finalizer.
  - The DNS record on the DNS provider is also not deleted by the operator since
    currently it is in `Unmanaged` state and needs to be manually deleted by the
    cluster admin.
   
    
__Note__: Similarly, to create a new ingresscontroller where DNS is managed
          externally, the same workflow can be followed. No *DNSRecord*s will be
          created in the flow.

### API Extensions

The ingresscontroller CRD is extended by adding an optional field `DNSManagementPolicy`
of type `LoadBalancerDNSManagementPolicy`. This field will default to `Managed`.

```go
// LoadBalancerStrategy holds parameters for a load balancer.
type LoadBalancerStrategy struct {
    // <snip>

    // dnsManagementPolicy indicates if the lifecyle of the wildcard DNS record
    // associated with the load balancer service will be managed by
    // the ingress operator.
    //
    // +kubebuilder:default:="Managed"
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:Enum=Managed;Unmanaged
    DNSManagementPolicy LoadBalancerDNSManagementPolicy `json:"dnsManagementPolicy"`
}

// LoadBalancerDNSManagementPolicy is a policy for configuring how
// ingresscontrollers manage DNS.
//
// +kubebuilder:validation:Enum=Managed;Unmanaged
type LoadBalancerDNSManagementPolicy string

const (
    // ManagedLoadBalancerDNS configures the ingresscontroller to manage
    // the wildcard DNS records associated with it.
    ManagedLoadBalancerDNS LoadBalancerDNSManagementPolicy = "Managed"
    // UnmanagedLoadBalancerDNS configures the ingresscontroller to not
    // manage the wildcard DNS records associated with it.
    UnmanagedLoadBalancerDNS LoadBalancerDNSManagementPolicy = "Unmanaged"
)

// DNSRecordSpec contains the details of a DNS record.
type DNSRecordSpec struct {
  // <snip>

  // dnsManagementPolicy denotes the current policy applied on the dns record.
  // Records that have policy set as "Unmanaged" are ignored by the ingress
  // operator and should be managed at the discretion of the cluster admin.
  //
  // +kubebuilder:default:="Managed"
  // +kubebuilder:validation:Optional
  // +kubebuilder:validation:Enum=Managed;Unmanaged
  DNSManagementPolicy DNSManagementPolicy `json:"dnsManagementPolicy"`
}

// DNSManagementPolicy is a policy for configuring how the dns controller 
// manages DNSRecords.
//
// +kubebuilder:validation:Enum=Managed;Unmanaged
type DNSManagementPolicy string

const (
    // ManagedDNS configures the dns controller manage the lifecyle of the
    // dns record on the appropriate platform.
    ManagedDNS DNSManagementPolicy = "Managed"
    // UnmanagedDNS configures the dns controller to ignore the dns record
    // and allows to be manually managed by the cluster admin.
    UnmanagedDNS DNSManagementPolicy = "Unmanaged"
)
```

Re-uses existing condition `DNSManaged` of the controller to denote if
the wildcard *DNSRecord* associated with the controller is managed/unmanaged. 

### Implementation Details/Notes/Constraints

Based on `.loadbalancer.dnsManagementPolicy`, the controller decides whether to ensure creating
a wildcard *DNSRecord*. If the customer is updating `.loadbalancer.dnsManagementPolicy` from 
`"Managed"` to `"Unmanaged"` the previously created *DNSRecord* will be updated
appropriately.

The existing `.loadbalancer.dnsManagementPolicy` value of the ingresscontroller
will be replicated to the *DNSRecord*'s `.spec.dnsManagementPolicy` as well, to
clearly state which dns record is unmanaged. The cluster admin can choose to delete
the *DNSRecord* at his own discretion (this will involve also manually removing the finalizer).

Once the *DNSRecord* is set to `Unmanaged`, any updates done to it will be ignored
by the operator, but it is to be noted that if the ingresscontroller is updated from
`Unmanaged` to `Managed` any changes done will be lost during re-sync.

If the ingresscontroller is deleted, and the associated *DNSRecord* still exists
it will also be marked for deletion and finalizer on the *DNSRecord* will not be
removed by the operator, so the cluster admin will need to manually remove the
finalizer on the *DNSRecord* to ensure it is deleted along with the ingresscontroller.
This ensures we don't have any orphaned *DNSRecords* (records that don't have an associated
ingresscontroller). The operator also will not delete the DNS records on the DNS provider
that are associated with the *DNSRecord* CR, this will need to be manually deleted
by the cluster admin.

The support provided by the installer and this new field aren't entirely
mutually exclusive, i.e. if customer has opted to disable DNS management cluster
wide (as documented
[here](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones)),
this field is of no value and need not be set since *DNSRecord* created is silently
reconciled by the DNS controller.

__Note:__ Appropriate logging is a must on all controllers clearly indicating the
current DNS management policy set on both ingresscontroller and DNSRecord and any
subsquent effects such as skipping reconciliation, etc must also be logged.

### Risks and Mitigations

This feature does not support default ingresscontrollers, if updated to be unmanaged
in conjunction with updating the domain could result in breaking cluster connectivity
if not all components are properly updated. This should be done at the discretion
of the cluster admin. The manual creation of the required DNS records will need to be
done prior to making this change, as the canary controller will try to ensure that the
ingress domains are reachable, and otherwise this sets the `cluster-ingress-operator`
in a degraded state.

### Drawbacks

After an ingresscontroller is updated from `Managed` to `Unmanaged`, the previous
*DNSRecord* is left in an `Unmanaged` state which could result in orphaned resources
on the cluster and the cloud provider.

## Design Details

### Open Questions

Currently, as per documentation `.spec.domain` in the ingresscontroller cannot be
updated after creation since updating the [domain](https://github.com/openshift/cluster-ingress-operator/blob/972f09b9dbb181ae5c414da2a990b57c60fde9d8/pkg/operator/controller/ingress/controller.go#L342-L355)
in the ingresscontroller doesn't result in [updating](https://github.com/openshift/cluster-ingress-operator/blob/972f09b9dbb181ae5c414da2a990b57c60fde9d8/pkg/operator/controller/ingress/dns.go#L97)
the wildcard *DNSRecord*. 
> 1. Can this logic be changed to support moving from managed DNS to
     unmanaged DNS on the `default` ingresscontroller?
     
  __Closed:__ The operator will continue to retain immutability of `.spec.domain`
              and will not support updating domain when migrating from `Managed` to 
              `Unmanaged` DNS.
  

### Test Plan

- Test by creating a new secondary ingresscontroller with a custom domain
  and `.loadbalancer.dnsManagementPolicy` set to `Unmanaged`. Ensure proper conditions
  and connectivity.
- Test deletion of ingresscontrollers to ensure correct behaviour of the associated
  *DNSRecord*.  
- Test the following update paths on a custom ingresscontroller
  - Updating `.loadbalancer.dnsManagementPolicy` from `Managed` -> `Unmanaged` -> `Managed` and
    to ensure no conflicts during creation/recreation of *DNSRecord*.
  - Updating `Unmanaged` -> `Managed` and to ensure creation of new *DNSRecord*
    and correct conditions.

### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

On upgrades, `.loadbalancer.dnsManagementPolicy` will default to `Managed` which is consistent with the
older versions of the ingress operator.

On downgrades, there are 2 possible variations
* When downgrading to an unsupported version, having the `.loadbalancer.dnsManagementPolicy` set to
  `Unmanaged`,
  * If the cluster domain is not the same as the domain in the ingresscontroller,
    could result in failing to create the wildcard *DNSRecord* associated on the 
    ingresscontroller and thereby result in the `cluster-ingress-operator` entering 
    a degraded state. To recover DNS management will have to be disabled cluster 
    wide ([docs](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones)).
  * When the cluster domain is the same as the domain in the ingresscontroller,
    this will not cause any issues during downgrades, but a new wildcard *DNSRecord*
    will be created by the operator. 
* When downgrading to an unsupported version, having the `.loadbalancer.dnsManagementPolicy` set to
  `Managed` or leaving it as the default in the ingresscontroller will not result
  in any issue as it would be consistent with the older versions.

### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

N/A.

#### Failure Modes

- Application workloads will experiance connectivity issues, when replacing/re-creating
  an ingresscontroller from `Managed` to `Unmanaged` since the associated deployment,
  routes, loadbalancer will be recreated/reconfigured. Eventually, once all components
  reach stable conditions, workload connectivity should be restored.

#### Support Procedures

N/A.

## Implementation History

N/A.

## Alternatives

N/A.
