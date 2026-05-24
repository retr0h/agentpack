# Package Format

A `.claudia` file is a gzipped tarball (`.tar.gz` with a `.claudia` extension).
The internal layout mirrors `~/.claude/plugins/` so the end user can install
with plain `tar` or `claudia install`.

## Archive layout

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

## Generated files

### `.claude-plugin/marketplace.json`

Single-plugin marketplace pattern — the marketplace IS the plugin:

```json
{
  "name": "my-plugin",
  "owner": { "name": "John Dewey", "email": "john@dewey.ws" },
  "metadata": { "description": "My plugin", "version": "1.0.0" },
  "plugins": [
    {
      "name": "my-plugin",
      "source": "./",
      "description": "My plugin",
      "version": "1.0.0",
      "author": { "name": "John Dewey", "email": "john@dewey.ws" },
      "license": "MIT"
    }
  ]
}
```

### `.claude-plugin/plugin.json`

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "My plugin",
  "commands": ["./commands/init.md", "./commands/analyze.md"]
}
```

The `commands` array is populated from resolved command entries. Skills are
discovered by Claude Code via the directory structure.

### `.claudia/metadata.json`

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

### `.claudia/checksums.txt`

Every file in the archive (except `checksums.txt` itself) gets a SHA256
checksum. Format matches `sha256sum(1)` output — two spaces between hash and
path:

```
e3b0c44298fc1c14...  marketplaces/my-plugin/.claude-plugin/marketplace.json
a7ffc6f8bf1ed766...  marketplaces/my-plugin/skills/review.md
```

### `.mcp.json` (MCP servers)

Claudia generates the `.mcp.json` that Claude Code needs to launch MCP servers.
The developer's config (e.g. `go run ./cmd/server`) won't work on a machine
without Go — the packaged version references the pre-built binary.

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
{
  "mcpServers": {
    "my-remote": { "url": "https://mcp.example.com/v1" }
  }
}
```

**ux** — npx package, end user must have the package runner:

```json
{
  "mcpServers": {
    "my-ux": {
      "command": "npx",
      "args": ["@mycompany/my-server", "--verbose"]
    }
  }
}
```

## Metadata directory

The `.claudia/` metadata directory lives inside `marketplaces/{name}/`, not at
the archive root. This means:

- **No collision** — each plugin's metadata is namespaced. Untarring ten plugins
  never stomps.
- **Self-contained** — delete `marketplaces/my-plugin/` and the metadata goes
  with it.
- **Follows Claude Code convention** — Claude Code puts `.claude-plugin/` inside
  the marketplace directory. `.claudia/` is the same pattern. Claude Code
  ignores it because it only looks for `.claude-plugin/`.

## Manual installation

Until `claudia install` is available, install with plain `tar`:

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

Subsequent updates: re-run the same `tar` command. The `enabledPlugins` entry
does not change between versions.
