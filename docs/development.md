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
cmd/               Cobra CLI shim (root, build, verify, version)
pkg/archive/       Tarball creation and extraction (.claudia archives)
pkg/build/         Build pipeline orchestration
pkg/checksum/      Per-file SHA256 checksumming and verification
pkg/cli/           Themed terminal output (banner, colors)
pkg/manifest/      claudia.yaml parsing and validation
pkg/metadata/      Git SHA, version, timestamp capture
pkg/plugin/        Claude Code plugin structure generation
pkg/verify/        Verify pipeline orchestration
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
per function, with rows covering both the happy path and every failure mode. No
ad-hoc `Test*` functions. Every test must follow this pattern:

```go
func TestFunctionName(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name string
        // ...fields
    }{
        {name: "happy path", ...},
        {name: "failure case", ...},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            // ...
        })
    }
}
```

### File naming (non-negotiable)

**One test file per production file.** `archive.go` is tested by
`archive_test.go` -- never `helpers_archive_test.go` or similar.

### Filesystem mocking

Tests use [AVFS](https://github.com/avfs/avfs) for virtual filesystem:

- Production: `osfs.NewWithNoIdm()` (real OS)
- Tests: `memfs.New()` (in-memory)
- Error injection: wrap `memfs` with a custom struct overriding methods

## Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

- **Subject line**: max 50 characters, imperative mood, capitalized, no period
- **Body**: wrap at 72 characters, separated from subject by a blank line
- **Format**: `type(scope): description`
- **Types**: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`
- **Scopes**: `cli`, `archive`, `build`, `checksum`, `manifest`, `metadata`,
  `plugin`, `theme`, `verify`
