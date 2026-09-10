---
title: hypershift-spot-instances
authors:
  - "@enxebre"
reviewers:
  - "@csrwng"
  - "@muraee"
  - "@sjenning"
  - "@devguyio"
approvers:
  - "@csrwng"
  - "@muraee"
  - "@sjenning"
  - "@devguyio"
api-approvers:
  - "@joelspeed"
creation-date: 2026-03-02
last-updated: 2026-03-02
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1677
see-also:
  - "/enhancements/hypershift/node-lifecycle.md"
---

# AWS Spot Instance Support for Hosted Control Planes

## Summary

This enhancement adds first-class API fields for EC2 Spot instances to HyperShift. A `marketType` field is added to the existing `PlacementOptions` struct on `AWSNodePoolPlatform` (enum: `OnDemand`, `Spot`, `CapacityBlocks`), and a companion `spot` field (`SpotOptions` struct with an optional `maxPrice`) holds spot-specific configuration.

On the HostedCluster side, a `terminationHandlerQueueURL` field is added to `AWSPlatformSpec` to configure the SQS queue for the AWS Node Termination Handler.

When `marketType` is `Spot`, the NodePool controller sets `SpotMarketOptions` on the CAPA `AWSMachineTemplate`, labels machines with `hypershift.openshift.io/interruptible-instance`, tags instances for the NTH, and creates a spot-specific MachineHealthCheck and also a new spot termination controller for rapid replacement of interrupted instances.

## Glossary

- **Spot instance** - An EC2 instance that uses spare compute capacity at up to 90% discount compared to on-demand pricing. AWS can reclaim spot instances with a 2-minute interruption notice delivered via instance metadata and EventBridge/SQS.
- **CAPI** - Cluster API. The Kubernetes sub-project that provides declarative APIs for cluster lifecycle management. HyperShift uses the CAPA provider (Cluster API Provider AWS) to manage worker node infrastructure.
- **CAPA** - Cluster API Provider AWS. The CAPI provider that reconciles `AWSMachineTemplate` resources into EC2 instances via the RunInstances API.
- **MachineHealthCheck (MHC)** - A CAPI resource that monitors machine health and triggers remediation (machine deletion and replacement) when unhealthy conditions persist beyond a configured timeout.
- **NTH** - AWS Node Termination Handler. An open-source controller that polls an SQS queue for EC2 lifecycle events (spot interruption, rebalance recommendation, scheduled maintenance) and cordons/drains affected nodes before termination.
- **Rebalance recommendation** - An AWS signal sent before a spot interruption notice, indicating that a spot instance is at elevated risk of interruption. Provides more lead time than the 2-minute interruption notice.

## Motivation

Spot instances offer 60-90% cost savings compared to on-demand pricing. For workloads that tolerate interruption (batch processing, stateless web services, CI/CD runners, dev/test environments), spot instances dramatically reduce infrastructure costs.

Many HyperShift users run large fleets of hosted clusters where worker node cost is the dominant expense, making spot instance support a high-value feature.

### User Stories

- As a **hosted cluster administrator**, I want to create NodePools that use spot instances on AWS by setting a field in the NodePool spec, so that I can reduce worker node costs for fault-tolerant workloads.

- As a **hosted cluster administrator**, I want to specify a maximum price I am willing to pay for spot instances, so that I can control costs and avoid paying more than my budget allows during price spikes.

- As a **service provider (ROSA)**, I want spot instance configuration to be part of the formal NodePool API with validation, so that I can expose it to customers through my managed service console with proper guardrails.

- As a **hosted cluster administrator**, I want spot instance interruptions to be handled gracefully (cordon, drain, replace) without manual intervention, so that my workloads experience minimal disruption when spot capacity is reclaimed.

- As an **SRE**, I want to monitor the interruption rate and replacement latency of spot instances across hosted clusters, so that I can identify clusters experiencing excessive churn and recommend configuration changes.

### Goals

- Add `Spot` as a value to the existing `MarketType` enum and add a `marketType` field to `PlacementOptions` on `AWSNodePoolPlatform`.
- Add a `SpotOptions` struct with an optional `maxPrice` field, referenced from `PlacementOptions` via a `spot` field.
- Add a `terminationHandlerQueueURL` field to `AWSPlatformSpec` on the HostedCluster to configure the NTH SQS queue as a proper API field.
- Ensure that spot instances are automatically labeled with `hypershift.openshift.io/interruptible-instance` so that workloads can use node affinity and anti-affinity rules to control placement.
- Ensure that a spot-specific MachineHealthCheck is automatically created when `marketType` is `Spot`, with appropriate timeouts for rapid replacement of interrupted instances.
- Tag spot instances with `aws-node-termination-handler/managed` so that the NTH (when deployed) can identify and handle them.

### Non-Goals

- Adding spot support for platforms other than AWS.
- Automatic SQS queue provisioning for the AWS Node Termination Handler. NTH infrastructure setup is a separate concern.
- Supporting AWS Spot Fleet or EC2 Fleet APIs. HyperShift uses CAPI, which manages individual instances through the RunInstances API with spot market options, not fleet-based allocation.
- Adding spot support for control plane nodes. Control plane components require high availability and must not run on interruptible instances.
- Cost optimization features like automatic fallback from spot to on-demand when spot capacity is unavailable. CAPI handles this at the infrastructure level based on the configured market type.

## Proposal

### Architecture Overview

The proposal extends `PlacementOptions` (which already lives on `AWSNodePoolPlatform`) with `marketType` and `spot` fields, and adds `terminationHandlerQueueURL` to `AWSPlatformSpec` on the HostedCluster. The NodePool controller maps these fields to the CAPA `AWSMachineTemplate`. The changes are scoped to:

1. **API types** (`api/hypershift/v1beta1/aws.go`):
   - `MarketType` and `Spot` fields on `PlacementOptions`.
   - New `SpotOptions` struct.
   - `Spot` added to the existing `MarketType` enum.
   - `TerminationHandlerQueueURL` field on `AWSPlatformSpec`.
2. **NodePool controller** (`hypershift-operator/controllers/nodepool/aws.go` and `capi.go`): Reconciliation logic that sets `SpotMarketOptions` on the CAPA machine template, applies the
   `hypershift.openshift.io/interruptible-instance` label, adds the NTH resource tag, and manages the spot MachineHealthCheck. The `interruptible-instance` label is the selector used by both
   the spot MachineHealthCheck and the spot remediation controller to scope their operations exclusively to spot-backed machines.
3. **Control-plane-operator (CPv2)**: Reconciles the NTH Deployment in the HCP namespace. The NTH component is enabled when the HostedControlPlane has `terminationHandlerQueueURL` set on
   its AWS platform spec. The control-plane-operator reads this field during reconciliation and creates the NTH Deployment with the queue URL passed as an environment variable. The NTH image
   (`aws-node-termination-handler` from https://github.com/aws/aws-node-termination-handler) needs to be included in the OCP release payload, ensuring it is available in disconnected
   environments and version-aligned with the rest of the control plane.
4. **Spot remediation controller** (`control-plane-operator/hostedclusterconfigoperator/controllers/spotremediation/`): A new controller in the hosted-cluster-config-operator (HCCO) that
   watches Nodes in the guest cluster for taints with the `aws-node-termination-handler/` prefix (e.g. `spot-itn`, `rebalance-recommendation`). When a tainted node is detected, the controller
   looks up the corresponding CAPI Machine, verifies it carries the `hypershift.openshift.io/interruptible-instance` label, annotates it with
   `hypershift.openshift.io/spot-interruption-signal` for auditability, and deletes the Machine to trigger immediate replacement. This controller bridges the gap between the NTH (which
   operates on guest cluster Nodes) and CAPI (which manages Machines on the management cluster), providing faster replacement than waiting for the MHC timeout. The MHC remains as a fallback
   safety net for cases where the spot remediation controller or NTH is unavailable.
5. **Validation**: CEL rules on `PlacementOptions` that enforce relationships between `marketType`, `spot`, `capacityReservation`, and `tenancy`.

### Workflow Description

**Cluster administrator** is a user responsible for managing HostedClusters and NodePools. Infrastructure setup (SQS queue, EventBridge rules) may be performed by the service provider or the service consumer. See the [NTH infrastructure setup guide](https://github.com/aws/aws-node-termination-handler/tree/9ab67bafaf61c40c9053080e57034c30b88f1f8e#infrastructure-setup) for details.

#### Configuring the HostedCluster for Spot

Before creating spot NodePools, the cluster administrator configures the HostedCluster with an SQS queue URL for the NTH. The SQS queue must be pre-provisioned and configured to receive EC2 Spot Instance Interruption Warnings and EC2 Instance Rebalance Recommendations via EventBridge rules:

```yaml
apiVersion: hypershift.openshift.io/v1beta1
kind: HostedCluster
metadata:
  name: my-cluster
  namespace: clusters
spec:
  platform:
    type: AWS
    aws:
      region: us-east-1
      rolesRef: ...
      terminationHandlerQueueURL: "https://sqs.us-east-1.amazonaws.com/123456789012/my-nth-queue"
```

#### Creating a Spot NodePool

1. The cluster administrator creates a NodePool with `spec.platform.aws.placement.marketType` set to `Spot` and `spec.platform.aws.placement.spot` configured:

   ```yaml
   apiVersion: hypershift.openshift.io/v1beta1
   kind: NodePool
   metadata:
     name: spot-workers
     namespace: clusters
   spec:
     clusterName: my-cluster
     replicas: 5
     release:
       image: quay.io/openshift-release-dev/ocp-release:4.18.0-x86_64
     platform:
       type: AWS
       aws:
         instanceType: m5.xlarge
         placement:
           marketType: Spot
           spot:
             maxPrice: "0.10"
         subnet:
           id: subnet-0123456789abcdef0
     management:
       upgradeType: Replace
   ```

2. The HyperShift operator's NodePool controller reconciles the NodePool:
   a. Sets `SpotMarketOptions` on the `AWSMachineTemplate` spec with the configured `MaxPrice` (or empty for the on-demand price cap).
   b. Labels the MachineDeployment template with `hypershift.openshift.io/interruptible-instance`.
   c. Adds the `aws-node-termination-handler/managed` resource tag to the EC2 instances so the NTH processes events for these instances.
   d. Creates a spot-specific MachineHealthCheck with `maxUnhealthy: 100%` and an 8-minute timeout for `NodeReady` conditions.

3. CAPA creates EC2 spot instances with the specified market options.

4. The NTH (deployed by the control-plane-operator when `terminationHandlerQueueURL` is set on the HostedCluster) watches for spot interruption and rebalance recommendation events, and cordons/drains nodes before termination.
5. The spot remediation controller in the HCCO watches Nodes tainted with the `aws-node-termination-handler/` prefix and triggers Machine deletion for immediate replacement.
6. If `terminationHandlerQueueURL` is not set, the spot MachineHealthCheck provides fallback remediation by detecting `NotReady` Nodes and `Failed` Machines, triggering machine replacement.

#### Interruption Handling

When AWS reclaims a spot instance, the handling depends on whether the NTH is configured:

**With NTH configured (`terminationHandlerQueueURL` set on HostedCluster):**

1. AWS sends a spot interruption notice (or rebalance recommendation) to the SQS queue.
2. The NTH detects the event and cordons the affected node.
3. The NTH drains the node, evicting pods according to PodDisruptionBudgets.
4. AWS terminates the instance after the 2-minute notice period.
5. The spot remediation controller in the HCCO detects the NTH taint on the Node, looks up the corresponding CAPI Machine, and deletes it to trigger immediate replacement.
6. The spot MachineHealthCheck exists as an additional safety net to detect `Failed` Machines or `NotReady` Nodes. It deletes the Machine and the CAPI MachineDeployment controller creates a replacement.

**Without NTH configured:**

1. AWS terminates the spot instance.
2. The Machine becomes `Failed`.
3. The spot MachineHealthCheck triggers remediation.
4. The CAPI MachineDeployment controller creates a replacement machine.

In both cases, the replacement machine is a new spot instance (because the `AWSMachineTemplate` retains the `SpotMarketOptions` configuration). If spot capacity is unavailable for the requested instance type and availability zone, the replacement will remain pending until capacity becomes available.

### API Extensions

#### Changes to `PlacementOptions`

The `marketType` and `spot` fields are added to the existing `PlacementOptions` struct, which already contains `tenancy` and `capacityReservation`. The existing `MarketType` enum (which already has `OnDemand` and `CapacityBlocks` values) gains a `Spot` value.

```go
// PlacementOptions specifies the placement options for the EC2 instances.
//
// The instance market type is determined by the marketType field:
// - "OnDemand" (default): Standard on-demand instances
// - "Spot": Spot instances using spare EC2 capacity at reduced prices
// - "CapacityBlocks": Scheduled pre-purchased compute capacity for ML workloads
//
// +kubebuilder:validation:XValidation:rule="has(self.tenancy) && self.tenancy == 'host' ? !has(self.capacityReservation) : true", message="AWS Capacity Reservations cannot be used with Dedicated Hosts (tenancy 'host')"
// +kubebuilder:validation:XValidation:rule="!has(self.marketType) || self.marketType != 'Spot' || !has(self.capacityReservation)", message="Spot instances cannot be combined with Capacity Reservations"
// +kubebuilder:validation:XValidation:rule="!has(self.marketType) || self.marketType != 'Spot' || !has(self.tenancy) || self.tenancy == '' || self.tenancy == 'default'", message="Spot instances require tenancy 'default' or unset"
// +kubebuilder:validation:XValidation:rule="!has(self.marketType) || self.marketType != 'CapacityBlocks' || has(self.capacityReservation)", message="CapacityBlocks market type requires capacityReservation to be specified"
// +kubebuilder:validation:XValidation:rule="!has(self.spot) || (has(self.marketType) && self.marketType == 'Spot')", message="spot options can only be specified when marketType is 'Spot'"
// +kubebuilder:validation:XValidation:rule="has(self.marketType) && self.marketType == 'Spot' ? has(self.spot) : true", message="spot options must be specified when marketType is 'Spot'"
type PlacementOptions struct {
    // tenancy indicates if instance should run on shared or single-tenant hardware.
    // +optional
    // +kubebuilder:validation:Enum:=default;dedicated;host
    Tenancy string `json:"tenancy,omitempty"`

    // marketType specifies the EC2 instance purchasing model.
    //
    // Possible values:
    // - "OnDemand": Standard on-demand instances (default if unset)
    // - "Spot": Spot instances using spare EC2 capacity at reduced prices but may be interrupted.
    //           Requires spot options and terminationHandlerQueueURL on the HostedCluster.
    // - "CapacityBlocks": Scheduled pre-purchased compute capacity. Recommended for GPU/ML workloads.
    //                     Requires capacityReservation with a specific reservation ID.
    //
    // When omitted, the backend will use "OnDemand" as the default.
    // +optional
    // +kubebuilder:validation:Enum:=OnDemand;Spot;CapacityBlocks
    MarketType MarketType `json:"marketType,omitempty"`

    // spot configures Spot instance options.
    // Required when marketType is "Spot".
    //
    // Spot instances use spare EC2 capacity at reduced prices but may be interrupted
    // with a 2-minute warning. Requires terminationHandlerQueueURL to be set on the
    // HostedCluster's AWS platform spec for graceful handling of interruptions.
    //
    // +optional
    Spot *SpotOptions `json:"spot,omitempty"`

    // capacityReservation specifies Capacity Reservation options for the NodePool instances.
    // ...existing field...
    // +optional
    CapacityReservation *CapacityReservationOptions `json:"capacityReservation,omitempty"`
}

// SpotOptions configures options for Spot instances.
//
// Spot instances use spare EC2 capacity at reduced prices but may be interrupted
// with a 2-minute warning when EC2 needs the capacity back.
type SpotOptions struct {
    // maxPrice defines the maximum price the user is willing to pay for Spot instances.
    // If not specified, the on-demand price is used as the maximum (you pay the actual spot price).
    // The value should be a decimal number representing the price per hour in USD.
    // For example, "0.50" means 50 cents per hour.
    //
    // Note: AWS recommends NOT setting maxPrice to reduce interruption frequency.
    // When omitted, you pay the current Spot price (capped at On-Demand price).
    // AWS minimum allowed value is $0.001.
    //
    // +optional
    // +kubebuilder:validation:Pattern=`^[0-9]+(\.[0-9]+)?$`
    // +kubebuilder:validation:MaxLength=20
    MaxPrice *string `json:"maxPrice,omitempty"`
}

// MarketType describes the market type for EC2 instances.
type MarketType string

const (
    // MarketTypeOnDemand is a MarketType enum value for standard on-demand instances.
    MarketTypeOnDemand MarketType = "OnDemand"

    // MarketTypeCapacityBlock is a MarketType enum value for Capacity Blocks.
    MarketTypeCapacityBlock MarketType = "CapacityBlocks"

    // MarketTypeSpot is a MarketType enum value for Spot instances.
    // Spot instances use spare EC2 capacity at reduced prices but may be interrupted.
    MarketTypeSpot MarketType = "Spot"
)
```

The CEL validations on `PlacementOptions` enforce:

- **Spot cannot combine with Capacity Reservations**: Spot instances are allocated from spare capacity and cannot target a specific reservation.
- **Spot requires default tenancy**: Spot instances cannot run on Dedicated Hosts (`tenancy: host`) or Dedicated Instances (`tenancy: dedicated`).
- **CapacityBlocks requires capacityReservation**: CapacityBlocks is a reservation-based model and must reference a specific reservation ID.
- **`spot` and `marketType: Spot` are co-required**: Setting `marketType: Spot` without `spot` options is rejected, and setting `spot` options without `marketType: Spot` is rejected.

#### New Field on `AWSPlatformSpec` (HostedCluster)

```go
type AWSPlatformSpec struct {
    // ...existing fields...

    // terminationHandlerQueueURL specifies the SQS queue URL for EC2 spot interruption events.
    // This is required when using spot instances (marketType: Spot) in NodePools to enable
    // graceful handling of spot instance terminations.
    //
    // The queue should be configured to receive EC2 Spot Instance Interruption Warnings
    // and EC2 Instance Rebalance Recommendations via EventBridge rules.
    // The AWS Node Termination Handler will poll this queue and cordon/drain nodes
    // before they are terminated, providing a best effort for graceful shutdown.
    //
    // Supports both standard and FIFO queues (FIFO queues end with .fifo suffix).
    //
    // +optional
    // +kubebuilder:validation:Pattern=`^https://sqs\.[a-z0-9-]+\.amazonaws\.com/[0-9]{12}/[a-zA-Z0-9_-]+(\.fifo)?$`
    // +kubebuilder:validation:MaxLength=512
    TerminationHandlerQueueURL *string `json:"terminationHandlerQueueURL,omitempty"`
}
```

The `terminationHandlerQueueURL` field replaces the previous annotation-based mechanism (`hypershift.openshift.io/aws-termination-handler-queue-url`). The field includes a regex pattern validation that enforces the SQS URL format (region, account ID, queue name) and supports both standard and FIFO queues.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is exclusively for the HyperShift topology.

The spot-specific MachineHealthCheck runs in the management cluster's HCP namespace alongside the existing MachineHealthChecks. The NTH (when configured) also runs in the HCP namespace.

#### Standalone Clusters

Not applicable. Standalone OpenShift clusters use MachineSet and MachinePools from the Machine API, which have their own spot instance configuration. Termination handling is managed via IMDS. This proposal introduces the NTH in the payload, which standalone clusters might choose to use in the future.

#### Single-node Deployments or MicroShift

Not applicable. Single-node deployments and MicroShift do not use NodePools.

#### OpenShift Kubernetes Engine

This enhancement does not depend on features excluded from OKE. The NodePool API and CAPI integration are part of the HyperShift operator, which is available in both OCP and OKE.

### Implementation Details/Notes/Constraints

#### NodePool Controller: AWS Spot Detection

The `isSpotEnabled()` function in `hypershift-operator/controllers/nodepool/aws.go` checks the `marketType` field on `PlacementOptions`:

```go
func isSpotEnabled(nodePool *hyperv1.NodePool) bool {
    if nodePool.Spec.Platform.AWS == nil {
        return false
    }
    if nodePool.Spec.Platform.AWS.Placement == nil {
        return false
    }
    return nodePool.Spec.Platform.AWS.Placement.MarketType == hyperv1.MarketTypeSpot
}
```

#### NodePool Controller: CAPA Machine Template Mapping

The `awsMachineTemplateSpec()` function sets `SpotMarketOptions` on the CAPA `AWSMachineTemplate` when `marketType` is `Spot`:

```go
if isSpotEnabled(nodePool) {
    spotOpts := &capiaws.SpotMarketOptions{}
    placement := nodePool.Spec.Platform.AWS.Placement
    if placement.Spot != nil && placement.Spot.MaxPrice != nil &&
       *placement.Spot.MaxPrice != "" {
        spotOpts.MaxPrice = placement.Spot.MaxPrice
    }
    machineTemplate.SpotMarketOptions = spotOpts
}
```

The CAPA `SpotMarketOptions` struct has a single `MaxPrice` field (`*string`). When `MaxPrice` is nil or empty, CAPA passes an empty `SpotMarketOptions` to the EC2 RunInstances API, which uses the on-demand price as the default maximum. This means users pay up to the on-demand price but never more, while still benefiting from the lower spot price when available.

#### NodePool Controller: Resource Tags

The `awsAdditionalTags()` function adds the `aws-node-termination-handler/managed` tag when spot is enabled:

```go
if isSpotEnabled(nodePool) {
    tags["aws-node-termination-handler/managed"] = ""
}
```

This tag allows the NTH to identify which instances it should manage. The NTH filters SQS events to only act on instances that carry this tag.

#### NodePool Controller: Interruptible Instance Label

When spot is enabled, the MachineDeployment template is labeled with `hypershift.openshift.io/interruptible-instance`:

```go
if isSpotEnabled(nodePool) {
    machineDeployment.Spec.Template.Labels["hypershift.openshift.io/interruptible-instance"] = ""
}
```

This label propagates to the CAPI Machine objects and ultimately to the Kubernetes Node objects. Workloads can use `nodeSelector` or node affinity rules to target or avoid spot instances:

```yaml
# Schedule only on spot instances
nodeSelector:
  hypershift.openshift.io/interruptible-instance: ""

# Avoid spot instances
affinity:
  nodeAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      nodeSelectorTerms:
        - matchExpressions:
            - key: hypershift.openshift.io/interruptible-instance
              operator: DoesNotExist
```

#### Spot MachineHealthCheck

When `marketType` is `Spot`, the NodePool controller creates a dedicated MachineHealthCheck named `<nodepool-name>-spot` in the HCP namespace. This MHC is separate from the standard MHC and has settings tuned for spot interruption patterns:

```go
func (c *CAPI) reconcileSpotMachineHealthCheck(mhc *capiv1.MachineHealthCheck) {
    maxUnhealthy := intstr.FromString("100%")
    timeOut := 8 * time.Minute
    nodeStartupTimeout := 20 * time.Minute

    mhc.Spec = capiv1.MachineHealthCheckSpec{
        ClusterName: c.capiClusterName,
        Selector: metav1.LabelSelector{
            MatchLabels: map[string]string{
                "hypershift.openshift.io/interruptible-instance": "",
            },
        },
        UnhealthyConditions: []capiv1.UnhealthyCondition{
            {
                Type:    corev1.NodeReady,
                Status:  corev1.ConditionFalse,
                Timeout: metav1.Duration{Duration: timeOut},
            },
            {
                Type:    corev1.NodeReady,
                Status:  corev1.ConditionUnknown,
                Timeout: metav1.Duration{Duration: timeOut},
            },
        },
        MaxUnhealthy:       &maxUnhealthy,
        NodeStartupTimeout: &metav1.Duration{Duration: nodeStartupTimeout},
    }
}
```

Key configuration choices:

- **`maxUnhealthy: 100%`**: Allows all machines in the spot NodePool to be simultaneously marked unhealthy. This is necessary because a spot reclamation event can affect all instances in the same availability zone and instance type at once. A lower threshold (e.g. 50%) would block remediation during a mass reclamation event.
- **8-minute timeout**: Balances fast replacement (important for maintaining desired replica count) against false positives from transient network issues. The 2-minute AWS interruption notice plus node shutdown time plus detection delay fits within this window.
- **20-minute node startup timeout**: Allows time for the replacement EC2 instance to launch, join the cluster, and become `Ready`. This accounts for potential delays when spot capacity is constrained.

When `marketType` is not `Spot` (or is unset), the controller deletes the spot MHC if it exists.

#### AWS Node Termination Handler Integration

The NTH is deployed as a component in the HCP namespace when the HostedCluster has `terminationHandlerQueueURL` set in `spec.platform.aws`. The NTH deployment is reconciled by the control-plane-operator (CPv2 component framework). The CPv2 predicate checks:

1. The platform type is AWS.
2. `terminationHandlerQueueURL` is set and non-empty.

When both conditions are met, the control-plane-operator creates the NTH Deployment with the queue URL passed as an environment variable. The NTH image (`aws-node-termination-handler`) is included in the OCP release payload as part of this proposal. The Deployment scales replicas down to zero if no NodePools request spot instances.

The NTH operates in SQS queue mode:

1. It polls the configured SQS queue for EC2 lifecycle events.
2. When a spot interruption notice or rebalance recommendation is received for a managed instance (identified by the `aws-node-termination-handler/managed` tag), the NTH:
   a. Cordons the node to prevent new pod scheduling.
   b. Drains the node, evicting pods while respecting PodDisruptionBudgets.
   c. Taints the node with the `aws-node-termination-handler/` prefix (e.g. `aws-node-termination-handler/spot-itn`).
3. The spot remediation controller in the HCCO detects the taint, looks up the corresponding CAPI Machine, and deletes it to trigger immediate replacement by the CAPI MachineDeployment controller.
4. After the drain completes (or the 2-minute notice period expires), AWS terminates the instance.

The NTH is optional. Without it, the spot MHC still provides remediation when the Machine becomes `Failed` after instance termination, but pods are terminated without graceful drain. The NTH adds graceful handling at the cost of requiring SQS queue infrastructure.

#### IAM Permissions for SQS

The NTH deployment uses the NodePoolManagement IAM role credentials (via IRSA/STS) to poll and acknowledge messages from the SQS queue. This requires adding `sqs:ReceiveMessage` and `sqs:DeleteMessage` permissions to the NodePoolManagement role's IAM policy:

```json
{
  "Effect": "Allow",
  "Action": [
    "sqs:DeleteMessage",
    "sqs:ReceiveMessage"
  ],
  "Resource": [
    "*"
  ]
}
```

These permissions are added alongside the existing EC2 permissions in the NodePoolManagement role policy. The `sqs:ReceiveMessage` permission allows the NTH to long-poll the SQS queue for spot interruption and rebalance recommendation events. The `sqs:DeleteMessage` permission allows the NTH to remove processed messages from the queue after acting on them, preventing duplicate processing.

Without these permissions, the NTH deployment will fail to poll the SQS queue and spot interruption events will not be handled gracefully, falling back to the MHC-based remediation (which does not provide graceful drain).

#### Spot Remediation Controller

The spot remediation controller runs in the hosted-cluster-config-operator (HCCO) on the management cluster. It watches Nodes in the guest cluster and reacts to taints applied by the NTH:

1. When a Node receives a taint with the `aws-node-termination-handler/` prefix, the controller reconciles.
2. It resolves the Node to its corresponding CAPI Machine using the `cluster.x-k8s.io/machine` annotation on the Node.
3. It verifies the Machine carries the `hypershift.openshift.io/interruptible-instance` label. Machines without this label are skipped, ensuring the controller only operates on spot-backed machines.
4. It annotates the Machine with `hypershift.openshift.io/spot-interruption-signal` (containing the taint key) for auditability.
5. It deletes the Machine, triggering the CAPI MachineDeployment controller to create a replacement.

This controller provides faster machine replacement than the MHC alone. The MHC waits for the Machine to become `Failed`, which only happens after the instance is already deleted. The spot remediation controller acts immediately upon receiving the NTH taint -- before the instance is actually terminated.

The MHC remains as a fallback for cases where the spot remediation controller or NTH is unavailable.

### Risks and Mitigations

**Risk**: `maxPrice` set below the current spot price silently prevents instance creation.

**Mitigation**: The NodePool status conditions should bubble up CAPA errors, including insufficient capacity. The API documentation warns that a max price below the current spot price will prevent instances from launching.

**Risk**: During correlated spot interruptions (e.g., AWS reclaiming an entire instance type in an AZ), a single SQS queue shared across many NodePools within one HostedCluster could generate a large volume of events simultaneously. While AWS SQS standard queues support nearly unlimited throughput, the NTH deployment processes events with a fixed number of workers (default: 10). For large HostedClusters with many spot NodePools and high replica counts, cordon and drain operations could back up, reducing the effectiveness of the 2-minute interruption notice window and resulting in ungraceful pod terminations.

**Mitigation**: Performance testing should be conducted to determine the practical limits of a single NTH deployment under correlated interruption scenarios across varying numbers of NodePools and replicas. Based on the results, the `WORKERS` environment variable on the NTH Deployment should be tuned or made configurable per-HostedCluster. Additionally, the spot MachineHealthCheck provides a fallback remediation path that does not depend on NTH throughput.

### Drawbacks

- **No graceful drain without NTH**: Without `terminationHandlerQueueURL` configured on the HostedCluster, spot interruptions result in abrupt pod termination. The MHC provides replacement but not graceful drain. This is a limitation of the current NTH architecture (requiring SQS queue provisioning) rather than this enhancement.

## Alternatives (Not Implemented)

### IMDS-based interruption detection instead of SQS

Poll the EC2 Instance Metadata Service (IMDS) endpoint (`http://169.254.169.254/latest/meta-data/spot/instance-action`) from each node to detect spot interruption notices, instead of using an SQS queue.

Rejected because:
- IMDS polling must run on every spot instance as a DaemonSet or sidecar. In HyperShift, worker nodes belong to the guest cluster but interruption handling is a management cluster concern. Running an IMDS poller on guest nodes would require injecting a management-plane component into the data plane with a highly privileged ServiceAccount.
- IMDS only provides a 2-minute warning with no rebalance recommendation signal. SQS receives both spot interruption notices and rebalance recommendations, giving more lead time for graceful drain.

### Shared SQS queue with Karpenter

Use a single SQS queue shared between the NTH and a future Karpenter integration, rather than a dedicated `terminationHandlerQueueURL` field on `AWSPlatformSpec`.

Rejected because:
- Karpenter's SQS queue configuration and lifecycle are managed by the Karpenter controller and may have different EventBridge rule requirements, IAM permissions, and message filtering than the NTH.
- Coupling the NTH queue to Karpenter would create a dependency between two independent features. A cluster that uses spot instances without Karpenter should not need Karpenter infrastructure, and vice versa.
- A dedicated field is explicit and self-documenting. If future integration requires a shared queue, both fields can point to the same URL without API changes.

Relevant links: https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html

https://github.com/aws/aws-node-termination-handler/blob/e24a1e5c926f9477afb8b2412714c4e547dac12c/pkg/monitor/sqsevent/sqs-monitor.go#L283

https://karpenter.sh/docs/faq/#should-i-use-karpenter-interruption-handling-alongside-node-termination-handler

### Top-level `marketType` on `AWSNodePoolPlatform`

Place `marketType` and `spotMaxPrice` directly on `AWSNodePoolPlatform` instead of inside `PlacementOptions`.

Rejected because:
- `PlacementOptions` already exists and groups placement-related configuration (`tenancy`, `capacityReservation`). Market type is a placement concern -- it determines how EC2 allocates capacity for the instance.
- CEL validations need to express relationships between `marketType`, `tenancy`, and `capacityReservation`. Placing all three on the same struct makes the validation rules straightforward and self-contained.
- The existing `MarketType` enum (with `OnDemand` and `CapacityBlocks`) already lives in the context of capacity reservation placement. Extending it with `Spot` is a natural fit.

## Open Questions

This section is intentionally left empty. All design decisions have been resolved.

## Test Plan

- **Unit tests**:
  - Verify that `isSpotEnabled()` correctly detects `placement.marketType: Spot` and returns false for `OnDemand`, `CapacityBlocks`, and nil placement.
  - Verify that `awsMachineTemplateSpec()` sets `SpotMarketOptions` with and without `spot.maxPrice`.
  - Verify that `awsAdditionalTags()` includes the NTH managed tag when spot is enabled and omits it otherwise.
  - Verify CEL validation rules: `spot` rejected without `marketType: Spot`; `marketType: Spot` rejected without `spot`; spot rejected with `capacityReservation`; spot rejected with `tenancy: host` or `tenancy: dedicated`.
  - Verify `terminationHandlerQueueURL` pattern validation accepts valid SQS URLs (standard and FIFO) and rejects invalid ones.

- **Integration tests**:
  - Verify that the NodePool controller creates the spot-specific MachineHealthCheck when `marketType` is `Spot`.
  - Verify that the MachineHealthCheck is deleted when `marketType` is not `Spot`.
  - Verify that the `interruptible-instance` label is set on the MachineDeployment template for spot NodePools and absent for on-demand.
  - Verify that the NTH deployment is created when `terminationHandlerQueueURL` is set and deleted when it is removed.

- **E2E tests**:
  - Create a NodePool with `marketType: Spot` using a mock or the `hypershift.openshift.io/enable-spot` annotation to avoid CI flakes based on spot capacity availability.
  - Verify that the spot MachineHealthCheck is created with the correct selector (`hypershift.openshift.io/interruptible-instance`) and thresholds (`maxUnhealthy: 100%`, 8-minute timeout).
  - Verify that `spot.maxPrice` is passed through to the CAPA machine template.
  - Verify NTH integration: with `terminationHandlerQueueURL` configured, simulate a rebalance recommendation event and verify that the node is cordoned and drained before replacement.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `placement.marketType` and `spot` fields functional end to end.
- `terminationHandlerQueueURL` field functional on HostedCluster.
- Spot-specific MachineHealthCheck automatically created and configured.
- NTH integration working with the new API fields.
- E2E tests passing for spot NodePool creation and interruption handling.
- User-facing documentation for the NodePool and HostedCluster API fields.

### Tech Preview -> GA

- Load testing with large numbers of spot NodePools to validate MachineHealthCheck behavior under mass interruption scenarios.
- Support procedures documented for diagnosing spot-related issues.
- Upgrade/downgrade testing passing.

### Removing a deprecated feature

N/A. This is a new feature with no deprecated predecessors.

## Upgrade / Downgrade Strategy

**Upgrade**: When the HyperShift operator is upgraded to a version that includes the new API fields, existing NodePools are unaffected. The new fields default to nil/unset, which preserves on-demand behavior. No action is required from users.

**Downgrade**: If the HyperShift operator is downgraded to a version that does not include the new API fields, NodePools with `placement.marketType: Spot` will have the field ignored by the older controller. Existing spot instances continue to run, but new instances created by the older controller will be on-demand. No data loss occurs.

## Version Skew Strategy

The new API fields are consumed exclusively by the NodePool controller in the HyperShift operator. There is no version skew concern because the API types and the controller are released together as part of the same operator image.

During an upgrade of the HyperShift operator, the old controller may briefly reconcile NodePools before the new controller takes over. Since the old controller ignores unknown fields, this does not cause issues. The new controller's first reconciliation will apply the correct spot configuration.

Only HostedCluster versions with the control-plane-operator changes for the NTH will benefit from graceful termination handling.

## Operational Aspects of API Extensions

The new fields are optional and default to on-demand behavior. Adding them does not affect existing clusters or NodePools. There are no webhooks, finalizers, or aggregated API servers introduced.

**Impact on existing SLIs**: None. The fields are purely additive and only affect NodePools that explicitly opt in to spot instances.

**Failure modes**:

- If `spot.maxPrice` is set below the current spot price, no instances will launch. The NodePool status will report insufficient capacity. This is a user configuration error, not a system failure.
- If the CAPA version does not support `SpotMarketOptions`, the MachineDeployment will fail to create machines. The NodePool status will reflect the error from CAPA.

## Support Procedures

- **Detecting spot-related issues**:
  - Check NodePool status conditions for machine creation failures.
  - Check the MachineDeployment and Machine objects in the HCP namespace for error messages from CAPA.
  - Check the spot MachineHealthCheck status for remediation counts: `oc get machinehealthcheck <nodepool>-spot -n <hcp-namespace>`.
  - Verify the `hypershift.openshift.io/interruptible-instance` label is present on nodes: `oc get nodes -l hypershift.openshift.io/interruptible-instance`.

- **Diagnosing spot capacity issues**:
  - Verify the instance type and availability zone have spot capacity: `aws ec2 describe-spot-price-history --instance-types <type> --availability-zone <az>`.
  - Check if `spot.maxPrice` is set below the current spot price.
  - Check the EC2 console for spot instance request status and errors.
  - Check CAPA controller logs for RunInstances errors.

- **Diagnosing interruption handling**:
  - Verify NTH is running: `oc get deploy aws-node-termination-handler -n <hcp-namespace>`.
  - Check NTH logs for SQS polling and event processing.
  - Check the spot MHC for machines pending remediation: `oc get machinehealthcheck <nodepool>-spot -n <hcp-namespace> -o yaml`.
  - Verify `terminationHandlerQueueURL` is set on the HostedCluster: `oc get hostedcluster <name> -n clusters -o jsonpath='{.spec.platform.aws.terminationHandlerQueueURL}'`.

- **Impact of spot configuration on cluster operations**:
  - Spot instances may be interrupted during cluster upgrades, causing upgrade retries. The MachineHealthCheck handles replacement, but upgrades may take longer if spot capacity is constrained.
  - The `maxUnhealthy: 100%` setting on the spot MHC means that all machines in a spot NodePool can be simultaneously replaced. This is intentional (spot reclamation can affect all instances) but may cause temporary capacity reduction.
