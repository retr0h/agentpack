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
    drivers, shared file operations go in `internal/driver/`. Check what
    already exists there before writing any file copy, MCP merge, hook merge,
    or skill install logic. See [docs/development.md](docs/development.md)
    for the driver development guide.

## Bash Command Constraints

To avoid triggering the CLI environment's security parser blocks (e.g.,
`Contains simple_expansion`), adhere to these strict command formatting rules.
Prefer the dedicated Read/Grep/Edit/Write tools over shelling out — they bypass
this parser entirely and never prompt.

1. **No shell variables.** Never create or reference bash variables in a command
   (do NOT use `VALUE=x`, `$VALUE`, or `${VALUE}`).
2. **No command substitution.** Never use `$(command)` or backticks
   `` `command` `` to pass the output of one command into another.
3. **Multi-step execution, literal values only.** If you need the output of a
   command (a git SHA, a process ID, an env value), run that command by itself
   first, read the result from the transcript, then hardcode that exact literal
   string into the next command.
4. **No comments in multi-line quoted arguments.** Never write multi-line
   commands (`python -c`, `node -e`, shell blocks) where a newline is followed by
   a `#` comment inside quotes — it trips the parser's path validation check.
5. **Inline scripts.** If you must execute a script: strip all `#` comments when
   running inline; keep it on one line with semicolons where possible; or write
   it to a temp file with a quoted heredoc (`cat << 'EOF' > temp.py`), execute
   the file, then delete it.
6. **No redirections or pipes for I/O.** Never use `>`, `>>`, `<`, `<<`, `2>&1`,
   or `2>/dev/null`. The parser cannot statically analyze `file_redirect` syntax
   and halts for manual permission.
7. **Alternatives:**
   - **File generation/appends:** instead of redirecting stdout to a file, use
     the editing/file tools, or write a clean script that takes the path as an
     explicit argument.
   - **Input:** pass file paths as arguments instead of piping via `<` (use
     `grep "pattern" filename`, not `grep "pattern" < filename`).
   - **Streams:** do not append `2>&1` or error-silencing flags; let stdout and
     stderr flow naturally to the transcript.
8. **Never chain `cd` and `git`.** Avoid `cd path && git status` — it trips the
   guardrails. Use the native `-C` flag instead: `git -C path <command>`.
9. **Scratch files stay inside the repo.** Write coverage profiles, temp scripts,
   and any throwaway artifacts under the repo root or `./.tmp/` (gitignored) —
   NEVER `/tmp` or `/private/tmp`. Files outside the project directory always
   trigger a manual permission prompt no matter the allow-list. Name them so
   `.gitignore` catches them (`*.out`, `*.html`, `.tmp/`). Clean up with one
   `rm <file>` per command — no globs, no `2>&1`, no redirects.

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
