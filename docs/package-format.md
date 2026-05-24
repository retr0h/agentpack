# Package Format

## agentpack.yaml — build manifest

Declares what to package. Lives at the root of the plugin repo.

### Single-plugin

```yaml
name: my-plugin
version: 1.0.0
description: "My security toolkit"

author:
  name: "John Dewey"
  email: "john@dewey.ws"
license: MIT
homepage: "https://github.com/retr0h/my-plugin"
keywords: [security, hooks]
category: security

skills:
  - skills/*.md
commands:
  - commands/*.md
hooks:
  - hooks/hooks.json
  - hooks/*.sh
agents:
  - agents/*.md
mcp:
  - type: remote
    name: my-remote
    url: "https://mcp.example.com/v1"
  - type: ux
    name: my-ux
    package: "@mycompany/my-mcp-server"
    args: ["--verbose"]
settings:
  - settings.json
```

### Multi-plugin

For repos where files don't follow standard layout:

```yaml
author:
  name: "John Dewey"
  email: "john@dewey.ws"
license: MIT

plugins:
  - name: zeek-pros
    version: 1.0.0
    description: "Zeek protocol analysis"
    skills:
      - src: prompts/zeek/*.md
        dest: skills/
  - name: git-commands
    version: 2.1.0
    description: "Git workflow commands"
    commands:
      - src: cli/git/*.md
        dest: commands/
```

Shared fields (`author`, `license`, `homepage`) cascade to all plugins.
Each plugin can override any shared field.

### Field reference

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Plugin name (kebab-case) |
| `version` | yes | SemVer version string |
| `description` | yes | One-line description |
| `author` | no | `name` and `email` |
| `license` | no | SPDX license identifier |
| `homepage` | no | Project URL |
| `keywords` | no | Search/discovery tags |
| `category` | no | Plugin category |
| `skills` | no | Skill markdown files (glob or src/dest) |
| `commands` | no | Command markdown files (glob or src/dest) |
| `hooks` | no | hooks.json and hook scripts (glob or src/dest) |
| `agents` | no | Agent markdown files (glob or src/dest) |
| `mcp` | no | MCP server entries — `remote` and `ux` only (see below) |
| `settings` | no | JSON fragments (glob or src/dest) |

### Content entry formats

**Simple glob** — files already in the right place:

```yaml
skills:
  - skills/*.md
```

**Source-to-destination mapping** — files need remapping:

```yaml
skills:
  - src: prompts/zeek/*.md
    dest: skills/
```

When `dest` ends with `/`, the source filename is preserved. When `dest`
is a full path (e.g. `skills/review.md`), the file is renamed.

### MCP entry types

Binary executables are not permitted in agentpack archives. The `binary` MCP
type is rejected at build time. Use `remote` or `ux` instead.

| Field | remote | ux |
|-------|--------|----|
| `type` | required | required |
| `name` | required | required |
| `url` | required | - |
| `package` | - | required |
| `config` | optional | - |
| `args` | - | optional |
| `env` | - | - |

---

## agentpack-packages.yaml — sync manifest

Declares what should be installed on the target machine. Used by
`agentpack sync`.

```yaml
packages:
  - name: my-plugin
    source: https://drive.google.com/uc?id=1ABC&export=download
  - name: local-plugin
    source: ~/Downloads/local-plugin-1.0.0.agentpack
  - name: git-plugin
    git: github.com/org/my-plugin
  - name: git-plugin-pinned
    git: github.com/org/my-plugin
    ref: v2.1.0
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Plugin name (for display) |
| `source` | no | Local path or URL to a `.agentpack` archive |
| `git` | no | Git repository (e.g. `github.com/org/repo`) |
| `ref` | no | Tag, branch, or SHA to check out (default: HEAD) |

Exactly one of `source` or `git` must be set per package entry.

When `git` is set, the sync pipeline:
1. Clones the repository (shallow clone) using `GitFetcher`
2. Builds all plugins defined in `agentpack.yaml` at the repo root
3. Installs each resulting archive to all detected targets

Supported source schemes:

| Prefix | Backend |
|--------|---------|
| `/`, `./`, `~/` | Local file copy |
| `https://`, `http://` | HTTP download |
| `github.com/`, `gitlab.com/`, `bitbucket.org/` | Git clone |
| `*.git` suffix | Git clone |
| `s3://` | AWS S3 (planned) |
| `gs://` | Google Cloud Storage (planned) |

---

## .agentpack archive format

A `.agentpack` file is a gzipped tarball. The internal layout is
**generic** — organized by content type, not by target platform. The
target driver (Claude Code, Cursor, etc.) handles translation to
platform-specific layout at install time.

### Archive layout

```
my-plugin-1.0.0.agentpack
  .agentpack/
    manifest.yaml             # Copy of the agentpack.yaml for this plugin
    metadata.json             # Git SHA, version, timestamps
    checksums.txt             # Per-file SHA256 checksums
  skills/
    *.md
  commands/
    *.md
  hooks/
    hooks.json
    *.sh
  agents/
    *.md
  mcp/
    .mcp.json                 # Generated MCP config (remote/ux types)
  settings/
    settings.json             # Settings fragments
```

The archive is platform-agnostic. Content is grouped by type so that
each target driver can pick what it supports and ignore the rest.

### Generated files

#### `.agentpack/metadata.json`

Captured at build time from the git repo:

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "gitCommitSha": "a1b2c3d4e5f6...",
  "gitBranch": "main",
  "buildTimestamp": "2026-05-23T14:30:00Z",
  "builderVersion": "dev",
  "platform": "darwin-arm64"
}
```

#### `.agentpack/checksums.txt`

Every file in the archive (except `checksums.txt` itself) gets a SHA256
checksum. Format matches `sha256sum(1)` — two spaces between hash and
path:

```
e3b0c44298fc1c14...  skills/review.md
a7ffc6f8bf1ed766...  commands/scan.md
b94d27b9934d3e08...  .agentpack/metadata.json
```

#### `.mcp.json` (MCP servers)

agentpack generates the `.mcp.json` from the manifest. Binary MCP servers are
not supported. Use `remote` (a hosted endpoint) or `ux` (an npx package).

**remote** — hosted endpoint:

```json
{ "mcpServers": { "my-remote": { "url": "https://mcp.example.com/v1" } } }
```

**ux** — npx package:

```json
{
  "mcpServers": {
    "my-ux": { "command": "npx", "args": ["@mycompany/my-server"] }
  }
}
```

### Target driver translation

The archive is generic. When installing, the target driver translates
it to the platform's expected layout:

**Claude Code** target produces:
```
~/.claude/plugins/marketplaces/my-plugin/
  .claude-plugin/marketplace.json    # generated by driver
  .claude-plugin/plugin.json         # generated by driver
  .agentpack/metadata.json
  .agentpack/checksums.txt
  skills/*.md
  commands/*.md
  hooks/hooks.json
  mcp/.mcp.json                      # ${CLAUDE_PLUGIN_ROOT} paths
```

**Cursor** target produces:
```
~/.cursor/skills/my-plugin/
  *.md                               # skills only
```

**Universal** target (`.agents/skills/` convention):
```
.agents/skills/my-plugin/
  *.md                               # skills only
```

Each driver installs what it understands and silently skips the rest.
See [architecture.md](architecture.md) for the full driver design.
