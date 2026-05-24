# Architecture

## Overview

agentpack has three driver layers:

```
agentpack.yaml → [build] → .agentpack archive → [fetcher] → [target] → installed
```

1. **Build** reads `agentpack.yaml`, resolves content, and produces a
   generic `.agentpack` archive organized by content type.
2. **Fetcher** retrieves archives from a source (local file, HTTP, S3).
3. **Target** installs archive content into the right locations for a
   specific AI coding agent (Claude Code, Cursor, Copilot, etc.).

## Driver interfaces

### Fetcher — how to GET an archive

```go
// pkg/fetcher/fetcher.go
type Fetcher interface {
    Fetch(ctx context.Context, source string, dest string) error
}
```

| Driver | Scheme | Status |
|--------|--------|--------|
| `FileFetcher` | `/`, `./`, `~/` | implemented |
| `HTTPFetcher` | `http://`, `https://` | implemented |
| `S3Fetcher` | `s3://` | planned |
| `GCSFetcher` | `gs://` | planned |

`fetcher.New(source)` detects the scheme and returns the right driver.

### Target — where to INSTALL content

```go
// pkg/target/target.go
type Target interface {
    // Name returns the agent identifier (e.g. "claude-code", "cursor").
    Name() string

    // Detect returns true if this agent is installed on the system.
    Detect() bool

    // Install receives the unpacked archive contents and lays them into
    // the correct locations for this agent.
    Install(ctx context.Context, opts InstallOpts) error

    // List returns installed agentpack plugins for this agent.
    List() ([]InstalledPlugin, error)
}

type InstallOpts struct {
    Name     string            // plugin name
    Version  string            // plugin version
    Contents map[string][]byte // content-type → files
    Meta     *metadata.Metadata
}
```

| Driver | Agent | Skills dir | Status |
|--------|-------|------------|--------|
| `ClaudeCode` | Claude Code | `~/.claude/plugins/marketplaces/` | implemented |
| `Cursor` | Cursor | `~/.cursor/skills/` | planned |
| `Copilot` | GitHub Copilot | `~/.copilot/skills/` | planned |
| `GeminiCLI` | Gemini CLI | `~/.gemini/skills/` | planned |
| `Windsurf` | Windsurf | `~/.codeium/windsurf/skills/` | planned |
| `Codex` | OpenAI Codex | `~/.codex/skills/` | planned |
| `Universal` | `.agents/skills/` convention | `.agents/skills/` | planned |

Target paths sourced from the [agentskills.io](https://github.com/vercel-labs/skills)
`agents.ts` registry.

### How a target driver works

The archive contains content organized by type:

```
skills/review.md
commands/scan.md
hooks/hooks.json
agents/security-reviewer.md
mcp/.mcp.json
```

The Claude Code target driver:
1. Creates `~/.claude/plugins/marketplaces/{name}/`
2. Generates `.claude-plugin/marketplace.json` and `plugin.json`
3. Copies skills, commands, hooks, agents into the marketplace dir
4. Generates `.mcp.json` with `${CLAUDE_PLUGIN_ROOT}` paths
5. Writes `.agentpack/metadata.json` and `checksums.txt`

The Cursor target driver:
1. Copies skills to `~/.cursor/skills/{name}/`
2. Skills-only — hooks, MCP, binaries are Claude Code-specific

Each driver only installs what it understands. A target that doesn't
support hooks silently skips them.

## Content types

The archive format is generic. Content is organized by type, not by
platform:

| Type | Directory | Description |
|------|-----------|-------------|
| skills | `skills/` | Markdown skill files |
| commands | `commands/` | Slash command definitions |
| hooks | `hooks/` | Hook definitions + scripts |
| agents | `agents/` | Agent markdown definitions |
| mcp | `mcp/` | MCP server binaries + config |
| binaries | `binaries/` | Pre-built executables |
| settings | `settings/` | JSON fragments |

Not all agents support all types:

| Type | Claude Code | Cursor | Copilot | Gemini CLI |
|------|-------------|--------|---------|------------|
| skills | yes | yes | yes | yes |
| commands | yes | no | no | no |
| hooks | yes | no | no | no |
| agents | yes | no | no | no |
| mcp | yes | partial | no | no |
| binaries | yes | no | no | no |
| settings | yes | no | no | no |

## Install flow

```
agentpack install my-plugin-1.0.0.agentpack
  │
  ├─ 1. Fetch    (fetcher.New detects scheme → file/http/s3)
  │     └─ downloads archive to temp file
  │
  ├─ 2. Extract  (archive.Extract)
  │     └─ unpacks to temp dir, rejects symlinks/traversal
  │
  ├─ 3. Verify   (checksum.Verify)
  │     └─ SHA256 every file against checksums.txt
  │
  ├─ 4. Detect   (target.Detect on each registered target)
  │     └─ which agents are installed on this system?
  │
  └─ 5. Install  (target.Install for each detected agent)
        ├─ Claude Code: full install (marketplace structure)
        ├─ Cursor: skills only (to ~/.cursor/skills/)
        └─ Gemini CLI: skills only (to ~/.gemini/skills/)
```

## Sync flow

```
agentpack sync
  │
  ├─ 1. Read agentpack-packages.yaml
  │
  ├─ 2. For each package:
  │     ├─ Fetch archive
  │     ├─ Extract + verify
  │     └─ Install to all detected targets
  │
  └─ 3. Report results
```

## Package layout

```
cmd/                    Cobra CLI shim
pkg/archive/            Tarball creation and extraction
pkg/build/              Build pipeline orchestration
pkg/checksum/           Per-file SHA256 checksumming
pkg/cli/                Themed terminal output
pkg/fetcher/            Fetch drivers (file, http, s3, gcs)
pkg/install/            Install pipeline orchestration
pkg/list/               List installed plugins
pkg/manifest/           agentpack.yaml parsing
pkg/metadata/           Git metadata capture
pkg/plugin/             Plugin descriptor generation
pkg/sync/               Declarative sync
pkg/target/             Target drivers (claude-code, cursor, ...)
pkg/target/claudecode/  Claude Code target implementation
pkg/verify/             Archive verification
```
