# Operator Lifecycle Management

**Category**: OpenShift Platform Pattern  
**Last Updated**: 2026-04-08  

## Overview

OpenShift operators follow a standardized lifecycle from installation through upgrades to removal. Understanding this lifecycle is essential for building robust operators.

## Lifecycle Stages

| Stage | Description | Key Operations |
|-------|-------------|----------------|
| **Installation** | Initial deployment via CVO | Create ClusterOperator, deploy resources |
| **Available** | Operator running normally | Reconcile resources, report status |
| **Upgrading** | Version change in progress | Coordinate with CVO, migrate resources |
| **Degraded** | Partial functionality | Report degraded condition, attempt recovery |
| **Removal** | Operator being deleted | Cleanup resources, remove finalizers |

## Installation Phase

### CVO-Managed Operators

```yaml
# Installed via ClusterVersion payload
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-operator
  namespace: openshift-my-operator
  annotations:
    release.openshift.io/version: "4.16.0"
```

**Process**:
1. CVO extracts operator manifest from release payload
2. CVO creates namespace, RBAC, deployment
3. Operator pod starts
4. Operator creates/updates ClusterOperator resource
5. Operator sets Available=True when ready

### OLM-Managed Operators

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: my-operator
  namespace: openshift-operators
spec:
  channel: stable
  name: my-operator
  source: certified-operators
```

**Process**:
1. OLM resolves dependencies from catalog
2. OLM creates OperatorGroup, CSV, deployment
3. Operator starts and becomes available

## Normal Operation

### Reconciliation Loop

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Get desired state
    config := &ConfigV1{}
    if err := r.Get(ctx, req.NamespacedName, config); err != nil {
        return ctrl.Result{}, err
    }
    
    // 2. Reconcile to desired state
    if err := r.ensureDeployment(ctx, config); err != nil {
        return ctrl.Result{}, err
    }
    
    // 3. Update status
    config.Status.ObservedGeneration = config.Generation
    if err := r.Status().Update(ctx, config); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

### Status Reporting

```yaml
status:
  conditions:
  - type: Available
    status: "True"
    reason: "AsExpected"
    message: "All components running"
    lastTransitionTime: "2024-01-15T10:00:00Z"
  - type: Progressing
    status: "False"
    reason: "AsExpected"
  - type: Degraded
    status: "False"
    reason: "AsExpected"
```

See [status-conditions.md](../operator-patterns/status-conditions.md) for pattern details.

## Upgrade Phase

### Pre-Upgrade

```go
// Operator detects upgrade starting
func (r *Reconciler) detectUpgrade() bool {
    currentVersion := os.Getenv("OPERATOR_IMAGE_VERSION")
    desiredVersion := r.releaseVersion
    return currentVersion != desiredVersion
}
```

### During Upgrade

**CVO coordinates upgrades in order**:
```
1. MCO (nodes first)
   ↓
2. API Server components
   ↓
3. Controller Manager, Scheduler
   ↓
4. Network operator
   ↓
5. Other operators (parallel when safe)
```

**Operator responsibilities**:
```go
func (r *Reconciler) handleUpgrade(ctx context.Context) error {
    // 1. Set Progressing=True
    r.setCondition(ProgressingCondition, True, "Upgrading", "...")
    
    // 2. Migrate resources if needed
    if err := r.migrateResources(ctx); err != nil {
        r.setCondition(DegradedCondition, True, "MigrationFailed", err.Error())
        return err
    }
    
    // 3. Update managed resources
    if err := r.updateDeployments(ctx); err != nil {
        return err
    }
    
    // 4. Wait for rollout
    if !r.deploymentReady() {
        return fmt.Errorf("waiting for deployment")
    }
    
    // 5. Set Available=True, Progressing=False
    r.setCondition(AvailableCondition, True, "UpgradeComplete", "...")
    r.setCondition(ProgressingCondition, False, "AsExpected", "")
    
    return nil
}
```

### Version Skew Handling

```go
// Support N and N-1 API versions during upgrade
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &MyResourceV2{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        // Try v1 if v2 not found
        objV1 := &MyResourceV1{}
        if err := r.Get(ctx, req.NamespacedName, objV1); err != nil {
            return ctrl.Result{}, err
        }
        obj = convertV1ToV2(objV1)
    }
    
    // Reconcile with v2
    return r.reconcileV2(ctx, obj)
}
```

## Degraded State

### Detecting Degradation

```go
func (r *Reconciler) checkHealth(ctx context.Context) error {
    // Check critical resources
    deployment := &appsv1.Deployment{}
    if err := r.Get(ctx, types.NamespacedName{
        Name: "my-operator",
        Namespace: "openshift-my-operator",
    }, deployment); err != nil {
        return fmt.Errorf("deployment missing: %w", err)
    }
    
    // Check availability
    if deployment.Status.AvailableReplicas < 1 {
        return fmt.Errorf("no available replicas")
    }
    
    return nil
}
```

### Reporting Degraded

```yaml
status:
  conditions:
  - type: Degraded
    status: "True"
    reason: "DeploymentUnavailable"
    message: "operator deployment has 0 available replicas"
  - type: Available
    status: "False"
    reason: "DeploymentUnavailable"
```

### Recovery

```go
func (r *Reconciler) attemptRecovery(ctx context.Context) error {
    // 1. Log degradation
    log.Error(err, "Operator degraded, attempting recovery")
    
    // 2. Try to fix
    if err := r.ensureDeployment(ctx); err != nil {
        return err
    }
    
    // 3. Verify recovery
    if err := r.checkHealth(ctx); err != nil {
        return err // Still degraded
    }
    
    // 4. Clear degraded condition
    r.setCondition(DegradedCondition, False, "Recovered", "...")
    return nil
}
```

## Removal Phase

### Cleanup with Finalizers

```go
const myOperatorFinalizer = "my-operator.openshift.io/cleanup"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Handle deletion
    if !obj.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(obj, myOperatorFinalizer) {
            // Cleanup
            if err := r.cleanup(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
            
            // Remove finalizer
            controllerutil.RemoveFinalizer(obj, myOperatorFinalizer)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }
    
    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(obj, myOperatorFinalizer) {
        controllerutil.AddFinalizer(obj, myOperatorFinalizer)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    // Normal reconciliation
    return r.reconcile(ctx, obj)
}
```

See [finalizers.md](../operator-patterns/finalizers.md) for pattern details.

## Best Practices

### Installation
- Create ClusterOperator immediately on startup
- Set Available=False until fully initialized
- Use init containers for prerequisites

### Normal Operation
- Report accurate status conditions
- Handle transient failures gracefully
- Implement exponential backoff for retries

### Upgrades
- Support N-1 version compatibility
- Migrate resources before updating
- Set Progressing=True during upgrade
- Test upgrades in CI

### Degraded State
- Report specific reason in condition message
- Attempt automatic recovery when possible
- Preserve user data even when degraded

### Removal
- Use finalizers for critical cleanup
- Don't block deletion indefinitely
- Log cleanup failures but don't retry forever

## Examples by Component

| Component | Installation | Upgrade Pattern | Cleanup |
|-----------|-------------|-----------------|---------|
| machine-config-operator | CVO-managed | Rolling node updates | None (core) |
| cluster-network-operator | CVO-managed | Coordinated with MCO | None (core) |
| console-operator | CVO-managed | Deployment rollout | Remove extensions |
| cert-manager-operator | OLM-managed | CSV update | Revoke certificates |

## References

- **Status Conditions**: [status-conditions.md](../operator-patterns/status-conditions.md)
- **Upgrade Strategies**: [upgrade-strategies.md](../operator-patterns/upgrade-strategies.md)
- **Finalizers**: [finalizers.md](../operator-patterns/finalizers.md)
- **ClusterOperator**: [cluster-operators.md](./cluster-operators.md)
