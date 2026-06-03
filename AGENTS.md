# Agent Instructions

Canonical guidance for any AI coding agent working in this repository.

**Progressive disclosure.** Every document in this repo is self-contained
and ordered most-critical-first. Never use `@`-includes, "read X first"
directives, or cross-file dependencies that force the reader to jump
elsewhere before they can act. If an agent stops reading at any heading,
they should have enough context to work safely up to that depth.

## What is this?

agentpack is a git-free package manager for Claude Code plugins. It builds,
checksums, and distributes plugin archives (`.agentpack` tarballs) so
non-technical users can install Claude Code skills, commands, hooks, agents,
MCP servers, and settings without needing git, Go, or any build toolchain.

Two personas, one binary:
- **Builder** — run in a git repo to package plugins into distributable archives
- **Installer** — run by anyone to unpack archives into `~/.claude/plugins/`

Skunkworks workflow — commits land directly on `main`.

## Hard rules

1. **`just ready` before committing.** Runs fmt, vet, lint.
2. **Table-driven tests.** One table per public function. Happy + failure rows
   in the same table. No ad-hoc `Test*` functions. Every test function must
   have `tests := []struct{...}` and `for _, tt := range tests`.
3. **Testify for assertions.** Use `assert` (non-fatal) and `require` (fatal)
   from `github.com/stretchr/testify`. No stdlib `t.Errorf`/`t.Fatalf` for
   value checks. No custom assertion helpers. No custom messages on
   assertions — `assert.Equal(t, want, got)` not
   `assert.Equal(t, want, got, "should match")`.
4. **One test file per production file.** `foo.go` → `foo_test.go`. Never a
   test file named after a non-existent source file.
5. **`cmd/` is a thin shim.** No business logic. Parse flags, create context
   and VFS, call `pkg/`, format output. Testable logic lives in `pkg/`.
6. **Never expose `pkg/` types in `cmd/`** beyond what cobra needs.
7. **Never `//nolint:errcheck`.** Handle errors properly.
8. **Context and VFS.** Functions that iterate or do I/O accept `context.Context`
   and `avfs.VFS`. Check `ctx.Err()` in loops. Pure functions skip both.
9. **All output through `internal/cli/`.** No raw `fmt.Fprintf` in `cmd/`.
10. **No hand-rolled mocks.** Use `go.uber.org/mock/mockgen` for interface
    mocks. VFS error-injecting wrappers are decorators, not mocks.
11. **Interfaces where consumed.** Every package that calls another package
    defines a small interface for what it needs, accepts it (nil = default),
    and calls through it. This includes `cmd/` — each cmd file defines an
    unexported interface for the `pkg/` function it calls, with a package-level
    var defaulting to the real implementation.
12. **Never duplicate code across packages.** If a function would be identical
    in two or more packages, put it in a shared package from the start. For
    drivers, shared file operations go in `internal/driver/fs/`. Check what
    already exists there before writing any file copy, MCP merge, hook merge,
    or skill install logic. See [docs/development.md](docs/development.md)
    for the driver development guide.

## Architecture

See [docs/architecture.md](docs/architecture.md) for pipelines, driver
interfaces, and data flows. See [docs/adr/](docs/adr/README.md) for
design decisions.

Key deps: cobra, yaml.v3, lipgloss, avfs, go-git, mockgen, testify.

## Testing

See [docs/development.md](docs/development.md) for conventions, mock
generation, VFS patterns, and the test example template.

## Commits

[Conventional Commits](https://www.conventionalcommits.org/). Scopes match
package names.

When the agent is Claude, end every commit with:

```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

## Brand

Theme colors and palette details are in [docs/development.md](docs/development.md).
CLI colors auto-detect dark/light terminal backgrounds.
