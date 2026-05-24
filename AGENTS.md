# Agent Instructions

Canonical guidance for any AI coding agent working in this repository.

**Progressive disclosure.** Every document in this repo is self-contained
and ordered most-critical-first. Never use `@`-includes, "read X first"
directives, or cross-file dependencies that force the reader to jump
elsewhere before they can act. If an agent stops reading at any heading,
they should have enough context to work safely up to that depth.

## What is this?

claudia is a git-free package manager for Claude Code plugins. It builds,
checksums, and distributes plugin archives (`.claudia` tarballs) so
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
3. **One test file per production file.** `foo.go` → `foo_test.go`. Never a
   test file named after a non-existent source file.
4. **`cmd/` is a thin shim.** No business logic. Parse flags, create context
   and VFS, call `pkg/`, format output. Testable logic lives in `pkg/`.
5. **Never expose `pkg/` types in `cmd/`** beyond what cobra needs.
6. **Never `//nolint:errcheck`.** Handle errors properly.
7. **Context and VFS.** Functions that iterate or do I/O accept `context.Context`
   and `avfs.VFS`. Check `ctx.Err()` in loops. Pure functions skip both.
8. **All output through `pkg/cli/`.** No raw `fmt.Fprintf` in `cmd/`.

## Architecture

```
cmd/               Cobra CLI shim (root, build, verify, install, list, sync, version)
pkg/archive/       Tarball creation and extraction
pkg/build/         Build pipeline orchestration
pkg/checksum/      Per-file SHA256 checksumming and verification
pkg/cli/           Themed terminal output (banner, colors)
pkg/fetcher/       Backend interface + drivers (file, http)
pkg/install/       Install pipeline orchestration
pkg/list/          List installed plugins
pkg/manifest/      claudia.yaml parsing and validation
pkg/metadata/      Git SHA, version, timestamp capture
pkg/plugin/        Claude Code plugin structure generation
pkg/sync/          Declarative sync from claudia-packages.yaml
pkg/verify/        Verify pipeline orchestration
```

- `cmd/` — thin shim: parse flags, create context + VFS, call `pkg/`, print output
- `pkg/` — public library API, consumable without the CLI
- Filesystem I/O via `avfs.VFS` (production: `osfs`, tests: `memfs`)
- `context.Context` threads from CLI through cancellable operations

Key deps: cobra, yaml.v3, lipgloss, avfs, crypto/sha256, archive/tar, compress/gzip.

## Testing

Tests use [AVFS](https://github.com/avfs/avfs) for virtual filesystem mocking.
Error injection: wrap `memfs` with a custom struct overriding methods to return
errors.

```go
func TestFunctionName(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name string
    }{
        {name: "happy path"},
        {name: "failure case"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
        })
    }
}
```

## Commits

[Conventional Commits](https://www.conventionalcommits.org/). Scopes match
package names: `cli`, `archive`, `build`, `checksum`, `fetcher`, `install`,
`list`, `manifest`, `metadata`, `plugin`, `sync`, `verify`.

When the agent is Claude, end every commit with:

```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

## Brand

Claude Code orange `#cc7c5e` (R:204 G:124 B:94). The `claude` theme in
`pkg/cli/` and `install.sh` ANSI escape must use the same color.
