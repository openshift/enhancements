# OpenShift Enhancements - Testing & Validation Guide

> Testing in this repository means linting and frontmatter validation, not unit tests (except for the `tools/` Go CLI).

## CI Linting Pipeline

`make lint` runs three checks on every PR. The repository has a linter CI job; the job can be overridden in certain cases (e.g., template updates on mature PRs) — see [README.md](../README.md#template-updates) for the current override policy.

| Linter | Scope | Key Details |
|--------|-------|-------------|
| **markdownlint** | ALL `enhancements/**/*.md` | Style rules from `.markdownlint-cli2.yaml` (line-length: 400) |
| **metadata-lint** | **Newly added** EP files only | Title-to-filename match (`tools/cmd/metadataLint.go`), people fields and tracking-link (`tools/enhancements/metadata.go`) via `hack/find_new.sh` |
| **template-lint** | **Newly added** EP files only | Required sections from `guidelines/enhancement_template.md` via `hack/find_new.sh` |

**Scope detail**: `hack/find_new.sh` selects files that appear in the PR diff but do NOT exist in `origin/master` — only newly added enhancement files, not modified ones.

## Running Linters

```bash
make lint                        # Full suite (builds container, runs all three)
PULL_BASE_SHA=HEAD~1 make lint   # Override comparison base
RUNTIME=docker make lint         # Use docker instead of podman
```

Individual scripts and their prerequisites:
```bash
hack/markdownlint.sh    # Needs markdownlint-cli2 (the container from `make image` provides it)
hack/metadata-lint.sh   # Needs Go toolchain — builds the Go tool directly (no container required)
hack/template-lint.sh   # Pure shell/grep — no container or Go toolchain required
```

## Common Validation Failures

| Failure | Cause | Fix |
|---------|-------|-----|
| Line too long | Exceeds 400 chars | Break long lines |
| Missing tracking-link | Required for all EPs | Add Jira/Bugzilla URL |
| Missing api-approvers | Always required by metadata linter | Use `"None"` if no API changes |
| People field invalid | Empty or "TBD" value | Provide a non-empty, non-TBD value |
| Title mismatch | YAML `title` ≠ filename | Align title with filename |
| Deleted section heading | Template linter requires all headings | Restore heading, write N/A if not applicable |

## Tools CLI Tests

```bash
cd tools && go test ./...    # Run all Go tests
```

## See Also

- [Architecture](./ARCHITECTURE.md) — CI pipeline details
- [Markdownlint Config](../.markdownlint-cli2.yaml)
- [Enhancement Template](../guidelines/enhancement_template.md)
- [hack/find_new.sh](../hack/find_new.sh) — file selection logic
- [tools/cmd/metadataLint.go](../tools/cmd/metadataLint.go) — title-to-filename check
- [tools/enhancements/metadata.go](../tools/enhancements/metadata.go) — people fields and tracking-link validation
