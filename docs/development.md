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
git clone https://github.com/retr0h/claudia.git
cd claudia
go test ./...
```

## Layout

```
cmd/               Cobra CLI commands (root, build, verify, version)
internal/archive/  Tarball creation and extraction (.claudia archives)
internal/checksum/ Per-file SHA256 checksumming and verification
internal/manifest/ claudia.yaml parsing and validation
internal/metadata/ Git SHA, version, timestamp capture
internal/plugin/   Claude Code plugin structure generation
```

## Common tasks

```bash
go test ./...              # run tests
go vet ./...               # vet
gofmt -l .                 # find unformatted files
go build -o claudia .      # build
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

**Every public function and method MUST have a table-driven test.** One table
per function, with rows covering both the happy path and every failure mode.

### File naming (non-negotiable)

**One test file per production file.** `archive.go` is tested by
`archive_test.go` -- never `helpers_archive_test.go` or similar.

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- **Subject line**: max 50 characters, imperative mood, capitalized, no period
- **Body**: wrap at 72 characters, separated from subject by a blank line
- **Format**: `type(scope): description`
- **Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`
- **Scopes**: `cli`, `archive`, `checksum`, `manifest`, `metadata`, `plugin`
