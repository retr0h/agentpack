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
go build -o agentpack .      # build
```

Or via just:

```bash
just build          # build binary
just test           # run tests
just ready          # fmt + vet + lint (pre-commit gate)
just run            # build and run
just clean          # remove binary
```

## Layout

```
cmd/               Cobra CLI shim (root, build, verify, install, list, sync, version)
pkg/archive/       Tarball creation and extraction (.agentpack archives)
pkg/build/         Build pipeline orchestration
pkg/checksum/      Per-file SHA256 checksumming and verification
pkg/cli/           Themed terminal output (banner, colors)
pkg/fetcher/       Backend interface + drivers (file, http)
pkg/install/       Install pipeline orchestration
pkg/list/          List installed plugins
pkg/manifest/      agentpack.yaml parsing and validation
pkg/metadata/      Git SHA, version, timestamp capture
pkg/plugin/        Claude Code plugin structure generation
pkg/sync/          Declarative sync from agentpack-packages.yaml
pkg/verify/        Verify pipeline orchestration
```

## Testing conventions

**Every public function MUST have a table-driven test.** One table per
function, with rows covering both the happy path and every failure mode.

**One test file per production file.** `archive.go` → `archive_test.go`.

Tests use [AVFS](https://github.com/avfs/avfs) for virtual filesystem:
production uses `osfs.NewWithNoIdm()`, tests use `memfs.New()`, error
injection wraps `memfs` with a custom struct overriding methods.

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- **Format**: `type(scope): description`
- **Subject**: max 50 chars, imperative mood, no period
- **Body**: wrap at 72 chars, blank line after subject
- **Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`
- **Scopes**: `cli`, `archive`, `build`, `checksum`, `fetcher`, `install`,
  `list`, `manifest`, `metadata`, `plugin`, `sync`, `verify`
