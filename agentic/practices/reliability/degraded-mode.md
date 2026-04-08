# Degraded Mode Handling

**Category**: Engineering Practice  
**Applies To**: All OpenShift ClusterOperators  
**Last Updated**: 2026-04-08  

## Overview

Operators should gracefully handle partial failures and continue providing functionality where possible, rather than failing completely.

## Degraded vs Failed

| State | Meaning | Example |
|-------|---------|---------|
| **Degraded** | Partial functionality | 1 of 3 replicas down |
| **Failed/Unavailable** | No functionality | All replicas down |

## Reporting Degraded State

### ClusterOperator Conditions

```yaml
status:
  conditions:
  - type: Available
    status: "True"  # Still functioning
  - type: Degraded
    status: "True"  # But degraded
    reason: "DeploymentReplicasNotAvailable"
    message: "deployment my-operator has 1 of 3 replicas available"
  - type: Progressing
    status: "False"
```

### When to Set Degraded

```go
func (r *Reconciler) updateStatus(ctx context.Context) error {
    deployment := &appsv1.Deployment{}
    err := r.Get(ctx, types.NamespacedName{
        Name:      "my-operator",
        Namespace: "openshift-my-operator",
    }, deployment)
    
    if err != nil {
        // Critical resource missing - Degraded
        r.setCondition(DegradedCondition, metav1.ConditionTrue,
            "DeploymentMissing",
            "Required deployment not found")
        r.setCondition(AvailableCondition, metav1.ConditionFalse,
            "DeploymentMissing", "")
        return err
    }
    
    // Check replica availability
    desiredReplicas := *deployment.Spec.Replicas
    availableReplicas := deployment.Status.AvailableReplicas
    
    if availableReplicas == 0 {
        // No replicas - Unavailable
        r.setCondition(AvailableCondition, metav1.ConditionFalse,
            "NoReplicasAvailable", "")
        r.setCondition(DegradedCondition, metav1.ConditionTrue,
            "NoReplicasAvailable",
            "deployment has 0 available replicas")
    } else if availableReplicas < desiredReplicas {
        // Some replicas - Degraded but Available
        r.setCondition(AvailableCondition, metav1.ConditionTrue,
            "AsExpected", "")
        r.setCondition(DegradedCondition, metav1.ConditionTrue,
            "DeploymentReplicasNotAvailable",
            fmt.Sprintf("deployment has %d of %d replicas available",
                availableReplicas, desiredReplicas))
    } else {
        // All replicas - Healthy
        r.setCondition(AvailableCondition, metav1.ConditionTrue,
            "AsExpected", "")
        r.setCondition(DegradedCondition, metav1.ConditionFalse,
            "AsExpected", "")
    }
    
    return nil
}
```

## Graceful Degradation Patterns

### Pattern 1: Reduced Capacity

Continue with reduced replicas:

```go
if deployment.Status.AvailableReplicas > 0 {
    // Can still reconcile, just slower
    log.Info("Running with reduced capacity",
        "available", deployment.Status.AvailableReplicas,
        "desired", *deployment.Spec.Replicas)
    return ctrl.Result{}, nil
}
```

### Pattern 2: Feature Degradation

Disable non-critical features:

```go
func (r *Reconciler) reconcile(ctx context.Context) error {
    // Always do critical work
    if err := r.ensureCriticalResources(ctx); err != nil {
        return err
    }
    
    // Optional: Nice-to-have features
    if r.isHealthy() {
        r.optimizeResources(ctx)
        r.cleanupOldResources(ctx)
    } else {
        log.Info("Skipping optimization due to degraded state")
    }
    
    return nil
}
```

### Pattern 3: Cached Data

Use stale data if fresh data unavailable:

```go
func (r *Reconciler) getConfig(ctx context.Context) (*Config, error) {
    // Try to fetch fresh config
    config, err := r.fetchConfigFromAPI(ctx)
    if err != nil {
        log.Error(err, "Failed to fetch config, using cached version")
        r.setCondition(DegradedCondition, metav1.ConditionTrue,
            "UsingCachedConfig",
            "Unable to fetch fresh config from API")
        
        // Use cached config if available
        if r.cachedConfig != nil {
            return r.cachedConfig, nil
        }
        return nil, err
    }
    
    // Update cache and clear degraded
    r.cachedConfig = config
    r.setCondition(DegradedCondition, metav1.ConditionFalse,
        "AsExpected", "")
    return config, nil
}
```

### Pattern 4: Retry with Backoff

Don't fail permanently on transient errors:

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    err := r.reconcile(ctx, req)
    if err != nil {
        // Check if transient
        if isTransient(err) {
            // Exponential backoff
            log.Info("Transient error, will retry",
                "error", err,
                "retryAfter", "1m")
            r.setCondition(DegradedCondition, metav1.ConditionTrue,
                "TransientError",
                fmt.Sprintf("Temporary failure: %v", err))
            return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
        }
        
        // Permanent error
        r.setCondition(DegradedCondition, metav1.ConditionTrue,
            "ReconcileFailed",
            err.Error())
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

## Automatic Recovery

### Self-Healing

```go
func (r *Reconciler) attemptRecovery(ctx context.Context) error {
    log.Info("Attempting automatic recovery")
    
    // 1. Identify problem
    deployment := &appsv1.Deployment{}
    err := r.Get(ctx, types.NamespacedName{
        Name:      "my-operator",
        Namespace: "openshift-my-operator",
    }, deployment)
    if err != nil {
        // Recreate missing deployment
        log.Info("Deployment missing, recreating")
        return r.createDeployment(ctx)
    }
    
    // 2. Check if degraded
    if deployment.Status.AvailableReplicas == 0 {
        // Try to identify failing pods
        pods, err := r.getFailedPods(ctx)
        if err != nil {
            return err
        }
        
        // Delete crashlooping pods to trigger restart
        for _, pod := range pods {
            if pod.Status.ContainerStatuses[0].RestartCount > 5 {
                log.Info("Deleting crashlooping pod", "pod", pod.Name)
                r.Delete(ctx, &pod)
            }
        }
    }
    
    return nil
}
```

### Recovery Verification

```go
func (r *Reconciler) verifyRecovery(ctx context.Context) (bool, error) {
    // Check if still degraded after recovery attempt
    deployment := &appsv1.Deployment{}
    err := r.Get(ctx, types.NamespacedName{
        Name:      "my-operator",
        Namespace: "openshift-my-operator",
    }, deployment)
    if err != nil {
        return false, err
    }
    
    if deployment.Status.AvailableReplicas >= *deployment.Spec.Replicas {
        log.Info("Recovery successful")
        r.setCondition(DegradedCondition, metav1.ConditionFalse,
            "Recovered", "Automatic recovery successful")
        return true, nil
    }
    
    log.Info("Still degraded after recovery attempt",
        "available", deployment.Status.AvailableReplicas,
        "desired", *deployment.Spec.Replicas)
    return false, nil
}
```

## Degradation Causes

### Common Causes

| Cause | Detection | Recovery |
|-------|-----------|----------|
| **Resource contention** | High CPU/memory | Scale up, reduce load |
| **Network issues** | Connection timeouts | Retry with backoff |
| **Missing dependencies** | API errors | Wait for dependency |
| **Configuration errors** | Validation failures | Fix config, rollback |
| **Node failures** | Pod evictions | Reschedule pods |

### Detection Examples

```go
// Resource contention
if pod.Status.Phase == corev1.PodFailed {
    for _, status := range pod.Status.ContainerStatuses {
        if status.State.Terminated != nil &&
           status.State.Terminated.Reason == "OOMKilled" {
            return fmt.Errorf("pod OOMKilled - insufficient memory")
        }
    }
}

// Network issues
if errors.Is(err, context.DeadlineExceeded) {
    return fmt.Errorf("network timeout")
}

// Missing dependencies
if errors.IsNotFound(err) {
    return fmt.Errorf("dependency not found: %w", err)
}
```

## User Communication

### Clear Messages

```yaml
# Bad
status:
  conditions:
  - type: Degraded
    status: "True"
    reason: "Error"
    message: "something wrong"

# Good
status:
  conditions:
  - type: Degraded
    status: "True"
    reason: "DeploymentReplicasNotAvailable"
    message: "deployment my-operator has 1 of 3 replicas available due to insufficient CPU on nodes"
```

### Actionable Information

```go
// Include what user can do
message := fmt.Sprintf(
    "deployment %s has %d of %d replicas available. "+
    "Check pod status: oc get pods -n %s",
    deployment.Name,
    deployment.Status.AvailableReplicas,
    *deployment.Spec.Replicas,
    deployment.Namespace,
)
```

## Examples

| Component | Degraded Scenario | Behavior |
|-----------|------------------|----------|
| machine-config-operator | Some nodes failing update | Continue updating other nodes |
| cluster-network-operator | SDN pods not ready | Network still works with reduced redundancy |
| cluster-version-operator | One operator degraded | Continue monitoring, block upgrades |

## Testing Degraded Scenarios

```go
func TestDegradedMode(t *testing.T) {
    // Create deployment with 0 replicas
    deployment := &appsv1.Deployment{
        Spec: appsv1.DeploymentSpec{
            Replicas: int32Ptr(3),
        },
        Status: appsv1.DeploymentStatus{
            AvailableReplicas: 1,  // Degraded
        },
    }
    
    // Reconcile
    result, err := reconciler.Reconcile(ctx, req)
    
    // Verify degraded condition set
    co := &configv1.ClusterOperator{}
    client.Get(ctx, types.NamespacedName{Name: "my-operator"}, co)
    
    degradedCond := getCondition(co.Status.Conditions, "Degraded")
    if degradedCond.Status != metav1.ConditionTrue {
        t.Error("Expected Degraded=True")
    }
    
    // Should still be Available
    availableCond := getCondition(co.Status.Conditions, "Available")
    if availableCond.Status != metav1.ConditionTrue {
        t.Error("Expected Available=True even when degraded")
    }
}
```

## References

- **Status Conditions**: [../platform/operator-patterns/status-conditions.md](../../platform/operator-patterns/status-conditions.md)
- **SLO Framework**: [slo-framework.md](./slo-framework.md)
- **Observability**: [observability.md](./observability.md)
