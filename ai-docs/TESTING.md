# Testing Guide — openshift/enhancements

## Test Infrastructure

This repository does not contain traditional unit/integration tests for application code. Testing here focuses on:

1. **Markdown linting** — CI-enforced template compliance
2. **Tools CLI tests** — Go tests for the newsletter/stats tooling
3. **Enhancement quality** — Review process as the "test" for proposal quality

## CI Linting

### Markdown Linter

The primary CI check validates markdown formatting and template compliance:

```bash
# Run the linter locally (requires container runtime)
make lint

# With custom base ref
make lint PULL_BASE_SHA=<sha-or-branch>
```

**What the linter checks**:
- Markdown formatting rules (`.markdownlint-cli2.yaml`)
- Required template sections are present in enhancement proposals
- Only changed files are checked (against `PULL_BASE_SHA`)

**Common linter failures**:
| Failure | Cause | Fix |
|---------|-------|-----|
| Missing section header | Removed a template section | Add the header back with "N/A — [reason]" |
| Template mismatch | Template was updated after PR opened | Update EP to match new template, or override for mature EPs |
| Markdown formatting | Spacing, indentation, list format | Follow `.markdownlint-cli2.yaml` rules |

### Linter Configuration

The linter image is built from `hack/Dockerfile.markdownlint` and configured by `.markdownlint-cli2.yaml` at the repo root. It runs inside a container to ensure reproducible results:

```bash
# Build the linter container image
make image

# Clean the cached image
make image-clean
```

## Tools CLI Tests

The `tools/` directory contains a Go CLI with standard Go tests:

```bash
cd tools
go test ./...
```

The tools code follows standard Go testing patterns with `testify` assertions (`tools/go.mod` dependency).

## Enhancement Quality Assurance

The primary "testing" mechanism for enhancement proposals is the review process:

1. **Template compliance**: CI linter validates structural requirements
2. **Domain review**: SMEs from affected areas review technical accuracy
3. **API review**: Dedicated API approvers review CRD/webhook changes
4. **Approver signoff**: Final approval confirms consensus reached
5. **Topology review**: Reviewers verify Hypershift/HCP, Standalone, SNO, MicroShift, and OKE considerations

### What Reviewers Check

| Aspect | Expectation |
|--------|------------|
| Technical feasibility | Proposal is implementable as described |
| Topology coverage | All deployment topologies addressed with specifics, not "N/A" |
| Upgrade strategy | N→N+1 upgrade path clearly defined |
| API design | Follows `dev-guide/api-conventions.md` patterns |
| Test plan | Concrete, measurable test criteria specified |
| Operational aspects | Failure modes, metrics, support procedures documented |

## Writing Test Plans in Enhancement Proposals

Every EP requires a Test Plan section. Good test plans specify:

```markdown
## Test Plan

### Unit Tests
- Controller reconciliation logic tested with mock client
- Webhook validation tested with table-driven cases

### Integration Tests
- E2E test verifying feature enablement via FeatureGate
- Upgrade test from N-1 to N with feature enabled

### Disruption Tests
- Verify zero disruption to existing workloads during rollout
- Verify graceful degradation when dependent service unavailable
```

See `dev-guide/test-conventions.md` for cross-repo testing standards that EP test plans should reference.

## Platform Documentation

For testing standards that apply across all OpenShift component repos, see:
- [dev-guide/test-conventions.md](../dev-guide/test-conventions.md) — Cross-repo testing conventions
- [dev-guide/feature-zero-to-hero.md](../dev-guide/feature-zero-to-hero.md) — Feature lifecycle including test milestones

## SME Review Recommended

- Linter rule details and edge cases should be verified against `.markdownlint-cli2.yaml` and recent CI runs
- Tools CLI test coverage and patterns should be verified by a contributor familiar with the tooling
