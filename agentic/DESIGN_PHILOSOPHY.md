# OpenShift Design Philosophy

**Purpose**: Core principles guiding OpenShift architecture and development

**Last Updated**: 2026-04-08

---

## Table of Contents

1. [Kubernetes Foundation](#kubernetes-foundation)
2. [The Operator Pattern](#the-operator-pattern)
3. [Immutable Infrastructure](#immutable-infrastructure)
4. [API-First Design](#api-first-design)
5. [Declarative Over Imperative](#declarative-over-imperative)
6. [Upgrade Safety](#upgrade-safety)
7. [Observability by Default](#observability-by-default)

---

## Kubernetes Foundation

### Desired State vs Current State

**Core Principle**: Describe what you want, Kubernetes makes it happen.

```
User declares:  "I want 3 replicas"  (Desired State)
Kubernetes sees: "I have 1 replica"  (Current State)
Controller:     "I'll create 2 more" (Reconciliation)
```

**Why This Matters**:
- Self-healing: If a pod dies, controller recreates it
- Idempotent: Applying same config multiple times = same result
- Eventual consistency: System converges to desired state

**In OpenShift**: Every operator follows this pattern.

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Read desired state (spec)
    obj := &MyResource{}
    r.Get(ctx, req.NamespacedName, obj)
    
    // 2. Read current state (deployed resources)
    deployment := &appsv1.Deployment{}
    r.Get(ctx, deploymentName, deployment)
    
    // 3. Reconcile: Make current match desired
    if deployment.Spec.Replicas != obj.Spec.Replicas {
        deployment.Spec.Replicas = obj.Spec.Replicas
        r.Update(ctx, deployment)
    }
    
    // 4. Update status (observed state)
    obj.Status.Replicas = deployment.Status.ReadyReplicas
    r.Status().Update(ctx, obj)
    
    return ctrl.Result{}, nil
}
```

**References**: [controller-runtime.md](./platform/operator-patterns/controller-runtime.md)

---

## The Operator Pattern

### Why Operators?

**Problem**: Complex applications need domain-specific operational knowledge.

**Traditional Approach**: Runbooks, manual procedures, scripts
- Human must interpret state
- Error-prone
- Doesn't scale
- Knowledge in people's heads

**Operator Approach**: Encode operational knowledge in software
- Software interprets state
- Automated
- Scales to 1000s of clusters
- Knowledge in code

### Operator = Controller + Custom Resource

```
Custom Resource (API)  →  Describes "what"
      ↓
Operator (Controller)  →  Implements "how"
```

**Example**: Database operator

```yaml
# User declares WHAT they want
apiVersion: database.example.com/v1
kind: PostgreSQL
spec:
  version: "14"
  replicas: 3
  backup:
    schedule: "0 2 * * *"
```

**Operator knows HOW**:
1. Create StatefulSet with 3 replicas
2. Configure replication
3. Set up backup CronJob
4. Monitor health
5. Handle failover
6. Perform upgrades

**In OpenShift**: ~70 operators manage platform components.

**References**: 
- [ADR: Why operator-sdk](./decisions/adr-0001-operator-sdk.md)
- [Operator patterns](./platform/operator-patterns/)

---

## Immutable Infrastructure

### Nodes Are Immutable

**Principle**: Don't modify running systems, replace them.

**Traditional Approach**:
```bash
# SSH to server
ssh node-1
yum update kernel
reboot
# Hope nothing breaks
```

**OpenShift Approach**:
```yaml
# Declare OS configuration
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
spec:
  kernelArguments:
  - nosmt
```

**What Happens**:
1. MachineConfigOperator renders new OS image
2. Node drains workloads
3. Node reboots with new configuration
4. Node rejoins cluster
5. Workloads rescheduled

**Why Immutable**:
- Configuration drift impossible
- Rollback = use previous MachineConfig
- Reproducible across 1000s of nodes
- Auditability: git history = configuration history

**Technologies**:
- **RHCOS**: Red Hat Enterprise Linux CoreOS
- **rpm-ostree**: Atomic OS updates
- **Ignition**: First-boot provisioning

**References**: [MachineConfig](./domain/openshift/machineconfig.md)

---

## API-First Design

### Everything Is an API

**Principle**: All functionality exposed via declarative APIs.

**Why APIs**:
1. **Consistency**: Same interface for all operations
2. **Automation**: APIs = automatable
3. **Versioning**: Clear compatibility contracts
4. **Discoverability**: `oc api-resources` shows everything
5. **RBAC**: Fine-grained access control

### API Stability Contract

Once released, APIs follow strict compatibility:

**v1alpha1**: Unstable, can change
- Disabled by default
- Experimental features
- Can be removed

**v1beta1**: Semi-stable
- Enabled by default
- Deprecation warnings for changes
- 1 version compatibility window

**v1**: Stable, guaranteed
- Backward compatible changes only
- Minimum 1 year deprecation period
- Multiple version support

**Example Evolution**:
```yaml
# v1beta1: Field optional
spec:
  replicas: 3
  logLevel: info  # New field, optional

# v1: Field validated but compatible
spec:
  replicas: 3
  logLevel: debug  # Now validated: debug|info|warn
```

**References**: 
- [API Evolution](./practices/development/api-evolution.md)
- [API Conventions](../../dev-guide/api-conventions.md) (official)

---

## Declarative Over Imperative

### Declarative: "What" not "How"

**Imperative** (scripts, commands):
```bash
kubectl run nginx --image=nginx:1.21
kubectl scale deployment nginx --replicas=3
kubectl set env deployment nginx LOG_LEVEL=debug
```
- Order matters
- State in command history
- Doesn't handle failures
- Not reproducible

**Declarative** (resources):
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
        env:
        - name: LOG_LEVEL
          value: debug
```
- Describe desired state
- Git-trackable
- Self-healing
- Reproducible

**Apply** changes declaratively:
```bash
kubectl apply -f deployment.yaml
# Idempotent - safe to run multiple times
```

**OpenShift Preference**: Always use manifests over imperative commands.

---

## Upgrade Safety

### Zero-Downtime Upgrades

**Principle**: Platforms must upgrade without user impact.

**Requirements**:
1. **Component independence**: Operators upgrade independently
2. **API compatibility**: Old clients work with new APIs
3. **Graceful rollback**: Detect failure, roll back automatically
4. **Observable progress**: Users see upgrade status
5. **Payload ordering**: CVO coordinates component sequencing

### Upgrade Patterns

**ClusterVersion manifest**:
```yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
spec:
  desiredUpdate:
    version: 4.14.0
status:
  conditions:
  - type: Available
    status: "True"
  - type: Progressing
    status: "True"
    message: "Working towards 4.14.0"
```

**Operator responsibilities**:
- Set `Upgradeable=False` if prerequisites not met
- Use rolling updates (not all-at-once)
- Validate new config before applying
- Report progress via status conditions

**References**:
- [ADR: CVO Coordinates Upgrades](./decisions/adr-0003-cvo-coordination.md)
- [Upgrade strategies](./platform/operator-patterns/upgrade-strategies.md)

---

## Observability by Default

### Everything Must Be Observable

**Principle**: Platforms must expose internal state for debugging.

**Four Pillars**:

**1. Metrics** (Prometheus)
```go
operatorStatus := prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "cluster_operator_up",
        Help: "Cluster operator status (1=up, 0=down)",
    },
    []string{"name"},
)
```

**2. Logs** (Structured)
```go
log.Info("Reconciling resource",
    "namespace", req.Namespace,
    "name", req.Name,
    "generation", obj.Generation)
```

**3. Traces** (Distributed tracing)
- Track requests across services
- Identify bottlenecks
- Debug latency

**4. Events** (Kubernetes Events)
```go
r.Recorder.Event(obj, corev1.EventTypeWarning,
    "UpgradeFailed",
    "Failed to upgrade to version 2.0")
```

### SLO Framework

Every operator should define Service Level Objectives:

| Objective | Target | Measurement |
|-----------|--------|-------------|
| Availability | 99.9% | `up{job="my-operator"}` |
| Reconciliation Latency | p99 < 5s | `reconciliation_duration_seconds` |
| API Success Rate | 99.5% | `api_request_total{status!~"5.."}` |

**References**:
- [SLO Framework](./practices/reliability/slo-framework.md)
- [Observability Patterns](./practices/reliability/observability.md)

---

## Summary

These principles guide all OpenShift development:

| Principle | Key Insight | Example |
|-----------|-------------|---------|
| **Kubernetes Foundation** | Desired state reconciliation | Controller loops |
| **Operator Pattern** | Encode operational knowledge | 70+ operators |
| **Immutable Infrastructure** | Replace, don't modify | MachineConfig |
| **API-First** | Everything is declarative | CRDs everywhere |
| **Declarative** | What, not how | YAML manifests |
| **Upgrade Safety** | Zero downtime | CVO coordination |
| **Observability** | Always debuggable | Metrics/logs/traces |

**Further Reading**:
- [Platform Patterns](./platform/)
- [Engineering Practices](./practices/)
- [Architectural Decisions](./decisions/)

---

**Last Updated**: 2026-04-08  
**Maintainers**: OpenShift Engineering

<!-- 
EXTRACTION NOTES (for autonomous enrichment):
- Extract real examples from enhancements/ "Motivation" sections
- Add code snippets from merged enhancement implementations
- Cross-reference enhancements that demonstrate each principle
- Update "Why" sections with real rationales from enhancement decisions
- Add anti-patterns from "Alternatives Considered" sections
- Enrich with debugging tips from implementation experience
-->
