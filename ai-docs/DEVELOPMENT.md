# OpenShift Enhancements - Development Guide

> This is a documentation repository. Development means writing enhancement proposals and maintaining platform docs.

## Writing an Enhancement Proposal

1. Copy template: `cp guidelines/enhancement_template.md enhancements/<domain>/<proposal-name>.md`
2. Fill out YAML frontmatter: title, authors, reviewers, approvers, api-approvers, status, tracking-link, creation-date, last-updated
3. Fill out all required sections — keep every heading from the template; write N/A if not applicable
4. Run `make lint` to validate
5. Create PR and choose an appropriate approver (see [README.md](../README.md#how-are-enhancements-reviewed-and-approved) for guidance on approver selection based on scope)

## Validating Changes

```bash
# Run all linters (builds container, checks newly added files)
make lint

# Override base comparison ref (default: origin/master)
PULL_BASE_SHA=HEAD~1 make lint

# Use docker instead of podman
RUNTIME=docker make lint
```

## Common Tasks

| Task | Command |
|------|---------|
| New EP | Copy `guidelines/enhancement_template.md` → fill frontmatter → `make lint` |
| Update EP | Edit content and `last-updated` → `make lint` |
| Weekly report | `make report` (never hand-edit `this-week/` files) |
| Annual summary | `make annual-summary` |
| Modify tools CLI | `cd tools && go build ./... && go test ./...` |

## Common Mistakes

1. **Deleting required sections** — the template linter rejects missing headings. Use N/A instead.
2. **Missing YAML frontmatter** — metadata-lint validates title, authors, reviewers, approvers, api-approvers, and tracking-link on newly added EP files. People fields must not be empty or "TBD".
3. **Hand-editing `this-week/` reports** — these are auto-generated. Modify `hack/` or `tools/` scripts instead.
4. **Using "Openshift"** — always capitalize as "OpenShift" per `CONVENTIONS.md`.

## See Also

- [Testing Guide](./TESTING.md) — linter details
- [Architecture](./ARCHITECTURE.md) — repo layout and CI pipeline
- [Enhancement Template](../guidelines/enhancement_template.md)
- [CONVENTIONS.md](../CONVENTIONS.md)
