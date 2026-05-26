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

## Architecture

```
cmd/                          Cobra CLI shim
internal/archive/             Tarball creation and extraction (internal)
internal/checksum/            SHA256 hashing (internal)
internal/cli/                 Themed output helpers (internal)
internal/fetcher/             Fetch drivers (file, http, git) (internal)
internal/fetcher/mocks/       Generated MockFetcher (internal)
internal/lockfile/            Lockfile I/O (internal)
internal/manifest/            agentpack.yaml parsing and validation (internal)
internal/metadata/            Git metadata capture (internal)
internal/plugin/              Plugin descriptor generation (internal)
pkg/build/                    Build pipeline orchestration
pkg/inspect/                  Archive inspection
pkg/install/                  Install pipeline orchestration
pkg/list/                     List installed plugins
pkg/list/mocks/               Generated MockRegistry
pkg/outdated/                 Outdated detection
pkg/outdated/mocks/           Generated MockRegistry, MockRemoteChecker
pkg/registry/                 Installed package tracking
pkg/remove/                   Safe package removal
pkg/remove/mocks/             Generated MockRegistry
pkg/sync/                     Declarative sync with injectable interfaces
pkg/sync/mocks/               Generated MockBuilder, MockInstaller
pkg/target/                   Target interface + registry
pkg/target/mocks/             Generated MockTarget
pkg/target/claudecode/        Claude Code target implementation
pkg/target/cursor/            Cursor target implementation
pkg/target/universal/         Universal target implementation
pkg/update/                   Update pipeline
pkg/update/mocks/             Generated MockRegistryLoader, MockInstaller
pkg/verify/                   Archive verification
```

Interfaces are defined WHERE CONSUMED (accept interfaces, return structs).
Each consuming package owns its interface definitions and generated mocks in a
`mocks/` subdirectory alongside it. Implementation-detail packages (cli,
checksum, metadata, plugin, lockfile) live under `internal/` and are not
importable outside this module.

Three primary driver interfaces: `Fetcher` (how to get content), `Target`
(where to install), and sync's `Builder`/`Installer` (testable pipeline
stages).

No binary executables in archives — security policy. MCP remote/ux only.

- `cmd/` — thin shim: parse flags, create context + VFS, call `pkg/`
- `pkg/` — public library API, consumable without the CLI
- `internal/` — implementation details, not importable outside the module
- `docs/adr/` — Architecture Decision Records for significant design choices
- Filesystem I/O via `avfs.VFS` (production: `osfs`, tests: `memfs`)
- `context.Context` threads from CLI through cancellable operations

Architecture decisions are recorded as ADRs in
[docs/adr/](docs/adr/README.md). Check there for context on any design
question before proposing changes.

Key deps: cobra, yaml.v3, lipgloss, avfs, go-git, mockgen, testify.

## Testing

Tests use [testify](https://github.com/stretchr/testify) for assertions and
[AVFS](https://github.com/avfs/avfs) for virtual filesystem mocking. Error
injection: wrap `memfs` with a custom struct overriding methods to return errors.

Assertions: use `assert` for non-fatal checks, `require` for fatal ones. Never
pass custom messages — the default output is sufficient. Never write custom
assertion helpers.

```go
func TestFunctionName(t *testing.T) {
    t.Parallel()
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr string
    }{
        {name: "happy path", input: "good", want: "good"},
        {name: "failure case", input: "", wantErr: "empty"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got, err := DoThing(tt.input)
            if tt.wantErr != "" {
                require.ErrorContains(t, err, tt.wantErr)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

## Commits

[Conventional Commits](https://www.conventionalcommits.org/). Scopes match
package names: `cli`, `archive`, `build`, `checksum`, `fetcher`, `inspect`,
`install`, `list`, `lockfile`, `manifest`, `metadata`, `outdated`, `plugin`,
`registry`, `remove`, `sync`, `target`, `update`, `verify`.

When the agent is Claude, end every commit with:

```
Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

## Brand

Theme colors and palette details are in [Development](docs/development.md).
CLI colors auto-detect dark/light terminal backgrounds.
