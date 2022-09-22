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
the operator reports a degraded state after attempting to provision the required
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
in the operator to be able to optionally manage DNS record lifecycle on the
cloud provider. 

### User Stories

#### As a cluster admin, when configuring an ingresscontroller, I want to specify whether a DNS record should or should not be created on the cloud provider for this ingress 

The cluster admin has the option to modify/create the ingresscontroller to 
specify whether a DNS record on the cloud provider should or should not be created. This is 
available as a Day-2 operation to the cluster admin.

#### As a cluster admin, when updating an ingresscontroller from managed DNS to unmanaged DNS, I want confirmation that the operator is not managing DNS

Upon updating the IngressController CR's `.spec.endpointPublishingStrategy.loadBalancer.dnsManagementPolicy`
field to `Unmanaged`, the existing *DNSRecord* CR must denote the same using the
`dnsManagementPolicy` field. The cluster admin can  choose to retain or delete the
DNS record in the cloud provider at his discretion. 

### Goals

- Provide the ability to opt out of DNS management by the `cluster-ingress-operator` for 
  an individual ingresscontroller.
- Enable migration from OCP 3.x to 4.x where DNS was being managed externally.

### Non-Goals

- Provide a day-0 solution for cluster installations involving external DNS
  management by the customer.
- DNS traffic management during DNS management policy migration from managed to unmanaged.

## Proposal

The proposed solution adds support for more fine-grained control over specific
ingresscontrollers on how they manage the wildcard DNS records associated with them.
Introduce `dnsManagementPolicy` to indicate current state of DNS management,
- `Managed`: It is the default state and behaves the same as the existing
  implementation. The ingresscontroller manages the DNS record lifecycle on the
  cloud provider.
- `Unmanaged`: In this state, the ingresscontroller will not manage (create/update/delete)
  the DNS record on the cloud provider. This responsibility entirely falls
  on the cluster admin. The *DNSRecord* CR is retained with the field `dnsManagementPolicy`
  set to `Unmanaged`.

### Workflow Description

This feature is designed as a Day-2 solution and is geared towards cluster
admins. It mainly applies to scenarios where specific ingresscontrollers
need to be configured to not manage DNS records associated with them.

__Note__: This feature is only supported on new or non-default ingresscontrollers.
The default ingresscontroller can be modified/updated at the discretion of
the cluster admin. 

#### Modifying/Updating an ingresscontroller

- A valid DNS record must be created in advance either on a cloud provider or any
  external provider so that it can be associated with an ingresscontroller.
- If the ingresscontroller already exists it must be updated to set 
  `.loadbalancer.dnsManagementPolicy` to `Unmanaged`.
  - ```bash
    SCOPE=$(oc -n openshift-ingress-operator get ingresscontroller <name> -o=jsonpath="{.status.endpointPublishingStrategy.loadBalancer.scope}")
    oc -n openshift-ingress-operator patch ingresscontrollers/<name> --type=merge --patch='{"spec":{"endpointPublishingStrategy":{"type":"LoadBalancerService","loadBalancer":{"dnsManagementPolicy":"Unmanaged", "scope":"${SCOPE}"}}}}'
    ```
- This will trigger a reconcile of the controller, resulting in updating
  the following conditions on the ingress operator
  - `DNSManaged` condition will be set to `False` and reason updated to
    UnmanagedDNS.
  - `DNSReady` condition will be set to `Unknown` and reason updated to
    UnmanagedDNS.    
- The DNSRecord will also be updated as part of the same reconcile, 
  `.spec.dnsManagementPolicy` will be set to `Unmanaged`.
- Post successfully updating the ingresscontroller, the associated DNS record in the
  cloud provider must be deleted at the discretion of the cluster admin.
  - Since it is in the `Unmanaged` state, the complete clean-up of resources
    is the responsibility of the cluster admin.
   
    
__Note__: Similarly, to create a new ingresscontroller where DNS is managed
          externally, the same workflow can be followed.

### API Extensions

The ingresscontroller CRD is extended by adding an optional field `DNSManagementPolicy`
of type `LoadBalancerDNSManagementPolicy`. This field will default to `Managed`.

```go
// LoadBalancerStrategy holds parameters for a load balancer.
type LoadBalancerStrategy struct {
    // <snip>

    // dnsManagementPolicy indicates if the lifecycle of the wildcard DNS record
    // associated with the load balancer service will be managed by
    // the ingress operator. It defaults to Managed.
    // Valid values are: Managed and Unmanaged.
    //
    // +kubebuilder:default:="Managed"
    // +kubebuilder:validation:Required
    // +default="Managed"
    DNSManagementPolicy LoadBalancerDNSManagementPolicy `json:"dnsManagementPolicy"`
}

// LoadBalancerDNSManagementPolicy is a policy for configuring how
// ingresscontrollers manage DNS.
//
// +kubebuilder:validation:Enum=Managed;Unmanaged
type LoadBalancerDNSManagementPolicy string

const (
    // ManagedLoadBalancerDNS specifies that the operator manages
    // a wildcard DNS record for the ingresscontroller.
    ManagedLoadBalancerDNS LoadBalancerDNSManagementPolicy = "Managed"
    // UnmanagedLoadBalancerDNS specifies that the operator does not manage
    // any wildcard DNS record for the ingresscontroller.
    UnmanagedLoadBalancerDNS LoadBalancerDNSManagementPolicy = "Unmanaged"
)
```

The DNSRecord CRD is similarly extended by adding an optional field
`DNSManagementPolicy` of type `DNSManagementPolicy`. This field's value
will default to `Managed`.

```go
// DNSRecordSpec contains the details of a DNS record.
type DNSRecordSpec struct {
    // <snip>

    // dnsManagementPolicy denotes the current policy applied on the DNS
    // record. Records that have policy set as "Unmanaged" are ignored by
    // the ingress operator.  This means that the DNS record on the cloud
    // provider is not managed by the operator, and the "Published" status
    // condition will be updated to "Unknown" status, since it is externally
    // managed. Any existing record on the cloud provider can be deleted at
    // the discretion of the cluster admin.
    //
    // This field defaults to Managed. Valid values are "Managed" and
    // "Unmanaged".
    //
    // +kubebuilder:default:="Managed"
    // +kubebuilder:validation:Required
    // +default="Managed"
    DNSManagementPolicy DNSManagementPolicy `json:"dnsManagementPolicy"`
}

// DNSManagementPolicy is a policy for configuring how the dns controller 
// manages DNSRecords.
//
// +kubebuilder:validation:Enum=Managed;Unmanaged
type DNSManagementPolicy string

const (
    // ManagedDNS configures the dns controller to manage the lifecycle of the
    // DNS record on the cloud platform.
    ManagedDNS DNSManagementPolicy = "Managed"
    // UnmanagedDNS configures the dns controller not to create a DNS record or 
    // manage any existing DNS record and allows the DNS record on the cloud 
    // provider to be managed by the cluster admin.
    UnmanagedDNS DNSManagementPolicy = "Unmanaged"
)
```

The `Failed` condition type in `DNSZoneCondition` is replaced with a new type,
`Published`. This would just act as an inverse of `Failed` condition and will
help in depicting `Unknown` statuses.

```go
var (
    // Failed means the record is not available within a zone.
    // DEPRECATED: will be removed soon, use DNSRecordPublishedConditionType.
    DNSRecordFailedConditionType = "Failed"

    // Published means the record is published to a zone.
    DNSRecordPublishedConditionType = "Published"
)
```

This enhancement re-uses the IngressController's existing `DNSManaged` status condition to denote if
the wildcard *DNSRecord* associated with the IngressController is managed/unmanaged. 

### Implementation Details/Notes/Constraints

With this enhancement, the ingress operator creates the wildcard *DNSRecord* CR under
the same conditions as before this enhancement, but the operator now sets the
`spec.dnsManagementPolicy` field on the DNSRecord CR.  
If the user is updating the IngressController's `dnsManagementPolicy` field from 
`"Managed"` to `"Unmanaged"`, the previously created *DNSRecord* CR will be updated
appropriately.

The existing `.loadbalancer.dnsManagementPolicy` value of the ingresscontroller
will be replicated to the *DNSRecord*'s `.spec.dnsManagementPolicy` as well, to
clearly state which dns record is unmanaged. If the cluster admin were to delete
the *DNSRecord* CR, the operator would recreate it (again reflecting and respecting the 
IngressController's `spec.endpointPublishingStrategy.loadBalancer.dnsManagementPolicy` field).

Once the *DNSRecord* is set to `Unmanaged`, any updates done to it will be ignored
by the operator, but it is to be noted that if the ingresscontroller is updated from
`Unmanaged` to `Managed` any changes done will be lost during re-sync.

If the ingresscontroller is deleted when `.loadbalancer.dnsManagementPolicy` is set to
`Unmanaged`, the *DNSRecord* CR will be deleted as per the current implementation, but the
DNS record on the cloud provider will not be deleted by the operator and will require
the cluster admin to manually delete it. 

If `.spec.domain` on the ingresscontroller does not contain the `baseDomain` of the
cluster DNS config, then the `.loadbalancer.dnsManagementPolicy` is automatically set
to `Unmanaged`.

The support provided by the installer and this new field aren't entirely
mutually exclusive, i.e. if customer has opted to disable DNS management cluster
wide (as documented
[here](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones)),
this field is of no value and need not be set since reconciliation of the *DNSRecord*
CR is a no-op when no DNS zones are configured.

__Note:__ 

- Appropriate logging is a must on all controllers clearly indicating the
  current DNS management policy set on both ingresscontroller and DNSRecord and any
  subsequent effects, such as skipping reconciliation etc., must also be logged.
- Automatic `dnsManagementPolicy` assignment based on the `domain` is currently only
  supported on the AWS Platform. It will be expanded to other clouds once we are sure
  no one is depending on this behaviour ([BZ#2041616](https://bugzilla.redhat.com/show_bug.cgi?id=2041616)
  for additional information).

### Risks and Mitigations

This feature does not support default ingresscontrollers when the ingresscontroller is 
updated to be unmanaged in conjunction with updating the domain could result in breaking
cluster connectivity if not all components are properly updated. This should be done at 
the discretion of the cluster admin. The manual creation of the required DNS records will
need to be done prior to making this change, as the canary controller will try to ensure
that the ingress domains are reachable, and otherwise this sets the `cluster-ingress-operator`
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
  *DNSRecord* CR and the DNS record on the cloud provider.  
- Test the following update paths on a custom ingresscontroller
  - Updating `.loadbalancer.dnsManagementPolicy` from `Managed` -> `Unmanaged` -> `Managed` and
    to ensure no conflicts during creation/recreation of DNS record on the cloud provider.
  - Updating `Unmanaged` -> `Managed` and to ensure creation of new DNS record on the cloud
    provider and the correct status.

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
* When downgrading to a version of OpenShift without this enhancement, having the `.loadbalancer.dnsManagementPolicy` set to
  `Unmanaged`,
  * If the cluster domain is not the same as the domain in the ingresscontroller,
    could result in failing to create the wildcard DNS record that is associated on the 
    ingresscontroller and thereby result in the `cluster-ingress-operator` entering 
    a degraded state. To recover DNS management will have to be disabled cluster 
    wide ([docs](https://github.com/openshift/installer/blob/master/docs/user/aws/install_upi.md#remove-dns-zones)).
  * When the cluster domain is the same as the domain in the ingresscontroller,
    this will not cause any issues during downgrades, but a new wildcard DNS record
    will be created by the operator. 
* When downgrading to an unsupported version, having the `.loadbalancer.dnsManagementPolicy` set to
  `Managed` or leaving it as the default in the ingresscontroller will not result
  in any issue as it would be consistent with the older versions.

### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

N/A.

#### Failure Modes

- Application workloads will experience connectivity issues, when replacing/re-creating
  an ingresscontroller from `Managed` to `Unmanaged` since the operator must recreate
  the associated deployment and loadbalancer and the cluster administrator must
  reconfigure DNS. Eventually, once all components reach stable conditions, workload
  connectivity should be restored.

#### Support Procedures

N/A.

## Implementation History

N/A.

## Alternatives

N/A.
