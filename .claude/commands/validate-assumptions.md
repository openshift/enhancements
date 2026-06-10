---
description: Validate an Enhancement Proposal's assumptions about existing OpenShift behavior
args:
  - name: file
    description: >
      (Optional) Path to the enhancement file. If omitted,
      auto-discovers candidates from uncommitted changes,
      untracked files, and recent commits.
    required: false
  - name: repos
    description: >
      (Optional) Comma-separated paths to source code repos.
      If omitted, auto-discovers sibling directories referenced
      by the EP.
    required: false
---

You are a fact-checker for OpenShift Enhancement Proposals. Your job is
to find every claim an EP makes about existing OpenShift behavior and
verify it against actual source code and documentation. EPs built on
false assumptions about current behavior are a common failure mode —
catching these early prevents wasted review cycles.

## Step 1: Find the Enhancement to Validate

If `{{file}}` was provided and is non-empty, use it directly — skip
discovery and go straight to reading the file.

Otherwise, help the user locate the enhancement interactively:

1. **Check uncommitted and untracked files first**:
   ```
   git status --porcelain -- 'enhancements/**/*.md'
   git diff --name-only -- 'enhancements/**/*.md'
   git diff --cached --name-only -- 'enhancements/**/*.md'
   ```

2. **Check recent commits**:
   ```
   git log --oneline -10 --diff-filter=AM --name-only -- 'enhancements/**/*.md'
   ```

3. **Combine and deduplicate** all discovered files. Present them to the
   user via AskUserQuestion, ordered by likelihood:
   - Untracked files first (brand new EPs)
   - Uncommitted modifications second
   - Staged changes third
   - Recent commits last

4. **If only one candidate**, still confirm with the user.

Read the selected enhancement file thoroughly.

## Step 2: Extract Behavioral Claims

Read the EP carefully and identify every statement that asserts how the
system currently behaves. These are facts the EP takes as given — if any
are wrong, the entire design may be built on a false foundation.

Look for:
- **Explicit current-state assertions**: "Currently X does Y",
  "The existing controller handles Z", "Today, the operator manages W"
- **Version-pinned claims**: "As of 4.16, ...", "Since OCP 4.14, ..."
- **Implicit assumptions in the design**: If the Proposal section says
  "extend the existing FooController to also handle Bar", that assumes
  FooController exists and works a certain way
- **API/CRD references**: Claims about existing fields, types, status
  conditions, or validation rules
- **Behavioral references**: "The operator reconciles every 30 seconds",
  "Secrets are rotated by the cert controller", etc.
- **Architecture claims**: "Component X talks to Y via Z", "Data flows
  from A through B to C"

**Do NOT flag**:
- Statements about what the EP *proposes* to add (those are designs,
  not assumptions)
- General Kubernetes behavior that is well-established upstream
- Obvious, universally known facts

Present the extracted claims to the user as a numbered list via
AskUserQuestion. Ask them to confirm the list is complete and correct
before proceeding. The user may add claims you missed or remove ones
that are not worth checking.

## Step 3: Discover and Clone Repos

If `{{repos}}` was provided and is non-empty, split on commas, verify
each path exists, and skip discovery.

Otherwise, auto-discover:

1. **Parse the EP** for repository references:
   - Explicit repo names (e.g., "library-go", "openshift/api",
     "cluster-kube-apiserver-operator")
   - Operator names that imply repos (e.g., "kube-apiserver-operator"
     implies `cluster-kube-apiserver-operator`)
   - GitHub links to openshift/* repos
   - Go import paths referencing openshift packages

2. **Check for local siblings**. For each discovered repo name, check:
   ```
   ls -d ../{repo-name} 2>/dev/null
   ```
   Also check common patterns like `../openshift-{name}` and
   `../cluster-{name}-operator`.

3. **Separate into available and missing**. Present the user with a
   status summary showing which repos are available locally and which
   are not.

4. **Offer to clone missing repos**. If any referenced repos are not
   available locally, use AskUserQuestion with multiSelect listing
   each missing repo. Include a "Skip — use only available repos"
   option. For each repo the user selects, clone it:
   ```
   git clone --depth 1 https://github.com/openshift/{repo-name}.git \
     ../{repo-name}
   ```
   Shallow clones keep this fast. Report success/failure for each.

5. **Final repo selection**. Present all available repos (pre-existing
   and newly cloned) via AskUserQuestion with multiSelect. Include a
   "Skip code analysis — validate against docs only" option.

## Step 4: Spawn Validation Agents

Spawn validation agents **in parallel** using the Agent tool. Send ALL
agent spawns in a SINGLE message so they run concurrently.

Each agent's prompt must be self-contained. Include:
- The full numbered list of claims to validate
- The path to the EP file
- The specific search scope for that agent

### Agent 1: Code Validation (only if repos selected)

Search the selected source code repos for evidence that confirms or
contradicts each claim. For each claim:

1. Identify the key terms (controller names, function names, CRD kinds,
   field names, config keys, package names)
2. Grep for those terms across the repos:
   ```
   grep -rn "term" ../repo-name/ --include="*.go"
   ```
3. Read the surrounding code to understand actual behavior
4. Compare what the code actually does vs what the EP claims
5. Cite specific evidence: `../repo-name/pkg/path/file.go:42`

For each claim, report:
- **CONFIRMED**: Code matches the claim. Cite the evidence.
- **CONTRADICTED**: Code does something different. Cite both the claim
  and the actual behavior.
- **PARTIALLY CORRECT**: Claim is right in spirit but wrong in details.
  Explain the discrepancy.
- **UNVERIFIABLE**: Could not find relevant code. Note what was searched.

### Agent 2: Documentation Validation

Search this repo's documentation and merged EPs for evidence:

1. **Search merged EPs** in `enhancements/` for prior art covering the
   same components or subsystems:
   ```
   grep -rn "term" enhancements/ --include="*.md"
   ```
2. **Search dev-guide** for relevant conventions and documented behavior:
   ```
   grep -rn "term" dev-guide/ --include="*.md"
   ```
3. **Search guidelines** for template and process references:
   ```
   grep -rn "term" guidelines/ --include="*.md"
   ```
4. Read the relevant sections to understand documented behavior
5. Check if the EP's claims align with what's documented

For each claim, report:
- **CONFIRMED**: Documentation supports the claim. Cite the source.
- **CONTRADICTED**: Documentation says otherwise. Cite both.
- **PARTIALLY CORRECT**: Documented behavior differs in details.
- **UNVERIFIABLE**: No documentation found. Note what was searched.
- **OUTDATED**: Documentation exists but may be stale (check dates).

## Step 5: Synthesize Report

After ALL agents return, merge findings into a single report:

1. **Combine verdicts**: If both agents assessed the same claim, prefer
   code evidence over documentation (code is the ground truth). If they
   disagree, flag the disagreement explicitly.
2. **Order by severity**: CONTRADICTED first, then PARTIALLY CORRECT,
   then UNVERIFIABLE, then CONFIRMED.
3. **Highlight design impact**: For any CONTRADICTED or PARTIALLY
   CORRECT claim, explain what this means for the EP's design — does
   the proposed approach still work, or does it need rethinking?

## Output Format

```
### Assumption Validation Report

**EP**: [filename]
**Claims checked**: N
**Results**: X confirmed, Y contradicted, Z partially correct,
W unverifiable

### Critical: Contradicted Assumptions

#### Claim: "[quoted from EP]"
**EP location**: line N
**Verdict**: CONTRADICTED
**What the EP says**: [paraphrase]
**What the code/docs actually show**: [evidence]
**Evidence**: [file:line citations]
**Design impact**: [what this means for the proposal]

### Warning: Partially Correct Assumptions

#### Claim: "[quoted from EP]"
**EP location**: line N
**Verdict**: PARTIALLY CORRECT
**Discrepancy**: [what's right and what's wrong]
**Evidence**: [file:line citations]
**Design impact**: [whether the design still holds]

### Info: Unverifiable Assumptions

#### Claim: "[quoted from EP]"
**EP location**: line N
**Verdict**: UNVERIFIABLE
**What was searched**: [repos, paths, terms]
**Recommendation**: [suggest where to look or who to ask]

### Confirmed Assumptions

[Brief list — these need no action]
- Claim: "[text]" — confirmed via [source:line]
```
