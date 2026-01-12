---
title: add-dns-and-loadbalancer-conditions-to-managed-gateway
authors:
  - rikatz
reviewers:
  - alebedev87
  - candita
  - Miciah
approvers:
  - Miciah
api-approvers:
  - None
creation-date: 2025-10-21
last-updated: 2025-10-21
tracking-link:
  - https://issues.redhat.com/browse/NE-1816
see-also:
  - "/enhancements/ingress/configurable-dns-loadbalancerservice.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# Add DNS and LoadBalancer Conditions to Managed Gateway resource

## Summary

This enhancement adds DNS status conditions to Gateway listeners and LoadBalancer
status conditions to Gateway resources managed by OpenShift in the `openshift-ingress`
namespace. Specifically, it adds `DNSManaged` and `DNSReady` conditions to each
listener's status (in `status.listeners[].conditions[]`) and `LoadBalancerReady`
conditions to the Gateway status (in `status.conditions[]`). These conditions
provide visibility into per-listener DNS provisioning and gateway-level cloud
LoadBalancer service status, similar to the existing conditions on OpenShift
IngressController resources. This feature is scoped to cloud platform deployments
where native LoadBalancer services are available (AWS, Azure, GCP, etc.).

## Motivation

Cluster administrators and end users currently lack visibility into DNS and 
LoadBalancer provisioning failures for Gateway resources managed by OpenShift. 
Unlike IngressController resources which provide detailed condition status for 
DNS and LoadBalancer operations, Gateway resources do not expose this 
information. This gap makes it difficult to diagnose and troubleshoot issues 
when Gateways fail due to infrastructure provisioning problems.

By adding these conditions to managed Gateway resources, users gain the same 
observability they currently have with IngressController, enabling faster issue 
detection and resolution for DNS and cloud LoadBalancer failures.

### User Stories

* As a cluster administrator, I want to know which specific Gateway listeners have
DNS issues so that I can proactively address DNS configuration or quota problems
for specific hostnames.
* As a cluster administrator, I want to know when my Gateway has errors due to
a LoadBalancer issue so that I can troubleshoot cloud provider integration
issues or resource limits.

### Goals

* Add `DNSManaged` and `DNSReady` conditions to each Gateway listener status
(in `status.listeners[].conditions[]`) that reflect whether DNS is managed and
the state of DNS record provisioning for that specific listener
* Add `LoadBalancerReady` condition to Gateway status (in `status.conditions[]`)
that reflects the state of cloud LoadBalancer service provisioning
* Ensure conditions follow the same semantics and behavior as existing
IngressController DNS and LoadBalancer conditions by reusing the same status
computation logic
* Scope the feature to Gateways in the `openshift-ingress` namespace that are managed
by Openshift Gateway Class (`openshift.io/gateway-controller/v1`)
* Support cloud platforms with native LoadBalancer services (AWS, Azure, GCP, etc.)

### Non-Goals

* Supporting bare metal or on-premise deployments that do not have cloud 
LoadBalancer services
* Supporting environments where DNS is not managed by Openshift
* Adding these conditions to user-managed Gateway resources outside the 
`openshift-ingress` namespace
* Modifying or changing existing IngressController condition behavior or semantics
* Introducing custom condition types beyond DNS and LoadBalancer at this time
* Automatic remediation of DNS or LoadBalancer failures (this enhancement 
provides visibility only)

## Proposal

This enhancement proposes extending the Gateway status with DNS conditions at the
listener level and LoadBalancer conditions at the gateway level. Specifically:

* **Listener-level conditions** (in `status.listeners[].conditions[]`):
  - `DNSManaged`: Indicates whether OpenShift should be managing DNS for this
    listener's hostname
  - `DNSReady`: Indicates whether the DNS record for this listener's hostname
    is functioning correctly

* **Gateway-level conditions** (in `status.conditions[]`):
  - `LoadBalancerReady`: Indicates whether the cloud LoadBalancer service for
    the entire Gateway is functioning correctly

These conditions will be managed by a new gateway-status controller in the
cluster-ingress-operator. The placement reflects the architecture where:
- Each listener with a unique hostname gets its own DNSRecord resource
- The entire Gateway shares a single LoadBalancer Service resource

The conditions follow a two-tier model:
* **Managed conditions** (`DNSManaged`): Indicate whether OpenShift should be
managing DNS for a specific listener based on configuration (DNS zones, publishing
strategy, DNSManagementPolicy, etc.)
* **Ready conditions** (`DNSReady`, `LoadBalancerReady`): Indicate whether the
managed resource is actually functioning correctly

The implementation reuses the existing status computation logic from the
IngressController by:
1. Creating a shared `pkg/resources/status` package with condition computation
functions
2. Refactoring existing IngressController status code to use this shared package
3. Creating Gateway-specific wrapper functions that call the shared logic and
convert to Gateway API condition types

The gateway-status controller will:
1. Watch Gateway resources in `openshift-ingress` namespace that specify the
OpenShift Gateway Class
2. Discover associated DNSRecord and Service resources using the
`gateway.networking.k8s.io/gateway-name` label
3. For each listener in the Gateway:
   - Find the corresponding DNSRecord by matching the listener's hostname with
     the DNSRecord's `.spec.dnsName` field
   - Compute DNS conditions using the shared status logic (same as IngressController)
   - Update the listener's status in `status.listeners[].conditions[]`
4. For the Gateway's LoadBalancer service:
   - Fetch the single Service resource for the Gateway
   - Compute LoadBalancer condition using the shared status logic
   - Update the Gateway's status in `status.conditions[]`
5. Fetch cluster DNS and Infrastructure configuration
6. Patch Gateway status with the computed conditions including ObservedGeneration

This is a purely additive change that does not modify existing Gateway behavior 
or APIs. The conditions provide read-only status information to users and 
monitoring systems.

### Workflow Description

**Cluster Administrator** is a user who manages OpenShift cluster infrastructure
and troubleshoots platform issues.

**Customer/Developer** is a user who deploys applications and monitors
application ingress health.

**Ingress Operator** is the OpenShift operator responsible for managing ingress
resources including Gateways and IngressControllers.

**DNS Operator** is responsible for managing DNS records for cluster ingress.

**Cloud Provider API** is the underlying cloud infrastructure that provisions
LoadBalancer services.

#### Normal Flow (Success Case)

1. Customer creates or updates a Gateway resource with a single or multiple listeners in the `openshift-ingress` namespace
2. Gateway API controller (Istio) creates LoadBalancer service for the Gateway
3. Cloud Provider API provisions the LoadBalancer successfully
4. LoadBalancer service status is updated with external IP/hostname
5. Cluster Ingress Operator detects the Gateway resource and begins reconciliation
6. Cluster Ingress Operator initiates DNS record provisioning through its own dns controller,
creating one DNSRecord per unique listener hostname
7. Cluster Ingress Operator dns controller successfully creates all DNS records and
updates their status
8. Gateway Status Controller updates Gateway condition `LoadBalancerReady=True`
with reason "LoadBalancerProvisioned" in `status.conditions[]`
9. Gateway Status Controller updates each listener's conditions in `status.listeners[].conditions[]`:
   - Sets `DNSManaged=True` with reason "Normal" (DNS should be managed for this listener)
   - Sets `DNSReady=True` with reason "Normal" (DNS record for this listener provisioned successfully)
10. Customer checks Gateway status and sees:
    - Gateway-level condition `LoadBalancerReady=True`
    - Each listener has `DNSManaged=True` and `DNSReady=True`
    - Confirming the Gateway is fully operational

#### DNS Failure Flow

1. Customer creates a Gateway resource with multiple listeners in the `openshift-ingress` namespace
2. Cluster Ingress Operator initiates DNS record provisioning through its own dns controller,
creating one DNSRecord per unique listener hostname
3. Cluster Ingress Operator DNS controller encounters an error for one listener's DNSRecord
(e.g., invalid zone, quota exceeded, provider API error)
4. Cluster Ingress Operator DNS controller reports failure status in the affected
DNSRecord resource
5. Gateway Status Controller updates the affected listener's conditions in
`status.listeners[].conditions[]`:
   - Sets `DNSManaged=True` (DNS should be managed, configuration is correct)
   - Sets `DNSReady=False` with reason `FailedZones` and detailed error message from DNS provider
6. Other listeners with successful DNS records show `DNSReady=True`
7. Cluster Administrator reviews Gateway status and identifies which specific listener
has DNS issues by checking each listener's conditions
8. Cluster Administrator resolves the DNS issue (e.g., increases quota,
fixes zone configuration)
9. DNS Operator retries and successfully provisions the DNS record
10. Gateway Status Controller updates the affected listener's condition `DNSReady=True` with
reason "Normal"

#### LoadBalancer Failure Flow

1. Customer creates a Gateway resource in the `openshift-ingress` namespace
2. Gateway API controller creates LoadBalancer service for the Gateway
3. Cloud Provider API fails to provision LoadBalancer (e.g., quota exceeded,
subnet full, invalid configuration)
4. LoadBalancer service remains in Pending state with event describing the error
5. Gateway Status Controller updates Gateway condition `LoadBalancerReady=False`
with reason `LoadBalancerPending` and error details from service events
6. Cluster Administrator reviews Gateway status and identifies the cloud
infrastructure issue from the `LoadBalancerReady` condition message
7. Cluster Administrator resolves the issue (e.g., increases quota, adjusts VPC
configuration)
8. Cloud Provider API successfully provisions the LoadBalancer
9. LoadBalancer service status is updated with external IP/hostname
10. Gateway Status Controller updates Gateway condition `LoadBalancerReady=True`
with reason "LoadBalancerProvisioned"


```mermaid
sequenceDiagram
    participant User
    participant Gateway
    participant GWStatus as Gateway Status Controller
    participant IngressOp as Ingress Operator
    participant DNSRecords as DNSRecords (per listener hostname)
    participant DNSCtrl as Ingress Operator DNS Controller
    participant LBSvc as LoadBalancer Service
    participant Cloud as Cloud Provider

    User->>Gateway: Create/Update Gateway with multiple listeners (openshift-ingress namespace)
    Gateway->>IngressOp: Reconcile trigger (main ingress operator)
    IngressOp->>DNSRecords: Create DNSRecord per unique listener hostname with label gateway.networking.k8s.io/gateway-name
    IngressOp->>LBSvc: Create LoadBalancer Service with label gateway.networking.k8s.io/gateway-name

    Gateway->>GWStatus: Reconcile trigger (status controller)
    GWStatus->>DNSRecords: Fetch all DNSRecords by label
    GWStatus->>LBSvc: Fetch Service by label
    GWStatus->>GWStatus: Match each listener to its DNSRecord by hostname

    DNSRecords->>DNSCtrl: Ingress Operator DNS controller processes each DNSRecord
    DNSCtrl->>DNSCtrl: Provision DNS records

    alt DNS Success for Listener 1
        DNSCtrl-->>DNSRecords: Update DNSRecord status with successful zones
        GWStatus->>Gateway: Set listener[0].conditions: DNSManaged=True, DNSReady=True (reason: Normal)
    else DNS Failure for Listener 1
        DNSCtrl-->>DNSRecords: Update DNSRecord status with failed zones and errors
        GWStatus->>Gateway: Set listener[0].conditions: DNSManaged=True, DNSReady=False (reason: FailedZones)
    end

    Note over GWStatus,Gateway: Process remaining listeners independently...

    LBSvc->>Cloud: Request LoadBalancer provisioning

    alt LB Success
        Cloud-->>LBSvc: LoadBalancer provisioned (IP/hostname in status.loadBalancer.ingress)
        GWStatus->>Gateway: Set status.conditions: LoadBalancerReady=True (reason: LoadBalancerProvisioned)
    else LB Failure
        Cloud-->>LBSvc: Provisioning failed (service events contain error)
        GWStatus->>Gateway: Set status.conditions: LoadBalancerReady=False (reason: LoadBalancerPending)
    end

    User->>Gateway: Check status
    Gateway-->>User: Gateway-level LoadBalancer condition + per-listener DNS conditions show status
```

### API Extensions

No API extension will be made, just conditions being added to the already existing 
Gateway resource.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement applies to Hypershift environments with the following considerations:

* The ingress operator runs in the management cluster but manages Gateway resources for the hosted cluster
* DNS and LoadBalancer provisioning happens in the context of the hosted cluster's infrastructure
* Conditions will reflect the status of infrastructure provisioned for the hosted cluster
* No special handling is required; the same condition logic applies

#### Standalone Clusters

This enhancement is fully relevant for standalone OpenShift clusters deployed on
cloud platforms (AWS, Azure, GCP, etc.). The conditions provide visibility into
DNS and LoadBalancer provisioning for the cluster's ingress infrastructure.

#### Single-node Deployments or MicroShift

**Single-node OpenShift (SNO):**
* Applicable only when SNO is deployed on a cloud platform with LoadBalancer support
* On bare metal SNO deployments, LoadBalancer conditions would remain in `Unknown`
or `False` state as LoadBalancer services are not available
* DNS conditions apply regardless of platform if DNS records are being managed

**MicroShift:**
* This enhancement is not applicable to MicroShift deployments
* No impact on MicroShift resource consumption or configuration

**Resource Impact:**
* Minimal CPU/memory impact: only adds condition updates during reconciliation
* A new `gateway-status` controller is created on existing Cluster Ingress Operator
* Negligible increase in etcd storage for condition status (~1KB per Gateway)

### Implementation Details/Notes/Constraints

**Architecture Overview:**

This enhancement maps infrastructure resources to Gateway status as follows:

```
Gateway (1)
├── Spec
│   └── Listeners (N)
│       ├── Listener[0] (hostname: *.stage.example.com)
│       ├── Listener[1] (hostname: *.prod.example.com)
│       └── Listener[2] (hostname: *.stage.example.com)  # shares hostname with Listener[0]
│
└── Infrastructure Resources
    ├── Service (1) ────────────────────► status.conditions[]: LoadBalancerReady
    │   └── LoadBalancer (cloud)
    │
    └── DNSRecords (N unique hostnames)
        ├── DNSRecord[0] (*.stage.example.com) ──► status.listeners[0].conditions[]: DNSManaged, DNSReady
        │                                        └► status.listeners[2].conditions[]: DNSManaged, DNSReady
        └── DNSRecord[1] (*.prod.example.com)  ──► status.listeners[1].conditions[]: DNSManaged, DNSReady
```

**Key Architectural Decisions:**
1. **One LoadBalancer Service per Gateway** → Gateway-level `LoadBalancerReady` condition
2. **One DNSRecord per unique hostname** → Listener-level `DNSManaged` and `DNSReady` conditions
3. **Multiple listeners can share a hostname** → They map to the same DNSRecord and get identical DNS conditions
4. **Listeners without hostnames** → No DNS conditions added

**Example Gateway Status:**

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: example-gateway
  namespace: openshift-ingress
spec:
  listeners:
  - name: stage-http
    hostname: "*.stage.example.com"
    port: 80
    protocol: HTTP
  - name: stage-https
    hostname: "*.stage.example.com"
    port: 443
    protocol: HTTPS
  - name: prod-https
    hostname: "*.prod.example.com"
    port: 443
    protocol: HTTPS
status:
  # Gateway-level conditions (LoadBalancer status)
  conditions:
  - type: LoadBalancerReady
    status: "True"
    reason: LoadBalancerProvisioned
    message: "LoadBalancer is provisioned"
    observedGeneration: 1
    lastTransitionTime: "2025-01-12T10:00:00Z"

  # Listener-level conditions (DNS status per listener)
  listeners:
  - name: stage-http
    conditions:
    - type: DNSManaged
      status: "True"
      reason: Normal
      message: "..."
      observedGeneration: 1
      lastTransitionTime: "2025-01-12T10:00:00Z"
    - type: DNSReady
      status: "True"
      reason: Normal
      message: "...."
      observedGeneration: 1
      lastTransitionTime: "2025-01-12T10:00:00Z"

  - name: stage-https
    conditions:
    - type: DNSManaged
      status: "True"
      reason: Normal
      message: "...."
      observedGeneration: 1
      lastTransitionTime: "2025-01-12T10:00:00Z"
    - type: DNSReady
      status: "True"
      reason: Normal
      message: "..."
      observedGeneration: 1
      lastTransitionTime: "2025-01-12T10:00:00Z"

  - name: prod-https
    conditions:
    - type: DNSManaged
      status: "True"
      reason: Normal
      message: "...."
      observedGeneration: 1
      lastTransitionTime: "2025-01-12T10:00:00Z"
    - type: DNSReady
      status: "False"
      reason: FailedZones
      message: "Failed to publish DNS records: quota exceeded in zone prod.example.com"
      observedGeneration: 1
      lastTransitionTime: "2025-01-12T10:00:00Z"
```

Note: In this example, `stage-http` and `stage-https` share the same hostname, so they
have identical DNS conditions (both map to the same DNSRecord). The `prod-https` listener
has a different hostname and shows a DNS failure, while the LoadBalancer is working for
the entire Gateway.

**Controller Architecture:**
* A new `gateway-status` controller is added to the cluster-ingress-operator
* Uses the controller-runtime framework with predicates to filter relevant Gateway resources

**Condition Update Logic:**
* The ingress operator watches Gateway resources in the `openshift-ingress`
namespace that have the OpenShift Gateway Class as their `.spec.gatewayClassName` controller
* Associated DNSRecord and Service resources are discovered using the
`gateway.networking.k8s.io/gateway-name` label
* Only the first matching Service in the same namespace is used
for LoadBalancer status computation, as it is expected that Istio provisions just one LoadBalancer per Gateway
* For each listener in the Gateway:
  - The controller finds the matching DNSRecord by comparing the listener's hostname
    (from `.spec.listeners[].hostname`) with the DNSRecord's DNS name (from `.spec.dnsName`)
  - DNS conditions are computed using the shared status logic
  - Conditions are updated in `status.listeners[].conditions[]` for the matching listener
  - If no hostname is specified for a listener, DNS conditions are not added
* For the Gateway's LoadBalancer service:
  - LoadBalancer conditions are computed using the shared status logic
  - Conditions are updated in `status.conditions[]` at the Gateway level
* These functions wrap the existing functions used for IngressController, ensuring consistency
* The operator patches Gateway status using the Kubernetes API with the computed conditions
* Conditions include `ObservedGeneration` to track which Gateway generation the status reflects

**DNS Condition Details:**

DNS conditions are set per-listener in `status.listeners[].conditions[]`:

*DNSManaged Condition (per listener):*
* Set to `False` with reason `NoDNSZones` when cluster DNS configuration has no zones
* Set to `False` with reason `UnsupportedEndpointPublishingStrategy` when the
publishing strategy is not LoadBalancerService
* Set to `False` with reason `UnmanagedLoadBalancerDNS` when DNSManagementPolicy is `Unmanaged`
* Set to `True` with reason `Normal` when DNS records should be managed by OpenShift for this listener

*DNSReady Condition (per listener):*
* Set to `False` with reason `RecordNotFound` when the DNSRecord for this listener's hostname cannot be found
* Set to `False` with reason `NoZones` when the DNSRecord has no zones configured
* Set to `False` with reason `FailedZones` when one or more zones have failed to publish
* Set to `False` with reason `UnknownZones` when zone status cannot be determined
* Set to `True` with reason `Normal` when all zones have successfully published DNS records for this listener
* Error messages include specific DNS provider errors when provisioning fails
* If a listener has no hostname specified, DNS conditions are not added to that listener

**LoadBalancer Condition Details:**

LoadBalancer condition is set at the Gateway level in `status.conditions[]`:

*LoadBalancerReady Condition (gateway-level):*
* Set to `False` with reason `ServiceNotFound` when the associated Service
resource cannot be found
* Set to `False` with reason `LoadBalancerPending` when the service exists but
has no LoadBalancer ingress (external IP/hostname)
* Set to `False` with reason `SyncLoadBalancerFailed` when cloud provider events
indicate provisioning failures
* Set to `True` with reason `LoadBalancerProvisioned` when the service has an
external IP or hostname assigned
* Error messages include cloud provider error details extracted from service events

**Code Reuse:**
* The implementation creates a shared package that contains status computation logic
* This logic is used by both the existing IngressController status controller
and the new Gateway status controller
* Condition computation functions accept generic inputs (DNSRecord, Service,
events, config) and return conditions
* Gateway-specific wrappers convert between internal condition types
and Gateway API `metav1.Condition` types:
  - `ComputeGatewayAPIDNSStatus`: Converts to listener-level conditions in `status.listeners[].conditions[]`
  - `ComputeGatewayAPILoadBalancerStatus`: Converts to gateway-level conditions in `status.conditions[]`

**Resource Discovery:**
* The controller watches the following resources to trigger reconciliation:
  - Gateway resources in `openshift-ingress` namespace that uses GatewayClasses with 
  `openshift.io/gateway-controller/v1` as their `.spec.controllerName` 
  - DNSRecord resources with `gateway.networking.k8s.io/gateway-name` label
  - Service resources with `gateway.networking.k8s.io/gateway-name` label
  - DNS cluster configuration changes
  - Infrastructure cluster configuration changes

**Platform Detection:**
* Platform type is determined from the cluster Infrastructure resource
* DNS zone configuration comes from the cluster DNS resource

**Condition Lifecycle:**
* Conditions are added when a Gateway is reconciled in the `openshift-ingress` namespace
* Listener-level DNS conditions are updated in-place in `status.listeners[].conditions[]`
using `condutils.SetStatusCondition()` and the transition time must reflect changes
* Gateway-level LoadBalancer conditions are updated in-place in `status.conditions[]`
using `condutils.SetStatusCondition()` and the transition time must reflect changes
* Conditions are automatically garbage collected when the Gateway is deleted (part of Gateway status)
* Maximum of 8 conditions per listener and 8 conditions at gateway level are maintained
to prevent unbounded growth

**Permissions:**
* The cluster-ingress-operator service account is uses existing RBAC permissions to:
  - Get, List, Watch Gateway resources
  - Patch Gateway status subresource
  - Get, List, Watch DNSRecord and Service resources by label

### Risks and Mitigations

**Risk: Condition limit on Gateway and Listener resources**
Gateway API establishes a limit of 8 conditions on both the Gateway status and each
listener status. Istio, which is used as our Gateway API implementation, sets 2
gateway-level conditions and also sets listener-level conditions. Cluster Ingress
Operator will set 1 additional gateway-level condition (`LoadBalancerReady`) and
2 listener-level conditions per listener (`DNSManaged`, `DNSReady`). If Istio starts
adding more conditions, this can be a problem.

* Mitigation: An e2e test should be added to assert that the total number of conditions
at both gateway and listener levels is consistent with the current Istio and Cluster
Ingress Operator conditions. In case of change, the test should fail and engineering
should verify if Istio is adding additional conditions.

**Risk: Exposure of sensitive information in condition messages**
* Mitigation: Sanitize error messages to remove any credentials or sensitive
data before including in conditions

**Risk: Compatibility with future Gateway API versions**
* Mitigation: Use well-defined condition types that align with Kubernetes
conventions and Gateway API patterns

**Security Review:**
* No new authentication or authorization mechanisms introduced
* Conditions are read-only status information
* Standard RBAC applies to Gateway resources
* Cluster Ingress Operator cluster role needs a new permission to `UPDATE` and `PATCH`
Gateway status subresource.

**UX Review:**
* Condition messages should be clear and actionable for both administrators and developers
* Error messages should include enough detail to enable self-service troubleshooting

### Drawbacks

**Additional API surface:**
* Adding conditions increases the Gateway status size. With listener-level DNS
conditions, the size increase scales with the number of listeners (~1KB per
Gateway + ~1KB per listener with DNS conditions)

**Maintenance burden:**
* Requires ongoing maintenance to keep condition logic in sync with DNS operator and cloud provider behavior
* May need updates when DNS operator or cloud provider integrations change

**Potential confusion:**
* Users may expect these conditions to appear on all Gateway resources, not just
those in `openshift-ingress`
* Users need to understand the difference between gateway-level conditions (LoadBalancer)
and listener-level conditions (DNS)
* Clear documentation will be needed to explain the scoping and condition placement

**Not applicable to all environments:**
* The LoadBalancer condition is only meaningful on cloud platforms or platforms with
`LoadBalancer` support.
* Users on bare metal may see persistent `False` or `Unknown` status which could
be confusing

## Alternatives (Not Implemented)

**Alternative 1: Use Gateway API standard conditions only**
* The Gateway API specification defines standard conditions like `Accepted` and `Programmed`
* We could map DNS/LB status into these existing conditions rather than adding new ones
* Rejected because: Standard conditions don't provide the granularity needed to
distinguish DNS vs. LoadBalancer issues

**Alternative 2: Add conditions to all Gateway resources cluster-wide**
* Could add these conditions to any Gateway resource, not just those in `openshift-ingress`
* Rejected because: User-managed Gateways may not follow the same DNS/LB
provisioning patterns, and this would create inconsistent behavior

**Alternative 3: Create separate status CRD for infrastructure details**
* Could create a new CRD (e.g., `GatewayInfrastructureStatus`) to hold DNS/LB information
* Rejected because: Adds unnecessary complexity; conditions are the standard
Kubernetes pattern for status reporting

**Alternative 4: Emit events instead of conditions**
* Could use Kubernetes events to report DNS/LB status instead of conditions
* Rejected because: Events are transient and harder to query programmatically;
conditions provide persistent, queryable status

**Alternative 5: Put DNS conditions at Gateway level instead of listener level**
* Could add DNS conditions to `status.conditions[]` (gateway-level) and aggregate
status across all DNSRecords
* Rejected because:
  - Each listener with a unique hostname gets its own DNSRecord resource
  - A single gateway-level DNS condition cannot accurately represent the state when
    some listeners have working DNS and others don't
  - Users need to know which specific listener has DNS issues for troubleshooting
  - Gateway API supports listener-level conditions specifically for this use case
  - The current implementation in cluster-ingress-operator creates per-hostname DNSRecords,
    making listener-level status the natural fit

## Open Questions [optional]

1. What should the condition status be on platforms without LoadBalancer support (bare metal)?
   - Proposed: We do not set `LoadBalancerManaged` on Gateway API conditions, and the
   condition `LoadBalancerReady` will reflect the availability of the load balancer provisioned
   by the Gateway API controller (Istio).
  
## Test Plan

**Unit Tests:**
* Test `ComputeDNSStatus` with various DNSRecord states (no zones, failed zones, successful zones, unmanaged)
* Test `ComputeLoadBalancerStatus` with various Service states (not found, pending, provisioned, failed)
* Test `ComputeGatewayAPIDNSStatus` wrapper correctly converts internal conditions to Gateway API listener conditions
* Test `ComputeGatewayAPILoadBalancerStatus` wrapper correctly converts internal conditions to Gateway API gateway-level conditions
* Test condition computation with DNSManagementPolicy set to Managed vs Unmanaged
* Test ObservedGeneration is correctly set on conditions
* Test matching DNSRecords to listeners by hostname
* Test handling of listeners without hostnames (no DNS conditions should be added)
* Test handling of multiple listeners with the same hostname (should map to same DNSRecord)

**E2E Tests:**
* Create Gateway in `openshift-ingress` namespace with listeners and verify:
  - Gateway-level condition `LoadBalancerReady=True` in `status.conditions[]`
  - Each listener has `DNSManaged=True` and `DNSReady=True` in `status.listeners[].conditions[]`
* On the same test, verify that changing Gateway generation will bump the conditions `observedGeneration`
* On the same test, verify that the Gateway API controller (Istio) reconciliation
does not replace OpenShift conditions
* On the same test, verify the condition count is consistent with Istio and OpenShift
added conditions (both at gateway and listener levels)
* Create Gateway out of `openshift-ingress` and verify that no OpenShift condition is added
* Create Gateway with wrong DNS Domain and verify that listener DNS conditions reflect the failure
in `status.listeners[].conditions[]`
* Create Gateway with multiple listeners with different hostnames. Verify:
  - Each listener gets its own `DNSReady` and `DNSManaged` conditions in `status.listeners[].conditions[]`
  - Each listener's conditions correspond to its specific DNSRecord status
* Create Gateway with multiple listeners sharing the same hostname. Verify:
  - All listeners with the same hostname show the same DNS condition status
  - Only one DNSRecord is created for the shared hostname
* Create Gateway with a listener without a hostname. Verify:
  - That listener does not have DNS conditions added
  - Other listeners with hostnames do have DNS conditions

## Graduation Criteria

This proposal just adds new conditions to Gateway, and don't impact the Gateway
behavior. There is no API or behavior change, so no need to go through graduation criteria

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

**Upgrade Strategy:**
* This is a purely additive feature with no configuration changes required
* On upgrade to a version with this enhancement:
  - Existing Gateway resources in `openshift-ingress` namespace will have 
  conditions added during next reconciliation
  - No user action required
  - No disruption to existing Gateways or traffic flow
* The feature is backwards compatible; older clients that don't understand 
these conditions will ignore them

**Downgrade Strategy:**
* On downgrade to a version without this enhancement:
  - The new conditions will remain in the Gateway status but will not be updated
  - The CVO does not delete status fields, so conditions will persist but become stale
  - No impact on Gateway functionality; conditions are read-only status
  - If desired, users can manually remove the conditions, but this is not required

**Version Skew:**
* Ingress operator and Gateway resources can be at different versions during upgrade
* Older ingress operator will not add/update the new conditions (graceful degradation)
* Newer ingress operator will add conditions to any Gateway in `openshift-ingress` regardless of when it was created

## Version Skew Strategy

This enhancement involves coordination between:
* Ingress operator (adds conditions to Gateway status)
* Gateway API controller (adds labels, creates services)

**Version Skew Handling:**
* No kubelet changes are involved
* No CSI, CRI, or CNI changes are involved

**Compatibility:**
* Feature works with Gateway API v1
* No breaking changes to Gateway API contract
* Follows standard Kubernetes condition conventions

## Operational Aspects of API Extensions

**API Extension Type:**
* This enhancement adds status conditions to existing Gateway resources (no new CRDs)
* Conditions are added at two levels:
  - Gateway-level conditions in `status.conditions[]` for LoadBalancer status
  - Listener-level conditions in `status.listeners[].conditions[]` for DNS status
* No admission webhooks, conversion webhooks, aggregated API servers, or finalizers are introduced
* Status conditions are a standard Kubernetes and Gateway API pattern and do not require API approval for new types

**Impact on Existing SLIs:**
* Negligible impact on API throughput: condition updates happen during normal reconciliation
* No new API calls introduced; only status updates to existing Gateway resources
* Expected number of managed Gateways in `openshift-ingress` namespace: typically 1-10 per cluster

**Failure Modes:**
* If ingress operator is unavailable, conditions will not be updated, but Gateways continue to function
* Conditions are status-only and do not affect Gateway data plane functionality

**Health Indicators:**
* Monitor ingress-operator pod health and logs for errors updating Gateway conditions
* Existing ingress-operator metrics and alerts apply

**Responsible Teams:**
* Networking team (ingress): primary owner for Gateway condition logic

## Support Procedures

**Detecting Issues:**

*Symptom: Listener conditions show `DNSManaged=False`*

Note: This is not an issue for users that opted to not have DNS managed.
* Check Gateway status: `oc get gateway <name> -n openshift-ingress -o yaml`
* Look at the specific listener's conditions in `status.listeners[].conditions[]`
* Review condition reason and message:
  - Reason `NoDNSZones`: Cluster DNS configuration has no zones configured
  - Reason `UnsupportedEndpointPublishingStrategy`: Publishing strategy is not LoadBalancerService
  - Reason `UnmanagedLoadBalancerDNS`: DNSManagementPolicy is set to Unmanaged
* Check cluster DNS configuration: `oc get dns.config.openshift.io cluster -o yaml`
* Check Infrastructure configuration for platform type: `oc get infrastructure cluster -o yaml`

*Symptom: Listener conditions show `DNSReady=False`*
* Check Gateway status: `oc get gateway <name> -n openshift-ingress -o yaml`
* Look at the specific listener's conditions in `status.listeners[].conditions[]`
* Review condition reason and message:
  - Reason `RecordNotFound`: DNSRecord resource for this listener's hostname not found
  - Reason `NoZones`: DNSRecord has no zones configured
  - Reason `FailedZones`: One or more DNS zones failed to publish (message contains provider errors)
  - Reason `UnknownZones`: Zone status cannot be determined
* Find the listener's hostname from `.spec.listeners[].hostname`
* Check DNSRecord resource by matching the hostname: `oc get dnsrecord -n openshift-ingress -l gateway.networking.k8s.io/gateway-name=<gateway-name> -o yaml`
* Look for the DNSRecord where `.spec.dnsName` matches the listener's hostname
* Check DNS operator logs: `oc logs -n openshift-dns-operator deployment/dns-operator`
* Common causes: DNS zone misconfiguration, DNS provider quota exceeded, invalid DNS records, provider API failures

*Symptom: Gateway conditions show `LoadBalancerReady=False`*
* Check Gateway status: `oc get gateway <name> -n openshift-ingress -o yaml`
* Review condition reason and message:
  - Reason `ServiceNotFound`: Service resource with label `gateway.networking.k8s.io/gateway-name=<gateway-name>` not found
  - Reason `LoadBalancerPending`: Service exists but has no external IP/hostname (message may contain cloud provider errors)
  - Reason `SyncLoadBalancerFailed`: Cloud provider events indicate provisioning failures
* Check Service resource: `oc get svc -n openshift-ingress -l gateway.networking.k8s.io/gateway-name=<gateway-name> -o yaml`
* Check service events: `oc describe svc <lb-service-name> -n openshift-ingress`
* Common causes: Cloud quota exceeded, subnet IP exhaustion, invalid security group configuration, VPC limits

*Symptom: Conditions are missing entirely*
* Check ingress operator health: `oc get pods -n openshift-ingress-operator`
* Check ingress operator logs for errors: `oc logs -n openshift-ingress-operator deployment/ingress-operator | grep gateway-status`
* Verify Gateway is in `openshift-ingress` namespace (conditions only apply there)
* Verify Gateway has correct controller class: `oc get gateway <name> -n openshift-ingress -o jsonpath='{.spec.gatewayClassName}'`
* Check if gateway-status controller is running
* For DNS conditions: Check that listeners have hostnames specified in `.spec.listeners[].hostname`
  (DNS conditions are only added to listeners with hostnames)
* For LoadBalancer conditions: Check in `status.conditions[]` at the Gateway level
* For DNS conditions: Check in `status.listeners[].conditions[]` for each listener

**Disabling the Feature:**
* This feature cannot be easily disabled as it's integrated into ingress operator reconciliation
* Conditions are harmless status fields and do not need to be disabled
* If needed, users can ignore these conditions; they do not affect Gateway functionality

**Graceful Failure:**
* Condition updates are best-effort; failures to update conditions do not block Gateway provisioning
* If condition updates fail, ingress operator logs errors but continues reconciliation
* Stale conditions do not prevent Gateways from functioning correctly

**Escalation:**
* For DNS-related issues: escalate to DNS/Networking team
* For LoadBalancer issues: escalate to Cloud Platform team for the specific cloud provider
* For condition update issues: escalate to Networking/Ingress team

## Infrastructure Needed [optional]

No new infrastructure required. This enhancement uses existing OpenShift components:
* Ingress operator (existing)
* DNS operator (existing)
* Cloud controller manager / LoadBalancer services (existing)
