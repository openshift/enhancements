# OpenShift Enhancements

**Repository**: [openshift/enhancements](https://github.com/openshift/enhancements) | **Branch**: `master`

Enhancement tracking and design proposal repository for OpenShift (OCP & OKD). This is the **platform documentation hub** — it contains cross-repo conventions, development guides, and the enhancement proposal process, not a component implementation.

## Critical Warnings

1. **NEVER** modify `guidelines/enhancement_template.md` without understanding the linter CI enforces template compliance on all open PRs
2. **NEVER** remove required EP sections — the markdown linter rejects PRs missing template headers
3. **NEVER** treat `enhancements/` proposals as current API docs — they capture design intent, not live implementation state
4. **NEVER** add component-specific implementation docs here — those belong in the component repo
5. **ALWAYS** check `CONVENTIONS.md` before proposing naming, API, or UX patterns

## Architecture at a Glance

```
enhancements/          # 600+ design proposals organized by domain (78 areas)
guidelines/            # Enhancement process: template, commit guidelines, supportability
dev-guide/             # Development conventions: APIs, testing, feature gates, rebasing
CONVENTIONS.md         # Project-wide naming, API, and UX conventions
ROADMAP.md             # Project roadmap and priorities
tools/                 # Go CLI for weekly newsletter, stats, PR management
this-week/             # Weekly newsletter archives
hack/                  # CI scripts (markdown linter Dockerfile)
```

## Documentation

| Need | Start Here |
|------|-----------|
| Write an enhancement proposal | [guidelines/enhancement_template.md](guidelines/enhancement_template.md) |
| Understand the EP process | [guidelines/README.md](guidelines/README.md) |
| API design conventions | [dev-guide/api-conventions.md](dev-guide/api-conventions.md) |
| Feature lifecycle (Dev Preview to GA) | [dev-guide/feature-zero-to-hero.md](dev-guide/feature-zero-to-hero.md) |
| Feature gate / FeatureSet guidance | [dev-guide/featuresets.md](dev-guide/featuresets.md) |
| Testing conventions | [dev-guide/test-conventions.md](dev-guide/test-conventions.md) |
| Naming and UX conventions | [CONVENTIONS.md](CONVENTIONS.md) |
| Topology considerations | [ai-docs/TOPOLOGY_CONSIDERATIONS.md](ai-docs/TOPOLOGY_CONSIDERATIONS.md) |
| Architecture & design philosophy | [ai-docs/ARCHITECTURE.md](ai-docs/ARCHITECTURE.md) |
| Development & contribution guide | [ai-docs/DEVELOPMENT.md](ai-docs/DEVELOPMENT.md) |

## Key Files

| File | Purpose |
|------|---------|
| `guidelines/enhancement_template.md` | Authoritative EP template (528 lines) |
| `CONVENTIONS.md` | Project-wide conventions (613 lines) |
| `dev-guide/api-conventions.md` | API design rules (794 lines) |
| `dev-guide/operators.md` | Operator conventions (331 lines) |
| `OWNERS` | Repo approvers and reviewers |
| `.markdownlint-cli2.yaml` | Linter configuration for CI |

## External References

- [OpenShift API types](https://github.com/openshift/api)
- [Kubernetes Enhancement Proposals](https://github.com/kubernetes/enhancements)
- [OpenShift Documentation](https://docs.redhat.com/en/documentation/openshift_container_platform/)
