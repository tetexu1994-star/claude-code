#!/bin/bash
# Register tlaude-code CLI with CC Switch
# Usage: bash scripts/register-cc-switch.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CLI_PATH="$PROJECT_DIR/tlaude-code"

echo "=== CC Switch Registration for Claude Code CLI ==="
echo ""

# Check if binary exists
if [ ! -f "$CLI_PATH" ]; then
    echo "Binary not found at $CLI_PATH"
    echo "Building first..."
    cd "$PROJECT_DIR"
    go build -o tlaude-code ./cmd/tlaude-code/
    echo "Build complete."
fi

# Make binary executable
chmod +x "$CLI_PATH"

echo "CLI binary: $CLI_PATH"
echo "Config: $HOME/.tlaude-code/config.yaml"
echo ""

# Create CC Switch manifest
MANIFEST_DIR="$PROJECT_DIR/.cc-switch"
mkdir -p "$MANIFEST_DIR"

cat > "$MANIFEST_DIR/manifest.json" << 'MANIFEST_EOF'
{
  "name": "Claude Code (Go)",
  "version": "0.1.0",
  "description": "Go-based Claude Code alternative with multi-provider support, MCP, and TUI",
  "binary": "tlaude-code",
  "args": ["--print"],
  "config_format": "yaml",
  "config_path": "~/.tlaude-code/config.yaml",
  "homepage": "",
  "env": {},
  "category": "developer-tools"
}
MANIFEST_EOF

echo "Manifest created: $MANIFEST_DIR/manifest.json"
echo ""
echo "===== Manual Steps ====="
echo "1. Open CC Switch"
echo "2. Go to Settings → Custom CLI Tools → Add"
echo "3. Fill in:"
echo "   - Name: Claude Code (Go)"
echo "   - Binary: $CLI_PATH"
echo "   - Args: --print"
echo "   - Config: ~/.tlaude-code/config.yaml"
echo ""
echo "Or run the installation script first:"
echo "  bash scripts/install.sh"
echo ""
echo "To unregister, remove the entry from CC Switch settings."
