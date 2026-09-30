# Review Instructions: openshift/enhancements

## What This Repository Contains

Enhancement proposals (EPs) for OpenShift. Primarily markdown documents following `guidelines/enhancement_template.md`.

## Review Priorities

### Critical (Block merge)

1. **Missing template sections** -- all sections from `enhancement_template.md` must be present. If not applicable, explain why
2. **Topology Considerations gaps** -- every EP must address Hypershift, Standalone, SNO, MicroShift, OKE. "N/A" without explanation is insufficient
3. **Missing YAML frontmatter** -- `title`, `authors`, `reviewers`, `approvers`, `status`, `tracking-link` required
4. **Missing `api-approvers`** -- required for ALL EPs. Set named approvers for API changes; set `None` when there is no API change

### Important (Request changes)

5. **Vague graduation criteria** -- must have concrete metrics, not "when it's ready"
6. **Missing upgrade/downgrade strategy** -- must address N-1 version skew
7. **Generic test plan** -- must describe specific test scenarios, not "we will add tests"
8. **Incomplete operational aspects** -- missing failure modes, monitoring, support procedures

### Style

9. Follow `CONVENTIONS.md` for naming, grammar (Oxford comma, US English)
10. Follow `guidelines/commit_and_pr_text.md` for PR descriptions

## Do Not Report

These are handled by CI or are out of scope for code review:

- Markdown formatting issues (enforced by `make lint`)
- Line length violations (linter-enforced)
- `this-week/**` newsletter content
- `tools/**` Go code style (separate review process)
- Enhancement content disagreements (handled by approver consensus)

## Path-Specific Rules

- `enhancements/**/*.md` -- verify template compliance, topology coverage, API review designation
- `dev-guide/**` -- verify accuracy against current OCP practices
- `guidelines/**` -- high scrutiny; changes affect all future EPs

## Platform Citations

- Naming conventions: `CONVENTIONS.md`
- API design: `dev-guide/api-conventions.md`
- Test expectations: `dev-guide/test-conventions.md`
- Component lifecycle: `dev-guide/new-components.md`
- Feature progression: `dev-guide/development-phases.md`
