# OpenShift - Agent Navigation

> Master entry point for all OpenShift repositories

**Version**: 1.0  
**Last Updated**: 2026-04-08  
**Repository**: openshift/enhancements  

**NEW**: [Visual Knowledge Graph](./KNOWLEDGE_GRAPH.md) - See the big picture first!  

---

## For AI Agents: Navigation Strategy

**DON'T read all 45+ docs**. Use [KNOWLEDGE_GRAPH.md](./KNOWLEDGE_GRAPH.md) to find your task path (4-5 docs).

**Steps**: 
1. Read KNOWLEDGE_GRAPH.md (2 min)
2. Use "I want to..." table for direct links
3. Read task-specific docs only (~1500 lines)
4. Reference glossary/indexes on-demand

**Examples**:
- Building operator? → DESIGN_PHILOSOPHY.md → controller-runtime.md → status-conditions.md → implementing-features.md (done)
- Adding feature? → implementing-features.md → enhancement-process.md → api-evolution.md (done)
- Debugging issue? → observability.md → must-gather.md → repo-index.md (done)

**Pattern**: Foundation (philosophy) → Task-specific patterns → Implementation. Not everything sequentially.

---

## Quick Navigation by Role

**Working on a specific component**  
→ [Repository index](./references/repo-index.md) - Find your component

**Understanding OpenShift platform**  
→ [Platform architecture](./platform/) - How OpenShift works

**Implementing cross-repo feature**  
→ [Enhancement proposals](../enhancements/) - Feature designs across repos

**Learning engineering practices**  
→ [Practices](./practices/) - Testing, security, reliability, development

**Understanding decisions**  
→ [Architectural decisions](./decisions/) - Cross-repo ADRs

---

## Core Platform Concepts

| Concept | Description | Documentation |
|---------|-------------|---------------|
| **ClusterOperator** | How operators report status to CVO | [clusteroperator.md](./domain/openshift/clusteroperator.md) |
| **ClusterVersion** | Platform upgrade coordination | [clusterversion.md](./domain/openshift/clusterversion.md) |
| **Machine API** | Node lifecycle management | [machine.md](./domain/openshift/machine.md) |
| **Custom Resource** | Kubernetes API extensions | [crds.md](./domain/kubernetes/crds.md) |
| **Operator Pattern** | Controller reconciliation | [operator-patterns/](./platform/operator-patterns/) |

---

## Standard Operator Patterns

All OpenShift operators follow these patterns:

| Pattern | Purpose | Documentation |
|---------|---------|---------------|
| **Status Conditions** | Available/Progressing/Degraded reporting | [status-conditions.md](./platform/operator-patterns/status-conditions.md) |
| **controller-runtime** | Reconciliation loops | [controller-runtime.md](./platform/operator-patterns/controller-runtime.md) |
| **Leader Election** | High availability for controllers | [leader-election.md](./platform/operator-patterns/leader-election.md) |
| **RBAC Patterns** | ServiceAccount and Role design | [rbac-patterns.md](./platform/operator-patterns/rbac-patterns.md) |
| **Upgrade Strategies** | Rolling updates and version skew | [upgrade-strategies.md](./platform/operator-patterns/upgrade-strategies.md) |

---

## Engineering Practices

All OpenShift repositories follow these practices:

| Practice | Documentation |
|----------|---------------|
| **Testing Pyramid** | Unit → Integration → E2E strategy | [pyramid.md](./practices/testing/pyramid.md) |
| **E2E Framework** | openshift-tests usage | [e2e-framework.md](./practices/testing/e2e-framework.md) |
| **CI Integration** | Prow and OpenShift CI | [ci-integration.md](./practices/testing/ci-integration.md) |
| **Threat Modeling** | STRIDE security analysis | [threat-modeling.md](./practices/security/threat-modeling.md) |
| **RBAC Guidelines** | Least privilege security | [rbac-guidelines.md](./practices/security/rbac-guidelines.md) |
| **SLO Framework** | Defining service level objectives | [slo-framework.md](./practices/reliability/slo-framework.md) |
| **Observability** | Metrics, logging, tracing | [observability.md](./practices/reliability/observability.md) |
| **Git Workflow** | Branching and commit standards | [git-workflow.md](./practices/development/git-workflow.md) |
| **API Evolution** | Versioning and breaking changes | [api-evolution.md](./practices/development/api-evolution.md) |

---

## Component Repository Index

**Find your component**: [repo-index.md](./references/repo-index.md)

70+ repositories organized by: Core Platform, Networking, Storage, Auth, Monitoring, Logging, Developer Experience. [See full index](./references/repo-index.md)

---

## Cross-Repo Architectural Decisions

Platform-wide decisions affecting multiple repositories:

[decisions/](./decisions/) - All architectural decision records (ADRs)

**Key decisions**:
- Why OpenShift uses etcd for cluster state
- Why CVO coordinates operator upgrades
- Platform-wide operator patterns
- [See all ADRs](./decisions/index.md)

---

## Relationship to Other Documentation

**This directory (`/agentic`)**: Structured knowledge for AI agents  
**[/enhancements](../enhancements/)**: Enhancement proposals (WHAT to build)  

These complement each other:
- Enhancements describe features and proposals
- Agentic provides structured knowledge for agents

---

## How to Use This Documentation

**For AI agents**:
1. **Start**: OPENSHIFT_AGENTS.md (this file) - You are here
2. **Navigate**: [KNOWLEDGE_GRAPH.md](./KNOWLEDGE_GRAPH.md) - Get task-specific reading path
3. **Foundation**: [DESIGN_PHILOSOPHY.md](./DESIGN_PHILOSOPHY.md) - Understand WHY (read this for any task)
4. **Task-specific**: Follow path from knowledge graph (4-5 docs per task)
5. **Reference**: Use [glossary.md](./references/glossary.md), [repo-index.md](./references/repo-index.md) on-demand

**For humans**:
- Use [/enhancements](../enhancements/) for feature proposals
- Use `/agentic` for structured reference

**Key principle**: Progressive disclosure - read foundation + task-specific docs only. Don't read all 45+ docs.

---

**Constraint**: This file MUST remain ≤170 lines for fast navigation.
