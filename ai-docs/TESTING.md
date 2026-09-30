# Testing: openshift/enhancements

## Test Infrastructure

This repository uses markdown linting as its primary CI validation. There are no Go unit tests for the enhancement content itself — the `tools/` directory has its own Go tests for the CLI tooling.

## Linter Validation

### Running the Linter

```bash
# Full lint (builds container image if needed)
make lint

# Lint against a specific base ref
PULL_BASE_SHA=HEAD~5 make lint

# Build linter image only
make image
```

### What the Linter Checks

| Check | Enforced By | What It Validates |
|-------|-------------|-------------------|
| Markdown formatting | `markdownlint` in container | Heading levels, list formatting, line length |
| Required sections | Template section checker | All headers from `enhancement_template.md` present |
| YAML frontmatter | Frontmatter validator | Required fields: title, authors, reviewers, approvers, status |
| Link validity | Link checker | Internal cross-references resolve |

### Linter Container

The linter runs inside a container built from `hack/Dockerfile.markdownlint`:
- Image name: `enhancements-markdownlint:latest`
- Mounts the repo at `/workdir`
- Environment: `RUN_LOCAL=true`, `VALIDATE_MARKDOWN=true`
- Compares against `PULL_BASE_SHA` (default: `origin/master`) to lint only changed files

### Common Linter Failures

| Error | Cause | Fix |
|-------|-------|-----|
| Missing required section | Template header removed | Add the section back with "Not applicable" explanation |
| Invalid frontmatter | Missing YAML fields | Add required fields: title, authors, status, tracking-link |
| Heading level skip | H1 → H3 without H2 | Use sequential heading levels |
| Template change conflict | New sections added to template | For mature EPs: override job. For drafts: update to match |

## Tools Testing

The Go CLI under `tools/` can be tested independently:

```bash
cd tools
go test ./...
go build ./main.go
```

### CLI Commands

| Command | Tests | Purpose |
|---------|-------|---------|
| `closed-stale` | Integration with GitHub API | Comments on lifecycle-bot-closed PRs |
| `annual-summary` | Report generation | Generates yearly summary |
| `show-stale` | Dry-run of closed-stale | Preview without posting comments |

## Validation Before PR

1. Run `make lint` — must pass
2. Verify YAML frontmatter fields are complete
3. Verify all template sections are present (check against `guidelines/enhancement_template.md`)
4. Verify Topology Considerations section addresses all form factors
5. Verify any internal links to other EPs or docs resolve

## Platform Documentation

For test conventions in OCP component repos, see `dev-guide/test-conventions.md` in this repository.
