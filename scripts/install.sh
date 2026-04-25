#!/bin/bash
set -euo pipefail

REPO="szibis/claude-escalate"
INSTALL_DIR="${HOME}/.local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Detecting system: ${OS}/${ARCH}"

# Get latest version
VERSION=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

if [ -z "$VERSION" ]; then
    echo "Could not determine latest version. Check https://github.com/${REPO}/releases"
    exit 1
fi

echo "Latest version: ${VERSION}"

# Download
BINARY="claude-escalate-${VERSION}-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}"

echo "Downloading ${URL}..."
mkdir -p "$INSTALL_DIR"
curl -sSL "$URL" -o "${INSTALL_DIR}/claude-escalate"
chmod +x "${INSTALL_DIR}/claude-escalate"

echo ""
echo "Installed claude-escalate ${VERSION} to ${INSTALL_DIR}/claude-escalate"
echo ""

# Verify
if "${INSTALL_DIR}/claude-escalate" version 2>/dev/null; then
    echo ""
    echo "Next steps:"
    echo "  1. Add to your Claude Code settings.json:"
    echo '     "hooks": { "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "claude-escalate hook", "timeout": 5 }] }] }'
    echo ""
    echo "  2. Start the dashboard:"
    echo "     claude-escalate dashboard"
else
    echo "Warning: binary installed but could not run. Check your PATH includes ${INSTALL_DIR}"
fi
