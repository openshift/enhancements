# OpenShift Enhancements - Agent Navigation Index

**Version**: 1.0 | **Docs**: ./ai-docs/ | **Role**: Ecosystem Hub

---

## CRITICAL: Retrieval Strategy

**IMPORTANT**: Prefer retrieval-led reasoning over pre-training-led reasoning.

When working on OpenShift:
- ✅ **DO**: Read relevant docs from `./ai-docs/` first
- ✅ **DO**: Verify patterns match current APIs (`oc explain`)
- ✅ **DO**: Check enhancement guidelines in `./guidelines/`
- ❌ **DON'T**: Rely solely on training data
- ❌ **DON'T**: Guess at API structures or enhancement process

---

## AI Navigation: DON'T Read All Docs

**Read 4-5 docs per task, not everything.**

### Common Task Flows

**Writing enhancement proposal?**
→ `./guidelines/enhancement_template.md` → `./ai-docs/workflows/enhancement-process.md` → `./ai-docs/practices/development/api-evolution.md`

**Building operator?**
→ `./ai-docs/DESIGN_PHILOSOPHY.md` → `./ai-docs/platform/operator-patterns/controller-runtime.md` → `./ai-docs/platform/operator-patterns/status-conditions.md` → `./ai-docs/practices/testing/pyramid.md`

**Adding API to existing operator?**
→ `./ai-docs/practices/development/api-evolution.md` → `./ai-docs/domain/kubernetes/crds.md` → `./ai-docs/platform/operator-patterns/webhooks.md`

**Understanding cluster upgrade process?**
→ `./ai-docs/domain/openshift/clusterversion.md` → `./ai-docs/platform/openshift-specifics/upgrade-strategies.md` → `./ai-docs/decisions/adr-0001-cvo-orchestration.md`

**Need visual map?**
→ `./ai-docs/KNOWLEDGE_GRAPH.md`

---

## Quick Navigation by Role

| Role | Start Here | Then Read |
|------|-----------|-----------|
| **Enhancement Author** | `./guidelines/enhancement_template.md` | `./ai-docs/workflows/enhancement-process.md` |
| **Operator Developer** | `./ai-docs/DESIGN_PHILOSOPHY.md` | `./ai-docs/platform/operator-patterns/` |
| **API Designer** | `./ai-docs/practices/development/api-evolution.md` | `./dev-guide/` |
| **Platform Architect** | `./ai-docs/decisions/` | `./ai-docs/DESIGN_PHILOSOPHY.md` |

---

## Core Platform Concepts

| Topic | File | Description |
|-------|------|-------------|
| **Design principles** | `./ai-docs/DESIGN_PHILOSOPHY.md` | Core architectural philosophy |
| **Visual navigation** | `./ai-docs/KNOWLEDGE_GRAPH.md` | Graph-based doc navigation |
| **Cluster operators** | `./ai-docs/domain/openshift/clusteroperator.md` | Status reporting, lifecycle |
| **Cluster upgrades** | `./ai-docs/domain/openshift/clusterversion.md` | CVO orchestration, upgrade ordering |
| **Custom resources** | `./ai-docs/domain/kubernetes/crds.md` | Extending Kubernetes API |
| **Pods** | `./ai-docs/domain/kubernetes/pod.md` | Container workload fundamentals |
| **Services** | `./ai-docs/domain/kubernetes/service.md` | Stable networking and discovery |

---

## Standard Operator Patterns

| Pattern | File | When to Use |
|---------|------|-------------|
| **Controller runtime** | `./ai-docs/platform/operator-patterns/controller-runtime.md` | Every operator (reconcile loops) |
| **Status conditions** | `./ai-docs/platform/operator-patterns/status-conditions.md` | Available/Progressing/Degraded reporting |
| **Webhooks** | `./ai-docs/platform/operator-patterns/webhooks.md` | Validation/mutation/conversion |
| **Finalizers** | `./ai-docs/platform/operator-patterns/finalizers.md` | Cleanup external resources on deletion |
| **RBAC** | `./ai-docs/platform/operator-patterns/rbac.md` | Service account permissions |
| **must-gather** | `./ai-docs/platform/operator-patterns/must-gather.md` | Debugging and diagnostics |
| **Upgrade safety** | `./ai-docs/platform/openshift-specifics/upgrade-strategies.md` | N→N+1 version skew, CVO coordination |

---

## Engineering Practices

| Area | Index | Description |
|------|-------|-------------|
| **Testing** | `./ai-docs/practices/testing/` | Pyramid (60/30/10), e2e framework |
| **Security** | `./ai-docs/practices/security/` | STRIDE, RBAC patterns, secret handling |
| **Reliability** | `./ai-docs/practices/reliability/` | SLI/SLO/SLA, degraded-mode patterns |
| **Development** | `./ai-docs/practices/development/` | API evolution, compatibility |

---

## Workflows

| Workflow | File | Links to Authoritative Source |
|----------|------|-------------------------------|
| **Enhancement process** | `./ai-docs/workflows/enhancement-process.md` | `./guidelines/enhancement_template.md` |
| **Feature implementation** | `./ai-docs/workflows/implementing-features.md` | `./dev-guide/` |
| **Exec-plan guidance** | `./ai-docs/workflows/exec-plans/` | Template for multi-week features |

---

## Component Repository Index

**Finding component repos**: See `./ai-docs/references/repo-index.md`

**Pattern**: Most components are in `openshift/<component-name>-operator` or `openshift/<component-name>`

**Search**: [GitHub org search](https://github.com/orgs/openshift/repositories)

---

## Cross-Repo Architectural Decisions

**Location**: `./ai-docs/decisions/`

**Index**: See `./ai-docs/decisions/index.md`

**Common ADRs**:
- Why etcd as backend
- Why CVO orchestration model
- Why immutable nodes (RHCOS + rpm-ostree)

---

## Relationship to Other Documentation

| Source | Purpose | When to Use |
|--------|---------|-------------|
| **This repo (`./ai-docs/`)** | AI-optimized ecosystem hub | Starting point, cross-repo patterns |
| **`./guidelines/`** | Authoritative enhancement process | Writing enhancement proposals |
| **`./dev-guide/`** | Development conventions | Git workflow, CI, coding standards |
| **`./enhancements/`** | Historical design docs | Understanding past decisions |
| **Component repos** | Implementation specifics | Component architecture, internal details |

---

## How to Use This Documentation

### For AI Agents
1. Start with task-specific flow (see "AI Navigation" section)
2. Read 4-5 linked docs, not entire tree
3. Use `oc explain <resource>` for field-level API details
4. Check `./guidelines/` for authoritative enhancement process
5. Link to component repos for implementation details

### For Humans
- **Skim**: Read index files (`index.md`) to orient
- **Search**: Use `grep -r "keyword" ./ai-docs/`
- **Verify**: Cross-reference with `oc explain` and `./guidelines/`
- **Navigate**: Use `KNOWLEDGE_GRAPH.md` for visual map

---

## Documentation Structure

```
./ai-docs/
├── DESIGN_PHILOSOPHY.md         # Core principles
├── KNOWLEDGE_GRAPH.md            # Visual navigation
├── platform/                     # Operator patterns
│   ├── operator-patterns/        # Controller runtime, status, webhooks
│   └── openshift-specifics/      # Upgrade safety, CVO coordination
├── domain/                       # Core API concepts
│   ├── kubernetes/               # Pod, Service, CRD, Node
│   └── openshift/                # ClusterOperator, ClusterVersion, Machine
├── practices/                    # Cross-cutting concerns
│   ├── testing/                  # Pyramid, e2e framework
│   ├── security/                 # STRIDE, RBAC, secrets
│   ├── reliability/              # SLI/SLO, degraded mode
│   └── development/              # API evolution, compatibility
├── decisions/                    # Cross-repo ADRs
├── workflows/                    # AI-optimized process guides
│   ├── exec-plans/               # Feature tracking templates (Tier 1)
│   ├── enhancement-process.md
│   └── implementing-features.md
└── references/                   # Pointers (GitHub links, oc commands)
    ├── repo-index.md
    ├── glossary.md
    └── api-reference.md
```

---

**Navigation**: Start with `KNOWLEDGE_GRAPH.md` for visual overview.

**Feedback**: Report issues at https://github.com/openshift/enhancements/issues
