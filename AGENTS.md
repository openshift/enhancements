# OpenShift Enhancements

**Repository**: openshift/enhancements — Enhancement tracking and platform conventions hub

> **Retrieval-led reasoning**: Read the linked authoritative sources below before relying on training data.

## Critical Warnings

1. **NEVER remove required EP headers** — the template linter enforces all sections from `guidelines/enhancement_template.md`; explain N/A rather than deleting
2. **NEVER submit an EP without valid YAML frontmatter** — `tools/cmd/metadataLint.go` validates title-to-filename match; `tools/enhancements/metadata.go` validates tracking-link and people fields (authors, reviewers, approvers, api-approvers must not be empty or "TBD")
3. **NEVER hand-edit generated reports** — `this-week/` files come from `make report`; modify `hack/` or `tools/` instead
4. **NEVER bypass markdownlint** — `make lint` enforces `.markdownlint-cli2.yaml` rules (line-length 400)

## Task Router

| Task | Start Here |
|------|-----------|
| Write an EP | [guidelines/enhancement_template.md](guidelines/enhancement_template.md) then [README.md](README.md) for process |
| Choose an approver | [README.md](README.md#how-are-enhancements-reviewed-and-approved) — depends on proposal scope |
| Run linters | `make lint` — see [ai-docs/TESTING.md](ai-docs/TESTING.md) for details |
| API conventions | [dev-guide/api-conventions.md](dev-guide/api-conventions.md) |
| Coding standards | [CONVENTIONS.md](CONVENTIONS.md) |
| Operator patterns | [ai-docs/platform/](ai-docs/platform/) and [dev-guide/operators.md](dev-guide/operators.md) |
| Topology considerations | [ai-docs/workflows/topology-considerations-guide.md](ai-docs/workflows/topology-considerations-guide.md) |
| Understand repo layout | [ai-docs/ARCHITECTURE.md](ai-docs/ARCHITECTURE.md) |

## CI Linting (Quick Reference)

- **markdownlint**: ALL `enhancements/**/*.md` files — style rules
- **metadata-lint**: **Newly added** EP files only (via `hack/find_new.sh`) — YAML frontmatter
- **template-lint**: **Newly added** EP files only (via `hack/find_new.sh`) — required sections

## Key Files

| What | Where |
|------|-------|
| Enhancement template | `guidelines/enhancement_template.md` |
| Process & approvers | `README.md` |
| Conventions / API rules | `CONVENTIONS.md`, `dev-guide/api-conventions.md` |
| Makefile targets | `make help` |
| File selection logic | `hack/find_new.sh` |
| Title-to-filename check | `tools/cmd/metadataLint.go` |
| People & tracking-link validation | `tools/enhancements/metadata.go` |

`CLAUDE.md` is a symlink to this file.
