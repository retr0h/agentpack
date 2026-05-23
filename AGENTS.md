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
   in the same table. No ad-hoc `Test*` functions outside a table. Every test
   function must have `tests := []struct{...}` and `for _, tt := range tests`.
3. **One test file per production file.** `foo.go` → `foo_test.go`. Never a
   test file named after a non-existent source file.
4. **`cmd/` is a thin shim.** No business logic. Parse flags, create context
   and VFS, call `pkg/`, format output. Testable logic lives in `pkg/`.
5. **Never expose `pkg/` types in `cmd/`** beyond what cobra needs.
6. **Never `//nolint:errcheck`.** Handle errors properly — helpers, log, return.
7. **Context and VFS.** Functions that iterate or do I/O accept `context.Context`
   and `avfs.VFS`. Check `ctx.Err()` in loops. Pure functions skip both.

## Architecture

```
cmd/               Cobra CLI shim (root, build, verify, version)
pkg/archive/       Tarball creation and extraction
pkg/build/         Build pipeline orchestration
pkg/checksum/      Per-file SHA256 checksumming and verification
pkg/cli/           Themed terminal output (banner, colors)
pkg/manifest/      claudia.yaml parsing and validation
pkg/metadata/      Git SHA, version, timestamp capture
pkg/plugin/        Claude Code plugin structure generation
pkg/verify/        Verify pipeline orchestration
```

- `cmd/` is a thin shim — parse flags, create context + VFS, call `pkg/`, print output
- `pkg/` is the public library API — consumable without the CLI
- Filesystem I/O uses `avfs.VFS` (production: `osfs`, tests: `memfs`)
- `context.Context` threads from CLI through cancellable operations

Key deps: cobra, yaml.v3, lipgloss, avfs, crypto/sha256, archive/tar, compress/gzip.

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
`cli`, `archive`, `build`, `checksum`, `manifest`, `metadata`, `plugin`, `theme`, `verify`.

When the agent is Claude, end every commit with:

```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

@docs/development.md
