# OpenShift Knowledge Graph

**Quick Navigation Map** - Start here to find what you need

```
                         ┌─────────────────────────────────────┐
                         │   OPENSHIFT_AGENTS.md (YOU ARE HERE) │
                         │   Master Entry Point                 │
                         └──────────────┬──────────────────────┘
                                        │
                                        ▼
                         ┌──────────────────────────────────┐
                         │   DESIGN_PHILOSOPHY.md           │
                         │   Read this FIRST (WHY)          │
                         └──────────────┬───────────────────┘
                                        │
                ┌───────────────────────┼───────────────────────┐
                │                       │                       │
                ▼                       ▼                       ▼
        ┌───────────────┐      ┌───────────────┐      ┌───────────────┐
        │   PLATFORM    │      │   PRACTICES   │      │    DOMAIN     │
        │   How to      │      │   How we      │      │   What exists │
        │   Build       │      │   Work        │      │   in cluster  │
        └───────┬───────┘      └───────┬───────┘      └───────┬───────┘
                │                      │                      │
    ┌───────────┴─────────┐   ┌────────┴────────┐   ┌────────┴────────┐
    │                     │   │                 │   │                 │
    ▼                     ▼   ▼                 ▼   ▼                 ▼
┌─────────┐         ┌─────────────┐      ┌──────────┐        ┌──────────┐
│Operator │         │  Testing    │      │Kubernetes│        │OpenShift │
│Patterns │         │  Security   │      │  Core    │        │Resources │
│(~10 docs│         │Reliability  │      │ Concepts │        │ Concepts │
└─────────┘         │Development  │      └──────────┘        └──────────┘
                    │  (~13 docs) │
                    └─────────────┘


        ┌───────────────┐      ┌───────────────┐      ┌───────────────┐
        │   DECISIONS   │      │   WORKFLOWS   │      │  REFERENCES   │
        │   Why we      │      │   How to      │      │   Where to    │
        │   chose this  │      │   Contribute  │      │   Look up     │
        └───────┬───────┘      └───────┬───────┘      └───────┬───────┘
                │                      │                      │
                ▼                      ▼                      ▼
        ┌──────────────┐      ┌──────────────┐      ┌──────────────┐
        │ ADR files    │      │ Workflows    │      │ References   │
        │ Key ADRs:    │      │ • Process    │      │ • Repo Index │
        │ • Example 1  │      │ • Example 2  │      │ • API Ref    │
        └──────────────┘      └──────────────┘      └──────────────┘
```

---

## Repository Structure

```
{repository}/
│
├─ agentic/                    ← YOU ARE HERE
│  ├─ OPENSHIFT_AGENTS.md          Entry point for all agents
│  ├─ DESIGN_PHILOSOPHY.md         Core principles (WHY)
│  ├─ KNOWLEDGE_GRAPH.md           This file (navigation)
│  ├─ platform/                    Patterns (HOW)
│  ├─ practices/                   Engineering practices
│  ├─ domain/                      Concepts (WHAT)
│  ├─ decisions/                   ADRs (WHY we chose this)
│  ├─ workflows/                   Process docs
│  └─ references/                  Indexes, glossary
│
└─ [Other repository content]
```

---

## Quick Lookup by Task Type

```
┌────────────────────────────────────────────────────────────────────────┐
│ Task Type                       →  Category                           │
├────────────────────────────────────────────────────────────────────────┤
│ UNDERSTAND (learn concepts)     →  DESIGN_PHILOSOPHY.md + domain/     │
│ BUILD (implement features)      →  platform/ + workflows/             │
│ DEBUG (troubleshoot issues)     →  practices/reliability/ + patterns  │
│ SECURE (protect systems)        →  practices/security/                │
│ TEST (validate code)            →  practices/testing/                 │
│ DECIDE (understand choices)     →  decisions/ (ADRs)                  │
│ FIND (locate things)            →  references/ (indexes + glossary)   │
└────────────────────────────────────────────────────────────────────────┘
```

---

## By Role Navigation

### New to OpenShift?

**Start here**:
1. [DESIGN_PHILOSOPHY.md](./DESIGN_PHILOSOPHY.md) - Core principles
2. [references/glossary.md](./references/glossary.md) - Key terms
3. [domain/kubernetes/](./domain/kubernetes/) - K8s fundamentals
4. [domain/openshift/](./domain/openshift/) - OpenShift concepts

### Building an Operator?

**Recommended path** (4-5 docs, ~1500 lines):
1. [DESIGN_PHILOSOPHY.md](./DESIGN_PHILOSOPHY.md) - Understand WHY
2. [workflows/implementing-features.md](./workflows/implementing-features.md) - See example
3. [platform/operator-patterns/controller-runtime.md](./platform/operator-patterns/controller-runtime.md) - Core pattern
4. [platform/operator-patterns/status-conditions.md](./platform/operator-patterns/status-conditions.md) - Status reporting
5. [practices/testing/pyramid.md](./practices/testing/pyramid.md) - Test strategy

### Adding a Feature?

**Recommended path**:
1. [workflows/enhancement-process.md](./workflows/enhancement-process.md) - Process
2. [practices/development/api-evolution.md](./practices/development/api-evolution.md) - API design
3. [practices/testing/](./practices/testing/) - Test requirements
4. [decisions/](./decisions/) - Check existing ADRs

### Debugging an Issue?

**Recommended path**:
1. [practices/reliability/observability.md](./practices/reliability/observability.md) - Debugging tools
2. [platform/operator-patterns/must-gather.md](./platform/operator-patterns/must-gather.md) - Collect diagnostics
3. [references/repo-index.md](./references/repo-index.md) - Find component repos

### Understanding a Decision?

**Check**:
1. [decisions/](./decisions/) - All ADRs
2. [decisions/index.md](./decisions/index.md) - ADR index
3. Search glossary for terms

---

## Documentation Categories

### Platform Patterns (`platform/`)

**Purpose**: Reusable patterns for building operators and controllers

| Pattern | File | Lines | Complexity |
|---------|------|-------|------------|
| Status Conditions | status-conditions.md | ~139-276 | Standard |
| controller-runtime | controller-runtime.md | ~139 | Standard |
| Finalizers | finalizers.md | ~443-467 | Complex |
| Webhooks | webhooks.md | ~443-589 | Complex |
| Leader Election | leader-election.md | ~154 | Standard |
| RBAC Patterns | rbac-patterns.md | ~119-121 | Simple |
| Owner References | owner-references.md | ~443-497 | Complex |
| Upgrade Strategies | upgrade-strategies.md | ~119 | Simple |
| must-gather | must-gather.md | ~443 | Complex |

**When to read**: Building operators, implementing controllers

---

### Engineering Practices (`practices/`)

**Purpose**: How we work, best practices across teams

**Testing** (`practices/testing/`):
- `pyramid.md` - Test strategy
- `e2e-framework.md` - E2E testing
- `ci-integration.md` - CI/CD
- `test-flake-policy.md` - Handling flakes

**Security** (`practices/security/`):
- `threat-modeling.md` - STRIDE methodology
- `rbac-guidelines.md` - Access control
- `secrets-management.md` - Handling secrets

**Reliability** (`practices/reliability/`):
- `slo-framework.md` - Service levels
- `observability.md` - Metrics/logs/traces
- `alerting.md` - Alert design

**Development** (`practices/development/`):
- `api-evolution.md` - API versioning
- `git-workflow.md` - Git/PR standards

**When to read**: Setting up new components, reviewing practices

---

### Domain Concepts (`domain/`)

**Purpose**: What exists in the cluster

**Kubernetes** (`domain/kubernetes/`):
- Core concepts: Pod, Node, Service, ConfigMap, Secret
- CRDs and custom resources

**OpenShift** (`domain/openshift/`):
- OpenShift-specific: ClusterOperator, ClusterVersion, Route
- Machine API: Machine, MachineConfig
- Operator concepts

**When to read**: Learning platform, understanding resources

---

### Architectural Decisions (`decisions/`)

**Purpose**: Why we made specific choices

Format: ADR (Architectural Decision Record)
- **Status**: Accepted/Proposed/Superseded
- **Context**: What problem we faced
- **Decision**: What we chose
- **Consequences**: Tradeoffs and implications

**When to read**: Understanding design rationale, making similar decisions

---

### Workflows (`workflows/`)

**Purpose**: How to contribute and implement features

- `enhancement-process.md` - Proposal workflow
- `implementing-features.md` - End-to-end example

**When to read**: Contributing features, following process

---

### References (`references/`)

**Purpose**: Lookup information

- `glossary.md` - Term definitions
- `repo-index.md` - Component repository map
- `enhancement-index.md` - Enhancement proposals index
- `api-reference.md` - API catalog
- `index.md` - Master index

**When to read**: Looking up terms, finding repos, browsing APIs

---

## Navigation Tips for AI Agents

### DON'T Read Everything

The documentation is 45+ files, 11,800+ lines. **Don't read it all**.

### DO Use This Navigation Strategy

1. **Start here**: Read this file (KNOWLEDGE_GRAPH.md)
2. **Find your task**: Use "By Role Navigation" or "Quick Lookup by Task Type"
3. **Read 4-5 targeted docs**: Follow the recommended path (~1500 lines)
4. **Reference on-demand**: Use glossary and indexes as needed

### Example Efficient Path

**Task**: "Build a new operator"

**Naive approach** (❌): Read all 45 docs (11,800 lines, ~2 hours)

**Smart approach** (✅): 
1. DESIGN_PHILOSOPHY.md → Understand principles
2. controller-runtime.md → Core pattern
3. status-conditions.md → Status reporting
4. implementing-features.md → See example
5. pyramid.md → Test strategy

**Total**: 5 docs, ~1,500 lines, ~20 minutes

---

## Content Depth Guide

Files vary in depth based on topic complexity:

| Category | Range | Why |
|----------|-------|-----|
| **Simple patterns** | 119-154 lines | Straightforward concepts (RBAC, upgrades) |
| **Standard patterns** | 139-276 lines | Common patterns (status, runtime) |
| **Complex patterns** | 443-589 lines | Advanced topics (webhooks, finalizers) |
| **Philosophy** | ~526 lines | Core principles with examples |
| **Navigation** | ~464 lines | This file - maps everything |

Don't expect uniform depth - complexity drives length.

---

## Maintenance

This file should be updated when:
- New directories added to `/agentic`
- Navigation patterns change
- New recommended paths identified
- File counts or line counts shift significantly

**Last Updated**: 2026-04-08

---

<!-- 
EXTRACTION NOTES (for autonomous enrichment):
- Update file counts and line counts from actual structure
- Add new navigation paths as patterns emerge
- Cross-reference enhancement proposals that demonstrate paths
- Add real user journeys from actual usage
- Update complexity ratings based on feedback
-->
