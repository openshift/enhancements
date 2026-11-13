# Review instructions

## Must fix before merge

Flag as must-fix:
- Removing or renaming a required section from `guidelines/enhancement_template.md` — CI enforces all headings
- Invalid or missing YAML frontmatter fields — the metadata linter validates title, authors, reviewers, approvers, api-approvers, and tracking-link; status, creation-date, and last-updated are required by the template but checked manually (`hack/metadata-lint.sh`)
- Enhancement status set to a value not listed in `guidelines/enhancement_template.md`
- Broken internal links to `dev-guide/`, `guidelines/`, or `ai-docs/` files
- Contradictions between `ai-docs/` content and authoritative sources (`CONVENTIONS.md`, `dev-guide/`)
- "Openshift" (lowercase s) anywhere — always "OpenShift" (`CONVENTIONS.md`)
- Hand-editing auto-generated `this-week/` report files
- Modifying `ai-docs/` platform content without updating all cross-file references to the same concept

## Minor issue volume

Report at most five minor issues. Overflow: "plus N similar items" in the summary. If all minor, lead with "No blocking issues."

## Do not report

- CI-enforced: markdownlint formatting, line length, whitespace — `.markdownlint-cli2.yaml` rules run in CI
- `this-week/**` — auto-generated newsletter reports
- `tools/go.sum` — dependency checksums; investigate only unexplained churn
- `enhancements/**/*.drawio.svg` — generated diagrams

Review `tools/go.mod` for semantic changes (added/removed dependencies, Go version bumps, replace directives).

## Always check

- EP YAML frontmatter completeness: all required fields present and valid (`hack/metadata-lint.sh`)
- EP section completeness: all required sections from template present; N/A explanation rather than deletion (`hack/template-lint.sh`)
- `api-approvers` field: required when EP modifies APIs; "None" if no API changes (`dev-guide/api-conventions.md`)
- Enhancement status matches actual implementation state (`guidelines/enhancement_template.md`)
- `ai-docs/` cross-file consistency: same concept described identically across all files
- Internal link validity: all relative paths resolve to existing files
- `ai-docs/` claims backed by verifiable references — file paths, Makefile targets, or authoritative doc links
- Platform convention citations in `ai-docs/` point to stable paths (`dev-guide/`, `guidelines/`, `CONVENTIONS.md`), not volatile `ai-docs/` internals

## Verification bar

Every comment must cite file:line evidence from the diff or linked source. If you cannot point to a specific line, do not post the comment. Read surrounding context before flagging — the answer may be nearby.

## Re-review

On re-review of an updated PR, only comment on lines that changed since the last review. Do not re-raise resolved issues or introduce new nits on unchanged code. Converge toward approval.

## Path-specific rules

### `enhancements/**`

Enhancement proposals: verify YAML frontmatter, required sections, and that `tracking-link` URLs are valid. Check topology considerations section when architecture changes affect deployment topologies.

### `dev-guide/**`, `guidelines/**`, `CONVENTIONS.md`

Authoritative platform conventions: changes here affect all component repos. Verify backward compatibility and cross-reference with existing component documentation.

### `ai-docs/**`

AI-optimized platform knowledge: verify factual accuracy against authoritative sources (`dev-guide/`, `guidelines/`, `CONVENTIONS.md`). Check that all internal links resolve and that cross-file references to the same concept are consistent.

### `tools/**`

Go CLI tooling: standard Go review practices apply. Check that `go.mod` Go version matches documented version. Verify cobra command registration.

### `hack/**`

CI scripts: changes affect all PR validation. Test locally with `make lint` before merging.
