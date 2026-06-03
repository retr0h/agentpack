# Development Guide

## Prerequisites

Install tools using [mise](https://mise.jdx.dev/):

```bash
mise install
```

- **[Go](https://go.dev)** >= 1.25
- **[just](https://just.systems)** -- task runner

## Quick Start

```bash
git clone https://github.com/retr0h/agentpack.git
cd agentpack
go test ./...
```

## Common tasks

```bash
go test ./...              # run tests
go vet ./...               # vet
gofmt -l .                 # find unformatted files
go build -o agentpack .    # build
```

Or via just:

```bash
just build          # build binary
just test           # run tests
just ready          # fmt + vet + lint (pre-commit gate)
just run            # build and run
just clean          # remove binary
```

## Testing conventions

**Every public function MUST have a table-driven test.** One table per function,
with rows covering both the happy path and every failure mode.

**One test file per production file.** `archive.go` → `archive_test.go`.

### Assertions

Use [testify](https://github.com/stretchr/testify) — `assert` for non-fatal
checks, `require` for fatal ones. No stdlib `t.Errorf`/`t.Fatalf` for value
checks. No custom assertion helpers. No custom messages on assertions:

```go
// Good
assert.Equal(t, want, got)
require.NoError(t, err)
require.ErrorContains(t, err, "not found")

// Bad — no custom messages
assert.Equal(t, want, got, "values should match")

// Bad — no custom helpers
func assertContains(t *testing.T, ...) { ... }

// Bad — no stdlib assertions
if got != want { t.Errorf("got %v, want %v", got, want) }
```

### Virtual filesystem

Tests use [AVFS](https://github.com/avfs/avfs) for virtual filesystem:
production uses `osfs.NewWithNoIdm()`, tests use `memfs.New()`, error injection
wraps `memfs` with a custom struct overriding methods.

### Interface mocking

Use [mockgen](https://github.com/uber-go/mock) for interface mocks. Never
hand-roll mock structs. Generated mocks live in `mocks/` subdirectories
alongside the interface they mock.

Generate with `//go:generate` directives in `mocks/generate.go`:

    //go:generate go tool go.uber.org/mock/mockgen -destination=target.gen.go -package=mocks github.com/retr0h/agentpack/internal/target Target

Regenerate all mocks:

    go generate ./...

VFS error-injecting wrappers (wrapping `avfs.VFS` to return errors) are NOT
mocks — they are test decorators and are fine hand-rolled.

### Interfaces where consumed

Every package that calls another package defines a small interface for what it
needs. The interface is defined in the CONSUMING package, not the producing
package. The consuming code accepts the interface (nil defaults to the real
implementation) and calls through it.

This applies to `cmd/` too — each cmd file defines an unexported interface for
the `pkg/` type it uses:

```go
// cmd/add.go
type installer interface {
    Run(ctx context.Context, opts install.Options) (*install.Result, error)
}

var pkgInstaller installer = install.New()
```

Then `RunE` calls `pkgInstaller.Run(...)`.

## Driver development

Each target driver lives in its own subpackage under `internal/driver/`. Shared
filesystem operations live in the top-level `internal/driver/` package —
`CopyFile`, `CopyTreeIfExists`, `EnumerateFiles`, `InstallMCP`,
`InstallHooksJSON`, `InstallSkillEntry`.

**Never duplicate code across drivers.** Before writing a function in a driver,
check if `internal/driver/` already has it. If you need a function that two or
more drivers would share, put it in `internal/driver/` from the start — not in
one driver with a plan to "extract later."

Common patterns that MUST use shared code:

- Copying files/trees → `driver.CopyFile`, `driver.CopyTreeIfExists`
- Enumerating installed files → `driver.EnumerateFiles`
- Installing MCP from JSON → `driver.InstallMCP`
- Installing hooks from JSON → `driver.InstallHooksJSON`
- Installing a skill entry → `driver.InstallSkillEntry`

Drivers with non-standard formats (YAML config, TOML config, executable hook
scripts) implement their own handlers but still use `driver.*` for file
operations.

When creating a new driver, follow an existing driver of similar complexity:

- Skill-only → `forgecode/`
- Skill + MCP (JSON) → `cursor/`
- Skill + MCP + Hook (JSON) → `windsurf/`
- Skill + MCP (YAML) → `goose/`
- Skill + Config (TOML) → `codex/`

## Brand and theming

Two themes in `internal/cli/theme.go` — auto-detected via
`termenv.HasDarkBackground()`:

| Role   | Dark (`ThemeDark`) | Light (`ThemeLight`)   |
| ------ | ------------------ | ---------------------- |
| Accent | `#c678dd` magenta  | `#9b59b6` purple       |
| OK     | `#50fa7b` green    | `#27ae60` forest green |
| Err    | `#ff6ec7` pink     | `#e74c3c` red          |
| Info   | `#00d4ff` cyan     | `#2980b9` blue         |
| Tag    | `#ffb86c` orange   | `#d35400` burnt orange |
| Mute   | faint              | `#888888` gray         |

`install.sh` must use the same accent RGB as `ThemeDark` for the banner.

Color roles across CLI output:

- **Accent** — package names, headlines
- **White (plain)** — versions, file paths, important readable values
- **Tag** — targets/categories (claude-code, cursor, universal)
- **Info** — dates, timestamps
- **OK** — checkmarks, success states
- **Err** — failures, mismatches
- **Mute** — labels, SHAs, sources, secondary metadata

## Architecture Decision Records

See [docs/adr/README.md](adr/README.md) for format, conventions, and index.

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- **Format**: `type(scope): description`
- **Subject**: max 50 chars, imperative mood, no period
- **Body**: wrap at 72 chars, blank line after subject
- **Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`
- **Scopes**: match package names (e.g. `cli`, `install`, `target`, `registry`)
