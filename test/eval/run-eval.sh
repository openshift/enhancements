#!/bin/bash
# Quick evaluation runner for OpenShift Enhancements agentic docs

set -e

# Get script's directory and calculate repo root
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
REPO_ROOT="$( cd "$SCRIPT_DIR/../.." && pwd )"

echo "=== OpenShift Enhancements Agentic Docs Evaluation (Promptfoo) ==="
echo

# Load nvm if available
export NVM_DIR="$HOME/.nvm"
if [ -s "$NVM_DIR/nvm.sh" ]; then
    source "$NVM_DIR/nvm.sh"
    nvm use 22 &>/dev/null || true
fi

# Check prerequisites
if ! command -v node &> /dev/null; then
    echo "❌ Error: Node.js not found. Install Node.js 18+ first."
    exit 1
fi

echo "✅ Prerequisites check passed"

# Show which backend will be used
if [ -n "$ANTHROPIC_VERTEX_PROJECT_ID" ]; then
    echo "ℹ️  Using Vertex AI (project: $ANTHROPIC_VERTEX_PROJECT_ID)"
elif [ "$CLAUDE_CODE_USE_VERTEX" = "true" ]; then
    echo "ℹ️  Using Vertex AI (from ~/.config/claude/settings.json)"
elif [ -n "$ANTHROPIC_API_KEY" ]; then
    echo "ℹ️  Using Anthropic API"
else
    echo "⚠️  Warning: No API configuration detected"
    echo "   Set one of: ANTHROPIC_VERTEX_PROJECT_ID, CLAUDE_CODE_USE_VERTEX=true, or ANTHROPIC_API_KEY"
fi
echo

# Run evaluation
echo "🚀 Running evaluations..."
echo

# Change to repo root (where config and files are)
cd "$REPO_ROOT"

if [ $# -eq 0 ]; then
    # No arguments: run all tests
    npx --yes promptfoo@latest eval -c promptfooconfig.yaml
else
    # With arguments: filter tests by pattern
    npx --yes promptfoo@latest eval -c promptfooconfig.yaml --filter-pattern "$1"
fi

echo
echo "✅ Evaluation complete!"
echo
echo "💡 View detailed results:"
echo "   make eval-view"
