#!/usr/bin/env bash
# Post-write hook: run quick security lint on modified files
set -euo pipefail

file="$1"

case "$file" in
  *.go|*.py|*.js|*.ts)
    echo "acme: scanning $file for secrets..."
    # Placeholder — real scanner would run here
    ;;
esac
