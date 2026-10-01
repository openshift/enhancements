# Review Instructions — openshift/enhancements

## Purpose

Review instructions for PRs to the OpenShift enhancements repository. This repo contains enhancement proposals, development conventions, and project-wide guidelines.

## Priority Review Areas

### Enhancement Proposals (`enhancements/`)

- **Template compliance**: All required sections from `guidelines/enhancement_template.md` must be present
- **Topology sections**: Hypershift, Standalone, SNO, MicroShift must have specific explanations, not just "N/A"
- **API extensions**: CRD/webhook changes must follow `dev-guide/api-conventions.md`
- **Upgrade strategy**: Must address N→N+1 upgrade path and version skew
- **Test plan**: Must include concrete, measurable test criteria

### Development Guides (`dev-guide/`)

- Consistency with `CONVENTIONS.md` and existing guides
- Accuracy of cross-references to other guides
- Examples should be verified against current APIs

### Conventions (`CONVENTIONS.md`)

- Changes affect ALL component repos — review with broad impact in mind
- Naming conventions must be backward-compatible
- Alert naming must follow `<Component><Condition>` format

## Do Not Report

These are intentional patterns in this repository:

- `enhancements/**` — Enhancement proposals are design documents with varying markdown styles; do not flag style inconsistencies
- `this-week/**` — Newsletter archives; do not flag formatting
- `tools/**` — Go tooling; standard Go review applies
- `hack/**` — CI scripts; operational code
- `vendor/**` — Vendored dependencies
- `go.sum` — Generated lockfile

## Path-Specific Rules

### `guidelines/enhancement_template.md`

The template is CI-enforced. Changes here affect linter behavior on all open PRs. Verify:
- New sections are backward-compatible (existing EPs shouldn't break)
- Required vs optional sections are clearly marked
- Placeholder text is clearly distinguishable from instructions

### `dev-guide/api-conventions.md`

Changes define API design rules for all of OpenShift. Verify:
- Rules are consistent with upstream Kubernetes API conventions
- Examples use current `openshift/api` types
- Breaking changes are called out explicitly

### `CONVENTIONS.md`

Project-wide conventions. Verify:
- Changes don't conflict with existing component implementations
- Naming patterns are consistent with existing `openshift/*` repos

## Platform Rule Citations

- API conventions: `dev-guide/api-conventions.md`
- Operator requirements: `dev-guide/operators.md`
- Testing standards: `dev-guide/test-conventions.md`
- Feature lifecycle: `dev-guide/feature-zero-to-hero.md`
- Project conventions: `CONVENTIONS.md`
- Enhancement template: `guidelines/enhancement_template.md`
