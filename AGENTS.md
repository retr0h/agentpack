# Agent Instructions

Canonical guidance for any AI coding agent working in this repository.

> **Read [`docs/development.md`](docs/development.md) before writing code.**

## What is this?

claudia is a git-free package manager for Claude Code plugins. It builds,
checksums, and distributes plugin archives (`.claudia` tarballs) so
non-technical users can install Claude Code skills, commands, hooks, agents,
MCP servers, and settings without needing git, Go, or any build toolchain.

Two personas, one binary:
- **Builder** — run in a git repo to package plugins into distributable archives
- **Installer** — run by anyone to unpack archives into `~/.claude/plugins/`

Replicates Claude Code's native marketplace directory structure so installed
packages are indistinguishable from git-cloned plugins. Family: grind (orange),
jot (magenta), meshx (green), stack (cyan), claudia (violet).

Skunkworks workflow — commits land directly on `main`.

## Hard rules

1. **`just ready` before committing.** Runs fmt, vet, lint.
2. **Table-driven tests.** One table per public function. Happy + failure rows
   in the same table.
3. **Never expose `internal/` types in `cmd/`** beyond what cobra needs.
4. **Never `//nolint:errcheck`.** Handle errors properly — helpers, log, return.

## Architecture

```
cmd/               Cobra CLI (root, build, verify, version)
internal/archive/  Tarball creation and extraction
internal/checksum/ Per-file SHA256 checksumming and verification
internal/manifest/ claudia.yaml parsing and validation
internal/metadata/ Git SHA, version, timestamp capture
internal/plugin/   Claude Code plugin structure generation
```

Key deps: cobra, yaml.v3, crypto/sha256, archive/tar, compress/gzip.

## Package contents

A `.claudia` archive can contain any combination of:
- **Skills** — markdown files
- **Commands** — markdown files referenced in plugin.json `commands[]`
- **Hooks** — `hooks.json` + shell scripts
- **Agents** — markdown agent definitions
- **MCP servers (binary)** — pre-built binaries for the target platform
- **MCP servers (remote)** — `.mcp.json` pointing to a remote endpoint
- **MCP servers (ux/npx)** — config referencing a UX package
- **Binaries** — pre-built executables to include as-is
- **Settings fragments** — JSON to merge into Claude Code's `settings.json`

## Commits

[Conventional Commits](https://www.conventionalcommits.org/). Scopes:
`cli`, `archive`, `checksum`, `manifest`, `metadata`, `plugin`.

When the agent is Claude, end every commit with:

```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

@docs/development.md
