---
title: aws-nlb-security-group-selection
authors:
  - "@asuryana"
reviewers:
  - "@alebedev87"
  - "@candita"
  - "@frobware"
  - "@gcs278"
  - "@knobunc"
  - "@Miciah"
  - "@miheer"
  - "@rfredette"
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2026-06-03
last-updated: 2026-06-17
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/NE-2386
see-also:
  - "/enhancements/cloud-integration/aws/service-aws-nlb-security-group.md"
replaces:
  - ""
superseded-by:
  - ""
---

# IngressController API for AWS NLB Security Group Selection

## Summary

This enhancement extends the IngressController API to allow administrators to
specify custom (Bring Your Own) security groups for AWS Network Load Balancers
(NLBs). A new `securityGroups` field is added to
`AWSNetworkLoadBalancerParameters`. The Cluster Ingress Operator translates
the field into the Service annotation
`service.beta.kubernetes.io/aws-load-balancer-security-groups`, which the Cloud
Controller Manager (CCM) uses to attach the specified security groups to the
NLB.

This builds on the CCM-level security group support added in the
[service-aws-nlb-security-group](/enhancements/cloud-integration/aws/service-aws-nlb-security-group.md)
enhancement.

## Motivation

The [service-aws-nlb-security-group](/enhancements/cloud-integration/aws/service-aws-nlb-security-group.md)
enhancement adds CCM support for automatically creating managed security groups
for NLBs and accepting user-provided security groups via the
`service.beta.kubernetes.io/aws-load-balancer-security-groups` Service
annotation. However, the Cluster Ingress Operator manages the
Service resource for the default router, and administrators should not directly
edit operator-managed resources. Without this enhancement, users wanting
custom security groups on the default router's NLB would need to manually edit
the Service, and those edits would be overwritten by the operator during
reconciliation.

### User Stories

#### Story 1

As an OpenShift administrator on AWS, I want to specify custom security groups
for the default Ingress Controller's NLB through the IngressController API, so
that I can use pre-existing security groups managed by my security team.

### Goals

- Add a `securityGroups` field to `AWSNetworkLoadBalancerParameters` in the
  IngressController API for specifying BYO security group IDs.
- Update the Cluster Ingress Operator to translate the new field into the
  `service.beta.kubernetes.io/aws-load-balancer-security-groups` Service
  annotation.
- Target ROSA as the primary deployment type. The feature is also
  available on self-managed OpenShift on AWS.
- Maintain backward compatibility with existing IngressControllers that do not
  specify security groups.

### Non-Goals

- Adding security group configuration at cluster installation time via
  `install-config.yaml`. The target of this epic is ROSA, which does not
  need install-time support for the default Ingress Controller.
- Automatic creation or management of security groups by the Cluster Ingress Operator.
  The CCM already handles managed security group creation; this enhancement
  only addresses the BYO use case.
- Managing security group rules through the IngressController API. Users are
  responsible for configuring ingress and egress rules on their BYO security
  groups directly in AWS.
- Support for non-AWS platforms.

## Proposal

### Workflow Description

#### Prerequisites

1. The CCM must support the BYO security group annotation
   (upstream [cloud-provider-aws#1379](https://github.com/kubernetes/cloud-provider-aws/pull/1379)).
2. The CCM cloud-config must have `NLBSecurityGroupMode = Managed`. This
   setting is required for the BYO SG annotation to be honored. The
   cloud-controller-manager-operator automatically enables this setting
   in the cloud-config when the CCM version supports it. For self-managed
   clusters, this is enforced by default starting in OpenShift 4.22.

#### Creating an IngressController with BYO security groups

1. **Cluster administrator** creates security groups in AWS:

```bash
aws ec2 create-security-group \
  --group-name my-ingress-sg \
  --description "Custom SG for OpenShift default router" \
  --vpc-id vpc-0123456789abcdef0

aws ec2 authorize-security-group-ingress \
  --group-id sg-0123456789abcdef0 \
  --protocol tcp --port 443 --cidr 0.0.0.0/0
```

2. **Cluster administrator** creates the IngressController with the security
   group IDs:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  endpointPublishingStrategy:
    type: LoadBalancerService
    loadBalancer:
      scope: External
      providerParameters:
        type: AWS
        aws:
          type: NLB
          networkLoadBalancer:
            securityGroups:
            - sg-0123456789abcdef0
```

3. **Cluster Ingress Operator** reads the `securityGroups` field and creates
   the router Service with the annotation:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: router-default
  namespace: openshift-ingress
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    service.beta.kubernetes.io/aws-load-balancer-security-groups: "sg-0123456789abcdef0"
```

4. **Cloud Controller Manager** reads the annotation and attaches the
   specified security groups to the NLB instead of creating a managed security
   group.

Adding the `securityGroups` field to an existing IngressController (including
the default) follows the effectuation pattern. See the "Effectuating Security
Group Updates" section.

#### Updating security groups on an existing IngressController

1. **Cluster administrator** updates the `securityGroups` field in the
   IngressController spec.
2. **Cluster Ingress Operator** sets the `LoadBalancerProgressing` condition
   to `Status: True` and waits for the administrator to delete the Service.

See the "Effectuating Security Group Updates" section for the complete workflow.

#### Removing BYO security groups

1. **Cluster administrator** removes the `securityGroups` field from the
   IngressController spec.
2. **Cluster Ingress Operator** sets the `LoadBalancerProgressing` condition
   to `Status: True` and waits for the administrator to delete the Service.

See the "Effectuating Security Group Updates" section for the complete workflow.

When the Service is recreated, the CCM provisions a new NLB without the BYO
security group annotation and creates a new managed security group for the NLB.
The original BYO security groups remain in AWS but are no longer associated with
the NLB.

#### Effectuating Security Group Updates

Changing the `securityGroups` field on an existing IngressController requires
deleting and recreating the Service.

1. **Cluster administrator** updates the `securityGroups` field in the
   IngressController spec.

2. **Cluster Ingress Operator** detects the change and sets the
   `LoadBalancerProgressing` condition to `Status: True`:

```yaml
status:
  conditions:
  - type: LoadBalancerProgressing
    status: "True"
    reason: LoadBalancerRecreationRequired
    message: |
      The IngressController securityGroups were changed from ["sg-old123"] to
      ["sg-new456"]. To effectuate this change, you must delete the service:
      `oc -n openshift-ingress delete svc/router-default`; the service
      load-balancer will then be deprovisioned and a new one created. This will
      cause the new load-balancer to have a different host name and IP address
      and will cause disruption.
```

3. **Cluster administrator** deletes the Service:

```bash
oc delete svc router-default -n openshift-ingress
```

4. **Cloud Controller Manager** deprovisions the existing NLB.

5. **Cluster Ingress Operator** detects the Service deletion and recreates the
   Service with the new `service.beta.kubernetes.io/aws-load-balancer-security-groups`
   annotation value.

6. **Cloud Controller Manager** provisions a new NLB with the specified security
   groups.

7. **Cluster Ingress Operator** sets `LoadBalancerProgressing` to `Status: False`
   and `LoadBalancerReady` to `Status: True` when the new NLB is provisioned.

##### Automatic Service Deletion

For automation and GitOps workflows, administrators can opt in to automatic
Service deletion by adding an annotation to the IngressController:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
  annotations:
    ingress.operator.openshift.io/auto-delete-load-balancer: "true"
spec:
  endpointPublishingStrategy:
    loadBalancer:
      providerParameters:
        aws:
          networkLoadBalancer:
            securityGroups:
            - sg-new456
```

When this annotation is present, the Cluster Ingress Operator automatically
deletes and recreates the Service when the `securityGroups` field changes. This
causes the same disruption as manual deletion (new NLB, different hostname/IP),
but without requiring manual intervention.

#### Variation: Invalid Security Group ID

1. The administrator specifies a security group ID that does not exist
   in the cluster's VPC, or belongs to a different VPC.

2. The API server validates the ID format via CRD validation rules
   (must match `sg-` followed by 8-17 hex characters) and accepts it.

3. The CCM attempts to attach the security group and fails. The error
   is surfaced as a `SyncLoadBalancerFailed` event on the Service. The
   IngressController status shows `LoadBalancerReady=False`.

4. The administrator corrects the security group ID in the
   IngressController spec. This triggers the effectuation pattern
   (see "Effectuating Security Group Updates").

### API Extensions

A new `securityGroups` field is added to `AWSNetworkLoadBalancerParameters` in
the `openshift/api` repository. This follows the same pattern as the existing
`subnets` and `eipAllocations` fields on the same struct.

```go
// AWSNetworkLoadBalancerParameters holds configuration parameters for an
// AWS Network load balancer.
type AWSNetworkLoadBalancerParameters struct {
	// subnets specifies the subnets to which the load balancer will attach.
	// ...
	// +optional
	Subnets *AWSSubnets `json:"subnets,omitempty"`

	// eipAllocations is a list of IDs for Elastic IP (EIP) addresses that
	// are assigned to the Network Load Balancer.
	// ...
	// +optional
	// +listType=atomic
	EIPAllocations []EIPAllocation `json:"eipAllocations"`

	// securityGroups is a list of security group IDs to attach to the
	// Network Load Balancer. When specified, these security groups replace
	// the managed security group that the Cloud Controller Manager would
	// otherwise create automatically. The user is responsible for
	// configuring the ingress and egress rules on the specified security
	// groups.
	//
	// The specified security groups must exist in the same VPC as the
	// cluster and must allow the necessary traffic for the
	// IngressController to function.
	//
	// When this field is omitted and NLBSecurityGroupMode is set to
	// Managed in the CCM cloud-config, the Cloud Controller Manager
	// automatically creates and manages a security group for the NLB.
	//
	// Each security group ID must be unique. A maximum of 5 security
	// groups can be specified.
	//
	// +optional
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=5
	// +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="securityGroups cannot contain duplicates"
	SecurityGroups []SecurityGroupID `json:"securityGroups,omitempty"`
}

// SecurityGroupID is an AWS EC2 security group ID.
// Values must begin with `sg-` followed by between 8 and 17 hexadecimal
// characters.
//
// +kubebuilder:validation:MinLength=11
// +kubebuilder:validation:MaxLength=20
// +kubebuilder:validation:XValidation:rule=`self.startsWith('sg-')`,message="securityGroups must start with 'sg-'"
// +kubebuilder:validation:XValidation:rule=`self.split("-", 2)[1].matches('^[0-9a-fA-F]{8,17}$')`,message="securityGroups must be 'sg-' followed by 8 to 17 hexadecimal characters"
type SecurityGroupID string
```

### Topology Considerations

The Cluster Ingress Operator behavior is identical across all deployment
topologies. It reads the `securityGroups` field from the IngressController
spec and sets the corresponding Service annotation when the Service is
created or recreated. No topology-specific logic is required.

#### Hypershift / Hosted Control Planes

No unique considerations. See above.

#### Standalone Clusters

No unique considerations. See above.

#### Single-node Deployments or MicroShift

No unique considerations. See above.

#### OpenShift Kubernetes Engine

No unique considerations. See above.

### Implementation Details/Notes/Constraints

The Cluster Ingress Operator follows the existing pattern used for `subnets` and
`eipAllocations` — it reads the `securityGroups` field from the
IngressController spec and sets the corresponding Service annotation when the
Service is created or recreated following the effectuation pattern.

The CCM behavior for BYO security groups on NLBs is documented in the
upstream [cloud-provider-aws NLB security group documentation](https://github.com/kubernetes/cloud-provider-aws/blob/31a27a5f9ac61ad68f9b4d0a8da765ff060245d3/docs/nlb_security_groups.md).

### Risks and Mitigations

- **BYO security groups with incorrect rules.** If the user specifies
  security groups that do not allow the necessary traffic, the
  IngressController will not function correctly. This is documented as
  the user's responsibility.

- **Interaction with managed security groups.** When BYO security groups
  are specified, the CCM attaches the user-provided security groups
  instead of creating a managed security group. When removing the
  securityGroups field, the Service is deleted and recreated. The CCM
  provisions a new NLB with a managed security group. The original BYO
  security groups remain in AWS but are no longer associated with any NLB.

### Drawbacks

- Adds a new field to the IngressController API, increasing API surface area.
- Users must manage security group rules outside of OpenShift, which requires
  AWS knowledge.

## Alternatives (Not Implemented)

### Continue using manual Service annotation editing

Users could manually edit the router Service to add the
`service.beta.kubernetes.io/aws-load-balancer-security-groups` annotation.
This was rejected because operator-managed resources should not be edited
directly — the Cluster Ingress Operator would overwrite the changes during
reconciliation. This approach is also not GitOps-friendly and provides a
poor user experience.


## Open Questions

None.

## Test Plan

**Unit tests:**

- Service generation with and without security groups specified.
- Validation of security group ID format (`sg-` prefix, hex characters).
- LoadBalancerProgressing status when `securityGroups` changes.

**E2E tests:**

1. Create an IngressController with `securityGroups` specified. Verify
   the router Service has the
   `service.beta.kubernetes.io/aws-load-balancer-security-groups` annotation
   with the correct value.
2. Verify the NLB is provisioned with the specified security groups
   attached (via AWS API or NLB describe).
3. Update the `securityGroups` field and verify LoadBalancerProgressing is set.
   Delete the Service and verify a new NLB is provisioned with updated security
   groups.
4. Remove the `securityGroups` field and verify LoadBalancerProgressing is set.
   Delete the Service and verify a new NLB is provisioned with a managed security
   group.
5. Add the `ingress.operator.openshift.io/auto-delete-load-balancer`
   annotation and update the `securityGroups` field. Verify the Service is
   automatically deleted and recreated with the new security groups.
6. Verify that an IngressController without `securityGroups` specified
   continues to function as before (no regression).

## Graduation Criteria

This feature will be introduced as GA when the prerequisite CCM support
is available and stable.

### Dev Preview -> Tech Preview

N/A.

### Tech Preview -> GA

**Testing requirements for GA promotion:**

- E2E tests consistently passing
- CCM BYO security group support is GA (upstream cloud-provider-aws#1379)

### Removing a deprecated feature

N/A.

## Upgrade / Downgrade Strategy

### Upgrade

Existing IngressControllers without the `securityGroups` field continue
working unchanged. The new field is optional and has no default value.

Adding the `securityGroups` field to an existing IngressController follows the
same effectuation pattern as updating or removing it. See the "Effectuating
Security Group Updates" section.

### Downgrade

Administrators must remove the `securityGroups` field from all
IngressControllers before downgrading to a version that does not support
this feature. Older CCM versions do not recognize the
`service.beta.kubernetes.io/aws-load-balancer-security-groups` annotation and
will report an error during Service reconciliation if the annotation is present.

## Version Skew Strategy

This feature requires CCM support for the BYO security group annotation
(upstream [cloud-provider-aws#1379](https://github.com/kubernetes/cloud-provider-aws/pull/1379))
and `NLBSecurityGroupMode = Managed` in the CCM cloud-config.

## Operational Aspects of API Extensions

N/A.

## Support Procedures

If the LoadBalancer Service remains in a `Pending` state after specifying
security groups, check the following:

- Check the IngressController status for `LoadBalancerReady` and `Available`
  conditions. If `LoadBalancerReady` is `False`, the NLB has not been
  provisioned:

```sh
oc get ingresscontroller <name> -n openshift-ingress-operator -o yaml
```

- Check the CCM logs for specific security group errors (e.g., invalid
  security group ID, security group in wrong VPC, missing IAM
  permissions). The IngressController status only reports that the
  LoadBalancer is not ready — the specific error details are in the CCM
  logs:

```sh
oc logs -n openshift-cloud-controller-manager -l k8s-app=aws-cloud-controller-manager
```

- Check for `SyncLoadBalancerFailed` events on the router Service:

```sh
oc get events -n openshift-ingress --field-selector involvedObject.name=router-<name>
```

- Verify the security groups exist in the same VPC as the cluster and have
  the correct ingress and egress rules configured.

## Infrastructure Needed

Because security groups are AWS objects, this proposal is valid only for the
AWS environment. E2E tests require test clusters on AWS with IAM permissions
for security group operations.
