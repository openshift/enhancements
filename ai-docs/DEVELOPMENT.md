# Development Guide: openshift/enhancements

## Prerequisites

- Container runtime: `podman` (default) or `docker` (set `RUNTIME=docker`)
- Go toolchain (for `tools/` CLI only): check `tools/go.mod` for version
- Git with standard GitHub PR workflow

## Quick Start

```bash
# Clone
git clone https://github.com/openshift/enhancements.git
cd enhancements

# Run markdown linter (builds container image first)
make lint

# Generate weekly report
make report

# Check stale PRs
make closed-stale
make show-stale  # dry-run
```

## Build & Lint

| Command | What It Does |
|---------|-------------|
| `make lint` | Build linter image + run markdown validation on changed files |
| `make image` | Build the `enhancements-markdownlint` container image |
| `make image-clean` | Remove cached linter image |
| `PULL_BASE_SHA=HEAD~3 make lint` | Lint against a specific base ref |

The linter runs inside a container (`hack/Dockerfile.markdownlint`) and validates:
- Markdown formatting rules
- Required template sections from `guidelines/enhancement_template.md`
- YAML frontmatter presence and structure

## Common Tasks

### Writing a New Enhancement Proposal

1. Choose the domain directory under `enhancements/` (e.g., `enhancements/network/`)
2. Copy `guidelines/enhancement_template.md` into the domain directory
3. Fill required YAML frontmatter: `title`, `authors`, `reviewers`, `approvers`, `status`, `tracking-link`
4. Fill all required sections -- do NOT remove any headers (linter enforces them)
5. Address ALL topology sections: Hypershift, Standalone, SNO, MicroShift, OKE
6. Run `make lint` to verify
7. Open PR, request reviews from domain experts and designated approver

### Updating an Existing Enhancement

1. Find the EP under `enhancements/<domain>/`
2. Update the `last-updated` field in YAML frontmatter
3. Update `status` field if the lifecycle stage changed
4. Run `make lint` -- if template changes cause linter failures on existing EPs, see override policy in README.md

### Running Reports

```bash
# Weekly newsletter (runs closed-stale first, then generates report)
make report

# Annual summary for previous year
make annual-summary

# Upload report to HackMD (requires hackmd-cli image)
make report-image
```

### Overriding Linter for Template Changes

When the template changes and your EP fails the linter due to newly required sections (not content issues):
- **Draft EP with active discussion**: update EP to match new template
- **Mature EP near merge**: override the linter job
- **Updating a merged EP**: override the linter job

## PR Workflow

### Commit Conventions

- See `guidelines/commit_and_pr_text.md` for commit message format
- Push update patches rather than force-pushing (helps reviewers see changes)
- Use `/label tide/merge-method-squash` to squash on merge if using incremental commits

### Labels

- `priority/important-soon` — top-level release priority EP (highlighted in newsletters)
- `life-cycle/stale` — auto-applied after 28 days of inactivity
- `life-cycle/rotten` — auto-applied 7 days after stale
- Auto-close: 7 days after rotten

### Getting Reviews

1. Respond to comments quickly
2. Push incremental updates (not force-pushes)
3. Escalate in `#forum-arch` on Slack or bring to architecture review meetings
4. Allow at least 1-2 business days before pinging reviewers on Slack

## Common Mistakes

1. **Removing template sections**: The linter requires ALL sections from `enhancement_template.md`. If a section doesn't apply, write "Not applicable because..." -- do not delete the header
2. **Generic topology answers**: "No special considerations for SNO" is rejected. Explain WHY (resource impact, replica changes, HA assumptions)
3. **Missing tracking link**: The `tracking-link` YAML field must reference a Jira Feature or Epic
4. **Force-pushing PRs**: Makes review history harder to follow. Use incremental commits
5. **Skipping API review**: EPs with API changes need `api-approvers` field set and review in `#forum-api-review`
6. **Writing code before EP merge**: EP should be merged (or at least have consensus) before significant implementation begins

## Directory Conventions

- Enhancement files go in `enhancements/<domain>/` matching the component area
- Domain directories use lowercase with hyphens: `machine-config`, `cloud-integration`
- EP filenames should be descriptive: `configurable-network-diagnostics.md`, not `proposal.md`
- Images/diagrams go alongside the EP file in the same directory

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `RUNTIME` | `podman` | Container runtime for linter |
| `PULL_BASE_SHA` | `origin/master` | Base ref for linter diff |

## Platform Documentation

For generic development patterns, see:
- `dev-guide/` in this repo for OCP development conventions
- `guidelines/` for the enhancement process
- `CONVENTIONS.md` for platform-wide coding standards
