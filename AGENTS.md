# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Repository Overview

This is the OpenShift Enhancement Proposals repository, inspired by the Kubernetes enhancement process. It provides a centralized place to propose, discuss, and document enhancements to OpenShift (both OKD and OCP). Enhancements are tracked as markdown documents with YAML frontmatter metadata.

## Repository Structure

- `enhancements/` - Enhancement proposals organized by domain (e.g., `network/`, `installer/`, `machine-config/`, `authentication/`, etc.)
- `guidelines/` - Process documentation including the enhancement template
- `dev-guide/` - Developer guides for various OpenShift subsystems
- `tools/` - Go-based CLI tools for managing enhancements
- `this-week/` - Weekly and annual enhancement summary reports
- `hack/` - Shell scripts for linting and automation

## Common Commands

### Linting
```bash
# Build the linter container image
make image

# Run markdown linter on changed files
make lint

# Override the base ref for template checking
PULL_BASE_SHA=origin/master make lint
```

### Enhancement Tools
The `tools/` directory contains a Go CLI for managing enhancements. Run from the `tools/` directory:

```bash
# Generate weekly enhancement report
cd tools && go run ./main.go report

# Generate annual summary
cd tools && go run ./main.go annual-summary

# Show reviewer statistics (last 31 days)
cd tools && go run ./main.go reviewers

# Show stale enhancement status
cd tools && go run ./main.go closed-stale --dry-run

# Leave comments on stale enhancements
cd tools && go run ./main.go closed-stale
```

### Makefile Targets
```bash
# Show all available make targets
make help

# Run weekly report generation (includes closed-stale check and linting)
make report

# Generate annual summary
make annual-summary
```

## Working with Enhancements

### Creating a New Enhancement

1. Choose the appropriate domain directory under `enhancements/` (or use root for broad-scope enhancements)
2. Copy `guidelines/enhancement_template.md` to your chosen location
3. Fill out the YAML metadata header (required fields: title, authors, reviewers, approvers, api-approvers, creation-date, tracking-link)
4. Complete all required sections - the linter enforces this
5. Create a PR and assign domain experts as reviewers

### Enhancement Metadata

Each enhancement must have YAML frontmatter with:
- `title` - lowercased, spaces/punctuation replaced with `-`
- `authors` - list of GitHub handles
- `reviewers` - list of reviewers with their domain expertise
- `approvers` - single approver preferred (responsible for consensus)
- `api-approvers` - required for API changes, use "None" if no API changes
- `creation-date` - format: `yyyy-mm-dd`
- `tracking-link` - link to Jira feature/epic

### Review Process

- Authors manage the enhancement through review and approval
- Reviewers must include representatives from teams doing implementation work
- Approvers recognize when consensus is reached
- PRs are merged when design is complete and consensus achieved
- Implementation should ideally wait until enhancement is merged

### Life-cycle

- Active PRs stay open indefinitely
- Inactive PRs get `life-cycle/stale` label after 28 days
- Stale PRs become `life-cycle/rotten` after 7 more days
- Rotten PRs close after another 7 days

## Writing Conventions

### Terminology
- "OpenShift" or "openshift" (NEVER "Openshift")
- Use U.S. English spelling and grammar
- Always use the Oxford comma
- "bare metal" - follow the [metal3-io style guide](https://github.com/metal3-io/metal3-docs/blob/master/design/bare-metal-style-guide.md)

### Naming Patterns
- Repository names match component names (e.g., `openshift/console-operator`)
- Image names tagged into ImageStreams match component names
- Git branches: `master` for development, `release-4.#` for maintenance
- API objects follow [Kubernetes API naming conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#naming-conventions)

## OpenShift Architecture Concepts

### Operator-Based Architecture
All cluster components are managed by operators that:
- Deploy, reconfigure, self-heal, and report health status
- Are coordinated by the cluster-version-operator (CVO) for core operators
- Use OLM (Operator Lifecycle Manager) for optional components with independent lifecycles

### Global Configuration
- Exposed via API resources in `config.openshift.io` API group
- Singleton objects named "cluster" control cluster-wide settings
- `spec` contains user configuration, `status` contains system state
- Components watch/poll configuration for changes

### High Availability Patterns
For 3-node control plane + 2-worker minimum topology:
- 2 replicas with hard pod anti-affinity on hostname
- Use maxUnavailable rollout strategy (25% default)
- For >=3 replicas on workers: soft pod anti-affinity
- Persistent storage considerations affect HA deployment

### Priority Classes
- `openshift-user-critical` - can be preempted by user workloads
- `system-cluster-critical` - not preempted but can be OOMKilled
- `system-node-critical` - not preempted, OOMKilled last

### Resource Requests
- Components MUST declare CPU and memory requests
- Components SHOULD NOT set resource limits
- CPU requests based on proportional baseline (etcd for control plane, SDN for all nodes)
- Memory requests: 90th percentile usage + 10% overhead

### Metrics and Monitoring
Core operators should expose Prometheus metrics:
- Use HTTPS with TLS client certificate authentication
- Read API server's TLS security profile for allowed versions/ciphers
- Support local authorization for well-known scraping identity
- Deploy ServiceMonitors for collection by in-cluster Prometheus

## Tools Configuration

The enhancement tools require `~/.config/ocp-enhancements/config.yml`:

```yaml
github:
  token: "your-github-personal-access-token"
reviewers:
  ignore:
    - openshift-ci-robot
```

## Testing and CI

- Markdown linting runs via containerized markdownlint
- Template validation ensures all required sections are present
- Metadata validation via `tools/cmd/metadataLint.go`
- Linter can be overridden if template changes after PR creation (see README for guidance)

## Key References

- Enhancement template: `guidelines/enhancement_template.md`
- OpenShift conventions: `CONVENTIONS.md`
- API conventions: `dev-guide/api-conventions.md`
- Breaking changes guide: `dev-guide/breaking-changes.md`
- Test conventions: `dev-guide/test-conventions.md`
