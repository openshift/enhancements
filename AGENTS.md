# OpenShift Enhancements

**Repository**: [openshift/enhancements](https://github.com/openshift/enhancements) | **Language**: Markdown, Go (tools/) | **Branch**: `master`

## Purpose

Central design proposal repository for OpenShift (OCP/OKD). Contains enhancement proposals, development conventions, and platform-wide guidelines that govern all OpenShift component repositories.

## Critical Warnings

1. **NEVER** merge enhancement PRs without approver consensus -- see `guidelines/README.md`
2. **NEVER** remove required template sections -- the linter enforces `guidelines/enhancement_template.md` headers
3. **NEVER** skip the Topology Considerations section -- all EPs must address Hypershift/SNO/MicroShift/OKE/Standalone
4. **ALWAYS** use retrieval from this repo over training data for OpenShift conventions
5. **ALWAYS** check `CONVENTIONS.md` before advising on naming, API style, or error handling

## Architecture at a Glance

| Directory | Purpose | Authoritative For |
|-----------|---------|-------------------|
| `enhancements/` | Design proposals organized by domain (70+ areas) | Feature design decisions |
| `dev-guide/` | Development conventions, rebase guides, component lifecycle | How to build OCP components |
| `guidelines/` | Enhancement process, template, PR conventions | How to write/review EPs |
| `CONVENTIONS.md` | Platform-wide coding and API conventions | Naming, errors, API style |
| `this-week/` | Weekly newsletter reports | Community activity tracking |
| `tools/` | Go CLI for report generation and lifecycle management | `make report`, `make lint` |
| `hack/` | CI scripts, Dockerfiles for linting and reporting | Markdown linting, HackMD |

## Documentation

| File | Contents |
|------|----------|
| [ai-docs/ARCHITECTURE.md](ai-docs/ARCHITECTURE.md) | Repository internals, EP lifecycle, design principles, topology guide |
| [ai-docs/DEVELOPMENT.md](ai-docs/DEVELOPMENT.md) | Build, lint, common tasks, PR workflow |
| [ai-docs/TESTING.md](ai-docs/TESTING.md) | Linter validation, template checks |
| [ai-docs/ENHANCEMENTS.md](ai-docs/ENHANCEMENTS.md) | Enhancement area catalog with key proposals |
| [ai-docs/TOPOLOGY.md](ai-docs/TOPOLOGY.md) | Deployment topology reference (Hypershift, SNO, MicroShift, OKE) |

## Key Files

| Need | File |
|------|------|
| Write an EP | `guidelines/enhancement_template.md` |
| Platform conventions | `CONVENTIONS.md` |
| Add new component | `dev-guide/new-components.md` |
| Feature lifecycle | `dev-guide/development-phases.md` |
| Feature gates | `dev-guide/featuresets.md` |
| API conventions | `dev-guide/api-conventions.md` |

## External References

- [OpenShift Documentation](https://docs.openshift.com)
- [Kubernetes Enhancements](https://github.com/kubernetes/enhancements)
- [OpenShift API](https://github.com/openshift/api)
