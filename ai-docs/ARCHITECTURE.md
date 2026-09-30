# Architecture: openshift/enhancements

## Repository Layout

```
openshift/enhancements/
├── enhancements/           # Enhancement proposals by domain (70+ subdirs)
│   ├── authentication/     # Auth-related EPs
│   ├── hypershift/         # Hosted Control Planes EPs
│   ├── installer/          # Cluster installation EPs
│   ├── machine-config/     # MCO-related EPs
│   ├── network/            # Networking EPs
│   ├── storage/            # Storage EPs
│   ├── update/             # CVO/upgrade EPs
│   └── ...                 # ~70 domain directories
├── dev-guide/              # Development conventions for OCP contributors
│   ├── api-conventions.md  # API design rules — AUTHORITATIVE
│   ├── new-components.md   # Adding components to OCP payload
│   ├── featuresets.md      # Feature gate mechanics
│   ├── development-phases.md # Dev Preview → Tech Preview → GA lifecycle
│   └── ...
├── guidelines/             # Enhancement process governance
│   ├── enhancement_template.md  # EP template — LINTER-ENFORCED
│   ├── README.md           # Quick-start for EP authors
│   └── supportability.md   # Supportability requirements
├── CONVENTIONS.md          # Platform-wide coding conventions — AUTHORITATIVE
├── ROADMAP.md              # Project-level objectives
├── tools/                  # Go CLI: report generation, lifecycle management
│   └── main.go             # Entrypoint: `make report`, `make closed-stale`
├── hack/                   # CI: markdown linting Dockerfile, report scripts
└── this-week/              # Weekly "This Week in Enhancements" newsletters
```

## Key Domain Concepts

### Enhancement Proposals (EPs)

EPs are the primary artifact. Each is a markdown file following `guidelines/enhancement_template.md` with required YAML frontmatter (title, authors, reviewers, approvers, status, tracking-link) and required sections (Summary, Motivation, Proposal, Design Details, Topology Considerations, etc.). The CI linter enforces section presence.

**EP Lifecycle**: `provisional` → `implementable` → `implemented` | `deferred` | `rejected` | `withdrawn` | `replaced` | `informational`

**PR Lifecycle**: Active discussion keeps PRs open indefinitely. Inactivity triggers `life-cycle/stale` (28 days) → `life-cycle/rotten` (7 days) → auto-close (7 days). The `tools/` CLI manages stale-closing comments via `make closed-stale`.

### Domain Organization

Enhancement proposals are organized by component domain under `enhancements/`. Major areas include: `authentication`, `autoscaling`, `baremetal`, `cloud-integration`, `cluster-api`, `console`, `etcd`, `hypershift`, `ingress`, `installer`, `machine-api`, `machine-config`, `microshift`, `monitoring`, `multi-arch`, `network`, `oc`, `olm`, `rhcos`, `scheduling`, `security`, `storage`, `update`, and ~50 more.

### Development Conventions

`dev-guide/` contains authoritative development guides consumed by all OCP component repos:

| Guide | Governs |
|-------|---------|
| `api-conventions.md` | API field naming, deprecation, versioning |
| `new-components.md` | Payload vs OLM delivery, operator requirements |
| `featuresets.md` | FeatureSet/FeatureGate mechanics, TechPreview gating |
| `development-phases.md` | Dev Preview → Tech Preview → GA promotion |
| `test-conventions.md` | Test naming, e2e patterns, CI expectations |
| `breaking-changes.md` | What constitutes a breaking change, mitigation |
| `kubernetes-rebase.md` | Kubernetes version rebase process |

### Platform Conventions (`CONVENTIONS.md`)

`CONVENTIONS.md` is the authoritative source for cross-project coding standards covering: naming (US English, Oxford comma), API conventions (conditions, status reporting), error handling, logging, CLI behavior, and security practices.

## OpenShift Design Principles

These principles guide all enhancement proposals and platform development decisions. They are distilled from the project's architectural foundations.

### Kubernetes Foundation: Desired State Reconciliation

OpenShift builds on Kubernetes' core model: users declare desired state, controllers reconcile current state to match. This produces self-healing, idempotent, eventually consistent systems. All OCP components must follow this pattern — imperative workflows are rejected in EP review.

### The Operator Pattern

Every OCP component is an operator: CustomResourceDefinition + Controller = Operator. Operators codify operational knowledge, automate Day 2 operations, and follow Kubernetes API conventions. See `dev-guide/operators.md` and `dev-guide/new-components.md` for requirements.

### Immutable Infrastructure

Nodes run RHCOS (Red Hat Enterprise Linux CoreOS) with rpm-ostree. Node configuration is managed through MachineConfig resources and applied via Ignition. Manual SSH changes are unsupported — all configuration flows through Kubernetes APIs. This ensures predictable state, rollback capability, and eliminates configuration drift.

### API-First Design

All configuration is expressed as Kubernetes API resources. No SSH, no local config files (exception: MicroShift uses `/etc/microshift/config.yaml`). This enables GitOps workflows, audit trails, and version-controlled infrastructure. See `dev-guide/api-conventions.md`.

### Declarative Over Imperative

EPs must propose declarative APIs. Users declare intent ("I want 3 replicas"), controllers determine steps. Imperative procedures (scripts, manual steps) are only acceptable for one-time bootstrap operations. See `CONVENTIONS.md` for API design rules.

### Upgrade Safety

Zero-downtime upgrades are a platform guarantee. The Cluster Version Operator (CVO) orchestrates operator upgrades with defined ordering (etcd → kube-apiserver → controllers → operators). Components must tolerate N-1/N+1 version skew during rolling updates. See `enhancements/update/` for upgrade-related proposals.

### Observability by Default

All platform components must expose: Prometheus metrics, status conditions (Available/Progressing/Degraded), and structured logging. The ClusterOperator API's three-condition model is the standard for reporting component health across the fleet. See `CONVENTIONS.md` for status condition conventions.

### Cross-Cutting Concerns

- **Security by default**: RBAC, SecurityContextConstraints (SCCs), network policies enforced
- **Multi-tenancy**: Namespace isolation, resource quotas, priority classes
- **Supportability**: must-gather diagnostics, structured alerts, support bundles — see `guidelines/supportability.md`

## Enhancement Template Structure

The template at `guidelines/enhancement_template.md` defines required EP sections. Key sections that AI agents must understand:

| Section | Purpose | Common Mistakes |
|---------|---------|-----------------|
| **Summary** | 2-3 sentence feature description | Too vague; must be actionable |
| **Motivation** | Why this matters, goals/non-goals | Missing non-goals |
| **Proposal** | API changes, workflow, architecture | Skipping API field definitions |
| **Design Details** | Implementation specifics | Omitting graduation criteria |
| **Topology Considerations** | Hypershift/SNO/MicroShift/OKE impact | "N/A" without explanation |
| **Test Plan** | Unit, integration, e2e coverage | Generic testing prose |
| **Graduation Criteria** | Dev Preview → Tech Preview → GA gates | Missing metrics/alerts |
| **Upgrade/Downgrade Strategy** | Version skew tolerance | Ignoring N-1 compatibility |
| **Operational Aspects** | Failure modes, monitoring, support | Missing must-gather changes |

## Tooling and Automation

### Go CLI (`tools/`)

The `tools/` directory contains a Go CLI for repository management:
- `make report` — generates the weekly "This Week in Enhancements" newsletter
- `make closed-stale` — comments on lifecycle-bot-closed PRs
- `make annual-summary` — generates annual summary report
- `make lint` — runs markdown linter via container (`hack/Dockerfile.markdownlint`)

### CI/Linting

- Markdown linting enforced via container: `hack/Dockerfile.markdownlint`
- Template section enforcement: linter checks for required headers from `enhancement_template.md`
- PR text conventions: `guidelines/commit_and_pr_text.md`

## Deployment Topology Awareness

All enhancement proposals must address how changes affect each OCP deployment topology. See [ai-docs/TOPOLOGY.md](TOPOLOGY.md) for the detailed reference guide. Summary:

| Topology | Key Constraint | EP Must Address |
|----------|---------------|-----------------|
| **Hypershift** | Split control/data plane | Which cluster runs each component |
| **Standalone** | Traditional self-hosted | Default behavior, no split |
| **SNO** | Single node, no HA | Resource overhead, replica count, HA assumptions |
| **MicroShift** | Minimal operator set, config-file driven | Operator dependencies, memory budget |
| **OKE** | No platform operators | OCP-only feature dependencies |

## Form Factor Detection

Operators detect topology via infrastructure API and node labels:

```
# SNO detection
oc get node -l node.openshift.io/single-node-cluster

# Infrastructure topology
oc get infrastructure cluster -o jsonpath='{.status.controlPlaneTopology}'
# Returns: HighlyAvailable | SingleReplica | External

# HCP detection
oc get hostedcontrolplane -A  # management cluster
```

## Design References

### EP Governance Model

Enhancement proposals require explicit approver designation. Approvers are typically team leads or staff engineers. Scope determines approver level: component-scoped EPs need team leads; cross-cutting EPs need staff engineers. Consensus is reached through GitHub review, Slack (`#forum-arch`, `#forum-api-review`), and architecture review meetings. See `guidelines/README.md`.

### Feature Gate System

OpenShift uses FeatureGate/FeatureSet to control feature availability. Features progress through Dev Preview (off by default), TechPreview (FeatureSet=TechPreviewNoUpgrade), and GA (default-on). The mechanics are defined in `dev-guide/featuresets.md` with implementation in `openshift/api` and `openshift/library-go`. Components must fail-closed when a gate is disabled.

### Component Delivery

New components enter OCP as either payload components (CVO-managed, always installed) or OLM-managed optional operators. The decision criteria and requirements are defined in `dev-guide/new-components.md`. Payload components must meet strict requirements for upgrade safety, resource footprint, and supportability.

## Platform Documentation

This repository IS the platform documentation hub for OpenShift development. Component repositories should reference:
- `dev-guide/` for development conventions and coding standards
- `guidelines/` for enhancement process and template
- `CONVENTIONS.md` for platform-wide conventions
- `enhancements/` for design decisions and architectural context

Do NOT duplicate content from `dev-guide/` or `guidelines/` in component repo docs. Link here instead.

## Enhancement Proposal Status Values

EPs use these status values in their YAML frontmatter:

| Status | Meaning |
|--------|---------|
| `provisional` | Initial draft, design not finalized |
| `implementable` | Design approved, ready for implementation |
| `implemented` | Feature shipped in a release |
| `deferred` | Accepted but postponed |
| `rejected` | Proposal declined after review |
| `withdrawn` | Author withdrew the proposal |
| `replaced` | Superseded by another EP |
| `informational` | Reference document, not a feature proposal |

## API Review Process

EPs that introduce or modify APIs require:
1. `api-approvers` field in YAML frontmatter (distinct from `approvers`)
2. Review request posted in `#forum-api-review` Slack channel
3. API reviewer assigned from the API review team
4. Must follow `dev-guide/api-conventions.md` rules

Key API conventions from `CONVENTIONS.md`:
- Use `conditions` for status reporting (Available, Progressing, Degraded)
- Follow Kubernetes API naming patterns (camelCase fields, PascalCase types)
- Never remove fields; deprecate with `+optional` marker
- Use `// +kubebuilder:validation:` markers for CRD validation
- Status subresource is mandatory for all CRDs

## PR Lifecycle Automation

The repository uses lifecycle labels for automatic PR management:

```
Active discussion → (28 days idle) → life-cycle/stale
                    (7 days more)  → life-cycle/rotten
                    (7 days more)  → auto-close
```

The `tools/` CLI generates comments on lifecycle-bot-closed PRs via `make closed-stale`. The `priority/important-soon` label highlights EPs related to top-level release priorities in the weekly newsletters.

## Newsletter Pipeline

Weekly "This Week in Enhancements" newsletters are generated via:
1. `make closed-stale` — processes recently closed PRs
2. `hack/this-week.sh` — generates the newsletter report
3. Output goes to `this-week/` directory
4. Optionally uploaded to HackMD via `make report-image`

## SME Review Recommended

- Enhancement domain coverage: the `enhancements/` directory contains 70+ areas; only major areas are cataloged in ENHANCEMENTS.md
- CI pipeline details: the markdown linter configuration and enforcement details may have evolved beyond what is documented here
- Tool CLI subcommands: the `tools/` Go CLI may have additional commands beyond `report`, `closed-stale`, and `annual-summary`
