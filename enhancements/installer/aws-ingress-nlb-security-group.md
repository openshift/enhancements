---
title: aws-ingress-nlb-security-group
authors:
  - "@mtulio"
reviewers:
  - "@rvanderp3"     # SPLAT review
  - "@patrickdillon" # Installer API updates
  - "@JoelSpeed"     # OCP API and CCM-AWS
  - "@elmiko"        # CCM-AWS
  - "@Miciah"        # Cluster Ingress Operator
  - # TBD ROSA Classic
  - # TBD ROSA HCP
approvers:
  - "@elmiko"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-05-20
last-updated: 2025-06-04
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1553
  - https://issues.redhat.com/browse/SPLAT-2137
see-also:
  - "/enhancements/enhancements/installer/aws-load-balancer-type.md"
replaces:
  - None
superseded-by:
  - None
---

# Supporting Security Groups for NLBs on AWS through Ingress

## Summary

This enhancement proposes adding support for automatically creating and managing an AWS Security Group (SG) for Network Load Balancers (NLB) on Service resources managed by the AWS Cloud Controller Manager (CCM). This enhances the default OpenShift Ingress Controller when deployed on AWS.

This feature introduces a cloud provider configuration to enforce a Security Group when creating a Service type-LoadBalancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-type` set to `nlb`. This allows administrators to enhance the security of their ingress traffic, similar to how Security Groups are currently managed for Classic Load Balancers (CLBs). The implementation will primarily involve changes within the AWS Cloud Controller Manager (CCM) and the Cluster Cloud Controller Manager Operator (CCCMO).

## Motivation

Customers deploying OpenShift on AWS using Network Load Balancers (NLBs) for the default router have expressed the need for security configuration similar to that provided by Classic Load Balancers (CLBs), where a security group is created and associated with the Load Balancer, and managed by CCM. This allows for more granular control over inbound and outbound traffic at the load balancer level, aligning with AWS security best practices and addressing security findings that flag the lack of a security group in the NLB provisioned by the CCM.

The default router in OpenShift, an IngressController object managed by the Cluster Ingress Controller Operator (CIO), can be created with a Service type-LoadBalancer using an NLB instead of the default Classic Load Balancer (CLB) during installation. This can be achieved through opt-in configuration in the `install-config.yaml` on self-managed clusters, or enforced by default by ROSA Classic and HCP. Currently, the Cloud Controller Manager (CCM), which satisfies `Service` resources, provisions an AWS Load Balancer of type NLB without a Security Group (SG) directly attached to it. Instead, security rules are managed on the worker nodes' security groups.

AWS [announced support for Security Groups when deploying an NLB in August 2023][nlb-supports-sg], but the CCM for AWS (within kubernetes/cloud-provider-aws) does not currently implement the feature of automatically creating and managing security groups for `Service` resources type-LoadBalancer using NLBs. While the [AWS Load Balancer Controller (ALBC/LBC)][aws-lbc] project already supports deploying security groups for NLBs, this enhancement focuses on adding minimal, opt-in support to the existing CCM to address immediate customer needs without a full migration to the LBC. This approach aims to provide the necessary functionality without requiring significant changes in other OpenShift components like the Ingress Controller, installer, ROSA, etc.

Using a Network Load Balancer is a recommended network-based Load Balancer by AWS, and attaching a Security Group to an NLB is a security best practice. NLBs also do not support attaching security groups after they are created.

[nlb-supports-sg]: https://aws.amazon.com/about-aws/whats-new/2023/08/network-load-balancer-supports-security-groups/
[aws-lbc]: https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/

### User Stories

- As an OpenShift administrator, I want to deploy a cluster on AWS (self-managed, ROSA Classic, and ROSA HCP) using a Network Load Balancer with Security Groups for the default router service, so that I can comply with AWS best practices and address "security findings"[1].

- As a Developer, I want to create the Default Ingress Controller on OpenShift on AWS using a NLB with a Security Group.

- As a Developer, I want to deploy a Service type-LoadBalancer with a NLB and security groups.

- As an OpenShift Engineer of CIO, I want the CCM to manage the Security Group when creating a resource `Service` type-LoadBalancer NLB, so that it:
  - a) decreases the amount of provider-specific changes on CIO;
  - b) decreases the amount of maintained code/projects by the team (e.g., ALBC);
  - c) enhances new configurations to the Ingress Controller when using NLB;
  - d) decreases the amount of images in the core payload;

- As an OpenShift Engineer, I want to make Security Groups managed by CCM to be the default on OpenShift deployments when creating a Service type-LoadBalancer NLB, providing a mechanism to automatically use Security Groups for the Default router in new deployments, or when it is recreated, ensuring best practices adoption on OpenShift products.

[1] TODO: "Security Findings" need to be expanded to collect exact examples. This comes from the customer's comment: https://issues.redhat.com/browse/RFE-5440?focusedId=25761057&page=com.atlassian.jira.plugin.system.issuetabpanels:comment-tabpanel#comment-25761057s


### Goals

**Default NLB provisioning and managed Security Group for Default Ingress router by CCM**.

Users will be able to deploy OpenShift on AWS with the Default Ingress router, and standalone Service type `LoadBalancer`, with Security Group by default when using Network Load Balancer (NLB) (NLB is still optional through `install-config.yaml`).

Proposed Phases:

**Phase 1: CCM Support managed security group for Service type-LoadBalancer NLB**

- Implement support of cloud provider configuration on CCM to managed Security Group by default when creating resource Service type-LoadBalancer NLB.

**Phase 2: OpenShift defaults to Security Group when Service type-LoadBalancer is NLB**

- OpenShift Cluster Cloud Controller Manager Operator (CCCMO) must enforce cloud-provider configuration on AWS CCM to manage Security Group when Service type-LoadBalancer NLB.
- Ensure the configuration is added for all variants: self-managed, ROSA HCP and Classic.

**Phase 3: CCM support BYO SG Annotation when Service type-LoadBalanacer NLB**

- Introduce Annotations to CCM to allow BYO SG to Service type-LoadBalancer NLB to opt-out the global `Managed` security group configuration.
    - The annotation must follow the same standard as ALBC. Must be optional.
    - An annotation to allow managing backend rules must be added to prevent manual changes by the user. Must be opt-out by default

### Non-Goals

- Migrate to use ALBC as the provisioner on CIO for the default router service (See more in Alternatives).
- Use NLB as the default router deployment - Service type-LoadBalancer.
- Synchronize all NLB features from ALBC to CCM.
- Change the existing CCM flow when deploying NLB without the new configuration.
- Change the default OpenShift IPI install flow when deploying the default router using IPI (users still need to explicitly set the `lbType` configuration to `nlb` to automatically consume this feature).
- Change any ROSA code base, both HCP or Classic, to support this feature.

## Proposal

**Phase 1: CCM Support of Security Group through opt-in in cloud-config**

Introduce a cloud-config global configuration to CCM allowing the CCM to provision managed Security Groups by default every time a new Service type-LoadBalancer is created.

The CCM, the controller which manages the `Service` resource, will have a global configuration on cloud-config to signalize the controller to manage the Security Group by default when creating a Service type-LoadBalancer NLB -  annotation `service.beta.kubernetes.io/aws-load-balancer-type` set to `nlb`. This change paves the path to default the controller to managed security groups, following the same path AWS LBC defaults to since version v2.6.0

The controller must create and manage the lifecycle of the Security Group when the Load Balancer is created, update the SG ingress rules according to the NLB Listeners configurations, and the Egress Rules according to the Target Group configurations.

The SG must be deleted when the resource `Service` is removed.

Users will be able to deploy OpenShift on AWS with the default Ingress using Network Load Balancers with Security Groups when enabled (opt-in) in the `install-config.yaml`.

Goals:
- Cloud Controller Manager (CCM) - Service type-LoadBalancer controller:
  - Enable global configuration (cloud-config) to default managed Security Group when a Service type-LoadBalancer NLB is created - annotation `service.beta.kubernetes.io/aws-load-balancer-type: nlb`.
  - When the configuration is present in the NLB flow, the CCM will:
    - Create a new Security Group instance for the NLB. The name should follow a convention like `k8s-elb-a<generated-name-from-service-uid>`.
    - Create Ingress and Egress rules in the Security Group based on the NLB Listeners' and Target Groups' ports. Egress rules should be restricted to the necessary ports for backend communication (traffic and health check ports).
    - Delete the Security Group when the corresponding service is deleted.
  - Enhance existing tests for the Load Balancer component in the CCM to include scenarios with the new annotation.


**Phase 2: Default OpenShift to use SG when creating Service type-LoadBalancer NLB**

Update CCCMO to update the cloud-config to enforce SG on OpenShift.

TBD

Goals:
- Cluster Cloud Controller Manager Operator (CCCMO):
  - Enforce default configuration to manage security groups on NLB
- Validate TP on OpenShift offerings: self-managed, ROSA Classic, ROSA Managed

**Phase 3: CCM support BYO SG Annotation when Service type-LoadBalanacer NLB**

CCM must support BYO SG annotation to override the global configuration, with annotation naming convention following the same as ALBC.

TBD

Goals:
- CCM - Service type-LoadBalancer controller:
  - Enable (or introduce) annotation BYO SG on NLB provisioning (parity with ALBC)
    - [service.beta.kubernetes.io/aws-load-balancer-security-groups][an-sg]
    - [service.beta.kubernetes.io/aws-load-balancer-manage-backend-security-group-rules][an-be]
  - Enable backend rules management annotation when BYO SG on NLB provisioning (parity with ALBC)
- Validate TP on OpenShift offerings: self-managed, ROSA Classic, ROSA Managed


[an-sg]: https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/service/annotations/#security-groups
[an-be]: https://kubernetes-sigs.github.io/aws-load-balancer-controller/latest/guide/service/annotations/#manage-backend-sg-rules

### Workflow Description

#### Phases 1 and 2: Default to NLB+SG

**OpenShift Self-managed**

- 1.  User:
    - Creates `install-config.yaml` enabling the use of NLB:
```yaml
# install-config.yaml
platform:
  aws:
    region: us-east-1
    lbType: NLB                   <-- enforce CIO to use NLB. Flow already exists.
[...]
```
    - Run `openshift-install create cluster`
- 2.  Installer:
    - Generates the CIO manifests (`IngressController.operator.openshift.io`) with `spec.endpointPublishingStrategy.providerParameters.type: AWS`. (existing flow)
    - Generates the cloud-config. (existing flow)
- 3.  CCCMO:
    - The sync controller transforms the cloud-config enforcing the CCM flag to "Manage Security Groups when NLB"
    - The sync controller creates the cloud-config to the CCM namespace
- 4.  Cluster Ingress Operator (CIO):
    - CIO will create the Service type-LoadBalancer instance for the default router, with the annotations to use NLB. (existing flow)
- 5.  Cloud Controller Manager (CCM):
    - Synchronize resource `Service` type-LoadBalancer with the annotation `service.beta.kubernetes.io/aws-load-balancer-type: nlb`. The CCM will:
        - Create a new AWS Security Group for the NLB. The name should follow a convention like `k8s-elb-a<generated-name-from-service-uid>`.
        - Configure Ingress rules in the Security Group to allow traffic on the ports defined in the Service's `spec.ports`. The source for these rules will be determined by the `service.beta.kubernetes.io/load-balancer-source-ranges` annotation on the Service (if present, otherwise default to allowing from all IPs).
        - Configure Egress rules in the Security Group to allow traffic to the backend pods on the `targetPort` specified in the Service's `spec.ports` and the health check port. Initially, this should be restricted to the cluster's VPC CIDR or the specific CIDRs of the worker nodes.
        - When creating the NLB using the AWS ELBv2 API, the CCM will include the ID of the newly created Security Group in the `SecurityGroups` parameter of the `CreateLoadBalancerInput`.
        - When the Service annotation is added after the Service is created, the CCM will need to (?)
        - When the Service is deleted, the CCM will also delete the associated Security Group, ensuring proper cleanup.
    - Manages the Security Group lifecycle (controllers may exist in CLB).
    - (TBD what happens on updates, examples):
        - If the global config is disabled in day-2, then the Service is deleted, the SG will leak. Do we need to allow that considering we can't detach SGs from NLBs?

**OpenShift Managed**

ROSA Classic and ROSA HCP must inherit CCCMO and CCM defaults to SG.

- 1. User
- 2. `rosa` CLI. ROSA controllers/backend must enable the use of NLB (default flow)
- 3. CCCMO same behavior as self-managed
- 4. CIO same behavior as self-managed
- 5. CCM same behavior as self-managed

#### Phase 3 - BYO SG

TBD

### API Extensions

> WIP/TBReviewed

#### AWS Cloud Controller Manager (CCM)

- Introduction a new global configuration (cloud-config) `Global.NLBSecurityGroupMode` in `pkg/providers/v1/config/config.go`.

```go
const (
    // NLBSecurityGroupModeManaged indicates the controller is managing security groups on service type loadbalancer NLB.
    NLBSecurityGroupModeManaged = "Managed"

    // NLBSecurityGroupModeUnmanaged indicates the controller is not managing security groups on service type loadbalancer NLB.
    NLBSecurityGroupModeUnmanaged = "Unmanaged"
)

type CloudConfig struct {
    Global struct {
        // NLBSecurityGroupMode determines if the controller manage, creates and attaches, the security group when the service type
        // loadbalancer NLB is created.
        NLBSecurityGroupMode string `json:"nlbSecurityGroupMode,omitempty" yaml:"nlbSecurityGroupMode,omitempty"`
    }
```

- Logic in the service controller within the CCM (`pkg/providers/v1/aws.go` and `pkg/providers/v1/aws_loadbalancer.go` ) to recognize and handle the new configuration `Global.NLBSecurityGroupMode` equals to `Managed` when the service type is `nlb` (`ServiceAnnotationLoadBalancerType = "service.beta.kubernetes.io/aws-load-balancer-type"` ).
- Functionality within the CCM to create and manage the lifecycle of AWS Security Groups for NLBs, including creating ingress and egress rules based on the service specification. This would likely involve using the AWS SDK for Go to interact with the EC2 API for creating and managing security groups.

#### OpenShift Cluster Cloud Controller Manager (CCCMO)

- Enforce `NLBSecurityGroupMode` to `Managed` in the cloud-config transformer:
```go
func setOpenShiftDefaults(cfg *awsconfig.CloudConfig) {
	if cfg.Global.NLBSecurityGroupMode != awsconfig.NLBSecurityGroupModeManaged {
		// OpenShift enforces security group by default when deploying
		// service type loadbalancer NLB.
		cfg.Global.NLBSecurityGroupMode = awsconfig.NLBSecurityGroupModeManaged
	}
}
```


#### ROSA Classic

- No changes required as ROSA Classic as CCCMO is global within the cluster, and Classic enables NLB by default in the existing flow.

#### Hypershift/ROSA HCP

- No changes required as ROSA Classic as CCCMO is global within the cluster, and HCP enables NLB by default in the existing flow.

### Topology Considerations

#### Hypershift / Hosted Control Planes

- The flow using self-manage core controllers and defaulting to NLB is already a core piece of HyperShift.
- TODO: we need to figure out if hypershift won't override or change the cloud-config in the lifecycle of the workload cluster.

#### Standalone Clusters

<!-- Is the change relevant for standalone clusters? -->

> TODO/TBD: All changes are proposed initially and exclusively for Standalone clusters.

#### Single-node Deployments or MicroShift

> TODO/TBD


### Implementation Details/Notes/Constraints

> WIP/TBReviewed

- The initial implementation will focus on creating a single Security Group per NLB.
- Egress rules management in CCM needs careful consideration to avoid overly permissive rules. The initial implementation should restrict egress to the necessary ports and protocols for communication with the backend pods (traffic ports and health check ports) within the cluster's VPC.

TODO review the following items:

- The Security Group naming convention should be consistent and informative, including the cluster ID, namespace, and service name to aid in identification and management in the AWS console. (TODO: need review, perhaps we just create the same name as LB?)
- Proper IAM permissions for the CCM's service account will be required to allow it to create, describe, and delete Security Groups in AWS. This needs to be documented as a prerequisite. (TODO review: maybe we don't need this as IAM permissions would be already granted for CLB [review it])


### Risks and Mitigations

> WIP/TBReviewed

- **Increased complexity in CCM**: Adding security group management to CCM increases its complexity. Mitigation: Focus on a minimal and well-tested implementation, drawing inspiration from the existing CLB security group management logic in CCM.
- **Potential for inconsistencies with ALBC**: If users later decide to migrate to ALBC, there might be inconsistencies in how security groups are managed. Mitigation: Clearly document the limitations of this approach and the benefits of using ALBC for more advanced scenarios or a broader range of features.
- **Maintenance burden**: Maintaining this feature in CCM might become challenging if the upstream cloud-provider-aws project evolves significantly in its load balancer management. Mitigation: Upstream the changes to benefit from community maintenance and align with the long-term strategy for load balancer controllers in OpenShift.
- **IAM Permissions(?)**: Incorrectly configured IAM permissions for the CCM could lead to failures in creating or managing security groups. Mitigation: Provide clear documentation on the necessary IAM permissions and potentially include checks in the CCM to verify these permissions.


### Drawbacks

> WIP/TBReviewed

- The short-term solution requires engineering effort to implement and stabilize the changes in CCM and other components.
- The extent of changes in CCM may require significant engineering effort from Red Hat to maintain this functionality. To mitigate it, this feature proposes minimum changes without disrupting the existing flow.
- This approach duplicates some functionality that already exists in the AWS Load Balancer Controller.

## Alternatives (Not Implemented)

> TODO/TBD

### **Defaulting to AWS Load Balancer Controller (ALBC) for the Default Router**:

While ALBC provides more comprehensive support for NLB security groups, this option was deemed out of scope for the initial goal of minimal changes and addressing immediate customer needs for security issues within the existing CCM framework.

Migrating to ALBC would involve significant architectural changes and could potentially impact existing deployments. However, this remains a viable long-term strategy.

Here is an overall effort:
- Determine how ALBC will be managed: Will ALBO be used, or will CCM manage ALBC? Neither ALBO nor ALBC is currently included in the payload. Moving either into the payload requires migrating from CPaaS to Prow and obtaining approval to add a new component to the core payload. When ALBO was created, the guidance was that all new components should be addons, with no new components added to the core payload.
- Migrating from CCM to ALBC would require either disruption to customer workloads (e.g., deleting and recreating the load balancer) or a significant engineering effort to re-architect the way router deployments, load balancers, and DNS are managed. This would involve enabling two ELBs in parallel for the same router deployment, gradually shifting traffic, and then deleting the old ELB.
- Red Hat may need to continue supporting CCM indefinitely for customers unwilling to migrate, meaning this would not be a one-release migration. It would likely require supporting all configurations (CLB with CCM, NLB with CCM, NLB with ALBC) as well as the migration process in perpetuity.
- The above points only scratch the surface of special cases, such as custom security groups, custom DNS configurations, or potential regressions when transitioning from CCM to ALBC.

### **Day-2 operations to switch the default router to use ALBC/LBC**:

This would require users to manually deploy and configure ALBC after cluster installation, which does not meet the requirement for an opt-in during initial cluster deployment.

## Open Questions [optional]

> WIP

- Q: Is it required to create a KEP to the CCM changes?

    A: [No][a1]. But we will need to document the feature in the CCM repo.

- Q: Does CCM require to recreate the NLB when configuration is updated (e.g., `Unmanaged`)?

- Q: Does CCM require to recreate the NLB when configuration is added?

- Q: [Can we reduce the number of rules in the backend SG provided by installer/CAPA][q1]?



[a1]: https://github.com/openshift/enhancements/pull/1802#discussion_r2101097973

[q1]: https://github.com/openshift/enhancements/pull/1802/files#r2112104305

## Test Plan

> WIP/TBReviewed

**cloud-provider-aws (CCM):**:

- e2e service Load Balancer type NLB with Security Groups (SG) needs to be implemented in the CCM component (upstream)
- Implement e2e tests for service Load Balancer type-LoadBalancer NLB updating/disabling cloud-config. These tests should verify:

    - The creation of an NLB with a newly created and associated Security Group.
    - The correct configuration of Ingress rules in the Security Group based on the service ports and loadBalancerSourceRanges.
    - The configuration of Egress rules, ensuring they are appropriately restricted.
    - The deletion of the Security Group when the service is deleted.

## Graduation Criteria

> TODO/TBD

### Dev Preview -> Tech Preview

N/A. This feature will be introduced as Tech Preview (TBReviewed).

### Tech Preview -> GA

The E2E tests should be consistently passing, and a PR will be created to enable the feature gate by default.

### Removing a deprecated feature

> TODO/TBD: depends on the options.

- Announce deprecation and support policy of the existing lbType configuration (if accepted)
- Deprecate the field

## Upgrade / Downgrade Strategy

No upgrade or downgrade concerns are anticipated because all changes are backward-compatible and opt-in. Existing configurations will remain unaffected unless the feature is explicitly enabled.

There is no plan to migrate existing routers with NLB to use Security Groups, as NLBs must be recreated to attach a Security Group. Instead, Day-2 operations must be manually performed, and objects patched if the user wants to consume this feature. For example, users can enable this feature by manually patching the IngressController and associated Service objects to include the required annotations. Note that this may require recreating the NLB, which could result in temporary downtime.

In the case of a downgrade to a CCM version that does not support the new <TBD> cloud-config, the configuration will be ignored, and the Security Group will not be managed by the CCM. Users must manually manage Security Groups in such scenarios.

## Version Skew Strategy

> TODO/TBD

## Operational Aspects of API Extensions

- Monitoring of the CCM for errors related to Security Group creation, rule application, and deletion. Metrics around the number of managed security groups could also be useful.
- Logging in the CCM to track the status of Security Group operations, including creation, updates, and deletions. Include relevant AWS API call details in the logs for debugging.
- Potential for rate limiting on AWS API calls for Security Group management. Implement appropriate backoff and retry mechanisms in the CCM.
- Alerting on failures to create or manage security groups.

## Support Procedures

> TODO/TBD: depends on the options.

Document troubleshooting steps for issues related to NLB Security Group management, including:

- Ensuring the new configuration is applied to the Controller.
- Checking the CCM logs for errors related to Security Group creation, updates, or deletion.
- Verifying the IAM permissions of the CCM's service account to ensure it has the required access.
- Confirming the presence and configuration of the Network Load Balancer and the managed Security Group in the AWS console.
- Providing guidance on common misconfigurations and their resolutions, such as incorrect annotations or missing IAM permissions.

## Infrastructure Needed [optional]

> TODO/TBD: depends on the options.

(Double check: probably those permissions is already granted as CLB support manages SG, keeping it here just to review later)

Additional AWS permissions will be required for the CCM's IAM role to create, modify, and delete Security Groups:

- ec2:CreateSecurityGroup
- ec2:DescribeSecurityGroups
- ec2:DeleteSecurityGroup
- ec2:AuthorizeSecurityGroupIngress
- ec2:AuthorizeSecurityGroupEgress
- ec2:RevokeSecurityGroupIngress
- and ec2:RevokeSecurityGroupEgress

This needs to be clearly documented as a prerequisite for enabling this feature.
