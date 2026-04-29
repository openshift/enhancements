# OpenShift Knowledge Graph

**Purpose**: Visual navigation map showing how concepts connect

**Last Updated**: YYYY-MM-DD

---

## How to Use This

**Don't read everything.** Follow your task path:
1. Find your task in "I want to..." table below
2. Read only the 4-5 docs in your path
3. Use cross-references as needed

---

## I Want To...

| Task | Start Here | Then Read | Finally |
|------|-----------|----------|---------|
| **Build an operator** | DESIGN_PHILOSOPHY.md | platform/operator-patterns/controller-runtime.md<br>platform/operator-patterns/status-conditions.md | domain/openshift/clusteroperator.md |
| **Add a feature** | workflows/index.md (links to enhancement process) | practices/development/index.md (links to API conventions) | practices/testing/index.md |
| **Debug an issue** | practices/reliability/index.md | platform/operator-patterns/must-gather.md | references/repo-index.md |
| **Understand a concept** | DESIGN_PHILOSOPHY.md | domain/kubernetes/ or domain/openshift/ | platform/operator-patterns/ |
| **Find a component** | references/repo-index.md | Component's AGENTS.md | Component's agentic/ docs |

---

## Knowledge Map

```
┌─────────────────────────────────────────────────────────────┐
│                    DESIGN_PHILOSOPHY.md                      │
│         (WHY - Core principles, read this first)             │
└─────────────────────────────────────────────────────────────┘
                              │
                ┌─────────────┼─────────────┐
                │             │             │
                ▼             ▼             ▼
┌───────────────────┐ ┌──────────────┐ ┌──────────────────┐
│  PLATFORM/        │ │   DOMAIN/    │ │   PRACTICES/     │
│  (HOW patterns)   │ │   (WHAT)     │ │   (WHEN/WHERE)   │
├───────────────────┤ ├──────────────┤ ├──────────────────┤
│ operator-patterns/│ │ kubernetes/  │ │ testing/         │
│ openshift-specifics│ │ openshift/   │ │ security/        │
│                   │ │              │ │ reliability/     │
│                   │ │              │ │ development/     │
└───────────────────┘ └──────────────┘ └──────────────────┘
         │                    │                  │
         └────────────────────┼──────────────────┘
                              │
                              ▼
                ┌─────────────────────────┐
                │   DECISIONS/            │
                │   (WHY decisions)       │
                │   Cross-repo ADRs       │
                └─────────────────────────┘
                              │
                              ▼
                ┌─────────────────────────┐
                │   REFERENCES/           │
                │   (WHERE to find)       │
                │   repo-index, glossary  │
                └─────────────────────────┘
```

---

## Concept Dependencies

### Operator Development Path
```
DESIGN_PHILOSOPHY.md
  ↓
controller-runtime.md (how reconciliation works)
  ↓
status-conditions.md (how to report health)
  ↓
clusteroperator.md (how CVO sees your operator)
  ↓
webhooks.md, rbac.md, finalizers.md (advanced patterns)
```

### API Development Path
```
DESIGN_PHILOSOPHY.md
  ↓
practices/development/index.md (→ links to dev-guide/api-conventions.md)
  ↓
domain/kubernetes/crds.md (CustomResourceDefinition basics)
  ↓
platform/operator-patterns/webhooks.md (validation)
```

### Testing Path
```
practices/testing/index.md (→ links to dev-guide/test-conventions.md)
  ↓
Testing pyramid concept
  ↓
E2E framework specifics
```

---

## Document Types

| Type | Purpose | Example |
|------|---------|---------|
| **Patterns** | How to implement | platform/operator-patterns/controller-runtime.md |
| **Concepts** | What something is | domain/openshift/clusteroperator.md |
| **Practices** | Links to official docs | practices/testing/index.md → dev-guide/ |
| **Decisions** | Why choices were made | decisions/adr-0001-*.md |
| **References** | Where to find things | references/repo-index.md |

---

## Progressive Disclosure

**Read in this order**:

1. **Foundation** (5 min): DESIGN_PHILOSOPHY.md
2. **Your task** (10 min): 2-3 docs from task path above
3. **Details** (20 min): Related patterns/concepts as needed
4. **Reference** (on-demand): Glossary, repo index when needed

**Total**: ~35 minutes to understand and start, not hours reading everything.

---

## Cross-References

- Official docs: See ../../dev-guide/ and ../../guidelines/
- Component repos: See references/repo-index.md
- Enhancement proposals: See ../../enhancements/
