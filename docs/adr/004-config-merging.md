---
status: 'accepted'
date: 2026-05-25
---

# ADR-004: Config Merging for MCP, Hooks, and Settings

## Context and Problem Statement

Plugins can declare MCP servers, hooks, and settings fragments in their
archives. Currently these content types are silently ignored during install —
only skills, commands, and agents are copied as files.

MCP servers and hooks must be merged into the target's config file (e.g.
`.claude/settings.json` for Claude Code) rather than copied as standalone files.
This merge must be reversible so `del` can undo it.

## Decision Drivers

- Plugins declare MCP servers, hooks, and settings that must be active
- Config files (`.claude/settings.json`) are shared — can't overwrite
- Merges must be reversible so `del` can undo them cleanly
- Only Claude Code has structured config — other targets just copy files

## Considered Options

- **Copy-only (no merging)** — simple but MCP/hooks would be silently ignored,
  plugins couldn't fully configure agents
- **Separate config file per plugin** — avoids merge conflicts but Claude Code
  reads one `settings.json`, not per-plugin files
- **Full settings.json replacement** — destructive, loses user settings

## Decision Outcome

### Merge targets

| Content type | Claude Code target                              | Other targets  |
| ------------ | ----------------------------------------------- | -------------- |
| `mcp/`       | Merge into `.claude/settings.json` `mcpServers` | Not applicable |
| `hooks/`     | Merge into `.claude/settings.json` `hooks`      | Not applicable |
| `settings/`  | Merge into `.claude/settings.json`              | Not applicable |

Only the Claude Code target performs config merging. Other targets (Cursor,
Windsurf, Copilot, Gemini) only support skills — they ignore MCP, hooks, and
settings content types.

### MCP merge strategy

Each `mcp/*.json` file in the archive declares one MCP server:

```json
{
  "name": "my-api",
  "type": "remote",
  "url": "https://mcp.example.com/v1"
}
```

On install, the server is added to `.claude/settings.json` under `mcpServers`,
keyed by name:

```json
{
  "mcpServers": {
    "my-api": {
      "type": "remote",
      "url": "https://mcp.example.com/v1"
    }
  }
}
```

### Hooks merge strategy

`hooks/hooks.json` in the archive declares hook entries:

```json
{
  "PreToolUse": [
    {
      "matcher": "Bash",
      "hooks": [
        {
          "type": "command",
          "command": ".claude/hooks/my-plugin/lint.sh"
        }
      ]
    }
  ]
}
```

Hook entries are appended to the existing arrays in `.claude/settings.json`
under `hooks`. Each entry is tagged with the plugin name in a `_plugin` field so
it can be identified during removal.

### Settings merge strategy

`settings/*.json` files contain key-value fragments that are merged into the top
level of `.claude/settings.json`. Only keys declared in the fragment are set —
existing keys outside the fragment are not touched.

### Reversibility

The registry manifest (`.config/agentpack/packages/{name}.yaml`) already tracks
installed files. It is extended with a `configMerges` field that records exactly
what was added to each settings file:

```yaml
configMerges:
  - file: .claude/settings.json
    mcpServers: ['my-api']
    hooks:
      PreToolUse: [0] # index of the appended entry
```

On `del`, these recorded merges are reversed: MCP servers are removed by key,
hook entries are removed by `_plugin` tag, settings keys are removed if they
still match the plugin's value.

### Safety

- Never overwrite an existing MCP server with the same name (error)
- Never delete a hook entry that was modified by the user
- Read → modify → write with a file lock to prevent concurrent corruption
- Back up settings.json before first merge

## Consequences

- Plugins can now fully configure Claude Code (MCP, hooks, settings)
- Install becomes a write operation on config files, not just file copy
- `del` must parse and modify JSON, adding complexity
- Only Claude Code benefits initially — other targets skip config merge
- The `_plugin` tag in hooks is an agentpack convention, not a Claude Code
  standard
