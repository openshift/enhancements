# OpenShift Enhancements - Repository Architecture

Quick-reference map for AI agents and contributors. For full details, read the linked authoritative sources.

## Repository Layout

```text
enhancements/                     # Root
├── AGENTS.md / CLAUDE.md         # Agent briefing
├── CONVENTIONS.md                # Cross-project coding standards (authoritative)
├── OWNERS                        # Approvers list
├── Makefile                      # lint, report, annual-summary targets
├── enhancements/<domain>/        # Enhancement proposals with YAML frontmatter
├── guidelines/                   # Template, process rules, supportability
│   └── enhancement_template.md   # Required template — linter enforces all sections
├── dev-guide/                    # Developer conventions (authoritative)
├── ai-docs/                      # AI-optimized platform knowledge hub
├── tools/                        # Go CLI for reports and EP management
├── hack/                         # CI and automation scripts
└── this-week/                    # Auto-generated reports — NEVER hand-edit
```

## CI Linting Pipeline

`make lint` runs three checks on every PR. The repository has a linter CI job; the job can be overridden in certain cases (e.g., template updates on mature PRs) — see [README.md](../README.md#template-updates) for the current override policy.

| Linter | Script | Scope | What It Checks |
|--------|--------|-------|----------------|
| **markdownlint** | `hack/markdownlint.sh` | ALL `enhancements/**/*.md` files | Markdown style per `.markdownlint-cli2.yaml` (line-length 400) |
| **metadata-lint** | `hack/metadata-lint.sh` | **Newly added** EP files only (via `hack/find_new.sh`) | YAML frontmatter: title, authors, reviewers, approvers, api-approvers, tracking-link. People fields must be non-empty and non-TBD. Title must match filename. |
| **template-lint** | `hack/template-lint.sh` | **Newly added** EP files only (via `hack/find_new.sh`) | All required section headings from `guidelines/enhancement_template.md` must be present |

**How `find_new.sh` works**: It calls `find_changed.sh` to get files changed since `PULL_BASE_SHA`, then filters to only files that do NOT exist in `origin/master` — selecting newly added files, not modified ones. See `hack/find_new.sh` lines 10-16.

## Approver Selection

Choosing an approver depends on the proposal's scope ([README.md](../README.md#how-are-enhancements-reviewed-and-approved)):
- **Team/component-scoped**: A peer or lead on that team
- **Broad scope** (changes OpenShift definition, adds dependency, changes support model): A staff engineer or their delegate — ask in `#forum-arch`

## EP Status Values

Valid statuses from `guidelines/enhancement_template.md`: `provisional`, `implementable`, `implemented`, `deferred`, `rejected`, `withdrawn`, `replaced`, `informational`. The template does not mandate a specific transition order.

## Key Authoritative Sources

| Topic | Read This |
|-------|-----------|
| Enhancement process | [README.md](../README.md) |
| EP template & sections | [guidelines/enhancement_template.md](../guidelines/enhancement_template.md) |
| API conventions | [dev-guide/api-conventions.md](../dev-guide/api-conventions.md) |
| Coding standards | [CONVENTIONS.md](../CONVENTIONS.md) |
| Linter config | [.markdownlint-cli2.yaml](../.markdownlint-cli2.yaml) |
| Title-to-filename check | [tools/cmd/metadataLint.go](../tools/cmd/metadataLint.go) |
| People & tracking-link validation | [tools/enhancements/metadata.go](../tools/enhancements/metadata.go) |
| File-selection logic | [hack/find_new.sh](../hack/find_new.sh) |

## See Also

- [Development Guide](./DEVELOPMENT.md) — contributing tasks
- [Testing Guide](./TESTING.md) — running linters
- [Design Philosophy](./DESIGN_PHILOSOPHY.md) — core principles
