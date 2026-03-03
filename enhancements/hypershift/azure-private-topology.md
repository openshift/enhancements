---
title: azure-private-topology
authors:
  - "@bryan-cox"
reviewers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
approvers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
api-approvers:
  - "@csrwng"
creation-date: 2026-02-26
last-updated: 2026-02-26
tracking-link:
  - https://issues.redhat.com/browse/CNTRLPLANE-1985
see-also:
  - "/enhancements/hypershift/self-managed-azure.md"
  - "/enhancements/hypershift/networking/networking.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# Azure Private Topology for Self-Managed HyperShift

## Summary

This enhancement adds private endpoint access support to self-managed Azure
HyperShift clusters using Azure Private Link Service (PLS). Currently,
self-managed Azure clusters only support public endpoint access. This proposal
delivers three endpoint access modes — Public (default), PublicAndPrivate, and
Private — mirroring the private topology support already available on AWS
(via VPC PrivateLink) and GCP (via Private Service Connect).

## Motivation

Enterprise customers running self-managed Azure HyperShift require private
network connectivity to hosted control plane endpoints for security and
compliance. Today, the kube-apiserver (KAS) is exposed only via a public
load balancer, which does not meet the requirements of organizations that
prohibit public endpoint exposure. Azure is the only HyperShift platform
without an `EndpointAccess` configuration — AWS and GCP both support
`Public`, `PublicAndPrivate`, and `Private` modes.

### User Stories

#### Story 1: Security-Conscious Enterprise Customer

As a platform engineer at an enterprise with strict network security policies,
I want to deploy hosted clusters where the API server is accessible only via
Azure Private Link, so that no control plane endpoints are exposed to the
public internet.

#### Story 2: Hybrid Connectivity Deployment

As a platform engineer managing a transition from public to private
infrastructure, I want to deploy hosted clusters with both public and private
endpoints (PublicAndPrivate mode), so that existing integrations can continue
using the public endpoint while new workloads connect privately.

#### Story 3: Operations Team Monitoring Private Clusters

As an operations team member, I want to monitor the health of the Private Link
Service infrastructure (PLS, Private Endpoint, DNS) through standard
Kubernetes conditions on a custom resource, so that I can identify connectivity
issues without accessing the Azure portal.

#### Story 4: CLI-Driven Private Cluster Creation

As a platform engineer, I want to create a private Azure hosted cluster using
the HyperShift CLI with simple flags (`--endpoint-access Private`), so that I
don't have to manually configure Azure networking resources.

### Goals

1. Support three endpoint access modes for self-managed Azure: Public
   (default), PublicAndPrivate, and Private, matching AWS and GCP parity.
   Transitions between PublicAndPrivate and Private are supported after
   creation, consistent with AWS behavior.

2. Automate Azure Private Link Service lifecycle through HyperShift
   controllers, following the same split-controller pattern used by AWS
   (AWSEndpointService) and GCP (GCPPrivateServiceConnect).

3. Provide CLI flags for configuring private topology at cluster creation time.

4. Design the API to accommodate future enhancements (e.g., dedicated OAuth
   private load balancer) and current/future managed services scenarios
   (e.g., ARO HCP) without breaking changes.

### Non-Goals

1. ARO HCP (managed Azure) support — ARO HCP uses Swift for private
   connectivity, which is a different mechanism from the Azure Private Link
   Service approach proposed here. This enhancement only targets
   self-managed Azure.

2. Dedicated OAuth private load balancer — the API is designed to accommodate
   this in the future via the `endpointAccess.private` struct, but it is not
   implemented in this enhancement.

3. Bring-your-own Private Endpoint (BYOPE) — customers creating their own
   Private Endpoints outside of HyperShift management is a future enhancement.

4. Customer-managed Private DNS Zones — DNS zones are created and managed by
   the controllers.

## Proposal

This enhancement extends self-managed Azure HyperShift with private topology
support by adding:

1. **API fields** on `AzurePlatformSpec` — an `endpointAccess` struct that
   groups the access type with its associated private connectivity
   configuration
2. **A new CRD** (`AzurePrivateLinkService`) that coordinates private
   connectivity lifecycle between controllers
3. **Three controllers** following the established AWS/GCP split pattern:
   - CPO Observer: watches the private router Service for the internal LB IP
   - HO Platform Controller: creates the Azure PLS (management-side)
   - CPO Controller: creates the Private Endpoint and DNS (customer-side)
4. **CLI flags** on `hypershift create cluster azure` for endpoint access
   configuration

The `endpointAccess` struct groups the access type discriminator with its
associated private connectivity configuration, making the relationship
explicit in the API. This allows future extension (e.g., OAuth-specific
private connectivity) without API restructuring.

### Workflow Description

**Platform engineer** is a human user responsible for deploying hosted clusters.

**HyperShift Operator (HO)** is the management-cluster controller managing
HostedCluster lifecycle.

**Control Plane Operator (CPO)** is the per-HCP controller managing control
plane components.

1. The platform engineer creates a hosted cluster with private endpoint access:
   ```bash
   hypershift create cluster azure \
     --name my-private-cluster \
     --endpoint-access Private \
     --endpoint-access-private-nat-subnet-id /subscriptions/.../subnets/pls-nat \
     --endpoint-access-private-allowed-subscriptions <subscription-id> \
     # ... other required flags
   ```

2. The HyperShift Operator reconciles the HostedCluster and propagates
   `endpointAccess` (type and private config) to the HostedControlPlane spec.

3. The CPO configures KAS as a Route on the private router. The private
   router's Service is annotated with
   `service.beta.kubernetes.io/azure-load-balancer-internal: "true"`, causing
   Azure to provision an internal load balancer. All private services (KAS,
   OAuth, Konnectivity, Ignition) share this single router LB, requiring
   only one PLS per cluster.

4. The CPO Observer detects the internal LB IP on the private router Service
   and creates an `AzurePrivateLinkService` CR in the HCP namespace,
   populating it with the LB IP, guest network info (VNet ID, subnet ID
   from the NodePool), and private connectivity configuration from the HCP
   spec.

5. The HO Platform Controller sees the new `AzurePrivateLinkService` CR,
   authenticates to Azure using the HO's federated managed identity,
   looks up the internal LB, and creates an Azure Private Link Service
   attached to the LB's frontend IP configuration. It updates the CR status
   with the PLS resource ID and alias.

6. The CPO Controller sees the PLS alias in the CR status and creates:
   - A Private Endpoint in the guest VNet's worker subnet (from the
     `guestSubnetID` populated by the CPO Observer) targeting the PLS
   - A Private DNS Zone with an A record mapping the KAS hostname to the PE's
     private IP
   It updates the CR status with the PE and DNS resource IDs.

7. Guest cluster worker nodes resolve the KAS hostname to the Private
   Endpoint's private IP and communicate with the control plane via:
   `Worker → PE → PLS → Router ILB → Router → KAS pod`

8. The HostedCluster controller aggregates `AzurePrivateLinkService` CR
   conditions into the HostedCluster status, so errors are visible at
   the HC level. The platform engineer can also inspect the CR directly:
   ```bash
   kubectl get azpls -A
   ```

```mermaid
sequenceDiagram
    participant Engineer as Platform Engineer
    participant CLI as HyperShift CLI
    participant HO as HyperShift Operator
    participant CPO as Control Plane Operator
    participant Azure as Azure Cloud
    participant Worker as Guest Worker Nodes

    Engineer->>CLI: create cluster azure --endpoint-access Private
    CLI->>HO: Create HostedCluster CR
    HO->>CPO: Propagate config to HostedControlPlane
    CPO->>Azure: Configure private router Service (internal LB annotation)
    Azure-->>CPO: Router internal LB provisioned with IP
    CPO->>CPO: Observer creates AzurePrivateLinkService CR
    HO->>Azure: Create Private Link Service (on router ILB)
    HO->>HO: Update CR status with PLS alias
    CPO->>Azure: Create Private Endpoint (in guest VNet)
    CPO->>Azure: Create Private DNS Zone + A record
    CPO->>CPO: Update CR status with PE IP
    Worker->>Azure: Resolve KAS hostname → PE private IP
    Worker->>Azure: Connect via PE → PLS → Router ILB → KAS
```

#### Error Handling and Retry Strategy

All three controllers follow the patterns established by the existing AWS
and GCP private connectivity controllers. The specific patterns referenced
below are from the HyperShift codebase at `openshift/hypershift`.

**Rate limiter**: Each controller uses an exponential failure rate limiter
with a 3-second initial backoff and 30-second maximum:
```go
RateLimiter: workqueue.NewTypedItemExponentialFailureRateLimiter[reconcile.Request](
    3*time.Second, 30*time.Second,
)
```
This matches the configuration used by the AWS private link controller
(`control-plane-operator/controllers/awsprivatelink/awsprivatelink_controller.go:329`)
and the GCP PSC controller
(`control-plane-operator/controllers/gcpprivateserviceconnect/psc_endpoint_controller.go:125`).

**Error-specific requeue intervals**: Azure API errors are classified by
HTTP status code to determine appropriate retry timing, following the
pattern established by the GCP PSC controller's `handleGCPError` function
(`psc_endpoint_controller.go:891-941`):
- **Rate limiting (HTTP 429)**: requeue after 5 minutes
- **Permission denied (HTTP 403)**: requeue after 10 minutes (likely
  requires operator intervention)
- **Conflict (HTTP 409)**: requeue after 30 seconds
- **Configuration errors (HTTP 400)**: requeue after 5 minutes with an
  actionable error message
- **Transient errors**: requeue after 2 minutes

The AWS controller does not classify errors by HTTP status code; it uses
the rate limiter's exponential backoff for all failures
(`awsprivatelink_controller.go:468-472`). The GCP approach of classifying
errors is more appropriate for Azure because Azure SDK errors also carry
HTTP status codes.

**Drift detection**: After a successful reconciliation, controllers requeue
after 5 minutes to detect out-of-band changes to Azure resources (e.g., a
PLS deleted via the Azure portal). Both the AWS controller
(`awsprivatelink_controller.go:496-498`, comment: "always requeue to catch
and report out of band changes in AWS") and the GCP controller
(`psc_endpoint_controller.go:55-57`, `driftDetectionRequeueInterval`)
use this same 5-minute interval.

**Idempotent cloud operations**: Before creating any Azure resource, the
controller checks whether it already exists (by name or tag). If it exists,
the controller adopts it by updating the CR status with the existing
resource's IDs. This prevents duplicate resource creation after a failure
between the cloud API call and the status update. The AWS controller uses
this pattern when adopting VPC endpoints
(`awsprivatelink_controller.go:629-657`), and the GCP controller checks
for existing IP addresses and PSC endpoints before creating
(`psc_endpoint_controller.go:587-597`, `671-681`).

**Status-based short-circuit**: Controllers cache Azure resource IDs in the
CR status (e.g., `privateLinkServiceID`, `privateEndpointID`). On
subsequent reconciliations, the controller uses the cached ID to look up
the resource directly rather than performing a list/search operation.

**Condition reporting**: Each condition uses a constant machine-readable
`Reason` (e.g., `AzureError`, `AzureSuccess`) and a human-readable
`Message` that provides actionable context. This follows the pattern used
by both the AWS controller (`AWSErrorReason`/`AWSSuccessReason` at
`awsprivatelink_controller.go:468-485`) and the GCP controller
(`GCPErrorReason`/`GCPSuccessReason` at `psc_endpoint_controller.go:928-934`).
Example condition messages:
- Success: `"Private Link Service is ready"`
- Permission error: `"Azure API permission denied, check HO federated
  managed identity RBAC on management resource group"`
- Configuration error: includes the Azure error message to help diagnose
  NAT subnet or VNet issues

The HostedCluster controller aggregates these CR conditions into
`HostedCluster.Status.Conditions`, following the pattern in
`computeAWSEndpointServiceCondition`
(`hostedcluster_controller.go:3116-3155`). This ensures errors are
visible at the HC level without requiring operators to inspect the CR
directly.

**Downstream gating**: Controllers wait for upstream conditions before
proceeding. The CPO Controller will not create a PE until
`AzurePLSCreated=True` and the PLS alias is populated in the CR status.
If the upstream condition is `False`, the controller returns without
requeue and relies on a watch event when the CR status is updated.

#### Deletion

When the HostedCluster is deleted:
- The `AzurePrivateLinkService` CR is garbage collected via owner references
  to the HostedControlPlane
- Each controller uses finalizers to clean up its Azure resources in reverse
  order: DNS → PE → PLS
- The router's internal LB is cleaned up automatically when the router Service
  is deleted

### API Extensions

This enhancement introduces the following API changes:

#### New field on AzurePlatformSpec

One new field on `AzurePlatformSpec` in the `hypershift.openshift.io`
API group:

- `endpointAccess` (`*AzureEndpointAccessSpec`): Controls visibility and
  private connectivity configuration of the KAS endpoint. Transitions
  between `PublicAndPrivate` and `Private` are supported after creation,
  consistent with AWS behavior. Contains:
  - `type` (`AzureEndpointAccessType`): Access type enum — `Public` (default),
    `PublicAndPrivate`, `Private`.
  - `private` (`*AzurePrivateConnectivityConfig`): Configures Azure Private
    Link Service parameters. Required when `type` is not `Public`. Contains
    `natSubnetID` (subnet for PLS NAT) and `allowedSubscriptions`
    (subscription IDs permitted to create PEs).

This structure groups the access type discriminator with its associated
private connectivity configuration, making the relationship self-documenting.
While AWS and GCP use a bare string for `endpointAccess`, Azure requires
additional configuration for private networking that those platforms do not,
so a struct is the appropriate design.

CEL validation ensures `private` is required when `type != Public`.

#### HO-level Azure Identity for Private Link Service

The HyperShift Operator pod requires a federated managed identity with
Network Contributor RBAC on the management cluster's resource group to
create and manage Azure PLS resources. This identity is configured at the
HO deployment level (via Azure Workload Identity annotations on the HO
service account), not per-HostedCluster.

This follows the same pattern as AWS and GCP:
- AWS: the HO uses `AWS_SHARED_CREDENTIALS_FILE` and `AWS_REGION` env vars
  set on the operator deployment
  (`hypershift-operator/controllers/platform/aws/controller.go:109-113`)
- GCP: the HO uses `GCP_PROJECT` and `GCP_REGION` env vars with application
  default credentials
  (`hypershift-operator/controllers/platform/gcp/privateserviceconnect_controller.go:55-73`)

The identity is only required when the HO manages clusters with non-Public
endpoint access.

#### New CRD: AzurePrivateLinkService

A namespaced custom resource in the `hypershift.openshift.io` API group:

- **Spec fields**: `loadBalancerIP`, `subscriptionID`, `resourceGroupName`,
  `location`, `natSubnetID`, `allowedSubscriptions`, `guestSubnetID`,
  `guestVNetID`
- **Status fields**: `conditions`, `internalLoadBalancerID`,
  `privateLinkServiceID`, `privateLinkServiceAlias`, `privateEndpointID`,
  `privateEndpointIP`, `privateDNSZoneID`
- **Conditions**: `AzureInternalLoadBalancerAvailable`,
  `AzurePLSCreated`, `AzurePrivateEndpointAvailable`,
  `AzurePrivateDNSAvailable`, `AzurePrivateLinkServiceAvailable`

This CRD follows the same coordination pattern as `AWSEndpointService` and
`GCPPrivateServiceConnect`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specifically designed for the HyperShift topology. It
affects:

- **Management cluster**: The HO platform controller runs here and creates
  Azure PLS resources in the management cluster's VNet. The HO pod's
  federated managed identity (configured at operator installation) must
  have Network Contributor RBAC on the management resource group.
- **HCP namespace**: The CPO observer and controller run here, creating the
  `AzurePrivateLinkService` CR and customer-side Azure resources (PE, DNS).
- **Guest cluster**: Worker nodes use Private DNS to resolve the KAS hostname
  to the PE private IP. No changes to guest cluster components.

The existing ARO HCP code paths are not modified. The private topology
controllers are only registered when the platform is Azure and `IsPrivateHCP()`
returns true, which only matches self-managed Azure clusters with non-Public
endpoint access.

#### Standalone Clusters

Not applicable. This enhancement is specific to the HyperShift topology.

#### Single-node Deployments or MicroShift

Not applicable. This enhancement does not affect SNO or MicroShift
deployments.

#### OpenShift Kubernetes Engine

Not applicable. OKE does not support HyperShift hosted control planes.

### Implementation Details/Notes/Constraints

#### Architecture Overview

The following diagram shows the Azure networking architecture for a private
hosted cluster. The management cluster VNet contains the private router's
internal load balancer and Private Link Service. The guest VNet contains the
Private Endpoint and worker nodes. Private DNS resolves the KAS hostname to
the PE's private IP.

KAS is exposed via a Route on the private router, so all private services
(KAS, OAuth, Konnectivity, Ignition) share a single router LB and PLS.

The **data flow** direction is: Worker → PE → PLS → Router ILB → Router →
KAS. The NAT subnet is not a hop in the data path — it is a subnet from
which Azure PLS allocates source IPs for SNAT. When traffic arrives from a
PE, Azure rewrites the source IP to one from the NAT subnet range before
forwarding to the ILB, hiding the PE's original IP from the backend.

```mermaid
graph TB
    subgraph "Management Cluster VNet"
        subgraph "HCP Namespace"
            KAS[KAS Pod]
            Router[Private Router]
        end
        ILB[Router Internal Load Balancer]
        PLS[Azure Private Link Service]
        NAT[NAT Subnet<br/>used by PLS for source IP translation]

        PLS -.->|allocates SNAT IPs from| NAT
        PLS -->|forwards traffic to| ILB
        ILB -->|routes to| Router
        Router -->|Route passthrough| KAS
    end

    subgraph "Guest Cluster VNet"
        PE[Private Endpoint]
        DNS[Private DNS Zone<br/>api.cluster.example.com → PE IP]
        subgraph "Worker Subnet"
            W1[Worker Node 1]
            W2[Worker Node 2]
        end

        W1 -->|1. resolve KAS hostname| DNS
        W2 -->|1. resolve KAS hostname| DNS
        DNS -.->|returns PE private IP| W1
        DNS -.->|returns PE private IP| W2
        W1 -->|2. connect to PE IP| PE
        W2 -->|2. connect to PE IP| PE
    end

    PE -->|3. Azure Private Link tunnel| PLS

    style PLS fill:#4a9,stroke:#333
    style PE fill:#4a9,stroke:#333
    style ILB fill:#69c,stroke:#333
    style DNS fill:#fc6,stroke:#333
    style NAT fill:#ddd,stroke:#999
```

The following diagram shows the controller coordination pattern. Three
controllers interact via the `AzurePrivateLinkService` CRD to manage the
lifecycle of Azure resources:

```mermaid
graph LR
    subgraph "Control Plane Operator (per HCP)"
        OBS[CPO Observer<br/>watches Private Router Service]
        CTRL[CPO Controller<br/>creates PE + DNS]
    end

    subgraph "HyperShift Operator (singleton)"
        HO[HO Platform Controller<br/>creates Azure PLS]
    end

    subgraph "Kubernetes Resources"
        SVC[Private Router Service<br/>type: LoadBalancer<br/>internal annotation]
        CR[AzurePrivateLinkService CR<br/>spec + status]
    end

    subgraph "Azure Resources"
        APLSS[Private Link Service]
        APES[Private Endpoint]
        ADNS[Private DNS Zone]
    end

    SVC -->|ILB IP assigned| OBS
    OBS -->|creates CR with LB IP| CR
    CR -->|watches CR| HO
    HO -->|creates| APLSS
    HO -->|updates status:<br/>PLS ID + alias| CR
    CR -->|watches status| CTRL
    CTRL -->|creates| APES
    CTRL -->|creates| ADNS
    CTRL -->|updates status:<br/>PE IP + DNS zone| CR

    style CR fill:#ff9,stroke:#333
    style APLSS fill:#4a9,stroke:#333
    style APES fill:#4a9,stroke:#333
    style ADNS fill:#fc6,stroke:#333
```

#### Controller Architecture

The implementation follows the established split-controller pattern used by
AWS and GCP:

| Component | Location | Watches | Creates (Azure) | Auth |
| --------- | -------- | ------- | --------------- | ---- |
| CPO Observer | `control-plane-operator/controllers/azureprivatelinkservice/observer.go` | Private Router Service | AzurePrivateLinkService CR | N/A (K8s only) |
| HO Platform Controller | `hypershift-operator/controllers/platform/azure/controller.go` | AzurePrivateLinkService CR | Private Link Service | HO-level federated managed identity |
| CPO Controller | `control-plane-operator/controllers/azureprivatelinkservice/controller.go` | AzurePrivateLinkService CR | Private Endpoint + DNS | Existing CPO workload identity |

This split provides least-privilege security:
- The HO identity can only create PLS in the management cluster's resource
  group
- The CPO identity can only create PE and DNS in the guest subscription
- If either credential is compromised, the blast radius is contained

#### Comparison to AWS and GCP

| Aspect | AWS | GCP | Azure (this EP) |
| ------ | --- | --- | --------------- |
| CRD | AWSEndpointService | GCPPrivateServiceConnect | AzurePrivateLinkService |
| Management-side resource | VPC Endpoint Service | Service Attachment | Private Link Service |
| Customer-side resource | VPC Endpoint + SG | PSC Endpoint (Forwarding Rule) | Private Endpoint |
| DNS | Route53 Private Zone | Cloud DNS | Private DNS Zone |
| HO creates | VPC Endpoint Service | Service Attachment | PLS |
| CPO creates | VPC Endpoint + SG + DNS | PSC Endpoint + IP + DNS | PE + DNS |

#### API Design: endpointAccess Struct

The `endpointAccess` field is a struct that groups the access type discriminator
with its associated private connectivity configuration. This makes the
relationship between the access type and private config self-documenting,
rather than relying on CEL validation alone to enforce it. While AWS and GCP
use `endpointAccess` as a bare string (since those platforms require no
additional private networking configuration), Azure has platform-specific
specs and is not bound by those conventions.

```go
type AzureEndpointAccessSpec struct {
    Type    AzureEndpointAccessType      `json:"type"`
    Private *AzurePrivateConnectivityConfig `json:"private,omitempty"`
}

type AzurePrivateConnectivityConfig struct {
    NATSubnetID          string   `json:"natSubnetID"`
    AllowedSubscriptions []string `json:"allowedSubscriptions"`
    // Future: OAuthLoadBalancer *AzureOAuthLBConfig `json:"oauthLoadBalancer,omitempty"`
}
```

#### KAS via Route and Private Router

KAS is exposed via a Route on the private router rather than via a dedicated
LoadBalancer Service. The private router's Service is annotated with
`service.beta.kubernetes.io/azure-load-balancer-internal: "true"`, and Azure's
cloud provider creates an internal LB for it. All private services (KAS,
OAuth, Konnectivity, Ignition) share this single router LB, so only one PLS
per cluster is needed.

When `endpointAccess.type` is `Private`, the KAS Service remains ClusterIP
(no public LB). All external access goes through the private router's
internal LB → PLS → PE path.

When `endpointAccess.type` is `PublicAndPrivate`, the public KAS endpoint
is preserved alongside the private router path. The implementation follows
the same pattern as AWS `PublicAndPrivate`.

#### NAT Subnet Requirements

The PLS NAT subnet must:
- Exist in the management cluster's VNet
- Have `privateLinkServiceNetworkPolicies` set to `Disabled`
- Be dedicated to PLS NAT (not shared with other workloads)

The customer provides this subnet ID via the
`--endpoint-access-private-nat-subnet-id` CLI flag or the
`endpointAccess.private.natSubnetID` API field.

### Risks and Mitigations

**Risk**: Azure PLS has a limit of 1000 PLS resources per subscription and
region.

**Mitigation**: This is sufficient for most self-managed deployments. Document
the limit and recommend separate subscriptions for very large deployments.

---

**Risk**: The HO workload identity has network-level access to the management
cluster's VNet.

**Mitigation**: The identity is scoped to Network Contributor on the management
resource group only. The CPO identity is separately scoped to the guest
subscription. This follows the same least-privilege pattern as AWS/GCP.

---

**Risk**: NAT subnet misconfiguration (e.g., `privateLinkServiceNetworkPolicies`
not disabled) will cause PLS creation to fail.

**Mitigation**: The controller reports clear error conditions on the
`AzurePrivateLinkService` CR. CLI validation can check subnet configuration
before cluster creation. Documentation covers subnet requirements.

---

**Risk**: Breaking existing ARO HCP code paths.

**Mitigation**: All new controllers are gated on `IsPrivateHCP()` which only
returns true for self-managed Azure with non-Public endpoint access. ARO HCP
uses Swift for private connectivity and is unaffected by these changes.

### Drawbacks

1. **Additional Azure SDK surface**: The controllers introduce new Azure SDK
   dependencies (armnetwork, armprivatedns). This increases the vendor
   footprint and the surface area for Azure API breaking changes.

2. **New workload identity requirement**: The HO deployment must be configured
   with a federated managed identity that has Network Contributor RBAC on
   the management resource group. This is a one-time setup at operator
   installation, not per-cluster, but adds complexity to the infrastructure
   prerequisites.

3. **New CRD**: The `AzurePrivateLinkService` CRD adds one more resource type
   to the HyperShift API surface. However, this follows the established
   pattern (AWS has `AWSEndpointService`, GCP has `GCPPrivateServiceConnect`).

## Alternatives (Not Implemented)

### Alternative 1: Flat Topology Enum (Option A)

Instead of the `endpointAccess` struct with `type` and `private` sub-fields,
define all permutations as a single enum:

```go
// Example: publicAndPrivateLink, privateLinkAndOAuth, etc.
EndpointTopology AzureEndpointTopology
```

**Rejected because**: Combinatorial explosion as features are added (OAuth LB,
shared ingress, Swift permutations). The struct approach chosen by this
enhancement — grouping `type` (visibility) with `private` (mechanism config)
under `endpointAccess` — is more composable and maintainable. New connectivity
methods can be added as sub-fields without creating new topology enum values
for every combination.

### Alternative 2: CPO-Only Controllers (No HO Involvement)

Since the CPO already has Azure credentials, it could create both the PLS
(management-side) and PE (customer-side), eliminating the need for a new HO
workload identity.

**Rejected because**: This violates least-privilege. The CPO identity would need
access to both management and guest Azure resources. The split HO/CPO pattern
provides defense in depth and is consistent with AWS/GCP.

### Alternative 3: CLI-Driven Resource Creation

Instead of controllers creating Azure resources, the CLI or customer would
create PLS, PE, and DNS manually or via `hypershift create infra azure`.

**Rejected because**: AWS and GCP both use controller-driven resource creation
for private connectivity. Controller-driven creation is declarative — the user
sets `endpointAccess.type: Private` and the controllers handle the lifecycle of all
cloud resources, including cleanup on deletion. CLI-driven creation requires
the user to manually sequence resource creation (wait for ILB, then create PLS,
then create PE, then create DNS) and manually clean up resources on cluster
teardown, which is error-prone and inconsistent with the existing platforms.

### Alternative 4: Swift Networking (VNet Injection)

ARO HCP uses Swift networking — Azure's CNI infrastructure that injects
pods directly into customer-delegated subnets. With Swift, the control
plane pods are placed into the customer's VNet context, so private
connectivity is handled at the networking layer itself without needing
PLS or Private Endpoints.

**Not used for self-managed because**: Swift is an Azure-managed, first-party
infrastructure feature tied to AKS. It requires Azure's orchestration layer
to delegate subnets and inject pods into customer VNets. Self-managed
HyperShift runs on standard OpenShift clusters, not AKS, so Swift's
VNet injection infrastructure is not available. PLS is the standard Azure
mechanism for cross-VNet private connectivity available to any Azure
customer regardless of the underlying cluster platform.

## Open Questions [optional]

1. **PublicAndPrivate dual-path pattern**: The exact mechanism for maintaining
   both a public KAS endpoint and the private router path needs to be verified
   against the AWS `PublicAndPrivate` implementation. With the Route-based
   approach, the private path goes through the router's internal LB while the
   public KAS endpoint may remain via its existing public LB.

2. **PE subnet**: The current design places the PE in the guest VNet's subnet
   (same as worker nodes). Some deployments may want the PE in a dedicated
   subnet. This can be addressed in a follow-up by adding an optional
   `privateEndpointSubnetID` to `endpointAccess.private`.

3. **Cross-cloud management cluster**: The parent self-managed Azure EP states
   the management cluster can run on Azure or AWS. Private topology requires
   the PLS to be created in the management cluster's Azure VNet. If the
   management cluster is on AWS, there is no Azure VNet. Private topology
   should be constrained to Azure management clusters only, or an alternative
   architecture is needed for cross-cloud scenarios.

## Test Plan

### Unit Tests

- **API validation tests**: CEL rules for `endpointAccess.private` requirement,
  valid transitions between endpoint access types
- **HO Platform Controller tests**: PLS creation, status updates, cleanup,
  idempotency, owner references (using Azure SDK interface fakes)
- **CPO Observer tests**: CR creation on router Service ILB IP detection,
  skip on Public endpoint access, skip on missing ILB annotation
- **CPO Controller tests**: PE creation, DNS zone + A record creation, status
  updates, cleanup, requeue on missing PLS alias
- **Visibility helper tests**: `IsPrivateHCP`, `IsPublicHCP`, `IsPrivateHC`
  for Azure endpoint access types
- **CLI validation tests**: Flag parsing, validation of required flags for
  non-Public endpoint access

### E2E Tests

- **TestAzurePrivateTopology**: Full lifecycle — create HC with
  `endpointAccess.type: Private`, verify `AzurePrivateLinkService` CR progresses
  through all conditions, verify KAS is reachable via PE private IP, verify
  cluster reaches `Available` state.

E2E requires a pre-provisioned NAT subnet with
`privateLinkServiceNetworkPolicies` disabled in the CI Azure VNet.

## Graduation Criteria

This feature follows the graduation path of the parent self-managed Azure
enhancement (OCPSTRAT-1915 / OCPSTRAT-2252 / OCPSTRAT-2253).

### Dev Preview -> Tech Preview

- End-to-end private cluster lifecycle works (create, upgrade, delete)
- Unit tests cover all controller logic with >80% coverage
- E2E test runs in CI
- Documentation covers private cluster creation workflow and prerequisites

### Tech Preview -> GA

- Sufficient time for customer feedback (at least one minor release)
- Scale testing completed (multiple private clusters per management cluster)
- Upgrade and downgrade scenarios validated
- User-facing documentation in openshift-docs
- Support procedures documented

### Removing a deprecated feature

N/A — This is a new feature.

## Upgrade / Downgrade Strategy

**Upgrade**: Existing public self-managed Azure clusters are unaffected. The
`endpointAccess.type` field defaults to `Public` when not set. New clusters can
opt in to private topology at creation time. Existing clusters can transition
between `PublicAndPrivate` and `Private` after creation, consistent with AWS
behavior.

Before upgrading the HyperShift operator to a version that supports private
topology, customers who intend to use it must:
1. Configure the HO deployment with a federated managed identity that has
   Network Contributor RBAC on the management cluster's resource group.
2. Provision a NAT subnet in the management cluster's VNet with
   `privateLinkServiceNetworkPolicies` disabled.

These prerequisites are only required if the customer plans to create private
clusters after the upgrade. Existing public clusters are unaffected and do not
require new identities or subnets.

**Downgrade**: If a management cluster is downgraded to a version that does
not support the `AzurePrivateLinkService` CRD, the CRD and its instances
will remain in the cluster but be unreconciled. The Azure PLS, PE, and DNS
resources will continue to exist in Azure but will not be actively managed.
The customer would need to manually clean up Azure resources if they want to
fully remove the private topology infrastructure.

## Version Skew Strategy

The `AzurePrivateLinkService` CRD is reconciled by both the HO (management
cluster) and CPO (per-HCP). During upgrades:

- The HO is upgraded first (as part of the HyperShift operator upgrade)
- The CPO is upgraded per-HCP (as each hosted cluster is upgraded)

During the window where the HO is upgraded but CPO is not, the CPO Observer
may create `AzurePrivateLinkService` CRs that the upgraded HO can reconcile.
The CPO Controller (old version) will not have the PE/DNS logic, but this is
acceptable because new private clusters should only be created after both
components are upgraded.

Existing public clusters are unaffected by version skew as the private
topology controllers are gated on `IsPrivateHCP()`.

## Operational Aspects of API Extensions

### AzurePrivateLinkService CRD

- **Expected scale**: One `AzurePrivateLinkService` CR per private hosted
  cluster. For most deployments, fewer than 100 instances.
- **Impact on existing SLIs**: Minimal. The CRD adds one watch per controller
  (HO, CPO Observer, CPO Controller). No impact on API throughput for
  unrelated resources.
- **Failure modes**:
  - If the HO Platform Controller fails, PLS is not created. The
    `AzurePLSCreated` condition will be `False`. The hosted cluster will not
    reach `Available` because workers cannot connect to the KAS.
  - If the CPO Controller fails, PE and DNS are not created. The
    `AzurePrivateEndpointAvailable` condition will be `False`. Same impact.
  - Both controllers retry with exponential backoff. Transient Azure API
    failures are handled automatically.
- **Health indicators**:
  - `AzurePrivateLinkService` CR conditions
  - HO controller logs (`controllers.AzurePrivateLinkService`)
  - CPO controller logs (`controllers.AzurePrivateLinkServiceEndpoint`)
  - HostedCluster `Available` condition

### HO Workload Identity

The HO's federated managed identity for PLS operations is configured at the
operator deployment level, not per-cluster. It is only required when the HO
manages clusters with non-Public endpoint access. It does not affect existing
clusters or identities.

## Support Procedures

### Detecting Issues

1. **Private cluster not reaching Available**:
   ```bash
   kubectl get azpls -A -o wide
   ```
   Check which condition is `False`:
   - `AzureInternalLoadBalancerAvailable=False`: ILB not provisioned. Check
     private router Service annotation and Azure cloud provider logs.
   - `AzurePLSCreated=False`: PLS creation failed. Check HO logs and the
     HO's federated managed identity RBAC.
   - `AzurePrivateEndpointAvailable=False`: PE creation failed. Check CPO logs
     and guest subscription permissions.
   - `AzurePrivateDNSAvailable=False`: DNS creation failed. Check CPO logs.

2. **Workers cannot reach KAS**:
   - Verify PE is in `Approved` state in Azure portal
   - Verify Private DNS Zone has correct A record
   - Verify VNet link is active for the guest VNet
   - Check NSG rules allow traffic to PE subnet

3. **PLS creation fails with permission error**:
   - Verify the HO's federated managed identity has Network Contributor
     on the management resource group
   - Verify federated credential is configured for the HO service account

### Remediation

1. **Stale Azure resources after failed deletion**: If the
   `AzurePrivateLinkService` CR finalizer fails, manually delete Azure
   resources (PLS, PE, DNS zone) via Azure CLI or portal, then remove the
   finalizer from the CR.

2. **NAT subnet misconfiguration**: Check that the NAT subnet has
   `privateLinkServiceNetworkPolicies` set to `Disabled`. This cannot be
   changed after PLS creation — the PLS must be deleted and recreated with
   a corrected subnet.

## Infrastructure Needed [optional]

- Pre-provisioned NAT subnet with `privateLinkServiceNetworkPolicies` disabled
  in the CI Azure VNet for E2E testing
- The CI Azure subscription ID added to the allowed subscriptions list for
  test PLS resources
