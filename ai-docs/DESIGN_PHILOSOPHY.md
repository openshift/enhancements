# OpenShift Design Philosophy

**Purpose**: Core principles guiding OpenShift architecture and development

**Last Updated**: YYYY-MM-DD

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

---

## The Operator Pattern

**Principle**: Extend Kubernetes with domain-specific knowledge.

**Pattern**: CustomResourceDefinition + Controller = Operator

**Benefits**:
- Codifies operational knowledge
- Automates Day 2 operations
- Follows Kubernetes patterns

---

## Immutable Infrastructure

**Principle**: Nodes are immutable. Changes require reboot.

**Implementation**: RHCOS + rpm-ostree + Ignition + MachineConfig

**Benefits**:
- Predictable state
- Easier rollback
- Reduced configuration drift

---

## API-First Design

**Principle**: Everything is an API resource.

**Pattern**: All configuration via Kubernetes/OpenShift APIs (no manual SSH, no local files)

**Benefits**:
- GitOps-friendly
- Auditable
- Version-controlled

---

## Declarative Over Imperative

**Principle**: Declare intent, not steps.

**Example**:
- ❌ Imperative: "Run pod1, then pod2, then update service"
- ✅ Declarative: "I want this state" (controller figures out steps)

---

## Upgrade Safety

**Principle**: Zero-downtime upgrades for platform and workloads.

**Mechanisms**:
- CVO orchestrates operator upgrades
- Rolling updates for nodes
- Upgrade ordering (etcd → kube → operators)

---

## Observability by Default

**Principle**: Platform components expose metrics, logs, and health status.

**Implementation**:
- Prometheus metrics
- Status conditions (Available/Progressing/Degraded)
- Structured logging

---

## Cross-Cutting Concerns

- **Security by default**: RBAC, SCCs, network policies
- **Multi-tenancy**: Namespace isolation
- **Supportability**: must-gather, diagnostics

---

**See Also**:
- [Operator Patterns](./platform/operator-patterns/)
- [Platform APIs](./domain/openshift/)
