# OpenShift Knowledge Graph

**Purpose**: Visual navigation map showing how concepts connect

**Last Updated**: 2026-06-23

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
| **Build a core operator** | DESIGN_PHILOSOPHY.md | platform/operator-patterns/controller-runtime.md<br>platform/operator-patterns/status-conditions.md | domain/openshift/clusteroperator.md |
| **Build an OLM operator** | DESIGN_PHILOSOPHY.md | platform/operator-patterns/controller-runtime.md<br>platform/operator-patterns/status-conditions.md | [OLM packaging](https://olm.operatorframework.io/docs/tasks/creating-operator-manifests/) |
| **Add a feature** | workflows/index.md (links to enhancement process) | practices/development/index.md (links to API conventions) | practices/testing/index.md |
| **Debug an issue** | practices/reliability/index.md | references/repo-index.md | `oc adm must-gather --help` |
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

### Operator Development Paths

```
DESIGN_PHILOSOPHY.md
  ↓
controller-runtime.md (how reconciliation works)
  ↓
status-conditions.md (how to report health)
  ↓
  ├─── Core / Platform Operators ───────── OLM-Managed Operators ──┐
  │                                                                 │
  │  clusteroperator.md                    Your CRD's .status       │
  │  (report to CVO via ClusterOperator)   (conditions on your CR)  │
  │                                                                 │
  │  CVO orchestrates upgrades             OLM manages lifecycle    │
  │  Available/Progressing/Degraded/       metav1.Condition types    │
  │    Upgradeable                         CSV defines install/RBAC │
  │  configv1 condition types                                       │
  │                                                                 │
  └─────────────────────┬───────────────────────────┘
                        ↓
        webhooks.md, rbac.md, finalizers.md (advanced patterns)
```

**Key difference**: Core operators create a `ClusterOperator` resource so the
CVO can track their health and gate upgrades (the CVO advances when
`Available=True`, `Degraded=False`, and the operator version matches the
target version; setting `Upgradeable=False` blocks
minor-version updates). OLM-managed operators report status on their own
Custom Resource using standard `metav1.Condition` and rely on OLM's
`ClusterServiceVersion` for lifecycle management. OLM does **not** proxy
operator health to the CVO — a degraded OLM-managed operator does not block
cluster upgrades (though a CSV's `olm.maxOpenShiftVersion` annotation can
block minor-version upgrades as a static compatibility constraint).

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

1. **Foundation**: DESIGN_PHILOSOPHY.md
2. **Your task**: 2-3 docs from task path above
3. **Details**: Related patterns/concepts as needed
4. **Reference** (on-demand): Glossary, repo index when needed

---

## Cross-References

- Official docs: See ../../dev-guide/ and ../../guidelines/
- Component repos: See references/repo-index.md
- Enhancement proposals: See ../../enhancements/
