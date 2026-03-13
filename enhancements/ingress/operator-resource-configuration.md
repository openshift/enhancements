---
title: ingress-router-resource-configuration
authors:
  - "@joseorpa"
reviewers:
  - "@miciah"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2025-10-27
last-updated: 2026-01-12
tracking-link:
  - https://issues.redhat.com/browse/RFE-1476
see-also:
  - "/enhancements/monitoring/cluster-monitoring-config.md"
replaces: []
superseded-by: []
---

# Ingress Router Resource Configuration

## Summary

This enhancement proposes adding the ability to configure resource limits for 
ingress router pods (HAProxy deployments) via a new field in the v1 IngressController 
API, gated behind a feature gate. This will allow router pods to achieve Guaranteed 
QoS class by setting limits equal to requests.

## Motivation

Currently, ingress router pods (the HAProxy deployments that handle ingress traffic) 
are created with resource requests only (CPU: 100m, Memory: 256Mi) but no resource 
limits defined. According to [RFE-1476](https://issues.redhat.com/browse/RFE-1476), 
this presents challenges for:

1. **QoS Class Requirements**: Without limits, router pods have "Burstable" QoS class. 
   Setting limits equal to requests achieves "Guaranteed" QoS class, providing better 
   stability and predictability for critical ingress infrastructure.
2. **Compliance requirements**: Some organizations require all pods have both requests 
   and limits defined for auditing, cost allocation, and capacity planning purposes.
3. **Resource accounting**: Better cost allocation and resource planning when limits 
   are explicitly defined.
4. **Cluster resource constraints**: Guaranteed QoS provides better protection against 
   resource contention and eviction.

Currently, there is no way to configure resource requirements for router pods via the 
IngressController API. The router pods use hardcoded default values from the deployment 
template. Customers need the ability to configure both **requests** and **limits** to 
achieve Guaranteed QoS class. This enhancement introduces this capability via a new 
field in the v1 API, protected behind a feature gate during the Tech Preview period.

### User Stories

#### Story 1: Platform Administrator Needs Guaranteed QoS for Router Pods

As a platform administrator, I want to set resource limits on ingress router pods 
to ensure they have a QoS class of "Guaranteed" for critical ingress infrastructure. 
Currently, router pods only have requests defined, giving them "Burstable" QoS class, 
which makes them susceptible to resource contention and potential eviction.

Acceptance Criteria:
- Can specify resource limits via IngressController CR
- Router pods reflect the configured limits
- Router pods achieve QoS class "Guaranteed" when requests == limits
- Configuration applies to all router pod replicas

#### Story 2: High-Traffic Application Requiring Resource Guarantees

As an operator of a high-traffic e-commerce platform, I need guaranteed resource 
allocation for my ingress router pods to ensure consistent performance during traffic 
spikes (seasonal sales, marketing events). Without resource limits, the pods have 
Burstable QoS and may be throttled or evicted under cluster resource pressure, 
causing service disruptions.

Acceptance Criteria:
- Can configure resource limits matching requests for Guaranteed QoS
- Router pods maintain stable performance under cluster resource pressure
- Configuration survives router pod restarts and upgrades

#### Story 3: Resource-Constrained Edge Deployment

As an operator of an edge computing deployment with limited resources, I need to 
set strict resource limits on ingress router pods to prevent them from consuming 
excessive resources and impacting other critical workloads. Without limits, router 
pods with Burstable QoS can burst beyond their requests, potentially starving other 
pods in the resource-constrained environment.

Acceptance Criteria:
- Can configure resource limits to cap maximum resource consumption
- Router pods do not exceed defined resource boundaries
- Guaranteed QoS ensures router pods get their allocated resources under pressure
- Other workloads on the node are protected from router resource overuse

### Goals

- Allow configuration of resource limits for ingress router pods (HAProxy containers)
- Add feature to v1 API protected by a feature gate for simplicity
- Maintain backward compatibility with existing IngressController v1 API
- Use feature gate for Tech Preview period, then promote to Default feature set for GA
- Enable router pods to achieve Guaranteed QoS class
- Provide sensible defaults that work for most deployments
- Support configuration for router container and sidecar containers (logs, metrics)

### Non-Goals

- Configuring resources for the ingress-operator deployment itself (the controller)
- Auto-scaling or dynamic resource adjustment based on traffic load
- Creating a separate v1alpha1 API version (using v1 with feature gate instead)
- Vertical Pod Autoscaler (VPA) integration (may be future work)
- Horizontal Pod Autoscaler (HPA) configuration (separate concern)

## Proposal

### Workflow Description

**Platform Administrator** is a human responsible for configuring the OpenShift cluster.

1. Platform administrator enables the `IngressRouterResourceLimits` feature gate
2. Platform administrator creates or updates an IngressController CR using the v1 API
3. Platform administrator sets the new `resources` field with desired resource limits/requests
4. The ingress-operator watches for changes to the IngressController CR
5. The ingress-operator reconciles the router deployment with the specified resources
6. Kubernetes performs a rolling restart of the router pods with the new resource configuration
7. Router pods achieve Guaranteed QoS class (when limits == requests)
8. Platform administrator verifies the changes with `oc describe deployment router-default -n openshift-ingress`
9. Platform administrator confirms QoS class with `oc get pod -n openshift-ingress -o jsonpath='{.items[*].status.qosClass}'`

### API Extensions

Add a new field to the existing v1 IngressController API in the `operator.openshift.io` 
group, gated behind a feature gate. This approach is preferred by the networking team 
for its simplicity - adding the feature directly to the stable v1 API while protecting 
it behind a feature gate during the Tech Preview period.

The new field allows configuring resource **requests** and **limits** for router pods. 
Currently, there is no existing way to configure router pod resources via the API - 
the router pods use hardcoded default values from the deployment template. This 
enhancement adds a new field to configure both requests and limits, enabling router 
pods to achieve Guaranteed QoS class.

#### Feature Gate

This feature requires a new feature gate: **`IngressRouterResourceLimits`**

The feature gate controls whether the new v1 API field is recognized and enforced 
by the ingress-operator. Initially, the feature gate will be part of the 
TechPreviewNoUpgrade feature set, and will be promoted to the Default feature set 
once the feature graduates to GA.

**Enabling the Feature Gate:**

To enable the feature gate in your OpenShift cluster, you can use either the patch command 
or apply a FeatureGate configuration.

**Option 1: Enable all Tech Preview features (includes IngressRouterResourceLimits):**
```bash
oc patch featuregate cluster --type merge --patch '{"spec":{"featureSet":"TechPreviewNoUpgrade"}}'
```

**Option 2: Enable only the specific feature gate using patch command:**
```bash
oc patch featuregate cluster --type merge --patch '{"spec":{"featureSet":"CustomNoUpgrade","customNoUpgrade":{"enabled":["IngressRouterResourceLimits"]}}}'
```

**Option 3: Apply a custom FeatureGate configuration file:**
```yaml
apiVersion: config.openshift.io/v1
kind: FeatureGate
metadata:
  name: cluster
spec:
  featureSet: CustomNoUpgrade
  customNoUpgrade:
    enabled:
    - IngressRouterResourceLimits
```

Apply with:
```bash
oc apply -f featuregate.yaml
```

**Note**: Enabling feature gates may require cluster components to restart. For 
production environments, test in non-production clusters first. Using `TechPreviewNoUpgrade` 
or `CustomNoUpgrade` means the cluster cannot be upgraded and should only be used for 
testing.

#### API Changes

Add a new field to the existing `IngressControllerSpec` in the v1 API:

```go
package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    corev1 "k8s.io/api/core/v1"
)

// IngressControllerSpec is the specification of the desired behavior of the IngressController.
type IngressControllerSpec struct {
    // ... existing v1 fields ...

    // resources defines resource requirements (requests and limits) for the
    // router pods (HAProxy containers). This field allows setting resource limits
    // to achieve Guaranteed QoS class for router pods.
    //
    // When not specified, defaults to:
    //   router container:
    //     requests: cpu: 100m, memory: 256Mi
    //     limits: none (Burstable QoS)
    //
    // To achieve Guaranteed QoS, set limits equal to requests:
    //   resources:
    //     routerContainer:
    //       requests:
    //         cpu: 100m
    //         memory: 256Mi
    //       limits:
    //         cpu: 100m
    //         memory: 256Mi
    //
    // Note: Changing these values will cause router pods to perform a rolling restart.
    //
    // +optional
    // +openshift:enable:FeatureGate=IngressRouterResourceLimits
    Resources RouterResourceRequirements `json:"resources,omitzero"`
}

// +kubebuilder:validation:MinProperties=1
// RouterResourceRequirements defines resource requirements for ingress router pod containers.
// At least one of routerContainer, metricsContainer, or logsContainer must be set.
type RouterResourceRequirements struct {
    // routerContainer specifies resource requirements (requests and limits) for the
    // router (HAProxy) container in router pods.
    //
    // If not specified, defaults to:
    //   requests: cpu: 100m, memory: 256Mi
    //   limits: none
    //
    // +optional
    RouterContainer *corev1.ResourceRequirements `json:"routerContainer,omitempty"`

    // metricsContainer specifies resource requirements for the metrics sidecar
    // container in router pods.
    //
    // If not specified, uses Kubernetes default behavior (no requests or limits).
    //
    // +optional
    MetricsContainer *corev1.ResourceRequirements `json:"metricsContainer,omitempty"`
    
    // logsContainer specifies resource requirements for the logs sidecar container
    // in router pods (if logs sidecar is enabled).
    //
    // If not specified, uses Kubernetes default behavior (no requests or limits).
    //
    // +optional
    LogsContainer *corev1.ResourceRequirements `json:"logsContainer,omitempty"`
}
```

#### Example Usage

**Example 1: Setting limits to achieve Guaranteed QoS**

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  # All existing v1 fields continue to work
  replicas: 2
  domain: apps.example.com
  
  # New resources field for router pod resource configuration with limits
  # This achieves Guaranteed QoS by setting limits equal to requests
  # Requires IngressRouterResourceLimits feature gate to be enabled
  resources:
    routerContainer:
      requests:
        cpu: 100m
        memory: 256Mi
      limits:
        cpu: 100m
        memory: 256Mi
```

**Example 2: Higher resources for high-traffic clusters**

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  replicas: 3
  domain: apps.example.com
  
  # Configure resources for router and metrics containers
  resources:
    routerContainer:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 1000m
        memory: 1Gi
    metricsContainer:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 100m
        memory: 128Mi
```

#### API Validation

The following validations will be enforced:

1. **Resource limits must be >= requests**: Kubernetes standard validation enforced by API server
2. **Feature gate check**: If `IngressRouterResourceLimits` feature gate is disabled, 
   the `resources` field will be ignored (with a warning event logged)
3. **Minimum values** (recommendations, not hard limits):
   - Router container: cpu >= 100m, memory >= 128Mi recommended for production
   - Values below recommendations will generate warning events but not block the request

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift environments:
- The management cluster has its own IngressController for management traffic
- Each hosted cluster has its own IngressController for guest traffic
- This enhancement applies to both contexts independently
- Configuration is specific to each IngressController instance's router pods
- Router pods for hosted clusters run in the hosted cluster namespace

#### Standalone Clusters

Standard behavior - configuration applies to router pod deployments in the 
`openshift-ingress` namespace managed by the IngressController.

#### Single-node Deployments

Particularly beneficial for single-node OpenShift (SNO) deployments where:
- Resource constraints are tighter
- Setting appropriate limits helps prevent resource contention for router pods
- Guaranteed QoS class improves stability for critical ingress infrastructure
- Single router replica benefits from predictable resource allocation

### Implementation Details/Notes/Constraints

#### Feature Gate Strategy

- **Feature Gate**: `IngressRouterResourceLimits`
- **Initial State**: Part of TechPreviewNoUpgrade feature set
- **GA Promotion**: Move to Default feature set when graduating to GA
- **Field Protection**: The new `spec.resources` field is protected by the feature gate
- **Compatibility**: Existing v1 API clients continue working; new field ignored when feature gate disabled

#### Controller Implementation

The existing deployment controller in the cluster-ingress-operator will be enhanced to 
handle the new `resources` field when reconciling router deployments.

**Controller enhancements:**
1. Watch IngressController resources (v1 API)
2. Check `IngressRouterResourceLimits` feature gate status
3. When feature gate is enabled and `spec.resources` field is set:
   - Use `spec.resources` field to configure router pod container resources
4. When feature gate is disabled or `spec.resources` field is not set:
   - Use hardcoded defaults from deployment template (current behavior)
   - Log warning event if `spec.resources` is set but feature gate is disabled
5. Reconcile router `Deployment` in `openshift-ingress` namespace
6. Update container resource specifications (router, metrics, logs containers)
7. Handle error cases gracefully (invalid values, etc.)
8. Generate events for configuration issues (warnings, validation failures)

#### Default Behavior

When the `spec.resources` field is not set or feature gate is disabled:

**Current behavior (unchanged):**
- Router pods use hardcoded defaults from deployment template:
  - Router container: requests(cpu: 100m, memory: 256Mi), no limits
  - QoS class: Burstable

**With `spec.resources` field set (and feature gate enabled):**
- Router pods use `spec.resources` configuration
- Users can set limits to achieve Guaranteed QoS:
  - Router container: requests(cpu: 100m, memory: 256Mi), limits(cpu: 100m, memory: 256Mi)
  - QoS class: Guaranteed

**Backward compatibility:**
- Existing IngressControllers continue working unchanged
- New field is ignored when feature gate is disabled
- Defaults remain the same as today

#### Upgrade Behavior

When upgrading to a version with this enhancement:
1. New `resources` field is added to v1 IngressController API (gated by feature gate)
2. Feature gate `IngressRouterResourceLimits` is part of TechPreviewNoUpgrade feature set
3. Existing IngressController CRs continue working unchanged
4. Router pods continue with existing resource configuration (no automatic changes)
5. No user action required - existing behavior preserved
6. Users can enable feature gate and use new `resources` field when ready
7. When feature is promoted to GA, feature gate moves to Default feature set

### Risks and Mitigations

#### Risk: User sets resources too low, router pods become unhealthy

**Impact**: Router pods may OOMKill, fail to handle traffic, or become unresponsive, 
causing ingress traffic disruptions

**Mitigation**:
- Document minimum recommended values (cpu: 100m, memory: 256Mi as baseline)
- Add validation warnings (not blocking) for values below minimums
- Include troubleshooting guide for common issues (OOM, CPU throttling)
- Monitor router pod health metrics
- Provide example configurations for common scenarios (low/medium/high traffic)
- CPU throttling and memory pressure metrics available via Prometheus

**Likelihood**: Medium

**Detection**: Router pod restarts, increased error rates, degraded performance

#### Risk: Existing tooling may not recognize feature-gated field

**Impact**: External tools may not recognize or handle the new `spec.resources` field

**Mitigation**:
- v1 API remains unchanged and fully functional
- Field is simply ignored if not understood by tools
- No breaking changes to existing functionality
- Standard Kubernetes resource requirements types used

**Likelihood**: Low

#### Risk: Router pod rolling restart causes brief traffic disruption

**Impact**: Configuration changes trigger rolling restart of router pods, potential 
brief connection disruptions during pod replacement

**Mitigation**:
- Document that changes trigger rolling restart (expected Kubernetes behavior)
- Rolling restart minimizes impact - only one pod restarted at a time
- Connection draining allows graceful termination of existing connections
- Load balancer redistributes traffic to remaining healthy pods
- Changes to router resources are not expected to be frequent operations
- Recommended to perform during maintenance windows for production systems

**Likelihood**: High (by design), **Severity**: Low to Medium (depends on traffic patterns)

#### Risk: Resource configuration drift

**Impact**: Manual changes to router pod deployment could be overwritten by operator reconciliation

**Mitigation**:
- Operator reconciliation loop detects and corrects drift automatically
- Document that configuration must be via IngressController CR, not direct deployment edits
- Events generated when drift is detected and corrected
- Router deployments are managed resources - manual changes not supported

**Likelihood**: Low

### Drawbacks

1. **Feature gate dependency**: Adds operational complexity with feature gate management during Tech Preview
2. **Documentation overhead**: Need to document new field and usage patterns
3. **Testing complexity**: Must test upgrade scenarios and feature gate behavior
4. **Potential for misconfiguration**: Users may set inappropriate resource values affecting router stability

## Design Details

### Open Questions

1. **Q**: Should we support auto-scaling (VPA) for router pods in the future?
   - **A**: Out of scope for initial implementation, but API design should not preclude it

2. **Q**: Should we add hard validation for minimum resource values?
   - **A**: Start with warnings/documentation, consider hard validation if widespread issues arise

3. **Q**: Should this apply to all IngressControllers or only the default?
   - **A**: API supports any IngressController, including custom IngressControllers

### Test Plan

#### Unit Tests

- **Feature gate handling**: Behavior with gate enabled/disabled
- **Controller reconciliation logic**: Mock router deployment updates with resources field
- **Resource requirement validation**: Edge cases and invalid inputs
- **Default value handling**: Ensure defaults applied correctly when field not set

Coverage target: >80% for new code

#### Integration Tests

- **API server integration**: v1 API field recognition when feature gate enabled
- **Controller watches**: IngressController changes trigger router deployment reconciliation
- **Feature gate integration**: Verify feature gate controls field recognition

#### E2E Tests

- **Create IngressController with resources field**
  - Verify router deployment is updated with correct resource limits
  - Verify router pods achieve Guaranteed QoS class
  - Verify router continues handling traffic normally
  
- **Update existing IngressController to add resource limits**
  - Verify rolling update of router pods occurs
  - Verify no traffic disruption during rolling restart
  - Verify new pods have correct QoS class
  
- **Remove resource requirements (revert to defaults)**
  - Verify router deployment reverts to default values
  - Verify router pods revert to Burstable QoS
  
- **Feature gate disabled scenario**
  - Set resources field with feature gate disabled
  - Verify field is ignored with warning
  - Verify fallback to default behavior
  
- **Upgrade scenario tests**
  - Upgrade from version without feature to version with feature
  - Verify existing IngressControllers continue working unchanged
  - Enable feature gate and verify resources field works
  
- **Downgrade scenario tests**
  - Downgrade from version with feature to version without
  - Verify graceful degradation (resources field ignored)
  - Verify router pods continue with default configuration

#### Manual Testing

- Test in resource-constrained environments (e.g., single-node)
- Verify QoS class changes as expected (Burstable → Guaranteed)
- Test with various resource configurations (very low, very high)
- Test router pod behavior when limits are hit (OOMKill, CPU throttling)
- Test with multiple IngressController instances
- Monitor router performance metrics (latency, throughput) with different resource configs
- Test traffic handling during resource limit OOM scenarios

### Graduation Criteria

#### Dev Preview -> Tech Preview (feature gated v1 API)

- [x] Feature implemented behind feature gate
- [x] Unit and integration tests passing
- [x] E2E tests passing in CI
- [x] Documentation published in OpenShift docs
- [x] Enhancement proposal approved

#### Tech Preview -> GA (promote feature gate to Default)

This section describes criteria for graduating from Tech Preview (feature gate in 
TechPreviewNoUpgrade) to GA (feature gate in Default) in the same release development 
cycle. For a straightforward feature like this, the typical approach is to introduce as 
Tech Preview and graduate to GA within the same release.

- [ ] Feature implemented and stable during Tech Preview period
- [ ] No major bugs or design issues discovered during Tech Preview
- [ ] Unit, integration, and E2E tests passing consistently
- [ ] Performance impact assessed and documented (minimal/acceptable)
- [ ] Documentation complete and reviewed

**Timeline**: Promote to GA (move feature gate to Default feature set) in the same release 
cycle if Tech Preview period shows no issues. If significant issues are discovered, address 
them and consider promotion in the next release.

#### Removing a deprecated feature

N/A - this is a new feature

### Upgrade / Downgrade Strategy

#### Upgrade

**From version without feature → version with feature:**

1. v1 IngressController API updated with new `resources` field (protected by feature gate)
2. Feature gate added to TechPreviewNoUpgrade feature set
3. Existing IngressController CRs remain functional and unchanged
4. Users can enable feature gate and use new `resources` field
5. No breaking changes to existing functionality

**User action required**: None for default behavior

**User action optional**: Enable feature gate and use new `resources` field to configure router pod resource limits

#### Downgrade

**From version with feature → version without feature:**

1. Feature gate `IngressRouterResourceLimits` no longer recognized
2. IngressController CRs with `spec.resources` field set will have it ignored
3. The field remains in the CR but is not processed
4. Router pods fall back to hardcoded defaults (cpu: 100m, memory: 256Mi)
5. No data loss - CR remains valid, field just ignored
6. Router pods will revert to Burstable QoS if Guaranteed was configured via `spec.resources`

**User impact**: Loss of custom router resource limits configured via the feature-gated 
field; reverts to hardcoded defaults

#### Version Skew

Supported version skew follows standard OpenShift practices:
- API server and operator may be one minor version apart during upgrades
- v1 API compatibility maintained across all versions
- Feature gate status synchronized across components

### Version Skew Strategy

#### Operator and API Server Skew

During cluster upgrades, the API server may be updated before or after the ingress-operator:

**Scenario 1**: API server updated first (knows about new field), operator not yet updated
- New `resources` field accepted by API server
- Old operator version ignores the field (doesn't know about it yet)
- No impact, field configuration waits for operator upgrade

**Scenario 2**: Operator updated first (can process new field), API server not yet updated
- Operator can process `resources` field
- API server already knows about v1 API schema
- Field works immediately once feature gate is enabled

**Maximum skew**: 1 minor version (OpenShift standard)

### Operational Aspects of API Extensions

#### Failure Modes

1. **Invalid resource values**: 
   - Rejected by Kubernetes validation
   - User receives clear error message
   - Operator continues with existing configuration

2. **Controller failure**: 
   - Router deployment remains at current configuration
   - IngressController status reflects error
   - Operator logs provide debugging information

3. **Router pod restart loop due to low resources**:
   - Kubernetes backoff prevents rapid restarts
   - Events and logs indicate resource pressure (OOMKilled, etc.)
   - Admin can update IngressController to increase resource limits
   - Traffic may be degraded during restart loop

4. **Feature gate disabled but `spec.resources` field used**:
   - `spec.resources` field is ignored
   - Warning event logged
   - Falls back to hardcoded defaults
   - No traffic impact

#### Support Procedures

Standard OpenShift support procedures apply:

**Gathering debug information**:
```bash
# View IngressController configuration
oc get ingresscontroller default -n openshift-ingress-operator -o yaml

# View router deployment
oc describe deployment router-default -n openshift-ingress

# Check router pod resource configuration
oc get deployment router-default -n openshift-ingress -o jsonpath='{.spec.template.spec.containers[*].resources}'

# Check router pod resource usage
oc adm top pod -n openshift-ingress

# Check router pod QoS class
oc get pod -n openshift-ingress -l ingresscontroller.operator.openshift.io/deployment-ingresscontroller=default -o jsonpath='{.items[*].status.qosClass}'

# Check router pod events (for OOMKilled, etc.)
oc get events -n openshift-ingress --field-selector involvedObject.kind=Pod

# Check operator logs for reconciliation
oc logs -n openshift-ingress-operator deployment/ingress-operator -c ingress-operator | grep -i resource

# Check feature gate status
oc get featuregate cluster -o yaml | grep IngressRouterResourceLimits
```

**Common issues and resolutions**:
- **OOMKilled router pods**: Increase memory limits in `spec.resources.routerContainer`
- **CPU throttling**: Increase CPU limits or verify requests match actual load
- **Configuration not applied**: 
  - Check feature gate is enabled
  - Check operator logs for errors
  - Verify `spec.resources` field is properly set
- **Burstable QoS when Guaranteed expected**: Ensure limits equal requests
- **`spec.resources` field ignored**: Verify feature gate is enabled

## Implementation History

- 2025-10-27: Enhancement proposed
- TBD: Enhancement approved
- TBD: API implementation merged to openshift/api
- TBD: Controller implementation merged to cluster-ingress-operator
- TBD: Feature available in Tech Preview (target: OpenShift 4.X)
- TBD: Promotion to GA (target: OpenShift 4.Y, ~2 releases after Tech Preview)

## Alternatives

### Alternative 1: Configuration via ConfigMap

Use a ConfigMap for router pod resource configuration instead of API field.

**Pros**:
- Simpler to implement
- No API version changes needed
- Easy to update without CRD changes

**Cons**:
- Less type-safe
- Doesn't follow OpenShift patterns (IngressController is the proper API)
- No automatic validation
- Harder to discover and document
- Not GitOps friendly
- Separation from other router configuration

**Decision**: Rejected - API-based configuration via IngressController is the established OpenShift pattern

### Alternative 2: Separate CRD for ingress configuration

Create a new IngressConfiguration CRD separate from IngressController.

**Pros**:
- Separation of concerns (configuration vs. controller spec)
- Could handle other ingress-level configuration

**Cons**:
- Increases API surface unnecessarily
- IngressController is the logical place for router pod configuration
- More CRDs to manage
- Inconsistent with existing IngressController design patterns
- Confusing for users - where to configure what?

**Decision**: Rejected - IngressController CR is the appropriate location for router configuration

### Alternative 3: Router deployment annotations or environment variables

Configure router pod resources via deployment annotations or environment variables.

**Pros**:
- Very simple to implement
- No API changes needed

**Cons**:
- Not GitOps friendly
- Requires direct deployment modification (operator would overwrite)
- Not discoverable via API
- Doesn't follow OpenShift declarative configuration patterns
- Difficult to audit and version control
- Operator reconciliation would fight manual changes

**Decision**: Rejected - Declarative API configuration via IngressController is required

### Alternative 4: Use v1alpha1 API version instead of v1 with feature gate

Create a separate v1alpha1 API version for IngressController and add the new `resources` 
field there, following the pattern used by some other OpenShift components like cluster 
monitoring.

**Pros**:
- Clear separation between stable (v1) and experimental (v1alpha1) APIs
- Can iterate on API design during Tech Preview without affecting v1
- Field only visible in v1alpha1, clearer signal that it's experimental
- Can change or remove field design if Tech Preview reveals issues
- Follows pattern used by some OpenShift components

**Cons**:
- Adds API complexity with multiple versions
- Requires API conversion webhooks between v1 and v1alpha1
- Users must explicitly switch to v1alpha1 API to use the feature
- More maintenance burden (two API versions to support)
- Need to eventually promote changes back to v1 for GA
- Networking team prefers simpler approach of v1 with feature gate
- For straightforward features like this, v1 alpha1 adds unnecessary complexity
- Not all OpenShift APIs follow this pattern - many add fields to v1 with feature gates

**Decision**: Rejected in favor of adding field to v1 API with feature gate. It is the simpler 
approach that adds the field directly to v1 protected by a feature gate. 
This avoids API versioning complexity while still providing Tech Preview 
protection. For a straightforward additive feature like this, the additional complexity of 
v1alpha1 is not warranted.

## Infrastructure Needed

### Development Infrastructure

- Standard OpenShift CI/CD pipeline (already exists)
- No special hardware or cloud resources required

### Testing Infrastructure

- CI jobs for unit, integration, and E2E tests (leverage existing CI)
- Access to test clusters for manual testing (existing QE infrastructure)
- Performance testing environment for load testing (optional, future work)

### Documentation Infrastructure

- OpenShift documentation repository access
- Standard docs.openshift.com publishing pipeline

### Monitoring Infrastructure

- Existing operator metrics (no new infrastructure needed)
- Alert rules may be added in future iterations

## Dependencies

### Code Dependencies

- `github.com/openshift/api` - API definitions (will be updated)
- `k8s.io/api` - Kubernetes core types
- `sigs.k8s.io/controller-runtime` - Controller framework

### Team Dependencies

- **Ingress team**: Implementation and maintenance
- **API team**: API review and approval
- **Docs team**: Documentation
- **QE team**: Testing
- **ART team**: Release and build processes

