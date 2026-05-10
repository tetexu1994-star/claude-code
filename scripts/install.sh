#!/bin/bash
# Install tlaude-code CLI to system PATH
# Usage: bash scripts/install.sh [--prefix=/usr/local]

set -euo pipefail

PREFIX="${1:-/usr/local}"
BINDIR="$PREFIX/bin"

echo "=== Claude Code CLI Installation ==="
echo ""

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "Building tlaude-code..."
cd "$PROJECT_DIR"
go build -o tlaude-code ./cmd/tlaude-code/
echo "Build complete."

echo ""
echo "Installing to $BINDIR/tlaude-code..."
mkdir -p "$BINDIR"
cp tlaude-code "$BINDIR/tlaude-code"
chmod +x "$BINDIR/tlaude-code"

echo ""
echo "✅ Installation complete!"
echo "   Try: tlaude-code --version"
echo ""
echo "To register with CC Switch:"
echo "   bash scripts/register-cc-switch.sh"
echo ""
echo "First run will create config at ~/.tlaude-code/config.yaml"
