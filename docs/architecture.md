# Architecture

## Overview

agentpack has three driver layers and two pipeline interfaces:

```
agentpack.yaml → [build] → .agentpack archive → [fetcher] → [target] → installed
                                                     ↑
                            agentpack-packages.yaml → [install] → fetcher + builder + installer
                            agentpack.lock ──────────────────────── SHA pinning
```

1. **Build** reads `agentpack.yaml`, resolves content, and produces a
   checksummed `.agentpack` archive (see [ADR-001](adr/001-agentpack-format.md)).
2. **Fetcher** retrieves content from a source (local file, HTTP, git).
3. **Target** installs content into the right locations for a specific AI
   coding agent.
4. **Install** orchestrates fetch → build → install from a declarative
   `agentpack-packages.yaml`, using locked SHAs for reproducibility
   (see [ADR-003](adr/003-dependency-management.md)).

## Driver interfaces

### Fetcher — how to GET content

```go
type Fetcher interface {
    Fetch(ctx context.Context, source string, dest string) error
}
```

| Driver        | Scheme                                | Status      |
|---------------|---------------------------------------|-------------|
| `FileFetcher` | `/`, `./`, `~/`                       | implemented |
| `HTTPFetcher` | `http://`, `https://`                 | implemented |
| `GitFetcher`  | `github.com/`, `gitlab.com/`, `*.git` | implemented |

`fetcher.New(source)` detects the scheme and returns the right driver.
The `GitFetcher` uses [go-git](https://github.com/go-git/go-git) for
clone caching and version checkout — no external git binary needed.

### Target — where to INSTALL content

```go
type Target interface {
    Name() string
    DisplayName() string
    Detect() bool
    Install(ctx context.Context, opts InstallOpts) error
    List() ([]InstalledPlugin, error)
}
```

| Driver       | Agent          | Install directory                 | Detect                   |
|--------------|----------------|-----------------------------------|--------------------------|
| `ClaudeCode` | Claude Code    | `.claude/skills/`, `/commands/`, `/agents/` | `~/.claude/` exists |
| `Cursor`     | Cursor         | `.cursor/rules/{name}/`           | `~/.cursor/` exists      |
| `Windsurf`   | Windsurf       | `.windsurf/rules/{name}/`         | `~/.windsurf/` exists    |
| `Copilot`    | GitHub Copilot | `.github/copilot/{name}/`         | `.github/` in cwd        |
| `Gemini`     | Gemini CLI     | `.gemini/skills/{name}/`          | `~/.gemini/` exists      |
| `Universal`  | `.agents/`     | `.agents/skills/{name}/`          | always                   |

Drivers self-register via `init()` + blank import in `cmd/root.go`.

### Install pipeline interfaces

The install pipeline (pkg/sync) uses injectable interfaces so it can be
tested with mockgen mocks without real git repos, builds, or installs:

```go
type Builder interface {
    Build(ctx context.Context, dir string) ([]build.Result, error)
}

type Installer interface {
    Install(ctx context.Context, source string) (*install.Result, error)
}
```

## Dependency management

See [ADR-003](adr/003-dependency-management.md) for the full model.

```
agentpack-packages.yaml   ← user-edited spec (what you want)
agentpack.lock            ← resolved snapshot (what you got)
.config/agentpack/packages/  ← runtime registry (what's installed)
```

`add` updates the yaml + lock. `install` reads the yaml + lock and
installs everything. `del` removes from yaml + lock + disk.

## Content types

Archives are organized by type, not by platform. No binary executables
are permitted (see [ADR-001](adr/001-agentpack-format.md)).

| Type     | Directory   | Description                            |
|----------|-------------|----------------------------------------|
| skills   | `skills/`   | Markdown skill files                   |
| commands | `commands/` | Slash command definitions              |
| hooks    | `hooks/`    | Hook definitions + scripts             |
| agents   | `agents/`   | Agent markdown definitions             |
| mcp      | `mcp/`      | MCP server config (remote and ux only) |
| settings | `settings/` | JSON fragments                         |

## Install flow

```
agentpack add github.com/org/repo
  │
  ├─ 1. Fetch    (fetcher.New → git/file/http)
  ├─ 2. Package  (auto-package into .agentpack archive)
  ├─ 3. Extract  (archive.Extract, rejects symlinks/traversal)
  ├─ 4. Verify   (checksum.Verify, SHA256 per file)
  ├─ 5. Detect   (target.Detect on each registered target)
  ├─ 6. Install  (target.Install for each detected agent)
  └─ 7. Record   (update agentpack-packages.yaml + agentpack.lock)
```

## Install flow (from manifest)

```
agentpack install
  │
  ├─ 1. Read agentpack-packages.yaml
  ├─ 2. Read agentpack.lock (if exists, use locked SHAs)
  │
  ├─ 3. For each package:
  │     ├─ source: → Installer.Install(source)
  │     └─ git:    → Fetcher.Fetch → Builder.Build → Installer.Install
  │
  └─ 4. Report results
```

For the full directory layout see [Development](development.md).
