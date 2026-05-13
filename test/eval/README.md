# OpenShift Enhancements Agentic Docs Evaluation

Evaluation framework for testing AI agent usage of `./ai-docs/` and `CLAUDE.md` documentation using [promptfoo](https://promptfoo.dev).

## Quick Start

```bash
cd test/eval

# Run all evaluations (6 tests: 2 navigation + 1 authoring + 3 anti-pattern)
./run-eval.sh

# Run specific test by pattern
./run-eval.sh finding-enhancement-process

# Run only anti-pattern tests (what docs prevent)
./run-eval.sh anti-pattern

# View results in web UI
npx promptfoo view
```

**Notes:** 
- The config file (`promptfooconfig.yaml`) lives at **repo root**, not in `test/eval/`, because the Claude Agent SDK needs to run from repo root to access files.
- Default run executes **all 6 tests** (takes ~30-60 minutes total with 10 min timeout per test)

## Prerequisites

- **Node.js 18+** (for promptfoo and Claude Agent SDK)
- **Vertex AI configured** - The Claude Agent SDK needs to know to use Vertex. Choose one:
  
  **Option 1**: Explicit Vertex project (recommended)
  ```bash
  export ANTHROPIC_VERTEX_PROJECT_ID=your-gcp-project-id
  ```
  
  **Option 2**: Use Claude Code's Vertex settings
  ```bash
  export CLAUDE_CODE_USE_VERTEX=true
  ```
  This tells the SDK to read from `~/.config/claude/settings.json`
  
  **Option 3**: Direct Anthropic API (if you don't have Vertex)
  ```bash
  export ANTHROPIC_API_KEY=your-api-key
  ```

## File Structure

```
# In repo root:
├── promptfooconfig.yaml          # Main eval config (agent needs access to repo files)

# In test/eval/:
test/eval/
├── run-eval.sh                   # Quick-start script (uses npx)
└── README.md                     # This file
```

**Why config at root?** The Claude Agent SDK runs from the config directory and needs access to `CLAUDE.md`, `ai-docs/`, and `guidelines/`.

## Test Scenarios

| Test | Category | Description |
|------|----------|-------------|
| `navigation/finding-enhancement-process` | Navigation | Tests if agent finds enhancement process docs |
| `navigation/finding-operator-patterns` | Navigation | Tests if agent locates operator pattern docs |
| `enhancement-authoring/new-power-scheduler-proposal` | Authoring | Tests if agent can design fictional enhancement using docs |
| `anti-pattern/api-version-stable-from-start` | Anti-pattern | Tests if agent rejects v1 from start, uses v1alpha1 |
| `anti-pattern/custom-status-conditions` | Anti-pattern | Tests if agent rejects custom conditions, uses standard ones |
| `anti-pattern/breaking-field-rename` | Anti-pattern | Tests if agent rejects breaking rename, uses deprecation |

**Anti-pattern tests** measure what docs *prevent* (not just enable). These scenarios test whether agents naturally read and apply guidance that prevents harmful patterns.

## How It Works

1. **Provider**: Uses `anthropic:claude-agent-sdk` provider with Claude Sonnet 4.6
2. **Natural Discovery**: Tests don't tell agents to "read docs" - they must naturally discover CLAUDE.md
3. **Assertions**: Combines:
   - **String checks**: `icontains`, `contains-any` for expected content
   - **LLM rubric**: Judges whether response uses correct docs and provides actionable guidance
4. **Scoring**: Each assertion has a `weight`; final score is weighted average
5. **Verification**: "Documentation Used" section proves which files agent actually read

**Key Design Choice**: Agents are NOT explicitly told to read documentation. They must:
- Naturally check for project documentation (CLAUDE.md)
- Follow the navigation guidance therein
- Apply repo-specific rules from ai-docs/

This tests whether the agentic documentation approach works in practice, not just when explicitly instructed.

## Adding New Tests

Edit `promptfooconfig.yaml` (at repo root):

```yaml
tests:
  - description: "category/test-name"
    vars:
      prompt: |
        Your task prompt here
    assert:
      - type: icontains
        value: "## Documentation Used"
        weight: 4
      - type: llm-rubric
        value: |
          The response should reference specific ai-docs/ files
        weight: 3
```

Then run:
```bash
./run-eval.sh test-name
```

## Documentation

This README is the complete guide. Configuration lives in `promptfooconfig.yaml` at repo root.

## Advanced Usage

### Run Multiple Times

```bash
# Run each test 3 times
npx promptfoo eval --repeat 3
```

### Filter Tests

```bash
# Run only navigation tests
./run-eval.sh navigation

# Run only anti-pattern tests
./run-eval.sh anti-pattern

# Run specific test
./run-eval.sh finding-enhancement-process

# Or use promptfoo directly
npx promptfoo eval --filter-first-n 1  # Quick smoke test
```

### Export Results

```bash
# Export to JSON
npx promptfoo eval --output results.json

# Export to CSV
npx promptfoo eval --output results.csv
```

### Clear Cache

```bash
make eval-clean
```

## Cost Tracking

Promptfoo tracks costs automatically. View cost breakdown in the web UI (Cost tab).

## Troubleshooting

### Tests timing out
Increase timeout in `promptfooconfig.yaml`:
```yaml
defaultTest:
  options:
    timeout: 900000  # 15 minutes
```

### Agent SDK not finding files
Verify `working_dir: .` in `promptfooconfig.yaml` and that you're running from repo root.

### Vertex authentication issues
```bash
# Check Vertex project ID
echo $ANTHROPIC_VERTEX_PROJECT_ID

# Or check Claude Code settings
cat ~/.config/claude/settings.json | grep vertex
```
