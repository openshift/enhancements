# Development Guide — openshift/enhancements

## Build & Prerequisites

| Item | Value |
|------|-------|
| Go version (tools only) | 1.21 (`tools/go.mod:3`) |
| Default branch | `master` |
| Container runtime | `podman` (configurable via `RUNTIME` var) |
| CI linter | containerized markdownlint |

## Build Commands

```bash
# Lint markdown (requires container runtime)
make lint

# Override base ref for template check
make lint PULL_BASE_SHA=origin/master

# Build tools CLI
cd tools && go build ./...

# Generate weekly newsletter report
make report

# Check stale-closed enhancements
make closed-stale

# Dry-run stale check
make show-stale

# Build linter image
make image

# Clean linter image
make image-clean
```

## Common Tasks

### Writing a New Enhancement Proposal

1. Identify the domain area (e.g., `network`, `storage`, `authentication`)
2. Copy the template: `cp guidelines/enhancement_template.md enhancements/<domain>/<feature-name>.md`
3. Fill out the YAML frontmatter metadata (title, authors, reviewers, approvers, api-approvers, status)
4. Write required sections — do NOT remove any template headers, the linter rejects missing sections
5. For sections that don't apply, keep the header and explain why it's N/A
6. Run `make lint` to verify markdown formatting before opening PR
7. Open PR, request reviews from domain SMEs, get approver signoff

### Updating an Existing Enhancement

1. Read the existing EP to understand current status and context
2. Update the `last-updated` field in YAML frontmatter
3. Update `status` if the feature has progressed (e.g., `provisional` → `implementable`)
4. Add implementation details, graduation criteria, or test plans as they mature
5. Run `make lint` before pushing

### Adding a New Dev Guide

1. Create the file in `dev-guide/` with YAML frontmatter (title, authors, status: informational)
2. Follow the existing format: problem statement, conventions, examples, rationale
3. Reference from CONVENTIONS.md or other guides where appropriate
4. Run `make lint` to validate

### Running the Tools CLI

```bash
cd tools
go run ./main.go report        # Generate weekly report
go run ./main.go closed-stale  # Process lifecycle-bot closures
go run ./main.go pull-requests # List pull requests
```

The tools CLI requires GitHub tokens for API access. See `tools/README.md` for configuration.

## Common Mistakes

1. **Removing template sections**: The linter CI job requires ALL template headers present. Keep headers and write "N/A — [reason]" instead of deleting
2. **Force-pushing PR updates**: Use incremental commits so reviewers can see what changed. Use `/label tide/merge-method-squash` if needed
3. **Forgetting topology sections**: Since the template update, all EPs must address Hypershift, Standalone, SNO, and MicroShift topologies
4. **Skipping API approver**: Any EP with API changes (CRDs, webhooks, admission) must list an api-approver and get review from `#forum-api-review`
5. **Treating EPs as implementation docs**: EPs capture design decisions and rationale, not step-by-step implementation. Implementation lives in component repos
6. **Stale PRs**: Inactive PRs get `life-cycle/stale` after 28 days, `life-cycle/rotten` after 35, closed after 42. Keep PRs active or they auto-close

## Repository Contribution Workflow

1. Fork the repository
2. Create a feature branch from `master`
3. Make changes following the enhancement template
4. Run `make lint` locally (requires podman/docker)
5. Open PR with descriptive title following `guidelines/commit_and_pr_text.md`
6. Address reviewer feedback with incremental commits
7. Get approver signoff and merge

## Useful Searches

```bash
# Find all EPs for a specific component
ls enhancements/<domain>/

# Find EPs by status
grep -rl "status: implemented" enhancements/

# Find EPs mentioning a specific feature
grep -rl "feature-gate" enhancements/ | head -20

# Check which domains exist
ls enhancements/ | sort

# Count EPs per domain
for d in enhancements/*/; do echo "$(ls "$d"/*.md 2>/dev/null | wc -l) $(basename "$d")"; done | sort -rn | head -20
```

## SME Review Recommended

- Tools CLI configuration and authentication requirements should be verified by a contributor familiar with the weekly report generation process
- Linter behavior on template changes may have evolved; verify edge cases against recent CI runs

## Platform Documentation

This IS the platform documentation repository for OpenShift. For generic development patterns, see:
- [dev-guide/](../dev-guide/) — API conventions, operator requirements, testing standards
- [guidelines/](../guidelines/) — Enhancement process, template, PR guidelines
- [CONVENTIONS.md](../CONVENTIONS.md) — Project-wide naming and API conventions
