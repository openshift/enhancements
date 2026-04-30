---
description: Adversarial review of an OpenShift Enhancement Proposal
args:
  - name: file
    description: >
      (Optional) Path to the enhancement file to review. If omitted,
      the skill will auto-discover candidates from uncommitted changes,
      untracked files, and recent commits.
    required: false
  - name: repos
    description: >
      (Optional) Comma-separated paths to source code repos for
      code-grounded review. If omitted, the skill will auto-discover
      sibling directories referenced by the EP.
      Example: ../library-go,../cluster-kube-apiserver-operator
    required: false
---

You are the orchestrator for an adversarial review of an OpenShift Enhancement
Proposal. Your review system is modeled on the most prolific reviewers in the
openshift/enhancements repository, distilled from analysis of ~40,000 review
comments across 586 merged PRs.

## Step 1: Find the Enhancement to Review

If `{{file}}` was provided and is non-empty, use it directly — skip
discovery and go straight to reading the file. This fast-path exists
for CI/PR automation where the target is already known.

Otherwise, help the user locate the enhancement interactively:

1. **Check uncommitted and untracked files first** — these are the most
   likely candidates (someone just wrote or edited an EP):
   ```
   git status --porcelain -- 'enhancements/**/*.md'
   git diff --name-only -- 'enhancements/**/*.md'
   git diff --cached --name-only -- 'enhancements/**/*.md'
   ```

2. **Check recent commits** for recently added/modified enhancements:
   ```
   git log --oneline -10 --diff-filter=AM --name-only -- 'enhancements/**/*.md'
   ```

3. **Combine and deduplicate** all discovered files. Present them to the
   user via AskUserQuestion, ordered by likelihood:
   - Untracked files first (brand new EPs)
   - Uncommitted modifications second
   - Staged changes third
   - Recent commits last
   - Always include an "Other" option (AskUserQuestion provides this
     automatically) so the user can type a custom path

4. **If only one candidate**, still confirm with the user — don't assume.

Read the selected enhancement file thoroughly.

## Step 2: Discover Relevant Repos

If `{{repos}}` was provided and is non-empty, use those paths directly —
split on commas, verify each path exists, and skip discovery.

Otherwise, auto-discover:

1. **Parse the EP** for repository references. Look for:
   - Explicit repo names (e.g., "library-go", "openshift/api",
     "cluster-kube-apiserver-operator")
   - Operator names that imply repos (e.g., "kube-apiserver-operator"
     implies `../cluster-kube-apiserver-operator`)
   - GitHub links to openshift/* repos
   - Go import paths referencing openshift packages

2. **Check for local siblings**. For each discovered repo name, check:
   ```
   ls -d ../{repo-name} 2>/dev/null
   ```
   Also check common patterns like `../openshift-{name}` and
   `../cluster-{name}-operator`.

3. **If local repos found**, use AskUserQuestion with multiSelect to
   let the user pick which to include. Add a "Skip code analysis"
   option for document-only review.

4. **If no local repos found**, inform the user the review will be
   document-only. Suggest which repos they could clone for a deeper
   review next time.

## Step 3: Spawn Parallel Review Agents

Spawn the review agents **in parallel** using the Agent tool. Each agent
gets a fresh context window and operates independently.

**IMPORTANT**: Send ALL agent spawns in a SINGLE message so they run
concurrently. Do not wait for one to finish before starting the next.

Each agent's prompt must be self-contained. Include:
- The path to the enhancement file (so the agent can read it)
- The paths to reference docs the agent should read
- The specific lens instructions for that agent
- The repo paths (for Agent 4 only)

### Agent 1: API Design & Document Quality Review

**Lenses**: 1, 6, 7

Reference docs to read:
- The enhancement file
- `dev-guide/api-conventions.md`
- `dev-guide/feature-zero-to-hero.md`
- `guidelines/enhancement_template.md`

### Agent 2: Systems Safety & Upgrade Review

**Lenses**: 2, 4

Reference docs to read:
- The enhancement file
- `CONVENTIONS.md`

### Agent 3: Architecture & Platform Scope Review

**Lenses**: 3, 5

Reference docs to read:
- The enhancement file
- `guidelines/enhancement_template.md`

### Agent 4: Code Verification Review (only if repos selected)

**Lens**: 8

Only spawn this agent if repos were selected (either via `{{repos}}` arg
or user selection in Step 2). It reads the enhancement file and explores
the selected repos.

## Step 4: Merge and Report

After ALL agents return, synthesize findings into a single unified review:

1. **Deduplicate**: If two agents flagged the same issue, keep the more
   specific version.
2. **Classify severity**: Ensure consistent BLOCKING / SIGNIFICANT / NIT
   ratings.
3. **Order by importance**: Blocking first, then code-grounded, then
   significant, then nits.
4. **Add Reviewer Simulations**: Pick the 3 most relevant archetypes:
   - **API Guardian** (JoelSpeed-style)
   - **Systems Thinker** (deads2k/p0lyn0mial-style)
   - **Platform Advocate** (tssurya/simonpasquier-style)
   - **Precision Editor** (Miciah-style)
   - **Process Enforcer** (everettraven/bparees-style)
   - **Simplifier** (zaneb/danwinship-style)
   - **Security Pedant** (mtrmac/avishayt-style)
   - **User Advocate** (dhellmann/candita-style)
   - **Scope Enforcer** (staebler-style)
   - **Upgrade Paranoid** (sdodson/jsafrane/sttts-style)
5. **Note what's good**: 2-3 things the enhancement does well.

## Output Format

```
### Summary
One paragraph overall assessment.

### Blocking Issues
[merged from all agents]

### Code-Grounded Findings (if repos were analyzed)
[from Agent 4: CONTRADICTION / MISSING / UNDERSTATED / ACCURACY]

### Significant Issues
[merged from all agents]

### Nits
[grouped to reduce noise]

### What's Good
[2-3 positives]

### Reviewer Simulations
[3 archetypes, one paragraph each]
```

---

## Lens Definitions

Include the relevant lens text in each agent's prompt so it knows
exactly what to check.

### Lens 1: API Design Rigor

Check every API change against OpenShift conventions from
`dev-guide/api-conventions.md`:
- Fields must be declarative nouns, not verbs
- No `Ref` suffix in OpenShift APIs
- Use `*Config` not `*Options` for configuration structs
- Every field tagged `+optional` or `+required`
- Godoc must start with json tag name, describe valid values/validations
- Use discriminated unions for mode-dependent fields
- CEL validation with custom messages, not regex (OCP 4.16+)
- Only `int32`/`int64` — never `uint`
- All arrays must have `+listType` and sensible `maxItems`
- Enum values PascalCase
- Pointer vs value semantics — is nil vs empty meaningful?
- Owner refs cannot cross scope boundaries
- Annotation keys: qualified names, max 63 chars after `/`
- Feature gates at highest new field, not leaf fields
- Challenge immutability vs mutability decisions
- Challenge if API exposes implementation details
- Check `metadata.name` character set limitations

### Lens 2: Failure Modes, Recovery, and Observability

For every component, workflow step, external dependency:
- Every failure mode enumerated with detection + mitigation?
- Pre-flight validations?
- Circuit breaker / timeout for external dependencies?
- Recovery from incorrect configuration?
- What happens on reboot, lost events, data leaks?
- Where do errors surface? (conditions, logs, alerts — be specific)
- "Get out of jail" escape hatch?
- What if admin doesn't check status after change?
- Concurrent controller behaviors during transitions?
- Finalizer ownership: only adder removes?
- Can misconfiguration brick the cluster?
- Support Procedures: concrete symptoms, log examples, `oc` commands?

### Lens 3: Platform, Topology, and Scope

- SNO: Resource impact? Replica assumptions?
- HyperShift/HCP: Management vs guest cluster?
- MicroShift: Relevant? Config exposure?
- Bare Metal: Cloud-only assumptions?
- OKE: Depends on excluded features?
- Disconnected: Internet needed? Mirroring? Payload size?
- Non-goals with rationale?
- Anything that belongs in separate EP?
- Deferred items that block usefulness?
- Day-0 vs Day-2 separated?
- Phase limitations vs permanent decisions?

### Lens 4: Upgrade, Downgrade, and Migration Safety

- Explicit upgrade path?
- Downgrade story?
- Version skew during rolling upgrades?
- Migration from unsupported workarounds?
- Version upgrade combined with feature migration?
- EUS: versions skippable?
- Breaking change in Y-stream?
- Deprecation path for old API fields?
- Support procedures dependent on feature gates?
- Cross-team dependency timelines aligned?
- Per-release schedule for phased rollouts?

### Lens 5: Architectural Boundaries and Ownership

- Which operator owns which resources?
- API placement correct? (config.openshift.io vs operator.openshift.io)
- Core payload vs OLM-managed?
- Static pod deps on deployment-based services?
- Who creates resources?
- Can users modify operator-created resources?
- Deletion: who cleans up? Data loss?
- Split-brain risk?
- Cross-namespace interactions?
- Trace downstream consumers
- Challenge: separate operator vs integration?
- Challenge: new CRD vs existing mechanisms?
- Challenge: "Do we need this complexity?"

### Lens 6: Document Quality, Template Compliance, Precision

Metadata: YAML fields present? status correct? last-updated current?
reviewers with domain comments? api-approvers set? tracking-link?

Content: User stories with "so that"? Acronyms defined? No "support"?
Verbose YAML trimmed? Open Questions actually open? Pinned commit links?
No contradictions? No duplicated paragraphs? Sections ordered for
readability? Concrete examples in Support Procedures?

Empty sections: explained not blank. Metrics/telemetry addressed.
Performance/scale considered.

AI detection: hallucinated references? fabricated risk analyses?
verbose security boilerplate? referenced KEPs/PRs exist?

### Lens 7: Cross-Feature Interaction and Testing

- Adjacent feature impacts?
- Unreleased upstream dependencies with timelines?
- Test plan specific (scenarios, configs, failure modes)?
- Regression tests for GA?
- CI lane design?
- Feature gate labels per feature-zero-to-hero.md?
- Graduation criteria concrete (5 tests, 7/week, 14/platform, 95%)?
- Alternatives with rejection rationale?
- Prior art consistency?
- Enhancement before implementation?
- Right teams tagged?

### Lens 8: Implementation Verification (requires repos)

Controller Behavior: Find Sync methods. Verify EP descriptions.
Check watch predicates, error handling, leader election, concurrency.

API Types: Read Go types. Compare fields, tags, validation, godoc.
Check feature gates wired up. Check enum values. Find TODO/FIXME/HACK.

Secret/Resource Layout: Verify data keys, namespaces, labels, annotations.

Sidecar/Static Pod: Check volume mounts, socket paths, resource requests,
security contexts.

Cross-Operator Coordination: Shared state? Ordering assumptions?

Existing vs Proposed: Flag if EP implies nonexistent code or proposes
duplicating existing code.

Cite dual locations: `EP line N` vs `repo/path/file.go:M`.
Categorize: CONTRADICTION / MISSING / UNDERSTATED / ACCURACY.
