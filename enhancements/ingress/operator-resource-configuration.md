---
title: ingress-operator-resource-configuration
authors:
  - "@jortizpa"
reviewers:
  - "@Miciah"
  - "@frobware"
  - "@candita"
  - "@danehans"
approvers:
  - "@deads2k"
api-approvers:
  - "@JoelSpeed"
  - "@deads2k"
creation-date: 2025-01-14
last-updated: 2025-01-14
tracking-link:
  - https://issues.redhat.com/browse/RFE-1476
see-also:
  - "/enhancements/monitoring/cluster-monitoring-config.md"
replaces: []
superseded-by: []
---

# Ingress Operator Resource Configuration

## Summary

This enhancement proposes adding the ability to configure resource limits and 
requests for the ingress-operator deployment containers via a new v1alpha1 API 
field in the IngressController custom resource.

## Motivation

Currently, the ingress-operator deployment has hardcoded resource requests 
(CPU: 10m, Memory: 56Mi for the main container, and CPU: 10m, Memory: 40Mi for 
the kube-rbac-proxy sidecar) with no resource limits defined. This presents 
challenges for:

1. **Clusters with resource constraints**: Cannot guarantee QoS guarantees without limits
2. **Large-scale deployments**: May need higher resource allocation
3. **Compliance requirements**: Some organizations require all pods have limits
4. **Resource accounting**: Better cost allocation and resource planning

Related to [RFE-1476](https://issues.redhat.com/browse/RFE-1476).

### User Stories

#### Story 1: Platform Administrator Needs Resource Limits

As a platform administrator, I want to set resource limits on the ingress-operator 
to ensure it has a QoS class of "Guaranteed" for critical cluster infrastructure.

Acceptance Criteria:
- Can specify resource limits via IngressController CR
- Operator pod reflects the configured limits
- Pod achieves QoS class "Guaranteed" when requests == limits

#### Story 2: Large Cluster Operator

As an operator of a large-scale cluster with thousands of routes, I need to 
increase the ingress-operator's resource allocation to handle the increased load 
from managing many IngressController instances and routes.

Acceptance Criteria:
- Can configure higher CPU and memory allocations
- Operator performs adequately under high load
- Configuration survives operator restarts and upgrades

#### Story 3: Compliance Requirements

As a compliance officer, I need all pods in my OpenShift cluster to have both 
resource requests and limits defined for auditing, cost allocation, and capacity 
planning purposes.

Acceptance Criteria:
- All operator containers can have limits configured
- Configuration is auditable via oc commands
- Meets organizational policy requirements

### Goals

- Allow configuration of resource requests and limits for ingress-operator containers
- Follow established patterns from cluster monitoring configuration
- Maintain backward compatibility with existing IngressController v1 API
- Use v1alpha1 API version for this Tech Preview feature
- Provide sensible defaults that work for most deployments
- Support both the ingress-operator and kube-rbac-proxy containers

### Non-Goals

- Configuring resources for router pods (these are separate workloads managed by the operator)
- Auto-scaling or dynamic resource adjustment based on load
- Configuring resources for other operators in the cluster
- Modifying the v1 API (stable API remains unchanged)
- Vertical Pod Autoscaler (VPA) integration (may be future work)

## Proposal

### Workflow Description

**Platform Administrator** is a human responsible for configuring the OpenShift cluster.

1. Platform administrator creates or updates an IngressController CR using the 
   v1alpha1 API version
2. Platform administrator sets the `operatorResourceRequirements` field with 
   desired resource limits/requests
3. The ingress-operator watches for changes to the IngressController CR
4. A new operator deployment controller reconciles the operator's own deployment 
   with the specified resources
5. Kubernetes performs a rolling restart of the operator pods with the new resource configuration
6. Platform administrator verifies the changes with `oc describe deployment ingress-operator -n openshift-ingress-operator`

### API Extensions

Create a new v1alpha1 API version for IngressController in the 
`operator.openshift.io` group, following the pattern established by 
[cluster monitoring v1alpha1 configuration](https://github.com/openshift/api/blob/94481d71bb6f3ce6019717ea7900e6f88f42fa2c/config/v1alpha1/types_cluster_monitoring.go#L172-L193).

#### New API Types

```go
package v1alpha1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    corev1 "k8s.io/api/core/v1"
    
    operatorv1 "github.com/openshift/api/operator/v1"
)

// IngressController describes a managed ingress controller for the cluster.
// This is a v1alpha1 Tech Preview API that extends the v1 API with additional
// configuration options.
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason.
// +openshift:compatibility-gen:level=4
type IngressController struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    // spec is the specification of the desired behavior of the IngressController.
    Spec IngressControllerSpec `json:"spec,omitempty"`
    
    // status is the most recently observed status of the IngressController.
    Status operatorv1.IngressControllerStatus `json:"status,omitempty"`
}

// IngressControllerSpec extends the v1 IngressControllerSpec with v1alpha1 fields.
type IngressControllerSpec struct {
    // Embed the entire v1 spec for backwards compatibility
    operatorv1.IngressControllerSpec `json:",inline"`

    // operatorResourceRequirements defines resource requirements for the
    // ingress operator's own containers (not the router pods managed by the operator).
    // This allows configuring CPU and memory limits/requests for the operator deployment.
    //
    // When not specified, the operator uses default resource requirements:
    //   ingress-operator container: requests(cpu: 10m, memory: 56Mi), limits(cpu: 10m, memory: 56Mi)
    //   kube-rbac-proxy container: requests(cpu: 10m, memory: 40Mi), limits(cpu: 10m, memory: 40Mi)
    //
    // Note: Changing these values will cause the ingress-operator pod to restart.
    //
    // +optional
    // +openshift:enable:FeatureGate=IngressOperatorResourceManagement
    OperatorResourceRequirements *OperatorResourceRequirements `json:"operatorResourceRequirements,omitempty"`
}

// OperatorResourceRequirements defines resource requirements for ingress operator containers.
// Similar to the pattern used in cluster monitoring configuration.
type OperatorResourceRequirements struct {
    // ingressOperatorContainer specifies resource requirements for the
    // ingress-operator container in the operator deployment.
    //
    // If not specified, defaults to:
    //   requests: cpu: 10m, memory: 56Mi
    //   limits: cpu: 10m, memory: 56Mi
    //
    // +optional
    IngressOperatorContainer *corev1.ResourceRequirements `json:"ingressOperatorContainer,omitempty"`

    // kubeRbacProxyContainer specifies resource requirements for the
    // kube-rbac-proxy sidecar container in the operator deployment.
    //
    // If not specified, defaults to:
    //   requests: cpu: 10m, memory: 40Mi
    //   limits: cpu: 10m, memory: 40Mi
    //
    // +optional
    KubeRbacProxyContainer *corev1.ResourceRequirements `json:"kubeRbacProxyContainer,omitempty"`
}
```

#### Example Usage

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  # All existing v1 fields continue to work
  replicas: 2
  domain: apps.example.com
  
  # New v1alpha1 field for operator resource configuration
  operatorResourceRequirements:
    ingressOperatorContainer:
      requests:
        cpu: 20m
        memory: 100Mi
      limits:
        cpu: 100m
        memory: 200Mi
    kubeRbacProxyContainer:
      requests:
        cpu: 10m
        memory: 40Mi
      limits:
        cpu: 50m
        memory: 80Mi
```

#### API Validation

The following validations will be enforced:

1. **Resource limits must be >= requests**: Kubernetes standard validation
2. **Minimum values** (recommendations, not enforced):
   - ingress-operator container: cpu >= 10m, memory >= 56Mi
   - kube-rbac-proxy container: cpu >= 10m, memory >= 40Mi
3. **API conversion**: v1alpha1-specific fields are dropped when converting to v1

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift environments:
- The management cluster runs the ingress-operator for the management cluster
- Each hosted cluster's control plane runs its own ingress-operator
- This enhancement applies to both contexts independently
- Configuration is specific to each IngressController instance

#### Standalone Clusters

Standard behavior - configuration applies to the cluster's ingress-operator deployment 
in the `openshift-ingress-operator` namespace.

#### Single-node Deployments

Particularly beneficial for single-node OpenShift (SNO) deployments where:
- Resource constraints are tighter
- Setting appropriate limits helps prevent resource contention
- Guaranteed QoS class improves stability

### Implementation Details/Notes/Constraints

#### API Versioning Strategy

- **v1 API**: Remains stable and unchanged (storage version)
- **v1alpha1 API**: Served but not stored
- **Conversion**: Automatic conversion between versions via conversion webhooks
- **Field handling**: v1alpha1-specific fields are dropped when reading via v1 API
- **Compatibility**: Existing v1 clients continue working without changes

#### Controller Implementation

A new controller (`operator-deployment-controller`) in the cluster-ingress-operator 
watches the default IngressController CR and reconciles the operator's own deployment 
when `operatorResourceRequirements` is specified.

**Controller responsibilities:**
1. Watch IngressController resources (v1alpha1)
2. Reconcile `ingress-operator` Deployment in `openshift-ingress-operator` namespace
3. Update container resource specifications
4. Handle error cases gracefully (invalid values, conflicts, etc.)

#### Default Behavior

When `operatorResourceRequirements` is not set or when using the v1 API:

**Current state** (what exists now):
- ingress-operator container: requests only (cpu: 10m, memory: 56Mi), no limits
- kube-rbac-proxy container: requests only (cpu: 10m, memory: 40Mi), no limits

**New default** (after this enhancement):
- Static manifest updated to include limits matching requests
- ingress-operator container: requests(cpu: 10m, memory: 56Mi), limits(cpu: 10m, memory: 56Mi)
- kube-rbac-proxy container: requests(cpu: 10m, memory: 40Mi), limits(cpu: 10m, memory: 40Mi)
- This provides QoS class "Guaranteed" by default

#### Upgrade Behavior

When upgrading to a version with this enhancement:
1. Existing deployments get updated manifests with new default limits
2. IngressController CRs remain at v1 unless explicitly changed
3. No user action required for default behavior
4. Users can opt-in to v1alpha1 to customize resources

### Risks and Mitigations

#### Risk: User sets resources too low, operator becomes unhealthy

**Impact**: Operator may OOMKill, fail to reconcile, or become unresponsive

**Mitigation**:
- Document minimum recommended values
- Add validation warnings (not blocking) for values below minimums
- Include troubleshooting guide for common issues
- Monitor operator health metrics

**Likelihood**: Medium

#### Risk: Incompatibility with existing tooling expecting v1 API only

**Impact**: External tools may not recognize v1alpha1 resources

**Mitigation**:
- v1 API remains unchanged and fully functional
- v1alpha1 is opt-in
- Document migration path
- Conversion webhooks ensure cross-version compatibility

**Likelihood**: Low

#### Risk: Operator restart causes brief unavailability

**Impact**: Configuration changes trigger pod restart, brief reconciliation delay

**Mitigation**:
- Document that changes trigger rolling restart (expected behavior)
- Operator restart is typically < 30 seconds
- Router pods continue serving traffic during operator restart
- Changes to operator resources are not expected to be frequent

**Likelihood**: High (by design), **Severity**: Low

#### Risk: Resource configuration drift

**Impact**: Manual changes to deployment could be overwritten by controller

**Mitigation**:
- Controller reconciliation loop detects and corrects drift
- Document that configuration should be via IngressController CR, not direct deployment edits
- Admission webhooks prevent direct deployment modifications

**Likelihood**: Low

### Drawbacks

1. **Increased API complexity**: Adds another version and configuration surface
2. **Maintenance burden**: Requires maintaining v1alpha1 API version and conversion logic
3. **Operator self-modification**: Operator modifying its own deployment adds complexity
4. **Documentation overhead**: Need to document new field and migration path
5. **Testing complexity**: Must test version conversion and upgrade scenarios

## Design Details

### Open Questions

1. **Q**: Should we support auto-scaling (VPA) in the future?
   - **A**: Out of scope for initial implementation, but API should not preclude it

2. **Q**: Should we add validation for minimum resource values?
   - **A**: Start with warnings/documentation, consider hard validation if issues arise

3. **Q**: Should this apply to all IngressControllers or only the default?
   - **A**: Initial implementation only default, but API supports any IngressController

4. **Q**: How do we handle the operator modifying its own deployment safely?
   - **A**: Use owner references carefully, reconcile loop with backoff

### Test Plan

#### Unit Tests

- **API conversion tests**: v1 ↔ v1alpha1 conversion correctness
- **Controller reconciliation logic**: Mock deployment updates
- **Resource requirement validation**: Edge cases and invalid inputs
- **Default value handling**: Ensure defaults applied correctly

Coverage target: >80% for new code

#### Integration Tests

- **API server integration**: v1alpha1 CRD registration and serving
- **Conversion webhook**: Automatic conversion between versions
- **Controller watches**: IngressController changes trigger reconciliation

#### E2E Tests

- **Create IngressController with operatorResourceRequirements**
  - Verify operator deployment is updated with correct resources
  - Verify operator continues functioning normally
  
- **Update existing IngressController to add resource requirements**
  - Verify rolling update occurs
  - Verify no disruption to router functionality
  
- **Remove resource requirements (revert to defaults)**
  - Verify deployment reverts to default values
  
- **Upgrade scenario tests**
  - Upgrade from version without feature to version with feature
  - Verify existing IngressControllers continue working
  - Verify v1 API remains functional
  
- **Downgrade scenario tests**
  - Downgrade from version with v1alpha1 to version without
  - Verify graceful degradation (v1alpha1 fields ignored)

#### Manual Testing

- Test in resource-constrained environments (e.g., single-node)
- Verify QoS class changes as expected (None → Burstable → Guaranteed)
- Test with various resource configurations (very low, very high)
- Test operator behavior when limits are hit (OOMKill, CPU throttling)
- Test with multiple IngressController instances

### Graduation Criteria

#### Dev Preview -> Tech Preview (v1alpha1)

- [x] Feature implemented behind feature gate
- [x] Unit and integration tests passing
- [x] E2E tests passing in CI
- [x] Documentation published in OpenShift docs
- [x] Enhancement proposal approved
- [ ] Feedback collected from at least 3 early adopters
- [ ] Known issues documented

#### Tech Preview -> GA (promotion to v1)

This section describes criteria for graduating from v1alpha1 to v1 (stable API).

- [ ] Sufficient field testing (2+ minor releases in Tech Preview)
- [ ] No major bugs reported for 2 consecutive releases
- [ ] Performance impact assessed and documented
- [ ] API design validated by diverse user scenarios
- [ ] At least 10 production users providing positive feedback
- [ ] All tests consistently passing
- [ ] Documentation complete and reviewed
- [ ] Upgrade/downgrade tested extensively
- [ ] API review completed and approved for promotion

Timeline estimate: 6-12 months after Tech Preview release

#### Removing a deprecated feature

N/A - this is a new feature

### Upgrade / Downgrade Strategy

#### Upgrade

**From version without feature → version with feature:**

1. CRD updated to include v1alpha1 version
2. Existing IngressController CRs remain at v1 (storage version)
3. Operator deployment updated with default resource limits
4. Users can opt-in to v1alpha1 API to customize resources
5. No breaking changes to existing functionality

**User action required**: None for default behavior

**User action optional**: Update to v1alpha1 API to customize operator resources

#### Downgrade

**From version with feature → version without feature:**

1. v1alpha1 API becomes unavailable
2. IngressController CRs remain at v1 (storage version, unaffected)
3. v1alpha1-specific fields (operatorResourceRequirements) are ignored
4. Operator deployment falls back to static manifest defaults
5. No data loss as v1 remains storage version

**User impact**: Loss of custom operator resource configuration, reverts to defaults

#### Version Skew

Supported version skew follows standard OpenShift practices:
- API server and operator may be one minor version apart during upgrades
- v1 API compatibility maintained across all versions
- Conversion webhooks handle any necessary translations

### Version Skew Strategy

#### Operator and API Server Skew

During cluster upgrades, the API server may be updated before or after the ingress-operator:

**Scenario 1**: API server updated first (has v1alpha1), operator not yet updated
- v1alpha1 CRs accepted by API server
- Old operator version ignores v1alpha1 fields (reads via v1 API)
- No impact, custom resources wait for operator upgrade

**Scenario 2**: Operator updated first (supports v1alpha1), API server not yet updated
- Operator can handle v1alpha1 resources
- API server doesn't serve v1alpha1 yet
- Users continue using v1 API until API server updates

**Maximum skew**: 1 minor version (OpenShift standard)

### Operational Aspects of API Extensions

#### Failure Modes

1. **Invalid resource values**: 
   - Rejected by Kubernetes validation
   - User receives clear error message
   - Operator continues with existing configuration

2. **Controller failure**: 
   - Operator deployment remains at current configuration
   - Deployment status reflects error
   - Operator logs provide debugging information

3. **API conversion failure**: 
   - Request fails with error message
   - User notified of conversion issue
   - Existing resources unaffected

4. **Operator restart loop due to low resources**:
   - Kubernetes backoff prevents rapid restarts
   - Events and logs indicate resource pressure
   - Admin can update IngressController to increase resources

#### Support Procedures

Standard OpenShift support procedures apply:

**Gathering debug information**:
```bash
# View IngressController configuration
oc get ingresscontroller default -n openshift-ingress-operator -o yaml

# View operator deployment
oc describe deployment ingress-operator -n openshift-ingress-operator

# Check operator logs
oc logs -n openshift-ingress-operator deployment/ingress-operator -c ingress-operator

# Check pod resource usage
oc adm top pod -n openshift-ingress-operator

# Check QoS class
oc get pod -n openshift-ingress-operator -o jsonpath='{.items[*].status.qosClass}'
```

**Common issues and resolutions**:
- OOMKilled operator: Increase memory limits
- CPU throttling: Increase CPU limits or reduce requests if not needed
- Configuration not applied: Check operator logs for reconciliation errors

## Implementation History

- 2025-01-14: Enhancement proposed
- TBD: Enhancement approved
- TBD: API implementation merged to openshift/api
- TBD: Controller implementation merged to cluster-ingress-operator
- TBD: Feature available in Tech Preview (target: OpenShift 4.X)
- TBD: Promotion to GA (target: OpenShift 4.Y, ~2 releases after Tech Preview)

## Alternatives

### Alternative 1: Configuration via ConfigMap

Use a ConfigMap for operator resource configuration instead of API field.

**Pros**:
- Simpler to implement
- No API version changes needed
- Easy to update without CRD changes

**Cons**:
- Less type-safe
- Doesn't follow OpenShift patterns
- No automatic validation
- Harder to discover and document

**Decision**: Rejected - API-based configuration is the established OpenShift pattern

### Alternative 2: Modify v1 API directly

Add `operatorResourceRequirements` field directly to stable v1 API.

**Pros**:
- No need for v1alpha1 version
- Simpler for users (one API version)

**Cons**:
- Changes stable API (breaking compatibility promise)
- Cannot iterate on design easily
- Difficult to remove if issues found
- Against OpenShift API stability guarantees

**Decision**: Rejected - Use v1alpha1 for new features as per OpenShift conventions

### Alternative 3: Separate CRD for operator configuration

Create a new OperatorConfiguration CRD (similar to how cluster monitoring works).

**Pros**:
- Separation of concerns
- Can configure multiple operators uniformly

**Cons**:
- Increases API surface unnecessarily
- IngressController is the logical place for ingress-operator configuration
- More CRDs to manage
- Inconsistent with how other operators handle self-configuration

**Decision**: Rejected - IngressController CR is the appropriate configuration location

### Alternative 4: Operator command-line flags or environment variables

Configure operator resources via deployment environment variables or command flags.

**Pros**:
- Very simple to implement
- No API changes needed

**Cons**:
- Not GitOps friendly
- Requires direct deployment modification
- Not discoverable via API
- Doesn't follow OpenShift declarative configuration patterns
- Difficult to audit and version control

**Decision**: Rejected - Declarative API configuration is required

### Alternative 5: Use OperatorHub/OLM configuration

Leverage Operator Lifecycle Manager (OLM) subscription configuration.

**Pros**:
- Follows OLM patterns
- Could work for OLM-managed operators

**Cons**:
- Ingress operator is not OLM-managed (it's a cluster operator)
- Adds OLM dependency
- Not applicable to this operator's deployment model

**Decision**: Rejected - Not applicable to cluster operators

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

