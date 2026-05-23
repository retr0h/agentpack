#!/usr/bin/env bash
# Pre-tool hook: warn if terraform destroy is detected
set -euo pipefail

if echo "$CLAUDE_TOOL_INPUT" | grep -qi 'terraform destroy'; then
    echo "WARNING: terraform destroy detected — requires confirmation"
    exit 1
fi
