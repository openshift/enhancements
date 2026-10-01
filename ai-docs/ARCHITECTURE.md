# Architecture — openshift/enhancements

## Repository Layout

```
openshift/enhancements/
├── enhancements/              # 600+ enhancement proposals in 78 domain areas
│   ├── authentication/        #   Auth: OAuth, OIDC, token, identity providers
│   ├── hypershift/            #   Hosted Control Planes design proposals
│   ├── installer/             #   IPI/UPI install, platform support
│   ├── machine-config/        #   MCO, node config, OS updates
│   ├── network/               #   CNO, OVN-K, ingress, DNS
│   ├── storage/               #   CSI drivers, persistent volumes
│   ├── update/                #   CVO, upgrade orchestration, OTA
│   └── ...                    #   65+ more domain areas
├── guidelines/                # Enhancement process governance
│   ├── enhancement_template.md  # Authoritative EP template — CI-enforced
│   ├── commit_and_pr_text.md    # PR title/description conventions
│   ├── supportability.md        # Supportability requirements for features
│   └── README.md                # Quick-start guide to the EP process
├── dev-guide/                 # Cross-repo development conventions
│   ├── api-conventions.md       # API design rules (794 lines, builds on k8s conventions)
│   ├── featuresets.md           # FeatureSet/FeatureGate lifecycle (514 lines)
│   ├── feature-zero-to-hero.md  # Full feature lifecycle guide (370 lines)
│   ├── operators.md             # Operator patterns and requirements (331 lines)
│   ├── test-conventions.md      # Testing standards across OCP
│   ├── new-components.md        # Adding components to OCP payload
│   ├── kubernetes-rebase.md     # Kubernetes version rebase process
│   ├── breaking-changes.md      # Breaking change policy and process
│   └── ...                      # Additional guides
├── CONVENTIONS.md             # Project-wide naming, API, UX conventions (613 lines)
├── ROADMAP.md                 # Project priorities and roadmap (256 lines)
├── tools/                     # Go CLI for newsletter, stats, PR management
│   ├── main.go                  # Entrypoint — cobra CLI
│   ├── cmd/                     # Subcommands (report, stats, closed-stale)
│   └── go.mod                   # Go 1.21, depends on go-github, go-jira
├── this-week/                 # "This Week in Enhancements" newsletter archives
└── hack/                      # CI scripts (Dockerfile.markdownlint)
```

## Key Domain Concepts

### Enhancement Proposals (EPs)

Enhancement proposals are the core artifact of this repository. Each EP is a structured markdown document that captures the design, rationale, and implementation plan for a significant OpenShift change. EPs follow a strict template (`guidelines/enhancement_template.md`) enforced by CI linting.

**Lifecycle**: `provisional` → `implementable` → `implemented` | `deferred` | `rejected` | `withdrawn` | `replaced`

**Required sections**: Summary, Motivation, User Stories, Proposal (including Topology Considerations, API Extensions, Risks), Design Details (including Graduation Criteria, Upgrade/Downgrade Strategy, Operational Aspects), Test Plan, Alternatives.

### Two-Tier Documentation Architecture

This repository serves as the **platform documentation tier** — conventions, patterns, and processes that apply across all OpenShift components. Individual component repos contain only component-specific docs.

**Decision rule**: "Would another repo need to duplicate this?" YES → platform (here). NO → component repo.

### Development Conventions

The `dev-guide/` directory contains authoritative cross-repo standards:

| Guide | Scope | Key Rules |
|-------|-------|-----------|
| `api-conventions.md` | API shape, naming, validation | Config vs Workload APIs; union discriminators; status conditions |
| `featuresets.md` | Feature lifecycle | FeatureSet → FeatureGate mapping; TechPreview → GA graduation |
| `feature-zero-to-hero.md` | End-to-end feature delivery | From idea to GA: EP → prototype → TP → GA with checklists |
| `operators.md` | Operator requirements | ClusterOperator status, must-gather, metrics, leader election |
| `test-conventions.md` | Test standards | Disruption testing, serial vs parallel, upgrade testing |
| `breaking-changes.md` | Compatibility policy | N-1 support, deprecation windows |

### Project Conventions (CONVENTIONS.md)

The `CONVENTIONS.md` file defines project-wide rules for:
- **Naming**: Components, images, repos, API types follow `cluster-X-operator` / `ClusterX` patterns
- **Grammar**: US English, Oxford comma
- **API patterns**: Optional vs Required, Defaulting, Status conventions
- **Alerts**: Naming (`<Component><Condition>`), severity levels, documentation requirements
- **Operators**: Namespace naming (`openshift-<component>`), CVO management, image references

## Design Philosophy

### Kubernetes Foundation

OpenShift builds on Kubernetes' core design principles:
- **Desired state reconciliation**: Controllers continuously drive current state toward declared desired state. Self-healing, idempotent, eventually consistent
- **Declarative configuration**: Users declare intent ("I want 3 replicas"), not steps. All config via Kubernetes API resources — no manual SSH, no local files

### The Operator Pattern

All OpenShift platform components are managed by operators. Each operator follows: CRD + Controller = Operator. This codifies operational knowledge into software, automating Day 2 operations while following Kubernetes patterns.

### Immutable Infrastructure

Nodes run RHCOS with rpm-ostree, configured via Ignition and MachineConfig. Changes require reboot. Benefits: predictable state, atomic rollback, no configuration drift.

### API-First Design

Everything is an API resource. Configuration, workloads, and cluster state are all managed through the Kubernetes API, making the system GitOps-friendly, auditable, and version-controlled.

### Upgrade Safety

Zero-downtime upgrades for platform and workloads. CVO orchestrates operator upgrades in dependency order. Rolling updates for nodes with configurable disruption policies. N→N+1 version skew tolerance is a hard requirement.

### Observability by Default

Platform components expose Prometheus metrics, report health via status conditions (Available/Progressing/Degraded), and use structured logging. The monitoring stack is a core platform component, not optional.

### Cross-Cutting Concerns

- **Security by default**: RBAC, SecurityContextConstraints, network policies
- **Multi-tenancy**: Namespace isolation, quota enforcement
- **Supportability**: must-gather diagnostics, structured error reporting

## Enhancement Process Workflow

```
1. Author writes EP using guidelines/enhancement_template.md
2. PR opened → CI linter validates template sections present
3. Reviewers from affected domains provide feedback
4. Approver (typically staff engineer or team lead) confirms consensus
5. EP merged as "provisional" or "implementable"
6. Implementation proceeds in component repos
7. EP updated to "implemented" when feature reaches GA
```

**Key governance rules**:
- Every EP needs exactly one approver and multiple reviewers (`guidelines/enhancement_template.md:7-8`)
- API changes require a dedicated API approver from `#forum-api-review`
- Lifecycle labels: `life-cycle/stale` after 28 days inactive, closed after 42 days
- Template changes may cause linter failures on open PRs — override for mature EPs, update for drafts

## Tools CLI

The `tools/` directory contains a Go CLI (`tools/main.go`) used for:

| Command | Purpose |
|---------|---------|
| `report` | Generate "This Week in Enhancements" newsletter |
| `closed-stale` | Comment on EPs closed by lifecycle bot |
| `stats` | Generate enhancement statistics |

Build: `cd tools && go build ./...`

## CI and Linting

The repository uses a containerized markdown linter:

- **Linter image**: Built from `hack/Dockerfile.markdownlint`
- **Config**: `.markdownlint-cli2.yaml` at repo root
- **Trigger**: Linter runs on PRs, validates markdown formatting and required template sections
- **Makefile targets**: `make lint` (requires container runtime), `make report`, `make closed-stale`

## Enhancement Domain Areas

The 78 domain directories under `enhancements/` map to OpenShift architectural areas. Key areas by volume:

| Area | Focus |
|------|-------|
| `network/` | OVN-Kubernetes, ingress, DNS, network policy |
| `storage/` | CSI, persistent volumes, snapshots |
| `installer/` | IPI, UPI, platform-specific install |
| `machine-config/` | MCO, node configuration, OS lifecycle |
| `update/` | CVO, OTA, upgrade strategy |
| `hypershift/` | Hosted Control Planes architecture |
| `authentication/` | OAuth, OIDC, identity providers |
| `monitoring/` | Metrics, alerting, observability |
| `cluster-api/` | CAPI integration, machine management |
| `microshift/` | Edge deployment, minimal footprint |

## OpenShift Integration Points

As the platform documentation hub, this repo defines conventions consumed by all component repos:

| Integration | How Components Use It |
|-------------|----------------------|
| EP template | Component teams write proposals following `guidelines/enhancement_template.md` |
| API conventions | `dev-guide/api-conventions.md` governs all `openshift/api` changes |
| Operator standards | `dev-guide/operators.md` sets ClusterOperator, metrics, must-gather requirements |
| Feature gates | `dev-guide/featuresets.md` defines FeatureSet/FeatureGate lifecycle |
| Test standards | `dev-guide/test-conventions.md` governs CI test expectations |
| Naming | `CONVENTIONS.md` dictates component, namespace, and alert naming |

## API Behavioral Contracts

### EP Metadata Requirements

Every enhancement file must start with a YAML frontmatter block containing:
- `title`, `authors`, `reviewers`, `approvers`, `api-approvers`
- `creation-date`, `last-updated`, `status`, `tracking-link`
- Optional: `see-also`, `replaces`, `superseded-by`

Status must be one of: `provisional`, `implementable`, `implemented`, `deferred`, `rejected`, `withdrawn`, `replaced`, `informational`

### Template Section Requirements

The CI linter enforces that all EPs contain the template's required headers. Removing a section causes CI failure — use "N/A with explanation" instead. Key required sections:
- Topology Considerations (Hypershift, Standalone, SNO, MicroShift)
- API Extensions (CRDs, webhooks, admission plugins)
- Operational Aspects (failure modes, metrics, support procedures)
- Upgrade / Downgrade Strategy
- Test Plan

### Convention Authority

`CONVENTIONS.md` is the authoritative source for project-wide naming and API conventions. Key contracts:
- Operator repos: `openshift/<component>-operator` (`CONVENTIONS.md:78`)
- Operator namespaces: `openshift-<component>` (`CONVENTIONS.md`)
- ClusterOperator names: lowercase, hyphenated, matching component (`CONVENTIONS.md`)
- Alert naming: `<Component><Condition>` format with required documentation

## Design References

### Why CVO Orchestration

CVO (Cluster Version Operator) manages the lifecycle of all cluster operators. It reads a release payload (a set of operator manifests), applies them in dependency order, and reports aggregate status via the ClusterVersion resource. This model ensures coordinated upgrades and rollback capability across 50+ operators. See: `enhancements/update/` for upgrade-related EPs.

### Why Enhancement Proposals

The EP process exists because OpenShift spans hundreds of repositories and dozens of teams. Individual PRs lack the cross-team visibility needed for features that touch multiple components. EPs provide a single place to discuss, debate, and reach consensus before implementation begins. See: `README.md`, `guidelines/README.md`.

### Why Immutable Nodes (RHCOS + rpm-ostree)

OpenShift chose immutable node infrastructure to eliminate configuration drift, enable atomic rollback, and simplify lifecycle management at scale. The MachineConfig Operator and rpm-ostree together provide a transactional OS update model. See: `enhancements/ocp-coreos-layering/`, `enhancements/machine-config/`.

## Platform Documentation

This IS the platform documentation repository. For component-specific documentation, see the individual component repos:

- **OpenShift API types**: [openshift/api](https://github.com/openshift/api)
- **Cluster Version Operator**: [openshift/cluster-version-operator](https://github.com/openshift/cluster-version-operator)
- **Machine Config Operator**: [openshift/machine-config-operator](https://github.com/openshift/machine-config-operator)
- **Kubernetes Enhancement Proposals**: [kubernetes/enhancements](https://github.com/kubernetes/enhancements)
- **OpenShift Official Docs**: [docs.redhat.com](https://docs.redhat.com/en/documentation/openshift_container_platform/)

## SME Review Recommended

- Enhancement domain coverage: This doc maps the 78 domain areas at a high level. SMEs should verify that newer domain directories are accurately categorized
- Convention currency: CONVENTIONS.md is actively evolving; verify specific convention claims against the current file
