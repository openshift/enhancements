---
title: lb-subnet-selection-aws
authors:
  - "@gcs278"
reviewers:
  - "@candita"
  - "@frobware"
  - "@rfredette"
  - "@alebedev87"
  - "@miheer"
  - "@JoelSpeed, for review regarding CCM"
approvers:
  - "@Miciah"
api-approvers:
  - "@deads2k"
creation-date: 2024-03-06
last-updated: 2024-08-16
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/NE-705
see-also:
replaces:
superseded-by:
---

# LoadBalancer Subnet Selection for AWS

## Summary

This enhancement extends the IngressController API to allow a cluster admin to
configure specific subnets for AWS load balancers. By default, AWS auto-discovers
the subnets, with logic to break ties when there are multiple subnets for a particular
availability zone. This proposal works by configuring the
`service.beta.kubernetes.io/aws-load-balancer-subnets` annotation on the
LoadBalancer-type service, followed by a deletion of the service at a time when
disruption can be tolerated.

## Motivation

Cluster admins on AWS may have dedicated subnets for their load balancers due to
security reasons, architecture, or infrastructure constraints. 

Currently, cluster admins can configure subnets by manually adding the
`service.beta.kubernetes.io/aws-load-balancer-subnets` annotation to the
service exposing the IngressController. However, this approach does not
allow deploy-time subnet configuration and is unsupported because it is
untested and could interfere with the operations of the Ingress Operator.

Cluster admins need an API for the IngressController which will populate
this annotation simultaneously with the creation of LoadBalancer-type Services,
such as during a cluster launch.

### User Stories

#### Day 2 Load Balancer Subnet Selection on AWS

_"As a cluster admin, when configuring an IngressController in AWS, I want to
specify the subnet selection for the LoadBalancer-type Service on Day 2."_

After a cluster has been installed, the cluster admin wants to specify
a list of subnets for an IngressController's LoadBalancer-type service,
one for each availability zone (AZ).

See [Creating a Custom IngressController with Subnets after Installation Workflow](#creating-a-custom-ingresscontroller-with-subnets-after-installation-workflow)
and [Updating an Existing IngressController with New Subnets Workflow](#updating-an-existing-ingresscontroller-with-new-subnets-workflow)
for the workflows for this user story.

#### Updating from an Unmanaged to a Managed Subnet Annotation

_"As a cluster admin, I want to migrate an IngressController using a LoadBalancer-type
service with an unmanaged service.beta.kubernetes.io/aws-load-balancer-subnets
annotation, to using the new IngressController API field so that the annotation is now
managed by the Ingress Operator."_

A cluster admin manually set the `service.beta.kubernetes.io/aws-load-balancer-subnets`
service annotation, and now wishes to upgrade to an OpenShift version that manages the
annotation. They want to ensure that their load balancer's subnets remain unchanged
during the upgrade, while also intending to migrate the new `Subnets` API field.

See [Unmanaged Subnet Annotation Migration Workflow](#unmanaged-subnet-annotation-migration-workflow)
for the workflow for this user story.

#### Automate Changing an Existing IngressController's Subnets

_"As the provider of a managed service, I want to automate changing
an existing IngressController's subnets, without having to delete
the LoadBalancer-type service."_

A managed service that uses configuration management tooling, such as Hive, should
have the ability to update an existing IngressController's subnets and ensure that
the Ingress Operator effectuates these changes automatically (see [Effectuating Subnet Updates](#effectuating-subnet-updates)
for more context).

See [Automatically Updating an existing IngressController with new Subnets Workflow](#automatically-updating-an-existing-ingresscontroller-with-new-subnets-workflow)
for the workflow for this user story.

### Goals

- Introduce new fields in the IngressController API for subnet selection in AWS.
- Support upgrade compatibility and migration to the new API when using an unmanaged
  `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation.
- Support the new API fields for both Classic Load Balancers (CLB) and Network Load Balancers (NLB).

### Non-Goals

- Extend support to platforms other than AWS.
- Support an install-time (Day 1) configuration for subnet selection of the default IngressController, as this feature
  is anticipated to be supported in another proposal as part of https://issues.redhat.com/browse/CORS-3440.
- Automatically configure subnets for private clusters.
- Extend the feature to ALBs or the AWS Load Balancer Operator (which already has API for subnet configuration).
- Enable automatic updating of the load balancer service subnets by automatically deleting
  the service, which is disruptive.
- Support configuring subnets for Services that aren't associated with an IngressController.
- Use a generic API field for configuring subnets for all types of IngressControllers.

## Proposal

### IngressController API Proposal

The `IngressController` API is extended by adding an optional parameter `Subnets`
of type `[]AWSSubnets` to the `AWSNetworkLoadBalancerParameters` and `AWSClassicLoadBalancerParameters`
structs, to manage the`service.beta.kubernetes.io/aws-load-balancer-subnets` annotation on the
LoadBalancer-type service.

Though the new `Subnets` API fields will be used identically for both CLBs and NLBs, adding it
separately to `AWSNetworkLoadBalancerParameters` and `AWSClassicLoadBalancerParameters`
reduces risk associated with future additions of new load balancer types that may not support
configuring subnets in the same way.

While AWS, GCP, and Azure provide annotations for subnet configuration, AWS's
annotation accepts multiple subnets, whereas GCP and Azure only permit a
single subnet. For this reason, we made this API specific to AWS by adding
the configuration under `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws`.

```go
// AWSNetworkLoadBalancerParameters holds configuration parameters for an
// AWS Network load balancer.
type AWSNetworkLoadBalancerParameters struct {
    // subnets specifies the subnets to which the load balancer will
    // attach. The subnets may be specified by either their
    // ID or name. The total number of subnets is limited to 10.
    //
    // In order for the load balancer to be provisioned with subnets:
    // * Each subnet must exist.
    // * Each subnet must be from a different availability zone.
    // * The load balancer service must be recreated to pick up new values.
    //
    // When omitted from the spec, the subnets will be auto-discovered
    // for each availability zone. Auto-discovered subnets are not
    // propagated in the status of this field.
    //
    // +optional
    // +openshift:enable:FeatureGate=IngressControllerLBSubnetsAWS
    Subnets *AWSSubnets `json:"subnets,omitempty"`
}

// AWSClassicLoadBalancerParameters holds configuration parameters for an
// AWS Classic load balancer.
type AWSClassicLoadBalancerParameters struct {
[...]

    // subnets specifies the subnets to which the load balancer will
    // attach. The subnets may be specified by either their
    // ID or name. The total number of subnets is limited to 10.
    //
    // In order for the load balancer to be provisioned with subnets:
    // * Each subnet must exist.
    // * Each subnet must be from a different availability zone.
    // * The load balancer service must be recreated to pick up new values.
    //
    // When omitted from the spec, the subnets will be auto-discovered
    // for each availability zone. Auto-discovered subnets are not
    // propagated in the status of this field.
    //
    // +optional
    // +openshift:enable:FeatureGate=IngressControllerLBSubnetsAWS
    Subnets *AWSSubnets `json:"subnets,omitempty"`
}

// AWSSubnets contains a list of references to AWS subnets by
// ID or name.
// +kubebuilder:validation:XValidation:rule=`has(self.ids) && has(self.names) ? size(self.ids + self.names) <= 10 : true`,message="the total number of subnets cannot exceed 10"
// +kubebuilder:validation:XValidation:rule=`has(self.ids) || has(self.names)`,message="must specify at least 1 subnet name or id"
type AWSSubnets struct {
    // ids specifies a list of AWS subnets by subnet ID.
    // Subnet IDs must start with "subnet-", consist only
    // of alphanumeric characters, must be exactly 24
    // characters long, must be unique, and the total
    // number of subnets specified by ids and names
    // must not exceed 10.
    //
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="subnet ids cannot contain duplicates"
    // + Note: Though it may seem redundant, MaxItems is necessary to prevent exceeding of the cost budget for the validation rules.
    // +kubebuilder:validation:MaxItems=10
    IDs []AWSSubnetID `json:"ids,omitempty"`

    // names specifies a list of AWS subnets by subnet name.
    // Subnet names must not start with "subnet-", must not
    // include commas, must be under 256 characters in length,
    // must be unique, and the total number of subnets
    // specified by ids and names must not exceed 10.
    //
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:XValidation:rule=`self.all(x, self.exists_one(y, x == y))`,message="subnet names cannot contain duplicates"
    // + Note: Though it may seem redundant, MaxItems is necessary to prevent exceeding of the cost budget for the validation rules.
    // +kubebuilder:validation:MaxItems=10
    Names []AWSSubnetName `json:"names,omitempty"`
}

// AWSSubnetID is a reference to an AWS subnet ID.
// +kubebuilder:validation:MinLength=24
// +kubebuilder:validation:MaxLength=24
// +kubebuilder:validation:Pattern=`^subnet-[0-9A-Za-z]+$`
type AWSSubnetID string

// AWSSubnetName is a reference to an AWS subnet name.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=256
// +kubebuilder:validation:XValidation:rule=`!self.contains(',')`,message="subnet name cannot contain a comma"
// +kubebuilder:validation:XValidation:rule=`!self.startsWith('subnet-')`,message="subnet name cannot start with 'subnet-'"
type AWSSubnetName string
```

The `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation accepts both subnet IDs and subnet names. Subnet
IDs are 24-character alphanumeric AWS resource identifiers starting with `subnet-`. Subnet names are AWS tags with the
key of `name` and the subnet name as the value. By default, AWS tags for subnets do not have character restrictions.
However, this proposed API disallows commas (`,`) as they function as delimiters in the annotation.

Since multiple subnets cannot be from the same Availability Zone (AZ) (see [Invalid Subnet Annotation Values](#invalid-subnet-annotation-values)),
the maximum number of subnets for a load balancer is limited by the maximum number of AZs in an AWS region. At the time
of writing this, the AWS region with the most AZs is the US East (N. Virginia) region, which has 6 AZs. This API
restricts the total number of subnets to a maximum of 10. This allows for future AZ additions without the need to
modify to the API.

#### Effectuating Subnet Updates

If a cluster admin updates a `Subnets` field on an existing IngressController,
the Ingress Operator doesn't effectuate this change automatically. Instead, the
Ingress Operator sets the `LoadBalancerProgressing` condition to `Status: True`
provides a message indicating to the cluster admin that they must delete the service.

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: custom
  namespace: openshift-ingress-operator
spec:
  replicas: 2
  domain: mycluster.com
  endpointPublishingStrategy:
    type: LoadBalancerService
    loadBalancer:
      scope: External
      providerParameters:
        type: AWS
        aws:
          type: Classic
          classicLoadBalancer:
              subnets:
                - subnetA
                - subnetB
                - subnetC
status:
  availableReplicas: 2
  conditions:
  - message: Deployment has minimum availability.
    reason: MinimumReplicasAvailable
    status: "True"
    type: Available
  - message: "The IngressController subnets were changed from [\"\"] to [\"subnetA\", \"subnetB\", \"subnetC\"].
              To effectuate this change, you must delete the service: `oc -n openshift-ingress delete
              svc/custom-default`; the service load-balancer will then be deprovisioned and a new one created.
              This will most likely cause the new load-balancer to have a different host name and IP address and
              cause disruption. To return to the previous state, you can revert the change to the IngressController:
              `oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge
              --patch='{\"spec\":{\"endpointPublishingStrategy\":{\"type\":\"LoadBalancerService\",\"loadBalancer\":
              \"providerParameters\":{\"type\":\AWS\",\"aws\":{\"type\":\"Classic\",\"classicLoadBalancer\":{\"subnets\":[]}}}}}}}'"
    reason: OperandsProgressing
    status: "True"
    type: LoadBalancerProgressing
```

We have intentionally designed this feature to require manual deletion of the
service for the following reasons:

##### Reason #1: To mitigate risks associated with cluster admins providing an invalid annotation value.

While the AWS Cloud Controller Manager (CCM) handles invalid annotations by
producing errors and events, it is difficult for the Ingress Operator to
decipher these CCM events _after_ the load balancer has been provisioned.
Therefore, the Ingress Operator can't indicate to a cluster admin that an
IngressController's load balancer is in a malfunctioning state (i.e., the
service is not getting reconciled) while the service is already provisioned.

When the IngressController is first created and the LoadBalancer-type
service is not yet provisioned, these same invalid `Subnets` values will
prevent the load balancer from being provisioned. The existing Ingress
Operator logic will clearly indicate to the cluster admin that the load balancer
failed to provision via the `LoadBalancerReady` status condition. In addition,
the `LoadBalancerReady` condition will include the CCM event logs that indicate
to the cluster admin that the `Subnets` value is invalid (see [Support Procedures](#support-procedures)
for examples). Therefore, requiring manual service deletion provides a way
for the Ingress Operator to produce a clear signal to the cluster admin
that `Subnets` is invalid.

See [Invalid Subnet Annotation Values](#invalid-subnet-annotation-values) for
examples of invalid subnet annotation values.

##### Reason #2: To mitigate upgrade compatibility issues.

**Note**: Directly configuring the `service.beta.kubernetes.io/aws-load-balancer-subnets`
on the LoadBalancer-type service is not supported and never has
been. However, this enhancement is designed to prevent cluster disruption upon
upgrading with this unsupported configuration.

If the `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation was
previously configured and the cluster is upgraded, the default value `[]` for
`Subnets` will clear the annotation for existing IngressControllers, which
would break cluster ingress. However, requiring the service to be deleted to effectuate
a `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets`
or `...aws.classicLoadBalancer.subnets` value different from the current value of the
annotation prevents this automatic removal of the service annotation on upgrade.

**Note**: Cluster admins upgrading with an unmanaged subnet annotation don't
need to delete the service to propagate the subnet values, instead they can follow the
[Unmanaged Subnet Annotation Migration Workflow](#unmanaged-subnet-annotation-migration-workflow).

##### Reason #3: The CCM Doesn't Reconcile NLB Subnets Updates after creation.

The CCM doesn't reconcile `service.beta.kubernetes.io/aws-load-balancer-subnets`
annotation updates to NLBs once it has been created. However, requiring a service
deletion will effectuate these changes.

See [CCM Doesn't Reconcile NLB Subnets Updates](#ccm-doesnt-reconcile-nlb-subnets-updates)
for more details on why the CCM doesn't reconcile these updates.

##### Auto Effectuation with auto-delete-load-balance Annotation

In addition to deleting the IngressController explicitly, it is possible to use the existing
`ingress.operator.openshift.io/auto-delete-load-balancer` annotation to instruct the Ingress Operator to automatically
delete the service after a subnet update. While this annotation was initially introduced to enable automatic scope
changes in [Ingress Mutable Publishing Scope](/enhancements/ingress/mutable-publishing-scope.md), we've chosen to extend
its usage to support the `Subnets` fields. The auto-delete annotation is not intended for end-users to use directly, but
instead for configuration management tooling.

### Implementation Details/Notes/Constraints

When an IngressController is created with
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets` or
`...aws.neworkLoadBalancer.subnets` specified, the Ingress Operator will set the
`service.beta.kubernetes.io/aws-load-balancer-subnets` annotation
on the LoadBalancer-type Service. The Ingress Operator will update the annotation using the
`Subnets` field from `...aws.classicLoadBalancer.subnets` when the`Type` is `Classic`, and from
`...aws.networkLoadBalancer` when `Type` is `NLB`.

However, if a `Subnets` field is updated on an existing IngressController, the Ingress Operator will NOT
update `service.beta.kubernetes.io/aws-load-balancer-subnets` until the
cluster admin deletes the LoadBalancer-type service as instructed by the
`LoadBalancerProgressing` status (see [Effectuating Subnet Updates](#effectuating-subnet-updates)).
When the Ingress Operator recreates the LoadBalancer-type service, it will
then configure `service.beta.kubernetes.io/aws-load-balancer-subnets` with
the new `Subnets` value. The only exception is if the
`ingress.operator.openshift.io/auto-delete-load-balancer` annotation is set
on the IngressController, in which case the operator automatically deletes
the service and effectuates the subnet update.

The`...aws.neworkLoadBalancer.subnets` and `...aws.classicLoadBalancer.subnets` fields in `status.endpointPublishingStrategy`
will eventually reflect the configured subnet value by mirroring the value of
`service.beta.kubernetes.io/aws-load-balancer-subnets` on the service. There are
two scenarios in which the `Subnets` status won't be equal to the `Subnets` spec:

1. The cluster admin manually configured the `service.beta.kubernetes.io/aws-load-balancer-subnets`
   annotation on the service, which is not supported and likely to cause issues.
2. The cluster admin configured the IngressController's `Subnets` spec, but hasn't
   effectuated the change by deleting the service (see [Effectuating Subnet Updates](#effectuating-subnet-updates)).

This proposal's use of `status.endpointPublishingStrategy` is consistent with the approach in
[LoadBalancer Allowed Source Ranges](/enhancements/ingress/lb-allowed-source-ranges.md),
but diverges with the approach in [Ingress Mutable Publishing Scope](/enhancements/ingress/mutable-publishing-scope.md).
The Ingress Mutable Publishing Scope design sets `status.endpointPublishingStrategy.loadBalancer.scope`
equal to `spec.endpointPublishingStrategy.loadBalancer.scope`, which may not always reflect the actual scope of the
load balancer. This proposal for `Subnets` ensures `status.endpointPublishingStrategy` reflects the _actual_ value
(in our case, the value of `service.beta.kubernetes.io/aws-load-balancer-subnets` on the Service).

#### Changing Subnets with a Load Balancer Type Change

Because subnets are tied to the load balancer type (`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.type`),
changing the load balancer type will cause a `Progressing=True` condition if
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets` and
`spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.networkLoadBalancer.subnets` specify different
subnets. In this case, the Ingress Operator will effectuate the new load balancer type update as usual, but the incoming
subnet change still won't effectuate automatically. The cluster admin will still need to follow the instructions in the
`Progressing` condition by either:

1. Reverting the incoming change by ensuring the subnets for the new load balancer type match the subnets for the
   previous load balancer type (if the same subnets for the new load balancer type are desired)
2. Deleting the service (if different subnets for the new load balancer type are desired).

### Workflow Description

#### Creating a Custom IngressController with Subnets after Installation Workflow

Creating an IngressController while specifying the subnets for the LoadBalancer-type
service after installation:

1. A cluster admin creates an IngressController with
   `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets` or
   `...aws.neworkLoadBalancer.subnets` specified, depending on whether they are using a CLB or NLB.
2. The Ingress Operator creates the LoadBalancer-type Service and sets the
   `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation with the subnets
   provided by the user.

#### Updating an existing IngressController with new Subnets Workflow

Updating an existing IngressController with new subnets requires the service
to be deleted and is disruptive:

1. A cluster admin edits an IngressController's
   `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets` or
   `...aws.neworkLoadBalancer.subnets` specified, depending on whether they are using a CLB or NLB.
2. The Ingress Operator sets the `LoadBalancerProgressing` condition to `Status: True`
   on the IngressController and adds status a message directing the cluster admin
   to delete the service.
3. The cluster admin deletes the LoadBalancer-type service indicated in the
   `LoadBalancerProgressing` condition message.
4. The Ingress Operator recreates the LoadBalancer-type service and configures the
   `service.beta.kubernetes.io/aws-load-balancer-subnets` on the LoadBalancer-type
   service, thus creating the load balancer with the desired subnets specified by the
   cluster admin.

#### Automatically Updating an existing IngressController with new Subnets Workflow

A cluster admin can add the `ingress.operator.openshift.io/auto-delete-load-balancer`
annotation to update the subnets without requiring manual deletion of the service:

1. A cluster admin adds the `ingress.operator.openshift.io/auto-delete-load-balancer`
   annotation to the IngressController.
2. A cluster admin edits an IngressController's
   `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets` or
   `...aws.neworkLoadBalancer.subnets` specified, depending on whether they are using a CLB or NLB.
3. The Ingress Operator automatically deletes and recreates the LoadBalancer-type service
   with `service.beta.kubernetes.io/aws-load-balancer-subnets` configured, thus creating
   the load balancer with the desired subnets specified by the cluster admin.

#### Unmanaged Subnet Annotation Migration Workflow

Migrating an unmanaged `service.beta.kubernetes.io/aws-load-balancer-subnets`
service annotation to using one of the new `Subnets` field in the spec
after upgrading to 4.y doesn't require a cluster admin to delete the LoadBalancer-type
service:

1. Cluster admin confirms that initially `service.beta.kubernetes.io/aws-load-balancer-subnets`
   is set on the LoadBalancer-type service managed by the IngressController.
2. The cluster admin upgrades OpenShift to v4.y and the service annotation is not
   changed or removed.
3. After upgrading, the IngressController will emit a `LoadBalancerProgressing` condition
   with `Status: True` because the spec's `Subnets` (an empty slice `[]`) does not equal
   the current annotation.
4. In this case, the cluster admin must directly update
   `spec.endpointPublishingStrategy.loadBalancer.providerParameters.aws.classicLoadBalancer.subnets` or
   `...aws.neworkLoadBalancer.subnets`, depending on their type of load balancer, to the
   current value of the `service.beta.kubernetes.io/aws-load-balancer-subnets` service.
5. The Ingress Operator will resolve the `LoadBalancerProgressing` condition back to
   `Status: False` as long as `Subnets` is equivalent to `service.beta.kubernetes.io/aws-load-balancer-subnets`.

### API Extensions

This proposal doesn't add any API extensions other than the new `Subnets` fields proposed in
[IngressController API Proposal](#ingresscontroller-api-proposal).

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is directly enabling the https://issues.redhat.com/browse/XCMSTRAT-545
feature for ROSA Hosted Control Planes.

#### Standalone Clusters

This proposal does not have any special implications for standalone clusters.

#### Single-node Deployments or MicroShift

This proposal does not have any special implications for single-node
deployments or MicroShift.

### Risks and Mitigations

#### Invalid Subnet Annotation Values

One type of risk is if the `service.beta.kubernetes.io/aws-load-balancer-subnets`
annotation is configured with invalid subnets. In this situation, the AWS Cloud
Controller Manager (CCM) will throw an error and not reconcile the service. 

This means that if a service is created (or recreated) with an invalid annotation,
the load balancer will not be provisioned. If an existing service has an invalid
annotation added, any future updates to the service will remain unreconciled until
the invalid subnet is fixed.

The following are examples of invalid values that can cause an error in the AWS CCM:

1. Multiple Subnets in Same Availability Zone (AZ)
2. Non-Existent Subnets
3. Replacing All Subnets (disjoint union) in a Classic Load Balancer (CLB)

Since #1 and #2 can occur during service creation, they remain valid risks.
See [Support Procedures](#support-procedures) for how to debug #1 and #2.

However, we have completely mitigated #3 by requiring services to be deleted
upon updating `Subnets` (see [Effectuating Subnet Updates](#effectuating-subnet-updates)).
See the next section, [Replacing All Subnets in a CLB](#replacing-all-subnets-in-a-clb)
for more details on #3.

##### Replacing All Subnets in a CLB

If an update to the list of subnets in `service.beta.kubernetes.io/aws-load-balancer-subnets`
would completely replace the existing subnets on an existing service
using a CLB, the AWS CCM will throw an error and not reconcile the service.
As outlined in this [GitHub comment](https://github.com/kubernetes/cloud-provider-aws/issues/249#issuecomment-906595623),
a new set of subnets causes the AWS controller attempts to detach all
existing subnets on CLBs which is not a supported operation.

A cluster admin wanting to replace all subnets would need to delete and
recreate the LoadBalancer-type service. It may not be obvious that to the cluster
admin that they are replacing all subnets because they must be aware of which
subnets are currently used by the load balancer. For example, if the subnets
were automatically selected subnets they are not listed anywhere.

As mentioned above, to mitigate this we are requiring services to be deleted
upon updating `Subnets` as outlined in [Effectuating Subnet Updates](#effectuating-subnet-updates).

#### CCM Doesn't Reconcile NLB Subnets Updates

As long as the subnets in the annotation are valid, CLBs can update subnets
after creation. However, the same is not true for NLBs. Once a NLB is created
by the CCM, any updates to the `service.beta.kubernetes.io/aws-load-balancer-subnets`
annotation will be validated, but will not be propagated to
the NLB. The [ensureLoadBalancerv2](https://github.com/openshift/cloud-provider-aws/blob/51180c32ceb70298faad1aac1022d8983fdf2e78/pkg/providers/v1/aws_loadbalancer.go#L146)
function responsible for provisioning NLBs lacks the logic to update NLB
subnets after provisioning.

Requiring services to be recreated upon updating the `...aws.networkLoadBalancer.subnets` field
as outlined in [Effectuating Subnet Updates](#effectuating-subnet-updates) will mitigate
cluster admins from experiencing this issue.

### Drawbacks

Requiring the cluster admin to delete the LoadBalancer-type
service leads to several minutes of ingress traffic disruption.

This enhancement brings additional engineering complexity for upgrade
scenarios because cluster admins have previously been allowed to directly
add this annotation on a service. 

Debugging invalid subnets will be confusing for cluster admins and may
lead to extra support cases or bugs. By requiring service to be recreated,
it guarantees that the service won't be provisioned when invalid subnets
are provided, thus clearly indicating to the cluster admin that something
is wrong.

The design in [Effectuating Subnet Updates](#effectuating-subnet-updates)
requires a cluster admin to check the IngressController's status after
updating a `Subnets` field for instructions on how to proceed.

## Open Questions

- Do we need to configure subnets for default IngressController at cluster installation (i.e. Day 1 API)?
  - **Answer**: Yes. Service Delivery needs the ability to pick subnets for the default IngressController during
    installation for ROSA. However, this work is being done as a separate epic (https://issues.redhat.com/browse/CORS-3440),
    which includes redesigning the installconfig fields and deprecating the current `platform.aws.subnets` field.
    We will address an install-time API in a separate enhancement proposal.
- Should we create an alert for when the IngressController is in a `Progressing` status due to the inconsistency
  between subnets?
  - **Answer**: There is interest in exploring this, but this effort will be handled outside the scope of the proposal.

## Test Plan

Unit tests as well as E2E tests will be added to the Ingress Operator
repository.

E2E tests will cover the following scenarios:

- Creating an IngressController with `Subnets` and observing
  `service.beta.kubernetes.io/aws-load-balancer-subnets` on the LoadBalancer-type Service.
- Updating an IngressController with new `Subnets`, deleting the
  LoadBalancer-type service, and observing `service.beta.kubernetes.io/aws-load-balancer-subnets`
  (as described in [Updating an existing IngressController with new Subnets Workflow](#updating-an-existing-ingresscontroller-with-new-subnets-workflow)).
- Directly configuring `service.beta.kubernetes.io/aws-load-balancer-subnets` on the
  LoadBalancer-type service and setting `Subnets` on the IngressController while observing
  `LoadBalancerProgressing` transitioning back to `Status: False` (as described in
  [Unmanaged Subnet Annotation Migration Workflow](#unmanaged-subnet-annotation-migration-workflow)).
- Creating a IngressController with the `ingress.operator.openshift.io/auto-delete-load-balancer`
  annotation, updating an IngressController with new `Subnets`, and observing the service
  get automatically deleted and recreated with `service.beta.kubernetes.io/aws-load-balancer-subnets`
  configured to the new `Subnets`.

## Graduation Criteria

This feature will initially be released as Tech Preview only, behind the
`TechPreviewNoUpgrade` feature gate.

### Dev Preview -> Tech Preview

N/A. This feature will be introduced as Tech Preview.

### Tech Preview -> GA

The E2E tests should be consistently passing and a PR will be created
to enable the feature gate by default.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

Upgrade strategy has been discussed in [Reason #2: To mitigate upgrade compatibility issues](#reason-2-to-mitigate-upgrade-compatibility-issues).
This design supports cluster admins who previously set the `service.beta.kubernetes.io/aws-load-balancer-subnets`
annotation by not automatically impacting IngressController subnet configuration on cluster upgrade. Cluster
admins will have to follow [Unmanaged Subnet Annotation Migration Workflow](#unmanaged-subnet-annotation-migration-workflow)
in order to clear the `LoadBalancerProgressing` status condition message.

Downgrades will also maintain compatibility. Downgrading to 4.(y-1) will preserve
the current value of the `service.beta.kubernetes.io/aws-load-balancer-subnets` annotation.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

### Multiple Subnets in Same AZ Support Procedure

If a cluster admin has provided two subnets in the same Availability Zone (AZ), as
discussed in [Invalid Subnet Annotation Values](#invalid-subnet-annotation-values),
the IngressController with have a status with a relevant error:

```yaml
  - lastTransitionTime: "2024-04-02T22:07:59Z"
    message: |-
      The service-controller component is reporting SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure load balancer: InvalidConfigurationRequest: ELB cannot be attached to multiple subnets in the same AZ.
      The kube-controller-manager logs may contain more details.
    reason: SyncLoadBalancerFailed
    status: "False"
    type: LoadBalancerReady
```

### Non-Existent Subnet Support Procedure

If a cluster admin has provided a subnet that doesn't exist in a `Subnets` field,
as discussed in [Invalid Subnet Annotation Values](#invalid-subnet-annotation-values),
the IngressController with have a status with a relevant error:

```yaml
  - lastTransitionTime: "2024-04-02T22:07:59Z"
    message: |-
      The service-controller component is reporting SyncLoadBalancerFailed events like: Error syncing load balancer: failed to ensure load balancer: expected to find 1, but found 0 subnets
      The kube-controller-manager logs may contain more details.
    reason: SyncLoadBalancerFailed
    status: "False"
    type: LoadBalancerReady
```

## Alternatives

### Automatically Recreating the Load Balancer

One alternative to requiring cluster admins to recreate the LoadBalancer-type
service, as described in [Effectuating Subnet Updates](#effectuating-subnet-updates),
is to have the Ingress Operator automatically delete and recreate the service.

While simpler in design, this approach could result in several minutes of unexpected
disruption to ingress traffic. A cluster admin might not anticipate that updating
the Subnets field could cause disruption, leading to an unwelcome surprise.

### Immutability

Another alternative is to make the `Subnets` fields immutable. This would require a
cluster admin to delete and recreate the IngressController if they wanted to update
the subnets.

The advantage of this approach is that the API Server would prevent any updates
to the `Subnets` fields, clearly indicating to the cluster admin that they would 
need to recreate the IngressController. This is different from our current design,
as described in [Effectuating Subnet Updates](#effectuating-subnet-updates),
which is more subtle. With the current design, a cluster admin would need to check the
IngressController's status after updating `Subnets` for instructions on how to proceed.

However, we didn't use the immutability design because of the following drawbacks:

1. **Not Just a Load Balancer**: The IngressController, which encompasses DNS, HaProxy routers, and more, extends
   beyond just a load balancer. Updating the subnets on the Ingress Controller only needs to propagate to the
   LB-type service, not the other components. Deleting the entire IngressController just to change subnets is excessive,
   considering that only the service itself requires deletion.
2. **Prior Art**: We have an established a precedent in the [Ingress Mutable Publishing Scope](/enhancements/ingress/mutable-publishing-scope.md)
   design, where cluster admins are allowed to mutate an IngressController API field, but must delete the Service to
   effectuate the changes.
3. **Future Possibilities**: Should the AWS CCM eventually supports reliably modifying subnets without deleting the
   LB-type service, we can then easily adapt the Ingress Operator to immediately effectuate the subnets and eliminate
   the need for a cluster admin to delete the LB-type service.

## Infrastructure Needed [optional]

N/A
