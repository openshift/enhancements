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
last-updated: 2026-06-10
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
- Automatic creation or management of security groups by the Ingress Operator.
  The CCM already handles managed security group creation; this enhancement
  only addresses the BYO use case.
- Managing security group rules through the IngressController API. Users are
  responsible for configuring ingress and egress rules on their BYO security
  groups directly in AWS.
- Support for non-AWS platforms.

## Proposal

### Workflow Description

#### Prerequisites

1. A feature gate must be enabled (see [Open Questions](#open-questions)
   for the feature gate decision).
2. The CCM must support the BYO security group annotation
   (upstream [cloud-provider-aws#1379](https://github.com/kubernetes/cloud-provider-aws/pull/1379)).
3. The CCM cloud-config must have `NLBSecurityGroupMode = Managed`. This
   setting is required for the BYO SG annotation to be honored — without
   it, the annotation is silently ignored. How this value gets set
   depends on the feature gate decision (see
   [Open Questions](#open-questions)).
4. The master node IAM role must have the
   `elasticloadbalancing:SetSecurityGroups` permission (added in
   [openshift/installer#10512](https://github.com/openshift/installer/pull/10512)).

#### Configuring BYO security groups on the default IngressController

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

2. **Cluster administrator** updates the IngressController with the security
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
   (or updates) the router Service with the annotation:

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

#### Updating security groups on an existing IngressController

1. **Cluster administrator** updates the `securityGroups` field in the
   IngressController spec.
2. **Cluster Ingress Operator** updates the Service annotation.
3. **CCM** detects the annotation change and transitions the NLB to the new
   security groups.

**Note:** If the NLB was initially created without any security group (before
this feature was available), the NLB must be recreated to support security
groups. This is an AWS platform limitation. The administrator must delete and
recreate the IngressController, which causes downtime.

#### Removing BYO security groups

1. **Cluster administrator** removes the `securityGroups` field from the
   IngressController spec.
2. **Cluster Ingress Operator** removes the annotation from the Service.
3. **CCM** detaches the BYO security groups from the NLB. The BYO
   security groups are not deleted (the user retains ownership). The
   exact CCM behavior for transitioning back to a managed security group
   after BYO removal is not documented upstream and should be verified
   during implementation.

#### Variation: Feature Gate Not Enabled

1. The feature gate for this feature is not enabled on the cluster.

2. The `securityGroups` field is not present in the IngressController CRD
   because it is gated behind the feature gate marker.

3. Any attempt to set the `securityGroups` field is rejected by the API
   server with a validation error.

4. IngressControllers continue to work with NLBs that have no security
   groups attached (the pre-feature behavior).

#### Variation: NLB Created Before Security Group Support

1. A cluster was provisioned before the feature gate was available.
   The existing NLBs were created without security groups.

2. The cluster is upgraded to a version with the feature gate, and the
   administrator enables it.

3. The administrator adds the `securityGroups` field to the
   IngressController spec.

4. The Cluster Ingress Operator adds the annotation to the Service.

5. The CCM attempts to attach the security groups to the NLB, but AWS
   does not allow adding security groups to an NLB that was created
   without security group support. The CCM reports an error.

6. The administrator must delete the IngressController and recreate it
   to provision a new NLB with security group support. This causes
   downtime for routes served by that IngressController.

#### Variation: NLBSecurityGroupMode Not Set in Cloud-Config

1. The feature gate is enabled, but the CCM cloud-config does not have
   `NLBSecurityGroupMode = Managed`
   (e.g., the CCCMO has not been updated to set it).

2. The administrator adds `securityGroups` to the IngressController spec.

3. The Ingress Operator sets the annotation on the Service.

4. The CCM silently ignores the annotation because managed mode is not
   active. The NLB is provisioned without security groups.

5. This scenario should not occur in practice because
   `NLBSecurityGroupMode = Managed` is expected to be set automatically
   (see [Prerequisites](#prerequisites)). However, if a version skew
   exists between the CCM configuration and the Ingress Operator, this
   mismatch is possible.

#### Variation: Invalid Security Group ID

1. The administrator specifies a security group ID that does not exist
   in the cluster's VPC, or belongs to a different VPC.

2. The API server validates the ID format via CRD validation rules
   (must match `sg-` followed by 8-17 hex characters) and accepts it.

3. The CCM attempts to attach the security group and fails. The error
   is surfaced as a `SyncLoadBalancerFailed` event on the Service. The
   IngressController status shows `LoadBalancerReady=False`.

4. The administrator corrects the security group ID in the
   IngressController spec.

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
	// +openshift:enable:FeatureGate=TBD
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

The Ingress Operator behavior is identical across all deployment
topologies. It reads the `securityGroups` field from the IngressController
spec and sets the corresponding annotation on the Service. No
topology-specific logic is required.

#### Hypershift / Hosted Control Planes

No unique considerations. See above.

#### Standalone Clusters

No unique considerations. See above.

#### Single-node Deployments or MicroShift

No unique considerations. See above.

#### OpenShift Kubernetes Engine

No unique considerations. See above.

### Implementation Details/Notes/Constraints

The Ingress Operator follows the existing pattern used for `subnets` and
`eipAllocations` — it reads the `securityGroups` field from the
IngressController spec and sets the corresponding Service annotation.

The CCM behavior for BYO security groups on NLBs is documented in the
upstream [cloud-provider-aws NLB security group documentation](https://github.com/kubernetes/cloud-provider-aws/blob/31a27a5f9ac61ad68f9b4d0a8da765ff060245d3/docs/nlb_security_groups.md).

### Risks and Mitigations

- **BYO security groups with incorrect rules.** If the user specifies
  security groups that do not allow the necessary traffic, the
  IngressController will not function correctly. This is documented as
  the user's responsibility.

- **Interaction with managed security groups.** When BYO security groups
  are specified, the CCM attaches the user-provided security groups
  instead of creating a managed security group. The exact transition
  behavior between managed and BYO security groups is implemented in
  the CCM and is transparent to the Ingress Operator. The transition
  details should be verified during implementation against the upstream
  CCM behavior.


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

1. **Which feature gate should the `securityGroups` field use?** The
   existing `AWSServiceLBNetworkSecurityGroup` feature gate is for the
   managed security group feature. The CCM-side BYO SG support (PR
   #1379) may land ungated via a CCM rebase. Should the IngressController
   API field use `AWSServiceLBNetworkSecurityGroup`, a new dedicated
   feature gate, or no feature gate at all?

## Test Plan

**Unit tests:**

- Service generation with and without security groups specified.
- Validation of security group ID format (`sg-` prefix, hex characters).
- Annotation removal when `securityGroups` is cleared.

**E2E tests:**

1. Create an IngressController with `securityGroups` specified. Verify
   the router Service has the
   `service.beta.kubernetes.io/aws-load-balancer-security-groups` annotation
   with the correct value.
2. Verify the NLB is provisioned with the specified security groups
   attached (via AWS API or NLB describe).
3. Update the `securityGroups` field and verify the Service annotation
   and NLB are updated.
4. Remove the `securityGroups` field and verify the annotation is
   removed from the Service.
5. Verify that an IngressController without `securityGroups` specified
   continues to function as before (no regression).

## Graduation Criteria

This feature will be released behind a feature gate (see
[Open Questions](#open-questions) for the feature gate decision).

### Dev Preview -> Tech Preview

N/A. This feature will be introduced as Tech Preview.

### Tech Preview -> GA

**Testing requirements for GA promotion:**

Once E2E tests are consistently passing, a PR will be created to move
the feature gate to the default feature set.

### Removing a deprecated feature

N/A.

## Upgrade / Downgrade Strategy

### Upgrade

Existing IngressControllers without the `securityGroups` field continue
working unchanged. The new field is optional and has no default value.

If an administrator adds the `securityGroups` field to an existing
IngressController whose NLB was created before the feature gate was
available (i.e., the NLB has no security group attached), the NLB must
be recreated. The administrator must
delete and recreate the IngressController, which causes downtime. This is an
AWS platform limitation, not an OpenShift limitation.

### Downgrade

On downgrade to a version that does not recognize the `securityGroups` field,
the field is ignored. Existing NLBs with attached security groups continue
functioning. No managed security groups will be created or modified by older
versions.

## Version Skew Strategy

This feature requires CCM support for the BYO security group annotation
(upstream [cloud-provider-aws#1379](https://github.com/kubernetes/cloud-provider-aws/pull/1379)).
The feature gate ensures that both the Ingress Operator and CCM are at
compatible versions.

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
