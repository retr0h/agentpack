# Package Format

## claudia.yaml — build manifest

Declares what to package. Lives at the root of the plugin repo.

### Single-plugin

```yaml
name: my-plugin
version: 1.0.0
description: "My Claude Code plugin"

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
  - type: binary
    name: my-server
    src: bin/my-server
    args: ["--port", "3000"]
    env:
      MY_VAR: "value"
    platforms: [darwin-arm64, linux-amd64]
  - type: remote
    name: my-remote
    url: "https://mcp.example.com/v1"
  - type: ux
    name: my-ux
    package: "@mycompany/my-mcp-server"
    args: ["--verbose"]
binaries:
  - bin/my-tool
settings:
  - settings.json
```

### Multi-plugin

For repos where files don't follow Claude Code's layout:

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
| `category` | no | Claude Code plugin category |
| `skills` | no | Skill markdown files (glob or src/dest) |
| `commands` | no | Command markdown files (glob or src/dest) |
| `hooks` | no | hooks.json and hook scripts (glob or src/dest) |
| `agents` | no | Agent markdown files (glob or src/dest) |
| `mcp` | no | MCP server entries (see below) |
| `binaries` | no | Pre-built executables (glob or src/dest) |
| `settings` | no | JSON fragments for settings.json (glob or src/dest) |

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

| Field | binary | remote | ux |
|-------|--------|--------|----|
| `type` | required | required | required |
| `name` | required | required | required |
| `src` | required | - | - |
| `url` | - | required | - |
| `package` | - | - | required |
| `config` | - | optional | - |
| `args` | optional | - | optional |
| `env` | optional | - | - |
| `platforms` | optional | - | - |

The `config` field on `remote` includes an existing `.mcp.json` as-is
instead of generating one.

---

## claudia-packages.yaml — sync manifest

Declares what should be installed on the target machine. Used by
`claudia sync`.

```yaml
packages:
  - name: my-plugin
    source: https://drive.google.com/uc?id=1ABC&export=download
  - name: local-plugin
    source: ~/Downloads/local-plugin-1.0.0.claudia
  - name: s3-plugin
    source: s3://my-bucket/plugins/s3-plugin-1.0.0.claudia
```

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Plugin name (for display) |
| `source` | yes | Local path or URL to the `.claudia` archive |

Supported source schemes:

| Prefix | Backend |
|--------|---------|
| `/`, `./`, `~/` | Local file copy |
| `https://`, `http://` | HTTP download |
| `s3://` | AWS S3 (planned) |
| `gs://` | Google Cloud Storage (planned) |

---

## .claudia archive format

A `.claudia` file is a gzipped tarball (`.tar.gz` with a `.claudia`
extension). The internal layout mirrors `~/.claude/plugins/` so the end
user can install with `claudia install` or plain `tar`.

### Archive layout

```
my-plugin-1.0.0.claudia
  marketplaces/my-plugin/
    .claude-plugin/
      marketplace.json        # Generated — Claude Code marketplace descriptor
      plugin.json             # Generated — Claude Code plugin descriptor
    .claudia/
      metadata.json           # Generated — git SHA, version, timestamps
      checksums.txt           # Generated — per-file SHA256 checksums
      claudia.yaml            # Copy of the manifest for this plugin
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
      my-server               # Binary MCP servers
      .mcp.json               # Generated — MCP server config
    bin/
      my-tool                 # Pre-built binaries
    settings/
      settings.json           # Settings fragments
```

### Generated files

#### `.claude-plugin/marketplace.json`

Single-plugin marketplace pattern — the marketplace IS the plugin:

```json
{
  "name": "my-plugin",
  "owner": { "name": "John Dewey", "email": "john@dewey.ws" },
  "metadata": { "description": "My plugin", "version": "1.0.0" },
  "plugins": [{
    "name": "my-plugin",
    "source": "./",
    "description": "My plugin",
    "version": "1.0.0",
    "author": { "name": "John Dewey", "email": "john@dewey.ws" },
    "license": "MIT"
  }]
}
```

#### `.claude-plugin/plugin.json`

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "My plugin",
  "commands": ["./commands/init.md", "./commands/analyze.md"]
}
```

The `commands` array is populated from resolved command entries. Skills
are discovered by Claude Code via the directory structure.

#### `.claudia/metadata.json`

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

#### `.claudia/checksums.txt`

Every file in the archive (except `checksums.txt` itself) gets a SHA256
checksum. Format matches `sha256sum(1)` output — two spaces between
hash and path:

```
e3b0c44298fc1c14...  marketplaces/my-plugin/.claude-plugin/marketplace.json
a7ffc6f8bf1ed766...  marketplaces/my-plugin/skills/review.md
```

#### `.mcp.json` (MCP servers)

Claudia generates the `.mcp.json` that Claude Code needs to launch MCP
servers. The developer's config (e.g. `go run ./cmd/server`) won't work
on a machine without Go — the packaged version references the pre-built
binary.

**binary** — pre-built executable, launched via `${CLAUDE_PLUGIN_ROOT}`:

```json
{
  "mcpServers": {
    "my-server": {
      "command": "${CLAUDE_PLUGIN_ROOT}/mcp/my-server",
      "args": ["--port", "3000"],
      "env": { "MY_VAR": "value" }
    }
  }
}
```

**remote** — hosted endpoint, no binary ships:

```json
{ "mcpServers": { "my-remote": { "url": "https://mcp.example.com/v1" } } }
```

**ux** — npx package, end user must have the package runner:

```json
{
  "mcpServers": {
    "my-ux": { "command": "npx", "args": ["@mycompany/my-server", "--verbose"] }
  }
}
```

### Metadata directory

The `.claudia/` metadata directory lives inside `marketplaces/{name}/`,
not at the archive root. This means:

- **No collision** — each plugin's metadata is namespaced. Untarring ten
  plugins never stomps.
- **Self-contained** — delete `marketplaces/my-plugin/` and the metadata
  goes with it.
- **Follows Claude Code convention** — Claude Code puts `.claude-plugin/`
  inside the marketplace directory. `.claudia/` is the same pattern.
  Claude Code ignores it because it only looks for `.claude-plugin/`.

### Manual installation

As an alternative to `claudia install`, use plain `tar`:

```bash
tar xzf my-plugin-1.0.0.claudia -C ~/.claude/plugins/
```

First time only, enable the plugin in `~/.claude/settings.json`:

```json
{
  "enabledPlugins": {
    "my-plugin@my-plugin": true
  }
}
```

Subsequent updates: re-run the same `tar` command.
